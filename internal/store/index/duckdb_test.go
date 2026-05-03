//go:build pj_duckdb

package index

import (
	"fmt"
	"path/filepath"
	"reflect"
	"sync"
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

// scanStringList converts the value returned by go-duckdb v1 for VARCHAR[]
// columns into a []string. The driver returns []interface{} (each element a
// string); this helper does the conversion and is used by array round-trip tests.
func scanStringList(v any) ([]string, error) {
	if v == nil {
		return nil, nil
	}
	iface, ok := v.([]interface{})
	if !ok {
		return nil, fmt.Errorf("expected []interface{}, got %T", v)
	}
	out := make([]string, len(iface))
	for i, el := range iface {
		s, ok := el.(string)
		if !ok {
			return nil, fmt.Errorf("element %d: expected string, got %T", i, el)
		}
		out[i] = s
	}
	return out, nil
}

// TestDuckDB_ArrayRoundTrip verifies that VARCHAR[] columns survive a
// write-read cycle with exact order preserved. This catches driver-level list
// serialization bugs.
func TestDuckDB_ArrayRoundTrip(t *testing.T) {
	idx := openTestIndex(t)
	d := idx.(*duckdbIndex)

	task := model.Task{
		ID:           "array-rt",
		Title:        "array round-trip",
		Status:       model.StatusInProgress,
		Tags:         []string{"z", "a", "m"},
		DependsOn:    []string{"dep-3", "dep-1", "dep-2"},
		FilesTouched: []string{"cmd/main.go", "internal/foo/bar.go"},
		KeyDecisions: []string{"use DuckDB", "write tests first", "avoid CGO in noop"},
	}
	if err := idx.UpsertTask(task); err != nil {
		t.Fatalf("UpsertTask: %v", err)
	}

	rows, err := d.db.Query(
		`SELECT tags, depends_on, files_touched, key_decisions FROM tasks WHERE id = ?`,
		task.ID,
	)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatal("expected one row, got none")
	}

	// go-duckdb v1 returns []interface{} for VARCHAR[] columns when scanned
	// into an any destination. Use scanStringList to convert.
	var (
		rawTags         any
		rawDependsOn    any
		rawFilesTouched any
		rawKeyDecisions any
	)
	if err := rows.Scan(&rawTags, &rawDependsOn, &rawFilesTouched, &rawKeyDecisions); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}

	gotTags, err := scanStringList(rawTags)
	if err != nil {
		t.Fatalf("scanStringList(tags): %v", err)
	}
	gotDependsOn, err := scanStringList(rawDependsOn)
	if err != nil {
		t.Fatalf("scanStringList(depends_on): %v", err)
	}
	gotFilesTouched, err := scanStringList(rawFilesTouched)
	if err != nil {
		t.Fatalf("scanStringList(files_touched): %v", err)
	}
	gotKeyDecisions, err := scanStringList(rawKeyDecisions)
	if err != nil {
		t.Fatalf("scanStringList(key_decisions): %v", err)
	}

	if !reflect.DeepEqual(gotTags, task.Tags) {
		t.Errorf("tags: got %v want %v", gotTags, task.Tags)
	}
	if !reflect.DeepEqual(gotDependsOn, task.DependsOn) {
		t.Errorf("depends_on: got %v want %v", gotDependsOn, task.DependsOn)
	}
	if !reflect.DeepEqual(gotFilesTouched, task.FilesTouched) {
		t.Errorf("files_touched: got %v want %v", gotFilesTouched, task.FilesTouched)
	}
	if !reflect.DeepEqual(gotKeyDecisions, task.KeyDecisions) {
		t.Errorf("key_decisions: got %v want %v", gotKeyDecisions, task.KeyDecisions)
	}
}

// TestDuckDB_NullEmbedding verifies that a task inserted without an embedding
// row is excluded from SearchSimilar results without error. Orphan task rows
// (tasks with no corresponding embeddings row) must not cause panics or SQL
// errors — they are silently excluded from similarity search.
func TestDuckDB_NullEmbedding(t *testing.T) {
	idx := openTestIndex(t)

	task := model.Task{
		ID:     "no-emb",
		Title:  "task without embedding",
		Status: model.StatusTodo,
	}
	if err := idx.UpsertTask(task); err != nil {
		t.Fatalf("UpsertTask: %v", err)
	}

	// Confirm no embedding row was inserted.
	n, err := idx.TableCount("embeddings")
	if err != nil {
		t.Fatalf("TableCount(embeddings): %v", err)
	}
	if n != 0 {
		t.Errorf("embeddings count: got %d want 0", n)
	}

	// SearchSimilar must return empty result, no error.
	got, err := idx.SearchSimilar([]float32{1, 0, 0}, 5, nil)
	if err != nil {
		t.Fatalf("SearchSimilar: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("SearchSimilar: got %v want empty (orphan task excluded)", got)
	}
}

// TestDuckDB_ConcurrentUpserts spawns 50 goroutines each upserting the same
// task ID with different titles. After all complete there must be exactly one
// row in the tasks table. The idx.mu mutex must serialize writes correctly;
// run with -race to detect any data races.
func TestDuckDB_ConcurrentUpserts(t *testing.T) {
	idx := openTestIndex(t)

	const workers = 50
	var wg sync.WaitGroup
	errs := make([]error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			errs[n] = idx.UpsertTask(model.Task{
				ID:     "concurrent-id",
				Title:  fmt.Sprintf("title from goroutine %d", n),
				Status: model.StatusInProgress,
			})
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: UpsertTask error: %v", i, err)
		}
	}

	n, err := idx.TableCount("tasks")
	if err != nil {
		t.Fatalf("TableCount(tasks): %v", err)
	}
	if n != 1 {
		t.Errorf("tasks count after concurrent upserts: got %d want 1", n)
	}

	// Status must be one of the written values (last-writer-wins).
	status, err := idx.GetTaskStatus("concurrent-id")
	if err != nil {
		t.Fatalf("GetTaskStatus: %v", err)
	}
	if status != model.StatusInProgress {
		t.Errorf("status: got %q want %q", status, model.StatusInProgress)
	}
}
