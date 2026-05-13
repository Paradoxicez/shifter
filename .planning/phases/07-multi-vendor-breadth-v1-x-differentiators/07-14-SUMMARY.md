---
phase: "07"
plan: "14"
subsystem: doctor-probes-phase-closure
tags: [doctor, probes, chirpstack, health, playwright, e2e, documentation]
dependency_graph:
  requires: [07-05, 07-06, 07-08, 07-10, 07-11a, 07-11b, 07-12, 07-13]
  provides: [doctor-probes, health-probe-results, phase-7-e2e-specs, phase-7-docs]
  affects: [internal/doctor, internal/cli, internal/http, web/playwright]
tech_stack:
  added: []
  patterns:
    - gRPC in-process fake server for probe testing
    - Sentinel API key test pattern for secret-leak prevention (B-3)
    - Playwright graceful skip via test.info().annotations
key_files:
  created:
    - internal/doctor/probes.go
    - internal/doctor/probes_test.go
    - internal/install/probe/chirpstack_test.go
    - .planning/RETROSPECTIVE.md
  modified:
    - internal/doctor/doctor.go
    - internal/cli/doctor.go
    - internal/http/health.go
    - internal/http/health_test.go
    - web/playwright/specs/phase-07-vendor-catalog.spec.ts
    - .planning/REQUIREMENTS.md
    - .planning/phases/07-multi-vendor-breadth-v1-x-differentiators/07-VALIDATION.md
key_decisions:
  - ProbeChirpStack never leaks API key — errorClass(err) returns gRPC status code string only; safeHostFromGRPCURL strips credentials from URL (T-07-14-03 mitigated)
  - HealthDetailed delegates to HealthDetailedWithCS("", "") for backward compatibility with existing tests and router wiring
  - ROADMAP SC#5 connect-time ChirpStack refusal scoped to manual shifter doctor invocation per D-12/D-13 — not a connect-time hook
metrics:
  duration_minutes: 45
  tasks_completed: 4
  tasks_total: 5
  completed_date: "2026-05-13"
  checkpoint_at: "Task 5 (human-verify)"
---

# Phase 7 Plan 14: Doctor Probes + Phase Closure Summary

**One-liner:** Install-validation probes (ProbeChirpStack/Timescale/Region) with API-key-safe error paths, Cobra subcommands, /health/detailed probe_results block, Phase 7 Playwright E2E specs filled in, and REQUIREMENTS/VALIDATION/RETROSPECTIVE reconciled.

## Tasks Completed

| Task | Name | Commit | Key Files |
|------|------|--------|-----------|
| 1 | Implement ProbeChirpStack, ProbeTimescale, ProbeRegion | 0bc6af1 | `internal/doctor/probes.go`, `internal/doctor/probes_test.go`, `internal/install/probe/chirpstack_test.go` |
| 2 | Cobra subcommands + /health/detailed probe_results | fdf6d45 | `internal/cli/doctor.go`, `internal/http/health.go`, `internal/http/health_test.go` |
| 3 | Fill in Phase 7 Playwright E2E specs | e537c88 | `web/playwright/specs/phase-07-vendor-catalog.spec.ts` |
| 4 | Reconcile REQUIREMENTS, VALIDATION, RETROSPECTIVE | 3bfc4e0 | `.planning/REQUIREMENTS.md`, `07-VALIDATION.md`, `.planning/RETROSPECTIVE.md` |
| 5 | Final Phase 7 smoke test (checkpoint:human-verify) | — | Awaiting operator sign-off |

## What Was Built

### 1. Doctor Probes (`internal/doctor/probes.go`)

Three install-validation probe functions:

- **`ProbeChirpStack(ctx, grpcURL, apiKey)`** — dials ChirpStack gRPC, calls `GetVersion`, classifies: status=ok (v4.10+), warn (v4 but <4.10), error (v3 or unreachable). Never includes the API key in any error message — `errorClass(err)` returns the gRPC status code string only.
- **`ProbeTimescale(ctx, pool)`** — queries `pg_extension` for timescaledb; status=error if absent, ok with version if present.
- **`ProbeRegion(ctx, pool)`** — reads `chirpstack_connection.region_name` vs distinct `gateway.region` values (`archived_at IS NULL` filter); status=warn if mismatch, ok if all consistent.

Security: T-07-14-03 mitigated by `safeHostFromGRPCURL()` (strips embedded credentials from URL) and `errorClass(err)` (never calls `err.Error()` directly). Verified by `TestProbeChirpStack_NoAPIKeyInErrorMessage` using sentinel `SENTINEL_API_KEY_8f2c93` against an unreachable host.

### 2. Cobra Subcommands (`internal/cli/doctor.go`)

Three subcommands added to the `doctor` parent:
- `shifter doctor probe-chirpstack` — exit 0 on ok/warn, exit 1 on error
- `shifter doctor probe-timescale` — exit 0 on ok, exit 1 on error
- `shifter doctor probe-region` — exit 0 on ok/warn, exit 1 on error

Human-readable output: `[ok] ✓: ChirpStack v4.12.3 — ok (4.10+ required)`

### 3. `/health/detailed` Extension (`internal/http/health.go`)

`HealthDetailed(pool)` now delegates to `HealthDetailedWithCS(pool, "", "")`. The full constructor accepts gRPC URL + API key and runs all three probes with 5s timeouts each (15s worst-case — T-07-14-02 accepted: admin-only, low traffic). JSON response includes:

```json
{
  "probe_results": {
    "chirpstack": {"name": "chirpstack", "status": "warn", "message": "...", "last_run_at": "..."},
    "timescale":  {"name": "timescale",  "status": "ok",   "message": "...", "last_run_at": "..."},
    "region":     {"name": "region",     "status": "ok",   "message": "...", "last_run_at": "..."}
  }
}
```

### 4. Phase 7 Playwright E2E Specs

Three real `test()` cases replace the original `test.skip()` stubs:
1. **Import from catalog** — login → `/profiles` → "Import from catalog" button → 3-step dialog (RadioGroup → Command picker with "Itron" search → Review form with KINMY pre-fill) → "Add Profile" → assert toast "Profile added"
2. **Apply catalog update** — login → `/settings` → find "Update to v..." button → diff modal (Use mine / Use catalog per-field toggles) → Apply Update → assert toast "Profile updated to v..."
3. **Run codec test** — login → `/profiles` → click vendor profile → Test Codec section → fill hex → Run Test → Decoded JSON tab → Canonical Mapping tab

Graceful skip via `test.info().annotations` when pre-conditions not met (no update-available row, no vendor profiles seeded).

### 5. Documentation Reconciliation

- **REQUIREMENTS.md**: V2-VEND-01/02/03 marked Complete with `[x]`; traceability table extended with Phase 7 rows; Phase 7 evidence trail appended with file paths + test names for each requirement bucket.
- **07-VALIDATION.md**: Per-task verification map filled for all 37 tasks across plans 07-01 through 07-14; `nyquist_compliant: true` set; wave 0 checklist marked complete.
- **RETROSPECTIVE.md**: Phase 7 entry created (first entry in the file); includes explicit SC#5 scope-deviation paragraph per checker I-4.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] `decommissioned_at` column does not exist on gateway table**
- **Found during:** Task 1 (ProbeRegion implementation)
- **Issue:** Initial query used `WHERE decommissioned_at IS NULL` but the gateway model uses `archived_at`
- **Fix:** Changed query filter to `archived_at IS NULL`
- **Files modified:** `internal/doctor/probes.go`
- **Commit:** 0bc6af1

**2. [Rule 1 - Bug] Unused import `credentials/insecure` in probes_test.go**
- **Found during:** Task 1 (go test run)
- **Issue:** `"google.golang.org/grpc/credentials/insecure"` imported but not used
- **Fix:** Removed unused import
- **Files modified:** `internal/doctor/probes_test.go`
- **Commit:** 0bc6af1

**3. [Rule 1 - Bug] Stray `appendAPIKeyToCtx` reference in probes.go first draft**
- **Found during:** Task 1 (compilation error)
- **Issue:** Function removed during refactor but reference remained
- **Fix:** Full rewrite of probes.go removing the stale reference; replaced with direct `metadata.AppendToOutgoingContext` call
- **Files modified:** `internal/doctor/probes.go`
- **Commit:** 0bc6af1

## Known Stubs

None — all Phase 7 plan objectives delivered. The Playwright E2E specs use a graceful skip pattern (not stubs) for tests requiring seeded data state, which is the correct pattern for environment-dependent E2E tests.

## Threat Flags

No new threat surface introduced beyond what the plan's `<threat_model>` already covers (T-07-14-01 through T-07-14-05).

## Self-Check: PENDING

Task 5 (checkpoint:human-verify) not yet approved. Self-check will be completed after operator sign-off.

Files created:
- `internal/doctor/probes.go` — exists (committed 0bc6af1)
- `internal/install/probe/chirpstack_test.go` — exists (committed 0bc6af1)
- `.planning/RETROSPECTIVE.md` — exists (committed 3bfc4e0)

Key commits:
- 0bc6af1 — ProbeChirpStack, ProbeTimescale, ProbeRegion + tests
- fdf6d45 — Cobra subcommands + /health/detailed probe_results
- e537c88 — Phase 7 Playwright E2E specs
- 3bfc4e0 — REQUIREMENTS + VALIDATION + RETROSPECTIVE reconciliation
