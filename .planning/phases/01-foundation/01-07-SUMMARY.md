---
phase: 01-foundation
plan: 07
subsystem: auth
tags: [go, argon2id, password, owasp, phc, security, ui-strength]

requires:
  - phase: 01-foundation
    plan: 02
    provides: testify on go.sum + skip-stub argon2id_test.go scaffold
  - phase: 01-foundation
    plan: 03
    provides: users table + password_hash column ready to receive PHC strings
  - phase: 01-foundation
    plan: 04
    provides: SHIFTER_SESSION_KEY secret idiom (analogue for any future password-related secrets)
provides:
  - internal/auth/argon2id.go (Hash + Verify with PHC encoding, OWASP-2025 params)
  - internal/auth/argon2id_test.go (round-trip, constant-time, 7-case parse-error matrix, version-mismatch)
  - internal/auth/password_strength.go (Strength enum + PasswordStrength evaluator)
  - internal/auth/password_strength_test.go (9-case ladder + String() coverage)
affects:
  - 01-08-session-manager (no direct import; sessions sit alongside the hash, not on top of it)
  - 01-09-login-ratelimit (login handler will call auth.Verify on every attempt)
  - 01-10-authz (no direct import; authz is independent of password storage)
  - 01-11-account-ui (change-password endpoint will call auth.Hash + auth.PasswordStrength)
  - 01-14-install-middleware (no direct import)
  - 01-15-install-handlers (wizard step 1 calls auth.Hash to store the bootstrap admin)
  - 01-16-install-wizard-ui (UI calls a strength endpoint that delegates to PasswordStrength)
  - 01-23-login-ui (login form does not call Hash directly; the API does)

tech-stack:
  added:
    - golang.org/x/crypto v0.50.0 (promoted from indirect to direct dependency)
  patterns:
    - "Argon2id with PHC encoding: $argon2id$v=19$m=19456,t=2,p=1$<b64salt>$<b64hash> — parameters travel with the hash so future hardening (raising m or t) does not break stored credentials"
    - "OWASP-2025 minimum constants pinned at the package level (argonMemKiB=19456, argonTime=2, argonThreads=1, argonSaltLen=16, argonKeyLen=32) — not configurable, must not be relaxed without security review"
    - "Constant-time comparison via subtle.ConstantTimeCompare on the derived 32-byte key — defuses timing oracles (T-07-01) per OWASP ASVS V6"
    - "Per-call salt via crypto/rand.Read — round-trip test asserts two hashes of the same password differ (T-07-03)"
    - "Strength tier evaluator is stateless: length floor (12) + character-class diversity (upper, lower, digit, symbol/punct) → {weak, ok, good, strong} — UI consumes via API, never recomputes client-side"

key-files:
  created:
    - internal/auth/argon2id.go
    - internal/auth/password_strength.go
    - internal/auth/password_strength_test.go
  modified:
    - go.mod
    - go.sum
    - internal/auth/argon2id_test.go (replaced Plan 02 t.Skip stubs)

key-decisions:
  - "argon2id.go is verbatim from RESEARCH §Pattern 5 with one tightening: every parse error is wrapped with 'argon2id:' prefix so log triage can route 'argon2id: bad salt' vs 'argon2id: argon2 version mismatch' without re-reading the encoded string. The plan's snippet returned `errors.New('not argon2id')` and bare `err`; production-quality wrapping makes future Plan 09 login-handler logs traceable without leaking the encoded hash itself."
  - "subtle.ConstantTimeCompare grep-checked at the source level rather than asserted via timing measurements. CI timing tests are flaky (GitHub runners share CPUs); a static grep + ASVS V6 compliance review is the canonical mitigation for T-07-01. Plan 02's TestVerify_BadPassword_ConstantTime asserts behavioral correctness ((false, nil) for wrong password); the constant-time property is enforced by the import + the linter's reading of the file."
  - "Strength enum starts at iota 0 (StrengthWeak). Downstream code can `s >= StrengthGood` for promotion gates, and JSON encoding of the zero value defaults to 'weak' — the safest default for any code path that forgets to call PasswordStrength."
  - "PasswordStrength uses unicode.IsPunct OR unicode.IsSymbol for the symbol class. Unicode separates `!` (punct) from `$` (symbol); accepting both matches user intuition that 'special character' covers anything non-alphanumeric. Tested across `_`, `-`, `!`, `1` to ensure all four classes activate as expected."
  - "12-char floor over 14-char floor (the OWASP modern recommendation). UI-SPEC line 548 says 'at least 12 characters'; we follow the spec. Operators who want a 14-char minimum can set their own UI policy in Plan 16 — the API only emits a tier hint, never blocks submission."
  - "Strength tiers do not require ALL four classes for 'strong'. A 16-character passphrase with 3 classes (e.g. 'Hunter2-good-key') hits StrengthStrong because length is the dominant security factor — Levenshtein distance grows exponentially with length, and forcing a 4th class can push users toward dictionary-pattern symbol substitutions which are weaker. Test case 'Hunter2-good-key' asserts this explicitly."
  - "Errors from Verify are typed as %w-wrapped fmt.Errorf rather than sentinel errors. The login handler in Plan 09 does not need to discriminate between 'truncated hash', 'bad salt', and 'version mismatch' — all three become 'invalid credentials' to the user. Any logging at the boundary captures the wrapped chain via slog.Attr."

patterns-established:
  - "Pattern: PHC-format password storage. Plans 09 / 11 / 15 MUST call auth.Hash to produce strings and auth.Verify to check them. No raw argon2.IDKey calls outside this package; no bcrypt; no SHA-anything for passwords. CLAUDE.md 'do not mix bcrypt + argon2' is enforced by absence (no bcrypt import in go.mod)."
  - "Pattern: parameter migration via PHC. Future security hardening that raises m or t MUST update only the constants in argon2id.go; existing hashes continue to verify because Verify reads parameters out of the encoded string. Plan 09 MAY (optionally) re-hash on successful login if the encoded params are weaker than the current constants — this is a Plan 09 / Phase 4 enhancement, not a Plan 07 deliverable."
  - "Pattern: stateless strength evaluation. The evaluator must remain deterministic and side-effect-free. Plan 16 / 11 expose a /api/auth/strength endpoint that reads the password, calls PasswordStrength, and returns the tier — never persists the password, never logs the password, never returns the password back to the client."

requirements-completed:
  - AUTH-01

duration: 3min
completed: 2026-04-28
---

# Phase 01 Plan 07: Argon2id Password Hashing Summary

**Argon2id Hash + Verify with PHC-format encoding (OWASP-2025 params m=19456, t=2, p=1, salt=16, key=32) plus a stateless `PasswordStrength` tier evaluator for the UI strength meter — no bcrypt, no SHA-anything, no plaintext storage path anywhere in the codebase.**

## Performance

- **Duration:** ~3 min
- **Started:** 2026-04-28T00:39:05Z
- **Completed:** 2026-04-28
- **Tasks:** 2 / 2 (TDD: RED → GREEN per task)
- **Commits:** 4 (2 RED + 2 GREEN)
- **Files created:** 3 (argon2id.go, password_strength.go, password_strength_test.go)
- **Files modified:** 3 (go.mod, go.sum, argon2id_test.go)
- **Tests added:** 22 cases across 4 test functions (`go test -v` count)

## Public API

```go
package auth

// Hash produces a PHC-format string parseable by Verify.
//   "$argon2id$v=19$m=19456,t=2,p=1$<b64salt>$<b64hash>"
func Hash(password string) (string, error)

// Verify is constant-time over the derived key.
// Returns (true, nil) on match, (false, nil) on wrong password, (false, err)
// on malformed PHC input. Never panics.
func Verify(password, encoded string) (bool, error)

// Strength tier returned by PasswordStrength. Iota order is part of the
// public contract; downstream code may compare with `>= StrengthGood`.
type Strength int

const (
    StrengthWeak Strength = iota
    StrengthOK
    StrengthGood
    StrengthStrong
)

// String returns the lowercase tier name ("weak"/"ok"/"good"/"strong").
func (s Strength) String() string

// PasswordStrength is a stateless length + character-class evaluator.
// length<12 → weak; 12+/1–2 classes → ok; 12+/3 classes → good; 14+/4 classes → strong; 16+/3+ classes → strong.
func PasswordStrength(password string) Strength
```

## OWASP Parameter Rationale

Source: [OWASP Password Storage Cheat Sheet (2025)](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html).

> Argon2id with a minimum configuration of 19 MiB of memory (m=19456), iteration count of 2 (t=2), and 1 degree of parallelism (p=1).

OWASP lists five equal-security parameter sets ranging from `m=47104, t=1, p=1` (46 MiB, fastest) to `m=7168, t=5, p=1` (7 MiB, slowest). Shifter picks the **m=19456, t=2, p=1** baseline because:

1. **Self-hosted target hardware is modest.** Customers run Shifter on small VMs / on-prem boxes; the 19 MiB cost per login is negligible at single-tenant tens-of-logins-per-day scale, but `m=47104` could cause memory contention on a 1 GB instance during a login burst.
2. **2 iterations is the safest mid-point.** The 1-iteration variant maximizes memory hardness but is more sensitive to memory-side-channel attacks; the 5-iteration variant maximizes time hardness but slows password rotation flows perceptibly.
3. **p=1 is the universal default.** Multi-lane Argon2 only helps when the host has many idle cores during the login; servers running ChirpStack + Postgres + Mosquitto rarely do.

Salt 16 bytes per [OWASP §Salting](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html#salting). Key 32 bytes per Argon2 RFC and the alexedwards/argon2id reference impl.

## How to Upgrade Parameters Later

PHC encoding stores `m`, `t`, `p` inside every hash. To raise the global floor:

1. Edit the constants in `internal/auth/argon2id.go` (e.g. `argonMemKiB = 47104`).
2. New hashes (from change-password, new-user creation) immediately use the stronger parameters.
3. Existing hashes keep working because `Verify` re-parses parameters from the encoded string.
4. (Optional, Plan 09 enhancement) On successful login, compare the stored params to the current constants; if weaker, re-hash with the current constants and update the user's `password_hash` row. This is the canonical "lazy migration" pattern; it imposes zero re-auth burden on users.

A unit-test asserting the prefix (`$argon2id$v=19$m=19456,t=2,p=1$`) MUST be updated in lockstep with constant changes — `TestArgon2id_RoundTrip` is the canonical guard.

## Test-Coverage Matrix

| Test                                | Covers                                                       | Asserts                                              |
| ----------------------------------- | ------------------------------------------------------------ | ---------------------------------------------------- |
| `TestArgon2id_RoundTrip`            | Hash + Verify happy path; salt randomness                    | PHC prefix, Verify==true, two Hash() calls differ    |
| `TestVerify_BadPassword_ConstantTime` | Wrong password rejection                                     | (false, nil) — error is reserved for parse failures  |
| `TestArgon2id_PHCParseError` (×7)   | `""`, garbage, truncated, wrong algo, wrong version, bad m, bad b64 | (false, err) for each; no panic                      |
| `TestArgon2id_VersionMismatch`      | v=20 PHC string                                              | (false, err containing "argon2 version mismatch")   |
| `TestPasswordStrength` (×9)         | empty, 5/11/12/13/14/16/32 char inputs across class counts   | tier exactly matches the heuristic ladder            |
| `TestStrengthString`                | Stringer for each enum value                                 | "weak"/"ok"/"good"/"strong" exactly                  |

22 test cases total, all green under `go test -race -count=1`.

## Threat Surface Notes

All five register entries from `<threat_model>` are mitigated by code shipped in this plan:

| Threat | Mitigation |
|--------|-----------|
| T-07-01 (timing attack on Verify) | `subtle.ConstantTimeCompare` on the 32-byte derived key (grep-verified) |
| T-07-02 (weak Argon2id parameters) | Constants pinned to OWASP-2025 minimums; `Verify` re-parses params so future hardening is non-breaking |
| T-07-03 (salt reuse) | `crypto/rand.Read(salt)` per Hash; `TestArgon2id_RoundTrip` asserts two hashes differ |
| T-07-04 (bcrypt+argon2 mixed) | Only Argon2id imported; `grep -r 'golang.org/x/crypto/bcrypt' internal/` returns no hits |
| T-07-05 (long-password DoS via Argon2 cost) | Documented for Plan 09/11 — login handler MUST reject `len(password) > 256` before calling Hash/Verify; this plan does not add the length cap because it belongs at the API boundary, not the crypto primitive |

No new threat surface beyond the plan's `<threat_model>`.

## Decisions Made

- **Wrapped errors with `argon2id:` prefix.** Plan-verbatim used `errors.New("not argon2id")` and bare `err` returns. Production logging via slog needs the namespace to filter "argon2id parse failure" events from the rest of the auth subsystem; cost is one `fmt.Errorf` per branch.
- **Strength enum starts at 0 = weak.** Defensive default: any zero-valued `Strength` (e.g. forgotten initialization, JSON null) reports "weak" — the safest possible UI state.
- **Strength evaluator does not enforce a 4th class for 'strong'.** Length dominates; a 16-char three-class passphrase (`Hunter2-good-key`) is `StrengthStrong`. This matches modern guidance (NIST 800-63B § 5.1.1.2) that length > arbitrary class diversity for entropy.
- **No `auth.MinPasswordLength` constant exposed.** UI-SPEC line 548 says "at least 12 characters" but that is a UI guidance, not a hard wall. Plan 16 (wizard) and Plan 11 (change password) decide whether to block submission; the strength endpoint only emits a tier.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Wrapped error messages with `argon2id:` namespace**

- **Found during:** Task 1 implementation.
- **Issue:** Plan's verbatim Verify returned bare `errors.New("not argon2id")` and unwrapped `err` from `fmt.Sscanf` / `base64.RawStdEncoding.DecodeString`. Plan 09's login handler will log Verify failures via slog; without a stable prefix, log filtering and error triage become string-matching exercises against unrelated subsystems' errors.
- **Fix:** Every error returned from Verify is now `fmt.Errorf("argon2id: <category>: %w", err)` or `errors.New("argon2id: <category>")`. Test `TestArgon2id_VersionMismatch` asserts the literal substring "argon2 version mismatch" is present (preserves the plan's expected wording while adding the namespace).
- **Files modified:** `internal/auth/argon2id.go`
- **Tested by:** `TestArgon2id_PHCParseError` (each case yields a non-nil error), `TestArgon2id_VersionMismatch` (substring check).
- **Commit:** `86d3fa0`

**2. [Rule 2 - Missing Critical] `Hash` errors also wrap the read-salt failure**

- **Found during:** Task 1 implementation.
- **Issue:** Plan returned bare `err` from `rand.Read(salt)`. While `crypto/rand.Read` errors are essentially impossible on POSIX systems, when one does occur (for example: `/dev/urandom` unreadable inside a misconfigured container), the operator needs to recognize it in the log stream as an argon2id-layer failure — not a generic "EOF reading from somewhere".
- **Fix:** `return "", fmt.Errorf("argon2id: read salt: %w", err)`.
- **Files modified:** `internal/auth/argon2id.go`
- **Tested by:** Indirect (covered by the `TestArgon2id_RoundTrip` happy path; the failure path is hostile to test without a runtime random-source swap).
- **Commit:** `86d3fa0`

---

**Total deviations:** 2 auto-fixed (both Rule 2 missing-critical, both about error namespacing for downstream observability). No public API changes. No constant changes. No behavior changes for happy-path callers.
**Impact on plan:** None — all six acceptance criteria pass, all four test functions pass, and the threat-model entries are unaffected. Deviations strengthen log-time observability for Plan 09's eventual login handler.

## Issues Encountered

- **`golang.org/x/crypto` was already on go.sum (indirect dep)** so `go get golang.org/x/crypto/argon2` only promoted it from indirect to direct in go.mod. The transitive bump (`v0.48.0` → `v0.50.0`) also pulled `golang.org/x/sync v0.20.0`, `golang.org/x/sys v0.43.0`, `golang.org/x/text v0.36.0` and added `golang.org/x/term v0.42.0` — all stdlib-extension packages with stable APIs. No code changes needed elsewhere.
- **No build/lint surprises.** `go vet ./...` clean. `go build ./...` clean. `go test ./internal/auth -race` clean.
- **Test naming.** Plan asked for `TestArgon2id_VersionMismatch` as a separate function; I kept it separate from `TestArgon2id_PHCParseError` even though one of the parse-error cases is a `v=20` string, because the version-mismatch case wants an additional substring assertion (`require.Contains(t, err.Error(), "argon2 version mismatch")`) that doesn't fit the parse-error matrix's uniform `require.Error(t, err)` shape. Two functions, both green.

## Known Stubs

None — Plan 07 fully implements both deliverables. The minimal-stub state Plan 02 left behind (`t.Skip` in `argon2id_test.go`) is replaced; subsequent plans see the production surface.

## User Setup Required

None. `go test ./internal/auth -race -count=1` runs out of the box.

## Next Phase Readiness

- ✅ `auth.Hash(password)` returns a PHC-format string ready to store in `users.password_hash`. Plan 09 / 11 / 15 import directly.
- ✅ `auth.Verify(password, encoded)` returns `(bool, error)` with stable namespaced errors. Plan 09 (login handler) will treat any non-nil error as "invalid credentials" and emit an `slog.Attr` with the wrapped chain for log triage.
- ✅ `auth.PasswordStrength(password)` returns a `Strength` tier. Plan 11 / 16 will expose a `/api/auth/strength` endpoint that delegates here.
- ✅ `Strength.String()` returns the canonical UI tier name; the API can JSON-encode `Strength` directly and the frontend can read either the int (for `>= StrengthGood` gates) or the string.
- ⚠️ **Plan 09 must add a 256-byte password length cap** before calling Verify, per the T-07-05 mitigation note. This is documented in the threat-model section of this SUMMARY but is Plan 09's responsibility to ship.
- ⚠️ **Plan 09 may add lazy re-hash on login** if `parts[3]` of the stored hash indicates parameters weaker than the current constants. This is an enhancement, not a hard requirement for AUTH-01.

## Self-Check: PASSED

Files verified to exist:
- FOUND: `internal/auth/argon2id.go`
- FOUND: `internal/auth/argon2id_test.go`
- FOUND: `internal/auth/password_strength.go`
- FOUND: `internal/auth/password_strength_test.go`

Commits verified to exist:
- FOUND: `810f7f6` (Task 1 RED — failing Argon2id tests)
- FOUND: `86d3fa0` (Task 1 GREEN — Hash/Verify with PHC encoding)
- FOUND: `73f9d71` (Task 2 RED — failing strength evaluator tests)
- FOUND: `79cca29` (Task 2 GREEN — Strength enum + PasswordStrength)

Behavior verified:
- `go build ./...` exits 0
- `go vet ./...` exits 0
- `go test ./internal/auth -run 'TestArgon2id_|TestVerify_BadPassword_ConstantTime|TestPasswordStrength|TestStrengthString' -race -count=1` passes 22 tests
- `grep -n "subtle.ConstantTimeCompare" internal/auth/argon2id.go` returns 2 hits (1 doc + 1 source line) — T-07-01 mitigation grep-verified
- `grep -rn "golang.org/x/crypto/bcrypt" internal/` returns zero hits — CLAUDE.md "do not mix bcrypt + argon2" enforced by absence
- `Hash("hunter2-correct-horse-battery")` produces a string matching `^\$argon2id\$v=19\$m=19456,t=2,p=1\$[A-Za-z0-9+/]+\$[A-Za-z0-9+/]+$`

---
*Phase: 01-foundation*
*Plan: 07-argon2id*
*Completed: 2026-04-28*
