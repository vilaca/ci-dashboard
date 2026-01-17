# CI Dashboard

A dashboard for monitoring CI/CD pipelines from GitLab and GitHub in one unified interface.

## Features

- 🔄 **Multi-Platform Support**: Monitor both GitLab and GitHub Actions pipelines
- 📊 **Real-time Status**: View latest pipeline statuses at a glance
- 🎨 **Clean UI**: Simple, intuitive web interface with dark mode toggle
- 🔌 **REST API**: JSON API for integration with other tools
- ⚡ **Fast**: Concurrent API calls for quick data retrieval
- 🔒 **Repository Whitelisting**: Restrict access to specific repositories for security
- ⚙️ **Flexible Configuration**: YAML file or environment variables (env vars take priority)
- 🎯 **GitHub Workflows**: Each GitHub Actions workflow displayed as a separate pipeline
- ⏱️ **Duration Tracking**: View pipeline execution time at a glance
- 📈 **Smart Sorting**: Repositories sorted by latest run time

## Quick Start

### Prerequisites

- Go 1.23 or later
- GitLab personal access token (optional)
- GitHub personal access token (optional)

### Creating Access Tokens

**⚠️ IMPORTANT: This dashboard only requires READ-ONLY access. Never grant write permissions.**

#### GitLab Personal Access Token (Read-Only)

1. Log in to your GitLab instance (e.g., https://gitlab.com)
2. Click on your avatar in the top-right corner
3. Select **Settings** → **Access Tokens**
4. Fill in the token details:
   - **Token name**: `ci-dashboard-readonly` (or any name you prefer)
   - **Expiration date**: Set according to your security policy (recommended: 90 days or less)
   - **Select scopes**: Check **ONLY** `read_api`
     - ✅ `read_api` - Read-only access to API
     - ❌ Do NOT select any other scopes (write, admin, etc.)
5. Click **Create personal access token**
6. Copy the token immediately (you won't be able to see it again)
7. The token will look like: `glpat-xxxxxxxxxxxxxxxxxxxx`

**Required Scope (Read-Only):**
- ✅ `read_api` - Grants read-only access to repositories and pipelines

**DO NOT grant:**
- ❌ `api` - Gives write access
- ❌ `write_repository` - Gives write access to repositories
- ❌ Any other scopes

#### GitHub Personal Access Token (Read-Only)

**⚠️ Note**: GitHub classic tokens don't offer true read-only access. We recommend using **Fine-grained tokens** for better security.

##### Option 1: Fine-grained Personal Access Tokens (RECOMMENDED - True Read-Only)

1. Log in to GitHub (https://github.com)
2. Click on your avatar in the top-right corner
3. Select **Settings** → **Developer settings** → **Personal access tokens** → **Fine-grained tokens**
4. Click **Generate new token**
5. Fill in the token details:
   - **Token name**: `ci-dashboard-readonly`
   - **Expiration**: 90 days or less (recommended)
   - **Repository access**: Choose based on your needs:
     - **Public Repositories (read-only)**: Select "Public Repositories (read-only)"
     - **All repositories** or **Only select repositories**: Choose specific repos
   - **Permissions**: Under "Repository permissions", set:
     - ✅ **Actions**: `Read-only` (to view workflow runs)
     - ✅ **Contents**: `Read-only` (to view repositories)
     - ✅ **Metadata**: `Read-only` (automatically selected)
     - ❌ All other permissions: `No access` (leave unselected)
6. Click **Generate token**
7. Copy the token immediately (you won't be able to see it again)
8. The token will look like: `github_pat_xxxxxxxxxxxxxxxxxxxx`

**Benefits of Fine-grained tokens:**
- ✅ True read-only access (no write permissions)
- ✅ Granular control over repositories
- ✅ More secure than classic tokens
- ✅ Better audit trail

##### Option 2: Classic Personal Access Tokens (Legacy - Not Truly Read-Only)

⚠️ **Warning**: Classic tokens with `repo` or `public_repo` scopes also grant write access. Use fine-grained tokens instead for true read-only access.

If you must use classic tokens:

1. Log in to GitHub (https://github.com)
2. Click on your avatar in the top-right corner
3. Select **Settings** → **Developer settings** → **Personal access tokens** → **Tokens (classic)**
4. Click **Generate new token** → **Generate new token (classic)**
5. Fill in the token details:
   - **Note**: `ci-dashboard-readonly` (though not truly read-only)
   - **Expiration**: Set according to your security policy (recommended: 90 days or less)
   - **Select scopes**: Choose based on your repository visibility:
     - **For public repositories only**: ✅ `public_repo` (⚠️ also grants write access)
     - **For private repositories**: ✅ `repo` (⚠️ also grants write access)
     - ❌ Do NOT select `workflow`, `write:packages`, `delete:packages`, or any admin scopes
6. Click **Generate token**
7. Copy the token immediately (you won't be able to see it again)
8. The token will look like: `ghp_xxxxxxxxxxxxxxxxxxxx`

**Classic Token Scopes:**
- ⚠️ `public_repo` - Access to public repositories (includes write)
- OR ⚠️ `repo` - Access to private repositories (includes write)

**DO NOT grant:**
- ❌ `workflow` - Workflow modification
- ❌ `write:packages` - Package write access
- ❌ `delete:packages` - Package deletion
- ❌ `admin:*` - Administrative access
- ❌ Any other scopes

**Security Notes:**
- ✅ This dashboard ONLY reads data - no write operations are performed
- ✅ Use the minimum required scopes (principle of least privilege)
- ✅ Store tokens securely (never commit them to version control)
- ✅ Use environment variables or secret management tools
- ✅ Set expiration dates (90 days or less recommended)
- ✅ Rotate tokens periodically
- ✅ Revoke tokens immediately if compromised
- ⚠️ Monitor token usage in platform audit logs

### Configuration

The dashboard supports two configuration methods: **YAML file** and **environment variables**.

**Configuration Priority Order:**
1. **Environment Variables** (highest priority)
2. **YAML Configuration File**
3. **Default Values** (lowest priority)

#### Option 1: YAML Configuration File (Recommended)

Create a `config.yaml` file in the application directory:

```yaml
# Copy from config.example.yaml and customize
port: 8080

gitlab:
  url: https://gitlab.com
  token: your-gitlab-token
  watched_repos:
    - "123"      # GitLab project IDs
    - "456"

github:
  url: https://api.github.com
  token: your-github-token
  watched_repos:
    - "owner/repo1"    # GitHub owner/repo format
    - "owner/repo2"

display:
  runs_per_repository: 3      # Number of recent runs per repo
  recent_pipelines_limit: 50  # Total pipelines on recent page
```

**YAML File Location:**
- Default: `./config.yaml` or `./config.yml`
- Custom: Set `CONFIG_FILE` environment variable

#### Option 2: Environment Variables

```bash
# Server Configuration
export PORT=8080                          # Default: 8080
export CONFIG_FILE="/path/to/config.yaml" # Optional: Custom config file

# GitLab Configuration (optional)
export GITLAB_URL="https://gitlab.com"           # Default: https://gitlab.com
export GITLAB_TOKEN="your-gitlab-token"
export GITLAB_WATCHED_REPOS="123,456"            # Comma-separated project IDs

# GitHub Configuration (optional)
export GITHUB_URL="https://api.github.com"       # Default: https://api.github.com
export GITHUB_TOKEN="your-github-token"
export GITHUB_WATCHED_REPOS="owner/repo1,owner/repo2"  # Comma-separated repos

# Display Configuration
export RUNS_PER_REPOSITORY=3              # Default: 3
export RECENT_PIPELINES_LIMIT=50          # Default: 50
```

#### Repository Whitelisting (Security Feature)

**Important:** For security and privacy, you can restrict which repositories the dashboard can access:

- **GitLab**: Use numeric project IDs (e.g., `123`, `456`)
- **GitHub**: Use `owner/repo` format (e.g., `facebook/react`, `golang/go`)

When whitelists are configured:
- ✅ **Only whitelisted repositories** will be monitored
- ❌ **All other repositories** will be ignored, even if the token has access
- 🔒 **No data** from non-whitelisted repos will be fetched or displayed

**Empty/Unset Whitelist:**
- If no whitelist is configured, **all accessible repositories** will be monitored

#### Configuration Examples

**Example 1: YAML with Whitelist (Recommended for Production)**
```yaml
# config.yaml
gitlab:
  token: glpat-xxxxxxxxxxxx
  watched_repos:
    - "123"  # Only monitor project 123

github:
  token: github_pat_xxxxxxxxxxxx
  watched_repos:
    - "myorg/app1"
    - "myorg/app2"
```

**Example 2: Mixed YAML + Environment Variables**
```yaml
# config.yaml - Base configuration
port: 8080
github:
  watched_repos:
    - "myorg/app1"
```

```bash
# Override token via environment variable (keeps it out of YAML file)
export GITHUB_TOKEN="github_pat_xxxxxxxxxxxx"
```

**Example 3: Environment Variables Only**
```bash
export GITHUB_TOKEN="github_pat_xxxxxxxxxxxx"
export GITHUB_WATCHED_REPOS="owner/repo1,owner/repo2"
export RUNS_PER_REPOSITORY=5
```

### Building

```bash
make build
```

### Running

```bash
# Set your tokens
export GITLAB_TOKEN="glpat-xxxxxxxxxxxx"
export GITHUB_TOKEN="ghp_xxxxxxxxxxxx"

# Start the server
make run
```

The server will start on port 8080 by default.

### Development with Hot-Reload

For development with automatic reload on code changes, use [Air](https://github.com/air-verse/air):

```bash
# Run with hot-reload (auto-installs Air if needed)
make dev

# Or manually install Air first
make install-air
air

# With environment variables
GITHUB_TOKEN="ghp_xxx" GITLAB_TOKEN="glpat_xxx" make dev
```

Air will:
- ✨ Automatically rebuild when you save Go files
- 🔄 Restart the server with the new build
- 📝 Show build errors in real-time
- 🚀 Significantly speed up development workflow

Configuration is already set up in `.air.toml`.

**Note**: If you want to run `air` directly (without `make dev`), ensure your Go bin directory is in your PATH:
```bash
# Add to your ~/.bashrc, ~/.zshrc, or ~/.profile
export PATH="$PATH:$(go env GOPATH)/bin"
```

### Testing

```bash
make test
```

## Project Structure

- `cmd/ci-dashboard/` - Main application entry point
- `internal/api/` - CI/CD platform API clients
- `internal/config/` - Configuration management
- `internal/dashboard/` - Dashboard HTTP handlers and logic
- `pkg/client/` - Public client libraries
- `web/` - Web assets (static files and templates)

## Usage

Once running, access the dashboard at:

- **Home (Repositories View)**: http://localhost:8080/
  - Repository cards sorted by latest activity
  - Last N runs per repository (configurable via `RUNS_PER_REPOSITORY`)
  - Each run shows: workflow name, time, duration, and status

- **Recent Pipelines View**: http://localhost:8080/pipelines
  - Table of most recent pipelines across all repositories
  - Sorted by update time (most recent first)
  - Limit configurable via `RECENT_PIPELINES_LIMIT`

- **Grouped Pipelines View**: http://localhost:8080/pipelines/grouped
  - Pipelines grouped by workflow name (GitHub) or repository (GitLab)

- **Workflow Runs**: http://localhost:8080/pipelines/workflow?project=owner/repo&workflow=123
  - View all runs for a specific GitHub Actions workflow

- **JSON API**: http://localhost:8080/api/pipelines
  - Programmatic access to pipeline data

The dashboard features:
- 🌓 **Dark mode toggle** in the top-right corner
- 🔄 **Auto-refresh** capability
- 📊 **Duration tracking** for all pipeline runs
- 🔗 **Direct links** to view full details on GitLab/GitHub

## API Endpoints

### Web Interface
- `GET /` - Repositories view (cards with recent runs, sorted by latest activity)
- `GET /repository` - Repository detail page (requires `?id=owner/repo` or `?id=123` query param)
  - Shows repository statistics (success rate, total runs, average duration)
  - Lists all recent runs for the repository
- `GET /pipelines` - Recent pipelines view (table of most recent pipelines)
- `GET /pipelines/grouped` - Grouped pipelines view (organized by workflow/repository)
- `GET /pipelines/workflow` - Workflow-specific runs (requires `?project=owner/repo&workflow=123` query params)

### REST API
- `GET /api/health` - Health check endpoint (returns `{"status":"ok"}`)
- `GET /api/pipelines` - Pipelines data (JSON format)

### API Response Example

```json
{
  "pipelines": [
    {
      "id": "123",
      "project_id": "456",
      "repository": "my-project",
      "branch": "main",
      "status": "success",
      "created_at": "2024-01-01T10:00:00Z",
      "updated_at": "2024-01-01T10:05:00Z",
      "duration": 300000000000,
      "web_url": "https://gitlab.com/user/my-project/-/pipelines/123",
      "workflow_name": "CI",
      "workflow_id": "12345"
    }
  ],
  "count": 1
}
```

**Note**:
- `duration` is in nanoseconds (300000000000 = 5 minutes)
- `workflow_name` and `workflow_id` are present for GitHub Actions, null for GitLab

## Design Principles

This project strictly follows industry-standard software engineering principles:

**Core Principles**: DRY, SOLID, KISS, SRP, POLA/POLS, SLAP, SoC, IoC, PIE, Law of Demeter, High Cohesion/Low Coupling

**Testing**: FIRST, AAA

See [DESIGN.md](DESIGN.md) for detailed explanations and examples.

Key architectural decisions:
- **Dependency Injection**: All dependencies injected via constructors
- **Interface-based Design**: Depend on abstractions, not concrete types
- **Separation of Concerns**: Clear boundaries between HTTP, rendering, and business logic
- **Law of Demeter**: Components only talk to immediate dependencies
- **High Cohesion / Low Coupling**: Focused components with minimal dependencies
- **Testability**: 100% testable with mock implementations

## Development

This project uses:
- Go 1.23+ with minimal external dependencies
  - `gopkg.in/yaml.v3` - YAML configuration file support
- Dependency injection for loose coupling
- Interface-based design for testability
- AAA testing pattern with FIRST principles