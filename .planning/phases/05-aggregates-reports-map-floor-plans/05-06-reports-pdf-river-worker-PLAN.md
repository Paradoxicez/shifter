---
phase: 05-aggregates-reports-map-floor-plans
plan: 06
type: execute
wave: 3
depends_on: [03]
files_modified:
  - internal/report/pdf.go
  - internal/report/pdf_test.go
  - internal/report/pdf_worker.go
  - internal/report/pdf_worker_test.go
  - internal/report/cleanup.go
  - internal/report/download_handler.go
  - internal/report/download_handler_test.go
  - internal/report/handlers.go
  - internal/report/handlers_test.go
  - internal/http/router.go
  - cmd/serve/serve.go
  - compose/bundled.yml
  - compose/external.yml
autonomous: true
requirements: [REPT-05, REPT-06]
threat_refs: [T-05-06-01, T-05-06-02, T-05-06-03]

must_haves:
  truths:
    - "maroto/v2 PDF writer renders the report with header (logo + display_name + address) and footer 'Generated <ISO ts> <install tz> — page N of M' on EVERY page; N and M are correct integers (1-indexed, M = total page count) (D-02)"
    - "PDF generation runs in a River background worker — POST /api/reports/generate enqueues the job via InsertTx inside the same pgx.Tx as the report INSERT + audit row (D-06 + D-23)"
    - "River client started in cmd/serve with workers map containing PDFReportWorker; queue MaxWorkers=4"
    - "Worker queries the assembled Report data, calls WritePDF, writes to <artifact_dir>/report.pdf, updates report.pdf_status='ready' and report.pdf_path"
    - "Failed PDF jobs (River retry exhausted) update report.pdf_status='failed' and surface error in audit log"
    - "GET /api/reports/:id returns the report's pdf_status (pending|running|ready|failed|expired) plus scope + range + created_at so the frontend can poll until the PDF artifact is downloadable"
    - "GET /api/reports/:id/file/{csv|xlsx|pdf} streams the artifact; auth-gated; 404 when report expired or not ready"
    - "ID parameter is UUID-validated before path construction to prevent path traversal (T-05-06-02)"
    - "River PeriodicJob: cleanup every 1 hour scans report.expires_at < now() AND pdf_status <> 'expired'; for each: rm -rf the artifact_dir; UPDATE pdf_status='expired'; audit row (D-07 24h TTL)"
    - "Volume mount for /var/lib/shifter/reports declared in both compose flavors so PDFs survive container restarts"
  artifacts:
    - path: "internal/report/pdf.go"
      provides: "WritePDF(rpt *Report, identity InstallIdentity, logoPath string) ([]byte, error) — maroto v2 builder with RegisterHeader/RegisterFooter and page-N-of-M numbering"
      contains: "maroto.New"
    - path: "internal/report/pdf_worker.go"
      provides: "PDFReportArgs + PDFReportWorker (river.Worker); Work() loads report row, queries data, writes PDF, updates status"
      contains: "func (PDFReportArgs) Kind"
    - path: "internal/report/cleanup.go"
      provides: "CleanupExpiredReportsArgs + worker; PeriodicJob entry registered with River"
      contains: "CleanupExpiredReportsArgs"
    - path: "internal/report/download_handler.go"
      provides: "GET /api/reports/{id}/file/{kind} handler — UUID validation, expiry check, content-type + content-disposition"
      contains: "func DownloadHandler"
    - path: "internal/report/handlers.go"
      provides: "GenerateHandler (POST /api/reports/generate) + StatusHandler (GET /api/reports/{id}) + RegisterRoutes wiring all three endpoints"
      contains: "func StatusHandler"
    - path: "internal/report/handlers_test.go"
      provides: "TestGenerateHandler_EnqueuesPDFJob + TestStatusHandler_PendingReadyFailed + TestStatusHandler_NotFound + TestStatusHandler_BadUUID"
      contains: "TestStatusHandler_PendingReadyFailed"
    - path: "cmd/serve/serve.go"
      provides: "River client construction + worker registration + Start/Stop lifecycle"
      contains: "river.NewClient"
    - path: "compose/bundled.yml"
      provides: "Named volume reports_cache → /var/lib/shifter/reports"
      contains: "reports_cache"
    - path: "compose/external.yml"
      provides: "Same reports_cache volume"
      contains: "reports_cache"
  key_links:
    - from: "internal/report/handlers.go GenerateHandler"
      to: "river.InsertTx(ctx, tx, PDFReportArgs{ReportID, ArtifactDir}, nil)"
      via: "deps.EnqueuePDF function injected at boot"
      pattern: "riverClient\\.InsertTx\\(.*tx,"
    - from: "internal/report/handlers.go StatusHandler"
      to: "deps.Queries.GetReport(ctx, id)"
      via: "GET /api/reports/{id} route"
      pattern: "GetReport\\(r\\.Context\\(\\), id\\)"
    - from: "internal/report/pdf_worker.go Work"
      to: "internal/report/pdf.go WritePDF"
      via: "after re-assembling Report from DB"
      pattern: "WritePDF\\(rpt,"
    - from: "internal/report/download_handler.go"
      to: "<artifact_dir>/<kind>.<ext>"
      via: "filepath.Join(artifactDir, kindToFile[kind])"
      pattern: "filepath\\.Join\\(plan\\.ArtifactDir"
---

<objective>
Finish the report stack: maroto v2 PDF writer with full per-page branding (D-02), River background worker that consumes jobs enqueued by plan 05-03's GenerateHandler, hourly cleanup PeriodicJob that purges artifacts older than 24h (D-07), download handler that streams CSV/XLSX/PDF artifacts with auth + expiry checks, status endpoint (`GET /api/reports/:id`) that plan 05-09 polls, and the cmd/serve wiring that boots the River client at server start. Adds the `/var/lib/shifter/reports` volume mount to both compose flavors.

Purpose: Plan 05-03 produces CSV + Excel synchronously; this plan delivers the asynchronous PDF path (REPT-05/06), the status-polling endpoint that the frontend (plan 05-09) consumes via `useReportPDFStatus`, and the unified download endpoint. After this plan: the entire reports server stack works end-to-end; frontend lands in plan 05-09.

Output: 4 new Go files (pdf, pdf_worker, cleanup, download_handler) + updates to handlers.go (wire EnqueuePDF + add StatusHandler) + handlers_test.go (4 new tests) + cmd/serve.go (start River) + both compose flavors (reports_cache volume).
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/05-aggregates-reports-map-floor-plans/05-CONTEXT.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-RESEARCH.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-UI-SPEC.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-03-reports-backend-csv-excel-PLAN.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-09-reports-frontend-PLAN.md
@internal/report/handlers.go
@internal/report/assembler.go
@internal/report/csv.go
@internal/report/excel.go
@internal/audit/log.go
@cmd/serve/serve.go
@compose/bundled.yml
@compose/external.yml

<interfaces>
<!-- From plan 05-03: GenerateHandler accepts deps.EnqueuePDF; this plan wires it -->
```go
type Deps struct {
    Pool          *pgxpool.Pool
    Queries       *sqlc.Queries
    Identity      InstallIdentity
    ArtifactsRoot string
    EnqueuePDF    func(ctx context.Context, tx pgx.Tx, reportID uuid.UUID, artifactDir string) error  // ← this plan supplies a non-nil impl
}
```

<!-- Status response shape — consumed by plan 05-09 useReportPDFStatus.
     The frontend's `ReportStatus` type in 05-09 <interfaces> block matches this verbatim. -->
```go
type StatusResponse struct {
    ID        uuid.UUID `json:"id"`
    PdfStatus string    `json:"pdf_status"`            // pending | running | ready | failed | expired
    PdfPath   *string   `json:"pdf_path,omitempty"`    // present when status == ready
    CreatedAt time.Time `json:"created_at"`
    Scope     string    `json:"scope"`                 // all | site | meter
    Range     string    `json:"range"`                 // daily | monthly | yearly | custom
}
```

<!-- River client construction (per RESEARCH §River — Background Job Queue Setup) -->
```go
workers := river.NewWorkers()
river.AddWorker(workers, &PDFReportWorker{deps: workerDeps})
river.AddWorker(workers, &CleanupExpiredReportsWorker{deps: workerDeps})

periodicJobs := []*river.PeriodicJob{
    river.NewPeriodicJob(
        river.PeriodicInterval(1*time.Hour),
        func() (river.JobArgs, *river.InsertOpts) { return CleanupExpiredReportsArgs{}, nil },
        &river.PeriodicJobOpts{RunOnStart: false},
    ),
}

riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
    Queues: map[string]river.QueueConfig{ river.QueueDefault: {MaxWorkers: 4} },
    Workers: workers,
    PeriodicJobs: periodicJobs,
})
```
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: maroto v2 PDF writer (header + footer + summary + detail)</name>
  <read_first>
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-CONTEXT.md §D-01 §D-02
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-RESEARCH.md §PDF Generation — maroto v2 §Common Pitfalls #10 §Open Questions #3
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-UI-SPEC.md §Report Layout Spec
    - internal/report/assembler.go (Report struct shape)
    - internal/report/csv.go (InstallIdentity shape — shared)
  </read_first>
  <behavior>
    - Test 1: WritePDF returns non-empty []byte that parses as PDF (`%PDF-` magic bytes)
    - Test 2: Header includes display_name AND address strings on every page (probe via PDF text extract or by re-rendering and counting occurrences)
    - Test 3: Footer includes `Generated <ISO ts>` AND `page N of M` text where N and M are correct integers (1-indexed, M = total page count). Implementation note: try maroto v2's `{number}/{total}` placeholders first; if the test detects unsubstituted braces in output, fall back to a two-pass approach (RESEARCH Open Q #3) — the user-observable assertion is "correct N of M on every page", not which substitution mechanism produced it.
    - Test 4: When identity.LogoPath is empty, the PDF still renders (no panic) and the header uses text-only branding
    - Test 5: When report has >50 PeriodRows, the PDF is multi-page and the footer "page N of M" updates per page (every page shows its own N, identical M)
    - Test 6: LowMemory mode option compiles and runs (will use it for large reports)
  </behavior>
  <action>
**Step A — `internal/report/pdf.go`:**

```go
package report

import (
    "errors"
    "fmt"
    "os"
    "time"

    "github.com/johnfercher/maroto/v2"
    "github.com/johnfercher/maroto/v2/pkg/components/col"
    "github.com/johnfercher/maroto/v2/pkg/components/image"
    "github.com/johnfercher/maroto/v2/pkg/components/line"
    "github.com/johnfercher/maroto/v2/pkg/components/row"
    "github.com/johnfercher/maroto/v2/pkg/components/text"
    "github.com/johnfercher/maroto/v2/pkg/config"
    "github.com/johnfercher/maroto/v2/pkg/consts/align"
    "github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
    "github.com/johnfercher/maroto/v2/pkg/consts/generation"
    "github.com/johnfercher/maroto/v2/pkg/props"
)

// WritePDF generates a multi-page report PDF using maroto/v2 with branded
// header + footer on every page (D-02). When the report has >50 period rows
// the generation runs in LowMemory mode to avoid holding the full document
// in RAM.
//
// Pitfall #10 (RESEARCH): header area > page area returns an error from
// maroto. Header rows are kept to ≤ 25% of A4 (4 rows × 8mm = 32mm; A4
// usable height ≈ 257mm).
//
// IMPLEMENTATION NOTE — page-N-of-M numbering (RESEARCH Open Q #3):
// The user-observable contract is: every page footer shows "page N of M"
// with correct integers (1-indexed; M = total page count). Try maroto v2's
// `{number}` / `{total}` WithPageNumber placeholders first (documented).
// If the unit test detects unsubstituted braces in the output bytes
// (TestPDFBranding asserts no literal `{number}` or `{total}` remains),
// implement the two-pass fallback inside WritePDF: generate once with a
// stub footer to count pages via doc.GetPages(), then regenerate with a
// per-page-aware footer using literal computed strings. The choice is
// invisible to consumers — both paths satisfy the same must_have truth.
func WritePDF(rpt *Report, identity InstallIdentity) ([]byte, error) {
    cfg := buildConfig(rpt)
    m := maroto.New(cfg)

    if err := registerHeader(m, identity); err != nil {
        return nil, fmt.Errorf("register header: %w", err)
    }
    if err := registerFooter(m, identity); err != nil {
        return nil, fmt.Errorf("register footer: %w", err)
    }

    // Summary section
    m.AddRow(8, text.NewCol(12, "Shifter consumption report", props.Text{Size: 14, Style: fontstyle.Bold}))
    m.AddRow(6, text.NewCol(12, describeScope(rpt.Config), props.Text{Size: 10, Color: &props.Color{Red: 80, Green: 80, Blue: 80}}))
    m.AddRow(8, text.NewCol(12, fmt.Sprintf("Total: %.3f %s", rpt.Summary.TotalConsumption, unitLabel("", identity.Units)),
        props.Text{Size: 12}))
    if rpt.Summary.PriorDelta != nil {
        m.AddRow(6, text.NewCol(12, fmt.Sprintf("Δ vs prior period: %+.1f%%", rpt.Summary.PriorDelta.Percent), props.Text{Size: 10}))
    }
    if rpt.Summary.YoYDelta != nil {
        m.AddRow(6, text.NewCol(12, fmt.Sprintf("Δ vs same period last year: %+.1f%%", rpt.Summary.YoYDelta.Percent), props.Text{Size: 10}))
    }

    // Spacer
    m.AddRow(4)

    // Period detail table — section heading
    m.AddRow(8, text.NewCol(12, "Per-period breakdown", props.Text{Size: 12, Style: fontstyle.Bold}))
    m.AddRow(6,
        text.NewCol(4, "Period",        props.Text{Style: fontstyle.Bold}),
        text.NewCol(3, "Consumption",   props.Text{Style: fontstyle.Bold, Align: align.Right}),
        text.NewCol(2, "Δ prior",       props.Text{Style: fontstyle.Bold, Align: align.Right}),
        text.NewCol(2, "Δ YoY",         props.Text{Style: fontstyle.Bold, Align: align.Right}),
    )
    for _, prow := range rpt.PeriodRows {
        m.AddRow(5,
            text.NewCol(4, prow.Period.In(identity.Timezone).Format("2006-01-02 15:04")),
            text.NewCol(3, fmt.Sprintf("%.3f", prow.Consumption), props.Text{Align: align.Right}),
            text.NewCol(2, deltaText(prow.DeltaVsPrior), props.Text{Align: align.Right}),
            text.NewCol(2, deltaText(prow.DeltaVsYoY), props.Text{Align: align.Right}),
        )
    }

    if rpt.Config.Scope != "meter" && len(rpt.MeterRows) > 0 {
        m.AddRow(4)
        m.AddRow(8, text.NewCol(12, "Per-meter rollup", props.Text{Size: 12, Style: fontstyle.Bold}))
        m.AddRow(6,
            text.NewCol(4, "Meter",       props.Text{Style: fontstyle.Bold}),
            text.NewCol(3, "Site",        props.Text{Style: fontstyle.Bold}),
            text.NewCol(2, "Utility",     props.Text{Style: fontstyle.Bold}),
            text.NewCol(3, "Total",       props.Text{Style: fontstyle.Bold, Align: align.Right}),
        )
        for _, mrow := range rpt.MeterRows {
            m.AddRow(5,
                text.NewCol(4, mrow.Name),
                text.NewCol(3, mrow.SiteName),
                text.NewCol(2, mrow.UtilityClass),
                text.NewCol(3, fmt.Sprintf("%.3f", mrow.Consumption), props.Text{Align: align.Right}),
            )
        }
    }

    doc, err := m.Generate()
    if err != nil { return nil, fmt.Errorf("maroto generate: %w", err) }
    return doc.GetBytes(), nil
}

func buildConfig(rpt *Report) *config.Config {
    b := config.NewBuilder()
    // RESEARCH §PDF Generation: LowMemory for large reports
    if len(rpt.PeriodRows)+len(rpt.MeterRows) > 50 {
        b = b.WithGenerationMode(generation.LowMemory)
    }
    return b.WithMargins(15, 18, 15).WithPageNumber("{number} of {total}", props.PageNumber{Place: props.Bottom}).Build()
    // {number}/{total} is maroto v2's documented page-number placeholder (verify against pkg.go.dev).
    // If the test detects unrendered braces, switch to the manual two-pass approach (see WritePDF docstring).
}

func registerHeader(m maroto.Maroto, identity InstallIdentity) error {
    rows := []row.Row{}
    // Logo + name left, address right
    if identity.LogoPath != "" {
        if _, err := os.Stat(identity.LogoPath); err == nil {
            rows = append(rows, row.New(10).Add(
                image.NewFromFileCol(2, identity.LogoPath, props.Rect{Center: false}),
                text.NewCol(6, identity.DisplayName, props.Text{Size: 12, Style: fontstyle.Bold}),
                text.NewCol(4, identity.Address, props.Text{Size: 9, Align: align.Right}),
            ))
        }
    }
    if len(rows) == 0 {
        rows = append(rows, row.New(10).Add(
            text.NewCol(8, identity.DisplayName, props.Text{Size: 12, Style: fontstyle.Bold}),
            text.NewCol(4, identity.Address, props.Text{Size: 9, Align: align.Right}),
        ))
    }
    // Navy rule
    rows = append(rows, row.New(1).Add(line.NewCol(12, props.Line{Color: &props.Color{Red: 28, Green: 47, Blue: 109}, Thickness: 0.5})))
    return m.RegisterHeader(rows...)
}

func registerFooter(m maroto.Maroto, identity InstallIdentity) error {
    ts := time.Now().In(identity.Timezone).Format(time.RFC3339)
    // "Generated <ts> <tz> — page N of M" — the {number}/{total} substitution
    // turns into 1-indexed N and total M on every page. Fallback path
    // documented in WritePDF docstring (RESEARCH Open Q #3).
    return m.RegisterFooter(row.New(5).Add(
        text.NewCol(12, fmt.Sprintf("Generated %s %s — page {number} of {total}", ts, identity.Timezone.String()),
            props.Text{Size: 8, Color: &props.Color{Red: 100, Green: 100, Blue: 100}}),
    ))
}

func deltaText(d *DeltaResult) string {
    if d == nil { return "" }
    return fmt.Sprintf("%+.1f%%", d.Percent)
}
```

**Step B — Replace `t.Skip` in `internal/report/pdf_test.go`:**

```go
func TestPDFBranding(t *testing.T) {
    bangkok, _ := time.LoadLocation("Asia/Bangkok")
    rpt := &Report{
        Config:  ReportConfig{Scope: "meter", RangeKind: "daily", Start: time.Now(), End: time.Now(), Timezone: bangkok},
        Summary: Summary{TotalConsumption: 123.456, PriorDelta: &DeltaResult{Percent: 5}, YoYDelta: nil},
        PeriodRows: []PeriodRow{ {Period: time.Now(), Consumption: 50} },
    }
    identity := InstallIdentity{DisplayName: "Acme Co", Address: "123 Main St", Timezone: bangkok}

    raw, err := WritePDF(rpt, identity)
    require.NoError(t, err)
    require.NotEmpty(t, raw)

    // 1) Magic bytes
    require.Equal(t, []byte("%PDF-"), raw[:5], "must be a PDF")

    // 2) Content contains display name + address (decoded from PDF stream).
    rawStr := string(raw)
    require.Contains(t, rawStr, "Acme Co", "header must include display_name")
    require.Contains(t, rawStr, "123 Main St", "header must include address")
    require.Contains(t, rawStr, "Generated", "footer must include Generated label")
    require.Contains(t, rawStr, "Asia/Bangkok", "footer must include install timezone")
    require.Contains(t, rawStr, "page 1 of 1", "single-page report must show 'page 1 of 1' (M = total page count)")

    // 3) No literal unsubstituted braces — either maroto substituted or the
    //    two-pass fallback replaced them. The must_have is "correct N of M",
    //    not which mechanism produced it.
    require.NotContains(t, rawStr, "{number}", "page-number placeholder must be substituted (native or two-pass fallback)")
    require.NotContains(t, rawStr, "{total}", "page-total placeholder must be substituted (native or two-pass fallback)")
}

func TestPDFBranding_NoLogo_StillRenders(t *testing.T) {
    identity := InstallIdentity{DisplayName: "Acme", Timezone: time.UTC}  // LogoPath empty
    _, err := WritePDF(&Report{Config: ReportConfig{Scope: "all", Timezone: time.UTC}}, identity)
    require.NoError(t, err)
}

func TestPDFBranding_MultiPage(t *testing.T) {
    bangkok, _ := time.LoadLocation("Asia/Bangkok")
    rows := make([]PeriodRow, 100)
    for i := range rows {
        rows[i] = PeriodRow{Period: time.Now().Add(time.Duration(-i) * 24 * time.Hour), Consumption: float64(i)}
    }
    raw, err := WritePDF(&Report{Config: ReportConfig{Scope: "meter", Timezone: bangkok}, PeriodRows: rows}, InstallIdentity{DisplayName: "Acme", Timezone: bangkok})
    require.NoError(t, err)
    rawStr := string(raw)
    // Multi-page PDFs have multiple `/Type /Page` entries.
    pageCount := strings.Count(rawStr, "/Type /Page\n") + strings.Count(rawStr, "/Type /Page>>")
    require.GreaterOrEqual(t, pageCount, 2, "100 rows should span 2+ pages")
    // Every page footer must report the correct M (= pageCount) and an N that varies per page.
    require.Contains(t, rawStr, fmt.Sprintf("page 1 of %d", pageCount), "page 1 footer must show 'page 1 of M'")
    require.Contains(t, rawStr, fmt.Sprintf("page 2 of %d", pageCount), "page 2 footer must show 'page 2 of M'")
}
```

**Note for the executor:** If `TestPDFBranding`'s page-placeholder assertion fails (`{number}` / `{total}` not substituted by maroto natively), implement the two-pass fallback in WritePDF:

1. Generate once with `WithPageNumber("placeholder")` to count pages via doc.GetPages()
2. For the real pass, build the footer with a per-page-aware row that uses literal computed strings (e.g. `fmt.Sprintf("page %d of %d", n, total)`) rather than maroto placeholders
3. Document the chosen path in SUMMARY.md

The fallback is invasive — only implement if the first-attempt placeholder doesn't work. Either way, the test asserts the user-observable behavior (correct N of M on every page) and ignores the mechanism.
  </action>
  <verify>
    <automated>go test ./internal/report/... -race -count=1 -short -run "TestPDFBranding"</automated>
  </verify>
  <acceptance_criteria>
    - `internal/report/pdf.go` contains literal `maroto.New(cfg)`, `m.RegisterHeader`, `m.RegisterFooter`, `generation.LowMemory`
    - `internal/report/pdf.go` exports `WritePDF(rpt *Report, identity InstallIdentity) ([]byte, error)`
    - `internal/report/pdf.go` reads identity.LogoPath via `os.Stat` and gracefully degrades to text-only header when LogoPath empty or file missing
    - `TestPDFBranding` asserts PDF magic bytes AND display_name AND address AND timezone AND `page 1 of 1` literal AND no unsubstituted `{number}`/`{total}` placeholders
    - `TestPDFBranding_MultiPage` asserts 100 rows produce ≥ 2 pages AND `page 1 of M` AND `page 2 of M` literals present in output
    - `go test ./internal/report/... -run TestPDFBranding -short` exits 0
  </acceptance_criteria>
  <done>maroto v2 PDF writer produces branded multi-page reports with correct "page N of M" footer on every page (native placeholder or two-pass fallback — invisible to consumers).</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: River worker + InsertTx wiring + StatusHandler + cmd/serve River bootstrap</name>
  <read_first>
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-RESEARCH.md §River — Background Job Queue Setup
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-09-reports-frontend-PLAN.md (consumer of GET /api/reports/:id — see <interfaces> ReportStatus shape and useReportPDFStatus poll pattern)
    - internal/report/handlers.go (GenerateHandler deps.EnqueuePDF hook from plan 05-03)
    - internal/report/pdf.go (WritePDF signature)
    - cmd/serve/serve.go (existing service boot sequence)
    - internal/db/migrations/0024_river_tables.up.sql (River schema embedded in plan 05-01)
  </read_first>
  <behavior>
    - Test 1: PDFReportArgs.Kind() returns "pdf_report"
    - Test 2: Worker.Work loads the report row by ID, rebuilds the Report struct (re-runs BuildReport), calls WritePDF, writes `<artifact_dir>/report.pdf`, UPDATEs report.pdf_status='ready' and pdf_path
    - Test 3: Worker on transient DB error retries; on max retry → UPDATE pdf_status='failed'
    - Test 4: GenerateHandler InsertTx enqueues job in same tx as report INSERT — verified by injecting a tx-level failure and asserting no job in river_job table AND no report row
    - Test 5: cmd/serve constructs the River client AND registers PDFReportWorker AND starts the client
    - Test 6: River client shutdown is honored on server context cancel
    - Test 7 (StatusHandler — pending/ready/failed): seed three report rows with pdf_status in {pending, ready, failed}; GET /api/reports/:id returns 200 JSON with the matching pdf_status, pdf_path (present iff ready), created_at, scope, range
    - Test 8 (StatusHandler — 404): GET /api/reports/:id for a UUID with no row → 404
    - Test 9 (StatusHandler — 400 bad UUID): GET /api/reports/not-a-uuid → 400 invalid_report_id
    - Test 10 (StatusHandler — auth): viewers can read their own reports; admins can read all; unauthenticated → 401 (same gating as DownloadHandler)
  </behavior>
  <action>
**Step A — `internal/report/pdf_worker.go`:**

```go
package report

import (
    "context"
    "fmt"
    "os"
    "path/filepath"
    "time"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/riverqueue/river"
    "log/slog"

    sqlc "shifter/internal/db/sqlc"
)

// PDFReportArgs identifies the report to render. Kept minimal — all the
// detail is rebuilt from the DB row inside Work() so the job arg JSON
// stays small (River truncates very long arg payloads).
type PDFReportArgs struct {
    ReportID uuid.UUID `json:"report_id"`
}

func (PDFReportArgs) Kind() string { return "pdf_report" }

// PDFReportWorker generates the PDF artifact and updates report.pdf_status.
// Retries: 3 attempts; final failure marks pdf_status='failed' (handled in
// AfterJobFailed hook via the River client config).
type PDFReportWorker struct {
    river.WorkerDefaults[PDFReportArgs]

    Pool     *pgxpool.Pool
    Queries  *sqlc.Queries
    Identity InstallIdentityProvider
    Log      *slog.Logger
}

type InstallIdentityProvider interface {
    Load(ctx context.Context) (InstallIdentity, error)
}

func (w *PDFReportWorker) Work(ctx context.Context, job *river.Job[PDFReportArgs]) error {
    // 1) Mark running.
    if _, err := w.Pool.Exec(ctx, `UPDATE report SET pdf_status = 'running' WHERE id = $1`, job.Args.ReportID); err != nil {
        return fmt.Errorf("mark running: %w", err)
    }

    // 2) Load report row to recover scope + range + artifact_dir.
    plan, err := w.Queries.GetReport(ctx, job.Args.ReportID)
    if err != nil {
        return fmt.Errorf("load report row: %w", err)
    }

    identity, err := w.Identity.Load(ctx)
    if err != nil { return fmt.Errorf("load identity: %w", err) }

    cfg := planRowToConfig(plan, identity.Timezone)
    rpt, err := BuildReport(ctx, w.Queries, cfg)
    if err != nil {
        return fmt.Errorf("build report: %w", err)
    }

    pdfBytes, err := WritePDF(rpt, identity)
    if err != nil {
        return fmt.Errorf("write pdf: %w", err)
    }

    pdfPath := filepath.Join(plan.ArtifactDir, "report.pdf")
    if err := os.WriteFile(pdfPath, pdfBytes, 0o644); err != nil {
        return fmt.Errorf("write pdf file: %w", err)
    }

    if err := w.Queries.UpdateReportPDFStatus(ctx, sqlc.UpdateReportPDFStatusParams{
        ID: job.Args.ReportID, PdfStatus: "ready", PdfPath: pgxString(pdfPath),
    }); err != nil {
        return fmt.Errorf("update pdf_status ready: %w", err)
    }
    w.Log.Info("report.pdf.ready", "report_id", job.Args.ReportID, "bytes", len(pdfBytes), "path", pdfPath)
    return nil
}

// AfterJobFailed marks failed status when River exhausts retries.
// Wire via river.Config.ErrorHandler — see cmd/serve.
```

**Step B — `internal/report/cleanup.go`:**

```go
package report

import (
    "context"
    "fmt"
    "os"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/riverqueue/river"
    "log/slog"

    sqlc "shifter/internal/db/sqlc"
    "shifter/internal/audit"
)

type CleanupExpiredReportsArgs struct{}

func (CleanupExpiredReportsArgs) Kind() string { return "report_cleanup" }

type CleanupExpiredReportsWorker struct {
    river.WorkerDefaults[CleanupExpiredReportsArgs]
    Pool    *pgxpool.Pool
    Queries *sqlc.Queries
    Log     *slog.Logger
}

func (w *CleanupExpiredReportsWorker) Work(ctx context.Context, job *river.Job[CleanupExpiredReportsArgs]) error {
    expired, err := w.Queries.ListExpiredReports(ctx)
    if err != nil { return fmt.Errorf("list expired: %w", err) }
    for _, e := range expired {
        if err := os.RemoveAll(e.ArtifactDir); err != nil {
            w.Log.Warn("cleanup: removeAll failed", "id", e.ID, "err", err)
            // continue — DB mark as expired anyway so we don't retry forever
        }
        if err := w.Queries.MarkReportExpired(ctx, e.ID); err != nil {
            w.Log.Warn("cleanup: mark expired failed", "id", e.ID, "err", err)
        }
    }
    w.Log.Info("report.cleanup.cycle", "expired_count", len(expired))
    return nil
}
```

**Step C — `internal/report/download_handler.go`:**

```go
package report

import (
    "errors"
    "fmt"
    "io"
    "net/http"
    "os"
    "path/filepath"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"

    "shifter/internal/auth"
)

// DownloadHandler — GET /api/reports/{id}/file/{kind}
//
// kind ∈ {csv, xlsx, pdf}. Resolves to <artifact_dir>/report.<ext>.
// 401 if unauthenticated, 404 if report not found OR expired, 425 (Too Early)
// if kind=pdf and pdf_status != 'ready'.
//
// UUID validation is mandatory before path construction (T-05-06-02): an
// invalid UUID returns 400 BEFORE filepath.Join is called.
func DownloadHandler(deps Deps) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        if _, ok := auth.UserFromCtx(r.Context()); !ok {
            writeError(w, 401, "unauthorized"); return
        }
        idStr := chi.URLParam(r, "id")
        id, err := uuid.Parse(idStr)
        if err != nil { writeError(w, 400, "invalid_report_id"); return }
        kind := chi.URLParam(r, "kind")
        ext, contentType, ok := kindToExt(kind)
        if !ok { writeError(w, 400, "invalid_kind"); return }

        plan, err := deps.Queries.GetReport(r.Context(), id)
        if errors.Is(err, pgx.ErrNoRows) { writeError(w, 404, "report_not_found"); return }
        if err != nil { writeError(w, 500, "lookup_failed"); return }

        if time.Now().After(plan.ExpiresAt) || plan.PdfStatus == "expired" {
            writeError(w, 404, "report_expired"); return
        }

        if kind == "pdf" && plan.PdfStatus != "ready" {
            writeError(w, 425, "pdf_not_ready"); return
        }

        absPath := filepath.Join(plan.ArtifactDir, fmt.Sprintf("report.%s", ext))
        f, err := os.Open(absPath)
        if err != nil { writeError(w, 404, "file_missing"); return }
        defer f.Close()

        w.Header().Set("Content-Type", contentType)
        w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="shifter-report-%s.%s"`, id.String()[:8], ext))
        _, _ = io.Copy(w, f)
    }
}

func kindToExt(kind string) (ext, contentType string, ok bool) {
    switch kind {
    case "csv":  return "csv",  "text/csv; charset=utf-8", true
    case "xlsx": return "xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", true
    case "pdf":  return "pdf",  "application/pdf", true
    default:     return "", "", false
    }
}
```

**Step D — `internal/report/handlers.go`: add `StatusHandler` AND wire RegisterRoutes:**

```go
// StatusResponse mirrors plan 05-09's frontend ReportStatus shape verbatim.
// The frontend polls this endpoint via useReportPDFStatus until pdf_status
// flips to 'ready' or 'failed'.
type StatusResponse struct {
    ID        uuid.UUID `json:"id"`
    PdfStatus string    `json:"pdf_status"`            // pending | running | ready | failed | expired
    PdfPath   *string   `json:"pdf_path,omitempty"`    // present when status == ready
    CreatedAt time.Time `json:"created_at"`
    Scope     string    `json:"scope"`                 // all | site | meter
    Range     string    `json:"range"`                 // daily | monthly | yearly | custom
}

// StatusHandler — GET /api/reports/{id}
//
// Returns the report's current pdf_status + metadata so plan 05-09's
// useReportPDFStatus poll can stop firing once status is ready/failed/expired.
//
// Auth: same gating as DownloadHandler — viewers see their own reports,
// admins see all, unauthenticated returns 401. UUID is validated BEFORE
// any DB lookup (defense in depth — even though the query itself is
// parameterised, rejecting bad UUIDs at the edge keeps the 404 path
// reserved for "row truly absent").
func StatusHandler(deps Deps) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        user, ok := auth.UserFromCtx(r.Context())
        if !ok {
            writeError(w, 401, "unauthorized"); return
        }
        idStr := chi.URLParam(r, "id")
        id, err := uuid.Parse(idStr)
        if err != nil {
            writeError(w, 400, "invalid_report_id"); return
        }

        plan, err := deps.Queries.GetReport(r.Context(), id)
        if errors.Is(err, pgx.ErrNoRows) {
            writeError(w, 404, "report_not_found"); return
        }
        if err != nil {
            writeError(w, 500, "lookup_failed"); return
        }

        // Auth scope: viewers can only see their own reports; admins all.
        // Mirrors DownloadHandler's enforcement so polling and downloading
        // share one rule.
        if user.Role != "admin" && plan.CreatedBy != user.ID {
            writeError(w, 404, "report_not_found"); return  // 404 not 403 to avoid disclosing existence
        }

        resp := StatusResponse{
            ID:        plan.ID,
            PdfStatus: plan.PdfStatus,
            CreatedAt: plan.CreatedAt,
            Scope:     plan.Scope,
            Range:     plan.RangeKind,
        }
        if plan.PdfStatus == "ready" && plan.PdfPath.Valid {
            v := plan.PdfPath.String
            resp.PdfPath = &v
        }
        writeJSON(w, 200, resp)
    }
}

func RegisterRoutes(r chi.Router, deps Deps) {
    r.Post("/api/reports/generate",        GenerateHandler(deps))
    r.Get("/api/reports/{id}",             StatusHandler(deps))   // ← consumed by plan 05-09 useReportPDFStatus
    r.Get("/api/reports/{id}/file/{kind}", DownloadHandler(deps))
}
```

**Step E — Wire River client in `cmd/serve/serve.go`:**

After the pool is built and migrations run, BEFORE the HTTP server starts:

```go
// River background-job client (Phase 5 REPT-06 + report cleanup PeriodicJob).
workers := river.NewWorkers()
river.AddWorker(workers, &report.PDFReportWorker{
    Pool: pool, Queries: q, Identity: installIdentityProvider, Log: log.With("component", "pdf_worker"),
})
river.AddWorker(workers, &report.CleanupExpiredReportsWorker{
    Pool: pool, Queries: q, Log: log.With("component", "report_cleanup"),
})

periodicJobs := []*river.PeriodicJob{
    river.NewPeriodicJob(
        river.PeriodicInterval(1*time.Hour),
        func() (river.JobArgs, *river.InsertOpts) { return report.CleanupExpiredReportsArgs{}, nil },
        &river.PeriodicJobOpts{RunOnStart: false},
    ),
}

riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
    Queues:       map[string]river.QueueConfig{ river.QueueDefault: {MaxWorkers: 4} },
    Workers:      workers,
    PeriodicJobs: periodicJobs,
})
if err != nil { return fmt.Errorf("river NewClient: %w", err) }

// EnqueuePDF closure for report handler deps.
reportDeps := report.Deps{
    Pool: pool, Queries: q, Identity: identity, ArtifactsRoot: cfg.ReportsRoot,
    EnqueuePDF: func(ctx context.Context, tx pgx.Tx, reportID uuid.UUID, artifactDir string) error {
        _, err := riverClient.InsertTx(ctx, tx, report.PDFReportArgs{ReportID: reportID}, nil)
        return err
    },
}

go func() {
    if err := riverClient.Start(ctx); err != nil { log.Error("river start", "err", err) }
}()
defer func() {
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    _ = riverClient.Stop(shutdownCtx)
}()
```

**Step F — Compose volume mount for `/var/lib/shifter/reports`:**

Both `compose/bundled.yml` and `compose/external.yml`:
- Add `reports_cache:` to top-level `volumes:` block
- Add `- reports_cache:/var/lib/shifter/reports` to the `shifter` service's volumes list

**Step G — Replace `t.Skip` in `internal/report/pdf_worker_test.go`:**

```go
func TestPDFJob(t *testing.T) {
    // 1) Setup: testcontainer Postgres with River schema migrated via golang-migrate (plan 05-01 0024).
    // 2) Insert a report row into `report` with artifact_dir = t.TempDir()/<uuid>.
    // 3) Build a PDFReportWorker with the test pool + queries + identity provider mock.
    // 4) Run worker.Work(ctx, &river.Job[PDFReportArgs]{Args: PDFReportArgs{ReportID: reportID}}).
    // 5) Assert: report.pdf_status == 'ready', report.pdf_path != null, file exists at <artifact_dir>/report.pdf with %PDF- prefix.
    require.NoError(t, worker.Work(ctx, job))
    plan, _ := q.GetReport(ctx, reportID)
    require.Equal(t, "ready", plan.PdfStatus)
    pdfBytes, _ := os.ReadFile(plan.PdfPath.String)
    require.Equal(t, []byte("%PDF-"), pdfBytes[:5])
}

func TestEnqueuePDF_AtomicWithReport(t *testing.T) {
    // POST /api/reports/generate with a tx-level injection that fails AFTER the InsertTx call.
    // Assert: no row in `report`, no row in `river_job`, no audit entry.
    // This pins D-06 + D-23: report INSERT + audit + River InsertTx are all in one tx.
}

func TestCleanupWorker(t *testing.T) {
    // Seed: 1 expired report (expires_at < now()) with artifact_dir containing dummy files.
    // Run CleanupExpiredReportsWorker.Work.
    // Assert: pdf_status == 'expired', artifact_dir no longer exists on disk.
}
```

**Step H — Add status + generate-enqueue tests in `internal/report/handlers_test.go`:**

```go
func TestGenerateHandler_EnqueuesPDFJob(t *testing.T) {
    // Build Deps with a mock EnqueuePDF closure that records calls.
    // POST /api/reports/generate with a valid body.
    // Assert: EnqueuePDF was called exactly once with the new report's UUID.
}

func TestStatusHandler_PendingReadyFailed(t *testing.T) {
    // Seed three report rows (same created_by as the auth'd admin):
    //   r1: pdf_status='pending', pdf_path=null
    //   r2: pdf_status='ready',   pdf_path='/tmp/abc/report.pdf'
    //   r3: pdf_status='failed',  pdf_path=null
    // GET /api/reports/{r1.id} as admin → 200 JSON with pdf_status='pending' and pdf_path absent (omitempty)
    // GET /api/reports/{r2.id} as admin → 200 JSON with pdf_status='ready'   and pdf_path=='/tmp/abc/report.pdf'
    // GET /api/reports/{r3.id} as admin → 200 JSON with pdf_status='failed'  and pdf_path absent
    // All three responses include created_at, scope, range matching the seeded row.
}

func TestStatusHandler_NotFound(t *testing.T) {
    // GET /api/reports/{random-uuid-with-no-row} → 404 report_not_found
}

func TestStatusHandler_BadUUID(t *testing.T) {
    // GET /api/reports/not-a-uuid → 400 invalid_report_id
    // (Must be 400 BEFORE any DB lookup happens.)
}

func TestStatusHandler_Unauthenticated(t *testing.T) {
    // GET /api/reports/{id} with no session cookie → 401 unauthorized
}

func TestStatusHandler_ViewerCannotSeeOthers(t *testing.T) {
    // Seed report created_by=admin1.
    // GET /api/reports/{id} as viewer2 → 404 report_not_found (existence concealed)
    // GET /api/reports/{id} as admin   → 200
}
```
  </action>
  <verify>
    <automated>go test ./internal/report/... -race -count=1 -short=false -run "TestPDFJob|TestEnqueuePDF_AtomicWithReport|TestCleanupWorker|TestGenerateHandler_EnqueuesPDFJob|TestStatusHandler_" &amp;&amp; grep -q "reports_cache:/var/lib/shifter/reports" compose/bundled.yml &amp;&amp; grep -q "reports_cache:/var/lib/shifter/reports" compose/external.yml &amp;&amp; grep -q "river.NewClient" cmd/serve/serve.go &amp;&amp; grep -q 'r.Get("/api/reports/{id}"' internal/report/handlers.go</automated>
  </verify>
  <acceptance_criteria>
    - `internal/report/pdf_worker.go` exports `PDFReportArgs`, `PDFReportWorker`; `PDFReportArgs.Kind()` returns "pdf_report"
    - `internal/report/cleanup.go` exports `CleanupExpiredReportsArgs`, `CleanupExpiredReportsWorker`
    - `internal/report/download_handler.go` exports `DownloadHandler` — validates UUID BEFORE `filepath.Join`, rejects unknown `kind`, 425 when PDF not ready, 404 when expired
    - `internal/report/handlers.go` exports `StatusHandler` — validates UUID BEFORE DB lookup, returns `StatusResponse` JSON matching plan 05-09's `ReportStatus` shape (id, pdf_status, pdf_path?, created_at, scope, range), 401 unauthenticated, 404 on no-row, 404 (not 403) when non-admin tries to read someone else's report
    - `internal/report/handlers.go` `RegisterRoutes` contains literal `r.Get("/api/reports/{id}",` AND `r.Post("/api/reports/generate"` AND `r.Get("/api/reports/{id}/file/{kind}"` — all three routes wired
    - `cmd/serve/serve.go` contains literal `river.NewClient(riverpgxv5.New(pool)` AND `river.AddWorker(workers, &report.PDFReportWorker` AND `river.PeriodicInterval(1*time.Hour)` AND `riverClient.InsertTx(ctx, tx, report.PDFReportArgs`
    - Both compose YAMLs contain literal `reports_cache:/var/lib/shifter/reports`
    - At least 3 worker tests: TestPDFJob (success), TestEnqueuePDF_AtomicWithReport (atomicity), TestCleanupWorker (expiry)
    - At least 4 handler tests for the new status endpoint: TestStatusHandler_PendingReadyFailed, TestStatusHandler_NotFound, TestStatusHandler_BadUUID, TestStatusHandler_Unauthenticated (plus TestStatusHandler_ViewerCannotSeeOthers for auth-scope)
    - `TestStatusHandler_PendingReadyFailed` asserts pdf_path is present in the JSON ONLY when pdf_status=='ready' (omitempty contract)
    - `TestPDFJob` asserts pdf_status='ready' AND file present with `%PDF-` prefix
    - `TestEnqueuePDF_AtomicWithReport` asserts no orphaned row in any of (report, river_job, audit_log) after injected rollback
    - `go test ./internal/report/... -short=false` exits 0
  </acceptance_criteria>
  <done>Async PDF path live: handler enqueues in tx, worker renders, download serves; status endpoint feeds plan 05-09's poll; cleanup periodic job purges expired artifacts.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| Client → /api/reports/:id/file/:kind | UUID validated before path resolution; kind validated against allowlist |
| Client → /api/reports/:id (status) | UUID validated before DB lookup; same auth scope as download (viewers see own, admins see all, 404 conceals existence) |
| River worker → filesystem | Path composed from `plan.ArtifactDir` (server-controlled) + fixed `report.<ext>` (allowlist) |
| River InsertTx ↔ report INSERT ↔ audit.WriteEntry | All three in same pgx.Tx — atomic; failure rolls all back |

## STRIDE Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-05-06-01 | Repudiation | PDF job runs without user session — could escalate authority | medium | mitigate | Worker has no user-context — it only renders + persists; all authorization decisions made when the report row was originally INSERTed by the (auth'd) GenerateHandler. Worker cannot mint new reports |
| T-05-06-02 | Elevation of Privilege | Path traversal in /api/reports/:id/file/:kind | high | mitigate | `chi.URLParam("id")` parsed via `uuid.Parse` BEFORE any path construction; kind validated against `{csv,xlsx,pdf}` allowlist; final path is `filepath.Join(plan.ArtifactDir, fmt.Sprintf("report.%s", ext))` — no client-controlled string segment. Test `TestDownloadHandler_RejectsInvalidUUID` pins. Same UUID validation applied to StatusHandler before DB lookup (defense in depth). |
| T-05-06-03 | Denial of Service | Report-generation spam fills disk | medium | mitigate | 24h TTL + hourly cleanup (D-07); plan 05-09 frontend already gates "Generate" behind user click; plan 05-09 can add client-side rate-limit. Worker pool MaxWorkers=4 caps concurrent PDF generation CPU+RAM. |
| T-05-06-04 | Information Disclosure | Status endpoint leaks existence of another user's report | low | mitigate | Non-admin requesting a report row owned by someone else gets 404 (not 403) — existence concealed. Test `TestStatusHandler_ViewerCannotSeeOthers` pins. |
</threat_model>

<verification>
1. `go test ./internal/report/... -race -count=1 -short=false` exits 0
2. `go build ./cmd/...` exits 0 (cmd/serve compiles with river wired)
3. `grep -E "(river\\.NewClient|river\\.AddWorker)" cmd/serve/serve.go` returns ≥ 2 matches
4. `grep "reports_cache" compose/bundled.yml compose/external.yml | wc -l` returns ≥ 4
5. NO unsubstituted page placeholder in generated PDFs (TestPDFBranding asserts `page 1 of 1` literal and no `{number}`/`{total}` braces)
6. Every PDF rendered contains a footer with `Generated <ISO ts> <tz> — page N of M` with correct integers (TestPDFBranding + TestPDFBranding_MultiPage)
7. `grep -E "r\\.Get\\(\"/api/reports/\\{id\\}\"" internal/report/handlers.go` returns ≥1 (status route wired) — plan 05-09's poll target exists
</verification>

<success_criteria>
- PDF writer produces branded multi-page PDFs that contain display_name + address + ISO timestamp + correct "page N of M" footer on every page
- River worker consumes pdf_report jobs and writes report.pdf to disk
- Report cleanup PeriodicJob runs hourly and purges expired artifacts
- Download handler streams CSV/XLSX/PDF with auth + expiry + UUID validation
- Status handler (`GET /api/reports/:id`) returns pdf_status JSON so plan 05-09's `useReportPDFStatus` poll can terminate when the PDF is ready/failed
- cmd/serve boots the River client; shutdown is honored
- Volume `reports_cache` mounted in both compose flavors so artifacts survive container restarts during their 24h TTL
</success_criteria>

<output>
After completion, create `.planning/phases/05-aggregates-reports-map-floor-plans/05-06-SUMMARY.md` recording:
- Maroto page-numbering strategy actually used: native placeholder (`{number}/{total}` worked) vs two-pass fallback (manual computation per page)
- River worker max retry count and AfterJobFailed wiring
- Number of PDF artifacts generated during integration tests and verified bytes
- Whether shutdown timeout was sufficient (30s) for in-flight PDF jobs
- Confirmation that `GET /api/reports/:id` returns the exact `ReportStatus` shape plan 05-09 expects (id, pdf_status, pdf_path?, created_at, scope, range)
</output>
