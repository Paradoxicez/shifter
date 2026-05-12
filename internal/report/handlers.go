package report

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shifter-io/shifter/internal/audit"
	"github.com/shifter-io/shifter/internal/auth"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// Deps bundles the dependencies the report handler needs.
type Deps struct {
	Pool          *pgxpool.Pool
	Queries       *sqlc.Queries
	SessionMgr    *scs.SessionManager
	Identity      InstallIdentity
	ArtifactsRoot string // e.g. /var/lib/shifter/reports
	// EnqueuePDF is called inside the same pgx.Tx as the report INSERT to
	// schedule async PDF generation. It is nil in plan 05-03 (no River worker
	// yet); plan 05-06 wires the River InsertTx call here.
	EnqueuePDF func(ctx context.Context, tx pgx.Tx, reportID uuid.UUID, artifactDir string) error
}

// GenerateRequest is the JSON body for POST /api/reports/generate.
type GenerateRequest struct {
	Scope           string `json:"scope"`              // "all" | "site" | "meter"
	SiteID          string `json:"site_id,omitempty"`
	MeteringPointID string `json:"mp_id,omitempty"`
	GroupBy         string `json:"group,omitempty"`    // "site" | "category" | "none"
	Range           string `json:"range"`              // "daily" | "monthly" | "yearly" | "custom"
	Start           string `json:"start,omitempty"`    // ISO-8601, required for custom
	End             string `json:"end,omitempty"`      // ISO-8601, required for custom
}

// GenerateResponse is the JSON response for POST /api/reports/generate.
type GenerateResponse struct {
	ReportID   uuid.UUID   `json:"report_id"`
	Summary    Summary     `json:"summary"`
	PeriodRows []PeriodRow `json:"period_rows"`
	MeterRows  []MeterRow  `json:"meter_rows"`
	PDFStatus  string      `json:"pdf_status"`
}

// ToConfig validates the request and converts it into a ReportConfig.
// Returns a descriptive error when validation fails.
func (r GenerateRequest) ToConfig(tz *time.Location) (*ReportConfig, error) {
	if tz == nil {
		tz = time.UTC
	}

	// Validate scope.
	validScopes := map[string]bool{"all": true, "site": true, "meter": true}
	if !validScopes[r.Scope] {
		return nil, fmt.Errorf("scope must be one of: all, site, meter; got %q", r.Scope)
	}

	// Validate range.
	validRanges := map[string]bool{"daily": true, "monthly": true, "yearly": true, "custom": true}
	if !validRanges[r.Range] {
		return nil, fmt.Errorf("range must be one of: daily, monthly, yearly, custom; got %q", r.Range)
	}

	// Conditional scope requirements.
	var siteID, mpID uuid.UUID
	var err error
	if r.Scope == "site" {
		if r.SiteID == "" {
			return nil, errors.New("site_id is required when scope=site")
		}
		siteID, err = uuid.Parse(r.SiteID)
		if err != nil {
			return nil, fmt.Errorf("site_id is not a valid UUID: %w", err)
		}
	}
	if r.Scope == "meter" {
		if r.MeteringPointID == "" {
			return nil, errors.New("mp_id is required when scope=meter")
		}
		mpID, err = uuid.Parse(r.MeteringPointID)
		if err != nil {
			return nil, fmt.Errorf("mp_id is not a valid UUID: %w", err)
		}
	}

	// Validate group_by (optional; default to "none").
	groupBy := r.GroupBy
	if groupBy == "" {
		groupBy = "none"
	}
	validGroups := map[string]bool{"site": true, "category": true, "none": true}
	if !validGroups[groupBy] {
		return nil, fmt.Errorf("group must be one of: site, category, none; got %q", groupBy)
	}

	// Determine date range.
	now := time.Now().In(tz)
	var start, end time.Time
	switch r.Range {
	case "daily":
		end = now
		start = now.AddDate(0, 0, -7)
	case "monthly":
		end = now
		start = now.AddDate(0, -1, 0)
	case "yearly":
		end = now
		start = now.AddDate(-1, 0, 0)
	case "custom":
		if r.Start == "" || r.End == "" {
			return nil, errors.New("start and end are required for range=custom")
		}
		start, err = time.Parse(time.RFC3339, r.Start)
		if err != nil {
			return nil, fmt.Errorf("start is not a valid RFC3339 timestamp: %w", err)
		}
		end, err = time.Parse(time.RFC3339, r.End)
		if err != nil {
			return nil, fmt.Errorf("end is not a valid RFC3339 timestamp: %w", err)
		}
		if !end.After(start) {
			return nil, errors.New("end must be after start")
		}
	}

	return &ReportConfig{
		Scope:           r.Scope,
		SiteID:          siteID,
		MeteringPointID: mpID,
		GroupBy:         groupBy,
		RangeKind:       r.Range,
		Start:           start,
		End:             end,
		Timezone:        tz,
	}, nil
}

// GenerateHandler handles POST /api/reports/generate synchronously:
//  1. Build the in-memory Report from CAGGs (BuildReport).
//  2. Open a pgx.Tx and insert the report row + audit entry in one commit (D-23).
//  3. Enqueue the PDF job inside the same tx (deps.EnqueuePDF — nil in 05-03).
//  4. Write CSV + Excel to disk synchronously.
//  5. Return JSON with report_id + assembled data + pdf_status="pending".
func GenerateHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		user, ok := auth.GetUser(ctx, deps.SessionMgr)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}

		var req GenerateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
			return
		}

		tz := deps.Identity.Timezone
		if tz == nil {
			tz = time.UTC
		}

		cfg, err := req.ToConfig(tz)
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
			return
		}

		// Set capabilities from identity so the assembler filters correctly (D-09).
		// The capabilities field would normally come from install_identity.capabilities;
		// for now we use a default of "both" and plan 05-09 can wire the real value.
		cfg.Capabilities = "both"

		// Build report synchronously from CAGGs — this is CPU + DB only, no I/O.
		rpt, err := BuildReport(ctx, deps.Queries, *cfg)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "build_failed"})
			return
		}

		// Open transaction: report row + audit entry + optional PDF job.
		tx, err := deps.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "tx_begin_failed"})
			return
		}
		defer tx.Rollback(ctx) //nolint:errcheck

		q := deps.Queries.WithTx(tx)

		reportID := uuid.New()
		artifactDir := filepath.Join(deps.ArtifactsRoot, reportID.String())
		if err := os.MkdirAll(artifactDir, 0o755); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "artifact_dir_failed"})
			return
		}

		expiresAt := time.Now().Add(24 * time.Hour)

		userUUID := mustParseUUID(user.ID)
		if _, err := q.CreateReport(ctx, sqlc.CreateReportParams{
			ID:              pgtype.UUID{Bytes: reportID, Valid: true},
			UserID:          pgtype.UUID{Bytes: userUUID, Valid: true},
			Scope:           cfg.Scope,
			SiteID:          toNullableUUID(cfg.SiteID),
			MeteringPointID: toNullableUUID(cfg.MeteringPointID),
			GroupBy:         nullableString(cfg.GroupBy),
			RangeKind:       cfg.RangeKind,
			RangeStart:      pgtype.Timestamptz{Time: cfg.Start, Valid: true},
			RangeEnd:        pgtype.Timestamptz{Time: cfg.End, Valid: true},
			ArtifactDir:     artifactDir,
			ExpiresAt:       pgtype.Timestamptz{Time: expiresAt, Valid: true},
		}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "create_report_failed"})
			return
		}

		// Audit entry in the same transaction (D-23 invariant).
		if err := audit.WriteEntry(ctx, tx, audit.Entry{
			UserID:     userUUID,
			Action:     audit.ActionGenerateReport,
			EntityType: audit.EntityTypeReport,
			EntityID:   reportID,
			After: map[string]any{
				"scope":    cfg.Scope,
				"range":    cfg.RangeKind,
				"group_by": cfg.GroupBy,
			},
			RequestID: middleware.GetReqID(ctx),
		}); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "audit_failed"})
			return
		}

		// Enqueue PDF job inside the same tx (nil-tolerant per plan 05-03).
		if deps.EnqueuePDF != nil {
			if err := deps.EnqueuePDF(ctx, tx, reportID, artifactDir); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "enqueue_pdf_failed"})
				return
			}
		}

		if err := tx.Commit(ctx); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "tx_commit_failed"})
			return
		}

		// Write CSV + Excel synchronously after the committed tx (D-06).
		csvPath := filepath.Join(artifactDir, "report.csv")
		if csvFile, err := os.Create(csvPath); err == nil {
			_ = WriteCSV(csvFile, rpt, deps.Identity)
			csvFile.Close()
		}

		if xlsxBytes, err := WriteExcel(rpt, deps.Identity); err == nil {
			_ = os.WriteFile(filepath.Join(artifactDir, "report.xlsx"), xlsxBytes, 0o644)
		}

		writeJSON(w, http.StatusOK, GenerateResponse{
			ReportID:   reportID,
			Summary:    rpt.Summary,
			PeriodRows: rpt.PeriodRows,
			MeterRows:  rpt.MeterRows,
			PDFStatus:  "pending",
		})
	}
}

// RegisterRoutes mounts the report API routes on the given router.
// GET /api/reports/{id}/file/{kind} is wired in plan 05-06 (download path)
// because it depends on the PDF artifact lifecycle from the River worker.
func RegisterRoutes(r chi.Router, deps Deps) {
	r.Post("/api/reports/generate", GenerateHandler(deps))
}

// mustParseUUID parses a UUID string; returns uuid.Nil on error (treated as
// system-event by the audit package per D-23 convention).
func mustParseUUID(s string) uuid.UUID {
	u, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil
	}
	return u
}

// nullableString wraps a string as a *string (nil when empty).
func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// writeJSON encodes body as JSON and writes it with the given status code.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
