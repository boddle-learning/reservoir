package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/boddle/reservoir/internal/token"
	"github.com/boddle/reservoir/internal/user"
)

// Service handles authentication business logic
type Service struct {
	userRepo       *user.Repository
	tokenService   *token.Service
	tokenBlacklist *token.Blacklist
	rateLimiter    RateLimiter
}

// RateLimiter interface for rate limiting
type RateLimiter interface {
	CheckLoginAttempt(ctx context.Context, email, ipAddress string) (allowed bool, remaining int, lockoutRemaining time.Duration, err error)
	RecordFailedAttempt(ctx context.Context, email, ipAddress string) error
	RecordSuccessfulAttempt(ctx context.Context, email, ipAddress string) error
}

// NewService creates a new authentication service
func NewService(
	userRepo *user.Repository,
	tokenService *token.Service,
	blacklist *token.Blacklist,
	rateLimiter RateLimiter,
) *Service {
	return &Service{
		userRepo:       userRepo,
		tokenService:   tokenService,
		tokenBlacklist: blacklist,
		rateLimiter:    rateLimiter,
	}
}

// LoginRequest represents a login request
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse represents a login response
type LoginResponse struct {
	Token     *token.TokenPair  `json:"token"`
	User      *user.User        `json:"user"`
	Meta      interface{}       `json:"meta,omitempty"`
}

// AuthenticateEmailPassword authenticates with email and password
func (s *Service) AuthenticateEmailPassword(ctx context.Context, email, password, ipAddress string) (*LoginResponse, error) {
	// Sanitize email
	email = SanitizeEmail(email)

	// Check rate limit
	if s.rateLimiter != nil {
		allowed, _, lockoutRemaining, err := s.rateLimiter.CheckLoginAttempt(ctx, email, ipAddress)
		if err != nil {
			// Log error but don't fail login
			fmt.Printf("rate limiter error: %v\n", err)
		} else if !allowed {
			return nil, fmt.Errorf("too many failed attempts, locked out for %v", lockoutRemaining.Round(time.Second))
		}
	}

	// Find user by email
	usr, err := s.userRepo.FindByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	}

	if usr == nil {
		// Record failed attempt
		_ = s.userRepo.RecordLoginAttempt(ctx, email, ipAddress, false)
		if s.rateLimiter != nil {
			_ = s.rateLimiter.RecordFailedAttempt(ctx, email, ipAddress)
		}
		return nil, fmt.Errorf("invalid credentials")
	}

	// Verify password
	if err := VerifyPassword(password, usr.PasswordDigest); err != nil {
		// Record failed attempt
		_ = s.userRepo.RecordLoginAttempt(ctx, email, ipAddress, false)
		if s.rateLimiter != nil {
			_ = s.rateLimiter.RecordFailedAttempt(ctx, email, ipAddress)
		}
		return nil, fmt.Errorf("invalid credentials")
	}

	// Record successful attempt
	_ = s.userRepo.RecordLoginAttempt(ctx, email, ipAddress, true)
	if s.rateLimiter != nil {
		_ = s.rateLimiter.RecordSuccessfulAttempt(ctx, email, ipAddress)
	}

	// Update last logged on
	if err := s.userRepo.UpdateLastLoggedOn(ctx, usr.ID); err != nil {
		// Log error but don't fail login
		fmt.Printf("failed to update last_logged_on: %v\n", err)
	}

	// Load meta data
	userWithMeta, err := s.userRepo.FindWithMeta(ctx, usr.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to load user meta: %w", err)
	}

	// Generate JWT token
	boddleUID := ""
	if usr.BoddleUID.Valid {
		boddleUID = usr.BoddleUID.String
	}

	tokenPair, err := s.tokenService.Generate(
		usr.ID,
		boddleUID,
		usr.Email,
		userWithMeta.GetFullName(),
		usr.MetaType,
		usr.MetaID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	return &LoginResponse{
		Token: tokenPair,
		User:  usr,
		Meta:  userWithMeta.Meta,
	}, nil
}

// AuthenticateLoginToken authenticates with a login token (magic link)
func (s *Service) AuthenticateLoginToken(ctx context.Context, secret string) (*LoginResponse, error) {
	// Find login token
	loginToken, err := s.userRepo.FindLoginToken(ctx, secret)
	if err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	}

	if loginToken == nil {
		return nil, fmt.Errorf("invalid token")
	}

	// Check if token is expired (5 minutes for non-permanent tokens)
	if !loginToken.Permanent {
		expiryTime := loginToken.CreatedAt.Add(5 * time.Minute)
		if time.Now().After(expiryTime) {
			return nil, fmt.Errorf("token expired")
		}

		// Delete non-permanent token after use
		if err := s.userRepo.DeleteLoginToken(ctx, loginToken.ID); err != nil {
			// Log error but don't fail login
			fmt.Printf("failed to delete login token: %v\n", err)
		}
	}

	// Load user with meta
	userWithMeta, err := s.userRepo.FindWithMeta(ctx, loginToken.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to load user: %w", err)
	}

	if userWithMeta == nil {
		return nil, fmt.Errorf("user not found")
	}

	usr := &userWithMeta.User

	// Update last logged on
	if err := s.userRepo.UpdateLastLoggedOn(ctx, usr.ID); err != nil {
		fmt.Printf("failed to update last_logged_on: %v\n", err)
	}

	// Generate JWT token
	boddleUID := ""
	if usr.BoddleUID.Valid {
		boddleUID = usr.BoddleUID.String
	}

	tokenPair, err := s.tokenService.Generate(
		usr.ID,
		boddleUID,
		usr.Email,
		userWithMeta.GetFullName(),
		usr.MetaType,
		usr.MetaID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	return &LoginResponse{
		Token: tokenPair,
		User:  usr,
		Meta:  userWithMeta.Meta,
	}, nil
}

// ValidateToken validates a JWT token
func (s *Service) ValidateToken(ctx context.Context, tokenString string) (*token.Claims, error) {
	// Validate token signature and expiry
	claims, err := s.tokenService.Validate(tokenString)
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	// Check if token is blacklisted
	blacklisted, err := s.tokenBlacklist.IsBlacklisted(ctx, claims.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to check blacklist: %w", err)
	}

	if blacklisted {
		return nil, fmt.Errorf("token revoked")
	}

	return claims, nil
}

// Logout revokes a token by adding it to the blacklist
func (s *Service) Logout(ctx context.Context, tokenString string) error {
	// Extract token ID and expiry
	claims, err := s.tokenService.Validate(tokenString)
	if err != nil {
		// If token is already invalid, logout succeeds
		return nil
	}

	// Add to blacklist with TTL = token expiry
	expiry := claims.ExpiresAt.Time
	if err := s.tokenBlacklist.Add(ctx, claims.ID, expiry); err != nil {
		return fmt.Errorf("failed to blacklist token: %w", err)
	}

	return nil
}

// GetCurrentUser gets the current user from token claims
func (s *Service) GetCurrentUser(ctx context.Context, claims *token.Claims) (*user.UserWithMeta, error) {
	userWithMeta, err := s.userRepo.FindWithMeta(ctx, claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to load user: %w", err)
	}

	if userWithMeta == nil {
		return nil, fmt.Errorf("user not found")
	}

	return userWithMeta, nil
}
