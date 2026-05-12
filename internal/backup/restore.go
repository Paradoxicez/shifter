package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shifter-io/shifter/internal/audit"
)

// ShifterAdvisoryLockID is the 64-bit PG advisory lock constant used to detect
// whether the Shifter HTTP server is currently running.
//
// The value 0x5348494654455231 is the little-endian hex encoding of "SHIFTER1"
// (ASCII bytes 0x53 0x48 0x49 0x46 0x54 0x45 0x52 0x31).
//
// The Shifter serve command acquires this lock at startup (T-06-09-02);
// restore refuses to proceed if it cannot be acquired.
const ShifterAdvisoryLockID int64 = 0x5348494654455231

var (
	// ErrShifterStillServing is returned when the PG advisory lock cannot be
	// acquired because Shifter is still serving HTTP requests.
	ErrShifterStillServing = errors.New(
		"restore: Shifter is still serving — stop the Shifter container before restoring " +
			"(docker compose stop shifter) then retry",
	)

	// ErrTarballChecksumMismatch is returned when the outer tarball sha256 does
	// not match the expected value passed via --expected-sha256.
	ErrTarballChecksumMismatch = errors.New("restore: outer tarball sha256 mismatch")

	// ErrFileChecksumMismatch is returned when a file inside the tarball does
	// not match the sha256 recorded in manifest.json.
	ErrFileChecksumMismatch = errors.New("restore: per-file sha256 mismatch")

	// ErrManifestMissing is returned when the tarball does not contain a
	// manifest.json entry.
	ErrManifestMissing = errors.New("restore: manifest.json not found in tarball")
)

// RestorerConfig holds the Postgres connection parameters the Restorer needs
// to orchestrate DROP/CREATE DATABASE, psql hooks, and pg_restore.
type RestorerConfig struct {
	// Shifter DB connection.
	DBHost     string
	DBPort     int
	DBUser     string
	DBName     string
	DBPassword string

	// ChirpStack DB (used only when manifest.chirpstack_db_included is true).
	ChirpStackDBName string
	ChirpStackDBUser string

	// FloorPlansDir is the target directory for restored floor-plan files.
	// Defaults to /var/lib/shifter/floor-plans if empty.
	FloorPlansDir string
}

// Restorer restores a Shifter backup tarball produced by Runner.Backup.
//
// Safety properties (D-44, T-06-09-01..04):
//   - Acquires PG advisory lock before touching any DB; refuses with
//     ErrShifterStillServing if the lock is held (i.e., Shifter is serving).
//   - Verifies the optional outer tarball sha256 (--expected-sha256 flag).
//   - Verifies every file's sha256 against manifest.json before pg_restore runs.
//   - Wraps pg_restore in timescaledb_pre_restore() / timescaledb_post_restore()
//     for the Shifter DB.  ChirpStack DB skips these hooks (no TimescaleDB).
//   - NEVER passes -j / --jobs to pg_restore (TimescaleDB Pitfall 1).
//   - Writes a 'backup.restore' audit row on success.
type Restorer struct {
	Pool *pgxpool.Pool
	Cfg  RestorerConfig
	Log  *slog.Logger
}

// NewRestorer returns a Restorer backed by pool.
func NewRestorer(pool *pgxpool.Pool, cfg RestorerConfig, log *slog.Logger) *Restorer {
	return &Restorer{Pool: pool, Cfg: cfg, Log: log}
}

// Restore reads the tarball at srcPath, verifies checksums, runs the
// TimescaleDB restore sequence for each DB in the manifest, rsyncs floor
// plans, and writes a 'backup.restore' audit row.
//
// If expectedOuterSHA256 is non-empty the tarball is checked against it before
// any DB operations begin.  Pass empty string to skip the outer check (the
// per-file manifest checks are always enforced).
func (r *Restorer) Restore(ctx context.Context, srcPath string, expectedOuterSHA256 string) error {
	// ------------------------------------------------------------------ //
	// Step 1: Optional outer tarball sha256 check (T-06-09-01).          //
	// ------------------------------------------------------------------ //
	if expectedOuterSHA256 != "" {
		actual, err := ComputeFileSHA256(srcPath)
		if err != nil {
			return fmt.Errorf("compute outer sha256: %w", err)
		}
		if actual != expectedOuterSHA256 {
			return fmt.Errorf("%w: expected %s got %s",
				ErrTarballChecksumMismatch, expectedOuterSHA256, actual)
		}
	}

	// ------------------------------------------------------------------ //
	// Step 2: Acquire advisory lock.  Refuse if Shifter is serving.      //
	// ------------------------------------------------------------------ //
	var acquired bool
	if err := r.Pool.QueryRow(ctx,
		`SELECT pg_try_advisory_lock($1)`, ShifterAdvisoryLockID,
	).Scan(&acquired); err != nil {
		return fmt.Errorf("acquire advisory lock: %w", err)
	}
	if !acquired {
		return ErrShifterStillServing
	}
	defer func() {
		// Release on all exit paths (success or error) using a background ctx
		// so a cancelled ctx does not skip the release.
		_, _ = r.Pool.Exec(context.Background(),
			`SELECT pg_advisory_unlock($1)`, ShifterAdvisoryLockID)
	}()

	// ------------------------------------------------------------------ //
	// Step 3: Extract the tarball to a temp directory.                   //
	// ------------------------------------------------------------------ //
	tmpDir, err := os.MkdirTemp("", "shifter-restore-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	if err := extractTarGz(srcPath, tmpDir); err != nil {
		return fmt.Errorf("extract tarball: %w", err)
	}

	// ------------------------------------------------------------------ //
	// Step 4: Parse manifest.json.                                        //
	// ------------------------------------------------------------------ //
	manifestBytes, err := os.ReadFile(filepath.Join(tmpDir, "manifest.json"))
	if err != nil {
		return ErrManifestMissing
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}

	// ------------------------------------------------------------------ //
	// Step 5: Verify per-file sha256 sums (T-06-09-01).                  //
	// ------------------------------------------------------------------ //
	for relPath, expectedSum := range manifest.SHA256Sums {
		if relPath == "manifest.json" {
			continue // manifest cannot self-verify
		}
		fullPath := filepath.Join(tmpDir, filepath.FromSlash(relPath))
		info, err := os.Stat(fullPath)
		if err != nil {
			return fmt.Errorf("manifest references missing file %q: %w", relPath, err)
		}
		if info.IsDir() {
			continue // directory entries in the map are skipped; per-file entries cover contents
		}
		actual, err := ComputeFileSHA256(fullPath)
		if err != nil {
			return fmt.Errorf("compute sha256 for %q: %w", relPath, err)
		}
		if actual != expectedSum {
			return fmt.Errorf("%w: file=%s expected=%s actual=%s",
				ErrFileChecksumMismatch, relPath, expectedSum, actual)
		}
	}

	// ------------------------------------------------------------------ //
	// Step 6: Restore each DB.  Shifter first; ChirpStack second if      //
	// included (D-44 ordering).                                           //
	// ------------------------------------------------------------------ //
	shifterDump := filepath.Join(tmpDir, "db", "shifter.dump")
	if err := r.restoreDB(ctx, shifterDump, r.Cfg.DBUser, r.Cfg.DBName, true /* isShifterDB */); err != nil {
		return fmt.Errorf("restore Shifter DB: %w", err)
	}

	if manifest.ChirpStackDBIncluded {
		csDBName := r.Cfg.ChirpStackDBName
		if csDBName == "" {
			csDBName = "chirpstack"
		}
		csDBUser := r.Cfg.ChirpStackDBUser
		if csDBUser == "" {
			csDBUser = "chirpstack"
		}
		chirpDump := filepath.Join(tmpDir, "db", "chirpstack.dump")
		if err := r.restoreDB(ctx, chirpDump, csDBUser, csDBName, false /* isShifterDB */); err != nil {
			return fmt.Errorf("restore ChirpStack DB: %w", err)
		}
	}

	// ------------------------------------------------------------------ //
	// Step 7: Rsync floor plans from the extracted tarball dir.          //
	// ------------------------------------------------------------------ //
	srcFloorPlansDir := filepath.Join(tmpDir, "floor-plans")
	if _, statErr := os.Stat(srcFloorPlansDir); statErr == nil {
		destDir := r.Cfg.FloorPlansDir
		if destDir == "" {
			destDir = "/var/lib/shifter/floor-plans"
		}
		if err := copyDir(srcFloorPlansDir, destDir); err != nil {
			return fmt.Errorf("rsync floor-plans: %w", err)
		}
	}

	// ------------------------------------------------------------------ //
	// Step 8: Write 'backup.restore' audit row (T-06-09-07).             //
	// ------------------------------------------------------------------ //
	entityID, err := parseInstallUUID(manifest.InstallID)
	if err != nil {
		// Non-fatal: use nil UUID rather than aborting a successful restore.
		r.Log.Warn("restore: cannot parse manifest install_id for audit row",
			"install_id", manifest.InstallID, "err", err)
	}
	tx, err := r.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin audit tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if auditErr := audit.WriteEntry(ctx, tx, audit.Entry{
		Action:     audit.ActionBackupRestore,
		EntityType: audit.EntityTypeBackupRun,
		EntityID:   entityID,
		Notes: fmt.Sprintf(
			"restored from %s (schema=%s, install=%s)",
			filepath.Base(srcPath), manifest.DBSchemaVersion, manifest.InstallSlug,
		),
	}); auditErr != nil {
		return fmt.Errorf("write restore audit row: %w", auditErr)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit audit tx: %w", err)
	}

	r.Log.Info("restore completed",
		"tarball", filepath.Base(srcPath),
		"schema", manifest.DBSchemaVersion,
		"install", manifest.InstallSlug,
	)
	return nil
}

// RestoreInPlace is like Restore but skips the DROP DATABASE / CREATE DATABASE
// step. It is used by the integration round-trip test (TestBackupRestoreRoundtrip)
// where the test harness already performed DROP SCHEMA CASCADE + CREATE SCHEMA
// to simulate a fresh database within the same testcontainers-managed DB
// instance (testcontainers cannot expose a separate maintenance connection to
// the "postgres" DB that the standard DROP DATABASE flow requires).
//
// Production callers (shifter restore CLI) always use Restore(), never
// RestoreInPlace(). RestoreInPlace is intentionally un-exported from the
// package-level docs perspective — it carries the "InPlace" suffix as an
// explicit call-site signal.
func (r *Restorer) RestoreInPlace(ctx context.Context, srcPath string, expectedOuterSHA256 string) error {
	// Advisory lock + sha256 + extract + manifest parse + per-file check
	// are identical to Restore().
	if expectedOuterSHA256 != "" {
		actual, err := ComputeFileSHA256(srcPath)
		if err != nil {
			return fmt.Errorf("compute outer sha256: %w", err)
		}
		if actual != expectedOuterSHA256 {
			return fmt.Errorf("%w: expected %s got %s",
				ErrTarballChecksumMismatch, expectedOuterSHA256, actual)
		}
	}

	var acquired bool
	if err := r.Pool.QueryRow(ctx,
		`SELECT pg_try_advisory_lock($1)`, ShifterAdvisoryLockID,
	).Scan(&acquired); err != nil {
		return fmt.Errorf("acquire advisory lock: %w", err)
	}
	if !acquired {
		return ErrShifterStillServing
	}
	defer func() {
		_, _ = r.Pool.Exec(context.Background(),
			`SELECT pg_advisory_unlock($1)`, ShifterAdvisoryLockID)
	}()

	tmpDir, err := os.MkdirTemp("", "shifter-restore-inplace-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	if err := extractTarGz(srcPath, tmpDir); err != nil {
		return fmt.Errorf("extract tarball: %w", err)
	}

	manifestBytes, err := os.ReadFile(filepath.Join(tmpDir, "manifest.json"))
	if err != nil {
		return ErrManifestMissing
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}

	for relPath, expectedSum := range manifest.SHA256Sums {
		if relPath == "manifest.json" {
			continue
		}
		fullPath := filepath.Join(tmpDir, filepath.FromSlash(relPath))
		info, err := os.Stat(fullPath)
		if err != nil {
			return fmt.Errorf("manifest references missing file %q: %w", relPath, err)
		}
		if info.IsDir() {
			continue
		}
		actual, err := ComputeFileSHA256(fullPath)
		if err != nil {
			return fmt.Errorf("compute sha256 for %q: %w", relPath, err)
		}
		if actual != expectedSum {
			return fmt.Errorf("%w: file=%s expected=%s actual=%s",
				ErrFileChecksumMismatch, relPath, expectedSum, actual)
		}
	}

	// Restore Shifter DB in-place (skip DROP/CREATE — schema already fresh).
	shifterDump := filepath.Join(tmpDir, "db", "shifter.dump")
	if err := r.restoreDBInPlace(ctx, shifterDump, r.Cfg.DBUser, r.Cfg.DBName); err != nil {
		return fmt.Errorf("restore Shifter DB in-place: %w", err)
	}

	// Rsync floor plans.
	srcFloorPlansDir := filepath.Join(tmpDir, "floor-plans")
	if _, statErr := os.Stat(srcFloorPlansDir); statErr == nil {
		destDir := r.Cfg.FloorPlansDir
		if destDir == "" {
			destDir = "/var/lib/shifter/floor-plans"
		}
		if err := copyDir(srcFloorPlansDir, destDir); err != nil {
			return fmt.Errorf("rsync floor-plans: %w", err)
		}
	}

	// Write audit row.
	entityID, _ := parseInstallUUID(manifest.InstallID)
	tx, err := r.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin audit tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if auditErr := audit.WriteEntry(ctx, tx, audit.Entry{
		Action:     audit.ActionBackupRestore,
		EntityType: audit.EntityTypeBackupRun,
		EntityID:   entityID,
		Notes: fmt.Sprintf(
			"restored in-place from %s (schema=%s, install=%s)",
			filepath.Base(srcPath), manifest.DBSchemaVersion, manifest.InstallSlug,
		),
	}); auditErr != nil {
		return fmt.Errorf("write restore audit row: %w", auditErr)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit audit tx: %w", err)
	}
	return nil
}

// restoreDBInPlace runs pre_restore → pg_restore → post_restore without
// the DROP/CREATE step. Used by the integration round-trip test only.
func (r *Restorer) restoreDBInPlace(ctx context.Context, dumpPath, dbUser, dbName string) error {
	port := r.Cfg.DBPort
	if port == 0 {
		port = 5432
	}

	// Pre-restore hook (Pitfall 2).
	if err := r.execPsql(ctx, dbName, `SELECT timescaledb_pre_restore();`); err != nil {
		return fmt.Errorf("timescaledb_pre_restore: %w", err)
	}

	// pg_restore — NO -j / --jobs (Pitfall 1).
	args := []string{
		"--host", r.Cfg.DBHost,
		"--port", fmt.Sprintf("%d", port),
		"--username", dbUser,
		"--dbname", dbName,
		"--no-owner",
		"--no-acl",
		dumpPath,
	}
	for _, a := range args {
		if a == "-j" || a == "--jobs" || strings.HasPrefix(a, "--jobs=") {
			return fmt.Errorf("restore: -j/--jobs forbidden (Pitfall 1)")
		}
	}
	cmd := exec.CommandContext(ctx, "pg_restore", args...)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+r.Cfg.DBPassword)
	if out, err := cmd.CombinedOutput(); err != nil {
		outStr := strings.TrimSpace(string(out))
		if outStr != "" {
			r.Log.Warn("pg_restore output (in-place)", "db", dbName, "output", outStr)
		}
		return fmt.Errorf("pg_restore %q in-place: %w: %s", dbName, err, outStr)
	}

	// Post-restore hook (Pitfall 2).
	if err := r.execPsql(ctx, dbName, `SELECT timescaledb_post_restore();`); err != nil {
		return fmt.Errorf("timescaledb_post_restore: %w", err)
	}
	return nil
}

// restoreDB runs the TimescaleDB-aware restore sequence for a single database.
//
// For the Shifter DB (isShifterDB=true):
//  1. DROP DATABASE IF EXISTS + CREATE DATABASE (via psql against "postgres" DB)
//  2. CREATE EXTENSION IF NOT EXISTS timescaledb
//  3. SELECT timescaledb_pre_restore()       ← Pitfall 2 mitigation
//  4. pg_restore --no-owner --no-acl         ← NO -j/--jobs (Pitfall 1 mitigation)
//  5. SELECT timescaledb_post_restore()      ← Pitfall 2 mitigation
//
// For the ChirpStack DB (isShifterDB=false):
//  1. DROP DATABASE IF EXISTS + CREATE DATABASE
//  2. pg_restore --no-owner --no-acl (no TimescaleDB hooks needed)
//
// The integration test TestRestorer_ExternalMode_SkipsChirpstack and the
// roundtrip test TestBackupRestoreRoundtrip validate both code paths.
func (r *Restorer) restoreDB(ctx context.Context, dumpPath, dbUser, dbName string, isShifterDB bool) error {
	port := r.Cfg.DBPort
	if port == 0 {
		port = 5432
	}

	// Step 1: Drop + recreate database using a connection to the "postgres"
	// maintenance DB (cannot DROP the DB you are connected to).
	dropSQL := fmt.Sprintf("DROP DATABASE IF EXISTS %q;", dbName)
	if err := r.execPsql(ctx, "postgres", dropSQL); err != nil {
		return fmt.Errorf("drop db %q: %w", dbName, err)
	}
	createSQL := fmt.Sprintf(`CREATE DATABASE %q WITH OWNER %q;`, dbName, dbUser)
	if err := r.execPsql(ctx, "postgres", createSQL); err != nil {
		return fmt.Errorf("create db %q: %w", dbName, err)
	}

	if isShifterDB {
		// Step 2: Install TimescaleDB extension in the fresh database.
		if err := r.execPsql(ctx, dbName, `CREATE EXTENSION IF NOT EXISTS timescaledb;`); err != nil {
			return fmt.Errorf("create extension timescaledb: %w", err)
		}
		// Step 3: Pre-restore hook (Pitfall 2 — MUST run before pg_restore).
		if err := r.execPsql(ctx, dbName, `SELECT timescaledb_pre_restore();`); err != nil {
			return fmt.Errorf("timescaledb_pre_restore: %w", err)
		}
	}

	// Step 4: pg_restore — NO -j / --jobs (Pitfall 1).
	//
	// Belt-and-suspenders: the args slice is hardcoded (no caller can inject
	// --jobs) AND the loop below panics at runtime if any arg matches -j/--jobs.
	args := []string{
		"--host", r.Cfg.DBHost,
		"--port", fmt.Sprintf("%d", port),
		"--username", dbUser,
		"--dbname", dbName,
		"--no-owner",
		"--no-acl",
		dumpPath,
	}
	// Runtime sanity check (Pitfall 1 belt-and-suspenders):
	for _, a := range args {
		if a == "-j" || a == "--jobs" || strings.HasPrefix(a, "--jobs=") {
			return fmt.Errorf("restore: -j/--jobs forbidden — breaks TimescaleDB catalog ordering (Pitfall 1)")
		}
	}

	cmd := exec.CommandContext(ctx, "pg_restore", args...)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+r.Cfg.DBPassword)
	if out, err := cmd.CombinedOutput(); err != nil {
		// pg_restore exits non-zero for warnings too (e.g., "role does not exist").
		// Log the output but only return an error for actual failures.
		outStr := strings.TrimSpace(string(out))
		if outStr != "" {
			r.Log.Warn("pg_restore output", "db", dbName, "output", outStr)
		}
		return fmt.Errorf("pg_restore %q: %w: %s", dbName, err, outStr)
	}

	if isShifterDB {
		// Step 5: Post-restore hook (Pitfall 2 — MUST run after pg_restore).
		if err := r.execPsql(ctx, dbName, `SELECT timescaledb_post_restore();`); err != nil {
			return fmt.Errorf("timescaledb_post_restore: %w", err)
		}
	}

	return nil
}

// execPsql executes a SQL command against dbName via psql.
func (r *Restorer) execPsql(ctx context.Context, dbName, sql string) error {
	port := r.Cfg.DBPort
	if port == 0 {
		port = 5432
	}
	cmd := exec.CommandContext(ctx, "psql",
		"--host", r.Cfg.DBHost,
		"--port", fmt.Sprintf("%d", port),
		"--username", r.Cfg.DBUser,
		"--dbname", dbName,
		"--command", sql,
	)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+r.Cfg.DBPassword)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("psql -d %q: %w: %s", dbName, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// extractTarGz extracts a .tar.gz archive to destDir.
func extractTarGz(srcPath, destDir string) error {
	f, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open tarball: %w", err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar entry: %w", err)
		}

		// Security: reject entries that would escape destDir.
		target := filepath.Join(destDir, filepath.FromSlash(hdr.Name))
		if !strings.HasPrefix(target+string(os.PathSeparator), destDir+string(os.PathSeparator)) {
			if target != destDir+string(os.PathSeparator)+filepath.Base(target) {
				// Allow entries that are exactly within destDir.
				rel, relErr := filepath.Rel(destDir, target)
				if relErr != nil || strings.HasPrefix(rel, "..") {
					return fmt.Errorf("tarball entry %q would escape destination dir (path traversal)", hdr.Name)
				}
			}
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("mkdir %q: %w", target, err)
			}
		case tar.TypeReg, 0: // 0 is TypeReg in older archives
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("mkdir parent of %q: %w", target, err)
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return fmt.Errorf("create %q: %w", target, err)
			}
			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				return fmt.Errorf("write %q: %w", target, err)
			}
			if err := out.Close(); err != nil {
				return fmt.Errorf("close %q: %w", target, err)
			}
		}
	}
	return nil
}

// copyDir copies all files from srcDir into destDir (creating destDir if
// needed).  Mirrors the "rsync floor-plans" step in RESEARCH §Decision A.
func copyDir(srcDir, destDir string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("mkdir dest %q: %w", destDir, err)
	}
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(destDir, rel)
		if info.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}
		return copyFile(path, dest)
	})
}

// copyFile copies the file at src to dst, creating dst's parent directories
// if necessary.
func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// parseInstallUUID parses a UUID string for the audit row's entity_id.
// Returns uuid.Nil on parse error so a bad manifest UUID never aborts a
// successful restore.
func parseInstallUUID(s string) ([16]byte, error) {
	if s == "" {
		return [16]byte{}, nil
	}
	// Manual UUID parse — avoid importing github.com/google/uuid just for this.
	// We already have it via the runner but keep the function self-contained.
	s = strings.ReplaceAll(s, "-", "")
	if len(s) != 32 {
		return [16]byte{}, fmt.Errorf("invalid UUID length: %d", len(s))
	}
	var b [16]byte
	for i := 0; i < 16; i++ {
		hi := hexNibble(s[i*2])
		lo := hexNibble(s[i*2+1])
		if hi < 0 || lo < 0 {
			return [16]byte{}, fmt.Errorf("invalid UUID hex at position %d", i*2)
		}
		b[i] = byte(hi<<4 | lo)
	}
	return b, nil
}

func hexNibble(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}
