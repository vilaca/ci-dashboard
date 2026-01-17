package dashboard

import (
	"context"
	"log"
	"net/http"

	"github.com/vilaca/ci-dashboard/internal/domain"
)

// Handler handles HTTP requests for the dashboard.
// Each handler method has a Single Responsibility (SRP).
type Handler struct {
	renderer        Renderer
	logger          Logger
	pipelineService PipelineService
}

// Logger interface for logging operations (Interface Segregation Principle).
type Logger interface {
	Printf(format string, v ...interface{})
}

// PipelineService interface for pipeline operations (Dependency Inversion Principle).
type PipelineService interface {
	GetAllProjects(ctx context.Context) ([]domain.Project, error)
	GetPipelinesByProject(ctx context.Context, projectIDs []string) ([]domain.Pipeline, error)
	GetLatestPipelines(ctx context.Context) ([]domain.Pipeline, error)
	GroupPipelinesByWorkflow(pipelines []domain.Pipeline) map[string][]domain.Pipeline
	GetPipelinesByWorkflow(ctx context.Context, projectID, workflowID string, limit int) ([]domain.Pipeline, error)
}

// NewHandler creates a new Handler with injected dependencies (Dependency Inversion Principle).
// This follows IoC (Inversion of Control) by accepting dependencies rather than creating them.
func NewHandler(renderer Renderer, logger Logger, pipelineService PipelineService) *Handler {
	return &Handler{
		renderer:        renderer,
		logger:          logger,
		pipelineService: pipelineService,
	}
}

// RegisterRoutes registers all HTTP routes.
// Separated from constructor to follow Single Level of Abstraction Principle (SLAP).
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", h.handleIndex)
	mux.HandleFunc("/api/health", h.handleHealth)
	mux.HandleFunc("/pipelines", h.handlePipelines)
	mux.HandleFunc("/pipelines/grouped", h.handlePipelinesGrouped)
	mux.HandleFunc("/pipelines/workflow", h.handleWorkflowRuns)
	mux.HandleFunc("/api/pipelines", h.handlePipelinesAPI)
}

// handleIndex serves the main dashboard page.
func (h *Handler) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")

	if err := h.renderer.RenderIndex(w); err != nil {
		h.logger.Printf("failed to render index: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

// handleHealth serves the health check endpoint.
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if err := h.renderer.RenderHealth(w); err != nil {
		h.logger.Printf("failed to render health: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

// handlePipelines serves the pipelines page (HTML).
func (h *Handler) handlePipelines(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")

	pipelines, err := h.pipelineService.GetLatestPipelines(r.Context())
	if err != nil {
		h.logger.Printf("failed to get pipelines: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if err := h.renderer.RenderPipelines(w, pipelines); err != nil {
		h.logger.Printf("failed to render pipelines: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

// handlePipelinesAPI serves the pipelines API endpoint (JSON).
func (h *Handler) handlePipelinesAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	pipelines, err := h.pipelineService.GetLatestPipelines(r.Context())
	if err != nil {
		h.logger.Printf("failed to get pipelines: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if err := h.renderer.RenderPipelinesJSON(w, pipelines); err != nil {
		h.logger.Printf("failed to render pipelines JSON: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

// handlePipelinesGrouped serves pipelines grouped by workflow.
func (h *Handler) handlePipelinesGrouped(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")

	pipelines, err := h.pipelineService.GetLatestPipelines(r.Context())
	if err != nil {
		h.logger.Printf("failed to get pipelines: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	grouped := h.pipelineService.GroupPipelinesByWorkflow(pipelines)

	if err := h.renderer.RenderPipelinesGrouped(w, grouped); err != nil {
		h.logger.Printf("failed to render grouped pipelines: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

// handleWorkflowRuns serves runs for a specific workflow.
// Query params: ?project=owner/repo&workflow=123
func (h *Handler) handleWorkflowRuns(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")

	projectID := r.URL.Query().Get("project")
	workflowID := r.URL.Query().Get("workflow")

	if projectID == "" || workflowID == "" {
		http.Error(w, "Missing project or workflow parameter", http.StatusBadRequest)
		return
	}

	pipelines, err := h.pipelineService.GetPipelinesByWorkflow(r.Context(), projectID, workflowID, 50)
	if err != nil {
		h.logger.Printf("failed to get workflow runs: %v", err)
		http.Error(w, "Workflow not found", http.StatusNotFound)
		return
	}

	if err := h.renderer.RenderPipelines(w, pipelines); err != nil {
		h.logger.Printf("failed to render workflow runs: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

// StdLogger wraps the standard log package to implement Logger interface.
type StdLogger struct{}

func NewStdLogger() *StdLogger {
	return &StdLogger{}
}

func (l *StdLogger) Printf(format string, v ...interface{}) {
	log.Printf(format, v...)
}
