package config

import (
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Default configuration values
const (
	DefaultPort                         = 8080
	DefaultRunsPerRepository            = 3
	DefaultRecentPipelinesLimit         = 50
	DefaultCacheDurationSeconds         = 1800  // 30 minutes
	DefaultStaleCacheTTLSeconds         = 86400 // 24 hours
	DefaultBackgroundRefreshSeconds     = 300   // 5 minutes
	DefaultUIRefreshIntervalSeconds     = 5
	DefaultGitLabURL                    = "https://gitlab.com"
	DefaultGitHubURL                    = "https://api.github.com"
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
	StaleCacheTTLSeconds       int // How long to serve stale cache data (default: 86400 = 24 hours)

	// Background refresh configuration
	BackgroundRefreshIntervalSeconds int // How often to refresh all caches in background (default: 300 = 5 minutes)

	// UI configuration
	UIRefreshIntervalSeconds int // How often UI auto-refreshes data in seconds (default: 5)

	// Current user configuration (for filtering "your branches")
	GitLabCurrentUser string // GitLab username for filtering branches (from GITLAB_USER)
	GitHubCurrentUser string // GitHub username for filtering branches (from GITHUB_USER)

	// Repository filtering
	FilterUserRepos bool // If true, only fetch repositories where user has membership (default: false - disabled until permissions API is fully working)
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
	Cache struct {
		StaleTTLSeconds int `yaml:"stale_ttl_seconds"`
	} `yaml:"cache"`
	Background struct {
		RefreshIntervalSeconds int `yaml:"refresh_interval_seconds"`
	} `yaml:"background"`
	UI struct {
		RefreshIntervalSeconds int `yaml:"refresh_interval_seconds"`
	} `yaml:"ui"`
	Filter struct {
		UserRepos bool `yaml:"user_repos"`
	} `yaml:"filter"`
}

// loadIntConfig loads an integer configuration value with fallback priority:
// 1. Environment variable (if set and valid)
// 2. YAML config value (if non-zero)
// 3. Default value
func loadIntConfig(envVar string, yamlValue int, defaultValue int, validator func(int) bool) int {
	// Try environment variable first
	if envStr := os.Getenv(envVar); envStr != "" {
		if val, err := strconv.Atoi(envStr); err == nil && validator(val) {
			return val
		}
	}

	// Fall back to YAML config if set
	if yamlValue != 0 {
		return yamlValue
	}

	// Use default value
	return defaultValue
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
	port := DefaultPort
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
			gitlabURL = DefaultGitLabURL
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
			githubURL = DefaultGitHubURL
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

	runsPerRepo := loadIntConfig("RUNS_PER_REPOSITORY", yc.Display.RunsPerRepository, DefaultRunsPerRepository, func(v int) bool { return v > 0 })

	recentLimit := loadIntConfig("RECENT_PIPELINES_LIMIT", yc.Display.RecentPipelinesLimit, DefaultRecentPipelinesLimit, func(v int) bool { return v > 0 })

	gitlabCacheDuration := loadIntConfig("GITLAB_CACHE_DURATION_SECONDS", yc.GitLab.CacheDurationSeconds, DefaultCacheDurationSeconds, func(v int) bool { return v >= 0 })

	githubCacheDuration := loadIntConfig("GITHUB_CACHE_DURATION_SECONDS", yc.GitHub.CacheDurationSeconds, DefaultCacheDurationSeconds, func(v int) bool { return v >= 0 })

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

	uiRefreshInterval := loadIntConfig("UI_REFRESH_INTERVAL_SECONDS", yc.UI.RefreshIntervalSeconds, DefaultUIRefreshIntervalSeconds, func(v int) bool { return v > 0 })

	staleCacheTTL := loadIntConfig("STALE_CACHE_TTL_SECONDS", yc.Cache.StaleTTLSeconds, DefaultStaleCacheTTLSeconds, func(v int) bool { return v > 0 })

	backgroundRefreshInterval := loadIntConfig("BACKGROUND_REFRESH_INTERVAL_SECONDS", yc.Background.RefreshIntervalSeconds, DefaultBackgroundRefreshSeconds, func(v int) bool { return v > 0 })

	// Load filter configuration (DISABLED by default until permissions are properly populated)
	// Priority: Environment variable -> Default (false)
	// To enable, set FILTER_USER_REPOS=true or FILTER_USER_REPOS=1
	filterUserRepos := false
	if envFilter := os.Getenv("FILTER_USER_REPOS"); envFilter != "" {
		filterUserRepos = envFilter == "true" || envFilter == "1"
	}

	return &Config{
		Port:                             port,
		GitLabURL:                        gitlabURL,
		GitLabToken:                      gitlabToken,
		GitHubURL:                        githubURL,
		GitHubToken:                      githubToken,
		GitLabWatchedRepos:               gitlabWatchedRepos,
		GitHubWatchedRepos:               githubWatchedRepos,
		RunsPerRepository:                runsPerRepo,
		RecentPipelinesLimit:             recentLimit,
		GitLabCacheDurationSeconds:       gitlabCacheDuration,
		GitHubCacheDurationSeconds:       githubCacheDuration,
		StaleCacheTTLSeconds:             staleCacheTTL,
		BackgroundRefreshIntervalSeconds: backgroundRefreshInterval,
		UIRefreshIntervalSeconds:         uiRefreshInterval,
		GitLabCurrentUser:                gitlabCurrentUser,
		GitHubCurrentUser:                githubCurrentUser,
		FilterUserRepos:                  filterUserRepos,
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
