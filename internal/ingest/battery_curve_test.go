package ingest

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestApplyBatteryCurve_LinearPctPassthrough verifies that linear_pct and
// none return (0, false) — sentinel: codec owns battery_pct, do nothing.
func TestApplyBatteryCurve_LinearPctPassthrough(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		curve   string
		voltage float64
	}{
		{"linear_pct_at_3v6", BatteryCurveLinearPct, 3.6},
		{"linear_pct_at_0v", BatteryCurveLinearPct, 0},
		{"none_at_3v6", BatteryCurveNone, 3.6},
		{"empty_string", "", 3.6},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pct, ok := ApplyBatteryCurve(tc.curve, tc.voltage)
			require.False(t, ok, "linear_pct and none must return ok=false")
			require.Equal(t, int16(0), pct)
		})
	}
}

// TestApplyBatteryCurve_LiSOCl23V6_Breakpoints verifies all 5 plateau
// breakpoints of the li_socl2_3v6 curve.
func TestApplyBatteryCurve_LiSOCl23V6_Breakpoints(t *testing.T) {
	t.Parallel()

	cases := []struct {
		voltageV float64
		wantPct  int16
	}{
		{3.6, 100},  // top of full plateau
		{3.7, 100},  // above full plateau → clamp to 100
		{3.4, 85},   // top of next band
		{3.2, 50},   // mid plateau
		{3.0, 20},   // low-battery threshold
		{2.8, 0},    // depletion floor
		{2.0, 0},    // below depletion → 0
	}
	for _, tc := range cases {
		t.Run("", func(t *testing.T) {
			pct, ok := ApplyBatteryCurve(BatteryCurveLiSOCl23V6, tc.voltageV)
			require.True(t, ok, "li_socl2_3v6 must return ok=true")
			require.Equal(t, tc.wantPct, pct,
				"voltage %.2fV should map to %d%%", tc.voltageV, tc.wantPct)
		})
	}
}

// TestApplyBatteryCurve_LiSOCl23V6_InterpolatedPoints validates that
// voltages between breakpoints produce plausible interpolated values.
func TestApplyBatteryCurve_LiSOCl23V6_InterpolatedPoints(t *testing.T) {
	t.Parallel()

	// 3.5V is halfway in the band 3.4–3.6 (85–100) → expect ~92-93
	pct, ok := ApplyBatteryCurve(BatteryCurveLiSOCl23V6, 3.5)
	require.True(t, ok)
	require.GreaterOrEqual(t, pct, int16(90))
	require.LessOrEqual(t, pct, int16(95))

	// 3.1V is halfway in band 3.0–3.2 (20–50) → expect ~35
	pct, ok = ApplyBatteryCurve(BatteryCurveLiSOCl23V6, 3.1)
	require.True(t, ok)
	require.GreaterOrEqual(t, pct, int16(32))
	require.LessOrEqual(t, pct, int16(38))

	// 2.9V is halfway in band 2.8–3.0 (0–20) → expect ~10
	pct, ok = ApplyBatteryCurve(BatteryCurveLiSOCl23V6, 2.9)
	require.True(t, ok)
	require.GreaterOrEqual(t, pct, int16(8))
	require.LessOrEqual(t, pct, int16(12))
}

// TestApplyBatteryCurve_LiMnO23V0_Boundaries verifies the stub linear curve
// for li_mnox_3v0 at its nominal and depleted endpoints.
func TestApplyBatteryCurve_LiMnO23V0_Boundaries(t *testing.T) {
	t.Parallel()

	pct, ok := ApplyBatteryCurve(BatteryCurveLiMnO23V0, 3.0)
	require.True(t, ok)
	require.Equal(t, int16(100), pct)

	pct, ok = ApplyBatteryCurve(BatteryCurveLiMnO23V0, 2.4)
	require.True(t, ok)
	require.Equal(t, int16(0), pct)

	pct, ok = ApplyBatteryCurve(BatteryCurveLiMnO23V0, 1.0)
	require.True(t, ok)
	require.Equal(t, int16(0), pct)
}

// TestApplyBatteryCurve_Alkaline3V0_Boundaries verifies the stub linear curve
// for alkaline_3v0 at its nominal and depleted endpoints.
func TestApplyBatteryCurve_Alkaline3V0_Boundaries(t *testing.T) {
	t.Parallel()

	pct, ok := ApplyBatteryCurve(BatteryCurveAlkaline3V0, 3.0)
	require.True(t, ok)
	require.Equal(t, int16(100), pct)

	pct, ok = ApplyBatteryCurve(BatteryCurveAlkaline3V0, 2.0)
	require.True(t, ok)
	require.Equal(t, int16(0), pct)

	pct, ok = ApplyBatteryCurve(BatteryCurveAlkaline3V0, 0.5)
	require.True(t, ok)
	require.Equal(t, int16(0), pct)
}

// TestApplyBatteryCurve_UnknownCurve_SafeDefault verifies that an unknown
// curve name returns (0, false) gracefully — T-07-09a-02 mitigation.
func TestApplyBatteryCurve_UnknownCurve_SafeDefault(t *testing.T) {
	t.Parallel()

	pct, ok := ApplyBatteryCurve("unknown_curve_xyz", 3.6)
	require.False(t, ok)
	require.Equal(t, int16(0), pct)
}

// TestApplyBatteryCurve_NegativeVoltage_SafeDefault verifies out-of-range
// battery voltages (negative) return 0 cleanly — T-07-09a-02 mitigation.
func TestApplyBatteryCurve_NegativeVoltage_SafeDefault(t *testing.T) {
	t.Parallel()

	pct, ok := ApplyBatteryCurve(BatteryCurveLiSOCl23V6, -1.0)
	require.True(t, ok, "li_socl2_3v6 should still return ok=true for negative voltage")
	require.Equal(t, int16(0), pct, "negative voltage maps to 0%%")
}
