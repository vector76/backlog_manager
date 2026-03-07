// Package model defines the core data types for the backlog manager.
package model

import "time"

// Project represents a client project.
type Project struct {
	Name      string    `json:"name"`
	Token     string    `json:"token"`
	CreatedAt time.Time `json:"created_at"`
}

// Feature represents a single feature in a project.
// Description content is NOT stored here — it lives in versioned markdown files on disk.
type Feature struct {
	ID               string        `json:"id"`
	Project          string        `json:"project"`
	Name             string        `json:"name"`
	Status           FeatureStatus `json:"status"`
	CurrentIteration int           `json:"current_iteration"`
	// GenerateAfter is the ID of a feature this one depends on (optional).
	GenerateAfter string   `json:"generate_after,omitempty"`
	BeadIDs       []string `json:"bead_ids,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// DialogIteration stores per-round metadata for a feature's dialog.
// Actual content (description, questions, user response) is in versioned files on disk.
type DialogIteration struct {
	Round     int       `json:"round"`
	IsFinal   bool      `json:"is_final"`
	CreatedAt time.Time `json:"created_at"`
}
