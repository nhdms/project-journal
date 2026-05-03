//go:build !pj_duckdb

package index

import "github.com/nhdms/project-journal/internal/model"

// noopIndex is the default implementation when DuckDB is not compiled in.
// All write methods silently succeed; SearchSimilar returns empty.
// Callers (notably context.go) detect the empty result and fall back to
// the JSONL-based cosine path.
type noopIndex struct{}

func newNoop() Index { return noopIndex{} }

// open returns a noop index. Always succeeds.
func open(dir string) (Index, error) { return newNoop(), nil }

// Enabled reports whether a real (non-noop) index is compiled in.
func Enabled() bool { return false }

func (noopIndex) UpsertTask(model.Task) error                      { return nil }
func (noopIndex) UpsertPhase(model.Phase) error                    { return nil }
func (noopIndex) UpsertEmbedding(EmbeddingRecord) error            { return nil }
func (noopIndex) AppendTrajectory(string, model.TrajectoryEvent) error {
	return nil
}
func (noopIndex) SearchSimilar([]float32, int, []string) ([]SimilarTask, error) {
	return nil, nil
}
func (noopIndex) Rebuild([]model.Task, []model.Phase, []EmbeddingRecord) error {
	return nil
}
func (noopIndex) Close() error                           { return nil }
func (noopIndex) TableCount(string) (int, error)         { return 0, nil }
func (noopIndex) GetTaskStatus(string) (string, error)   { return "", nil }
