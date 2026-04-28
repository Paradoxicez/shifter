---
phase: 01-foundation
plan: 19
subsystem: spa-embed
tags: [go, embed, spa, vite, http, fileserver, cache-control, fs-sub, fstest, pitfall-4]

requires:
  - phase: 01-foundation
    plan: 06
    provides: Vite + React 19 SPA producing web/dist/ output (index.html + hashed /assets/* bundle)
  - phase: 01-foundation
    plan: 18
    provides: chi router '/*' catch-all targeting deps.SPA; SPAHandler() symbol stub Plan 18 mounted in NewRouter

provides:
  - web/embed.go (//go:embed all:dist surfacing Dist embed.FS at the repo-root web/ package)
  - internal/http/spa.go (SPAHandler() production + SPAHandlerFS(fs.FS) test seam)
  - web/dist/.gitkeep (placeholder so embed compiles before pnpm build runs)
  - 5 SPA tests: TestSPA_FallbackIndex / TestSPA_NoFallbackForAsset / TestSPA_AssetCacheHeaders / TestSPA_IndexHasNoCacheHeader / TestSPA_NoFallbackForAPI

affects:
  - 01-20-compose-bundled (`pnpm build && go build` produces a single image with the SPA embedded — no separate static-asset volume)
  - 01-21-compose-external (same image artifact in external mode)
  - 01-22-caddyfile (Caddy proxies all paths to shifter; the binary serves /assets/* with immutable cache + index.html with no-cache, so Caddy needs zero static-file routing)
  - 01-24-readme-docs (documents the `pnpm build && go build` build prerequisite)
  - Phase 2+ (any new SPA route added in React just works — no backend route registration; the chi `/*` catch-all + SPAHandlerFS history-mode fallback covers it)

tech-stack:
  added:
    - (none — uses Go stdlib `embed`, `io/fs`, `net/http`, `testing/fstest`)
  patterns:
    - "Pattern 1 (RESEARCH §Pattern 8): //go:embed all:dist + fs.Sub('dist') for the embedded SPA bundle. The `all:` prefix is mandatory (T-19-03 — without it, dotfiles like .vite/manifest.json are silently excluded)."
    - "Pattern 2: SPA handler exposes both SPAHandler() (production, embedded FS) and SPAHandlerFS(fs.FS) (testable, accepts any fs.FS). Tests inject testing/fstest.MapFS so behavior is verified deterministically without depending on `pnpm build` having produced real artifacts at compile time (Warning #7 fix)."
    - "Pattern 3: Embed location seam — go:embed paths cannot use `../`, so the directive lives at web/embed.go (where `dist/` is a sibling) rather than internal/http/spa.go. The http package consumes web.Dist via fs.Sub('dist'). This is a pure code-organization detail; the public API remains internal/http.SPAHandler."
    - "Pattern 4: history-mode fallback — paths with no extension or .html that don't exist in the FS fall through to /index.html with Cache-Control: no-cache. Paths with non-html extensions that don't exist return 404 (PITFALL #4 anchor: /api/missing-image.png MUST NOT return the SPA index)."
    - "Pattern 5: serveFile() helper — index.html is served via http.ServeContent (with manual Cache-Control header injection) rather than http.FileServer because FileServer auto-redirects /index.html → / per RFC 3875 directory-index convention. For SPA fallback, the canonical URL is the requested route — index.html is the response payload, not a redirect target."
    - "Pattern 6: build prerequisite documented — `pnpm build` must run before `go build` for production. The web/dist/.gitkeep placeholder lets `go:embed all:dist` compile in CI before the frontend build runs (the SPA bundle is just empty in that case, but the binary still links)."

key-files:
  created:
    - web/embed.go
    - web/dist/.gitkeep
  modified:
    - internal/http/spa.go (replaced 503 placeholder with full handler)
    - internal/http/spa_test.go (replaced 3 t.Skip stubs with 5 real tests using fstest.MapFS)
    - .gitignore (allow web/dist/.gitkeep alongside ignored /web/dist/* contents)
    - web/.gitignore (allow dist/.gitkeep alongside ignored dist/* contents)

key-decisions:
  - "Embed directive lives at web/embed.go (//go:embed all:dist), NOT at internal/http/spa.go (//go:embed all:web/dist). The plan specified the latter, but Go's go:embed directive cannot reference paths outside the source file's directory subtree (no `../` allowed). Solution: a thin web package at the repo root holds the embed; internal/http consumes web.Dist via fs.Sub('dist'). Public API (SPAHandler / SPAHandlerFS) is unchanged — only the directive's physical location differs from the plan's verbatim. Future plans referencing the embed should grep `//go:embed all:dist` in `web/embed.go`, NOT in `internal/http/spa.go`."
  - "SPAHandler is the production entry point; SPAHandlerFS(fs.FS) is the testable seam. The split is mandatory because fstest.MapFS-backed tests must run BEFORE `pnpm build` has populated web/dist/ — relying on the real embed would require ordering test runs after the frontend build (slow + adds CI fragility). Future SPA-handling additions (e.g., per-locale variants in V2) MUST extend SPAHandlerFS so the test seam is preserved."
  - "index.html served via http.ServeContent, not http.FileServer. FileServer auto-redirects /index.html → / per RFC 3875; for SPA fallback we want the requested URL preserved (index.html is response payload, not redirect target). serveFile() helper handles both direct /index.html requests AND history-mode fallback paths; both branches set Cache-Control: no-cache before delegating. http.FileServer is still used for /assets/* requests where its standard behavior is correct."
  - "/assets/* gets `Cache-Control: public, max-age=31536000, immutable`. This pairs with Vite's content-hashed asset filenames (e.g., index-BC86Dl5m.js) — the hash changes on every content change, so cached assets are valid forever. RESEARCH §Pattern 8."
  - "/index.html (and any HTML fallback) gets `Cache-Control: no-cache`. Index.html is the only HTML the SPA serves; it must always be re-fetched so the operator's browser picks up new asset hashes after a deploy. Cache-Control: no-cache (not no-store) lets the browser revalidate via ETag without forcing a full re-download when content hasn't changed."
  - "PITFALL #4 anchor: paths with non-html extensions that don't exist in the FS return 404, NEVER fall through to index.html. Plan 18 mounts the SPA `/*` catch-all LAST, so an unknown /api/foo.json reaches the SPA handler and gets a 404 (consistent with the chi router's NotFound behavior for /api/*). TestSPA_NoFallbackForAPI is the canonical regression test — chi-router-level, runs in <100ms, mirrors Plan 18's middleware composition."
  - "Path traversal mitigation (T-19-01) via path.Clean + fs.Sub. path.Clean normalizes `../../etc/passwd` to `etc/passwd`; fs.Sub confines reads to the embedded dist/ subtree. Even a fully malformed path can never escape the FS root. ASVS V12. No additional sanitization needed at the handler layer."

patterns-established:
  - "Pattern: SPA handlers expose both the production-bound and FS-injected forms. Future SPA refactors (V2 multi-locale, per-tenant theming) MUST keep SPAHandlerFS(fs.FS) as the test seam — never collapse to a single SPAHandler() that only takes the embedded FS, because that ties test execution to `pnpm build` ordering."
  - "Pattern: //go:embed lives at the package nearest the source tree. For Vite output, that's web/embed.go. For database migrations (Plan 03), it's internal/db/migrations.go (which has migrations/*.sql as a sibling). Future embeddable artifacts (e.g., default device profile JSON, region catalog) follow the same rule — embed at the package nearest the source files."
  - "Pattern: history-mode fallback ext-check is a 2-bucket switch — `ext == '' || ext == '.html'` falls through to index.html, everything else 404s. Future deviations (e.g., custom .txt routes) MUST be added to this whitelist explicitly; the default deny-list shape preserves PITFALL #4."
  - "Pattern: serve embedded HTML via http.ServeContent (not FileServer) when the canonical URL must be preserved. FileServer's directory-index redirect is correct for filesystem servers but wrong for SPA fallback. Future binary-embedded HTML (e.g., a future static error page) follows the same pattern."

requirements-completed: []  # Plan 19 has no requirements field; plan-19 satisfies infrastructure (D-13 SPA serving) without a specific REQ-ID.

duration: 19min
completed: 2026-04-28
---

# Phase 01 Plan 19: SPA Embed Summary

**go:embed-backed SPA handler at internal/http.SPAHandler (production) + SPAHandlerFS(fs.FS) (test seam) consuming web.Dist embed.FS from web/embed.go; history-mode fallback for unknown SPA routes; /assets/* gets immutable cache + /index.html no-cache; PITFALL #4 anchor verified by chi-router-level TestSPA_NoFallbackForAPI; binary size 30M → 33.6M with the SPA bundle embedded.**

## Performance

- **Duration:** ~19 min (incl. ~5 min recovery from a transient 100% disk-full condition partway through verification)
- **Started:** 2026-04-28T06:10:38Z
- **Completed:** 2026-04-28T06:29:18Z
- **Tasks:** 1 / 1
- **Commits:** 2 (1 RED + 1 GREEN)
- **Files created:** 2 (`web/embed.go`, `web/dist/.gitkeep`)
- **Files modified:** 4 (`internal/http/spa.go`, `internal/http/spa_test.go`, `.gitignore`, `web/.gitignore`)
- **Tests added:** 5 (replaced 3 t.Skip stubs with 5 real tests via fstest.MapFS)

## Accomplishments

- Production SPA serving works — `shifter serve` now responds to `/dashboard`, `/sites/abc`, etc. with the real Vite-built index.html (200) and cache headers per RESEARCH §Pattern 8.
- PITFALL #4 anchor verified at the chi-router level — `/api/nonexistent-endpoint` returns JSON 404, NOT the SPA HTML.
- Test seam preserved: `SPAHandlerFS(fs.FS)` accepts any fs.FS so unit tests use `testing/fstest.MapFS` instead of depending on `pnpm build` ordering.
- Binary is now self-contained: `go build ./cmd/shifter` produces a 33.6M static binary that includes the entire SPA — no separate static-asset volume needed for Plans 20/21 compose deploys.

## Task Commits

Each task was committed atomically (TDD: RED → GREEN):

1. **Task 1 RED — Failing SPA tests** — `c093db1` (test): 5 SPA tests using `testing/fstest.MapFS`, fail to compile because `SPAHandlerFS` undefined.
2. **Task 1 GREEN — SPAHandler implementation** — `a00b105` (feat): web/embed.go + internal/http/spa.go full body + .gitignore updates + web/dist/.gitkeep. All 5 SPA tests pass.

**Plan metadata commit:** (this SUMMARY commit)

## Files Created/Modified

| File | What | Status |
|------|------|--------|
| `web/embed.go` | `//go:embed all:dist` surfacing `web.Dist` embed.FS | Created |
| `web/dist/.gitkeep` | Empty placeholder so embed compiles before `pnpm build` runs | Created |
| `internal/http/spa.go` | Full SPAHandler / SPAHandlerFS body (RESEARCH §Pattern 8) | Replaced 503 placeholder |
| `internal/http/spa_test.go` | 5 SPA tests using `testing/fstest.MapFS` | Replaced 3 t.Skip stubs |
| `.gitignore` | Allow `/web/dist/.gitkeep` while ignoring `/web/dist/*` | Modified |
| `web/.gitignore` | Allow `dist/.gitkeep` while ignoring `dist/*` | Modified |

## Behavior Summary

| Request                              | Response                                                          |
| ------------------------------------ | ----------------------------------------------------------------- |
| `GET /` or `GET /index.html`         | 200 + `Cache-Control: no-cache` + index.html bytes                |
| `GET /dashboard` (SPA route)         | 200 + `Cache-Control: no-cache` + index.html bytes (fallback)     |
| `GET /assets/index-abc.js` (exists)  | 200 + `Cache-Control: public, max-age=31536000, immutable` + JS   |
| `GET /missing-image.png`             | 404 (no fallback — PITFALL #4 anchor)                             |
| `GET /api/foo` (chi router context)  | JSON 404 from chi NotFoundHandler (NEVER reaches SPA handler)     |

## Test-Coverage Matrix

| Test                              | What It Asserts                                                              |
| --------------------------------- | ---------------------------------------------------------------------------- |
| `TestSPA_FallbackIndex`           | `/dashboard` → 200 + index.html body + `Cache-Control: no-cache`             |
| `TestSPA_NoFallbackForAsset`      | `/missing-image.png` → 404 (PITFALL #4)                                      |
| `TestSPA_AssetCacheHeaders`       | `/assets/index-abc123.js` → `max-age=31536000` + `immutable`                 |
| `TestSPA_IndexHasNoCacheHeader`   | `/index.html` direct hit → 200 + `Cache-Control: no-cache` (no 301 redirect) |
| `TestSPA_NoFallbackForAPI`        | chi router with NotFoundHandler — `/api/foo` returns JSON 404, NOT HTML      |

All 5 pass under `go test -race -count=1`.

## Decisions Made

See `key-decisions` in frontmatter for the canonical list. Highlights:

- **Embed directive lives at web/embed.go**, not internal/http/spa.go — Go's `go:embed` cannot use `../` so paths must be descendants of the source file's directory. internal/http imports web.Dist and consumes it via fs.Sub('dist').
- **SPAHandler + SPAHandlerFS split** — production uses embedded FS, tests inject fstest.MapFS. Eliminates `pnpm build` ordering coupling.
- **index.html via http.ServeContent**, not http.FileServer — FileServer's `/index.html → /` redirect is correct for filesystem servers but breaks SPA semantics where the canonical URL is the requested route.
- **/assets/* immutable, /index.html no-cache** — pairs with Vite's content-hashed asset names so deploys flip cleanly without orphaning cached HTML.
- **Path traversal mitigated by path.Clean + fs.Sub** — no additional sanitization at handler layer (T-19-01 / ASVS V12).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug in plan] go:embed directive cannot live at internal/http/spa.go**

- **Found during:** Task 1 GREEN — first `go build` after writing internal/http/spa.go failed with `pattern all:web/dist: no matching files found`.
- **Issue:** Plan-verbatim specified `//go:embed all:web/dist` in `internal/http/spa.go`, with the literal directive checked by an acceptance grep. Go's `go:embed` rule: patterns must point at descendants of the source file's directory. From `internal/http/spa.go`, `web/dist` would mean `internal/http/web/dist`, which doesn't exist. Patterns cannot use `..` to escape, so the verbatim directive cannot compile from `internal/http/`.
- **Fix:** Created `web/embed.go` (package `web`) with the canonical directive `//go:embed all:dist` (which DOES work because `web/dist/` is a sibling of `web/embed.go`). Refactored `internal/http/spa.go` to import `web` and call `fs.Sub(web.Dist, "dist")` instead of embedding directly. Public API (SPAHandler / SPAHandlerFS exports from internal/http) is unchanged — only the directive's physical location differs.
- **Files modified:** `web/embed.go` (new), `internal/http/spa.go` (consume `web.Dist` instead of local embed).
- **Verification:** `go build ./...` passes; binary embeds the SPA bundle correctly (binary size 30M → 33.6M); all 5 SPA tests pass.
- **Commit:** `a00b105` (Task 1 GREEN).

**2. [Rule 1 - Bug] http.FileServer auto-redirects /index.html → / and breaks Cache-Control invariant**

- **Found during:** Task 1 GREEN — first test run after implementing `SPAHandlerFS`, `TestSPA_IndexHasNoCacheHeader` failed with `expected 200, got 301`.
- **Issue:** Plan-verbatim used `http.FileServer` for both direct hits and history-mode fallback. `http.FileServer.ServeHTTP` redirects `/index.html` → `/` per RFC 3875 directory-index convention — meaning a direct request for `/index.html` gets a 301 redirect, not the file body. This breaks both `TestSPA_IndexHasNoCacheHeader` (which asserts 200) and the underlying invariant (direct /index.html hits should return the bytes, not redirect).
- **Fix:** Added `serveFile()` helper that reads the file from the FS and uses `http.ServeContent` to write the response (preserves the requested URL). Used `serveFile()` for both /index.html direct hits and history-mode fallback. `http.FileServer` is still used for /assets/* requests where its standard behavior is correct.
- **Files modified:** `internal/http/spa.go`.
- **Verification:** `TestSPA_IndexHasNoCacheHeader` passes (200 + Cache-Control: no-cache); `TestSPA_FallbackIndex` still passes (history-mode fallback also serves index.html with no-cache header).
- **Commit:** `a00b105` (Task 1 GREEN).

**3. [Rule 2 - Missing critical] web/.gitignore also ignored dist/.gitkeep**

- **Found during:** Task 1 GREEN — `git status` showed `web/dist/` as untracked even after creating `web/dist/.gitkeep` because `web/.gitignore` had a blanket `dist` entry.
- **Issue:** Without tracking `web/dist/.gitkeep`, fresh clones would have no `web/dist/` directory and `go build` would fail (`pattern all:dist: no matching files found`). The repo-root `.gitignore` was correctly updated, but `web/.gitignore` independently masked the .gitkeep.
- **Fix:** Updated `web/.gitignore` from `dist` to `dist/*` + `!dist/.gitkeep` so the placeholder is tracked.
- **Files modified:** `web/.gitignore`.
- **Verification:** `git check-ignore -v web/dist/.gitkeep` shows `!dist/.gitkeep` rule wins; .gitkeep is tracked in HEAD.
- **Commit:** `a00b105` (Task 1 GREEN).

---

**Total deviations:** 3 auto-fixed (2 Rule 1 plan-bug fixes, 1 Rule 2 missing critical).

**Impact on plan:** None on the success criteria or public API. SPAHandler / SPAHandlerFS export shapes match the plan exactly. The literal grep `//go:embed all:web/dist` in `internal/http/spa.go` no longer matches — instead the canonical grep is `//go:embed all:dist` in `web/embed.go`. RESEARCH §Pattern 8 invariants (`all:` prefix, history-mode fallback, /assets cache headers, /index.html no-cache, no-fallback for non-html missing assets) are all preserved. PITFALL #4 anchor unchanged.

## Issues Encountered

- **Disk space hit 100% mid-verification.** A `go test -race` run filled `/tmp` to capacity, causing the linker to fail with `mapping output file failed: no space left on device`. Recovery: `go clean -cache -testcache` freed 1.5GB. Documented because future verification commands using `-race` should account for ~3GB of intermediate test binaries on a fresh cache.
- **Docker / testcontainer-dependent tests in `internal/http` and `internal/auth` did not run** in this session (Docker daemon was unresponsive). The 5 new SPA tests are pure-unit (no Docker) and all passed. The tests that were skipped due to Docker unavailability (`TestHealthDetailed_RequiresAdmin`, `TestRBAC_AdminAllowed`, `TestAccount_ChangePassword`) are pre-existing and unrelated to Plan 19's surface — none of them touch the SPA handler. Plan 20's compose smoke run remains the integration anchor for the full HTTP stack against testcontainer dependencies.

## Known Stubs

None. Every Plan 18 stub related to the SPA (`TestSPA_FallbackIndex`, `TestSPA_NoFallbackForAPI`, `TestSPA_AssetCacheHeaders`) is now a real implementation. The `SPAHandler()` 503 placeholder Plan 18 created has been replaced.

## User Setup Required

None for development / testing. Production deploy build sequence:

```bash
# 1. Build frontend (populates web/dist/)
cd web && pnpm build && cd ..

# 2. Build Go binary (embeds web/dist/)
go build ./cmd/shifter

# 3. Run
./shifter serve
```

The Justfile (Plan 01) and Plan 24's README will document this two-step build. Plans 20/21's Docker images bake both steps into a multi-stage build.

## Next Phase Readiness

- ✅ Plan 18's `r.Handle("/*", deps.SPA)` catch-all now serves real SPA HTML in production — no more 503 placeholder response.
- ✅ Plan 20 (compose-bundled): single-binary image. The Dockerfile will run `pnpm build && go build` in a multi-stage build; final image only ships the binary (~34MB) — no node_modules / no separate /assets/ volume.
- ✅ Plan 21 (compose-external): same image artifact in external mode.
- ✅ Plan 22 (caddyfile): Caddy proxies all paths to shifter; the binary's own Cache-Control headers handle asset vs HTML differentiation, so Caddy doesn't need static-file routing logic.
- ✅ Plan 23 (login-ui — already shipped): /login renders correctly via the SPA fallback; the 5 SPA tests don't change Plan 23's behavior.
- ✅ Plan 24 (readme-docs): the build prerequisite (`pnpm build` before `go build`) is the headline operator-facing item to document.
- ✅ Phase 2+: any new SPA route added in React just works — no backend route registration. The chi `/*` catch-all + history-mode fallback covers it.

## Self-Check: PASSED

Files verified to exist:

- FOUND: `web/embed.go`
- FOUND: `web/dist/.gitkeep` (tracked: `git ls-files web/dist/.gitkeep` returns the path)
- FOUND: `internal/http/spa.go` (replaced — full body)
- FOUND: `internal/http/spa_test.go` (replaced — 5 real tests via fstest.MapFS)
- FOUND: `.gitignore` (modified — `/web/dist/*` + `!/web/dist/.gitkeep`)
- FOUND: `web/.gitignore` (modified — `dist/*` + `!dist/.gitkeep`)

Commits verified to exist:

- FOUND: `c093db1` (Task 1 RED — failing SPA tests)
- FOUND: `a00b105` (Task 1 GREEN — SPAHandler + web/embed.go + gitignore + .gitkeep)

Behavior verified:

- `go build ./...` exits 0
- `go vet ./...` exits 0
- `go build ./cmd/shifter` exits 0; binary size 33.6M (vs Plan 18's 30M — ~3.6MB SPA bundle embedded)
- `go test ./internal/http -run TestSPA_ -race -count=1` → 5 passed
- `go test ./internal/config ./internal/version ./internal/logging ./web -short -race -count=1` → 26 passed (no regressions in the unit-only packages)
- `cd web && pnpm build` → succeeds; produces real index.html + /assets/* hashed bundle
- After `pnpm build`, `go test ./internal/http -run TestSPA_ -race -count=1` → 5 passed (real embedded FS works the same as fstest.MapFS)

Acceptance grep proofs:

- `grep -n 'go:embed all:dist' web/embed.go` → 1 match at line 25 (the canonical directive; deviation from plan's expected `//go:embed all:web/dist` in internal/http/spa.go — see Deviation #1)
- `grep -E '^func (SPAHandler|SPAHandlerFS)' internal/http/spa.go` → 2 matches (both required exports present)
- `grep -n 'fs.Sub' internal/http/spa.go` → 1 match (`fs.Sub(web.Dist, "dist")` strips the prefix)
- `grep -n 'max-age=31536000' internal/http/spa.go` → 1 match (immutable cache for /assets/*)
- `grep -n '"no-cache"' internal/http/spa.go` → 2 matches (HTML fallback + index.html direct hit)
- `grep -n 'http.NotFound' internal/http/spa.go` → 2 matches (PITFALL #4: missing assets 404, missing index.html 404)
- `git ls-files web/dist/.gitkeep` → returns path (placeholder tracked)
- `grep -n '!/web/dist/.gitkeep' .gitignore` → 1 match (gitignore exception)
- `grep -n '!dist/.gitkeep' web/.gitignore` → 1 match (web-level gitignore exception)

---
*Phase: 01-foundation*
*Plan: 19-spa-embed*
*Completed: 2026-04-28*
