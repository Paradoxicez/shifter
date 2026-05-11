package events

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// payload builds a minimal valid D-02 JSON payload for the given metering
// point UUID. Used by hub tests that need a dispatch-able payload.
func payloadFor(t *testing.T, mpID uuid.UUID) []byte {
	t.Helper()
	m := Measurement{
		MeteringPointID: mpID,
		Time:            time.Now().UTC(),
		Quality:         "ok",
	}
	b, err := json.Marshal(m)
	require.NoError(t, err)
	return b
}

// TestHub_TopicFiltering verifies that:
//   - A subscriber on "dashboard:global" receives every uplink.
//   - A subscriber on "mp:<uuid-A>" receives uplinks for mp-A but NOT mp-B.
func TestHub_TopicFiltering(t *testing.T) {
	h := NewHub(nil)

	mpA := uuid.New()
	mpB := uuid.New()

	// Sub1 and Sub2 both subscribe to dashboard:global (they should get all uplinks).
	ch1 := h.Subscribe("conn-1", []string{"dashboard:global"})
	ch2 := h.Subscribe("conn-2", []string{"dashboard:global"})

	// Sub3 subscribes only to mp-A (should get mp-A uplinks but NOT mp-B).
	ch3 := h.Subscribe("conn-3", []string{"mp:" + mpA.String()})

	payloadA := payloadFor(t, mpA)
	payloadB := payloadFor(t, mpB)

	// Dispatch an uplink for mp-A.
	h.dispatch(payloadA)

	// All three subscribers should receive the mp-A uplink.
	require.Len(t, ch1, 1, "conn-1 (dashboard:global) must receive mp-A uplink")
	require.Len(t, ch2, 1, "conn-2 (dashboard:global) must receive mp-A uplink")
	require.Len(t, ch3, 1, "conn-3 (mp:A) must receive its own uplink")

	// Drain channels.
	<-ch1
	<-ch2
	<-ch3

	// Dispatch an uplink for mp-B.
	h.dispatch(payloadB)

	// Sub1 and Sub2 (global) must receive; Sub3 (mp-A only) must NOT.
	require.Len(t, ch1, 1, "conn-1 (dashboard:global) must receive mp-B uplink")
	require.Len(t, ch2, 1, "conn-2 (dashboard:global) must receive mp-B uplink")
	assert.Len(t, ch3, 0, "conn-3 (mp:A only) must NOT receive mp-B uplink")
}

// TestHub_UplinksTopic verifies that a subscriber on "mp:<uuid>:uplinks"
// also receives uplinks for that metering point.
func TestHub_UplinksTopic(t *testing.T) {
	h := NewHub(nil)
	mpA := uuid.New()

	ch := h.Subscribe("conn-uplinks", []string{"mp:" + mpA.String() + ":uplinks"})

	h.dispatch(payloadFor(t, mpA))

	require.Len(t, ch, 1, "uplinks-topic subscriber must receive uplink for its mp")
}

// TestHub_SlowSubscriberIsolation verifies that a slow subscriber (one that
// does NOT read from its channel) does NOT block fast subscribers from
// receiving subsequent broadcasts within 100ms.
func TestHub_SlowSubscriberIsolation(t *testing.T) {
	h := NewHub(nil)
	mp := uuid.New()

	chFast1 := h.Subscribe("fast-1", []string{"dashboard:global"})
	chFast2 := h.Subscribe("fast-2", []string{"dashboard:global"})
	// slow — never read from.
	_ = h.Subscribe("slow", []string{"dashboard:global"})

	payload := payloadFor(t, mp)

	// Flood the slow subscriber's channel (capacity 32) so it overflows.
	for i := 0; i < 33; i++ {
		h.dispatch(payload)
	}

	// Fast subscribers should have received all 33 dispatches without blocking.
	assert.Len(t, chFast1, 32, "fast-1 channel should be at capacity (32) — all slots filled")
	assert.Len(t, chFast2, 32, "fast-2 channel should be at capacity (32) — all slots filled")

	// Now send one more message; fast subscribers receive it within 100ms.
	done := make(chan struct{})
	go func() {
		h.dispatch(payload)
		close(done)
	}()
	select {
	case <-done:
		// OK: dispatch returned without blocking.
	case <-time.After(100 * time.Millisecond):
		t.Fatal("dispatch blocked for >100ms — slow subscriber is blocking the hub")
	}
}

// TestHub_Unsubscribe verifies that unsubscribing closes the channel and
// removes the subscriber from future dispatches.
func TestHub_Unsubscribe(t *testing.T) {
	h := NewHub(nil)
	mp := uuid.New()

	ch := h.Subscribe("conn", []string{"dashboard:global"})
	h.Unsubscribe("conn")

	// Channel must be closed.
	_, open := <-ch
	assert.False(t, open, "channel must be closed after Unsubscribe")

	// Dispatch after unsubscribe must not panic.
	assert.NotPanics(t, func() { h.dispatch(payloadFor(t, mp)) })
}

// TestHub_UpdateTopics verifies that UpdateTopics atomically replaces
// the subscriber's topic set.
func TestHub_UpdateTopics(t *testing.T) {
	h := NewHub(nil)
	mpA := uuid.New()
	mpB := uuid.New()

	// Start subscribed to mp-A only.
	ch := h.Subscribe("conn", []string{"mp:" + mpA.String()})

	// Dispatch for mp-B — should not receive (not subscribed yet).
	h.dispatch(payloadFor(t, mpB))
	assert.Len(t, ch, 0, "before UpdateTopics: mp-B should not be received")

	// Switch to mp-B.
	h.UpdateTopics("conn", []string{"mp:" + mpB.String()})

	// Now dispatch for mp-B — should receive.
	h.dispatch(payloadFor(t, mpB))
	assert.Len(t, ch, 1, "after UpdateTopics: mp-B should be received")

	// But mp-A dispatches are no longer received.
	<-ch
	h.dispatch(payloadFor(t, mpA))
	assert.Len(t, ch, 0, "after UpdateTopics: mp-A should no longer be received")
}

// TestHub_MalformedPayload verifies that malformed JSON is dropped without
// panicking or affecting other subscribers (T-04-02-02).
func TestHub_MalformedPayload(t *testing.T) {
	h := NewHub(nil)
	mp := uuid.New()

	ch := h.Subscribe("conn", []string{"dashboard:global"})

	// Dispatch garbage.
	assert.NotPanics(t, func() { h.dispatch([]byte("not-json")) })
	assert.Len(t, ch, 0, "malformed payload must be dropped")

	// Normal payload must still work after the bad one.
	h.dispatch(payloadFor(t, mp))
	assert.Len(t, ch, 1, "good payload after bad must still be delivered")
}
