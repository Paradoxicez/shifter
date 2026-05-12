package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/shifter-io/shifter/internal/config"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/doctor"
	"github.com/shifter-io/shifter/internal/logging"
)

// doctorCmd prints a redacted diagnostic bundle for Shifter support.
var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Print a redacted diagnostic bundle for Shifter support",
	Long: `Outputs a JSON bundle with config-check, /health/detailed, alert worker
status, last backup, ChirpStack gRPC ping, and last 100 audit rows (emails
masked to the j***@example.com form). Operators email the bundle when filing
a support ticket.

Container logs are NOT included in v1 (Docker socket dependency deferred to
v1.x per D-50). The bundle's 'logs' field instructs operators to attach
'docker compose logs --tail=200 shifter' output manually.

Usage:
  shifter doctor              # write JSON to stdout
  shifter doctor --out /tmp/bundle.json  # write to file`,
	RunE: runDoctorCmd,
}

var doctorFlagOut string

func init() {
	doctorCmd.Flags().StringVar(&doctorFlagOut, "out", "",
		"Optional output file path (default stdout)")
}

func runDoctorCmd(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("doctor: load config: %w", err)
	}
	log := logging.New(cfg.LogLevel)

	ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
	defer cancel()

	pool, err := db.NewPool(ctx, cfg.DB.DSN(), cfg.DB.MaxConns)
	if err != nil {
		return fmt.Errorf("doctor: db pool: %w", err)
	}
	defer pool.Close()

	_ = log

	d := &doctor.Doctor{
		Pool:      pool,
		StartedAt: time.Now(), // best-effort uptime in standalone CLI invocation
	}

	bundle, err := d.SnapshotBundle(ctx)
	if err != nil {
		return fmt.Errorf("doctor: snapshot bundle: %w", err)
	}

	raw, err := bundle.MarshalRedacted()
	if err != nil {
		return fmt.Errorf("doctor: marshal: %w", err)
	}

	if doctorFlagOut != "" {
		if err := os.WriteFile(doctorFlagOut, raw, 0o600); err != nil {
			return fmt.Errorf("doctor: write file %s: %w", doctorFlagOut, err)
		}
		return nil
	}

	_, err = cmd.OutOrStdout().Write(raw)
	return err
}
