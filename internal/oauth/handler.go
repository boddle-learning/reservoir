package oauth

import (
	"net/http"

	"github.com/boddle/reservoir/pkg/response"
	"github.com/gin-gonic/gin"
)

// Handler handles OAuth HTTP requests
type Handler struct {
	authService *AuthService
	googleSvc   *GoogleService
	cleverSvc   *CleverService
	icloudSvc   *iCloudService
}

// NewHandler creates a new OAuth handler
func NewHandler(authService *AuthService, googleSvc *GoogleService, cleverSvc *CleverService, icloudSvc *iCloudService) *Handler {
	return &Handler{
		authService: authService,
		googleSvc:   googleSvc,
		cleverSvc:   cleverSvc,
		icloudSvc:   icloudSvc,
	}
}

// GoogleLogin initiates Google OAuth flow
// GET /auth/google?redirect_url=...
func (h *Handler) GoogleLogin(c *gin.Context) {
	redirectURL := c.Query("redirect_url")
	if redirectURL == "" {
		redirectURL = "/" // Default redirect
	}

	// Generate OAuth URL
	authURL, err := h.googleSvc.GetAuthURL(c.Request.Context(), redirectURL)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Redirect to Google OAuth page
	c.Redirect(http.StatusTemporaryRedirect, authURL)
}

// GoogleCallback handles Google OAuth callback
// GET /auth/google/callback?code=...&state=...
func (h *Handler) GoogleCallback(c *gin.Context) {
	code := c.Query("code")
	state := c.Query("state")

	if code == "" || state == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_REQUEST",
				"message": "Missing code or state parameter",
			},
		})
		return
	}

	// Authenticate with Google
	result, redirectURL, err := h.authService.AuthenticateWithGoogle(c.Request.Context(), code, state)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "OAUTH_FAILED",
				"message": err.Error(),
			},
		})
		return
	}

	// For web clients, we can redirect with token in URL (or use a different flow)
	// For now, return JSON response
	response.Success(c, http.StatusOK, gin.H{
		"token":        result.Token,
		"user":         result.User,
		"meta":         result.Meta,
		"redirect_url": redirectURL,
	})
}

// CleverLogin initiates Clever SSO flow
// GET /auth/clever?redirect_url=...
func (h *Handler) CleverLogin(c *gin.Context) {
	redirectURL := c.Query("redirect_url")
	if redirectURL == "" {
		redirectURL = "/" // Default redirect
	}

	// Generate OAuth URL
	authURL, err := h.cleverSvc.GetAuthURL(c.Request.Context(), redirectURL)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Redirect to Clever OAuth page
	c.Redirect(http.StatusTemporaryRedirect, authURL)
}

// CleverCallback handles Clever OAuth callback
// GET /auth/clever/callback?code=...&state=...
func (h *Handler) CleverCallback(c *gin.Context) {
	code := c.Query("code")
	state := c.Query("state")

	if code == "" || state == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_REQUEST",
				"message": "Missing code or state parameter",
			},
		})
		return
	}

	// Authenticate with Clever
	result, redirectURL, err := h.authService.AuthenticateWithClever(c.Request.Context(), code, state)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "OAUTH_FAILED",
				"message": err.Error(),
			},
		})
		return
	}

	// Return JSON response
	response.Success(c, http.StatusOK, gin.H{
		"token":        result.Token,
		"user":         result.User,
		"meta":         result.Meta,
		"redirect_url": redirectURL,
	})
}

// ICloudLogin initiates iCloud Sign In flow
// GET /auth/icloud?redirect_url=...
func (h *Handler) ICloudLogin(c *gin.Context) {
	redirectURL := c.Query("redirect_url")
	if redirectURL == "" {
		redirectURL = "/" // Default redirect
	}

	// Generate OAuth URL
	authURL, err := h.icloudSvc.GetAuthURL(c.Request.Context(), redirectURL)
	if err != nil {
		response.Error(c, err)
		return
	}

	// Redirect to iCloud Sign In page
	c.Redirect(http.StatusTemporaryRedirect, authURL)
}

// ICloudCallback handles iCloud Sign In callback
// POST /auth/icloud/callback (Apple uses form_post)
// Note: Apple sends the callback as a POST with form data
func (h *Handler) ICloudCallback(c *gin.Context) {
	// iCloud sends callback as POST with form data
	code := c.PostForm("code")
	state := c.PostForm("state")

	// Fallback to query params for GET requests (during testing)
	if code == "" {
		code = c.Query("code")
	}
	if state == "" {
		state = c.Query("state")
	}

	if code == "" || state == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_REQUEST",
				"message": "Missing code or state parameter",
			},
		})
		return
	}

	// Authenticate with iCloud
	result, redirectURL, err := h.authService.AuthenticateWithiCloud(c.Request.Context(), code, state)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "OAUTH_FAILED",
				"message": err.Error(),
			},
		})
		return
	}

	// Return JSON response
	response.Success(c, http.StatusOK, gin.H{
		"token":        result.Token,
		"user":         result.User,
		"meta":         result.Meta,
		"redirect_url": redirectURL,
	})
}
