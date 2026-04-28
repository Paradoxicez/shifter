---
phase: 01-foundation
plan: 14
subsystem: install
tags: [go, middleware, install-wizard, postgres, pgxpool, atomic-cache, lorawan-regions]

requires:
  - phase: 01-foundation
    plan: 03
    provides: install_state singleton table (CHECK id=1) + 0002 user table with role/disabled_at columns
  - phase: 01-foundation
    plan: 09
    provides: auth.Store user-table facade (AdminExists pattern, lowercase email, disabled_at filter)
provides:
  - internal/install.Store — Store{pool} with GetOrCreate / UpdateStep1..4 / Delete; singleton CHECK id=1 enforcement via INSERT .. ON CONFLICT DO NOTHING; whitelisted column-name interpolation for per-step JSONB writes; monotonic current_step via GREATEST
  - internal/install.State — typed snapshot of install_state row (StartedAt, CompletedAt, CurrentStep, Step{1..4}{Admin,ChirpStack,Region,Identity})
  - internal/install.FirstRunGate — http.Handler factory with one-way atomic.Bool cache (PITFALL #10); HTML→307 /install, /api/*→409 install_required when no admin; whitelist for wizard, login, health, /assets/, common static suffixes
  - internal/install.Regions — 8-entry hardcoded LoRaWAN region catalog (AS923-1..4, EU868, US915, AU915, IN865) with Thailand pre-selection (AS923-2, DefaultForCountry="TH")
  - internal/install.RegionByName — whitelist lookup (T-14-04 mitigation for Plan 15)
affects: [01-15-install-handlers, 01-16-install-wizard-ui, 01-17-test-connection, 01-18-router-health]

tech-stack:
  added: []
  patterns:
    - "Pattern: Singleton-row store via INSERT .. ON CONFLICT (id) DO NOTHING + SELECT WHERE id = 1. Phase-1 third use of the singleton pattern (install_state, install_identity, chirpstack_connection); locks the read path: never SELECT * without the id=1 predicate."
    - "Pattern: Per-step UPDATE column whitelist. updateStep accepts only hardcoded column names from the four call sites (step1_admin / step2_chirpstack / step3_region / step4_identity) — payload remains $-bound. T-14-04 mitigation; future per-step writes MUST use this pattern, never accept user-supplied column names."
    - "Pattern: Monotonic step progression via GREATEST(current_step, $N). Re-submitting an earlier step never regresses the wizard pointer; collapses the race between two browser tabs editing different steps to a deterministic outcome."
    - "Pattern: One-way atomic.Bool cache for install-completion gate. PITFALL #10 mitigation locks: once set true, NEVER reset — even if every admin row is later soft-deleted, the install has demonstrably been completed. Future `is-bootstrapped`-style gates MUST use this one-way pattern, not a TTL cache."
    - "Pattern: Gate-handler routing on Accept-vs-path. /api/* → 409 JSON; everything else → 307 redirect. Plans 16/23 frontend code consumes the 409 install_required response for SPA routing; Plan 18 wires the gate as the FIRST middleware after RequestID/Logger."
    - "Pattern: Whitelist by path prefix + suffix (no Accept-header sniffing). Prefix bucket: /install, /api/install, /login, /health, /assets/. Suffix bucket: .svg .ico .png .woff* .css .js .map. Fast (string ops only) and operator-readable; Phase 2+ static assets that must serve pre-install (e.g. floor-plan thumbnails — but those WON'T need to serve pre-install) inherit by extending the buckets, not adding a new code path."

key-files:
  created:
    - internal/install/state.go
    - internal/install/regions.go
    - internal/install/middleware.go
  modified:
    - internal/install/state_test.go
    - internal/install/middleware_test.go
    - internal/install/handlers_test.go

key-decisions:
  - "FirstRunGate gate uses one-way atomic.Bool cache (PITFALL #10). Once `adminExists` returns true, subsequent requests skip the DB query unconditionally. The cache never resets — a soft-delete of every admin row does not re-engage the gate (the install demonstrably completed). Plan 09 `shifter create-admin --reset` is the explicit recovery path; the gate trusts the operator's process boundary."
  - "Whitelist includes /login (not just /install). Plan 23 ships the login screen; reserving /login here means post-install logout doesn't bounce against the gate, and a stale session cookie's redirect chain (cookie expired → /login → /install loop) cannot form. /login must never serve a 307 → /install."
  - "Common static suffixes (.svg .ico .png .woff* .css .js .map) are whitelisted even outside /assets/. Vite builds put hashed assets under /assets/, but the wizard SPA (Plan 16) and login screen (Plan 23) ship favicons / logos at root paths (e.g. /favicon.ico, /logo.svg) that the browser fetches BEFORE the operator authenticates. Suffix-bucket avoids per-asset whitelist entries."
  - "adminExists filters disabled_at IS NOT NULL — symmetric with auth.Store.GetUserByEmail (Plan 09). Soft-deleting the only admin must NOT re-engage the install gate; the operator must `shifter create-admin --reset` (Plan 09) to recover. The disabled_at filter on Plan 09 + the one-way cache here are belt + suspenders."
  - "Plan 15 stubs (TestStep2_RejectsV3, TestFinishSetup_Atomic) relocated from state_test.go to handlers_test.go before replacing state_test.go. Keeps the test-file convention `state_test.go` covers Store-only behavior; `handlers_test.go` covers HTTP handler behavior. Plan 15 will replace handlers_test.go skip-stubs with real tests."
  - "Region catalog hardcoded for Phase 1 (RESEARCH §Pattern 13). 8 entries (AS923-1, AS923-2 TH-default, AS923-3, AS923-4, EU868, US915 sub-band 1, AU915 sub-band 1, IN865). A regulator-versioned catalog file is deferred to Phase 7; INST-04 (Thailand AS923-2 default) is the only Phase-1 acceptance check."
  - "Store.GetOrCreate scans the singleton row in a single round-trip after the upsert. INSERT .. ON CONFLICT DO NOTHING + SELECT is two queries on first call, one on every re-entry. RETURNING was considered — pgx's `INSERT .. ON CONFLICT DO NOTHING RETURNING *` returns zero rows on conflict, requiring a fall-back SELECT anyway, so the simpler two-statement form was chosen."
  - "Cache-prevention test (TestFirstRun_Gate_Cache) asserts via DROP TABLE \"user\" CASCADE mid-test. After the first request primes the cache, dropping the user table would surface as a 500 if the gate were still hitting the DB; the test passes only when the cached path bypasses the SQL entirely. This is a stronger assertion than counting query calls because the test imposes a state in which the `SELECT EXISTS(...)` query CANNOT succeed."

patterns-established:
  - "Pattern: Install-flow per-step JSONB write. Each UpdateStepN(payload) receives a pre-validated []byte from the handler; Store does not interpret content, only persists it and bumps current_step monotonically. Plan 15 handlers MUST validate payload structure before passing through; Store is intentionally schema-agnostic to avoid a churn point when Plan 15 evolves the wizard."
  - "Pattern: Region whitelist consumed via RegionByName. Plan 15 step 3 handler MUST call RegionByName(payload.name) and reject the submission if !ok — never trust the JSONB payload's `name` field as-is (T-14-04 / ASVS V5)."
  - "Pattern: One-way completion gate. The install gate is the first instance; future Phase 2+ feature gates with similar 'demonstrably done' semantics (e.g. 'has any device been provisioned?') should reuse the atomic.Bool one-way pattern rather than introducing TTL caches."

requirements-completed:
  - INST-01

duration: ~7min
completed: 2026-04-28
---

# Phase 01 Plan 14: Install Middleware Summary

**install_state Store (singleton-row CRUD with whitelisted per-step JSONB writes), 8-entry hardcoded LoRaWAN region catalog (Thailand AS923-2 default), and FirstRunGate middleware with a one-way atomic.Bool cache that redirects HTML to /install or returns 409 install_required for /api/* when no admin user exists.**

## Performance

- **Duration:** ~7 min
- **Started:** 2026-04-28T01:55Z (approx)
- **Completed:** 2026-04-28T02:03Z
- **Tasks:** 2 / 2 (both TDD: RED → GREEN)
- **Files created:** 3 (`state.go`, `regions.go`, `middleware.go`)
- **Files modified:** 3 (`state_test.go`, `middleware_test.go`, `handlers_test.go` — relocated Plan-15 stubs)

## Accomplishments

- `go test ./internal/install -race -count=1` exits 0 with 12 tests passing — 6 Store tests + 1 Regions test + 5 FirstRunGate tests.
- `internal/install.Store` exports the canonical install_state CRUD: `GetOrCreate` (idempotent singleton), `UpdateStep1..4` (whitelisted column + monotonic current_step), `Delete` (Plan 15 finish-cleanup hook).
- `internal/install.FirstRunGate` redirects HTML traffic to `/install` (307) and returns `409 {"error":"install_required"}` for `/api/*` traffic when no admin user exists; once an admin row is found, an atomic.Bool caches the answer and ALL subsequent requests skip the DB query (PITFALL #10 prevention).
- `internal/install.Regions` ships the 8-entry hardcoded LoRaWAN region catalog with the Thailand AS923-2 default (INST-04 anchor); `RegionByName` is the whitelist lookup Plan 15's step-3 handler will use.
- INST-01 is now ground-truth-testable end-to-end via `TestFirstRun_Gate_RedirectsHTML` and `TestPostFinish_NoWizardAccess`.

## Task Commits

Each task followed strict TDD (RED → GREEN); refactor passes were unnecessary because the GREEN implementations cleared the test contract on first authoring.

1. **Task 1 RED:** `c8e2670` — `test(01-14): add failing tests for install Store + Regions catalog`
2. **Task 1 GREEN:** `f958636` — `feat(01-14): install Store + Regions catalog`
3. **Task 2 RED:** `ce116c4` — `test(01-14): add failing tests for FirstRunGate middleware`
4. **Task 2 GREEN:** `d8fa547` — `feat(01-14): FirstRunGate middleware (D-08, PITFALL #10)`

**Plan metadata:** _pending — created at end of plan_

## Files Created/Modified

- `internal/install/state.go` — Store + State; singleton GetOrCreate (INSERT .. ON CONFLICT DO NOTHING + SELECT WHERE id=1); UpdateStep1..4 share `updateStep(column, nextStep, payload)` with whitelisted column-name interpolation; Delete (Plan 15 finish-cleanup hook)
- `internal/install/regions.go` — Region struct + Regions() catalog (8 entries) + RegionByName() whitelist lookup
- `internal/install/middleware.go` — FirstRunGate factory; one-way atomic.Bool cache; isWhitelisted (prefix bucket + suffix bucket); adminExists (single SQL EXISTS, disabled_at filter)
- `internal/install/state_test.go` — Replaced Plan-02 skip-stubs with 7 tests (GetOrCreate new + reentrant, UpdateStep1..4, Regions Thailand default)
- `internal/install/middleware_test.go` — Replaced Plan-02 skip-stubs with 5 tests (RedirectsHTML, API_Returns409, Whitelist, PostFinish_NoWizardAccess, Gate_Cache)
- `internal/install/handlers_test.go` — Inherited Plan-15 skip-stubs (TestInstallState_Reentrant) plus relocated stubs (TestStep2_RejectsV3, TestFinishSetup_Atomic) so Plan 15 has a clean handlers-only test file to fill in

## Decisions Made

See `key-decisions` in frontmatter for the canonical list. Highlights:

- **One-way atomic.Bool cache** (PITFALL #10) — never resets, even if every admin row is soft-deleted. Plan 09 `shifter create-admin --reset` is the explicit recovery path.
- **Whitelist includes `/login`** — reserves the post-install login route so logout doesn't bounce against the gate; defends against a stale-cookie redirect loop.
- **Common static suffixes whitelisted globally** (not just under /assets/) — accommodates root-level /favicon.ico, /logo.svg the browser fetches before authentication.
- **`adminExists` filters disabled_at IS NOT NULL** — symmetric with auth.Store.GetUserByEmail (Plan 09); soft-delete cannot re-engage the install gate.
- **Plan-15 stubs relocated** to `handlers_test.go` so `state_test.go` covers Store-only behavior. Keeps the test-file convention clean: state-vs-handlers split mirrors the source files.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Plan-verbatim test seed used short Argon2id hash that violated PHC parser's salt/hash length floors**

- **Found during:** Task 2 (FirstRunGate tests, `setupGate.seedAdmin`)
- **Issue:** Plan's verbatim seed hash `$argon2id$v=19$m=19456,t=2,p=1$AAAA$AAAA` would be rejected by Plan 07's `auth.Verify` if anything later validated it. The middleware tests don't invoke Verify (they only check column presence), so the short hash works for THIS plan, but a future copy-paste into a Plan-15 test that DOES verify would silently fail.
- **Fix:** Replaced with a longer pad-zeroed hash (`$argon2id$v=19$m=19456,t=2,p=1$AAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA`) that passes Plan 07's PHC parser even though no test in this plan actually verifies it. Defense in depth against test-helper drift.
- **Files modified:** `internal/install/middleware_test.go`
- **Verification:** All 5 middleware tests pass; the seed hash is referenced only by the gate's `EXISTS` query path (column existence, not content).
- **Committed in:** `ce116c4` (Task 2 RED)

**2. [Rule 2 - Missing Critical] handlers_test.go relocation to keep Plan-15 surface intact**

- **Found during:** Task 1 RED, before replacing state_test.go.
- **Issue:** The plan instructed "Replace `internal/install/state_test.go`" with the new tests. The existing state_test.go held three Plan-15 skip-stubs (`TestStep2_RejectsV3`, `TestFinishSetup_Atomic`, plus `TestStep1_PersistsAdmin` / `TestStep2_CapturesCS` / `TestStep3_PersistsRegion` / `TestStep4_PersistsIdentity`). The latter four are owned by THIS plan; the first two are Plan-15 territory (they require handlers, not just Store). Wholesale replacement would have deleted the Plan-15 stubs and broken Wave 0's "stub-then-fill" contract (Plan 02 decision).
- **Fix:** Relocated `TestStep2_RejectsV3` and `TestFinishSetup_Atomic` from state_test.go to handlers_test.go (alongside the existing `TestInstallState_Reentrant` stub). state_test.go now contains real Plan-14 tests; handlers_test.go contains the three Plan-15 skip-stubs.
- **Files modified:** `internal/install/handlers_test.go` (added two stubs), `internal/install/state_test.go` (removed Plan-15 stubs as part of the replace)
- **Verification:** `go test ./internal/install -count=1` shows 12 PASS + 3 SKIP (the relocated Plan-15 stubs), with no test deletions.
- **Committed in:** `c8e2670` (Task 1 RED)

**3. [Rule 2 - Missing Critical] Defense-in-depth cache test asserts via `DROP TABLE`, not query-counting**

- **Found during:** Task 2 (writing TestFirstRun_Gate_Cache).
- **Issue:** The plan suggested verifying the cache via "counting queries via a stub." Counting requires either (a) wrapping the pgxpool in a test interceptor or (b) running a Postgres extension that captures statement counts — both are setup-heavy. A simpler stronger test: prime the cache with a real admin row, then DROP the `"user"` table. If the cache works, subsequent requests pass; if it doesn't, the next `SELECT EXISTS(SELECT 1 FROM "user" ...)` errors with "relation user does not exist" and the gate returns 500.
- **Fix:** TestFirstRun_Gate_Cache prime → `DROP TABLE "user" CASCADE` → request → expect 200. Strictly stronger than query-counting because it imposes a state in which a cache-bypass request CANNOT succeed.
- **Files modified:** `internal/install/middleware_test.go`
- **Verification:** Test passes; manual sanity-check: removing the `adminExistsCache.Store(true)` line from middleware.go would make this test fail with a 500.
- **Committed in:** `ce116c4` (Task 2 RED) — implementation verified the assertion in `d8fa547` (Task 2 GREEN).

---

**Total deviations:** 3 auto-fixed (1 Rule 3 blocking, 2 Rule 2 missing critical)
**Impact on plan:** All deviations strengthen the test contract or preserve Wave-0 plan boundaries. No architectural intent changed; Plan 15 inherits a clean handlers_test.go to fill in.

## Issues Encountered

None — both tasks went RED → GREEN on the first authoring pass with no debug iterations. The full project test suite (`go test ./internal/... -short -race`) reports 106 tests passing across 11 packages after this plan; no regressions in upstream packages.

## User Setup Required

None — Plan 14 is fully scaffolded by code. Reproducing requires:

- Docker Engine running (testcontainers — TimescaleDB 2.26.0-pg16 image, ~150 MB; same image as Plans 02/03/07/08/09/10/11).

## Known Stubs

| Stub | File | Reason | Resolved by |
|------|------|--------|-------------|
| TestStep2_RejectsV3 (skip) | `internal/install/handlers_test.go` | Plan 15 owns the v3 rejection HTTP handler; Store-only Plan 14 cannot exercise the ProbeVersion → destructive banner path | Plan 15 |
| TestFinishSetup_Atomic (skip) | `internal/install/handlers_test.go` | Plan 15 implements the FinishSetup atomic transaction across user / chirpstack_connection / install_identity | Plan 15 |
| TestInstallState_Reentrant (skip) | `internal/install/handlers_test.go` | Plan 15 wires GET handlers that hydrate the form from Store.GetOrCreate; this plan's Store tests cover the underlying re-entrancy | Plan 15 |
| Store.Delete is exported but unused | `internal/install/state.go` | Plan 15's FinishSetup will call Delete after committing the drafts (admin user + chirpstack_connection + install_identity rows) | Plan 15 |

All stubs are tracked and explicitly scheduled for resolution in Plan 15.

## Threat Flags

None — implementation matches the threat register's `mitigate` dispositions:

- **T-14-01 (singleton race):** mitigated by CHECK (id = 1) at the schema level; Store reads/writes always include `WHERE id = 1`.
- **T-14-02 (JSONB inspection):** install_state stores the password HASH (Argon2id; Plan 15 will pass through from auth.Hash); ChirpStack token is stored as a path REF (Plan 15 surface), not raw value.
- **T-14-04 (SQL injection on column name):** updateStep accepts only hardcoded column names from the four `UpdateStepN` call sites; payload remains $-bound. Verified by code inspection — call sites pass string literals, never variables.
- **T-14-05 (DoS via per-request DB hit):** atomic.Bool cache; one DB query per process lifetime once an admin exists.

## Next Phase Readiness

- ✅ INST-01 acceptance test (`TestFirstRun_Gate`) passes against testcontainer Postgres.
- ✅ Plan 15 (install-handlers) can import `install.Store`, `install.Regions`, `install.RegionByName` directly — public API matches plan interface.
- ✅ Plan 16 (install-wizard-ui) consumes `install.Regions` JSON via Plan 15's `/api/install/regions` endpoint; the catalog is JSON-serializable as-is (struct tags present).
- ✅ Plan 18 (router-health) wires `install.FirstRunGate(pool, log)` as the FIRST middleware after RequestID/Logger; the factory signature matches the chi `func(http.Handler) http.Handler` shape.
- ✅ All 12 tests pass; `go vet ./...` clean; `go build ./...` clean.

## Self-Check: PASSED

Files verified to exist:
- FOUND: `internal/install/state.go`
- FOUND: `internal/install/regions.go`
- FOUND: `internal/install/middleware.go`
- FOUND: `internal/install/state_test.go`
- FOUND: `internal/install/middleware_test.go`
- FOUND: `internal/install/handlers_test.go`

Commits verified to exist:
- FOUND: `c8e2670` (Task 1 RED — failing state + regions tests)
- FOUND: `f958636` (Task 1 GREEN — Store + Regions implementation)
- FOUND: `ce116c4` (Task 2 RED — failing FirstRunGate tests)
- FOUND: `d8fa547` (Task 2 GREEN — FirstRunGate implementation)

Behavior verified:
- `go test ./internal/install -race -count=1` → 12 passed (7 from Task 1 + 5 from Task 2)
- `go test ./internal/install -run 'TestStep4_PersistsIdentity' -race` → exit 0 (VALIDATION.md target)
- `go test ./internal/install -run 'TestStep3_PersistsRegion' -race` → exit 0 (VALIDATION.md target)
- `go test ./internal/install -run TestFirstRun_Gate -race` → exit 0 (VALIDATION.md target)
- `go test ./internal/install -run TestPostFinish_NoWizardAccess -race` → exit 0 (VALIDATION.md target)
- `go test ./internal/... -short -race` → 106 passed, 11 packages (no regressions)
- `go vet ./...` → exit 0
- `go build ./...` → exit 0

Acceptance criteria from PLAN.md verified:
- File `internal/install/state.go` exports `type State struct`, `type Store struct`, `func NewStore(*pgxpool.Pool) *Store`, methods `GetOrCreate`, `UpdateStep1`, `UpdateStep2`, `UpdateStep3`, `UpdateStep4`, `Delete` ✅
- `GetOrCreate` uses `INSERT ... ON CONFLICT (id) DO NOTHING` ✅
- Each `UpdateStepN` bumps `current_step` to GREATEST ✅
- File `internal/install/regions.go` exports `func Regions() []Region` and `func RegionByName(name string) (Region, bool)` ✅
- `Regions()` returns 8 entries including `name="as923_2"` with `DefaultForCountry="TH"` ✅
- File `internal/install/middleware.go` exports `func FirstRunGate(pool *pgxpool.Pool, log *slog.Logger) func(http.Handler) http.Handler` ✅
- File contains `var adminExistsCache atomic.Bool` set after positive `adminExists` ✅
- Whitelist includes `/install`, `/api/install/`, `/health`, `/assets/`, `/login`, common static suffixes ✅
- `/api/*` non-whitelisted paths return 409 + JSON `{"error":"install_required"}` ✅
- HTML traffic redirects to `/install` with 307 ✅

---
*Phase: 01-foundation*
*Plan: 14-install-middleware*
*Completed: 2026-04-28*
