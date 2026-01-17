package dashboard

import (
	"bytes"
	"strings"
	"testing"
)

// TestHTMLRenderer_RenderIndex tests the index page rendering.
// Follows AAA (Arrange, Act, Assert) pattern.
func TestHTMLRenderer_RenderIndex(t *testing.T) {
	// Arrange
	renderer := NewHTMLRenderer()
	buf := &bytes.Buffer{}

	// Act
	err := renderer.RenderIndex(buf)

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "CI Dashboard") {
		t.Errorf("expected output to contain 'CI Dashboard', got %q", output)
	}
	if !strings.Contains(output, "<!DOCTYPE html>") {
		t.Errorf("expected output to contain '<!DOCTYPE html>', got %q", output)
	}
}

// TestHTMLRenderer_RenderHealth tests the health check rendering.
// Follows AAA (Arrange, Act, Assert) pattern.
func TestHTMLRenderer_RenderHealth(t *testing.T) {
	// Arrange
	renderer := NewHTMLRenderer()
	buf := &bytes.Buffer{}

	// Act
	err := renderer.RenderHealth(buf)

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	expected := `{"status":"ok"}`
	if buf.String() != expected {
		t.Errorf("expected %q, got %q", expected, buf.String())
	}
}
