# Design Principles Reference

This document explains how the CI Dashboard codebase implements core software engineering principles.

## Principles Applied

### 1. DRY (Don't Repeat Yourself)
- Renderer logic extracted into separate interface/implementation
- Common logging behavior abstracted into `Logger` interface
- Test mocks defined once and reused across test cases

### 2. SOLID Principles

#### Single Responsibility Principle (SRP)
- `Handler`: Only handles HTTP requests
- `Renderer`: Only handles rendering output
- `Logger`: Only handles logging
- `Config`: Only holds configuration data

#### Open/Closed Principle
- New renderers can be added by implementing `Renderer` interface
- New loggers can be added by implementing `Logger` interface
- No modification to `Handler` needed to support new implementations

#### Liskov Substitution Principle
- Any `Renderer` implementation can replace `HTMLRenderer`
- Any `Logger` implementation can replace `StdLogger`
- Tests use mock implementations that are fully substitutable

#### Interface Segregation Principle
- `Renderer` interface contains only rendering methods
- `Logger` interface contains only logging methods
- Small, focused interfaces that do one thing well

#### Dependency Inversion Principle
- `Handler` depends on `Renderer` and `Logger` interfaces, not concrete types
- High-level modules don't depend on low-level modules
- Both depend on abstractions (interfaces)

### 3. KISS (Keep It Simple, Stupid)
- Simple, clear function names
- Minimal abstractions - only what's needed
- Standard library used where possible (no unnecessary frameworks)
- Straightforward control flow

### 4. SoC (Separation of Concerns)
- **HTTP handling**: `handler.go` - manages HTTP requests/responses
- **Rendering**: `renderer.go` - generates output content
- **Logging**: Abstracted via `Logger` interface
- **Configuration**: `config.go` - separate package for config management
- **Composition**: `main.go` - wires everything together

### 5. IoC (Inversion of Control)
- Dependencies are injected via constructors
- `buildServer()` in `main.go` is the composition root
- Components don't create their own dependencies
- Makes testing easier with mock implementations

### 6. SLAP (Single Level of Abstraction Principle)
```go
// main() operates at high level
func main() {
    cfg, err := config.Load()        // Load config
    server := buildServer(cfg)       // Build server
    http.ListenAndServe(addr, server) // Start server
}

// buildServer() operates at medium level
func buildServer(cfg *config.Config) http.Handler {
    logger := dashboard.NewStdLogger()     // Create logger
    renderer := dashboard.NewHTMLRenderer() // Create renderer
    handler := dashboard.NewHandler(...)    // Create handler
    // ...
}

// Each handler method operates at its own level
func (h *Handler) handleIndex(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html")
    if err := h.renderer.RenderIndex(w); err != nil {
        h.logger.Printf("failed to render index: %v", err)
        http.Error(w, "Internal Server Error", http.StatusInternalServerError)
    }
}
```

### 7. POLA/POLS (Principle of Least Astonishment)
- Function names clearly describe what they do
- `NewHandler()` creates a new handler (expected)
- `RegisterRoutes()` registers routes (expected)
- Error returns behave as expected in Go
- No surprising side effects or hidden behaviors

### 8. Law of Demeter (LoD)
Also known as "Don't Talk to Strangers" or "Principle of Least Knowledge"

**Rule**: A method should only call methods on:
- Itself
- Objects passed as parameters
- Objects it creates
- Its direct dependencies (fields)

**Applied in codebase**:
```go
// ✅ GOOD - Follows Law of Demeter
func (h *Handler) handlePipelines(w http.ResponseWriter, r *http.Request) {
    // Calls method on direct dependency (pipelineService)
    pipelines, err := h.pipelineService.GetLatestPipelines(r.Context())
    if err != nil {
        // Calls method on direct dependency (logger)
        h.logger.Printf("failed to get pipelines: %v", err)
        return
    }

    // Calls method on direct dependency (renderer)
    h.renderer.RenderPipelines(w, pipelines)
}

// ❌ BAD - Violates Law of Demeter (chaining)
func (h *Handler) handlePipelinesBAD(w http.ResponseWriter, r *http.Request) {
    // Don't do this! Reaching through multiple objects
    html := h.pipelineService.GetRenderer().GetTemplate().Execute(...)
}
```

**Benefits**:
- Reduces coupling between components
- Makes code easier to refactor
- Limits knowledge of system structure
- Each component only knows about its immediate collaborators

### 9. High Cohesion / Low Coupling

**High Cohesion**: Elements within a component are strongly related and focused on a single purpose.

**Low Coupling**: Components have minimal dependencies on each other.

**Applied in codebase**:

#### High Cohesion Examples
```go
// ✅ GitLabClient - All methods related to GitLab API
type GitLabClient struct {
    GetProjects()       // GitLab-specific
    GetLatestPipeline() // GitLab-specific
    GetPipelines()      // GitLab-specific
    doRequest()         // GitLab-specific helper
    convertPipeline()   // GitLab-specific conversion
}

// ✅ PipelineService - All methods related to pipeline orchestration
type PipelineService struct {
    GetAllProjects()       // Orchestration
    GetPipelinesByProject() // Orchestration
    GetLatestPipelines()   // Orchestration
    RegisterClient()       // Client management
}

// ❌ BAD - Low cohesion (unrelated responsibilities)
type BadService struct {
    GetPipelines()    // Pipeline stuff
    SendEmail()       // Email stuff (unrelated!)
    LogMetrics()      // Logging stuff (unrelated!)
    UpdateDatabase()  // Database stuff (unrelated!)
}
```

#### Low Coupling Examples
```go
// ✅ Handler depends ONLY on interfaces (minimal coupling)
type Handler struct {
    renderer        Renderer         // Interface - can be swapped
    logger          Logger           // Interface - can be swapped
    pipelineService PipelineService  // Interface - can be swapped
}

// ✅ PipelineService depends ONLY on Client interface
type PipelineService struct {
    clients map[string]api.Client  // Interface - any implementation works
}

// ❌ BAD - High coupling (depends on concrete types)
type BadHandler struct {
    htmlRenderer   *HTMLRenderer        // Concrete type - tightly coupled
    stdLogger      *log.Logger          // Concrete type - tightly coupled
    gitlabClient   *gitlab.Client       // Concrete type - tightly coupled
    githubClient   *github.Client       // Concrete type - tightly coupled
}
```

**Benefits**:
- **High Cohesion**:
  - Easier to understand (related code is together)
  - Easier to maintain (changes are localized)
  - Easier to test (focused scope)

- **Low Coupling**:
  - Components can be changed independently
  - Easier to test (mock dependencies)
  - Easier to reuse components
  - Reduces ripple effects of changes

**Measuring in our codebase**:
- Each package has single, clear purpose (high cohesion)
- Components communicate through interfaces (low coupling)
- No package directly imports concrete implementations from other packages
- Composition root (`main.go`) is the only place that knows about all concrete types

## Testing Principles

### FIRST Principles

#### Fast
- Tests use in-memory mocks (no I/O, no network, no database)
- Tests complete in milliseconds
- No external dependencies required

#### Independent
- Each test is self-contained
- Tests don't share state
- Tests can run in any order
- Mock objects created fresh for each test

#### Repeatable
- No reliance on external state
- No randomness or time-based logic without mocking
- Same input always produces same output
- Environment variables cleaned up after tests

#### Self-validating
- Tests return clear pass/fail
- Descriptive error messages
- No manual verification needed

#### Timely
- Tests written alongside production code
- Each component has corresponding test file
- Test coverage maintained as code evolves

### AAA (Arrange-Act-Assert)

All tests follow this pattern with explicit comments:

```go
func TestHandleHealth(t *testing.T) {
    // Arrange - Set up test dependencies and inputs
    renderer := &mockRenderer{}
    logger := &mockLogger{}
    handler := NewHandler(renderer, logger)

    // Act - Execute the code being tested
    mux.ServeHTTP(w, req)

    // Assert - Verify the results
    if w.Code != http.StatusOK {
        t.Errorf("expected status 200, got %d", w.Code)
    }
}
```

## Code Examples

### Before Refactoring (Violations)
```go
// ❌ Violates SRP, DIP, SoC, IoC
type Handler struct {
    config *config.Config
    mux    *http.ServeMux  // Creates own dependency
}

func NewHandler(cfg *config.Config) *Handler {
    h := &Handler{
        config: cfg,
        mux:    http.NewServeMux(), // ❌ Creates dependency internally
    }
    h.registerRoutes()
    return h
}

// ❌ Violates SoC - HTML mixed with handler logic
func (h *Handler) handleIndex(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html")
    w.Write([]byte(`<!DOCTYPE html>...`)) // ❌ Rendering logic in handler
}
```

### After Refactoring (Correct)
```go
// ✅ Follows SRP, DIP, SoC, IoC
type Handler struct {
    renderer Renderer  // ✅ Depends on interface
    logger   Logger    // ✅ Depends on interface
}

// ✅ Dependencies injected via constructor (IoC)
func NewHandler(renderer Renderer, logger Logger) *Handler {
    return &Handler{
        renderer: renderer,
        logger:   logger,
    }
}

// ✅ Separated concerns - delegates to renderer
func (h *Handler) handleIndex(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "text/html")
    if err := h.renderer.RenderIndex(w); err != nil {  // ✅ Delegates rendering
        h.logger.Printf("failed to render index: %v", err)
        http.Error(w, "Internal Server Error", http.StatusInternalServerError)
    }
}

// ✅ Renderer handles its own concern
type Renderer interface {
    RenderIndex(w io.Writer) error
}
```

## Benefits Achieved

1. **Testability**: Easy to test with mock implementations
2. **Maintainability**: Clear separation makes changes easier
3. **Flexibility**: Swap implementations without changing consumers
4. **Readability**: Code intent is clear and self-documenting
5. **Extensibility**: Easy to add new features following same patterns
6. **Reliability**: Comprehensive tests ensure correctness
7. **Simplicity**: Each component has one clear purpose
