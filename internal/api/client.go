package api

import (
	"context"

	"github.com/vilaca/ci-dashboard/internal/domain"
)

// Client defines the interface for CI/CD platform clients.
// This follows Interface Segregation Principle - small, focused interface.
// Allows dependency inversion - consumers depend on this interface, not concrete implementations.
type Client interface {
	// GetProjects returns all projects accessible by the configured credentials.
	GetProjects(ctx context.Context) ([]domain.Project, error)

	// GetLatestPipeline returns the most recent pipeline for a given project and branch.
	GetLatestPipeline(ctx context.Context, projectID, branch string) (*domain.Pipeline, error)

	// GetPipelines returns recent pipelines for a given project.
	GetPipelines(ctx context.Context, projectID string, limit int) ([]domain.Pipeline, error)
}

// ClientConfig holds common configuration for API clients.
type ClientConfig struct {
	BaseURL string
	Token   string
}
