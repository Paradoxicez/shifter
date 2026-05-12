package ingest

// BatteryCurveType is the battery curve enum from D-33 catalog (migration 0050
// CHECK constraint). Values match the CHECK constraint in 0050_catalog_metadata.
type BatteryCurveType = string

const (
	// BatteryCurveLinearPct signals that the codec already emits battery_pct
	// directly. The ingest path must not override it — this is a passthrough
	// sentinel that returns (0, false) from ApplyBatteryCurve.
	BatteryCurveLinearPct = "linear_pct"

	// BatteryCurveLiSOCl23V6 is the 5-breakpoint plateau-then-cliff curve for
	// Lithium Thionyl Chloride (Li-SOCl2) 3.6V nominal cells — the chemistry
	// used by Itron KINMY LoRa water meter modules and similar devices.
	// Breakpoints from RESEARCH §A3: 3.6V=100%, 3.4V=85%, 3.2V=50%, 3.0V=20%,
	// 2.8V=0%. Linear interpolation within each band.
	BatteryCurveLiSOCl23V6 = "li_socl2_3v6"

	// BatteryCurveLiMnO23V0 is a stub linear curve for Lithium Manganese Dioxide
	// (Li-MnO2) 3.0V nominal cells. Nominal=3.0V, depletion=2.4V. Stub — will be
	// refined in v1.1 with real datasheet breakpoints (T-07-09a-01 disposition).
	BatteryCurveLiMnO23V0 = "li_mnox_3v0"

	// BatteryCurveAlkaline3V0 is a stub linear curve for alkaline 3.0V cells.
	// Nominal=3.0V, cutoff=2.0V. Stub — will be refined in v1.1.
	BatteryCurveAlkaline3V0 = "alkaline_3v0"

	// BatteryCurveNone signals no battery — ApplyBatteryCurve returns (0, false).
	BatteryCurveNone = "none"
)

// ApplyBatteryCurve converts a battery voltage to a battery_pct (0–100)
// per the named curve.
//
// Returns (pct, true) when the curve was applied and pct is valid.
// Returns (0, false) for "linear_pct" or "none" — these are passthrough
// sentinels meaning the ingest path MUST NOT override the codec's own
// battery_pct (if any).
//
// Out-of-range voltages (negative, extremely high) are handled defensively:
// the lowest band clamps to 0 and the highest clamps to 100 (T-07-09a-02
// mitigation — never panics, never produces nonsense percentages).
func ApplyBatteryCurve(curve BatteryCurveType, voltageV float64) (int16, bool) {
	switch curve {
	case BatteryCurveLinearPct, BatteryCurveNone, "":
		return 0, false
	case BatteryCurveLiSOCl23V6:
		return liSOCl23V6(voltageV), true
	case BatteryCurveLiMnO23V0:
		return liMnO23V0(voltageV), true
	case BatteryCurveAlkaline3V0:
		return alkaline3V0(voltageV), true
	default:
		// Unknown curve — safe default, never crash (T-07-09a-02).
		return 0, false
	}
}

// liSOCl23V6 implements the 5-breakpoint plateau-then-cliff discharge curve
// for Li-SOCl2 3.6V nominal cells. Breakpoints: 3.6V=100, 3.4V=85, 3.2V=50,
// 3.0V=20, 2.8V=0. Linear interpolation between breakpoints.
func liSOCl23V6(v float64) int16 {
	switch {
	case v >= 3.6:
		return 100
	case v >= 3.4:
		return int16(85 + (v-3.4)/(3.6-3.4)*15)
	case v >= 3.2:
		return int16(50 + (v-3.2)/(3.4-3.2)*35)
	case v >= 3.0:
		return int16(20 + (v-3.0)/(3.2-3.0)*30)
	case v >= 2.8:
		return int16((v - 2.8) / (3.0 - 2.8) * 20)
	default:
		return 0
	}
}

// liMnO23V0 is a stub linear curve for Li-MnO2 3.0V nominal cells.
// Nominal=3.0V → 100%, depletion=2.4V → 0%. Linear between. Stub — refine
// in v1.1 with real datasheet breakpoints (T-07-09a-01).
func liMnO23V0(v float64) int16 {
	if v >= 3.0 {
		return 100
	}
	if v <= 2.4 {
		return 0
	}
	return int16((v - 2.4) / (3.0 - 2.4) * 100)
}

// alkaline3V0 is a stub linear curve for alkaline 3.0V cells.
// Nominal=3.0V → 100%, cutoff=2.0V → 0%. Linear between. Stub — refine
// in v1.1 (T-07-09a-01).
func alkaline3V0(v float64) int16 {
	if v >= 3.0 {
		return 100
	}
	if v <= 2.0 {
		return 0
	}
	return int16((v - 2.0) / (3.0 - 2.0) * 100)
}
