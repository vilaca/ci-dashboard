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
	</style>`
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

// htmlFooter returns the common HTML footer with theme toggle script.
func htmlFooter() string {
	return themeToggleScript() + `
</body>
</html>`
}

// buildNavigation returns the common navigation bar HTML.
func buildNavigation() string {
	return `<div class="nav">
			<a href="/">Repositories</a>
			<a href="/pipelines">Recent Pipelines</a>
			<a href="/api/pipelines">API (JSON)</a>
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
func externalLink(url, text string) string {
	return fmt.Sprintf(`<a href="%s" target="_blank" rel="noopener noreferrer">%s</a>`,
		escapeHTML(url), escapeHTML(text))
}

// pageCSS returns page-specific CSS styles.
func pageCSS(styles string) string {
	return fmt.Sprintf("<style>%s</style>", styles)
}

