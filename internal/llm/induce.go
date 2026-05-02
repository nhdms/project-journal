package llm

import (
	"context"
	"fmt"
	"strings"

	"github.com/nhdms/project-journal/internal/model"
)

// InduceProposal is the structured summary the LLM extracts from a trajectory.
type InduceProposal struct {
	Summary           string   `json:"summary"`
	FilesTouched      []string `json:"files_touched"`
	KeyDecisions      []string `json:"key_decisions"`
	BlockersResolved  []string `json:"blockers_resolved"`
	TodosLeft         []string `json:"todos_left"`
	InterfacesExposed []string `json:"interfaces_exposed"`
	Tags              []string `json:"tags"`
}

// Induce calls the LLM to produce a structured proposal from a task and its
// trajectory. The trajectory is compacted before being sent.
func Induce(ctx context.Context, c *Client, task model.Task, phaseTitle string, events []model.TrajectoryEvent) (InduceProposal, error) {
	compact := CompactTrajectory(events, DefaultMaxChars)
	user := buildInducePrompt(task, phaseTitle, compact)
	var out InduceProposal
	if err := c.ChatJSON(ctx, InduceSystemPrompt, user, &out); err != nil {
		return InduceProposal{}, err
	}
	return out, nil
}

func buildInducePrompt(task model.Task, phaseTitle string, events []model.TrajectoryEvent) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Task ID: %s\n", task.ID)
	fmt.Fprintf(&sb, "Title: %s\n", task.Title)
	if phaseTitle == "" {
		sb.WriteString("Phase: none\n")
	} else {
		fmt.Fprintf(&sb, "Phase: %s\n", phaseTitle)
	}
	if task.UserIntent != "" {
		fmt.Fprintf(&sb, "User intent: %s\n", task.UserIntent)
	}
	sb.WriteString("\nTrajectory:\n")
	sb.WriteString(FormatTrajectory(events))
	sb.WriteString("\n")
	return sb.String()
}
