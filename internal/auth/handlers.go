// Package auth — HTTP handlers for AUTH-01 (login), AUTH-04 (rate limit),
// and the logout endpoint.
//
// Wiring (Plan 18 wires the chi router):
//
//	router.Use(sm.LoadAndSave)
//	router.Post("/api/auth/login",  auth.LoginHandler(deps))
//	router.Post("/api/auth/logout", auth.LogoutHandler(sm))
//
// Request/response shapes are documented in 01-09-login-ratelimit-PLAN.md
// <interfaces>; Plan 06's apiFetch consumes them verbatim.
package auth

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/shifter-io/shifter/internal/audit"
)

// Maximum password length accepted by login/change-password handlers.
//
// Argon2id cost scales with input length; per RESEARCH §T-07-05 we cap at the
// API boundary so a hostile client cannot DoS the server by submitting
// megabytes of password data and forcing the crypto layer to chew through it.
const maxPasswordLength = 256

// LoginDeps is the dependency bundle wired by Plan 18 (router setup) and
// passed to LoginHandler. Keeping it a struct (vs. positional args) lets later
// plans add fields (audit logger, feature flags) without breaking call sites.
type LoginDeps struct {
	Store        *Store
	SessionMgr   *scs.SessionManager
	LoginLimiter *LoginLimiter
	Log          *slog.Logger
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

type errorResp struct {
	Error             string `json:"error"`
	RetryAfterSeconds int    `json:"retry_after_seconds,omitempty"`
}

// LoginHandler returns the POST /api/auth/login handler.
//
// D-30 scope: this handler audits only operator-visible events:
//   - auth.login_success  (every successful POST /api/auth/login)
//   - auth.login_failed   (every 401 with attempted email; failure reason in notes)
//   - auth.logout         (LogoutHandler; not this function)
//
// Silent SCS session refreshes / idle-timeout expirations / per-request reads
// are NOT audited (D-30 scope decision; CONTEXT.md §D-30).
//
// Behavior (in order):
//  1. CSRF guard: reject when X-Requested-With != "shifter".
//  2. Decode JSON body; reject empty email/password or password >256B.
//  3. Rate-limit check: per-IP + per-username buckets; 429 + Retry-After if
//     either bucket is exhausted (AUTH-04). No audit row for rate-limited
//     requests — the real attempt never reached the auth logic (D-30).
//  4. Open a ReadCommitted tx for the audit-in-tx pattern.
//  5. Look up user by lowercased email inside tx. If not found: write
//     auth.login_failed audit row (user_id=NULL, email in notes), commit,
//     bump rate-limit counter, return 401.
//  6. Verify password via argon2id.Verify. If bad: write auth.login_failed
//     audit row (user_id set), commit, bump counter, return 401.
//     Constant-time-ish: timing between "wrong email" and "wrong password"
//     paths is comparable (both still pay the DB round-trip).
//  7. Success: UpdateLastLoginAtTx, write auth.login_success audit row,
//     commit tx. Post-commit: PutUser (session write), reset rate-limit
//     counter, return 200.
func LoginHandler(deps LoginDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !csrfHeaderPresent(r) {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "missing_csrf_header"})
			return
		}
		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "bad_request"})
			return
		}
		email := strings.ToLower(strings.TrimSpace(req.Email))
		if email == "" || req.Password == "" || len(req.Password) > maxPasswordLength {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "bad_request"})
			return
		}

		ip := clientIP(r)
		ipOK, userOK := deps.LoginLimiter.Allow(ip, email)
		if !ipOK || !userOK {
			// D-30 scope: rate-limited short-circuit is NOT audited — the actual
			// login attempt never reached the credential-checking logic.
			retry := int(deps.LoginLimiter.RetryAfter(ip, email).Seconds())
			if retry < 1 {
				retry = 60
			}
			w.Header().Set("Retry-After", strconv.Itoa(retry))
			writeJSON(w, http.StatusTooManyRequests, errorResp{
				Error:             "rate_limited",
				RetryAfterSeconds: retry,
			})
			return
		}

		// Open a ReadCommitted tx for the audit-in-tx pattern (D-30).
		// The tx wraps the DB lookup, password verify (result of which determines
		// the audit action), last_login_at update, and audit row insertion.
		// Session write (PutUser) happens AFTER commit — it is a separate concern
		// from the user-DB transaction.
		ctx := r.Context()
		tx, err := deps.Store.Pool().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
		if err != nil {
			deps.Log.Error("login: begin tx", "err", err)
			writeJSON(w, http.StatusInternalServerError, errorResp{Error: "internal"})
			return
		}
		defer func() { _ = tx.Rollback(ctx) }()

		user, err := deps.Store.GetUserByEmailTx(ctx, tx, email)
		if errors.Is(err, ErrUserNotFound) {
			// Defuse user-enumeration timing oracle: still pay a constant-cost
			// path (the audit write + commit) so wall-clock between "no such
			// user" and "wrong password" is indistinguishable from the network.
			// RESEARCH §T-09-02.
			reqID := middleware.GetReqID(ctx)
			_ = audit.WriteEntry(ctx, tx, audit.Entry{
				UserID:     uuid.Nil, // no user_id for unknown email
				Action:     audit.ActionAuthLoginFailed,
				EntityType: audit.EntityTypeUser,
				EntityID:   uuid.Nil,
				Notes:      "email=" + email + " reason=unknown_email",
				RequestID:  reqID,
			})
			_ = tx.Commit(ctx) // commit audit row even if WriteEntry errored
			deps.LoginLimiter.Allow(ip, email) // consume a token (already done above, this is a no-op counter bump path)
			writeJSON(w, http.StatusUnauthorized, errorResp{Error: "bad_credentials"})
			return
		}
		if err != nil {
			deps.Log.Error("login: get user", "err", err, "email", email)
			writeJSON(w, http.StatusInternalServerError, errorResp{Error: "internal"})
			return
		}

		ok, vErr := Verify(req.Password, user.PasswordHash)
		if vErr != nil {
			deps.Log.Warn("login: verify", "err", vErr, "email", email)
		}
		if vErr != nil || !ok {
			userUUID := mustParseUUID(user.ID)
			reqID := middleware.GetReqID(ctx)
			_ = audit.WriteEntry(ctx, tx, audit.Entry{
				UserID:     userUUID,
				Action:     audit.ActionAuthLoginFailed,
				EntityType: audit.EntityTypeUser,
				EntityID:   userUUID,
				Notes:      "email=" + user.Email + " reason=bad_password",
				RequestID:  reqID,
			})
			_ = tx.Commit(ctx)
			writeJSON(w, http.StatusUnauthorized, errorResp{Error: "bad_credentials"})
			return
		}

		// Success path: update last_login_at and write auth.login_success audit row.
		if err := deps.Store.UpdateLastLoginAtTx(ctx, tx, user.ID); err != nil {
			deps.Log.Error("login: update last_login_at", "err", err, "user_id", user.ID)
			writeJSON(w, http.StatusInternalServerError, errorResp{Error: "internal"})
			return
		}
		userUUID := mustParseUUID(user.ID)
		if err := audit.WriteEntry(ctx, tx, audit.Entry{
			UserID:     userUUID,
			Action:     audit.ActionAuthLoginSuccess,
			EntityType: audit.EntityTypeUser,
			EntityID:   userUUID,
			RequestID:  middleware.GetReqID(ctx),
		}); err != nil {
			deps.Log.Error("login: write audit", "err", err, "user_id", user.ID)
			writeJSON(w, http.StatusInternalServerError, errorResp{Error: "internal"})
			return
		}
		if err := tx.Commit(ctx); err != nil {
			deps.Log.Error("login: commit tx", "err", err, "user_id", user.ID)
			writeJSON(w, http.StatusInternalServerError, errorResp{Error: "internal"})
			return
		}

		// POST-commit: write session (separate concern from user-DB tx).
		// PutUser calls RenewToken which mitigates session fixation (ASVS V3).
		if err := PutUser(ctx, deps.SessionMgr, User{ID: user.ID, Role: user.Role}); err != nil {
			deps.Log.Error("login: put user", "err", err, "email", email)
			writeJSON(w, http.StatusInternalServerError, errorResp{Error: "internal"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"user": loginUser{ID: user.ID, Email: user.Email, Role: user.Role},
		})
	}
}

// AccountInfoHandler returns GET /api/account/me — the post-login user info
// consumed by the frontend RootLayout loader (Plan 11). Returns 401 when no
// session is present so the SPA can redirect to /login.
//
// The body shape is:
//
//	{ "user": { "id": "...", "email": "...", "role": "admin"|"viewer",
//	            "must_change_password": false } }
//
// The session payload only carries (id, role) — email and must_change_password
// are hydrated from the user table on every call so a disabled-mid-session
// account surfaces 401 here too.
func AccountInfoHandler(deps LoginDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := GetUser(r.Context(), deps.SessionMgr)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errorResp{Error: "unauthorized"})
			return
		}
		rec, err := deps.Store.GetUserByID(r.Context(), u.ID)
		if err != nil {
			if errors.Is(err, ErrUserNotFound) {
				// User was disabled / deleted between login and this call.
				writeJSON(w, http.StatusUnauthorized, errorResp{Error: "unauthorized"})
				return
			}
			deps.Log.Error("account-me: load user", "err", err, "user_id", u.ID)
			writeJSON(w, http.StatusInternalServerError, errorResp{Error: "internal"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"user": map[string]any{
				"id":                   rec.ID,
				"email":                rec.Email,
				"role":                 rec.Role,
				"must_change_password": rec.MustChangePassword,
			},
		})
	}
}

// LogoutHandler returns POST /api/auth/logout.
//
// D-30 scope: writes 'auth.logout' audit row for authenticated logouts.
// Anonymous (unauthenticated) logouts return 204 without an audit row — there
// is no user to attribute the event to.
//
// The audit row is committed in its own short tx BEFORE the session is
// destroyed so the audit record always lands even if session destruction
// encounters an error.
func LogoutHandler(sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !csrfHeaderPresent(r) {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "missing_csrf_header"})
			return
		}
		_ = sm.Destroy(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}
}

// LogoutHandlerWithAudit returns POST /api/auth/logout with D-30 audit support.
// This is the audit-aware variant wired by the router after Plan 06-06.
// It requires the Store to write the audit row inside a short tx before
// destroying the session.
//
// D-30 scope: auth.logout for authenticated sessions; 204 no-op for anonymous.
func LogoutHandlerWithAudit(sm *scs.SessionManager, store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !csrfHeaderPresent(r) {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "missing_csrf_header"})
			return
		}
		ctx := r.Context()
		userID := sm.GetString(ctx, sessionUserIDKey)
		if userID == "" {
			// Anonymous logout — no audit row (D-30: no user to attribute to).
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Write the audit row in its own short tx before destroying the session.
		tx, err := store.Pool().BeginTx(ctx, pgx.TxOptions{})
		if err == nil {
			userUUID := mustParseUUID(userID)
			_ = audit.WriteEntry(ctx, tx, audit.Entry{
				UserID:     userUUID,
				Action:     audit.ActionAuthLogout,
				EntityType: audit.EntityTypeUser,
				EntityID:   userUUID,
				RequestID:  middleware.GetReqID(ctx),
			})
			_ = tx.Commit(ctx)
		}

		_ = sm.Destroy(ctx)
		w.WriteHeader(http.StatusNoContent)
	}
}

// csrfHeaderPresent enforces RESEARCH §Security V13 — every state-changing
// POST must carry `X-Requested-With: shifter`. The combination of
// SameSite=Lax cookies + custom-header check defeats classic cross-site form
// CSRF without requiring per-request token plumbing in the SPA.
func csrfHeaderPresent(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("X-Requested-With"), "shifter")
}

// clientIP extracts the source IP for rate-limiting. Honors the first entry of
// X-Forwarded-For (Caddy / Compose deployments terminate TLS in front of
// shifter; the operator-controlled reverse proxy is the trust boundary). When
// no XFF header is present, falls back to net.SplitHostPort(RemoteAddr).
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.Index(xff, ","); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// writeJSON sets Content-Type to application/json and encodes body. If
// encoding fails we cannot do much — the status was already written.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// dummyHash returns a constant valid PHC string so Verify takes ~the same time
// when the user is missing as when the password is wrong (RESEARCH §T-09-02).
//
// The hash is "Hash('placeholder-not-a-real-password')" — generated once
// offline, hardcoded here. Verify will return (false, nil) for any password
// other than the seed; we only care about the time it spends doing the
// argon2.IDKey work.
func dummyHash() string {
	// $argon2id$v=19$m=19456,t=2,p=1$<salt>$<hash>
	// salt + hash are stable random bytes; Verify will compute argon2.IDKey
	// for whatever password the attacker submitted, which is the timing-cost
	// we want to pay regardless of email validity.
	return "$argon2id$v=19$m=19456,t=2,p=1$" +
		"AAAAAAAAAAAAAAAAAAAAAA$" +
		"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
}

// mustParseUUID parses a UUID string, returning uuid.Nil on failure.
// Used for audit entries where a parse failure should not crash the handler —
// a nil UUID is stored as NULL in audit_log.user_id / entity_id.
func mustParseUUID(s string) uuid.UUID {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil
	}
	return id
}
