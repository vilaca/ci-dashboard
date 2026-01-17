package dashboard

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/vilaca/ci-dashboard/internal/domain"
	"github.com/vilaca/ci-dashboard/internal/service"
)

// Renderer handles rendering responses to HTTP clients.
// This interface follows Interface Segregation Principle (SOLID-I).
type Renderer interface {
	RenderIndex(w io.Writer) error
	RenderHealth(w io.Writer) error
	RenderPipelines(w io.Writer, pipelines []domain.Pipeline) error
	RenderPipelinesJSON(w io.Writer, pipelines []domain.Pipeline) error
	RenderPipelinesGrouped(w io.Writer, pipelinesByWorkflow map[string][]domain.Pipeline) error
	RenderRepositories(w io.Writer, repositories []service.RepositoryWithRuns) error
	RenderRecentPipelines(w io.Writer, pipelines []domain.Pipeline) error
	RenderRepositoryDetail(w io.Writer, repository service.RepositoryWithRuns) error
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
	var sb strings.Builder

	sb.WriteString(htmlHead("CI Dashboard", "Monitor CI/CD pipelines from GitLab and GitHub in one place"))
	sb.WriteString(pageCSS(`
		.card { background: var(--bg-secondary); padding: 20px; border-radius: 8px; box-shadow: 0 2px 4px var(--shadow); margin-bottom: 20px; transition: background-color 0.3s; }
		p { color: var(--text-secondary); }
	`))
	sb.WriteString(`<body>
	<div class="container">
		<h1>CI Dashboard</h1>
`)
	sb.WriteString(buildNavigation())
	sb.WriteString(`
		<div class="card">
			<h2>Welcome</h2>
			<p>Monitor your CI/CD pipelines from GitLab and GitHub in one place.</p>
			<p><a href="/pipelines">View Pipelines →</a></p>
		</div>
	</div>`)
	sb.WriteString(htmlFooter())

	_, err := w.Write([]byte(sb.String()))
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

	sb.WriteString(htmlHead("Pipelines", "View all CI/CD pipelines"))
	sb.WriteString(pageCSS(`
		.pipeline { background: var(--bg-secondary); padding: 20px; border-radius: 8px; box-shadow: 0 2px 4px var(--shadow); margin-bottom: 15px; transition: background-color 0.3s; }
		.pipeline-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px; }
		.pipeline-title { font-size: 18px; font-weight: 600; color: var(--text-primary); }
		.pipeline-info { color: var(--text-secondary); font-size: 14px; }
		.pipeline-link { color: var(--link-color); text-decoration: none; }
		.pipeline-link:hover { text-decoration: underline; }
		.pipeline-subtitle { font-size: 14px; color: var(--text-secondary); font-weight: 400; margin-top: 4px; }
		.refresh { margin-bottom: 20px; }
		.refresh button { padding: 8px 16px; background: var(--button-bg); color: white; border: none; border-radius: 4px; cursor: pointer; font-size: 14px; transition: background-color 0.3s; }
		.refresh button:hover { background: var(--button-hover); }
	`))
	sb.WriteString(`<body>
	<div class="container">
		<h1>CI/CD Pipelines</h1>
`)
	sb.WriteString(buildNavigation())
	sb.WriteString(`
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

	sb.WriteString(`	</div>`)
	sb.WriteString(htmlFooter())

	return sb.String()
}

// writePipelineCard writes a single pipeline card to the string builder.
func (r *HTMLRenderer) writePipelineCard(sb *strings.Builder, p domain.Pipeline) {
	statusClass := fmt.Sprintf("status status-%s", p.Status)

	// Show workflow name (GitHub) or repository (GitLab)
	title := escapeHTML(p.Repository)
	subtitle := ""
	if p.WorkflowName != nil && *p.WorkflowName != "" {
		title = escapeHTML(*p.WorkflowName)
		subtitle = fmt.Sprintf("<div class=\"pipeline-subtitle\">%s</div>", escapeHTML(p.Repository))
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
				%s
			</div>
		</div>
`, title, subtitle, statusClass, strings.ToUpper(string(p.Status)), escapeHTML(p.Branch), p.CreatedAt.Format("2006-01-02 15:04:05"), externalLink(p.WebURL, "View Pipeline →")))
}

// RenderPipelinesGrouped renders pipelines grouped by workflow.
func (r *HTMLRenderer) RenderPipelinesGrouped(w io.Writer, pipelinesByWorkflow map[string][]domain.Pipeline) error {
	var sb strings.Builder

	sb.WriteString(htmlHead("Grouped Pipelines", "View CI/CD pipelines grouped by workflow"))
	sb.WriteString(pageCSS(`
		.pipeline { background: var(--bg-secondary); padding: 20px; border-radius: 8px; box-shadow: 0 2px 4px var(--shadow); margin-bottom: 15px; transition: background-color 0.3s; }
		.pipeline-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px; }
		.pipeline-title { font-size: 18px; font-weight: 600; color: var(--text-primary); }
		.pipeline-info { color: var(--text-secondary); font-size: 14px; }
		.pipeline-link { color: var(--link-color); text-decoration: none; }
		.pipeline-link:hover { text-decoration: underline; }
		.pipeline-subtitle { font-size: 14px; color: var(--text-secondary); font-weight: 400; margin-top: 4px; }
		.workflow-group { margin-bottom: 30px; }
		.workflow-group-header {
			font-size: 20px;
			font-weight: 600;
			color: var(--text-primary);
			margin-bottom: 15px;
			padding-bottom: 10px;
			border-bottom: 2px solid var(--border-color);
		}
	`))
	sb.WriteString(`<body>
	<div class="container">
		<h1>CI/CD Pipelines</h1>
`)
	sb.WriteString(buildNavigation())
	sb.WriteString("\n")

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

	sb.WriteString(`	</div>`)
	sb.WriteString(htmlFooter())

	_, err := w.Write([]byte(sb.String()))
	return err
}

