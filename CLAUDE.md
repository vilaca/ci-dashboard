# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A web-based dashboard for monitoring CI/CD pipelines across multiple platforms. Built with Go, using the standard library HTTP server.

## Development Commands

The project uses a Makefile for common tasks:

### Building
```bash
make build          # Builds binary to bin/ci-dashboard
```

### Running
```bash
make run            # Run with go run
PORT=3000 make run  # Run on custom port
```

### Testing
```bash
make test           # Run all tests with verbose output
make test-coverage  # Run tests with coverage
go test -run TestHandleHealth ./internal/dashboard  # Run specific test
```

### Code Quality
```bash
make lint  # Run go vet
make fmt   # Format code with go fmt
```

## Project Structure

- `cmd/ci-dashboard/main.go` - Application entry point, composition root
- `internal/api/` - CI/CD platform API clients
  - `client.go` - Common Client interface
  - `gitlab/` - GitLab API client implementation
  - `github/` - GitHub Actions API client implementation
- `internal/config/` - Configuration loading from environment variables
- `internal/dashboard/` - HTTP request handlers and routing logic
  - `handler.go` - HTTP handlers with dependency injection
  - `renderer.go` - HTML and JSON rendering logic
- `internal/domain/` - Domain models (Pipeline, Build, Project, Status)
- `internal/service/` - Business logic layer
  - `pipeline_service.go` - Orchestrates pipeline operations across platforms
- `pkg/client/` - Public client interfaces for external use
- `web/static/` - Static assets (CSS, JS, images)
- `web/templates/` - HTML templates

## Design Principles

This codebase strictly follows these software engineering principles:

### Core Principles
- **DRY (Don't Repeat Yourself)**: Avoid code duplication by extracting common logic into reusable functions/types
- **KISS (Keep It Simple, Stupid)**: Prefer simple, straightforward solutions over complex ones
- **SOLID**:
  - **S**ingle Responsibility: Each type/function has one clear responsibility
  - **O**pen/Closed: Open for extension, closed for modification
  - **L**iskov Substitution: Interfaces can be substituted with implementations
  - **I**nterface Segregation: Small, focused interfaces (e.g., `Renderer`, `Logger`)
  - **D**ependency Inversion: Depend on interfaces, not concrete types
- **SoC (Separation of Concerns)**: Separate rendering, business logic, and HTTP handling
- **IoC (Inversion of Control)**: Dependencies are injected, not created internally
- **SLAP (Single Level of Abstraction Principle)**: Each function operates at one level of abstraction
- **POLA/POLS (Principle of Least Astonishment)**: Code behaves as expected, no surprises
- **Law of Demeter (LoD)**: Don't talk to strangers - only interact with direct dependencies
- **High Cohesion / Low Coupling**: Related functionality together, minimal dependencies between components

### Testing Principles
- **FIRST**:
  - **F**ast: Tests run quickly without external dependencies
  - **I**ndependent: Tests don't depend on each other
  - **R**epeatable: Tests produce same results every time
  - **S**elf-validating: Tests have clear pass/fail
  - **T**imely: Tests are written alongside code
- **AAA (Arrange-Act-Assert)**: All tests follow this structure with clear comments

### Implementation Guidelines

#### Dependency Injection
All dependencies are injected through constructors:
```go
func NewHandler(renderer Renderer, logger Logger) *Handler
```

Never create dependencies internally. The composition root is in `cmd/ci-dashboard/main.go:buildServer()`.

#### Interface Design
Define small, focused interfaces:
```go
type Renderer interface {
    RenderIndex(w io.Writer) error
    RenderHealth(w io.Writer) error
}
```

Depend on interfaces, not concrete types.

#### Law of Demeter
Only call methods on:
- Direct dependencies (fields)
- Parameters passed to the method
- Objects you create
- Yourself

**Example**:
```go
// ✅ GOOD
func (h *Handler) handlePipelines(w http.ResponseWriter, r *http.Request) {
    pipelines, err := h.pipelineService.GetLatestPipelines(r.Context())
    h.renderer.RenderPipelines(w, pipelines)
}

// ❌ BAD - Method chaining violates LoD
func (h *Handler) bad(w http.ResponseWriter, r *http.Request) {
    html := h.pipelineService.GetClient().GetHTTP().DoRequest(...)
}
```

#### High Cohesion / Low Coupling
- **High Cohesion**: Each package/type has strongly related, focused functionality
  - `gitlab` package: Only GitLab API operations
  - `PipelineService`: Only pipeline orchestration
  - `Handler`: Only HTTP handling

- **Low Coupling**: Minimal dependencies via interfaces
  - Handler depends on 3 interfaces (not concrete types)
  - Service depends only on Client interface
  - Composition root is only place that knows concrete types

#### Error Handling
- Always handle errors explicitly
- Log errors at appropriate level
- Return errors to caller for handling
- Use descriptive error messages

#### Testing
- Use mock implementations for all interfaces
- Follow AAA pattern with comments
- Test both success and error paths
- Tests must be independent and fast

## Architecture

### HTTP Server
The application uses Go's standard `net/http` package with dependency injection:

**Composition Root** (`cmd/ci-dashboard/main.go`):
- `buildServer()` function wires all dependencies
- Creates concrete implementations (renderer, logger)
- Injects them into handlers
- Returns configured `http.Handler`

**Handler** (`internal/dashboard/handler.go`):
- Accepts `Renderer` and `Logger` interfaces via constructor
- `RegisterRoutes()` registers HTTP routes on provided mux
- Each handler method has single responsibility
- Errors are logged and returned as HTTP errors

**Renderer** (`internal/dashboard/renderer.go`):
- `Renderer` interface defines rendering contract
- `HTMLRenderer` implements concrete HTML rendering
- Separation of concerns: rendering logic isolated from HTTP handling

### Configuration
Configuration is loaded from environment variables via `internal/config/config.go`:
- `PORT` - Server port (default: 8080)
- `GITLAB_URL` - GitLab instance URL (default: https://gitlab.com)
- `GITLAB_TOKEN` - GitLab personal access token
- `GITHUB_URL` - GitHub API URL (default: https://api.github.com)
- `GITHUB_TOKEN` - GitHub personal access token
- `WATCHED_REPOS` - Comma-separated list of project IDs to watch
- Config struct is simple data container
- `Load()` function handles environment variable parsing
- Helper methods: `HasGitLabConfig()`, `HasGitHubConfig()`, `GetWatchedRepos()`

### Domain Model
The `internal/domain` package defines platform-agnostic models:
- `Pipeline` - Represents a CI/CD pipeline run
- `Build` - Represents a job/build within a pipeline
- `Project` - Represents a code repository
- `Status` - Enum for pipeline/build status (pending, running, success, failed, etc.)

These models provide a unified interface regardless of the underlying CI platform.

### Service Layer
The `PipelineService` (`internal/service/pipeline_service.go`) orchestrates business logic:
- Manages multiple CI platform clients
- Fetches data concurrently from all platforms
- Aggregates results into unified domain models
- Follows Open/Closed Principle - new platforms can be added without modifying service

### API Clients
Each CI platform has its own client implementation:
- All implement the `api.Client` interface
- Responsible for HTTP communication with platform APIs
- Convert platform-specific responses to domain models
- Injected with `HTTPClient` interface for testability

**GitLab Client** (`internal/api/gitlab/client.go`):
- Uses GitLab REST API v4
- Authenticates with `PRIVATE-TOKEN` header
- Converts GitLab-specific types to domain models

**GitHub Client** (`internal/api/github/client.go`):
- Uses GitHub REST API v3
- Authenticates with Bearer token
- Maps workflow runs to Pipeline domain model
- Handles GitHub's status+conclusion pattern

### Adding New Features

#### Adding New Routes
1. Define new method in `Renderer` interface if needed
2. Implement method in `HTMLRenderer`
3. Add handler method to `Handler` struct in `internal/dashboard/handler.go`
4. Register route in `RegisterRoutes()` method
5. Add tests following AAA pattern in `internal/dashboard/handler_test.go`
6. Test with mock renderer to verify behavior

#### Adding New Dependencies
1. Define interface for the dependency
2. Create implementation
3. Add constructor accepting dependencies via injection
4. Wire in `buildServer()` composition root
5. Write tests with mock implementations

#### Adding a New CI Platform

To add support for a new CI/CD platform (e.g., Jenkins, CircleCI):

1. **Create client implementation** in `internal/api/<platform>/client.go`:
```go
package jenkins

import (
    "context"
    "github.com/vilaca/ci-dashboard/internal/api"
    "github.com/vilaca/ci-dashboard/internal/domain"
)

type Client struct {
    baseURL    string
    token      string
    httpClient HTTPClient
}

func NewClient(config api.ClientConfig, httpClient HTTPClient) *Client {
    return &Client{
        baseURL:    config.BaseURL,
        token:      config.Token,
        httpClient: httpClient,
    }
}

func (c *Client) GetProjects(ctx context.Context) ([]domain.Project, error) {
    // Implementation
}

func (c *Client) GetLatestPipeline(ctx context.Context, projectID, branch string) (*domain.Pipeline, error) {
    // Implementation
}

func (c *Client) GetPipelines(ctx context.Context, projectID string, limit int) ([]domain.Pipeline, error) {
    // Implementation
}
```

2. **Add configuration** to `internal/config/config.go`:
```go
type Config struct {
    // ... existing fields ...
    JenkinsURL   string
    JenkinsToken string
}

func (c *Config) HasJenkinsConfig() bool {
    return c.JenkinsToken != ""
}
```

3. **Register in composition root** (`cmd/ci-dashboard/main.go`):
```go
if cfg.HasJenkinsConfig() {
    jenkinsClient := jenkins.NewClient(api.ClientConfig{
        BaseURL: cfg.JenkinsURL,
        Token:   cfg.JenkinsToken,
    }, httpClient)
    pipelineService.RegisterClient("jenkins", jenkinsClient)
}
```

4. **Write comprehensive tests** in `internal/api/<platform>/client_test.go`:
```go
func TestGetProjects(t *testing.T) {
    // Arrange
    mockHTTP := &mockHTTPClient{/* ... */}
    client := NewClient(/* ... */, mockHTTP)

    // Act
    projects, err := client.GetProjects(context.Background())

    // Assert
    if err != nil {
        t.Fatalf("expected no error, got %v", err)
    }
    // ... more assertions
}
```

That's it! The service layer automatically handles the new platform without modification (Open/Closed Principle).

### Module
Module path: `github.com/vilaca/ci-dashboard`

## Environment Variables Reference

```bash
# Server
PORT=8080                                    # HTTP server port

# GitLab
GITLAB_URL=https://gitlab.com                # GitLab instance URL
GITLAB_TOKEN=glpat-xxxxxxxxxxxx             # GitLab personal access token

# GitHub
GITHUB_URL=https://api.github.com            # GitHub API URL
GITHUB_TOKEN=ghp_xxxxxxxxxxxx               # GitHub personal access token

# Optional: Specific repos to watch (otherwise shows all)
WATCHED_REPOS=123,456                        # GitLab project IDs
WATCHED_REPOS=owner/repo1,owner/repo2        # GitHub repos
```
