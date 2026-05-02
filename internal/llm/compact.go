package llm

import (
	"fmt"
	"time"

	"github.com/nhdms/project-journal/internal/model"
)

// DefaultMaxChars is the default character budget for a compacted trajectory.
const DefaultMaxChars = 30000

const (
	perFieldCap   = 500
	keepHeadCount = 5
	keepTailCount = 15
)

// dropTools is the set of tool names whose tool_use events are dropped
// during compaction (already low-signal: filesystem reads/searches).
var dropTools = map[string]bool{
	"Read": true,
	"Glob": true,
	"Grep": true,
}

// CompactTrajectory shrinks events for inclusion in an LLM prompt:
//   - drops Read/Glob/Grep tool_use events
//   - truncates Content/InputSummary/OutputSummary to perFieldCap chars
//   - if over maxChars, keeps head+tail and replaces middle with a marker
func CompactTrajectory(events []model.TrajectoryEvent, maxChars int) []model.TrajectoryEvent {
	if maxChars <= 0 {
		maxChars = DefaultMaxChars
	}

	// 1) filter low-signal tool events.
	filtered := make([]model.TrajectoryEvent, 0, len(events))
	for _, ev := range events {
		if ev.Type == model.EventToolUse && dropTools[ev.Tool] {
			continue
		}
		filtered = append(filtered, ev)
	}

	// 2) truncate per-field.
	for i := range filtered {
		filtered[i].Content = truncate(filtered[i].Content, perFieldCap)
		filtered[i].InputSummary = truncate(filtered[i].InputSummary, perFieldCap)
		filtered[i].OutputSummary = truncate(filtered[i].OutputSummary, perFieldCap)
	}

	// 3) check total size; if over budget, fold middle.
	if eventCharLen(filtered) <= maxChars {
		return filtered
	}
	if len(filtered) <= keepHeadCount+keepTailCount {
		return filtered
	}

	head := filtered[:keepHeadCount]
	tail := filtered[len(filtered)-keepTailCount:]
	omitted := len(filtered) - keepHeadCount - keepTailCount

	marker := model.TrajectoryEvent{
		Timestamp: time.Now().UTC(),
		Type:      "summary_marker",
		Content:   fmt.Sprintf("[%d events omitted]", omitted),
	}

	out := make([]model.TrajectoryEvent, 0, keepHeadCount+1+keepTailCount)
	out = append(out, head...)
	out = append(out, marker)
	out = append(out, tail...)
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…[truncated]"
}

func eventCharLen(events []model.TrajectoryEvent) int {
	total := 0
	for _, ev := range events {
		total += len(ev.Content) + len(ev.InputSummary) + len(ev.OutputSummary) + len(ev.Tool) + len(ev.Type)
	}
	return total
}
