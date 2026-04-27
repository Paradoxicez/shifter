package auth

import "testing"

// TestArgon2id_RoundTrip — Hash(password) followed by Verify(hash, password)
// returns true for the original password and false for a tweaked password.
// Implementation: Plan 07 (argon2id).
func TestArgon2id_RoundTrip(t *testing.T) {
	t.Skip("Plan 07: Argon2id Hash+Verify implementation pending")
}

// TestVerify_BadPassword_ConstantTime — Verify against a known-good PHC string
// runs in roughly the same wall-clock time regardless of which byte first
// differs. Used to prove constant-time comparison (no early exit).
// Implementation: Plan 07 (argon2id).
func TestVerify_BadPassword_ConstantTime(t *testing.T) {
	t.Skip("Plan 07: constant-time Verify pending")
}

// TestArgon2id_PHCParseError — Verify against malformed PHC strings (truncated,
// wrong algorithm tag, missing parameters) returns a typed parse error rather
// than panicking.
// Implementation: Plan 07 (argon2id).
func TestArgon2id_PHCParseError(t *testing.T) {
	t.Skip("Plan 07: PHC parsing edge cases pending")
}
