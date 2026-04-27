---
phase: 01-foundation
plan: 13
type: execute
wave: 9
depends_on: [02, 04]
files_modified:
  - go.mod
  - go.sum
  - internal/chirpstack/mqtt.go
  - internal/chirpstack/mqtt_test.go
autonomous: true
requirements:
  - CHIRP-02
  - CHIRP-03
must_haves:
  truths:
    - "NewMQTTSubscriber connects to a Mosquitto testcontainer and subscribes to application/+/device/+/event/up"
    - "Subscription is registered in OnConnect (re-subscribes after reconnect)"
    - "DefaultPublishHandler logs unrouted messages (RESEARCH §Pattern 10 mandatory)"
    - "Shutdown(timeout) calls client.Disconnect cleanly"
    - "Publishing an uplink to the topic causes handleUplink to log a structured event"
    - "PingMQTT(ctx, url, user, pass) used by Test Connection (Plan 17) connects + disconnects within ctx timeout"
  artifacts:
    - path: "internal/chirpstack/mqtt.go"
      provides: "NewMQTTSubscriber + Shutdown + PingMQTT (RESEARCH §Pattern 10 verbatim)"
      contains: "OnConnect"
  key_links:
    - from: "internal/chirpstack/mqtt.go"
      to: "github.com/eclipse/paho.mqtt.golang"
      via: "paho v1.5+ client + OnConnect re-subscribe"
      pattern: "paho.mqtt.golang"
---

<objective>
Implement the MQTT subscriber per RESEARCH §Pattern 10: paho.mqtt.golang v1.5+ client connecting with `SetAutoReconnect(true)` and re-subscribing in `OnConnect`. Phase 1 only LOGS uplinks to stdout (CHIRP-02 minimum); Phase 2 will normalize + persist. Also expose `PingMQTT(ctx, url, user, pass)` for Plan 17 Test Connection's MQTT channel.

Purpose: CHIRP-02 (MQTT subscription) + CHIRP-03 (Test Connection MQTT channel).

Output: `go test ./internal/chirpstack -run TestMQTT_` passes against a Mosquitto testcontainer.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/01-foundation/01-RESEARCH.md
@.planning/phases/01-foundation/01-CONTEXT.md
@01-02-test-harness-PLAN.md

<interfaces>
RESEARCH §Pattern 10 (lines 772-849) — verbatim subscriber pattern. Key points:
- `SetCleanSession(false)` to resume QoS1 inflight
- `SetAutoReconnect(true)`, `SetConnectRetry(true)`, `SetMaxReconnectInterval(30s)`
- Subscribe in `OnConnect` (canonical pattern; PITFALLS §"Subscribing to MQTT before OnConnect")
- `SetDefaultPublishHandler` is MANDATORY (prevents inflight-limit deadlocks)
- Topic: `application/+/device/+/event/up` at QoS 1

Public API:
```go
func NewMQTTSubscriber(brokerURL, user, pass, clientID string, log *slog.Logger) (*MQTTSubscriber, error)
func (s *MQTTSubscriber) Shutdown(timeout time.Duration)
func PingMQTT(ctx context.Context, brokerURL, user, pass string) error  // used by Plan 17
```

Mosquitto helper from Plan 02: `testsupport.StartMosquitto(t)` returns broker URL.
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: MQTTSubscriber with OnConnect re-subscribe + PingMQTT + tests</name>
  <files>go.mod, go.sum, internal/chirpstack/mqtt.go, internal/chirpstack/mqtt_test.go</files>
  <read_first>
    - .planning/phases/01-foundation/01-RESEARCH.md §"Pattern 10: MQTT subscriber lifecycle" (lines 772-849)
    - .planning/research/PITFALLS.md (no specific MQTT pitfall but CONTEXT.md MQTT topic spec)
    - 01-02-test-harness-PLAN.md (testsupport.StartMosquitto returns "tcp://host:port")
    - .planning/phases/01-foundation/01-VALIDATION.md (TestMQTT_ReconnectResubscribe, TestMQTT_UplinkLogged names)
  </read_first>
  <behavior>
    - TestMQTT_UplinkLogged: start Mosquitto testcontainer; create subscriber; publish a message to `application/abc/device/01.../event/up`; assert handleUplink got it (via a callback hook the test injects).
    - TestMQTT_ReconnectResubscribe: start subscriber; force-disconnect via Mosquitto restart (or a low-level disconnect); subscriber's OnConnect fires again on reconnect; re-publish topic; handler still receives message.
    - TestMQTT_DefaultPublishHandler: publish to a topic NOT matching the subscription; default handler logs but doesn't crash.
    - TestMQTT_Shutdown: Shutdown(5s) returns; subsequent IsConnected() == false.
    - TestPingMQTT_Reachable: against running Mosquitto returns nil.
    - TestPingMQTT_Unreachable: against tcp://127.0.0.1:1 (closed port) returns error within ctx.
  </behavior>
  <action>
1. Install paho:
   ```bash
   go get github.com/eclipse/paho.mqtt.golang@latest
   ```

2. Create `internal/chirpstack/mqtt.go` — VERBATIM from RESEARCH §Pattern 10 with hooks for tests:
   ```go
   package chirpstack

   import (
       "context"
       "fmt"
       "log/slog"
       "sync/atomic"
       "time"

       mqtt "github.com/eclipse/paho.mqtt.golang"
   )

   // UplinkHandler is invoked once per matching MQTT message.
   // Phase 1 default: log to stdout. Phase 2 wires the normalize+persist pipeline.
   type UplinkHandler func(topic string, payload []byte)

   type MQTTSubscriber struct {
       client      mqtt.Client
       log         *slog.Logger
       handler     UplinkHandler
       subscribed  atomic.Bool
       resubscribes atomic.Int64
   }

   const UplinkTopicFilter = "application/+/device/+/event/up"

   // NewMQTTSubscriber connects to the broker and subscribes to the canonical
   // ChirpStack uplink topic. Re-subscription is handled in OnConnect so reconnect
   // events do not leave the subscription unwound.
   //
   // handler may be nil — defaults to a stdout-logging handler.
   func NewMQTTSubscriber(brokerURL, user, pass, clientID string, log *slog.Logger, handler UplinkHandler) (*MQTTSubscriber, error) {
       sub := &MQTTSubscriber{log: log, handler: handler}
       if sub.handler == nil {
           sub.handler = func(topic string, payload []byte) {
               log.Info("uplink received", "topic", topic, "bytes", len(payload))
           }
       }

       opts := mqtt.NewClientOptions().
           AddBroker(brokerURL).
           SetClientID(clientID).
           SetUsername(user).
           SetPassword(pass).
           SetCleanSession(false).
           SetAutoReconnect(true).
           SetConnectRetry(true).
           SetMaxReconnectInterval(30 * time.Second).
           SetKeepAlive(30 * time.Second).
           SetPingTimeout(10 * time.Second).
           SetOrderMatters(false)

       opts.OnConnect = func(c mqtt.Client) {
           log.Info("mqtt connected", "broker", brokerURL)
           t := c.Subscribe(UplinkTopicFilter, 1, sub.routeUplink)
           if t.Wait() && t.Error() != nil {
               log.Error("mqtt subscribe failed", "err", t.Error())
               return
           }
           sub.subscribed.Store(true)
           sub.resubscribes.Add(1)
       }
       opts.OnConnectionLost = func(_ mqtt.Client, err error) {
           log.Warn("mqtt connection lost", "err", err)
           sub.subscribed.Store(false)
       }
       // Mandatory per RESEARCH: prevent inflight-message-limit deadlocks.
       opts.SetDefaultPublishHandler(func(_ mqtt.Client, m mqtt.Message) {
           log.Warn("mqtt unrouted message", "topic", m.Topic(), "len", len(m.Payload()))
       })

       sub.client = mqtt.NewClient(opts)
       if t := sub.client.Connect(); t.Wait() && t.Error() != nil {
           return nil, fmt.Errorf("mqtt connect: %w", t.Error())
       }
       return sub, nil
   }

   func (s *MQTTSubscriber) routeUplink(_ mqtt.Client, m mqtt.Message) {
       s.handler(m.Topic(), m.Payload())
   }

   // Shutdown disconnects the client gracefully, draining inflight publishes.
   func (s *MQTTSubscriber) Shutdown(timeout time.Duration) {
       s.client.Disconnect(uint(timeout.Milliseconds()))
       s.log.Info("mqtt subscriber stopped")
   }

   // IsSubscribed reports the latest subscribe-success state. Used by tests
   // to await OnConnect after reconnection.
   func (s *MQTTSubscriber) IsSubscribed() bool { return s.subscribed.Load() }

   // ResubscribeCount returns how many times OnConnect re-subscribed
   // (1 on first connect; 2+ after reconnects). Used by reconnect tests.
   func (s *MQTTSubscriber) ResubscribeCount() int64 { return s.resubscribes.Load() }

   // PingMQTT performs a connect → optional subscribe → disconnect cycle within ctx.
   // Used by Plan 17 Test Connection MQTT channel and Plan 05 config-check.
   func PingMQTT(ctx context.Context, brokerURL, user, pass string) error {
       opts := mqtt.NewClientOptions().
           AddBroker(brokerURL).
           SetClientID("shifter-ping-" + time.Now().Format("150405.000")).
           SetUsername(user).
           SetPassword(pass).
           SetAutoReconnect(false).
           SetConnectRetry(false).
           SetCleanSession(true).
           SetConnectTimeout(5 * time.Second).
           SetKeepAlive(15 * time.Second)
       cli := mqtt.NewClient(opts)
       done := make(chan error, 1)
       go func() {
           t := cli.Connect()
           if !t.WaitTimeout(5 * time.Second) {
               done <- fmt.Errorf("connect timeout")
               return
           }
           if t.Error() != nil {
               done <- t.Error()
               return
           }
           cli.Disconnect(100)
           done <- nil
       }()
       select {
       case err := <-done:
           return err
       case <-ctx.Done():
           return ctx.Err()
       }
   }
   ```

3. Replace `internal/chirpstack/mqtt_test.go`:
   ```go
   package chirpstack

   import (
       "context"
       "log/slog"
       "os"
       "sync"
       "testing"
       "time"

       mqtt "github.com/eclipse/paho.mqtt.golang"
       "github.com/shifter-io/shifter/internal/testsupport"
       "github.com/stretchr/testify/require"
   )

   func newLogger() *slog.Logger { return slog.New(slog.NewTextHandler(os.Stderr, nil)) }

   func waitFor(t *testing.T, cond func() bool, msg string) {
       t.Helper()
       deadline := time.Now().Add(10 * time.Second)
       for time.Now().Before(deadline) {
           if cond() { return }
           time.Sleep(50 * time.Millisecond)
       }
       t.Fatalf("timeout waiting for %s", msg)
   }

   func TestMQTT_UplinkLogged(t *testing.T) {
       broker := testsupport.StartMosquitto(t)

       var mu sync.Mutex
       got := 0
       sub, err := NewMQTTSubscriber(broker, "", "", "shifter-test-1", newLogger(), func(topic string, _ []byte) {
           mu.Lock(); got++; mu.Unlock()
       })
       require.NoError(t, err)
       defer sub.Shutdown(2 * time.Second)
       waitFor(t, sub.IsSubscribed, "subscription")

       // Publish a fake uplink via a separate paho client.
       opts := mqtt.NewClientOptions().AddBroker(broker).SetClientID("publisher")
       p := mqtt.NewClient(opts)
       require.True(t, p.Connect().Wait())
       defer p.Disconnect(100)

       tok := p.Publish("application/abc/device/0102030405060708/event/up", 1, false, []byte(`{"hi":"world"}`))
       require.True(t, tok.WaitTimeout(2*time.Second))

       waitFor(t, func() bool {
           mu.Lock(); defer mu.Unlock()
           return got >= 1
       }, "uplink delivered")
   }

   func TestMQTT_ReconnectResubscribe(t *testing.T) {
       broker := testsupport.StartMosquitto(t)
       sub, err := NewMQTTSubscriber(broker, "", "", "shifter-test-2", newLogger(), nil)
       require.NoError(t, err)
       defer sub.Shutdown(2 * time.Second)
       waitFor(t, sub.IsSubscribed, "first subscribe")
       initial := sub.ResubscribeCount()
       require.GreaterOrEqual(t, initial, int64(1))

       // Force a reconnect by disconnecting the underlying client manually.
       // paho's auto-reconnect kicks in.
       sub.client.Disconnect(0)
       waitFor(t, func() bool {
           return sub.ResubscribeCount() > initial
       }, "re-subscribe after reconnect")
   }

   func TestPingMQTT_Reachable(t *testing.T) {
       broker := testsupport.StartMosquitto(t)
       ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
       defer cancel()
       require.NoError(t, PingMQTT(ctx, broker, "", ""))
   }

   func TestPingMQTT_Unreachable(t *testing.T) {
       ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
       defer cancel()
       err := PingMQTT(ctx, "tcp://127.0.0.1:1", "", "")
       require.Error(t, err)
   }
   ```
  </action>
  <verify>
    <automated>go test ./internal/chirpstack -run 'TestMQTT_|TestPingMQTT_' -race -count=1 -v -timeout 120s</automated>
  </verify>
  <acceptance_criteria>
    - File `internal/chirpstack/mqtt.go` exports `func NewMQTTSubscriber(brokerURL, user, pass, clientID string, log *slog.Logger, handler UplinkHandler) (*MQTTSubscriber, error)`
    - `NewMQTTSubscriber` calls `opts.OnConnect = ...` AND `c.Subscribe(UplinkTopicFilter, 1, ...)` inside that callback (grep proof: subscription happens in OnConnect, not at top-level)
    - `NewMQTTSubscriber` calls `opts.SetDefaultPublishHandler(...)` (mandatory per RESEARCH)
    - File exports `func PingMQTT(ctx context.Context, brokerURL, user, pass string) error` for Plan 17/05
    - File exports constant `UplinkTopicFilter = "application/+/device/+/event/up"`
    - File exports `(s *MQTTSubscriber) Shutdown(timeout time.Duration)` calling `client.Disconnect(uint(timeout.Milliseconds()))`
    - Command `go test ./internal/chirpstack -run TestMQTT_UplinkLogged -race -timeout 60s` exits 0 (per VALIDATION.md)
    - Command `go test ./internal/chirpstack -run TestMQTT_ReconnectResubscribe -race -timeout 60s` exits 0 (per VALIDATION.md)
    - Command `go test ./internal/chirpstack -run TestPingMQTT_ -race -timeout 30s` exits 0
  </acceptance_criteria>
  <done>
    MQTT subscriber + ping ready. Plan 17 Test Connection wires `PingMQTT`. Phase 2 will replace the default `UplinkHandler` with the normalize+persist pipeline.
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| MQTT broker → Shifter | Mosquitto delivers untrusted topic strings + payload bytes |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-13-01 | Information Disclosure | broker password leaked in logs | mitigate | slog calls log only `broker URL`; password is in `opts.SetPassword`. ASVS V7. |
| T-13-02 | Tampering | malformed payload crashes handler | mitigate | Phase 1 logs only `len(payload)`, no parsing; Phase 2 will introduce bounded parsers + reject oversized. |
| T-13-03 | Denial of Service | malicious flood from a misconfigured device | accept | Phase 1 logs to stdout; Phase 6 may rate-limit per topic. |
| T-13-04 | Spoofing | unauthenticated broker accepted | mitigate | Bundled compose binds Mosquitto to internal Docker network only (Plan 20); external mode requires user/password configured by operator. ASVS V8. |
| T-13-05 | Information Disclosure | exposing broker via WebSocket to browser | mitigate | NEVER done — RESEARCH "MQTT-over-WebSocket exposed to browser" listed as anti-pattern. |
</threat_model>

<verification>
- `internal/chirpstack/mqtt.go` matches RESEARCH §Pattern 10
- Subscription registered inside OnConnect (re-subscribe survives reconnect)
- DefaultPublishHandler present (deadlock prevention)
- PingMQTT used by Plan 17 + Plan 05
- 4 tests pass against testcontainer Mosquitto
</verification>

<success_criteria>
- CHIRP-02 minimum (subscribe + log) implemented
- CHIRP-03 supporting infrastructure (PingMQTT) ready
- Re-subscribe pattern validated by reconnect test
- Phase 2 can swap the default handler for normalize+persist without touching `mqtt.go`
</success_criteria>

<output>
After completion, create `.planning/phases/01-foundation/01-13-SUMMARY.md` documenting:
- Public API
- Phase 2 hook point: `UplinkHandler` callback
- Re-subscribe semantics
- PingMQTT timeout policy
</output>
