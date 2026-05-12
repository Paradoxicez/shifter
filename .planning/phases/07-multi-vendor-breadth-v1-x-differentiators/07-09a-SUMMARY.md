---
phase: 07-multi-vendor-breadth-v1-x-differentiators
plan: "09a"
subsystem: ingest
tags: [battery-curve, normalize, tdd, ingest, voltage, itron, kinmy]
dependency_graph:
  requires:
    - phase: 07-multi-vendor-breadth-v1-x-differentiators
      plan: "02"
      provides: "migration 0050 with battery_curve CHECK constraint + device_profile.battery_curve column"
    - phase: 07-multi-vendor-breadth-v1-x-differentiators
      plan: "07"
      provides: "RunCodecTest + goja sandbox for Itron golden vector test"
  provides:
    - internal/ingest/battery_curve.go: ApplyBatteryCurve with 4 curves
    - internal/ingest/normalize.go: NormalizeMeasurement extended with batteryCurve param
    - internal/db/sqlc: GetActiveBindingByDevEUI now returns dp.battery_curve
    - internal/resolver/cache.go: Binding.BatteryCurve field
    - internal/profile/codecs/itron_kinmy_lora_test.go: golden vector test no longer skips
  affects:
    - plan 07-09b (profile-aware alert workers consume BatteryCurve from Binding)
tech_stack:
  added: []
  patterns:
    - "TDD RED→GREEN for battery curve registry and normalize extension"
    - "5-breakpoint plateau-then-cliff curve for Li-SOCl2 3.6V (li_socl2_3v6)"
    - "Passthrough sentinels: linear_pct and none return (0, false) — codec owns battery_pct"
    - "extraFloat64 helper handles both float64 and *big.Float from Extra map"
    - "goja exports integer-result JS divisions as int64; toFloat64 helper in test accepts both"
key_files:
  created:
    - internal/ingest/battery_curve.go
    - internal/ingest/battery_curve_test.go
  modified:
    - internal/ingest/normalize.go (batteryCurve param + ApplyBatteryCurve call + extraFloat64 helper)
    - internal/ingest/normalize_test.go (all calls updated + 3 new battery curve test cases)
    - internal/ingest/handler.go (binding.BatteryCurve passed to NormalizeMeasurement)
    - internal/api/codec_test_handler.go (pass "" as batteryCurve — test-codec has no binding)
    - internal/resolver/cache.go (Binding.BatteryCurve string field added)
    - internal/db/queries/bindings.sql (GetActiveBindingByDevEUI returns dp.battery_curve)
    - internal/db/sqlc/bindings.sql.go (sqlc regenerated — BatteryCurve in row struct)
    - internal/cli/serve.go (sqlcResolverLoader populates BatteryCurve from row)
    - internal/testharness/scenarios_test.go (sqlcLoader populates BatteryCurve from row)
    - internal/cli/testharness_helpers_test.go (cliSqlcLoader populates BatteryCurve)
    - internal/profile/codecs/itron_kinmy_lora_test.go (skeleton replaced with golden vector test)
decisions:
  - "batteryCurve passed as third arg to NormalizeMeasurement (not a separate function) — keeps the full ingest path in one call, compiler enforces callers update"
  - "extraFloat64 helper extracts battery_v from Extra map handling both float64 (JSON-decoded) and *big.Float (mapping with data_type=numeric) — Itron mapping routes battery_v to extra.battery_v as numeric"
  - "BatteryCurve added to resolver.Binding — avoids extra DB round-trip per uplink; the resolver already holds all profile metadata needed for the hot path"
  - "codec_test_handler passes '' as batteryCurve — the test-codec endpoint has no binding context, passthrough semantics are correct"
  - "toFloat64 helper in golden vector test accepts int64 — goja exports exact-integer JS results (1000/1000=1) as int64, fractional results (100/1000=0.1) as float64"
metrics:
  duration: 9min
  completed_date: "2026-05-13"
  tasks_completed: 1
  files_changed: 11
requirements_completed: [ALERT-04, V2-VEND-03]
---

# Phase 7 Plan 09a: Battery Curve and Normalize Summary

**One-liner:** Battery curve registry (li_socl2_3v6 5-breakpoint + li_mnox_3v0/alkaline_3v0 stubs) wired into NormalizeMeasurement via a new batteryCurve param; Itron+KINMY golden vector test filled in and green.

## What Was Built

### Task 1: Battery curve registry + NormalizeMeasurement extension (TDD)

**RED commit:** `ec97df9` — failing tests for ApplyBatteryCurve, NormalizeMeasurement extension, and Itron golden vector

**GREEN commit:** `fae97db` — all implementations

#### battery_curve.go

- `BatteryCurveType = string` type alias with 5 named constants matching migration 0050 CHECK constraint
- `ApplyBatteryCurve(curve, voltageV) (int16, bool)` — returns `(0, false)` for `linear_pct`/`none`/`""` (passthrough), `(pct, true)` for voltage-based curves
- `liSOCl23V6`: 5-breakpoint plateau-then-cliff — 3.6V=100%, 3.4V=85%, 3.2V=50%, 3.0V=20%, 2.8V=0% with linear interpolation between bands
- `liMnO23V0`: stub linear 3.0V=100% → 2.4V=0%
- `alkaline3V0`: stub linear 3.0V=100% → 2.0V=0%
- Out-of-range inputs (negative voltage, extreme high) handled defensively — never panics (T-07-09a-02 mitigation)

#### normalize.go changes

- `NormalizeMeasurement` signature extended to `(decoded, mappings, batteryCurve string)`
- After mapping pass: `extraFloat64(out.Extra, "battery_v")` extracts voltage from Extra map (handles both `float64` from JSON decode and `*big.Float` from numeric-typed mapping)
- `ApplyBatteryCurve(batteryCurve, v)` called; when `ok=true`, `Layer1.BatteryPct` set to curve output
- `extraFloat64` helper added (package-private)

#### Binding and SQL changes

- `resolver.Binding.BatteryCurve string` field added
- `GetActiveBindingByDevEUI` SQL extended with `dp.battery_curve` column
- sqlc regenerated: `GetActiveBindingByDevEUIRow.BatteryCurve string`
- `sqlcResolverLoader.LoadActive` in serve.go, `sqlcLoader` in testharness, `cliSqlcLoader` in CLI tests all populate `BatteryCurve` from row

#### Itron golden vector test

- `TestItronKinmyLoRa_GoldenVector` skeleton replaced with full 28-byte test
- Constructed payload: forward_flow=1000L→1.0m³, reverse_flow=100L→0.1m³, battery=0x24→3.6V
- Asserts: `forward_flow_m3 ≈ 1.0`, `reverse_flow_m3 ≈ 0.1`, `battery_v ≈ 3.6`, `meter_id` non-empty
- Asserts: `ApplyBatteryCurve("li_socl2_3v6", 3.6) = (100, true)`

## Task Commits

1. **Task 1 RED** — `ec97df9` (test)
2. **Task 1 GREEN** — `fae97db` (feat)

## Test Results

- `go test ./internal/ingest/... -count=1`: 61 passed (all battery curve + normalize tests + existing suite)
- `go test ./internal/profile/codecs/... -count=1`: 1 passed (TestItronKinmyLoRa_GoldenVector)
- `go test ./... -count=1 -short`: 542 passed across 42 packages — no regressions
- `go build ./...`: exits 0

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] goja exports integer-valued JS results as int64, not float64**
- **Found during:** Task 1 first GREEN attempt — `TestItronKinmyLoRa_GoldenVector` failed at line 77 "forward_flow_m3 must be float64"
- **Issue:** The Itron codec computes `1000/1000 = 1` which JS treats as an integer. goja's `Export()` returns Go `int64` for integer-valued numbers, not `float64`. The plan's test sketch used a direct `.(float64)` type assertion which fails for `int64`.
- **Fix:** Added `toFloat64(v any) (float64, bool)` helper in the test that accepts `float64`, `int64`, `int`, and `float32`. Used it for all numeric assertions in the golden vector test. The important invariant is the *value*, not the Go type.
- **Files modified:** `internal/profile/codecs/itron_kinmy_lora_test.go`
- **Commit:** `fae97db`

## Known Stubs

| File | Line | Stub | Resolving Plan |
|------|------|------|----------------|
| `internal/ingest/battery_curve.go` | liMnO23V0 | Stub linear 3.0→2.4V; comment says "refine in v1.1 with real datasheet breakpoints" | v1.1 (T-07-09a-01 accepted disposition) |
| `internal/ingest/battery_curve.go` | alkaline3V0 | Stub linear 3.0→2.0V; comment says "refine in v1.1" | v1.1 (T-07-09a-01 accepted disposition) |

These stubs are intentional — the plan specifies "ship reasonable approximations (stubbed for now: linear from nominal to 0)" for li_mnox_3v0 and alkaline_3v0. The only curve exercised by current hardware (Itron+KINMY) is li_socl2_3v6 which is fully implemented.

## Threat Surface Scan

No new network endpoints, auth paths, or file access patterns introduced. The battery curve logic runs entirely within the existing ingest pipeline trust boundary. T-07-09a-02 (out-of-range voltage crashes) is mitigated: the `default: return 0, false` in the switch and clamping in each curve function handle negative/extreme voltages cleanly.

## Self-Check: PASSED
