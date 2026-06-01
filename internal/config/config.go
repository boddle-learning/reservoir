package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Config holds all application configuration
type Config struct {
	// Server configuration
	Port string `envconfig:"PORT" default:"8080"`
	Env  string `envconfig:"ENV" default:"development"`

	// TrustedProxies is a comma-separated list of proxy IPs/CIDRs (the ALB)
	// that Gin should trust when deriving the client IP from X-Forwarded-For.
	// EMPTY MEANS TRUST NONE: c.ClientIP() then returns the direct peer and a
	// client-supplied X-Forwarded-For is ignored. Leaving it empty is the safe
	// default — it prevents the rate-limit bypass in Finding 4 / LMS-6515,
	// though behind an ALB it collapses all clients to the ALB's IP (the
	// email-keyed limiter still distinguishes accounts). Set it to the ALB's
	// CIDR(s) in production to recover true per-client IPs.
	TrustedProxies string `envconfig:"TRUSTED_PROXIES"`

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

	// New Relic APM configuration
	NewRelic NewRelicConfig
}

// DatabaseConfig holds PostgreSQL configuration
type DatabaseConfig struct {
	Host               string `envconfig:"DB_HOST" required:"true"`
	ReaderHost         string `envconfig:"DB_READER_HOST"`                    // optional; falls back to DB_HOST when unset
	Port               int    `envconfig:"DB_PORT" default:"5432"`
	User               string `envconfig:"DB_USER" required:"true"`
	Password           string `envconfig:"DB_PASSWORD" required:"true"`
	Name               string `envconfig:"DB_NAME" required:"true"`
	SSLMode            string `envconfig:"DB_SSL_MODE" default:"require"`
	MaxOpenConns       int    `envconfig:"DB_MAX_OPEN_CONNS" default:"25"`        // floor(r7g.8xlarge_max_connections * 0.8 / max_tasks); override per env in SSM
	ReaderMaxOpenConns int    `envconfig:"DB_READER_MAX_OPEN_CONNS" default:"11"` // floor(serverless_v2_min_acus_max_connections * 0.8 / max_tasks); override per env in SSM
}

// ConnectionString returns the writer PostgreSQL connection string.
func (d DatabaseConfig) ConnectionString() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.Name, d.SSLMode,
	)
}

// ReaderConnectionString returns the read-replica connection string.
// Falls back to the writer host when DB_READER_HOST is not set.
func (d DatabaseConfig) ReaderConnectionString() string {
	host := d.ReaderHost
	if host == "" {
		host = d.Host
	}
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		host, d.Port, d.User, d.Password, d.Name, d.SSLMode,
	)
}

// HasReader reports whether a dedicated read-replica host is configured.
func (d DatabaseConfig) HasReader() bool {
	return d.ReaderHost != ""
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

	// TokenAudiences is the comma-separated allowlist of Google OAuth client
	// IDs that may present access tokens to POST /auth/google (i.e. the LMS's
	// own OmniAuth client ID(s), which differ from ClientID above). When set,
	// the token's aud/azp is verified against this list via Google's tokeninfo
	// endpoint, preventing a confused-deputy replay of a token minted for an
	// unrelated OAuth app. Empty disables the check. See LMS-6511 follow-up.
	TokenAudiences string `envconfig:"GOOGLE_TOKEN_AUDIENCES"`
}

// CleverConfig holds Clever SSO configuration
type CleverConfig struct {
	ClientID     string `envconfig:"CLEVER_CLIENT_ID" required:"true"`
	ClientSecret string `envconfig:"CLEVER_CLIENT_SECRET" required:"true"`
	RedirectURL  string `envconfig:"CLEVER_REDIRECT_URL" required:"true"`
}

// ICloudConfig holds Apple "Sign in with Apple" (iCloud) configuration.
type ICloudConfig struct {
	// ClientIDs is the comma-separated allowlist of Apple client IDs (the iOS
	// app bundle ID and/or web service ID) that an ID token's aud must match.
	// Empty leaves POST /auth/icloud failing closed: it cannot verify a token's
	// audience, so it rejects every request. Set this in production.
	ClientIDs string `envconfig:"APPLE_CLIENT_IDS"`
}

// CORSConfig holds CORS configuration
type CORSConfig struct {
	AllowedOrigins string `envconfig:"CORS_ALLOWED_ORIGINS" default:"*"`
}

// RateLimitConfig holds rate limiting configuration.
//
// Two dimensions are tracked per login: a tight per-(ip,email) bucket and a
// wider per-email backstop. The email backstop is what defends against an
// attacker rotating source IPs (e.g. via X-Forwarded-For) to brute-force a
// single account — see Finding 4 / LMS-6515.
type RateLimitConfig struct {
	Window          time.Duration `envconfig:"RATE_LIMIT_WINDOW" default:"10m"`
	MaxAttempts     int           `envconfig:"RATE_LIMIT_MAX_ATTEMPTS" default:"5"`
	LockoutDuration time.Duration `envconfig:"RATE_LIMIT_LOCKOUT_DURATION" default:"15m"`

	// Email-only backstop: wider window and a higher threshold (a single user
	// may legitimately fumble a password across devices/networks).
	EmailWindow      time.Duration `envconfig:"RATE_LIMIT_EMAIL_WINDOW" default:"1h"`
	EmailMaxAttempts int           `envconfig:"RATE_LIMIT_EMAIL_MAX_ATTEMPTS" default:"20"`
}

// NewRelicConfig holds New Relic APM configuration. Empty LicenseKey leaves
// the agent disabled — the service still boots, nrgin/nrpq integrations
// become no-ops. Wired in response to PIR 2026-05-19, where the absence of
// APM let a per-request DB write failure go unobserved for ~31 hours.
type NewRelicConfig struct {
	LicenseKey string `envconfig:"NEW_RELIC_LICENSE_KEY"`
	AppName    string `envconfig:"NEW_RELIC_APP_NAME" default:"reservoir"`
}

// Enabled reports whether the agent should connect to New Relic. The
// agent is enabled only when a license key is provided.
func (n NewRelicConfig) Enabled() bool {
	return n.LicenseKey != ""
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

// TrustedProxyList parses TRUSTED_PROXIES into trimmed, non-empty entries.
// An empty result is passed to router.SetTrustedProxies as nil, which makes
// Gin trust no proxies and ignore client-supplied X-Forwarded-For headers.
func (c *Config) TrustedProxyList() []string {
	var out []string
	for _, p := range strings.Split(c.TrustedProxies, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
