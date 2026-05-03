package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version is the pj CLI version. Keep in sync with plugin.json.
const Version = "0.5.0"

// NewVersionCmd creates `pj version`.
func NewVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print pj version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println(Version)
			return nil
		},
	}
}
