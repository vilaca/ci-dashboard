package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/vilaca/ci-dashboard/internal/domain"
)

// CachingClient wraps a Client with caching capabilities.
// Follows Decorator pattern to add caching without modifying the underlying client.
type CachingClient struct {
	client        Client
	extendedClient ExtendedClient // May be nil if underlying client doesn't implement ExtendedClient
	userClient    UserClient     // May be nil if underlying client doesn't implement UserClient
	cache         *cache
}

// NewCachingClient creates a new caching client wrapper.
func NewCachingClient(client Client, cacheDuration time.Duration) *CachingClient {
	extendedClient, _ := client.(ExtendedClient)
	userClient, _ := client.(UserClient)

	return &CachingClient{
		client:         client,
		extendedClient: extendedClient,
		userClient:     userClient,
		cache:          newCache(cacheDuration),
	}
}

// GetProjects retrieves projects with caching.
func (c *CachingClient) GetProjects(ctx context.Context) ([]domain.Project, error) {
	key := "GetProjects"

	// Try cache first
	if cached, found := c.cache.get(key); found {
		if projects, ok := cached.([]domain.Project); ok {
			log.Printf("[Cache] %s - HIT (%d projects)", key, len(projects))
			return projects, nil
		}
	}

	// Cache miss - fetch from underlying client
	log.Printf("[Cache] %s - MISS", key)
	projects, err := c.client.GetProjects(ctx)
	if err != nil {
		return nil, err
	}

	// Store in cache
	c.cache.set(key, projects)

	return projects, nil
}

// GetProjectsPage retrieves a single page of projects (no caching for individual pages).
func (c *CachingClient) GetProjectsPage(ctx context.Context, page int) ([]domain.Project, bool, error) {
	// Don't cache individual pages - they're only used during streaming
	return c.client.GetProjectsPage(ctx, page)
}

// GetProjectCount retrieves the total project count with caching.
func (c *CachingClient) GetProjectCount(ctx context.Context) (int, error) {
	key := "GetProjectCount"

	// Try cache first
	if cached, found := c.cache.get(key); found {
		if count, ok := cached.(int); ok {
			log.Printf("[Cache] %s - HIT (%d projects)", key, count)
			return count, nil
		}
	}

	// Cache miss - fetch from underlying client
	log.Printf("[Cache] %s - MISS", key)
	count, err := c.client.GetProjectCount(ctx)
	if err != nil {
		return 0, err
	}

	// Store in cache
	c.cache.set(key, count)

	return count, nil
}

// GetLatestPipeline retrieves the latest pipeline with caching.
func (c *CachingClient) GetLatestPipeline(ctx context.Context, projectID, branch string) (*domain.Pipeline, error) {
	key := fmt.Sprintf("GetLatestPipeline:%s:%s", projectID, branch)

	// Try cache first
	if cached, found := c.cache.get(key); found {
		if pipeline, ok := cached.(*domain.Pipeline); ok {
			log.Printf("[Cache] %s - HIT", key)
			return pipeline, nil
		}
	}

	// Cache miss - fetch from underlying client
	log.Printf("[Cache] %s - MISS", key)
	pipeline, err := c.client.GetLatestPipeline(ctx, projectID, branch)
	if err != nil {
		return nil, err
	}

	// Store in cache
	c.cache.set(key, pipeline)

	return pipeline, nil
}

// GetPipelines retrieves pipelines with caching.
func (c *CachingClient) GetPipelines(ctx context.Context, projectID string, limit int) ([]domain.Pipeline, error) {
	key := fmt.Sprintf("GetPipelines:%s:%d", projectID, limit)

	// Try cache first
	if cached, found := c.cache.get(key); found {
		if pipelines, ok := cached.([]domain.Pipeline); ok {
			log.Printf("[Cache] %s - HIT (%d pipelines)", key, len(pipelines))
			return pipelines, nil
		}
	}

	// Cache miss - fetch from underlying client
	log.Printf("[Cache] %s - MISS", key)
	pipelines, err := c.client.GetPipelines(ctx, projectID, limit)
	if err != nil {
		return nil, err
	}

	// Store in cache
	c.cache.set(key, pipelines)

	return pipelines, nil
}

// GetBranches retrieves branches with caching.
func (c *CachingClient) GetBranches(ctx context.Context, projectID string, limit int) ([]domain.Branch, error) {
	key := fmt.Sprintf("GetBranches:%s:%d", projectID, limit)

	// Try cache first
	if cached, found := c.cache.get(key); found {
		if branches, ok := cached.([]domain.Branch); ok {
			log.Printf("[Cache] %s - HIT (%d branches)", key, len(branches))
			return branches, nil
		}
	}

	// Cache miss - fetch from underlying client
	log.Printf("[Cache] %s - MISS", key)
	branches, err := c.client.GetBranches(ctx, projectID, limit)
	if err != nil {
		return nil, err
	}

	// Store in cache
	c.cache.set(key, branches)

	return branches, nil
}

// GetMergeRequests retrieves merge requests with caching (ExtendedClient).
func (c *CachingClient) GetMergeRequests(ctx context.Context, projectID string) ([]domain.MergeRequest, error) {
	if c.extendedClient == nil {
		return nil, fmt.Errorf("underlying client does not support GetMergeRequests")
	}

	key := fmt.Sprintf("GetMergeRequests:%s", projectID)

	// Try cache first
	if cached, found := c.cache.get(key); found {
		if mrs, ok := cached.([]domain.MergeRequest); ok {
			log.Printf("[Cache] %s - HIT (%d MRs)", key, len(mrs))
			return mrs, nil
		}
	}

	// Cache miss - fetch from underlying client
	log.Printf("[Cache] %s - MISS", key)
	mrs, err := c.extendedClient.GetMergeRequests(ctx, projectID)
	if err != nil {
		return nil, err
	}

	// Store in cache
	c.cache.set(key, mrs)

	return mrs, nil
}

// GetIssues retrieves issues with caching (ExtendedClient).
func (c *CachingClient) GetIssues(ctx context.Context, projectID string) ([]domain.Issue, error) {
	if c.extendedClient == nil {
		return nil, fmt.Errorf("underlying client does not support GetIssues")
	}

	key := fmt.Sprintf("GetIssues:%s", projectID)

	// Try cache first
	if cached, found := c.cache.get(key); found {
		if issues, ok := cached.([]domain.Issue); ok {
			log.Printf("[Cache] %s - HIT (%d issues)", key, len(issues))
			return issues, nil
		}
	}

	// Cache miss - fetch from underlying client
	log.Printf("[Cache] %s - MISS", key)
	issues, err := c.extendedClient.GetIssues(ctx, projectID)
	if err != nil {
		return nil, err
	}

	// Store in cache
	c.cache.set(key, issues)

	return issues, nil
}

// GetCurrentUser retrieves the current user profile with caching (UserClient).
func (c *CachingClient) GetCurrentUser(ctx context.Context) (*domain.UserProfile, error) {
	if c.userClient == nil {
		return nil, fmt.Errorf("underlying client does not support GetCurrentUser")
	}

	key := "GetCurrentUser"

	// Try cache first
	if cached, found := c.cache.get(key); found {
		if profile, ok := cached.(*domain.UserProfile); ok {
			log.Printf("[Cache] %s - HIT", key)
			return profile, nil
		}
	}

	// Cache miss - fetch from underlying client
	log.Printf("[Cache] %s - MISS", key)
	profile, err := c.userClient.GetCurrentUser(ctx)
	if err != nil {
		return nil, err
	}

	// Store in cache
	c.cache.set(key, profile)

	return profile, nil
}

// cache implements a thread-safe TTL cache.
type cache struct {
	mu       sync.RWMutex
	entries  map[string]*cacheEntry
	duration time.Duration
}

// cacheEntry holds a cached value with expiry time.
type cacheEntry struct {
	value     interface{}
	expiresAt time.Time
}

// newCache creates a new cache with the specified duration.
func newCache(duration time.Duration) *cache {
	c := &cache{
		entries:  make(map[string]*cacheEntry),
		duration: duration,
	}

	// Start cleanup goroutine
	go c.cleanup()

	return c
}

// get retrieves a value from cache.
func (c *cache) get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.entries[key]
	if !exists {
		return nil, false
	}

	// Check if entry has expired
	if time.Now().After(entry.expiresAt) {
		return nil, false
	}

	return entry.value, true
}

// set stores a value in cache with TTL.
func (c *cache) set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = &cacheEntry{
		value:     value,
		expiresAt: time.Now().Add(c.duration),
	}
}

// cleanup periodically removes expired entries.
func (c *cache) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for key, entry := range c.entries {
			if now.After(entry.expiresAt) {
				delete(c.entries, key)
			}
		}
		c.mu.Unlock()
	}
}

// generateCacheKey generates a cache key from parameters.
func generateCacheKey(parts ...interface{}) string {
	data, _ := json.Marshal(parts)
	hash := sha256.Sum256(data)
	return fmt.Sprintf("%x", hash)
}
