package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/nhduc/project-journal/internal/model"
)

// AutoevalResult is the LLM's independent judgment of task completion.
type AutoevalResult struct {
	Status     string  `json:"status"`
	Reason     string  `json:"reason"`
	Confidence float64 `json:"confidence"`
}

// Autoeval calls the LLM independently of Induce to classify task status.
func Autoeval(ctx context.Context, c *Client, task model.Task, events []model.TrajectoryEvent) (AutoevalResult, error) {
	compact := CompactTrajectory(events, DefaultMaxChars)
	user := buildAutoevalPrompt(task, compact)
	var out AutoevalResult
	if err := c.ChatJSON(ctx, AutoevalSystemPrompt, user, &out); err != nil {
		return AutoevalResult{}, err
	}
	return out, nil
}

func buildAutoevalPrompt(task model.Task, events []model.TrajectoryEvent) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Task ID: %s\n", task.ID)
	fmt.Fprintf(&sb, "Title: %s\n", task.Title)
	if task.UserIntent != "" {
		fmt.Fprintf(&sb, "User intent: %s\n", task.UserIntent)
	} else {
		sb.WriteString("User intent: (not provided)\n")
	}
	sb.WriteString("\nTrajectory:\n")
	sb.WriteString(FormatTrajectory(events))
	sb.WriteString("\n")
	return sb.String()
}
