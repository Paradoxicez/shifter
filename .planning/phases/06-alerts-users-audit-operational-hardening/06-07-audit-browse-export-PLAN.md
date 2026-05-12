---
phase: 06-alerts-users-audit-operational-hardening
plan: 07
type: execute
wave: 2
depends_on: [06-01, 06-06]
files_modified:
  - internal/audit/browse_store.go
  - internal/audit/browse_store_test.go
  - internal/audit/handler.go
  - internal/audit/handler_test.go
  - internal/audit/export.go
  - internal/audit/export_test.go
  - internal/audit/export_worker.go
  - internal/audit/export_worker_test.go
  - internal/audit/queries.sql
  - internal/http/router.go
  - internal/cli/serve.go
  - web/src/routes/audit/index.tsx
  - web/src/routes/audit/index.test.tsx
  - web/src/routes/audit/AuditFilterChips.tsx
  - web/src/routes/audit/AuditTable.tsx
  - web/src/routes/audit/AuditRowExpand.tsx
  - web/src/routes/audit/AuditExportButton.tsx
  - web/src/hooks/useAudit.ts
  - web/src/lib/auditParams.ts
  - web/src/components/metering-point/JsonTree.tsx
  - web/src/App.tsx
  - web/playwright/specs/audit-export.spec.ts
autonomous: true
requirements: [AUDIT-02, AUDIT-03]
must_haves:
  truths:
    - "GET /api/audit returns cursor-paginated rows (page size 100) using row-comparison cursor (time DESC, id DESC) per RESEARCH §Decision F"
    - "Filters supported via query params: from, to, user_id, entity_type[], action[], request_id (free-text LIKE)"
    - "Default cold-arrival view = last 7 days, all entity types, all users (D-33)"
    - "GET /api/audit/export?... downloads CSV inline when row count <= 50,000 (UTF-8 BOM, ISO-8601 timestamps in install_tz, timezone label in header block); response respects active filters"
    - "POST /api/audit/export-async kicks off River AuditExportWorker for >50k rows; worker writes /var/lib/shifter/reports/{job_id}/audit-export.csv; 24h TTL; download URL returned"
    - "/audit route admin-only (Can(user,'audit.read') = false for viewer at route-level + sidebar item; viewer redirected to /)"
    - "Filter chip UI uses URL state (useSearchParams + zod with .catch fallbacks per Phase 3 D-15 pattern)"
    - "Expanded row renders before/after as side-by-side JsonTree with changed keys highlighted (D-34); reuses Phase 4 component with new highlightKeys prop"
    - "No live-tail / no SSE — manual Refresh button + refetchOnWindowFocus (D-37)"
  artifacts:
    - path: internal/audit/browse_store.go
      provides: "ListCursor + CountAudit functions"
    - path: internal/audit/export.go
      provides: "StreamCSVExport — UTF-8 BOM + ISO-8601 + CSV-injection prefix"
    - path: internal/audit/export_worker.go
      provides: "AuditExportWorker for >50k async export (mirrors REPT-06 PDF worker pattern)"
    - path: web/src/routes/audit/index.tsx
      provides: "/audit page with URL-state filter chips + virtualized table"
    - path: web/src/components/metering-point/JsonTree.tsx
      provides: "Extended with optional highlightKeys prop for D-34 diff highlighting"
  key_links:
    - from: internal/audit/handler.go
      to: internal/audit/queries.sql::ListAuditRowsCursor
      via: "row-comparison cursor pagination by (time DESC, id DESC)"
      pattern: "ListAuditRowsCursor"
    - from: web/src/routes/audit/AuditRowExpand.tsx
      to: web/src/components/metering-point/JsonTree.tsx
      via: "JsonTree highlightKeys prop wraps changed keys in bg-warning/20"
      pattern: "highlightKeys"
---

<objective>
Ship the audit log browse + CSV export surfaces. Closes AUDIT-02 (filtered browse) and AUDIT-03 (CSV export). Admin-only.

Purpose: every Phase 6 mutation writes an audit row (Plans 06-01/02/03/04/05/06 already commit-in-tx; Plans 06-08/09/11 will follow). This plan provides the operator's read surface — filterable, deep-linkable URL chips, side-by-side before/after diff, CSV export respecting filters. The 50k inline / >50k River-job split mirrors REPT-06's PDF pattern (Phase 5).

Output: backend handlers + cursor query + CSV exporter + River worker for >50k async + /audit React route with filter chips + JsonTree highlightKeys extension + Playwright E2E.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/06-alerts-users-audit-operational-hardening/06-CONTEXT.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-UI-SPEC.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-01-alert-engine-substrate-PLAN.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-06-auth-event-audit-retrofit-PLAN.md
@internal/audit/log.go
@internal/audit/diff.go
@internal/db/migrations/0016_audit_log.up.sql
@internal/auth/authz.go
@internal/http/router.go
@internal/report/csv.go
@internal/report/job.go
@internal/cli/serve.go
@web/src/components/metering-point/JsonTree.tsx
@web/src/components/dashboard/DateRangePicker.tsx
@web/src/components/dashboard/EmptyStateOnboarding.tsx

<interfaces>
internal/auth/authz.go has `ActionAuditRead Action = "audit.read"` (line 116) — admin only in roleBundles per Phase 2 setup. Add:
- ActionAuditExport Action = "audit.export"  — admin only

audit_log schema (migration 0016):
- id UUID PK, time TIMESTAMPTZ DEFAULT now(), user_id UUID NULL, action TEXT, entity_type TEXT, entity_id UUID, before JSONB, after JSONB, notes TEXT, request_id TEXT
- INDEXES: time DESC, (user_id, time DESC), (entity_type, entity_id, time DESC), request_id WHERE NOT NULL

Plan 06-01 added retention_config.audit_log_days column (default 1825).

Plan 06-06 retrofit guarantees auth.login_success / login_failed / logout / password_change audit rows exist.

Phase 5 REPT-06 PDF worker pattern (internal/report/job.go) — model the AuditExportWorker on it:
- River worker
- writes artifact to /var/lib/shifter/reports/{uuid}/audit-export.csv
- 24h cleanup PeriodicJob already exists for /var/lib/shifter/reports

Phase 4 D-19 JsonTree component (web/src/components/metering-point/JsonTree.tsx): currently renders one JSON tree. Extension: optional `highlightKeys: string[]` prop. Keys matching the array get `bg-warning/20` class on the row.

Phase 4 cursor pagination pattern (internal/dashboard/uplinks.go or internal/meteringpoint/uplinks_handler.go) — model Plan 06-07's handler shape.

zod URL-state schema (06-UI-SPEC §Route Architecture):
```ts
const auditParams = z.object({
  from: z.string().datetime().catch(() => sub7days(new Date()).toISOString()),
  to: z.string().datetime().catch(() => new Date().toISOString()),
  user_id: z.string().uuid().optional(),
  entity_type: z.array(z.string()).catch([]),
  action: z.array(z.string()).catch([]),
  request_id: z.string().optional(),
});
```
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Cursor query + browse store + ListHandler + filter validation</name>
  <files>internal/audit/queries.sql, internal/audit/browse_store.go, internal/audit/browse_store_test.go, internal/audit/handler.go, internal/audit/handler_test.go, internal/auth/authz.go, internal/http/router.go</files>
  <read_first>
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md §Decision F "Cursor Pagination Pattern (D-36)" — exact SQL with row-comparison
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-CONTEXT.md §D-31..D-37 (browse decisions)
    - internal/audit/log.go (existing constants + Entry struct)
    - internal/db/migrations/0016_audit_log.up.sql (existing indexes)
    - internal/dashboard/uplinks_handler.go OR internal/meteringpoint/uplinks_handler.go (Phase 4 cursor pagination reference)
    - internal/auth/authz.go (existing ActionAuditRead; this plan adds ActionAuditExport)
  </read_first>
  <behavior>
    - Test (TestCursorQuery_DefaultsTo7Days): no from/to params → server applies from=now()-7d, to=now() per D-33.
    - Test (TestCursorQuery_RowComparisonStable): seed 250 rows; first page returns 100 (cursor=true), second page 100, third page 50 — no row dropped or duplicated. Concurrent insert mid-pagination doesn't break ordering.
    - Test (TestCursorQuery_FiltersByEntityType): pass `entity_type=["user","alert"]` → only user + alert rows.
    - Test (TestCursorQuery_FiltersByActionArray): pass `action=["auth.login_success","auth.logout"]` → only these two actions.
    - Test (TestCursorQuery_RequestIDLike): pass request_id="abc" → ILIKE '%abc%' match.
    - Test (TestCursorQuery_RejectsInvalidEntityType): server-side enum validation rejects entity_type='nonexistent' with 422.
    - Test (TestHandler_AdminOnly): RoleViewer GET /api/audit → 403.
    - Test (TestHandler_ReturnsCursorAndCount): response shape `{rows:[...], next_cursor: "abc..." | null, total_estimated: 1234}`.
    - Test (TestHandler_ExportButtonAware): GET /api/audit/count?filters returns total count for the Export button's >50k decision.
  </behavior>
  <action>
    **internal/auth/authz.go:** Add `ActionAuditExport Action = "audit.export"` in the Phase 6 const block (with companion roleBundles[RoleAdmin] entry; absent from viewer bundle).

    **internal/audit/queries.sql** (new file in this package; sqlc was already configured per Phase 2):
    ```sql
    -- name: ListAuditRowsCursor :many
    -- D-36 + D-32 cursor pagination by (time DESC, id DESC) with row-comparison
    -- to be stable under concurrent inserts (Pitfall: offset/limit can skip rows).
    --
    -- $1=cursor_time, $2=cursor_id (both nullable on first page)
    -- $3=from, $4=to (default 7d if both null at app layer)
    -- $5=user_id (nullable)
    -- $6=entity_type_arr (nullable text[] — `= ANY($6)` filter applied when non-null)
    -- $7=action_arr (nullable text[] — same pattern)
    -- $8=request_id (nullable, ILIKE '%...%')
    SELECT a.id, a.time, a.action, a.entity_type, a.entity_id,
           a.before, a.after, a.notes, a.request_id,
           a.user_id, u.email AS user_email, u.name AS user_name
    FROM audit_log a
    LEFT JOIN "user" u ON a.user_id = u.id
    WHERE (
        $1::TIMESTAMPTZ IS NULL
        OR (a.time, a.id) < ($1::TIMESTAMPTZ, $2::UUID)
    )
    AND ($3::TIMESTAMPTZ IS NULL OR a.time >= $3)
    AND ($4::TIMESTAMPTZ IS NULL OR a.time <  $4)
    AND ($5::UUID IS NULL OR a.user_id = $5)
    AND ($6::TEXT[] IS NULL OR a.entity_type = ANY($6))
    AND ($7::TEXT[] IS NULL OR a.action = ANY($7))
    AND ($8::TEXT IS NULL OR a.request_id ILIKE '%' || $8 || '%')
    ORDER BY a.time DESC, a.id DESC
    LIMIT 100;

    -- name: CountAuditRows :one
    -- Same WHERE clause without the cursor; returns total matching count for the
    -- Export button's >50k decision and for the "Audit log (N)" total counter.
    SELECT count(*)
    FROM audit_log a
    WHERE ($1::TIMESTAMPTZ IS NULL OR a.time >= $1)
      AND ($2::TIMESTAMPTZ IS NULL OR a.time <  $2)
      AND ($3::UUID IS NULL OR a.user_id = $3)
      AND ($4::TEXT[] IS NULL OR a.entity_type = ANY($4))
      AND ($5::TEXT[] IS NULL OR a.action = ANY($5))
      AND ($6::TEXT IS NULL OR a.request_id ILIKE '%' || $6 || '%');

    -- name: DistinctActions :many
    -- For the Action chip's dropdown options — current vocabulary.
    SELECT DISTINCT action FROM audit_log ORDER BY action ASC;

    -- name: DistinctEntityTypes :many
    SELECT DISTINCT entity_type FROM audit_log ORDER BY entity_type ASC;
    ```

    Run `sqlc generate`. The generated functions go into the existing internal/db/sqlc package.

    **internal/audit/browse_store.go:**
    ```go
    package audit

    // Filter mirrors the URL-state schema; nil fields = "no filter on this dimension".
    type Filter struct {
        From         *time.Time
        To           *time.Time
        UserID       *uuid.UUID
        EntityTypes  []string  // empty = no filter
        Actions      []string  // empty = no filter
        RequestIDLike *string
    }

    type Cursor struct {
        Time time.Time
        ID   uuid.UUID
    }

    // EncodeCursor returns a base64url-encoded string; nil → "".
    func EncodeCursor(c *Cursor) string
    // DecodeCursor returns nil for "" or invalid.
    func DecodeCursor(s string) *Cursor

    type ListResult struct {
        Rows       []Row
        NextCursor string
        Total      int64
    }

    func (s *Store) ListCursor(ctx context.Context, f Filter, cursor *Cursor) (*ListResult, error)
    func (s *Store) Count(ctx context.Context, f Filter) (int64, error)
    func (s *Store) DistinctActions(ctx context.Context) ([]string, error)
    func (s *Store) DistinctEntityTypes(ctx context.Context) ([]string, error)
    ```

    **internal/audit/handler.go:**
    - `ListHandler` — parse query (zod-mirroring Go struct via `url.Values` → Filter), call store.ListCursor + store.Count; return `{rows, next_cursor, total}`. Server enforces D-33 default (apply 7d window if from/to absent).
    - `DistinctsHandler` — GET /api/audit/distincts → `{actions: [...], entity_types: [...]}` for filter dropdowns.
    - `CountHandler` — GET /api/audit/count?... → just the count (used by frontend for ">50k → async" decision).

    Filter parsing — IMPORTANT: validate entity_type and action against the server's known vocabulary (the constants defined in audit/log.go + the migration CHECKs). Reject unknown values with 422 to prevent the LIKE/= ANY queries seeing arbitrary user input outside the vocabulary.

    **internal/http/router.go:**
    ```go
    r.Route("/api/audit", func(r chi.Router) {
        r.With(auth.RequireAction(sm, auth.ActionAuditRead)).Get("/", audit.ListHandler(deps))
        r.With(auth.RequireAction(sm, auth.ActionAuditRead)).Get("/count", audit.CountHandler(deps))
        r.With(auth.RequireAction(sm, auth.ActionAuditRead)).Get("/distincts", audit.DistinctsHandler(deps))
        r.With(auth.RequireAction(sm, auth.ActionAuditExport)).Get("/export", audit.ExportHandler(deps))         // sync ≤ 50k (Task 2)
        r.With(auth.RequireAction(sm, auth.ActionAuditExport)).Post("/export-async", audit.ExportAsyncHandler(deps)) // > 50k (Task 2)
    })
    ```

    Currently `ActionAuditRead` is admin-only per Phase 2 setup; ensure roleBundles[RoleViewer] does NOT contain it (per D-31).
  </action>
  <verify>
    <automated>go test ./internal/audit/... ./internal/auth/... -run "TestCursorQuery_|TestHandler_AdminOnly|TestHandler_ReturnsCursor|TestHandler_ExportButtonAware" -count=1 -timeout=90s</automated>
  </verify>
  <acceptance_criteria>
    - `internal/audit/queries.sql` contains the row-comparison cursor: grep `"(a.time, a.id) < (\$1::TIMESTAMPTZ, \$2::UUID)"` or equivalent escaped
    - `internal/audit/queries.sql` contains 4 sqlc named queries: `ListAuditRowsCursor`, `CountAuditRows`, `DistinctActions`, `DistinctEntityTypes`
    - `internal/audit/browse_store.go` exports types `Filter`, `Cursor`, `ListResult` and methods `ListCursor`, `Count`, `DistinctActions`, `DistinctEntityTypes`, `EncodeCursor`, `DecodeCursor`
    - `internal/audit/handler.go` exports `ListHandler`, `CountHandler`, `DistinctsHandler`
    - `internal/auth/authz.go` declares `ActionAuditExport`; present in roleBundles[RoleAdmin]; absent from RoleViewer
    - `internal/http/router.go` mounts `/api/audit/*` with admin-only `RequireAction` guards
    - All 9 listed tests pass: `go test ./internal/audit/... -run "TestCursorQuery|TestHandler" -count=1` exits 0
    - `TestCursorQuery_RowComparisonStable` asserts no row dropped/duplicated across 3 pages despite a concurrent insert between pages
  </acceptance_criteria>
  <done>Cursor pagination is stable under concurrent inserts; filters are server-validated against vocabulary; admin-only; the response shape is contracted for the React Query hook.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: CSV export — inline ≤50k + River AuditExportWorker for >50k</name>
  <files>internal/audit/export.go, internal/audit/export_test.go, internal/audit/export_worker.go, internal/audit/export_worker_test.go, internal/cli/serve.go</files>
  <read_first>
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md §Decision F "CSV Export with Streaming + 50k cap (D-35)"
    - internal/report/csv.go (REPT-03 spec: UTF-8 BOM + ISO-8601 + timezone header — reuse the pattern)
    - internal/report/job.go (REPT-06 PDF worker pattern; AuditExportWorker mirrors it)
    - internal/audit/browse_store.go (Task 1 Filter/Cursor types)
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-CONTEXT.md §D-35 (exact column list)
  </read_first>
  <behavior>
    - Test (TestExport_BOM): response body starts with bytes `\xEF\xBB\xBF` (UTF-8 BOM).
    - Test (TestExport_Headers): Content-Type `text/csv; charset=utf-8`; Content-Disposition `attachment; filename="audit-export-...csv"`.
    - Test (TestExport_TimezoneHeader): first row after BOM is a comment-style header line referencing the install timezone (e.g., `# Timezone: Asia/Bangkok`) or the column header row, depending on REPT-03 pattern — match whatever Phase 5 REPT-03 does.
    - Test (TestExport_ColumnsOrder): row 2 (header) is exactly: `time,user_email,user_id,action,entity_type,entity_id,request_id,notes,before_json,after_json`.
    - Test (TestExport_ISOTimestampsInInstallTZ): time column rendered as `2026-05-12T14:30:00+07:00` (install_tz applied via time.In(installTZ)).
    - Test (TestExport_CSVInjection_PrependsApostrophe): if any cell starts with `=`, `+`, `-`, or `@`, a leading `'` is prepended (REPT-03 Phase 5 mitigation). Seed an audit row with notes='=cmd' → CSV contains `'=cmd`.
    - Test (TestExport_RespectsFilters): pass `entity_type=user` → only user rows in CSV.
    - Test (TestExport_50kCap_Returns413): seed 50001 rows (use bulk INSERT) → GET /api/audit/export returns 413 with JSON `{error:"too_many_rows", suggest:"async", total: 50001}` and NO CSV bytes.
    - Test (TestExportAsync_EnqueuesRiverJob): POST /api/audit/export-async with same 50001 → 202 with `{job_id: uuid, status:"queued"}`; River job kind="audit_export" inserted; eventually writes /var/lib/shifter/reports/{uuid}/audit-export.csv.
    - Test (TestExportAsync_24HCleanup): the reports cleanup PeriodicJob (Phase 5) deletes audit-export.csv 24h after creation — assert the worker writes the file with the same path pattern report cleanup picks up.
    - Test (TestExportAsync_AuditRow): the export-async enqueue writes audit row 'report.generate' (or a new 'audit.export' action — pick one; this plan does NOT add a new audit action; reuse `report.generate` with entity_type='audit_log' or skip the audit since exports don't mutate state).
  </behavior>
  <action>
    **internal/audit/export.go:**
    ```go
    package audit

    import (
        "context"
        "encoding/csv"
        "encoding/json"
        "fmt"
        "net/http"
        "regexp"
        "strings"
        "time"
        "github.com/jackc/pgx/v5/pgxpool"
    )

    const csvExportRowCap = 50_000
    const csvBOM = "\xEF\xBB\xBF"

    // injectionPrefixChars per OWASP CSV-injection mitigation (REPT-03 Phase 5 pattern).
    var csvInjectionRegex = regexp.MustCompile(`^[=+\-@]`)

    func sanitizeForCSV(s string) string {
        if csvInjectionRegex.MatchString(s) { return "'" + s }
        return s
    }

    // StreamCSVExport writes the BOM + header + filtered rows to w. Returns error
    // for any DB/IO failure. The caller (ExportHandler) decides between inline
    // (≤50k) and async (>50k) based on Count(filter) first.
    func StreamCSVExport(ctx context.Context, w http.ResponseWriter, pool *pgxpool.Pool, filter Filter, installTZ *time.Location) error {
        w.Header().Set("Content-Type", "text/csv; charset=utf-8")
        filename := fmt.Sprintf("audit-export-%s.csv", time.Now().UTC().Format("20060102-150405"))
        w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
        if _, err := w.Write([]byte(csvBOM)); err != nil { return err }

        cw := csv.NewWriter(w)
        defer cw.Flush()

        // Header block — match REPT-03 spec
        _ = cw.Write([]string{"# Timezone: " + installTZ.String()})
        _ = cw.Write([]string{"time","user_email","user_id","action","entity_type","entity_id","request_id","notes","before_json","after_json"})

        // Stream rows via pgx.Rows
        rows, err := pool.Query(ctx, exportSQL, filterArgs(filter)...)
        if err != nil { return err }
        defer rows.Close()
        for rows.Next() {
            var t time.Time
            var userEmail, action, entityType, requestID, notes pgtype.Text
            var userID, entityID pgtype.UUID
            var before, after pgtype.JSONB
            if err := rows.Scan(&t, &userEmail, &userID, &action, &entityType, &entityID, &requestID, &notes, &before, &after); err != nil {
                return err
            }
            _ = cw.Write([]string{
                t.In(installTZ).Format(time.RFC3339),
                sanitizeForCSV(userEmail.String),
                uuidToString(userID),
                sanitizeForCSV(action.String),
                sanitizeForCSV(entityType.String),
                uuidToString(entityID),
                sanitizeForCSV(requestID.String),
                sanitizeForCSV(notes.String),
                string(before.Bytes), // JSON is already structured; no injection risk
                string(after.Bytes),
            })
        }
        return rows.Err()
    }

    // ExportHandler is the sync ≤50k path.
    func ExportHandler(deps Deps) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
            filter, err := parseFilterFromQuery(r.URL.Query())
            if err != nil { writeError(w, 422, "bad_filter"); return }
            count, err := deps.Store.Count(r.Context(), filter)
            if err != nil { writeError(w, 500, "count"); return }
            if count > csvExportRowCap {
                w.WriteHeader(http.StatusRequestEntityTooLarge)
                _ = json.NewEncoder(w).Encode(map[string]any{
                    "error":   "too_many_rows",
                    "suggest": "async",
                    "total":   count,
                })
                return
            }
            installTZ, _ := loadInstallTZ(r.Context(), deps.Pool)
            if err := StreamCSVExport(r.Context(), w, deps.Pool, filter, installTZ); err != nil {
                deps.Log.Error("audit_export_stream", "err", err)
            }
        }
    }

    // ExportAsyncHandler is the >50k path. Enqueues a River job and returns 202.
    func ExportAsyncHandler(deps Deps) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
            filter, err := parseFilterFromQuery(r.URL.Query())
            if err != nil { writeError(w, 422, "bad_filter"); return }
            jobID := uuid.New()
            args := AuditExportArgs{
                JobID:  jobID.String(),
                Filter: filter, // serialized as JSON in River args
            }
            _, err = deps.RiverClient.Insert(r.Context(), args, nil)
            if err != nil { writeError(w, 500, "enqueue"); return }
            writeJSON(w, 202, map[string]any{"job_id": jobID, "status": "queued"})
        }
    }
    ```

    **internal/audit/export_worker.go:**
    ```go
    type AuditExportArgs struct {
        JobID  string `json:"job_id"`
        Filter Filter `json:"filter"`
    }
    func (AuditExportArgs) Kind() string { return "audit_export" }
    func (AuditExportArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{MaxAttempts: 3} }

    type AuditExportWorker struct {
        river.WorkerDefaults[AuditExportArgs]
        Pool        *pgxpool.Pool
        Store       *Store
        ReportsDir  string  // /var/lib/shifter/reports (Phase 5)
        Log         *slog.Logger
    }

    func (w *AuditExportWorker) Work(ctx context.Context, job *river.Job[AuditExportArgs]) error {
        dir := filepath.Join(w.ReportsDir, job.Args.JobID)
        if err := os.MkdirAll(dir, 0o755); err != nil { return err }
        path := filepath.Join(dir, "audit-export.csv")
        f, err := os.Create(path)
        if err != nil { return err }
        defer f.Close()

        installTZ, _ := loadInstallTZ(ctx, w.Pool)
        // Adapt StreamCSVExport to a non-http.ResponseWriter (extract to a generic streamer)
        if err := StreamCSVExportToWriter(ctx, f, w.Pool, job.Args.Filter, installTZ); err != nil { return err }
        w.Log.Info("audit_export_complete", "job_id", job.Args.JobID, "path", path)
        return nil
    }
    ```

    Refactor StreamCSVExport so the core streaming logic lives in `StreamCSVExportToWriter(ctx, io.Writer, pool, filter, installTZ)`; ExportHandler wraps with http headers; AuditExportWorker writes directly to file.

    **internal/cli/serve.go:** register `AuditExportWorker`:
    ```go
    river.AddWorker(riverWorkers, &audit.AuditExportWorker{
        Pool: pool, Store: auditStore, ReportsDir: cfg.ReportsDir, Log: log,
    })
    ```
    No periodic schedule — this worker is on-demand only. Reuse the Phase 5 24h cleanup PeriodicJob; the cleanup pattern globs `/var/lib/shifter/reports/*/*.{pdf,csv}` per Phase 5 D-07.

    **Reports cleanup glob extension:** confirm internal/report/job.go's CleanupExpiredReportsWorker walks every subdir under reports/ and deletes files older than 24h. If it's PDF-only by glob (`*.pdf`), extend the glob to include `*.csv` so audit-export.csv files get pruned. If unclear, ADD the .csv glob safely.
  </action>
  <verify>
    <automated>go test ./internal/audit/... -run "TestExport" -count=1 -timeout=120s</automated>
  </verify>
  <acceptance_criteria>
    - `internal/audit/export.go` contains `csvExportRowCap = 50_000` and `csvBOM = "\xEF\xBB\xBF"`
    - `internal/audit/export.go` contains `sanitizeForCSV` function with regex matching `^[=+\-@]`
    - `internal/audit/export.go` exports both `ExportHandler` and `ExportAsyncHandler`
    - Export streams via `csv.NewWriter(w)` (stdlib); time rendered via `t.In(installTZ).Format(time.RFC3339)`
    - `internal/audit/export_worker.go` defines `AuditExportArgs` with `Kind()` returning `"audit_export"` and `InsertOpts` with MaxAttempts=3
    - `internal/cli/serve.go` registers `&audit.AuditExportWorker{`
    - The Phase 5 `CleanupExpiredReportsWorker` glob includes `*.csv` (verify with grep) OR a new note in internal/report/job.go references the audit-export.csv cleanup
    - All 11 listed tests pass: `go test ./internal/audit/... -run TestExport -count=1` exits 0
    - `TestExport_CSVInjection_PrependsApostrophe` proves the `'` prefix is added for cells starting with `=`, `+`, `-`, `@`
    - `TestExport_50kCap_Returns413` asserts 413 status code and no CSV bytes
    - `TestExportAsync_24HCleanup` asserts the file path matches the Phase 5 cleanup glob pattern
  </acceptance_criteria>
  <done>Audit CSV export ships with REPT-03 spec compliance (BOM + ISO-8601 + tz + CSV-injection mitigation) + the 50k inline / >50k River-job split mirroring REPT-06. 24h cleanup reuses Phase 5 infrastructure.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 3: /audit React route with URL-state filter chips + side-by-side JsonTree diff + E2E</name>
  <files>web/src/routes/audit/index.tsx, web/src/routes/audit/index.test.tsx, web/src/routes/audit/AuditFilterChips.tsx, web/src/routes/audit/AuditTable.tsx, web/src/routes/audit/AuditRowExpand.tsx, web/src/routes/audit/AuditExportButton.tsx, web/src/hooks/useAudit.ts, web/src/lib/auditParams.ts, web/src/components/metering-point/JsonTree.tsx, web/src/App.tsx, web/playwright/specs/audit-export.spec.ts</files>
  <read_first>
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-UI-SPEC.md §Surface 6 (full /audit page contract; copy column widths + chip behavior + expanded row layout)
    - web/src/components/metering-point/JsonTree.tsx (existing; extending with highlightKeys prop)
    - web/src/components/dashboard/DateRangePicker.tsx (reused for date-range filter chip)
    - web/src/components/dashboard/EmptyStateOnboarding.tsx (reused for empty states)
    - web/src/lib/use-current-user.ts (admin-only route guard)
  </read_first>
  <behavior>
    - Test (AuditParams_DefaultLast7Days): visiting /audit with no params → useAuditParams returns from = now()-7d, to = now() (D-33).
    - Test (AuditParams_DeepLink): /audit?from=2026-04-01T00:00:00Z&to=2026-05-01T00:00:00Z&entity_type=user → chips reflect these values.
    - Test (AuditTable_VirtualizedRows): seeded 5000 rows scroll smoothly via `@tanstack/react-virtual`; only ~30 DOM rows mounted at a time.
    - Test (AuditTable_RowExpand): clicking chevron renders AuditRowExpand below the row; JsonTree side-by-side panels appear; clicking again collapses.
    - Test (AuditRowExpand_HighlightChangedKeys): row with before={name:"old"} after={name:"new"} → "name" key highlighted with `bg-warning/20` in BOTH panels.
    - Test (AuditRowExpand_HandlesCreatedRemoved): before=null after={...} → left renders "(created)" in text-muted-foreground; after=null → right renders "(removed)".
    - Test (JsonTree_HighlightKeysProp): unit test that JsonTree accepts `highlightKeys: string[]` and applies the highlight class only to matching keys.
    - Test (ExportButton_InlineLEQ50k): mock /api/audit/count → 1234; clicking Export triggers immediate browser download via blob URL.
    - Test (ExportButton_AsyncGT50k): count returns 60000; clicking Export → POST /api/audit/export-async; sonner toast "Full export queued. You'll be notified when it's ready."; inline Alert below header.
    - Test (RefreshButton_RefetchesNoSSE): clicking Refresh calls react-query refetch; no EventSource opened (D-37 — no live tail).
    - Test (ViewerRedirects): visiting /audit as viewer → `<Navigate to="/" replace />` (D-31 admin-only sidebar + route guard).
    - Playwright (audit-export.spec.ts): admin logs in → /audit shows last-7-days default → filters by entity_type=user → list updates → clicks "Export CSV" → CSV downloads → opens in spreadsheet (assert filename + BOM byte by reading first 3 bytes).
  </behavior>
  <action>
    **web/src/lib/auditParams.ts:** zod schema verbatim from UI-SPEC + `useAuditParams()` hook returning `[params, setParams]` via useSearchParams + zod.parse with .catch fallbacks.

    **web/src/hooks/useAudit.ts:**
    ```ts
    export function useAuditList(params: AuditFilters) {
      // useInfiniteQuery for cursor pagination
      return useInfiniteQuery({
        queryKey: ['audit', params],
        queryFn: ({ pageParam }) => apiGet('/api/audit', { ...params, cursor: pageParam }),
        getNextPageParam: (lastPage) => lastPage.next_cursor || undefined,
      });
    }
    export function useAuditCount(params: AuditFilters) {
      return useQuery({ queryKey: ['audit-count', params], queryFn: () => apiGet('/api/audit/count', params) });
    }
    export function useAuditDistincts() {
      return useQuery({ queryKey: ['audit-distincts'], queryFn: () => apiGet('/api/audit/distincts') });
    }
    export function useExportAuditAsync() {
      return useMutation({ mutationFn: (params: AuditFilters) => apiPost('/api/audit/export-async', params) });
    }
    ```

    **web/src/components/metering-point/JsonTree.tsx (EXTEND):** add prop `highlightKeys?: string[]`. When rendering each key, check `highlightKeys?.includes(key)` and conditionally apply `className="bg-warning/20"`. Also add aria-label "▸ changed" for screen-reader signaling per UI-SPEC §Accessibility.

    **web/src/routes/audit/index.tsx (AuditPage):** layout per UI-SPEC §Surface 6:
    - Top-level guard: if `useCurrentUser()?.role !== 'admin'` → `<Navigate to="/" replace />`
    - H1 "Audit log" + right-aligned Refresh button (RotateCw icon) + Export CSV button (AuditExportButton component)
    - AuditFilterChips row
    - AuditTable (TanStack Table + @tanstack/react-virtual)
    - "Load more" button when nextCursor present
    - Empty states per UI-SPEC §Empty States table
    - Keyboard: ↑/↓ row focus, Enter toggles expand, `E` triggers Export

    **AuditFilterChips.tsx:** Popover-per-chip pattern:
    - Date range chip uses Phase 4 DateRangePicker
    - User chip — Combobox populated from /api/users (admin-only — reuse Plan 06-05's useUsersList)
    - Entity type chip — multi-select Popover with checkboxes; options from useAuditDistincts
    - Action chip — multi-select with options from useAuditDistincts
    - Request ID chip — Input with 400ms debounce
    - "Clear all" link reset to defaults (NOT empty — D-33 defaults back to last 7 days)

    **AuditTable.tsx:** columns per UI-SPEC §Surface 6:
    | Column | Width | Render |
    | Time | 120px | Relative + Tooltip absolute install_tz |
    | User | 200px | Email (or "system") |
    | Action | 200px | monospace |
    | Entity | 240px | `{entity_type}:{short-id}` monospace |
    | Request ID | 120px | `r/{first-6}` mono; click copies |
    | Expand | 32px | Chevron |

    Expand toggle calls onExpand(rowId).

    **AuditRowExpand.tsx:** computes `changedKeys` from before vs after (deep key diff; helper `diffKeys(a, b): string[]`). Renders `<div className="grid grid-cols-1 md:grid-cols-2 gap-6">` with two `<JsonTree highlightKeys={changedKeys} />` instances. before=null → "(created)"; after=null → "(removed)".

    **AuditExportButton.tsx:** consumes useAuditCount; if count ≤ 50000 → triggers inline `window.location.href = '/api/audit/export?...'` (browser downloads via Content-Disposition); else → calls useExportAuditAsync mutation → sonner toast + inline Alert with the async job_id. Optional: poll /api/audit/export-async/{job_id}/status for completion (deferred for v1.x; Phase 6 ships the queue + download-when-ready link in toast).

    **web/src/App.tsx:** Register `<Route path="/audit" element={<AuditPage />} />`. The sidebar item for "/audit" is admin-only (already conditioned in Plan 06-04 sidebar).

    **web/playwright/specs/audit-export.spec.ts:**
    ```ts
    test('admin exports filtered CSV', async ({ page, context }) => {
      await loginAsAdmin(page);
      await page.goto('/audit');
      await expect(page.locator('h1')).toContainText('Audit log');
      // Default chip shows last 7 days
      await expect(page.locator('text=Last 7 days')).toBeVisible();
      // Filter by entity type
      await page.click('text=Entity type:');
      await page.click('text=user');
      await page.click('body'); // close popover
      // Wait for table to update
      const [download] = await Promise.all([
        page.waitForEvent('download'),
        page.click('text=Export CSV'),
      ]);
      const path = await download.path();
      // Read first 3 bytes — must be UTF-8 BOM
      const fs = require('fs');
      const buf = fs.readFileSync(path).slice(0, 3);
      expect(buf.equals(Buffer.from([0xEF, 0xBB, 0xBF]))).toBe(true);
    });
    ```
  </action>
  <verify>
    <automated>pnpm -C web test --run web/src/routes/audit web/src/components/metering-point/JsonTree && pnpm -C web exec playwright test audit-export</automated>
  </verify>
  <acceptance_criteria>
    - `web/src/lib/auditParams.ts` exports `auditParams` zod schema with `.catch` fallbacks for from / to (default 7-day window)
    - `web/src/routes/audit/index.tsx` contains `<Navigate to="/" replace />` for role !== 'admin' guard
    - `web/src/routes/audit/AuditTable.tsx` uses `@tanstack/react-table` + `@tanstack/react-virtual`
    - `web/src/routes/audit/AuditRowExpand.tsx` uses `JsonTree` with `highlightKeys` prop in BOTH left and right panel (grep for `highlightKeys=` twice)
    - `web/src/components/metering-point/JsonTree.tsx` accepts new `highlightKeys?: string[]` prop and applies `bg-warning/20` class to matching keys
    - `web/src/routes/audit/AuditExportButton.tsx` conditionally calls `useExportAuditAsync` only when count > 50000 (grep `> 50000` or `> 50_000`)
    - `web/src/App.tsx` registers `<Route path="/audit"`
    - All 11 listed component tests pass: `pnpm -C web test --run web/src/routes/audit` exits 0
    - Playwright spec passes: `pnpm -C web exec playwright test audit-export` exits 0
    - Playwright spec asserts the downloaded file's first 3 bytes are `EF BB BF` (UTF-8 BOM)
  </acceptance_criteria>
  <done>Operator can browse the audit log with URL-deep-linkable filter chips, expand any row for side-by-side diff with highlighted changes, export CSV inline ≤50k or async >50k, all admin-only.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| client→/api/audit/* | Admin-only; viewer 403 server-side AND no sidebar item AND route redirect |
| audit CSV export → browser | Downloaded file may contain operator/user emails — admin role required at server |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-06-07-01 | Elevation of Privilege | viewer reads audit log | mitigate | `auth.RequireAction(sm, auth.ActionAuditRead)` on every /api/audit route; ActionAuditRead is admin-only in roleBundles (already established Phase 2; Plan 06-07 preserves). Frontend redirects viewer at /audit route. Test: TestHandler_AdminOnly. |
| T-06-07-02 | Tampering | filter param SQL injection | mitigate | All filter values bound as positional pgx params; entity_type and action arrays validated server-side against the audit vocabulary CHECK; ILIKE pattern wraps user input with `'%' || $8 || '%'` (parameterized, not concatenated). Test: TestCursorQuery_RejectsInvalidEntityType. |
| T-06-07-03 | Tampering | CSV injection (formula execution in Excel) | mitigate | `sanitizeForCSV` prepends `'` to any cell starting with `=`, `+`, `-`, `@` (REPT-03 Phase 5 mitigation reused). Tests: TestExport_CSVInjection_PrependsApostrophe. |
| T-06-07-04 | DoS | unbounded full-table CSV scan | mitigate | Server enforces ≤ 50,000 row cap on inline export; > 50k returns 413 + suggests async path. Async path runs as River background job and writes to disk (24h TTL via existing Phase 5 cleanup). Test: TestExport_50kCap_Returns413. |
| T-06-07-05 | Information Disclosure | audit export contains password hashes | accept (out of scope) | The `audit_log.before`/`after` columns serialize domain entity state at mutation time; user updates do NOT include password_hash in the diff (Plan 06-05's audit.WriteEntry call passes Before/After maps without password_hash key). Audit ops in this plan don't read or expose password hashes. Plan 06-05 acceptance criteria specifically asserts UserDTO has no PasswordHash field. |
| T-06-07-06 | Spoofing | filter chip URL state poisoned via crafted URL | mitigate | zod schema with `.catch` fallbacks means invalid params don't crash UI; server enums also reject invalid filter values with 422. Pattern is the same as Phase 3 D-15 / Phase 4 D-14, well-tested. |
| T-06-07-07 | DoS | repeated GET /api/audit/count with extreme filters | accept | Count query uses the same (time DESC) index; even at 10M audit rows the count should be sub-second; if Phase 7 finds this hot, add a materialized count cache. |
</threat_model>

<verification>
- All AUDIT-02 + AUDIT-03 surfaces shipped: backend handlers + cursor query + filter chips + virtualized table + JsonTree highlight extension + CSV export sync + async
- 50k cap honored; >50k path mirrors REPT-06 PDF worker
- `go test ./internal/audit/... -count=1 -timeout=120s` passes
- `pnpm -C web test --run web/src/routes/audit` passes
- Playwright `audit-export.spec.ts` passes including BOM-byte assertion
</verification>

<success_criteria>
- AUDIT-02 covered: filter by date range / user / entity type / action / request ID
- AUDIT-03 covered: CSV export inline ≤50k + async >50k mirroring REPT-06
- D-31..D-37 implemented (admin-only route, URL chips, last-7-days default, JsonTree diff, REPT-03 CSV shape, cursor pagination, no live tail)
- Plan 06-01's audit_log_days retention column is wired into Plan 06-10's Settings card (this plan only consumes existing retention; SETT-card editing is Plan 06-10)
</success_criteria>

<output>
After completion, create `.planning/phases/06-alerts-users-audit-operational-hardening/06-07-SUMMARY.md`
</output>
