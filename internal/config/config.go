package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds application configuration.
// Follows Single Responsibility - only holds configuration data.
type Config struct {
	Port int

	// GitLab configuration
	GitLabURL   string
	GitLabToken string

	// GitHub configuration
	GitHubURL   string
	GitHubToken string

	// Watched repositories (comma-separated list of project IDs)
	// Format for GitLab: project-id (e.g., "123,456")
	// Format for GitHub: owner/repo (e.g., "facebook/react,golang/go")
	WatchedRepos string
}

// Load loads configuration from environment variables.
func Load() (*Config, error) {
	port := 8080
	if portStr := os.Getenv("PORT"); portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			port = p
		}
	}

	return &Config{
		Port:         port,
		GitLabURL:    getEnvOrDefault("GITLAB_URL", "https://gitlab.com"),
		GitLabToken:  os.Getenv("GITLAB_TOKEN"),
		GitHubURL:    getEnvOrDefault("GITHUB_URL", "https://api.github.com"),
		GitHubToken:  os.Getenv("GITHUB_TOKEN"),
		WatchedRepos: os.Getenv("WATCHED_REPOS"),
	}, nil
}

// GetWatchedRepos returns the list of watched repository IDs.
func (c *Config) GetWatchedRepos() []string {
	if c.WatchedRepos == "" {
		return nil
	}

	repos := strings.Split(c.WatchedRepos, ",")
	result := make([]string, 0, len(repos))
	for _, repo := range repos {
		repo = strings.TrimSpace(repo)
		if repo != "" {
			result = append(result, repo)
		}
	}
	return result
}

// HasGitLabConfig returns true if GitLab is configured.
func (c *Config) HasGitLabConfig() bool {
	return c.GitLabToken != ""
}

// HasGitHubConfig returns true if GitHub is configured.
func (c *Config) HasGitHubConfig() bool {
	return c.GitHubToken != ""
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
