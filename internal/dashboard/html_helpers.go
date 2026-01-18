package dashboard

import (
	"fmt"
	"strings"
)

// htmlHead returns the common HTML head section with proper meta tags.
func htmlHead(title, description string) string {
	if description == "" {
		description = "Monitor CI/CD pipelines from GitLab and GitHub in one unified dashboard"
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0, viewport-fit=cover">
	<meta name="description" content="%s">
	<meta name="author" content="CI Dashboard">

	<!-- Open Graph / Social Media -->
	<meta property="og:type" content="website">
	<meta property="og:title" content="%s">
	<meta property="og:description" content="%s">

	<!-- Favicon -->
	<link rel="icon" type="image/svg+xml" href="data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 100 100'><text y='0.9em' font-size='90'>📊</text></svg>">

	<title>%s - CI Dashboard</title>
	%s
</head>`, description, title, description, title, commonCSS())
}

// commonCSS returns the shared CSS styles used across all pages.
func commonCSS() string {
	return `<style>
		/* CSS Variables for theming */
		:root {
			--bg-primary: #f5f5f5;
			--bg-secondary: white;
			--text-primary: #333;
			--text-secondary: #666;
			--link-color: #0066cc;
			--button-bg: #0066cc;
			--button-hover: #0052a3;
			--border-color: #e0e0e0;
			--shadow: rgba(0,0,0,0.1);
			--success-bg: #d4edda;
			--success-text: #155724;
			--failed-bg: #f8d7da;
			--failed-text: #721c24;
			--running-bg: #d1ecf1;
			--running-text: #0c5460;
			--pending-bg: #fff3cd;
			--pending-text: #856404;
			--canceled-bg: #e2e3e5;
			--canceled-text: #383d41;
		}

		[data-theme="dark"] {
			--bg-primary: #1a1a1a;
			--bg-secondary: #2d2d2d;
			--text-primary: #e0e0e0;
			--text-secondary: #b0b0b0;
			--link-color: #4d9fff;
			--button-bg: #4d9fff;
			--button-hover: #3d89ef;
			--border-color: #404040;
			--shadow: rgba(0,0,0,0.3);
			--success-bg: #1e4620;
			--success-text: #90ee90;
			--failed-bg: #4a1a1a;
			--failed-text: #ff6b6b;
			--running-bg: #1a3a4a;
			--running-text: #5dade2;
			--pending-bg: #4a3a1a;
			--pending-text: #ffd966;
			--canceled-bg: #2a2a2a;
			--canceled-text: #999;
		}

		/* Base styles */
		* {
			box-sizing: border-box;
			margin: 0;
			padding: 0;
		}

		body {
			font-family: system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, sans-serif;
			margin: 0;
			padding: 20px;
			background: var(--bg-primary);
			color: var(--text-primary);
			transition: background-color 0.3s, color 0.3s;
			line-height: 1.6;
		}

		/* Container */
		.container {
			max-width: 1200px;
			margin: 0 auto;
		}

		/* Typography */
		h1 {
			color: var(--text-primary);
			margin-bottom: 10px;
			font-size: 2rem;
			font-weight: 600;
		}

		h2 {
			color: var(--text-primary);
			font-size: 1.5rem;
			font-weight: 600;
		}

		/* Navigation */
		.nav {
			margin-bottom: 30px;
			display: flex;
			align-items: center;
			gap: 15px;
			flex-wrap: wrap;
		}

		.nav a {
			color: var(--link-color);
			text-decoration: none;
			transition: opacity 0.2s;
		}

		.nav a:hover {
			text-decoration: underline;
			opacity: 0.8;
		}

		.nav a:focus {
			outline: 2px solid var(--link-color);
			outline-offset: 2px;
		}

		/* Theme Toggle Button */
		.theme-toggle {
			padding: 8px 16px;
			background: var(--button-bg);
			color: white;
			border: none;
			border-radius: 4px;
			cursor: pointer;
			font-size: 14px;
			font-weight: 500;
			transition: background-color 0.3s, transform 0.1s;
		}

		.theme-toggle:hover {
			background: var(--button-hover);
		}

		.theme-toggle:active {
			transform: scale(0.98);
		}

		.theme-toggle:focus {
			outline: 2px solid var(--link-color);
			outline-offset: 2px;
		}

		/* Status Badges */
		.status-badge {
			padding: 4px 10px;
			border-radius: 4px;
			font-size: 12px;
			font-weight: 500;
			text-transform: uppercase;
			display: inline-block;
		}

		.status-badge.success {
			background: var(--success-bg);
			color: var(--success-text);
		}

		.status-badge.failed {
			background: var(--failed-bg);
			color: var(--failed-text);
		}

		.status-badge.running {
			background: var(--running-bg);
			color: var(--running-text);
		}

		.status-badge.pending {
			background: var(--pending-bg);
			color: var(--pending-text);
		}

		.status-badge.canceled {
			background: var(--canceled-bg);
			color: var(--canceled-text);
		}

		/* Links */
		a {
			color: var(--link-color);
			transition: opacity 0.2s;
		}

		a:hover {
			opacity: 0.8;
		}

		a:focus {
			outline: 2px solid var(--link-color);
			outline-offset: 2px;
		}

		/* Empty State */
		.empty {
			text-align: center;
			padding: 40px;
			color: var(--text-secondary);
			font-size: 16px;
		}

		/* Utility */
		.meta-text {
			color: var(--text-secondary);
			font-size: 14px;
		}

		/* Filters */
		.filters {
			background: var(--bg-secondary);
			padding: 20px;
			border-radius: 8px;
			box-shadow: 0 2px 4px var(--shadow);
			margin-bottom: 20px;
			display: flex;
			flex-wrap: wrap;
			gap: 15px;
			align-items: center;
		}

		.filter-group {
			display: flex;
			flex-direction: column;
			gap: 5px;
			min-width: 200px;
		}

		.filter-group label {
			font-size: 12px;
			font-weight: 500;
			color: var(--text-secondary);
			text-transform: uppercase;
		}

		.filter-group select {
			padding: 8px 12px;
			border: 1px solid var(--border-color);
			border-radius: 4px;
			font-size: 14px;
			background: var(--bg-primary);
			color: var(--text-primary);
			transition: border-color 0.2s;
			cursor: pointer;
		}

		.filter-group select:focus {
			outline: none;
			border-color: var(--link-color);
		}

		.filter-group select:hover {
			border-color: var(--link-color);
		}

		.filter-count {
			margin-left: auto;
			font-size: 14px;
			color: var(--text-secondary);
			font-weight: 500;
		}

		/* Loading Progress Bar */
		.loading-progress-bar {
			position: fixed;
			top: 0;
			left: 0;
			width: 0%;
			height: 4px;
			background: linear-gradient(90deg, var(--link-color), var(--button-hover));
			z-index: 10000;
			transition: width 0.3s ease-out, opacity 0.3s ease-out;
			opacity: 1;
			box-shadow: 0 2px 5px rgba(0, 0, 0, 0.2);
		}

		.loading-progress-bar.complete {
			width: 100%;
			opacity: 0;
		}

		/* Loading Spinner - only shown for slow loads */
		.loading-overlay {
			position: fixed;
			top: 0;
			left: 0;
			width: 100%;
			height: 100%;
			background: rgba(0, 0, 0, 0.3);
			backdrop-filter: blur(3px);
			display: none;
			justify-content: center;
			align-items: center;
			z-index: 9999;
			opacity: 0;
			transition: opacity 0.3s ease-out;
		}

		.loading-overlay.visible {
			display: flex;
			opacity: 1;
		}

		.loading-overlay.hidden {
			opacity: 0;
			pointer-events: none;
		}

		.loading-spinner {
			width: 60px;
			height: 60px;
			border: 5px solid rgba(255, 255, 255, 0.3);
			border-top-color: var(--link-color);
			border-radius: 50%;
			animation: spin 0.8s linear infinite;
			background: var(--bg-secondary);
			box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
		}

		@keyframes spin {
			to { transform: rotate(360deg); }
		}
	</style>`
}

// loadingSpinner returns the HTML for the loading overlay and progress bar.
func loadingSpinner() string {
	return `<div class="loading-progress-bar" id="loading-progress-bar"></div>
	<div class="loading-overlay" id="loading-overlay">
		<div class="loading-spinner"></div>
	</div>`
}

// loadingScript returns JavaScript to animate the progress bar and show spinner for slow loads.
func loadingScript() string {
	return `<script>
		(function() {
			const progressBar = document.getElementById('loading-progress-bar');
			const overlay = document.getElementById('loading-overlay');
			let pageLoaded = false;
			let spinnerTimeout = null;

			// Show spinner after 500ms if page is still loading
			spinnerTimeout = setTimeout(function() {
				if (!pageLoaded && overlay) {
					overlay.classList.add('visible');
				}
			}, 500);

			// Simulate progress bar animation
			function animateProgress() {
				if (!progressBar) return;

				// Quick start to 30%
				setTimeout(function() {
					if (!pageLoaded) progressBar.style.width = '30%';
				}, 100);

				// Progress to 60%
				setTimeout(function() {
					if (!pageLoaded) progressBar.style.width = '60%';
				}, 400);

				// Progress to 85%
				setTimeout(function() {
					if (!pageLoaded) progressBar.style.width = '85%';
				}, 800);
			}

			// Start progress animation immediately
			animateProgress();

			// Complete progress and hide spinner when page is fully loaded
			window.addEventListener('load', function() {
				pageLoaded = true;

				// Cancel spinner timeout if it hasn't fired yet
				if (spinnerTimeout) {
					clearTimeout(spinnerTimeout);
				}

				// Complete progress bar
				if (progressBar) {
					progressBar.style.width = '100%';
					setTimeout(function() {
						progressBar.classList.add('complete');
						// Remove from DOM after fade out
						setTimeout(function() {
							progressBar.style.display = 'none';
						}, 400);
					}, 200);
				}

				// Hide loading spinner if it was shown
				if (overlay) {
					overlay.classList.add('hidden');
					overlay.classList.remove('visible');
					// Remove from DOM after transition
					setTimeout(function() {
						overlay.style.display = 'none';
					}, 400);
				}

				// Setup navigation loading indicators for all internal links
				setupNavigationLoading();
			});

			// Show loading indicators when clicking internal links
			function setupNavigationLoading() {
				document.addEventListener('click', function(e) {
					// Find the clicked link (could be nested in the clicked element)
					let target = e.target;
					while (target && target.tagName !== 'A') {
						target = target.parentElement;
					}

					// Check if it's an internal navigation link
					if (target && target.tagName === 'A' && target.href) {
						const url = new URL(target.href);
						const currentUrl = new URL(window.location.href);

						// Only show loading for same-origin navigation (not external links)
						if (url.origin === currentUrl.origin &&
						    !target.hasAttribute('target') &&
						    !e.ctrlKey && !e.metaKey && !e.shiftKey) {

							// Show progress bar and spinner immediately
							showNavigationLoading();
						}
					}
				});
			}

			// Show loading indicators for navigation
			function showNavigationLoading() {
				// Create or reuse progress bar
				let navProgressBar = document.getElementById('nav-progress-bar');
				if (!navProgressBar) {
					navProgressBar = document.createElement('div');
					navProgressBar.id = 'nav-progress-bar';
					navProgressBar.className = 'loading-progress-bar';
					document.body.insertBefore(navProgressBar, document.body.firstChild);
				}
				navProgressBar.style.display = 'block';
				navProgressBar.style.width = '0%';
				navProgressBar.style.opacity = '1';

				// Create or reuse overlay
				let navOverlay = document.getElementById('nav-overlay');
				if (!navOverlay) {
					navOverlay = document.createElement('div');
					navOverlay.id = 'nav-overlay';
					navOverlay.className = 'loading-overlay';
					navOverlay.innerHTML = '<div class="loading-spinner"></div>';
					document.body.appendChild(navOverlay);
				}

				// Start animations
				setTimeout(function() {
					navProgressBar.style.width = '30%';
				}, 50);

				setTimeout(function() {
					navProgressBar.style.width = '60%';
				}, 300);

				setTimeout(function() {
					navProgressBar.style.width = '80%';
				}, 600);

				// Show spinner after 500ms if still loading
				setTimeout(function() {
					if (navOverlay) {
						navOverlay.classList.add('visible');
					}
				}, 500);
			}

			// Also listen for browser back/forward buttons
			window.addEventListener('beforeunload', function() {
				showNavigationLoading();
			});

			// Handle form submissions
			document.addEventListener('submit', function(e) {
				const form = e.target;
				if (form && form.method !== 'GET') {
					showNavigationLoading();
				}
			});
		})();
	</script>`
}

// themeToggleScript returns the common theme toggle JavaScript.
func themeToggleScript() string {
	return `<script>
		function toggleTheme() {
			const html = document.documentElement;
			const currentTheme = html.getAttribute('data-theme');
			const newTheme = currentTheme === 'dark' ? 'light' : 'dark';
			html.setAttribute('data-theme', newTheme);
			localStorage.setItem('theme', newTheme);
			updateToggleButton(newTheme);
		}

		function updateToggleButton(theme) {
			const button = document.querySelector('.theme-toggle');
			if (button) {
				button.textContent = theme === 'dark' ? '☀️ Light Mode' : '🌙 Dark Mode';
				button.setAttribute('aria-label', theme === 'dark' ? 'Switch to light mode' : 'Switch to dark mode');
			}
		}

		// Initialize theme from localStorage
		(function() {
			const savedTheme = localStorage.getItem('theme') || 'light';
			document.documentElement.setAttribute('data-theme', savedTheme);
			updateToggleButton(savedTheme);
		})();
	</script>`
}

// filterScript returns the common table filtering JavaScript.
func filterScript() string {
	return `<script>
		function setupFilters(tableId, filters) {
			const table = document.getElementById(tableId);
			if (!table) return;

			const rows = table.querySelectorAll('tbody tr');

			// Populate all dropdowns initially
			filters.forEach(filter => {
				populateDropdown(filter, rows, filters, rows);
			});

			// Add change listeners
			filters.forEach(filter => {
				const select = document.getElementById(filter.inputId);
				if (!select) return;

				select.addEventListener('change', () => {
					filterTable(rows, filters);
					updateCascadingDropdowns(rows, filters);
				});
			});

			// Initial count
			updateCount(rows);
		}

		function populateDropdown(filter, allRows, allFilters, visibleRows) {
			const select = document.getElementById(filter.inputId);
			if (!select) return;

			const currentValue = select.value;
			const uniqueValues = new Set();

			visibleRows.forEach(row => {
				const value = row.getAttribute('data-' + filter.attr);
				if (value && value.trim() !== '') {
					uniqueValues.add(value.trim());
				}
			});

			// Sort values alphabetically
			const sortedValues = Array.from(uniqueValues).sort((a, b) =>
				a.toLowerCase().localeCompare(b.toLowerCase())
			);

			// Clear and rebuild options
			select.innerHTML = '<option value="">All</option>';

			// Add options
			sortedValues.forEach(value => {
				const option = document.createElement('option');
				option.value = value;
				option.textContent = value;
				select.appendChild(option);
			});

			// Restore previous selection if still valid
			if (currentValue && sortedValues.includes(currentValue)) {
				select.value = currentValue;
			} else if (currentValue && currentValue !== '') {
				// Current value no longer valid, reset to "All"
				select.value = '';
			}
		}

		function updateCascadingDropdowns(rows, filters) {
			// Get current filter values
			const filterValues = {};
			filters.forEach(filter => {
				const select = document.getElementById(filter.inputId);
				if (select) {
					filterValues[filter.attr] = select.value.toLowerCase();
				}
			});

			// Find rows that match current filters
			const visibleRows = Array.from(rows).filter(row => {
				for (const [attr, value] of Object.entries(filterValues)) {
					if (value === '') continue;
					const rowValue = (row.getAttribute('data-' + attr) || '').toLowerCase();
					if (rowValue !== value) {
						return false;
					}
				}
				return true;
			});

			// Update each dropdown with available options based on visible rows
			filters.forEach(filter => {
				populateDropdown(filter, rows, filters, visibleRows);
			});
		}

		function filterTable(rows, filters) {
			const filterValues = {};
			filters.forEach(filter => {
				const select = document.getElementById(filter.inputId);
				if (select) {
					filterValues[filter.attr] = select.value.toLowerCase();
				}
			});

			rows.forEach(row => {
				let show = true;

				for (const [attr, value] of Object.entries(filterValues)) {
					if (value === '') continue;

					const rowValue = (row.getAttribute('data-' + attr) || '').toLowerCase();
					if (rowValue !== value) {
						show = false;
						break;
					}
				}

				row.style.display = show ? '' : 'none';
			});

			updateCount(rows);
		}

		function updateCount(rows) {
			const visibleCount = Array.from(rows).filter(row => row.style.display !== 'none').length;
			const countElement = document.querySelector('.filter-count');
			if (countElement) {
				countElement.textContent = visibleCount + ' of ' + rows.length + ' items';
			}
		}
	</script>`
}

// htmlFooter returns the common HTML footer with all scripts.
func htmlFooter() string {
	return loadingScript() + themeToggleScript() + filterScript() + `
</body>
</html>`
}

// buildNavigation returns the common navigation bar HTML.
func buildNavigation() string {
	return `<div class="nav">
			<a href="/">Repositories</a>
			<a href="/pipelines">Recent Pipelines</a>
			<a href="/pipelines/failed">Failed Pipelines</a>
			<a href="/mrs">Open MRs/PRs</a>
			<a href="/issues">Open Issues</a>
			<button class="theme-toggle" onclick="toggleTheme()" aria-label="Toggle theme">🌙 Dark Mode</button>
		</div>`
}

// escapeHTML escapes special HTML characters to prevent XSS.
func escapeHTML(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&#39;",
	)
	return replacer.Replace(s)
}

// externalLink creates a safe external link with proper security attributes.
// Automatically detects platform from URL and generates appropriate text.
func externalLink(url, text string) string {
	// Detect platform from URL
	linkText := text
	if strings.Contains(text, "View") || strings.Contains(text, "→") {
		if strings.Contains(url, "github.com") {
			linkText = "Open on GitHub →"
		} else if strings.Contains(url, "gitlab.com") || strings.Contains(url, "gitlab.") {
			linkText = "Open on GitLab →"
		}
	}

	return fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener noreferrer">%s</a>`,
		escapeHTML(url), escapeHTML(linkText))
}

// pageCSS returns page-specific CSS styles.
func pageCSS(styles string) string {
	return fmt.Sprintf("<style>%s</style>", styles)
}

