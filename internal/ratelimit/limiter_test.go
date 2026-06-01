package ratelimit

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

func testLimiter() *Limiter {
	return NewLimiter(nil, 10*time.Minute, 5, 15*time.Minute, time.Hour, 20, zap.NewNop())
}

// TestDimensions verifies the two rate-limit axes: a tight per-(ip,email)
// bucket and a wider per-email backstop.
func TestDimensions(t *testing.T) {
	l := testLimiter()
	dims := l.dimensions("a@b.com", "1.2.3.4")

	if len(dims) != 2 {
		t.Fatalf("expected 2 dimensions, got %d", len(dims))
	}

	ip := dims[0]
	if ip.attemptKey != "ratelimit:login:1.2.3.4:a@b.com" {
		t.Errorf("ip attemptKey = %q", ip.attemptKey)
	}
	if ip.lockoutKey != "ratelimit:lockout:1.2.3.4:a@b.com" {
		t.Errorf("ip lockoutKey = %q", ip.lockoutKey)
	}
	if ip.window != 10*time.Minute || ip.max != 5 {
		t.Errorf("ip window/max = %v/%d, want 10m/5", ip.window, ip.max)
	}

	email := dims[1]
	if email.attemptKey != "ratelimit:login:email:a@b.com" {
		t.Errorf("email attemptKey = %q", email.attemptKey)
	}
	if email.lockoutKey != "ratelimit:lockout:email:a@b.com" {
		t.Errorf("email lockoutKey = %q", email.lockoutKey)
	}
	if email.window != time.Hour || email.max != 20 {
		t.Errorf("email window/max = %v/%d, want 1h/20", email.window, email.max)
	}
}

// TestEmailDimensionIgnoresIP is the crux of the Finding 4 fix: the email-only
// backstop key does NOT include the IP, so an attacker rotating
// X-Forwarded-For values still lands in the same bucket for a given account.
func TestEmailDimensionIgnoresIP(t *testing.T) {
	l := testLimiter()

	a := l.dimensions("victim@school.edu", "1.1.1.1")[1]
	b := l.dimensions("victim@school.edu", "9.9.9.9")[1]

	if a.attemptKey != b.attemptKey {
		t.Errorf("email attempt key changed with IP: %q vs %q", a.attemptKey, b.attemptKey)
	}
	if a.lockoutKey != b.lockoutKey {
		t.Errorf("email lockout key changed with IP: %q vs %q", a.lockoutKey, b.lockoutKey)
	}

	// And the per-(ip,email) bucket DOES vary by IP (so legit distinct clients
	// aren't lumped together when proxies are trusted).
	ipA := l.dimensions("victim@school.edu", "1.1.1.1")[0]
	ipB := l.dimensions("victim@school.edu", "9.9.9.9")[0]
	if ipA.attemptKey == ipB.attemptKey {
		t.Error("per-(ip,email) key should differ when IP differs")
	}
}
