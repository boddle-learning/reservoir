package auth

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestVerifyPassword(t *testing.T) {
	tests := []struct {
		name        string
		password    string
		hash        string
		shouldError bool
	}{
		{
			name:        "valid password",
			password:    "TestPassword123",
			hash:        mustHashPassword("TestPassword123"),
			shouldError: false,
		},
		{
			name:        "invalid password",
			password:    "WrongPassword",
			hash:        mustHashPassword("TestPassword123"),
			shouldError: true,
		},
		{
			name:        "empty password",
			password:    "",
			hash:        mustHashPassword("TestPassword123"),
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyPassword(tt.password, tt.hash)
			if (err != nil) != tt.shouldError {
				t.Errorf("VerifyPassword() error = %v, shouldError = %v", err, tt.shouldError)
			}
		})
	}
}

func TestHashPassword(t *testing.T) {
	password := "TestPassword123"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() failed: %v", err)
	}

	// Verify the hash can be validated
	err = bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		t.Errorf("Generated hash cannot be validated: %v", err)
	}

	// Verify wrong password fails
	err = bcrypt.CompareHashAndPassword([]byte(hash), []byte("WrongPassword"))
	if err == nil {
		t.Error("Wrong password should fail validation")
	}
}

func TestHashPasswordDifferentHashes(t *testing.T) {
	password := "TestPassword123"

	hash1, _ := HashPassword(password)
	hash2, _ := HashPassword(password)

	// Bcrypt should generate different hashes due to random salt
	if hash1 == hash2 {
		t.Error("Same password should generate different hashes (bcrypt uses random salt)")
	}
}

func mustHashPassword(password string) string {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	return string(hash)
}
