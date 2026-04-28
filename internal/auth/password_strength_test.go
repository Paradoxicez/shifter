package auth

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPasswordStrength — covers the UI-SPEC §548 hint:
// "At least 12 characters with mixed case, a number, and a symbol."
//
// Tier ladder:
//
//	weak    → <12 chars
//	ok      → 12+ chars, 1–2 character classes
//	good    → 12+ chars, 3 classes (or 12+ with 4 classes < length 14)
//	strong  → 14+ chars with 4 classes, OR 16+ chars with 3+ classes
func TestPasswordStrength(t *testing.T) {
	cases := []struct {
		name string
		pw   string
		want Strength
	}{
		{"empty", "", StrengthWeak},
		{"short", "short", StrengthWeak},
		{"11 chars", "abcdefghijk", StrengthWeak},
		{"12 chars one class", "abcdefghijkl", StrengthOK},
		{"12 chars symbol+lower", "twelve_chars", StrengthOK},
		{"13 chars three classes", "Twelve_charsX", StrengthGood},
		{"14 chars four classes", "Strong-Pass-1!", StrengthStrong},
		{"32 chars one class", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", StrengthOK},
		{"16 chars three classes", "Hunter2-good-key", StrengthStrong},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := PasswordStrength(c.pw)
			require.Equal(t, c.want, got, "pw=%q want=%s got=%s", c.pw, c.want, got)
		})
	}
}

// TestStrengthString — String() returns the lowercase tier name used by the UI
// strength meter (UI-SPEC §234).
func TestStrengthString(t *testing.T) {
	require.Equal(t, "weak", StrengthWeak.String())
	require.Equal(t, "ok", StrengthOK.String())
	require.Equal(t, "good", StrengthGood.String())
	require.Equal(t, "strong", StrengthStrong.String())
}
