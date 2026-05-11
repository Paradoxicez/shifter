package events

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"log/slog"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sseFrame captures a parsed SSE frame from the stream.
type sseFrame struct {
	event string
	data  string
	// comment holds bare comment lines like ": heartbeat"
	comment string
}

// parseSSEFrames parses a multi-frame SSE body string into structured frames.
// Each frame ends with a blank line. Lines prefixed with "event: " set the
// event type; "data: " sets the data; ": " (colon-space) sets the comment.
func parseSSEFrames(body string) []sseFrame {
	var frames []sseFrame
	var current sseFrame
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "event: "):
			current.event = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			current.data = strings.TrimPrefix(line, "data: ")
		case strings.HasPrefix(line, ": "):
			current.comment = strings.TrimPrefix(line, ": ")
		case line == "":
			frames = append(frames, current)
			current = sseFrame{}
		}
	}
	return frames
}

// newTestDeps constructs a Deps suitable for unit tests with a real Hub.
func newTestDeps() Deps {
	h := NewHub(slog.Default())
	return Deps{
		Hub:    h,
		Logger: slog.Default(),
	}
}

// Test 2: SSE headers are set correctly.
func TestSSEHandler_Headers(t *testing.T) {
	d := newTestDeps()

	req := httptest.NewRequest(http.MethodGet, "/api/events?topics=dashboard:global", nil)
	// Use a cancellable context so the handler returns quickly.
	ctx, cancel := context.WithTimeout(req.Context(), 200*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	d.handleSSE(w, req)

	res := w.Result()
	assert.Equal(t, "text/event-stream", res.Header.Get("Content-Type"))
	assert.Equal(t, "no-cache", res.Header.Get("Cache-Control"))
	assert.Equal(t, "no", res.Header.Get("X-Accel-Buffering"))
}

// Test 3: Snapshot-ready marker emitted immediately on connect.
func TestSSEHandler_SnapshotOnConnect(t *testing.T) {
	d := newTestDeps()

	req := httptest.NewRequest(http.MethodGet, "/api/events?topics=dashboard:global", nil)
	ctx, cancel := context.WithTimeout(req.Context(), 200*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	d.handleSSE(w, req)

	frames := parseSSEFrames(w.Body.String())
	require.NotEmpty(t, frames, "expected at least one SSE frame")
	first := frames[0]
	assert.Equal(t, "snapshot", first.event, "first frame must be event: snapshot")
	assert.Contains(t, first.data, `"ready":true`, "snapshot data must have ready:true")
	assert.Contains(t, first.data, `"topics"`, "snapshot data must have topics key")
	assert.Contains(t, first.data, "dashboard:global", "snapshot data must include subscribed topic")
	// conn_id must NOT appear in the wire payload (plan §wire-format)
	assert.NotContains(t, first.data, "conn_id", "conn_id must not appear in snapshot payload")
}

// Test 4: Heartbeat is emitted after the configured interval.
func TestSSEHandler_Heartbeat(t *testing.T) {
	orig := HeartbeatInterval
	HeartbeatInterval = 50 * time.Millisecond
	defer func() { HeartbeatInterval = orig }()

	d := newTestDeps()

	req := httptest.NewRequest(http.MethodGet, "/api/events?topics=dashboard:global", nil)
	// allow enough time for at least one heartbeat
	ctx, cancel := context.WithTimeout(req.Context(), 300*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	d.handleSSE(w, req)

	body := w.Body.String()
	assert.Contains(t, body, ": heartbeat", "heartbeat comment must appear in SSE stream")
}

// Test 5: Measurement event forwarded when Hub broadcasts.
func TestSSEHandler_MeasurementForwarded(t *testing.T) {
	d := newTestDeps()

	req := httptest.NewRequest(http.MethodGet, "/api/events?topics=dashboard:global", nil)

	// Run the handler in the background; cancel after injecting one measurement.
	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		d.handleSSE(w, req)
	}()

	// Wait briefly for Subscribe to register, then dispatch a fake measurement.
	time.Sleep(30 * time.Millisecond)
	payload := []byte(`{"metering_point_id":"00000000-0000-0000-0000-000000000001","time":"2026-05-11T12:34:56.789Z","cumulative_value":1234.56,"instant_value":4.2,"quality":"ok","battery_pct":87,"rssi":-65}`)
	d.Hub.dispatch(payload)

	// Give the handler time to write the frame, then disconnect.
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	body := w.Body.String()
	assert.Contains(t, body, "event: measurement", "measurement event must appear after dispatch")
	assert.Contains(t, body, `"cumulative_value":1234.56`, "measurement data must contain the payload")
}

// Test 6: Client cancellation causes handler to return and unsubscribe from Hub.
func TestSSEHandler_ClientDisconnect(t *testing.T) {
	d := newTestDeps()

	req := httptest.NewRequest(http.MethodGet, "/api/events?topics=dashboard:global", nil)
	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		d.handleSSE(w, req)
	}()

	// Wait for subscribe to register.
	time.Sleep(30 * time.Millisecond)

	// Confirm a subscriber is registered in the Hub.
	d.Hub.mu.RLock()
	subCount := len(d.Hub.subs)
	d.Hub.mu.RUnlock()
	assert.Equal(t, 1, subCount, "one subscriber should be registered")

	// Cancel context (simulate client disconnect).
	cancel()

	// Handler should exit within 100ms.
	select {
	case <-done:
		// Good — handler returned.
	case <-time.After(100 * time.Millisecond):
		t.Fatal("handler did not return within 100ms after context cancellation")
	}

	// Hub should have no subscribers after teardown.
	d.Hub.mu.RLock()
	subCountAfter := len(d.Hub.subs)
	d.Hub.mu.RUnlock()
	assert.Equal(t, 0, subCountAfter, "Hub should have no subscribers after disconnect")
}

// Test 7: Invalid topic returns 400 before any SSE headers.
func TestSSEHandler_InvalidTopic_Returns400(t *testing.T) {
	d := newTestDeps()

	req := httptest.NewRequest(http.MethodGet, "/api/events?topics=invalid-topic", nil)
	w := httptest.NewRecorder()
	d.handleSSE(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "invalid topic must return 400")
	// Content-Type must NOT be text/event-stream if we returned 400 before SSE headers.
	assert.NotEqual(t, "text/event-stream", w.Header().Get("Content-Type"))
}

// Test 8: Too many topics (>64) returns 400.
func TestSSEHandler_TooManyTopics_Returns400(t *testing.T) {
	d := newTestDeps()

	// Build 65 valid dashboard:global topics (they're all the same, but parse-level
	// count happens before dedup so 65 strings > maxTopicsPerConnection).
	topics := make([]string, 65)
	for i := range topics {
		topics[i] = "dashboard:global"
	}
	query := "?topics=" + strings.Join(topics, ",")

	req := httptest.NewRequest(http.MethodGet, "/api/events"+query, nil)
	w := httptest.NewRecorder()
	d.handleSSE(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "too many topics must return 400")
}

// Test 7b: Missing topics query param returns 400.
func TestSSEHandler_MissingTopics_Returns400(t *testing.T) {
	d := newTestDeps()

	req := httptest.NewRequest(http.MethodGet, "/api/events", nil)
	w := httptest.NewRecorder()
	d.handleSSE(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "missing topics must return 400")
}

// --- parseTopics unit tests ---

func TestParseTopics_Valid(t *testing.T) {
	tests := []struct {
		raw  string
		want []string
	}{
		{
			raw:  "dashboard:global",
			want: []string{"dashboard:global"},
		},
		{
			raw:  "mp:12345678-1234-1234-1234-123456789abc",
			want: []string{"mp:12345678-1234-1234-1234-123456789abc"},
		},
		{
			raw:  "mp:12345678-1234-1234-1234-123456789abc:uplinks",
			want: []string{"mp:12345678-1234-1234-1234-123456789abc:uplinks"},
		},
		{
			raw:  "dashboard:global,mp:12345678-1234-1234-1234-123456789abc",
			want: []string{"dashboard:global", "mp:12345678-1234-1234-1234-123456789abc"},
		},
	}
	for _, tt := range tests {
		got, err := parseTopics(tt.raw)
		require.NoError(t, err, "raw=%q", tt.raw)
		assert.Equal(t, tt.want, got, "raw=%q", tt.raw)
	}
}

func TestParseTopics_Invalid(t *testing.T) {
	invalid := []string{
		"",
		"invalid",
		"bad:topic",
		"mp:short",
		"dashboard:other",
		"mp:UPPERCASE-1234-1234-1234-123456789abc",
	}
	for _, raw := range invalid {
		_, err := parseTopics(raw)
		assert.Error(t, err, "expected error for raw=%q", raw)
	}
}

func TestParseTopics_TooMany(t *testing.T) {
	topics := make([]string, 65)
	for i := range topics {
		topics[i] = "dashboard:global"
	}
	_, err := parseTopics(strings.Join(topics, ","))
	assert.Error(t, err, "65 topics should exceed the cap")
}

// Test that handleSSE does not export handleSubscribe or handleUnsubscribe
// (this is a compile-time check — if those methods existed on Deps, they'd
// need to be removed per the plan's acceptance criteria).
// The grep-based acceptance in the plan catches this; no runtime test needed.
