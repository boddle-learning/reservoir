# JWT Authentication Configuration
#
# Place this file in config/initializers/jwt_auth.rb
#
# This initializer registers the JWT middleware and configures
# the integration with the Go Authentication Gateway (Reservoir).

Rails.application.configure do
  # Register JWT middleware
  # This must come before ActionDispatch::Session::CookieStore to intercept
  # requests before session handling
  config.middleware.insert_before ActionDispatch::Session::CookieStore, JwtAuth

  # JWT Configuration
  config.x.jwt = ActiveSupport::OrderedOptions.new

  # Secret key for JWT validation (MUST match Go gateway)
  config.x.jwt.secret_key = ENV['JWT_SECRET_KEY']

  # Enable fallback to session-based auth during migration
  # Set to 'false' once migration is complete
  config.x.jwt.fallback_to_session = ENV.fetch('JWT_FALLBACK_TO_SESSION', 'true') == 'true'

  # Redis URL for token blacklist checking
  config.x.jwt.redis_url = ENV.fetch('REDIS_URL', 'redis://localhost:6379/0')

  # Paths that skip JWT validation (public endpoints)
  config.x.jwt.skip_paths = [
    '/health',
    '/auth/',
    '/public/',
    '/assets/',
    '/favicon.ico'
  ]

  # Raise error if JWT_SECRET_KEY is not set in production
  if Rails.env.production? && config.x.jwt.secret_key.blank?
    raise 'JWT_SECRET_KEY environment variable must be set in production'
  end
end

# Logging configuration
if defined?(Rails.logger)
  Rails.logger.info('[JWT Auth] Middleware registered')
  Rails.logger.info("[JWT Auth] Fallback to session: #{Rails.configuration.x.jwt.fallback_to_session}")
end
