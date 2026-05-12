package audit

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
)

// AuditExportWorker implements the River worker for async audit CSV export.
// It mirrors the Phase 5 PDFReportWorker pattern (REPT-06):
//   - writes artifact to {ReportsDir}/{jobID}/audit-export.csv
//   - 24h TTL managed by PruneExpiredCSVExports (filesystem mtime-based)
//   - UTF-8 BOM + REPT-03 CSV format via StreamCSVExportToWriter
type AuditExportWorker struct {
	river.WorkerDefaults[AuditExportArgs]

	Pool       *pgxpool.Pool
	Store      *Store
	ReportsDir string // /var/lib/shifter/reports (mirrors Phase 5 cfg.ReportsRoot)
	Log        *slog.Logger
}

// Work executes the async CSV export job.
// Writes {ReportsDir}/{jobID}/audit-export.csv with BOM + REPT-03 format.
func (w *AuditExportWorker) Work(ctx context.Context, job *river.Job[AuditExportArgs]) error {
	dir := filepath.Join(w.ReportsDir, job.Args.JobID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("audit_export_worker: mkdir %s: %w", dir, err)
	}

	path := filepath.Join(dir, "audit-export.csv")
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("audit_export_worker: create %s: %w", path, err)
	}
	defer f.Close()

	installTZ, _ := loadInstallTZ(ctx, w.Pool)

	if err := StreamCSVExportToWriter(ctx, f, w.Pool, job.Args.Filter, installTZ); err != nil {
		return fmt.Errorf("audit_export_worker: stream: %w", err)
	}

	w.Log.Info("audit_export_worker.complete",
		"job_id", job.Args.JobID,
		"path", path,
	)
	return nil
}

// PruneExpiredCSVExports scans {reportsDir}/*/ for directories that contain
// an audit-export.csv file whose mtime is older than ttl, and removes the
// entire subdirectory. This provides 24h TTL for audit exports without
// requiring a DB row (mirrors Phase 5 D-07 cleanup semantics).
//
// Called by the hourly CleanupExpiredReportsWorker in internal/report/cleanup.go
// (or a standalone PeriodicJob) to prune stale audit exports alongside PDF artifacts.
func PruneExpiredCSVExports(reportsDir string, ttl time.Duration) error {
	entries, err := os.ReadDir(reportsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no reports dir yet — nothing to prune
		}
		return fmt.Errorf("audit.PruneExpiredCSVExports: readdir %s: %w", reportsDir, err)
	}

	now := time.Now()
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		subDir := filepath.Join(reportsDir, entry.Name())
		csvPath := filepath.Join(subDir, "audit-export.csv")

		info, err := os.Stat(csvPath)
		if os.IsNotExist(err) {
			continue // not an audit export directory
		}
		if err != nil {
			continue // stat error; skip
		}

		if now.Sub(info.ModTime()) > ttl {
			if rmErr := os.RemoveAll(subDir); rmErr != nil {
				// Log but continue — same best-effort pattern as CleanupExpiredReportsWorker.
				continue
			}
		}
	}
	return nil
}
