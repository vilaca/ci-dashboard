package api

import (
	"context"
	"time"

	"github.com/vilaca/ci-dashboard/internal/domain"
)

// Client defines the interface for CI/CD platform clients.
// This follows Interface Segregation Principle - small, focused interface.
// Allows dependency inversion - consumers depend on this interface, not concrete implementations.
type Client interface {
	// GetProjects returns all projects accessible by the configured credentials.
	GetProjects(ctx context.Context) ([]domain.Project, error)

	// GetProjectsPage returns a single page of projects.
	// Returns: projects for this page, whether there's a next page, and error.
	GetProjectsPage(ctx context.Context, page int) ([]domain.Project, bool, error)

	// GetProjectCount returns the total number of projects accessible by the configured credentials.
	GetProjectCount(ctx context.Context) (int, error)

	// GetLatestPipeline returns the most recent pipeline for a given project and branch.
	GetLatestPipeline(ctx context.Context, projectID, branch string) (*domain.Pipeline, error)

	// GetPipelines returns recent pipelines for a given project.
	GetPipelines(ctx context.Context, projectID string, limit int) ([]domain.Pipeline, error)

	// GetBranches returns branches for a given project.
	// limit controls max branches to return (0 = all branches).
	GetBranches(ctx context.Context, projectID string, limit int) ([]domain.Branch, error)

	// GetBranch retrieves a single branch by name.
	GetBranch(ctx context.Context, projectID, branchName string) (*domain.Branch, error)
}

// WorkflowClient extends Client with workflow-specific operations.
// This is optional - only GitHub implements it.
// Follows Interface Segregation Principle.
type WorkflowClient interface {
	Client

	// GetWorkflowRuns returns runs for a specific workflow.
	GetWorkflowRuns(ctx context.Context, projectID string, workflowID string, limit int) ([]domain.Pipeline, error)
}

// ExtendedClient extends Client with merge request and issue operations.
// Both GitLab and GitHub implement this.
// Follows Interface Segregation Principle.
type ExtendedClient interface {
	Client

	// GetMergeRequests returns open merge requests (PRs) for a project.
	GetMergeRequests(ctx context.Context, projectID string) ([]domain.MergeRequest, error)

	// GetIssues returns open issues for a project.
	GetIssues(ctx context.Context, projectID string) ([]domain.Issue, error)
}

// UserClient extends Client with user profile operations.
// Follows Interface Segregation Principle.
type UserClient interface {
	Client

	// GetCurrentUser returns the profile of the authenticated user.
	GetCurrentUser(ctx context.Context) (*domain.UserProfile, error)
}

// EventsClient extends Client with event polling operations.
// Follows Interface Segregation Principle.
type EventsClient interface {
	Client

	// GetEvents returns events for a project since a specific time.
	GetEvents(ctx context.Context, projectID string, since time.Time) ([]domain.Event, error)
}

// ClientConfig holds common configuration for API clients.
type ClientConfig struct {
	BaseURL string
	Token   string
}
