---
phase: 05-aggregates-reports-map-floor-plans
plan: 03
type: execute
wave: 2
depends_on: [02]
files_modified:
  - internal/db/queries/reports.sql
  - internal/db/sqlc/reports.sql.go
  - internal/db/sqlc/models.go
  - sqlc.yaml
  - internal/report/doc.go
  - internal/report/assembler.go
  - internal/report/assembler_test.go
  - internal/report/delta.go
  - internal/report/delta_test.go
  - internal/report/csv.go
  - internal/report/csv_test.go
  - internal/report/excel.go
  - internal/report/excel_test.go
  - internal/report/handlers.go
  - internal/report/handlers_test.go
autonomous: true
requirements: [REPT-01, REPT-02, REPT-03, REPT-04, REPT-07]
threat_refs: [T-05-03-01, T-05-03-02]

must_haves:
  truths:
    - "POST /api/reports/generate accepts {scope=all|site|meter, site_id?, mp_id?, range=daily|monthly|yearly|custom, start?, end?, group?} and returns {report_id, summary, period_rows, meter_rows}"
    - "Period-delta column is present on every period row: delta_vs_prior is always populated; delta_vs_yoy is populated only when ≥1 measurement exists in the same window one year earlier (D-03 silent fallback)"
    - "CSV writer prepends UTF-8 BOM (\\xEF\\xBB\\xBF), uses comma separator, formats timestamps as ISO-8601 with the install_identity.timezone offset, and writes a 3-row metadata header block before data rows"
    - "Excel writer produces a workbook with 3 sheets named exactly Summary / Period Detail / Meter Detail; dates formatted as Excel dates (NOT strings); totals row is bold + top-border"
    - "All report data queries hit CAGGs (measurement_daily / measurement_monthly / measurement_yearly), NEVER raw measurement (Pitfall: 90d+ raw queries are expensive)"
    - "Daily range uses measurement_hourly; monthly uses measurement_daily; yearly uses measurement_monthly — strict level shift per ROADMAP success criteria #2"
    - "Group=site sums per site_id (JOIN measurement_* with metering_point and site); group=category (utility class) sums per metering_point.utility_class"
    - "scope=all hides absent capabilities — single-capability install (Phase 4 D-09 capabilities=water) produces water-only sections, no empty electricity section"
    - "Audit log entry written inside the same pgx.Tx that creates the report record per Phase 2 D-23 invariant"
    - "All sqlc queries parameter-binding; NO string concatenation in SQL"
  artifacts:
    - path: "internal/db/queries/reports.sql"
      provides: "sqlc-annotated SELECT queries against measurement_hourly/daily/monthly/yearly with scope + group permutations"
      contains: "name: ReportDaily"
    - path: "internal/report/doc.go"
      provides: "Package documentation: scope/range/group matrix, CAGG selection rules, CSV/Excel format spec"
      contains: "package report"
    - path: "internal/report/assembler.go"
      provides: "BuildReport(ctx, q, ReportConfig) → Report struct (summary + period_rows + meter_rows)"
      contains: "func BuildReport"
    - path: "internal/report/delta.go"
      provides: "ComputeDelta(curr, prior) → DeltaResult{absolute, percent}; ComputeYoY(curr, prior_year_window) → *DeltaResult (nil = silent fallback)"
      contains: "func ComputeDelta"
    - path: "internal/report/csv.go"
      provides: "WriteCSV(w io.Writer, rpt *Report, identity InstallIdentity) — UTF-8 BOM + 3-row metadata header + period detail rows"
      contains: "0xEF, 0xBB, 0xBF"
    - path: "internal/report/excel.go"
      provides: "WriteExcel(rpt *Report, identity InstallIdentity) ([]byte, error) — 3 sheets via excelize/v2"
      contains: "Summary"
    - path: "internal/report/handlers.go"
      provides: "POST /api/reports/generate handler (synchronous: returns assembled Report + creates DB record + audit row in same tx)"
      contains: "func GenerateHandler"
  key_links:
    - from: "internal/report/assembler.go"
      to: "measurement_hourly / measurement_daily / measurement_monthly / measurement_yearly"
      via: "sqlc-generated query functions from reports.sql.go"
      pattern: "q\\.Report(Daily|Monthly|Yearly)"
    - from: "internal/report/handlers.go"
      to: "audit.WriteEntry(ctx, tx, …)"
      via: "same pgx.Tx as report record INSERT"
      pattern: "audit\\.WriteEntry\\(ctx, tx,"
---

<objective>
Build the data layer for Phase 5 reports: sqlc queries against the CAGG hierarchy (covering daily/monthly/yearly × all-meters/site/single-meter × group-by-site/category), the assembler that maps query results into a `Report` struct with prior-period and YoY deltas (D-03 silent fallback), the CSV writer (UTF-8 BOM + ISO timestamps + metadata header), the Excel writer (3 sheets via excelize), and the synchronous `POST /api/reports/generate` handler.

Purpose: Reports are Phase 5's purchase justification (per ROADMAP). This plan ships everything the user can download immediately after clicking Generate — CSV + Excel + the in-memory data the PDF worker (plan 05-06) will format.

Output: One sqlc query file, one Go package (`internal/report/`) with 5 source + test files, HTTP handler scaffolding for the synchronous immediate-download path. PDF generation + River worker land in plan 05-06.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/05-aggregates-reports-map-floor-plans/05-CONTEXT.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-RESEARCH.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-UI-SPEC.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-02-cagg-hierarchy-retention-PLAN.md
@internal/aggregate/doc.go
@internal/db/queries/dashboard.sql
@internal/db/sqlc/dashboard.sql.go
@internal/audit/log.go
@internal/dashboard/kpi.go
@sqlc.yaml

<interfaces>
<!-- Existing audit.WriteEntry signature (from internal/audit/log.go) -->
```go
type Entry struct {
    UserID     uuid.UUID
    Action     string
    EntityType string
    EntityID   uuid.UUID
    Before     map[string]any
    After      map[string]any
    RequestID  string
}

func WriteEntry(ctx context.Context, tx pgx.Tx, e Entry) error
```

<!-- New audit constants this plan adds -->
```go
const (
    ActionGenerateReport = "report.generate"
    EntityTypeReport     = "report"
)
```

<!-- New table this plan creates (in queries/reports.sql or via inline 0030 migration —
     planner decision: store metadata in a regular table, not a hypertable) -->
```sql
report (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id       UUID NOT NULL REFERENCES users(id),
  scope         TEXT NOT NULL CHECK (scope IN ('all','site','meter')),
  site_id       UUID,        -- when scope='site'
  metering_point_id UUID,    -- when scope='meter'
  group_by      TEXT CHECK (group_by IN ('site','category','none')),
  range_kind    TEXT NOT NULL CHECK (range_kind IN ('daily','monthly','yearly','custom')),
  range_start   TIMESTAMPTZ NOT NULL,
  range_end     TIMESTAMPTZ NOT NULL,
  artifact_dir  TEXT NOT NULL,
  pdf_status    TEXT NOT NULL DEFAULT 'pending'
                CHECK (pdf_status IN ('pending','running','ready','failed','expired')),
  pdf_path      TEXT,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at    TIMESTAMPTZ NOT NULL  -- created_at + 24 hours (D-07)
)
```
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: sqlc queries + report table migration + assembler + delta math</name>
  <read_first>
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-CONTEXT.md §D-01 §D-03 §D-04 §D-05
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-RESEARCH.md §CAGG Hierarchy §Pattern 4 (per-period breakdown)
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-UI-SPEC.md §Report Layout Spec §Period-Delta Display
    - internal/aggregate/doc.go (CAGG hierarchy + LAG semantics)
    - internal/db/queries/dashboard.sql (existing sqlc-against-measurement reference)
    - internal/dashboard/kpi.go (existing assembler pattern reference)
  </read_first>
  <behavior>
    - Test 1: Daily report for one MP over one week → 7 period rows with delta_vs_prior populated for rows 2-7 (first row has nil prior)
    - Test 2: Monthly report when prior year has data → delta_vs_yoy populated; when prior year has no data → delta_vs_yoy is nil (silent fallback per D-03)
    - Test 3: scope=all + group=category → produces water section + electricity section (or one section for single-capability install)
    - Test 4: scope=site + group=none → meter_rows lists every MP belonging to the site
    - Test 5: Daily range hits measurement_hourly; monthly hits measurement_daily; yearly hits measurement_monthly (verified via sqlc query name routing)
  </behavior>
  <action>
**Step A — Migration 0030_report.up.sql (and .down.sql):**

```sql
-- 0030_report.up.sql
-- Report metadata records (Phase 5 REPT-01..07). Ephemeral artifacts (D-07):
-- expires_at is 24h after created_at; River PeriodicJob in plan 05-06 purges
-- expired rows + their artifact_dir contents.

CREATE TABLE report (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id           UUID NOT NULL REFERENCES users(id),
  scope             TEXT NOT NULL CHECK (scope IN ('all','site','meter')),
  site_id           UUID REFERENCES site(id),
  metering_point_id UUID REFERENCES metering_point(id),
  group_by          TEXT CHECK (group_by IN ('site','category','none')),
  range_kind        TEXT NOT NULL CHECK (range_kind IN ('daily','monthly','yearly','custom')),
  range_start       TIMESTAMPTZ NOT NULL,
  range_end         TIMESTAMPTZ NOT NULL CHECK (range_end > range_start),
  artifact_dir      TEXT NOT NULL,
  pdf_status        TEXT NOT NULL DEFAULT 'pending'
                    CHECK (pdf_status IN ('pending','running','ready','failed','expired')),
  pdf_path          TEXT,
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at        TIMESTAMPTZ NOT NULL,
  CONSTRAINT report_scope_keys CHECK (
    (scope = 'site' AND site_id IS NOT NULL AND metering_point_id IS NULL) OR
    (scope = 'meter' AND metering_point_id IS NOT NULL AND site_id IS NULL) OR
    (scope = 'all' AND site_id IS NULL AND metering_point_id IS NULL)
  )
);

CREATE INDEX report_user_created_idx ON report (user_id, created_at DESC);
CREATE INDEX report_expires_idx     ON report (expires_at) WHERE pdf_status <> 'expired';
```

```sql
-- 0030_report.down.sql
DROP TABLE IF EXISTS report;
```

**Step B — `internal/db/queries/reports.sql` (sqlc-annotated):**

Write queries (one per range × scope combination — sqlc generates a typed function per query):

```sql
-- name: CreateReport :one
INSERT INTO report (user_id, scope, site_id, metering_point_id, group_by, range_kind, range_start, range_end, artifact_dir, expires_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING *;

-- name: GetReport :one
SELECT * FROM report WHERE id = $1;

-- name: UpdateReportPDFStatus :exec
UPDATE report SET pdf_status = $2, pdf_path = $3 WHERE id = $1;

-- name: ListExpiredReports :many
SELECT id, artifact_dir FROM report
WHERE expires_at < now() AND pdf_status <> 'expired';

-- name: MarkReportExpired :exec
UPDATE report SET pdf_status = 'expired' WHERE id = $1;

---------- DATA QUERIES ----------

-- name: ReportDailyByMP :many
-- Daily report — sources measurement_hourly, buckets to days.
-- Returns one row per (day, metering_point_id).
SELECT
  time_bucket('1 day', bucket) AS period,
  metering_point_id,
  sum(cumulative_delta)        AS consumption,
  sum(uplink_count)            AS uplinks,
  sum(flagged_count)           AS flagged
FROM measurement_hourly
WHERE metering_point_id = $1
  AND bucket >= $2 AND bucket < $3
GROUP BY 1, 2
ORDER BY 1;

-- name: ReportDailyBySite :many
SELECT
  time_bucket('1 day', mh.bucket) AS period,
  mp.site_id,
  s.name                          AS site_name,
  sum(mh.cumulative_delta)        AS consumption
FROM measurement_hourly mh
JOIN metering_point mp ON mp.id = mh.metering_point_id
JOIN site s ON s.id = mp.site_id
WHERE mh.bucket >= $1 AND mh.bucket < $2
  AND ($3::UUID IS NULL OR mp.site_id = $3)
GROUP BY 1, 2, 3
ORDER BY 1, 3;

-- name: ReportDailyByCategory :many
-- Group by metering_point.utility_class (D-04: utility class = water | electricity).
SELECT
  time_bucket('1 day', mh.bucket) AS period,
  mp.utility_class,
  sum(mh.cumulative_delta)        AS consumption
FROM measurement_hourly mh
JOIN metering_point mp ON mp.id = mh.metering_point_id
WHERE mh.bucket >= $1 AND mh.bucket < $2
GROUP BY 1, 2
ORDER BY 1, 2;

-- name: ReportMonthlyByMP :many
SELECT
  bucket AS period,
  metering_point_id,
  cumulative_delta AS consumption,
  uplink_count AS uplinks,
  flagged_count AS flagged
FROM measurement_monthly
WHERE metering_point_id = $1
  AND bucket >= $2 AND bucket < $3
ORDER BY bucket;

-- name: ReportMonthlyBySite :many
SELECT
  mm.bucket          AS period,
  mp.site_id,
  s.name             AS site_name,
  sum(mm.cumulative_delta) AS consumption
FROM measurement_monthly mm
JOIN metering_point mp ON mp.id = mm.metering_point_id
JOIN site s ON s.id = mp.site_id
WHERE mm.bucket >= $1 AND mm.bucket < $2
  AND ($3::UUID IS NULL OR mp.site_id = $3)
GROUP BY 1, 2, 3
ORDER BY 1, 3;

-- name: ReportMonthlyByCategory :many
SELECT
  mm.bucket           AS period,
  mp.utility_class,
  sum(mm.cumulative_delta) AS consumption
FROM measurement_monthly mm
JOIN metering_point mp ON mp.id = mm.metering_point_id
WHERE mm.bucket >= $1 AND mm.bucket < $2
GROUP BY 1, 2
ORDER BY 1, 2;

-- name: ReportYearlyByMP :many
SELECT bucket AS period, metering_point_id, cumulative_delta AS consumption
FROM measurement_yearly
WHERE metering_point_id = $1
  AND bucket >= $2 AND bucket < $3
ORDER BY bucket;

-- name: ReportYearlyBySite :many
SELECT
  my.bucket          AS period,
  mp.site_id,
  s.name             AS site_name,
  sum(my.cumulative_delta) AS consumption
FROM measurement_yearly my
JOIN metering_point mp ON mp.id = my.metering_point_id
JOIN site s ON s.id = mp.site_id
WHERE my.bucket >= $1 AND my.bucket < $2
  AND ($3::UUID IS NULL OR mp.site_id = $3)
GROUP BY 1, 2, 3
ORDER BY 1, 3;

-- name: ReportYearlyByCategory :many
SELECT
  my.bucket           AS period,
  mp.utility_class,
  sum(my.cumulative_delta) AS consumption
FROM measurement_yearly my
JOIN metering_point mp ON mp.id = my.metering_point_id
WHERE my.bucket >= $1 AND my.bucket < $2
GROUP BY 1, 2
ORDER BY 1, 2;

-- name: ListMetersInScope :many
-- Returns metering points in scope (all / site / single) for the meter_rows table.
SELECT
  mp.id,
  mp.name,
  mp.utility_class,
  mp.site_id,
  s.name AS site_name
FROM metering_point mp
JOIN site s ON s.id = mp.site_id
WHERE
  ($1::TEXT = 'all') OR
  ($1::TEXT = 'site' AND mp.site_id = $2) OR
  ($1::TEXT = 'meter' AND mp.id = $3)
ORDER BY s.name, mp.name;
```

`sqlc generate` after editing. Confirm `sqlc.yaml` includes `queries/reports.sql`.

**Step C — `internal/report/doc.go`:**

```go
// Package report assembles consumption reports from the CAGG hierarchy
// (DATA-11..13) and emits CSV (REPT-03) + Excel (REPT-04) artifacts. PDF
// generation lives in plan 05-06 via a River background worker (REPT-05/06)
// and writes to the same on-disk artifact directory.
//
// # Scope and grouping (D-01, D-04, D-05)
//
//   scope=all + group=site      → per-site rollups
//   scope=all + group=category  → per-utility-class rollups (water / electricity)
//   scope=all + group=none      → fleet-wide totals only
//   scope=site                  → per-MP rows within one site
//   scope=meter                 → single MP detail
//
// # CAGG selection
//
//   range=daily   → measurement_hourly (bucketed to days in the query)
//   range=monthly → measurement_daily  (bucketed to months in the query)
//   range=yearly  → measurement_monthly (bucketed to years in the query)
//   range=custom  → planner-routed: ≤30d uses hourly, ≤1y uses daily, longer uses monthly
//
// Never raw measurement — 90d+ raw queries are expensive (Phase 4 D-12 lesson).
//
// # Period-delta (D-03)
//
//   delta_vs_prior — always populated when a prior period exists in the data
//   delta_vs_yoy   — populated only when ≥1 measurement exists in the same
//                     window one year earlier; nil = silent fallback (D-03)
//
// # Single-capability install
//
// When install_identity.capabilities = 'water' (Phase 4 D-09), reports
// silently drop the electricity section; the assembler emits Sections only
// for capability-matching MPs. UI-SPEC empty state covers the zero-MP case.
//
// # Ephemeral artifacts (D-07)
//
// Each report.id has an artifact_dir under /var/lib/shifter/reports/<uuid>/.
// CSV + XLSX written synchronously; PDF arrives async via plan 05-06. River
// PeriodicJob purges expires_at < now() every hour.
//
// # Audit (D-23)
//
// audit.WriteEntry(ctx, tx, …) lands in the same pgx.Tx as the report INSERT.
// Action: report.generate. Entity: report. After: scope, range, group_by.
package report
```

**Step D — `internal/report/assembler.go`:**

```go
package report

import (
    "context"
    "fmt"
    "time"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgtype"

    sqlc "shifter/internal/db/sqlc"
)

// ReportConfig is the validated request payload.
type ReportConfig struct {
    Scope           string                  // "all" | "site" | "meter"
    SiteID          uuid.UUID               // when Scope == "site"
    MeteringPointID uuid.UUID               // when Scope == "meter"
    GroupBy         string                  // "site" | "category" | "none"
    RangeKind       string                  // "daily" | "monthly" | "yearly" | "custom"
    Start           time.Time
    End             time.Time
    Capabilities    string                  // install_identity.capabilities (water|electricity|both)
    Timezone        *time.Location
}

// Report is the assembled in-memory result handed to CSV / Excel / PDF writers.
type Report struct {
    Config     ReportConfig
    Summary    Summary
    PeriodRows []PeriodRow
    MeterRows  []MeterRow
}

type Summary struct {
    TotalConsumption float64
    PriorDelta       *DeltaResult  // nil when no prior period in data
    YoYDelta         *DeltaResult  // nil = silent fallback (D-03)
    PerCategory      map[string]float64  // utility_class → total (only populated when GroupBy = "category")
}

type PeriodRow struct {
    Period         time.Time
    Consumption    float64
    DeltaVsPrior   *DeltaResult
    DeltaVsYoY     *DeltaResult
    SiteID         *uuid.UUID  // populated when GroupBy = "site"
    SiteName       string
    UtilityClass   string      // populated when GroupBy = "category"
}

type MeterRow struct {
    MeteringPointID  uuid.UUID
    Name             string
    UtilityClass     string
    SiteName         string
    Consumption      float64
    DeltaVsPrior     *DeltaResult
}

// BuildReport routes to the right CAGG-backed query and returns the assembled struct.
// Side-effect-free — does NOT INSERT the report row (handler does that).
func BuildReport(ctx context.Context, q *sqlc.Queries, cfg ReportConfig) (*Report, error) {
    rpt := &Report{Config: cfg}

    // 1) Period rows from the CAGG matching RangeKind + scope/group combination.
    if err := rpt.loadPeriodRows(ctx, q); err != nil {
        return nil, fmt.Errorf("load period rows: %w", err)
    }
    // 2) Meter rows for scope=all or scope=site.
    if cfg.Scope != "meter" {
        if err := rpt.loadMeterRows(ctx, q); err != nil {
            return nil, fmt.Errorf("load meter rows: %w", err)
        }
    }
    // 3) Compute summary (total + prior delta + YoY).
    rpt.computeSummary(ctx, q)
    // 4) Compute per-period prior + YoY deltas.
    rpt.computeRowDeltas(ctx, q)

    return rpt, nil
}

// Internal methods below — each routes to one of the 9 sqlc query functions
// based on (RangeKind, Scope, GroupBy). See test cases for the full matrix.
// (Detailed implementations match the sqlc-generated signatures verbatim.)
```

(Implementation body: dispatch on `cfg.RangeKind` and `cfg.GroupBy` to call the matching sqlc query; map rows into `PeriodRow` / `MeterRow`; handle nullable numerics via `pgtype.Numeric → float64` helper. Use the existing `numeric()` helper in `internal/db/`.)

**Step E — `internal/report/delta.go`:**

```go
package report

import (
    "context"
    "time"

    sqlc "shifter/internal/db/sqlc"
)

// DeltaResult expresses period-over-period change. Absolute is in the same
// unit as consumption (m³ for water, kWh for electricity). Percent is rounded
// to 1 decimal — UI displays "+12.3%" / "−8.1%".
type DeltaResult struct {
    Absolute float64
    Percent  float64  // (curr − prior) / prior × 100; prior == 0 → 0.0 (no /0 trap)
}

// ComputeDelta returns nil when prior is nil (no prior period available).
// Otherwise computes (curr − prior).
func ComputeDelta(curr float64, prior *float64) *DeltaResult {
    if prior == nil {
        return nil
    }
    absolute := curr - *prior
    var pct float64
    if *prior != 0 {
        pct = (absolute / *prior) * 100
    }
    return &DeltaResult{Absolute: absolute, Percent: round1(pct)}
}

// ComputeYoY queries the same-window-one-year-earlier consumption. Returns
// nil silently (D-03) when there's no measurement data in that window.
func ComputeYoY(ctx context.Context, q *sqlc.Queries, curr float64, cfg ReportConfig) *DeltaResult {
    priorStart := cfg.Start.AddDate(-1, 0, 0)
    priorEnd := cfg.End.AddDate(-1, 0, 0)
    // Query the same CAGG/route as the current period (helper reused).
    prior, ok := loadConsumptionForRange(ctx, q, cfg, priorStart, priorEnd)
    if !ok {
        return nil  // silent fallback per D-03
    }
    return ComputeDelta(curr, &prior)
}

func round1(v float64) float64 { /* round to 1 decimal */ }
```

**Step F — test bodies:**

Replace `t.Skip` in:

- `internal/report/delta_test.go` (`TestPeriodDelta`):
  1. ComputeDelta(100, nil) → nil
  2. ComputeDelta(110, &100) → {Absolute: 10, Percent: 10.0}
  3. ComputeDelta(100, &0) → {Absolute: 100, Percent: 0.0} (no /0 panic)
  4. ComputeYoY against test fixture where prior-year window has rows → DeltaResult
  5. ComputeYoY against test fixture where prior-year window is empty → nil (silent fallback)

- `internal/report/assembler_test.go` (`TestReportScopeGrouping`):
  1. Seed measurement rows across 2 MPs with utility_class=water and 1 MP with utility_class=electricity, run hourly+daily CAGG refresh.
  2. BuildReport scope=all group=category range=daily → 2 sections (water+electricity)
  3. BuildReport scope=all group=site range=daily → rows grouped per site
  4. BuildReport scope=meter range=monthly → measurement_daily-sourced rows for the single MP
  5. BuildReport with cfg.Capabilities=water → assembler emits ONLY water-class rows (drops electricity silently per D-09 / D-04)
  </action>
  <verify>
    <automated>just sqlc &amp;&amp; go test ./internal/report/... -race -count=1 -short=false -run "TestPeriodDelta|TestReportScopeGrouping" &amp;&amp; go test ./internal/db/... -race -count=1 -short=false -run "TestRunMigrations"</automated>
  </verify>
  <acceptance_criteria>
    - `internal/db/migrations/0030_report.up.sql` exists and contains literal `CHECK (scope IN ('all','site','meter'))` and literal `CHECK (range_end > range_start)`
    - `internal/db/queries/reports.sql` contains literal `-- name: ReportDailyByMP :many`, `-- name: ReportMonthlyBySite :many`, `-- name: ReportYearlyByCategory :many` and 6 more report queries (9 total) + `CreateReport`, `GetReport`, `UpdateReportPDFStatus`, `ListExpiredReports`, `MarkReportExpired`, `ListMetersInScope`
    - `internal/db/sqlc/reports.sql.go` regenerated (exists with sqlc header)
    - `internal/report/doc.go` contains literal `package report` and `delta_vs_yoy` and `silent fallback (D-03)`
    - `internal/report/assembler.go` exports `BuildReport`, `ReportConfig`, `Report`, `Summary`, `PeriodRow`, `MeterRow`
    - `internal/report/delta.go` exports `DeltaResult`, `ComputeDelta`, `ComputeYoY`
    - `ComputeDelta(100, nil)` returns nil — verified in `delta_test.go`
    - `TestReportScopeGrouping` body has at minimum the 5 cases listed (all/site, all/category, site/none, meter/monthly, single-capability filter)
    - NO raw measurement query: `grep -E "FROM measurement\\b" internal/db/queries/reports.sql` returns empty (only measurement_hourly/daily/monthly/yearly)
    - `go test ./internal/report/... -short=false` exits 0
  </acceptance_criteria>
  <done>Report struct assembles from CAGGs, period-delta and YoY math correct including silent fallback.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: CSV writer (UTF-8 BOM, ISO timestamps, metadata header)</name>
  <read_first>
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-CONTEXT.md §D-07 §Claude's Discretion (CSV format spec)
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-UI-SPEC.md §Export Formats — CSV
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-RESEARCH.md §Don't Hand-Roll (BOM one-liner)
    - internal/report/assembler.go (Report struct produced by Task 1)
    - internal/install/finish.go (install_identity shape — display_name, timezone, address)
  </read_first>
  <behavior>
    - Test 1: First three bytes of CSV are 0xEF 0xBB 0xBF (UTF-8 BOM)
    - Test 2: First 3 lines are metadata block: title + generated-at (ISO-8601 with install_identity.timezone offset) + scope description
    - Test 3: Data rows: period (ISO-8601), consumption, unit, delta_vs_prior, delta_vs_yoy
    - Test 4: Empty YoY column is empty string (not "null"); silent fallback rendering
  </behavior>
  <action>
**Step A — `internal/report/csv.go`:**

```go
package report

import (
    "encoding/csv"
    "fmt"
    "io"
    "time"
)

// WriteCSV emits a UTF-8 BOM, a 3-row metadata header, then per-period detail
// rows. REPT-03 contract:
//   - UTF-8 BOM (\xEF\xBB\xBF) — Excel auto-detects encoding when the BOM is
//     present; without it Excel falls back to system locale and mangles non-
//     ASCII characters in metering-point names.
//   - Comma separator.
//   - ISO-8601 timestamps formatted in install_identity.timezone (D-07 / D-03).
//   - Timezone label printed in the metadata header so the customer knows the
//     interpretation when CSV is opened with no metadata context.
type InstallIdentity struct {
    DisplayName string
    Address     string
    Timezone    *time.Location
    Units       string  // "metric" | "imperial" — drives consumption unit label
    LogoPath    string  // used only by PDF writer (plan 05-06)
}

func WriteCSV(w io.Writer, rpt *Report, identity InstallIdentity) error {
    // 1) UTF-8 BOM as raw bytes — MUST be the first bytes emitted, before the
    //    csv.Writer touches the stream.
    if _, err := w.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
        return fmt.Errorf("csv bom: %w", err)
    }

    cw := csv.NewWriter(w)
    defer cw.Flush()

    // 2) 3-row metadata header.
    nowISO := time.Now().In(identity.Timezone).Format(time.RFC3339)
    if err := cw.Write([]string{"Shifter consumption report", identity.DisplayName}); err != nil {
        return err
    }
    if err := cw.Write([]string{"Generated", nowISO, identity.Timezone.String()}); err != nil {
        return err
    }
    if err := cw.Write([]string{"Scope", describeScope(rpt.Config), "Range", rpt.Config.RangeKind}); err != nil {
        return err
    }
    if err := cw.Write([]string{}); err != nil { // blank separator row
        return err
    }

    // 3) Data header row.
    if err := cw.Write([]string{"Period", "Consumption", "Unit", "Δ vs prior", "Δ vs YoY"}); err != nil {
        return err
    }

    // 4) Detail rows.
    for _, row := range rpt.PeriodRows {
        rec := []string{
            row.Period.In(identity.Timezone).Format(time.RFC3339),
            fmt.Sprintf("%.3f", row.Consumption),
            unitLabel(row.UtilityClass, identity.Units),
            deltaString(row.DeltaVsPrior),
            deltaString(row.DeltaVsYoY),  // empty string when nil (silent fallback)
        }
        if err := cw.Write(rec); err != nil {
            return fmt.Errorf("csv row: %w", err)
        }
    }
    return nil
}

func deltaString(d *DeltaResult) string {
    if d == nil {
        return ""
    }
    return fmt.Sprintf("%+.1f%%", d.Percent)
}

func describeScope(cfg ReportConfig) string { /* "All meters", "Site: <name>", "Meter: <name>" */ }
func unitLabel(class, units string) string  { /* "m³" / "L" for water; "kWh" / "Wh" for electricity */ }
```

**Step B — Replace `t.Skip` in `internal/report/csv_test.go`:**

```go
package report

import (
    "bytes"
    "encoding/csv"
    "strings"
    "testing"
    "time"

    "github.com/stretchr/testify/require"
)

func TestCSVFormat(t *testing.T) {
    buf := &bytes.Buffer{}
    bangkok, _ := time.LoadLocation("Asia/Bangkok")
    rpt := &Report{
        Config: ReportConfig{Scope: "meter", RangeKind: "daily", Start: time.Now(), End: time.Now(), Timezone: bangkok},
        PeriodRows: []PeriodRow{
            {Period: time.Date(2026, 5, 1, 0, 0, 0, 0, bangkok), Consumption: 1.234, UtilityClass: "water", DeltaVsPrior: &DeltaResult{Percent: 12.3}, DeltaVsYoY: nil},
            {Period: time.Date(2026, 5, 2, 0, 0, 0, 0, bangkok), Consumption: 1.345, UtilityClass: "water", DeltaVsPrior: &DeltaResult{Percent: -3.1}, DeltaVsYoY: &DeltaResult{Percent: 8.0}},
        },
    }
    identity := InstallIdentity{DisplayName: "Acme", Timezone: bangkok, Units: "metric"}

    require.NoError(t, WriteCSV(buf, rpt, identity))

    raw := buf.Bytes()

    // 1) UTF-8 BOM.
    require.Equal(t, []byte{0xEF, 0xBB, 0xBF}, raw[:3], "must start with UTF-8 BOM")

    // 2) Parse via csv.Reader (after consuming BOM).
    r := csv.NewReader(bytes.NewReader(raw[3:]))
    rows, err := r.ReadAll()
    require.NoError(t, err)
    require.GreaterOrEqual(t, len(rows), 6, "metadata 3 + blank + data header + 2 data rows")

    // 3) Metadata header content.
    require.Equal(t, "Shifter consumption report", rows[0][0])
    require.Equal(t, "Acme", rows[0][1])
    require.Equal(t, "Generated", rows[1][0])
    require.Regexp(t, `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\+\d{2}:\d{2}$`, rows[1][1], "ISO-8601 with tz offset")
    require.Equal(t, "Asia/Bangkok", rows[1][2])

    // 4) Data header.
    require.Equal(t, []string{"Period", "Consumption", "Unit", "Δ vs prior", "Δ vs YoY"}, rows[4])

    // 5) First data row.
    require.Regexp(t, `^2026-05-01T00:00:00\+07:00$`, rows[5][0], "period in install_tz")
    require.Equal(t, "1.234", rows[5][1])
    require.Equal(t, "m³", rows[5][2])
    require.Equal(t, "+12.3%", rows[5][3])
    require.Equal(t, "", rows[5][4], "YoY silent fallback = empty string (D-03)")

    // 6) Second data row YoY present.
    require.Equal(t, "+8.0%", rows[6][4])

    // 7) No commas escaped incorrectly (verify by checking the raw bytes contain only one BOM).
    require.Equal(t, 1, strings.Count(string(raw), "\xEF\xBB\xBF"))
}
```
  </action>
  <verify>
    <automated>go test ./internal/report/... -race -count=1 -short -run TestCSVFormat</automated>
  </verify>
  <acceptance_criteria>
    - `internal/report/csv.go` contains literal `{0xEF, 0xBB, 0xBF}` and literal `time.RFC3339`
    - `internal/report/csv.go` exports `WriteCSV(w io.Writer, rpt *Report, identity InstallIdentity) error`
    - `internal/report/csv.go` exports `InstallIdentity` struct with `DisplayName`, `Address`, `Timezone`, `Units`, `LogoPath`
    - `TestCSVFormat` body asserts BOM = `0xEF 0xBB 0xBF`, ISO-8601 with `+07:00` offset, empty YoY column for silent fallback, header columns exact
    - No `t.Skip` in `internal/report/csv_test.go`
    - `go test ./internal/report/... -run TestCSVFormat -short` exits 0
  </acceptance_criteria>
  <done>CSV writer emits BOM-led UTF-8 with ISO timestamps in install timezone and silent YoY fallback.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 3: Excel writer (3 sheets, formatted dates, bold totals) + HTTP handler scaffold</name>
  <read_first>
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-UI-SPEC.md §Export Formats — Excel
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-CONTEXT.md §Claude's Discretion (Excel content depth)
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-RESEARCH.md §Don't Hand-Roll (excelize already in go.mod)
    - internal/report/assembler.go (Report struct)
    - internal/report/csv.go (shared InstallIdentity)
    - internal/audit/log.go (WriteEntry signature)
    - internal/dashboard/snapshot_handler.go (existing chi handler pattern — auth + JSON)
  </read_first>
  <behavior>
    - Test 1: Excel workbook has exactly 3 sheets named "Summary", "Period Detail", "Meter Detail"
    - Test 2: Period column on "Period Detail" uses Excel date format (numFmt 14 or similar) — NOT a string
    - Test 3: Last row of "Period Detail" is a totals row with bold style + top border
    - Test 4: Column headers include unit in parentheses: "Consumption (m³)" / "Consumption (kWh)"
    - Test 5: POST /api/reports/generate returns 200 + JSON {report_id, summary, period_rows, meter_rows} for a happy path; creates a `report` row + audit entry in one tx
    - Test 6: POST /api/reports/generate rejects invalid scope (400) and missing site_id when scope=site (422)
  </behavior>
  <action>
**Step A — `internal/report/excel.go`:**

```go
package report

import (
    "bytes"
    "fmt"
    "time"

    "github.com/xuri/excelize/v2"
)

// WriteExcel returns an in-memory .xlsx workbook with 3 sheets per UI-SPEC:
//   Summary       — period range + totals + delta tiles
//   Period Detail — per-period breakdown with formatted Excel dates
//   Meter Detail  — per-MP rows when scope = all or site (empty for scope=meter)
//
// excelize/v2 v2.10.0 is already in go.mod (Phase 3 bulk import). RESEARCH §Don't
// Hand-Roll: custom xlsx ZIP is 200+ LoC of manifest math; excelize handles it.
func WriteExcel(rpt *Report, identity InstallIdentity) ([]byte, error) {
    f := excelize.NewFile()
    defer f.Close()

    // Default sheet "Sheet1" → rename to Summary
    if err := f.SetSheetName("Sheet1", "Summary"); err != nil {
        return nil, fmt.Errorf("rename sheet: %w", err)
    }
    if _, err := f.NewSheet("Period Detail"); err != nil {
        return nil, fmt.Errorf("new sheet period: %w", err)
    }
    if _, err := f.NewSheet("Meter Detail"); err != nil {
        return nil, fmt.Errorf("new sheet meter: %w", err)
    }

    // SUMMARY sheet
    f.SetCellValue("Summary", "A1", "Shifter consumption report")
    f.SetCellValue("Summary", "A2", identity.DisplayName)
    f.SetCellValue("Summary", "A3", "Generated")
    f.SetCellValue("Summary", "B3", time.Now().In(identity.Timezone))
    f.SetCellValue("Summary", "C3", identity.Timezone.String())
    f.SetCellValue("Summary", "A5", "Period range")
    f.SetCellValue("Summary", "B5", rpt.Config.Start.In(identity.Timezone))
    f.SetCellValue("Summary", "C5", rpt.Config.End.In(identity.Timezone))
    f.SetCellValue("Summary", "A7", "Total consumption")
    f.SetCellValue("Summary", "B7", rpt.Summary.TotalConsumption)
    if rpt.Summary.PriorDelta != nil {
        f.SetCellValue("Summary", "A8", "Δ vs prior period")
        f.SetCellValue("Summary", "B8", rpt.Summary.PriorDelta.Percent/100)  // store as ratio
        // Apply percent formatter to B8.
    }
    if rpt.Summary.YoYDelta != nil {
        f.SetCellValue("Summary", "A9", "Δ vs same period last year")
        f.SetCellValue("Summary", "B9", rpt.Summary.YoYDelta.Percent/100)
    }

    // Apply Excel date numFmt to B3, B5, C5.
    dateStyle, _ := f.NewStyle(&excelize.Style{NumFmt: 22})  // numFmt 22 = m/d/yyyy h:mm
    f.SetCellStyle("Summary", "B3", "B3", dateStyle)
    f.SetCellStyle("Summary", "B5", "C5", dateStyle)

    // PERIOD DETAIL sheet
    unitLabel := "m³"  // derive from rpt.Config / utility class
    f.SetCellValue("Period Detail", "A1", "Period")
    f.SetCellValue("Period Detail", "B1", fmt.Sprintf("Consumption (%s)", unitLabel))
    f.SetCellValue("Period Detail", "C1", "Δ vs prior")
    f.SetCellValue("Period Detail", "D1", "Δ vs YoY")
    for i, row := range rpt.PeriodRows {
        rowIdx := i + 2
        f.SetCellValue("Period Detail", fmt.Sprintf("A%d", rowIdx), row.Period.In(identity.Timezone))
        f.SetCellValue("Period Detail", fmt.Sprintf("B%d", rowIdx), row.Consumption)
        if row.DeltaVsPrior != nil {
            f.SetCellValue("Period Detail", fmt.Sprintf("C%d", rowIdx), row.DeltaVsPrior.Percent/100)
        }
        if row.DeltaVsYoY != nil {
            f.SetCellValue("Period Detail", fmt.Sprintf("D%d", rowIdx), row.DeltaVsYoY.Percent/100)
        }
        // Apply date style to column A; percent style to C,D.
    }

    // Totals row (bold + top border) at the end of Period Detail.
    totalRow := len(rpt.PeriodRows) + 2
    f.SetCellValue("Period Detail", fmt.Sprintf("A%d", totalRow), "Total")
    f.SetCellFormula("Period Detail", fmt.Sprintf("B%d", totalRow), fmt.Sprintf("SUM(B2:B%d)", totalRow-1))
    totalStyle, _ := f.NewStyle(&excelize.Style{
        Font:   &excelize.Font{Bold: true},
        Border: []excelize.Border{{Type: "top", Color: "000000", Style: 1}},
    })
    f.SetCellStyle("Period Detail", fmt.Sprintf("A%d", totalRow), fmt.Sprintf("D%d", totalRow), totalStyle)

    // METER DETAIL sheet (when scope = all or site)
    if rpt.Config.Scope != "meter" {
        f.SetCellValue("Meter Detail", "A1", "Meter")
        f.SetCellValue("Meter Detail", "B1", "Site")
        f.SetCellValue("Meter Detail", "C1", "Utility")
        f.SetCellValue("Meter Detail", "D1", fmt.Sprintf("Total (%s)", unitLabel))
        f.SetCellValue("Meter Detail", "E1", "Δ vs prior")
        for i, m := range rpt.MeterRows {
            rowIdx := i + 2
            f.SetCellValue("Meter Detail", fmt.Sprintf("A%d", rowIdx), m.Name)
            f.SetCellValue("Meter Detail", fmt.Sprintf("B%d", rowIdx), m.SiteName)
            f.SetCellValue("Meter Detail", fmt.Sprintf("C%d", rowIdx), m.UtilityClass)
            f.SetCellValue("Meter Detail", fmt.Sprintf("D%d", rowIdx), m.Consumption)
            if m.DeltaVsPrior != nil {
                f.SetCellValue("Meter Detail", fmt.Sprintf("E%d", rowIdx), m.DeltaVsPrior.Percent/100)
            }
        }
    }

    f.SetActiveSheet(0)  // Summary

    var buf bytes.Buffer
    if err := f.Write(&buf); err != nil {
        return nil, fmt.Errorf("write xlsx: %w", err)
    }
    return buf.Bytes(), nil
}
```

**Step B — Replace `t.Skip` in `internal/report/excel_test.go`:**

```go
func TestExcelFormat(t *testing.T) {
    bangkok, _ := time.LoadLocation("Asia/Bangkok")
    rpt := &Report{
        Config:  ReportConfig{Scope: "all", RangeKind: "monthly", Start: time.Now().AddDate(0, -1, 0), End: time.Now(), Timezone: bangkok},
        Summary: Summary{TotalConsumption: 123.456, PriorDelta: &DeltaResult{Percent: 12.3}},
        PeriodRows: []PeriodRow{
            {Period: time.Now().AddDate(0, 0, -1), Consumption: 50, DeltaVsPrior: &DeltaResult{Percent: 5}},
        },
        MeterRows: []MeterRow{{Name: "MP-1", SiteName: "HQ", UtilityClass: "water", Consumption: 50}},
    }

    raw, err := WriteExcel(rpt, InstallIdentity{DisplayName: "Acme", Timezone: bangkok, Units: "metric"})
    require.NoError(t, err)

    f, err := excelize.OpenReader(bytes.NewReader(raw))
    require.NoError(t, err)

    // 1) Exactly 3 sheets.
    sheets := f.GetSheetList()
    require.Equal(t, []string{"Summary", "Period Detail", "Meter Detail"}, sheets)

    // 2) Period column has date numFmt — read the cell and assert style has NumFmt set.
    styleIDStr, _ := f.GetCellStyle("Period Detail", "A2")
    require.NotZero(t, styleIDStr, "Period column A must have a date style applied")

    // 3) Header includes unit label.
    header, _ := f.GetCellValue("Period Detail", "B1")
    require.Contains(t, header, "(m³)")

    // 4) Last row is the bold/border totals row.
    totalRow := len(rpt.PeriodRows) + 2
    cellA, _ := f.GetCellValue("Period Detail", fmt.Sprintf("A%d", totalRow))
    require.Equal(t, "Total", cellA)
    cellBStyle, _ := f.GetCellStyle("Period Detail", fmt.Sprintf("B%d", totalRow))
    require.NotZero(t, cellBStyle)
}
```

**Step C — HTTP handler scaffold (`internal/report/handlers.go`):**

Build the synchronous handler for plan 05-06 to extend with the PDF job:

```go
package report

import (
    "context"
    "encoding/json"
    "errors"
    "net/http"
    "os"
    "path/filepath"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"
    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"

    sqlc "shifter/internal/db/sqlc"
    "shifter/internal/audit"
    "shifter/internal/auth"
)

type Deps struct {
    Pool          *pgxpool.Pool
    Queries       *sqlc.Queries
    Identity      InstallIdentity
    ArtifactsRoot string                  // /var/lib/shifter/reports
    EnqueuePDF    func(ctx context.Context, tx pgx.Tx, reportID uuid.UUID, artifactDir string) error
}

const (
    auditActionGenerateReport = "report.generate"
    auditEntityTypeReport     = "report"
)

func GenerateHandler(deps Deps) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        user, ok := auth.UserFromCtx(r.Context())
        if !ok { writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"}); return }

        var req GenerateRequest
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
            writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
            return
        }
        cfg, err := req.ToConfig(deps.Identity.Timezone)
        if err != nil {
            writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
            return
        }

        // Build the report SYNCHRONOUSLY — CSV and Excel must be ready in the
        // same response. PDF is async via deps.EnqueuePDF (plan 05-06 wires
        // River; plan 05-03 ships a no-op fallback so handler tests pass).
        rpt, err := BuildReport(r.Context(), deps.Queries, *cfg)
        if err != nil {
            writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "build_failed"})
            return
        }

        // Tx: insert report row + audit entry + enqueue PDF job (D-23, D-06).
        tx, err := deps.Pool.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
        if err != nil { /* 500 */ return }
        defer tx.Rollback(r.Context())
        q := deps.Queries.WithTx(tx)

        reportID := uuid.New()
        artifactDir := filepath.Join(deps.ArtifactsRoot, reportID.String())
        if err := os.MkdirAll(artifactDir, 0o755); err != nil { /* 500 */ return }
        expiresAt := time.Now().Add(24 * time.Hour)
        if _, err := q.CreateReport(r.Context(), sqlc.CreateReportParams{
            ID: reportID, UserID: user.ID, Scope: cfg.Scope, /* … */
            ArtifactDir: artifactDir, ExpiresAt: expiresAt,
        }); err != nil { /* 500 */ return }

        if err := audit.WriteEntry(r.Context(), tx, audit.Entry{
            UserID: user.ID, Action: auditActionGenerateReport,
            EntityType: auditEntityTypeReport, EntityID: reportID,
            After: map[string]any{"scope": cfg.Scope, "range": cfg.RangeKind, "group_by": cfg.GroupBy},
            RequestID: middleware.GetReqID(r.Context()),
        }); err != nil { /* 500 */ return }

        // Enqueue PDF job inside the same tx. deps.EnqueuePDF is nil-tolerant
        // in plan 05-03 (no-op); plan 05-06 wires the River InsertTx call.
        if deps.EnqueuePDF != nil {
            if err := deps.EnqueuePDF(r.Context(), tx, reportID, artifactDir); err != nil { /* 500 */ return }
        }

        if err := tx.Commit(r.Context()); err != nil { /* 500 */ return }

        // Write CSV + Excel synchronously to disk (already-committed report row
        // is the trigger; PDF lands later via the worker).
        csvPath := filepath.Join(artifactDir, "report.csv")
        f, _ := os.Create(csvPath)
        WriteCSV(f, rpt, deps.Identity)
        f.Close()

        xlsxBytes, _ := WriteExcel(rpt, deps.Identity)
        os.WriteFile(filepath.Join(artifactDir, "report.xlsx"), xlsxBytes, 0o644)

        writeJSON(w, http.StatusOK, GenerateResponse{ReportID: reportID, Report: rpt, PDFStatus: "pending"})
    }
}

func RegisterRoutes(r chi.Router, deps Deps) {
    r.Post("/api/reports/generate", GenerateHandler(deps))
    // GET /api/reports/{id}/file/{kind} is wired in plan 05-06 (download path)
    // because it depends on the PDF artifact lifecycle from the River worker.
}

type GenerateRequest struct {
    Scope         string    `json:"scope"`
    SiteID        string    `json:"site_id,omitempty"`
    MeteringPointID string  `json:"mp_id,omitempty"`
    GroupBy       string    `json:"group,omitempty"`
    Range         string    `json:"range"`
    Start         time.Time `json:"start,omitempty"`
    End           time.Time `json:"end,omitempty"`
}

func (r GenerateRequest) ToConfig(tz *time.Location) (*ReportConfig, error) {
    // Validate scope in {all, site, meter}; range in {daily, monthly, yearly, custom};
    // when scope == site, site_id required; when scope == meter, mp_id required.
    // Return ReportConfig or error with field-specific message.
    return nil, errors.New("TODO")
}

type GenerateResponse struct {
    ReportID  uuid.UUID `json:"report_id"`
    Report    *Report   `json:"report"`
    PDFStatus string    `json:"pdf_status"`
}

func writeJSON(w http.ResponseWriter, status int, body any) { /* helper */ }
```

**Step D — Add new audit constants to `internal/audit/log.go`:**

`ActionGenerateReport = "report.generate"` and `EntityTypeReport = "report"` (after the existing constants block). Update the audit_log CHECK vocabulary migration (0020_audit_log_vocabulary) — OR add a new migration `0031_audit_vocab_phase5` that ALTERs the CHECK constraint to include the new values. Read 0020 first to choose the right approach.

**Step E — Replace `t.Skip` in `internal/report/handlers_test.go`:**

Add at minimum:

1. `TestGenerateHandler_HappyPath_ScopeMeter` — seed measurement rows, POST /api/reports/generate with valid body, expect 200 + JSON containing `report_id` + non-empty `period_rows`; verify `report` row exists in DB + audit entry exists with action=report.generate
2. `TestGenerateHandler_InvalidScope_400`
3. `TestGenerateHandler_MissingSiteID_422`
4. `TestGenerateHandler_AuditInSameTx_RollbackBoth` — inject a tx-failure mock; assert NEITHER report row NOR audit row exists after error
5. `TestGenerateHandler_CSVAndExcelLandOnDisk` — temp dir as ArtifactsRoot; after handler completes, assert `report.csv` and `report.xlsx` exist in `<ArtifactsRoot>/<report_id>/`
  </action>
  <verify>
    <automated>go test ./internal/report/... -race -count=1 -short -run "TestExcelFormat|TestGenerateHandler"</automated>
  </verify>
  <acceptance_criteria>
    - `internal/report/excel.go` exports `WriteExcel(rpt *Report, identity InstallIdentity) ([]byte, error)`
    - `internal/report/excel.go` contains literals `"Summary"`, `"Period Detail"`, `"Meter Detail"`, `f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}`, and `Border: []excelize.Border{{Type: "top"`
    - `TestExcelFormat` body asserts `GetSheetList()` returns exactly `["Summary","Period Detail","Meter Detail"]` in order
    - `internal/report/handlers.go` exports `GenerateHandler`, `RegisterRoutes`, `Deps`, `GenerateRequest`, `GenerateResponse`
    - `internal/report/handlers.go` contains literal `audit.WriteEntry(r.Context(), tx,` (same-tx audit invariant)
    - `internal/report/handlers.go` contains literal `pgx.TxOptions{IsoLevel: pgx.ReadCommitted}` (transactional handler)
    - `internal/audit/log.go` contains literal `ActionGenerateReport = "report.generate"` and `EntityTypeReport = "report"`
    - At least 5 handler tests exist: HappyPath, InvalidScope, MissingSiteID, AuditInSameTx_RollbackBoth, CSVAndExcelLandOnDisk
    - `go test ./internal/report/... -short` exits 0
  </acceptance_criteria>
  <done>Excel writer ships 3-sheet workbook; HTTP handler creates report row + audit in one tx; CSV+XLSX hit disk synchronously.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| Client → /api/reports/generate | JSON body with scope/range/IDs; UUID + enum validation before sqlc binding |
| Handler → CAGG views | sqlc parameter binding only; no raw SQL string concat |
| Handler → /var/lib/shifter/reports/<uuid>/ | Path uses validated UUID and ArtifactsRoot constant; no path traversal vectors |

## STRIDE Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-05-03-01 | Tampering / Injection | SQL injection via scope or range parameters | high | mitigate | All queries are sqlc-generated; parameters passed as typed values; scope + range + group_by validated against enum allowlist in `GenerateRequest.ToConfig` before binding |
| T-05-03-02 | Information Disclosure | Report contains data for sites/MPs the requesting user can't see | medium | mitigate | Authorization check in handler uses existing `auth.Can(user, "report.read", scope)` — Phase 1 AUTH-06 already enforces "viewers can generate reports but admin-only sees admin-MP data"; report data assembled per-user; **defer admin/viewer asymmetry to plan 05-09 (frontend role hiding)** if requirement-narrow per AUTH-06 Phase 1 |
</threat_model>

<verification>
1. `just sqlc` regenerates `internal/db/sqlc/reports.sql.go` cleanly
2. `go test ./internal/report/... -race -count=1 -short` exits 0
3. `go test ./internal/db/... -race -count=1 -short=false -run TestRunMigrations` exits 0 (0030_report migration round-trips)
4. `grep -E "FROM measurement\\b" internal/db/queries/reports.sql` returns empty (only CAGGs, not raw)
5. `grep "0xEF, 0xBB, 0xBF" internal/report/csv.go` returns ≥1 match
6. `grep "audit.WriteEntry(r.Context(), tx" internal/report/handlers.go` returns ≥1 match
</verification>

<success_criteria>
- 9 sqlc-generated report query functions (3 ranges × 3 groupings) live in `internal/db/sqlc/reports.sql.go`
- `Report` struct holds Summary + PeriodRows + MeterRows; assembler routes correctly across all (range, scope, group) tuples
- Period-delta math: prior present, YoY silent fallback (D-03)
- CSV writer: BOM + ISO + metadata header + empty YoY column for silent fallback
- Excel writer: 3 sheets with formatted dates and bold/border totals row
- POST /api/reports/generate: creates report row + audit entry in same tx; writes CSV + XLSX to disk synchronously; returns assembled Report JSON with `pdf_status: "pending"`
</success_criteria>

<output>
After completion, create `.planning/phases/05-aggregates-reports-map-floor-plans/05-03-SUMMARY.md` recording:
- Number of sqlc queries shipped
- Test cases that exercise each (range, scope, group) tuple
- Whether the Excel `NumFmt: 22` date style renders correctly when opened in Excel (manual UAT note)
- Audit vocabulary migration approach taken (ALTER existing 0020 vs new 0031)
- Open question for plan 05-06: should `EnqueuePDF` block in 05-06 land River-only or also surface a synchronous PDF fallback path?
</output>
