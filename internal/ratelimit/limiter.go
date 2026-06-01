package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// Limiter handles rate limiting using Redis
type Limiter struct {
	client          *redis.Client
	window          time.Duration // Time window for counting per-(ip,email) attempts
	maxAttempts     int           // Maximum per-(ip,email) attempts allowed in window
	lockoutDuration time.Duration // How long to block after exceeding a limit

	// Email-only backstop. A wider window and higher threshold catch an
	// attacker who rotates source IPs to brute-force one account, which the
	// per-(ip,email) bucket alone cannot (Finding 4 / LMS-6515).
	emailWindow      time.Duration
	emailMaxAttempts int

	logger *zap.Logger
}

// NewLimiter creates a new rate limiter
func NewLimiter(client *redis.Client, window time.Duration, maxAttempts int, lockoutDuration time.Duration, emailWindow time.Duration, emailMaxAttempts int, logger *zap.Logger) *Limiter {
	return &Limiter{
		client:           client,
		window:           window,
		maxAttempts:      maxAttempts,
		lockoutDuration:  lockoutDuration,
		emailWindow:      emailWindow,
		emailMaxAttempts: emailMaxAttempts,
		logger:           logger,
	}
}

// dimension is one independently-counted rate-limit axis.
type dimension struct {
	attemptKey string
	lockoutKey string
	window     time.Duration
	max        int
}

// dimensions returns the axes evaluated for a login: a tight per-(ip,email)
// bucket and a wider per-email backstop.
func (l *Limiter) dimensions(email, ipAddress string) []dimension {
	return []dimension{
		{
			attemptKey: fmt.Sprintf("ratelimit:login:%s:%s", ipAddress, email),
			lockoutKey: fmt.Sprintf("ratelimit:lockout:%s:%s", ipAddress, email),
			window:     l.window,
			max:        l.maxAttempts,
		},
		{
			attemptKey: fmt.Sprintf("ratelimit:login:email:%s", email),
			lockoutKey: fmt.Sprintf("ratelimit:lockout:email:%s", email),
			window:     l.emailWindow,
			max:        l.emailMaxAttempts,
		},
	}
}

// LoginAttemptKey returns the Redis key for tracking per-(ip,email) attempts.
func (l *Limiter) LoginAttemptKey(email, ipAddress string) string {
	return fmt.Sprintf("ratelimit:login:%s:%s", ipAddress, email)
}

// LoginLockoutKey returns the Redis key for per-(ip,email) lockout status.
func (l *Limiter) LoginLockoutKey(email, ipAddress string) string {
	return fmt.Sprintf("ratelimit:lockout:%s:%s", ipAddress, email)
}

// CheckLoginAttempt checks if a login attempt is allowed. A request is blocked
// if EITHER dimension is locked out or over its limit.
// Returns: allowed (bool), remainingAttempts (int), lockoutRemaining (time.Duration), error
func (l *Limiter) CheckLoginAttempt(ctx context.Context, email, ipAddress string) (bool, int, time.Duration, error) {
	dims := l.dimensions(email, ipAddress)

	// First pass: honor any active lockout. Continued attempts while locked out
	// slide the lockout window forward, so an attacker who keeps hammering can't
	// simply wait out the cooldown and resume (Finding 4 / LMS-6515).
	for _, d := range dims {
		ttl, err := l.client.TTL(ctx, d.lockoutKey).Result()
		if err != nil && err != redis.Nil {
			return false, 0, 0, fmt.Errorf("failed to check lockout status: %w", err)
		}
		if ttl > 0 {
			if err := l.client.Set(ctx, d.lockoutKey, "1", l.lockoutDuration).Err(); err != nil {
				l.logger.Warn("failed to extend lockout window", zap.Error(err))
			}
			return false, 0, l.lockoutDuration, nil
		}
	}

	// Second pass: evaluate attempt counters. If any dimension is at its limit,
	// open a lockout for that dimension.
	minRemaining := -1
	for _, d := range dims {
		count, err := l.client.Get(ctx, d.attemptKey).Int()
		if err != nil && err != redis.Nil {
			return false, 0, 0, fmt.Errorf("failed to get attempt count: %w", err)
		}

		remaining := d.max - count
		if remaining <= 0 {
			if err := l.client.Set(ctx, d.lockoutKey, "1", l.lockoutDuration).Err(); err != nil {
				return false, 0, 0, fmt.Errorf("failed to set lockout: %w", err)
			}
			if err := l.client.Del(ctx, d.attemptKey).Err(); err != nil {
				l.logger.Warn("failed to clear attempt counter", zap.Error(err))
			}
			return false, 0, l.lockoutDuration, nil
		}
		if minRemaining < 0 || remaining < minRemaining {
			minRemaining = remaining
		}
	}

	return true, minRemaining, 0, nil
}

// RecordFailedAttempt records a failed login attempt on every dimension.
func (l *Limiter) RecordFailedAttempt(ctx context.Context, email, ipAddress string) error {
	for _, d := range l.dimensions(email, ipAddress) {
		count, err := l.client.Incr(ctx, d.attemptKey).Result()
		if err != nil {
			return fmt.Errorf("failed to increment attempt counter: %w", err)
		}
		// Set expiry on first attempt so the counter window starts now.
		if count == 1 {
			if err := l.client.Expire(ctx, d.attemptKey, d.window).Err(); err != nil {
				return fmt.Errorf("failed to set expiry: %w", err)
			}
		}
	}
	return nil
}

// RecordSuccessfulAttempt clears the per-(ip,email) attempt counter after a
// successful login. The email backstop counter is intentionally left to expire
// on its own so one success doesn't reset an in-progress distributed attack
// against the same account; it ages out within emailWindow.
func (l *Limiter) RecordSuccessfulAttempt(ctx context.Context, email, ipAddress string) error {
	if err := l.client.Del(ctx, l.LoginAttemptKey(email, ipAddress)).Err(); err != nil {
		return fmt.Errorf("failed to clear attempt counter: %w", err)
	}
	return nil
}

// ClearLockout manually clears all lockouts and counters for the (email, ip)
// pair, including the email-only backstop (admin function).
func (l *Limiter) ClearLockout(ctx context.Context, email, ipAddress string) error {
	keys := make([]string, 0, 4)
	for _, d := range l.dimensions(email, ipAddress) {
		keys = append(keys, d.lockoutKey, d.attemptKey)
	}
	if err := l.client.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("failed to clear lockout: %w", err)
	}
	return nil
}

// GetAttemptCount returns the current per-(ip,email) attempt count
func (l *Limiter) GetAttemptCount(ctx context.Context, email, ipAddress string) (int, error) {
	attemptKey := l.LoginAttemptKey(email, ipAddress)

	count, err := l.client.Get(ctx, attemptKey).Int()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get attempt count: %w", err)
	}

	return count, nil
}
