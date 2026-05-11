// Package events — SSE handler for the GET /api/events endpoint.
//
// Wire format (per plan §interfaces):
//
//	HTTP/1.1 200 OK
//	Content-Type: text/event-stream
//	Cache-Control: no-cache
//	Connection: keep-alive
//	X-Accel-Buffering: no
//
//	event: snapshot
//	data: {"ready":true,"topics":["dashboard:global","mp:abc-123"]}
//
//	: heartbeat
//
//	event: measurement
//	data: {"metering_point_id":"abc-123",...}
//
//	: heartbeat
//
// Each frame ends with \n\n. Heartbeat is a comment ": heartbeat\n\n" every
// HeartbeatInterval. The snapshot marker (D-03) tells the client "you are
// live now; go fetch your REST snapshot." The actual snapshot payload is
// returned by the dashboard/snapshot and metering-points/:id/detail endpoints
// (Plans 04 and 05).
//
// T-04-03-01: Route MUST be mounted under the authenticated group so anonymous
// requests receive 401 from the session middleware before reaching this handler.
//
// T-04-03-04: Topic regex is anchored + bounded; topic count capped at 64.
// Requests that exceed the cap or contain invalid topics receive 400 BEFORE
// any SSE headers are written.
//
// D-23: no admin-only guard here — both admin and viewer roles may subscribe.
package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// HeartbeatInterval controls how often the SSE handler emits a keepalive
// comment. It is a package-level variable (not a const) so tests can override
// it to a short duration without waiting 30 seconds.
var HeartbeatInterval = 30 * time.Second

const (
	// maxTopicsPerConnection is the planner cap (T-04-03-04). Requests with
	// more than this many topics receive 400 before any SSE headers are written.
	maxTopicsPerConnection = 64
)

// topicRegex matches the three canonical topic shapes defined in Plan 04-02:
//
//	dashboard:global           — every uplink
//	mp:<uuid>                  — per metering-point deltas
//	mp:<uuid>:uplinks          — per metering-point, Uplinks log tab
//
// The regex is anchored (^ and $) and the UUID portion is bounded to
// [0-9a-f-]{36} so an attacker cannot craft a topic that triggers catastrophic
// backtracking (T-04-03-04).
var topicRegex = regexp.MustCompile(`^(dashboard:global|mp:[0-9a-f-]{36}(:uplinks)?)$`)

// Deps groups the dependencies the SSE handler needs. It is constructed in
// cmd/shifter/serve.go (Task 3) and registered via RegisterRoutes (Task 2).
type Deps struct {
	Hub    *Hub
	Logger *slog.Logger
}

// snapshotPayload is the wire shape for the snapshot-ready marker (D-03).
// Only two fields: ready (always true) and topics (the subscribed set).
// conn_id is deliberately absent from the wire format (plan §wire-format).
type snapshotPayload struct {
	Ready  bool     `json:"ready"`
	Topics []string `json:"topics"`
}

// handleSSE is the GET /api/events SSE handler. It:
//
//  1. Validates the ?topics= query parameter (400 on error, before any headers).
//  2. Checks streaming support (500 on unsupported — extremely rare in Go).
//  3. Writes SSE headers.
//  4. Subscribes to Hub with an internal conn_id (uuid, NOT emitted on wire).
//  5. Emits event: snapshot immediately.
//  6. Loops: forwards Hub payloads as event: measurement, ticks heartbeats.
//  7. On ctx.Done() (client disconnect) — defer Unsubscribe — goroutine exits.
func (d Deps) handleSSE(w http.ResponseWriter, r *http.Request) {
	topics, err := parseTopics(r.URL.Query().Get("topics"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Write SSE headers BEFORE the first Flush call so they reach the browser
	// in the initial response. WriteHeader must come after Header().Set calls.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	// connID is an internal correlation key — NOT emitted on the wire.
	connID := uuid.NewString()
	ch := d.Hub.Subscribe(connID, topics)
	defer d.Hub.Unsubscribe(connID)

	// Snapshot-ready marker (D-03). Wire payload: {ready: bool, topics: []string}.
	snap := snapshotPayload{Ready: true, Topics: topics}
	snapJSON, _ := json.Marshal(snap)
	fmt.Fprintf(w, "event: snapshot\ndata: %s\n\n", snapJSON)
	flusher.Flush()

	heartbeat := time.NewTicker(HeartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case payload, open := <-ch:
			if !open {
				// Hub closed the channel (e.g., server shutdown path).
				return
			}
			fmt.Fprintf(w, "event: measurement\ndata: %s\n\n", payload)
			flusher.Flush()
		case <-heartbeat.C:
			fmt.Fprint(w, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}

// parseTopics parses the comma-separated ?topics= query parameter.
//
// Returns an error (HTTP 400) if:
//   - raw is empty (topics param required)
//   - more than maxTopicsPerConnection topics are requested (T-04-03-04)
//   - any topic does not match topicRegex (T-04-03-04)
func parseTopics(raw string) ([]string, error) {
	if raw == "" {
		return nil, errors.New("topics query param required")
	}
	parts := strings.Split(raw, ",")
	if len(parts) > maxTopicsPerConnection {
		return nil, fmt.Errorf("too many topics (max %d)", maxTopicsPerConnection)
	}
	for _, t := range parts {
		if !topicRegex.MatchString(t) {
			return nil, fmt.Errorf("invalid topic: %q", t)
		}
	}
	return parts, nil
}
