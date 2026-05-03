//go:build pj_duckdb

package index

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nhdms/project-journal/internal/model"

	_ "github.com/marcboeker/go-duckdb/v2"
)

// schemaVersion is the on-disk schema epoch. Bump when CREATE TABLE
// statements change. On mismatch, the index is wiped and re-created;
// callers are expected to call Rebuild() to repopulate from JSONL.
const schemaVersion = 1

// duckdbIndex is the DuckDB-backed implementation of Index.
type duckdbIndex struct {
	mu sync.Mutex
	db *sql.DB
}

// open opens (or creates) the DuckDB index file at <dir>/index.duckdb.
// On schema_version mismatch the index is wiped and re-initialized empty;
// callers should detect emptiness (e.g. via Rebuild on first run) and
// repopulate from JSONL.
func open(dir string) (Index, error) {
	dbPath := filepath.Join(dir, "index.duckdb")
	db, err := sql.Open("duckdb", dbPath)
	if err != nil {
		return nil, fmt.Errorf("duckdb open %s: %w", dbPath, err)
	}
	// DuckDB Go driver multiplexes a single underlying connection by default;
	// keep it explicit for predictable lock behavior.
	db.SetMaxOpenConns(1)

	idx := &duckdbIndex{db: db}
	if err := idx.bootstrap(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return idx, nil
}

// Enabled reports whether a real (non-noop) index is compiled in.
func Enabled() bool { return true }

func newNoop() Index { return noopFallback{} }

// noopFallback is returned when DuckDB open fails inside For(). It mirrors
// the !pj_duckdb noop implementation.
type noopFallback struct{}

func (noopFallback) UpsertTask(model.Task) error                      { return nil }
func (noopFallback) UpsertPhase(model.Phase) error                    { return nil }
func (noopFallback) UpsertEmbedding(EmbeddingRecord) error            { return nil }
func (noopFallback) AppendTrajectory(string, model.TrajectoryEvent) error {
	return nil
}
func (noopFallback) SearchSimilar([]float32, int, []string) ([]SimilarTask, error) {
	return nil, nil
}
func (noopFallback) Rebuild([]model.Task, []model.Phase, []EmbeddingRecord) error {
	return nil
}
func (noopFallback) Close() error { return nil }

// bootstrap creates the schema if missing or wipes+recreates on version
// mismatch. The schema_meta table holds a single row recording the on-disk
// schema epoch; if it disagrees with schemaVersion the database is wiped.
func (idx *duckdbIndex) bootstrap() error {
	if _, err := idx.db.Exec(`CREATE TABLE IF NOT EXISTS schema_meta (version INTEGER PRIMARY KEY)`); err != nil {
		return fmt.Errorf("duckdb: create schema_meta: %w", err)
	}
	var got int
	row := idx.db.QueryRow(`SELECT version FROM schema_meta LIMIT 1`)
	switch err := row.Scan(&got); {
	case err == sql.ErrNoRows:
		got = 0
	case err != nil:
		return fmt.Errorf("duckdb: read schema_meta: %w", err)
	}
	if got != schemaVersion {
		if err := idx.wipe(); err != nil {
			return err
		}
	}
	if err := idx.createSchema(); err != nil {
		return err
	}
	if _, err := idx.db.Exec(`DELETE FROM schema_meta`); err != nil {
		return fmt.Errorf("duckdb: clear schema_meta: %w", err)
	}
	if _, err := idx.db.Exec(`INSERT INTO schema_meta (version) VALUES (?)`, schemaVersion); err != nil {
		return fmt.Errorf("duckdb: write schema_meta: %w", err)
	}
	return nil
}

// wipe drops every user table. Safe to call when the database is empty.
func (idx *duckdbIndex) wipe() error {
	for _, t := range []string{"trajectory", "embeddings", "tasks", "phases"} {
		if _, err := idx.db.Exec(`DROP TABLE IF EXISTS ` + t); err != nil {
			return fmt.Errorf("duckdb: drop %s: %w", t, err)
		}
	}
	return nil
}

// createSchema creates the user tables if they don't already exist.
// Embeddings use a variable-length FLOAT[] so the same schema supports
// any embedding dimension (text-embedding-3-small=1536, -3-large=3072).
func (idx *duckdbIndex) createSchema() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS phases (
			id          VARCHAR PRIMARY KEY,
			title       VARCHAR NOT NULL,
			created_at  TIMESTAMP NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS tasks (
			id                  VARCHAR PRIMARY KEY,
			phase_id            VARCHAR,
			title               VARCHAR NOT NULL,
			user_intent         TEXT,
			summary             TEXT,
			status              VARCHAR NOT NULL,
			session_id          VARCHAR,
			tags                VARCHAR[],
			depends_on          VARCHAR[],
			files_touched       VARCHAR[],
			key_decisions       VARCHAR[],
			todos_left          VARCHAR[],
			blockers_resolved   VARCHAR[],
			interfaces_exposed  VARCHAR[],
			started_at          TIMESTAMP,
			ended_at            TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS embeddings (
			task_id    VARCHAR PRIMARY KEY,
			text       TEXT NOT NULL,
			embedding  FLOAT[] NOT NULL,
			updated_at TIMESTAMP NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS trajectory (
			task_id        VARCHAR NOT NULL,
			ts             TIMESTAMP NOT NULL,
			type           VARCHAR NOT NULL,
			tool           VARCHAR,
			content        TEXT,
			input_summary  TEXT,
			output_summary TEXT,
			PRIMARY KEY (task_id, ts, type)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_phase  ON tasks(phase_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status)`,
		`CREATE INDEX IF NOT EXISTS idx_traj_task    ON trajectory(task_id)`,
	}
	for _, s := range stmts {
		if _, err := idx.db.Exec(s); err != nil {
			return fmt.Errorf("duckdb: %s: %w", firstLine(s), err)
		}
	}
	return nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

// UpsertTask mirrors a task into the tasks table.
func (idx *duckdbIndex) UpsertTask(t model.Task) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	const q = `
		INSERT OR REPLACE INTO tasks (
			id, phase_id, title, user_intent, summary, status, session_id,
			tags, depends_on, files_touched, key_decisions, todos_left,
			blockers_resolved, interfaces_exposed, started_at, ended_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := idx.db.Exec(q,
		t.ID,
		nilIfEmpty(t.PhaseID),
		t.Title,
		nilIfEmpty(t.UserIntent),
		nilIfEmpty(t.Summary),
		t.Status,
		nilIfEmpty(t.SessionID),
		stringList(t.Tags),
		stringList(t.DependsOn),
		stringList(t.FilesTouched),
		stringList(t.KeyDecisions),
		stringList(t.TodosLeft),
		stringList(t.BlockersResolved),
		stringList(t.InterfacesExposed),
		nilIfNilTime(t.StartedAt),
		nilIfNilTime(t.EndedAt),
	)
	if err != nil {
		return fmt.Errorf("duckdb: upsert task %s: %w", t.ID, err)
	}
	return nil
}

// UpsertPhase mirrors a phase.
func (idx *duckdbIndex) UpsertPhase(p model.Phase) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	_, err := idx.db.Exec(
		`INSERT OR REPLACE INTO phases (id, title, created_at) VALUES (?, ?, ?)`,
		p.ID, p.Title, p.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("duckdb: upsert phase %s: %w", p.ID, err)
	}
	return nil
}

// UpsertEmbedding mirrors an embedding row.
func (idx *duckdbIndex) UpsertEmbedding(rec EmbeddingRecord) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	_, err := idx.db.Exec(
		`INSERT OR REPLACE INTO embeddings (task_id, text, embedding, updated_at) VALUES (?, ?, ?, ?)`,
		rec.TaskID, rec.Text, float32List(rec.Embedding), rec.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("duckdb: upsert embedding %s: %w", rec.TaskID, err)
	}
	return nil
}

// AppendTrajectory appends an event. Duplicate (task_id, ts, type) tuples
// silently no-op via INSERT OR IGNORE — JSONL allows them, but the columnar
// mirror only needs one copy for analytical queries.
func (idx *duckdbIndex) AppendTrajectory(taskID string, ev model.TrajectoryEvent) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	_, err := idx.db.Exec(
		`INSERT OR IGNORE INTO trajectory (task_id, ts, type, tool, content, input_summary, output_summary)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		taskID, ev.Timestamp, ev.Type,
		nilIfEmpty(ev.Tool), nilIfEmpty(ev.Content),
		nilIfEmpty(ev.InputSummary), nilIfEmpty(ev.OutputSummary),
	)
	if err != nil {
		return fmt.Errorf("duckdb: append trajectory %s: %w", taskID, err)
	}
	return nil
}

// SearchSimilar returns the topK tasks ranked by cosine similarity between
// the query vector and each embedding. Filters by allowedStatuses if set.
func (idx *duckdbIndex) SearchSimilar(query []float32, topK int, allowedStatuses []string) ([]SimilarTask, error) {
	if len(query) == 0 || topK <= 0 {
		return nil, nil
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()

	var (
		sb   strings.Builder
		args []any
	)
	// list_cosine_similarity works on variable-length FLOAT[] lists.
	// (array_cosine_similarity requires fixed-size FLOAT[N] arrays.)
	sb.WriteString(`SELECT e.task_id,
	       list_cosine_similarity(e.embedding, CAST(? AS FLOAT[])) AS sim
	  FROM embeddings e`)
	args = append(args, float32List(query))
	if len(allowedStatuses) > 0 {
		sb.WriteString(` JOIN tasks t ON t.id = e.task_id WHERE t.status IN (`)
		for i, s := range allowedStatuses {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString("?")
			args = append(args, s)
		}
		sb.WriteString(`) AND len(e.embedding) = ?`)
	} else {
		sb.WriteString(` WHERE len(e.embedding) = ?`)
	}
	args = append(args, len(query))
	sb.WriteString(` ORDER BY sim DESC NULLS LAST LIMIT ?`)
	args = append(args, topK)

	rows, err := idx.db.Query(sb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("duckdb: search: %w", err)
	}
	defer rows.Close()

	var out []SimilarTask
	for rows.Next() {
		var (
			id  string
			sim sql.NullFloat64
		)
		if err := rows.Scan(&id, &sim); err != nil {
			return nil, fmt.Errorf("duckdb: scan: %w", err)
		}
		if !sim.Valid {
			continue
		}
		out = append(out, SimilarTask{TaskID: id, Similarity: sim.Float64})
	}
	return out, rows.Err()
}

// Rebuild wipes user data and re-populates from the provided slices.
// Schema is preserved.
func (idx *duckdbIndex) Rebuild(tasks []model.Task, phases []model.Phase, embs []EmbeddingRecord) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	tx, err := idx.db.Begin()
	if err != nil {
		return fmt.Errorf("duckdb: begin: %w", err)
	}
	rollback := func() {
		_ = tx.Rollback()
	}
	for _, q := range []string{
		`DELETE FROM trajectory`,
		`DELETE FROM embeddings`,
		`DELETE FROM tasks`,
		`DELETE FROM phases`,
	} {
		if _, err := tx.Exec(q); err != nil {
			rollback()
			return fmt.Errorf("duckdb: %s: %w", q, err)
		}
	}
	for _, p := range phases {
		if _, err := tx.Exec(
			`INSERT INTO phases (id, title, created_at) VALUES (?, ?, ?)`,
			p.ID, p.Title, p.CreatedAt,
		); err != nil {
			rollback()
			return fmt.Errorf("duckdb: rebuild phase %s: %w", p.ID, err)
		}
	}
	for _, t := range tasks {
		if _, err := tx.Exec(
			`INSERT INTO tasks (
				id, phase_id, title, user_intent, summary, status, session_id,
				tags, depends_on, files_touched, key_decisions, todos_left,
				blockers_resolved, interfaces_exposed, started_at, ended_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			t.ID,
			nilIfEmpty(t.PhaseID),
			t.Title,
			nilIfEmpty(t.UserIntent),
			nilIfEmpty(t.Summary),
			t.Status,
			nilIfEmpty(t.SessionID),
			stringList(t.Tags),
			stringList(t.DependsOn),
			stringList(t.FilesTouched),
			stringList(t.KeyDecisions),
			stringList(t.TodosLeft),
			stringList(t.BlockersResolved),
			stringList(t.InterfacesExposed),
			nilIfNilTime(t.StartedAt),
			nilIfNilTime(t.EndedAt),
		); err != nil {
			rollback()
			return fmt.Errorf("duckdb: rebuild task %s: %w", t.ID, err)
		}
	}
	for _, e := range embs {
		if _, err := tx.Exec(
			`INSERT INTO embeddings (task_id, text, embedding, updated_at) VALUES (?, ?, ?, ?)`,
			e.TaskID, e.Text, float32List(e.Embedding), e.UpdatedAt,
		); err != nil {
			rollback()
			return fmt.Errorf("duckdb: rebuild embedding %s: %w", e.TaskID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("duckdb: commit: %w", err)
	}
	return nil
}

// Close closes the underlying DB handle.
func (idx *duckdbIndex) Close() error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if idx.db == nil {
		return nil
	}
	err := idx.db.Close()
	idx.db = nil
	return err
}

// nilIfEmpty returns nil for empty strings so they are stored as SQL NULL
// rather than empty-string. Keeps the index honest about absence.
func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// nilIfNilTime mirrors nilIfEmpty for *time.Time. Returns SQL NULL for nil
// or zero-valued timestamps; otherwise returns the dereferenced time.Time.
func nilIfNilTime(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return *t
}

// stringList converts a Go []string to a DuckDB list literal value. The
// driver accepts []string directly via the duckdb.Composite mapping; we
// pass nil for empty slices to store SQL NULL.
func stringList(s []string) any {
	if len(s) == 0 {
		return nil
	}
	return s
}

// float32List converts a Go []float32 to whatever the driver expects for
// FLOAT[]. v2 of the driver accepts []float32 directly.
func float32List(v []float32) any {
	if len(v) == 0 {
		return nil
	}
	return v
}
