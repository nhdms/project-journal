package cli

import (
	"fmt"
	"os"

	"github.com/nhdms/project-journal/internal/store"
	"github.com/spf13/cobra"
)

// NewInitCmd creates `pj init`.
func NewInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize a .project-journal/ directory in the current directory",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			created, err := store.Init(cwd)
			if err != nil {
				return err
			}
			l, err := store.LayoutFor(cwd)
			if err != nil {
				return err
			}
			if !created {
				fmt.Printf("Already initialized at %s\n", l.Dir)
				return nil
			}
			fmt.Printf("Initialized journal at %s\n", l.Dir)
			return nil
		},
	}
}
