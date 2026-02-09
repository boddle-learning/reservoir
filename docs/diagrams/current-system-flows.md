# Current System Authentication Flow Diagrams

This document contains detailed sequence diagrams showing how authentication currently works in the Rails LMS application.

## Table of Contents

1. [Email/Password Login Flow](#emailpassword-login-flow)
2. [Google OAuth2 Flow](#google-oauth2-flow)
3. [Clever SSO Flow](#clever-sso-flow)
4. [Login Token (Magic Link) Flow](#login-token-magic-link-flow)
5. [Session Validation Flow](#session-validation-flow)
6. [Rate Limiting Flow](#rate-limiting-flow)

---

## Email/Password Login Flow

### Teacher/Parent Login

```mermaid
sequenceDiagram
    participant Browser
    participant Rails as Rails LMS
    participant reCAPTCHA as Google reCAPTCHA
    participant DB as PostgreSQL
    participant Session as Cookie Session

    Browser->>Rails: GET /teachers/login
    Rails-->>Browser: Login form

    Browser->>Rails: POST /teachers/login<br/>{email, password, recaptcha_token}

    Rails->>reCAPTCHA: verify_recaptcha(token)
    reCAPTCHA-->>Rails: Verification result

    alt reCAPTCHA failed
        Rails-->>Browser: 403 Redirect with error<br/>"reCAPTCHA verification failed"
    else reCAPTCHA passed
        Rails->>DB: SELECT COUNT(*) FROM login_attempts<br/>WHERE ip_address=? AND created_at > NOW() - 10 min
        DB-->>Rails: Attempt count

        alt Too many attempts (>= 5)
            Rails->>DB: SELECT MAX(created_at) FROM login_attempts<br/>WHERE ip_address=?
            DB-->>Rails: Last attempt time

            alt Within lockout period (15 min)
                Rails-->>Browser: 429 Redirect with error<br/>"Too many attempts. Try again in X minutes"
            end
        end

        Rails->>Rails: Normalize email (downcase, strip)
        Rails->>DB: SELECT * FROM users<br/>WHERE LOWER(email) = ?

        alt User not found
            DB-->>Rails: NULL
            Rails->>DB: INSERT INTO login_attempts<br/>(ip_address, blocked=false)
            Rails-->>Browser: 401 Redirect with error<br/>"Invalid email or password"
        else User found
            DB-->>Rails: User record

            Rails->>Rails: user.authenticate(password)<br/>(bcrypt comparison)

            alt Password incorrect
                Rails->>DB: INSERT INTO login_attempts<br/>(user_id, ip_address, blocked=false)
                Rails-->>Browser: 401 Redirect with error<br/>"Invalid email or password"
            else Password correct
                Rails->>DB: UPDATE users<br/>SET last_logged_on = NOW()
                Rails->>DB: INSERT INTO teacher_logins<br/>(user_id, login_via='EMAIL')

                Rails->>Session: session[:user_id] = user.id
                Session-->>Rails: Cookie created

                Rails-->>Browser: 302 Redirect to /teacher_home<br/>Set-Cookie: _lms_session=...

                Browser->>Rails: GET /teacher_home<br/>Cookie: _lms_session=...
                Rails->>Session: Find user by session[:user_id]
                Session-->>Rails: User object
                Rails-->>Browser: 200 Teacher dashboard
            end
        end
    end
```

**Key Files**:
- Controller: `app/controllers/teachers_controller.rb:768`
- Model: `app/models/user.rb`
- Helper: `app/helpers/admins_helper.rb#log_in`

**Security Features**:
1. **reCAPTCHA v3**: Bot protection
2. **Rate Limiting**: 5 attempts per 10 minutes
3. **Lockout**: 15-minute lockout after 5 failed attempts
4. **Case-insensitive email**: Normalized to lowercase
5. **bcrypt**: Password hashed with cost factor 10
6. **Audit trail**: `login_attempts` and `teacher_logins` tables

**Session Cookie**:
- **Name**: `_lms_session`
- **Expiry**: 6 hours
- **Secure**: Yes (production only)
- **HttpOnly**: Yes
- **SameSite**: Lax

---

### Student Login

```mermaid
sequenceDiagram
    participant Browser
    participant Rails as Rails LMS
    participant DB as PostgreSQL
    participant Session as Cookie Session

    Note over Browser,Session: Students use username@student.student format

    Browser->>Rails: GET /students/login
    Rails-->>Browser: Login form (username field)

    Browser->>Rails: POST /students/login<br/>{username, password}

    Rails->>Rails: Format username<br/>username → "username@student.student"

    Rails->>DB: SELECT * FROM users<br/>WHERE email = 'username@student.student'

    alt Student not found
        Rails-->>Browser: 401 "Invalid username or password"
    else Student found
        DB-->>Rails: User record (meta_type='Student')

        Rails->>Rails: user.authenticate(password)

        alt Password incorrect
            Rails-->>Browser: 401 "Invalid username or password"
        else Password correct
            Rails->>DB: UPDATE users SET last_logged_on = NOW()
            Rails->>Session: session[:user_id] = user.id
            Rails-->>Browser: 302 Redirect to /student_home<br/>Set-Cookie: _lms_session=...
        end
    end
```

**Key Differences from Teacher Login**:
- No reCAPTCHA required
- No rate limiting (students don't have email-based attack vector)
- Username format: `username@student.student`
- Simpler password requirements (minimum 3 characters vs 8)

---

## Google OAuth2 Flow

### Teacher OAuth Login

```mermaid
sequenceDiagram
    participant Browser
    participant Rails as Rails LMS
    participant Google as Google OAuth2
    participant DB as PostgreSQL
    participant Session as Cookie Session

    Browser->>Rails: GET /auth/google_oauth2
    Rails->>Rails: Generate OAuth state parameter

    Rails-->>Browser: 302 Redirect to Google
    Note over Rails,Browser: https://accounts.google.com/o/oauth2/auth?<br/>client_id=...&<br/>redirect_uri=.../teachers/create_google_oauth&<br/>scope=email+profile+classroom&<br/>access_type=offline

    Browser->>Google: Authorization request
    Google-->>Browser: Google consent screen

    Browser->>Google: User grants permission
    Google-->>Browser: 302 Redirect to callback<br/>?code=AUTH_CODE&state=...

    Browser->>Rails: GET /teachers/create_google_oauth?code=AUTH_CODE

    Rails->>Google: POST /oauth/token<br/>{code, client_id, client_secret, redirect_uri}
    Google-->>Rails: {access_token, refresh_token, id_token}

    Rails->>Google: GET /oauth2/v2/userinfo<br/>Authorization: Bearer {access_token}
    Google-->>Rails: {sub, email, name, picture}

    Rails->>DB: INSERT INTO sso_oauth_sessions<br/>(uid, email, name, provider='google')

    Rails->>DB: SELECT t.*, u.* FROM teachers t<br/>JOIN users u ON u.meta_id = t.id<br/>WHERE t.google_uid = ?

    alt Teacher with google_uid exists
        DB-->>Rails: Existing teacher record

        Rails->>DB: UPDATE users SET last_logged_on = NOW()
        Rails->>DB: INSERT INTO teacher_logins<br/>(user_id, login_via='GOOGLE')

        Rails->>Session: session[:user_id] = user.id
        Rails-->>Browser: 302 Redirect to /teacher_home
    else No google_uid match - check email
        DB-->>Rails: NULL

        Rails->>DB: SELECT * FROM users<br/>WHERE LOWER(email) = ? AND meta_type = 'Teacher'

        alt Email matches existing teacher
            DB-->>Rails: Existing user record

            Rails->>DB: UPDATE teachers<br/>SET google_uid = ? WHERE id = user.meta_id
            Note over Rails,DB: Link Google account to existing teacher

            Rails->>DB: UPDATE users SET last_logged_on = NOW()
            Rails->>DB: INSERT INTO teacher_logins<br/>(user_id, login_via='GOOGLE')

            Rails->>Session: session[:user_id] = user.id
            Rails-->>Browser: 302 Redirect to /teacher_home
        else No matching account - create new
            DB-->>Rails: NULL

            Rails->>DB: BEGIN TRANSACTION
            Rails->>DB: INSERT INTO teachers<br/>(first_name, last_name, google_uid, is_verified=true)
            DB-->>Rails: teacher.id

            Rails->>DB: INSERT INTO users<br/>(email, name, password_digest, boddle_uid,<br/>meta_type='Teacher', meta_id=teacher.id)
            DB-->>Rails: user.id

            Rails->>DB: COMMIT

            Rails->>DB: INSERT INTO teacher_logins<br/>(user_id, login_via='GOOGLE')

            Rails->>Session: session[:user_id] = user.id
            Rails-->>Browser: 302 Redirect to /teacher_home
        end
    end
```

**Configuration**: `config/initializers/omniauth.rb`

```ruby
provider :google_oauth2,
  ENV['GOOGLE_CLIENT_ID'],
  ENV['GOOGLE_CLIENT_SECRET'],
  {
    scope: 'userinfo.email, profile, https://www.googleapis.com/auth/classroom.courses.readonly',
    access_type: 'offline',
    prompt: 'consent'
  }
```

**OAuth Scopes**:
- `userinfo.email` - User's email address
- `profile` - User's name and profile picture
- `classroom.courses.readonly` - Read Google Classroom courses (for teachers)

**Access Type**: `offline` - Provides refresh token for long-term access

**Account Matching Logic**:
1. **Priority 1**: Match by `teachers.google_uid` (direct OAuth link)
2. **Priority 2**: Match by email (link OAuth to existing account)
3. **Priority 3**: Create new teacher account

**Stored Data**:
- `teachers.google_uid` - Google's unique identifier (`sub` claim from JWT)
- `sso_oauth_sessions` - Temporary session data during OAuth flow

---

## Clever SSO Flow

### District-Level SSO for K-12 Schools

```mermaid
sequenceDiagram
    participant Browser
    participant Rails as Rails LMS
    participant Clever as Clever SSO
    participant DB as PostgreSQL
    participant Session as Cookie Session

    Browser->>Rails: GET /auth/clever
    Rails-->>Browser: 302 Redirect to Clever<br/>https://clever.com/oauth/authorize?<br/>response_type=code&<br/>client_id=...&<br/>redirect_uri=...

    Browser->>Clever: Authorization request
    Clever-->>Browser: Clever district login page

    Note over Browser,Clever: User authenticates with<br/>district credentials

    Browser->>Clever: District authentication
    Clever-->>Browser: 302 Redirect to callback<br/>?code=AUTH_CODE

    Browser->>Rails: GET /teachers/clever_oauth?code=AUTH_CODE

    Rails->>Clever: POST https://clever.com/oauth/tokens<br/>{code, grant_type, redirect_uri}<br/>Basic Auth: client_id:client_secret
    Clever-->>Rails: {access_token, token_type}

    Rails->>Clever: GET https://api.clever.com/v3.0/me<br/>Authorization: Bearer {access_token}
    Clever-->>Rails: {<br/>  id: "clever_id",<br/>  type: "teacher" or "student",<br/>  email: "...",<br/>  name: {...},<br/>  district: "...",<br/>  school: "..."<br/>}

    Rails->>DB: INSERT INTO sso_oauth_sessions<br/>(uid, email, provider='clever')

    alt Clever type = 'teacher'
        Rails->>DB: SELECT t.*, u.* FROM teachers t<br/>JOIN users u ON u.meta_id = t.id<br/>WHERE t.clever_uid = ?

        alt Teacher with clever_uid exists
            DB-->>Rails: Existing teacher

            Rails->>DB: UPDATE users SET last_logged_on = NOW()
            Rails->>DB: INSERT INTO teacher_logins<br/>(user_id, login_via='CLEVER')
            Rails->>Session: session[:user_id] = user.id
            Rails-->>Browser: 302 Redirect to /teacher_home
        else Check email match
            Rails->>DB: SELECT * FROM users<br/>WHERE email = ? AND meta_type = 'Teacher'

            alt Email matches
                Rails->>DB: UPDATE teachers SET clever_uid = ?<br/>WHERE id = user.meta_id
                Rails->>Session: session[:user_id] = user.id
                Rails-->>Browser: 302 Redirect to /teacher_home
            else Create new teacher
                Rails->>DB: INSERT INTO teachers (clever_uid, first_name, last_name)
                Rails->>DB: INSERT INTO users (email, meta_type='Teacher', meta_id)
                Rails->>Session: session[:user_id] = user.id
                Rails-->>Browser: 302 Redirect to /teacher_home
            end
        end

    else Clever type = 'student'
        Rails->>DB: SELECT s.*, u.* FROM students s<br/>JOIN users u ON u.meta_id = s.id<br/>WHERE s.clever_uid = ?

        alt Student with clever_uid exists
            Rails->>DB: UPDATE users SET last_logged_on = NOW()
            Rails->>Session: session[:user_id] = user.id
            Rails-->>Browser: 302 Redirect to /student_home
        else Create new student
            Rails->>DB: INSERT INTO students<br/>(clever_uid, first_name, last_name, grade_level)
            Rails->>DB: INSERT INTO users<br/>(email, meta_type='Student', meta_id)
            Rails->>Session: session[:user_id] = user.id
            Rails-->>Browser: 302 Redirect to /student_home
        end
    end
```

**Key Features**:
- **Roster Auto-Provisioning**: Students and teachers auto-created from Clever roster
- **District-Level**: Single sign-on for entire school district
- **Role Detection**: Clever provides `type` field (teacher vs student)
- **Rich Metadata**: Includes district, school, section information
- **Premium Licenses**: Clever can manage premium license allocation

**Clever API Data**:
```json
{
  "data": {
    "id": "589c5bc0ace351ab11000001",
    "type": "teacher",
    "email": "teacher@district.edu",
    "name": {
      "first": "John",
      "last": "Smith"
    },
    "district": "589c5bc0ace351ab11000000",
    "schools": ["589c5bc0ace351ab11000002"]
  }
}
```

**Controller**: `app/controllers/teachers_controller.rb:344-410`

---

## Login Token (Magic Link) Flow

### Teacher Generates Link for Student

```mermaid
sequenceDiagram
    participant Teacher
    participant RailsWeb as Rails Web UI
    participant RailsAPI as Rails Backend
    participant DB as PostgreSQL
    participant Student
    participant GameClient as Game Client

    Note over Teacher,GameClient: Teacher wants to give student quick game access

    Teacher->>RailsWeb: Click "Generate Game Link" for student
    RailsWeb->>RailsAPI: POST /students/:id/generate_login_token

    RailsAPI->>RailsAPI: secret = SecureRandom.urlsafe_base64(32)

    RailsAPI->>DB: INSERT INTO login_tokens<br/>(user_id, secret, permanent)<br/>VALUES (student.user.id, secret, false)
    DB-->>RailsAPI: Token created

    RailsAPI-->>RailsWeb: {token: secret, url: "..."}

    RailsWeb-->>Teacher: Display link or QR code<br/>"https://game.boddle.com/auth/token?token=ABC123..."

    Teacher->>Student: Share link (email, print, display on screen)

    Note over Student,GameClient: Student clicks link or scans QR

    Student->>GameClient: Navigate to URL<br/>https://game.boddle.com/auth/token?token=ABC123...

    GameClient->>RailsAPI: GET /auth/token?token=ABC123...

    RailsAPI->>DB: SELECT * FROM login_tokens<br/>WHERE secret = 'ABC123...'

    alt Token not found
        DB-->>RailsAPI: NULL
        RailsAPI-->>GameClient: 401 Unauthorized<br/>"Invalid or expired token"
        GameClient-->>Student: Show error message
    else Token found
        DB-->>RailsAPI: Token record

        RailsAPI->>RailsAPI: Check if permanent

        alt permanent = false
            RailsAPI->>RailsAPI: Check expiry<br/>created_at < 5.minutes.ago?

            alt Token expired
                RailsAPI->>DB: DELETE FROM login_tokens WHERE id = ?
                RailsAPI-->>GameClient: 401 Unauthorized<br/>"Token expired (5 minute limit)"
                GameClient-->>Student: Show error message
            else Token valid
                RailsAPI->>DB: SELECT * FROM users WHERE id = token.user_id
                DB-->>RailsAPI: User record (student)

                RailsAPI->>DB: UPDATE users SET last_logged_on = NOW()
                RailsAPI->>DB: DELETE FROM login_tokens WHERE id = ?<br/>Note: Delete non-permanent after use

                RailsAPI->>RailsAPI: session[:user_id] = user.id
                RailsAPI-->>GameClient: 302 Redirect to /game<br/>Set-Cookie: _lms_session=...

                GameClient-->>Student: Game loads with authenticated session
            end
        else permanent = true
            Note over RailsAPI,DB: Permanent tokens never expire,<br/>not deleted after use

            RailsAPI->>DB: SELECT * FROM users WHERE id = token.user_id
            RailsAPI->>DB: UPDATE users SET last_logged_on = NOW()
            RailsAPI->>RailsAPI: session[:user_id] = user.id
            RailsAPI-->>GameClient: 302 Redirect to /game<br/>Set-Cookie: _lms_session=...
            GameClient-->>Student: Game loads
        end
    end
```

**Token Types**:

| Type | Expiry | Reusable | Use Case |
|------|--------|----------|----------|
| **Temporary** | 5 minutes | No (deleted after use) | Quick classroom access |
| **Permanent** | Never | Yes | Deep links in apps, bookmarks |

**Model**: `app/models/login_token.rb`

**Security Considerations**:
- **Random secret**: 32-byte URL-safe base64 (256 bits of entropy)
- **Short expiry**: 5 minutes for temporary tokens
- **Single use**: Temporary tokens deleted after authentication
- **No password**: Student doesn't need to enter password

**Use Cases**:
1. **Classroom Quick Start**: Teacher displays QR code on projector, students scan to join
2. **Homework Links**: Teacher emails magic link to students
3. **Parent Access**: Parent gets temporary link to view child's progress
4. **Game Deep Links**: Permanent tokens for direct game access

---

## Session Validation Flow

### Every Protected Request

```mermaid
sequenceDiagram
    participant Browser
    participant Rails as Rails LMS
    participant Session as Cookie Session
    participant DB as PostgreSQL

    Browser->>Rails: GET /teacher_home<br/>Cookie: _lms_session=encrypted_session_data

    Rails->>Session: Decrypt session cookie
    Session-->>Rails: session_data = {user_id: 123}

    Rails->>Rails: Check session expiry<br/>(created_at + 6 hours)

    alt Session expired
        Rails->>Session: Clear session
        Rails-->>Browser: 302 Redirect to /teachers/login<br/>Flash: "Your session has expired"
    else Session valid
        Rails->>Rails: @current_user = User.find_by(id: session[:user_id])

        alt User not found
            Note over Rails: User was deleted
            Rails->>Session: Clear session
            Rails-->>Browser: 302 Redirect to /teachers/login
        else User found
            Rails->>Rails: Authorize user for this resource<br/>(check role, classroom access, etc.)

            alt Not authorized
                Rails-->>Browser: 403 Forbidden or 404 Not Found
            else Authorized
                Rails->>DB: SELECT classroom data, students, etc.
                DB-->>Rails: Resource data
                Rails-->>Browser: 200 OK with page content
            end
        end
    end
```

**Session Helper**: `app/helpers/admins_helper.rb`

```ruby
def current_user
  @current_user ||= User.find_by(id: session[:user_id])
end

def logged_in?
  !current_user.nil?
end

def require_login
  unless logged_in?
    redirect_to login_path
  end
end
```

**Session Configuration**: `config/initializers/session_store.rb`

```ruby
Rails.application.config.session_store :cookie_store,
  key: '_lms_session',
  expire_after: 6.hours,
  secure: Rails.env.production?,
  httponly: true,
  same_site: :lax
```

**Authorization Example**:

```ruby
# app/controllers/class_rooms_controller.rb
before_action :require_login
before_action :check_classroom_access, only: [:show, :edit, :update]

def check_classroom_access
  @class_room = ClassRoom.find(params[:id])

  if current_user.is_teacher?
    teacher = Teacher.find(current_user.meta_id)
    unless teacher.class_room_allowed?(@class_room.id)
      render_404
    end
  elsif current_user.is_student?
    # Students can only access their assigned classroom
    unless @class_room.students.include?(current_user.meta)
      render_404
    end
  else
    render_404
  end
end
```

---

## Rate Limiting Flow

### Failed Login Protection

```mermaid
sequenceDiagram
    participant Attacker
    participant Rails as Rails LMS
    participant DB as PostgreSQL

    Note over Attacker,DB: Attacker attempts password guessing

    Attacker->>Rails: POST /teachers/login (attempt 1)<br/>{email, wrong_password}
    Rails->>DB: Check login attempts<br/>SELECT COUNT(*) FROM login_attempts<br/>WHERE ip_address = ? AND created_at > NOW() - 10 min
    DB-->>Rails: count = 0

    Rails->>DB: Verify credentials (FAIL)
    Rails->>DB: INSERT INTO login_attempts<br/>(ip_address, user_id, blocked=false)
    Rails-->>Attacker: 401 "Invalid credentials"

    Note over Attacker,Rails: Attacker tries again (attempts 2-4)

    loop Attempts 2-4
        Attacker->>Rails: POST /teachers/login<br/>{email, wrong_password}
        Rails->>DB: SELECT COUNT(*) FROM login_attempts<br/>WHERE ip_address = ?
        DB-->>Rails: count = 1, 2, 3
        Rails->>DB: INSERT INTO login_attempts
        Rails-->>Attacker: 401 "Invalid credentials"
    end

    Note over Attacker,Rails: 5th attempt - rate limit triggered

    Attacker->>Rails: POST /teachers/login (attempt 5)
    Rails->>DB: SELECT COUNT(*) FROM login_attempts<br/>WHERE ip_address = ? AND created_at > NOW() - 10 min
    DB-->>Rails: count = 4

    Rails->>DB: INSERT INTO login_attempts<br/>(blocked=true)
    Rails-->>Attacker: 401 "Invalid credentials"

    Note over Attacker,Rails: 6th attempt - blocked

    Attacker->>Rails: POST /teachers/login (attempt 6)
    Rails->>DB: SELECT COUNT(*) FROM login_attempts<br/>WHERE ip_address = ? AND created_at > NOW() - 10 min
    DB-->>Rails: count = 5

    Rails->>DB: SELECT MAX(created_at) FROM login_attempts<br/>WHERE ip_address = ?
    DB-->>Rails: last_attempt = 2 minutes ago

    Rails->>Rails: Calculate lockout remaining<br/>15 min - 2 min = 13 min

    Rails-->>Attacker: 429 Too Many Requests<br/>"Too many failed attempts.<br/>Try again in 13 minutes"

    Note over Attacker,Rails: Attacker waits 15 minutes

    Attacker->>Rails: POST /teachers/login (after 15 min)
    Rails->>DB: SELECT COUNT(*) FROM login_attempts<br/>WHERE ip_address = ? AND created_at > NOW() - 10 min
    DB-->>Rails: count = 0 (old attempts expired)

    Note over Rails: Rate limit reset, attempts allowed again
```

**Rate Limiting Rules**:
- **Attempt Window**: 10 minutes (rolling window)
- **Attempt Limit**: 5 failed attempts
- **Lockout Duration**: 15 minutes from last attempt
- **Tracking**: By IP address + email combination

**Model**: `app/models/login_attempt.rb`

**Query Logic**:

```ruby
# Check if IP is blocked
def self.is_blocked?(ip, email)
  # Count attempts in last 10 minutes
  attempts = where(ip_address: ip)
    .where('created_at > ?', 10.minutes.ago)
    .count

  # If 5 or more attempts, check lockout
  if attempts >= 5
    last_attempt = where(ip_address: ip)
      .order(created_at: :desc)
      .first

    # Blocked if last attempt was within 15 minutes
    return last_attempt.created_at > 15.minutes.ago
  end

  false
end
```

**Cleanup Job**: Old login attempts should be purged periodically

```ruby
# Remove attempts older than 24 hours
LoginAttempt.where('created_at < ?', 24.hours.ago).delete_all
```

---

## Summary: Current System Limitations

### Identified Issues

1. **Cookie-based sessions**:
   - Don't work across different domains
   - Require session affinity for load balancing
   - Difficult for mobile apps to manage

2. **Multiple auth mechanisms**:
   - Web UI uses sessions
   - Game uses login tokens
   - API uses IP-based access control
   - Inconsistent security models

3. **Database-dependent rate limiting**:
   - Every login check requires DB query
   - Slow for high-traffic scenarios
   - Can't easily block by IP across multiple app servers

4. **Tight coupling**:
   - Authentication logic embedded in controllers
   - Difficult to extract or reuse
   - Can't easily add new authentication methods

5. **No token revocation**:
   - Sessions only expire after 6 hours
   - No way to force logout across devices
   - No centralized session management

6. **Scaling challenges**:
   - Cookie sessions require sticky sessions
   - Can't easily distribute authentication
   - Hard to add new services that need auth

### What Needs to Change

These flows will be replaced by the Go Authentication Gateway, which will:
- Issue JWTs instead of session cookies
- Centralize all authentication logic
- Use Redis for fast rate limiting
- Provide token revocation (blacklist)
- Enable stateless authentication
- Support easy horizontal scaling

See [New System Authentication Flows](../diagrams/new-system-flows.md) for the planned implementation.
