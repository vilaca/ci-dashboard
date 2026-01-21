package dashboard

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vilaca/ci-dashboard/internal/domain"
	"github.com/vilaca/ci-dashboard/internal/service"
)

// mockRenderer is a test double for Renderer (follows FIRST - Independent).
type mockRenderer struct {
	indexErr             error
	healthErr            error
	pipelinesErr         error
	pipelinesGroupedErr  error
	repositoriesErr      error
	recentPipelinesErr   error
	repositoryDetailErr  error
	failedPipelinesErr   error
	mergeRequestsErr     error
	issuesErr            error
}

func (m *mockRenderer) RenderIndex(w io.Writer) error {
	if m.indexErr != nil {
		return m.indexErr
	}
	_, err := w.Write([]byte("mock index"))
	return err
}

func (m *mockRenderer) RenderHealth(w io.Writer) error {
	if m.healthErr != nil {
		return m.healthErr
	}
	_, err := w.Write([]byte(`{"status":"ok"}`))
	return err
}

func (m *mockRenderer) RenderPipelines(w io.Writer, pipelines []domain.Pipeline) error {
	if m.pipelinesErr != nil {
		return m.pipelinesErr
	}
	_, err := w.Write([]byte("mock pipelines"))
	return err
}

func (m *mockRenderer) RenderPipelinesGrouped(w io.Writer, pipelinesByWorkflow map[string][]domain.Pipeline) error {
	if m.pipelinesGroupedErr != nil {
		return m.pipelinesGroupedErr
	}
	_, err := w.Write([]byte("mock grouped pipelines"))
	return err
}

func (m *mockRenderer) RenderRepositories(w io.Writer, repositories []service.RepositoryWithRuns) error {
	if m.repositoriesErr != nil {
		return m.repositoriesErr
	}
	_, err := w.Write([]byte("mock repositories"))
	return err
}

func (m *mockRenderer) RenderRecentPipelines(w io.Writer, pipelines []domain.Pipeline) error {
	if m.recentPipelinesErr != nil {
		return m.recentPipelinesErr
	}
	_, err := w.Write([]byte("mock recent pipelines"))
	return err
}

func (m *mockRenderer) RenderRepositoryDetail(w io.Writer, repository service.RepositoryWithRuns, mrs []domain.MergeRequest, issues []domain.Issue) error {
	if m.repositoryDetailErr != nil {
		return m.repositoryDetailErr
	}
	_, err := w.Write([]byte("mock repository detail"))
	return err
}

func (m *mockRenderer) RenderFailedPipelines(w io.Writer, pipelines []domain.Pipeline) error {
	if m.failedPipelinesErr != nil {
		return m.failedPipelinesErr
	}
	_, err := w.Write([]byte("mock failed pipelines"))
	return err
}

func (m *mockRenderer) RenderMergeRequests(w io.Writer, mrs []domain.MergeRequest) error {
	if m.mergeRequestsErr != nil {
		return m.mergeRequestsErr
	}
	_, err := w.Write([]byte("mock merge requests"))
	return err
}

func (m *mockRenderer) RenderIssues(w io.Writer, issues []domain.Issue) error {
	if m.issuesErr != nil {
		return m.issuesErr
	}
	_, err := w.Write([]byte("mock issues"))
	return err
}

func (m *mockRenderer) RenderBranches(w io.Writer, branches []domain.BranchWithPipeline) error {
	_, err := w.Write([]byte("mock branches"))
	return err
}

func (m *mockRenderer) RenderYourBranches(w io.Writer, branches []domain.BranchWithPipeline, gitlabUsername, githubUsername string) error {
	_, err := w.Write([]byte("mock your branches"))
	return err
}

// mockLogger is a test double for Logger.
type mockLogger struct {
	messages []string
}

func (m *mockLogger) Printf(format string, v ...interface{}) {
	m.messages = append(m.messages, format)
}

// mockPipelineService is a test double for PipelineService.
type mockPipelineService struct {
	getAllProjectsFunc               func(ctx context.Context) ([]domain.Project, error)
	getPipelinesByProjectFunc        func(ctx context.Context, projectIDs []string) ([]domain.Pipeline, error)
	getLatestPipelinesFunc           func(ctx context.Context) ([]domain.Pipeline, error)
	groupPipelinesByWorkflowFunc     func(pipelines []domain.Pipeline) map[string][]domain.Pipeline
	getPipelinesByWorkflowFunc       func(ctx context.Context, projectID, workflowID string, limit int) ([]domain.Pipeline, error)
	getRepositoriesWithRecentRunsFunc func(ctx context.Context, runsPerRepo int) ([]service.RepositoryWithRuns, error)
	getRecentPipelinesFunc            func(ctx context.Context, totalLimit int) ([]domain.Pipeline, error)
	getAllMergeRequestsFunc           func(ctx context.Context) ([]domain.MergeRequest, error)
	getAllIssuesFunc                  func(ctx context.Context) ([]domain.Issue, error)
}

func (m *mockPipelineService) GetAllProjects(ctx context.Context) ([]domain.Project, error) {
	if m.getAllProjectsFunc != nil {
		return m.getAllProjectsFunc(ctx)
	}
	return []domain.Project{}, nil
}

func (m *mockPipelineService) GetPipelinesByProject(ctx context.Context, projectIDs []string) ([]domain.Pipeline, error) {
	if m.getPipelinesByProjectFunc != nil {
		return m.getPipelinesByProjectFunc(ctx, projectIDs)
	}
	return []domain.Pipeline{}, nil
}

func (m *mockPipelineService) GetLatestPipelines(ctx context.Context) ([]domain.Pipeline, error) {
	if m.getLatestPipelinesFunc != nil {
		return m.getLatestPipelinesFunc(ctx)
	}
	return []domain.Pipeline{}, nil
}

func (m *mockPipelineService) GroupPipelinesByWorkflow(pipelines []domain.Pipeline) map[string][]domain.Pipeline {
	if m.groupPipelinesByWorkflowFunc != nil {
		return m.groupPipelinesByWorkflowFunc(pipelines)
	}
	return make(map[string][]domain.Pipeline)
}

func (m *mockPipelineService) GetPipelinesByWorkflow(ctx context.Context, projectID, workflowID string, limit int) ([]domain.Pipeline, error) {
	if m.getPipelinesByWorkflowFunc != nil {
		return m.getPipelinesByWorkflowFunc(ctx, projectID, workflowID, limit)
	}
	return []domain.Pipeline{}, nil
}

func (m *mockPipelineService) GetRepositoriesWithRecentRuns(ctx context.Context, runsPerRepo int) ([]service.RepositoryWithRuns, error) {
	if m.getRepositoriesWithRecentRunsFunc != nil {
		return m.getRepositoriesWithRecentRunsFunc(ctx, runsPerRepo)
	}
	return []service.RepositoryWithRuns{}, nil
}

func (m *mockPipelineService) GetRecentPipelines(ctx context.Context, totalLimit int) ([]domain.Pipeline, error) {
	if m.getRecentPipelinesFunc != nil {
		return m.getRecentPipelinesFunc(ctx, totalLimit)
	}
	return []domain.Pipeline{}, nil
}

func (m *mockPipelineService) GetAllMergeRequests(ctx context.Context) ([]domain.MergeRequest, error) {
	if m.getAllMergeRequestsFunc != nil {
		return m.getAllMergeRequestsFunc(ctx)
	}
	return []domain.MergeRequest{}, nil
}

func (m *mockPipelineService) GetAllIssues(ctx context.Context) ([]domain.Issue, error) {
	if m.getAllIssuesFunc != nil {
		return m.getAllIssuesFunc(ctx)
	}
	return []domain.Issue{}, nil
}

func (m *mockPipelineService) GetBranchesWithPipelines(ctx context.Context, limit int) ([]domain.BranchWithPipeline, error) {
	return []domain.BranchWithPipeline{}, nil
}

func (m *mockPipelineService) FilterBranchesByAuthor(branches []domain.BranchWithPipeline, gitlabUsername, githubUsername string) []domain.BranchWithPipeline {
	return branches
}

// TestHandleHealth tests the health check endpoint.
// Follows AAA (Arrange, Act, Assert) and FIRST principles.
func TestHandleHealth(t *testing.T) {
	// Arrange
	renderer := &mockRenderer{}
	logger := &mockLogger{}
	pipelineService := &mockPipelineService{}
	handler := NewHandler(renderer, logger, pipelineService, 3, 50, "", "")
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()

	// Act
	mux.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	expected := `{"status":"ok"}`
	if w.Body.String() != expected {
		t.Errorf("expected body %q, got %q", expected, w.Body.String())
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}
}

// TestHandleHealth_RenderError tests error handling in health endpoint.
func TestHandleHealth_RenderError(t *testing.T) {
	// Arrange
	renderer := &mockRenderer{healthErr: errors.New("render error")}
	logger := &mockLogger{}
	pipelineService := &mockPipelineService{}
	handler := NewHandler(renderer, logger, pipelineService, 3, 50, "", "")
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()

	// Act
	mux.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", w.Code)
	}

	if len(logger.messages) == 0 {
		t.Error("expected error to be logged")
	}
}

// TestHandleIndex tests the index page endpoint.
func TestHandleIndex(t *testing.T) {
	// Arrange
	renderer := &mockRenderer{}
	logger := &mockLogger{}
	pipelineService := &mockPipelineService{}
	handler := NewHandler(renderer, logger, pipelineService, 3, 50, "", "")
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	// Act
	mux.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "text/html" {
		t.Errorf("expected Content-Type text/html, got %s", contentType)
	}

	if !strings.Contains(w.Body.String(), "mock repositories") {
		t.Errorf("expected body to contain 'mock repositories', got %q", w.Body.String())
	}
}

// TestHandleIndex_RenderError tests error handling in index endpoint.
func TestHandleIndex_RenderError(t *testing.T) {
	// Arrange
	renderer := &mockRenderer{repositoriesErr: errors.New("render error")}
	logger := &mockLogger{}
	pipelineService := &mockPipelineService{}
	handler := NewHandler(renderer, logger, pipelineService, 3, 50, "", "")
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	// Act
	mux.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", w.Code)
	}

	if len(logger.messages) == 0 {
		t.Error("expected error to be logged")
	}
}

// TestHandlePipelines tests the pipelines page endpoint.
func TestHandlePipelines(t *testing.T) {
	// Arrange
	renderer := &mockRenderer{}
	logger := &mockLogger{}
	pipelineService := &mockPipelineService{
		getRecentPipelinesFunc: func(ctx context.Context, totalLimit int) ([]domain.Pipeline, error) {
			return []domain.Pipeline{
				{ID: "1", Repository: "test-repo", Status: domain.StatusSuccess},
			}, nil
		},
	}
	handler := NewHandler(renderer, logger, pipelineService, 3, 50, "", "")
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/pipelines", nil)
	w := httptest.NewRecorder()

	// Act
	mux.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if !strings.Contains(w.Body.String(), "mock recent pipelines") {
		t.Errorf("expected body to contain 'mock recent pipelines', got %q", w.Body.String())
	}
}

// TestHandlePipelines_ServiceError tests error handling when service fails.
func TestHandlePipelines_ServiceError(t *testing.T) {
	// Arrange
	renderer := &mockRenderer{}
	logger := &mockLogger{}
	pipelineService := &mockPipelineService{
		getRecentPipelinesFunc: func(ctx context.Context, totalLimit int) ([]domain.Pipeline, error) {
			return nil, errors.New("service error")
		},
	}
	handler := NewHandler(renderer, logger, pipelineService, 3, 50, "", "")
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/pipelines", nil)
	w := httptest.NewRecorder()

	// Act
	mux.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", w.Code)
	}

	if len(logger.messages) == 0 {
		t.Error("expected error to be logged")
	}
}

// TestNewHandler verifies handler construction.
func TestNewHandler(t *testing.T) {
	// Arrange
	renderer := &mockRenderer{}
	logger := &mockLogger{}
	pipelineService := &mockPipelineService{}

	// Act
	handler := NewHandler(renderer, logger, pipelineService, 3, 50, "", "")

	// Assert
	if handler == nil {
		t.Fatal("expected handler to be non-nil")
	}
	if handler.renderer == nil {
		t.Error("expected renderer to be set")
	}
	if handler.logger == nil {
		t.Error("expected logger to be set")
	}
	if handler.pipelineService == nil {
		t.Error("expected pipeline service to be set")
	}
}
