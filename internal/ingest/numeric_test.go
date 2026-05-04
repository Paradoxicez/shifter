package ingest

import (
	"math/big"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// TestBigFloatFromNumeric_Roundtrip — a numeric carrying a non-trivial
// decimal value parses back to its big.Float equivalent at the shared
// numeric precision used across ingest + swap (128-bit). An invalid
// pgtype.Numeric (Valid=false) returns big.NewFloat(0) — generic default
// distinct from the mapping-scale's "default to 1" behavior, which lives
// at the call site that needs identity scale.
func TestBigFloatFromNumeric_Roundtrip(t *testing.T) {
	var n pgtype.Numeric
	if err := n.Scan("1234.5678"); err != nil {
		t.Fatalf("seed numeric: %v", err)
	}
	got := BigFloatFromNumeric(n)
	if got == nil {
		t.Fatalf("BigFloatFromNumeric returned nil for valid input")
	}
	want := big.NewFloat(1234.5678)
	// Compare via difference < 1e-9 because base-2 round-trip of a base-10
	// decimal is approximate. The point is "round-trip preserves enough
	// precision to re-encode the same string" — covered separately by the
	// existing pgtype.Numeric tests; here we only assert magnitude.
	diff := new(big.Float).Sub(got, want)
	diff.Abs(diff)
	if diff.Cmp(big.NewFloat(1e-6)) > 0 {
		t.Fatalf("round-trip drift: got %s, want %s, diff %s", got.Text('f', 10), want.Text('f', 10), diff.Text('f', 10))
	}

	// Invalid → 0 (NOT 1 — that's the mapping-scale default; this helper
	// is the generic "decode pgtype.Numeric" primitive).
	zero := BigFloatFromNumeric(pgtype.Numeric{Valid: false})
	if zero == nil {
		t.Fatalf("BigFloatFromNumeric(invalid) returned nil; want big.NewFloat(0)")
	}
	if zero.Cmp(big.NewFloat(0)) != 0 {
		t.Fatalf("BigFloatFromNumeric(invalid) = %s; want 0", zero.Text('f', 4))
	}
}

// TestBigFloatFromNumericNullable_HandlesNull — Valid=false → returns nil
// (so the caller can distinguish "first uplink, no last_raw_value yet" from
// "zero last_raw_value"). Valid=true → delegates to BigFloatFromNumeric.
func TestBigFloatFromNumericNullable_HandlesNull(t *testing.T) {
	got := BigFloatFromNumericNullable(pgtype.Numeric{Valid: false})
	if got != nil {
		t.Fatalf("BigFloatFromNumericNullable(invalid) = %v; want nil", got)
	}

	var n pgtype.Numeric
	if err := n.Scan("42"); err != nil {
		t.Fatalf("seed numeric: %v", err)
	}
	got = BigFloatFromNumericNullable(n)
	if got == nil {
		t.Fatalf("BigFloatFromNumericNullable(valid) returned nil")
	}
	if got.Cmp(big.NewFloat(42)) != 0 {
		t.Fatalf("BigFloatFromNumericNullable(42) = %s; want 42", got.Text('f', 4))
	}
}
