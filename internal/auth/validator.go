package auth

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	// Email validation regex (RFC 5322 simplified)
	emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

	// Password requirements
	minPasswordLength = 3 // Matches Rails validation
)

// ValidationError represents a validation error
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidateLoginRequest validates a login request
func ValidateLoginRequest(req *LoginRequest) error {
	errors := make([]ValidationError, 0)

	// Validate email
	if req.Email == "" {
		errors = append(errors, ValidationError{
			Field:   "email",
			Message: "Email is required",
		})
	} else if !IsValidEmail(req.Email) {
		errors = append(errors, ValidationError{
			Field:   "email",
			Message: "Email format is invalid",
		})
	}

	// Validate password
	if req.Password == "" {
		errors = append(errors, ValidationError{
			Field:   "password",
			Message: "Password is required",
		})
	} else if len(req.Password) < minPasswordLength {
		errors = append(errors, ValidationError{
			Field:   "password",
			Message: fmt.Sprintf("Password must be at least %d characters", minPasswordLength),
		})
	}

	if len(errors) > 0 {
		return &validationErrors{Errors: errors}
	}

	return nil
}

type validationErrors struct {
	Errors []ValidationError
}

func (e *validationErrors) Error() string {
	messages := make([]string, len(e.Errors))
	for i, err := range e.Errors {
		messages[i] = err.Error()
	}
	return strings.Join(messages, "; ")
}

// IsValidEmail checks if an email address is valid
func IsValidEmail(email string) bool {
	email = strings.TrimSpace(email)
	if len(email) > 254 {
		return false
	}
	return emailRegex.MatchString(email)
}

// SanitizeEmail normalizes an email address
func SanitizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// IsStudentEmail checks if an email is a student email (username@student.student)
func IsStudentEmail(email string) bool {
	return strings.HasSuffix(strings.ToLower(email), "@student.student")
}
