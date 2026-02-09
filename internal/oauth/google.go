package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/boddle/reservoir/internal/config"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// GoogleService handles Google OAuth2 authentication
type GoogleService struct {
	config       *oauth2.Config
	stateManager *StateManager
}

// NewGoogleService creates a new Google OAuth service
func NewGoogleService(cfg config.GoogleConfig, stateManager *StateManager) *GoogleService {
	oauthConfig := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Scopes: []string{
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
		},
		Endpoint: google.Endpoint,
	}

	return &GoogleService{
		config:       oauthConfig,
		stateManager: stateManager,
	}
}

// GetAuthURL generates the Google OAuth authorization URL
func (gs *GoogleService) GetAuthURL(ctx context.Context, redirectURL string) (string, error) {
	// Generate and save state
	state, err := gs.stateManager.GenerateState()
	if err != nil {
		return "", err
	}

	if err := gs.stateManager.SaveState(ctx, state, redirectURL); err != nil {
		return "", err
	}

	// Generate OAuth URL
	url := gs.config.AuthCodeURL(state, oauth2.AccessTypeOffline)

	return url, nil
}

// HandleCallback handles the OAuth callback and returns user info
func (gs *GoogleService) HandleCallback(ctx context.Context, code, state string) (*OAuthUserInfo, string, error) {
	// Validate state
	redirectURL, err := gs.stateManager.ValidateState(ctx, state)
	if err != nil {
		return nil, "", fmt.Errorf("invalid state: %w", err)
	}

	// Exchange code for token
	token, err := gs.config.Exchange(ctx, code)
	if err != nil {
		return nil, "", fmt.Errorf("failed to exchange code: %w", err)
	}

	// Fetch user info
	userInfo, err := gs.fetchUserInfo(ctx, token.AccessToken)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch user info: %w", err)
	}

	return userInfo, redirectURL, nil
}

// fetchUserInfo fetches user information from Google
func (gs *GoogleService) fetchUserInfo(ctx context.Context, accessToken string) (*OAuthUserInfo, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		"GET",
		"https://www.googleapis.com/oauth2/v2/userinfo",
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
		return nil, fmt.Errorf("Google API returned status %d: %s", resp.StatusCode, string(body))
	}

	var googleUser struct {
		ID            string `json:"id"`
		Email         string `json:"email"`
		VerifiedEmail bool   `json:"verified_email"`
		GivenName     string `json:"given_name"`
		FamilyName    string `json:"family_name"`
		Picture       string `json:"picture"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&googleUser); err != nil {
		return nil, fmt.Errorf("failed to decode user info: %w", err)
	}

	return &OAuthUserInfo{
		ProviderUserID: googleUser.ID,
		Email:          googleUser.Email,
		FirstName:      googleUser.GivenName,
		LastName:       googleUser.FamilyName,
		Picture:        googleUser.Picture,
		EmailVerified:  googleUser.VerifiedEmail,
	}, nil
}
