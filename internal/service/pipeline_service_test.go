package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/vilaca/ci-dashboard/internal/domain"
)

// mockClient is a test double for api.Client.
// Follows FIRST principles - Independent tests.
type mockClient struct {
	getProjectsFunc       func(ctx context.Context) ([]domain.Project, error)
	getLatestPipelineFunc func(ctx context.Context, projectID, branch string) (*domain.Pipeline, error)
	getPipelinesFunc      func(ctx context.Context, projectID string, limit int) ([]domain.Pipeline, error)
	getBranchesFunc       func(ctx context.Context, projectID string, limit int) ([]domain.Branch, error)
}

func (m *mockClient) GetProjects(ctx context.Context) ([]domain.Project, error) {
	if m.getProjectsFunc != nil {
		return m.getProjectsFunc(ctx)
	}
	return nil, nil
}

func (m *mockClient) GetLatestPipeline(ctx context.Context, projectID, branch string) (*domain.Pipeline, error) {
	if m.getLatestPipelineFunc != nil {
		return m.getLatestPipelineFunc(ctx, projectID, branch)
	}
	return nil, nil
}

func (m *mockClient) GetPipelines(ctx context.Context, projectID string, limit int) ([]domain.Pipeline, error) {
	if m.getPipelinesFunc != nil {
		return m.getPipelinesFunc(ctx, projectID, limit)
	}
	return nil, nil
}

func (m *mockClient) GetBranches(ctx context.Context, projectID string, limit int) ([]domain.Branch, error) {
	if m.getBranchesFunc != nil {
		return m.getBranchesFunc(ctx, projectID, limit)
	}
	return nil, nil
}

// TestNewPipelineService tests service creation.
// Follows AAA pattern.
func TestNewPipelineService(t *testing.T) {
	// Arrange & Act
	service := NewPipelineService(nil, nil)

	// Assert
	if service == nil {
		t.Fatal("expected service to be non-nil")
	}

	if service.clients == nil {
		t.Error("expected clients map to be initialized")
	}
}

// TestRegisterClient tests client registration.
func TestRegisterClient(t *testing.T) {
	// Arrange
	service := NewPipelineService(nil, nil)
	client := &mockClient{}

	// Act
	service.RegisterClient("gitlab", client)

	// Assert
	service.mu.RLock()
	registered := service.clients["gitlab"]
	service.mu.RUnlock()

	if registered == nil {
		t.Error("expected client to be registered")
	}
}

// TestGetAllProjects tests retrieving projects from all platforms.
func TestGetAllProjects(t *testing.T) {
	// Arrange
	gitlabClient := &mockClient{
		getProjectsFunc: func(ctx context.Context) ([]domain.Project, error) {
			return []domain.Project{
				{ID: "1", Name: "gitlab-project", Platform: "gitlab"},
			}, nil
		},
	}

	githubClient := &mockClient{
		getProjectsFunc: func(ctx context.Context) ([]domain.Project, error) {
			return []domain.Project{
				{ID: "owner/repo", Name: "github-project", Platform: "github"},
			}, nil
		},
	}

	service := NewPipelineService(nil, nil)
	service.RegisterClient("gitlab", gitlabClient)
	service.RegisterClient("github", githubClient)

	// Act
	projects, err := service.GetAllProjects(context.Background())

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}

	// Verify both platforms are represented
	platformCount := make(map[string]int)
	for _, p := range projects {
		platformCount[p.Platform]++
	}

	if platformCount["gitlab"] != 1 {
		t.Errorf("expected 1 gitlab project, got %d", platformCount["gitlab"])
	}

	if platformCount["github"] != 1 {
		t.Errorf("expected 1 github project, got %d", platformCount["github"])
	}
}

// TestGetAllProjects_ClientError tests error handling.
func TestGetAllProjects_ClientError(t *testing.T) {
	// Arrange
	failingClient := &mockClient{
		getProjectsFunc: func(ctx context.Context) ([]domain.Project, error) {
			return nil, errors.New("API error")
		},
	}

	service := NewPipelineService(nil, nil)
	service.RegisterClient("gitlab", failingClient)

	// Act
	projects, err := service.GetAllProjects(context.Background())

	// Assert
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if projects != nil {
		t.Errorf("expected nil projects on error, got %v", projects)
	}
}

// TestGetPipelinesByProject tests retrieving pipelines for specific projects.
func TestGetPipelinesByProject(t *testing.T) {
	// Arrange
	client := &mockClient{
		getLatestPipelineFunc: func(ctx context.Context, projectID, branch string) (*domain.Pipeline, error) {
			if projectID == "123" && branch == "main" {
				return &domain.Pipeline{
					ID:         "p1",
					ProjectID:  projectID,
					Repository: projectID,
					Branch:     branch,
					Status:     domain.StatusSuccess,
				}, nil
			}
			return nil, errors.New("not found")
		},
	}

	service := NewPipelineService(nil, nil)
	service.RegisterClient("gitlab", client)

	// Act
	pipelines, err := service.GetPipelinesByProject(context.Background(), []string{"123"})

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(pipelines) == 0 {
		t.Fatal("expected at least one pipeline")
	}

	if pipelines[0].ID != "p1" {
		t.Errorf("expected pipeline ID 'p1', got '%s'", pipelines[0].ID)
	}
}

// TestGetPipelinesByProject_EmptyInput tests handling empty input.
func TestGetPipelinesByProject_EmptyInput(t *testing.T) {
	// Arrange
	service := NewPipelineService(nil, nil)

	// Act
	pipelines, err := service.GetPipelinesByProject(context.Background(), []string{})

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(pipelines) != 0 {
		t.Errorf("expected empty pipelines, got %d", len(pipelines))
	}
}

// TestGetAllProjects_GitLabWhitelist tests GitLab whitelist filtering.
func TestGetAllProjects_GitLabWhitelist(t *testing.T) {
	// Arrange
	gitlabClient := &mockClient{
		getProjectsFunc: func(ctx context.Context) ([]domain.Project, error) {
			return []domain.Project{
				{ID: "123", Name: "allowed-project", Platform: "gitlab"},
				{ID: "456", Name: "blocked-project", Platform: "gitlab"},
			}, nil
		},
	}

	// Create service with GitLab whitelist allowing only project 123
	service := NewPipelineService([]string{"123"}, nil)
	service.RegisterClient("gitlab", gitlabClient)

	// Act
	projects, err := service.GetAllProjects(context.Background())

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(projects) != 1 {
		t.Fatalf("expected 1 project (whitelisted), got %d", len(projects))
	}

	if projects[0].ID != "123" {
		t.Errorf("expected project 123, got %s", projects[0].ID)
	}
}

// TestGetAllProjects_GitHubWhitelist tests GitHub whitelist filtering.
func TestGetAllProjects_GitHubWhitelist(t *testing.T) {
	// Arrange
	githubClient := &mockClient{
		getProjectsFunc: func(ctx context.Context) ([]domain.Project, error) {
			return []domain.Project{
				{ID: "owner/allowed", Name: "allowed-project", Platform: "github"},
				{ID: "owner/blocked", Name: "blocked-project", Platform: "github"},
			}, nil
		},
	}

	// Create service with GitHub whitelist allowing only owner/allowed
	service := NewPipelineService(nil, []string{"owner/allowed"})
	service.RegisterClient("github", githubClient)

	// Act
	projects, err := service.GetAllProjects(context.Background())

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(projects) != 1 {
		t.Fatalf("expected 1 project (whitelisted), got %d", len(projects))
	}

	if projects[0].ID != "owner/allowed" {
		t.Errorf("expected project owner/allowed, got %s", projects[0].ID)
	}
}

// TestGetAllProjects_MixedWhitelist tests both GitLab and GitHub whitelists together.
func TestGetAllProjects_MixedWhitelist(t *testing.T) {
	// Arrange
	gitlabClient := &mockClient{
		getProjectsFunc: func(ctx context.Context) ([]domain.Project, error) {
			return []domain.Project{
				{ID: "123", Name: "gitlab-allowed", Platform: "gitlab"},
				{ID: "456", Name: "gitlab-blocked", Platform: "gitlab"},
			}, nil
		},
	}

	githubClient := &mockClient{
		getProjectsFunc: func(ctx context.Context) ([]domain.Project, error) {
			return []domain.Project{
				{ID: "owner/allowed", Name: "github-allowed", Platform: "github"},
				{ID: "owner/blocked", Name: "github-blocked", Platform: "github"},
			}, nil
		},
	}

	// Create service with both whitelists
	service := NewPipelineService([]string{"123"}, []string{"owner/allowed"})
	service.RegisterClient("gitlab", gitlabClient)
	service.RegisterClient("github", githubClient)

	// Act
	projects, err := service.GetAllProjects(context.Background())

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(projects) != 2 {
		t.Fatalf("expected 2 projects (whitelisted), got %d", len(projects))
	}

	// Verify both whitelisted projects are present
	projectIDs := make(map[string]bool)
	for _, p := range projects {
		projectIDs[p.ID] = true
	}

	if !projectIDs["123"] {
		t.Error("expected GitLab project 123 to be present")
	}

	if !projectIDs["owner/allowed"] {
		t.Error("expected GitHub project owner/allowed to be present")
	}

	if projectIDs["456"] {
		t.Error("expected GitLab project 456 to be blocked")
	}

	if projectIDs["owner/blocked"] {
		t.Error("expected GitHub project owner/blocked to be blocked")
	}
}

// TestGetAllProjects_NoWhitelist tests that no whitelist allows all repositories.
func TestGetAllProjects_NoWhitelist(t *testing.T) {
	// Arrange
	gitlabClient := &mockClient{
		getProjectsFunc: func(ctx context.Context) ([]domain.Project, error) {
			return []domain.Project{
				{ID: "123", Name: "gitlab-project", Platform: "gitlab"},
			}, nil
		},
	}

	githubClient := &mockClient{
		getProjectsFunc: func(ctx context.Context) ([]domain.Project, error) {
			return []domain.Project{
				{ID: "owner/repo", Name: "github-project", Platform: "github"},
			}, nil
		},
	}

	// Create service with no whitelist (nil, nil)
	service := NewPipelineService(nil, nil)
	service.RegisterClient("gitlab", gitlabClient)
	service.RegisterClient("github", githubClient)

	// Act
	projects, err := service.GetAllProjects(context.Background())

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(projects) != 2 {
		t.Fatalf("expected 2 projects (no filtering), got %d", len(projects))
	}
}

// TestGetAllProjects_EmptyWhitelist tests that empty whitelist allows all repositories.
func TestGetAllProjects_EmptyWhitelist(t *testing.T) {
	// Arrange
	gitlabClient := &mockClient{
		getProjectsFunc: func(ctx context.Context) ([]domain.Project, error) {
			return []domain.Project{
				{ID: "123", Name: "gitlab-project", Platform: "gitlab"},
			}, nil
		},
	}

	// Create service with empty whitelist ([]string{}, []string{})
	service := NewPipelineService([]string{}, []string{})
	service.RegisterClient("gitlab", gitlabClient)

	// Act
	projects, err := service.GetAllProjects(context.Background())

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(projects) != 1 {
		t.Fatalf("expected 1 project (no filtering), got %d", len(projects))
	}
}

// TestGetRepositoriesWithRecentRuns_Sorting tests that repositories are sorted by latest run time.
func TestGetRepositoriesWithRecentRuns_Sorting(t *testing.T) {
	// Arrange
	now := time.Now()
	oldTime := now.Add(-2 * time.Hour)
	middleTime := now.Add(-1 * time.Hour)
	recentTime := now

	client := &mockClient{
		getProjectsFunc: func(ctx context.Context) ([]domain.Project, error) {
			return []domain.Project{
				{ID: "project1", Name: "Project 1", Platform: "github"},
				{ID: "project2", Name: "Project 2", Platform: "github"},
				{ID: "project3", Name: "Project 3", Platform: "github"},
			}, nil
		},
		getPipelinesFunc: func(ctx context.Context, projectID string, limit int) ([]domain.Pipeline, error) {
			// Return pipelines with different update times for each project
			switch projectID {
			case "project1":
				// Project 1 has oldest run
				return []domain.Pipeline{
					{ID: "p1", ProjectID: projectID, UpdatedAt: oldTime, Status: domain.StatusSuccess},
				}, nil
			case "project2":
				// Project 2 has most recent run
				return []domain.Pipeline{
					{ID: "p2", ProjectID: projectID, UpdatedAt: recentTime, Status: domain.StatusSuccess},
				}, nil
			case "project3":
				// Project 3 has middle time run
				return []domain.Pipeline{
					{ID: "p3", ProjectID: projectID, UpdatedAt: middleTime, Status: domain.StatusSuccess},
				}, nil
			}
			return []domain.Pipeline{}, nil
		},
	}

	service := NewPipelineService(nil, nil)
	service.RegisterClient("github", client)

	// Act
	results, err := service.GetRepositoriesWithRecentRuns(context.Background(), 3)

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 repositories, got %d", len(results))
	}

	// Verify sorting: project2 (most recent) should be first
	if results[0].Project.ID != "project2" {
		t.Errorf("expected first repository to be project2, got %s", results[0].Project.ID)
	}

	// project3 (middle time) should be second
	if results[1].Project.ID != "project3" {
		t.Errorf("expected second repository to be project3, got %s", results[1].Project.ID)
	}

	// project1 (oldest) should be last
	if results[2].Project.ID != "project1" {
		t.Errorf("expected third repository to be project1, got %s", results[2].Project.ID)
	}
}

// TestGetRepositoriesWithRecentRuns_NoRuns tests repositories with no runs appear at the end.
func TestGetRepositoriesWithRecentRuns_NoRuns(t *testing.T) {
	// Arrange
	now := time.Now()

	client := &mockClient{
		getProjectsFunc: func(ctx context.Context) ([]domain.Project, error) {
			return []domain.Project{
				{ID: "project1", Name: "Project 1", Platform: "github"},
				{ID: "project2", Name: "Project 2", Platform: "github"},
			}, nil
		},
		getPipelinesFunc: func(ctx context.Context, projectID string, limit int) ([]domain.Pipeline, error) {
			// Only project2 has runs
			if projectID == "project2" {
				return []domain.Pipeline{
					{ID: "p2", ProjectID: projectID, UpdatedAt: now, Status: domain.StatusSuccess},
				}, nil
			}
			// project1 has no runs
			return []domain.Pipeline{}, nil
		},
	}

	service := NewPipelineService(nil, nil)
	service.RegisterClient("github", client)

	// Act
	results, err := service.GetRepositoriesWithRecentRuns(context.Background(), 3)

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 repositories, got %d", len(results))
	}

	// Verify project2 (with runs) comes before project1 (no runs)
	if results[0].Project.ID != "project2" {
		t.Errorf("expected first repository to be project2 (with runs), got %s", results[0].Project.ID)
	}

	if results[1].Project.ID != "project1" {
		t.Errorf("expected second repository to be project1 (no runs), got %s", results[1].Project.ID)
	}
}
