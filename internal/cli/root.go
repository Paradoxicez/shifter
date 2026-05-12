package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// rootCmd is the canonical Cobra root for the `shifter` binary. Subcommands
// (serve, migrate, version, create-admin, config-check, healthcheck,
// test-harness) are registered in Execute(). D-12 + D-27.
var rootCmd = &cobra.Command{
	Use:   "shifter",
	Short: "Shifter — self-hosted LoRaWAN water/electricity monitoring",
	Long: `shifter is the self-hosted LoRaWAN water/electricity monitoring backend.

It wraps ChirpStack as the LoRaWAN Network Server and serves the Shifter
dashboard SPA. Subcommands:

  serve         Run migrations then start the HTTP server (D-13)
  migrate       Apply, force, or inspect database migrations (D-16)
  version       Print binary version
  create-admin  Create or reset an admin user (recovery — D-14)
  config-check  Validate config syntax and probe endpoints (D-07)
  healthcheck   Localhost HTTP GET /health (D-15, Docker HEALTHCHECK)
  test-harness  Publish synthetic DATA-06 scenarios to the broker (D-27)`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute is the package entry point invoked by cmd/shifter/main.go. It
// registers every subcommand and runs Cobra's dispatch. Errors are printed to
// stderr and surfaced to the caller so main() can exit non-zero.
//
// The 9 canonical subcommands per D-12 + D-27 + OPS-02 + OPS-04:
// serve, migrate, version, create-admin, config-check, healthcheck,
// test-harness, backup, restore.
func Execute() error {
	rootCmd.AddCommand(
		serveCmd,
		migrateCmd,
		versionCmd,
		createAdminCmd,
		configCheckCmd,
		healthcheckCmd,
		TestHarnessCmd,
		backupCmd,
		restoreCmd,
	)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return err
	}
	return nil
}
