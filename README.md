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