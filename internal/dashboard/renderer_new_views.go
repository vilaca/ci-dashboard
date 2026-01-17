package dashboard

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/vilaca/ci-dashboard/internal/domain"
	"github.com/vilaca/ci-dashboard/internal/service"
)

// RenderRepositories renders repository cards with recent runs.
func (r *HTMLRenderer) RenderRepositories(w io.Writer, repositories []service.RepositoryWithRuns) error {
	var sb strings.Builder

	sb.WriteString(htmlHead("Repositories", "View all CI/CD repositories and their recent pipeline runs"))
	sb.WriteString(pageCSS(`
		.repo-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(400px, 1fr)); gap: 20px; }
		.repo-card { background: var(--bg-secondary); padding: 20px; border-radius: 8px; box-shadow: 0 2px 4px var(--shadow); transition: background-color 0.3s; }
		.repo-header { border-bottom: 2px solid var(--border-color); padding-bottom: 10px; margin-bottom: 15px; }
		.repo-title { font-size: 20px; font-weight: 600; margin-bottom: 5px; }
		.repo-title a { color: var(--text-primary); text-decoration: none; }
		.repo-title a:hover { color: var(--link-color); text-decoration: underline; }
		.repo-platform { font-size: 12px; color: var(--text-secondary); text-transform: uppercase; }
		.runs-list { list-style: none; padding: 0; margin: 0; }
		.run-item { padding: 12px; margin-bottom: 8px; border-left: 4px solid var(--border-color); background: var(--bg-primary); border-radius: 4px; transition: background-color 0.3s; }
		.run-item:hover { opacity: 0.9; }
		.run-item.success { border-left-color: #28a745; }
		.run-item.failed { border-left-color: #dc3545; }
		.run-item.running { border-left-color: #17a2b8; }
		.run-item.pending { border-left-color: #ffc107; }
		.run-item.canceled { border-left-color: #6c757d; }
		.run-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 5px; }
		.run-name { font-weight: 500; color: var(--text-primary); }
		.run-meta { font-size: 13px; color: var(--text-secondary); }
		.run-link { color: var(--link-color); text-decoration: none; font-size: 13px; }
		.run-link:hover { text-decoration: underline; }
	`))
	sb.WriteString(`<body>
	<div class="container">
		<h1>Repositories</h1>
`)
	sb.WriteString(buildNavigation())
	sb.WriteString("\n")

	if len(repositories) == 0 {
		sb.WriteString(`		<div class="empty">No repositories found. Configure GITLAB_TOKEN or GITHUB_TOKEN environment variables.</div>`)
	} else {
		sb.WriteString(`		<div class="repo-grid">
`)
		for _, repo := range repositories {
			r.writeRepositoryCard(&sb, repo)
		}
		sb.WriteString(`		</div>
`)
	}

	sb.WriteString(`	</div>`)
	sb.WriteString(htmlFooter())

	_, err := w.Write([]byte(sb.String()))
	return err
}

// writeRepositoryCard writes a single repository card to the string builder.
func (r *HTMLRenderer) writeRepositoryCard(sb *strings.Builder, repo service.RepositoryWithRuns) {
	sb.WriteString(fmt.Sprintf(`			<div class="repo-card">
				<div class="repo-header">
					<div class="repo-title"><a href="/repository?id=%s">%s</a></div>
					<div class="repo-platform">%s</div>
				</div>
`, repo.Project.ID, repo.Project.Name, repo.Project.Platform))

	if len(repo.Runs) == 0 {
		sb.WriteString(`				<div class="empty" style="padding: 20px;">No recent runs</div>
`)
	} else {
		sb.WriteString(`				<ul class="runs-list">
`)
		for _, run := range repo.Runs {
			r.writeRunItem(sb, run)
		}
		sb.WriteString(`				</ul>
`)
	}

	sb.WriteString(`			</div>
`)
}

// writeRunItem writes a single run item to the string builder.
func (r *HTMLRenderer) writeRunItem(sb *strings.Builder, run domain.Pipeline) {
	// Get workflow name or use repository name
	name := run.Repository
	if run.WorkflowName != nil && *run.WorkflowName != "" {
		name = *run.WorkflowName
	}

	statusClass := strings.ToLower(string(run.Status))

	sb.WriteString(fmt.Sprintf(`					<li class="run-item %s">
						<div class="run-header">
							<span class="run-name">%s</span>
							<span class="run-status %s">%s</span>
						</div>
						<div class="run-meta">
							<div>⏱️ %s | ⏰ %s</div>
							<a href="%s" target="_blank" rel="noopener noreferrer" class="run-link">View Details →</a>
						</div>
					</li>
`, statusClass, name, statusClass, strings.ToUpper(string(run.Status)),
		formatDuration(run.Duration),
		formatTimeAgo(run.UpdatedAt),
		run.WebURL))
}

// RenderRecentPipelines renders a list of recent pipelines across all repositories.
func (r *HTMLRenderer) RenderRecentPipelines(w io.Writer, pipelines []domain.Pipeline) error {
	var sb strings.Builder

	sb.WriteString(htmlHead("Recent Pipelines", "View the most recent CI/CD pipeline runs across all repositories"))
	sb.WriteString(pageCSS(`
		.pipelines-table { width: 100%; background: var(--bg-secondary); border-radius: 8px; box-shadow: 0 2px 4px var(--shadow); overflow: hidden; transition: background-color 0.3s; }
		.pipelines-table table { width: 100%; border-collapse: collapse; }
		.pipelines-table th { background: var(--bg-primary); padding: 15px; text-align: left; font-weight: 600; color: var(--text-primary); border-bottom: 2px solid var(--border-color); }
		.pipelines-table td { padding: 15px; border-bottom: 1px solid var(--border-color); color: var(--text-primary); }
		.pipelines-table tr:hover { background: var(--bg-primary); }
		.pipeline-name { font-weight: 500; }
		.pipeline-repo { font-size: 13px; color: var(--text-secondary); }
		.repo-link { color: var(--text-primary); text-decoration: none; }
		.repo-link:hover { color: var(--link-color); text-decoration: underline; }
		.link { color: var(--link-color); text-decoration: none; }
		.link:hover { text-decoration: underline; }
		@media (max-width: 768px) {
			.pipelines-table { overflow-x: auto; }
			.pipelines-table th, .pipelines-table td { padding: 10px; font-size: 14px; }
		}
	`))
	sb.WriteString(`<body>
	<div class="container">
		<h1>Recent Pipelines</h1>
`)
	sb.WriteString(buildNavigation())
	sb.WriteString("\n")

	if len(pipelines) == 0 {
		sb.WriteString(`		<div class="empty">No pipelines found. Configure GITLAB_TOKEN or GITHUB_TOKEN environment variables.</div>`)
	} else {
		sb.WriteString(`		<div class="pipelines-table">
			<table>
				<thead>
					<tr>
						<th>Repository</th>
						<th>Name</th>
						<th>Duration</th>
						<th>Time</th>
						<th>Actions</th>
						<th>Status</th>
					</tr>
				</thead>
				<tbody>
`)
		for _, pipeline := range pipelines {
			r.writePipelineRow(&sb, pipeline)
		}
		sb.WriteString(`				</tbody>
			</table>
		</div>
`)
	}

	sb.WriteString(`	</div>`)
	sb.WriteString(htmlFooter())

	_, err := w.Write([]byte(sb.String()))
	return err
}

// writePipelineRow writes a single pipeline row to the string builder.
func (r *HTMLRenderer) writePipelineRow(sb *strings.Builder, pipeline domain.Pipeline) {
	// Get workflow name or use repository name
	name := pipeline.Repository
	if pipeline.WorkflowName != nil && *pipeline.WorkflowName != "" {
		name = *pipeline.WorkflowName
	}

	statusClass := strings.ToLower(string(pipeline.Status))

	sb.WriteString(fmt.Sprintf(`					<tr>
						<td class="meta-text"><a href="/repository?id=%s" class="repo-link">%s</a></td>
						<td>
							<div class="pipeline-name">%s</div>
							<div class="pipeline-repo">%s</div>
						</td>
						<td class="meta-text">%s</td>
						<td class="meta-text">%s</td>
						<td><a href="%s" target="_blank" rel="noopener noreferrer" class="link">View →</a></td>
						<td><span class="status-badge %s">%s</span></td>
					</tr>
`, pipeline.ProjectID, pipeline.Repository, name, pipeline.Branch,
		formatDuration(pipeline.Duration),
		formatTimeAgo(pipeline.UpdatedAt),
		pipeline.WebURL, statusClass, strings.ToUpper(string(pipeline.Status))))
}

// formatDuration formats a duration in a human-readable format.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh %dm", int(d.Hours()), int(d.Minutes())%60)
}

// formatTimeAgo formats a time as "X ago" format.
func formatTimeAgo(t time.Time) string {
	duration := time.Since(t)

	if duration < time.Minute {
		return "just now"
	}
	if duration < time.Hour {
		mins := int(duration.Minutes())
		if mins == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d mins ago", mins)
	}
	if duration < 24*time.Hour {
		hours := int(duration.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	}
	days := int(duration.Hours() / 24)
	if days == 1 {
		return "1 day ago"
	}
	if days < 7 {
		return fmt.Sprintf("%d days ago", days)
	}
	return t.Format("Jan 2, 2006")
}

// RenderRepositoryDetail renders the detail page for a single repository.
func (r *HTMLRenderer) RenderRepositoryDetail(w io.Writer, repository service.RepositoryWithRuns) error {
	var sb strings.Builder

	// Calculate statistics
	totalRuns := len(repository.Runs)
	successCount := 0
	failedCount := 0
	var totalDuration time.Duration

	for _, run := range repository.Runs {
		if run.Status == domain.StatusSuccess {
			successCount++
		} else if run.Status == domain.StatusFailed {
			failedCount++
		}
		totalDuration += run.Duration
	}

	var avgDuration time.Duration
	if totalRuns > 0 {
		avgDuration = totalDuration / time.Duration(totalRuns)
	}

	successRate := 0.0
	if totalRuns > 0 {
		successRate = float64(successCount) / float64(totalRuns) * 100
	}

	description := fmt.Sprintf("View pipeline runs and statistics for %s", repository.Project.Name)
	sb.WriteString(htmlHead(repository.Project.Name, description))
	sb.WriteString(pageCSS(`
		.repo-url { color: var(--text-secondary); font-size: 14px; margin-bottom: 30px; }
		.repo-url a { color: var(--link-color); text-decoration: none; }
		.repo-url a:hover { text-decoration: underline; }
		.stats-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 20px; margin-bottom: 30px; }
		.stat-card { background: var(--bg-secondary); padding: 20px; border-radius: 8px; box-shadow: 0 2px 4px var(--shadow); transition: background-color 0.3s; }
		.stat-label { font-size: 14px; color: var(--text-secondary); margin-bottom: 8px; }
		.stat-value { font-size: 28px; font-weight: 600; color: var(--text-primary); }
		.runs-section { background: var(--bg-secondary); border-radius: 8px; box-shadow: 0 2px 4px var(--shadow); padding: 20px; transition: background-color 0.3s; }
		.runs-section h2 { margin-top: 0; color: var(--text-primary); }
		.run-item { padding: 15px; border-bottom: 1px solid var(--border-color); display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 10px; }
		.run-item:last-child { border-bottom: none; }
		.run-item:hover { background: var(--bg-primary); }
		.run-left { flex: 1; min-width: 200px; }
		.run-name { font-weight: 500; color: var(--text-primary); margin-bottom: 5px; }
		.run-branch { font-size: 13px; color: var(--text-secondary); }
		.run-meta { display: flex; gap: 15px; align-items: center; flex-wrap: wrap; font-size: 14px; color: var(--text-secondary); }
		.run-link { color: var(--link-color); text-decoration: none; font-size: 14px; }
		.run-link:hover { text-decoration: underline; }
		@media (max-width: 768px) {
			.run-item { flex-direction: column; align-items: flex-start; }
			.run-meta { flex-direction: column; align-items: flex-start; gap: 8px; }
		}
	`))
	sb.WriteString(fmt.Sprintf(`<body>
	<div class="container">
		<h1>%s</h1>
		<div class="repo-url">
			%s
		</div>
`, repository.Project.Name, externalLink(repository.Project.WebURL, repository.Project.WebURL+" →")))

	// Navigation
	sb.WriteString(`		<div class="nav">
			<a href="/">← Back to Repositories</a>
			<a href="/pipelines">Recent Pipelines</a>
			<a href="/api/pipelines">API (JSON)</a>
			<button class="theme-toggle" onclick="toggleTheme()" aria-label="Toggle theme">🌙 Dark Mode</button>
		</div>

		<div class="stats-grid">
			<div class="stat-card">
				<div class="stat-label">Total Runs</div>
				<div class="stat-value">` + fmt.Sprintf("%d", totalRuns) + `</div>
			</div>
			<div class="stat-card">
				<div class="stat-label">Success Rate</div>
				<div class="stat-value">` + fmt.Sprintf("%.1f%%", successRate) + `</div>
			</div>
			<div class="stat-card">
				<div class="stat-label">Successful</div>
				<div class="stat-value">` + fmt.Sprintf("%d", successCount) + `</div>
			</div>
			<div class="stat-card">
				<div class="stat-label">Failed</div>
				<div class="stat-value">` + fmt.Sprintf("%d", failedCount) + `</div>
			</div>
			<div class="stat-card">
				<div class="stat-label">Average Duration</div>
				<div class="stat-value" style="font-size: 20px;">` + formatDuration(avgDuration) + `</div>
			</div>
		</div>

		<div class="runs-section">
			<h2>Recent Runs</h2>
`)

	if len(repository.Runs) == 0 {
		sb.WriteString(`			<p style="color: var(--text-secondary); text-align: center; padding: 40px 0;">No runs found for this repository.</p>`)
	} else {
		for _, run := range repository.Runs {
			r.writeRepositoryDetailRun(&sb, run)
		}
	}

	sb.WriteString(`		</div>
	</div>`)
	sb.WriteString(htmlFooter())

	_, err := w.Write([]byte(sb.String()))
	return err
}

// writeRepositoryDetailRun writes a single run item for the repository detail page.
func (r *HTMLRenderer) writeRepositoryDetailRun(sb *strings.Builder, run domain.Pipeline) {
	name := run.Repository
	if run.WorkflowName != nil && *run.WorkflowName != "" {
		name = *run.WorkflowName
	}

	statusClass := strings.ToLower(string(run.Status))

	sb.WriteString(fmt.Sprintf(`			<div class="run-item">
				<div class="run-left">
					<div class="run-name">%s</div>
					<div class="run-branch">Branch: %s</div>
				</div>
				<div class="run-meta">
					<span>⏱️ %s</span>
					<span>⏰ %s</span>
					%s
					<span class="status-badge %s">%s</span>
				</div>
			</div>
`, name, run.Branch,
		formatDuration(run.Duration),
		formatTimeAgo(run.UpdatedAt),
		externalLink(run.WebURL, "View Details →"),
		statusClass, strings.ToUpper(string(run.Status))))
}
