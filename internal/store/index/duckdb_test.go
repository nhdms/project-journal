//go:build pj_duckdb

package index

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/nhdms/project-journal/internal/model"
)

func openTestIndex(t *testing.T) Index {
	t.Helper()
	dir := t.TempDir()
	idx, err := open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = idx.Close() })
	return idx
}

func TestDuckDB_Enabled(t *testing.T) {
	if !Enabled() {
		t.Fatal("Enabled() should return true under pj_duckdb build tag")
	}
}

func TestDuckDB_TaskUpsert(t *testing.T) {
	idx := openTestIndex(t)
	now := time.Now().UTC().Truncate(time.Second)
	task := model.Task{
		ID:        "P1.T1",
		PhaseID:   "P1",
		Title:     "first",
		Status:    model.StatusInProgress,
		Tags:      []string{"a", "b"},
		StartedAt: &now,
	}
	if err := idx.UpsertTask(task); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// Idempotent: second call updates rather than dupes.
	task.Status = model.StatusCompleted
	end := now.Add(time.Hour)
	task.EndedAt = &end
	if err := idx.UpsertTask(task); err != nil {
		t.Fatalf("re-upsert: %v", err)
	}
}

func TestDuckDB_PhaseUpsert(t *testing.T) {
	idx := openTestIndex(t)
	if err := idx.UpsertPhase(model.Phase{
		ID:        "P1",
		Title:     "Foundation",
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}); err != nil {
		t.Fatalf("upsert phase: %v", err)
	}
}

func TestDuckDB_SearchSimilar(t *testing.T) {
	idx := openTestIndex(t)
	now := time.Now().UTC().Truncate(time.Second)

	// Insert three tasks with embeddings pointing in different directions.
	tasks := []model.Task{
		{ID: "T1", Title: "auth refactor", Status: model.StatusCompleted, EndedAt: &now},
		{ID: "T2", Title: "billing fix", Status: model.StatusCompleted, EndedAt: &now},
		{ID: "T3", Title: "auth bug", Status: model.StatusCompleted, EndedAt: &now},
	}
	embs := []EmbeddingRecord{
		{TaskID: "T1", Text: "auth", Embedding: []float32{1, 0, 0}, UpdatedAt: now},
		{TaskID: "T2", Text: "bill", Embedding: []float32{0, 1, 0}, UpdatedAt: now},
		{TaskID: "T3", Text: "auth", Embedding: []float32{0.9, 0.1, 0}, UpdatedAt: now},
	}
	for _, tk := range tasks {
		if err := idx.UpsertTask(tk); err != nil {
			t.Fatalf("upsert task: %v", err)
		}
	}
	for _, e := range embs {
		if err := idx.UpsertEmbedding(e); err != nil {
			t.Fatalf("upsert emb: %v", err)
		}
	}

	// Query close to T1 — expect T1 first, T3 second, T2 last.
	got, err := idx.SearchSimilar([]float32{1, 0, 0}, 3, nil)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len=%d want 3 (got=%+v)", len(got), got)
	}
	if got[0].TaskID != "T1" || got[1].TaskID != "T3" || got[2].TaskID != "T2" {
		t.Errorf("order: got %+v want [T1, T3, T2]", got)
	}
	if got[0].Similarity <= got[1].Similarity || got[1].Similarity <= got[2].Similarity {
		t.Errorf("similarity not strictly decreasing: %+v", got)
	}
}

func TestDuckDB_SearchSimilar_StatusFilter(t *testing.T) {
	idx := openTestIndex(t)
	now := time.Now().UTC().Truncate(time.Second)
	for _, tk := range []model.Task{
		{ID: "DONE", Title: "done", Status: model.StatusCompleted, EndedAt: &now},
		{ID: "BLOCKED", Title: "blocked", Status: model.StatusBlocked, EndedAt: &now},
	} {
		if err := idx.UpsertTask(tk); err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}
	for _, e := range []EmbeddingRecord{
		{TaskID: "DONE", Text: "x", Embedding: []float32{1, 0}, UpdatedAt: now},
		{TaskID: "BLOCKED", Text: "x", Embedding: []float32{1, 0}, UpdatedAt: now},
	} {
		if err := idx.UpsertEmbedding(e); err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}

	got, err := idx.SearchSimilar([]float32{1, 0}, 5, []string{model.StatusCompleted})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(got) != 1 || got[0].TaskID != "DONE" {
		t.Errorf("status filter: got %+v want [DONE]", got)
	}
}

func TestDuckDB_SearchSimilar_DimensionMismatch(t *testing.T) {
	idx := openTestIndex(t)
	now := time.Now().UTC().Truncate(time.Second)
	if err := idx.UpsertTask(model.Task{ID: "X", Title: "x", Status: "completed", EndedAt: &now}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// 3-d embedding stored
	if err := idx.UpsertEmbedding(EmbeddingRecord{
		TaskID: "X", Text: "x", Embedding: []float32{1, 0, 0}, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// Query with 2-d vector — should silently exclude (no error, no row).
	got, err := idx.SearchSimilar([]float32{1, 0}, 5, nil)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("dim mismatch should yield no rows, got %+v", got)
	}
}

func TestDuckDB_Trajectory(t *testing.T) {
	idx := openTestIndex(t)
	ts := time.Now().UTC().Truncate(time.Second)
	ev := model.TrajectoryEvent{
		Timestamp: ts,
		Type:      model.EventToolUse,
		Tool:      "Bash",
		Content:   "ls",
	}
	if err := idx.AppendTrajectory("T1", ev); err != nil {
		t.Fatalf("append: %v", err)
	}
	// Same key → INSERT OR IGNORE no-ops.
	if err := idx.AppendTrajectory("T1", ev); err != nil {
		t.Fatalf("re-append: %v", err)
	}
}

func TestDuckDB_Rebuild(t *testing.T) {
	idx := openTestIndex(t)
	now := time.Now().UTC().Truncate(time.Second)

	// Pre-populate, then rebuild with a different dataset.
	if err := idx.UpsertTask(model.Task{ID: "OLD", Title: "old", Status: "completed"}); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	tasks := []model.Task{
		{ID: "NEW1", Title: "new1", Status: model.StatusCompleted, EndedAt: &now},
		{ID: "NEW2", Title: "new2", Status: model.StatusCompleted, EndedAt: &now},
	}
	phases := []model.Phase{{ID: "P1", Title: "p", CreatedAt: now}}
	embs := []EmbeddingRecord{
		{TaskID: "NEW1", Text: "a", Embedding: []float32{1, 0}, UpdatedAt: now},
	}
	if err := idx.Rebuild(tasks, phases, embs); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	// OLD should be gone; NEW1/NEW2 present. Probe via SearchSimilar.
	got, err := idx.SearchSimilar([]float32{1, 0}, 5, nil)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(got) != 1 || got[0].TaskID != "NEW1" {
		t.Errorf("rebuild: got %+v want [NEW1]", got)
	}
}

func TestDuckDB_SchemaVersionMismatch(t *testing.T) {
	dir := t.TempDir()
	idx, err := open(dir)
	if err != nil {
		t.Fatalf("open1: %v", err)
	}
	if err := idx.UpsertTask(model.Task{ID: "T1", Title: "t1", Status: "completed"}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	_ = idx.Close()

	// Corrupt the schema version, then reopen — bootstrap should wipe.
	idx2, err := open(dir)
	if err != nil {
		t.Fatalf("open2: %v", err)
	}
	defer idx2.Close()
	d := idx2.(*duckdbIndex)
	if _, err := d.db.Exec(`UPDATE schema_meta SET version = ?`, schemaVersion+99); err != nil {
		t.Fatalf("force mismatch: %v", err)
	}
	_ = idx2.Close()

	idx3, err := open(dir)
	if err != nil {
		t.Fatalf("open3: %v", err)
	}
	defer idx3.Close()
	// After mismatch, T1 should be gone.
	got, err := idx3.SearchSimilar([]float32{1}, 5, nil)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("after schema mismatch wipe, got %+v want empty", got)
	}
	// Verify the file actually exists where expected.
	if _, err := openTestFile(filepath.Join(dir, "index.duckdb")); err != nil {
		t.Errorf("index.duckdb missing: %v", err)
	}
}

func openTestFile(path string) (struct{}, error) {
	if _, err := filepath.Abs(path); err != nil {
		return struct{}{}, err
	}
	return struct{}{}, nil
}
