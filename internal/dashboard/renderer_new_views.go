package dashboard

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/vilaca/ci-dashboard/internal/domain"
	"github.com/vilaca/ci-dashboard/internal/service"
)

// formatDuration formats a duration into human-readable format.
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

// RenderRepositoryDetailSkeleton renders the repository detail page skeleton for progressive loading.
func (r *HTMLRenderer) RenderRepositoryDetailSkeleton(w io.Writer, repositoryID string) error {
	var sb strings.Builder

	sb.WriteString(htmlHead("Repository - CI Dashboard", "Loading repository details..."))
	sb.WriteString(pageCSS(`
		.loading-container { display: flex; flex-direction: column; align-items: center; justify-content: center; min-height: 400px; }
		.spinner { border: 4px solid var(--border); border-top: 4px solid var(--link-color); border-radius: 50%; width: 50px; height: 50px; animation: spin 1s linear infinite; }
		@keyframes spin { 0% { transform: rotate(0deg); } 100% { transform: rotate(360deg); } }
		.loading-text { margin-top: 20px; color: var(--text-secondary); font-size: 16px; }
		.stats-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 20px; margin-bottom: 30px; }
		.stat-card { background: var(--bg-secondary); padding: 20px; border-radius: 8px; box-shadow: 0 2px 4px var(--shadow); transition: background-color 0.3s; }
		.stat-label { font-size: 14px; color: var(--text-secondary); margin-bottom: 8px; }
		.stat-value { font-size: 28px; font-weight: 600; color: var(--text-primary); }
		.skeleton { background: linear-gradient(90deg, var(--bg-secondary) 25%, var(--border) 50%, var(--bg-secondary) 75%); background-size: 200% 100%; animation: loading 1.5s ease-in-out infinite; }
		@keyframes loading { 0% { background-position: 200% 0; } 100% { background-position: -200% 0; } }
		.skeleton-text { height: 20px; border-radius: 4px; margin-bottom: 10px; }
		.skeleton-title { height: 40px; width: 60%; border-radius: 4px; margin-bottom: 20px; }
		.repo-url { margin-bottom: 30px; }
		.tabs { display: flex; gap: 10px; margin-bottom: 20px; border-bottom: 2px solid var(--border); }
		.tab-button { background: none; border: none; padding: 12px 24px; cursor: pointer; font-size: 16px; color: var(--text-secondary); border-bottom: 3px solid transparent; margin-bottom: -2px; transition: all 0.3s; }
		.tab-button:hover { color: var(--text-primary); background: var(--bg-secondary); }
		.tab-button.active { color: var(--link-color); border-bottom-color: var(--link-color); font-weight: 600; }
		.tab-content { display: none; }
		.tab-content.active { display: block; }
		.runs-section { margin-top: 20px; }
		.run-item, .mr-item, .issue-item { background: var(--bg-secondary); padding: 20px; border-radius: 8px; margin-bottom: 15px; display: flex; justify-content: space-between; align-items: center; box-shadow: 0 2px 4px var(--shadow); transition: background-color 0.3s; }
		.run-item:hover, .mr-item:hover, .issue-item:hover { background: var(--border); }
		.run-left { flex: 1; }
		.run-name { font-size: 18px; font-weight: 600; color: var(--text-primary); margin-bottom: 5px; }
		.run-branch { font-size: 14px; color: var(--text-secondary); }
		.run-meta { display: flex; gap: 15px; align-items: center; flex-wrap: wrap; }
		.mr-title, .issue-title { font-size: 18px; font-weight: 600; color: var(--text-primary); margin-bottom: 8px; }
		.mr-meta, .issue-meta { font-size: 14px; color: var(--text-secondary); }
	`))
	sb.WriteString(`<body>
	<div class="container">
`)
	sb.WriteString(buildNavigationWithProfiles(nil))
	sb.WriteString(`
		<div class="skeleton skeleton-title"></div>
		<div class="loading-container">
			<div class="spinner"></div>
			<div class="loading-text" id="loading-status">Loading repository details...</div>
		</div>

		<div id="content" style="display: none;">
			<!-- Content will be populated by JavaScript -->
		</div>
	</div>
	<script>
		const repositoryID = ` + fmt.Sprintf("%q", repositoryID) + `;

		function switchTab(tabName, button) {
			// Hide all tab contents
			document.querySelectorAll('.tab-content').forEach(tab => {
				tab.classList.remove('active');
			});

			// Remove active from all buttons
			document.querySelectorAll('.tab-button').forEach(btn => {
				btn.classList.remove('active');
			});

			// Show selected tab
			const selectedTab = document.getElementById(tabName + '-tab');
			if (selectedTab) {
				selectedTab.classList.add('active');
			}

			// Add active to clicked button
			if (button) {
				button.classList.add('active');
			}
		}

		async function loadRepositoryDetail() {
			try {
				const response = await fetch('/api/repository-detail?id=' + encodeURIComponent(repositoryID));
				if (!response.ok) {
					throw new Error('Failed to load repository details');
				}

				const data = await response.json();

				// Hide loading, show content
				document.querySelector('.loading-container').style.display = 'none';
				document.querySelector('.skeleton-title').style.display = 'none';
				const contentDiv = document.getElementById('content');
				contentDiv.style.display = 'block';
				contentDiv.innerHTML = data.html;
			} catch (error) {
				document.getElementById('loading-status').textContent = 'Error: ' + error.message;
				document.getElementById('loading-status').style.color = 'var(--status-failed)';
			}
		}

		// Load data when page is ready
		if (document.readyState === 'loading') {
			document.addEventListener('DOMContentLoaded', loadRepositoryDetail);
		} else {
			loadRepositoryDetail();
		}
	</script>
`)
	sb.WriteString(themeToggleScript())
	sb.WriteString(`</body>
</html>
`)

	_, err := w.Write([]byte(sb.String()))
	return err
}

// RenderRepositoryDetail renders the detail page for a single repository.
// This renders only the content fragment to be inserted via AJAX.
func (r *HTMLRenderer) RenderRepositoryDetail(w io.Writer, repository service.RepositoryWithRuns, mrs []domain.MergeRequest, issues []domain.Issue) error {
	var sb strings.Builder

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

	// Just render the content fragment (no full page structure)
	sb.WriteString(fmt.Sprintf(`<h1>%s</h1>
		<div class="repo-url">
			%s
		</div>
`, repository.Project.Name, externalLink(repository.Project.WebURL, "View →")))
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
			<button class="tab-button active" data-tab="pipelines" onclick="switchTab('pipelines', this)">Pipelines (` + fmt.Sprintf("%d", len(repository.Runs)) + `)</button>
			<button class="tab-button" data-tab="mrs" onclick="switchTab('mrs', this)">MRs/PRs (` + fmt.Sprintf("%d", len(mrs)) + `)</button>
			<button class="tab-button" data-tab="issues" onclick="switchTab('issues', this)">Issues (` + fmt.Sprintf("%d", len(issues)) + `)</button>
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
`)

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
