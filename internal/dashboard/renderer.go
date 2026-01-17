package dashboard

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/vilaca/ci-dashboard/internal/domain"
)

// Renderer handles rendering responses to HTTP clients.
// This interface follows Interface Segregation Principle (SOLID-I).
type Renderer interface {
	RenderIndex(w io.Writer) error
	RenderHealth(w io.Writer) error
	RenderPipelines(w io.Writer, pipelines []domain.Pipeline) error
	RenderPipelinesJSON(w io.Writer, pipelines []domain.Pipeline) error
	RenderPipelinesGrouped(w io.Writer, pipelinesByWorkflow map[string][]domain.Pipeline) error
}

// HTMLRenderer implements Renderer for HTML responses.
type HTMLRenderer struct {
	// All HTML is embedded in methods, no external templates needed
}

// NewHTMLRenderer creates a new HTML renderer.
func NewHTMLRenderer() *HTMLRenderer {
	return &HTMLRenderer{}
}

func (r *HTMLRenderer) RenderIndex(w io.Writer) error {
	_, err := w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
	<title>CI Dashboard</title>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<style>
		body { font-family: system-ui, -apple-system, sans-serif; margin: 0; padding: 20px; background: #f5f5f5; }
		.container { max-width: 1200px; margin: 0 auto; }
		h1 { color: #333; }
		.card { background: white; padding: 20px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); margin-bottom: 20px; }
		.nav { margin-bottom: 30px; }
		.nav a { color: #0066cc; text-decoration: none; margin-right: 20px; }
		.nav a:hover { text-decoration: underline; }
	</style>
</head>
<body>
	<div class="container">
		<h1>CI Dashboard</h1>
		<div class="nav">
			<a href="/">Home</a>
			<a href="/pipelines">Pipelines</a>
			<a href="/api/pipelines">API (JSON)</a>
		</div>
		<div class="card">
			<h2>Welcome</h2>
			<p>Monitor your CI/CD pipelines from GitLab and GitHub in one place.</p>
			<p><a href="/pipelines">View Pipelines →</a></p>
		</div>
	</div>
</body>
</html>`))
	return err
}

func (r *HTMLRenderer) RenderHealth(w io.Writer) error {
	_, err := w.Write([]byte(`{"status":"ok"}`))
	return err
}

func (r *HTMLRenderer) RenderPipelines(w io.Writer, pipelines []domain.Pipeline) error {
	html := r.buildPipelinesHTML(pipelines)
	_, err := w.Write([]byte(html))
	return err
}

func (r *HTMLRenderer) RenderPipelinesJSON(w io.Writer, pipelines []domain.Pipeline) error {
	return json.NewEncoder(w).Encode(map[string]interface{}{
		"pipelines": pipelines,
		"count":     len(pipelines),
	})
}

// buildPipelinesHTML constructs the HTML for displaying pipelines.
// Follows SLAP - operates at single level of abstraction.
func (r *HTMLRenderer) buildPipelinesHTML(pipelines []domain.Pipeline) string {
	var sb strings.Builder

	sb.WriteString(`<!DOCTYPE html>
<html>
<head>
	<title>Pipelines - CI Dashboard</title>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<style>
		body { font-family: system-ui, -apple-system, sans-serif; margin: 0; padding: 20px; background: #f5f5f5; }
		.container { max-width: 1200px; margin: 0 auto; }
		h1 { color: #333; }
		.nav { margin-bottom: 30px; }
		.nav a { color: #0066cc; text-decoration: none; margin-right: 20px; }
		.nav a:hover { text-decoration: underline; }
		.pipeline { background: white; padding: 20px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); margin-bottom: 15px; }
		.pipeline-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px; }
		.pipeline-title { font-size: 18px; font-weight: 600; color: #333; }
		.status { padding: 6px 12px; border-radius: 4px; font-size: 14px; font-weight: 500; }
		.status-success { background: #d4edda; color: #155724; }
		.status-failed { background: #f8d7da; color: #721c24; }
		.status-running { background: #d1ecf1; color: #0c5460; }
		.status-pending { background: #fff3cd; color: #856404; }
		.status-canceled { background: #e2e3e5; color: #383d41; }
		.pipeline-info { color: #666; font-size: 14px; }
		.pipeline-link { color: #0066cc; text-decoration: none; }
		.pipeline-link:hover { text-decoration: underline; }
		.pipeline-subtitle { font-size: 14px; color: #666; font-weight: 400; margin-top: 4px; }
		.empty { text-align: center; padding: 40px; color: #666; }
		.refresh { margin-bottom: 20px; }
		.refresh button { padding: 10px 20px; background: #0066cc; color: white; border: none; border-radius: 4px; cursor: pointer; }
		.refresh button:hover { background: #0052a3; }
	</style>
</head>
<body>
	<div class="container">
		<h1>CI/CD Pipelines</h1>
		<div class="nav">
			<a href="/">Home</a>
			<a href="/pipelines">Pipelines</a>
			<a href="/api/pipelines">API (JSON)</a>
		</div>
		<div class="refresh">
			<button onclick="location.reload()">Refresh</button>
		</div>
`)

	if len(pipelines) == 0 {
		sb.WriteString(`		<div class="pipeline">
			<div class="empty">No pipelines found. Configure GITLAB_TOKEN or GITHUB_TOKEN environment variables.</div>
		</div>
`)
	} else {
		for _, p := range pipelines {
			r.writePipelineCard(&sb, p)
		}
	}

	sb.WriteString(`	</div>
</body>
</html>`)

	return sb.String()
}

// writePipelineCard writes a single pipeline card to the string builder.
func (r *HTMLRenderer) writePipelineCard(sb *strings.Builder, p domain.Pipeline) {
	statusClass := fmt.Sprintf("status status-%s", p.Status)

	// Show workflow name (GitHub) or repository (GitLab)
	title := p.Repository
	subtitle := ""
	if p.WorkflowName != nil && *p.WorkflowName != "" {
		title = *p.WorkflowName
		subtitle = fmt.Sprintf("<div class=\"pipeline-subtitle\">%s</div>", p.Repository)
	}

	sb.WriteString(fmt.Sprintf(`		<div class="pipeline">
			<div class="pipeline-header">
				<div>
					<div class="pipeline-title">%s</div>
					%s
				</div>
				<span class="%s">%s</span>
			</div>
			<div class="pipeline-info">
				<strong>Branch:</strong> %s<br>
				<strong>Created:</strong> %s<br>
				<a href="%s" target="_blank" class="pipeline-link">View Pipeline →</a>
			</div>
		</div>
`, title, subtitle, statusClass, strings.ToUpper(string(p.Status)), p.Branch, p.CreatedAt.Format("2006-01-02 15:04:05"), p.WebURL))
}

// RenderPipelinesGrouped renders pipelines grouped by workflow.
func (r *HTMLRenderer) RenderPipelinesGrouped(w io.Writer, pipelinesByWorkflow map[string][]domain.Pipeline) error {
	var sb strings.Builder

	// Build HTML header with group styles
	sb.WriteString(`<!DOCTYPE html>
<html>
<head>
	<title>Pipelines - CI Dashboard</title>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<style>
		body { font-family: system-ui, -apple-system, sans-serif; margin: 0; padding: 20px; background: #f5f5f5; }
		.container { max-width: 1200px; margin: 0 auto; }
		h1 { color: #333; }
		.nav { margin-bottom: 30px; }
		.nav a { color: #0066cc; text-decoration: none; margin-right: 20px; }
		.nav a:hover { text-decoration: underline; }
		.pipeline { background: white; padding: 20px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); margin-bottom: 15px; }
		.pipeline-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px; }
		.pipeline-title { font-size: 18px; font-weight: 600; color: #333; }
		.status { padding: 6px 12px; border-radius: 4px; font-size: 14px; font-weight: 500; }
		.status-success { background: #d4edda; color: #155724; }
		.status-failed { background: #f8d7da; color: #721c24; }
		.status-running { background: #d1ecf1; color: #0c5460; }
		.status-pending { background: #fff3cd; color: #856404; }
		.status-canceled { background: #e2e3e5; color: #383d41; }
		.pipeline-info { color: #666; font-size: 14px; }
		.pipeline-link { color: #0066cc; text-decoration: none; }
		.pipeline-link:hover { text-decoration: underline; }
		.pipeline-subtitle { font-size: 14px; color: #666; font-weight: 400; margin-top: 4px; }
		.workflow-group { margin-bottom: 30px; }
		.workflow-group-header {
			font-size: 20px;
			font-weight: 600;
			color: #333;
			margin-bottom: 15px;
			padding-bottom: 10px;
			border-bottom: 2px solid #e0e0e0;
		}
	</style>
</head>
<body>
	<div class="container">
		<h1>CI/CD Pipelines</h1>
		<div class="nav">
			<a href="/">Home</a>
			<a href="/pipelines">All Pipelines</a>
			<a href="/pipelines/grouped">Grouped by Workflow</a>
		</div>
`)

	// Sort workflow names for consistent display
	workflowNames := make([]string, 0, len(pipelinesByWorkflow))
	for name := range pipelinesByWorkflow {
		workflowNames = append(workflowNames, name)
	}

	// Simple sorting - empty string goes last
	sortedNames := []string{}
	emptyName := ""
	hasEmpty := false
	for _, name := range workflowNames {
		if name == "" {
			hasEmpty = true
			continue
		}
		sortedNames = append(sortedNames, name)
	}
	// Basic bubble sort for simplicity
	for i := 0; i < len(sortedNames)-1; i++ {
		for j := 0; j < len(sortedNames)-i-1; j++ {
			if sortedNames[j] > sortedNames[j+1] {
				sortedNames[j], sortedNames[j+1] = sortedNames[j+1], sortedNames[j]
			}
		}
	}
	if hasEmpty {
		sortedNames = append(sortedNames, emptyName)
	}

	// Render each workflow group
	for _, workflowName := range sortedNames {
		pipelines := pipelinesByWorkflow[workflowName]

		groupTitle := workflowName
		if groupTitle == "" {
			groupTitle = "Other Pipelines"
		}

		sb.WriteString(fmt.Sprintf(`
		<div class="workflow-group">
			<div class="workflow-group-header">%s</div>
`, groupTitle))

		for _, p := range pipelines {
			r.writePipelineCard(&sb, p)
		}

		sb.WriteString(`		</div>
`)
	}

	sb.WriteString(`	</div>
</body>
</html>`)

	_, err := w.Write([]byte(sb.String()))
	return err
}

