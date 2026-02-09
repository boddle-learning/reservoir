package auth

import (
	"net/http"

	"github.com/boddle/reservoir/internal/token"
	"github.com/boddle/reservoir/pkg/response"
	"github.com/gin-gonic/gin"
)

// Handler handles authentication HTTP requests
type Handler struct {
	service *Service
}

// NewHandler creates a new authentication handler
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Login handles email/password login
// POST /auth/login
func (h *Handler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, err.Error())
		return
	}

	// Get client IP address
	ipAddress := c.ClientIP()

	// Authenticate
	result, err := h.service.AuthenticateEmailPassword(c.Request.Context(), req.Email, req.Password, ipAddress)
	if err != nil {
		// Return 401 for invalid credentials
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_CREDENTIALS",
				"message": "Invalid email or password",
			},
		})
		return
	}

	response.Success(c, http.StatusOK, result)
}

// LoginWithToken handles login token authentication (magic links)
// GET /auth/token?token=SECRET
func (h *Handler) LoginWithToken(c *gin.Context) {
	secret := c.Query("token")
	if secret == "" {
		response.ValidationError(c, "token parameter is required")
		return
	}

	// Authenticate
	result, err := h.service.AuthenticateLoginToken(c.Request.Context(), secret)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_TOKEN",
				"message": "Invalid or expired token",
			},
		})
		return
	}

	response.Success(c, http.StatusOK, result)
}

// Logout handles logout (token revocation)
// POST /auth/logout
func (h *Handler) Logout(c *gin.Context) {
	// Get token from Authorization header
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		response.ValidationError(c, "Authorization header is required")
		return
	}

	// Extract token (format: "Bearer TOKEN")
	tokenString := ""
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		tokenString = authHeader[7:]
	} else {
		response.ValidationError(c, "Invalid Authorization header format")
		return
	}

	// Revoke token
	if err := h.service.Logout(c.Request.Context(), tokenString); err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, http.StatusOK, gin.H{
		"message": "Logged out successfully",
	})
}

// Me returns the authenticated user's information
// GET /auth/me
func (h *Handler) Me(c *gin.Context) {
	// Get claims from context (set by auth middleware)
	claimsInterface, exists := c.Get("claims")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "UNAUTHORIZED",
				"message": "Not authenticated",
			},
		})
		return
	}

	claims, ok := claimsInterface.(*token.Claims)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INTERNAL_ERROR",
				"message": "Invalid claims type",
			},
		})
		return
	}

	// Get full user data
	userWithMeta, err := h.service.GetCurrentUser(c.Request.Context(), claims)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, http.StatusOK, gin.H{
		"user": userWithMeta.User,
		"meta": userWithMeta.Meta,
	})
}

// Health returns health status
// GET /health
func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "healthy",
	})
}
