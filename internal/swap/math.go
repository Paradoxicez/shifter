package swap

import "math/big"

// numericPrecision is the math/big.Float bit-precision used for every
// computation in this package. 128 bits ≈ 38 decimal digits — exceeds any
// plausible meter precision (typical meters report 6–10 decimal digits) so
// the math is exact for all real inputs without IEEE-754 rounding error.
//
// Centralized in one constant so a future "we need more precision" decision
// is a single-line change. big.Rat would be even more correct but the
// performance + ergonomics cost vs Float128 isn't worth it for this domain
// (no chained operations that compound rounding).
const numericPrecision = 128

// ProposeOffset computes the reading_offset for a new binding such that the
// cumulative_value remains continuous across a meter swap.
//
// Definitions (CONTEXT D-13):
//
//	display_old_at_swap = raw_old + offset_old        (the visible reading at swap time, R)
//	display_new_at_first_uplink = raw_new + offset_new   (must equal R for continuity)
//	raw_new = N (the new meter's initial counter value, often 0)
//	⇒ offset_new = R - N
//
// Operands are *big.Float (NUMERIC arbitrary precision). Both inputs are
// captured by the swap dialog at "operator confirmed swap" time. The
// returned *big.Float has prec=numericPrecision so subsequent operations
// don't silently lose digits.
//
// PURE: no DB, no IO, no time.Now() — easily unit-testable.
func ProposeOffset(R, N *big.Float) *big.Float {
	out := new(big.Float).SetPrec(numericPrecision)
	return out.Sub(R, N)
}

// DetectRollover reports whether the device counter wrapped between two
// consecutive uplinks: curr < prev (PITFALLS §2 — counter wrap detection).
//
// Caller MUST NOT call DetectRollover at a binding boundary. The new
// device's first uplink will generally have curr < prev because the new
// meter's raw counter started fresh (e.g. 0) and the previous binding's
// last_raw_value was whatever the OUTGOING meter last reported. The ingest
// pipeline (Plan 02-09) handles the binding-boundary case by skipping
// rollover detection on the first uplink for a freshly opened binding
// (binding.last_raw_value IS NULL is the boundary signal).
//
// PURE: cheap to call once per uplink in the hot path; no allocations beyond
// the comparison.
func DetectRollover(prev, curr *big.Float) bool {
	return curr.Cmp(prev) < 0
}

// ApplyRollover bumps a binding's reading_offset by counter_modulus when
// DATA-05 rollover detection fires. The modulus is the maximum value the
// raw counter can hold + 1 (e.g. 2^32 = 4294967296 for a 32-bit counter,
// 2^16 = 65536 for a 16-bit counter — both used in the wild). Stored on
// device_profile.counter_modulus.
//
//	new_offset = old_offset + counter_modulus
//
// After this call, the caller (ingest pipeline) writes the new offset to
// the binding row via sqlc.AdvanceReadingOffset and emits an audit_log entry
// with action='rollover_detected' so Phase 6 audit browse can show "device
// X wrapped at time T" alongside swap events.
//
// PURE.
func ApplyRollover(currentOffset *big.Float, counterModulus int64) *big.Float {
	m := new(big.Float).SetInt64(counterModulus)
	return new(big.Float).SetPrec(numericPrecision).Add(currentOffset, m)
}
