package dashboard

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vilaca/ci-dashboard/internal/domain"
	"github.com/vilaca/ci-dashboard/internal/service"
)

// avatarCacheEntry stores avatar data with expiration time
type avatarCacheEntry struct {
	data      []byte
	expiresAt time.Time
}

// Handler handles HTTP requests for the dashboard.
// Each handler method has a Single Responsibility (SRP).
type Handler struct {
	renderer            Renderer
	logger              Logger
	pipelineService     PipelineService
	runsPerRepo         int
	recentLimit         int
	uiRefreshInterval   int
	gitlabCurrentUser   string
	githubCurrentUser   string
	avatarCache         map[string]*avatarCacheEntry // platform:username -> cached data with TTL
	avatarCacheMu       sync.RWMutex
}

// Logger interface for logging operations (Interface Segregation Principle).
type Logger interface {
	Printf(format string, v ...interface{})
}

// PipelineService interface for pipeline operations (Dependency Inversion Principle).
type PipelineService interface {
	GetAllProjects(ctx context.Context) ([]domain.Project, error)
	GetPipelinesByProject(ctx context.Context, projectIDs []string) ([]domain.Pipeline, error)
	GetPipelinesForProject(ctx context.Context, projectID string, limit int) ([]domain.Pipeline, error)
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

// HandlerConfig holds configuration for creating a new Handler
type HandlerConfig struct {
	Renderer          Renderer
	Logger            Logger
	PipelineService   PipelineService
	RunsPerRepo       int
	RecentLimit       int
	UIRefreshInterval int
	GitLabUser        string
	GitHubUser        string
}

// NewHandler creates a new Handler with injected dependencies (Dependency Inversion Principle).
// This follows IoC (Inversion of Control) by accepting dependencies rather than creating them.
func NewHandler(cfg HandlerConfig) *Handler {
	h := &Handler{
		renderer:          cfg.Renderer,
		logger:            cfg.Logger,
		pipelineService:   cfg.PipelineService,
		runsPerRepo:       cfg.RunsPerRepo,
		recentLimit:       cfg.RecentLimit,
		uiRefreshInterval: cfg.UIRefreshInterval,
		gitlabCurrentUser: cfg.GitLabUser,
		githubCurrentUser: cfg.GitHubUser,
		avatarCache:       make(map[string]*avatarCacheEntry),
	}

	// Start background cleanup goroutine for avatar cache (runs every hour)
	go h.cleanupAvatarCache(1 * time.Hour)

	return h
}

// RegisterRoutes registers all HTTP routes.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", h.handleRepositories)
	mux.HandleFunc("/api/health", h.handleHealth)
	mux.HandleFunc("/api/repositories", h.handleRepositoriesBulk)
	mux.HandleFunc("/api/repository-detail", h.handleRepositoryDetailAPI)
	mux.HandleFunc("/api/avatar/", h.handleAvatar)
	mux.HandleFunc("/repository", h.handleRepositoryDetail)
}

// handleIndex serves the main dashboard page.
// handleHealth serves the health check endpoint.
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if err := h.renderer.RenderHealth(w); err != nil {
		h.logger.Printf("failed to render health: %v", err)
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
	if err := h.renderer.RenderRepositoriesSkeleton(w, userProfiles, h.uiRefreshInterval); err != nil {
		h.logger.Printf("failed to render repositories skeleton: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

// RepositoryDefaultBranch holds repository info with default branch details.
type RepositoryDefaultBranch struct {
	Project        domain.Project   `json:"Project"`
	DefaultBranch  *domain.Branch   `json:"DefaultBranch"`
	Pipeline       *domain.Pipeline `json:"Pipeline"`
	BranchCount    int              `json:"BranchCount"`
	OpenMRCount    int              `json:"OpenMRCount"`
	DraftMRCount   int              `json:"DraftMRCount"`
	ReviewingCount int              `json:"ReviewingCount"`
}

// handleRepositoriesBulk returns repositories as paginated JSON (cache only, no API calls).
func (h *Handler) handleRepositoriesBulk(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")

	page := 1
	limit := 50
	if pageParam := r.URL.Query().Get("page"); pageParam != "" {
		if p, err := strconv.Atoi(pageParam); err == nil && p > 0 {
			page = p
		}
	}
	if limitParam := r.URL.Query().Get("limit"); limitParam != "" {
		if l, err := strconv.Atoi(limitParam); err == nil && l > 0 && l <= 10000 {
			limit = l
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	projects, err := h.pipelineService.GetAllProjects(ctx)
	if err != nil {
		h.logger.Printf("[BulkAPI] ERROR: %v", err)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"repositories": []interface{}{},
			"pagination": map[string]interface{}{
				"page": 1, "limit": 50, "total": 0, "totalPages": 0, "hasNext": false,
			},
		})
		return
	}

	results := make([]RepositoryDefaultBranch, 0, len(projects))
	for _, project := range projects {
		// Fetch cached data for this project
		defaultBranch, pipeline, branchCount, _ := h.pipelineService.GetDefaultBranchForProject(ctx, project)

		// Fetch cached MRs for this project
		mrs, _ := h.pipelineService.GetMergeRequestsForProject(ctx, project)

		// Count open and draft MRs
		openMRCount := 0
		draftMRCount := 0
		reviewingCount := 0
		for _, mr := range mrs {
			if mr.State == "opened" {
				openMRCount++
				if mr.IsDraft {
					draftMRCount++
				}
				// Check if current user is a reviewer
				for _, reviewer := range mr.Reviewers {
					if reviewer == h.gitlabCurrentUser || reviewer == h.githubCurrentUser {
						reviewingCount++
						break
					}
				}
			}
		}

		results = append(results, RepositoryDefaultBranch{
			Project:        project,
			DefaultBranch:  defaultBranch,
			Pipeline:       pipeline,
			BranchCount:    branchCount,
			OpenMRCount:    openMRCount,
			DraftMRCount:   draftMRCount,
			ReviewingCount: reviewingCount,
		})
	}

	startIndex := (page - 1) * limit
	endIndex := startIndex + limit
	if startIndex >= len(results) {
		startIndex = len(results)
	}
	if endIndex > len(results) {
		endIndex = len(results)
	}

	paginatedResults := results[startIndex:endIndex]
	totalCount := len(results)
	totalPages := (totalCount + limit - 1) / limit
	hasNext := page < totalPages

	response := map[string]interface{}{
		"repositories": paginatedResults,
		"pagination": map[string]interface{}{
			"page":       page,
			"limit":      limit,
			"total":      totalCount,
			"totalPages": totalPages,
			"hasNext":    hasNext,
		},
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Printf("failed to encode repositories: %v", err)
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

	h.logger.Printf("[RepositoryDetail] Fetching data for repository ID: %s", repositoryID)

	// Get all projects to find the specific one (from cache)
	projects, err := h.pipelineService.GetAllProjects(r.Context())
	if err != nil {
		h.logger.Printf("[RepositoryDetail] ERROR: failed to get projects: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	h.logger.Printf("[RepositoryDetail] Found %d projects in cache", len(projects))

	// Find the project by ID
	var project *domain.Project
	for i := range projects {
		if projects[i].ID == repositoryID {
			project = &projects[i]
			break
		}
	}

	if project == nil {
		h.logger.Printf("[RepositoryDetail] ERROR: Repository not found with ID: %s", repositoryID)
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	h.logger.Printf("[RepositoryDetail] Found project: %s (platform: %s)", project.Name, project.Platform)

	// Get pipelines for this specific project (from cache)
	pipelines, err := h.pipelineService.GetPipelinesForProject(r.Context(), repositoryID, 50)
	if err != nil {
		h.logger.Printf("[RepositoryDetail] ERROR: failed to get pipelines for project %s: %v", repositoryID, err)
		// Continue with empty pipelines
		pipelines = []domain.Pipeline{}
	} else {
		h.logger.Printf("[RepositoryDetail] Found %d pipelines for project %s", len(pipelines), repositoryID)
	}

	// Fill in repository name from project
	for i := range pipelines {
		if pipelines[i].Repository == "" || pipelines[i].Repository == project.ID {
			pipelines[i].Repository = project.Name
		}
	}

	repository := RepositoryWithRuns{
		Project: *project,
		Runs:    pipelines,
	}

	// Get MRs and Issues for this repository (from cache)
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

	// Cache avatars for users in MRs and Issues
	var wg sync.WaitGroup
	seen := make(map[string]bool)

	// Cache MR authors (all from same repository/platform)
	for _, mr := range repoMRs {
		key := project.Platform + ":" + mr.Author
		if !seen[key] && mr.Author != "" {
			seen[key] = true
			wg.Add(1)
			go func(platform, author string) {
				defer wg.Done()
				h.cacheAvatar(platform, author, "", "")
			}(project.Platform, mr.Author)
		}
	}

	// Cache Issue authors (all from same repository/platform)
	for _, issue := range repoIssues {
		key := project.Platform + ":" + issue.Author
		if !seen[key] && issue.Author != "" {
			seen[key] = true
			wg.Add(1)
			go func(platform, author string) {
				defer wg.Done()
				h.cacheAvatar(platform, author, "", "")
			}(project.Platform, issue.Author)
		}
	}

	wg.Wait()

	h.logger.Printf("[RepositoryDetail] Final counts: %d runs, %d MRs, %d issues", len(repository.Runs), len(repoMRs), len(repoIssues))

	// Render to a buffer to get HTML string
	var buf strings.Builder
	if err := h.renderer.RenderRepositoryDetail(&buf, repository, repoMRs, repoIssues); err != nil {
		h.logger.Printf("[RepositoryDetail] ERROR: failed to render: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	h.logger.Printf("[RepositoryDetail] Successfully rendered %d bytes of HTML", buf.Len())

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
	entry, found := h.avatarCache[cacheKey]
	h.avatarCacheMu.RUnlock()

	if !found || entry == nil {
		// Avatar not cached yet - return placeholder or 404
		http.Error(w, "Avatar not cached yet", http.StatusNotFound)
		return
	}

	// Check if expired
	if time.Now().After(entry.expiresAt) {
		// Expired - remove from cache and return 404
		h.avatarCacheMu.Lock()
		delete(h.avatarCache, cacheKey)
		h.avatarCacheMu.Unlock()
		http.Error(w, "Avatar expired", http.StatusNotFound)
		return
	}

	imageData := entry.data

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

	// Store in cache with 24 hour TTL
	h.avatarCacheMu.Lock()
	h.avatarCache[cacheKey] = &avatarCacheEntry{
		data:      imageData,
		expiresAt: time.Now().Add(24 * time.Hour),
	}
	h.avatarCacheMu.Unlock()

	h.logger.Printf("cached avatar for %s (%s)", username, platform)
}

// cleanupAvatarCache periodically removes expired entries from avatar cache
func (h *Handler) cleanupAvatarCache(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		h.avatarCacheMu.Lock()
		now := time.Now()
		removed := 0
		for key, entry := range h.avatarCache {
			if entry != nil && now.After(entry.expiresAt) {
				delete(h.avatarCache, key)
				removed++
			}
		}
		h.avatarCacheMu.Unlock()

		if removed > 0 {
			h.logger.Printf("cleaned up %d expired avatar(s) from cache", removed)
		}
	}
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
