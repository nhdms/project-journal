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

// NewReindexCmd creates `pj reindex`. Backfills embeddings for all
// finished tasks (any non-todo, non-in_progress status). With --index-only
// it rebuilds just the derived DuckDB index from JSONL (fast, no LLM call).
func NewReindexCmd() *cobra.Command {
	var force bool
	var indexOnly bool
	var check bool
	cmd := &cobra.Command{
		Use:   "reindex",
		Short: "(Re)build embeddings for finished tasks; --index-only rebuilds derived index from JSONL; --check reports drift",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			l, err := resolveLayout()
			if err != nil {
				return err
			}
			if check {
				r, err := store.IndexDrift(l)
				if err != nil {
					return fmt.Errorf("drift: %w", err)
				}
				fmt.Printf("Tasks      : JSONL=%d  Index=%d\n", r.TasksJSONL, r.TasksIndex)
				fmt.Printf("Phases     : JSONL=%d  Index=%d\n", r.PhasesJSONL, r.PhasesIndex)
				fmt.Printf("Embeddings : JSONL=%d  Index=%d\n", r.EmbeddingsJSONL, r.EmbeddingsIndex)
				if r.Drift {
					fmt.Println("\nDrift detected. Run `pj reindex --index-only` to rebuild the derived index from JSONL.")
					os.Exit(3) // distinct exit code so scripts can detect drift
				}
				fmt.Println("\nNo drift.")
				return nil
			}
			if indexOnly {
				if err := store.RebuildIndex(l); err != nil {
					return fmt.Errorf("rebuild index: %w", err)
				}
				fmt.Println("Derived index rebuilt from JSONL.")
				return nil
			}
			if !llm.HasAPIKey() {
				return fmt.Errorf("OPENAI_API_KEY not set")
			}
			c, err := llm.NewClient()
			if err != nil {
				return err
			}

			tasks, err := store.LoadTasks(l)
			if err != nil {
				return err
			}
			cached, err := store.LoadEmbeddings(l)
			if err != nil {
				return err
			}

			var targets []model.Task
			for _, t := range tasks {
				switch t.Status {
				case model.StatusCompleted, model.StatusPartial, model.StatusBlocked, model.StatusNeedsReview:
					targets = append(targets, t)
				}
			}
			if len(targets) == 0 {
				fmt.Fprintln(os.Stderr, "no finished tasks to embed")
				return nil
			}

			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			n := len(targets)
			done, skipped, failed := 0, 0, 0
			for i, t := range targets {
				text := llm.BuildEmbeddingText(t)
				if strings.TrimSpace(text) == "" {
					fmt.Fprintf(os.Stderr, "[%d/%d] %s: empty embedding text — skip\n", i+1, n, t.ID)
					skipped++
					continue
				}
				if !force {
					if rec, ok := store.FindEmbedding(cached, t.ID); ok && rec.Text == text {
						fmt.Fprintf(os.Stderr, "[%d/%d] %s: cached — skip\n", i+1, n, t.ID)
						skipped++
						continue
					}
				}
				fmt.Fprintf(os.Stderr, "[%d/%d] %s: embedding…\n", i+1, n, t.ID)
				vec, err := c.Embed(ctx, text)
				if err != nil {
					fmt.Fprintf(os.Stderr, "  error: %v\n", err)
					failed++
					continue
				}
				rec := store.EmbeddingRecord{
					TaskID:    t.ID,
					Text:      text,
					Embedding: vec,
					UpdatedAt: time.Now().UTC(),
				}
				if err := store.UpsertEmbedding(l, rec); err != nil {
					fmt.Fprintf(os.Stderr, "  persist error: %v\n", err)
					failed++
					continue
				}
				done++
			}
			fmt.Printf("Reindex done: %d embedded, %d skipped, %d failed (of %d)\n", done, skipped, failed, n)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "Re-embed even if a cached embedding exists with matching text")
	cmd.Flags().BoolVar(&indexOnly, "index-only", false, "Rebuild the derived index from JSONL without calling the embedding API")
	cmd.Flags().BoolVar(&check, "check", false, "Report row-count drift between JSONL source-of-truth and derived index (exit 3 if drift)")
	return cmd
}
