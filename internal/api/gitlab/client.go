package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/vilaca/ci-dashboard/internal/api"
	"github.com/vilaca/ci-dashboard/internal/domain"
)

// Client implements api.Client for GitLab.
// Follows Single Responsibility Principle - only handles GitLab API communication.
type Client struct {
	baseURL    string
	token      string
	httpClient HTTPClient

	// Rate limiting and deduplication
	semaphore  chan struct{}            // Limits concurrent requests
	inFlight   map[string]*requestState // Tracks in-flight requests
	inFlightMu sync.Mutex               // Protects inFlight map
}

// requestState tracks a single in-flight request.
type requestState struct {
	done   chan struct{}  // Closed when request completes
	result interface{}    // The result (or nil)
	err    error          // Any error
}

// HTTPClient interface for HTTP operations (allows mocking in tests).
// Follows Interface Segregation Principle.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// NewClient creates a new GitLab client.
// Uses dependency injection for HTTPClient (IoC).
func NewClient(config api.ClientConfig, httpClient HTTPClient) *Client {
	return &Client{
		baseURL:    config.BaseURL,
		token:      config.Token,
		httpClient: httpClient,
		semaphore:  make(chan struct{}, 5), // Max 5 concurrent requests
		inFlight:   make(map[string]*requestState),
	}
}

// doRequestWithDedup performs an HTTP request with deduplication and rate limiting.
// If the same request is already in-flight, it waits for that request to complete
// and returns the cached result instead of making a duplicate API call.
func (c *Client) doRequestWithDedup(ctx context.Context, key string, fn func() (interface{}, error)) (interface{}, error) {
	startTime := time.Now()

	// Check if this request is already in-flight
	c.inFlightMu.Lock()
	if state, exists := c.inFlight[key]; exists {
		// Request is in-flight, wait for it to complete
		c.inFlightMu.Unlock()
		log.Printf("[GitLab] %s - DEDUPED (waiting for in-flight request)", key)

		select {
		case <-state.done:
			duration := time.Since(startTime)
			if state.err != nil {
				log.Printf("[GitLab] %s - DEDUPED ERROR (waited %v): %v", key, duration.Round(time.Millisecond), state.err)
			} else {
				log.Printf("[GitLab] %s - DEDUPED SUCCESS (waited %v)", key, duration.Round(time.Millisecond))
			}
			return state.result, state.err
		case <-ctx.Done():
			log.Printf("[GitLab] %s - DEDUPED CANCELLED (waited %v): %v", key, time.Since(startTime).Round(time.Millisecond), ctx.Err())
			return nil, ctx.Err()
		}
	}

	// Create new request state
	state := &requestState{
		done: make(chan struct{}),
	}
	c.inFlight[key] = state
	c.inFlightMu.Unlock()

	// Ensure cleanup on exit
	defer func() {
		c.inFlightMu.Lock()
		delete(c.inFlight, key)
		c.inFlightMu.Unlock()
		close(state.done)
	}()

	// Acquire semaphore (rate limiting)
	semaphoreStart := time.Now()
	select {
	case c.semaphore <- struct{}{}:
		defer func() { <-c.semaphore }()
		queueTime := time.Since(semaphoreStart)
		if queueTime > 100*time.Millisecond {
			log.Printf("[GitLab] %s - QUEUED for %v (rate limit)", key, queueTime.Round(time.Millisecond))
		}
	case <-ctx.Done():
		state.err = ctx.Err()
		log.Printf("[GitLab] %s - CANCELLED (while queuing): %v", key, ctx.Err())
		return nil, state.err
	}

	// Execute the actual request
	log.Printf("[GitLab] %s - START", key)
	requestStart := time.Now()
	state.result, state.err = fn()
	requestDuration := time.Since(requestStart)
	totalDuration := time.Since(startTime)

	if state.err != nil {
		log.Printf("[GitLab] %s - ERROR (request: %v, total: %v): %v",
			key, requestDuration.Round(time.Millisecond), totalDuration.Round(time.Millisecond), state.err)
	} else {
		log.Printf("[GitLab] %s - SUCCESS (request: %v, total: %v)",
			key, requestDuration.Round(time.Millisecond), totalDuration.Round(time.Millisecond))
	}

	return state.result, state.err
}

// GetProjects retrieves all projects from GitLab.
func (c *Client) GetProjects(ctx context.Context) ([]domain.Project, error) {
	key := "GetProjects"

	result, err := c.doRequestWithDedup(ctx, key, func() (interface{}, error) {
		// Fetch all accessible projects (not just direct membership)
		// This works better with organization/group-based access
		url := fmt.Sprintf("%s/api/v4/projects?per_page=100", c.baseURL)

		var glProjects []gitlabProject
		if err := c.doRequest(ctx, url, &glProjects); err != nil {
			return nil, fmt.Errorf("failed to get projects: %w", err)
		}

		return c.convertProjects(glProjects), nil
	})

	if err != nil {
		return nil, err
	}
	return result.([]domain.Project), nil
}

// GetLatestPipeline retrieves the most recent pipeline for a project and branch.
func (c *Client) GetLatestPipeline(ctx context.Context, projectID, branch string) (*domain.Pipeline, error) {
	key := fmt.Sprintf("GetLatestPipeline:%s:%s", projectID, branch)

	result, err := c.doRequestWithDedup(ctx, key, func() (interface{}, error) {
		url := fmt.Sprintf("%s/api/v4/projects/%s/pipelines?ref=%s&per_page=1", c.baseURL, projectID, branch)

		var glPipelines []gitlabPipeline
		if err := c.doRequest(ctx, url, &glPipelines); err != nil {
			return nil, fmt.Errorf("failed to get pipeline: %w", err)
		}

		if len(glPipelines) == 0 {
			return (*domain.Pipeline)(nil), nil
		}

		return c.convertPipeline(glPipelines[0], projectID), nil
	})

	if err != nil {
		return nil, err
	}
	return result.(*domain.Pipeline), nil
}

// GetPipelines retrieves recent pipelines for a project.
func (c *Client) GetPipelines(ctx context.Context, projectID string, limit int) ([]domain.Pipeline, error) {
	key := fmt.Sprintf("GetPipelines:%s:%d", projectID, limit)

	result, err := c.doRequestWithDedup(ctx, key, func() (interface{}, error) {
		url := fmt.Sprintf("%s/api/v4/projects/%s/pipelines?per_page=%d", c.baseURL, projectID, limit)

		var glPipelines []gitlabPipeline
		if err := c.doRequest(ctx, url, &glPipelines); err != nil {
			return nil, fmt.Errorf("failed to get pipelines: %w", err)
		}

		pipelines := make([]domain.Pipeline, len(glPipelines))
		for i, glp := range glPipelines {
			pipelines[i] = *c.convertPipeline(glp, projectID)
		}

		return pipelines, nil
	})

	if err != nil {
		return nil, err
	}
	return result.([]domain.Pipeline), nil
}

// GetBranches retrieves branches for a project.
func (c *Client) GetBranches(ctx context.Context, projectID string, limit int) ([]domain.Branch, error) {
	perPage := limit
	if perPage == 0 || perPage > 100 {
		perPage = 100
	}

	key := fmt.Sprintf("GetBranches:%s:%d", projectID, perPage)

	result, err := c.doRequestWithDedup(ctx, key, func() (interface{}, error) {
		url := fmt.Sprintf("%s/api/v4/projects/%s/repository/branches?per_page=%d", c.baseURL, projectID, perPage)

		var glBranches []gitlabBranch
		if err := c.doRequest(ctx, url, &glBranches); err != nil {
			return nil, fmt.Errorf("failed to get branches: %w", err)
		}

		branches := make([]domain.Branch, len(glBranches))
		for i, glb := range glBranches {
			branches[i] = c.convertBranch(glb, projectID)
		}

		return branches, nil
	})

	if err != nil {
		return nil, err
	}
	return result.([]domain.Branch), nil
}

// doRequest performs an HTTP request to GitLab API.
// Follows Single Level of Abstraction Principle (SLAP).
func (c *Client) doRequest(ctx context.Context, url string, result interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("PRIVATE-TOKEN", c.token)
	req.Header.Set("Accept", "application/json")

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

// convertProjects converts GitLab projects to domain models.
func (c *Client) convertProjects(glProjects []gitlabProject) []domain.Project {
	projects := make([]domain.Project, len(glProjects))
	for i, glp := range glProjects {
		projects[i] = domain.Project{
			ID:       fmt.Sprintf("%d", glp.ID),
			Name:     glp.Name,
			WebURL:   glp.WebURL,
			Platform: "gitlab",
		}
	}
	return projects
}

// convertPipeline converts a GitLab pipeline to domain model.
func (c *Client) convertPipeline(glp gitlabPipeline, projectID string) *domain.Pipeline {
	// Calculate duration
	duration := glp.UpdatedAt.Sub(glp.CreatedAt)

	return &domain.Pipeline{
		ID:         fmt.Sprintf("%d", glp.ID),
		ProjectID:  projectID,
		Branch:     glp.Ref,
		Status:     convertStatus(glp.Status),
		CreatedAt:  glp.CreatedAt,
		UpdatedAt:  glp.UpdatedAt,
		Duration:   duration,
		WebURL:     glp.WebURL,
		Repository: "", // Will be filled by service layer
	}
}

// convertBranch converts GitLab branch to domain model.
func (c *Client) convertBranch(glb gitlabBranch, projectID string) domain.Branch {
	return domain.Branch{
		Name:           glb.Name,
		ProjectID:      projectID,
		Repository:     projectID,
		LastCommitSHA:  glb.Commit.ID,
		LastCommitMsg:  glb.Commit.Message,
		LastCommitDate: glb.Commit.CommittedDate,
		CommitAuthor:   glb.Commit.AuthorName,
		IsDefault:      glb.Default,
		IsProtected:    glb.Protected,
		WebURL:         glb.WebURL,
		Platform:       "gitlab",
	}
}

// convertStatus converts GitLab status to domain status.
func convertStatus(glStatus string) domain.Status {
	switch glStatus {
	case "pending":
		return domain.StatusPending
	case "running":
		return domain.StatusRunning
	case "success":
		return domain.StatusSuccess
	case "failed":
		return domain.StatusFailed
	case "canceled":
		return domain.StatusCanceled
	case "skipped":
		return domain.StatusSkipped
	default:
		return domain.Status(glStatus)
	}
}

// GitLab API response types
type gitlabProject struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	WebURL string `json:"web_url"`
}

type gitlabPipeline struct {
	ID        int       `json:"id"`
	Status    string    `json:"status"`
	Ref       string    `json:"ref"`
	WebURL    string    `json:"web_url"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type gitlabBranch struct {
	Name      string `json:"name"`
	Default   bool   `json:"default"`
	Protected bool   `json:"protected"`
	WebURL    string `json:"web_url"`
	Commit    struct {
		ID            string    `json:"id"`
		Message       string    `json:"message"`
		CommittedDate time.Time `json:"committed_date"`
		AuthorName    string    `json:"author_name"`
	} `json:"commit"`
}

// GetMergeRequests retrieves open merge requests for a project.
func (c *Client) GetMergeRequests(ctx context.Context, projectID string) ([]domain.MergeRequest, error) {
	key := fmt.Sprintf("GetMergeRequests:%s", projectID)

	result, err := c.doRequestWithDedup(ctx, key, func() (interface{}, error) {
		url := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests?state=opened&per_page=50", c.baseURL, projectID)

		var glMRs []gitlabMergeRequest
		if err := c.doRequest(ctx, url, &glMRs); err != nil {
			return nil, fmt.Errorf("failed to get merge requests: %w", err)
		}

		mrs := make([]domain.MergeRequest, len(glMRs))
		for i, glMR := range glMRs {
			mrs[i] = c.convertMergeRequest(glMR, projectID)
		}

		return mrs, nil
	})

	if err != nil {
		return nil, err
	}
	return result.([]domain.MergeRequest), nil
}

// GetIssues retrieves open issues for a project.
func (c *Client) GetIssues(ctx context.Context, projectID string) ([]domain.Issue, error) {
	key := fmt.Sprintf("GetIssues:%s", projectID)

	result, err := c.doRequestWithDedup(ctx, key, func() (interface{}, error) {
		url := fmt.Sprintf("%s/api/v4/projects/%s/issues?state=opened&per_page=50", c.baseURL, projectID)

		var glIssues []gitlabIssue
		if err := c.doRequest(ctx, url, &glIssues); err != nil {
			return nil, fmt.Errorf("failed to get issues: %w", err)
		}

		issues := make([]domain.Issue, len(glIssues))
		for i, glIssue := range glIssues {
			issues[i] = c.convertIssue(glIssue, projectID)
		}

		return issues, nil
	})

	if err != nil {
		return nil, err
	}
	return result.([]domain.Issue), nil
}

// convertMergeRequest converts GitLab MR to domain MergeRequest.
func (c *Client) convertMergeRequest(glMR gitlabMergeRequest, projectID string) domain.MergeRequest {
	return domain.MergeRequest{
		ID:           fmt.Sprintf("%d", glMR.IID),
		Number:       glMR.IID,
		Title:        glMR.Title,
		Description:  glMR.Description,
		State:        glMR.State,
		SourceBranch: glMR.SourceBranch,
		TargetBranch: glMR.TargetBranch,
		Author:       glMR.Author.Username,
		CreatedAt:    glMR.CreatedAt,
		UpdatedAt:    glMR.UpdatedAt,
		WebURL:       glMR.WebURL,
		ProjectID:    projectID,
		Repository:   glMR.Title, // GitLab doesn't return repo name in MR, using title
	}
}

// convertIssue converts GitLab issue to domain Issue.
func (c *Client) convertIssue(glIssue gitlabIssue, projectID string) domain.Issue {
	labels := make([]string, len(glIssue.Labels))
	copy(labels, glIssue.Labels)

	assignee := ""
	if glIssue.Assignee != nil {
		assignee = glIssue.Assignee.Username
	}

	return domain.Issue{
		ID:          fmt.Sprintf("%d", glIssue.IID),
		Number:      glIssue.IID,
		Title:       glIssue.Title,
		Description: glIssue.Description,
		State:       glIssue.State,
		Labels:      labels,
		Author:      glIssue.Author.Username,
		Assignee:    assignee,
		CreatedAt:   glIssue.CreatedAt,
		UpdatedAt:   glIssue.UpdatedAt,
		WebURL:      glIssue.WebURL,
		ProjectID:   projectID,
		Repository:  glIssue.Title, // GitLab doesn't return repo name in issue, using title
	}
}

// GitLab MergeRequest type
type gitlabMergeRequest struct {
	IID          int       `json:"iid"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	State        string    `json:"state"`
	SourceBranch string    `json:"source_branch"`
	TargetBranch string    `json:"target_branch"`
	Author       gitlabUser `json:"author"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	WebURL       string    `json:"web_url"`
}

// GitLab Issue type
type gitlabIssue struct {
	IID         int         `json:"iid"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	State       string      `json:"state"`
	Labels      []string    `json:"labels"`
	Author      gitlabUser  `json:"author"`
	Assignee    *gitlabUser `json:"assignee"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
	WebURL      string      `json:"web_url"`
}

// GitLab User type
type gitlabUser struct {
	Username string `json:"username"`
	Name     string `json:"name"`
}
