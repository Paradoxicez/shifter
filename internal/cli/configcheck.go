package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/shifter-io/shifter/internal/chirpstack"
	"github.com/shifter-io/shifter/internal/config"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/logging"
	"github.com/spf13/cobra"
)

// configCheckCmd validates config.yaml syntax and probes Postgres, ChirpStack
// gRPC, and MQTT in that order — the canonical D-07 fail-fast diagnostic. Each
// probe writes a `PASS <name>` (success) or `FAIL <name>: <error>` (failure)
// line to stdout and any failure short-circuits the rest of the chain so the
// operator sees one clear root cause instead of a cascade.
//
// Probe order:
//  1. config syntax  — config.Load (viper parse + secret resolution + Validate)
//  2. postgres       — db.NewPool + Ping (uses cfg.DB.DSN())
//  3. chirpstack gRPC — chirpstack.Dial + chirpstack.ProbeVersion (refuses v3)
//  4. mqtt            — chirpstack.PingMQTT (connect-only probe)
//
// The same probe primitives are reused by `internal/http.TestConnHandler`
// (`chirpstack.Dial` + `ProbeVersion` + `PingMQTT`) so a successful
// `shifter config-check` and a successful Test Connection click in Settings
// exercise the same code paths — one regression surface, two operator
// affordances (CLI + UI).
var configCheckCmd = &cobra.Command{
	Use:   "config-check",
	Short: "Validate config.yaml syntax and probe Postgres + ChirpStack + MQTT (D-07)",
	RunE: func(cmd *cobra.Command, _ []string) error {
		out := cmd.OutOrStdout()

		cfg, err := config.Load()
		if err != nil {
			fmt.Fprintf(out, "FAIL config: %v\n", err)
			return err
		}
		fmt.Fprintf(out, "PASS config syntax (env=%s, tls.mode=%s)\n", cfg.Env, cfg.TLS.Mode)

		log := logging.New(cfg.LogLevel)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Postgres — uses the same pool surface the runtime uses.
		pool, err := db.NewPool(ctx, cfg.DB.DSN(), cfg.DB.MaxConns)
		if err != nil {
			fmt.Fprintf(out, "FAIL postgres: %v\n", err)
			return err
		}
		defer pool.Close()
		if err := pool.Ping(ctx); err != nil {
			fmt.Fprintf(out, "FAIL postgres ping: %v\n", err)
			return err
		}
		fmt.Fprintln(out, "PASS postgres")

		// ChirpStack gRPC — same Dial + ProbeVersion path as TestConnHandler.
		csConn, err := chirpstack.Dial(ctx, cfg.ChirpStack)
		if err != nil {
			fmt.Fprintf(out, "FAIL chirpstack dial: %v\n", err)
			return err
		}
		defer csConn.Close()
		v, err := chirpstack.ProbeVersion(ctx, csConn)
		if err != nil {
			fmt.Fprintf(out, "FAIL chirpstack probe: %v\n", err)
			return err
		}
		fmt.Fprintf(out, "PASS chirpstack (%s)\n", v)

		// MQTT — same PingMQTT path as TestConnHandler.
		if err := chirpstack.PingMQTT(ctx, cfg.MQTT.URL, cfg.MQTT.User, cfg.MQTT.Password); err != nil {
			fmt.Fprintf(out, "FAIL mqtt: %v\n", err)
			return err
		}
		fmt.Fprintln(out, "PASS mqtt")

		_ = log
		return nil
	},
}
