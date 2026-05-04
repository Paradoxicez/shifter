package testharness_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/testharness"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// TestPublisher_RoundTrip — Mosquitto testcontainer + paho subscriber
// confirms the harness publisher actually delivers a message on the
// canonical CS v4 topic.
func TestPublisher_RoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}

	brokerURL := testsupport.StartMosquitto(t)

	// Subscriber: a paho client we drive directly, separate from the harness.
	subOpts := paho.NewClientOptions().
		AddBroker(brokerURL).
		SetClientID("th-test-sub").
		SetConnectTimeout(5 * time.Second)
	sub := paho.NewClient(subOpts)
	tk := sub.Connect()
	require.True(t, tk.WaitTimeout(5*time.Second), "sub connect timeout")
	require.NoError(t, tk.Error(), "sub connect")
	defer sub.Disconnect(250)

	var mu sync.Mutex
	var received [][]byte
	subTk := sub.Subscribe("application/+/device/+/event/up", 1, func(_ paho.Client, msg paho.Message) {
		mu.Lock()
		defer mu.Unlock()
		// Copy: paho buffer reuse semantics are not friendly to retaining slices.
		buf := make([]byte, len(msg.Payload()))
		copy(buf, msg.Payload())
		received = append(received, buf)
	})
	require.True(t, subTk.WaitTimeout(5*time.Second), "subscribe timeout")
	require.NoError(t, subTk.Error(), "subscribe")

	// Publisher: the harness API under test.
	pub, err := testharness.NewPublisher(brokerURL, "", "", "th-test-pub")
	require.NoError(t, err, "NewPublisher")
	defer pub.Close()

	event := []byte(`{"deviceInfo":{"devEui":"0102030405060708"},"fCnt":1,"fPort":1}`)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, pub.PublishUplink(ctx, "00000000-0000-0000-0000-000000000000", "0102030405060708", event))

	// Wait up to 2s for the subscriber to receive.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		got := len(received) > 0
		mu.Unlock()
		if got {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, received, "subscriber should have received at least one message within 2s")
	assert.JSONEq(t, string(event), string(received[0]))
}

// TestBuildAxiomaW1Uplink_DecodedObjectShape — B2 fix: the decoded `object`
// keys MUST match axioma_w1.js result.* exactly (cumulative_l, battery_pct,
// temperature_c, leak, tamper) per the pinned table in 02-13-PLAN.md.
func TestBuildAxiomaW1Uplink_DecodedObjectShape(t *testing.T) {
	t.Parallel()
	raw, err := testharness.BuildAxiomaW1Uplink(12345, 87, 23, false, false, 1, time.Now())
	require.NoError(t, err)

	var ev map[string]any
	require.NoError(t, json.Unmarshal(raw, &ev))

	obj, ok := ev["object"].(map[string]any)
	require.True(t, ok, "object must be a map")

	// Pinned keys per axioma_w1.js result.* (02-13-PLAN.md <codec_keys_pinned>):
	expectedKeys := []string{"cumulative_l", "battery_pct", "temperature_c", "leak", "tamper"}
	for _, k := range expectedKeys {
		_, present := obj[k]
		assert.True(t, present, "object missing pinned key %q", k)
	}

	// Verify cumulative_l value round-trips (JSON numbers decode as float64).
	cl, _ := obj["cumulative_l"].(float64)
	assert.Equal(t, 12345.0, cl)
	bp, _ := obj["battery_pct"].(float64)
	assert.Equal(t, 87.0, bp)
	tc, _ := obj["temperature_c"].(float64)
	assert.Equal(t, 23.0, tc)
	leak, _ := obj["leak"].(bool)
	assert.False(t, leak)
	tamper, _ := obj["tamper"].(bool)
	assert.False(t, tamper)

	// fPort=100 per axioma_w1.js dispatch guard.
	fp, _ := ev["fPort"].(float64)
	assert.Equal(t, 100.0, fp)

	// data field is base64-encoded raw bytes (decorative — CS would have
	// run the codec; here ingest only consumes object).
	d, _ := ev["data"].(string)
	assert.NotEmpty(t, d, "data field should be present (base64)")
}

// TestBuildAcrelADW300Uplink_DecodedObjectShape — analogous; pins all 13
// ADW300 superset keys per the pinned table.
func TestBuildAcrelADW300Uplink_DecodedObjectShape(t *testing.T) {
	t.Parallel()
	raw, err := testharness.BuildAcrelADW300Uplink(
		1234.56,    // kwhForward
		2400.0,     // powerW
		230.1, 230.2, 230.3, // voltL1..3
		5.1, 5.2, 5.3, // currL1..3
		0.95, 0.96, 0.97, // pfL1..3
		88,        // batteryPct
		25,        // tempC
		7,         // fCnt
		time.Now(),
	)
	require.NoError(t, err)

	var ev map[string]any
	require.NoError(t, json.Unmarshal(raw, &ev))

	obj, ok := ev["object"].(map[string]any)
	require.True(t, ok, "object must be a map")

	// Pinned 13-key set per acrel_family.js result.* (02-13-PLAN.md):
	expectedKeys := []string{
		"kwh_forward", "power_total_w",
		"voltage_l1", "current_l1", "pf_l1",
		"voltage_l2", "current_l2", "pf_l2",
		"voltage_l3", "current_l3", "pf_l3",
		"battery_pct", "temperature_c",
	}
	for _, k := range expectedKeys {
		_, present := obj[k]
		assert.True(t, present, "object missing pinned key %q", k)
	}

	// Spot-check round-trip values (JSON numbers → float64).
	kwh, _ := obj["kwh_forward"].(float64)
	assert.InDelta(t, 1234.56, kwh, 1e-6)
	pwr, _ := obj["power_total_w"].(float64)
	assert.InDelta(t, 2400.0, pwr, 1e-6)
	v1, _ := obj["voltage_l1"].(float64)
	assert.InDelta(t, 230.1, v1, 1e-6)
	bat, _ := obj["battery_pct"].(float64)
	assert.Equal(t, 88.0, bat)
}
