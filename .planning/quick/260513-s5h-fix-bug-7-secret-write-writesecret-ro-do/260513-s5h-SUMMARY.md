---
id: 260513-s5h
type: quick-fix
title: Make writeSecret idempotent under RO docker secret mounts
date: 2026-05-13
commit: e07f4ae
subsystem: install-wizard
tags: [bug-fix, docker-secrets, idempotency, install]
key-files:
  modified:
    - internal/install/handlers.go
  created:
    - internal/install/writesecret_test.go
decisions:
  - Skip-if-matches over try-write-catch: avoids triggering permission errors on RO mounts; semantically correct (same value = already done)
  - Error on content mismatch rather than silent override: operator must explicitly update the host secret file; avoids silent data loss on RO mounts
  - TrimSpace only: no other validation per scope constraint
metrics:
  duration: ~20min
  tasks: 1
  files: 2
---

# Quick Fix 260513-s5h: Make writeSecret idempotent under RO docker secret mounts

**One-liner:** Skip-if-matches idempotency in `writeSecret` so bundled-compose RO docker secret mounts no longer cause wizard step 2 to return 500.

## Problem

`writeSecret` called `os.WriteFile` unconditionally. In bundled compose, `/run/secrets` is a READ-ONLY docker secrets mount pre-populated by `bootstrap-chirpstack-token.sh`. The wizard's step 2 handler received the same token value from the operator but tried to overwrite the RO file — `permission denied` → 500 `secret_write`.

## Fix Applied

Modified `writeSecret(dir, name, value string) (string, error)` in `internal/install/handlers.go`:

1. `strings.TrimSpace` the operator value first (paste/newline artifacts)
2. `os.ReadFile(path)` — if file exists:
   - `strings.TrimSpace(existing) == value` → return path unchanged (skip write)
   - content differs → return descriptive error ("different content (RO docker mount?); update the host secret file and retry")
3. File absent → `os.MkdirAll` + `os.WriteFile` as before (dev and external-mode paths)

## Tests

New file `internal/install/writesecret_test.go` with `TestWriteSecret`:
- **Case 1 (absent):** write succeeds, content matches, path returned
- **Case 2 (match, trim-aware):** existing `"  abc  \n"` vs value `"abc"` → skip, mtime unchanged
- **Case 3 (mismatch):** error containing "different content"

`go test ./internal/install/... -count=1` → **38 passed**

## Live Wizard Rehearsal (bundled compose)

After `DELETE FROM install_state; DELETE FROM "user";`:

| Step | Endpoint | HTTP Code |
|------|----------|-----------|
| 1 | POST /api/install/step/1 | **200** |
| 2 | POST /api/install/step/2 | **200** (was 500) |
| 3 | POST /api/install/step/3 | **200** |
| 4 | POST /api/install/step/4 | **200** |
| finish | POST /api/install/finish | **200** |
| login | POST /api/auth/login | **200** + Set-Cookie |

## Deviations

None — fix executed exactly as designed in inline plan.

## Self-Check: PASSED

- `internal/install/handlers.go` — modified writeSecret function confirmed present
- `internal/install/writesecret_test.go` — test file confirmed created
- Commit `e07f4ae` exists and contains both files
- All 38 install package tests pass
- Live wizard end-to-end completes successfully
