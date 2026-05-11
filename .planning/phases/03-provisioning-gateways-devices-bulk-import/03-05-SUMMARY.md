---
phase: 03-provisioning-gateways-devices-bulk-import
plan: 05
subsystem: bulk-import
tags: [xlsx, csv, excelize, multipart, audit-envelope, idempotency, ttl, chirpstack-grpc, sqlc, pgx]

requires:
  - phase: 02-domain-model-canonical-schema
    provides:
      - audit.WriteEntry (entry-shape; same-tx writer)
      - atomic CS+PG transaction pattern (D-16; reused per-row)
      - sqlc/pgx codegen pipeline
  - phase: 03-01
    provides:
      - testsupport.xlsx_fixtures + testsupport.eui_fixtures
      - excelize/v2 dependency in go.mod
      - Wave 0 import-package test skeletons
  - phase: 03-02
    provides:
      - audit_log 0020 vocabulary extension (device.bulk_import, import_job)
      - import_job + import_job_row tables (migration 0019)
      - ActionDeviceBulkImport authz constant
  - phase: 03-04
    provides:
      - GatewayDeps nil-guard pattern (template for ImportDeps wiring)

provides:
  - "internal/import/euikeys.go: Normalize{DevEUI,JoinEUI,AppKey,DevAddr,NwkSKey,AppSKey} + ParseEUI64 alias"
  - "internal/import/parser_xlsx.go: ParseXLSX(io.Reader) → []ParsedRow (lossless raw_payload preservation)"
  - "internal/import/parser_csv.go: ParseCSV with BOM strip + UTF-8 enforcement (D-04a)"
  - "internal/import/template.go: GenerateTemplate() with navy header + NumFmt=49 hex + OTAA/ABP dropdown"
  - "internal/import/errors_xlsx.go: GenerateErrorsXLSX with appended error_reason column"
  - "internal/import/dryrun.go: DryRunDeps.Validate (per-row outcome + intra-file dedup + FK lookups)"
  - "internal/import/commit.go: CommitDeps.Commit (per-row Serializable tx + envelope+per-device audit)"
  - "internal/import/job_ttl.go: ExpireIfStale lazy preview→expired transition"
  - "internal/import/handlers.go: 5 HTTP endpoints + RegisterRoutes + 5 MiB / 5000-row / .xlsm guards"
  - "13 sqlc queries for import_job + import_job_row state machine"

affects: [03-09 frontend bulk import dialog, 09 audit query UI, 09 retention sweeper]

tech-stack:
  added:
    - "github.com/xuri/excelize/v2 (first production use — was test-only in 03-01)"
  patterns:
    - "Per-row Serializable tx with best-effort CS rollback (mirrors 02 D-16)"
    - "Audit envelope + per-device shape with request_id=job_id grouping (D-33/D-34)"
    - "Lazy TTL transition via conditional UPDATE in ExpireIfStale (no background worker)"
    - "Lossless raw_payload JSONB preservation for errors.xlsx round-trip"
    - "Multipart upload guard chain: MaxBytesReader → extension sniff → parse → row cap"

key-files:
  created:
    - internal/db/queries/import_jobs.sql
    - internal/import/euikeys.go
    - internal/import/parser_xlsx.go
    - internal/import/parser_csv.go
    - internal/import/template.go
    - internal/import/errors_xlsx.go
    - internal/import/dryrun.go
    - internal/import/commit.go
    - internal/import/job_ttl.go
    - internal/import/handlers.go
    - internal/import/handlers_test.go
    - internal/import/fixture_test.go
  modified:
    - internal/db/sqlc/import_jobs.sql.go (sqlc generated)
    - internal/db/sqlc/querier.go (sqlc interface extension)
    - internal/import/euikeys_test.go
    - internal/import/parser_xlsx_test.go
    - internal/import/parser_csv_test.go
    - internal/import/template_test.go
    - internal/import/dryrun_test.go
    - internal/import/commit_test.go
    - internal/import/job_ttl_test.go
    - internal/audit/bulk_import_envelope_test.go
    - internal/http/router.go

key-decisions:
  - "tags stored as JSONB column on gateway only (NOT on device for bulk-import); device tags deferred to Phase 7 vendor catalog work — bulk-import schema documents tags column but parser is permissive about its absence"
  - "buildBigXLSX test helper emits CSV bytes (faster than excelize for 5001-row cap test); handler routes by extension so .csv route hits ParseCSV"
  - "isNoRows error recogniser uses substring match on 'no rows in result set' rather than importing pgx — keeps dryrun.go thin"
  - "Per-row commit fails the row only (not the job) on CS errors; envelope audit row always lands so the operator gets the summary"
  - "audit_test package (not audit) for bulk_import_envelope_test.go to avoid import cycle with importpkg"

patterns-established:
  - "Multipart upload guards layered as decorators (MaxBytesReader wraps Body before ParseMultipartForm), so each cap fires with its own error code"
  - "Test fixture builder (newImportFixture) returns the testcontainer + seeded users + sites + profiles + CS/bootstrap fakes in one helper, reused across dryrun/commit/job_ttl tests"
  - "Idempotent commit short-circuits via summaryFromJob when import_job.status='committed' on entry"

requirements-completed: [DEV-03, DEV-06, DEV-07, DEV-08, DEV-09, UX-03, CHIRP-05, CHIRP-06]

duration: 45min
completed: 2026-05-11
---

# Phase 3 Plan 03-05: Bulk-Import Backend Summary

**Two-phase XLSX/CSV bulk import — parse, dry-run, idempotent commit — with audit envelope per D-33/D-34, lazy 1h TTL on previews, and 5 RBAC-gated HTTP endpoints.**

## Performance

- **Duration:** ~45 min
- **Started:** 2026-05-11T06:55:00Z
- **Completed:** 2026-05-11T07:39:00Z
- **Tasks:** 4 (all TDD)
- **Files modified:** 22 (12 created, 10 modified)
- **Test count:** 33 import package tests + 2 audit envelope tests, all passing

## Accomplishments

- Idempotent two-phase upload workflow: same XLSX re-uploaded yields `already_exists` outcomes, never duplicates
- Per-row Serializable tx commit with best-effort ChirpStack rollback when a per-row PG insert fails (Phase 2 D-16 pattern reused)
- Audit shape per D-33/D-34: 1 envelope row + N per-device rows sharing `request_id = job_id` (single-query reconstruction)
- Defense-in-depth: 5 MiB body cap, 5000-row cap, .xlsm rejection, UTF-8 enforcement on CSV with operator-readable error
- Lazy TTL: preview jobs auto-transition to `expired` on read past their 1h deadline; no background worker required for Phase 3
- Errors.xlsx round-trip: operator downloads, fixes rows, re-uploads → idempotent re-commit creates only the fixed rows

## Public Go API surface

### `internal/import/euikeys.go` — normalisation

```go
func NormalizeDevEUI(raw string) (string, error)   // 16 hex
func NormalizeJoinEUI(raw string) (string, error)  // 16 hex
func NormalizeAppKey(raw string) (string, error)   // 32 hex
func NormalizeDevAddr(raw string) (string, error)  // 8 hex
func NormalizeNwkSKey(raw string) (string, error)  // 32 hex
func NormalizeAppSKey(raw string) (string, error)  // 32 hex
func ParseEUI64(raw string) (string, error)        // alias of NormalizeDevEUI
```

### `internal/import/parser_*.go` — file ingest

```go
func ParseXLSX(r io.Reader) ([]ParsedRow, error)
func ParseCSV(r io.Reader)  ([]ParsedRow, error)  // BOM strip + UTF-8 only
type ParsedRow struct {
    RowIndex int                // 1-based file row
    Raw      map[string]string  // lowercase header → trimmed cell value
}
```

### `internal/import/dryrun.go` — validation

```go
type DryRunDeps struct { Queries *sqlc.Queries }
func NewDryRunDeps(q *sqlc.Queries) *DryRunDeps
func (d *DryRunDeps) Validate(ctx, rows) ([]RowOutcome, error)

type RowOutcome struct {
    RowIndex int
    Status   OutcomeStatus  // valid|invalid|already_exists
    Reason   string
    Parsed   map[string]any  // canonical values for commit phase
}
```

### `internal/import/commit.go` — apply

```go
type CommitDeps struct {
    Pool      *pgxpool.Pool
    CS        CommitCSClient        // narrow CS interface (testable)
    Bootstrap CommitBootstrap       // CS tenant+app resolver
    Log       *slog.Logger
}
func (d *CommitDeps) Commit(ctx, jobID, actorID uuid.UUID) (CommitSummary, error)
```

### `internal/import/job_ttl.go` — TTL

```go
func ExpireIfStale(ctx, q, jobID) (sqlc.ImportJob, error)
```

### `internal/import/template.go` / `errors_xlsx.go`

```go
func GenerateTemplate() ([]byte, error)
func GenerateErrorsXLSX(rows []sqlc.ImportJobRow) ([]byte, error)
```

### `internal/import/handlers.go` — HTTP

```go
func RegisterRoutes(r chi.Router, d Deps)
type Deps struct { Pool, SessionMgr, Log, Commit *CommitDeps }
```

## 5 HTTP endpoint contracts

| Method | Path | Body | RBAC | Status codes |
|---|---|---|---|---|
| POST | `/api/imports` | multipart/form-data `file` ≤ 5 MiB | `device.bulk_import` (admin) | 200 / 400 (parse, xlsm, too_many_rows) / 401 / 403 / 413 (over cap) |
| GET | `/api/imports` | — | admin | 200 |
| GET | `/api/imports/template.xlsx` | — | admin | 200 (XLSX body) |
| GET | `/api/imports/{job_id}` | — | admin | 200 / 404 |
| POST | `/api/imports/{job_id}/commit` | — | admin | 200 / 404 / 409 (not_commitable) / 410 (expired) |
| GET | `/api/imports/{job_id}/errors.xlsx` | — | admin | 200 (XLSX) / 404 |

### Upload response shape (200)

```json
{
  "job_id": "uuid",
  "status": "preview",
  "total": 5,
  "valid_count": 5,
  "invalid_count": 0,
  "already_exists_count": 0,
  "expires_at": "2026-05-11T08:39:00Z",
  "outcomes": [
    { "row_index": 2, "status": "valid" }
  ],
  "outcomes_limit": 50
}
```

### Commit response shape (200)

```json
{
  "job_id": "uuid",
  "total": 5,
  "created": 5,
  "already_exists": 0,
  "failed": 0,
  "invalid": 0,
  "valid": 5,
  "envelope_audit_written": true
}
```

## Audit shape (D-33/D-34)

| Column | Envelope row | Per-device row |
|---|---|---|
| `action` | `device.bulk_import` | `create` |
| `entity_type` | `import_job` | `device` |
| `entity_id` | `import_job.job_id` (UUID) | newly-inserted `device.id` |
| `before` | NULL | NULL |
| `after` | `{total, created, already_exists, failed, invalid, valid, job_id, file_name, file_format}` | `{dev_eui, name, device_profile_id, join_eui, description, activation_mode}` |
| `notes` | empty | `bulk_import` |
| `request_id` | `job_id.String()` | `job_id.String()` (SAME — D-34) |

Reconstruct an entire import operation: `SELECT * FROM audit_log WHERE request_id = $1`.

## Defense-in-depth bounds

| Bound | Source | Value | Enforcement |
|---|---|---|---|
| body size | `http.MaxBytesReader` | 5 MiB | Returns 413 on over-cap (T-3-40) |
| row count | `MaxRowsPerJob` | 5000 | Handler checks `len(rows) > 5000` after parse |
| filename extension | `.xlsx` / `.csv` only | reject `.xlsm` | T-3-40 macro vector |
| CSV encoding | `utf8.Valid` | UTF-8 only | Rejects with D-04a remediation message (T-3-42) |
| viewer mutation | `auth.RequireAction(ActionDeviceBulkImport)` | admin-only | Returns 403 (T-3-43) |
| errors.xlsx formula injection | `f.SetCellStr` (text) not `SetCellValue` (typed) | inert | T-3-41 |

## Idempotency proof

Re-uploading the same XLSX produces these outcomes:

1. **First upload:** all rows `valid` → commit creates N devices, audit gets N+1 rows
2. **Second upload of the same file:** all rows `already_exists` (D-06; dry-run hits `q.GetDeviceByDevEUI` and short-circuits)
3. **Second commit of the same `job_id`:** `summaryFromJob` short-circuits in `Commit`; CS is never called a second time; PG device count is unchanged

Verified by `TestCommit_Idempotent` and `TestCommitHandler_Idempotent`.

## Decisions Made

- **Per-row commit failure isolation:** A CS error on row N marks that row `failed` and proceeds with row N+1. The job still completes; the envelope audit row carries the failed count. Operator can `errors.xlsx` → fix → re-upload. (Plan 03-05 D-Discretion #6.)
- **Lazy TTL over background sweep:** `ExpireIfStale` runs on every read of a preview job; no Phase 3 scheduler needed. Phase 9's retention cron will batch-expire `import_job` rows older than 90 days separately.
- **tags JSONB deferred:** Plan output requested documenting tags as JSONB (mirroring gateway), but Phase 3 device schema has no tags column. The bulk-import parser accepts tags in the file (operator-pasted) but currently drops them — Phase 7 vendor-catalog work will surface them. Rationale: adding a column now would expand the device schema scope outside the plan boundary.
- **audit_test package:** The bulk_import_envelope_test.go file uses the `audit_test` package (not `audit`) so it can call `audit.WriteEntry` without forming a cycle through importpkg.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Comment containing literal BOM character broke parser_csv.go compile**
- **Found during:** Task 2 (parser_csv.go)
- **Issue:** Go source files reject literal U+FEFF (BOM) characters even inside comments — `illegal byte order mark` build error
- **Fix:** Rewrote the BOM comment to spell out "BOM-prefixed dev_eui string" without the literal character
- **Files modified:** internal/import/parser_csv.go
- **Committed in:** 59f6cee (Task 2)

**2. [Rule 1 - Bug] 5000-row cap test used .xlsx filename for CSV bytes**
- **Found during:** Task 4 (handler tests)
- **Issue:** buildBigXLSX is a CSV producer (faster than excelize for 5001 rows); uploading the bytes with `.xlsx` filename routed to ParseXLSX which failed with "not a valid zip archive" → test got "parse_failed" instead of "too_many_rows"
- **Fix:** Changed test filename to `big.csv` so handler routes to ParseCSV
- **Files modified:** internal/import/handlers_test.go
- **Committed in:** ba471bc (Task 4)

---

**Total deviations:** 2 auto-fixed (1 blocking compile, 1 test wiring bug)
**Impact on plan:** Neither affects production behaviour; both caught and fixed inside the same task commit. No scope creep.

## Issues Encountered

- One flaky testcontainer port-binding failure during the full-suite run (TestCommit_PerRowCS_FailureSkipsRow). Resolved by re-running serially. Root cause: when multiple test binaries spawn testcontainers concurrently, the Postgres ports collide. Not a code bug.

## User Setup Required

None — bulk import uses the existing ChirpStack and Postgres wiring from Phases 1–3.

## Next Phase Readiness

Plan 03-09 (frontend bulk-import dialog + imports admin pages) can now consume:
- `POST /api/imports` for upload + preview rendering
- `POST /api/imports/{job_id}/commit` button-confirm flow
- `GET /api/imports/{job_id}` for the preview row table (D-09 expandable rows for `reason`)
- `GET /api/imports/template.xlsx` "Download template" CTA
- `GET /api/imports/{job_id}/errors.xlsx` "Download errors" CTA

ImportDeps is NOT yet wired in `internal/cli/serve.go` — same status as GatewayDeps from 03-04. Wiring lands in a follow-up plan alongside the gateway wiring; both share the CS-client construction prerequisite that Plan 03-10 (cmd-wiring catch-up) is expected to do.

## Self-Check: PASSED

- `[FOUND]` internal/import/euikeys.go
- `[FOUND]` internal/import/parser_xlsx.go
- `[FOUND]` internal/import/parser_csv.go
- `[FOUND]` internal/import/template.go
- `[FOUND]` internal/import/errors_xlsx.go
- `[FOUND]` internal/import/dryrun.go
- `[FOUND]` internal/import/commit.go
- `[FOUND]` internal/import/job_ttl.go
- `[FOUND]` internal/import/handlers.go
- `[FOUND]` internal/db/queries/import_jobs.sql
- `[FOUND]` commit `32c70e5` (Task 1: sqlc + euikeys)
- `[FOUND]` commit `59f6cee` (Task 2: parsers + template + errors.xlsx)
- `[FOUND]` commit `6a383c5` (Task 3: dryrun + commit + ttl + audit envelope)
- `[FOUND]` commit `ba471bc` (Task 4: 5 HTTP handlers + router mount)

---
*Phase: 03-provisioning-gateways-devices-bulk-import*
*Completed: 2026-05-11*
