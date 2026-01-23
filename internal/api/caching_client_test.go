package api

import (
	"context"
	"testing"
	"time"

	"github.com/vilaca/ci-dashboard/internal/domain"
)

// TestPrePopulateCache tests that cache pre-population works correctly.
func TestPrePopulateCache(t *testing.T) {
	// Arrange
	mockClient := &mockHTTPClient{}
	client := &CachingClient{
		client: mockClient,
		cache:  newCache(30 * time.Minute),
	}

	projects := []domain.Project{
		{ID: "1", Name: "project1", Platform: "gitlab"},
		{ID: "2", Name: "project2", Platform: "github"},
	}

	pipelines := []domain.Pipeline{
		{ID: "p1", Repository: "1", Status: domain.StatusSuccess},
	}

	branches := []domain.Branch{
		{Name: "main", ProjectID: "1", IsDefault: true},
	}

	// Act
	client.PrePopulateCache(projects, pipelines, branches, nil, nil, nil)

	// Assert
	// Check that projects were cached
	cached, found := client.cache.get("GetProjects")
	if !found {
		t.Error("Expected projects to be cached")
	}
	cachedProjects := cached.([]domain.Project)
	if len(cachedProjects) != 2 {
		t.Errorf("Expected 2 cached projects, got %d", len(cachedProjects))
	}

	// Check that project count was cached
	cachedCount, found := client.cache.get("GetProjectCount")
	if !found {
		t.Error("Expected project count to be cached")
	}
	if cachedCount.(int) != 2 {
		t.Errorf("Expected count 2, got %d", cachedCount.(int))
	}

	// Check that branches were cached by project
	cachedBranches, found := client.cache.get("GetBranches:1:50")
	if !found {
		t.Error("Expected branches for project 1 to be cached")
	}
	branchList := cachedBranches.([]domain.Branch)
	if len(branchList) != 1 {
		t.Errorf("Expected 1 cached branch, got %d", len(branchList))
	}
}

// mockHTTPClient is a minimal mock for testing
type mockHTTPClient struct{}

func (m *mockHTTPClient) GetProjects(ctx context.Context) ([]domain.Project, error) {
	return nil, nil
}

func (m *mockHTTPClient) GetProjectsPage(ctx context.Context, page int) ([]domain.Project, bool, error) {
	return nil, false, nil
}

func (m *mockHTTPClient) GetProjectCount(ctx context.Context) (int, error) {
	return 0, nil
}

func (m *mockHTTPClient) GetLatestPipeline(ctx context.Context, projectID, branch string) (*domain.Pipeline, error) {
	return nil, nil
}

func (m *mockHTTPClient) GetPipelines(ctx context.Context, projectID string, limit int) ([]domain.Pipeline, error) {
	return nil, nil
}

func (m *mockHTTPClient) GetBranches(ctx context.Context, projectID string, limit int) ([]domain.Branch, error) {
	return nil, nil
}
