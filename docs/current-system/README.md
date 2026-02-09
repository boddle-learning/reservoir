# Current System Documentation - Index

This directory contains comprehensive documentation about the current Boddle authentication system, including both the Go Authentication Gateway (Reservoir) and Rails LMS integration.

## Documents

### 1. Rails Integration Guide
**File:** `rails-integration.md`

**Purpose:** Complete guide for integrating JWT authentication with Rails LMS

**Contents:**
- Files added to Rails application
- Configuration requirements
- Testing procedures
- Migration strategy (4 phases)
- Monitoring and troubleshooting
- Performance expectations
- Security considerations
- Rollback procedures

**Audience:** All teams (Development, DevOps, QA)

**Use When:**
- Setting up JWT authentication in Rails
- Planning migration from sessions to JWT
- Troubleshooting authentication issues
- Understanding the integration architecture

---

### 2. JWT Quick Reference
**File:** `jwt-quick-reference.md`

**Purpose:** Quick reference for developers using JWT authentication

**Contents:**
- Environment setup
- Helper methods reference
- Usage examples
- JWT payload structure
- Debugging commands
- Common issues and fixes
- API endpoints
- Migration phases

**Audience:** Developers

**Use When:**
- Need quick syntax reference
- Writing authentication code
- Debugging authentication issues
- Testing JWT integration

---

### 3. Authentication System Overview
**File:** `authentication.md`

**Purpose:** High-level overview of the authentication system

**Contents:**
- System architecture
- Authentication methods
- User model structure
- OAuth flows
- Session management
- Security features

**Audience:** All technical staff

**Use When:**
- Onboarding new team members
- Understanding system architecture
- Planning new features
- Security audits

---

### 4. Database Schema
**File:** `database-schema.md`

**Purpose:** Database schema documentation for authentication-related tables

**Contents:**
- Users table structure
- Meta tables (teachers, students, parents)
- OAuth UID columns
- Login tokens and attempts
- Relationships and indexes

**Audience:** Developers, Database Administrators

**Use When:**
- Understanding data model
- Writing database queries
- Planning schema changes
- Debugging data issues

---

## Quick Start

### For Developers

1. **Read:** `jwt-quick-reference.md` for syntax and examples
2. **Read:** `rails-integration.md` for setup instructions
3. **Set up:** Environment variables (JWT_SECRET_KEY, REDIS_URL)
4. **Test:** Authentication flow with JWT tokens
5. **Reference:** Quick reference while coding

### For DevOps

1. **Read:** `rails-integration.md` migration strategy
2. **Configure:** Production environment variables
3. **Deploy:** Go Authentication Gateway
4. **Deploy:** Rails JWT integration
5. **Monitor:** Metrics and error rates
6. **Execute:** Gradual rollout (Phases 1-4)

### For QA

1. **Read:** `rails-integration.md` testing section
2. **Test:** All authentication methods (email, OAuth, SSO, tokens)
3. **Test:** Both JWT and session authentication
4. **Test:** Token revocation and expiry
5. **Load test:** Using k6 script (`tests/load-test.js`)

---

## Architecture Overview

```
┌─────────────────┐
│   Web Client    │
│  Mobile Client  │
│   Game Client   │
└────────┬────────┘
         │
         ▼
┌─────────────────────────────────┐
│  Go Authentication Gateway      │
│  (Reservoir)                    │
├─────────────────────────────────┤
│ - Email/Password Auth           │
│ - Google OAuth2                 │
│ - Clever SSO                    │
│ - iCloud Sign In                │
│ - Login Tokens (Magic Links)    │
│ - JWT Generation                │
│ - Rate Limiting                 │
└────────┬────────────────────────┘
         │ JWT Token
         ▼
┌─────────────────────────────────┐
│  Rails LMS Application          │
│  (learning-management-system)   │
├─────────────────────────────────┤
│ - JWT Validation Middleware     │
│ - Session Fallback (migration)  │
│ - Token Blacklist Check         │
│ - Current User Population       │
└─────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────┐
│  Database & Cache               │
├─────────────────────────────────┤
│ - PostgreSQL (users, meta)      │
│ - Redis (blacklist, rate limit) │
└─────────────────────────────────┘
```

---

## Authentication Methods

### 1. Email/Password
- Traditional username/password authentication
- Password hashed with bcrypt
- Rate limited (5 attempts per 10 minutes)
- Returns JWT token on success

### 2. Google OAuth2
- OAuth 2.0 flow with Google
- Scopes: email, profile
- Account linking by email or Google UID
- Supports teachers and students

### 3. Clever SSO
- OAuth 2.0 flow with Clever
- K-12 education platform integration
- Account linking by email or Clever UID
- Supports teachers and students

### 4. iCloud Sign In (Apple)
- OAuth 2.0 flow with Apple
- JWT-signed client secret (ES256)
- Supports "Hide My Email"
- Primarily for students and parents

### 5. Login Tokens (Magic Links)
- Time-limited token (5 minutes)
- Permanent tokens for game links
- Database-backed validation
- Backward compatible with Rails

---

## Key Concepts

### JWT Structure
- **Algorithm:** HS256 (HMAC-SHA256)
- **Expiry:** 6 hours (access), 30 days (refresh)
- **Claims:** user_id, email, name, meta_type, boddle_uid
- **Blacklist:** Redis-backed for revocation

### Dual Authentication
- **JWT:** Stateless, scalable, modern
- **Session:** Stateful, legacy, backward compatible
- **Fallback:** Controlled by JWT_FALLBACK_TO_SESSION
- **Migration:** Gradual rollout by percentage

### Security Features
- **Rate Limiting:** 5 attempts/10 min, 15min lockout
- **Token Blacklist:** Revoked tokens in Redis
- **OAuth State:** CSRF protection with Redis
- **Security Headers:** XSS, clickjacking, MIME protection
- **HTTPS:** Enforced in production

---

## Migration Status

### Current Phase: Phase 1 (Dual Authentication)
✅ Go Authentication Gateway deployed
✅ Rails JWT middleware installed
✅ JWT validation working
✅ Session fallback enabled
⏳ Production deployment pending

### Completed
- [x] Phase 1-7 implementation (Go gateway)
- [x] Rails integration files added
- [x] Documentation completed
- [x] Testing infrastructure ready
- [x] Load testing scripts created

### In Progress
- [ ] Environment configuration
- [ ] Local testing
- [ ] Production deployment
- [ ] Monitoring setup

### Upcoming
- [ ] Gradual rollout (25% → 50% → 75% → 100%)
- [ ] JWT-only mode
- [ ] Legacy code cleanup

---

## File Locations

### Go Authentication Gateway (Reservoir)
```
/Users/stjohncj/dev/boddle/reservoir/
├── cmd/server/main.go                    # Entry point
├── internal/
│   ├── auth/                             # Auth logic
│   ├── oauth/                            # OAuth providers
│   ├── token/                            # JWT service
│   ├── ratelimit/                        # Rate limiting
│   └── middleware/                       # HTTP middleware
├── docs/
│   ├── RAILS_MIGRATION_GUIDE.md          # Full migration guide
│   ├── IMPLEMENTATION_SUMMARY.md         # Project summary
│   ├── current-system/                   # This directory
│   │   ├── rails-integration.md          # Rails integration
│   │   ├── jwt-quick-reference.md        # Quick reference
│   │   ├── authentication.md             # Auth overview
│   │   └── database-schema.md            # DB schema
│   └── rails/                            # Rails code files
│       ├── app/middleware/
│       ├── app/controllers/concerns/
│       └── config/initializers/
└── tests/
    └── load-test.js                      # k6 load tests
```

### Rails LMS Application
```
/Users/stjohncj/dev/boddle/learning-management-system/
├── app/
│   ├── middleware/
│   │   └── jwt_auth.rb                   # JWT validation
│   └── controllers/
│       ├── application_controller.rb     # Base controller
│       └── concerns/
│           └── application_controller_jwt_helpers.rb  # Helpers
├── config/
│   └── initializers/
│       └── jwt_auth.rb                   # JWT config
└── JWT_INTEGRATION_GUIDE.md              # Setup guide
```

---

## Environment Variables Reference

### Required
```bash
JWT_SECRET_KEY="64-char-hex-key"          # MUST match between Go/Rails
REDIS_URL="redis://localhost:6379/0"     # Shared Redis instance
```

### OAuth Configuration
```bash
GOOGLE_CLIENT_ID="..."
GOOGLE_CLIENT_SECRET="..."
GOOGLE_REDIRECT_URL="http://localhost:8080/auth/google/callback"

CLEVER_CLIENT_ID="..."
CLEVER_CLIENT_SECRET="..."
CLEVER_REDIRECT_URL="http://localhost:8080/auth/clever/callback"

ICLOUD_SERVICE_ID="com.boddle.auth"
ICLOUD_TEAM_ID="..."
ICLOUD_KEY_ID="..."
ICLOUD_PRIVATE_KEY_PATH="/secrets/key.p8"
ICLOUD_REDIRECT_URL="http://localhost:8080/auth/icloud/callback"
```

### Migration Control
```bash
JWT_FALLBACK_TO_SESSION="true"            # true=both, false=JWT only
JWT_ROLLOUT_PERCENTAGE="0"                # 0-100, gradual rollout
```

---

## Common Commands

### Start Services
```bash
# Redis
redis-server

# Go Gateway
cd /Users/stjohncj/dev/boddle/reservoir
go run cmd/server/main.go

# Rails LMS
cd /Users/stjohncj/dev/boddle/learning-management-system
rails server
```

### Testing
```bash
# Get JWT token
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"password"}'

# Test Rails with JWT
curl http://localhost:3000/api/endpoint \
  -H "Authorization: Bearer TOKEN_HERE"

# Load test
k6 run tests/load-test.js
```

### Debugging
```bash
# Rails console
rails console
require 'jwt'
JWT.decode(token, ENV['JWT_SECRET_KEY'], true, {algorithm: 'HS256'})

# Redis
redis-cli
EXISTS blacklist:jti:TOKEN_ID
TTL blacklist:jti:TOKEN_ID

# Logs
tail -f log/development.log | grep -i jwt
```

---

## Support Resources

### Documentation
- **This directory:** Complete system documentation
- **Migration guide:** `../RAILS_MIGRATION_GUIDE.md`
- **Project summary:** `../IMPLEMENTATION_SUMMARY.md`
- **Rails guide:** `../../learning-management-system/JWT_INTEGRATION_GUIDE.md`

### Code Locations
- **Go gateway:** `/Users/stjohncj/dev/boddle/reservoir`
- **Rails LMS:** `/Users/stjohncj/dev/boddle/learning-management-system`

### Contact
- **Engineering:** engineering@boddle.com
- **Slack:** #auth-gateway-migration
- **Issues:** GitHub Issues

---

## Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | Feb 8, 2026 | Initial documentation |
| | | - Rails integration complete |
| | | - JWT authentication working |
| | | - Migration strategy defined |

---

*Last updated: February 8, 2026*
*Maintained by: Engineering Team*
