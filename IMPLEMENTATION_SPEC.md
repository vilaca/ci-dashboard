# CI/CD Dashboard - Complete Implementation Specification

## Table of Contents
1. [System Overview](#system-overview)
2. [Architecture Principles](#architecture-principles)
3. [System Architecture](#system-architecture)
4. [Domain Model](#domain-model)
5. [API Client Layer](#api-client-layer)
6. [Caching Strategy](#caching-strategy)
7. [Service Layer](#service-layer)
8. [Background Refresh System](#background-refresh-system)
9. [HTTP Handler Layer](#http-handler-layer)
10. [Frontend Implementation](#frontend-implementation)
11. [Configuration](#configuration)
12. [Rate Limiting](#rate-limiting)
13. [Implementation Steps](#implementation-steps)
14. [Testing Strategy](#testing-strategy)

---

## System Overview

### Purpose
A read-only web dashboard for monitoring CI/CD pipelines across multiple platforms (GitLab, GitHub Actions, and extensible to others).

### Key Requirements
- **Read-Only**: Never write, modify, or delete data on CI/CD platforms
- **Multi-Platform**: Support multiple CI/CD platforms simultaneously
- **Fast Loading**: Page loads instantly without waiting for API calls
- **Real-time Updates**: Frontend polls every 5 seconds for updates
- **Background Refresh**: Cache refreshes every 5 minutes in background
- **Rate Limit Aware**: Respects API rate limits (especially GitHub's 5000/hour)
- **Stale-While-Revalidate**: Serve cached data instantly, refresh in background

### Technology Stack
- **Language**: Go (using standard library)
- **HTTP Server**: net/http (standard library)
- **Templates**: html/template (standard library)
- **No External Dependencies**: Pure Go standard library only

---

## Architecture Principles

### SOLID Principles

#### Single Responsibility Principle (SRP)
Each type/function has one clear responsibility:
- `Client`: Only handles API communication with one platform
- `PipelineService`: Only orchestrates business logic
- `Handler`: Only handles HTTP requests/responses
- `Renderer`: Only renders HTML/JSON
- `BackgroundRefresher`: Only manages periodic cache refresh

#### Open/Closed Principle (OCP)
Open for extension, closed for modification:
- Adding new CI/CD platforms doesn't require modifying existing code
- New platforms implement the `Client` interface
- Register new clients in composition root only

#### Liskov Substitution Principle (LSP)
Interfaces can be substituted with any implementation:
- Any `Client` implementation works with `PipelineService`
- Any `Renderer` implementation works with `Handler`

#### Interface Segregation Principle (ISP)
Small, focused interfaces:
- `Client` - Basic operations (GetProjects, GetPipelines, GetBranches)
- `ExtendedClient` - Optional operations (GetMergeRequests, GetIssues)
- `UserClient` - Optional user operations (GetCurrentUser)
- `WorkflowClient` - Optional workflow operations (GitHub-specific)
- `EventsClient` - Optional event operations (for cache invalidation)

#### Dependency Inversion Principle (DIP)
Depend on interfaces, not concrete types:
- `Handler` depends on `Renderer` interface, not `HTMLRenderer`
- `PipelineService` depends on `Client` interface, not `GitLabClient`
- Concrete implementations injected through constructors

### Additional Principles

#### Dependency Injection (DI)
All dependencies injected through constructors:
```go
func NewHandler(renderer Renderer, logger Logger, service PipelineService) *Handler
```

#### Inversion of Control (IoC)
Composition root (`cmd/ci-dashboard/main.go`) wires all dependencies.

#### Law of Demeter (LoD)
Only call methods on:
- Direct dependencies (fields)
- Parameters
- Objects you create
- Yourself

Never chain: `service.GetClient().DoRequest()` ❌

#### High Cohesion / Low Coupling
- Each package has strongly related functionality
- Minimal dependencies via interfaces
- Composition root is only place knowing concrete types

---

## System Architecture

### Component Overview

```
┌─────────────────────────────────────────────────────────────┐
│                         Browser                              │
│  (JavaScript polls /api/repositories every 5 seconds)       │
└────────────────────────────┬────────────────────────────────┘
                             │ HTTP
                             ↓
┌─────────────────────────────────────────────────────────────┐
│                      HTTP Handler                            │
│  - handleRepositories: Serve HTML skeleton                   │
│  - handleRepositoriesBulk: Return JSON (cache-only)         │
│  - handleRepositoryDetail: Serve detail page                 │
│  - handleHealth: Health check endpoint                       │
└────────────────────────────┬────────────────────────────────┘
                             │
                             ↓
┌─────────────────────────────────────────────────────────────┐
│                    Pipeline Service                          │
│  - GetAllProjects: Fetch from all platforms                  │
│  - GetDefaultBranchForProject: Get default branch + pipeline │
│  - GetMergeRequestsForProject: Get MRs for project          │
│  - Orchestrates calls to multiple clients                    │
└────────────────────────────┬────────────────────────────────┘
                             │
              ┌──────────────┼──────────────┐
              ↓              ↓              ↓
    ┌─────────────┐  ┌─────────────┐  ┌──────────┐
    │ GitLab      │  │ GitHub      │  │ Jenkins  │
    │ Client      │  │ Client      │  │ Client   │
    │ (Cached)    │  │ (Cached)    │  │ (Future) │
    └─────────────┘  └─────────────┘  └──────────┘
           │                │
           ↓                ↓
    ┌─────────────┐  ┌─────────────┐
    │ StaleCache  │  │ StaleCache  │
    │ (Memory)    │  │ (Memory)    │
    └─────────────┘  └─────────────┘
           │                │
           ↓                ↓
    ┌─────────────┐  ┌─────────────┐
    │ GitLab      │  │ GitHub      │
    │ REST API    │  │ REST API    │
    └─────────────┘  └─────────────┘

         Background Refresher
         (Every 5 minutes)
              │
              ↓
    ForceRefreshAllCaches()
    - Fetches fresh data from APIs
    - Populates all caches
    - Non-blocking
```

### Data Flow

#### Initial Page Load (Empty Cache)
1. User requests `/` (main page)
2. Handler calls `GetUserProfiles()` → Returns empty (cache miss)
3. Handler renders HTML skeleton immediately
4. Browser receives page instantly
5. JavaScript starts polling `/api/repositories` every 5 seconds
6. First poll returns empty array (cache empty)

#### Background Refresh (Happens in parallel)
1. Background refresher starts immediately on server start
2. Calls `ForceRefreshAllCaches()`
3. For each platform (GitLab, GitHub):
   - Fetch page 1 of projects → Cache immediately
   - For each project in page:
     - Fetch branches (limit 200) → Cache
     - Fetch pipelines (limit 50) → Cache
     - Fetch merge requests → Cache
     - Fetch issues → Cache
   - Fetch page 2 of projects → Cache immediately
   - Repeat...
4. Cache now populated with fresh data

#### Subsequent Polls (Cache Populated)
1. JavaScript polls `/api/repositories`
2. Handler calls `GetAllProjects()` → Returns from cache (fast)
3. For each project, calls `GetDefaultBranchForProject()` → Returns from cache
4. Returns JSON with all data
5. Browser updates UI

#### Every 5 Minutes
1. Background refresher triggers periodic refresh
2. Repeats the ForceRefreshAllCaches process
3. Updates all caches with fresh data
4. Next poll picks up updated data

---

## Domain Model

The domain model provides platform-agnostic types that unify GitLab, GitHub, and future platforms.

### Location
`internal/domain/` package

### Core Types

#### Project
Represents a code repository/project.

```go
package domain

type Project struct {
    ID            string       // Platform-specific ID (e.g., "123" or "owner/repo")
    Name          string       // Human-readable name
    Description   string       // Project description
    WebURL        string       // URL to view project in browser
    DefaultBranch string       // Name of default branch (e.g., "main", "master")
    Platform      string       // "gitlab" or "github"
    Visibility    string       // "public", "private", "internal"
    AvatarURL     string       // Project avatar/logo URL
    LastActivity  time.Time    // Last activity timestamp
    Permissions   Permissions  // User's access level
}
```

#### Permissions
User's access level to a project.

```go
type Permissions struct {
    AccessLevel int  // GitLab: 10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner
    Admin       bool // GitHub: Admin access
    Push        bool // GitHub: Push access
    Pull        bool // GitHub: Pull access
}
```

#### Pipeline
Represents a CI/CD pipeline run.

```go
type Pipeline struct {
    ID         string    // Pipeline ID
    ProjectID  string    // Project ID this pipeline belongs to
    Repository string    // Project name (for display)
    Status     Status    // Pipeline status
    Ref        string    // Branch or tag name
    Branch     string    // Branch name
    SHA        string    // Git commit SHA
    WebURL     string    // URL to view pipeline
    CreatedAt  time.Time // Pipeline creation time
    UpdatedAt  time.Time // Pipeline last update time
    StartedAt  time.Time // Pipeline start time
    FinishedAt time.Time // Pipeline finish time
    Duration   int       // Duration in seconds
    Author     string    // Pipeline author username
    Message    string    // Commit message
}
```

#### Status
Unified status enum across platforms.

```go
type Status string

const (
    StatusPending  Status = "pending"   // Waiting to run
    StatusRunning  Status = "running"   // Currently running
    StatusSuccess  Status = "success"   // Completed successfully
    StatusFailed   Status = "failed"    // Failed
    StatusCanceled Status = "canceled"  // Canceled by user
    StatusSkipped  Status = "skipped"   // Skipped
    StatusCreated  Status = "created"   // Created but not started
    StatusManual   Status = "manual"    // Waiting for manual action
)
```

**Platform Mapping:**

GitLab Status → Domain Status:
- `pending` → `pending`
- `running` → `running`
- `success` → `success`
- `failed` → `failed`
- `canceled` → `canceled`
- `skipped` → `skipped`
- `created` → `created`
- `manual` → `manual`

GitHub Status+Conclusion → Domain Status:
- `queued` → `pending`
- `in_progress` → `running`
- `completed` + `success` → `success`
- `completed` + `failure` → `failed`
- `completed` + `cancelled` → `canceled`
- `completed` + `skipped` → `skipped`

#### Branch
Represents a git branch.

```go
type Branch struct {
    Name           string    // Branch name
    ProjectID      string    // Project ID
    Repository     string    // Project name (for display)
    IsDefault      bool      // Is this the default branch?
    Protected      bool      // Is branch protected?
    SHA            string    // Latest commit SHA
    CommitAuthor   string    // Latest commit author
    CommitMessage  string    // Latest commit message
    LastCommitDate time.Time // Latest commit timestamp
}
```

#### MergeRequest
Represents a merge request (GitLab) or pull request (GitHub).

```go
type MergeRequest struct {
    ID          string    // MR ID
    IID         int       // Internal ID (display number)
    ProjectID   string    // Project ID
    Repository  string    // Project name
    Title       string    // MR title
    Description string    // MR description
    State       string    // "opened", "closed", "merged"
    IsDraft     bool      // Is draft MR?
    Author      string    // MR author username
    SourceBranch string   // Source branch name
    TargetBranch string   // Target branch name
    WebURL      string    // URL to view MR
    CreatedAt   time.Time // MR creation time
    UpdatedAt   time.Time // MR last update time
    Reviewers   []string  // List of reviewer usernames
}
```

#### Issue
Represents an issue.

```go
type Issue struct {
    ID         string    // Issue ID
    IID        int       // Internal ID (display number)
    ProjectID  string    // Project ID
    Repository string    // Project name
    Title      string    // Issue title
    Description string   // Issue description
    State      string    // "opened", "closed"
    Author     string    // Issue author username
    Assignees  []string  // List of assignee usernames
    Labels     []string  // List of labels
    WebURL     string    // URL to view issue
    CreatedAt  time.Time // Issue creation time
    UpdatedAt  time.Time // Issue last update time
}
```

#### UserProfile
Represents authenticated user's profile.

```go
type UserProfile struct {
    ID        string // User ID
    Username  string // Username
    Email     string // Email address
    Name      string // Full name
    AvatarURL string // Avatar image URL
    Platform  string // "gitlab" or "github"
}
```

#### Event
Represents a repository event (for cache invalidation).

```go
type Event struct {
    Type      string    // Event type (push, merge_request, etc.)
    ProjectID string    // Project ID
    CreatedAt time.Time // Event timestamp
}
```

### Platform Constants

```go
const (
    PlatformGitLab = "gitlab"
    PlatformGitHub = "github"
)
```

---

## API Client Layer

### Location
`internal/api/` package

### Client Interface
The core interface all platform clients must implement.

```go
package api

type Client interface {
    // GetProjects returns all projects accessible by the configured credentials.
    GetProjects(ctx context.Context) ([]domain.Project, error)

    // GetProjectsPage returns a single page of projects.
    // Returns: projects for this page, whether there's a next page, and error.
    GetProjectsPage(ctx context.Context, page int) ([]domain.Project, bool, error)

    // GetProjectCount returns the total number of projects accessible.
    GetProjectCount(ctx context.Context) (int, error)

    // GetLatestPipeline returns the most recent pipeline for a given project and branch.
    GetLatestPipeline(ctx context.Context, projectID, branch string) (*domain.Pipeline, error)

    // GetPipelines returns recent pipelines for a given project.
    GetPipelines(ctx context.Context, projectID string, limit int) ([]domain.Pipeline, error)

    // GetBranches returns branches for a given project.
    // limit controls max branches to return (0 = all branches).
    GetBranches(ctx context.Context, projectID string, limit int) ([]domain.Branch, error)

    // GetBranch retrieves a single branch by name.
    GetBranch(ctx context.Context, projectID, branchName string) (*domain.Branch, error)
}
```

### Extended Interfaces

#### ExtendedClient
For platforms supporting merge requests and issues (GitLab, GitHub).

```go
type ExtendedClient interface {
    Client

    // GetMergeRequests returns open merge requests (PRs) for a project.
    GetMergeRequests(ctx context.Context, projectID string) ([]domain.MergeRequest, error)

    // GetIssues returns open issues for a project.
    GetIssues(ctx context.Context, projectID string) ([]domain.Issue, error)
}
```

#### UserClient
For platforms supporting user profile operations.

```go
type UserClient interface {
    Client

    // GetCurrentUser returns the profile of the authenticated user.
    GetCurrentUser(ctx context.Context) (*domain.UserProfile, error)
}
```

#### WorkflowClient
For platforms with workflow-specific operations (GitHub).

```go
type WorkflowClient interface {
    Client

    // GetWorkflowRuns returns runs for a specific workflow.
    GetWorkflowRuns(ctx context.Context, projectID string, workflowID string, limit int) ([]domain.Pipeline, error)
}
```

#### EventsClient
For platforms supporting event polling (for cache invalidation).

```go
type EventsClient interface {
    Client

    // GetEvents returns events for a project since a specific time.
    GetEvents(ctx context.Context, projectID string, since time.Time) ([]domain.Event, error)
}
```

### Client Configuration

```go
type ClientConfig struct {
    BaseURL string // API base URL
    Token   string // Authentication token
}
```

---

## GitLab Client Implementation

### Location
`internal/api/gitlab/client.go`

### Structure

```go
package gitlab

type Client struct {
    BaseURL    string
    Token      string
    HTTPClient HTTPClient
}

type HTTPClient interface {
    Do(req *http.Request) (*http.Response, error)
}

func NewClient(config api.ClientConfig, httpClient HTTPClient) *Client {
    return &Client{
        BaseURL:    config.BaseURL,
        Token:      config.Token,
        HTTPClient: httpClient,
    }
}
```

### Authentication
GitLab uses `PRIVATE-TOKEN` header:

```go
func (c *Client) doRequest(ctx context.Context, url string, result interface{}) error {
    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return err
    }

    req.Header.Set("PRIVATE-TOKEN", c.Token)

    resp, err := c.HTTPClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
    }

    return json.NewDecoder(resp.Body).Decode(result)
}
```

### GetProjects Implementation

**API Endpoint**: `GET /api/v4/projects?membership=true&per_page=100`

```go
func (c *Client) GetProjects(ctx context.Context) ([]domain.Project, error) {
    var allProjects []domain.Project
    page := 1

    for {
        url := fmt.Sprintf("%s/api/v4/projects?membership=true&per_page=100&page=%d",
            c.BaseURL, page)

        var projects []gitlabProject
        if err := c.doRequest(ctx, url, &projects); err != nil {
            return nil, err
        }

        if len(projects) == 0 {
            break
        }

        for _, p := range projects {
            allProjects = append(allProjects, c.convertProject(p))
        }

        page++
    }

    return allProjects, nil
}
```

**GitLab Response Structure**:
```json
{
  "id": 123,
  "name": "my-project",
  "description": "Project description",
  "web_url": "https://gitlab.com/user/my-project",
  "default_branch": "main",
  "visibility": "private",
  "avatar_url": "https://...",
  "last_activity_at": "2026-01-26T12:00:00.000Z",
  "permissions": {
    "project_access": {
      "access_level": 40
    }
  }
}
```

**Conversion**:
```go
func (c *Client) convertProject(p gitlabProject) domain.Project {
    return domain.Project{
        ID:            strconv.Itoa(p.ID),
        Name:          p.Name,
        Description:   p.Description,
        WebURL:        p.WebURL,
        DefaultBranch: p.DefaultBranch,
        Platform:      domain.PlatformGitLab,
        Visibility:    p.Visibility,
        AvatarURL:     p.AvatarURL,
        LastActivity:  p.LastActivityAt,
        Permissions: domain.Permissions{
            AccessLevel: p.Permissions.ProjectAccess.AccessLevel,
        },
    }
}
```

### GetBranches Implementation

**API Endpoint**: `GET /api/v4/projects/{id}/repository/branches?per_page={limit}`

```go
func (c *Client) GetBranches(ctx context.Context, projectID string, limit int) ([]domain.Branch, error) {
    if limit == 0 {
        limit = 50
    }

    url := fmt.Sprintf("%s/api/v4/projects/%s/repository/branches?per_page=%d",
        c.BaseURL, projectID, limit)

    var branches []gitlabBranch
    if err := c.doRequest(ctx, url, &branches); err != nil {
        return nil, err
    }

    result := make([]domain.Branch, len(branches))
    for i, b := range branches {
        result[i] = c.convertBranch(b, projectID)
    }

    return result, nil
}
```

**GitLab Response Structure**:
```json
{
  "name": "main",
  "default": true,
  "protected": true,
  "commit": {
    "id": "abc123...",
    "message": "Commit message",
    "author_name": "John Doe",
    "committed_date": "2026-01-26T12:00:00.000Z"
  }
}
```

**Conversion**:
```go
func (c *Client) convertBranch(b gitlabBranch, projectID string) domain.Branch {
    return domain.Branch{
        Name:           b.Name,
        ProjectID:      projectID,
        IsDefault:      b.Default,
        Protected:      b.Protected,
        SHA:            b.Commit.ID,
        CommitAuthor:   b.Commit.AuthorName,
        CommitMessage:  b.Commit.Message,
        LastCommitDate: b.Commit.CommittedDate,
    }
}
```

### GetBranch Implementation

**API Endpoint**: `GET /api/v4/projects/{id}/repository/branches/{branch}`

```go
func (c *Client) GetBranch(ctx context.Context, projectID, branchName string) (*domain.Branch, error) {
    encodedBranch := url.QueryEscape(branchName)
    url := fmt.Sprintf("%s/api/v4/projects/%s/repository/branches/%s",
        c.BaseURL, projectID, encodedBranch)

    var branch gitlabBranch
    if err := c.doRequest(ctx, url, &branch); err != nil {
        return nil, err
    }

    result := c.convertBranch(branch, projectID)
    return &result, nil
}
```

### GetLatestPipeline Implementation

**API Endpoint**: `GET /api/v4/projects/{id}/pipelines?ref={branch}&per_page=1`

```go
func (c *Client) GetLatestPipeline(ctx context.Context, projectID, branch string) (*domain.Pipeline, error) {
    url := fmt.Sprintf("%s/api/v4/projects/%s/pipelines?ref=%s&per_page=1",
        c.BaseURL, projectID, branch)

    var pipelines []gitlabPipeline
    if err := c.doRequest(ctx, url, &pipelines); err != nil {
        return nil, err
    }

    if len(pipelines) == 0 {
        return nil, nil
    }

    pipeline := c.convertPipeline(pipelines[0], projectID)
    return &pipeline, nil
}
```

**GitLab Response Structure**:
```json
{
  "id": 456,
  "status": "success",
  "ref": "main",
  "sha": "abc123...",
  "web_url": "https://gitlab.com/user/project/-/pipelines/456",
  "created_at": "2026-01-26T12:00:00.000Z",
  "updated_at": "2026-01-26T12:05:00.000Z",
  "started_at": "2026-01-26T12:00:30.000Z",
  "finished_at": "2026-01-26T12:05:00.000Z",
  "duration": 270
}
```

**Status Conversion**:
```go
func (c *Client) convertStatus(status string) domain.Status {
    switch status {
    case "pending":
        return domain.StatusPending
    case "running":
        return domain.StatusRunning
    case "success":
        return domain.StatusSuccess
    case "failed":
        return domain.StatusFailed
    case "canceled":
        return domain.StatusCanceled
    case "skipped":
        return domain.StatusSkipped
    case "created":
        return domain.StatusCreated
    case "manual":
        return domain.StatusManual
    default:
        return domain.StatusPending
    }
}
```

### GetMergeRequests Implementation

**API Endpoint**: `GET /api/v4/projects/{id}/merge_requests?state=opened`

```go
func (c *Client) GetMergeRequests(ctx context.Context, projectID string) ([]domain.MergeRequest, error) {
    url := fmt.Sprintf("%s/api/v4/projects/%s/merge_requests?state=opened",
        c.BaseURL, projectID)

    var mrs []gitlabMergeRequest
    if err := c.doRequest(ctx, url, &mrs); err != nil {
        return nil, err
    }

    result := make([]domain.MergeRequest, len(mrs))
    for i, mr := range mrs {
        result[i] = c.convertMergeRequest(mr, projectID)
    }

    return result, nil
}
```

---

## GitHub Client Implementation

### Location
`internal/api/github/client.go`

### Structure

```go
package github

type Client struct {
    *api.BaseClient
    rateLimitMu        sync.RWMutex
    rateLimitRemaining int
    rateLimitReset     time.Time
}

func NewClient(baseURL string, config api.ClientConfig, httpClient HTTPClient) *Client {
    return &Client{
        BaseClient:         api.NewBaseClient(baseURL, config.Token, httpClient),
        rateLimitRemaining: -1, // -1 means "not yet known", 0 means "exhausted"
    }
}
```

### Authentication
GitHub uses `Authorization: Bearer {token}` header:

```go
func (c *Client) doRequest(ctx context.Context, url string, result interface{}) error {
    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return err
    }

    req.Header.Set("Authorization", "Bearer "+c.Token)
    req.Header.Set("Accept", "application/vnd.github.v3+json")

    resp, err := c.HTTPClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    // Update rate limit tracking from response headers
    c.updateRateLimit(resp.Header)

    if resp.StatusCode != http.StatusOK {
        body, _ := io.ReadAll(resp.Body)
        return fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
    }

    return json.NewDecoder(resp.Body).Decode(result)
}
```

### Rate Limiting Implementation

GitHub enforces 5000 requests/hour. The client must respect this limit.

#### Rate Limit Headers
GitHub returns these headers with every response:
- `X-RateLimit-Limit`: Total requests allowed (5000)
- `X-RateLimit-Remaining`: Requests remaining
- `X-RateLimit-Reset`: Unix timestamp when limit resets

#### Rate Limit Blocking

**Before Each Request**:
```go
func (c *Client) waitForRateLimit(ctx context.Context) error {
    c.rateLimitMu.RLock()
    remaining := c.rateLimitRemaining
    resetTime := c.rateLimitReset
    c.rateLimitMu.RUnlock()

    // -1 = uninitialized (allow), > 0 = have requests (allow)
    if remaining < 0 || remaining > 0 || resetTime.IsZero() {
        return nil
    }

    // Rate limit exhausted (remaining == 0) - wait until reset
    waitDuration := time.Until(resetTime)
    if waitDuration <= 0 {
        return nil // Reset time already passed
    }

    log.Printf("GitHub API: Rate limit exhausted (0 requests remaining). Waiting %v until reset at %v",
        waitDuration.Round(time.Second), resetTime.Format("15:04:05"))

    select {
    case <-time.After(waitDuration):
        log.Printf("GitHub API: Rate limit reset, resuming requests")
        return nil
    case <-ctx.Done():
        return fmt.Errorf("context cancelled while waiting for rate limit reset: %w", ctx.Err())
    }
}
```

**After Each Response**:
```go
func (c *Client) updateRateLimit(headers http.Header) {
    limit := headers.Get("X-RateLimit-Limit")
    remaining := headers.Get("X-RateLimit-Remaining")
    reset := headers.Get("X-RateLimit-Reset")

    if remaining == "" || reset == "" {
        return
    }

    remainingInt, err := strconv.Atoi(remaining)
    if err != nil {
        return
    }

    resetUnix, err := strconv.ParseInt(reset, 10, 64)
    if err != nil {
        return
    }

    resetTime := time.Unix(resetUnix, 0)

    c.rateLimitMu.Lock()
    c.rateLimitRemaining = remainingInt
    c.rateLimitReset = resetTime
    c.rateLimitMu.Unlock()

    // Log warnings
    if limitInt, _ := strconv.Atoi(limit); limitInt > 0 {
        if remainingInt > 0 && remainingInt < limitInt/20 {
            // Below 5% remaining (but not 0)
            log.Printf("GitHub API: Rate limit warning - %d/%d requests remaining (resets at %v)",
                remainingInt, limitInt, resetTime.Format("15:04:05"))
        } else if remainingInt == 0 {
            // Exhausted
            log.Printf("GitHub API: Rate limit exhausted - further requests will block until %v",
                resetTime.Format("15:04:05"))
        }
    }
}
```

#### Request Wrapper

Wrap all API calls with rate limit checking:

```go
func (c *Client) doRequestWithRetry(ctx context.Context, url string, result interface{}) error {
    // Check and wait if rate limit exhausted
    if err := c.waitForRateLimit(ctx); err != nil {
        return err
    }

    return c.doRequest(ctx, url, result)
}
```

### GetProjects Implementation

**API Endpoint**: `GET /user/repos?per_page=100&page={page}`

```go
func (c *Client) GetProjects(ctx context.Context) ([]domain.Project, error) {
    var allProjects []domain.Project
    page := 1

    for {
        url := fmt.Sprintf("%s/user/repos?per_page=100&page=%d", c.BaseURL, page)

        var repos []githubRepository
        if err := c.doRequestWithRetry(ctx, url, &repos); err != nil {
            return nil, err
        }

        if len(repos) == 0 {
            break
        }

        for _, repo := range repos {
            allProjects = append(allProjects, c.convertRepository(repo))
        }

        page++
    }

    return allProjects, nil
}
```

**GitHub Response Structure**:
```json
{
  "id": 123456789,
  "full_name": "owner/repo",
  "name": "repo",
  "description": "Repository description",
  "html_url": "https://github.com/owner/repo",
  "default_branch": "main",
  "visibility": "private",
  "owner": {
    "avatar_url": "https://..."
  },
  "updated_at": "2026-01-26T12:00:00Z",
  "permissions": {
    "admin": true,
    "push": true,
    "pull": true
  }
}
```

**Conversion**:
```go
func (c *Client) convertRepository(repo githubRepository) domain.Project {
    return domain.Project{
        ID:            repo.FullName, // "owner/repo"
        Name:          repo.Name,
        Description:   repo.Description,
        WebURL:        repo.HTMLURL,
        DefaultBranch: repo.DefaultBranch,
        Platform:      domain.PlatformGitHub,
        Visibility:    repo.Visibility,
        AvatarURL:     repo.Owner.AvatarURL,
        LastActivity:  repo.UpdatedAt,
        Permissions: domain.Permissions{
            Admin: repo.Permissions.Admin,
            Push:  repo.Permissions.Push,
            Pull:  repo.Permissions.Pull,
        },
    }
}
```

### GetBranches Implementation

**API Endpoint**: `GET /repos/{owner}/{repo}/branches?per_page={limit}`

**IMPORTANT**: Only fetch commit details for the default branch to conserve rate limits.

```go
func (c *Client) GetBranches(ctx context.Context, projectID string, limit int) ([]domain.Branch, error) {
    if limit == 0 {
        limit = 50
    }

    // Get repository info to know default branch
    repoURL := fmt.Sprintf("%s/repos/%s", c.BaseURL, projectID)
    var repo githubRepository
    if err := c.doRequestWithRetry(ctx, repoURL, &repo); err != nil {
        return nil, err
    }
    defaultBranch := repo.DefaultBranch

    // Get branches
    url := fmt.Sprintf("%s/repos/%s/branches?per_page=%d", c.BaseURL, projectID, limit)
    var branches []githubBranch
    if err := c.doRequestWithRetry(ctx, url, &branches); err != nil {
        return nil, err
    }

    result := make([]domain.Branch, len(branches))
    for i, b := range branches {
        isDefault := (b.Name == defaultBranch)

        // Only fetch commit details for default branch to save rate limits
        if isDefault {
            commitURL := fmt.Sprintf("%s/repos/%s/commits/%s", c.BaseURL, projectID, b.Commit.SHA)
            var commitDetails githubCommit
            if err := c.doRequestWithRetry(ctx, commitURL, &commitDetails); err != nil {
                // If commit fetch fails, convert without details
                result[i] = c.convertBranch(b, projectID, nil, isDefault)
            } else {
                result[i] = c.convertBranch(b, projectID, &commitDetails, isDefault)
            }
        } else {
            // Non-default branch - no commit details
            result[i] = c.convertBranch(b, projectID, nil, isDefault)
        }
    }

    return result, nil
}
```

**GitHub Response Structure**:
```json
{
  "name": "main",
  "protected": true,
  "commit": {
    "sha": "abc123...",
    "commit": {
      "author": {
        "name": "John Doe",
        "date": "2026-01-26T12:00:00Z"
      },
      "message": "Commit message"
    }
  }
}
```

### GetLatestPipeline Implementation

GitHub Actions uses workflow runs.

**API Endpoint**: `GET /repos/{owner}/{repo}/actions/runs?branch={branch}&per_page=1`

```go
func (c *Client) GetLatestPipeline(ctx context.Context, projectID, branch string) (*domain.Pipeline, error) {
    url := fmt.Sprintf("%s/repos/%s/actions/runs?branch=%s&per_page=1",
        c.BaseURL, projectID, branch)

    var response struct {
        WorkflowRuns []githubWorkflowRun `json:"workflow_runs"`
    }
    if err := c.doRequestWithRetry(ctx, url, &response); err != nil {
        return nil, err
    }

    if len(response.WorkflowRuns) == 0 {
        return nil, nil
    }

    pipeline := c.convertWorkflowRun(response.WorkflowRuns[0], projectID)
    return &pipeline, nil
}
```

**GitHub Response Structure**:
```json
{
  "workflow_runs": [
    {
      "id": 123456789,
      "status": "completed",
      "conclusion": "success",
      "head_branch": "main",
      "head_sha": "abc123...",
      "html_url": "https://github.com/owner/repo/actions/runs/123456789",
      "created_at": "2026-01-26T12:00:00Z",
      "updated_at": "2026-01-26T12:05:00Z",
      "run_started_at": "2026-01-26T12:00:30Z"
    }
  ]
}
```

**Status Conversion**:
```go
func (c *Client) convertWorkflowStatus(status, conclusion string) domain.Status {
    switch status {
    case "queued", "waiting", "requested":
        return domain.StatusPending
    case "in_progress":
        return domain.StatusRunning
    case "completed":
        switch conclusion {
        case "success":
            return domain.StatusSuccess
        case "failure":
            return domain.StatusFailed
        case "cancelled":
            return domain.StatusCanceled
        case "skipped":
            return domain.StatusSkipped
        default:
            return domain.StatusFailed
        }
    default:
        return domain.StatusPending
    }
}
```

---

## Caching Strategy

### Overview
The system uses a **Stale-While-Revalidate** caching pattern:
- Serve cached data instantly (even if expired)
- Never block on API calls from request handlers
- Background refresher populates caches
- All cache reads are **cache-only** (no fallback to API)

### Location
`internal/api/stale_cache.go`

### Cache Structure

```go
type StaleCache struct {
    entries  map[string]*staleCacheEntry
    mu       sync.RWMutex
    ttl      time.Duration      // How long data is considered fresh
    staleTTL time.Duration      // How long to serve stale data
}

type staleCacheEntry struct {
    value      interface{}
    cachedAt   time.Time
    expiresAt  time.Time   // Fresh until this time
    staleUntil time.Time   // Usable until this time (much longer)
    projectID  string      // For priority refresh
    lastCommit time.Time   // For priority refresh
}
```

### Cache Lifecycle

```
Time:    0s          5m          29m         30m
         │           │           │           │
         ├───────────┼───────────┼───────────┤
         │  FRESH    │   STALE   │  EXPIRED  │
         ├───────────┼───────────┼───────────┤
cached   expiresAt            staleUntil
```

- **0-5m**: Fresh - serve from cache
- **5m-30m**: Stale but usable - serve from cache, mark for refresh
- **30m+**: Expired - don't serve, must refresh

### Cache Operations

#### Get (Read-Only)

```go
func (c *StaleCache) Get(key string) (interface{}, bool, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()

    entry, exists := c.entries[key]
    if !exists {
        return nil, false, false // (value, isFresh, exists)
    }

    now := time.Now()

    // Check if completely stale (unusable)
    if now.After(entry.staleUntil) {
        return nil, false, false
    }

    // Check if fresh
    isFresh := now.Before(entry.expiresAt)

    return entry.value, isFresh, true
}
```

#### Set (Write)

```go
func (c *StaleCache) Set(key string, value interface{}, projectID string, lastCommit time.Time) {
    c.mu.Lock()
    defer c.mu.Unlock()

    now := time.Now()
    c.entries[key] = &staleCacheEntry{
        value:      value,
        cachedAt:   now,
        expiresAt:  now.Add(c.ttl),        // 5 minutes
        staleUntil: now.Add(c.staleTTL),   // 24 hours
        projectID:  projectID,
        lastCommit: lastCommit,
    }
}
```

### StaleCachingClient Wrapper

Wraps any `Client` implementation with caching.

```go
type StaleCachingClient struct {
    client         Client
    extendedClient ExtendedClient
    userClient     UserClient
    eventsClient   EventsClient
    cache          *StaleCache
}

func NewStaleCachingClient(client Client, ttl, staleTTL time.Duration) *StaleCachingClient {
    // Type-assert for optional interfaces
    extendedClient, _ := client.(ExtendedClient)
    userClient, _ := client.(UserClient)
    eventsClient, _ := client.(EventsClient)

    return &StaleCachingClient{
        client:         client,
        extendedClient: extendedClient,
        userClient:     userClient,
        eventsClient:   eventsClient,
        cache:          NewStaleCache(ttl, staleTTL),
    }
}
```

### Cache-Only Methods

**CRITICAL**: All read methods are cache-only. On cache miss, they return empty/nil WITHOUT calling the underlying API client.

```go
// GetProjects - CACHE-ONLY
func (c *StaleCachingClient) GetProjects(ctx context.Context) ([]domain.Project, error) {
    key := "GetProjects"
    projects, found := getCached(c.cache, key, []domain.Project{})
    if found {
        return projects, nil
    }
    // Cache miss - return empty slice (background refresher will populate)
    return []domain.Project{}, nil
}

// GetLatestPipeline - CACHE-ONLY
func (c *StaleCachingClient) GetLatestPipeline(ctx context.Context, projectID, branch string) (*domain.Pipeline, error) {
    key := fmt.Sprintf("GetLatestPipeline:%s:%s", projectID, branch)
    pipeline, found := getCached(c.cache, key, (*domain.Pipeline)(nil))
    if found {
        return pipeline, nil
    }
    // Cache miss - return nil (background refresher will populate)
    return nil, nil
}

// GetBranches - CACHE-ONLY
func (c *StaleCachingClient) GetBranches(ctx context.Context, projectID string, limit int) ([]domain.Branch, error) {
    key := fmt.Sprintf("GetBranches:%s:%d", projectID, limit)
    branches, found := getCached(c.cache, key, []domain.Branch{})
    if found {
        return branches, nil
    }
    // Cache miss - return empty slice (background refresher will populate)
    return []domain.Branch{}, nil
}

// GetBranch - CACHE-ONLY
func (c *StaleCachingClient) GetBranch(ctx context.Context, projectID, branchName string) (*domain.Branch, error) {
    key := fmt.Sprintf("GetBranch:%s:%s", projectID, branchName)
    branch, found := getCached(c.cache, key, &domain.Branch{})
    if found {
        return branch, nil
    }
    // Cache miss - return nil (background refresher will populate)
    return nil, nil
}

// GetMergeRequests - CACHE-ONLY
func (c *StaleCachingClient) GetMergeRequests(ctx context.Context, projectID string) ([]domain.MergeRequest, error) {
    if c.extendedClient == nil {
        return []domain.MergeRequest{}, nil
    }

    key := fmt.Sprintf("GetMergeRequests:%s", projectID)
    mrs, found := getCached(c.cache, key, []domain.MergeRequest{})
    if found {
        return mrs, nil
    }
    // Cache miss - return empty slice (background refresher will populate)
    return []domain.MergeRequest{}, nil
}

// GetIssues - CACHE-ONLY
func (c *StaleCachingClient) GetIssues(ctx context.Context, projectID string) ([]domain.Issue, error) {
    if c.extendedClient == nil {
        return []domain.Issue{}, nil
    }

    key := fmt.Sprintf("GetIssues:%s", projectID)
    issues, found := getCached(c.cache, key, []domain.Issue{})
    if found {
        return issues, nil
    }
    // Cache miss - return empty slice (background refresher will populate)
    return []domain.Issue{}, nil
}

// GetCurrentUser - CACHE-ONLY
func (c *StaleCachingClient) GetCurrentUser(ctx context.Context) (*domain.UserProfile, error) {
    if c.userClient == nil {
        return nil, nil
    }

    key := "GetCurrentUser"
    user, found := getCached(c.cache, key, (*domain.UserProfile)(nil))
    if found {
        return user, nil
    }
    // Cache miss - return nil (background refresher will populate)
    return nil, nil
}
```

### ForceRefresh (Write-Only)

Only `ForceRefresh` triggers API calls. Called exclusively by background refresher.

```go
func (c *StaleCachingClient) ForceRefresh(ctx context.Context, key string) error {
    parts := strings.Split(key, ":")
    if len(parts) == 0 {
        return fmt.Errorf("invalid cache key: %s", key)
    }

    method := parts[0]

    switch method {
    case "GetProjects":
        projects, err := c.client.GetProjects(ctx)
        if err != nil {
            return err
        }
        c.cache.Set(key, projects, "", time.Time{})

    case "GetLatestPipeline":
        if len(parts) != 3 {
            return fmt.Errorf("invalid key format: %s", key)
        }
        pipeline, err := c.client.GetLatestPipeline(ctx, parts[1], parts[2])
        if err != nil {
            return err
        }
        var lastCommit time.Time
        if pipeline != nil {
            lastCommit = pipeline.UpdatedAt
        }
        c.cache.Set(key, pipeline, parts[1], lastCommit)

    case "GetBranches":
        if len(parts) != 3 {
            return fmt.Errorf("invalid key format: %s", key)
        }
        limit, _ := strconv.Atoi(parts[2])
        branches, err := c.client.GetBranches(ctx, parts[1], limit)
        if err != nil {
            return err
        }
        var lastCommit time.Time
        if len(branches) > 0 {
            lastCommit = branches[0].LastCommitDate
        }
        c.cache.Set(key, branches, parts[1], lastCommit)

    // ... similar for other methods

    default:
        return fmt.Errorf("unknown method: %s", method)
    }

    return nil
}
```

### Cache Keys

Cache keys follow consistent patterns:

| Method | Cache Key Pattern | Example |
|--------|------------------|---------|
| GetProjects | `GetProjects` | `GetProjects` |
| GetProjectCount | `GetProjectCount` | `GetProjectCount` |
| GetLatestPipeline | `GetLatestPipeline:{projectID}:{branch}` | `GetLatestPipeline:123:main` |
| GetPipelines | `GetPipelines:{projectID}:{limit}` | `GetPipelines:123:50` |
| GetBranches | `GetBranches:{projectID}:{limit}` | `GetBranches:123:200` |
| GetBranch | `GetBranch:{projectID}:{branchName}` | `GetBranch:123:main` |
| GetMergeRequests | `GetMergeRequests:{projectID}` | `GetMergeRequests:123` |
| GetIssues | `GetIssues:{projectID}` | `GetIssues:123` |
| GetCurrentUser | `GetCurrentUser` | `GetCurrentUser` |

**IMPORTANT**: Cache keys must be consistent between reads and writes. The limit parameter affects the cache key, so:
- Background refresher must cache with same limits as readers
- Example: If `GetDefaultBranchForProject` reads with limit=200, background refresher must cache with limit=200

---

## Service Layer

### Location
`internal/service/pipeline_service.go`

### PipelineService Structure

```go
type PipelineService struct {
    clients         map[string]api.Client // platform name -> client
    gitlabWhitelist []string              // allowed GitLab project IDs (nil = all)
    githubWhitelist []string              // allowed GitHub project IDs (nil = all)
    mu              sync.RWMutex
}

func NewPipelineService(gitlabWhitelist, githubWhitelist []string) *PipelineService {
    return &PipelineService{
        clients:         make(map[string]api.Client),
        gitlabWhitelist: gitlabWhitelist,
        githubWhitelist: githubWhitelist,
    }
}
```

### Registering Clients

```go
func (s *PipelineService) RegisterClient(platform string, client api.Client) {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.clients[platform] = client
}
```

### GetAllProjects

Fetches projects from all registered platforms concurrently.

```go
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

    // Collect errors
    var errs []error
    for err := range errChan {
        errs = append(errs, err)
    }
    if len(errs) > 0 {
        return nil, errs[0]
    }

    // Filter projects based on whitelist
    if len(s.gitlabWhitelist) > 0 || len(s.githubWhitelist) > 0 {
        filtered := make([]domain.Project, 0, len(allProjects))
        for _, project := range allProjects {
            if s.isProjectAllowed(project) {
                filtered = append(filtered, project)
            }
        }
        allProjects = filtered
    }

    return allProjects, nil
}

func (s *PipelineService) isProjectAllowed(project domain.Project) bool {
    switch project.Platform {
    case domain.PlatformGitLab:
        if len(s.gitlabWhitelist) == 0 {
            return true // No whitelist = allow all
        }
        for _, id := range s.gitlabWhitelist {
            if project.ID == id {
                return true
            }
        }
        return false

    case domain.PlatformGitHub:
        if len(s.githubWhitelist) == 0 {
            return true // No whitelist = allow all
        }
        for _, id := range s.githubWhitelist {
            if project.ID == id {
                return true
            }
        }
        return false

    default:
        return true // Unknown platforms allowed by default
    }
}
```

### GetDefaultBranchForProject

Gets default branch with commit details and latest pipeline.

```go
func (s *PipelineService) GetDefaultBranchForProject(ctx context.Context, project domain.Project) (*domain.Branch, *domain.Pipeline, int, error) {
    // Find client for this project's platform
    s.mu.RLock()
    client, exists := s.clients[project.Platform]
    s.mu.RUnlock()

    if !exists {
        return nil, nil, 0, fmt.Errorf("no client for platform %s", project.Platform)
    }

    // Get branches (cache-only, returns empty if cache miss)
    branches, err := client.GetBranches(ctx, project.ID, 200)
    if err != nil {
        return nil, nil, 0, err
    }

    branchCount := len(branches)

    // Fix repository names
    for i := range branches {
        if branches[i].Repository == project.ID {
            branches[i].Repository = project.Name
        }
    }

    // Find default branch - try IsDefault flag first
    var defaultBranch *domain.Branch
    for i := range branches {
        if branches[i].IsDefault {
            defaultBranch = &branches[i]
            break
        }
    }

    // If not found by flag, search by name from project metadata
    if defaultBranch == nil && project.DefaultBranch != "" {
        for i := range branches {
            if branches[i].Name == project.DefaultBranch {
                defaultBranch = &branches[i]
                break
            }
        }
    }

    // If still not found, fetch directly by name
    if defaultBranch == nil && project.DefaultBranch != "" {
        branch, err := client.GetBranch(ctx, project.ID, project.DefaultBranch)
        if err == nil && branch != nil {
            // Fix repository name
            if branch.Repository == project.ID {
                branch.Repository = project.Name
            }
            defaultBranch = branch
        }
    }

    // Get latest pipeline for default branch
    var pipeline *domain.Pipeline
    if defaultBranch != nil {
        pipeline, _ = client.GetLatestPipeline(ctx, project.ID, defaultBranch.Name)
        if pipeline != nil && (pipeline.Repository == "" || pipeline.Repository == project.ID) {
            pipeline.Repository = project.Name
        }
    }

    return defaultBranch, pipeline, branchCount, nil
}
```

### GetMergeRequestsForProject

Gets merge requests for a specific project.

```go
func (s *PipelineService) GetMergeRequestsForProject(ctx context.Context, project domain.Project) ([]domain.MergeRequest, error) {
    s.mu.RLock()
    client, exists := s.clients[project.Platform]
    s.mu.RUnlock()

    if !exists {
        return []domain.MergeRequest{}, nil
    }

    // Check if client supports ExtendedClient interface
    extendedClient, ok := client.(api.ExtendedClient)
    if !ok {
        return []domain.MergeRequest{}, nil
    }

    mrs, err := extendedClient.GetMergeRequests(ctx, project.ID)
    if err != nil {
        return []domain.MergeRequest{}, err
    }

    // Fix repository names
    for i := range mrs {
        if mrs[i].Repository == "" || mrs[i].Repository == mrs[i].Title {
            mrs[i].Repository = project.Name
        }
    }

    return mrs, nil
}
```

### GetUserProfiles

Gets user profiles from all platforms.

```go
func (s *PipelineService) GetUserProfiles(ctx context.Context) ([]domain.UserProfile, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()

    var profiles []domain.UserProfile
    var mu sync.Mutex
    var wg sync.WaitGroup

    for platform, c := range s.clients {
        // Check if client supports UserClient interface
        userClient, ok := c.(api.UserClient)
        if !ok {
            continue
        }

        wg.Add(1)
        go func(p string, client api.UserClient) {
            defer wg.Done()

            profile, err := client.GetCurrentUser(ctx)
            if err != nil || profile == nil {
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
```

---

## Background Refresh System

### Location
`internal/service/background_refresher.go`

### Structure

```go
type BackgroundRefresher struct {
    pipelineService *PipelineService
    refreshInterval time.Duration
    logger          Logger
    stopChan        chan struct{}
    wg              sync.WaitGroup
    mu              sync.Mutex
    running         bool
}

func NewBackgroundRefresher(pipelineService *PipelineService, refreshInterval time.Duration, logger Logger) *BackgroundRefresher {
    return &BackgroundRefresher{
        pipelineService: pipelineService,
        refreshInterval: refreshInterval,
        logger:          logger,
        stopChan:        make(chan struct{}),
    }
}
```

### Start and Stop

```go
func (r *BackgroundRefresher) Start() {
    r.mu.Lock()
    if r.running {
        r.mu.Unlock()
        return
    }
    r.running = true
    r.mu.Unlock()

    r.logger.Printf("Background refresher: Starting with %v interval", r.refreshInterval)

    r.wg.Add(1)
    go r.refreshLoop()
}

func (r *BackgroundRefresher) Stop() {
    r.mu.Lock()
    if !r.running {
        r.mu.Unlock()
        return
    }
    r.running = false
    r.mu.Unlock()

    r.logger.Printf("Background refresher: Stopping...")
    close(r.stopChan)
    r.wg.Wait()
    r.logger.Printf("Background refresher: Stopped")
}
```

### Refresh Loop

```go
func (r *BackgroundRefresher) refreshLoop() {
    defer r.wg.Done()

    // Perform initial fetch immediately (non-blocking)
    r.logger.Printf("Background refresher: Performing initial background fetch...")
    r.refreshData()

    // Setup periodic refresh ticker
    ticker := time.NewTicker(r.refreshInterval)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            r.logger.Printf("Background refresher: Performing periodic refresh...")
            r.refreshData()
        case <-r.stopChan:
            return
        }
    }
}
```

### Refresh Data

```go
const RefreshOperationTimeout = 10 * time.Minute

func (r *BackgroundRefresher) refreshData() {
    ctx, cancel := context.WithTimeout(context.Background(), RefreshOperationTimeout)
    defer cancel()

    startTime := time.Now()

    // Force refresh all client caches
    if err := r.pipelineService.ForceRefreshAllCaches(ctx); err != nil {
        r.logger.Printf("Background refresher: Failed to force-refresh caches: %v", err)
        return
    }

    // Fetch all data to verify caches populated
    projects, _ := r.pipelineService.GetAllProjects(ctx)
    projectCount := len(projects)

    repositories, _ := r.pipelineService.GetRepositoriesWithRecentRuns(ctx, 200)
    var pipelineCount int
    for _, repo := range repositories {
        pipelineCount += len(repo.Runs)
    }

    branches, _ := r.pipelineService.GetAllBranches(ctx, 200)
    branchCount := len(branches)

    mrs, _ := r.pipelineService.GetAllMergeRequests(ctx)
    mrCount := len(mrs)

    issues, _ := r.pipelineService.GetAllIssues(ctx)
    issueCount := len(issues)

    profiles, _ := r.pipelineService.GetUserProfiles(ctx)
    profileCount := len(profiles)

    duration := time.Since(startTime)
    r.logger.Printf("Background refresher: Completed in %v (projects: %d, pipelines: %d, branches: %d, MRs: %d, issues: %d, profiles: %d)",
        duration, projectCount, pipelineCount, branchCount, mrCount, issueCount, profileCount)
}
```

### ForceRefreshAllCaches

Called by background refresher to populate all caches.

```go
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
    successCount := len(s.clients)
    for err := range errChan {
        errs = append(errs, err)
        successCount--
    }

    // Only fail if ALL platforms failed
    if len(errs) > 0 && successCount == 0 {
        return fmt.Errorf("all platforms failed: %v", errs)
    }

    return nil
}
```

### Force Refresh Page-by-Page

Fetches projects page by page and caches incrementally.

```go
func (s *PipelineService) forceRefreshClientPageByPage(ctx context.Context, platform string, client api.Client) error {
    // Type-assert for required methods
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
        // Check context cancellation
        select {
        case <-ctx.Done():
            return fmt.Errorf("refresh cancelled: %w", ctx.Err())
        default:
        }

        // Fetch one page
        projects, hasNext, err := cacher.GetProjectsPage(ctx, page)
        if err != nil {
            return fmt.Errorf("failed to fetch page %d: %w", page, err)
        }

        if len(projects) == 0 {
            break
        }

        // Process each project individually
        for _, project := range projects {
            allProjects = append(allProjects, project)

            // Cache immediately (1, 2, 3... N)
            cacher.PopulateProjects(allProjects)

            // Fetch data for this project
            if err := s.forceRefreshDataForProjects(ctx, platform, cacher, []domain.Project{project}); err != nil {
                log.Printf("[%s] Warning: failed to fetch data for project %s: %v", platform, project.Name, err)
            }
        }

        if !hasNext {
            break
        }

        page++
    }

    return nil
}
```

### Force Refresh Data for Projects

Fetches all related data for a batch of projects.

```go
func (s *PipelineService) forceRefreshDataForProjects(ctx context.Context, platform string, client interface{ ForceRefresh(context.Context, string) error }, projects []domain.Project) error {
    var wg sync.WaitGroup

    for _, project := range projects {
        projectID := project.ID
        projectName := project.Name
        defaultBranch := project.DefaultBranch
        if defaultBranch == "" {
            defaultBranch = "main"
        }

        // Fetch default branch pipeline
        wg.Add(1)
        go func(pid, pname, branch string) {
            defer wg.Done()
            key := fmt.Sprintf("GetLatestPipeline:%s:%s", pid, branch)
            if err := client.ForceRefresh(ctx, key); err != nil {
                // Try master if main fails
                if branch == "main" {
                    key = fmt.Sprintf("GetLatestPipeline:%s:master", pid)
                    client.ForceRefresh(ctx, key)
                }
            }
        }(projectID, projectName, defaultBranch)

        // Fetch branches (MUST match limit used by GetDefaultBranchForProject)
        wg.Add(1)
        go func(pid string) {
            defer wg.Done()
            key := fmt.Sprintf("GetBranches:%s:200", pid)
            client.ForceRefresh(ctx, key)
        }(projectID)

        // Fetch pipelines
        wg.Add(1)
        go func(pid string) {
            defer wg.Done()
            key := fmt.Sprintf("GetPipelines:%s:50", pid)
            client.ForceRefresh(ctx, key)
        }(projectID)

        // Fetch merge requests
        wg.Add(1)
        go func(pid string) {
            defer wg.Done()
            key := fmt.Sprintf("GetMergeRequests:%s", pid)
            client.ForceRefresh(ctx, key)
        }(projectID)

        // Fetch issues
        wg.Add(1)
        go func(pid string) {
            defer wg.Done()
            key := fmt.Sprintf("GetIssues:%s", pid)
            client.ForceRefresh(ctx, key)
        }(projectID)
    }

    wg.Wait()
    return nil
}
```

**CRITICAL**: Cache keys must match read patterns:
- `GetBranches:projectID:200` (not :50) because `GetDefaultBranchForProject` reads with limit 200

---

## HTTP Handler Layer

### Location
`internal/dashboard/handler.go`

### Handler Structure

```go
type Handler struct {
    renderer          Renderer
    logger            Logger
    pipelineService   PipelineService
    runsPerRepo       int
    recentLimit       int
    uiRefreshInterval int
    gitlabCurrentUser string
    githubCurrentUser string
    httpClient        *http.Client
    avatarCache       map[string]*avatarCacheEntry
    avatarCacheMu     sync.RWMutex
    stopAvatarCleanup chan struct{}
}

type Renderer interface {
    RenderHealth(w io.Writer) error
    RenderRepositoriesSkeleton(w io.Writer, userProfiles []domain.UserProfile, refreshInterval int) error
    RenderRepositoryDetailSkeleton(w io.Writer, repositoryID string) error
    RenderRepositoryDetail(w io.Writer, repo RepositoryWithRuns, mrs []domain.MergeRequest, issues []domain.Issue) error
}

type Logger interface {
    Printf(format string, v ...interface{})
}

type PipelineService interface {
    GetAllProjects(ctx context.Context) ([]domain.Project, error)
    GetDefaultBranchForProject(ctx context.Context, project domain.Project) (*domain.Branch, *domain.Pipeline, int, error)
    GetMergeRequestsForProject(ctx context.Context, project domain.Project) ([]domain.MergeRequest, error)
    GetPipelinesForProject(ctx context.Context, projectID string, limit int) ([]domain.Pipeline, error)
    GetAllMergeRequests(ctx context.Context) ([]domain.MergeRequest, error)
    GetAllIssues(ctx context.Context) ([]domain.Issue, error)
    GetUserProfiles(ctx context.Context) ([]domain.UserProfile, error)
}
```

### Constructor

```go
type HandlerConfig struct {
    Renderer          Renderer
    Logger            Logger
    PipelineService   PipelineService
    RunsPerRepo       int
    RecentLimit       int
    UIRefreshInterval int
    GitLabUser        string
    GitHubUser        string
}

func NewHandler(cfg HandlerConfig) *Handler {
    h := &Handler{
        renderer:          cfg.Renderer,
        logger:            cfg.Logger,
        pipelineService:   cfg.PipelineService,
        runsPerRepo:       cfg.RunsPerRepo,
        recentLimit:       cfg.RecentLimit,
        uiRefreshInterval: cfg.UIRefreshInterval,
        gitlabCurrentUser: cfg.GitLabUser,
        githubCurrentUser: cfg.GitHubUser,
        httpClient: &http.Client{
            Timeout: 10 * time.Second,
        },
        avatarCache:       make(map[string]*avatarCacheEntry),
        stopAvatarCleanup: make(chan struct{}),
    }

    go h.cleanupAvatarCache(1 * time.Hour)

    return h
}
```

### Route Registration

```go
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
    mux.HandleFunc("/", h.handleRepositories)
    mux.HandleFunc("/api/health", h.handleHealth)
    mux.HandleFunc("/api/repositories", h.handleRepositoriesBulk)
    mux.HandleFunc("/api/repository-detail", h.handleRepositoryDetailAPI)
    mux.HandleFunc("/api/avatar/", h.handleAvatar)
    mux.HandleFunc("/repository", h.handleRepositoryDetail)
}
```

### handleRepositories

Serves the main page HTML skeleton (instant load).

```go
func (h *Handler) handleRepositories(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html")

    // Fetch user profiles (cache-only, returns empty if cache miss)
    userProfiles, err := h.pipelineService.GetUserProfiles(r.Context())
    if err != nil {
        h.logger.Printf("failed to get user profiles: %v", err)
        userProfiles = []domain.UserProfile{}
    }

    // Cache avatars (wait for completion to avoid 404s)
    var wg sync.WaitGroup
    for _, profile := range userProfiles {
        wg.Add(1)
        go func(p domain.UserProfile) {
            defer wg.Done()
            h.cacheAvatar(p.Platform, p.Username, p.Email, p.AvatarURL)
        }(profile)
    }
    wg.Wait()

    // Render empty skeleton with user profiles
    if err := h.renderer.RenderRepositoriesSkeleton(w, userProfiles, h.uiRefreshInterval); err != nil {
        h.logger.Printf("failed to render repositories skeleton: %v", err)
        http.Error(w, "Internal Server Error", http.StatusInternalServerError)
        return
    }
}
```

### handleRepositoriesBulk

Returns repositories as JSON (cache-only, no blocking).

```go
type RepositoryDefaultBranch struct {
    Project        domain.Project   `json:"Project"`
    DefaultBranch  *domain.Branch   `json:"DefaultBranch"`
    Pipeline       *domain.Pipeline `json:"Pipeline"`
    BranchCount    int              `json:"BranchCount"`
    OpenMRCount    int              `json:"OpenMRCount"`
    DraftMRCount   int              `json:"DraftMRCount"`
    ReviewingCount int              `json:"ReviewingCount"`
}

func (h *Handler) handleRepositoriesBulk(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    w.Header().Set("Cache-Control", "no-cache")

    // Parse pagination
    page := 1
    limit := 1000
    if pageParam := r.URL.Query().Get("page"); pageParam != "" {
        if p, err := strconv.Atoi(pageParam); err == nil && p > 0 {
            page = p
        }
    }
    if limitParam := r.URL.Query().Get("limit"); limitParam != "" {
        if l, err := strconv.Atoi(limitParam); err == nil && l > 0 && l <= 10000 {
            limit = l
        }
    }

    ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
    defer cancel()

    // Get all projects (cache-only)
    projects, err := h.pipelineService.GetAllProjects(ctx)
    if err != nil {
        h.logger.Printf("[BulkAPI] ERROR: %v", err)
        json.NewEncoder(w).Encode(map[string]interface{}{
            "repositories": []interface{}{},
            "pagination": map[string]interface{}{
                "page": 1, "limit": limit, "total": 0, "totalPages": 0, "hasNext": false,
            },
        })
        return
    }

    results := make([]RepositoryDefaultBranch, 0, len(projects))
    for _, project := range projects {
        // Get default branch + pipeline (cache-only)
        defaultBranch, pipeline, branchCount, err := h.pipelineService.GetDefaultBranchForProject(ctx, project)
        if err != nil {
            h.logger.Printf("Failed to get default branch for project %s: %v", project.Name, err)
        }

        // Get merge requests (cache-only)
        mrs, err := h.pipelineService.GetMergeRequestsForProject(ctx, project)
        if err != nil {
            h.logger.Printf("Failed to get merge requests for project %s: %v", project.Name, err)
        }

        // Count MR stats
        openMRCount := 0
        draftMRCount := 0
        reviewingCount := 0
        for _, mr := range mrs {
            if mr.State == "opened" {
                openMRCount++
                if mr.IsDraft {
                    draftMRCount++
                }
                for _, reviewer := range mr.Reviewers {
                    if reviewer == h.gitlabCurrentUser || reviewer == h.githubCurrentUser {
                        reviewingCount++
                        break
                    }
                }
            }
        }

        results = append(results, RepositoryDefaultBranch{
            Project:        project,
            DefaultBranch:  defaultBranch,
            Pipeline:       pipeline,
            BranchCount:    branchCount,
            OpenMRCount:    openMRCount,
            DraftMRCount:   draftMRCount,
            ReviewingCount: reviewingCount,
        })
    }

    // Paginate
    startIndex := (page - 1) * limit
    endIndex := startIndex + limit
    if startIndex >= len(results) {
        startIndex = len(results)
    }
    if endIndex > len(results) {
        endIndex = len(results)
    }

    paginatedResults := results[startIndex:endIndex]
    totalCount := len(results)
    totalPages := (totalCount + limit - 1) / limit
    hasNext := page < totalPages

    response := map[string]interface{}{
        "repositories": paginatedResults,
        "pagination": map[string]interface{}{
            "page":       page,
            "limit":      limit,
            "total":      totalCount,
            "totalPages": totalPages,
            "hasNext":    hasNext,
        },
    }

    json.NewEncoder(w).Encode(response)
}
```

### handleHealth

Health check endpoint.

```go
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")

    if err := h.renderer.RenderHealth(w); err != nil {
        h.logger.Printf("failed to render health: %v", err)
        http.Error(w, "Internal Server Error", http.StatusInternalServerError)
        return
    }
}
```

---

## Frontend Implementation

### Location
`web/templates/repositories_table.tmpl`

### HTML Structure

```html
<!DOCTYPE html>
<html>
<head>
    <title>CI/CD Dashboard</title>
    <link rel="stylesheet" href="/static/style.css">
</head>
<body>
    <h1>Repositories</h1>

    <div id="loading">Loading repositories...</div>

    <table id="repositories-table" style="display:none;">
        <thead>
            <tr>
                <th>Repository</th>
                <th>Role</th>
                <th>Default Branch</th>
                <th>Pipeline Status</th>
                <th>Last Commit</th>
                <th>Last Commit Author</th>
                <th>Branches</th>
                <th>Open MRs</th>
            </tr>
        </thead>
        <tbody id="repositories-tbody">
            <!-- Populated by JavaScript -->
        </tbody>
    </table>

    <script>
        const REFRESH_INTERVAL_SECONDS = {{.RefreshInterval}};

        // Fetch repositories
        async function fetchRepositories() {
            try {
                const response = await fetch('/api/repositories?limit=1000');
                const data = await response.json();

                if (data.repositories && data.repositories.length > 0) {
                    document.getElementById('loading').style.display = 'none';
                    document.getElementById('repositories-table').style.display = 'table';
                    renderRepositories(data.repositories);
                }
            } catch (error) {
                console.error('Failed to fetch repositories:', error);
            }
        }

        // Render repositories
        function renderRepositories(repositories) {
            const tbody = document.getElementById('repositories-tbody');
            tbody.innerHTML = '';

            repositories.forEach(repo => {
                const row = document.createElement('tr');

                // Repository name
                const nameCell = document.createElement('td');
                const link = document.createElement('a');
                link.href = `/repository?id=${encodeURIComponent(repo.Project.ID)}`;
                link.textContent = repo.Project.Name;
                nameCell.appendChild(link);
                row.appendChild(nameCell);

                // Role
                const roleCell = document.createElement('td');
                roleCell.textContent = getRoleName(repo.Project.Permissions);
                row.appendChild(roleCell);

                // Default Branch
                const branchCell = document.createElement('td');
                if (repo.DefaultBranch) {
                    branchCell.textContent = repo.DefaultBranch.Name;
                } else {
                    branchCell.textContent = '-';
                }
                row.appendChild(branchCell);

                // Pipeline Status
                const statusCell = document.createElement('td');
                if (repo.Pipeline) {
                    statusCell.className = `status-${repo.Pipeline.Status}`;
                    statusCell.textContent = repo.Pipeline.Status;
                } else {
                    statusCell.textContent = '-';
                }
                row.appendChild(statusCell);

                // Last Commit Date
                const commitDateCell = document.createElement('td');
                if (repo.DefaultBranch && repo.DefaultBranch.LastCommitDate) {
                    const date = new Date(repo.DefaultBranch.LastCommitDate);
                    commitDateCell.textContent = formatDate(date);
                } else {
                    commitDateCell.textContent = '-';
                }
                row.appendChild(commitDateCell);

                // Last Commit Author
                const authorCell = document.createElement('td');
                if (repo.DefaultBranch && repo.DefaultBranch.CommitAuthor) {
                    authorCell.textContent = repo.DefaultBranch.CommitAuthor;
                } else {
                    authorCell.textContent = '-';
                }
                row.appendChild(authorCell);

                // Branch Count
                const branchCountCell = document.createElement('td');
                branchCountCell.textContent = repo.BranchCount || '0';
                row.appendChild(branchCountCell);

                // MR Count
                const mrCell = document.createElement('td');
                const mrText = [];
                if (repo.OpenMRCount > 0) {
                    mrText.push(`${repo.OpenMRCount} open`);
                    if (repo.DraftMRCount > 0) {
                        mrText.push(`${repo.DraftMRCount} draft`);
                    }
                    if (repo.ReviewingCount > 0) {
                        mrText.push(`${repo.ReviewingCount} reviewing`);
                    }
                }
                mrCell.textContent = mrText.length > 0 ? mrText.join(', ') : '-';
                row.appendChild(mrCell);

                tbody.appendChild(row);
            });
        }

        // Get role name from permissions
        function getRoleName(permissions) {
            if (!permissions) return '-';

            const accessLevel = permissions.AccessLevel || 0;

            if (accessLevel >= 50 || permissions.Admin) {
                return 'Owner';
            } else if (accessLevel >= 40 || (permissions.Push && permissions.Admin === false)) {
                return 'Maintainer';
            } else if (accessLevel >= 30 || permissions.Push) {
                return 'Developer';
            } else if (accessLevel >= 20) {
                return 'Reporter';
            } else if (accessLevel >= 10 || permissions.Pull) {
                return 'Guest';
            }
            return '-';
        }

        // Format date
        function formatDate(date) {
            const now = new Date();
            const diffMs = now - date;
            const diffMins = Math.floor(diffMs / 60000);
            const diffHours = Math.floor(diffMs / 3600000);
            const diffDays = Math.floor(diffMs / 86400000);

            if (diffMins < 1) return 'just now';
            if (diffMins < 60) return `${diffMins}m ago`;
            if (diffHours < 24) return `${diffHours}h ago`;
            if (diffDays < 7) return `${diffDays}d ago`;

            return date.toLocaleDateString();
        }

        // Initial fetch
        fetchRepositories();

        // Poll every N seconds
        setInterval(fetchRepositories, REFRESH_INTERVAL_SECONDS * 1000);
    </script>
</body>
</html>
```

---

## Configuration

### Location
`internal/config/config.go`

### Structure

```go
package config

import (
    "os"
    "strconv"
    "strings"
)

type Config struct {
    // Server
    Port int

    // GitLab
    GitLabURL   string
    GitLabToken string

    // GitHub
    GitHubURL   string
    GitHubToken string

    // Optional: Specific repos to watch
    WatchedRepos []string

    // Cache intervals
    CacheRefreshInterval int // Seconds (default: 300 = 5 minutes)
    UIRefreshInterval    int // Seconds (default: 5)
}

func Load() (*Config, error) {
    cfg := &Config{
        Port:                 8080,
        GitLabURL:            "https://gitlab.com",
        GitHubURL:            "https://api.github.com",
        CacheRefreshInterval: 300,
        UIRefreshInterval:    5,
    }

    // Port
    if port := os.Getenv("PORT"); port != "" {
        if p, err := strconv.Atoi(port); err == nil {
            cfg.Port = p
        }
    }

    // GitLab
    if url := os.Getenv("GITLAB_URL"); url != "" {
        cfg.GitLabURL = url
    }
    cfg.GitLabToken = os.Getenv("GITLAB_TOKEN")

    // GitHub
    if url := os.Getenv("GITHUB_URL"); url != "" {
        cfg.GitHubURL = url
    }
    cfg.GitHubToken = os.Getenv("GITHUB_TOKEN")

    // Watched repos
    if watched := os.Getenv("WATCHED_REPOS"); watched != "" {
        cfg.WatchedRepos = strings.Split(watched, ",")
        for i := range cfg.WatchedRepos {
            cfg.WatchedRepos[i] = strings.TrimSpace(cfg.WatchedRepos[i])
        }
    }

    // Cache refresh interval
    if interval := os.Getenv("CACHE_REFRESH_INTERVAL"); interval != "" {
        if i, err := strconv.Atoi(interval); err == nil && i > 0 {
            cfg.CacheRefreshInterval = i
        }
    }

    // UI refresh interval
    if interval := os.Getenv("UI_REFRESH_INTERVAL"); interval != "" {
        if i, err := strconv.Atoi(interval); err == nil && i > 0 {
            cfg.UIRefreshInterval = i
        }
    }

    return cfg, nil
}

func (c *Config) HasGitLabConfig() bool {
    return c.GitLabToken != ""
}

func (c *Config) HasGitHubConfig() bool {
    return c.GitHubToken != ""
}
```

### Environment Variables

```bash
# Server
PORT=8080

# GitLab (read-only access)
GITLAB_URL=https://gitlab.com
GITLAB_TOKEN=glpat-xxxxxxxxxxxx  # read_api scope ONLY

# GitHub (read-only access)
GITHUB_URL=https://api.github.com
GITHUB_TOKEN=ghp_xxxxxxxxxxxx   # public_repo or repo scope

# Optional: Filter specific repos
WATCHED_REPOS=123,456            # GitLab project IDs
WATCHED_REPOS=owner/repo1,owner/repo2  # GitHub repos

# Cache intervals
CACHE_REFRESH_INTERVAL=300  # Background refresh every 5 minutes
UI_REFRESH_INTERVAL=5       # Frontend polls every 5 seconds
```

---

## Rate Limiting

### GitHub Rate Limiting

GitHub enforces 5000 requests/hour. This implementation handles rate limiting properly.

#### Initialization

```go
type Client struct {
    *api.BaseClient
    rateLimitMu        sync.RWMutex
    rateLimitRemaining int          // -1 = unknown, 0 = exhausted, >0 = available
    rateLimitReset     time.Time
}

func NewClient(...) *Client {
    return &Client{
        ...
        rateLimitRemaining: -1,  // -1 means "not yet known"
    }
}
```

#### Before Each Request

```go
func (c *Client) waitForRateLimit(ctx context.Context) error {
    c.rateLimitMu.RLock()
    remaining := c.rateLimitRemaining
    resetTime := c.rateLimitReset
    c.rateLimitMu.RUnlock()

    // -1 = uninitialized (allow), > 0 = have requests (allow)
    if remaining < 0 || remaining > 0 || resetTime.IsZero() {
        return nil
    }

    // Rate limit exhausted (remaining == 0) - wait
    waitDuration := time.Until(resetTime)
    if waitDuration <= 0 {
        return nil // Reset time passed
    }

    log.Printf("GitHub API: Rate limit exhausted. Waiting %v until reset at %v",
        waitDuration.Round(time.Second), resetTime.Format("15:04:05"))

    select {
    case <-time.After(waitDuration):
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

#### After Each Response

```go
func (c *Client) updateRateLimit(headers http.Header) {
    remaining := headers.Get("X-RateLimit-Remaining")
    reset := headers.Get("X-RateLimit-Reset")

    if remaining == "" || reset == "" {
        return
    }

    remainingInt, _ := strconv.Atoi(remaining)
    resetUnix, _ := strconv.ParseInt(reset, 10, 64)
    resetTime := time.Unix(resetUnix, 0)

    c.rateLimitMu.Lock()
    c.rateLimitRemaining = remainingInt
    c.rateLimitReset = resetTime
    c.rateLimitMu.Unlock()

    // Log warning at 5% remaining
    if limitStr := headers.Get("X-RateLimit-Limit"); limitStr != "" {
        limit, _ := strconv.Atoi(limitStr)
        if remainingInt > 0 && remainingInt < limit/20 {
            log.Printf("GitHub API: Rate limit warning - %d/%d requests remaining",
                remainingInt, limit)
        } else if remainingInt == 0 {
            log.Printf("GitHub API: Rate limit exhausted - further requests will block until %v",
                resetTime.Format("15:04:05"))
        }
    }
}
```

#### Request Wrapper

```go
func (c *Client) doRequestWithRetry(ctx context.Context, url string, result interface{}) error {
    // Wait if rate limit exhausted
    if err := c.waitForRateLimit(ctx); err != nil {
        return err
    }

    return c.doRequest(ctx, url, result)
}
```

---

## Implementation Steps

### Phase 1: Domain Model & Interfaces

1. Create `internal/domain/` package
2. Define all domain types:
   - Project
   - Pipeline
   - Branch
   - MergeRequest
   - Issue
   - UserProfile
   - Status enum
   - Permissions

3. Create `internal/api/` package
4. Define Client interface
5. Define extended interfaces (ExtendedClient, UserClient, WorkflowClient, EventsClient)
6. Create ClientConfig struct

### Phase 2: GitLab Client

1. Create `internal/api/gitlab/` package
2. Implement Client struct with HTTPClient dependency
3. Implement GetProjects with pagination
4. Implement GetBranches
5. Implement GetBranch (single branch fetch)
6. Implement GetLatestPipeline
7. Implement GetPipelines
8. Implement GetMergeRequests (ExtendedClient)
9. Implement GetIssues (ExtendedClient)
10. Implement GetCurrentUser (UserClient)
11. Write unit tests with mock HTTP client

### Phase 3: GitHub Client

1. Create `internal/api/github/` package
2. Implement Client struct with rate limiting fields
3. Implement rate limiting logic:
   - waitForRateLimit()
   - updateRateLimit()
   - doRequestWithRetry()
4. Implement GetProjects (user/repos)
5. Implement GetBranches (with selective commit fetching)
6. Implement GetBranch
7. Implement GetLatestPipeline (workflow runs)
8. Implement GetPipelines
9. Implement GetMergeRequests (pull requests)
10. Implement GetIssues
11. Implement GetCurrentUser
12. Write unit tests

### Phase 4: Caching Layer

1. Create `internal/api/stale_cache.go`
2. Implement StaleCache:
   - staleCacheEntry struct
   - Get() method (read-only, respects staleTTL)
   - Set() method
   - Invalidate() methods
3. Implement StaleCachingClient wrapper:
   - Constructor with interface detection
   - Implement all Client interface methods (cache-only)
   - Implement ForceRefresh() for background writes
4. Implement cache key helpers
5. Write unit tests

### Phase 5: Service Layer

1. Create `internal/service/` package
2. Implement PipelineService:
   - RegisterClient()
   - GetAllProjects() with filtering
   - GetDefaultBranchForProject() with fallback logic
   - GetMergeRequestsForProject()
   - GetUserProfiles()
   - ForceRefreshAllCaches()
   - forceRefreshClientPageByPage()
   - forceRefreshDataForProjects()
3. Write unit tests with mock clients

### Phase 6: Background Refresh

1. Create `internal/service/background_refresher.go`
2. Implement BackgroundRefresher:
   - Start() / Stop()
   - refreshLoop()
   - refreshData()
3. Ensure immediate initial refresh (no delay)
4. Ensure periodic refresh every N minutes
5. Write unit tests

### Phase 7: Configuration

1. Create `internal/config/` package
2. Implement Config struct
3. Implement Load() from environment variables
4. Implement helper methods (HasGitLabConfig, etc.)
5. Write unit tests

### Phase 8: HTTP Handlers

1. Create `internal/dashboard/` package
2. Define Renderer, Logger, PipelineService interfaces
3. Implement Handler struct
4. Implement handleRepositories (serve HTML skeleton)
5. Implement handleRepositoriesBulk (JSON API)
6. Implement handleHealth
7. Implement handleRepositoryDetail
8. Implement avatar caching (optional)
9. Write unit tests with mock dependencies

### Phase 9: Renderer

1. Create `internal/dashboard/renderer.go`
2. Implement Renderer interface
3. Implement RenderRepositoriesSkeleton()
4. Implement RenderHealth()
5. Implement RenderRepositoryDetail()
6. Write unit tests

### Phase 10: Frontend

1. Create `web/templates/` directory
2. Create HTML templates:
   - repositories_table.tmpl
   - repository_detail.tmpl
3. Implement JavaScript:
   - fetchRepositories()
   - renderRepositories()
   - Polling logic (setInterval)
   - Status formatting
   - Date formatting
4. Create `web/static/` directory
5. Create CSS styles:
   - Table styles
   - Status colors
   - Loading indicators

### Phase 11: Composition Root

1. Create `cmd/ci-dashboard/main.go`
2. Implement main():
   - Load configuration
   - Create HTTP client
   - Create GitLab client if configured
   - Create GitHub client if configured
   - Wrap clients with StaleCachingClient
   - Create PipelineService
   - Register clients
   - Create BackgroundRefresher
   - Start background refresher
   - Create Renderer
   - Create Handler
   - Register routes
   - Start HTTP server
   - Handle graceful shutdown

### Phase 12: Testing & Validation

1. Run all unit tests
2. Test with empty cache (instant page load)
3. Test with populated cache
4. Test GitHub rate limiting
5. Test pagination
6. Test error handling
7. Test graceful shutdown
8. Load testing
9. Integration tests

### Phase 13: Documentation

1. Write README.md
2. Write API documentation
3. Write deployment guide
4. Write troubleshooting guide
5. Document environment variables
6. Document architecture decisions

---

## Testing Strategy

### Unit Tests

Test each component in isolation with mocks.

#### GitLab Client Tests

```go
func TestGitLabClient_GetProjects(t *testing.T) {
    // Arrange
    mockHTTP := &mockHTTPClient{
        responses: map[string]string{
            "/api/v4/projects?membership=true&per_page=100&page=1": `[
                {"id": 123, "name": "test-project", "default_branch": "main"}
            ]`,
        },
    }
    client := gitlab.NewClient(api.ClientConfig{
        BaseURL: "https://gitlab.com",
        Token:   "test-token",
    }, mockHTTP)

    // Act
    projects, err := client.GetProjects(context.Background())

    // Assert
    if err != nil {
        t.Fatalf("expected no error, got %v", err)
    }
    if len(projects) != 1 {
        t.Fatalf("expected 1 project, got %d", len(projects))
    }
    if projects[0].Name != "test-project" {
        t.Errorf("expected name 'test-project', got %q", projects[0].Name)
    }
}
```

#### Cache Tests

```go
func TestStaleCache_GetSet(t *testing.T) {
    // Arrange
    cache := api.NewStaleCache(5*time.Minute, 24*time.Hour)

    // Act
    cache.Set("key1", "value1", "", time.Time{})
    value, isFresh, exists := cache.Get("key1")

    // Assert
    if !exists {
        t.Fatal("expected key to exist")
    }
    if !isFresh {
        t.Error("expected value to be fresh")
    }
    if value != "value1" {
        t.Errorf("expected 'value1', got %v", value)
    }
}

func TestStaleCache_CacheMiss(t *testing.T) {
    // Arrange
    cache := api.NewStaleCache(5*time.Minute, 24*time.Hour)

    // Act
    _, _, exists := cache.Get("nonexistent")

    // Assert
    if exists {
        t.Error("expected key not to exist")
    }
}
```

#### Handler Tests

```go
func TestHandler_HandleRepositoriesBulk_EmptyCache(t *testing.T) {
    // Arrange
    mockRenderer := &mockRenderer{}
    mockLogger := &mockLogger{}
    mockService := &mockPipelineService{
        projects: []domain.Project{}, // Empty cache
    }
    handler := dashboard.NewHandler(dashboard.HandlerConfig{
        Renderer:        mockRenderer,
        Logger:          mockLogger,
        PipelineService: mockService,
    })

    req := httptest.NewRequest("GET", "/api/repositories", nil)
    w := httptest.NewRecorder()

    // Act
    handler.RegisterRoutes(http.NewServeMux())
    handler.ServeHTTP(w, req)

    // Assert
    if w.Code != http.StatusOK {
        t.Errorf("expected 200, got %d", w.Code)
    }

    var response map[string]interface{}
    json.Unmarshal(w.Body.Bytes(), &response)

    repos := response["repositories"].([]interface{})
    if len(repos) != 0 {
        t.Errorf("expected empty array, got %d items", len(repos))
    }
}
```

### Integration Tests

Test multiple components together.

```go
func TestIntegration_FullFlow(t *testing.T) {
    // 1. Setup
    cfg := &config.Config{
        Port:                 9999,
        GitLabToken:          "test-token",
        CacheRefreshInterval: 300,
    }

    // 2. Create components
    httpClient := &http.Client{}
    gitlabClient := gitlab.NewClient(api.ClientConfig{
        BaseURL: cfg.GitLabURL,
        Token:   cfg.GitLabToken,
    }, httpClient)
    cachedClient := api.NewStaleCachingClient(gitlabClient, 5*time.Minute, 24*time.Hour)

    service := service.NewPipelineService(nil, nil)
    service.RegisterClient("gitlab", cachedClient)

    // 3. Force refresh
    ctx := context.Background()
    err := service.ForceRefreshAllCaches(ctx)
    if err != nil {
        t.Fatalf("force refresh failed: %v", err)
    }

    // 4. Query
    projects, err := service.GetAllProjects(ctx)
    if err != nil {
        t.Fatalf("get projects failed: %v", err)
    }

    // 5. Verify
    if len(projects) == 0 {
        t.Error("expected at least one project")
    }
}
```

---

## Summary

This specification provides a complete, detailed blueprint for implementing the CI/CD Dashboard. Key characteristics:

### Architecture
- **Clean Architecture**: Layers separated by interfaces
- **SOLID Principles**: Applied throughout
- **Dependency Injection**: All dependencies injected
- **Interface Segregation**: Small, focused interfaces

### Performance
- **Instant Page Load**: Page renders immediately without API calls
- **Cache-Only Reads**: Never block on API calls from handlers
- **Background Refresh**: All API calls happen in background
- **Stale-While-Revalidate**: Serve cached data, refresh async

### Scalability
- **Multi-Platform**: Easily add new CI/CD platforms
- **Concurrent**: Heavy use of goroutines
- **Efficient Caching**: Reduces API calls dramatically
- **Rate Limit Aware**: Respects GitHub's rate limits

### Maintainability
- **Testable**: Interfaces enable easy mocking
- **Documented**: Clear comments and godoc
- **Separation of Concerns**: Each component has one job
- **Standard Library**: No external dependencies

The implementation follows Go best practices and creates a production-ready, scalable, and maintainable CI/CD monitoring dashboard.
