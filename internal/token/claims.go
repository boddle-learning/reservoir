package token

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims represents the JWT access-token claims structure
type Claims struct {
	UserID    int    `json:"user_id"`
	BoddleUID string `json:"boddle_uid"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	MetaType  string `json:"meta_type"` // "Student", "Teacher", "Parent", "Admin"
	MetaID    int    `json:"meta_id"`
	// TokenVersion mirrors users.token_version at issue time. Logout bumps the
	// column, after which tokens carrying the old version are rejected. See
	// security review Finding 2 / LMS-6513.
	TokenVersion int `json:"tver"`
	jwt.RegisteredClaims
}

// RefreshClaims represents the JWT refresh-token claims. It carries the same
// TokenVersion so a refresh is rejected once the user's version is bumped.
type RefreshClaims struct {
	TokenVersion int `json:"tver"`
	jwt.RegisteredClaims
}

// TokenPair represents an access and refresh token pair
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	TokenType    string    `json:"token_type"`
}

// TokenType constants
const (
	TokenTypeBearer = "Bearer"
)
