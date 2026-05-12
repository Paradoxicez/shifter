package user

import (
	"strings"
	"testing"

	"github.com/shifter-io/shifter/internal/auth"
	"github.com/stretchr/testify/require"
)

// TestGenerateRandomPassword_Strong — every generated password meets the
// length contract AND clears the Phase 1 StrengthScore evaluator at the
// Good tier or above (D-28 reuse).
func TestGenerateRandomPassword_Strong(t *testing.T) {
	for i := 0; i < 200; i++ {
		pw, err := GenerateRandomPassword()
		require.NoError(t, err)
		require.Len(t, pw, passwordLen)
		require.GreaterOrEqual(t, int(auth.PasswordStrength(pw)), int(auth.StrengthGood),
			"generated password %q must meet StrengthGood (D-28); got %s",
			pw, auth.PasswordStrength(pw))
	}
}

// TestGenerateRandomPassword_NoAmbiguousChars — the alphabet excludes the
// ambiguous characters 1, l, I, 0, O per CONTEXT.md Claude's Discretion +
// D-23. Sample a large batch and assert every character lands in the
// expected alphabet.
func TestGenerateRandomPassword_NoAmbiguousChars(t *testing.T) {
	const ambiguous = "1lI0O"
	for i := 0; i < 500; i++ {
		pw, err := GenerateRandomPassword()
		require.NoError(t, err)
		require.False(t, strings.ContainsAny(pw, ambiguous),
			"password %q must not contain any of %q", pw, ambiguous)
		for _, r := range pw {
			require.True(t, strings.ContainsRune(passwordAlphabet, r),
				"password char %q must be in passwordAlphabet", string(r))
		}
	}
}

// TestGenerateRandomPassword_AlphabetExcludesAmbiguous — defensive structural
// check on the alphabet constant itself (the production-time gate against
// accidental edits that re-introduce ambiguous chars).
func TestGenerateRandomPassword_AlphabetExcludesAmbiguous(t *testing.T) {
	for _, r := range "1lI0O" {
		require.False(t, strings.ContainsRune(passwordAlphabet, r),
			"passwordAlphabet must not contain ambiguous char %q", string(r))
	}
	require.Equal(t, 16, passwordLen, "passwordLen must be 16 per D-23")
}
