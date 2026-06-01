package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newTestContext(method, target, body string, headers map[string]string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, target, bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, target, nil)
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	c.Request = r
	return c, w
}

func TestExtractLoginTokenSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name    string
		target  string
		body    string
		headers map[string]string
		want    string
	}{
		{
			name:    "from Authorization Bearer header",
			target:  "/auth/token",
			headers: map[string]string{"Authorization": "Bearer secret-abc"},
			want:    "secret-abc",
		},
		{
			name:   "from JSON body",
			target: "/auth/token",
			body:   `{"token":"secret-body"}`,
			want:   "secret-body",
		},
		{
			name:    "header takes precedence over body",
			target:  "/auth/token",
			body:    `{"token":"from-body"}`,
			headers: map[string]string{"Authorization": "Bearer from-header"},
			want:    "from-header",
		},
		{
			name:   "no credential present",
			target: "/auth/token",
			want:   "",
		},
		{
			// The secret must NOT be read from the query string — that is the
			// vulnerability being fixed (Finding 3 / LMS-6514).
			name:   "query string token is ignored",
			target: "/auth/token?token=should-be-ignored",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newTestContext(http.MethodPost, tt.target, tt.body, tt.headers)
			if got := extractLoginTokenSecret(c); got != tt.want {
				t.Errorf("extractLoginTokenSecret() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoginWithToken_MissingCredential(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// service is nil: a missing credential is rejected before any service call.
	handler := &Handler{service: nil}

	c, w := newTestContext(http.MethodPost, "/auth/token?token=ignored", "", nil)
	handler.LoginWithToken(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["success"] != false {
		t.Error("expected success=false for missing credential")
	}
}
