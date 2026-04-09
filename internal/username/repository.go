package username

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// SequenceStore defines the interface for username sequence operations.
type SequenceStore interface {
	// NextNumber atomically increments and returns the next number for the
	// given base username. If no row exists yet, it inserts with max_number = 1.
	NextNumber(ctx context.Context, base string) (int, error)

	// CurrentNumber returns the current max number for a base username, or 0
	// if none exists.
	CurrentNumber(ctx context.Context, base string) (int, error)

	// IsUsernameTaken checks whether a username already exists in the students table.
	IsUsernameTaken(ctx context.Context, username string) (bool, error)
}

// Repository implements SequenceStore against PostgreSQL.
type Repository struct {
	db *sqlx.DB
}

// NewRepository creates a new username repository.
func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

// NextNumber atomically increments and returns the next number for the given
// base username. Postgres row-level locking on the UPSERT ensures that
// concurrent callers with the same base always receive distinct numbers.
func (r *Repository) NextNumber(ctx context.Context, base string) (int, error) {
	var num int
	query := `
		INSERT INTO username_sequences (base_username, max_number)
		VALUES ($1, 1)
		ON CONFLICT (base_username)
		DO UPDATE SET max_number = username_sequences.max_number + 1
		RETURNING max_number`

	err := r.db.QueryRowContext(ctx, query, base).Scan(&num)
	if err != nil {
		return 0, fmt.Errorf("failed to get next username number for base %q: %w", base, err)
	}
	return num, nil
}

// CurrentNumber returns the current max number for a base username, or 0 if
// none exists.
func (r *Repository) CurrentNumber(ctx context.Context, base string) (int, error) {
	var num int
	query := `SELECT COALESCE(max_number, 0) FROM username_sequences WHERE base_username = $1`

	err := r.db.QueryRowContext(ctx, query, base).Scan(&num)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get current username number for base %q: %w", base, err)
	}
	return num, nil
}

// IsUsernameTaken checks whether a username already exists in the students table.
func (r *Repository) IsUsernameTaken(ctx context.Context, username string) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM students WHERE username = $1)`

	err := r.db.QueryRowContext(ctx, query, username).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check username %q: %w", username, err)
	}
	return exists, nil
}
