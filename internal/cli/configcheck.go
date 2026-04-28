package cli

import (
	"fmt"

	"github.com/shifter-io/shifter/internal/config"
	"github.com/spf13/cobra"
)

// configCheckCmd validates config.yaml syntax and (eventually) probes external
// dependencies. D-07. Plan 17 fills in the gRPC ProbeVersion + MQTT PingMQTT
// connectivity probes; today this verifies that config.Load() succeeds and
// emits PASS / FAIL lines per check so install kits can pipe the output into a
// log without grep gymnastics.
var configCheckCmd = &cobra.Command{
	Use:   "config-check",
	Short: "Validate config.yaml syntax and probe endpoints (D-07)",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "FAIL config: %v\n", err)
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "PASS config syntax (env=%s, tls.mode=%s)\n", cfg.Env, cfg.TLS.Mode)

		// TODO(plan-17): connectivity probes
		//   - PASS/FAIL postgres ping (db.NewPool().Ping)
		//   - PASS/FAIL chirpstack gRPC ProbeVersion (Plan 12)
		//   - PASS/FAIL MQTT PingMQTT (Plan 13)
		// Until Plan 17 lands, config-check only verifies syntax + secret
		// resolution.
		fmt.Fprintln(cmd.OutOrStdout(), "SKIP connectivity probes (pending Plan 17)")
		return nil
	},
}
