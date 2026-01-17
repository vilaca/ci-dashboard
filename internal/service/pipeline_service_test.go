package service

import (
	"context"
	"errors"
	"testing"

	"github.com/vilaca/ci-dashboard/internal/domain"
)

// mockClient is a test double for api.Client.
// Follows FIRST principles - Independent tests.
type mockClient struct {
	getProjectsFunc       func(ctx context.Context) ([]domain.Project, error)
	getLatestPipelineFunc func(ctx context.Context, projectID, branch string) (*domain.Pipeline, error)
	getPipelinesFunc      func(ctx context.Context, projectID string, limit int) ([]domain.Pipeline, error)
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

// TestNewPipelineService tests service creation.
// Follows AAA pattern.
func TestNewPipelineService(t *testing.T) {
	// Arrange & Act
	service := NewPipelineService()

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
	service := NewPipelineService()
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

	service := NewPipelineService()
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

	service := NewPipelineService()
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

	service := NewPipelineService()
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
	service := NewPipelineService()

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
