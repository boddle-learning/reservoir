package auth

import (
	"context"
	"net/http"
	"time"

	"github.com/boddle/reservoir/internal/token"
	"github.com/boddle/reservoir/pkg/response"
	"github.com/gin-gonic/gin"
)

// DBPinger is satisfied by *database.DB. Defined here to avoid an import
// cycle between auth and database packages.
type DBPinger interface {
	Health(ctx context.Context) error
}

// Handler handles authentication HTTP requests
type Handler struct {
	service  *Service
	dbWriter DBPinger
	dbReader DBPinger // nil when no dedicated read replica is configured
}

// NewHandler creates a new authentication handler. Pass nil for dbReader when
// no read replica is configured — it will be omitted from the health response.
func NewHandler(service *Service, dbWriter DBPinger, dbReader DBPinger) *Handler {
	return &Handler{service: service, dbWriter: dbWriter, dbReader: dbReader}
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

// Refresh exchanges a valid refresh token for a new token pair
// POST /auth/refresh
func (h *Handler) Refresh(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, "refresh_token is required")
		return
	}

	result, err := h.service.RefreshToken(c.Request.Context(), req.RefreshToken)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"error": gin.H{
				"code":    "INVALID_REFRESH_TOKEN",
				"message": "Invalid or expired refresh token",
			},
		})
		return
	}

	response.Success(c, http.StatusOK, result)
}

// Health returns service health with DB connectivity status.
// Always returns HTTP 200 — DB errors are reported in the body, not the status
// code, so ALB health checks never kill tasks due to a transient DB blip.
// GET /health
func (h *Handler) Health(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	dbStatus := func(p DBPinger) string {
		if err := p.Health(ctx); err != nil {
			return "error"
		}
		return "ok"
	}

	body := gin.H{
		"status":    "healthy",
		"db_writer": dbStatus(h.dbWriter),
	}
	if h.dbReader != nil {
		body["db_reader"] = dbStatus(h.dbReader)
	}

	c.JSON(http.StatusOK, body)
}
