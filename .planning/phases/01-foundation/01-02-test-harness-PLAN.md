---
phase: 01-foundation
plan: 02
type: execute
wave: 2
depends_on: [01]
files_modified:
  - go.mod
  - go.sum
  - web/package.json
  - web/pnpm-lock.yaml
  - web/vitest.config.ts
  - web/src/test-setup.ts
  - internal/auth/argon2id_test.go
  - internal/auth/session_test.go
  - internal/auth/ratelimit_test.go
  - internal/auth/authz_test.go
  - internal/auth/account_test.go
  - internal/install/state_test.go
  - internal/install/middleware_test.go
  - internal/install/handlers_test.go
  - internal/chirpstack/version_test.go
  - internal/chirpstack/mqtt_test.go
  - internal/chirpstack/client_test.go
  - internal/db/migrations_test.go
  - internal/http/health_test.go
  - internal/http/testconn_test.go
  - internal/http/spa_test.go
  - internal/http/rbac_test.go
  - internal/http/session_persistence_test.go
  - internal/cli/serve_test.go
  - internal/version/version_test.go
  - internal/testsupport/postgres.go
  - internal/testsupport/mosquitto.go
  - internal/testsupport/chirpstack_mock.go
  - web/src/lib/auth.test.ts
  - web/src/routes/install/region-step.test.tsx
  - web/src/components/status-row.test.tsx
  - web/src/components/responsive-dialog.test.tsx
  - web/src/components/theme-provider.test.tsx
  - web/src/components/account-menu.test.tsx
  - web/src/routes/login.test.tsx
autonomous: true
requirements: []
must_haves:
  truths:
    - "All Wave 0 backend test files exist with at least one TODO-stub test that exits skip (so go test ./... compiles)"
    - "All Wave 0 frontend test files exist with at least one TODO-stub test that vitest skips"
    - "go test ./... -short -race exits 0 (every package has a parseable test file even if all tests are skipped)"
    - "pnpm --dir web test --run exits 0 with all tests skipped"
    - "testcontainers-go testify mockgen vitest @testing-library/react @testing-library/jest-dom jsdom installed"
  artifacts:
    - path: "internal/auth/argon2id_test.go"
      provides: "Test scaffolding for Argon2id Hash/Verify (filled in Plan 07)"
      contains: "func TestArgon2id_RoundTrip"
    - path: "internal/auth/ratelimit_test.go"
      provides: "Test scaffolding for per-IP and per-username rate limit (filled in Plan 09)"
      contains: "func TestLogin_RateLimit_PerIP"
    - path: "internal/install/state_test.go"
      provides: "Test scaffolding for wizard draft persistence and atomic commit (filled in Plan 14, 15)"
      contains: "func TestStep4_PersistsIdentity"
    - path: "internal/chirpstack/version_test.go"
      provides: "Test scaffolding for v3/v4 probe (filled in Plan 12)"
      contains: "func TestProbeVersion_v3"
    - path: "internal/testsupport/postgres.go"
      provides: "testcontainers-go helper that starts TimescaleDB and applies migrations (used by every integration test)"
      contains: "func StartPostgres"
    - path: "internal/testsupport/mosquitto.go"
      provides: "testcontainers-go helper that starts Mosquitto for MQTT integration tests"
      contains: "func StartMosquitto"
    - path: "internal/testsupport/chirpstack_mock.go"
      provides: "Helper that wires a mockgen-generated InternalServiceServer for v3/v4 simulation"
      contains: "func NewChirpStackMock"
    - path: "web/vitest.config.ts"
      provides: "vitest config: jsdom env, src/test-setup.ts setup file, ignore dist+node_modules"
      contains: "jsdom"
    - path: "web/src/test-setup.ts"
      provides: "@testing-library/jest-dom matcher imports"
      contains: "@testing-library/jest-dom"
  key_links:
    - from: "go test ./..."
      to: "internal/testsupport/postgres.go"
      via: "tests call testsupport.StartPostgres(t) which returns *pgxpool.Pool"
      pattern: "testsupport\\.StartPostgres"
    - from: "pnpm --dir web test"
      to: "web/src/test-setup.ts"
      via: "vitest config setupFiles loads jest-dom matchers globally"
      pattern: "setupFiles"
---

<objective>
Install all test dependencies (Go: testify, testcontainers-go, mockgen; Frontend: vitest, @testing-library/react, jsdom) and create skeleton test files for every entity that later plans will fill in. This is Wave 0 — every later plan's `<verify>` block points at a test that EXISTS as a stub today and gets bodies filled in by the implementing plan.

Purpose: Per VALIDATION.md and RESEARCH.md §Validation Architecture, no plan task can have a `<verify>` pointing at a missing file. This plan creates the canonical scaffolding so every subsequent verify command names a real, parseable test file.

Output: `go test ./... -race` and `pnpm --dir web test --run` both exit 0 with every required test file present (most as `t.Skip("Plan NN: pending")`).
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/01-foundation/01-VALIDATION.md
@.planning/phases/01-foundation/01-RESEARCH.md
@.planning/phases/01-foundation/01-CONTEXT.md
@01-01-repo-scaffold-PLAN.md

<interfaces>
testcontainers-go pattern (used by every backend integration test):
```go
// internal/testsupport/postgres.go
package testsupport

import (
    "context"
    "testing"
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/testcontainers/testcontainers-go"
    "github.com/testcontainers/testcontainers-go/modules/postgres"
)

// StartPostgres starts a fresh TimescaleDB container with a unique DB,
// applies all migrations, returns *pgxpool.Pool. Container is t.Cleanup'd.
func StartPostgres(t *testing.T) *pgxpool.Pool { ... }
```

vitest config target (from VALIDATION.md):
```ts
// web/vitest.config.ts
import { defineConfig, mergeConfig } from 'vitest/config'
import viteConfig from './vite.config'
export default mergeConfig(viteConfig, defineConfig({
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test-setup.ts'],
    globals: true,
  },
}))
```

Test naming canon (from VALIDATION.md per-task verification map):
- `TestLogin_Success`, `TestLogin_RateLimit_PerIP`, `TestLogin_RateLimit_PerUsername`
- `TestSession_IdleTimeout`, `TestSessionPersistence`
- `TestVerify_BadPassword_ConstantTime`, `TestArgon2id_RoundTrip`
- `TestAccount_ChangePassword`, `TestAccount_ChangePassword_RevokeOtherSessions`
- `TestRBAC_AdminAllowed`, `TestRBAC_ViewerForbidden`
- `TestWizardAdmin_NoForceChange`
- `TestFirstRun_Gate`, `TestPostFinish_NoWizardAccess`
- `TestStep2_CapturesCS`, `TestStep2_RejectsV3`
- `TestStep3_PersistsRegion`
- `TestStep4_PersistsIdentity`
- `TestProbeVersion_v3`, `TestProbeVersion_v4`, `TestClient_ListDevices_Mock`
- `TestMQTT_ReconnectResubscribe`, `TestMQTT_UplinkLogged`
- `TestHealth_Public`, `TestHealthDetailed_RequiresAdmin`
- `TestTestConn_Happy`, `TestTestConn_V3Refused`
- `TestSPA_FallbackIndex`, `TestSPA_NoFallbackForAPI`
- `TestServe_RefusesV3`
- `TestImageTagPinned`
- `TestRunMigrations_Clean`, `TestRunMigrations_Idempotent`, `TestRunMigrations_DirtyState`
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 1: Install Go test deps + write testsupport helpers + create all backend stub test files</name>
  <files>go.mod, go.sum, internal/testsupport/postgres.go, internal/testsupport/mosquitto.go, internal/testsupport/chirpstack_mock.go, internal/auth/argon2id_test.go, internal/auth/session_test.go, internal/auth/ratelimit_test.go, internal/auth/authz_test.go, internal/auth/account_test.go, internal/install/state_test.go, internal/install/middleware_test.go, internal/install/handlers_test.go, internal/chirpstack/version_test.go, internal/chirpstack/mqtt_test.go, internal/chirpstack/client_test.go, internal/db/migrations_test.go, internal/http/health_test.go, internal/http/testconn_test.go, internal/http/spa_test.go, internal/http/rbac_test.go, internal/http/session_persistence_test.go, internal/cli/serve_test.go, internal/version/version_test.go</files>
  <read_first>
    - .planning/phases/01-foundation/01-VALIDATION.md (per-task verification map — every test name comes from here)
    - .planning/phases/01-foundation/01-RESEARCH.md §"Validation Architecture" (lines 1730-1826)
    - .planning/phases/01-foundation/01-RESEARCH.md §"Wave 0 Gaps" (lines 1796-1825)
    - 01-01-repo-scaffold-PLAN.md (file paths and module name)
  </read_first>
  <action>
1. Install Go test deps:
   ```bash
   go get github.com/stretchr/testify@latest
   go get github.com/testcontainers/testcontainers-go@latest
   go get github.com/testcontainers/testcontainers-go/modules/postgres@latest
   go get go.uber.org/mock/mockgen@latest
   ```
   Vendor `github.com/eclipse/paho.mqtt.golang` and `github.com/jackc/pgx/v5` already loaded by Plan 03/13 — Plan 02 only ensures testify and testcontainers exist now.

2. Create `internal/testsupport/postgres.go`:
   ```go
   package testsupport

   import (
       "context"
       "testing"
       "time"

       "github.com/jackc/pgx/v5/pgxpool"
       tc "github.com/testcontainers/testcontainers-go"
       "github.com/testcontainers/testcontainers-go/modules/postgres"
       "github.com/testcontainers/testcontainers-go/wait"
   )

   // StartPostgres starts TimescaleDB 2.26 in a container, returns a pool.
   // Migrations are NOT applied here — caller invokes db.RunMigrations
   // (Plan 03 implements that helper).
   func StartPostgres(t *testing.T) *pgxpool.Pool {
       t.Helper()
       ctx := context.Background()
       container, err := postgres.Run(ctx,
           "timescale/timescaledb:2.26.0-pg16",
           postgres.WithDatabase("shifter_test"),
           postgres.WithUsername("shifter"),
           postgres.WithPassword("shifter"),
           tc.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").
               WithOccurrence(2).WithStartupTimeout(60*time.Second)),
       )
       if err != nil {
           t.Fatalf("postgres container: %v", err)
       }
       t.Cleanup(func() {
           _ = container.Terminate(context.Background())
       })

       dsn, err := container.ConnectionString(ctx, "sslmode=disable")
       if err != nil {
           t.Fatalf("postgres dsn: %v", err)
       }
       pool, err := pgxpool.New(ctx, dsn)
       if err != nil {
           t.Fatalf("pgxpool: %v", err)
       }
       t.Cleanup(pool.Close)
       return pool
   }
   ```

3. Create `internal/testsupport/mosquitto.go`:
   ```go
   package testsupport

   import (
       "context"
       "testing"
       "time"

       tc "github.com/testcontainers/testcontainers-go"
       "github.com/testcontainers/testcontainers-go/wait"
   )

   // StartMosquitto starts eclipse-mosquitto:2 with anonymous access enabled.
   // Returns the broker URL like tcp://localhost:32789.
   func StartMosquitto(t *testing.T) string {
       t.Helper()
       ctx := context.Background()
       req := tc.ContainerRequest{
           Image:        "eclipse-mosquitto:2.0.18",
           ExposedPorts: []string{"1883/tcp"},
           WaitingFor:   wait.ForLog("mosquitto version").WithStartupTimeout(30 * time.Second),
           Cmd:          []string{"mosquitto", "-c", "/mosquitto-no-auth.conf"},
       }
       cont, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{
           ContainerRequest: req, Started: true,
       })
       if err != nil {
           t.Fatalf("mosquitto: %v", err)
       }
       t.Cleanup(func() { _ = cont.Terminate(context.Background()) })
       host, _ := cont.Host(ctx)
       port, _ := cont.MappedPort(ctx, "1883/tcp")
       return "tcp://" + host + ":" + port.Port()
   }
   ```

4. Create `internal/testsupport/chirpstack_mock.go` as a STUB documenting the pattern Plan 12 will fill:
   ```go
   package testsupport

   import "testing"

   // NewChirpStackMock returns a gRPC server address that simulates ChirpStack.
   // Plan 12 implements this using mockgen-generated InternalServiceServer.
   //
   // mode: "v4" — GetVersion returns a successful response
   //       "v3" — GetVersion returns codes.Unimplemented
   //       "down" — server refuses connection
   func NewChirpStackMock(t *testing.T, mode string) (addr string, apiToken string) {
       t.Helper()
       t.Skip("Plan 12: ChirpStack gRPC mock pending")
       return "", ""
   }
   ```

5. Create EVERY backend stub test file listed in `<files>` above. Use this template (substitute the test names from `<interfaces>` per file):
   ```go
   package auth // or install, chirpstack, db, http, cli, version

   import "testing"

   func TestArgon2id_RoundTrip(t *testing.T) {
       t.Skip("Plan 07: Argon2id Hash+Verify implementation pending")
   }
   func TestVerify_BadPassword_ConstantTime(t *testing.T) {
       t.Skip("Plan 07: pending")
   }
   ```
   The plan-to-test mapping (so executor knows which test belongs in which file):

   - `internal/auth/argon2id_test.go`: `TestArgon2id_RoundTrip`, `TestVerify_BadPassword_ConstantTime`, `TestArgon2id_PHCParseError`
   - `internal/auth/session_test.go`: `TestSession_IdleTimeout`, `TestSession_DevSecureToggle`
   - `internal/auth/ratelimit_test.go`: `TestLogin_RateLimit_PerIP`, `TestLogin_RateLimit_PerUsername`, `TestRateLimit_Cleanup`
   - `internal/auth/authz_test.go`: `TestCan_AdminAllowsAll`, `TestCan_ViewerDenied`, `TestCan_NilUser`
   - `internal/auth/account_test.go`: `TestAccount_ChangePassword`, `TestAccount_ChangePassword_RevokeOtherSessions`, `TestWizardAdmin_NoForceChange`
   - `internal/install/state_test.go`: `TestStep1_PersistsAdmin`, `TestStep2_CapturesCS`, `TestStep2_RejectsV3`, `TestStep3_PersistsRegion`, `TestStep4_PersistsIdentity`, `TestFinishSetup_Atomic`
   - `internal/install/middleware_test.go`: `TestFirstRun_Gate`, `TestPostFinish_NoWizardAccess`
   - `internal/install/handlers_test.go`: `TestInstallState_Reentrant`
   - `internal/chirpstack/version_test.go`: `TestProbeVersion_v4`, `TestProbeVersion_v3`
   - `internal/chirpstack/mqtt_test.go`: `TestMQTT_ReconnectResubscribe`, `TestMQTT_UplinkLogged`
   - `internal/chirpstack/client_test.go`: `TestClient_ListDevices_Mock`
   - `internal/db/migrations_test.go`: `TestRunMigrations_Clean`, `TestRunMigrations_Idempotent`, `TestRunMigrations_DirtyState`
   - `internal/http/health_test.go`: `TestHealth_Public`, `TestHealthDetailed_RequiresAdmin`
   - `internal/http/testconn_test.go`: `TestTestConn_Happy`, `TestTestConn_V3Refused`, `TestTestConn_BothFail`
   - `internal/http/spa_test.go`: `TestSPA_FallbackIndex`, `TestSPA_NoFallbackForAPI`, `TestSPA_AssetCacheHeaders`
   - `internal/http/rbac_test.go`: `TestRBAC_AdminAllowed`, `TestRBAC_ViewerForbidden`
   - `internal/http/session_persistence_test.go`: `TestSessionPersistence`, `TestLogin_Success`
   - `internal/cli/serve_test.go`: `TestServe_RefusesV3`, `TestServe_AutoMigrate`
   - `internal/version/version_test.go`: `TestImageTagPinned`

6. Each file must compile under its own package (so the package directory itself does not have to exist as a non-test source — Go allows tests in a directory with no `.go` files yet, but the package declaration must be consistent across files in that dir). Pre-create the package directories: `internal/auth`, `internal/install`, `internal/chirpstack`, `internal/db`, `internal/http`, `internal/cli`, `internal/version`. Each gets a `doc.go` with one line: `// Package X — described in 01-RESEARCH.md.` so `go vet` passes.

7. Run `go test ./... -short -race` — every test must skip cleanly (exit 0).
  </action>
  <verify>
    <automated>go test ./... -short -race 2>&1 | tee /tmp/test-out.txt && grep -q 'PASS' /tmp/test-out.txt && ! grep -q 'FAIL' /tmp/test-out.txt</automated>
  </verify>
  <acceptance_criteria>
    - File `internal/testsupport/postgres.go` exists and exports `func StartPostgres(t *testing.T) *pgxpool.Pool`
    - File `internal/testsupport/mosquitto.go` exists and exports `func StartMosquitto(t *testing.T) string`
    - File `internal/testsupport/chirpstack_mock.go` exists and exports `func NewChirpStackMock(t *testing.T, mode string) (string, string)`
    - All 20 backend test files listed in `<files>` exist
    - For each test name in the plan-to-test mapping above, `grep -r "func ${TEST_NAME}" internal/` returns at least one match
    - Each package directory listed has a `doc.go`
    - `go.sum` includes lines for `github.com/stretchr/testify` and `github.com/testcontainers/testcontainers-go`
    - Command `go test ./... -short -race` exits 0
    - Command `go vet ./...` exits 0
  </acceptance_criteria>
  <done>
    Every backend Wave 0 test file exists with skip-stubs. testcontainers helpers wired. Subsequent plans fill in test bodies and switch `t.Skip` to real assertions.
  </done>
</task>

<task type="auto">
  <name>Task 2: Install frontend test deps + write vitest config + create all frontend stub test files</name>
  <files>web/package.json, web/pnpm-lock.yaml, web/vitest.config.ts, web/src/test-setup.ts, web/src/lib/auth.test.ts, web/src/routes/install/region-step.test.tsx, web/src/components/status-row.test.tsx, web/src/components/responsive-dialog.test.tsx, web/src/components/theme-provider.test.tsx, web/src/components/account-menu.test.tsx, web/src/routes/login.test.tsx</files>
  <read_first>
    - .planning/phases/01-foundation/01-VALIDATION.md (frontend test files list, lines 105-112)
    - 01-01-repo-scaffold-PLAN.md (vitest.config.ts must extend vite.config.ts via mergeConfig)
  </read_first>
  <action>
1. Install frontend test deps from `web/`:
   ```bash
   cd web && pnpm add -D vitest @vitest/ui @testing-library/react @testing-library/jest-dom @testing-library/user-event jsdom happy-dom
   ```
   Use `jsdom` (per VALIDATION.md, more compat than happy-dom for shadcn portals).

2. Add to `web/package.json` scripts:
   ```json
   "test": "vitest",
   "test:run": "vitest --run",
   "test:ui": "vitest --ui"
   ```
   Update the existing `test --run` script in package.json so `pnpm test --run` works as VALIDATION.md commands expect.

3. Create `web/vitest.config.ts`:
   ```ts
   import { defineConfig, mergeConfig } from 'vitest/config'
   import viteConfig from './vite.config'

   export default mergeConfig(
     viteConfig,
     defineConfig({
       test: {
         environment: 'jsdom',
         setupFiles: ['./src/test-setup.ts'],
         globals: true,
         exclude: ['node_modules', 'dist'],
         include: ['src/**/*.{test,spec}.{ts,tsx}'],
       },
     }),
   )
   ```

4. Create `web/src/test-setup.ts`:
   ```ts
   import '@testing-library/jest-dom/vitest'

   // Mock matchMedia for theme-provider tests (jsdom doesn't implement it)
   Object.defineProperty(window, 'matchMedia', {
     writable: true,
     value: (query: string) => ({
       matches: false,
       media: query,
       onchange: null,
       addEventListener: () => {},
       removeEventListener: () => {},
       addListener: () => {},
       removeListener: () => {},
       dispatchEvent: () => false,
     }),
   })
   ```

5. Create stub test files. Each starts with vitest's `describe.skip` so no test fails before its implementing plan ships:

   `web/src/lib/auth.test.ts`:
   ```ts
   import { describe, it } from 'vitest'

   describe.skip('auth fetch wrapper (Plan 11/16)', () => {
     it('redirects on 401', () => {})
     it('attaches X-Requested-With header', () => {})
   })
   ```

   `web/src/routes/install/region-step.test.tsx`:
   ```tsx
   import { describe, it } from 'vitest'

   describe.skip('Wizard step 3 region picker (Plan 16)', () => {
     it('defaults AS923-2 for Thailand', () => {})
     it('shows other regions grouped', () => {})
   })
   ```

   `web/src/components/status-row.test.tsx`:
   ```tsx
   import { describe, it } from 'vitest'

   describe.skip('StatusRow component (Plan 06/17)', () => {
     it('renders reachable state with green dot', () => {})
     it('renders unreachable state with red icon', () => {})
     it('renders skipped state with muted icon', () => {})
   })
   ```

   `web/src/components/responsive-dialog.test.tsx`:
   ```tsx
   import { describe, it } from 'vitest'

   describe.skip('ResponsiveDialog (Plan 06)', () => {
     it('renders Dialog on md+ viewport', () => {})
     it('renders Sheet bottom on <md viewport', () => {})
   })
   ```

   `web/src/components/theme-provider.test.tsx`:
   ```tsx
   import { describe, it } from 'vitest'

   describe.skip('ThemeProvider (Plan 06)', () => {
     it('persists choice in localStorage', () => {})
     it('applies class="dark" to root when dark', () => {})
     it('respects system when set to system', () => {})
   })
   ```

   `web/src/components/account-menu.test.tsx`:
   ```tsx
   import { describe, it } from 'vitest'

   describe.skip('AccountMenu (Plan 11)', () => {
     it('hides admin-only items from viewer', () => {})
     it('shows Change password and Sign out', () => {})
   })
   ```

   `web/src/routes/login.test.tsx`:
   ```tsx
   import { describe, it } from 'vitest'

   describe.skip('Login screen (Plan 23)', () => {
     it('renders shadcn navy primary CSS variable', () => {})
     it('uses Inter font family', () => {})
     it('has English copy "Sign in to Shifter"', () => {})
   })
   ```

6. Run `pnpm test --run` and confirm all suites are skipped, exit 0.
  </action>
  <verify>
    <automated>cd web && pnpm install --frozen-lockfile && pnpm test:run 2>&1 | tee /tmp/web-test.txt && grep -q 'skipped' /tmp/web-test.txt && ! grep -q 'failed' /tmp/web-test.txt</automated>
  </verify>
  <acceptance_criteria>
    - File `web/vitest.config.ts` exists and contains `environment: 'jsdom'` and `setupFiles: ['./src/test-setup.ts']`
    - File `web/src/test-setup.ts` exists and contains `import '@testing-library/jest-dom/vitest'`
    - All 7 frontend test files listed in `<files>` exist
    - Each frontend test file uses `describe.skip(...)` (so they exist as parseable test suites without failing)
    - `web/package.json` devDependencies include: `vitest`, `@testing-library/react`, `@testing-library/jest-dom`, `@testing-library/user-event`, `jsdom`
    - `web/package.json` scripts include `"test:run": "vitest --run"` exactly
    - Command `cd web && pnpm test:run` exits 0 with all suites reported as skipped
    - Command `cd web && pnpm install --frozen-lockfile` exits 0 (lockfile is up to date)
  </acceptance_criteria>
  <done>
    Frontend Wave 0 test scaffolding live. vitest runs in jsdom with jest-dom matchers globally. Subsequent plans replace `describe.skip(...)` with real component tests.
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| test container → host | Postgres/Mosquitto containers expose ephemeral ports; isolated to dev |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-02-01 | Tampering | testcontainers image supply chain | mitigate | Pin specific tags (`timescale/timescaledb:2.26.0-pg16`, `eclipse-mosquitto:2.0.18`); ASVS V10 / OPS-07. |
| T-02-02 | Information Disclosure | test secrets accidentally leak to ci logs | accept | Test-only credentials are `shifter/shifter`, isolated DB. ASVS V8. |
| T-02-03 | Denial of Service | unbounded testcontainers consume host resources | mitigate | `t.Cleanup` always terminates containers; CI uses container-per-test pattern. |
</threat_model>

<verification>
- `go test ./... -short -race` exits 0 with all skips reported
- `pnpm --dir web test:run` exits 0 with all skips reported
- `internal/testsupport/postgres.go` and `internal/testsupport/mosquitto.go` use pinned image tags
- `web/vitest.config.ts` extends vite config via mergeConfig
- All 27 stub test files in `<files>` exist
</verification>

<success_criteria>
- Every Wave 0 test file from VALIDATION.md exists, even if its body is a skip-stub
- testify, testcontainers-go, mockgen installed (Go side)
- vitest, @testing-library/react, jsdom installed (frontend side)
- Subsequent plans can fill test bodies without creating new files or installing new test deps
- `go vet ./...` and `go test ./... -short` both exit 0
</success_criteria>

<output>
After completion, create `.planning/phases/01-foundation/01-02-SUMMARY.md` documenting:
- Test deps installed (versions)
- Mapping of test name → file (so later plans know exactly where to fill bodies)
- testcontainers helper signatures
- Any test infrastructure patterns to reuse
</output>
