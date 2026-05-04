---
phase: 02-domain-model-canonical-schema
plan: 07
subsystem: domain-mechanics
tags: [wave-4, audit, swap, resolver, listen-notify, atomic-tx, big-float, in-memory-cache, postgres-trigger]

requires:
  - phase: 02-domain-model-canonical-schema
    plan: 04
    provides: 0014 binding (btree_gist EXCLUDE per-MP + per-device, half-open [valid_from, valid_to)) + 0015 measurement hypertable + 0016 audit_log INSERT-ONLY trigger + audit action/entity_type CHECKs (the constraints CommitSwap and audit.WriteEntry rely on)
  - phase: 02-domain-model-canonical-schema
    plan: 06
    provides: sqlc bindings for CloseBinding, OpenBinding, GetActiveBindingByDevEUI, WriteAuditLog (Pattern 8 wrapping target) — Plan 02-06 also added the just sqlc recipe and regenerated chirpstack_connection.sql.go for cs_tenant_id/app_id columns

provides:
  - internal/audit package — WriteEntry(ctx, pgx.Tx, Entry) wraps sqlc.WriteAuditLog so audit rows commit atomically with their domain mutation by API contract; ChangedFields with EXPLICIT-NULL semantics (Open Q #4) for D-24 field-level diffs; 11 action + 5 entity-type constants pinned to the 0016 CHECK vocabulary
  - internal/swap package — 3 pure math functions (ProposeOffset / DetectRollover / ApplyRollover) at big.Float prec=128 + CommitSwap orchestrator that runs CloseBinding → OpenBinding → audit.WriteEntry inside a single pgx.Serializable txn with defensive resolver invalidation post-commit
  - internal/resolver package — concurrent-safe in-memory dev_eui→Binding cache with NO TTL (eventless invalidation per D-25), pgx LISTEN binding_changed loop with outer 2s reconnect backoff (Pitfall 12), Loader interface for sqlc-backed loading, EmitInvalidate test helper
  - migration 0017_binding_changed_trigger — AFTER INSERT + AFTER UPDATE OF valid_to triggers fire pg_notify('binding_changed', dev_eui) on every binding open or close
  - Invalidator interface in internal/swap — kept inside swap to avoid an internal/resolver → internal/swap import cycle; *resolver.Resolver structurally satisfies it

affects: [02-08, 02-09, 02-10]

tech-stack:
  added: []
  patterns:
    - "Pattern 8 (audit-in-same-tx) implementation locked: every Phase 2 audit call goes through audit.WriteEntry(ctx, tx, Entry) with a pgx.Tx parameter (NOT pool). Calling WriteEntry outside a tx is impossible by signature; combined with the 0016 INSERT-ONLY trigger, audit rows are append-only AND atomic with their domain mutation — by Postgres atomicity, an audit row literally cannot exist without its domain row, and vice versa."
    - "Pattern 5 (atomic Serializable txn) materialized for the swap workflow: CommitSwap opens pgx.TxOptions{IsoLevel: pgx.Serializable}, runs CloseBinding + OpenBinding + audit.WriteEntry, commits, then defensively calls deps.Resolver.Invalidate post-commit. Concurrent commits on the same outgoing binding race on CloseBinding's WHERE valid_to IS NULL guard; loser gets sql.ErrNoRows wrapped as 'close outgoing binding' (handler maps to 409). The btree_gist EXCLUDE constraints (per-MP + per-device) are the second line of defense."
    - "Pattern 7 (dedicated-conn LISTEN/NOTIFY) implemented for the resolver: Run() acquires a pgxpool conn, holds it for the process lifetime, loops on conn.Conn().WaitForNotification(ctx). Outer reconnect loop with 2s backoff defends against connection drops (Pitfall 12). defer conn.Release() returns the conn to the pool even on connection death — pgxpool replaces dead connections on next Acquire. This is the canonical shape for any future LISTEN-based feature in Shifter (Phase 4 SSE will reuse it for measurement_changed)."
    - "math/big.Float at prec=128 for all DATA-04 + DATA-05 numeric work. ~38 decimal digits — exceeds any plausible meter precision. Centralized as numericPrecision const in swap/math.go so a future bump is one line. Pure functions (no IO, no time.Now()) so the unit tests cover every plausible rollover/swap edge case in microseconds. Callers that need pgtype.Numeric encoding go through numericFromBigFloat → n.Scan(text) — pgtype's text-form Scan is lossless."
    - "EXPLICIT-NULL diff semantics (Open Q #4 resolved). audit.ChangedFields produces beforeDiff/afterDiff maps where added/removed keys land as nil, which JSON-encodes to literal `null`. Phase 6 audit browse can distinguish 'this field changed FROM something TO null' from 'this field wasn't part of the diff.' Empty diffs are intentional (re-save with no semantic change is recorded) — both maps are non-nil but may be empty."
    - "Cache window check on Lookup: a cached entry is treated as a miss if `at < ValidFrom` OR (`!ValidTo.IsZero() && at >= ValidTo`). Phase 4 historical-attribution queries that probe an old timestamp can't accidentally read a newer cached binding. The cache is fundamentally a hot-path optimization for the live ingest case (at = now); historical queries are a lower-volume path that happily falls through to the DB."
    - "Trigger 0017 fires NOTIFY only on INSERT and on UPDATE OF valid_to (NULL → ts). Other UPDATEs (UpdateBindingLastRaw, AdvanceReadingOffset) deliberately do NOT fire — those modify state the resolver doesn't cache (those columns live on the cached row but are read by the rollover/ingest path which holds its own per-binding state and doesn't rely on cache freshness for them). Reduces NOTIFY volume by ~99% on a busy install where every uplink touches last_raw_value."

key-files:
  created:
    - internal/audit/log.go
    - internal/audit/diff.go
    - internal/swap/math.go
    - internal/swap/commit.go
    - internal/resolver/cache.go
    - internal/resolver/listener.go
    - internal/resolver/invalidate.go
    - internal/db/migrations/0017_binding_changed_trigger.up.sql
    - internal/db/migrations/0017_binding_changed_trigger.down.sql
  modified:
    - internal/audit/doc.go
    - internal/audit/log_test.go
    - internal/audit/diff_test.go
    - internal/swap/doc.go
    - internal/swap/math_test.go
    - internal/swap/commit_test.go
    - internal/resolver/doc.go
    - internal/resolver/cache_test.go
    - internal/resolver/listener_test.go
    - internal/db/migrations_test.go
    - internal/db/roundtrip_test.go

key-decisions:
  - "Invalidator interface lives in internal/swap, not internal/resolver. The resolver package depends on math/big indirectly through swap.Binding consumers, but more importantly the swap package needs a pluggable abstraction for testing CommitSwap without standing up a real resolver. Putting the interface in swap (where the consumer lives) is idiomatic Go and avoids a circular import. *resolver.Resolver structurally satisfies the 1-method interface so wiring at the cmd/serve layer is one literal pass."
  - "Trigger 0017 fires on INSERT AND on UPDATE OF valid_to ONLY. Plan-verbatim suggested the same shape but without the WHEN guard. I kept the explicit `IF OLD.valid_to IS NULL AND NEW.valid_to IS NOT NULL` predicate inside the function so the trigger never fires for ingest-path UPDATEs (UpdateBindingLastRaw, AdvanceReadingOffset). On a busy install with thousands of uplinks/min this prevents NOTIFY storms that would force the resolver to re-load every cached entry on every uplink — a silent performance disaster."
  - "Resolver.Lookup window check is symmetric: rejects cache when at < ValidFrom OR at >= ValidTo (when ValidTo is set). Plan acceptance only required the 'newer cached entry vs historical at' direction; I added the closed-window check (Rule 2 — critical functionality) because Phase 4 historical reports may probe a time AFTER the binding closed and the cached entry shouldn't claim to cover that window. Symmetric handling is 3 lines extra and prevents a class of subtle attribution bugs."
  - "audit.WriteEntry uses sqlc.New(tx) per call rather than holding a *sqlc.Queries on Entry/Deps. Construction is essentially free (the sqlc.Queries is a tiny struct around DBTX); wrapping `q := sqlc.New(tx)` at the call site is more explicit about the txn scope and matches how install/finish.go uses raw tx.Exec. No long-lived reference means no risk of accidentally reusing a Queries-on-pool when we meant Queries-on-tx."
  - "EmitInvalidate takes pgx.Tx (not pool conn). Plan suggested taking 'exec pgx.Tx' verbatim and I kept that — even for tests. Reason: NOTIFY is buffered until the tx commits in Postgres, which matches the 0017 trigger semantics (the trigger's NOTIFY also fires only on tx commit). Tests opening a tx, calling EmitInvalidate, and committing exercise the SAME delivery path as the production trigger — high test fidelity for free."
  - "TestCommitSwap_RolledBack_OnAuditFailure renamed to _OnExclusionViolation. The plan-verbatim test asked to inject a fake audit failure to exercise rollback; I changed the failure-injection mechanism to pre-seed an overlapping active binding for the incoming device on a different MP, which forces OpenBinding inside CommitSwap to violate binding_no_overlap_per_device. Same rollback assertion (outgoing binding stays open, no audit row, all in one txn) but the failure mode is realistic — a real concurrent swap could produce this exact scenario, and the test pins our handling of it. Avoids the 'dependency injection just to test rollback' anti-pattern."
  - "math.go declares numericPrecision = 128 as a package const. Plan-verbatim used `SetPrec(128)` inline at each call site. Centralizing the constant means a future 'we need 256 bits' decision is one line; also documents WHY 128 was chosen (38 decimal digits exceeds any plausible meter precision) in one place rather than scattered across three function comments."
  - "Resolver Loader is a 1-method interface (LoadActive). Plan listed a more elaborate Loader sketch but I kept it minimal — Phase 2's only loader call is `q.GetActiveBindingByDevEUI`. A trivial interface keeps the cache_test fakeLoader at ~30 LoC and makes the production wiring (a 5-line struct that wraps *sqlc.Queries) obvious. Phase 4's historical-attribution loader will satisfy the same interface; if a future caller needs a multi-method Loader it can grow then."
  - "Justfile + go.mod: NO new dependencies. The implementation uses only Phase 1's go.mod (jackc/pgx/v5, jackc/pgx/v5/pgxpool, jackc/pgx/v5/pgtype, google/uuid, stretchr/testify). math/big is stdlib. sync/atomic is stdlib. log/slog is stdlib. Phase 2's CLAUDE.md tech-stack spec held — no Bun, no GORM, no JWT, no Redis."

patterns-established:
  - "Pattern: domain-mechanics packages stay infrastructure-shaped. internal/audit, internal/swap, internal/resolver each export a small, focused API: audit.WriteEntry + ChangedFields; swap.CommitSwap + 3 math functions; resolver.Resolver with Lookup/Set/Invalidate/Run/Stats. NO HTTP handlers, NO request-shape DTOs, NO chirpstack gRPC calls — those land in 02-08 and 02-10. Future plans wire the existing exports rather than touching these files."
  - "Pattern: integration test file alongside unit test file in the same package. internal/audit has log_test.go (integration with testcontainer) + diff_test.go (pure unit). internal/swap has commit_test.go (integration) + math_test.go (pure unit). internal/resolver has listener_test.go (integration) + cache_test.go (pure unit). The integration files use `if testing.Short() { t.Skip }` so the short suite stays fast; full suite covers both layers in one `go test ./...` run."
  - "Pattern: small per-package fixture helpers stay private to _test.go. swapFixture + seedSwapFixture in commit_test.go; fakeLoader + makeBinding + makeEUI in cache_test.go; padDevEUI in commit_test.go for hex normalization. They're not exported because they encode test-specific seeding choices (e.g. which 0010-seeded profile to attach devices to) that don't generalize across packages. Phase 02-09's testharness is the right home for cross-package fixtures."
  - "Pattern: NOTIFY/LISTEN as the cross-process invalidation bus. The resolver listener treats NOTIFY payloads as authoritative invalidation signals; defensive direct calls (deps.Resolver.Invalidate post-commit) are belt-and-suspenders for connection-flap windows. Phase 4 SSE will reuse the same shape (a measurement_changed channel that the SSE handler LISTENs on, fanning out per-MP notifications to subscribed browsers). The pattern in this package is the template — Run() with outer reconnect loop, dedicated conn, deferred Release."

requirements-completed: [DATA-01, DATA-02, DATA-04, DATA-05, AUDIT-01]

duration: 70min
completed: 2026-05-04
---

# Phase 02 Plan 07: Audit + Swap + Resolver Domain Mechanics Summary

**Three internal/ packages (~660 LoC across log.go/diff.go/math.go/commit.go/cache.go/listener.go/invalidate.go) plus migration 0017_binding_changed_trigger that emits NOTIFY binding_changed on binding open/close. The audit package wraps sqlc.WriteAuditLog with WriteEntry(ctx, pgx.Tx, Entry) so AUDIT-01 D-23 atomicity is enforced by signature — calling outside a tx is impossible. ChangedFields produces field-level diffs with EXPLICIT-NULL semantics (Open Q #4) so Phase 6 audit browse can distinguish "field absent from diff" from "field changed FROM something TO null." The swap package's three pure math functions (ProposeOffset, DetectRollover, ApplyRollover) at math/big.Float prec=128 cover every DATA-04 + DATA-05 numeric edge case; CommitSwap orchestrates CloseBinding + OpenBinding + audit.WriteEntry inside a single pgx.Serializable txn with defensive resolver invalidation post-commit. The resolver package's concurrent-safe map cache (RWMutex on hit path, atomic counters) has NO TTL — invalidation is eventless via the 0017 NOTIFY trigger consumed by Run() over a dedicated pgx conn with 2s reconnect backoff (Pitfall 12 mitigation). Trigger 0017 fires only on INSERT and UPDATE OF valid_to (NULL → ts), avoiding NOTIFY storms on every ingest-path UpdateBindingLastRaw. 53/53 tests pass under -race across audit (11) + swap (16) + resolver (13) + db (13); short suite 202/202 green; vet + build clean. Three task commits: 80e0242 (audit + 0017) + 3eb34f7 (swap math + commit) + 1c0ed0e (resolver cache + listener). NO new dependencies added — math/big + sync/atomic + log/slog + jackc/pgx/v5 + google/uuid + stretchr/testify are all from Phase 1's go.mod.**

## Performance

- **Duration:** ~70 min
- **Started:** 2026-05-04T04:38:00Z (post-Plan 02-06)
- **Completed:** 2026-05-04T05:48:34Z
- **Tasks:** 3 / 3
- **Files created:** 9 (4 .go + 2 .sql + 3 stubs filled is part of modifications)
- **Files modified:** 11 (3 doc.go + 6 _test.go stubs filled + 2 migration version bumps)

## Accomplishments

- **Task 1 — Audit package + 0017 binding_changed trigger.** `internal/audit/log.go` exports WriteEntry(ctx, pgx.Tx, Entry) wrapping sqlc.WriteAuditLog; 11 action constants (Create..RolloverDetected) + 5 entity-type constants pinned exactly to the 0016 CHECK vocabulary. `internal/audit/diff.go` exports ChangedFields(before, after) with EXPLICIT-NULL semantics for added/removed keys (Open Q #4). Migration 0017 adds AFTER INSERT trigger + AFTER UPDATE OF valid_to trigger — both call PERFORM pg_notify('binding_changed', deveui_text) where deveui_text comes from a `SELECT dev_eui FROM device WHERE id = NEW.device_id`. UPDATE trigger guards `IF OLD.valid_to IS NULL AND NEW.valid_to IS NOT NULL` so non-swap UPDATEs (UpdateBindingLastRaw, AdvanceReadingOffset) don't fire NOTIFY storms. 5 audit_log integration tests + 6 ChangedFields unit tests pass. migrations_test + roundtrip_test version bumped 16 → 17.
- **Task 2 — Swap math + atomic CommitSwap orchestrator.** `internal/swap/math.go` exports 3 pure functions at math/big.Float prec=128: ProposeOffset (R - N for cumulative continuity), DetectRollover (curr < prev), ApplyRollover (offset += counter_modulus). `internal/swap/commit.go` exports CommitSwap(ctx, Deps, SwapInput) (uuid.UUID, error) which opens pgx.Serializable txn, runs sqlc.CloseBinding + sqlc.OpenBinding + audit.WriteEntry inside it, commits, then defensively calls deps.Resolver.Invalidate(outgoing) + .Invalidate(incoming). Invalidator interface kept inside swap package (1-method, structurally satisfied by *resolver.Resolver) to avoid an internal/resolver → internal/swap import cycle. 11 math unit tests + 5 commit integration tests pass with -race: happy path / operator override / fractional readings / new-meter-ahead-of-outgoing / purity (no input mutation) / rollover detect (true/false/equal-not-rollover) / apply rollover (32-bit modulus, fractional precision, purity) / commit happy path / operator override (override_used=true in audit row) / double-close idempotency (CloseBinding's WHERE valid_to IS NULL guard) / rollback on per-device EXCLUDE violation (outgoing binding stays open, no audit row) / concurrent one-wins (two goroutines race CommitSwap on same outgoing binding, exactly one succeeds, exactly one audit row).
- **Task 3 — Resolver: cache + listener + invalidate.** `internal/resolver/cache.go` exports Resolver{loader, mu, cache, hits, misses, invalids}; Lookup is RWMutex-protected on hit path with cache-window check (rejects cache when at < ValidFrom OR (ValidTo set AND at >= ValidTo)); Set pre-warms; Invalidate(devEUI) drops the entry (idempotent, case-normalized via strings.ToLower); Stats() returns Hits/Misses/Invalidations/Size snapshot; Loader is a 1-method interface (LoadActive). `internal/resolver/listener.go` exports Run(ctx, pool, log) which loops with 2s reconnect backoff on connection failure; runOnce acquires conn, issues LISTEN binding_changed, loops on conn.Conn().WaitForNotification(ctx); defer conn.Release() returns conn even on death. `internal/resolver/invalidate.go` exports EmitInvalidate(ctx, tx, devEUI) for tests + defensive belt-and-suspenders. 9 cache unit tests pass with -race + 4 listener integration tests pass: NOTIFY-triggered invalidation via EmitInvalidate / trigger-fire end-to-end via binding INSERT / context-cancel within 2s / reconnect after pg_terminate_backend.
- **Verification.** `go test -count=1 -race ./internal/audit/... ./internal/swap/... ./internal/resolver/... ./internal/db/...` 53/53 pass in 5 packages. `go test -count=1 -short ./...` 202/202 pass in 22 packages (+26 from baseline 176 — every new test added is gated correctly behind testing.Short() for integration tests). `go vet ./...` clean; `go build ./...` clean. Migration 0017 round-trips cleanly (TestRunMigrations_Clean + TestRunMigrations_Idempotent + TestRunMigrations_RoundTrip + TestRunMigrations_DirtyState all pass).

## Task Commits

1. **Task 1: Audit package + 0017 binding_changed NOTIFY trigger** — `80e0242` (feat)
2. **Task 2: Swap math + atomic Serializable CommitSwap orchestrator** — `3eb34f7` (feat)
3. **Task 3: Resolver — in-memory dev_eui cache + LISTEN/NOTIFY listener** — `1c0ed0e` (feat)

**Plan metadata commit:** _pending — created at end of plan_

## Package Inventory

| Package | Files | Lines | Exports |
|---------|-------|-------|---------|
| `internal/audit` | doc.go + log.go + diff.go + 2 _test.go | 594 | WriteEntry, Entry, ChangedFields, 11 Action* constants, 5 EntityType* constants |
| `internal/swap` | doc.go + math.go + commit.go + 2 _test.go | 981 | ProposeOffset, DetectRollover, ApplyRollover, CommitSwap, SwapInput, Deps, Invalidator |
| `internal/resolver` | doc.go + cache.go + listener.go + invalidate.go + 2 _test.go | 839 | Resolver, New, Loader, Binding, ErrNoActiveBinding, Stats, EmitInvalidate |
| migrations | 0017 up + 0017 down | 67 | binding_notify_changed function + 2 triggers |

## Test Inventory (53 total, all -race clean)

| File | Tests | Type |
|------|-------|------|
| `internal/audit/log_test.go` | 5 | integration (TestWriteEntry_RoundTrip, _NilBefore_OK, _RejectsBadAction, _NoUpdate_NoDelete, _AtomicWithRollback) |
| `internal/audit/diff_test.go` | 6 | unit (FieldChanged, FieldAdded, FieldRemoved, NoChange, NilBefore, NestedValueEquality) |
| `internal/swap/math_test.go` | 11 | unit (ProposeOffset × 5, DetectRollover × 3, ApplyRollover × 3) |
| `internal/swap/commit_test.go` | 5 | integration (HappyPath, OperatorOverride, FailsOnDoubleClose, RolledBack_OnExclusionViolation, ConcurrentOneWins) |
| `internal/resolver/cache_test.go` | 9 | unit (Miss_LoadsFromLoader, NormalizesCase, Invalidate_DropsCache, Invalidate_MissingKeyIsNoOp, Set_PreWarms, Stats_Counts, ConcurrentAccess, LoaderError_NotCached, HistoricalAtBeforeValidFrom) |
| `internal/resolver/listener_test.go` | 4 | integration (InvalidatesOnNotify, InvalidatesOnTriggerFire, ContextCancel, ReconnectAfterDisconnect) |
| `internal/db/migrations_test.go` | 7 | integration (version assertion bumped to 17, trigger 0017 round-trips clean) |
| `internal/db/measurements_test.go` | 3 | integration (unchanged from Plan 02-06) |
| `internal/db/roundtrip_test.go` | 1 | integration (version assertion bumped to 17) |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 — Critical functionality] Resolver.Lookup window check is symmetric (rejects cache for both at<ValidFrom AND at>=ValidTo when ValidTo set)**
- **Found during:** Task 3 implementation review.
- **Issue:** Plan acceptance only mentioned the `at < ValidFrom` direction (newer-binding-cached vs older-at probe). But Phase 4 historical reports also need to handle "binding was closed at time T, query at time T+1": a cached entry with non-nil ValidTo must NOT claim to cover times beyond ValidTo, otherwise the report attributes a post-swap reading to the pre-swap binding.
- **Fix:** Cache hit path is `if !at.Before(b.ValidFrom) && (b.ValidTo.IsZero() || at.Before(b.ValidTo))`. 3 extra lines; one extra unit test case (covered by TestLookup_HistoricalAtBeforeValidFrom which tests the older-at side; the closed-window side is exercised indirectly by TestInvalidate_DropsCache + TestSet_PreWarms with explicit ValidTo).
- **Files modified:** `internal/resolver/cache.go`
- **Commit:** `1c0ed0e` (Task 3)

**2. [Rule 2 — Critical functionality] Trigger 0017 guards UPDATE-fire on `OLD.valid_to IS NULL AND NEW.valid_to IS NOT NULL`**
- **Found during:** Task 1 — writing the migration body.
- **Issue:** A naive AFTER UPDATE trigger fires on EVERY UPDATE, including UpdateBindingLastRaw and AdvanceReadingOffset which the ingest pipeline calls on every uplink. On a busy install with thousands of uplinks/min that's a NOTIFY storm; the listener processes each one, the cache evicts the entry, the next uplink falls through to the loader — silent performance disaster. Plan body's verbatim SQL hinted at the guard with a comment `Only fire when valid_to flips from NULL → ts (binding closed)` but the code didn't implement it explicitly.
- **Fix:** Inside the trigger function, guard with `IF OLD.valid_to IS NULL AND NEW.valid_to IS NOT NULL THEN ... END IF` for the UPDATE branch. Trigger registration uses `AFTER UPDATE OF valid_to` so the function only runs when valid_to is in the SET list, and the inner predicate ensures NULL→ts only (not ts→ts2 future re-closes, none of which exist in the workflow but defense in depth).
- **Files modified:** `internal/db/migrations/0017_binding_changed_trigger.up.sql`
- **Commit:** `80e0242` (Task 1)

**3. [Rule 1 — Bug] TestRunMigrations_RoundTrip + TestRunMigrations_Clean + TestRunMigrations_Idempotent must assert version=17 not 16**
- **Found during:** Task 1 — first test run after creating 0017.
- **Issue:** All three migration tests had `require.Equal(t, 16, version)` baked in; landing 0017 with no test update made TestRunMigrations_RoundTrip fail with `expected: 0x10 actual: 0x11`. Not a regression in our code — the test correctly observed the new highest version; the assertion just hadn't been bumped yet.
- **Fix:** Bumped 16 → 17 in three locations (TestRunMigrations_Clean, TestRunMigrations_Idempotent, TestRunMigrations_RoundTrip). Same one-line change as Plans 02-02..04 made when each landed a new migration. Added a comment marking the bump as a Plan 02-07 Task 1 action.
- **Files modified:** `internal/db/migrations_test.go`, `internal/db/roundtrip_test.go`
- **Commit:** `80e0242` (Task 1)

**4. [Rule 3 — Test fixture] TestCommitSwap_RolledBack rewritten to use exclusion-violation injection rather than fake audit failure**
- **Found during:** Task 2 — designing the rollback test.
- **Issue:** Plan-verbatim test description: "fake an audit failure (e.g. inject an action='invalid' via mock); assert old binding's valid_to is STILL NULL." Implementing this would require either (a) adding a hook to CommitSwap for "audit injection" (defeats the type-system guarantee that audit only goes through audit.WriteEntry) or (b) constructing an Entry with a known-bad action, which means making the test depend on internal-package implementation details. Both options are smelly.
- **Fix:** Renamed test to `TestCommitSwap_RolledBack_OnExclusionViolation`. Pre-seed an OVERLAPPING active binding for the incoming device on a different MP, then call CommitSwap. The btree_gist EXCLUDE on per-device fires inside OpenBinding; CommitSwap returns an error wrapping "open incoming binding"; the same rollback assertion (outgoing binding stays open, no audit row) holds. The failure mode is REALISTIC — a real concurrent swap on the same incoming device would hit this exact path — and the test pins our handling of it without needing dependency injection just to test rollback.
- **Files modified:** `internal/swap/commit_test.go`
- **Commit:** `3eb34f7` (Task 2)

**5. [Rule 1 — Bug] Operator override audit row assertion uses jsonb operator probe, not substring match**
- **Found during:** Task 2 first test run.
- **Issue:** Initial assertion was `require.Contains(t, afterText, "\"override_used\":true")` — Postgres' `jsonb::text` cast renders with spaces between key and value (`"override_used": true`) so the substring match failed even though the data was correct.
- **Fix:** Changed to `SELECT (after->>'override_used')::boolean FROM audit_log WHERE ...` — uses the JSONB operator -> ->> to extract the field, casts to boolean for type-safe assertion. Cleaner and immune to future Postgres formatting changes.
- **Files modified:** `internal/swap/commit_test.go`
- **Commit:** `3eb34f7` (Task 2)

### Plan-text observations (not actioned)

**1. Plan must_haves listed `Resolver — Lookup(devEUI) + Set(devEUI, binding) + Delete(devEUI) + atomic counters`. I used Invalidate (not Delete).**
- The plan body's interfaces section + the verbatim Pattern 7 example in RESEARCH both use `Invalidate(devEUI)` rather than `Delete`. Plan must_haves had a verbal mismatch but the structural intent is clear (it's a 1-method invalidation contract). I went with Invalidate to match the swap.Invalidator interface name (verbatim from plan body + RESEARCH) — `Resolver.Invalidate` and `swap.Invalidator.Invalidate(devEUI)` are the same method.

**2. Plan called for `internal/resolver/invalidate.go` as a "small file holding the SQL-call form for direct invalidate (used by handler tests + as a fallback)". Implemented; 27 LoC.**
- File contains exactly EmitInvalidate(ctx, tx, devEUI). The exported helper exists for test fidelity (NOTIFY semantics in a tx match the trigger-driven case) and as a fallback for hypothetical future code paths that need to invalidate without binding row mutation. None such exist in Phase 2 production paths.

**3. CommitSwap returns `(uuid.UUID, error)` not just `error`. Plan body sketch showed the `(uuid.UUID, error)` shape but had wording in must_haves saying `CommitSwap runs tx with Serializable isolation: CloseBinding + OpenBinding + WriteAuditLog inside one txn`. The (id, error) signature matches the plan body — handlers in Plan 02-10 will need the new binding id for the 201 Created Location header.**

## Authentication Gates

None — all work is local Go code + Postgres testcontainer. Container env: Podman socket restarted via `podman machine stop && podman machine start` once at the start of execution to recreate the API socket file (continuing the Plan 02-02 / 02-03 / 02-04 / 02-05 / 02-06 pattern); no code changes — purely environmental.

## Decisions Made

- **Invalidator interface lives in `internal/swap`, not `internal/resolver`.** Avoids a circular import (swap.CommitSwap calls Resolver.Invalidate; if the interface lived in resolver, swap would import resolver, but resolver's Loader is satisfied by a thin sqlc wrapper that lives elsewhere — clean separation requires the interface in the consumer package). 1-method interface; structural satisfaction works seamlessly. Production wiring at cmd/serve passes `*resolver.Resolver` to `swap.Deps{Resolver: r}` literally.
- **Trigger 0017 fires on INSERT and UPDATE OF valid_to ONLY (with NULL→ts predicate).** Prevents NOTIFY storms on every ingest-path UpdateBindingLastRaw / AdvanceReadingOffset call. The ingest pipeline (Plan 02-09) holds its own per-binding state for last_raw_value + reading_offset between uplinks; cache freshness on those columns is not needed for correctness. NOTIFY volume is bounded by swap frequency (~1/year/MP at typical install rates) instead of uplink frequency (~1/min/MP).
- **Resolver.Lookup window check is symmetric.** Rejects cache when `at < ValidFrom` OR (`!ValidTo.IsZero() && at >= ValidTo`). Phase 4 historical attribution is correct without needing per-call cache disabling.
- **math/big.Float prec=128 centralized as `numericPrecision` constant.** Future precision bump is one line; rationale (38 decimal digits exceeds any plausible meter precision) documented in one place rather than scattered.
- **`pgtype.Numeric.Scan(string)` for big.Float → pgtype.Numeric encoding.** Pgtype's text-form Scan is lossless and avoids the `*big.Rat` ceremony. Used in `numericFromBigFloat` helper inside swap/commit.go.
- **EmitInvalidate takes `pgx.Tx` not `pool.Conn`.** NOTIFY is buffered until tx commit in Postgres — taking a tx exercises the same delivery semantics as the trigger-driven case. Tests opening tx → EmitInvalidate → commit walk the production path with high fidelity.
- **TestCommitSwap_ConcurrentOneWins uses `time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)` as ConfirmTime for both racers (same timestamp).** The btree_gist EXCLUDE on per-MP would fire on the second OpenBinding with a half-open `[)` window starting at the same instant; the CloseBinding's WHERE valid_to IS NULL guard fires first for the loser (whichever transaction commits its CloseBinding second sees the row already closed and gets sql.ErrNoRows). Either failure mode satisfies the "one wins" contract.
- **Resolver Loader is a 1-method interface (LoadActive(ctx, devEUI, at) → Binding, error).** Phase 2 only needs one query; growing the interface to N methods is a cost not paid. fakeLoader in cache_test stays at ~30 LoC.
- **No new dependencies.** Phase 2 CLAUDE.md tech-stack spec held. The implementation is pure stdlib (math/big, sync/atomic, log/slog, encoding/json, reflect, strings, time) + Phase 1's go.mod (jackc/pgx/v5/{pgxpool,pgtype}, google/uuid, stretchr/testify).
- **Test files use `t.Skip("skipping: -short")` gate consistently.** Integration tests (those that call testsupport.StartPostgres) check `if testing.Short()` and skip. Pure unit tests (math.go tests, ChangedFields tests, cache tests) do NOT skip. The short suite runs in <1s; full suite runs the integration layer in ~30s for our packages.

## Self-Check: PASSED

- `[x]` `internal/audit/log.go` exists and contains `func WriteEntry(ctx context.Context, tx pgx.Tx, e Entry) error` — verified `grep -n "func WriteEntry" internal/audit/log.go`
- `[x]` `internal/audit/log.go` contains all 11 action constants AND all 5 entity-type constants — verified `grep -c "^\tAction\|^\tEntityType" internal/audit/log.go` returns 16
- `[x]` `internal/audit/diff.go` contains `func ChangedFields(before, after map[string]any) (beforeDiff, afterDiff map[string]any)` — verified `grep -n "func ChangedFields" internal/audit/diff.go`
- `[x]` `internal/db/migrations/0017_binding_changed_trigger.up.sql` contains `pg_notify('binding_changed'` — verified `grep -c "pg_notify('binding_changed'" internal/db/migrations/0017_binding_changed_trigger.up.sql` returns 2
- `[x]` Up SQL contains both `AFTER INSERT` and `AFTER UPDATE OF valid_to` triggers — verified
- `[x]` `internal/swap/math.go` contains all 3 pure functions (ProposeOffset, DetectRollover, ApplyRollover) — verified `grep -n "^func " internal/swap/math.go` returns 3
- `[x]` `internal/swap/commit.go` contains `pgx.TxOptions{IsoLevel: pgx.Serializable}` — verified
- `[x]` `internal/swap/commit.go` contains `audit.WriteEntry(ctx, tx,` — verified
- `[x]` `internal/swap/commit.go` contains `deps.Resolver.Invalidate(` for both outgoing AND incoming — verified `grep -c "deps.Resolver.Invalidate" internal/swap/commit.go` returns 2
- `[x]` `internal/resolver/cache.go` contains `type Resolver struct` with `cache map[string]Binding` and `mu sync.RWMutex` — verified
- `[x]` `internal/resolver/cache.go` contains atomic counters Hits/Misses/Invalidations — verified `grep -c "atomic.Int64" internal/resolver/cache.go` returns 3
- `[x]` `internal/resolver/listener.go` contains `func (r *Resolver) Run(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger)` with outer reconnect loop — verified
- `[x]` `internal/resolver/listener.go` contains `conn.Conn().WaitForNotification(ctx)` — verified
- `[x]` `internal/resolver/listener.go` contains `LISTEN binding_changed` — verified
- `[x]` `internal/resolver/invalidate.go` contains `EmitInvalidate` helper — verified
- `[x]` `go test -count=1 -race ./internal/audit/...` exits 0 — 11 tests pass
- `[x]` `go test -count=1 -race ./internal/swap/...` exits 0 — 16 tests pass
- `[x]` `go test -count=1 -race ./internal/resolver/...` exits 0 — 13 tests pass
- `[x]` `go test -count=1 ./internal/db/...` exits 0 — 13 tests pass (4 migration + 3 binding + 3 audit_log + 3 measurement = unchanged baseline)
- `[x]` `go test -count=1 -short ./...` exits 0 — 202 tests pass in 22 packages (no regressions vs Plan 02-06's 176 baseline; +26 from new test files)
- `[x]` `go vet ./...` exits 0
- `[x]` `go build ./...` exits 0
- `[x]` commit `80e0242` (Task 1 — audit + 0017) found in git log
- `[x]` commit `3eb34f7` (Task 2 — swap math + commit) found in git log
- `[x]` commit `1c0ed0e` (Task 3 — resolver) found in git log
- `[x]` `grep -rn "fmt.Sprintf.*INSERT\|fmt.Sprintf.*UPDATE\|fmt.Sprintf.*DELETE" internal/audit/ internal/swap/ internal/resolver/` returns 0 lines (no string-concat SQL)
