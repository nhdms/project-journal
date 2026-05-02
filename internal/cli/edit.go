package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/nhdms/project-journal/internal/model"
	"github.com/nhdms/project-journal/internal/store"
	"github.com/spf13/cobra"
)

// NewEditCmd creates `pj edit`.
func NewEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit <id>",
		Short: "Edit a task or phase as JSON in $EDITOR",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			l, err := resolveLayout()
			if err != nil {
				return err
			}

			tasks, err := store.LoadTasks(l)
			if err != nil {
				return err
			}
			if t, ok := store.FindTask(tasks, id); ok {
				return editTask(l, t)
			}
			phases, err := store.LoadPhases(l)
			if err != nil {
				return err
			}
			if p, ok := store.FindPhase(phases, id); ok {
				return editPhase(l, p)
			}
			return fmt.Errorf("no task or phase with id %q", id)
		},
	}
}

func editorBinary() string {
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	return "vi"
}

func openInEditor(initial []byte, suffix string) ([]byte, error) {
	tmp, err := os.CreateTemp("", "pj-edit-*"+suffix)
	if err != nil {
		return nil, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(initial); err != nil {
		_ = tmp.Close()
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, err
	}

	cmd := exec.Command(editorBinary(), tmpPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("editor exited with error: %w", err)
	}
	return os.ReadFile(tmpPath)
}

func editTask(l store.Layout, t model.Task) error {
	original, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	original = append(original, '\n')
	current := original
	r := Stdin()
	for {
		edited, err := openInEditor(current, ".json")
		if err != nil {
			return err
		}
		var nt model.Task
		if err := json.Unmarshal(edited, &nt); err != nil {
			fmt.Fprintf(os.Stderr, "Invalid JSON: %v\n", err)
			yes, perr := PromptYesNo(r, "Re-edit? [Y/n]: ")
			if perr != nil {
				return perr
			}
			if !yes {
				return fmt.Errorf("aborted: invalid JSON")
			}
			current = edited
			continue
		}
		if nt.ID != t.ID {
			return fmt.Errorf("task ID cannot be changed (%q -> %q)", t.ID, nt.ID)
		}
		return store.ReplaceTask(l, nt)
	}
}

func editPhase(l store.Layout, p model.Phase) error {
	original, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	original = append(original, '\n')
	current := original
	r := Stdin()
	for {
		edited, err := openInEditor(current, ".json")
		if err != nil {
			return err
		}
		var np model.Phase
		if err := json.Unmarshal(edited, &np); err != nil {
			fmt.Fprintf(os.Stderr, "Invalid JSON: %v\n", err)
			yes, perr := PromptYesNo(r, "Re-edit? [Y/n]: ")
			if perr != nil {
				return perr
			}
			if !yes {
				return fmt.Errorf("aborted: invalid JSON")
			}
			current = edited
			continue
		}
		if np.ID != p.ID {
			return fmt.Errorf("phase ID cannot be changed (%q -> %q)", p.ID, np.ID)
		}
		return store.ReplacePhase(l, np)
	}
}

