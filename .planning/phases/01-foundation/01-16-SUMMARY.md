---
phase: 01-foundation
plan: 16
subsystem: install
tags: [react, vite, install-wizard, shadcn, stepper, react-router, lazy-load, regions, thailand-default]

requires:
  - phase: 01-foundation
    plan: 06
    provides: Stepper component, ResponsiveDialog primitive, lib/api apiFetch with X-Requested-With CSRF header, AuthLayout route shell
  - phase: 01-foundation
    plan: 11
    provides: rootLoader pattern + ApiError + redirect-on-401 conventions
  - phase: 01-foundation
    plan: 14
    provides: Region catalog reference (REGIONS frontend hardcode mirrors install/regions.go entries)
  - phase: 01-foundation
    plan: 15
    provides: 6 install endpoints (GET /api/install/state, POST /api/install/step/1..4, POST /api/install/finish) — UI calls these
provides:
  - web/src/lib/install.ts — typed install client (fetchInstallState, postStep1..4, postFinish, REGIONS catalog, Region/InstallState types)
  - web/src/routes/install/index.tsx — wizard shell branching on state.CurrentStep (1..5); 410 → /login navigate; finish → /login navigate
  - web/src/routes/install/admin-step.tsx — step 1 form; password-confirm match check; 422 → "Password is too weak" inline error
  - web/src/routes/install/chirpstack-step.tsx — step 2 form; v3_detected → destructive Alert (UI-SPEC verbatim); grpc_unreachable → contextual error
  - web/src/routes/install/region-step.tsx — step 3 picker; AS923-2 default for Thailand operator (PITFALLS §8); helper hint conditional on selection
  - web/src/routes/install/identity-step.tsx — step 4 form; system-default timezone via Intl heuristic; metric/imperial radio
  - web/src/routes/install/review-step.tsx — step 5 review (JSON-rendered drafts via <pre>) + Finish setup CTA
  - rootLoader install-state pre-check (web/src/routes/_root.tsx) — install incomplete → /install redirect before session check
  - App.tsx /install route lazy-loads InstallWizard via React.lazy + Suspense
affects:
  - 01-23-login-ui (finish navigates to /login; login-ui will own that screen)
  - 01-17-test-connection (settings page reuses ChirpStack form patterns; 5-step wizard not relevant)
  - 01-18-router-health (FirstRunGate + the 6 install endpoints stay mounted unchanged; this UI is the consumer)

tech-stack:
  added: []
  patterns:
    - "Lazy-loaded route shell: install wizard is React.lazy(() => import('@/routes/install')) wrapped in <Suspense fallback={null}> at App.tsx. Keeps the wizard's 5 step components out of the protected-app bundle (they never render once install is complete). Cost: one extra HTTP/2 chunk on the public AuthLayout. Pattern extends to any future post-install full-screen flow."
    - "Wizard shell pattern: useEffect-driven state.CurrentStep branching rather than nested routes. Plan 06 Stepper renders the 5-step indicator; the 5 step components are sibling files under routes/install/* with a uniform onAdvance() callback that re-fetches /api/install/state. Avoids react-router nested-route plumbing for a flow where step transitions are server-driven (the backend's current_step is the source of truth)."
    - "rootLoader composition: install-state pre-check BEFORE session-check. fetchInstallState() returns null on 410 (install completed) → continues to session check. Non-null state → throw redirect('/install'). Network error on the install probe → falls through to session check (FirstRunGate catches missing-admin condition on the /api/account/me call). Belt-and-suspenders with the backend FirstRunGate."
    - "Dialog-Submit-Error pattern (Plan 11 lock) reused per step: useState(inputs + busy + error); onSubmit setBusy(true) → try { await postStepN(); onAdvance() } catch ApiError → setError(byStatus(err.status, err.body)) finally setBusy(false). Single-place error mapping per step keeps the wizard handlers testable in isolation."
    - "Step-2 v3 detection: catch ApiError where status === 422 AND body.error === 'v3_detected' → setV3(true) (rendered as destructive Alert with UI-SPEC verbatim copy). Other 422 errors fall through to generic error state. Plan 15 backend error envelope (`{error, detail?}`) is the wire contract."
    - "Region picker hardcodes the catalog inline (REGIONS const in install.ts) rather than fetching from /api/install/regions. Matches RESEARCH §Pattern 13 example; saves a request round-trip on the most-visited step; a Phase 7 regulator-versioned catalog promotes to a fetched endpoint."

key-files:
  created:
    - web/src/lib/install.ts
    - web/src/routes/install/index.tsx
    - web/src/routes/install/admin-step.tsx
    - web/src/routes/install/chirpstack-step.tsx
    - web/src/routes/install/region-step.tsx
    - web/src/routes/install/identity-step.tsx
    - web/src/routes/install/review-step.tsx
  modified:
    - web/src/App.tsx (lazy-load InstallWizard at /install)
    - web/src/routes/_root.tsx (rootLoader install-state pre-check)
    - web/src/routes/install/region-step.test.tsx (replaced describe.skip stub with 3 passing tests)

key-decisions:
  - "REGIONS catalog hardcoded inline in lib/install.ts. Plan 18 will mount GET /api/install/regions (Plan 14 backend already exposes the catalog), but Phase-1 UI hardcodes the same 8 entries. Saves a round-trip on the most-visited wizard step (step 3); the test file specifically expects the inline catalog (mocks @/lib/install). A Phase-7 regulator-versioned catalog can promote to /api/install/regions without breaking the wizard's structure — only the data source changes."
  - "rootLoader install-state pre-check is belt-and-suspenders with FirstRunGate (Plan 14). The backend's gate already redirects HTML / → /install (307) and returns 409 install_required for /api/* requests. The frontend pre-check makes the SPA-internal navigation (e.g., Plan 23 login → /) consistent: a navigation that doesn't hit a fresh HTML round-trip still bounces to /install. Cost: one /api/install/state call per protected-route load; cached by the browser within a tab session."
  - "Step components own their own error mapping. Each step's submit handler catches ApiError, inspects status + body.error, and renders an inline Alert. No central error boundary. Keeps the per-step UX precise (step 2's grpc_unreachable copy is different from step 4's invalid_timezone copy) and matches Plan 11's Dialog-Submit-Error pattern lock. Future steps that need shared error copy can pull strings into a constants module without restructuring."
  - "Step 2 maps 422 v3_detected separately from generic 422 errors. The destructive Alert is a UI-SPEC requirement (verbatim copy) — generic error rendering would fail acceptance. The setV3(true) state is reset on every submit so the operator can correct the URL and try again without page reload."
  - "Wizard shell uses useEffect + state.CurrentStep branching, not react-router nested routes. The 5 steps share state (the InstallState fetched once and passed to ReviewStep) and the transition is server-driven (backend's current_step). Nested routes would require a <Outlet /> + per-step loaders + the same onAdvance callback indirection. Trade-off: deep-linking to a specific step (e.g., /install/step/3) is impossible — but that's intentional, the operator MUST flow through steps in order to validate inputs incrementally."
  - "InstallState field names use Go-export casing (CurrentStep, Step1Admin) verbatim from Plan 15's pgx-scanned struct. Avoids a JSON-tag layer at the Plan 15 boundary; the SPA TypeScript types match the Go struct field names 1:1. If Phase 2+ moves to JSON tags, lib/install.ts updates in lockstep — but Phase 1's `json.Marshal` of a struct without tags emits exported field names exactly as-is."
  - "ReviewStep renders drafts via JSON.stringify inside <pre>{...}</pre>. T-16-01 mitigation: React + <pre> auto-escapes; embedded HTML in display name or address is text, not interpreted. T-16-02 (password hash visible in review) is plan-accepted residual: operators see Argon2id PHC string; non-reversible."
  - "App.tsx lazy-loads InstallWizard via React.lazy + Suspense fallback={null}. The wizard never renders once install is complete (FirstRunGate redirects /install → 200 only when install is incomplete; otherwise 410 → /login navigate). Lazy load keeps the post-install bundle smaller. The 'fallback={null}' choice over a loading spinner is intentional: the wizard chunk is small (5 step components, no charts/maps), the extra spinner introduces flash on a fast load."

requirements-completed:
  - INST-01
  - INST-02
  - INST-03
  - INST-04
  - INST-05
  - UX-01

duration: 5min
completed: 2026-04-28
---

# Phase 01 Plan 16: Install Wizard UI Summary

**Five-step install wizard at `/install` — admin → ChirpStack → region → identity → review → finish — built on Plan 06's Stepper + shadcn primitives + Plan 15's 6 endpoints. AS923-2 pre-selected for the Thailand operator base (PITFALLS §8); ChirpStack v3 rejection renders a destructive Alert with UI-SPEC verbatim copy. After finish success, the wizard navigates to /login (Plan 23 owns the login screen). 22 frontend tests pass (3 net-new region-step tests; 3 stubs replaced).**

## Performance

- **Duration:** ~5 min
- **Started:** 2026-04-28T02:41:02Z
- **Completed:** 2026-04-28T02:45:49Z
- **Tasks:** 2 / 2
- **Commits:** 2 (1 feat, 1 test)
- **Files created:** 7
- **Files modified:** 3

## Task Commits

1. **Task 1: install client + wizard shell + 5 step components + App lazy-load + rootLoader pre-check** — `9227289` (feat)
2. **Task 2: 3 RegionStep tests (Thailand default, button label, postStep3 invocation)** — `a42fdf2` (test)

**Plan metadata commit:** _pending — created at end of plan_

## Wizard Route Structure

```
/install (AuthLayout, public, FirstRunGate-whitelisted on backend)
  └── InstallWizard (lazy-loaded via React.lazy + Suspense)
       ├── on mount: fetchInstallState()
       │    ├── 410 → navigate('/login', { replace: true })
       │    └── 200 → setState(installState)
       │
       ├── Stepper (Plan 06) — visual indicator currentIndex = state.CurrentStep - 1
       │
       └── Active step body (branches on state.CurrentStep):
            ├── 1: <AdminStep onAdvance={reload} />
            ├── 2: <ChirpStackStep onAdvance={reload} />
            ├── 3: <RegionStep onAdvance={reload} />     ← AS923-2 pre-selected
            ├── 4: <IdentityStep onAdvance={reload} />   ← timezone defaults to system
            └── 5: <ReviewStep state={state} onComplete={navigate('/login')} />

reload = fetch /api/install/state again → setState(installState) (or navigate /login if 410)
```

## install.ts Client API

```ts
// State (null when 410 Gone)
export async function fetchInstallState(): Promise<InstallState | null>

// Step submissions — each returns { current_step: number } (or augmented)
export const postStep1 = (b: Step1Body)
export const postStep2 = (b: Step2Body)   // returns { current_step, chirpstack_version }
export const postStep3 = (b: { name: string })
export const postStep4 = (b: Step4Body)
export const postFinish = ()              // returns { ok: true }

// Region catalog — 8 entries; AS923-2 carries default_for_country: 'TH'
export const REGIONS: Region[]

// Types
export interface InstallState {
  CurrentStep: number
  StartedAt: string
  CompletedAt: string | null
  Step1Admin: unknown
  Step2ChirpStack: unknown
  Step3Region: unknown
  Step4Identity: unknown
}
export interface Step1Body { email: string; name: string; password: string }
export interface Step2Body {
  mode: 'bundled' | 'external'
  grpc_url: string
  api_token: string
  mqtt_url: string
  mqtt_user?: string
  mqtt_password?: string
}
export interface Step4Body {
  display_name: string
  address?: string
  timezone: string
  units: 'metric' | 'imperial'
  logo_path?: string
}
export interface Region {
  name: string
  display: string
  common_name: string
  group: 'asia' | 'europe' | 'americas' | 'oceania' | 'india'
  default_for_country?: string
  note?: string
}
```

## REGIONS Catalog (frontend hardcode — mirrors internal/install/regions.go)

| name      | display                       | group     | default_for_country |
| --------- | ----------------------------- | --------- | ------------------- |
| as923     | AS923-1                       | asia      | —                   |
| as923_2   | AS923-2 (Thailand)            | asia      | TH                  |
| as923_3   | AS923-3                       | asia      | —                   |
| as923_4   | AS923-4                       | asia      | —                   |
| eu868     | EU868 (Europe)                | europe    | —                   |
| us915_0   | US915 sub-band 1 (ch 0-7)     | americas  | —                   |
| au915_0   | AU915 sub-band 1              | oceania   | —                   |
| in865     | IN865 (India)                 | india     | —                   |

## Verbatim Copy Strings (UI-SPEC)

| Surface                                | String                                                                                                          | Source                                  |
| -------------------------------------- | --------------------------------------------------------------------------------------------------------------- | --------------------------------------- |
| Wizard h1                              | "Set up Shifter"                                                                                                | UI-SPEC §Install wizard                 |
| Wizard subtitle                        | "Five quick steps and you're ready to monitor."                                                                 | UI-SPEC                                 |
| Step 1 h2                              | "Create the admin account"                                                                                      | UI-SPEC                                 |
| Step 1 subtitle                        | "This account has full control of Shifter. You can add more users later."                                       | UI-SPEC                                 |
| Step 1 password helper                 | "At least 12 characters with mixed case, a number, and a symbol."                                               | UI-SPEC                                 |
| Step 1 weak-password error             | "Password is too weak. Add length, mixed case, a number, and a symbol."                                         | UI-SPEC                                 |
| Step 1 password mismatch               | "Passwords don't match."                                                                                        | UI-SPEC                                 |
| Step 2 h2                              | "Connect to ChirpStack"                                                                                         | UI-SPEC                                 |
| Step 2 subtitle                        | "Shifter wraps your ChirpStack v4 server. Choose how you want it deployed."                                     | UI-SPEC                                 |
| Step 2 v3 alert title                  | "Shifter doesn't support ChirpStack v3"                                                                         | UI-SPEC verbatim                        |
| Step 2 v3 alert body                   | "We detected ChirpStack v3 at this URL. Shifter requires v4 or newer. Upgrade ChirpStack and try again."        | UI-SPEC verbatim                        |
| Step 2 mode option (bundled)           | "Bundled — Shifter installs ChirpStack for you"                                                                 | UI-SPEC                                 |
| Step 2 mode option (external)          | "External — connect to an existing ChirpStack"                                                                  | UI-SPEC                                 |
| Step 2 button (idle / loading)         | "Next" / "Testing connection…"                                                                                  | UI-SPEC                                 |
| Step 3 h2                              | "Choose your LoRaWAN region"                                                                                    | UI-SPEC                                 |
| Step 3 subtitle                        | "This sets the default frequency plan for new gateways. You can override per-gateway later."                    | UI-SPEC                                 |
| Step 3 Thailand hint                   | "We pre-selected AS923-2 because the install address is in Thailand."                                           | UI-SPEC + PITFALLS §8                   |
| Step 4 h2                              | "Tell us about your install"                                                                                    | UI-SPEC                                 |
| Step 4 subtitle                        | "This appears in the topbar and on every report you export."                                                    | UI-SPEC                                 |
| Step 4 display-name helper             | `What your team calls this site, e.g. "Acme Water Co."`                                                         | UI-SPEC                                 |
| Step 5 h2                              | "Review and finish"                                                                                             | UI-SPEC                                 |
| Step 5 subtitle                        | "Confirm everything looks right. You can change any of this in Settings later."                                 | UI-SPEC                                 |
| Step 5 button (idle / loading)         | "Finish setup" / "Finishing setup…"                                                                             | UI-SPEC                                 |

## Plan 23 Login Navigate Target

`ReviewStep.onComplete = () => navigate('/login', { replace: true })` is the post-finish navigation. Plan 23 (login-ui) owns the `/login` route element; replaces the placeholder `<div>Login screen — Plan 23</div>` with the real shadcn-Card login form. The wizard exits with `replace: true` so the operator can't browser-back into the now-410-Gone wizard route.

The 410-Gone path also navigates to /login: `fetchInstallState()` returns null on 410 (install completed) → InstallWizard's `useEffect` calls `navigate('/login', { replace: true })`. Same target, same rationale.

## Acceptance Criteria Verification

- ✅ `web/src/lib/install.ts` exports `fetchInstallState`, `postStep1..4`, `postFinish`, `REGIONS`
- ✅ `REGIONS` array has exactly 8 entries; `as923_2` has `default_for_country: 'TH'`
- ✅ `web/src/routes/install/index.tsx` renders `Stepper` and switches on `state.CurrentStep` between 1-5
- ✅ `admin-step.tsx` form has fields with ids `adm-email`, `adm-name`, `adm-pw`, `adm-confirm`
- ✅ Strength helper text exact: "At least 12 characters with mixed case, a number, and a symbol."
- ✅ v3 alert title exact: "Shifter doesn't support ChirpStack v3"
- ✅ v3 alert body exact (matches UI-SPEC)
- ✅ Region step initial state is `'as923_2'`
- ✅ Region step shows Thailand hint when selected==='as923_2'
- ✅ Finish button labels: idle "Finish setup", loading "Finishing setup…"
- ✅ `cd web && pnpm build` exits 0
- ✅ Region step has 3 tests (default selection, button label, postStep3 invocation)
- ✅ `describe.skip` removed from region-step.test.tsx
- ✅ Tests mock `@/lib/install`'s `postStep3` (no network)
- ✅ `pnpm test:run` exits 0 — 22 passed, 3 still-skipped (other plans' stubs)

## Decisions Made

See `key-decisions` in frontmatter for the canonical list. Highlights:

- **REGIONS hardcoded inline** in `lib/install.ts` — saves a round-trip on the most-visited wizard step; matches RESEARCH §Pattern 13 example; test file specifically expects the inline catalog.
- **rootLoader install-state pre-check** — belt-and-suspenders with backend FirstRunGate; ensures SPA-internal navigation also bounces to /install when install is incomplete.
- **Step components own their own error mapping** — per-step UX is precise (step 2's grpc_unreachable copy ≠ step 4's invalid_timezone copy); matches Plan 11's Dialog-Submit-Error lock.
- **Wizard shell uses useEffect + state.CurrentStep branching, not nested routes** — backend's current_step is the source of truth; nested routes would add Outlet/loader plumbing for no UX win.
- **InstallState field names use Go-export casing** verbatim from Plan 15's pgx-scanned struct — no JSON tag layer; types match Go struct 1:1.
- **App.tsx lazy-loads InstallWizard via React.lazy + Suspense fallback={null}** — wizard chunk excluded from the post-install bundle; null fallback avoids spinner-flash on fast loads.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] rootLoader install-state pre-check beyond Task 1 instructions**

- **Found during:** Reading the plan's `<must_haves>` truths — "rootLoader composes install-state-check before session-check: if /api/install/state returns valid state (not 410), redirect to /install".
- **Issue:** Task 1's `<action>` step 8 only modified `App.tsx` to lazy-load the wizard at `/install`. It didn't modify `_root.tsx` to add the install-state pre-check, even though the `<files_modified>` frontmatter listed `_root.tsx` AND the must_haves required the loader composition. Without the pre-check, an SPA-internal navigation from /login → / (after a stale-cookie-induced login) would hit RootLayout's session loader, which would 401 and bounce to /login again — instead of bouncing to /install where the operator is supposed to be. The backend FirstRunGate handles fresh HTML loads (307 redirect), but SPA-internal navigation skips the gate.
- **Fix:** Added install-state pre-check at the top of `rootLoader` in `_root.tsx`. Calls `fetchInstallState()`; if it returns non-null (install in progress), throws `redirect('/install')`. On network failure, falls through to the session check (FirstRunGate catches missing-admin on /api/account/me). Re-throws react-router `redirect` Response objects so they propagate.
- **Files modified:** `web/src/routes/_root.tsx`
- **Verification:** `pnpm build` exits 0; `pnpm test:run` exits 0 (auth.test.ts still passes — the existing tests don't exercise the new install-state path, but they don't regress either).
- **Committed in:** `9227289` (Task 1)

**2. [Rule 1 - Bug] Plan-verbatim Step 4 invalid_units / invalid_timezone errors had no inline mapping**

- **Found during:** Authoring `identity-step.tsx`.
- **Issue:** Plan-verbatim only handled the happy path + a generic "Saving…" busy state. Backend Plan 15 returns 422 with `body.error` of `invalid_timezone` / `invalid_units` / `missing_fields` for various validation failures. Without per-error mapping, the operator would see the form silently swallow errors (busy → idle, no message) and not know what to fix.
- **Fix:** Added `try { ... } catch (err)` block in `submit()` that inspects `ApiError`'s status + body.error and renders inline `<Alert variant="destructive">` with appropriate copy: "That timezone isn't recognized. Use an IANA name like Asia/Bangkok." for invalid_timezone, "Pick metric or imperial." for invalid_units, "Please fill in every required field." for missing_fields. Mirrors the pattern in admin-step / chirpstack-step / region-step.
- **Files modified:** `web/src/routes/install/identity-step.tsx`
- **Verification:** Build passes; the rendering path is structurally identical to the other step components which already had inline error mapping.
- **Committed in:** `9227289` (Task 1)

**3. [Rule 1 - Bug] Plan-verbatim Step 3 had no error mapping for unknown_region**

- **Found during:** Authoring `region-step.tsx`.
- **Issue:** Plan-verbatim only handled the happy path. Backend Plan 15 returns 422 unknown_region if the submitted region name fails the RegionByName whitelist (T-14-04 mitigation). Without inline error mapping, an unknown_region response would silently fail. Note: in practice this can't happen with the hardcoded REGIONS catalog (every value is whitelisted), but defensive UX guards against future catalog drift.
- **Fix:** Added `try / catch (err: ApiError)` mapping 422 → "That region isn't available. Pick another." Generic 500/network errors fall through to "Something went wrong. Try again."
- **Files modified:** `web/src/routes/install/region-step.tsx`
- **Verification:** Tests cover the happy path (postStep3 mocked to resolve); unknown-region path is rendering-only (no test added because the REGIONS catalog mirrors the backend whitelist 1:1; future Phase-7 regulator-versioned catalog drift would surface as a network mismatch caught by integration tests).
- **Committed in:** `9227289` (Task 1)

---

**Total deviations:** 3 auto-fixed (1 Rule 2 missing-critical, 2 Rule 1 bug). All deviations strengthen UX correctness without changing the public API contracts. Plan-15 acceptance is unaffected.

## Issues Encountered

- **Local Node 22.11 below the .nvmrc=22.12 floor.** Reproduces Plan 11's open todo. All `pnpm` commands ran via `~/.nvm/versions/node/v22.20.0/bin/node` + corepack pnpm 10.33.2 (per the plan's `<sequential_execution>` note). `vitest --run` exits 0; `vite build` exits 0. The `Bootstrap docs — Node version pre-flight check` open todo from Plan 11 still applies; whichever plan lands the developer onboarding doc should add the explicit pre-flight.
- **The plan's vitest filter syntax `pnpm test:run -- -t 'install|region'` doesn't filter via vitest 4 — the `-t` flag is consumed by `pnpm` script-pass-through.** Used `pnpm vitest --run src/routes/install/region-step.test.tsx` instead to scope the test run; same outcome (3 region-step tests pass).
- **The placeholder route element at `/login` is still `<div>Login screen — Plan 23</div>`.** The post-finish navigate target is correct (/login), but Plan 23 hasn't replaced the placeholder yet. Operators completing the wizard today land on the placeholder; this is intentional and tracked in Plan 06's known-stubs section.

## Known Stubs

| Stub                                                                          | File                          | Reason                                                              | Resolved by         |
| ----------------------------------------------------------------------------- | ----------------------------- | ------------------------------------------------------------------- | ------------------- |
| `<div>Login screen — Plan 23</div>` placeholder at `/login`                   | `web/src/App.tsx`             | Plan 23 ships the real login form; wizard finish navigates here     | 01-23-login-ui      |
| `<div>Settings — Plan 17</div>` placeholder at `/settings`                    | `web/src/App.tsx`             | Plan 11 (Account card) + Plan 17 (Test Connection) fill this        | 01-11, 01-17 (both done; placeholder pending Plan 11+17 final UI integration) |
| Hardcoded `installDisplayName="Shifter"` in `_root.tsx` (carried from Plan 06) | `web/src/routes/_root.tsx`    | Future plan wires `/api/install/identity` loader for live name      | Phase 2+            |

The wizard itself ships no stubs: every step component has functional submission, error mapping, and onAdvance/onComplete callbacks wired. The 3 placeholder stubs above are pre-existing from Plan 06 and don't block Plan 16's stated goal (5-step install wizard at /install consuming Plan 15's endpoints).

## Threat Surface Notes

All 5 entries in the plan's `<threat_model>` are mitigated by code shipped in this plan or upstream:

| Threat   | Mitigation                                                                                                      |
| -------- | --------------------------------------------------------------------------------------------------------------- |
| T-16-01 (XSS — display name on review)                | ReviewStep renders drafts via `<pre>{JSON.stringify(...)}</pre>`; React + <pre> auto-escapes (ASVS V5).        |
| T-16-02 (Information Disclosure — password hash in review JSON) | Plan-accepted residual; admin sees Argon2id PHC hash (non-reversible) in their own review. ASVS V8.       |
| T-16-03 (Tampering — malicious timezone)              | Plan 15 backend validates via `time.LoadLocation`; UI surfaces 422 invalid_timezone as inline error.            |
| T-16-04 (Spoofing — wizard shown after install completes) | `fetchInstallState()` returns null on 410; useEffect triggers `navigate('/login')` immediately. Plus FirstRunGate. |
| T-16-05 (Information Disclosure — api_token in network tab) | Plan-accepted; self-hosted single-tenant; operator types the token themselves. Server stores by REF (Plan 15). |

No new threat surface beyond the plan's register; no `## Threat Flags` section needed.

## User Setup Required

None for development. Reproducing the wizard locally requires the full Plan 03/14/15 backend running:

```bash
# Terminal 1 — start backend with FirstRunGate active (no admin yet):
just serve

# Terminal 2 — start SPA dev server:
cd web && pnpm dev

# Browser → http://localhost:5173
# Vite dev server proxies /api/* to backend; FirstRunGate redirects HTML / → /install
# (or App.tsx's rootLoader pre-check bounces SPA-internal navigation to /install)
# Wizard renders 5 steps; step 1 → step 2 → ... → step 5 → finish → /login
```

For test reproduction:

```bash
# Test the wizard end-to-end:
PATH=$HOME/.nvm/versions/node/v22.20.0/bin:$PATH corepack pnpm vitest --run src/routes/install/

# Test full SPA suite:
PATH=$HOME/.nvm/versions/node/v22.20.0/bin:$PATH corepack pnpm test:run
# Expected: 22 passed, 3 skipped (Plan 02 stubs in auth/login/account-menu tests)
```

## Next Phase Readiness

- ✅ INST-01 satisfied: GET /api/install/state on mount; render wizard or navigate /login on 410.
- ✅ INST-02 satisfied: step 1 form posts to /api/install/step/1 with email/name/password; backend hashes via Argon2id.
- ✅ INST-03 satisfied: 5-step wizard renders Stepper indicator + per-step component branching on `state.CurrentStep`.
- ✅ INST-04 satisfied: step 3 region picker pre-selects 'as923_2' on mount; Thailand helper hint conditional.
- ✅ INST-05 satisfied: step 2 catches 422 v3_detected and renders destructive Alert with UI-SPEC verbatim copy.
- ✅ UX-01: wizard is the canonical full-screen stepped exception per UI-SPEC §"Install wizard (full-screen stepped dialog)".
- ✅ Cancel-LEFT, primary-RIGHT footer convention not applicable here (wizard is full-screen, not dialog); each step's CTA is a single right-aligned primary button.
- ✅ Plan 17 (test-connection): can reuse the ChirpStack form patterns from `chirpstack-step.tsx` (mode radio, grpc_url, api_token, mqtt_url) when building the Settings → ChirpStack card. The Test Connection probe is post-install (settings page); the form fields are shared.
- ✅ Plan 23 (login-ui): owns `/login` route element; wizard's `onComplete` navigate target is `/login` with `replace: true`. Login form will read the operator's email/password (created in step 1) and POST /api/auth/login (Plan 09).
- ✅ Plan 18 (router-health): mounts the 6 install endpoints + the FirstRunGate — already completed in Plan 14/15. No new wiring needed.
- ✅ All UI-SPEC verbatim copy strings present (verified by grep).

## Self-Check: PASSED

Files verified to exist:
- FOUND: `web/src/lib/install.ts`
- FOUND: `web/src/routes/install/index.tsx`
- FOUND: `web/src/routes/install/admin-step.tsx`
- FOUND: `web/src/routes/install/chirpstack-step.tsx`
- FOUND: `web/src/routes/install/region-step.tsx`
- FOUND: `web/src/routes/install/identity-step.tsx`
- FOUND: `web/src/routes/install/review-step.tsx`
- FOUND: `web/src/routes/install/region-step.test.tsx` (real tests, no describe.skip)
- FOUND: `web/src/App.tsx` (modified — lazy-load /install)
- FOUND: `web/src/routes/_root.tsx` (modified — install-state pre-check)

Commits verified to exist:
- FOUND: `9227289` (Task 1 — install client + wizard shell + 5 step components + App lazy-load + rootLoader pre-check)
- FOUND: `a42fdf2` (Task 2 — 3 RegionStep tests; describe.skip removed)

Behavior verified:
- `cd web && pnpm build` → exit 0; emits `dist/assets/index-*.js` (482.91 kB) + lazy chunk `dist/assets/index-iiuy9VXU.js` (34.08 kB — wizard) + 25 self-hosted font files.
- `cd web && pnpm test:run` → 22 passed / 3 skipped (3 net-new region-step tests; Plan 02 stubs for auth/login/account-menu still skipped).
- `pnpm vitest --run src/routes/install/region-step.test.tsx` → 3 passed (Thailand default, Next button label, postStep3 invocation).
- `grep "Shifter doesn't support ChirpStack v3" web/src/routes/install/chirpstack-step.tsx` → 1 match.
- `grep "We pre-selected AS923-2 because the install address is in Thailand" web/src/routes/install/region-step.tsx` → 1 match.
- `grep "Finish setup" web/src/routes/install/review-step.tsx` → 1 match (idle label, also covers loading "Finishing setup…").
- `grep "default_for_country: 'TH'" web/src/lib/install.ts` → 1 match.
- `grep "describe.skip" web/src/routes/install/region-step.test.tsx` → 0 matches.

---
*Phase: 01-foundation*
*Plan: 16-install-wizard-ui*
*Completed: 2026-04-28*
