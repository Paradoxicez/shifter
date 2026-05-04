---
phase: 2
slug: domain-model-canonical-schema
status: approved
nyquist_compliant: true
wave_0_complete: true
created: 2026-05-03
approved: 2026-05-04
---

# Phase 2 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.
> Source: `.planning/phases/02-domain-model-canonical-schema/02-RESEARCH.md` §Validation Architecture and the 10 plan files (02-01 .. 02-10).

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Backend framework** | Go stdlib `testing` + `testify/assert` (already installed in Phase 1) |
| **Backend integration** | `testcontainers-go` Postgres+TimescaleDB and Mosquitto; `mockgen` for ChirpStack gRPC interfaces |
| **Frontend framework** | `vitest` (Vite-native) + `@testing-library/react` (already installed in Phase 1) |
| **Backend config file** | `go.mod` (no separate test config); migrations applied per test via `db.RunMigrations` |
| **Frontend config file** | `web/vitest.config.ts` (extends `vite.config.ts`) |
| **Quick run command** | `go test ./internal/<package> -short -race && pnpm --dir web test <path> --run` |
| **Full suite command** | `go test ./... -race -count=1 && pnpm --dir web test --run && pnpm --dir web build && just lint` |
| **Estimated runtime** | ~150 seconds (full suite incl. testcontainer cold start; testharness scenarios in 02-09 add ~20s) |

---

## Sampling Rate

- **After every task commit:** Run `go test ./<package> -race && pnpm --dir web test <relevant-files> --run`
- **After every plan wave:** Run `go test ./... -race -count=1 && pnpm --dir web test --run && pnpm --dir web build && just lint`
- **Before `/gsd-verify-work`:** Full suite must be green; the synthetic-data test harness (DATA-06) must pass all 5 scenarios
- **Max feedback latency:** 30 seconds (quick run, hot testcontainers)

---

## Per-Task Verification Map

> Each Phase 2 requirement is mapped to at least one automated assertion. File paths are placeholders that the executor will create per the owning plan.

| Req ID | Behavior | Test Type | Automated Command | File Owner | Status |
|--------|----------|-----------|-------------------|------------|--------|
| SITE-01 | Admin can create site via dialog (POST /api/sites) | integration | `go test ./internal/site -run TestCreateSite_AdminAllowed` | 02-10 | ✅ green |
| SITE-01 | Viewer cannot mutate sites (403) | integration | `go test ./internal/site -run TestCreateSite_ViewerForbidden` | 02-10 | ✅ green |
| SITE-01 | Site archive/restore is soft-delete (archived_at toggles) | integration | `go test ./internal/site -run TestArchiveAndRestoreSite` | 02-10 | ✅ green |
| SITE-01 | Create-site dialog renders with lat/lng fields | component | `pnpm --dir web test routes/sites/create-site-dialog.test.tsx` | 02-10 / 02-14 | ✅ green |
| DATA-01 | Migration 0007/0008 creates `site` and `metering_point` with UUID PKs | integration | `go test ./internal/db -run TestRunMigrations_Clean` | 02-02 | ✅ green |
| DATA-01 | Measurement hypertable PK is `(metering_point_id, time)` — never `dev_eui` | integration | `go test ./internal/db -run TestRunMigrations_Clean` (asserts is_hypertable + invariant via `command grep -iE 'dev_eui\|device_id' internal/db/migrations/0015_measurement.up.sql \| command grep -v -- '--'` returning 0 lines) | 02-02 / 02-04 | ✅ green |
| DATA-01 | Resolver maps incoming `dev_eui` → active `metering_point_id` via binding | integration | `go test ./internal/resolver -run TestLookup_Miss_LoadsFromLoader` + `go test ./internal/cli -run TestServe_FullBoot_MQTTUplinkPersists` | 02-04 / 02-12 | ✅ green |
| DATA-02 | Binding has `(valid_from, valid_to, reading_offset)` columns | integration | `go test ./internal/db -run TestBinding_HalfOpenInterval` | 02-02 / 02-03 | ✅ green |
| DATA-02 | Resolver picks the binding active at uplink time, not at query time | integration | `go test ./internal/resolver -run TestLookup_HistoricalAtBeforeValidFrom` | 02-04 | ✅ green |
| DATA-02 | Resolver cache invalidates on binding change via LISTEN/NOTIFY | integration | `go test ./internal/resolver -run TestListener_InvalidatesOnNotify` + `TestListener_InvalidatesOnTriggerFire` | 02-04 / 02-07 | ✅ green |
| DATA-03 | Hypertable `time` column is server-set (now()), not from payload | integration | `go test ./internal/ingest -run TestUplinkHandler_DataTimeIsServerSide` + `go test ./internal/cli -run TestServe_FullBoot_MQTTUplinkPersists` | 02-04 / 02-12 | ✅ green |
| DATA-03 | `gateway_rx_time` and `device_time` persisted as diagnostics columns | integration | `go test ./internal/ingest -run TestDecodeChirpStackEvent_EarliestGatewayRxTime` + `go test ./internal/db -run TestAppendMeasurement_RoundTrip` | 02-04 / 02-06 | ✅ green |
| DATA-04 | Swap math: proposed offset such that displayed cumulative is continuous | unit | `go test ./internal/swap -run TestProposeOffset_HappyPath` | 02-07 | ✅ green |
| DATA-04 | Swap commit: closes prior binding (valid_to=now), opens new (valid_from=now) atomically | integration | `go test ./internal/swap -run TestCommitSwap_HappyPath` | 02-07 | ✅ green |
| DATA-04 | Concurrent swap commits on same MP: one wins, the other returns 409 | integration | `go test ./internal/swap -run TestCommitSwap_ConcurrentOneWins` + `go test ./internal/swap -run TestSwapHandler_Concurrent_409` | 02-07 / 02-11 | ✅ green |
| DATA-04 | Swap dialog reads outgoing reading R, lets admin confirm proposed offset | component | `pnpm --dir web test routes/metering-points/swap-meter-dialog.test.tsx` | 02-10 / 02-14 | ✅ green |
| DATA-05 | Counter rollover detected when raw < previous; offset advances by `counter_modulus` | unit | `go test ./internal/swap -run TestDetectRollover_True` + `TestApplyRollover_Add2to32` | 02-07 | ✅ green |
| DATA-05 | Rollover event is logged in audit_log | integration | `go test ./internal/ingest -run TestPersist_RolloverDetected` | 02-07 / 02-09 | ✅ green |
| DATA-06 | Synthetic harness: clean swap | integration | `go test ./internal/testharness -run TestScenario_CleanSwap` | 02-13 | ✅ green |
| DATA-06 | Synthetic harness: swap with concurrent in-flight uplink | integration | `go test ./internal/testharness -run TestScenario_SwapWithInflightUplink` | 02-13 | ✅ green |
| DATA-06 | Synthetic harness: rollover | integration | `go test ./internal/testharness -run TestScenario_Rollover` | 02-13 | ✅ green |
| DATA-06 | Synthetic harness: swap + rollover combined | integration | `go test ./internal/testharness -run TestScenario_SwapAndRollover` | 02-13 | ✅ green |
| DATA-06 | Synthetic harness: overlapping uplinks during swap window | integration | `go test ./internal/testharness -run TestScenario_OverlappingUplinks` | 02-13 | ✅ green |
| DATA-07 | Ingest persists raw payload, decoded `object`, and canonical fields — none lost | integration | `go test ./internal/ingest -run TestPersist_PreservesRawAndDecodedAcrossQualityLevels` + `go test ./internal/cli -run TestServe_FullBoot_MQTTUplinkPersists` | 02-04 / 02-12 | ✅ green |
| DATA-08 | Measurement row has canonical columns (cumulative, flow_rate, voltage, current, battery_pct, rssi, snr) + JSONB `extra` | integration | `go test ./internal/db -run TestAppendMeasurement_RoundTrip` | 02-02 / 02-06 | ✅ green |
| DATA-08 | Normalize step writes vendor-specific fields to `extra` JSONB, not lost | unit | `go test ./internal/ingest -run TestNormalizeMeasurement_ExtraFields` | 02-04 | ✅ green |
| DATA-09 | Profile editor saves field-mapping JSON (decoded path → canonical column) | integration | `go test ./internal/profile -run TestSaveProfile_Update` + `go test ./internal/profile -run TestProfileHandler_Update_PreservesSlug` | 02-08 / 02-11 | ✅ green |
| DATA-09 | New profile mapping takes effect without backend restart (resolver re-reads) | integration | `go test ./internal/profile -run TestRunSeedSync_FreshInstall` + listener invalidation tests cover hot-reload pathway | 02-08 | ✅ green |
| DATA-09 | Mapping editor UI: click decoded JSON tree → assigns canonical column | component | `pnpm --dir web test routes/profiles/mapping-editor.test.tsx` | 02-08 / 02-14 | ✅ green |
| DATA-10 | Migration 0010 seeds 3 vendor profiles (Axioma W1, Acrel ADL200, Acrel ADW300) | integration | `go test ./internal/profile -run TestRunSeedSync_FreshInstall` | 02-02 / 02-08 | ✅ green |
| DATA-10 | At least one profile (Axioma W1) has codec + mapping wired end-to-end on real synthetic uplink | integration | `go test ./internal/testharness -run TestScenario_AxiomaW1_E2E` | 02-09 / 02-13 | ✅ green |
| AUDIT-01 | Every site/MP/device/profile mutation writes audit_log row inside the same txn | integration | `go test ./internal/audit -run TestWriteEntry_AtomicWithRollback` + `go test ./internal/audit -run TestWriteEntry_RoundTrip` | 02-07 / 02-08 | ✅ green |
| AUDIT-01 | audit_log captures (user, ts, entity, before, after) — diff is field-level | unit | `go test ./internal/audit -run TestChangedFields_FieldChanged` + `TestChangedFields_FieldRemoved` + `TestChangedFields_FieldAdded` | 02-07 | ✅ green |
| AUDIT-01 | Meter swap writes a single audit_log row of type `meter.swap` | integration | `go test ./internal/swap -run TestCommitSwap_HappyPath` (asserts audit row written) + `TestSwapHandler_OperatorOverride_PersistsInAudit` | 02-07 / 02-11 | ✅ green |
| CHIRP-04 | Add-device handler creates CS tenant→application→profile→device→keys then Shifter row in one user action | integration | `go test ./internal/device -run TestAddDevice_HappyPath_NoBinding` + `TestAddDevice_HappyPath_WithBinding` + `go test ./internal/cli -run TestServe_FullBoot_DegradedMode_NoCS` | 02-05 / 02-10 / 02-12 | ✅ green |
| CHIRP-04 | If CS step fails mid-flight, rollback unwinds the partial CS state (best-effort) and Shifter row is not written | integration | `go test ./internal/device -run TestAddDevice_CSFailure_RollsBack` + `TestAddDevice_CSKeysFailure_DeletesCSDevice` + `TestAddDevice_PostgresFailure_DeletesCSDevice` | 02-05 / 02-10 | ✅ green |
| CHIRP-04 | DevEUI parser surfaces MSB+LSB interpretations + OUI vendor hint | unit | `go test ./internal/device -run TestParseDevEUI_AxiomaOUIHint` + `TestParseDevEUI_AcrelOUIHint` + `TestParseDevEUI_HappyPath_MSB` | 02-10 | ✅ green |
| CHIRP-04 | Add-device dialog is a single one-action submit (no multi-page wizard) | component | `pnpm --dir web test routes/devices/add-device-dialog.test.tsx` + `pnpm --dir web test routes/devices/deveui-parser.test.tsx` | 02-10 / 02-14 | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

> Plan 02-01 IS Wave 0 — it scaffolds every Phase 2 test file as a failing skeleton before any implementation lands.

**Backend test files (stubs created by 02-01):**
- [x] `internal/site/handlers_test.go` — site CRUD + soft-delete + RBAC
- [x] `internal/meteringpoint/handlers_test.go` — MP CRUD + active binding view
- [x] `internal/device/handlers_test.go` — device add atomic + decommission
- [x] `internal/device/deveui_test.go` — DevEUI parse (MSB/LSB + OUI)
- [x] `internal/swap/math_test.go` — proposed offset, rollover detect
- [x] `internal/swap/commit_test.go` — atomic close+open, concurrent commit
- [x] `internal/ingest/handler_test.go` — MQTT uplink end-to-end
- [x] `internal/ingest/normalize_test.go` — canonical column extraction + JSONB extra
- [x] `internal/resolver/cache_test.go` — time-windowed binding lookup, cache invalidation
- [x] `internal/resolver/listener_test.go` — Postgres LISTEN/NOTIFY pickup
- [x] `internal/profile/editor_test.go` — mapping save + hot reload
- [x] `internal/profile/seed_test.go` — three seed profiles present at boot
- [x] `internal/audit/log_test.go` — same-transaction insert
- [x] `internal/audit/diff_test.go` — field-level diff
- [x] `internal/testharness/scenarios_test.go` — 5 DATA-06 scenarios
- [x] `internal/chirpstack/tenant_test.go` — tenant get-or-create
- [x] `internal/chirpstack/application_test.go` — application get-or-create
- [x] `internal/chirpstack/device_profile_test.go` — profile get-or-create
- [x] `internal/chirpstack/device_test.go` — device create + keys
- [x] `internal/chirpstack/bootstrap_test.go` — CHIRP-04 atomic orchestrator

**Frontend test files (stubs created by 02-01):**
- [x] `web/src/routes/sites/create-site-dialog.test.tsx`
- [x] `web/src/routes/devices/add-device-dialog.test.tsx`
- [x] `web/src/routes/devices/deveui-parser.test.tsx`
- [x] `web/src/routes/metering-points/swap-meter-dialog.test.tsx`
- [x] `web/src/routes/profiles/mapping-editor.test.tsx`

**Shared test support (updated by 02-01):**
- [x] `internal/testsupport/mosquitto.go` — testcontainer Mosquitto helper for ingest path tests (Phase 1 helper repinned from `eclipse-mosquitto:2.0.18` to `eclipse-mosquitto:2.0.20` per T-02-01-01)

**Framework install:** none new — Phase 1 already installed testify, testcontainers-go, vitest, @testing-library/react. Plan 02-01 Task 1 added `@tanstack/react-table` 8.x to `web/package.json` (runtime dep for downstream Phase 2 data-table UIs; not a test framework but recorded here for the Wave 0 audit trail).

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Cumulative chart shows no visible discontinuity after admin confirms swap | DATA-04 | "Continuity" is a perceptual claim about the rendered chart; the math is asserted in unit tests, but the visual check that the line doesn't jump belongs in `/gsd-verify-work` | After committing a swap via dialog on a populated MP, open the per-meter detail (Phase 4 dashboard) and confirm the cumulative line crosses the swap timestamp without a vertical step |
| Mapping editor UX "feels conversational" — tree-click is preferred path, manual JSON pointer is escape hatch | DATA-09 | Tone/UX subjective per UI-SPEC; lint cannot catch "feels overwhelming" | Reviewer walks through mapping a new profile end-to-end, confirms the tree-click path completes without ever opening the manual JSON editor for the canonical fields |
| Add-device dialog completes in a single submit on real ChirpStack (not bufconn mock) | CHIRP-04 | bufconn covers protocol correctness; final acceptance is round-trip against the bundled ChirpStack v4 container | `/gsd-verify-work` operator-driven test on the bundled compose stack |

---

## Validation Sign-Off

- [x] All Phase 2 requirements (SITE-01, DATA-01..10, AUDIT-01, CHIRP-04) have ≥1 automated assertion above
- [x] Sampling continuity: every plan owns at least one row in the verification map (no 3 consecutive plans without automated verify)
- [x] Wave 0 (plan 02-01) covers all MISSING test-file references
- [x] No watch-mode flags (`--run` is explicit on vitest, `-count=1` on go test)
- [x] Feedback latency < 30s for `quick run` path
- [x] `nyquist_compliant: true` set in frontmatter
- [x] **Gap-closure complete (Plan 02-15, 2026-05-04):** All 38 ⬜ pending rows flipped to ✅ green pointing at the actual landed test names. Evidence: `go test ./... -race -count=1` 383 passed / 0 failed; `pnpm --dir web test --run` 51 passed / 0 failed; `pnpm --dir web build` clean; `TestScenario_*` 6/6 PASS via W4 per-test loop (zero MISSING); `TestServe_FullBoot_*` 4/4 PASS; `command grep -iE 'dev_eui\|device_id' internal/db/migrations/0015_measurement.up.sql \| command grep -v -- '--'` returns 0 lines (DATA-01 invariant intact).

**Approval:** approved 2026-05-04 (initial) — re-approved 2026-05-04 (gap-closure complete after Plans 02-11..14 landed; Plan 02-15 reconciled per-task verification map to actual landed test names; all rows ✅ green).
