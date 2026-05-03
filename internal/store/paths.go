package store

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MarkerName is the on-disk name for the per-repo journal pointer. It is a
// regular file (JSON containing a key) for new journals; older journals store
// data directly inside a directory of the same name and are still recognized.
const MarkerName = ".project-journal"

// DirName is retained for compatibility with callers that referenced the
// legacy in-tree directory name.
const DirName = MarkerName

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

// Marker is the JSON schema for the .project-journal marker file.
type Marker struct {
	Key string `json:"key"`
}

// HomeBase returns the parent directory under $HOME that holds per-project
// journal data: ~/.project-journal.
func HomeBase() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".project-journal"), nil
}

// DeriveKey returns a stable, human-readable key for the journal rooted at
// absRoot. Format: <sanitized-basename>-<8-hex-chars-of-sha256(absRoot)>.
func DeriveKey(absRoot string) string {
	base := strings.ToLower(filepath.Base(absRoot))
	var b strings.Builder
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	clean := strings.Trim(b.String(), "-")
	if clean == "" {
		clean = "journal"
	}
	sum := sha256.Sum256([]byte(absRoot))
	return fmt.Sprintf("%s-%x", clean, sum[:4])
}

// readMarker inspects root/MarkerName. If it's a regular file, the parsed
// marker is returned with isFile=true. If it's a directory (legacy in-tree
// layout) or absent, isFile=false. Errors are returned only for IO failures
// or malformed JSON in a regular file.
func readMarker(root string) (m Marker, isFile bool, err error) {
	p := filepath.Join(root, MarkerName)
	st, statErr := os.Lstat(p)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return Marker{}, false, nil
		}
		return Marker{}, false, statErr
	}
	if st.IsDir() {
		return Marker{}, false, nil
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return Marker{}, false, err
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return Marker{}, false, fmt.Errorf("parse %s: %w", p, err)
	}
	if strings.TrimSpace(m.Key) == "" {
		return Marker{}, false, fmt.Errorf("%s: empty key", p)
	}
	return m, true, nil
}

// layoutAt builds a Layout whose data files live under dir.
func layoutAt(root, dir string) Layout {
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

// LayoutFor returns the journal layout for the repo rooted at root. When a
// marker file is present the data lives under ~/.project-journal/<key>/;
// when only a legacy directory is present, the in-tree path is used. If
// neither exists, the in-tree path is returned (callers like Init use this
// to construct the initial layout before any data has been written).
func LayoutFor(root string) (Layout, error) {
	m, isFile, err := readMarker(root)
	if err != nil {
		return Layout{}, err
	}
	if isFile {
		base, err := HomeBase()
		if err != nil {
			return Layout{}, err
		}
		return layoutAt(root, filepath.Join(base, m.Key)), nil
	}
	return layoutAt(root, filepath.Join(root, MarkerName)), nil
}

// FindRoot walks up from start looking for a .project-journal marker (file
// or directory). Returns ("", false) if not found.
func FindRoot(start string) (string, bool) {
	cur, err := filepath.Abs(start)
	if err != nil {
		return "", false
	}
	for {
		if _, err := os.Lstat(filepath.Join(cur, MarkerName)); err == nil {
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
	Version    int       `json:"version"`
	CreatedAt  time.Time `json:"created_at"`
	LLMEnabled bool      `json:"llm_enabled,omitempty"` // default false; must be explicitly opted in
}

// LoadConfig reads config.json for the given layout. Returns a zero Config
// (with LLMEnabled=false) if the file is absent or the field is missing —
// safe for backwards compatibility with older journals.
func LoadConfig(l Layout) (Config, error) {
	data, err := os.ReadFile(l.Config)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, nil
		}
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", l.Config, err)
	}
	return cfg, nil
}

// SaveConfig writes cfg to config.json atomically.
func SaveConfig(l Layout, cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return WriteFileAtomic(l.Config, data, 0o644)
}

// Init creates the home-dir data directory plus the marker file at root. If
// a legacy in-tree .project-journal/ directory already exists, its contents
// are migrated to ~/.project-journal/<key>/ via os.Rename and the marker
// file is written in its place. Returns true if the journal was newly
// created or migrated, false if it was already present.
func Init(root string) (bool, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false, err
	}
	markerPath := filepath.Join(absRoot, MarkerName)

	st, statErr := os.Lstat(markerPath)
	switch {
	case statErr == nil && st.IsDir():
		return migrateLegacy(absRoot)
	case statErr == nil:
		l, err := LayoutFor(absRoot)
		if err != nil {
			return false, err
		}
		return false, ensureDataDir(l)
	case !os.IsNotExist(statErr):
		return false, statErr
	}

	key := DeriveKey(absRoot)
	base, err := HomeBase()
	if err != nil {
		return false, err
	}
	dataDir := filepath.Join(base, key)
	if err := scaffoldDataDir(dataDir); err != nil {
		return false, err
	}
	if err := writeMarker(markerPath, key); err != nil {
		return false, err
	}
	return true, nil
}

// migrateLegacy moves an existing in-tree .project-journal/ directory to
// ~/.project-journal/<key>/ and replaces the in-tree path with a marker file.
func migrateLegacy(absRoot string) (bool, error) {
	legacyDir := filepath.Join(absRoot, MarkerName)
	key := DeriveKey(absRoot)
	base, err := HomeBase()
	if err != nil {
		return false, err
	}
	dataDir := filepath.Join(base, key)

	if _, err := os.Stat(dataDir); err == nil {
		return false, fmt.Errorf("migration target already exists: %s (move or remove it first)", dataDir)
	} else if !os.IsNotExist(err) {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(dataDir), 0o755); err != nil {
		return false, err
	}
	if err := os.Rename(legacyDir, dataDir); err != nil {
		return false, fmt.Errorf("migrate %s -> %s: %w", legacyDir, dataDir, err)
	}
	if err := writeMarker(filepath.Join(absRoot, MarkerName), key); err != nil {
		return false, err
	}
	return true, nil
}

func writeMarker(path, key string) error {
	data, err := json.MarshalIndent(Marker{Key: key}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	// 0o600: the marker file contains the journal key (a path component);
	// readable only by the owner to avoid leaking project structure.
	return WriteFileAtomic(path, data, 0o600)
}

// scaffoldDataDir creates a fresh data directory with empty jsonl files and
// an initial config.json.
func scaffoldDataDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sessions"), 0o755); err != nil {
		return fmt.Errorf("mkdir sessions: %w", err)
	}
	cfg := Config{Version: 1, CreatedAt: time.Now().UTC()}
	cfgData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	cfgData = append(cfgData, '\n')
	if err := WriteFileAtomic(filepath.Join(dir, "config.json"), cfgData, 0o644); err != nil {
		return err
	}
	for _, name := range []string{"phases.jsonl", "tasks.jsonl", "current"} {
		full := filepath.Join(dir, name)
		if _, err := os.Stat(full); os.IsNotExist(err) {
			if err := WriteFileAtomic(full, []byte{}, 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

// ensureDataDir backfills any missing files inside an existing data dir.
func ensureDataDir(l Layout) error {
	if err := os.MkdirAll(l.Dir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(l.SessionsDir, 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(l.Config); os.IsNotExist(err) {
		cfg := Config{Version: 1, CreatedAt: time.Now().UTC()}
		d, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return err
		}
		d = append(d, '\n')
		if err := WriteFileAtomic(l.Config, d, 0o644); err != nil {
			return err
		}
	}
	for _, p := range []string{l.PhasesJSONL, l.TasksJSONL, l.Current} {
		if _, err := os.Stat(p); os.IsNotExist(err) {
			if err := WriteFileAtomic(p, []byte{}, 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

// ResolveRoot finds an existing journal root or, if interactive, offers to
// init one at start. If noPrompt is true, the function will only succeed if
// a journal already exists.
func ResolveRoot(start string, noPrompt bool) (Layout, error) {
	if root, ok := FindRoot(start); ok {
		return LayoutFor(root)
	}
	if noPrompt {
		return Layout{}, fmt.Errorf("no %s found from %s (use `pj init` first)", MarkerName, start)
	}
	fmt.Fprintf(os.Stderr, "No %s found. Init at %s? [Y/n]: ", MarkerName, start)
	r := bufio.NewReader(os.Stdin)
	line, _ := r.ReadString('\n')
	ans := strings.ToLower(strings.TrimSpace(line))
	if ans != "" && ans != "y" && ans != "yes" {
		return Layout{}, fmt.Errorf("aborted: no journal initialized")
	}
	if _, err := Init(start); err != nil {
		return Layout{}, err
	}
	return LayoutFor(start)
}
