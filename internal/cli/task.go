package cli

import (
	"fmt"
	"strings"

	"github.com/nhduc/project-journal/internal/model"
	"github.com/nhduc/project-journal/internal/store"
	"github.com/spf13/cobra"
)

// NewTaskCmd creates `pj task` with subcommands.
func NewTaskCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "task",
		Short: "Manage tasks",
	}
	cmd.AddCommand(newTaskAddCmd())
	return cmd
}

func newTaskAddCmd() *cobra.Command {
	var (
		phaseID   string
		dependsOn string
	)
	cmd := &cobra.Command{
		Use:   "add <id> <title>",
		Short: "Add a new task with status=todo",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, title := args[0], args[1]
			l, err := resolveLayout()
			if err != nil {
				return err
			}
			if phaseID != "" {
				phases, err := store.LoadPhases(l)
				if err != nil {
					return err
				}
				if _, ok := store.FindPhase(phases, phaseID); !ok {
					fmt.Printf("warning: phase %q does not exist (creating task anyway)\n", phaseID)
				}
			}
			t := model.Task{
				ID:        id,
				PhaseID:   phaseID,
				DependsOn: parseCSV(dependsOn),
				Title:     title,
				Status:    model.StatusTodo,
			}
			if err := store.AppendTask(l, t); err != nil {
				return err
			}
			fmt.Printf("Added task %s — %s\n", t.ID, t.Title)
			return nil
		},
	}
	cmd.Flags().StringVar(&phaseID, "phase", "", "Parent phase ID")
	cmd.Flags().StringVar(&dependsOn, "depends-on", "", "Comma-separated list of task IDs this task depends on")
	return cmd
}

func parseCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
