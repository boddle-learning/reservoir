# Rails JWT Authentication Middleware
#
# This middleware validates JWT tokens issued by the Go Authentication Gateway (Reservoir)
# and populates the current_user for Rails controllers.
#
# Installation:
# 1. Add to Gemfile: gem 'jwt'
# 2. Place this file in app/middleware/jwt_auth.rb
# 3. Register in config/application.rb: config.middleware.use JwtAuth
# 4. Set environment variables:
#    - JWT_SECRET_KEY (must match Go gateway)
#    - JWT_FALLBACK_TO_SESSION (true/false)
#    - REDIS_URL (for token blacklist)

require 'jwt'
require 'redis'

class JwtAuth
  ALGORITHM = 'HS256'.freeze
  SKIP_PATHS = ['/health', '/auth/', '/public/', '/assets/'].freeze

  def initialize(app)
    @app = app
    @redis = Redis.new(url: ENV['REDIS_URL'] || 'redis://localhost:6379/0')
  rescue Redis::CannotConnectError => e
    Rails.logger.error("Redis connection failed for JWT blacklist: #{e.message}")
    @redis = nil
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
        # Validate JWT and populate user
        payload = decode_token(token)

        # Check if token is blacklisted (revoked)
        if token_blacklisted?(payload['jti'])
          return unauthorized_response('Token has been revoked')
        end

        # Find user by ID from JWT claims
        user = User.find_by(id: payload['user_id'])
        unless user
          return unauthorized_response('User not found')
        end

        # Store in request env for controllers
        env['current_user'] = user
        env['jwt_payload'] = payload
        env['jwt_token'] = token

      elsif fallback_to_session?
        # Backward compatibility: check Rails session
        # This allows gradual migration from session to JWT
        # Skip to allow existing session-based auth to work
      else
        # JWT is required
        return unauthorized_response('Missing authentication token')
      end

      @app.call(env)

    rescue JWT::ExpiredSignature
      unauthorized_response('Token has expired')
    rescue JWT::DecodeError => e
      Rails.logger.warn("JWT decode error: #{e.message}")
      unauthorized_response('Invalid token format')
    rescue StandardError => e
      Rails.logger.error("JWT authentication error: #{e.class} - #{e.message}")
      Rails.logger.error(e.backtrace.join("\n"))
      unauthorized_response('Authentication error')
    end
  end

  private

  def decode_token(token)
    secret_key = ENV['JWT_SECRET_KEY']
    raise 'JWT_SECRET_KEY environment variable not set' unless secret_key

    decoded = JWT.decode(
      token,
      secret_key,
      true,
      {
        algorithm: ALGORITHM,
        verify_expiration: true,
        verify_iat: true
      }
    )

    decoded.first
  end

  def extract_token(request)
    # Check Authorization header first (preferred method)
    auth_header = request.env['HTTP_AUTHORIZATION']
    if auth_header&.start_with?('Bearer ')
      return auth_header.split(' ').last
    end

    # Fallback: check query parameter (for special use cases like downloads)
    request.params['token']
  end

  def token_blacklisted?(jti)
    return false unless jti && @redis

    begin
      @redis.exists?("blacklist:jti:#{jti}") == 1
    rescue Redis::BaseError => e
      Rails.logger.error("Redis blacklist check failed: #{e.message}")
      # Fail open - allow request if Redis is down
      false
    end
  end

  def fallback_to_session?
    ENV['JWT_FALLBACK_TO_SESSION'] == 'true'
  end

  def skip_jwt_validation?(path)
    SKIP_PATHS.any? { |skip_path| path.start_with?(skip_path) }
  end

  def unauthorized_response(message)
    [
      401,
      { 'Content-Type' => 'application/json' },
      [{ success: false, error: { code: 'UNAUTHORIZED', message: message } }.to_json]
    ]
  end
end
