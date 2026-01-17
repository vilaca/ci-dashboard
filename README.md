# CI Dashboard

A dashboard for monitoring CI/CD pipelines from GitLab and GitHub in one unified interface.

## Features

- 🔄 **Multi-Platform Support**: Monitor both GitLab and GitHub Actions pipelines
- 📊 **Real-time Status**: View latest pipeline statuses at a glance
- 🎨 **Clean UI**: Simple, intuitive web interface
- 🔌 **REST API**: JSON API for integration with other tools
- ⚡ **Fast**: Concurrent API calls for quick data retrieval

## Quick Start

### Prerequisites

- Go 1.23 or later
- GitLab personal access token (optional)
- GitHub personal access token (optional)

### Creating Access Tokens

#### GitLab Personal Access Token

1. Log in to your GitLab instance (e.g., https://gitlab.com)
2. Click on your avatar in the top-right corner
3. Select **Settings** → **Access Tokens**
4. Fill in the token details:
   - **Token name**: `ci-dashboard` (or any name you prefer)
   - **Expiration date**: Set according to your security policy
   - **Select scopes**: Check `read_api` (read-only access to API)
5. Click **Create personal access token**
6. Copy the token immediately (you won't be able to see it again)
7. The token will look like: `glpat-xxxxxxxxxxxxxxxxxxxx`

**Required Scope:**
- `read_api` - Read-only access to repositories and pipelines

#### GitHub Personal Access Token

1. Log in to GitHub (https://github.com)
2. Click on your avatar in the top-right corner
3. Select **Settings** → **Developer settings** → **Personal access tokens** → **Tokens (classic)**
4. Click **Generate new token** → **Generate new token (classic)**
5. Fill in the token details:
   - **Note**: `ci-dashboard` (or any description)
   - **Expiration**: Set according to your security policy
   - **Select scopes**: Check the following:
     - `repo` - Full control of private repositories (or `public_repo` for public repos only)
     - `workflow` - Update GitHub Action workflows
6. Click **Generate token**
7. Copy the token immediately (you won't be able to see it again)
8. The token will look like: `ghp_xxxxxxxxxxxxxxxxxxxx`

**Required Scopes:**
- `repo` - Access to repositories and workflow runs
- `workflow` - Access to GitHub Actions

**Security Notes:**
- Store tokens securely (never commit them to version control)
- Use environment variables or secret management tools
- Set reasonable expiration dates
- Use minimum required scopes
- Rotate tokens periodically

### Configuration

Set environment variables for the platforms you want to monitor:

```bash
# GitLab (optional)
export GITLAB_URL="https://gitlab.com"  # Default: https://gitlab.com
export GITLAB_TOKEN="your-gitlab-token"

# GitHub (optional)
export GITHUB_URL="https://api.github.com"  # Default: https://api.github.com
export GITHUB_TOKEN="your-github-token"

# Server configuration
export PORT=8080  # Default: 8080
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

- **Home**: http://localhost:8080/
- **Pipelines View**: http://localhost:8080/pipelines
- **JSON API**: http://localhost:8080/api/pipelines

The dashboard will automatically fetch and display:
- Latest pipeline status for all accessible projects
- Pipeline branch information
- Links to view full details on GitLab/GitHub

## API Endpoints

### Web Interface
- `GET /` - Main dashboard landing page
- `GET /pipelines` - Pipelines status page (HTML)

### REST API
- `GET /api/health` - Health check endpoint
- `GET /api/pipelines` - Pipelines data (JSON)

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
      "web_url": "https://gitlab.com/user/my-project/-/pipelines/123"
    }
  ],
  "count": 1
}
```

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
- Go 1.23+ with standard library (no external dependencies)
- Dependency injection for loose coupling
- Interface-based design for testability
- AAA testing pattern with FIRST principles