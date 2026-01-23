package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/vilaca/ci-dashboard/internal/domain"
)

// EventPoller polls repository events and invalidates caches when changes are detected.
// Follows Single Responsibility - only handles event polling and cache invalidation.
type EventPoller struct {
	pipelineService *PipelineService
	pollInterval    time.Duration
	logger          *log.Logger
	stopChan        chan struct{}
	wg              sync.WaitGroup
	mu              sync.Mutex
	running         bool

	// Track last seen event time per project
	lastEventTime map[string]map[string]time.Time // platform -> projectID -> last event time
}

// NewEventPoller creates a new event poller.
func NewEventPoller(pipelineService *PipelineService, pollInterval time.Duration) *EventPoller {
	return &EventPoller{
		pipelineService: pipelineService,
		pollInterval:    pollInterval,
		logger:          log.Default(),
		stopChan:        make(chan struct{}),
		lastEventTime:   make(map[string]map[string]time.Time),
	}
}

// Start begins periodic event polling.
func (p *EventPoller) Start() {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return
	}
	p.running = true
	p.mu.Unlock()

	p.logger.Printf("[EventPoller] Starting with %v poll interval", p.pollInterval)

	// Start background polling goroutine
	p.wg.Add(1)
	go p.pollLoop()
}

// Stop gracefully stops the event poller.
func (p *EventPoller) Stop() {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return
	}
	p.running = false
	p.mu.Unlock()

	p.logger.Printf("[EventPoller] Stopping...")
	close(p.stopChan)
	p.wg.Wait()
	p.logger.Printf("[EventPoller] Stopped")
}

// pollLoop periodically polls events from all repositories.
func (p *EventPoller) pollLoop() {
	defer p.wg.Done()

	// Wait a bit before first poll (server startup)
	time.Sleep(10 * time.Second)
	p.logger.Printf("[EventPoller] Performing initial event poll...")
	p.pollEvents()

	// Setup periodic polling ticker
	ticker := time.NewTicker(p.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.logger.Printf("[EventPoller] Performing periodic event poll...")
			p.pollEvents()
		case <-p.stopChan:
			return
		}
	}
}

// pollEvents polls events from all registered clients.
func (p *EventPoller) pollEvents() {
	ctx := context.Background()
	startTime := time.Now()

	// Get all projects to poll events for
	projects, err := p.pipelineService.GetAllProjects(ctx)
	if err != nil {
		p.logger.Printf("[EventPoller] Failed to get projects: %v", err)
		return
	}

	totalInvalidated := 0

	// Poll events for each client/platform
	p.pipelineService.mu.RLock()
	clients := make(map[string]interface{})
	for name, client := range p.pipelineService.clients {
		clients[name] = client
	}
	p.pipelineService.mu.RUnlock()

	for platform, client := range clients {
		// Check if client supports events
		type eventsGetter interface {
			GetEvents(ctx context.Context, projectID string, since time.Time) ([]domain.Event, error)
		}

		eventsClient, ok := client.(eventsGetter)
		if !ok {
			continue // Skip clients that don't support events
		}

		// Ensure platform map exists
		if p.lastEventTime[platform] == nil {
			p.lastEventTime[platform] = make(map[string]time.Time)
		}

		// Poll events for projects on this platform
		polledProjects := 0
		for _, project := range projects {
			if project.Platform != platform {
				continue
			}

			polledProjects++

			// Get last seen event time (or 1 hour ago if first poll)
			since := p.lastEventTime[platform][project.ID]
			if since.IsZero() {
				since = time.Now().Add(-1 * time.Hour)
			}

			// Poll events
			events, err := eventsClient.GetEvents(ctx, project.ID, since)
			if err != nil {
				p.logger.Printf("[EventPoller] %s: Failed to get events for %s: %v", platform, project.ID, err)
				continue
			}

			if len(events) == 0 {
				continue
			}

			p.logger.Printf("[EventPoller] %s: Found %d new events for %s", platform, len(events), project.Name)

			// Process events and invalidate caches
			invalidated := p.processEvents(client, project.ID, events)
			totalInvalidated += invalidated

			// Update last event time
			for _, event := range events {
				if event.CreatedAt.After(p.lastEventTime[platform][project.ID]) {
					p.lastEventTime[platform][project.ID] = event.CreatedAt
				}
			}
		}

		if polledProjects > 0 {
			p.logger.Printf("[EventPoller] %s: Polled %d projects for events", platform, polledProjects)
		}
	}

	duration := time.Since(startTime)
	if totalInvalidated > 0 {
		p.logger.Printf("[EventPoller] Completed in %v: Invalidated %d cache entries",
			duration.Round(time.Millisecond), totalInvalidated)
	} else {
		p.logger.Printf("[EventPoller] Completed in %v: No changes detected",
			duration.Round(time.Millisecond))
	}
}

// processEvents processes events and invalidates relevant cache entries.
func (p *EventPoller) processEvents(client interface{}, projectID string, events []domain.Event) int {
	// Type assert to get invalidation methods
	type cacheInvalidator interface {
		Invalidate(key string)
		InvalidatePattern(pattern string) int
		ForceRefresh(ctx context.Context, key string) error
	}

	invalidator, ok := client.(cacheInvalidator)
	if !ok {
		return 0
	}

	totalInvalidated := 0
	ctx := context.Background()

	for _, event := range events {
		switch event.Type {
		case "pushed to", "PushEvent":
			// Push to branch - invalidate branch data and pipelines for that branch
			branch := extractBranchName(event.TargetID)
			if branch != "" {
				// Invalidate and refresh latest pipeline for this branch
				key := fmt.Sprintf("GetLatestPipeline:%s:%s", projectID, branch)
				invalidator.Invalidate(key)
				if err := invalidator.ForceRefresh(ctx, key); err != nil {
					p.logger.Printf("[EventPoller] Failed to refresh %s: %v", key, err)
				}
				totalInvalidated++

				// Invalidate branches list (order may have changed)
				pattern := fmt.Sprintf("GetBranches:%s:", projectID)
				totalInvalidated += invalidator.InvalidatePattern(pattern)
			}

		case "opened", "closed", "merged", "accepted", "PullRequestEvent":
			// MR/PR event - invalidate merge requests
			pattern := fmt.Sprintf("GetMergeRequests:%s", projectID)
			invalidator.Invalidate(pattern)
			if err := invalidator.ForceRefresh(ctx, pattern); err != nil {
				p.logger.Printf("[EventPoller] Failed to refresh %s: %v", pattern, err)
			}
			totalInvalidated++

		case "IssuesEvent":
			// Issue event - invalidate issues
			pattern := fmt.Sprintf("GetIssues:%s", projectID)
			invalidator.Invalidate(pattern)
			if err := invalidator.ForceRefresh(ctx, pattern); err != nil {
				p.logger.Printf("[EventPoller] Failed to refresh %s: %v", pattern, err)
			}
			totalInvalidated++

		case "WorkflowRunEvent":
			// Workflow run - invalidate pipelines
			pattern := fmt.Sprintf("GetPipelines:%s:", projectID)
			totalInvalidated += invalidator.InvalidatePattern(pattern)
		}
	}

	return totalInvalidated
}

// extractBranchName extracts branch name from various formats.
// Handles "refs/heads/main", "main", etc.
func extractBranchName(ref string) string {
	// Remove "refs/heads/" prefix if present
	if strings.HasPrefix(ref, "refs/heads/") {
		return strings.TrimPrefix(ref, "refs/heads/")
	}
	return ref
}
