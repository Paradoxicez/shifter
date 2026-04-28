---
phase: 01-foundation
plan: 10
subsystem: auth
tags: [go, authz, rbac, middleware, owasp, asvs-v3, asvs-v4, auth-06]

requires:
  - phase: 01-foundation
    plan: 08
    provides: auth.User struct + GetUser context helper (RequireAction reads user from SCS-bound context)
provides:
  - internal/auth.Action string-typed enum (9 declared actions; 4 Phase 1, 5 forward-declared Phase 2+)
  - internal/auth.Role string-typed enum (RoleAdmin, RoleViewer)
  - internal/auth.roleBundles map[Role]map[Action]bool (package-private; single source of truth)
  - internal/auth.Can(user, action, resource) — fail-closed permission predicate
  - internal/auth.RequireAction(sm, action) — chi-friendly middleware factory; 401 / 403 split
affects:
  - 01-11-account-ui (account password-change endpoint will wrap with RequireAction(sm, ActionAccountSelfEdit))
  - 01-17-test-connection (settings page mutating endpoints wrap RequireAction(sm, ActionConnectionEdit); test probe wraps RequireAction(sm, ActionConnectionTest))
  - 01-18-router-health (/health/detailed wraps RequireAction(sm, ActionHealthDetailed); /health stays public)
  - Phase 2 (device CRUD): RequireAction(sm, ActionDeviceCreate / Update / Delete)
  - Phase 6 USER-04: extends roleBundles map only — call sites already point at the right Action constants

tech-stack:
  added: []
  patterns:
    - "Single source of truth: roleBundles map[Role]map[Action]bool. Adding a role in Phase 6 is a map-extension, never a call-site refactor (PITFALLS §14)."
    - "Fail-closed default: Can() returns false for nil user, zero User, unknown role, and unmapped action. Adding a role to the user_role enum without registering it in roleBundles cannot accidentally elevate privileges (T-10-04)."
    - "Forward-declared actions: 5 Phase 2+ Action constants (UserManage / Device.* / AuditView) ship now so the API surface is locked. Future plans reference the existing constant; no constant churn at the integration boundary."
    - "401-vs-403 split: middleware returns 401 when no session is present (request never reaches the handler — T-10-02) and 403 when authenticated-but-forbidden (T-10-01). 403 leaking endpoint existence is explicitly accepted (T-10-03; admin endpoint paths are documented, no obscurity claim)."
    - "Reserved resource parameter: Can(user, action, resource any) keeps the third argument now even though Phase 1 ignores it. Locks the signature so per-row authz (Phase 6 / Phase 7 — 'user can manage own dashboard') extends the body without re-signing every call site."
    - "Middleware reads user via GetUser (panic-safe per Plan 08): if no LoadAndSave middleware ran, GetUser recovers and returns ok=false, which RequireAction surfaces as 401. No panics in production paths."

key-files:
  created:
    - internal/auth/authz.go (Action / Role enums + roleBundles + Can + RequireAction; 187 LOC including doc comments)
  modified:
    - internal/auth/authz_test.go (replaces 3 t.Skip stubs from Plan 02 with 5 real tests)
    - internal/http/rbac_test.go (replaces 2 t.Skip stubs from Plan 02 with 3 real integration tests)

key-decisions:
  - "roleBundles is package-private (var roleBundles map[Role]...). Reason: external consumers MUST go through Can() — exposing the map would let a downstream plan iterate it and accidentally introduce its own permission check that diverges from the canonical predicate. Plan 11/17/18 import only the Action constants and Can / RequireAction functions; the map stays an implementation detail."
  - "ActionConnectionTest is on the viewer's allow-list. Rationale: the read-only Settings page (Plan 17) shows a 'Test Connection' button that the viewer can click — the result is a probe, not a state change. Restricting Test Connection to admins would force viewers to wait for an admin to verify operability after a network blip, which adds friction with zero security benefit (the probe leaks only 'reachable' or 'unreachable' which is also visible from the dashboard's freshness indicators). Locked into roleBundles so future plans cannot accidentally tighten it."
  - "ActionHealthDetailed is admin-only even though /health/probe (basic) is unauthenticated. /health/detailed (Plan 18) exposes DB connection counts, MQTT broker status, and the migration version — operationally useful but enumerable as a fingerprint for an attacker. Splitting at the action boundary lets the operator's monitoring system probe /health without a session, while internal observability dashboards probe /health/detailed with admin credentials."
  - "Action string values use dot-namespacing (`connection.edit`, `account.self.edit`). Two reasons: (1) audit logs (Phase 6) can group by namespace via prefix match; (2) future per-namespace policy (e.g. 'audit all connection.* actions') is a one-line addition without re-shaping the enum."
  - "RequireAction returns `func(http.Handler) http.Handler` (the standard Go middleware shape) rather than `http.Handler` directly. Reason: chi.Router.Use / Method composition wants the wrapper-of-wrappers shape; building Plan 17's `r.Method(\"POST\", \"/api/settings/chirpstack\", auth.RequireAction(sm, ActionConnectionEdit)(handler))` reads naturally and matches the existing sm.LoadAndSave shape from Plan 09."
  - "writeAuthzError is a private helper inside authz.go rather than re-using handlers.go writeJSON. Reason: keeps authz.go self-contained — if a future refactor splits internal/auth into sub-packages (e.g. internal/auth/authz, internal/auth/login), authz.go does not need to import its sibling. The cost is one duplicated 4-line helper; the benefit is independent evolvability."
  - "Action constants are declared in two const blocks (Phase 1 set + forward-declared Phase 2+ set) rather than one big block. Visual signal in the source: a grep for `Phase 1 actions` lands on the constants the current phase actually wires, while `Forward-declared` is the API-stability buffer. Future plans appending Phase 2 actions (Plan 18 already declares HealthDetailed; Plans for device CRUD will land DeviceCreate/Update/Delete) move the constant from the second block into a Phase-2 block as those actions ship."

requirements-completed:
  - AUTH-06

duration: 3min
completed: 2026-04-28
---

# Phase 01 Plan 10: Authorization Summary

**Authorization API (`Can(user, action, resource)`) + `RequireAction(sm, action)` middleware backed by a `roleBundles map[Role]map[Action]bool` — implements AUTH-06 server-side enforcement with a 401/403 split, fail-closed defaults on unknown roles, and a forward-compatible map structure that lets Phase 6 USER-04 add roles without touching any call site.**

## Performance

- **Duration:** ~3 min
- **Started:** 2026-04-28T01:15:53Z
- **Completed:** 2026-04-28
- **Tasks:** 1 / 1 (TDD: RED → GREEN, no REFACTOR needed)
- **Commits:** 2 (1 RED + 1 GREEN)
- **Files created:** 1 (`internal/auth/authz.go`)
- **Files modified:** 2 (`internal/auth/authz_test.go`, `internal/http/rbac_test.go`)
- **Tests added:** 8 (5 unit Can() tests + 3 integration RBAC tests; replaces 5 t.Skip stubs from Plan 02)

## Action Enum Surface

```go
package auth

type Action string

// Phase 1 actions — wired by Plans 09 / 17 / 18.
const (
    ActionConnectionEdit  Action = "connection.edit"   // Plan 17 settings dialog (mutate)
    ActionConnectionTest  Action = "connection.test"   // Plan 17 Test Connection probe
    ActionAccountSelfEdit Action = "account.self.edit" // Plan 09 / 11 change own password
    ActionHealthDetailed  Action = "health.detailed"   // Plan 18 /health/detailed
)

// Forward-declared Phase 2+ actions — declared NOW per PITFALLS §14.
const (
    ActionUserManage   Action = "user.manage"   // Phase 6 USER-04
    ActionDeviceCreate Action = "device.create" // Phase 2 / 3 device CRUD
    ActionDeviceUpdate Action = "device.update"
    ActionDeviceDelete Action = "device.delete"
    ActionAuditView    Action = "audit.view"    // Phase 6 audit log read
)
```

**9 actions total.** The first 4 are wired in Phase 1 (Plans 09, 17, 18); the last 5 are placeholders so the call-site shape is locked when those phases ship.

## Role Bundle Table

| Role          | Phase 1 actions allowed                                  | Phase 2+ actions allowed |
| ------------- | -------------------------------------------------------- | ------------------------ |
| `RoleAdmin`   | connection.edit / connection.test / account.self.edit / health.detailed | every forward-declared action |
| `RoleViewer`  | account.self.edit, connection.test                       | (none)                   |
| unknown role  | — (fail-closed; T-10-04)                                 | —                        |

**Why viewer keeps `connection.test`:** the read-only Settings page surfaces a Test Connection button. The probe leaks only "reachable" / "unreachable" — the same information the dashboard's freshness indicators expose. Restricting it would add friction with zero security benefit.

**Why viewer is denied `health.detailed`:** /health/detailed (Plan 18) exposes DB connection counts, MQTT broker status, migration version. Useful operationally, but a fingerprint for an attacker. Basic /health is unauthenticated; the detailed variant is admin-only.

## Public API

```go
package auth

// Action / Role string-typed enums (see above).

// Can returns true iff the user is authorized for the action.
//
// Fail-closed defaults:
//   - nil user           → false
//   - User{} (empty ID)  → false
//   - unknown role       → false
//   - action not in role → false
//
// `resource` is reserved for Phase 6+ per-row authz; ignored in v1.
func Can(user *User, action Action, resource any) bool

// RequireAction returns a chi-friendly middleware that gates the wrapped
// handler on Can(currentUser, action, nil).
//
// Status codes:
//   - 401 + {"error": "unauthorized"} when no session present (T-10-02)
//   - 403 + {"error": "forbidden"}    when authenticated but Can returns false (T-10-01)
//
// Caller MUST be downstream of sm.LoadAndSave (Plan 18 wires once at the
// chi router root).
func RequireAction(sm *scs.SessionManager, action Action) func(http.Handler) http.Handler
```

## Middleware Usage Examples

### Plan 11 — account password-change

```go
r.Method("POST", "/api/account/password",
    auth.RequireAction(sm, auth.ActionAccountSelfEdit)(
        auth.ChangePasswordHandler(accountDeps),
    ),
)
// Both admin and viewer reach this handler; the inner handler still
// re-verifies current_password (defense in depth).
```

### Plan 17 — settings page

```go
// Mutating: admin only.
r.Method("POST", "/api/settings/chirpstack",
    auth.RequireAction(sm, auth.ActionConnectionEdit)(handler),
)
// Probe: admin or viewer.
r.Method("POST", "/api/settings/chirpstack/test",
    auth.RequireAction(sm, auth.ActionConnectionTest)(handler),
)
```

### Plan 18 — health detailed

```go
// Public probe — no middleware.
r.Method("GET", "/health", basicHealthHandler)

// Detailed probe — admin only.
r.Method("GET", "/health/detailed",
    auth.RequireAction(sm, auth.ActionHealthDetailed)(detailedHealthHandler),
)
```

## Phase 6 USER-04 Forward-Compat Recipe

When Phase 6 lands a third role (say, `RoleAuditor` who can view audit logs but cannot mutate anything), the change is **localized to roleBundles**:

```go
// internal/auth/authz.go — single-line addition to the const block:
const RoleAuditor Role = "auditor"

// internal/auth/authz.go — bundle map addition:
var roleBundles = map[Role]map[Action]bool{
    RoleAdmin:  { /* unchanged */ },
    RoleViewer: { /* unchanged */ },
    RoleAuditor: {
        ActionAccountSelfEdit: true,
        ActionAuditView:       true,
    },
}
```

**Zero call-site changes.** Plan 17/18/Phase-2 device routes that already wrap `RequireAction(sm, ActionDeviceCreate)` immediately respect the new role's policy. The migration to add `auditor` to the `user_role` Postgres enum is the only other touchpoint; the application layer is purely additive.

This is exactly the lock-in benefit PITFALLS §14 calls out: "design `Can(user, action, resource)` API now so the two-role coarseness in v1 doesn't lock us into refactor-everywhere later."

## Test-Coverage Matrix

| Test                                | Covers                              | Asserts                                                |
| ----------------------------------- | ----------------------------------- | ------------------------------------------------------ |
| `TestCan_AdminAllowsAll`            | admin role allow-list               | Can() == true for all 9 declared actions               |
| `TestCan_ViewerSelfEditAllowed`     | viewer narrow allow-list            | Can() == true for AccountSelfEdit + ConnectionTest     |
| `TestCan_ViewerConnectionEditDenied`| viewer denial set                   | Can() == false for ConnectionEdit + UserManage + HealthDetailed + Device.* + AuditView |
| `TestCan_NilUser`                   | anonymous request                   | Can() == false for nil user AND zero User              |
| `TestCan_UnknownRole`               | T-10-04 fail-closed                 | Can(&User{Role:"ghost"}, ...) == false                 |
| `TestRBAC_AdminAllowed`             | end-to-end admin path               | seed admin + POST /protected → 200                     |
| `TestRBAC_ViewerForbidden`          | T-10-01 viewer-on-admin-route       | seed viewer + POST /protected → 403                    |
| `TestRBAC_NoSession`                | T-10-02 anonymous-on-protected-route| no cookie + POST /protected → 401                      |

8 tests pass under `-race -count=1`. Replaces 5 t.Skip stubs from Plan 02 (3 `TestCan_*` in `internal/auth/authz_test.go`, 2 `TestRBAC_*` in `internal/http/rbac_test.go`); adds 3 new test cases that go beyond the original stubs (`TestCan_UnknownRole`, `TestCan_NilUser` zero-User branch, `TestRBAC_NoSession`).

## Threat Surface Notes

All 4 entries in the plan's `<threat_model>` are mitigated by code shipped in this plan:

| Threat  | Category                | Mitigation                                                                                |
| ------- | ----------------------- | ----------------------------------------------------------------------------------------- |
| T-10-01 | Elevation of Privilege  | `RequireAction` middleware returns 403 for authenticated-but-forbidden; `TestRBAC_ViewerForbidden` regression-tests the path |
| T-10-02 | Spoofing                | Middleware returns 401 BEFORE handler runs when no session present; `TestRBAC_NoSession` covers       |
| T-10-03 | Information Disclosure  | **Accepted** — Phase 1 returns 403 (not 404) on forbidden routes. Admin endpoint paths are documented, no obscurity claim |
| T-10-04 | Tampering               | `Can()` returns false for unknown roles (fail-closed); `TestCan_UnknownRole` regression-tests       |

**No new threat surface beyond the plan's register.** RequireAction is the single chokepoint per the trust-boundary table; downstream plans (11, 17, 18) MUST wrap every state-changing or privileged route in `RequireAction` — a missing wrapper is the only way to introduce a vulnerability in this layer.

## Decisions Made

- **roleBundles is package-private.** External consumers go through `Can()` only — exposing the map would let a downstream plan iterate it and accidentally introduce its own permission check that diverges from the canonical predicate.
- **Action / Role types are string-aliased.** `type Action string` (not `type Action int`) lets audit logs (Phase 6) record the action verbatim and lets future per-namespace policy match by string prefix without an additional registry.
- **`resource any` parameter is reserved but ignored in v1.** Adding it now means Phase 6/7 per-row authz extends the function body without re-signing every call site.
- **`writeAuthzError` is local to authz.go**, not a re-use of handlers.go writeJSON. Independent evolvability if internal/auth ever splits into sub-packages; cost is 4 duplicated lines.
- **Action constants in two blocks** — Phase 1 set + Forward-declared Phase 2+ set. Visual signal in the source distinguishing "wired now" from "API-stability buffer". Future plans shipping Phase 2 actions move the constant out of the forward-declared block as those actions go live.
- **Viewer keeps `connection.test`.** The probe is read-only; locking it to admins would add friction without security benefit. Documented in roleBundles inline comments so future plans cannot tighten by accident.
- **`health.detailed` is admin-only.** Basic /health stays unauthenticated for monitoring systems; detailed variant exposes operational fingerprint and goes behind RequireAction.

## Deviations from Plan

None. The plan-verbatim Action enum, role bundles, Can() body, and RequireAction wiring all matched the test acceptance criteria on the first GREEN pass; no Rule 1/2/3 fixes were needed.

The only intentional addition beyond the plan's verbatim test set is `TestCan_UnknownRole` — the plan's behavior block called out fail-closed default for unknown roles (T-10-04 mitigation requirement), so the test landed alongside the plan's other Can() cases. This is in the plan's own acceptance criteria ("`Can(&User{ID: \"x\", Role: \"ghost\"}, ...)` returns false (unknown role denied)"); it was simply broken out into its own named test for clearer regression isolation.

## Issues Encountered

None. Build was green from the GREEN-phase commit; vet was clean; no testcontainer flakes on this run (all 3 RBAC tests passed first try; no port-mapping race surfaced).

## Known Stubs

None. authz.go is fully implemented:

- `Can()` body covers all four fail-closed branches (nil, empty ID, unknown role, action not in bundle).
- `RequireAction` body returns the documented 401 / 403 codes; no TODO/FIXME placeholders.
- All 9 Action constants are real values backed by the roleBundles map.
- The roleBundles map covers both RoleAdmin and RoleViewer with explicit boolean values; no implicit defaults.

## Threat Flags

None — no new security-relevant surface beyond what's in the plan's `<threat_model>`. The 4 register entries (T-10-01..04) are exactly the surface this plan introduces.

## User Setup Required

None. Plan 10 is server-side authorization plumbing; the operator never sees authz code directly. The role they pick when their first admin row is created (via `shifter create-admin` from Plan 09) determines what they can do in the running app — which is invisible until Plans 11/17/18 surface admin-only UI elements.

## Next Phase Readiness

- ✅ AUTH-06: server-side role enforcement is live behind `RequireAction(sm, action)`. UI side ships in Plan 11 (admin-only "Manage Users" tab; viewer sees disabled / hidden controls).
- ✅ PITFALLS §14: forward-compatible role bundle map; Phase 6 USER-04 adds roles without call-site refactors.
- ✅ 401/403 split correct (no session vs forbidden) — `TestRBAC_NoSession` and `TestRBAC_ViewerForbidden` lock the contract.
- ✅ Anonymous requests blocked at middleware before handler runs.

**Plan 11 (account UI):** wraps `POST /api/account/password` with `auth.RequireAction(sm, auth.ActionAccountSelfEdit)`. The handler from Plan 09 is unchanged — middleware composes outside.

**Plan 14 (install middleware):** install routes are gated by `Store.AdminExists` (boolean), not by RequireAction — the wizard is reachable BEFORE any admin exists. Once the wizard finishes (Plan 15), Plan 14's middleware redirects /install/* away; admin-only routes from that point on use RequireAction.

**Plan 17 (test-connection):** wraps the mutating settings endpoint with `RequireAction(sm, ActionConnectionEdit)` and the probe endpoint with `RequireAction(sm, ActionConnectionTest)`. Viewer can probe but not mutate.

**Plan 18 (router-health):** mounts `/health` (public) and `/health/detailed` (wrapped in `RequireAction(sm, ActionHealthDetailed)` — admin only). Also wires the chi root so `sm.LoadAndSave` is the outermost middleware (RequireAction depends on it).

**Phase 2 device CRUD:** `Device{Create,Update,Delete}` constants already exist; routes use them directly, no Phase-1 churn.

**Phase 6 USER-04:** new role added by extending roleBundles only. Existing `RequireAction(sm, ActionUserManage)` call site (which Phase 6 wires for the admin-user-management UI) immediately respects the new role.

## Self-Check: PASSED

Files verified to exist:
- FOUND: `internal/auth/authz.go`
- FOUND: `internal/auth/authz_test.go`
- FOUND: `internal/http/rbac_test.go`

Commits verified to exist:
- FOUND: `f0d4c8e` (Task 1 RED — failing authz tests)
- FOUND: `4a92108` (Task 1 GREEN — Action enum + Can + RequireAction)

Behavior verified:
- `go build ./...` exits 0
- `go vet ./...` exits 0
- `go test ./internal/auth -run TestCan -race -count=1` passes 5 tests
- `go test ./internal/http -run TestRBAC_ -race -count=1` passes 3 tests
- `go test ./internal/auth -race -count=1` passes 56 tests (51 prior + 5 new TestCan_*)
- `grep -E '^(type Action|type Role|var roleBundles|func Can|func RequireAction)' internal/auth/authz.go` returns 5 hits (all required exported symbols present)
- `grep -E 'Action(ConnectionEdit|ConnectionTest|AccountSelfEdit|HealthDetailed|UserManage|DeviceCreate|DeviceUpdate|DeviceDelete|AuditView)\s+Action' internal/auth/authz.go` returns 9 hits (all 9 declared actions present)

---
*Phase: 01-foundation*
*Plan: 10-authz*
*Completed: 2026-04-28*
