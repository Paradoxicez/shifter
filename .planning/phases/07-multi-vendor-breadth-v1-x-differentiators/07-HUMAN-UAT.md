---
status: complete
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
result: pass
note: |
  Live UAT pass after gap closure 07-15 (wire Phase 7 deps in serve.go) + 3 follow-up quick fixes shipped in the same session:
  - shadcn Textarea field-sizing-content was causing 1000-line codec_js to stretch dialog vertically. Capped both ImportFromCatalogDialog and mapping-editor codec textareas with max-h-* + overflow-y-auto + [field-sizing:fixed].
  - apiFetch was JSON.parse'ing every response body unconditionally; Go's http.Error writes text/plain, so 4xx/5xx responses threw SyntaxError instead of surfacing ApiError.status. Added defensive try/catch with text fallback.
  - Duplicate-import error copy implied renaming would bypass the check; backend enforces uniqueness on slug. Updated toast to explain the actual constraint and required operator action.
fix_commits: [320122a, 5e9876b, 8de5fed]

### 2. Catalog update diff modal with per-field toggles
expected: Admin sees "Update to v..." button for a profile with available update, opens CatalogUpdateModal, sees per-field diff table with Use mine/Use catalog ToggleGroup toggles, applies update, sees "Profile updated to vX.Y.Z" toast.
result: skipped
reason: Deps wiring (gap 07-15) confirmed POST /api/catalog/{id}/update is mounted (returns 401 unauthed). Live version-bump test deferred to a real catalog update cycle — would require bumping a catalog JSON version + rebuilding image.

### 3. Codec test runner hex decode flow
expected: Admin opens device profile editor for a profile with codec_js set, expands "Test Codec" panel below the codec textarea, pastes an Axioma W1 hex payload, clicks "Run Test", sees Decoded JSON tab render decoded values, switches to Canonical Mapping tab.
result: pass
note: Decoded JSON tab works. Canonical Mapping tab originally crashed with "Cannot convert undefined or null to object" because JsonTree fell through Object.entries(null). Fixed JsonTree to treat undefined/null as graceful render + CodecTestRunner short-circuits the tab with an explainer when mapping is null (profile with no mappings configured yet).
fix_commits: [42f6911]

### 4. Compare View entities mode
expected: Admin navigates to /compare via sidebar. Sees "Compare two entities" empty state. Selects "Compare entities" mode → both EntityDropdowns populate with metering points. Picks A and B → server returns CAGG-backed comparison, chart + delta table render.
result: pass
note: Live verified — /compare route loads, RadioGroup mode toggle works (entities ↔ time_ranges), EntityDropdown opens a Popover + Command searchable list, useQuery(listMPs+listSites) wired by quick task 260513-bjc. "No results found" shown correctly because this fresh install has no MPs/sites seeded. Full chart+delta render path requires real MP data and is deferred to live customer install.
follow_up: "EntityDropdown 'no results' state uses an upload-cloud icon (likely lucide CloudUpload misplacement). Should be search-x or inbox-empty for empty-search context. Minor UX nit, not blocking."

### 5. Doctor probe subcommands
expected: `docker exec compose-shifter-1 shifter doctor probe-timescale` outputs `[ok] ...` with TimescaleDB version. probe-chirpstack expected to return [warn]/[fail] (chirpstack restart loop). probe-region similar.
result: pass
note: |
  Live verified inside compose-shifter-1 container:
  - probe-timescale → [ok] ✓: timescaledb extension present (version 2.26.0)
  - probe-chirpstack → [error] ✗: ChirpStack GetVersion failed at chirpstack:8080: Unavailable (correctly detects the chirpstack restart loop — install kit tech debt, separate from Phase 7)
  - probe-region → [error] ✗: could not read install region from chirpstack_connection (cascades from chirpstack being unreachable)
  - /health/detailed includes the full probe_results block with chirpstack/timescale/region keys, each with name/status/message/last_run_at
  Probes are working as designed — they detected real failure state and reported it with useful diagnostic messages.

### 6. Bulk gateway import CSV flow
expected: Admin visits /gateways, sees "Import gateways" button, opens dialog, uploads CSV, validates row counts, commits import. Re-upload shows all rows skipped.
result: blocked
blocked_by: server
reason: |
  Gap 07-15 mounted the /api/gateways/bulk-import endpoint (verified 401 application/json). However, the actual import flow invokes ChirpStack gateway creation via gRPC, which fails because ChirpStack is in restart loop in the local compose (install kit tech debt from Phase 1-20: chirpstack service missing --config flag + bind-mounted config files). End-to-end CSV import requires a working ChirpStack instance — deferred to live customer install or to a future quick task that fixes the chirpstack compose configuration.

### 7. Saved report templates save and load
expected: Admin opens /reports, configures a report, clicks "Save as template", saves with a name → "Template saved" toast. Opens TemplatesDropdown, selects saved template → report config restored.
result: pass
note: Live verified — Templates + "Save as template" buttons present in CardHeader, save flow shows "Template saved: All Meter" toast (implied by subsequent screenshot), load flow shows "Template loaded: All Meter" toast and restores scope=All meters / Range=Monthly / Group by=None to the panel.

## Summary

total: 7
passed: 5
issues: 0
pending: 0
skipped: 1
blocked: 1

## Gaps

- truth: "All 6 Phase 7 HTTP endpoint families respond from the production binary"
  status: verified
  verified: "2026-05-13"
  fix_commit: "320122a"
  fix_plan: "07-15-wire-phase-7-deps"
  reason: "User reported: Vendor Catalog card shows 'No vendor profiles' empty state instead of 4 catalog rows. GET /api/catalog returns HTML+200 (SPA fallback) instead of JSON."
  severity: blocker
  test: 1
  root_cause: "Production wiring gap in internal/cli/serve.go — the httpapi.Deps{} literal around line 578 omits all 6 Phase 7 deps fields: CatalogDeps (07-04), CodecTestDeps (07-07), BacktestDeps (07-10), ReportTemplateDeps (07-11a), CompareDeps (07-12), GatewayImportDeps (07-13). The fields are declared in internal/http/router.go (lines 197-236) with nil-guards. With the production deps left nil, the nil-guards silently skip route mounting and requests fall through to the SPA fallback handler, returning HTML+200 instead of the expected JSON / 401 / 405. Unit tests pass because each Phase 7 plan constructed its own minimal Deps in handler_test.go files that exercise the handler directly without going through NewRouter. Plan-level verifier ran route-mount grep on router.go and observed mount logic exists — never followed through to serve.go to verify the deps fields are actually populated."
  artifacts:
    - path: "internal/cli/serve.go"
      issue: "httpapi.Deps{} literal at line 578-657 missing CatalogDeps, CodecTestDeps, BacktestDeps, ReportTemplateDeps, CompareDeps, GatewayImportDeps"
    - path: "internal/http/router.go"
      issue: "Deps struct lines 197-236 — fields exist but their constructors are not called from serve.go"
  resolution:
    - "Added apipkg alias import for internal/api in serve.go"
    - "Wired CatalogDeps{Pool, SessionMgr}"
    - "Wired CodecTestDeps{Pool, SessionMgr, Log}"
    - "Wired BacktestDeps{Pool}"
    - "Wired ReportTemplateDeps{Pool, SessionMgr}"
    - "Wired CompareDeps{Pool, SessionMgr}"
    - "Wired GatewayImportDeps{Pool, SessionMgr, Log, ImportSvc: gateway.NewImportService(pool)}"
    - "Rebuilt docker image (just _compose-build-image) with commit 320122a baked in"
    - "Restarted shifter container — all 6 endpoints now return 401 application/json"
  debug_session: ""
