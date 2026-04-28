// Package http — chi middleware adapters.
//
// SlogLogger is the structured-log replacement for chi/middleware.Logger so
// every HTTP request emits a single JSON line via slog (D-24). The adapter
// honors PITFALL #4's anchor: it sits AFTER chi.middleware.RequestID so the
// emitted request_id field is the same identifier used in downstream handlers
// and propagated to upstream proxies (Caddy → Plan 22).
//
// What's intentionally NOT logged:
//   - request headers (Authorization, Cookie would leak; T-18-06 mitigation).
//   - request body (uploads / passwords; PII surface).
//
// What IS logged: method, path, status, bytes, ms, request_id.
package http

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// SlogLogger emits one structured log line per request via slog. Use as a
// chi middleware: `r.Use(SlogLogger(deps.Log))`.
//
// Wraps the response with chi's WrapResponseWriter so we can capture the
// status code + bytes-written without re-implementing the response recorder.
func SlogLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t0 := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			log.Info("http",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"ms", time.Since(t0).Milliseconds(),
				"request_id", middleware.GetReqID(r.Context()),
			)
		})
	}
}
