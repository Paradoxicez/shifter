// Package auth — Argon2id password hashing.
//
// OWASP Password Storage Cheat Sheet (2025) recommends Argon2id with a minimum
// configuration of 19 MiB memory (m=19456), iteration count 2 (t=2), and 1 lane
// of parallelism (p=1). Salt is 16 bytes (cryptographically random per Hash);
// derived key is 32 bytes. These constants are pinned and MUST NOT be relaxed
// without a security review.
//
// Source: https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html
//
// Hash returns a PHC-format string parseable by `Verify`. `Verify` re-parses
// the encoded parameters so future hardening (raising m or t) does not break
// existing hashes — the new parameters travel with the hash.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// OWASP Password Storage Cheat Sheet (2025): minimum Argon2id parameters.
const (
	argonMemKiB  uint32 = 19_456 // 19 MiB
	argonTime    uint32 = 2
	argonThreads uint8  = 1
	argonSaltLen        = 16
	argonKeyLen  uint32 = 32
)

// Hash returns a PHC-format string:
//
//	$argon2id$v=19$m=19456,t=2,p=1$<b64salt>$<b64hash>
//
// Each call generates a fresh 16-byte salt via crypto/rand, so two calls with
// the same password produce different encoded strings.
func Hash(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("argon2id: read salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemKiB, argonThreads, argonKeyLen)
	b64 := base64.RawStdEncoding.EncodeToString
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemKiB, argonTime, argonThreads, b64(salt), b64(key)), nil
}

// Verify checks `password` against the PHC-format `encoded` string. The
// per-byte comparison uses subtle.ConstantTimeCompare so attackers cannot
// learn information about the stored hash via timing differences.
//
// Returns (true, nil) on match, (false, nil) on mismatch, and (false, err)
// when the encoded string cannot be parsed (truncation, wrong algorithm tag,
// version mismatch, malformed parameters, bad base64). Verify never panics on
// malformed input.
func Verify(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errors.New("argon2id: not a valid PHC string")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, fmt.Errorf("argon2id: bad version segment: %w", err)
	}
	if version != argon2.Version {
		return false, errors.New("argon2id: argon2 version mismatch")
	}
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false, fmt.Errorf("argon2id: bad param segment: %w", err)
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("argon2id: bad salt: %w", err)
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("argon2id: bad hash: %w", err)
	}
	got := argon2.IDKey([]byte(password), salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
