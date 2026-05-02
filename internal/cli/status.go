package cli

import (
	"fmt"

	"github.com/nhdms/project-journal/internal/model"
	"github.com/nhdms/project-journal/internal/store"
	"github.com/spf13/cobra"
)

// NewStatusCmd creates `pj status`.
func NewStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Print a high-level summary of the journal",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			l, err := resolveLayout()
			if err != nil {
				return err
			}
			phases, err := store.LoadPhases(l)
			if err != nil {
				return err
			}
			tasks, err := store.LoadTasks(l)
			if err != nil {
				return err
			}
			counts := map[string]int{}
			var lastFinished *model.Task
			for i := range tasks {
				t := tasks[i]
				counts[t.Status]++
				if t.EndedAt != nil {
					if lastFinished == nil || t.EndedAt.After(*lastFinished.EndedAt) {
						lastFinished = &t
					}
				}
			}
			cur, _ := store.ReadCurrent(l)
			fmt.Printf("Phases: %d\n", len(phases))
			fmt.Printf("Tasks:  %d\n", len(tasks))
			for _, st := range []string{
				model.StatusTodo, model.StatusInProgress, model.StatusCompleted,
				model.StatusPartial, model.StatusBlocked, model.StatusNeedsReview,
			} {
				fmt.Printf("  %-13s %d\n", st, counts[st])
			}
			if cur != "" {
				fmt.Printf("Current task: %s\n", cur)
			} else {
				fmt.Println("Current task: (none)")
			}
			if lastFinished != nil {
				fmt.Printf("Last finished: %s — %s [%s]\n", lastFinished.ID, lastFinished.Title, lastFinished.Status)
			}
			return nil
		},
	}
}
