# JWT Authentication - Quick Reference

Quick reference guide for using JWT authentication in the Rails LMS with the Go Authentication Gateway.

## Environment Setup

```bash
# Required
export JWT_SECRET_KEY="your-64-char-hex-key"  # MUST match Go gateway
export REDIS_URL="redis://localhost:6379/0"

# Migration control
export JWT_FALLBACK_TO_SESSION="true"  # true during migration, false after
```

## Files Added

```
learning-management-system/
├── app/
│   ├── middleware/
│   │   └── jwt_auth.rb                    # JWT validation middleware
│   └── controllers/
│       └── concerns/
│           └── application_controller_jwt_helpers.rb  # Helper methods
└── config/
    └── initializers/
        └── jwt_auth.rb                     # Configuration
```

## Helper Methods

### Authentication
```ruby
current_user              # User from JWT or session
jwt_current_user          # User from JWT only
session_current_user      # User from session only
logged_in?                # Is user authenticated?
authenticate_user!        # Require authentication
```

### JWT Data
```ruby
jwt_payload               # Decoded JWT payload (hash)
jwt_token                 # JWT token string
```

### User Meta
```ruby
current_user_meta         # Get Teacher/Student/Parent record
current_user_teacher?     # Is user a teacher?
current_user_student?     # Is user a student?
current_user_parent?      # Is user a parent?
current_user_boddle_uid   # Get Boddle UID
```

## Usage Examples

### Require Authentication
```ruby
class DashboardController < ApplicationController
  before_action :authenticate_user!

  def index
    # current_user is automatically available
    @user = current_user
  end
end
```

### Check Authentication Method
```ruby
if jwt_current_user
  # User authenticated via JWT
  puts "JWT User: #{jwt_payload['email']}"
elsif session_current_user
  # User authenticated via session
  puts "Session User: #{session[:user_id]}"
end
```

### Access JWT Claims
```ruby
if jwt_payload
  user_id = jwt_payload['user_id']
  email = jwt_payload['email']
  meta_type = jwt_payload['meta_type']  # Teacher/Student/Parent
  boddle_uid = jwt_payload['boddle_uid']
end
```

### Check User Type
```ruby
if current_user_teacher?
  # Teacher-specific logic
  render :teacher_dashboard
elsif current_user_student?
  # Student-specific logic
  render :student_dashboard
elsif current_user_parent?
  # Parent-specific logic
  render :parent_dashboard
end
```

## Testing

### Get JWT Token (Go Gateway)
```bash
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"password123"}'
```

### Test Rails Endpoint with JWT
```bash
curl http://localhost:3000/api/endpoint \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
```

### Test Rails Endpoint with Session
```bash
# Login first
curl -X POST http://localhost:3000/login \
  -d "email=user@example.com&password=password123" \
  --cookie-jar cookies.txt

# Use session
curl http://localhost:3000/api/endpoint \
  --cookie cookies.txt
```

## JWT Payload Structure

```ruby
{
  "user_id" => 123,                    # User primary key
  "boddle_uid" => "abc123",            # Boddle unique identifier
  "email" => "user@example.com",       # User email
  "name" => "John Doe",                # Full name
  "meta_type" => "Teacher",            # User type
  "meta_id" => 456,                    # Meta table primary key
  "exp" => 1707415200,                 # Expiration timestamp
  "iat" => 1707393600,                 # Issued at timestamp
  "jti" => "uuid-here"                 # JWT ID (for blacklist)
}
```

## Authentication Flow

### JWT Authentication
```
1. User logs in via Go gateway (http://localhost:8080/auth/login)
2. Go gateway returns JWT token
3. Client includes token in Authorization header
4. Rails JWT middleware validates token
5. Middleware populates request.env['current_user']
6. current_user available in controllers
```

### Session Authentication (Fallback)
```
1. User logs in via Rails (traditional)
2. Rails creates session with user_id
3. Session cookie sent to client
4. JWT middleware checks JWT_FALLBACK_TO_SESSION
5. If true and no JWT, falls back to session
6. current_user populated from session
```

## Debugging

### Check JWT in Rails Console
```ruby
rails console

require 'jwt'
secret = ENV['JWT_SECRET_KEY']
token = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."

# Decode token
payload = JWT.decode(token, secret, true, { algorithm: 'HS256' }).first

# Check user
user = User.find_by(id: payload['user_id'])
```

### Check Redis Blacklist
```bash
redis-cli
> EXISTS blacklist:jti:abc-123-def-456
> TTL blacklist:jti:abc-123-def-456
```

### Check Logs
```bash
# JWT authentication events
tail -f log/development.log | grep -i jwt

# Errors only
tail -f log/development.log | grep -i "jwt.*error"
```

## Common Issues

### "Invalid token format"
**Cause:** JWT_SECRET_KEY mismatch
**Fix:** Verify keys match between Go and Rails

### "Token has expired"
**Cause:** Token older than 6 hours
**Fix:** Get new token from Go gateway

### "User not found"
**Cause:** User deleted after JWT issued
**Fix:** User must log in again

### Redis connection error
**Cause:** Redis unavailable
**Fix:** Start Redis: `redis-server`

## Migration Phases

### Phase 1: Dual Auth (Week 1-2)
```bash
JWT_FALLBACK_TO_SESSION=true
JWT_ROLLOUT_PERCENTAGE=0
```
Both JWT and session work.

### Phase 2: Gradual (Week 3-6)
```bash
JWT_FALLBACK_TO_SESSION=true
JWT_ROLLOUT_PERCENTAGE=25  # Then 50, 75, 100
```
Gradually migrate users to JWT.

### Phase 3: JWT-Only (Week 7)
```bash
JWT_FALLBACK_TO_SESSION=false
JWT_ROLLOUT_PERCENTAGE=100
```
All authentication via JWT.

### Phase 4: Cleanup (Week 8)
Remove legacy session code.

## API Endpoints (Go Gateway)

### Authentication
- `POST /auth/login` - Email/password login
- `GET /auth/token?token=SECRET` - Login token
- `GET /auth/google?redirect_url=...` - Google OAuth
- `GET /auth/clever?redirect_url=...` - Clever SSO
- `GET /auth/icloud?redirect_url=...` - iCloud Sign In
- `POST /auth/logout` - Logout (blacklist token)

### Protected
- `GET /auth/me` - Get current user (requires JWT)

## Security Checklist

- [ ] JWT_SECRET_KEY set and matches Go gateway
- [ ] JWT_SECRET_KEY not committed to git
- [ ] HTTPS enabled in production
- [ ] Redis highly available (Sentinel/Cluster)
- [ ] Rate limiting enabled
- [ ] Monitoring configured
- [ ] Error tracking enabled
- [ ] Logs don't include JWT tokens

## Performance Targets

| Metric | Target |
|--------|--------|
| JWT validation (p95) | < 10ms |
| Auth success rate | > 99% |
| Error rate | < 0.1% |
| Throughput | 1000+ req/s |

## Support

- **Documentation:** `docs/current-system/rails-integration.md`
- **Migration Guide:** `docs/RAILS_MIGRATION_GUIDE.md`
- **Troubleshooting:** See full Rails integration guide
- **Contact:** engineering@boddle.com

---

*Quick reference v1.0 - February 2026*
