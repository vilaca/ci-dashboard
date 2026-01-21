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
`)
	sb.WriteString(loadingSpinner())
	sb.WriteString(`	<div class="container">
		<h1>Repositories</h1>
`)
	sb.WriteString(buildNavigation())
	sb.WriteString("\n")

	if len(repositories) == 0 {
		sb.WriteString(`		<div class="empty">No repositories found. Configure GITLAB_TOKEN or GITHUB_TOKEN environment variables.</div>`)
	} else {
		sb.WriteString(fmt.Sprintf(`		<div class="filters">
			<div class="filter-group">
				<label for="filter-name">Repository Name</label>
				<select id="filter-name"><option value="">All</option></select>
			</div>
			<div class="filter-group">
				<label for="filter-platform">Platform</label>
				<select id="filter-platform"><option value="">All</option></select>
			</div>
			<span class="filter-count">%d items</span>
		</div>
		<div class="repo-grid" id="repo-grid">
`, len(repositories)))
		for _, repo := range repositories {
			r.writeRepositoryCard(&sb, repo)
		}
		sb.WriteString(`		</div>
		<script>
			document.addEventListener('DOMContentLoaded', function() {
				// Grid filtering uses card visibility instead of table rows
				const grid = document.getElementById('repo-grid');
				if (!grid) return;

				const cards = grid.querySelectorAll('.repo-card');
				const filters = [
					{ inputId: 'filter-name', attr: 'name' },
					{ inputId: 'filter-platform', attr: 'platform' }
				];

				// Populate dropdowns with unique values
				filters.forEach(filter => {
					const select = document.getElementById(filter.inputId);
					if (!select) return;

					const uniqueValues = new Set();
					cards.forEach(card => {
						const value = card.getAttribute('data-' + filter.attr);
						if (value && value.trim() !== '') {
							uniqueValues.add(value.trim());
						}
					});

					// Sort values alphabetically
					const sortedValues = Array.from(uniqueValues).sort((a, b) =>
						a.toLowerCase().localeCompare(b.toLowerCase())
					);

					// Clear existing options except "All"
					select.innerHTML = '<option value="">All</option>';

					// Add options
					sortedValues.forEach(value => {
						const option = document.createElement('option');
						option.value = value;
						option.textContent = value;
						select.appendChild(option);
					});

					// Add change listener
					select.addEventListener('change', () => {
						filterGridCards();
					});
				});

				function filterGridCards() {
					const filterValues = {};
					filters.forEach(f => {
						const sel = document.getElementById(f.inputId);
						if (sel) filterValues[f.attr] = sel.value.toLowerCase();
					});

					cards.forEach(card => {
						let show = true;
						for (const [attr, value] of Object.entries(filterValues)) {
							if (value === '') continue;
							const cardValue = (card.getAttribute('data-' + attr) || '').toLowerCase();
							if (cardValue !== value) {
								show = false;
								break;
							}
						}
						card.style.display = show ? '' : 'none';
					});

					const visibleCards = Array.from(cards).filter(card => card.style.display !== 'none');
					const visibleCount = visibleCards.length;
					const countElement = document.querySelector('.filter-count');
					if (countElement) {
						countElement.textContent = visibleCount + ' of ' + cards.length + ' items';
					}

					// Auto-navigate to detail page when only one repository is visible
					if (visibleCount === 1) {
						const card = visibleCards[0];
						const link = card.querySelector('.repo-title a');
						if (link && link.href) {
							window.location.href = link.href;
						}
					}
				}

				// Initial count
				const countElement = document.querySelector('.filter-count');
				if (countElement) {
					countElement.textContent = cards.length + ' of ' + cards.length + ' items';
				}
			});
		</script>
`)
	}

	sb.WriteString(`	</div>`)
	sb.WriteString(htmlFooter())

	_, err := w.Write([]byte(sb.String()))
	return err
}

// writeRepositoryCard writes a single repository card to the string builder.
func (r *HTMLRenderer) writeRepositoryCard(sb *strings.Builder, repo service.RepositoryWithRuns) {
	sb.WriteString(fmt.Sprintf(`			<div class="repo-card" data-name="%s" data-platform="%s">
				<div class="repo-header">
					<div class="repo-title"><a href="/repository?id=%s">%s</a></div>
					<div class="repo-platform">%s</div>
				</div>
`, escapeHTML(repo.Project.Name), strings.ToLower(repo.Project.Platform), repo.Project.ID, escapeHTML(repo.Project.Name), repo.Project.Platform))

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
							%s
						</div>
					</li>
`, statusClass, name, statusClass, strings.ToUpper(string(run.Status)),
		formatDuration(run.Duration),
		formatTimeAgo(run.UpdatedAt),
		externalLink(run.WebURL, "View Details →")))
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
`)
	sb.WriteString(loadingSpinner())
	sb.WriteString(`	<div class="container">
		<h1>Recent Pipelines</h1>
`)
	sb.WriteString(buildNavigation())
	sb.WriteString("\n")

	if len(pipelines) == 0 {
		sb.WriteString(`		<div class="empty">No pipelines found. Configure GITLAB_TOKEN or GITHUB_TOKEN environment variables.</div>`)
	} else {
		sb.WriteString(fmt.Sprintf(`		<div class="filters">
			<div class="filter-group">
				<label for="filter-repo">Repository</label>
				<select id="filter-repo"><option value="">All</option></select>
			</div>
			<div class="filter-group">
				<label for="filter-name">Name</label>
				<select id="filter-name"><option value="">All</option></select>
			</div>
			<div class="filter-group">
				<label for="filter-branch">Branch</label>
				<select id="filter-branch"><option value="">All</option></select>
			</div>
			<div class="filter-group">
				<label for="filter-status">Status</label>
				<select id="filter-status"><option value="">All</option></select>
			</div>
			<span class="filter-count">%d items</span>
		</div>
		<div class="pipelines-table">
			<table id="pipelines-table">
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
`, len(pipelines)))
		for _, pipeline := range pipelines {
			r.writePipelineRow(&sb, pipeline)
		}
		sb.WriteString(`				</tbody>
			</table>
		</div>
		<script>
			document.addEventListener('DOMContentLoaded', function() {
				setupFilters('pipelines-table', [
					{ inputId: 'filter-repo', attr: 'repo' },
					{ inputId: 'filter-name', attr: 'name' },
					{ inputId: 'filter-branch', attr: 'branch' },
					{ inputId: 'filter-status', attr: 'status' }
				]);
			});
		</script>
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

	sb.WriteString(fmt.Sprintf(`					<tr data-repo="%s" data-name="%s" data-branch="%s" data-status="%s">
						<td class="meta-text"><a href="/repository?id=%s" class="repo-link">%s</a></td>
						<td>
							<div class="pipeline-name">%s</div>
							<div class="pipeline-repo">%s</div>
						</td>
						<td class="meta-text">%s</td>
						<td class="meta-text">%s</td>
						<td>%s</td>
						<td><span class="status-badge %s">%s</span></td>
					</tr>
`, escapeHTML(pipeline.Repository), escapeHTML(name), escapeHTML(pipeline.Branch), strings.ToLower(string(pipeline.Status)),
		pipeline.ProjectID, escapeHTML(pipeline.Repository), escapeHTML(name), escapeHTML(pipeline.Branch),
		formatDuration(pipeline.Duration),
		formatTimeAgo(pipeline.UpdatedAt),
		externalLink(pipeline.WebURL, "View →"), statusClass, strings.ToUpper(string(pipeline.Status))))
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
func (r *HTMLRenderer) RenderRepositoryDetail(w io.Writer, repository service.RepositoryWithRuns, mrs []domain.MergeRequest, issues []domain.Issue) error {
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
		.tabs { display: flex; gap: 10px; margin-bottom: 20px; border-bottom: 2px solid var(--border-color); }
		.tab-button { padding: 12px 24px; background: none; border: none; border-bottom: 3px solid transparent; color: var(--text-secondary); cursor: pointer; font-size: 16px; font-weight: 500; transition: all 0.2s; }
		.tab-button:hover { color: var(--text-primary); background: var(--bg-primary); }
		.tab-button.active { color: var(--link-color); border-bottom-color: var(--link-color); }
		.tab-content { display: none; }
		.tab-content.active { display: block; }
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
		.mr-item, .issue-item { padding: 15px; border-bottom: 1px solid var(--border-color); }
		.mr-item:last-child, .issue-item:last-child { border-bottom: none; }
		.mr-item:hover, .issue-item:hover { background: var(--bg-primary); }
		.mr-title, .issue-title { font-weight: 500; color: var(--text-primary); margin-bottom: 5px; }
		.mr-meta, .issue-meta { font-size: 13px; color: var(--text-secondary); }
		@media (max-width: 768px) {
			.run-item { flex-direction: column; align-items: flex-start; }
			.run-meta { flex-direction: column; align-items: flex-start; gap: 8px; }
			.tabs { overflow-x: auto; }
		}
	`))
	sb.WriteString(`<body>
`)
	sb.WriteString(loadingSpinner())
	sb.WriteString(fmt.Sprintf(`	<div class="container">
		<h1>%s</h1>
		<div class="repo-url">
			%s
		</div>
`, repository.Project.Name, externalLink(repository.Project.WebURL, "View →")))

	// Navigation
	sb.WriteString(buildNavigation())
	sb.WriteString(`
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
				<div class="stat-value">` + formatDuration(avgDuration) + `</div>
			</div>
		</div>

		<!-- Tabs -->
		<div class="tabs">
			<button class="tab-button active" onclick="switchTab('pipelines')">Pipelines (` + fmt.Sprintf("%d", len(repository.Runs)) + `)</button>
			<button class="tab-button" onclick="switchTab('mrs')">MRs/PRs (` + fmt.Sprintf("%d", len(mrs)) + `)</button>
			<button class="tab-button" onclick="switchTab('issues')">Issues (` + fmt.Sprintf("%d", len(issues)) + `)</button>
		</div>

		<!-- Pipelines Tab -->
		<div id="pipelines-tab" class="tab-content active">
			<div class="runs-section">
				<h2>Recent Pipeline Runs</h2>
`)

	if len(repository.Runs) == 0 {
		sb.WriteString(`				<p style="color: var(--text-secondary); text-align: center; padding: 40px 0;">No runs found for this repository.</p>`)
	} else {
		for _, run := range repository.Runs {
			r.writeRepositoryDetailRun(&sb, run)
		}
	}

	sb.WriteString(`			</div>
		</div>

		<!-- MRs/PRs Tab -->
		<div id="mrs-tab" class="tab-content">
			<div class="runs-section">
				<h2>Open Merge Requests / Pull Requests</h2>
`)

	if len(mrs) == 0 {
		sb.WriteString(`				<p style="color: var(--text-secondary); text-align: center; padding: 40px 0;">No open merge requests or pull requests.</p>`)
	} else {
		for _, mr := range mrs {
			r.writeRepositoryDetailMR(&sb, mr)
		}
	}

	sb.WriteString(`			</div>
		</div>

		<!-- Issues Tab -->
		<div id="issues-tab" class="tab-content">
			<div class="runs-section">
				<h2>Open Issues</h2>
`)

	if len(issues) == 0 {
		sb.WriteString(`				<p style="color: var(--text-secondary); text-align: center; padding: 40px 0;">No open issues.</p>`)
	} else {
		for _, issue := range issues {
			r.writeRepositoryDetailIssue(&sb, issue)
		}
	}

	sb.WriteString(`			</div>
		</div>

		<script>
			function switchTab(tabName) {
				// Hide all tabs
				document.querySelectorAll('.tab-content').forEach(tab => {
					tab.classList.remove('active');
				});
				document.querySelectorAll('.tab-button').forEach(btn => {
					btn.classList.remove('active');
				});

				// Show selected tab
				document.getElementById(tabName + '-tab').classList.add('active');
				event.target.classList.add('active');
			}
		</script>
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

// writeRepositoryDetailMR writes a single MR item in the repository detail view.
func (r *HTMLRenderer) writeRepositoryDetailMR(sb *strings.Builder, mr domain.MergeRequest) {
	sb.WriteString(fmt.Sprintf(`			<div class="mr-item">
				<div class="mr-title">%s</div>
				<div class="mr-meta">
					<span>%s → %s</span> |
					<span>by %s</span> |
					<span>Updated %s</span> |
					%s
				</div>
			</div>
`, escapeHTML(mr.Title),
		escapeHTML(mr.SourceBranch), escapeHTML(mr.TargetBranch),
		escapeHTML(mr.Author),
		formatTimeAgo(mr.UpdatedAt),
		externalLink(mr.WebURL, "View →")))
}

// writeRepositoryDetailIssue writes a single issue item in the repository detail view.
func (r *HTMLRenderer) writeRepositoryDetailIssue(sb *strings.Builder, issue domain.Issue) {
	assignee := issue.Assignee
	if assignee == "" {
		assignee = "Unassigned"
	}

	labels := ""
	if len(issue.Labels) > 0 {
		labels = strings.Join(issue.Labels, ", ")
	} else {
		labels = "No labels"
	}

	sb.WriteString(fmt.Sprintf(`			<div class="issue-item">
				<div class="issue-title">%s</div>
				<div class="issue-meta">
					<span>%s</span> |
					<span>by %s</span> |
					<span>Assigned to %s</span> |
					<span>Updated %s</span> |
					%s
				</div>
			</div>
`, escapeHTML(issue.Title),
		escapeHTML(labels),
		escapeHTML(issue.Author),
		escapeHTML(assignee),
		formatTimeAgo(issue.UpdatedAt),
		externalLink(issue.WebURL, "View →")))
}

// RenderFailedPipelines renders a list of failed pipelines.
func (r *HTMLRenderer) RenderFailedPipelines(w io.Writer, pipelines []domain.Pipeline) error {
	var sb strings.Builder

	sb.WriteString(htmlHead("Failed Pipelines", "View all failed CI/CD pipeline runs"))
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
		.failure-notice { background: var(--failed-bg); color: var(--failed-text); padding: 15px; border-radius: 8px; margin-bottom: 20px; border-left: 4px solid var(--failed-text); }
		@media (max-width: 768px) {
			.pipelines-table { overflow-x: auto; }
			.pipelines-table th, .pipelines-table td { padding: 10px; font-size: 14px; }
		}
	`))
	sb.WriteString(`<body>
`)
	sb.WriteString(loadingSpinner())
	sb.WriteString(`	<div class="container">
		<h1>Failed Pipelines</h1>
`)
	sb.WriteString(buildNavigation())
	sb.WriteString("\n")

	if len(pipelines) == 0 {
		sb.WriteString(`		<div class="empty">No failed pipelines. All systems operational! ✅</div>`)
	} else {
		sb.WriteString(fmt.Sprintf(`		<div class="failure-notice">
			⚠️ Found %d failed pipeline(s) requiring attention
		</div>
		<div class="filters">
			<div class="filter-group">
				<label for="filter-repo">Repository</label>
				<select id="filter-repo"><option value="">All</option></select>
			</div>
			<div class="filter-group">
				<label for="filter-name">Name</label>
				<select id="filter-name"><option value="">All</option></select>
			</div>
			<div class="filter-group">
				<label for="filter-branch">Branch</label>
				<select id="filter-branch"><option value="">All</option></select>
			</div>
			<span class="filter-count">%d items</span>
		</div>
		<div class="pipelines-table">
			<table id="pipelines-table">
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
`, len(pipelines), len(pipelines)))
		for _, pipeline := range pipelines {
			r.writePipelineRow(&sb, pipeline)
		}
		sb.WriteString(`				</tbody>
			</table>
		</div>
		<script>
			document.addEventListener('DOMContentLoaded', function() {
				setupFilters('pipelines-table', [
					{ inputId: 'filter-repo', attr: 'repo' },
					{ inputId: 'filter-name', attr: 'name' },
					{ inputId: 'filter-branch', attr: 'branch' }
				]);
			});
		</script>
`)
	}

	sb.WriteString(`	</div>`)
	sb.WriteString(htmlFooter())

	_, err := w.Write([]byte(sb.String()))
	return err
}

// RenderMergeRequests renders a list of open merge requests/pull requests.
func (r *HTMLRenderer) RenderMergeRequests(w io.Writer, mrs []domain.MergeRequest) error {
	var sb strings.Builder

	sb.WriteString(htmlHead("Open MRs/PRs", "View all open merge requests and pull requests"))
	sb.WriteString(pageCSS(`
		.mrs-table { width: 100%; background: var(--bg-secondary); border-radius: 8px; box-shadow: 0 2px 4px var(--shadow); overflow: hidden; transition: background-color 0.3s; }
		.mrs-table table { width: 100%; border-collapse: collapse; }
		.mrs-table th { background: var(--bg-primary); padding: 15px; text-align: left; font-weight: 600; color: var(--text-primary); border-bottom: 2px solid var(--border-color); }
		.mrs-table td { padding: 15px; border-bottom: 1px solid var(--border-color); color: var(--text-primary); }
		.mrs-table tr:hover { background: var(--bg-primary); }
		.mr-title { font-weight: 500; }
		.mr-branch { font-size: 13px; color: var(--text-secondary); }
		.mr-author { font-size: 13px; color: var(--text-secondary); }
		.repo-link { color: var(--text-primary); text-decoration: none; }
		.repo-link:hover { color: var(--link-color); text-decoration: underline; }
		.link { color: var(--link-color); text-decoration: none; }
		.link:hover { text-decoration: underline; }
		@media (max-width: 768px) {
			.mrs-table { overflow-x: auto; }
			.mrs-table th, .mrs-table td { padding: 10px; font-size: 14px; }
		}
	`))
	sb.WriteString(`<body>
`)
	sb.WriteString(loadingSpinner())
	sb.WriteString(`	<div class="container">
		<h1>Open Merge Requests / Pull Requests</h1>
`)
	sb.WriteString(buildNavigation())
	sb.WriteString("\n")

	if len(mrs) == 0 {
		sb.WriteString(`		<div class="empty">No open merge requests or pull requests found.</div>`)
	} else {
		sb.WriteString(fmt.Sprintf(`		<div class="filters">
			<div class="filter-group">
				<label for="filter-repo">Repository</label>
				<select id="filter-repo"><option value="">All</option></select>
			</div>
			<div class="filter-group">
				<label for="filter-title">Title</label>
				<select id="filter-title"><option value="">All</option></select>
			</div>
			<div class="filter-group">
				<label for="filter-author">Author</label>
				<select id="filter-author"><option value="">All</option></select>
			</div>
			<div class="filter-group">
				<label for="filter-branch">Branch</label>
				<select id="filter-branch"><option value="">All</option></select>
			</div>
			<span class="filter-count">%d items</span>
		</div>
		<div class="mrs-table">
			<table id="mrs-table">
				<thead>
					<tr>
						<th>Repository</th>
						<th>Title</th>
						<th>Branches</th>
						<th>Author</th>
						<th>Updated</th>
						<th>Actions</th>
					</tr>
				</thead>
				<tbody>
`, len(mrs)))
		for _, mr := range mrs {
			r.writeMRRow(&sb, mr)
		}
		sb.WriteString(`				</tbody>
			</table>
		</div>
		<script>
			document.addEventListener('DOMContentLoaded', function() {
				setupFilters('mrs-table', [
					{ inputId: 'filter-repo', attr: 'repo' },
					{ inputId: 'filter-title', attr: 'title' },
					{ inputId: 'filter-author', attr: 'author' },
					{ inputId: 'filter-branch', attr: 'branch' }
				]);
			});
		</script>
`)
	}

	sb.WriteString(`	</div>`)
	sb.WriteString(htmlFooter())

	_, err := w.Write([]byte(sb.String()))
	return err
}

// writeMRRow writes a single MR/PR row to the string builder.
func (r *HTMLRenderer) writeMRRow(sb *strings.Builder, mr domain.MergeRequest) {
	branches := fmt.Sprintf("%s %s", mr.SourceBranch, mr.TargetBranch)
	sb.WriteString(fmt.Sprintf(`					<tr data-repo="%s" data-title="%s" data-author="%s" data-branch="%s">
						<td class="meta-text"><a href="/repository?id=%s" class="repo-link">%s</a></td>
						<td>
							<div class="mr-title">%s</div>
							<div class="mr-author">by %s</div>
						</td>
						<td class="mr-branch">%s → %s</td>
						<td class="mr-author">%s</td>
						<td class="meta-text">%s</td>
						<td>%s</td>
					</tr>
`, escapeHTML(mr.Repository), escapeHTML(mr.Title), escapeHTML(mr.Author), escapeHTML(branches),
		mr.ProjectID, escapeHTML(mr.Repository), escapeHTML(mr.Title), escapeHTML(mr.Author),
		escapeHTML(mr.SourceBranch), escapeHTML(mr.TargetBranch),
		escapeHTML(mr.Author),
		formatTimeAgo(mr.UpdatedAt),
		externalLink(mr.WebURL, "View →")))
}

// RenderIssues renders a list of open issues.
func (r *HTMLRenderer) RenderIssues(w io.Writer, issues []domain.Issue) error {
	var sb strings.Builder

	sb.WriteString(htmlHead("Open Issues", "View all open issues across repositories"))
	sb.WriteString(pageCSS(`
		.issues-table { width: 100%; background: var(--bg-secondary); border-radius: 8px; box-shadow: 0 2px 4px var(--shadow); overflow: hidden; transition: background-color 0.3s; }
		.issues-table table { width: 100%; border-collapse: collapse; }
		.issues-table th { background: var(--bg-primary); padding: 15px; text-align: left; font-weight: 600; color: var(--text-primary); border-bottom: 2px solid var(--border-color); }
		.issues-table td { padding: 15px; border-bottom: 1px solid var(--border-color); color: var(--text-primary); }
		.issues-table tr:hover { background: var(--bg-primary); }
		.issue-title { font-weight: 500; }
		.issue-labels { font-size: 12px; }
		.label { display: inline-block; padding: 2px 8px; margin: 2px; background: var(--border-color); border-radius: 3px; font-size: 11px; }
		.issue-author { font-size: 13px; color: var(--text-secondary); }
		.repo-link { color: var(--text-primary); text-decoration: none; }
		.repo-link:hover { color: var(--link-color); text-decoration: underline; }
		.link { color: var(--link-color); text-decoration: none; }
		.link:hover { text-decoration: underline; }
		@media (max-width: 768px) {
			.issues-table { overflow-x: auto; }
			.issues-table th, .issues-table td { padding: 10px; font-size: 14px; }
		}
	`))
	sb.WriteString(`<body>
`)
	sb.WriteString(loadingSpinner())
	sb.WriteString(`	<div class="container">
		<h1>Open Issues</h1>
`)
	sb.WriteString(buildNavigation())
	sb.WriteString("\n")

	if len(issues) == 0 {
		sb.WriteString(`		<div class="empty">No open issues found.</div>`)
	} else {
		sb.WriteString(fmt.Sprintf(`		<div class="filters">
			<div class="filter-group">
				<label for="filter-repo">Repository</label>
				<select id="filter-repo"><option value="">All</option></select>
			</div>
			<div class="filter-group">
				<label for="filter-title">Title</label>
				<select id="filter-title"><option value="">All</option></select>
			</div>
			<div class="filter-group">
				<label for="filter-labels">Labels</label>
				<select id="filter-labels"><option value="">All</option></select>
			</div>
			<div class="filter-group">
				<label for="filter-author">Author</label>
				<select id="filter-author"><option value="">All</option></select>
			</div>
			<div class="filter-group">
				<label for="filter-assignee">Assignee</label>
				<select id="filter-assignee"><option value="">All</option></select>
			</div>
			<span class="filter-count">%d items</span>
		</div>
		<div class="issues-table">
			<table id="issues-table">
				<thead>
					<tr>
						<th>Repository</th>
						<th>Title</th>
						<th>Labels</th>
						<th>Author</th>
						<th>Assignee</th>
						<th>Updated</th>
						<th>Actions</th>
					</tr>
				</thead>
				<tbody>
`, len(issues)))
		for _, issue := range issues {
			r.writeIssueRow(&sb, issue)
		}
		sb.WriteString(`				</tbody>
			</table>
		</div>
		<script>
			document.addEventListener('DOMContentLoaded', function() {
				setupFilters('issues-table', [
					{ inputId: 'filter-repo', attr: 'repo' },
					{ inputId: 'filter-title', attr: 'title' },
					{ inputId: 'filter-labels', attr: 'labels' },
					{ inputId: 'filter-author', attr: 'author' },
					{ inputId: 'filter-assignee', attr: 'assignee' }
				]);
			});
		</script>
`)
	}

	sb.WriteString(`	</div>`)
	sb.WriteString(htmlFooter())

	_, err := w.Write([]byte(sb.String()))
	return err
}

// writeIssueRow writes a single issue row to the string builder.
func (r *HTMLRenderer) writeIssueRow(sb *strings.Builder, issue domain.Issue) {
	// Format labels
	var labelsHTML string
	labelsText := strings.Join(issue.Labels, " ")
	if len(issue.Labels) > 0 {
		for _, label := range issue.Labels {
			labelsHTML += fmt.Sprintf(`<span class="label">%s</span>`, escapeHTML(label))
		}
	} else {
		labelsHTML = `<span class="meta-text">—</span>`
	}

	assignee := issue.Assignee
	if assignee == "" {
		assignee = "—"
	}

	sb.WriteString(fmt.Sprintf(`					<tr data-repo="%s" data-title="%s" data-labels="%s" data-author="%s" data-assignee="%s">
						<td class="meta-text"><a href="/repository?id=%s" class="repo-link">%s</a></td>
						<td>
							<div class="issue-title">%s</div>
							<div class="issue-author">by %s</div>
						</td>
						<td class="issue-labels">%s</td>
						<td class="issue-author">%s</td>
						<td class="issue-author">%s</td>
						<td class="meta-text">%s</td>
						<td>%s</td>
					</tr>
`, escapeHTML(issue.Repository), escapeHTML(issue.Title), escapeHTML(labelsText), escapeHTML(issue.Author), escapeHTML(assignee),
		issue.ProjectID, escapeHTML(issue.Repository), escapeHTML(issue.Title), escapeHTML(issue.Author),
		labelsHTML,
		escapeHTML(issue.Author),
		escapeHTML(assignee),
		formatTimeAgo(issue.UpdatedAt),
		externalLink(issue.WebURL, "View →")))
}

// RenderBranches renders all branches with their latest pipeline status.
func (r *HTMLRenderer) RenderBranches(w io.Writer, branches []domain.BranchWithPipeline) error {
	var sb strings.Builder

	sb.WriteString(htmlHead("Branches", "View all branches across repositories"))
	sb.WriteString(pageCSS(`
		.branches-table { width: 100%; background: var(--bg-secondary); border-radius: 8px; box-shadow: 0 2px 4px var(--shadow); overflow: hidden; transition: background-color 0.3s; }
		.branches-table table { width: 100%; border-collapse: collapse; }
		.branches-table th { background: var(--bg-primary); padding: 15px; text-align: left; font-weight: 600; color: var(--text-primary); border-bottom: 2px solid var(--border-color); }
		.branches-table td { padding: 15px; border-bottom: 1px solid var(--border-color); color: var(--text-primary); }
		.branches-table tr:hover { background: var(--bg-primary); }
		.branch-name { font-weight: 500; }
		.branch-badges { display: flex; gap: 5px; margin-top: 5px; }
		.badge { display: inline-block; padding: 2px 8px; border-radius: 3px; font-size: 11px; font-weight: 600; }
		.badge-default { background: var(--running-bg); color: var(--running-text); }
		.badge-protected { background: var(--pending-bg); color: var(--pending-text); }
		.commit-msg { font-size: 13px; color: var(--text-secondary); margin-top: 5px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 400px; }
		.repo-link { color: var(--text-primary); text-decoration: none; }
		.repo-link:hover { color: var(--link-color); text-decoration: underline; }
		@media (max-width: 768px) {
			.branches-table { overflow-x: auto; }
			.branches-table th, .branches-table td { padding: 10px; font-size: 14px; }
		}
	`))
	sb.WriteString(`<body>
`)
	sb.WriteString(loadingSpinner())
	sb.WriteString(`	<div class="container">
		<h1>Branches</h1>
`)
	sb.WriteString(buildNavigation())
	sb.WriteString("\n")

	if len(branches) == 0 {
		sb.WriteString(`		<div class="empty">No branches found. Configure GITLAB_TOKEN or GITHUB_TOKEN environment variables.</div>`)
	} else {
		sb.WriteString(fmt.Sprintf(`		<div class="filters">
			<div class="filter-group">
				<label for="filter-repo">Repository</label>
				<select id="filter-repo"><option value="">All</option></select>
			</div>
			<div class="filter-group">
				<label for="filter-branch">Branch Name</label>
				<select id="filter-branch"><option value="">All</option></select>
			</div>
			<div class="filter-group">
				<label for="filter-status">Pipeline Status</label>
				<select id="filter-status"><option value="">All</option></select>
			</div>
			<div class="filter-group">
				<label for="filter-author">Author</label>
				<select id="filter-author"><option value="">All</option></select>
			</div>
			<span class="filter-count">%d items</span>
		</div>
		<div class="branches-table">
			<table id="branches-table">
				<thead>
					<tr>
						<th>Repository</th>
						<th>Branch</th>
						<th>Author</th>
						<th>Last Commit</th>
						<th>Pipeline Status</th>
						<th>Actions</th>
					</tr>
				</thead>
				<tbody>
`, len(branches)))
		for _, branch := range branches {
			r.writeBranchRow(&sb, branch)
		}
		sb.WriteString(`				</tbody>
			</table>
		</div>
		<script>
			document.addEventListener('DOMContentLoaded', function() {
				setupFilters('branches-table', [
					{ inputId: 'filter-repo', attr: 'repo' },
					{ inputId: 'filter-branch', attr: 'branch' },
					{ inputId: 'filter-status', attr: 'status' },
					{ inputId: 'filter-author', attr: 'author' }
				]);
			});
		</script>
`)
	}

	sb.WriteString(`	</div>`)
	sb.WriteString(htmlFooter())

	_, err := w.Write([]byte(sb.String()))
	return err
}

// RenderYourBranches renders branches filtered by current user.
func (r *HTMLRenderer) RenderYourBranches(w io.Writer, branches []domain.BranchWithPipeline, gitlabUsername, githubUsername string) error {
	var sb strings.Builder

	title := "Your Branches"
	usernames := []string{}
	if gitlabUsername != "" {
		usernames = append(usernames, gitlabUsername)
	}
	if githubUsername != "" {
		usernames = append(usernames, githubUsername)
	}
	if len(usernames) > 0 {
		title = fmt.Sprintf("Branches by %s", strings.Join(usernames, ", "))
	}

	sb.WriteString(htmlHead(title, "View your branches"))
	sb.WriteString(pageCSS(`
		.branches-table { width: 100%; background: var(--bg-secondary); border-radius: 8px; box-shadow: 0 2px 4px var(--shadow); overflow: hidden; transition: background-color 0.3s; }
		.branches-table table { width: 100%; border-collapse: collapse; }
		.branches-table th { background: var(--bg-primary); padding: 15px; text-align: left; font-weight: 600; color: var(--text-primary); border-bottom: 2px solid var(--border-color); }
		.branches-table td { padding: 15px; border-bottom: 1px solid var(--border-color); color: var(--text-primary); }
		.branches-table tr:hover { background: var(--bg-primary); }
		.branch-name { font-weight: 500; }
		.branch-badges { display: flex; gap: 5px; margin-top: 5px; }
		.badge { display: inline-block; padding: 2px 8px; border-radius: 3px; font-size: 11px; font-weight: 600; }
		.badge-default { background: var(--running-bg); color: var(--running-text); }
		.badge-protected { background: var(--pending-bg); color: var(--pending-text); }
		.commit-msg { font-size: 13px; color: var(--text-secondary); margin-top: 5px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 400px; }
		.repo-link { color: var(--text-primary); text-decoration: none; }
		.repo-link:hover { color: var(--link-color); text-decoration: underline; }
		.user-notice { background: var(--running-bg); color: var(--running-text); padding: 15px; border-radius: 8px; margin-bottom: 20px; border-left: 4px solid var(--running-text); }
		@media (max-width: 768px) {
			.branches-table { overflow-x: auto; }
			.branches-table th, .branches-table td { padding: 10px; font-size: 14px; }
		}
	`))
	sb.WriteString(`<body>
`)
	sb.WriteString(loadingSpinner())
	sb.WriteString(fmt.Sprintf(`	<div class="container">
		<h1>%s</h1>
`, title))
	sb.WriteString(buildNavigation())

	if gitlabUsername == "" && githubUsername == "" {
		sb.WriteString(`
		<div class="user-notice">
			No GITLAB_USER or GITHUB_USER configured. Set these environment variables to filter branches.
		</div>
`)
	}

	if len(branches) == 0 {
		if gitlabUsername == "" && githubUsername == "" {
			sb.WriteString(`		<div class="empty">No branches found. Configure GITLAB_TOKEN or GITHUB_TOKEN environment variables.</div>`)
		} else {
			sb.WriteString(fmt.Sprintf(`		<div class="empty">No branches found authored by %s.</div>`, escapeHTML(strings.Join(usernames, ", "))))
		}
	} else {
		sb.WriteString(fmt.Sprintf(`		<div class="filters">
			<div class="filter-group">
				<label for="filter-repo">Repository</label>
				<select id="filter-repo"><option value="">All</option></select>
			</div>
			<div class="filter-group">
				<label for="filter-branch">Branch Name</label>
				<select id="filter-branch"><option value="">All</option></select>
			</div>
			<div class="filter-group">
				<label for="filter-status">Pipeline Status</label>
				<select id="filter-status"><option value="">All</option></select>
			</div>
			<span class="filter-count">%d items</span>
		</div>
		<div class="branches-table">
			<table id="branches-table">
				<thead>
					<tr>
						<th>Repository</th>
						<th>Branch</th>
						<th>Last Commit</th>
						<th>Pipeline Status</th>
						<th>Actions</th>
					</tr>
				</thead>
				<tbody>
`, len(branches)))
		for _, branch := range branches {
			r.writeBranchRow(&sb, branch)
		}
		sb.WriteString(`				</tbody>
			</table>
		</div>
		<script>
			document.addEventListener('DOMContentLoaded', function() {
				setupFilters('branches-table', [
					{ inputId: 'filter-repo', attr: 'repo' },
					{ inputId: 'filter-branch', attr: 'branch' },
					{ inputId: 'filter-status', attr: 'status' }
				]);
			});
		</script>
`)
	}

	sb.WriteString(`	</div>`)
	sb.WriteString(htmlFooter())

	_, err := w.Write([]byte(sb.String()))
	return err
}

// writeBranchRow writes a single branch row to the string builder.
func (r *HTMLRenderer) writeBranchRow(sb *strings.Builder, bwp domain.BranchWithPipeline) {
	branch := bwp.Branch
	pipeline := bwp.Pipeline

	// Determine pipeline status
	statusClass := ""
	if pipeline != nil {
		statusClass = strings.ToLower(string(pipeline.Status))
	}

	// Build badges
	badges := ""
	if branch.IsDefault {
		badges += `<span class="badge badge-default">DEFAULT</span>`
	}
	if branch.IsProtected {
		badges += `<span class="badge badge-protected">PROTECTED</span>`
	}

	// Truncate commit message
	commitMsg := branch.LastCommitMsg
	if len(commitMsg) > 60 {
		commitMsg = commitMsg[:60] + "..."
	}

	author := branch.CommitAuthor
	if author == "" {
		author = "—"
	}

	commitTime := formatTimeAgo(branch.LastCommitDate)
	if branch.LastCommitDate.IsZero() {
		commitTime = "—"
	}

	sb.WriteString(fmt.Sprintf(`					<tr data-repo="%s" data-branch="%s" data-status="%s" data-author="%s">
						<td class="meta-text"><a href="/repository?id=%s" class="repo-link">%s</a></td>
						<td>
							<div class="branch-name">%s</div>
							<div class="branch-badges">%s</div>
						</td>
						<td class="meta-text">%s</td>
						<td>
							<div class="commit-msg" title="%s">%s</div>
							<div class="meta-text">%s</div>
						</td>
						<td>%s</td>
						<td>%s</td>
					</tr>
`, escapeHTML(branch.Repository), escapeHTML(branch.Name), statusClass, escapeHTML(author),
		branch.ProjectID, escapeHTML(branch.Repository),
		escapeHTML(branch.Name), badges,
		escapeHTML(author),
		escapeHTML(branch.LastCommitMsg), escapeHTML(commitMsg),
		commitTime,
		r.formatBranchStatus(pipeline),
		externalLink(branch.WebURL, "View Branch →")))
}

// formatBranchStatus formats the pipeline status badge for a branch.
func (r *HTMLRenderer) formatBranchStatus(pipeline *domain.Pipeline) string {
	if pipeline == nil {
		return `<span class="meta-text">No pipeline</span>`
	}

	statusClass := strings.ToLower(string(pipeline.Status))
	statusText := strings.ToUpper(string(pipeline.Status))

	return fmt.Sprintf(`<span class="status-badge %s">%s</span>`, statusClass, statusText)
}
