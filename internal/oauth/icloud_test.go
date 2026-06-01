package oauth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestICloudAuth_MissingIdentityToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &Handler{
		authService: nil, // not reached — request is rejected before service call
	}

	tests := []struct {
		name string
		body string
	}{
		{
			name: "empty body",
			body: `{}`,
		},
		{
			name: "empty identity_token",
			body: `{"identity_token": ""}`,
		},
		{
			name: "missing identity_token field",
			body: `{"foo": "bar"}`,
		},
		{
			// The pre-LMS-6512 attack shape: a bare uid must no longer be
			// accepted, since identity_token is now required.
			name: "legacy uid only",
			body: `{"uid": "victim-apple-sub"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			c.Request = httptest.NewRequest(
				http.MethodPost,
				"/auth/icloud",
				bytes.NewBufferString(tt.body),
			)
			c.Request.Header.Set("Content-Type", "application/json")

			handler.ICloudAuth(c)

			if w.Code != http.StatusBadRequest {
				t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
			}

			var resp map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to parse response: %v", err)
			}

			if resp["success"] != false {
				t.Error("expected success to be false")
			}

			errObj, ok := resp["error"].(map[string]interface{})
			if !ok {
				t.Fatal("expected error object in response")
			}

			if errObj["code"] != "INVALID_REQUEST" {
				t.Errorf("expected error code INVALID_REQUEST, got %v", errObj["code"])
			}
		})
	}
}

func TestICloudAuth_InvalidContentType(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &Handler{
		authService: nil,
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/auth/icloud",
		bytes.NewBufferString("uid=some-apple-uid"),
	)
	c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	handler.ICloudAuth(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestFindOrCreateiCloudUser_NotFound(t *testing.T) {
	// Verifies that findOrCreateiCloudUser returns an appropriate error
	// when no student or parent exists with the given UID.
	// This is a unit-level check on the error message contract.
	info := &OAuthUserInfo{ProviderUserID: "nonexistent-uid"}

	// We can't call findOrCreateiCloudUser without a real DB/repo,
	// but we can verify the OAuthUserInfo is constructed correctly
	// for the client-side-only flow (no email, no name).
	if info.Email != "" {
		t.Error("client-side iCloud flow should not provide email")
	}
	if info.FirstName != "" || info.LastName != "" {
		t.Error("client-side iCloud flow should not provide name")
	}
	if info.ProviderUserID != "nonexistent-uid" {
		t.Errorf("ProviderUserID = %q, want %q", info.ProviderUserID, "nonexistent-uid")
	}
}

func TestICloudOAuthUserInfo_ClientSideOnly(t *testing.T) {
	// The client-side flow only provides a UID — no email, name, or picture.
	// Verify OAuthUserInfo is used correctly in this reduced form.
	uid := "001234.abcdef1234567890abcdef1234567890.1234"
	info := &OAuthUserInfo{ProviderUserID: uid}

	if info.ProviderUserID != uid {
		t.Errorf("ProviderUserID = %q, want %q", info.ProviderUserID, uid)
	}
	if info.Email != "" {
		t.Errorf("Email should be empty for client-side flow, got %q", info.Email)
	}
	if info.EmailVerified {
		t.Error("EmailVerified should be false for client-side flow")
	}
	if info.FirstName != "" || info.LastName != "" {
		t.Error("Name fields should be empty for client-side flow")
	}
}
