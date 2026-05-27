package database

import (
	"context"
	"fmt"
	"time"

	"github.com/boddle/reservoir/internal/config"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq" // PostgreSQL driver
	// nrpq registers the "nrpostgres" driver, a lib/pq wrapper that emits
	// New Relic datastore segments for each query when a transaction is in
	// the request context. No-op when the New Relic agent is disabled.
	_ "github.com/newrelic/go-agent/v3/integrations/nrpq"
)

// DB wraps the sqlx database connection
type DB struct {
	*sqlx.DB
}

// NewPostgresDB creates a new PostgreSQL database connection.
// Uses the "nrpostgres" driver (a lib/pq wrapper from nrpq) so each query
// becomes a datastore segment in the surrounding New Relic transaction.
// When the agent is disabled, the wrapper degrades to a no-op delegate.
func NewPostgresDB(cfg config.DatabaseConfig) (*DB, error) {
	connStr := cfg.ConnectionString()

	db, err := sqlx.Connect("nrpostgres", connStr)
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
