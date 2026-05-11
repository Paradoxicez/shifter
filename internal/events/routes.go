package events

import "github.com/go-chi/chi/v5"

// RegisterRoutes mounts the SSE endpoint under the chi router.
//
// Only one route is registered in Phase 4: GET /api/events. There are no
// subscribe/unsubscribe routes — topic-set changes are handled by the client
// reconnecting with a new ?topics=... query string (reconnect-instead-of-update
// per plan §design_notes, D-04 semantics).
//
// D-23: this MUST be called inside the authenticated group so both admin and
// viewer roles can subscribe. The auth gate is enforced at the router level
// (internal/http/router.go) via the EventsDeps nil-guard placement under the
// authenticated chi group.
func RegisterRoutes(r chi.Router, deps Deps) {
	r.Get("/api/events", deps.handleSSE)
}
