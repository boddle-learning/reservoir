package username

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

const (
	MaxUsernameLength = 15
	maxRetries        = 10
)

// Service generates unique student usernames.
type Service struct {
	store SequenceStore
}

// NewService creates a new username service.
func NewService(store SequenceStore) *Service {
	return &Service{store: store}
}

// Generate produces the next unique username for a student given their first
// and last name. The format is {firstName}{lastInitial}{number}, lowercased,
// capped at 15 characters total.
//
// Thread-safety: the underlying SequenceStore.NextNumber call is an atomic
// Postgres UPSERT that serialises via row-level locking, so concurrent callers
// sharing the same base always receive distinct numbers.
//
// As defence-in-depth, the generated username is verified against the students
// table. If a collision is detected (e.g. from cross-base truncation overlap or
// pre-existing data), the method retries with the next number.
func (s *Service) Generate(ctx context.Context, firstName, lastName string) (string, error) {
	base := BuildBase(firstName, lastName)
	if base == "" {
		return "", fmt.Errorf("cannot generate username: first name is empty")
	}

	// Truncate to leave room for at least a 1-digit number.
	if len(base) > MaxUsernameLength-1 {
		base = base[:MaxUsernameLength-1]
	}

	for attempt := 0; attempt < maxRetries; attempt++ {
		num, err := s.store.NextNumber(ctx, base)
		if err != nil {
			return "", err
		}

		uname := formatUsername(base, num)
		if uname == "" {
			return "", fmt.Errorf("username number %d exceeds maximum length for base %q", num, base)
		}

		taken, err := s.store.IsUsernameTaken(ctx, uname)
		if err != nil {
			return "", err
		}
		if !taken {
			return uname, nil
		}
		// Collision detected — loop to get the next number.
	}

	return "", fmt.Errorf("failed to generate unique username for %q %q after %d attempts", firstName, lastName, maxRetries)
}

// formatUsername combines a base and number into a username that fits within
// MaxUsernameLength. Returns "" if the number is too large to fit.
func formatUsername(base string, num int) string {
	numStr := strconv.Itoa(num)
	maxBaseLen := MaxUsernameLength - len(numStr)
	if maxBaseLen < 1 {
		return ""
	}
	if len(base) > maxBaseLen {
		base = base[:maxBaseLen]
	}
	return base + numStr
}

// BuildBase computes the lowercase base username from a first name and last
// name: all alphabetic characters from firstName, plus the first alphabetic
// character of lastName.
func BuildBase(firstName, lastName string) string {
	first := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) {
			return unicode.ToLower(r)
		}
		return -1 // drop non-letters
	}, firstName)

	lastInitial := ""
	for _, r := range lastName {
		if unicode.IsLetter(r) {
			lastInitial = string(unicode.ToLower(r))
			break
		}
	}

	return first + lastInitial
}
