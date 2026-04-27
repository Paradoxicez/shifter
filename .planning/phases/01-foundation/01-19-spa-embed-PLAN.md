---
phase: 01-foundation
plan: 19
type: execute
wave: 13
depends_on: [06, 18]
files_modified:
  - internal/http/spa.go
  - internal/http/spa_test.go
  - web/dist/.gitkeep
  - .gitignore
autonomous: true
requirements: []
must_haves:
  truths:
    - "SPAHandler returns index.html for any non-existent route that doesn't look like a static asset"
    - "SPAHandler returns 404 for missing static assets (no fallback for asset paths with extensions)"
    - "Hashed assets under /assets/* get Cache-Control: public, max-age=31536000, immutable (unit-tested via TestSPA_AssetCacheHeaders against fstest.MapFS)"
    - "/index.html is served with Cache-Control: no-cache (unit-tested via TestSPA_IndexHasNoCacheHeader)"
    - "/api/* 404s do NOT return the SPA index — they return a JSON 404 from the router (PITFALL #4; unit-tested via TestSPA_NoFallbackForAPI)"
    - "//go:embed all:web/dist (with the all: prefix per RESEARCH §Pattern 8)"
    - "SPAHandlerFS(fs.FS) is exported for unit-test injection of synthetic SPA artifacts"
  artifacts:
    - path: "internal/http/spa.go"
      provides: "go:embed-backed SPA handler with history-mode fallback (RESEARCH §Pattern 8); also exports SPAHandlerFS(fs.FS) for unit-test injection (Warning #7)"
      contains: "//go:embed all:web/dist"
      exports: ["SPAHandler", "SPAHandlerFS"]
  key_links:
    - from: "internal/http/spa.go"
      to: "web/dist (Vite build output)"
      via: "go:embed all:web/dist"
      pattern: "all:web/dist"
---

<objective>
Implement the canonical SPA embed handler per RESEARCH §Pattern 8: `//go:embed all:web/dist`, `fs.Sub` to strip the prefix, fall through to `index.html` for any non-existent route that's not a static asset, and set proper cache headers.

Purpose: The `serve` command's `httpapi.SPAHandler()` returns this; without it the binary serves no UI in production.

Output: `go test ./internal/http -run TestSPA_` passes; `web/dist/` is a build artifact (gitignored except for `.gitkeep` so go:embed doesn't fail in CI before frontend build runs).
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/01-foundation/01-RESEARCH.md
@01-06-frontend-shell-PLAN.md
@01-18-router-health-PLAN.md

<interfaces>
RESEARCH §Pattern 8 (lines 668-727) — verbatim SPAHandler implementation. Key invariants:
- `//go:embed all:web/dist` (the `all:` prefix is mandatory)
- `fs.Sub(spaFS, "web/dist")` strips the prefix
- For paths with NO extension or `.html` extension that don't exist → fall through to index.html
- For paths WITH a non-`.html` extension that don't exist → 404
- /assets/* → `Cache-Control: public, max-age=31536000, immutable`
- index.html (and any HTML fallback) → `Cache-Control: no-cache`
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: SPAHandler + tests + web/dist seed</name>
  <files>internal/http/spa.go, internal/http/spa_test.go, web/dist/.gitkeep, .gitignore</files>
  <read_first>
    - .planning/phases/01-foundation/01-RESEARCH.md §"Pattern 8: go:embed SPA + history-mode fallback" (lines 668-727)
    - .planning/phases/01-foundation/01-VALIDATION.md (TestSPA_FallbackIndex, TestSPA_NoFallbackForAPI names)
  </read_first>
  <behavior>
    - TestSPA_FallbackIndex: GET /dashboard (a SPA route, no extension, no file in dist) → 200 + body of index.html.
    - TestSPA_NoFallbackForAPI: handler is mounted at `/*` AFTER /api/* routes per Plan 18; this test verifies that requests reaching the SPA handler with `.json` / `.png` / etc. NOT in dist return 404 (so /api/whatever-image.png 404s instead of returning HTML). Note: the API 404 path is exercised by router tests (Plan 18); this test asserts the handler's own behavior for unknown asset paths.
    - TestSPA_AssetCacheHeaders: GET /assets/index-abc.js (must exist in test fixture) → Cache-Control header contains "immutable".
    - TestSPA_IndexHasNoCacheHeader: GET /index.html → Cache-Control: no-cache.
  </behavior>
  <action>
1. Update `.gitignore` to keep tracking `web/dist/.gitkeep` while ignoring everything else in dist. The Plan 01 .gitignore already has `/web/dist/`. Replace with:
   ```
   # Build artifacts (allow .gitkeep for go:embed)
   /web/dist/*
   !/web/dist/.gitkeep
   ```
   Apply this change in addition to other already-present entries.

2. Create `web/dist/.gitkeep` — empty file. This satisfies `//go:embed all:web/dist` in CI before `pnpm build` runs.

3. Create `internal/http/spa.go` — VERBATIM from RESEARCH §Pattern 8, refactored to expose a testable `SPAHandlerFS` (Warning #7 fix — lets tests provide a synthetic fs without depending on `pnpm build` having produced real `web/dist/` artifacts at compile time):
   ```go
   package http

   import (
       "embed"
       "io/fs"
       "net/http"
       "path"
       "strings"
   )

   //go:embed all:web/dist
   var spaFS embed.FS

   // SPAHandler is the production entry point. It serves the embedded Vite build
   // with history-mode fallback. Mount LAST per PITFALL #4.
   func SPAHandler() http.Handler {
       sub, err := fs.Sub(spaFS, "web/dist")
       if err != nil {
           panic(err)
       }
       return SPAHandlerFS(sub)
   }

   // SPAHandlerFS is the testable form: returns a handler over any fs.FS rooted
   // at the SPA build output (i.e. the dir containing index.html and assets/).
   //
   // Behavior (RESEARCH §Pattern 8):
   //   - Existing files are served with cache headers:
   //       /assets/*  → Cache-Control: public, max-age=31536000, immutable
   //       /index.html (and HTML fallback) → Cache-Control: no-cache
   //   - Unknown paths with no extension or .html → fall through to index.html.
   //   - Unknown paths with non-.html extension → 404 (PITFALL #4 anchor).
   func SPAHandlerFS(sub fs.FS) http.Handler {
       fileServer := http.FileServer(http.FS(sub))

       return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
           cleanPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
           if cleanPath == "" {
               cleanPath = "index.html"
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
               r2 := r.Clone(r.Context())
               r2.URL.Path = "/"
               w.Header().Set("Cache-Control", "no-cache")
               fileServer.ServeHTTP(w, r2)
               return
           }

           http.NotFound(w, r)
       })
   }
   ```

4. Replace `internal/http/spa_test.go` — uses `testing/fstest.MapFS` to provide synthetic SPA artifacts (no dependency on `pnpm build` at test-compile time, no manipulation of the `web/dist` directory). The PRODUCTION `SPAHandler()` still uses the embedded FS; tests exercise `SPAHandlerFS(sub fs.FS)`:
   ```go
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
   //   index.html, assets/index-abc123.js
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
               _, _ = w.Write([]byte(` + "`{"error":"not_found"}`" + `))
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
   ```
  </action>
  <verify>
    <automated>cd web && pnpm build && cd .. && go test ./internal/http -run 'TestSPA_FallbackIndex|TestSPA_NoFallbackForAsset|TestSPA_AssetCacheHeaders|TestSPA_IndexHasNoCacheHeader|TestSPA_NoFallbackForAPI' -race -count=1 -v</automated>
  </verify>
  <acceptance_criteria>
    - File `internal/http/spa.go` contains the literal directive `//go:embed all:web/dist` (line-anchored, exactly that prefix per RESEARCH §Pattern 8 — `all:` mandatory)
    - File exports both `func SPAHandler() http.Handler` (production, uses embedded FS) and `func SPAHandlerFS(sub fs.FS) http.Handler` (testable, accepts any fs.FS)
    - `SPAHandler()` delegates to `SPAHandlerFS(fs.Sub(spaFS, "web/dist"))` — single source of truth for the SPA serving logic
    - File handles SPA history-mode fallback for paths with no extension or `.html`
    - File returns 404 for paths with non-`.html` extensions that don't exist (PITFALL #4 plus asset-vs-page distinction)
    - File sets `Cache-Control: public, max-age=31536000, immutable` for `/assets/*` paths
    - File sets `Cache-Control: no-cache` for HTML fallback AND for /index.html
    - File `web/dist/.gitkeep` exists (so go:embed compiles before first frontend build)
    - `.gitignore` has `/web/dist/*` and `!/web/dist/.gitkeep` (allows the placeholder)
    - Command `cd web && pnpm build` produces `web/dist/index.html`
    - Command `go build ./internal/http` exits 0 (with `web/dist/.gitkeep` present)
    - Command `go test ./internal/http -run TestSPA_FallbackIndex -race -count=1` exits 0 (per VALIDATION.md)
    - Command `go test ./internal/http -run TestSPA_NoFallbackForAsset -race -count=1` exits 0
    - Command `go test ./internal/http -run TestSPA_AssetCacheHeaders -race -count=1` exits 0 (Warning #7 — no longer a t.Skip; uses fstest.MapFS to provide a synthetic /assets/* file)
    - Command `go test ./internal/http -run TestSPA_IndexHasNoCacheHeader -race -count=1` exits 0
    - Command `go test ./internal/http -run TestSPA_NoFallbackForAPI -race -count=1` exits 0 (Warning #7 — no longer a t.Skip; chi-router-level test asserting `/api/*` 404 returns JSON, not the SPA index)
  </acceptance_criteria>
  <done>
    SPA embed wired. `shifter serve` (Plan 18) now serves the full UI in production. Plan 20/21 compose verifies `pnpm build && go build` produces a runnable image.
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| browser → SPA assets | Static files served from embedded FS; no per-request DB |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-19-01 | Information Disclosure | path traversal `/../../etc/passwd` reaches embedded FS | mitigate | `path.Clean` + `fs.Sub` confine reads to `web/dist`. ASVS V12. |
| T-19-02 | Information Disclosure | source maps shipped in production | accept | Self-hosted single-tenant; sourcemaps aid debugging. Operator may disable per-install. |
| T-19-03 | Tampering | dotfiles in dist (e.g. `.vite/manifest.json`) silently excluded | mitigate | `all:` prefix in go:embed includes dotfiles. RESEARCH §Pattern 8. |
| T-19-04 | Information Disclosure | embedded FS exposes internal Go source | mitigate | go:embed only includes `web/dist` subtree (not `.go` files). |
</threat_model>

<verification>
- `//go:embed all:web/dist` directive present
- `web/dist/.gitkeep` checked in
- `.gitignore` updated to allow .gitkeep
- 5 SPA tests pass (FallbackIndex, NoFallbackForAsset, AssetCacheHeaders, IndexHasNoCacheHeader, NoFallbackForAPI) — Warning #7 fix replaces 2 t.Skip stubs with real fstest.MapFS-backed unit tests
- `SPAHandlerFS(fs.FS)` exported for testability; production `SPAHandler()` delegates to it
- `go build` exits 0 even when `web/dist/` only contains `.gitkeep`
</verification>

<success_criteria>
- SPA fallback per RESEARCH §Pattern 8
- Cache headers correct (immutable for /assets/*, no-cache for HTML)
- PITFALL #4 already mitigated by Plan 18 mount order
- Single binary serves UI + API
</success_criteria>

<output>
After completion, create `.planning/phases/01-foundation/01-19-SUMMARY.md` documenting:
- go:embed directive + reason for `all:` prefix
- Cache-control rules
- web/dist/.gitkeep convention
- Build prerequisite: `pnpm build` before `go build` for production
</output>
