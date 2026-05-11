---
phase: 04-realtime-dashboard
plan: 02
subsystem: events

tags: [postgres, listen-notify, sse-substrate, hub, fan-out, testcontainers, timescaledb]

# Dependency graph
requires:
  - phase: 04-realtime-dashboard
    plan: 01
    provides: 0021_measurement_inserted_trigger (AFTER INSERT NOTIFY on measurement hypertable), 7-field D-02 payload contract pinned by TestRunMigrations_MeasurementInsertedNotifyPropagatesToChunks

provides:
  - internal/events/doc.go — package doc: LISTEN channel, D-02 payload shape, topic conventions, drop-oldest backpressure
  - internal/events/hub.go — Hub struct: Subscribe/Unsubscribe/UpdateTopics/dispatch; in-process fan-out with non-blocking send (T-04-02-01); Measurement struct with 7 D-02 fields
  - internal/events/listener.go — Hub.Run/runOnce: LISTEN measurement_inserted reconnect loop (mirrors resolver/listener.go 1:1); 2s reconnectBackoff; dispatches to Hub
  - internal/events/hub_test.go — 6 unit tests: topic filtering, uplinks-topic routing, slow-subscriber isolation, unsubscribe, UpdateTopics, malformed-JSON drop
  - internal/events/listener_test.go — 4 unit/integration tests: fake-pool reconnect, pre-canceled ctx, ctx-cancel exit, end-to-end NOTIFY→Hub dispatch
  - internal/events/trigger_test.go — 2 load-bearing integration tests (SHIFTER_INTEGRATION_TESTS=1): TestMeasurementTrigger_PropagatesToChunks + TestMeasurementTrigger_AcrossChunks

affects:
  - 04-03-PLAN (SSE handler — consumes Hub.Subscribe/Unsubscribe/UpdateTopics + topic conventions)
  - 04-04-PLAN (online/offline KPI rule — depends on events substrate being live)
  - cmd/serve (must start hub.Run goroutine at boot alongside resolver.Run)

# Tech tracking
tech-stack:
  added: []  # zero new dependencies — pure stdlib + existing pgx/slog/uuid/testify
  patterns:
    - "Hub.dispatch does one JSON decode per broadcast (extract metering_point_id for routing), then sends raw []byte to subscriber channels — no re-encoding"
    - "Non-blocking send: select { case sub.Ch <- payload: default: sub.dropped++ } per PITFALL §9 — slow subscriber cannot block the listener goroutine"
    - "Topic fan-out: every NOTIFY publishes to dashboard:global + mp:<uuid> + mp:<uuid>:uplinks; SSE handler filters at Subscribe time"
    - "Listener pattern: same outer-loop shape as resolver/listener.go (Acquire → LISTEN → WaitForNotification loop → Release on error; 2s reconnectBackoff)"

key-files:
  created:
    - "internal/events/doc.go"
    - "internal/events/hub.go"
    - "internal/events/hub_test.go"
    - "internal/events/listener.go"
    - "internal/events/listener_test.go"
    - "internal/events/trigger_test.go"
  modified: []  # no existing files modified

key-decisions:
  - "Direct INSERT (no goroutine) in TestMeasurementTrigger_PropagatesToChunks: Postgres buffers NOTIFY for already-listening connections even before WaitForNotification is called, so goroutine concurrency is unnecessary and introduces race-with-goroutine-scheduling timing fragility."
  - "Hub.dispatch decodes into Measurement struct (not map[string]any) so json.Unmarshal validates full payload shape, not just MeteringPointID extraction — catches malformed payloads from future trigger edits earlier."
  - "reconnectBackoff lives at package scope (const) matching resolver's exact value (2s) so both listeners have identical reconnect behavior — prevents operational confusion."
  - "listener_test.go integration test TestListener_DispatchesNotifyToHub covers the full listener→hub→subscriber pipeline end-to-end, complementing the trigger_test.go which tests the DB→NOTIFY path."

patterns-established:
  - "Every NOTIFY-consuming package MUST have an integration test asserting WaitForNotification fires within 2s for a real INSERT — the listener_test.go and trigger_test.go tests both pin this contract."
  - "Backpressure drop policy: drop-oldest (non-blocking send to buffered ch of size 32), WARN every 10 drops. This is the Phase 4 standard — Plan 04-03 (SSE handler) MUST NOT change this without updating the hub."

requirements-completed: [DASH-02]

# Metrics
duration: 9min
completed: 2026-05-11
---

# Phase 4 Plan 02: LISTEN/NOTIFY Substrate Summary

**`internal/events` package: Hub with topic-filtered fan-out + LISTEN measurement_inserted reconnect loop + load-bearing chunk-propagation integration tests**

## Performance

- **Duration:** ~9 minutes
- **Started:** 2026-05-11T13:50:23Z
- **Completed:** 2026-05-11T13:59:35Z
- **Tasks:** 3
- **Files modified:** 6 (all new)

## Accomplishments

- **Hub API surface** (consumed by Plan 04-03 SSE handler): `NewHub(log)`, `Subscribe(connID, topics) <-chan []byte`, `Unsubscribe(connID)`, `UpdateTopics(connID, topics)`. Internally `dispatch(payload)` decodes JSON, computes `dashboard:global + mp:<uuid> + mp:<uuid>:uplinks` topics, non-blocking sends to matching subscribers (T-04-02-01: drop-oldest, WARN every 10 drops).
- **Listener** mirrors `internal/resolver/listener.go` 1:1: `Hub.Run(ctx, pool, log)` outer reconnect loop with 2s backoff, `runOnce` issues `LISTEN measurement_inserted`, loops on `WaitForNotification`, calls `h.dispatch(notif.Payload)`. Malformed JSON dropped at WARN (T-04-02-02).
- **Integration tests** satisfy VALIDATION.md's load-bearing assertion: `TestMeasurementTrigger_PropagatesToChunks` and `TestMeasurementTrigger_AcrossChunks` both pass against TimescaleDB 2.26.0-pg16 testcontainer. The chunk-propagation contract (AFTER INSERT on parent table triggers NOTIFY on chunk rows) is pinned by name with the required error message.

## Hub API Surface

```go
// Channel naming conventions (fixed for Phase 4):
//   dashboard:global       — every uplink
//   mp:<uuid>              — per metering-point
//   mp:<uuid>:uplinks      — same MP, Uplinks log tab

func NewHub(log *slog.Logger) *Hub
func (h *Hub) Subscribe(connID string, topics []string) <-chan []byte  // cap 32
func (h *Hub) Unsubscribe(connID string)                               // closes channel
func (h *Hub) UpdateTopics(connID string, topics []string)             // atomic replace
func (h *Hub) Run(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger)  // blocking
```

## 7-Field D-02 Payload Struct

```go
type Measurement struct {
    MeteringPointID uuid.UUID `json:"metering_point_id"`
    Time            time.Time `json:"time"`
    CumulativeValue *float64  `json:"cumulative_value"` // nullable
    InstantValue    *float64  `json:"instant_value"`    // nullable
    Quality         string    `json:"quality"`
    BatteryPct      *int16    `json:"battery_pct"`      // nullable
    RSSI            *int16    `json:"rssi"`              // nullable
}
```

## Backpressure Policy (PITFALL §9)

Non-blocking send with drop-oldest fallback, WARN every 10 drops:

```go
select {
case sub.Ch <- payload:
default:
    sub.dropped++
    if sub.dropped%10 == 0 {
        h.log.Warn("events: dropping payload to slow subscriber",
            "conn_id", sub.ID, "dropped", sub.dropped)
    }
}
```

## Task Commits

1. **Task 1: Hub with topic registry** — `d9871d4` (feat)
2. **Task 2: Listener reconnect loop** — `c7a44b8` (feat)
3. **Task 3: Trigger integration tests** — `3ee798c` (test)

**Plan metadata:** (final docs commit, see footer)

## Files Created/Modified

### Created

- `internal/events/doc.go` — package doc: LISTEN channel, D-02 payload shape, topic conventions, backpressure policy
- `internal/events/hub.go` — Hub struct, Measurement struct, Subscribe/Unsubscribe/UpdateTopics/dispatch, non-blocking send with drop-oldest
- `internal/events/hub_test.go` — 6 tests: topic filtering, uplinks-topic routing, slow-subscriber isolation, unsubscribe, UpdateTopics, malformed-JSON drop
- `internal/events/listener.go` — Hub.Run/runOnce: LISTEN measurement_inserted reconnect loop mirroring resolver/listener.go
- `internal/events/listener_test.go` — 4 tests: fake-pool reconnect return, pre-canceled ctx exit, ctx-cancel exit within 2s, end-to-end NOTIFY→Hub dispatch
- `internal/events/trigger_test.go` — 2 integration tests: PropagatesToChunks + AcrossChunks (both pass against TimescaleDB testcontainer)

## Decisions Made

- **Direct INSERT (no goroutine) in TestMeasurementTrigger_PropagatesToChunks.** Postgres buffers NOTIFY payloads for already-listening connections until the client polls via WaitForNotification. The goroutine pattern (used in roundtrip_test.go) introduced a race where the goroutine could complete before WaitForNotification started, causing timeout. Direct INSERT then WaitForNotification is simpler and correct.
- **Hub.dispatch decodes into Measurement struct, not map[string]any.** The full struct unmarshal validates the payload shape (7 required fields, correct types), not just the MeteringPointID needed for routing. This catches future trigger body regressions at decode time rather than silently routing garbled data.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Goroutine INSERT race in TestMeasurementTrigger_PropagatesToChunks**
- **Found during:** Task 3 (integration test run)
- **Issue:** The plan's test template uses a goroutine for the INSERT + WaitForNotification concurrently. In practice the INSERT goroutine completed before WaitForNotification was called (goroutine scheduled first), so the NOTIFY was already delivered before the client was polling — resulting in a 2s timeout. The passing `TestMeasurementTrigger_AcrossChunks` uses direct (non-goroutine) INSERTs and proved this pattern works correctly.
- **Fix:** Replaced goroutine INSERT with a direct synchronous INSERT followed by WaitForNotification. Removed unused `sync` import.
- **Files modified:** `internal/events/trigger_test.go`
- **Verification:** Both integration tests pass against TimescaleDB testcontainer.
- **Committed in:** `3ee798c`

**2. [Rule 1 - Bug] Invalid bytea literal `'\x0a\x1f'::bytea`**
- **Found during:** Task 3 (first integration test run after goroutine fix)
- **Issue:** PostgreSQL hex-escape syntax in bytea literals is `'\xHHHH'` (all hex digits as one sequence), not `'\xHH\xHH'` (multiple escape sequences). The second `\x` was interpreted as a literal backslash by Postgres → SQLSTATE 22023.
- **Fix:** Replaced `'\x0a\x1f'::bytea` with `'\x00'::bytea` (single-byte, same as the passing tests in roundtrip_test.go and AcrossChunks).
- **Files modified:** `internal/events/trigger_test.go`
- **Verification:** SQL error gone; INSERT succeeds; NOTIFY fires.
- **Committed in:** `3ee798c`

---

**Total deviations:** 2 auto-fixed (both Rule 1 — test correctness bugs in the plan's template code). Zero scope changes. All plan success criteria met.

## Issues Encountered

None beyond the two auto-fixed bugs above.

## User Setup Required

None — internal Go package only. No external service configuration required.

## Next Phase Readiness

- **Plan 04-03 (SSE handler)** is now unblocked: import `internal/events`, call `hub.Subscribe(connID, topics)` to get a `<-chan []byte`, write each received payload to the SSE response. Unsubscribe on client disconnect. Topic conventions are fixed and documented in `doc.go`.
- **cmd/serve** must start `hub.Run(ctx, pool, log)` as a goroutine at boot (same pattern as `resolver.Run`). Plan 04-03 or 04-05 will wire this.
- **354 short tests** pass project-wide (29 packages, 8 new from this plan); `go build ./...` exits 0.

## Threat Flags

None — this plan adds no new network surface, no new auth paths, no new trust boundaries. All T-04-02-* mitigations are implemented as planned: non-blocking dispatch (T-04-02-01), malformed-JSON drop+log (T-04-02-02), T-04-02-03 accepted (Postgres channel is internal, spoofing requires DB-level RCE).

## Self-Check: PASSED

- `internal/events/doc.go` — FOUND
- `internal/events/hub.go` — FOUND
- `internal/events/hub_test.go` — FOUND
- `internal/events/listener.go` — FOUND
- `internal/events/listener_test.go` — FOUND
- `internal/events/trigger_test.go` — FOUND
- Commit `d9871d4` (Task 1) — FOUND
- Commit `c7a44b8` (Task 2) — FOUND
- Commit `3ee798c` (Task 3) — FOUND
- `go build ./...` — exits 0
- `go test ./internal/events/ -count=1 -race` — 10 PASS
- `SHIFTER_INTEGRATION_TESTS=1 go test ./internal/events/ -run TestMeasurementTrigger -count=1 -timeout 180s` — 2 PASS
- Full short suite (354 tests, 29 packages) — PASS

---
*Phase: 04-realtime-dashboard*
*Plan: 02*
*Completed: 2026-05-11*
