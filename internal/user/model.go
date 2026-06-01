package user

import (
	"database/sql"
	"time"
)

// User represents the users table (polymorphic base)
type User struct {
	ID             int            `db:"id" json:"id"`
	Name           string         `db:"name" json:"name"`
	Email          string         `db:"email" json:"email"`
	PasswordDigest string         `db:"password_digest" json:"-"`
	BoddleUID      sql.NullString `db:"boddle_uid" json:"boddle_uid,omitempty"`
	MetaType       string         `db:"meta_type" json:"meta_type"`
	MetaID         int            `db:"meta_id" json:"meta_id"`
	LastLoggedOn   sql.NullTime   `db:"last_logged_on" json:"last_logged_on,omitempty"`
	TokenVersion   int            `db:"token_version" json:"-"`
	CreatedAt      time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time      `db:"updated_at" json:"updated_at"`
}

// Teacher represents the teachers table
type Teacher struct {
	ID         int            `db:"id" json:"id"`
	FirstName  string         `db:"first_name" json:"first_name"`
	LastName   string         `db:"last_name" json:"last_name"`
	GoogleUID  sql.NullString `db:"google_uid" json:"google_uid,omitempty"`
	CleverUID  sql.NullString `db:"clever_uid" json:"clever_uid,omitempty"`
	IsVerified bool           `db:"is_verified" json:"is_verified"`
	CreatedAt  time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt  time.Time      `db:"updated_at" json:"updated_at"`
}

// Student represents the students table
// Note: students don't have first_name/last_name/username columns.
// The display name comes from users.name via the polymorphic association.
type Student struct {
	ID                int            `db:"id" json:"id"`
	GameCharacterName sql.NullString `db:"game_character_name" json:"game_character_name,omitempty"`
	GoogleUID         sql.NullString `db:"google_uid" json:"google_uid,omitempty"`
	CleverUID         sql.NullString `db:"clever_uid" json:"clever_uid,omitempty"`
	ICloudUID         sql.NullString `db:"icloud_uid" json:"icloud_uid,omitempty"`
	ParentID          sql.NullInt64  `db:"parent_id" json:"parent_id,omitempty"`
	CreatedAt         time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt         time.Time      `db:"updated_at" json:"updated_at"`
}

// Parent represents the parents table
type Parent struct {
	ID        int            `db:"id" json:"id"`
	FirstName string         `db:"first_name" json:"first_name"`
	LastName  string         `db:"last_name" json:"last_name"`
	ICloudUID sql.NullString `db:"icloud_uid" json:"icloud_uid,omitempty"`
	CreatedAt time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt time.Time      `db:"updated_at" json:"updated_at"`
}

// LoginAttempt represents the login_attempts table for rate limiting
type LoginAttempt struct {
	ID          int       `db:"id" json:"id"`
	Email       string    `db:"email" json:"email"`
	IPAddress   string    `db:"ip_address" json:"ip_address"`
	Success     bool      `db:"success" json:"success"`
	AttemptedAt time.Time `db:"attempted_at" json:"attempted_at"`
}

// LoginToken represents the login_tokens table for magic links
type LoginToken struct {
	ID        int       `db:"id" json:"id"`
	UserID    int       `db:"user_id" json:"user_id"`
	Secret    string    `db:"secret" json:"secret"`
	Permanent bool      `db:"permanent" json:"permanent"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

// UserWithMeta combines User with their meta type data (Teacher/Student/Parent)
type UserWithMeta struct {
	User   User
	Meta   interface{} // Can be Teacher, Student, or Parent
}

// GetFullName returns the full name based on meta type.
// Teachers and Parents have first_name/last_name on their own tables.
// Students do not — their name comes from users.name.
func (u *UserWithMeta) GetFullName() string {
	switch meta := u.Meta.(type) {
	case *Teacher:
		return meta.FirstName + " " + meta.LastName
	case *Student:
		return u.User.Name
	case *Parent:
		return meta.FirstName + " " + meta.LastName
	default:
		return u.User.Name
	}
}
