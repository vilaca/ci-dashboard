package api

import (
	"context"
	"net/http"
)

const (
	// MaxConcurrentRequests limits concurrent API requests to avoid overwhelming the API
	MaxConcurrentRequests = 5
	// MaxRetryAttempts is the maximum number of retry attempts for failed requests
	MaxRetryAttempts = 3
	// DefaultPageSize is the default number of items per page
	DefaultPageSize = 100
)

// HTTPClient interface for HTTP operations (allows mocking in tests).
// Follows Interface Segregation Principle.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// BaseClient contains common fields and functionality for all API clients.
// Follows DRY principle by extracting shared code.
type BaseClient struct {
	BaseURL    string
	Token      string
	HTTPClient HTTPClient
	Semaphore  chan struct{} // Limits concurrent requests
}

// NewBaseClient creates a new base client with rate limiting.
func NewBaseClient(baseURL, token string, httpClient HTTPClient) *BaseClient {
	return &BaseClient{
		BaseURL:    baseURL,
		Token:      token,
		HTTPClient: httpClient,
		Semaphore:  make(chan struct{}, MaxConcurrentRequests),
	}
}

// DoRateLimited performs an operation with rate limiting via semaphore.
// This method is used by platform-specific clients to wrap API calls.
func (c *BaseClient) DoRateLimited(ctx context.Context, fn func() (interface{}, error)) (interface{}, error) {
	// Acquire semaphore (rate limiting)
	select {
	case c.Semaphore <- struct{}{}:
		defer func() { <-c.Semaphore }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Execute the request
	return fn()
}
