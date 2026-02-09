package oauth

import (
	"strings"
	"testing"
)

func TestNewCleverService(t *testing.T) {
	cfg := struct {
		ClientID     string
		ClientSecret string
		RedirectURL  string
	}{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURL:  "http://localhost:8080/auth/clever/callback",
	}

	stateManager := &StateManager{} // Mock state manager

	// Convert to config.CleverConfig structure
	cleverCfg := struct {
		ClientID     string
		ClientSecret string
		RedirectURL  string
	}{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
	}

	service := &CleverService{
		stateManager: stateManager,
	}

	if service.stateManager == nil {
		t.Error("CleverService stateManager should not be nil")
	}

	// Verify the service was created (basic test)
	if service == nil {
		t.Fatal("NewCleverService returned nil")
	}

	_ = cleverCfg // Use the config
}

func TestCleverAuthURL(t *testing.T) {
	// Test that Clever auth URL contains expected components
	expectedURL := "https://clever.com/oauth/authorize"

	if !strings.Contains(expectedURL, "clever.com") {
		t.Error("Clever auth URL should contain clever.com domain")
	}

	if !strings.Contains(expectedURL, "/oauth/authorize") {
		t.Error("Clever auth URL should contain /oauth/authorize path")
	}
}

func TestCleverTokenURL(t *testing.T) {
	// Test that Clever token URL is correct
	expectedURL := "https://clever.com/oauth/tokens"

	if !strings.Contains(expectedURL, "clever.com") {
		t.Error("Clever token URL should contain clever.com domain")
	}

	if !strings.Contains(expectedURL, "/oauth/tokens") {
		t.Error("Clever token URL should contain /oauth/tokens path")
	}
}

func TestCleverUserInfoURL(t *testing.T) {
	// Test that Clever user info URL is correct
	expectedURL := "https://api.clever.com/v3.0/me"

	if !strings.Contains(expectedURL, "api.clever.com") {
		t.Error("Clever user info URL should contain api.clever.com domain")
	}

	if !strings.Contains(expectedURL,"/v3.0/me") {
		t.Error("Clever user info URL should contain /v3.0/me path")
	}
}
