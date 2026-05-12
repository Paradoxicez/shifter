package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/shifter-io/shifter/internal/backup"
	"github.com/shifter-io/shifter/internal/config"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/logging"
)

// backupFlagTo is the --to flag value (destination directory for the tarball).
var backupFlagTo string

// backupFlagTrigger is the --trigger flag value (written to backup_run.trigger_kind).
var backupFlagTrigger string

// backupCmd is the Cobra subcommand `shifter backup` (OPS-02, OPS-03, D-43).
//
// It runs pg_dump on the Shifter DB (and ChirpStack DB in bundled mode),
// copies the floor-plan volume, and produces a single tar.gz manifest-verified
// backup. The tarball is written to --to (falls back to $SHIFTER_BACKUP_DIR or
// the compiled default /var/lib/shifter/backups).
var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Create a Shifter backup tarball",
	Long: `Runs pg_dump on the Shifter DB (and ChirpStack DB if bundled mode),
copies the floor-plan volume, and produces a single tar.gz manifest-verified
backup. The tarball is written to --to (defaults to SHIFTER_BACKUP_DIR env var
or /var/lib/shifter/backups).

The backup_run history table records every attempt; audit_log records
'backup.start' + 'backup.complete' or 'backup.failed'.

Exit codes:
  0  Backup completed successfully
  1  Backup failed (pg_dump error, disk full, etc.)
`,
	RunE: runBackupCmd,
}

func init() {
	backupCmd.Flags().StringVar(&backupFlagTo, "to", "",
		"Destination directory for the tarball (defaults to SHIFTER_BACKUP_DIR or /var/lib/shifter/backups)")
	backupCmd.Flags().StringVar(&backupFlagTrigger, "trigger", "cli",
		"Trigger kind written to backup_run.trigger_kind (cli|cron|api)")
}

func runBackupCmd(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	log := logging.New(cfg.LogLevel)

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	// Resolve destination directory: --to flag > SHIFTER_BACKUP_DIR env > config default > compile default.
	destDir := backupFlagTo
	if destDir == "" {
		destDir = cfg.BackupDir
	}
	if destDir == "" {
		destDir = backup.DefaultBackupDir
	}

	// Validate trigger kind.
	switch backupFlagTrigger {
	case "cli", "cron", "api":
		// ok
	default:
		return fmt.Errorf("--trigger must be one of cli|cron|api (got %q)", backupFlagTrigger)
	}

	pool, err := db.NewPool(ctx, cfg.DB.DSN(), cfg.DB.MaxConns)
	if err != nil {
		return fmt.Errorf("db pool: %w", err)
	}
	defer pool.Close()

	// Read install_state for chirpstack_mode + slug + id.
	var csMode, installSlug, installID string
	row := pool.QueryRow(ctx,
		`SELECT COALESCE(chirpstack_mode, 'external'), COALESCE(slug, ''), COALESCE(id::text, '')
		   FROM install_identity LIMIT 1`)
	if err := row.Scan(&csMode, &installSlug, &installID); err != nil {
		// Non-fatal: fall back to external mode.
		log.Warn("install_identity not found; defaulting to external mode", "err", err)
		csMode = "external"
	}

	// Read schema version from schema_migrations.
	var schemaVersion string
	vrow := pool.QueryRow(ctx, `SELECT version::text FROM schema_migrations`)
	if err := vrow.Scan(&schemaVersion); err != nil {
		schemaVersion = "unknown"
	}

	store := backup.NewStore(pool)
	runner := &backup.Runner{
		Pool:  pool,
		Store: store,
		Cfg: backup.RunnerConfig{
			DBHost:         cfg.DB.Host,
			DBPort:         cfg.DB.Port,
			DBUser:         cfg.DB.User,
			DBName:         cfg.DB.Database,
			DBPassword:     cfg.DB.Password,
			ChirpStackMode: csMode,
			FloorPlansDir:  cfg.FloorPlanRoot,
			InstallSlug:    installSlug,
			InstallID:      installID,
			SchemaVersion:  schemaVersion,
		},
		Log: log,
	}

	log.Info("starting backup", "dest", destDir, "trigger", backupFlagTrigger, "mode", csMode)

	backupID, tarPath, err := runner.Backup(ctx, destDir, backupFlagTrigger, nil)
	if err != nil {
		return fmt.Errorf("backup failed: %w", err)
	}

	// Print JSON result to stdout for scripting.
	fi, statErr := os.Stat(tarPath)
	result := map[string]any{
		"backup_run_id": backupID.String(),
		"file":          tarPath,
		"status":        "completed",
	}
	if statErr == nil {
		result["size_bytes"] = fi.Size()
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		return fmt.Errorf("encode result: %w", err)
	}
	_ = slog.Default() // suppress unused import warning if slog not used directly
	return nil
}
