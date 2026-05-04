package chirpstack

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// UplinkTopicFilter is the canonical ChirpStack v4 uplink topic at QoS 1.
// `application/+/device/+/event/up` matches every uplink across every
// application + device under the broker's namespace. Subscription is
// registered inside OnConnect so the filter survives reconnects.
const UplinkTopicFilter = "application/+/device/+/event/up"

// UplinkHandler is invoked once per matching MQTT message. Phase 1 default:
// log to stdout (CHIRP-02 minimum). Phase 2 wires the normalize+persist
// pipeline by passing a non-nil handler to NewMQTTSubscriber — mqtt.go does
// not need to change.
type UplinkHandler func(topic string, payload []byte)

// MQTTSubscriber wraps a paho.mqtt.golang client connected to the ChirpStack
// MQTT integration broker. The subscriber owns the client lifecycle; callers
// drive shutdown via Shutdown(timeout).
type MQTTSubscriber struct {
	client       mqtt.Client
	log          *slog.Logger
	handler      UplinkHandler
	subscribed   atomic.Bool
	resubscribes atomic.Int64
}

// NewMQTTSubscriber connects to the broker and subscribes to the canonical
// ChirpStack uplink topic. Re-subscription is registered in OnConnect so
// reconnect events do not leave the subscription unwound (RESEARCH §Pattern 10
// + paho.mqtt.golang/issues/22).
//
// `handler` may be nil — defaults to a stdout-logging handler (Phase 1
// CHIRP-02 minimum). Phase 2 will inject a normalize+persist handler.
//
// The mandatory DefaultPublishHandler is always installed: it logs unrouted
// messages and prevents the inflight-message-limit deadlock the upstream
// paho docs warn about.
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
		SetCleanSession(false). // resume QoS1 inflight on reconnect
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
	// Mandatory per RESEARCH §Pattern 10: prevents inflight-message-limit
	// deadlocks if a message arrives on a topic with no registered handler.
	opts.SetDefaultPublishHandler(func(_ mqtt.Client, m mqtt.Message) {
		log.Warn("mqtt unrouted message", "topic", m.Topic(), "len", len(m.Payload()))
	})

	sub.client = mqtt.NewClient(opts)
	if t := sub.client.Connect(); t.Wait() && t.Error() != nil {
		return nil, fmt.Errorf("mqtt connect: %w", t.Error())
	}
	return sub, nil
}

// routeUplink is the internal subscribe-callback. It strips the paho client
// argument and delegates to the configured UplinkHandler so tests can inject
// a callback without depending on the paho.Message interface.
func (s *MQTTSubscriber) routeUplink(_ mqtt.Client, m mqtt.Message) {
	s.handler(m.Topic(), m.Payload())
}

// Shutdown disconnects the client gracefully, draining inflight publishes up
// to `timeout`. Reusing the receiver after Shutdown is unsafe — create a new
// MQTTSubscriber if reconnect is needed.
func (s *MQTTSubscriber) Shutdown(timeout time.Duration) {
	s.client.Disconnect(uint(timeout.Milliseconds()))
	s.log.Info("mqtt subscriber stopped")
}

// SetUplinkHandler swaps the active uplink handler in place. Plan 02-09's
// `cmd/shifter serve` boot wiring calls this after constructing the ingest
// pipeline so we don't have to thread the pipeline through NewMQTTSubscriber's
// constructor. The handler is invoked from the paho receive goroutine —
// implementations MUST be safe to call concurrently with subscribe/disconnect
// events (the ingest pipeline opens its own context, so this is satisfied
// trivially today).
//
// Passing nil is rejected silently to avoid wedging the consumer at runtime
// (paho would panic on dispatch with a nil function value). The default
// stdout-logging handler installed at construction time stays in place.
func (s *MQTTSubscriber) SetUplinkHandler(h UplinkHandler) {
	if h == nil {
		return
	}
	s.handler = h
}

// IsSubscribed reports the latest subscribe-success state. True after
// OnConnect has registered the topic filter; flips back to false on
// connection-lost and back to true after the next reconnect+subscribe.
// Used by tests to await subscription readiness.
func (s *MQTTSubscriber) IsSubscribed() bool { return s.subscribed.Load() }

// ResubscribeCount returns the number of times OnConnect has successfully
// re-subscribed. 1 on first connect; 2+ after reconnects. Used by the
// reconnect test to assert the OnConnect path fired again after a forced
// disconnect.
func (s *MQTTSubscriber) ResubscribeCount() int64 { return s.resubscribes.Load() }

// PingMQTT performs a connect → disconnect cycle within ctx. Returns nil if
// the broker accepted the connection, otherwise the underlying paho error or
// ctx.Err(). Used by Plan 17 Test Connection's MQTT channel and Plan 05's
// `shifter config-check` connectivity probe.
//
// Auto-reconnect is disabled so a misconfigured broker URL fails fast rather
// than spinning under SetConnectRetry. Clean session is enabled so the probe
// leaves no broker-side state.
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
			done <- fmt.Errorf("mqtt ping: connect timeout")
			return
		}
		if t.Error() != nil {
			done <- fmt.Errorf("mqtt ping: %w", t.Error())
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
