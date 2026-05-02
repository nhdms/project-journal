package cli

import (
	"fmt"
	"sort"

	"github.com/nhdms/project-journal/internal/model"
	"github.com/nhdms/project-journal/internal/store"
	"github.com/spf13/cobra"
)

// NewTreeCmd creates `pj tree`.
func NewTreeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tree",
		Short: "Print phases and tasks as an ASCII tree",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			l, err := resolveLayout()
			if err != nil {
				return err
			}
			phases, err := store.LoadPhases(l)
			if err != nil {
				return err
			}
			tasks, err := store.LoadTasks(l)
			if err != nil {
				return err
			}
			renderTree(phases, tasks)
			return nil
		},
	}
}

func statusIcon(status string) string {
	switch status {
	case model.StatusCompleted:
		return "✅"
	case model.StatusInProgress:
		return "🚧"
	case model.StatusTodo:
		return "⬜"
	case model.StatusPartial:
		return "⚠️"
	case model.StatusBlocked:
		return "🚫"
	case model.StatusNeedsReview:
		return "📝"
	default:
		return "•"
	}
}

func renderTree(phases []model.Phase, tasks []model.Task) {
	fmt.Println("📂 Project Journal")

	// Group tasks by phase id; "" = unassigned.
	byPhase := map[string][]model.Task{}
	for _, t := range tasks {
		byPhase[t.PhaseID] = append(byPhase[t.PhaseID], t)
	}

	phaseList := make([]model.Phase, len(phases))
	copy(phaseList, phases)
	sort.SliceStable(phaseList, func(i, j int) bool {
		return phaseList[i].CreatedAt.Before(phaseList[j].CreatedAt)
	})

	hasUnassigned := len(byPhase[""]) > 0
	totalGroups := len(phaseList)
	if hasUnassigned {
		totalGroups++
	}

	rendered := 0
	for _, p := range phaseList {
		rendered++
		isLast := rendered == totalGroups
		branch, indent := branchPrefix(isLast)
		group := byPhase[p.ID]
		done := 0
		for _, t := range group {
			if t.Status == model.StatusCompleted {
				done++
			}
		}
		fmt.Printf("%s🎯 %s — %s (%d/%d done)\n", branch, p.ID, p.Title, done, len(group))
		printGroup(group, indent)
	}
	if hasUnassigned {
		isLast := true
		branch, indent := branchPrefix(isLast)
		fmt.Printf("%s📋 Unassigned\n", branch)
		printGroup(byPhase[""], indent)
	}
}

func branchPrefix(isLast bool) (branch, indent string) {
	if isLast {
		return "└── ", "    "
	}
	return "├── ", "│   "
}

func printGroup(group []model.Task, indent string) {
	g := make([]model.Task, len(group))
	copy(g, group)
	sort.SliceStable(g, func(i, j int) bool { return g[i].ID < g[j].ID })
	for i, t := range g {
		isLast := i == len(g)-1
		var connector string
		if isLast {
			connector = "└── "
		} else {
			connector = "├── "
		}
		extra := ""
		if t.Status != model.StatusCompleted && t.Status != model.StatusTodo {
			extra = fmt.Sprintf(" (%s)", t.Status)
		}
		fmt.Printf("%s%s%s %s — %s%s\n", indent, connector, statusIcon(t.Status), t.ID, t.Title, extra)
	}
}
