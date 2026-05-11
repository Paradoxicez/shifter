---
phase: 04-realtime-dashboard
plan: 03
subsystem: events

tags: [sse, hub, events, caddy, chi-router, realtime, authentication]

# Dependency graph
requires:
  - phase: 04-realtime-dashboard
    plan: 02
    provides: Hub.Subscribe/Unsubscribe/UpdateTopics, topic conventions (dashboard:global / mp:<uuid> / mp:<uuid>:uplinks), Measurement struct, Hub.Run reconnect loop

provides:
  - internal/events/handler.go — SSE handler (snapshot-on-connect, heartbeat, write loop, topic validation)
  - internal/events/routes.go — RegisterRoutes(r chi.Router, deps Deps) mounting GET /api/events only
  - internal/http/router.go — EventsDeps *events.Deps field + nil-guard RegisterRoutes call under authenticated group
  - internal/cli/serve.go — events.NewHub construction + go eventsHub.Run(ctx, pool, log) at boot + EventsDeps wiring
  - Caddyfile — @sse matcher extended to /api/events and /api/events/* with flush_interval -1

affects:
  - 04-06-PLAN (useSSE hook — GET /api/events is now live; hook can connect)
  - 04-07-PLAN (dashboard — subscribes to dashboard:global via useSSE)
  - 04-09-PLAN (MP detail — subscribes to mp:<uuid> via useSSE)

# Tech tracking
tech-stack:
  added: []  # zero new dependencies — pure stdlib + existing uuid/slog/chi
  patterns:
    - "SSE handler emits snapshot-ready marker BEFORE entering the event loop; client knows to fetch REST snapshot immediately"
    - "HeartbeatInterval is a package-level var (not const) so tests override it to 50ms without real 30s waits"
    - "parseTopics validates: non-empty, ≤64 topics, each matches anchored topicRegex — all errors return 400 BEFORE SSE headers"
    - "connID (uuid) is internal correlation only — NOT emitted on the wire (wire format: {ready, topics} only)"
    - "defer Hub.Unsubscribe(connID) is the goroutine-leak guard — fires on ctx.Done() and Hub channel close"

key-files:
  created:
    - "internal/events/handler.go"
    - "internal/events/handler_test.go"
    - "internal/events/routes.go"
  modified:
    - "internal/http/router.go"
    - "internal/cli/serve.go"
    - "Caddyfile"

key-decisions:
  - "HeartbeatInterval as package var instead of const: allows tests to run at 50ms without a 30s wait; production value remains 30s"
  - "Snapshot wire format {ready: bool, topics: []string} — no conn_id field exposed to browser (plan §wire-format verbatim)"
  - "No subscribe/unsubscribe HTTP endpoints in Phase 4: topic-set changes handled by client reconnect with new ?topics= (D-04 semantics)"
  - "EventsDeps always populated unconditionally in serve.go (Hub starts at boot regardless of CS availability) — SSE is not CS-dependent"

requirements-completed: [DASH-02, DASH-03]

# Metrics
duration: 5min
completed: 2026-05-11
---

# Phase 4 Plan 03: SSE Endpoint Summary

**GET /api/events SSE handler — snapshot-on-connect, heartbeat, Hub fan-out, Caddy non-buffering**

## Performance

- **Duration:** ~5 minutes
- **Started:** 2026-05-11T14:03:37Z
- **Completed:** 2026-05-11T14:08:41Z
- **Tasks:** 4
- **Files modified:** 6 (3 new, 3 modified)

## Accomplishments

- **SSE handler** (`internal/events/handler.go`): validates ?topics=, writes SSE headers, subscribes to Hub, emits snapshot-ready marker, loops forwarding measurement events + heartbeats, unsubscribes on client disconnect.
- **routes.go** (`internal/events/routes.go`): `RegisterRoutes(r chi.Router, deps Deps)` mounts only `GET /api/events`.
- **Router wiring** (`internal/http/router.go`): `EventsDeps *events.Deps` field added; nil-guard block under authenticated group mirrors DeviceDeps pattern.
- **Boot wiring** (`internal/cli/serve.go`): `events.NewHub` constructed, `go eventsHub.Run(ctx, pool, log)` started alongside resolver; `EventsDeps` populated unconditionally.
- **Caddyfile**: `@sse path` matcher extended from `/sse /sse/*` to include `/api/events /api/events/*`; `flush_interval -1` still in effect.
- **Tests**: 11 handler + parseTopics tests all pass under `-race`.

## SSE Wire Format

```
HTTP/1.1 200 OK
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
X-Accel-Buffering: no

event: snapshot
data: {"ready":true,"topics":["dashboard:global","mp:abc-123"]}

: heartbeat

event: measurement
data: {"metering_point_id":"abc-123","time":"2026-05-11T12:34:56.789Z","cumulative_value":1234.56,"instant_value":4.2,"quality":"ok","battery_pct":87,"rssi":-65}

: heartbeat
```

Each frame ends with `\n\n`. Heartbeat is a comment `: heartbeat\n\n` every 30 seconds.

**Snapshot payload shape** (verbatim — `conn_id` is NOT emitted):
```json
{"ready": true, "topics": ["dashboard:global"]}
```

## Topic Regex + Max-Topics Cap

```go
var topicRegex = regexp.MustCompile(`^(dashboard:global|mp:[0-9a-f-]{36}(:uplinks)?)$`)
const maxTopicsPerConnection = 64
```

- Anchored (`^` and `$`) — no catastrophic backtracking (T-04-03-04).
- UUID portion bounded to `[0-9a-f-]{36}` (lowercase hex + hyphens only).
- Requests with > 64 topics receive 400 BEFORE any SSE headers are written.

Valid topic shapes:
- `dashboard:global` — every uplink, all subscribers
- `mp:<36-char-uuid>` — per metering-point delta
- `mp:<36-char-uuid>:uplinks` — per metering-point Uplinks log tab

## Subscribe/Unsubscribe Decision (Not Shipped)

Phase 4 ships **without** POST /api/events/subscribe or DELETE /api/events/subscribe endpoints. Topic-set changes happen via client reconnect with a new `?topics=...` query string (D-04 reconnect-and-snapshot semantics). Every reconnect emits a fresh snapshot marker so the React-Query cache invalidation path is uniform. Two fewer handlers, no conn_id round-tripping, no auth-owner-matches-conn-id check. Tracked as a non-blocking future enhancement.

## Hub Boot Wiring

`internal/cli/serve.go` now starts the Hub immediately after the resolver listener:

```go
eventsHub := events.NewHub(log.With("component", "events"))
go eventsHub.Run(ctx, pool, log.With("component", "events.listener"))
```

`EventsDeps` is populated unconditionally (Hub does not depend on ChirpStack):

```go
EventsDeps: &events.Deps{
    Hub:    eventsHub,
    Logger: log.With("component", "events.handler"),
},
```

## Caddyfile Amendment

Before (Phase 1–3):
```caddy
@sse path /sse /sse/*
```

After (Phase 4 Plan 03):
```caddy
@sse path /sse /sse/* /api/events /api/events/*
```

Verify in a deploy: `grep '@sse' Caddyfile` — must include `/api/events`.

The `@sse` handle block still contains `flush_interval -1`, `read_buffer 0`, `response_header_timeout 0` so live updates (DASH-02) work through Caddy without buffering.

## Task Commits

1. **Task 1: SSE handler + tests (TDD RED+GREEN)** — `46ac165` (feat)
2. **Task 2: routes.go + router wiring** — `dc881d3` (feat)
3. **Task 3: Boot Hub.Run in serve.go** — `9771d2c` (feat)
4. **Task 4: Caddyfile @sse matcher amendment** — `b381ec9` (chore)

## Files Created/Modified

### Created
- `internal/events/handler.go` — SSE handler, Deps struct, parseTopics, HeartbeatInterval var
- `internal/events/handler_test.go` — 11 tests covering headers, snapshot, heartbeat, measurement forwarding, client disconnect, topic validation, overflow
- `internal/events/routes.go` — RegisterRoutes mounting GET /api/events only

### Modified
- `internal/http/router.go` — EventsDeps field + nil-guard RegisterRoutes call
- `internal/cli/serve.go` — events import, NewHub construction, go eventsHub.Run, EventsDeps wiring
- `Caddyfile` — @sse matcher + header comment updated

## Decisions Made

- **HeartbeatInterval as package var**: Enables test overrides at 50ms instead of waiting 30 real seconds. Production value unchanged at 30s.
- **Snapshot payload omits conn_id**: The wire format contains only `{ready: bool, topics: []string}`. The connID is an internal correlation UUID used for Hub.Subscribe/Unsubscribe only — never sent to the browser.
- **EventsDeps unconditionally populated**: The Hub does not depend on ChirpStack. SSE works even in degraded mode (CS unreachable). This is correct because NOTIFY-based fan-out depends only on Postgres (already required).

## Deviations from Plan

None — plan executed exactly as written.

## Known Stubs

None — handler wires to real Hub, real Unsubscribe on disconnect, real HeartbeatInterval.

## Threat Flags

No new network surface beyond the planned `GET /api/events`. T-04-03-01 (unauthenticated drain) mitigated by mounting under authenticated group. T-04-03-04 (regex DoS) mitigated by anchored regex + 64-topic cap.

## Self-Check: PASSED

- `internal/events/handler.go` — FOUND
- `internal/events/handler_test.go` — FOUND
- `internal/events/routes.go` — FOUND
- `internal/http/router.go` has `EventsDeps *events.Deps` — FOUND
- `internal/http/router.go` has `events.RegisterRoutes` — FOUND
- `internal/cli/serve.go` has `events.NewHub` — FOUND
- `internal/cli/serve.go` has `eventsHub.Run` — FOUND
- `Caddyfile` @sse matcher includes `/api/events` — FOUND
- Commit `46ac165` (Task 1) — FOUND
- Commit `dc881d3` (Task 2) — FOUND
- Commit `9771d2c` (Task 3) — FOUND
- Commit `b381ec9` (Task 4) — FOUND
- `go build ./...` — exits 0
- `go test ./internal/events/ -count=1 -race` — 21 PASS

---
*Phase: 04-realtime-dashboard*
*Plan: 03*
*Completed: 2026-05-11*
