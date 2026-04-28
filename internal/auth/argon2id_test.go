package auth

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestArgon2id_RoundTrip — Hash(password) followed by Verify(hash, password)
// returns true for the original password and false for a tweaked password.
// Two hashes of the same password must differ (per-call salt randomness).
func TestArgon2id_RoundTrip(t *testing.T) {
	hash, err := Hash("hunter2-correct-horse-battery")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(hash, "$argon2id$v=19$m=19456,t=2,p=1$"),
		"OWASP-current params expected; got %q", hash)

	ok, err := Verify("hunter2-correct-horse-battery", hash)
	require.NoError(t, err)
	require.True(t, ok)

	// Same password hashed twice produces different encoded strings (salt randomness).
	h2, err := Hash("hunter2-correct-horse-battery")
	require.NoError(t, err)
	require.NotEqual(t, hash, h2)
}

// TestVerify_BadPassword_ConstantTime — Verify against a known-good PHC string
// returns (false, nil) for a wrong password. Constant-time comparison is
// asserted at the source level (subtle.ConstantTimeCompare grep), not via
// timing measurements (which are unreliable in CI).
func TestVerify_BadPassword_ConstantTime(t *testing.T) {
	hash, err := Hash("good-password-1234567890")
	require.NoError(t, err)

	ok, err := Verify("wrong-password-........", hash)
	require.NoError(t, err)
	require.False(t, ok)
}

// TestArgon2id_PHCParseError — Verify against malformed PHC strings returns a
// non-nil error rather than panicking. Covers truncated input, wrong algorithm
// tag, wrong version, bad parameter integers, and bad base64 segments.
func TestArgon2id_PHCParseError(t *testing.T) {
	cases := []string{
		"",
		"not-a-phc-string",
		"$argon2id$",                                    // truncated
		"$argon2i$v=19$m=19456,t=2,p=1$abc$def",         // wrong algorithm
		"$argon2id$v=20$m=19456,t=2,p=1$abc$def",        // wrong version
		"$argon2id$v=19$m=oops,t=2,p=1$abc$def",         // bad m
		"$argon2id$v=19$m=19456,t=2,p=1$!notbase64$def", // bad salt b64
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			ok, err := Verify("anything", c)
			require.False(t, ok)
			require.Error(t, err, "should reject malformed PHC: %q", c)
		})
	}
}

// TestArgon2id_VersionMismatch — A PHC string with v=20 (unknown to argon2 v19)
// returns a "version mismatch" error rather than silently passing.
func TestArgon2id_VersionMismatch(t *testing.T) {
	encoded := "$argon2id$v=20$m=19456,t=2,p=1$YWJjZGVmZ2hpamtsbW5vcA$YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXowMTIzNDU"
	ok, err := Verify("anything", encoded)
	require.False(t, ok)
	require.Error(t, err)
	require.Contains(t, err.Error(), "argon2 version mismatch")
}
