package dashboard

import (
	"context"
	"log"
	"net/http"

	"github.com/vilaca/ci-dashboard/internal/domain"
	"github.com/vilaca/ci-dashboard/internal/service"
)

// Handler handles HTTP requests for the dashboard.
// Each handler method has a Single Responsibility (SRP).
type Handler struct {
	renderer        Renderer
	logger          Logger
	pipelineService PipelineService
	runsPerRepo     int
	recentLimit     int
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
	GetRepositoriesWithRecentRuns(ctx context.Context, runsPerRepo int) ([]RepositoryWithRuns, error)
	GetRecentPipelines(ctx context.Context, totalLimit int) ([]domain.Pipeline, error)
	GetAllMergeRequests(ctx context.Context) ([]domain.MergeRequest, error)
	GetAllIssues(ctx context.Context) ([]domain.Issue, error)
}

// RepositoryWithRuns is imported from service package
type RepositoryWithRuns = service.RepositoryWithRuns

// NewHandler creates a new Handler with injected dependencies (Dependency Inversion Principle).
// This follows IoC (Inversion of Control) by accepting dependencies rather than creating them.
func NewHandler(renderer Renderer, logger Logger, pipelineService PipelineService, runsPerRepo, recentLimit int) *Handler {
	return &Handler{
		renderer:        renderer,
		logger:          logger,
		pipelineService: pipelineService,
		runsPerRepo:     runsPerRepo,
		recentLimit:     recentLimit,
	}
}

// RegisterRoutes registers all HTTP routes.
// Separated from constructor to follow Single Level of Abstraction Principle (SLAP).
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", h.handleRepositories)
	mux.HandleFunc("/api/health", h.handleHealth)
	mux.HandleFunc("/repository", h.handleRepositoryDetail)
	mux.HandleFunc("/pipelines", h.handleRecentPipelines)
	mux.HandleFunc("/pipelines/failed", h.handleFailedPipelines)
	mux.HandleFunc("/pipelines/grouped", h.handlePipelinesGrouped)
	mux.HandleFunc("/pipelines/workflow", h.handleWorkflowRuns)
	mux.HandleFunc("/mrs", h.handleMergeRequests)
	mux.HandleFunc("/issues", h.handleIssues)
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

// handleFailedPipelines serves only failed pipelines.
func (h *Handler) handleFailedPipelines(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")

	// Use GetRecentPipelines with larger limit to capture more history
	pipelines, err := h.pipelineService.GetRecentPipelines(r.Context(), h.recentLimit*2)
	if err != nil {
		h.logger.Printf("failed to get pipelines: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Filter for failed pipelines only
	var failedPipelines []domain.Pipeline
	for _, p := range pipelines {
		if p.Status == domain.StatusFailed {
			failedPipelines = append(failedPipelines, p)
		}
	}

	if err := h.renderer.RenderFailedPipelines(w, failedPipelines); err != nil {
		h.logger.Printf("failed to render failed pipelines: %v", err)
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

// handleRepositories serves the repositories page with recent runs.
func (h *Handler) handleRepositories(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")

	repositories, err := h.pipelineService.GetRepositoriesWithRecentRuns(r.Context(), h.runsPerRepo)
	if err != nil {
		h.logger.Printf("failed to get repositories: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if err := h.renderer.RenderRepositories(w, repositories); err != nil {
		h.logger.Printf("failed to render repositories: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

// handleRepositoryDetail serves the repository detail page.
// Query param: ?id=owner/repo or ?id=123
func (h *Handler) handleRepositoryDetail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")

	repositoryID := r.URL.Query().Get("id")
	if repositoryID == "" {
		http.Error(w, "Missing repository id parameter", http.StatusBadRequest)
		return
	}

	// Get all repositories to find the specific one
	repositories, err := h.pipelineService.GetRepositoriesWithRecentRuns(r.Context(), 50)
	if err != nil {
		h.logger.Printf("failed to get repositories: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Find the repository by ID
	var repository *RepositoryWithRuns
	for i := range repositories {
		if repositories[i].Project.ID == repositoryID {
			repository = &repositories[i]
			break
		}
	}

	if repository == nil {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	// Get MRs and Issues for this repository
	allMRs, err := h.pipelineService.GetAllMergeRequests(r.Context())
	if err != nil {
		h.logger.Printf("failed to get merge requests: %v", err)
		// Continue even if MRs fail
		allMRs = []domain.MergeRequest{}
	}

	allIssues, err := h.pipelineService.GetAllIssues(r.Context())
	if err != nil {
		h.logger.Printf("failed to get issues: %v", err)
		// Continue even if Issues fail
		allIssues = []domain.Issue{}
	}

	// Filter MRs for this repository
	var repoMRs []domain.MergeRequest
	for _, mr := range allMRs {
		if mr.ProjectID == repositoryID {
			repoMRs = append(repoMRs, mr)
		}
	}

	// Filter Issues for this repository
	var repoIssues []domain.Issue
	for _, issue := range allIssues {
		if issue.ProjectID == repositoryID {
			repoIssues = append(repoIssues, issue)
		}
	}

	if err := h.renderer.RenderRepositoryDetail(w, *repository, repoMRs, repoIssues); err != nil {
		h.logger.Printf("failed to render repository detail: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

// handleRecentPipelines serves the recent pipelines page.
func (h *Handler) handleRecentPipelines(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")

	pipelines, err := h.pipelineService.GetRecentPipelines(r.Context(), h.recentLimit)
	if err != nil {
		h.logger.Printf("failed to get recent pipelines: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if err := h.renderer.RenderRecentPipelines(w, pipelines); err != nil {
		h.logger.Printf("failed to render recent pipelines: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}


// handleMergeRequests serves the merge requests/pull requests page.
func (h *Handler) handleMergeRequests(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")

	mrs, err := h.pipelineService.GetAllMergeRequests(r.Context())
	if err != nil {
		h.logger.Printf("failed to get merge requests: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if err := h.renderer.RenderMergeRequests(w, mrs); err != nil {
		h.logger.Printf("failed to render merge requests: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

// handleIssues serves the issues page.
func (h *Handler) handleIssues(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")

	issues, err := h.pipelineService.GetAllIssues(r.Context())
	if err != nil {
		h.logger.Printf("failed to get issues: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if err := h.renderer.RenderIssues(w, issues); err != nil {
		h.logger.Printf("failed to render issues: %v", err)
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
