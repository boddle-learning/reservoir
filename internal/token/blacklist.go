package token

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Blacklist handles token revocation using Redis
type Blacklist struct {
	client *redis.Client
}

// NewBlacklist creates a new token blacklist
func NewBlacklist(client *redis.Client) *Blacklist {
	return &Blacklist{client: client}
}

// Add adds a token to the blacklist
func (b *Blacklist) Add(ctx context.Context, tokenID string, expiry time.Time) error {
	key := fmt.Sprintf("blacklist:jti:%s", tokenID)
	ttl := time.Until(expiry)

	if ttl <= 0 {
		// Token already expired, no need to blacklist
		return nil
	}

	err := b.client.Set(ctx, key, "1", ttl).Err()
	if err != nil {
		return fmt.Errorf("failed to blacklist token: %w", err)
	}

	return nil
}

// IsBlacklisted checks if a token is blacklisted
func (b *Blacklist) IsBlacklisted(ctx context.Context, tokenID string) (bool, error) {
	key := fmt.Sprintf("blacklist:jti:%s", tokenID)

	exists, err := b.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check blacklist: %w", err)
	}

	return exists > 0, nil
}

// Remove removes a token from the blacklist (mainly for testing)
func (b *Blacklist) Remove(ctx context.Context, tokenID string) error {
	key := fmt.Sprintf("blacklist:jti:%s", tokenID)

	err := b.client.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("failed to remove from blacklist: %w", err)
	}

	return nil
}
