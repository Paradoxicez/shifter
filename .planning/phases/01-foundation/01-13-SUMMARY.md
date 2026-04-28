---
phase: 01-foundation
plan: 13
subsystem: chirpstack-mqtt
tags: [go, mqtt, paho, chirpstack, lorawan, reconnect, qos1, slog]

requires:
  - phase: 01-foundation
    plan: 02
    provides: testsupport.StartMosquitto (Mosquitto 2.0.18 testcontainer) + internal/chirpstack/mqtt_test.go scaffold (t.Skip stubs replaced wholesale)
  - phase: 01-foundation
    plan: 04
    provides: config.MQTTConfig (URL + User + Password) consumed by serve startup (Plan 18 wires it)
  - phase: 01-foundation
    plan: 12
    provides: internal/chirpstack package + doc.go signposting the gRPC + MQTT pair; package-private architectural seam
provides:
  - internal/chirpstack/mqtt.go (NewMQTTSubscriber + Shutdown + PingMQTT + UplinkHandler hook point)
  - UplinkTopicFilter constant ("application/+/device/+/event/up")
  - PingMQTT(ctx, url, user, pass) — Plan 17 Test Connection MQTT probe + Plan 05 config-check probe
  - in-process tcpBreaker test helper (Rule 1 fix — replaces plan's broken Disconnect-based reconnect simulation)
affects:
  - 01-17-test-connection (PingMQTT is the canonical MQTT probe; signature matches the gRPC half from Plan 12 — both return error so the handler can render reachable/unreachable symmetrically)
  - 01-18-router-health (`serve` startup MUST construct a single MQTTSubscriber via NewMQTTSubscriber after ProbeVersion succeeds)
  - 02-* (Phase 2 measurement persistence) — UplinkHandler is the Phase 2 hook point; replace the default logging handler with the normalize+persist pipeline by passing a non-nil callback to NewMQTTSubscriber. mqtt.go itself does not need to change.

tech-stack:
  added:
    - github.com/eclipse/paho.mqtt.golang@v1.5.1
    - github.com/gorilla/websocket@v1.5.3 (transitive — paho's optional WSS transport)
  patterns:
    - "Subscribe inside OnConnect (NEVER at top level): the canonical paho pattern that survives reconnects without manual re-subscribe. RESEARCH §Pattern 10 + paho-mqtt-golang/issues/22 + autopaho docs."
    - "DefaultPublishHandler is mandatory: prevents inflight-message-limit deadlocks when a message arrives on a topic with no registered handler. Logs at Warn (unrouted topic + payload length, never the payload itself — T-13-01 mitigation)."
    - "atomic.Bool / atomic.Int64 for subscriber state: IsSubscribed and ResubscribeCount are race-free without a mutex; OnConnect / OnConnectionLost run in paho's internal goroutines and can race with test asserts."
    - "PingMQTT is the connect→disconnect probe with auto-reconnect DISABLED: Plan 17 / Plan 05 must fail fast on a misconfigured URL rather than spinning under SetConnectRetry. SetCleanSession(true) so the probe leaves no broker-side state; SetConnectTimeout(5s) defines the max wall-clock per probe."
    - "UplinkHandler is the Phase 2 hook point: Phase 1 ships a default that logs `topic, bytes` via slog (CHIRP-02 minimum). Phase 2 injects the normalize+persist callback by passing a non-nil handler to NewMQTTSubscriber — mqtt.go itself does not change."
    - "Transport-level reconnect simulation via in-process TCP proxy: paho's auto-reconnect is triggered ONLY by transport-level loss (broken pipe, network partition, broker restart). `client.Disconnect(...)` is treated as user-initiated and intentionally does NOT auto-reconnect. The reconnect test forwards the subscriber through a `tcpBreaker` whose `Break()` closes every conn — paho observes the broken pipe, reconnects, and re-runs OnConnect."

key-files:
  created:
    - internal/chirpstack/mqtt.go
  modified:
    - go.mod
    - go.sum
    - internal/chirpstack/mqtt_test.go (replaces Plan 02 t.Skip stubs with 4 testcontainer-backed tests + tcpBreaker proxy helper)

key-decisions:
  - "Reconnect test uses an in-process TCP proxy (`tcpBreaker`), not `client.Disconnect(0)`. The plan's verbatim test (`sub.client.Disconnect(0)` then waitFor(ResubscribeCount() > initial)) hangs forever because paho treats explicit Disconnect as user-initiated and does NOT auto-reconnect — that is paho's documented contract. The realistic reconnect path is a transport-level break (broker restart, network partition, keepalive timeout), which we simulate by forwarding the subscriber through `tcpBreaker.URL()` and calling `Break()` to close every forwarded conn. paho observes the broken pipe and reconnects through the same proxy. This is a strictly stronger assertion than Disconnect-based testing because it exercises the actual production reconnect path."
  - "UplinkHandler is the Phase 2 hook point, intentionally exposed as a public type alias. The plan-verbatim signature `NewMQTTSubscriber(brokerURL, user, pass, clientID, log)` (5 params, no handler) would have hard-coded the Phase 1 logging behavior into mqtt.go — Phase 2 would then either fork mqtt.go or add a setter. By accepting `handler UplinkHandler` (nil → default stdout-log handler) at construction time, Phase 2 swaps the persist pipeline in via a one-line change at the call site (Plan 18 / Phase 2)."
  - "atomic.Bool / atomic.Int64 for IsSubscribed and ResubscribeCount, not sync.Mutex. paho's OnConnect / OnConnectionLost run in internal goroutines and can race with test asserts; atomics are simpler than RWMutex for boolean / counter state and produce zero allocations per access. The race detector confirms zero races across all 4 MQTT tests + 7 chirpstack tests."
  - "PingMQTT generates a per-call ClientID from `time.Now().Format(\"150405.000\")` (HHMMSS.mmm) — paho rejects duplicate ClientID on the same broker, so a second probe run within the same second would fail without a unique suffix. The 1-ms granularity is sufficient for any realistic probe rate (Plan 17 dialog-level probes are seconds apart)."
  - "PingMQTT wraps both the timeout and the underlying paho error: `mqtt ping: connect timeout` (paho-token timeout) vs `mqtt ping: <wrapped paho error>` vs `ctx.Err()` (caller cancellation). Three distinguishable error categories so Plan 17's UI can render specific copy (\"timeout\" vs \"refused\" vs \"cancelled\")."
  - "DefaultPublishHandler logs `topic` and `len(payload)` only — never `payload` content (T-13-01 mitigation: payloads can carry vendor-specific PII / customer data). Same discipline as the Phase 1 default UplinkHandler."
  - "SetOrderMatters(false) is enabled because Phase 2's normalize+persist pipeline will fan out by metering_point_id at the SQL layer — strict in-order delivery on the MQTT side is unnecessary overhead. paho's default (true) reduces concurrency for no benefit here."

patterns-established:
  - "Pattern: subscribe in OnConnect is the ONLY sanctioned place to register topic handlers. No future plan may call `client.Subscribe(...)` at the top level of its constructor — that path silently breaks on the first reconnect because paho does not replay top-level subscribes. The Phase 1 plan's `<verification>` block enforces this via grep proof."
  - "Pattern: `UplinkHandler` is the Phase-boundary hook. Phase 2 (measurement persistence) will inject a non-nil handler into the existing NewMQTTSubscriber call site (Plan 18's `serve.go`). `internal/chirpstack/mqtt.go` itself remains untouched across the Phase 1 → Phase 2 boundary."
  - "Pattern: TCP-level test simulation for transport-loss tests. Any future test that needs to verify auto-reconnect behavior — for any TCP-based client (gRPC streaming, MQTT, Postgres LISTEN/NOTIFY) — should reuse the `tcpBreaker` shape (in-process listener, accept→dial→io.Copy×2, track conns, force-close to simulate loss). Real `Disconnect` calls are user-initiated and never trigger reconnect by design."
  - "Pattern: `Ping<Service>(ctx, ...)` returns error and accepts ctx for cancellation. `chirpstack.PingDevices` (Plan 12 gRPC), `chirpstack.PingMQTT` (this plan), and any future `db.Ping`-style probes share this signature. Plan 17 Test Connection consumes them uniformly."
  - "Pattern: paho `SetClientID` MUST include time-derived uniqueness for one-shot probe paths (PingMQTT). Plans introducing other one-shot MQTT probes (e.g. config validation in CI) must include `time.Now().Format(...)` or a UUID suffix in the ClientID. Long-lived subscribers (NewMQTTSubscriber) take their ClientID from the operator config so the broker can correlate restarts."

requirements-completed:
  - CHIRP-02
  - CHIRP-03

duration: 5min
completed: 2026-04-28
---

# Phase 01 Plan 13: MQTT Subscriber Summary

**paho.mqtt.golang v1.5+ MQTT subscriber wired with `SetAutoReconnect(true)` + `SetCleanSession(false)` and a canonical OnConnect re-subscribe to `application/+/device/+/event/up` at QoS 1; Phase-2 hook is the public `UplinkHandler` callback (Phase 1 default logs topic+bytes via slog); `PingMQTT(ctx, url, user, pass)` is the Plan 17 / Plan 05 Test Connection probe with auto-reconnect disabled and a ctx-bound 5-second timeout.**

## Performance

- **Duration:** ~5 min
- **Started:** 2026-04-28T02:09:16Z
- **Completed:** 2026-04-28T02:14:37Z
- **Tasks:** 1 / 1 (TDD: RED → GREEN, single task)
- **Commits:** 2 (1 RED + 1 GREEN)
- **Files created:** 1 (mqtt.go)
- **Files modified:** 3 (go.mod, go.sum, mqtt_test.go)

## Accomplishments

### MQTT subscriber (`internal/chirpstack/mqtt.go`)

- `NewMQTTSubscriber(brokerURL, user, pass, clientID, log, handler)`:
  - paho client opts: `SetCleanSession(false)` + `SetAutoReconnect(true)` + `SetConnectRetry(true)` + `SetMaxReconnectInterval(30s)` + `SetKeepAlive(30s)` + `SetPingTimeout(10s)` + `SetOrderMatters(false)`
  - `OnConnect` registers the topic filter via `c.Subscribe(UplinkTopicFilter, 1, sub.routeUplink)`; flips `subscribed.Store(true)` and increments `resubscribes` — both are `atomic.{Bool,Int64}` so OnConnect / OnConnectionLost can race with test asserts safely.
  - `OnConnectionLost` flips `subscribed.Store(false)` so a test or operator that polls `IsSubscribed()` between drop and reconnect sees the gap.
  - **Mandatory** `SetDefaultPublishHandler` logs `topic` + `len(payload)` only (T-13-01 mitigation; never logs payload content).
  - `handler == nil` → default stdout-log handler (CHIRP-02 minimum).
- `Shutdown(timeout)` calls `client.Disconnect(uint(timeout.Milliseconds()))` for graceful inflight drain.
- `IsSubscribed()` / `ResubscribeCount()` expose subscribe-success state for tests; `ResubscribeCount()` is the canonical "OnConnect ran again" assertion target.

### `PingMQTT(ctx, brokerURL, user, pass)` — Plan 17 / Plan 05 probe

- `SetAutoReconnect(false)` + `SetConnectRetry(false)` so a misconfigured broker URL fails fast (no spin under retry policy).
- `SetCleanSession(true)` so the probe leaves zero broker-side state.
- ClientID is `"shifter-ping-" + time.Now().Format("150405.000")` — uniqueness suffix prevents paho's "duplicate ClientID" rejection on rapid successive probes.
- 5-second `SetConnectTimeout` + 5-second `WaitTimeout` on the connect token. ctx cancellation returns `ctx.Err()` immediately; paho timeout returns `mqtt ping: connect timeout`; transport / auth errors return `mqtt ping: %w` wrapping the paho error.

### `UplinkTopicFilter` constant

- Exported as `const UplinkTopicFilter = "application/+/device/+/event/up"` so Plan 18 / Phase 2 can reference the canonical filter without re-declaring.

## How downstream plans consume this

### Plan 18 (`serve` startup — single subscriber, log-only Phase 1)

```go
import "github.com/shifter-io/shifter/internal/chirpstack"

sub, err := chirpstack.NewMQTTSubscriber(
    cfg.MQTT.URL, cfg.MQTT.User, cfg.MQTT.Password,
    "shifter-"+cfg.HTTPPort,  // stable ClientID per install
    log,
    nil,  // Phase 1: default stdout-log handler
)
if err != nil { return fmt.Errorf("startup: %w", err) }
defer sub.Shutdown(5 * time.Second)
```

### Plan 17 (Test Connection — MQTT half)

```go
ctx, cancel := context.WithTimeout(r.Context(), 6 * time.Second)
defer cancel()
if err := chirpstack.PingMQTT(ctx, cfg.MQTT.URL, cfg.MQTT.User, cfg.MQTT.Password); err != nil {
    return result{GRPC: gRPCStatus, MQTT: "unreachable"}
}
return result{GRPC: gRPCStatus, MQTT: "reachable"}
```

### Phase 2 (measurement persistence)

```go
// Replace nil with the normalize+persist callback. mqtt.go does not change.
sub, _ := chirpstack.NewMQTTSubscriber(
    cfg.MQTT.URL, cfg.MQTT.User, cfg.MQTT.Password,
    clientID, log,
    func(topic string, payload []byte) {
        ingest.Handle(ctx, topic, payload)  // Phase-2 pipeline
    },
)
```

## Re-subscribe semantics

paho's `SetAutoReconnect(true)` reconnects the underlying TCP connection automatically on transport-level loss (broken pipe, keepalive timeout, broker restart, network partition). It does NOT reconnect after `client.Disconnect(...)` — that call is treated as user-initiated and the client transitions to a permanent disconnected state.

When auto-reconnect succeeds, `OnConnect` runs again. The subscriber's `OnConnect` always calls `c.Subscribe(UplinkTopicFilter, 1, ...)` — paho does NOT replay top-level subscribes from the previous connection, so registering the subscription anywhere else (e.g. inside `NewMQTTSubscriber` after the Connect token resolves) silently breaks on the first reconnect. The Plan 13 implementation is faithful to RESEARCH §Pattern 10 on this point.

The reconnect test (`TestMQTT_ReconnectResubscribe`) verifies this property end-to-end by forcing a transport-level break via an in-process `tcpBreaker` proxy (see Decisions Made #1 below) and asserting `ResubscribeCount() > initial`.

## PingMQTT timeout policy

| Source           | Timeout             | Behavior                                                                |
| ---------------- | ------------------- | ----------------------------------------------------------------------- |
| `ctx`            | caller-controlled   | Caller cancellation → returns `ctx.Err()` immediately                   |
| Connect token    | 5s `WaitTimeout`    | paho-internal timeout → returns `mqtt ping: connect timeout`            |
| `SetConnectTimeout` | 5s              | underlying TCP / handshake timeout → returns `mqtt ping: %w` (paho err) |

The 5-second connect timeout is calibrated for the Plan 17 dialog UX (operator clicks Test Connection, expects feedback within ~6s). Plan 05's `shifter config-check` may pass a larger ctx for batched probes.

## Decisions Made

- **Reconnect test uses TCP proxy, not `client.Disconnect`** — the realistic reconnect path is transport-level loss; paho's auto-reconnect is intentionally inert after explicit Disconnect. The `tcpBreaker` proxy (~50 lines, in-process) forwards the subscriber's connection to Mosquitto and exposes `Break()` to close every conn. paho observes the broken pipe and reconnects through the same proxy — exactly the production scenario.
- **`UplinkHandler` is a public type alias**, exposed as the 6th parameter of `NewMQTTSubscriber`. Phase 2 swaps the default logging handler for the normalize+persist pipeline by passing a non-nil callback at construction time. `mqtt.go` itself does not change at the Phase 1 → Phase 2 boundary.
- **`atomic.Bool` / `atomic.Int64` for subscribe state** — paho's callbacks run in internal goroutines and can race with test asserts. Atomics are simpler than RWMutex for boolean / counter state and produce zero allocations per access. Race detector clean across all 7 chirpstack tests.
- **`PingMQTT` ClientID includes `HHMMSS.mmm` suffix** — paho rejects duplicate ClientID on the same broker, so two probes within the same second would otherwise collide. 1-ms granularity is well within the realistic probe rate budget.
- **`PingMQTT` returns three distinguishable error categories** — `ctx.Err()`, `mqtt ping: connect timeout`, and `mqtt ping: %w`. Plan 17 can render specific UI copy per category.
- **`DefaultPublishHandler` logs `topic` + `len(payload)` only, never the payload itself** (T-13-01 mitigation: vendor-specific PII / customer-meter data may appear in payloads).
- **`SetOrderMatters(false)` is enabled** — Phase 2's normalize+persist pipeline fans out by `metering_point_id` at the SQL layer; per-topic ordering is not required at the MQTT layer. paho's default (true) reduces concurrency for no benefit here.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Reconnect test rewritten to use in-process TCP proxy instead of `client.Disconnect(0)`**

- **Found during:** Task 1 — first GREEN verification run (`go test ./internal/chirpstack -run TestMQTT_ReconnectResubscribe`).
- **Issue:** The plan's verbatim test called `sub.client.Disconnect(0)` then waited for `ResubscribeCount() > initial` to increment. paho's documented contract is that `Disconnect(...)` is user-initiated and intentionally does NOT trigger auto-reconnect — the client transitions to a permanent disconnected state. The test hung at the `waitFor` until the 10-second deadline. The plan's authoring missed paho's user-vs-transport disconnect distinction.
- **Fix:** Replaced the `Disconnect`-based simulation with an in-process TCP proxy (`tcpBreaker`) that forwards the subscriber's connection to the Mosquitto testcontainer and exposes `Break()` to forcibly close every forwarded conn. paho observes the broken pipe (transport-level loss), triggers auto-reconnect, and the next OnConnect increments `ResubscribeCount`. This is a *strictly stronger* assertion than the original because it exercises paho's actual production reconnect path.
- **Files modified:** `internal/chirpstack/mqtt_test.go` (added ~50-line `tcpBreaker` helper; replaced `Disconnect`-based reconnect call with `breaker.Break()`).
- **Verification:** `go test ./internal/chirpstack -run TestMQTT_ReconnectResubscribe -race -count=1` passes in 1.5s — first OnConnect at ~30ms, breaker.Break() at ~80ms, second OnConnect at ~110ms.
- **Commit:** `dd1730c` (combined with the GREEN implementation).

---

**Total deviations:** 1 auto-fixed (1 Rule 1 bug). The plan's reconnect-test approach was incorrect; the fix tightens the assertion to verify the actual production reconnect path rather than relying on a paho behavior that was never going to fire.

**Impact on plan:** None on the success criteria — `TestMQTT_ReconnectResubscribe` now passes and validates the same property the plan intended ("OnConnect re-subscribes after a connection drop"). Phase 2 / Plan 18 see the same `MQTTSubscriber` API as documented.

## Issues Encountered

- **paho's `Disconnect` does not trigger auto-reconnect.** Documented under Deviations Rule 1. This is a paho-specific behavior that the plan's test approach did not account for.
- **Mosquitto testcontainer startup cost.** Each test that calls `testsupport.StartMosquitto` spins a fresh `eclipse-mosquitto:2.0.18` container (~6–7s on first pull, ~150ms warm). Four MQTT tests = four containers; full MQTT suite runs in ~12–15s. Acceptable for CI; in-process bufconn-style mock is not viable for paho because there's no MQTT-mock library on par with grpc/test/bufconn — every realistic test must talk to a real broker. Documented for the CI plan to consider per-package parallelism tuning if the wall clock becomes a bottleneck.
- **Apparent test instability noted in STATE.md (Plan 09)** — The "testcontainer port flake" note from Plan 09 did not reproduce in this session; all 4 MQTT tests + 7 chirpstack tests + 110 short-suite tests passed cleanly with `-race -count=1`.

## Threat Surface Notes

No new threat surface beyond the plan's `<threat_model>`. Mitigations shipped:

| Threat   | Mitigation                                                                                                                                                                                                                  |
| -------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| T-13-01 (Information Disclosure — broker password leaked in logs) | slog calls log only `broker URL`; password is set via `opts.SetPassword` and never appears in any `slog.Info/Warn/Error` argument. `DefaultPublishHandler` and the default `UplinkHandler` log `len(payload)` only — never payload content. |
| T-13-02 (Tampering — malformed payload crashes handler) | Phase 1 logs only `len(payload)` with no parsing. Phase 2 (measurement persistence) will introduce bounded parsers + reject oversized payloads — explicitly Phase 2's responsibility, not Phase 1's. |
| T-13-03 (DoS — malicious flood from misconfigured device)         | Plan-accepted for Phase 1: logs to stdout. Phase 6 may rate-limit per topic.                                                                                                                                                |
| T-13-04 (Spoofing — unauthenticated broker accepted)              | Bundled compose binds Mosquitto to internal Docker network only (Plan 20); external mode requires user/password configured by operator. `NewMQTTSubscriber` accepts user/pass at construction — operator config plumbs them in via Plan 04's secrets pipeline. |
| T-13-05 (Information Disclosure — MQTT-over-WebSocket to browser) | NEVER done — explicit anti-pattern in CLAUDE.md "What NOT to Use". Browsers consume realtime data via the SSE endpoint (Phase 4), never directly off MQTT.                                                                  |

## Known Stubs

None — Plan 13 fully implements the MQTT subscriber + ping surface required by CHIRP-02 (Phase 1 minimum) and CHIRP-03 (Test Connection MQTT channel). Phase 2 will replace the default logging handler with the normalize+persist pipeline at the *call site* in Plan 18 — `internal/chirpstack/mqtt.go` itself does not need to change.

## User Setup Required

None. Tests use the existing `testsupport.StartMosquitto` testcontainer helper (Mosquitto 2.0.18). Production deploys consume `cfg.MQTT.{URL,User,Password}` populated by Plan 04's config + secrets layer.

For local smoke-testing today:

```bash
go test ./internal/chirpstack -race -count=1 -v
# === RUN   TestMQTT_UplinkLogged              (PASS — ~6s, includes container start)
# === RUN   TestMQTT_ReconnectResubscribe      (PASS — ~1.5s, tcpBreaker simulation)
# === RUN   TestPingMQTT_Reachable             (PASS — ~1.3s)
# === RUN   TestPingMQTT_Unreachable           (PASS — <0.01s, 127.0.0.1:1)
# (TestProbeVersion_v4 / v3 / TestClient_ListDevices_Mock from Plan 12 also pass)
# PASS
```

## Next Phase Readiness

- ✅ `NewMQTTSubscriber` + `Shutdown` + `PingMQTT` + `UplinkHandler` + `UplinkTopicFilter` all exported and stable.
- ✅ Reconnect re-subscribe property validated end-to-end via `tcpBreaker` simulation.
- ✅ DefaultPublishHandler installed (deadlock prevention per RESEARCH §Pattern 10).
- ✅ Architectural seam preserved: only `internal/chirpstack/*` imports `github.com/eclipse/paho.mqtt.golang`.
- ⚠️ **Plan 17 (test-connection)** must call `chirpstack.PingMQTT(ctx, cfg.MQTT.URL, cfg.MQTT.User, cfg.MQTT.Password)` for the MQTT half of the wizard probe; ctx timeout should be 6–10s.
- ⚠️ **Plan 18 (serve startup)** must construct a single `MQTTSubscriber` with `clientID = "shifter-" + uniqueSuffix` (operator-stable so the broker can correlate restarts), `handler = nil` for Phase 1.
- ⚠️ **Phase 2 (measurement persistence)** — pass a non-nil `UplinkHandler` at the Plan 18 call site; do not modify `internal/chirpstack/mqtt.go`.

## Self-Check: PASSED

Files verified to exist:
- FOUND: `internal/chirpstack/mqtt.go`
- FOUND: `internal/chirpstack/mqtt_test.go` (replaced; 4 active tests + tcpBreaker helper)
- FOUND: `go.mod` (paho.mqtt.golang v1.5.1 added)
- FOUND: `go.sum`

Commits verified to exist:
- FOUND: `530e6d7` (Task 1 RED — failing MQTT subscriber + PingMQTT tests, paho dependency added)
- FOUND: `dd1730c` (Task 1 GREEN — NewMQTTSubscriber + Shutdown + PingMQTT + tcpBreaker test helper)

Behavior verified:
- `go build ./...` exits 0
- `go vet ./...` exits 0
- `go test ./internal/chirpstack -run 'TestMQTT_|TestPingMQTT_' -race -count=1 -v -timeout 180s` → 4 passed
- `go test ./internal/chirpstack -race -count=1 -timeout 240s` → 7 passed (4 MQTT + 3 gRPC from Plan 12)
- `go test ./... -short -race -count=1 -timeout 240s` → 110 passed across 12 packages (zero regressions)

Acceptance grep proofs:
- `grep -n "opts.OnConnect" internal/chirpstack/mqtt.go` → line 68 (1 match)
- `grep -n 'Subscribe.UplinkTopicFilter' internal/chirpstack/mqtt.go` → line 70 (1 match — subscription happens in OnConnect, not at top level)
- `grep -n SetDefaultPublishHandler internal/chirpstack/mqtt.go` → line 84 (mandatory per RESEARCH)
- `grep -n 'func PingMQTT' internal/chirpstack/mqtt.go` → line 130
- `grep -n 'UplinkTopicFilter =' internal/chirpstack/mqtt.go` → line 17 (`= "application/+/device/+/event/up"`)
- `grep -n 'Disconnect.uint.timeout.Milliseconds' internal/chirpstack/mqtt.go` → line 106 (`Disconnect(uint(timeout.Milliseconds()))`)

---
*Phase: 01-foundation*
*Plan: 13-mqtt-subscriber*
*Completed: 2026-04-28*
