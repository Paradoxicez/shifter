---
phase: 02-domain-model-canonical-schema
plan: 13
subsystem: testing
tags: [testharness, mqtt, paho, testcontainers, scenarios, codec, axioma_w1, acrel_adw300, sync-barrier]

# Dependency graph
requires:
  - phase: 02-domain-model-canonical-schema
    provides: ingest pipeline (02-09), swap commit (02-07), profile mappings (02-08), measurement schema (02-04)
provides:
  - Synthetic uplink publisher (paho MQTT) for arbitrary CS v4 events
  - Two vendor uplink builders (Axioma W1, Acrel ADW300) emitting decoded.object keys matching codec result.* exactly
  - 5 named D-27 scenario builders (clean_swap, swap_with_inflight_uplink, rollover, swap_then_rollover, overlapping_uplinks_during_swap) + AxiomaW1 E2E (DATA-10)
  - W3 broker-mode ingest sync barrier (waitForMeasurementCount, 100ms poll, 10s ceiling, operator-facing timeout error)
  - shifter test-harness CLI subcommand (D-27 dual-entry — operator runbook proof tool)
affects: [phase-02 plan-02-15 (VALIDATION.md flip), phase-04 dashboards, phase-06 audit browse]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "publishOrInline: scenarios run in two transports (MQTT broker mode with W3 sync barrier; in-process synchronous handler) sharing the SAME Go code"
    - "Codec-pinned mapping seeds: harness mapping rows reference json_pointer values matching axioma_w1.js / acrel_family.js result.* keys EXACTLY (B2 fix)"
    - "Operator-facing W3 timeout error embeds the broker URL so the dominant 'shifter serve not connected to broker' failure mode is named in the message"
    - "FNV-1a-based dev_eui pad: stable label→16-hex avoids the 'all letters strip to same hex' pitfall a naive hex-only filter has"

key-files:
  created:
    - internal/testharness/uplink_publisher.go
    - internal/testharness/vendor_axioma_w1.go
    - internal/testharness/vendor_acrel_adw300.go
    - internal/testharness/uplink_publisher_test.go
    - internal/testharness/scenarios.go
    - internal/cli/testharness.go
    - internal/cli/testharness_test.go
    - internal/cli/testharness_helpers_test.go
  modified:
    - internal/testharness/doc.go (rewritten — references DATA-06, D-27, W3)
    - internal/testharness/scenarios_test.go (replaced 12-line t.Skip placeholder with 6 named scenario tests)
    - internal/db/queries/measurements.sql (added CountMeasurementsByMP)
    - internal/db/sqlc/measurements.sql.go (regenerated)
    - internal/db/sqlc/querier.go (regenerated)
    - internal/cli/root.go (registered TestHarnessCmd)

key-decisions:
  - "Vendor builders emit codec-pinned decoded.object keys verbatim (B2 fix) — harness mapping rows seed json_pointer values matching codec result.* keys EXACTLY so normalize.go produces canonical values without invoking codec_js"
  - "publishOrInline branches on Publisher==nil: in-process mode invokes ingest.UplinkHandler synchronously (no barrier needed); broker mode publishes via paho + waits for the row count to advance (W3)"
  - "W3 timeout error message is fixed-string + broker URL: 'broker publish succeeded but ingest pipeline did not produce row in 10s — verify shifter serve is running and connected to <url>'"
  - "CLI subcommand does NOT seed fixtures (locates them) — operator must pre-create via UI; helps prevent the harness from masking 'install never connected to broker' failures by silently fabricating state"
  - "AxiomaW1 E2E maps /cumulative_l → raw_value (not cumulative_value): rollover detection reads raw_value from binding.last_raw_value; cumulative_value is computed by persist (raw + offset) — two birds, one mapping row"

patterns-established:
  - "Test transport duality (D-27): same scenario library runs against testcontainer infra in CI AND against a deployed install via CLI; scenarios remain identical — only RunInput.Publisher / IngestDeps differ"
  - "Sync barrier with operator-facing error: when crossing a process/transport boundary, polling-based barrier carries the boundary URL into the timeout message so operators don't have to grep logs"
  - "Codec result-key pinning: vendor builders + mapping seeders + assertions all reference the SAME pinned table; codec source is the source of truth, NOT migration SQL"

requirements-completed: [DATA-06, DATA-10]

# Metrics
duration: 35min
completed: 2026-05-04
---

# Phase 2 Plan 13: DATA-06 Synthetic Test Harness Summary

**Synthetic uplink harness with codec-pinned vendor builders, 5 named DATA-06 scenarios, AxiomaW1 E2E proof, W3 broker-mode sync barrier, and the `shifter test-harness` CLI subcommand for D-27 dual-entry operator validation.**

## Performance

- **Duration:** ~35 min
- **Started:** 2026-05-04T09:07:00Z
- **Completed:** 2026-05-04T09:42:47Z
- **Tasks:** 3
- **Files created:** 8
- **Files modified:** 6

## Accomplishments

- **DATA-06 proof shipped (the literal phase goal):** 5 named scenario tests run the full ingest pipeline against testcontainer Postgres+TimescaleDB; each asserts cumulative_value continuity AND audit row presence. The phase's ROADMAP success criterion #5 ("proven by synthetic-data tests covering every swap and rollover edge case") goes from 12 lines of t.Skip placeholder to 13 net-new tests.
- **DATA-10 proof shipped:** TestScenario_AxiomaW1_E2E pushes a real Axioma F1 V1.8-shaped payload through codec-mapped pointers → measurement row with cumulative_value=12345, battery_pct=87, temperature_c=23, raw_payload + decoded_object preserved (DATA-07).
- **D-27 dual-entry CLI:** `shifter test-harness <scenario> --vendor <slug>` lets an operator publish the same scenarios CI runs to a deployed install's MQTT broker. After install, an operator runs `shifter test-harness clean_swap` and observes the cumulative chart go through a swap without discontinuity — same code path nightly CI exercises.
- **W3 sync barrier:** broker-mode scenarios poll q.CountMeasurementsByMP every 100ms with a 10s ceiling; broker-down or ingest-down failures surface as a clear operator-facing error referencing the broker URL.
- **B2 fix verified:** vendor builders emit decoded.object keys matching axioma_w1.js / acrel_family.js result.* keys EXACTLY (`cumulative_l`, `battery_pct`, `temperature_c`, `leak`, `tamper` for Axioma; `kwh_forward`, `power_total_w`, voltage/current/pf_l1..3, etc. for Acrel ADW300). Mapping seeders use the same pointer keys; tests assert key presence.
- **264 short tests pass project-wide** (above 260 baseline before this plan).

## Task Commits

Each task was committed atomically:

1. **Task 1: testharness publisher + vendor uplink builders** — `7230b9e` (feat)
2. **Task 2: testharness scenarios — 5 D-27 named scenarios + AxiomaW1 E2E** — `770c384` (feat)
3. **Task 3: shifter test-harness CLI subcommand (D-27 dual-entry)** — `d5287f1` (feat)

## Files Created/Modified

### Created

- `internal/testharness/uplink_publisher.go` — paho MQTT publisher with 5s connect timeout + QoS=1 publish on canonical `application/<app>/device/<eui>/event/up` topic
- `internal/testharness/vendor_axioma_w1.go` — BuildAxiomaW1Uplink + BuildAxiomaW1UplinkForDevEUI; emits 5 codec-pinned object keys + raw F1 V1.8 frame in `data` (base64) + fPort=100
- `internal/testharness/vendor_acrel_adw300.go` — BuildAcrelADW300Uplink + ForDevEUI variant; emits 13-key 3-phase superset matching acrel_family.js result.*
- `internal/testharness/uplink_publisher_test.go` — 3 tests: TestPublisher_RoundTrip (Mosquitto + paho subscribe), TestBuildAxiomaW1Uplink_DecodedObjectShape (5 pinned keys), TestBuildAcrelADW300Uplink_DecodedObjectShape (13 pinned keys)
- `internal/testharness/scenarios.go` (~500 LoC) — Vendor / ScenarioName / Fixture / RunInput / Result types; 5 scenario builders + RunByName; publishOrInline + waitForMeasurementCount (W3); SeedAxiomaW1Mappings + SeedAcrelADW300Mappings; collectResult
- `internal/cli/testharness.go` — Cobra subcommand with --vendor / --broker / --application-id flags; lookupFixture + lookupCSApplicationID; "unknown scenario" early validation; W3 NewPublisher error wrapping
- `internal/cli/testharness_test.go` — 4 tests (Help, UnknownScenario, FlowAgainstTestcontainer, BrokerDownErrorMessage with 2 subtests)
- `internal/cli/testharness_helpers_test.go` — cliPadDevEUI (FNV-1a), cliSqlcLoader, buildCLITestIngestDeps, runMockMQTTConsumer

### Modified

- `internal/testharness/doc.go` — package doc rewritten; references DATA-06 + D-27 + W3 (was a placeholder pointing at plan-02-15)
- `internal/testharness/scenarios_test.go` — 12-line t.Skip placeholder replaced with 6 named scenario tests
- `internal/db/queries/measurements.sql` + regenerated sqlc — added CountMeasurementsByMP for the W3 sync barrier
- `internal/cli/root.go` — appended TestHarnessCmd to AddCommand list; Long help mentions 7 subcommands (was 6)

## Scenario Assertion Summary

| Scenario | Setup | Assertions |
|----------|-------|------------|
| `TestScenario_CleanSwap` | offset=0, last_raw=nil | 2 measurement rows; 1 swap audit; cumulative=10600 (=100+10500) |
| `TestScenario_SwapWithInflightUplink` | offset=0, last_raw=nil | 2 measurement rows; 1 swap audit; cumulative=5050 (=50+5000); no row dropped |
| `TestScenario_Rollover` | offset=0, last_raw=4294967200 | 1 measurement row; 0 swap; 1 rollover_detected audit; cumulative=4294967396 (=100+2^32) |
| `TestScenario_SwapAndRollover` | offset=0, last_raw=4294967200 | 2 measurement rows; 1 swap; 1 rollover audit; cumulative=4294967506 |
| `TestScenario_OverlappingUplinks` | offset=0, last_raw=nil | 5 measurement rows (2 outgoing + 3 incoming); 1 swap audit; cumulative=1160 (=60+1100) |
| `TestScenario_AxiomaW1_E2E` (DATA-10) | full pipeline single uplink | raw_value=12345, cumulative_value=12345, battery_pct=87, temperature_c=23, raw_payload+decoded_object non-empty, quality=ok |

## B2 Fix Confirmation

Vendor builders emit decoded.object keys matching axioma_w1.js + acrel_family.js result.* output **exactly**. The harness mapping seeders (`SeedAxiomaW1Mappings`, `SeedAcrelADW300Mappings`) use the SAME json_pointer values:

**Axioma W1 (5 mapping rows seeded):**

| json_pointer | canonical column | data_type |
|--------------|------------------|-----------|
| /cumulative_l | raw_value | numeric |
| /battery_pct | battery_pct | int |
| /temperature_c | temperature_c | numeric |
| /leak | leak_detected | bool |
| /tamper | tamper_detected | bool |

(Note: maps `/cumulative_l → raw_value` so rollover detection has a counter to compare; cumulative_value is computed by persist.go as raw + offset.)

**Acrel ADW300 (13 mapping rows seeded):** kwh_forward → raw_value, power_total_w → instant_value, battery_pct → battery_pct, temperature_c → temperature_c, voltage/current/pf_l1..3 → extra.* (DATA-08 hybrid wide+JSONB).

Pinned table re-verified against codec source files before Task 1 implementation; no discrepancies found between plan body and codec source.

## W3 Fix Confirmation

`waitForMeasurementCount` polls `q.CountMeasurementsByMP` every 100ms with a 10s ceiling. On timeout it returns:

> broker publish succeeded but ingest pipeline did not produce row in 10s — verify shifter serve is running and connected to `<broker URL>`

CLI test `TestTestHarnessCLI_BrokerDownErrorMessage` covers both failure modes:
- **connect_fail subtest:** `--broker tcp://127.0.0.1:1` (no listener) → NewPublisher returns connect error; CLI wraps as `mqtt connect to tcp://127.0.0.1:1: <root>`.
- **sync_barrier_timeout subtest:** Mosquitto up but no consumer subscribed → 10s W3 barrier fires; assertion checks the EXACT timeout message + broker URL.

## CLI Subcommand Verification

```
$ shifter --help
shifter is the self-hosted LoRaWAN water/electricity monitoring backend.

It wraps ChirpStack as the LoRaWAN Network Server and serves the Shifter
dashboard SPA. Subcommands:

  serve         Run migrations then start the HTTP server (D-13)
  migrate       Apply, force, or inspect database migrations (D-16)
  version       Print binary version
  create-admin  Create or reset an admin user (recovery — D-14)
  config-check  Validate config syntax and probe endpoints (D-07)
  healthcheck   Localhost HTTP GET /health (D-15, Docker HEALTHCHECK)
  test-harness  Publish synthetic DATA-06 scenarios to the broker (D-27)
```

```
$ shifter test-harness --help
Available scenarios:
  clean_swap                       — basic meter swap with offset continuity
  swap_with_inflight_uplink        — uplink racing with confirm_time
  rollover                         — counter wrap (raw < prev) auto-detected
  swap_then_rollover               — rollover then swap then post-uplink
  overlapping_uplinks_during_swap  — 5 uplinks during swap window
  ...
  --vendor string         vendor profile slug (axioma_w1 or acrel_adw300) (default "axioma_w1")
  --broker string         MQTT broker URL (defaults to config)
  --application-id string ChirpStack application UUID (defaults to chirpstack_connection.cs_application_id)
```

## Decisions Made

- **AxiomaW1 mapping `/cumulative_l → raw_value` (not cumulative_value):** the rollover detector reads raw_value from binding.last_raw_value to compare against the next uplink. Mapping cumulative_l directly to cumulative_value would skip the rollover path entirely. persist.go computes cumulative_value = raw + offset when no explicit cumulative_value mapping fires, so this single mapping satisfies BOTH rollover detection AND cumulative continuity.
- **CLI does NOT seed fixtures:** operator must pre-create site/MP/devices via the UI; CLI's `lookupFixture` finds them. Reasoning: a self-seeding harness would mask "deployed install hasn't been onboarded yet" failures, defeating the runbook-proof use case.
- **FNV-1a-based padDevEUI:** the existing `padDevEUI` in swap/commit_test.go uses a hex-only filter that collapses different inputs to the same dev_eui (e.g. "axiomacleanout" and "axiomacleanin" both → 00000000000aacea). Replaced with FNV-1a hash so distinct inputs yield distinct dev_euis; both internal/testharness/scenarios_test.go and internal/cli/testharness_helpers_test.go use the new helper.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] FNV-1a-based padDevEUI replaces hex-filter helper**
- **Found during:** Task 2 (running TestScenario_CleanSwap for the first time)
- **Issue:** The plan's referenced `padDevEUI` (mirrored from internal/swap/commit_test.go) strips non-hex chars then left-pads with zeros. With suffixes like "clean", "inflight", "rollover", different test fixtures generated the SAME 16-hex string (e.g. "axiomacleanout" and "axiomacleanin" both filtered to "aacea" → "00000000000aacea"), causing UNIQUE constraint violations on `device.dev_eui`.
- **Fix:** Rewrote `padDevEUI` to FNV-1a-hash the input then format as 16 lowercase hex chars. Distinct inputs reliably yield distinct dev_euis. Same fix applied to internal/cli/testharness_helpers_test.go (`cliPadDevEUI`).
- **Files modified:** internal/testharness/scenarios_test.go (Task 2 commit), internal/cli/testharness_helpers_test.go (Task 3 commit)
- **Verification:** All 6 TestScenario_* tests pass against testcontainer Postgres without the duplicate-key error.
- **Committed in:** 770c384 (Task 2), d5287f1 (Task 3)

**2. [Rule 2 - Missing critical] CountMeasurementsByMP sqlc query added**
- **Found during:** Task 2 (writing waitForMeasurementCount per the plan's W3 spec)
- **Issue:** The plan body referenced `q.CountMeasurementsByMP` for the W3 sync barrier, but the query did not exist in `internal/db/queries/measurements.sql`. Without it, scenarios cannot poll for ingest progress in broker mode.
- **Fix:** Added `CountMeasurementsByMP :one` query + comment referencing W3 + Plan 02-13; ran `sqlc generate` to regenerate `measurements.sql.go` + `querier.go`.
- **Files modified:** internal/db/queries/measurements.sql, internal/db/sqlc/measurements.sql.go, internal/db/sqlc/querier.go
- **Verification:** `go build ./...` passes; W3 sync barrier compiles and timeout test asserts the expected error.
- **Committed in:** 770c384 (Task 2)

**3. [Rule 2 - Missing critical] CLI builds binary in test 5 to catch root.go AddCommand regressions**
- **Found during:** Task 3 (writing the CLI test surface)
- **Issue:** A package-level test on `TestHarnessCmd.RunE` directly does NOT exercise root.go's AddCommand registration; a refactor that drops `TestHarnessCmd` from the AddCommand chain would pass the package tests but break the binary.
- **Fix:** Added `TestTestHarnessBinary_HelpListsTestHarness` which `go build`s the binary into a temp dir and asserts `shifter --help` lists the subcommand.
- **Verification:** Binary builds + lists test-harness in --help output.
- **Committed in:** d5287f1 (Task 3)

---

**Total deviations:** 3 auto-fixed (1 blocking, 2 missing-critical)
**Impact on plan:** All deviations were necessary for correctness. Padding-helper rewrite is a defensive improvement that benefits any future test using the same pattern. CountMeasurementsByMP plugs an explicit gap between plan body and existing sqlc surface. Binary-help test catches a registration regression class the package tests can't see.

## Issues Encountered

None beyond the deviations documented above. Builder + scenario logic compiled and passed first time after the dev_eui collision was fixed.

## Next Phase Readiness

Plan 02-15 can now flip VALIDATION.md DATA-06 + DATA-10 from ⬜ pending to ✅ green by mapping to the TestScenario_* commands documented above:
- DATA-06 → `go test ./internal/testharness/... -run "TestScenario_(CleanSwap|SwapWithInflightUplink|Rollover|SwapAndRollover|OverlappingUplinks)"` (5 scenario test names)
- DATA-10 → `go test ./internal/testharness/... -run TestScenario_AxiomaW1_E2E`

Operators on a deployed install can additionally validate via `shifter test-harness clean_swap --vendor axioma_w1` (D-27 dual-entry runbook).

## Self-Check: PASSED

- internal/testharness/uplink_publisher.go — FOUND
- internal/testharness/vendor_axioma_w1.go — FOUND
- internal/testharness/vendor_acrel_adw300.go — FOUND
- internal/testharness/scenarios.go — FOUND
- internal/cli/testharness.go — FOUND
- internal/cli/testharness_test.go — FOUND
- internal/cli/testharness_helpers_test.go — FOUND
- Commit 7230b9e — FOUND
- Commit 770c384 — FOUND
- Commit d5287f1 — FOUND

---
*Phase: 02-domain-model-canonical-schema*
*Completed: 2026-05-04*
