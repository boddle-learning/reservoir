package token

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Service handles JWT token operations
type Service struct {
	secretKey        []byte
	refreshSecretKey []byte
	accessTokenTTL   time.Duration
	refreshTokenTTL  time.Duration
}

// NewService creates a new token service
func NewService(secretKey, refreshSecretKey string, accessTTL, refreshTTL time.Duration) *Service {
	return &Service{
		secretKey:        []byte(secretKey),
		refreshSecretKey: []byte(refreshSecretKey),
		accessTokenTTL:   accessTTL,
		refreshTokenTTL:  refreshTTL,
	}
}

// Generate generates a new token pair (access + refresh)
func (s *Service) Generate(userID int, boddleUID, email, name, metaType string, metaID int) (*TokenPair, error) {
	now := time.Now()
	accessExpiry := now.Add(s.accessTokenTTL)
	refreshExpiry := now.Add(s.refreshTokenTTL)

	// Generate access token
	accessClaims := Claims{
		UserID:    userID,
		BoddleUID: boddleUID,
		Email:     email,
		Name:      name,
		MetaType:  metaType,
		MetaID:    metaID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(accessExpiry),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "boddle-auth-gateway",
			Subject:   fmt.Sprintf("%d", userID),
			ID:        uuid.New().String(), // JTI for token revocation
		},
	}

	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessTokenString, err := accessToken.SignedString(s.secretKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign access token: %w", err)
	}

	// Generate refresh token
	refreshClaims := jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(refreshExpiry),
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now),
		Issuer:    "boddle-auth-gateway",
		Subject:   fmt.Sprintf("%d", userID),
		ID:        uuid.New().String(),
	}

	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshTokenString, err := refreshToken.SignedString(s.refreshSecretKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessTokenString,
		RefreshToken: refreshTokenString,
		ExpiresAt:    accessExpiry,
		TokenType:    TokenTypeBearer,
	}, nil
}

// Validate validates an access token and returns the claims
func (s *Service) Validate(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.secretKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}

// ValidateRefreshToken validates a refresh token and returns the standard claims
func (s *Service) ValidateRefreshToken(tokenString string) (*jwt.RegisteredClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &jwt.RegisteredClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.refreshSecretKey, nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse refresh token: %w", err)
	}

	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid refresh token claims")
	}

	return claims, nil
}

// ExtractTokenID extracts the JTI (JWT ID) from a token string without full validation
// This is useful for blacklist checking before expensive validation
func (s *Service) ExtractTokenID(tokenString string) (string, error) {
	token, _, err := jwt.NewParser().ParseUnverified(tokenString, &Claims{})
	if err != nil {
		return "", fmt.Errorf("failed to parse token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		return "", fmt.Errorf("invalid token claims")
	}

	return claims.ID, nil
}
