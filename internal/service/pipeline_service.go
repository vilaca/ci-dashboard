package service

import (
	"context"
	"fmt"
	"sync"

	"github.com/vilaca/ci-dashboard/internal/api"
	"github.com/vilaca/ci-dashboard/internal/domain"
)

// PipelineService handles business logic for pipeline operations.
// Follows Single Responsibility Principle - orchestrates pipeline operations.
type PipelineService struct {
	clients map[string]api.Client // platform name -> client
	mu      sync.RWMutex
}

// NewPipelineService creates a new pipeline service.
func NewPipelineService() *PipelineService {
	return &PipelineService{
		clients: make(map[string]api.Client),
	}
}

// RegisterClient registers a CI/CD platform client.
// Follows Open/Closed Principle - can add new platforms without modifying service.
func (s *PipelineService) RegisterClient(platform string, client api.Client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clients[platform] = client
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
