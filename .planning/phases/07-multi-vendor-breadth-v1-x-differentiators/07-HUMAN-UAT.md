---
status: diagnosed
phase: 07-multi-vendor-breadth-v1-x-differentiators
source: [07-VERIFICATION.md]
started: 2026-05-13T01:12:06Z
updated: 2026-05-13T02:50:00Z
---

## Current Test

[testing paused — blocker gap diagnosed, awaiting gap closure plan]

## Tests

### 1. Vendor catalog import flow end-to-end
expected: Admin navigates to /settings → Vendor Catalog card, sees 4 vendor catalog entries with Status badges. Then /profiles → "Import from catalog" → 3-step dialog (RadioGroup → Command picker search → Review form) → "Add Profile" → "Profile added" toast → new profile appears in device profiles list.
result: issue
reported: "Vendor Catalog card shows 'No vendor profiles' empty state instead of 4 catalog rows. GET /api/catalog returns HTML+200 (SPA fallback) instead of JSON."
severity: blocker

### 2. Catalog update diff modal with per-field toggles
expected: Admin sees "Update to v..." button for a profile with available update, opens CatalogUpdateModal, sees per-field diff table with Use mine/Use catalog ToggleGroup toggles, applies update, sees "Profile updated to vX.Y.Z" toast.
result: blocked
blocked_by: prior-phase
reason: Blocked by same root cause as test 1 — POST /api/catalog/{id}/update is not mounted.

### 3. Codec test runner hex decode flow
expected: Admin opens device profile editor for a profile with codec_js set, expands "Test Codec" panel below the codec textarea, pastes an Axioma W1 hex payload, clicks "Run Test", sees Decoded JSON tab render decoded values, switches to Canonical Mapping tab.
result: blocked
blocked_by: prior-phase
reason: Blocked by same root cause as test 1 — POST /api/device-profiles/{id}/test-codec is not mounted.

### 4. Compare View entities mode
expected: Admin navigates to /compare via sidebar. Sees "Compare two entities" empty state. Selects "Compare entities" mode → both EntityDropdowns populate with metering points. Picks A and B → server returns CAGG-backed comparison, chart + delta table render.
result: blocked
blocked_by: prior-phase
reason: Blocked by same root cause as test 1 — POST /api/reports/compare is not mounted.

### 5. Doctor probe subcommands
expected: `docker exec compose-shifter-1 shifter doctor probe-timescale` outputs `[ok] ...` with TimescaleDB version. probe-chirpstack expected to return [warn]/[fail] (chirpstack restart loop). probe-region similar.
result: [pending]
note: Doctor probes are CLI subcommands, not HTTP routes — independent of the deps wiring gap. Will be tested after gap closure rebuild.

### 6. Bulk gateway import CSV flow
expected: Admin visits /gateways, sees "Import gateways" button, opens dialog, uploads CSV, validates row counts, commits import. Re-upload shows all rows skipped.
result: blocked
blocked_by: server
reason: Blocked by both (a) ChirpStack restart loop in local compose (compose template tech debt) AND (b) same root cause as test 1 — POST /api/gateways/bulk-import not mounted.

### 7. Saved report templates save and load
expected: Admin opens /reports, configures a report, clicks "Save as template", saves with a name → "Template saved" toast. Opens TemplatesDropdown, selects saved template → report config restored.
result: blocked
blocked_by: prior-phase
reason: Blocked by same root cause as test 1 — /api/reports/templates routes not mounted.

## Summary

total: 7
passed: 0
issues: 1
pending: 1
skipped: 0
blocked: 5

## Gaps

- truth: "All 6 Phase 7 HTTP endpoint families respond from the production binary"
  status: failed
  reason: "User reported: Vendor Catalog card shows 'No vendor profiles' empty state instead of 4 catalog rows. GET /api/catalog returns HTML+200 (SPA fallback) instead of JSON."
  severity: blocker
  test: 1
  root_cause: "Production wiring gap in internal/cli/serve.go — the httpapi.Deps{} literal around line 578 omits all 6 Phase 7 deps fields: CatalogDeps (07-04), CodecTestDeps (07-07), BacktestDeps (07-10), ReportTemplateDeps (07-11a), CompareDeps (07-12), GatewayImportDeps (07-13). The fields are declared in internal/http/router.go (lines 197-236) with nil-guards. With the production deps left nil, the nil-guards silently skip route mounting and requests fall through to the SPA fallback handler, returning HTML+200 instead of the expected JSON / 401 / 405. Unit tests pass because each Phase 7 plan constructed its own minimal Deps in handler_test.go files that exercise the handler directly without going through NewRouter. Plan-level verifier ran route-mount grep on router.go and observed mount logic exists — never followed through to serve.go to verify the deps fields are actually populated."
  artifacts:
    - path: "internal/cli/serve.go"
      issue: "httpapi.Deps{} literal at line 578-657 missing CatalogDeps, CodecTestDeps, BacktestDeps, ReportTemplateDeps, CompareDeps, GatewayImportDeps"
    - path: "internal/http/router.go"
      issue: "Deps struct lines 197-236 — fields exist but their constructors are not called from serve.go"
  missing:
    - "Wire CatalogDeps: NewCatalogDeps(pool, q, embed.FS) — needs access to internal/codec.LoadAll() output (catalog entries) plus pool for catalog metadata queries"
    - "Wire CodecTestDeps: needs pool + sessionMgr + getProfileForCodecTest query + rate limiter (30/min per-user, see 07-07-SUMMARY)"
    - "Wire BacktestDeps: needs pool + sessionMgr + alert.Rules + measurement_hourly CAGG access"
    - "Wire ReportTemplateDeps: needs report.TemplateStore + sessionMgr + auditStore (audit-in-tx semantics)"
    - "Wire CompareDeps: needs pool + sessionMgr (CompareSiteDaily / CompareMeteringPointDaily already in queries.sql.go)"
    - "Wire GatewayImportDeps: needs gateway.NewImportService + sessionMgr + auditStore + chirpstack gateway client (will be nil/disabled when chirpstack is down, but the import service must accept that gracefully)"
    - "After wiring, rebuild docker image (just _compose-build-image) and restart shifter container — confirm all 6 endpoints return non-200 (401 for unauthed, 405 for wrong method) instead of 200+HTML"
  debug_session: ""
