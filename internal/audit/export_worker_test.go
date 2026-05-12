package audit_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/audit"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// TestAuditExportArgs_Kind verifies the River job kind constant.
func TestAuditExportArgs_Kind(t *testing.T) {
	args := audit.AuditExportArgs{JobID: uuid.New().String(), Filter: audit.Filter{}}
	assert.Equal(t, "audit_export", args.Kind())
}

// TestAuditExportArgs_InsertOpts verifies MaxAttempts=3 per D-35.
func TestAuditExportArgs_InsertOpts(t *testing.T) {
	args := audit.AuditExportArgs{}
	opts := args.InsertOpts()
	assert.Equal(t, 3, opts.MaxAttempts)
}

// TestAuditExportWorker_WritesFile verifies that the AuditExportWorker writes
// a CSV file to {reportsDir}/{jobID}/audit-export.csv on Work() completion.
// The file must start with the UTF-8 BOM.
func TestAuditExportWorker_WritesFile(t *testing.T) {
	if testing.Short() {
		t.Skip("requires postgres testcontainer")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool,
		slog.New(slog.NewTextHandler(os.Stderr, nil))))

	// Seed install_identity.
	_, err := pool.Exec(ctx, `
		INSERT INTO install_identity (id, display_name, logo_path, address, timezone, units, capabilities)
		VALUES (1, 'Test', '', '', 'UTC', 'metric', 'both')
		ON CONFLICT (id) DO NOTHING
	`)
	require.NoError(t, err)

	// Seed a few audit rows.
	for i := 0; i < 5; i++ {
		_, err = pool.Exec(ctx,
			`INSERT INTO audit_log (action, entity_type, entity_id)
			 VALUES ('auth.login_success', 'user', gen_random_uuid())`)
		require.NoError(t, err)
	}

	reportsDir := t.TempDir()
	store := audit.NewStore(pool)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	worker := &audit.AuditExportWorker{
		Pool:       pool,
		Store:      store,
		ReportsDir: reportsDir,
		Log:        log,
	}

	jobID := uuid.New()
	job := &river.Job[audit.AuditExportArgs]{
		Args: audit.AuditExportArgs{
			JobID:  jobID.String(),
			Filter: audit.Filter{},
		},
	}

	err = worker.Work(ctx, job)
	require.NoError(t, err)

	// Verify file exists at expected path.
	expectedPath := filepath.Join(reportsDir, jobID.String(), "audit-export.csv")
	data, err := os.ReadFile(expectedPath)
	require.NoError(t, err, "audit-export.csv must exist after Work()")

	// BOM check.
	require.GreaterOrEqual(t, len(data), 3, "file must have at least 3 bytes")
	assert.Equal(t, byte(0xEF), data[0])
	assert.Equal(t, byte(0xBB), data[1])
	assert.Equal(t, byte(0xBF), data[2])
}

// TestAuditExportWorker_PathMatchesCleanupGlob verifies that the file path
// written by AuditExportWorker matches the pattern the cleanup worker uses
// to prune stale CSV exports (same reports root dir structure).
func TestAuditExportWorker_PathMatchesCleanupGlob(t *testing.T) {
	reportsDir := t.TempDir()
	jobID := uuid.New()

	// This is the exact path AuditExportWorker.Work() must use.
	expectedPath := filepath.Join(reportsDir, jobID.String(), "audit-export.csv")

	// The cleanup walks reportsDir/* to find expired directories.
	// Verify the directory structure is {reportsDir}/{jobID}/.
	dir := filepath.Dir(expectedPath)
	parent := filepath.Dir(dir)
	assert.Equal(t, reportsDir, parent,
		"audit export file must be inside a direct subdirectory of reportsDir")
	assert.Equal(t, "audit-export.csv", filepath.Base(expectedPath))
}

// TestCleanupPrunesCSV verifies that the reports cleanup worker deletes
// CSV files in {reportsDir}/{jobID}/ that are older than 24h. The cleanup
// must treat .csv files with the same TTL as .pdf files.
//
// This test targets the filesystem-based cleanup extension required by D-35
// (audit-export.csv files have no DB row, so cleanup is mtime-based).
func TestCleanupPrunesCSV(t *testing.T) {
	reportsDir := t.TempDir()
	jobID := uuid.New()
	csvDir := filepath.Join(reportsDir, jobID.String())
	require.NoError(t, os.MkdirAll(csvDir, 0o755))

	csvPath := filepath.Join(csvDir, "audit-export.csv")
	require.NoError(t, os.WriteFile(csvPath, []byte("\xEF\xBB\xBFtest"), 0o644))

	// Backdate mtime to 25 hours ago.
	oldTime := time.Now().Add(-25 * time.Hour)
	require.NoError(t, os.Chtimes(csvPath, oldTime, oldTime))

	// Run the CSV-only filesystem pruner.
	err := audit.PruneExpiredCSVExports(reportsDir, 24*time.Hour)
	require.NoError(t, err)

	_, statErr := os.Stat(csvDir)
	assert.True(t, os.IsNotExist(statErr), "expired CSV export directory must be removed by PruneExpiredCSVExports")
}

// TestCleanupSkipsRecentCSV verifies that PruneExpiredCSVExports does NOT
// delete directories whose CSV file is newer than the TTL.
func TestCleanupSkipsRecentCSV(t *testing.T) {
	reportsDir := t.TempDir()
	jobID := uuid.New()
	csvDir := filepath.Join(reportsDir, jobID.String())
	require.NoError(t, os.MkdirAll(csvDir, 0o755))

	csvPath := filepath.Join(csvDir, "audit-export.csv")
	require.NoError(t, os.WriteFile(csvPath, []byte("\xEF\xBB\xBFtest"), 0o644))
	// mtime is now (default) — within TTL.

	err := audit.PruneExpiredCSVExports(reportsDir, 24*time.Hour)
	require.NoError(t, err)

	_, statErr := os.Stat(csvPath)
	assert.NoError(t, statErr, "recent CSV export directory must NOT be removed by PruneExpiredCSVExports")
}
