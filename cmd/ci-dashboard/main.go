package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
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
		log.Printf("  Cache TTL: %ds", cfg.GitLabCacheDurationSeconds)
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
		log.Printf("  Cache TTL: %ds", cfg.GitHubCacheDurationSeconds)
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

	// Create HTTP server with graceful shutdown support
	httpServer := &http.Server{
		Addr:    addr,
		Handler: server,
	}

	// Start server in goroutine
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Wait for shutdown signal
	<-sigChan
	log.Printf("Shutdown signal received, shutting down gracefully...")

	// Stop background refresher first
	if refresher != nil {
		refresher.Stop()
	}

	// Shutdown HTTP server with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Printf("Server stopped")
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
		// StaleTTL: how long to serve stale data
		cacheDuration := time.Duration(cfg.GitLabCacheDurationSeconds) * time.Second
		staleTTL := time.Duration(cfg.StaleCacheTTLSeconds) * time.Second
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
		staleTTL := time.Duration(cfg.StaleCacheTTLSeconds) * time.Second
		cachedGitHubClient := api.NewStaleCachingClient(githubClient, cacheDuration, staleTTL)
		pipelineService.RegisterClient("github", cachedGitHubClient)
	}

	// Create handler with dependencies (Dependency Injection)
	handler := dashboard.NewHandler(dashboard.HandlerConfig{
		Renderer:          renderer,
		Logger:            logger,
		PipelineService:   pipelineService,
		RunsPerRepo:       cfg.RunsPerRepository,
		RecentLimit:       cfg.RecentPipelinesLimit,
		UIRefreshInterval: cfg.UIRefreshIntervalSeconds,
		GitLabUser:        cfg.GitLabCurrentUser,
		GitHubUser:        cfg.GitHubCurrentUser,
	})

	// Register routes
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Create background refresher to pre-populate and maintain cache
	refreshInterval := time.Duration(cfg.BackgroundRefreshIntervalSeconds) * time.Second
	refresher := service.NewBackgroundRefresher(pipelineService, refreshInterval, logger)

	return mux, refresher
}
