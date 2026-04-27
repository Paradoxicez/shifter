---
phase: 01-foundation
plan: 07
type: execute
wave: 5
depends_on: [02, 03, 04]
files_modified:
  - go.mod
  - go.sum
  - internal/auth/argon2id.go
  - internal/auth/argon2id_test.go
  - internal/auth/password_strength.go
  - internal/auth/password_strength_test.go
autonomous: true
requirements:
  - AUTH-01
must_haves:
  truths:
    - "Hash + Verify round-trip returns true for the original password"
    - "Verify with the wrong password returns false (constant-time comparison)"
    - "PHC encoded output starts with $argon2id$v=19$m=19456,t=2,p=1$"
    - "Verify rejects malformed PHC strings without panicking"
    - "PasswordStrength returns weak/ok/good/strong tier based on length + character class diversity"
  artifacts:
    - path: "internal/auth/argon2id.go"
      provides: "Hash + Verify (Argon2id, OWASP-2025 params m=19456 t=2 p=1, salt=16 key=32)"
      contains: "func Hash"
    - path: "internal/auth/password_strength.go"
      provides: "Stateless strength tier evaluator (length + classes) for UI hints"
      contains: "PasswordStrength"
  key_links:
    - from: "internal/auth/argon2id.go"
      to: "golang.org/x/crypto/argon2"
      via: "argon2.IDKey"
      pattern: "argon2\\.IDKey"
---

<objective>
Implement Argon2id password hashing and verification with PHC-format encoding (RESEARCH §Pattern 5, OWASP-recommended parameters m=19456 KiB, t=2, p=1, salt=16 bytes, key=32 bytes). Add a stateless password-strength tier evaluator used by the wizard step 1 (Plan 16) and the change-password dialog (Plan 11).

Purpose: AUTH-01 (login). Plan 09 (login handler), Plan 11 (change password), Plan 14/15 (wizard creates the bootstrap admin). Without this plan, no password can be safely stored.

Output: `go test ./internal/auth -run 'TestArgon2id_'` passes including round-trip + constant-time + PHC parse error tests.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/01-foundation/01-RESEARCH.md
@.planning/phases/01-foundation/01-CONTEXT.md
@.planning/phases/01-foundation/01-UI-SPEC.md
@01-02-test-harness-PLAN.md

<interfaces>
RESEARCH §Pattern 5 (lines 500-568) — the Argon2id Hash+Verify pattern is given verbatim. OWASP params: `m=19456 KiB, t=2, p=1`. Salt 16 bytes, key 32 bytes.

Strength tiers used by UI (UI-SPEC §Force-change-password section, line 234): weak/ok/good/strong. UI-SPEC heuristic (UI-SPEC line 548): "At least 12 characters with mixed case, a number, and a symbol."

Public API:
```go
package auth

func Hash(password string) (string, error)
func Verify(password, encoded string) (bool, error)

type Strength int
const (
    StrengthWeak Strength = iota
    StrengthOK
    StrengthGood
    StrengthStrong
)

func PasswordStrength(password string) Strength
```
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Argon2id Hash/Verify with PHC encoding</name>
  <files>go.mod, go.sum, internal/auth/argon2id.go, internal/auth/argon2id_test.go</files>
  <read_first>
    - .planning/phases/01-foundation/01-RESEARCH.md §"Pattern 5: Argon2id with PHC encoding" (lines 500-568) — copy verbatim
    - .planning/phases/01-foundation/01-VALIDATION.md (TestArgon2id_RoundTrip, TestVerify_BadPassword_ConstantTime expected names)
    - 01-02-test-harness-PLAN.md (existing skip-stub file)
  </read_first>
  <behavior>
    - TestArgon2id_RoundTrip: Hash("hunter2") returns a string starting with `$argon2id$v=19$m=19456,t=2,p=1$`. Verify("hunter2", encoded) returns (true, nil). Hash twice with the same password produces different encoded strings (salt differs).
    - TestVerify_BadPassword_ConstantTime: Verify("wrong", encoded) returns (false, nil). Time difference between true and false comparisons must not leak per-byte information (smoke test: > 100 iterations of false vs true don't fail any sanity check).
    - TestArgon2id_PHCParseError: Verify("anything", "not-a-phc-string") returns (false, error containing "argon2id"). Verify with wrong version (`v=20`) returns error. Verify with truncated string returns error. None panic.
    - TestArgon2id_VersionMismatch: PHC string with `v=20` returns false + error "argon2 version mismatch".
  </behavior>
  <action>
1. Install `golang.org/x/crypto`:
   ```bash
   go get golang.org/x/crypto/argon2
   ```

2. Create `internal/auth/argon2id.go` — VERBATIM from RESEARCH §Pattern 5. The constants and function bodies are not negotiable (OWASP-recommended values):
   ```go
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

   // OWASP Password Storage Cheat Sheet (2025): minimum Argon2id params.
   // Source: https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html
   const (
       argonMemKiB  uint32 = 19_456 // 19 MiB
       argonTime    uint32 = 2
       argonThreads uint8  = 1
       argonSaltLen        = 16
       argonKeyLen  uint32 = 32
   )

   // Hash returns a PHC-format string: $argon2id$v=19$m=19456,t=2,p=1$<b64salt>$<b64hash>
   func Hash(password string) (string, error) {
       salt := make([]byte, argonSaltLen)
       if _, err := rand.Read(salt); err != nil {
           return "", fmt.Errorf("read salt: %w", err)
       }
       key := argon2.IDKey([]byte(password), salt, argonTime, argonMemKiB, argonThreads, argonKeyLen)
       b64 := base64.RawStdEncoding.EncodeToString
       return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
           argon2.Version, argonMemKiB, argonTime, argonThreads, b64(salt), b64(key)), nil
   }

   // Verify is constant-time. It re-parses the PHC params so future hardening
   // (raising m or t) doesn't break existing hashes.
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
   ```

3. Replace `internal/auth/argon2id_test.go` (the skip-stubs from Plan 02):
   ```go
   package auth

   import (
       "strings"
       "testing"

       "github.com/stretchr/testify/require"
   )

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

   func TestVerify_BadPassword_ConstantTime(t *testing.T) {
       hash, err := Hash("good-password-1234567890")
       require.NoError(t, err)
       ok, err := Verify("wrong-password-........", hash)
       require.NoError(t, err)
       require.False(t, ok)
   }

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
   ```
  </action>
  <verify>
    <automated>go test ./internal/auth -run 'TestArgon2id_' -race -count=1 -v</automated>
  </verify>
  <acceptance_criteria>
    - File `internal/auth/argon2id.go` exports `func Hash(password string) (string, error)` and `func Verify(password, encoded string) (bool, error)`
    - Constants in `internal/auth/argon2id.go` are EXACTLY: `argonMemKiB uint32 = 19_456`, `argonTime uint32 = 2`, `argonThreads uint8 = 1`, `argonSaltLen = 16`, `argonKeyLen uint32 = 32` (OWASP recommendation; do NOT modify)
    - `Verify` uses `subtle.ConstantTimeCompare` (grep proof: file contains the literal string `subtle.ConstantTimeCompare`)
    - `Hash("x")` returns a string starting with literal `$argon2id$v=19$m=19456,t=2,p=1$`
    - `Verify("x", malformedPHC)` returns `(false, error)` for at least 7 malformed-input cases without panicking
    - Command `go test ./internal/auth -run 'TestArgon2id_' -race -count=1` exits 0 (per VALIDATION.md)
    - Command `go test ./internal/auth -run TestVerify_BadPassword_ConstantTime` exits 0 (per VALIDATION.md)
  </acceptance_criteria>
  <done>
    Argon2id production-ready. Plan 09 imports `auth.Hash` for create-admin and login handlers; Plan 11 imports it for change-password.
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Password strength tier evaluator</name>
  <files>internal/auth/password_strength.go, internal/auth/password_strength_test.go</files>
  <read_first>
    - .planning/phases/01-foundation/01-UI-SPEC.md §"Force-change-password screen" (line 234) — strength meter contract
    - .planning/phases/01-foundation/01-UI-SPEC.md §"Phase 1 copy table" (line 548) — "At least 12 characters with mixed case, a number, and a symbol."
  </read_first>
  <behavior>
    - "" → StrengthWeak
    - "short" → StrengthWeak (< 12 chars)
    - "twelve_chars" → StrengthOK (12 chars, lower + symbol)
    - "Twelve_charsX" → StrengthGood (mixed case + symbol + 13 chars)
    - "Strong-Pass-1!" → StrengthStrong (length 14 + 4 classes: upper, lower, digit, symbol)
    - 32-char alphabet-only → StrengthGood (length but only one class)
  </behavior>
  <action>
1. Create `internal/auth/password_strength.go`:
   ```go
   package auth

   import "unicode"

   type Strength int

   const (
       StrengthWeak Strength = iota
       StrengthOK
       StrengthGood
       StrengthStrong
   )

   func (s Strength) String() string {
       switch s {
       case StrengthWeak:   return "weak"
       case StrengthOK:     return "ok"
       case StrengthGood:   return "good"
       case StrengthStrong: return "strong"
       }
       return "unknown"
   }

   // PasswordStrength returns the UI tier: weak/ok/good/strong.
   // UI-SPEC line 548 hint: "At least 12 characters with mixed case, a number, and a symbol."
   //
   // Heuristic:
   //   length < 12          → weak
   //   length 12+, 1 class  → ok
   //   length 12+, 2 cls    → ok
   //   length 12+, 3 cls    → good
   //   length 14+, 4 cls    → strong
   //   length 12+, 4 cls    → good
   //   length 16+, 3 cls    → strong
   func PasswordStrength(password string) Strength {
       if len(password) < 12 {
           return StrengthWeak
       }
       var hasUpper, hasLower, hasDigit, hasSymbol bool
       for _, r := range password {
           switch {
           case unicode.IsUpper(r): hasUpper = true
           case unicode.IsLower(r): hasLower = true
           case unicode.IsDigit(r): hasDigit = true
           case unicode.IsPunct(r), unicode.IsSymbol(r): hasSymbol = true
           }
       }
       classes := 0
       for _, b := range []bool{hasUpper, hasLower, hasDigit, hasSymbol} {
           if b { classes++ }
       }
       switch {
       case classes == 4 && len(password) >= 14:
           return StrengthStrong
       case classes >= 3 && len(password) >= 16:
           return StrengthStrong
       case classes >= 3:
           return StrengthGood
       case classes == 4:
           return StrengthGood
       default:
           return StrengthOK
       }
   }
   ```

2. Create `internal/auth/password_strength_test.go`:
   ```go
   package auth

   import (
       "testing"

       "github.com/stretchr/testify/require"
   )

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
   ```
  </action>
  <verify>
    <automated>go test ./internal/auth -run TestPasswordStrength -race -count=1</automated>
  </verify>
  <acceptance_criteria>
    - File `internal/auth/password_strength.go` exports `type Strength int` with consts `StrengthWeak`, `StrengthOK`, `StrengthGood`, `StrengthStrong` in iota order starting at 0
    - File exports `func PasswordStrength(password string) Strength`
    - `PasswordStrength("")` returns `StrengthWeak`
    - `PasswordStrength("Strong-Pass-1!")` returns `StrengthStrong`
    - `PasswordStrength("aaaaaaaaaaaa")` returns `StrengthOK` (length 12, one class)
    - `String()` method returns "weak"/"ok"/"good"/"strong" for the four constants
    - Command `go test ./internal/auth -run TestPasswordStrength -race` exits 0
  </acceptance_criteria>
  <done>
    Strength evaluator deterministic, no I/O, no allocations beyond per-rune classification. Plan 11 / 16 will call this from API endpoints to return the tier alongside form validation.
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| user input → password hash | All passwords cross this trust boundary; no other path stores plaintext |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-07-01 | Information Disclosure | timing attacks against Verify | mitigate | `subtle.ConstantTimeCompare` — explicit grep-checked. ASVS V6. |
| T-07-02 | Tampering | weak Argon2id parameters | mitigate | Constants pinned to OWASP 2025 minimums (m=19456, t=2, p=1); `Verify` re-parses params so future hardening is non-breaking. ASVS V6. |
| T-07-03 | Information Disclosure | salt reuse | mitigate | `crypto/rand.Read(salt)` per-Hash; round-trip test asserts two hashes differ. ASVS V6. |
| T-07-04 | Tampering | bcrypt+argon2 mixed (per CLAUDE.md "do not use") | mitigate | Only Argon2id is implemented; no bcrypt import. ASVS V6. |
| T-07-05 | Denial of Service | very long password DoS via Argon2 cost | mitigate | Plan 09/11 reject password length >256 chars before calling Hash. (Documented for downstream plan.) |
</threat_model>

<verification>
- `internal/auth/argon2id.go` matches RESEARCH §Pattern 5 verbatim
- OWASP params hard-coded as constants (m=19456, t=2, p=1, salt=16, key=32)
- All 4 test groups (RoundTrip, ConstantTime, PHCParseError, PasswordStrength) pass
- `subtle.ConstantTimeCompare` present in source
- Strength evaluator handles 9 enumerated cases correctly
</verification>

<success_criteria>
- AUTH-01 unblocked: Hash + Verify ready for login handler (Plan 09)
- AUTH-05 unblocked: Hash + Verify ready for change-password (Plan 11)
- Wizard step 1 (Plan 14/15) can call Hash to store the bootstrap admin
- UI strength hint endpoint (Plan 11/16) can call PasswordStrength
- No bcrypt or alternative hash mixed in (CLAUDE.md "do not use")
</success_criteria>

<output>
After completion, create `.planning/phases/01-foundation/01-07-SUMMARY.md` documenting:
- Public API: `Hash`, `Verify`, `PasswordStrength`, `Strength` enum
- OWASP parameter rationale and how to upgrade later
- Test-coverage matrix
</output>
