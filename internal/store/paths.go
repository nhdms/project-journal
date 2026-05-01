package store

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DirName is the journal directory name created by `pj init`.
const DirName = ".project-journal"

// Layout returns paths to journal artifacts under root.
type Layout struct {
	Root        string
	Dir         string
	Config      string
	Current     string
	PhasesJSONL string
	TasksJSONL  string
	SessionsDir string
}

// LayoutFor returns the journal layout rooted at the given directory.
func LayoutFor(root string) Layout {
	dir := filepath.Join(root, DirName)
	return Layout{
		Root:        root,
		Dir:         dir,
		Config:      filepath.Join(dir, "config.json"),
		Current:     filepath.Join(dir, "current"),
		PhasesJSONL: filepath.Join(dir, "phases.jsonl"),
		TasksJSONL:  filepath.Join(dir, "tasks.jsonl"),
		SessionsDir: filepath.Join(dir, "sessions"),
	}
}

// FindRoot walks up from start looking for a directory containing
// .project-journal/. Returns ("", false) if not found.
func FindRoot(start string) (string, bool) {
	cur, err := filepath.Abs(start)
	if err != nil {
		return "", false
	}
	for {
		if st, err := os.Stat(filepath.Join(cur, DirName)); err == nil && st.IsDir() {
			return cur, true
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", false
		}
		cur = parent
	}
}

// Config is the on-disk schema for config.json.
type Config struct {
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
}

// Init creates the journal directory and config at root if missing.
// Returns true if it was newly created.
func Init(root string) (bool, error) {
	l := LayoutFor(root)
	if st, err := os.Stat(l.Dir); err == nil && st.IsDir() {
		return false, nil
	}
	if err := os.MkdirAll(l.Dir, 0o755); err != nil {
		return false, fmt.Errorf("mkdir %s: %w", l.Dir, err)
	}
	if err := os.MkdirAll(l.SessionsDir, 0o755); err != nil {
		return false, fmt.Errorf("mkdir %s: %w", l.SessionsDir, err)
	}
	cfg := Config{Version: 1, CreatedAt: time.Now().UTC()}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return false, err
	}
	data = append(data, '\n')
	if err := WriteFileAtomic(l.Config, data, 0o644); err != nil {
		return false, err
	}
	// Touch jsonl files and current.
	for _, p := range []string{l.PhasesJSONL, l.TasksJSONL, l.Current} {
		if _, err := os.Stat(p); os.IsNotExist(err) {
			if err := WriteFileAtomic(p, []byte{}, 0o644); err != nil {
				return false, err
			}
		}
	}
	return true, nil
}

// ResolveRoot finds an existing journal root or, if interactive, offers to
// init one at start. If noPrompt is true, the function will only succeed if
// a journal already exists.
func ResolveRoot(start string, noPrompt bool) (Layout, error) {
	if root, ok := FindRoot(start); ok {
		return LayoutFor(root), nil
	}
	if noPrompt {
		return Layout{}, fmt.Errorf("no %s/ found from %s (use `pj init` first)", DirName, start)
	}
	fmt.Fprintf(os.Stderr, "No %s/ found. Init at %s? [Y/n]: ", DirName, start)
	r := bufio.NewReader(os.Stdin)
	line, _ := r.ReadString('\n')
	ans := strings.ToLower(strings.TrimSpace(line))
	if ans != "" && ans != "y" && ans != "yes" {
		return Layout{}, fmt.Errorf("aborted: no journal initialized")
	}
	if _, err := Init(start); err != nil {
		return Layout{}, err
	}
	return LayoutFor(start), nil
}
