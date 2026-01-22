package domain

import "time"

// MergeRequest represents a merge request (GitLab) or pull request (GitHub).
type MergeRequest struct {
	ID          string
	Number      int
	Title       string
	Description string
	State       string // "opened", "closed", "merged"
	IsDraft     bool   // true if MR/PR is marked as draft/WIP
	SourceBranch string
	TargetBranch string
	Author      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	WebURL      string
	ProjectID   string
	Repository  string
}
