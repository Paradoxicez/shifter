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

// Phase 2 — Plan 02-10: Site / Metering Point / Device / Profile mutation
// + read actions wired by the Phase 2 HTTP handlers (internal/site,
// internal/meteringpoint, internal/device).
//
// Mutating actions are admin-only; read actions are open to admin AND viewer
// per AUTH-06. Every Phase 2 handler calls Can(user, <action>, nil) at entry
// as defense-in-depth; the chi router additionally wraps mutating route
// groups with RequireAction so a missing handler-level check still 403s.
const (
	// Site CRUD (D-17 + D-20) — admin only.
	ActionSiteCreate  Action = "site.create"
	ActionSiteUpdate  Action = "site.update"
	ActionSiteArchive Action = "site.archive"
	ActionSiteRestore Action = "site.restore"
	// Site read — admin AND viewer.
	ActionSiteRead Action = "site.read"

	// Metering Point CRUD (D-19 + D-20) — admin only.
	ActionMeteringPointCreate  Action = "metering_point.create"
	ActionMeteringPointUpdate  Action = "metering_point.update"
	ActionMeteringPointArchive Action = "metering_point.archive"
	ActionMeteringPointRestore Action = "metering_point.restore"
	// Metering Point read — admin AND viewer.
	ActionMeteringPointRead Action = "metering_point.read"

	// Device add (CHIRP-04 atomic) + decommission (D-15) — admin only.
	ActionDeviceAdd          Action = "device.add"
	ActionDeviceDecommission Action = "device.decommission"
	// Device read — admin AND viewer.
	ActionDeviceRead Action = "device.read"

	// Device profile editor (Plan 02-08) — admin only.
	ActionDeviceProfileCreate  Action = "device_profile.create"
	ActionDeviceProfileUpdate  Action = "device_profile.update"
	ActionDeviceProfileArchive Action = "device_profile.archive"
	// Device profile read — admin AND viewer.
	ActionDeviceProfileRead Action = "device_profile.read"

	// Meter swap (D-13 + D-14) — admin only.
	ActionMeterSwap Action = "meter.swap"

	// ActionAuditRead gates the audit browse surface (GET /api/audit/*).
	// Phase 6 Plan 06-07 D-31: admin-only. Removed from RoleViewer bundle.
	ActionAuditRead Action = "audit.read"

	// ActionAuditExport gates CSV export (GET /api/audit/export + POST /api/audit/export-async).
	// D-35: admin-only (exported data contains PII — user emails, entity states).
	ActionAuditExport Action = "audit.export"
)

// Phase 5 — Plan 05-11: settings update (data retention). Admin-only.
// T-05-11-01 mitigation: viewer PATCH → 403 enforced by Can() fail-closed default.
const (
	// ActionSettingsUpdate gates PATCH /api/settings/retention (admin only).
	ActionSettingsUpdate Action = "settings.update"
)

// Phase 6 — Plan 06-08: backup actions (OPS-02 / OPS-03 / SETT-05).
//
// ActionBackupRun gates POST /api/backup/run-now — admin-only (T-06-08-01).
// ActionBackupRead gates GET /api/backup/list, /last, /jobs/{id} — admin +
//   viewer (D-46: viewer can see backup status + history but cannot trigger).
// ActionBackupConfigure is reserved for the SETT-05 threshold edit surface
//   that Plan 06-10 wires; declared here so the action vocabulary is
//   locked before the handler is written.
const (
	ActionBackupRun       Action = "backup.run"
	ActionBackupRead      Action = "backup.read"
	ActionBackupConfigure Action = "backup.configure"
)

// Phase 6 — Plan 06-04: alert-center actions (D-11 viewer read-only).
// ActionAlertRead is the only action granted to RoleViewer; every mutating
// action is admin-only. Server-side RBAC mirrors the UI: the drawer + page
// hide Ack/Snooze for viewer, AND the handler 403s if a viewer forges the
// request.
const (
	ActionAlertRead         Action = "alert.read"
	ActionAlertAck          Action = "alert.ack"
	ActionAlertSnooze       Action = "alert.snooze"
	ActionAlertMute         Action = "alert.mute"
	ActionAlertRuleCreate   Action = "alert.rule_create"
	ActionAlertRuleUpdate   Action = "alert.rule_update"
	ActionAlertRuleDisable  Action = "alert.rule_disable"
	ActionAlertRuleEnable   Action = "alert.rule_enable"
	ActionAlertTestFire     Action = "alert.test_fire"
)

// Phase 6 — Plan 06-05: fine-grained user-management actions. The Phase 1
// umbrella ActionUserManage stays as the legacy "do anything with users"
// admin gate. The new actions split that umbrella so each REST verb is
// individually grant-checkable, the future viewer-self-edit surface
// (ActionUserReadSelf) doesn't accidentally inherit the umbrella, and audit
// rows can record the specific verb that was permitted.
//
// All eight mutating actions are admin-only. ActionUserReadSelf is granted
// to RoleViewer so the Phase 6 viewer-of-own-row UI doesn't 403 the
// /api/account/me-equivalent endpoints (today /api/account/me satisfies
// the role; ActionUserReadSelf is reserved for any future per-row read).
const (
	ActionUserList             Action = "user.list"
	ActionUserCreate           Action = "user.create"
	ActionUserUpdate           Action = "user.update"
	ActionUserDisable          Action = "user.disable"
	ActionUserEnable           Action = "user.enable"
	ActionUserChangeRole       Action = "user.change_role"
	ActionUserResetPassword    Action = "user.reset_password"
	ActionUserLogoutEverywhere Action = "user.logout_everywhere"
	ActionUserReadSelf         Action = "user.read_self"
)

// Phase 3 — Plan 03-02: gateway CRUD + bulk-import + secret reveal.
// All mutating actions admin-only; viewer denied via fail-closed default.
// ActionGatewayRead is the single exception viewers retain (mirrors
// ActionSiteRead / ActionDeviceRead in Phase 2).
//
// T-3-10 mitigation: every action below is explicitly listed in
// roleBundles[RoleAdmin]; only ActionGatewayRead is listed in
// roleBundles[RoleViewer]. TestCan_AnonymousDeniedAllPhase3 proves nil-user
// is denied for every action regardless of role.
const (
	ActionGatewayCreate       Action = "gateway.create"
	ActionGatewayUpdate       Action = "gateway.update"
	ActionGatewayArchive      Action = "gateway.archive"
	ActionGatewayRestore      Action = "gateway.restore"
	ActionGatewayRead         Action = "gateway.read"
	ActionDeviceBulkImport    Action = "device.bulk_import"
	ActionDeviceRevealSecrets Action = "device.reveal_secrets"
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

		// Phase 2 — Plan 02-10: admin can perform every Phase 2 mutation.
		ActionSiteCreate:           true,
		ActionSiteUpdate:           true,
		ActionSiteArchive:          true,
		ActionSiteRestore:          true,
		ActionSiteRead:             true,
		ActionMeteringPointCreate:  true,
		ActionMeteringPointUpdate:  true,
		ActionMeteringPointArchive: true,
		ActionMeteringPointRestore: true,
		ActionMeteringPointRead:    true,
		ActionDeviceAdd:            true,
		ActionDeviceDecommission:   true,
		ActionDeviceRead:           true,
		ActionDeviceProfileCreate:  true,
		ActionDeviceProfileUpdate:  true,
		ActionDeviceProfileArchive: true,
		ActionDeviceProfileRead:    true,
		ActionMeterSwap:   true,
		ActionAuditRead:   true,
		ActionAuditExport: true,

		// Phase 3 — Plan 03-02: gateway CRUD + bulk-import + reveal secrets.
		// Admin can perform every Phase 3 mutating action and read gateways.
		ActionGatewayCreate:       true,
		ActionGatewayUpdate:       true,
		ActionGatewayArchive:      true,
		ActionGatewayRestore:      true,
		ActionGatewayRead:         true,
		ActionDeviceBulkImport:    true,
		ActionDeviceRevealSecrets: true,

		// Phase 5 — Plan 05-11: settings update (data retention). Admin only.
		// T-05-11-01: viewers cannot PATCH retention; Can() returns false for
		// RoleViewer (ActionSettingsUpdate intentionally absent from viewer bundle).
		ActionSettingsUpdate: true,

		// Phase 6 — Plan 06-05: fine-grained user management. Every mutating
		// action is admin-only. ActionUserReadSelf is the lone read-self
		// surface, granted to both roles below.
		ActionUserList:             true,
		ActionUserCreate:           true,
		ActionUserUpdate:           true,
		ActionUserDisable:          true,
		ActionUserEnable:           true,
		ActionUserChangeRole:       true,
		ActionUserResetPassword:    true,
		ActionUserLogoutEverywhere: true,
		ActionUserReadSelf:         true,

		// Phase 6 — Plan 06-04: alert-center. Admin has every action; viewer
		// has only ActionAlertRead below (D-11 read-only).
		ActionAlertRead:        true,
		ActionAlertAck:         true,
		ActionAlertSnooze:      true,
		ActionAlertMute:        true,
		ActionAlertRuleCreate:  true,
		ActionAlertRuleUpdate:  true,
		ActionAlertRuleDisable: true,
		ActionAlertRuleEnable:  true,
		ActionAlertTestFire:    true,

		// Phase 6 — Plan 06-08: backup surface. Admin can run, read, and
		// configure backups. ActionBackupRun is admin-only (T-06-08-01).
		// ActionBackupRead is also granted to RoleViewer below (D-46).
		ActionBackupRun:       true,
		ActionBackupRead:      true,
		ActionBackupConfigure: true,
	},
	RoleViewer: {
		// Viewers can change their own password.
		ActionAccountSelfEdit: true,
		// Viewers can run Test Connection from the read-only Settings page
		// (it's a probe, not a state change). The mutating action
		// ActionConnectionEdit stays admin-only.
		ActionConnectionTest: true,

		// Phase 2 — Plan 02-10: viewer can READ every Phase 2 entity but
		// CANNOT mutate. Every mutation action is intentionally absent —
		// fail-closed default in Can() means viewer mutation attempts 403.
		//
		// ActionAuditRead is intentionally absent from RoleViewer (D-31 from
		// Plan 06-07): the audit log browse is admin-only because it surfaces
		// PII (user emails), action history, and before/after diffs for every
		// operator mutation. Viewers are redirected to / at the route level.
		ActionSiteRead:          true,
		ActionMeteringPointRead: true,
		ActionDeviceRead:        true,
		ActionDeviceProfileRead: true,

		// Phase 3 — Plan 03-02: viewers can ONLY read gateways. Mutating
		// actions (create/update/archive/restore), bulk_import, and
		// reveal_secrets are all intentionally absent — fail-closed.
		ActionGatewayRead: true,

		// Phase 6 — Plan 06-05: viewer can read their own user row (the
		// /api/account/me path is already granted via ActionAccountSelfEdit;
		// this action is reserved for future per-row read surfaces). NONE of
		// the user-mgmt mutating actions are granted to viewer — every
		// mutating endpoint 403s for viewer per D-26 + the umbrella RBAC test.
		ActionUserReadSelf: true,

		// Phase 6 — Plan 06-04: viewer can READ alerts (D-11). Every
		// mutating alert action (ack, snooze, mute, rule create/update/
		// disable/enable, test_fire) is intentionally absent — fail-closed.
		ActionAlertRead: true,

		// Phase 6 — Plan 06-08: viewer can READ backup status + history (D-46).
		// ActionBackupRun and ActionBackupConfigure are intentionally absent —
		// viewer cannot trigger a backup or change backup thresholds.
		ActionBackupRead: true,
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
