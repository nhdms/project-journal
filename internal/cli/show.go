package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/nhdms/project-journal/internal/model"
	"github.com/nhdms/project-journal/internal/store"
	"github.com/spf13/cobra"
)

// NewShowCmd creates `pj show`.
func NewShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show a task or phase",
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
				printTask(t)
				return nil
			}
			phases, err := store.LoadPhases(l)
			if err != nil {
				return err
			}
			if p, ok := store.FindPhase(phases, id); ok {
				printPhase(p)
				return nil
			}
			return fmt.Errorf("no task or phase with id %q", id)
		},
	}
}

func printPhase(p model.Phase) {
	fmt.Printf("Phase %s\n", p.ID)
	fmt.Printf("  Title:      %s\n", p.Title)
	fmt.Printf("  Created at: %s\n", p.CreatedAt.Format(time.RFC3339))
}

func printTask(t model.Task) {
	fmt.Printf("Task %s\n", t.ID)
	fmt.Printf("  Title:    %s\n", t.Title)
	fmt.Printf("  Status:   %s\n", t.Status)
	if t.PhaseID != "" {
		fmt.Printf("  Phase:    %s\n", t.PhaseID)
	}
	if len(t.DependsOn) > 0 {
		fmt.Printf("  Depends:  %s\n", strings.Join(t.DependsOn, ", "))
	}
	if t.SessionID != "" {
		fmt.Printf("  Session:  %s\n", t.SessionID)
	}
	if t.StartedAt != nil {
		fmt.Printf("  Started:  %s\n", t.StartedAt.Format(time.RFC3339))
	}
	if t.EndedAt != nil {
		fmt.Printf("  Ended:    %s\n", t.EndedAt.Format(time.RFC3339))
	}
	if t.UserIntent != "" {
		fmt.Printf("  Intent:   %s\n", t.UserIntent)
	}
	if t.Summary != "" {
		fmt.Println("  Summary:")
		for _, line := range strings.Split(t.Summary, "\n") {
			fmt.Printf("    %s\n", line)
		}
	}
	printList("  Files touched", t.FilesTouched)
	printList("  Key decisions", t.KeyDecisions)
	printList("  Blockers resolved", t.BlockersResolved)
	printList("  TODOs left", t.TodosLeft)
	printList("  Interfaces exposed", t.InterfacesExposed)
	printList("  Tags", t.Tags)
}

func printList(label string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Printf("%s:\n", label)
	for _, x := range items {
		fmt.Printf("    - %s\n", x)
	}
}
