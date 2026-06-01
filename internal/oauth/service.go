package oauth

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/boddle/reservoir/internal/auth"
	"github.com/boddle/reservoir/internal/token"
	"github.com/boddle/reservoir/internal/user"
)

// AuthService handles OAuth authentication business logic
type AuthService struct {
	userRepo     *user.Repository
	tokenService *token.Service
	googleSvc    *GoogleService
	cleverSvc    *CleverService
	lastLogin    user.LastLoginEnqueuer
}

// NewAuthService creates a new OAuth authentication service
func NewAuthService(
	userRepo *user.Repository,
	tokenService *token.Service,
	googleSvc *GoogleService,
	cleverSvc *CleverService,
	lastLogin user.LastLoginEnqueuer,
) *AuthService {
	return &AuthService{
		userRepo:     userRepo,
		tokenService: tokenService,
		googleSvc:    googleSvc,
		cleverSvc:    cleverSvc,
		lastLogin:    lastLogin,
	}
}

// AuthenticateWithGoogle authenticates a user with Google OAuth
func (s *AuthService) AuthenticateWithGoogle(ctx context.Context, code, state string) (*auth.LoginResponse, string, error) {
	// Handle Google OAuth callback
	oauthUserInfo, redirectURL, err := s.googleSvc.HandleCallback(ctx, code, state)
	if err != nil {
		return nil, "", err
	}

	// Find or create user
	usr, meta, err := s.findOrCreateGoogleUser(ctx, oauthUserInfo)
	if err != nil {
		return nil, "", err
	}

	s.lastLogin.Enqueue(usr.ID)

	// Generate JWT token
	boddleUID := ""
	if usr.BoddleUID.Valid {
		boddleUID = usr.BoddleUID.String
	}

	fullName := usr.Name
	if m, ok := meta.(*user.Teacher); ok {
		fullName = m.FirstName + " " + m.LastName
	}

	tokenPair, err := s.tokenService.Generate(
		usr.ID,
		boddleUID,
		usr.Email,
		fullName,
		usr.MetaType,
		usr.MetaID,
	)
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate token: %w", err)
	}

	return &auth.LoginResponse{
		Token: tokenPair,
		User:  usr,
		Meta:  meta,
	}, redirectURL, nil
}

// findOrCreateGoogleUser finds an existing user by Google UID or email, or returns error
// Note: User creation is handled by Rails, so we only link existing accounts
func (s *AuthService) findOrCreateGoogleUser(ctx context.Context, info *OAuthUserInfo) (*user.User, interface{}, error) {
	// Try to find teacher by Google UID
	teacher, err := s.userRepo.FindTeacherByGoogleUID(ctx, info.ProviderUserID)
	if err != nil {
		return nil, nil, err
	}

	if teacher != nil {
		usr, err := s.userRepo.FindUserByMeta(ctx, "Teacher", teacher.ID)
		if err != nil {
			return nil, nil, err
		}
		return usr, teacher, nil
	}

	// Try to find student by Google UID
	student, err := s.userRepo.FindStudentByGoogleUID(ctx, info.ProviderUserID)
	if err != nil {
		return nil, nil, err
	}

	if student != nil {
		usr, err := s.userRepo.FindUserByMeta(ctx, "Student", student.ID)
		if err != nil {
			return nil, nil, err
		}
		return usr, student, nil
	}

	// Try to find by email (account linking)
	usr, err := s.userRepo.FindByEmail(ctx, info.Email)
	if err != nil {
		return nil, nil, err
	}

	if usr == nil {
		return nil, nil, fmt.Errorf("no account found for this Google account. Please sign up first.")
	}

	// Link account by updating Google UID
	switch usr.MetaType {
	case "Teacher":
		teacher, err := s.userRepo.FindTeacher(ctx, usr.MetaID)
		if err != nil {
			return nil, nil, err
		}
		if teacher == nil {
			return nil, nil, fmt.Errorf("teacher meta not found")
		}

		// Update Google UID
		if err := s.userRepo.UpdateTeacherGoogleUID(ctx, teacher.ID, info.ProviderUserID); err != nil {
			return nil, nil, fmt.Errorf("failed to link Google account: %w", err)
		}

		teacher.GoogleUID = sql.NullString{String: info.ProviderUserID, Valid: true}
		return usr, teacher, nil

	case "Student":
		student, err := s.userRepo.FindStudent(ctx, usr.MetaID)
		if err != nil {
			return nil, nil, err
		}
		if student == nil {
			return nil, nil, fmt.Errorf("student meta not found")
		}

		// Update Google UID
		if err := s.userRepo.UpdateStudentGoogleUID(ctx, student.ID, info.ProviderUserID); err != nil {
			return nil, nil, fmt.Errorf("failed to link Google account: %w", err)
		}

		student.GoogleUID = sql.NullString{String: info.ProviderUserID, Valid: true}
		return usr, student, nil

	default:
		return nil, nil, fmt.Errorf("unsupported user type for Google OAuth: %s", usr.MetaType)
	}
}

// AuthenticateWithGoogleToken authenticates using a pre-obtained Google access token.
// Used when the LMS has already completed the Google OAuth flow via OmniAuth and
// passes the resulting access token to Reservoir for JWT issuance.
//
// The access token is verified directly against Google: the identity (uid,
// email, name) is taken from Google's userinfo response, never from the caller.
// A caller can therefore only mint a JWT for an identity it holds a valid
// Google token for — it cannot assert an arbitrary uid/email. See LMS-6511 /
// security review Finding 0.
func (s *AuthService) AuthenticateWithGoogleToken(ctx context.Context, accessToken string) (*auth.LoginResponse, error) {
	oauthUserInfo, err := s.googleSvc.fetchUserInfo(ctx, accessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to verify Google access token: %w", err)
	}

	usr, meta, err := s.findOrCreateGoogleUser(ctx, oauthUserInfo)
	if err != nil {
		return nil, err
	}

	s.lastLogin.Enqueue(usr.ID)

	boddleUID := ""
	if usr.BoddleUID.Valid {
		boddleUID = usr.BoddleUID.String
	}

	fullName := usr.Name
	if m, ok := meta.(*user.Teacher); ok {
		fullName = m.FirstName + " " + m.LastName
	}

	tokenPair, err := s.tokenService.Generate(
		usr.ID, boddleUID, usr.Email, fullName, usr.MetaType, usr.MetaID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	return &auth.LoginResponse{Token: tokenPair, User: usr, Meta: meta}, nil
}

// AuthenticateWithCleverToken authenticates using a pre-obtained Clever access token.
// Used when the LMS has already completed the Clever SSO flow via OmniAuth and
// passes the resulting access token to Reservoir for JWT issuance.
//
// The access token is verified directly against Clever: the identity (uid,
// email, name) is taken from Clever's /me response, never from the caller.
// A caller can therefore only mint a JWT for an identity it holds a valid
// Clever token for — it cannot assert an arbitrary uid/email. See LMS-6511 /
// security review Finding 0.
func (s *AuthService) AuthenticateWithCleverToken(ctx context.Context, accessToken string) (*auth.LoginResponse, error) {
	oauthUserInfo, err := s.cleverSvc.fetchUserInfo(ctx, accessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to verify Clever access token: %w", err)
	}

	usr, meta, err := s.findOrCreateCleverUser(ctx, oauthUserInfo)
	if err != nil {
		return nil, err
	}

	s.lastLogin.Enqueue(usr.ID)

	boddleUID := ""
	if usr.BoddleUID.Valid {
		boddleUID = usr.BoddleUID.String
	}

	fullName := usr.Name
	if m, ok := meta.(*user.Teacher); ok {
		fullName = m.FirstName + " " + m.LastName
	}

	tokenPair, err := s.tokenService.Generate(
		usr.ID, boddleUID, usr.Email, fullName, usr.MetaType, usr.MetaID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	return &auth.LoginResponse{Token: tokenPair, User: usr, Meta: meta}, nil
}

// AuthenticateWithClever authenticates a user with Clever SSO
func (s *AuthService) AuthenticateWithClever(ctx context.Context, code, state string) (*auth.LoginResponse, string, error) {
	// Handle Clever OAuth callback
	oauthUserInfo, redirectURL, err := s.cleverSvc.HandleCallback(ctx, code, state)
	if err != nil {
		return nil, "", err
	}

	// Find or create user
	usr, meta, err := s.findOrCreateCleverUser(ctx, oauthUserInfo)
	if err != nil {
		return nil, "", err
	}

	s.lastLogin.Enqueue(usr.ID)

	// Generate JWT token
	boddleUID := ""
	if usr.BoddleUID.Valid {
		boddleUID = usr.BoddleUID.String
	}

	fullName := usr.Name
	if m, ok := meta.(*user.Teacher); ok {
		fullName = m.FirstName + " " + m.LastName
	}

	tokenPair, err := s.tokenService.Generate(
		usr.ID,
		boddleUID,
		usr.Email,
		fullName,
		usr.MetaType,
		usr.MetaID,
	)
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate token: %w", err)
	}

	return &auth.LoginResponse{
		Token: tokenPair,
		User:  usr,
		Meta:  meta,
	}, redirectURL, nil
}

// findOrCreateCleverUser finds an existing user by Clever UID or email, or returns error
// Note: User creation is handled by Rails, so we only link existing accounts
func (s *AuthService) findOrCreateCleverUser(ctx context.Context, info *OAuthUserInfo) (*user.User, interface{}, error) {
	// Try to find teacher by Clever UID
	teacher, err := s.userRepo.FindTeacherByCleverUID(ctx, info.ProviderUserID)
	if err != nil {
		return nil, nil, err
	}

	if teacher != nil {
		usr, err := s.userRepo.FindUserByMeta(ctx, "Teacher", teacher.ID)
		if err != nil {
			return nil, nil, err
		}
		return usr, teacher, nil
	}

	// Try to find student by Clever UID
	student, err := s.userRepo.FindStudentByCleverUID(ctx, info.ProviderUserID)
	if err != nil {
		return nil, nil, err
	}

	if student != nil {
		usr, err := s.userRepo.FindUserByMeta(ctx, "Student", student.ID)
		if err != nil {
			return nil, nil, err
		}
		return usr, student, nil
	}

	// Try to find by email (account linking)
	usr, err := s.userRepo.FindByEmail(ctx, info.Email)
	if err != nil {
		return nil, nil, err
	}

	if usr == nil {
		return nil, nil, fmt.Errorf("no account found for this Clever account. Please sign up first.")
	}

	// Link account by updating Clever UID
	switch usr.MetaType {
	case "Teacher":
		teacher, err := s.userRepo.FindTeacher(ctx, usr.MetaID)
		if err != nil {
			return nil, nil, err
		}
		if teacher == nil {
			return nil, nil, fmt.Errorf("teacher meta not found")
		}

		// Update Clever UID
		if err := s.userRepo.UpdateTeacherCleverUID(ctx, teacher.ID, info.ProviderUserID); err != nil {
			return nil, nil, fmt.Errorf("failed to link Clever account: %w", err)
		}

		teacher.CleverUID = sql.NullString{String: info.ProviderUserID, Valid: true}
		return usr, teacher, nil

	case "Student":
		student, err := s.userRepo.FindStudent(ctx, usr.MetaID)
		if err != nil {
			return nil, nil, err
		}
		if student == nil {
			return nil, nil, fmt.Errorf("student meta not found")
		}

		// Update Clever UID
		if err := s.userRepo.UpdateStudentCleverUID(ctx, student.ID, info.ProviderUserID); err != nil {
			return nil, nil, fmt.Errorf("failed to link Clever account: %w", err)
		}

		student.CleverUID = sql.NullString{String: info.ProviderUserID, Valid: true}
		return usr, student, nil

	default:
		return nil, nil, fmt.Errorf("unsupported user type for Clever SSO: %s", usr.MetaType)
	}
}

// AuthenticateWithiCloud authenticates a user with an Apple UID provided by the client.
// The client handles Sign in with Apple directly and passes the UID to the server.
// No server-side token verification is performed (matches LMS behavior).
func (s *AuthService) AuthenticateWithiCloud(ctx context.Context, uid string) (*auth.LoginResponse, error) {
	// Find user by iCloud UID
	usr, meta, err := s.findOrCreateiCloudUser(ctx, &OAuthUserInfo{ProviderUserID: uid})
	if err != nil {
		return nil, err
	}

	s.lastLogin.Enqueue(usr.ID)

	// Generate JWT token
	boddleUID := ""
	if usr.BoddleUID.Valid {
		boddleUID = usr.BoddleUID.String
	}

	fullName := usr.Name
	if m, ok := meta.(*user.Parent); ok {
		fullName = m.FirstName + " " + m.LastName
	}

	tokenPair, err := s.tokenService.Generate(
		usr.ID,
		boddleUID,
		usr.Email,
		fullName,
		usr.MetaType,
		usr.MetaID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	return &auth.LoginResponse{
		Token: tokenPair,
		User:  usr,
		Meta:  meta,
	}, nil
}

// findOrCreateiCloudUser finds an existing user by iCloud UID
// Note: User creation is handled by Rails, so we only look up existing accounts.
// The client handles Sign in with Apple and passes the UID — no email-based
// linking since we don't receive email from the client in this flow.
func (s *AuthService) findOrCreateiCloudUser(ctx context.Context, info *OAuthUserInfo) (*user.User, interface{}, error) {
	// Try to find student by iCloud UID
	student, err := s.userRepo.FindStudentByiCloudUID(ctx, info.ProviderUserID)
	if err != nil {
		return nil, nil, err
	}

	if student != nil {
		usr, err := s.userRepo.FindUserByMeta(ctx, "Student", student.ID)
		if err != nil {
			return nil, nil, err
		}
		return usr, student, nil
	}

	// Try to find parent by iCloud UID
	parent, err := s.userRepo.FindParentByiCloudUID(ctx, info.ProviderUserID)
	if err != nil {
		return nil, nil, err
	}

	if parent != nil {
		usr, err := s.userRepo.FindUserByMeta(ctx, "Parent", parent.ID)
		if err != nil {
			return nil, nil, err
		}
		return usr, parent, nil
	}

	return nil, nil, fmt.Errorf("no account found for this iCloud UID. Please sign up first.")
}
