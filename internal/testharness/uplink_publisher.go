package testharness

import (
	"context"
	"fmt"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
)

// Publisher publishes synthetic ChirpStack v4 uplink JSON events on the
// canonical `application/<appID>/device/<devEUI>/event/up` topic. Used by
// both the Go integration scenarios (against a Mosquitto testcontainer) and
// the `shifter test-harness` CLI subcommand (against a deployed broker).
//
// The connection is established eagerly in NewPublisher; reconnect logic is
// deliberately absent — scenarios are short-lived (≤ 30s) and a transient
// broker outage during a scenario should fail fast so the operator sees it.
type Publisher struct {
	c paho.Client
}

// NewPublisher connects to brokerURL (e.g. "tcp://localhost:1883") with
// optional username/password. Returns once CONNACK is received or fails
// after 5s.
//
// W3 (gap-closure revision): broker connect failures surface here as a
// clear error so the CLI subcommand can wrap them with the broker URL in
// its operator-facing message.
func NewPublisher(brokerURL, user, pass, clientID string) (*Publisher, error) {
	opts := paho.NewClientOptions().AddBroker(brokerURL).SetClientID(clientID)
	if user != "" {
		opts.SetUsername(user)
	}
	if pass != "" {
		opts.SetPassword(pass)
	}
	opts.SetConnectTimeout(5 * time.Second)
	c := paho.NewClient(opts)
	tk := c.Connect()
	if !tk.WaitTimeout(5 * time.Second) {
		return nil, fmt.Errorf("connect timeout")
	}
	if err := tk.Error(); err != nil {
		return nil, err
	}
	return &Publisher{c: c}, nil
}

// PublishUplink publishes the given event JSON on the canonical
// `application/<applicationID>/device/<devEUI>/event/up` topic at QoS=1.
// Blocks until the broker acknowledges OR ctx fires (whichever first).
func (p *Publisher) PublishUplink(ctx context.Context, applicationID, devEUI string, event []byte) error {
	topic := fmt.Sprintf("application/%s/device/%s/event/up", applicationID, devEUI)
	tk := p.c.Publish(topic, 1, false, event)
	done := make(chan error, 1)
	go func() {
		tk.Wait()
		done <- tk.Error()
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

// Close disconnects the underlying paho client. Safe to call multiple times.
func (p *Publisher) Close() {
	if p.c != nil && p.c.IsConnected() {
		p.c.Disconnect(250)
	}
}
