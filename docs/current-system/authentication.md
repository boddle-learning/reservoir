# Current Rails LMS Authentication System

This document provides a comprehensive overview of the authentication system currently implemented in the Boddle Learning Management System (Rails application).

## Overview

The LMS uses a **custom authentication system** without relying on Devise or Authlogic. It implements multiple authentication methods to support different user types and use cases.

**Location**: `/Users/stjohncj/dev/boddle/learning-management-system/`

## Authentication Architecture

### User Model Structure

The system uses a **polymorphic user model** with inheritance:

```
User (base model)
├── Student
├── Teacher
├── Parent
├── Admin
└── Administrator
```

**Key File**: `app/models/user.rb`

#### User Table Schema

```ruby
# Table: users
# Columns:
#   id                    :integer (primary key)
#   email                 :string (unique)
#   name                  :string
#   password_digest       :string (bcrypt hashed)
#   boddle_uid            :string (UUID - unique internal identifier)
#   meta_type             :string (polymorphic: Student, Teacher, Parent, Admin)
#   meta_id               :integer (foreign key to specific account type)
#   last_logged_on        :datetime
#   last_session_duration :integer
#   timezone              :string
#   timezone_offset       :integer
#   created_at            :datetime
#   updated_at            :datetime
```

### Password Hashing

The system uses Rails' built-in `has_secure_password` with bcrypt:

```ruby
# app/models/user.rb
has_secure_password
```

**Parameters**:
- **Cost Factor**: 10 (default bcrypt cost)
- **Algorithm**: bcrypt
- **Gem**: `gem 'bcrypt', '~> 3.1.7'`

**Password Validation**:
```ruby
PASSWORD_REGEX = /(?=.*[a-z])(?=.*[A-Z])(?=.*\d)(?=.*[^\w\s]).{8,}/

validates :password,
  presence: true,
  length: { minimum: 3 },
  format: { with: PASSWORD_REGEX },
  if: -> { !self.is_student? },
  on: :create
```

**Requirements**:
- Minimum 8 characters
- At least one lowercase letter
- At least one uppercase letter
- At least one digit
- At least one special character
- **Exception**: Students only require minimum 3 characters

## Authentication Methods

### 1. Email/Password Authentication

**User Types**: Teachers, Parents, Students

#### Student Username Format
Students use a special username format instead of email:
```
{username}@student.student
```

Example: `john123@student.student`

#### Login Flow

**Endpoint**: `POST /teachers/login` (handles all user types)

**Controller**: `app/controllers/teachers_controller.rb:768`

```ruby
def login_create
  email = params[:teacher][:email].strip.downcase
  password = params[:teacher][:password]

  # Verify reCAPTCHA
  unless verify_recaptcha(action: 'login_create')
    flash[:error] = "reCAPTCHA verification failed"
    redirect_to teachers_login_path and return
  end

  # Rate limiting check
  if LoginAttempt.is_blocked?(request.remote_ip, email)
    flash[:error] = "Too many failed attempts. Please try again later."
    redirect_to teachers_login_path and return
  end

  # Find user
  user = User.find_by(email: email)

  # Authenticate password
  if user&.authenticate(password)
    log_in(user, TeacherLogin::LOGIN_VIA_EMAIL)
    session[:login_event] = true
    redirect_to teacher_home_path
  else
    # Record failed attempt
    LoginAttempt.create_attempt(request.remote_ip, email, user&.id, false)
    flash[:error] = "Invalid email or password"
    redirect_to teachers_login_path
  end
end
```

**Key Security Features**:
1. **reCAPTCHA v3**: Protects against bots
2. **Rate Limiting**: 5 failed attempts = 15-minute lockout
3. **Case-insensitive email**: `downcase` normalization
4. **Audit trail**: `TeacherLogin` records login method

### 2. Google OAuth2 (SSO)

**Gem**: `gem 'omniauth-google-oauth2', '~> 0.8'`

**Configuration**: `config/initializers/omniauth.rb`

```ruby
Rails.application.config.middleware.use OmniAuth::Builder do
  provider :google_oauth2,
    ENV['GOOGLE_CLIENT_ID'],
    ENV['GOOGLE_CLIENT_SECRET'],
    {
      scope: 'userinfo.email, profile, https://www.googleapis.com/auth/classroom.courses.readonly',
      access_type: 'offline',
      prompt: 'consent'
    }
end
```

**Scopes**:
- `userinfo.email` - User's email address
- `profile` - User's name and profile info
- `classroom.courses.readonly` - Read Google Classroom data (for teachers)

**Access Type**: `offline` - Provides refresh tokens for long-term access

#### OAuth Flow

**Initiation**: `GET /auth/google_oauth2`

**Callback**: `GET /teachers/create_google_oauth` (teachers) or `/students/create_google_oauth` (students)

**Controller**: `app/controllers/teachers_controller.rb:276-342`

```ruby
def create_google_oauth
  # Get OAuth data from OmniAuth
  auth = request.env['omniauth.auth']
  uid = auth.uid
  email = auth.info.email
  name = auth.info.name

  # Create temporary session
  session_data = SsoOauthSession.create_google_session(uid, email, name)

  # Find existing user by google_uid
  teacher = Teacher.find_by(google_uid: uid)

  if teacher
    # Existing Google account
    user = teacher.user
    log_in(user, TeacherLogin::LOGIN_VIA_GOOGLE)
    redirect_to teacher_home_path
  else
    # Check if email matches existing account
    user = User.find_by(email: email)

    if user && user.is_teacher?
      # Link Google account to existing user
      teacher = Teacher.find(user.meta_id)
      teacher.update(google_uid: uid)
      log_in(user, TeacherLogin::LOGIN_VIA_GOOGLE)
      redirect_to teacher_home_path
    else
      # Create new teacher account
      user = User.create_teacher_account(email, name, google_uid: uid)
      log_in(user, TeacherLogin::LOGIN_VIA_GOOGLE)
      redirect_to teacher_home_path
    end
  end
rescue => e
  flash[:error] = "Google authentication failed: #{e.message}"
  redirect_to teachers_login_path
end
```

**User Matching Priority**:
1. Find by `google_uid` (direct match)
2. Find by email (link account)
3. Create new account

**Stored Fields**:
- Teachers: `teachers.google_uid`
- Students: `students.google_uid`
- Parents: `parents.google_uid`

### 3. Clever SSO (K-12 Education Platform)

**Integration Type**: OAuth2-based SSO

**Endpoint**: `POST /teachers/clever_oauth`

**Controller**: `app/controllers/teachers_controller.rb:344-410`

Clever provides district-level SSO for K-12 schools. It automatically provisions accounts based on roster data.

#### Clever Flow

```ruby
def clever_oauth
  code = params[:code]

  # Exchange code for token
  token_response = HTTParty.post(
    'https://clever.com/oauth/tokens',
    body: {
      code: code,
      grant_type: 'authorization_code',
      redirect_uri: ENV['CLEVER_REDIRECT_URI']
    },
    basic_auth: {
      username: ENV['CLEVER_CLIENT_ID'],
      password: ENV['CLEVER_CLIENT_SECRET']
    }
  )

  access_token = token_response['access_token']

  # Get user info from Clever
  user_response = HTTParty.get(
    'https://api.clever.com/v3.0/me',
    headers: { 'Authorization' => "Bearer #{access_token}" }
  )

  clever_id = user_response['data']['id']
  clever_type = user_response['data']['type'] # 'teacher' or 'student'

  # Find or create user
  if clever_type == 'teacher'
    teacher = Teacher.find_by(clever_uid: clever_id)
    if teacher
      log_in(teacher.user, TeacherLogin::LOGIN_VIA_CLEVER)
    else
      # Create new teacher from Clever data
      user = User.create_teacher_from_clever(user_response['data'])
      log_in(user, TeacherLogin::LOGIN_VIA_CLEVER)
    end
  elsif clever_type == 'student'
    student = Student.find_by(clever_uid: clever_id)
    if student
      log_in(student.user)
    else
      # Create new student from Clever data
      user = User.create_student_from_clever(user_response['data'])
      log_in(user)
    end
  end

  redirect_to appropriate_home_path
rescue => e
  flash[:error] = "Clever authentication failed"
  redirect_to login_path
end
```

**Clever Data Includes**:
- District information
- School information
- Section (class) enrollment
- Student/Teacher role
- Email (if available)

**Stored Fields**:
- Teachers: `teachers.clever_uid`
- Students: `students.clever_uid`

### 4. iCloud Sign In (Apple Sign In)

**Integration Type**: OAuth2 (Apple ID)

**User Types**: Primarily Parents, also Students

**Stored Fields**:
- Parents: `parents.icloud_uid`
- Students: `students.icloud_uid`

**Special Considerations**:
- Apple's "Hide My Email" feature generates random email addresses
- Must handle email obfuscation
- Requires Apple Developer account configuration

**Note**: Implementation details similar to Google OAuth2 but with Apple-specific token generation (requires signing JWT with Apple private key).

### 5. Login Tokens (Magic Links)

**Purpose**: Quick authentication for game client access

**Model**: `app/models/login_token.rb`

**Table Schema**:
```ruby
# Table: login_tokens
# Columns:
#   id           :integer (primary key)
#   secret       :string (random token)
#   user_id      :integer (foreign key to users)
#   permanent    :boolean (default: false)
#   created_at   :datetime
#   updated_at   :datetime
```

#### Token Generation

**Typical Use Case**: Teacher generates link for student to access game

```ruby
# Generate login token
token = LoginToken.create(
  user_id: student.user.id,
  secret: SecureRandom.urlsafe_base64(32),
  permanent: false
)

# Generate URL
url = "https://game.boddle.com/auth/token?token=#{token.secret}"
```

#### Token Validation

**Endpoint**: `GET /auth/token?token=SECRET`

**Validation Logic**:
```ruby
def login_with_token
  secret = params[:token]

  # Find token
  login_token = LoginToken.find_by(secret: secret)

  unless login_token
    render json: { error: 'Invalid token' }, status: :unauthorized
    return
  end

  # Check expiry (5 minutes for non-permanent)
  unless login_token.permanent?
    if login_token.created_at < 5.minutes.ago
      login_token.destroy
      render json: { error: 'Token expired' }, status: :unauthorized
      return
    end
  end

  # Authenticate user
  user = User.find(login_token.user_id)
  log_in(user)

  # Delete non-permanent token
  login_token.destroy unless login_token.permanent?

  redirect_to game_path
end
```

**Token Types**:
- **Temporary**: Expires after 5 minutes, deleted after use
- **Permanent**: Never expires, can be reused (for deep links)

## Session Management

### Session Configuration

**File**: `config/initializers/session_store.rb`

```ruby
Rails.application.config.session_store :cookie_store,
  key: '_lms_session',
  expire_after: 6.hours,
  secure: Rails.env.production?,
  httponly: true,
  same_site: :lax
```

**Parameters**:
- **Storage**: Cookie-based (encrypted)
- **Key**: `_lms_session`
- **Expiry**: 6 hours
- **Secure**: HTTPS only in production
- **HttpOnly**: Cannot be accessed by JavaScript
- **SameSite**: Lax (allows top-level navigation)

### Session Helper Methods

**File**: `app/helpers/admins_helper.rb`

```ruby
def log_in(user, login_method = nil)
  session[:user_id] = user.id
  user.update_last_logged_on
  TeacherLogin.create_login_entry(user, login_method) if login_method
end

def current_user
  @current_user ||= User.find_by(id: session[:user_id])
end

def logged_in?
  !current_user.nil?
end

def log_out
  user = User.find_by(id: session[:user_id])
  user&.update_last_session_duration
  session.delete(:user_id)
  @current_user = nil
end

def require_login
  unless logged_in?
    redirect_to login_path
  end
end
```

### Session Tracking

The system tracks:
- **Last logged on**: `users.last_logged_on` (timestamp)
- **Session duration**: `users.last_session_duration` (seconds)
- **Login method**: `teacher_logins.login_via` (EMAIL, GOOGLE, CLEVER)

## Rate Limiting & Security

### Login Attempt Tracking

**Model**: `app/models/login_attempt.rb`

**Table Schema**:
```ruby
# Table: login_attempts
# Columns:
#   id            :integer (primary key)
#   user_id       :integer
#   ip_address    :string
#   user_agent    :text
#   blocked       :boolean
#   created_at    :datetime
```

### Rate Limiting Logic

```ruby
class LoginAttempt < ApplicationRecord
  ATTEMPT_LIMIT = 5
  LOCKOUT_PERIOD = 15.minutes
  TIME_WINDOW = 10.minutes

  def self.is_blocked?(ip, email)
    user = User.find_by(email: email)

    # Count recent attempts
    attempts = where(ip_address: ip)
      .where('created_at > ?', TIME_WINDOW.ago)
      .count

    # Check if blocked
    if attempts >= ATTEMPT_LIMIT
      last_attempt = where(ip_address: ip).order(created_at: :desc).first
      return true if last_attempt.created_at > LOCKOUT_PERIOD.ago
    end

    false
  end

  def self.create_attempt(ip, email, user_id, blocked)
    create(
      ip_address: ip,
      user_id: user_id,
      user_agent: request.user_agent,
      blocked: blocked
    )
  end
end
```

**Rules**:
- **Limit**: 5 failed attempts
- **Window**: 10 minutes
- **Lockout**: 15 minutes
- **Tracking**: By IP address + email combination

### Other Security Features

1. **reCAPTCHA v3**: `verify_recaptcha(action: 'login_create')`
2. **CSRF Protection**: `protect_from_forgery with: :exception`
3. **Email Verification**: Teachers and Parents require email confirmation
4. **Secure Cookies**: HTTPS-only in production
5. **Blacklisted Domains**: `BlacklistedDomain.deny_email?(email)` prevents specific domains

## Authorization (Role-Based Access Control)

### User Role Checking

```ruby
# app/models/user.rb
def is_student?
  meta_type == 'Student'
end

def is_teacher?
  meta_type == 'Teacher'
end

def is_parent?
  meta_type == 'Parent'
end

def is_admin?
  meta_type == 'Admin'
end

def is_administrator?
  meta_type == 'Administrator'
end
```

### Classroom-Level Authorization

Teachers can only access their assigned classrooms:

```ruby
# app/models/teacher.rb
def class_room_allowed?(class_room_id)
  class_rooms.pluck(:id).include?(class_room_id.to_i)
end
```

**Usage in Controllers**:
```ruby
# app/controllers/class_rooms_controller.rb
def show
  @class_room = ClassRoom.find(params[:id])

  unless current_user.is_teacher? &&
         Teacher.find(current_user.meta_id).class_room_allowed?(params[:id])
    render_404
  end
end
```

### Admin Permissions (Fine-Grained)

**Model**: `app/models/admin.rb`

Admins have granular permissions:
- `is_superAdmin` - Full system access
- `user_manager` - Manage users
- `class_room_manager` - Edit classrooms
- `curriculum_manager` - Edit curriculum
- `analytic_manager` - View analytics
- `publisher_manager` - Manage publishers
- `template_manager` - Manage templates
- `scaffoldings` - Manage adaptive learning
- `data_bank` - Access data
- `class_room_curriculum` - Manage class curriculum
- `ui_customization` - Customize UI
- `variables` - Manage variables

**Usage**:
```ruby
def has_write_access?(category)
  admin = Admin.find_by_id(current_user.meta_id)
  admin[category] == true
end
```

## API Authentication

### Grape API Base

**File**: `app/controllers/api/v1/base.rb`

The API uses IP-based access control:

```ruby
unless ENV['SLS_IP'] == request.ip
  result['message'] = 'Access Denied'
  result['success'] = false
  return result
end
```

### API Endpoints

**File**: `app/controllers/api/v1/users.rb`

Key endpoints:
- `POST /api/v1/users/create` - Create user account
- `POST /api/v1/users/verify` - Verify credentials
- `POST /api/v1/users/link` - Link SSO account
- `POST /api/v1/users/premium/status` - Check premium status

### JWT Support

The Rails app has JWT gem installed but not actively used for authentication:

```ruby
# Gemfile
gem 'jwt'
```

This gem is available for the future migration to JWT-based authentication.

## Audit Trail

### Teacher Login Tracking

**Model**: `app/models/teacher_login.rb`

```ruby
class TeacherLogin < ApplicationRecord
  LOGIN_VIA_EMAIL = 'EMAIL'
  LOGIN_VIA_GOOGLE = 'GOOGLE'
  LOGIN_VIA_CLEVER = 'CLEVER'

  def self.create_login_entry(user, login_method)
    create(
      user_id: user.id,
      login_via: login_method,
      logged_on: Time.current
    )
  end
end
```

**Table Schema**:
```ruby
# Table: teacher_logins
# Columns:
#   id          :integer (primary key)
#   user_id     :integer
#   login_via   :string (EMAIL, GOOGLE, CLEVER)
#   logged_on   :datetime
```

## Key Files Reference

| File | Purpose |
|------|---------|
| `app/models/user.rb` | Base user model, polymorphic associations |
| `app/models/teacher.rb` | Teacher-specific logic, OAuth UIDs |
| `app/models/student.rb` | Student-specific logic, username format |
| `app/models/parent.rb` | Parent-specific logic |
| `app/models/login_token.rb` | Magic link tokens |
| `app/models/login_attempt.rb` | Rate limiting |
| `app/controllers/teachers_controller.rb` | Login, OAuth callbacks |
| `app/controllers/students_controller.rb` | Student login |
| `app/controllers/api/v1/users.rb` | API authentication |
| `app/helpers/admins_helper.rb` | Session management helpers |
| `config/initializers/omniauth.rb` | OAuth configuration |
| `config/initializers/session_store.rb` | Session configuration |

## Environment Variables

```bash
# Google OAuth2
GOOGLE_CLIENT_ID=...
GOOGLE_CLIENT_SECRET=...

# Clever SSO
CLEVER_CLIENT_ID=...
CLEVER_CLIENT_SECRET=...
CLEVER_REDIRECT_URI=https://lms.boddle.com/teachers/clever_oauth

# iCloud Sign In
ICLOUD_CLIENT_ID=...
ICLOUD_CLIENT_SECRET=...

# API Access
SLS_IP=... # Whitelisted IP for API access

# reCAPTCHA
RECAPTCHA_SITE_KEY=...
RECAPTCHA_SECRET_KEY=...
```

## Limitations & Issues

### Current Challenges

1. **Cookie-based sessions**:
   - Don't work well for mobile apps
   - Require session affinity for horizontal scaling
   - Can't be easily shared across services

2. **Separate auth mechanisms**:
   - Web uses sessions
   - Game uses login tokens
   - API uses IP-based access

3. **Tight coupling**:
   - Auth logic embedded in Rails controllers
   - Difficult to extract or reuse

4. **Limited token support**:
   - JWT gem installed but not used
   - No refresh token mechanism
   - No token revocation

5. **Scaling concerns**:
   - Session storage in cookies limits load balancing
   - Rate limiting in database (slow for high traffic)

## Next Steps

See [Migration Documentation](../migration/rails-changes.md) for how this system will be transformed with the Go authentication gateway.
