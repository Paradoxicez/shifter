package http

import "testing"

// TestHealth_Public — GET /health returns 200 with JSON
// {status, version, uptime_seconds} and does NOT require authentication.
// Implementation: Plan 18 (router-health).
func TestHealth_Public(t *testing.T) {
	t.Skip("Plan 18: public /health pending")
}

// TestHealthDetailed_RequiresAdmin — GET /health/detailed requires an
// admin session; it returns DB ping latency and ChirpStack reachability.
// Anonymous returns 401, viewer returns 403.
// Implementation: Plan 18 (router-health).
func TestHealthDetailed_RequiresAdmin(t *testing.T) {
	t.Skip("Plan 18: admin-gated /health/detailed pending")
}
