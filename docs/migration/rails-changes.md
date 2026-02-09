# Rails Changes Required for Go Authentication Gateway

This document outlines all changes that need to be made to the Rails LMS application to support the new Go authentication gateway with JWT tokens.

## Overview

The migration will be **non-breaking** and use a **phased rollout** approach:

1. **Phase 1**: Rails accepts BOTH cookies and JWTs (dual mode)
2. **Phase 2**: Gradually route more traffic to Go Gateway
3. **Phase 3**: All authentication through Go, JWT-only
4. **Phase 4**: Remove Rails authentication code

## Changes Required

### 1. Add JWT Validation Middleware

**New File**: `app/middleware/jwt_auth.rb`

```ruby
# app/middleware/jwt_auth.rb
require 'jwt'

class JwtAuth
  def initialize(app)
    @app = app
  end

  def call(env)
    request = Rack::Request.new(env)

    # Skip JWT validation for certain paths
    if skip_jwt_validation?(request.path)
      return @app.call(env)
    end

    begin
      token = extract_token(request)

      if token
        # JWT provided - validate it
        payload = decode_token(token)

        # Check if token is blacklisted (Redis check)
        if token_blacklisted?(payload['jti'])
          return unauthorized_response('Token revoked')
        end

        # Populate user from JWT claims
        user = User.find_by(id: payload['user_id'])
        unless user
          return unauthorized_response('User not found')
        end

        # Verify user data matches JWT claims (security check)
        unless user.boddle_uid == payload['boddle_uid']
          return unauthorized_response('Invalid token claims')
        end

        # Store in request env for controllers
        env['current_user'] = user
        env['jwt_payload'] = payload

      elsif fallback_to_session?
        # Backward compatibility: allow cookie-based sessions
        # (Existing session logic will run in controllers)
      else
        # JWT required, none provided
        return unauthorized_response('Missing authentication token')
      end

      @app.call(env)

    rescue JWT::ExpiredSignature
      unauthorized_response('Token expired')
    rescue JWT::DecodeError => e
      Rails.logger.error("JWT decode error: #{e.message}")
      unauthorized_response('Invalid token')
    rescue => e
      Rails.logger.error("JWT auth error: #{e.class} - #{e.message}")
      unauthorized_response('Authentication failed')
    end
  end

  private

  def decode_token(token)
    JWT.decode(
      token,
      ENV['JWT_SECRET_KEY'],
      true,
      {
        algorithm: 'HS256',
        verify_expiration: true,
        verify_iat: true
      }
    ).first
  end

  def extract_token(request)
    # Try Authorization header first
    auth_header = request.env['HTTP_AUTHORIZATION']
    if auth_header&.start_with?('Bearer ')
      return auth_header.split(' ', 2).last
    end

    # Fall back to query parameter (for some game client scenarios)
    request.params['token']
  end

  def token_blacklisted?(jti)
    return false unless jti

    # Check Redis for blacklisted token
    redis = Redis.new(url: ENV['REDIS_URL'])
    result = redis.exists?("blacklist:jti:#{jti}")
    redis.close
    result
  rescue => e
    Rails.logger.error("Redis blacklist check failed: #{e.message}")
    # Fail open: if Redis is down, allow token
    false
  end

  def fallback_to_session?
    # Feature flag to enable cookie sessions during migration
    ENV['JWT_FALLBACK_TO_SESSION'] == 'true'
  end

  def skip_jwt_validation?(path)
    # Skip JWT for these paths
    skip_patterns = [
      '/health',
      '/auth/',           # Auth endpoints (handled by Go Gateway)
      '/public/',         # Public assets
      '/assets/',         # Asset pipeline
      '/rails/active_storage/',  # Active Storage
    ]

    skip_patterns.any? { |pattern| path.start_with?(pattern) }
  end

  def unauthorized_response(message)
    [
      401,
      { 'Content-Type' => 'application/json' },
      [{ error: message, code: 'unauthorized' }.to_json]
    ]
  end
end
```

---

### 2. Register JWT Middleware

**File**: `config/application.rb`

```ruby
# config/application.rb
module BoddleLms
  class Application < Rails::Application
    # ... existing configuration ...

    # Add JWT authentication middleware
    # Place it early in the stack, before session middleware
    config.middleware.insert_before ActionDispatch::Session::CookieStore, JwtAuth
  end
end
```

**Alternative Placement** (if you want JWT to run after session):
```ruby
config.middleware.insert_after ActionDispatch::Session::CookieStore, JwtAuth
```

---

### 3. Update ApplicationController

**File**: `app/controllers/application_controller.rb`

```ruby
# app/controllers/application_controller.rb
class ApplicationController < ActionController::Base
  protect_from_forgery with: :exception, unless: -> { jwt_request? }

  private

  def current_user
    # Priority 1: JWT-authenticated user (from middleware)
    @current_user ||= request.env['current_user']

    # Priority 2: Session-based user (backward compatibility)
    @current_user ||= User.find_by(id: session[:user_id]) if fallback_to_session?

    @current_user
  end

  def jwt_payload
    request.env['jwt_payload']
  end

  def jwt_request?
    # Check if request has JWT (used to disable CSRF for JWT requests)
    request.env['jwt_payload'].present?
  end

  def logged_in?
    current_user.present?
  end

  def require_login
    unless logged_in?
      respond_to do |format|
        format.html { redirect_to login_path, alert: 'Please log in' }
        format.json { render json: { error: 'Unauthorized' }, status: :unauthorized }
      end
    end
  end

  def fallback_to_session?
    ENV['JWT_FALLBACK_TO_SESSION'] == 'true'
  end

  # For debugging (remove in production)
  def auth_source
    if request.env['jwt_payload'].present?
      'JWT'
    elsif session[:user_id].present?
      'Session'
    else
      'None'
    end
  end
end
```

---

### 4. Update API Controllers (Grape)

**File**: `app/controllers/api/v1/defaults.rb`

```ruby
# app/controllers/api/v1/defaults.rb
module API
  module V1
    module Defaults
      extend ActiveSupport::Concern

      included do
        format :json

        rescue_from :all do |e|
          error_response = {
            error: e.message,
            type: e.class.to_s
          }

          Rails.logger.error("API Error: #{e.class} - #{e.message}")
          Rails.logger.error(e.backtrace.join("\n"))

          error!(error_response, 500)
        end

        helpers do
          def current_user
            # Try JWT first
            token = extract_jwt_token
            if token
              payload = decode_jwt(token)
              @current_user ||= User.find_by(id: payload['user_id'])
            end

            # Fall back to session (during migration)
            @current_user ||= User.find_by(id: session[:user_id]) if fallback_to_session?

            @current_user
          end

          def authenticate!
            error!('Unauthorized', 401) unless current_user
          end

          def extract_jwt_token
            # Check Authorization header
            auth_header = headers['Authorization']
            return auth_header.split(' ').last if auth_header&.start_with?('Bearer ')

            # Check params
            params[:token]
          end

          def decode_jwt(token)
            JWT.decode(
              token,
              ENV['JWT_SECRET_KEY'],
              true,
              { algorithm: 'HS256' }
            ).first
          rescue JWT::DecodeError => e
            Rails.logger.error("JWT decode error in API: #{e.message}")
            nil
          end

          def fallback_to_session?
            ENV['JWT_FALLBACK_TO_SESSION'] == 'true'
          end
        end

        # Remove IP-based authentication (replaced by JWT)
        # before do
        #   unless ENV['SLS_IP'] == request.ip
        #     error!('Access Denied', 403)
        #   end
        # end

        # Add JWT authentication requirement
        before do
          authenticate! unless public_endpoint?
        end

        def public_endpoint?
          # Define which endpoints don't require authentication
          request.path.match?(/^\/api\/v1\/(health|version)/)
        end
      end
    end
  end
end
```

---

### 5. Add Session Migration Endpoint

**New File**: `app/controllers/auth_controller.rb`

```ruby
# app/controllers/auth_controller.rb
class AuthController < ApplicationController
  # Convert existing cookie session to JWT
  # This allows gradual migration of existing users
  def migrate_to_jwt
    unless current_user
      render json: { error: 'Not authenticated' }, status: :unauthorized
      return
    end

    begin
      # Call Go Gateway to issue JWT
      response = Faraday.post(
        "#{ENV['AUTH_GATEWAY_URL']}/internal/issue-jwt",
        {
          user_id: current_user.id,
          boddle_uid: current_user.boddle_uid,
          email: current_user.email,
          name: current_user.name,
          meta_type: current_user.meta_type,
          meta_id: current_user.meta_id
        }.to_json,
        {
          'Content-Type' => 'application/json',
          'X-Internal-Token' => ENV['AUTH_GATEWAY_INTERNAL_TOKEN']
        }
      )

      if response.success?
        jwt_data = JSON.parse(response.body)

        # Optionally clear session after migration
        # session.delete(:user_id) if ENV['JWT_CLEAR_SESSION_AFTER_MIGRATION'] == 'true'

        render json: {
          token: jwt_data['token'],
          expires_at: jwt_data['expires_at'],
          user: {
            id: current_user.id,
            email: current_user.email,
            name: current_user.name,
            meta_type: current_user.meta_type
          }
        }
      else
        render json: { error: 'Failed to issue JWT' }, status: :internal_server_error
      end

    rescue Faraday::Error => e
      Rails.logger.error("Failed to connect to Auth Gateway: #{e.message}")
      render json: { error: 'Authentication service unavailable' }, status: :service_unavailable
    end
  end

  # Health check (used by load balancer)
  def health
    render json: { status: 'ok', timestamp: Time.current.iso8601 }
  end
end
```

**Add Route**:
```ruby
# config/routes.rb
post '/auth/migrate-to-jwt', to: 'auth#migrate_to_jwt'
get '/health', to: 'auth#health'
```

---

### 6. Add Redis Configuration

**File**: `config/initializers/redis.rb`

```ruby
# config/initializers/redis.rb
require 'redis'

# Configure Redis connection
REDIS_URL = ENV['REDIS_URL'] || 'redis://localhost:6379/0'

Rails.application.config.redis = Redis.new(
  url: REDIS_URL,
  reconnect_attempts: 3,
  reconnect_delay: 0.5,
  reconnect_delay_max: 5.0,
  timeout: 5
)

# Test connection on startup
begin
  Rails.application.config.redis.ping
  Rails.logger.info("Redis connected: #{REDIS_URL}")
rescue Redis::CannotConnectError => e
  Rails.logger.error("Redis connection failed: #{e.message}")
  Rails.logger.warn("JWT blacklist checks will be disabled")
end
```

**Add to Gemfile**:
```ruby
gem 'redis', '~> 5.0'
gem 'faraday', '~> 2.0'  # For HTTP requests to Go Gateway
```

Run: `bundle install`

---

### 7. Environment Variables

**Add to `.env` or configure in production**:

```bash
# JWT Configuration
JWT_SECRET_KEY=<64-character-hex-secret>  # MUST match Go Gateway
JWT_FALLBACK_TO_SESSION=true              # Enable during migration Phase 1-2
JWT_CLEAR_SESSION_AFTER_MIGRATION=false   # Optional: clear session after JWT issued

# Redis Configuration
REDIS_URL=redis://localhost:6379/0        # Match Go Gateway Redis

# Go Gateway Configuration
AUTH_GATEWAY_URL=http://localhost:8080    # Go Gateway URL
AUTH_GATEWAY_INTERNAL_TOKEN=<shared-secret>  # For internal API calls

# Feature Flags
JWT_ROLLOUT_PERCENTAGE=0                  # Start at 0%, increase gradually
```

**Generate JWT Secret**:
```bash
openssl rand -hex 64
```

**Important**: The `JWT_SECRET_KEY` **MUST be identical** between Rails and Go Gateway.

---

### 8. Update Authentication Controllers

#### Remove or Modify Login Routes

**File**: `config/routes.rb`

```ruby
# config/routes.rb

# Existing login routes - keep for backward compatibility during migration
get '/teachers/login', to: 'teachers#login'
post '/teachers/login', to: 'teachers#login_create'
get '/teachers/logout', to: 'teachers#logout'

# Similar for students, parents

# NEW: Redirect to Go Gateway (Phase 3)
# Uncomment these in Phase 3 when ready to fully migrate
# get '/teachers/login', to: redirect("#{ENV['AUTH_GATEWAY_URL']}/auth/login")
# get '/auth/google', to: redirect("#{ENV['AUTH_GATEWAY_URL']}/auth/google")
# get '/auth/clever', to: redirect("#{ENV['AUTH_GATEWAY_URL']}/auth/clever")
```

#### Modify TeachersController

**File**: `app/controllers/teachers_controller.rb`

Add feature flag check to existing login methods:

```ruby
# app/controllers/teachers_controller.rb
class TeachersController < ApplicationController
  def login
    # If JWT rollout is active, redirect to Go Gateway
    if use_go_gateway?
      redirect_to "#{ENV['AUTH_GATEWAY_URL']}/auth/login?redirect=#{request.original_url}"
      return
    end

    # Existing login form
    # ...
  end

  def login_create
    # If JWT rollout is active, this shouldn't be reached
    # but keep as fallback
    if use_go_gateway?
      flash[:error] = "Please use the new login system"
      redirect_to "#{ENV['AUTH_GATEWAY_URL']}/auth/login"
      return
    end

    # Existing login logic
    # ...
  end

  private

  def use_go_gateway?
    # Feature flag: route authentication to Go Gateway
    percentage = ENV['JWT_ROLLOUT_PERCENTAGE'].to_i
    return false if percentage == 0
    return true if percentage >= 100

    # Gradual rollout based on user ID hash
    user_hash = Digest::MD5.hexdigest(params[:teacher][:email] || '').hex
    (user_hash % 100) < percentage
  end
end
```

---

### 9. Disable Login Token Generation (Phase 3)

**File**: `app/models/login_token.rb`

Add deprecation notice:

```ruby
# app/models/login_token.rb
class LoginToken < ApplicationRecord
  belongs_to :user

  # DEPRECATED: Login tokens now generated by Go Gateway
  # This model is read-only in Rails during migration
  # Rails reads tokens, but Go Gateway creates/deletes them

  def self.create_for_user(user, permanent: false)
    Rails.logger.warn("LoginToken.create_for_user is deprecated. Use Go Gateway API.")

    # For backward compatibility, call Go Gateway
    response = Faraday.post(
      "#{ENV['AUTH_GATEWAY_URL']}/internal/create-login-token",
      {
        user_id: user.id,
        permanent: permanent
      }.to_json,
      {
        'Content-Type' => 'application/json',
        'X-Internal-Token' => ENV['AUTH_GATEWAY_INTERNAL_TOKEN']
      }
    )

    if response.success?
      JSON.parse(response.body)
    else
      raise "Failed to create login token via Go Gateway"
    end
  end
end
```

---

### 10. Add JWT Verification Test

**New File**: `test/integration/jwt_auth_test.rb`

```ruby
# test/integration/jwt_auth_test.rb
require 'test_helper'

class JwtAuthTest < ActionDispatch::IntegrationTest
  setup do
    @user = users(:teacher_one)
    @jwt_secret = ENV['JWT_SECRET_KEY']
  end

  test "should authenticate with valid JWT" do
    token = generate_jwt(@user)

    get api_v1_classrooms_path,
      headers: { 'Authorization' => "Bearer #{token}" }

    assert_response :success
  end

  test "should reject expired JWT" do
    token = generate_jwt(@user, exp: 1.hour.ago)

    get api_v1_classrooms_path,
      headers: { 'Authorization' => "Bearer #{token}" }

    assert_response :unauthorized
    assert_includes response.body, 'expired'
  end

  test "should reject invalid JWT" do
    get api_v1_classrooms_path,
      headers: { 'Authorization' => "Bearer invalid-token" }

    assert_response :unauthorized
  end

  test "should fall back to session when enabled" do
    ENV['JWT_FALLBACK_TO_SESSION'] = 'true'

    # Set session
    post teachers_login_path, params: {
      teacher: { email: @user.email, password: 'password123' }
    }

    # Access protected endpoint without JWT
    get api_v1_classrooms_path
    assert_response :success

    ENV['JWT_FALLBACK_TO_SESSION'] = 'false'
  end

  private

  def generate_jwt(user, exp: 6.hours.from_now)
    payload = {
      user_id: user.id,
      boddle_uid: user.boddle_uid,
      email: user.email,
      meta_type: user.meta_type,
      meta_id: user.meta_id,
      exp: exp.to_i,
      iat: Time.current.to_i,
      jti: SecureRandom.uuid
    }

    JWT.encode(payload, @jwt_secret, 'HS256')
  end
end
```

---

## Migration Checklist

### Phase 1: Dual Authentication (Week 8)

- [ ] Add `jwt` gem to Gemfile
- [ ] Add `redis` and `faraday` gems
- [ ] Create `app/middleware/jwt_auth.rb`
- [ ] Register middleware in `config/application.rb`
- [ ] Update `ApplicationController#current_user`
- [ ] Update API controllers (Grape)
- [ ] Create `app/controllers/auth_controller.rb`
- [ ] Add migration endpoint route
- [ ] Configure Redis connection
- [ ] Set environment variables:
  - [ ] `JWT_SECRET_KEY` (same as Go Gateway)
  - [ ] `JWT_FALLBACK_TO_SESSION=true`
  - [ ] `REDIS_URL`
  - [ ] `AUTH_GATEWAY_URL`
- [ ] Write integration tests
- [ ] Deploy to staging
- [ ] Test both cookie and JWT authentication
- [ ] Deploy to production

### Phase 2: Gradual Rollout (Week 9)

- [ ] Set `JWT_ROLLOUT_PERCENTAGE=10`
- [ ] Monitor error rates and performance
- [ ] Increase to `25`, then `50`, then `75`
- [ ] Monitor user login success rates
- [ ] Check Redis performance
- [ ] Verify JWT validation speed

### Phase 3: JWT-Only (Week 10)

- [ ] Set `JWT_ROLLOUT_PERCENTAGE=100`
- [ ] Set `JWT_FALLBACK_TO_SESSION=false`
- [ ] Update login routes to redirect to Go Gateway
- [ ] Stop generating login tokens in Rails
- [ ] Monitor for 7 days

### Phase 4: Cleanup (Week 11-12)

- [ ] Remove old authentication code from controllers
- [ ] Remove session-based authentication logic
- [ ] Remove unused login routes
- [ ] Remove `TeachersController#login_create`
- [ ] Remove `StudentsController#login_create`
- [ ] Remove reCAPTCHA verification (moved to Go)
- [ ] Remove rate limiting code (moved to Go)
- [ ] Update documentation
- [ ] Remove feature flags

---

## Testing Strategy

### Manual Testing

1. **JWT Authentication**:
   ```bash
   # Get JWT from Go Gateway
   curl -X POST http://localhost:8080/auth/login \
     -H "Content-Type: application/json" \
     -d '{"email":"teacher@example.com","password":"password123"}'

   # Use JWT with Rails
   curl http://localhost:3000/api/v1/classrooms \
     -H "Authorization: Bearer <JWT_TOKEN>"
   ```

2. **Session Migration**:
   ```bash
   # Login with cookie
   curl -X POST http://localhost:3000/teachers/login \
     -c cookies.txt \
     -d "teacher[email]=teacher@example.com&teacher[password]=password123"

   # Migrate to JWT
   curl -X POST http://localhost:3000/auth/migrate-to-jwt \
     -b cookies.txt
   ```

3. **Dual Authentication**:
   - Login with Go Gateway (get JWT)
   - Login with Rails (get session cookie)
   - Verify both work for protected endpoints

### Automated Testing

Run integration tests:
```bash
bundle exec rails test:integration
```

### Performance Testing

- Load test JWT validation: `ab -n 10000 -c 100 -H "Authorization: Bearer <JWT>" http://localhost:3000/api/v1/classrooms`
- Compare to session-based: `ab -n 10000 -c 100 -b cookies.txt http://localhost:3000/api/v1/classrooms`

---

## Rollback Plan

If issues arise:

1. **Immediate Rollback** (< 5 minutes):
   ```bash
   # Set environment variable
   export JWT_FALLBACK_TO_SESSION=true
   export JWT_ROLLOUT_PERCENTAGE=0

   # Restart Rails
   systemctl restart puma
   ```

2. **Code Rollback**:
   - Revert PR that added JWT middleware
   - Remove middleware registration from `config/application.rb`
   - Deploy previous version

3. **Database**:
   - No rollback needed (no schema changes)

4. **Go Gateway**:
   - Stop routing traffic to Go Gateway
   - Rails continues to handle all authentication

---

## Security Considerations

### JWT Secret Management

- **Generate strong secret**: `openssl rand -hex 64`
- **Store securely**: Use environment variables, not hardcoded
- **Rotate regularly**: Plan for secret rotation (requires coordination with Go Gateway)
- **Same secret**: Rails and Go Gateway must share identical secret

### Redis Security

- **Use authentication**: Set `REDIS_PASSWORD`
- **Network isolation**: Redis should not be publicly accessible
- **TLS**: Use `rediss://` URL scheme for encrypted connections

### CSRF Protection

- **JWT requests**: CSRF protection disabled (stateless)
- **Session requests**: CSRF protection remains enabled
- **Cookie-based sessions**: Continue using Rails CSRF tokens

---

## Performance Impact

### Expected Changes

| Metric | Current (Session) | With JWT |
|--------|------------------|----------|
| Auth Check Time | ~5-10ms (DB lookup) | ~1-2ms (signature verification) |
| Database Load | Session per auth check | No DB lookup needed |
| Redis Load | N/A | Blacklist check (~1ms) |
| Scalability | Session affinity required | Stateless, easy scaling |

### Monitoring

Monitor these metrics after deployment:

- Request latency (should improve)
- Database connection pool usage (should decrease)
- Redis connection pool usage (should increase slightly)
- Authentication failure rate (should remain same)
- Error rates (should remain same)

---

## Summary of Files Changed

### New Files
- `app/middleware/jwt_auth.rb`
- `app/controllers/auth_controller.rb`
- `config/initializers/redis.rb`
- `test/integration/jwt_auth_test.rb`

### Modified Files
- `config/application.rb` (register middleware)
- `config/routes.rb` (add migration endpoint)
- `app/controllers/application_controller.rb` (update `current_user`)
- `app/controllers/api/v1/defaults.rb` (update API auth)
- `app/controllers/teachers_controller.rb` (add rollout logic)
- `Gemfile` (add gems)
- `.env` (environment variables)

### No Changes Required
- Database schema (unchanged)
- User models (unchanged)
- Business logic (unchanged)
- Views (unchanged, except login redirect in Phase 3)
