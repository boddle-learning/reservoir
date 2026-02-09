package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Limiter handles rate limiting using Redis
type Limiter struct {
	client          *redis.Client
	window          time.Duration // Time window for counting attempts
	maxAttempts     int           // Maximum attempts allowed in window
	lockoutDuration time.Duration // How long to block after exceeding limit
}

// NewLimiter creates a new rate limiter
func NewLimiter(client *redis.Client, window time.Duration, maxAttempts int, lockoutDuration time.Duration) *Limiter {
	return &Limiter{
		client:          client,
		window:          window,
		maxAttempts:     maxAttempts,
		lockoutDuration: lockoutDuration,
	}
}

// LoginAttemptKey returns the Redis key for tracking login attempts
func (l *Limiter) LoginAttemptKey(email, ipAddress string) string {
	return fmt.Sprintf("ratelimit:login:%s:%s", ipAddress, email)
}

// LoginLockoutKey returns the Redis key for lockout status
func (l *Limiter) LoginLockoutKey(email, ipAddress string) string {
	return fmt.Sprintf("ratelimit:lockout:%s:%s", ipAddress, email)
}

// CheckLoginAttempt checks if a login attempt is allowed
// Returns: allowed (bool), remainingAttempts (int), lockoutRemaining (time.Duration), error
func (l *Limiter) CheckLoginAttempt(ctx context.Context, email, ipAddress string) (bool, int, time.Duration, error) {
	lockoutKey := l.LoginLockoutKey(email, ipAddress)

	// Check if currently locked out
	ttl, err := l.client.TTL(ctx, lockoutKey).Result()
	if err != nil && err != redis.Nil {
		return false, 0, 0, fmt.Errorf("failed to check lockout status: %w", err)
	}

	if ttl > 0 {
		// Still locked out
		return false, 0, ttl, nil
	}

	// Check attempt count
	attemptKey := l.LoginAttemptKey(email, ipAddress)
	count, err := l.client.Get(ctx, attemptKey).Int()
	if err != nil && err != redis.Nil {
		return false, 0, 0, fmt.Errorf("failed to get attempt count: %w", err)
	}

	remaining := l.maxAttempts - count
	if remaining <= 0 {
		// Exceeded max attempts, initiate lockout
		if err := l.client.Set(ctx, lockoutKey, "1", l.lockoutDuration).Err(); err != nil {
			return false, 0, 0, fmt.Errorf("failed to set lockout: %w", err)
		}
		// Clear attempt counter
		if err := l.client.Del(ctx, attemptKey).Err(); err != nil {
			// Log error but don't fail
			fmt.Printf("failed to clear attempt counter: %v\n", err)
		}
		return false, 0, l.lockoutDuration, nil
	}

	// Attempt allowed
	return true, remaining, 0, nil
}

// RecordFailedAttempt records a failed login attempt
func (l *Limiter) RecordFailedAttempt(ctx context.Context, email, ipAddress string) error {
	attemptKey := l.LoginAttemptKey(email, ipAddress)

	// Increment attempt counter
	count, err := l.client.Incr(ctx, attemptKey).Result()
	if err != nil {
		return fmt.Errorf("failed to increment attempt counter: %w", err)
	}

	// Set expiry on first attempt
	if count == 1 {
		if err := l.client.Expire(ctx, attemptKey, l.window).Err(); err != nil {
			return fmt.Errorf("failed to set expiry: %w", err)
		}
	}

	return nil
}

// RecordSuccessfulAttempt clears the attempt counter after a successful login
func (l *Limiter) RecordSuccessfulAttempt(ctx context.Context, email, ipAddress string) error {
	attemptKey := l.LoginAttemptKey(email, ipAddress)

	// Clear attempt counter
	if err := l.client.Del(ctx, attemptKey).Err(); err != nil {
		return fmt.Errorf("failed to clear attempt counter: %w", err)
	}

	return nil
}

// ClearLockout manually clears a lockout (admin function)
func (l *Limiter) ClearLockout(ctx context.Context, email, ipAddress string) error {
	lockoutKey := l.LoginLockoutKey(email, ipAddress)
	attemptKey := l.LoginAttemptKey(email, ipAddress)

	// Clear both lockout and attempt counter
	if err := l.client.Del(ctx, lockoutKey, attemptKey).Err(); err != nil {
		return fmt.Errorf("failed to clear lockout: %w", err)
	}

	return nil
}

// GetAttemptCount returns the current attempt count
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
