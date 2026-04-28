// Package http — SPA fallback handler placeholder.
//
// Plan 19 (spa-embed) replaces SPAHandler's body with a real go:embed-backed
// SPA serving Vite's `dist/` output (index.html fallback + hashed-asset cache
// headers). Plan 18 needs the symbol to exist so serve.go can wire the chi
// router's catch-all today; the placeholder returns 503 so Docker / Caddy
// upstream probes see a clear "service starting" signal rather than HTML
// surface from a half-wired install.
package http

import "net/http"

// SPAHandler is the SPA fallback Plan 19 fills in. Today: 503 placeholder.
//
// The router's `/*` catch-all is registered LAST (PITFALL #4) so this only
// fires when no /api/* / /health / /install route matched. Plan 19 will
// extend it to:
//   - serve `index.html` for unknown paths that don't look like assets
//   - serve hashed assets under /assets/ with long-lived Cache-Control
//   - return 404 for missing static asset filenames
func SPAHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "shifter SPA not yet embedded — Plan 19", http.StatusServiceUnavailable)
	})
}
