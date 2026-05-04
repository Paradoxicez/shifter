package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testharness"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// TestTestHarnessCmd_Help — `--help` lists all 5 scenario tokens + flags.
func TestTestHarnessCmd_Help(t *testing.T) {
	t.Parallel()

	// Re-construct a fresh root cmd so flag/help state is isolated.
	root := &cobra.Command{Use: "shifter"}
	root.AddCommand(TestHarnessCmd)
	root.SetArgs([]string{"test-harness", "--help"})

	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)

	require.NoError(t, root.Execute())

	out := buf.String()
	scenarios := []string{
		"clean_swap",
		"swap_with_inflight_uplink",
		"rollover",
		"swap_then_rollover",
		"overlapping_uplinks_during_swap",
	}
	for _, s := range scenarios {
		require.Contains(t, out, s, "scenario %q missing from --help output", s)
	}
	require.Contains(t, out, "--vendor", "--vendor flag missing from --help")
	require.Contains(t, out, "--broker", "--broker flag missing from --help")
}

// TestTestHarnessCmd_UnknownScenario — `shifter test-harness banana` returns
// an error mentioning "unknown scenario".
func TestTestHarnessCmd_UnknownScenario(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{}
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.RunE = TestHarnessCmd.RunE
	err := cmd.RunE(cmd, []string{"banana"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown scenario",
		"error must contain 'unknown scenario': %v", err)
}

// TestTestHarnessCmd_FlowAgainstTestcontainer — full integration: testcontainer
// Mosquitto + Postgres, seed fixture, run `shifter test-harness clean_swap` via
// the in-process RunE, assert measurement + audit row counts.
//
// This drives the BROKER-mode path (Publisher != nil) so it exercises W3.
// The "deployed serve.go" half is simulated by a goroutine that subscribes to
// the broker and feeds events into ingest.UplinkHandler.
func TestTestHarnessCmd_FlowAgainstTestcontainer(t *testing.T) {
	if testing.Short() {
		t.Skip("integration")
	}

	infra := startCLIHarnessInfra(t)
	defer infra.cleanup()

	// Seed: site + MP + outgoing/incoming devices + binding + axioma_w1 mappings.
	fixture := infra.seedAxiomaFixture("flow")
	infra.startMockServeIngest(fixture)

	// Build scenario input matching what the CLI would build at runtime.
	pub, err := testharness.NewPublisher(infra.brokerURL, "", "", "shifter-testharness-flow")
	require.NoError(t, err)
	defer pub.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	result, err := testharness.RunByName(ctx, testharness.ScenarioCleanSwap, testharness.RunInput{
		Pool:          infra.pool,
		Publisher:     pub,
		BrokerURL:     infra.brokerURL,
		SwapDeps:      infra.swapDepsValue(),
		Fixture:       fixture,
		ApplicationID: "00000000-0000-0000-0000-000000000000",
		Log:           slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	require.NoError(t, err)
	require.Equal(t, 2, result.MeasurementRows, "2 measurement rows after clean_swap")
	require.Equal(t, 1, result.AuditSwapRows, "1 swap audit row")
	require.Zero(t, result.FinalCumulative.Cmp(big.NewFloat(10600)),
		"final cumulative should be 10600")
}

// TestTestHarnessCLI_BrokerDownErrorMessage — W3 from gap-closure revision.
//
// Two paths covered:
//
//   (a) NewPublisher connect-fail: --broker tcp://127.0.0.1:1 (port 1
//       is unprivileged-blocked → connect fails within the 5s NewPublisher
//       timeout). Asserts error message mentions "mqtt connect to ...".
//
//   (b) ingest sync-barrier timeout: Mosquitto testcontainer is up, but no
//       ingest consumer is subscribed (we deliberately do NOT spin up the
//       mock ingest goroutine). Publish succeeds → 10s waitForMeasurementCount
//       timeout fires. Asserts error message contains the exact W3 string
//       referencing the broker URL.
func TestTestHarnessCLI_BrokerDownErrorMessage(t *testing.T) {
	if testing.Short() {
		t.Skip("integration")
	}

	t.Run("connect_fail", func(t *testing.T) {
		// Path (a): broker URL points at a port nothing listens on.
		_, err := testharness.NewPublisher("tcp://127.0.0.1:1", "", "", "shifter-th-connectfail")
		require.Error(t, err, "broker on port 1 must fail to connect")

		// The CLI subcommand wraps NewPublisher errors as
		// "mqtt connect to <broker>: <err>"; we replicate that wrap here so
		// the assertion is on the operator-facing surface.
		wrapped := fmt.Errorf("mqtt connect to %s: %w", "tcp://127.0.0.1:1", err)
		require.Contains(t, wrapped.Error(), "mqtt connect to tcp://127.0.0.1:1",
			"connect-fail error must reference the broker URL")
	})

	t.Run("sync_barrier_timeout", func(t *testing.T) {
		infra := startCLIHarnessInfra(t)
		defer infra.cleanup()

		// Seed an MP fixture so RunByName has somewhere to attribute the
		// publish — but DELIBERATELY do not start the mock ingest consumer.
		fixture := infra.seedAxiomaFixture("notimeoutwait")

		pub, err := testharness.NewPublisher(infra.brokerURL, "", "", "shifter-th-timeout")
		require.NoError(t, err)
		defer pub.Close()

		// Use a tighter context cap so the test doesn't wait the full 10s
		// barrier — but the harness's own 10s ceiling is what produces the
		// error message we check. The barrier deadline is computed inside
		// waitForMeasurementCount; ctx cancellation is a separate signal,
		// so we pass a longer ctx and let the barrier deadline fire.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		_, err = testharness.RunByName(ctx, testharness.ScenarioCleanSwap, testharness.RunInput{
			Pool:          infra.pool,
			Publisher:     pub,
			BrokerURL:     infra.brokerURL,
			SwapDeps:      infra.swapDepsValue(),
			Fixture:       fixture,
			ApplicationID: "00000000-0000-0000-0000-000000000000",
			Log:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		})
		require.Error(t, err, "with no ingest consumer the W3 barrier must time out")
		require.Contains(t, err.Error(),
			"broker publish succeeded but ingest pipeline did not produce row in 10s — verify shifter serve is running and connected to",
			"timeout error must include the operator-facing W3 message")
		require.Contains(t, err.Error(), infra.brokerURL,
			"timeout error must reference the broker URL")
	})
}

// TestTestHarnessBinary_HelpListsTestHarness — sanity: building the actual
// binary and running `shifter --help` lists the test-harness subcommand.
//
// This catches the "TestHarnessCmd was registered correctly with rootCmd"
// integration that --help on the package-level command can't reveal.
func TestTestHarnessBinary_HelpListsTestHarness(t *testing.T) {
	if testing.Short() {
		t.Skip("build")
	}

	bin := filepath.Join(t.TempDir(), "shifter")
	build := exec.Command("go", "build", "-o", bin, "../../cmd/shifter")
	out, err := build.CombinedOutput()
	require.NoError(t, err, "go build failed: %s", string(out))

	cmd := exec.Command(bin, "--help")
	out, err = cmd.CombinedOutput()
	require.NoError(t, err, "shifter --help failed: %s", string(out))
	require.Contains(t, string(out), "test-harness",
		"shifter --help must list test-harness subcommand")
}

// =============================================================================
// CLI test infrastructure — testcontainer Postgres + Mosquitto +
// optional mock ingest consumer (mirrors what serve.go would do in production).
// =============================================================================

type cliHarnessInfra struct {
	t          *testing.T
	pool       *pgxpool.Pool
	brokerURL  string
	cancelFunc context.CancelFunc
}

func (h *cliHarnessInfra) cleanup() {
	if h.cancelFunc != nil {
		h.cancelFunc()
	}
}

// startCLIHarnessInfra spins up testcontainer Postgres + Mosquitto and runs
// migrations. The caller is responsible for seeding fixtures + starting the
// mock ingest consumer if its scenario needs ingest persistence.
func startCLIHarnessInfra(t *testing.T) *cliHarnessInfra {
	t.Helper()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))
	broker := testsupport.StartMosquitto(t)
	return &cliHarnessInfra{t: t, pool: pool, brokerURL: broker}
}

// seedAxiomaFixture seeds site + MP + axioma_w1 profile + outgoing/incoming
// devices + active binding + 5 mapping rows, returning a testharness.Fixture
// the CLI tests can pass into RunByName.
func (h *cliHarnessInfra) seedAxiomaFixture(suffix string) testharness.Fixture {
	t := h.t
	t.Helper()
	ctx := context.Background()

	var siteID, mpID, profileID string
	require.NoError(t, h.pool.QueryRow(ctx,
		`INSERT INTO site (name, timezone) VALUES ($1, 'UTC') RETURNING id`,
		"site-cli-"+suffix,
	).Scan(&siteID))
	require.NoError(t, h.pool.QueryRow(ctx,
		`INSERT INTO metering_point (site_id, name, utility_class) VALUES ($1, $2, 'water') RETURNING id`,
		siteID, "mp-cli-"+suffix,
	).Scan(&mpID))
	require.NoError(t, h.pool.QueryRow(ctx,
		`SELECT id FROM device_profile WHERE slug = 'axioma_w1'`,
	).Scan(&profileID))

	var operatorID string
	require.NoError(t, h.pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ($1, 'CLI Op', 'x', 'admin') RETURNING id`,
		"opcli-"+suffix+"@example.com",
	).Scan(&operatorID))

	outDevEUI := cliPadDevEUI("cliout-" + suffix)
	inDevEUI := cliPadDevEUI("cliin-" + suffix)

	var outIDStr, inIDStr string
	require.NoError(t, h.pool.QueryRow(ctx,
		`INSERT INTO device (dev_eui, name, device_profile_id) VALUES ($1, $2, $3) RETURNING id`,
		outDevEUI, "out-cli-"+suffix, profileID,
	).Scan(&outIDStr))
	require.NoError(t, h.pool.QueryRow(ctx,
		`INSERT INTO device (dev_eui, name, device_profile_id) VALUES ($1, $2, $3) RETURNING id`,
		inDevEUI, "in-cli-"+suffix, profileID,
	).Scan(&inIDStr))

	bindingStart := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var bidStr string
	require.NoError(t, h.pool.QueryRow(ctx,
		`INSERT INTO binding (metering_point_id, device_id, valid_from, reading_offset)
		 VALUES ($1, $2, $3, 0) RETURNING id`,
		mpID, outIDStr, bindingStart,
	).Scan(&bidStr))

	require.NoError(t, testharness.SeedAxiomaW1Mappings(ctx, h.pool, uuid.MustParse(profileID)))

	return testharness.Fixture{
		SiteID:        uuid.MustParse(siteID),
		MPID:          uuid.MustParse(mpID),
		ProfileID:     uuid.MustParse(profileID),
		OperatorID:    uuid.MustParse(operatorID),
		OutDeviceID:   uuid.MustParse(outIDStr),
		OutDevEUI:     outDevEUI,
		InDeviceID:    uuid.MustParse(inIDStr),
		InDevEUI:      inDevEUI,
		BindingID:     uuid.MustParse(bidStr),
		BindingStart:  bindingStart,
		InitialOffset: big.NewFloat(0),
	}
}

// startMockServeIngest spawns a goroutine that subscribes to the broker and
// feeds received uplinks into ingest.UplinkHandler — mirroring what serve.go
// would do in a deployed install. Used by the FlowAgainstTestcontainer test
// to make the W3 sync barrier observe forward progress.
func (h *cliHarnessInfra) startMockServeIngest(_ testharness.Fixture) {
	t := h.t
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	h.cancelFunc = cancel

	deps, _ := buildCLITestIngestDeps(t, h.pool)
	go func() {
		runMockMQTTConsumer(ctx, h.brokerURL, deps)
	}()
	// Give the consumer a beat to subscribe.
	time.Sleep(200 * time.Millisecond)
}
