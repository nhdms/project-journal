package store

import (
	"github.com/nhdms/project-journal/internal/model"
	"github.com/nhdms/project-journal/internal/store/index"
)

// mirrorTask upserts t into the derived index. JSONL is the source of
// truth; index errors are non-fatal (the index rebuilds itself from JSONL
// on next `pj reindex --full`).
func mirrorTask(dir string, t model.Task) error {
	idx, err := index.For(dir)
	if err != nil {
		return nil
	}
	_ = idx.UpsertTask(t)
	return nil
}

func mirrorPhase(dir string, p model.Phase) error {
	idx, err := index.For(dir)
	if err != nil {
		return nil
	}
	_ = idx.UpsertPhase(p)
	return nil
}

func mirrorEmbedding(dir string, rec EmbeddingRecord) error {
	idx, err := index.For(dir)
	if err != nil {
		return nil
	}
	_ = idx.UpsertEmbedding(index.EmbeddingRecord{
		TaskID:    rec.TaskID,
		Text:      rec.Text,
		Embedding: rec.Embedding,
		UpdatedAt: rec.UpdatedAt,
	})
	return nil
}

func mirrorTrajectory(dir, taskID string, ev model.TrajectoryEvent) error {
	idx, err := index.For(dir)
	if err != nil {
		return nil
	}
	_ = idx.AppendTrajectory(taskID, ev)
	return nil
}

// RebuildIndex wipes the derived index and re-populates it from the JSONL
// data under l. Safe to call concurrently with reads but should not race
// with writes — callers should hold the journal lock or quiesce writers.
func RebuildIndex(l Layout) error {
	tasks, err := LoadTasks(l)
	if err != nil {
		return err
	}
	phases, err := LoadPhases(l)
	if err != nil {
		return err
	}
	embs, err := LoadEmbeddings(l)
	if err != nil {
		return err
	}
	idx, err := index.For(l.Dir)
	if err != nil {
		return err
	}
	idxEmbs := make([]index.EmbeddingRecord, 0, len(embs))
	for _, e := range embs {
		idxEmbs = append(idxEmbs, index.EmbeddingRecord{
			TaskID:    e.TaskID,
			Text:      e.Text,
			Embedding: e.Embedding,
			UpdatedAt: e.UpdatedAt,
		})
	}
	return idx.Rebuild(tasks, phases, idxEmbs)
}

// IndexEnabled reports whether a real (non-noop) derived index is compiled
// in. Callers can use this to short-circuit expensive load paths.
func IndexEnabled() bool { return index.Enabled() }

// SearchSimilar exposes index-backed cosine similarity search to callers
// that prefer it over scanning JSONL embeddings. Returns nil if no real
// index is compiled in or no embeddings exist.
func SearchSimilar(l Layout, query []float32, topK int, allowedStatuses []string) ([]index.SimilarTask, error) {
	idx, err := index.For(l.Dir)
	if err != nil {
		return nil, err
	}
	return idx.SearchSimilar(query, topK, allowedStatuses)
}
