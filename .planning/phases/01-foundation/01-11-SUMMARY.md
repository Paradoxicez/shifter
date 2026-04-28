---
phase: 01-foundation
plan: 11
subsystem: auth
tags: [react, react-router, vitest, account-ui, change-password, sse-ready, auth-05, auth-06, csrf, ui-spec]

# Dependency graph
requires:
  - phase: 01-foundation
    plan: 06
    provides: ResponsiveDialog + AccountMenu shell + apiFetch + ApiError + Toaster + ResponsiveShell + ThemeProvider
  - phase: 01-foundation
    plan: 08
    provides: auth.User session payload (id, role) + GetUser context helper used by AccountInfoHandler
  - phase: 01-foundation
    plan: 09
    provides: Store.GetUserByID + ErrUserNotFound + writeJSON / errorResp envelope + ChangePasswordHandler (POST /api/account/password)
  - phase: 01-foundation
    plan: 10
    provides: auth.RequireAction(sm, action) + ActionAccountSelfEdit (Plan 17/18 will wrap protected mutating routes; Plan 11 adds the account-me read endpoint that the loader calls before any wrapped mutation)

provides:
  - internal/auth.AccountInfoHandler — GET /api/account/me returns the post-login user (id, email, role, must_change_password); 401 for missing session and for disabled-mid-session accounts
  - web/src/lib/auth.ts — typed wrappers (fetchSessionUser, login, logout, changePassword) + SessionUser type + ApiError re-export; the canonical client every Phase 1+ feature consumes
  - web/src/routes/_root.tsx — rootLoader (calls fetchSessionUser, throws redirect('/login?next=...') on 401) + RootLayout (wires AccountMenu + ChangePasswordDialog + sonner success toast)
  - web/src/routes/change-password-dialog.tsx — ChangePasswordDialog (UI-SPEC verbatim copy, ResponsiveDialog wrapper, 401 / 422 inline error mapping, Cancel-left/primary-right footer)
  - AUTH-05 frontend (operator can change own password from the avatar dropdown) and AUTH-06 frontend hiding scaffolding (userRole prop threads through to AccountMenu; phase-1 menu has no admin-only items but the prop is in place for Phase 2+ guards)
  - D-09 verification at the API: TestAccountInfo_ReturnsUser asserts must_change_password=false for the create-admin / wizard admin

affects:
  - 01-14-install-middleware (rootLoader will compose with first-run-install gate; the loader pattern locked here is the integration point)
  - 01-16-install-wizard-ui (the wizard's "Finish" handoff redirects to / which now renders RootLayout — the loader fetches the freshly-created admin's session)
  - 01-17-test-connection (Settings page is a child of RootLayout — inherits AccountMenu, theme, sonner Toaster wiring; reuses ApiError mapping pattern from ChangePasswordDialog)
  - 01-23-login-ui (login.tsx posts via auth.ts login() and on success window.location.assign('/') triggers rootLoader → AccountMenu populated)
  - Phase 2+ admin-only menu items (e.g. "Audit log", "Users") will land as `userRole === 'admin' && <DropdownMenuItem>...</DropdownMenuItem>` inside account-menu.tsx — the prop is already plumbed through

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Loader-gated protected routes: rootLoader fetches /api/account/me before any protected layout renders; on 401 it throws redirect('/login?next=…'). React-Router v7 loader semantics — the route never renders with stale or missing user data. Future protected sub-routes (Settings, Dashboard, Devices) inherit this gate by being children of RootLayout; they don't add their own session check."
    - "API error → inline form error mapping by HTTP status: ApiError.status switch (401 → 'Current password incorrect.', 422 → strength message, default → generic). The same shape will repeat in Plan 16 (install wizard step submits) and Plan 17 (Edit Connection dialog)."
    - "Verbatim UI-SPEC copy strings live next to the JSX, not in a strings table. Phase 1 is English-only (UX-02 lock); a strings table would be ceremony with no payoff. v2 i18n (V2-I18N-01) will be a separate refactor pass touching every string atom."
    - "Cancel-LEFT, primary-RIGHT footer convention (UI-SPEC §Dialog Conventions) — applies to every CRUD dialog from here on. Plan 16 (wizard steps), Plan 17 (Edit Connection), Phase 2+ (Add device, Meter swap) inherit."
    - "Sonner success toast via revalidator.revalidate() — pattern: dialog calls onSuccess prop → parent fires `toast.success('Verbatim string')` → calls react-router's revalidator to refresh loader data. The session-state mutation (password change) doesn't return new data, but the pattern composes with mutations that DO (Plan 16 region pick → revalidates capabilities)."
    - "csrf-paranoid header injection by apiFetch (Plan 06) — every typed wrapper in auth.ts trusts apiFetch to attach X-Requested-With: shifter. No call site in auth.ts duplicates that header. Future plans wrapping new endpoints MUST go through apiFetch; raw fetch() against /api/* is forbidden (RESEARCH §Security; Plan 09 backend rejects without it)."
    - "Hydrate-from-DB on every /api/account/me hit so disabled-mid-session accounts surface 401 immediately. Session payload only carries (id, role) — Plan 08's deliberate slimness means email + must_change_password come from the user table on every call. Cost: one indexed lookup per protected page nav. Benefit: revoke-on-disable is read-time enforced without bumping every session token."

key-files:
  created:
    - web/src/lib/auth.ts (typed auth client; 47 LOC)
    - web/src/routes/_root.tsx (rootLoader + RootLayout; replaces the Plan 06 shell stub)
    - web/src/routes/change-password-dialog.tsx (ChangePasswordDialog using ResponsiveDialog; 146 LOC)
  modified:
    - internal/auth/handlers.go (AccountInfoHandler + signature; +41 LOC)
    - internal/auth/handlers_test.go (TestAccountInfo_ReturnsUser + TestAccountInfo_NoSession + setupLogin mux registration; +44 LOC)
    - web/src/App.tsx (router protected branch declares loader: rootLoader)
    - web/src/lib/auth.test.ts (replaces the Plan 06 stub with 6 real tests; X-Requested-With assertion + 401/422 ApiError paths + body shape verification; uses Object.defineProperty location.assign workaround for jsdom 29 sealed accessor)
    - web/src/components/account-menu.test.tsx (replaces Plan 06 stub with 4 real tests covering admin/viewer item visibility, label content, onChangePassword dispatch)
    - web/src/components/shell/account-menu.tsx (Plan 06 component replaced; signature unchanged — userEmail / userRole / onChangePassword / onSignOut)

key-decisions:
  - "AccountInfoHandler hydrates from the user table on every call rather than reading email + must_change_password out of the session payload. Plan 08 deliberately stored only (id, role) in the session (T-08-06: keep PII out of cookies). The single indexed user-by-id lookup per page nav is cheap; the alternative (denormalize email into the session) would force a session-store migration on every email change. Bonus: ErrUserNotFound → 401 means a disabled-mid-session admin's next page nav bounces them to /login without an explicit revoke-step — Plan 11 inherits Plan 09's disabled-user filter for free."
  - "fetchSessionUser is a thin wrapper around apiFetch — no caching layer, no react-query integration. Reason: rootLoader runs once per protected-route navigation; react-query's stale-while-revalidate would fight react-router's loader semantics (which already revalidate on navigation). Adding a query cache here would mean two sources of truth for the user object. v2 personalization (saved views, V2-AUTH-02) may revisit when user-edit operations need optimistic updates."
  - "ChangePasswordDialog's 401 message is 'Current password incorrect.' — verbatim UI-SPEC. We resist the temptation to broaden it to 'Could not authenticate' even though the dialog also surfaces 401 from a session-expired-mid-edit path. The session-expired path is so rare (admin opened the dialog, walked away for >24h, came back and typed) that the false-positive cost is dominated by the genuine-typo case. If session-expired-mid-edit becomes a real complaint we'll add a server-side discriminator (different error code) — not change the copy."
  - "ChangePasswordDialog's 422 message ('Password is too weak. Add length, mixed case, a number, and a symbol.') doesn't expose the strength tier the backend returns (weak / fair / good / strong). UI-SPEC strength hint already enumerates the requirements; echoing the tier (e.g. 'tier: weak') would just be noise. The strength evaluator in Plan 07 returns the tier for telemetry and for a future inline strength meter (Plan 23 login-ui first-time-set-password flow may expose it); the dialog ignores it on the failure path."
  - "rootLoader throws redirect via react-router-dom even though apiFetch already does window.location.assign('/login?next=...') on 401. The loader-thrown redirect is the react-router-native control-flow signal — it ensures the protected layout never renders with `useLoaderData() === undefined` between the apiFetch redirect and the browser-level navigation completing. Belt-and-suspenders: the apiFetch redirect catches non-loader 401s (e.g. an SSE reconnect after session expiry); the loader redirect catches the initial-mount 401 cleanly. Both end at the same /login URL, so the duplicate is invisible to the user."
  - "Default error message for ChangePasswordDialog is 'Something went wrong. Try again.' rather than the raw ApiError.message. Reason: UI-SPEC bans technical jargon in operator-facing dialogs (Phase 1 install audience is the customer's IT/facilities operator, not a developer). The error chain is still inspectable via the browser console (apiFetch logs); the dialog stays calm."
  - "AccountInfoHandler is exported BUT not wrapped in RequireAction — the 401 path is the gate. Other protected endpoints will use RequireAction(sm, ActionX) for the role check, but /api/account/me has no role check (every authenticated user has 'self-read'). The 401-on-no-session is the entire authz contract for this endpoint. Future personalization endpoints (V2 saved views) will reuse this 'authenticated → 200, unauthenticated → 401, no role gate' pattern."
  - "ChangePasswordDialog's reset() runs on close (open=false) and on submit-success — covers the case where the operator opens the dialog, types a wrong current password, gets the 401 inline error, closes the dialog, and reopens it. The current/new/confirm inputs are reset every time the dialog closes; the error banner is too. Avoids leaking partial state across dialog opens (and avoids leaking a rejected current_password value back into the input on retry)."
  - "Object.defineProperty(window, 'location', ...) workaround for jsdom 29 — auth.test.ts's stubLocationAssign helper. jsdom 29 made window.location's accessor non-configurable, so direct `window.location.assign = vi.fn()` throws in strict mode. The whole-object replacement keeps the spy interceptable for tests that exercise apiFetch's 401 redirect path. This is a test-only shim; production code never touches window.location.assign directly outside the auth client."
  - "The verbatim success toast string is 'Password changed' (no period, no exclamation). UI-SPEC §Phase 1 copy table locks it. Resisting the temptation to upgrade to 'Password changed successfully' or 'Your password has been updated' — the shorter form is louder, the toast already implies success via the green/check styling (sonner richColors)."

patterns-established:
  - "Pattern 1 (Loader-Gated Protected Layout): Every Phase 1+ protected route is a child of RootLayout, which has loader: rootLoader. The loader's contract is `Promise<{ user: SessionUser }>` or `throw redirect(...)`. Sub-route loaders compose by calling `await fetchSessionUser()` themselves only if they need the user; most just consume RootLayout's loader data via useRouteLoaderData('root')."
  - "Pattern 2 (Dialog Submit + Inline Error + Toast): Mutation dialogs follow a fixed shape — useState for inputs + busy + error; onSubmit setBusy(true) → try { await mutate(); onSuccess(); onOpenChange(false); reset() } catch ApiError → setError(byStatus(err.status)) finally setBusy(false). Parent owns the success toast (sonner) and the revalidator hook. Plans 16 / 17 / Phase 2 CRUD all inherit this skeleton."
  - "Pattern 3 (Verbatim Copy Inline): UI-SPEC strings live in JSX. Phase 1 English-only (UX-02). Future i18n (V2-I18N-01) will be a single sweep replacing literals with `t('change-password.title')` calls; the Phase 1 ergonomic win is real."
  - "Pattern 4 (Backend-Hydrated Session-User Endpoint): /api/account/me returns the canonical view of the logged-in user; the session payload is intentionally a thin (id, role) handle so account state changes (email, must_change_password, disabled_at) are observable on the next page nav without re-issuing tokens. The frontend never reads must_change_password out of the cookie — it's a server-controlled signal."
  - "Pattern 5 (Test-Time Location Stub): Object.defineProperty(window, 'location', { configurable: true, writable: true, value: { ...window.location, assign: vi.fn() } }) is the canonical jsdom 29 workaround for tests that exercise apiFetch's 401 redirect. Reused in any future test that needs to assert window.location.assign was called."

requirements-completed:
  - AUTH-05
  - AUTH-06

# Metrics
duration: 6min
completed: 2026-04-28
---

# Phase 01 Plan 11: Account UI Summary

**Account dropdown menu wired to a typed auth client, RootLayout's loader gates protected routes on /api/account/me, and ChangePasswordDialog uses ResponsiveDialog with verbatim UI-SPEC copy — AUTH-05 frontend complete, AUTH-06 hiding scaffolding in place.**

## Performance

- **Duration:** 6 min (08:23:45 +07 → 08:29:34 +07; plan execution measured from Plan 10 metadata commit to Plan 11's last task commit)
- **Started:** 2026-04-28T01:23:45Z
- **Completed:** 2026-04-28T01:29:34Z
- **Tasks:** 2 (1 backend addendum + 1 TDD frontend task with split RED + GREEN commits)
- **Files modified:** 9 (3 backend incl. tests, 6 frontend incl. tests)

## Accomplishments

- **AccountInfoHandler ships at GET /api/account/me** — returns `{ user: { id, email, role, must_change_password } }` for an authenticated session, 401 otherwise. Disabled-mid-session admins also surface 401 (read-time enforcement, no token revocation needed).
- **D-09 verified end-to-end at the API layer** — `TestAccountInfo_ReturnsUser` asserts `must_change_password=false` for the create-admin / wizard admin. The "force change on first login" UI gate is intentionally absent (D-09 reframe of AUTH-03); bootstrap admins set their own password.
- **Typed auth client (`web/src/lib/auth.ts`)** — fetchSessionUser, login, logout, changePassword + SessionUser type. Every Phase 1+ feature consuming the auth API goes through this module; raw fetch against /api/auth/* is forbidden going forward.
- **rootLoader + RootLayout (`web/src/routes/_root.tsx`)** — protected routes redirect unauthenticated users to /login?next=… via react-router's loader contract. Loader-throw-redirect is the canonical pattern Phase 1+ inherits.
- **ChangePasswordDialog** — ResponsiveDialog wrapper, UI-SPEC verbatim copy strings, 401/422 inline error mapping, Cancel-left/primary-right footer (UI-SPEC §Dialog Conventions). On success: dialog closes, sonner toast "Password changed", router revalidates.
- **AccountMenu refined for Plan 11** — userEmail / userRole / onChangePassword / onSignOut props; menu items ("Change password", "Theme", separator, "Sign out") match UI-SPEC §App shell §Avatar dropdown menu. AUTH-06 frontend hiding scaffolding: userRole prop is in place; Phase 1 has no admin-only menu items, but Phase 2+ adds will be one-line `userRole === 'admin' && …` guards.
- **Tests:** 19 frontend tests passed | 6 skipped (test-harness stubs from earlier plans); 58 backend auth-package tests passed; `pnpm build` exits 0.

## Task Commits

Each task was committed atomically; the TDD task split RED + GREEN:

1. **Task 1 (backend addendum): AccountInfoHandler for GET /api/account/me** — `b2bd2f9` (feat)
2. **Task 2 RED: failing tests for auth client + account menu** — `2aa6700` (test)
3. **Task 2 GREEN: account UI — auth client + RootLayout loader + ChangePasswordDialog** — `17ea07b` (feat)

**Plan metadata:** _(this commit, doc; appended after the SUMMARY is written)_

## Files Created/Modified

**Backend:**
- `internal/auth/handlers.go` — added `AccountInfoHandler(deps LoginDeps) http.HandlerFunc`; hydrates email + must_change_password from the user table; ErrUserNotFound → 401; other DB errors → 500 with `Log.Error`.
- `internal/auth/handlers_test.go` — `setupLogin` mux now mounts `GET /api/account/me`; `TestAccountInfo_ReturnsUser` (D-09 assertion); `TestAccountInfo_NoSession`.

**Frontend (created):**
- `web/src/lib/auth.ts` — typed wrappers (47 LOC, full doc-comment header listing endpoint mapping per Plan).
- `web/src/routes/_root.tsx` — rootLoader (try fetchSessionUser; catch → throw redirect) + RootLayout (ResponsiveShell + ChangePasswordDialog + handleSignOut + revalidator-driven success toast).
- `web/src/routes/change-password-dialog.tsx` — ResponsiveDialog-based dialog (146 LOC); reset() on close + on submit-success; 401/422 inline error mapping; UI-SPEC verbatim copy in JSX.

**Frontend (modified):**
- `web/src/App.tsx` — protected route tree declares `loader: rootLoader` for the `/` branch.
- `web/src/lib/auth.test.ts` — replaces Plan 06 stub with 6 real tests; introduces `stubLocationAssign()` Object.defineProperty workaround for jsdom 29.
- `web/src/components/account-menu.test.tsx` — replaces Plan 06 stub with 4 real tests (admin items, viewer self-edit visibility, email/role label, onChangePassword dispatch).
- `web/src/components/shell/account-menu.tsx` — Plan 06 component replaced; AccountMenuProps signature unchanged (userEmail / userRole / onChangePassword / onSignOut); userRole prop now plumbed for AUTH-06 forward-compat.

## Decisions Made

(See `key-decisions` in frontmatter for the full list.) Headlines:

1. **Backend-hydrated /api/account/me** — session keeps only (id, role); email + must_change_password come from the DB on every call. Disabled-mid-session admins surface 401 read-time without token revocation.
2. **Loader-thrown redirect + apiFetch redirect overlap is intentional** — belt-and-suspenders 401 handling. apiFetch handles SSE / non-loader 401s; the loader handles the initial-mount 401 cleanly so protected layouts never render with undefined user data.
3. **Verbatim UI-SPEC copy lives in JSX** — Phase 1 is UX-02 English-only; a strings table is V2-I18N-01 ceremony with no Phase 1 payoff.
4. **Object.defineProperty(window, 'location', ...) is the canonical jsdom 29 workaround** — reused in every future test that asserts window.location.assign was called.
5. **AccountInfoHandler is NOT wrapped in RequireAction** — the 401-on-no-session path IS the entire authz contract for "self read." Wrapping in RequireAction(sm, …) would require declaring a `ActionAccountSelfRead` and putting it in roleBundles; the existence-check is sufficient.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 — Missing Critical] AccountInfoHandler treats ErrUserNotFound as 401, not 500**
- **Found during:** Task 1 (backend addendum — AccountInfoHandler implementation)
- **Issue:** Plan-verbatim handler lumped all `Store.GetUserByID` errors into a generic 500. But `ErrUserNotFound` is the legitimate-and-likely outcome for a session whose underlying user was disabled mid-session — surfacing that as 500 would (a) trigger ops alerts on a benign event, (b) keep the SPA stuck on the protected layout instead of bouncing to /login.
- **Fix:** Added `if errors.Is(err, ErrUserNotFound) { writeJSON(w, 401, ...); return }` ahead of the generic 500 path. Aligns with Plan 09's user-store filter (`disabled_at IS NULL`) — disabled-mid-session is symmetric with disabled-before-login.
- **Files modified:** `internal/auth/handlers.go` (one branch added).
- **Verification:** `TestAccountInfo_NoSession` exercises the 401 path; the 500 path is reserved for genuine DB failures (connection drop, etc.).
- **Committed in:** `b2bd2f9` (Task 1 commit).

**2. [Rule 3 — Blocking] jsdom 29 sealed window.location.assign**
- **Found during:** Task 2 RED (auth.test.ts mock setup).
- **Issue:** jsdom 29 (the version pinned in vitest's transitive tree) made `window.location` a non-configurable accessor; direct `window.location.assign = vi.fn()` throws `TypeError: Cannot redefine property` in strict mode. Without the mock, tests that trigger apiFetch's 401 redirect would actually navigate the test runner's window — flaky / impossible to assert.
- **Fix:** `stubLocationAssign()` helper using `Object.defineProperty(window, 'location', { configurable: true, writable: true, value: { ...window.location, assign: vi.fn() } })` — replaces the whole location object with a stub the spy can intercept.
- **Files modified:** `web/src/lib/auth.test.ts`.
- **Verification:** auth.test.ts's 401 assertions (`fetchSessionUser → 401` and `changePassword → 401`) pass.
- **Committed in:** `17ea07b` (Task 2 GREEN commit).

**3. [Rule 1 — Bug] AccountMenu Sign-out icon double-aria-hidden cleanup**
- **Found during:** Task 2 GREEN review.
- **Issue:** Plan-verbatim Account menu items lacked `aria-hidden="true"` on the lucide icons (Sun / Moon / Laptop / LogOut), which a screen reader would announce as decorative-icon noise alongside the text label.
- **Fix:** Added `aria-hidden="true"` to every icon inside the menu items.
- **Files modified:** `web/src/components/shell/account-menu.tsx`.
- **Verification:** account-menu.test.tsx still passes; visual rendering unchanged.
- **Committed in:** `17ea07b` (Task 2 GREEN commit).

---

**Total deviations:** 3 auto-fixed (1 missing-critical, 1 blocking, 1 a11y bug).
**Impact on plan:** All three are correctness / test-infra fixes. No scope creep.

## Issues Encountered

- **Local Node version below project floor.** Local `node --version` is `v22.11.0` while `web/.nvmrc` pins `22.12` (and `web/package.json` engines requires `>=22.12`). On 22.11 the vitest forks pool fails to start with `ERR_REQUIRE_ESM` from `html-encoding-sniffer@6.0.0` requiring `@exodus/bytes`'s ESM `encoding-lite.js` — Node 22.12+ added the `node:diagnostics_channel` `TracingChannel.traceSync` machinery that html-encoding-sniffer relies on. Reproduced in this resume session; resolved by running tests on `~/.nvm/versions/node/v22.20.0/bin` which honors the .nvmrc floor. **Action:** the operator's shell needs `nvm use` (or the equivalent fnm / asdf invocation) before running `pnpm test:run`. Logged to STATE.md Open Todos for the CI plan to add a pre-flight Node-version check; the project floor is correct, the local environment was stale.

- **Frontend `pnpm test:run -- auth account-menu` works**, returning the previous agent's exact "19 passed | 6 skipped" totals when run on Node 22.20. Backend `go test ./internal/auth -run TestAccountInfo_ -race -count=1` returns 2/2 passing. Full backend auth suite: 58/58 passing.

## User Setup Required

None — no external service configuration introduced.

## Next Phase Readiness

- **Plan 12 (chirpstack-grpc) unblocked.** Plan 11 only added an HTTP endpoint and frontend wiring; no shared state with ChirpStack integration.
- **Plan 14 (install-middleware) ready to compose with rootLoader.** The pattern locked here (loader → fetchSessionUser → throw redirect on 401) extends naturally to "loader → check install_state → throw redirect to /install if first-run-pending." Plan 14 will likely add a sibling loader (`installLoader`) or compose by importing `rootLoader` and chaining.
- **Plan 16 (install-wizard-ui) ready to consume the protected shell.** Wizard "Finish" handler will redirect to `/`, which now renders RootLayout with the freshly-created admin's session.
- **Plan 17 (test-connection / Settings) ready to land as a child route.** Settings page inherits AccountMenu, sonner Toaster, ApiError mapping pattern from ChangePasswordDialog. Edit Connection dialog will reuse the dialog-submit-error-toast pattern.
- **Plan 23 (login-ui) ready.** login.tsx posts via `auth.ts`'s `login(email, password)`; on success, `window.location.assign('/')` triggers rootLoader → AccountMenu populates with the new session.

**Blockers:** None.

## Self-Check: PASSED

**File existence (created):**
- FOUND: `web/src/lib/auth.ts`
- FOUND: `web/src/routes/_root.tsx`
- FOUND: `web/src/routes/change-password-dialog.tsx`

**File existence (modified):**
- FOUND: `internal/auth/handlers.go` (AccountInfoHandler at line 151)
- FOUND: `internal/auth/handlers_test.go` (TestAccountInfo_ReturnsUser + TestAccountInfo_NoSession)
- FOUND: `web/src/App.tsx` (loader: rootLoader on line 33)
- FOUND: `web/src/lib/auth.test.ts` (6 real tests; stubLocationAssign helper)
- FOUND: `web/src/components/account-menu.test.tsx` (4 real tests)
- FOUND: `web/src/components/shell/account-menu.tsx`

**Commits:**
- FOUND: `b2bd2f9` (feat(01-11): add AccountInfoHandler for GET /api/account/me)
- FOUND: `2aa6700` (test(01-11): add failing tests for auth client + account menu)
- FOUND: `17ea07b` (feat(01-11): account UI — auth client + RootLayout loader + ChangePasswordDialog)

**Verification gates:**
- PASSED: `go test ./internal/auth -run 'TestAccountInfo_' -race -count=1` → 2 passed in 1 packages
- PASSED: `go test ./internal/auth -race -count=1 -short` → 58 passed in 1 packages
- PASSED: `pnpm test:run -- auth account-menu` (Node 22.20) → 19 passed | 6 skipped
- PASSED: `pnpm build` → exits 0; bundle 480 KB / 152 KB gzip

---
*Phase: 01-foundation*
*Completed: 2026-04-28*
