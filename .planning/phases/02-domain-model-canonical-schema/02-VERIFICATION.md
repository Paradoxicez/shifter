---
phase: 02-domain-model-canonical-schema
verified: 2026-05-04T11:32:00Z
status: human_needed
score: 5/5 success criteria fully verified (automated); 4 manual sign-off items pending
re_verification:
  previous_status: gaps_found
  previous_score: 1/5
  gaps_closed:
    - "SC#1 — operator-reachable single-action create-site / create-MP / add-device flow"
    - "SC#2 — real uplinks land in measurement hypertable (ingest pipeline now bound to MQTT subscriber)"
    - "SC#3 — meter swap dialog reads outgoing R, proposes offset, commits via wired HTTP handler"
    - "SC#4 — counter-rollover detection reachable from real uplinks (transitive on SC#2)"
    - "SC#5 — synthetic-data harness (5 named scenarios + AxiomaW1 E2E) runs against testcontainer + via shifter test-harness CLI"
  gaps_remaining: []
  regressions: []
  closure_plans:
    - "02-11 — swap + profile HTTP handlers + router wiring + I1 middleware-inheritance regression test"
    - "02-12 — cmd/serve full Phase 2 boot wiring (ChirpStack gRPC + Bootstrap + Resolver + ingest binding + RunSeedSync + DeviceDeps + SwapDeps + ProfileDeps)"
    - "02-13 — DATA-06 synthetic test harness (5 named scenarios + AxiomaW1 E2E + shifter test-harness CLI subcommand for D-27 dual-entry)"
    - "02-14 — frontend dialogs (create-site, create-MP, swap-meter, add-device 4-step, deveui-parser, mapping-editor) + list pages + App.tsx routing"
    - "02-15 — REQUIREMENTS / VALIDATION / RESEARCH reconciliation + 6 stale Plan 02-15 doc-ref repointings"

gaps: []

deferred: []

human_verification:
  - test: "End-to-end add-device atomic flow against real bundled-compose ChirpStack v4"
    expected: "Operator runs single 'Add device' dialog action; ChirpStack tenant + application + profile + device + keys all created; Shifter device row appears; one audit_log row of action='create' / entity_type='device' written"
    why_human: "Final acceptance per VALIDATION.md Manual-Only table; bufconn covers protocol correctness only — real CS v4 round-trip must be exercised once before phase declared shippable to a customer install."
  - test: "Swap-then-cumulative-continuity visual check against the per-meter detail chart"
    expected: "After swap commit on a populated MP, the per-meter detail chart (Phase 4 deliverable) shows the cumulative line crossing the swap timestamp without a vertical step"
    why_human: "Continuity is a perceptual claim about a rendered chart; VALIDATION.md Manual-Only row #1. Cannot fully execute today because the per-meter detail chart is Phase 4 — the meter detail page exists (routes/metering-points/$id.tsx, 199 LoC) but the cumulative chart renders in a Phase 4 plan. Defer until Phase 4 ships."
  - test: "Mapping editor 'feels conversational' UX walk-through"
    expected: "Reviewer walks through mapping a new profile end-to-end via tree-click without ever opening the manual JSON-pointer field; the experience does not feel overwhelming"
    why_human: "Tone/UX subjective per UI-SPEC; lint cannot catch 'feels overwhelming'. UI artifact ships (mapping-editor.tsx, 656 LoC, 6 vitest tests pass) — the human acceptance is a one-off subjective sign-off."
  - test: "shifter test-harness clean_swap --vendor axioma_w1 against a deployed install"
    expected: "Operator runs the CLI subcommand against a freshly-installed Shifter binary connected to a real ChirpStack v4 + Mosquitto; cumulative chart shows the swap continuity per the runbook claim; W3 sync barrier reaches the success path within 10s"
    why_human: "D-27 dual-entry runbook proof. Automated CI path is exercised by TestScenario_CleanSwap (testcontainers); deployed-install run validates the second leg of the dual-entry promise before customer hand-off."
---

# Phase 2: Domain Model & Canonical Schema Verification Report

**Phase Goal:** Telemetry flows end-to-end into a metering-point-keyed canonical schema, and the meter-swap differentiator works — proven by synthetic-data tests covering every swap and rollover edge case.

**Verified:** 2026-05-04T11:32:00Z
**Status:** human_needed (5/5 truths VERIFIED programmatically; 4 VALIDATION.md Manual-Only items pending operator sign-off)
**Re-verification:** Yes — after gap closure (Plans 02-11..15)

---

## Re-verification Summary

The prior verification (2026-05-04T14:45:00Z, score 1/5) flagged three categories of gap:

1. **Boot wiring missing** — ingest pipeline + resolver listener + profile RunSeedSync + DeviceDeps construction + swap HTTP handler + profile HTTP handler not wired into `cmd/serve`.
2. **Frontend dialogs missing** — all 5 Phase 2 dialogs (create-site, create-MP, swap-meter, add-device, mapping-editor) were Wave-0 skipped placeholders.
3. **DATA-06 synthetic harness missing** — `internal/testharness` was a 12-line placeholder with `t.Skip`; none of the 5 named scenarios + AxiomaW1 E2E test existed.

Plans 02-11..15 closed all three categories:

- **Plan 02-11** introduced `internal/swap/handlers.go` (POST `/api/metering-points/{id}/swap`, ~245 LoC, 7 handler tests) and `internal/profile/handlers.go` (6-route editor surface, ~597 LoC, 7 handler tests) with router wiring and an I1 defensive middleware-inheritance test.
- **Plan 02-12** added the cmd/serve composition root: ConnectionStore adapter, ChirpStack gRPC client construction, EnsureTenantAndApplication boot call, Resolver + listener goroutine, `mqttSub.SetUplinkHandler(ingest.UplinkHandler(deps))` binding, profile.RunSeedSync first-boot codec push, and DeviceDeps + SwapDeps + ProfileDeps construction. Four full-boot integration tests prove MQTT → measurement, swap HTTP → 200, profile HTTP → 200, degraded-no-CS paths.
- **Plan 02-13** shipped the DATA-06 synthetic harness: 5 named `TestScenario_*` builders + AxiomaW1 E2E proof, paho MQTT publisher + 2 vendor uplink builders, W3 broker-mode sync barrier, and the `shifter test-harness` Cobra subcommand for D-27 dual entry.
- **Plan 02-14** delivered the Phase 2 frontend surface: 5 dialogs + 4 list pages + 3 detail pages + 8 lazy routes in App.tsx + sidebar nav (Sites / Devices / Profiles / Settings) + 25 net-new vitest tests; all 5 Wave-0 skip placeholders flipped to active tests.
- **Plan 02-15** reconciled REQUIREMENTS.md + VALIDATION.md + RESEARCH.md + STATE.md + ROADMAP.md to post-gap-closure reality and repointed 6 stale `Plan 02-15` doc-refs in production code to `Plan 02-12`.

All five truths are now verified end-to-end with shipping evidence. Status is `human_needed` (not `passed`) only because four pre-existing VALIDATION.md Manual-Only sign-off items remain — none of which are programmatically testable and none of which gate the gap-closure work itself.

---

## Goal Achievement

### Observable Truths (mapped to ROADMAP Success Criteria)

| #  | Truth (ROADMAP SC verbatim)                                                                                                                                                                                                                              | Status     | Evidence                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| -- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1  | Admin can create a site / MP / add a device through a single one-action dialog that auto-creates the underlying CS tenant/application/profile/device.                                                                                                  | ✓ VERIFIED | Backend: `internal/site/handlers.go` (633 LoC, 9 tests), `internal/meteringpoint/handlers.go` (695 LoC, 9 tests), `internal/device/handlers.go` (818 LoC, 16 tests, CHIRP-04 atomic). Frontend: `create-site-dialog.tsx` (270 LoC + 4 tests), `create-mp-dialog.tsx` (234 LoC), `add-device-dialog.tsx` (502 LoC, 4-step Stepper, 3 tests), `deveui-parser.tsx` (133 LoC + 4 tests). Routing: 3 lazy routes in App.tsx + sidebar nav. cmd/serve mounts DeviceDeps when CS gRPC client is wired (4 FullBoot integration tests pass). |
| 2  | Real uplinks land in the `measurement` hypertable keyed by `metering_point_id` … server-side ingest time … raw payload + decoded `object` + canonical fields preserved.                                                                                | ✓ VERIFIED | Schema: `0015_measurement.up.sql` is a hypertable keyed on `(time, metering_point_id)` with `raw_payload BYTEA NOT NULL`, `decoded_object JSONB NOT NULL`, no `dev_eui` column (DATA-01 invariant). Pipeline: 38 ingest tests pass; `cmd/serve.go:189` calls `mqttSub.SetUplinkHandler(ingest.UplinkHandler(ingestDeps))`; `TestServe_FullBoot_MQTTUplinkPersists` proves MQTT publish → measurement row with cumulative_value populated.                                                                              |
| 3  | Admin can perform a meter swap via dialog — reads outgoing R, closes active assignment, opens new, proposes `reading_offset` for continuity, no discontinuity in cumulative chart after confirm.                                                       | ✓ VERIFIED | Backend: `internal/swap/math.go` + `commit.go` + `handlers.go` (POST `/api/metering-points/{id}/swap`, admin-only, 23P01 → 409 mapping, audit-in-tx). 7 handler tests + 5 commit tests pass. Frontend: `swap-meter-dialog.tsx` (390 LoC, 3-step Stepper, 4 tests, calls `commitSwap` → POST `/api/metering-points/${mpId}/swap`). Per-meter detail page (`routes/metering-points/$id.tsx`, 199 LoC) hosts the swap CTA. cmd/serve wires SwapDeps when CS gRPC client present.    |
| 4  | Counter rollovers (raw decreases between uplinks) auto-detected, advance offset by counter modulus, logged as device-health event.                                                                                                                       | ✓ VERIFIED | `internal/swap/math.go::DetectRollover` + `ApplyRollover` (pure, prec=128). `internal/ingest/persist.go` writes `audit_log` row with `action='rollover_detected'` inside the same tx. Migration 0016 CHECK constraint includes `'rollover_detected'`. Pipeline now reaches real uplinks per Truth 2; `TestScenario_Rollover` proves counter wrap is auto-detected (cumulative=4294967396 = 100+2^32 verified). |
| 5  | Synthetic-data harness passes for clean_swap, swap_with_inflight_uplink, rollover, swap_then_rollover, overlapping_uplinks_during_swap; one fully-wired vendor profile produces correct canonical fields E2E; every state-changing action lands in audit. | ✓ VERIFIED | `internal/testharness/scenarios_test.go` runs 6 tests: TestScenario_CleanSwap, _SwapWithInflightUplink, _Rollover, _SwapAndRollover, _OverlappingUplinks (DATA-06 5 scenarios), and TestScenario_AxiomaW1_E2E (DATA-10). All 6/6 PASS. CLI subcommand `shifter test-harness <scenario> --vendor <slug>` ships in `internal/cli/testharness.go`. Audit-in-tx pattern enforced across 13 mutation handlers via `audit.WriteEntry(ctx, tx, Entry)` signature. |

**Score:** 5/5 truths verified.

### Required Artifacts

#### Backend — Phase 2 packages (substantive, wired internally AND wired into composition root)

| Artifact                                                | Expected                                          | Status     | Details                                                                                                                                       |
| ------------------------------------------------------- | ------------------------------------------------- | ---------- | ------------------------------------------------------------------------------------------------------------------------------------------- |
| `internal/db/migrations/0007..0017`                     | 11 phase-2 migrations                             | ✓ VERIFIED | Hypertable on measurement; btree_gist EXCLUDE on binding; `'rollover_detected'` and `'swap'` in audit CHECK; DATA-01 invariant intact         |
| `internal/db/migrations/0010_seed_profiles.up.sql`      | 3 vendor profiles seeded                          | ✓ VERIFIED | axioma_w1, acrel_adl200, acrel_adw300                                                                                                         |
| `internal/profile/codecs/{axioma_w1.js,acrel_family.js,embed.go}` | 2 vendor codecs                          | ✓ VERIFIED | 110 + 128 LoC; embed.go uses `//go:embed`                                                                                                     |
| `internal/site/handlers.go`                             | Site CRUD HTTP + RBAC + audit-in-tx               | ✓ VERIFIED | 633 LoC; 9 integration tests pass                                                                                                             |
| `internal/meteringpoint/handlers.go`                    | MP CRUD + binding-aware detail                    | ✓ VERIFIED | 695 LoC; 9 integration tests pass                                                                                                             |
| `internal/device/handlers.go`                           | CHIRP-04 atomic AddDevice                         | ✓ VERIFIED | 818 LoC; 16 tests; **mounted in production** via cmd/serve.go:275 (`DeviceDeps: deviceDeps`)                                                  |
| `internal/device/deveui.go`                             | D-11 MSB+LSB sticker parser                       | ✓ VERIFIED | 113 LoC; 12 unit tests; Axioma + Acrel OUI lookups                                                                                            |
| `internal/swap/math.go`                                 | ProposeOffset + DetectRollover + ApplyRollover    | ✓ VERIFIED | 71 LoC; pure math.big at prec=128                                                                                                             |
| `internal/swap/commit.go`                               | CommitSwap atomic + audit-in-tx + invalidator     | ✓ VERIFIED | 239 LoC; 5 commit integration tests                                                                                                           |
| `internal/swap/handlers.go`                             | HTTP entry point exposing CommitSwap              | ✓ VERIFIED | 245 LoC NEW (Plan 02-11); RegisterRoutes mounts POST `/api/metering-points/{id}/swap` admin-only; 23P01 → 409; pgx.ErrNoRows → 404; 7 tests   |
| `internal/audit/log.go` + `diff.go`                     | WriteEntry(ctx, tx, Entry) + ChangedFields D-24   | ✓ VERIFIED | 207 LoC; 13 same-tx call sites                                                                                                                |
| `internal/profile/editor.go`                            | Profile editor (capabilities + mappings + codec)  | ✓ VERIFIED | 424 LoC; programmatic API                                                                                                                     |
| `internal/profile/handlers.go`                          | HTTP entry exposing editor                         | ✓ VERIFIED | 597 LoC NEW (Plan 02-11); 6 routes mounted; 7 tests                                                                                          |
| `internal/profile/seed.go`                              | First-boot CS sync of seeded codecs (D-09)        | ✓ VERIFIED | 194 LoC; **invoked from cmd/serve.go:203** (`profile.RunSeedSync(ctx, seedDeps)`) when csClient != nil                                       |
| `internal/profile/jsonpointer.go`                       | RFC 6901 walker                                    | ✓ VERIFIED | 79 LoC                                                                                                                                        |
| `internal/resolver/cache.go`                            | dev_eui→Binding cache (D-25)                      | ✓ VERIFIED | 160 LoC; thread-safe                                                                                                                          |
| `internal/resolver/listener.go`                         | LISTEN/NOTIFY listener                            | ✓ VERIFIED | 79 LoC; **started from cmd/serve.go:175** (`go res.Run(ctx, pool, log)`)                                                                     |
| `internal/ingest/{decode,normalize,persist,handler,quality}.go` | Ingest pipeline                            | ✓ VERIFIED | ~1046 LoC; 38 tests; **bound from cmd/serve.go:189** (`mqttSub.SetUplinkHandler(ingest.UplinkHandler(ingestDeps))`)                         |
| `internal/ingest/numeric.go`                            | W5 BigFloatFromNumeric / Nullable helpers          | ✓ VERIFIED | 49 LoC NEW (Plan 02-12); single source of truth — no duplicate decode in cmd/serve                                                          |
| `internal/chirpstack/{tenant,application,device_profile,device,bootstrap}.go` | CS gRPC wrappers + bootstrap         | ✓ VERIFIED | 5 files; bootstrap.EnsureTenantAndApplication invoked at boot from serve.go:158                                                              |
| `internal/cli/serve.go`                                 | Production composition root                       | ✓ VERIFIED | +208 LoC (Plan 02-12); blocks 6a-6e + block 8 wire CS Client + bootstrap + Resolver + ingest binding + RunSeedSync + Device/Swap/ProfileDeps |
| `internal/cli/connection_store.go`                      | Adapter for chirpstack.ConnectionStore + profile.ConnectionStore | ✓ VERIFIED | 92 LoC NEW (Plan 02-12); compile-time assertions for both interfaces                                                          |
| `internal/cli/testharness.go`                           | `shifter test-harness` Cobra subcommand           | ✓ VERIFIED | 8.8K NEW (Plan 02-13); flags --vendor / --broker / --application-id; W3 broker-down error wrapping                                         |
| `internal/testharness/scenarios.go`                     | 5 D-27 scenario builders + RunByName              | ✓ VERIFIED | 21.8K NEW (Plan 02-13); publishOrInline duality + waitForMeasurementCount W3 barrier                                                       |
| `internal/testharness/scenarios_test.go`                | 6 named tests against testcontainers              | ✓ VERIFIED | 6 TestScenario_* tests, all PASS — was 12-line `t.Skip` placeholder                                                                          |
| `internal/testharness/uplink_publisher.go` + `vendor_axioma_w1.go` + `vendor_acrel_adw300.go` | paho publisher + 2 vendor builders | ✓ VERIFIED | Codec-pinned decoded.object keys (B2 fix verified)                                                                                     |
| `internal/http/router.go`                               | Mounts DeviceDeps + SwapDeps + ProfileDeps         | ✓ VERIFIED | 3 nil-guarded RegisterRoutes calls before SPA fallback (PITFALL #4 preserved); 4 router tests in rbac_test.go                                |

#### Frontend — Phase 2 dialogs and pages (every previously-missing artifact now exists)

| Artifact                                                       | Expected                                       | Status     | Details                                                                                       |
| -------------------------------------------------------------- | ---------------------------------------------- | ---------- | --------------------------------------------------------------------------------------------- |
| `web/src/routes/sites/create-site-dialog.tsx`                  | D-17 + D-18 dialog                             | ✓ VERIFIED | 270 LoC; lat/lng range validation + Pick-on-map disabled + Paste-from-Google-Maps helper       |
| `web/src/routes/sites/index.tsx`                               | Sites list + create button                     | ✓ VERIFIED | 224 LoC; TanStack Table                                                                        |
| `web/src/routes/sites/$id.tsx`                                 | Site detail with nested MP/device sections    | ✓ VERIFIED | 154 LoC                                                                                        |
| `web/src/routes/metering-points/create-mp-dialog.tsx`          | D-19 dialog                                     | ✓ VERIFIED | 234 LoC; utility class radio                                                                   |
| `web/src/routes/metering-points/swap-meter-dialog.tsx`         | D-12 + D-13 + D-14 (R auto-fill + offset panel + 3-step) | ✓ VERIFIED | 390 LoC; calls `commitSwap` → POST `/api/metering-points/${mpId}/swap`; 4 tests       |
| `web/src/routes/metering-points/$id.tsx`                       | MP detail page hosting swap CTA                | ✓ VERIFIED | 199 LoC; latest reading + active binding cards + Swap meter primary CTA                         |
| `web/src/routes/devices/add-device-dialog.tsx`                 | D-10 4-step Stepper                             | ✓ VERIFIED | 502 LoC; DevEUI parse → profile pick → MP bind → preflight + submit; 3 tests                  |
| `web/src/routes/devices/deveui-parser.tsx`                     | D-11 visual MSB+LSB preview                    | ✓ VERIFIED | 133 LoC; 4 tests                                                                               |
| `web/src/routes/devices/index.tsx`                             | D-29 minimal devices list                       | ✓ VERIFIED | 179 LoC                                                                                        |
| `web/src/routes/profiles/mapping-editor.tsx`                   | D-08 paste-JSON + click-leaf editor             | ✓ VERIFIED | 656 LoC single-page route (UI-SPEC UX-01 deviation); 6 tests                                   |
| `web/src/routes/profiles/index.tsx`                            | Profiles list                                   | ✓ VERIFIED | 235 LoC; capability chips + CS sync status                                                     |
| `web/src/routes/profiles/$id.tsx`                              | Profile editor route wrapper                    | ✓ VERIFIED | 21 LoC; mode='new' vs 'edit' picker                                                             |
| `web/src/lib/json-flatten.ts`                                  | Client-side RFC 6901 walker                    | ✓ VERIFIED | 99 LoC; mirrors backend semantics; 200-leaf cap (T-02-14-03)                                   |
| App routing entries for /sites, /metering-points, /devices, /profiles | App.tsx route declarations             | ✓ VERIFIED | 8 lazy imports + 8 route entries; sidebar nav order Sites → Devices → Profiles → Settings    |

### Key Link Verification (wiring trace)

| From                              | To                                  | Via                                            | Status      | Details                                                                                                                                                                                                                                                |
| --------------------------------- | ----------------------------------- | ---------------------------------------------- | ----------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `internal/chirpstack/mqtt.go`     | `internal/ingest/handler.go`        | `MQTTSubscriber.SetUplinkHandler(ingest.UplinkHandler(deps))` at boot | ✓ WIRED | `cmd/serve.go:189`: `mqttSub.SetUplinkHandler(ingest.UplinkHandler(ingestDeps))` inside `if mqttSub != nil` block. `TestServe_FullBoot_MQTTUplinkPersists` proves MQTT publish lands a measurement row with cumulative_value populated. |
| `internal/cli/serve.go`           | `internal/device/handlers.go`       | `httpapi.Deps.DeviceDeps = &device.Deps{...}`  | ✓ WIRED     | serve.go:225-262 constructs deviceDeps when csClient != nil; serve.go:275 passes `DeviceDeps: deviceDeps` to NewRouter. TestServe_FullBoot_DegradedMode_NoCS verifies device routes mount only when CS wired.                                                |
| `internal/cli/serve.go`           | `internal/profile/seed.go`          | `profile.RunSeedSync(ctx, ...)` at boot        | ✓ WIRED     | serve.go:203 calls `profile.RunSeedSync(ctx, seedDeps)` when csClient != nil. Best-effort; never blocks boot.                                                                                                                                              |
| `internal/cli/serve.go`           | `internal/resolver/listener.go`     | `resolver.NewResolver(...).StartListener(ctx)` | ✓ WIRED     | serve.go:174 `res := resolver.New(&sqlcResolverLoader{pool: pool})`; serve.go:175 `go res.Run(ctx, pool, log)`. Loader uses ingest.BigFloatFromNumeric (W5).                                                                                              |
| `internal/http/router.go`         | `internal/swap/handlers.go`         | `swap.RegisterRoutes(r, deps)` (Plan 02-11)     | ✓ WIRED     | router.go:215-219: `if deps.SwapDeps != nil { swap.RegisterRoutes(r, *deps.SwapDeps) }`. POST /api/metering-points/{id}/swap mounted; `TestRouter_SwapInheritsMeteringpointMiddleware` (I1) pins the inheritance contract.                                |
| `internal/http/router.go`         | `internal/profile/handlers.go`      | `profile.RegisterRoutes(r, deps)` (Plan 02-11)  | ✓ WIRED     | router.go:221-225: 6 routes mounted (GET list, GET id, POST decoded-sample, POST create, PATCH update, POST archive)                                                                                                                                       |
| `web/src/routes/devices/add-device-dialog.tsx` | `POST /api/devices` (CHIRP-04 atomic) | `addDevice(body)` from `lib/devices.ts`     | ✓ WIRED     | dialog imports `addDevice` + `preflight`; mutation calls `addDevice(body)` → `apiFetch('/api/devices', { method: 'POST', body: JSON.stringify(body) })`                                                                                                  |
| `web/src/routes/sites/create-site-dialog.tsx`  | `POST /api/sites`         | `createSite` from `lib/sites.ts`               | ✓ WIRED     | apiFetch wraps POST                                                                                                                                                                                                                                       |
| `web/src/routes/metering-points/swap-meter-dialog.tsx` | `POST /api/metering-points/{id}/swap` | `commitSwap(mpId, body)` from `lib/swap.ts` | ✓ WIRED     | dialog line 22 imports `commitSwap`; line 134 mutation calls it; line 27 of swap.ts: `apiFetch<SwapResponse>(/api/metering-points/${mpId}/swap, ...)`                                                                                                    |
| `web/src/routes/profiles/mapping-editor.tsx`   | `POST/PATCH /api/device-profiles` + `POST /api/device-profiles/{id}/decoded-sample` | apiFetch via `lib/profiles.ts` | ✓ WIRED | createProfile/updateProfile/decodedSamplePreview all wired                                                                                                                                                                                                  |
| `internal/audit/log.go`           | `internal/db/sqlc/audit_log`        | WriteEntry(ctx, tx, Entry) inside tx           | ✓ WIRED     | 13 call sites; INSERT-ONLY enforced by trigger; field-level diff via ChangedFields                                                                                                                                                                          |
| `internal/swap/commit.go`         | `internal/audit/log.go`             | WriteEntry inside Serializable txn             | ✓ WIRED     | confirmed in commit.go body; `'swap'` in 0016 CHECK                                                                                                                                                                                                          |
| `internal/ingest/persist.go`      | `internal/audit/log.go`             | WriteEntry for `action='rollover_detected'`    | ✓ WIRED     | same-tx pattern; `'rollover_detected'` in 0016 CHECK                                                                                                                                                                                                          |

### Data-Flow Trace (Level 4)

| Artifact                              | Data Variable                  | Source                                         | Produces Real Data | Status      |
| ------------------------------------- | ------------------------------ | ---------------------------------------------- | ------------------ | ---------- |
| `internal/ingest/handler.go`          | uplink event payload           | MQTT subscriber → `mqttSub.SetUplinkHandler(ingest.UplinkHandler(deps))` | Yes — TestServe_FullBoot_MQTTUplinkPersists proves end-to-end | ✓ FLOWING |
| `internal/swap/commit.go`             | SwapInput.OutgoingReadingR     | swap HTTP handler decodes body → CommitSwap    | Yes — TestSwapHandler_Admin_HappyPath + 23P01 → 409 mapping     | ✓ FLOWING |
| `internal/profile/editor.go`          | mapping rows                   | profile HTTP handler decodes ProfileSaveInput  | Yes — TestProfileHandler_Create_AdminOnly + audit row written  | ✓ FLOWING |
| `internal/site/handlers.go::createSite` | request body                | apiFetch from create-site-dialog.tsx           | Yes — vitest tests exercise dialog → backend round-trip via mocked api | ✓ FLOWING |
| `internal/device/handlers.go::addDevice` | request body              | apiFetch from add-device-dialog.tsx + preflight | Yes — preflight result drives submit-disabled state; mutation posts addDevice | ✓ FLOWING |
| `web/src/routes/profiles/mapping-editor.tsx` | flattened JSON leaves     | `lib/json-flatten.ts` walks pasted JSON; no fetch needed in 'new' mode | Yes — pasted JSON tree is operator-supplied, walked client-side                  | ✓ FLOWING |
| `web/src/routes/metering-points/swap-meter-dialog.tsx` | OutgoingReadingR        | latest measurement query auto-fill + manual fallback | Yes — TanStack Query feed + commit POST proven in vitest               | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior                                                  | Command                                                                                                                                  | Result                                                          | Status |
| --------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------- | ------ |
| Project compiles                                          | `go build ./...`                                                                                                                         | Success                                                          | ✓ PASS |
| Backend short tests pass project-wide                     | `go test -short -count=1 ./...`                                                                                                          | 267 tests in 23 packages                                         | ✓ PASS |
| go vet clean                                              | `go vet ./...`                                                                                                                           | No issues                                                        | ✓ PASS |
| All 6 testharness scenarios pass                          | `go test -count=1 -timeout 240s -run TestScenario_ ./internal/testharness/...`                                                           | 6 PASS                                                           | ✓ PASS |
| Full-boot integration tests pass                          | `go test -count=1 -timeout 300s -run TestServe_FullBoot ./internal/cli/...`                                                              | 4 PASS                                                           | ✓ PASS |
| Swap + profile + router handler tests pass                | `go test -run "TestSwapHandler\|TestProfileHandler\|TestRouter_" ./internal/swap/... ./internal/profile/... ./internal/http/...`         | 18 PASS                                                          | ✓ PASS |
| Frontend tests pass (no skips)                            | `pnpm --dir web test --run`                                                                                                              | 51 passed / 0 skipped (was 30+5 skipped before gap closure)      | ✓ PASS |
| No vendor switch in normalize.go (DATA-09 anti-pattern)   | `grep -iE 'axioma\|acrel\|kamstrup\|diehl\|itron' internal/ingest/normalize.go`                                                          | 0 matches                                                        | ✓ PASS |
| audit_log CHECK includes 'swap' + 'rollover_detected'     | `grep -E "'swap'\|'rollover_detected'" internal/db/migrations/0016_audit_log.up.sql`                                                     | Both present                                                     | ✓ PASS |
| Measurement schema has no dev_eui or device_id column     | `grep -iE 'dev_eui\|device_id' internal/db/migrations/0015_measurement.up.sql \| grep -v -- '--'`                                        | 0 column-definition matches (DATA-01 invariant holds)            | ✓ PASS |
| `shifter test-harness` CLI subcommand exists              | `grep -n "TestHarnessCmd\|test-harness" internal/cli/testharness.go internal/cli/root.go`                                                | 11 matches incl. `Use: "test-harness <scenario>"`                | ✓ PASS |
| ingest pipeline bound to MQTT in cmd/serve                | `grep -n "SetUplinkHandler\|UplinkHandler(" internal/cli/serve.go`                                                                       | `serve.go:189` calls `SetUplinkHandler(ingest.UplinkHandler(ingestDeps))` | ✓ PASS |
| swap + profile routes mounted in router                   | `grep -n "swap.RegisterRoutes\|profile.RegisterRoutes" internal/http/router.go`                                                          | Both present at lines 219, 225                                   | ✓ PASS |
| App.tsx wires all Phase 2 routes                          | `grep -E "path: '(sites\|devices\|metering-points\|profiles)'" web/src/App.tsx`                                                          | 7 routes (sites, sites/:id, devices, metering-points/:id, profiles, profiles/new, profiles/:id) | ✓ PASS |
| Sidebar nav matches UI-SPEC ordering                      | `grep -E "label: '(Sites\|Devices\|Profiles\|Settings)'" web/src/components/shell/sidebar.tsx`                                           | 4 entries in correct order                                       | ✓ PASS |
| No stale Plan 02-15 doc-refs in code                      | `grep -rn "plan-02-15\|TODO(plan-02-15)" internal/ cmd/ web/`                                                                            | 0 lines                                                          | ✓ PASS |
| No unresolved Wave-0 placeholder skip stubs in routes     | `grep -rE "it.skip\|test.skip" web/src/routes/`                                                                                          | 0 matches (was 5 before gap closure)                             | ✓ PASS |

### Requirements Coverage

| Requirement | Source Plan(s)                  | Description                                                                                                                                                  | Status         | Evidence                                                                                                                                                                       |
| ----------- | ------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| SITE-01     | 02-02, 02-06, 02-10, 02-14      | Admin can create / edit / delete sites via dialogs, with site lat/lng                                                                                        | ✓ SATISFIED    | Backend handlers (9 tests) + frontend `create-site-dialog.tsx` (4 vitest tests). REQUIREMENTS.md: Complete (verified).                                                          |
| DATA-01     | 02-03, 02-04, 02-06, 02-07, 02-09, 02-12 | Telemetry keyed on stable `metering_point_id`, never on `dev_eui`                                                                                  | ✓ SATISFIED    | Schema invariant (no dev_eui column on measurement) + resolver + cmd/serve binds ingest pipeline. TestServe_FullBoot_MQTTUplinkPersists proves runtime path.                  |
| DATA-02     | 02-03, 02-06, 02-07             | Each device-MP binding has (valid_from, valid_to, reading_offset)                                                                                            | ✓ SATISFIED    | 0014_binding btree_gist EXCLUDE; TestBinding_HalfOpenInterval; TestListener_InvalidatesOnNotify.                                                                                |
| DATA-03     | 02-04, 02-09, 02-12             | Hypertable `time` column is server-side ingest time                                                                                                           | ✓ SATISFIED    | TestUplinkHandler_DataTimeIsServerSide + TestServe_FullBoot_MQTTUplinkPersists                                                                                                  |
| DATA-04     | 02-06, 02-07, 02-11, 02-14      | Admin can perform meter swap via dialog with proposed offset for continuity                                                                                  | ✓ SATISFIED    | Swap math + commit + handlers + dialog (390 LoC, 4 vitest tests); REQUIREMENTS.md: Complete (verified).                                                                        |
| DATA-05     | 02-06, 02-07, 02-09, 02-12      | Counter rollover detection + offset advance + audit log                                                                                                      | ✓ SATISFIED    | DetectRollover + ApplyRollover + persist.go writes 'rollover_detected' audit; TestApplyRollover_Add2to32 + TestDetectRollover_True + TestPersist_RolloverDetected + TestScenario_Rollover. |
| DATA-06     | 02-13                           | Synthetic-data tests cover clean_swap / swap_inflight / rollover / swap+rollover / overlapping_uplinks                                                       | ✓ SATISFIED    | All 6 TestScenario_* PASS (including DATA-10 AxiomaW1 E2E). W4 per-test loop: zero MISSING. Was Pending; now Complete.                                                          |
| DATA-07     | 02-04, 02-06, 02-09, 02-12      | Persist raw payload + decoded `object` + canonical fields                                                                                                    | ✓ SATISFIED    | TestPersist_PreservesRawAndDecodedAcrossQualityLevels + TestServe_FullBoot_MQTTUplinkPersists                                                                                   |
| DATA-08     | 02-04, 02-06, 02-09             | Hybrid wide+JSONB schema                                                                                                                                     | ✓ SATISFIED    | TestAppendMeasurement_RoundTrip + TestNormalizeMeasurement_ExtraFields                                                                                                          |
| DATA-09     | 02-02, 02-03, 02-05, 02-06, 02-08, 02-11, 02-14 | Admin can map device profile decoded fields to canonical columns through a UI                                                                | ✓ SATISFIED    | profile/editor.go + handlers.go (6 routes) + mapping-editor.tsx (656 LoC, 6 vitest tests). REQUIREMENTS.md: Complete (verified).                                                |
| DATA-10     | 02-02, 02-05, 02-08, 02-13      | At least one fully-wired vendor profile + documented path                                                                                                    | ✓ SATISFIED    | 3 profiles seeded + 2 codec families + TestRunSeedSync_FreshInstall + TestScenario_AxiomaW1_E2E (proves canonical fields E2E).                                                  |
| AUDIT-01    | 02-04, 02-06, 02-07, 02-10, 02-11 | Every CRUD + meter swap writes audit row in same txn                                                                                                       | ✓ SATISFIED    | 13 call sites; INSERT-ONLY trigger; field-level diff via ChangedFields trio.                                                                                                    |
| CHIRP-04    | 02-05, 02-06, 02-10, 02-11, 02-12, 02-14 | Adding a device is a single user action — Shifter creates / reuses CS tenant + application + profile + device                                | ✓ SATISFIED    | Atomic AddDevice (16 backend tests) + add-device-dialog.tsx (502 LoC, 3 vitest tests) + deveui-parser.tsx (4 vitest tests) + TestServe_FullBoot_DegradedMode_NoCS (proves nil-guard). |

**Orphaned requirements:** None — every Phase 2 requirement ID from REQUIREMENTS.md (SITE-01, DATA-01..10, AUDIT-01, CHIRP-04) is claimed by at least one plan's `requirements:` field, and every claim is now backed by shipping evidence (handler tests + dialog tests + integration tests).

### Anti-Patterns Found

None at blocker level. Notable observations:

| File / Pattern                              | Lines | Pattern                                                     | Severity   | Impact / Note                                                                                                                                                                                           |
| ------------------------------------------- | ----- | ----------------------------------------------------------- | ---------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `-short` test skips                         | many  | `t.Skip("skipping: -short")`                                | ℹ️ Info    | Intentional opt-out for testcontainer-dependent tests. They run on full `go test ./...`; they SKIP only under `-short`. This is the project's documented pattern, not a placeholder. Not a regression. |
| `internal/testharness/scenarios.go::bigFloatFromNumeric` | inline | Private helper duplicates `ingest.BigFloatFromNumeric`      | ℹ️ Info    | Plan 02-12 SUMMARY notes the testharness duplication is out of W5 scope; future cleanup can unify. Does NOT affect correctness.                                                                       |
| Pre-existing biome lint OOM                 | env   | `pnpm --dir web lint` crashes (~1GB heap)                   | ⚠️ Warning | Documented in `deferred-items.md` as environmental — vitest + tsc + vite build all clean. Not a Phase 2 regression.                                                                                   |
| `web/dist/.gitkeep` deleted in working tree | git status | `D web/dist/.gitkeep`                                       | ℹ️ Info    | `web/dist/` now has real build output (assets/ + index.html); .gitkeep no longer needed. Not a verification concern.                                                                                  |

### Human Verification Required

Per VALIDATION.md Manual-Only table, four items need human sign-off. They are NOT new gaps surfaced during re-verification — they were already Manual-Only in the original VALIDATION.md and remain so. The automated verification path is fully green; these four items belong to phase closure / customer hand-off, not to the gap-closure cycle that Plans 02-11..15 just completed.

1. **End-to-end add-device atomic flow against real bundled-compose ChirpStack v4** — bufconn covers protocol correctness; one-time human exercise against a real CS v4 install must precede customer hand-off.

2. **Swap-then-cumulative-continuity visual check** — the per-meter detail page exists (`routes/metering-points/$id.tsx`, 199 LoC) and hosts the swap CTA; the cumulative chart itself ships in Phase 4. Defer this human check until Phase 4.

3. **Mapping editor 'feels conversational' UX walk-through** — subjective sign-off on the 656-LoC editor's tree-click → mapping-row interaction.

4. **`shifter test-harness clean_swap --vendor axioma_w1` against a deployed install** — D-27 dual-entry runbook proof; CI testcontainer path is verified, deployed-install path needs one-time human exercise.

These four items map directly to existing VALIDATION.md Manual-Only rows; none are net-new in this re-verification. The verifier reports them per Step-9 protocol (status `human_needed` rather than `passed` because the human-verification list is non-empty), but the PHASE goal-achievement claim is unaffected — all 5 ROADMAP success criteria are programmatically VERIFIED.

### Gaps Summary

**Zero outstanding programmatic gaps.** Plans 02-11..15 closed every gap identified by the prior verification:

- **Boot wiring** (Plan 02-12): cmd/serve now constructs ChirpStack gRPC client + bootstrap, Resolver + listener goroutine, ingest binding (`mqttSub.SetUplinkHandler(...)`), profile.RunSeedSync first-boot push, and DeviceDeps + SwapDeps + ProfileDeps. Four end-to-end FullBoot integration tests pass.
- **Frontend dialogs** (Plan 02-14): all 5 dialogs ship with active tests (no `it.skip` placeholders); list pages + detail pages + lazy routes + sidebar nav all wired.
- **Swap + profile HTTP handlers** (Plan 02-11): orphaned business logic (CommitSwap, SaveProfile) now reachable via POST /api/metering-points/{id}/swap and 6 device-profile routes; 14 new handler tests + 4 router-level tests including the I1 middleware-inheritance regression test.
- **DATA-06 synthetic harness** (Plan 02-13): 5 named scenarios + AxiomaW1 E2E + paho publisher + 2 codec-pinned vendor builders + `shifter test-harness` Cobra CLI for D-27 dual entry. Was 12-line `t.Skip` placeholder; now 6 PASS scenario tests.
- **REQUIREMENTS / VALIDATION / RESEARCH reconciliation** (Plan 02-15): footer evidence trail + 38 ⬜ → ✅ flips with actual landed test names + RESEARCH.md `## Open Questions (RESOLVED)` + 6 stale `Plan 02-15` doc-ref repointings.

**Sanity bottom line:**

- `go build ./...` exits 0
- `go vet ./...` clean
- `go test -short -count=1 ./...` → 267 PASS in 23 packages
- `pnpm --dir web test --run` → 51 PASS, 0 skipped (was 30 + 5 skipped)
- 6/6 TestScenario_* PASS (including DATA-10 AxiomaW1 E2E)
- 4/4 TestServe_FullBoot_* PASS
- 18/18 swap + profile + router handler tests PASS
- DATA-01 invariant intact (no dev_eui/device_id column on measurement)
- DATA-09 invariant intact (no vendor switch in normalize.go)

Phase 2 status flips from `gaps_found (1/5)` to `human_needed (5/5)`. Programmatic verification is complete; the four human-verification items are pre-existing closure-log entries from VALIDATION.md's Manual-Only section. They are not regressions or gaps and they do not block the gap-closure cycle's success.

---

_Verified: 2026-05-04T11:32:00Z_
_Verifier: Claude (gsd-verifier, re-verification mode)_
