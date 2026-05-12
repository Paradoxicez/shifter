package user

import (
	"crypto/rand"
	"encoding/binary"
	"errors"

	"github.com/shifter-io/shifter/internal/auth"
)

// passwordAlphabet excludes the ambiguous characters 1, l, I, 0, O per
// CONTEXT.md "Claude's Discretion" + D-23. 65 chars: 26 lowercase minus l,
// 26 uppercase minus I and O, 10 digits minus 1 and 0, 16 symbols.
//
// At length 16 the search space is 65^16 ≈ 2^96 — well above any reasonable
// brute-force threshold and well above what Phase 1's StrengthScore
// evaluator expects.
const passwordAlphabet = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789!@#$%^&*"

// passwordLen is the random-password length per D-23 ("≥ 16 chars from full
// printable ASCII excluding ambiguous chars 1lI0O").
const passwordLen = 16

// generateMaxRetries caps the re-roll loop in GenerateRandomPassword. At
// length 16 from this alphabet the strength evaluator should pass on the
// first roll, but the bounded retry makes the function robust to future
// alphabet shrinkage that might dip below the StrengthGood threshold.
const generateMaxRetries = 10

// ErrPasswordGenFailed is returned when generateMaxRetries rolls have all
// failed the strength evaluator. Concrete trigger: a future alphabet edit
// that breaks the ≥4-character-class invariant — the function MUST fail
// loudly rather than ship a weak password.
var ErrPasswordGenFailed = errors.New("user: failed to produce strong password after retries")

// GenerateRandomPassword produces a uniformly-random password of length 16
// from an alphabet that excludes the ambiguous characters 1lI0O. Each
// candidate is verified against Phase 1's StrengthScore evaluator (D-28)
// before being returned; up to generateMaxRetries rolls are attempted.
//
// crypto/rand drives the byte stream. To avoid modulo bias against a 65-char
// alphabet (256 % 65 = 61 ≠ 0), the function consumes 32-bit uint segments
// and rejects values that would land in the truncated range — the same
// rejection-sampling pattern used by crypto/rand.Int.
//
// Returned passwords always include at least one lowercase, one uppercase,
// one digit, and one symbol (statistically expected at length 16 from this
// alphabet; the strength evaluator confirms before returning). The
// stronger-than-required nature of the output is intentional: D-28 says the
// same evaluator gates BOTH admin-typed passwords AND generated ones, so the
// generator must clear the highest tier the evaluator emits.
func GenerateRandomPassword() (string, error) {
	const albLen = uint32(len(passwordAlphabet))
	// max-uint32 mod-bias rejection threshold.
	bias := (^uint32(0)) - ((^uint32(0)) % albLen)

	for tries := 0; tries < generateMaxRetries; tries++ {
		out := make([]byte, passwordLen)
		idx := 0
		// Generate uniform indices via rejection sampling.
		for idx < passwordLen {
			var buf [4]byte
			if _, err := rand.Read(buf[:]); err != nil {
				return "", err
			}
			v := binary.BigEndian.Uint32(buf[:])
			if v >= bias {
				continue // rejected, re-roll this index
			}
			out[idx] = passwordAlphabet[v%albLen]
			idx++
		}
		candidate := string(out)
		if auth.PasswordStrength(candidate) >= auth.StrengthGood {
			return candidate, nil
		}
	}
	return "", ErrPasswordGenFailed
}
