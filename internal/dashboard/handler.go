package dashboard

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/vilaca/ci-dashboard/internal/domain"
	"github.com/vilaca/ci-dashboard/internal/service"
)

// Handler handles HTTP requests for the dashboard.
// Each handler method has a Single Responsibility (SRP).
type Handler struct {
	renderer          Renderer
	logger            Logger
	pipelineService   PipelineService
	runsPerRepo       int
	recentLimit       int
	gitlabCurrentUser string
	githubCurrentUser string
	avatarCache       map[string][]byte // platform:username -> image data
	avatarCacheMu     sync.RWMutex
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
	GetMergeRequestsForProject(ctx context.Context, project domain.Project) ([]domain.MergeRequest, error)
	GetAllIssues(ctx context.Context) ([]domain.Issue, error)
	GetBranchesWithPipelines(ctx context.Context, limit int) ([]domain.BranchWithPipeline, error)
	GetBranchesForProject(ctx context.Context, project domain.Project, limit int) ([]domain.BranchWithPipeline, error)
	GetDefaultBranchForProject(ctx context.Context, project domain.Project) (*domain.Branch, *domain.Pipeline, int, error)
	FilterBranchesByAuthor(branches []domain.BranchWithPipeline, gitlabUsername, githubUsername string) []domain.BranchWithPipeline
	GetUserProfiles(ctx context.Context) ([]domain.UserProfile, error)
	GetProjectsPageByPlatform(ctx context.Context, platform string, page int) ([]domain.Project, bool, error)
	GetTotalProjectCount(ctx context.Context) (int, error)
}

// RepositoryWithRuns is imported from service package
type RepositoryWithRuns = service.RepositoryWithRuns

// NewHandler creates a new Handler with injected dependencies (Dependency Inversion Principle).
// This follows IoC (Inversion of Control) by accepting dependencies rather than creating them.
func NewHandler(renderer Renderer, logger Logger, pipelineService PipelineService, runsPerRepo, recentLimit int, gitlabCurrentUser, githubCurrentUser string) *Handler {
	return &Handler{
		renderer:          renderer,
		logger:            logger,
		pipelineService:   pipelineService,
		runsPerRepo:       runsPerRepo,
		recentLimit:       recentLimit,
		gitlabCurrentUser: gitlabCurrentUser,
		githubCurrentUser: githubCurrentUser,
		avatarCache:       make(map[string][]byte),
	}
}

// RegisterRoutes registers all HTTP routes.
// Separated from constructor to follow Single Level of Abstraction Principle (SLAP).
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", h.handleRepositories)
	mux.HandleFunc("/api/health", h.handleHealth)
	mux.HandleFunc("/api/stream/repositories", h.handleStreamRepositories)
	mux.HandleFunc("/api/repository-detail", h.handleRepositoryDetailAPI)
	mux.HandleFunc("/api/avatar/", h.handleAvatar)
	mux.HandleFunc("/repository", h.handleRepositoryDetail)
	mux.HandleFunc("/pipelines", h.handleRecentPipelines)
	mux.HandleFunc("/pipelines/failed", h.handleFailedPipelines)
	mux.HandleFunc("/pipelines/grouped", h.handlePipelinesGrouped)
	mux.HandleFunc("/pipelines/workflow", h.handleWorkflowRuns)
	mux.HandleFunc("/branches", h.handleBranches)
	mux.HandleFunc("/your-branches", h.handleYourBranches)
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

// handleRepositories serves the repositories page with progressive loading.
func (h *Handler) handleRepositories(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")

	// Fetch user profiles from all platforms
	userProfiles, err := h.pipelineService.GetUserProfiles(r.Context())
	if err != nil {
		h.logger.Printf("failed to get user profiles: %v", err)
		// Continue without user profiles
		userProfiles = []domain.UserProfile{}
	}

	// Cache avatars before rendering (wait for completion to avoid 404s)
	var wg sync.WaitGroup
	for _, profile := range userProfiles {
		wg.Add(1)
		go func(p domain.UserProfile) {
			defer wg.Done()
			h.cacheAvatar(p.Platform, p.Username, p.Email, p.AvatarURL)
		}(profile)
	}
	wg.Wait()

	// Render empty page skeleton with user profiles
	if err := h.renderer.RenderRepositoriesSkeleton(w, userProfiles); err != nil {
		h.logger.Printf("failed to render repositories skeleton: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

// RepositoryDefaultBranch holds repository info with default branch details.
type RepositoryDefaultBranch struct {
	Project       domain.Project         `json:"Project"`
	DefaultBranch *domain.Branch         `json:"DefaultBranch"`
	Pipeline      *domain.Pipeline       `json:"Pipeline"`
	BranchCount   int                    `json:"BranchCount"`
	OpenMRCount   int                    `json:"OpenMRCount"`
	DraftMRCount  int                    `json:"DraftMRCount"`
}

// handleStreamRepositories streams repository data via Server-Sent Events.
// Fetches projects page-by-page and processes each page before fetching the next.
func (h *Handler) handleStreamRepositories(w http.ResponseWriter, r *http.Request) {
	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Fetch total count first
	totalCount, err := h.pipelineService.GetTotalProjectCount(r.Context())
	if err != nil {
		h.logger.Printf("failed to get total project count: %v", err)
		// Continue without total count
		totalCount = 0
	}

	// Send total count event
	if totalCount > 0 {
		fmt.Fprintf(w, "event: total\ndata: %d\n\n", totalCount)
		flusher.Flush()
	}

	// Channel to collect repository data from both platforms
	type repoResult struct {
		data RepositoryDefaultBranch
		err  error
	}
	resultChan := make(chan repoResult, 100)

	// WaitGroup to track when both platforms are done
	var wg sync.WaitGroup

	// Helper function to process a single project
	processProject := func(proj domain.Project) repoResult {
		// Get only default branch and its pipeline (optimized to reduce cache misses)
		defaultBranch, defaultPipeline, branchCount, err := h.pipelineService.GetDefaultBranchForProject(r.Context(), proj)
		if err != nil {
			h.logger.Printf("failed to get default branch for %s: %v", proj.Name, err)
		}

		// Get MRs for this project
		openMRCount := 0
		draftMRCount := 0
		mrs, err := h.pipelineService.GetMergeRequestsForProject(r.Context(), proj)
		if err != nil {
			h.logger.Printf("failed to get MRs for %s: %v", proj.Name, err)
		} else {
			for _, mr := range mrs {
				if mr.State == "opened" || mr.State == "open" {
					openMRCount++
					if mr.IsDraft {
						draftMRCount++
					}
				}
			}
		}

		return repoResult{
			data: RepositoryDefaultBranch{
				Project:       proj,
				DefaultBranch: defaultBranch,
				Pipeline:      defaultPipeline,
				BranchCount:   branchCount,
				OpenMRCount:   openMRCount,
				DraftMRCount:  draftMRCount,
			},
			err: nil,
		}
	}

	// Process GitLab repositories page-by-page
	wg.Add(1)
	go func() {
		defer wg.Done()
		page := 1
		for {
			select {
			case <-r.Context().Done():
				return
			default:
			}

			// Fetch one page of projects
			projects, hasNext, err := h.pipelineService.GetProjectsPageByPlatform(r.Context(), "gitlab", page)
			if err != nil {
				h.logger.Printf("failed to get GitLab projects page %d: %v", page, err)
				return
			}

			// Process each project in this page
			for _, proj := range projects {
				select {
				case <-r.Context().Done():
					return
				default:
				}

				result := processProject(proj)
				resultChan <- result
			}

			// Move to next page or stop
			if !hasNext {
				break
			}
			page++
		}
	}()

	// Process GitHub repositories page-by-page
	wg.Add(1)
	go func() {
		defer wg.Done()
		page := 1
		for {
			select {
			case <-r.Context().Done():
				return
			default:
			}

			// Fetch one page of projects
			projects, hasNext, err := h.pipelineService.GetProjectsPageByPlatform(r.Context(), "github", page)
			if err != nil {
				h.logger.Printf("failed to get GitHub projects page %d: %v", page, err)
				return
			}

			// Process each project in this page
			for _, proj := range projects {
				select {
				case <-r.Context().Done():
					return
				default:
				}

				result := processProject(proj)
				resultChan <- result
			}

			// Move to next page or stop
			if !hasNext {
				break
			}
			page++
		}
	}()

	// Close channel when both goroutines are done
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Stream results as they arrive from either platform
	streamed := 0
	for {
		select {
		case <-r.Context().Done():
			return
		case result, ok := <-resultChan:
			if !ok {
				// Channel closed - both platforms done
				fmt.Fprintf(w, "event: done\ndata: complete\n\n")
				flusher.Flush()
				h.logger.Printf("streaming complete - sent %d repositories", streamed)
				return
			}

			// Send as JSON
			data, err := json.Marshal(result.data)
			if err != nil {
				h.logger.Printf("failed to marshal repository: %v", err)
				continue
			}

			fmt.Fprintf(w, "event: repository\ndata: %s\n\n", string(data))
			flusher.Flush()
			streamed++
		}
	}
}

// handleRepositoryDetail serves the repository detail page with progressive loading.
// Query param: ?id=owner/repo or ?id=123
func (h *Handler) handleRepositoryDetail(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")

	repositoryID := r.URL.Query().Get("id")
	if repositoryID == "" {
		http.Error(w, "Missing repository id parameter", http.StatusBadRequest)
		return
	}

	// Render skeleton immediately
	if err := h.renderer.RenderRepositoryDetailSkeleton(w, repositoryID); err != nil {
		h.logger.Printf("failed to render repository detail skeleton: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

// handleRepositoryDetailAPI serves the repository detail data as JSON.
// Query param: ?id=owner/repo or ?id=123
func (h *Handler) handleRepositoryDetailAPI(w http.ResponseWriter, r *http.Request) {
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

	// Render to a buffer to get HTML string
	var buf strings.Builder
	if err := h.renderer.RenderRepositoryDetail(&buf, *repository, repoMRs, repoIssues); err != nil {
		h.logger.Printf("failed to render repository detail: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Return as JSON
	w.Header().Set("Content-Type", "application/json")
	response := map[string]string{
		"html": buf.String(),
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Printf("failed to encode response: %v", err)
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

// handleBranches serves the branches page showing all branches.
func (h *Handler) handleBranches(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")

	branches, err := h.pipelineService.GetBranchesWithPipelines(r.Context(), 50)
	if err != nil {
		h.logger.Printf("failed to get branches: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if err := h.renderer.RenderBranches(w, branches); err != nil {
		h.logger.Printf("failed to render branches: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

// handleYourBranches serves branches filtered by current user.
func (h *Handler) handleYourBranches(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")

	branches, err := h.pipelineService.GetBranchesWithPipelines(r.Context(), 50)
	if err != nil {
		h.logger.Printf("failed to get branches: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Filter by current user
	filteredBranches := h.pipelineService.FilterBranchesByAuthor(branches, h.gitlabCurrentUser, h.githubCurrentUser)

	if err := h.renderer.RenderYourBranches(w, filteredBranches, h.gitlabCurrentUser, h.githubCurrentUser); err != nil {
		h.logger.Printf("failed to render your branches: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

// handleAvatar serves cached avatar images.
// URL pattern: /api/avatar/{platform}/{username}
func (h *Handler) handleAvatar(w http.ResponseWriter, r *http.Request) {
	// Parse path: /api/avatar/{platform}/{username}
	path := strings.TrimPrefix(r.URL.Path, "/api/avatar/")
	parts := strings.Split(path, "/")

	if len(parts) != 2 {
		http.Error(w, "Invalid avatar path", http.StatusBadRequest)
		return
	}

	platform := parts[0]
	username := parts[1]
	cacheKey := platform + ":" + username

	// Get from cache
	h.avatarCacheMu.RLock()
	imageData, found := h.avatarCache[cacheKey]
	h.avatarCacheMu.RUnlock()

	if !found {
		// Avatar not cached yet - return placeholder or 404
		http.Error(w, "Avatar not cached yet", http.StatusNotFound)
		return
	}

	// Detect image type from content
	contentType := "image/png"
	if len(imageData) > 3 {
		// Check for JPEG magic bytes
		if imageData[0] == 0xFF && imageData[1] == 0xD8 && imageData[2] == 0xFF {
			contentType = "image/jpeg"
		} else if len(imageData) > 8 && imageData[0] == 0x89 && imageData[1] == 0x50 && imageData[2] == 0x4E && imageData[3] == 0x47 {
			contentType = "image/png"
		} else if len(imageData) > 5 && string(imageData[0:6]) == "GIF89a" || string(imageData[0:6]) == "GIF87a" {
			contentType = "image/gif"
		}
	}

	// Serve the image
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Write(imageData)
}

// cacheAvatar stores an avatar in the cache after downloading it.
func (h *Handler) cacheAvatar(platform, username, email, avatarURL string) {
	if avatarURL == "" {
		return
	}

	cacheKey := platform + ":" + username

	// Check if already cached
	h.avatarCacheMu.RLock()
	_, exists := h.avatarCache[cacheKey]
	h.avatarCacheMu.RUnlock()

	if exists {
		return
	}

	// GitLab /uploads/ URLs require web session authentication - use Gravatar fallback
	if platform == "gitlab" && strings.Contains(avatarURL, "/uploads/") {
		if email == "" {
			h.logger.Printf("skipping gitlab uploaded avatar for %s (no email for Gravatar fallback)", username)
			return
		}
		avatarURL = h.getGravatarURL(email)
		h.logger.Printf("using Gravatar fallback for %s", username)
	}

	// Download avatar
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, avatarURL, nil)
	if err != nil {
		h.logger.Printf("failed to create avatar request for %s: %v", username, err)
		return
	}

	// GitHub avatars are public CDN URLs, no auth needed
	// GitLab Gravatar URLs are also public, no auth needed

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Follow redirects but limit to 10
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		h.logger.Printf("failed to fetch avatar for %s: %v", username, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		h.logger.Printf("avatar fetch failed for %s (%s) with status %d for URL: %s", username, platform, resp.StatusCode, avatarURL)
		return
	}

	// Read image data
	imageData, err := io.ReadAll(resp.Body)
	if err != nil {
		h.logger.Printf("failed to read avatar data for %s: %v", username, err)
		return
	}

	// Store in cache
	h.avatarCacheMu.Lock()
	h.avatarCache[cacheKey] = imageData
	h.avatarCacheMu.Unlock()

	h.logger.Printf("cached avatar for %s (%s)", username, platform)
}

// getGravatarURL generates a Gravatar URL from an email address.
func (h *Handler) getGravatarURL(email string) string {
	// MD5 hash of lowercase trimmed email
	email = strings.ToLower(strings.TrimSpace(email))
	hash := fmt.Sprintf("%x", md5.Sum([]byte(email)))
	return fmt.Sprintf("https://www.gravatar.com/avatar/%s?s=80&d=identicon", hash)
}

// StdLogger wraps the standard log package to implement Logger interface.
type StdLogger struct{}

func NewStdLogger() *StdLogger {
	return &StdLogger{}
}

func (l *StdLogger) Printf(format string, v ...interface{}) {
	log.Printf(format, v...)
}
