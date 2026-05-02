package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nhdms/project-journal/internal/llm"
	"github.com/nhdms/project-journal/internal/model"
	"github.com/nhdms/project-journal/internal/store"
	"github.com/spf13/cobra"
)

// minAutoConfidence: below this, --auto downgrades to needs_review.
const minAutoConfidence = 0.5

// NewFinishCmd creates `pj finish`.
func NewFinishCmd() *cobra.Command {
	var auto bool
	cmd := &cobra.Command{
		Use:   "finish <id>",
		Short: "Finish a task. With OPENAI_API_KEY set, proposes summary + status from trajectory.",
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
			if t.Status != model.StatusInProgress {
				return fmt.Errorf("task %s is not in_progress (status=%s)", id, t.Status)
			}
			now := time.Now().UTC()
			t.EndedAt = &now

			events, _ := store.LoadTrajectory(l, id)

			// Decide whether to attempt LLM analysis.
			var client *llm.Client
			if llm.HasAPIKey() && len(events) > 0 {
				c, err := llm.NewClient()
				if err != nil {
					fmt.Fprintf(os.Stderr, "warning: LLM unavailable: %v\n", err)
				} else {
					client = c
				}
			}

			if client != nil {
				if handled, ferr := finishWithLLM(cmd.Context(), client, l, &t, events, auto); ferr != nil {
					return ferr
				} else if handled {
					return finalizeFinish(l, t)
				}
				// fall through to manual on LLM total failure
			}

			// Manual / Phase-1 fallback.
			if auto {
				t.Status = model.StatusNeedsReview
			} else {
				if err := manualFinishPrompt(&t); err != nil {
					return err
				}
			}
			return finalizeFinish(l, t)
		},
	}
	cmd.Flags().BoolVar(&auto, "auto", false, "Skip prompts; auto-apply LLM proposal (or mark needs_review without LLM)")
	return cmd
}

// finishWithLLM runs induce+autoeval and either applies the proposal or asks
// the user. Returns (handled, error). If handled=false and error=nil, caller
// should fall through to manual flow.
func finishWithLLM(ctx context.Context, c *llm.Client, l store.Layout, t *model.Task, events []model.TrajectoryEvent, auto bool) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	fmt.Fprintln(os.Stderr, "Analyzing trajectory…")
	phaseTitle := resolvePhaseTitle(l, *t)
	res := runLLMAnalysis(ctx, c, *t, phaseTitle, events)

	if res.ProposalErr != nil && res.EvalErr != nil {
		fmt.Fprintf(os.Stderr, "warning: both LLM calls failed (induce: %v; autoeval: %v); falling back to manual\n", res.ProposalErr, res.EvalErr)
		return false, nil
	}

	if auto {
		if res.ProposalErr == nil {
			applyProposal(t, res.Proposal)
		}
		switch {
		case res.EvalErr != nil:
			t.Status = model.StatusNeedsReview
		case res.Eval.Confidence < minAutoConfidence:
			fmt.Fprintf(os.Stderr, "autoeval confidence %.2f < %.2f → status=needs_review\n", res.Eval.Confidence, minAutoConfidence)
			t.Status = model.StatusNeedsReview
		default:
			t.Status = mapAutoevalStatus(res.Eval.Status)
		}
		// embed best-effort
		embedAndStore(ctx, c, l, *t)
		return true, nil
	}

	// Interactive: print and prompt.
	printProposal(os.Stderr, res.Eval, res.Proposal, res.EvalErr, res.ProposalErr)
	r := Stdin()
	ans, err := Prompt(r, "[a]ccept / [e]dit / [r]eject / [n]eeds-review: ")
	if err != nil {
		return true, err
	}
	switch strings.ToLower(strings.TrimSpace(ans)) {
	case "a", "accept", "":
		if res.ProposalErr == nil {
			applyProposal(t, res.Proposal)
		}
		if res.EvalErr == nil {
			t.Status = mapAutoevalStatus(res.Eval.Status)
		} else {
			t.Status = model.StatusNeedsReview
		}
		embedAndStore(ctx, c, l, *t)
	case "e", "edit":
		base := res.Proposal
		if res.ProposalErr != nil {
			base = llm.InduceProposal{}
		}
		edited, err := editProposal(base)
		if err != nil {
			return true, err
		}
		applyProposal(t, edited)
		if res.EvalErr == nil {
			t.Status = mapAutoevalStatus(res.Eval.Status)
		} else {
			t.Status = model.StatusNeedsReview
		}
		embedAndStore(ctx, c, l, *t)
	case "r", "reject":
		// keep task fields as-is; do NOT change status from autoeval, leave as needs_review
		// per spec: still set EndedAt + clear current, do not save trajectory-derived fields.
		t.Status = model.StatusNeedsReview
	case "n", "needs-review", "needs_review":
		t.Status = model.StatusNeedsReview
	default:
		// treat unknown as accept
		if res.ProposalErr == nil {
			applyProposal(t, res.Proposal)
		}
		if res.EvalErr == nil {
			t.Status = mapAutoevalStatus(res.Eval.Status)
		} else {
			t.Status = model.StatusNeedsReview
		}
		embedAndStore(ctx, c, l, *t)
	}
	return true, nil
}

func manualFinishPrompt(t *model.Task) error {
	r := Stdin()
	summary, err := PromptMultiline(r, "Summary (multi-line; finish with empty line):")
	if err != nil {
		return err
	}
	if summary != "" {
		t.Summary = summary
	}
	ans, err := Prompt(r, "Status? [c]ompleted / [p]artial / [b]locked: ")
	if err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(ans)) {
	case "p", "partial":
		t.Status = model.StatusPartial
	case "b", "blocked":
		t.Status = model.StatusBlocked
	case "c", "completed", "":
		t.Status = model.StatusCompleted
	default:
		t.Status = model.StatusCompleted
	}
	return nil
}

// finalizeFinish persists the task and clears `current` if it pointed here.
func finalizeFinish(l store.Layout, t model.Task) error {
	if err := store.ReplaceTask(l, t); err != nil {
		return err
	}
	cur, _ := store.ReadCurrent(l)
	if cur == t.ID {
		if err := store.WriteCurrent(l, ""); err != nil {
			return err
		}
	}
	fmt.Printf("Finished %s [status=%s]\n", t.ID, t.Status)
	return nil
}

// mapAutoevalStatus normalises LLM-returned status strings to model constants.
// Unknown values become needs_review.
func mapAutoevalStatus(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "completed", "complete", "done":
		return model.StatusCompleted
	case "partial":
		return model.StatusPartial
	case "blocked":
		return model.StatusBlocked
	case "needs_review", "needs-review", "review":
		return model.StatusNeedsReview
	default:
		return model.StatusNeedsReview
	}
}
