package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
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

// setupTestRestorer starts a TimescaleDB container, applies migrations, and
// returns a Restorer ready for integration tests.
func setupTestRestorer(t *testing.T) (*Restorer, *pgxpool.Pool) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test: -short")
	}
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(io.Discard, nil))))

	cfg := RestorerConfig{
		DBHost:     pool.Config().ConnConfig.Host,
		DBPort:     int(pool.Config().ConnConfig.Port),
		DBUser:     pool.Config().ConnConfig.User,
		DBName:     pool.Config().ConnConfig.Database,
		DBPassword: "shifter", // testcontainers default
		// ChirpStack fields used only in bundled mode tests.
		ChirpStackDBName: "chirpstack",
		ChirpStackDBUser: "chirpstack",
		FloorPlansDir:    t.TempDir(),
	}
	restorer := NewRestorer(pool, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return restorer, pool
}

// setupTestRunnerAndRestorer returns both a Runner and Restorer sharing the
// same pool, for round-trip tests within this package.
func setupTestRunnerAndRestorer(t *testing.T) (*Runner, *Restorer, *pgxpool.Pool) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test: -short")
	}
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(io.Discard, nil))))

	fpDir := t.TempDir()
	rCfg := RunnerConfig{
		DBHost:         pool.Config().ConnConfig.Host,
		DBPort:         int(pool.Config().ConnConfig.Port),
		DBUser:         pool.Config().ConnConfig.User,
		DBName:         pool.Config().ConnConfig.Database,
		DBPassword:     "shifter",
		ChirpStackMode: "external",
		FloorPlansDir:  fpDir,
		InstallSlug:    "test-install",
		InstallID:      "00000000-0000-0000-0000-000000000001",
		SchemaVersion:  "46",
	}
	store := NewStore(pool)
	runner := &Runner{Pool: pool, Store: store, Cfg: rCfg, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}

	// Restorer uses the same FloorPlansDir so backup → restore is truly round-trip.
	restCfg := RestorerConfig{
		DBHost:        pool.Config().ConnConfig.Host,
		DBPort:        int(pool.Config().ConnConfig.Port),
		DBUser:        pool.Config().ConnConfig.User,
		DBName:        pool.Config().ConnConfig.Database,
		DBPassword:    "shifter",
		FloorPlansDir: fpDir,
	}
	restorer := NewRestorer(pool, restCfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return runner, restorer, pool
}

// TestRestorer_AdvisoryLockConst verifies the lock constant is the expected hex value.
func TestRestorer_AdvisoryLockConst(t *testing.T) {
	t.Parallel()
	// 0x5348494654455231 == hex encoding of "SHIFTER1"
	require.Equal(t, int64(0x5348494654455231), ShifterAdvisoryLockID)
}

// TestRestorer_RefusesIfShifterRunning acquires the advisory lock from a
// parallel goroutine and asserts that Restore returns ErrShifterStillServing.
func TestRestorer_RefusesIfShifterRunning(t *testing.T) {
	restorer, pool := setupTestRestorer(t)

	// Acquire the lock in a separate connection so it looks like Shifter is
	// already serving.
	conn, err := pool.Acquire(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { conn.Release() })

	var acquired bool
	require.NoError(t,
		conn.QueryRow(context.Background(), `SELECT pg_try_advisory_lock($1)`, ShifterAdvisoryLockID).Scan(&acquired),
	)
	require.True(t, acquired, "pre-acquire must succeed in test setup")

	// Create a minimal tarball so the restorer doesn't fail before trying the lock.
	tarPath := makeDummyTarball(t, nil)

	err = restorer.Restore(context.Background(), tarPath, "")
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrShifterStillServing),
		"expected ErrShifterStillServing; got: %v", err)
}

// TestRestorer_VerifiesOuterSHA256 tampers with the tarball and asserts that
// Restore returns ErrTarballChecksumMismatch.
func TestRestorer_VerifiesOuterSHA256(t *testing.T) {
	restorer, _ := setupTestRestorer(t)

	tarPath := makeDummyTarball(t, nil)
	// Compute real sha256 then corrupt the file.
	real, err := ComputeFileSHA256(tarPath)
	require.NoError(t, err)

	// Corrupt the file by appending a byte.
	f, err := os.OpenFile(tarPath, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.Write([]byte{0x00})
	require.NoError(t, err)
	f.Close()

	err = restorer.Restore(context.Background(), tarPath, real /* now stale */)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrTarballChecksumMismatch),
		"expected ErrTarballChecksumMismatch; got: %v", err)
}

// TestRestorer_VerifiesPerFileSHA256 creates a tarball with a corrupt per-file
// sha256 in the manifest and asserts that Restore returns ErrFileChecksumMismatch.
func TestRestorer_VerifiesPerFileSHA256(t *testing.T) {
	restorer, _ := setupTestRestorer(t)

	tarPath := makeDummyTarball(t, map[string]string{
		"db/shifter.dump": "0000000000000000000000000000000000000000000000000000000000000000", // wrong
	})
	err := restorer.Restore(context.Background(), tarPath, "")
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrFileChecksumMismatch),
		"expected ErrFileChecksumMismatch; got: %v", err)
}

// TestRestorer_NeverUsesJobsFlag reads restore.go source and asserts that the
// pg_restore args slice does not include -j or --jobs as positional arguments.
//
// The file may contain "-j" as a string literal inside a guard/sanity-check
// (the runtime belt-and-suspenders enforcement of Pitfall 1). The test checks
// for the absence of --jobs in the hardcoded args []string{...} block to
// confirm no caller-injected parallelism is possible.
func TestRestorer_NeverUsesJobsFlag(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile("restore.go")
	require.NoError(t, err)
	srcStr := string(src)

	// The args slice for pg_restore must contain --no-owner and --no-acl.
	require.Contains(t, srcStr, `"--no-owner"`, "pg_restore args must include --no-owner")
	require.Contains(t, srcStr, `"--no-acl"`, "pg_restore args must include --no-acl")

	// Count lines containing --jobs or -j in the args slice literal (not in
	// guard/comment/string). A correct implementation has exactly ZERO such lines
	// in the hardcoded args = []string{...} block. Guard checks that reference
	// these strings are OK — they detect injection; they don't inject.
	//
	// We verify by asserting the args slice construction lines do not start with
	// a quoted jobs flag entry.
	for _, line := range strings.Split(srcStr, "\n") {
		trimmed := strings.TrimSpace(line)
		// Skip comment lines, guard condition lines, and error message lines.
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		if strings.Contains(trimmed, "==") || strings.Contains(trimmed, "HasPrefix") {
			continue // guard comparison — allowed
		}
		if strings.Contains(trimmed, "fmt.Errorf") || strings.Contains(trimmed, "return fmt") {
			continue // error messages — allowed
		}
		// Any line that places "-j" or "--jobs=" as a standalone args entry is forbidden.
		require.False(t,
			trimmed == `"-j",` || trimmed == `"--jobs",` || strings.HasPrefix(trimmed, `"--jobs=`),
			"pg_restore args must not include -j/--jobs: found forbidden line: %q", trimmed)
	}
}

// TestRestorer_SourceContainsAdvisoryLock verifies that the source file uses
// the correct pg advisory lock functions.
func TestRestorer_SourceContainsAdvisoryLock(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile("restore.go")
	require.NoError(t, err)
	srcStr := string(src)
	require.Contains(t, srcStr, "pg_try_advisory_lock", "restore.go must call pg_try_advisory_lock")
	require.Contains(t, srcStr, "pg_advisory_unlock", "restore.go must call pg_advisory_unlock")
}

// TestRestorer_SourceContainsTimescaleHooks verifies pre/post restore calls in source.
func TestRestorer_SourceContainsTimescaleHooks(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile("restore.go")
	require.NoError(t, err)
	srcStr := string(src)
	require.Contains(t, srcStr, "timescaledb_pre_restore", "restore.go must call timescaledb_pre_restore")
	require.Contains(t, srcStr, "timescaledb_post_restore", "restore.go must call timescaledb_post_restore")
}

// TestRestorer_SourceUsesAuditWriteEntry verifies that the source uses
// audit.WriteEntry and never raw INSERT INTO audit_log.
func TestRestorer_SourceUsesAuditWriteEntry(t *testing.T) {
	t.Parallel()
	src, err := os.ReadFile("restore.go")
	require.NoError(t, err)
	srcStr := string(src)
	require.Contains(t, srcStr, "audit.WriteEntry", "restore.go must use audit.WriteEntry")
	require.NotContains(t, srcStr, "INSERT INTO audit_log", "restore.go must NOT use raw INSERT INTO audit_log")
}

// TestRestorer_ExternalMode_SkipsChirpstack backs up a tarball with
// chirpstack_db_included=false and verifies Restore does not try to touch a
// ChirpStack DB.
func TestRestorer_ExternalMode_SkipsChirpstack(t *testing.T) {
	restorer, pool := setupTestRestorer(t)

	// Build a tarball without a chirpstack dump.
	tarPath := makeRealBackupTarball(t, pool, "external")

	// Restore; if it tries to access ChirpStack DB it will fail (no such DB).
	err := restorer.Restore(context.Background(), tarPath, "")
	require.NoError(t, err, "external mode restore must not error")
}

// TestRestorer_FloorPlansRsync backs up a tarball with a floor plan file and
// verifies the file exists in the restore destination after restore.
func TestRestorer_FloorPlansRsync(t *testing.T) {
	runner, restorer, pool := setupTestRunnerAndRestorer(t)
	_ = pool

	// Place a test floor plan file in the runner's source dir.
	fpContent := []byte("fake-png-bytes-for-test")
	fpFile := filepath.Join(runner.Cfg.FloorPlansDir, "test-floor.png")
	require.NoError(t, os.WriteFile(fpFile, fpContent, 0o644))

	destDir := t.TempDir()
	_, tarPath, err := runner.Backup(context.Background(), destDir, "cli", nil)
	require.NoError(t, err)

	// Clear the floor plans dir to simulate a fresh restore target.
	require.NoError(t, os.Remove(fpFile))

	// Restore.
	err = restorer.Restore(context.Background(), tarPath, "")
	require.NoError(t, err)

	// Assert the floor plan is present.
	restoredContent, err := os.ReadFile(fpFile)
	require.NoError(t, err)
	require.Equal(t, fpContent, restoredContent, "floor plan file must be restored with identical content")
}

// TestRestorer_WritesAuditOnSuccess verifies that after a successful restore,
// a 'backup.restore' audit row exists.
func TestRestorer_WritesAuditOnSuccess(t *testing.T) {
	runner, restorer, pool := setupTestRunnerAndRestorer(t)

	destDir := t.TempDir()
	_, tarPath, err := runner.Backup(context.Background(), destDir, "cli", nil)
	require.NoError(t, err)

	err = restorer.Restore(context.Background(), tarPath, "")
	require.NoError(t, err)

	var count int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE action='backup.restore' AND entity_type='backup_run'`,
	).Scan(&count))
	require.GreaterOrEqual(t, count, 1, "must have at least one backup.restore audit row")
}

// TestRestorer_AbortsAndReleasesLockOnError verifies that if the restore fails
// mid-way, the advisory lock is released so a subsequent restore can succeed.
func TestRestorer_AbortsAndReleasesLockOnError(t *testing.T) {
	restorer, _ := setupTestRestorer(t)

	// Use a tarball with a bad per-file sha256 to cause a failure after the
	// advisory lock is acquired (the outer sha256 check is skipped so we get
	// past the advisory lock step).
	tarPath := makeDummyTarball(t, map[string]string{
		"db/shifter.dump": "badsha256badsha256badsha256badsha256badsha256badsha256badsha256ba",
	})

	err := restorer.Restore(context.Background(), tarPath, "")
	require.Error(t, err, "tampered tarball must fail")

	// A second restore attempt must not fail with ErrShifterStillServing
	// (the lock must have been released even on the first attempt's failure).
	tarPath2 := makeDummyTarball(t, map[string]string{
		"db/shifter.dump": "badsha256badsha256badsha256badsha256badsha256badsha256badsha256ba",
	})
	err2 := restorer.Restore(context.Background(), tarPath2, "")
	// This should also fail (bad sha), but NOT with ErrShifterStillServing.
	require.False(t, errors.Is(err2, ErrShifterStillServing),
		"second restore attempt must not fail with ErrShifterStillServing (lock must be released)")
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// makeDummyTarball creates a minimal tar.gz file with a manifest.json and a
// dummy db/shifter.dump entry. sha256Overrides lets the caller inject wrong
// checksums into the manifest to trigger per-file verification failures.
// If sha256Overrides is nil, real checksums are computed.
func makeDummyTarball(t *testing.T, sha256Overrides map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	tarPath := filepath.Join(dir, "test-backup.tar.gz")

	f, err := os.Create(tarPath)
	require.NoError(t, err)
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()
	tw := tar.NewWriter(gw)
	defer tw.Close()

	// Write a dummy db/shifter.dump.
	dumpContent := []byte("PGDMP-fake-content-for-testing")
	dumpSHA, err := ComputeReaderSHA256(newBytesReader(dumpContent))
	require.NoError(t, err)

	writeEntry := func(name string, data []byte) {
		hdr := &tar.Header{Name: name, Size: int64(len(data)), Mode: 0o644}
		require.NoError(t, tw.WriteHeader(hdr))
		_, writeErr := tw.Write(data)
		require.NoError(t, writeErr)
	}

	writeEntry("db/shifter.dump", dumpContent)

	// Build manifest.
	sums := map[string]string{"db/shifter.dump": dumpSHA}
	if sha256Overrides != nil {
		for k, v := range sha256Overrides {
			sums[k] = v
		}
	}
	mf := Manifest{
		ManifestVersion: "1.0",
		DBSchemaVersion: "46",
		ChirpStackMode:  "external",
		InstallID:       "00000000-0000-0000-0000-000000000001",
		InstallSlug:     "test-install",
		SHA256Sums:      sums,
		Included:        []string{"db/shifter.dump"},
	}
	manifestJSON, err := json.Marshal(mf)
	require.NoError(t, err)
	writeEntry("manifest.json", manifestJSON)

	return tarPath
}

// makeRealBackupTarball uses a real Runner to create a backup tarball in the
// given csMode. pg_dump must be available in PATH.
func makeRealBackupTarball(t *testing.T, pool *pgxpool.Pool, csMode string) string {
	t.Helper()
	fpDir := t.TempDir()
	store := NewStore(pool)
	cfg := RunnerConfig{
		DBHost:         pool.Config().ConnConfig.Host,
		DBPort:         int(pool.Config().ConnConfig.Port),
		DBUser:         pool.Config().ConnConfig.User,
		DBName:         pool.Config().ConnConfig.Database,
		DBPassword:     "shifter",
		ChirpStackMode: csMode,
		FloorPlansDir:  fpDir,
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
	destDir := t.TempDir()
	_, tarPath, err := runner.Backup(context.Background(), destDir, "cli", nil)
	require.NoError(t, err)
	return tarPath
}

// newBytesReader returns an io.Reader wrapping the given byte slice.
func newBytesReader(data []byte) io.Reader {
	return &bytesReadCloser{data: data, pos: 0}
}

type bytesReadCloser struct {
	data []byte
	pos  int
}

func (b *bytesReadCloser) Read(p []byte) (int, error) {
	if b.pos >= len(b.data) {
		return 0, io.EOF
	}
	n := copy(p, b.data[b.pos:])
	b.pos += n
	return n, nil
}
