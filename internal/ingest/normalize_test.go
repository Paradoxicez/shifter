package ingest

import (
	"math/big"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/profile"
)

// TestNormalizeMeasurement_SingleField — single mapping promotes one decoded
// field to a canonical column. Scale=0.001 → 100000 raw → 100.0 cumulative.
func TestNormalizeMeasurement_SingleField(t *testing.T) {
	t.Parallel()

	scale := big.NewFloat(0.001)
	mappings := []profile.Mapping{
		{JSONPointer: "/liters", Target: "raw_value", Scale: scale, DataType: "numeric", Position: 0},
	}
	decoded := map[string]any{"liters": 100000.0}

	out, err := NormalizeMeasurement(decoded, mappings, "")
	require.NoError(t, err)
	require.NotNil(t, out.RawValue)
	f, _ := out.RawValue.Float64()
	require.InDelta(t, 100.0, f, 1e-9, "raw_value scaled by 0.001")
}

// TestNormalizeMeasurement_MultipleCanonical — three mappings, three columns.
func TestNormalizeMeasurement_MultipleCanonical(t *testing.T) {
	t.Parallel()

	mappings := []profile.Mapping{
		{JSONPointer: "/liters", Target: "raw_value", DataType: "numeric", Position: 0},
		{JSONPointer: "/battery", Target: "battery_pct", DataType: "int", Position: 1},
		{JSONPointer: "/temp", Target: "temperature_c", DataType: "numeric", Position: 2},
	}
	decoded := map[string]any{
		"liters":  100.0,
		"battery": 85.0,
		"temp":    22.5,
	}

	out, err := NormalizeMeasurement(decoded, mappings, "")
	require.NoError(t, err)
	require.NotNil(t, out.RawValue)
	require.NotNil(t, out.BatteryPct)
	require.NotNil(t, out.TemperatureC)

	require.Equal(t, int16(85), *out.BatteryPct)
	require.InDelta(t, 22.5, *out.TemperatureC, 1e-3)
}

// TestNormalizeMeasurement_ExtraFields — fields with target prefix "extra."
// land in Layer1.Extra. Multiple extra mappings accumulate.
func TestNormalizeMeasurement_ExtraFields(t *testing.T) {
	t.Parallel()

	mappings := []profile.Mapping{
		{JSONPointer: "/liters", Target: "raw_value", DataType: "numeric", Position: 0},
		{JSONPointer: "/voltage_l1", Target: "extra.voltage_l1", DataType: "numeric", Position: 1},
		{JSONPointer: "/voltage_l2", Target: "extra.voltage_l2", DataType: "numeric", Position: 2},
		{JSONPointer: "/factory_id", Target: "extra.factory_id", DataType: "text", Position: 3},
	}
	decoded := map[string]any{
		"liters":     100.0,
		"voltage_l1": 230.0,
		"voltage_l2": 231.0,
		"factory_id": "ABC123",
	}

	out, err := NormalizeMeasurement(decoded, mappings, "")
	require.NoError(t, err)
	require.NotNil(t, out.Extra)
	require.Contains(t, out.Extra, "voltage_l1")
	require.Contains(t, out.Extra, "voltage_l2")
	require.Contains(t, out.Extra, "factory_id")
	require.Equal(t, "ABC123", out.Extra["factory_id"])
}

// TestNormalizeMeasurement_MissingPointer_Skips — a mapping whose pointer
// resolves to nothing is silently skipped. If the only mapping was that one,
// ErrNoCanonicalValue surfaces (no canonical value populated).
func TestNormalizeMeasurement_MissingPointer_Skips(t *testing.T) {
	t.Parallel()

	mappings := []profile.Mapping{
		{JSONPointer: "/missing", Target: "raw_value", DataType: "numeric", Position: 0},
	}
	decoded := map[string]any{"other": 42.0}

	out, err := NormalizeMeasurement(decoded, mappings, "")
	require.ErrorIs(t, err, ErrNoCanonicalValue)
	require.Nil(t, out.RawValue, "missing pointer left RawValue unset")
}

// TestNormalizeMeasurement_DataTypeBool — leak_detected with data_type=bool;
// payload provides 0/1 numeric → coerce to bool.
func TestNormalizeMeasurement_DataTypeBool(t *testing.T) {
	t.Parallel()

	mappings := []profile.Mapping{
		{JSONPointer: "/leak", Target: "leak_detected", DataType: "bool", Position: 0},
		{JSONPointer: "/v", Target: "raw_value", DataType: "numeric", Position: 1},
	}

	cases := []struct {
		name    string
		decoded map[string]any
		want    bool
	}{
		{"true_bool", map[string]any{"leak": true, "v": 1.0}, true},
		{"false_bool", map[string]any{"leak": false, "v": 1.0}, false},
		{"int_one_true", map[string]any{"leak": 1.0, "v": 1.0}, true},
		{"int_zero_false", map[string]any{"leak": 0.0, "v": 1.0}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := NormalizeMeasurement(tc.decoded, mappings, "")
			require.NoError(t, err)
			require.NotNil(t, out.LeakDetected, "leak_detected populated")
			require.Equal(t, tc.want, *out.LeakDetected)
		})
	}
}

// TestNormalizeMeasurement_PositionOrderApplied — mappings with non-monotonic
// Position values are sorted before the pass. Two mappings target the same
// canonical column; the higher-Position one wins (last write).
func TestNormalizeMeasurement_PositionOrderApplied(t *testing.T) {
	t.Parallel()

	// Both mappings target raw_value but with different scales. Position 5
	// should be applied AFTER position 1; position 5's value wins.
	mappings := []profile.Mapping{
		{JSONPointer: "/v", Target: "raw_value", Scale: big.NewFloat(1.0), DataType: "numeric", Position: 5},
		{JSONPointer: "/v", Target: "raw_value", Scale: big.NewFloat(2.0), DataType: "numeric", Position: 1},
	}
	decoded := map[string]any{"v": 10.0}

	out, err := NormalizeMeasurement(decoded, mappings, "")
	require.NoError(t, err)
	require.NotNil(t, out.RawValue)
	f, _ := out.RawValue.Float64()
	// Position 1 (scale 2.0) → 20; Position 5 (scale 1.0) → 10. Last writer wins.
	require.InDelta(t, 10.0, f, 1e-9, "Position-5 mapping with scale 1.0 overrides earlier")
}

// TestNormalizeMeasurement_InstantValueSatisfiesCanonical — a mapping that
// only populates instant_value (no raw_value) does NOT trigger
// ErrNoCanonicalValue — instant flow / power is a valid canonical value.
func TestNormalizeMeasurement_InstantValueSatisfiesCanonical(t *testing.T) {
	t.Parallel()

	mappings := []profile.Mapping{
		{JSONPointer: "/flow", Target: "instant_value", DataType: "numeric", Position: 0},
	}
	decoded := map[string]any{"flow": 12.5}

	out, err := NormalizeMeasurement(decoded, mappings, "")
	require.NoError(t, err, "instant_value alone satisfies canonical requirement")
	require.NotNil(t, out.InstantValue)
	require.Nil(t, out.RawValue)
}

// TestNormalizeMeasurement_NoVendorSwitchInSource — DATA-09 + Anti-Pattern
// "no decoders.ts": the normalize source MUST NOT contain vendor name string
// literals. The mapping table is the only configuration.
func TestNormalizeMeasurement_NoVendorSwitchInSource(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile("normalize.go")
	require.NoError(t, err, "read normalize.go")

	src := strings.ToLower(string(body))
	forbidden := []string{
		"axioma", "acrel", "kamstrup", "diehl", "itron",
	}
	for _, name := range forbidden {
		require.NotContains(t, src, name,
			"normalize.go MUST NOT mention vendor %q (DATA-09 + Anti-Pattern)", name)
	}
}

// TestNormalizeMeasurement_GrepCheckMatchesPlan — additionally use the literal
// grep -E pattern from the plan's verify step so the regression check is
// exactly the one a CI pipeline would run.
func TestNormalizeMeasurement_GrepCheckMatchesPlan(t *testing.T) {
	t.Parallel()

	cmd := exec.Command("grep", "-iE", "axioma|acrel|kamstrup|diehl|itron", "normalize.go")
	out, _ := cmd.CombinedOutput()
	if len(out) > 0 {
		t.Fatalf("normalize.go contains forbidden vendor name(s):\n%s", string(out))
	}
}

// TestNormalizeMeasurement_RootPointer_WholeObject — RFC 6901: empty pointer
// addresses the whole document. Useful for codecs that emit a single number
// at the root.
func TestNormalizeMeasurement_RootPointer_WholeObject(t *testing.T) {
	t.Parallel()

	// JSON document is just a number at root: when the codec returns
	// {"value": 100} we'd map "/value"; for completeness pin the empty-pointer
	// behavior (returns the whole map → coerce numeric on a map fails →
	// skip → ErrNoCanonicalValue).
	mappings := []profile.Mapping{
		{JSONPointer: "", Target: "raw_value", DataType: "numeric", Position: 0},
	}
	decoded := map[string]any{"x": 1.0}

	out, err := NormalizeMeasurement(decoded, mappings, "")
	// The whole-document is map[string]any, not a number — coerce fails →
	// nothing populated → ErrNoCanonicalValue.
	require.ErrorIs(t, err, ErrNoCanonicalValue)
	require.Nil(t, out.RawValue)
}

// TestNormalizeMeasurement_BatteryCurve_LiSOCl23V6 verifies that passing
// battery_curve="li_socl2_3v6" and Extra["battery_v"]=3.5 produces a
// BatteryPct of approximately 92 (band 3.4–3.6: 85–100, halfway → ~92).
func TestNormalizeMeasurement_BatteryCurve_LiSOCl23V6(t *testing.T) {
	t.Parallel()

	// The codec emits battery_v as a float; the mapping routes it to extra.battery_v.
	// The battery curve then converts that voltage to BatteryPct.
	mappings := []profile.Mapping{
		{JSONPointer: "/v", Target: "raw_value", DataType: "numeric", Position: 0},
		{JSONPointer: "/battery_v", Target: "extra.battery_v", DataType: "numeric", Position: 1},
	}
	decoded := map[string]any{
		"v":         1000.0,
		"battery_v": 3.5,
	}

	out, err := NormalizeMeasurement(decoded, mappings, BatteryCurveLiSOCl23V6)
	require.NoError(t, err)
	require.NotNil(t, out.BatteryPct, "BatteryPct must be set when battery curve applied")
	// 3.5V is in the 3.4–3.6 band (85–100), so ~92%.
	require.GreaterOrEqual(t, *out.BatteryPct, int16(90))
	require.LessOrEqual(t, *out.BatteryPct, int16(95))
	// Extra["battery_v"] must still be preserved.
	require.Contains(t, out.Extra, "battery_v", "extra.battery_v should be preserved")
}

// TestNormalizeMeasurement_BatteryCurve_LinearPct_Passthrough verifies that
// linear_pct does NOT override the codec's own battery_pct mapping.
func TestNormalizeMeasurement_BatteryCurve_LinearPct_Passthrough(t *testing.T) {
	t.Parallel()

	mappings := []profile.Mapping{
		{JSONPointer: "/v", Target: "raw_value", DataType: "numeric", Position: 0},
		{JSONPointer: "/battery_pct", Target: "battery_pct", DataType: "int", Position: 1},
		{JSONPointer: "/battery_v", Target: "extra.battery_v", DataType: "numeric", Position: 2},
	}
	decoded := map[string]any{
		"v":           1000.0,
		"battery_pct": 75.0,
		"battery_v":   3.5,
	}

	// With linear_pct curve, the codec's own battery_pct (75) must win.
	out, err := NormalizeMeasurement(decoded, mappings, BatteryCurveLinearPct)
	require.NoError(t, err)
	require.NotNil(t, out.BatteryPct)
	require.Equal(t, int16(75), *out.BatteryPct, "linear_pct: codec's 75%% must be preserved")
}

// TestNormalizeMeasurement_BatteryCurve_NoBatteryV_NoOverride verifies that
// if the decoded payload has no battery_v field in Extra, the curve is not
// applied (no spurious BatteryPct is generated).
func TestNormalizeMeasurement_BatteryCurve_NoBatteryV_NoOverride(t *testing.T) {
	t.Parallel()

	mappings := []profile.Mapping{
		{JSONPointer: "/v", Target: "raw_value", DataType: "numeric", Position: 0},
	}
	decoded := map[string]any{"v": 1000.0}

	out, err := NormalizeMeasurement(decoded, mappings, BatteryCurveLiSOCl23V6)
	require.NoError(t, err)
	// No battery_v in Extra → BatteryPct must remain nil.
	require.Nil(t, out.BatteryPct, "no battery_v → no BatteryPct override")
}
