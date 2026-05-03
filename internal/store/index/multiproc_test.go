//go:build pj_duckdb

package index

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nhdms/project-journal/internal/model"
)

// TestHelperProcess is the subprocess entry point used by the multi-process
// concurrency tests below. It runs only when PJ_TEST_HELPER=1; otherwise it
// is a no-op and is skipped by `go test`.
//
// The helper opens the index at the directory passed as the last argument
// and performs the action specified by PJ_TEST_HELPER_ACTION:
//
//   - "open_hold" : open the index, hold for 2s, exit 0. Tests whether a
//     concurrent RW open from a sibling process fails or queues.
//   - "open_quick": open, write one task, close, exit 0. Used to confirm
//     serial opens succeed once the prior process releases.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("PJ_TEST_HELPER") != "1" {
		return
	}
	defer os.Exit(0)

	args := os.Args
	dir := args[len(args)-1]
	action := os.Getenv("PJ_TEST_HELPER_ACTION")

	idx, err := open(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "HELPER_OPEN_FAIL: %v\n", err)
		os.Exit(2)
	}
	defer idx.Close()

	switch action {
	case "open_hold":
		fmt.Fprintln(os.Stderr, "HELPER_HOLDING")
		time.Sleep(2 * time.Second)
	case "open_quick":
		now := time.Now().UTC().Truncate(time.Second)
		err := idx.UpsertTask(model.Task{
			ID: "FROM_HELPER", Title: "from helper",
			Status: model.StatusCompleted, EndedAt: &now,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "HELPER_WRITE_FAIL: %v\n", err)
			os.Exit(3)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown action: %q\n", action)
		os.Exit(4)
	}
	fmt.Fprintln(os.Stderr, "HELPER_DONE")
}

// runHelper invokes the same test binary with TestHelperProcess as the only
// run target, passing dir as the final positional argument. action selects
// the helper's behavior. The returned cmd is started but not waited on so
// the caller can race the helper against in-process operations.
func runHelper(t *testing.T, action, dir string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0],
		"-test.run=TestHelperProcess",
		"-test.timeout=30s",
		"--", dir,
	)
	cmd.Env = append(os.Environ(),
		"PJ_TEST_HELPER=1",
		"PJ_TEST_HELPER_ACTION="+action,
	)
	cmd.Stderr = &captureWriter{name: "helper-stderr", t: t}
	cmd.Stdout = &captureWriter{name: "helper-stdout", t: t}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	return cmd
}

// captureWriter logs subprocess output via t.Logf so test failures surface
// the helper's stderr/stdout. The mutex is required because exec.Cmd
// spawns its own goroutine to copy from the subprocess pipe into Write,
// which races with the test goroutine calling String() to poll for
// progress markers.
type captureWriter struct {
	name string
	t    *testing.T
	mu   sync.Mutex
	buf  strings.Builder
}

func (w *captureWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf.Write(p)
	return len(p), nil
}

func (w *captureWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// TestMultiProc_TwoHandlesSameProcess opens two duckdbIndex instances against
// the same file from within ONE process. DuckDB's lock is process-level, so
// two handles in the same process should not block each other — but they
// share the underlying file lock. This characterizes the baseline.
func TestMultiProc_TwoHandlesSameProcess(t *testing.T) {
	dir := t.TempDir()

	idx1, err := open(dir)
	if err != nil {
		t.Fatalf("open1: %v", err)
	}
	defer idx1.Close()

	idx2, err := open(dir)
	if err != nil {
		// If this fails, it means the same-process case also conflicts —
		// surprising but not necessarily a bug. Document the failure.
		t.Logf("second handle in same process failed: %v (this is OK if process-level lock is enforced even within one process)", err)
		return
	}
	defer idx2.Close()

	// Both opened. Try writing through both.
	now := time.Now().UTC().Truncate(time.Second)
	if err := idx1.UpsertTask(model.Task{ID: "A", Title: "a", Status: "completed", EndedAt: &now}); err != nil {
		t.Errorf("write via idx1: %v", err)
	}
	if err := idx2.UpsertTask(model.Task{ID: "B", Title: "b", Status: "completed", EndedAt: &now}); err != nil {
		t.Errorf("write via idx2: %v", err)
	}
	// Cross-handle visibility: idx1 should see B (DuckDB commits to disk).
	n1, _ := idx1.TableCount("tasks")
	n2, _ := idx2.TableCount("tasks")
	t.Logf("same-process cross-handle counts: idx1=%d idx2=%d (each process sees its own + on-commit visible writes)", n1, n2)
}

// TestMultiProc_SecondProcessBlocked verifies the multi-process contention
// behavior: while one process holds the index open RW, can a second process
// also open it? Per DuckDB docs, only one process may hold a database in
// read-write mode; this test characterizes whether the second process fails
// fast (expected) or hangs.
func TestMultiProc_SecondProcessBlocked(t *testing.T) {
	dir := t.TempDir()

	// Spawn a helper that opens the DB and holds it for 2 seconds.
	helper := runHelper(t, "open_hold", dir)
	t.Cleanup(func() {
		_ = helper.Process.Kill()
		_, _ = helper.Process.Wait()
	})

	// Wait for helper to actually have the lock.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if w, ok := helper.Stderr.(*captureWriter); ok && strings.Contains(w.String(), "HELPER_HOLDING") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Try to open from this process while helper holds the lock.
	start := time.Now()
	idx, err := open(dir)
	elapsed := time.Since(start)

	if err != nil {
		// Expected on most DuckDB versions: clean failure.
		t.Logf("PASS: second process blocked with error after %v: %v", elapsed, err)
		return
	}
	defer idx.Close()
	// Surprising but possible: DuckDB allowed two RW opens. Document and
	// continue — the index has its own mutex so writes don't corrupt.
	t.Logf("UNEXPECTED: second process succeeded in %v (DuckDB allowed concurrent RW)", elapsed)
}

// TestMultiProc_ForRecoversAfterContention verifies that index.For does not
// cache the noop fallback on a contention error: once the contending
// process exits, the next For() call must succeed and return a real index.
// This guards against the regression where a single open failure would
// permanently disable mirroring for the rest of the process's lifetime.
func TestMultiProc_ForRecoversAfterContention(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(resetForTest)

	helper := runHelper(t, "open_hold", dir)
	t.Cleanup(func() {
		_ = helper.Process.Kill()
		_, _ = helper.Process.Wait()
	})

	// Wait for helper to acquire the lock.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if w, ok := helper.Stderr.(*captureWriter); ok && strings.Contains(w.String(), "HELPER_HOLDING") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// First For() while contended: should fail (noop returned, error set).
	idx1, err := For(dir)
	if err == nil {
		t.Fatalf("For during contention should return error, got nil")
	}
	if _, ok := idx1.(noopFallback); !ok {
		t.Errorf("For during contention should return noop fallback, got %T", idx1)
	}

	// Wait for helper to release the lock.
	if err := helper.Wait(); err != nil {
		t.Fatalf("helper wait: %v", err)
	}

	// Second For() after release: should succeed with a real index.
	idx2, err := For(dir)
	if err != nil {
		t.Fatalf("For after contention released: %v (regression: noop got cached?)", err)
	}
	if _, ok := idx2.(*duckdbIndex); !ok {
		t.Errorf("For after contention should return real index, got %T", idx2)
	}
	// And it should be writable.
	now := time.Now().UTC().Truncate(time.Second)
	if err := idx2.UpsertTask(model.Task{
		ID: "RECOVERED", Title: "ok", Status: model.StatusCompleted, EndedAt: &now,
	}); err != nil {
		t.Errorf("upsert after recovery: %v", err)
	}
}

// TestMultiProc_SerialAfterClose verifies that a second process can open
// the index successfully after the first has closed.
func TestMultiProc_SerialAfterClose(t *testing.T) {
	dir := t.TempDir()

	// First process: write a task, exit.
	helper1 := runHelper(t, "open_quick", dir)
	if err := helper1.Wait(); err != nil {
		t.Fatalf("helper1: %v", err)
	}

	// Second process: should now be able to open and write.
	helper2 := runHelper(t, "open_quick", dir)
	if err := helper2.Wait(); err != nil {
		t.Fatalf("helper2 (should succeed after helper1 closed): %v", err)
	}

	// Verify the row from helper1 survived (helper2 wrote the same ID, but
	// ON CONFLICT DO UPDATE preserves the row).
	idx, err := open(dir)
	if err != nil {
		t.Fatalf("open from test process: %v", err)
	}
	defer idx.Close()
	n, err := idx.TableCount("tasks")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("count after two helpers: got %d want 1", n)
	}
}
