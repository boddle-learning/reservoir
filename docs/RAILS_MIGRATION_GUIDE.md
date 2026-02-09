# Rails Integration & Migration Guide

This guide provides step-by-step instructions for integrating Rails LMS with the Go Authentication Gateway (Reservoir) and migrating from cookie-based sessions to JWT tokens.

## Table of Contents
1. [Prerequisites](#prerequisites)
2. [Installation](#installation)
3. [Configuration](#configuration)
4. [Testing](#testing)
5. [Rollout Strategy](#rollout-strategy)
6. [Monitoring](#monitoring)
7. [Troubleshooting](#troubleshooting)

---

## Prerequisites

- Go Authentication Gateway (Reservoir) deployed and running
- Rails LMS application
- Redis instance (shared with Go gateway)
- JWT secret key (shared between Go and Rails)

### Required Gems

Add to your `Gemfile`:

```ruby
gem 'jwt', '~> 2.7'
gem 'redis', '~> 5.0'
```

Then run:
```bash
bundle install
```

---

## Installation

### Step 1: Add JWT Middleware

Copy the JWT middleware to your Rails application:

```bash
cp docs/rails/app/middleware/jwt_auth.rb app/middleware/
```

### Step 2: Add Controller Helpers

Copy the controller helpers:

```bash
cp docs/rails/app/controllers/concerns/application_controller_jwt_helpers.rb \
   app/controllers/concerns/
```

### Step 3: Add Initializer

Copy the JWT configuration initializer:

```bash
cp docs/rails/config/initializers/jwt_auth.rb config/initializers/
```

### Step 4: Update ApplicationController

Add JWT helpers to your `ApplicationController`:

```ruby
# app/controllers/application_controller.rb
class ApplicationController < ActionController::Base
  include ApplicationControllerJwtHelpers

  # Replace existing authenticate_user! if you have one
  # The helper module provides JWT-aware authentication

  # Optional: Require authentication for all actions
  # before_action :authenticate_user!

  # Optional: Skip CSRF for API endpoints (JWT handles auth)
  # skip_before_action :verify_authenticity_token, if: :jwt_request?

  private

  def jwt_request?
    jwt_current_user.present?
  end
end
```

---

## Configuration

### Environment Variables

Set the following environment variables in your Rails application:

```bash
# Required: JWT secret key (MUST match Go gateway)
export JWT_SECRET_KEY="your-64-character-hex-secret-key-here"

# Optional: Redis URL (for token blacklist)
export REDIS_URL="redis://localhost:6379/0"

# Migration: Enable session fallback during rollout
export JWT_FALLBACK_TO_SESSION="true"

# Production: After migration is complete
export JWT_FALLBACK_TO_SESSION="false"
```

### Verify JWT Secret Key Matches

Ensure the `JWT_SECRET_KEY` in Rails **exactly matches** the value in the Go gateway:

**Go (.env):**
```
JWT_SECRET_KEY=abc123...
```

**Rails (.env):**
```
JWT_SECRET_KEY=abc123...
```

---

## Testing

### Test JWT Validation

Create a test endpoint to verify JWT validation:

```ruby
# app/controllers/test_controller.rb
class TestController < ApplicationController
  def jwt_status
    render json: {
      authenticated: logged_in?,
      user: current_user&.as_json(only: [:id, :email, :meta_type]),
      jwt_payload: jwt_payload,
      session_user: session[:user_id].present?
    }
  end
end
```

Add route:
```ruby
# config/routes.rb
get '/test/jwt_status', to: 'test#jwt_status'
```

### Test with curl

1. **Get a JWT from Go gateway:**
```bash
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "test@example.com",
    "password": "password123"
  }'
```

2. **Test Rails endpoint with JWT:**
```bash
curl http://localhost:3000/test/jwt_status \
  -H "Authorization: Bearer YOUR_JWT_TOKEN_HERE"
```

Expected response:
```json
{
  "authenticated": true,
  "user": {
    "id": 123,
    "email": "test@example.com",
    "meta_type": "Teacher"
  },
  "jwt_payload": {
    "user_id": 123,
    "email": "test@example.com",
    ...
  },
  "session_user": false
}
```

---

## Rollout Strategy

### Phase 1: Dual Authentication (Week 1)

**Goal:** Both session cookies and JWTs work simultaneously.

**Configuration:**
```bash
JWT_FALLBACK_TO_SESSION=true
```

**What happens:**
- Existing users continue using sessions
- New logins through Go gateway receive JWTs
- Both authentication methods work

**Actions:**
1. Deploy Go gateway (not enforced yet)
2. Deploy Rails JWT middleware
3. Monitor for errors
4. Test both auth methods work

### Phase 2: Gradual Migration (Weeks 2-3)

**Goal:** Migrate users to JWT gradually.

**Configuration:**
```bash
JWT_FALLBACK_TO_SESSION=true
JWT_ROLLOUT_PERCENTAGE=25  # Start with 25%
```

**Implementation:**
```ruby
# app/controllers/sessions_controller.rb
def create
  user = User.authenticate(params[:email], params[:password])

  if rollout_jwt_for_user?(user)
    # Redirect to Go gateway for JWT login
    redirect_to "http://auth.boddle.com/auth/login?email=#{user.email}&redirect_url=#{dashboard_url}"
  else
    # Traditional session-based login
    session[:user_id] = user.id
    redirect_to dashboard_path
  end
end

private

def rollout_jwt_for_user?(user)
  # Gradually roll out by user ID
  percentage = ENV.fetch('JWT_ROLLOUT_PERCENTAGE', '0').to_i
  (user.id % 100) < percentage
end
```

**Rollout schedule:**
- Week 2: 25% of users
- Week 3: 50% of users
- Week 4: 75% of users
- Week 5: 100% of users

### Phase 3: JWT-Only (Week 6)

**Goal:** All authentication through JWT.

**Configuration:**
```bash
JWT_FALLBACK_TO_SESSION=false
```

**Actions:**
1. Update login flow to always use Go gateway
2. Invalidate all existing sessions
3. Force all users to re-login through Go gateway
4. Monitor error rates

### Phase 4: Cleanup (Week 7)

**Goal:** Remove legacy session code.

**Actions:**
1. Remove session-based authentication code
2. Remove SessionsController (if fully replaced)
3. Remove session middleware configuration
4. Remove rollout percentage logic
5. Update documentation

---

## Monitoring

### Key Metrics to Track

1. **Authentication Success Rate**
```ruby
# config/initializers/metrics.rb
require 'prometheus/client'

prometheus = Prometheus::Client.registry

AUTH_SUCCESS = prometheus.counter(
  :auth_success_total,
  docstring: 'Total successful authentications',
  labels: [:method] # jwt, session
)

AUTH_FAILURE = prometheus.counter(
  :auth_failure_total,
  docstring: 'Total failed authentications',
  labels: [:method, :reason]
)
```

2. **JWT Validation Duration**
```ruby
JWT_VALIDATION_DURATION = prometheus.histogram(
  :jwt_validation_duration_seconds,
  docstring: 'JWT validation duration'
)
```

3. **Active User Sessions**
```ruby
ACTIVE_JWT_USERS = prometheus.gauge(
  :active_jwt_users,
  docstring: 'Number of users authenticated via JWT'
)

ACTIVE_SESSION_USERS = prometheus.gauge(
  :active_session_users,
  docstring: 'Number of users authenticated via session'
)
```

### Monitoring Dashboard (Grafana)

Create Grafana dashboard with:
- Authentication method breakdown (JWT vs Session)
- Error rate by authentication method
- JWT validation latency (p50, p95, p99)
- Active users by authentication method
- Token blacklist hit rate

### Logging

Add structured logging for JWT authentication:

```ruby
# app/middleware/jwt_auth.rb
def call(env)
  start_time = Time.now

  # ... JWT validation logic ...

  duration = Time.now - start_time
  Rails.logger.info({
    event: 'jwt_validation',
    duration_ms: (duration * 1000).round(2),
    user_id: user&.id,
    success: user.present?
  }.to_json)

  @app.call(env)
end
```

### Alerts

Set up alerts for:
1. JWT validation error rate > 1%
2. JWT validation latency p95 > 100ms
3. Redis connection failures
4. Token blacklist unavailable

---

## Troubleshooting

### Issue: "Invalid token format"

**Cause:** JWT secret key mismatch between Go and Rails.

**Solution:**
```bash
# Verify keys match exactly
echo $JWT_SECRET_KEY  # In Go environment
echo $JWT_SECRET_KEY  # In Rails environment
```

### Issue: "Token has expired"

**Cause:** Token TTL mismatch or clock skew.

**Solution:**
- Verify Go gateway and Rails server clocks are synchronized (use NTP)
- Check token TTL configuration matches:
  ```bash
  # Go
  JWT_ACCESS_TOKEN_TTL=6h

  # Rails should validate same TTL
  ```

### Issue: "User not found"

**Cause:** User ID in JWT doesn't exist in database.

**Solution:**
- Check database connectivity
- Verify user wasn't deleted after JWT was issued
- Check JWT claims: `JWT.decode(token, secret, true, { algorithm: 'HS256' })`

### Issue: Redis connection error

**Cause:** Redis unavailable for blacklist checking.

**Solution:**
- JWT middleware fails open (allows request) if Redis is down
- Check Redis connectivity: `redis-cli ping`
- Verify REDIS_URL environment variable

### Issue: Session users can't access protected routes

**Cause:** JWT_FALLBACK_TO_SESSION=false but users still have sessions.

**Solution:**
- Set JWT_FALLBACK_TO_SESSION=true during migration
- Force re-login for all users
- Clear all sessions: `rake sessions:clear`

---

## Testing Checklist

- [ ] JWT validation works for valid tokens
- [ ] Expired tokens are rejected
- [ ] Blacklisted tokens are rejected
- [ ] Session fallback works when enabled
- [ ] current_user returns correct user for JWT
- [ ] current_user returns correct user for session
- [ ] current_user_meta loads correct meta type
- [ ] API endpoints work with Bearer token
- [ ] Redis connection failure doesn't block requests
- [ ] Logout blacklists JWT token
- [ ] Monitoring metrics are being collected
- [ ] Logs show JWT validation events
- [ ] Load testing shows acceptable performance

---

## Performance Benchmarks

Expected performance after migration:

| Metric | Target | Notes |
|--------|--------|-------|
| JWT validation latency (p95) | < 10ms | Without Redis blacklist check |
| JWT validation latency (p95) | < 50ms | With Redis blacklist check |
| Throughput | 10,000 req/s | Per Rails instance |
| Memory overhead | < 10MB | JWT middleware |
| Error rate | < 0.1% | JWT validation errors |

---

## Security Considerations

1. **JWT Secret Key Rotation**
   - Plan for periodic JWT secret key rotation
   - Implement graceful key rotation with dual-key support
   - Document key rotation procedure

2. **Token Revocation**
   - Ensure Redis blacklist is highly available
   - Consider Redis Sentinel or Cluster for production
   - Monitor blacklist size and set TTL=token expiry

3. **HTTPS Only**
   - Always use HTTPS in production
   - JWT tokens are bearer tokens (anyone with token can authenticate)
   - Never log JWT tokens

4. **Rate Limiting**
   - Maintain rate limiting on authentication endpoints
   - Rate limit by IP and user ID
   - Monitor for brute force attempts

---

## Rollback Plan

If issues occur during migration:

1. **Immediate Rollback:**
```bash
# Disable JWT enforcement
JWT_FALLBACK_TO_SESSION=true
JWT_ROLLOUT_PERCENTAGE=0

# Restart Rails servers
```

2. **Redeploy Previous Version:**
```bash
# Revert code changes
git revert HEAD
git push
# Trigger deployment
```

3. **Communication:**
- Notify team via Slack/email
- Update status page
- Post incident report

---

## Support

For issues or questions:
- Check logs: `tail -f log/production.log | grep jwt`
- Check Go gateway logs: `kubectl logs -f deployment/reservoir`
- Contact: engineering@boddle.com

---

## Next Steps

After successful migration:
1. Remove session-based authentication code
2. Remove JWT_FALLBACK_TO_SESSION configuration
3. Update documentation to reflect JWT-only auth
4. Celebrate! 🎉
