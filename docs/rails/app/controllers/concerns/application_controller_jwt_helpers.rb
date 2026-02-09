# Application Controller JWT Integration
#
# Add these methods to your ApplicationController to work with JWT authentication.
# These methods provide backward compatibility with existing session-based auth
# while supporting the new JWT tokens from the Go gateway.
#
# Usage:
# 1. Add these methods to app/controllers/application_controller.rb
# 2. Update authenticate_user! to use jwt_current_user
# 3. Update current_user to use jwt_current_user with session fallback

module ApplicationControllerJwtHelpers
  extend ActiveSupport::Concern

  included do
    # Make these methods available in views
    helper_method :current_user, :logged_in?, :current_user_meta
  end

  # Returns the current user from JWT or session
  def current_user
    @current_user ||= jwt_current_user || session_current_user
  end

  # Returns the current user from JWT (set by middleware)
  def jwt_current_user
    request.env['current_user']
  end

  # Returns the current user from session (legacy)
  def session_current_user
    return nil unless session[:user_id]
    @session_current_user ||= User.find_by(id: session[:user_id])
  end

  # Returns the JWT payload if authenticated via JWT
  def jwt_payload
    request.env['jwt_payload']
  end

  # Returns the JWT token string
  def jwt_token
    request.env['jwt_token']
  end

  # Check if user is logged in (via JWT or session)
  def logged_in?
    current_user.present?
  end

  # Require authentication (JWT or session)
  def authenticate_user!
    return if current_user

    respond_to do |format|
      format.html { redirect_to login_path, alert: 'Please log in to continue' }
      format.json do
        render json: {
          success: false,
          error: {
            code: 'UNAUTHORIZED',
            message: 'Authentication required'
          }
        }, status: :unauthorized
      end
    end
  end

  # Get user's meta (Teacher/Student/Parent) from JWT or database
  def current_user_meta
    return nil unless current_user

    # If from JWT, get meta from payload
    if jwt_payload
      case jwt_payload['meta_type']
      when 'Teacher'
        @current_user_meta ||= Teacher.find_by(id: jwt_payload['meta_id'])
      when 'Student'
        @current_user_meta ||= Student.find_by(id: jwt_payload['meta_id'])
      when 'Parent'
        @current_user_meta ||= Parent.find_by(id: jwt_payload['meta_id'])
      end
    else
      # Fallback to database lookup
      @current_user_meta ||= current_user.meta
    end
  end

  # Check if user is a teacher
  def current_user_teacher?
    current_user&.meta_type == 'Teacher'
  end

  # Check if user is a student
  def current_user_student?
    current_user&.meta_type == 'Student'
  end

  # Check if user is a parent
  def current_user_parent?
    current_user&.meta_type == 'Parent'
  end

  # Get Boddle UID from JWT or database
  def current_user_boddle_uid
    if jwt_payload
      jwt_payload['boddle_uid']
    else
      current_user&.boddle_uid
    end
  end
end

# Example: Add to ApplicationController
# -------------------------------------
# class ApplicationController < ActionController::Base
#   include ApplicationControllerJwtHelpers
#
#   # Optional: Require authentication for all actions by default
#   # before_action :authenticate_user!
#
#   # Optional: Skip JWT validation for specific actions
#   # skip_before_action :verify_authenticity_token, only: [:api_endpoint]
# end
