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
	server, refresher := buildServer(cfg)

	// Start background refresher to pre-populate and maintain cache
	if refresher != nil {
		refresher.Start()
		defer refresher.Stop()
	}

	// Start server
	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("=== CI Dashboard Configuration ===")
	log.Printf("Server: http://localhost%s", addr)
	log.Printf("Runs per repository: %d", cfg.RunsPerRepository)
	log.Printf("Recent pipelines limit: %d", cfg.RecentPipelinesLimit)

	if cfg.HasGitLabConfig() {
		log.Printf("GitLab: ENABLED")
		log.Printf("  URL: %s", cfg.GitLabURL)
		log.Printf("  Cache TTL: %ds (30 min)", cfg.GitLabCacheDurationSeconds)
		if cfg.GitLabCurrentUser != "" {
			log.Printf("  Current user: %s", cfg.GitLabCurrentUser)
		}
		if len(cfg.GetGitLabWatchedRepos()) > 0 {
			log.Printf("  Watching: %d specific repositories", len(cfg.GetGitLabWatchedRepos()))
		} else {
			log.Printf("  Watching: all accessible repositories")
		}
	} else {
		log.Printf("GitLab: DISABLED (set GITLAB_TOKEN to enable)")
	}

	if cfg.HasGitHubConfig() {
		log.Printf("GitHub: ENABLED")
		log.Printf("  URL: %s", cfg.GitHubURL)
		log.Printf("  Cache TTL: %ds (30 min)", cfg.GitHubCacheDurationSeconds)
		if cfg.GitHubCurrentUser != "" {
			log.Printf("  Current user: %s", cfg.GitHubCurrentUser)
		}
		if len(cfg.GetGitHubWatchedRepos()) > 0 {
			log.Printf("  Watching: %d specific repositories", len(cfg.GetGitHubWatchedRepos()))
		} else {
			log.Printf("  Watching: all accessible repositories")
		}
	} else {
		log.Printf("GitHub: DISABLED (set GITHUB_TOKEN to enable)")
	}

	if !cfg.HasGitLabConfig() && !cfg.HasGitHubConfig() {
		log.Printf("WARNING: No CI platforms configured!")
	}
	log.Printf("==================================")
	log.Printf("Server starting...")

	if err := http.ListenAndServe(addr, server); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

// buildServer wires up all dependencies and returns the configured HTTP handler and background refresher.
// This is the composition root where all dependencies are created and injected.
// Follows SOLID principles and IoC (Inversion of Control).
func buildServer(cfg *config.Config) (http.Handler, *service.BackgroundRefresher) {
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

	// Register CI clients based on configuration with stale-while-revalidate caching
	if cfg.HasGitLabConfig() {
		gitlabClient := gitlab.NewClient(api.ClientConfig{
			BaseURL: cfg.GitLabURL,
			Token:   cfg.GitLabToken,
		}, httpClient)

		// Wrap with stale-while-revalidate caching layer
		// TTL: how long data is considered fresh
		// StaleTTL: how long to serve stale data (24 hours)
		cacheDuration := time.Duration(cfg.GitLabCacheDurationSeconds) * time.Second
		staleTTL := 24 * time.Hour
		cachedGitLabClient := api.NewStaleCachingClient(gitlabClient, cacheDuration, staleTTL)
		pipelineService.RegisterClient("gitlab", cachedGitLabClient)
	}

	if cfg.HasGitHubConfig() {
		githubClient := github.NewClient(api.ClientConfig{
			BaseURL: cfg.GitHubURL,
			Token:   cfg.GitHubToken,
		}, httpClient)

		// Wrap with stale-while-revalidate caching layer
		cacheDuration := time.Duration(cfg.GitHubCacheDurationSeconds) * time.Second
		staleTTL := 24 * time.Hour
		cachedGitHubClient := api.NewStaleCachingClient(githubClient, cacheDuration, staleTTL)
		pipelineService.RegisterClient("github", cachedGitHubClient)
	}

	// Create handler with dependencies (Dependency Injection)
	handler := dashboard.NewHandler(renderer, logger, pipelineService, cfg.RunsPerRepository, cfg.RecentPipelinesLimit, cfg.UIRefreshIntervalSeconds, cfg.GitLabCurrentUser, cfg.GitHubCurrentUser)

	// Register routes
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Create file cache for persistent caching
	fileCache := service.NewFileCache("/tmp/ci-dashboard-cache.json", logger)

	// Create background refresher to pre-populate and maintain cache
	refreshInterval := 5 * time.Minute
	refresher := service.NewBackgroundRefresher(pipelineService, fileCache, refreshInterval, logger)

	return mux, refresher
}
