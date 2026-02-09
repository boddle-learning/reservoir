package user

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// Repository handles user data operations
type Repository struct {
	db *sqlx.DB
}

// NewRepository creates a new user repository
func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

// FindByEmail finds a user by email address
func (r *Repository) FindByEmail(ctx context.Context, email string) (*User, error) {
	var user User
	query := `SELECT id, email, password_digest, boddle_uid, meta_type, meta_id, last_logged_on, created_at, updated_at
			  FROM users
			  WHERE email = $1`

	err := r.db.GetContext(ctx, &user, query, email)
	if err == sql.ErrNoRows {
		return nil, nil // User not found
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find user by email: %w", err)
	}

	return &user, nil
}

// FindByID finds a user by ID
func (r *Repository) FindByID(ctx context.Context, id int) (*User, error) {
	var user User
	query := `SELECT id, email, password_digest, boddle_uid, meta_type, meta_id, last_logged_on, created_at, updated_at
			  FROM users
			  WHERE id = $1`

	err := r.db.GetContext(ctx, &user, query, id)
	if err == sql.ErrNoRows {
		return nil, nil // User not found
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find user by ID: %w", err)
	}

	return &user, nil
}

// FindByBoddleUID finds a user by Boddle UID
func (r *Repository) FindByBoddleUID(ctx context.Context, boddleUID string) (*User, error) {
	var user User
	query := `SELECT id, email, password_digest, boddle_uid, meta_type, meta_id, last_logged_on, created_at, updated_at
			  FROM users
			  WHERE boddle_uid = $1`

	err := r.db.GetContext(ctx, &user, query, boddleUID)
	if err == sql.ErrNoRows {
		return nil, nil // User not found
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find user by Boddle UID: %w", err)
	}

	return &user, nil
}

// FindWithMeta retrieves user with their meta data (Teacher/Student/Parent)
func (r *Repository) FindWithMeta(ctx context.Context, userID int) (*UserWithMeta, error) {
	user, err := r.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, nil
	}

	result := &UserWithMeta{User: *user}

	// Load meta data based on meta_type
	switch user.MetaType {
	case "Teacher":
		meta, err := r.FindTeacher(ctx, user.MetaID)
		if err != nil {
			return nil, err
		}
		result.Meta = meta

	case "Student":
		meta, err := r.FindStudent(ctx, user.MetaID)
		if err != nil {
			return nil, err
		}
		result.Meta = meta

	case "Parent":
		meta, err := r.FindParent(ctx, user.MetaID)
		if err != nil {
			return nil, err
		}
		result.Meta = meta
	}

	return result, nil
}

// FindTeacher finds a teacher by ID
func (r *Repository) FindTeacher(ctx context.Context, id int) (*Teacher, error) {
	var teacher Teacher
	query := `SELECT id, user_id, first_name, last_name, google_uid, clever_uid, verified, created_at, updated_at
			  FROM teachers
			  WHERE id = $1`

	err := r.db.GetContext(ctx, &teacher, query, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find teacher: %w", err)
	}

	return &teacher, nil
}

// FindTeacherByGoogleUID finds a teacher by Google UID
func (r *Repository) FindTeacherByGoogleUID(ctx context.Context, googleUID string) (*Teacher, error) {
	var teacher Teacher
	query := `SELECT id, user_id, first_name, last_name, google_uid, clever_uid, verified, created_at, updated_at
			  FROM teachers
			  WHERE google_uid = $1`

	err := r.db.GetContext(ctx, &teacher, query, googleUID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find teacher by Google UID: %w", err)
	}

	return &teacher, nil
}

// FindTeacherByCleverUID finds a teacher by Clever UID
func (r *Repository) FindTeacherByCleverUID(ctx context.Context, cleverUID string) (*Teacher, error) {
	var teacher Teacher
	query := `SELECT id, user_id, first_name, last_name, google_uid, clever_uid, verified, created_at, updated_at
			  FROM teachers
			  WHERE clever_uid = $1`

	err := r.db.GetContext(ctx, &teacher, query, cleverUID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find teacher by Clever UID: %w", err)
	}

	return &teacher, nil
}

// FindStudent finds a student by ID
func (r *Repository) FindStudent(ctx context.Context, id int) (*Student, error) {
	var student Student
	query := `SELECT id, user_id, username, first_name, last_name, google_uid, clever_uid, icloud_uid, created_at, updated_at
			  FROM students
			  WHERE id = $1`

	err := r.db.GetContext(ctx, &student, query, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find student: %w", err)
	}

	return &student, nil
}

// FindStudentByGoogleUID finds a student by Google UID
func (r *Repository) FindStudentByGoogleUID(ctx context.Context, googleUID string) (*Student, error) {
	var student Student
	query := `SELECT id, user_id, username, first_name, last_name, google_uid, clever_uid, icloud_uid, created_at, updated_at
			  FROM students
			  WHERE google_uid = $1`

	err := r.db.GetContext(ctx, &student, query, googleUID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find student by Google UID: %w", err)
	}

	return &student, nil
}

// FindParent finds a parent by ID
func (r *Repository) FindParent(ctx context.Context, id int) (*Parent, error) {
	var parent Parent
	query := `SELECT id, user_id, first_name, last_name, icloud_uid, created_at, updated_at
			  FROM parents
			  WHERE id = $1`

	err := r.db.GetContext(ctx, &parent, query, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find parent: %w", err)
	}

	return &parent, nil
}

// UpdateLastLoggedOn updates the last_logged_on timestamp
func (r *Repository) UpdateLastLoggedOn(ctx context.Context, userID int) error {
	query := `UPDATE users SET last_logged_on = $1 WHERE id = $2`
	_, err := r.db.ExecContext(ctx, query, time.Now(), userID)
	if err != nil {
		return fmt.Errorf("failed to update last logged on: %w", err)
	}
	return nil
}

// RecordLoginAttempt records a login attempt for rate limiting
func (r *Repository) RecordLoginAttempt(ctx context.Context, email, ipAddress string, success bool) error {
	query := `INSERT INTO login_attempts (email, ip_address, success, attempted_at)
			  VALUES ($1, $2, $3, $4)`

	_, err := r.db.ExecContext(ctx, query, email, ipAddress, success, time.Now())
	if err != nil {
		return fmt.Errorf("failed to record login attempt: %w", err)
	}
	return nil
}

// GetRecentLoginAttempts gets recent login attempts for rate limiting
func (r *Repository) GetRecentLoginAttempts(ctx context.Context, email, ipAddress string, since time.Time) ([]LoginAttempt, error) {
	var attempts []LoginAttempt
	query := `SELECT id, email, ip_address, success, attempted_at
			  FROM login_attempts
			  WHERE email = $1 AND ip_address = $2 AND attempted_at >= $3
			  ORDER BY attempted_at DESC`

	err := r.db.SelectContext(ctx, &attempts, query, email, ipAddress, since)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent login attempts: %w", err)
	}

	return attempts, nil
}

// FindLoginToken finds a login token by secret
func (r *Repository) FindLoginToken(ctx context.Context, secret string) (*LoginToken, error) {
	var token LoginToken
	query := `SELECT id, user_id, secret, permanent, created_at
			  FROM login_tokens
			  WHERE secret = $1`

	err := r.db.GetContext(ctx, &token, query, secret)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find login token: %w", err)
	}

	return &token, nil
}

// DeleteLoginToken deletes a login token (for non-permanent tokens after use)
func (r *Repository) DeleteLoginToken(ctx context.Context, id int) error {
	query := `DELETE FROM login_tokens WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete login token: %w", err)
	}
	return nil
}

// UpdateTeacherGoogleUID updates a teacher's Google UID
func (r *Repository) UpdateTeacherGoogleUID(ctx context.Context, teacherID int, googleUID string) error {
	query := `UPDATE teachers SET google_uid = $1, updated_at = $2 WHERE id = $3`
	_, err := r.db.ExecContext(ctx, query, googleUID, time.Now(), teacherID)
	if err != nil {
		return fmt.Errorf("failed to update teacher Google UID: %w", err)
	}
	return nil
}

// UpdateStudentGoogleUID updates a student's Google UID
func (r *Repository) UpdateStudentGoogleUID(ctx context.Context, studentID int, googleUID string) error {
	query := `UPDATE students SET google_uid = $1, updated_at = $2 WHERE id = $3`
	_, err := r.db.ExecContext(ctx, query, googleUID, time.Now(), studentID)
	if err != nil {
		return fmt.Errorf("failed to update student Google UID: %w", err)
	}
	return nil
}

// UpdateTeacherCleverUID updates a teacher's Clever UID
func (r *Repository) UpdateTeacherCleverUID(ctx context.Context, teacherID int, cleverUID string) error {
	query := `UPDATE teachers SET clever_uid = $1, updated_at = $2 WHERE id = $3`
	_, err := r.db.ExecContext(ctx, query, cleverUID, time.Now(), teacherID)
	if err != nil {
		return fmt.Errorf("failed to update teacher Clever UID: %w", err)
	}
	return nil
}

// UpdateStudentCleverUID updates a student's Clever UID
func (r *Repository) UpdateStudentCleverUID(ctx context.Context, studentID int, cleverUID string) error {
	query := `UPDATE students SET clever_uid = $1, updated_at = $2 WHERE id = $3`
	_, err := r.db.ExecContext(ctx, query, cleverUID, time.Now(), studentID)
	if err != nil {
		return fmt.Errorf("failed to update student Clever UID: %w", err)
	}
	return nil
}

// FindStudentByiCloudUID finds a student by iCloud UID
func (r *Repository) FindStudentByiCloudUID(ctx context.Context, icloudUID string) (*Student, error) {
	var student Student
	query := `SELECT id, user_id, username, first_name, last_name, google_uid, clever_uid, icloud_uid, created_at, updated_at
			  FROM students
			  WHERE icloud_uid = $1`

	err := r.db.GetContext(ctx, &student, query, icloudUID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find student by iCloud UID: %w", err)
	}

	return &student, nil
}

// FindParentByiCloudUID finds a parent by iCloud UID
func (r *Repository) FindParentByiCloudUID(ctx context.Context, icloudUID string) (*Parent, error) {
	var parent Parent
	query := `SELECT id, user_id, first_name, last_name, icloud_uid, created_at, updated_at
			  FROM parents
			  WHERE icloud_uid = $1`

	err := r.db.GetContext(ctx, &parent, query, icloudUID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find parent by iCloud UID: %w", err)
	}

	return &parent, nil
}

// FindStudentByCleverUID finds a student by Clever UID
func (r *Repository) FindStudentByCleverUID(ctx context.Context, cleverUID string) (*Student, error) {
	var student Student
	query := `SELECT id, user_id, username, first_name, last_name, google_uid, clever_uid, icloud_uid, created_at, updated_at
			  FROM students
			  WHERE clever_uid = $1`

	err := r.db.GetContext(ctx, &student, query, cleverUID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find student by Clever UID: %w", err)
	}

	return &student, nil
}

// UpdateStudentiCloudUID updates a student's iCloud UID
func (r *Repository) UpdateStudentiCloudUID(ctx context.Context, studentID int, icloudUID string) error {
	query := `UPDATE students SET icloud_uid = $1, updated_at = $2 WHERE id = $3`
	_, err := r.db.ExecContext(ctx, query, icloudUID, time.Now(), studentID)
	if err != nil {
		return fmt.Errorf("failed to update student iCloud UID: %w", err)
	}
	return nil
}

// UpdateParentiCloudUID updates a parent's iCloud UID
func (r *Repository) UpdateParentiCloudUID(ctx context.Context, parentID int, icloudUID string) error {
	query := `UPDATE parents SET icloud_uid = $1, updated_at = $2 WHERE id = $3`
	_, err := r.db.ExecContext(ctx, query, icloudUID, time.Now(), parentID)
	if err != nil {
		return fmt.Errorf("failed to update parent iCloud UID: %w", err)
	}
	return nil
}
