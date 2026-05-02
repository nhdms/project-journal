package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/nhdms/project-journal/internal/llm"
	"github.com/nhdms/project-journal/internal/model"
	"github.com/nhdms/project-journal/internal/store"
	"github.com/spf13/cobra"
)

// NewContextCmd creates `pj context`.
func NewContextCmd() *cobra.Command {
	var forID string
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Render a markdown briefing for the current (or specified) task",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			l, err := resolveLayout()
			if err != nil {
				return err
			}
			id := forID
			if id == "" {
				cur, err := store.ReadCurrent(l)
				if err != nil {
					return err
				}
				if cur == "" {
					return fmt.Errorf("no current task; pass --for <id>")
				}
				id = cur
			}
			return RenderContext(os.Stdout, l, id)
		},
	}
	cmd.Flags().StringVar(&forID, "for", "", "Task ID to render briefing for (defaults to current)")
	return cmd
}

// RenderContext writes the markdown briefing for taskID to w.
func RenderContext(w io.Writer, l store.Layout, taskID string) error {
	tasks, err := store.LoadTasks(l)
	if err != nil {
		return err
	}
	phases, err := store.LoadPhases(l)
	if err != nil {
		return err
	}
	t, ok := store.FindTask(tasks, taskID)
	if !ok {
		return fmt.Errorf("task %q not found", taskID)
	}

	var phase model.Phase
	hasPhase := false
	if t.PhaseID != "" {
		if p, ok := store.FindPhase(phases, t.PhaseID); ok {
			phase = p
			hasPhase = true
		}
	}

	fmt.Fprintf(w, "# 🎯 Current Task\n\n")
	fmt.Fprintf(w, "- **ID:** %s\n", t.ID)
	fmt.Fprintf(w, "- **Title:** %s\n", t.Title)
	if hasPhase {
		fmt.Fprintf(w, "- **Phase:** %s — %s\n", phase.ID, phase.Title)
	}
	if t.UserIntent != "" {
		fmt.Fprintf(w, "- **Intent:** %s\n", t.UserIntent)
	}
	fmt.Fprintln(w)

	if hasPhase {
		fmt.Fprintf(w, "## 📦 Phase Goal\n\n%s\n\n", phase.Title)
	}

	// Hard dependencies.
	if len(t.DependsOn) > 0 {
		fmt.Fprintf(w, "## 🔗 Hard dependencies\n\n")
		for _, depID := range t.DependsOn {
			if dep, ok := store.FindTask(tasks, depID); ok {
				fmt.Fprintf(w, "- %s — %s [%s]\n", dep.ID, dep.Title, dep.Status)
				if dep.Summary != "" {
					fmt.Fprintf(w, "  %s\n", dep.Summary)
				}
			} else {
				fmt.Fprintf(w, "- %s _(not found)_\n", depID)
			}
		}
		fmt.Fprintln(w)
	}

	// Relevant past tasks: blended (embeddings if available + heuristics).
	relevant := selectRelevantTasks(l, t, tasks)
	fmt.Fprintf(w, "## ✨ Relevant past tasks\n\n")
	if len(relevant) == 0 {
		fmt.Fprintln(w, "_(none)_")
		fmt.Fprintln(w)
	} else {
		for _, s := range relevant {
			fmt.Fprintf(w, "### %s — %s\n", s.ID, s.Title)
			if s.Summary != "" {
				fmt.Fprintf(w, "%s\n", s.Summary)
			}
			if len(s.FilesTouched) > 0 {
				fmt.Fprintf(w, "- Files touched: %s\n", strings.Join(s.FilesTouched, ", "))
			}
			if len(s.InterfacesExposed) > 0 {
				fmt.Fprintf(w, "- Interfaces exposed: %s\n", strings.Join(s.InterfacesExposed, ", "))
			}
			fmt.Fprintln(w)
		}
	}

	// Open TODOs aggregated from completed tasks (same phase if present, else all).
	var open []string
	for _, x := range tasks {
		if x.Status != model.StatusCompleted {
			continue
		}
		if hasPhase && x.PhaseID != t.PhaseID {
			continue
		}
		for _, todo := range x.TodosLeft {
			open = append(open, fmt.Sprintf("(%s) %s", x.ID, todo))
		}
	}
	if len(open) > 0 {
		fmt.Fprintf(w, "## 🚧 Open TODOs\n\n")
		for _, line := range open {
			fmt.Fprintf(w, "- %s\n", line)
		}
		fmt.Fprintln(w)
	}

	// Coming next: sibling todos.
	if hasPhase {
		var next []model.Task
		for _, x := range tasks {
			if x.ID == t.ID {
				continue
			}
			if x.PhaseID == t.PhaseID && x.Status == model.StatusTodo {
				next = append(next, x)
			}
		}
		if len(next) > 0 {
			fmt.Fprintf(w, "## ⏭️ Coming next\n\n")
			for _, n := range next {
				fmt.Fprintf(w, "- %s — %s\n", n.ID, n.Title)
			}
			fmt.Fprintln(w)
		}
	}

	return nil
}

// candidateScore is used internally for blended ranking.
type candidateScore struct {
	task  model.Task
	score float64
}

// selectRelevantTasks blends embedding similarity with heuristic boosts and
// returns up to 5 tasks. Falls back to heuristic-only when embeddings are
// unavailable or OpenAI cannot be reached.
func selectRelevantTasks(l store.Layout, t model.Task, tasks []model.Task) []model.Task {
	depSet := make(map[string]bool, len(t.DependsOn))
	for _, d := range t.DependsOn {
		depSet[d] = true
	}

	// Pool of candidates: every other completed/finished task.
	var pool []model.Task
	for _, x := range tasks {
		if x.ID == t.ID {
			continue
		}
		switch x.Status {
		case model.StatusCompleted, model.StatusPartial, model.StatusBlocked, model.StatusNeedsReview:
			pool = append(pool, x)
		}
	}
	if len(pool) == 0 {
		return nil
	}

	// Try embeddings.
	cosines := computeEmbeddingScores(l, t, pool)

	now := time.Now().UTC()
	scored := make([]candidateScore, 0, len(pool))
	for _, x := range pool {
		s := cosines[x.ID] // 0 when unavailable
		if depSet[x.ID] {
			s += 0.3
		}
		if t.PhaseID != "" && x.PhaseID == t.PhaseID {
			s += 0.2
		}
		s += recencyBoost(x.EndedAt, now)
		scored = append(scored, candidateScore{task: x, score: s})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].task.ID < scored[j].task.ID
	})

	const topN = 5
	limit := topN
	if len(scored) < limit {
		limit = len(scored)
	}
	out := make([]model.Task, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, scored[i].task)
	}
	return out
}

// computeEmbeddingScores returns cosine(query, candidate) keyed by task ID.
// Returns an empty map if embeddings or the OpenAI client are unavailable —
// callers then degrade to heuristic-only ranking.
func computeEmbeddingScores(l store.Layout, t model.Task, pool []model.Task) map[string]float64 {
	out := map[string]float64{}
	cached, err := store.LoadEmbeddings(l)
	if err != nil || len(cached) == 0 {
		return out
	}
	if !llm.HasAPIKey() {
		// We can still compute cosines among cached vectors only if the query
		// task itself is cached. Try that first.
		if rec, ok := store.FindEmbedding(cached, t.ID); ok {
			for _, x := range pool {
				if er, ok := store.FindEmbedding(cached, x.ID); ok {
					out[x.ID] = llm.CosineSimilarity(rec.Embedding, er.Embedding)
				}
			}
		}
		return out
	}

	c, err := llm.NewClient()
	if err != nil {
		return out
	}
	queryText := t.Title
	if t.UserIntent != "" {
		queryText += "\n" + t.UserIntent
	}
	if strings.TrimSpace(queryText) == "" {
		return out
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout())
	defer cancel()
	qvec, err := c.Embed(ctx, queryText)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: query embedding failed: %v\n", err)
		// fall back to using cached query embedding if any
		if rec, ok := store.FindEmbedding(cached, t.ID); ok {
			qvec = rec.Embedding
		} else {
			return out
		}
	}
	for _, x := range pool {
		if er, ok := store.FindEmbedding(cached, x.ID); ok {
			out[x.ID] = llm.CosineSimilarity(qvec, er.Embedding)
		}
	}
	return out
}

// recencyBoost: linear decay from 0.1 (today) to 0 at 30 days.
func recencyBoost(endedAt *time.Time, now time.Time) float64 {
	if endedAt == nil {
		return 0
	}
	days := now.Sub(*endedAt).Hours() / 24.0
	if days < 0 {
		days = 0
	}
	if days >= 30 {
		return 0
	}
	return 0.1 * (1.0 - days/30.0)
}
