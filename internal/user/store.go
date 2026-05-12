package user

import (
	"time"

	"github.com/shifter-io/shifter/internal/auth"
)

// UserDTO is the JSON wire shape returned by every /api/users endpoint. It
// intentionally OMITS PasswordHash so the admin Users table cannot leak hash
// material to the browser even if a future audit drift forgets to redact a
// field (T-06-05-10 mitigation).
type UserDTO struct {
	ID                 string     `json:"id"`
	Email              string     `json:"email"`
	Name               string     `json:"name"`
	Role               string     `json:"role"`
	MustChangePassword bool       `json:"must_change_password"`
	DisabledAt         *time.Time `json:"disabled_at,omitempty"`
	LastLoginAt        *time.Time `json:"last_login_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// ToDTO copies an auth.UserRecord into a UserDTO, dropping the password hash.
// The disabled_at / last_login_at fields stay pointer-shaped so JSON nulls
// pass through cleanly for never-logged-in or active users.
func ToDTO(u auth.UserRecord) UserDTO {
	return UserDTO{
		ID:                 u.ID,
		Email:              u.Email,
		Name:               u.Name,
		Role:               u.Role,
		MustChangePassword: u.MustChangePassword,
		DisabledAt:         u.DisabledAt,
		LastLoginAt:        u.LastLoginAt,
		CreatedAt:          u.CreatedAt,
		UpdatedAt:          u.UpdatedAt,
	}
}

// ToDTOs maps a slice of UserRecord into a slice of DTOs, preserving order.
func ToDTOs(rows []auth.UserRecord) []UserDTO {
	out := make([]UserDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, ToDTO(r))
	}
	return out
}
