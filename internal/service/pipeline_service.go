package service

import (
	"context"
	"fmt"
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

			// Try to fetch pipelines from the appropriate client
			for _, client := range s.clients {
				pipelines, err := client.GetPipelines(ctx, proj.ID, runsPerRepo)
				if err == nil && len(pipelines) > 0 {
					// Fill in repository name from project
					for i := range pipelines {
						if pipelines[i].Repository == "" || pipelines[i].Repository == proj.ID {
							pipelines[i].Repository = proj.Name
						}
					}
					runs = pipelines
					break
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

			// Try to fetch pipelines from the appropriate client
			for _, client := range s.clients {
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
					break
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

			// Check if client supports ExtendedClient interface
			var client api.ExtendedClient
			for _, c := range s.clients {
				if ec, ok := c.(api.ExtendedClient); ok {
					client = ec
					break
				}
			}

			if client == nil {
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

			// Check if client supports ExtendedClient interface
			var client api.ExtendedClient
			for _, c := range s.clients {
				if ec, ok := c.(api.ExtendedClient); ok {
					client = ec
					break
				}
			}

			if client == nil {
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
