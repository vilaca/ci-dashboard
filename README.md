# CI Dashboard

A unified dashboard for monitoring GitLab and GitHub CI/CD pipelines with real-time updates and auto-refresh.

## Features

- 🔄 Multi-platform support (GitLab + GitHub Actions)
- ⚡ Real-time auto-refresh (configurable interval, default 5s)
- 📊 Progressive loading with per-project incremental caching
- 👤 User profile avatars (GitLab + GitHub)
- 🎨 Dark mode toggle
- ⭐ Favorite repositories
- 🔀 Merge Requests/PRs with draft detection
- 🌿 Branch management with pipeline status
- 🐛 Issues tracking
- 🔒 Repository whitelisting for security
- ⚙️ YAML or environment variable configuration

## Quick Start

### Prerequisites

- Go 1.23+
- Access tokens (read-only)

### Access Tokens

**GitLab (Read-Only):**
1. Settings → Access Tokens
2. Scope: `read_api` only
3. Token format: `glpat-xxxxxxxxxxxx`

**GitHub (Read-Only):**

**Option 1: Fine-grained tokens (recommended)**
- Settings → Developer settings → Personal access tokens → Fine-grained tokens
- Permissions: `Actions: Read-only`, `Contents: Read-only`
- Token format: `github_pat_xxxxxxxxxxxx`

**Option 2: Classic tokens (legacy)**
- Scope: `public_repo` or `repo` (⚠️ includes write access)
- Token format: `ghp_xxxxxxxxxxxx`

### Configuration

Environment variables take priority over YAML configuration.

**Environment Variables:**
```bash
# Tokens
export GITLAB_TOKEN="glpat-xxxxxxxxxxxx"
export GITHUB_TOKEN="github_pat_xxxxxxxxxxxx"

# Optional
export PORT=8080
export GITLAB_URL="https://gitlab.com"
export GITHUB_URL="https://api.github.com"
export GITLAB_USER="your-username"          # For "Your Branches" filtering
export GITHUB_USER="your-username"
export GITLAB_WATCHED_REPOS="123,456"       # Whitelist (GitLab project IDs)
export GITHUB_WATCHED_REPOS="owner/repo1,owner/repo2"  # Whitelist
export RUNS_PER_REPOSITORY=3
export RECENT_PIPELINES_LIMIT=50
export UI_REFRESH_INTERVAL_SECONDS=5        # Auto-refresh interval
```

**YAML Configuration (config.yaml):**
```yaml
port: 8080

gitlab:
  url: https://gitlab.com
  token: glpat-xxxxxxxxxxxx
  user: your-username
  watched_repos:
    - "123"
    - "456"

github:
  url: https://api.github.com
  token: github_pat_xxxxxxxxxxxx
  user: your-username
  watched_repos:
    - "owner/repo1"
    - "owner/repo2"

display:
  runs_per_repository: 3
  recent_pipelines_limit: 50

ui:
  refresh_interval_seconds: 5
```

### Build & Run

```bash
# Build
make build

# Run
make run

# Development with hot-reload
make dev
```

## Usage

Access at http://localhost:8080

**Pages:**
- `/` - Repositories (sorted by recent activity, auto-refresh)
- `/repository?id=owner/repo` - Repository details with statistics
- `/pipelines` - Recent pipelines
- `/merge-requests` - Open MRs/PRs
- `/issues` - Open issues
- `/branches` - All branches with pipeline status
- `/your-branches` - Your branches only (requires GITLAB_USER/GITHUB_USER)

**API:**
- `/api/health` - Health check
- `/api/repositories` - Repository data (JSON)
- `/api/repository-detail?id=owner/repo` - Repository details (JSON)
- `/api/avatar/{platform}/{username}` - Cached avatars

## Architecture

**Core Principles:** DRY, SOLID, KISS, IoC, High Cohesion/Low Coupling

**Caching Strategy:**
- In-memory stale-while-revalidate cache
- Background refresh every 5 minutes
- Page-by-page progressive loading
- Per-project incremental caching (1, 2, 3... instead of waiting for 100)
- UI auto-refresh (default 5s, configurable via `UI_REFRESH_INTERVAL_SECONDS`)

**Project Structure:**
- `cmd/ci-dashboard/` - Entry point
- `internal/api/` - Platform API clients with stale caching
- `internal/config/` - Configuration management
- `internal/dashboard/` - HTTP handlers
- `internal/service/` - Business logic with background refresh
- `internal/domain/` - Domain models

**Dependencies:**
- Go stdlib only, except:
  - `gopkg.in/yaml.v3` - YAML config support

## Development

```bash
make test    # Run tests
make lint    # Run go vet
make fmt     # Format code
make dev     # Hot-reload with Air
```
