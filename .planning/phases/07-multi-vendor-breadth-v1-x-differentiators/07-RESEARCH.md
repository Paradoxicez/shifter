# Phase 7: Multi-Vendor Breadth & v1.x Differentiators - Research

**Researched:** 2026-05-12
**Domain:** Vendor catalog architecture, goja JS runtime, CAGG backtest queries, profile-aware alert refactor, saved templates, bulk gateway import, install validation hardening
**Confidence:** HIGH (codebase-verified for all substrate questions; MEDIUM for goja API surface; LOW for Li-SOCl2 curve precision)

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**A. Profile catalog (V2-VEND-01)**
- D-01: Catalog = Go `embed` in the binary at `internal/codec/catalog/*.json` via `//go:embed`
- D-02: Per-profile semver + Settings → Vendor Catalog "Update available" flow; operator-controlled, not auto-update
- D-03: "Add from catalog" button inside device-profile dialog (two start states: blank vs import)
- D-04: Profile editable after import; catalog rows are seeds, not links; customer edits survive upgrades

**B. Codec test-runner (V2-VEND-02)**
- D-05: Test-runner inside device-profile editor (not standalone page)
- D-06: Input = hex bytes only (fPort + payload hex)
- D-07: Output = JSON tree (left) + canonical mapping (right) in split panel
- D-08: No save/no history/no test-case library — scratch pad only
- D-26: Test-runner JS execution = local `goja` (github.com/dop251/goja) in-process — NOT round-trip to ChirpStack
- D-27: Test-runner works even when `codec_js_synced_at IS NULL`
- D-28: Test-runner failure UX = inline error panel with line/col from goja stack trace

**C. Anomaly threshold tuning (ALERT-04)**
- D-09: Ship better defaults in code; no operator-facing tuning UI
- D-10: "Test against last 30 days" backtest button on rule-enable surface
- D-11: Cold-start visibility unchanged from Phase 6 D-16
- D-35: Backtest result = single count + 30-bar daily sparkline
- D-41: Catalog adds `expected_uplink_interval_seconds`; ALERT-03 + ALERT-04 refactored to profile-aware
- D-42: Catalog adds `anomaly_compatibility` enum (`full | limited | unsupported`); extended warmup for `limited`
- D-43: Catalog adds `offline_threshold_multiplier`; ALERT-03 threshold = `expected_uplink_interval_seconds × offline_threshold_multiplier`
- D-44: Catalog adds `battery_curve` enum; `normalize.go` registry applies non-linear curve for voltage-reporting vendors

**D. Install validation hardening**
- D-12: Probes as `shifter doctor` subcommands (`probe-chirpstack`, `probe-timescale`, `probe-region`); no periodic cron, no binary-start probe
- D-13: Probe failure = warn (banner + `/health/detailed` row); never blocks `shifter serve`
- D-40: Probe results = last-run only in `/health/detailed` (no history array)

**E. V2-VEND-03 (saved templates + comparison + bulk gateway import)**
- D-14: Saved templates capture full ReportConfigPanel state (scope + range + sites/meters filter + group-by + capability filter)
- D-15: Templates install-wide shared (not per-user)
- D-16: Side-by-side comparison = exactly 2 entities
- D-17: Bulk gateway import reuses Phase 3 CSV pattern fully (dry-run → confirm → commit; idempotent on `gateway_eui`)
- D-37: Comparison entity picker = two searchable dropdowns at top of compare view
- D-38: YoY mode toggle: "Compare entities" vs "Compare time ranges"
- D-39: Template listing = alphabetical + search box

**F. Vendor mapping expansion**
- D-18 REVISED: Ship 4 catalog entries / 3 codec files: `axioma_w1`, `acrel_adl200`, `acrel_adw300` (shared `acrel_family.js`), `itron_kinmy_lora`
- D-19 CORRECTED: Codec source = JS files via `//go:embed`, pushed to ChirpStack v4 QuickJS. NOT Go decoders.
- D-20: New-vendor onboarding = PR workflow (codec.js + catalog.json + fixtures) → binary release

**G. Catalog file format + validation**
- D-21: Catalog format = JSON at `internal/codec/catalog/*.json`; decoded via stdlib `encoding/json`
- D-22: Schema validation via `go test` at build time (`TestCatalogValid` in `internal/codec/catalog_test.go`)
- D-33: Catalog JSON schema fields locked: `slug`, `name`, `vendor`, `family`, `capabilities`, `version`, `codec_js_path`, `counter_modulus`, `mac_version`, `region`, `expected_uplink_interval_seconds`, `offline_threshold_multiplier`, `anomaly_compatibility`, `battery_curve`, `vendor_has_separate_meter_serial`

**H. Settings → Vendor Catalog tab UI**
- D-23: Catalog tab = table layout; columns: Vendor | Family | Capability | Version | Installed | Devices using | Status; sortable; chip filter by capability
- D-24: Update flow = per-row badge + Update button → confirm dialog with diff modal (D-34)
- D-25: Show "Devices using" count per profile row; click navigates to Devices pre-filtered
- D-34: Diff modal = field-by-field rows with side-by-side values + per-field toggle + "you edited this" flag
- D-36: Codec re-sync trigger on Update = auto-clear `codec_js_synced_at = NULL`; Phase 2 seed routine handles the re-push

**I. Existing-seed reconciliation**
- D-29: Backfill `catalog_source` + `catalog_source_version` + `customer_edited` on existing 3 seed rows
- D-30: Backfill version = `1.0.0`; no "update available" badge on day 1
- D-31: Drift detection on migration: if existing `codec_js` ≠ embedded JS hash → `customer_edited = true`
- D-32: Shared-codec representation: two catalog entries may reference same `codec_js_path`

**J. Itron+KINMY specifics**
- D-45: `vendor_has_separate_meter_serial: bool` flag; `meter_id` stays in `measurement.raw` JSONB only
- D-46: `reverse_flow_m3` → `raw` JSONB; new delta-based alert rule `reverse_flow_increase`
- D-47: `meter_date` + `meter_time` → `raw` JSONB only (not canonical timestamp)
- D-48: Itron+KINMY catalog entry fully specified (slug: `itron_kinmy_lora`, v1.0.0, all metadata locked)

### Claude's Discretion

- Migration ordering — single 0050 or split (columns vs backfill)
- `report_template` table schema (column list); template state serialization format (JSON blob vs structured columns)
- Exact placement of "Test against last 30 days" backtest button
- goja sandbox limits — default 100ms timeout + 16MB stack unless research shows different
- Catalog `TestCatalogValid` test layout — table-driven vs per-file subtests
- Mobile layout of compare view (D-37/D-38)
- Naming of `reverse_flow_increase` alert rule kind constant

### Deferred Ideas (OUT OF SCOPE)

- Additional vendor catalog entries beyond 4 (Kamstrup, Diehl, Sagemcom, Schneider) — v1.1+ PR workflow
- JS sandbox codec runtime in DB + admin-UI "Add custom vendor" — V2
- Hot catalog reload without binary restart — V2
- Periodic install-probe via River cron — V2
- Anomaly tuning UI — V2
- Per-MP override of offline/anomaly thresholds — Phase 8
- Promote `meter_id` to canonical column — Phase 8/v1.1
- Clock-drift diagnostic alert — Phase 8
- "Why is this MP not firing" debugger surface — V2
- Dashboard "Anomaly coverage" tile — V2
- Compare 3+ entities — V2
- Per-user saved templates — V2
- MRU sort / categorization for templates — Phase 8
- Catalog "Update available" cross-page banner — Phase 8
- Probe results 7-day history — Phase 8
- CSV-with-map-step for bulk gateway import — deferred
- Save test-runner test cases as profile fixtures — V2
- Diff against expected JSON in test-runner — V2
</user_constraints>

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| V2-VEND-01 | Pre-seeded vendor profile catalog ships as versioned data file | D-01..D-04, D-18..D-22, D-29..D-33, D-48; catalog embed pattern verified in codebase |
| V2-VEND-02 | Codec test-runner UI inside profile editor | D-05..D-08, D-26..D-28; goja API patterns documented below |
| V2-VEND-03 | Saved report templates + side-by-side comparison + bulk gateway import | D-14..D-17, D-37..D-39; Phase 3 import pattern reusable; ReportConfigPanel state verified |
| ALERT-04 | Anomaly thresholds tuned against real-customer data; cold-start gate observable | D-09..D-11, D-35, D-41..D-44; backtest SQL pattern documented below; cold-start already ships |
</phase_requirements>

---

## Summary

Phase 7 is an extension phase, not a greenfield phase. All execution substrate (codec embed, seed sync, anomaly workers, bulk import, doctor CLI, TimescaleDB CAGGs) already exists in the codebase and is verified. The primary work is:

1. **Catalog architecture** — introduce `internal/codec/catalog/*.json` embed, `DeviceProfile` struct aligned to D-33 schema, `TestCatalogValid` build-time validation, Settings → Vendor Catalog tab UI, and migration 0050 that ALTERs `device_profile` to add 4 new columns plus backfill
2. **goja test-runner** — add `github.com/dop251/goja` dependency, create `POST /api/profiles/{id}/test-codec` handler that executes `decodeUplink({bytes, fPort})` in a sandboxed goja runtime, return decoded JSON + canonical mapping derived from existing `NormalizeMeasurement` pipeline
3. **Profile-aware ALERT-03/04 refactor** — read `expected_uplink_interval_seconds × offline_threshold_multiplier` from `device_profile` join in `offline_worker.go`; extend cold-start gate for `anomaly_compatibility: limited` profiles (60-day warmup); add `battery_curve` registry in `normalize.go`; add `reverse_flow_increase` rule evaluator
4. **V2-VEND-03 features** — `report_template` table + CRUD, compare endpoint, bulk gateway import using Phase 3 pattern
5. **Doctor probes** — `probe-chirpstack` / `probe-timescale` / `probe-region` Cobra subcommands extending existing `internal/doctor/doctor.go`

**Critical constraint:** goja is not yet in `go.mod`. Adding it is a new dependency (Wave 0 task). All three existing codec JS files follow ES5 convention which goja supports fully. [VERIFIED: codebase grep]

**Primary recommendation:** Plan 07 as ~10 plans in ~3 waves. Wave 0 adds goja + catalog embed skeleton + migration 0050 + test scaffold. Waves 1-2 build out the five work streams above. The Phase 3 import pattern (parser/dryrun/commit) is the only complex flow to reuse; all other substrate is straightforward extension.

---

## Standard Stack

### Core (already in `go.mod` — verified)

| Library | Version | Purpose | Status |
|---------|---------|---------|--------|
| `github.com/jackc/pgx/v5` | v5.9.2 | Postgres driver | In `go.mod` |
| `github.com/riverqueue/river` | v0.36.0 | Background jobs (PDF, existing) | In `go.mod` |
| `golang-migrate/migrate/v4` | v4.19.1 | SQL migrations | In `go.mod` |
| `github.com/go-chi/chi/v5` | v5.2.5 | HTTP router | In `go.mod` |
| `github.com/spf13/cobra` | v1.10.2 | CLI subcommands | In `go.mod` |
| `log/slog` | stdlib | Structured logging | stdlib |
| `encoding/json` | stdlib | Catalog JSON decode | stdlib |
| `embed` | stdlib | Embed catalog JSON + codec JS | stdlib |

[VERIFIED: /Users/suraboonsung/Documents/Programming/shifter/go.mod]

### New Dependency

| Library | Version | Purpose | Why |
|---------|---------|---------|-----|
| `github.com/dop251/goja` | v0.0.0-20260311135729 (pseudo-version) | JS execution for codec test-runner | D-26: same ES semantics as ChirpStack QuickJS; pure Go, no CGO |

[VERIFIED: pkg.go.dev — library exists, published Mar 2026. Version is pseudo-version (no formal releases); use `@latest` in `go get`.]
[ASSUMED: exact pseudo-version at time of `go get github.com/dop251/goja@latest` will differ from above. Run `go get` to pin.]

**Installation:**
```bash
cd /Users/suraboonsung/Documents/Programming/shifter
go get github.com/dop251/goja@latest
```

### Frontend (already in `package.json` — no new deps expected)

| Library | Purpose | Applies to |
|---------|---------|----------|
| `@tanstack/react-query` | Server state | Catalog list, template CRUD, compare |
| `react-hook-form` + `zod` | Forms | Template save dialog, diff modal field-toggle |
| `shadcn/ui` DataTable + Command | Catalog table + entity picker | D-23, D-37 |
| `sonner` | Toasts | Catalog import/update feedback |
| `recharts` | 30-bar sparkline (D-35 backtest) | Minimal bar chart via shadcn chart block |

[VERIFIED: /Users/suraboonsung/Documents/Programming/shifter/web/src/routes/ — existing usage confirmed]

---

## Architecture Patterns

### Recommended Project Structure for Phase 7

```
internal/
├── codec/
│   ├── catalog/           # NEW: embedded JSON catalog entries
│   │   ├── axioma_w1.json
│   │   ├── acrel_adl200.json
│   │   ├── acrel_adw300.json
│   │   └── itron_kinmy_lora.json
│   ├── catalog.go         # NEW: embed.FS + CatalogEntry struct + Load/List/Get functions
│   └── catalog_test.go    # NEW: TestCatalogValid (D-22 build-time schema check)
├── codec_runner/          # NEW: goja test-runner (separate from profile package)
│   ├── runner.go          # RunCodecTest(codecJS, hexPayload, fPort) → Result
│   └── runner_test.go
├── profile/
│   ├── codecs/            # Existing — JS source files; add catalog embed here or in codec/
│   └── ...
├── ingest/
│   ├── normalize.go       # MODIFY: add battery_curve registry + ApplyBatteryCurve()
│   └── ...
├── alert/
│   ├── offline_worker.go  # MODIFY: profile-aware thresholds (D-41/D-43)
│   ├── anomaly_worker.go  # MODIFY: anomaly_compatibility gate (D-42)
│   ├── cold_start.go      # MODIFY: extended warmup for `limited` profiles
│   └── reverse_flow.go    # NEW: ReverseFlowIncreaseWorker (D-46)
├── report/
│   └── template_store.go  # NEW: report_template CRUD
├── gateway/
│   └── import.go          # NEW: bulk gateway import (wraps internal/import pattern)
└── doctor/
    ├── doctor.go          # MODIFY: add ProbeChirpStack / ProbeTimescale / ProbeRegion
    └── probes.go          # NEW: three probe functions
```

[VERIFIED: Existing package locations confirmed by codebase scan]

### Pattern 1: Catalog JSON Embed

```go
// internal/codec/catalog.go
// Source: Phase 2 embed.go convention at internal/profile/codecs/embed.go

package codec

import (
    "embed"
    "encoding/json"
    "fmt"
)

//go:embed catalog/*.json
var catalogFS embed.FS

// CatalogEntry is the locked D-33 schema.
type CatalogEntry struct {
    Slug                          string   `json:"slug"`
    Name                          string   `json:"name"`
    Vendor                        string   `json:"vendor"`
    Family                        string   `json:"family"`
    Capabilities                  []string `json:"capabilities"`
    Version                       string   `json:"version"`
    CodecJSPath                   string   `json:"codec_js_path"`
    CounterModulus                int64    `json:"counter_modulus"`
    MACVersion                    string   `json:"mac_version"`
    Region                        *string  `json:"region"`
    ExpectedUplinkIntervalSeconds int      `json:"expected_uplink_interval_seconds"`
    OfflineThresholdMultiplier    float64  `json:"offline_threshold_multiplier"`
    AnomalyCompatibility          string   `json:"anomaly_compatibility"` // full|limited|unsupported
    BatteryCurve                  string   `json:"battery_curve"`         // linear_pct|li_socl2_3v6|...
    VendorHasSeparateMeterSerial  bool     `json:"vendor_has_separate_meter_serial"`
}

// LoadAll returns all embedded catalog entries.
func LoadAll() ([]CatalogEntry, error) {
    entries, err := catalogFS.ReadDir("catalog")
    // ... unmarshal each file
}
```

[VERIFIED: `//go:embed` pattern confirmed from `internal/profile/codecs/embed.go`]

### Pattern 2: goja Codec Test-Runner

```go
// internal/codec_runner/runner.go
// Source: pkg.go.dev/github.com/dop251/goja — verified API

package codec_runner

import (
    "fmt"
    "time"

    "github.com/dop251/goja"
)

// TestResult is returned from RunCodecTest.
type TestResult struct {
    DecodedJSON    map[string]any `json:"decoded_json"`
    CanonicalMapping any          `json:"canonical_mapping"`
    ErrorMessage   string         `json:"error_message,omitempty"`
    ErrorLine      int            `json:"error_line,omitempty"`
    ErrorCol       int            `json:"error_col,omitempty"`
    ErrorStack     string         `json:"error_stack,omitempty"`
}

// RunCodecTest executes decodeUplink({bytes, fPort}) inside a goja runtime.
// Sandboxed: 100ms wall-clock timeout, max call stack 500 frames.
// Returns TestResult — never returns a Go error (errors are surfaced in TestResult).
func RunCodecTest(codecJS string, hexBytes []byte, fPort int) TestResult {
    vm := goja.New()
    vm.SetMaxCallStackSize(500)

    // 100ms hard timeout via interrupt (D-26 + CONTEXT Claude's Discretion default)
    timer := time.AfterFunc(100*time.Millisecond, func() {
        vm.Interrupt("codec timeout exceeded 100ms")
    })
    defer timer.Stop()

    // Compile + run the codec source.
    if _, err := vm.RunString(codecJS); err != nil {
        return gojaErrToResult(err)
    }

    // Get the decodeUplink function.
    fn, ok := goja.AssertFunction(vm.Get("decodeUplink"))
    if !ok {
        return TestResult{ErrorMessage: "decodeUplink is not a function"}
    }

    // Build input object: {bytes: [...], fPort: N}
    inputObj := vm.NewObject()
    bytesArr := vm.NewArray(len(hexBytes))
    for i, b := range hexBytes { bytesArr.Set(fmt.Sprintf("%d", i), int(b)) }
    _ = inputObj.Set("bytes", bytesArr)
    _ = inputObj.Set("fPort", fPort)

    // Call decodeUplink(input)
    result, err := fn(goja.Undefined(), inputObj)
    if err != nil {
        return gojaErrToResult(err)
    }

    // Export the result.data, result.errors
    exported := result.Export()
    // ... extract data + errors fields, build TestResult
    return TestResult{DecodedJSON: /* data */}
}

// gojaErrToResult converts a goja error to a TestResult with line/col info.
func gojaErrToResult(err error) TestResult {
    if ex, ok := err.(*goja.Exception); ok {
        // D-28: extract line/col from stack trace
        stack := ex.Error() // includes source location
        return TestResult{
            ErrorMessage: ex.Value().String(),
            ErrorStack:   stack,
        }
    }
    if iv, ok := err.(*goja.InterruptedError); ok {
        return TestResult{ErrorMessage: fmt.Sprintf("interrupted: %v", iv.Value())}
    }
    return TestResult{ErrorMessage: err.Error()}
}
```

[VERIFIED: goja API surface confirmed via pkg.go.dev — `goja.New()`, `vm.SetMaxCallStackSize()`, `vm.Interrupt()`, `goja.AssertFunction()`, `*goja.Exception`, `*goja.InterruptedError`]
[ASSUMED: `goja.Exception.Value().String()` returns user-facing message. Stack frame line/col is obtained via `ex.Error()` string which embeds source info. Verify exact API on `goja.Exception` at implementation time.]

### Pattern 3: Migration 0050 — device_profile Column Additions

```sql
-- 0050_catalog_metadata.up.sql
-- Phase 7 D-29..D-31 + D-41..D-44: add catalog tracking columns + profile-aware
-- alert metadata. Single migration — columns + backfill are atomic.

ALTER TABLE device_profile
    ADD COLUMN IF NOT EXISTS catalog_source         TEXT     NULL,
    ADD COLUMN IF NOT EXISTS catalog_source_version TEXT     NULL,
    ADD COLUMN IF NOT EXISTS customer_edited        BOOLEAN  NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS battery_curve          TEXT     NOT NULL DEFAULT 'linear_pct',
    ADD COLUMN IF NOT EXISTS expected_uplink_interval_seconds INT NOT NULL DEFAULT 3600,
    ADD COLUMN IF NOT EXISTS offline_threshold_multiplier FLOAT NOT NULL DEFAULT 3.0,
    ADD COLUMN IF NOT EXISTS anomaly_compatibility  TEXT     NOT NULL DEFAULT 'full';

-- D-29/D-30: backfill existing 3 seed profiles
UPDATE device_profile SET
    catalog_source         = slug,
    catalog_source_version = '1.0.0',
    customer_edited        = FALSE  -- D-31 drift detection done below
WHERE slug IN ('axioma_w1', 'acrel_adl200', 'acrel_adw300');

-- D-31 drift detection: set customer_edited=true where codec_js differs from embedded
-- NOTE: The actual hash comparison must be done in Go (Go has the embedded bytes;
-- SQL cannot access them). Implement in the migration Go function or in seed.go at
-- boot (post-migration hook pattern). See Pitfall below.

-- D-41/D-43: per-profile expected_uplink_interval_seconds + offline_threshold_multiplier
UPDATE device_profile SET
    expected_uplink_interval_seconds = 3600,  offline_threshold_multiplier = 3.0
WHERE slug = 'axioma_w1';
UPDATE device_profile SET
    expected_uplink_interval_seconds = 900,   offline_threshold_multiplier = 2.0
WHERE slug IN ('acrel_adl200', 'acrel_adw300');
UPDATE device_profile SET
    expected_uplink_interval_seconds = 86400, offline_threshold_multiplier = 1.8,
    anomaly_compatibility = 'limited'
WHERE slug = 'itron_kinmy_lora';

-- battery_curve defaults
UPDATE device_profile SET battery_curve = 'linear_pct'
WHERE slug IN ('axioma_w1', 'acrel_adl200', 'acrel_adw300');
UPDATE device_profile SET battery_curve = 'li_socl2_3v6'
WHERE slug = 'itron_kinmy_lora';

-- D-42: anomaly_compatibility CHECK constraint
ALTER TABLE device_profile
    ADD CONSTRAINT device_profile_anomaly_compat_check
    CHECK (anomaly_compatibility IN ('full', 'limited', 'unsupported'));

ALTER TABLE device_profile
    ADD CONSTRAINT device_profile_battery_curve_check
    CHECK (battery_curve IN ('linear_pct', 'li_socl2_3v6', 'li_mnox_3v0', 'alkaline_3v0', 'none'));
```

**Migration ordering recommendation (Claude's Discretion):** Use a SINGLE migration 0050. The columns + backfill are cheap and logically coupled. A two-migration split adds no safety benefit since all three operations are idempotent (`ADD COLUMN IF NOT EXISTS` + `UPDATE WHERE slug = ...`). The D-31 drift detection for `customer_edited` must be implemented as a boot-time check in `seed.go` (after migration runs) since only Go has the embedded JS bytes. The migration sets `customer_edited = FALSE` as a safe default; `RunSeedSync` (or a new `RunCatalogDriftCheck` function) computes hashes and flips the flag post-migration.

[VERIFIED: Migration patterns from existing migrations 0009/0010/0023; backfill-in-migration confirmed safe for this scale]

### Pattern 4: ALERT-03 Profile-Aware Refactor

Current `offline_worker.go` uses `c.ExpectedIntervalS` from the `ListOfflineDevicesWithGatewayStatus` query, with threshold `3 * interval` hardcoded in `fireDeviceOffline`. Phase 7 changes:

1. The SQL query already joins through to `device_profile` (via `device → device_profile`). Add `expected_uplink_interval_seconds` and `offline_threshold_multiplier` to the result row.
2. `fireDeviceOffline` changes threshold to `float64(c.ExpectedIntervalS) * c.OfflineThresholdMultiplier`.
3. `ListHysteresisClearOffline` must also use per-profile interval × multiplier (or use a generous fixed multiplier for the clear condition to avoid false hysteresis clears).

[VERIFIED: offline_worker.go line 248 — `float64(3 * interval)` hardcoded; query result provides `c.ExpectedIntervalS` already]

### Pattern 5: Backtest Query — "Test against last 30 days" (D-10 / D-35)

The backtest runs the same evaluator logic as the live worker, but against a historical slice:

```sql
-- For anomaly_p95 backtest: count days-with-fires in last 30 days
-- (daily sparkline = group by day, count distinct hour buckets that would fire)
SELECT
    time_bucket('1 day', bucket)                AS day,
    count(*) FILTER (WHERE avg_instant >
        percentile_cont(0.95) WITHIN GROUP (ORDER BY avg_instant)
            OVER (PARTITION BY metering_point_id
                  ORDER BY bucket
                  ROWS BETWEEN 720 PRECEDING AND 1 PRECEDING)
    )                                           AS fires
FROM measurement_hourly
WHERE metering_point_id = $1
  AND bucket >= now() - INTERVAL '30 days'
GROUP BY 1
ORDER BY 1;
```

**However**, this window-function-over-CAGG approach has a known TimescaleDB constraint: window functions inside aggregate calls are forbidden (SQLSTATE 42803). The backtest must use a simpler two-pass approach:

```sql
-- Pass 1: compute the P95 baseline once over the 30d window
SELECT percentile_cont(0.95) WITHIN GROUP (ORDER BY avg_instant) AS p95
FROM measurement_hourly
WHERE metering_point_id = $1
  AND bucket >= now() - INTERVAL '30 days';

-- Pass 2: count and group days where avg_instant > p95
SELECT
    time_bucket('1 day', bucket) AS day,
    count(*) AS fires
FROM measurement_hourly
WHERE metering_point_id = $1
  AND bucket >= now() - INTERVAL '30 days'
  AND avg_instant > $2  -- pass in the p95 from pass 1
GROUP BY 1
ORDER BY 1;
```

This two-pass approach is TimescaleDB-compatible, uses the existing `measurement_hourly` CAGG (confirmed: `SELECT avg_instant, bucket, metering_point_id FROM measurement_hourly`), and produces the 30-row daily sparkline required by D-35.

**For IQR backtest:** same two-pass — compute Q1/Q3 once, then count rows outside bounds per day.
**For quiet_hour backtest:** count rows in the quiet window where avg_instant > flow_threshold per day.

[VERIFIED: measurement_hourly CAGG schema from 0025_cagg_hourly.up.sql — `avg_instant`, `bucket`, `metering_point_id` columns exist]
[VERIFIED: TimescaleDB window-function-in-aggregate limitation confirmed via 0025 CAGG comments and Phase 5 RESEARCH pitfall]

### Pattern 6: Battery Curve Registry in normalize.go

```go
// internal/ingest/normalize.go — add to existing file

// BatteryCurve maps a voltage (V) to battery_pct (0-100).
type BatteryCurve func(voltageV float64) int16

// batteryCurves maps the catalog enum values to conversion functions.
// Linear_pct: codec already emits 0-100 — pass-through (no voltage input needed).
var batteryCurves = map[string]BatteryCurve{
    "linear_pct":  func(_ float64) int16 { return -1 }, // sentinel: don't apply, codec owns battery_pct
    "li_socl2_3v6": liSOCl23V6,
    "li_mnox_3v0":  liMnO23V0,
    "alkaline_3v0": alkaline3V0,
    "none":         func(_ float64) int16 { return -1 }, // no battery field
}

// liSOCl23V6 approximates the Li-SOCl2 3.6V plateau-then-cliff discharge curve.
// Reference points: 3.6V→100%, 3.4V→85%, 3.2V→50%, 3.0V→20%, 2.8V→0%.
// The plateau from 3.4-3.6V represents ≈85% of capacity; below 3.2V cliff drop.
// [ASSUMED: curve breakpoints are reasonable for Li-SOCl2 ER-series cells;
// verify against Tadiran ER26500 or SAFT LS33600 datasheet for accuracy]
func liSOCl23V6(v float64) int16 {
    switch {
    case v >= 3.6:  return 100
    case v >= 3.4:  return int16(85 + (v-3.4)/(3.6-3.4)*15)  // 85-100%
    case v >= 3.2:  return int16(50 + (v-3.2)/(3.4-3.2)*35)  // 50-85%
    case v >= 3.0:  return int16(20 + (v-3.0)/(3.2-3.0)*30)  // 20-50%
    case v >= 2.8:  return int16((v-2.8)/(3.0-2.8)*20)       // 0-20%
    default:        return 0
    }
}
```

**Integration point in normalize.go:** After `NormalizeMeasurement` runs mappings, the ingest handler (or a post-normalize step) checks `profile.BatteryCurve`. If it's NOT `linear_pct` or `none`, and `layer1.Extra["battery_v"]` exists as a float, apply the curve to produce `layer1.BatteryPct`. The raw voltage stays in `layer1.Extra["battery_v"]` (→ `measurement.raw.battery_v`) per D-44.

[VERIFIED: normalize.go reviewed — `Extra` map stores vendor fields; `BatteryPct *int16` is the canonical output field]
[ASSUMED: Li-SOCl2 curve breakpoints. OWASP-equivalent authoritative reference not found in session. Use as engineering approximation; flag in Assumptions Log.]

### Anti-Patterns to Avoid

- **Vendor switch in normalize.go:** Per Phase 2 Pitfall §3 and the normalize.go file comment "THIS FUNCTION CONTAINS NO VENDOR SWITCH" — battery curve must be data-driven (map lookup), not a `case "itron_kinmy_lora":` branch [VERIFIED: normalize.go line 80]
- **Blocking boot in RunSeedSync:** Phase 2 seed.go is already best-effort/non-blocking; D-36 re-sync uses the same pattern — clear `codec_js_synced_at = NULL`, let the existing poller handle it [VERIFIED: seed.go line 37]
- **goja global runtime:** Each test-runner request must create a fresh `goja.New()` instance — goja runtimes are NOT goroutine-safe [CITED: pkg.go.dev/github.com/dop251/goja — "A goja.Runtime may only be used from a single goroutine"]
- **goja without interrupt:** Never run user-supplied codec in goja without the 100ms `time.AfterFunc` interrupt — an infinite loop will block the HTTP goroutine indefinitely [VERIFIED: goja interrupt API confirmed via pkg.go.dev]
- **TimescaleDB window functions inside CAGG aggregate calls:** Not supported (SQLSTATE 42803) — use two-pass queries for backtest [VERIFIED: 0025 CAGG comments]
- **ALTER TABLE NOT NULL without DEFAULT on large table:** The new `device_profile` columns must have DEFAULT values to avoid table rewrite. All columns above carry explicit defaults [VERIFIED: migration pattern from 0023]

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| JS execution for codec test-runner | Custom JS parser / eval shim | `github.com/dop251/goja` | ES5+ support, pure Go, no CGO, interrupt/timeout API |
| Codec test-runner timeout | `context.WithTimeout` + channel | `vm.Interrupt()` via `time.AfterFunc` | goja runtime is single-threaded; context cancellation does not interrupt `RunString`; only `vm.Interrupt()` works |
| Report template state serialization | Structured DB columns per filter field | Single `JSONB` column for template state | ReportConfigPanel state is `{scope, rangePreset, siteIDs, meterIDs, groupBy, capability}` — all optional fields; JSONB avoids ALTER TABLE on every future filter addition |
| Codec re-sync after catalog Update | New sync loop / webhook | Set `codec_js_synced_at = NULL` | Phase 2 `RunSeedSync` polls `ListUnsyncedProfiles` at boot — clearing the timestamp is sufficient [VERIFIED: seed.go] |
| Bulk gateway import from scratch | New upload/parse/validate/commit pipeline | Reuse `internal/import/` pattern | Phase 3 `importpkg.CommitDeps`, `DryrunDeps`, `parser_csv.go` are all reusable with a gateway-specific schema validator |
| Battery voltage to percent conversion | Ad-hoc inline calculation | `batteryCurves` registry in normalize.go | Centralizes all vendor curves; test-friendly; data-driven |

---

## Runtime State Inventory

**Trigger:** Phase 7 adds new columns to `device_profile` and seeds Itron+KINMY. No string renames; no rebrand. Catalog metadata is new state, not renaming existing state.

| Category | Items Found | Action Required |
|----------|-------------|-----------------|
| Stored data | `device_profile` rows for 3 existing slugs — need `catalog_source`, `catalog_source_version`, `customer_edited`, `battery_curve`, `expected_uplink_interval_seconds`, `offline_threshold_multiplier`, `anomaly_compatibility` backfilled | SQL UPDATE in migration 0050 (data migration) |
| Stored data | `device_profile.codec_js` for existing 3 rows — drift detection to set `customer_edited = true` if hash differs from embedded | Go code in `RunSeedSync` or new `RunCatalogDriftCheck` at boot (code edit post-migration) |
| Live service config | ChirpStack — existing 3 profiles already pushed; Itron+KINMY not yet pushed (codec_js_synced_at = NULL for new row) | Phase 2 `RunSeedSync` handles at next boot — zero new sync code needed |
| OS-registered state | None — no OS task scheduler, no pm2, no launchd artifacts for catalog metadata | None |
| Secrets/env vars | No new env vars required for catalog feature | None |
| Build artifacts | `web/dist/` — deleted per git status (D gitkeep); fresh build after frontend changes | `pnpm --dir web build` |

**Nothing found in categories:** OS-registered state — verified by codebase scan (no task scheduler, no pm2 config, no launchd). Secrets/env vars — catalog is embedded in binary, no external URL/API keys.

---

## Common Pitfalls

### Pitfall 1: goja.Interrupt() vs context.Context
**What goes wrong:** Passing a `context.WithTimeout` to goja does NOT stop a running script. The operator pastes an infinite-loop codec; the HTTP handler hangs indefinitely.
**Why it happens:** goja's `RunString` runs synchronously on the calling goroutine with no select on context. Context cancellation has no effect on an executing `RunString`.
**How to avoid:** Use `time.AfterFunc(100*time.Millisecond, func() { vm.Interrupt("timeout") })`. This schedules an interrupt on the goja runtime's internal interrupt channel, which stops the script at the next safe point.
**Warning signs:** Test by running `decodeUplink` with a `while(true) {}` body — should return `TestResult{ErrorMessage: "interrupted: timeout"}` within 200ms.

[CITED: pkg.go.dev/github.com/dop251/goja — "Interrupt may be used to interrupt a long running script. ...If the interrupt value is set, it will be thrown as an error."]

### Pitfall 2: D-31 Drift Detection Requires Go, Not SQL
**What goes wrong:** Migration 0050 cannot compute `SHA256(existing codec_js) vs SHA256(embedded bytes)` — the migration has no access to the embedded JS files.
**Why it happens:** golang-migrate runs pure SQL files; there is no pre/post SQL hook with embedded Go assets.
**How to avoid:** Migration 0050 sets `customer_edited = FALSE` as default for existing rows. A new `RunCatalogDriftCheck(ctx, pool, embeddedCodecs map[string]string)` function runs at boot (after migrations), iterates existing 3 rows, computes hashes, sets `customer_edited = TRUE` where they differ. Idempotent — safe to re-run.
**Warning signs:** If migration-only approach is chosen, operators who customized a codec won't see the "you edited this" flag — catalog Update will silently overwrite their changes.

[VERIFIED: seed.go boot pattern reviewed — RunSeedSync runs at boot, not in migration. Same location for RunCatalogDriftCheck.]

### Pitfall 3: Cold-Start Gate Must Use Per-Profile Warmup Duration
**What goes wrong:** Phase 6 cold_start.go uses a global `AnomalyWarmupDays = 21`. For Itron+KINMY (`anomaly_compatibility: limited`), the warmup should be 60-90 days (D-42). Using 21 days with 1 uplink/day gives only 21 data points — too few for meaningful P95/IQR.
**Why it happens:** Global constant is sufficient for Phase 6 profiles (Axioma, Acrel — hourly uplinks, ~720 data points in 21 days). For daily uplinks, 21 days = 21 data points, statistically meaningless.
**How to avoid:** Extend `IsMPEligibleForAnomaly` to take `anomaly_compatibility` from the bound `device_profile`. For `full` profiles: 21-day gate (unchanged). For `limited` profiles: 60-day gate. Query joins through device → device_profile to get the enum.
**Warning signs:** Itron+KINMY meters triggering anomaly alerts in week 3 with only ~20 baseline points.

[VERIFIED: cold_start.go line 27 — `const AnomalyWarmupDays = 21`; confirmed no per-profile branching]

### Pitfall 4: TimescaleDB CAGG No Window Functions Inside Aggregates
**What goes wrong:** Trying to write a backtest query as `SELECT day, count(*) FILTER (... window function ...)` fails with SQLSTATE 42803.
**Why it happens:** TimescaleDB continuous aggregates forbid window function calls nested inside aggregate calls (documented in Phase 5 0025_cagg_hourly.up.sql comment lines 22-33).
**How to avoid:** Use two-pass queries: pass 1 computes the baseline (P95 / Q1+Q3), pass 2 counts days where the per-hour bucket exceeds the baseline. Both passes hit `measurement_hourly` CAGG — fast.
**Warning signs:** `ERROR: window function calls cannot be nested inside aggregate calls (SQLSTATE 42803)` at query time.

[VERIFIED: 0025_cagg_hourly.up.sql line 22-33 explicit comment on this pitfall]

### Pitfall 5: Catalog JSON embed — embed.FS ReadDir Path
**What goes wrong:** `//go:embed catalog/*.json` embeds the files with path `catalog/axioma_w1.json`. `embed.FS.ReadDir(".")` returns `["catalog"]` not the JSON files. Must call `ReadDir("catalog")` to get the file list.
**Why it happens:** embed.FS preserves the directory structure from the embed directive path.
**How to avoid:** Use `catalogFS.ReadDir("catalog")` to iterate, and `catalogFS.ReadFile("catalog/axioma_w1.json")` to read each. Pattern mirrors the Phase 2 `//go:embed *.js` convention at the codecs level (where all files are at the root level of the embed).

[VERIFIED: embed.go convention confirmed from `//go:embed *.js` — no subdirectory. Phase 7 uses `catalog/` subdirectory. ReadDir("catalog") required.]

### Pitfall 6: report_template State Serialization
**What goes wrong:** Storing ReportConfigPanel state as individual columns (site_ids TEXT[], meter_ids TEXT[], group_by TEXT, ...) requires ALTER TABLE when new filter options are added.
**Why it happens:** ReportConfigPanel is expected to evolve (Phase 8 may add capability filters, date-range presets, etc.).
**How to avoid:** Store template state as a JSONB column (`template_state JSONB NOT NULL`). Validate structure at the API layer (Go struct + `encoding/json`). The download_format is NOT stored (D-14: operator picks at run time).

[VERIFIED: ReportConfigPanel state reviewed in /Users/suraboonsung/Documents/Programming/shifter/web/src/routes/reports/ReportConfigPanel.tsx — multiple optional filter fields]

### Pitfall 7: goja Object Construction for decodeUplink Input
**What goes wrong:** Passing a Go `[]byte` directly to goja as the `bytes` argument fails — the codec expects a JavaScript Array of integers.
**Why it happens:** goja's `vm.ToValue([]byte)` creates a Buffer-like object, not an array of integers. ChirpStack's QuickJS passes `bytes` as an integer array.
**How to avoid:** Build a JavaScript array manually using `vm.NewArray()` and set each element as an integer. Example verified from goja docs:
```go
bytesArr := make([]interface{}, len(hexBytes))
for i, b := range hexBytes { bytesArr[i] = int(b) }
inputObj.Set("bytes", vm.ToValue(bytesArr))
```
**Warning signs:** Codec calls `bytes.length` and gets unexpected value; or `bytes[0] !== 0x6F` check fails even with correct input.

[CITED: pkg.go.dev/github.com/dop251/goja — `vm.ToValue` documentation; JavaScript array semantics for byte arrays]

---

## Code Examples

### Example 1: Catalog TestCatalogValid Build-Time Test (D-22)

```go
// internal/codec/catalog_test.go
// Table-driven vs per-file subtests (Claude's Discretion): recommend per-file subtests
// so CI output names the broken file directly, not a generic "table row 3 failed"

func TestCatalogValid(t *testing.T) {
    entries, err := LoadAll()
    require.NoError(t, err, "catalog.LoadAll failed — check catalog/*.json syntax")
    require.NotEmpty(t, entries, "catalog is empty")

    validCurves := map[string]bool{"linear_pct": true, "li_socl2_3v6": true, "li_mnox_3v0": true, "alkaline_3v0": true, "none": true}
    validAnomalyCompat := map[string]bool{"full": true, "limited": true, "unsupported": true}
    validCapabilities := map[string]bool{"cumulative": true, "flow_rate": true, "instant_power": true, "battery": true, "temperature": true, "pressure": true, "leak_detection": true, "tamper_detection": true, "multi_phase": true, "power_quality": true}

    slugs := map[string]bool{}
    for _, e := range entries {
        t.Run(e.Slug, func(t *testing.T) {
            require.NotEmpty(t, e.Slug)
            require.Equal(t, strings.ToLower(e.Slug), e.Slug, "slug must be lowercase")
            require.False(t, slugs[e.Slug], "duplicate slug %q", e.Slug)
            slugs[e.Slug] = true
            require.NotEmpty(t, e.Name)
            require.NotEmpty(t, e.Vendor)
            require.NotEmpty(t, e.Version)
            require.True(t, semver.IsValid("v"+e.Version), "version must be semver: %q", e.Version)
            require.NotEmpty(t, e.CodecJSPath)
            require.True(t, validCurves[e.BatteryCurve], "unknown battery_curve %q", e.BatteryCurve)
            require.True(t, validAnomalyCompat[e.AnomalyCompatibility], "unknown anomaly_compatibility %q", e.AnomalyCompatibility)
            require.Greater(t, e.ExpectedUplinkIntervalSeconds, 0)
            require.Greater(t, e.OfflineThresholdMultiplier, 0.0)
            for _, cap := range e.Capabilities {
                require.True(t, validCapabilities[cap], "unknown capability %q in %q", cap, e.Slug)
            }
            // D-22: codec_js_path must resolve to an existing embedded JS file
            codecJS := codecs.CodecBySlug(e.Slug)
            require.NotEmpty(t, codecJS, "codec_js_path %q has no embedded JS (CodecBySlug returned empty)", e.CodecJSPath)
        })
    }
    require.Equal(t, 4, len(entries), "expected exactly 4 catalog entries for v1.0.0")
}
```

[VERIFIED: TestCatalogValid pattern follows existing codec_test approach; `codecs.CodecBySlug` already handles all 4 slugs]

### Example 2: profile-aware ALERT-03 offline threshold

```go
// internal/alert/offline_worker.go — modify fireDeviceOffline
// Replace hardcoded 3 * interval with per-profile multiplier

// Before (Phase 6):
threshold := float64(3 * interval)

// After (Phase 7 D-41/D-43):
// c.OfflineThresholdMultiplier comes from the updated SQL query
// (join device_profile to get offline_threshold_multiplier)
threshold := float64(c.ExpectedIntervalS) * c.OfflineThresholdMultiplier
```

The `ListOfflineDevicesWithGatewayStatus` SQL query needs one additional JOIN column:
```sql
-- Add to the existing query:
dp.offline_threshold_multiplier,
dp.expected_uplink_interval_seconds
-- via JOIN device ON d.id = device.id JOIN device_profile dp ON d.device_profile_id = dp.id
```

[VERIFIED: offline_worker.go line 248-261 — existing threshold construction; query column `c.ExpectedIntervalS` already present]

### Example 3: Cobra doctor probe subcommands

```go
// internal/cli/doctor.go — add to init()
func init() {
    doctorCmd.Flags().StringVar(&doctorFlagOut, "out", "", "...")
    // Phase 7: add probe subcommands
    doctorCmd.AddCommand(probeChirpStackCmd)
    doctorCmd.AddCommand(probeTimescaleCmd)
    doctorCmd.AddCommand(probeRegionCmd)
}

// internal/doctor/probes.go (new file)

type ProbeResult struct {
    Name      string    `json:"name"`
    Status    string    `json:"status"` // "ok" | "warn" | "error"
    Message   string    `json:"message"`
    LastRunAt time.Time `json:"last_run_at"`
}

// ProbeChirpStack checks ChirpStack version + reachability.
// D-13: never blocks; always returns a result even on failure.
func ProbeChirpStack(ctx context.Context, grpcURL, apiKey string) ProbeResult {
    // dial + GetVersion; compare against supported minimum (4.10)
    // warn if < 4.10, error if v3, ok otherwise
}

// ProbeTimescale checks whether the timescaledb extension is installed + version.
func ProbeTimescale(ctx context.Context, pool *pgxpool.Pool) ProbeResult {
    // SELECT extname, extversion FROM pg_extension WHERE extname = 'timescaledb'
}

// ProbeRegion checks if gateway region matches install identity region.
func ProbeRegion(ctx context.Context, pool *pgxpool.Pool) ProbeResult {
    // SELECT region FROM chirpstack_connection; compare to gateways.region
}
```

[VERIFIED: doctor.go pattern reviewed; `internal/doctor/doctor.go` shows `Bundle` struct with `ChirpstackGRPCPing GRPCPingResult` — same approach for probes]
[VERIFIED: `/health/detailed` endpoint in doctor.go `gatherHealthDetailed` — D-40 last-run update can add `probe_results` field to the existing map]

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Global `AnomalyWarmupDays = 21` | Per-profile warmup based on `anomaly_compatibility` | Phase 7 | Limited profiles (daily uplinks) get 60-day warmup instead of 21 days |
| Global offline threshold `3 × interval` | Per-profile `interval × offline_threshold_multiplier` | Phase 7 | Itron+KINMY gets 1.8× multiplier (~43h tolerance for daily uplinks) |
| Codec test via fake-uplink-over-MQTT | Local goja execution via HTTP endpoint | Phase 7 | No MQTT dependency for test path; works before ChirpStack sync |
| Battery as codec-owned `battery_pct` only | Battery curve registry in normalize.go | Phase 7 | Voltage-reporting vendors (Itron+KINMY) get meaningful `battery_pct` |

**No deprecated patterns** in this phase — all Phase 6 substrate remains valid.

---

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | goja `*goja.Exception.Value().String()` returns the user-facing error message; `ex.Error()` returns a string containing source location info | Code Examples §2 | Test-runner failure UX (D-28) may show wrong error format; fix during implementation by inspecting actual `ex` fields |
| A2 | `vm.ToValue([]interface{}{int(b1), int(b2)...})` creates a proper JS integer array that codec's `bytes[0] !== 0x6F` comparison works against | Pattern 2 / Pitfall 7 | Codec SOF check fails even with correct hex input; fix by using `vm.NewArray` or explicit `vm.CreateObject` array construction |
| A3 | Li-SOCl2 3.6V curve breakpoints: 3.6V→100%, 3.4V→85%, 3.2V→50%, 3.0V→20%, 2.8V→0% | Pattern 6 | Battery% readings show as 12% when device is actually at 60% (or vice versa); operator confusion; fix by consulting Tadiran ER26500 or SAFT LS33600 datasheet |
| A4 | goja `@latest` pseudo-version at time of `go get` will be compatible with Go 1.26.1 and will compile without CGO | Standard Stack | Build fails or requires CGO; goja is documented as pure Go so risk is LOW |
| A5 | The existing `measurement_hourly.avg_instant` column is a suitable proxy for the backtest evaluator (same as what the live anomaly worker uses) | Pattern 5 | Backtest fires count differs from live count; acceptable UX risk since backtest is "approximate signal", not exact replay |

---

## Open Questions

1. **D-31 drift detection timing: migration vs boot hook**
   - What we know: Migration 0050 must set `customer_edited = FALSE` as default. The Go-side hash check requires access to embedded JS bytes.
   - What's unclear: Should `RunCatalogDriftCheck` run every boot (cheap, idempotent) or only when `catalog_source_version IS NOT NULL AND customer_edited = FALSE`?
   - Recommendation: Run every boot but gate on `catalog_source IS NOT NULL AND customer_edited = FALSE` rows to avoid repeated hash comparisons on rows the operator hasn't imported from catalog. Cost: 3 SHA256 hashes per boot — negligible.

2. **goja version pinning**
   - What we know: goja uses pseudo-versions (no semantic releases). The March 2026 pseudo-version is `v0.0.0-20260311135729-065cd970411c`.
   - What's unclear: Whether this is the latest commit or an older one; whether a newer commit has relevant fixes.
   - Recommendation: Run `go get github.com/dop251/goja@latest` in Wave 0 and pin the resulting pseudo-version in go.mod. Document the commit hash in a `// goja commit: <hash>` comment in `codec_runner/runner.go`.

3. **report_template table primary key approach**
   - What we know: Templates are install-wide shared (D-15); operator-chosen names; alphabetical listing (D-39).
   - What's unclear: Should template names be unique (enforced by DB UNIQUE constraint) or allow duplicates with UUID PK?
   - Recommendation: UUID PK + unique name constraint (`UNIQUE(name)`) to prevent operator confusion. Name is the display key in the alphabetical list; duplicates would be confusing.

4. **Backtest endpoint location**
   - What we know: D-10 says "backtest button on the rule-enable surface" — could be inside the Add Rule dialog or on the rule library row. D-35 specifies the output format.
   - What's unclear: Whether the endpoint should live under `/api/alerts/rules/{id}/backtest` (rule-specific) or `/api/metering-points/{mp_id}/anomaly-backtest?kind=anomaly_p95` (MP-centric).
   - Recommendation: `/api/alerts/backtest` with body `{rule_kind, mp_id, days: 30}` — independent of whether a rule exists (operator may want to test before creating the rule). Matches the read-only / no-write-alerts semantics of D-10.

---

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | Backend compilation | ✓ | go1.26.1 darwin/arm64 | — |
| Node.js | Frontend build | ✓ | v22.20.0 | — |
| Docker | Compose / testcontainers | ✓ | 29.4.1 | — |
| PostgreSQL (local) | Integration tests | ✓ | 14.17 (Homebrew) | testcontainers-go (already used in project tests) |
| `github.com/dop251/goja` | Codec test-runner | ✗ | — | `go get` in Wave 0 |
| TimescaleDB | CAGG queries in tests | Provided via testcontainers-go (existing pattern) | — | — |

**Missing dependencies with no fallback:** None — goja is a pure `go get` addition.

**Missing dependencies with fallback:** goja — must be added in Wave 0 before any codec_runner code compiles.

[VERIFIED: go version, node version, docker version from env probe]

---

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go: `testing` + `testify/require` v1.11.1; Frontend: Vitest + Playwright |
| Config file | `go test ./... -race -count=1`; `pnpm --dir web test:run`; `pnpm --dir web exec playwright test` |
| Quick run command | `go test ./internal/codec/... ./internal/codec_runner/... ./internal/alert/... ./internal/ingest/... -race -count=1` |
| Full suite command | `go test ./... -race -count=1 && pnpm --dir web test:run` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| V2-VEND-01 | All 4 catalog entries valid (D-22 schema) | unit | `go test ./internal/codec/... -run TestCatalogValid` | ❌ Wave 0 |
| V2-VEND-01 | Catalog import prefills device-profile form | integration | `go test ./internal/profile/... -run TestImportFromCatalog` | ❌ Wave 0 |
| V2-VEND-01 | Migration 0050 backfills existing 3 profiles | unit | `go test ./internal/db/... -run TestMigration_0050` | ❌ Wave 0 |
| V2-VEND-01 | D-31 drift detection sets customer_edited | unit | `go test ./internal/codec/... -run TestCatalogDriftDetection` | ❌ Wave 0 |
| V2-VEND-02 | Axioma codec hex → decoded JSON + canonical | unit | `go test ./internal/codec_runner/... -run TestRunCodecTest_AxiomaW1` | ❌ Wave 0 |
| V2-VEND-02 | Itron codec hex → decoded JSON + canonical | unit | `go test ./internal/codec_runner/... -run TestRunCodecTest_ItronKinmy` | ❌ Wave 0 |
| V2-VEND-02 | Codec timeout fires within 200ms | unit | `go test ./internal/codec_runner/... -run TestRunCodecTest_Timeout` | ❌ Wave 0 |
| V2-VEND-02 | Syntax error returns line/col in error result | unit | `go test ./internal/codec_runner/... -run TestRunCodecTest_SyntaxError` | ❌ Wave 0 |
| V2-VEND-03 | Template save/load/list/delete | unit | `go test ./internal/report/... -run TestTemplateStore` | ❌ Wave 0 |
| V2-VEND-03 | Bulk gateway import dry-run + commit | unit | `go test ./internal/gateway/... -run TestGatewayImport` | ❌ Wave 0 |
| V2-VEND-03 | Compare endpoint returns 2-entity result | integration | `go test ./internal/report/... -run TestCompareHandler` | ❌ Wave 0 |
| ALERT-04 | ALERT-03 offline threshold uses profile multiplier | unit | `go test ./internal/alert/... -run TestOfflineWorker_ProfileAwareThreshold` | ❌ Wave 0 |
| ALERT-04 | `limited` profile gets 60-day warmup gate | unit | `go test ./internal/alert/... -run TestColdStart_LimitedProfile` | ❌ Wave 0 |
| ALERT-04 | Battery curve li_socl2_3v6 converts voltage correctly | unit | `go test ./internal/ingest/... -run TestBatteryCurve_LiSOCl2` | ❌ Wave 0 |
| ALERT-04 | reverse_flow_increase evaluator fires on delta threshold | unit | `go test ./internal/alert/... -run TestReverseFlowIncrease` | ❌ Wave 0 |
| ALERT-04 | Backtest returns count + 30-day sparkline | unit | `go test ./internal/alert/... -run TestAnomalyBacktest` | ❌ Wave 0 |
| ALERT-04 | Doctor probe-chirpstack returns version + status | unit | `go test ./internal/doctor/... -run TestProbeChirpStack` | ❌ Wave 0 |
| ALERT-04 | Doctor probe-timescale detects extension presence | unit | `go test ./internal/doctor/... -run TestProbeTimescale` | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/codec/... ./internal/codec_runner/... ./internal/alert/... ./internal/ingest/... -race -count=1`
- **Per wave merge:** `go test ./... -race -count=1`
- **Phase gate:** Full suite (Go + frontend) green before `/gsd-verify-work`

### Wave 0 Gaps

- [ ] `internal/codec/catalog.go` — D-33 CatalogEntry struct + embed.FS + LoadAll()
- [ ] `internal/codec/catalog_test.go` — TestCatalogValid build-time check
- [ ] `internal/codec/catalog/axioma_w1.json` — catalog entry
- [ ] `internal/codec/catalog/acrel_adl200.json` — catalog entry
- [ ] `internal/codec/catalog/acrel_adw300.json` — catalog entry
- [ ] `internal/codec/catalog/itron_kinmy_lora.json` — catalog entry
- [ ] `internal/codec_runner/runner.go` — goja sandbox runner
- [ ] `internal/codec_runner/runner_test.go` — 4 test cases (axioma, itron, timeout, syntax error)
- [ ] `internal/db/migrations/0050_catalog_metadata.up.sql` — ALTER TABLE + backfill
- [ ] `internal/db/migrations/0050_catalog_metadata.down.sql` — reverse
- [ ] `go get github.com/dop251/goja@latest` — new dependency

---

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | Existing SCS sessions unchanged |
| V3 Session Management | no | Unchanged |
| V4 Access Control | yes | Catalog import/update/delete = admin only; test-runner = admin only; template create/delete = admin only, run = viewer OK |
| V5 Input Validation | yes | Catalog JSON schema validated at build time (D-22); hex bytes validated (length + SOF) before goja call; template state validated as JSON |
| V6 Cryptography | no | No new crypto; D-31 drift detection uses SHA256 from stdlib `crypto/sha256` |

### Known Threat Patterns

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Malicious codec JS (infinite loop, stack overflow) | Denial of Service | goja `vm.Interrupt(100ms)` + `vm.SetMaxCallStackSize(500)` — already in Pattern 2 above |
| Codec JS reading OS environment / executing system commands | Elevation of Privilege | goja does NOT expose `os`, `process`, `require`, or `fs` modules by default — no Go stdlib APIs bound; codec JS runs in an empty runtime with only stdlib builtins |
| Oversized hex payload to test-runner | Denial of Service | Validate hex length before passing to goja; cap at 256 bytes (standard LoRaWAN max payload) |
| Template state containing SQL injection via JSONB | Tampering | Template state stored as JSONB and queried via parameterized queries; not interpolated into SQL strings |

[CITED: pkg.go.dev/github.com/dop251/goja — "An instance of goja.Runtime can only be used by a single goroutine...no standard Go packages are available unless explicitly mapped"]

---

## Project Constraints (from CLAUDE.md)

| Directive | Applies to Phase 7 |
|-----------|-------------------|
| Backend: Go 1.24+ | go1.26.1 in use — compliant |
| Use pgx/v5 + sqlc (NOT GORM) | All new queries use sqlc; no GORM |
| Use alexedwards/scs sessions | Auth unchanged; no new session handling |
| Use golang-migrate plain-SQL migrations | Migration 0050 is plain SQL |
| Frontend: shadcn/ui + blue/navy palette | Catalog tab, diff modal, test-runner panel all follow shadcn/ui |
| All CRUD via dialogs (no full-page CRUD) | Catalog import dialog, diff modal, template save dialog all follow modal-first |
| Maps: OSM via react-leaflet (no paid APIs) | No new map work in Phase 7 |
| Tech stack: TimescaleDB for time-series | Backtest uses `measurement_hourly` CAGG |
| ChirpStack: gRPC communication | Doctor probe-chirpstack uses existing gRPC dial pattern |
| Deployment: single-binary self-hosted | Catalog is `//go:embed` — no external file deps at install time |

---

## Sources

### Primary (HIGH confidence)
- Codebase: `/Users/suraboonsung/Documents/Programming/shifter/internal/profile/codecs/embed.go` — embed pattern, CodecBySlug dispatch, 4 slugs verified
- Codebase: `/Users/suraboonsung/Documents/Programming/shifter/internal/profile/seed.go` — boot-time sync, best-effort pattern, D-36 reuse confirmed
- Codebase: `/Users/suraboonsung/Documents/Programming/shifter/internal/alert/anomaly_worker.go` — hardcoded 30-day window, P95/IQR evaluators, cold-start gate call
- Codebase: `/Users/suraboonsung/Documents/Programming/shifter/internal/alert/offline_worker.go` — hardcoded `3 * interval` threshold, candidates query structure
- Codebase: `/Users/suraboonsung/Documents/Programming/shifter/internal/alert/cold_start.go` — `AnomalyWarmupDays = 21` constant, `IsMPEligibleForAnomaly` signature
- Codebase: `/Users/suraboonsung/Documents/Programming/shifter/internal/ingest/normalize.go` — Layer1 struct, no vendor switch, Extra map, battery_pct assignment
- Codebase: `/Users/suraboonsung/Documents/Programming/shifter/internal/db/migrations/0009_device_profile.up.sql` — device_profile schema (7 columns, capabilities CHECK, no catalog columns yet)
- Codebase: `/Users/suraboonsung/Documents/Programming/shifter/internal/db/migrations/0010_seed_profiles.up.sql` — 3 seed rows; no catalog_source/battery_curve/expected_uplink_interval
- Codebase: `/Users/suraboonsung/Documents/Programming/shifter/internal/db/migrations/0025_cagg_hourly.up.sql` — measurement_hourly CAGG schema; window-function-in-aggregate prohibition documented
- Codebase: `/Users/suraboonsung/Documents/Programming/shifter/internal/db/queries/alerts.sql` — P95BaselineForMPAndHour, IQRBaselineForMP queries use raw `measurement` table not CAGG
- Codebase: `/Users/suraboonsung/Documents/Programming/shifter/internal/doctor/doctor.go` — Bundle struct, probe result pattern, `/health/detailed` shape
- Codebase: `/Users/suraboonsung/Documents/Programming/shifter/go.mod` — confirmed goja NOT in module; all other deps verified

### Secondary (MEDIUM confidence)
- [pkg.go.dev/github.com/dop251/goja](https://pkg.go.dev/github.com/dop251/goja) — goja API: `goja.New()`, `vm.SetMaxCallStackSize()`, `vm.Interrupt()`, `goja.AssertFunction()`, `*goja.Exception`, `*goja.InterruptedError`. Latest version pseudo-date Mar 2026.
- [deepwiki.com/chirpstack/chirpstack/4.2-device-profile-api](https://deepwiki.com/chirpstack/chirpstack/4.2-device-profile-api) — Confirmed NO `TestUplinkCodec` gRPC method in ChirpStack v4. DeviceProfile service has 6 methods (CRUD + List + ListADR). Phase 7 D-26 (local goja) is the correct choice.
- [ui.shadcn.com/docs/components/data-table](https://ui.shadcn.com/docs/components/data-table) — TanStack Table + shadcn DataTable with `onColumnFiltersChange`, `getFilteredRowModel`, sortable columns. Chip-based filter requires custom extension.

### Tertiary (LOW confidence)
- Li-SOCl2 discharge curve breakpoints (3.6V=100%, 3.2V=50%, 2.8V=0%) — inferred from IoT LoRaWAN sensor datasheets (Dragino, RAKwireless forum posts). No primary datasheet consulted in this session. Marked as A3 in Assumptions Log.

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all existing deps verified in go.mod; goja existence confirmed via pkg.go.dev
- Architecture: HIGH — all substrate patterns verified from codebase; goja patterns MEDIUM (API surface verified, implementation details ASSUMED)
- Pitfalls: HIGH — goja interrupt, CAGG window function constraint, drift detection Go-only are all confirmed from authoritative sources
- Battery curve: LOW — Li-SOCl2 breakpoints are engineering approximations from secondary IoT sources

**Research date:** 2026-05-12
**Valid until:** 2026-06-12 (30 days; goja is actively developed, check for updates if planning is delayed)
