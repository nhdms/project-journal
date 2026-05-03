package cli

import (
	"testing"

	"github.com/nhdms/project-journal/internal/model"
)

func TestValidateEditedTask(t *testing.T) {
	phases := []model.Phase{
		{ID: "P1", Title: "Phase one"},
	}

	orig := model.Task{
		ID:     "T1",
		Status: model.StatusTodo,
		Title:  "Original task",
	}

	tests := []struct {
		name    string
		edited  model.Task
		wantErr string
	}{
		{
			name: "valid no change",
			edited: model.Task{
				ID:     "T1",
				Status: model.StatusInProgress,
				Title:  "Original task",
			},
		},
		{
			name: "valid with existing phase",
			edited: model.Task{
				ID:      "T1",
				Status:  model.StatusCompleted,
				Title:   "Original task",
				PhaseID: "P1",
			},
		},
		{
			name: "invalid: ID changed",
			edited: model.Task{
				ID:     "T2",
				Status: model.StatusTodo,
				Title:  "Original task",
			},
			wantErr: "ID cannot be changed",
		},
		{
			name: "invalid: bad status",
			edited: model.Task{
				ID:     "T1",
				Status: "done",
				Title:  "Original task",
			},
			wantErr: "invalid status",
		},
		{
			name: "invalid: non-existent phase",
			edited: model.Task{
				ID:      "T1",
				Status:  model.StatusTodo,
				Title:   "Original task",
				PhaseID: "GHOST",
			},
			wantErr: "does not exist",
		},
		{
			name: "invalid: self-dependency",
			edited: model.Task{
				ID:        "T1",
				Status:    model.StatusTodo,
				Title:     "Original task",
				DependsOn: []string{"T1"},
			},
			wantErr: "cannot depend on itself",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateEditedTask(orig, tc.edited, phases)
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tc.wantErr)
				} else if !contains(err.Error(), tc.wantErr) {
					t.Errorf("expected error containing %q, got %q", tc.wantErr, err.Error())
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}
