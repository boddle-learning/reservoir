# Boddle Reservoir - Go Authentication Gateway

A high-performance authentication gateway built in Go that centralizes authentication for Boddle LMS and game clients, replacing cookie-based sessions with JWT tokens.

## Features

- **Multiple Authentication Methods**:
  - Email/Password (bcrypt)
  - Google OAuth2
  - Clever SSO (K-12 education platform)
  - iCloud Sign In
  - Login Tokens (magic links)

- **JWT Token-Based Authentication**:
  - Access tokens (6-hour TTL)
  - Refresh tokens (30-day TTL)
  - Token revocation (blacklist)

- **Security**:
  - Rate limiting (5 attempts per 10 minutes, 15-minute lockout)
  - Request validation and sanitization
  - Security headers (XSS, clickjacking, MIME sniffing protection)
  - bcrypt password hashing
  - Token blacklisting via Redis
  - CORS configuration

- **Observability**:
  - Prometheus metrics (HTTP requests, auth attempts, JWT validations)
  - Structured logging with Zap
  - Request/response logging
  - Performance metrics

- **High Performance**:
  - Built with Go and Gin framework
  - PostgreSQL connection pooling
  - Redis caching for rate limiting and blacklist
  - Horizontal scaling support
  - Sub-second authentication

## Quick Start

### Prerequisites

- Go 1.22+
- Docker and Docker Compose
- PostgreSQL 15+
- Redis 7+

### Development Setup

1. Clone the repository:
```bash
cd /Users/stjohncj/dev/boddle/reservoir
```

2. Copy environment variables:
```bash
cp .env.example .env
# Edit .env with your configuration
```

3. Start services with Docker Compose:
```bash
docker-compose up -d
```

4. Run the gateway:
```bash
go run cmd/server/main.go
```

The gateway will start on `http://localhost:8080`.

### Using Docker Compose Only

```bash
docker-compose up --build
```

This will start:
- Auth Gateway on port 8080
- PostgreSQL on port 5432
- Redis on port 6379
- Adminer (DB UI) on port 8081
- Redis Commander on port 8082

## API Endpoints

### Public Endpoints

- `GET /health` - Health check
- `GET /metrics` - Prometheus metrics
- `POST /auth/login` - Email/password login
- `GET /auth/token?token=SECRET` - Login token authentication
- `GET /auth/google?redirect_url=...` - Initiate Google OAuth flow
- `GET /auth/google/callback` - Google OAuth callback
- `GET /auth/clever?redirect_url=...` - Initiate Clever SSO flow
- `GET /auth/clever/callback` - Clever SSO callback
- `GET /auth/icloud?redirect_url=...` - Initiate iCloud Sign In flow
- `POST /auth/icloud/callback` - iCloud Sign In callback (Apple uses form_post)
- `POST /auth/logout` - Logout (revoke token)

### Protected Endpoints

Require `Authorization: Bearer <JWT>` header:

- `GET /auth/me` - Get current user information

### Examples

**Login with email/password:**
```bash
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "teacher@example.com",
    "password": "password123"
  }'
```

**Login with Google OAuth:**
```bash
# Step 1: Get OAuth URL
curl "http://localhost:8080/auth/google?redirect_url=/dashboard"
# User is redirected to Google for authentication

# Step 2: Google redirects to callback with code
# GET /auth/google/callback?code=...&state=...
# Returns JWT token
```

**Login with Clever SSO:**
```bash
# Step 1: Get Clever OAuth URL
curl "http://localhost:8080/auth/clever?redirect_url=/dashboard"
# User is redirected to Clever for authentication

# Step 2: Clever redirects to callback with code
# GET /auth/clever/callback?code=...&state=...
# Returns JWT token
```

**Login with iCloud Sign In:**
```bash
# Step 1: Get iCloud Sign In URL
curl "http://localhost:8080/auth/icloud?redirect_url=/dashboard"
# User is redirected to Apple for authentication

# Step 2: Apple redirects to callback with code (as form_post)
# POST /auth/icloud/callback (code and state in form data)
# Returns JWT token
```

**Get current user:**
```bash
curl -X GET http://localhost:8080/auth/me \
  -H "Authorization: Bearer <your-jwt-token>"
```

**Logout:**
```bash
curl -X POST http://localhost:8080/auth/logout \
  -H "Authorization: Bearer <your-jwt-token>"
```

## Project Structure

```
reservoir/
├── cmd/
│   └── server/
│       └── main.go           # Application entry point
├── internal/
│   ├── auth/                 # Authentication logic
│   │   ├── handler.go        # HTTP handlers
│   │   ├── service.go        # Business logic
│   │   └── password.go       # Password verification
│   ├── token/                # JWT token management
│   │   ├── jwt.go            # JWT generation/validation
│   │   ├── claims.go         # JWT claims structure
│   │   └── blacklist.go      # Token revocation
│   ├── user/                 # User data access
│   │   ├── model.go          # Data models
│   │   └── repository.go     # Database operations
│   ├── database/             # Database connections
│   │   ├── postgres.go       # PostgreSQL client
│   │   └── redis.go          # Redis client
│   ├── config/               # Configuration management
│   │   └── config.go         # Config loading
│   └── middleware/           # HTTP middleware
│       ├── cors.go           # CORS headers
│       ├── logger.go         # Request logging
│       ├── recovery.go       # Panic recovery
│       └── auth.go           # JWT validation
├── pkg/
│   ├── errors/               # Custom error types
│   └── response/             # API response helpers
├── tests/
│   ├── integration/          # Integration tests
│   └── mocks/                # Test mocks
├── .env.example              # Environment variables template
├── Dockerfile                # Docker image definition
├── docker-compose.yml        # Docker Compose configuration
└── README.md                 # This file
```

## Configuration

All configuration is done via environment variables. See `.env.example` for available options.

### Required Variables

- `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME` - PostgreSQL connection
- `REDIS_URL` - Redis connection string
- `JWT_SECRET_KEY` - JWT signing secret (minimum 32 characters)
- `JWT_REFRESH_SECRET_KEY` - Refresh token signing secret

### Optional Variables

- `PORT` - Server port (default: 8080)
- `ENV` - Environment (development/production)
- `CORS_ALLOWED_ORIGINS` - Comma-separated allowed origins
- OAuth credentials for Google, Clever, iCloud

## Database Schema

The gateway connects directly to the Rails LMS PostgreSQL database and uses the following tables:

- `users` - Base user table (polymorphic)
- `teachers` - Teacher-specific data
- `students` - Student-specific data
- `parents` - Parent-specific data
- `login_attempts` - Rate limiting data
- `login_tokens` - Magic link tokens

## Testing

```bash
# Run unit tests
go test ./... -v

# Run with coverage
go test ./... -cover

# Run integration tests
go test ./tests/integration/... -v
```

## Building for Production

```bash
# Build binary
go build -o reservoir cmd/server/main.go

# Build Docker image
docker build -t boddle/reservoir:latest .
```

## Implementation Status

### ✅ Phase 1 (Completed)
- Project structure initialized
- Configuration management
- Database connections (PostgreSQL + Redis)
- User models and repository
- JWT token service with blacklist
- Password verification (bcrypt)
- Authentication service (email/password + login tokens)
- HTTP handlers and middleware
- Docker and Docker Compose setup

### ✅ Phase 2 (Completed)
- Rate limiting service (Redis-backed)
- Request validation and sanitization
- Security headers middleware
- Prometheus metrics
- Unit tests for core components
- Integrated rate limiting into auth flow

### ✅ Phase 3 (Completed)
- Google OAuth2 integration
- OAuth state management with Redis (CSRF protection)
- Account linking by email and Google UID
- Automatic Google UID updates for existing users
- OAuth callback handling with JWT issuance

### ✅ Phase 4 (Completed)
- Clever SSO integration
- Clever OAuth2 flow implementation
- Account linking by email and Clever UID
- Automatic Clever UID updates for existing users
- Support for teacher and student accounts via Clever

### ✅ Phase 5 (Completed)
- iCloud Sign In integration
- Apple OAuth2 flow with JWT-signed client secret
- ECDSA private key loading and management
- ID token parsing and validation
- Account linking by email and iCloud UID
- Automatic iCloud UID updates for existing users
- Support for Apple "Hide My Email" feature
- Support for students and parents primarily
- form_post response mode for enhanced security

### ✅ Phase 6 (Completed)
- Login Tokens (Magic Links) fully functional
- Support for permanent tokens (game links)
- 5-minute expiry for non-permanent tokens
- Automatic token deletion after use
- Backward compatible with Rails-generated tokens

### ✅ Phase 7 (Completed - Ready for Deployment)
- Rails JWT validation middleware implemented
- ApplicationController helpers for JWT/session dual authentication
- Configuration initializer for Rails
- Comprehensive migration guide with rollout strategy
- Load testing script (k6) for performance validation
- Monitoring and alerting guidelines
- Troubleshooting documentation
- Security considerations documented
- Rollback plan included

**What's Ready:**
- All code files for Rails integration (see `docs/rails/`)
- Migration guide: `docs/RAILS_MIGRATION_GUIDE.md`
- Load testing script: `tests/load-test.js`

**Deployment Checklist:**
- [ ] Deploy Go Authentication Gateway to production
- [ ] Configure environment variables (JWT_SECRET_KEY, REDIS_URL)
- [ ] Install JWT gem in Rails: `gem 'jwt', '~> 2.7'`
- [ ] Copy Rails middleware and helpers from `docs/rails/`
- [ ] Configure Rails initializer
- [ ] Run load tests to validate performance
- [ ] Enable JWT_FALLBACK_TO_SESSION=true for gradual rollout
- [ ] Monitor metrics and error rates
- [ ] Gradually increase JWT_ROLLOUT_PERCENTAGE (0% → 25% → 50% → 75% → 100%)
- [ ] Set JWT_FALLBACK_TO_SESSION=false after full migration
- [ ] Remove legacy session code

### 🎉 Project Status: Implementation Complete!

All 7 phases of the Go Authentication Gateway are now complete. The system is ready for deployment and Rails integration.

## Documentation

See the `docs/` directory for detailed documentation:

- Architecture diagrams
- API specifications
- Deployment guides
- Migration strategy from Rails

## License

Copyright © 2024 Boddle Learning Inc.
