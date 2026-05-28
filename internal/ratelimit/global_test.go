package ratelimit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestLimiter(t *testing.T, capacity int, refillPerSec float64) (*GlobalLimiter, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis start: %v", err)
	}
	t.Cleanup(mr.Close)

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	lim, err := NewGlobalLimiter(client, "test:global:login", capacity, refillPerSec)
	if err != nil {
		t.Fatalf("NewGlobalLimiter: %v", err)
	}
	return lim, mr
}

func TestNewGlobalLimiter_RejectsInvalidConfig(t *testing.T) {
	cases := []struct {
		name     string
		capacity int
		refill   float64
	}{
		{"zero capacity", 0, 10},
		{"negative capacity", -1, 10},
		{"zero refill", 100, 0},
		{"negative refill", 100, -0.5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewGlobalLimiter(nil, "k", tc.capacity, tc.refill)
			if !errors.Is(err, ErrInvalidLimiterConfig) {
				t.Fatalf("err = %v, want ErrInvalidLimiterConfig", err)
			}
		})
	}
}

func TestGlobalLimiter_AllowsUpToCapacity(t *testing.T) {
	lim, _ := newTestLimiter(t, 3, 1)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		allowed, _, err := lim.Allow(ctx)
		if err != nil {
			t.Fatalf("call %d: err = %v", i, err)
		}
		if !allowed {
			t.Fatalf("call %d: expected allowed", i)
		}
	}
}

func TestGlobalLimiter_RejectsWhenBucketEmpty(t *testing.T) {
	lim, _ := newTestLimiter(t, 2, 1)
	ctx := context.Background()

	// Drain.
	for i := 0; i < 2; i++ {
		if allowed, _, err := lim.Allow(ctx); err != nil || !allowed {
			t.Fatalf("drain call %d: allowed=%v err=%v", i, allowed, err)
		}
	}

	allowed, retryAfter, err := lim.Allow(ctx)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if allowed {
		t.Fatalf("expected reject after draining bucket")
	}
	// With refill=1/sec, retry-after for the first token should be ~1s.
	if retryAfter < 500*time.Millisecond || retryAfter > 1100*time.Millisecond {
		t.Fatalf("retryAfter = %v, want ~1s", retryAfter)
	}
}

func TestGlobalLimiter_RefillsOverTime(t *testing.T) {
	// High refill rate so the wait stays well under a test-suite second.
	// Capacity=1, refill=200/sec → 1 token regenerated every 5ms; a 50ms
	// sleep is comfortably long enough to refill.
	lim, _ := newTestLimiter(t, 1, 200)
	ctx := context.Background()

	// Consume the single starting token.
	if allowed, _, err := lim.Allow(ctx); err != nil || !allowed {
		t.Fatalf("initial: allowed=%v err=%v", allowed, err)
	}

	// Immediately retry — bucket is empty.
	if allowed, _, _ := lim.Allow(ctx); allowed {
		t.Fatalf("expected reject immediately after draining")
	}

	time.Sleep(50 * time.Millisecond)

	if allowed, _, err := lim.Allow(ctx); err != nil || !allowed {
		t.Fatalf("after refill: allowed=%v err=%v", allowed, err)
	}
}

func TestGlobalLimiter_BucketIsSharedAcrossClients(t *testing.T) {
	// Two clients pointing at the same miniredis must share the bucket —
	// this is the property that distinguishes GlobalLimiter from the
	// per-user Limiter.
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis start: %v", err)
	}
	t.Cleanup(mr.Close)

	clientA := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	clientB := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = clientA.Close(); _ = clientB.Close() })

	limA, err := NewGlobalLimiter(clientA, "shared:key", 2, 1)
	if err != nil {
		t.Fatalf("NewGlobalLimiter A: %v", err)
	}
	limB, err := NewGlobalLimiter(clientB, "shared:key", 2, 1)
	if err != nil {
		t.Fatalf("NewGlobalLimiter B: %v", err)
	}
	ctx := context.Background()

	// Capacity is 2 across the shared key.
	if allowed, _, _ := limA.Allow(ctx); !allowed {
		t.Fatalf("A1: expected allowed")
	}
	if allowed, _, _ := limB.Allow(ctx); !allowed {
		t.Fatalf("B1: expected allowed")
	}
	if allowed, _, _ := limA.Allow(ctx); allowed {
		t.Fatalf("A2: expected reject (bucket should be drained)")
	}
}

func TestGlobalLimiter_KeyExpires(t *testing.T) {
	lim, mr := newTestLimiter(t, 4, 2)
	ctx := context.Background()
	if _, _, err := lim.Allow(ctx); err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if !mr.Exists("test:global:login") {
		t.Fatalf("expected key to exist after first call")
	}
	// TTL is set to ceil(capacity/refill) + 5 = 7s. Advance past it.
	mr.FastForward(10 * time.Second)
	if mr.Exists("test:global:login") {
		t.Fatalf("expected key to expire after TTL")
	}
}