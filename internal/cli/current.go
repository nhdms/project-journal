package cli

import (
	"fmt"
	"os"

	"github.com/nhdms/project-journal/internal/store"
	"github.com/spf13/cobra"
)

// NewCurrentCmd creates `pj current`.
//
// Exit codes:
//
//	0 — a task ID was printed
//	1 — no active task (not an error; hooks use this to skip gracefully)
//	2 — real error (FS failure, layout error, etc.)
func NewCurrentCmd() *cobra.Command {
	var quiet bool
	cmd := &cobra.Command{
		Use:   "current",
		Short: "Print the current active task ID (exit 1 if none, exit 2 on error)",
		Args:  cobra.NoArgs,
		// SilenceErrors/Usage so cobra doesn't print on os.Exit paths.
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			cwd, err := os.Getwd()
			if err != nil {
				if !quiet {
					fmt.Fprintf(os.Stderr, "pj current: %v\n", err)
				}
				os.Exit(2)
			}
			root, ok := store.FindRoot(cwd)
			if !ok {
				// Not initialized — treated as "no task", not an error.
				os.Exit(1)
			}
			l, err := store.LayoutFor(root)
			if err != nil {
				if !quiet {
					fmt.Fprintf(os.Stderr, "pj current: %v\n", err)
				}
				os.Exit(2)
			}
			cur, err := store.ReadCurrent(l)
			if err != nil {
				if !quiet {
					fmt.Fprintf(os.Stderr, "pj current: %v\n", err)
				}
				os.Exit(2)
			}
			if cur == "" {
				// Journal exists but no task is active.
				os.Exit(1)
			}
			fmt.Println(cur)
			return nil
		},
	}
	cmd.Flags().BoolVar(&quiet, "quiet", false, "Suppress error messages")
	return cmd
}
