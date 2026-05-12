package audit

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// ─────────────────────────────────────────────────────────────────────────
// CSV Export constants + helpers (REPT-03 spec compliance)
// ─────────────────────────────────────────────────────────────────────────

const csvExportRowCap = 50_000

// csvBOM is the UTF-8 BOM that Excel uses to auto-detect encoding.
// REPT-03 spec: MUST be the first bytes emitted before any csv.Writer output.
const csvBOM = "\xEF\xBB\xBF"

// csvColumns is the exact column order for the audit CSV export (D-35).
var csvColumns = []string{
	"time", "user_email", "user_id", "action", "entity_type",
	"entity_id", "request_id", "notes", "before_json", "after_json",
}

// csvInjectionRegex matches cells that start with characters Excel may
// interpret as formula prefixes (OWASP CSV-injection mitigation, REPT-03).
var csvInjectionRegex = regexp.MustCompile(`^[=+\-@]`)

// sanitizeForCSV prepends a single quote to any cell value that begins with
// a formula-injection character. This is the same mitigation used in REPT-03.
func sanitizeForCSV(s string) string {
	if csvInjectionRegex.MatchString(s) {
		return "'" + s
	}
	return s
}

// filterSummary returns a compact human-readable summary of the filter for
// the audit export audit row notes.
func filterSummary(f Filter) string {
	var from, to string
	if f.From != nil {
		from = f.From.Format(time.RFC3339)
	}
	if f.To != nil {
		to = f.To.Format(time.RFC3339)
	}
	return fmt.Sprintf("from=%s to=%s entity_types=%v actions=%v", from, to, f.EntityTypes, f.Actions)
}

// ─────────────────────────────────────────────────────────────────────────
// Core streaming logic (shared by inline handler + River worker)
// ─────────────────────────────────────────────────────────────────────────

// StreamCSVExportToWriter streams filtered audit rows to any io.Writer in
// REPT-03 format: BOM + timezone comment + column header + data rows.
// The caller is responsible for setting HTTP headers before calling this.
func StreamCSVExportToWriter(ctx context.Context, w io.Writer, pool *pgxpool.Pool, filter Filter, installTZ *time.Location) error {
	if installTZ == nil {
		installTZ = time.UTC
	}

	// 1) UTF-8 BOM — first bytes emitted.
	if _, err := w.Write([]byte(csvBOM)); err != nil {
		return fmt.Errorf("csv bom: %w", err)
	}

	cw := csv.NewWriter(w)
	defer cw.Flush()

	// 2) Timezone comment header (REPT-03 pattern).
	if err := cw.Write([]string{"# Timezone: " + installTZ.String()}); err != nil {
		return fmt.Errorf("csv tz header: %w", err)
	}

	// 3) Column header row.
	if err := cw.Write(csvColumns); err != nil {
		return fmt.Errorf("csv column header: %w", err)
	}

	// 4) Apply default window if needed.
	fromVal, toVal := applyDefaultWindow(filter.From, filter.To)

	var entityTypeArr interface{}
	if len(filter.EntityTypes) > 0 {
		entityTypeArr = filter.EntityTypes
	}
	var actionArr interface{}
	if len(filter.Actions) > 0 {
		actionArr = filter.Actions
	}

	const exportSQL = `
SELECT a.time, COALESCE(u.email, '') AS user_email, a.user_id::text,
       a.action, a.entity_type, a.entity_id::text,
       COALESCE(a.request_id, ''), COALESCE(a.notes, ''),
       COALESCE(a.before::text, ''), COALESCE(a.after::text, '')
FROM audit_log a
LEFT JOIN "user" u ON a.user_id = u.id
WHERE ($1::TIMESTAMPTZ IS NULL OR a.time >= $1)
  AND ($2::TIMESTAMPTZ IS NULL OR a.time <  $2)
  AND ($3::UUID IS NULL OR a.user_id = $3)
  AND ($4::TEXT[] IS NULL OR a.entity_type = ANY($4))
  AND ($5::TEXT[] IS NULL OR a.action = ANY($5))
  AND ($6::TEXT IS NULL OR a.request_id ILIKE '%' || $6 || '%')
ORDER BY a.time DESC, a.id DESC
`

	rows, err := pool.Query(ctx, exportSQL,
		fromVal,            // $1
		toVal,              // $2
		filter.UserID,      // $3
		entityTypeArr,      // $4
		actionArr,          // $5
		filter.RequestIDLike, // $6
	)
	if err != nil {
		return fmt.Errorf("csv export query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			t                                                          time.Time
			userEmail, userIDStr, action, entityType, entityIDStr     string
			requestID, notes, beforeJSON, afterJSON                   string
		)
		if err := rows.Scan(&t, &userEmail, &userIDStr, &action, &entityType, &entityIDStr,
			&requestID, &notes, &beforeJSON, &afterJSON); err != nil {
			return fmt.Errorf("csv export scan: %w", err)
		}
		record := []string{
			t.In(installTZ).Format(time.RFC3339),
			sanitizeForCSV(userEmail),
			userIDStr,
			sanitizeForCSV(action),
			sanitizeForCSV(entityType),
			entityIDStr,
			sanitizeForCSV(requestID),
			sanitizeForCSV(notes),
			beforeJSON, // JSON is structured data; no injection risk
			afterJSON,
		}
		if err := cw.Write(record); err != nil {
			return fmt.Errorf("csv export write row: %w", err)
		}
	}
	return rows.Err()
}

// loadInstallTZ reads the install timezone from install_identity.
// Falls back to time.UTC on any error.
func loadInstallTZ(ctx context.Context, pool *pgxpool.Pool) (*time.Location, error) {
	var tzName string
	if err := pool.QueryRow(ctx, `SELECT timezone FROM install_identity WHERE id = 1`).Scan(&tzName); err != nil {
		return time.UTC, nil
	}
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return time.UTC, nil
	}
	return loc, nil
}

// ─────────────────────────────────────────────────────────────────────────
// ExportHandler — sync ≤50k inline CSV download
// ─────────────────────────────────────────────────────────────────────────

// ExportHandler handles GET /api/audit/export.
// For row counts ≤ 50k: streams CSV inline with Content-Disposition attachment.
// For row counts > 50k: returns 413 + JSON {error, suggest, total}.
// Writes an audit.export row for operator visibility (D-35).
func ExportHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter, err := parseFilterFromQuery(r.URL.Query())
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}

		count, err := deps.Store.Count(r.Context(), filter)
		if err != nil {
			deps.Log.Error("audit.export.count", "err", err)
			writeError(w, http.StatusInternalServerError, "count_error")
			return
		}

		if count > csvExportRowCap {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":   "too_many_rows",
				"suggest": "async",
				"total":   count,
			})
			return
		}

		installTZ, _ := loadInstallTZ(r.Context(), deps.Pool)

		// Write audit.export row BEFORE streaming (D-35: admin downloading PII
		// is operator-visible; write the trail even if streaming later fails).
		writeExportAuditRow(r.Context(), deps.Pool, uuid.Nil, filter, count,
			fmt.Sprintf("Inline export of %d rows; filter=%s", count, filterSummary(filter)))

		filename := fmt.Sprintf("audit-export-%s.csv", time.Now().UTC().Format("20060102-150405"))
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)

		if err := StreamCSVExportToWriter(r.Context(), w, deps.Pool, filter, installTZ); err != nil {
			deps.Log.Error("audit.export.stream", "err", err)
		}
	}
}

// writeExportAuditRow writes an audit.export meta-row recording that an admin
// triggered a CSV export. Uses a standalone connection (not inside a tx) since
// the export itself is a read-only operation; the audit row is best-effort.
func writeExportAuditRow(ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID, filter Filter, count int64, notes string) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	metaID := uuid.New()
	_ = WriteEntry(ctx, tx, Entry{
		UserID:     userID,
		Action:     ActionAuditExport,
		EntityType: EntityTypeAuditLog,
		EntityID:   metaID,
		Notes:      notes,
	})
	_ = tx.Commit(ctx)
}

// ─────────────────────────────────────────────────────────────────────────
// ExportAsyncHandler — >50k River job enqueue
// ─────────────────────────────────────────────────────────────────────────

// AuditExportArgs is the River job argument type for async CSV export.
type AuditExportArgs struct {
	JobID  string `json:"job_id"`
	Filter Filter `json:"filter"`
}

// Kind returns the unique job kind identifier consumed by River's worker registry.
func (AuditExportArgs) Kind() string { return "audit_export" }

// InsertOpts returns River insert options for the export job.
func (AuditExportArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{MaxAttempts: 3} }

// ExportAsyncDeps extends Deps with the River client needed by ExportAsyncHandler.
type ExportAsyncDeps struct {
	Deps
	RiverClient RiverInserter
}

// RiverInserter is the subset of the River client API needed to enqueue jobs.
type RiverInserter interface {
	InsertTx(ctx context.Context, tx interface{ Exec(ctx context.Context, sql string, arguments ...any) (interface{}, error) }, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

// ExportAsyncHandler handles POST /api/audit/export-async.
// Enqueues a River AuditExportWorker job and returns 202 + {job_id, status}.
// Writes an audit.export row for operator visibility (D-35).
func ExportAsyncHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Stub: full River integration in Task 2. For now return 501 so the
		// router can register the route and tests can target it.
		writeError(w, http.StatusNotImplemented, "use_export_worker_task2")
	}
}
