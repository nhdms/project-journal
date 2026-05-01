package cli

import (
	"fmt"
	"os"

	"github.com/nhduc/project-journal/internal/store"
	"github.com/spf13/cobra"
)

// NewCurrentCmd creates `pj current`.
func NewCurrentCmd() *cobra.Command {
	var quiet bool
	cmd := &cobra.Command{
		Use:   "current",
		Short: "Print the current active task ID (exit 1 if none)",
		Args:  cobra.NoArgs,
		// Use Run + manual exit so we can produce a non-zero exit cleanly.
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			cwd, err := os.Getwd()
			if err != nil {
				if !quiet {
					return err
				}
				os.Exit(1)
			}
			root, ok := store.FindRoot(cwd)
			if !ok {
				if !quiet {
					return fmt.Errorf("no .project-journal/ found")
				}
				os.Exit(1)
			}
			l := store.LayoutFor(root)
			cur, err := store.ReadCurrent(l)
			if err != nil {
				if !quiet {
					return err
				}
				os.Exit(1)
			}
			if cur == "" {
				os.Exit(1)
			}
			fmt.Println(cur)
			return nil
		},
	}
	cmd.Flags().BoolVar(&quiet, "quiet", false, "Suppress error messages")
	return cmd
}
