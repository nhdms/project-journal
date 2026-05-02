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
// finished tasks (any non-todo, non-in_progress status).
func NewReindexCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "reindex",
		Short: "(Re)build embeddings for all finished tasks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			l, err := resolveLayout()
			if err != nil {
				return err
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
	return cmd
}
