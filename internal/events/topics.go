// Package events — topics.go declares the canonical topic constants
// referenced across the codebase.
//
// Plan 06-04 introduces the `alert` topic for the in-app alert center: the
// alert workers publish raw alert JSON onto the topic when they fire/clear,
// and the alert drawer subscribes via the SSE handler so unread state
// updates without a 30-second polling lag.
//
// Topic is a typed string so callers reading topic literals get
// compile-time hints (no string-typing pitfalls) and the topic regex in
// handler.go can extend without refactoring the call sites.
package events

// Topic is the typed identifier for an SSE subscription channel. Topic
// values match the regex in handler.go's parseTopics; new well-known
// topics added here MUST also be permitted by the regex.
type Topic string

// AlertTopic is the canonical channel the alert workers publish onto when
// raising or clearing alerts. The SSE handler exposes it via
// ?topic=alert (singleton — no per-rule or per-target subtopics in v1).
const AlertTopic Topic = "alert"

// String returns the underlying topic string. Convenience for places that
// expect a raw string parameter (e.g. parseTopics input).
func (t Topic) String() string { return string(t) }
