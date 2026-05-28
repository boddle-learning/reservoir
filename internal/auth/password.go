package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// VerifyPassword verifies a password against a bcrypt hash
// Cost is read from the stored hash itself; Rails uses cost 12 (bcrypt gem default).
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

// HashPassword creates a bcrypt hash of a password at cost 12, matching the
// bcrypt Ruby gem default used by Rails (has_secure_password).
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(hash), nil
}
