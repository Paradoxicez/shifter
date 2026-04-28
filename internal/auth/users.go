// Package auth — user store wrapper around the pgxpool.
//
// `Store` provides the narrow query surface needed by Plan 09 (login,
// create-admin) and Plan 11 (account UI / change password). It uses raw SQL
// (not sqlc) for now because the auth package must not import internal/db/sqlc
// without an import-cycle risk and the queries here are stable + few.
//
// When richer transactional flows arrive (Plan 14/15 install handlers), those
// plans use the sqlc-generated helpers directly via internal/db/sqlc.
package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrUserNotFound is returned by GetUserByEmail when no row matches and the
// caller should respond 401 / "bad_credentials". Distinct from a generic DB
// error so the login handler can distinguish "wrong email" from "DB blew up".
var ErrUserNotFound = errors.New("auth: user not found")

// UserRecord is the persisted representation, distinct from auth.User
// (in-session, primitives only).
type UserRecord struct {
	ID                 string
	Email              string
	Name               string
	PasswordHash       string
	Role               string
	MustChangePassword bool
}

// Store is the user/session DB facade.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore wraps a pgxpool.Pool.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Pool exposes the underlying pgxpool for callers that need to issue
// transactional queries beyond the Store's surface (account.go uses this for
// the iterate-and-revoke session cleanup).
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// GetUserByEmail loads a user by email. Email must be lowercased by the caller
// (the table has a CHECK constraint enforcing the invariant). Disabled users
// (disabled_at IS NOT NULL) are NOT returned — they appear as ErrUserNotFound.
func (s *Store) GetUserByEmail(ctx context.Context, email string) (*UserRecord, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id::text, email, name, password_hash, role::text, must_change_password
           FROM "user" WHERE email = $1 AND disabled_at IS NULL`, email)
	var u UserRecord
	if err := row.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Role, &u.MustChangePassword); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("get user: %w", err)
	}
	return &u, nil
}

// GetUserByID loads an active user by UUID string.
func (s *Store) GetUserByID(ctx context.Context, id string) (*UserRecord, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT id::text, email, name, password_hash, role::text, must_change_password
           FROM "user" WHERE id = $1::uuid AND disabled_at IS NULL`, id)
	var u UserRecord
	if err := row.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Role, &u.MustChangePassword); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("get user: %w", err)
	}
	return &u, nil
}

// AdminExists reports whether at least one active admin row exists. Used by
// Plan 14's install middleware (gate the wizard on absence of any admin) and
// by `shifter create-admin` (decide create-vs-reset).
func (s *Store) AdminExists(ctx context.Context) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM "user" WHERE role = 'admin' AND disabled_at IS NULL)`,
	).Scan(&exists)
	return exists, err
}

// InsertAdminUser creates a new admin row with `must_change_password=false`
// (D-09: wizard / create-admin admins set their own password; force-change is
// for operator-provisioned default admins, which Shifter does not have).
//
// Returns the new UUID as a string. Email is lowercased before insert to
// satisfy the CHECK constraint defensively even if a caller forgot to.
func (s *Store) InsertAdminUser(ctx context.Context, email, name, passwordHash string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role, must_change_password)
            VALUES (lower($1), $2, $3, 'admin', FALSE)
            RETURNING id::text`, email, name, passwordHash,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("insert admin: %w", err)
	}
	return id, nil
}

// UpdatePassword replaces the password_hash for the given user UUID. The
// updated_at trigger (touch_updated_at) bumps automatically.
func (s *Store) UpdatePassword(ctx context.Context, userID, passwordHash string) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE "user" SET password_hash = $2 WHERE id = $1::uuid AND disabled_at IS NULL`,
		userID, passwordHash)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}
