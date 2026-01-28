# CI/CD Dashboard - Modular Monolith Architecture

## Table of Contents
1. [Overview](#overview)
2. [Architecture Principles](#architecture-principles)
3. [Module Structure](#module-structure)
4. [Plugin System for Connectors](#plugin-system-for-connectors)
5. [In-Process Communication](#in-process-communication)
6. [Project Structure](#project-structure)
7. [Adding New Platform Connectors](#adding-new-platform-connectors)
8. [Configuration](#configuration)
9. [Deployment](#deployment)
10. [Comparison with Current Implementation](#comparison-with-current-implementation)

---

## Overview

### Goal
Run the entire CI/CD dashboard as a **single binary** on a **single computer** (including user laptops), while maintaining clean module boundaries and making it easy to add new CI/CD platforms.

### Key Characteristics
- ✅ **Single Process**: Everything runs in one Go process
- ✅ **No Network Latency**: Direct function calls (nanoseconds instead of milliseconds)
- ✅ **Lightweight**: Can run on a laptop with minimal resources
- ✅ **Simple Deployment**: Just run `./ci-dashboard` or `docker run`
- ✅ **Plugin Architecture**: Drop in new connectors as Go packages
- ✅ **Clean Boundaries**: Modules communicate through interfaces
- ✅ **Easy Development**: No need for multiple terminals, docker-compose, etc.

---

## Architecture Principles

### Modular Monolith Pattern

A modular monolith maintains the benefits of microservices architecture (modularity, separation of concerns) while running as a single process.

**Key Differences from Traditional Monolith**:
- Clear module boundaries (not just packages, but architectural modules)
- Modules communicate through well-defined interfaces
- Each module can be tested independently
- Low coupling between modules
- High cohesion within modules

**Key Differences from Microservices**:
- Single deployment unit (one binary)
- In-process communication (function calls, not HTTP)
- Shared memory and database
- No network overhead
- Simpler operational model

### Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                    Single Go Process                             │
│                    (./ci-dashboard)                              │
│                                                                  │
│  ┌────────────────────────────────────────────────────────────┐ │
│  │                    HTTP Server Module                       │ │
│  │  (Handlers, Routes, Middleware)                            │ │
│  └────────────────────┬───────────────────────────────────────┘ │
│                       │                                          │
│  ┌────────────────────▼───────────────────────────────────────┐ │
│  │              Aggregation Module                            │ │
│  │  (Orchestrates data from all connectors)                   │ │
│  └────────────────────┬───────────────────────────────────────┘ │
│                       │                                          │
│         ┌─────────────┼─────────────┬──────────────┐            │
│         │             │             │              │            │
│  ┌──────▼──────┐ ┌───▼──────┐ ┌───▼──────┐ ┌────▼──────┐      │
│  │   GitLab    │ │  GitHub  │ │ Jenkins  │ │  BitBucket│      │
│  │  Connector  │ │ Connector│ │ Connector│ │  Connector│      │
│  │   Module    │ │  Module  │ │  Module  │ │   Module  │      │
│  └──────┬──────┘ └───┬──────┘ └───┬──────┘ └────┬──────┘      │
│         │            │            │              │             │
│         └────────────┼────────────┼──────────────┘             │
│                      │            │                            │
│  ┌───────────────────▼────────────▼──────────────────────────┐ │
│  │              Cache Module (In-Memory or Redis)            │ │
│  └───────────────────────────────────────────────────────────┘ │
│                                                                 │
│  ┌───────────────────────────────────────────────────────────┐ │
│  │         Configuration Module (Environment + YAML)         │ │
│  └───────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

---

## Module Structure

### Core Modules

#### 1. HTTP Server Module
**Location**: `internal/http/`

**Responsibilities**:
- HTTP routing and handlers
- Request/response handling
- Middleware (logging, auth, CORS)
- Template rendering
- Static file serving

**Interface**: Exposes HTTP endpoints to external clients

#### 2. Aggregation Module
**Location**: `internal/aggregation/`

**Responsibilities**:
- Orchestrate data fetching from multiple connectors
- Merge and filter results
- Apply user preferences
- Handle concurrent fetching (fan-out/fan-in)

**Interface**:
```go
type Aggregator interface {
    GetAllProjects(ctx context.Context) ([]domain.Project, error)
    GetProjectDetails(ctx context.Context, projectID string) (*ProjectDetails, error)
    GetDashboardData(ctx context.Context) (*DashboardData, error)
}
```

#### 3. Connector Modules (Plugins)
**Location**: `internal/connectors/gitlab/`, `internal/connectors/github/`, etc.

**Responsibilities** (per connector):
- Platform-specific API communication
- Response parsing and transformation
- Rate limiting (if applicable)
- Caching integration

**Interface**:
```go
// Each connector implements this interface
type Connector interface {
    Name() string
    Platform() string
    GetProjects(ctx context.Context) ([]domain.Project, error)
    GetPipelines(ctx context.Context, projectID string, limit int) ([]domain.Pipeline, error)
    GetBranches(ctx context.Context, projectID string, limit int) ([]domain.Branch, error)
    GetBranch(ctx context.Context, projectID, branchName string) (*domain.Branch, error)
}
```

#### 4. Cache Module
**Location**: `internal/cache/`

**Responsibilities**:
- Stale-while-revalidate cache implementation
- In-memory or Redis backend
- Cache invalidation
- Background refresh coordination

**Interface**:
```go
type Cache interface {
    Get(key string) (interface{}, bool)
    Set(key string, value interface{}, ttl time.Duration)
    Invalidate(key string)
    Keys(pattern string) []string
}
```

#### 5. Configuration Module
**Location**: `internal/config/`

**Responsibilities**:
- Load configuration from environment variables
- Load configuration from YAML file
- Validate configuration
- Provide typed access to config values

---

## Plugin System for Connectors

### Plugin Registry

**Location**: `internal/connectors/registry.go`

```go
package connectors

import (
    "context"
    "fmt"
    "sync"
)

// Connector defines the interface all platform connectors must implement
type Connector interface {
    // Metadata
    Name() string                     // e.g., "gitlab-main"
    Platform() string                 // e.g., "gitlab", "github"

    // Core operations
    GetProjects(ctx context.Context) ([]domain.Project, error)
    GetProjectCount(ctx context.Context) (int, error)
    GetLatestPipeline(ctx context.Context, projectID, branch string) (*domain.Pipeline, error)
    GetPipelines(ctx context.Context, projectID string, limit int) ([]domain.Pipeline, error)
    GetBranches(ctx context.Context, projectID string, limit int) ([]domain.Branch, error)
    GetBranch(ctx context.Context, projectID, branchName string) (*domain.Branch, error)
}

// ExtendedConnector adds optional operations
type ExtendedConnector interface {
    Connector
    GetMergeRequests(ctx context.Context, projectID string) ([]domain.MergeRequest, error)
    GetIssues(ctx context.Context, projectID string) ([]domain.Issue, error)
}

// UserConnector adds user profile operations
type UserConnector interface {
    Connector
    GetCurrentUser(ctx context.Context) (*domain.UserProfile, error)
}

// Registry manages all registered connectors
type Registry struct {
    connectors map[string]Connector
    mu         sync.RWMutex
}

func NewRegistry() *Registry {
    return &Registry{
        connectors: make(map[string]Connector),
    }
}

// Register adds a connector to the registry
func (r *Registry) Register(connector Connector) error {
    r.mu.Lock()
    defer r.mu.Unlock()

    name := connector.Name()
    if _, exists := r.connectors[name]; exists {
        return fmt.Errorf("connector %s already registered", name)
    }

    r.connectors[name] = connector
    return nil
}

// Get retrieves a connector by name
func (r *Registry) Get(name string) (Connector, bool) {
    r.mu.RLock()
    defer r.mu.RUnlock()

    connector, exists := r.connectors[name]
    return connector, exists
}

// All returns all registered connectors
func (r *Registry) All() []Connector {
    r.mu.RLock()
    defer r.mu.RUnlock()

    connectors := make([]Connector, 0, len(r.connectors))
    for _, connector := range r.connectors {
        connectors = append(connectors, connector)
    }
    return connectors
}

// ByPlatform returns all connectors for a specific platform
func (r *Registry) ByPlatform(platform string) []Connector {
    r.mu.RLock()
    defer r.mu.RUnlock()

    var connectors []Connector
    for _, connector := range r.connectors {
        if connector.Platform() == platform {
            connectors = append(connectors, connector)
        }
    }
    return connectors
}
```

### Connector Factory Pattern

**Location**: `internal/connectors/factory.go`

```go
package connectors

import (
    "fmt"
    "net/http"

    "github.com/vilaca/ci-dashboard/internal/cache"
    "github.com/vilaca/ci-dashboard/internal/config"
    "github.com/vilaca/ci-dashboard/internal/connectors/github"
    "github.com/vilaca/ci-dashboard/internal/connectors/gitlab"
)

// Factory creates connectors from configuration
type Factory struct {
    cache      cache.Cache
    httpClient *http.Client
}

func NewFactory(cache cache.Cache, httpClient *http.Client) *Factory {
    return &Factory{
        cache:      cache,
        httpClient: httpClient,
    }
}

// CreateFromConfig creates all connectors defined in configuration
func (f *Factory) CreateFromConfig(cfg *config.Config) ([]Connector, error) {
    var connectors []Connector

    // Create GitLab connectors
    if cfg.HasGitLab() {
        for _, gitlabCfg := range cfg.GitLab {
            connector, err := f.CreateGitLabConnector(gitlabCfg)
            if err != nil {
                return nil, fmt.Errorf("failed to create GitLab connector: %w", err)
            }
            connectors = append(connectors, connector)
        }
    }

    // Create GitHub connectors
    if cfg.HasGitHub() {
        for _, githubCfg := range cfg.GitHub {
            connector, err := f.CreateGitHubConnector(githubCfg)
            if err != nil {
                return nil, fmt.Errorf("failed to create GitHub connector: %w", err)
            }
            connectors = append(connectors, connector)
        }
    }

    // Future: Jenkins, BitBucket, CircleCI, etc.

    return connectors, nil
}

// CreateGitLabConnector creates a GitLab connector
func (f *Factory) CreateGitLabConnector(cfg config.GitLabConfig) (Connector, error) {
    // Create base client
    client := gitlab.NewClient(gitlab.Config{
        Name:    cfg.Name,
        BaseURL: cfg.URL,
        Token:   cfg.Token,
    }, f.httpClient)

    // Wrap with cache
    cachedClient := gitlab.NewCachedClient(client, f.cache)

    return cachedClient, nil
}

// CreateGitHubConnector creates a GitHub connector
func (f *Factory) CreateGitHubConnector(cfg config.GitHubConfig) (Connector, error) {
    // Create base client
    client := github.NewClient(github.Config{
        Name:    cfg.Name,
        BaseURL: cfg.URL,
        Token:   cfg.Token,
    }, f.httpClient)

    // Wrap with cache
    cachedClient := github.NewCachedClient(client, f.cache)

    return cachedClient, nil
}
```

---

## In-Process Communication

### Direct Function Calls

No network overhead - just direct Go function calls:

```go
// HTTP Handler calls Aggregation Module
func (h *Handler) GetRepositories(w http.ResponseWriter, r *http.Request) {
    // Direct function call (nanoseconds)
    projects, err := h.aggregator.GetAllProjects(r.Context())
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    json.NewEncoder(w).Encode(projects)
}

// Aggregation Module calls Connector Modules
func (a *Aggregator) GetAllProjects(ctx context.Context) ([]domain.Project, error) {
    var allProjects []domain.Project
    var mu sync.Mutex
    var wg sync.WaitGroup

    connectors := a.registry.All()

    for _, connector := range connectors {
        wg.Add(1)
        go func(c Connector) {
            defer wg.Done()

            // Direct function call (nanoseconds)
            projects, err := c.GetProjects(ctx)
            if err != nil {
                log.Printf("Connector %s failed: %v", c.Name(), err)
                return
            }

            mu.Lock()
            allProjects = append(allProjects, projects...)
            mu.Unlock()
        }(connector)
    }

    wg.Wait()
    return allProjects, nil
}
```

**Performance Comparison**:
```
Microservices (HTTP):
  Handler → Aggregation Service: 1-5ms (network + serialization)
  Aggregation → Connector Service: 1-5ms per connector
  Total: 5-20ms just for inter-service communication

Modular Monolith (Function Call):
  Handler → Aggregation Module: ~100ns (direct function call)
  Aggregation → Connector Module: ~100ns per connector
  Total: <1µs for all inter-module communication
```

### Shared Memory

All modules share the same memory space:

```go
// Cache module stores data in memory
type InMemoryCache struct {
    data map[string]*cacheEntry
    mu   sync.RWMutex
}

// Connectors read/write to same cache instance
func (c *GitLabConnector) GetProjects(ctx context.Context) ([]domain.Project, error) {
    // Check cache (no network call, just memory read)
    if cached, found := c.cache.Get("gitlab:projects"); found {
        return cached.([]domain.Project), nil
    }

    // Fetch from API
    projects, err := c.fetchFromAPI(ctx)
    if err != nil {
        return nil, err
    }

    // Store in cache (no network call, just memory write)
    c.cache.Set("gitlab:projects", projects, 5*time.Minute)

    return projects, nil
}
```

---

## Project Structure

### Directory Layout

```
ci-dashboard/
├── cmd/
│   └── ci-dashboard/
│       └── main.go                    # Application entry point
│
├── internal/
│   ├── domain/                        # Domain models (shared)
│   │   ├── project.go
│   │   ├── pipeline.go
│   │   ├── branch.go
│   │   └── status.go
│   │
│   ├── config/                        # Configuration module
│   │   ├── config.go
│   │   ├── loader.go
│   │   └── validator.go
│   │
│   ├── cache/                         # Cache module
│   │   ├── cache.go                   # Cache interface
│   │   ├── memory.go                  # In-memory implementation
│   │   ├── redis.go                   # Redis implementation (optional)
│   │   └── stale.go                   # Stale-while-revalidate logic
│   │
│   ├── connectors/                    # Connector modules (plugins)
│   │   ├── connector.go               # Connector interface
│   │   ├── registry.go                # Plugin registry
│   │   ├── factory.go                 # Connector factory
│   │   │
│   │   ├── gitlab/                    # GitLab connector plugin
│   │   │   ├── client.go
│   │   │   ├── cached_client.go
│   │   │   ├── types.go
│   │   │   └── converter.go
│   │   │
│   │   ├── github/                    # GitHub connector plugin
│   │   │   ├── client.go
│   │   │   ├── cached_client.go
│   │   │   ├── rate_limiter.go
│   │   │   ├── types.go
│   │   │   └── converter.go
│   │   │
│   │   ├── jenkins/                   # Jenkins connector plugin (future)
│   │   │   └── ...
│   │   │
│   │   └── bitbucket/                 # BitBucket connector plugin (future)
│   │       └── ...
│   │
│   ├── aggregation/                   # Aggregation module
│   │   ├── aggregator.go              # Main orchestrator
│   │   ├── filters.go                 # Filtering logic
│   │   └── refresher.go               # Background refresh
│   │
│   ├── http/                          # HTTP server module
│   │   ├── server.go                  # HTTP server setup
│   │   ├── handlers/                  # Request handlers
│   │   │   ├── repositories.go
│   │   │   ├── repository_detail.go
│   │   │   └── health.go
│   │   ├── middleware/                # HTTP middleware
│   │   │   ├── logging.go
│   │   │   ├── cors.go
│   │   │   └── auth.go
│   │   └── renderer/                  # Template rendering
│   │       ├── renderer.go
│   │       └── templates.go
│   │
│   └── app/                           # Application wiring
│       ├── app.go                     # Application struct
│       └── builder.go                 # Dependency injection
│
├── web/
│   ├── templates/
│   │   └── *.html
│   └── static/
│       ├── css/
│       ├── js/
│       └── img/
│
├── config/
│   └── config.yaml                    # Default configuration file
│
├── scripts/
│   ├── build.sh
│   └── run.sh
│
├── Dockerfile                         # Single container
├── docker-compose.yml                 # Optional: with Redis
├── Makefile
├── go.mod
└── README.md
```

### Application Bootstrap

**cmd/ci-dashboard/main.go**:
```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"

    "github.com/vilaca/ci-dashboard/internal/app"
)

func main() {
    // Load configuration
    cfg, err := app.LoadConfig()
    if err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }

    // Build application with all dependencies
    application, err := app.Build(cfg)
    if err != nil {
        log.Fatalf("Failed to build application: %v", err)
    }

    // Start application
    if err := application.Start(); err != nil {
        log.Fatalf("Failed to start application: %v", err)
    }

    log.Printf("CI Dashboard running on http://localhost:%d", cfg.Port)

    // Wait for shutdown signal
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
    <-sigChan

    log.Println("Shutting down...")

    // Graceful shutdown
    ctx := context.Background()
    if err := application.Shutdown(ctx); err != nil {
        log.Printf("Shutdown error: %v", err)
    }
}
```

**internal/app/builder.go**:
```go
package app

import (
    "net/http"
    "time"

    "github.com/vilaca/ci-dashboard/internal/aggregation"
    "github.com/vilaca/ci-dashboard/internal/cache"
    "github.com/vilaca/ci-dashboard/internal/config"
    "github.com/vilaca/ci-dashboard/internal/connectors"
    httpserver "github.com/vilaca/ci-dashboard/internal/http"
)

// Application holds all modules
type Application struct {
    config     *config.Config
    cache      cache.Cache
    registry   *connectors.Registry
    aggregator *aggregation.Aggregator
    server     *httpserver.Server
}

// Build constructs the application with all dependencies
func Build(cfg *config.Config) (*Application, error) {
    // 1. Create cache
    var cacheImpl cache.Cache
    if cfg.Cache.Type == "redis" {
        cacheImpl = cache.NewRedis(cfg.Cache.RedisURL)
    } else {
        cacheImpl = cache.NewInMemory()
    }

    // 2. Create connector registry
    registry := connectors.NewRegistry()

    // 3. Create HTTP client (shared by all connectors)
    httpClient := &http.Client{
        Timeout: 30 * time.Second,
    }

    // 4. Create connector factory
    factory := connectors.NewFactory(cacheImpl, httpClient)

    // 5. Create and register connectors from config
    connectorList, err := factory.CreateFromConfig(cfg)
    if err != nil {
        return nil, err
    }

    for _, connector := range connectorList {
        if err := registry.Register(connector); err != nil {
            return nil, err
        }
    }

    // 6. Create aggregation module
    aggregator := aggregation.NewAggregator(registry, cfg)

    // 7. Start background refresh
    go aggregator.StartBackgroundRefresh()

    // 8. Create HTTP server module
    server := httpserver.NewServer(cfg, aggregator)

    return &Application{
        config:     cfg,
        cache:      cacheImpl,
        registry:   registry,
        aggregator: aggregator,
        server:     server,
    }, nil
}

// Start starts the application
func (a *Application) Start() error {
    return a.server.Start()
}

// Shutdown gracefully shuts down the application
func (a *Application) Shutdown(ctx context.Context) error {
    // Stop background refresh
    a.aggregator.Stop()

    // Shutdown HTTP server
    return a.server.Shutdown(ctx)
}
```

---

## Adding New Platform Connectors

### Step-by-Step Guide

**Example: Adding Jenkins Support**

#### Step 1: Create Connector Package

Create `internal/connectors/jenkins/`:

```
internal/connectors/jenkins/
├── client.go           # Jenkins API client
├── cached_client.go    # Caching wrapper
├── types.go            # Jenkins-specific types
└── converter.go        # Convert to domain types
```

#### Step 2: Implement Connector Interface

**internal/connectors/jenkins/client.go**:
```go
package jenkins

import (
    "context"
    "fmt"
    "net/http"

    "github.com/vilaca/ci-dashboard/internal/domain"
)

type Config struct {
    Name    string
    BaseURL string
    Token   string
}

type Client struct {
    name       string
    baseURL    string
    token      string
    httpClient *http.Client
}

func NewClient(cfg Config, httpClient *http.Client) *Client {
    return &Client{
        name:       cfg.Name,
        baseURL:    cfg.BaseURL,
        token:      cfg.Token,
        httpClient: httpClient,
    }
}

// Implement Connector interface
func (c *Client) Name() string {
    return c.name
}

func (c *Client) Platform() string {
    return "jenkins"
}

func (c *Client) GetProjects(ctx context.Context) ([]domain.Project, error) {
    // Implementation: Call Jenkins API, parse response, convert to domain.Project
    url := fmt.Sprintf("%s/api/json?tree=jobs[name,url,color]", c.baseURL)

    var response jenkinsJobsResponse
    if err := c.doRequest(ctx, url, &response); err != nil {
        return nil, err
    }

    return convertJenkinsJobs(response.Jobs), nil
}

func (c *Client) GetPipelines(ctx context.Context, projectID string, limit int) ([]domain.Pipeline, error) {
    // Implementation
}

// ... implement other required methods
```

#### Step 3: Create Cached Wrapper

**internal/connectors/jenkins/cached_client.go**:
```go
package jenkins

import (
    "context"
    "fmt"

    "github.com/vilaca/ci-dashboard/internal/cache"
    "github.com/vilaca/ci-dashboard/internal/domain"
)

type CachedClient struct {
    client *Client
    cache  cache.Cache
}

func NewCachedClient(client *Client, cache cache.Cache) *CachedClient {
    return &CachedClient{
        client: client,
        cache:  cache,
    }
}

func (c *CachedClient) Name() string {
    return c.client.Name()
}

func (c *CachedClient) Platform() string {
    return c.client.Platform()
}

func (c *CachedClient) GetProjects(ctx context.Context) ([]domain.Project, error) {
    key := fmt.Sprintf("jenkins:%s:projects", c.client.name)

    // Try cache first
    if cached, found := c.cache.Get(key); found {
        return cached.([]domain.Project), nil
    }

    // Cache miss - return empty (background refresh will populate)
    return []domain.Project{}, nil
}

// ForceRefresh is called by background refresher
func (c *CachedClient) ForceRefresh(ctx context.Context) error {
    // Fetch from Jenkins
    projects, err := c.client.GetProjects(ctx)
    if err != nil {
        return err
    }

    // Update cache
    key := fmt.Sprintf("jenkins:%s:projects", c.client.name)
    c.cache.Set(key, projects, 5*time.Minute)

    return nil
}
```

#### Step 4: Update Factory

**internal/connectors/factory.go**:
```go
import (
    "github.com/vilaca/ci-dashboard/internal/connectors/jenkins"
)

func (f *Factory) CreateFromConfig(cfg *config.Config) ([]Connector, error) {
    var connectors []Connector

    // ... existing GitLab and GitHub ...

    // Create Jenkins connectors
    if cfg.HasJenkins() {
        for _, jenkinsCfg := range cfg.Jenkins {
            connector, err := f.CreateJenkinsConnector(jenkinsCfg)
            if err != nil {
                return nil, fmt.Errorf("failed to create Jenkins connector: %w", err)
            }
            connectors = append(connectors, connector)
        }
    }

    return connectors, nil
}

func (f *Factory) CreateJenkinsConnector(cfg config.JenkinsConfig) (Connector, error) {
    client := jenkins.NewClient(jenkins.Config{
        Name:    cfg.Name,
        BaseURL: cfg.URL,
        Token:   cfg.Token,
    }, f.httpClient)

    cachedClient := jenkins.NewCachedClient(client, f.cache)

    return cachedClient, nil
}
```

#### Step 5: Update Configuration

**internal/config/config.go**:
```go
type Config struct {
    // ... existing ...

    Jenkins []JenkinsConfig `yaml:"jenkins"`
}

type JenkinsConfig struct {
    Name  string `yaml:"name"`   // e.g., "jenkins-prod"
    URL   string `yaml:"url"`    // e.g., "https://jenkins.example.com"
    Token string `yaml:"token"`  // Jenkins API token
}

func (c *Config) HasJenkins() bool {
    return len(c.Jenkins) > 0
}
```

#### Step 6: Add to Configuration File

**config/config.yaml**:
```yaml
gitlab:
  - name: gitlab-main
    url: https://gitlab.com
    token: ${GITLAB_TOKEN}

github:
  - name: github-main
    url: https://api.github.com
    token: ${GITHUB_TOKEN}

jenkins:
  - name: jenkins-prod
    url: https://jenkins.example.com
    token: ${JENKINS_TOKEN}
```

**That's it!** The Jenkins connector is now available and will be automatically:
- Registered in the connector registry
- Called by the aggregation module
- Cached by the cache module
- Refreshed by the background refresher
- Displayed in the dashboard

---

## Configuration

### Configuration File Format

**config/config.yaml**:
```yaml
# Server configuration
server:
  port: 8080
  host: 0.0.0.0

# Cache configuration
cache:
  type: memory          # "memory" or "redis"
  redis_url: redis://localhost:6379
  ttl: 5m              # Cache TTL
  stale_ttl: 24h       # How long to serve stale data
  refresh_interval: 5m  # Background refresh interval

# UI configuration
ui:
  refresh_interval: 5s  # Frontend polling interval
  title: "CI/CD Dashboard"

# GitLab instances
gitlab:
  - name: gitlab-main
    url: https://gitlab.com
    token: ${GITLAB_TOKEN}

  - name: gitlab-self-hosted
    url: https://gitlab.company.com
    token: ${GITLAB_SELF_HOSTED_TOKEN}

# GitHub instances
github:
  - name: github-main
    url: https://api.github.com
    token: ${GITHUB_TOKEN}

  - name: github-enterprise
    url: https://github.company.com/api/v3
    token: ${GITHUB_ENTERPRISE_TOKEN}

# Jenkins instances
jenkins:
  - name: jenkins-prod
    url: https://jenkins.example.com
    token: ${JENKINS_TOKEN}

# BitBucket instances
bitbucket:
  - name: bitbucket-main
    url: https://api.bitbucket.org/2.0
    token: ${BITBUCKET_TOKEN}

# Filtering (optional)
filters:
  watched_repos:
    - "123"           # GitLab project ID
    - "owner/repo"    # GitHub repo
    - "job-name"      # Jenkins job

# Logging
logging:
  level: info         # debug, info, warn, error
  format: json        # json or text
```

### Environment Variable Overrides

Environment variables override YAML config:

```bash
# Server
export PORT=3000

# Cache
export CACHE_TYPE=redis
export REDIS_URL=redis://localhost:6379

# Tokens (recommended: use env vars for secrets)
export GITLAB_TOKEN=glpat-xxxxxxxxxxxx
export GITHUB_TOKEN=ghp_xxxxxxxxxxxx
export JENKINS_TOKEN=xxxxxxxxxxxx
```

### Configuration Loading Priority

1. Default values (hardcoded)
2. YAML file (`config/config.yaml` or `--config` flag)
3. Environment variables (highest priority)

---

## Deployment

### Single Binary Deployment

**Build**:
```bash
# Build single binary
go build -o ci-dashboard ./cmd/ci-dashboard

# Cross-compile for different platforms
GOOS=linux GOARCH=amd64 go build -o ci-dashboard-linux ./cmd/ci-dashboard
GOOS=darwin GOARCH=arm64 go build -o ci-dashboard-mac ./cmd/ci-dashboard
GOOS=windows GOARCH=amd64 go build -o ci-dashboard.exe ./cmd/ci-dashboard
```

**Run**:
```bash
# With YAML config
./ci-dashboard --config config.yaml

# With environment variables only
export GITLAB_TOKEN=xxx
export GITHUB_TOKEN=yyy
./ci-dashboard

# Custom port
./ci-dashboard --port 3000
```

### Docker Deployment

**Dockerfile**:
```dockerfile
# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o ci-dashboard ./cmd/ci-dashboard

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/ci-dashboard .

# Copy web assets
COPY --from=builder /build/web ./web

# Copy default config
COPY --from=builder /build/config ./config

# Expose port
EXPOSE 8080

# Run
CMD ["./ci-dashboard"]
```

**Build and Run**:
```bash
# Build image
docker build -t ci-dashboard:latest .

# Run container (memory cache)
docker run -d \
  -p 8080:8080 \
  -e GITLAB_TOKEN=xxx \
  -e GITHUB_TOKEN=yyy \
  --name ci-dashboard \
  ci-dashboard:latest

# Run with volume-mounted config
docker run -d \
  -p 8080:8080 \
  -v $(pwd)/config.yaml:/app/config/config.yaml:ro \
  --name ci-dashboard \
  ci-dashboard:latest
```

### Docker Compose (with Redis)

**docker-compose.yml**:
```yaml
version: '3.8'

services:
  app:
    build: .
    ports:
      - "8080:8080"
    environment:
      - CACHE_TYPE=redis
      - REDIS_URL=redis://redis:6379
      - GITLAB_TOKEN=${GITLAB_TOKEN}
      - GITHUB_TOKEN=${GITHUB_TOKEN}
    volumes:
      - ./config.yaml:/app/config/config.yaml:ro
    depends_on:
      - redis
    restart: unless-stopped

  redis:
    image: redis:7-alpine
    volumes:
      - redis-data:/data
    restart: unless-stopped

volumes:
  redis-data:
```

**Run**:
```bash
docker-compose up -d
```

### Systemd Service (Linux)

**/etc/systemd/system/ci-dashboard.service**:
```ini
[Unit]
Description=CI/CD Dashboard
After=network.target

[Service]
Type=simple
User=ci-dashboard
WorkingDirectory=/opt/ci-dashboard
ExecStart=/opt/ci-dashboard/ci-dashboard --config /opt/ci-dashboard/config.yaml
Restart=on-failure
RestartSec=10

# Environment
Environment="GITLAB_TOKEN=xxx"
Environment="GITHUB_TOKEN=yyy"

# Security
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

**Commands**:
```bash
sudo systemctl enable ci-dashboard
sudo systemctl start ci-dashboard
sudo systemctl status ci-dashboard
```

### Kubernetes Deployment (Single Pod)

**deployment.yaml**:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ci-dashboard
spec:
  replicas: 1  # Single replica (no need for more with in-memory cache)
  selector:
    matchLabels:
      app: ci-dashboard
  template:
    metadata:
      labels:
        app: ci-dashboard
    spec:
      containers:
      - name: ci-dashboard
        image: registry.example.com/ci-dashboard:latest
        ports:
        - containerPort: 8080
        env:
        - name: CACHE_TYPE
          value: "memory"
        - name: GITLAB_TOKEN
          valueFrom:
            secretKeyRef:
              name: ci-dashboard-secrets
              key: gitlab-token
        - name: GITHUB_TOKEN
          valueFrom:
            secretKeyRef:
              name: ci-dashboard-secrets
              key: github-token
        resources:
          requests:
            memory: "256Mi"
            cpu: "200m"
          limits:
            memory: "1Gi"
            cpu: "1000m"
        livenessProbe:
          httpGet:
            path: /api/health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /api/health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
---
apiVersion: v1
kind: Service
metadata:
  name: ci-dashboard
spec:
  selector:
    app: ci-dashboard
  ports:
  - port: 80
    targetPort: 8080
  type: LoadBalancer
```

---

## Comparison with Current Implementation

### Current Implementation

**Pros**:
- ✅ Already working
- ✅ Clean architecture with interfaces
- ✅ Good separation of concerns

**Cons**:
- ❌ Less structured plugin system
- ❌ Harder to add new platforms (need to modify multiple files)
- ❌ Platform clients tightly coupled to service layer
- ❌ No clear module boundaries

### Proposed Modular Monolith

**Changes**:

1. **Plugin Registry**: Centralized connector management
   - Current: Clients registered in main.go
   - Proposed: Registry pattern with automatic discovery

2. **Connector Factory**: Standardized connector creation
   - Current: Manual creation in main.go
   - Proposed: Factory creates connectors from config

3. **Module Boundaries**: Clear separation
   - Current: Packages (internal/api, internal/service)
   - Proposed: Modules (internal/connectors, internal/aggregation, internal/http)

4. **Configuration**: More flexible
   - Current: Environment variables only
   - Proposed: YAML + environment variables, multiple instances per platform

5. **Adding Platforms**: Simpler
   - Current: Create client, modify main.go, modify service, modify interfaces
   - Proposed: Create connector package, update factory, done

### Migration Path

**Phase 1**: Refactor existing code into modules
- Move GitLab/GitHub clients to `internal/connectors/`
- Create registry
- Create factory
- Update main.go to use factory

**Phase 2**: Add plugin system
- Implement connector interface
- Wrap existing clients with interface
- Test everything still works

**Phase 3**: Add new platforms
- Use new plugin system to add Jenkins, etc.

---

## Resource Usage

### Memory Usage

**Single Instance** (typical):
```
Base application:     50 MB
In-memory cache:      100 MB (for ~1000 projects)
HTTP server:          20 MB
Goroutines:           10 MB
Total:                ~180 MB
```

**With Redis** (cache offloaded):
```
Base application:     50 MB
HTTP server:          20 MB
Goroutines:           10 MB
Total:                ~80 MB
```

### CPU Usage

**Idle**: <5% of 1 core

**Active** (background refresh):
- Fetching data: 20-40% of 1 core
- Duration: 10-30 seconds every 5 minutes

**Under Load** (100 req/s):
- HTTP handling: 40-60% of 1 core
- Cache reads: negligible

### Comparison with Microservices

**Modular Monolith**:
- 1 process
- 1 container
- 180 MB memory
- 0.2 CPU cores
- ~$10-20/month (small VPS)

**Microservices** (6 services):
- 6 processes
- 6 containers
- 6 × 256 MB = 1.5 GB memory
- 6 × 0.2 = 1.2 CPU cores
- ~$100-200/month (Kubernetes cluster)

---

## Two-Process Architecture (Recommended for Production)

For better separation of concerns and resource management, split into 2 processes on the same machine:

### Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    Single Computer                       │
│                                                          │
│  ┌───────────────────────────────────────────────────┐  │
│  │         Process 1: Web Server                     │  │
│  │         (./ci-dashboard-web)                      │  │
│  │                                                   │  │
│  │  - Serves HTTP requests                          │  │
│  │  - Renders templates                             │  │
│  │  - Handles user sessions                         │  │
│  │  - Queries aggregation service                   │  │
│  │                                                   │  │
│  │  Port: 8080                                       │  │
│  └─────────────────────┬─────────────────────────────┘  │
│                        │                                 │
│                        │ HTTP/IPC                        │
│                        │ (localhost only)                │
│                        │                                 │
│  ┌─────────────────────▼─────────────────────────────┐  │
│  │         Process 2: Aggregation Service            │  │
│  │         (./ci-dashboard-aggregator)               │  │
│  │                                                   │  │
│  │  - Fetches data from GitLab/GitHub/etc.          │  │
│  │  - Manages cache (in-memory or Redis)            │  │
│  │  - Background refresh (every 5 minutes)          │  │
│  │  - Provides data API (localhost only)            │  │
│  │                                                   │  │
│  │  Port: 8081 (localhost only)                      │  │
│  └───────────────────────────────────────────────────┘  │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

### Benefits

✅ **Isolation**:
- Web server can restart without losing cached data
- Heavy data fetching doesn't block web requests
- Each process can be monitored separately

✅ **Resource Management**:
- Web server: Low memory, handles HTTP connections
- Aggregator: High memory (cache), CPU-intensive background jobs

✅ **Still Simple**:
- Both processes run on same machine
- Communicate via localhost (minimal latency)
- Easy to debug (2 processes, not 6+)
- Can use Unix sockets for even faster IPC

✅ **Deployment**:
- 2 binaries or 1 binary with different commands
- Can restart independently
- Still much simpler than microservices

### Process Communication

**Option 1: Local HTTP** (Simplest)
```go
// Web server calls aggregator via localhost
resp, err := http.Get("http://localhost:8081/api/projects")
```

**Option 2: Unix Domain Socket** (Faster, ~30% less latency than TCP)
```go
// Aggregator listens on Unix socket
listener, _ := net.Listen("unix", "/tmp/ci-dashboard-aggregator.sock")

// Web server connects via Unix socket
conn, _ := net.Dial("unix", "/tmp/ci-dashboard-aggregator.sock")
```

**Option 3: gRPC** (If you need streaming or bi-directional communication)
```go
// Define service
service ProjectService {
  rpc GetProjects(GetProjectsRequest) returns (GetProjectsResponse);
  rpc StreamUpdates(StreamRequest) returns (stream Update);
}
```

### Implementation

**cmd/ci-dashboard-web/main.go**:
```go
package main

import (
    "log"
    "net/http"

    "github.com/vilaca/ci-dashboard/internal/aggregation/client"
    httpserver "github.com/vilaca/ci-dashboard/internal/http"
)

func main() {
    // Create client to aggregation service
    aggregatorClient := client.NewHTTPClient("http://localhost:8081")

    // Create HTTP server
    server := httpserver.NewServer(aggregatorClient)

    log.Println("Web server starting on :8080")
    if err := server.Start(":8080"); err != nil {
        log.Fatal(err)
    }
}
```

**cmd/ci-dashboard-aggregator/main.go**:
```go
package main

import (
    "log"
    "net/http"

    "github.com/vilaca/ci-dashboard/internal/aggregation"
    "github.com/vilaca/ci-dashboard/internal/cache"
    "github.com/vilaca/ci-dashboard/internal/connectors"
)

func main() {
    // Create cache
    cache := cache.NewInMemory()

    // Create and register connectors
    registry := connectors.NewRegistry()
    // ... register GitLab, GitHub, etc.

    // Create aggregation service
    aggregator := aggregation.NewAggregator(registry, cache)

    // Start background refresh
    go aggregator.StartBackgroundRefresh()

    // Start HTTP server (localhost only)
    mux := http.NewServeMux()
    mux.HandleFunc("/api/projects", aggregator.HandleGetProjects)
    mux.HandleFunc("/api/projects/", aggregator.HandleGetProjectDetails)
    mux.HandleFunc("/api/branches/", aggregator.HandleGetBranches)

    log.Println("Aggregation service starting on :8081 (localhost only)")
    if err := http.ListenAndServe("127.0.0.1:8081", mux); err != nil {
        log.Fatal(err)
    }
}
```

### Makefile

```makefile
.PHONY: build-web build-aggregator build-all run-all

# Build web server
build-web:
	go build -o bin/ci-dashboard-web ./cmd/ci-dashboard-web

# Build aggregator
build-aggregator:
	go build -o bin/ci-dashboard-aggregator ./cmd/ci-dashboard-aggregator

# Build both
build-all: build-web build-aggregator

# Run both processes
run-all: build-all
	./bin/ci-dashboard-aggregator & \
	sleep 2 && \
	./bin/ci-dashboard-web

# Stop all
stop-all:
	pkill -f ci-dashboard-web || true
	pkill -f ci-dashboard-aggregator || true
```

### Systemd Services (Linux)

**/etc/systemd/system/ci-dashboard-aggregator.service**:
```ini
[Unit]
Description=CI Dashboard Aggregation Service
After=network.target

[Service]
Type=simple
User=ci-dashboard
WorkingDirectory=/opt/ci-dashboard
ExecStart=/opt/ci-dashboard/ci-dashboard-aggregator
Restart=on-failure
RestartSec=10

Environment="GITLAB_TOKEN=xxx"
Environment="GITHUB_TOKEN=yyy"

[Install]
WantedBy=multi-user.target
```

**/etc/systemd/system/ci-dashboard-web.service**:
```ini
[Unit]
Description=CI Dashboard Web Server
After=network.target ci-dashboard-aggregator.service
Requires=ci-dashboard-aggregator.service

[Service]
Type=simple
User=ci-dashboard
WorkingDirectory=/opt/ci-dashboard
ExecStart=/opt/ci-dashboard/ci-dashboard-web
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
```

**Commands**:
```bash
sudo systemctl enable ci-dashboard-aggregator ci-dashboard-web
sudo systemctl start ci-dashboard-aggregator
sudo systemctl start ci-dashboard-web
sudo systemctl status ci-dashboard-aggregator
sudo systemctl status ci-dashboard-web
```

### Docker Compose

**docker-compose.yml**:
```yaml
version: '3.8'

services:
  aggregator:
    build:
      context: .
      dockerfile: Dockerfile.aggregator
    environment:
      - GITLAB_TOKEN=${GITLAB_TOKEN}
      - GITHUB_TOKEN=${GITHUB_TOKEN}
      - CACHE_TYPE=memory
    volumes:
      - ./config.yaml:/app/config.yaml:ro
    networks:
      - internal
    restart: unless-stopped

  web:
    build:
      context: .
      dockerfile: Dockerfile.web
    ports:
      - "8080:8080"
    environment:
      - AGGREGATOR_URL=http://aggregator:8081
    depends_on:
      - aggregator
    networks:
      - internal
    restart: unless-stopped

networks:
  internal:
```

### Resource Usage

**Web Server Process**:
- Memory: ~50 MB
- CPU: ~10% (when serving requests)
- Handles: HTTP connections, template rendering

**Aggregator Process**:
- Memory: ~200 MB (includes cache)
- CPU: ~30% during background refresh
- Handles: API fetching, caching, background jobs

**Total**: ~250 MB, ~40% CPU average

**Comparison**:
- 1 Process: 180 MB, easier but less isolated
- 2 Processes: 250 MB, better isolation and resource management
- 6+ Microservices: 1.5 GB, complex but highly scalable

### Graceful Restart

**Zero-downtime restart of web server**:
```bash
# Aggregator keeps running, cache stays warm
systemctl restart ci-dashboard-web

# Web server restarts in ~1 second
# No cache loss, no data fetching delay
```

**Restart aggregator** (will lose in-memory cache):
```bash
systemctl restart ci-dashboard-aggregator

# Web server continues serving (may get errors until aggregator is back)
# Cache repopulates within ~30 seconds
```

---

## Summary

### Three Options

**1. Single Process (Development)**
- Simplest to run and debug
- Perfect for local development
- Just run `./ci-dashboard`
- 180 MB memory

**2. Two Processes (Production - Recommended)**
- Web server + Aggregation service
- Better isolation and resource management
- Web can restart without losing cache
- Still runs on single computer
- 250 MB memory
- Recommended for production on a single server

**3. Microservices (Large Scale)**
- 6+ separate services
- Full isolation and independent scaling
- Requires orchestration (Kubernetes)
- 1.5+ GB memory
- Only needed at large scale

### Recommendation by Use Case

**Development/Testing**:
→ Use **1 Process** (simplest)

**Production (Single Server/VPS/Laptop)**:
→ Use **2 Processes** (best balance)

**Production (Large Scale/Multiple Servers)**:
→ Use **Microservices** (complex but necessary)

### Why 2 Processes is the Sweet Spot

✅ **Separation of concerns**:
- Web serving isolated from data fetching
- Each process has a single responsibility

✅ **Better resource management**:
- Web process: Optimized for HTTP handling
- Aggregator process: Optimized for batch operations

✅ **Independent restarts**:
- Update web UI without losing cache
- Restart aggregator without affecting web requests (brief errors only)

✅ **Still simple**:
- Same machine, localhost communication
- Easy debugging (just 2 processes)
- Simple deployment (2 binaries)
- No orchestration needed

✅ **Low latency**:
- Localhost HTTP: ~0.1ms
- Unix sockets: ~0.07ms
- Compare to microservices: 1-5ms per hop

The 2-process architecture provides **80% of microservices benefits with 20% of the complexity**.
