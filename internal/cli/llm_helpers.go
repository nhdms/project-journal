package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nhdms/project-journal/internal/llm"
	"github.com/nhdms/project-journal/internal/model"
	"github.com/nhdms/project-journal/internal/store"
)

// llmAnalysis bundles the parallel induce + autoeval results.
type llmAnalysis struct {
	Proposal    llm.InduceProposal
	ProposalErr error
	Eval        llm.AutoevalResult
	EvalErr     error
}

// runLLMAnalysis fires Induce and Autoeval in parallel and waits for both.
func runLLMAnalysis(ctx context.Context, c *llm.Client, t model.Task, phaseTitle string, events []model.TrajectoryEvent) llmAnalysis {
	var (
		wg  sync.WaitGroup
		res llmAnalysis
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		res.Proposal, res.ProposalErr = llm.Induce(ctx, c, t, phaseTitle, events)
	}()
	go func() {
		defer wg.Done()
		res.Eval, res.EvalErr = llm.Autoeval(ctx, c, t, events)
	}()
	wg.Wait()
	return res
}

// printProposal renders the proposed analysis in a compact box for stderr.
func printProposal(w io.Writer, eval llm.AutoevalResult, prop llm.InduceProposal, evalErr, propErr error) {
	fmt.Fprintln(w, "┌──────────────────────────────────────")
	if evalErr != nil {
		fmt.Fprintf(w, "│ Autoeval: ERROR — %v\n", evalErr)
	} else {
		fmt.Fprintf(w, "│ Proposed status: %s (conf %.2f)\n", eval.Status, eval.Confidence)
		if eval.Reason != "" {
			fmt.Fprintf(w, "│ Reason: %s\n", eval.Reason)
		}
	}
	fmt.Fprintln(w, "├──────────────────────────────────────")
	if propErr != nil {
		fmt.Fprintf(w, "│ Induce: ERROR — %v\n", propErr)
	} else {
		if prop.Summary != "" {
			fmt.Fprintf(w, "│ Summary: %s\n", prop.Summary)
		}
		fmt.Fprintf(w, "│ Files (%d):%s\n", len(prop.FilesTouched), inlineList(prop.FilesTouched))
		fmt.Fprintf(w, "│ Decisions (%d):%s\n", len(prop.KeyDecisions), inlineList(prop.KeyDecisions))
		fmt.Fprintf(w, "│ Blockers resolved (%d):%s\n", len(prop.BlockersResolved), inlineList(prop.BlockersResolved))
		fmt.Fprintf(w, "│ TODOs (%d):%s\n", len(prop.TodosLeft), inlineList(prop.TodosLeft))
		fmt.Fprintf(w, "│ Interfaces (%d):%s\n", len(prop.InterfacesExposed), inlineList(prop.InterfacesExposed))
		if len(prop.Tags) > 0 {
			fmt.Fprintf(w, "│ Tags: %s\n", strings.Join(prop.Tags, ", "))
		}
	}
	fmt.Fprintln(w, "└──────────────────────────────────────")
}

func inlineList(items []string) string {
	if len(items) == 0 {
		return " (none)"
	}
	const previewMax = 3
	var preview []string
	if len(items) > previewMax {
		preview = items[:previewMax]
	} else {
		preview = items
	}
	out := " " + strings.Join(preview, "; ")
	if len(items) > previewMax {
		out += fmt.Sprintf("; …(+%d more)", len(items)-previewMax)
	}
	return out
}

// applyProposal merges induce fields into the task in place.
func applyProposal(t *model.Task, p llm.InduceProposal) {
	if p.Summary != "" {
		t.Summary = p.Summary
	}
	if len(p.FilesTouched) > 0 {
		t.FilesTouched = p.FilesTouched
	}
	if len(p.KeyDecisions) > 0 {
		t.KeyDecisions = p.KeyDecisions
	}
	if len(p.BlockersResolved) > 0 {
		t.BlockersResolved = p.BlockersResolved
	}
	if len(p.TodosLeft) > 0 {
		t.TodosLeft = p.TodosLeft
	}
	if len(p.InterfacesExposed) > 0 {
		t.InterfacesExposed = p.InterfacesExposed
	}
	if len(p.Tags) > 0 {
		t.Tags = p.Tags
	}
}

// editProposal opens the proposal as JSON in $EDITOR and returns the parsed result.
func editProposal(p llm.InduceProposal) (llm.InduceProposal, error) {
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return llm.InduceProposal{}, err
	}
	data = append(data, '\n')
	r := Stdin()
	current := data
	for {
		edited, err := openInEditor(current, ".json")
		if err != nil {
			return llm.InduceProposal{}, err
		}
		var np llm.InduceProposal
		if err := json.Unmarshal(edited, &np); err != nil {
			fmt.Fprintf(os.Stderr, "Invalid JSON: %v\n", err)
			yes, perr := PromptYesNo(r, "Re-edit? [Y/n]: ")
			if perr != nil {
				return llm.InduceProposal{}, perr
			}
			if !yes {
				return llm.InduceProposal{}, fmt.Errorf("aborted: invalid JSON")
			}
			current = edited
			continue
		}
		return np, nil
	}
}

// embedAndStore embeds the task's title+summary+tags and upserts the cache.
// Errors are logged to stderr but not returned (best-effort).
func embedAndStore(ctx context.Context, c *llm.Client, l store.Layout, t model.Task) {
	if c == nil {
		return
	}
	text := llm.BuildEmbeddingText(t)
	if strings.TrimSpace(text) == "" {
		return
	}
	vec, err := c.Embed(ctx, text)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: embed failed for %s: %v\n", t.ID, err)
		return
	}
	rec := store.EmbeddingRecord{
		TaskID:    t.ID,
		Text:      text,
		Embedding: vec,
		UpdatedAt: time.Now().UTC(),
	}
	if err := store.UpsertEmbedding(l, rec); err != nil {
		fmt.Fprintf(os.Stderr, "warning: persist embedding for %s failed: %v\n", t.ID, err)
	}
}

// resolvePhaseTitle returns the task's phase title or "" if no phase or not found.
func resolvePhaseTitle(l store.Layout, t model.Task) string {
	if t.PhaseID == "" {
		return ""
	}
	phases, err := store.LoadPhases(l)
	if err != nil {
		return ""
	}
	if p, ok := store.FindPhase(phases, t.PhaseID); ok {
		return p.Title
	}
	return ""
}
