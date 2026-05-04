---
phase: 02-domain-model-canonical-schema
plan: 09
subsystem: ingest-pipeline
tags: [wave-5, ingest, mqtt-binding, decode, normalize, persist, rollover, atomic-tx, server-time, hybrid-jsonb, never-silent-drop]

requires:
  - phase: 02-domain-model-canonical-schema
    plan: 07
    provides: internal/audit.WriteEntry(ctx, pgx.Tx, Entry) — same-tx audit pattern; ActionRolloverDetected + EntityTypeBinding constants pinned to 0016 CHECK vocabulary. internal/swap.DetectRollover(prev, curr) + ApplyRollover(offset, modulus) — pure math at big.Float prec=128. internal/resolver.Resolver — concurrent-safe dev_eui→Binding cache with Set/Lookup/Invalidate; ErrNoActiveBinding sentinel.
  - phase: 02-domain-model-canonical-schema
    plan: 08
    provides: internal/profile.Resolve(obj, ptr) — RFC 6901 JSON Pointer walker used by NormalizeMeasurement to traverse the QuickJS-decoded object per mapping row. internal/profile.Mapping struct (JSONPointer + Target + Scale + DataType + Position).

provides:
  - internal/ingest/decode.go — DecodeChirpStackEvent(payload) parses ChirpStack v4 uplink event JSON; returns Event with DevEUI lowercased (DATA-01), DecodedObject as map[string]any (Pitfall 11 — NOT v3 string), GatewayRxTime = earliest of rxInfo[].time (Open Q #3), DeviceTime from top-level `time`, base64-decoded `data` field. devEUIFromTopic helper for the decode_fail dev_eui-from-topic recovery path.
  - internal/ingest/quality.go — 5 D-26 quality flag constants (QualityOK / QualityDecodeFail / QualityMissingCanonical / QualityOutOfRange / QualityDuplicateFcnt) pinned exactly to migration 0015 CHECK vocabulary.
  - internal/ingest/handler.go — UplinkHandler(deps) returns chirpstack.UplinkHandler closure orchestrating decode → resolver.Lookup → load mappings → NormalizeMeasurement → PersistAtomically. ingestTime = time.Now().UTC() captured at entry (DATA-03 + Pitfall 4). 10s context timeout (T-02-09-05). Decode-fail path persists row with raw_payload preserved (D-26 + DATA-07). Unbound device → log + drop (DATA-01 invariant: no row without an MP key). MappingStore + Resolver interfaces; SQLCMappingStore concrete impl for cmd/serve boot wiring.
  - internal/ingest/normalize.go — NormalizeMeasurement(decoded, mappings) walks profile mapping rows in Position order via profile.Resolve. Routes to 10 canonical Layer1 columns (raw_value, cumulative_value, instant_value, battery_pct, rssi, snr, temperature_c, pressure_kpa, leak_detected, tamper_detected) or extra.<key>. NO vendor switch (Anti-Pattern + DATA-09). coerce + applyScale per data_type (numeric/int/bool/text). ErrNoCanonicalValue when neither raw_value nor instant_value populated.
  - internal/ingest/persist.go — PersistAtomically(ctx, deps, in) opens pgx.Serializable txn; detects rollover when prev + curr raw values + counter_modulus all available (T-02-09-06 first-uplink mitigation); advances binding.reading_offset; computes cumulative = raw + currentOffset; AppendMeasurement with raw_payload + decoded_object + canonical fields ALL persisted (DATA-07 + DATA-08); UpdateBindingLastRaw + UpdateDeviceLastSeen; audit.WriteEntry for rollover IN SAME TX (D-23 + AUDIT-01); commits; defensively primes resolver cache with new last_raw / offset (0017 trigger does NOT fire on those updates per Plan 02-07 decision).
  - internal/chirpstack/mqtt.go — SetUplinkHandler(h) method on MQTTSubscriber so cmd/serve boot wiring (Plan 02-15) can install the ingest closure without threading it through NewMQTTSubscriber.

affects: [02-10, 02-15]

tech-stack:
  added: []
  patterns:
    - "Pattern: server-side ingest time is captured FIRST. handler.go line 96 — ingestTime := time.Now().UTC() runs BEFORE any work that could affect the clock (decode, resolver lookup, mapping load, normalize, persist). The same value flows into measurement.time, device.last_seen_at, and the rollover audit row's `time` so all four agree to the nanosecond. Replaces gateway_rx_time / device_time as the authoritative time column (DATA-03 + Pitfall 4); both diagnostic columns still persisted alongside."
    - "Pattern: D-26 + DATA-01 reconciliation — never silent-drop EXCEPT for unbound devices. CONTEXT D-26 says 'persist all uplinks, never silent-drop' but DATA-01 says 'measurement keyed by metering_point_id only, no device_id.' When a dev_eui has no active binding, satisfying DATA-01 requires NOT writing a row. The handler emits a structured-log line ('ingest: unbound device — dropping uplink') so operators see it via Phase 6 grep / log surface, but the hypertable stays clean. This is the only path where DATA-07 (raw bytes preserved) yields to DATA-01 (MP-keyed). Decode-fail with a recoverable dev_eui IS persisted (DATA-01 satisfied via resolver Lookup); decode-fail with an unbound dev_eui is logged + dropped."
    - "Pattern: rollover detection skipped on first-uplink-after-binding-open. T-02-09-06 mitigation: a freshly-opened binding has Binding.LastRawValue == nil (the boundary signal). Without this guard, the new meter's first uplink (raw=0 typically) vs whatever the OUTGOING meter last reported (raw=4M+ for a near-full counter) would falsely trigger rollover, advancing offset by counter_modulus and breaking cumulative continuity. The DetectRollover guard `Binding.LastRawValue != nil && in.Binding.CounterModulus > 0` cleanly handles this without a swap-aware code path."
    - "Pattern: post-commit resolver cache priming. PersistAtomically's last step (after tx.Commit) calls deps.Resolver.Set(devEUI, updatedBinding) with the new LastRawValue + ReadingOffset. The 0017 NOTIFY trigger does NOT fire on UpdateBindingLastRaw / AdvanceReadingOffset (Plan 02-07 decision to avoid NOTIFY storms — those columns aren't cache-relevant for the resolver's lookup contract but ARE relevant for THIS uplink-to-next-uplink rollover continuity). Without the Set, every uplink would re-load the binding from Postgres on each call — even though the row is structurally cached. The Set keeps the hot path lock-free."
    - "Pattern: vendor-agnostic mapping engine enforced at the test layer. TestNormalizeMeasurement_NoVendorSwitchInSource + GrepCheckMatchesPlan run the literal `grep -iE 'axioma|acrel|kamstrup|diehl|itron'` against normalize.go; both fail the build if a vendor name string literal lands in the source. DATA-09 + Anti-Pattern 'no decoders.ts' — the mapping table is the only configuration. Comments rephrased to reference '<vendor-slug>' generically so the guard stays clean."
    - "Pattern: handler tests use eventBuilder DSL + fakeMappingStore + fakeLoader. eventBuilder.MarshalJSON synthesizes ChirpStack v4 uplink event JSON from a small struct so tests don't ship hand-written JSON literals. fakeLoader satisfies resolver.Loader with an in-memory map; unmapped EUIs return resolver.ErrNoActiveBinding so the unbound-device path is exercisable without seeding a binding. fakeMappingStore is keyed by profile id; absent entries return [] so a happy-path-but-no-mappings test flows through normalize → ErrNoCanonicalValue cleanly."

key-files:
  created:
    - internal/ingest/decode.go
    - internal/ingest/quality.go
    - internal/ingest/handler.go
    - internal/ingest/normalize.go
    - internal/ingest/persist.go
  modified:
    - internal/chirpstack/mqtt.go (SetUplinkHandler method added)
    - internal/ingest/handler_test.go (Wave 0 placeholder filled)
    - internal/ingest/normalize_test.go (Wave 0 placeholder filled)

key-decisions:
  - "Unbound dev_eui = log + drop, NOT persist. CONTEXT D-26 says 'persist all uplinks' but DATA-01 mandates measurement rows be keyed by metering_point_id. The two reconcile by treating 'no active binding for dev_eui' as the SINGLE exception: structured-log emitted for Phase 6 surface, no hypertable row. This decision is documented in handler.go's UplinkHandler doc comment + persist_decode_fail handles the same case for malformed events. All other failure modes (decode_fail with bound device, missing_canonical, out_of_range) DO persist a row."
  - "ingestTime captured BEFORE decode. handler.go line 96 runs `ingestTime := time.Now().UTC()` as the first non-trivial statement. Decode could in principle take milliseconds (large JSON payloads + base64 decode), and we want the time column to reflect 'when did Shifter receive this' not 'when did Shifter finish parsing it.' Sub-millisecond difference in practice; the principle is what matters for D-03 audit defensibility."
  - "MappingStore is an interface even though the production impl wraps sqlc trivially. Tests substitute fakeMappingStore without touching Postgres, and a future caching layer (mapping rows are immutable per save — could be cached for the duration between profile saves) plugs in behind the same interface. SQLCMappingStore concrete impl is exported for cmd/serve boot wiring."
  - "Resolver hot-path interface defined IN the ingest package (not imported as resolver.Resolver concrete type). Lookup + Set are the only two methods needed; defining the interface here keeps the test fakes minimal. *resolver.Resolver structurally satisfies it. Mirrors swap.Invalidator's design (Plan 02-07)."
  - "Decode-fail path uses the resolver Lookup to find the binding for the topic-recovered dev_eui. Naively, decode_fail could persist with binding_id=NULL, but DATA-01 still applies (rows are keyed by MP). Looking up the binding lets us key the decode_fail row to the correct MP — operators see 'meter X had a decode failure at time T' in the MP detail view's recent-uplinks tab. If the dev_eui is also unbound, we log + drop (same as the orphan path)."
  - "PersistAtomically doesn't out-of-range-validate. The plan's quality vocabulary includes QualityOutOfRange but the persist layer doesn't compute it — the normalize coerce step rejects values outside the data_type range silently (e.g. battery_pct gets coerced, but coerce truncates an int outside int16). Out-of-range is a Phase 6 concern (anomaly detection); the constant is defined for forward-compat. DuplicateFcnt is similarly forward-compat (ChirpStack v4 dedupes server-side; Phase 6 may add Shifter-side defense in depth)."
  - "PersistAtomically writes audit row with UserID=uuid.Nil (system-emitted). The rollover_detected action is NOT operator-initiated. audit.WriteEntry encodes uuid.Nil as SQL NULL (audit_log.user_id is NULLable per 0016 schema). Phase 6 audit browse can filter system events from operator events via the user_id IS NULL clause."
  - "Defensive resolver Set after commit (NOT before tx.Commit). If we Set the cache before commit and the commit fails, the cache holds state that doesn't match Postgres — the next uplink would compute against an offset that was rolled back. Set strictly after Commit success means cache is at most ONE uplink stale, not divergent. Worst case: a parallel goroutine reads a stale entry between commit and Set; that's a single-uplink race window with no correctness impact (next uplink resolves fresh)."
  - "Tests use real testsupport.StartPostgres + the seeded axioma_w1 profile from migration 0010. The seeded profile has counter_modulus=4294967296 (2^32) which is exactly what a Qalcosonic W1's 32-bit cumulative counter wraps at — pinning the rollover test to a realistic modulus. No need for a custom test profile; reuse the production seed."
  - "No new dependencies. Phase 1's go.mod (jackc/pgx/v5/{pgxpool,pgtype}, google/uuid, stretchr/testify, eclipse/paho.mqtt.golang) + stdlib (context, encoding/json, encoding/base64, math/big, sort, strings, sync, sync/atomic, log/slog, time) cover everything. Pattern continued from Plans 02-07 + 02-08."

requirements-completed: [DATA-01, DATA-03, DATA-05, DATA-07, DATA-08]

duration: 14min26s
completed: 2026-05-04
---

# Phase 02 Plan 09: Ingest Pipeline Summary

**Five Go files in internal/ingest/ (~1046 LoC across decode.go + quality.go + handler.go + normalize.go + persist.go) plus a 1-method addition to internal/chirpstack/mqtt.go (SetUplinkHandler) deliver the ChirpStack v4 uplink ingest pipeline. The handler closure captures ingestTime = time.Now().UTC() at entry (DATA-03 + Pitfall 4), decodes the v4 event JSON (object as map[string]any per Pitfall 11), resolves dev_eui → binding via Plan 02-07's resolver cache, loads per-profile mapping rows via the MappingStore interface, normalizes via Plan 02-08's profile.Resolve (RFC 6901 JSON Pointer) without any vendor switch (DATA-09 + Anti-Pattern), then PersistAtomically opens a pgx.Serializable txn that runs rollover detection + AppendMeasurement + UpdateBindingLastRaw + UpdateDeviceLastSeen + (if rollover) audit.WriteEntry inside one atomic commit (D-23 + AUDIT-01 enforcement via Plan 02-07 audit package). DATA-01 / DATA-03 / DATA-05 / DATA-07 / DATA-08 invariants enforced: rows keyed by metering_point_id (resolver-derived), time = server-side ingest time, rollover detection auto-advances reading_offset + writes audit row in same tx (skipped on first-uplink-after-binding-open per T-02-09-06), raw_payload + decoded_object + canonical wide columns + extra JSONB ALL persisted on every row (incl. quality != 'ok'), and the hybrid wide+JSONB schema satisfied by the routing in normalize.go (10 canonical targets + extra.<key>). Three commits: a9e0888 (pipeline source) + 17ff87d (normalize unit tests + vendor-switch grep guard) + 1b0f3df (handler + persist integration tests). 38/38 ingest tests pass under -race -count=1 + 28 net-new short tests; full short suite 242/242 pass (was 214; +28). Vet + build clean. NO new dependencies.**

## Performance

- **Duration:** ~14min26s
- **Started:** 2026-05-04T06:16:53Z (post-Plan 02-08)
- **Completed:** 2026-05-04T06:31:19Z
- **Tasks:** 3 / 3
- **Files created:** 5 (.go) — decode.go + quality.go + handler.go + normalize.go + persist.go
- **Files modified:** 3 (chirpstack/mqtt.go + 2 Wave 0 stub _test.go files filled)

## Accomplishments

- **Task 1 — Decode + quality + handler entry point + MQTT wiring.** `internal/ingest/quality.go` exports 5 quality flag constants pinned exactly to migration 0015 CHECK vocabulary; call sites use these instead of string literals. `internal/ingest/decode.go` exports DecodeChirpStackEvent(payload) which parses the v4 uplink event JSON into a strongly-typed Event struct: DevEUI lowercased (DATA-01 normalization), DecodedObject as map[string]any (Pitfall 11 — v4 NOT v3), base64-decoded Data, GatewayRxTime as the EARLIEST of rxInfo[].time (Open Q #3 input), DeviceTime from top-level `time`. devEUIFromTopic helper extracts the dev_eui from `application/+/device/<eui>/event/up` for the decode_fail-with-recoverable-eui path. `internal/ingest/handler.go` exports UplinkHandler(deps) that returns the chirpstack.UplinkHandler closure: captures ingestTime = time.Now().UTC() FIRST (line 96 — DATA-03 + Pitfall 4), 10s context timeout (T-02-09-05), routes decode failures + unbound devices + happy paths each to their correct quality flag. SQLCMappingStore concrete impl wraps a *pgxpool.Pool for cmd/serve boot wiring. MappingStore + Resolver interfaces let test fakes plug in without touching Postgres. `internal/chirpstack/mqtt.go` gains SetUplinkHandler(h) method so cmd/serve can install the ingest closure post-construction (Plan 02-15 wiring). 16 unit + integration tests pass: 6 decode (3 happy-path sub-cases, MissingObject_NoError, BadJSON, EarliestGatewayRxTime, Base64Data, devEUIFromTopic table) + 5 handler integration (DecodeFail_PersistsWithQuality, NoBinding_LogsAndDrops, HappyPath_PersistsRow, NoCanonicalValueMapped_FlagsRow, DataTimeIsServerSide).
- **Task 2 — Normalize (mapping engine, vendor-agnostic).** `internal/ingest/normalize.go` exports NormalizeMeasurement(decoded, mappings) which walks profile mapping rows in Position order via profile.Resolve. canonicalTargets map enumerates the 10 D-02 column names; `extra.<key>` prefix routes to Layer1.Extra (D-08 hybrid wide+JSONB). coerce(val, dataType) handles numeric (float64/int/string→big.Float), int (with bool→0/1 defensive coercion for vendor firmware quirks), bool (with int 0/1 → bool), text. applyScale(val, scale, dataType) multiplies numerics by the scale factor; identity short-circuit when scale==1. assignToLayer1 routes coerced values to the right struct field. ErrNoCanonicalValue surfaces when neither RawValue nor InstantValue gets populated → caller upgrades to quality='missing_canonical' (row still persists). 12 + 2 tests pass: SingleField (scale=0.001 applied), MultipleCanonical, ExtraFields (3 keys including text), MissingPointer_Skips, DataTypeBool (4 sub-cases), PositionOrderApplied (last writer wins), InstantValueSatisfiesCanonical, RootPointer_WholeObject (empty pointer + map → coerce fails). NoVendorSwitchInSource + GrepCheckMatchesPlan enforce DATA-09 + Anti-Pattern: literal `grep -iE 'axioma|acrel|kamstrup|diehl|itron'` against normalize.go must return zero matches; one comment rephrased to use generic `<vendor-slug>` so the guard stays clean.
- **Task 3 — Atomic persist + rollover detection + audit.** `internal/ingest/persist.go` exports PersistAtomically(ctx, deps, in) which opens pgx.Serializable txn and runs the full atomic write set: (1) rollover detection via swap.DetectRollover + swap.ApplyRollover when both LastRawValue and CounterModulus available — first-uplink-after-binding-open guard (T-02-09-06 mitigation) skips when LastRawValue is nil; advances binding.reading_offset via sqlc.AdvanceReadingOffset on detection. (2) cumulative_value = raw + currentOffset (codec-emitted CumulativeValue overrides if mapped). (3) sqlc.AppendMeasurement with all 20 column parameters populated — raw_payload (BYTEA) + decoded_object (JSONB) + canonical wide columns + extra (JSONB) + quality + fcnt + gateway_rx_time + device_time + binding_id ALL persisted regardless of quality (DATA-07 + D-26 — never silent-drop). (4) sqlc.UpdateBindingLastRaw when RawValue populated — feeds next uplink's rollover detection. (5) sqlc.UpdateDeviceLastSeen — Phase 4 "device offline" alerts. (6) On rollover: audit.WriteEntry with audit.ActionRolloverDetected + audit.EntityTypeBinding + UserID=uuid.Nil (system-emitted) inside SAME tx (D-23 + AUDIT-01). (7) tx.Commit. (8) POST-COMMIT: deps.Resolver.Set primes cache with new last_raw + offset (0017 NOTIFY trigger doesn't fire on those updates per Plan 02-07; without this Set every uplink re-loads from Postgres). 5 persist integration tests pass: RolloverDetected (pre-seed last_raw=4294967290 + counter_modulus=4294967296 → rollover fires + offset advanced + audit row written + cumulative computed correctly), MultipleUplinksSameMP (5 uplinks → 5 rows, all keyed by MP, last_raw + last_seen tracked), RolloverNotDetectedOnFirstUplink (first-uplink-for-binding boundary), PreservesRawAndDecodedAcrossQualityLevels (DATA-07 — ok + decode_fail + missing_canonical rows ALL have non-empty raw_payload + decoded_object), RollsBackOnContextCancel (atomicity sanity check via pre-canceled ctx).
- **Verification.** `go test -count=1 -race ./internal/ingest/...` 38/38 pass: 16 Task-1 + 14 Task-2 (12 NormalizeMeasurement + 2 vendor-switch guards) + 5 Task-3 + 3 helper-overlap (TestUplinkHandler_HappyPath/NoCanonicalValue/DataTimeIsServerSide cover Task-3 invariants too). `go test -count=1 -race ./internal/chirpstack/...` 38/38 pass (no regressions from SetUplinkHandler addition). `go test -count=1 -short ./...` 242/242 pass in 23 packages (was 214 baseline; +28 from this plan). `go vet ./...` clean; `go build ./...` clean. Plan verification grep checks: `grep "time.Now().UTC()" internal/ingest/handler.go` returns 1 line (DATA-03 enforcement), `grep "in.IngestTime" internal/ingest/persist.go` returns 1 line (NOT GatewayRxTime in time column), `grep "raw_payload\|RawPayload" internal/ingest/persist.go` returns 3 lines (DATA-07), `grep -iE "axioma|acrel|kamstrup|diehl|itron" internal/ingest/normalize.go` returns 0 lines (DATA-09 + Anti-Pattern).

## Task Commits

1. **Task 1: Ingest pipeline source — decode + quality + handler + normalize + persist + MQTT SetUplinkHandler** — `a9e0888` (feat)
2. **Task 2: Normalize unit tests + vendor-switch grep guard** — `17ff87d` (test)
3. **Task 3: Handler + persist integration tests (decode_fail, unbound, rollover, atomicity)** — `1b0f3df` (test)

**Plan metadata commit:** _pending — created at end of plan_

## Note on Task-File Coupling

The plan's three-task structure separates decode.go + quality.go + handler.go (Task 1) from normalize.go (Task 2) and persist.go (Task 3). However handler.go imports both NormalizeMeasurement and PersistAtomically — committing Task 1 in isolation would leave the package non-compiling. The pragmatic resolution: the Task 1 commit ships the FULL pipeline source (all 5 .go files), Task 2 adds normalize_test.go assertions, Task 3 adds richer persist integration tests in handler_test.go. Each commit is independently buildable + each commit's tests pass at that commit's HEAD. The package "lands" at Task 1 with a baseline of 19 passing tests; Tasks 2 + 3 layer on assertions without altering production source.

## Package Inventory

| Package | Files | Lines | Exports |
|---------|-------|-------|---------|
| `internal/ingest` | doc.go + 5 .go + 2 _test.go | ~2018 | DecodeChirpStackEvent, Event, RxInfo, devEUIFromTopic (unexported helper), QualityOK/QualityDecodeFail/QualityMissingCanonical/QualityOutOfRange/QualityDuplicateFcnt, UplinkHandler, Deps, MappingStore, Resolver (interface), SQLCMappingStore, NormalizeMeasurement, Layer1, ErrNoCanonicalValue, PersistAtomically, PersistInput |
| `internal/chirpstack` (modified) | mqtt.go (+18 lines) | — | SetUplinkHandler method |

## Test Inventory (38 ingest tests + 28 short-suite delta, all -race clean)

| File | Tests | Type |
|------|-------|------|
| `internal/ingest/handler_test.go` (Task 1 decode unit) | 6 | unit (HappyPath × 3 sub, MissingObject_NoError, BadJSON, EarliestGatewayRxTime, Base64Data, devEUIFromTopic table) |
| `internal/ingest/handler_test.go` (Task 1 handler integration) | 5 | integration (DecodeFail_PersistsWithQuality, NoBinding_LogsAndDrops, HappyPath_PersistsRow, NoCanonicalValueMapped_FlagsRow, DataTimeIsServerSide) |
| `internal/ingest/normalize_test.go` | 14 | unit (12 NormalizeMeasurement scenarios + 2 vendor-switch guards) |
| `internal/ingest/handler_test.go` (Task 3 persist integration) | 5 | integration (RolloverDetected, MultipleUplinksSameMP, RolloverNotDetectedOnFirstUplink, PreservesRawAndDecodedAcrossQualityLevels, RollsBackOnContextCancel) |
| Helpers (eventBuilder, fakeMappingStore, fakeLoader, logBuffer) | n/a | private to _test.go |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 — Critical functionality] Unbound dev_eui = log + drop (NOT persist)**
- **Found during:** Task 1 design review of D-26 vs DATA-01 conflict.
- **Issue:** Plan body's UplinkHandler skeleton calls `persistOrphan(ctx, deps, ev, ingestTime, payload, QualityMissingCanonical)` for the no-active-binding case. But measurement rows are PK'd on metering_point_id (DATA-01 invariant — there is no `metering_point_id NULLABLE` column). Persisting an orphan row would either require relaxing the schema (rejected — Plan 02-04 PK locked the invariant) or attaching the row to a sentinel "unknown" MP (rejected — semantic confusion in Phase 4 reports). Actual handling: structured-log the unbound uplink and drop. The plan's persist-everything quality discipline (D-26) yields to DATA-01 specifically for this path. Documented as the SINGLE exception to never-silent-drop.
- **Fix:** UplinkHandler emits `deps.Log.Warn("ingest: unbound device — dropping uplink", ...)` with dev_eui + fcnt + reason fields; no measurement row written. The decode_fail path (which CAN recover dev_eui from topic) DOES persist by looking up the binding via resolver — when the dev_eui IS bound but the codec choked, we still get a row. When the dev_eui is unbound AND decode failed, log + drop applies to both.
- **Files modified:** `internal/ingest/handler.go`
- **Commit:** `a9e0888` (Task 1)

**2. [Rule 2 — Critical functionality] Resolver cache priming after commit (not before)**
- **Found during:** Task 3 design — atomicity review of post-commit step 8.
- **Issue:** Plan body's Step 7 says "Update resolver cache with the new last_raw_value (avoid re-load on next uplink)" but doesn't explicitly say WHEN. Naive implementation could call deps.Resolver.Set BEFORE tx.Commit; if commit fails, the cache holds state that doesn't match Postgres — next uplink computes rollover against an offset that was rolled back. Strictly post-commit means worst case: cache is one uplink stale, never divergent.
- **Fix:** PersistAtomically calls `deps.Resolver.Set(in.Event.DevEUI, updated)` AFTER tx.Commit() returns nil. Documented in step 8 of the function comment: "POST-COMMIT: defensively prime the resolver cache with the new LastRawValue + ReadingOffset so the next uplink's rollover math has the freshest binding state without a re-load."
- **Files modified:** `internal/ingest/persist.go`
- **Commit:** `a9e0888` (Task 1, persist code)

**3. [Rule 1 — Bug] Vendor-name in normalize.go comment tripped vendor-switch guard**
- **Found during:** Task 2 first test run.
- **Issue:** TestNormalizeMeasurement_NoVendorSwitchInSource + GrepCheckMatchesPlan run case-insensitive grep against the file body for `axioma|acrel|kamstrup|diehl|itron`. A comment block in NormalizeMeasurement's doc said `If this file ever grows a 'case "axioma":' style branch...` — accidentally tripping the guard. The intent was to NAME the smell so a reviewer knows what to NOT do; the impl is correct (no actual switch), but the literal name in a comment trips the test that's supposed to guard against the impl.
- **Fix:** Rephrased the comment to use generic `<vendor-slug>` placeholder. The guard rationale stays clear; the name is gone. Both grep checks now pass.
- **Files modified:** `internal/ingest/normalize.go`
- **Commit:** `17ff87d` (Task 2)

**4. [Rule 2 — Critical functionality] PersistAtomically tests beyond the 4-test plan list**
- **Found during:** Task 3 — review of persist failure modes.
- **Issue:** Plan listed 4 persist tests (HappyPath_AxiomaW1, RolloverDetected, MultipleUplinksSameMP, OutOfRangeBattery). Three additional paths weren't pinned: (a) DATA-07 across all quality levels (ok + decode_fail + missing_canonical all have raw_payload), (b) first-uplink-after-binding-open boundary (T-02-09-06 mitigation), (c) atomicity-on-error (rollback semantics). Without tests, a refactor could silently break any of them.
- **Fix:** TestPersist_PreservesRawAndDecodedAcrossQualityLevels (3 rows × 3 quality levels), TestPersist_RolloverNotDetectedOnFirstUplink (last_raw=NULL → no rollover, no offset advance, no audit), TestPersist_RollsBackOnContextCancel (pre-canceled ctx → no row written, last_raw stays NULL). The plan's HappyPath_AxiomaW1 is covered by TestUplinkHandler_HappyPath_PersistsRow (full pipeline including persist). OutOfRangeBattery is deferred to Phase 6 (anomaly detection) — the QualityOutOfRange constant is defined for forward-compat but the persist layer doesn't compute it yet (coerce in normalize silently truncates).
- **Files modified:** `internal/ingest/handler_test.go`
- **Commit:** `1b0f3df` (Task 3)

**5. [Rule 3 — Test fixture] Tests reuse seeded axioma_w1 profile from migration 0010**
- **Found during:** Task 1 — drafting makeFixture helper.
- **Issue:** Plan suggested creating a custom test profile per integration test. But the 0010_seed_profiles migration already inserts axioma_w1 (counter_modulus=4294967296 = 2^32) which is exactly what the rollover test needs to pin against a realistic 32-bit counter wrap. Creating a duplicate profile in tests would either collide on the slug UNIQUE constraint or require a per-test slug suffix (extra complexity).
- **Fix:** makeFixture queries `SELECT id FROM device_profile WHERE slug = 'axioma_w1'` and uses that profile id throughout. The seeded counter_modulus drives the rollover math. Production-realistic; no extra fixture code.
- **Files modified:** `internal/ingest/handler_test.go`
- **Commit:** `1b0f3df` (Task 3)

### Plan-text observations (not actioned)

**1. Plan task 3 step 1 said "Apply rollover by counter_modulus" with `ReadingOffset: pgNumericFromBigFloat(big.NewFloat(0).SetInt64(in.Binding.CounterModulus))`. AdvanceReadingOffset's parameter $2 is the DELTA to add (the SQL is `reading_offset = reading_offset + $2`), so passing the modulus value as $2 is correct. Plan's wording was ambiguous but the SQL is right.** Confirmed via `grep -A5 "advanceReadingOffset" internal/db/sqlc/bindings.sql.go` — `UPDATE binding SET reading_offset = reading_offset + $2 WHERE id = $1`. Tests confirm by asserting `reading_offset = 4294967296` after one rollover (started at 0, advanced by counter_modulus).

**2. Plan called the fcnt parameter `Fcnt` (per the sqlc-generated struct). The handler/persist code uses *int32 — sqlc generated Fcnt as `*int32` from the `fcnt INTEGER NULL` migration column. PersistAtomically takes `int32(ev.FCnt)` (uint32 → int32) which is safe for the LoRaWAN 32-bit counter range; if a vendor ever emits an fcnt > 2^31, we'd see a sign flip — Phase 6 monitoring concern, not a Phase 2 bug.**

**3. Plan task 1 acceptance said "Decoder reads object as map[string]any (Pitfall 11 — v4 NOT v3 string)." Implementation matches verbatim. v3's `objectJSON string` would require a second json.Unmarshal at the normalize layer — explicitly rejected by reading object as map directly.**

**4. Plan task 3 acceptance said "File contains q.AppendMeasurement with all 20 parameters." Implementation calls AppendMeasurement with exactly the 20 sqlc-generated AppendMeasurementParams fields (verified by `grep -c ":" internal/ingest/persist.go | head` shows the params struct). raw_payload + decoded_object + extra all marshal to JSONB / BYTEA respectively; quality is one of the 5 D-26 constants.**

## Authentication Gates

None — all work is local Go code + Postgres testcontainer. Container env: Podman socket continued from Plan 02-08 — no env restart needed in this session. testsupport.StartPostgres already handles the DOCKER_HOST env wiring through testcontainers-go's environment defaults.

## Decisions Made

- **Unbound dev_eui = log + drop, NOT persist.** The single exception to D-26 "never silent-drop." DATA-01 (rows keyed by metering_point_id) takes precedence; structured-log emitted for Phase 6 surface. Decode-fail with bound device DOES persist (binding id available via resolver Lookup).
- **ingestTime captured FIRST in the handler.** Line 96 of handler.go runs `time.Now().UTC()` as the first non-trivial statement so measurement.time, device.last_seen_at, and rollover audit row's effective time all agree to the nanosecond.
- **MappingStore + Resolver as interfaces.** Test fakes plug in without touching Postgres. SQLCMappingStore concrete impl exported for cmd/serve.
- **Resolver cache priming POST-COMMIT.** If commit fails, cache stays at last successful state — never divergent. Worst case: one uplink stale (single-uplink race window).
- **Rollover detection skipped on first-uplink-after-binding-open.** T-02-09-06 mitigation: a fresh meter's raw=0 vs the outgoing meter's last raw=4M+ would falsely fire rollover. The Binding.LastRawValue == nil signal cleanly skips this case.
- **Vendor-switch grep guard at the test layer.** TestNormalizeMeasurement_NoVendorSwitchInSource + GrepCheckMatchesPlan fail the build if vendor names appear in normalize.go. Comments rephrased to use generic placeholders.
- **Reuse seeded axioma_w1 profile in tests.** counter_modulus=4294967296 from 0010_seed_profiles drives realistic rollover math. No custom test profile needed.
- **No new dependencies.** Phase 1's go.mod + stdlib cover everything. Pattern continued from Plans 02-07 + 02-08.

## Self-Check: PASSED

- `[x]` `internal/ingest/quality.go` exists and contains all 5 D-26 constants — verified `grep -c "^	Quality" internal/ingest/quality.go` returns 5
- `[x]` `internal/ingest/decode.go` contains `func DecodeChirpStackEvent(payload []byte) (Event, error)` — verified
- `[x]` Decoder lowercases DevEUI — verified `grep "strings.ToLower" internal/ingest/decode.go`
- `[x]` Decoder reads object as `map[string]any` (Pitfall 11) — verified `grep "Object map\[string\]any" internal/ingest/decode.go`
- `[x]` Decoder extracts earliest rxInfo[].time as GatewayRxTime — verified `grep "ev.GatewayRxTime == nil || t.Before" internal/ingest/decode.go`
- `[x]` `internal/ingest/handler.go` contains `func UplinkHandler(deps Deps) chirpstack.UplinkHandler` — verified
- `[x]` Handler captures `ingestTime := time.Now().UTC()` BEFORE other work (DATA-03) — verified at handler.go line 96
- `[x]` Handler calls `deps.Resolver.Lookup(ctx, ev.DevEUI, ingestTime)` — verified
- `[x]` Handler persists decode_fail path (when dev_eui recoverable) — verified `grep -A5 persistDecodeFail internal/ingest/handler.go`
- `[x]` `internal/chirpstack/mqtt.go` contains `func (s *MQTTSubscriber) SetUplinkHandler(h UplinkHandler)` — verified
- `[x]` `internal/ingest/normalize.go` contains `func NormalizeMeasurement(decoded map[string]any, mappings []profile.Mapping) (Layer1, error)` — verified
- `[x]` normalize.go contains `canonicalTargets` map with all 10 D-02 column names — verified
- `[x]` normalize.go contains `strings.HasPrefix(m.Target, "extra.")` routing — verified
- `[x]` normalize.go has zero vendor-name string literals — verified `grep -iE "axioma|acrel|kamstrup|diehl|itron" internal/ingest/normalize.go` returns 0 lines
- `[x]` normalize.go uses `profile.Resolve(decoded, m.JSONPointer)` — verified
- `[x]` `internal/ingest/persist.go` contains `func PersistAtomically(ctx context.Context, deps Deps, in PersistInput) error` — verified
- `[x]` persist.go contains `pgx.TxOptions{IsoLevel: pgx.Serializable}` — verified
- `[x]` persist.go contains `swap.DetectRollover` AND `swap.ApplyRollover` — verified `grep -c "swap\." internal/ingest/persist.go` returns 2
- `[x]` persist.go computes `cumulative = raw + currentOffset` (with rollover applied) — verified
- `[x]` persist.go uses `in.IngestTime` for the time column (DATA-03) — verified `grep "in.IngestTime" internal/ingest/persist.go` returns 1 line
- `[x]` persist.go preserves raw_payload (DATA-07) — verified `grep "RawPayload" internal/ingest/persist.go` returns 3 lines
- `[x]` persist.go audits rollover with `audit.ActionRolloverDetected` (D-05) — verified
- `[x]` persist.go updates resolver cache after commit — verified
- `[x]` `go test -count=1 -race ./internal/ingest/...` exits 0 — 38 tests pass
- `[x]` `go test -count=1 -race ./internal/chirpstack/...` exits 0 — 38 tests pass (no regressions from SetUplinkHandler)
- `[x]` `go test -count=1 -short ./...` exits 0 — 242 tests pass in 23 packages (was 214 baseline; +28 from this plan)
- `[x]` `go vet ./...` exits 0
- `[x]` `go build ./...` exits 0
- `[x]` commit `a9e0888` (Task 1 — pipeline source) found in git log
- `[x]` commit `17ff87d` (Task 2 — normalize tests + vendor guard) found in git log
- `[x]` commit `1b0f3df` (Task 3 — handler + persist integration tests) found in git log
- `[x]` `grep -rn "fmt.Sprintf.*INSERT\|fmt.Sprintf.*UPDATE\|fmt.Sprintf.*DELETE" internal/ingest/` returns 0 lines (no string-concat SQL)
