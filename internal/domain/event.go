package domain

import "time"

// Event represents a repository activity event (push, MR, pipeline, etc.)
// Used for detecting changes and triggering cache invalidation.
type Event struct {
	ID         string    // Event ID
	ProjectID  string    // Project/repository identifier
	Type       string    // Event type: "push", "merge_request", "pipeline", "issue", etc.
	TargetType string    // What the event targets: "branch", "merge_request", "pipeline", etc.
	TargetID   string    // ID of the target (branch name, MR ID, etc.)
	CreatedAt  time.Time // When the event occurred
	Author     string    // Username who triggered the event
	ActionName string    // Specific action: "created", "updated", "closed", "merged", etc.
}
