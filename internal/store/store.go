package store

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/nhdms/project-journal/internal/model"
)

// AppendJSONL appends a single JSON-encoded record (followed by \n) to path.
// Creates the file if missing.
func AppendJSONL(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if _, err := f.Write(data); err != nil {
		return err
	}
	return nil
}

// readLines reads non-empty lines from path. Returns empty slice if file missing.
func readLines(path string) ([][]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var lines [][]byte
	r := bufio.NewReader(f)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			trim := bytes.TrimRight(line, "\r\n")
			if len(trim) > 0 {
				cp := make([]byte, len(trim))
				copy(cp, trim)
				lines = append(lines, cp)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	return lines, nil
}

// LoadPhases reads all phases from phases.jsonl.
func LoadPhases(l Layout) ([]model.Phase, error) {
	lines, err := readLines(l.PhasesJSONL)
	if err != nil {
		return nil, err
	}
	out := make([]model.Phase, 0, len(lines))
	for i, ln := range lines {
		var p model.Phase
		if err := json.Unmarshal(ln, &p); err != nil {
			return nil, fmt.Errorf("phases.jsonl line %d: %w", i+1, err)
		}
		out = append(out, p)
	}
	return out, nil
}

// LoadTasks reads all tasks from tasks.jsonl.
func LoadTasks(l Layout) ([]model.Task, error) {
	lines, err := readLines(l.TasksJSONL)
	if err != nil {
		return nil, err
	}
	out := make([]model.Task, 0, len(lines))
	for i, ln := range lines {
		var t model.Task
		if err := json.Unmarshal(ln, &t); err != nil {
			return nil, fmt.Errorf("tasks.jsonl line %d: %w", i+1, err)
		}
		out = append(out, t)
	}
	return out, nil
}

// FindPhase returns a copy of the phase with id, or (zero, false).
func FindPhase(phases []model.Phase, id string) (model.Phase, bool) {
	for _, p := range phases {
		if p.ID == id {
			return p, true
		}
	}
	return model.Phase{}, false
}

// FindTask returns a copy of the task with id, or (zero, false).
func FindTask(tasks []model.Task, id string) (model.Task, bool) {
	for _, t := range tasks {
		if t.ID == id {
			return t, true
		}
	}
	return model.Task{}, false
}

// AppendPhase appends a phase, erroring if a phase with the same ID exists.
// On success, mirrors the phase into the derived index.
func AppendPhase(l Layout, p model.Phase) error {
	return withLock(l.Dir, func() error {
		existing, err := LoadPhases(l)
		if err != nil {
			return err
		}
		if _, ok := FindPhase(existing, p.ID); ok {
			return fmt.Errorf("phase %q already exists", p.ID)
		}
		if err := AppendJSONL(l.PhasesJSONL, p); err != nil {
			return err
		}
		return mirrorPhase(l.Dir, p)
	})
}

// AppendTask appends a task, erroring if a task with the same ID exists.
// On success, mirrors the task into the derived index.
func AppendTask(l Layout, t model.Task) error {
	return withLock(l.Dir, func() error {
		existing, err := LoadTasks(l)
		if err != nil {
			return err
		}
		if _, ok := FindTask(existing, t.ID); ok {
			return fmt.Errorf("task %q already exists", t.ID)
		}
		if err := AppendJSONL(l.TasksJSONL, t); err != nil {
			return err
		}
		return mirrorTask(l.Dir, t)
	})
}

// ReplaceTask replaces a task by ID. Errors if no task with that ID exists.
// Rewrites tasks.jsonl atomically and mirrors into the derived index.
func ReplaceTask(l Layout, t model.Task) error {
	return withLock(l.Dir, func() error {
		tasks, err := LoadTasks(l)
		if err != nil {
			return err
		}
		found := false
		for i, x := range tasks {
			if x.ID == t.ID {
				tasks[i] = t
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("task %q not found", t.ID)
		}
		if err := writeTasks(l, tasks); err != nil {
			return err
		}
		return mirrorTask(l.Dir, t)
	})
}

// ReplacePhase replaces a phase by ID. Errors if not found. Mirrors into
// the derived index on success.
func ReplacePhase(l Layout, p model.Phase) error {
	return withLock(l.Dir, func() error {
		phases, err := LoadPhases(l)
		if err != nil {
			return err
		}
		found := false
		for i, x := range phases {
			if x.ID == p.ID {
				phases[i] = p
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("phase %q not found", p.ID)
		}
		if err := writePhases(l, phases); err != nil {
			return err
		}
		return mirrorPhase(l.Dir, p)
	})
}

func writeTasks(l Layout, tasks []model.Task) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	for _, t := range tasks {
		if err := enc.Encode(t); err != nil {
			return err
		}
	}
	return WriteFileAtomic(l.TasksJSONL, buf.Bytes(), 0o644)
}

func writePhases(l Layout, phases []model.Phase) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	for _, p := range phases {
		if err := enc.Encode(p); err != nil {
			return err
		}
	}
	return WriteFileAtomic(l.PhasesJSONL, buf.Bytes(), 0o644)
}

// ReadCurrent returns the active task ID, or "" if none.
func ReadCurrent(l Layout) (string, error) {
	data, err := os.ReadFile(l.Current)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(bytes.TrimSpace(data)), nil
}

// WriteCurrent sets the active task ID (empty string clears it).
func WriteCurrent(l Layout, id string) error {
	return WriteFileAtomic(l.Current, []byte(id), 0o644)
}

// AppendTrajectory appends a trajectory event for taskID and mirrors it
// into the derived index.
func AppendTrajectory(l Layout, taskID string, ev model.TrajectoryEvent) error {
	if err := os.MkdirAll(l.SessionsDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(l.SessionsDir, taskID+".jsonl")
	return withLock(l.Dir, func() error {
		if err := AppendJSONL(path, ev); err != nil {
			return err
		}
		return mirrorTrajectory(l.Dir, taskID, ev)
	})
}

// LoadTrajectory loads all events for a task. Returns nil if no log exists.
func LoadTrajectory(l Layout, taskID string) ([]model.TrajectoryEvent, error) {
	path := filepath.Join(l.SessionsDir, taskID+".jsonl")
	lines, err := readLines(path)
	if err != nil {
		return nil, err
	}
	out := make([]model.TrajectoryEvent, 0, len(lines))
	for i, ln := range lines {
		var ev model.TrajectoryEvent
		if err := json.Unmarshal(ln, &ev); err != nil {
			return nil, fmt.Errorf("%s line %d: %w", path, i+1, err)
		}
		out = append(out, ev)
	}
	return out, nil
}
