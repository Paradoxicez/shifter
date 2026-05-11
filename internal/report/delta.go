package report

import (
	"math"
)

// DeltaResult expresses period-over-period change. Absolute is in the same
// unit as consumption (m³ for water, kWh for electricity). Percent is rounded
// to 1 decimal — UI displays "+12.3%" / "−8.1%".
type DeltaResult struct {
	Absolute float64
	Percent  float64 // (curr − prior) / prior × 100; prior == 0 → 0.0 (no /0 trap)
}

// ComputeDelta returns nil when prior is nil (no prior period available).
// Otherwise computes (curr − prior) and the percentage change.
// Division by zero is guarded: if prior == 0, Percent is 0.0.
func ComputeDelta(curr float64, prior *float64) *DeltaResult {
	if prior == nil {
		return nil
	}
	absolute := curr - *prior
	var pct float64
	if *prior != 0 {
		pct = (absolute / *prior) * 100
	}
	return &DeltaResult{Absolute: absolute, Percent: round1(pct)}
}

// ComputeYoY computes year-over-year delta given the prior-year consumption.
// Returns nil silently (D-03 silent fallback) when priorYearConsumption is nil
// (meaning no measurement data exists in the same window one year earlier).
func ComputeYoY(curr float64, priorYearConsumption *float64) *DeltaResult {
	return ComputeDelta(curr, priorYearConsumption)
}

// round1 rounds a float64 to 1 decimal place.
func round1(v float64) float64 {
	return math.Round(v*10) / 10
}
