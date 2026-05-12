package report

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"log/slog"

	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// PDFReportArgs identifies the report to render. Kept minimal — all the
// detail is rebuilt from the DB row inside Work() so the job arg JSON
// stays small (River truncates very long arg payloads).
type PDFReportArgs struct {
	ReportID uuid.UUID `json:"report_id"`
}

// Kind returns the unique job kind identifier consumed by River's worker registry.
func (PDFReportArgs) Kind() string { return "pdf_report" }

// InstallIdentityProvider loads the install identity at worker runtime rather
// than embedding it at construction time. This ensures workers always use the
// current identity even if it was updated after the binary started.
type InstallIdentityProvider interface {
	Load(ctx context.Context) (InstallIdentity, error)
}

// PDFReportWorker generates the PDF artifact and updates report.pdf_status.
// River retries failed jobs up to 3 times (default); exhausted retries are
// handled by marking pdf_status='failed' via the ErrorHandler in cmd/serve.
type PDFReportWorker struct {
	river.WorkerDefaults[PDFReportArgs]

	Pool     *pgxpool.Pool
	Queries  *sqlc.Queries
	Identity InstallIdentityProvider
	Log      *slog.Logger
}

// Work generates the PDF for the given report ID.
//
//  1. Mark report.pdf_status='running' so the frontend poll knows work is in progress.
//  2. Load the report row to recover scope + range + artifact_dir.
//  3. Load the install identity for branding.
//  4. Re-build the in-memory Report from CAGGs via BuildReport.
//  5. Write the PDF to <artifact_dir>/report.pdf via WritePDF.
//  6. Update report.pdf_status='ready' and report.pdf_path.
func (w *PDFReportWorker) Work(ctx context.Context, job *river.Job[PDFReportArgs]) error {
	reportPGUUID := pgtype.UUID{Bytes: job.Args.ReportID, Valid: true}

	// 1) Mark running.
	if _, err := w.Pool.Exec(ctx,
		`UPDATE report SET pdf_status = 'running' WHERE id = $1`,
		reportPGUUID,
	); err != nil {
		return fmt.Errorf("mark running: %w", err)
	}

	// 2) Load report row.
	plan, err := w.Queries.GetReport(ctx, reportPGUUID)
	if err != nil {
		return fmt.Errorf("load report row: %w", err)
	}

	// 3) Load install identity for PDF branding.
	identity, err := w.Identity.Load(ctx)
	if err != nil {
		return fmt.Errorf("load identity: %w", err)
	}

	// 4) Re-build the report data from CAGGs.
	cfg := planRowToConfig(plan, identity.Timezone)
	rpt, err := BuildReport(ctx, w.Queries, cfg)
	if err != nil {
		return fmt.Errorf("build report: %w", err)
	}

	// 5) Generate PDF bytes.
	pdfBytes, err := WritePDF(rpt, identity)
	if err != nil {
		return fmt.Errorf("write pdf: %w", err)
	}

	// 6) Write file and update DB status.
	pdfPath := filepath.Join(plan.ArtifactDir, "report.pdf")
	if err := os.WriteFile(pdfPath, pdfBytes, 0o644); err != nil {
		return fmt.Errorf("write pdf file: %w", err)
	}

	pdfPathStr := pdfPath
	if err := w.Queries.UpdateReportPDFStatus(ctx, sqlc.UpdateReportPDFStatusParams{
		ID:        reportPGUUID,
		PdfStatus: "ready",
		PdfPath:   &pdfPathStr,
	}); err != nil {
		return fmt.Errorf("update pdf_status ready: %w", err)
	}

	w.Log.Info("report.pdf.ready",
		"report_id", job.Args.ReportID,
		"bytes", len(pdfBytes),
		"path", pdfPath,
	)
	return nil
}

// planRowToConfig converts a sqlc Report row back to a ReportConfig suitable
// for passing to BuildReport. The timezone comes from the install identity
// since the report table stores the config as opaque columns, not a tz string.
func planRowToConfig(plan sqlc.Report, tz *time.Location) ReportConfig {
	if tz == nil {
		tz = time.UTC
	}

	var siteID uuid.UUID
	if plan.SiteID.Valid {
		siteID = uuid.UUID(plan.SiteID.Bytes)
	}
	var mpID uuid.UUID
	if plan.MeteringPointID.Valid {
		mpID = uuid.UUID(plan.MeteringPointID.Bytes)
	}

	var groupBy string
	if plan.GroupBy != nil {
		groupBy = *plan.GroupBy
	}
	if groupBy == "" {
		groupBy = "none"
	}

	var start, end time.Time
	if plan.RangeStart.Valid {
		start = plan.RangeStart.Time
	}
	if plan.RangeEnd.Valid {
		end = plan.RangeEnd.Time
	}

	return ReportConfig{
		Scope:           plan.Scope,
		SiteID:          siteID,
		MeteringPointID: mpID,
		GroupBy:         groupBy,
		RangeKind:       plan.RangeKind,
		Start:           start,
		End:             end,
		Capabilities:    "both", // worker always generates full report; auth already checked at handler time
		Timezone:        tz,
	}
}
