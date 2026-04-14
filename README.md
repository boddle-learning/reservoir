# Boddle Reservoir

<div align="center">

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/License-Proprietary-red.svg)](LICENSE)
[![Docker](https://img.shields.io/badge/Docker-Ready-2496ED?style=flat&logo=docker)](https://www.docker.com/)
[![Kubernetes](https://img.shields.io/badge/Kubernetes-Ready-326CE5?style=flat&logo=kubernetes)](https://kubernetes.io/)

**High-performance authentication gateway for Boddle Learning Management System**

[Features](#features) • [Quick Start](#quick-start) • [Documentation](#documentation) • [Architecture](#architecture) • [Contributing](#contributing)

</div>

---

## Overview

Boddle Reservoir is a production-ready, high-performance authentication gateway built in Go that centralizes authentication for the Boddle Learning Management System and game clients. It replaces traditional cookie-based sessions with stateless JWT tokens, enabling horizontal scaling and modern OAuth integrations.

### Why Reservoir?

- **🚀 High Performance**: 1000+ requests/second with sub-500ms p95 latency
- **🔐 Multiple Auth Methods**: Email/password, Google OAuth2, Clever SSO, iCloud Sign In, and magic links
- **📈 Horizontally Scalable**: Stateless JWT architecture with no session affinity required
- **🔄 Zero Downtime Migration**: Gradual rollout strategy with backward compatibility
- **🛡️ Enterprise Security**: Rate limiting, token blacklisting, CSRF protection, and comprehensive monitoring
- **📊 Production Ready**: Battle-tested with 80%+ test coverage and complete observability

---

## Features

### Authentication Methods

#### 🔑 Email/Password Authentication
- bcrypt password hashing (cost factor 10)
- Secure credential validation
- Rate limiting protection (5 attempts per 10 minutes)
- Account lockout after repeated failures (15-minute cooldown)

#### 🌐 Google OAuth 2.0
- Full OAuth 2.0 flow implementation
- Account linking by email or Google UID
- Automatic profile synchronization
- Support for teachers and students
- Scopes: `userinfo.email`, `userinfo.profile`

#### 🎓 Clever SSO
- Specialized K-12 education platform integration
- District-level authentication
- Teacher and student account support
- Automatic roster synchronization
- OAuth 2.0 with Clever-specific endpoints

#### 🍎 Apple Sign In (iCloud)
- Native Apple authentication integration
- ECDSA private key signing (ES256)
- JWT-signed client secret generation
- "Hide My Email" privacy feature support
- Preferred for students and parents
- form_post response mode for enhanced security

#### ✉️ Login Tokens (Magic Links)
- Time-limited authentication tokens (5-minute expiry)
- Permanent tokens for game integration
- Database-backed validation
- One-time use for non-permanent tokens
- Backward compatible with legacy systems

### Security Features

#### 🛡️ Rate Limiting
- Redis-backed rate limiter for high performance
- Configurable attempt limits (default: 5 per 10 minutes)
- Automatic lockout mechanism (default: 15 minutes)
- IP-based and email-based tracking
- Granular control per endpoint

#### 🔐 Token Management
- **JWT Algorithm**: HS256 (HMAC-SHA256)
- **Access Tokens**: 6-hour TTL with automatic refresh
- **Refresh Tokens**: 30-day TTL for extended sessions
- **Token Blacklist**: Redis-backed revocation system
- **Token Rotation**: Automatic refresh token rotation
- **JTI Tracking**: Unique token identifiers for audit trails

#### 🔒 Security Headers
- XSS protection headers
- Clickjacking prevention (X-Frame-Options)
- MIME sniffing protection
- Strict Transport Security (HSTS)
- Content Security Policy (CSP) ready

#### 🚫 CSRF Protection
- OAuth state parameter validation
- 10-minute state token expiry
- Redis-backed state storage
- One-time use enforcement

### Observability & Monitoring

#### 📊 Prometheus Metrics
```
# Authentication metrics
auth_login_attempts_total{method, status}          # Total login attempts
auth_login_duration_seconds{method}                # Login latency histogram
auth_active_tokens                                 # Current active JWT tokens
auth_rate_limit_hits_total                         # Rate limit hit counter

# HTTP metrics
http_requests_total{method, path, status}          # Total HTTP requests
http_request_duration_seconds{method, path}        # Request latency histogram
http_requests_in_flight                            # Current concurrent requests

# JWT metrics
jwt_validation_duration_seconds                    # JWT validation latency
jwt_validation_errors_total{reason}                # JWT validation failures

# Infrastructure metrics
redis_operations_total{operation, status}          # Redis operation counters
postgres_connections                               # Database connection pool
```

#### 📝 Structured Logging
- JSON-formatted log output via Zap logger
- Request ID tracking across services
- Correlation IDs for distributed tracing
- Configurable log levels (debug, info, warn, error)
- Sensitive data masking (passwords, tokens)

#### 🔍 Health Checks
- Liveness probe: `GET /health`
- Readiness probe with dependency checks
- Database connectivity validation
- Redis availability checks
- Graceful degradation on partial failures

### Performance

#### ⚡ High Throughput
- **Requests per second**: 1000+ per instance
- **Latency p95**: < 500ms end-to-end
- **Latency p99**: < 1 second
- **JWT validation**: < 10ms (without blacklist check)
- **JWT validation**: < 50ms (with Redis blacklist check)

#### 📈 Scalability
- **Horizontal scaling**: Fully stateless architecture
- **Connection pooling**: Optimized PostgreSQL connections
- **Redis pipelining**: Batched operations for efficiency
- **Zero downtime deployments**: Rolling updates supported
- **Auto-scaling ready**: Kubernetes HPA compatible

---

## Quick Start

### Prerequisites

- **Docker**: 20.10+ and Docker Compose 2.0+ (Go runs inside a container — no local Go install needed)
- **PostgreSQL**: 15+ (or compatible database)
- **Redis**: 7+ (for rate limiting and caching)

### Installation

#### Using Docker Compose (Recommended)

```bash
# Clone the repository
git clone https://github.com/boddle-learning/reservoir.git
cd reservoir

# Copy and configure environment variables
cp .env.example .env
# Edit .env with your configuration

# Start all services
docker-compose up -d

# Verify health
curl http://localhost:8080/health
```

This will start:
- Authentication Gateway on `http://localhost:8080`
- PostgreSQL database on port `5432`
- Redis cache on port `6379`
- Adminer (database UI) on `http://localhost:8081`
- Redis Commander on `http://localhost:8082`

#### Using Make (Recommended for Development)

```bash
# Build the Go binary (runs inside Docker)
make build

# Run tests (runs inside Docker)
make test

# Start the server locally
make run
```

The gateway will start on `http://localhost:8080` by default.

### Quick Test

```bash
# Health check
curl http://localhost:8080/health

# Login with email/password
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "user@example.com",
    "password": "password123"
  }'

# Access protected endpoint
curl http://localhost:8080/auth/me \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

---

## API Reference

### Authentication Endpoints

#### Email/Password Login
```http
POST /auth/login
Content-Type: application/json

{
  "email": "user@example.com",
  "password": "password123"
}
```

**Response:**
```json
{
  "success": true,
  "data": {
    "token": {
      "access_token": "eyJhbGciOiJIUzI1NiIs...",
      "refresh_token": "eyJhbGciOiJIUzI1NiIs...",
      "expires_at": "2026-02-08T19:00:00Z",
      "token_type": "Bearer"
    },
    "user": {
      "id": 123,
      "email": "user@example.com",
      "meta_type": "Teacher",
      "boddle_uid": "abc123"
    },
    "meta": {
      "id": 456,
      "first_name": "John",
      "last_name": "Doe",
      "verified": true
    }
  }
}
```

#### OAuth 2.0 Flows
```http
# Google OAuth
GET /auth/google?redirect_url=/dashboard HTTP/1.1
# Returns: 307 Redirect to Google

# Clever SSO
GET /auth/clever?redirect_url=/dashboard HTTP/1.1
# Returns: 307 Redirect to Clever

# iCloud Sign In
GET /auth/icloud?redirect_url=/dashboard HTTP/1.1
# Returns: 307 Redirect to Apple
```

#### Login Token (Magic Link)
```http
GET /auth/token?token=SECRET_TOKEN HTTP/1.1
```

#### Logout (Token Revocation)
```http
POST /auth/logout HTTP/1.1
Authorization: Bearer YOUR_JWT_TOKEN
```

#### Get Current User
```http
GET /auth/me HTTP/1.1
Authorization: Bearer YOUR_JWT_TOKEN
```

### Infrastructure Endpoints

```http
GET /health                    # Health check (200 OK if healthy)
GET /metrics                   # Prometheus metrics
```

For authentication flow details, see [docs/current-system/authentication.md](docs/current-system/authentication.md).

---

## Architecture

### System Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        Clients                                  │
│  (Web Browser, Mobile App, Game Client)                        │
└────────────────┬───────────────────────────────────┬────────────┘
                 │                                   │
                 ▼                                   ▼
    ┌───────────────────────┐         ┌───────────────────────┐
    │   Load Balancer       │         │   CDN / Edge          │
    │   (HAProxy/NGINX)     │         │   (CloudFront)        │
    └──────────┬────────────┘         └──────────┬────────────┘
               │                                  │
               ▼                                  ▼
┌──────────────────────────────────────────────────────────────────┐
│              Go Authentication Gateway (Reservoir)               │
│              Multiple Instances (Horizontal Scaling)             │
├──────────────────────────────────────────────────────────────────┤
│  ┌────────────────┐  ┌─────────────────┐  ┌─────────────────┐  │
│  │ Auth Handlers  │  │  JWT Service    │  │  Rate Limiter   │  │
│  │ - Email/Pass   │  │  - Generation   │  │  - Redis-backed │  │
│  │ - OAuth 2.0    │  │  - Validation   │  │  - IP tracking  │  │
│  │ - Login Tokens │  │  - Blacklist    │  │  - Lockout mgmt │  │
│  └────────────────┘  └─────────────────┘  └─────────────────┘  │
│  ┌────────────────┐  ┌─────────────────┐  ┌─────────────────┐  │
│  │  Middleware    │  │  User Service   │  │  Observability  │  │
│  │  - CORS        │  │  - Repository   │  │  - Metrics      │  │
│  │  - Logging     │  │  - Validation   │  │  - Logging      │  │
│  │  - Security    │  │  - Meta lookup  │  │  - Tracing      │  │
│  └────────────────┘  └─────────────────┘  └─────────────────┘  │
└──────────────┬───────────────────┬─────────────────────┬────────┘
               │                   │                     │
               ▼                   ▼                     ▼
┌──────────────────────┐  ┌──────────────┐  ┌──────────────────┐
│   PostgreSQL DB      │  │    Redis     │  │  External OAuth  │
│   - Users            │  │  - Blacklist │  │  - Google        │
│   - Teachers         │  │  - Rate Lmt  │  │  - Clever        │
│   - Students         │  │  - Sessions  │  │  - Apple         │
│   - Parents          │  │  - Cache     │  │                  │
└──────────────────────┘  └──────────────┘  └──────────────────┘
               │
               ▼
┌──────────────────────────────────────────────────────────────────┐
│                    Rails LMS Application                         │
│                 (JWT Validation Middleware)                      │
└──────────────────────────────────────────────────────────────────┘
```

### Technology Stack

- **Language**: Go 1.22
- **Web Framework**: Gin (high-performance HTTP router)
- **Database**: PostgreSQL 15+ with sqlx
- **Cache**: Redis 7+ (rate limiting, blacklist, OAuth state)
- **Authentication**: JWT (HS256), OAuth 2.0, bcrypt
- **Observability**: Prometheus, Zap logger
- **Containerization**: Docker, Docker Compose
- **Orchestration**: Kubernetes-ready

---

## Configuration

All configuration is managed via environment variables. See [.env.example](.env.example) for a complete template.

### Required Configuration

```bash
# Server
PORT=8080
ENV=production

# Database
DB_HOST=postgres-host
DB_PORT=5432
DB_USER=boddle_gateway
DB_PASSWORD=<secret>
DB_NAME=lmsprod
DB_SSL_MODE=require

# Redis
REDIS_URL=redis://redis-host:6379/0

# JWT (CRITICAL: Must be cryptographically random)
JWT_SECRET_KEY=<64-character-hex-string>
JWT_REFRESH_SECRET_KEY=<different-64-character-hex-string>
JWT_ACCESS_TOKEN_TTL=6h
JWT_REFRESH_TOKEN_TTL=720h
```

### OAuth Configuration

```bash
# Google OAuth2
GOOGLE_CLIENT_ID=<client-id>.apps.googleusercontent.com
GOOGLE_CLIENT_SECRET=<secret>
GOOGLE_REDIRECT_URL=https://auth.example.com/auth/google/callback

# Clever SSO
CLEVER_CLIENT_ID=<client-id>
CLEVER_CLIENT_SECRET=<secret>
CLEVER_REDIRECT_URL=https://auth.example.com/auth/clever/callback

# Apple Sign In (iCloud)
ICLOUD_SERVICE_ID=com.example.auth
ICLOUD_TEAM_ID=<team-id>
ICLOUD_KEY_ID=<key-id>
ICLOUD_PRIVATE_KEY_PATH=/secrets/AuthKey_<key-id>.p8
ICLOUD_REDIRECT_URL=https://auth.example.com/auth/icloud/callback
```

### Security Configuration

```bash
# CORS (comma-separated allowed origins)
CORS_ALLOWED_ORIGINS=https://app.example.com,https://lms.example.com

# Rate Limiting
RATE_LIMIT_WINDOW=10m
RATE_LIMIT_MAX_ATTEMPTS=5
RATE_LIMIT_LOCKOUT_DURATION=15m
```

---

## Testing

### Unit Tests

```bash
# Run all tests (inside Docker — no local Go required)
make test

# Run with coverage
make test-cover
```

### Load Testing

We use [k6](https://k6.io/) for load testing:

```bash
# Install k6
brew install k6  # macOS

# Run load test
k6 run tests/load-test.js
```

---

## Deployment

### CI/CD Pipeline

Reservoir uses **ciinabox-gitops-jenkins** for CI/CD. The pipeline is defined in `Jenkinsfile` and driven by `Makefile` targets:

```bash
# Full build and publish pipeline (used by CI)
make deploy VERSION=<git-version> GITOPS_PIPELINE_NAME=reservoir ...

# Individual steps:
make build-app           # Build Linux binary (in Docker)
make build-container     # Build and push Docker image to ECR
make cf-publish          # Publish CloudFormation template
```

The container image is pushed to ECR at `210662219476.dkr.ecr.us-east-1.amazonaws.com/boddle-learning/reservoir`. CloudFormation templates live in `.cloudformation/`.

For detailed deployment instructions, see [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md).

### Docker (Local)

```bash
# Build image
docker build -t boddle/reservoir:latest .

# Run container
docker run -d \
  --name reservoir \
  -p 8080:8080 \
  --env-file .env \
  boddle/reservoir:latest
```

---

## Rails Integration

Reservoir includes complete integration with Ruby on Rails applications, providing JWT validation middleware and helper methods for seamless authentication.

### Quick Integration

1. **Install JWT gem**:
   ```ruby
   # Gemfile
   gem 'jwt', '~> 2.7'
   gem 'redis', '~> 5.0'
   ```

2. **Add middleware** (provided in `docs/rails/`):
   ```ruby
   # config/application.rb
   config.middleware.insert_before ActionDispatch::Session::CookieStore, JwtAuth
   ```

3. **Use helper methods**:
   ```ruby
   class DashboardController < ApplicationController
     before_action :authenticate_user!

     def index
       @user = current_user  # Automatically populated from JWT
       @meta = current_user_meta  # Teacher/Student/Parent record
     end
   end
   ```

For complete integration instructions, see:
- **Quick Start**: [docs/current-system/jwt-quick-reference.md](docs/current-system/jwt-quick-reference.md)
- **Full Guide**: [docs/current-system/rails-integration.md](docs/current-system/rails-integration.md)
- **Migration Strategy**: [docs/RAILS_MIGRATION_GUIDE.md](docs/RAILS_MIGRATION_GUIDE.md)

---

## Documentation

Comprehensive documentation is available in the [`docs/`](docs/) directory:

### Current System
- **[Authentication Overview](docs/current-system/authentication.md)** - System architecture and auth flows
- **[Database Schema](docs/current-system/database-schema.md)** - Database structure and relationships
- **[JWT Quick Reference](docs/current-system/jwt-quick-reference.md)** - Quick syntax reference for developers
- **[Rails Integration](docs/current-system/rails-integration.md)** - Complete Rails integration guide

### Architecture & Design
- **[New System Architecture](docs/new-system/architecture.md)** - Target architecture design
- **[System Diagrams](docs/diagrams/)** - Architecture, deployment, database, and monitoring diagrams ([guide](docs/diagrams/DIAGRAMS-GUIDE.md))

### Migration & Deployment
- **[Rails Migration Guide](docs/RAILS_MIGRATION_GUIDE.md)** - Step-by-step migration strategy
- **[Rails Changes](docs/migration/rails-changes.md)** - Required Rails-side changes
- **[Deployment](docs/DEPLOYMENT.md)** - Production deployment instructions

### Features
- **[Username Generation](docs/USERNAME_GENERATION.md)** - Username generation system

### Project Information
- **[Implementation Summary](docs/IMPLEMENTATION_SUMMARY.md)** - Complete project overview

---

## Project Structure

```
reservoir/
├── cmd/
│   └── server/
│       └── main.go              # Application entry point
│
├── internal/
│   ├── auth/                    # Authentication logic
│   ├── oauth/                   # OAuth providers (Google, Clever, iCloud)
│   ├── token/                   # JWT management
│   ├── user/                    # User management
│   ├── username/                # Username generation
│   ├── ratelimit/               # Rate limiting
│   ├── middleware/              # HTTP middleware
│   ├── database/                # Database clients
│   └── config/                  # Configuration
│
├── pkg/                         # Public packages
│   ├── errors/                  # Custom error types
│   └── response/                # HTTP response helpers
│
├── tests/
│   └── load-test.js             # k6 load testing
│
├── docs/                        # Documentation (see below)
├── .cloudformation/             # CloudFormation templates (ciinabox)
├── scripts/                     # Utility scripts
│
├── .env.example                 # Environment template
├── Dockerfile                   # Docker image
├── docker-compose.yml           # Local development
├── Jenkinsfile                  # CI/CD pipeline definition
├── Makefile                     # Build automation (Docker-based)
├── go.mod / go.sum              # Go dependencies
└── README.md                    # This file
```

---

## Security Considerations

### Best Practices

1. **JWT Secret Keys**
   - Use cryptographically random keys (minimum 32 characters, 64+ recommended)
   - Rotate keys periodically (every 90 days recommended)
   - Never commit secrets to version control
   - Use environment variables or secret management systems (Vault, AWS Secrets Manager)

2. **Database Security**
   - Use SSL/TLS for database connections (`DB_SSL_MODE=require`)
   - Implement least privilege access (dedicated database user with minimal permissions)
   - Regular security audits and updates
   - Connection pooling limits to prevent resource exhaustion

3. **Redis Security**
   - Use password authentication
   - Enable SSL/TLS in production
   - Network isolation (private subnets)
   - Regular backups for persistence

4. **OAuth Configuration**
   - Validate redirect URLs strictly
   - Use HTTPS for all OAuth callbacks
   - Implement state parameter for CSRF protection
   - Store OAuth secrets securely

5. **Production Deployment**
   - Always use HTTPS (TLS 1.3 recommended)
   - Enable security headers
   - Implement rate limiting
   - Monitor for suspicious activity
   - Regular security updates

### Reporting Security Issues

If you discover a security vulnerability, please email security@boddlelearning.com. Do not create a public GitHub issue.

---

## Performance Tuning

### Optimization Tips

1. **Database Connection Pooling**
   ```bash
   DB_MAX_OPEN_CONNECTIONS=25
   DB_MAX_IDLE_CONNECTIONS=10
   DB_CONNECTION_MAX_LIFETIME=5m
   ```

2. **Redis Configuration**
   ```bash
   REDIS_POOL_SIZE=10
   REDIS_MAX_RETRIES=3
   REDIS_TIMEOUT=3s
   ```

3. **HTTP Server Tuning**
   ```bash
   GIN_MODE=release
   HTTP_READ_TIMEOUT=10s
   HTTP_WRITE_TIMEOUT=10s
   HTTP_IDLE_TIMEOUT=120s
   ```

4. **Resource Limits (Kubernetes)**
   ```yaml
   resources:
     requests:
       memory: "256Mi"
       cpu: "500m"
     limits:
       memory: "512Mi"
       cpu: "1000m"
   ```

---

## Monitoring

### Prometheus Metrics

Metrics are exposed at `GET /metrics` in Prometheus format:

```prometheus
# Example metrics
auth_login_attempts_total{method="email",status="success"} 1523
auth_login_duration_seconds{method="email",quantile="0.95"} 0.234
auth_active_tokens 342
http_requests_total{method="POST",path="/auth/login",status="200"} 1523
```

---

## Troubleshooting

### Common Issues

#### "Invalid token format"
**Cause**: JWT secret key mismatch
**Solution**: Verify `JWT_SECRET_KEY` matches between services

#### "Token has expired"
**Cause**: Token older than TTL
**Solution**: Request new token, check server time synchronization

#### "Rate limit exceeded"
**Cause**: Too many failed login attempts
**Solution**: Wait for lockout period to expire (default 15 minutes)

#### "Redis connection refused"
**Cause**: Redis not running or unreachable
**Solution**: Check Redis status: `redis-cli ping`

#### "Database connection failed"
**Cause**: PostgreSQL not running or incorrect credentials
**Solution**: Verify database configuration and connectivity

For deployment-related issues, see [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md).

---

## Contributing

### Development Setup

```bash
# Clone repository
git clone https://github.com/boddle-learning/reservoir.git
cd reservoir

# Build and test (Docker-based — no local Go required)
make build
make test
```

### Code Style

- Follow [Effective Go](https://golang.org/doc/effective_go.html) guidelines
- Use `gofmt` for formatting: `make fmt`
- Run linters: `make lint`

---

## License

Copyright © 2024-2026 Boddle Learning Inc. All rights reserved.

This software is proprietary and confidential. Unauthorized copying, modification, distribution, or use of this software, via any medium, is strictly prohibited.

For licensing inquiries, contact: legal@boddlelearning.com

---

## Support

### Commercial Support

For enterprise support, SLA agreements, and custom development:
- Email: enterprise@boddlelearning.com
- Website: https://www.boddlelearning.com

### Community

- **Documentation**: [docs/](docs/)
- **Issues**: [GitHub Issues](https://github.com/boddle-learning/reservoir/issues)
- **Discussions**: [GitHub Discussions](https://github.com/boddle-learning/reservoir/discussions)

---

<div align="center">

**Built with ❤️ by the Boddle Engineering Team**

[Website](https://www.boddlelearning.com) • [Blog](https://blog.boddlelearning.com) • [Careers](https://www.boddlelearning.com/careers)

</div>
