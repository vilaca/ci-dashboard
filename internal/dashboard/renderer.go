package dashboard

import (
	"io"

	"github.com/vilaca/ci-dashboard/internal/domain"
	"github.com/vilaca/ci-dashboard/internal/service"
)

// Renderer handles rendering responses to HTTP clients.
// This interface follows Interface Segregation Principle (SOLID-I).
type Renderer interface {
	RenderHealth(w io.Writer) error
	RenderRepositoriesSkeleton(w io.Writer, userProfiles []domain.UserProfile) error
	RenderRepositoryDetail(w io.Writer, repository service.RepositoryWithRuns, mrs []domain.MergeRequest, issues []domain.Issue) error
	RenderRepositoryDetailSkeleton(w io.Writer, repositoryID string) error
}

// HTMLRenderer implements Renderer for HTML responses.
type HTMLRenderer struct {
	// All HTML is embedded in methods, no external templates needed
}

// NewHTMLRenderer creates a new HTML renderer.
func NewHTMLRenderer() *HTMLRenderer {
	return &HTMLRenderer{}
}


func (r *HTMLRenderer) RenderHealth(w io.Writer) error {
	_, err := w.Write([]byte(`{"status":"ok"}`))
	return err
}


