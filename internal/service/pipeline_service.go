package service

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/vilaca/ci-dashboard/internal/api"
	"github.com/vilaca/ci-dashboard/internal/domain"
)

const (
	// MaxConcurrentWorkers limits the number of concurrent goroutines for fetching data
	// This prevents overwhelming the system when processing many projects
	MaxConcurrentWorkers = 50

	// DefaultPipelinesPerProject is the default number of pipelines to fetch per project
	DefaultPipelinesPerProject = 10

	// PipelineFetchBuffer is the extra buffer added when calculating pipelines per project
	PipelineFetchBuffer = 5

	// RefreshOperationTimeout is the maximum time allowed for a refresh operation
	RefreshOperationTimeout = 10 * time.Minute

	// InitialRefreshDelay is the delay before the first background refresh
	// This allows the server to fully start before fetching data
	InitialRefreshDelay = 2 * time.Second
)

// PipelineService handles business logic for pipeline operations.
// Follows Single Responsibility Principle - orchestrates pipeline operations.
type PipelineService struct {
	clients          map[string]api.Client // platform name -> client
	gitlabWhitelist  []string              // allowed GitLab repository IDs (nil = allow all)
	githubWhitelist  []string              // allowed GitHub repository IDs (nil = allow all)
	mu               sync.RWMutex
}

// NewPipelineService creates a new pipeline service.
// gitlabWhitelist and githubWhitelist restrict access to specified repositories (nil = allow all).
func NewPipelineService(gitlabWhitelist, githubWhitelist []string) *PipelineService {
	return &PipelineService{
		clients:         make(map[string]api.Client),
		gitlabWhitelist: gitlabWhitelist,
		githubWhitelist: githubWhitelist,
	}
}

// Helper functions to fix repository names (convert project ID to project name)

// fixBranchRepositoryNames updates branch repository names from project ID to project name.
func fixBranchRepositoryNames(branches []domain.Branch, project domain.Project) {
	for i := range branches {
		if branches[i].Repository == project.ID {
			branches[i].Repository = project.Name
		}
	}
}

// fixPipelineRepositoryNames updates pipeline repository names from project ID to project name.
func fixPipelineRepositoryNames(pipelines []domain.Pipeline, project domain.Project) {
	for i := range pipelines {
		if pipelines[i].Repository == "" || pipelines[i].Repository == project.ID {
			pipelines[i].Repository = project.Name
		}
	}
}

// fixMRRepositoryNames updates merge request repository names from title/ID to project name.
func fixMRRepositoryNames(mrs []domain.MergeRequest, project domain.Project) {
	for i := range mrs {
		if mrs[i].Repository == "" || mrs[i].Repository == mrs[i].Title {
			mrs[i].Repository = project.Name
		}
	}
}

// fixIssueRepositoryNames updates issue repository names from title/ID to project name.
func fixIssueRepositoryNames(issues []domain.Issue, project domain.Project) {
	for i := range issues {
		if issues[i].Repository == "" || issues[i].Repository == issues[i].Title {
			issues[i].Repository = project.Name
		}
	}
}

// Worker pool helper to limit concurrent goroutines

// processProjectsConcurrently processes a list of projects with limited concurrency.
// Uses a worker pool pattern to prevent launching unbounded goroutines.
func processProjectsConcurrently[T any](
	ctx context.Context,
	projects []domain.Project,
	maxWorkers int,
	processFunc func(context.Context, domain.Project) ([]T, error),
) []T {
	if maxWorkers <= 0 {
		maxWorkers = MaxConcurrentWorkers
	}

	results := make(chan []T, len(projects))
	semaphore := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup

	for _, proj := range projects {
		wg.Add(1)
		go func(p domain.Project) {
			defer wg.Done()

			// Acquire semaphore slot
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				return
			}

			// Process project
			items, err := processFunc(ctx, p)
			if err == nil && len(items) > 0 {
				results <- items
			}
		}(proj)
	}

	// Wait for all goroutines to complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	var allResults []T
	for items := range results {
		allResults = append(allResults, items...)
	}

	return allResults
}

// RegisterClient registers a CI/CD platform client.
// Follows Open/Closed Principle - can add new platforms without modifying service.
func (s *PipelineService) RegisterClient(platform string, client api.Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clients[platform] = client
}

// ForceRefreshAllCaches forces all clients to fetch fresh data page-by-page and populate their caches.
// This is used by the background refresher to initially populate caches.
// Fetches one page of projects at a time, then fetches all related data for those projects.
func (s *PipelineService) ForceRefreshAllCaches(ctx context.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var wg sync.WaitGroup
	errChan := make(chan error, len(s.clients))

	for platform, client := range s.clients {
		wg.Add(1)
		go func(p string, c api.Client) {
			defer wg.Done()
			if err := s.forceRefreshClientPageByPage(ctx, p, c); err != nil {
				errChan <- fmt.Errorf("%s: %w", p, err)
			}
		}(platform, client)
	}

	wg.Wait()
	close(errChan)

	// Collect and log errors, but don't fail if some platforms succeed
	var errs []error
	successCount := len(s.clients)
	for err := range errChan {
		errs = append(errs, err)
		successCount--
		log.Printf("Force refresh error: %v", err)
	}

	// Only fail if ALL platforms failed
	if len(errs) > 0 && successCount == 0 {
		return fmt.Errorf("all platforms failed: %v", errs)
	}

	// Some platforms succeeded, consider it a success
	return nil
}

// forceRefreshClientPageByPage fetches projects page-by-page and all related data for each page.
func (s *PipelineService) forceRefreshClientPageByPage(ctx context.Context, platform string, client api.Client) error {
	log.Printf("[%s] Starting page-by-page refresh...", platform)

	// Check if client is a stale caching client
	type staleCacher interface {
		GetProjectsPage(ctx context.Context, page int) ([]domain.Project, bool, error)
		ForceRefresh(ctx context.Context, key string) error
		PopulateProjects(projects []domain.Project)
	}

	cacher, ok := client.(staleCacher)
	if !ok {
		return fmt.Errorf("client does not support page-by-page refresh")
	}

	page := 1
	var allProjects []domain.Project

	for {
		// Check if context has been cancelled (timeout or shutdown)
		select {
		case <-ctx.Done():
			return fmt.Errorf("refresh cancelled: %w", ctx.Err())
		default:
		}

		// Fetch one page of projects
		projects, hasNext, err := cacher.GetProjectsPage(ctx, page)
		if err != nil {
			return fmt.Errorf("failed to fetch page %d: %w", page, err)
		}

		if len(projects) == 0 {
			break
		}

		// Process each project individually and cache incrementally
		for _, project := range projects {
			// Add this project to accumulator
			allProjects = append(allProjects, project)

			// Cache immediately after each project (1, 2, 3... 17...)
			cacher.PopulateProjects(allProjects)

			// Fetch data for this single project
			if err := s.forceRefreshDataForProjects(ctx, platform, cacher, []domain.Project{project}); err != nil {
				log.Printf("[%s] Warning: failed to fetch data for project %s: %v", platform, project.Name, err)
			}
		}

		if !hasNext {
			break
		}

		page++
	}

	return nil
}

// forceRefreshDataForProjects fetches all related data for a batch of projects.
func (s *PipelineService) forceRefreshDataForProjects(ctx context.Context, platform string, client interface{ ForceRefresh(context.Context, string) error }, projects []domain.Project) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(projects)*4) // 4 operations per project max

	for _, project := range projects {
		projectID := project.ID
		projectName := project.Name
		defaultBranch := project.DefaultBranch
		if defaultBranch == "" {
			defaultBranch = "main"
		}

		// Fetch default branch pipeline
		wg.Add(1)
		go func(pid, pname, branch string) {
			defer wg.Done()
			key := fmt.Sprintf("GetLatestPipeline:%s:%s", pid, branch)
			if err := client.ForceRefresh(ctx, key); err != nil {
				// Try master if main fails
				if branch == "main" {
					key = fmt.Sprintf("GetLatestPipeline:%s:master", pid)
					if err := client.ForceRefresh(ctx, key); err != nil {
						log.Printf("Failed to fetch pipeline for %s on both main and master branches: %v", pname, err)
					}
				}
			}
		}(projectID, projectName, defaultBranch)

		// Fetch branches (use 200 to match GetDefaultBranchForProject cache key)
		wg.Add(1)
		go func(pid, pname string) {
			defer wg.Done()
			key := fmt.Sprintf("GetBranches:%s:200", pid)
			if err := client.ForceRefresh(ctx, key); err != nil {
				errChan <- fmt.Errorf("GetBranches %s: %w", pname, err)
			}
		}(projectID, projectName)

		// Fetch pipelines for repository detail page
		wg.Add(1)
		go func(pid string) {
			defer wg.Done()
			key := fmt.Sprintf("GetPipelines:%s:50", pid)
			if err := client.ForceRefresh(ctx, key); err != nil {
				// Ignore errors - will retry on next refresh
			}
		}(projectID)

		// Fetch merge requests
		wg.Add(1)
		go func(pid string) {
			defer wg.Done()
			key := fmt.Sprintf("GetMergeRequests:%s", pid)
			if err := client.ForceRefresh(ctx, key); err != nil {
				// Ignore errors - not all clients support this
			}
		}(projectID)

		// Fetch issues
		wg.Add(1)
		go func(pid string) {
			defer wg.Done()
			key := fmt.Sprintf("GetIssues:%s", pid)
			if err := client.ForceRefresh(ctx, key); err != nil {
				// Ignore errors - not all clients support this
			}
		}(projectID)
	}

	wg.Wait()
	close(errChan)

	// Collect errors (but don't fail - just log)
	for err := range errChan {
		log.Printf("[%s] Warning: %v", platform, err)
	}

	return nil
}

// getClientForPlatform returns the appropriate client for a given platform.
// Returns nil if no client is registered for the platform.
func (s *PipelineService) getClientForPlatform(platform string) api.Client {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.clients[platform]
}

// GetPipelinesForProject retrieves pipelines for a single project.
func (s *PipelineService) GetPipelinesForProject(ctx context.Context, projectID string, limit int) ([]domain.Pipeline, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Try to find which platform this project belongs to
	projects, err := s.getAllProjectsLocked(ctx)
	if err != nil {
		return nil, err
	}

	var platform string
	for _, p := range projects {
		if p.ID == projectID {
			platform = p.Platform
			break
		}
	}

	if platform == "" {
		return nil, fmt.Errorf("project not found: %s", projectID)
	}

	client := s.getClientForPlatform(platform)
	if client == nil {
		return nil, fmt.Errorf("no client for platform: %s", platform)
	}

	pipelines, err := client.GetPipelines(ctx, projectID, limit)
	if err != nil {
		return nil, err
	}

	if len(pipelines) == 0 {
		log.Printf("[PipelineService] GetPipelinesForProject: project %s has 0 pipelines (may be cache miss)", projectID)
	}

	return pipelines, nil
}

// GetBranchesForProject retrieves branches with pipelines for a single project.
func (s *PipelineService) GetBranchesForProject(ctx context.Context, project domain.Project, limit int) ([]domain.BranchWithPipeline, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	client := s.getClientForPlatform(project.Platform)
	if client == nil {
		return nil, fmt.Errorf("no client for platform: %s", project.Platform)
	}

	// Get branches
	branches, err := client.GetBranches(ctx, project.ID, limit)
	if err != nil {
		return nil, err
	}

	// Fill in repository name
	fixBranchRepositoryNames(branches, project)

	// Get pipeline for each branch
	var results []domain.BranchWithPipeline
	for _, branch := range branches {
		var pipeline *domain.Pipeline
		p, err := client.GetLatestPipeline(ctx, branch.ProjectID, branch.Name)
		if err == nil && p != nil {
			pipeline = p
		}

		results = append(results, domain.BranchWithPipeline{
			Branch:   branch,
			Pipeline: pipeline,
		})
	}

	return results, nil
}

// GetDefaultBranchForProject retrieves only the default branch with its pipeline for a project.
// This is optimized for cases where you only need the default branch (e.g., repository listing).
// Returns: defaultBranch, defaultPipeline, totalBranchCount, error
func (s *PipelineService) GetDefaultBranchForProject(ctx context.Context, project domain.Project) (*domain.Branch, *domain.Pipeline, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	client := s.getClientForPlatform(project.Platform)
	if client == nil {
		return nil, nil, 0, fmt.Errorf("no client for platform: %s", project.Platform)
	}

	// Get branches (just metadata, not pipelines yet)
	branches, err := client.GetBranches(ctx, project.ID, 200)
	if err != nil {
		log.Printf("[GetDefaultBranchForProject] Failed to get branches for %s: %v", project.Name, err)
		return nil, nil, 0, err
	}

	// Fix repository names for all branches
	fixBranchRepositoryNames(branches, project)

	// Find default branch - first try by IsDefault flag, then by project.DefaultBranch name
	var defaultBranch *domain.Branch
	for i := range branches {
		if branches[i].IsDefault {
			defaultBranch = &branches[i]
			break
		}
	}

	// If not found by flag, search by name from project metadata
	if defaultBranch == nil && project.DefaultBranch != "" {
		for i := range branches {
			if branches[i].Name == project.DefaultBranch {
				defaultBranch = &branches[i]
				log.Printf("[GetDefaultBranchForProject] Found default branch for %s by name: %s", project.Name, project.DefaultBranch)
				break
			}
		}
	}

	// If still not found, fetch the default branch directly by name using project metadata
	if defaultBranch == nil && project.DefaultBranch != "" {
		log.Printf("[GetDefaultBranchForProject] Default branch %s not in first %d branches for %s, fetching directly",
			project.DefaultBranch, len(branches), project.Name)

		branch, err := client.GetBranch(ctx, project.ID, project.DefaultBranch)
		if err != nil {
			log.Printf("[GetDefaultBranchForProject] Failed to fetch default branch %s for %s: %v",
				project.DefaultBranch, project.Name, err)
		} else if branch != nil {
			// Fix repository name
			if branch.Repository == project.ID {
				branch.Repository = project.Name
			}
			defaultBranch = branch
			log.Printf("[GetDefaultBranchForProject] Successfully fetched default branch %s for %s directly",
				project.DefaultBranch, project.Name)
		}
	}

	if defaultBranch == nil {
		log.Printf("[GetDefaultBranchForProject] No default branch found for %s (total branches: %d, project.DefaultBranch: %s)",
			project.Name, len(branches), project.DefaultBranch)
	} else if defaultBranch.LastCommitDate.IsZero() {
		log.Printf("[GetDefaultBranchForProject] Default branch %s for %s has zero commit date (Author: %s)", defaultBranch.Name, project.Name, defaultBranch.CommitAuthor)
	}

	// Get pipeline only for default branch
	var defaultPipeline *domain.Pipeline
	if defaultBranch != nil {
		p, err := client.GetLatestPipeline(ctx, defaultBranch.ProjectID, defaultBranch.Name)
		if err == nil && p != nil {
			defaultPipeline = p
		}
	}

	return defaultBranch, defaultPipeline, len(branches), nil
}

// isWhitelisted checks if a project is in the appropriate whitelist.
// Returns true if whitelist is empty (allow all) or if project is in whitelist.
func (s *PipelineService) isWhitelisted(project domain.Project) bool {
	var whitelist []string

	// Select the appropriate whitelist based on platform
	switch project.Platform {
	case "gitlab":
		whitelist = s.gitlabWhitelist
	case "github":
		whitelist = s.githubWhitelist
	default:
		// Unknown platform - deny by default if any whitelist is set
		return len(s.gitlabWhitelist) == 0 && len(s.githubWhitelist) == 0
	}

	// No whitelist for this platform means allow all
	if len(whitelist) == 0 {
		return true
	}

	// Check if project ID is in whitelist
	for _, allowed := range whitelist {
		if project.ID == allowed {
			return true
		}
	}

	return false
}

// GetTotalProjectCount retrieves the total count of projects from all configured platforms.
func (s *PipelineService) GetTotalProjectCount(ctx context.Context) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	totalCount := 0

	// Get count from GitLab
	if gitlabClient := s.getClientForPlatform("gitlab"); gitlabClient != nil {
		count, err := gitlabClient.GetProjectCount(ctx)
		if err != nil {
			return 0, fmt.Errorf("gitlab: %w", err)
		}
		totalCount += count
	}

	// Get count from GitHub
	if githubClient := s.getClientForPlatform("github"); githubClient != nil {
		count, err := githubClient.GetProjectCount(ctx)
		if err != nil {
			return 0, fmt.Errorf("github: %w", err)
		}
		totalCount += count
	}

	return totalCount, nil
}

// GetProjectsPageByPlatform retrieves a single page of projects from a specific platform.
func (s *PipelineService) GetProjectsPageByPlatform(ctx context.Context, platform string, page int) ([]domain.Project, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	client := s.getClientForPlatform(platform)
	if client == nil {
		return nil, false, fmt.Errorf("no client for platform: %s", platform)
	}

	projects, hasNext, err := client.GetProjectsPage(ctx, page)
	if err != nil {
		return nil, false, fmt.Errorf("%s: %w", platform, err)
	}

	// Filter projects based on whitelist
	var whitelist []string
	if platform == domain.PlatformGitLab {
		whitelist = s.gitlabWhitelist
	} else if platform == domain.PlatformGitHub {
		whitelist = s.githubWhitelist
	}

	if len(whitelist) > 0 {
		filtered := make([]domain.Project, 0, len(projects))
		for _, project := range projects {
			if s.isWhitelisted(project) {
				filtered = append(filtered, project)
			}
		}
		projects = filtered
	}

	return projects, hasNext, nil
}

// GetAllProjects retrieves projects from all configured platforms.
func (s *PipelineService) GetAllProjects(ctx context.Context) ([]domain.Project, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.getAllProjectsLocked(ctx)
}

// getAllProjectsLocked is an internal helper that assumes the caller holds s.mu (read or write lock).
// This prevents deadlock when called from other methods that already hold the lock.
func (s *PipelineService) getAllProjectsLocked(ctx context.Context) ([]domain.Project, error) {
	var allProjects []domain.Project
	var mu sync.Mutex
	var wg sync.WaitGroup
	errChan := make(chan error, len(s.clients))

	// Fetch projects from all platforms concurrently
	for platform, client := range s.clients {
		wg.Add(1)
		go func(p string, c api.Client) {
			defer wg.Done()

			projects, err := c.GetProjects(ctx)
			if err != nil {
				errChan <- fmt.Errorf("%s: %w", p, err)
				return
			}

			mu.Lock()
			allProjects = append(allProjects, projects...)
			mu.Unlock()
		}(platform, client)
	}

	wg.Wait()
	close(errChan)

	// Collect all errors from closed channel
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return nil, errs[0]
	}

	// Filter projects based on whitelist
	if len(s.gitlabWhitelist) > 0 || len(s.githubWhitelist) > 0 {
		filtered := make([]domain.Project, 0, len(allProjects))
		for _, project := range allProjects {
			if s.isWhitelisted(project) {
				filtered = append(filtered, project)
			}
		}
		allProjects = filtered
	}

	return allProjects, nil
}

// GetPipelinesByProject retrieves pipelines for specific project IDs.
// projectIDs can be from any platform (service auto-detects).
func (s *PipelineService) GetPipelinesByProject(ctx context.Context, projectIDs []string) ([]domain.Pipeline, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(projectIDs) == 0 {
		return []domain.Pipeline{}, nil
	}

	var allPipelines []domain.Pipeline
	var mu sync.Mutex
	var wg sync.WaitGroup
	errChan := make(chan error, len(projectIDs)*len(s.clients))

	// For each project, try all clients (client will fail if project doesn't belong to it)
	for _, projectID := range projectIDs {
		for platform, client := range s.clients {
			wg.Add(1)
			go func(projID, plat string, c api.Client) {
				defer wg.Done()

				// Get latest pipeline (branch can be "main" or "master" by default)
				pipeline, err := c.GetLatestPipeline(ctx, projID, "main")
				if err != nil {
					// Try master branch if main fails
					pipeline, err = c.GetLatestPipeline(ctx, projID, "master")
					if err != nil {
						return // Silently skip - project might not belong to this platform
					}
				}

				if pipeline != nil {
					pipeline.Repository = projID

					mu.Lock()
					allPipelines = append(allPipelines, *pipeline)
					mu.Unlock()
				}
			}(projectID, platform, client)
		}
	}

	wg.Wait()
	close(errChan)

	return allPipelines, nil
}

// GetLatestPipelines retrieves the most recent pipeline for each project.
func (s *PipelineService) GetLatestPipelines(ctx context.Context) ([]domain.Pipeline, error) {
	projects, err := s.GetAllProjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get projects: %w", err)
	}

	projectIDs := make([]string, len(projects))
	for i, p := range projects {
		projectIDs[i] = p.ID
	}

	return s.GetPipelinesByProject(ctx, projectIDs)
}

// GroupPipelinesByWorkflow groups pipelines by their workflow name.
// Pipelines without workflow (GitLab) are grouped under empty string key.
func (s *PipelineService) GroupPipelinesByWorkflow(pipelines []domain.Pipeline) map[string][]domain.Pipeline {
	grouped := make(map[string][]domain.Pipeline)

	for _, p := range pipelines {
		key := ""
		if p.WorkflowName != nil {
			key = *p.WorkflowName
		}
		grouped[key] = append(grouped[key], p)
	}

	return grouped
}

// GetPipelinesByWorkflow retrieves pipelines for a specific workflow.
func (s *PipelineService) GetPipelinesByWorkflow(ctx context.Context, projectID, workflowID string, limit int) ([]domain.Pipeline, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, client := range s.clients {
		if wc, ok := client.(api.WorkflowClient); ok {
			pipelines, err := wc.GetWorkflowRuns(ctx, projectID, workflowID, limit)
			if err == nil && len(pipelines) > 0 {
				return pipelines, nil
			}
		}
	}

	return nil, fmt.Errorf("workflow not found")
}

// RepositoryWithRuns represents a repository with its recent pipeline runs.
type RepositoryWithRuns struct {
	Project domain.Project
	Runs    []domain.Pipeline
}

// GetRepositoriesWithRecentRuns retrieves all repositories with their recent pipeline runs.
func (s *PipelineService) GetRepositoriesWithRecentRuns(ctx context.Context, runsPerRepo int) ([]RepositoryWithRuns, error) {
	projects, err := s.GetAllProjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get projects: %w", err)
	}

	// No need for lock here - getClientForPlatform has its own lock
	var results []RepositoryWithRuns
	var mu sync.Mutex
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, MaxConcurrentWorkers)

	// For each project, fetch recent runs with limited concurrency
	for _, project := range projects {
		wg.Add(1)
		go func(proj domain.Project) {
			defer wg.Done()

			// Acquire semaphore slot
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				return
			}

			var runs []domain.Pipeline

			// Get the appropriate client for this project's platform
			client := s.getClientForPlatform(proj.Platform)
			if client != nil {
				pipelines, err := client.GetPipelines(ctx, proj.ID, runsPerRepo)
				if err == nil && len(pipelines) > 0 {
					// Fill in repository name from project
					fixPipelineRepositoryNames(pipelines, proj)
					runs = pipelines
				}
			}

			mu.Lock()
			results = append(results, RepositoryWithRuns{
				Project: proj,
				Runs:    runs,
			})
			mu.Unlock()
		}(project)
	}

	wg.Wait()

	// Sort repositories by latest run time (most recent first)
	// Repositories with no runs appear at the end
	sort.Slice(results, func(i, j int) bool {
		timeI := getLatestRunTime(results[i])
		timeJ := getLatestRunTime(results[j])
		// Sort: most recent first (later time comes before earlier time)
		return timeI.After(timeJ)
	})

	return results, nil
}

// getLatestRunTime returns the latest UpdatedAt time from a repository's runs.
// Returns zero time if there are no runs.
func getLatestRunTime(repo RepositoryWithRuns) time.Time {
	var latest time.Time
	for _, run := range repo.Runs {
		if run.UpdatedAt.After(latest) {
			latest = run.UpdatedAt
		}
	}
	return latest
}

// GetRecentPipelines retrieves the most recent pipelines across all projects.
func (s *PipelineService) GetRecentPipelines(ctx context.Context, totalLimit int) ([]domain.Pipeline, error) {
	projects, err := s.GetAllProjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get projects: %w", err)
	}

	// No need for lock here - getClientForPlatform has its own lock
	var allPipelines []domain.Pipeline
	var mu sync.Mutex
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, MaxConcurrentWorkers)

	// Calculate how many pipelines to fetch per project
	// Fetch more than needed, then we'll sort and limit
	pipelinesPerProject := DefaultPipelinesPerProject
	if len(projects) > 0 {
		pipelinesPerProject = (totalLimit / len(projects)) + PipelineFetchBuffer
	}

	// For each project, fetch recent runs with limited concurrency
	for _, project := range projects {
		wg.Add(1)
		go func(proj domain.Project) {
			defer wg.Done()

			// Acquire semaphore slot
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				return
			}

			// Get the appropriate client for this project's platform
			client := s.getClientForPlatform(proj.Platform)
			if client != nil {
				pipelines, err := client.GetPipelines(ctx, proj.ID, pipelinesPerProject)
				if err == nil && len(pipelines) > 0 {
					// Fill in repository name from project
					fixPipelineRepositoryNames(pipelines, proj)

					mu.Lock()
					allPipelines = append(allPipelines, pipelines...)
					mu.Unlock()
				}
			}
		}(project)
	}

	wg.Wait()

	// Sort by UpdatedAt (most recent first)
	sort.Slice(allPipelines, func(i, j int) bool {
		return allPipelines[i].UpdatedAt.After(allPipelines[j].UpdatedAt)
	})

	// Limit to totalLimit
	if len(allPipelines) > totalLimit {
		allPipelines = allPipelines[:totalLimit]
	}

	return allPipelines, nil
}

// GetAllMergeRequests retrieves all open merge requests/pull requests across all projects.
func (s *PipelineService) GetAllMergeRequests(ctx context.Context) ([]domain.MergeRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Get all projects first (using internal locked version to avoid deadlock)
	projects, err := s.getAllProjectsLocked(ctx)
	if err != nil {
		return nil, err
	}

	// Fetch merge requests concurrently with limited workers
	type result struct {
		mrs []domain.MergeRequest
		err error
	}

	results := make(chan result, len(projects))
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, MaxConcurrentWorkers)

	for _, project := range projects {
		wg.Add(1)
		go func(proj domain.Project) {
			defer wg.Done()

			// Acquire semaphore slot
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				return
			}

			// Get the appropriate client for this project's platform
			c := s.getClientForPlatform(proj.Platform)
			if c == nil {
				results <- result{mrs: nil, err: nil}
				return
			}

			// Check if client supports ExtendedClient interface
			client, ok := c.(api.ExtendedClient)
			if !ok {
				results <- result{mrs: nil, err: nil}
				return
			}

			mrs, err := client.GetMergeRequests(ctx, proj.ID)
			if err != nil {
				results <- result{mrs: nil, err: fmt.Errorf("failed to get MRs for %s: %w", proj.Name, err)}
				return
			}

			// Set repository name from project
			fixMRRepositoryNames(mrs, proj)

			results <- result{mrs: mrs, err: nil}
		}(project)
	}

	// Wait for all goroutines to complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect all merge requests
	var allMRs []domain.MergeRequest
	for r := range results {
		if r.err != nil {
			// Log error but continue with other projects
			log.Printf("Error fetching merge requests: %v", r.err)
			continue
		}
		allMRs = append(allMRs, r.mrs...)
	}

	// Sort by updated time (most recent first)
	sort.Slice(allMRs, func(i, j int) bool {
		return allMRs[i].UpdatedAt.After(allMRs[j].UpdatedAt)
	})

	return allMRs, nil
}

// GetMergeRequestsForProject retrieves open merge requests/pull requests for a single project.
func (s *PipelineService) GetMergeRequestsForProject(ctx context.Context, project domain.Project) ([]domain.MergeRequest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Get the appropriate client for this project's platform
	c := s.getClientForPlatform(project.Platform)
	if c == nil {
		return nil, fmt.Errorf("no client for platform: %s", project.Platform)
	}

	// Check if client supports ExtendedClient interface
	client, ok := c.(api.ExtendedClient)
	if !ok {
		return []domain.MergeRequest{}, nil // Platform doesn't support MRs, return empty list
	}

	mrs, err := client.GetMergeRequests(ctx, project.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get MRs for %s: %w", project.Name, err)
	}

	// Set repository name from project
	fixMRRepositoryNames(mrs, project)

	return mrs, nil
}

// GetUserProfiles retrieves user profiles from all configured platforms.
func (s *PipelineService) GetUserProfiles(ctx context.Context) ([]domain.UserProfile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var profiles []domain.UserProfile
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Fetch user profiles from all platforms concurrently
	for platform, c := range s.clients {
		// Check if client supports UserClient interface
		userClient, ok := c.(api.UserClient)
		if !ok {
			continue // Skip clients that don't support user profiles
		}

		wg.Add(1)
		go func(p string, client api.UserClient) {
			defer wg.Done()

			profile, err := client.GetCurrentUser(ctx)
			if err != nil {
				// Log error but don't fail the whole operation
				log.Printf("Failed to get user profile for %s: %v", p, err)
				return
			}

			// Check if profile is nil (cache miss in stale cache)
			if profile == nil {
				return
			}

			mu.Lock()
			profiles = append(profiles, *profile)
			mu.Unlock()
		}(platform, userClient)
	}

	wg.Wait()

	return profiles, nil
}

// GetAllIssues retrieves all open issues across all projects.
func (s *PipelineService) GetAllIssues(ctx context.Context) ([]domain.Issue, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Get all projects first (using internal locked version to avoid deadlock)
	projects, err := s.getAllProjectsLocked(ctx)
	if err != nil {
		return nil, err
	}

	// Fetch issues concurrently
	type result struct {
		issues []domain.Issue
		err    error
	}

	results := make(chan result, len(projects))
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, MaxConcurrentWorkers)

	for _, project := range projects {
		wg.Add(1)
		go func(proj domain.Project) {
			defer wg.Done()

			// Limit concurrent goroutines
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				return
			}

			// Get the appropriate client for this project's platform
			c := s.getClientForPlatform(proj.Platform)
			if c == nil {
				results <- result{issues: nil, err: nil}
				return
			}

			// Check if client supports ExtendedClient interface
			client, ok := c.(api.ExtendedClient)
			if !ok {
				results <- result{issues: nil, err: nil}
				return
			}

			issues, err := client.GetIssues(ctx, proj.ID)
			if err != nil {
				results <- result{issues: nil, err: fmt.Errorf("failed to get issues for %s: %w", proj.Name, err)}
				return
			}

			// Set repository name from project
			fixIssueRepositoryNames(issues, proj)

			results <- result{issues: issues, err: nil}
		}(project)
	}

	// Wait for all goroutines to complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect all issues
	var allIssues []domain.Issue
	for r := range results {
		if r.err != nil {
			// Log error but continue with other projects
			log.Printf("Error fetching issues: %v", r.err)
			continue
		}
		allIssues = append(allIssues, r.issues...)
	}

	// Sort by updated time (most recent first)
	sort.Slice(allIssues, func(i, j int) bool {
		return allIssues[i].UpdatedAt.After(allIssues[j].UpdatedAt)
	})

	return allIssues, nil
}

// GetAllBranches retrieves all branches across all projects.
func (s *PipelineService) GetAllBranches(ctx context.Context, limit int) ([]domain.Branch, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Get all projects first (using internal locked version to avoid deadlock)
	projects, err := s.getAllProjectsLocked(ctx)
	if err != nil {
		return nil, err
	}

	// Fetch branches concurrently with limited concurrency
	type result struct {
		branches []domain.Branch
		err      error
	}

	results := make(chan result, len(projects))
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, MaxConcurrentWorkers)

	for _, project := range projects {
		wg.Add(1)
		go func(proj domain.Project) {
			defer wg.Done()

			// Limit concurrent goroutines
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				return
			}

			// Get the appropriate client for this project's platform
			client := s.getClientForPlatform(proj.Platform)
			if client != nil {
				branches, err := client.GetBranches(ctx, proj.ID, limit)
				if err == nil && len(branches) > 0 {
					// Fill in repository name from project
					fixBranchRepositoryNames(branches, proj)
					results <- result{branches: branches, err: nil}
					return
				}
			}

			results <- result{branches: nil, err: nil}
		}(project)
	}

	// Wait for all goroutines
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect all branches
	var allBranches []domain.Branch
	for r := range results {
		if r.err != nil {
			log.Printf("Error fetching branches: %v", r.err)
			continue
		}
		allBranches = append(allBranches, r.branches...)
	}

	// Sort by last commit date (most recent first)
	sort.Slice(allBranches, func(i, j int) bool {
		return allBranches[i].LastCommitDate.After(allBranches[j].LastCommitDate)
	})

	return allBranches, nil
}

// GetBranchesWithPipelines retrieves branches with their latest pipeline status.
func (s *PipelineService) GetBranchesWithPipelines(ctx context.Context, limit int) ([]domain.BranchWithPipeline, error) {
	branches, err := s.GetAllBranches(ctx, limit)
	if err != nil {
		return nil, err
	}

	// For each branch, try to get latest pipeline with limited concurrency
	var results []domain.BranchWithPipeline
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, branch := range branches {
		wg.Add(1)
		go func(b domain.Branch) {
			defer wg.Done()

			// Try to get latest pipeline for this branch
			var pipeline *domain.Pipeline
			client := s.getClientForPlatform(b.Platform)
			if client != nil {
				p, err := client.GetLatestPipeline(ctx, b.ProjectID, b.Name)
				if err == nil && p != nil {
					pipeline = p
				}
			}

			mu.Lock()
			results = append(results, domain.BranchWithPipeline{
				Branch:   b,
				Pipeline: pipeline,
			})
			mu.Unlock()
		}(branch)
	}

	wg.Wait()

	// Sort by branch commit date (most recent first)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Branch.LastCommitDate.After(results[j].Branch.LastCommitDate)
	})

	return results, nil
}

// FilterBranchesByAuthor filters branches to those authored by the given usernames.
// Matches against the CommitAuthor field (case-insensitive contains), platform-aware.
func (s *PipelineService) FilterBranchesByAuthor(branches []domain.BranchWithPipeline, gitlabUsername, githubUsername string) []domain.BranchWithPipeline {
	if gitlabUsername == "" && githubUsername == "" {
		return branches
	}

	gitlabUsername = strings.ToLower(gitlabUsername)
	githubUsername = strings.ToLower(githubUsername)

	var filtered []domain.BranchWithPipeline

	for _, b := range branches {
		author := strings.ToLower(b.Branch.CommitAuthor)

		// Match based on platform
		if b.Branch.Platform == "gitlab" && gitlabUsername != "" {
			if strings.Contains(author, gitlabUsername) {
				filtered = append(filtered, b)
			}
		} else if b.Branch.Platform == "github" && githubUsername != "" {
			if strings.Contains(author, githubUsername) {
				filtered = append(filtered, b)
			}
		}
	}

	return filtered
}
