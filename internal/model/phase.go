package model

import "time"

// Phase represents a logical grouping of tasks.
type Phase struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
}
