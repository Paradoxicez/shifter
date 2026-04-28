package chirpstack

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/url"
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
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", msg)
}

// tcpBreaker is a minimal in-process TCP proxy used by the reconnect test.
// It forwards every accepted connection to the upstream broker and tracks
// active conns so the test can `Break()` them to simulate a transport-level
// drop (which is what triggers paho's auto-reconnect — `client.Disconnect`
// is treated as user-initiated and does NOT auto-reconnect).
type tcpBreaker struct {
	listener net.Listener
	upstream string

	mu    sync.Mutex
	conns []net.Conn
}

// startBreaker accepts on 127.0.0.1:0 and forwards to upstreamURL
// (`tcp://host:port`). Cleanup is wired via t.Cleanup.
func startBreaker(t *testing.T, upstreamURL string) *tcpBreaker {
	t.Helper()
	u, err := url.Parse(upstreamURL)
	require.NoError(t, err)
	require.Equal(t, "tcp", u.Scheme)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	b := &tcpBreaker{listener: ln, upstream: u.Host}
	go b.serve()
	t.Cleanup(func() {
		_ = ln.Close()
		b.Break()
	})
	return b
}

func (b *tcpBreaker) URL() string {
	return "tcp://" + b.listener.Addr().String()
}

func (b *tcpBreaker) serve() {
	for {
		c, err := b.listener.Accept()
		if err != nil {
			return
		}
		go b.proxy(c)
	}
}

func (b *tcpBreaker) proxy(client net.Conn) {
	upstream, err := net.Dial("tcp", b.upstream)
	if err != nil {
		_ = client.Close()
		return
	}
	b.mu.Lock()
	b.conns = append(b.conns, client, upstream)
	b.mu.Unlock()

	go func() { _, _ = io.Copy(upstream, client); _ = upstream.Close() }()
	_, _ = io.Copy(client, upstream)
	_ = client.Close()
}

// Break forcibly closes every tracked conn. paho observes the broken pipe
// and triggers auto-reconnect (which then re-runs OnConnect through the
// breaker — newly accepted conns will be proxied normally).
func (b *tcpBreaker) Break() {
	b.mu.Lock()
	conns := b.conns
	b.conns = nil
	b.mu.Unlock()
	for _, c := range conns {
		_ = c.Close()
	}
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

// TestMQTT_ReconnectResubscribe — After the underlying TCP connection is
// dropped at the transport layer (network partition / broker restart /
// keepalive timeout), paho's auto-reconnect path fires OnConnect again and
// re-subscribes to the uplink topic without manual intervention.
//
// Note: paho treats `client.Disconnect(...)` as user-initiated and does NOT
// auto-reconnect after it. The realistic reconnect path is a transport-level
// loss, which we simulate via an in-process TCP proxy whose `Break()` closes
// every conn in flight.
func TestMQTT_ReconnectResubscribe(t *testing.T) {
	upstream := testsupport.StartMosquitto(t)
	breaker := startBreaker(t, upstream)

	sub, err := NewMQTTSubscriber(breaker.URL(), "", "", "shifter-test-2", newLogger(), nil)
	require.NoError(t, err)
	defer sub.Shutdown(2 * time.Second)
	waitFor(t, sub.IsSubscribed, "first subscribe")
	initial := sub.ResubscribeCount()
	require.GreaterOrEqual(t, initial, int64(1))

	// Force a transport-level drop. paho's auto-reconnect kicks in and the
	// next OnConnect must increment ResubscribeCount.
	breaker.Break()
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
