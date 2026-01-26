# CI/CD Dashboard - Microservices Architecture

## Table of Contents
1. [Overview](#overview)
2. [Service Decomposition](#service-decomposition)
3. [Service Details](#service-details)
4. [Communication Patterns](#communication-patterns)
5. [Data Management](#data-management)
6. [Deployment Architecture](#deployment-architecture)
7. [Comparison: Monolith vs Microservices](#comparison-monolith-vs-microservices)
8. [Migration Strategy](#migration-strategy)
9. [Trade-offs and Considerations](#trade-offs-and-considerations)

---

## Overview

### Current Monolithic Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Single Application                        │
│                                                              │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │   GitLab     │  │   GitHub     │  │  Future      │     │
│  │   Client     │  │   Client     │  │  Platforms   │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
│         │                 │                  │              │
│         └─────────────────┼──────────────────┘              │
│                           │                                 │
│                  ┌────────▼────────┐                        │
│                  │  Stale Cache    │                        │
│                  └────────┬────────┘                        │
│                           │                                 │
│                  ┌────────▼────────┐                        │
│                  │ Pipeline Service│                        │
│                  └────────┬────────┘                        │
│                           │                                 │
│                  ┌────────▼────────┐                        │
│                  │  HTTP Handlers  │                        │
│                  └─────────────────┘                        │
└─────────────────────────────────────────────────────────────┘
```

### Proposed Microservices Architecture

```
                          ┌─────────────────┐
                          │   API Gateway   │
                          │  (Kong/Nginx)   │
                          └────────┬────────┘
                                   │
        ┌──────────────────────────┼──────────────────────────┐
        │                          │                          │
        ▼                          ▼                          ▼
┌───────────────┐          ┌───────────────┐        ┌───────────────┐
│  Dashboard    │          │  Aggregation  │        │   Webhook     │
│   Service     │◄─────────│    Service    │────────►   Service    │
│  (Frontend)   │          │ (Orchestrator)│        │  (Events)     │
└───────────────┘          └───────┬───────┘        └───────────────┘
                                   │
        ┌──────────────────────────┼──────────────────────────┐
        │                          │                          │
        ▼                          ▼                          ▼
┌───────────────┐          ┌───────────────┐        ┌───────────────┐
│    GitLab     │          │    GitHub     │        │    Jenkins    │
│   Connector   │          │   Connector   │        │   Connector   │
│    Service    │          │    Service    │        │    Service    │
└───────┬───────┘          └───────┬───────┘        └───────┬───────┘
        │                          │                          │
        ▼                          ▼                          ▼
┌───────────────┐          ┌───────────────┐        ┌───────────────┐
│ GitLab Cache  │          │ GitHub Cache  │        │ Jenkins Cache │
│     (Redis)   │          │     (Redis)   │        │     (Redis)   │
└───────────────┘          └───────────────┘        └───────────────┘
```

---

## Service Decomposition

### Domain Boundaries

Based on Domain-Driven Design (DDD), we identify these bounded contexts:

1. **Platform Integration Context** - Each CI/CD platform (GitLab, GitHub, etc.)
2. **Data Aggregation Context** - Cross-platform data orchestration
3. **Presentation Context** - User interface and API
4. **Event Processing Context** - Webhooks, real-time updates
5. **User Context** - Authentication, authorization, preferences

---

## Service Details

### 1. API Gateway

**Purpose**: Single entry point, routing, authentication, rate limiting

**Technology**: Kong, Traefik, or Nginx with Lua

**Responsibilities**:
- Route requests to appropriate services
- JWT authentication/validation
- Rate limiting per user/IP
- Request/response transformation
- CORS handling
- SSL termination
- Load balancing

**Endpoints**:
- `GET /api/repositories` → Dashboard Service
- `GET /api/gitlab/*` → GitLab Connector Service
- `GET /api/github/*` → GitHub Connector Service
- `POST /webhooks/gitlab` → Webhook Service
- `POST /webhooks/github` → Webhook Service

**Configuration Example (Kong)**:
```yaml
services:
  - name: dashboard-service
    url: http://dashboard-service:8080
    routes:
      - name: repositories-route
        paths:
          - /api/repositories
        methods:
          - GET

  - name: gitlab-connector
    url: http://gitlab-connector:8081
    routes:
      - name: gitlab-route
        paths:
          - /api/gitlab
        strip_path: true
```

---

### 2. Dashboard Service (Frontend/BFF)

**Purpose**: Backend-for-Frontend, serves UI and aggregates data

**Technology**: Go (or Node.js for rich frontend integration)

**Port**: 8080

**Responsibilities**:
- Serve HTML/CSS/JavaScript
- Provide BFF API for frontend
- Call Aggregation Service for data
- Handle user sessions
- Manage UI-specific caching
- Avatar caching

**Database**:
- Redis (user sessions, UI cache)

**API Endpoints**:
```
GET  /                          - Serve main page
GET  /api/health                - Health check
GET  /api/repositories          - Get all repositories (calls Aggregation Service)
GET  /api/repository-detail     - Get repository detail
GET  /api/avatar/:platform/:user - Serve cached avatar
```

**Dependencies**:
- Aggregation Service (HTTP)
- Redis (cache)

**Code Structure**:
```
dashboard-service/
├── cmd/
│   └── server/
│       └── main.go
├── internal/
│   ├── handlers/
│   │   ├── repositories.go
│   │   ├── health.go
│   │   └── avatar.go
│   ├── renderer/
│   │   └── html.go
│   └── client/
│       └── aggregation_client.go  # Calls Aggregation Service
├── web/
│   ├── templates/
│   └── static/
└── Dockerfile
```

**Configuration**:
```yaml
server:
  port: 8080

aggregation_service:
  url: http://aggregation-service:8082
  timeout: 30s

redis:
  url: redis://redis:6379

cache:
  avatar_ttl: 24h
  session_ttl: 1h
```

---

### 3. Aggregation Service

**Purpose**: Orchestrates data from multiple connector services

**Technology**: Go

**Port**: 8082

**Responsibilities**:
- Aggregate data from multiple connector services
- Apply filtering (watched repos)
- Combine results from GitLab + GitHub + others
- Provide unified API for Dashboard Service
- Handle cross-platform queries
- Manage request fan-out and fan-in

**Database**:
- Redis (aggregation cache)
- Optional: PostgreSQL (metadata, user preferences)

**API Endpoints**:
```
GET  /api/projects                    - Get all projects from all platforms
GET  /api/projects/:id                - Get specific project details
GET  /api/projects/:id/pipelines      - Get pipelines for project
GET  /api/projects/:id/branches       - Get branches for project
GET  /api/projects/:id/merge-requests - Get merge requests for project
GET  /api/user/profiles               - Get user profiles from all platforms
```

**Dependencies**:
- GitLab Connector Service (HTTP)
- GitHub Connector Service (HTTP)
- Future Connector Services (HTTP)
- Redis (cache)

**Code Structure**:
```
aggregation-service/
├── cmd/
│   └── server/
│       └── main.go
├── internal/
│   ├── handlers/
│   │   ├── projects.go
│   │   ├── pipelines.go
│   │   └── branches.go
│   ├── orchestrator/
│   │   ├── service.go           # Main orchestration logic
│   │   └── aggregator.go        # Data merging logic
│   ├── client/
│   │   ├── connector_client.go  # Generic connector client
│   │   └── registry.go          # Service discovery
│   └── cache/
│       └── redis.go
└── Dockerfile
```

**Configuration**:
```yaml
server:
  port: 8082

connectors:
  gitlab:
    url: http://gitlab-connector:8081
    timeout: 30s
  github:
    url: http://github-connector:8083
    timeout: 30s

redis:
  url: redis://redis:6379

cache:
  ttl: 5m
  stale_ttl: 24h

filtering:
  watched_repos:
    - "123"  # GitLab project IDs
    - "owner/repo"  # GitHub repos
```

**Orchestration Pattern**:
```go
func (s *Service) GetAllProjects(ctx context.Context) ([]domain.Project, error) {
    // Fan-out: Call all connectors concurrently
    var wg sync.WaitGroup
    projectsChan := make(chan []domain.Project, len(s.connectors))
    errorsChan := make(chan error, len(s.connectors))

    for name, connector := range s.connectors {
        wg.Add(1)
        go func(n string, c ConnectorClient) {
            defer wg.Done()
            projects, err := c.GetProjects(ctx)
            if err != nil {
                errorsChan <- fmt.Errorf("%s: %w", n, err)
                return
            }
            projectsChan <- projects
        }(name, connector)
    }

    wg.Wait()
    close(projectsChan)
    close(errorsChan)

    // Fan-in: Aggregate results
    var allProjects []domain.Project
    for projects := range projectsChan {
        allProjects = append(allProjects, projects...)
    }

    // Apply filtering
    filtered := s.filterByWatchedRepos(allProjects)

    return filtered, nil
}
```

---

### 4. GitLab Connector Service

**Purpose**: Dedicated service for GitLab API integration

**Technology**: Go

**Port**: 8081

**Responsibilities**:
- All GitLab API communication
- GitLab-specific authentication
- GitLab response parsing/transformation
- GitLab data caching
- GitLab rate limit handling (if any)

**Database**:
- Redis (GitLab-specific cache)

**API Endpoints**:
```
GET  /api/projects                    - Get all GitLab projects
GET  /api/projects/:id                - Get specific project
GET  /api/projects/:id/pipelines      - Get pipelines
GET  /api/projects/:id/branches       - Get branches
GET  /api/projects/:id/branches/:name - Get specific branch
GET  /api/projects/:id/merge-requests - Get merge requests
GET  /api/projects/:id/issues         - Get issues
GET  /api/user/profile                - Get current user profile
POST /api/cache/refresh               - Trigger cache refresh
```

**Dependencies**:
- GitLab API (external)
- Redis (cache)

**Code Structure**:
```
gitlab-connector/
├── cmd/
│   └── server/
│       └── main.go
├── internal/
│   ├── handlers/
│   │   ├── projects.go
│   │   ├── pipelines.go
│   │   ├── branches.go
│   │   └── merge_requests.go
│   ├── client/
│   │   ├── gitlab_client.go     # HTTP client for GitLab API
│   │   └── types.go             # GitLab-specific types
│   ├── converter/
│   │   └── domain.go            # Convert GitLab types to domain
│   ├── cache/
│   │   ├── stale_cache.go       # Stale-while-revalidate cache
│   │   └── redis.go
│   └── refresher/
│       └── background.go        # Background cache refresh
└── Dockerfile
```

**Configuration**:
```yaml
server:
  port: 8081

gitlab:
  url: https://gitlab.com
  token: ${GITLAB_TOKEN}
  timeout: 30s

redis:
  url: redis://redis-gitlab:6379

cache:
  ttl: 5m
  stale_ttl: 24h
  refresh_interval: 5m
```

**Environment Variables**:
```bash
GITLAB_TOKEN=glpat-xxxxxxxxxxxx
```

---

### 5. GitHub Connector Service

**Purpose**: Dedicated service for GitHub API integration

**Technology**: Go

**Port**: 8083

**Responsibilities**:
- All GitHub API communication
- GitHub-specific authentication
- GitHub response parsing/transformation
- GitHub data caching
- **GitHub rate limit handling** (5000 req/hour)

**Database**:
- Redis (GitHub-specific cache)

**API Endpoints**:
```
GET  /api/projects                    - Get all GitHub repositories
GET  /api/projects/:owner/:repo       - Get specific repository
GET  /api/projects/:owner/:repo/pipelines      - Get workflow runs
GET  /api/projects/:owner/:repo/branches       - Get branches
GET  /api/projects/:owner/:repo/branches/:name - Get specific branch
GET  /api/projects/:owner/:repo/pull-requests  - Get pull requests
GET  /api/projects/:owner/:repo/issues         - Get issues
GET  /api/user/profile                         - Get current user profile
POST /api/cache/refresh                        - Trigger cache refresh
GET  /api/rate-limit                           - Get current rate limit status
```

**Dependencies**:
- GitHub API (external)
- Redis (cache)

**Code Structure**:
```
github-connector/
├── cmd/
│   └── server/
│       └── main.go
├── internal/
│   ├── handlers/
│   │   ├── repositories.go
│   │   ├── workflows.go
│   │   ├── branches.go
│   │   └── pull_requests.go
│   ├── client/
│   │   ├── github_client.go     # HTTP client for GitHub API
│   │   ├── rate_limiter.go      # Rate limit tracking and blocking
│   │   └── types.go             # GitHub-specific types
│   ├── converter/
│   │   └── domain.go            # Convert GitHub types to domain
│   ├── cache/
│   │   ├── stale_cache.go
│   │   └── redis.go
│   └── refresher/
│       └── background.go
└── Dockerfile
```

**Configuration**:
```yaml
server:
  port: 8083

github:
  url: https://api.github.com
  token: ${GITHUB_TOKEN}
  timeout: 30s

redis:
  url: redis://redis-github:6379

cache:
  ttl: 5m
  stale_ttl: 24h
  refresh_interval: 5m

rate_limit:
  max_per_hour: 5000
  buffer_threshold: 250  # Start aggressive caching at 250 remaining
```

**Rate Limit Metrics Endpoint**:
```json
GET /api/rate-limit

{
  "limit": 5000,
  "remaining": 4750,
  "reset": "2026-01-26T14:00:00Z",
  "percentage_used": 5.0
}
```

---

### 6. Webhook Service

**Purpose**: Handle real-time updates from CI/CD platforms

**Technology**: Go + Message Queue (RabbitMQ/NATS)

**Port**: 8084

**Responsibilities**:
- Receive webhooks from GitLab/GitHub
- Validate webhook signatures
- Parse webhook payloads
- Publish events to message queue
- Trigger cache invalidation
- Rate limit webhook processing

**Database**:
- PostgreSQL (webhook event log)
- Message Queue (RabbitMQ/NATS)

**API Endpoints**:
```
POST /webhooks/gitlab/:project_id  - Receive GitLab webhooks
POST /webhooks/github/:owner/:repo - Receive GitHub webhooks
GET  /api/events                   - Query recent events
GET  /api/health                   - Health check
```

**Dependencies**:
- Message Queue (publish events)
- PostgreSQL (event storage)
- GitLab Connector (trigger cache invalidation)
- GitHub Connector (trigger cache invalidation)

**Code Structure**:
```
webhook-service/
├── cmd/
│   └── server/
│       └── main.go
├── internal/
│   ├── handlers/
│   │   ├── gitlab_webhook.go
│   │   └── github_webhook.go
│   ├── validator/
│   │   ├── gitlab_signature.go
│   │   └── github_signature.go
│   ├── parser/
│   │   ├── gitlab_events.go
│   │   └── github_events.go
│   ├── publisher/
│   │   └── event_publisher.go   # Publish to message queue
│   └── store/
│       └── postgres.go
└── Dockerfile
```

**Configuration**:
```yaml
server:
  port: 8084

gitlab:
  webhook_secret: ${GITLAB_WEBHOOK_SECRET}

github:
  webhook_secret: ${GITHUB_WEBHOOK_SECRET}

postgres:
  url: postgres://user:pass@postgres:5432/webhooks

message_queue:
  type: rabbitmq
  url: amqp://guest:guest@rabbitmq:5672/
  exchange: ci-events

connectors:
  gitlab:
    url: http://gitlab-connector:8081
  github:
    url: http://github-connector:8083
```

**Event Flow**:
```
1. Webhook received
2. Validate signature
3. Parse payload
4. Store event in PostgreSQL
5. Publish event to message queue
6. Call connector service to invalidate cache
7. Respond 200 OK
```

**Message Queue Event Format**:
```json
{
  "type": "pipeline.completed",
  "platform": "gitlab",
  "project_id": "123",
  "project_name": "my-project",
  "pipeline_id": "456",
  "status": "success",
  "branch": "main",
  "timestamp": "2026-01-26T12:00:00Z",
  "metadata": {
    "duration": 270,
    "author": "john.doe"
  }
}
```

---

### 7. User Service (Optional)

**Purpose**: Centralized user management, authentication, preferences

**Technology**: Go + PostgreSQL

**Port**: 8085

**Responsibilities**:
- User authentication (JWT)
- User preferences (watched repos, filters)
- Multi-platform user mapping
- Authorization rules
- User session management

**Database**:
- PostgreSQL (users, preferences)
- Redis (sessions)

**API Endpoints**:
```
POST /api/auth/login              - User login
POST /api/auth/logout             - User logout
POST /api/auth/refresh            - Refresh JWT token
GET  /api/users/me                - Get current user
PUT  /api/users/me/preferences    - Update preferences
GET  /api/users/me/watched-repos  - Get watched repositories
POST /api/users/me/watched-repos  - Add watched repository
```

**Code Structure**:
```
user-service/
├── cmd/
│   └── server/
│       └── main.go
├── internal/
│   ├── handlers/
│   │   ├── auth.go
│   │   └── preferences.go
│   ├── auth/
│   │   ├── jwt.go
│   │   └── session.go
│   └── store/
│       └── postgres.go
└── Dockerfile
```

---

### 8. Notification Service (Optional/Future)

**Purpose**: Send notifications based on pipeline events

**Technology**: Go + Message Queue Consumer

**Port**: 8086

**Responsibilities**:
- Consume events from message queue
- Send email notifications
- Send Slack/Teams notifications
- Manage notification preferences
- Template rendering

**Dependencies**:
- Message Queue (consume events)
- SMTP Server (email)
- Slack/Teams APIs

---

## Communication Patterns

### Synchronous Communication (HTTP/gRPC)

**When to Use**:
- Request-response patterns
- Real-time data needs
- Dashboard queries

**Example Flow**:
```
Browser → API Gateway → Dashboard Service → Aggregation Service
                                                ├→ GitLab Connector
                                                ├→ GitHub Connector
                                                └→ Jenkins Connector
```

**Protocol**: REST over HTTP/1.1 or HTTP/2

**Timeouts**:
- API Gateway → Dashboard: 30s
- Dashboard → Aggregation: 30s
- Aggregation → Connectors: 10s each
- Connectors → External APIs: 5s

**Circuit Breaker Pattern**:
```go
// Aggregation Service
func (s *Service) GetProjects(ctx context.Context, connectorName string) ([]domain.Project, error) {
    cb := s.circuitBreakers[connectorName]

    result, err := cb.Execute(func() (interface{}, error) {
        return s.connectors[connectorName].GetProjects(ctx)
    })

    if err != nil {
        // Circuit open - return cached data
        return s.cache.GetProjects(connectorName)
    }

    return result.([]domain.Project), nil
}
```

---

### Asynchronous Communication (Message Queue)

**When to Use**:
- Event-driven updates
- Background processing
- Cache invalidation
- Decoupling services

**Technology**: RabbitMQ or NATS

**Example Flow**:
```
GitLab/GitHub Webhook → Webhook Service → Message Queue → Connector Services
                                                        → Notification Service
```

**Message Format**:
```json
{
  "event_id": "550e8400-e29b-41d4-a716-446655440000",
  "type": "pipeline.completed",
  "platform": "gitlab",
  "project_id": "123",
  "timestamp": "2026-01-26T12:00:00Z",
  "payload": { ... }
}
```

**Exchange Configuration (RabbitMQ)**:
```
Exchange: ci-events (topic)
Queues:
  - gitlab-connector-queue    (routing key: gitlab.*)
  - github-connector-queue    (routing key: github.*)
  - notification-queue        (routing key: *.pipeline.*)
```

---

### Service Discovery

**Options**:

1. **Static Configuration** (Simple, for small deployments):
```yaml
# aggregation-service config
connectors:
  gitlab:
    url: http://gitlab-connector:8081
  github:
    url: http://github-connector:8083
```

2. **DNS-Based** (Kubernetes):
```yaml
# Use Kubernetes service DNS
connectors:
  gitlab:
    url: http://gitlab-connector.default.svc.cluster.local:8081
  github:
    url: http://github-connector.default.svc.cluster.local:8083
```

3. **Service Mesh** (Istio/Linkerd):
- Automatic service discovery
- Traffic management
- Observability
- mTLS between services

4. **Consul/Eureka** (Traditional):
```go
// Service registration
consul.Register(&consul.ServiceRegistration{
    ID:      "gitlab-connector-1",
    Name:    "gitlab-connector",
    Port:    8081,
    Address: "10.0.1.5",
    Tags:    []string{"v1", "connector"},
    Check: &consul.HealthCheck{
        HTTP:     "http://10.0.1.5:8081/health",
        Interval: "10s",
    },
})

// Service discovery
services, _ := consul.Discover("gitlab-connector")
```

---

## Data Management

### Database Per Service Pattern

Each service owns its data:

```
┌─────────────────────┐
│ GitLab Connector    │
│ ┌─────────────────┐ │
│ │ Redis           │ │
│ │ - Projects      │ │
│ │ - Pipelines     │ │
│ │ - Branches      │ │
│ └─────────────────┘ │
└─────────────────────┘

┌─────────────────────┐
│ GitHub Connector    │
│ ┌─────────────────┐ │
│ │ Redis           │ │
│ │ - Repositories  │ │
│ │ - Workflows     │ │
│ │ - Branches      │ │
│ └─────────────────┘ │
└─────────────────────┘

┌─────────────────────┐
│ Webhook Service     │
│ ┌─────────────────┐ │
│ │ PostgreSQL      │ │
│ │ - Events Log    │ │
│ │ - Deliveries    │ │
│ └─────────────────┘ │
└─────────────────────┘

┌─────────────────────┐
│ User Service        │
│ ┌─────────────────┐ │
│ │ PostgreSQL      │ │
│ │ - Users         │ │
│ │ - Preferences   │ │
│ └─────────────────┘ │
└─────────────────────┘
```

### Cache Strategy

**Cache per Connector**:
- Each connector service has its own Redis instance
- Stale-while-revalidate pattern maintained
- Independent cache invalidation
- No shared cache dependencies

**Cache Keys**:
```
GitLab Connector Redis:
- gitlab:projects
- gitlab:project:123:pipelines:50
- gitlab:project:123:branches:200
- gitlab:project:123:branch:main

GitHub Connector Redis:
- github:repos
- github:repo:owner/name:workflows:50
- github:repo:owner/name:branches:200
- github:repo:owner/name:branch:main
```

**Cache Invalidation via Events**:
```go
// Connector service subscribes to events
func (s *Service) handleEvent(event Event) {
    switch event.Type {
    case "pipeline.completed":
        // Invalidate pipeline cache
        s.cache.Invalidate(fmt.Sprintf("project:%s:pipelines:*", event.ProjectID))

    case "push":
        // Invalidate branch cache
        s.cache.Invalidate(fmt.Sprintf("project:%s:branch:%s", event.ProjectID, event.Branch))
    }
}
```

---

## Deployment Architecture

### Container Orchestration (Kubernetes)

**Namespace Structure**:
```yaml
namespaces:
  - ci-dashboard       # Main application services
  - ci-infrastructure  # Redis, RabbitMQ, PostgreSQL
  - ci-monitoring      # Prometheus, Grafana
```

**Service Deployment Example**:
```yaml
# gitlab-connector-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gitlab-connector
  namespace: ci-dashboard
spec:
  replicas: 3
  selector:
    matchLabels:
      app: gitlab-connector
  template:
    metadata:
      labels:
        app: gitlab-connector
        version: v1
    spec:
      containers:
      - name: gitlab-connector
        image: registry.example.com/gitlab-connector:v1.0.0
        ports:
        - containerPort: 8081
        env:
        - name: GITLAB_TOKEN
          valueFrom:
            secretKeyRef:
              name: gitlab-credentials
              key: token
        - name: REDIS_URL
          value: redis://redis-gitlab:6379
        resources:
          requests:
            memory: "128Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health
            port: 8081
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 5

---
apiVersion: v1
kind: Service
metadata:
  name: gitlab-connector
  namespace: ci-dashboard
spec:
  selector:
    app: gitlab-connector
  ports:
  - port: 8081
    targetPort: 8081
  type: ClusterIP

---
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: gitlab-connector-hpa
  namespace: ci-dashboard
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: gitlab-connector
  minReplicas: 2
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
```

**Redis Deployment (StatefulSet)**:
```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: redis-gitlab
  namespace: ci-infrastructure
spec:
  serviceName: redis-gitlab
  replicas: 1
  selector:
    matchLabels:
      app: redis-gitlab
  template:
    metadata:
      labels:
        app: redis-gitlab
    spec:
      containers:
      - name: redis
        image: redis:7-alpine
        ports:
        - containerPort: 6379
        volumeMounts:
        - name: redis-data
          mountPath: /data
        resources:
          requests:
            memory: "256Mi"
            cpu: "100m"
          limits:
            memory: "1Gi"
            cpu: "500m"
  volumeClaimTemplates:
  - metadata:
      name: redis-data
    spec:
      accessModes: [ "ReadWriteOnce" ]
      resources:
        requests:
          storage: 10Gi
```

**Ingress (API Gateway)**:
```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: ci-dashboard-ingress
  namespace: ci-dashboard
  annotations:
    nginx.ingress.kubernetes.io/rate-limit: "100"
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
spec:
  ingressClassName: nginx
  tls:
  - hosts:
    - ci-dashboard.example.com
    secretName: ci-dashboard-tls
  rules:
  - host: ci-dashboard.example.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: dashboard-service
            port:
              number: 8080
      - path: /webhooks
        pathType: Prefix
        backend:
          service:
            name: webhook-service
            port:
              number: 8084
```

---

### Docker Compose (Development)

**docker-compose.yml**:
```yaml
version: '3.8'

services:
  # API Gateway
  api-gateway:
    image: nginx:alpine
    ports:
      - "8080:80"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf:ro
    depends_on:
      - dashboard-service
      - aggregation-service
      - webhook-service

  # Dashboard Service
  dashboard-service:
    build:
      context: ./dashboard-service
      dockerfile: Dockerfile
    ports:
      - "8081:8080"
    environment:
      - AGGREGATION_SERVICE_URL=http://aggregation-service:8082
      - REDIS_URL=redis://redis-dashboard:6379
    depends_on:
      - redis-dashboard
      - aggregation-service

  # Aggregation Service
  aggregation-service:
    build:
      context: ./aggregation-service
      dockerfile: Dockerfile
    ports:
      - "8082:8082"
    environment:
      - GITLAB_CONNECTOR_URL=http://gitlab-connector:8081
      - GITHUB_CONNECTOR_URL=http://github-connector:8083
      - REDIS_URL=redis://redis-aggregation:6379
    depends_on:
      - redis-aggregation
      - gitlab-connector
      - github-connector

  # GitLab Connector
  gitlab-connector:
    build:
      context: ./gitlab-connector
      dockerfile: Dockerfile
    ports:
      - "8083:8081"
    environment:
      - GITLAB_URL=https://gitlab.com
      - GITLAB_TOKEN=${GITLAB_TOKEN}
      - REDIS_URL=redis://redis-gitlab:6379
    depends_on:
      - redis-gitlab

  # GitHub Connector
  github-connector:
    build:
      context: ./github-connector
      dockerfile: Dockerfile
    ports:
      - "8084:8083"
    environment:
      - GITHUB_URL=https://api.github.com
      - GITHUB_TOKEN=${GITHUB_TOKEN}
      - REDIS_URL=redis://redis-github:6379
    depends_on:
      - redis-github

  # Webhook Service
  webhook-service:
    build:
      context: ./webhook-service
      dockerfile: Dockerfile
    ports:
      - "8085:8084"
    environment:
      - GITLAB_WEBHOOK_SECRET=${GITLAB_WEBHOOK_SECRET}
      - GITHUB_WEBHOOK_SECRET=${GITHUB_WEBHOOK_SECRET}
      - POSTGRES_URL=postgres://user:pass@postgres:5432/webhooks
      - RABBITMQ_URL=amqp://guest:guest@rabbitmq:5672/
      - GITLAB_CONNECTOR_URL=http://gitlab-connector:8081
      - GITHUB_CONNECTOR_URL=http://github-connector:8083
    depends_on:
      - postgres
      - rabbitmq
      - gitlab-connector
      - github-connector

  # Redis instances
  redis-dashboard:
    image: redis:7-alpine
    volumes:
      - redis-dashboard-data:/data

  redis-aggregation:
    image: redis:7-alpine
    volumes:
      - redis-aggregation-data:/data

  redis-gitlab:
    image: redis:7-alpine
    volumes:
      - redis-gitlab-data:/data

  redis-github:
    image: redis:7-alpine
    volumes:
      - redis-github-data:/data

  # PostgreSQL
  postgres:
    image: postgres:15-alpine
    environment:
      - POSTGRES_USER=user
      - POSTGRES_PASSWORD=pass
      - POSTGRES_DB=webhooks
    volumes:
      - postgres-data:/var/lib/postgresql/data

  # RabbitMQ
  rabbitmq:
    image: rabbitmq:3-management-alpine
    ports:
      - "15672:15672"  # Management UI
    environment:
      - RABBITMQ_DEFAULT_USER=guest
      - RABBITMQ_DEFAULT_PASS=guest
    volumes:
      - rabbitmq-data:/var/lib/rabbitmq

volumes:
  redis-dashboard-data:
  redis-aggregation-data:
  redis-gitlab-data:
  redis-github-data:
  postgres-data:
  rabbitmq-data:
```

---

## Comparison: Monolith vs Microservices

### Monolith Architecture

**Pros**:
- ✅ Simple deployment (single binary)
- ✅ No network latency between components
- ✅ Easier to debug and trace
- ✅ Transactions are straightforward
- ✅ No distributed system complexity
- ✅ Lower operational overhead
- ✅ Faster development for small teams

**Cons**:
- ❌ Entire app must be deployed together
- ❌ Scaling is all-or-nothing
- ❌ Technology stack is uniform
- ❌ Larger codebase to understand
- ❌ Harder to isolate failures
- ❌ One service failure = entire app down

### Microservices Architecture

**Pros**:
- ✅ Independent deployment per service
- ✅ Targeted scaling (scale only what needs it)
- ✅ Technology diversity (Go, Node.js, Python per service)
- ✅ Team autonomy (teams own services)
- ✅ Failure isolation (one service down ≠ entire system down)
- ✅ Easier to add new platforms (just add new connector)
- ✅ Better CI/CD (deploy only changed services)

**Cons**:
- ❌ Distributed system complexity
- ❌ Network latency between services
- ❌ Harder to debug across services
- ❌ Data consistency challenges
- ❌ More operational overhead (monitoring, logging, tracing)
- ❌ Service discovery complexity
- ❌ Testing is more complex

---

### When to Use Each

**Use Monolith When**:
- Small team (< 5 developers)
- Early stage product (MVP, validation)
- Simple domain
- Low traffic (< 100 req/sec)
- Quick time-to-market priority
- Limited operational expertise

**Use Microservices When**:
- Large team (> 10 developers)
- Multiple teams working independently
- Need to scale different parts differently (GitHub connector needs 10x resources)
- Different platforms have different SLAs
- Want to isolate platform failures
- Need different deployment cadences
- Have DevOps/SRE expertise

---

## Migration Strategy

### Phase 1: Extract Connector Services (Strangler Pattern)

**Step 1**: Keep monolith, add GitLab Connector Service alongside

```
┌─────────────────────────────────┐
│        Monolith                  │
│  ┌──────────────────────┐       │      ┌─────────────────────┐
│  │  GitLab Client       │◄──────┼──────│ GitLab Connector    │
│  │  (Deprecated)        │       │      │     Service         │
│  └──────────────────────┘       │      │  (New)              │
│                                  │      └─────────────────────┘
│  ┌──────────────────────┐       │
│  │  GitHub Client       │       │
│  └──────────────────────┘       │
└─────────────────────────────────┘
```

- Deploy GitLab Connector as separate service
- Route 10% of GitLab requests to new service (feature flag)
- Monitor performance, errors
- Gradually increase to 100%
- Remove GitLab client from monolith

**Step 2**: Extract GitHub Connector Service

- Repeat same pattern for GitHub
- Now have 2 connector services + smaller monolith

**Step 3**: Extract Aggregation Service

- Create Aggregation Service
- Move orchestration logic from monolith
- Monolith becomes thin BFF

**Step 4**: Extract Webhook Service

- Move webhook handling to separate service
- Configure webhooks to point to new service

---

### Phase 2: Service-by-Service Extraction

Extract services one at a time, testing thoroughly between each:

```
Week 1-2:  GitLab Connector Service
Week 3-4:  GitHub Connector Service
Week 5-6:  Aggregation Service
Week 7-8:  Webhook Service
Week 9-10: User Service (if needed)
Week 11+:  Gradual decommission of monolith
```

---

### Phase 3: Data Migration

**Challenge**: Migrate from single cache to per-service caches

**Strategy**:
1. **Dual Write**: Write to both old and new caches
2. **Backfill**: Copy existing cache data to new service caches
3. **Switch Reads**: Read from new caches
4. **Remove Old**: Delete old cache

```go
// During migration
func (s *Service) GetProjects(ctx context.Context) ([]domain.Project, error) {
    // Read from new service (primary)
    projects, err := s.connectorClient.GetProjects(ctx)
    if err == nil {
        return projects, nil
    }

    // Fallback to old cache
    log.Printf("Connector service unavailable, using legacy cache")
    return s.legacyCache.GetProjects()
}
```

---

## Trade-offs and Considerations

### Operational Complexity

**Monolith**:
- 1 deployment
- 1 log stream
- 1 monitoring dashboard

**Microservices**:
- 6+ deployments
- 6+ log streams (need centralized logging: ELK, Loki)
- 6+ monitoring dashboards (need: Prometheus, Grafana)
- Distributed tracing required (Jaeger, Zipkin)

### Cost

**Monolith**:
- 1 server/container
- 1 cache instance
- ~$50-100/month (small instance)

**Microservices**:
- 6+ containers (at least 2 replicas each = 12+ containers)
- 4+ Redis instances
- 1 PostgreSQL
- 1 RabbitMQ
- 1 API Gateway
- ~$500-1000/month (small instances)

### Team Structure

**Monolith**: 1 team owns everything

**Microservices**: Teams per domain
- Platform Team: Connector services
- API Team: Aggregation, Dashboard services
- Infrastructure Team: Webhook, User services
- DevOps Team: Deployment, monitoring

### Development Velocity

**Monolith**:
- Faster for small changes (single codebase)
- Slower for large features (conflicts, coordination)

**Microservices**:
- Slower initially (setup, contracts)
- Faster long-term (parallel development, independent deploys)

---

## Observability

### Distributed Tracing

**OpenTelemetry Integration**:

```go
// dashboard-service/internal/handlers/repositories.go
func (h *Handler) GetRepositories(w http.ResponseWriter, r *http.Request) {
    ctx, span := tracer.Start(r.Context(), "dashboard.GetRepositories")
    defer span.End()

    // Call aggregation service
    repos, err := h.aggregationClient.GetRepositories(ctx)
    if err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, err.Error())
        return
    }

    span.SetAttributes(attribute.Int("repository.count", len(repos)))
}

// aggregation-service/internal/orchestrator/service.go
func (s *Service) GetRepositories(ctx context.Context) ([]domain.Project, error) {
    ctx, span := tracer.Start(ctx, "aggregation.GetRepositories")
    defer span.End()

    // Fan out to connectors
    var wg sync.WaitGroup

    // GitLab
    wg.Add(1)
    go func() {
        defer wg.Done()
        _, childSpan := tracer.Start(ctx, "aggregation.GetRepositories.gitlab")
        defer childSpan.End()

        repos, err := s.gitlabClient.GetProjects(ctx)
        // ...
    }()

    // GitHub
    wg.Add(1)
    go func() {
        defer wg.Done()
        _, childSpan := tracer.Start(ctx, "aggregation.GetRepositories.github")
        defer childSpan.End()

        repos, err := s.githubClient.GetProjects(ctx)
        // ...
    }()

    wg.Wait()
}
```

**Trace Example**:
```
dashboard.GetRepositories [200ms]
  └─ aggregation.GetRepositories [180ms]
       ├─ aggregation.GetRepositories.gitlab [150ms]
       │    └─ gitlab.GetProjects [145ms]
       │         └─ http.request [140ms]
       └─ aggregation.GetRepositories.github [170ms]
            └─ github.GetProjects [165ms]
                 └─ http.request [160ms]
```

### Logging

**Centralized Logging (ELK Stack)**:

```yaml
# filebeat.yml
filebeat.inputs:
  - type: container
    paths:
      - '/var/lib/docker/containers/*/*.log'

processors:
  - add_kubernetes_metadata:
      host: ${NODE_NAME}
      matchers:
      - logs_path:
          logs_path: "/var/lib/docker/containers/"

output.elasticsearch:
  hosts: ["elasticsearch:9200"]
  index: "ci-dashboard-%{+yyyy.MM.dd}"
```

**Structured Logging**:
```go
log.WithFields(log.Fields{
    "service":    "gitlab-connector",
    "trace_id":   traceID,
    "project_id": projectID,
    "duration":   duration,
}).Info("fetched project successfully")
```

### Metrics

**Prometheus Metrics**:

```go
var (
    httpRequestsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "http_requests_total",
            Help: "Total HTTP requests",
        },
        []string{"service", "method", "endpoint", "status"},
    )

    httpRequestDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "http_request_duration_seconds",
            Help:    "HTTP request duration",
            Buckets: prometheus.DefBuckets,
        },
        []string{"service", "method", "endpoint"},
    )

    cacheHitsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "cache_hits_total",
            Help: "Total cache hits",
        },
        []string{"service", "cache_key"},
    )
)

// Instrument handler
func (h *Handler) GetProjects(w http.ResponseWriter, r *http.Request) {
    start := time.Now()

    // ... handler logic ...

    duration := time.Since(start).Seconds()
    httpRequestDuration.WithLabelValues("gitlab-connector", r.Method, "/api/projects").Observe(duration)
    httpRequestsTotal.WithLabelValues("gitlab-connector", r.Method, "/api/projects", strconv.Itoa(statusCode)).Inc()
}
```

---

## Summary

### Recommended Approach

**For Most Teams**: Start with **Monolith**
- Simpler to build and operate
- Faster time to market
- Lower costs
- Can always migrate later

**When to Migrate to Microservices**:
- Team grows beyond 10 developers
- Different parts need different scaling
- Want to add many more platforms (Jenkins, CircleCI, Travis, etc.)
- Have dedicated DevOps/SRE team
- Platform failures are causing too much impact

### Hybrid Approach (Recommended)

Start with monolith, extract only connector services:

```
┌─────────────────────────────────┐
│      Main Application           │
│  ┌──────────────────────┐       │
│  │  Dashboard           │       │
│  │  Aggregation         │       │
│  │  Caching             │       │
│  └──────────┬───────────┘       │
└─────────────┼───────────────────┘
              │
    ┌─────────┼─────────┐
    │         │         │
    ▼         ▼         ▼
┌────────┐ ┌────────┐ ┌────────┐
│ GitLab │ │ GitHub │ │Jenkins │
│Connector│ │Connector│ │Connector│
└────────┘ └────────┘ └────────┘
```

**Benefits**:
- Easy to add new platforms (just deploy new connector)
- Platform isolation (GitLab issues don't affect GitHub)
- Targeted scaling (scale GitHub connector independently)
- Still relatively simple (4 deployments instead of 1)

This provides 80% of microservices benefits with 20% of the complexity.
