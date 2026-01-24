package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/vilaca/ci-dashboard/internal/api"
	"github.com/vilaca/ci-dashboard/internal/domain"
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

	// Collect errors
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("force refresh errors: %v", errs)
	}

	return nil
}

// forceRefreshClientPageByPage fetches projects page-by-page and all related data for each page.
func (s *PipelineService) forceRefreshClientPageByPage(ctx context.Context, platform string, client api.Client) error {
	fmt.Printf("[%s] Starting page-by-page refresh...\n", platform)

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
				fmt.Printf("[%s] Warning: failed to fetch data for project %s: %v\n", platform, project.Name, err)
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
		go func(pid, branch string) {
			defer wg.Done()
			key := fmt.Sprintf("GetLatestPipeline:%s:%s", pid, branch)
			if err := client.ForceRefresh(ctx, key); err != nil {
				// Try master if main fails
				if branch == "main" {
					key = fmt.Sprintf("GetLatestPipeline:%s:master", pid)
					_ = client.ForceRefresh(ctx, key)
				}
			}
		}(projectID, defaultBranch)

		// Fetch branches
		wg.Add(1)
		go func(pid, pname string) {
			defer wg.Done()
			key := fmt.Sprintf("GetBranches:%s:50", pid)
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
		fmt.Printf("[%s] Warning: %v\n", platform, err)
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
	projects, err := s.GetAllProjects(ctx)
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
	for i := range branches {
		if branches[i].Repository == project.ID {
			branches[i].Repository = project.Name
		}
	}

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
	branches, err := client.GetBranches(ctx, project.ID, 50)
	if err != nil {
		return nil, nil, 0, err
	}

	// Find default branch
	var defaultBranch *domain.Branch
	for i := range branches {
		if branches[i].IsDefault {
			// Fix repository name
			if branches[i].Repository == project.ID {
				branches[i].Repository = project.Name
			}
			defaultBranch = &branches[i]
			break
		}
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
	if platform == "gitlab" {
		whitelist = s.gitlabWhitelist
	} else if platform == "github" {
		whitelist = s.githubWhitelist
	}

	if len(whitelist) > 0 {
		filtered := make([]domain.Project, 0)
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

	// Check for errors
	if len(errChan) > 0 {
		return nil, <-errChan
	}

	// Filter projects based on whitelist
	if len(s.gitlabWhitelist) > 0 || len(s.githubWhitelist) > 0 {
		filtered := make([]domain.Project, 0)
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

	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []RepositoryWithRuns
	var mu sync.Mutex
	var wg sync.WaitGroup

	// For each project, fetch recent runs
	for _, project := range projects {
		wg.Add(1)
		go func(proj domain.Project) {
			defer wg.Done()

			var runs []domain.Pipeline

			// Get the appropriate client for this project's platform
			client := s.getClientForPlatform(proj.Platform)
			if client != nil {
				pipelines, err := client.GetPipelines(ctx, proj.ID, runsPerRepo)
				if err == nil && len(pipelines) > 0 {
					// Fill in repository name from project
					for i := range pipelines {
						if pipelines[i].Repository == "" || pipelines[i].Repository == proj.ID {
							pipelines[i].Repository = proj.Name
						}
					}
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
	for i := 0; i < len(results)-1; i++ {
		for j := 0; j < len(results)-i-1; j++ {
			// Get latest run time for repository j
			timeJ := getLatestRunTime(results[j])
			// Get latest run time for repository j+1
			timeJPlus1 := getLatestRunTime(results[j+1])

			// Sort: most recent first (later time comes before earlier time)
			if timeJ.Before(timeJPlus1) {
				results[j], results[j+1] = results[j+1], results[j]
			}
		}
	}

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

	s.mu.RLock()
	defer s.mu.RUnlock()

	var allPipelines []domain.Pipeline
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Calculate how many pipelines to fetch per project
	// Fetch more than needed, then we'll sort and limit
	pipelinesPerProject := 10
	if len(projects) > 0 {
		pipelinesPerProject = (totalLimit / len(projects)) + 5
	}

	// For each project, fetch recent runs
	for _, project := range projects {
		wg.Add(1)
		go func(proj domain.Project) {
			defer wg.Done()

			// Get the appropriate client for this project's platform
			client := s.getClientForPlatform(proj.Platform)
			if client != nil {
				pipelines, err := client.GetPipelines(ctx, proj.ID, pipelinesPerProject)
				if err == nil && len(pipelines) > 0 {
					// Fill in repository name from project
					for i := range pipelines {
						if pipelines[i].Repository == "" || pipelines[i].Repository == proj.ID {
							pipelines[i].Repository = proj.Name
						}
					}

					mu.Lock()
					allPipelines = append(allPipelines, pipelines...)
					mu.Unlock()
				}
			}
		}(project)
	}

	wg.Wait()

	// Sort by UpdatedAt (most recent first)
	// Simple bubble sort for now
	for i := 0; i < len(allPipelines)-1; i++ {
		for j := 0; j < len(allPipelines)-i-1; j++ {
			if allPipelines[j].UpdatedAt.Before(allPipelines[j+1].UpdatedAt) {
				allPipelines[j], allPipelines[j+1] = allPipelines[j+1], allPipelines[j]
			}
		}
	}

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

	// Get all projects first
	projects, err := s.GetAllProjects(ctx)
	if err != nil {
		return nil, err
	}

	// Fetch merge requests concurrently
	type result struct {
		mrs []domain.MergeRequest
		err error
	}

	results := make(chan result, len(projects))
	var wg sync.WaitGroup

	for _, project := range projects {
		wg.Add(1)
		go func(proj domain.Project) {
			defer wg.Done()

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
			for i := range mrs {
				if mrs[i].Repository == "" || mrs[i].Repository == mrs[i].Title {
					mrs[i].Repository = proj.Name
				}
			}

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
			continue
		}
		allMRs = append(allMRs, r.mrs...)
	}

	// Sort by updated time (most recent first)
	for i := 0; i < len(allMRs)-1; i++ {
		for j := 0; j < len(allMRs)-i-1; j++ {
			if allMRs[j].UpdatedAt.Before(allMRs[j+1].UpdatedAt) {
				allMRs[j], allMRs[j+1] = allMRs[j+1], allMRs[j]
			}
		}
	}

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
	for i := range mrs {
		if mrs[i].Repository == "" || mrs[i].Repository == mrs[i].Title {
			mrs[i].Repository = project.Name
		}
	}

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
				fmt.Printf("Failed to get user profile for %s: %v\n", p, err)
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

	// Get all projects first
	projects, err := s.GetAllProjects(ctx)
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

	for _, project := range projects {
		wg.Add(1)
		go func(proj domain.Project) {
			defer wg.Done()

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
			for i := range issues {
				if issues[i].Repository == "" || issues[i].Repository == issues[i].Title {
					issues[i].Repository = proj.Name
				}
			}

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
			continue
		}
		allIssues = append(allIssues, r.issues...)
	}

	// Sort by updated time (most recent first)
	for i := 0; i < len(allIssues)-1; i++ {
		for j := 0; j < len(allIssues)-i-1; j++ {
			if allIssues[j].UpdatedAt.Before(allIssues[j+1].UpdatedAt) {
				allIssues[j], allIssues[j+1] = allIssues[j+1], allIssues[j]
			}
		}
	}

	return allIssues, nil
}

// GetAllBranches retrieves all branches across all projects.
func (s *PipelineService) GetAllBranches(ctx context.Context, limit int) ([]domain.Branch, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Get all projects first
	projects, err := s.GetAllProjects(ctx)
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

	for _, project := range projects {
		wg.Add(1)
		go func(proj domain.Project) {
			defer wg.Done()

			// Get the appropriate client for this project's platform
			client := s.getClientForPlatform(proj.Platform)
			if client != nil {
				branches, err := client.GetBranches(ctx, proj.ID, limit)
				if err == nil && len(branches) > 0 {
					// Fill in repository name from project
					for i := range branches {
						if branches[i].Repository == proj.ID {
							branches[i].Repository = proj.Name
						}
					}
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
			continue
		}
		allBranches = append(allBranches, r.branches...)
	}

	// Sort by last commit date (most recent first)
	for i := 0; i < len(allBranches)-1; i++ {
		for j := 0; j < len(allBranches)-i-1; j++ {
			if allBranches[j].LastCommitDate.Before(allBranches[j+1].LastCommitDate) {
				allBranches[j], allBranches[j+1] = allBranches[j+1], allBranches[j]
			}
		}
	}

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
	for i := 0; i < len(results)-1; i++ {
		for j := 0; j < len(results)-i-1; j++ {
			if results[j].Branch.LastCommitDate.Before(results[j+1].Branch.LastCommitDate) {
				results[j], results[j+1] = results[j+1], results[j]
			}
		}
	}

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
