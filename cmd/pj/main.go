package main

import (
	"fmt"
	"os"

	"github.com/nhdms/project-journal/internal/cli"
	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:           "pj",
		Short:         "project-journal — task journal for multi-task projects",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cli.AddGlobalFlags(root)
	root.AddCommand(
		cli.NewInitCmd(),
		cli.NewPhaseCmd(),
		cli.NewTaskCmd(),
		cli.NewStartCmd(),
		cli.NewFinishCmd(),
		cli.NewLogCmd(),
		cli.NewShowCmd(),
		cli.NewTreeCmd(),
		cli.NewContextCmd(),
		cli.NewEditCmd(),
		cli.NewStatusCmd(),
		cli.NewCurrentCmd(),
		cli.NewInduceCmd(),
		cli.NewReindexCmd(),
	)
	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "pj: %v\n", err)
		os.Exit(1)
	}
}
