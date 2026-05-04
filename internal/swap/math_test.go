package swap

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

// floatFromString builds a *big.Float at the package's chosen precision from
// a decimal string — the readable form for fixture values.
func floatFromString(t *testing.T, s string) *big.Float {
	t.Helper()
	f, _, err := big.ParseFloat(s, 10, numericPrecision, big.ToNearestEven)
	require.NoError(t, err, "parse %q", s)
	return f
}

// floatEqual compares two *big.Float at the package's precision via
// f.Cmp() == 0. Avoids float64 round-trip noise.
func floatEqual(t *testing.T, want, got *big.Float, msg string) {
	t.Helper()
	require.Equal(t, 0, want.Cmp(got), "%s: want=%s got=%s", msg, want.Text('g', 32), got.Text('g', 32))
}

// TestProposeOffset_HappyPath — DATA-04 continuity baseline. R=12345, N=0
// (new meter from zero) → offset must equal 12345 so the new meter's first
// display = 0 + 12345 = 12345 = the outgoing display. No discontinuity.
func TestProposeOffset_HappyPath(t *testing.T) {
	R := floatFromString(t, "12345")
	N := floatFromString(t, "0")

	got := ProposeOffset(R, N)

	floatEqual(t, floatFromString(t, "12345"), got, "offset = R - N when N=0")
}

// TestProposeOffset_NewMeterNonZero — refurbished new meter starts non-zero.
// R=12345, N=100 → offset=12245, so display = 100 + 12245 = 12345. Same
// continuity guarantee, less obvious arithmetic.
func TestProposeOffset_NewMeterNonZero(t *testing.T) {
	R := floatFromString(t, "12345")
	N := floatFromString(t, "100")

	got := ProposeOffset(R, N)

	floatEqual(t, floatFromString(t, "12245"), got, "offset = R - N for non-zero N")
}

// TestProposeOffset_NewMeterAheadOfOutgoing — pathological but possible: a
// refurbished new meter has a higher initial counter than the outgoing's
// captured display. R=100, N=500 → offset=-400 (display = 500 + -400 = 100).
// Negative offsets are legal — the schema is NUMERIC, not unsigned.
func TestProposeOffset_NewMeterAheadOfOutgoing(t *testing.T) {
	R := floatFromString(t, "100")
	N := floatFromString(t, "500")

	got := ProposeOffset(R, N)

	floatEqual(t, floatFromString(t, "-400"), got, "negative offset legal for N > R")
}

// TestProposeOffset_FractionalReadings — water meters report fractional
// cubic meters; offset math must preserve fractional precision.
func TestProposeOffset_FractionalReadings(t *testing.T) {
	R := floatFromString(t, "1234.567")
	N := floatFromString(t, "0.123")

	got := ProposeOffset(R, N)

	floatEqual(t, floatFromString(t, "1234.444"), got, "fractional precision preserved")
}

// TestProposeOffset_PurityNoMutation — input pointers MUST NOT be mutated.
// Hot-path callers reuse the same R / N values across logs; mutation would
// silently corrupt downstream audit_log entries.
func TestProposeOffset_PurityNoMutation(t *testing.T) {
	R := floatFromString(t, "12345")
	N := floatFromString(t, "100")
	rOriginal := new(big.Float).Copy(R)
	nOriginal := new(big.Float).Copy(N)

	_ = ProposeOffset(R, N)

	floatEqual(t, rOriginal, R, "R must not be mutated")
	floatEqual(t, nOriginal, N, "N must not be mutated")
}

// TestDetectRollover_True — counter wrapped: prev=4294967295 (2^32 - 1), curr=10.
// Classic 32-bit rollover for a meter that wraps at 2^32.
func TestDetectRollover_True(t *testing.T) {
	prev := floatFromString(t, "4294967295")
	curr := floatFromString(t, "10")

	require.True(t, DetectRollover(prev, curr), "counter wrap: curr < prev")
}

// TestDetectRollover_False — normal monotonic increment.
func TestDetectRollover_False(t *testing.T) {
	prev := floatFromString(t, "10")
	curr := floatFromString(t, "11")

	require.False(t, DetectRollover(prev, curr), "monotonic: curr > prev")
}

// TestDetectRollover_EqualNotRollover — curr == prev is NOT a rollover (it's
// a stalled meter or replayed uplink). DetectRollover returns false.
func TestDetectRollover_EqualNotRollover(t *testing.T) {
	prev := floatFromString(t, "12345")
	curr := floatFromString(t, "12345")

	require.False(t, DetectRollover(prev, curr), "equal is not a rollover")
}

// TestApplyRollover_Add2to32 — 32-bit counter modulus. current_offset=0,
// modulus=4294967296 → new_offset=4294967296.
func TestApplyRollover_Add2to32(t *testing.T) {
	current := floatFromString(t, "0")

	got := ApplyRollover(current, 4294967296)

	floatEqual(t, floatFromString(t, "4294967296"), got, "0 + 2^32 = 2^32")
}

// TestApplyRollover_PreservesPrecision — fractional offset + integer modulus
// must remain exact (no float64 round-trip, no decimal drift).
func TestApplyRollover_PreservesPrecision(t *testing.T) {
	current := floatFromString(t, "12345.6789")

	got := ApplyRollover(current, 10000000)

	floatEqual(t, floatFromString(t, "10012345.6789"), got, "fractional preserved across +modulus")
}

// TestApplyRollover_PurityNoMutation — input offset MUST NOT be mutated;
// caller still holds a reference for audit_log Before/After construction.
func TestApplyRollover_PurityNoMutation(t *testing.T) {
	current := floatFromString(t, "12345.6789")
	currentCopy := new(big.Float).Copy(current)

	_ = ApplyRollover(current, 4294967296)

	floatEqual(t, currentCopy, current, "currentOffset must not be mutated")
}
