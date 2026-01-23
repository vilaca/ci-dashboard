# CI Dashboard - Caching Architecture

## Overview: Stale-While-Revalidate Strategy

The dashboard uses a **stale-while-revalidate** caching strategy to maximize performance:
- **Always serve from cache** (even if expired/stale)
- **Never block on API calls** - background refresh only
- **Prioritize recent activity** - repos with commits in last week refreshed first
- **Monitor memory usage** - logs RAM usage every 30 seconds

## Architecture Components

### 1. Stale Cache (`internal/api/stale_cache.go`)

**Two TTL Values:**
- **Fresh TTL** (4 hours): Data is considered fresh, no refresh needed
- **Stale TTL** (24 hours): Data can still be served, but needs background refresh

**Cache Entry States:**
```
Fresh    (age < 4h):   Serve immediately, no refresh
Stale    (4h < age < 24h): Serve immediately, mark for background refresh
Expired  (age > 24h):  Don't serve, fetch from API
```

**Example Timeline:**
```
0h:    Cache entry created        → FRESH
3h:    User requests data          → FRESH (serve from cache)
5h:    User requests data          → STALE (serve from cache + queue refresh)
25h:   User requests data          → EXPIRED (fetch from API)
```

### 2. Stale-Caching Client (`internal/api/stale_cache.go`)

**Wraps API clients with stale-aware caching:**
```go
// Always returns cached data if available (even if stale)
projects, err := client.GetProjects(ctx)

// Cache behavior:
// - Fresh data: Log "FRESH HIT"
// - Stale data: Log "STALE HIT" + mark for refresh
// - No data: Log "MISS" + fetch from API
```

**Tracks metadata for priority refresh:**
- Project ID
- Last commit date
- Cache age

### 3. Expiration Refresher (`internal/service/expiration_refresher.go`)

**Background worker that runs every 10 minutes:**
```
Every 10 minutes:
  1. Get list of expired keys from all clients
  2. Sort by priority (recent commits first)
  3. Refresh expired entries (max 5 concurrent)
  4. Update cache
```

**Priority Refresh Order:**
1. Repositories with commits in last 7 days
2. Repositories with commits in last 30 days
3. All other repositories

### 4. Memory Monitor

**Logs every 30 seconds:**
```
[Memory] Alloc=45 MB, TotalAlloc=1250 MB, Sys=78 MB, NumGC=42 |
         Cache: 324 total (187 fresh, 112 stale, 25 expired)
```

## Data Flow

### Server Startup Sequence

```
1. Server starts
   ↓
2. Load cache from .cache/ci-dashboard.json
   ↓
3. If cache file exists:
     - Load all entries into StaleCache
     - Even expired data is loaded (can be served if < 24h old)
   ↓
4. Server ready (responds immediately to requests)
   ↓
5. After 5 seconds: ExpirationRefresher starts first check
     - Identifies expired entries
     - Refreshes them in background (prioritized)
   ↓
6. Every 10 minutes: Periodic refresh check
```

### Page Load Sequence

```
User loads /repositories
   ↓
1. Render skeleton (instant - no API calls)
   ↓
2. Browser requests /api/stream/repositories
   ↓
3. For each repository:
     a. GetBranches(projectID, 50)
        → Check cache
        → Return cached data (fresh or stale)
        → If stale: mark for background refresh

     b. GetLatestPipeline(projectID, branch)
        → Check cache
        → Return cached data (fresh or stale)
        → If stale: mark for background refresh

     c. GetMergeRequests(projectID)
        → Check cache
        → Return cached data (fresh or stale)
        → If stale: mark for background refresh
   ↓
4. Stream results to browser as fast as cache allows
   ↓
5. Background: Stale entries queued for refresh
```

### Background Refresh Sequence

```
Every 10 minutes:
   ↓
1. ExpirationRefresher wakes up
   ↓
2. Query each client for expired keys
     client.GetExpiredKeys() → ["GetLatestPipeline:123:main", ...]
   ↓
3. Keys are already sorted by priority:
     - Recent commits (last 7 days) first
     - Older commits last
   ↓
4. Refresh keys (max 5 concurrent):
     For "GetLatestPipeline:123:main":
       → Parse key
       → Call client.GetLatestPipeline(ctx, "123", "main")
       → Update cache
       → Log success
   ↓
5. Log completion:
     "Refreshed 47/89 expired entries in 8.5s"
```

## Cache Key Format

All cache keys follow a predictable pattern:

```
GetProjects                              → All projects
GetProjectCount                          → Project count
GetBranches:projectID:limit              → Branches for project
GetLatestPipeline:projectID:branch       → Latest pipeline for branch
GetPipelines:projectID:limit             → Recent pipelines for project
GetMergeRequests:projectID               → Open MRs for project
GetIssues:projectID                      → Open issues for project
GetCurrentUser                           → Current user profile
```

## Memory Usage

**Typical Memory Footprint:**
```
100 repositories × 3 branches × 5 cache entries per branch = 1,500 entries

Average entry size:
- Projects: ~500 bytes
- Pipelines: ~1 KB
- Branches: ~300 bytes
- MRs: ~800 bytes

Estimated total: 1,500 entries × ~800 bytes = ~1.2 MB
Plus Go runtime overhead: ~50-100 MB total
```

## Configuration

### Environment Variables

```bash
# Cache TTL (fresh data)
GITLAB_CACHE_DURATION_SECONDS=14400   # 4 hours
GITHUB_CACHE_DURATION_SECONDS=14400   # 4 hours

# Stale TTL (how long to serve stale data)
STALE_CACHE_DURATION_SECONDS=86400    # 24 hours

# Refresh interval
EXPIRATION_CHECK_INTERVAL_SECONDS=600  # 10 minutes

# File cache location
CACHE_FILE_PATH=.cache/ci-dashboard.json
```

### In-Code Configuration

File: `cmd/ci-dashboard/main.go`
```go
// Create stale-caching client
freshTTL := time.Duration(cfg.GitLabCacheDurationSeconds) * time.Second  // 4h
staleTTL := 24 * time.Hour                                               // 24h
cachedClient := api.NewStaleCachingClient(client, freshTTL, staleTTL)

// Create expiration refresher
refreshInterval := 10 * time.Minute
refresher := service.NewExpirationRefresher(pipelineService, refreshInterval)
refresher.Start()
```

## Performance Characteristics

### Page Load Times

**With Warm Cache (all fresh):**
- First byte: < 10ms
- Full page: < 100ms (streaming)

**With Stale Cache (4-24 hours old):**
- First byte: < 10ms
- Full page: < 100ms (streaming)
- Background refresh: Happens after page load

**With Cold Cache (> 24 hours old):**
- First byte: < 10ms (skeleton)
- Full page: 5-15 seconds (must fetch from APIs)
- Subsequent loads: < 100ms (cached)

### API Call Reduction

**Without Caching:**
- Page load: 100+ API calls
- Background refresh: 0 calls
- Total: 100+ calls per page load

**With Stale Caching:**
- Page load: 0-5 API calls (only for expired entries)
- Background refresh: 10-50 calls every 10 minutes
- Total: < 1 call per page load

## Monitoring & Debugging

### Log Messages

**Cache Operations:**
```
[StaleCache] GetLatestPipeline:123:main - FRESH HIT (age: 2h15m)
[StaleCache] GetLatestPipeline:456:main - STALE HIT (expired 1h ago, still usable)
[StaleCache] GetLatestPipeline:789:main - TOO STALE (cached 25h ago)
[StaleCache] GetLatestPipeline:999:main - MISS, fetching from API
```

**Memory Monitoring:**
```
[Memory] Alloc=45 MB, TotalAlloc=1250 MB, Sys=78 MB, NumGC=42 |
         Cache: 324 total (187 fresh, 112 stale, 25 expired)
```

**Background Refresh:**
```
[ExpirationRefresher] Starting with 10m check interval
[ExpirationRefresher] gitlab: Found 47 expired cache entries
[ExpirationRefresher] github: Found 12 expired cache entries
[ExpirationRefresher] Completed in 8.5s: 59/59 entries refreshed
```

## Trade-offs

### Advantages
✅ Near-instant page loads (serve stale data)
✅ No blocking on API calls
✅ Reduced API pressure (refresh only expired entries)
✅ Graceful degradation (old data better than no data)
✅ Automatic prioritization (recent repos first)

### Disadvantages
❌ May serve stale data (up to 4 hours old)
❌ Higher memory usage (stores more data)
❌ Complexity (two TTLs, background workers)

## Future Improvements

1. **Smart Invalidation**: Invalidate cache on webhook events
2. **Adaptive TTL**: Shorter TTL for active repos, longer for inactive
3. **LRU Eviction**: Remove least recently used entries when memory is tight
4. **Distributed Cache**: Redis/Memcached for multi-instance deployments
5. **Cache Warming**: Pre-fetch on server start for critical repos
