package cli

import (
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/nhdms/project-journal/internal/model"
	"github.com/nhdms/project-journal/internal/store"
	"github.com/spf13/cobra"
)

const logFieldMaxBytes = 64 * 1024 // 64 KB hard cap per text field

// capLogField truncates s to at most logFieldMaxBytes bytes, backing up to a
// valid UTF-8 rune boundary so no multi-byte sequence is split.
func capLogField(s string) string {
	if len(s) <= logFieldMaxBytes {
		return s
	}
	cut := s[:logFieldMaxBytes]
	// Back up until we're at a rune start to avoid broken sequences.
	for len(cut) > 0 && !utf8.RuneStart(cut[len(cut)-1]) {
		cut = cut[:len(cut)-1]
	}
	return cut + "…[truncated]"
}

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
				Content:       capLogField(content),
				InputSummary:  capLogField(inputSummary),
				OutputSummary: capLogField(outputSummary),
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
