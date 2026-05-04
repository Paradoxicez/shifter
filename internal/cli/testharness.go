// Package cli — `shifter test-harness` D-27 dual-entry operator-runbook tool.
//
// The CLI subcommand publishes the same DATA-06 scenarios that
// internal/testharness/scenarios_test.go runs in CI, but against a deployed
// install's MQTT broker. The use case: after install, an operator runs
// `shifter test-harness clean_swap --vendor axioma_w1` and observes the
// cumulative chart go through a swap without discontinuity — proof that
// the install's full ingest pipeline is functioning end-to-end.
//
// The subcommand expects an existing site + MP + binding seeded against the
// vendor's profile (the operator created them via the UI). Unlike the Go
// integration tests, it does NOT seed fixtures — it locates them.
//
// W3 sync barrier: scenarios poll the deployed install's measurement table
// every 100ms (10s ceiling) after each PublishUplink; broker-down or
// ingest-down failures surface as a clear operator-facing error referencing
// the broker URL.

package cli

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"github.com/shifter-io/shifter/internal/config"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/logging"
	"github.com/shifter-io/shifter/internal/swap"
	"github.com/shifter-io/shifter/internal/testharness"
)

var (
	testHarnessVendor    string
	testHarnessBroker    string
	testHarnessApplication string
)

// TestHarnessCmd is the operator-runbook proof tool for D-27 dual-entry.
//
// BROKER MODE SYNC BARRIER (W3): scenarios poll q.CountMeasurementsByMP every
// 100ms with a 10s ceiling after each PublishUplink. If shifter serve is not
// running OR not subscribed to the configured broker, the scenario returns
// an error referencing the broker URL.
var TestHarnessCmd = &cobra.Command{
	Use:   "test-harness <scenario>",
	Short: "Publish synthetic DATA-06 scenarios to the configured broker",
	Long: `Available scenarios:
  clean_swap                       — basic meter swap with offset continuity
  swap_with_inflight_uplink        — uplink racing with confirm_time
  rollover                         — counter wrap (raw < prev) auto-detected
  swap_then_rollover               — rollover then swap then post-uplink
  overlapping_uplinks_during_swap  — 5 uplinks during swap window

Vendor profiles: axioma_w1 (water), acrel_adw300 (3-phase electricity).
Default vendor: axioma_w1.

Note: scenarios will write rows to your production measurement table.
Use a non-production install or be prepared to clean up.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := testharness.ScenarioName(args[0])
		// Validate scenario name early so unknown scenarios fail fast before
		// loading config / opening DB / connecting to broker.
		if !knownScenario(name) {
			return fmt.Errorf("unknown scenario %q (valid: clean_swap, swap_with_inflight_uplink, rollover, swap_then_rollover, overlapping_uplinks_during_swap)", string(name))
		}

		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("config: %w", err)
		}
		log := logging.New(cfg.LogLevel)
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()

		broker := testHarnessBroker
		if broker == "" {
			broker = cfg.MQTT.URL
		}
		if broker == "" {
			return fmt.Errorf("no MQTT broker configured (use --broker or run inside a configured install)")
		}

		pool, err := db.NewPool(ctx, cfg.DB.DSN(), cfg.DB.MaxConns)
		if err != nil {
			return fmt.Errorf("db: %w", err)
		}
		defer pool.Close()

		pub, err := testharness.NewPublisher(broker, cfg.MQTT.User, cfg.MQTT.Password, "shifter-testharness")
		if err != nil {
			return fmt.Errorf("mqtt connect to %s: %w", broker, err)
		}
		defer pub.Close()

		// Locate fixture from existing data.
		applicationID := testHarnessApplication
		fixture, err := lookupFixture(ctx, pool, testHarnessVendor)
		if err != nil {
			return err
		}
		if applicationID == "" {
			applicationID = lookupCSApplicationID(ctx, pool)
		}
		if applicationID == "" {
			applicationID = "00000000-0000-0000-0000-000000000000"
		}

		result, err := testharness.RunByName(ctx, name, testharness.RunInput{
			Pool:          pool,
			Publisher:     pub,
			BrokerURL:     broker, // W3: broker URL surfaced into timeout error message
			SwapDeps:      swap.Deps{Pool: pool, Log: log},
			Fixture:       fixture,
			ApplicationID: applicationID,
			Log:           log,
		})
		if err != nil {
			return err
		}

		finalStr := "<nil>"
		if result.FinalCumulative != nil {
			finalStr = result.FinalCumulative.Text('f', 4)
		}
		fmt.Fprintf(cmd.OutOrStdout(),
			"scenario=%s vendor=%s rows=%d swaps=%d rollovers=%d final_cumulative=%s\n",
			name, testHarnessVendor, result.MeasurementRows, result.AuditSwapRows,
			result.AuditRolloverRows, finalStr)
		return nil
	},
}

func init() {
	TestHarnessCmd.Flags().StringVar(&testHarnessVendor, "vendor", "axioma_w1", "vendor profile slug (axioma_w1 or acrel_adw300)")
	TestHarnessCmd.Flags().StringVar(&testHarnessBroker, "broker", "", "MQTT broker URL (defaults to config)")
	TestHarnessCmd.Flags().StringVar(&testHarnessApplication, "application-id", "", "ChirpStack application UUID (defaults to chirpstack_connection.cs_application_id)")
}

// knownScenario returns true for any of the 5 D-27 scenario tokens.
func knownScenario(n testharness.ScenarioName) bool {
	switch n {
	case testharness.ScenarioCleanSwap,
		testharness.ScenarioSwapWithInflightUplink,
		testharness.ScenarioRollover,
		testharness.ScenarioSwapAndRollover,
		testharness.ScenarioOverlappingUplinks:
		return true
	}
	return false
}

// lookupFixture finds the first non-archived MP whose active binding's device
// uses the vendor's profile, plus a second device on the same profile to act
// as the swap "incoming" meter. The CLI requires the operator pre-create
// these via the UI (it does not seed) — empty results yield an actionable
// "Add a device with vendor=..." error.
func lookupFixture(ctx context.Context, pool *pgxpool.Pool, vendorSlug string) (testharness.Fixture, error) {
	var f testharness.Fixture
	var profileID, mpID, siteID, bindingID, outDeviceID pgtype.UUID
	var outDevEUI string

	err := pool.QueryRow(ctx, `
		SELECT dp.id, mp.id, mp.site_id, b.id, d.id, d.dev_eui
		FROM device_profile dp
		JOIN device d   ON d.device_profile_id = dp.id
		JOIN binding b  ON b.device_id = d.id AND b.valid_to IS NULL
		JOIN metering_point mp ON mp.id = b.metering_point_id
		WHERE dp.slug = $1
		ORDER BY b.created_at ASC
		LIMIT 1`,
		vendorSlug,
	).Scan(&profileID, &mpID, &siteID, &bindingID, &outDeviceID, &outDevEUI)
	if err != nil {
		return f, fmt.Errorf("no active binding found for vendor=%s — add a device with that vendor via the UI first", vendorSlug)
	}

	// Find a second device on the same profile that's NOT currently bound
	// (will become the "incoming" meter for the swap).
	var inDeviceID pgtype.UUID
	var inDevEUI string
	err = pool.QueryRow(ctx, `
		SELECT d.id, d.dev_eui FROM device d
		WHERE d.device_profile_id = $1
		  AND NOT EXISTS (SELECT 1 FROM binding b WHERE b.device_id = d.id AND b.valid_to IS NULL)
		ORDER BY d.created_at ASC
		LIMIT 1`,
		profileID,
	).Scan(&inDeviceID, &inDevEUI)
	if err != nil {
		return f, fmt.Errorf("no unbound second device found for vendor=%s — add a second %s device via the UI so the harness has an incoming meter for swap scenarios", vendorSlug, vendorSlug)
	}

	// Find an admin user for audit_log.user_id.
	var operatorID pgtype.UUID
	if err := pool.QueryRow(ctx,
		`SELECT id FROM "user" WHERE role = 'admin' ORDER BY created_at ASC LIMIT 1`,
	).Scan(&operatorID); err != nil {
		return f, fmt.Errorf("no admin user found — create one via /install or `shifter create-admin`")
	}

	f = testharness.Fixture{
		SiteID:        uuid.UUID(siteID.Bytes),
		MPID:          uuid.UUID(mpID.Bytes),
		ProfileID:     uuid.UUID(profileID.Bytes),
		OperatorID:    uuid.UUID(operatorID.Bytes),
		OutDeviceID:   uuid.UUID(outDeviceID.Bytes),
		OutDevEUI:     outDevEUI,
		InDeviceID:    uuid.UUID(inDeviceID.Bytes),
		InDevEUI:      inDevEUI,
		BindingID:     uuid.UUID(bindingID.Bytes),
		BindingStart:  time.Now().UTC(),
		InitialOffset: big.NewFloat(0),
	}
	return f, nil
}

// lookupCSApplicationID reads chirpstack_connection.cs_application_id (D-28).
// Returns "" on any error so the caller falls back to a placeholder UUID
// (acceptable because the harness's MQTT topic structure routes by dev_eui,
// not application_id, so a placeholder still flows through the deployed
// serve.go's ingest pipeline).
func lookupCSApplicationID(ctx context.Context, pool *pgxpool.Pool) string {
	var appID *string
	if err := pool.QueryRow(ctx,
		`SELECT cs_application_id FROM chirpstack_connection WHERE id = 1`,
	).Scan(&appID); err != nil {
		return ""
	}
	if appID == nil {
		return ""
	}
	return *appID
}
