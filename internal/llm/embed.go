package llm

import (
	"context"
	"math"
	"strings"

	"github.com/nhduc/project-journal/internal/model"
)

// BuildEmbeddingText composes the canonical text used for a task embedding.
// Format: "<title>\n<summary>\nTags: <tags>".
func BuildEmbeddingText(t model.Task) string {
	var sb strings.Builder
	sb.WriteString(t.Title)
	if t.Summary != "" {
		sb.WriteString("\n")
		sb.WriteString(t.Summary)
	}
	if len(t.Tags) > 0 {
		sb.WriteString("\nTags: ")
		sb.WriteString(strings.Join(t.Tags, ", "))
	}
	return sb.String()
}

// EmbedText is a convenience wrapper around Client.Embed.
func EmbedText(ctx context.Context, c *Client, text string) ([]float32, error) {
	return c.Embed(ctx, text)
}

// CosineSimilarity returns cosine similarity in [-1, 1] (normalised to [0, 1]
// when both vectors are well-formed embeddings, OpenAI returns unit vectors).
// Returns 0 if either vector is empty or lengths mismatch.
func CosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		x := float64(a[i])
		y := float64(b[i])
		dot += x * y
		na += x * x
		nb += y * y
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}
