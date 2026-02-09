package oauth

import (
	"testing"
)

func TestStateManager_GenerateState(t *testing.T) {
	// Note: This test requires Redis to be running
	// In a real scenario, we'd mock Redis for unit tests

	sm := &StateManager{
		client: nil, // Would use mock client
	}

	state, err := sm.GenerateState()
	if err != nil {
		t.Fatalf("GenerateState() failed: %v", err)
	}

	if state == "" {
		t.Error("GenerateState() returned empty state")
	}

	if len(state) != 64 { // 32 bytes = 64 hex characters
		t.Errorf("GenerateState() length = %d, want 64", len(state))
	}

	// Generate another state to verify uniqueness
	state2, err := sm.GenerateState()
	if err != nil {
		t.Fatalf("GenerateState() second call failed: %v", err)
	}

	if state == state2 {
		t.Error("GenerateState() should generate unique states")
	}
}

func TestOAuthUserInfo(t *testing.T) {
	info := &OAuthUserInfo{
		ProviderUserID: "google-123",
		Email:          "test@example.com",
		FirstName:      "John",
		LastName:       "Doe",
		EmailVerified:  true,
	}

	if info.ProviderUserID != "google-123" {
		t.Errorf("ProviderUserID = %q, want %q", info.ProviderUserID, "google-123")
	}

	if info.Email != "test@example.com" {
		t.Errorf("Email = %q, want %q", info.Email, "test@example.com")
	}

	if !info.EmailVerified {
		t.Error("EmailVerified = false, want true")
	}
}
