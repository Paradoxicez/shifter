// Package user — HTTP handlers for the Phase 6 admin user-management surface
// (USER-01..04 + D-23/D-24/D-25/D-26/D-27/D-30).
//
// Every mutating handler follows the same orchestration:
//
//  1. Decode + minimally validate the request body.
//  2. Run RejectSelfAction at the handler layer (D-26 server-side enforcement).
//  3. Open a Serializable transaction (TOCTOU mitigation per Pitfall 6).
//  4. Inside the tx: GetByIDForUpdate, run RejectLastAdminDemote when
//     applicable, run the Store mutation, write audit_log row(s).
//  5. Commit. On 40001 serialization failure, surface 409 so the operator can
//     retry.
//  6. After commit, call auth.IterateAndRevoke when the mutation needs to
//     "kick the user out everywhere" (D-24 implicit triggers: disable,
//     change-role, reset-password, logout-everywhere).
//
// Audit rows always land INSIDE the same tx as the mutation (D-30): a domain
// row literally cannot exist without its audit row.
package user

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shifter-io/shifter/internal/audit"
	"github.com/shifter-io/shifter/internal/auth"
)

// Deps bundles the shared infra every user-mgmt handler needs.
type Deps struct {
	Pool       *pgxpool.Pool
	Store      *auth.Store
	SessionMgr *scs.SessionManager
	Log        *slog.Logger
}

// ─────────────────────────────────────────────────────────────────────────
// Request / response shapes
// ─────────────────────────────────────────────────────────────────────────

type createRequest struct {
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}

type updateRequest struct {
	Name string `json:"name"`
}

type changeRoleRequest struct {
	Role string `json:"role"`
}

type errorResp struct {
	Error string `json:"error"`
}

// createResponse is the ONLY wire shape that carries the plaintext password
// — once, in the response body to POST /api/users + POST /api/users/{id}/reset-password.
// The Argon2id hash is the only persistent representation.
type createResponse struct {
	User            UserDTO `json:"user"`
	InitialPassword string  `json:"initial_password"`
}

type resetPasswordResponse struct {
	InitialPassword string `json:"initial_password"`
}

type logoutResponse struct {
	UserID string `json:"id"`
}

// ─────────────────────────────────────────────────────────────────────────
// Route registration
// ─────────────────────────────────────────────────────────────────────────

// RegisterRoutes mounts /api/users under r.
//
// Route table:
//
//	GET    /api/users                          user.list             — admin only
//	POST   /api/users                          user.create           — admin only
//	PATCH  /api/users/{id}                     user.update           — admin only
//	POST   /api/users/{id}/disable             user.disable          — admin only
//	POST   /api/users/{id}/enable              user.enable           — admin only
//	PATCH  /api/users/{id}/role                user.change_role      — admin only
//	POST   /api/users/{id}/reset-password      user.reset_password   — admin only
//	POST   /api/users/{id}/logout-everywhere   user.logout_everywhere — admin only
func RegisterRoutes(r chi.Router, deps Deps) {
	r.Route("/api/users", func(r chi.Router) {
		r.Group(func(rt chi.Router) {
			rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionUserList))
			rt.Get("/", ListHandler(deps))
		})
		r.Group(func(rt chi.Router) {
			rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionUserCreate))
			rt.Post("/", CreateHandler(deps))
		})
		r.Group(func(rt chi.Router) {
			rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionUserUpdate))
			rt.Patch("/{id}", UpdateHandler(deps))
		})
		r.Group(func(rt chi.Router) {
			rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionUserDisable))
			rt.Post("/{id}/disable", DisableHandler(deps))
		})
		r.Group(func(rt chi.Router) {
			rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionUserEnable))
			rt.Post("/{id}/enable", EnableHandler(deps))
		})
		r.Group(func(rt chi.Router) {
			rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionUserChangeRole))
			rt.Patch("/{id}/role", ChangeRoleHandler(deps))
		})
		r.Group(func(rt chi.Router) {
			rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionUserResetPassword))
			rt.Post("/{id}/reset-password", ResetPasswordHandler(deps))
		})
		r.Group(func(rt chi.Router) {
			rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionUserLogoutEverywhere))
			rt.Post("/{id}/logout-everywhere", LogoutEverywhereHandler(deps))
		})
	})
}

// ─────────────────────────────────────────────────────────────────────────
// Handlers
// ─────────────────────────────────────────────────────────────────────────

// ListHandler returns GET /api/users[?scope=active|disabled|all]. scope
// defaults to "active". RoleViewer is blocked at the RequireAction
// middleware (T-06-05-01).
func ListHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		scope := strings.TrimSpace(r.URL.Query().Get("scope"))
		if scope == "" {
			scope = "active"
		}
		rows, err := deps.Store.List(r.Context(), scope)
		if err != nil {
			if errors.Is(err, auth.ErrInvalidScope) {
				writeJSON(w, http.StatusBadRequest, errorResp{Error: "invalid_scope"})
				return
			}
			internalError(deps.Log, w, "list users", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"users": ToDTOs(rows),
		})
	}
}

// CreateHandler returns POST /api/users. Backend generates a random
// password (D-23) and returns the plaintext exactly once in the response
// body. The persisted hash is Argon2id.
func CreateHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acting, ok := auth.GetUser(r.Context(), deps.SessionMgr)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errorResp{Error: "unauthorized"})
			return
		}
		var req createRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "bad_json"})
			return
		}
		email := strings.ToLower(strings.TrimSpace(req.Email))
		name := strings.TrimSpace(req.Name)
		role := strings.TrimSpace(req.Role)
		if email == "" || name == "" {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "missing_fields"})
			return
		}
		if _, err := mail.ParseAddress(email); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "invalid_email"})
			return
		}
		if role != "admin" && role != "viewer" {
			writeJSON(w, http.StatusUnprocessableEntity, errorResp{Error: "invalid_role"})
			return
		}

		// Generate the random password (D-23) BEFORE the tx so the
		// expensive Argon2id hash work is not held inside the SERIALIZABLE
		// envelope.
		plaintext, err := GenerateRandomPassword()
		if err != nil {
			internalError(deps.Log, w, "generate password", err)
			return
		}
		hash, err := auth.Hash(plaintext)
		if err != nil {
			internalError(deps.Log, w, "hash password", err)
			return
		}

		ctx := r.Context()
		tx, err := deps.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			internalError(deps.Log, w, "begin tx", err)
			return
		}
		defer func() { _ = tx.Rollback(ctx) }()

		newID, err := deps.Store.CreateTx(ctx, tx, email, name, role, hash, true /*mustChange*/)
		if err != nil {
			if errors.Is(err, auth.ErrDuplicateEmail) {
				writeJSON(w, http.StatusConflict, errorResp{Error: "duplicate_email"})
				return
			}
			internalError(deps.Log, w, "create user", err)
			return
		}

		// Audit in tx (D-30). After diff includes the new role; password
		// hash NEVER lands in audit_log.
		if err := audit.WriteEntry(ctx, tx, audit.Entry{
			UserID:     uuid.MustParse(acting.ID),
			Action:     audit.ActionUserCreate,
			EntityType: audit.EntityTypeUser,
			EntityID:   uuid.MustParse(newID),
			After: map[string]any{
				"email": email,
				"name":  name,
				"role":  role,
			},
			RequestID: middleware.GetReqID(ctx),
		}); err != nil {
			internalError(deps.Log, w, "audit user.create", err)
			return
		}

		if err := tx.Commit(ctx); err != nil {
			if isSerializationFailure(err) {
				writeJSON(w, http.StatusConflict, errorResp{Error: "serialization_retry"})
				return
			}
			internalError(deps.Log, w, "commit", err)
			return
		}

		// Reload the row to return the full DTO with created_at /
		// updated_at populated. Use a plain read tx (default isolation)
		// and commit immediately so the connection returns to the pool.
		readTx, err := deps.Pool.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			internalError(deps.Log, w, "begin read tx", err)
			return
		}
		rec, err := deps.Store.GetByIDForUpdate(ctx, readTx, newID)
		_ = readTx.Commit(ctx)
		if err != nil {
			internalError(deps.Log, w, "reload created user", err)
			return
		}

		writeJSON(w, http.StatusCreated, createResponse{
			User:            ToDTO(*rec),
			InitialPassword: plaintext,
		})
	}
}

// UpdateHandler returns PATCH /api/users/{id}. Only the name field is
// updatable (role goes through ChangeRoleHandler; email is out of scope for
// Phase 6).
func UpdateHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acting, ok := auth.GetUser(r.Context(), deps.SessionMgr)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errorResp{Error: "unauthorized"})
			return
		}
		targetID, ok := parseIDParam(w, r)
		if !ok {
			return
		}
		var req updateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "bad_json"})
			return
		}
		name := strings.TrimSpace(req.Name)
		if name == "" {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "missing_name"})
			return
		}

		ctx := r.Context()
		tx, err := deps.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			internalError(deps.Log, w, "begin tx", err)
			return
		}
		defer func() { _ = tx.Rollback(ctx) }()

		before, err := deps.Store.GetByIDForUpdate(ctx, tx, targetID)
		if err != nil {
			if errors.Is(err, auth.ErrUserNotFound) {
				writeJSON(w, http.StatusNotFound, errorResp{Error: "not_found"})
				return
			}
			internalError(deps.Log, w, "load user", err)
			return
		}
		if before.Name == name {
			// No-op — still return 200 with the current DTO. No audit row
			// for a noop (audit captures state changes, not state reads).
			writeJSON(w, http.StatusOK, ToDTO(*before))
			return
		}
		if err := deps.Store.UpdateTx(ctx, tx, targetID, name); err != nil {
			internalError(deps.Log, w, "update user", err)
			return
		}
		if err := audit.WriteEntry(ctx, tx, audit.Entry{
			UserID:     uuid.MustParse(acting.ID),
			Action:     audit.ActionUserUpdate,
			EntityType: audit.EntityTypeUser,
			EntityID:   uuid.MustParse(targetID),
			Before:     map[string]any{"name": before.Name},
			After:      map[string]any{"name": name},
			RequestID:  middleware.GetReqID(ctx),
		}); err != nil {
			internalError(deps.Log, w, "audit user.update", err)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			if isSerializationFailure(err) {
				writeJSON(w, http.StatusConflict, errorResp{Error: "serialization_retry"})
				return
			}
			internalError(deps.Log, w, "commit", err)
			return
		}
		// Reload via a fresh read tx (committed immediately so the
		// connection returns to the pool) to surface the bumped updated_at.
		readTx, err := deps.Pool.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			internalError(deps.Log, w, "begin read tx", err)
			return
		}
		after, err := deps.Store.GetByIDForUpdate(ctx, readTx, targetID)
		_ = readTx.Commit(ctx)
		if err != nil {
			internalError(deps.Log, w, "reload user", err)
			return
		}
		writeJSON(w, http.StatusOK, ToDTO(*after))
	}
}

// DisableHandler returns POST /api/users/{id}/disable. Sets disabled_at
// and revokes all sessions for the user (D-24 implicit trigger 1 of 4).
func DisableHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acting, ok := auth.GetUser(r.Context(), deps.SessionMgr)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errorResp{Error: "unauthorized"})
			return
		}
		targetID, ok := parseIDParam(w, r)
		if !ok {
			return
		}
		if err := RejectSelfAction(acting.ID, targetID); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, errorResp{Error: "self_action_forbidden"})
			return
		}

		ctx := r.Context()
		tx, err := deps.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			internalError(deps.Log, w, "begin tx", err)
			return
		}
		defer func() { _ = tx.Rollback(ctx) }()

		before, err := deps.Store.GetByIDForUpdate(ctx, tx, targetID)
		if err != nil {
			if errors.Is(err, auth.ErrUserNotFound) {
				writeJSON(w, http.StatusNotFound, errorResp{Error: "not_found"})
				return
			}
			internalError(deps.Log, w, "load user", err)
			return
		}
		if before.DisabledAt != nil {
			// Idempotent — already disabled. Return 200 with the current
			// DTO; do not emit a duplicate audit row.
			writeJSON(w, http.StatusOK, ToDTO(*before))
			return
		}
		if err := deps.Store.DisableTx(ctx, tx, targetID); err != nil {
			internalError(deps.Log, w, "disable user", err)
			return
		}
		if err := audit.WriteEntry(ctx, tx, audit.Entry{
			UserID:     uuid.MustParse(acting.ID),
			Action:     audit.ActionUserDisable,
			EntityType: audit.EntityTypeUser,
			EntityID:   uuid.MustParse(targetID),
			Before:     map[string]any{"disabled_at": nil},
			After:      map[string]any{"disabled_at": "now"},
			RequestID:  middleware.GetReqID(ctx),
		}); err != nil {
			internalError(deps.Log, w, "audit user.disable", err)
			return
		}
		if err := audit.WriteEntry(ctx, tx, audit.Entry{
			UserID:     uuid.MustParse(acting.ID),
			Action:     audit.ActionAuthSessionRevoked,
			EntityType: audit.EntityTypeSession,
			EntityID:   uuid.MustParse(targetID),
			Notes:      "implicit revoke on user.disable",
			RequestID:  middleware.GetReqID(ctx),
		}); err != nil {
			internalError(deps.Log, w, "audit auth.session_revoked", err)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			if isSerializationFailure(err) {
				writeJSON(w, http.StatusConflict, errorResp{Error: "serialization_retry"})
				return
			}
			internalError(deps.Log, w, "commit", err)
			return
		}
		revokeAfterCommit(ctx, deps, targetID)
		writeJSON(w, http.StatusOK, map[string]any{"id": targetID, "disabled": true})
	}
}

// EnableHandler returns POST /api/users/{id}/enable. Clears disabled_at;
// password_hash and must_change_password preserved per D-27.
func EnableHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acting, ok := auth.GetUser(r.Context(), deps.SessionMgr)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errorResp{Error: "unauthorized"})
			return
		}
		targetID, ok := parseIDParam(w, r)
		if !ok {
			return
		}

		ctx := r.Context()
		tx, err := deps.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			internalError(deps.Log, w, "begin tx", err)
			return
		}
		defer func() { _ = tx.Rollback(ctx) }()

		before, err := deps.Store.GetByIDForUpdate(ctx, tx, targetID)
		if err != nil {
			if errors.Is(err, auth.ErrUserNotFound) {
				writeJSON(w, http.StatusNotFound, errorResp{Error: "not_found"})
				return
			}
			internalError(deps.Log, w, "load user", err)
			return
		}
		if before.DisabledAt == nil {
			// Idempotent — already enabled.
			writeJSON(w, http.StatusOK, ToDTO(*before))
			return
		}
		if err := deps.Store.EnableTx(ctx, tx, targetID); err != nil {
			internalError(deps.Log, w, "enable user", err)
			return
		}
		if err := audit.WriteEntry(ctx, tx, audit.Entry{
			UserID:     uuid.MustParse(acting.ID),
			Action:     audit.ActionUserEnable,
			EntityType: audit.EntityTypeUser,
			EntityID:   uuid.MustParse(targetID),
			Before:     map[string]any{"disabled_at": before.DisabledAt.Format("2006-01-02T15:04:05Z07:00")},
			After:      map[string]any{"disabled_at": nil},
			RequestID:  middleware.GetReqID(ctx),
		}); err != nil {
			internalError(deps.Log, w, "audit user.enable", err)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			if isSerializationFailure(err) {
				writeJSON(w, http.StatusConflict, errorResp{Error: "serialization_retry"})
				return
			}
			internalError(deps.Log, w, "commit", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": targetID, "disabled": false})
	}
}

// ChangeRoleHandler returns PATCH /api/users/{id}/role.
// Triggers session revoke per D-25 (one of D-24's four implicit triggers).
// Rejects self-role-change (D-26) and last-admin-demote with TOCTOU
// mitigation (Pitfall 6 — Serializable tx + GetByIDForUpdate).
func ChangeRoleHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acting, ok := auth.GetUser(r.Context(), deps.SessionMgr)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errorResp{Error: "unauthorized"})
			return
		}
		targetID, ok := parseIDParam(w, r)
		if !ok {
			return
		}
		var req changeRoleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "bad_json"})
			return
		}
		newRole := strings.TrimSpace(req.Role)
		if newRole != "admin" && newRole != "viewer" {
			writeJSON(w, http.StatusUnprocessableEntity, errorResp{Error: "invalid_role"})
			return
		}
		if err := RejectSelfAction(acting.ID, targetID); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, errorResp{Error: "self_action_forbidden"})
			return
		}

		ctx := r.Context()
		tx, err := deps.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			internalError(deps.Log, w, "begin tx", err)
			return
		}
		defer func() { _ = tx.Rollback(ctx) }()

		before, err := deps.Store.GetByIDForUpdate(ctx, tx, targetID)
		if err != nil {
			if errors.Is(err, auth.ErrUserNotFound) {
				writeJSON(w, http.StatusNotFound, errorResp{Error: "not_found"})
				return
			}
			internalError(deps.Log, w, "load user", err)
			return
		}
		if before.Role == newRole {
			// Idempotent no-op.
			writeJSON(w, http.StatusOK, ToDTO(*before))
			return
		}
		if err := RejectLastAdminDemote(ctx, tx, deps.Store, targetID, newRole); err != nil {
			if errors.Is(err, ErrLastAdmin) {
				writeJSON(w, http.StatusUnprocessableEntity, errorResp{Error: "last_admin"})
				return
			}
			internalError(deps.Log, w, "guard last-admin", err)
			return
		}
		if err := deps.Store.ChangeRoleTx(ctx, tx, targetID, newRole); err != nil {
			internalError(deps.Log, w, "change role", err)
			return
		}
		if err := audit.WriteEntry(ctx, tx, audit.Entry{
			UserID:     uuid.MustParse(acting.ID),
			Action:     audit.ActionUserRoleChange,
			EntityType: audit.EntityTypeUser,
			EntityID:   uuid.MustParse(targetID),
			Before:     map[string]any{"role": before.Role},
			After:      map[string]any{"role": newRole},
			RequestID:  middleware.GetReqID(ctx),
		}); err != nil {
			internalError(deps.Log, w, "audit user.role_change", err)
			return
		}
		if err := audit.WriteEntry(ctx, tx, audit.Entry{
			UserID:     uuid.MustParse(acting.ID),
			Action:     audit.ActionAuthSessionRevoked,
			EntityType: audit.EntityTypeSession,
			EntityID:   uuid.MustParse(targetID),
			Notes:      "implicit revoke on user.role_change",
			RequestID:  middleware.GetReqID(ctx),
		}); err != nil {
			internalError(deps.Log, w, "audit auth.session_revoked", err)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			if isSerializationFailure(err) {
				writeJSON(w, http.StatusConflict, errorResp{Error: "serialization_retry"})
				return
			}
			internalError(deps.Log, w, "commit", err)
			return
		}
		revokeAfterCommit(ctx, deps, targetID)
		writeJSON(w, http.StatusOK, map[string]any{"id": targetID, "role": newRole})
	}
}

// ResetPasswordHandler returns POST /api/users/{id}/reset-password.
// Generates a new random password, sets must_change_password=true, revokes
// all sessions for the user (D-24 implicit trigger 3 of 4). Returns the
// plaintext exactly once in the response body.
func ResetPasswordHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acting, ok := auth.GetUser(r.Context(), deps.SessionMgr)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errorResp{Error: "unauthorized"})
			return
		}
		targetID, ok := parseIDParam(w, r)
		if !ok {
			return
		}
		// Allowed for self? The plan body's RBAC test does NOT block
		// self-reset-password — the operator may reset their own forgotten
		// password from the Users surface as long as they still have a
		// session. However, the destructive-confirmation UI only exposes
		// "Reset password" on non-self rows (UI-SPEC §Surface 5 hides
		// self-actions). For server-side defense-in-depth we accept self-
		// reset here (lockout escape hatch is `shifter create-admin --reset`,
		// not this endpoint).

		plaintext, err := GenerateRandomPassword()
		if err != nil {
			internalError(deps.Log, w, "generate password", err)
			return
		}
		hash, err := auth.Hash(plaintext)
		if err != nil {
			internalError(deps.Log, w, "hash password", err)
			return
		}

		ctx := r.Context()
		tx, err := deps.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			internalError(deps.Log, w, "begin tx", err)
			return
		}
		defer func() { _ = tx.Rollback(ctx) }()

		if _, err := deps.Store.GetByIDForUpdate(ctx, tx, targetID); err != nil {
			if errors.Is(err, auth.ErrUserNotFound) {
				writeJSON(w, http.StatusNotFound, errorResp{Error: "not_found"})
				return
			}
			internalError(deps.Log, w, "load user", err)
			return
		}
		if err := deps.Store.UpdatePasswordTx(ctx, tx, targetID, hash, true /*mustChange*/); err != nil {
			internalError(deps.Log, w, "update password", err)
			return
		}
		if err := audit.WriteEntry(ctx, tx, audit.Entry{
			UserID:     uuid.MustParse(acting.ID),
			Action:     audit.ActionAuthPasswordResetByAdmin,
			EntityType: audit.EntityTypeUser,
			EntityID:   uuid.MustParse(targetID),
			Notes:      "admin-driven password rotation",
			RequestID:  middleware.GetReqID(ctx),
		}); err != nil {
			internalError(deps.Log, w, "audit auth.password_reset_by_admin", err)
			return
		}
		if err := audit.WriteEntry(ctx, tx, audit.Entry{
			UserID:     uuid.MustParse(acting.ID),
			Action:     audit.ActionAuthSessionRevoked,
			EntityType: audit.EntityTypeSession,
			EntityID:   uuid.MustParse(targetID),
			Notes:      "implicit revoke on password reset",
			RequestID:  middleware.GetReqID(ctx),
		}); err != nil {
			internalError(deps.Log, w, "audit auth.session_revoked", err)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			if isSerializationFailure(err) {
				writeJSON(w, http.StatusConflict, errorResp{Error: "serialization_retry"})
				return
			}
			internalError(deps.Log, w, "commit", err)
			return
		}
		revokeAfterCommit(ctx, deps, targetID)
		writeJSON(w, http.StatusOK, resetPasswordResponse{InitialPassword: plaintext})
	}
}

// LogoutEverywhereHandler returns POST /api/users/{id}/logout-everywhere.
// Explicit D-24 trigger 4 of 4. Server-side self-rejection per D-26 (admin
// uses /api/auth/logout to sign themselves out).
func LogoutEverywhereHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acting, ok := auth.GetUser(r.Context(), deps.SessionMgr)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errorResp{Error: "unauthorized"})
			return
		}
		targetID, ok := parseIDParam(w, r)
		if !ok {
			return
		}
		if err := RejectSelfAction(acting.ID, targetID); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, errorResp{Error: "self_action_forbidden"})
			return
		}

		ctx := r.Context()
		tx, err := deps.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			internalError(deps.Log, w, "begin tx", err)
			return
		}
		defer func() { _ = tx.Rollback(ctx) }()

		if _, err := deps.Store.GetByIDForUpdate(ctx, tx, targetID); err != nil {
			if errors.Is(err, auth.ErrUserNotFound) {
				writeJSON(w, http.StatusNotFound, errorResp{Error: "not_found"})
				return
			}
			internalError(deps.Log, w, "load user", err)
			return
		}
		if err := audit.WriteEntry(ctx, tx, audit.Entry{
			UserID:     uuid.MustParse(acting.ID),
			Action:     audit.ActionAuthSessionRevoked,
			EntityType: audit.EntityTypeSession,
			EntityID:   uuid.MustParse(targetID),
			Notes:      "explicit logout-everywhere by admin",
			RequestID:  middleware.GetReqID(ctx),
		}); err != nil {
			internalError(deps.Log, w, "audit auth.session_revoked", err)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			if isSerializationFailure(err) {
				writeJSON(w, http.StatusConflict, errorResp{Error: "serialization_retry"})
				return
			}
			internalError(deps.Log, w, "commit", err)
			return
		}
		revokeAfterCommit(ctx, deps, targetID)
		writeJSON(w, http.StatusOK, logoutResponse{UserID: targetID})
	}
}

// ─────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────

func parseIDParam(w http.ResponseWriter, r *http.Request) (string, bool) {
	raw := chi.URLParam(r, "id")
	if _, err := uuid.Parse(raw); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResp{Error: "invalid_id"})
		return "", false
	}
	return raw, true
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func internalError(log *slog.Logger, w http.ResponseWriter, op string, err error) {
	if log != nil {
		log.Error("user handler", "op", op, "err", err)
	}
	writeJSON(w, http.StatusInternalServerError, errorResp{Error: "internal"})
}

// isSerializationFailure reports whether err is a Postgres 40001
// serialization_failure, the canonical signal that a Serializable tx
// commit raced with another tx and must be retried.
func isSerializationFailure(err error) bool {
	if err == nil {
		return false
	}
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) && pgErr.SQLState() == "40001" {
		return true
	}
	return strings.Contains(err.Error(), "40001")
}

// revokeAfterCommit calls auth.IterateAndRevoke with an empty keep-token so
// ALL sessions for the user are deleted. Errors are logged but not surfaced
// to the operator — the domain mutation already committed; SCS session
// cleanup hiccups must not retroactively undo a "successful" disable /
// role-change.
func revokeAfterCommit(ctx context.Context, deps Deps, targetID string) {
	if deps.SessionMgr == nil {
		return
	}
	if err := auth.IterateAndRevoke(ctx, deps.SessionMgr, deps.Store, targetID, ""); err != nil && deps.Log != nil {
		deps.Log.Warn("user: session revoke after commit", "err", err, "user_id", targetID)
	}
}

