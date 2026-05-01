package llm

import (
	"fmt"
	"strings"

	"github.com/nhduc/project-journal/internal/model"
)

// InduceSystemPrompt instructs the model to extract a structured summary
// from a coding-task trajectory.
const InduceSystemPrompt = `You are a software engineering journal assistant. Read a coding task's trajectory (user prompt + tool calls + outputs) and extract a structured summary that future tasks can use as context.

Be concrete and specific. Cite file paths and function names. Capture decisions made (with reasoning if visible) and TODOs the agent left behind. Identify interfaces (APIs, functions, schemas) the task exposed for downstream tasks.

Output strict JSON matching this schema:
{
  "summary": "2-4 sentences describing what was accomplished",
  "files_touched": ["path/to/file"],
  "key_decisions": ["one decision per line, include reasoning"],
  "blockers_resolved": ["blocker and how solved"],
  "todos_left": ["explicit TODO or partial work"],
  "interfaces_exposed": ["API endpoint, function signature, or schema others can use"],
  "tags": ["short kebab-case tags: language, framework, domain"]
}

If a field has no content, return [] (or "" for summary). Do not invent details not in the trajectory.`

// AutoevalSystemPrompt instructs the model to judge task completion status
// independently of any induced summary.
const AutoevalSystemPrompt = `You are an independent judge evaluating whether a coding task was completed successfully.

You will see the user's intent and the agent's trajectory. Decide:
- "completed": task fully done, tests/verification passed if applicable
- "partial": meaningful progress but not finished (TODOs left, partial implementation)
- "blocked": agent hit a blocker that prevented progress
- "needs_review": cannot determine from trajectory alone

Output strict JSON:
{"status": "...", "reason": "1-2 sentences", "confidence": 0.0-1.0}

Be conservative — if unsure, prefer "needs_review" or lower confidence.`

// FormatTrajectory renders trajectory events as a readable text block for use
// in user prompts.
func FormatTrajectory(events []model.TrajectoryEvent) string {
	var sb strings.Builder
	for _, ev := range events {
		switch ev.Type {
		case model.EventUserPrompt:
			sb.WriteString("[user_prompt]\n")
			sb.WriteString(ev.Content)
			sb.WriteString("\n\n")
		case model.EventAssistantText:
			sb.WriteString("[assistant_text]\n")
			sb.WriteString(ev.Content)
			sb.WriteString("\n\n")
		case model.EventToolUse:
			fmt.Fprintf(&sb, "[tool_use:%s]\n", ev.Tool)
			if ev.InputSummary != "" {
				fmt.Fprintf(&sb, "input: %s\n", ev.InputSummary)
			}
			if ev.OutputSummary != "" {
				fmt.Fprintf(&sb, "output: %s\n", ev.OutputSummary)
			}
			sb.WriteString("\n")
		case model.EventCompactMarker:
			sb.WriteString("[compact_marker]\n")
			if ev.Content != "" {
				sb.WriteString(ev.Content)
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
		default:
			fmt.Fprintf(&sb, "[%s]\n", ev.Type)
			if ev.Content != "" {
				sb.WriteString(ev.Content)
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}
