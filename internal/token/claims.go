package token

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims represents the JWT claims structure
type Claims struct {
	UserID    int    `json:"user_id"`
	BoddleUID string `json:"boddle_uid"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	MetaType  string `json:"meta_type"` // "Student", "Teacher", "Parent", "Admin"
	MetaID    int    `json:"meta_id"`
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
