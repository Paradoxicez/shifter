package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shifter-io/shifter/internal/audit"
	"github.com/shifter-io/shifter/internal/version"
)

// RunnerConfig holds the connection parameters and install metadata the runner
// needs to orchestrate pg_dump and tarball construction.
type RunnerConfig struct {
	// Shifter DB connection.
	DBHost     string
	DBPort     int
	DBUser     string
	DBName     string
	DBPassword string

	// ChirpStack DB — used only when ChirpStackMode == "bundled".
	ChirpStackDBName string // typically "chirpstack"
	ChirpStackDBUser string // typically "chirpstack"
	ChirpStackDBPass string // typically same Postgres instance

	// Install context.
	ChirpStackMode string // "bundled" | "external"
	FloorPlansDir  string // /var/lib/shifter/floor-plans
	InstallSlug    string
	InstallID      string
	SchemaVersion  string
}

// Runner orchestrates the full backup sequence.
type Runner struct {
	Pool  *pgxpool.Pool
	Store *Store
	Cfg   RunnerConfig
	Log   *slog.Logger
}

// BackupWithNotify is like Backup but sends the backup_run.id on idCh as
// soon as the 'running' row is committed (before pg_dump starts). This lets
// HTTP callers return 202 with the id immediately without a separate
// pre-insert step that would create a second row.
//
// If idCh is nil, behavior is identical to Backup.
func (r *Runner) BackupWithNotify(ctx context.Context, destDir string, triggerKind string, userID *uuid.UUID, idCh chan<- uuid.UUID) (uuid.UUID, string, error) {
	// We need to capture the run ID after the first tx commits, before
	// pg_dump runs.  Temporarily replace the pool-backed Store with a wrapper
	// that notifies on first insert.  Simpler: override the notify inside
	// Backup by passing idCh to an inner context value — but Go contexts
	// don't carry typed values cleanly for internal use.  Instead: subclass
	// by running the start tx here, sending on idCh, then delegating rest.
	ctx, cancel := context.WithTimeout(ctx, 60*time.Minute)
	defer cancel()

	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return uuid.Nil, "", fmt.Errorf("create dest dir %q: %w", destDir, err)
	}

	csMode := r.Cfg.ChirpStackMode
	if csMode == "" {
		csMode = "external"
	}
	schemaVer := r.Cfg.SchemaVersion

	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("begin tx: %w", err)
	}
	sp := StartParams{
		TriggerKind:    triggerKind,
		TriggeredBy:    userID,
		DestinationDir: destDir,
		ChirpStackMode: &csMode,
		SchemaVersion:  &schemaVer,
	}
	runRow, err := r.Store.InsertStartedTx(ctx, tx, sp)
	if err != nil {
		_ = tx.Rollback(ctx)
		return uuid.Nil, "", fmt.Errorf("insert backup_run: %w", err)
	}
	auditUserID := uuid.Nil
	if userID != nil {
		auditUserID = *userID
	}
	if err := audit.WriteEntry(ctx, tx, audit.Entry{
		UserID:     auditUserID,
		Action:     audit.ActionBackupStart,
		EntityType: audit.EntityTypeBackupRun,
		EntityID:   runRow.ID,
		After:      map[string]any{"trigger_kind": triggerKind, "destination_dir": destDir},
	}); err != nil {
		_ = tx.Rollback(ctx)
		return uuid.Nil, "", fmt.Errorf("audit backup.start: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, "", fmt.Errorf("commit start tx: %w", err)
	}

	// Notify caller of the run ID now that the row is committed.
	if idCh != nil {
		idCh <- runRow.ID
	}

	// Proceed with the tarball build.
	tarPath, manifestData, finalErr := r.buildTarball(ctx, destDir, runRow.ID, csMode)
	if finalErr != nil {
		r.markFailed(ctx, runRow.ID, auditUserID, finalErr)
		return runRow.ID, "", finalErr
	}

	outerSHA, err := ComputeFileSHA256(tarPath)
	if err != nil {
		r.markFailed(ctx, runRow.ID, auditUserID, err)
		return runRow.ID, "", err
	}
	fi, err := os.Stat(tarPath)
	if err != nil {
		r.markFailed(ctx, runRow.ID, auditUserID, err)
		return runRow.ID, "", err
	}
	manifestJSON, err := json.Marshal(manifestData)
	if err != nil {
		r.markFailed(ctx, runRow.ID, auditUserID, err)
		return runRow.ID, "", err
	}

	finishTx, err := r.Pool.Begin(ctx)
	if err != nil {
		r.markFailed(ctx, runRow.ID, auditUserID, fmt.Errorf("begin finish tx: %w", err))
		return runRow.ID, "", err
	}
	cp := CompleteParams{
		ID:            runRow.ID,
		FileName:      filepath.Base(tarPath),
		FileSizeBytes: fi.Size(),
		SHA256:        outerSHA,
		ManifestJSON:  json.RawMessage(manifestJSON),
		FinishedAt:    manifestData.FinishedAt,
	}
	if err := r.Store.UpdateCompletedTx(ctx, finishTx, cp); err != nil {
		_ = finishTx.Rollback(ctx)
		r.markFailed(ctx, runRow.ID, auditUserID, err)
		return runRow.ID, "", err
	}
	if err := audit.WriteEntry(ctx, finishTx, audit.Entry{
		UserID:     auditUserID,
		Action:     audit.ActionBackupComplete,
		EntityType: audit.EntityTypeBackupRun,
		EntityID:   runRow.ID,
		After: map[string]any{
			"file_name":       cp.FileName,
			"file_size_bytes": cp.FileSizeBytes,
			"sha256":          outerSHA,
		},
	}); err != nil {
		_ = finishTx.Rollback(ctx)
		r.markFailed(ctx, runRow.ID, auditUserID, err)
		return runRow.ID, "", err
	}
	if err := finishTx.Commit(ctx); err != nil {
		r.markFailed(ctx, runRow.ID, auditUserID, err)
		return runRow.ID, "", err
	}

	r.Log.Info("backup completed",
		"backup_run_id", runRow.ID,
		"file", cp.FileName,
		"size_bytes", cp.FileSizeBytes,
		"sha256", outerSHA,
	)
	return runRow.ID, tarPath, nil
}

// Backup builds a backup tarball at destDir/{tarball-name}.tar.gz.
//
// It:
//  1. Inserts a backup_run row with status='running' and writes a
//     'backup.start' audit row in a committed transaction.
//  2. Runs pg_dump for the Shifter DB (and ChirpStack DB if bundled).
//  3. Adds the floor-plans directory to the tarball.
//  4. Writes manifest.json as the last entry.
//  5. Updates backup_run to status='completed' and writes 'backup.complete'.
//
// On any error, backup_run is set to status='failed' with the error message,
// and a 'backup.failed' audit row is written.
func (r *Runner) Backup(ctx context.Context, destDir string, triggerKind string, userID *uuid.UUID) (backupID uuid.UUID, tarballPath string, err error) {
	// Wrap with a 60-minute hard timeout (T-06-08-08).
	ctx, cancel := context.WithTimeout(ctx, 60*time.Minute)
	defer cancel()

	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return uuid.Nil, "", fmt.Errorf("create dest dir %q: %w", destDir, err)
	}

	csMode := r.Cfg.ChirpStackMode
	if csMode == "" {
		csMode = "external"
	}
	schemaVer := r.Cfg.SchemaVersion

	// Step 1: insert backup_run (status='running') + 'backup.start' audit in one tx.
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("begin tx: %w", err)
	}

	sp := StartParams{
		TriggerKind:    triggerKind,
		TriggeredBy:    userID,
		DestinationDir: destDir,
		ChirpStackMode: &csMode,
		SchemaVersion:  &schemaVer,
	}
	runRow, err := r.Store.InsertStartedTx(ctx, tx, sp)
	if err != nil {
		_ = tx.Rollback(ctx)
		return uuid.Nil, "", fmt.Errorf("insert backup_run: %w", err)
	}

	auditUserID := uuid.Nil
	if userID != nil {
		auditUserID = *userID
	}
	if err := audit.WriteEntry(ctx, tx, audit.Entry{
		UserID:     auditUserID,
		Action:     audit.ActionBackupStart,
		EntityType: audit.EntityTypeBackupRun,
		EntityID:   runRow.ID,
		After:      map[string]any{"trigger_kind": triggerKind, "destination_dir": destDir},
	}); err != nil {
		_ = tx.Rollback(ctx)
		return uuid.Nil, "", fmt.Errorf("audit backup.start: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, "", fmt.Errorf("commit start tx: %w", err)
	}

	backupID = runRow.ID

	// Steps 2-10: build the tarball.  Any error → mark failed.
	tarPath, manifestData, finalErr := r.buildTarball(ctx, destDir, runRow.ID, csMode)
	if finalErr != nil {
		r.markFailed(ctx, runRow.ID, auditUserID, finalErr)
		return backupID, "", finalErr
	}

	// Step 11-12: compute outer sha256, stat the file, update run row + audit.
	outerSHA, err := ComputeFileSHA256(tarPath)
	if err != nil {
		r.markFailed(ctx, runRow.ID, auditUserID, err)
		return backupID, "", err
	}
	fi, err := os.Stat(tarPath)
	if err != nil {
		r.markFailed(ctx, runRow.ID, auditUserID, err)
		return backupID, "", err
	}

	manifestJSON, err := json.Marshal(manifestData)
	if err != nil {
		r.markFailed(ctx, runRow.ID, auditUserID, err)
		return backupID, "", err
	}

	finishTx, err := r.Pool.Begin(ctx)
	if err != nil {
		r.markFailed(ctx, runRow.ID, auditUserID, fmt.Errorf("begin finish tx: %w", err))
		return backupID, "", err
	}

	cp := CompleteParams{
		ID:            runRow.ID,
		FileName:      filepath.Base(tarPath),
		FileSizeBytes: fi.Size(),
		SHA256:        outerSHA,
		ManifestJSON:  json.RawMessage(manifestJSON),
		FinishedAt:    manifestData.FinishedAt,
	}
	if err := r.Store.UpdateCompletedTx(ctx, finishTx, cp); err != nil {
		_ = finishTx.Rollback(ctx)
		r.markFailed(ctx, runRow.ID, auditUserID, err)
		return backupID, "", err
	}
	if err := audit.WriteEntry(ctx, finishTx, audit.Entry{
		UserID:     auditUserID,
		Action:     audit.ActionBackupComplete,
		EntityType: audit.EntityTypeBackupRun,
		EntityID:   runRow.ID,
		After: map[string]any{
			"file_name":       cp.FileName,
			"file_size_bytes": cp.FileSizeBytes,
			"sha256":          outerSHA,
		},
	}); err != nil {
		_ = finishTx.Rollback(ctx)
		r.markFailed(ctx, runRow.ID, auditUserID, err)
		return backupID, "", err
	}
	if err := finishTx.Commit(ctx); err != nil {
		r.markFailed(ctx, runRow.ID, auditUserID, err)
		return backupID, "", err
	}

	r.Log.Info("backup completed",
		"backup_run_id", backupID,
		"file", cp.FileName,
		"size_bytes", cp.FileSizeBytes,
		"sha256", outerSHA,
	)
	return backupID, tarPath, nil
}

// buildTarball executes pg_dump, assembles the tar.gz, and returns the tarball
// path plus a (partial) Manifest for the caller to finalise.
func (r *Runner) buildTarball(ctx context.Context, destDir string, runID uuid.UUID, csMode string) (tarPath string, mf *Manifest, err error) {
	now := time.Now().UTC()
	slug := r.Cfg.InstallSlug
	if slug == "" {
		slug = "shifter"
	}
	schemaVer := r.Cfg.SchemaVersion
	if schemaVer == "" {
		schemaVer = "unknown"
	}
	tarName := fmt.Sprintf("shifter-backup-%s-%s-%s.tar.gz",
		slug,
		now.Format("20060102-1504"),
		schemaVer,
	)
	tarPath = filepath.Join(destDir, tarName)

	// Temp directory for pg_dump output files.
	tmpDir, err := os.MkdirTemp("", "shifter-backup-*")
	if err != nil {
		return "", nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// --- pg_dump Shifter DB ---
	shifterDump := filepath.Join(tmpDir, "shifter.dump")
	if err := r.runPgDump(ctx, r.Cfg.DBUser, r.Cfg.DBPassword, r.Cfg.DBName, r.Cfg.DBHost, r.Cfg.DBPort, shifterDump); err != nil {
		return "", nil, fmt.Errorf("pg_dump shifter: %w", err)
	}

	// --- pg_dump ChirpStack DB (bundled mode only) ---
	var chirpstackDump string
	if csMode == "bundled" {
		chirpstackDump = filepath.Join(tmpDir, "chirpstack.dump")
		csUser := r.Cfg.ChirpStackDBUser
		if csUser == "" {
			csUser = "chirpstack"
		}
		csDB := r.Cfg.ChirpStackDBName
		if csDB == "" {
			csDB = "chirpstack"
		}
		csPass := r.Cfg.ChirpStackDBPass
		if err := r.runPgDump(ctx, csUser, csPass, csDB, r.Cfg.DBHost, r.Cfg.DBPort, chirpstackDump); err != nil {
			return "", nil, fmt.Errorf("pg_dump chirpstack: %w", err)
		}
	}

	// --- Build tar.gz ---
	tarFile, err := os.Create(tarPath)
	if err != nil {
		return "", nil, fmt.Errorf("create tar %q: %w", tarPath, err)
	}
	defer tarFile.Close()

	gzWriter := gzip.NewWriter(tarFile)
	defer gzWriter.Close()
	tw := tar.NewWriter(gzWriter)
	defer tw.Close()

	mf = &Manifest{
		ManifestVersion: "1.0",
		ShifterVersion:  version.Version,
		DBSchemaVersion: schemaVer,
		ChirpStackMode:  csMode,
		InstallID:       r.Cfg.InstallID,
		InstallSlug:     slug,
		StartedAt:       now,
		SHA256Sums:      make(map[string]string),
	}

	// Add Shifter dump.
	sha, err := addFileToTar(tw, shifterDump, "db/shifter.dump")
	if err != nil {
		return "", nil, fmt.Errorf("add shifter.dump to tar: %w", err)
	}
	mf.Included = append(mf.Included, "db/shifter.dump")
	mf.SHA256Sums["db/shifter.dump"] = sha

	// Add ChirpStack dump (bundled).
	if csMode == "bundled" && chirpstackDump != "" {
		sha, err := addFileToTar(tw, chirpstackDump, "db/chirpstack.dump")
		if err != nil {
			return "", nil, fmt.Errorf("add chirpstack.dump to tar: %w", err)
		}
		mf.Included = append(mf.Included, "db/chirpstack.dump")
		mf.SHA256Sums["db/chirpstack.dump"] = sha
		mf.ChirpStackDBIncluded = true
	}

	// Add floor-plans directory.
	fpDir := r.Cfg.FloorPlansDir
	if fpDir == "" {
		fpDir = "/var/lib/shifter/floor-plans"
	}
	if err := addDirToTar(tw, fpDir, "floor-plans/", mf.SHA256Sums); err != nil {
		// Non-fatal if the directory doesn't exist yet (new installs).
		r.Log.Warn("floor-plans dir not added to backup", "err", err, "dir", fpDir)
	} else {
		mf.Included = append(mf.Included, "floor-plans/")
	}

	// Write manifest as the LAST entry.
	mf.FinishedAt = time.Now().UTC()
	manifestBytes, err := json.MarshalIndent(mf, "", "  ")
	if err != nil {
		return "", nil, fmt.Errorf("marshal manifest: %w", err)
	}
	hdr := &tar.Header{
		Name:    "manifest.json",
		Size:    int64(len(manifestBytes)),
		Mode:    0o644,
		ModTime: mf.FinishedAt,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return "", nil, fmt.Errorf("write manifest header: %w", err)
	}
	if _, err := tw.Write(manifestBytes); err != nil {
		return "", nil, fmt.Errorf("write manifest body: %w", err)
	}

	// Flush writers before computing the outer sha256.
	if err := tw.Close(); err != nil {
		return "", nil, fmt.Errorf("close tar writer: %w", err)
	}
	if err := gzWriter.Close(); err != nil {
		return "", nil, fmt.Errorf("close gzip writer: %w", err)
	}
	if err := tarFile.Close(); err != nil {
		return "", nil, fmt.Errorf("close tar file: %w", err)
	}

	return tarPath, mf, nil
}

// runPgDump executes pg_dump with the given parameters.
//
// PITFALL 1: NEVER add --jobs / -j to this command.  TimescaleDB does not
// support parallel restore and it breaks catalog ordering.  The args slice is
// hardcoded so no caller can accidentally inject parallelism.
func (r *Runner) runPgDump(ctx context.Context, user, password, dbname, host string, port int, outFile string) error {
	dbPort := port
	if dbPort == 0 {
		dbPort = 5432
	}
	// Hardcoded args — no --jobs / -j anywhere (Pitfall 1).
	args := []string{
		"--host", host,
		"--port", fmt.Sprintf("%d", dbPort),
		"--username", user,
		"--dbname", dbname,
		"--format=custom",
		"--no-owner",
		"--no-acl",
		"--file", outFile,
	}
	cmd := exec.CommandContext(ctx, "pg_dump", args...)
	// Pass PGPASSWORD via env (never as a command-line flag so it doesn't
	// appear in process listings — T-06-08-03 partial mitigation).
	cmd.Env = append(os.Environ(), "PGPASSWORD="+password)

	out, err := cmd.CombinedOutput()
	if len(out) > 0 {
		r.Log.Info("pg_dump output", "dbname", dbname, "output", string(out))
	}
	if err != nil {
		return fmt.Errorf("pg_dump %q exit: %w; output: %s", dbname, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// addFileToTar adds the file at srcPath into the tarball under archiveName.
// Returns the hex SHA-256 of the file.
func addFileToTar(tw *tar.Writer, srcPath, archiveName string) (string, error) {
	f, err := os.Open(srcPath)
	if err != nil {
		return "", fmt.Errorf("open %q: %w", srcPath, err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("stat %q: %w", srcPath, err)
	}

	hdr := &tar.Header{
		Name:    archiveName,
		Size:    fi.Size(),
		Mode:    int64(fi.Mode()),
		ModTime: fi.ModTime(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return "", fmt.Errorf("write header %q: %w", archiveName, err)
	}

	// Tee through sha256 while copying.
	sha, err := computeReaderSHA256(io.TeeReader(f, tw))
	if err != nil {
		return "", fmt.Errorf("copy+sha256 %q: %w", archiveName, err)
	}
	return sha, nil
}

// addDirToTar recursively adds all files under srcDir into the tarball under
// archivePrefix.  Updates sha256Sums in-place with archive-path → sha256.
func addDirToTar(tw *tar.Writer, srcDir, archivePrefix string, sha256Sums map[string]string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		archiveName := archivePrefix + filepath.ToSlash(rel)
		if info.IsDir() {
			if rel == "." {
				return nil // skip root dir entry
			}
			hdr := &tar.Header{
				Name:    archiveName + "/",
				Typeflag: tar.TypeDir,
				Mode:    0o755,
				ModTime: info.ModTime(),
			}
			return tw.WriteHeader(hdr)
		}
		sha, err := addFileToTar(tw, path, archiveName)
		if err != nil {
			return err
		}
		sha256Sums[archiveName] = sha
		return nil
	})
}

// markFailed updates the backup_run row to failed + writes audit in a new tx.
func (r *Runner) markFailed(ctx context.Context, runID uuid.UUID, userID uuid.UUID, cause error) {
	failCtx := context.Background() // use background so a cancelled ctx doesn't skip this
	tx, txErr := r.Pool.Begin(failCtx)
	if txErr != nil {
		r.Log.Error("backup: begin fail tx", "err", txErr, "run_id", runID)
		return
	}
	if err := r.Store.UpdateFailedTx(failCtx, tx, runID, cause.Error()); err != nil {
		r.Log.Error("backup: update failed", "err", err, "run_id", runID)
		_ = tx.Rollback(failCtx)
		return
	}
	if err := audit.WriteEntry(failCtx, tx, audit.Entry{
		UserID:     userID,
		Action:     audit.ActionBackupFailed,
		EntityType: audit.EntityTypeBackupRun,
		EntityID:   runID,
		After:      map[string]any{"error": cause.Error()},
	}); err != nil {
		r.Log.Error("backup: audit failed", "err", err, "run_id", runID)
		_ = tx.Rollback(failCtx)
		return
	}
	if err := tx.Commit(failCtx); err != nil {
		r.Log.Error("backup: commit fail tx", "err", err, "run_id", runID)
	}
}
