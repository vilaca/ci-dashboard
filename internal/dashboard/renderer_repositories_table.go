package dashboard

import (
	"fmt"
	"io"
	"strings"

	"github.com/vilaca/ci-dashboard/internal/domain"
)

// RenderRepositoriesSkeleton renders the repositories page skeleton for progressive loading.
func (r *HTMLRenderer) RenderRepositoriesSkeleton(w io.Writer, userProfiles []domain.UserProfile, refreshInterval int) error {
	var sb strings.Builder

	sb.WriteString(htmlHead("Repositories - CI Dashboard", "Monitor CI/CD pipeline runs across all repositories"))
	sb.WriteString(pageCSS(repositoriesTablePageCSS))
	sb.WriteString(`<body>
	<div class="container">
`)
	sb.WriteString(buildNavigationWithProfiles(userProfiles))
	sb.WriteString(`
		<div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 20px;">
			<h1 style="margin: 0;">Repositories</h1>
			<div id="progress-info" class="progress-info">Loading...</div>
		</div>

		<div class="filters" style="margin-bottom: 20px;">
			<input type="text" id="repoFilter" placeholder="Filter by repository..." class="filter-input">
			<select id="platformFilter" class="filter-select">
				<option value="">All Platforms</option>
				<option value="gitlab">GitLab</option>
				<option value="github">GitHub</option>
			</select>
			<select id="statusFilter" class="filter-select">
				<option value="">All Statuses</option>
				<option value="success">SUCCESS</option>
				<option value="failed">FAILED</option>
				<option value="running">RUNNING</option>
				<option value="pending">PENDING</option>
				<option value="canceled">CANCELED</option>
				<option value="skipped">SKIPPED</option>
				<option value="none">NONE</option>
			</select>
			<select id="forkFilter" class="filter-select">
				<option value="no">Hide Forks</option>
				<option value="yes">Show Forks</option>
			</select>
		</div>

		<table class="pipeline-table">
			<thead>
				<tr>
					<th style="text-align: left;">Repository</th>
					<th style="text-align: center;">Platform</th>
					<th style="text-align: center;">Role</th>
					<th style="text-align: center;">Status</th>
					<th style="text-align: center;">Branches</th>
					<th style="text-align: center;">MRs/PRs</th>
					<th style="text-align: center;">Last Commit Author</th>
					<th style="text-align: right;">Last Commit</th>
				</tr>
			</thead>
			<tbody id="repositories-tbody">
				<tr><td colspan="8" class="loading-cell">Loading repositories...</td></tr>
			</tbody>
		</table>
	</div>
`)
	sb.WriteString(themeToggleScript())
	sb.WriteString(fmt.Sprintf(`<script>const REFRESH_INTERVAL_SECONDS = %d;</script>`, refreshInterval))
	sb.WriteString(repositoriesTableScript())
	sb.WriteString(`</body>
</html>
`)

	_, err := w.Write([]byte(sb.String()))
	return err
}

const repositoriesTablePageCSS = `
	.progress-info {
		font-size: 14px;
		color: var(--text-secondary);
		font-weight: 500;
	}
	.loading-cell {
		text-align: center;
		padding: 40px;
		color: var(--text-secondary);
		font-style: italic;
	}
	.user-profiles {
		display: flex;
		gap: 10px;
		align-items: center;
		margin-right: 15px;
	}
	.user-profile {
		display: flex;
		align-items: center;
	}
	.profile-avatar {
		width: 32px;
		height: 32px;
		border-radius: 50%;
		border: 2px solid var(--border);
		transition: transform 0.2s, border-color 0.2s;
	}
	.profile-avatar:hover {
		transform: scale(1.15);
		border-color: var(--link-color);
	}
	.filter-input {
		padding: 8px 12px;
		border: 1px solid var(--border);
		border-radius: 4px;
		background: var(--bg-primary);
		color: var(--text-primary);
		margin-right: 10px;
	}
	.filter-select {
		padding: 8px 12px;
		border: 1px solid var(--border);
		border-radius: 4px;
		background: var(--bg-primary);
		color: var(--text-primary);
		margin-right: 10px;
	}
	.favorite-star {
		font-size: 18px;
		color: var(--text-secondary);
		transition: color 0.2s, transform 0.1s;
		display: inline-block;
		user-select: none;
	}
	.favorite-star:hover {
		color: #ffd700;
		transform: scale(1.2);
	}
	.favorite-star.favorited {
		color: #ffd700;
	}
	.platform-badge {
		padding: 2px 8px;
		border-radius: 3px;
		font-size: 11px;
		font-weight: 600;
		margin-left: 8px;
		text-transform: uppercase;
		color: white;
		text-decoration: none;
		display: inline-block;
		transition: opacity 0.2s, transform 0.1s;
	}
	.platform-badge:hover {
		opacity: 0.8;
		transform: translateY(-1px);
	}
	.platform-gitlab {
		background: #fc6d26;
	}
	.platform-github {
		background: #24292e;
	}
	.fork-badge {
		padding: 2px 8px;
		border-radius: 3px;
		font-size: 11px;
		font-weight: 600;
		margin-left: 8px;
		text-transform: uppercase;
		color: white;
		background: #6c757d;
		display: inline-block;
	}
	.repo-link:hover {
		text-decoration: underline;
		color: var(--link-color) !important;
	}
	.pipeline-table {
		table-layout: fixed;
		width: 100%;
	}
	.pipeline-table thead {
		background: var(--bg-secondary);
	}
	.pipeline-table th {
		padding: 12px 8px;
	}
	.pipeline-table th:nth-child(1) { width: 30%; text-align: left; }
	.pipeline-table th:nth-child(2) { width: 7%; text-align: center; }
	.pipeline-table th:nth-child(3) { width: 8%; text-align: center; }
	.pipeline-table th:nth-child(4) { width: 9%; text-align: center; }
	.pipeline-table th:nth-child(5) { width: 7%; text-align: center; }
	.pipeline-table th:nth-child(6) { width: 10%; text-align: center; }
	.pipeline-table th:nth-child(7) { width: 17%; text-align: center; }
	.pipeline-table th:nth-child(8) { width: 12%; text-align: right; }
	.platform-cell {
		text-align: center;
	}
	.status-cell {
		text-align: center;
	}
	.commit-cell {
		text-align: right;
	}
	.committer-cell {
		text-align: center;
	}
	.count-cell {
		text-align: center;
	}
`

func repositoriesTableScript() string {
	return `
	<script>
		// Global applyFilters function
		let applyFilters;

		// Dynamic filtering
		function setupDynamicFilters() {
			const repoFilter = document.getElementById('repoFilter');
			const platformFilter = document.getElementById('platformFilter');
			const statusFilter = document.getElementById('statusFilter');
			const forkFilter = document.getElementById('forkFilter');
			const tbody = document.getElementById('repositories-tbody');

			applyFilters = function() {
				const repoValue = repoFilter.value.toLowerCase();
				const platformValue = platformFilter.value.toLowerCase();
				const statusValue = statusFilter.value.toLowerCase();
				const forkValue = forkFilter.value.toLowerCase();

				const rows = tbody.querySelectorAll('tr.filterable');

				rows.forEach(row => {
					const repo = (row.getAttribute('data-repository') || '').toLowerCase();
					const platform = (row.getAttribute('data-platform') || '').toLowerCase();
					const status = (row.getAttribute('data-status') || '').toLowerCase();
					const isFork = row.getAttribute('data-is-fork') === 'true';

					const repoMatch = !repoValue || repo.includes(repoValue);
					const platformMatch = !platformValue || platform === platformValue;
					const statusMatch = !statusValue || status === statusValue;

					let forkMatch = true;
					if (forkValue === 'no') {
						forkMatch = !isFork;
					}

					if (repoMatch && platformMatch && statusMatch && forkMatch) {
						row.style.display = '';
					} else {
						row.style.display = 'none';
					}
				});
			}

			repoFilter.addEventListener('input', applyFilters);
			platformFilter.addEventListener('change', applyFilters);
			statusFilter.addEventListener('change', applyFilters);
			forkFilter.addEventListener('change', applyFilters);
		}

		if (document.readyState === 'loading') {
			document.addEventListener('DOMContentLoaded', setupDynamicFilters);
		} else {
			setupDynamicFilters();
		}

		const tbody = document.getElementById('repositories-tbody');
		const progressInfo = document.getElementById('progress-info');
		let loadedCount = 0;
		let totalCount = 0;
		let allLoaded = false;
		let allRepositories = [];

		// Favorites management
		function getFavorites() {
			const stored = localStorage.getItem('ci-dashboard-favorites');
			return stored ? JSON.parse(stored) : [];
		}

		let reorderTimeout = null;

		function toggleFavorite(repoId) {
			const favorites = getFavorites();
			const index = favorites.indexOf(repoId);
			const isNowFavorite = index === -1;

			if (index > -1) {
				favorites.splice(index, 1);
			} else {
				favorites.push(repoId);
			}
			localStorage.setItem('ci-dashboard-favorites', JSON.stringify(favorites));

			const starElement = document.querySelector('[data-repo-id="' + repoId + '"]');
			if (starElement) {
				if (isNowFavorite) {
					starElement.textContent = '★';
					starElement.classList.add('favorited');
					starElement.title = 'Remove from favorites';
				} else {
					starElement.textContent = '☆';
					starElement.classList.remove('favorited');
					starElement.title = 'Add to favorites';
				}
			}

			if (reorderTimeout) {
				clearTimeout(reorderTimeout);
			}
			reorderTimeout = setTimeout(() => {
				renderAllRepositories();
			}, 2000);
		}

		function isFavorite(repoId) {
			return getFavorites().includes(repoId);
		}

		tbody.innerHTML = '';

		progressInfo.textContent = 'Loading repositories...';
		progressInfo.style.color = '';

		fetch('/api/repositories?limit=10000')
			.then(response => {
				if (!response.ok) {
					throw new Error('Failed to fetch repositories');
				}
				return response.json();
			})
			.then(data => {
				allRepositories = data.repositories || [];
				totalCount = data.pagination ? data.pagination.total : allRepositories.length;
				loadedCount = allRepositories.length;
				allLoaded = data.pagination ? !data.pagination.hasNext : true;

				allRepositories.sort((a, b) => {
					const aFav = isFavorite(a.Project.ID);
					const bFav = isFavorite(b.Project.ID);
					if (aFav && !bFav) return -1;
					if (!aFav && bFav) return 1;

					if (!a.DefaultBranch || !a.DefaultBranch.LastCommitDate) return 1;
					if (!b.DefaultBranch || !b.DefaultBranch.LastCommitDate) return -1;

					const dateA = new Date(a.DefaultBranch.LastCommitDate);
					const dateB = new Date(b.DefaultBranch.LastCommitDate);

					if (dateA.getFullYear() < 1970) return 1;
					if (dateB.getFullYear() < 1970) return -1;

					return dateB - dateA;
				});

				renderAllRepositories();

				if (allLoaded) {
					progressInfo.textContent = '✓ Loaded ' + loadedCount + ' repositories';
					progressInfo.style.color = 'var(--status-success)';
				} else {
					progressInfo.textContent = loadedCount + '+ repositories (loading...)';
					progressInfo.style.color = 'var(--text-secondary)';
				}
			})
			.catch(error => {
				console.error('Error loading repositories:', error);
				progressInfo.textContent = '✗ Error loading repositories';
				progressInfo.style.color = 'var(--status-failed)';
			});

		function escapeHtml(text) {
			const div = document.createElement('div');
			div.textContent = text;
			return div.innerHTML;
		}

		function getRoleName(permissions) {
			if (!permissions) {
				return '-';
			}

			// GitLab access levels: 10=Guest, 20=Reporter, 30=Developer, 40=Maintainer, 50=Owner
			// GitHub uses Admin/Push/Pull booleans
			const accessLevel = permissions.AccessLevel || 0;

			if (accessLevel >= 50 || permissions.Admin) {
				return 'Owner';
			} else if (accessLevel >= 40 || (permissions.Push && permissions.Admin === false)) {
				return 'Maintainer';
			} else if (accessLevel >= 30 || permissions.Push) {
				return 'Developer';
			} else if (accessLevel >= 20) {
				return 'Reporter';
			} else if (accessLevel >= 10 || permissions.Pull) {
				return 'Guest';
			}

			return '-';
		}

		function renderAllRepositories() {
			tbody.innerHTML = '';

			allRepositories.forEach(repo => {
				const row = document.createElement('tr');
				row.className = 'filterable';
				row.setAttribute('data-repository', repo.Project.Name);
				row.setAttribute('data-platform', repo.Project.Platform);
				row.setAttribute('data-is-fork', repo.Project.IsFork ? 'true' : 'false');

				let status = 'none';
				let statusDisplay = '<span class="status-badge canceled">NONE</span>';
				if (repo.Pipeline) {
					status = repo.Pipeline.Status;
					statusDisplay = '<span class="status-badge ' + status + '">' + status.toUpperCase() + '</span>';
				}
				row.setAttribute('data-status', status);

				let lastCommit = '-';
				if (repo.DefaultBranch && repo.DefaultBranch.LastCommitDate) {
					const commitDate = new Date(repo.DefaultBranch.LastCommitDate);
					lastCommit = formatTimeAgo(commitDate);
				}

				let committer = '-';
				if (repo.DefaultBranch && repo.DefaultBranch.CommitAuthor) {
					committer = escapeHtml(repo.DefaultBranch.CommitAuthor);
				}

				const branchCount = repo.BranchCount || 0;
				const openMRs = repo.OpenMRCount || 0;
				const draftMRs = repo.DraftMRCount || 0;

				// Display open MRs count with draft indication
				let mrDisplay = '-';
				if (openMRs > 0) {
					if (draftMRs > 0) {
						mrDisplay = openMRs + ' <span style="font-size: 11px; color: var(--text-secondary);">(' + draftMRs + ' draft)</span>';
					} else {
						mrDisplay = openMRs.toString();
					}
				}

				const repoDetailLink = '/repository?id=' + encodeURIComponent(repo.Project.ID);
				const platformLink = repo.Project.WebURL || '#';
				const platformBadge = '<a href="' + platformLink + '" target="_blank" rel="noopener noreferrer" class="platform-badge platform-' + repo.Project.Platform + '" title="Open on ' + repo.Project.Platform + '">' + repo.Project.Platform + '</a>';
				const forkBadge = repo.Project.IsFork ? '<span class="fork-badge" title="This is a forked repository">FORK</span>' : '';

				const favIcon = isFavorite(repo.Project.ID) ? '★' : '☆';
				const favClass = isFavorite(repo.Project.ID) ? 'favorite-star favorited' : 'favorite-star';
				const favTitle = isFavorite(repo.Project.ID) ? 'Remove from favorites' : 'Add to favorites';

				const roleName = getRoleName(repo.Project.Permissions);
				const roleDisplay = '<span style="font-size: 12px; color: var(--text-secondary);">' + roleName + '</span>';

				row.innerHTML = '<td>' +
					'<span class="' + favClass + '" data-repo-id="' + escapeHtml(repo.Project.ID) + '" title="' + favTitle + '" style="cursor: pointer; margin-right: 8px;">' + favIcon + '</span>' +
					'<a href="' + repoDetailLink + '" style="color: var(--text-primary); text-decoration: none; font-weight: 600;" class="repo-link"><strong>' + escapeHtml(repo.Project.Name) + '</strong></a> ' +
					forkBadge +
					'</td>' +
					'<td class="platform-cell">' + platformBadge + '</td>' +
					'<td class="count-cell">' + roleDisplay + '</td>' +
					'<td class="status-cell">' + statusDisplay + '</td>' +
					'<td class="count-cell">' + branchCount + '</td>' +
					'<td class="count-cell">' + mrDisplay + '</td>' +
					'<td class="committer-cell">' + committer + '</td>' +
					'<td class="commit-cell">' + lastCommit + '</td>';

				tbody.appendChild(row);
			});

			tbody.querySelectorAll('.favorite-star').forEach(star => {
				star.addEventListener('click', (e) => {
					e.preventDefault();
					e.stopPropagation();
					const repoId = star.getAttribute('data-repo-id');
					toggleFavorite(repoId);
				});
			});

			if (typeof applyFilters === 'function') {
				applyFilters();
			}
		}

		function formatTimeAgo(date) {
			if (!date || date.getTime() === 0 || date.getFullYear() < 1970) {
				return '-';
			}

			const now = new Date();
			const diffMs = now - date;

			if (diffMs < 0 || diffMs > 100 * 365 * 24 * 60 * 60 * 1000) {
				return '-';
			}

			const diffSecs = Math.floor(diffMs / 1000);
			const diffMins = Math.floor(diffSecs / 60);
			const diffHours = Math.floor(diffMins / 60);
			const diffDays = Math.floor(diffHours / 24);

			if (diffSecs < 60) return 'just now';
			if (diffMins < 60) return diffMins + ' min' + (diffMins !== 1 ? 's' : '') + ' ago';
			if (diffHours < 24) return diffHours + ' hour' + (diffHours !== 1 ? 's' : '') + ' ago';
			if (diffDays < 7) return diffDays + ' day' + (diffDays !== 1 ? 's' : '') + ' ago';
			if (diffDays < 30) {
				const weeks = Math.floor(diffDays / 7);
				return weeks + ' week' + (weeks !== 1 ? 's' : '') + ' ago';
			}
			if (diffDays < 365) {
				const months = Math.floor(diffDays / 30);
				return months + ' month' + (months !== 1 ? 's' : '') + ' ago';
			}
			const years = Math.floor(diffDays / 365);
			return years + ' year' + (years !== 1 ? 's' : '') + ' ago';
		}

		// Auto-refresh functionality
		if (REFRESH_INTERVAL_SECONDS > 0) {
			setInterval(() => {
				fetch('/api/repositories?limit=10000')
					.then(response => {
						if (!response.ok) {
							throw new Error('Failed to fetch repositories');
						}
						return response.json();
					})
					.then(data => {
						const newRepositories = data.repositories || [];
						const newTotalCount = data.pagination ? data.pagination.total : newRepositories.length;
						const newAllLoaded = data.pagination ? !data.pagination.hasNext : true;

						// Create a map of current repositories by ID for quick lookup
						const currentRepoMap = new Map();
						allRepositories.forEach(repo => {
							currentRepoMap.set(repo.Project.ID, repo);
						});

						// Create a map of new repositories by ID
						const newRepoMap = new Map();
						newRepositories.forEach(repo => {
							newRepoMap.set(repo.Project.ID, repo);
						});

						// Find added and removed repositories
						const addedRepos = newRepositories.filter(repo => !currentRepoMap.has(repo.Project.ID));
						const removedRepoIDs = Array.from(currentRepoMap.keys()).filter(id => !newRepoMap.has(id));

						// Update existing repositories and track changes
						let hasChanges = false;
						newRepositories.forEach(newRepo => {
							const existingRepo = currentRepoMap.get(newRepo.Project.ID);
							if (existingRepo) {
								// Check if data has changed
								if (JSON.stringify(existingRepo) !== JSON.stringify(newRepo)) {
									hasChanges = true;
									// Update the existing repo in allRepositories
									const index = allRepositories.findIndex(r => r.Project.ID === newRepo.Project.ID);
									if (index !== -1) {
										allRepositories[index] = newRepo;
									}
								}
							}
						});

						// Add new repositories
						if (addedRepos.length > 0) {
							hasChanges = true;
							allRepositories = allRepositories.concat(addedRepos);
						}

						// Remove deleted repositories
						if (removedRepoIDs.length > 0) {
							hasChanges = true;
							allRepositories = allRepositories.filter(repo => !removedRepoIDs.includes(repo.Project.ID));
						}

						// Update the progress info with current count
						totalCount = newTotalCount;
						loadedCount = allRepositories.length;
						allLoaded = newAllLoaded;

						if (allLoaded) {
							progressInfo.textContent = '✓ Loaded ' + loadedCount + ' repositories';
							progressInfo.style.color = 'var(--status-success)';
						} else {
							progressInfo.textContent = loadedCount + '+ repositories (loading...)';
							progressInfo.style.color = 'var(--text-secondary)';
						}

						// Re-sort and re-render if there were any changes
						if (hasChanges || addedRepos.length > 0 || removedRepoIDs.length > 0) {
							allRepositories.sort((a, b) => {
								const aFav = isFavorite(a.Project.ID);
								const bFav = isFavorite(b.Project.ID);
								if (aFav && !bFav) return -1;
								if (!aFav && bFav) return 1;

								if (!a.DefaultBranch || !a.DefaultBranch.LastCommitDate) return 1;
								if (!b.DefaultBranch || !b.DefaultBranch.LastCommitDate) return -1;

								const dateA = new Date(a.DefaultBranch.LastCommitDate);
								const dateB = new Date(b.DefaultBranch.LastCommitDate);

								if (dateA.getFullYear() < 1970) return 1;
								if (dateB.getFullYear() < 1970) return -1;

								return dateB - dateA;
							});

							renderAllRepositories();
						}
					})
					.catch(error => {
						console.error('Auto-refresh error:', error);
					});
			}, REFRESH_INTERVAL_SECONDS * 1000);
		}
	</script>
`
}
