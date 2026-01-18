package domain

import "time"

// Issue represents an issue from GitLab or GitHub.
type Issue struct {
	ID          string
	Number      int
	Title       string
	Description string
	State       string // "opened", "closed"
	Labels      []string
	Author      string
	Assignee    string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	WebURL      string
	ProjectID   string
	Repository  string
}
