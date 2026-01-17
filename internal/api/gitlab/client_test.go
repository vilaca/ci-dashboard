package gitlab

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/vilaca/ci-dashboard/internal/api"
)

// mockHTTPClient is a test double for HTTPClient.
// Follows FIRST principles - tests are Fast and Independent.
type mockHTTPClient struct {
	doFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return m.doFunc(req)
}

// TestGetProjects tests retrieving projects from GitLab.
// Follows AAA (Arrange, Act, Assert) pattern.
func TestGetProjects(t *testing.T) {
	// Arrange
	responseBody := `[
		{"id": 123, "name": "test-project", "web_url": "https://gitlab.com/user/test-project"}
	]`

	mockHTTP := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			// Verify request setup
			if req.Header.Get("PRIVATE-TOKEN") == "" {
				t.Error("expected PRIVATE-TOKEN header to be set")
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(responseBody)),
			}, nil
		},
	}

	client := NewClient(api.ClientConfig{
		BaseURL: "https://gitlab.com",
		Token:   "test-token",
	}, mockHTTP)

	// Act
	projects, err := client.GetProjects(context.Background())

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}

	if projects[0].ID != "123" {
		t.Errorf("expected project ID '123', got '%s'", projects[0].ID)
	}

	if projects[0].Name != "test-project" {
		t.Errorf("expected project name 'test-project', got '%s'", projects[0].Name)
	}

	if projects[0].Platform != "gitlab" {
		t.Errorf("expected platform 'gitlab', got '%s'", projects[0].Platform)
	}
}

// TestGetProjects_APIError tests error handling when API returns error.
func TestGetProjects_APIError(t *testing.T) {
	// Arrange
	mockHTTP := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(bytes.NewBufferString(`{"error":"unauthorized"}`)),
			}, nil
		},
	}

	client := NewClient(api.ClientConfig{
		BaseURL: "https://gitlab.com",
		Token:   "invalid-token",
	}, mockHTTP)

	// Act
	projects, err := client.GetProjects(context.Background())

	// Assert
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if projects != nil {
		t.Errorf("expected nil projects on error, got %v", projects)
	}

	if !strings.Contains(err.Error(), "401") {
		t.Errorf("expected error to mention status code 401, got: %v", err)
	}
}

// TestGetLatestPipeline tests retrieving the latest pipeline.
func TestGetLatestPipeline(t *testing.T) {
	// Arrange
	responseBody := `[
		{
			"id": 456,
			"status": "success",
			"ref": "main",
			"web_url": "https://gitlab.com/user/test-project/-/pipelines/456",
			"created_at": "2024-01-01T10:00:00Z",
			"updated_at": "2024-01-01T10:05:00Z"
		}
	]`

	mockHTTP := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(responseBody)),
			}, nil
		},
	}

	client := NewClient(api.ClientConfig{
		BaseURL: "https://gitlab.com",
		Token:   "test-token",
	}, mockHTTP)

	// Act
	pipeline, err := client.GetLatestPipeline(context.Background(), "123", "main")

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if pipeline == nil {
		t.Fatal("expected pipeline, got nil")
	}

	if pipeline.ID != "456" {
		t.Errorf("expected pipeline ID '456', got '%s'", pipeline.ID)
	}

	if pipeline.Branch != "main" {
		t.Errorf("expected branch 'main', got '%s'", pipeline.Branch)
	}

	if pipeline.Status != "success" {
		t.Errorf("expected status 'success', got '%s'", pipeline.Status)
	}
}

// TestGetLatestPipeline_NoPipelines tests when no pipelines exist.
func TestGetLatestPipeline_NoPipelines(t *testing.T) {
	// Arrange
	mockHTTP := &mockHTTPClient{
		doFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(`[]`)),
			}, nil
		},
	}

	client := NewClient(api.ClientConfig{
		BaseURL: "https://gitlab.com",
		Token:   "test-token",
	}, mockHTTP)

	// Act
	pipeline, err := client.GetLatestPipeline(context.Background(), "123", "main")

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if pipeline != nil {
		t.Errorf("expected nil pipeline, got %v", pipeline)
	}
}

// TestConvertStatus tests status conversion from GitLab to domain.
func TestConvertStatus(t *testing.T) {
	tests := []struct {
		name           string
		gitlabStatus   string
		expectedStatus string
	}{
		{"pending", "pending", "pending"},
		{"running", "running", "running"},
		{"success", "success", "success"},
		{"failed", "failed", "failed"},
		{"canceled", "canceled", "canceled"},
		{"skipped", "skipped", "skipped"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange & Act
			status := convertStatus(tt.gitlabStatus)

			// Assert
			if string(status) != tt.expectedStatus {
				t.Errorf("expected status '%s', got '%s'", tt.expectedStatus, status)
			}
		})
	}
}
