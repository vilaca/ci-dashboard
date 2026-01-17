package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/vilaca/ci-dashboard/internal/api"
	"github.com/vilaca/ci-dashboard/internal/domain"
)

// Client implements api.Client for GitHub Actions.
// Follows Single Responsibility Principle - only handles GitHub API communication.
type Client struct {
	baseURL    string
	token      string
	httpClient HTTPClient
}

// HTTPClient interface for HTTP operations (allows mocking in tests).
// Follows Interface Segregation Principle.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// NewClient creates a new GitHub Actions client.
// Uses dependency injection for HTTPClient (IoC).
func NewClient(config api.ClientConfig, httpClient HTTPClient) *Client {
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.github.com"
	}

	return &Client{
		baseURL:    baseURL,
		token:      config.Token,
		httpClient: httpClient,
	}
}

// GetProjects retrieves repositories with Actions enabled.
func (c *Client) GetProjects(ctx context.Context) ([]domain.Project, error) {
	url := fmt.Sprintf("%s/user/repos?per_page=100", c.baseURL)

	var ghRepos []githubRepository
	if err := c.doRequest(ctx, url, &ghRepos); err != nil {
		return nil, fmt.Errorf("failed to get repositories: %w", err)
	}

	return c.convertProjects(ghRepos), nil
}

// GetLatestPipeline retrieves the most recent workflow run for a repository and branch.
func (c *Client) GetLatestPipeline(ctx context.Context, projectID, branch string) (*domain.Pipeline, error) {
	// projectID format: "owner/repo"
	url := fmt.Sprintf("%s/repos/%s/actions/runs?branch=%s&per_page=1", c.baseURL, projectID, branch)

	var response githubWorkflowRunsResponse
	if err := c.doRequest(ctx, url, &response); err != nil {
		return nil, fmt.Errorf("failed to get workflow runs: %w", err)
	}

	if len(response.WorkflowRuns) == 0 {
		return nil, nil
	}

	return c.convertPipeline(response.WorkflowRuns[0], projectID), nil
}

// GetPipelines retrieves recent workflow runs for a repository.
func (c *Client) GetPipelines(ctx context.Context, projectID string, limit int) ([]domain.Pipeline, error) {
	url := fmt.Sprintf("%s/repos/%s/actions/runs?per_page=%d", c.baseURL, projectID, limit)

	var response githubWorkflowRunsResponse
	if err := c.doRequest(ctx, url, &response); err != nil {
		return nil, fmt.Errorf("failed to get workflow runs: %w", err)
	}

	pipelines := make([]domain.Pipeline, len(response.WorkflowRuns))
	for i, run := range response.WorkflowRuns {
		pipelines[i] = *c.convertPipeline(run, projectID)
	}

	return pipelines, nil
}

// GetWorkflowRuns retrieves runs for a specific workflow.
func (c *Client) GetWorkflowRuns(ctx context.Context, projectID string, workflowID string, limit int) ([]domain.Pipeline, error) {
	url := fmt.Sprintf("%s/repos/%s/actions/workflows/%s/runs?per_page=%d",
		c.baseURL, projectID, workflowID, limit)

	var response githubWorkflowRunsResponse
	if err := c.doRequest(ctx, url, &response); err != nil {
		return nil, fmt.Errorf("failed to get workflow runs: %w", err)
	}

	pipelines := make([]domain.Pipeline, len(response.WorkflowRuns))
	for i, run := range response.WorkflowRuns {
		pipelines[i] = *c.convertPipeline(run, projectID)
	}

	return pipelines, nil
}

// doRequest performs an HTTP request to GitHub API.
// Follows Single Level of Abstraction Principle (SLAP).
func (c *Client) doRequest(ctx context.Context, url string, result interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	return nil
}

// convertProjects converts GitHub repositories to domain models.
func (c *Client) convertProjects(ghRepos []githubRepository) []domain.Project {
	projects := make([]domain.Project, 0, len(ghRepos))
	for _, repo := range ghRepos {
		projects = append(projects, domain.Project{
			ID:       repo.FullName,
			Name:     repo.Name,
			WebURL:   repo.HTMLURL,
			Platform: "github",
		})
	}
	return projects
}

// convertPipeline converts a GitHub workflow run to domain model.
func (c *Client) convertPipeline(run githubWorkflowRun, projectID string) *domain.Pipeline {
	// Extract repository name from projectID (owner/repo)
	parts := strings.Split(projectID, "/")
	repository := ""
	if len(parts) == 2 {
		repository = parts[1]
	}

	// Convert workflow info to pointers for optional fields
	workflowName := run.Name
	workflowID := fmt.Sprintf("%d", run.WorkflowID)

	// Calculate duration
	duration := run.UpdatedAt.Sub(run.CreatedAt)

	return &domain.Pipeline{
		ID:           fmt.Sprintf("%d", run.ID),
		ProjectID:    projectID,
		Repository:   repository,
		Branch:       run.HeadBranch,
		Status:       convertStatus(run.Status, run.Conclusion),
		CreatedAt:    run.CreatedAt,
		UpdatedAt:    run.UpdatedAt,
		Duration:     duration,
		WebURL:       run.HTMLURL,
		WorkflowName: &workflowName,
		WorkflowID:   &workflowID,
	}
}

// convertStatus converts GitHub status and conclusion to domain status.
func convertStatus(status, conclusion string) domain.Status {
	// GitHub uses both 'status' (queued, in_progress, completed) and 'conclusion' (success, failure, etc.)
	if status == "queued" {
		return domain.StatusPending
	}
	if status == "in_progress" {
		return domain.StatusRunning
	}

	// Status is 'completed', check conclusion
	switch conclusion {
	case "success":
		return domain.StatusSuccess
	case "failure":
		return domain.StatusFailed
	case "cancelled":
		return domain.StatusCanceled
	case "skipped":
		return domain.StatusSkipped
	default:
		return domain.StatusFailed
	}
}

// GitHub API response types
type githubRepository struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	HTMLURL  string `json:"html_url"`
}

type githubWorkflowRunsResponse struct {
	TotalCount   int                 `json:"total_count"`
	WorkflowRuns []githubWorkflowRun `json:"workflow_runs"`
}

type githubWorkflowRun struct {
	ID         int       `json:"id"`
	Name       string    `json:"name"`
	WorkflowID int       `json:"workflow_id"`
	HeadBranch string    `json:"head_branch"`
	Status     string    `json:"status"`
	Conclusion string    `json:"conclusion"`
	HTMLURL    string    `json:"html_url"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
