package cli

import (
	"fmt"
	"time"

	"github.com/nhduc/project-journal/internal/model"
	"github.com/nhduc/project-journal/internal/store"
	"github.com/spf13/cobra"
)

// NewPhaseCmd creates `pj phase` with subcommands.
func NewPhaseCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "phase",
		Short: "Manage phases (logical groupings of tasks)",
	}
	cmd.AddCommand(newPhaseAddCmd())
	return cmd
}

func newPhaseAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <id> <title>",
		Short: "Add a new phase",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, title := args[0], args[1]
			l, err := resolveLayout()
			if err != nil {
				return err
			}
			p := model.Phase{ID: id, Title: title, CreatedAt: time.Now().UTC()}
			if err := store.AppendPhase(l, p); err != nil {
				return err
			}
			fmt.Printf("Added phase %s — %s\n", p.ID, p.Title)
			return nil
		},
	}
}
