package service

import (
	"context"
	"sync"
	"time"

	"github.com/vilaca/ci-dashboard/internal/domain"
)

// BackgroundRefresher pre-fetches and periodically refreshes data to warm up caches.
// Follows Single Responsibility Principle - only handles background data fetching.
type BackgroundRefresher struct {
	pipelineService *PipelineService
	refreshInterval time.Duration
	logger          Logger
	stopChan        chan struct{}
	wg              sync.WaitGroup
	mu              sync.Mutex
	running         bool
}

// Logger interface for logging operations.
type Logger interface {
	Printf(format string, v ...interface{})
}

// NewBackgroundRefresher creates a new background refresher.
// Follows Dependency Injection - accepts dependencies via constructor.
func NewBackgroundRefresher(pipelineService *PipelineService, refreshInterval time.Duration, logger Logger) *BackgroundRefresher {
	return &BackgroundRefresher{
		pipelineService: pipelineService,
		refreshInterval: refreshInterval,
		logger:          logger,
		stopChan:        make(chan struct{}),
	}
}

// Start begins periodic background data refreshing.
// Non-blocking - launches goroutine and returns immediately.
// The cache file is loaded immediately and used to pre-populate in-memory caches.
func (r *BackgroundRefresher) Start() {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return
	}
	r.running = true
	r.mu.Unlock()

	r.logger.Printf("Background refresher: Starting with %v interval", r.refreshInterval)

	// Start background refresh goroutine (only periodic refreshes)
	r.wg.Add(1)
	go r.refreshLoop()
}

// Stop gracefully stops the background refresher.
func (r *BackgroundRefresher) Stop() {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return
	}
	r.running = false
	r.mu.Unlock()

	r.logger.Printf("Background refresher: Stopping...")
	close(r.stopChan)
	r.wg.Wait()
	r.logger.Printf("Background refresher: Stopped")
}

// refreshLoop performs periodic data refreshes.
// Performs an initial fetch in the background after a short delay.
func (r *BackgroundRefresher) refreshLoop() {
	defer r.wg.Done()

	// Perform initial fetch after 2 seconds (allows server to fully start)
	// This runs in background, not blocking the server
	time.Sleep(2 * time.Second)
	r.logger.Printf("Background refresher: Performing initial background fetch...")
	r.refreshData()

	// Setup periodic refresh ticker
	ticker := time.NewTicker(r.refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.logger.Printf("Background refresher: Performing periodic refresh...")
			r.refreshData()
		case <-r.stopChan:
			return
		}
	}
}

// refreshData fetches all key data to warm up caches.
// This triggers force-fetch on all clients to populate their stale caches.
func (r *BackgroundRefresher) refreshData() {
	ctx := context.Background()
	startTime := time.Now()

	// Force refresh all client caches
	// This bypasses the cache-only reads and actually fetches from APIs
	r.logger.Printf("Background refresher: Force-refreshing all client caches...")
	if err := r.pipelineService.ForceRefreshAllCaches(ctx); err != nil {
		r.logger.Printf("Background refresher: Failed to force-refresh caches: %v", err)
		return
	}

	// Now fetch projects (will come from freshly populated cache)
	projects, err := r.pipelineService.GetAllProjects(ctx)
	if err != nil {
		r.logger.Printf("Background refresher: Failed to fetch projects: %v", err)
		return
	}
	projectCount := len(projects)

	// Fetch repositories with recent runs - this sorts by most recent activity
	// This ensures we prioritize active repositories (fetched in order of most recent commits)
	// Fetch 50 pipelines per repo to populate cache for repository detail pages
	reposWithRuns, err := r.pipelineService.GetRepositoriesWithRecentRuns(ctx, 50)
	if err != nil {
		r.logger.Printf("Background refresher: Failed to fetch repositories with runs: %v", err)
	}
	// GetRepositoriesWithRecentRuns already sorts by most recent activity
	// So we're automatically fetching most recently active repos first

	// Collect ALL pipelines from all repositories (not just recent 50)
	// This ensures we cache default branch pipelines for ALL projects
	var pipelines []domain.Pipeline
	for _, repo := range reposWithRuns {
		pipelines = append(pipelines, repo.Runs...)
	}
	pipelineCount := len(pipelines)
	r.logger.Printf("Background refresher: Collected %d pipelines from %d repositories", pipelineCount, len(reposWithRuns))

	// Fetch branches (these are sorted by last commit date, most recent first)
	branches, err := r.pipelineService.GetAllBranches(ctx, 50)
	if err != nil {
		r.logger.Printf("Background refresher: Failed to fetch branches: %v", err)
	}
	branchCount := len(branches)

	// Fetch merge requests
	mrs, err := r.pipelineService.GetAllMergeRequests(ctx)
	if err != nil {
		r.logger.Printf("Background refresher: Failed to fetch merge requests: %v", err)
	}
	mrCount := len(mrs)

	// Fetch issues
	issues, err := r.pipelineService.GetAllIssues(ctx)
	if err != nil {
		r.logger.Printf("Background refresher: Failed to fetch issues: %v", err)
	}
	issueCount := len(issues)

	// Fetch user profiles
	profiles, err := r.pipelineService.GetUserProfiles(ctx)
	if err != nil {
		r.logger.Printf("Background refresher: Failed to fetch user profiles: %v", err)
	}
	profileCount := len(profiles)

	duration := time.Since(startTime)
	r.logger.Printf("Background refresher: Completed in %v (projects: %d, pipelines: %d, branches: %d, MRs: %d, issues: %d, profiles: %d)",
		duration, projectCount, pipelineCount, branchCount, mrCount, issueCount, profileCount)
}
