package cli

import (
	"context"
	"fmt"
	"strconv"

	"github.com/shifter-io/shifter/internal/config"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/logging"
	"github.com/spf13/cobra"
)

// migrateCmd is the parent for `shifter migrate {up,force,version}`. D-16:
// golang-migrate is consumed as a library here, never as a separate CLI; the
// migration files are embedded in the binary via go:embed (Plan 03).
var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Database schema migrations",
}

// migrateUpCmd applies all pending up migrations. Idempotent — re-running on a
// schema already at HEAD is a no-op.
var migrateUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Apply all pending migrations",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		log := logging.New(cfg.LogLevel)
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		pool, err := db.NewPool(ctx, cfg.DB.DSN(), cfg.DB.MaxConns)
		if err != nil {
			return fmt.Errorf("db: %w", err)
		}
		defer pool.Close()
		return db.RunMigrations(ctx, pool, log)
	},
}

// migrateForceCmd marks schema_migrations clean at the given version. D-16
// recovery escape hatch: when a migration aborts mid-way the schema is
// flagged dirty; running `shifter migrate force <prev>` after fixing the
// problem unblocks the next `shifter migrate up`.
var migrateForceCmd = &cobra.Command{
	Use:   "force <version>",
	Short: "Mark the schema clean at the given version (dirty-state recovery)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		v, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("version must be an integer")
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		pool, err := db.NewPool(ctx, cfg.DB.DSN(), cfg.DB.MaxConns)
		if err != nil {
			return fmt.Errorf("db: %w", err)
		}
		defer pool.Close()
		return db.ForceVersion(ctx, pool, v)
	},
}

func init() {
	migrateCmd.AddCommand(migrateUpCmd, migrateForceCmd)
}
