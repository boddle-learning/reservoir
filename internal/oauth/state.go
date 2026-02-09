package oauth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// StateManager manages OAuth state tokens for CSRF prevention
type StateManager struct {
	client *redis.Client
	ttl    time.Duration
}

// NewStateManager creates a new OAuth state manager
func NewStateManager(client *redis.Client) *StateManager {
	return &StateManager{
		client: client,
		ttl:    10 * time.Minute, // State expires after 10 minutes
	}
}

// GenerateState generates a random state token
func (sm *StateManager) GenerateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random state: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// SaveState saves a state token to Redis
func (sm *StateManager) SaveState(ctx context.Context, state, redirectURL string) error {
	key := fmt.Sprintf("oauth:state:%s", state)

	err := sm.client.Set(ctx, key, redirectURL, sm.ttl).Err()
	if err != nil {
		return fmt.Errorf("failed to save OAuth state: %w", err)
	}

	return nil
}

// ValidateState validates a state token and returns the redirect URL
func (sm *StateManager) ValidateState(ctx context.Context, state string) (string, error) {
	key := fmt.Sprintf("oauth:state:%s", state)

	redirectURL, err := sm.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", fmt.Errorf("invalid or expired state token")
	}
	if err != nil {
		return "", fmt.Errorf("failed to validate OAuth state: %w", err)
	}

	// Delete state after use (one-time use)
	_ = sm.client.Del(ctx, key).Err()

	return redirectURL, nil
}

// OAuthUserInfo represents user information from OAuth provider
type OAuthUserInfo struct {
	ProviderUserID string
	Email          string
	FirstName      string
	LastName       string
	Picture        string
	EmailVerified  bool
}
