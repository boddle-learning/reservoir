package middleware

import (
	"net/http"
	"strings"

	"github.com/boddle/reservoir/internal/auth"
	"github.com/gin-gonic/gin"
)

// Auth creates an authentication middleware
func Auth(authService *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get token from Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "UNAUTHORIZED",
					"message": "Missing Authorization header",
				},
			})
			return
		}

		// Extract token (format: "Bearer TOKEN")
		tokenString := ""
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenString = authHeader[7:]
		} else {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "UNAUTHORIZED",
					"message": "Invalid Authorization header format",
				},
			})
			return
		}

		// Validate token
		claims, err := authService.ValidateToken(c.Request.Context(), tokenString)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "INVALID_TOKEN",
					"message": err.Error(),
				},
			})
			return
		}

		// Set claims in context
		c.Set("claims", claims)
		c.Set("user_id", claims.UserID)

		c.Next()
	}
}
