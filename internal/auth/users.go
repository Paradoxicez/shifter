// Package auth — user store wrapper around the pgxpool.
//
// `Store` provides the narrow query surface needed by Plan 09 (login,
// create-admin) and Plan 11 (account UI / change password). It uses raw SQL
// (not sqlc) for now because the auth package must not import internal/db/sqlc
// without an import-cycle risk and the queries here are stable + few.
//
// When richer transactional flows arrive (Plan 14/15 install handlers), those
// plans use the sqlc-generated helpers directly via internal/db/sqlc.
//
// Plan 06-05 extends the Store with administrative user-management methods
// (List, CreateTx, UpdateTx, DisableTx, EnableTx, ChangeRoleTx,
// UpdatePasswordTx, CountActiveAdminsExcluding, GetByIDForUpdate). These are
// the building blocks consumed by internal/user/handler.go. All mutating
// methods take a pgx.Tx so the caller can wrap the user mutation + audit row
// + (optionally) IterateAndRevoke in a single Serializable transaction —
// the Pitfall 6 TOCTOU mitigation for the "last admin demote" race.
package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrUserNotFound is returned by GetUserByEmail when no row matches and the
// caller should respond 401 / "bad_credentials". Distinct from a generic DB
// error so the login handler can distinguish "wrong email" from "DB blew up".
var ErrUserNotFound = errors.New("auth: user not found")

// ErrDuplicateEmail is returned by CreateTx when the email already exists.
// Distinct from a generic DB error so handlers can respond 409 instead of 500.
var ErrDuplicateEmail = errors.New("auth: duplicate email")

// ErrInvalidScope is returned by List for an unrecognized scope.
var ErrInvalidScope = errors.New("auth: invalid List scope")

// UserRecord is the persisted representation, distinct from auth.User
// (in-session, primitives only).
//
// Phase 6 extends with DisabledAt / CreatedAt / UpdatedAt / LastLoginAt for the
// admin Users table (Surface 5). The narrow login/account paths from Phase 1
// don't populate these — they remain zero values when loaded by
// GetUserByEmail / GetUserByID.
type UserRecord struct {
	ID                 string
	Email              string
	Name               string
	PasswordHash       string
	Role               string
	MustChangePassword bool
	DisabledAt         *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
	LastLoginAt        *time.Time
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

// ─────────────────────────────────────────────────────────────────────────
// Phase 6 — Plan 06-05 user management extensions.
//
// All mutating methods take a pgx.Tx so the caller can wrap the mutation +
// audit row + session revoke in a single Serializable transaction.
// ─────────────────────────────────────────────────────────────────────────

// scanUserRecordFull is the column shape used by List and GetByIDForUpdate —
// every UserRecord field is populated.
const userColumnsFull = `id::text, email, name, password_hash, role::text,
        must_change_password, disabled_at, created_at, updated_at, last_login_at`

func scanUserRecordFull(scanner interface {
	Scan(dest ...any) error
}) (*UserRecord, error) {
	var u UserRecord
	if err := scanner.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Role,
		&u.MustChangePassword, &u.DisabledAt, &u.CreatedAt, &u.UpdatedAt, &u.LastLoginAt); err != nil {
		return nil, err
	}
	return &u, nil
}

// List returns users filtered by scope. scope must be "active" (disabled_at
// IS NULL), "disabled" (disabled_at IS NOT NULL), or "all".
//
// Rows are sorted by created_at DESC so the admin Users table renders newest
// users first (UI-SPEC Surface 5: "Sort: created_at DESC; current user pinned
// top" — the pinning is a client-side concern).
func (s *Store) List(ctx context.Context, scope string) ([]UserRecord, error) {
	var where string
	switch scope {
	case "active":
		where = "WHERE disabled_at IS NULL"
	case "disabled":
		where = "WHERE disabled_at IS NOT NULL"
	case "all":
		where = ""
	default:
		return nil, fmt.Errorf("%w %q", ErrInvalidScope, scope)
	}
	rows, err := s.pool.Query(ctx,
		`SELECT `+userColumnsFull+` FROM "user" `+where+` ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	var out []UserRecord
	for rows.Next() {
		rec, err := scanUserRecordFull(rows)
		if err != nil {
			return nil, fmt.Errorf("scan user row: %w", err)
		}
		out = append(out, *rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user rows: %w", err)
	}
	return out, nil
}

// GetByIDForUpdate loads a user row with SELECT ... FOR UPDATE inside the
// caller's tx. Pairs with CountActiveAdminsExcluding under a Serializable tx
// to mitigate the Pitfall 6 TOCTOU race on last-admin demote.
//
// Unlike GetUserByID (which filters disabled rows for the login path), this
// method returns disabled rows too — admin re-enable / view-history flows
// need them.
func (s *Store) GetByIDForUpdate(ctx context.Context, tx pgx.Tx, id string) (*UserRecord, error) {
	row := tx.QueryRow(ctx,
		`SELECT `+userColumnsFull+` FROM "user" WHERE id = $1::uuid FOR UPDATE`, id)
	u, err := scanUserRecordFull(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("get user for update: %w", err)
	}
	return u, nil
}

// CreateTx inserts a new user row. role must be "admin" or "viewer"; the
// caller validates before calling. Email is lowercased so the
// user_email_lowercase CHECK passes. Returns ErrDuplicateEmail (23505) when
// the email already exists.
func (s *Store) CreateTx(ctx context.Context, tx pgx.Tx, email, name, role, passwordHash string, mustChange bool) (string, error) {
	var id string
	err := tx.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role, must_change_password)
            VALUES (lower($1), $2, $3, $4::user_role, $5)
            RETURNING id::text`, email, name, passwordHash, role, mustChange,
	).Scan(&id)
	if err != nil {
		// Postgres unique-violation = 23505. The user_email_unique index is
		// the only unique constraint on this table.
		if isUniqueViolation(err) {
			return "", ErrDuplicateEmail
		}
		return "", fmt.Errorf("create user: %w", err)
	}
	return id, nil
}

// UpdateTx changes the user's display name. Email and role are NOT updatable
// from this method (D-26 routes role through ChangeRoleTx so the
// session-revoke side-effect is forced; email changes are out of scope for
// Phase 6).
func (s *Store) UpdateTx(ctx context.Context, tx pgx.Tx, id, name string) error {
	tag, err := tx.Exec(ctx,
		`UPDATE "user" SET name = $2 WHERE id = $1::uuid`, id, name)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// DisableTx marks the user soft-deleted by setting disabled_at = now(). The
// row remains so audit history continues to reference it (Phase 2 D-20 no-
// hard-delete pattern, mirrored for users per D-27).
func (s *Store) DisableTx(ctx context.Context, tx pgx.Tx, id string) error {
	tag, err := tx.Exec(ctx,
		`UPDATE "user" SET disabled_at = now() WHERE id = $1::uuid`, id)
	if err != nil {
		return fmt.Errorf("disable user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// EnableTx clears disabled_at so the user can sign in again. Per D-27,
// password_hash and must_change_password are NOT touched — the user resumes
// with the same credentials they had at disable time.
func (s *Store) EnableTx(ctx context.Context, tx pgx.Tx, id string) error {
	tag, err := tx.Exec(ctx,
		`UPDATE "user" SET disabled_at = NULL WHERE id = $1::uuid`, id)
	if err != nil {
		return fmt.Errorf("enable user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// ChangeRoleTx updates the user's role. newRole must be "admin" or "viewer"
// (the caller validates; the DB user_role enum will also raise 22P02 on
// invalid input). The caller MUST call IterateAndRevoke after commit so the
// user's existing sessions cannot keep operating with the previous role
// (D-25 + D-24).
func (s *Store) ChangeRoleTx(ctx context.Context, tx pgx.Tx, id, newRole string) error {
	tag, err := tx.Exec(ctx,
		`UPDATE "user" SET role = $2::user_role WHERE id = $1::uuid`, id, newRole)
	if err != nil {
		return fmt.Errorf("change role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// UpdatePasswordTx replaces the password_hash AND sets must_change_password
// in a single update. The Phase 1 UpdatePassword method clears
// must_change_password implicitly (the user updated their own); this method
// is for admin Reset Password where the admin forces the user to rotate
// again on next login (mustChange=true).
func (s *Store) UpdatePasswordTx(ctx context.Context, tx pgx.Tx, id, passwordHash string, mustChange bool) error {
	tag, err := tx.Exec(ctx,
		`UPDATE "user" SET password_hash = $2, must_change_password = $3 WHERE id = $1::uuid`,
		id, passwordHash, mustChange)
	if err != nil {
		return fmt.Errorf("update password tx: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// CountActiveAdminsExcluding counts active admins whose id is NOT the
// excluded one. Used inside a Serializable tx by RejectLastAdminDemote.
//
// "Active admin" means role='admin' AND disabled_at IS NULL. A disabled admin
// can still be excluded — they don't count, so excluding them doesn't change
// the result.
func (s *Store) CountActiveAdminsExcluding(ctx context.Context, tx pgx.Tx, excludeID string) (int, error) {
	var n int
	err := tx.QueryRow(ctx,
		`SELECT count(*) FROM "user"
            WHERE role = 'admin' AND disabled_at IS NULL AND id <> $1::uuid`,
		excludeID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count active admins: %w", err)
	}
	return n, nil
}

// GetUserByEmailTx loads a user by email inside the caller's transaction.
// Same semantics as GetUserByEmail (returns ErrUserNotFound for disabled users)
// but uses tx.QueryRow so the lookup is part of the caller's atomic envelope.
// Added by Plan 06-06 for the audit-in-tx login retrofit (D-30).
func (s *Store) GetUserByEmailTx(ctx context.Context, tx pgx.Tx, email string) (*UserRecord, error) {
	row := tx.QueryRow(ctx,
		`SELECT id::text, email, name, password_hash, role::text, must_change_password
           FROM "user" WHERE email = $1 AND disabled_at IS NULL`, email)
	var u UserRecord
	if err := row.Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.Role, &u.MustChangePassword); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("get user by email tx: %w", err)
	}
	return &u, nil
}

// UpdateLastLoginAtTx sets user.last_login_at = now() inside the caller's tx.
// Called by the LoginHandler immediately before commit so a successful login
// is atomically reflected in the user's last_login_at + audit_log.
// Added by Plan 06-06 (D-30 auth-event audit retrofit).
func (s *Store) UpdateLastLoginAtTx(ctx context.Context, tx pgx.Tx, userID string) error {
	_, err := tx.Exec(ctx, `UPDATE "user" SET last_login_at = now() WHERE id = $1::uuid`, userID)
	if err != nil {
		return fmt.Errorf("update last_login_at: %w", err)
	}
	return nil
}

// isUniqueViolation reports whether err is a Postgres 23505 unique violation.
// Used by CreateTx to surface ErrDuplicateEmail instead of a generic wrap.
func isUniqueViolation(err error) bool {
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) {
		return pgErr.SQLState() == "23505"
	}
	return false
}
