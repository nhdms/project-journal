package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/nhduc/project-journal/internal/llm"
	"github.com/nhduc/project-journal/internal/store"
	"github.com/spf13/cobra"
)

// NewInduceCmd creates `pj induce <id>`. Re-runs induce + autoeval against an
// existing trajectory. Useful for already-finished tasks (e.g. after
// improving prompts or upgrading model).
func NewInduceCmd() *cobra.Command {
	var auto bool
	cmd := &cobra.Command{
		Use:   "induce <id>",
		Short: "Re-run LLM induction (and autoeval) on an existing task's trajectory",
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
			t, ok := store.FindTask(tasks, id)
			if !ok {
				return fmt.Errorf("task %q not found", id)
			}
			events, err := store.LoadTrajectory(l, id)
			if err != nil {
				return err
			}
			if len(events) == 0 {
				return fmt.Errorf("task %s has no trajectory events to induce from", id)
			}
			if !llm.HasAPIKey() {
				return fmt.Errorf("OPENAI_API_KEY not set")
			}
			c, err := llm.NewClient()
			if err != nil {
				return err
			}

			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			fmt.Fprintln(os.Stderr, "Analyzing trajectory…")
			phaseTitle := resolvePhaseTitle(l, t)
			res := runLLMAnalysis(ctx, c, t, phaseTitle, events)

			if res.ProposalErr != nil && res.EvalErr != nil {
				return fmt.Errorf("both LLM calls failed: induce=%v; autoeval=%v", res.ProposalErr, res.EvalErr)
			}

			if auto {
				if res.ProposalErr == nil {
					applyProposal(&t, res.Proposal)
				}
				if err := store.ReplaceTask(l, t); err != nil {
					return err
				}
				embedAndStore(ctx, c, l, t)
				fmt.Printf("Re-induced %s\n", t.ID)
				return nil
			}

			printProposal(os.Stderr, res.Eval, res.Proposal, res.EvalErr, res.ProposalErr)
			r := Stdin()
			ans, err := Prompt(r, "[a]ccept / [e]dit / [s]kip: ")
			if err != nil {
				return err
			}
			switch strings.ToLower(strings.TrimSpace(ans)) {
			case "a", "accept", "":
				if res.ProposalErr == nil {
					applyProposal(&t, res.Proposal)
				}
			case "e", "edit":
				base := res.Proposal
				if res.ProposalErr != nil {
					base = llm.InduceProposal{}
				}
				edited, err := editProposal(base)
				if err != nil {
					return err
				}
				applyProposal(&t, edited)
			case "s", "skip":
				fmt.Fprintln(os.Stderr, "Skipped — task fields unchanged.")
				return nil
			default:
				fmt.Fprintln(os.Stderr, "Unknown choice; skipping.")
				return nil
			}

			// Optionally update status separately.
			if res.EvalErr == nil {
				yes, err := PromptYesNo(r, fmt.Sprintf("Update status to %s (autoeval)? [Y/n]: ", res.Eval.Status))
				if err != nil {
					return err
				}
				if yes {
					t.Status = mapAutoevalStatus(res.Eval.Status)
				}
			}

			if err := store.ReplaceTask(l, t); err != nil {
				return err
			}
			embedAndStore(ctx, c, l, t)
			fmt.Printf("Re-induced %s\n", t.ID)
			return nil
		},
	}
	cmd.Flags().BoolVar(&auto, "auto", false, "Apply proposal automatically without prompting")
	return cmd
}
