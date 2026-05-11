package events

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Measurement is the D-02 7-field struct decoded from pg_notify payloads on
// the `measurement_inserted` channel. JSON field names are verbatim from the
// trigger body (migration 0021) and MUST NOT be renamed without updating both.
type Measurement struct {
	MeteringPointID uuid.UUID `json:"metering_point_id"`
	Time            time.Time `json:"time"`
	CumulativeValue *float64  `json:"cumulative_value"` // nullable in DB
	InstantValue    *float64  `json:"instant_value"`    // nullable in DB
	Quality         string    `json:"quality"`
	BatteryPct      *int16    `json:"battery_pct"` // nullable in DB
	RSSI            *int16    `json:"rssi"`         // nullable in DB
}

// subscriber is the internal per-connection state kept by the Hub.
type subscriber struct {
	id      string
	topics  map[string]struct{}
	ch      chan []byte
	dropped uint64
}

// Hub is the in-process fan-out registry for measurement_inserted events.
// Methods are goroutine-safe. The Hub decodes each NOTIFY payload exactly once
// and dispatches the raw bytes (not re-encoded) to all matching subscriber
// channels so downstream SSE handlers can forward them without re-work.
type Hub struct {
	mu   sync.RWMutex
	subs map[string]*subscriber
	log  *slog.Logger
}

// NewHub constructs a Hub backed by the provided logger. log may be nil, in
// which case slog.Default() is used.
func NewHub(log *slog.Logger) *Hub {
	if log == nil {
		log = slog.Default()
	}
	return &Hub{
		subs: make(map[string]*subscriber),
		log:  log,
	}
}

// Subscribe registers connID as a subscriber interested in the given topics.
// It returns a receive-only channel on which raw NOTIFY payloads ([]byte) are
// delivered. Channel capacity is 32 (PITFALL §9: non-blocking sends drop on
// overflow rather than blocking the listener goroutine).
//
// If connID is already subscribed, the existing channel is returned unchanged
// (call UpdateTopics to replace the topic set).
func (h *Hub) Subscribe(connID string, topics []string) <-chan []byte {
	h.mu.Lock()
	defer h.mu.Unlock()

	if sub, ok := h.subs[connID]; ok {
		return sub.ch
	}
	sub := &subscriber{
		id:     connID,
		topics: topicSet(topics),
		ch:     make(chan []byte, 32),
	}
	h.subs[connID] = sub
	return sub.ch
}

// Unsubscribe removes connID from the registry and closes its channel. After
// this call the channel returned by Subscribe will be drained and then receive
// the zero value (nil), signaling EOF to the SSE writer.
func (h *Hub) Unsubscribe(connID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	sub, ok := h.subs[connID]
	if !ok {
		return
	}
	delete(h.subs, connID)
	close(sub.ch)
}

// UpdateTopics atomically replaces the topic set for connID. If connID is not
// subscribed, UpdateTopics is a no-op (Subscribe first).
func (h *Hub) UpdateTopics(connID string, topics []string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	sub, ok := h.subs[connID]
	if !ok {
		return
	}
	sub.topics = topicSet(topics)
}

// dispatch is called by the listener with the raw NOTIFY payload bytes. It
// decodes the payload to extract metering_point_id, computes the full topic
// set (dashboard:global, mp:<uuid>, mp:<uuid>:uplinks), and non-blocking-sends
// the raw payload bytes to every subscriber whose topic set intersects.
//
// Malformed JSON is logged at WARN and the payload is silently dropped
// (T-04-02-02 mitigation — bad JSON must never crash the listener).
func (h *Hub) dispatch(payload []byte) {
	// Decode minimally: only extract metering_point_id for topic routing.
	// We decode into the full Measurement struct so json.Unmarshal validates
	// the payload shape even though we only use the MeteringPointID for
	// routing. Errors are logged and dropped (T-04-02-02).
	var m Measurement
	if err := json.Unmarshal(payload, &m); err != nil {
		h.log.Warn("events: malformed NOTIFY payload — dropping", "err", err)
		return
	}

	mpID := m.MeteringPointID.String()

	// Compute the canonical topic set for this measurement.
	notifyTopics := map[string]struct{}{
		"dashboard:global":                struct{}{},
		fmt.Sprintf("mp:%s", mpID):        struct{}{},
		fmt.Sprintf("mp:%s:uplinks", mpID): struct{}{},
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, sub := range h.subs {
		if !intersects(sub.topics, notifyTopics) {
			continue
		}
		select {
		case sub.ch <- payload:
		default:
			sub.dropped++
			if sub.dropped%10 == 0 {
				h.log.Warn("events: dropping payload to slow subscriber",
					"conn_id", sub.id, "dropped", sub.dropped)
			}
		}
	}
}

// topicSet converts a slice of topic strings into a lookup map.
func topicSet(topics []string) map[string]struct{} {
	m := make(map[string]struct{}, len(topics))
	for _, t := range topics {
		m[t] = struct{}{}
	}
	return m
}

// intersects reports whether maps a and b share at least one key.
func intersects(a, b map[string]struct{}) bool {
	for k := range a {
		if _, ok := b[k]; ok {
			return true
		}
	}
	return false
}
