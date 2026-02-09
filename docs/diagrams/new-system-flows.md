# New System Authentication Flow Diagrams

This document contains detailed sequence diagrams showing how authentication will work with the Go Authentication Gateway using JWT tokens.

## Table of Contents

1. [Email/Password Login with JWT](#emailpassword-login-with-jwt)
2. [Google OAuth2 with JWT](#google-oauth2-with-jwt)
3. [JWT Validation in Rails](#jwt-validation-in-rails)
4. [Token Refresh Flow](#token-refresh-flow)
5. [Logout and Token Revocation](#logout-and-token-revocation)
6. [Session Migration Flow](#session-migration-flow)

---

## Email/Password Login with JWT

### Complete Flow: Client → Go Gateway → Rails

```mermaid
sequenceDiagram
    participant Browser
    participant GoGateway as Go Auth Gateway
    participant Redis
    participant DB as PostgreSQL (Shared)
    participant Rails as Rails LMS

    Note over Browser,Rails: Phase 1: Authentication

    Browser->>GoGateway: POST /auth/login<br/>{email, password}

    GoGateway->>Redis: INCR ratelimit:login:192.168.1.1:user@example.com
    Redis-->>GoGateway: attempt_count = 3

    alt Rate limited (>= 5 attempts)
        Redis-->>GoGateway: attempt_count >= 5
        GoGateway->>Redis: TTL ratelimit:login:192.168.1.1:user@example.com
        Redis-->>GoGateway: ttl = 420 seconds (7 minutes remaining)
        GoGateway-->>Browser: 429 Too Many Requests<br/>{error: "Rate limited", retry_after: 420}
    else Allowed
        GoGateway->>GoGateway: Normalize email (downcase, strip)

        GoGateway->>DB: SELECT * FROM users<br/>WHERE LOWER(email) = $1
        DB-->>GoGateway: User record

        alt User not found
            GoGateway->>DB: INSERT INTO login_attempts<br/>(ip_address, blocked=false)
            GoGateway-->>Browser: 401 Unauthorized
        else User found
            GoGateway->>GoGateway: bcrypt.CompareHashAndPassword(<br/>  user.password_digest,<br/>  password<br/>)

            alt Password incorrect
                GoGateway->>DB: INSERT INTO login_attempts<br/>(user_id, ip_address, blocked=false)
                GoGateway-->>Browser: 401 Unauthorized<br/>{error: "Invalid credentials"}
            else Password correct
                GoGateway->>DB: UPDATE users<br/>SET last_logged_on = NOW()

                GoGateway->>GoGateway: Generate JWT<br/>claims = {<br/>  user_id, boddle_uid, email,<br/>  meta_type, meta_id,<br/>  exp: now + 6 hours,<br/>  jti: uuid<br/>}

                GoGateway->>GoGateway: Sign JWT with HS256<br/>token = jwt.Sign(claims, SECRET_KEY)

                GoGateway->>DB: INSERT INTO refresh_tokens<br/>(user_id, token, expires_at)

                GoGateway-->>Browser: 200 OK<br/>{<br/>  token: "eyJhbGc...",<br/>  refresh_token: "...",<br/>  expires_at: "2024-03-15T18:00:00Z",<br/>  user: {id, email, name, meta_type}<br/>}

                Note over Browser: Store JWT in localStorage

                Browser->>Browser: localStorage.setItem('jwt_token', token)
                Browser->>Browser: localStorage.setItem('jwt_expires_at', expires_at)
            end
        end
    end

    Note over Browser,Rails: Phase 2: Using JWT for API Requests

    Browser->>Rails: GET /api/v1/classrooms<br/>Authorization: Bearer eyJhbGc...

    Rails->>Rails: JWTMiddleware extracts token

    Rails->>Rails: JWT.decode(token, SECRET_KEY, HS256)

    alt Invalid signature
        Rails-->>Browser: 401 Unauthorized<br/>{error: "Invalid token"}
    else Valid signature
        Rails->>Rails: Check expiry (exp claim)

        alt Token expired
            Rails-->>Browser: 401 Unauthorized<br/>{error: "Token expired"}
        else Not expired
            Rails->>Redis: EXISTS blacklist:jti:TOKEN_ID
            Redis-->>Rails: 0 (not blacklisted)

            Rails->>Rails: Extract claims:<br/>user_id = payload['user_id']

            Rails->>Rails: @current_user = User.find(user_id)

            Rails->>DB: SELECT * FROM class_rooms<br/>WHERE teacher_id = ?
            DB-->>Rails: Classroom data

            Rails-->>Browser: 200 OK<br/>{classrooms: [...]}
        end
    end
```

**Key Improvements Over Current System**:

1. **Stateless**: No session storage needed
2. **Fast Rate Limiting**: Redis INCR (O(1) operation)
3. **Horizontal Scaling**: No session affinity required
4. **Cross-Service**: JWT works for Rails, game client, mobile apps
5. **Performance**: JWT validation ~1-2ms vs session lookup ~5-10ms

**Go Handler Code** (simplified):

```go
func (h *AuthHandler) Login(c *gin.Context) {
    var req LoginRequest
    c.BindJSON(&req)

    // Rate limit check
    if h.rateLimiter.IsBlocked(c.ClientIP(), req.Email) {
        c.JSON(429, gin.H{"error": "Rate limited"})
        return
    }

    // Find user
    user, err := h.userRepo.FindByEmail(req.Email)
    if err != nil {
        h.rateLimiter.RecordAttempt(c.ClientIP(), req.Email)
        c.JSON(401, gin.H{"error": "Invalid credentials"})
        return
    }

    // Verify password
    if !bcrypt.CompareHashAndPassword(user.PasswordDigest, req.Password) {
        h.rateLimiter.RecordAttempt(c.ClientIP(), req.Email)
        c.JSON(401, gin.H{"error": "Invalid credentials"})
        return
    }

    // Generate JWT
    token, err := h.jwtService.Generate(JWTClaims{
        UserID:    user.ID,
        BoddleUID: user.BoddleUID,
        Email:     user.Email,
        MetaType:  user.MetaType,
        MetaID:    user.MetaID,
    })

    // Update last login
    h.userRepo.UpdateLastLoggedOn(user.ID)

    c.JSON(200, gin.H{
        "token":         token,
        "expires_at":    time.Now().Add(6 * time.Hour),
        "refresh_token": refreshToken,
        "user":          user,
    })
}
```

---

## Google OAuth2 with JWT

### OAuth Flow Ending with JWT Token

```mermaid
sequenceDiagram
    participant Browser
    participant GoGateway as Go Auth Gateway
    participant Google as Google OAuth2
    participant Redis
    participant DB as PostgreSQL
    participant Rails as Rails LMS

    Browser->>GoGateway: GET /auth/google

    GoGateway->>GoGateway: state = SecureRandom.uuid()

    GoGateway->>Redis: SET oauth:state:STATE_ID "pending" EX 600<br/>(10 minute expiry)

    GoGateway-->>Browser: 302 Redirect to Google<br/>https://accounts.google.com/o/oauth2/auth?<br/>  client_id=...&<br/>  redirect_uri=.../auth/google/callback&<br/>  state=STATE_ID&<br/>  scope=email+profile

    Browser->>Google: Authorization request

    Google-->>Browser: Google consent screen

    Browser->>Google: User grants permission

    Google-->>Browser: 302 Redirect to callback<br/>?code=AUTH_CODE&state=STATE_ID

    Browser->>GoGateway: GET /auth/google/callback?code=AUTH_CODE&state=STATE_ID

    GoGateway->>Redis: GET oauth:state:STATE_ID
    Redis-->>GoGateway: "pending"

    GoGateway->>Redis: DEL oauth:state:STATE_ID

    alt State mismatch or expired
        GoGateway-->>Browser: 401 Unauthorized<br/>{error: "Invalid OAuth state (CSRF)"}
    else State valid
        GoGateway->>Google: POST /oauth/token<br/>{<br/>  code: AUTH_CODE,<br/>  client_id: ...,<br/>  client_secret: ...,<br/>  redirect_uri: ...<br/>}

        Google-->>GoGateway: {<br/>  access_token: "ya29...",<br/>  refresh_token: "...",<br/>  id_token: "eyJ..."<br/>}

        GoGateway->>Google: GET /oauth2/v2/userinfo<br/>Authorization: Bearer ya29...

        Google-->>GoGateway: {<br/>  sub: "google_user_id",<br/>  email: "user@example.com",<br/>  name: "John Doe",<br/>  picture: "https://..."<br/>}

        GoGateway->>DB: SELECT t.*, u.* FROM teachers t<br/>JOIN users u ON u.meta_id = t.id<br/>WHERE t.google_uid = $1

        alt Teacher with google_uid exists
            DB-->>GoGateway: Existing teacher record
            GoGateway->>DB: UPDATE users SET last_logged_on = NOW()

        else Check email match
            GoGateway->>DB: SELECT * FROM users<br/>WHERE email = $1 AND meta_type = 'Teacher'

            alt Email matches existing teacher
                DB-->>GoGateway: Existing user
                GoGateway->>DB: UPDATE teachers<br/>SET google_uid = $1<br/>WHERE id = $2
                Note over GoGateway,DB: Link Google account

            else Create new teacher
                GoGateway->>DB: BEGIN TRANSACTION
                GoGateway->>DB: INSERT INTO teachers<br/>(first_name, last_name, google_uid, is_verified=true)
                GoGateway->>DB: INSERT INTO users<br/>(email, name, password_digest, boddle_uid,<br/>meta_type='Teacher', meta_id)
                GoGateway->>DB: COMMIT
            end
        end

        GoGateway->>GoGateway: Generate JWT<br/>claims = {user_id, email, meta_type, exp, jti}

        GoGateway->>GoGateway: Sign JWT with HS256

        GoGateway->>DB: INSERT INTO refresh_tokens<br/>(user_id, token, expires_at)

        GoGateway-->>Browser: 302 Redirect to frontend<br/>https://app.boddle.com/auth/callback?token=eyJhbGc...

        Note over Browser: Frontend extracts token from URL

        Browser->>Browser: const params = new URLSearchParams(location.search)<br/>const token = params.get('token')<br/>localStorage.setItem('jwt_token', token)

        Browser->>Browser: Redirect to dashboard

        Browser->>Rails: GET /teacher_home<br/>Authorization: Bearer eyJhbGc...

        Rails->>Rails: Validate JWT (see next diagram)

        Rails-->>Browser: 200 OK (dashboard HTML)
    end
```

**OAuth State Management**:
- **Storage**: Redis with 10-minute TTL
- **Purpose**: CSRF protection
- **Format**: UUID v4
- **Key**: `oauth:state:{uuid}`

**Account Linking Logic** (same as current system):
1. Match by `google_uid` (direct link)
2. Match by email (link to existing account)
3. Create new account

**Frontend Token Handling**:

```javascript
// Extract token from OAuth redirect
const urlParams = new URLSearchParams(window.location.search);
const token = urlParams.get('token');

if (token) {
    // Store JWT
    localStorage.setItem('jwt_token', token);

    // Parse to get expiry
    const payload = JSON.parse(atob(token.split('.')[1]));
    localStorage.setItem('jwt_expires_at', payload.exp);

    // Clean up URL
    window.history.replaceState({}, document.title, window.location.pathname);

    // Redirect to dashboard
    window.location.href = '/dashboard';
}
```

---

## JWT Validation in Rails

### How Rails Validates Go-Issued JWTs

```mermaid
sequenceDiagram
    participant Client
    participant Rack as Rack Middleware
    participant JWT as JWT Middleware
    participant Redis
    participant Controller as Rails Controller
    participant DB as PostgreSQL

    Client->>Rack: GET /api/v1/classrooms<br/>Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...

    Rack->>JWT: JwtAuth.call(env)

    JWT->>JWT: Extract token from header<br/>auth_header.split(' ').last

    alt No token provided
        JWT->>JWT: Check if fallback_to_session?<br/>ENV['JWT_FALLBACK_TO_SESSION']

        alt Fallback enabled
            JWT->>Rack: Continue (session auth will run)
            Note over JWT,Controller: Allow old session-based auth
        else Fallback disabled
            JWT-->>Client: 401 Unauthorized<br/>{error: "Missing authentication token"}
        end
    else Token provided
        JWT->>JWT: JWT.decode(token, SECRET_KEY, true, {algorithm: 'HS256'})

        alt Invalid signature
            JWT-->>Client: 401 Unauthorized<br/>{error: "Invalid token"}
        else Valid signature
            JWT->>JWT: payload = decoded_token[0]<br/>Extract claims: user_id, boddle_uid, exp, jti

            JWT->>JWT: Check expiry<br/>Time.at(payload['exp']) > Time.now

            alt Token expired
                JWT-->>Client: 401 Unauthorized<br/>{error: "Token expired"}
            else Not expired
                JWT->>Redis: EXISTS blacklist:jti:#{jti}
                Redis-->>JWT: 0 (not revoked)

                alt Token revoked
                    Redis-->>JWT: 1 (revoked)
                    JWT-->>Client: 401 Unauthorized<br/>{error: "Token revoked"}
                else Token valid
                    JWT->>DB: SELECT * FROM users WHERE id = $1
                    DB-->>JWT: User record

                    alt User not found
                        JWT-->>Client: 401 Unauthorized<br/>{error: "User not found"}
                    else User found
                        JWT->>JWT: Verify boddle_uid matches<br/>(security check)

                        alt UID mismatch
                            JWT-->>Client: 401 Unauthorized<br/>{error: "Invalid token claims"}
                        else UID matches
                            JWT->>JWT: Store in env<br/>env['current_user'] = user<br/>env['jwt_payload'] = payload

                            JWT->>Controller: Continue request

                            Controller->>Controller: current_user = request.env['current_user']

                            Controller->>DB: SELECT * FROM classrooms<br/>WHERE teacher_id = ?

                            DB-->>Controller: Classroom data

                            Controller-->>Client: 200 OK<br/>{classrooms: [...]}
                        end
                    end
                end
            end
        end
    end
```

**JWT Validation Timeline**:

```
Request arrives
    ↓
Extract token from Authorization header
    ↓
Verify JWT signature (HS256 with shared secret)
    ↓ (~1ms)
Check token expiry (exp claim)
    ↓ (<1ms)
Check Redis blacklist (if logged out)
    ↓ (~1ms)
Load user from database (for authorization)
    ↓ (~3-5ms)
Inject current_user into request
    ↓
Continue to controller
    ↓
Total: ~5-8ms (vs 10-15ms for session lookup)
```

**Rails Middleware Code**:

```ruby
# app/middleware/jwt_auth.rb
class JwtAuth
  def call(env)
    request = Rack::Request.new(env)

    # Skip for public paths
    return @app.call(env) if skip_jwt_validation?(request.path)

    token = extract_token(request)

    if token
      begin
        # Decode and verify
        payload = JWT.decode(
          token,
          ENV['JWT_SECRET_KEY'],
          true,
          { algorithm: 'HS256' }
        ).first

        # Check blacklist
        if token_blacklisted?(payload['jti'])
          return unauthorized_response('Token revoked')
        end

        # Load user
        user = User.find_by(id: payload['user_id'])
        return unauthorized_response('User not found') unless user

        # Verify claims
        return unauthorized_response('Invalid claims') unless user.boddle_uid == payload['boddle_uid']

        # Inject into request
        env['current_user'] = user
        env['jwt_payload'] = payload

      rescue JWT::ExpiredSignature
        return unauthorized_response('Token expired')
      rescue JWT::DecodeError
        return unauthorized_response('Invalid token')
      end
    elsif fallback_to_session?
      # Allow session-based auth during migration
    else
      return unauthorized_response('Missing token')
    end

    @app.call(env)
  end
end
```

---

## Token Refresh Flow

### Obtaining New Access Token with Refresh Token

```mermaid
sequenceDiagram
    participant Frontend
    participant GoGateway as Go Auth Gateway
    participant DB as PostgreSQL
    participant Redis

    Note over Frontend: Access token expired after 6 hours

    Frontend->>Frontend: Try API request with expired token

    Frontend->>GoGateway: GET /api/v1/data<br/>Authorization: Bearer EXPIRED_TOKEN

    GoGateway->>GoGateway: Validate JWT

    GoGateway-->>Frontend: 401 Unauthorized<br/>{error: "Token expired", code: "token_expired"}

    Note over Frontend: Detect expired token, use refresh token

    Frontend->>Frontend: refreshToken = localStorage.getItem('refresh_token')

    Frontend->>GoGateway: POST /auth/refresh<br/>{refresh_token: "..."}

    GoGateway->>DB: SELECT * FROM refresh_tokens<br/>WHERE token = $1 AND revoked = false

    alt Refresh token not found or revoked
        DB-->>GoGateway: NULL or revoked=true
        GoGateway-->>Frontend: 401 Unauthorized<br/>{error: "Invalid refresh token", code: "refresh_token_invalid"}
        Note over Frontend: Redirect to login page
    else Refresh token valid
        DB-->>GoGateway: Refresh token record

        GoGateway->>GoGateway: Check expiry<br/>expires_at > NOW()

        alt Refresh token expired
            GoGateway->>DB: UPDATE refresh_tokens<br/>SET revoked = true WHERE id = $1
            GoGateway-->>Frontend: 401 Unauthorized<br/>{error: "Refresh token expired", code: "refresh_token_expired"}
            Note over Frontend: Redirect to login page
        else Refresh token not expired
            GoGateway->>DB: SELECT * FROM users WHERE id = $1
            DB-->>GoGateway: User record

            GoGateway->>GoGateway: Generate new access token<br/>claims = {user_id, email, meta_type,<br/>exp: now + 6 hours, jti: uuid}

            GoGateway->>GoGateway: Sign JWT with HS256

            Note over GoGateway: Optionally rotate refresh token

            alt Rotate refresh token
                GoGateway->>DB: UPDATE refresh_tokens<br/>SET revoked = true WHERE id = $1
                GoGateway->>DB: INSERT INTO refresh_tokens<br/>(user_id, token, expires_at = now + 30 days)
                DB-->>GoGateway: New refresh token
            end

            GoGateway-->>Frontend: 200 OK<br/>{<br/>  token: "eyJhbGc...",<br/>  refresh_token: "..." (if rotated),<br/>  expires_at: "2024-03-15T18:00:00Z"<br/>}

            Frontend->>Frontend: localStorage.setItem('jwt_token', new_token)
            Frontend->>Frontend: localStorage.setItem('jwt_expires_at', expires_at)

            Frontend->>Frontend: Retry original API request

            Frontend->>GoGateway: GET /api/v1/data<br/>Authorization: Bearer NEW_TOKEN

            GoGateway-->>Frontend: 200 OK {data: [...]}
        end
    end
```

**Token Lifetimes**:
- **Access Token**: 6 hours
- **Refresh Token**: 30 days

**Refresh Token Rotation**:
- **Option 1**: Keep same refresh token (simpler)
- **Option 2**: Issue new refresh token on each refresh (more secure)

**Frontend Auto-Refresh Logic**:

```javascript
// Axios interceptor for automatic token refresh
axios.interceptors.response.use(
    response => response,
    async error => {
        const originalRequest = error.config;

        if (error.response.status === 401 &&
            error.response.data.code === 'token_expired' &&
            !originalRequest._retry) {

            originalRequest._retry = true;

            try {
                const refreshToken = localStorage.getItem('refresh_token');
                const response = await axios.post('/auth/refresh', {
                    refresh_token: refreshToken
                });

                const { token, refresh_token, expires_at } = response.data;

                localStorage.setItem('jwt_token', token);
                localStorage.setItem('jwt_expires_at', expires_at);
                if (refresh_token) {
                    localStorage.setItem('refresh_token', refresh_token);
                }

                // Retry original request with new token
                originalRequest.headers['Authorization'] = `Bearer ${token}`;
                return axios(originalRequest);

            } catch (refreshError) {
                // Refresh failed, redirect to login
                localStorage.clear();
                window.location.href = '/login';
                return Promise.reject(refreshError);
            }
        }

        return Promise.reject(error);
    }
);
```

---

## Logout and Token Revocation

### Blacklist Token to Prevent Further Use

```mermaid
sequenceDiagram
    participant Frontend
    participant GoGateway as Go Auth Gateway
    participant Redis
    participant DB as PostgreSQL

    Note over Frontend: User clicks "Logout"

    Frontend->>GoGateway: POST /auth/logout<br/>Authorization: Bearer eyJhbGc...

    GoGateway->>GoGateway: Decode JWT to extract claims<br/>(no signature verification needed)

    GoGateway->>GoGateway: Extract jti (JWT ID) and exp (expiry)<br/>jti = payload['jti']<br/>exp = payload['exp']

    GoGateway->>GoGateway: Calculate TTL<br/>ttl = exp - now<br/>(time until token expires naturally)

    GoGateway->>Redis: SET blacklist:jti:JTI_VALUE "revoked" EX ttl

    Note over Redis: Token blacklisted until it would<br/>have expired anyway

    GoGateway->>DB: UPDATE refresh_tokens<br/>SET revoked = true<br/>WHERE user_id = $1

    Note over DB: Revoke all refresh tokens for this user<br/>(forces re-login on all devices)

    GoGateway-->>Frontend: 200 OK<br/>{message: "Logged out successfully"}

    Frontend->>Frontend: Clear local storage<br/>localStorage.removeItem('jwt_token')<br/>localStorage.removeItem('refresh_token')<br/>localStorage.removeItem('jwt_expires_at')

    Frontend->>Frontend: Redirect to login page<br/>window.location.href = '/login'

    Note over Frontend,GoGateway: Verify token is blacklisted

    Frontend->>GoGateway: GET /api/v1/classrooms<br/>Authorization: Bearer eyJhbGc...<br/>(using revoked token)

    GoGateway->>GoGateway: Verify JWT signature (valid)

    GoGateway->>GoGateway: Check expiry (not expired yet)

    GoGateway->>Redis: EXISTS blacklist:jti:JTI_VALUE

    Redis-->>GoGateway: 1 (key exists = token blacklisted)

    GoGateway-->>Frontend: 401 Unauthorized<br/>{error: "Token revoked"}

    Frontend->>Frontend: Redirect to login (already there)
```

**Blacklist Key Design**:
- **Key Format**: `blacklist:jti:{token_id}`
- **Value**: `"revoked"` (or timestamp of revocation)
- **TTL**: Remaining time until token expires naturally
- **Why TTL**: No need to keep blacklist entry after token would expire anyway

**Logout Scenarios**:

1. **Single Device Logout**:
   - Blacklist current JWT (jti)
   - Keep refresh tokens for other devices

2. **All Devices Logout**:
   - Blacklist current JWT
   - Revoke ALL refresh tokens for user
   - User must re-login everywhere

**Admin-Initiated Logout** (ban user):

```mermaid
sequenceDiagram
    participant Admin
    participant GoGateway as Go Auth Gateway
    participant Redis
    participant DB as PostgreSQL

    Admin->>GoGateway: POST /admin/users/:id/revoke-all-tokens<br/>Authorization: Bearer ADMIN_JWT

    GoGateway->>GoGateway: Verify admin permissions

    GoGateway->>DB: UPDATE refresh_tokens<br/>SET revoked = true<br/>WHERE user_id = $1

    GoGateway->>Redis: SET user:banned:USER_ID "true" EX 86400<br/>(24 hour cache)

    Note over Redis: Mark user as banned to reject<br/>new tokens during validation

    GoGateway-->>Admin: 200 OK<br/>{message: "All tokens revoked"}

    Note over GoGateway,Redis: Existing JWTs still valid until expiry,<br/>but refresh tokens revoked

    Note over GoGateway: On next JWT validation for this user:

    GoGateway->>Redis: GET user:banned:USER_ID
    Redis-->>GoGateway: "true"
    GoGateway-->>Frontend: 401 Unauthorized<br/>{error: "Account suspended"}
```

---

## Session Migration Flow

### Convert Existing Cookie Session to JWT

```mermaid
sequenceDiagram
    participant Browser
    participant Rails as Rails LMS
    participant GoGateway as Go Auth Gateway
    participant DB as PostgreSQL

    Note over Browser,Rails: User has active cookie session from old system

    Browser->>Rails: GET /dashboard<br/>Cookie: _lms_session=encrypted_session

    Rails->>Rails: Decrypt session cookie<br/>session_data = {user_id: 123}

    Rails->>Rails: @current_user = User.find(123)

    Rails-->>Browser: 200 OK (dashboard page)

    Note over Browser: Frontend detects old session,<br/>prompts migration to JWT

    Browser->>Browser: if (!localStorage.getItem('jwt_token')) {<br/>  // Migrate to JWT<br/>}

    Browser->>Rails: POST /auth/migrate-to-jwt<br/>Cookie: _lms_session=encrypted_session

    Rails->>Rails: Check current_user (from session)

    alt Not authenticated
        Rails-->>Browser: 401 Unauthorized
    else Authenticated
        Rails->>Rails: Prepare user claims<br/>{user_id, boddle_uid, email, meta_type, meta_id}

        Rails->>GoGateway: POST /internal/issue-jwt<br/>X-Internal-Token: SHARED_SECRET<br/>{<br/>  user_id: 123,<br/>  boddle_uid: "uuid",<br/>  email: "user@example.com",<br/>  meta_type: "Teacher",<br/>  meta_id: 456<br/>}

        GoGateway->>GoGateway: Verify internal token<br/>(prevent unauthorized JWT issuance)

        alt Invalid internal token
            GoGateway-->>Rails: 403 Forbidden
            Rails-->>Browser: 500 Internal Server Error
        else Valid internal token
            GoGateway->>GoGateway: Generate JWT<br/>claims = {user_id, boddle_uid, email,<br/>meta_type, meta_id, exp, jti}

            GoGateway->>GoGateway: Sign JWT with HS256

            GoGateway->>DB: INSERT INTO refresh_tokens<br/>(user_id, token, expires_at)

            GoGateway-->>Rails: 200 OK<br/>{<br/>  token: "eyJhbGc...",<br/>  refresh_token: "...",<br/>  expires_at: "2024-03-15T18:00:00Z"<br/>}

            Rails-->>Browser: 200 OK<br/>{<br/>  token: "eyJhbGc...",<br/>  refresh_token: "...",<br/>  expires_at: "2024-03-15T18:00:00Z",<br/>  user: {id, email, name}<br/>}

            Browser->>Browser: Store JWT<br/>localStorage.setItem('jwt_token', token)<br/>localStorage.setItem('refresh_token', refresh_token)<br/>localStorage.setItem('jwt_expires_at', expires_at)

            Browser->>Browser: // Optionally clear session cookie<br/>// (if configured)

            Note over Browser: Future requests use JWT instead of cookie

            Browser->>Rails: GET /api/v1/classrooms<br/>Authorization: Bearer eyJhbGc...

            Rails->>Rails: JWT validation (see previous diagram)

            Rails-->>Browser: 200 OK {classrooms: [...]}
        end
    end
```

**Migration Trigger Options**:

1. **Automatic Migration**:
   ```javascript
   // Frontend automatically migrates on first load
   if (hasSessionCookie() && !hasJWT()) {
       await migrateToJWT();
   }
   ```

2. **User-Initiated**:
   ```javascript
   // Show banner: "Switch to new login system"
   <button onClick={migrateToJWT}>Update Login Method</button>
   ```

3. **Forced Migration**:
   ```javascript
   // After certain date, force all users to migrate
   if (Date.now() > MIGRATION_DEADLINE && !hasJWT()) {
       await migrateToJWT();
   }
   ```

**Security Considerations**:

1. **Internal Token**: Prevent unauthorized JWT issuance
   - Shared secret between Rails and Go Gateway
   - Only Rails can request JWT issuance
   - Not exposed to clients

2. **Session Clearing**: Optional
   - Can keep session during transition (safer)
   - Or clear session after JWT issued (cleaner)

3. **Audit Trail**:
   - Log all session→JWT migrations
   - Track which users have migrated
   - Monitor for issues

---

## Architecture Comparison

### Side-by-Side: Current vs New

```mermaid
graph LR
    subgraph "Current System"
        A1[Client] -->|1. Login| B1[Rails Login Controller]
        B1 -->|2. Query| C1[PostgreSQL]
        C1 -->|3. User Data| B1
        B1 -->|4. Set Cookie| A1
        A1 -->|5. Request + Cookie| D1[Rails API]
        D1 -->|6. Session Lookup| C1
        C1 -->|7. User Data| D1
        D1 -->|8. Response| A1

        style B1 fill:#e1f5fe
        style D1 fill:#e1f5fe
        style C1 fill:#fff9c4
    end

    subgraph "New System"
        A2[Client] -->|1. Login| B2[Go Gateway]
        B2 -->|2. Query| C2[PostgreSQL]
        C2 -->|3. User Data| B2
        B2 -->|4. Return JWT| A2
        A2 -->|5. Request + JWT| D2[Rails API]
        D2 -->|6. Validate JWT| E2[Redis]
        E2 -->|7. Not Blacklisted| D2
        D2 -->|8. Response| A2

        style B2 fill:#c8e6c9
        style D2 fill:#e1f5fe
        style C2 fill:#fff9c4
        style E2 fill:#ffccbc
    end
```

**Performance Comparison**:

| Operation | Current (Cookie/Session) | New (JWT) | Improvement |
|-----------|-------------------------|-----------|-------------|
| Login | ~50-100ms | ~30-50ms | 40% faster |
| Auth Check | ~5-10ms (DB lookup) | ~1-2ms (signature verify) | 5x faster |
| Rate Limit | ~10-20ms (DB query) | ~1ms (Redis) | 10x faster |
| Logout | Clear cookie (~1ms) | Redis SET (~1ms) | Same |
| Scaling | Session affinity required | Stateless, any server | ∞ better |
| Cross-service | Requires shared DB | JWT works everywhere | N/A |

**Benefits Summary**:

1. **Performance**: 5x faster authentication checks
2. **Scalability**: Stateless, no session affinity
3. **Security**: Token revocation, rate limiting in Redis
4. **Flexibility**: Works for web, mobile, game clients
5. **Maintainability**: Centralized auth logic in Go Gateway
6. **Monitoring**: Better observability (Prometheus metrics)

---

## Next Steps

To implement these flows:

1. **Phase 1**: Build Go Gateway (email/password + JWT)
2. **Phase 2**: Add JWT validation to Rails (dual auth mode)
3. **Phase 3**: Implement OAuth providers in Go Gateway
4. **Phase 4**: Add refresh tokens and token revocation
5. **Phase 5**: Migrate existing sessions to JWT
6. **Phase 6**: Remove old authentication code

See [Implementation Plan](../../README.md) for detailed timeline.
