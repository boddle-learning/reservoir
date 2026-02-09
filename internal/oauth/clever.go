package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/boddle/reservoir/internal/config"
	"golang.org/x/oauth2"
)

// CleverService handles Clever SSO authentication
type CleverService struct {
	config       *oauth2.Config
	stateManager *StateManager
}

// NewCleverService creates a new Clever SSO service
func NewCleverService(cfg config.CleverConfig, stateManager *StateManager) *CleverService {
	oauthConfig := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Scopes:       []string{}, // Clever doesn't use scopes in the same way
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://clever.com/oauth/authorize",
			TokenURL: "https://clever.com/oauth/tokens",
		},
	}

	return &CleverService{
		config:       oauthConfig,
		stateManager: stateManager,
	}
}

// GetAuthURL generates the Clever OAuth authorization URL
func (cs *CleverService) GetAuthURL(ctx context.Context, redirectURL string) (string, error) {
	// Generate and save state
	state, err := cs.stateManager.GenerateState()
	if err != nil {
		return "", err
	}

	if err := cs.stateManager.SaveState(ctx, state, redirectURL); err != nil {
		return "", err
	}

	// Generate OAuth URL with district_id parameter for district-specific login
	url := cs.config.AuthCodeURL(state)

	return url, nil
}

// HandleCallback handles the Clever OAuth callback and returns user info
func (cs *CleverService) HandleCallback(ctx context.Context, code, state string) (*OAuthUserInfo, string, error) {
	// Validate state
	redirectURL, err := cs.stateManager.ValidateState(ctx, state)
	if err != nil {
		return nil, "", fmt.Errorf("invalid state: %w", err)
	}

	// Exchange code for token
	token, err := cs.config.Exchange(ctx, code)
	if err != nil {
		return nil, "", fmt.Errorf("failed to exchange code: %w", err)
	}

	// Fetch user info
	userInfo, err := cs.fetchUserInfo(ctx, token.AccessToken)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch user info: %w", err)
	}

	return userInfo, redirectURL, nil
}

// fetchUserInfo fetches user information from Clever API
func (cs *CleverService) fetchUserInfo(ctx context.Context, accessToken string) (*OAuthUserInfo, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		"GET",
		"https://api.clever.com/v3.0/me",
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
		return nil, fmt.Errorf("Clever API returned status %d: %s", resp.StatusCode, string(body))
	}

	var cleverResponse struct {
		Data struct {
			ID    string `json:"id"`
			Type  string `json:"type"` // "teacher" or "student" or "district_admin"
			Email string `json:"email"`
			Name  struct {
				First string `json:"first"`
				Last  string `json:"last"`
			} `json:"name"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&cleverResponse); err != nil {
		return nil, fmt.Errorf("failed to decode user info: %w", err)
	}

	data := cleverResponse.Data

	return &OAuthUserInfo{
		ProviderUserID: data.ID,
		Email:          data.Email,
		FirstName:      data.Name.First,
		LastName:       data.Name.Last,
		EmailVerified:  true, // Clever accounts are pre-verified by schools
	}, nil
}
