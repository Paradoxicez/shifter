package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
	"github.com/stretchr/testify/require"
)

// setupTestRunner starts a TimescaleDB container, applies migrations, and
// returns a Runner ready for integration tests.
func setupTestRunner(t *testing.T, csMode string) (*Runner, *pgxpool.Pool) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test: -short")
	}
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(io.Discard, nil))))

	store := NewStore(pool)
	cfg := RunnerConfig{
		DBHost:         pool.Config().ConnConfig.Host,
		DBPort:         int(pool.Config().ConnConfig.Port),
		DBUser:         pool.Config().ConnConfig.User,
		DBName:         pool.Config().ConnConfig.Database,
		DBPassword:     "shifter", // testcontainers default
		ChirpStackMode: csMode,
		FloorPlansDir:  t.TempDir(),
		InstallSlug:    "test-install",
		InstallID:      "00000000-0000-0000-0000-000000000001",
		SchemaVersion:  "46",
	}
	runner := &Runner{
		Pool:  pool,
		Store: store,
		Cfg:   cfg,
		Log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	return runner, pool
}

// TestRunner_External_DumpsShifterOnly verifies that in external mode the
// tarball contains db/shifter.dump but NOT db/chirpstack.dump.
func TestRunner_External_DumpsShifterOnly(t *testing.T) {
	runner, _ := setupTestRunner(t, "external")
	destDir := t.TempDir()

	_, tarPath, err := runner.Backup(context.Background(), destDir, "cli", nil)
	require.NoError(t, err)
	require.NotEmpty(t, tarPath)

	entries := listTarEntries(t, tarPath)
	require.Contains(t, entries, "db/shifter.dump", "external backup must include db/shifter.dump")
	for _, e := range entries {
		require.NotEqual(t, "db/chirpstack.dump", e, "external backup must NOT include db/chirpstack.dump")
	}
	require.Contains(t, entries, "manifest.json")
}

// TestRunner_PgDumpFlags asserts that the pg_dump invocation uses
// --format=custom, --no-owner, --no-acl, and NEVER --jobs or -j.
func TestRunner_PgDumpFlags(t *testing.T) {
	t.Parallel()
	// Static assertion: read runner.go source and verify the args slice
	// contains required flags and NO -j / --jobs.
	src, err := os.ReadFile("runner.go")
	require.NoError(t, err)
	srcStr := string(src)

	require.Contains(t, srcStr, `"--format=custom"`, "args must include --format=custom")
	require.Contains(t, srcStr, `"--no-owner"`, "args must include --no-owner")
	require.Contains(t, srcStr, `"--no-acl"`, "args must include --no-acl")
	// Pitfall 1: no parallel flag.
	require.NotContains(t, srcStr, `"--jobs"`, "args MUST NOT contain --jobs (Pitfall 1 — breaks TimescaleDB)")
	require.NotContains(t, srcStr, `"-j"`, "args MUST NOT contain -j (Pitfall 1)")
}

// TestRunner_TarballNameConvention verifies the produced filename matches
// shifter-backup-{slug}-{YYYYMMDD-HHMM}-{schema}.tar.gz
func TestRunner_TarballNameConvention(t *testing.T) {
	runner, _ := setupTestRunner(t, "external")
	destDir := t.TempDir()

	_, tarPath, err := runner.Backup(context.Background(), destDir, "cli", nil)
	require.NoError(t, err)

	base := filepath.Base(tarPath)
	require.True(t, strings.HasPrefix(base, "shifter-backup-test-install-"),
		"tarball name must start with shifter-backup-{slug}-: got %s", base)
	require.True(t, strings.HasSuffix(base, ".tar.gz"),
		"tarball must end with .tar.gz: got %s", base)
	require.Contains(t, base, "-46.", "tarball name must embed schema version: got %s", base)
}

// TestRunner_WritesAuditAndBackupRun verifies that a successful backup writes:
//   - one backup_run row with status='completed'
//   - audit rows for 'backup.start' and 'backup.complete'
func TestRunner_WritesAuditAndBackupRun(t *testing.T) {
	runner, pool := setupTestRunner(t, "external")
	destDir := t.TempDir()

	backupID, _, err := runner.Backup(context.Background(), destDir, "cli", nil)
	require.NoError(t, err)

	// Verify backup_run row.
	row, err := runner.Store.Get(context.Background(), backupID)
	require.NoError(t, err)
	require.NotNil(t, row)
	require.Equal(t, "completed", row.Status)
	require.NotNil(t, row.FinishedAt)
	require.NotNil(t, row.FileSizeBytes)
	require.Greater(t, *row.FileSizeBytes, int64(0))

	// Verify audit rows for backup.start and backup.complete.
	var startCount, completeCount int
	ctx := context.Background()
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE action='backup.start' AND entity_type='backup_run' AND entity_id=$1`,
		backupID,
	).Scan(&startCount))
	require.Equal(t, 1, startCount, "must have one backup.start audit row")

	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE action='backup.complete' AND entity_type='backup_run' AND entity_id=$1`,
		backupID,
	).Scan(&completeCount))
	require.Equal(t, 1, completeCount, "must have one backup.complete audit row")
}

// TestRunner_FailureWritesAuditFailed forces a pg_dump failure (bad dbname)
// and verifies that backup_run.status='failed' and a 'backup.failed' audit row
// is written.
func TestRunner_FailureWritesAuditFailed(t *testing.T) {
	runner, pool := setupTestRunner(t, "external")
	// Corrupt the DB name so pg_dump fails.
	runner.Cfg.DBName = "nonexistent_db_does_not_exist"
	destDir := t.TempDir()

	backupID, _, err := runner.Backup(context.Background(), destDir, "cli", nil)
	require.Error(t, err)
	require.NotEqual(t, backupID.String(), "00000000-0000-0000-0000-000000000000",
		"backup_run ID should be populated even on failure")

	row, dbErr := runner.Store.Get(context.Background(), backupID)
	require.NoError(t, dbErr)
	require.NotNil(t, row)
	require.Equal(t, "failed", row.Status)
	require.NotNil(t, row.ErrorMessage)
	require.NotEmpty(t, *row.ErrorMessage)

	var failCount int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE action='backup.failed' AND entity_type='backup_run' AND entity_id=$1`,
		backupID,
	).Scan(&failCount))
	require.Equal(t, 1, failCount, "must have one backup.failed audit row")
}

// TestConfig_BackupDir verifies the DefaultBackupDir constant.
func TestConfig_BackupDir(t *testing.T) {
	t.Parallel()
	require.Equal(t, "/var/lib/shifter/backups", DefaultBackupDir,
		"DefaultBackupDir must match documented default")
}

// listTarEntries opens a .tar.gz and returns all entry names.
func listTarEntries(t *testing.T, tarPath string) []string {
	t.Helper()
	f, err := os.Open(tarPath)
	require.NoError(t, err)
	defer f.Close()

	gz, err := gzip.NewReader(f)
	require.NoError(t, err)
	defer gz.Close()

	tr := tar.NewReader(gz)
	var names []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		names = append(names, hdr.Name)
	}
	return names
}
