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
	// Iterations holds per-round dialog metadata (e.g. is_final flag).
	Iterations []DialogIteration `json:"iterations,omitempty"`
	// GenerateAfter is the ID of a feature this one depends on (optional).
	GenerateAfter string    `json:"generate_after,omitempty"`
	BeadIDs       []string  `json:"bead_ids,omitempty"`
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

// IterationContent holds the content of a single dialog iteration (round >= 1).
type IterationContent struct {
	Round       int    `json:"round"`
	Description string `json:"description,omitempty"`
	Questions   string `json:"questions,omitempty"`
	Response    string `json:"response,omitempty"`
}

// FeatureDetail includes a Feature plus its full dialog history read from disk.
type FeatureDetail struct {
	Feature
	InitialDescription string             `json:"initial_description"`
	Iterations         []IterationContent `json:"iterations,omitempty"`
}
