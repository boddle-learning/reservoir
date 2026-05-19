package database

import (
	"context"
	"fmt"
	"time"

	"github.com/boddle/reservoir/internal/config"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq" // PostgreSQL driver
)

// DB wraps the sqlx database connection
type DB struct {
	*sqlx.DB
}

// NewPostgresDB creates a new PostgreSQL database connection
func NewPostgresDB(cfg config.DatabaseConfig) (*DB, error) {
	connStr := cfg.ConnectionString()

	db, err := sqlx.Connect("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(50)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(10 * time.Minute)

	// Test the connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{DB: db}, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.DB.Close()
}

// Health checks the database health
func (db *DB) Health(ctx context.Context) error {
	return db.PingContext(ctx)
}

// VerifyWritable confirms the connection can execute writes against the
// users table. Run at startup so a misconfigured DB_HOST that resolves to
// a reader replica or a read-only role fails the task before it joins the
// ALB pool, instead of silently failing every auth request in production.
//
// The probe runs inside a transaction and rolls back, so it leaves no
// residue even if the WHERE clause ever matched a real row. The
// `id = -1` predicate is impossible (id is a positive serial), so this
// is a zero-row UPDATE that still forces Postgres to evaluate write
// permission and surface `cannot execute UPDATE in a read-only transaction`.
func (db *DB) VerifyWritable(ctx context.Context) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin write-probe tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`UPDATE users SET last_logged_on = last_logged_on WHERE id = -1`,
	); err != nil {
		return fmt.Errorf("write probe failed (DB_HOST may point at a reader): %w", err)
	}
	return nil
}
