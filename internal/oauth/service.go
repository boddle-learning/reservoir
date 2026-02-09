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
	icloudSvc    *iCloudService
}

// NewAuthService creates a new OAuth authentication service
func NewAuthService(
	userRepo *user.Repository,
	tokenService *token.Service,
	googleSvc *GoogleService,
	cleverSvc *CleverService,
	icloudSvc *iCloudService,
) *AuthService {
	return &AuthService{
		userRepo:     userRepo,
		tokenService: tokenService,
		googleSvc:    googleSvc,
		cleverSvc:    cleverSvc,
		icloudSvc:    icloudSvc,
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

	// Update last logged on
	if err := s.userRepo.UpdateLastLoggedOn(ctx, usr.ID); err != nil {
		fmt.Printf("failed to update last_logged_on: %v\n", err)
	}

	// Generate JWT token
	boddleUID := ""
	if usr.BoddleUID.Valid {
		boddleUID = usr.BoddleUID.String
	}

	fullName := ""
	switch m := meta.(type) {
	case *user.Teacher:
		fullName = m.FirstName + " " + m.LastName
	case *user.Student:
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
		// Found by Google UID
		usr, err := s.userRepo.FindByID(ctx, teacher.UserID)
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
		// Found by Google UID
		usr, err := s.userRepo.FindByID(ctx, student.UserID)
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

	// Update last logged on
	if err := s.userRepo.UpdateLastLoggedOn(ctx, usr.ID); err != nil {
		fmt.Printf("failed to update last_logged_on: %v\n", err)
	}

	// Generate JWT token
	boddleUID := ""
	if usr.BoddleUID.Valid {
		boddleUID = usr.BoddleUID.String
	}

	fullName := ""
	switch m := meta.(type) {
	case *user.Teacher:
		fullName = m.FirstName + " " + m.LastName
	case *user.Student:
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
		// Found by Clever UID
		usr, err := s.userRepo.FindByID(ctx, teacher.UserID)
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
		// Found by Clever UID
		usr, err := s.userRepo.FindByID(ctx, student.UserID)
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

// AuthenticateWithiCloud authenticates a user with iCloud Sign In
func (s *AuthService) AuthenticateWithiCloud(ctx context.Context, code, state string) (*auth.LoginResponse, string, error) {
	// Handle iCloud OAuth callback
	oauthUserInfo, redirectURL, err := s.icloudSvc.HandleCallback(ctx, code, state)
	if err != nil {
		return nil, "", err
	}

	// Find or create user
	usr, meta, err := s.findOrCreateiCloudUser(ctx, oauthUserInfo)
	if err != nil {
		return nil, "", err
	}

	// Update last logged on
	if err := s.userRepo.UpdateLastLoggedOn(ctx, usr.ID); err != nil {
		fmt.Printf("failed to update last_logged_on: %v\n", err)
	}

	// Generate JWT token
	boddleUID := ""
	if usr.BoddleUID.Valid {
		boddleUID = usr.BoddleUID.String
	}

	fullName := ""
	switch m := meta.(type) {
	case *user.Student:
		fullName = m.FirstName + " " + m.LastName
	case *user.Parent:
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

// findOrCreateiCloudUser finds an existing user by iCloud UID or email, or returns error
// Note: User creation is handled by Rails, so we only link existing accounts
func (s *AuthService) findOrCreateiCloudUser(ctx context.Context, info *OAuthUserInfo) (*user.User, interface{}, error) {
	// iCloud Sign In is primarily for students and parents
	// Try to find student by iCloud UID
	student, err := s.userRepo.FindStudentByiCloudUID(ctx, info.ProviderUserID)
	if err != nil {
		return nil, nil, err
	}

	if student != nil {
		// Found by iCloud UID
		usr, err := s.userRepo.FindByID(ctx, student.UserID)
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
		// Found by iCloud UID
		usr, err := s.userRepo.FindByID(ctx, parent.UserID)
		if err != nil {
			return nil, nil, err
		}
		return usr, parent, nil
	}

	// Try to find by email (account linking)
	usr, err := s.userRepo.FindByEmail(ctx, info.Email)
	if err != nil {
		return nil, nil, err
	}

	if usr == nil {
		return nil, nil, fmt.Errorf("no account found for this iCloud account. Please sign up first.")
	}

	// Link account by updating iCloud UID
	switch usr.MetaType {
	case "Student":
		student, err := s.userRepo.FindStudent(ctx, usr.MetaID)
		if err != nil {
			return nil, nil, err
		}
		if student == nil {
			return nil, nil, fmt.Errorf("student meta not found")
		}

		// Update iCloud UID
		if err := s.userRepo.UpdateStudentiCloudUID(ctx, student.ID, info.ProviderUserID); err != nil {
			return nil, nil, fmt.Errorf("failed to link iCloud account: %w", err)
		}

		student.ICloudUID = sql.NullString{String: info.ProviderUserID, Valid: true}
		return usr, student, nil

	case "Parent":
		parent, err := s.userRepo.FindParent(ctx, usr.MetaID)
		if err != nil {
			return nil, nil, err
		}
		if parent == nil {
			return nil, nil, fmt.Errorf("parent meta not found")
		}

		// Update iCloud UID
		if err := s.userRepo.UpdateParentiCloudUID(ctx, parent.ID, info.ProviderUserID); err != nil {
			return nil, nil, fmt.Errorf("failed to link iCloud account: %w", err)
		}

		parent.ICloudUID = sql.NullString{String: info.ProviderUserID, Valid: true}
		return usr, parent, nil

	default:
		return nil, nil, fmt.Errorf("unsupported user type for iCloud Sign In: %s (iCloud is for students and parents only)", usr.MetaType)
	}
}
