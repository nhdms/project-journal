package cli

import (
	"fmt"
	"time"

	"github.com/nhdms/project-journal/internal/model"
	"github.com/nhdms/project-journal/internal/store"
	"github.com/spf13/cobra"
)

// NewLogCmd creates `pj log`.
func NewLogCmd() *cobra.Command {
	var (
		eventType     string
		tool          string
		content       string
		inputSummary  string
		outputSummary string
		firstOnly     bool
	)
	cmd := &cobra.Command{
		Use:   "log <id>",
		Short: "Append a trajectory event to a task's session log",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			if eventType == "" {
				return fmt.Errorf("--type is required")
			}
			switch eventType {
			case model.EventUserPrompt, model.EventToolUse, model.EventAssistantText, model.EventCompactMarker:
			default:
				return fmt.Errorf("invalid --type %q (expected user_prompt|tool_use|assistant_text|compact_marker)", eventType)
			}
			l, err := resolveLayout()
			if err != nil {
				return err
			}
			if firstOnly {
				existing, err := store.LoadTrajectory(l, id)
				if err != nil {
					return err
				}
				for _, ev := range existing {
					if ev.Type == eventType {
						return nil
					}
				}
			}
			ev := model.TrajectoryEvent{
				Timestamp:     time.Now().UTC(),
				Type:          eventType,
				Tool:          tool,
				Content:       content,
				InputSummary:  inputSummary,
				OutputSummary: outputSummary,
			}
			return store.AppendTrajectory(l, id, ev)
		},
	}
	cmd.Flags().StringVar(&eventType, "type", "", "Event type (user_prompt|tool_use|assistant_text|compact_marker)")
	cmd.Flags().StringVar(&tool, "tool", "", "Tool name (for tool_use events)")
	cmd.Flags().StringVar(&content, "content", "", "Raw event content")
	cmd.Flags().StringVar(&inputSummary, "input-summary", "", "Summary of input")
	cmd.Flags().StringVar(&outputSummary, "output-summary", "", "Summary of output")
	cmd.Flags().BoolVar(&firstOnly, "first-only", false, "Skip if an event of the same type already exists")
	return cmd
}
