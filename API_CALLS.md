# API Calls Documentation

This document lists all API calls made to GitLab and GitHub, explaining their purpose and what data they retrieve.

## Overview

The CI Dashboard is a **READ-ONLY** monitoring tool. It never writes, modifies, or deletes data on GitLab or GitHub. All API operations are read-only GET requests.

---

## GitLab API Calls

All GitLab API calls use the base URL: `https://gitlab.com/api/v4` (or your configured GitLab instance)

Authentication: `PRIVATE-TOKEN` header with read-only token

### 1. Get Projects
**Endpoint:** `GET /api/v4/projects?membership=true&per_page=100&page=X`

**Purpose:** Fetch all projects the authenticated user has access to

**Why needed:**
- Main data source - lists all repositories to monitor
- Provides project ID, name, web URL, default branch, fork status

**Frequency:**
- On startup (cache miss)
- Every 10 minutes (background refresh if expired)
- On cache invalidation (event detected)

**Data returned:**
```json
{
  "id": 8222,
  "name": "rebot-data-analysis",
  "web_url": "https://gitlab.com/...",
  "default_branch": "main",
  "forked_from_project": {...}
}
```

---

### 2. Get Branches
**Endpoint:** `GET /api/v4/projects/{projectId}/repository/branches?per_page=50`

**Purpose:** List all branches in a repository with last commit info

**Why needed:**
- Show branch list on repository detail page
- Get default branch information for main dashboard
- Display last commit date, author, and message
- Power the "Your Branches" filter

**Frequency:**
- On-demand when viewing repository details
- When loading repositories table (default branch only)

**Data returned:**
```json
{
  "name": "main",
  "default": true,
  "protected": true,
  "commit": {
    "id": "abc123...",
    "message": "Fix bug",
    "committed_date": "2026-01-22T10:00:00Z",
    "author_name": "João Vilaca"
  }
}
```

---

### 3. Get Latest Pipeline
**Endpoint:** `GET /api/v4/projects/{projectId}/pipelines?ref={branch}&per_page=1`

**Purpose:** Get the most recent pipeline run for a specific branch

**Why needed:**
- Display pipeline status (success/failed/running) on main dashboard
- Show build status badges
- Provide pipeline duration and update time

**Frequency:**
- On-demand when loading repository data
- Cached for 4 hours (configurable)

**Data returned:**
```json
{
  "id": 12345,
  "status": "success",
  "ref": "main",
  "web_url": "https://gitlab.com/.../pipelines/12345",
  "created_at": "2026-01-22T10:00:00Z",
  "updated_at": "2026-01-22T10:05:00Z"
}
```

---

### 4. Get Merge Requests
**Endpoint:** `GET /api/v4/projects/{projectId}/merge_requests?state=opened&per_page=50`

**Purpose:** List all open merge requests in a repository

**Why needed:**
- Show MR/PR count on repositories table
- Display draft MR count
- **NEW:** Show which MRs you are reviewing (checks `reviewers` field)
- Power the "MRs/PRs" page

**Frequency:**
- On-demand when loading repository data
- Cached for 4 hours

**Data returned:**
```json
{
  "iid": 234,
  "title": "Add new feature",
  "state": "opened",
  "draft": false,
  "source_branch": "feature-branch",
  "target_branch": "main",
  "author": {"username": "john.doe"},
  "reviewers": [
    {"username": "joao.vilaca"},
    {"username": "jane.smith"}
  ],
  "created_at": "2026-01-22T10:00:00Z",
  "web_url": "https://gitlab.com/.../merge_requests/234"
}
```

---

### 5. Get Current User
**Endpoint:** `GET /api/v4/user`

**Purpose:** Get authenticated user's profile information

**Why needed:**
- Display user avatar in top-right corner
- Match reviewer username for "Reviewing" column
- Filter "Your Branches" page
- Show user's full name

**Frequency:**
- On startup (when loading repositories page)
- Cached for 4 hours

**Data returned:**
```json
{
  "id": 12345,
  "username": "joao.vilaca",
  "name": "João Vilaca",
  "email": "joao@example.com",
  "avatar_url": "https://secure.gravatar.com/...",
  "web_url": "https://gitlab.com/joao.vilaca"
}
```

---

### 6. Get Issues
**Endpoint:** `GET /api/v4/projects/{projectId}/issues?state=opened&per_page=50`

**Purpose:** List all open issues in a repository

**Why needed:**
- Power the "Issues" page
- Show issue counts (if implemented)

**Frequency:**
- On-demand when viewing issues page
- Cached for 4 hours

**Data returned:**
```json
{
  "iid": 42,
  "title": "Bug report",
  "state": "opened",
  "labels": ["bug", "high-priority"],
  "author": {"username": "john.doe"},
  "assignee": {"username": "jane.smith"},
  "created_at": "2026-01-22T10:00:00Z",
  "web_url": "https://gitlab.com/.../issues/42"
}
```

---

### 7. Get Events (for change detection)
**Endpoint:** `GET /api/v4/projects/{projectId}/events?per_page=100&sort=desc`

**Purpose:** Detect recent activity in repositories to invalidate cache

**Why needed:**
- Event polling - detect pushes, MR changes, issue updates
- Automatically refresh stale data when changes occur
- Keep dashboard up-to-date without manual refresh

**Frequency:**
- Every 5 minutes (background event poller)

**Data returned:**
```json
{
  "id": 98765,
  "project_id": 8222,
  "action_name": "pushed to",
  "target_type": null,
  "target_title": null,
  "created_at": "2026-01-22T10:00:00Z",
  "author_username": "joao.vilaca",
  "push_data": {
    "ref": "refs/heads/main",
    "action": "pushed"
  }
}
```

**Events monitored:**
- `pushed to` → Invalidates branch and pipeline cache
- `opened`, `closed`, `merged`, `accepted` → Invalidates MR cache
- Other events for comprehensive change detection

---

## GitHub API Calls

All GitHub API calls use the base URL: `https://api.github.com`

Authentication: `Authorization: Bearer {token}` header with read-only token

### 1. Get Repositories
**Endpoint:** `GET /user/repos?per_page=100&page=X`

**Purpose:** Fetch all repositories the authenticated user has access to

**Why needed:**
- Same as GitLab - main data source for repositories list

**Frequency:**
- On startup (cache miss)
- Every 10 minutes (background refresh)

**Data returned:**
```json
{
  "id": 123456,
  "name": "my-project",
  "full_name": "owner/my-project",
  "html_url": "https://github.com/owner/my-project",
  "default_branch": "main",
  "fork": false
}
```

---

### 2. Get Branches
**Endpoint:** `GET /repos/{owner}/{repo}/branches?per_page=50`

**Purpose:** List all branches in a repository

**Why needed:**
- Same as GitLab - show branches on detail page

**Frequency:**
- On-demand when viewing repository details

**Data returned:**
```json
{
  "name": "main",
  "protected": true,
  "commit": {
    "sha": "abc123...",
    "commit": {
      "message": "Fix bug",
      "author": {
        "name": "Jane Smith",
        "date": "2026-01-22T10:00:00Z"
      }
    }
  }
}
```

---

### 3. Get Workflow Runs (Pipeline equivalent)
**Endpoint:** `GET /repos/{owner}/{repo}/actions/runs?branch={branch}&per_page=1`

**Purpose:** Get latest GitHub Actions workflow run for a branch

**Why needed:**
- Same as GitLab pipelines - show build status
- GitHub Actions = CI/CD equivalent to GitLab pipelines

**Frequency:**
- On-demand when loading repository data
- Cached for 4 hours

**Data returned:**
```json
{
  "id": 98765,
  "name": "CI",
  "status": "completed",
  "conclusion": "success",
  "head_branch": "main",
  "html_url": "https://github.com/.../actions/runs/98765",
  "created_at": "2026-01-22T10:00:00Z",
  "updated_at": "2026-01-22T10:05:00Z"
}
```

---

### 4. Get Pull Requests
**Endpoint:** `GET /repos/{owner}/{repo}/pulls?state=open&per_page=50`

**Purpose:** List all open pull requests (GitHub equivalent of merge requests)

**Why needed:**
- Same as GitLab MRs - show PR count, draft count
- **NEW:** Show which PRs you are reviewing (checks `requested_reviewers` field)

**Frequency:**
- On-demand when loading repository data
- Cached for 4 hours

**Data returned:**
```json
{
  "number": 42,
  "title": "Add new feature",
  "state": "open",
  "draft": false,
  "head": {"ref": "feature-branch"},
  "base": {"ref": "main"},
  "user": {"login": "john-doe"},
  "requested_reviewers": [
    {"login": "jane-smith"},
    {"login": "bob-jones"}
  ],
  "created_at": "2026-01-22T10:00:00Z",
  "html_url": "https://github.com/.../pull/42"
}
```

---

### 5. Get Authenticated User
**Endpoint:** `GET /user`

**Purpose:** Get authenticated user's profile

**Why needed:**
- Same as GitLab - display avatar, match reviewer username

**Frequency:**
- On startup
- Cached for 4 hours

**Data returned:**
```json
{
  "login": "jane-smith",
  "name": "Jane Smith",
  "email": "jane@example.com",
  "avatar_url": "https://avatars.githubusercontent.com/...",
  "html_url": "https://github.com/jane-smith"
}
```

---

### 6. Get Issues
**Endpoint:** `GET /repos/{owner}/{repo}/issues?state=open&per_page=50`

**Purpose:** List all open issues (NOTE: GitHub includes PRs in issues endpoint)

**Why needed:**
- Same as GitLab - power the Issues page
- Filters out pull requests (issues with `pull_request` field)

**Frequency:**
- On-demand when viewing issues page
- Cached for 4 hours

**Data returned:**
```json
{
  "number": 123,
  "title": "Bug report",
  "state": "open",
  "labels": [{"name": "bug"}],
  "user": {"login": "john-doe"},
  "assignee": {"login": "jane-smith"},
  "created_at": "2026-01-22T10:00:00Z",
  "html_url": "https://github.com/.../issues/123",
  "pull_request": null
}
```

---

## Cache Strategy

### Stale-While-Revalidate
- **Fresh TTL:** 4 hours (configurable via `GITLAB_CACHE_DURATION_SECONDS`, `GITHUB_CACHE_DURATION_SECONDS`)
- **Stale TTL:** 24 hours (configurable via `STALE_CACHE_DURATION_SECONDS`)
- **Strategy:** Serve cached data immediately (even if expired), refresh in background

### Cache Invalidation
Two mechanisms keep data fresh:

1. **Expiration Refresher** (every 10 minutes)
   - Checks for expired cache entries
   - Refreshes them in background

2. **Event Poller** (every 5 minutes)
   - Polls GitLab/GitHub events API
   - Detects changes (pushes, MRs, etc.)
   - Invalidates specific cache entries
   - Forces immediate refresh of affected data

### File Cache
- **Location:** `.cache/ci-dashboard.json`
- **Purpose:** Persist cache to disk for instant startup
- **Loaded:** On server startup
- **Saved:** Every 10 minutes (background saver)

---

## API Rate Limits

### GitLab
- **Default:** 2,000 requests per user per minute
- **Authenticated:** Much higher with personal access token
- **Strategy:** Caching reduces API calls to ~1-2 calls per project per 4 hours

### GitHub
- **Authenticated:** 5,000 requests per hour
- **Strategy:** Same caching approach keeps well under limits

### Dashboard Efficiency
With 50 repositories:
- **Cold start:** ~150 API calls (projects + branches + pipelines + MRs)
- **Steady state:** ~25 API calls per hour (background refresh + event polling)
- **Well under rate limits** for both platforms

---

## Security: Read-Only Tokens

### GitLab Token Scopes (Required)
✅ **`read_api`** - Read-only access to API (RECOMMENDED)

❌ **Never use:**
- `api` (full read/write access)
- `write_repository` (can push code)
- `admin` scopes (dangerous)

### GitHub Token Scopes (Required)
✅ **Fine-grained tokens (RECOMMENDED):**
- Actions: Read-only
- Contents: Read-only
- Metadata: Read-only (automatic)

✅ **Classic tokens:**
- `public_repo` or `repo` (⚠️ also grants write access)

❌ **Never use:**
- `workflow` (can modify workflows)
- `write:*` (write permissions)
- `delete:*` (delete permissions)
- `admin:*` (admin permissions)

---

## Why Each API Call is Necessary

### Core Dashboard Functionality
1. **GetProjects** - Can't show repositories without this ✅
2. **GetBranches** - Need to show default branch, commit info ✅
3. **GetLatestPipeline** - Main purpose: show CI/CD status ✅

### Enhanced Features
4. **GetMergeRequests** - Show MR/PR counts, **NEW: show what you're reviewing** ✅
5. **GetCurrentUser** - Display avatar, power "Your Branches", **NEW: match reviewers** ✅
6. **GetIssues** - Optional: Issues page ⚠️
7. **GetEvents** - Smart caching: only refresh when needed ✅

### Performance
8. **File Cache** - Instant startup (load from disk) ✅
9. **Stale-While-Revalidate** - Never block on API calls ✅
10. **Event Polling** - Detect changes, invalidate only what changed ✅

---

## Summary

**Total API Endpoints Used:**
- **GitLab:** 7 endpoints (all read-only)
- **GitHub:** 6 endpoints (all read-only)

**All calls are necessary** for the dashboard's core functionality:
- Monitoring CI/CD pipeline status ✅
- Showing repository information ✅
- **NEW:** Tracking code reviews you're involved in ✅
- Displaying user avatars ✅
- Filtering your branches ✅
- Efficient caching with change detection ✅

**Zero write operations** - This is a pure monitoring/observability tool.
