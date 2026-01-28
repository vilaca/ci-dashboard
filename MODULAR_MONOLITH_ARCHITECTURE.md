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

#### 5. Storage Module (PostgreSQL)
**Location**: `internal/storage/`

**Responsibilities**:
- Persistent data storage (historical data)
- Pipeline run history and metrics
- User preferences and watched repositories
- Webhook event logs
- Trend analysis and statistics

**Interface**:
```go
type Storage interface {
    // Projects (metadata only, not cached data)
    SaveProject(ctx context.Context, project *domain.Project) error
    GetProject(ctx context.Context, id string) (*domain.Project, error)
    ListProjects(ctx context.Context) ([]domain.Project, error)

    // Pipelines (historical records)
    SavePipeline(ctx context.Context, pipeline *domain.Pipeline) error
    GetPipelineHistory(ctx context.Context, projectID string, limit int) ([]domain.Pipeline, error)
    GetPipelineStats(ctx context.Context, projectID string, days int) (*PipelineStats, error)

    // Branches (for trend tracking)
    SaveBranchSnapshot(ctx context.Context, branch *domain.Branch) error

    // User preferences
    SaveUserPreferences(ctx context.Context, userID string, prefs *UserPreferences) error
    GetUserPreferences(ctx context.Context, userID string) (*UserPreferences, error)

    // Webhook events
    SaveWebhookEvent(ctx context.Context, event *WebhookEvent) error
    GetRecentEvents(ctx context.Context, limit int) ([]WebhookEvent, error)
}
```

**Database Schema** (see [PostgreSQL Integration](#postgresql-integration) section below)

#### 6. Configuration Module
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

## PostgreSQL Integration

### Overview

PostgreSQL provides persistent storage for historical data, user preferences, and analytics. It complements the cache layer (which handles real-time data).

### Data Storage Strategy

**Cache (Redis/In-Memory)**:
- ✅ Current state (latest pipelines, branches)
- ✅ Frequent reads (every API request)
- ✅ TTL-based expiration (5 minutes)
- ✅ Stale-while-revalidate pattern
- ⚡ Ultra-fast reads (<1ms)

**PostgreSQL**:
- ✅ Historical records (all pipeline runs ever)
- ✅ User preferences and settings
- ✅ Webhook event logs
- ✅ Trend analysis and statistics
- ✅ Infrequent reads (reports, dashboards)
- 📊 Complex queries and aggregations

**Pattern**: **Cache-Aside with Write-Through**
```
Read Request → Check Cache → Return if found
                           → Query PostgreSQL if miss
                           → Store in cache
                           → Return

Background Refresh → Fetch from APIs
                   → Write to Cache (fast)
                   → Write to PostgreSQL (async, don't block)
```

### Database Schema

**migrations/001_initial_schema.sql**:
```sql
-- Projects table (metadata only)
CREATE TABLE projects (
    id VARCHAR(255) PRIMARY KEY,
    platform VARCHAR(50) NOT NULL,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    web_url TEXT,
    default_branch VARCHAR(255),
    visibility VARCHAR(50),
    avatar_url TEXT,
    last_activity_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(platform, id)
);

CREATE INDEX idx_projects_platform ON projects(platform);
CREATE INDEX idx_projects_last_activity ON projects(last_activity_at DESC);

-- Pipelines table (historical records)
CREATE TABLE pipelines (
    id VARCHAR(255) NOT NULL,
    project_id VARCHAR(255) NOT NULL REFERENCES projects(id),
    platform VARCHAR(50) NOT NULL,
    status VARCHAR(50) NOT NULL,
    ref VARCHAR(255),
    branch VARCHAR(255),
    sha VARCHAR(255),
    web_url TEXT,
    author VARCHAR(255),
    message TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    duration INTEGER,
    PRIMARY KEY (platform, project_id, id)
);

CREATE INDEX idx_pipelines_project ON pipelines(project_id, created_at DESC);
CREATE INDEX idx_pipelines_status ON pipelines(status);
CREATE INDEX idx_pipelines_branch ON pipelines(project_id, branch, created_at DESC);
CREATE INDEX idx_pipelines_created_at ON pipelines(created_at DESC);

-- Branch snapshots (for trend tracking)
CREATE TABLE branch_snapshots (
    id BIGSERIAL PRIMARY KEY,
    project_id VARCHAR(255) NOT NULL REFERENCES projects(id),
    branch_name VARCHAR(255) NOT NULL,
    sha VARCHAR(255),
    commit_author VARCHAR(255),
    commit_message TEXT,
    commit_date TIMESTAMPTZ,
    is_default BOOLEAN DEFAULT FALSE,
    snapshot_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_branch_snapshots_project ON branch_snapshots(project_id, branch_name, snapshot_at DESC);

-- User preferences
CREATE TABLE users (
    id VARCHAR(255) PRIMARY KEY,
    username VARCHAR(255) NOT NULL,
    email VARCHAR(255),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE user_preferences (
    user_id VARCHAR(255) PRIMARY KEY REFERENCES users(id),
    watched_projects TEXT[], -- Array of project IDs
    dashboard_layout JSONB,
    notification_settings JSONB,
    theme VARCHAR(50) DEFAULT 'light',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Webhook events (audit log)
CREATE TABLE webhook_events (
    id BIGSERIAL PRIMARY KEY,
    platform VARCHAR(50) NOT NULL,
    project_id VARCHAR(255),
    event_type VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    processed BOOLEAN DEFAULT FALSE,
    received_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_webhook_events_platform ON webhook_events(platform, received_at DESC);
CREATE INDEX idx_webhook_events_project ON webhook_events(project_id, received_at DESC);
CREATE INDEX idx_webhook_events_type ON webhook_events(event_type);

-- Pipeline statistics (materialized view for fast queries)
CREATE MATERIALIZED VIEW pipeline_stats AS
SELECT
    project_id,
    branch,
    COUNT(*) as total_runs,
    COUNT(*) FILTER (WHERE status = 'success') as successful_runs,
    COUNT(*) FILTER (WHERE status = 'failed') as failed_runs,
    AVG(duration) FILTER (WHERE duration > 0) as avg_duration,
    MAX(created_at) as last_run_at
FROM pipelines
WHERE created_at > NOW() - INTERVAL '30 days'
GROUP BY project_id, branch;

CREATE UNIQUE INDEX idx_pipeline_stats_project_branch ON pipeline_stats(project_id, branch);

-- Refresh materialized view periodically (e.g., via cron or application)
-- REFRESH MATERIALIZED VIEW CONCURRENTLY pipeline_stats;
```

### Storage Implementation

**internal/storage/postgres.go**:
```go
package storage

import (
    "context"
    "database/sql"
    "encoding/json"
    "time"

    "github.com/lib/pq"
    _ "github.com/lib/pq"

    "github.com/vilaca/ci-dashboard/internal/domain"
)

type PostgresStorage struct {
    db *sql.DB
}

func NewPostgresStorage(connString string) (*PostgresStorage, error) {
    db, err := sql.Open("postgres", connString)
    if err != nil {
        return nil, err
    }

    // Test connection
    if err := db.Ping(); err != nil {
        return nil, err
    }

    // Set connection pool settings
    db.SetMaxOpenConns(25)
    db.SetMaxIdleConns(5)
    db.SetConnMaxLifetime(5 * time.Minute)

    return &PostgresStorage{db: db}, nil
}

func (s *PostgresStorage) Close() error {
    return s.db.Close()
}

// SaveProject inserts or updates project metadata
func (s *PostgresStorage) SaveProject(ctx context.Context, project *domain.Project) error {
    query := `
        INSERT INTO projects (
            id, platform, name, description, web_url, default_branch,
            visibility, avatar_url, last_activity_at, updated_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
        ON CONFLICT (platform, id) DO UPDATE SET
            name = EXCLUDED.name,
            description = EXCLUDED.description,
            web_url = EXCLUDED.web_url,
            default_branch = EXCLUDED.default_branch,
            visibility = EXCLUDED.visibility,
            avatar_url = EXCLUDED.avatar_url,
            last_activity_at = EXCLUDED.last_activity_at,
            updated_at = NOW()
    `

    _, err := s.db.ExecContext(ctx, query,
        project.ID,
        project.Platform,
        project.Name,
        project.Description,
        project.WebURL,
        project.DefaultBranch,
        project.Visibility,
        project.AvatarURL,
        project.LastActivity,
    )

    return err
}

// SavePipeline saves pipeline run to history
func (s *PostgresStorage) SavePipeline(ctx context.Context, pipeline *domain.Pipeline) error {
    query := `
        INSERT INTO pipelines (
            id, project_id, platform, status, ref, branch, sha, web_url,
            author, message, created_at, updated_at, started_at, finished_at, duration
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
        ON CONFLICT (platform, project_id, id) DO UPDATE SET
            status = EXCLUDED.status,
            updated_at = EXCLUDED.updated_at,
            finished_at = EXCLUDED.finished_at,
            duration = EXCLUDED.duration
    `

    _, err := s.db.ExecContext(ctx, query,
        pipeline.ID,
        pipeline.ProjectID,
        determinePlatform(pipeline),
        pipeline.Status,
        pipeline.Ref,
        pipeline.Branch,
        pipeline.SHA,
        pipeline.WebURL,
        pipeline.Author,
        pipeline.Message,
        pipeline.CreatedAt,
        pipeline.UpdatedAt,
        pipeline.StartedAt,
        pipeline.FinishedAt,
        pipeline.Duration,
    )

    return err
}

// GetPipelineHistory retrieves historical pipeline runs
func (s *PostgresStorage) GetPipelineHistory(ctx context.Context, projectID string, limit int) ([]domain.Pipeline, error) {
    query := `
        SELECT id, project_id, platform, status, ref, branch, sha, web_url,
               author, message, created_at, updated_at, started_at, finished_at, duration
        FROM pipelines
        WHERE project_id = $1
        ORDER BY created_at DESC
        LIMIT $2
    `

    rows, err := s.db.QueryContext(ctx, query, projectID, limit)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var pipelines []domain.Pipeline
    for rows.Next() {
        var p domain.Pipeline
        var platform string

        err := rows.Scan(
            &p.ID,
            &p.ProjectID,
            &platform,
            &p.Status,
            &p.Ref,
            &p.Branch,
            &p.SHA,
            &p.WebURL,
            &p.Author,
            &p.Message,
            &p.CreatedAt,
            &p.UpdatedAt,
            &p.StartedAt,
            &p.FinishedAt,
            &p.Duration,
        )
        if err != nil {
            return nil, err
        }

        pipelines = append(pipelines, p)
    }

    return pipelines, rows.Err()
}

// GetPipelineStats retrieves statistics for a project
func (s *PostgresStorage) GetPipelineStats(ctx context.Context, projectID string, days int) (*PipelineStats, error) {
    query := `
        SELECT
            COUNT(*) as total_runs,
            COUNT(*) FILTER (WHERE status = 'success') as successful_runs,
            COUNT(*) FILTER (WHERE status = 'failed') as failed_runs,
            COUNT(*) FILTER (WHERE status = 'canceled') as canceled_runs,
            AVG(duration) FILTER (WHERE duration > 0) as avg_duration,
            MAX(duration) as max_duration,
            MIN(duration) FILTER (WHERE duration > 0) as min_duration
        FROM pipelines
        WHERE project_id = $1
          AND created_at > NOW() - INTERVAL '1 day' * $2
    `

    var stats PipelineStats
    var avgDuration, maxDuration, minDuration sql.NullFloat64

    err := s.db.QueryRowContext(ctx, query, projectID, days).Scan(
        &stats.TotalRuns,
        &stats.SuccessfulRuns,
        &stats.FailedRuns,
        &stats.CanceledRuns,
        &avgDuration,
        &maxDuration,
        &minDuration,
    )

    if err != nil {
        return nil, err
    }

    stats.AvgDuration = int(avgDuration.Float64)
    stats.MaxDuration = int(maxDuration.Float64)
    stats.MinDuration = int(minDuration.Float64)
    stats.SuccessRate = float64(stats.SuccessfulRuns) / float64(stats.TotalRuns) * 100

    return &stats, nil
}

// SaveUserPreferences saves user preferences
func (s *PostgresStorage) SaveUserPreferences(ctx context.Context, userID string, prefs *UserPreferences) error {
    // First ensure user exists
    _, err := s.db.ExecContext(ctx,
        `INSERT INTO users (id, username) VALUES ($1, $2) ON CONFLICT (id) DO NOTHING`,
        userID, userID)
    if err != nil {
        return err
    }

    // Marshal JSONB fields
    dashboardLayout, _ := json.Marshal(prefs.DashboardLayout)
    notificationSettings, _ := json.Marshal(prefs.NotificationSettings)

    query := `
        INSERT INTO user_preferences (
            user_id, watched_projects, dashboard_layout, notification_settings, theme, updated_at
        ) VALUES ($1, $2, $3, $4, $5, NOW())
        ON CONFLICT (user_id) DO UPDATE SET
            watched_projects = EXCLUDED.watched_projects,
            dashboard_layout = EXCLUDED.dashboard_layout,
            notification_settings = EXCLUDED.notification_settings,
            theme = EXCLUDED.theme,
            updated_at = NOW()
    `

    _, err = s.db.ExecContext(ctx, query,
        userID,
        pq.Array(prefs.WatchedProjects),
        dashboardLayout,
        notificationSettings,
        prefs.Theme,
    )

    return err
}

// GetUserPreferences retrieves user preferences
func (s *PostgresStorage) GetUserPreferences(ctx context.Context, userID string) (*UserPreferences, error) {
    query := `
        SELECT watched_projects, dashboard_layout, notification_settings, theme
        FROM user_preferences
        WHERE user_id = $1
    `

    var prefs UserPreferences
    var watchedProjects pq.StringArray
    var dashboardLayout, notificationSettings []byte

    err := s.db.QueryRowContext(ctx, query, userID).Scan(
        &watchedProjects,
        &dashboardLayout,
        &notificationSettings,
        &prefs.Theme,
    )

    if err == sql.ErrNoRows {
        // Return default preferences
        return &UserPreferences{
            WatchedProjects: []string{},
            Theme:           "light",
        }, nil
    }

    if err != nil {
        return nil, err
    }

    prefs.WatchedProjects = watchedProjects
    json.Unmarshal(dashboardLayout, &prefs.DashboardLayout)
    json.Unmarshal(notificationSettings, &prefs.NotificationSettings)

    return &prefs, nil
}

// SaveWebhookEvent logs webhook event
func (s *PostgresStorage) SaveWebhookEvent(ctx context.Context, event *WebhookEvent) error {
    payload, _ := json.Marshal(event.Payload)

    query := `
        INSERT INTO webhook_events (platform, project_id, event_type, payload)
        VALUES ($1, $2, $3, $4)
    `

    _, err := s.db.ExecContext(ctx, query,
        event.Platform,
        event.ProjectID,
        event.EventType,
        payload,
    )

    return err
}

// GetRecentEvents retrieves recent webhook events
func (s *PostgresStorage) GetRecentEvents(ctx context.Context, limit int) ([]WebhookEvent, error) {
    query := `
        SELECT id, platform, project_id, event_type, payload, received_at
        FROM webhook_events
        ORDER BY received_at DESC
        LIMIT $1
    `

    rows, err := s.db.QueryContext(ctx, query, limit)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var events []WebhookEvent
    for rows.Next() {
        var e WebhookEvent
        var payload []byte

        err := rows.Scan(&e.ID, &e.Platform, &e.ProjectID, &e.EventType, &payload, &e.ReceivedAt)
        if err != nil {
            return nil, err
        }

        json.Unmarshal(payload, &e.Payload)
        events = append(events, e)
    }

    return events, rows.Err()
}

// Helper types
type PipelineStats struct {
    TotalRuns      int
    SuccessfulRuns int
    FailedRuns     int
    CanceledRuns   int
    AvgDuration    int
    MaxDuration    int
    MinDuration    int
    SuccessRate    float64
}

type UserPreferences struct {
    WatchedProjects      []string
    DashboardLayout      map[string]interface{}
    NotificationSettings map[string]interface{}
    Theme                string
}

type WebhookEvent struct {
    ID         int64
    Platform   string
    ProjectID  string
    EventType  string
    Payload    map[string]interface{}
    ReceivedAt time.Time
}

func determinePlatform(p *domain.Pipeline) string {
    // Determine platform from project ID format
    if strings.Contains(p.ProjectID, "/") {
        return "github"
    }
    return "gitlab"
}
```

### Integrating with Aggregation Service

**internal/app/builder.go** (updated):
```go
func Build(cfg *config.Config) (*Application, error) {
    // 1. Create PostgreSQL storage (if configured)
    var storage storage.Storage
    if cfg.Database.PostgresURL != "" {
        pgStorage, err := storage.NewPostgresStorage(cfg.Database.PostgresURL)
        if err != nil {
            return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
        }
        storage = pgStorage
    }

    // 2. Create cache
    var cacheImpl cache.Cache
    if cfg.Cache.Type == "redis" {
        cacheImpl = cache.NewRedis(cfg.Cache.RedisURL)
    } else {
        cacheImpl = cache.NewInMemory()
    }

    // 3-6. Create connectors...

    // 7. Create aggregation module (with storage)
    aggregator := aggregation.NewAggregator(registry, cfg, storage)

    // 8. Start background refresh
    go aggregator.StartBackgroundRefresh()

    // ...
}
```

**internal/aggregation/aggregator.go** (updated):
```go
type Aggregator struct {
    registry *connectors.Registry
    config   *config.Config
    storage  storage.Storage // Can be nil
}

// After fetching pipelines from APIs, save to PostgreSQL
func (a *Aggregator) RefreshPipelines(ctx context.Context) error {
    pipelines, err := a.fetchPipelinesFromConnectors(ctx)
    if err != nil {
        return err
    }

    // Save to PostgreSQL asynchronously (don't block)
    if a.storage != nil {
        go func() {
            ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
            defer cancel()

            for _, pipeline := range pipelines {
                if err := a.storage.SavePipeline(ctx, &pipeline); err != nil {
                    log.Printf("Failed to save pipeline to PostgreSQL: %v", err)
                }
            }
        }()
    }

    return nil
}
```

### Configuration

**config/config.yaml** (updated):
```yaml
# Database configuration
database:
  postgres_url: postgres://user:pass@localhost:5432/ci_dashboard?sslmode=disable

# Cache configuration (unchanged)
cache:
  type: memory
  ttl: 5m
```

### Docker Compose with PostgreSQL

**docker-compose.yml** (updated):
```yaml
version: '3.8'

services:
  postgres:
    image: postgres:15-alpine
    environment:
      - POSTGRES_USER=ci_dashboard
      - POSTGRES_PASSWORD=changeme
      - POSTGRES_DB=ci_dashboard
    volumes:
      - postgres-data:/var/lib/postgresql/data
      - ./migrations:/docker-entrypoint-initdb.d:ro
    ports:
      - "5432:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ci_dashboard"]
      interval: 10s
      timeout: 5s
      retries: 5
    networks:
      - internal
    restart: unless-stopped

  aggregator:
    build:
      context: .
      dockerfile: Dockerfile.aggregator
    environment:
      - GITLAB_TOKEN=${GITLAB_TOKEN}
      - GITHUB_TOKEN=${GITHUB_TOKEN}
      - CACHE_TYPE=memory
      - POSTGRES_URL=postgres://ci_dashboard:changeme@postgres:5432/ci_dashboard?sslmode=disable
    volumes:
      - ./config.yaml:/app/config.yaml:ro
    depends_on:
      postgres:
        condition: service_healthy
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

volumes:
  postgres-data:

networks:
  internal:
```

### Migration Management

**scripts/migrate.sh**:
```bash
#!/bin/bash
set -e

POSTGRES_URL=${POSTGRES_URL:-"postgres://ci_dashboard:changeme@localhost:5432/ci_dashboard?sslmode=disable"}

echo "Running database migrations..."

for migration in migrations/*.sql; do
    echo "Applying $migration..."
    psql "$POSTGRES_URL" < "$migration"
done

echo "Migrations complete!"
```

### Use Cases

**1. Historical Trend Dashboard**:
```sql
-- Success rate over last 30 days
SELECT
    DATE(created_at) as date,
    COUNT(*) as total,
    COUNT(*) FILTER (WHERE status = 'success') as successful,
    ROUND(100.0 * COUNT(*) FILTER (WHERE status = 'success') / COUNT(*), 2) as success_rate
FROM pipelines
WHERE project_id = 'my-project'
  AND created_at > NOW() - INTERVAL '30 days'
GROUP BY DATE(created_at)
ORDER BY date;
```

**2. Slowest Pipelines**:
```sql
-- Find slowest pipelines in last 7 days
SELECT project_id, branch, id, duration, created_at
FROM pipelines
WHERE created_at > NOW() - INTERVAL '7 days'
  AND duration > 0
ORDER BY duration DESC
LIMIT 10;
```

**3. Most Active Projects**:
```sql
-- Projects with most pipeline runs
SELECT project_id, COUNT(*) as run_count
FROM pipelines
WHERE created_at > NOW() - INTERVAL '7 days'
GROUP BY project_id
ORDER BY run_count DESC
LIMIT 10;
```

**4. Failure Analysis**:
```sql
-- Projects with highest failure rate
SELECT
    project_id,
    COUNT(*) as total,
    COUNT(*) FILTER (WHERE status = 'failed') as failures,
    ROUND(100.0 * COUNT(*) FILTER (WHERE status = 'failed') / COUNT(*), 2) as failure_rate
FROM pipelines
WHERE created_at > NOW() - INTERVAL '7 days'
GROUP BY project_id
HAVING COUNT(*) > 10  -- At least 10 runs
ORDER BY failure_rate DESC;
```

### Performance Considerations

**Indexes**: Already included in schema for common queries

**Partitioning** (for very large datasets):
```sql
-- Partition pipelines table by month
CREATE TABLE pipelines (
    -- columns...
) PARTITION BY RANGE (created_at);

CREATE TABLE pipelines_2026_01 PARTITION OF pipelines
    FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');

CREATE TABLE pipelines_2026_02 PARTITION OF pipelines
    FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');
-- etc.
```

**Materialized Views**: Refresh periodically for expensive aggregations

**Connection Pooling**: Already configured (25 max connections)

### Resource Impact

**With PostgreSQL**:
- Aggregator Memory: ~250 MB (+50 MB for database driver)
- Disk Space: ~100 MB per 100K pipeline runs
- CPU: Minimal impact (async writes)

**Write Performance**:
- Cache writes: <1ms (in-memory)
- PostgreSQL writes: 5-10ms (async, don't block)
- Total impact: None (async)

---

## Redis vs PostgreSQL: Which to Use?

### Quick Answer

**For aggregating real-time data** → Use **Redis** (or in-memory cache)

**For storing historical data** → Use **PostgreSQL**

**Best approach** → Use **both together**:
- Redis for caching current state (fast reads)
- PostgreSQL for historical records (complex queries)

### Detailed Comparison

| Feature | Redis | PostgreSQL | Winner |
|---------|-------|------------|--------|
| **Read Speed** | <1ms | 5-10ms | Redis 10x faster |
| **Write Speed** | <1ms | 5-10ms | Redis 10x faster |
| **Data Structure** | Key-value, simple | Relational, complex | Depends on need |
| **Query Complexity** | Simple (GET/SET) | SQL (JOINs, aggregations) | PostgreSQL |
| **Persistence** | Optional (RDB/AOF) | Always persisted | PostgreSQL |
| **Data Loss Risk** | High (if memory-only) | Low (ACID) | PostgreSQL |
| **Memory Usage** | High (all in RAM) | Low (disk + cache) | PostgreSQL |
| **Cost** | RAM expensive | Disk cheap | PostgreSQL |
| **TTL/Expiration** | Built-in | Manual cleanup | Redis |
| **Scalability** | Horizontal (cluster) | Vertical (read replicas) | Redis |

### Use Case Comparison

#### Scenario 1: Real-time Dashboard (Current State Only)

**Need**: Show current status of all pipelines (latest only)

**Best Choice**: **Redis** or **In-Memory**

**Why**:
- Only care about current state
- Need ultra-fast reads (<1ms)
- Data is ephemeral (refreshed every 5 min)
- No historical queries needed
- Simpler deployment

**Architecture**:
```
APIs → Cache (Redis/In-Memory) → Dashboard
         ↑
    Background Refresh
    (every 5 minutes)
```

#### Scenario 2: Historical Analysis + Trends

**Need**: Show pipeline history, success rates over time, trends

**Best Choice**: **PostgreSQL**

**Why**:
- Need to query historical data
- Complex aggregations (AVG, GROUP BY, trends)
- Data must persist forever
- Relational queries (JOIN projects with pipelines)
- Generate reports

**Architecture**:
```
APIs → Cache (current) → Dashboard (real-time)
  ↓
PostgreSQL (historical) → Reports/Analytics
```

#### Scenario 3: Both Real-time + Historical (Recommended)

**Need**: Fast dashboard + historical analysis

**Best Choice**: **Redis + PostgreSQL Together**

**Why**:
- Get benefits of both
- Redis: Fast current state
- PostgreSQL: Historical records and analytics
- Small added complexity

**Architecture**:
```
┌─────────────────────────────────────────┐
│        Background Refresh               │
│                                         │
│  1. Fetch from APIs                     │
│  2. Write to Redis (fast, non-blocking)│
│  3. Write to PostgreSQL (async)        │
└─────────────────────────────────────────┘
             │           │
             ↓           ↓
        ┌────────┐  ┌──────────┐
        │ Redis  │  │PostgreSQL│
        │(cache) │  │(history) │
        └───┬────┘  └────┬─────┘
            │            │
            ↓            ↓
    ┌──────────┐  ┌──────────────┐
    │Dashboard │  │Analytics/    │
    │(fast)    │  │Reports       │
    └──────────┘  └──────────────┘
```

### Deployment Options

#### Option 1: In-Memory Only (Simplest)
```yaml
# Pros: Simplest, fastest, no external dependencies
# Cons: Data lost on restart, limited by RAM

cache:
  type: memory
  ttl: 5m
```

**When to use**: Development, small deployments, don't care about history

#### Option 2: Redis Only
```yaml
# Pros: Fast, persistent (with AOF), simple queries
# Cons: Limited query capabilities, expensive at scale

cache:
  type: redis
  redis_url: redis://localhost:6379
  ttl: 5m
```

**When to use**: Need persistence but no complex queries

#### Option 3: PostgreSQL Only
```yaml
# Pros: Full SQL, complex queries, persistent
# Cons: Slower reads, no built-in TTL

database:
  postgres_url: postgres://localhost:5432/ci_dashboard
```

**When to use**: Historical analysis is main use case

#### Option 4: Redis + PostgreSQL (Recommended)
```yaml
# Pros: Fast reads + complex queries + persistence
# Cons: Two systems to manage

cache:
  type: redis
  redis_url: redis://localhost:6379
  ttl: 5m

database:
  postgres_url: postgres://localhost:5432/ci_dashboard
```

**When to use**: Production with real-time + historical needs

### Redis Data Structures for Aggregation

If you choose Redis for aggregation:

**Option 1: Simple Key-Value**
```
Key: gitlab:projects
Value: JSON array of all projects

Key: github:projects
Value: JSON array of all projects

Key: gitlab:project:123:pipelines
Value: JSON array of recent pipelines
```

**Option 2: Redis Hashes** (more efficient)
```
HSET projects gitlab:123 '{"name":"my-project",...}'
HSET projects github:owner/repo '{"name":"my-repo",...}'

HSET pipelines gitlab:123:main:latest '{"status":"success",...}'
```

**Option 3: Redis Sorted Sets** (for rankings)
```
ZADD project:activity <timestamp> "gitlab:123"
ZADD project:activity <timestamp> "github:owner/repo"

# Get most active projects
ZREVRANGE project:activity 0 9  # Top 10
```

**Option 4: Redis Streams** (for events)
```
XADD pipeline:events * platform gitlab project 123 status success

# Read recent events
XREAD COUNT 100 STREAMS pipeline:events 0
```

### PostgreSQL Aggregation Queries

If you choose PostgreSQL for aggregation:

**Aggregating across platforms**:
```sql
-- All projects from all platforms
SELECT platform, COUNT(*) as count
FROM projects
GROUP BY platform;

-- Cross-platform pipeline statistics
SELECT
    platform,
    COUNT(*) as total,
    COUNT(*) FILTER (WHERE status = 'success') as successful,
    AVG(duration) as avg_duration
FROM pipelines
WHERE created_at > NOW() - INTERVAL '7 days'
GROUP BY platform;

-- Most active projects (across all platforms)
SELECT
    p.platform,
    p.name,
    COUNT(pl.id) as pipeline_count
FROM projects p
LEFT JOIN pipelines pl ON pl.project_id = p.id
WHERE pl.created_at > NOW() - INTERVAL '7 days'
GROUP BY p.platform, p.name
ORDER BY pipeline_count DESC
LIMIT 10;
```

### Resource Requirements

#### In-Memory Only
```
Memory: 200 MB (for ~1000 projects, ~10K pipelines)
Disk: 0 MB
Setup: None
```

#### Redis
```
Memory: 500 MB (Redis overhead + data)
Disk: 100 MB (for RDB snapshots)
Setup: Install Redis
```

#### PostgreSQL
```
Memory: 100 MB (just application)
Disk: 1 GB (for historical data)
Setup: Install PostgreSQL + run migrations
```

#### Redis + PostgreSQL
```
Memory: 600 MB (both)
Disk: 1 GB (PostgreSQL)
Setup: Install both + run migrations
```

### Decision Matrix

Choose based on your needs:

| Requirement | Recommended | Alternative |
|-------------|-------------|-------------|
| Dev/Testing | In-Memory | Redis |
| Simple production | Redis | In-Memory + cron backup |
| Historical analysis | PostgreSQL + Redis | PostgreSQL only |
| Large scale (>10K projects) | Redis + PostgreSQL | PostgreSQL + read replicas |
| Budget constrained | In-Memory | PostgreSQL |
| No DevOps expertise | In-Memory | Redis (managed) |
| Enterprise | Redis + PostgreSQL | All three (+ Redis cluster) |

### Recommended Approach

**Start Simple, Scale Up**:

1. **Phase 1** (MVP): In-memory only
   - Fastest to build
   - No external dependencies
   - Perfect for testing

2. **Phase 2** (Production): Add Redis
   - Persistent across restarts
   - Share cache between processes
   - Still simple

3. **Phase 3** (Analytics): Add PostgreSQL
   - Historical data storage
   - Complex queries and reports
   - Trend analysis

You can run all three phases with the same codebase - just change configuration!

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
