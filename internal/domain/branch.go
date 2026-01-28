package domain

import "time"

// Branch represents a Git branch from any platform.
// Follows Single Responsibility - only holds branch data.
type Branch struct {
	Name           string
	ProjectID      string
	Repository     string
	LastCommitSHA  string
	LastCommitMsg  string
	LastCommitDate time.Time
	CommitAuthor   string // Author of the last commit (username for GitHub, full name for GitLab)
	AuthorEmail    string // Author email (for matching when username not available)
	IsDefault      bool   // Is this the default branch (main/master)?
	IsProtected    bool   // Is this branch protected?
	WebURL         string // Link to branch in GitLab/GitHub
	Platform       string // CI/CD platform identifier (e.g., "gitlab", "github")
}

// BranchWithPipeline combines a branch with its latest pipeline status.
// This is used in the rendering layer to show branch + pipeline together.
// Follows composition pattern similar to RepositoryWithRuns.
type BranchWithPipeline struct {
	Branch   Branch
	Pipeline *Pipeline // nil if no pipeline exists for this branch
}
