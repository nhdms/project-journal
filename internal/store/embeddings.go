package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"
)

// EmbeddingsFile is the basename of the embeddings cache under the journal dir.
const EmbeddingsFile = "embeddings.jsonl"

// EmbeddingRecord is a cached embedding for a single task.
type EmbeddingRecord struct {
	TaskID    string    `json:"task_id"`
	Text      string    `json:"text"`
	Embedding []float32 `json:"embedding"`
	UpdatedAt time.Time `json:"updated_at"`
}

// embeddingsPath returns the absolute path to embeddings.jsonl.
func embeddingsPath(l Layout) string {
	return filepath.Join(l.Dir, EmbeddingsFile)
}

// LoadEmbeddings reads all cached embedding records, deduplicating by TaskID
// (keeping the latest occurrence). Returns nil if file is missing.
func LoadEmbeddings(l Layout) ([]EmbeddingRecord, error) {
	lines, err := readLines(embeddingsPath(l))
	if err != nil {
		return nil, err
	}
	byID := make(map[string]EmbeddingRecord, len(lines))
	order := make([]string, 0, len(lines))
	for i, ln := range lines {
		var rec EmbeddingRecord
		if err := json.Unmarshal(ln, &rec); err != nil {
			return nil, fmt.Errorf("embeddings.jsonl line %d: %w", i+1, err)
		}
		if _, seen := byID[rec.TaskID]; !seen {
			order = append(order, rec.TaskID)
		}
		byID[rec.TaskID] = rec
	}
	out := make([]EmbeddingRecord, 0, len(order))
	for _, id := range order {
		out = append(out, byID[id])
	}
	return out, nil
}

// UpsertEmbedding replaces (or inserts) the record for rec.TaskID and rewrites
// the file atomically.
func UpsertEmbedding(l Layout, rec EmbeddingRecord) error {
	existing, err := LoadEmbeddings(l)
	if err != nil {
		return err
	}
	replaced := false
	for i, r := range existing {
		if r.TaskID == rec.TaskID {
			existing[i] = rec
			replaced = true
			break
		}
	}
	if !replaced {
		existing = append(existing, rec)
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	for _, r := range existing {
		if err := enc.Encode(r); err != nil {
			return err
		}
	}
	return WriteFileAtomic(embeddingsPath(l), buf.Bytes(), 0o644)
}

// HasEmbedding reports whether an embedding exists for taskID.
func HasEmbedding(records []EmbeddingRecord, taskID string) bool {
	for _, r := range records {
		if r.TaskID == taskID {
			return true
		}
	}
	return false
}

// FindEmbedding returns the record for taskID, or (zero, false).
func FindEmbedding(records []EmbeddingRecord, taskID string) (EmbeddingRecord, bool) {
	for _, r := range records {
		if r.TaskID == taskID {
			return r, true
		}
	}
	return EmbeddingRecord{}, false
}
