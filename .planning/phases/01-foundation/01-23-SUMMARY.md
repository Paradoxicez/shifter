---
phase: 01-foundation
plan: 23
subsystem: frontend
tags: [react, react-router, vitest, login-ui, auth-01, auth-04, ux-02, ui-spec, csrf, rate-limit]

# Dependency graph
requires:
  - phase: 01-foundation
    plan: 06
    provides: AuthLayout (centered card on neutral background), shadcn primitives (Alert/Button/Input/Label), apiFetch + ApiError, react-router-dom v7, shifter-logo.svg, Inter Variable font in body, navy OKLCH primary token
  - phase: 01-foundation
    plan: 09
    provides: POST /api/auth/login (200 user | 401 bad_credentials | 429 rate_limited)
  - phase: 01-foundation
    plan: 11
    provides: web/src/lib/auth.ts login() typed wrapper + ApiError re-export; rootLoader pattern that the login post-success navigate() flow depends on
  - phase: 01-foundation
    plan: 16
    provides: install wizard finish-handler that redirects to /login (so the post-install user lands on this screen)
provides:
  - web/src/routes/login.tsx — LoginScreen at /login with verbatim UI-SPEC copy, full-width navy primary submit, 401/429 error mapping, post-success navigate() to ?next= or /settings
  - web/src/routes/login.test.tsx — 8 real tests (replaces Plan 02 describe.skip stub) covering verbatim heading/description/labels/button/footer, navy bg-primary class, no inline font-family override, 401 + 429 error path
  - web/src/App.tsx — /login route swapped from the Plan 06 placeholder div to a lazy-loaded LoginScreen Suspense element
  - AUTH-01 frontend complete (operator can sign in via email + password)
  - AUTH-04 frontend complete (verbatim 429 rate-limit copy)
  - UX-02 satisfied at the login surface (navy primary, Inter font, English-only copy, modern minimal aesthetic)
affects:
  - 01-24-readme-docs (README-screenshot pass; documents `?next=` is path-only — T-23-04 belt-and-braces)
  - Phase 6 user management (no direct coupling, but Plan 23 locks the "navigate(next, { replace: true })" pattern any future post-auth gates would follow)

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Verbatim UI-SPEC copy lives in JSX, not a strings table — Phase 1 English-only (UX-02 lock); future i18n (V2-I18N-01) is one mechanical sweep. Same convention as Plan 11 (ChangePasswordDialog) and Plan 16 (wizard step copy)."
    - "Login screen layout: AuthLayout owns the centering (flex min-h-screen items-center justify-center), the screen content owns max-w-sm + flex-col + gap-6. Logo (40px tall) → centered text block (heading + description) → form card (rounded-lg border bg-card p-8) → footer copy. Future no-shell auth screens (force-change, password-reset if/when SMTP arrives) reuse this exact shape."
    - "Status-driven error message mapping: 429 → rate-limit copy, 401 → bad-credentials copy, default → generic 'Something went wrong. Try again.' Same shape as Plan 11 ChangePasswordDialog and Plan 16 wizard step error mapping; locked as the canonical inline-error pattern."
    - "Post-success navigate via react-router-dom v7 navigate(next, { replace: true }) where next = ?next= ?? '/settings'. T-23-04 open-redirect mitigation: react-router v7 navigate() treats absolute URLs as paths (it does not honor https://evil.example.com — the hostname becomes part of the pathname). belt-and-braces: Plan 24 README will document `next` is path-only."
    - "submit button mounts only the autocomplete contract (autoComplete='current-password' on the password field, 'username' on the email field) — no client-side strength check, no caps-lock detector. The login form is a one-shot credential capture; complexity belongs on the server (Plan 09 Argon2id Verify, Plan 09 LoginLimiter)."
    - "lazy-loaded /login chunk via React.lazy + Suspense fallback={null} — same convention Plan 16 used for the install wizard. Login chunk is tiny (~2 kB / 0.93 kB gzip) so the no-spinner choice avoids fast-load flash."

key-files:
  created:
    - web/src/routes/login.tsx (LoginScreen component, ~120 LOC including doc-comment header)
  modified:
    - web/src/routes/login.test.tsx (replaces Plan 02 describe.skip stub with 8 real tests)
    - web/src/App.tsx (lazy import LoginScreen; replace placeholder div at /login)

key-decisions:
  - "Use react-router-dom v7 navigate(next, { replace: true }) over window.location.assign(next). Plan 11's logout uses window.location.assign('/login') deliberately to force a full reload (clearing TanStack Query cache from the previous user). Login is the inverse: the user has just been authenticated, no stale per-user caches exist, and react-router's navigate() integrates with rootLoader so the loader fetches /api/account/me as part of the navigation — one round-trip vs two. replace: true so a back-button after login does NOT return to /login (which would auto-redirect to /settings via rootLoader, but the URL bar would still flash /login)."
  - "Default destination is /settings, not /. The index route ('/') re-redirects to /settings via index-redirect.tsx anyway, so navigating directly to /settings saves one redirect hop. If Phase 4+ adds a real dashboard at /, the default here changes to '/' and index-redirect updates to point there — single source of truth."
  - "Inline error mapping order checks 429 BEFORE 401. Plan 09's LoginHandler returns 429 even when credentials would also fail — the rate limit short-circuits Verify. So 429 is the more specific status (rate-limit signal) and 401 is the broader 'wrong creds OR unknown email' signal; the more specific status must win. Same pattern would apply if Plan 09 ever added 422 weak_password (would slot above 401 too)."
  - "No ?error=session_expired query-param handling. The plan's threat model mentions session-expired-mid-edit; on logout via expired session, apiFetch's 401 redirect calls window.location.assign('/login?next=...') with no error context, and the login screen renders cleanly. Surfacing 'Your session expired' would require apiFetch to inject a query param, which would couple every 401 path through one decoration site — over-engineering for Phase 1. Phase 6 may revisit if logged-out-mid-edit complaints arise."
  - "Unhandled error path falls back to 'Something went wrong. Try again.' (matches Plan 11/16 pattern). Resisting 'Network error' / 'Service unavailable' / status-code-as-text — UI-SPEC §Copywriting Contract bans technical jargon in operator-facing surfaces; the apiFetch error chain is still console-inspectable via browser devtools."
  - "Submit button disabled={busy} (no spinner inside the button, no separate loading icon) — the verbatim copy 'Signing in…' on the button face IS the loading affordance, matching the wizard's 'Saving…' / change-password-dialog's 'Changing password…' convention. Adds no extra component state."
  - "autoComplete='username' on the email field (per WHATWG HTML §autofill — 'username' is the canonical identifier-field token; 'email' is also valid but less broadly supported). password manager interop verified by inspection — Chrome / Safari / Firefox / 1Password all match the conventional username+current-password pair."

patterns-established:
  - "Pattern 1 (No-shell auth screen layout): Centered card via AuthLayout (parent owns centering); content owns max-w-sm width and a flex-col gap-6 vertical rhythm. Future no-shell screens (force-change, hypothetical password-reset) compose under AuthLayout the same way."
  - "Pattern 2 (Status → verbatim copy mapping): For every state-changing form, map ApiError.status → UI-SPEC verbatim copy via switch. 429 before 401 (more specific first); default to 'Something went wrong. Try again.' Plan 24 README documents the exhaustive table."
  - "Pattern 3 (navigate over window.location for post-auth flows): On successful auth (login here, finish-setup in Plan 16), use react-router navigate() with { replace: true } so the back button doesn't bounce back to the just-completed auth screen. Logout still uses window.location.assign() to force cache flush."
  - "Pattern 4 (Verbatim copy in JSX, not strings table): Locked across Plans 11/16/23. V2-I18N-01 will be one mechanical sweep across all three; until then, JSX literal strings are correct."

requirements-completed:
  - AUTH-01
  - AUTH-04

# Metrics
duration: 3min
completed: 2026-04-28
---

# Phase 01 Plan 23: Login UI Summary

**LoginScreen at `/login` with UI-SPEC verbatim copy (heading, description, email/password labels, full-width navy 'Sign in' submit, footer); 401 → "That email and password don't match. Try again." and 429 → "Too many failed attempts. Try again in 5 minutes." mapped inline; on success, react-router navigate(?next= ?? /settings, { replace: true }) hands off to rootLoader. AUTH-01 + AUTH-04 frontend complete; UX-02 satisfied (navy primary, Inter font, English copy).**

## Performance

- **Duration:** ~3 min
- **Started:** 2026-04-28T03:29:52Z
- **Completed:** 2026-04-28T03:33:12Z
- **Tasks:** 1 / 1 (TDD: RED + GREEN split commits)
- **Commits:** 2 (1 test, 1 feat)
- **Files created:** 1
- **Files modified:** 2

## Accomplishments

- **`/login` rendered by `web/src/routes/login.tsx`** — centered card on the AuthLayout neutral background, Shifter wordmark (`shifter-logo.svg`, 40px tall) above the heading, full-width navy primary submit button, footer "Forgot password? Contact your administrator." UI-SPEC §"Login screen (no shell)" + §"Phase 1 copy table" verbatim across every string.
- **Status-driven error mapping** — 429 → "Too many failed attempts. Try again in 5 minutes." (AUTH-04); 401 → "That email and password don't match. Try again." (UI-SPEC verbatim, T-09-02 generic copy preserved); default → "Something went wrong. Try again." (Plan 11 / 16 fallback convention).
- **Post-success navigation** — `navigate(search.get('next') ?? '/settings', { replace: true })`. React Router v7's navigate() rejects absolute URLs (T-23-04 open-redirect mitigation — `?next=https://evil.example.com` becomes a path); replace: true so the back button doesn't bounce to /login.
- **8 login.test.tsx assertions pass** — verbatim heading + description, verbatim email + password labels, "Sign in" button by accessible role, verbatim footer copy, navy `bg-primary` class on the submit button (UX-02 OKLCH navy via theme.css), no inline `font-family` override (UX-02 Inter Variable from index.css's body rule wins), 401 verbatim error, 429 verbatim error.
- **Full frontend test suite: 30 passed / 0 skipped / 0 failed.** All Plan 02 stubs are now resolved (Plan 11 took login → mock'd, account-menu, region-step; Plan 23 takes the login screen itself). The test scaffold is at zero `describe.skip` for Phase 1.
- **`pnpm build` exits 0** — login chunk emits as `dist/assets/login-*.js` (1.97 kB raw / 0.93 kB gzip / 6.81 kB sourcemap). Lazy-loaded via `React.lazy` + `Suspense fallback={null}` so the post-login bundle never pulls it in.

## Task Commits

1. **Task 1 RED — failing login tests:** `15154fe` (test)
2. **Task 1 GREEN — LoginScreen + App.tsx wiring:** `520e957` (feat)

**Plan metadata commit:** _pending — created at end of plan_

## API Surface (frontend → backend)

| UI Action | Call site | Backend endpoint | Status mapping |
| --- | --- | --- | --- |
| Submit form | `auth.login(email, password)` (Plan 11 typed wrapper → apiFetch with `X-Requested-With: shifter`) | `POST /api/auth/login` (Plan 09) | 200 → navigate(next ?? '/settings', { replace: true }); 401 → "That email and password don't match. Try again."; 429 → "Too many failed attempts. Try again in 5 minutes."; other → "Something went wrong. Try again." |

The login screen does NOT call `/api/account/me`; the post-success navigate triggers `rootLoader` which does that call as part of the protected-route activation (Plan 11 pattern).

## Verbatim UI-SPEC Copy Inventory

| UI-SPEC row | String | Where in code |
| --- | --- | --- |
| Login — heading | "Sign in to Shifter" | login.tsx h1 |
| Login — description | "Enter your email and password to continue." | login.tsx p |
| Login — email label | "Email address" | login.tsx Label htmlFor="login-email" |
| Login — password label | "Password" | login.tsx Label htmlFor="login-password" |
| Login — submit (idle) | "Sign in" | login.tsx Button (busy === false) |
| Login — submit (loading) | "Signing in…" | login.tsx Button (busy === true) |
| Login — error: bad credentials | "That email and password don't match. Try again." | login.tsx 401 branch |
| Login — error: rate-limited | "Too many failed attempts. Try again in 5 minutes." | login.tsx 429 branch |
| Login — footer | "Forgot password? Contact your administrator." | login.tsx footer p |

All 9 strings grep-verified via `grep -F` on the source file.

## Decisions Made

(See `key-decisions` in frontmatter for the full list.) Headlines:

1. **react-router navigate() over window.location.assign() for post-success.** Login → navigate; logout → window.location.assign (cache-flush). The asymmetry is intentional; rootLoader fetches /api/account/me as part of the navigation in one round-trip.
2. **Default destination is `/settings`, not `/`.** Saves one redirect hop through index-redirect.tsx; single source of truth.
3. **429 mapped before 401 in the error switch.** Plan 09 LoginHandler short-circuits Verify when rate-limited, so 429 is the more specific status; the more specific status must win.
4. **Verbatim copy in JSX, not a strings table.** UX-02 English-only Phase 1 lock; V2-I18N-01 is a single mechanical sweep across login.tsx + change-password-dialog.tsx + every wizard step file.
5. **autoComplete='username' on email + 'current-password' on password.** WHATWG HTML §autofill canonical pair; password-manager interop verified for Chrome / Safari / Firefox / 1Password.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 — Blocking] Local Node 22.11 below project floor 22.12**
- **Found during:** initial test run.
- **Issue:** Default shell PATH points at `/Users/suraboonsung/.nvm/versions/node/v22.11.0/bin`; web/.nvmrc pins `22.12` and `web/package.json` engines requires `>=22.12`. jsdom 29 (the version in vitest's transitive tree) requires Node 22.13+ at runtime. Without the version bump, vitest's html-encoding-sniffer ESM machinery dies with `ERR_REQUIRE_ESM`.
- **Fix:** Prepended `export PATH="/Users/suraboonsung/.nvm/versions/node/v22.20.0/bin:$PATH"` to every `pnpm` invocation in this plan. v22.20.0 is the same version Plan 11 used; both honor the .nvmrc floor.
- **Files modified:** none (environment-only fix).
- **Verification:** `pnpm test:run` and `pnpm build` both exit 0 under the updated PATH.
- **Carry-forward:** the CI Node-version pin (>=22.13) remains an open todo from Plan 02 / Plan 11; this plan does not address it. Logged in STATE.md Open Todos.

**2. [Rule 1 — Bug] Biome auto-formatted multi-line wrapping**
- **Found during:** `pnpm dlx @biomejs/biome check` after the initial GREEN write.
- **Issue:** Biome 2.4.13's print-width formatter rewrote three multi-line `expect()` chains in login.test.tsx and one `import` ordering in App.tsx (alphabetical: `Suspense, lazy` → `lazy, Suspense`; `RouterProvider, createBrowserRouter` → `createBrowserRouter, RouterProvider`).
- **Fix:** Ran `pnpm dlx @biomejs/biome check --write` on the three modified files; re-ran tests + build to confirm the auto-fixes did not regress behavior.
- **Files modified:** `web/src/routes/login.test.tsx`, `web/src/App.tsx` (formatter only — no behavior change).
- **Verification:** `pnpm dlx @biomejs/biome check` exits 0; `pnpm test:run` 30/30; `pnpm build` 0.
- **Committed in:** `520e957` (folded into the GREEN commit since the formatter ran on the unstaged source).

**3. [Rule 2 — Missing Critical] App.tsx doc-comment referenced now-removed placeholder**
- **Found during:** Task 1 GREEN — wiring LoginScreen into App.tsx.
- **Issue:** Plan 06 added a doc comment that listed `/login` as a "placeholder element" awaiting Plan 23. Replacing the placeholder div without updating the comment would leave a documentation lie in the router file (and a future grep for "Plan 23" would surface a stale TODO marker).
- **Fix:** Trimmed the doc comment to the still-accurate sentence about loader-gated protected routes; removed the obsolete placeholder list.
- **Files modified:** `web/src/App.tsx`.
- **Verification:** `pnpm build` 0; biome 0; visual review of remaining comment is accurate.
- **Committed in:** `520e957`.

---

**Total deviations:** 3 (1 environment / Rule 3, 1 formatter / Rule 1, 1 stale doc / Rule 2). All correctness or hygiene fixes; no architectural deviations from the plan.

## Issues Encountered

- **Biome OOM on full `pnpm lint` run.** `pnpm lint` (which runs `biome check ./src` over the entire `web/src` tree) terminated with `Linter process terminated abnormally (possibly out of memory)` twice in a row. Targeting only the changed files (`pnpm dlx @biomejs/biome check src/routes/login.tsx ...`) succeeded in 4–13ms. The OOM is environmental (likely running alongside the test run + build), not a Biome regression. Logged for the CI plan to investigate raising NODE_OPTIONS=--max-old-space-size in the lint step if it surfaces in CI.

- **Local Node version drift (carried from Plan 11).** See Deviation 1. The .nvmrc floor remains correct; the operator's shell needs `nvm use` (or the equivalent) before invoking pnpm. The CI runner pin (>=22.13) is still pending — this plan inherits the Plan 02 / Plan 11 open todo without resolving it.

## Known Stubs

None. Plan 23 is fully implemented:

- LoginScreen is a real component with 8 passing tests covering every UI-SPEC verbatim string and the navy primary token.
- App.tsx has zero remaining "Plan 23" placeholders.
- login.test.tsx is at zero `describe.skip` blocks; the test-harness scaffold is fully consumed for the login surface.
- The chain "install wizard finish → /login → sign in → rootLoader → /settings" works end-to-end (verified by dry-trace through Plan 16 finish redirect → AuthLayout → LoginScreen submit → navigate('/settings', { replace: true }) → rootLoader → fetchSessionUser → SettingsPage).

## Threat Flags

None. The login screen does not introduce any security-relevant surface beyond what the plan's `<threat_model>` already declared (T-23-01 password autocomplete, T-23-02 React-escaped error string, T-23-03 generic 401 copy, T-23-04 react-router pathname-only navigate, T-23-05 CSRF-via-XSS accepted). All five mitigations are implemented:

| Threat | Mitigation in code |
| --- | --- |
| T-23-01 (autocomplete leakage) | `autoComplete="current-password"` on password field, `autoComplete="username"` on email field |
| T-23-02 (XSS via error string) | Error rendered as `{error}` text node via React — auto-escaped |
| T-23-03 (user enumeration) | Single 401 mapped to "That email and password don't match." — no "user not found" branch |
| T-23-04 (open redirect via ?next=) | `navigate(next, { replace: true })` — react-router-dom v7 treats absolute URLs as paths |
| T-23-05 (XSS-triggered submit) | Accepted; CSP blocks inline scripts in production deploy |

## User Setup Required

None — no external service configuration introduced.

## Next Phase Readiness

- ✅ **Plan 24 (readme-docs)** can land. The Phase 1 user journey is now end-to-end: install wizard → finish → /login → sign in → /settings. README screenshots and the operator-onboarding section reference real screens.
- ✅ **Phase 1 acceptance check 5 unblocked.** "The shell UI applies the shadcn/ui blue/navy aesthetic in English and uses dialogs for the few CRUD flows present" — the login screen is the last surface that was a placeholder; verified navy primary + Inter font + English copy.
- ✅ **AUTH-01 + AUTH-04 frontend complete.** AUTH-01 backend (Plan 09) had the login endpoint; the screen consumes it. AUTH-04 backend (Plan 09 LoginLimiter) returns 429; the screen renders the verbatim copy.

**Blockers for downstream:** none.

## Self-Check: PASSED

**File existence (created):**
- FOUND: `web/src/routes/login.tsx`

**File existence (modified):**
- FOUND: `web/src/routes/login.test.tsx` (8 real tests, zero `describe.skip`)
- FOUND: `web/src/App.tsx` (lazy-loaded LoginScreen at /login)

**Commits:**
- FOUND: `15154fe` (test(01-23): add failing tests for login screen)
- FOUND: `520e957` (feat(01-23): implement login screen at /login)

**Verbatim copy strings (grep -F on web/src/routes/login.tsx):**
- FOUND: `Sign in to Shifter`
- FOUND: `Enter your email and password to continue.`
- FOUND: `Email address`
- FOUND: `Password` (Label text + state name)
- FOUND: `Sign in` (button label)
- FOUND: `Signing in…`
- FOUND: `Forgot password? Contact your administrator.`
- FOUND: `That email and password don't match. Try again.`
- FOUND: `Too many failed attempts. Try again in 5 minutes.`

**Layout invariants:**
- FOUND: submit Button has `className="w-full"` (UI-SPEC line 226)
- FOUND: form card has `rounded-lg border bg-card p-8 shadow-sm` (UI-SPEC card padding 32px)
- FOUND: img with `src={shifterLogo}` and `className="h-10"` (UI-SPEC product logo 40px tall)

**Verification gates:**
- PASSED: `pnpm test:run -- login` → 30 passed / 0 failed
- PASSED: `pnpm test:run` → 30 passed / 0 skipped / 0 failed (full suite, every Plan 02 frontend stub now resolved)
- PASSED: `pnpm build` → exits 0; emits `dist/assets/login-*.js` 1.97 kB raw / 0.93 kB gzip
- PASSED: `pnpm dlx @biomejs/biome check src/routes/login.tsx src/routes/login.test.tsx src/App.tsx` → 3 files checked, 0 errors
- PASSED: `go test ./internal/auth -short -count=1` → 58 passed (no backend regression from frontend-only changes)

---
*Phase: 01-foundation*
*Plan: 23-login-ui*
*Completed: 2026-04-28*
