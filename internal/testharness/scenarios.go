package testharness

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
	"github.com/shifter-io/shifter/internal/ingest"
	"github.com/shifter-io/shifter/internal/swap"
)

// scenarioPrecision matches ingest/swap numericPrecision (128 bits) so
// big.Float comparisons in scenarios don't silently lose digits.
const scenarioPrecision = 128

// Vendor identifies which seeded device_profile a scenario runs against.
type Vendor string

const (
	VendorAxiomaW1    Vendor = "axioma_w1"
	VendorAcrelADW300 Vendor = "acrel_adw300"
)

// ScenarioName is the canonical D-27 token. The 5 named scenarios map to
// VALIDATION.md DATA-06 row.
type ScenarioName string

const (
	ScenarioCleanSwap              ScenarioName = "clean_swap"
	ScenarioSwapWithInflightUplink ScenarioName = "swap_with_inflight_uplink"
	ScenarioRollover               ScenarioName = "rollover"
	ScenarioSwapAndRollover        ScenarioName = "swap_then_rollover"
	ScenarioOverlappingUplinks     ScenarioName = "overlapping_uplinks_during_swap"
)

// Fixture is the seeded state a scenario starts from. The harness seeds
// site → MP → profile → outgoing-device + incoming-device → active binding
// (on the outgoing device) + per-profile mapping rows whose json_pointer
// values match the codec output (B2 fix; pinned per 02-13-PLAN.md).
type Fixture struct {
	SiteID        uuid.UUID
	MPID          uuid.UUID
	ProfileID     uuid.UUID
	OperatorID    uuid.UUID
	OutDeviceID   uuid.UUID
	OutDevEUI     string
	InDeviceID    uuid.UUID
	InDevEUI      string
	BindingID     uuid.UUID
	BindingStart  time.Time
	InitialOffset *big.Float // current binding's reading_offset
	InitialRaw    *big.Float // current binding's last_raw_value (nil = first uplink)
}

// RunInput packages everything a scenario needs to drive itself.
//
// W3: Publisher != nil → MQTT path with 10s ingest sync barrier.
// Publisher == nil   → in-process synchronous handler invocation.
type RunInput struct {
	Pool          *pgxpool.Pool
	Publisher     *Publisher  // when non-nil, scenarios dispatch via MQTT and apply the W3 sync barrier
	BrokerURL     string      // operator-facing diagnostic for W3 timeout error message
	IngestDeps    ingest.Deps // exposes Pool + Resolver + Mappings — used for direct UplinkHandler invocation when Publisher==nil
	SwapDeps      swap.Deps   // for in-scenario CommitSwap calls
	Fixture       Fixture
	ApplicationID string // CS app id — synthesized for tests
	Log           *slog.Logger
}

// Result is what a scenario reports after running. Cumulative continuity is
// asserted via FinalCumulative; audit row counts gate the swap/rollover
// invariants.
type Result struct {
	MeasurementRows   int
	AuditSwapRows     int
	AuditRolloverRows int
	FinalCumulative   *big.Float
}

// CleanSwap implements scenario 1 (clean_swap):
//
//  1. Publish initial uplink raw=10500 against the seeded outgoing binding.
//  2. CommitSwap with R=10500, N=0, override=nil → new binding offset=10500.
//  3. Publish post-swap uplink raw=100 against the NEW binding (incoming device).
//
// Asserts: 2 measurement rows, both cumulative-continuous; 1 audit row
// action='swap'; final cumulative = 100 + 10500 = 10600.
func CleanSwap(ctx context.Context, in RunInput) (*Result, error) {
	if err := validateRunInput(in); err != nil {
		return nil, err
	}
	f := in.Fixture
	t0 := f.BindingStart.Add(time.Hour)

	// Step 1: outgoing uplink raw=10500.
	preEvent, err := BuildAxiomaW1UplinkForDevEUI(f.OutDevEUI, in.ApplicationID, 10500, 90, 22, false, false, 1, t0)
	if err != nil {
		return nil, fmt.Errorf("build pre-swap uplink: %w", err)
	}
	if err := publishOrInline(ctx, in, f.MPID, in.ApplicationID, f.OutDevEUI, preEvent, 1); err != nil {
		return nil, fmt.Errorf("pre-swap publish: %w", err)
	}

	// Step 2: commit the swap.
	confirmTime := t0.Add(time.Minute)
	newBindingID, err := swap.CommitSwap(ctx, in.SwapDeps, swap.SwapInput{
		UserID:            f.OperatorID,
		RequestID:         "harness-clean_swap",
		MeteringPointID:   f.MPID,
		OutgoingBindingID: f.BindingID,
		OutgoingDevEUI:    f.OutDevEUI,
		IncomingDeviceID:  f.InDeviceID,
		IncomingDevEUI:    f.InDevEUI,
		ConfirmTime:       confirmTime,
		OutgoingReadingR:  big.NewFloat(10500),
		IncomingInitialN:  big.NewFloat(0),
		OperatorNotes:     "harness clean_swap",
	})
	if err != nil {
		return nil, fmt.Errorf("commit swap: %w", err)
	}

	// Step 3: post-swap uplink raw=100 (new meter started fresh at 0).
	postTime := confirmTime.Add(time.Minute)
	postEvent, err := BuildAxiomaW1UplinkForDevEUI(f.InDevEUI, in.ApplicationID, 100, 100, 22, false, false, 1, postTime)
	if err != nil {
		return nil, fmt.Errorf("build post-swap uplink: %w", err)
	}
	if err := publishOrInline(ctx, in, f.MPID, in.ApplicationID, f.InDevEUI, postEvent, 2); err != nil {
		return nil, fmt.Errorf("post-swap publish: %w", err)
	}

	_ = newBindingID
	return collectResult(ctx, in.Pool, f.MPID)
}

// SwapWithInflightUplink — scenario 2. A swap commit and an uplink arrive
// concurrently; per D-14 + D-15 the uplink's gateway_rx_time decides which
// binding it attributes to. We approximate "concurrent" by sequencing:
//
//  1. Publish uplink against OUTGOING binding at t1 (well before confirm).
//  2. CommitSwap at t2 = t1 + 5s.
//  3. Publish uplink against INCOMING binding at t3 = t2 + 1s.
//
// Asserts: 2 measurement rows; 1 audit row action='swap'; no row dropped.
func SwapWithInflightUplink(ctx context.Context, in RunInput) (*Result, error) {
	if err := validateRunInput(in); err != nil {
		return nil, err
	}
	f := in.Fixture
	t0 := f.BindingStart.Add(time.Hour)

	preEvent, err := BuildAxiomaW1UplinkForDevEUI(f.OutDevEUI, in.ApplicationID, 5000, 90, 21, false, false, 1, t0)
	if err != nil {
		return nil, fmt.Errorf("build pre-swap uplink: %w", err)
	}
	if err := publishOrInline(ctx, in, f.MPID, in.ApplicationID, f.OutDevEUI, preEvent, 1); err != nil {
		return nil, fmt.Errorf("pre-swap publish: %w", err)
	}

	confirmTime := t0.Add(5 * time.Second)
	if _, err := swap.CommitSwap(ctx, in.SwapDeps, swap.SwapInput{
		UserID:            f.OperatorID,
		RequestID:         "harness-swap_inflight",
		MeteringPointID:   f.MPID,
		OutgoingBindingID: f.BindingID,
		OutgoingDevEUI:    f.OutDevEUI,
		IncomingDeviceID:  f.InDeviceID,
		IncomingDevEUI:    f.InDevEUI,
		ConfirmTime:       confirmTime,
		OutgoingReadingR:  big.NewFloat(5000),
		IncomingInitialN:  big.NewFloat(0),
	}); err != nil {
		return nil, fmt.Errorf("commit swap: %w", err)
	}

	postTime := confirmTime.Add(1 * time.Second)
	postEvent, err := BuildAxiomaW1UplinkForDevEUI(f.InDevEUI, in.ApplicationID, 50, 100, 21, false, false, 1, postTime)
	if err != nil {
		return nil, fmt.Errorf("build post-swap uplink: %w", err)
	}
	if err := publishOrInline(ctx, in, f.MPID, in.ApplicationID, f.InDevEUI, postEvent, 2); err != nil {
		return nil, fmt.Errorf("post-swap publish: %w", err)
	}

	return collectResult(ctx, in.Pool, f.MPID)
}

// Rollover — scenario 3. The binding's last_raw_value starts near 2^32; the
// next uplink wraps below it. DetectRollover fires; offset advances by
// counter_modulus; cumulative stays monotonic; one audit row 'rollover_detected'.
//
// Pre-condition: in.Fixture.InitialRaw is set near the modulus (e.g. 2^32 - 100).
func Rollover(ctx context.Context, in RunInput) (*Result, error) {
	if err := validateRunInput(in); err != nil {
		return nil, err
	}
	f := in.Fixture
	t0 := f.BindingStart.Add(time.Hour)

	// Publish a single post-rollover uplink with raw=100. This forces
	// DetectRollover (curr=100 < prev=4294967200) → offset += 4294967296.
	event, err := BuildAxiomaW1UplinkForDevEUI(f.OutDevEUI, in.ApplicationID, 100, 90, 20, false, false, 5, t0)
	if err != nil {
		return nil, fmt.Errorf("build rollover uplink: %w", err)
	}
	if err := publishOrInline(ctx, in, f.MPID, in.ApplicationID, f.OutDevEUI, event, 1); err != nil {
		return nil, fmt.Errorf("rollover publish: %w", err)
	}

	return collectResult(ctx, in.Pool, f.MPID)
}

// SwapAndRollover — scenario 4: rollover fires, THEN a swap is committed,
// THEN a post-swap uplink lands. Asserts (a) 1 rollover audit row, (b) 1
// swap audit row, (c) all 3 measurement rows cumulative-continuous.
func SwapAndRollover(ctx context.Context, in RunInput) (*Result, error) {
	if err := validateRunInput(in); err != nil {
		return nil, err
	}
	f := in.Fixture
	t0 := f.BindingStart.Add(time.Hour)

	// Step 1: rollover uplink (raw=200; binding pre-seeded near modulus).
	roll, err := BuildAxiomaW1UplinkForDevEUI(f.OutDevEUI, in.ApplicationID, 200, 90, 21, false, false, 9, t0)
	if err != nil {
		return nil, fmt.Errorf("build rollover uplink: %w", err)
	}
	if err := publishOrInline(ctx, in, f.MPID, in.ApplicationID, f.OutDevEUI, roll, 1); err != nil {
		return nil, fmt.Errorf("rollover publish: %w", err)
	}

	// After step 1: outgoing binding has reading_offset = old + 2^32 = 4294967296,
	// last_raw_value = 200, cumulative_value of the row = 200 + 4294967296 = 4294967496.
	// Step 2: swap. Operator captures R = 4294967496 (display reading at swap).
	confirmTime := t0.Add(time.Minute)
	bigR := new(big.Float).SetPrec(scenarioPrecision)
	bigR.SetInt64(4294967496)
	if _, err := swap.CommitSwap(ctx, in.SwapDeps, swap.SwapInput{
		UserID:            f.OperatorID,
		RequestID:         "harness-swap_and_rollover",
		MeteringPointID:   f.MPID,
		OutgoingBindingID: f.BindingID,
		OutgoingDevEUI:    f.OutDevEUI,
		IncomingDeviceID:  f.InDeviceID,
		IncomingDevEUI:    f.InDevEUI,
		ConfirmTime:       confirmTime,
		OutgoingReadingR:  bigR,
		IncomingInitialN:  big.NewFloat(0),
	}); err != nil {
		return nil, fmt.Errorf("commit swap: %w", err)
	}

	// Step 3: post-swap uplink raw=10 (new meter starting fresh).
	postTime := confirmTime.Add(time.Minute)
	post, err := BuildAxiomaW1UplinkForDevEUI(f.InDevEUI, in.ApplicationID, 10, 100, 21, false, false, 1, postTime)
	if err != nil {
		return nil, fmt.Errorf("build post-swap uplink: %w", err)
	}
	if err := publishOrInline(ctx, in, f.MPID, in.ApplicationID, f.InDevEUI, post, 2); err != nil {
		return nil, fmt.Errorf("post-swap publish: %w", err)
	}

	return collectResult(ctx, in.Pool, f.MPID)
}

// OverlappingUplinks — scenario 5: 5 uplinks rapidly during the swap window.
// We sequence: 2 against outgoing, swap, 3 against incoming. Asserts all 5
// measurement rows landed; per-row binding_id matches the active binding at
// row time; exactly 1 swap audit row.
func OverlappingUplinks(ctx context.Context, in RunInput) (*Result, error) {
	if err := validateRunInput(in); err != nil {
		return nil, err
	}
	f := in.Fixture
	t0 := f.BindingStart.Add(time.Hour)

	for i, raw := range []uint32{1000, 1100} {
		ev, err := BuildAxiomaW1UplinkForDevEUI(f.OutDevEUI, in.ApplicationID, raw, 90, 21, false, false, uint32(i+1), t0.Add(time.Duration(i)*time.Second))
		if err != nil {
			return nil, fmt.Errorf("build outgoing uplink %d: %w", i, err)
		}
		if err := publishOrInline(ctx, in, f.MPID, in.ApplicationID, f.OutDevEUI, ev, i+1); err != nil {
			return nil, fmt.Errorf("outgoing publish %d: %w", i, err)
		}
	}

	confirmTime := t0.Add(10 * time.Second)
	if _, err := swap.CommitSwap(ctx, in.SwapDeps, swap.SwapInput{
		UserID:            f.OperatorID,
		RequestID:         "harness-overlapping",
		MeteringPointID:   f.MPID,
		OutgoingBindingID: f.BindingID,
		OutgoingDevEUI:    f.OutDevEUI,
		IncomingDeviceID:  f.InDeviceID,
		IncomingDevEUI:    f.InDevEUI,
		ConfirmTime:       confirmTime,
		OutgoingReadingR:  big.NewFloat(1100),
		IncomingInitialN:  big.NewFloat(0),
	}); err != nil {
		return nil, fmt.Errorf("commit swap: %w", err)
	}

	for i, raw := range []uint32{20, 40, 60} {
		ev, err := BuildAxiomaW1UplinkForDevEUI(f.InDevEUI, in.ApplicationID, raw, 100, 21, false, false, uint32(i+1), confirmTime.Add(time.Duration(i+1)*time.Second))
		if err != nil {
			return nil, fmt.Errorf("build incoming uplink %d: %w", i, err)
		}
		if err := publishOrInline(ctx, in, f.MPID, in.ApplicationID, f.InDevEUI, ev, i+1+2); err != nil {
			return nil, fmt.Errorf("incoming publish %d: %w", i, err)
		}
	}

	return collectResult(ctx, in.Pool, f.MPID)
}

// RunByName dispatches a scenario by D-27 name. Used by both the test
// suite and the CLI subcommand.
func RunByName(ctx context.Context, name ScenarioName, in RunInput) (*Result, error) {
	switch name {
	case ScenarioCleanSwap:
		return CleanSwap(ctx, in)
	case ScenarioSwapWithInflightUplink:
		return SwapWithInflightUplink(ctx, in)
	case ScenarioRollover:
		return Rollover(ctx, in)
	case ScenarioSwapAndRollover:
		return SwapAndRollover(ctx, in)
	case ScenarioOverlappingUplinks:
		return OverlappingUplinks(ctx, in)
	default:
		return nil, fmt.Errorf("unknown scenario %q", name)
	}
}

// publishOrInline dispatches a single uplink. When in.Publisher != nil
// (CLI mode against a deployed broker) it publishes via MQTT and applies
// the W3 ingest sync barrier. When Publisher == nil (Go integration tests
// running ingest in-process) it invokes ingest.UplinkHandler directly,
// which is synchronous and persists before returning.
//
// expectedTotalAfter is the cumulative measurement-row count the caller
// expects after this publish lands (used by the W3 polling loop). For
// in-process mode it's ignored — the handler is synchronous.
func publishOrInline(ctx context.Context, in RunInput, mpID uuid.UUID, applicationID, devEUI string, event []byte, expectedTotalAfter int) error {
	if in.Publisher != nil {
		if err := in.Publisher.PublishUplink(ctx, applicationID, devEUI, event); err != nil {
			return fmt.Errorf("publish: %w", err)
		}
		// W3 sync barrier — poll until the deployed serve.go's ingest
		// pipeline catches up. Without this, the next swap.CommitSwap
		// races against the not-yet-persisted measurement row.
		return waitForMeasurementCount(ctx, in.Pool, mpID, expectedTotalAfter, 10*time.Second, in.BrokerURL)
	}
	// In-process: handler is synchronous on the caller's goroutine.
	topic := fmt.Sprintf("application/%s/device/%s/event/up", applicationID, devEUI)
	ingest.UplinkHandler(in.IngestDeps)(topic, event)
	return nil
}

// waitForMeasurementCount polls q.CountMeasurementsByMP every 100ms until
// the count reaches target or the timeout elapses. On timeout, returns
// an operator-facing error referencing the broker URL — the most likely
// cause of a brokered scenario hanging is "shifter serve isn't connected
// to the broker." (W3 from gap-closure revision.)
func waitForMeasurementCount(ctx context.Context, pool *pgxpool.Pool, mpID uuid.UUID, target int, timeout time.Duration, brokerURL string) error {
	deadline := time.Now().Add(timeout)
	q := sqlc.New(pool)
	for {
		count, err := q.CountMeasurementsByMP(ctx, pgtype.UUID{Bytes: mpID, Valid: true})
		if err != nil {
			return fmt.Errorf("count measurements: %w", err)
		}
		if int(count) >= target {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("broker publish succeeded but ingest pipeline did not produce row in 10s — verify shifter serve is running and connected to %s", brokerURL)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// collectResult reads measurement count + the audit row counts (swap +
// rollover_detected) for the scenario's MP, and the final cumulative_value
// from the most-recent measurement row.
func collectResult(ctx context.Context, pool *pgxpool.Pool, mpID uuid.UUID) (*Result, error) {
	q := sqlc.New(pool)
	count, err := q.CountMeasurementsByMP(ctx, pgtype.UUID{Bytes: mpID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("count measurements: %w", err)
	}

	// Audit rows for swap / rollover scoped to this MP. Joined via binding.
	var swapCount, rollCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log a
		 JOIN binding b ON b.id = a.entity_id
		 WHERE a.action = 'swap' AND b.metering_point_id = $1`,
		pgtype.UUID{Bytes: mpID, Valid: true},
	).Scan(&swapCount); err != nil {
		return nil, fmt.Errorf("count swap audit: %w", err)
	}
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log a
		 JOIN binding b ON b.id = a.entity_id
		 WHERE a.action = 'rollover_detected' AND b.metering_point_id = $1`,
		pgtype.UUID{Bytes: mpID, Valid: true},
	).Scan(&rollCount); err != nil {
		return nil, fmt.Errorf("count rollover audit: %w", err)
	}

	finalCum := big.NewFloat(0).SetPrec(scenarioPrecision)
	if int(count) > 0 {
		latest, err := q.GetLatestMeasurement(ctx, pgtype.UUID{Bytes: mpID, Valid: true})
		if err != nil {
			return nil, fmt.Errorf("get latest measurement: %w", err)
		}
		f, perr := bigFloatFromNumeric(latest.CumulativeValue)
		if perr != nil {
			return nil, fmt.Errorf("decode cumulative: %w", perr)
		}
		finalCum = f
	}

	return &Result{
		MeasurementRows:   int(count),
		AuditSwapRows:     swapCount,
		AuditRolloverRows: rollCount,
		FinalCumulative:   finalCum,
	}, nil
}

func validateRunInput(in RunInput) error {
	if in.Pool == nil {
		return fmt.Errorf("RunInput.Pool is required")
	}
	if in.ApplicationID == "" {
		return fmt.Errorf("RunInput.ApplicationID is required")
	}
	if in.Fixture.MPID == uuid.Nil {
		return fmt.Errorf("RunInput.Fixture.MPID is required")
	}
	if in.Publisher == nil && in.IngestDeps.Pool == nil {
		return fmt.Errorf("either RunInput.Publisher (broker mode) or RunInput.IngestDeps (in-process mode) must be set")
	}
	return nil
}

// bigFloatFromNumeric decodes a pgtype.Numeric to *big.Float at scenarioPrecision.
// Encoded via the JSON form (pgtype.Numeric MarshalJSON renders decimal text
// losslessly).
func bigFloatFromNumeric(n pgtype.Numeric) (*big.Float, error) {
	if !n.Valid {
		return big.NewFloat(0).SetPrec(scenarioPrecision), nil
	}
	b, err := n.MarshalJSON()
	if err != nil {
		return nil, err
	}
	s := string(b)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	f, _, err := big.ParseFloat(s, 10, scenarioPrecision, big.ToNearestEven)
	if err != nil {
		return nil, err
	}
	return f, nil
}

// SeedAxiomaW1Mappings inserts the 5 mapping rows whose json_pointer values
// match axioma_w1.js result.* (B2 fix — pinned per 02-13-PLAN.md
// `<codec_keys_pinned>` table). Each mapping row shape:
//
//	/cumulative_l    → cumulative_value (numeric, scale 1)
//	/battery_pct     → battery_pct (int, scale 1)
//	/temperature_c   → temperature_c (numeric, scale 1)
//	/leak            → leak_detected (bool)
//	/tamper          → tamper_detected (bool)
//
// Note: the codec emits cumulative_l as the 32-bit liter counter; the harness
// maps it directly to cumulative_value (rather than raw_value) so the ingest
// pipeline stores the codec's authoritative cumulative_l. raw_value is
// populated separately by additionally mapping /cumulative_l → raw_value so
// rollover detection has a counter to compare. The scenarios specifically
// care about both raw_value (for DetectRollover) AND cumulative_value (for
// continuity assertions).
func SeedAxiomaW1Mappings(ctx context.Context, pool *pgxpool.Pool, profileID uuid.UUID) error {
	q := sqlc.New(pool)
	type m struct {
		ptr      string
		target   string
		dataType string
		pos      int32
	}
	rows := []m{
		// Map cumulative_l → raw_value FIRST so rollover detection (which
		// reads raw_value from binding.last_raw_value) gets the 32-bit counter.
		{"/cumulative_l", "raw_value", "numeric", 0},
		{"/battery_pct", "battery_pct", "int", 1},
		{"/temperature_c", "temperature_c", "numeric", 2},
		{"/leak", "leak_detected", "bool", 3},
		{"/tamper", "tamper_detected", "bool", 4},
	}
	for _, r := range rows {
		var scale pgtype.Numeric
		_ = scale.Scan("1")
		if _, err := q.CreateMapping(ctx, sqlc.CreateMappingParams{
			DeviceProfileID: pgtype.UUID{Bytes: profileID, Valid: true},
			JsonPointer:     r.ptr,
			Target:          r.target,
			Scale:           scale,
			DataType:        r.dataType,
			Position:        r.pos,
		}); err != nil {
			return fmt.Errorf("seed axioma mapping %q: %w", r.ptr, err)
		}
	}
	return nil
}

// SeedAcrelADW300Mappings inserts the 13 mapping rows for the 3-phase
// superset, json_pointer values pinned per 02-13-PLAN.md.
//
// Note: kwh_forward → cumulative_value (electricity utility); battery_pct,
// temperature_c are first-class canonical columns; voltage/current/pf_l1..3
// are routed to extra.* JSONB (DATA-08 hybrid wide+JSONB).
func SeedAcrelADW300Mappings(ctx context.Context, pool *pgxpool.Pool, profileID uuid.UUID) error {
	q := sqlc.New(pool)
	type m struct {
		ptr      string
		target   string
		dataType string
		pos      int32
	}
	rows := []m{
		{"/kwh_forward", "raw_value", "numeric", 0},
		{"/power_total_w", "instant_value", "numeric", 1},
		{"/battery_pct", "battery_pct", "int", 2},
		{"/temperature_c", "temperature_c", "numeric", 3},
		{"/voltage_l1", "extra.voltage_l1", "numeric", 4},
		{"/current_l1", "extra.current_l1", "numeric", 5},
		{"/pf_l1", "extra.pf_l1", "numeric", 6},
		{"/voltage_l2", "extra.voltage_l2", "numeric", 7},
		{"/current_l2", "extra.current_l2", "numeric", 8},
		{"/pf_l2", "extra.pf_l2", "numeric", 9},
		{"/voltage_l3", "extra.voltage_l3", "numeric", 10},
		{"/current_l3", "extra.current_l3", "numeric", 11},
		{"/pf_l3", "extra.pf_l3", "numeric", 12},
	}
	for _, r := range rows {
		var scale pgtype.Numeric
		_ = scale.Scan("1")
		if _, err := q.CreateMapping(ctx, sqlc.CreateMappingParams{
			DeviceProfileID: pgtype.UUID{Bytes: profileID, Valid: true},
			JsonPointer:     r.ptr,
			Target:          r.target,
			Scale:           scale,
			DataType:        r.dataType,
			Position:        r.pos,
		}); err != nil {
			return fmt.Errorf("seed acrel mapping %q: %w", r.ptr, err)
		}
	}
	return nil
}

// extraJSONB is a small helper for tests asserting decoded.object content.
// Returns the parsed map or {}.
func extraJSONB(raw []byte) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return map[string]any{}
	}
	return m
}

// suppress unused warnings on slog.Logger / extraJSONB if a future scenario
// drops them — keep imports stable by referencing both here.
var _ = slog.Default
var _ = extraJSONB
