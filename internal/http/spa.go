// Package http — SPA embed handler.
//
// Plan 19 (spa-embed): serves Vite's dist/ output bundled into the binary by
// the top-level `web` package's go:embed directive (web/embed.go). The chi
// router's `/*` catch-all (Plan 18) targets SPAHandler() so this only fires
// when no /api/* / /health / /install route matched (PITFALL #4).
//
// Behavior (RESEARCH §Pattern 8):
//   - GET /index.html or /any/spa/route (no extension or .html) → index.html
//     with Cache-Control: no-cache so deploys flip cleanly.
//   - GET /assets/<hashed-name>.{js,css,woff2,...} → file with
//     Cache-Control: public, max-age=31536000, immutable.
//   - GET /missing-image.png (non-html extension, file absent) → 404 — must
//     NOT fall through to index.html (PITFALL #4 anchor).
//
// Build prerequisite: `pnpm build` must run before `go build` for production
// so web/dist/ contains real artifacts. A web/dist/.gitkeep placeholder lets
// the embed directive compile in CI before the frontend build runs.
//
// Embed location note: the actual `//go:embed all:dist` directive lives in
// web/embed.go because go:embed patterns cannot reference paths outside the
// source file's directory tree (no `../` traversal). This package consumes
// the resulting embed.FS via fs.Sub.
package http

import (
	"bytes"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"

	web "github.com/shifter-io/shifter/web"
)

// SPAHandler is the production entry point. It serves the embedded Vite build
// with history-mode fallback. Mount LAST per PITFALL #4.
func SPAHandler() http.Handler {
	sub, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		// fs.Sub only fails if the prefix path is invalid — a compile-time
		// invariant given the literal "dist" matches the //go:embed directive
		// in web/embed.go. Panic surfaces a regression in directive vs prefix.
		panic(err)
	}
	return SPAHandlerFS(sub)
}

// SPAHandlerFS is the testable form: returns a handler over any fs.FS rooted
// at the SPA build output (i.e. the dir containing index.html and assets/).
// Tests inject a synthetic testing/fstest.MapFS so SPA behavior is verified
// without depending on `pnpm build` having produced real artifacts at compile
// time (Warning #7 fix).
//
// Behavior (RESEARCH §Pattern 8):
//   - Existing files are served with cache headers:
//     /assets/*  → Cache-Control: public, max-age=31536000, immutable
//     /index.html (and HTML fallback) → Cache-Control: no-cache
//   - Unknown paths with no extension or .html → fall through to index.html.
//   - Unknown paths with non-.html extension → 404 (PITFALL #4 anchor).
//
// Path traversal (T-19-01) is mitigated by `path.Clean` on the request path
// plus `fs.Sub` confining reads to the embedded subtree — `../../etc/passwd`
// can never escape the FS root.
//
// Implementation note: index.html is served directly via http.ServeContent
// rather than http.FileServer because FileServer auto-redirects /index.html
// → / (RFC 3875 directory-index convention). For the SPA fallback, the
// canonical URL is the requested route — index.html is the response payload,
// not a redirect target.
func SPAHandlerFS(sub fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cleanPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if cleanPath == "" {
			cleanPath = "index.html"
		}

		// Direct hit on index.html — serve the bytes ourselves so FileServer's
		// /index.html → / redirect doesn't fire. Cache-Control: no-cache.
		if cleanPath == "index.html" {
			w.Header().Set("Cache-Control", "no-cache")
			serveFile(w, r, sub, "index.html")
			return
		}

		f, err := sub.Open(cleanPath)
		if err == nil {
			f.Close()
			// Hashed assets — long cache. HTML — never cache.
			if strings.HasPrefix(cleanPath, "assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			fileServer.ServeHTTP(w, r)
			return
		}

		// Fall through to index.html for SPA routes (no extension or .html).
		ext := path.Ext(cleanPath)
		if ext == "" || ext == ".html" {
			w.Header().Set("Cache-Control", "no-cache")
			serveFile(w, r, sub, "index.html")
			return
		}

		http.NotFound(w, r)
	})
}

// serveFile reads a file from the embedded FS and writes it to the response,
// using http.ServeContent so Range / If-Modified-Since are honored. Used for
// both /index.html direct hits and SPA history-mode fallback so neither path
// triggers FileServer's /index.html → / redirect.
func serveFile(w http.ResponseWriter, r *http.Request, sub fs.FS, name string) {
	f, err := sub.Open(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// http.ServeContent needs an io.ReadSeeker; embed.FS files are seekable.
	rs, ok := f.(io.ReadSeeker)
	if !ok {
		// fstest.MapFS files satisfy io.ReadSeeker; embed.FS files do too.
		// Fallback: read into memory and serve from bytes.
		data, readErr := io.ReadAll(f)
		if readErr != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		http.ServeContent(w, r, name, stat.ModTime(), bytes.NewReader(data))
		return
	}
	http.ServeContent(w, r, name, stat.ModTime(), rs)
}
