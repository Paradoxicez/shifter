package alert

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestEngineEvaluateContext_HoldsDeps — the EvaluateContext struct must
// expose Pool, Queries, Hub, InstallTZ, Log fields so the three workers
// (Plan 06-02 / 06-03) can share the bundle without per-call wiring.
func TestEngineEvaluateContext_HoldsDeps(t *testing.T) {
	ctx := EvaluateContext{
		InstallTZ: time.UTC,
	}
	// Zero-value compile-check + simple sanity. Real types are exercised
	// by integration tests in Plan 06-02 / 06-03 via testcontainer-backed
	// worker tests.
	require.Equal(t, time.UTC, ctx.InstallTZ)
}

// TestCompareBound — pure helper that the threshold subtype calls inside
// its eval loop.
func TestCompareBound(t *testing.T) {
	high := 40.0
	low := 5.0

	cases := []struct {
		name  string
		value float64
		high  *float64
		low   *float64
		want  bool
	}{
		{"value above high → breach", 42, &high, nil, true},
		{"value at high → no breach (strict >)", 40, &high, nil, false},
		{"value below high → no breach", 39, &high, nil, false},
		{"value below low → breach", 4, nil, &low, true},
		{"value at low → no breach (strict <)", 5, nil, &low, false},
		{"value above low → no breach", 6, nil, &low, false},
		{"value in range → no breach", 20, &high, &low, false},
		{"value above high with both bounds → breach", 41, &high, &low, true},
		{"value below low with both bounds → breach", 3, &high, &low, true},
		{"both bounds nil → no breach", 999, nil, nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := CompareBound(c.value, c.high, c.low)
			require.Equal(t, c.want, got)
		})
	}
}
