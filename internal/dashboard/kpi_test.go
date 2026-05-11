package dashboard

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBucketIntervalForRange covers the complete D-12 bucket schedule with a
// table-driven test.
func TestBucketIntervalForRange(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name     string
		rangeStr string
		start    time.Time
		end      time.Time
		wantDur  time.Duration
		wantErr  bool
	}{
		{
			name:     "today → 5 minutes",
			rangeStr: "today",
			wantDur:  5 * time.Minute,
		},
		{
			name:     "24h → 5 minutes",
			rangeStr: "24h",
			wantDur:  5 * time.Minute,
		},
		{
			name:     "7d → 1 hour",
			rangeStr: "7d",
			wantDur:  time.Hour,
		},
		{
			name:     "30d → 4 hours",
			rangeStr: "30d",
			wantDur:  4 * time.Hour,
		},
		{
			name:     "custom ≤ 30d → 1 hour",
			rangeStr: "custom",
			start:    now.Add(-7 * 24 * time.Hour),
			end:      now,
			wantDur:  time.Hour,
		},
		{
			name:     "custom exactly 30d → 1 hour",
			rangeStr: "custom",
			start:    now.Add(-30 * 24 * time.Hour),
			end:      now,
			wantDur:  time.Hour,
		},
		{
			name:     "custom > 30d → 1 day",
			rangeStr: "custom",
			start:    now.Add(-31 * 24 * time.Hour),
			end:      now,
			wantDur:  24 * time.Hour,
		},
		{
			name:     "custom 365d → 1 day",
			rangeStr: "custom",
			start:    now.Add(-365 * 24 * time.Hour),
			end:      now,
			wantDur:  24 * time.Hour,
		},
		{
			name:     "invalid range → error",
			rangeStr: "weekly",
			wantErr:  true,
		},
		{
			name:     "custom end before start → error",
			rangeStr: "custom",
			start:    now,
			end:      now.Add(-time.Hour),
			wantErr:  true,
		},
		{
			name:     "custom range > 1 year → error",
			rangeStr: "custom",
			start:    now.Add(-366 * 24 * time.Hour),
			end:      now,
			wantErr:  true,
		},
		{
			name:     "custom equal start and end → error",
			rangeStr: "custom",
			start:    now,
			end:      now,
			wantErr:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := BucketIntervalForRange(tc.rangeStr, tc.start, tc.end)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantDur, got)
		})
	}
}

// TestUtilitiesFor verifies capability → utility slice mapping.
func TestUtilitiesFor(t *testing.T) {
	assert.Equal(t, []string{"water"}, utilitiesFor("water"))
	assert.Equal(t, []string{"electricity"}, utilitiesFor("electricity"))
	// "both" and any other value returns both
	assert.Equal(t, []string{"water", "electricity"}, utilitiesFor("both"))
	assert.Equal(t, []string{"water", "electricity"}, utilitiesFor(""))
}

// TestUnitLabel verifies the unit strings per utility class (Plan 07 contract).
func TestUnitLabel(t *testing.T) {
	todayUnit, instantUnit := unitLabel("water")
	assert.Equal(t, "m³", todayUnit)
	assert.Equal(t, "L/min", instantUnit)

	todayUnit, instantUnit = unitLabel("electricity")
	assert.Equal(t, "kWh", todayUnit)
	assert.Equal(t, "W", instantUnit)
}
