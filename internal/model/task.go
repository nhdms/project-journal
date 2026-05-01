package model

import "time"

// Task statuses.
const (
	StatusTodo        = "todo"
	StatusInProgress  = "in_progress"
	StatusCompleted   = "completed"
	StatusPartial     = "partial"
	StatusBlocked     = "blocked"
	StatusNeedsReview = "needs_review"
)

// Task represents a single unit of work in the journal.
type Task struct {
	ID                string     `json:"id"`
	PhaseID           string     `json:"phase_id,omitempty"`
	DependsOn         []string   `json:"depends_on,omitempty"`
	Title             string     `json:"title"`
	UserIntent        string     `json:"user_intent,omitempty"`
	Summary           string     `json:"summary,omitempty"`
	FilesTouched      []string   `json:"files_touched,omitempty"`
	KeyDecisions      []string   `json:"key_decisions,omitempty"`
	BlockersResolved  []string   `json:"blockers_resolved,omitempty"`
	TodosLeft         []string   `json:"todos_left,omitempty"`
	InterfacesExposed []string   `json:"interfaces_exposed,omitempty"`
	Tags              []string   `json:"tags,omitempty"`
	Status            string     `json:"status"`
	SessionID         string     `json:"session_id,omitempty"`
	StartedAt         *time.Time `json:"started_at,omitempty"`
	EndedAt           *time.Time `json:"ended_at,omitempty"`
}
