package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/shifter-io/shifter/internal/backup"
	"github.com/shifter-io/shifter/internal/config"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/logging"
)

// restoreFlagFrom is the --from flag value (path to the tarball to restore).
var restoreFlagFrom string

// restoreFlagExpectedSHA256 is the optional --expected-sha256 flag value.
// When set, the tarball's outer sha256 must match this value or the restore
// is aborted before any DB operations begin.
var restoreFlagExpectedSHA256 string

// restoreCmd is the Cobra subcommand `shifter restore` (OPS-04, D-44).
//
// Safety: Shifter must be STOPPED before running restore (the command refuses
// to proceed if the PG advisory lock is held by a running serve process).
//
// Operator workflow (documented in docs/operator-runbook.md):
//
//	docker compose stop shifter
//	docker compose run --rm shifter shifter restore --from /var/lib/shifter/backups/<file>.tar.gz
//	docker compose start shifter
var restoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Restore a Shifter backup tarball (Shifter must be stopped)",
	Long: `Restores a backup tarball produced by 'shifter backup'.

Safety properties:
  - Refuses to run if the Shifter HTTP server is still up (PG advisory lock).
  - Verifies per-file sha256 sums against manifest.json.
  - Wraps pg_restore in SELECT timescaledb_pre_restore() and
    SELECT timescaledb_post_restore() (required for TimescaleDB CAGG state).
  - Never passes -j/--jobs to pg_restore (TimescaleDB Pitfall 1).
  - Restores the Shifter DB first; in bundled mode, ChirpStack DB second.
  - Rsyncs the floor-plans volume from the tarball.
  - Writes a 'backup.restore' audit row on success.

Operator workflow:
  docker compose -f compose/bundled.yml stop shifter
  docker compose -f compose/bundled.yml run --rm shifter \
    shifter restore --from /var/lib/shifter/backups/<file>.tar.gz
  docker compose -f compose/bundled.yml start shifter

Cross-version restore (e.g., v0.5.0 backup → v0.6.0 install) is not
supported in v1. The manifest's db_schema_version field is the forward-compat
hook for v1.1.

Exit codes:
  0  Restore completed successfully
  1  Restore failed (checksum mismatch, Shifter still running, pg_restore error, etc.)
`,
	RunE: runRestoreCmd,
}

func init() {
	restoreCmd.Flags().StringVar(&restoreFlagFrom, "from", "",
		"Tarball path (.tar.gz) to restore (required)")
	restoreCmd.Flags().StringVar(&restoreFlagExpectedSHA256, "expected-sha256", "",
		"Optional: expected outer tarball sha256 hex; restore is aborted on mismatch (chain-of-custody verification)")
	_ = restoreCmd.MarkFlagRequired("from")
}

func runRestoreCmd(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	log := logging.New(cfg.LogLevel)

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	pool, err := db.NewPool(ctx, cfg.DB.DSN(), cfg.DB.MaxConns)
	if err != nil {
		return fmt.Errorf("db pool: %w", err)
	}
	defer pool.Close()

	// Read ChirpStack DB name/user from config or fall back to defaults.
	csDBName := "chirpstack"
	csDBUser := "chirpstack"

	restCfg := backup.RestorerConfig{
		DBHost:           cfg.DB.Host,
		DBPort:           cfg.DB.Port,
		DBUser:           cfg.DB.User,
		DBName:           cfg.DB.Database,
		DBPassword:       cfg.DB.Password,
		ChirpStackDBName: csDBName,
		ChirpStackDBUser: csDBUser,
		FloorPlansDir:    cfg.FloorPlanRoot,
	}

	restorer := backup.NewRestorer(pool, restCfg, log)

	log.Info("starting restore",
		"from", restoreFlagFrom,
		"expected_sha256", func() string {
			if restoreFlagExpectedSHA256 != "" {
				return restoreFlagExpectedSHA256[:8] + "..."
			}
			return "(not checked)"
		}(),
	)

	if err := restorer.Restore(ctx, restoreFlagFrom, restoreFlagExpectedSHA256); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "restore failed: %v\n", err)
		return fmt.Errorf("restore: %w", err)
	}

	log.Info("restore completed successfully", "from", restoreFlagFrom)
	_, _ = fmt.Fprintln(os.Stdout, "restore completed successfully")

	_ = slog.Default() // suppress unused import warning if slog not used directly
	return nil
}
