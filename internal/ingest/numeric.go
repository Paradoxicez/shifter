package ingest

import (
	"math/big"

	"github.com/jackc/pgx/v5/pgtype"
)

// BigFloatFromNumeric decodes a pgtype.Numeric to *big.Float at the package
// numeric precision used in normalize.go (matches swap.numericPrecision so
// scale arithmetic doesn't lose digits across packages).
//
// Plan 02-12 W5 fix: extracted from handler.go's package-private
// bigFloatFromNumeric so the cmd/serve resolver loader (in internal/cli) can
// reuse the same helper instead of inlining a duplicate body. Generic default
// is big.NewFloat(0) — call sites that need a different default (e.g. the
// SQLCMappingStore wants identity scale = 1 for missing rows) substitute at
// the call site after a Valid check.
func BigFloatFromNumeric(n pgtype.Numeric) *big.Float {
	if !n.Valid {
		return big.NewFloat(0)
	}
	// Use the JSON form for round-trip; pgtype.Numeric MarshalJSON renders
	// the decimal text losslessly.
	b, err := n.MarshalJSON()
	if err != nil {
		return big.NewFloat(0)
	}
	// MarshalJSON returns a string like `"123.45"` — strip quotes.
	s := string(b)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	f, _, err := big.ParseFloat(s, 10, normalizePrecision, big.ToNearestEven)
	if err != nil {
		return big.NewFloat(0)
	}
	return f
}

// BigFloatFromNumericNullable returns nil when n.Valid is false so the caller
// can distinguish "no value yet" (e.g. binding.last_raw_value before the first
// uplink) from "zero." Otherwise delegates to BigFloatFromNumeric.
func BigFloatFromNumericNullable(n pgtype.Numeric) *big.Float {
	if !n.Valid {
		return nil
	}
	return BigFloatFromNumeric(n)
}
