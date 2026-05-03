package store

import (
	"fmt"
	"os"
	"sync"

	"github.com/nhdms/project-journal/internal/model"
	"github.com/nhdms/project-journal/internal/store/index"
)

// mirrorTask upserts t into the derived index. JSONL is the source of
// truth; index errors are non-fatal (the index rebuilds itself from JSONL
// on next `pj reindex --full`).
func mirrorTask(dir string, t model.Task) error {
	idx, err := index.For(dir)
	if err != nil {
		return nil
	}
	_ = idx.UpsertTask(t)
	return nil
}

func mirrorPhase(dir string, p model.Phase) error {
	idx, err := index.For(dir)
	if err != nil {
		return nil
	}
	_ = idx.UpsertPhase(p)
	return nil
}

func mirrorEmbedding(dir string, rec EmbeddingRecord) error {
	idx, err := index.For(dir)
	if err != nil {
		return nil
	}
	_ = idx.UpsertEmbedding(index.EmbeddingRecord{
		TaskID:    rec.TaskID,
		Text:      rec.Text,
		Embedding: rec.Embedding,
		UpdatedAt: rec.UpdatedAt,
	})
	return nil
}

func mirrorTrajectory(dir, taskID string, ev model.TrajectoryEvent) error {
	idx, err := index.For(dir)
	if err != nil {
		return nil
	}
	_ = idx.AppendTrajectory(taskID, ev)
	return nil
}

// RebuildIndex wipes the derived index and re-populates it from the JSONL
// data under l. Safe to call concurrently with reads but should not race
// with writes — callers should hold the journal lock or quiesce writers.
func RebuildIndex(l Layout) error {
	tasks, err := LoadTasks(l)
	if err != nil {
		return err
	}
	phases, err := LoadPhases(l)
	if err != nil {
		return err
	}
	embs, err := LoadEmbeddings(l)
	if err != nil {
		return err
	}
	idx, err := index.For(l.Dir)
	if err != nil {
		return err
	}
	idxEmbs := make([]index.EmbeddingRecord, 0, len(embs))
	for _, e := range embs {
		idxEmbs = append(idxEmbs, index.EmbeddingRecord{
			TaskID:    e.TaskID,
			Text:      e.Text,
			Embedding: e.Embedding,
			UpdatedAt: e.UpdatedAt,
		})
	}
	return idx.Rebuild(tasks, phases, idxEmbs)
}

// IndexEnabled reports whether a real (non-noop) derived index is compiled
// in. Callers can use this to short-circuit expensive load paths.
func IndexEnabled() bool { return index.Enabled() }

// SearchSimilar exposes index-backed cosine similarity search to callers
// that prefer it over scanning JSONL embeddings. Returns nil if no real
// index is compiled in or no embeddings exist.
//
// On the first call per process for a given data dir, SearchSimilar runs
// EnsureIndexFresh — if the index has drifted from JSONL (e.g. mirror
// writes were skipped due to v0.5 lock contention, or the index file was
// just created on a v0.4.x journal), the index is rebuilt before serving
// the query. Subsequent calls in the same process skip this check, since
// in-process mirror writes keep both layers in lockstep.
func SearchSimilar(l Layout, query []float32, topK int, allowedStatuses []string) ([]index.SimilarTask, error) {
	EnsureIndexFresh(l)
	idx, err := index.For(l.Dir)
	if err != nil {
		return nil, err
	}
	return idx.SearchSimilar(query, topK, allowedStatuses)
}

// driftCheckedDirs records data dirs that have already been drift-checked
// in the current process so we don't pay the cost on every SearchSimilar
// call. Mirror writes keep the index in sync within a process, so one
// check per process per dir is sufficient.
var driftCheckedDirs sync.Map // map[string]struct{}

// EnsureIndexFresh runs IndexDrift once per process per data dir. On
// detected drift, it rebuilds the index from JSONL and logs one line to
// stderr. All errors are non-fatal: callers proceed with whatever the
// index currently contains, falling back to JSONL paths where possible.
//
// Idempotent and cheap on the second-and-later call: the per-process
// cache short-circuits to a single sync.Map load.
func EnsureIndexFresh(l Layout) {
	if !index.Enabled() {
		return
	}
	if _, done := driftCheckedDirs.Load(l.Dir); done {
		return
	}
	// Mark as checked BEFORE the (potentially slow) check so concurrent
	// callers within the same process don't run the check twice.
	driftCheckedDirs.Store(l.Dir, struct{}{})

	r, err := IndexDrift(l)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pj: index drift check failed: %v\n", err)
		return
	}
	if !r.Drift {
		return
	}
	fmt.Fprintf(os.Stderr,
		"pj: derived index out of sync (tasks: JSONL=%d index=%d, embeddings: JSONL=%d index=%d) — rebuilding…\n",
		r.TasksJSONL, r.TasksIndex, r.EmbeddingsJSONL, r.EmbeddingsIndex,
	)
	if err := RebuildIndex(l); err != nil {
		fmt.Fprintf(os.Stderr, "pj: index rebuild failed: %v (run `pj reindex --index-only` to retry)\n", err)
	}
}

// resetDriftCheckCacheForTest clears the per-process drift-check cache.
// Test-only — callers in production should never invalidate the cache,
// since mirror writes maintain lock-step within a process.
func resetDriftCheckCacheForTest() {
	driftCheckedDirs.Range(func(k, _ any) bool {
		driftCheckedDirs.Delete(k)
		return true
	})
}

// IndexDrift reports the row-count delta between the JSONL source of
// truth and the derived index. Drift is true when any of tasks/phases/
// embeddings have a mismatched count. Callers can use this to decide
// whether to auto-rebuild on startup.
//
// Intentionally simple: row counts catch the common drift causes
// (mirror skipped due to lock contention, schema-version wipe followed
// by no rebuild, JSONL edited externally). It does not detect content
// drift (same count, different bytes) — that is rare and requires a
// `pj reindex --full` to repair anyway.
//
// Returns Err == nil and Drift == false when no real index is compiled
// in, since the noop has nothing to drift from.
type IndexDriftReport struct {
	TasksJSONL      int
	TasksIndex      int
	PhasesJSONL     int
	PhasesIndex     int
	EmbeddingsJSONL int
	EmbeddingsIndex int
	Drift           bool
}

func IndexDrift(l Layout) (IndexDriftReport, error) {
	r := IndexDriftReport{}
	if !index.Enabled() {
		return r, nil
	}
	tasks, err := LoadTasks(l)
	if err != nil {
		return r, err
	}
	phases, err := LoadPhases(l)
	if err != nil {
		return r, err
	}
	embs, err := LoadEmbeddings(l)
	if err != nil {
		return r, err
	}
	idx, err := index.For(l.Dir)
	if err != nil {
		// Index unavailable (e.g. another process holds the lock). Treat
		// as drift = false: we can't measure, and the caller's mirror
		// writes are also being skipped, so the on-disk state is
		// consistent with itself.
		return r, nil
	}
	r.TasksJSONL = len(tasks)
	r.PhasesJSONL = len(phases)
	r.EmbeddingsJSONL = len(embs)
	if r.TasksIndex, err = idx.TableCount("tasks"); err != nil {
		return r, err
	}
	if r.PhasesIndex, err = idx.TableCount("phases"); err != nil {
		return r, err
	}
	if r.EmbeddingsIndex, err = idx.TableCount("embeddings"); err != nil {
		return r, err
	}
	r.Drift = r.TasksJSONL != r.TasksIndex ||
		r.PhasesJSONL != r.PhasesIndex ||
		r.EmbeddingsJSONL != r.EmbeddingsIndex
	return r, nil
}
