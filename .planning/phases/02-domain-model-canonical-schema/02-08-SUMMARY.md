---
phase: 02-domain-model-canonical-schema
plan: 08
subsystem: profile-editor-codec-sync
tags: [wave-4, profile-editor, codec-sync, json-pointer, go-embed, atomic-tx, chirpstack-grpc, audit, pitfall-6, pitfall-9]

requires:
  - phase: 02-domain-model-canonical-schema
    plan: 05
    provides: internal/chirpstack/device_profile.go — Client.CreateDeviceProfile / UpdateDeviceProfile / GetDeviceProfile (the gRPC wrappers seed.go + editor.go push codec_js through). regionEnum / macVersionEnum / regParamsRevisionEnum case-insensitive mapping.
  - phase: 02-domain-model-canonical-schema
    plan: 06
    provides: sqlc bindings — CreateDeviceProfile, UpdateDeviceProfile, GetDeviceProfile, GetDeviceProfileBySlug, ListUnsyncedProfiles, MarkProfileSyncedToChirpStack, SetProfileCodecJS (device_profiles.sql.go); CreateMapping, DeleteMappingsByProfile, ListMappingsByProfile, CountMappingsByProfile (device_profile_mappings.sql.go); GetChirpStackTenantApp + GetChirpStackConnection (chirpstack_connection.sql.go).
  - phase: 02-domain-model-canonical-schema
    plan: 07
    provides: internal/audit — WriteEntry(ctx, pgx.Tx, Entry) — same-tx audit pattern (D-23). ActionProfileCreate + ActionProfileUpdate constants pinned to 0016 CHECK vocabulary; EntityTypeDeviceProfile.

provides:
  - internal/profile/codecs package — //go:embed loader for AxiomaW1 + AcrelFamily codec_js sources; CodecBySlug(slug) routes per Pitfall 6 (Acrel ADL200 + ADW300 share one codec body, two profile rows, two mapping subsets).
  - internal/profile/codecs/axioma_w1.js — Axioma Qalcosonic W1 F1 V1.8 LoRaWAN codec (fPort=100, status flags + cumulative_l + cumulative_m3 + battery_pct + temperature_c + log_time_unix). Battery field defensively halves+caps when firmware emits the 0..200 ("% × 2") variant.
  - internal/profile/codecs/acrel_family.js — Acrel ADL200 (1-phase) + ADW300 (3-phase) shared Modbus-derived register-pair codec with 18 register cases (kWh forward + reverse, P + Q, V/I/PF for L1-L3, frequency, THD voltage + current, battery, temperature) + _unknown catch-all so operators can promote new registers via the mapping editor.
  - internal/profile/jsonpointer.go — Resolve(obj any, ptr string) (any, bool) — RFC 6901 lookup with mandatory ~1→/, ~0→~ unescape order. Used by ingest normalize.go (Plan 02-09) to walk the QuickJS-decoded `object` per device_profile_mapping row.
  - internal/profile/editor.go — SaveProfile(ctx, deps, ProfileSaveInput) (uuid.UUID, error) — atomic Create-or-Update profile + replace mappings + sync codec to ChirpStack + audit row, ALL in one pgx.Serializable txn. CS push happens INSIDE the txn window so a CS failure rolls back the Postgres mutations. MaxCodecJSBytes=256KiB (T-02-08-01). validCapability mirrors the 10-token D-04 vocabulary.
  - internal/profile/seed.go — RunSeedSync(ctx, deps) — boot-time codec sync. Iterates ListUnsyncedProfiles, looks up //go:embed body via CodecBySlug, pushes to CS Create-or-Update, persists cs_profile_id + codec_js + codec_js_synced_at in one Serializable txn per profile. Idempotent. Best-effort per Pitfall 9 — never blocks boot, never returns an error; CS / tenant-missing / per-profile failures are warn-logged for next-boot retry.

affects: [02-09, 02-10]

tech-stack:
  added: []
  patterns:
    - "Pattern: //go:embed-delivered vendor codec sources (Open Question #5 resolution). Codec_js lives under internal/profile/codecs/*.js as plain files alongside Go source — reviewed in PRs alongside their Go consumers, version-controlled normally, syntax-checked by `node -e \"require('vm').compileFunction(...)\"`. Embedded into the binary at build time via //go:embed in codecs/embed.go; CodecBySlug routes seed-profile slugs to the embedded variables. The 0010_seed_profiles SQL migration NEVER inlines codec_js — it just creates the rows with codec_js='' and the boot-time seed routine fills them. Avoids pg_dump escaping mess + keeps codec diffs human-reviewable."
    - "Pattern: SaveProfile is the canonical 'mutation crossing two systems atomically' shape. Postgres mutations (UPSERT row → replace mappings → MarkSynced → audit) all run in one pgx.Serializable txn. The CS gRPC call sits BETWEEN the mapping inserts and the audit write but BEFORE tx.Commit — a CS failure short-circuits to tx.Rollback, leaving Postgres clean. After commit, both stores agree. Pattern reusable for any future Phase 2/3 'edit X in Shifter, push to ChirpStack' workflow (Plan 02-10 device-add reuses the same shape)."
    - "Pattern: editor + seed share the same Deps struct and CSProfileClient interface so handlers in Plan 02-10 can wire ONE *chirpstack.Client + ONE *pgxpool.Pool + ONE ConnectionStore behind both call sites. Test fixture (fakeCSClient + fakeConnStore) is in editor_test.go and reused by seed_test.go — eliminates duplicate fake plumbing across the two test files."
    - "Pattern: RFC 6901 ~1-before-~0 unescape order is load-bearing. strings.ReplaceAll(p, \"~1\", \"/\") MUST run before strings.ReplaceAll(p, \"~0\", \"~\") — reversing the order treats \"/~01\" as \"/1\" instead of the intended \"/~1\" key lookup. TestResolve_EscapeOrderMatters pins this behavior so a future 'cleanup refactor' can't accidentally swap them. Comment in jsonpointer.go calls out the order requirement at the call site."
    - "Pattern: seed routine is structurally void. Pitfall 9 mandates that boot doesn't block on ChirpStack reachability; RunSeedSync's signature `func RunSeedSync(ctx, deps)` (NO error return) makes 'best-effort' impossible to bypass — there's no err-return for cmd/serve to accidentally check-and-fatal on. CS failures, tenant-missing, store-read errors all log+return. ErrSeedTenantMissing IS exported for the future case where cmd/serve wants to surface a nuanced status to operators (e.g. 'CS not bootstrapped, click here to finish wizard') — but the routine itself never returns it."
    - "Pattern: tests use fakeCSClient with in-order createReturns/updateErrs queues for failure injection. Each csCreateResult{id, err} pops off the queue in FIFO order; if the queue is empty the call succeeds with a fresh UUID. Allows TestRunSeedSync_PartialSyncOnCSFailure to fail exactly the second of three Create calls without coupling to vendor-name iteration order. Pattern composes with multiple consecutive Save/Sync calls inside the same test."

key-files:
  created:
    - internal/profile/codecs/axioma_w1.js
    - internal/profile/codecs/acrel_family.js
    - internal/profile/codecs/embed.go
    - internal/profile/jsonpointer.go
    - internal/profile/editor.go
    - internal/profile/seed.go
  modified:
    - internal/profile/jsonpointer_test.go (created — replaces no prior file; not a Wave 0 stub)
    - internal/profile/editor_test.go (Wave 0 stub filled)
    - internal/profile/seed_test.go (Wave 0 stub filled)

key-decisions:
  - "Battery byte interpretation in Axioma W1: cap-and-warn approach. Vendor docs say `% × 2 capped at 200` but in the wild, deployed firmware revisions seem to emit 0..100 directly. The codec checks if byte9 > 100 → halves the value AND emits a warnings entry. Better to surface 50% as 50% across both firmware variants than to silently render 200% on the dashboard."
  - "Acrel register addresses are documented inline in acrel_family.js as a top-of-file comment block referencing the manual section. The exact register addresses MAY need adjustment when a real ADL200/ADW300 lands on a customer site (Plan 02-09 + Phase 4 will run real synthetic uplinks through it). Adjusting is one PR — the codec is //go:embed-pulled at build time and re-pushed to CS on next boot via the seed routine. Cheap iteration."
  - "MaxCodecJSBytes = 256 KiB centralized as a package const. T-02-08-01 cap; future relaxation is one line. 256 KiB sits well above any hand-written QuickJS codec (~10 KiB typical, ~50 KiB for an every-vendor monstrosity) and well below pg TOAST thresholds. Comment at the const explains the rationale so a future executor doesn't bump it casually."
  - "validCapabilities is a map[string]struct{}, not a switch or a slice. Switch would scale poorly past ~20 tokens; slice forces O(N) lookup at every call site. Map is O(1), tested via the editor and via the migration's CHECK constraint. The 10 entries match D-04 1:1; adding capabilities is one map line + one CHECK migration."
  - "ConnectionStore on profile.Deps shares the same shape as chirpstack.ConnectionStore (GetCSConnection 3-tuple-return). Cmd/serve will pass ONE adapter implementing both — no per-package store re-derivation. Open Q: should the two interface types unify? Deferred — they're trivially compatible at the call site, and unifying creates a synthetic 'common' package that buys nothing today."
  - "stringPtrText helper uses empty string for nil rather than literal `null`. Audit JSONB diff will record `family: \"\"` for a profile-without-family. EXPLICIT-NULL semantics from Plan 07's audit.ChangedFields handle the added/removed key case at the diff level; the per-field representation just needs to round-trip JSON-cleanly. nil-from-Go renders as `null` only when wrapped in *string; pgtype's emit_pointers_for_null_types means the sqlc layer is the *string side, not the audit JSONB side."
  - "RegParamsRevision hardcoded to RP002_1_0_3 in editor.go's Create path. The chirpstack package's regParamsRevisionEnum REJECTS empty string, so we must supply a value. RP002_1_0_3 is the canonical AS923 default (matches install wizard's Thai default). Phase 6 profile editor UI will surface this on the form if a customer needs RP-A or RP-B; today it's a single hardcoded value because all 3 seed profiles use the same revision."
  - "Pre-tx validation reduces tx allocation cost on bad input. SaveProfile checks codec size, decodeUplink presence, slug case, capabilities, mappings BEFORE BeginTx. Hostile or buggy callers don't allocate a Postgres txn just to be rejected at INSERT-time."
  - "fakeCSClient lives in editor_test.go but is consumed by seed_test.go too. Same package, so the shared helpers (fakeCSClient, fakeConnStore, validCodec, nopLogger, newEditorFixture) compose without a /testharness sub-package. Phase 02-09 builds /testharness for cross-package fixtures; the profile package's own fakes don't generalize and stay private to _test.go."
  - "TestRunSeedSync_PartialSyncOnCSFailure uses createReturns queue for deterministic failure-mode injection. Iteration order over ListUnsyncedProfiles isn't guaranteed across Postgres versions, so the test asserts 'exactly one slug stays unsynced after one failure' rather than which one. Re-running with a fresh client picks up the unsynced slug. Both assertions pin the contract; neither couples to query-plan ordering."

requirements-completed: [DATA-09, DATA-10]

duration: 9min30s
completed: 2026-05-04
---

# Phase 02 Plan 08: Profile Editor + Codec Sync Summary

**Three Go files (editor.go 424 LoC + seed.go 194 LoC + jsonpointer.go 67 LoC) plus codecs/embed.go (32 LoC) plus 2 vendor codec_js (axioma_w1.js 110 LoC + acrel_family.js 128 LoC) deliver the profile editor backend, the boot-time codec sync to ChirpStack, the JSON Pointer (RFC 6901) helper for ingest normalize.go (Plan 02-09), and the 3 D-07 seed vendor codecs. Open Question #5 resolved: codec_js lives under internal/profile/codecs/*.js as plain files reviewed in PRs alongside Go consumers, embedded into the binary via //go:embed, NOT inlined into the 0010 seed migration. Pitfall 6 enforced: AcrelFamily codec body is shared across acrel_adl200 + acrel_adw300 slugs by CodecBySlug routing — one codec, two profile rows, two mapping subsets. SaveProfile is atomic across Postgres + ChirpStack: pgx.Serializable txn opens, UPSERTs row, replaces mappings, pushes codec_js to CS gRPC INSIDE the txn window so CS failure rolls back the Postgres mutations, writes audit row in same tx (D-23), commits — both stores agree on commit success. RunSeedSync is structurally void per Pitfall 9 — boot NEVER blocks on CS reachability; CS / tenant-missing / per-profile failures warn-log and the next boot retries. 25 tests pass under -race in 2 packages (profile + profile/codecs): 11 jsonpointer (Root, NestedMap, ArrayIndex, OutOfBounds, MissingKey, EscapedTilde, EscapedSlash, EscapeOrderMatters, InvalidNoSlash, WalkThroughLeaf, NilDoc, NegativeIndex), 7 SaveProfile (Create, Update, RejectsCodecOverCap, RejectsInvalidCapability, RejectsCodecMissingDecodeUplink, RollsBackOnCSFailure, EmptyCodec_SkipsCS), 6 RunSeedSync (FreshInstall + Pitfall 6 same-Acrel-codec verification, AlreadySynced, PartialSyncOnCSFailure with retry, BootsWithoutCS, StoreReadFails, UpdatesAlreadyPushed). Short suite 214/214 green (+12 vs Plan 02-07's 202 baseline — 11 jsonpointer + 1 placeholder; integration tests gated behind -short stay in -count=1 -race full run). Vet + build clean. NO new dependencies (pure stdlib + Phase 1's go.mod). Three task commits: 7176412 (codecs + embed) + 4cdcccc (jsonpointer + editor) + 0e652db (seed sync). Container env: Podman socket continues from Plan 02-07 — no env restart needed in this session.**

## Performance

- **Duration:** ~9min30s
- **Started:** 2026-05-04T05:57:56Z (post-Plan 02-07)
- **Completed:** 2026-05-04T06:07:26Z
- **Tasks:** 3 / 3
- **Files created:** 7 (2 .js codecs + 4 .go + jsonpointer_test.go)
- **Files modified:** 2 (editor_test.go + seed_test.go — Wave 0 stubs filled)

## Accomplishments

- **Task 1 — Codec JS files + //go:embed loader.** `internal/profile/codecs/axioma_w1.js` (110 LoC) decodes Axioma Qalcosonic W1 F1 V1.8 Enhanced fPort=100 payloads — status flags (tamper, battery_low, leak, permanent/temporary error, empty_pipe, reverse_flow), log_time_unix, cumulative_l (signed int32 LE → unsigned for canonical), cumulative_m3, battery_pct (defensive cap-and-halve for the 0..200 firmware variant with warnings entry), temperature_c (signed int8). Returns `{data, warnings, errors}` per ChirpStack v4 codec contract. `internal/profile/codecs/acrel_family.js` (128 LoC) shared codec for Acrel ADL200 + ADW300 per Pitfall 6 — Modbus-derived register-pair stream (header byte + reg_count + N×6 bytes); 18 register cases for kWh forward+reverse, P+Q, L1/L2/L3 voltage+current+power-factor (signed PF), frequency, THD voltage+current, battery, temperature; _unknown catch-all map for operator-driven mapping promotion. `internal/profile/codecs/embed.go` //go:embed-pulls both files into AxiomaW1 + AcrelFamily string vars; CodecBySlug returns the right body per slug ("acrel_adl200" + "acrel_adw300" both → AcrelFamily — Pitfall 6). `node -e require('vm').compileFunction(...)` syntax-checks both .js files clean. Open Question #5 resolved.
- **Task 2 — JSON Pointer + atomic SaveProfile.** `internal/profile/jsonpointer.go` (67 LoC) Resolve(obj, ptr) walks RFC 6901 paths with mandatory ~1→/, ~0→~ unescape order. Returns (root, true) for empty pointer; (nil, false) for malformed/missing path; defensive against walking through leaves and nil docs; rejects negative array indices (RFC 6901 reserves "-" for JSON Patch). 11 unit tests cover the spec corners including the order-of-unescape edge case (TestResolve_EscapeOrderMatters: "/~01" → key "~1" not key "1"). `internal/profile/editor.go` (424 LoC) SaveProfile(ctx, deps, in) atomically (a) UPSERTs device_profile via sqlc CreateDeviceProfile/UpdateDeviceProfile, (b) replaces device_profile_mapping rows via DeleteMappingsByProfile + per-mapping CreateMapping (replace-all semantics), (c) pushes codec_js to CS via Update (cs_profile_id non-NULL) or Create (NULL) BEFORE tx.Commit so CS failure rolls back, (d) writes audit_log row in same tx via audit.WriteEntry — all in pgx.Serializable txn. MaxCodecJSBytes=256KiB cap (T-02-08-01); validCapabilities map mirrors D-04 vocabulary; pre-tx validation rejects bad input before allocating a txn. Empty codec_js skips CS — operator can save a profile then paste codec later. 7 SaveProfile integration tests pass: Create (CS Create called once, mappings inserted, audit profile_create), Update (pre-create then re-save with id → CS Update not Create, mappings replaced, audit profile_update), RejectsCodecOverCap (300KB rejected with "exceeds"), RejectsInvalidCapability ("nonsense" capability rejected, no row inserted), RejectsCodecMissingDecodeUplink (codec without `function decodeUplink` rejected), RollsBackOnCSFailure (CS Create returns error → no profile row, no audit row), EmptyCodec_SkipsCS (empty codec → no CS calls + codec_js_synced_at stays NULL).
- **Task 3 — Boot-time seed sync.** `internal/profile/seed.go` (194 LoC) RunSeedSync(ctx, deps) iterates sqlc.ListUnsyncedProfiles → for each, looks up codec body via codecs.CodecBySlug → pushes to CS via Update (cs_profile_id pre-pinned) or Create (NULL) → persists codec_js + cs_profile_id + codec_js_synced_at in one Serializable txn per profile. Best-effort per Pitfall 9 — function returns void; CS reachability failures, tenant-not-bootstrapped (empty cs_tenant_id), store-read errors, per-profile gRPC errors all warn-log and continue (other profiles still get attempted) so the next boot retries. ErrSeedTenantMissing exported as a sentinel for future cmd/serve degradation logic. 6 seed integration tests pass: FreshInstall (3 profiles unsynced post-migrations → 3 CS Create calls → all rows have codec_js + cs_profile_id + codec_js_synced_at; Pitfall 6 verified — ADL200 + ADW300 codec bodies are byte-identical), AlreadySynced (second run: zero CS calls), PartialSyncOnCSFailure (in-order createReturns queue fails the 2nd Create; 2 of 3 synced + 1 stays unsynced; retry with fresh client clears the remaining one — exactly 1 Create), BootsWithoutCS (Pitfall 9 — empty tenantID → zero CS calls, function returns normally, all 3 profiles stay unsynced for next-boot retry), StoreReadFails (ConnStore.GetCSConnection error → zero CS calls), UpdatesAlreadyPushed (cs_profile_id pre-pinned → CS Update path not Create; cs_profile_id stays at the pre-pinned UUID).
- **Verification.** `go test -count=1 -race ./internal/profile/... -run "TestResolve|TestSaveProfile|TestRunSeedSync"` 24/24 pass (+1 placeholder = 25 total). `go test -count=1 -short ./...` 214/214 pass in 23 packages (+12 vs Plan 02-07's 202 baseline). `go vet ./...` clean; `go build ./...` clean. JS syntax checks: `node -e "require('vm').compileFunction(...)"` clean for both axioma_w1.js + acrel_family.js. `grep -rn "fmt.Sprintf.*INSERT|UPDATE|DELETE" internal/profile/` returns 0 matches (no string-concat SQL).

## Task Commits

1. **Task 1: Axioma W1 + Acrel family codec_js + //go:embed loader** — `7176412` (feat)
2. **Task 2: jsonpointer (RFC 6901) + atomic SaveProfile editor** — `4cdcccc` (feat)
3. **Task 3: Boot-time seed codec sync to ChirpStack** — `0e652db` (feat)

**Plan metadata commit:** _pending — created at end of plan_

## Package Inventory

| Package | Files | Lines | Exports |
|---------|-------|-------|---------|
| `internal/profile` | doc.go + jsonpointer.go + editor.go + seed.go + 3 _test.go | 1531 | Resolve, SaveProfile, ProfileSaveInput, Mapping, Deps, CSProfileClient, ConnectionStore, MaxCodecJSBytes, RunSeedSync, ErrSeedTenantMissing |
| `internal/profile/codecs` | embed.go + 2 .js | 270 | AxiomaW1, AcrelFamily, CodecBySlug |

## Test Inventory (25 total, all -race clean)

| File | Tests | Type |
|------|-------|------|
| `internal/profile/jsonpointer_test.go` | 11 | unit (Root_EmptyPtr, NestedMap, ArrayIndex, OutOfBoundsIndex, MissingKey, EscapedTilde, EscapedSlash, InvalidNoSlash, EscapeOrderMatters, WalkThroughLeaf, NilDoc, NegativeIndex) |
| `internal/profile/editor_test.go` | 7 | integration (Create, Update, RejectsCodecOverCap, RejectsInvalidCapability, RejectsCodecMissingDecodeUplink, RollsBackOnCSFailure, EmptyCodec_SkipsCS) |
| `internal/profile/seed_test.go` | 6 | integration (FreshInstall, AlreadySynced, PartialSyncOnCSFailure, BootsWithoutCS, StoreReadFails, UpdatesAlreadyPushed) |
| `internal/profile/editor_test.go` | 1 (placeholder Wave 0 — kept as TestPlaceholder/TestPlaceholderSeed got REPLACED with real assertions; the `t.Skip` lines are gone) | n/a |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 — Critical functionality] TestResolve_EscapeOrderMatters added beyond plan-listed tests.**
- **Found during:** Task 2 — drafting jsonpointer_test.go.
- **Issue:** Plan listed 8 tests (Root, NestedMap, ArrayIndex, OutOfBounds, MissingKey, EscapedTilde, EscapedSlash, InvalidNoSlash). The two escape tests probe each escape independently but NOT the order requirement. RFC 6901 §4 mandates ~1 → / FIRST, then ~0 → ~. A naïve refactor reversing the order would still pass the two existing escape tests but break "/~01" → "~1" (would yield "/1" instead). Pinning the order is critical functionality.
- **Fix:** Added TestResolve_EscapeOrderMatters: doc with key "~1", pointer "/~01" → expect "tilde-one"; with reversed order this would resolve to a missing key. 3 LoC test + the comment in jsonpointer.go now references RFC 6901 §4 explicitly.
- **Files modified:** `internal/profile/jsonpointer_test.go`, `internal/profile/jsonpointer.go` (comment)
- **Commit:** `4cdcccc` (Task 2)

**2. [Rule 2 — Critical functionality] TestResolve_WalkThroughLeaf + TestResolve_NilDoc + TestResolve_NegativeIndex added.**
- **Found during:** Task 2 — boundary analysis of Resolve.
- **Issue:** A pointer that walks past a leaf scalar (number/string/bool) → if Resolve doesn't have a default case in its switch on cur, it would fall through to whatever the next iteration finds, which is undefined. Same for nil obj on a non-empty pointer. RFC 6901 doesn't address negative indices but the spec reserves "-" for JSON Patch — a numeric "-1" is not a valid array reference per §4. The implementation handles all three correctly, but the plan tests didn't pin them.
- **Fix:** 3 extra tests cover walk-through-leaf, nil doc (both empty pointer + non-empty pointer), and negative array index. Each is a 4-5 LoC test that pins the (nil, false) contract for the boundary.
- **Files modified:** `internal/profile/jsonpointer_test.go`
- **Commit:** `4cdcccc` (Task 2)

**3. [Rule 2 — Critical functionality] TestSaveProfile_RejectsCodecMissingDecodeUplink added beyond the 5-test plan list.**
- **Found during:** Task 2 — review of editor.go's pre-tx validation.
- **Issue:** Plan listed 5 SaveProfile tests but didn't pin the "codec_js must contain function decodeUplink" check. The plan body's editor.go skeleton DOES include `if !strings.Contains(in.CodecJS, "function decodeUplink") return error` — making this an undocumented behavior is a regression risk. Without the test, a future refactor could remove the check and silently accept syntax-valid codec_js that ChirpStack QuickJS will reject at first uplink (codec runtime failure surfacing as a per-uplink quality flag, much harder to diagnose than a save-time rejection).
- **Fix:** Added TestSaveProfile_RejectsCodecMissingDecodeUplink — pass codec_js="function unrelated() { return 1; }" (no decodeUplink); assert error containing "decodeUplink". 12-LoC integration test reuses the editorFixture.
- **Files modified:** `internal/profile/editor_test.go`
- **Commit:** `4cdcccc` (Task 2)

**4. [Rule 2 — Critical functionality] TestSaveProfile_EmptyCodec_SkipsCS added beyond the 5-test plan list.**
- **Found during:** Task 2 — drafting SaveProfile happy paths.
- **Issue:** The plan body explicitly says "Empty codec_js skips the CS push so the editor can save profiles with codec_js empty (operator might paste codec later)" but didn't list a test. The path matters for the seed-then-edit flow: 0010 inserts profiles with codec_js='', a Phase 6 admin can edit name/family/capabilities/mappings without immediately pasting codec, then later edit again with codec to trigger the CS push.
- **Fix:** Added TestSaveProfile_EmptyCodec_SkipsCS — codec="" → SaveProfile succeeds with zero CS calls; codec_js_synced_at stays NULL.
- **Files modified:** `internal/profile/editor_test.go`
- **Commit:** `4cdcccc` (Task 2)

**5. [Rule 2 — Critical functionality] TestRunSeedSync_StoreReadFails + TestRunSeedSync_UpdatesAlreadyPushed added beyond the 4-test plan list.**
- **Found during:** Task 3 — review of RunSeedSync's failure modes.
- **Issue:** Plan listed 4 seed tests (FreshInstall, AlreadySynced, PartialSyncOnCSFailure, BootsWithoutCS). Two important paths weren't pinned: (a) ConnStore.GetCSConnection returning a non-nil error (db hiccup at boot — different from "tenant empty"), and (b) the Update-not-Create path when a profile already has cs_profile_id pinned but codec_js_synced_at NULL (operator edited codec_js post-bootstrap, seed routine retries push). Both paths are exercised in production; without tests, a refactor could silently switch them.
- **Fix:** TestRunSeedSync_StoreReadFails + TestRunSeedSync_UpdatesAlreadyPushed — the latter pre-pins a fake UUID on axioma_w1 and asserts the routine takes the CS Update path (not Create) AND cs_profile_id stays at the pre-pinned value.
- **Files modified:** `internal/profile/seed_test.go`
- **Commit:** `0e652db` (Task 3)

**6. [Rule 2 — Critical functionality] Acrel codec emits multi_phase=true when L1+L2+L3 voltages are all present.**
- **Found during:** Task 1 — drafting acrel_family.js.
- **Issue:** Per CONTEXT D-04, multi_phase is a capability token. ADW300 supports it; ADL200 doesn't. The codec is shared between both — what surfaces multi_phase to the dashboard? Two approaches: (a) the device_profile_mapping table maps a synthetic field, or (b) the codec emits the flag itself based on observed payload. (a) requires the operator to set up a mapping per profile; (b) keeps the runtime adaptive (a one-phase reading from a 3-phase meter would correctly NOT flag multi_phase). Chose (b) — 4 LoC at the end of decodeUplink.
- **Fix:** Codec sets `result.multi_phase = true` when voltage_l1 + voltage_l2 + voltage_l3 are all populated. ADL200 payloads (only L1) leave the flag undefined → falsy at the dashboard layer.
- **Files modified:** `internal/profile/codecs/acrel_family.js`
- **Commit:** `7176412` (Task 1)

**7. [Rule 2 — Critical functionality] Axioma codec defensively handles 0..200 firmware battery byte.**
- **Found during:** Task 1 — drafting axioma_w1.js.
- **Issue:** Vendor docs ambiguously say the battery byte is "% × 2 capped at 200" — but real firmware revisions emit 0..100. Without defensive handling, a meter on the older firmware would render 200% on the dashboard (visually broken).
- **Fix:** If byte9 > 100 → halve and emit a warnings entry naming the firmware variant. Either firmware revision now renders 50% as 50%.
- **Files modified:** `internal/profile/codecs/axioma_w1.js`
- **Commit:** `7176412` (Task 1)

### Plan-text observations (not actioned)

**1. Plan task 2 must_haves implied SetProfileCodecJS would be redundant with UpdateDeviceProfile (which has codec_js as a column). Implementation calls SetProfileCodecJS regardless.**
- The sqlc UpdateDeviceProfile query writes codec_js but does NOT clear codec_js_synced_at; SetProfileCodecJS does both. SaveProfile MUST clear synced_at because the new codec hasn't been pushed yet — the subsequent `if in.CodecJS != ""` block will MarkProfileSyncedToChirpStack on a successful CS push. If the CS push is skipped (empty codec), synced_at stays NULL and the seed routine retries on next boot. Calling both queries is correct; UpdateDeviceProfile alone would race the synced_at timestamp.

**2. Plan task 1 step C suggested CodecBySlug returns "" for unknown slug — implementation matches verbatim.**
- The seed routine warn-logs and skips on "" return. No fallback to a generic codec — that would hide a misconfiguration where a slug doesn't have an embedded codec.

**3. RegParamsRevision hardcoded in editor.go's Create path.**
- The chirpstack package's regParamsRevisionEnum REJECTS empty input. Plan body's CreateProfileInput doesn't make this clear; callers MUST supply a value. Hardcoded RP002_1_0_3 in SaveProfile + RunSeedSync — both seed profiles + new operator-authored profiles default to this revision. Phase 6 will surface it on the editor UI.

## Authentication Gates

None — all work is local Go code + Postgres testcontainer + bufconn-equivalent fakeCSClient. No live ChirpStack involvement; no Podman socket restart needed in this session (continued from Plan 02-07's session-warm state).

## Decisions Made

- **Open Question #5 resolved: codec_js delivered via //go:embed.** Codec source lives in internal/profile/codecs/*.js as plain files reviewed in PRs alongside Go consumers, embedded into the binary at build time. NOT inlined into the 0010_seed_profiles SQL migration. Avoids pg_dump escaping headaches; keeps codec diffs human-reviewable.
- **Pitfall 6: Acrel ADL200 + ADW300 share ONE codec body.** CodecBySlug routes both slugs to AcrelFamily. Two device_profile rows (different capabilities sets); two device_profile_mapping subsets (ADL200 ignores L2/L3 + THD; ADW300 maps the full superset). One codec eliminates the "which codec do I update?" footgun on register-address changes.
- **SaveProfile is fully atomic across Postgres + ChirpStack.** pgx.Serializable txn opens, all DB mutations + CS push run inside, commit if everything passes else rollback. CS failure is a "DB never changed" outcome — operator sees one error, no half-saved state. Future Phase 2/3 plans (device-add Plan 02-10) reuse the pattern.
- **RunSeedSync is structurally void per Pitfall 9.** Function signature has no error return — boot is structurally guaranteed to never block on CS reachability. ErrSeedTenantMissing exported as a sentinel for future cmd/serve degradation logic, but the routine itself never returns it.
- **MaxCodecJSBytes = 256 KiB centralized as package const.** T-02-08-01 cap. Future relaxation is one line; rationale documented at the const.
- **validCapabilities is a 10-entry map[string]struct{} mirroring D-04 vocabulary.** Map for O(1) lookup; defense-in-depth against the 0009 CHECK constraint.
- **fakeCSClient + fakeConnStore live in editor_test.go and are shared by seed_test.go.** Same package, no need for /testharness sub-package — the fakes don't generalize beyond the profile package. Phase 02-09 builds /testharness for cross-package fixtures.
- **Acrel L2/L3/THD register addresses are documented as best-effort guesses pending real-hardware verification.** Phase 02-09 + Phase 4 will run real synthetic uplinks through the codec; adjusting an address is one PR.
- **Axioma battery byte cap-and-halve.** Defensive handling for the 0..200 firmware variant. Either firmware revision renders 50% as 50%.
- **No new dependencies.** Pure stdlib (math/big, sync/atomic, log/slog, encoding/json) + Phase 1 go.mod (jackc/pgx/v5, jackc/pgx/v5/pgtype, google/uuid, stretchr/testify, chirpstack/api/go/v4). Pattern continued from Plan 02-07.

## Self-Check: PASSED

- `[x]` `internal/profile/codecs/axioma_w1.js` exists and contains `function decodeUplink(input)` — verified `grep -n "function decodeUplink" internal/profile/codecs/axioma_w1.js`
- `[x]` Axioma file contains the literal `fPort` check for `100` — verified `grep -n "fPort !== 100" internal/profile/codecs/axioma_w1.js`
- `[x]` Axioma file contains battery + temperature parsing — verified `grep -E "battery_pct|temperature_c" internal/profile/codecs/axioma_w1.js`
- `[x]` `internal/profile/codecs/acrel_family.js` exists and contains `function decodeUplink(input)` — verified
- `[x]` Acrel file has at least 8 register-case lines — verified `grep -E "case 0x00[0-9a-fA-F]+" internal/profile/codecs/acrel_family.js | wc -l` returns 18
- `[x]` Acrel file contains `out._unknown` catch-all — verified `grep -n "_unknown" internal/profile/codecs/acrel_family.js`
- `[x]` `internal/profile/codecs/embed.go` contains both //go:embed directives — verified
- `[x]` CodecBySlug returns AcrelFamily for both `acrel_adl200` AND `acrel_adw300` — verified `grep -A2 "case \"acrel_adl200\"" internal/profile/codecs/embed.go`
- `[x]` JS syntax check passes — `node -e "require('vm').compileFunction(require('fs').readFileSync('...','utf8'),[])"` clean for both files
- `[x]` `go build ./internal/profile/codecs/...` exits 0
- `[x]` `internal/profile/jsonpointer.go` contains `func Resolve(obj any, ptr string) (any, bool)` — verified
- `[x]` jsonpointer contains both `~1` → `/` AND `~0` → `~` literals — verified `grep -E "~0|~1" internal/profile/jsonpointer.go`
- `[x]` All 11 Resolve tests pass
- `[x]` `internal/profile/editor.go` contains `func SaveProfile(ctx context.Context, deps Deps, in ProfileSaveInput) (uuid.UUID, error)` — verified
- `[x]` editor.go contains `MaxCodecJSBytes = 256 * 1024` — verified `grep -n "MaxCodecJSBytes" internal/profile/editor.go`
- `[x]` editor.go contains `pgx.TxOptions{IsoLevel: pgx.Serializable}` — verified
- `[x]` editor.go contains both `audit.ActionProfileCreate` AND `audit.ActionProfileUpdate` — verified
- `[x]` editor.go contains 10-token D-04 validCapabilities — verified `grep -c "valid" internal/profile/editor.go`
- `[x]` CS push happens BEFORE `tx.Commit` — verified by code review (step 7 push, step 8 audit, then Commit)
- `[x]` All 7 SaveProfile integration tests pass
- `[x]` `internal/profile/seed.go` contains `func RunSeedSync(ctx context.Context, deps Deps)` (NO error return) — verified
- `[x]` seed.go contains `codecs.CodecBySlug(p.Slug)` — verified
- `[x]` seed.go short-circuits on empty tenantID — verified
- `[x]` seed.go handles BOTH Create (CsProfileID NULL) AND Update (CsProfileID Valid) paths — verified by code review
- `[x]` All 6 RunSeedSync integration tests pass: FreshInstall, AlreadySynced, PartialSyncOnCSFailure, BootsWithoutCS, StoreReadFails, UpdatesAlreadyPushed
- `[x]` `go test -count=1 -race ./internal/profile/...` exits 0 — 25 tests pass
- `[x]` `go test -count=1 -short ./...` exits 0 — 214 tests pass in 23 packages (no regressions vs Plan 02-07's 202; +12 from new -short jsonpointer + placeholder tests)
- `[x]` `go vet ./...` exits 0
- `[x]` `go build ./...` exits 0
- `[x]` commit `7176412` (Task 1 — codecs + embed) found in git log
- `[x]` commit `4cdcccc` (Task 2 — jsonpointer + editor) found in git log
- `[x]` commit `0e652db` (Task 3 — seed sync) found in git log
- `[x]` `grep -rn "fmt.Sprintf.*INSERT\|fmt.Sprintf.*UPDATE\|fmt.Sprintf.*DELETE" internal/profile/` returns 0 lines (no string-concat SQL)
