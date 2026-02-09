package response

import (
	"github.com/gin-gonic/gin"
	apperrors "github.com/boddle/reservoir/pkg/errors"
)

// Success sends a successful JSON response
func Success(c *gin.Context, status int, data interface{}) {
	c.JSON(status, gin.H{
		"success": true,
		"data":    data,
	})
}

// Error sends an error JSON response
func Error(c *gin.Context, err error) {
	if appErr, ok := err.(*apperrors.AppError); ok {
		c.JSON(appErr.Status, gin.H{
			"success": false,
			"error": gin.H{
				"code":    appErr.Code,
				"message": appErr.Message,
			},
		})
		return
	}

	// Default internal server error
	c.JSON(500, gin.H{
		"success": false,
		"error": gin.H{
			"code":    apperrors.ErrCodeInternalError,
			"message": "Internal server error",
		},
	})
}

// ValidationError sends a validation error response
func ValidationError(c *gin.Context, message string) {
	c.JSON(400, gin.H{
		"success": false,
		"error": gin.H{
			"code":    apperrors.ErrCodeValidationFailed,
			"message": message,
		},
	})
}
