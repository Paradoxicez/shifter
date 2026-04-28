package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/config"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/logging"
	"github.com/spf13/cobra"
)

// createAdminCmd is the recovery escape hatch — D-14. An operator with shell
// access can always (re-)provision an admin even if the install wizard never
// completed or every admin password has been lost.
//
// Behavior:
//   - --email + --password are required.
//   - If no admin row exists for the email, INSERT a new row with role=admin,
//     must_change_password=false (D-09).
//   - If a row exists and --reset is set, UPDATE its password_hash.
//   - If a row exists and --reset is NOT set, fail with a clear "use --reset"
//     message — defends against accidentally clobbering an operator's password.
//   - If a row exists for the email but its role != "admin", fail (refuse to
//     promote a viewer to admin via --reset; that's an explicit operator
//     decision the future user-management UI handles).
var (
	createAdminEmail    string
	createAdminPassword string
	createAdminName     string
	createAdminReset    bool

	createAdminCmd = &cobra.Command{
		Use:   "create-admin",
		Short: "Create or reset an admin user (recovery escape hatch — D-14)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if createAdminEmail == "" {
				return errors.New("--email is required")
			}
			if createAdminPassword == "" {
				return errors.New("--password is required")
			}
			if len(createAdminPassword) > 256 {
				return errors.New("--password must be at most 256 bytes")
			}
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
			if err := db.RunMigrations(ctx, pool, log); err != nil {
				return fmt.Errorf("migrate: %w", err)
			}
			return createOrResetAdmin(ctx, pool, createAdminEmail, createAdminName, createAdminPassword, createAdminReset, log.Info)
		},
	}
)

func init() {
	createAdminCmd.Flags().StringVar(&createAdminEmail, "email", "", "Admin email (required)")
	createAdminCmd.Flags().StringVar(&createAdminPassword, "password", "", "New password (required)")
	createAdminCmd.Flags().StringVar(&createAdminName, "name", "Admin", "Display name (defaults to 'Admin')")
	createAdminCmd.Flags().BoolVar(&createAdminReset, "reset", false, "Reset existing admin's password (otherwise refuses to overwrite)")
}

// logFn is the slog.Logger.Info shape; passed in so tests can capture without
// a real *slog.Logger.
type logFn func(msg string, args ...any)

// createOrResetAdmin owns the create-or-reset decision tree against an open
// pgxpool. Exposed for testability — Plan 09 unit tests can call this with a
// testcontainer pool to assert the ladder without invoking cobra/viper.
func createOrResetAdmin(ctx context.Context, pool *pgxpool.Pool, email, name, password string, reset bool, info logFn) error {
	email = strings.ToLower(strings.TrimSpace(email))
	store := auth.NewStore(pool)
	hash, err := auth.Hash(password)
	if err != nil {
		return fmt.Errorf("hash: %w", err)
	}

	existing, err := store.GetUserByEmail(ctx, email)
	switch {
	case err == nil && existing != nil:
		if !reset {
			return fmt.Errorf("user %s already exists — pass --reset to update password", email)
		}
		if existing.Role != "admin" {
			return fmt.Errorf("user %s exists but is not an admin (role=%s) — refusing to promote", email, existing.Role)
		}
		if err := store.UpdatePassword(ctx, existing.ID, hash); err != nil {
			return fmt.Errorf("update: %w", err)
		}
		if info != nil {
			info("admin password reset", "email", email)
		}
		return nil

	case errors.Is(err, auth.ErrUserNotFound):
		if name == "" {
			name = "Admin"
		}
		id, ierr := store.InsertAdminUser(ctx, email, name, hash)
		if ierr != nil {
			return fmt.Errorf("insert: %w", ierr)
		}
		if info != nil {
			info("admin created", "email", email, "id", id)
		}
		return nil

	default:
		return fmt.Errorf("lookup: %w", err)
	}
}
