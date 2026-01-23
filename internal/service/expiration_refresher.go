package service

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/vilaca/ci-dashboard/internal/api"
	"github.com/vilaca/ci-dashboard/internal/domain"
)

// ExpirationRefresher refreshes expired cache entries in the background.
// Only fetches what's expired, serves stale data immediately.
type ExpirationRefresher struct {
	pipelineService *PipelineService
	fileCache       *FileCache
	refreshInterval time.Duration
	logger          *log.Logger
	stopChan        chan struct{}
	wg              sync.WaitGroup
	mu              sync.Mutex
	running         bool
}

// NewExpirationRefresher creates a new expiration-based refresher.
// fileCache can be nil to disable file cache loading.
func NewExpirationRefresher(pipelineService *PipelineService, fileCache *FileCache, refreshInterval time.Duration) *ExpirationRefresher {
	return &ExpirationRefresher{
		pipelineService: pipelineService,
		fileCache:       fileCache,
		refreshInterval: refreshInterval,
		logger:          log.Default(),
		stopChan:        make(chan struct{}),
	}
}

// Start begins periodic checking and refreshing of expired entries.
func (r *ExpirationRefresher) Start() {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return
	}
	r.running = true
	r.mu.Unlock()

	r.logger.Printf("[ExpirationRefresher] Starting with %v check interval", r.refreshInterval)

	// Load file cache immediately if available (for instant page loads)
	if r.fileCache != nil {
		r.loadAndPrePopulate()
	}

	// Start background refresh goroutine
	r.wg.Add(1)
	go r.refreshLoop()
}

// Stop gracefully stops the expiration refresher.
func (r *ExpirationRefresher) Stop() {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return
	}
	r.running = false
	r.mu.Unlock()

	r.logger.Printf("[ExpirationRefresher] Stopping...")
	close(r.stopChan)
	r.wg.Wait()
	r.logger.Printf("[ExpirationRefresher] Stopped")
}

// loadAndPrePopulate loads file cache and pre-populates in-memory caches.
func (r *ExpirationRefresher) loadAndPrePopulate() {
	if r.fileCache == nil {
		return
	}

	cacheData, err := r.fileCache.Load()
	if err != nil || cacheData == nil {
		r.logger.Printf("[ExpirationRefresher] No file cache to load - will trigger initial fetch")
		// Trigger initial project fetch in background (non-blocking)
		go r.initialFetch()
		return
	}

	age := time.Since(cacheData.Timestamp)
	r.logger.Printf("[ExpirationRefresher] Loaded file cache (age: %v, projects: %d)",
		age.Round(time.Second), len(cacheData.Projects))

	// Pre-populate all clients with cached data
	r.pipelineService.mu.RLock()
	clients := make(map[string]interface{})
	for name, client := range r.pipelineService.clients {
		clients[name] = client
	}
	r.pipelineService.mu.RUnlock()

	for platform, client := range clients {
		// Check if client supports cache population
		type cachePopulator interface {
			PopulateProjects([]domain.Project)
			PopulatePipelines([]domain.Pipeline)
			PopulateBranches([]domain.Branch)
			PopulateMergeRequests([]domain.MergeRequest)
			PopulateIssues([]domain.Issue)
			PopulateUserProfiles([]domain.UserProfile)
		}

		populator, ok := client.(cachePopulator)
		if !ok {
			continue
		}

		r.logger.Printf("[ExpirationRefresher] Pre-populating %s cache...", platform)
		populator.PopulateProjects(cacheData.Projects)
		populator.PopulatePipelines(cacheData.Pipelines)
		populator.PopulateBranches(cacheData.Branches)
		populator.PopulateMergeRequests(cacheData.MRs)
		populator.PopulateIssues(cacheData.Issues)
		populator.PopulateUserProfiles(cacheData.Profiles)
	}

	r.logger.Printf("[ExpirationRefresher] Cache pre-population complete - ready for requests!")
}

// initialFetch performs an initial fetch of projects when cache is empty.
func (r *ExpirationRefresher) initialFetch() {
	r.logger.Printf("[ExpirationRefresher] Starting initial fetch (cache was empty)...")
	ctx := context.Background()

	// Fetch projects from all platforms - this will populate the cache
	projects, err := r.pipelineService.GetAllProjects(ctx)
	if err != nil {
		r.logger.Printf("[ExpirationRefresher] Initial fetch failed: %v", err)
		return
	}

	r.logger.Printf("[ExpirationRefresher] Initial fetch complete: %d projects loaded", len(projects))
}

// refreshLoop periodically checks for expired cache entries and refreshes them.
func (r *ExpirationRefresher) refreshLoop() {
	defer r.wg.Done()

	// Wait a bit before first check (server startup)
	time.Sleep(5 * time.Second)
	r.logger.Printf("[ExpirationRefresher] Performing initial cache check...")
	r.refreshExpired()

	// Setup periodic refresh ticker
	ticker := time.NewTicker(r.refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.logger.Printf("[ExpirationRefresher] Performing periodic cache check...")
			r.refreshExpired()
		case <-r.stopChan:
			return
		}
	}
}

// refreshExpired checks all clients for expired cache entries and refreshes them.
func (r *ExpirationRefresher) refreshExpired() {
	ctx := context.Background()
	startTime := time.Now()

	// Get expired keys from all registered clients
	r.pipelineService.mu.RLock()
	clients := make(map[string]interface{})
	for name, client := range r.pipelineService.clients {
		clients[name] = client
	}
	r.pipelineService.mu.RUnlock()

	totalExpired := 0
	totalRefreshed := 0

	for platform, client := range clients {
		// Check if client supports expiration checking
		type expiredKeysGetter interface {
			GetExpiredKeys() []string
		}

		if expGetter, ok := client.(expiredKeysGetter); ok {
			expiredKeys := expGetter.GetExpiredKeys()
			totalExpired += len(expiredKeys)

			if len(expiredKeys) > 0 {
				r.logger.Printf("[ExpirationRefresher] %s: Found %d expired cache entries", platform, len(expiredKeys))

				// Refresh expired entries (prioritized by recent commits)
				refreshed := r.refreshKeys(ctx, client, expiredKeys)
				totalRefreshed += refreshed
			}
		}
	}

	duration := time.Since(startTime)
	if totalExpired > 0 {
		r.logger.Printf("[ExpirationRefresher] Completed in %v: %d/%d entries refreshed",
			duration.Round(time.Millisecond), totalRefreshed, totalExpired)

		// Save cache to file after successful refresh
		if r.fileCache != nil && totalRefreshed > 0 {
			r.saveCacheToFile(ctx)
		}
	} else {
		r.logger.Printf("[ExpirationRefresher] Completed in %v: No expired entries",
			duration.Round(time.Millisecond))
	}
}

// saveCacheToFile saves current cache state to file.
func (r *ExpirationRefresher) saveCacheToFile(ctx context.Context) {
	if r.fileCache == nil {
		return
	}

	// Collect all data from service
	projects, _ := r.pipelineService.GetAllProjects(ctx)
	branches, _ := r.pipelineService.GetAllBranches(ctx, 50)
	mrs, _ := r.pipelineService.GetAllMergeRequests(ctx)
	issues, _ := r.pipelineService.GetAllIssues(ctx)
	profiles, _ := r.pipelineService.GetUserProfiles(ctx)

	// Collect pipelines from repositories
	var pipelines []domain.Pipeline
	repos, _ := r.pipelineService.GetRepositoriesWithRecentRuns(ctx, 3)
	for _, repo := range repos {
		pipelines = append(pipelines, repo.Runs...)
	}

	cacheData := &CacheData{
		Timestamp: time.Now(),
		Projects:  projects,
		Pipelines: pipelines,
		Branches:  branches,
		MRs:       mrs,
		Issues:    issues,
		Profiles:  profiles,
	}

	if err := r.fileCache.Save(cacheData); err != nil {
		r.logger.Printf("[ExpirationRefresher] Failed to save cache to file: %v", err)
	} else {
		r.logger.Printf("[ExpirationRefresher] Saved cache to file (%d projects, %d pipelines)",
			len(projects), len(pipelines))
	}
}

// refreshKeys refreshes a list of cache keys.
// Keys are already prioritized by the cache (recent commits first).
func (r *ExpirationRefresher) refreshKeys(ctx context.Context, client interface{}, keys []string) int {
	// Limit concurrent refreshes to avoid overwhelming APIs
	semaphore := make(chan struct{}, 5) // Max 5 concurrent refreshes
	var wg sync.WaitGroup
	var mu sync.Mutex
	refreshed := 0

	for _, key := range keys {
		wg.Add(1)
		go func(k string) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Refresh this specific cache entry
			if r.refreshSingleKey(ctx, client, k) {
				mu.Lock()
				refreshed++
				mu.Unlock()
			}
		}(key)
	}

	wg.Wait()
	return refreshed
}

// refreshSingleKey refreshes a single cache key by calling the appropriate API method.
func (r *ExpirationRefresher) refreshSingleKey(ctx context.Context, client interface{}, key string) bool {
	// Parse key to determine what to fetch
	// Key format examples:
	// - "GetProjects"
	// - "GetProjectCount"
	// - "GetLatestPipeline:projectID:branch"
	// - "GetBranches:projectID:limit"
	// - "GetPipelines:projectID:limit"
	// - "GetMergeRequests:projectID"
	// - "GetIssues:projectID"
	// - "GetCurrentUser"

	parts := splitCacheKey(key)
	method := parts[0]

	// Type assert to api.Client interface
	apiClient, ok := client.(api.Client)
	if !ok {
		r.logger.Printf("[ExpirationRefresher] Client does not implement api.Client interface for key: %s", key)
		return false
	}

	var err error
	switch method {
	case "GetProjects":
		_, err = apiClient.GetProjects(ctx)
	case "GetProjectCount":
		_, err = apiClient.GetProjectCount(ctx)
	case "GetLatestPipeline":
		if len(parts) != 3 {
			r.logger.Printf("[ExpirationRefresher] Invalid key format: %s", key)
			return false
		}
		projectID := parts[1]
		branch := parts[2]
		_, err = apiClient.GetLatestPipeline(ctx, projectID, branch)
	case "GetBranches":
		if len(parts) != 3 {
			r.logger.Printf("[ExpirationRefresher] Invalid key format: %s", key)
			return false
		}
		projectID := parts[1]
		limit := parseIntOrDefault(parts[2], 50)
		_, err = apiClient.GetBranches(ctx, projectID, limit)
	case "GetPipelines":
		if len(parts) != 3 {
			r.logger.Printf("[ExpirationRefresher] Invalid key format: %s", key)
			return false
		}
		projectID := parts[1]
		limit := parseIntOrDefault(parts[2], 50)
		_, err = apiClient.GetPipelines(ctx, projectID, limit)
	case "GetMergeRequests":
		// Try extended client interface
		if extClient, ok := client.(api.ExtendedClient); ok {
			if len(parts) != 2 {
				r.logger.Printf("[ExpirationRefresher] Invalid key format: %s", key)
				return false
			}
			projectID := parts[1]
			_, err = extClient.GetMergeRequests(ctx, projectID)
		} else {
			return true // Skip if not supported
		}
	case "GetIssues":
		// Try extended client interface
		if extClient, ok := client.(api.ExtendedClient); ok {
			if len(parts) != 2 {
				r.logger.Printf("[ExpirationRefresher] Invalid key format: %s", key)
				return false
			}
			projectID := parts[1]
			_, err = extClient.GetIssues(ctx, projectID)
		} else {
			return true // Skip if not supported
		}
	case "GetCurrentUser":
		// Try user client interface
		if userClient, ok := client.(api.UserClient); ok {
			_, err = userClient.GetCurrentUser(ctx)
		} else {
			return true // Skip if not supported
		}
	default:
		r.logger.Printf("[ExpirationRefresher] Unknown method in key: %s", key)
		return false
	}

	if err != nil {
		r.logger.Printf("[ExpirationRefresher] Failed to refresh %s: %v", key, err)
		return false
	}

	r.logger.Printf("[ExpirationRefresher] Successfully refreshed: %s", key)
	return true
}

// splitCacheKey splits a cache key by colons.
func splitCacheKey(key string) []string {
	result := []string{}
	current := ""
	for _, ch := range key {
		if ch == ':' {
			result = append(result, current)
			current = ""
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

// parseIntOrDefault parses a string to int, returns default if parsing fails.
func parseIntOrDefault(s string, defaultVal int) int {
	var result int
	for _, ch := range s {
		if ch >= '0' && ch <= '9' {
			result = result*10 + int(ch-'0')
		} else {
			return defaultVal
		}
	}
	if result == 0 {
		return defaultVal
	}
	return result
}
