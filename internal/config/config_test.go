package config

import (
	"os"
	"testing"
)

// TestLoad_DefaultPort tests loading config with default port.
// Follows AAA (Arrange, Act, Assert) pattern.
func TestLoad_DefaultPort(t *testing.T) {
	// Arrange
	os.Unsetenv("PORT")

	// Act
	cfg, err := Load()

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Port)
	}
}

// TestLoad_CustomPort tests loading config with custom port from environment.
func TestLoad_CustomPort(t *testing.T) {
	// Arrange
	os.Setenv("PORT", "3000")
	defer os.Unsetenv("PORT")

	// Act
	cfg, err := Load()

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.Port != 3000 {
		t.Errorf("expected port 3000, got %d", cfg.Port)
	}
}

// TestLoad_InvalidPort tests that invalid port falls back to default.
func TestLoad_InvalidPort(t *testing.T) {
	// Arrange
	os.Setenv("PORT", "invalid")
	defer os.Unsetenv("PORT")

	// Act
	cfg, err := Load()

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if cfg.Port != 8080 {
		t.Errorf("expected default port 8080 for invalid input, got %d", cfg.Port)
	}
}
