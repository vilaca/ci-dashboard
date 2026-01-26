package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/vilaca/ci-dashboard/internal/api"
	"github.com/vilaca/ci-dashboard/internal/domain"
)

// Client implements api.Client for GitLab.
// Follows Single Responsibility Principle - only handles GitLab API communication.
type Client struct {
	*api.BaseClient
}

// NewClient creates a new GitLab client.
// Uses dependency injection for HTTPClient (IoC).
func NewClient(config api.ClientConfig, httpClient api.HTTPClient) *Client {
	return &Client{
		BaseClient: api.NewBaseClient(config.BaseURL, config.Token, httpClient),
	}
}

// GetProjects retrieves all projects from GitLab.
func (c *Client) GetProjects(ctx context.Context) ([]domain.Project, error) {
	result, err := c.DoRateLimited(ctx, func() (interface{}, error) {
		// Fetch all accessible projects page by page
		// This works better with organization/group-based access
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
// GetProjectCount returns the total number of projects.
func (c *Client) GetProjectCount(ctx context.Context) (int, error) {
	// Make a lightweight request to get count from headers
	url := fmt.Sprintf("%s/api/v4/projects?per_page=1&page=1", c.BaseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("PRIVATE-TOKEN", c.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Read total count from header
	totalHeader := resp.Header.Get("X-Total")
	if totalHeader == "" {
		return 0, fmt.Errorf("X-Total header not found")
	}

	var total int
	if _, err := fmt.Sscanf(totalHeader, "%d", &total); err != nil {
		return 0, fmt.Errorf("failed to parse X-Total header: %w", err)
	}

	log.Printf("[GitLab] GetProjectCount: %d", total)
	return total, nil
}

func (c *Client) GetProjectsPage(ctx context.Context, page int) ([]domain.Project, bool, error) {
	// Order by last activity (commits, MRs, issues) - most recent first
	url := fmt.Sprintf("%s/api/v4/projects?per_page=%d&page=%d&order_by=last_activity_at&sort=desc", c.BaseURL, api.DefaultPageSize, page)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("PRIVATE-TOKEN", c.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, false, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Decode this page's projects
	var glProjects []gitlabProject
	if err := json.NewDecoder(resp.Body).Decode(&glProjects); err != nil {
		resp.Body.Close()
		return nil, false, fmt.Errorf("failed to decode response: %w", err)
	}

	// Get pagination info from headers
	totalPages := resp.Header.Get("X-Total-Pages")
	resp.Body.Close()

	// Convert projects from this page
	pageProjects := c.convertProjects(glProjects)

	var totalPagesInt int
	hasNextPage := false
	if totalPages != "" {
		// Header available - use it
		if _, err := fmt.Sscanf(totalPages, "%d", &totalPagesInt); err != nil {
			log.Printf("[GitLab] Warning: failed to parse X-Total-Pages header '%s': %v", totalPages, err)
			// Fall back to heuristic: assume more pages if we got a full page
			hasNextPage = len(glProjects) >= api.DefaultPageSize
		} else {
			hasNextPage = page < totalPagesInt
		}
	} else {
		// No header - assume more pages if we got a full page (100 projects)
		hasNextPage = len(glProjects) >= api.DefaultPageSize
	}

	// If no projects returned, definitely no more pages
	if len(glProjects) == 0 {
		hasNextPage = false
	}

	return pageProjects, hasNextPage, nil
}

// GetLatestPipeline retrieves the most recent pipeline for a project and branch.
func (c *Client) GetLatestPipeline(ctx context.Context, projectID, branch string) (*domain.Pipeline, error) {
	result, err := c.DoRateLimited(ctx, func() (interface{}, error) {
		url := fmt.Sprintf("%s/api/v4/projects/%s/pipelines?ref=%s&per_page=1", c.BaseURL, projectID, branch)

		var glPipelines []gitlabPipeline
		if err := c.doRequest(ctx, url, &glPipelines); err != nil {
			return nil, fmt.Errorf("failed to get pipeline (URL: %s): %w", url, err)
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
	result, err := c.DoRateLimited(ctx, func() (interface{}, error) {
		url := fmt.Sprintf("%s/api/v4/projects/%s/pipelines?per_page=%d", c.BaseURL, projectID, limit)

		var glPipelines []gitlabPipeline
		if err := c.doRequest(ctx, url, &glPipelines); err != nil {
			return nil, fmt.Errorf("failed to get pipelines (URL: %s): %w", url, err)
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

	result, err := c.DoRateLimited(ctx, func() (interface{}, error) {
		url := fmt.Sprintf("%s/api/v4/projects/%s/repository/branches?per_page=%d", c.BaseURL, projectID, perPage)

		var glBranches []gitlabBranch
		if err := c.doRequest(ctx, url, &glBranches); err != nil {
			return nil, fmt.Errorf("failed to get branches (URL: %s): %w", url, err)
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

// GetBranch retrieves a single branch by name.
func (c *Client) GetBranch(ctx context.Context, projectID, branchName string) (*domain.Branch, error) {
	result, err := c.DoRateLimited(ctx, func() (interface{}, error) {
		// URL encode the branch name to handle special characters
		encodedBranch := url.QueryEscape(branchName)
		url := fmt.Sprintf("%s/api/v4/projects/%s/repository/branches/%s", c.BaseURL, projectID, encodedBranch)

		var glBranch gitlabBranch
		if err := c.doRequest(ctx, url, &glBranch); err != nil {
			return nil, fmt.Errorf("failed to get branch %s (URL: %s): %w", branchName, url, err)
		}

		branch := c.convertBranch(glBranch, projectID)
		return &branch, nil
	})

	if err != nil {
		return nil, err
	}
	return result.(*domain.Branch), nil
}

// doRequest performs an HTTP request to GitLab API.
// Follows Single Level of Abstraction Principle (SLAP).
func (c *Client) doRequest(ctx context.Context, url string, result interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("PRIVATE-TOKEN", c.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
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
		project := domain.Project{
			ID:            fmt.Sprintf("%d", glp.ID),
			Name:          glp.Name,
			WebURL:        glp.WebURL,
			Platform:      "gitlab",
			IsFork:        glp.ForkedFromProject != nil,
			DefaultBranch: glp.DefaultBranch,
			LastActivity:  glp.LastActivityAt,
		}

		if glp.Owner != nil {
			project.Owner = &domain.ProjectOwner{
				Username: glp.Owner.Username,
				Name:     glp.Owner.Name,
				Type:     "user",
			}
		}

		if glp.Namespace != nil {
			project.Namespace = &domain.ProjectNamespace{
				ID:   fmt.Sprintf("%d", glp.Namespace.ID),
				Path: glp.Namespace.Path,
				Kind: glp.Namespace.Kind,
			}
		}

		if glp.Permissions != nil {
			accessLevel := 0
			if glp.Permissions.ProjectAccess != nil {
				accessLevel = glp.Permissions.ProjectAccess.AccessLevel
			} else if glp.Permissions.GroupAccess != nil {
				accessLevel = glp.Permissions.GroupAccess.AccessLevel
			}

			project.Permissions = &domain.ProjectPermissions{
				AccessLevel: accessLevel,
				Admin:       accessLevel >= 50,
				Push:        accessLevel >= 30,
				Pull:        accessLevel >= 10,
			}
		}

		projects[i] = project
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
	ID                int                  `json:"id"`
	Name              string               `json:"name"`
	WebURL            string               `json:"web_url"`
	DefaultBranch     string               `json:"default_branch"`
	ForkedFromProject *gitlabProjectRef    `json:"forked_from_project,omitempty"`
	Owner             *gitlabUser          `json:"owner"`
	Namespace         *gitlabNamespace     `json:"namespace"`
	Permissions       *gitlabPermissions   `json:"permissions"`
	LastActivityAt    time.Time            `json:"last_activity_at"`
}

type gitlabProjectRef struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type gitlabNamespace struct {
	ID   int    `json:"id"`
	Path string `json:"path"`
	Kind string `json:"kind"`
}

type gitlabPermissions struct {
	ProjectAccess *gitlabAccessLevel `json:"project_access"`
	GroupAccess   *gitlabAccessLevel `json:"group_access"`
}

type gitlabAccessLevel struct {
	AccessLevel int `json:"access_level"`
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
	result, err := c.DoRateLimited(ctx, func() (interface{}, error) {
		url := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests?state=opened&per_page=50", c.BaseURL, projectID)

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
	result, err := c.DoRateLimited(ctx, func() (interface{}, error) {
		url := fmt.Sprintf("%s/api/v4/projects/%s/issues?state=opened&per_page=50", c.BaseURL, projectID)

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

// GetCurrentUser retrieves the authenticated user's profile.
func (c *Client) GetCurrentUser(ctx context.Context) (*domain.UserProfile, error) {
	result, err := c.DoRateLimited(ctx, func() (interface{}, error) {
		url := fmt.Sprintf("%s/api/v4/user", c.BaseURL)

		var glUser gitlabUser
		if err := c.doRequest(ctx, url, &glUser); err != nil {
			return nil, fmt.Errorf("failed to get current user: %w", err)
		}

		profile := &domain.UserProfile{
			Username:  glUser.Username,
			Name:      glUser.Name,
			Email:     glUser.Email,
			AvatarURL: glUser.AvatarURL, // Use GitLab's native avatar URL (often Gravatar)
			WebURL:    glUser.WebURL,
			Platform:  "gitlab",
		}

		return profile, nil
	})

	if err != nil {
		return nil, err
	}
	return result.(*domain.UserProfile), nil
}

// convertMergeRequest converts GitLab MR to domain MergeRequest.
func (c *Client) convertMergeRequest(glMR gitlabMergeRequest, projectID string) domain.MergeRequest {
	return domain.MergeRequest{
		ID:           fmt.Sprintf("%d", glMR.IID),
		Number:       glMR.IID,
		Title:        glMR.Title,
		Description:  glMR.Description,
		State:        glMR.State,
		IsDraft:      glMR.Draft,
		SourceBranch: glMR.SourceBranch,
		TargetBranch: glMR.TargetBranch,
		Author:       glMR.Author.Username,
		CreatedAt:    glMR.CreatedAt,
		UpdatedAt:    glMR.UpdatedAt,
		WebURL:       glMR.WebURL,
		ProjectID:    projectID,
		Repository:   "", // Will be set by service layer from project name
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
		Repository:  "", // Will be set by service layer from project name
	}
}

// GitLab MergeRequest type
type gitlabMergeRequest struct {
	IID          int        `json:"iid"`
	Title        string     `json:"title"`
	Description  string     `json:"description"`
	State        string     `json:"state"`
	Draft        bool       `json:"draft"` // true if MR is in draft/WIP mode
	SourceBranch string     `json:"source_branch"`
	TargetBranch string     `json:"target_branch"`
	Author       gitlabUser `json:"author"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	WebURL       string     `json:"web_url"`
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
	ID        int    `json:"id"`
	Username  string `json:"username"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
	WebURL    string `json:"web_url"`
}
