package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// VerifyPassword verifies a password against a bcrypt hash
// This matches Rails' has_secure_password behavior (bcrypt cost factor 10)
func VerifyPassword(password, hash string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		if err == bcrypt.ErrMismatchedHashAndPassword {
			return fmt.Errorf("invalid password")
		}
		return fmt.Errorf("failed to verify password: %w", err)
	}
	return nil
}

// HashPassword creates a bcrypt hash of a password
// Cost factor 10 matches Rails' default
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(hash), nil
}
