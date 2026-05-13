package cli

import (
	"context"
	"encoding/json"
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
	doctorCmd.AddCommand(probeChirpStackCmd)
	doctorCmd.AddCommand(probeTimescaleCmd)
	doctorCmd.AddCommand(probeRegionCmd)
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

// ---- probe subcommands ----

// probeChirpStackCmd probes the ChirpStack version and reachability.
var probeChirpStackCmd = &cobra.Command{
	Use:   "probe-chirpstack",
	Short: "Probe ChirpStack version and reachability",
	Long: `Dials the configured ChirpStack gRPC endpoint, calls GetVersion,
and reports whether the version is compatible with Shifter.

Exit code: 0 on ok or warn, 1 on error.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("probe-chirpstack: load config: %w", err)
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
		defer cancel()

		result := doctor.ProbeChirpStack(ctx, cfg.ChirpStack.GRPCURL, cfg.ChirpStack.APIToken)
		printProbeResult(cmd, result)
		if result.Status == "error" {
			os.Exit(1)
		}
		return nil
	},
}

// probeTimescaleCmd probes the TimescaleDB extension presence.
var probeTimescaleCmd = &cobra.Command{
	Use:   "probe-timescale",
	Short: "Probe TimescaleDB extension presence",
	Long: `Queries pg_extension to verify the timescaledb extension is installed
in the connected PostgreSQL database.

Exit code: 0 on ok, 1 on error.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("probe-timescale: load config: %w", err)
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
		defer cancel()

		pool, err := db.NewPool(ctx, cfg.DB.DSN(), cfg.DB.MaxConns)
		if err != nil {
			return fmt.Errorf("probe-timescale: db pool: %w", err)
		}
		defer pool.Close()

		result := doctor.ProbeTimescale(ctx, pool)
		printProbeResult(cmd, result)
		if result.Status == "error" {
			os.Exit(1)
		}
		return nil
	},
}

// probeRegionCmd probes whether all gateway regions match the install region.
var probeRegionCmd = &cobra.Command{
	Use:   "probe-region",
	Short: "Probe gateway region consistency",
	Long: `Compares each gateway's region value against the install's configured
region (chirpstack_connection.region_name). A mismatch indicates gateways
may be mis-configured for the LoRaWAN plan of this deployment.

Exit code: 0 on ok or warn, 1 on error.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("probe-region: load config: %w", err)
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
		defer cancel()

		pool, err := db.NewPool(ctx, cfg.DB.DSN(), cfg.DB.MaxConns)
		if err != nil {
			return fmt.Errorf("probe-region: db pool: %w", err)
		}
		defer pool.Close()

		result := doctor.ProbeRegion(ctx, pool)
		printProbeResult(cmd, result)
		if result.Status == "error" {
			os.Exit(1)
		}
		return nil
	},
}

// printProbeResult prints a ProbeResult in human-readable or JSON format.
// JSON mode is used when --out flag or when output is not a terminal.
func printProbeResult(cmd *cobra.Command, result doctor.ProbeResult) {
	icon := map[string]string{"ok": "✓", "warn": "⚠", "error": "✗"}[result.Status]
	if icon == "" {
		icon = "?"
	}
	out := cmd.OutOrStdout()
	// Always print human-readable format.
	fmt.Fprintf(out, "[%s] %s: %s\n", result.Status, icon, result.Message)
}

// printProbeResultJSON prints a ProbeResult as JSON (for scripting/CI use).
//
//nolint:unused
func printProbeResultJSON(cmd *cobra.Command, result doctor.ProbeResult) {
	raw, _ := json.MarshalIndent(result, "", "  ")
	fmt.Fprintln(cmd.OutOrStdout(), string(raw))
}
