package domain

import "time"

// Pipeline represents a CI/CD pipeline from any platform.
// This is a domain model (part of business logic).
type Pipeline struct {
	ID         string
	ProjectID  string
	Repository string
	Branch     string
	Status     Status
	CreatedAt  time.Time
	UpdatedAt  time.Time
	WebURL     string
	Builds     []Build
}

// Build represents a single job/build within a pipeline.
type Build struct {
	ID        string
	Name      string
	Status    Status
	Stage     string
	Duration  time.Duration
	StartedAt time.Time
	WebURL    string
}

// Status represents the state of a pipeline or build.
type Status string

const (
	StatusPending  Status = "pending"
	StatusRunning  Status = "running"
	StatusSuccess  Status = "success"
	StatusFailed   Status = "failed"
	StatusCanceled Status = "canceled"
	StatusSkipped  Status = "skipped"
)

// IsTerminal returns true if the status is in a final state.
func (s Status) IsTerminal() bool {
	return s == StatusSuccess || s == StatusFailed || s == StatusCanceled
}

// Project represents a code repository project.
type Project struct {
	ID       string
	Name     string
	WebURL   string
	Platform string // "gitlab" or "github"
}
