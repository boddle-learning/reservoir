package oauth

import (
	"context"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/boddle/reservoir/internal/config"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/oauth2"
)

// iCloudService handles Apple iCloud Sign In authentication
type iCloudService struct {
	config       *oauth2.Config
	stateManager *StateManager
	privateKey   *ecdsa.PrivateKey
	keyID        string
	teamID       string
	serviceID    string
}

// NewiCloudService creates a new iCloud Sign In service
func NewiCloudService(cfg config.ICloudConfig, stateManager *StateManager) (*iCloudService, error) {
	// Load Apple private key
	privateKey, err := loadApplePrivateKey(cfg.PrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load Apple private key: %w", err)
	}

	oauthConfig := &oauth2.Config{
		ClientID:    cfg.ServiceID,
		RedirectURL: cfg.RedirectURL,
		Scopes:      []string{"name", "email"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://appleid.apple.com/auth/authorize",
			TokenURL: "https://appleid.apple.com/auth/token",
		},
	}

	return &iCloudService{
		config:       oauthConfig,
		stateManager: stateManager,
		privateKey:   privateKey,
		keyID:        cfg.KeyID,
		teamID:       cfg.TeamID,
		serviceID:    cfg.ServiceID,
	}, nil
}

// GetAuthURL generates the Apple Sign In authorization URL
func (is *iCloudService) GetAuthURL(ctx context.Context, redirectURL string) (string, error) {
	// Generate and save state
	state, err := is.stateManager.GenerateState()
	if err != nil {
		return "", err
	}

	if err := is.stateManager.SaveState(ctx, state, redirectURL); err != nil {
		return "", err
	}

	// Generate OAuth URL with response_mode=form_post for better security
	url := is.config.AuthCodeURL(state, oauth2.SetAuthURLParam("response_mode", "form_post"))

	return url, nil
}

// HandleCallback handles the Apple Sign In callback and returns user info
func (is *iCloudService) HandleCallback(ctx context.Context, code, state string) (*OAuthUserInfo, string, error) {
	// Validate state
	redirectURL, err := is.stateManager.ValidateState(ctx, state)
	if err != nil {
		return nil, "", fmt.Errorf("invalid state: %w", err)
	}

	// Generate client secret (Apple requires JWT-signed client secret)
	clientSecret, err := is.generateClientSecret()
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate client secret: %w", err)
	}

	// Exchange code for token with client secret
	token, err := is.config.Exchange(
		ctx,
		code,
		oauth2.SetAuthURLParam("client_secret", clientSecret),
	)
	if err != nil {
		return nil, "", fmt.Errorf("failed to exchange code: %w", err)
	}

	// Parse ID token to get user info
	userInfo, err := is.parseIDToken(token.Extra("id_token").(string))
	if err != nil {
		return nil, "", fmt.Errorf("failed to parse ID token: %w", err)
	}

	return userInfo, redirectURL, nil
}

// generateClientSecret generates a JWT client secret signed with Apple private key
// Apple requires this instead of a static client secret
func (is *iCloudService) generateClientSecret() (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Issuer:    is.teamID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
		Audience:  jwt.ClaimStrings{"https://appleid.apple.com"},
		Subject:   is.serviceID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = is.keyID

	signedToken, err := token.SignedString(is.privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign client secret: %w", err)
	}

	return signedToken, nil
}

// parseIDToken parses the Apple ID token and extracts user information
func (is *iCloudService) parseIDToken(idToken string) (*OAuthUserInfo, error) {
	// Parse JWT without verification (Apple's public keys would need to be fetched)
	// In production, you should verify the signature using Apple's public keys
	token, _, err := new(jwt.Parser).ParseUnverified(idToken, jwt.MapClaims{})
	if err != nil {
		return nil, fmt.Errorf("failed to parse ID token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}

	// Extract user information from claims
	sub, _ := claims["sub"].(string)             // Unique user identifier
	email, _ := claims["email"].(string)         // Email (may be private relay)
	emailVerified, _ := claims["email_verified"] // Email verification status

	// Apple may return "Hide My Email" proxy addresses
	// Format: random@privaterelay.appleid.com
	isPrivateEmail := false
	if email != "" {
		// Check if it's a private relay email
		isPrivateEmail = len(email) > 25 && email[len(email)-25:] == "@privaterelay.appleid.com"
	}

	userInfo := &OAuthUserInfo{
		ProviderUserID: sub,
		Email:          email,
		EmailVerified:  emailVerified == "true" || emailVerified == true,
	}

	// Note: Apple only provides name on the FIRST sign-in
	// Subsequent sign-ins won't include name data
	// The frontend should capture this on first login
	if nameData, ok := claims["name"].(map[string]interface{}); ok {
		if firstName, ok := nameData["firstName"].(string); ok {
			userInfo.FirstName = firstName
		}
		if lastName, ok := nameData["lastName"].(string); ok {
			userInfo.LastName = lastName
		}
	}

	// Store metadata about private email
	if isPrivateEmail {
		// You might want to handle this specially in your application
		fmt.Printf("User authenticated with Apple Private Relay email: %s\n", email)
	}

	return userInfo, nil
}

// loadApplePrivateKey loads the ECDSA private key from a PEM file
func loadApplePrivateKey(path string) (*ecdsa.PrivateKey, error) {
	keyData, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read private key file: %w", err)
	}

	block, _ := pem.Decode(keyData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	ecdsaKey, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not ECDSA")
	}

	return ecdsaKey, nil
}

// fetchUserInfo is not used for Apple Sign In (user info is in ID token)
// Kept for interface compatibility
func (is *iCloudService) fetchUserInfo(ctx context.Context, accessToken string) (*OAuthUserInfo, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		"GET",
		"https://appleid.apple.com/auth/userinfo",
		nil,
	)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch user info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Apple API returned status %d: %s", resp.StatusCode, string(body))
	}

	var appleUser struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&appleUser); err != nil {
		return nil, fmt.Errorf("failed to decode user info: %w", err)
	}

	return &OAuthUserInfo{
		ProviderUserID: appleUser.Sub,
		Email:          appleUser.Email,
		EmailVerified:  appleUser.EmailVerified,
	}, nil
}
