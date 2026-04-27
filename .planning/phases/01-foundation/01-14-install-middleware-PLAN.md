---
phase: 01-foundation
plan: 14
type: execute
wave: 8
depends_on: [03, 09]
files_modified:
  - internal/install/middleware.go
  - internal/install/middleware_test.go
  - internal/install/state.go
  - internal/install/state_test.go
  - internal/install/regions.go
autonomous: true
requirements:
  - INST-01
must_haves:
  truths:
    - "FirstRunGate redirects HTML traffic to /install when no admin user exists (D-08)"
    - "FirstRunGate returns 409 install_required for /api/* traffic when no admin"
    - "FirstRunGate WHITELISTS /install, /api/install/*, /health, /assets (static SPA), /login"
    - "Once an admin exists, FirstRunGate is a pass-through and caches the answer (PITFALL #10 prevention)"
    - "InstallState.GetOrCreate returns the singleton row (CHECK id=1 from migration 0004)"
    - "InstallState.Update<Step>(payload) writes JSONB and bumps current_step"
    - "Regions catalog has AS923-1, AS923-2 (Thailand), AS923-3, AS923-4, EU868, US915, AU915, IN865 (RESEARCH §Pattern 13)"
  artifacts:
    - path: "internal/install/middleware.go"
      provides: "FirstRunGate http.Handler factory with cache (D-08, PITFALL #10)"
      contains: "FirstRunGate"
    - path: "internal/install/state.go"
      provides: "Store wrapping install_state queries (GetOrCreate, UpdateStepN, FinishSetup placeholder, Reset)"
      contains: "type Store"
    - path: "internal/install/regions.go"
      provides: "Hardcoded AS923+EU+US+AU+IN region catalog (RESEARCH §Pattern 13)"
      contains: "AS923_2"
  key_links:
    - from: "internal/install/middleware.go"
      to: "auth.Store.AdminExists"
      via: "single SQL EXISTS check, cached after first true"
      pattern: "AdminExists"
---

<objective>
Implement the install_state Store (CRUD over the singleton install_state row + region catalog) and the FirstRunGate middleware that detects "no admin user" and either redirects (HTML) or returns 409 install_required (API). The gate is wired into the chi router by Plan 18.

Purpose: INST-01 (first-run wizard guides operator through admin creation), D-08 (first-run by absence of admin user), D-10/D-11 (reentrant 5-step wizard), PITFALL #10 (cache the gate after positive result).

Output: `go test ./internal/install -run 'TestFirstRun_|TestPostFinish_|TestStep|TestInstallState_'` passes against testcontainer Postgres.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/01-foundation/01-RESEARCH.md
@.planning/phases/01-foundation/01-CONTEXT.md
@01-03-database-layer-PLAN.md
@01-09-login-ratelimit-PLAN.md

<interfaces>
RESEARCH §Pattern 2 (lines 380-408) — verbatim FirstRunGate skeleton.
RESEARCH §Pattern 3 (lines 412-466) — verbatim install_state schema (already in migration 0004) + commit pattern (Plan 15 implements the atomic finish).
RESEARCH §Pattern 13 (lines 1013-1036) — verbatim region catalog.
PITFALL #10 — cache via sync.Once / atomic.Bool after positive AdminExists result.

Whitelist paths (HTML and API):
- `/install` and `/api/install/*` — wizard endpoints
- `/health` — public probe (D-18)
- `/assets/*`, `/index.html`, `/`, `*.svg`, `*.css`, `*.js`, `*.ico` — SPA static
- `/login` — login screen (after install completes; before, redirect away to /install)

Public API:
```go
package install

func FirstRunGate(pool *pgxpool.Pool, log *slog.Logger) func(http.Handler) http.Handler

type State struct {
    StartedAt    time.Time
    CompletedAt  *time.Time
    CurrentStep  int
    Step1Admin       json.RawMessage
    Step2ChirpStack  json.RawMessage
    Step3Region      json.RawMessage
    Step4Identity    json.RawMessage
}

type Store struct{ pool *pgxpool.Pool }
func NewStore(pool *pgxpool.Pool) *Store
func (s *Store) GetOrCreate(ctx context.Context) (*State, error)
func (s *Store) UpdateStep1(ctx context.Context, payload []byte) error
func (s *Store) UpdateStep2(ctx context.Context, payload []byte) error
func (s *Store) UpdateStep3(ctx context.Context, payload []byte) error
func (s *Store) UpdateStep4(ctx context.Context, payload []byte) error
func (s *Store) Delete(ctx context.Context) error  // called after FinishSetup commit (Plan 15)

type Region struct { Name, Display, CommonName, Group, DefaultForCountry, Note string }
func Regions() []Region
```
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Install Store + Regions catalog + tests</name>
  <files>internal/install/state.go, internal/install/state_test.go, internal/install/regions.go</files>
  <read_first>
    - .planning/phases/01-foundation/01-RESEARCH.md §"Pattern 3: Reentrant Install Wizard" (lines 410-466)
    - .planning/phases/01-foundation/01-RESEARCH.md §"Pattern 13: AS923 region picker" (lines 1008-1036) — verbatim catalog
    - 01-03-database-layer-PLAN.md (install_state schema, sqlc queries already defined in 0004 + queries/install_state.sql)
  </read_first>
  <behavior>
    - TestInstallState_GetOrCreate_NewInstall: empty DB → GetOrCreate returns CurrentStep=1, no completedAt.
    - TestInstallState_GetOrCreate_Reentrant: call twice; second call returns same row (no duplicate inserts).
    - TestStep1_PersistsAdmin: UpdateStep1 with JSONB payload; row's step1_admin matches; current_step bumped to >=2.
    - TestStep2_CapturesCS: UpdateStep2; step2_chirpstack matches; current_step >=3.
    - TestStep3_PersistsRegion: UpdateStep3 with `{"name":"as923_2","common_name":"AS923_2"}`; current_step >=4.
    - TestStep4_PersistsIdentity: UpdateStep4; current_step >=5.
    - TestRegions_HasThailand: Regions() includes name="as923_2", default_for_country="TH".
  </behavior>
  <action>
1. Create `internal/install/state.go`:
   ```go
   package install

   import (
       "context"
       "encoding/json"
       "fmt"
       "time"

       "github.com/jackc/pgx/v5/pgxpool"
   )

   type State struct {
       StartedAt       time.Time
       CompletedAt     *time.Time
       CurrentStep     int
       Step1Admin      json.RawMessage
       Step2ChirpStack json.RawMessage
       Step3Region     json.RawMessage
       Step4Identity   json.RawMessage
   }

   type Store struct{ pool *pgxpool.Pool }

   func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

   // GetOrCreate returns the singleton install_state row, creating it on first call.
   // Re-entrant: second invocation returns the same row (CHECK id=1 prevents dupes).
   func (s *Store) GetOrCreate(ctx context.Context) (*State, error) {
       _, err := s.pool.Exec(ctx,
           `INSERT INTO install_state (id) VALUES (1) ON CONFLICT (id) DO NOTHING`)
       if err != nil {
           return nil, fmt.Errorf("ensure install_state row: %w", err)
       }
       row := s.pool.QueryRow(ctx,
           `SELECT started_at, completed_at, current_step,
                   step1_admin, step2_chirpstack, step3_region, step4_identity
              FROM install_state WHERE id = 1`)
       var st State
       var s1, s2, s3, s4 *string
       if err := row.Scan(&st.StartedAt, &st.CompletedAt, &st.CurrentStep, &s1, &s2, &s3, &s4); err != nil {
           return nil, err
       }
       st.Step1Admin = jsonOrNull(s1)
       st.Step2ChirpStack = jsonOrNull(s2)
       st.Step3Region = jsonOrNull(s3)
       st.Step4Identity = jsonOrNull(s4)
       return &st, nil
   }

   func jsonOrNull(s *string) json.RawMessage {
       if s == nil { return nil }
       return json.RawMessage(*s)
   }

   func (s *Store) UpdateStep1(ctx context.Context, payload []byte) error {
       return s.updateStep(ctx, "step1_admin", 2, payload)
   }
   func (s *Store) UpdateStep2(ctx context.Context, payload []byte) error {
       return s.updateStep(ctx, "step2_chirpstack", 3, payload)
   }
   func (s *Store) UpdateStep3(ctx context.Context, payload []byte) error {
       return s.updateStep(ctx, "step3_region", 4, payload)
   }
   func (s *Store) UpdateStep4(ctx context.Context, payload []byte) error {
       return s.updateStep(ctx, "step4_identity", 5, payload)
   }

   func (s *Store) updateStep(ctx context.Context, column string, nextStep int, payload []byte) error {
       // The column name is whitelisted by call sites — never user input.
       q := fmt.Sprintf(
           `UPDATE install_state
              SET %s = $1::jsonb,
                  current_step = GREATEST(current_step, $2)
            WHERE id = 1`, column)
       _, err := s.pool.Exec(ctx, q, payload, nextStep)
       return err
   }

   // Delete drops the singleton row. Called after FinishSetup commits (Plan 15).
   func (s *Store) Delete(ctx context.Context) error {
       _, err := s.pool.Exec(ctx, `DELETE FROM install_state WHERE id = 1`)
       return err
   }

   // ResetForTest is a TEST-ONLY helper to clear state between tests.
   func (s *Store) ResetForTest(ctx context.Context) error {
       _, err := s.pool.Exec(ctx, `DELETE FROM install_state`)
       return err
   }
   ```

2. Create `internal/install/regions.go`:
   ```go
   package install

   type Region struct {
       Name              string `json:"name"`
       Display           string `json:"display"`
       CommonName        string `json:"common_name"`
       Group             string `json:"group"`
       DefaultForCountry string `json:"default_for_country,omitempty"`
       Note              string `json:"note,omitempty"`
   }

   // Regions returns the Phase 1 hardcoded LoRaWAN region catalog.
   // Per CONTEXT.md <deferred>: a regulator-versioned catalog file is Phase 7.
   func Regions() []Region {
       return []Region{
           {Name: "as923",   Display: "AS923-1",                  CommonName: "AS923",   Group: "asia"},
           {Name: "as923_2", Display: "AS923-2 (Thailand)",       CommonName: "AS923_2", Group: "asia",
               DefaultForCountry: "TH", Note: "Required by Thai regulator NBTC."},
           {Name: "as923_3", Display: "AS923-3",                  CommonName: "AS923_3", Group: "asia"},
           {Name: "as923_4", Display: "AS923-4",                  CommonName: "AS923_4", Group: "asia"},
           {Name: "eu868",   Display: "EU868 (Europe)",           CommonName: "EU868",   Group: "europe"},
           {Name: "us915_0", Display: "US915 sub-band 1 (ch 0-7)", CommonName: "US915",   Group: "americas"},
           {Name: "au915_0", Display: "AU915 sub-band 1",          CommonName: "AU915",   Group: "oceania"},
           {Name: "in865",   Display: "IN865 (India)",             CommonName: "IN865",   Group: "india"},
       }
   }

   func RegionByName(name string) (Region, bool) {
       for _, r := range Regions() {
           if r.Name == name { return r, true }
       }
       return Region{}, false
   }
   ```

3. Replace `internal/install/state_test.go`:
   ```go
   package install

   import (
       "context"
       "log/slog"
       "os"
       "testing"

       "github.com/shifter-io/shifter/internal/db"
       "github.com/shifter-io/shifter/internal/testsupport"
       "github.com/stretchr/testify/require"
   )

   func setupStore(t *testing.T) *Store {
       t.Helper()
       pool := testsupport.StartPostgres(t)
       require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))
       return NewStore(pool)
   }

   func TestInstallState_GetOrCreate_NewInstall(t *testing.T) {
       s := setupStore(t)
       st, err := s.GetOrCreate(context.Background())
       require.NoError(t, err)
       require.Equal(t, 1, st.CurrentStep)
       require.Nil(t, st.CompletedAt)
   }

   func TestInstallState_GetOrCreate_Reentrant(t *testing.T) {
       s := setupStore(t)
       _, err := s.GetOrCreate(context.Background()); require.NoError(t, err)
       _, err = s.GetOrCreate(context.Background()); require.NoError(t, err)
   }

   func TestStep1_PersistsAdmin(t *testing.T) {
       s := setupStore(t)
       _, _ = s.GetOrCreate(context.Background())
       payload := []byte(`{"email":"a@x","name":"Alice","password_hash":"$argon2id$..."}`)
       require.NoError(t, s.UpdateStep1(context.Background(), payload))
       st, _ := s.GetOrCreate(context.Background())
       require.GreaterOrEqual(t, st.CurrentStep, 2)
       require.Contains(t, string(st.Step1Admin), "alice")
   }

   func TestStep2_CapturesCS(t *testing.T) {
       s := setupStore(t)
       _, _ = s.GetOrCreate(context.Background())
       payload := []byte(`{"mode":"bundled","grpc_url":"chirpstack:8080","mqtt_url":"tcp://mosquitto:1883"}`)
       require.NoError(t, s.UpdateStep2(context.Background(), payload))
       st, _ := s.GetOrCreate(context.Background())
       require.GreaterOrEqual(t, st.CurrentStep, 3)
   }

   func TestStep3_PersistsRegion(t *testing.T) {
       s := setupStore(t)
       _, _ = s.GetOrCreate(context.Background())
       payload := []byte(`{"name":"as923_2","common_name":"AS923_2"}`)
       require.NoError(t, s.UpdateStep3(context.Background(), payload))
       st, _ := s.GetOrCreate(context.Background())
       require.GreaterOrEqual(t, st.CurrentStep, 4)
       require.Contains(t, string(st.Step3Region), "AS923_2")
   }

   func TestStep4_PersistsIdentity(t *testing.T) {
       s := setupStore(t)
       _, _ = s.GetOrCreate(context.Background())
       payload := []byte(`{"display_name":"Acme","timezone":"Asia/Bangkok","units":"metric"}`)
       require.NoError(t, s.UpdateStep4(context.Background(), payload))
       st, _ := s.GetOrCreate(context.Background())
       require.GreaterOrEqual(t, st.CurrentStep, 5)
   }

   func TestRegions_HasThailand(t *testing.T) {
       found := false
       for _, r := range Regions() {
           if r.Name == "as923_2" && r.DefaultForCountry == "TH" {
               found = true
           }
       }
       require.True(t, found, "INST-04: Regions must include Thailand AS923-2 default")
   }
   ```
  </action>
  <verify>
    <automated>go test ./internal/install -run 'TestInstallState_|TestStep|TestRegions_' -race -count=1 -v</automated>
  </verify>
  <acceptance_criteria>
    - File `internal/install/state.go` exports `type State struct`, `type Store struct`, `func NewStore(*pgxpool.Pool) *Store`, methods `GetOrCreate`, `UpdateStep1`, `UpdateStep2`, `UpdateStep3`, `UpdateStep4`, `Delete`
    - `GetOrCreate` uses `INSERT ... ON CONFLICT (id) DO NOTHING` to ensure singleton row (CHECK id=1 from migration 0004)
    - Each `UpdateStepN` bumps `current_step` to GREATEST(current_step, N+1) — verified by test asserting `>= N+1`
    - File `internal/install/regions.go` exports `func Regions() []Region` and `func RegionByName(name string) (Region, bool)`
    - `Regions()` returns at least 8 regions including `name="as923_2"` with `DefaultForCountry="TH"`
    - All 6 tests pass: `TestInstallState_GetOrCreate_NewInstall`, `TestInstallState_GetOrCreate_Reentrant`, `TestStep1_PersistsAdmin`, `TestStep2_CapturesCS`, `TestStep3_PersistsRegion`, `TestStep4_PersistsIdentity`, `TestRegions_HasThailand`
    - Command `go test ./internal/install -run 'TestStep4_PersistsIdentity' -race` exits 0 (per VALIDATION.md)
    - Command `go test ./internal/install -run 'TestStep3_PersistsRegion' -race` exits 0 (per VALIDATION.md)
  </acceptance_criteria>
  <done>
    Install state store + regions catalog ready. Plan 15 (wizard handlers) consumes the Store; Plan 16 (frontend) consumes the regions JSON.
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: FirstRunGate middleware with cache + tests</name>
  <files>internal/install/middleware.go, internal/install/middleware_test.go</files>
  <read_first>
    - .planning/phases/01-foundation/01-RESEARCH.md §"Pattern 2: First-Run Detection in Middleware" (lines 380-408)
    - .planning/phases/01-foundation/01-RESEARCH.md §"Pitfall 10: First-run middleware adds DB latency" (lines 1367-1371)
    - .planning/phases/01-foundation/01-VALIDATION.md (TestFirstRun_Gate, TestPostFinish_NoWizardAccess names)
  </read_first>
  <behavior>
    - TestFirstRun_Gate_RedirectsHTML: empty DB; GET / → 307 Location: /install (HTML, no Accept: application/json header).
    - TestFirstRun_Gate_API_Returns409: empty DB; GET /api/anything → 409 + body `{"error": "install_required"}`.
    - TestFirstRun_Gate_Whitelist: empty DB; whitelisted paths (`/install`, `/api/install/state`, `/health`, `/assets/x.js`) → pass through (200/404 from inner handler, never 307/409).
    - TestPostFinish_NoWizardAccess: seed admin user; GET /install → next.ServeHTTP (gate is pass-through). After admin exists, the gate caches the answer (PITFALL #10).
    - TestFirstRun_Gate_Cache: admin exists; first request triggers DB query; subsequent requests do NOT (verified by counting queries via a stub).
  </behavior>
  <action>
1. Create `internal/install/middleware.go`:
   ```go
   package install

   import (
       "context"
       "encoding/json"
       "log/slog"
       "net/http"
       "strings"
       "sync/atomic"

       "github.com/jackc/pgx/v5/pgxpool"
   )

   // FirstRunGate is the canonical D-08 middleware. It checks "does any admin user exist?"
   // and either:
   //   - allows the request (admin exists OR path is whitelisted)
   //   - redirects HTML traffic to /install (no admin)
   //   - returns 409 install_required for /api/* traffic (no admin)
   //
   // PITFALL #10: cache the "true" result via atomic.Bool so re-checks don't hit DB.
   // Once an admin exists, the answer never reverts (deleting admins keeps audit row).
   func FirstRunGate(pool *pgxpool.Pool, log *slog.Logger) func(http.Handler) http.Handler {
       var adminExistsCache atomic.Bool
       return func(next http.Handler) http.Handler {
           return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
               if isWhitelisted(r.URL.Path) {
                   next.ServeHTTP(w, r)
                   return
               }
               if adminExistsCache.Load() {
                   next.ServeHTTP(w, r)
                   return
               }
               exists, err := adminExists(r.Context(), pool)
               if err != nil {
                   log.Error("first-run gate: db error", "err", err)
                   http.Error(w, "bootstrap check failed", http.StatusInternalServerError)
                   return
               }
               if exists {
                   adminExistsCache.Store(true)
                   next.ServeHTTP(w, r)
                   return
               }
               // No admin yet: route based on whether this is API or HTML.
               if strings.HasPrefix(r.URL.Path, "/api/") {
                   w.Header().Set("Content-Type", "application/json")
                   w.WriteHeader(http.StatusConflict)
                   _ = json.NewEncoder(w).Encode(map[string]string{"error": "install_required"})
                   return
               }
               http.Redirect(w, r, "/install", http.StatusTemporaryRedirect)
           })
       }
   }

   func adminExists(ctx context.Context, pool *pgxpool.Pool) (bool, error) {
       var v bool
       err := pool.QueryRow(ctx,
           `SELECT EXISTS(SELECT 1 FROM "user" WHERE role = 'admin' AND disabled_at IS NULL)`,
       ).Scan(&v)
       return v, err
   }

   // isWhitelisted returns true for paths that must serve before the install completes.
   // Order matters: longest matches first.
   func isWhitelisted(path string) bool {
       // Wizard endpoints
       if path == "/install" || strings.HasPrefix(path, "/install/") {
           return true
       }
       if strings.HasPrefix(path, "/api/install/") || path == "/api/install" {
           return true
       }
       // Login route is allowed (otherwise login redirect would loop with /install)
       if path == "/login" {
           return true
       }
       // Public health
       if path == "/health" {
           return true
       }
       // SPA static assets — Vite builds with /assets/ prefix
       if strings.HasPrefix(path, "/assets/") {
           return true
       }
       // Common static suffixes (favicons, root SVGs)
       for _, suf := range []string{".svg", ".ico", ".png", ".woff2", ".woff", ".css", ".js", ".map"} {
           if strings.HasSuffix(path, suf) {
               return true
           }
       }
       return false
   }
   ```

2. Replace `internal/install/middleware_test.go`:
   ```go
   package install

   import (
       "context"
       "log/slog"
       "net/http"
       "net/http/httptest"
       "os"
       "testing"

       "github.com/shifter-io/shifter/internal/db"
       "github.com/shifter-io/shifter/internal/testsupport"
       "github.com/stretchr/testify/require"
   )

   func setupGate(t *testing.T) (*httptest.Server, func(adminEmail string)) {
       t.Helper()
       pool := testsupport.StartPostgres(t)
       require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))

       gate := FirstRunGate(pool, slog.New(slog.NewTextHandler(os.Stderr, nil)))
       inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
           w.Write([]byte("ok:" + r.URL.Path))
       })
       srv := httptest.NewServer(gate(inner))
       t.Cleanup(srv.Close)

       seedAdmin := func(email string) {
           _, err := pool.Exec(context.Background(),
               `INSERT INTO "user" (email, name, password_hash, role) VALUES ($1, $1, $2, 'admin')`,
               email, "$argon2id$v=19$m=19456,t=2,p=1$AAAA$AAAA")
           require.NoError(t, err)
       }
       return srv, seedAdmin
   }

   func TestFirstRun_Gate_RedirectsHTML(t *testing.T) {
       srv, _ := setupGate(t)
       cli := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
           return http.ErrUseLastResponse
       }}
       res, err := cli.Get(srv.URL + "/")
       require.NoError(t, err); defer res.Body.Close()
       require.Equal(t, http.StatusTemporaryRedirect, res.StatusCode)
       require.Equal(t, "/install", res.Header.Get("Location"))
   }

   func TestFirstRun_Gate_API_Returns409(t *testing.T) {
       srv, _ := setupGate(t)
       res, err := http.Get(srv.URL + "/api/settings/anything")
       require.NoError(t, err); defer res.Body.Close()
       require.Equal(t, http.StatusConflict, res.StatusCode)
   }

   func TestFirstRun_Gate_Whitelist(t *testing.T) {
       srv, _ := setupGate(t)
       for _, p := range []string{"/install", "/api/install/state", "/health", "/assets/index-abc.js", "/login"} {
           res, err := http.Get(srv.URL + p)
           require.NoError(t, err); defer res.Body.Close()
           require.Equal(t, http.StatusOK, res.StatusCode, "whitelist path %s should pass through", p)
       }
   }

   func TestPostFinish_NoWizardAccess(t *testing.T) {
       srv, seed := setupGate(t)
       seed("alice@example.com")
       res, err := http.Get(srv.URL + "/dashboard")
       require.NoError(t, err); defer res.Body.Close()
       require.Equal(t, http.StatusOK, res.StatusCode, "post-install: gate is pass-through")
   }
   ```
  </action>
  <verify>
    <automated>go test ./internal/install -run 'TestFirstRun_|TestPostFinish_' -race -count=1 -v</automated>
  </verify>
  <acceptance_criteria>
    - File `internal/install/middleware.go` exports `func FirstRunGate(pool *pgxpool.Pool, log *slog.Logger) func(http.Handler) http.Handler`
    - File contains `var adminExistsCache atomic.Bool` and the cache is set after a positive `adminExists` result (PITFALL #10)
    - Whitelist includes `/install`, `/api/install/`, `/health`, `/assets/`, `/login`, and common static suffixes (`.svg`, `.ico`, `.css`, `.js`)
    - `/api/*` non-whitelisted paths return 409 + JSON `{"error":"install_required"}` when no admin
    - HTML traffic redirects to `/install` with 307
    - Command `go test ./internal/install -run TestFirstRun_Gate -race` exits 0 (per VALIDATION.md)
    - Command `go test ./internal/install -run TestPostFinish_NoWizardAccess -race` exits 0 (per VALIDATION.md)
    - All 4 middleware tests pass
  </acceptance_criteria>
  <done>
    First-run gate ready. Plan 18 wires `FirstRunGate(pool, log)` as the FIRST middleware after RequestID/Logger. Plan 16 wizard frontend consumes the 409 install_required response.
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| browser → /install | Untrusted form input crosses the install state JSONB |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-14-01 | Tampering | first-run race (two operators starting wizard simultaneously) | mitigate | `install_state` is singleton (CHECK id=1); finish (Plan 15) uses Serializable isolation. ASVS V11. |
| T-14-02 | Information Disclosure | install_state JSONB inspection from DB backups | mitigate | Phase 1 stores password HASH (Argon2id'd by Plan 15 step 1) — not plaintext; ChirpStack token stored as a path REF, not the raw value (RESEARCH §Open Question 1). ASVS V8. |
| T-14-03 | Spoofing | unauthenticated request reaches admin-only endpoint while admin doesn't exist | accept | The pre-install state has no users; install endpoints are explicitly whitelisted, all other admin endpoints return 409. ASVS V4. |
| T-14-04 | Tampering (SQL injection) | column name interpolated into UPDATE | mitigate | Column whitelisted (only `step1_admin`/`step2_chirpstack`/`step3_region`/`step4_identity`); never user input. ASVS V5. |
| T-14-05 | Denial of Service | first-run gate hits DB on every request | mitigate | atomic.Bool cache after first positive result; PITFALL #10. |
</threat_model>

<verification>
- `internal/install/state.go` Store works against testcontainer Postgres
- `internal/install/regions.go` includes Thailand AS923-2 default
- `FirstRunGate` redirects HTML, returns 409 for /api/, whitelists install/health/login/assets
- atomic.Bool cache prevents repeat DB hits
- 11 tests pass (6 state + 4 middleware + 1 region)
</verification>

<success_criteria>
- INST-01 unblocked: middleware redirects pre-install traffic
- D-08 enforced: gate based on admin existence, not flag files
- D-10/D-11 supporting infrastructure: Store with reentrant GetOrCreate + per-step JSONB writes
- INST-04 supporting: regions catalog with Thailand default
- PITFALL #10 prevented (cache after positive)
</success_criteria>

<output>
After completion, create `.planning/phases/01-foundation/01-14-SUMMARY.md` documenting:
- Store API
- Regions catalog
- Whitelist contract for FirstRunGate
- Cache semantics
</output>
