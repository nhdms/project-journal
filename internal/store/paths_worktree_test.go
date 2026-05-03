package store

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// resolved returns the absolute path with symlinks evaluated. On macOS
// t.TempDir() returns a /var/... path that resolves to /private/var/... —
// git emits the resolved form, so tests must compare on equal footing.
func resolved(t *testing.T, p string) string {
	t.Helper()
	abs, err := filepath.Abs(p)
	if err != nil {
		t.Fatalf("abs(%q): %v", p, err)
	}
	r, err := filepath.EvalSymlinks(abs)
	if err != nil {
		t.Fatalf("evalsymlinks(%q): %v", abs, err)
	}
	return r
}

// initGitRepo runs `git init` in dir and sets a deterministic identity so
// `git worktree add` (which creates a commit) does not depend on the host
// gitconfig.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "test"},
		{"commit", "--allow-empty", "-q", "-m", "initial"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
}

func TestCanonicalRoot_NonGitDir(t *testing.T) {
	dir := t.TempDir()
	got, err := CanonicalRoot(dir)
	if err != nil {
		t.Fatalf("CanonicalRoot: %v", err)
	}
	want := resolved(t, dir)
	if got != want {
		t.Errorf("non-git dir: got %q, want %q", got, want)
	}
}

func TestCanonicalRoot_MainCheckout(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)
	got, err := CanonicalRoot(dir)
	if err != nil {
		t.Fatalf("CanonicalRoot: %v", err)
	}
	want := resolved(t, dir)
	if got != want {
		t.Errorf("main checkout: got %q, want %q", got, want)
	}
}

func TestCanonicalRoot_LinkedWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	main := t.TempDir()
	initGitRepo(t, main)

	// Create a linked worktree at a sibling path.
	wtBase := t.TempDir()
	wt := filepath.Join(wtBase, "feat-x")
	cmd := exec.Command("git", "-C", main, "worktree", "add", "-q", "-b", "feat-x", wt)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add: %v: %s", err, out)
	}

	canonicalMain, err := CanonicalRoot(main)
	if err != nil {
		t.Fatalf("canonical(main): %v", err)
	}
	canonicalWt, err := CanonicalRoot(wt)
	if err != nil {
		t.Fatalf("canonical(worktree): %v", err)
	}

	wantMain := resolved(t, main)
	if canonicalMain != wantMain {
		t.Errorf("main: got %q, want %q", canonicalMain, wantMain)
	}
	if canonicalWt != canonicalMain {
		t.Errorf("worktree should share canonical root: got %q, want %q", canonicalWt, canonicalMain)
	}

	// Both should derive the same key.
	if got, want := DeriveKey(canonicalWt), DeriveKey(canonicalMain); got != want {
		t.Errorf("DeriveKey diverged across worktrees: %q vs %q", got, want)
	}
}

func TestCanonicalRoot_FromSubdir(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	main := t.TempDir()
	initGitRepo(t, main)
	sub := filepath.Join(main, "internal", "deep")
	if err := exec.Command("mkdir", "-p", sub).Run(); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	got, err := CanonicalRoot(sub)
	if err != nil {
		t.Fatalf("canonical: %v", err)
	}
	want := resolved(t, main)
	if got != want {
		t.Errorf("subdir: got %q, want %q", got, want)
	}
}
