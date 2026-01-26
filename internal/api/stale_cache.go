package api

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vilaca/ci-dashboard/internal/domain"
)

// StaleCache implements stale-while-revalidate caching strategy.
// Always serves cached data immediately (even if expired), refreshes in background.
type StaleCache struct {
	entries  map[string]*staleCacheEntry
	mu       sync.RWMutex
	ttl      time.Duration
	staleTTL time.Duration // How long to serve stale data before considering it invalid
}

type staleCacheEntry struct {
	value      interface{}
	cachedAt   time.Time
	expiresAt  time.Time
	staleUntil time.Time // Can still be served until this time
	projectID  string    // For priority refresh
	lastCommit time.Time // For priority refresh
}

// NewStaleCache creates a new stale-while-revalidate cache.
// ttl: how long data is considered fresh
// staleTTL: how long to serve stale data (typically much longer, e.g., 24 hours)
func NewStaleCache(ttl, staleTTL time.Duration) *StaleCache {
	c := &StaleCache{
		entries:  make(map[string]*staleCacheEntry),
		ttl:      ttl,
		staleTTL: staleTTL,
	}

	return c
}

// Get retrieves a value from cache.
// Returns: (value, isFresh, exists)
// - isFresh = true: data is not expired
// - isFresh = false: data is expired but still usable (stale)
// - exists = false: no data available (not even stale)
func (c *StaleCache) Get(key string) (interface{}, bool, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[key]
	if !exists {
		return nil, false, false
	}

	now := time.Now()

	// Check if entry is completely stale (unusable)
	if now.After(entry.staleUntil) {
		return nil, false, false
	}

	// Check if entry is fresh
	isFresh := now.Before(entry.expiresAt)

	return entry.value, isFresh, true
}

// Set stores a value in cache with TTL.
func (c *StaleCache) Set(key string, value interface{}, projectID string, lastCommit time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	c.entries[key] = &staleCacheEntry{
		value:      value,
		cachedAt:   now,
		expiresAt:  now.Add(c.ttl),
		staleUntil: now.Add(c.staleTTL),
		projectID:  projectID,
		lastCommit: lastCommit,
	}
}

// Invalidate removes entries from cache, forcing them to be expired.
// Used by event poller when detecting changes in repositories.
func (c *StaleCache) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.entries[key]; exists {
		delete(c.entries, key)
		log.Printf("[StaleCache] Invalidated: %s", key)
	}
}

// InvalidatePattern removes all cache entries matching a pattern.
// Example: InvalidatePattern("GetLatestPipeline:123:") invalidates all branches for project 123
func (c *StaleCache) InvalidatePattern(pattern string) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	count := 0
	for key := range c.entries {
		// Simple pattern matching: check if key starts with pattern
		if len(key) >= len(pattern) && key[:len(pattern)] == pattern {
			delete(c.entries, key)
			count++
		}
	}

	if count > 0 {
		log.Printf("[StaleCache] Invalidated %d entries matching pattern: %s*", count, pattern)
	}

	return count
}

// GetExpiredKeys returns all cache keys that are expired but still usable.
// Returns keys sorted by priority (recent commits first).
func (c *StaleCache) GetExpiredKeys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	type keyWithPriority struct {
		key        string
		lastCommit time.Time
	}

	var expired []keyWithPriority
	now := time.Now()

	for key, entry := range c.entries {
		// Only include expired entries that are still usable
		if now.After(entry.expiresAt) && now.Before(entry.staleUntil) {
			expired = append(expired, keyWithPriority{
				key:        key,
				lastCommit: entry.lastCommit,
			})
		}
	}

	// Sort by last commit (most recent first)
	sort.Slice(expired, func(i, j int) bool {
		return expired[i].lastCommit.After(expired[j].lastCommit)
	})

	// Extract just the keys
	keys := make([]string, len(expired))
	for i, item := range expired {
		keys[i] = item.key
	}

	return keys
}

// GetStats returns cache statistics.
func (c *StaleCache) GetStats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	now := time.Now()
	stats := CacheStats{
		TotalEntries: len(c.entries),
	}

	for _, entry := range c.entries {
		if now.Before(entry.expiresAt) {
			stats.FreshEntries++
		} else if now.Before(entry.staleUntil) {
			stats.StaleEntries++
		} else {
			stats.ExpiredEntries++
		}
	}

	return stats
}

// CacheStats holds cache statistics.
type CacheStats struct {
	TotalEntries   int
	FreshEntries   int
	StaleEntries   int
	ExpiredEntries int
}

// StaleCachingClient wraps a Client with stale-while-revalidate caching.
type StaleCachingClient struct {
	client         Client
	extendedClient ExtendedClient
	userClient     UserClient
	eventsClient   EventsClient
	cache          *StaleCache
}

// NewStaleCachingClient creates a new stale-caching client wrapper.
func NewStaleCachingClient(client Client, ttl, staleTTL time.Duration) *StaleCachingClient {
	extendedClient, ok := client.(ExtendedClient)
	if !ok {
		log.Printf("[Cache] Client does not implement ExtendedClient interface (GetMergeRequests/GetIssues not available)")
	}

	userClient, ok := client.(UserClient)
	if !ok {
		log.Printf("[Cache] Client does not implement UserClient interface (GetCurrentUser not available)")
	}

	eventsClient, ok := client.(EventsClient)
	if !ok {
		log.Printf("[Cache] Client does not implement EventsClient interface (GetEvents not available)")
	}

	return &StaleCachingClient{
		client:         client,
		extendedClient: extendedClient,
		userClient:     userClient,
		eventsClient:   eventsClient,
		cache:          NewStaleCache(ttl, staleTTL),
	}
}

// getCached is a generic helper for retrieving typed values from cache.
// Returns the cached value and true if found and correctly typed, or defaultValue and false otherwise.
func getCached[T any](cache *StaleCache, key string, defaultValue T) (T, bool) {
	if cached, _, found := cache.Get(key); found {
		if value, ok := cached.(T); ok {
			return value, true
		}
		// Type mismatch - log and return default
		log.Printf("[Cache] Type mismatch for key %s", key)
	}
	return defaultValue, false
}

// GetProjects retrieves projects with stale-while-revalidate caching.
// CACHE-ONLY: Returns cached data or empty slice. Never triggers API calls.
func (c *StaleCachingClient) GetProjects(ctx context.Context) ([]domain.Project, error) {
	key := "GetProjects"
	projects, found := getCached(c.cache, key, []domain.Project{})
	if found {
		return projects, nil
	}
	// Cache miss - return empty slice (background refresher will populate)
	return []domain.Project{}, nil
}

// GetLatestPipeline retrieves the latest pipeline with stale-while-revalidate caching.
// CACHE-ONLY: Returns cached data or nil. Never triggers API calls.
func (c *StaleCachingClient) GetLatestPipeline(ctx context.Context, projectID, branch string) (*domain.Pipeline, error) {
	key := fmt.Sprintf("GetLatestPipeline:%s:%s", projectID, branch)
	pipeline, found := getCached(c.cache, key, (*domain.Pipeline)(nil))
	if found {
		return pipeline, nil
	}
	// Cache miss - return nil (background refresher will populate)
	return nil, nil
}

// Implement other Client methods...
func (c *StaleCachingClient) GetProjectsPage(ctx context.Context, page int) ([]domain.Project, bool, error) {
	return c.client.GetProjectsPage(ctx, page)
}

func (c *StaleCachingClient) GetProjectCount(ctx context.Context) (int, error) {
	key := "GetProjectCount"
	count, found := getCached(c.cache, key, 0)
	if found {
		return count, nil
	}
	// Cache miss - return 0 (background refresher will populate)
	return 0, nil
}

func (c *StaleCachingClient) GetPipelines(ctx context.Context, projectID string, limit int) ([]domain.Pipeline, error) {
	key := fmt.Sprintf("GetPipelines:%s:%d", projectID, limit)
	pipelines, found := getCached(c.cache, key, []domain.Pipeline{})
	if found {
		return pipelines, nil
	}
	// Cache miss - return empty slice (background refresher will populate)
	return []domain.Pipeline{}, nil
}

func (c *StaleCachingClient) GetBranches(ctx context.Context, projectID string, limit int) ([]domain.Branch, error) {
	key := fmt.Sprintf("GetBranches:%s:%d", projectID, limit)
	branches, found := getCached(c.cache, key, []domain.Branch{})
	if found {
		return branches, nil
	}
	// Cache miss - return empty slice (background refresher will populate)
	return []domain.Branch{}, nil
}

func (c *StaleCachingClient) GetBranch(ctx context.Context, projectID, branchName string) (*domain.Branch, error) {
	key := fmt.Sprintf("GetBranch:%s:%s", projectID, branchName)
	branch, found := getCached(c.cache, key, &domain.Branch{})
	if found {
		return branch, nil
	}
	// Cache miss - return nil (background refresher will populate)
	return nil, nil
}

// GetMergeRequests with caching
// CACHE-ONLY: Returns cached data or empty slice. Never triggers API calls.
func (c *StaleCachingClient) GetMergeRequests(ctx context.Context, projectID string) ([]domain.MergeRequest, error) {
	if c.extendedClient == nil {
		return []domain.MergeRequest{}, nil
	}

	key := fmt.Sprintf("GetMergeRequests:%s", projectID)
	mrs, found := getCached(c.cache, key, []domain.MergeRequest{})
	if found {
		return mrs, nil
	}
	// Cache miss - return empty slice (background refresher will populate)
	return []domain.MergeRequest{}, nil
}

// GetIssues with caching
// CACHE-ONLY: Returns cached data or empty slice. Never triggers API calls.
func (c *StaleCachingClient) GetIssues(ctx context.Context, projectID string) ([]domain.Issue, error) {
	if c.extendedClient == nil {
		return []domain.Issue{}, nil
	}

	key := fmt.Sprintf("GetIssues:%s", projectID)
	issues, found := getCached(c.cache, key, []domain.Issue{})
	if found {
		return issues, nil
	}
	// Cache miss - return empty slice (background refresher will populate)
	return []domain.Issue{}, nil
}

// GetCurrentUser with caching
// CACHE-ONLY: Returns cached data or nil. Never triggers API calls.
func (c *StaleCachingClient) GetCurrentUser(ctx context.Context) (*domain.UserProfile, error) {
	if c.userClient == nil {
		return nil, nil
	}

	key := "GetCurrentUser"
	user, found := getCached(c.cache, key, (*domain.UserProfile)(nil))
	if found {
		return user, nil
	}
	// Cache miss - return nil (background refresher will populate)
	return nil, nil
}

// GetEvents retrieves recent events (NOT cached - always fresh for event polling).
func (c *StaleCachingClient) GetEvents(ctx context.Context, projectID string, since time.Time) ([]domain.Event, error) {
	if c.eventsClient == nil {
		return nil, fmt.Errorf("underlying client does not support GetEvents")
	}

	// Events are NOT cached - they're used for cache invalidation detection
	return c.eventsClient.GetEvents(ctx, projectID, since)
}

// PopulateProjects pre-populates the cache with projects data.
// Used on startup to load from file cache for instant page loads.
func (c *StaleCachingClient) PopulateProjects(projects []domain.Project) {
	if len(projects) == 0 {
		return
	}
	key := "GetProjects"
	c.cache.Set(key, projects, "", time.Time{})
	c.cache.Set("GetProjectCount", len(projects), "", time.Time{})
}

// PopulatePipelines pre-populates the cache with pipeline data.
func (c *StaleCachingClient) PopulatePipelines(pipelines []domain.Pipeline) {
	if len(pipelines) == 0 {
		return
	}

	// Group pipelines by project for GetPipelines cache keys
	projectPipelines := make(map[string][]domain.Pipeline)
	for _, pipeline := range pipelines {
		projectPipelines[pipeline.ProjectID] = append(projectPipelines[pipeline.ProjectID], pipeline)
	}

	// Populate GetPipelines cache keys
	for projectID, pipes := range projectPipelines {
		key := fmt.Sprintf("GetPipelines:%s:50", projectID)
		var lastCommit time.Time
		if len(pipes) > 0 {
			lastCommit = pipes[0].UpdatedAt
		}
		c.cache.Set(key, pipes, projectID, lastCommit)
	}

	// Populate GetLatestPipeline cache keys (per branch)
	for _, pipeline := range pipelines {
		if pipeline.Branch != "" {
			key := fmt.Sprintf("GetLatestPipeline:%s:%s", pipeline.ProjectID, pipeline.Branch)
			c.cache.Set(key, &pipeline, pipeline.ProjectID, pipeline.UpdatedAt)
		}
	}
}

// PopulateBranches pre-populates the cache with branch data.
func (c *StaleCachingClient) PopulateBranches(branches []domain.Branch) {
	if len(branches) == 0 {
		return
	}

	// Group branches by project
	projectBranches := make(map[string][]domain.Branch)
	for _, branch := range branches {
		projectBranches[branch.ProjectID] = append(projectBranches[branch.ProjectID], branch)
	}

	// Populate GetBranches cache keys
	for projectID, branchList := range projectBranches {
		key := fmt.Sprintf("GetBranches:%s:50", projectID)
		var lastCommit time.Time
		if len(branchList) > 0 {
			lastCommit = branchList[0].LastCommitDate
		}
		c.cache.Set(key, branchList, projectID, lastCommit)
	}
}

// PopulateMergeRequests pre-populates the cache with merge request data.
func (c *StaleCachingClient) PopulateMergeRequests(mrs []domain.MergeRequest) {
	if len(mrs) == 0 {
		return
	}

	// Group MRs by project
	projectMRs := make(map[string][]domain.MergeRequest)
	for _, mr := range mrs {
		projectMRs[mr.ProjectID] = append(projectMRs[mr.ProjectID], mr)
	}

	// Populate GetMergeRequests cache keys
	for projectID, mrList := range projectMRs {
		key := fmt.Sprintf("GetMergeRequests:%s", projectID)
		var lastUpdate time.Time
		if len(mrList) > 0 {
			lastUpdate = mrList[0].UpdatedAt
		}
		c.cache.Set(key, mrList, projectID, lastUpdate)
	}
}

// PopulateIssues pre-populates the cache with issue data.
func (c *StaleCachingClient) PopulateIssues(issues []domain.Issue) {
	if len(issues) == 0 {
		return
	}

	// Group issues by project
	projectIssues := make(map[string][]domain.Issue)
	for _, issue := range issues {
		projectIssues[issue.ProjectID] = append(projectIssues[issue.ProjectID], issue)
	}

	// Populate GetIssues cache keys
	for projectID, issueList := range projectIssues {
		key := fmt.Sprintf("GetIssues:%s", projectID)
		var lastUpdate time.Time
		if len(issueList) > 0 {
			lastUpdate = issueList[0].UpdatedAt
		}
		c.cache.Set(key, issueList, projectID, lastUpdate)
	}
}

// PopulateUserProfiles pre-populates the cache with user profile data.
func (c *StaleCachingClient) PopulateUserProfiles(profiles []domain.UserProfile) {
	if len(profiles) == 0 {
		return
	}

	// For now, just cache the first profile as "current user"
	// (file cache doesn't distinguish between platforms)
	if len(profiles) > 0 {
		key := "GetCurrentUser"
		c.cache.Set(key, &profiles[0], "", time.Time{})
	}
}

// GetExpiredKeys returns cache keys that need refreshing.
func (c *StaleCachingClient) GetExpiredKeys() []string {
	return c.cache.GetExpiredKeys()
}

// Invalidate removes a cache entry, forcing it to be re-fetched.
func (c *StaleCachingClient) Invalidate(key string) {
	c.cache.Invalidate(key)
}

// InvalidatePattern removes all cache entries matching a pattern.
func (c *StaleCachingClient) InvalidatePattern(pattern string) int {
	return c.cache.InvalidatePattern(pattern)
}

// ForceRefresh immediately fetches and caches data for a specific key.
// Used by event poller to populate cache after invalidation.
func (c *StaleCachingClient) ForceRefresh(ctx context.Context, key string) error {
	parts := strings.Split(key, ":")

	if len(parts) == 0 {
		return fmt.Errorf("invalid cache key: %s", key)
	}

	method := parts[0]

	switch method {
	case "GetProjects":
		projects, fetchErr := c.client.GetProjects(ctx)
		if fetchErr != nil {
			return fetchErr
		}
		c.cache.Set(key, projects, "", time.Time{})

	case "GetProjectCount":
		count, fetchErr := c.client.GetProjectCount(ctx)
		if fetchErr != nil {
			return fetchErr
		}
		c.cache.Set(key, count, "", time.Time{})

	case "GetLatestPipeline":
		if len(parts) != 3 {
			return fmt.Errorf("invalid key format: %s", key)
		}
		pipeline, fetchErr := c.client.GetLatestPipeline(ctx, parts[1], parts[2])
		if fetchErr != nil {
			return fetchErr
		}
		var lastCommit time.Time
		if pipeline != nil {
			lastCommit = pipeline.UpdatedAt
		}
		c.cache.Set(key, pipeline, parts[1], lastCommit)

	case "GetBranches":
		if len(parts) != 3 {
			return fmt.Errorf("invalid key format: %s", key)
		}
		limit := 200
		if parsedLimit, err := strconv.Atoi(parts[2]); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
		branches, fetchErr := c.client.GetBranches(ctx, parts[1], limit)
		if fetchErr != nil {
			return fetchErr
		}
		var lastCommit time.Time
		if len(branches) > 0 {
			lastCommit = branches[0].LastCommitDate
		}
		c.cache.Set(key, branches, parts[1], lastCommit)

	case "GetPipelines":
		if len(parts) != 3 {
			return fmt.Errorf("invalid key format: %s", key)
		}
		limit := 200
		if parsedLimit, err := strconv.Atoi(parts[2]); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
		pipelines, fetchErr := c.client.GetPipelines(ctx, parts[1], limit)
		if fetchErr != nil {
			return fetchErr
		}
		var lastCommit time.Time
		if len(pipelines) > 0 {
			lastCommit = pipelines[0].UpdatedAt
		}
		c.cache.Set(key, pipelines, parts[1], lastCommit)

	case "GetMergeRequests":
		if c.extendedClient == nil {
			return fmt.Errorf("client does not support GetMergeRequests")
		}
		if len(parts) != 2 {
			return fmt.Errorf("invalid key format: %s", key)
		}
		mrs, fetchErr := c.extendedClient.GetMergeRequests(ctx, parts[1])
		if fetchErr != nil {
			return fetchErr
		}
		var lastUpdate time.Time
		if len(mrs) > 0 {
			lastUpdate = mrs[0].UpdatedAt
		}
		c.cache.Set(key, mrs, parts[1], lastUpdate)

	case "GetIssues":
		if c.extendedClient == nil {
			return fmt.Errorf("client does not support GetIssues")
		}
		if len(parts) != 2 {
			return fmt.Errorf("invalid key format: %s", key)
		}
		issues, fetchErr := c.extendedClient.GetIssues(ctx, parts[1])
		if fetchErr != nil {
			return fetchErr
		}
		var lastUpdate time.Time
		if len(issues) > 0 {
			lastUpdate = issues[0].UpdatedAt
		}
		c.cache.Set(key, issues, parts[1], lastUpdate)

	case "GetCurrentUser":
		if c.userClient == nil {
			return fmt.Errorf("client does not support GetCurrentUser")
		}
		profile, fetchErr := c.userClient.GetCurrentUser(ctx)
		if fetchErr != nil {
			return fetchErr
		}
		c.cache.Set(key, profile, "", time.Time{})

	default:
		return fmt.Errorf("unknown method: %s", method)
	}

	return nil
}
