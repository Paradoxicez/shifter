package api

// gateway_import_handler.go — Plan 07-13 bulk gateway import HTTP surface.
//
// Endpoints:
//
//	POST /api/gateways/bulk-import/validate  — parse + validate CSV, return ValidateResult
//	POST /api/gateways/bulk-import/commit    — parse + upsert + audit, return CommitResult
//	GET  /api/gateways/bulk-import/template  — stream a CSV template download
//
// Security:
//   - ActionGatewayBulkImport guards all three endpoints (admin-only).
//   - T-07-13-01: 5 MiB body cap before parse.
//   - T-07-13-03: 5000-row cap enforced inside ImportService.
//   - T-07-13-04: RequireAction middleware 403s viewer before handler runs.

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/gateway"
)

// maxGatewayImportBytes is the per-request body cap (T-07-13-01).
const maxGatewayImportBytes = 5 << 20 // 5 MiB

// gatewayCSVTemplateHeaders is the canonical column order for the CSV template.
var gatewayCSVTemplateHeaders = []string{
	"gateway_eui",
	"name",
	"description",
	"latitude",
	"longitude",
	"region",
}

// GatewayImportService is the narrow interface the handlers require from
// gateway.ImportService — makes test substitution straightforward.
type GatewayImportService interface {
	Validate(ctx interface{ Done() <-chan struct{} }, csvBytes []byte) (gateway.ValidateResult, error)
	Commit(ctx interface{ Done() <-chan struct{} }, csvBytes []byte, actorID string) (gateway.CommitResult, error)
}

// GatewayImportDeps bundles the dependencies for gateway bulk-import handlers.
type GatewayImportDeps struct {
	Pool       *pgxpool.Pool
	SessionMgr *scs.SessionManager
	Log        *slog.Logger
	ImportSvc  *gateway.ImportService
}

// RegisterGatewayImportRoutes mounts the 3 gateway import endpoints under a
// RequireAction(ActionGatewayBulkImport) group.
//
// Route table:
//
//	POST /api/gateways/bulk-import/validate  — admin-only
//	POST /api/gateways/bulk-import/commit    — admin-only
//	GET  /api/gateways/bulk-import/template  — admin-only (T-07-13-04)
func RegisterGatewayImportRoutes(r chi.Router, deps GatewayImportDeps) {
	r.Group(func(rt chi.Router) {
		rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionGatewayBulkImport))
		rt.Post("/api/gateways/bulk-import/validate", BulkImportValidateHandler(deps))
		rt.Post("/api/gateways/bulk-import/commit", BulkImportCommitHandler(deps))
		rt.Get("/api/gateways/bulk-import/template", DownloadTemplateHandler(deps))
	})
}

// BulkImportValidateHandler handles POST /api/gateways/bulk-import/validate.
// It parses the multipart CSV, validates every row, and returns a ValidateResult.
// No DB writes are performed.
func BulkImportValidateHandler(deps GatewayImportDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		csvBytes, ok := readMultipartCSV(w, r, deps.Log)
		if !ok {
			return
		}

		result, err := deps.ImportSvc.Validate(r.Context(), csvBytes)
		if err != nil {
			gwImportInternalErr(deps.Log, w, "validate", err)
			return
		}

		writeGWImportJSON(w, http.StatusOK, map[string]any{
			"valid_rows": result.ValidRows,
			"error_rows": result.ErrorRows,
			"errors":     outcomeSliceToJSON(result.Errors),
		})
	}
}

// BulkImportCommitHandler handles POST /api/gateways/bulk-import/commit.
// It parses the multipart CSV, upserts each valid gateway row, writes per-row
// audit entries, and returns a CommitResult.
func BulkImportCommitHandler(deps GatewayImportDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := auth.UserFromContext(r.Context(), deps.SessionMgr)
		if err != nil {
			writeGWImportJSON(w, http.StatusUnauthorized, gwErrResp{Error: "unauthorized"})
			return
		}

		csvBytes, ok := readMultipartCSV(w, r, deps.Log)
		if !ok {
			return
		}

		result, err := deps.ImportSvc.Commit(r.Context(), csvBytes, user.ID)
		if err != nil {
			gwImportInternalErr(deps.Log, w, "commit", err)
			return
		}

		writeGWImportJSON(w, http.StatusOK, map[string]any{
			"created":  result.Created,
			"updated":  result.Updated,
			"skipped":  result.Skipped,
			"outcomes": outcomeSliceToJSON(result.Outcomes),
		})
	}
}

// DownloadTemplateHandler handles GET /api/gateways/bulk-import/template.
// It streams a minimal CSV template with the canonical column headers.
func DownloadTemplateHandler(_ GatewayImportDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="gateway-import-template.csv"`)
		w.WriteHeader(http.StatusOK)
		// Write UTF-8 BOM so Excel on Windows opens the file with correct encoding.
		_, _ = w.Write([]byte("\xEF\xBB\xBF"))
		_, _ = io.WriteString(w, strings.Join(gatewayCSVTemplateHeaders, ",")+"\n")
		// One example row so the operator sees the expected format.
		_, _ = io.WriteString(w, "aabbccddeeff0001,My Gateway,Optional description,13.7563,100.5018,as923_2\n")
	}
}

// ----- helpers ---------------------------------------------------------------

// readMultipartCSV reads the "file" form field from a multipart request,
// enforces the size cap, and returns the raw CSV bytes. Returns (nil, false)
// and writes the error response on failure.
func readMultipartCSV(w http.ResponseWriter, r *http.Request, log *slog.Logger) ([]byte, bool) {
	// T-07-13-01: cap the request body before parsing so an attacker cannot
	// exhaust server memory with a 1 GB upload.
	r.Body = http.MaxBytesReader(w, r.Body, maxGatewayImportBytes)

	if err := r.ParseMultipartForm(maxGatewayImportBytes); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeGWImportJSON(w, http.StatusRequestEntityTooLarge, gwErrResp{
				Error:  "upload_too_large",
				Detail: "upload exceeds 5 MiB limit",
			})
			return nil, false
		}
		writeGWImportJSON(w, http.StatusBadRequest, gwErrResp{
			Error:  "bad_multipart",
			Detail: err.Error(),
		})
		return nil, false
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		writeGWImportJSON(w, http.StatusBadRequest, gwErrResp{Error: "missing_file"})
		return nil, false
	}
	defer func() { _ = file.Close() }()

	csvBytes, err := io.ReadAll(file)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			writeGWImportJSON(w, http.StatusRequestEntityTooLarge, gwErrResp{
				Error:  "upload_too_large",
				Detail: "upload exceeds 5 MiB limit",
			})
			return nil, false
		}
		gwImportInternalErr(log, w, "read file", err)
		return nil, false
	}

	return csvBytes, true
}

// outcomeSliceToJSON converts a []gateway.RowOutcome to a JSON-serialisable slice.
func outcomeSliceToJSON(outcomes []gateway.RowOutcome) []map[string]any {
	if len(outcomes) == 0 {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(outcomes))
	for _, o := range outcomes {
		m := map[string]any{
			"row_number":  o.RowNumber,
			"gateway_eui": o.GatewayEUI,
			"outcome":     o.Outcome,
		}
		if o.ErrorMessage != "" {
			m["error_message"] = o.ErrorMessage
		}
		out = append(out, m)
	}
	return out
}

type gwErrResp struct {
	Error  string `json:"error"`
	Detail string `json:"detail,omitempty"`
}

func writeGWImportJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func gwImportInternalErr(log *slog.Logger, w http.ResponseWriter, op string, err error) {
	if log != nil {
		log.Error("gateway import handler", "op", op, "err", err)
	}
	writeGWImportJSON(w, http.StatusInternalServerError, gwErrResp{Error: "internal"})
}
