package chirpstack

import "testing"

// TestMQTT_ReconnectResubscribe — After the broker is restarted (or the
// connection is dropped), the MQTT subscriber re-establishes the connection
// and re-subscribes to its uplink topic filter without manual intervention.
// Implementation: Plan 13 (mqtt-subscriber).
func TestMQTT_ReconnectResubscribe(t *testing.T) {
	t.Skip("Plan 13: MQTT reconnect+resubscribe pending")
}

// TestMQTT_UplinkLogged — A message published to
// `application/+/device/+/event/up` lands as a logged event in the subscriber
// (placeholder: real measurement persistence arrives in Phase 4).
// Implementation: Plan 13 (mqtt-subscriber).
func TestMQTT_UplinkLogged(t *testing.T) {
	t.Skip("Plan 13: uplink logging pending")
}
