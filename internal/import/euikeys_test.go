package importpkg

// Phase 3 Wave 1 — DevEUI / JoinEUI / AppKey / DevAddr / session-key
// normalization. D-05: strip `0x`, `:`, `-`, lowercase, length-validate.
//
// Package name is `importpkg` because `import` is a reserved keyword in Go;
// the directory is `internal/import/` (matches the human-readable plan
// reference and the URL slug `/admin/imports`).

import "testing"

// TestNormalizeDevEUI — 16-hex, mixed case, `:`/`-`/`0x`-prefixed variants
// all normalize to the same lowercase 16-hex output.
func TestNormalizeDevEUI(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/import/euikeys.go (03-VALIDATION row euikeys_test.TestNormalizeDevEUI)")
}

// TestNormalizeJoinEUI — same shape as DevEUI; canonical zero-JoinEUI
// `0000000000000000` is allowed (some vendors).
func TestNormalizeJoinEUI(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/import/euikeys.go (03-VALIDATION row euikeys_test.TestNormalizeJoinEUI)")
}

// TestNormalizeAppKey — 32-hex (16-byte) normalization + length validation.
func TestNormalizeAppKey(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/import/euikeys.go (03-VALIDATION row euikeys_test.TestNormalizeAppKey)")
}

// TestNormalizeDevAddr — 8-hex (4-byte) ABP DevAddr.
func TestNormalizeDevAddr(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/import/euikeys.go (03-VALIDATION row euikeys_test.TestNormalizeDevAddr)")
}

// TestNormalizeNwkSKey — 32-hex session key normalization (NwkSEncKey /
// SNwkSIntKey / FNwkSIntKey / AppSKey all share this shape).
func TestNormalizeNwkSKey(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/import/euikeys.go (03-VALIDATION row euikeys_test.TestNormalizeNwkSKey)")
}

// TestNormalize_StripPrefixesAndSeparators — table-driven coverage of all
// the shapes operators paste in (whitespace, `0x`, `:`, `-`, full-width
// chars, BOM-prefixed strings from CSV import).
func TestNormalize_StripPrefixesAndSeparators(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/import/euikeys.go (03-VALIDATION row euikeys_test.TestNormalize_StripPrefixesAndSeparators)")
}
