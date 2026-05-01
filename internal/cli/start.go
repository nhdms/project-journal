package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/nhduc/project-journal/internal/model"
	"github.com/nhduc/project-journal/internal/store"
	"github.com/spf13/cobra"
)

// NewStartCmd creates `pj start`.
func NewStartCmd() *cobra.Command {
	var (
		title   string
		phaseID string
	)
	cmd := &cobra.Command{
		Use:   "start <id>",
		Short: "Start a task (mark in_progress, set current, print briefing)",
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
			t, ok := store.FindTask(tasks, id)
			if !ok {
				if noPromptFlag {
					return fmt.Errorf("task %q does not exist (and --no-prompt set)", id)
				}
				r := Stdin()
				yes, err := PromptYesNo(r, fmt.Sprintf("Task %s doesn't exist. Create it? [Y/n]: ", id))
				if err != nil {
					return err
				}
				if !yes {
					return fmt.Errorf("aborted")
				}
				if title == "" {
					title, err = Prompt(r, "Title: ")
					if err != nil {
						return err
					}
					if title == "" {
						return fmt.Errorf("title is required")
					}
				}
				if phaseID == "" {
					phaseID, err = Prompt(r, "Phase ID (blank for none): ")
					if err != nil {
						return err
					}
				}
				t = model.Task{
					ID:      id,
					PhaseID: phaseID,
					Title:   title,
					Status:  model.StatusTodo,
				}
				if err := store.AppendTask(l, t); err != nil {
					return err
				}
			}
			now := time.Now().UTC()
			t.Status = model.StatusInProgress
			t.StartedAt = &now
			if t.SessionID == "" {
				t.SessionID = NewSessionID()
			}
			if err := store.ReplaceTask(l, t); err != nil {
				return err
			}
			if err := store.WriteCurrent(l, t.ID); err != nil {
				return err
			}
			fmt.Printf("Started %s — %s (session %s)\n\n", t.ID, t.Title, t.SessionID)
			if err := RenderContext(os.Stdout, l, t.ID); err != nil {
				return err
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "Title to use when creating a new task")
	cmd.Flags().StringVar(&phaseID, "phase", "", "Phase ID to use when creating a new task")
	return cmd
}
