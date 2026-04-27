package http

import "testing"

// TestSPA_FallbackIndex — Unknown routes (e.g. /sites/abc, /devices/xyz)
// fall back to the embedded index.html so client-side react-router can take
// over. Returns 200 with the SPA HTML body.
// Implementation: Plan 19 (spa-embed).
func TestSPA_FallbackIndex(t *testing.T) {
	t.Skip("Plan 19: SPA index fallback pending")
}

// TestSPA_NoFallbackForAPI — Requests under /api/* never fall back to
// index.html; an unknown /api/foo returns 404 JSON, not the SPA HTML.
// Implementation: Plan 19 (spa-embed).
func TestSPA_NoFallbackForAPI(t *testing.T) {
	t.Skip("Plan 19: /api/* no-SPA-fallback pending")
}

// TestSPA_AssetCacheHeaders — Hashed asset filenames under /assets/ get
// long-lived Cache-Control (immutable, 1y); the index.html itself is
// no-cache so deploys flip cleanly.
// Implementation: Plan 19 (spa-embed).
func TestSPA_AssetCacheHeaders(t *testing.T) {
	t.Skip("Plan 19: SPA cache-control pending")
}
