package model

import "time"

// Trajectory event types.
const (
	EventUserPrompt    = "user_prompt"
	EventToolUse       = "tool_use"
	EventAssistantText = "assistant_text"
	EventCompactMarker = "compact_marker"
)

// TrajectoryEvent is a single recorded event in a task's session log.
type TrajectoryEvent struct {
	Timestamp     time.Time `json:"ts"`
	Type          string    `json:"type"`
	Tool          string    `json:"tool,omitempty"`
	Content       string    `json:"content,omitempty"`
	InputSummary  string    `json:"input_summary,omitempty"`
	OutputSummary string    `json:"output_summary,omitempty"`
}
