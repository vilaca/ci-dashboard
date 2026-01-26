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
	"sync"
	"time"

	"github.com/vilaca/ci-dashboard/internal/api"
	"github.com/vilaca/ci-dashboard/internal/domain"
)

// Client implements api.Client for GitHub Actions.
// Follows Single Responsibility Principle - only handles GitHub API communication.
type Client struct {
	*api.BaseClient
	rateLimitMu       sync.RWMutex
	rateLimitRemaining int
	rateLimitReset     time.Time
}

// NewClient creates a new GitHub Actions client.
// Uses dependency injection for HTTPClient (IoC).
func NewClient(config api.ClientConfig, httpClient api.HTTPClient) *Client {
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.github.com"
	}

	return &Client{
		BaseClient: api.NewBaseClient(baseURL, config.Token, httpClient),
	}
}

// GetProjects retrieves repositories with Actions enabled.
func (c *Client) GetProjects(ctx context.Context) ([]domain.Project, error) {
	result, err := c.DoRateLimited(ctx, func() (interface{}, error) {
		var allProjects []domain.Project
		page := 1

		for {
			pageProjects, hasNext, err := c.GetProjectsPage(ctx, page)
			if err != nil {
				return nil, err
			}

			allProjects = append(allProjects, pageProjects...)

			if !hasNext {
				break
			}

			page++
		}

		return allProjects, nil
	})

	if err != nil {
		return nil, err
	}
	return result.([]domain.Project), nil
}

// GetProjectsPage fetches a single page of projects.
// GetProjectCount returns the total number of repositories for the authenticated user.
func (c *Client) GetProjectCount(ctx context.Context) (int, error) {
	// Use search API to get total count
	// Search for all repos owned by the authenticated user
	url := fmt.Sprintf("%s/user/repos?per_page=1&page=1", c.BaseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.Token))
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	c.updateRateLimit(resp.Header)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	// GitHub doesn't provide total count in headers for /user/repos
	// We need to parse Link header to get last page number
	linkHeader := resp.Header.Get("Link")

	// Parse Link header to find last page
	// Example: <https://api.github.com/user/repos?per_page=100&page=2>; rel="next", <https://api.github.com/user/repos?per_page=100&page=3>; rel="last"
	lastPage := 1
	if linkHeader != "" {
		links := strings.Split(linkHeader, ",")
		for _, link := range links {
			if strings.Contains(link, `rel="last"`) {
				// Extract page number from URL
				if start := strings.Index(link, "page="); start != -1 {
					start += 5
					end := start
					for end < len(link) && link[end] >= '0' && link[end] <= '9' {
						end++
					}
					if end > start {
						if _, err := fmt.Sscanf(link[start:end], "%d", &lastPage); err != nil {
							log.Printf("[GitHub] Warning: failed to parse page number from Link header: %v", err)
						}
					}
				}
			}
		}
	}

	// Estimate total count: lastPage * per_page (100)
	// This is an estimate, last page might have fewer items
	estimatedTotal := lastPage * 100

	log.Printf("[GitHub] GetProjectCount: ~%d (estimated from %d pages)", estimatedTotal, lastPage)
	return estimatedTotal, nil
}

func (c *Client) GetProjectsPage(ctx context.Context, page int) ([]domain.Project, bool, error) {
	// Sort by last push time - most recently updated first
	url := fmt.Sprintf("%s/user/repos?per_page=%d&page=%d&sort=pushed&direction=desc", c.BaseURL, api.DefaultPageSize, page)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.Token))
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("request failed: %w", err)
	}

	c.updateRateLimit(resp.Header)

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, false, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Decode this page's repositories
	var ghRepos []githubRepository
	if err := json.NewDecoder(resp.Body).Decode(&ghRepos); err != nil {
		resp.Body.Close()
		return nil, false, fmt.Errorf("failed to decode response: %w", err)
	}

	// Get Link header for pagination
	linkHeader := resp.Header.Get("Link")
	hasNextPage := strings.Contains(linkHeader, `rel="next"`)
	resp.Body.Close()

	// Convert projects from this page
	pageProjects := c.convertProjects(ghRepos)

	// If Link header is missing, assume more pages if we got a full page (100 repos)
	if linkHeader == "" && len(ghRepos) >= 100 {
		hasNextPage = true
	}

	// If no repositories returned, definitely no more pages
	if len(ghRepos) == 0 {
		hasNextPage = false
	}

	return pageProjects, hasNextPage, nil
}

// GetLatestPipeline retrieves the most recent workflow run for a repository and branch.
func (c *Client) GetLatestPipeline(ctx context.Context, projectID, branch string) (*domain.Pipeline, error) {
	result, err := c.DoRateLimited(ctx, func() (interface{}, error) {
		// projectID format: "owner/repo"
		url := fmt.Sprintf("%s/repos/%s/actions/runs?branch=%s&per_page=1", c.BaseURL, projectID, branch)

		var response githubWorkflowRunsResponse
		if err := c.doRequest(ctx, url, &response); err != nil {
			return nil, fmt.Errorf("failed to get workflow runs: %w", err)
		}

		if len(response.WorkflowRuns) == 0 {
			return (*domain.Pipeline)(nil), nil
		}

		return c.convertPipeline(response.WorkflowRuns[0], projectID), nil
	})

	if err != nil {
		return nil, err
	}
	return result.(*domain.Pipeline), nil
}

// GetPipelines retrieves recent workflow runs for a repository.
func (c *Client) GetPipelines(ctx context.Context, projectID string, limit int) ([]domain.Pipeline, error) {
	result, err := c.DoRateLimited(ctx, func() (interface{}, error) {
		url := fmt.Sprintf("%s/repos/%s/actions/runs?per_page=%d", c.BaseURL, projectID, limit)

		var response githubWorkflowRunsResponse
		if err := c.doRequest(ctx, url, &response); err != nil {
			return nil, fmt.Errorf("failed to get workflow runs: %w", err)
		}

		pipelines := make([]domain.Pipeline, len(response.WorkflowRuns))
		for i, run := range response.WorkflowRuns {
			pipelines[i] = *c.convertPipeline(run, projectID)
		}

		return pipelines, nil
	})

	if err != nil {
		return nil, err
	}
	return result.([]domain.Pipeline), nil
}

// GetWorkflowRuns retrieves runs for a specific workflow.
func (c *Client) GetWorkflowRuns(ctx context.Context, projectID string, workflowID string, limit int) ([]domain.Pipeline, error) {
	result, err := c.DoRateLimited(ctx, func() (interface{}, error) {
		url := fmt.Sprintf("%s/repos/%s/actions/workflows/%s/runs?per_page=%d",
			c.BaseURL, projectID, workflowID, limit)

		var response githubWorkflowRunsResponse
		if err := c.doRequest(ctx, url, &response); err != nil {
			return nil, fmt.Errorf("failed to get workflow runs: %w", err)
		}

		pipelines := make([]domain.Pipeline, len(response.WorkflowRuns))
		for i, run := range response.WorkflowRuns {
			pipelines[i] = *c.convertPipeline(run, projectID)
		}

		return pipelines, nil
	})

	if err != nil {
		return nil, err
	}
	return result.([]domain.Pipeline), nil
}

// GetBranches retrieves branches for a repository.
func (c *Client) GetBranches(ctx context.Context, projectID string, limit int) ([]domain.Branch, error) {
	perPage := limit
	if perPage == 0 || perPage > 100 {
		perPage = 100
	}

	result, err := c.DoRateLimited(ctx, func() (interface{}, error) {
		// First, get repository info to find the default branch
		repoURL := fmt.Sprintf("%s/repos/%s", c.BaseURL, projectID)
		var repo githubRepository
		if err := c.doRequest(ctx, repoURL, &repo); err != nil {
			log.Printf("[GitHub] Failed to get repo info for %s (URL: %s): %v", projectID, repoURL, err)
			// Continue without default branch info
		}
		defaultBranch := repo.DefaultBranch

		// Get branches
		url := fmt.Sprintf("%s/repos/%s/branches?per_page=%d", c.BaseURL, projectID, perPage)

		var ghBranches []githubBranch
		if err := c.doRequest(ctx, url, &ghBranches); err != nil {
			return nil, fmt.Errorf("failed to get branches (URL: %s): %w", url, err)
		}

		branches := make([]domain.Branch, len(ghBranches))
		for i, ghb := range ghBranches {
			isDefault := (defaultBranch != "" && ghb.Name == defaultBranch)

			// Only fetch commit details for the default branch to conserve API rate limits
			// Non-default branches will be fetched on-demand if needed via GetBranch()
			if isDefault {
				commitURL := fmt.Sprintf("%s/repos/%s/commits/%s", c.BaseURL, projectID, ghb.Commit.SHA)
				var commitDetails githubCommit
				if err := c.doRequest(ctx, commitURL, &commitDetails); err != nil {
					log.Printf("[GitHub] Failed to get commit details for %s/%s (URL: %s): %v", projectID, ghb.Name, commitURL, err)
					branches[i] = c.convertBranch(ghb, projectID, nil, isDefault)
				} else {
					branches[i] = c.convertBranch(ghb, projectID, &commitDetails, isDefault)
				}
			} else {
				// Non-default branch - no commit details to save API calls
				branches[i] = c.convertBranch(ghb, projectID, nil, isDefault)
			}
		}

		return branches, nil
	})

	if err != nil {
		return nil, err
	}
	return result.([]domain.Branch), nil
}

// GetBranch retrieves a single branch by name.
func (c *Client) GetBranch(ctx context.Context, projectID, branchName string) (*domain.Branch, error) {
	result, err := c.DoRateLimited(ctx, func() (interface{}, error) {
		url := fmt.Sprintf("%s/repos/%s/branches/%s", c.BaseURL, projectID, branchName)

		var ghBranch githubBranch
		if err := c.doRequest(ctx, url, &ghBranch); err != nil {
			return nil, fmt.Errorf("failed to get branch %s (URL: %s): %w", branchName, url, err)
		}

		// Fetch commit details
		commitURL := fmt.Sprintf("%s/repos/%s/commits/%s", c.BaseURL, projectID, ghBranch.Commit.SHA)
		var commitDetails githubCommit
		if err := c.doRequest(ctx, commitURL, &commitDetails); err != nil {
			log.Printf("[GitHub] Failed to get commit details for %s/%s: %v", projectID, branchName, err)
			branch := c.convertBranch(ghBranch, projectID, nil, true)
			return &branch, nil
		}

		branch := c.convertBranch(ghBranch, projectID, &commitDetails, true)
		return &branch, nil
	})

	if err != nil {
		return nil, err
	}
	return result.(*domain.Branch), nil
}

// doRequest performs an HTTP request to GitHub API with rate limit handling.
// Follows Single Level of Abstraction Principle (SLAP).
func (c *Client) doRequest(ctx context.Context, url string, result interface{}) error {
	return c.doRequestWithRetry(ctx, url, result, api.MaxRetryAttempts)
}

// doRequestWithRetry performs an HTTP request with exponential backoff retry.
func (c *Client) doRequestWithRetry(ctx context.Context, url string, result interface{}, maxRetries int) error {
	// Check if rate limit is exhausted before making request
	if err := c.waitForRateLimit(ctx); err != nil {
		return err
	}

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

		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.Token))
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			continue
		}

		// Update rate limit tracking and log status
		c.updateRateLimit(resp.Header)

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
							// Retry after rate limit resets (don't count against retry attempts)
							attempt--
							continue
						case <-ctx.Done():
							return ctx.Err()
						}
					}
				}
				// Rate limit with no reset time or reset time too far in future - fail immediately
				return fmt.Errorf("GitHub API rate limit exceeded (resets at %v): %s", resetTime.Format("15:04:05"), bodyStr)
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

// waitForRateLimit blocks if rate limit is exhausted, waiting until reset time.
func (c *Client) waitForRateLimit(ctx context.Context) error {
	c.rateLimitMu.RLock()
	remaining := c.rateLimitRemaining
	resetTime := c.rateLimitReset
	c.rateLimitMu.RUnlock()

	// If we have requests remaining or don't know the limit yet, proceed
	if remaining > 0 || resetTime.IsZero() {
		return nil
	}

	// Rate limit exhausted - calculate wait time
	waitDuration := time.Until(resetTime)
	if waitDuration <= 0 {
		// Reset time has passed, proceed
		return nil
	}

	log.Printf("GitHub API: Rate limit exhausted (0 requests remaining). Waiting %v until reset at %v",
		waitDuration.Round(time.Second), resetTime.Format("15:04:05"))

	// Wait until reset time or context cancellation
	select {
	case <-time.After(waitDuration):
		log.Printf("GitHub API: Rate limit reset, resuming requests")
		return nil
	case <-ctx.Done():
		return fmt.Errorf("context cancelled while waiting for rate limit reset: %w", ctx.Err())
	}
}

// updateRateLimit updates the rate limit state from response headers and logs warnings.
func (c *Client) updateRateLimit(headers http.Header) {
	limit := headers.Get("X-RateLimit-Limit")
	remaining := headers.Get("X-RateLimit-Remaining")
	reset := headers.Get("X-RateLimit-Reset")

	if limit == "" || remaining == "" || reset == "" {
		return
	}

	limitInt, err := strconv.Atoi(limit)
	if err != nil {
		return
	}

	remainingInt, err := strconv.Atoi(remaining)
	if err != nil {
		return
	}

	resetUnix, err := strconv.ParseInt(reset, 10, 64)
	if err != nil {
		return
	}
	resetTime := time.Unix(resetUnix, 0)

	// Update stored rate limit state
	c.rateLimitMu.Lock()
	c.rateLimitRemaining = remainingInt
	c.rateLimitReset = resetTime
	c.rateLimitMu.Unlock()

	// Log warning when below 5% of rate limit (but not at 0 - that will trigger blocking message)
	if limitInt > 0 && remainingInt > 0 && remainingInt < limitInt/20 {
		log.Printf("GitHub API: Rate limit warning - %d/%d requests remaining (resets at %v)",
			remainingInt, limitInt, resetTime.Format("15:04:05"))
	} else if remainingInt == 0 {
		log.Printf("GitHub API: Rate limit exhausted - further requests will block until %v",
			resetTime.Format("15:04:05"))
	}
}

// convertProjects converts GitHub repositories to domain models.
func (c *Client) convertProjects(ghRepos []githubRepository) []domain.Project {
	projects := make([]domain.Project, 0, len(ghRepos))
	for _, repo := range ghRepos {
		project := domain.Project{
			ID:            repo.FullName,
			Name:          repo.Name,
			WebURL:        repo.HTMLURL,
			Platform:      "github",
			IsFork:        repo.Fork,
			DefaultBranch: repo.DefaultBranch,
			LastActivity:  repo.UpdatedAt,
		}

		project.Owner = &domain.ProjectOwner{
			Username: repo.Owner.Login,
			Name:     repo.Owner.Name,
			Type:     repo.Owner.Type,
		}

		parts := strings.Split(repo.FullName, "/")
		if len(parts) == 2 {
			project.Namespace = &domain.ProjectNamespace{
				ID:   parts[0],
				Path: parts[0],
				Kind: strings.ToLower(repo.Owner.Type),
			}
		}

		if repo.Permissions != nil {
			accessLevel := 10
			if repo.Permissions.Push {
				accessLevel = 30
			}
			if repo.Permissions.Admin {
				accessLevel = 50
			}

			project.Permissions = &domain.ProjectPermissions{
				AccessLevel: accessLevel,
				Admin:       repo.Permissions.Admin,
				Push:        repo.Permissions.Push,
				Pull:        repo.Permissions.Pull,
			}
		}

		projects = append(projects, project)
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

// convertBranch converts GitHub branch to domain model.
func (c *Client) convertBranch(ghb githubBranch, projectID string, commit *githubCommit, isDefault bool) domain.Branch {
	// Parse owner/repo to construct branch URL
	webURL := fmt.Sprintf("https://github.com/%s/tree/%s", projectID, ghb.Name)

	// Extract commit details if available
	commitMsg := ""
	commitDate := time.Time{}
	commitAuthor := ""
	if commit != nil {
		commitMsg = commit.Commit.Message
		commitDate = commit.Commit.Author.Date
		commitAuthor = commit.Commit.Author.Name
	}

	return domain.Branch{
		Name:           ghb.Name,
		ProjectID:      projectID,
		Repository:     projectID,
		LastCommitSHA:  ghb.Commit.SHA,
		LastCommitMsg:  commitMsg,
		LastCommitDate: commitDate,
		CommitAuthor:   commitAuthor,
		IsDefault:      isDefault,
		IsProtected:    ghb.Protected,
		WebURL:         webURL,
		Platform:       "github",
	}
}

// GitHub API response types
type githubRepository struct {
	ID            int                  `json:"id"`
	Name          string               `json:"name"`
	FullName      string               `json:"full_name"`
	HTMLURL       string               `json:"html_url"`
	DefaultBranch string               `json:"default_branch"`
	Fork          bool                 `json:"fork"`
	Owner         githubUser           `json:"owner"`
	Permissions   *githubPermissions   `json:"permissions"`
	UpdatedAt     time.Time            `json:"updated_at"`
}

type githubPermissions struct {
	Admin    bool `json:"admin"`
	Maintain bool `json:"maintain"`
	Push     bool `json:"push"`
	Pull     bool `json:"pull"`
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

type githubBranch struct {
	Name      string `json:"name"`
	Protected bool   `json:"protected"`
	Commit    struct {
		SHA string `json:"sha"`
		URL string `json:"url"`
	} `json:"commit"`
}

type githubCommit struct {
	SHA    string `json:"sha"`
	Commit struct {
		Author struct {
			Name  string    `json:"name"`
			Email string    `json:"email"`
			Date  time.Time `json:"date"`
		} `json:"author"`
		Message string `json:"message"`
	} `json:"commit"`
}

// GetMergeRequests retrieves open pull requests for a repository.
func (c *Client) GetMergeRequests(ctx context.Context, projectID string) ([]domain.MergeRequest, error) {
	result, err := c.DoRateLimited(ctx, func() (interface{}, error) {
		// projectID format: "owner/repo"
		url := fmt.Sprintf("%s/repos/%s/pulls?state=open&per_page=50", c.BaseURL, projectID)

		var ghPRs []githubPullRequest
		if err := c.doRequest(ctx, url, &ghPRs); err != nil {
			return nil, fmt.Errorf("failed to get pull requests: %w", err)
		}

		mrs := make([]domain.MergeRequest, len(ghPRs))
		for i, pr := range ghPRs {
			mrs[i] = c.convertPullRequest(pr, projectID)
		}

		return mrs, nil
	})

	if err != nil {
		return nil, err
	}
	return result.([]domain.MergeRequest), nil
}

// GetIssues retrieves open issues for a repository.
func (c *Client) GetIssues(ctx context.Context, projectID string) ([]domain.Issue, error) {
	result, err := c.DoRateLimited(ctx, func() (interface{}, error) {
		// projectID format: "owner/repo"
		url := fmt.Sprintf("%s/repos/%s/issues?state=open&per_page=50", c.BaseURL, projectID)

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
	})

	if err != nil {
		return nil, err
	}
	return result.([]domain.Issue), nil
}

// GetCurrentUser retrieves the authenticated user's profile.
func (c *Client) GetCurrentUser(ctx context.Context) (*domain.UserProfile, error) {
	result, err := c.DoRateLimited(ctx, func() (interface{}, error) {
		url := fmt.Sprintf("%s/user", c.BaseURL)

		var ghUser githubUser
		if err := c.doRequest(ctx, url, &ghUser); err != nil {
			return nil, fmt.Errorf("failed to get current user: %w", err)
		}

		profile := &domain.UserProfile{
			Username:  ghUser.Login,
			Name:      ghUser.Name,
			AvatarURL: ghUser.AvatarURL,
			WebURL:    ghUser.HTMLURL,
			Platform:  "github",
		}

		return profile, nil
	})

	if err != nil {
		return nil, err
	}
	return result.(*domain.UserProfile), nil
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
		IsDraft:      pr.Draft,
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
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	State     string     `json:"state"`
	Draft     bool       `json:"draft"` // true if PR is in draft mode
	Head      githubRef  `json:"head"`
	Base      githubRef  `json:"base"`
	User      githubUser `json:"user"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	HTMLURL   string     `json:"html_url"`
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
	Login     string `json:"login"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	AvatarURL string `json:"avatar_url"`
	HTMLURL   string `json:"html_url"`
}

// GitHub Label type
type githubLabel struct {
	Name string `json:"name"`
}

// GitHub PR reference (to distinguish issues from PRs)
type githubPRRef struct {
	URL string `json:"url"`
}
