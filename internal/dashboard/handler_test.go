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
)

// mockRenderer is a test double for Renderer (follows FIRST - Independent).
type mockRenderer struct {
	indexErr             error
	healthErr            error
	pipelinesErr         error
	pipelinesJSONErr     error
	pipelinesGroupedErr  error
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

func (m *mockRenderer) RenderPipelinesJSON(w io.Writer, pipelines []domain.Pipeline) error {
	if m.pipelinesJSONErr != nil {
		return m.pipelinesJSONErr
	}
	_, err := w.Write([]byte(`{"pipelines":[]}`))
	return err
}

func (m *mockRenderer) RenderPipelinesGrouped(w io.Writer, pipelinesByWorkflow map[string][]domain.Pipeline) error {
	if m.pipelinesGroupedErr != nil {
		return m.pipelinesGroupedErr
	}
	_, err := w.Write([]byte("mock grouped pipelines"))
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
	getAllProjectsFunc        func(ctx context.Context) ([]domain.Project, error)
	getPipelinesByProjectFunc func(ctx context.Context, projectIDs []string) ([]domain.Pipeline, error)
	getLatestPipelinesFunc    func(ctx context.Context) ([]domain.Pipeline, error)
	groupPipelinesByWorkflowFunc func(pipelines []domain.Pipeline) map[string][]domain.Pipeline
	getPipelinesByWorkflowFunc func(ctx context.Context, projectID, workflowID string, limit int) ([]domain.Pipeline, error)
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

// TestHandleHealth tests the health check endpoint.
// Follows AAA (Arrange, Act, Assert) and FIRST principles.
func TestHandleHealth(t *testing.T) {
	// Arrange
	renderer := &mockRenderer{}
	logger := &mockLogger{}
	pipelineService := &mockPipelineService{}
	handler := NewHandler(renderer, logger, pipelineService)
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
	handler := NewHandler(renderer, logger, pipelineService)
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
	handler := NewHandler(renderer, logger, pipelineService)
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

	if !strings.Contains(w.Body.String(), "mock index") {
		t.Errorf("expected body to contain 'mock index', got %q", w.Body.String())
	}
}

// TestHandleIndex_RenderError tests error handling in index endpoint.
func TestHandleIndex_RenderError(t *testing.T) {
	// Arrange
	renderer := &mockRenderer{indexErr: errors.New("render error")}
	logger := &mockLogger{}
	pipelineService := &mockPipelineService{}
	handler := NewHandler(renderer, logger, pipelineService)
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
		getLatestPipelinesFunc: func(ctx context.Context) ([]domain.Pipeline, error) {
			return []domain.Pipeline{
				{ID: "1", Repository: "test-repo", Status: domain.StatusSuccess},
			}, nil
		},
	}
	handler := NewHandler(renderer, logger, pipelineService)
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

	if !strings.Contains(w.Body.String(), "mock pipelines") {
		t.Errorf("expected body to contain 'mock pipelines', got %q", w.Body.String())
	}
}

// TestHandlePipelines_ServiceError tests error handling when service fails.
func TestHandlePipelines_ServiceError(t *testing.T) {
	// Arrange
	renderer := &mockRenderer{}
	logger := &mockLogger{}
	pipelineService := &mockPipelineService{
		getLatestPipelinesFunc: func(ctx context.Context) ([]domain.Pipeline, error) {
			return nil, errors.New("service error")
		},
	}
	handler := NewHandler(renderer, logger, pipelineService)
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

// TestHandlePipelinesAPI tests the pipelines JSON API endpoint.
func TestHandlePipelinesAPI(t *testing.T) {
	// Arrange
	renderer := &mockRenderer{}
	logger := &mockLogger{}
	pipelineService := &mockPipelineService{
		getLatestPipelinesFunc: func(ctx context.Context) ([]domain.Pipeline, error) {
			return []domain.Pipeline{}, nil
		},
	}
	handler := NewHandler(renderer, logger, pipelineService)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/pipelines", nil)
	w := httptest.NewRecorder()

	// Act
	mux.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}
}

// TestNewHandler verifies handler construction.
func TestNewHandler(t *testing.T) {
	// Arrange
	renderer := &mockRenderer{}
	logger := &mockLogger{}
	pipelineService := &mockPipelineService{}

	// Act
	handler := NewHandler(renderer, logger, pipelineService)

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
