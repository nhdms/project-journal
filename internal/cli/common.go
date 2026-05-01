package cli

import (
	"os"

	"github.com/nhduc/project-journal/internal/store"
	"github.com/spf13/cobra"
)

// noPromptFlag is a global flag accessible to all subcommands.
var noPromptFlag bool

// AddGlobalFlags registers global flags on the root command.
func AddGlobalFlags(root *cobra.Command) {
	root.PersistentFlags().BoolVar(&noPromptFlag, "no-prompt", false, "Disable interactive prompts (fail instead of asking)")
}

// resolveLayout finds or initializes the journal layout based on cwd and the
// --no-prompt flag.
func resolveLayout() (store.Layout, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return store.Layout{}, err
	}
	return store.ResolveRoot(cwd, noPromptFlag)
}
