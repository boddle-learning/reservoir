package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/boddle/reservoir/internal/config"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// googleUserInfoURL is Google's OAuth2 userinfo endpoint. A request bearing a
// Google access token returns the identity that token was issued for; an
// invalid/expired token yields a non-200. This is what lets us treat the
// response as ground truth instead of trusting caller-supplied identity.
const googleUserInfoURL = "https://www.googleapis.com/oauth2/v2/userinfo"

// googleTokenInfoURL is Google's tokeninfo endpoint. Given an access token it
// returns the token's metadata — including aud/azp, the OAuth client the token
// was issued to — which lets us reject tokens minted for an unrelated app.
const googleTokenInfoURL = "https://oauth2.googleapis.com/tokeninfo"

// GoogleService handles Google OAuth2 authentication
type GoogleService struct {
	config           *oauth2.Config
	stateManager     *StateManager
	userInfoURL      string
	tokenInfoURL     string
	allowedAudiences []string
	httpClient       *http.Client
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
		config:           oauthConfig,
		stateManager:     stateManager,
		userInfoURL:      googleUserInfoURL,
		tokenInfoURL:     googleTokenInfoURL,
		allowedAudiences: parseAudiences(cfg.TokenAudiences),
		httpClient:       &http.Client{Timeout: 10 * time.Second},
	}
}

// parseAudiences splits a comma-separated audience allowlist into trimmed,
// non-empty entries.
func parseAudiences(raw string) []string {
	var out []string
	for _, a := range strings.Split(raw, ",") {
		if a = strings.TrimSpace(a); a != "" {
			out = append(out, a)
		}
	}
	return out
}

// verifyTokenAudience confirms a Google access token was issued to one of the
// configured client IDs (the LMS's OmniAuth app). It defends against a
// confused-deputy replay: without it, a valid Google token minted for any
// unrelated OAuth app could be exchanged for a Reservoir JWT, since the
// userinfo endpoint does not check audience.
//
// No-op when no audiences are configured (GOOGLE_TOKEN_AUDIENCES unset).
func (gs *GoogleService) verifyTokenAudience(ctx context.Context, accessToken string) error {
	if len(gs.allowedAudiences) == 0 {
		return nil
	}

	endpoint := gs.tokenInfoURL + "?access_token=" + url.QueryEscape(accessToken)
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return err
	}

	resp, err := gs.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call tokeninfo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("tokeninfo returned status %d: %s", resp.StatusCode, string(body))
	}

	var info struct {
		Aud string `json:"aud"`
		Azp string `json:"azp"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return fmt.Errorf("failed to decode tokeninfo: %w", err)
	}

	for _, allowed := range gs.allowedAudiences {
		if info.Aud == allowed || info.Azp == allowed {
			return nil
		}
	}
	return fmt.Errorf("access token audience %q not in allowlist", info.Aud)
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
		gs.userInfoURL,
		nil,
	)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := gs.httpClient.Do(req)
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
