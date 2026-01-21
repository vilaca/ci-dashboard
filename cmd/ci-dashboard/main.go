package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/vilaca/ci-dashboard/internal/api"
	"github.com/vilaca/ci-dashboard/internal/api/github"
	"github.com/vilaca/ci-dashboard/internal/api/gitlab"
	"github.com/vilaca/ci-dashboard/internal/config"
	"github.com/vilaca/ci-dashboard/internal/dashboard"
	"github.com/vilaca/ci-dashboard/internal/service"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Wire up dependencies (Dependency Injection / IoC)
	server := buildServer(cfg)

	// Start server
	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("Starting CI Dashboard on http://localhost%s", addr)

	if cfg.HasGitLabConfig() {
		log.Printf("GitLab integration enabled (cache: %ds)", cfg.GitLabCacheDurationSeconds)
		if len(cfg.GetGitLabWatchedRepos()) > 0 {
			log.Printf("GitLab whitelist: %d repositories", len(cfg.GetGitLabWatchedRepos()))
		}
	}
	if cfg.HasGitHubConfig() {
		log.Printf("GitHub integration enabled (cache: %ds)", cfg.GitHubCacheDurationSeconds)
		if len(cfg.GetGitHubWatchedRepos()) > 0 {
			log.Printf("GitHub whitelist: %d repositories", len(cfg.GetGitHubWatchedRepos()))
		}
	}
	if !cfg.HasGitLabConfig() && !cfg.HasGitHubConfig() {
		log.Printf("WARNING: No CI platforms configured. Set GITLAB_TOKEN or GITHUB_TOKEN")
	}

	if err := http.ListenAndServe(addr, server); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// buildServer wires up all dependencies and returns the configured HTTP handler.
// This is the composition root where all dependencies are created and injected.
// Follows SOLID principles and IoC (Inversion of Control).
func buildServer(cfg *config.Config) http.Handler {
	// Create shared dependencies
	logger := dashboard.NewStdLogger()
	renderer := dashboard.NewHTMLRenderer()
	httpClient := &http.Client{
		Timeout: 30 * time.Second, // Set reasonable timeout for API requests
	}

	// Create pipeline service with whitelists
	pipelineService := service.NewPipelineService(
		cfg.GetGitLabWatchedRepos(),
		cfg.GetGitHubWatchedRepos(),
	)

	// Register CI clients based on configuration with caching
	if cfg.HasGitLabConfig() {
		gitlabClient := gitlab.NewClient(api.ClientConfig{
			BaseURL: cfg.GitLabURL,
			Token:   cfg.GitLabToken,
		}, httpClient)

		// Wrap with caching layer
		cacheDuration := time.Duration(cfg.GitLabCacheDurationSeconds) * time.Second
		cachedGitLabClient := api.NewCachingClient(gitlabClient, cacheDuration)
		pipelineService.RegisterClient("gitlab", cachedGitLabClient)
	}

	if cfg.HasGitHubConfig() {
		githubClient := github.NewClient(api.ClientConfig{
			BaseURL: cfg.GitHubURL,
			Token:   cfg.GitHubToken,
		}, httpClient)

		// Wrap with caching layer
		cacheDuration := time.Duration(cfg.GitHubCacheDurationSeconds) * time.Second
		cachedGitHubClient := api.NewCachingClient(githubClient, cacheDuration)
		pipelineService.RegisterClient("github", cachedGitHubClient)
	}

	// Create handler with dependencies (Dependency Injection)
	handler := dashboard.NewHandler(renderer, logger, pipelineService, cfg.RunsPerRepository, cfg.RecentPipelinesLimit, cfg.GitLabCurrentUser, cfg.GitHubCurrentUser)

	// Register routes
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	return mux
}
