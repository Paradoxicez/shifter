package importpkg

// HTTP handlers for /api/imports. 5 endpoints per Plan 03-05:
//
//   POST /api/imports                      upload + dry-run, returns job_id
//   GET  /api/imports                      list jobs
//   GET  /api/imports/template.xlsx        download canonical template
//   GET  /api/imports/{job_id}             job + paginated row outcomes
//   POST /api/imports/{job_id}/commit      commit valid rows
//   GET  /api/imports/{job_id}/errors.xlsx errors-only XLSX export
//
// All endpoints are gated by auth.RequireAction(ActionDeviceBulkImport)
// which is admin-only — viewer → 403 (T-3-43 mitigation).

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shifter-io/shifter/internal/auth"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// Upload limits (T-3-40 mitigation). 5MB body cap + 5000 row cap.
const (
	MaxUploadBytes = 5 << 20 // 5 MiB
	MaxRowsPerJob  = 5000
)

// Deps bundles the handles every import endpoint needs.
type Deps struct {
	Pool       *pgxpool.Pool
	SessionMgr *scs.SessionManager
	Log        *slog.Logger

	// Commit is the per-row CS+PG mutation deps.
	Commit *CommitDeps
}

// RegisterRoutes mounts every import endpoint under /api/imports. Each
// route is wrapped in auth.RequireAction(ActionDeviceBulkImport) so
// viewer → 403 across the surface.
func RegisterRoutes(r chi.Router, d Deps) {
	r.Route("/api/imports", func(r chi.Router) {
		r.Use(auth.RequireAction(d.SessionMgr, auth.ActionDeviceBulkImport))
		r.Post("/", uploadHandler(d))
		r.Get("/", listJobsHandler(d))
		r.Get("/template.xlsx", templateHandler(d))
		r.Get("/{job_id}", getJobHandler(d))
		r.Post("/{job_id}/commit", commitHandler(d))
		r.Get("/{job_id}/errors.xlsx", errorsXLSXHandler(d))
	})
}

// ----- upload (POST /) ----------------------------------------------------

// UploadResponse is the JSON body returned by POST /api/imports. The
// `outcomes` slice is capped at the first 50 rows so the response stays
// small even for a 5000-row upload; the operator paginates the full set
// via GET /api/imports/{job_id}.
type UploadResponse struct {
	JobID              string             `json:"job_id"`
	Status             string             `json:"status"`
	Total              int                `json:"total"`
	Valid              int                `json:"valid_count"`
	Invalid            int                `json:"invalid_count"`
	AlreadyExists      int                `json:"already_exists_count"`
	ExpiresAt          string             `json:"expires_at"`
	FirstOutcomes      []OutcomeJSON      `json:"outcomes"`
	FirstOutcomesLimit int                `json:"outcomes_limit"`
}

// OutcomeJSON is the wire shape of one import_job_row outcome.
type OutcomeJSON struct {
	RowIndex int    `json:"row_index"`
	Status   string `json:"status"`
	Reason   string `json:"reason,omitempty"`
}

func uploadHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Body cap (T-3-40). MaxBytesReader wraps r.Body — reads past the
		// limit return an error and the response is 413 to the client.
		r.Body = http.MaxBytesReader(w, r.Body, MaxUploadBytes)

		user, err := auth.UserFromContext(r.Context(), d.SessionMgr)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, errResp{Error: "unauthorized"})
			return
		}
		actorID, err := uuid.Parse(user.ID)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, errResp{Error: "unauthorized"})
			return
		}

		if err := r.ParseMultipartForm(MaxUploadBytes); err != nil {
			// MaxBytesReader signals over-cap by returning an
			// http.MaxBytesError; respond 413 in that case, otherwise 400.
			var maxBytesErr *http.MaxBytesError
			if errors.As(err, &maxBytesErr) {
				writeJSON(w, http.StatusRequestEntityTooLarge, errResp{
					Error:  "upload_too_large",
					Detail: fmt.Sprintf("upload exceeds %d bytes", MaxUploadBytes),
				})
				return
			}
			writeJSON(w, http.StatusBadRequest, errResp{Error: "bad_multipart", Detail: err.Error()})
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errResp{Error: "missing_file"})
			return
		}
		defer func() { _ = file.Close() }()

		// 2. Reject .xlsm (macro-enabled) by extension. T-3-40 mitigation.
		filename := header.Filename
		ext := strings.ToLower(filepath.Ext(filename))
		if ext == ".xlsm" {
			writeJSON(w, http.StatusBadRequest, errResp{
				Error:  "xlsm_not_allowed",
				Detail: "macro-enabled workbooks (.xlsm) are not allowed; re-save as .xlsx",
			})
			return
		}

		var rows []ParsedRow
		var format string
		switch ext {
		case ".xlsx":
			format = "xlsx"
			rows, err = ParseXLSX(file)
		case ".csv":
			format = "csv"
			rows, err = ParseCSV(file)
		default:
			writeJSON(w, http.StatusBadRequest, errResp{
				Error:  "unsupported_format",
				Detail: "only .xlsx and .csv uploads are supported",
			})
			return
		}
		if err != nil {
			// Distinguish the D-04a UTF-8 message → 400 with the
			// operator-readable reason inline.
			writeJSON(w, http.StatusBadRequest, errResp{
				Error:  "parse_failed",
				Detail: err.Error(),
			})
			return
		}

		// 3. Row cap (T-3-40). 5001 rows → 400.
		if len(rows) > MaxRowsPerJob {
			writeJSON(w, http.StatusBadRequest, errResp{
				Error:  "too_many_rows",
				Detail: fmt.Sprintf("max %d rows; got %d", MaxRowsPerJob, len(rows)),
			})
			return
		}

		// 4. Dry-run validation.
		q := sqlc.New(d.Pool)
		outcomes, err := NewDryRunDeps(q).Validate(r.Context(), rows)
		if err != nil {
			internalErr(d.Log, w, "dryrun", err)
			return
		}

		// 5. Persist job + rows in a single tx.
		jobID := uuid.New()
		expiresAt := time.Now().UTC().Add(time.Hour)
		tx, err := d.Pool.BeginTx(r.Context(), pgx.TxOptions{})
		if err != nil {
			internalErr(d.Log, w, "begin tx", err)
			return
		}
		defer func() { _ = tx.Rollback(r.Context()) }()
		txQ := sqlc.New(tx)

		job, err := txQ.CreateImportJob(r.Context(), sqlc.CreateImportJobParams{
			JobID:      pgtypeUUID(jobID),
			OwnerID:    pgtypeUUID(actorID),
			FileName:   filename,
			FileFormat: format,
			TotalRows:  int32(len(outcomes)),
			ExpiresAt:  pgtype.Timestamptz{Time: expiresAt, Valid: true},
		})
		if err != nil {
			internalErr(d.Log, w, "create import_job", err)
			return
		}

		var valid, invalid, already int
		for _, o := range outcomes {
			rawJSON, _ := json.Marshal(rows[o.RowIndex-2].Raw)
			var parsedJSON []byte
			if o.Parsed != nil {
				parsedJSON, _ = json.Marshal(o.Parsed)
			}
			var reasonPtr *string
			if o.Reason != "" {
				s := o.Reason
				reasonPtr = &s
			}
			if _, err := txQ.InsertImportJobRow(r.Context(), sqlc.InsertImportJobRowParams{
				ImportJobID: job.ID,
				RowIndex:    int32(o.RowIndex),
				RawPayload:  rawJSON,
				Parsed:      parsedJSON,
				Status:      o.Status.ToSQLC(),
				Reason:      reasonPtr,
			}); err != nil {
				internalErr(d.Log, w, "insert row", err)
				return
			}
			switch o.Status {
			case StatusValid:
				valid++
			case StatusInvalid:
				invalid++
			case StatusAlreadyExists:
				already++
			}
		}

		if _, err := txQ.UpdateImportJobCounters(r.Context(), sqlc.UpdateImportJobCountersParams{
			ID:                 job.ID,
			TotalRows:          int32(len(outcomes)),
			ValidCount:         int32(valid),
			InvalidCount:       int32(invalid),
			AlreadyExistsCount: int32(already),
		}); err != nil {
			internalErr(d.Log, w, "update counters", err)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			internalErr(d.Log, w, "tx commit", err)
			return
		}

		// 6. Build response — first 50 outcomes inline.
		const previewLimit = 50
		preview := make([]OutcomeJSON, 0, previewLimit)
		for i, o := range outcomes {
			if i >= previewLimit {
				break
			}
			preview = append(preview, OutcomeJSON{
				RowIndex: o.RowIndex,
				Status:   string(o.Status),
				Reason:   o.Reason,
			})
		}
		writeJSON(w, http.StatusOK, UploadResponse{
			JobID:              jobID.String(),
			Status:             string(sqlc.ImportJobStatusPreview),
			Total:              len(outcomes),
			Valid:              valid,
			Invalid:            invalid,
			AlreadyExists:      already,
			ExpiresAt:          expiresAt.Format(time.RFC3339),
			FirstOutcomes:      preview,
			FirstOutcomesLimit: previewLimit,
		})
	}
}

// ----- commit (POST /{job_id}/commit) -------------------------------------

func commitHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := auth.UserFromContext(r.Context(), d.SessionMgr)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, errResp{Error: "unauthorized"})
			return
		}
		actorID, err := uuid.Parse(user.ID)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, errResp{Error: "unauthorized"})
			return
		}
		jobID, ok := parseJobIDParam(w, r)
		if !ok {
			return
		}

		q := sqlc.New(d.Pool)
		job, err := ExpireIfStale(r.Context(), q, jobID)
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errResp{Error: "not_found"})
			return
		}
		if err != nil {
			internalErr(d.Log, w, "expire stale", err)
			return
		}
		if job.Status == sqlc.ImportJobStatusExpired {
			writeJSON(w, http.StatusGone, errResp{Error: "expired"})
			return
		}
		if job.Status != sqlc.ImportJobStatusPreview {
			// Idempotent: re-commit of a committed job returns the stored
			// summary with 200; D-06.
			if job.Status == sqlc.ImportJobStatusCommitted {
				writeJSON(w, http.StatusOK, summaryToJSON(summaryFromJob(job)))
				return
			}
			writeJSON(w, http.StatusConflict, errResp{Error: "not_commitable", Detail: string(job.Status)})
			return
		}

		if d.Commit == nil {
			internalErr(d.Log, w, "commit not configured", errors.New("CommitDeps nil"))
			return
		}
		summary, err := d.Commit.Commit(r.Context(), jobID, actorID)
		if err != nil {
			internalErr(d.Log, w, "commit", err)
			return
		}
		writeJSON(w, http.StatusOK, summaryToJSON(summary))
	}
}

// ----- list (GET /) -------------------------------------------------------

func listJobsHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, perPage := parsePagination(r)
		q := sqlc.New(d.Pool)
		rows, err := q.ListImportJobs(r.Context(), sqlc.ListImportJobsParams{
			Limit:  int32(perPage),
			Offset: int32((page - 1) * perPage),
		})
		if err != nil {
			internalErr(d.Log, w, "list jobs", err)
			return
		}
		total, err := q.CountImportJobs(r.Context())
		if err != nil {
			internalErr(d.Log, w, "count jobs", err)
			return
		}
		out := make([]map[string]any, 0, len(rows))
		for _, j := range rows {
			out = append(out, jobToJSON(j))
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"jobs":     out,
			"total":    total,
			"page":     page,
			"per_page": perPage,
		})
	}
}

// ----- get (GET /{job_id}) ------------------------------------------------

func getJobHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID, ok := parseJobIDParam(w, r)
		if !ok {
			return
		}
		q := sqlc.New(d.Pool)
		job, err := ExpireIfStale(r.Context(), q, jobID)
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errResp{Error: "not_found"})
			return
		}
		if err != nil {
			internalErr(d.Log, w, "get job", err)
			return
		}

		page, perPage := parsePagination(r)
		rows, err := q.ListImportJobRows(r.Context(), sqlc.ListImportJobRowsParams{
			ImportJobID: job.ID,
			Limit:       int32(perPage),
			Offset:      int32((page - 1) * perPage),
		})
		if err != nil {
			internalErr(d.Log, w, "list job rows", err)
			return
		}
		rowTotal, err := q.CountImportJobRows(r.Context(), job.ID)
		if err != nil {
			internalErr(d.Log, w, "count job rows", err)
			return
		}

		outRows := make([]map[string]any, 0, len(rows))
		for _, jr := range rows {
			outRows = append(outRows, importRowToJSON(jr))
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"job":      jobToJSON(job),
			"rows":     outRows,
			"total":    rowTotal,
			"page":     page,
			"per_page": perPage,
		})
	}
}

// ----- template (GET /template.xlsx) --------------------------------------

func templateHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		b, err := GenerateTemplate()
		if err != nil {
			internalErr(d.Log, w, "template", err)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", `attachment; filename="device-import-template.xlsx"`)
		_, _ = w.Write(b)
	}
}

// ----- errors.xlsx (GET /{job_id}/errors.xlsx) ----------------------------

func errorsXLSXHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jobID, ok := parseJobIDParam(w, r)
		if !ok {
			return
		}
		q := sqlc.New(d.Pool)
		job, err := q.GetImportJobByJobID(r.Context(), pgtypeUUID(jobID))
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errResp{Error: "not_found"})
			return
		}
		if err != nil {
			internalErr(d.Log, w, "get job", err)
			return
		}
		// errors.xlsx is meaningful for both preview and committed jobs —
		// preview reveals the invalid rows the operator must fix before
		// committing; committed surfaces invalid + failed rows.
		rows, err := q.ListImportJobErrorRows(r.Context(), job.ID)
		if err != nil {
			internalErr(d.Log, w, "list error rows", err)
			return
		}
		b, err := GenerateErrorsXLSX(rows)
		if err != nil {
			internalErr(d.Log, w, "errors xlsx", err)
			return
		}
		filename := fmt.Sprintf("device-import-errors-%s.xlsx", jobID.String())
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		_, _ = w.Write(b)
	}
}

// ----- helpers ------------------------------------------------------------

type errResp struct {
	Error  string `json:"error"`
	Detail string `json:"detail,omitempty"`
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func internalErr(log *slog.Logger, w http.ResponseWriter, op string, err error) {
	if log != nil {
		log.Error("import handler", "op", op, "err", err)
	}
	writeJSON(w, http.StatusInternalServerError, errResp{Error: "internal"})
}

func parseJobIDParam(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	raw := chi.URLParam(r, "job_id")
	id, err := uuid.Parse(raw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errResp{Error: "invalid_job_id"})
		return uuid.Nil, false
	}
	return id, true
}

func parsePagination(r *http.Request) (page, perPage int) {
	page = 1
	perPage = 50
	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	if v := r.URL.Query().Get("per_page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			perPage = n
		}
	}
	return
}

func jobToJSON(j sqlc.ImportJob) map[string]any {
	out := map[string]any{
		"id":                   uuidStr(j.ID),
		"job_id":               uuidStr(j.JobID),
		"owner_id":             uuidStr(j.OwnerID),
		"file_name":            j.FileName,
		"file_format":          j.FileFormat,
		"total_rows":           int(j.TotalRows),
		"status":               string(j.Status),
		"valid_count":          int(j.ValidCount),
		"invalid_count":        int(j.InvalidCount),
		"already_exists_count": int(j.AlreadyExistsCount),
		"created_count":        int(j.CreatedCount),
		"failed_count":         int(j.FailedCount),
		"expires_at":           tsText(j.ExpiresAt),
		"committed_at":         tsText(j.CommittedAt),
		"created_at":           tsText(j.CreatedAt),
		"updated_at":           tsText(j.UpdatedAt),
	}
	if j.FailedReason != nil {
		out["failed_reason"] = *j.FailedReason
	}
	return out
}

func importRowToJSON(r sqlc.ImportJobRow) map[string]any {
	out := map[string]any{
		"row_index": int(r.RowIndex),
		"status":    string(r.Status),
	}
	if r.Reason != nil {
		out["reason"] = *r.Reason
	}
	if r.CreatedDeviceID.Valid {
		out["created_device_id"] = uuid.UUID(r.CreatedDeviceID.Bytes).String()
	}
	if len(r.RawPayload) > 0 {
		raw := map[string]any{}
		_ = json.Unmarshal(r.RawPayload, &raw)
		out["raw"] = raw
	}
	return out
}

func summaryToJSON(s CommitSummary) map[string]any {
	return map[string]any{
		"job_id":               s.JobID.String(),
		"total":                s.Total,
		"created":              s.Created,
		"already_exists":       s.AlreadyExists,
		"failed":               s.Failed,
		"invalid":              s.Invalid,
		"valid":                s.Valid,
		"envelope_audit_written": s.EnvelopeAuditWritten,
	}
}

func uuidStr(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	return uuid.UUID(u.Bytes).String()
}

func tsText(t pgtype.Timestamptz) any {
	if !t.Valid {
		return nil
	}
	return t.Time.UTC().Format(time.RFC3339)
}

// silence unused-import lints when io isn't used.
var _ = io.Discard
