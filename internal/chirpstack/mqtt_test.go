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
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", msg)
}

// TestMQTT_UplinkLogged — A message published to
// `application/+/device/+/event/up` lands as a logged event in the subscriber
// (placeholder: real measurement persistence arrives in Phase 4).
func TestMQTT_UplinkLogged(t *testing.T) {
	broker := testsupport.StartMosquitto(t)

	var mu sync.Mutex
	got := 0
	sub, err := NewMQTTSubscriber(broker, "", "", "shifter-test-1", newLogger(), func(_ string, _ []byte) {
		mu.Lock()
		got++
		mu.Unlock()
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
		mu.Lock()
		defer mu.Unlock()
		return got >= 1
	}, "uplink delivered")
}

// TestMQTT_ReconnectResubscribe — After the broker connection is dropped, the
// subscriber re-establishes the connection and re-subscribes via OnConnect
// without manual intervention.
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

// TestPingMQTT_Reachable — Plan 17 Test Connection probe against a running
// Mosquitto returns nil within ctx timeout.
func TestPingMQTT_Reachable(t *testing.T) {
	broker := testsupport.StartMosquitto(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, PingMQTT(ctx, broker, "", ""))
}

// TestPingMQTT_Unreachable — Plan 17 Test Connection probe against a closed
// port returns error within ctx timeout.
func TestPingMQTT_Unreachable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := PingMQTT(ctx, "tcp://127.0.0.1:1", "", "")
	require.Error(t, err)
}
