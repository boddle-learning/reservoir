package token

import (
	"testing"
	"time"
)

func newTestService(accessTTL time.Duration) *Service {
	return NewService(
		"test-secret-key-minimum-32-chars",
		"test-refresh-secret-key-32-chars",
		accessTTL,
		720*time.Hour,
	)
}

// TestGenerate_EmbedsTokenVersion verifies the token_version is carried in both
// the access and refresh tokens, so logout (which bumps the user's version) can
// invalidate them. See Finding 2 / LMS-6513.
func TestGenerate_EmbedsTokenVersion(t *testing.T) {
	svc := newTestService(6 * time.Hour)

	pair, err := svc.Generate(1, "uid", "a@b.com", "A B", "Teacher", 10, 7)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	access, err := svc.Validate(pair.AccessToken)
	if err != nil {
		t.Fatalf("Validate access: %v", err)
	}
	if access.TokenVersion != 7 {
		t.Errorf("access TokenVersion = %d, want 7", access.TokenVersion)
	}

	refresh, err := svc.ValidateRefreshToken(pair.RefreshToken)
	if err != nil {
		t.Fatalf("ValidateRefreshToken: %v", err)
	}
	if refresh.TokenVersion != 7 {
		t.Errorf("refresh TokenVersion = %d, want 7", refresh.TokenVersion)
	}
}

// TestValidateAllowExpired_AcceptsExpired confirms logout can still recover the
// claims (and thus the user ID) from an access token that has already expired,
// while strict Validate rejects it.
func TestValidateAllowExpired_AcceptsExpired(t *testing.T) {
	svc := newTestService(-1 * time.Hour) // mint an already-expired access token

	pair, err := svc.Generate(42, "uid", "a@b.com", "A B", "Teacher", 10, 3)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if _, err := svc.Validate(pair.AccessToken); err == nil {
		t.Error("strict Validate should reject an expired token")
	}

	claims, err := svc.ValidateAllowExpired(pair.AccessToken)
	if err != nil {
		t.Fatalf("ValidateAllowExpired should accept an expired token: %v", err)
	}
	if claims.UserID != 42 {
		t.Errorf("UserID = %d, want 42", claims.UserID)
	}
	if claims.TokenVersion != 3 {
		t.Errorf("TokenVersion = %d, want 3", claims.TokenVersion)
	}
}

// TestValidateAllowExpired_RejectsBadSignature ensures the signature is still
// enforced — an attacker can't forge a token to force logout of another user.
func TestValidateAllowExpired_RejectsBadSignature(t *testing.T) {
	signer := newTestService(6 * time.Hour)
	pair, err := signer.Generate(1, "uid", "a@b.com", "A B", "Teacher", 10, 1)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	other := NewService(
		"different-secret-key-minimum-32ch",
		"different-refresh-secret-32-chars",
		6*time.Hour,
		720*time.Hour,
	)
	if _, err := other.ValidateAllowExpired(pair.AccessToken); err == nil {
		t.Error("ValidateAllowExpired should reject a token signed with another key")
	}
}
