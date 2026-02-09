package errors

import "fmt"

// AppError represents a custom application error
type AppError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Status  int    `json:"-"`
}

// Error implements the error interface
func (e *AppError) Error() string {
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Common error codes
const (
	ErrCodeInvalidCredentials  = "INVALID_CREDENTIALS"
	ErrCodeInvalidToken        = "INVALID_TOKEN"
	ErrCodeTokenExpired        = "TOKEN_EXPIRED"
	ErrCodeTokenRevoked        = "TOKEN_REVOKED"
	ErrCodeRateLimitExceeded   = "RATE_LIMIT_EXCEEDED"
	ErrCodeValidationFailed    = "VALIDATION_FAILED"
	ErrCodeInternalError       = "INTERNAL_ERROR"
	ErrCodeUnauthorized        = "UNAUTHORIZED"
	ErrCodeForbidden           = "FORBIDDEN"
	ErrCodeNotFound            = "NOT_FOUND"
)

// NewAppError creates a new application error
func NewAppError(code, message string, status int) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Status:  status,
	}
}

// Common errors
var (
	ErrInvalidCredentials = NewAppError(ErrCodeInvalidCredentials, "Invalid email or password", 401)
	ErrInvalidToken       = NewAppError(ErrCodeInvalidToken, "Invalid token", 401)
	ErrTokenExpired       = NewAppError(ErrCodeTokenExpired, "Token expired", 401)
	ErrTokenRevoked       = NewAppError(ErrCodeTokenRevoked, "Token revoked", 401)
	ErrRateLimitExceeded  = NewAppError(ErrCodeRateLimitExceeded, "Too many login attempts", 429)
	ErrUnauthorized       = NewAppError(ErrCodeUnauthorized, "Unauthorized", 401)
)
