package store_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nhdms/project-journal/internal/model"
	"github.com/nhdms/project-journal/internal/store"
	"github.com/nhdms/project-journal/internal/store/index"
)

// newTestLayout builds a self-contained Layout under t.TempDir() and
// registers a cleanup that closes all cached indexes.
func newTestLayout(t *testing.T) store.Layout {
	t.Helper()
	dir := t.TempDir()
	l := store.Layout{
		Root:        dir,
		Dir:         dir,
		Config:      filepath.Join(dir, "config.json"),
		Current:     filepath.Join(dir, "current"),
		PhasesJSONL: filepath.Join(dir, "phases.jsonl"),
		TasksJSONL:  filepath.Join(dir, "tasks.jsonl"),
		SessionsDir: filepath.Join(dir, "sessions"),
	}
	if err := os.MkdirAll(l.SessionsDir, 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}
	t.Cleanup(func() { _ = index.CloseAll() })
	return l
}

// idxFor returns the index for l, failing the test if it cannot be opened.
func idxFor(t *testing.T, l store.Layout) index.Index {
	t.Helper()
	idx, err := index.For(l.Dir)
	if err != nil {
		t.Fatalf("index.For: %v", err)
	}
	return idx
}

// TestStoreAppendTaskMirrors verifies that AppendTask writes are reflected in
// the DuckDB tasks table immediately after the call returns.
func TestStoreAppendTaskMirrors(t *testing.T) {
	l := newTestLayout(t)

	t1 := model.Task{ID: "T1", Title: "first task", Status: model.StatusTodo}
	if err := store.AppendTask(l, t1); err != nil {
		t.Fatalf("AppendTask T1: %v", err)
	}

	idx := idxFor(t, l)
	n, err := idx.TableCount("tasks")
	if err != nil {
		t.Fatalf("TableCount: %v", err)
	}
	if n != 1 {
		t.Errorf("after first append: tasks count = %d, want 1", n)
	}

	t2 := model.Task{ID: "T2", Title: "second task", Status: model.StatusInProgress}
	if err := store.AppendTask(l, t2); err != nil {
		t.Fatalf("AppendTask T2: %v", err)
	}

	n, err = idx.TableCount("tasks")
	if err != nil {
		t.Fatalf("TableCount: %v", err)
	}
	if n != 2 {
		t.Errorf("after second append: tasks count = %d, want 2", n)
	}
}

// TestStoreReplaceTaskMirrors verifies that ReplaceTask updates the status
// column in the DuckDB index.
func TestStoreReplaceTaskMirrors(t *testing.T) {
	l := newTestLayout(t)

	task := model.Task{ID: "RT1", Title: "replaceable", Status: model.StatusTodo}
	if err := store.AppendTask(l, task); err != nil {
		t.Fatalf("AppendTask: %v", err)
	}

	idx := idxFor(t, l)
	status, err := idx.GetTaskStatus("RT1")
	if err != nil {
		t.Fatalf("GetTaskStatus before replace: %v", err)
	}
	if status != model.StatusTodo {
		t.Errorf("status before replace: got %q want %q", status, model.StatusTodo)
	}

	now := time.Now().UTC().Truncate(time.Second)
	task.Status = model.StatusCompleted
	task.EndedAt = &now
	if err := store.ReplaceTask(l, task); err != nil {
		t.Fatalf("ReplaceTask: %v", err)
	}

	status, err = idx.GetTaskStatus("RT1")
	if err != nil {
		t.Fatalf("GetTaskStatus after replace: %v", err)
	}
	if status != model.StatusCompleted {
		t.Errorf("status after replace: got %q want %q", status, model.StatusCompleted)
	}
}

// TestStoreUpsertEmbeddingMirrors verifies that UpsertEmbedding writes are
// reflected in the DuckDB embeddings table and visible via SearchSimilar.
func TestStoreUpsertEmbeddingMirrors(t *testing.T) {
	l := newTestLayout(t)

	task := model.Task{ID: "EMB1", Title: "embedding task", Status: model.StatusCompleted}
	if err := store.AppendTask(l, task); err != nil {
		t.Fatalf("AppendTask: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	rec := store.EmbeddingRecord{
		TaskID:    "EMB1",
		Text:      "embedding test text",
		Embedding: []float32{0.6, 0.8, 0.0}, // unit vector
		UpdatedAt: now,
	}
	if err := store.UpsertEmbedding(l, rec); err != nil {
		t.Fatalf("UpsertEmbedding: %v", err)
	}

	idx := idxFor(t, l)
	n, err := idx.TableCount("embeddings")
	if err != nil {
		t.Fatalf("TableCount(embeddings): %v", err)
	}
	if n != 1 {
		t.Errorf("embeddings count = %d, want 1", n)
	}

	// Query with a near-identical vector; similarity must be >= 0.99.
	query := []float32{0.6, 0.8, 0.0}
	results, err := idx.SearchSimilar(query, 5, nil)
	if err != nil {
		t.Fatalf("SearchSimilar: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("SearchSimilar returned empty; expected EMB1")
	}
	if results[0].TaskID != "EMB1" {
		t.Errorf("top result: got %q want EMB1", results[0].TaskID)
	}
	if results[0].Similarity < 0.99 {
		t.Errorf("similarity = %.4f, want >= 0.99", results[0].Similarity)
	}
}

// TestStoreAppendTrajectoryMirrors verifies that AppendTrajectory events are
// reflected in the DuckDB trajectory table.
func TestStoreAppendTrajectoryMirrors(t *testing.T) {
	l := newTestLayout(t)

	task := model.Task{ID: "TRJ1", Title: "trajectory task", Status: model.StatusInProgress}
	if err := store.AppendTask(l, task); err != nil {
		t.Fatalf("AppendTask: %v", err)
	}

	base := time.Now().UTC().Truncate(time.Second)
	for i := 0; i < 3; i++ {
		ev := model.TrajectoryEvent{
			Timestamp: base.Add(time.Duration(i) * time.Second),
			Type:      model.EventToolUse,
			Tool:      "Bash",
			Content:   "echo hello",
		}
		if err := store.AppendTrajectory(l, "TRJ1", ev); err != nil {
			t.Fatalf("AppendTrajectory[%d]: %v", i, err)
		}
	}

	idx := idxFor(t, l)
	n, err := idx.TableCount("trajectory")
	if err != nil {
		t.Fatalf("TableCount(trajectory): %v", err)
	}
	if n != 3 {
		t.Errorf("trajectory count = %d, want 3", n)
	}
}

// TestStoreRebuildIndexFromJSONL verifies RebuildIndex by seeding JSONL files
// directly (bypassing the mirror path) then calling RebuildIndex and asserting
// the counts. AppendJSONL is a pure file write — it does not call any mirror
// function — so writing JSONL before opening the index means the index starts
// empty; RebuildIndex must populate it from the files.
func TestStoreRebuildIndexFromJSONL(t *testing.T) {
	l := newTestLayout(t)
	now := time.Now().UTC().Truncate(time.Second)

	// Write 3 tasks via AppendJSONL (no mirror fires).
	tasks := []model.Task{
		{ID: "RB1", Title: "rebuild 1", Status: model.StatusCompleted},
		{ID: "RB2", Title: "rebuild 2", Status: model.StatusCompleted},
		{ID: "RB3", Title: "rebuild 3", Status: model.StatusInProgress},
	}
	for _, tk := range tasks {
		if err := store.AppendJSONL(l.TasksJSONL, tk); err != nil {
			t.Fatalf("AppendJSONL task %s: %v", tk.ID, err)
		}
	}

	// Write 1 phase.
	phase := model.Phase{ID: "P1", Title: "Rebuild phase", CreatedAt: now}
	if err := store.AppendJSONL(l.PhasesJSONL, phase); err != nil {
		t.Fatalf("AppendJSONL phase: %v", err)
	}

	// Write 2 embeddings.
	embPath := filepath.Join(l.Dir, store.EmbeddingsFile)
	emb1 := store.EmbeddingRecord{TaskID: "RB1", Text: "t1", Embedding: []float32{1, 0}, UpdatedAt: now}
	emb2 := store.EmbeddingRecord{TaskID: "RB2", Text: "t2", Embedding: []float32{0, 1}, UpdatedAt: now}
	for _, e := range []store.EmbeddingRecord{emb1, emb2} {
		if err := store.AppendJSONL(embPath, e); err != nil {
			t.Fatalf("AppendJSONL embedding %s: %v", e.TaskID, err)
		}
	}

	// RebuildIndex opens (or reuses) the index and populates it from JSONL.
	if err := store.RebuildIndex(l); err != nil {
		t.Fatalf("RebuildIndex: %v", err)
	}

	idx := idxFor(t, l)

	if n, err := idx.TableCount("tasks"); err != nil {
		t.Fatalf("TableCount(tasks): %v", err)
	} else if n != 3 {
		t.Errorf("tasks = %d, want 3", n)
	}

	if n, err := idx.TableCount("phases"); err != nil {
		t.Fatalf("TableCount(phases): %v", err)
	} else if n != 1 {
		t.Errorf("phases = %d, want 1", n)
	}

	if n, err := idx.TableCount("embeddings"); err != nil {
		t.Fatalf("TableCount(embeddings): %v", err)
	} else if n != 2 {
		t.Errorf("embeddings = %d, want 2", n)
	}
}

// TestIndexDrift_Clean: synchronous AppendTask path keeps JSONL and index
// in lockstep, so drift should be false immediately after the call.
func TestIndexDrift_Clean(t *testing.T) {
	l := newTestLayout(t)
	now := time.Now().UTC().Truncate(time.Second)
	if err := store.AppendTask(l, model.Task{
		ID: "P1.T1", Title: "t", Status: model.StatusInProgress, StartedAt: &now,
	}); err != nil {
		t.Fatalf("append: %v", err)
	}
	r, err := store.IndexDrift(l)
	if err != nil {
		t.Fatalf("drift: %v", err)
	}
	if r.Drift {
		t.Errorf("expected no drift, got %+v", r)
	}
	if r.TasksJSONL != 1 || r.TasksIndex != 1 {
		t.Errorf("counts: %+v", r)
	}
}

// TestIndexDrift_DetectsJSONLAhead: simulate the contention scenario by
// writing a task line directly to JSONL (bypassing AppendTask's mirror).
// Drift should be detected.
func TestIndexDrift_DetectsJSONLAhead(t *testing.T) {
	l := newTestLayout(t)
	now := time.Now().UTC().Truncate(time.Second)

	// Normal append: both layers see it.
	if err := store.AppendTask(l, model.Task{
		ID: "P1.T1", Title: "first", Status: model.StatusInProgress, StartedAt: &now,
	}); err != nil {
		t.Fatalf("append1: %v", err)
	}

	// Simulate a mirror that didn't fire: append directly to JSONL.
	if err := store.AppendJSONL(l.TasksJSONL, model.Task{
		ID: "P1.T2", Title: "second", Status: model.StatusInProgress, StartedAt: &now,
	}); err != nil {
		t.Fatalf("appendjsonl: %v", err)
	}

	r, err := store.IndexDrift(l)
	if err != nil {
		t.Fatalf("drift: %v", err)
	}
	if !r.Drift {
		t.Errorf("expected drift, got %+v", r)
	}
	if r.TasksJSONL != 2 || r.TasksIndex != 1 {
		t.Errorf("counts: got tasks JSONL=%d Index=%d, want 2/1", r.TasksJSONL, r.TasksIndex)
	}
}

// TestEnsureIndexFresh_RebuildsOnDrift: when JSONL is ahead of the index
// (mirror skipped due to lock contention, fresh install on a v0.4.x
// journal), EnsureIndexFresh detects the drift on first SearchSimilar
// call and rebuilds.
func TestEnsureIndexFresh_RebuildsOnDrift(t *testing.T) {
	l := newTestLayout(t)
	now := time.Now().UTC().Truncate(time.Second)

	// Seed JSONL directly: AppendJSONL bypasses the mirror, so the index
	// stays empty while JSONL has the row.
	if err := store.AppendJSONL(l.TasksJSONL, model.Task{
		ID: "T1", Title: "drift", Status: model.StatusCompleted, EndedAt: &now,
	}); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	embPath := filepath.Join(l.Dir, store.EmbeddingsFile)
	if err := store.AppendJSONL(embPath, store.EmbeddingRecord{
		TaskID: "T1", Text: "drift", Embedding: []float32{1, 0}, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed embedding: %v", err)
	}

	// Confirm precondition: index is empty.
	idx := idxFor(t, l)
	if n, _ := idx.TableCount("tasks"); n != 0 {
		t.Fatalf("precondition: tasks=%d, want 0", n)
	}

	// Trigger via SearchSimilar — first call per process should detect drift,
	// rebuild, then return the rebuilt result.
	res, err := store.SearchSimilar(l, []float32{1, 0}, 5, nil)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(res) != 1 || res[0].TaskID != "T1" {
		t.Errorf("expected [T1] after auto-rebuild, got %+v", res)
	}
	// Index should now match JSONL.
	if n, _ := idx.TableCount("tasks"); n != 1 {
		t.Errorf("post-rebuild tasks=%d, want 1", n)
	}
	if n, _ := idx.TableCount("embeddings"); n != 1 {
		t.Errorf("post-rebuild embeddings=%d, want 1", n)
	}
}

// TestIndexDrift_RebuildHeals: after RebuildIndex, drift returns to false.
func TestIndexDrift_RebuildHeals(t *testing.T) {
	l := newTestLayout(t)
	now := time.Now().UTC().Truncate(time.Second)

	if err := store.AppendJSONL(l.TasksJSONL, model.Task{
		ID: "P1.T1", Title: "raw", Status: model.StatusInProgress, StartedAt: &now,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	r, err := store.IndexDrift(l)
	if err != nil {
		t.Fatalf("drift1: %v", err)
	}
	if !r.Drift {
		t.Fatalf("precondition: expected drift, got %+v", r)
	}

	if err := store.RebuildIndex(l); err != nil {
		t.Fatalf("rebuild: %v", err)
	}

	r, err = store.IndexDrift(l)
	if err != nil {
		t.Fatalf("drift2: %v", err)
	}
	if r.Drift {
		t.Errorf("expected no drift after rebuild, got %+v", r)
	}
}
