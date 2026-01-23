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
	Duration   time.Duration
	WebURL     string
	Builds     []Build

	// Optional workflow fields for GitHub Actions (nil for GitLab)
	WorkflowName *string
	WorkflowID   *string
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
	ID            string
	Name          string
	WebURL        string
	Platform      string // CI/CD platform identifier (e.g., "gitlab", "github")
	IsFork        bool   // true if this is a forked repository
	DefaultBranch string // name of the default branch (e.g., "main", "master")
	Owner         *ProjectOwner
	Namespace     *ProjectNamespace
	Permissions   *ProjectPermissions
	LastActivity  time.Time
}

// ProjectOwner represents the owner of a project.
type ProjectOwner struct {
	Username string
	Name     string
	Type     string // "user" or "organization"
}

// ProjectNamespace represents the namespace/group a project belongs to.
type ProjectNamespace struct {
	ID   string
	Path string
	Kind string // "user" or "group"
}

// ProjectPermissions represents user's access level to a project.
type ProjectPermissions struct {
	AccessLevel int  // GitLab: 10-50 (10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner)
	Admin       bool // GitHub: admin permission (owner level)
	Push        bool // GitHub: push permission
	Pull        bool // GitHub: pull permission
}
