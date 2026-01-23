package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/vilaca/ci-dashboard/internal/domain"
)

// CacheData represents the structure of cached data.
type CacheData struct {
	Timestamp   time.Time               `json:"timestamp"`
	Projects    []domain.Project        `json:"projects"`
	Pipelines   []domain.Pipeline       `json:"pipelines,omitempty"`
	Branches    []domain.Branch         `json:"branches,omitempty"`
	MRs         []domain.MergeRequest   `json:"merge_requests,omitempty"`
	Issues      []domain.Issue          `json:"issues,omitempty"`
	Profiles    []domain.UserProfile    `json:"user_profiles,omitempty"`
}

// FileCache provides persistent file-based caching for repository data.
// Follows Single Responsibility Principle - only handles file-based caching.
type FileCache struct {
	filePath string
	mu       sync.RWMutex
	logger   Logger
}

// NewFileCache creates a new file cache.
// Follows Dependency Injection pattern.
func NewFileCache(filePath string, logger Logger) *FileCache {
	return &FileCache{
		filePath: filePath,
		logger:   logger,
	}
}

// Load loads cached data from the file.
// Returns nil if file doesn't exist or is invalid.
func (c *FileCache) Load() (*CacheData, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Check if file exists
	if _, err := os.Stat(c.filePath); os.IsNotExist(err) {
		c.logger.Printf("File cache: No cache file found at %s", c.filePath)
		return nil, nil
	}

	// Read file
	data, err := os.ReadFile(c.filePath)
	if err != nil {
		c.logger.Printf("File cache: Failed to read cache file: %v", err)
		return nil, err
	}

	// Parse JSON
	var cacheData CacheData
	if err := json.Unmarshal(data, &cacheData); err != nil {
		c.logger.Printf("File cache: Failed to parse cache file: %v", err)
		return nil, err
	}

	age := time.Since(cacheData.Timestamp)
	c.logger.Printf("File cache: Loaded cache from %s (age: %v, projects: %d)",
		c.filePath, age.Round(time.Second), len(cacheData.Projects))

	return &cacheData, nil
}

// Save saves cached data to the file.
func (c *FileCache) Save(data *CacheData) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Set timestamp
	data.Timestamp = time.Now()

	// Marshal to JSON with indentation for readability
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		c.logger.Printf("File cache: Failed to marshal data: %v", err)
		return err
	}

	// Ensure directory exists
	dir := filepath.Dir(c.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		c.logger.Printf("File cache: Failed to create directory %s: %v", dir, err)
		return err
	}

	// Write to temporary file first (atomic write)
	tempFile := c.filePath + ".tmp"
	if err := os.WriteFile(tempFile, jsonData, 0644); err != nil {
		c.logger.Printf("File cache: Failed to write temp file: %v", err)
		return err
	}

	// Rename (atomic operation on most filesystems)
	if err := os.Rename(tempFile, c.filePath); err != nil {
		c.logger.Printf("File cache: Failed to rename temp file: %v", err)
		os.Remove(tempFile) // Cleanup temp file
		return err
	}

	c.logger.Printf("File cache: Saved cache to %s (projects: %d)", c.filePath, len(data.Projects))
	return nil
}

// Clear removes the cache file.
func (c *FileCache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := os.Remove(c.filePath); err != nil && !os.IsNotExist(err) {
		c.logger.Printf("File cache: Failed to remove cache file: %v", err)
		return err
	}

	c.logger.Printf("File cache: Cleared cache file")
	return nil
}
