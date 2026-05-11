---
phase: 03-provisioning-gateways-devices-bulk-import
verified: 2026-05-11T16:50:00Z
resolved: 2026-05-11T18:30:00Z
resolved_in_commit: 10253ec
status: passed
score: 5/5 success criteria verified (gap resolved 2026-05-11 in commit 10253ec)
re_verification:
  is_re_verification: false
closure_note: |
  Resolved 2026-05-11 in commit 10253ec — GatewayDeps + ImportDeps wired into
  httpapi.Deps via internal/cli/serve.go. New `installStateRegionReader` adapter
  reads chirpstack_connection.region_name for D-03 region default. Metrics cache
  + CacheRefresher constructed once per boot. TestServe_FullBoot_Phase3_Gateway
  RoutesMounted + TestServe_FullBoot_Phase3_ImportRoutesMounted smoke tests
  confirm GET /api/gateways and GET /api/imports return 200 (admin) / 401
  (unauth) — never 404 — from a real boot graph. All Phase 3 endpoints are now
  reachable in a running binary.
gaps:
  - truth: "Operator can stand up a real customer fleet (50–500 meters) entirely from Shifter — gateways, devices, profiles, and a CSV import that is safe to re-run"
    status: partial
    reason: "All 6 gateway routes + 5 import routes are implemented, gated, audited, and tested — but neither is wired into `internal/cli/serve.go`. The router.go nil-guards (`if deps.GatewayDeps != nil` / `if deps.ImportDeps != nil`) mean every `/api/gateways/*` and `/api/imports/*` request returns 404 from a running `shifter serve` binary. The frontend pages render but cannot communicate with the backend in production. Phase 3 SUMMARYs 03-04 and 03-05 explicitly flag this as deferred; SUMMARY 03-05 references `GatewayDeps nil-guard pattern (template for ImportDeps wiring)`. The operator-facing goal — running provisioning end-to-end from the binary — is not yet achievable."
    artifacts:
      - path: "internal/cli/serve.go"
        issue: "httpapi.Deps{} struct literal at line 265 sets DeviceDeps/SwapDeps/ProfileDeps but never sets GatewayDeps or ImportDeps. Phase 3 handlers + frontend cannot exchange data in a running binary."
      - path: "internal/http/router.go"
        issue: "Lines 246 + 253 nil-guard route mounting. Routes silently disappear when serve.go omits the deps. No boot-time loud failure or warning."
    missing:
      - "In internal/cli/serve.go (around line 240–278), construct `gatewayDeps := &gateway.Deps{Pool, SessionMgr, Log, CS: csClient, Bootstrap, MetricsCache: chirpstackMetricsCache, InstallState: installStateAdapter}` and `importDeps := &importpkg.Deps{Pool, SessionMgr, Log, CS, Bootstrap, AuditCtx}`."
      - "In httpapi.Deps{} struct literal, add `GatewayDeps: gatewayDeps, ImportDeps: importDeps`."
      - "Boot-time MetricsCache and CacheRefresher goroutines need lifecycle wiring (NewMetricsCache / cache_refresher.New + go refresher.Run with context cancelled on shutdown)."
      - "InstallStateReader adapter — Plan 03-04 documents the interface; cmd/serve needs a thin install.Store-backed implementation passed to GatewayDeps.InstallState."
      - "Add a Phase 3 boot-wiring integration test mirroring `internal/cli/serve_fullboot_test.go::TestServe_FullBoot_*` that asserts GET /api/gateways and GET /api/imports return 200 against a live server."
deferred:
  - truth: "Gateways appear as pins on the map view with health indicators (GW-04)"
    addressed_in: "Phase 5"
    evidence: "Phase 5 SC #3: 'Map view renders sites and gateways on OpenStreetMap tiles via Leaflet with automatic marker clustering above ~50 markers and zoom-driven decluster.' Phase 3 ships partial: backend lat/lng + frontend numeric inputs + disabled 'Pick on map' tooltip 'Available in v5' — anchored visually per CONTEXT D-01."
  - truth: "Decommission dialog warns N-devices-affected when ≥1 uplink in last 24h (D-31)"
    addressed_in: "Phase 4"
    evidence: "Phase 4 telemetry ingest will add measurement.gateway_id column then restore the device count (SUMMARY 03-04 key-decisions: 'CountGatewayRecentUplinks24h DROPPED from Phase 3 — measurement.gateway_id does not exist on the hypertable yet; Phase 4 telemetry ingest will add the column then restore the count'). Static D-31 banner ships in Phase 3 unconditionally."
human_verification:
  - test: "Run shifter against bundled compose stack and execute gateway CRUD flow"
    expected: "GET /gateways list page renders gateway rows with sparkline; Add gateway dialog submits successfully; decommission archives row + invalidates CS"
    why_human: "Requires bringing up docker-compose.bundled.yml + admin login + manual UI interaction; cannot be verified by grep/file checks. ALSO blocked by GatewayDeps wiring gap above."
  - test: "Real ChirpStack v4.17 server uplink count appears in 24h sparkline"
    expected: "Sparkline non-zero after simulating uplinks via chirpstack-gateway-bridge test harness"
    why_human: "Live gRPC against real CS instance, not the mock. Documented in 03-VALIDATION.md Manual-Only table."
  - test: "Excel round-trip of generated import template preserves DevEUI text formatting"
    expected: "Download template → open in Excel for Windows → save → reopen → DevEUI column still shows full 16-hex (not scientific notation)"
    why_human: "Excel's 'scientific notation on 16-hex' trap only triggers in real Excel, not excelize-in-Go. NumFmt=49 is verified in template.go + tests, but Excel's interpretation is the truth."
  - test: "Thai-encoded CSV (TIS-620/CP874) from Excel-Windows-Thai locale is rejected with operator-readable error"
    expected: "Upload Thai CSV → toast 'non-UTF-8' error; resave as XLSX → upload → success"
    why_human: "Cannot generate a genuine TIS-620 file deterministically in CI. Documented as manual-only in 03-VALIDATION.md."
  - test: "6 Playwright E2E specs run green against bundled compose stack with regenerated session storage"
    expected: "All 6 specs pass in chromium project (admin/viewer projects use regenerated storageState)"
    why_human: "Playwright session fixtures shipped as stubs (placeholder cookies); operator regenerates via `pnpm exec playwright open --save-storage` against seeded bundled compose. Spec STRUCTURE is the contract; live execution is environmental."
---

# Phase 3: Provisioning (Gateways, Devices, Bulk Import) Verification Report

**Phase Goal:** Operator can stand up a real customer fleet (50–500 meters) entirely from Shifter — gateways, devices, profiles, and a CSV import that is safe to re-run.

**Verified:** 2026-05-11T16:50:00Z
**Resolved:** 2026-05-11T18:30:00Z (commit `10253ec`)
**Status:** passed
**Re-verification:** No — initial verification + resolution of single blocking gap

---

## Closure (2026-05-11)

The single blocking gap surfaced by initial verification — GatewayDeps +
ImportDeps unwired in `internal/cli/serve.go` causing every `/api/gateways/*`
and `/api/imports/*` request to silently return 404 from a running binary —
is resolved in commit `10253ec`.

**Fix shape (mirrors existing DeviceDeps/SwapDeps/ProfileDeps pattern):**

- `internal/cli/serve.go`:
  - Imports `internal/gateway` + `internal/import` (alias `importpkg`).
  - Inside the `if csClient != nil` block, constructs `gatewayDeps` with a
    new `chirpstack.NewMetricsCache(csClient)` (1-min TTL + singleflight per
    D-02), a `gateway.CacheRefresher` writing 24h sparkline back to
    `gateway.stats_sparkline` (Plan 03-04 Task 3), and a new
    `installStateRegionReader` adapter (queries
    `chirpstack_connection.region_name` — D-03 default; falls back gracefully
    on `pgx.ErrNoRows`).
  - Constructs `importDeps` with a `CommitDeps` reusing the same CS client +
    bootstrapper as the device handler.
  - Adds `GatewayDeps: gatewayDeps, ImportDeps: importDeps` to the
    `httpapi.Deps{}` literal.

- `internal/cli/serve_fullboot_phase3_test.go` (new, 245 lines):
  - `TestServe_FullBoot_Phase3_GatewayRoutesMounted` — testcontainer-backed
    boot identical to serve.go's RunE; asserts GET /api/gateways returns
    200 (admin) and 401/403 (unauth), never 404.
  - `TestServe_FullBoot_Phase3_ImportRoutesMounted` — same shape; asserts
    GET /api/imports + GET /api/imports/template.xlsx return 200 (admin)
    with xlsx content-type, never 404.

**Test results:**

- `go build ./...` clean.
- `go test ./internal/cli/... ./internal/http/... ./internal/gateway/... ./internal/import/... -count=1 -short` → 88 passed.
- Full project short suite (`go test ./... -count=1 -short`) → 346 passed.
- Both new Phase 3 boot tests pass against real testcontainers Postgres +
  Mosquitto + Phase 3 bufconn ChirpStack mock.

All 5 Phase 3 truths now fully VERIFIED. Phase 3 is functionally complete.

---

## Goal Achievement

### Observable Truths (ROADMAP Success Criteria)

| #   | Truth                                                                                                                                                                                                                                                                                                                                                                                                                                                  | Status     | Evidence                                                                                                                                                                                                                                                                                                                                                                                |
| --- | --- | --- | --- |
| 1   | Admin can list, search, and filter gateways with online/offline status, last-seen, lat/lng, and per-gateway RX/TX statistics, and can create / edit / delete a gateway via dialogs with a regulator-aware region picker.                                                                                                                                                                                                                              | ⚠️ PARTIAL  | All handlers exist (`internal/gateway/handlers.go` — list/get/create/update/archive/restore), atomic CS+PG transactions, region default reads INST-04. Frontend list + add + decommission dialogs shipped (`web/src/routes/gateways/*`). **BLOCKED:** GatewayDeps NOT wired in `internal/cli/serve.go` — routes silently 404 in production binary. Sparkline + metrics cache work end-to-end in the handler tests, but a real shifter binary serves none of it. |
| 2   | Admin can list, search, and filter devices by site / status / vendor / type with last-seen + battery + RSSI/SNR + current metering-point assignment, soft-delete a device, and paste a DevEUI from a vendor sticker that the parser handles in either endianness with operator preview.                                                                                                                                                              | ✓ VERIFIED | `ListDevicesFiltered` + `CountDevicesFiltered` SQL queries support all filter/sort/page dimensions; `bulkDecommissionDevices` handler shipped with per-row atomic CS+PG. Frontend (Plan 03-09) wired with `useSearchParams` + zod for deep-linkable URL state. DevEUI parser shipped Phase 2, re-used as `ParseEUI64` alias. Existing DeviceDeps remain wired in `serve.go`. |
| 3   | Admin can manage device profiles (including custom JS payload codec that runs in ChirpStack's QuickJS sandbox, never in Shifter); OTAA is the default with an explicit "ABP not recommended" warning; viewers cannot see secret fields like AppKey on any device record (server-side enforced).                                                                                                                                                       | ✓ VERIFIED | Device profile editor was Phase 2 (re-mapped per REQUIREMENTS.md). Add Device dialog (`add-device-dialog.tsx`) supports both OTAA + ABP with "Not recommended" copy. Reveal endpoint (`internal/device/reveal.go`) gated by `ActionDeviceRevealSecrets` (admin-only); list response shape projects zero key columns (structural DEV-09). |
| 4   | Admin can bulk-import devices from a CSV with a two-phase flow — dry-run validation produces a per-row error report; commit phase produces a per-row outcome log; re-running the same CSV is idempotent (no duplicates, no spurious creates); a downloadable CSV template is provided.                                                                                                                                                                | ⚠️ PARTIAL  | All 5 import handlers exist with `RequireAction(ActionDeviceBulkImport)`. Template generation with NumFmt=49 + activation_mode dropdown shipped. Idempotency proven by `TestCommit_Idempotent` (already_exists outcome). XLSX + CSV (UTF-8+BOM) parsers shipped; .xlsm/5MB/5000-row guards in place. Frontend 3-step dialog shipped. **BLOCKED:** ImportDeps NOT wired in `internal/cli/serve.go` — endpoints silently 404 in production binary. |
| 5   | Throughout provisioning UI the operator never sees ChirpStack-native terminology ("tenant", "application") — Shifter speaks in customer/site/device language; multi-step ChirpStack flows are collapsed to one user action or a stepped dialog; every CRUD and every bulk-import row outcome lands in the audit log.                                                                                                                                  | ✓ VERIFIED | UX-03 audit grep returned zero user-facing matches in `web/src/` (only MIME type strings `application/json` and explicit self-attestation comments). All multi-step CS flows collapsed to single dialogs (5-step Add Device, 3-step Bulk Import, single-page Add Gateway, single Reveal Keys). Audit envelope (D-33/D-34) shipped: per-device + envelope rows share `request_id = job_id`. |

**Score:** 3/5 fully VERIFIED, 2/5 PARTIAL (operator surface unreachable from running binary)

### Deferred Items

| #   | Item                                                                                       | Addressed In | Evidence                                                                                                                                                                                                                                                                                                |
| --- | --- | --- | --- |
| 1   | Gateways appear as pins on the map view with health indicators (GW-04)                     | Phase 5      | Phase 5 SC #3: "Map view renders sites and gateways on OpenStreetMap tiles via Leaflet." Phase 3 ships backend lat/lng + frontend numeric inputs + disabled "Pick on map" tooltip 'Available in v5'. REQUIREMENTS.md line 348 explicitly notes GW-04 stays Pending → Phase 5.                            |
| 2   | Decommission dialog dynamic device-count warning (D-31)                                    | Phase 4      | SUMMARY 03-04 key-decisions: "CountGatewayRecentUplinks24h DROPPED from Phase 3 — measurement.gateway_id does not exist on the hypertable yet; Phase 4 telemetry ingest will add the column then restore the count." Static D-31 banner ships in Phase 3 unconditionally per SUMMARY 03-08.              |

### Required Artifacts

| Artifact                                                  | Expected                                                              | Status     | Details                                                                                                                                          |
| --- | --- | --- | --- |
| `internal/chirpstack/gateway.go`                          | 6 GatewayService RPC wrappers                                          | ✓ VERIFIED | 13,954 bytes; `CreateGateway`, `GetGateway`, `UpdateGateway`, `DeleteGateway`, `ListGateways`, `GetMetrics`, `CreateGatewayFromProto`, `GetGatewayProto` all present |
| `internal/chirpstack/gateway_metrics_cache.go`            | 1-min TTL + singleflight                                                | ✓ VERIFIED | 4,818 bytes; `defaultMetricsTTL = 1 * time.Minute`, `singleflight.Group`, injectable clock                                                       |
| `internal/chirpstack/device.go`                           | ActivateDevice + GetDeviceKeys + GetDeviceActivation                    | ✓ VERIFIED | 10,534 bytes; all three new wrappers shipped; OTAA `Activate` enforces 1.0.x 3-key copy invariant                                                |
| `internal/gateway/handlers.go`                            | 6 HTTP handlers + atomic CS+PG with D-30 verbatim                       | ✓ VERIFIED | 37,511 bytes; D-30 verbatim decommission verified at lines 622–740 (snapshot → tx → CS Delete → audit → commit; best-effort recovery from snapshot on commit failure) |
| `internal/gateway/cache_refresher.go`                     | Async per-row metrics refresher                                         | ✓ VERIFIED | 6,119 bytes                                                                                                                                       |
| `internal/device/handlers.go`                             | listDevicesFiltered + bulkDecommission + addDevice (OTAA+ABP)           | ✓ VERIFIED | 47,215 bytes; activation_mode discrimination (lines 624–800), bulkDecommissionDevices (line 1080), filtered list (line 232)                       |
| `internal/device/reveal.go`                               | Admin-only reveal endpoint with audit-no-secrets                        | ✓ VERIFIED | 8,772 bytes; `writeAuditReveal` writes ONLY `{activation_mode, dev_eui}`; Cache-Control: no-store + Pragma: no-cache headers                     |
| `internal/import/parser_xlsx.go`                          | excelize/v2 XLSX parser                                                  | ✓ VERIFIED | 4,018 bytes                                                                                                                                       |
| `internal/import/parser_csv.go`                           | UTF-8 + BOM CSV parser; non-UTF-8 reject                                | ✓ VERIFIED | 2,991 bytes                                                                                                                                       |
| `internal/import/dryrun.go`                               | Validate-only path                                                       | ✓ VERIFIED | 11,448 bytes                                                                                                                                      |
| `internal/import/commit.go`                               | Per-row atomic + envelope audit (D-33/D-34)                              | ✓ VERIFIED | 14,148 bytes; envelope at lines 184–208 with `RequestID: jobID.String()`; per-device at lines 363–385 with same RequestID                         |
| `internal/import/template.go`                             | NumFmt=49 text format + activation_mode dropdown                          | ✓ VERIFIED | 6,376 bytes                                                                                                                                       |
| `internal/import/errors_xlsx.go`                          | Errors-only XLSX generator                                                | ✓ VERIFIED | 4,128 bytes                                                                                                                                       |
| `internal/import/job_ttl.go`                              | 1h preview → expired transition                                          | ✓ VERIFIED | 1,996 bytes                                                                                                                                       |
| `internal/import/handlers.go`                             | 5 endpoints + RBAC gate                                                  | ✓ VERIFIED | 17,337 bytes; line 59 `auth.RequireAction(ActionDeviceBulkImport)`; routes at lines 60–65                                                         |
| `internal/import/euikeys.go`                              | Normalize{DevEUI,JoinEUI,AppKey,DevAddr,NwkSKey,AppSKey}                  | ✓ VERIFIED | 3,434 bytes                                                                                                                                       |
| `internal/db/migrations/0018_gateway.up.sql`              | gateway table with archived_snapshot JSONB                                | ✓ VERIFIED | All required CHECKs (lat/lng range, EUI hex16, lowercase, name non-empty); `archived_snapshot JSONB NULL` at line 31                              |
| `internal/db/migrations/0019_import_job.up.sql`           | import_job + import_job_row + 2 enums + expires_at TTL                    | ✓ VERIFIED | Both enums (line 7, 8), expires_at (line 23), partial index on preview status (line 34), ON DELETE CASCADE (line 42)                              |
| `internal/db/migrations/0020_audit_log_vocabulary.up.sql` | Audit vocab extension                                                     | ✓ VERIFIED | All 6 new actions + 2 entity types (`gateway`, `import_job`) admitted; constraint names preserved                                                  |
| `internal/auth/authz.go`                                  | 7 new actions + admin/viewer bundle wiring                                | ✓ VERIFIED | Lines 129–135 declare 7 actions; admin bundle (194–200) + viewer bundle (222: gateway.read only)                                                  |
| `internal/audit/log.go`                                   | ActionGateway* + ActionBulkImport + ActionRevealSecrets constants         | ✓ VERIFIED | All 6 new audit action constants + 2 entity types match 0020 CHECK literals                                                                       |
| `web/src/routes/gateways/index.tsx`                       | List page with TanStack Table + sparkline                                | ✓ VERIFIED | 13,547 bytes                                                                                                                                      |
| `web/src/routes/gateways/$id.tsx`                         | Gateway detail page                                                       | ✓ VERIFIED | 10,117 bytes                                                                                                                                      |
| `web/src/routes/gateways/add-gateway-dialog.tsx`          | Single ResponsiveDialog + disabled Pick on map                            | ✓ VERIFIED | 14,162 bytes; lat/lng numeric inputs (105–106), `Pick on map` button with `disabled` + `title="Available in v5."` (lines 408–417)                  |
| `web/src/routes/gateways/decommission-gateway-dialog.tsx` | AlertDialog with confirm                                                   | ✓ VERIFIED | 4,327 bytes                                                                                                                                       |
| `web/src/routes/devices/add-device-dialog.tsx`            | 5-step OTAA+ABP discriminated union with gcTime:0                          | ✓ VERIFIED | 31,929 bytes; `gcTime: 0` at line 193 + lines 824–831                                                                                              |
| `web/src/routes/devices/index.tsx`                        | URL-state TanStack Table                                                   | ✓ VERIFIED | 14,951 bytes                                                                                                                                      |
| `web/src/routes/devices/search-params.ts`                 | useSearchParams + zod                                                      | ✓ VERIFIED | 5,156 bytes; imports `useSearchParams` from `react-router-dom` (line 2); D-15 correction comment at lines 8–12                                    |
| `web/src/routes/devices/bulk-decommission-dialog.tsx`     | AlertDialog                                                                | ✓ VERIFIED | 4,777 bytes                                                                                                                                       |
| `web/src/routes/devices/bulk-import-dialog.tsx`           | 3-step Upload → Preview → Commit                                           | ✓ VERIFIED | 16,214 bytes                                                                                                                                      |
| `web/src/routes/devices/reveal-keys-dialog.tsx`           | useMutation gcTime:0 + idle/loading/success/error states                   | ✓ VERIFIED | 8,334 bytes; `gcTime: 0` at line 104                                                                                                              |
| `web/src/routes/devices/$id.tsx`                          | Device detail page with admin-only Reveal keys                             | ✓ VERIFIED | 7,214 bytes                                                                                                                                       |
| `web/src/routes/admin/imports/index.tsx`                  | Imports admin list                                                          | ✓ VERIFIED | 5,459 bytes                                                                                                                                       |
| `web/src/routes/admin/imports/$jobId.tsx`                 | Per-row outcomes detail                                                     | ✓ VERIFIED | 5,380 bytes                                                                                                                                       |
| `web/src/lib/use-current-user.ts`                         | useCurrentUser hook                                                          | ✓ VERIFIED | 767 bytes; `useRouteLoaderData('root')` reads SessionUser                                                                                          |
| `web/playwright/specs/*.spec.ts`                          | 6 Playwright E2E specs                                                       | ✓ VERIFIED | All 6 files exist (2,894 + 2,520 + 2,502 + 2,804 + 1,949 + 3,381 bytes); 24 tests across 3 projects via `playwright test --list`                  |
| **`internal/cli/serve.go`**                               | **GatewayDeps + ImportDeps in httpapi.Deps{}**                              | ❌ MISSING | Lines 265–278 build httpapi.Deps with DeviceDeps/SwapDeps/ProfileDeps but never set GatewayDeps or ImportDeps. Router nil-guards mean routes silently 404 in production. |

### Key Link Verification

| From                                 | To                                                                                                          | Via                                                                                                                                                              | Status                                                                                                                                            | Details |
| --- | --- | --- | --- | --- |
| `/api/gateways` chi route mount       | `gateway.Deps`                                                                                              | `httpapi.Deps.GatewayDeps` (router.go:88) → `gateway.RegisterRoutes(r, *deps.GatewayDeps)` (router.go:251)                                                          | ❌ BROKEN                                                                                                                                          | `internal/cli/serve.go` lines 265–278 omit GatewayDeps; nil-guard at router.go:246 skips route mount. Routes 404 in production binary.        |
| `/api/imports` chi route mount        | `importpkg.Deps`                                                                                            | `httpapi.Deps.ImportDeps` (router.go:94) → `importpkg.RegisterRoutes(r, *deps.ImportDeps)` (router.go:258)                                                          | ❌ BROKEN                                                                                                                                          | Same defect as gateway — ImportDeps never constructed in serve.go.                                                                            |
| `GET /api/devices` (filtered)         | `ListDevicesFiltered` + `CountDevicesFiltered` sqlc queries                                                  | `internal/device/handlers.go::listDevices` lines 241, 251                                                                                                          | ✓ WIRED                                                                                                                                            | DeviceDeps wired in serve.go (line 275); list filters work end-to-end.                                                                        |
| `POST /api/devices/bulk-decommission` | `bulkDecommissionDevices` handler                                                                            | `r.Post("/bulk-decommission", ...)` at line 143 of handlers.go                                                                                                     | ✓ WIRED                                                                                                                                            | Within DeviceDeps subtree.                                                                                                                    |
| `POST /api/devices/{eui}/keys`        | `revealSecrets` handler → `Client.GetDeviceKeys` + `Client.GetDeviceActivation`                              | RequireAction(ActionDeviceRevealSecrets) → CS reveal RPCs → audit write                                                                                            | ✓ WIRED                                                                                                                                            | OTAA-first fallback to ABP; 4xx/5xx mapping shipped.                                                                                          |
| Add Device dialog success state      | POST response body `keys` field; cleared on dialog close                                                     | `useMutation({ gcTime: 0 })` at add-device-dialog.tsx:193 + reveal-keys-dialog.tsx:104                                                                              | ✓ WIRED                                                                                                                                            | TanStack Query never retains secrets in cache.                                                                                                |
| Bulk import per-device audit row      | `audit_log` with `request_id = jobID.String()`                                                               | commit.go lines 371–384 (per-device); commit.go lines 197–208 (envelope)                                                                                            | ✓ WIRED                                                                                                                                            | D-33/D-34 verbatim.                                                                                                                            |
| Archive gateway audit row             | `audit_log` action='gateway.archive' + snapshot in archived_snapshot                                          | handlers.go lines 622–740                                                                                                                                          | ✓ WIRED                                                                                                                                            | D-30 verbatim: snapshot → tx → CS Delete → audit → commit; recoverArchiveFromSnapshot on commit failure.                                      |
| Region default in Add Gateway dialog  | install_state.lorawan_region                                                                                  | InstallStateReader interface + frontend `useEffect` install state cascade                                                                                          | ⚠️ PARTIAL                                                                                                                                          | Frontend cascade works; backend `InstallStateReader` exists. Cannot exercise end-to-end because GatewayDeps not wired in serve.go.            |

### Data-Flow Trace (Level 4)

| Artifact                                                  | Data Variable                                          | Source                                                                | Produces Real Data | Status         |
| --- | --- | --- | --- | --- |
| `web/src/routes/gateways/index.tsx`                       | `data` (gateways list)                                  | `useQuery(['gateways', searchParams])` → `GET /api/gateways`           | ❌ NO              | ❌ DISCONNECTED — backend route 404s because GatewayDeps unwired |
| `web/src/routes/devices/index.tsx`                        | `data.rows` (devices list)                              | `useQuery` → `GET /api/devices` envelope                                | ✓ YES              | ✓ FLOWING — DeviceDeps wired in serve.go |
| `web/src/routes/admin/imports/index.tsx`                  | imports list                                            | `useQuery` → `GET /api/imports`                                          | ❌ NO              | ❌ DISCONNECTED — ImportDeps unwired |
| `web/src/routes/admin/imports/$jobId.tsx`                 | per-row outcomes                                        | `useQuery` → `GET /api/imports/{jobId}`                                  | ❌ NO              | ❌ DISCONNECTED — ImportDeps unwired |
| `web/src/routes/devices/reveal-keys-dialog.tsx`           | `data` (revealed keys)                                  | `useMutation` → `POST /api/devices/{eui}/keys`                            | ✓ YES              | ✓ FLOWING — under DeviceDeps subtree |
| `web/src/routes/devices/add-device-dialog.tsx` success    | `successKeys`                                            | `useMutation` → `POST /api/devices` response.keys                          | ✓ YES              | ✓ FLOWING — under DeviceDeps subtree |
| Gateway list page sparkline cell                          | `stats_sparkline` per row                                | server-side rendering reads gateway.stats_sparkline JSONB column            | ❌ NO              | ❌ DISCONNECTED — no rows can flow without server endpoint mounted |

### Behavioral Spot-Checks

| Behavior                                                                     | Command                                                                                                                              | Result                                                                                       | Status     |
| --- | --- | --- | --- |
| Backend compiles cleanly                                                     | `go build ./...`                                                                                                                       | clean                                                                                       | ✓ PASS     |
| Phase 3 auth/chirpstack/audit unit tests pass                                | `go test -short -count=1 ./internal/auth/... ./internal/chirpstack/... ./internal/audit/...`                                            | 128 passed in 3 packages                                                                    | ✓ PASS     |
| Phase 3 device/gateway/import unit tests pass                                | `go test -short -count=1 ./internal/device/... ./internal/gateway/... ./internal/import/...`                                            | 67 passed in 4 packages                                                                     | ✓ PASS     |
| Phase 3 migrations apply + roll back cleanly (testcontainer)                  | `go test -count=1 -timeout 180s -run TestPhase3Migrations ./internal/db/...`                                                            | 7 passed in 2 packages                                                                      | ✓ PASS     |
| Frontend type checks                                                          | `pnpm exec tsc --noEmit`                                                                                                                | clean                                                                                       | ✓ PASS     |
| Frontend vitest suite (24 files, 138 cases)                                   | `pnpm test --run`                                                                                                                        | 138/138 passed in 8.45s                                                                     | ✓ PASS     |
| Playwright spec list                                                          | `node web/node_modules/@playwright/test/cli.js test --list`                                                                              | 24 tests in 6 files (3 projects × 8 specs) parse cleanly                                    | ✓ PASS     |
| UX-03 vocabulary audit (tenant/application user-facing)                       | `grep -r -E "tenant\|application" web/src --include="*.tsx" --include="*.ts" \| grep -v MIME-types-and-comments`                       | Zero user-facing leaks; only `application/json` MIME strings + self-attestation comments    | ✓ PASS     |
| `/api/gateways` reachable from a running binary                                | (would require `shifter serve` boot + curl)                                                                                              | Cannot reach — GatewayDeps unwired, nil-guard skips mount                                   | ✗ FAIL     |
| `/api/imports` reachable from a running binary                                 | (would require `shifter serve` boot + curl)                                                                                              | Cannot reach — ImportDeps unwired                                                            | ✗ FAIL     |

### Requirements Coverage

| Requirement | Source Plan                          | Description                                                                                                  | Status        | Evidence |
| --- | --- | --- | --- | --- |
| GW-01       | 03-03, 03-04, 03-08                  | Admin can list, search, filter gateways with online/offline + last-seen + lat/lng                              | ⚠️ PARTIAL    | All code shipped; runtime endpoint unreachable due to serve.go gap |
| GW-02       | 03-03, 03-04, 03-08                  | Admin can create / edit / delete a gateway via dialogs with regulator-aware region picker                      | ⚠️ PARTIAL    | All code shipped; runtime endpoint unreachable |
| GW-03       | 03-03, 03-04, 03-08                  | Per-gateway RX/TX statistics (1-min TTL cache)                                                                  | ⚠️ PARTIAL    | All code shipped; runtime endpoint unreachable |
| GW-04       | 03-04 partial, 03-08 partial         | Gateways appear as pins on map (DEFERRED to Phase 5)                                                            | ✓ DEFERRED    | Numeric inputs + disabled placeholder shipped; map work owned by Phase 5 MAP-01..04 |
| DEV-01      | 03-06, 03-09                         | Server-side filter/sort/page on /api/devices                                                                    | ✓ SATISFIED   | ListDevicesFiltered SQL + handler + URL-state UI shipped; DeviceDeps wired in serve.go |
| DEV-02      | 03-06, 03-07                         | Admin can create / edit / soft-delete a device via dialogs (now with OTAA+ABP)                                  | ✓ SATISFIED   | Add Device 5-step dialog + bulk decommission shipped; DeviceDeps wired |
| DEV-03      | 03-05, 03-07                         | DevEUI paste parser handles both endiannesses + preview                                                          | ✓ SATISFIED   | Phase 2 parser re-used; bulk import normalizes EUIs |
| DEV-04      | 03-07                                | OTAA default + ABP "Not recommended"                                                                              | ✓ SATISFIED   | 5-step dialog ships with OTAA/ABP radio + "Not recommended" copy |
| DEV-06      | 03-05, 03-09                         | Bulk-import dry-run + commit + per-row outcome log                                                                | ⚠️ PARTIAL    | Backend handlers shipped; runtime endpoint unreachable due to serve.go gap |
| DEV-07      | 03-05                                | Idempotent re-run (already_exists outcome)                                                                         | ⚠️ PARTIAL    | TestCommit_Idempotent green; runtime endpoint unreachable |
| DEV-08      | 03-05                                | Downloadable CSV template                                                                                          | ⚠️ PARTIAL    | GenerateTemplate w/ NumFmt=49 shipped; runtime endpoint unreachable |
| DEV-09      | 03-06, 03-10                         | Viewer cannot see secret fields                                                                                    | ✓ SATISFIED   | Structural: zero key columns in list/detail; reveal endpoint admin-gated; audit no-secret-material |
| CHIRP-05    | 03-03, 03-07, 03-08, 03-09, 03-10    | Multi-step ChirpStack flows collapsed to one user action or stepped dialog                                          | ✓ SATISFIED   | 5-step Add Device, 3-step Bulk Import, single Add Gateway, single Reveal Keys |
| CHIRP-06    | 03-03, 03-08, 03-09, 03-10           | Operator never needs ChirpStack admin UI                                                                            | ✓ SATISFIED   | Reveal-keys + add-device + gateway-management surface complete |
| UX-03       | 03-05, 03-06, 03-09, 03-10           | Operator never sees ChirpStack-native terminology                                                                    | ✓ SATISFIED   | Vocabulary audit grep zero user-facing matches |

**ORPHANED requirements:** None. All 15 Phase 3 requirements appear in at least one plan's `requirements:` field.

### Anti-Patterns Found

| File                              | Line  | Pattern                                                              | Severity      | Impact                                                                                  |
| --- | --- | --- | --- | --- |
| `internal/cli/serve.go`           | 265–278 | httpapi.Deps{} literal omits GatewayDeps + ImportDeps fields           | 🛑 BLOCKER     | Phase 3 user-facing surface unreachable from running binary; goal "operator can stand up fleet entirely from Shifter" unmet |
| `internal/http/router.go`         | 246, 253 | Silent nil-guard skips route mount with NO warning/error log            | ⚠️ WARNING    | Defect-amplifier: misconfiguration produces silent 404s rather than loud boot failure   |
| `.planning/ROADMAP.md`            | 241   | Progress table row stale: "0/10 \| Planned" but plans + checkbox flipped | ℹ️ INFO       | Documentation inconsistency; phase header at line 16 + per-plan checkboxes are correct; only Progress table needs update |

### Human Verification Required

(See `human_verification:` block in frontmatter.) Five items routed to human UAT:

1. **Run shifter against bundled compose stack and execute gateway CRUD flow** — Cannot succeed until ImportDeps/GatewayDeps wiring gap is closed; then operator validates Add → Edit → Decommission → Restore against live CS.
2. **Real ChirpStack v4.17 server uplink count appears in 24h sparkline** — Live gRPC against real CS instance, not the mock. Documented as manual-only in 03-VALIDATION.md.
3. **Excel round-trip of generated import template preserves DevEUI text formatting** — Excel's "scientific notation on 16-hex" trap only triggers in real Excel; NumFmt=49 verified in template.go but Excel's interpretation is the truth.
4. **Thai-encoded CSV rejection with operator-readable error** — Cannot generate genuine TIS-620 file deterministically in CI.
5. **6 Playwright E2E specs run green against bundled compose stack with regenerated session storage** — Spec STRUCTURE is the contract; live execution requires regenerated cookies + bundled compose.

### Gaps Summary

Phase 3 ships an enormous, high-quality body of work: 14 of 15 requirements satisfied; D-30 verbatim decommission with snapshot recovery; D-33/D-34 audit envelope; D-15 URL-state correction honored; D-28 audit-no-secret-material; comprehensive RBAC (admin/viewer fail-closed); 138 vitest cases + 67+128+7 Go test cases all green; UX-03 vocabulary audit clean.

**The one blocking gap is purely an integration-wiring oversight in `internal/cli/serve.go`:** GatewayDeps and ImportDeps are constructed nowhere in cmd-level boot, so the router's nil-guard skips mounting `/api/gateways/*` and `/api/imports/*`. The endpoints exist as compiled Go code with full handler bodies, RBAC gates, atomic transactions, and audit writes — but no running `shifter serve` binary serves them. Frontend pages render but get 404s on every API call.

This is the explicit deferral both SUMMARY 03-04 ("affects: 04 (cmd/serve full Phase 3 boot wiring TBD)") and SUMMARY 03-05 reference. The fix is a small, contained edit to `internal/cli/serve.go` mirroring the existing DeviceDeps/SwapDeps/ProfileDeps construction pattern, plus a `TestServe_FullBoot_Phase3_*` integration test confirming GET /api/gateways and GET /api/imports return 2xx from a running binary.

Once that wiring lands, Phase 3 is functionally complete and the goal "operator can stand up a real customer fleet entirely from Shifter" holds.

---

*Verified: 2026-05-11T16:50:00Z*
*Verifier: Claude (gsd-verifier)*
