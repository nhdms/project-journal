// Package index maintains a derived index over the journal's JSONL data.
//
// JSONL files in the journal data directory are the source of truth: they
// are human-readable, atomic-replaceable, and trivially diffable. The index
// is a derived, columnar mirror used for analytical queries (status counts,
// vector similarity for context briefings, etc.) that would be slow over
// raw JSONL at scale.
//
// The index is always reconstructable from JSONL via Rebuild, so corruption
// or schema mismatches are non-fatal: the index is wiped and rebuilt on
// next open.
//
// Two implementations are provided behind build tags:
//
//   - default (no build tag): a noop implementation that does nothing.
//     Callers fall back to JSONL-based code paths. Pure Go, no CGO.
//   - `pj_duckdb`: a DuckDB-backed implementation. Requires CGO. Provides
//     full mirror of tasks, phases, embeddings, and trajectory plus SQL
//     vector similarity via array_cosine_similarity.
//
// The package-level For(dir) function returns the index for a given data
// directory, opening it lazily and caching it for the process lifetime.
// All methods are safe to call concurrently — the underlying implementation
// is responsible for synchronization. Callers should already hold the
// store-level file lock (store.withLock) when invoking write methods.
package index

import (
	"sync"
	"time"

	"github.com/nhdms/project-journal/internal/model"
)

// EmbeddingRecord mirrors store.EmbeddingRecord. Duplicated here to avoid an
// import cycle: store depends on this package, not the other way around.
type EmbeddingRecord struct {
	TaskID    string
	Text      string
	Embedding []float32
	UpdatedAt time.Time
}

// SimilarTask is a result row from SearchSimilar.
type SimilarTask struct {
	TaskID     string
	Similarity float64
}

// Index is a derived view over the journal's JSONL data. Implementations
// must be safe for concurrent use; in practice callers serialize writes
// via store.withLock.
type Index interface {
	// UpsertTask mirrors a task record. Idempotent.
	UpsertTask(t model.Task) error

	// UpsertPhase mirrors a phase record. Idempotent.
	UpsertPhase(p model.Phase) error

	// UpsertEmbedding mirrors an embedding record. Idempotent.
	UpsertEmbedding(rec EmbeddingRecord) error

	// AppendTrajectory appends a trajectory event for taskID. Idempotent on
	// (task_id, ts, type) — duplicate appends silently no-op.
	AppendTrajectory(taskID string, ev model.TrajectoryEvent) error

	// SearchSimilar returns the topK tasks whose embeddings have the highest
	// cosine similarity to query. Filter by allowedStatuses if non-empty.
	// Returns an empty slice if the index has no embeddings.
	SearchSimilar(query []float32, topK int, allowedStatuses []string) ([]SimilarTask, error)

	// Rebuild wipes and re-populates the index from the provided JSONL data.
	// Used on first open, schema mismatch, or explicit `pj reindex --full`.
	Rebuild(tasks []model.Task, phases []model.Phase, embeddings []EmbeddingRecord) error

	// Close releases any resources (open DB handles, file locks).
	Close() error

	// TableCount returns the number of rows in the named table. Allowed names:
	// "tasks", "phases", "embeddings", "trajectory", "schema_meta". Returns an
	// error for any other name. Intended for tests; the noop implementation
	// always returns 0, nil.
	TableCount(table string) (int, error)

	// GetTaskStatus returns the status column for the task with the given id.
	// Returns ("", nil) if the task does not exist. Intended for tests; the
	// noop implementation always returns "", nil.
	GetTaskStatus(id string) (string, error)
}

var (
	mu       sync.Mutex
	registry = map[string]Index{}
)

// For returns the index for the journal data directory dir. The first
// successful call opens the index; subsequent calls return the cached
// instance. If opening fails (typically because another process holds the
// DuckDB lock), a noop index is returned along with the error and NO
// caching is performed — subsequent calls will retry, so mirroring
// resumes once the contending process releases the lock. Callers should
// log the error but not treat it as fatal: JSONL is the source of truth.
func For(dir string) (Index, error) {
	mu.Lock()
	defer mu.Unlock()
	if idx, ok := registry[dir]; ok {
		return idx, nil
	}
	idx, err := open(dir)
	if err != nil {
		// Do NOT cache: contention is transient. Returning an uncached
		// noop lets the next call retry once the holder releases.
		return newNoop(), err
	}
	registry[dir] = idx
	return idx, nil
}

// CloseAll releases all cached indexes. Intended for tests and graceful
// shutdown. After CloseAll, the next For() call reopens the index.
func CloseAll() error {
	mu.Lock()
	defer mu.Unlock()
	var firstErr error
	for dir, idx := range registry {
		if err := idx.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(registry, dir)
	}
	return firstErr
}

// resetForTest clears the registry without closing — used by tests that
// need a fresh index in the same process.
func resetForTest() {
	mu.Lock()
	defer mu.Unlock()
	for _, idx := range registry {
		_ = idx.Close()
	}
	registry = map[string]Index{}
}
