package report

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPeriodDelta(t *testing.T) {
	t.Run("ComputeDelta_nil_prior_returns_nil", func(t *testing.T) {
		result := ComputeDelta(100.0, nil)
		require.Nil(t, result, "no prior period → nil (D-03 silent fallback)")
	})

	t.Run("ComputeDelta_increase", func(t *testing.T) {
		prior := 100.0
		result := ComputeDelta(110.0, &prior)
		require.NotNil(t, result)
		require.InDelta(t, 10.0, result.Absolute, 0.001)
		require.InDelta(t, 10.0, result.Percent, 0.001)
	})

	t.Run("ComputeDelta_decrease", func(t *testing.T) {
		prior := 100.0
		result := ComputeDelta(90.0, &prior)
		require.NotNil(t, result)
		require.InDelta(t, -10.0, result.Absolute, 0.001)
		require.InDelta(t, -10.0, result.Percent, 0.001)
	})

	t.Run("ComputeDelta_prior_zero_no_div_by_zero", func(t *testing.T) {
		prior := 0.0
		result := ComputeDelta(100.0, &prior)
		require.NotNil(t, result)
		require.InDelta(t, 100.0, result.Absolute, 0.001)
		require.InDelta(t, 0.0, result.Percent, 0.001, "prior == 0 → Percent = 0.0 (no /0 trap)")
	})

	t.Run("ComputeDelta_rounds_to_one_decimal", func(t *testing.T) {
		prior := 3.0
		result := ComputeDelta(4.0, &prior)
		require.NotNil(t, result)
		// 4/3 * 100 = 33.333... → rounds to 33.3
		require.InDelta(t, 33.3, result.Percent, 0.01)
	})

	t.Run("ComputeYoY_with_prior_year_data", func(t *testing.T) {
		priorYear := 80.0
		result := ComputeYoY(100.0, &priorYear)
		require.NotNil(t, result, "prior-year data present → delta populated")
		require.InDelta(t, 20.0, result.Absolute, 0.001)
		require.InDelta(t, 25.0, result.Percent, 0.001)
	})

	t.Run("ComputeYoY_no_prior_year_data_silent_fallback", func(t *testing.T) {
		result := ComputeYoY(100.0, nil)
		require.Nil(t, result, "no prior-year data → nil (D-03 silent fallback)")
	})
}
