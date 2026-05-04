---
phase: 02-domain-model-canonical-schema
verified: 2026-05-04T14:45:00Z
status: gaps_found
score: 1/5 success criteria fully verified
gaps:
  - truth: "Admin can create a site (with site lat/lng), create a metering point on that site, and add a device through a single one-action dialog that auto-creates the underlying ChirpStack tenant/application/profile/device behind the scenes."
    status: partial
    reason: "Backend HTTP handlers (site/MP/device) exist and pass integration tests, but the FRONTEND dialogs (create-site, create-MP, add-device) are not implemented — only Wave 0 placeholder test stubs exist with `it.skip('TODO(plan-02-12/13/14): implement after <dialog>.tsx exists')`. Plans 02-12/13/14 do not exist (Phase 2 only has 02-01..02-10). Additionally, the device routes never mount in production: cmd/serve does not construct or pass DeviceDeps to the router (router.go line 189 — `if deps.DeviceDeps != nil` is unreachable in production)."
    artifacts:
      - path: "web/src/routes/sites/create-site-dialog.test.tsx"
        issue: "Skipped placeholder; create-site-dialog.tsx does not exist"
      - path: "web/src/routes/devices/add-device-dialog.test.tsx"
        issue: "Skipped placeholder; add-device-dialog.tsx does not exist"
      - path: "web/src/routes/metering-points/swap-meter-dialog.test.tsx"
        issue: "Skipped placeholder; swap-meter-dialog.tsx does not exist"
      - path: "web/src/routes/profiles/mapping-editor.test.tsx"
        issue: "Skipped placeholder; mapping-editor.tsx does not exist"
      - path: "web/src/routes/devices/deveui-parser.test.tsx"
        issue: "Skipped placeholder; deveui-parser.tsx does not exist"
      - path: "internal/cli/serve.go"
        issue: "DeviceDeps is never constructed (cs gRPC client + Bootstrapper not wired); device routes therefore not mounted in production. Comment at L7 + L77 still says 'Phase 2+ adds device handlers' but Plan 02-15 wiring step never landed."
    missing:
      - "web/src/routes/sites/create-site-dialog.tsx (D-17 + D-18: lat/lng inputs + helper text + disabled 'Pick on map' button)"
      - "web/src/routes/sites/<list page>.tsx (operator-visible list of sites with create button)"
      - "web/src/routes/metering-points/create-mp-dialog.tsx (D-19 fields: name/site_id/utility_class/location_description)"
      - "web/src/routes/metering-points/swap-meter-dialog.tsx (D-12 + D-13: outgoing R auto-fill + reading offset proposal panel)"
      - "web/src/routes/devices/add-device-dialog.tsx (D-10 4-step stepped dialog reusing Phase 1 Stepper)"
      - "web/src/routes/devices/deveui-parser.tsx (D-11 component for MSB+LSB preview + OUI hint)"
      - "web/src/routes/profiles/mapping-editor.tsx (D-08 paste-JSON + click-leaf + TanStack Table)"
      - "Routing entries in web/src/App.tsx (or wherever routes are declared) for /sites, /sites/:id, /metering-points/:id, /devices, /profiles, /profiles/:id"
      - "cmd/serve wiring (Plan 02-15 equivalent): construct chirpstack.Client, device.Deps with Bootstrap+CSDeviceClient, set httpapi.Deps.DeviceDeps before NewRouter()"
      - "HTTP handler for swap commit — internal/swap has business logic but NO RegisterRoutes / chi route. The MP handlers do not expose POST /api/metering-points/{id}/swap. Without this, even a hand-rolled SPA dialog cannot drive the swap flow."
      - "HTTP handlers for profile editor — internal/profile has editor.go (programmatic API) but NO RegisterRoutes / chi route. Mapping editor dialog (DATA-09) cannot be wired to anything without backend route surface."

  - truth: "Real uplinks land in the `measurement` hypertable keyed by `metering_point_id` (never by `dev_eui`) with the server-side ingest time as the authoritative `time` column, and the row preserves raw payload, decoded `object`, and canonical normalized fields."
    status: partial
    reason: "Schema (DATA-01/03/07/08) is correct AND ingest pipeline (decode/normalize/persist) compiles + passes 38 tests. BUT the ingest UplinkHandler is never bound to the MQTT subscriber at runtime. cmd/serve.go starts MQTTSubscriber with `nil` handler (line 82) and there is no SetUplinkHandler(ingest.UplinkHandler(...)) call anywhere. Comment at line 76-77 says 'Phase 2 swaps in normalize+persist via the same Plan 13 hook point' — Plan 02-15 wiring did not land. Production binary will receive uplinks and silently drop them via the default stdout-log handler."
    artifacts:
      - path: "internal/ingest/handler.go"
        issue: "UplinkHandler exists and is correct, but no caller in cmd/serve binds it to MQTTSubscriber.SetUplinkHandler"
      - path: "internal/cli/serve.go"
        issue: "Lines 78-89: MQTTSubscriber starts with nil handler; never gets SetUplinkHandler call. ingest package not imported"
      - path: "internal/chirpstack/mqtt.go"
        issue: "SetUplinkHandler method exists (added by Plan 02-09) but production never calls it — the Plan 02-09 SUMMARY itself acknowledges 'cmd/serve boot wiring (Plan 02-15)' as the missing step"
    missing:
      - "Import ingest in cmd/serve, construct ingest.Deps (Pool + MappingStore + Resolver + Log + audit hooks)"
      - "Construct resolver.Resolver and start its LISTEN/NOTIFY listener goroutine"
      - "Call mqttSub.SetUplinkHandler(ingest.UplinkHandler(deps)) before <-ctx.Done() blocks"
      - "Boot-time profile.RunSeedSync invocation (D-09 first-boot CS sync)"

  - truth: "Admin can perform a meter swap via dialog — the dialog reads the outgoing reading R, closes the active assignment, opens a new assignment, proposes a `reading_offset` for continuity, and the cumulative chart shows no discontinuity after admin confirms."
    status: failed
    reason: "Backend swap math (internal/swap/math.go: ProposeOffset, DetectRollover, ApplyRollover) and atomic commit (internal/swap/commit.go: CommitSwap with audit-in-same-tx) exist and 5 integration tests pass. BUT (a) NO HTTP handler exposes CommitSwap — internal/swap has no handlers.go and no RegisterRoutes; (b) the swap-meter-dialog.tsx is a Wave 0 skipped placeholder with NO implementation; (c) the meter-detail page that would host the dialog does not exist. The differentiator that defines the entire phase ('meter-swap differentiator works') has zero user-reachable surface."
    artifacts:
      - path: "internal/swap/commit.go"
        issue: "CommitSwap function exists but has no HTTP entry point — orphaned business logic"
      - path: "web/src/routes/metering-points/swap-meter-dialog.test.tsx"
        issue: "`it.skip('TODO(plan-02-14): implement after swap-meter-dialog.tsx exists')` — dialog file does not exist"
    missing:
      - "internal/swap/handlers.go with RegisterRoutes mounting POST /api/metering-points/{id}/swap (or equivalent)"
      - "Router wiring for swap routes in internal/http/router.go"
      - "web/src/routes/metering-points/swap-meter-dialog.tsx implementing D-12 (R auto-fill) + D-13 (offset proposal panel)"
      - "Per-meter detail page that hosts the 'Swap meter' button (mentioned only in 02-CONTEXT and Phase 4 plan but referenced as interaction surface here)"

  - truth: "Synthetic-data test harness passes for clean swap, swap with concurrent in-flight uplink, rollover, swap+rollover, and overlapping uplinks during swap; one fully-wired vendor profile produces correct canonical fields end-to-end and every state-changing action lands in the audit table."
    status: failed
    reason: "DATA-06 is unfulfilled. internal/testharness/scenarios_test.go is a 12-line Wave-0 placeholder (TestPlaceholder calls t.Skip). NONE of the 5 named scenarios (CleanSwap, SwapWithInflightUplink, Rollover, SwapAndRollover, OverlappingUplinks) exist in any package. NO TestScenario_AxiomaW1_E2E test exists. The CLI subcommand `shifter test-harness <scenario>` (D-27) does not exist in internal/cli/. REQUIREMENTS.md correctly marks DATA-06 as Pending, but ROADMAP success criterion #5 is gated on these tests passing. The phase goal in ROADMAP is verbatim: 'proven by synthetic-data tests covering every swap and rollover edge case' — the very proof the phase claims to deliver does not exist."
    artifacts:
      - path: "internal/testharness/scenarios_test.go"
        issue: "12-line placeholder with TestPlaceholder calling t.Skip; no scenario tests"
      - path: "internal/testharness/doc.go"
        issue: "doc.go literally says 'Phase 2 placeholder. See plan-02-15 for body.' Plan 02-15 does not exist"
      - path: "internal/cli/<test-harness command>"
        issue: "No test-harness Cobra command exists in internal/cli — `shifter test-harness clean_swap --vendor axioma_w1` is undeliverable"
    missing:
      - "internal/testharness/scenarios.go — scenario builders (clean_swap, swap_with_inflight_uplink, rollover, swap_then_rollover, overlapping_uplinks_during_swap)"
      - "internal/testharness/scenarios_test.go — 5 TestScenario_* + TestScenario_AxiomaW1_E2E running each scenario through testcontainers Mosquitto + Postgres+TimescaleDB and asserting cumulative continuity / audit row presence"
      - "internal/cli/testharness.go — Cobra subcommand publishing scenarios to a deployed install's MQTT broker (D-27 dual entry point)"
      - "Synthetic uplink publishers for axioma_w1 + acrel_adw300 codec families"

  - truth: "Counter rollovers (raw reading decreases between consecutive uplinks) are auto-detected, advance the offset by the counter modulus, and are logged as a device-health event."
    status: partial
    reason: "Logic is correct: internal/swap/math.go DetectRollover + ApplyRollover; internal/ingest/persist.go calls them and writes audit_log row with action='rollover_detected' inside the same tx. Migration 0016 has 'rollover_detected' in the action CHECK constraint. Unit tests pass. HOWEVER, this is a transitive failure: because ingest is not wired into the MQTT subscriber (Truth 2 above), the rollover code path is unreachable from real uplinks at runtime. Once ingest is wired, this becomes ✓ VERIFIED."
    artifacts:
      - path: "internal/swap/math.go"
        issue: "Functions exist and tested. Wired to persist.go. But ingest pipeline never runs in production"
      - path: "internal/cli/serve.go"
        issue: "Same root cause as Truth 2 — ingest pipeline never bound to MQTT subscriber"
    missing:
      - "Same as Truth 2 — wire ingest.UplinkHandler to MQTTSubscriber.SetUplinkHandler in cmd/serve"

deferred: []

human_verification:
  - test: "(blocked) End-to-end add-device atomic flow against real bundled compose ChirpStack"
    expected: "Operator runs single 'Add device' dialog action; ChirpStack tenant + application + profile + device + keys all created; Shifter device row appears; one audit_log row of action='create' / entity_type='device' written"
    why_human: "Final acceptance per VALIDATION.md Manual-Only table; bufconn covers protocol correctness only. CANNOT EXECUTE because the add-device dialog UI is missing — block this human test until UI gap closure."
  - test: "(blocked) Swap-then-cumulative-continuity visual check"
    expected: "After swap commit on a populated MP, per-meter detail chart shows the cumulative line crossing the swap timestamp without a vertical step"
    why_human: "Continuity is a perceptual claim about the rendered chart; VALIDATION.md Manual-Only row #1. CANNOT EXECUTE because (a) swap dialog UI missing, (b) per-meter detail chart is Phase 4."
  - test: "(blocked) Mapping editor 'feels conversational' UX walk-through"
    expected: "Reviewer walks through mapping a new profile end-to-end via tree-click without ever opening the manual JSON pointer field for canonical columns"
    why_human: "Tone/UX subjective per UI-SPEC; lint cannot catch 'feels overwhelming'. CANNOT EXECUTE because mapping-editor.tsx does not exist."
---

# Phase 2: Domain Model & Canonical Schema Verification Report

**Phase Goal:** Telemetry flows end-to-end into a metering-point-keyed canonical schema, and the meter-swap differentiator works — proven by synthetic-data tests covering every swap and rollover edge case.

**Verified:** 2026-05-04T14:45:00Z
**Status:** gaps_found
**Re-verification:** No — initial verification

---

## Goal Achievement

### Observable Truths (mapped to ROADMAP Success Criteria)

| #  | Truth (ROADMAP SC verbatim)                                                                                                                                                                                                              | Status     | Evidence                                                                                                                                                                                                                                                                                                                                                                                          |
| -- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1  | Admin can create a site / MP / add a device through a single one-action dialog that auto-creates the underlying CS tenant/application/profile/device.                                                                                    | ✗ FAILED   | Backend handlers + 25 integration tests pass for site/MP/device. But: (a) ALL 5 frontend dialogs are skipped placeholders pointing to non-existent plans 02-12/13/14; (b) cmd/serve does NOT wire DeviceDeps so device routes never mount; (c) no list pages or routing entries for /sites, /devices, /metering-points exist in web/src/routes. The "single one-action dialog" the SC describes is not user-reachable.                                         |
| 2  | Real uplinks land in the `measurement` hypertable keyed by `metering_point_id` … server-side ingest time … raw payload + decoded `object` + canonical fields preserved.                                                                  | ✗ FAILED   | Schema is correct (0015 hypertable; (time, mp_id) ordering; raw_payload BYTEA NOT NULL; decoded_object JSONB NOT NULL; quality CHECK; no dev_eui column). Ingest pipeline in internal/ingest/ exists and 38 tests pass. BUT: cmd/serve.go starts MQTTSubscriber with nil handler and never calls SetUplinkHandler. Production binary drops every real uplink via the stdout-log default handler. Plan 02-09 SUMMARY explicitly defers this to "Plan 02-15" which doesn't exist. |
| 3  | Admin can perform a meter swap via dialog — reads outgoing reading R, closes the active assignment, opens a new assignment, proposes a `reading_offset` for continuity, and the cumulative chart shows no discontinuity after confirm.   | ✗ FAILED   | Backend swap math + commit (Serializable txn + audit-in-same-tx) exists; 5 integration tests pass. BUT: (a) NO HTTP handler exposes CommitSwap (internal/swap has no handlers.go); (b) swap-meter-dialog.tsx does not exist (Wave-0 placeholder skipped); (c) per-meter detail page that hosts the swap button is not in scope until Phase 4. The phase's "differentiator" has zero user-reachable surface.                                                  |
| 4  | Counter rollovers auto-detected, advance offset by counter modulus, logged as a device-health event.                                                                                                                                     | ⚠️ PARTIAL | Logic is correct + tested + audit_log CHECK constraint includes 'rollover_detected'. Plumbing complete inside ingest.PersistAtomically. Transitively fails because ingest is not wired to MQTT (Truth 2). Becomes ✓ VERIFIED automatically once Truth 2 is fixed.                                                                                                                                                                                              |
| 5  | Synthetic-data test harness passes for clean_swap, swap_with_inflight_uplink, rollover, swap_then_rollover, overlapping_uplinks_during_swap; one fully-wired vendor profile produces correct canonical fields E2E; every state-changing action lands in audit. | ✗ FAILED   | internal/testharness/scenarios_test.go is a 12-line placeholder with t.Skip. NONE of the 5 scenario tests exist in any package. No `shifter test-harness` CLI command. doc.go references "plan-02-15" which does not exist. The phase's stated proof mechanism — "proven by synthetic-data tests covering every swap and rollover edge case" — is wholly absent. (Audit-in-same-tx pattern itself IS verified across 13 mutation handlers; that's a separate sub-claim.) |

**Score:** 1/5 success criteria fully verified (only the audit-in-same-tx + schema correctness sub-claims under SC #5 hold)

### Required Artifacts

#### Backend Source (Phase 2 packages — all substantive, all wired internally)

| Artifact                              | Expected                                          | Status     | Details                                                                                                                              |
| ------------------------------------- | ------------------------------------------------- | ---------- | ------------------------------------------------------------------------------------------------------------------------------------ |
| `internal/db/migrations/0007..0017`   | 11 phase 2 migrations (site → trigger)            | ✓ VERIFIED | All 11 files present + non-trivial; create_hypertable on measurement; btree_gist EXCLUDE on binding                                  |
| `internal/db/migrations/0010_seed_profiles.up.sql` | 3 vendor profiles seeded            | ✓ VERIFIED | axioma_w1, acrel_adl200, acrel_adw300 inserted with capabilities + counter_modulus                                                   |
| `internal/profile/codecs/{axioma_w1.js,acrel_family.js,embed.go}` | 2 vendor codecs        | ✓ VERIFIED | 110 + 128 LoC; embed.go uses //go:embed                                                                                              |
| `internal/site/handlers.go`           | Site CRUD HTTP handlers + RBAC + audit-in-tx      | ✓ VERIFIED | 633 LoC; 8 handlers; Serializable txn; 9 integration tests pass                                                                      |
| `internal/meteringpoint/handlers.go`  | MP CRUD + binding-aware detail + quality summary  | ✓ VERIFIED | 695 LoC; 9 handlers; 9 integration tests pass; latest_measurement nullable                                                           |
| `internal/device/handlers.go`         | CHIRP-04 atomic AddDevice + decommission + parse  | ⚠️ ORPHANED | 818 LoC; 16 integration tests pass; CHIRP-04 atomic flow per D-16 with FRESH-context CS rollback. **But never mounted in production** because cmd/serve doesn't construct DeviceDeps. |
| `internal/device/deveui.go`           | D-11 MSB+LSB sticker parser w/ OUI hint           | ✓ VERIFIED | 113 LoC; 12 unit tests; Axioma + Acrel OUI lookups                                                                                   |
| `internal/swap/math.go`               | ProposeOffset + DetectRollover + ApplyRollover    | ✓ VERIFIED | 71 LoC; pure math.big at prec=128                                                                                                    |
| `internal/swap/commit.go`             | CommitSwap atomic with audit-in-tx + invalidator  | ⚠️ ORPHANED | 239 LoC; correct logic + 5 integration tests. **No HTTP handler calls it.**                                                          |
| `internal/swap/handlers.go`           | HTTP entry point exposing CommitSwap              | ✗ MISSING  | File does not exist; no RegisterRoutes; no chi mount                                                                                 |
| `internal/audit/log.go` + `diff.go`   | WriteEntry(ctx, tx, Entry) + ChangedFields D-24   | ✓ VERIFIED | 207 LoC total; called from 13 mutation sites in same-tx pattern                                                                      |
| `internal/profile/editor.go`          | Profile editor (capabilities + mapping rows + codec_js) | ⚠️ ORPHANED | 424 LoC; programmatic API only; **no HTTP handler exposes it.** DATA-09 mapping editor UI cannot bind to anything.                  |
| `internal/profile/seed.go`            | First-boot CS sync of seeded codecs (D-09)        | ⚠️ ORPHANED | 194 LoC; RunSeedSync function exists but never called from cmd/serve                                                                 |
| `internal/profile/jsonpointer.go`     | RFC 6901 walker for normalize.go                   | ✓ VERIFIED | 79 LoC; tested                                                                                                                       |
| `internal/resolver/cache.go`          | dev_eui→Binding lazy in-memory cache (D-25)       | ✓ VERIFIED | 160 LoC; thread-safe; ErrNoActiveBinding sentinel                                                                                    |
| `internal/resolver/listener.go`       | LISTEN/NOTIFY binding_changed listener            | ⚠️ ORPHANED | 79 LoC; correct. **Never started from cmd/serve** (resolver instance not constructed at boot)                                       |
| `internal/ingest/{decode,normalize,persist,handler,quality}.go` | Full ingest pipeline    | ⚠️ ORPHANED | ~1046 LoC across 5 files; 38 tests pass; correct DATA-01/03/05/07/08 logic. **Never bound to MQTTSubscriber.** UplinkHandler is dead code in production. |
| `internal/chirpstack/{tenant,application,device_profile,device,bootstrap}.go` | CS gRPC wrappers (D-28) | ✓ VERIFIED | All 5 files substantive; bootstrap.EnsureTenantAndApplication idempotent; tested via bufconn                                          |
| `internal/testharness/scenarios_test.go` | 5 named DATA-06 scenarios + AxiomaW1 E2E       | ✗ STUB     | 12-line Wave-0 placeholder; t.Skip; no scenarios                                                                                     |
| `internal/testharness/scenarios.go`   | Scenario builders (clean_swap, etc.)              | ✗ MISSING  | File does not exist                                                                                                                  |
| `internal/cli/testharness.go`         | `shifter test-harness <scenario>` Cobra subcommand | ✗ MISSING  | No CLI binding for D-27 dual entry point                                                                                             |

#### Frontend (Phase 2 dialogs — every single one is missing)

| Artifact                                                       | Expected                                       | Status    | Details                                                                          |
| -------------------------------------------------------------- | ---------------------------------------------- | --------- | -------------------------------------------------------------------------------- |
| `web/src/routes/sites/create-site-dialog.tsx`                  | D-17 + D-18 dialog                             | ✗ MISSING | Only `.test.tsx` Wave-0 placeholder exists                                       |
| `web/src/routes/sites/<list page>.tsx`                         | Site list + create button                      | ✗ MISSING | No file in routes/sites/ except the test stub                                    |
| `web/src/routes/metering-points/create-mp-dialog.tsx`          | D-19 dialog (name/site/utility_class)          | ✗ MISSING | Directory only contains swap-meter-dialog.test.tsx placeholder                   |
| `web/src/routes/metering-points/swap-meter-dialog.tsx`         | D-12 + D-13 (R auto-fill + offset panel)       | ✗ MISSING | Only test placeholder                                                            |
| `web/src/routes/devices/add-device-dialog.tsx`                 | D-10 4-step Stepper                            | ✗ MISSING | Only test placeholder                                                            |
| `web/src/routes/devices/deveui-parser.tsx`                     | D-11 visual MSB+LSB preview                    | ✗ MISSING | Only test placeholder                                                            |
| `web/src/routes/devices/<list page>.tsx`                       | D-29 minimal devices list (TanStack Table)     | ✗ MISSING | No file                                                                          |
| `web/src/routes/profiles/mapping-editor.tsx`                   | D-08 paste-JSON + click-leaf + table           | ✗ MISSING | Only test placeholder                                                            |
| `web/src/routes/profiles/<list page>.tsx`                      | Profile list with edit affordance              | ✗ MISSING | No file                                                                          |
| App routing entries for /sites, /metering-points, /devices, /profiles | Route declarations in App.tsx / router  | ✗ MISSING | App.tsx does not include these routes                                            |

### Key Link Verification (wiring trace)

| From                              | To                                  | Via                                            | Status      | Details                                                                                                                                                                                            |
| --------------------------------- | ----------------------------------- | ---------------------------------------------- | ----------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `internal/chirpstack/mqtt.go`     | `internal/ingest/handler.go`        | `MQTTSubscriber.SetUplinkHandler(ingest.UplinkHandler(deps))` at boot | ✗ NOT WIRED | `grep "ingest" cmd/ internal/cli/` returns 0 matches. Production MQTT subscriber starts with nil handler. Plan 02-09 SUMMARY says "cmd/serve boot wiring (Plan 02-15)" — Plan 02-15 missing.        |
| `internal/cli/serve.go`           | `internal/device/handlers.go`       | `httpapi.Deps.DeviceDeps = &device.Deps{...}`  | ✗ NOT WIRED | DeviceDeps never constructed in serve.go; router.go line 189 `if deps.DeviceDeps != nil` is permanently false in production. CHIRP-04 device routes do not mount.                                  |
| `internal/cli/serve.go`           | `internal/profile/seed.go`          | `profile.RunSeedSync(ctx, ...)` at boot        | ✗ NOT WIRED | Function exists but never invoked. Seeded profiles never reach ChirpStack until first AddDevice attempt manually pushes them.                                                                      |
| `internal/cli/serve.go`           | `internal/resolver/listener.go`     | `resolver.NewResolver(...).StartListener(ctx)` | ✗ NOT WIRED | No resolver constructed at boot. Listener (LISTEN/NOTIFY binding_changed) never starts.                                                                                                            |
| `internal/http/router.go`         | `internal/swap/<handlers>`          | `swap.RegisterRoutes(r, deps)` or via meteringpoint | ✗ NOT WIRED | No swap HTTP handler exists at all. CommitSwap is callable only from Go code.                                                                                                                      |
| `internal/http/router.go`         | `internal/profile/<handlers>`       | `profile.RegisterRoutes(r, deps)`              | ✗ NOT WIRED | No profile HTTP handler exists. Mapping editor + codec save have no API.                                                                                                                           |
| `web/src/routes/devices/add-device-dialog.tsx` | `POST /api/devices/add`   | `apiFetch('/api/devices/add', {method: 'POST', ...})` | ✗ NOT WIRED | Dialog file does not exist                                                                                                                                                                          |
| `web/src/routes/sites/create-site-dialog.tsx`  | `POST /api/sites`         | `apiFetch('/api/sites', {method: 'POST', ...})` | ✗ NOT WIRED | Dialog file does not exist                                                                                                                                                                          |
| `web/src/routes/metering-points/swap-meter-dialog.tsx` | swap commit API   | apiFetch swap endpoint                          | ✗ NOT WIRED | Dialog file does not exist; backend has no swap endpoint either                                                                                                                                    |
| `web/src/routes/profiles/mapping-editor.tsx`   | profile editor API        | apiFetch                                       | ✗ NOT WIRED | Both ends missing                                                                                                                                                                                   |
| `internal/audit/log.go`           | `internal/db/sqlc/audit_log`        | WriteEntry(ctx, tx, Entry) inside tx           | ✓ WIRED     | 13 call sites across site/MP/device/swap/ingest mutation handlers; all use the SAME pgx.Tx as the domain mutation. AUDIT-01 D-23 enforced by signature.                                            |
| `internal/swap/commit.go`         | `internal/audit/log.go`             | WriteEntry inside Serializable txn             | ✓ WIRED     | Confirmed in commit.go body; 'swap' action is in the 0016 CHECK constraint                                                                                                                          |
| `internal/ingest/persist.go`      | `internal/audit/log.go`             | WriteEntry for action='rollover_detected'      | ✓ WIRED     | Same-tx pattern; rollover_detected is in 0016 CHECK                                                                                                                                                 |

### Data-Flow Trace (Level 4)

Limited applicability — the truths here are server-driven (uplink → DB) rather than client-rendered. The wiring failures dominate.

| Artifact                           | Data Variable                  | Source                                | Produces Real Data | Status            |
| ---------------------------------- | ------------------------------ | ------------------------------------- | ------------------ | ----------------- |
| `internal/ingest/handler.go`       | uplink event payload           | MQTT subscriber (chirpstack/mqtt.go)  | N/A — never invoked | ✗ DISCONNECTED    |
| `internal/swap/commit.go`          | SwapInput.OutgoingReadingR     | (would-be) HTTP handler reading body  | N/A — no handler    | ✗ DISCONNECTED    |
| `internal/profile/editor.go` Save  | mapping rows                   | (would-be) HTTP handler reading body  | N/A — no handler    | ✗ DISCONNECTED    |
| `internal/site/handlers.go::createSite` | request body                | apiFetch from create-site-dialog.tsx  | N/A — no UI caller  | ⚠️ HOLLOW_PROP    |
| `internal/device/handlers.go::addDevice` | request body              | apiFetch from add-device-dialog.tsx   | N/A — no UI caller  | ⚠️ HOLLOW_PROP + route not mounted |

### Behavioral Spot-Checks

| Behavior                                                  | Command                                                                                                                                  | Result                                                          | Status |
| --------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------- | ------ |
| Project compiles                                          | `go build ./...`                                                                                                                         | Success                                                          | ✓ PASS |
| Backend Phase 2 short tests pass                          | `go test -short -count=1 ./internal/{site,meteringpoint,swap,audit,profile,resolver,ingest,device,chirpstack,testharness}/...`           | 116 tests in 11 packages                                         | ✓ PASS |
| Backend Phase 2 integration tests pass (testcontainer)    | `go test -count=1 ./internal/{site,meteringpoint,device,swap,audit,profile,resolver,ingest}/...` (with podman + testcontainers env)      | 148 tests in 9 packages                                          | ✓ PASS |
| Frontend tests pass                                       | `pnpm --dir web test --run`                                                                                                              | 30 passed + 5 skipped (5 = the missing Phase 2 dialogs)          | ⚠️ PASS-WITH-SKIP |
| testharness package has runnable scenario tests           | `go test -count=1 ./internal/testharness/...`                                                                                            | "No tests found"                                                  | ✗ FAIL |
| No vendor switch in normalize.go (DATA-09 anti-pattern)   | `grep -iE 'axioma\|acrel\|kamstrup\|diehl\|itron' internal/ingest/normalize.go`                                                          | 0 matches                                                        | ✓ PASS |
| audit_log CHECK includes 'swap' + 'rollover_detected'     | `grep -E "'swap'\|'rollover_detected'" internal/db/migrations/0016_audit_log.up.sql`                                                     | Both present                                                     | ✓ PASS |
| Measurement schema has no dev_eui or device_id column     | `grep -iE 'dev_eui\|device_id' internal/db/migrations/0015_measurement.up.sql`                                                           | 0 matches in column definitions (DATA-01 invariant holds)        | ✓ PASS |
| `shifter test-harness` CLI subcommand exists              | `grep -rn "test-harness\|TestHarness" cmd/ internal/cli/`                                                                                | 0 matches                                                        | ✗ FAIL |

### Requirements Coverage

| Requirement | Source Plan(s)                  | Description                                                                                                                                                  | Status         | Evidence                                                                                                                                       |
| ----------- | ------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| SITE-01     | 02-02, 02-06, 02-10             | Admin can create / edit / delete sites via dialogs, with site lat/lng for the map view                                                                       | ⚠️ BACKEND-ONLY | Site backend handlers + lat/lng validation + 9 tests. Frontend create-site-dialog.tsx missing. REQUIREMENTS.md marks "Complete" — overstated. |
| DATA-01     | 02-03, 02-04, 02-06, 02-07, 02-09 | Telemetry keyed on stable `metering_point_id`, never on `dev_eui`                                                                                            | ✓ SATISFIED    | Schema enforces (no dev_eui column on measurement); resolver maps dev_eui→MP via active binding. Logic correct even though pipeline is unwired. |
| DATA-02     | 02-03, 02-06, 02-07             | Each device-MP binding has (valid_from, valid_to, reading_offset)                                                                                            | ✓ SATISFIED    | 0014_binding.up.sql columns + EXCLUDE constraints; sqlc CRUD tested.                                                                            |
| DATA-03     | 02-04, 02-09                    | Hypertable `time` column is server-side ingest time (gateway_rx_time + device_time as diagnostics)                                                           | ⚠️ BACKEND-ONLY | ingest/handler.go captures ingestTime FIRST; tested. Transitively unsatisfied at runtime because pipeline not wired to MQTT.                  |
| DATA-04     | 02-06, 02-07                    | Admin can perform meter swap via dialog with proposed offset for continuity                                                                                  | ⚠️ BACKEND-ONLY | Swap math + commit logic + audit-in-tx; tested. No HTTP handler, no dialog. REQUIREMENTS.md marks "Complete" — overstated.                    |
| DATA-05     | 02-06, 02-07, 02-09             | Counter rollover detection + offset advance + audit log                                                                                                      | ⚠️ BACKEND-ONLY | DetectRollover + ApplyRollover; persist.go writes 'rollover_detected' audit. Transitively unsatisfied at runtime.                              |
| DATA-06     | 02-09 (per VALIDATION) — never delivered | Synthetic-data tests cover clean_swap / swap_inflight / rollover / swap+rollover / overlapping_uplinks                                              | ✗ BLOCKED      | 0 of 5 scenarios exist. testharness package is a placeholder. **REQUIREMENTS.md correctly marks "Pending".**                                  |
| DATA-07     | 02-04, 02-06, 02-09             | Persist raw payload + decoded `object` + canonical fields — none lost                                                                                        | ⚠️ BACKEND-ONLY | Schema + persist.go correct; tested. Transitively unsatisfied at runtime.                                                                       |
| DATA-08     | 02-04, 02-06, 02-09             | Hybrid wide+JSONB schema (canonical columns + extra JSONB)                                                                                                   | ✓ SATISFIED    | Schema is correct and the only piece this REQ asks for. Logic in normalize.go routes to canonical or extra.<key>. Tested.                      |
| DATA-09     | 02-02, 02-03, 02-05, 02-06, 02-08 | Admin can map device profile decoded fields to canonical columns through a UI; new vendors require no backend deploy                                      | ⚠️ BACKEND-ONLY | profile/editor.go programmatic API exists. **No HTTP handlers + no mapping-editor.tsx UI.** "Through a UI" half is unfulfilled.               |
| DATA-10     | 02-02, 02-05, 02-08             | At least one fully-wired vendor profile + documented path                                                                                                    | ⚠️ PARTIAL     | 3 profiles seeded + 2 codec families embedded. NO end-to-end synthetic-uplink test proves any profile produces correct canonical fields. (Tied to DATA-06.) |
| AUDIT-01    | 02-04, 02-06, 02-07, 02-10      | Every CRUD + meter swap writes audit row in same txn                                                                                                         | ✓ SATISFIED    | 13 call sites confirmed; same-tx pattern; INSERT-ONLY enforced by trigger; field-level diff via ChangedFields.                                  |
| CHIRP-04    | 02-05, 02-06, 02-10             | Adding a device is a single user action — Shifter creates / reuses CS tenant + application + profile + device binding behind the scenes                     | ⚠️ BACKEND-ONLY | AddDevice atomic flow + bootstrap + tests. Routes never mounted in production (DeviceDeps not constructed). add-device-dialog.tsx missing.    |

**Orphaned requirements:** None — every Phase 2 REQ-ID from REQUIREMENTS.md (SITE-01, DATA-01..10, AUDIT-01, CHIRP-04) is claimed by at least one plan's `requirements:` field. However, several plans overclaim — Plan 02-06 (sqlc query suite) lists 10 requirements but only delivers Go bindings, not user-visible fulfillment.

**Note for REQUIREMENTS.md hygiene:** REQUIREMENTS.md currently marks SITE-01, DATA-01..05, DATA-07..10, AUDIT-01, CHIRP-04 as "Complete" (line 259-273). This is **overstated** for any REQ whose "Complete" is being read by a human as "the operator can use this." A more accurate state is:
- "Complete" (substrate ready, end-user surface wired): AUDIT-01 (no UI surface required by REQ wording), DATA-01, DATA-02, DATA-08
- "Backend-Only": SITE-01, DATA-03, DATA-04, DATA-05, DATA-07, DATA-09, CHIRP-04
- "Pending": DATA-06, DATA-10 (DATA-10 partial — profile rows present but E2E unproven)

### Anti-Patterns Found

| File                                  | Line | Pattern                                                  | Severity   | Impact                                                                                                            |
| ------------------------------------- | ---- | -------------------------------------------------------- | ---------- | ----------------------------------------------------------------------------------------------------------------- |
| `internal/testharness/scenarios_test.go` | 5    | `// TODO(plan-02-15): implement after scenarios.go exists.` | 🛑 Blocker | Plan 02-15 doesn't exist; the placeholder will rot. DATA-06 unfulfilled.                                          |
| `internal/testharness/scenarios_test.go` | 10   | `t.Skip("Wave 0 placeholder — see TODO above")`         | 🛑 Blocker | The literal phase-goal "proven by synthetic-data tests" is a skipped test                                          |
| `internal/testharness/doc.go`         | 1    | `// Package testharness — Phase 2 placeholder. See plan-02-15 for body.` | 🛑 Blocker | Documentation lies about future plan that will never run                                                          |
| `web/src/routes/sites/create-site-dialog.test.tsx` | 4 | `it.skip('TODO(plan-02-12): implement after create-site-dialog.tsx exists', ...)` | 🛑 Blocker | Plan 02-12 doesn't exist. SITE-01 surface absent.                                                                  |
| `web/src/routes/devices/add-device-dialog.test.tsx` | 4 | `it.skip('TODO(plan-02-13): implement after add-device-dialog.tsx exists', ...)` | 🛑 Blocker | Plan 02-13 doesn't exist. CHIRP-04 user surface absent.                                                            |
| `web/src/routes/devices/deveui-parser.test.tsx` | (skipped) | DevEUI parser UI placeholder                       | 🛑 Blocker | D-11 user-facing preview missing                                                                                   |
| `web/src/routes/metering-points/swap-meter-dialog.test.tsx` | 4 | `it.skip('TODO(plan-02-14): implement after swap-meter-dialog.tsx exists', ...)` | 🛑 Blocker | The phase's headline differentiator has no UI                                                                       |
| `web/src/routes/profiles/mapping-editor.test.tsx` | 4 | `it.skip('TODO(plan-02-14): implement after mapping-editor.tsx exists', ...)` | 🛑 Blocker | DATA-09 "through a UI" missing                                                                                     |
| `internal/cli/serve.go`               | 76-77 | `// Phase 1 default handler logs uplinks; Phase 2 swaps in normalize+persist via the same Plan 13 hook point.` | 🛑 Blocker | Comment says it WILL be done; the wiring step (Plan 02-15 in current numbering) was never executed. Real uplinks dropped. |
| `internal/cli/serve.go`               | (entire) | DeviceDeps never set on httpapi.Deps                  | 🛑 Blocker | CHIRP-04 routes never mount in production binary                                                                   |
| `.planning/phases/02-domain-model-canonical-schema/02-VALIDATION.md` | 46-86 | "⬜ pending" status on every row | ⚠️ Warning | VALIDATION.md was never updated as plans executed; cannot be used as a status board                              |

### Human Verification Required

Three items normally would need human testing per VALIDATION.md Manual-Only table. ALL THREE are currently **blocked** by the gaps above and CANNOT BE EXECUTED until the missing UI + wiring lands. They are listed in the frontmatter for traceability but are not actionable today.

### Gaps Summary

The phase delivers a **strong backend foundation** — schema, sqlc bindings, swap math, audit pattern, ingest pipeline, CHIRP-04 atomic flow, profile editor, resolver — that is internally well-tested (148 testcontainer tests pass; 116 short tests pass; project builds cleanly). Migration set, JSONB extra schema, btree_gist EXCLUDE constraints, audit-in-same-tx pattern, and the DATA-09 vendor-agnostic mapping engine (no vendor switch in normalize.go) are exemplary work.

**However, the phase fails three of five ROADMAP success criteria because the work was never connected to operators or to real uplinks.** Three categories of gap:

1. **Boot wiring missing (Plan 02-15 never executed).** ingest pipeline + resolver listener + profile RunSeedSync + DeviceDeps construction + swap HTTP handler + profile HTTP handler are all absent from cmd/serve.go. Multiple SUMMARY files explicitly reference "Plan 02-15" as the wiring step. **Root cause:** Phase 2's plan inventory ended at 02-10 even though the original 02-01 plan-template anticipated 02-12 / 02-13 / 02-14 / 02-15.

2. **Frontend dialogs missing.** All 5 Phase 2 dialogs (create-site, create-MP, swap-meter, add-device, mapping-editor) are Wave-0 skipped placeholders. No list pages, no routing entries. Operator has zero surface to drive the headline phase deliverables.

3. **DATA-06 synthetic harness missing.** The phase goal *literally* says "proven by synthetic-data tests covering every swap and rollover edge case." testharness package is a 12-line placeholder. None of the 5 scenarios + the AxiomaW1 E2E test exist. CLI subcommand absent.

**Suggested gap-closure plan grouping:**
- **Plan A — Boot wiring + swap/profile HTTP handlers** (1 plan): construct DeviceDeps in serve.go; mount swap.RegisterRoutes + profile.RegisterRoutes; bind ingest.UplinkHandler to MQTTSubscriber; start resolver listener; call RunSeedSync at boot. Closes Truths 2 + 3 (backend half) + 4 (transitively).
- **Plan B — Frontend dialog set + list pages + routing** (1-2 plans): create-site, create-MP, swap-meter, add-device (4-step Stepper), deveui-parser preview, mapping-editor; minimal list pages for /sites, /devices, /metering-points, /profiles; routing entries; per-meter detail page MINIMAL (just enough to host the swap button — full chart waits for Phase 4). Closes Truths 1 + 3 (frontend half).
- **Plan C — Synthetic test harness** (1 plan): scenarios.go with 5 named scenario builders + axioma_w1 + acrel_adw300 synthetic publishers; scenarios_test.go with 6 TestScenario_* tests using testcontainers Mosquitto + Postgres+TimescaleDB; CLI subcommand `shifter test-harness <scenario> --vendor <slug>` for D-27 dual-entry. Closes Truth 5.

Plan A unblocks Truth 4 automatically and is a prerequisite for Plan C (synthetic uplinks must reach the wired ingest pipeline). Plans A + C can run in parallel only if Plan C uses an in-test ingest deps construction (not the production binary).

---

_Verified: 2026-05-04T14:45:00Z_
_Verifier: Claude (gsd-verifier)_
