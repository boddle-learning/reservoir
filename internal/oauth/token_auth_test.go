package oauth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestGoogleFetchUserInfo_UsesProviderIdentity verifies that the access token
// is presented to Google's userinfo endpoint as a bearer token and that the
// returned identity comes entirely from Google's response — the foundation of
// the LMS-6511 fix, where caller-supplied uid/email must never be trusted.
func TestGoogleFetchUserInfo_UsesProviderIdentity(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":             "google-sub-123",
			"email":          "real@school.edu",
			"verified_email": true,
			"given_name":     "Real",
			"family_name":    "Teacher",
		})
	}))
	defer srv.Close()

	gs := &GoogleService{userInfoURL: srv.URL, httpClient: srv.Client()}

	info, err := gs.fetchUserInfo(context.Background(), "valid-access-token")
	if err != nil {
		t.Fatalf("fetchUserInfo returned error: %v", err)
	}

	if gotAuth != "Bearer valid-access-token" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer valid-access-token")
	}
	if info.ProviderUserID != "google-sub-123" {
		t.Errorf("ProviderUserID = %q, want from provider %q", info.ProviderUserID, "google-sub-123")
	}
	if info.Email != "real@school.edu" {
		t.Errorf("Email = %q, want from provider %q", info.Email, "real@school.edu")
	}
	if !info.EmailVerified {
		t.Error("EmailVerified should reflect provider's verified_email=true")
	}
}

// TestGoogleFetchUserInfo_RejectsInvalidToken verifies that an unauthorized
// response (the case where a forged/expired token is presented) surfaces as an
// error rather than yielding a usable identity.
func TestGoogleFetchUserInfo_RejectsInvalidToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_token"}`))
	}))
	defer srv.Close()

	gs := &GoogleService{userInfoURL: srv.URL, httpClient: srv.Client()}

	if _, err := gs.fetchUserInfo(context.Background(), "forged-token"); err == nil {
		t.Fatal("expected error for unauthorized token, got nil")
	}
}

// TestCleverFetchUserInfo_UsesProviderIdentity is the Clever analogue of the
// Google test: identity is taken from Clever's /me response.
func TestCleverFetchUserInfo_UsesProviderIdentity(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"id":    "clever-id-456",
				"type":  "teacher",
				"email": "clever-real@school.edu",
				"name":  map[string]string{"first": "Clever", "last": "Teacher"},
			},
		})
	}))
	defer srv.Close()

	cs := &CleverService{userInfoURL: srv.URL, httpClient: srv.Client()}

	info, err := cs.fetchUserInfo(context.Background(), "valid-access-token")
	if err != nil {
		t.Fatalf("fetchUserInfo returned error: %v", err)
	}

	if gotAuth != "Bearer valid-access-token" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer valid-access-token")
	}
	if info.ProviderUserID != "clever-id-456" {
		t.Errorf("ProviderUserID = %q, want from provider %q", info.ProviderUserID, "clever-id-456")
	}
	if info.Email != "clever-real@school.edu" {
		t.Errorf("Email = %q, want from provider %q", info.Email, "clever-real@school.edu")
	}
}

func TestCleverFetchUserInfo_RejectsInvalidToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_token"}`))
	}))
	defer srv.Close()

	cs := &CleverService{userInfoURL: srv.URL, httpClient: srv.Client()}

	if _, err := cs.fetchUserInfo(context.Background(), "forged-token"); err == nil {
		t.Fatal("expected error for unauthorized token, got nil")
	}
}

// TestTokenAuth_RequiresToken verifies the handlers reject requests with no
// access token before reaching the service. Previously these endpoints
// required uid/email and would accept attacker-supplied identity; now the
// token is the only required field.
func TestTokenAuth_RequiresToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// authService is nil because a missing token must be rejected during
	// binding, before any service call.
	handler := &Handler{authService: nil}

	endpoints := []struct {
		name string
		fn   func(*gin.Context)
		path string
	}{
		{"google", handler.GoogleTokenAuth, "/auth/google"},
		{"clever", handler.CleverTokenAuth, "/auth/clever"},
	}

	bodies := []struct {
		name string
		body string
	}{
		{"empty body", `{}`},
		{"empty token", `{"token": ""}`},
		// uid/email present but no token — the old attack shape — must still 400.
		{"identity without token", `{"uid":"x","email":"victim@school.edu","name":"x"}`},
	}

	for _, ep := range endpoints {
		for _, b := range bodies {
			t.Run(ep.name+"/"+b.name, func(t *testing.T) {
				w := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(w)
				c.Request = httptest.NewRequest(http.MethodPost, ep.path, bytes.NewBufferString(b.body))
				c.Request.Header.Set("Content-Type", "application/json")

				ep.fn(c)

				if w.Code != http.StatusBadRequest {
					t.Fatalf("expected status %d, got %d", http.StatusBadRequest, w.Code)
				}

				var resp map[string]interface{}
				if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
					t.Fatalf("failed to parse response: %v", err)
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
}
