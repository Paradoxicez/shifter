// Package auth — authorization API (AUTH-06).
//
// This file implements RESEARCH §Pattern 16 + PITFALLS §14: a single
// `Can(user, action, resource)` permission predicate backed by a
// `roleBundles map[Role]map[Action]bool`, plus a chi-friendly
// `RequireAction` middleware factory that returns 401 (no session) or 403
// (authenticated but unauthorized).
//
// Forward-compatible by design (PITFALLS §14):
//
//   - Adding a new role in Phase 6 USER-04 means extending `roleBundles`
//     with a new key — call sites never change.
//   - Adding a new action means appending to the const block + populating
//     the bundle map — same shape, no new abstractions.
//   - The `resource` parameter is unused in Phase 1 but reserved so
//     per-row authz (Phase 6 / Phase 7) can be added without re-shaping
//     the API surface.
//
// Wiring (Plan 17 / Plan 18 use these patterns):
//
//	router.Method("POST", "/api/settings/chirpstack",
//	    auth.RequireAction(sm, auth.ActionConnectionEdit)(handler))
//	router.Method("GET", "/health/detailed",
//	    auth.RequireAction(sm, auth.ActionHealthDetailed)(handler))
package auth

import (
	"encoding/json"
	"net/http"

	"github.com/alexedwards/scs/v2"
)

// Action is a string-typed permission identifier. The dot-namespaced form
// (`connection.edit`, `account.self.edit`) keeps actions readable in audit
// logs and lets future phases group by namespace if needed.
type Action string

// Phase 1 actions — wired by Plans 09 / 17 / 18.
const (
	// ActionConnectionEdit gates POST/PUT against the ChirpStack connection
	// settings (Plan 17 wraps /api/settings/chirpstack with this).
	ActionConnectionEdit Action = "connection.edit"

	// ActionConnectionTest allows running the Test Connection probe. Viewers
	// keep this — they can read the result on the read-only Settings page.
	ActionConnectionTest Action = "connection.test"

	// ActionAccountSelfEdit lets a user change their own password / display
	// name. Both roles allow this (a viewer must be able to rotate their own
	// password without admin intervention).
	ActionAccountSelfEdit Action = "account.self.edit"

	// ActionHealthDetailed gates GET /health/detailed (Plan 18). The basic
	// /health endpoint is unauthenticated; /health/detailed exposes DB and
	// MQTT internals and must be admin-only.
	ActionHealthDetailed Action = "health.detailed"
)

// Forward-declared Phase 2+ actions. Declared NOW per PITFALLS §14 so the
// API surface is locked: adding a new role in Phase 6 USER-04 only edits
// `roleBundles`; call sites never refactor.
const (
	// ActionUserManage — Phase 6 USER-04: admin creates / disables users.
	ActionUserManage Action = "user.manage"
	// ActionDeviceCreate / Update / Delete — Phase 2 / 3 device CRUD.
	ActionDeviceCreate Action = "device.create"
	ActionDeviceUpdate Action = "device.update"
	ActionDeviceDelete Action = "device.delete"
	// ActionAuditView — Phase 6 audit log read.
	ActionAuditView Action = "audit.view"
)

// Role is the typed role identifier mirroring the Postgres user_role enum.
type Role string

// Role identifiers. These string values must match the user_role enum in
// migration 0002 — User.Role (Plan 08) carries the raw enum string from the
// session payload, and we cast to Role only at the bundle lookup boundary.
const (
	RoleAdmin  Role = "admin"
	RoleViewer Role = "viewer"
)

// roleBundles is the single source of truth for role→action permissions.
//
// PITFALLS §14: when Phase 6 USER-04 adds new roles (e.g. "auditor",
// "operator"), extend this map only. Every call site of Can() and
// RequireAction() inherits the change automatically.
//
// Fail-closed by default: actions absent from a bundle return false from
// Can(). An unknown role (not in this map) returns false for every action
// (T-10-04 — adding a role to the user table without registering it in the
// bundle map cannot accidentally elevate privileges).
var roleBundles = map[Role]map[Action]bool{
	RoleAdmin: {
		ActionConnectionEdit:  true,
		ActionConnectionTest:  true,
		ActionAccountSelfEdit: true,
		ActionHealthDetailed:  true,
		ActionUserManage:      true,
		ActionDeviceCreate:    true,
		ActionDeviceUpdate:    true,
		ActionDeviceDelete:    true,
		ActionAuditView:       true,
	},
	RoleViewer: {
		// Viewers can change their own password.
		ActionAccountSelfEdit: true,
		// Viewers can run Test Connection from the read-only Settings page
		// (it's a probe, not a state change). The mutating action
		// ActionConnectionEdit stays admin-only.
		ActionConnectionTest: true,
	},
}

// Can returns true iff the given user is authorized for the action.
//
// Contract:
//
//   - nil user            → false (anonymous request).
//   - User{} (empty ID)   → false (zero-value struct treated as anonymous).
//   - unknown role        → false (T-10-04 fail-closed default).
//   - action not in bundle→ false.
//
// The `resource` parameter is unused in Phase 1 but reserved for
// per-row / per-resource authorization in Phase 6+ (e.g. "user can manage
// their own dashboard" — `Can(u, ActionDashboardEdit, dashboard)`). Do not
// remove the parameter even though the body ignores it: keeping it in the
// signature now means the call sites that ship in Phase 1 do not need to
// be re-touched when per-row authz arrives.
func Can(user *User, action Action, resource any) bool {
	_ = resource
	if user == nil || user.ID == "" {
		return false
	}
	bundle, ok := roleBundles[Role(user.Role)]
	if !ok {
		return false
	}
	return bundle[action]
}

// RequireAction returns a middleware that gates the wrapped handler on
// `Can(currentUser, action, nil)`.
//
// Status codes (per RESEARCH §Pattern 16 + ASVS V3 / V4):
//
//   - No session at all  → 401 + `{"error": "unauthorized"}`
//     (T-10-02 mitigation — request never reaches the handler).
//   - Authenticated but
//     Can() returns false → 403 + `{"error": "forbidden"}`
//     (T-10-01 mitigation — viewer probing admin endpoint cannot
//     enumerate handler behavior).
//   - Authorized          → next.ServeHTTP.
//
// The middleware reads the current user via auth.GetUser, which expects the
// SCS context key to be present — i.e. the request MUST flow through
// sm.LoadAndSave first. Plan 18 wires LoadAndSave on the chi router root
// exactly once.
func RequireAction(sm *scs.SessionManager, action Action) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			u, ok := GetUser(r.Context(), sm)
			if !ok {
				writeAuthzError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			if !Can(&u, action, nil) {
				writeAuthzError(w, http.StatusForbidden, "forbidden")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// writeAuthzError writes a JSON `{"error": "<code>"}` body with the given
// status. Mirrors handlers.go writeJSON but kept here so the authz package
// stays self-contained — the helper is defensive against future refactors
// that might split auth into sub-packages.
func writeAuthzError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}
