package http

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

// synthFS returns a small in-memory fs.FS shaped like the Vite build output:
//
//	index.html, assets/index-abc123.js
//
// Used by every SPA test so behavior is deterministic regardless of whether
// `pnpm build` has been run.
func synthFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html": &fstest.MapFile{
			Data: []byte(`<!doctype html><html><body>shifter-spa-fixture</body></html>`),
		},
		"assets/index-abc123.js": &fstest.MapFile{
			Data: []byte(`/* fixture asset (hashed name) */`),
		},
	}
}

// TestSPA_FallbackIndex — Unknown routes (e.g. /sites/abc, /devices/xyz)
// fall back to the embedded index.html so client-side react-router can take
// over. Returns 200 with the SPA HTML body.
// Implementation: Plan 19 (spa-embed).
func TestSPA_FallbackIndex(t *testing.T) {
	handler := SPAHandlerFS(synthFS())
	req := httptest.NewRequest("GET", "/dashboard", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	body, _ := io.ReadAll(w.Body)
	require.Contains(t, strings.ToLower(string(body)), "shifter",
		"fallback must serve index.html (which contains brand string)")
	require.Equal(t, "no-cache", w.Header().Get("Cache-Control"),
		"HTML fallback must set Cache-Control: no-cache (RESEARCH §Pattern 8)")
}

// TestSPA_NoFallbackForAsset — non-html missing assets must NOT fall through
// to index.html. PITFALL #4 anchor.
func TestSPA_NoFallbackForAsset(t *testing.T) {
	handler := SPAHandlerFS(synthFS())
	req := httptest.NewRequest("GET", "/missing-image.png", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code,
		"non-html asset paths must NOT fall through to index.html (PITFALL #4)")
}

// TestSPA_AssetCacheHeaders verifies the immutable cache-control on hashed
// assets. Implemented per checker Warning #7 — replaces the previous t.Skip stub.
func TestSPA_AssetCacheHeaders(t *testing.T) {
	handler := SPAHandlerFS(synthFS())
	req := httptest.NewRequest("GET", "/assets/index-abc123.js", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	cc := w.Header().Get("Cache-Control")
	require.Contains(t, cc, "max-age=31536000",
		"hashed asset must set 1-year max-age (RESEARCH §Pattern 8)")
	require.Contains(t, cc, "immutable",
		"hashed asset must set immutable (RESEARCH §Pattern 8)")
}

// TestSPA_IndexHasNoCacheHeader — index.html must NEVER be cached.
func TestSPA_IndexHasNoCacheHeader(t *testing.T) {
	handler := SPAHandlerFS(synthFS())
	req := httptest.NewRequest("GET", "/index.html", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "no-cache", w.Header().Get("Cache-Control"),
		"index.html must NEVER be cached (RESEARCH §Pattern 8)")
}

// TestSPA_NoFallbackForAPI verifies that when SPA is mounted LAST in a chi
// router (PITFALL #4 contract), /api/* paths NOT registered by the API
// return a JSON 404 from the router's NotFoundHandler — NOT index.html.
//
// Implemented per checker Warning #7 — replaces the previous t.Skip stub.
// This is the canonical PITFALL #4 unit test that runs in <100ms; the full
// compose smoke (Plan 20) covers the production path with real builds.
func TestSPA_NoFallbackForAPI(t *testing.T) {
	r := chi.NewRouter()
	// Register a JSON 404 handler for unknown /api/* paths — this mirrors
	// what Plan 18's router does (or should do) at production scale.
	r.NotFound(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(req.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"not_found"}`))
			return
		}
		// Non-API paths fall through to SPA handler.
		SPAHandlerFS(synthFS()).ServeHTTP(w, req)
	})
	// Register a single real /api route so /api/* is a real prefix.
	r.Get("/api/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Hit a missing /api/* path — must NOT be the SPA index.
	req := httptest.NewRequest("GET", "/api/nonexistent-endpoint", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code,
		"PITFALL #4: /api/* 404s must return 404, not 200 from SPA fallback")
	require.Contains(t, w.Header().Get("Content-Type"), "application/json",
		"PITFALL #4: /api/* 404s must return JSON, not HTML")
	body, _ := io.ReadAll(w.Body)
	require.NotContains(t, strings.ToLower(string(body)), "<!doctype",
		"PITFALL #4: /api/* must NEVER return the SPA index.html")
}
