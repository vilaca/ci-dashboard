package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
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

// doRequest performs an HTTP request to GitHub API with rate limit handling.
// Follows Single Level of Abstraction Principle (SLAP).
func (c *Client) doRequest(ctx context.Context, url string, result interface{}) error {
	return c.doRequestWithRetry(ctx, url, result, 3)
}

// doRequestWithRetry performs an HTTP request with exponential backoff retry.
func (c *Client) doRequestWithRetry(ctx context.Context, url string, result interface{}, maxRetries int) error {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 2s, 4s, 8s
			backoff := time.Duration(1<<uint(attempt)) * time.Second
			log.Printf("GitHub API: Retrying in %v (attempt %d/%d)", backoff, attempt, maxRetries)

			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			continue
		}

		// Check and log rate limit status
		c.logRateLimitStatus(resp.Header)

		// Handle rate limit errors (403)
		if resp.StatusCode == http.StatusForbidden {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			bodyStr := string(body)

			// Check if this is a rate limit error
			if strings.Contains(bodyStr, "rate limit") || strings.Contains(bodyStr, "API rate limit exceeded") {
				resetTime := c.getRateLimitReset(resp.Header)
				if !resetTime.IsZero() {
					waitDuration := time.Until(resetTime)
					if waitDuration > 0 && waitDuration < 10*time.Minute {
						log.Printf("GitHub API: Rate limit exceeded. Waiting until %v (%v)", resetTime.Format("15:04:05"), waitDuration.Round(time.Second))

						select {
						case <-time.After(waitDuration + time.Second):
							continue // Retry after rate limit resets
						case <-ctx.Done():
							return ctx.Err()
						}
					}
				}
				lastErr = fmt.Errorf("GitHub API rate limit exceeded (resets at %v): %s", resetTime.Format("15:04:05"), bodyStr)
				continue
			}

			return fmt.Errorf("API returned status %d: %s", resp.StatusCode, bodyStr)
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
		}

		// Success - decode and return
		err = json.NewDecoder(resp.Body).Decode(result)
		resp.Body.Close()
		if err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}

		return nil
	}

	if lastErr != nil {
		return fmt.Errorf("request failed after %d retries: %w", maxRetries, lastErr)
	}
	return fmt.Errorf("request failed after %d retries", maxRetries)
}

// logRateLimitStatus logs the current rate limit status from response headers.
func (c *Client) logRateLimitStatus(headers http.Header) {
	limit := headers.Get("X-RateLimit-Limit")
	remaining := headers.Get("X-RateLimit-Remaining")

	if limit == "" || remaining == "" {
		return
	}

	limitInt, _ := strconv.Atoi(limit)
	remainingInt, _ := strconv.Atoi(remaining)

	// Log warning when below 20% of rate limit
	if limitInt > 0 && remainingInt < limitInt/5 {
		resetTime := c.getRateLimitReset(headers)
		log.Printf("GitHub API: Rate limit warning - %d/%d requests remaining (resets at %v)",
			remainingInt, limitInt, resetTime.Format("15:04:05"))
	}
}

// getRateLimitReset extracts the rate limit reset time from headers.
func (c *Client) getRateLimitReset(headers http.Header) time.Time {
	reset := headers.Get("X-RateLimit-Reset")
	if reset == "" {
		return time.Time{}
	}

	resetUnix, err := strconv.ParseInt(reset, 10, 64)
	if err != nil {
		return time.Time{}
	}

	return time.Unix(resetUnix, 0)
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

// GetMergeRequests retrieves open pull requests for a repository.
func (c *Client) GetMergeRequests(ctx context.Context, projectID string) ([]domain.MergeRequest, error) {
	// projectID format: "owner/repo"
	url := fmt.Sprintf("%s/repos/%s/pulls?state=open&per_page=50", c.baseURL, projectID)

	var ghPRs []githubPullRequest
	if err := c.doRequest(ctx, url, &ghPRs); err != nil {
		return nil, fmt.Errorf("failed to get pull requests: %w", err)
	}

	mrs := make([]domain.MergeRequest, len(ghPRs))
	for i, pr := range ghPRs {
		mrs[i] = c.convertPullRequest(pr, projectID)
	}

	return mrs, nil
}

// GetIssues retrieves open issues for a repository.
func (c *Client) GetIssues(ctx context.Context, projectID string) ([]domain.Issue, error) {
	// projectID format: "owner/repo"
	url := fmt.Sprintf("%s/repos/%s/issues?state=open&per_page=50", c.baseURL, projectID)

	var ghIssues []githubIssue
	if err := c.doRequest(ctx, url, &ghIssues); err != nil {
		return nil, fmt.Errorf("failed to get issues: %w", err)
	}

	// Filter out pull requests (GitHub API returns both issues and PRs in /issues endpoint)
	var issues []domain.Issue
	for _, ghIssue := range ghIssues {
		if ghIssue.PullRequest == nil {
			issues = append(issues, c.convertIssue(ghIssue, projectID))
		}
	}

	return issues, nil
}

// convertPullRequest converts GitHub PR to domain MergeRequest.
func (c *Client) convertPullRequest(pr githubPullRequest, projectID string) domain.MergeRequest {
	parts := strings.Split(projectID, "/")
	repoName := projectID
	if len(parts) == 2 {
		repoName = parts[1]
	}

	return domain.MergeRequest{
		ID:           fmt.Sprintf("%d", pr.Number),
		Number:       pr.Number,
		Title:        pr.Title,
		Description:  pr.Body,
		State:        pr.State,
		SourceBranch: pr.Head.Ref,
		TargetBranch: pr.Base.Ref,
		Author:       pr.User.Login,
		CreatedAt:    pr.CreatedAt,
		UpdatedAt:    pr.UpdatedAt,
		WebURL:       pr.HTMLURL,
		ProjectID:    projectID,
		Repository:   repoName,
	}
}

// convertIssue converts GitHub issue to domain Issue.
func (c *Client) convertIssue(ghIssue githubIssue, projectID string) domain.Issue {
	parts := strings.Split(projectID, "/")
	repoName := projectID
	if len(parts) == 2 {
		repoName = parts[1]
	}

	labels := make([]string, len(ghIssue.Labels))
	for i, label := range ghIssue.Labels {
		labels[i] = label.Name
	}

	assignee := ""
	if ghIssue.Assignee != nil {
		assignee = ghIssue.Assignee.Login
	}

	return domain.Issue{
		ID:          fmt.Sprintf("%d", ghIssue.Number),
		Number:      ghIssue.Number,
		Title:       ghIssue.Title,
		Description: ghIssue.Body,
		State:       ghIssue.State,
		Labels:      labels,
		Author:      ghIssue.User.Login,
		Assignee:    assignee,
		CreatedAt:   ghIssue.CreatedAt,
		UpdatedAt:   ghIssue.UpdatedAt,
		WebURL:      ghIssue.HTMLURL,
		ProjectID:   projectID,
		Repository:  repoName,
	}
}

// GitHub PullRequest type
type githubPullRequest struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	State     string    `json:"state"`
	Head      githubRef `json:"head"`
	Base      githubRef `json:"base"`
	User      githubUser `json:"user"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	HTMLURL   string    `json:"html_url"`
}

// GitHub Issue type
type githubIssue struct {
	Number      int              `json:"number"`
	Title       string           `json:"title"`
	Body        string           `json:"body"`
	State       string           `json:"state"`
	Labels      []githubLabel    `json:"labels"`
	User        githubUser       `json:"user"`
	Assignee    *githubUser      `json:"assignee"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
	HTMLURL     string           `json:"html_url"`
	PullRequest *githubPRRef     `json:"pull_request"` // Present if this is a PR
}

// GitHub Ref type
type githubRef struct {
	Ref string `json:"ref"`
}

// GitHub User type
type githubUser struct {
	Login string `json:"login"`
	Name  string `json:"name"`
}

// GitHub Label type
type githubLabel struct {
	Name string `json:"name"`
}

// GitHub PR reference (to distinguish issues from PRs)
type githubPRRef struct {
	URL string `json:"url"`
}
