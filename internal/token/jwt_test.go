package token

import (
	"testing"
	"time"
)

func TestService_Generate(t *testing.T) {
	service := NewService(
		"test-secret-key-minimum-32-chars",
		"test-refresh-secret-key-32-chars",
		6*time.Hour,
		720*time.Hour,
	)

	tokenPair, err := service.Generate(
		1,
		"boddle-uid-123",
		"test@example.com",
		"Test User",
		"Teacher",
		10,
	)

	if err != nil {
		t.Fatalf("Generate() failed: %v", err)
	}

	if tokenPair == nil {
		t.Fatal("Generate() returned nil token pair")
	}

	if tokenPair.AccessToken == "" {
		t.Error("AccessToken is empty")
	}

	if tokenPair.RefreshToken == "" {
		t.Error("RefreshToken is empty")
	}

	if tokenPair.TokenType != TokenTypeBearer {
		t.Errorf("TokenType = %q, want %q", tokenPair.TokenType, TokenTypeBearer)
	}

	if tokenPair.ExpiresAt.IsZero() {
		t.Error("ExpiresAt is zero")
	}

	// ExpiresAt should be approximately 6 hours from now
	expectedExpiry := time.Now().Add(6 * time.Hour)
	diff := tokenPair.ExpiresAt.Sub(expectedExpiry).Abs()
	if diff > time.Minute {
		t.Errorf("ExpiresAt = %v, expected around %v (diff: %v)", tokenPair.ExpiresAt, expectedExpiry, diff)
	}
}

func TestService_Validate(t *testing.T) {
	service := NewService(
		"test-secret-key-minimum-32-chars",
		"test-refresh-secret-key-32-chars",
		6*time.Hour,
		720*time.Hour,
	)

	// Generate a token
	tokenPair, err := service.Generate(
		1,
		"boddle-uid-123",
		"test@example.com",
		"Test User",
		"Teacher",
		10,
	)
	if err != nil {
		t.Fatalf("Generate() failed: %v", err)
	}

	// Validate the token
	claims, err := service.Validate(tokenPair.AccessToken)
	if err != nil {
		t.Fatalf("Validate() failed: %v", err)
	}

	// Check claims
	if claims.UserID != 1 {
		t.Errorf("UserID = %d, want 1", claims.UserID)
	}

	if claims.BoddleUID != "boddle-uid-123" {
		t.Errorf("BoddleUID = %q, want %q", claims.BoddleUID, "boddle-uid-123")
	}

	if claims.Email != "test@example.com" {
		t.Errorf("Email = %q, want %q", claims.Email, "test@example.com")
	}

	if claims.Name != "Test User" {
		t.Errorf("Name = %q, want %q", claims.Name, "Test User")
	}

	if claims.MetaType != "Teacher" {
		t.Errorf("MetaType = %q, want %q", claims.MetaType, "Teacher")
	}

	if claims.MetaID != 10 {
		t.Errorf("MetaID = %d, want 10", claims.MetaID)
	}

	if claims.ID == "" {
		t.Error("JTI (ID) is empty")
	}
}

func TestService_ValidateInvalidToken(t *testing.T) {
	service := NewService(
		"test-secret-key-minimum-32-chars",
		"test-refresh-secret-key-32-chars",
		6*time.Hour,
		720*time.Hour,
	)

	tests := []struct {
		name  string
		token string
	}{
		{"empty token", ""},
		{"invalid format", "not.a.jwt"},
		{"random string", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.invalid.signature"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.Validate(tt.token)
			if err == nil {
				t.Error("Validate() should fail for invalid token")
			}
		})
	}
}

func TestService_ValidateWrongSecret(t *testing.T) {
	// Generate token with one secret
	service1 := NewService(
		"secret-key-1-minimum-32-characters",
		"refresh-secret-1-minimum-32-chars",
		6*time.Hour,
		720*time.Hour,
	)

	tokenPair, err := service1.Generate(
		1,
		"boddle-uid-123",
		"test@example.com",
		"Test User",
		"Teacher",
		10,
	)
	if err != nil {
		t.Fatalf("Generate() failed: %v", err)
	}

	// Try to validate with different secret
	service2 := NewService(
		"secret-key-2-minimum-32-characters",
		"refresh-secret-2-minimum-32-chars",
		6*time.Hour,
		720*time.Hour,
	)

	_, err = service2.Validate(tokenPair.AccessToken)
	if err == nil {
		t.Error("Validate() should fail when secret key doesn't match")
	}
}

func TestService_ExtractTokenID(t *testing.T) {
	service := NewService(
		"test-secret-key-minimum-32-chars",
		"test-refresh-secret-key-32-chars",
		6*time.Hour,
		720*time.Hour,
	)

	// Generate a token
	tokenPair, err := service.Generate(
		1,
		"boddle-uid-123",
		"test@example.com",
		"Test User",
		"Teacher",
		10,
	)
	if err != nil {
		t.Fatalf("Generate() failed: %v", err)
	}

	// Extract token ID
	tokenID, err := service.ExtractTokenID(tokenPair.AccessToken)
	if err != nil {
		t.Fatalf("ExtractTokenID() failed: %v", err)
	}

	if tokenID == "" {
		t.Error("ExtractTokenID() returned empty token ID")
	}

	// Validate the token to get claims and compare IDs
	claims, err := service.Validate(tokenPair.AccessToken)
	if err != nil {
		t.Fatalf("Validate() failed: %v", err)
	}

	if tokenID != claims.ID {
		t.Errorf("ExtractTokenID() = %q, but Validate() claims.ID = %q", tokenID, claims.ID)
	}
}
