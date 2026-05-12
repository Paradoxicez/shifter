---
phase: 05-aggregates-reports-map-floor-plans
plan: "06"
subsystem: report
tags: [pdf, river, background-jobs, download, status-polling, cleanup]
dependency_graph:
  requires: [05-03]
  provides: [pdf_report_job, GET /api/reports/{id}, GET /api/reports/{id}/file/{kind}]
  affects: [05-09, internal/cli/serve.go, internal/http/router.go]
tech_stack:
  added:
    - "github.com/riverqueue/river v0.36.0 — promoted from indirect to active usage"
    - "github.com/riverqueue/river/riverdriver/riverpgxv5 — promoted from indirect"
  patterns:
    - "River InsertTx inside pgx.Tx for D-23 atomicity (report INSERT + audit + PDF job in one tx)"
    - "InstallIdentityProvider interface loaded at worker runtime (not embed at construction)"
    - "UUID-before-path-construction pattern (T-05-06-02 path-traversal defense)"
    - "404-not-403 existence concealment for cross-user report queries (T-05-06-04)"
key_files:
  created:
    - internal/report/pdf_worker.go
    - internal/report/cleanup.go
    - internal/report/download_handler.go
  modified:
    - internal/report/handlers.go
    - internal/report/handlers_test.go
    - internal/report/pdf_worker_test.go
    - internal/cli/serve.go
    - internal/config/config.go
    - internal/http/router.go
    - compose/bundled.yml
    - compose/external.yml
decisions:
  - "sqlcIdentityProvider loads install_identity at PDF worker runtime (not embedded at serve boot) so identity changes are reflected without restart"
  - "reports_cache volume mounted at /var/lib/shifter/reports in both compose flavors; default config via SHIFTER_REPORTS_ROOT env var"
  - "River PeriodicInterval(1*time.Hour) cleanup job RunOnStart=false to avoid artifact purge on every restart"
  - "PDFReportWorker and CleanupExpiredReportsWorker registered in cmd/serve (internal/cli/serve.go) — not cmd/serve/serve.go which does not exist in this project structure"
  - "ReportDeps nil-guarded in router.go following the established DeviceDeps/GatewayDeps pattern"
metrics:
  duration: "~25min (continuation of interrupted session)"
  completed_date: "2026-05-12"
  tasks_completed: 2
  files_modified: 11
---

# Phase 05 Plan 06: Reports PDF + River Worker Summary

**One-liner:** Async PDF pipeline with River background worker, hourly cleanup PeriodicJob, auth-gated download + status endpoints, and `reports_cache` volume in both compose flavors.

## What Was Built

### Task 1 (committed 276fb0e — previous session)
maroto v2 PDF writer (`internal/report/pdf.go`) with:
- `WritePDF(rpt *Report, identity InstallIdentity) ([]byte, error)`
- Branded header: logo (when file exists) + display_name + address; navy rule separator
- Footer: `Generated <ISO ts> <tz> — page N of M` using maroto's native `{number}/{total}` placeholders
- `generation.LowMemory` mode activated when `len(PeriodRows)+len(MeterRows) > 50`
- Per-period breakdown table + per-meter rollup table
- All 3 PDF branding tests pass (TestPDFBranding, TestPDFBranding_NoLogo_StillRenders, TestPDFBranding_MultiPage)

**Maroto page-numbering strategy:** Native `{number}/{total}` placeholders work correctly with maroto v2 — no two-pass fallback needed. The `buildConfig` function passes `WithPageNumber("{number} of {total}", props.PageNumber{Place: props.Bottom})`. Tests confirm no unsubstituted braces remain in the output bytes.

### Task 2 (committed 16b9f9a — this session)

**`internal/report/pdf_worker.go`** — `PDFReportArgs + PDFReportWorker`:
- `PDFReportArgs.Kind()` returns `"pdf_report"`
- `Work()`: marks running → loads report row → loads identity via `InstallIdentityProvider` → calls `BuildReport` → `WritePDF` → writes `<artifact_dir>/report.pdf` → updates `pdf_status='ready'` and `pdf_path`
- `InstallIdentityProvider` interface loaded at runtime (not embedded at construction)

**`internal/report/cleanup.go`** — `CleanupExpiredReportsArgs + CleanupExpiredReportsWorker`:
- `Kind()` returns `"report_cleanup"`
- `Work()`: scans `ListExpiredReports`, calls `os.RemoveAll` per artifact dir, then `MarkReportExpired`; errors per-row are logged but do not abort the cycle

**`internal/report/download_handler.go`** — `DownloadHandler`:
- UUID parsed via `uuid.Parse` BEFORE any `filepath.Join` call (T-05-06-02)
- Kind validated against `{csv, xlsx, pdf}` allowlist
- 425 Too Early when `kind=pdf` and `pdf_status != 'ready'`
- 404 for expired reports; 404 (not 403) for other-user reports (T-05-06-04)

**`internal/report/handlers.go`** additions:
- `StatusResponse` struct matching plan 05-09's `ReportStatus` shape (id, pdf_status, pdf_path?, created_at, scope, range)
- `StatusHandler` — GET /api/reports/{id}: UUID validated before DB lookup; viewers see own, admins see all; 404 conceals existence
- `RegisterRoutes` wires all three: POST /api/reports/generate, GET /api/reports/{id}, GET /api/reports/{id}/file/{kind}

**`internal/cli/serve.go`** River bootstrap:
- `river.NewWorkers()` → `river.AddWorker` for both `PDFReportWorker` and `CleanupExpiredReportsWorker`
- `river.NewPeriodicJob(river.PeriodicInterval(1*time.Hour), ...)` for cleanup
- `river.NewClient(riverpgxv5.New(pool), &river.Config{Queues: {QueueDefault: {MaxWorkers: 4}}})` 
- River client started in goroutine; 30s graceful stop on context cancel
- `EnqueuePDF` closure calls `riverClient.InsertTx(ctx, tx, PDFReportArgs{ReportID}, nil)`
- `sqlcIdentityProvider` added: loads `install_identity` row from Postgres at worker call time

**`internal/config/config.go`**: `ReportsRoot string` field + `reports_root` viper key + default `/var/lib/shifter/reports`

**`internal/http/router.go`**: `ReportDeps *report.Deps` field + nil-guarded `report.RegisterRoutes(r, *deps.ReportDeps)` before SPA fallback

**`compose/bundled.yml` + `compose/external.yml`**: `reports_cache` named volume declared in top-level `volumes:` block and mounted at `- reports_cache:/var/lib/shifter/reports` in the shifter service

## Test Coverage

| Test | File | What it asserts |
|------|------|-----------------|
| TestPDFReportArgs_Kind | pdf_worker_test.go | Kind() == "pdf_report" |
| TestCleanupExpiredReportsArgs_Kind | pdf_worker_test.go | Kind() == "report_cleanup" |
| TestPDFJob | pdf_worker_test.go | pdf_status='ready', file on disk with %PDF- prefix |
| TestCleanupWorker | pdf_worker_test.go | pdf_status='expired', artifact_dir removed |
| TestEnqueuePDF_AtomicWithReport | pdf_worker_test.go | Kind() shape + atomicity covered by TestGenerateHandler_AuditInSameTx_RollbackBoth |
| TestGenerateHandler_EnqueuesPDFJob | handlers_test.go | EnqueuePDF called once with new report UUID |
| TestStatusHandler_PendingReadyFailed | handlers_test.go | pdf_path present only when ready (omitempty contract) |
| TestStatusHandler_NotFound | handlers_test.go | 404 for unknown UUID |
| TestStatusHandler_BadUUID | handlers_test.go | 400 before any DB lookup |
| TestStatusHandler_Unauthenticated | handlers_test.go | 401 with no session |
| TestStatusHandler_ViewerCannotSeeOthers | handlers_test.go | 404 conceals existence from non-owner |

## Deviations from Plan

### Session Interruption (stream-idle timeout)
The previous executor session completed Task 1 (PDF writer, committed 276fb0e) but was interrupted mid-Task 2 by a stream-idle timeout. All Task 2 files were on disk but uncommitted. This continuation session:
1. Read all uncommitted files to assess quality and completeness
2. Found all three new files (cleanup.go, download_handler.go, pdf_worker.go) and modified handler/test files were complete and correct
3. Added the remaining missing pieces: config.go ReportsRoot field, router.go ReportDeps, serve.go River wiring + sqlcIdentityProvider, and compose volume mounts
4. Committed everything as one atomic Task 2 commit (16b9f9a)

### serve.go location (Rule 1 — plan referenced wrong path)
The plan's acceptance criteria references `cmd/serve/serve.go` but this project's serve logic lives at `internal/cli/serve.go` (cobra subcommand). River wiring was placed in the correct file. The `cmd/shifter/main.go` delegates to `cli.Execute()`.

### ReportsRoot added to config (Rule 2 — missing critical wiring)
The plan's `Deps.ArtifactsRoot` required a runtime-configurable path but no `ReportsRoot` field existed in `config.Config`. Added `ReportsRoot string` with viper key `reports_root` and default `/var/lib/shifter/reports` (matches compose volume mount). Without this, the serve binary would always use an empty string for `ArtifactsRoot`.

### sqlcIdentityProvider loads at worker runtime (Rule 2 — correctness)
The plan's serve.go snippet embedded a static `InstallIdentity` in `reportDeps`. If the operator updates display_name/address via the install wizard after boot, a static copy would be stale for all subsequent PDF renders. The `sqlcIdentityProvider` struct loads from `install_identity WHERE id=1` at each `Work()` call — one extra DB query per PDF job, negligible cost vs. PDF generation.

## Known Stubs
None — all data paths are wired. The `reportDeps.Identity` field in serve.go is set to a zero-value `report.InstallIdentity{}` (placeholder) because the actual identity is loaded at worker runtime via `identityProvider` (the `InstallIdentityProvider` interface). The zero value is never used — `PDFReportWorker.Work()` calls `w.Identity.Load(ctx)` which hits the DB.

## Threat Surface Scan
No new trust boundaries beyond those in the plan's threat model. All T-05-06-0x mitigations are implemented:
- T-05-06-02: UUID parsed before filepath.Join in both DownloadHandler and StatusHandler
- T-05-06-04: 404 (not 403) for cross-user report access in StatusHandler and DownloadHandler
- T-05-06-01: Worker has no user context — authorization enforced only at GenerateHandler time
- T-05-06-03: MaxWorkers=4 caps concurrent PDF CPU; 24h TTL + hourly cleanup enforced

## Verification Results

```
go test ./internal/report/... -race -count=1 -short         → 17 passed
go test ./internal/http/... -race -count=1 -short           → 21 passed
go test ./internal/cli/... -race -count=1 -short            → 13 passed
go build ./...                                               → SUCCESS
go build ./cmd/...                                           → SUCCESS
grep reports_cache compose/bundled.yml compose/external.yml → 4 matches (2 per file)
river.NewClient + river.AddWorker in internal/cli/serve.go  → 3 matches
r.Get("/api/reports/{id}" in handlers.go                    → present
```

**GET /api/reports/{id} returns exactly the ReportStatus shape plan 05-09 expects:**
```go
type StatusResponse struct {
    ID        uuid.UUID  `json:"id"`
    PdfStatus string     `json:"pdf_status"`         // pending|running|ready|failed|expired
    PdfPath   *string    `json:"pdf_path,omitempty"` // present only when ready
    CreatedAt time.Time  `json:"created_at"`
    Scope     string     `json:"scope"`
    Range     string     `json:"range"`
}
```

**River worker max retry count:** River default is 25 retries with exponential backoff. The plan did not specify a custom max retry — the default is appropriate for PDF generation (transient DB errors, disk pressure). `AfterJobFailed` is not wired as a separate error handler; the plan says "surface error in audit log" — this is a deferred item (see below).

**Shutdown timeout:** 30s stop context in the defer for `riverClient.Stop()`. At `MaxWorkers=4` with typical PDF generation taking <10s, 30s is sufficient for all in-flight jobs to complete.

## Deferred Items

| Item | Reason |
|------|---------|
| AfterJobFailed audit log entry | Plan says "surface error in audit log" — the River `ErrorHandler` hook was not wired because the `audit.WriteEntry` signature requires a `pgx.Tx` which River's error handler doesn't provide directly. A separate goroutine polling `river_job` for `state='discarded'` rows and writing audit entries would be the correct approach. Deferred to plan 05-09 or a dedicated hardening plan. |
| SHIFTER_REPORTS_ROOT in compose env | The env var is readable via `SHIFTER_REPORTS_ROOT` but not listed in the `environment:` blocks of bundled.yml/external.yml. The default `/var/lib/shifter/reports` matches the volume mount so this is cosmetic for now. |

## Self-Check: PASSED
