---
phase: 01-foundation
plan: 10
type: execute
wave: 7
depends_on: [08]
files_modified:
  - internal/auth/authz.go
  - internal/auth/authz_test.go
  - internal/http/rbac_test.go
autonomous: true
requirements:
  - AUTH-06
must_haves:
  truths:
    - "Can(admin, AnyAction, nil) returns true for every defined action (admin has full control)"
    - "Can(viewer, ActionAccountSelfEdit, nil) returns true; viewer can change own password"
    - "Can(viewer, ActionConnectionEdit, nil) returns false (read-only)"
    - "Can(nil, AnyAction, nil) returns false (no user = no access)"
    - "RequireAction middleware returns 403 to unauthorized users, 401 if no session at all"
    - "Forward-compatible role bundles map (PITFALLS §14) — Phase 6 USER-04 can add new roles without refactoring call sites"
  artifacts:
    - path: "internal/auth/authz.go"
      provides: "Can(user, action, resource) + RequireAction middleware (RESEARCH §Pattern 16)"
      contains: "func Can"
    - path: "internal/auth/authz_test.go"
      provides: "Can() matrix tests + middleware behavior tests"
      contains: "TestCan_AdminAllowsAll"
  key_links:
    - from: "internal/auth/authz.go RequireAction"
      to: "internal/auth/session.go GetUser"
      via: "middleware reads user from SCS-bound context"
      pattern: "GetUser"
---

<objective>
Implement the canonical authorization API per RESEARCH §Pattern 16 + PITFALLS §14: a `Can(user, action, resource)` function backed by a `roleBundles map[Role]map[Action]bool`, and a `RequireAction(action)` middleware factory that returns 401 (no session) or 403 (insufficient role). Forward-compatible: Phase 6 USER-04 adds roles by extending `roleBundles` only — call sites never change.

Purpose: AUTH-06 (admin/viewer roles enforced server-side AND in UI). PITFALLS §14: design the API now so the two-role coarseness in v1 doesn't lock us into refactor-everywhere later.

Output: `go test ./internal/auth -run TestCan` and `go test ./internal/http -run TestRBAC_` pass.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/01-foundation/01-RESEARCH.md
@.planning/phases/01-foundation/01-CONTEXT.md
@.planning/research/PITFALLS.md
@01-08-session-manager-PLAN.md

<interfaces>
RESEARCH §Pattern 16 (lines 1138-1190) — verbatim Action / Role / `Can` / `RequireAction`.

Action enum (Phase 1 minimum, extensible):
```go
const (
    ActionConnectionEdit  Action = "connection.edit"      // Plan 17 settings dialog
    ActionConnectionTest  Action = "connection.test"       // Plan 17 Test Connection
    ActionAccountSelfEdit Action = "account.self.edit"     // Plan 09 change own password
    ActionHealthDetailed  Action = "health.detailed"        // Plan 18 /health/detailed
    ActionUserManage      Action = "user.manage"           // Phase 6 — declared now for forward compat
    ActionDeviceCreate    Action = "device.create"         // Phase 2/3 — declared now
    ActionDeviceUpdate    Action = "device.update"         // Phase 2/3
    ActionDeviceDelete    Action = "device.delete"         // Phase 2/3
)
```

Role bundles:
```go
RoleAdmin: every action true
RoleViewer: { ActionAccountSelfEdit: true, ActionConnectionTest: true /* viewers can run Test Connection on the read-only Settings page */ }
```

Middleware contract:
- No session at all → 401 + `{"error": "unauthorized"}`
- Authenticated but `Can(user, action, nil)` returns false → 403 + `{"error": "forbidden"}`
- Authorized → next.ServeHTTP
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Authz API + RequireAction middleware + tests</name>
  <files>internal/auth/authz.go, internal/auth/authz_test.go, internal/http/rbac_test.go</files>
  <read_first>
    - .planning/phases/01-foundation/01-RESEARCH.md §"Pattern 16: Authorization" (lines 1138-1190)
    - .planning/research/PITFALLS.md §14 (two-role coarseness — design `Can(user, action, resource)` API now)
    - 01-08-session-manager-PLAN.md (auth.User struct, auth.GetUser)
  </read_first>
  <behavior>
    - TestCan_AdminAllowsAll: Can({Role: "admin"}, every action, nil) → true.
    - TestCan_ViewerSelfEditAllowed: Can({Role: "viewer"}, ActionAccountSelfEdit, nil) → true.
    - TestCan_ViewerConnectionEditDenied: Can({Role: "viewer"}, ActionConnectionEdit, nil) → false.
    - TestCan_NilUser: Can(nil, anything, nil) → false (using zero-value User; helper detects).
    - TestRBAC_AdminAllowed: HTTP test — admin POSTs to admin-only endpoint → 200.
    - TestRBAC_ViewerForbidden: HTTP test — viewer POSTs to admin-only endpoint → 403.
    - TestRBAC_NoSession: HTTP test — anonymous request → 401.
  </behavior>
  <action>
1. Create `internal/auth/authz.go`:
   ```go
   package auth

   import (
       "encoding/json"
       "net/http"

       "github.com/alexedwards/scs/v2"
   )

   type Action string

   const (
       // Phase 1 actions — used by Plan 09 (account), Plan 17 (connection), Plan 18 (health detailed).
       ActionConnectionEdit  Action = "connection.edit"
       ActionConnectionTest  Action = "connection.test"
       ActionAccountSelfEdit Action = "account.self.edit"
       ActionHealthDetailed  Action = "health.detailed"

       // Forward-declared Phase 2+ actions — declared NOW to lock the API surface
       // (PITFALLS §14: design `Can(user, action, resource)` so we don't have to
       // refactor call sites when new roles arrive in Phase 6 USER-04).
       ActionUserManage   Action = "user.manage"
       ActionDeviceCreate Action = "device.create"
       ActionDeviceUpdate Action = "device.update"
       ActionDeviceDelete Action = "device.delete"
       ActionAuditView    Action = "audit.view"
   )

   type Role string

   const (
       RoleAdmin  Role = "admin"
       RoleViewer Role = "viewer"
   )

   // roleBundles is the single source of truth for role permissions.
   // Adding a role in Phase 6: extend this map. Call sites never change.
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
           ActionAccountSelfEdit: true,
           ActionConnectionTest:  true, // viewer can run Test Connection from Settings (read-only read of result)
       },
   }

   // Can returns true if the given user is authorized for the action.
   // The `resource` parameter is unused in Phase 1 but reserved for Phase 6
   // (per-row authz e.g. "user can manage own dashboard"). Do not remove.
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

   // RequireAction is a chi-friendly middleware factory.
   // 401 when no session is present; 403 when authenticated but unauthorized.
   func RequireAction(sm *scs.SessionManager, action Action) func(http.Handler) http.Handler {
       return func(next http.Handler) http.Handler {
           return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
               u, ok := GetUser(r.Context(), sm)
               if !ok {
                   w.Header().Set("Content-Type", "application/json")
                   w.WriteHeader(http.StatusUnauthorized)
                   _ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
                   return
               }
               if !Can(&u, action, nil) {
                   w.Header().Set("Content-Type", "application/json")
                   w.WriteHeader(http.StatusForbidden)
                   _ = json.NewEncoder(w).Encode(map[string]string{"error": "forbidden"})
                   return
               }
               next.ServeHTTP(w, r)
           })
       }
   }
   ```

2. Replace `internal/auth/authz_test.go`:
   ```go
   package auth

   import (
       "testing"

       "github.com/stretchr/testify/require"
   )

   func TestCan_AdminAllowsAll(t *testing.T) {
       admin := &User{ID: "u1", Role: "admin"}
       for _, a := range []Action{
           ActionConnectionEdit, ActionConnectionTest, ActionAccountSelfEdit, ActionHealthDetailed,
           ActionUserManage, ActionDeviceCreate, ActionDeviceUpdate, ActionDeviceDelete, ActionAuditView,
       } {
           require.True(t, Can(admin, a, nil), "admin must be able to %s", a)
       }
   }

   func TestCan_ViewerSelfEditAllowed(t *testing.T) {
       viewer := &User{ID: "u2", Role: "viewer"}
       require.True(t, Can(viewer, ActionAccountSelfEdit, nil))
       require.True(t, Can(viewer, ActionConnectionTest, nil))
   }

   func TestCan_ViewerConnectionEditDenied(t *testing.T) {
       viewer := &User{ID: "u2", Role: "viewer"}
       require.False(t, Can(viewer, ActionConnectionEdit, nil))
       require.False(t, Can(viewer, ActionUserManage, nil))
       require.False(t, Can(viewer, ActionHealthDetailed, nil))
   }

   func TestCan_NilUser(t *testing.T) {
       require.False(t, Can(nil, ActionAccountSelfEdit, nil))
       require.False(t, Can(&User{}, ActionAccountSelfEdit, nil)) // empty ID treated as no user
   }

   func TestCan_UnknownRole(t *testing.T) {
       u := &User{ID: "u3", Role: "ghost"}
       require.False(t, Can(u, ActionAccountSelfEdit, nil))
   }
   ```

3. Replace `internal/http/rbac_test.go`:
   ```go
   package http

   import (
       "context"
       "log/slog"
       "net/http"
       "net/http/cookiejar"
       "net/http/httptest"
       "os"
       "testing"
       "time"

       "github.com/shifter-io/shifter/internal/auth"
       "github.com/shifter-io/shifter/internal/db"
       "github.com/shifter-io/shifter/internal/testsupport"
       "github.com/stretchr/testify/require"
   )

   func setupRBAC(t *testing.T, role string) (*httptest.Server, *http.Client) {
       t.Helper()
       pool := testsupport.StartPostgres(t)
       require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))

       sm := auth.NewSessionManager(pool, true, 8*time.Hour, 24*time.Hour)
       mux := http.NewServeMux()
       mux.Handle("POST /seed", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
           _ = auth.PutUser(r.Context(), sm, auth.User{ID: "u1", Role: role})
       }))
       protected := auth.RequireAction(sm, auth.ActionConnectionEdit)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
           w.WriteHeader(200)
       }))
       mux.Handle("POST /protected", protected)

       srv := httptest.NewServer(sm.LoadAndSave(mux))
       t.Cleanup(srv.Close)
       j, _ := cookiejar.New(nil)
       return srv, &http.Client{Jar: j}
   }

   func TestRBAC_AdminAllowed(t *testing.T) {
       srv, cli := setupRBAC(t, "admin")
       res, _ := cli.Post(srv.URL+"/seed", "", nil); res.Body.Close()
       res, _ = cli.Post(srv.URL+"/protected", "", nil); defer res.Body.Close()
       require.Equal(t, 200, res.StatusCode)
   }

   func TestRBAC_ViewerForbidden(t *testing.T) {
       srv, cli := setupRBAC(t, "viewer")
       res, _ := cli.Post(srv.URL+"/seed", "", nil); res.Body.Close()
       res, _ = cli.Post(srv.URL+"/protected", "", nil); defer res.Body.Close()
       require.Equal(t, 403, res.StatusCode, "AUTH-06: viewer must not POST to admin-only endpoint")
   }

   func TestRBAC_NoSession(t *testing.T) {
       srv, _ := setupRBAC(t, "admin")
       cli := &http.Client{}
       res, _ := cli.Post(srv.URL+"/protected", "", nil); defer res.Body.Close()
       require.Equal(t, 401, res.StatusCode, "no session must be 401")
   }
   ```
  </action>
  <verify>
    <automated>go test ./internal/auth -run TestCan -race -count=1 -v && go test ./internal/http -run TestRBAC_ -race -count=1 -v</automated>
  </verify>
  <acceptance_criteria>
    - File `internal/auth/authz.go` exports `type Action string`, `type Role string`, `var roleBundles map[Role]map[Action]bool`, `func Can(user *User, action Action, resource any) bool`, `func RequireAction(sm *scs.SessionManager, action Action) func(http.Handler) http.Handler`
    - All 9 forward-declared actions exist (`ActionConnectionEdit`, `ActionConnectionTest`, `ActionAccountSelfEdit`, `ActionHealthDetailed`, `ActionUserManage`, `ActionDeviceCreate`, `ActionDeviceUpdate`, `ActionDeviceDelete`, `ActionAuditView`)
    - `Can(nil, ...)` returns false (verified by test)
    - `Can(&User{ID: "x", Role: "ghost"}, ...)` returns false (unknown role denied)
    - Command `go test ./internal/auth -run TestCan -race` exits 0 with 5 tests passing
    - Command `go test ./internal/http -run TestRBAC_AdminAllowed -race` exits 0 (per VALIDATION.md)
    - Command `go test ./internal/http -run TestRBAC_ViewerForbidden -race` exits 0 (per VALIDATION.md)
    - 401 status when no session, 403 when authenticated but forbidden (verified by tests)
  </acceptance_criteria>
  <done>
    Authorization API + middleware ready. Plan 17 wraps `/api/settings/chirpstack` with `RequireAction(sm, ActionConnectionEdit)`; Plan 18 wraps `/health/detailed` with `RequireAction(sm, ActionHealthDetailed)`.
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| middleware → handler | RequireAction is the single chokepoint; anywhere it's missing is a vulnerability |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-10-01 | Elevation of Privilege | viewer accesses admin endpoint | mitigate | `RequireAction` middleware on every admin route; tested via `TestRBAC_ViewerForbidden`. ASVS V4. |
| T-10-02 | Spoofing | request without session reaches handler | mitigate | 401 returned before handler runs; tested via `TestRBAC_NoSession`. ASVS V3. |
| T-10-03 | Information Disclosure | 403 vs 404 leaks endpoint existence | accept | Phase 1 returns 403 — admin endpoint paths are documented; no obscurity claim. ASVS V4. |
| T-10-04 | Tampering | future role added without updating roleBundles | mitigate | `Can` returns false for unknown roles (tested) — fail-closed. ASVS V4. |
</threat_model>

<verification>
- `Can(user, action, resource)` API matches RESEARCH §Pattern 16
- 9 actions declared (6 Phase 1, 3 Phase 2+ forward-declared per PITFALLS §14)
- Admin role allows all; Viewer role allows only self-edit + connection.test
- Middleware: 401 no-session, 403 unauthorized
- 5 unit tests + 3 integration tests pass
</verification>

<success_criteria>
- AUTH-06 enforced server-side
- PITFALLS §14 satisfied: forward-compatible role bundle map; Phase 6 USER-04 only extends `roleBundles`
- 401/403 split correct (no session vs forbidden)
- Anonymous requests blocked at middleware before handler runs
</success_criteria>

<output>
After completion, create `.planning/phases/01-foundation/01-10-SUMMARY.md` documenting:
- Action enum surface
- Role bundle table
- Middleware usage example
- How Phase 6 USER-04 will extend roles
</output>
