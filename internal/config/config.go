package config

import (
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
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
	GitLabWatchedRepos string
	// Format for GitHub: owner/repo (e.g., "facebook/react,golang/go")
	GitHubWatchedRepos string

	// Display configuration
	RunsPerRepository    int // Number of recent runs to show per repository
	RecentPipelinesLimit int // Total number of pipelines to show in recent view

	// Cache configuration
	GitLabCacheDurationSeconds int // Duration to cache GitLab API responses (default: 1800 = 30 minutes)
	GitHubCacheDurationSeconds int // Duration to cache GitHub API responses (default: 1800 = 30 minutes)

	// Current user configuration (for filtering "your branches")
	GitLabCurrentUser string // GitLab username for filtering branches (from GITLAB_USER)
	GitHubCurrentUser string // GitHub username for filtering branches (from GITHUB_USER)
}

// yamlConfig represents the YAML file structure.
type yamlConfig struct {
	Port   int `yaml:"port"`
	GitLab struct {
		URL                  string   `yaml:"url"`
		Token                string   `yaml:"token"`
		WatchedRepos         []string `yaml:"watched_repos"`
		CacheDurationSeconds int      `yaml:"cache_duration_seconds"`
		CurrentUser          string   `yaml:"current_user"`
	} `yaml:"gitlab"`
	GitHub struct {
		URL                  string   `yaml:"url"`
		Token                string   `yaml:"token"`
		WatchedRepos         []string `yaml:"watched_repos"`
		CacheDurationSeconds int      `yaml:"cache_duration_seconds"`
		CurrentUser          string   `yaml:"current_user"`
	} `yaml:"github"`
	Display struct {
		RunsPerRepository    int `yaml:"runs_per_repository"`
		RecentPipelinesLimit int `yaml:"recent_pipelines_limit"`
	} `yaml:"display"`
}

// Load loads configuration from YAML file (if exists) and environment variables.
// Environment variables take precedence over YAML file values.
// Priority order: Environment Variables -> YAML File -> Default Values
func Load() (*Config, error) {
	var yc yamlConfig

	// Try to load YAML config file
	configFile := os.Getenv("CONFIG_FILE")
	if configFile == "" {
		// Try default locations
		for _, path := range []string{"config.yaml", "config.yml"} {
			if _, err := os.Stat(path); err == nil {
				configFile = path
				break
			}
		}
	}

	// Load YAML if file exists
	if configFile != "" {
		data, err := os.ReadFile(configFile)
		if err == nil {
			if err := yaml.Unmarshal(data, &yc); err != nil {
				return nil, err
			}
		}
	}

	// Load values with priority: Env -> YAML -> Default
	port := 8080
	if portStr := os.Getenv("PORT"); portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			port = p
		}
	} else if yc.Port != 0 {
		port = yc.Port
	}

	gitlabURL := getEnvOrDefault("GITLAB_URL", "")
	if gitlabURL == "" {
		if yc.GitLab.URL != "" {
			gitlabURL = yc.GitLab.URL
		} else {
			gitlabURL = "https://gitlab.com"
		}
	}

	gitlabToken := os.Getenv("GITLAB_TOKEN")
	if gitlabToken == "" {
		gitlabToken = yc.GitLab.Token
	}

	githubURL := getEnvOrDefault("GITHUB_URL", "")
	if githubURL == "" {
		if yc.GitHub.URL != "" {
			githubURL = yc.GitHub.URL
		} else {
			githubURL = "https://api.github.com"
		}
	}

	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		githubToken = yc.GitHub.Token
	}

	gitlabWatchedRepos := os.Getenv("GITLAB_WATCHED_REPOS")
	if gitlabWatchedRepos == "" {
		gitlabWatchedRepos = strings.Join(yc.GitLab.WatchedRepos, ",")
	}

	githubWatchedRepos := os.Getenv("GITHUB_WATCHED_REPOS")
	if githubWatchedRepos == "" {
		githubWatchedRepos = strings.Join(yc.GitHub.WatchedRepos, ",")
	}

	runsPerRepo := 3
	if runsStr := os.Getenv("RUNS_PER_REPOSITORY"); runsStr != "" {
		if r, err := strconv.Atoi(runsStr); err == nil && r > 0 {
			runsPerRepo = r
		}
	} else if yc.Display.RunsPerRepository != 0 {
		runsPerRepo = yc.Display.RunsPerRepository
	}

	recentLimit := 50
	if limitStr := os.Getenv("RECENT_PIPELINES_LIMIT"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			recentLimit = l
		}
	} else if yc.Display.RecentPipelinesLimit != 0 {
		recentLimit = yc.Display.RecentPipelinesLimit
	}

	gitlabCacheDuration := 1800 // 30 minutes default
	if cacheStr := os.Getenv("GITLAB_CACHE_DURATION_SECONDS"); cacheStr != "" {
		if c, err := strconv.Atoi(cacheStr); err == nil && c >= 0 {
			gitlabCacheDuration = c
		}
	} else if yc.GitLab.CacheDurationSeconds != 0 {
		gitlabCacheDuration = yc.GitLab.CacheDurationSeconds
	}

	githubCacheDuration := 1800 // 30 minutes default
	if cacheStr := os.Getenv("GITHUB_CACHE_DURATION_SECONDS"); cacheStr != "" {
		if c, err := strconv.Atoi(cacheStr); err == nil && c >= 0 {
			githubCacheDuration = c
		}
	} else if yc.GitHub.CacheDurationSeconds != 0 {
		githubCacheDuration = yc.GitHub.CacheDurationSeconds
	}

	// Load current user configuration with fallback
	currentUser := os.Getenv("CURRENT_USER") // Common fallback

	gitlabCurrentUser := os.Getenv("GITLAB_USER")
	if gitlabCurrentUser == "" {
		if yc.GitLab.CurrentUser != "" {
			gitlabCurrentUser = yc.GitLab.CurrentUser
		} else {
			gitlabCurrentUser = currentUser
		}
	}

	githubCurrentUser := os.Getenv("GITHUB_USER")
	if githubCurrentUser == "" {
		if yc.GitHub.CurrentUser != "" {
			githubCurrentUser = yc.GitHub.CurrentUser
		} else {
			githubCurrentUser = currentUser
		}
	}

	return &Config{
		Port:                       port,
		GitLabURL:                  gitlabURL,
		GitLabToken:                gitlabToken,
		GitHubURL:                  githubURL,
		GitHubToken:                githubToken,
		GitLabWatchedRepos:         gitlabWatchedRepos,
		GitHubWatchedRepos:         githubWatchedRepos,
		RunsPerRepository:          runsPerRepo,
		RecentPipelinesLimit:       recentLimit,
		GitLabCacheDurationSeconds: gitlabCacheDuration,
		GitHubCacheDurationSeconds: githubCacheDuration,
		GitLabCurrentUser:          gitlabCurrentUser,
		GitHubCurrentUser:          githubCurrentUser,
	}, nil
}

// GetGitLabWatchedRepos returns the list of watched GitLab repository IDs.
func (c *Config) GetGitLabWatchedRepos() []string {
	if c.GitLabWatchedRepos == "" {
		return nil
	}

	repos := strings.Split(c.GitLabWatchedRepos, ",")
	result := make([]string, 0, len(repos))
	for _, repo := range repos {
		repo = strings.TrimSpace(repo)
		if repo != "" {
			result = append(result, repo)
		}
	}
	return result
}

// GetGitHubWatchedRepos returns the list of watched GitHub repository IDs.
func (c *Config) GetGitHubWatchedRepos() []string {
	if c.GitHubWatchedRepos == "" {
		return nil
	}

	repos := strings.Split(c.GitHubWatchedRepos, ",")
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
