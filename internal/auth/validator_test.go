package auth

import (
	"strings"
	"testing"
)

func TestIsValidEmail(t *testing.T) {
	tests := []struct {
		name  string
		email string
		want  bool
	}{
		{"valid email", "test@example.com", true},
		{"valid with subdomain", "user@mail.example.com", true},
		{"valid with plus", "user+tag@example.com", true},
		{"valid with dots", "first.last@example.co.uk", true},
		{"invalid no @", "userexample.com", false},
		{"invalid no domain", "user@", false},
		{"invalid no user", "@example.com", false},
		{"invalid spaces", "user @example.com", false},
		{"invalid double @", "user@@example.com", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidEmail(tt.email); got != tt.want {
				t.Errorf("IsValidEmail(%q) = %v, want %v", tt.email, got, tt.want)
			}
		})
	}
}

func TestSanitizeEmail(t *testing.T) {
	tests := []struct {
		name  string
		email string
		want  string
	}{
		{"lowercase", "USER@EXAMPLE.COM", "user@example.com"},
		{"trim spaces", "  user@example.com  ", "user@example.com"},
		{"both", "  USER@EXAMPLE.COM  ", "user@example.com"},
		{"already clean", "user@example.com", "user@example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SanitizeEmail(tt.email); got != tt.want {
				t.Errorf("SanitizeEmail(%q) = %q, want %q", tt.email, got, tt.want)
			}
		})
	}
}

func TestIsStudentEmail(t *testing.T) {
	tests := []struct {
		name  string
		email string
		want  bool
	}{
		{"student email", "john123@student.student", true},
		{"student email uppercase", "JOHN123@STUDENT.STUDENT", true},
		{"teacher email", "teacher@example.com", false},
		{"regular email", "user@gmail.com", false},
		{"student in domain", "user@student.example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsStudentEmail(tt.email); got != tt.want {
				t.Errorf("IsStudentEmail(%q) = %v, want %v", tt.email, got, tt.want)
			}
		})
	}
}

func TestValidateLoginRequest(t *testing.T) {
	tests := []struct {
		name        string
		req         *LoginRequest
		shouldError bool
		errorField  string
	}{
		{
			name:        "valid request",
			req:         &LoginRequest{Email: "user@example.com", Password: "password123"},
			shouldError: false,
		},
		{
			name:        "empty email",
			req:         &LoginRequest{Email: "", Password: "password123"},
			shouldError: true,
			errorField:  "email",
		},
		{
			name:        "invalid email",
			req:         &LoginRequest{Email: "notanemail", Password: "password123"},
			shouldError: true,
			errorField:  "email",
		},
		{
			name:        "empty password",
			req:         &LoginRequest{Email: "user@example.com", Password: ""},
			shouldError: true,
			errorField:  "password",
		},
		{
			name:        "short password",
			req:         &LoginRequest{Email: "user@example.com", Password: "ab"},
			shouldError: true,
			errorField:  "password",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLoginRequest(tt.req)
			if (err != nil) != tt.shouldError {
				t.Errorf("ValidateLoginRequest() error = %v, shouldError = %v", err, tt.shouldError)
				return
			}

			if err != nil && tt.errorField != "" {
				errStr := err.Error()
				if !strings.Contains(errStr, tt.errorField) {
					t.Errorf("Expected error to contain field %q, got: %v", tt.errorField, err)
				}
			}
		})
	}
}
