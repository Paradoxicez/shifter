// Package events implements the LISTEN/NOTIFY substrate for Phase 4 SSE push.
//
// Architecture:
//
//   - listener.go — runs a dedicated pgx connection holding LISTEN
//     measurement_inserted for the process lifetime. Outer reconnect loop with
//     2s backoff defends against connection drops (Pitfall 12). The defer
//     conn.Release() inside runOnce returns the conn to the pool even if the
//     connection itself is dead — pgxpool replaces it on next Acquire.
//
//   - hub.go — in-process fan-out registry. Each SSE handler registers a
//     subscriber (connID → topic set → buffered channel). Hub.dispatch decodes
//     the NOTIFY payload from the listener and broadcasts to every subscriber
//     whose topic set intersects with the computed topics for that measurement.
//
// NOTIFY channel (from migration 0021):
//
//	measurement_inserted
//
// D-02 payload (7-field JSON, ≤200B per trigger body discipline):
//
//	{
//	    "metering_point_id": "<uuid>",
//	    "time":              "2026-05-11T13:00:00.000Z",
//	    "cumulative_value":  123.456,   // nullable
//	    "instant_value":     1.0,       // nullable
//	    "quality":           "ok",
//	    "battery_pct":       87,        // nullable
//	    "rssi":              -65        // nullable
//	}
//
// Topic conventions (fixed for Phase 4):
//
//	dashboard:global          — every uplink fans out here
//	mp:<uuid>                 — uplinks for the named metering point
//	mp:<uuid>:uplinks         — same MP, used by the Uplinks log tab
//
// For every incoming NOTIFY the listener publishes to ALL THREE topics;
// SSE handlers filter by their subscriber's declared topic set.
//
// Backpressure policy (PITFALL §9, T-04-02-01 mitigation):
//
// Hub.dispatch uses a non-blocking send per subscriber channel (capacity 32).
// If a subscriber's channel is full the payload is dropped and the subscriber's
// `dropped` counter is incremented. A WARN log is emitted every 10 drops so
// the operator can identify consistently slow SSE consumers without flooding
// the log on transient bursts.
package events
