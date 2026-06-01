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
	icloudSvc   *ICloudService
}

// NewHandler creates a new OAuth handler
func NewHandler(authService *AuthService, googleSvc *GoogleService, cleverSvc *CleverService, icloudSvc *ICloudService) *Handler {
	return &Handler{
		authService: authService,
		googleSvc:   googleSvc,
		cleverSvc:   cleverSvc,
		icloudSvc:   icloudSvc,
	}
}

// GoogleTokenAuth authenticates using a pre-obtained Google access token.
// Called by LMS after OmniAuth has already completed the Google OAuth flow.
// POST /auth/google { "uid": "...", "email": "...", "name": "...", "token": "..." }
func (h *Handler) GoogleTokenAuth(c *gin.Context) {
	var req struct {
		UID   string `json:"uid"   binding:"required"`
		Email string `json:"email" binding:"required"`
		Name  string `json:"name"`
		Token string `json:"token" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_REQUEST",
				"message": "uid, email, and token are required",
			},
		})
		return
	}

	result, err := h.authService.AuthenticateWithGoogleToken(c.Request.Context(), req.UID, req.Email, req.Name, req.Token)
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

	response.Success(c, http.StatusOK, result)
}

// CleverTokenAuth authenticates using a pre-obtained Clever access token.
// Called by LMS after OmniAuth has already completed the Clever SSO flow.
// POST /auth/clever { "uid": "...", "email": "...", "name": "...", "token": "..." }
func (h *Handler) CleverTokenAuth(c *gin.Context) {
	var req struct {
		UID   string `json:"uid"   binding:"required"`
		Email string `json:"email" binding:"required"`
		Name  string `json:"name"`
		Token string `json:"token" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_REQUEST",
				"message": "uid, email, and token are required",
			},
		})
		return
	}

	result, err := h.authService.AuthenticateWithCleverToken(c.Request.Context(), req.UID, req.Email, req.Name, req.Token)
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

	response.Success(c, http.StatusOK, result)
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

// ICloudNonce issues a single-use nonce for Sign in with Apple. The client
// requests it, feeds it into the Apple authorization request, and the resulting
// ID token carries it back as the `nonce` claim — which ICloudAuth verifies.
// POST /auth/icloud/nonce -> { "nonce": "..." }
func (h *Handler) ICloudNonce(c *gin.Context) {
	if !h.icloudSvc.Configured() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "OAUTH_UNAVAILABLE",
				"message": "iCloud sign-in is not configured",
			},
		})
		return
	}

	nonce, err := h.icloudSvc.IssueNonce(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "NONCE_FAILED",
				"message": "failed to issue nonce",
			},
		})
		return
	}

	response.Success(c, http.StatusOK, gin.H{"nonce": nonce})
}

// ICloudAuth authenticates a user from an Apple "Sign in with Apple" ID token.
// The client completes Sign in with Apple (using a nonce from ICloudNonce) and
// sends the resulting ID token. The server verifies it before issuing a JWT;
// the caller can no longer assert a bare Apple UID (see LMS-6512).
// POST /auth/icloud { "identity_token": "<apple-id-token>" }
func (h *Handler) ICloudAuth(c *gin.Context) {
	var req struct {
		IdentityToken string `json:"identity_token" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_REQUEST",
				"message": "identity_token is required",
				"details": err.Error(),
			},
		})
		return
	}

	result, err := h.authService.AuthenticateWithiCloud(c.Request.Context(), req.IdentityToken)
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

	response.Success(c, http.StatusOK, gin.H{
		"token": result.Token,
		"user":  result.User,
		"meta":  result.Meta,
	})
}
