package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

type stubLimiter struct {
	allow      bool
	retryAfter time.Duration
	err        error
	calls      int
}

func (s *stubLimiter) Allow(ctx context.Context) (bool, time.Duration, error) {
	s.calls++
	return s.allow, s.retryAfter, s.err
}

func newTestRouter(mw gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/auth/login", mw, func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	return r
}

func TestLoginQueue_NilLimiterPassesThrough(t *testing.T) {
	r := newTestRouter(LoginQueue(nil))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Fatalf("body = %q, want %q", w.Body.String(), "ok")
	}
}

func TestLoginQueue_AllowedPassesThrough(t *testing.T) {
	s := &stubLimiter{allow: true}
	r := newTestRouter(LoginQueue(s))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if s.calls != 1 {
		t.Fatalf("limiter calls = %d, want 1", s.calls)
	}
}

func TestLoginQueue_RejectedReturns429WithRetryAfter(t *testing.T) {
	s := &stubLimiter{allow: false, retryAfter: 2500 * time.Millisecond}
	r := newTestRouter(LoginQueue(s))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", w.Code)
	}
	// 2500ms should ceil to 3 seconds.
	if got := w.Header().Get("Retry-After"); got != "3" {
		t.Fatalf("Retry-After = %q, want %q", got, "3")
	}
	body := w.Body.String()
	for _, want := range []string{`"success":false`, `"code":"LOGIN_THROTTLED"`, `"retry_after":3`} {
		if !contains(body, want) {
			t.Fatalf("body missing %q; got %s", want, body)
		}
	}
}

func TestLoginQueue_SubSecondRetryFloorsToOne(t *testing.T) {
	s := &stubLimiter{allow: false, retryAfter: 50 * time.Millisecond}
	r := newTestRouter(LoginQueue(s))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", w.Code)
	}
	if got := w.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want %q (minimum floor)", got, "1")
	}
}

func TestLoginQueue_LimiterErrorFailsOpen(t *testing.T) {
	s := &stubLimiter{err: errors.New("redis down")}
	r := newTestRouter(LoginQueue(s))
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (fail-open)", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Fatalf("body = %q, want %q", w.Body.String(), "ok")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}