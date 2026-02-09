package config

import (
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Config holds all application configuration
type Config struct {
	// Server configuration
	Port string `envconfig:"PORT" default:"8080"`
	Env  string `envconfig:"ENV" default:"development"`

	// Database configuration
	Database DatabaseConfig

	// Redis configuration
	RedisURL string `envconfig:"REDIS_URL" required:"true"`

	// JWT configuration
	JWT JWTConfig

	// OAuth configuration
	Google GoogleConfig
	Clever CleverConfig
	ICloud ICloudConfig

	// CORS configuration
	CORS CORSConfig

	// Rate limiting configuration
	RateLimit RateLimitConfig
}

// DatabaseConfig holds PostgreSQL configuration
type DatabaseConfig struct {
	Host     string `envconfig:"DB_HOST" required:"true"`
	Port     int    `envconfig:"DB_PORT" default:"5432"`
	User     string `envconfig:"DB_USER" required:"true"`
	Password string `envconfig:"DB_PASSWORD" required:"true"`
	Name     string `envconfig:"DB_NAME" required:"true"`
	SSLMode  string `envconfig:"DB_SSL_MODE" default:"require"`
}

// ConnectionString returns the PostgreSQL connection string
func (d DatabaseConfig) ConnectionString() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.Name, d.SSLMode,
	)
}

// JWTConfig holds JWT token configuration
type JWTConfig struct {
	SecretKey        string        `envconfig:"JWT_SECRET_KEY" required:"true"`
	RefreshSecretKey string        `envconfig:"JWT_REFRESH_SECRET_KEY" required:"true"`
	AccessTokenTTL   time.Duration `envconfig:"JWT_ACCESS_TOKEN_TTL" default:"6h"`
	RefreshTokenTTL  time.Duration `envconfig:"JWT_REFRESH_TOKEN_TTL" default:"720h"`
}

// GoogleConfig holds Google OAuth2 configuration
type GoogleConfig struct {
	ClientID     string `envconfig:"GOOGLE_CLIENT_ID" required:"true"`
	ClientSecret string `envconfig:"GOOGLE_CLIENT_SECRET" required:"true"`
	RedirectURL  string `envconfig:"GOOGLE_REDIRECT_URL" required:"true"`
}

// CleverConfig holds Clever SSO configuration
type CleverConfig struct {
	ClientID     string `envconfig:"CLEVER_CLIENT_ID" required:"true"`
	ClientSecret string `envconfig:"CLEVER_CLIENT_SECRET" required:"true"`
	RedirectURL  string `envconfig:"CLEVER_REDIRECT_URL" required:"true"`
}

// ICloudConfig holds iCloud Sign In configuration
type ICloudConfig struct {
	ServiceID      string `envconfig:"ICLOUD_SERVICE_ID" required:"true"`
	TeamID         string `envconfig:"ICLOUD_TEAM_ID" required:"true"`
	KeyID          string `envconfig:"ICLOUD_KEY_ID" required:"true"`
	PrivateKeyPath string `envconfig:"ICLOUD_PRIVATE_KEY_PATH" required:"true"`
	RedirectURL    string `envconfig:"ICLOUD_REDIRECT_URL" required:"true"`
}

// CORSConfig holds CORS configuration
type CORSConfig struct {
	AllowedOrigins string `envconfig:"CORS_ALLOWED_ORIGINS" default:"*"`
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	Window          time.Duration `envconfig:"RATE_LIMIT_WINDOW" default:"10m"`
	MaxAttempts     int           `envconfig:"RATE_LIMIT_MAX_ATTEMPTS" default:"5"`
	LockoutDuration time.Duration `envconfig:"RATE_LIMIT_LOCKOUT_DURATION" default:"15m"`
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}
	return &cfg, nil
}

// IsDevelopment returns true if running in development environment
func (c *Config) IsDevelopment() bool {
	return c.Env == "development"
}

// IsProduction returns true if running in production environment
func (c *Config) IsProduction() bool {
	return c.Env == "production"
}
