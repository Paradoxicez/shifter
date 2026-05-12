package report

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"log/slog"

	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// CleanupExpiredReportsArgs is the job argument type for the hourly cleanup job.
// It carries no payload — the worker scans the DB for all expired reports each run.
type CleanupExpiredReportsArgs struct{}

// Kind returns the unique job kind identifier consumed by River's worker registry.
func (CleanupExpiredReportsArgs) Kind() string { return "report_cleanup" }

// CleanupExpiredReportsWorker implements the hourly cleanup PeriodicJob (D-07).
// It scans report rows where expires_at < now() AND pdf_status <> 'expired',
// removes the artifact directory from disk, and marks pdf_status='expired'.
type CleanupExpiredReportsWorker struct {
	river.WorkerDefaults[CleanupExpiredReportsArgs]

	Pool    *pgxpool.Pool
	Queries *sqlc.Queries
	Log     *slog.Logger
}

// Work runs the cleanup cycle.
//
//  1. Query all expired-but-not-yet-marked report rows.
//  2. For each: os.RemoveAll the artifact directory (idempotent on missing paths).
//  3. Mark report.pdf_status='expired' in the DB.
//
// Errors on individual rows are logged but do not abort the cycle — a single
// failed row will be retried on the next hourly run.
func (w *CleanupExpiredReportsWorker) Work(ctx context.Context, job *river.Job[CleanupExpiredReportsArgs]) error {
	expired, err := w.Queries.ListExpiredReports(ctx)
	if err != nil {
		return fmt.Errorf("list expired reports: %w", err)
	}

	for _, e := range expired {
		if err := os.RemoveAll(e.ArtifactDir); err != nil {
			w.Log.Warn("report.cleanup: removeAll failed",
				"id", e.ID,
				"artifact_dir", e.ArtifactDir,
				"err", err,
			)
			// Continue — mark expired so we don't retry the disk-op on a
			// directory that may have already been removed by the OS.
		}
		if err := w.Queries.MarkReportExpired(ctx, e.ID); err != nil {
			w.Log.Warn("report.cleanup: mark expired failed",
				"id", e.ID,
				"err", err,
			)
		}
	}

	w.Log.Info("report.cleanup.cycle", "expired_count", len(expired))
	return nil
}
