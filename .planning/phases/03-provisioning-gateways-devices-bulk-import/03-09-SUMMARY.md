---
phase: 03-provisioning-gateways-devices-bulk-import
plan: 09
subsystem: ui
tags: [react, react-router-dom, zod, tanstack-table, tanstack-query, shadcn, vite, bulk-import, deeplink]

# Dependency graph
requires:
  - phase: 02-foundation-app-shell
    provides: ResponsiveDialog/Stepper components, Device + MeteringPoint typed clients, swap-meter wiring
  - phase: 03-provisioning-gateways-devices-bulk-import-Plan-05
    provides: /api/imports/* (5 endpoints — upload/commit/list/get/template/errors.xlsx)
  - phase: 03-provisioning-gateways-devices-bulk-import-Plan-06
    provides: /api/devices filter/sort/page envelope + /api/devices/bulk-decommission
  - phase: 03-provisioning-gateways-devices-bulk-import-Plan-07
    provides: 5-step Add Device + AddDeviceDialog Stepper pattern
  - phase: 03-provisioning-gateways-devices-bulk-import-Plan-08
    provides: Gateway destructive AlertDialog pattern (decommission-gateway-dialog)
provides:
  - Devices list URL-state contract — deeplinkable filters/sort/pagination via react-router-dom v7 useSearchParams + zod
  - useDevicesSearch hook + devicesSearchSchema + devicesSearchToQuery (canonical wire mapping)
  - Bulk Decommission flow (BulkActionBar + BulkDecommissionDialog) wired to /api/devices/bulk-decommission
  - Bulk Import 3-step dialog (Upload → Preview → Commit) with semantic outcome chips + idempotency visibility (DEV-07)
  - /admin/imports list page + /admin/imports/:jobId detail page (D-36)
  - Sidebar Admin > Imports nav (admin-only)
  - useCurrentUser() helper (reads SessionUser via useRouteLoaderData('root'))
  - listDevicesFiltered() typed client + envelope unwrap fix for legacy listDevices()
affects: [phase-04-dashboard, phase-04-device-detail-page, phase-09-audit-ui]

# Tech tracking
tech-stack:
  added: []  # No new deps — used existing radix-ui Command/Popover/ToggleGroup
  patterns:
    - "URL state via react-router-dom v7 useSearchParams + zod safeParse fallback (NOT TanStack Router — 03-RESEARCH correction)"
    - "Multi-value URL params via URLSearchParams.append (never comma-separated)"
    - "Sticky bulk-action bar pattern (h-12 bg-secondary border-y)"
    - "Reusable per-row outcomes table component (exported from BulkImportDialog, reused in /admin/imports/:jobId)"
    - "Admin-only route guard via useCurrentUser + navigate('/') in useEffect (defense-in-depth)"
    - "TanStack Table manualPagination + manualSorting + manualFiltering for server-driven views"
    - "TanStack Query placeholderData: (prev) => prev for non-flashing pagination"
    - "Outcome chip → semantic token mapping (--success/--info/--warning/--destructive)"

key-files:
  created:
    - web/src/routes/devices/search-params.ts
    - web/src/routes/devices/search-params.test.ts
    - web/src/routes/devices/filter-toolbar.tsx
    - web/src/routes/devices/bulk-action-bar.tsx
    - web/src/routes/devices/bulk-decommission-dialog.tsx
    - web/src/routes/devices/bulk-decommission-dialog.test.tsx
    - web/src/routes/devices/bulk-import-dialog.tsx
    - web/src/routes/devices/bulk-import-dialog.test.tsx
    - web/src/routes/devices/index.test.tsx
    - web/src/routes/admin/imports/index.tsx
    - web/src/routes/admin/imports/index.test.tsx
    - web/src/routes/admin/imports/$jobId.tsx
    - web/src/routes/admin/imports/$jobId.test.tsx
    - web/src/lib/imports.ts
    - web/src/lib/use-current-user.ts
  modified:
    - web/src/lib/devices.ts (listDevicesFiltered + envelope unwrap on legacy listDevices + bulkDecommissionDevices)
    - web/src/routes/devices/index.tsx (full rewrite — TanStack Table + URL state + bulk-action bar)
    - web/src/App.tsx (root route id + /admin/imports/{,:jobId} routes)
    - web/src/components/shell/sidebar.tsx (Admin > Imports nav for admin role)
    - web/src/routes/metering-points/swap-meter-dialog.test.tsx (mock /api/devices envelope shape)

key-decisions:
  - "URL state via react-router-dom v7 useSearchParams + zod — NOT TanStack Router. The Phase 3 CONTEXT D-15 referenced TanStack Router; 03-RESEARCH §react-router-dom v7 URL State for Filters corrected this. The Vite SPA already imports react-router-dom v7; adding TanStack Router would have meant two routers, which 03-RESEARCH explicitly rejected."
  - "Multi-value site filter encoded as `site=uuid1&site=uuid2` (URLSearchParams.append + getAll). NEVER comma-separated — the Plan 03-06 backend reads `qv['site']` as a slice."
  - "Schema defaults stripped from URL on write (clean shareable URLs). The hook returns fully-typed state with defaults applied on read."
  - "listDevices() now unwraps the Plan 03-06 envelope so legacy callers (swap-meter-dialog) keep working. New filtered/paginated callers use listDevicesFiltered() returning the full envelope."
  - "BulkImport commit-step uses button-only confirm (NO type-to-confirm) per D-10. Idempotency means re-running the same file is safe; already_exists chips signal what will be skipped."
  - "Outcome chip → semantic token map: created → --success, already_exists → --info, invalid → --warning, failed → --destructive. This routes the visual urgency: skip (no action needed, blue) ≠ invalid (operator must fix, amber) ≠ failed (committed phase blew up, red)."
  - "Admin routes (/admin/imports/*) gate via useCurrentUser route guard PLUS server-side auth.RequireAction(ActionDeviceBulkImport). UI guard is convenience; server is authoritative (T-3-90 mitigation per threat model)."
  - "/admin/imports/:jobId detail page reuses BulkImport Step-2 OutcomesTable as a named export — single source of truth for the per-row outcomes UI."

patterns-established:
  - "Pattern: react-router-dom v7 URL state with zod — useSearchParams() + safeParse fallback to defaults; helper function (devicesSearchToQuery) for canonical wire mapping"
  - "Pattern: TanStack Table server-driven mode — state {rowSelection}, getRowId: r => r.id, enableRowSelection conditioned on role, manualPagination + manualSorting + manualFiltering, pageCount from server envelope"
  - "Pattern: BulkActionBar sticky strip surfacing N-selected + destructive primary + ghost Clear; component returns null when N=0"
  - "Pattern: AlertDialog destructive variant with no type-to-confirm (mirrors Plan 03-08 decommission-gateway-dialog)"
  - "Pattern: ResponsiveDialog stepped flow — index state in parent, body branches on `step`, footer renders Back/Next per step (now applied to BulkImport in addition to AddDevice)"
  - "Pattern: Outcome chip helper exported from BulkImport, reusable in detail page"

requirements-completed: [DEV-01, DEV-06, DEV-07, DEV-08, UX-03]

# Metrics
duration: 15 min
completed: 2026-05-11
---

# Phase 3 Plan 9: Devices URL-state filter UI + Bulk Decommission + Bulk Import 3-step + /admin/imports pages Summary

**Deeplinkable /devices via react-router-dom v7 useSearchParams + zod, sticky bulk-action bar with bulk-decommission AlertDialog, and the full 3-step Bulk Import dialog wired to /api/imports with semantic-token outcome chips + per-row idempotency visibility, plus the /admin/imports list and detail pages backing operator audit (D-36).**

## Performance

- **Duration:** 15 min
- **Started:** 2026-05-11T09:00:22Z
- **Completed:** 2026-05-11T09:15:31Z
- **Tasks:** 3
- **Files modified:** 20 (15 created, 5 modified)
- **Test coverage:** 122 vitest tests pass (+38 new tests across 6 new files: search-params, index, bulk-decommission-dialog, bulk-import-dialog, admin/imports/index, admin/imports/$jobId)
- **Build:** `pnpm tsc --noEmit` clean; `pnpm build` clean

## Accomplishments

- **DEV-01 frontend complete** — filter (site multi-select + activation Select + last_seen ToggleGroup + search) + sort (click-to-cycle 5 columns) + pagination (25/50/100 + prev/next) + URL deeplinking with browser back/forward.
- **DEV-06 frontend complete** — 3-step Bulk Import dialog (Upload → Preview → Commit) with file picker, template download, dry-run summary banner, per-row outcomes table with expandable reason, and button-only commit (D-10).
- **DEV-07 visibility** — idempotency surfaces via `already_exists` chip on the `--info` (blue) semantic token. Re-running the same file shows skipped rows without scaring the operator with red/error styling.
- **DEV-08 done** — `Download template (XLSX)` link on Step 1, `Download errors.xlsx` link on Step 2 and on the job detail page (only when invalid/failed > 0).
- **UX-03 maintained** — negative grep across 8 new files (admin/imports/*.tsx + bulk-import-dialog.tsx) returns no user-facing "tenant" or "application" copy. Only matches are in test assertions that explicitly check for absence, plus one doc-comment in bulk-import-dialog.tsx that names the vocab being avoided.
- **D-36 done** — `/admin/imports` list + `/admin/imports/:jobId` detail render row outcomes, summary card, expired banner, errors-xlsx download.

## Task Commits

1. **Task 1 — URL state hook + filter toolbar + sort/pagination** — `d089a13` (feat)
2. **Task 2 — Bulk-action bar + Bulk-decommission AlertDialog** — `6dff447` (feat)
3. **Task 3 — Bulk Import 3-step dialog + /admin/imports list+detail + sidebar nav** — `b9a3bd0` (feat)

## URL State Contract

```typescript
export const devicesSearchSchema = z.object({
  site:      z.array(z.string().uuid()).optional().default([]),
  status:    z.enum(['active', 'inactive', 'never_joined']).optional(),
  last_seen: z.enum(['24h', '7d', '30d', 'all']).optional().default('all'),
  q:         z.string().optional().default(''),
  page:      z.coerce.number().int().min(1).default(1),
  per_page:  z.union([z.literal(25), z.literal(50), z.literal(100)]).default(50),
  sort:      z.enum(['name','-name','dev_eui','-dev_eui','site','-site','last_seen','-last_seen','created_at','-created_at']).default('-last_seen'),
})
```

Wire format examples:

- Filter by two sites + active + 24h window:
  `/devices?site=11111111-1111-4111-8111-111111111111&site=22222222-2222-4222-8222-222222222222&status=active&last_seen=24h`
- Sorted by name desc, page 3, 100/page:
  `/devices?sort=-name&page=3&per_page=100`
- Default state (no params written):
  `/devices`

Defaults are stripped on write so shareable URLs stay short. Invalid values silently fall back to defaults (zod safeParse + spread; the operator sees a clean filter row rather than an exception — T-3-91 mitigation).

## "NOT TanStack Router" Correction Note

CONTEXT.md D-15 originally specified TanStack Router for URL state. 03-RESEARCH §react-router-dom v7 URL State for Filters reversed that decision: the SPA already imports react-router-dom v7 for routing, and adding a second router for URL state would split routing across two libraries. react-router-dom v7's `useSearchParams()` returns a `[URLSearchParams, SetURLSearchParams]` tuple identical in shape to TanStack Router's `useSearch`, and zod gives us the same validation guarantees. The negative grep in Task 1's acceptance criteria enforces this — `grep -E "@tanstack/react-router|useSearch.*tanstack" web/src/routes/devices/search-params.ts` must return 0 lines.

## Bulk Import 3-step UX Flow

1. **Upload (Step 1).** ResponsiveDialog `size="lg"` (sm:max-w-2xl). Heading "Upload your device list", info banner about CSV UTF-8 + XLSX recommendation, `Download template (XLSX)` outline button (anchor → `/api/imports/template.xlsx`), and a `<Input type="file" accept=".xlsx,.csv">`. Client-side guard: 5 MB cap + extension regex. Footer: Cancel (left), step indicator (center), "Validate file" primary (right). On click → POST /api/imports as multipart FormData via direct `fetch` (apiFetch is JSON-only); on success → advances to Step 2.
2. **Preview (Step 2).** Heading "Review what will happen", summary card with three semantic dots showing valid/already_exists/invalid counts, `Download errors.xlsx` link (only when invalid_count > 0), then a per-row TanStack-friendly table: chevron, row index (mono), Outcome chip, DevEUI (mono), name, truncated reason. Clicking the chevron expands the row to show the full reason. Footer: Back (left, returns to Step 1), step indicator, "Continue to commit" primary (right; disabled when valid_count === 0).
3. **Commit (Step 3).** Heading "Commit the import", description summarising counts, summary card with three semantic-colored count lines, idempotency note ("Re-running this same file is safe — Shifter uses DevEUI as the idempotency key"), and a button-only confirm "Import N devices" (D-10 mirrors D-15). On success → toast.success with created/skipped/failed counts, dialog closes, navigates to `/admin/imports/:job_id`.

## /admin/imports Route Registration + Admin Guard

```typescript
// App.tsx — both routes are children of the protected root layout
{ path: 'admin/imports',         element: <AdminImportsPage /> }
{ path: 'admin/imports/:jobId',  element: <ImportJobDetailPage /> }
```

Defense-in-depth: each page reads `useCurrentUser()` and `navigate('/', {replace: true})` in a useEffect when role !== 'admin'. The backend `auth.RequireAction(ActionDeviceBulkImport)` is the authoritative gate (T-3-90 mitigation — viewer hitting the URL directly still 403s at the API).

## Outcome Chip → Semantic Token Mapping

| Outcome (backend status string) | UI Token (`var(--*)`) | Rationale |
|---------------------------------|-----------------------|-----------|
| `created` / `valid`             | `--success` (green)   | The good path: row will be / was created. |
| `already_exists`                | `--info` (blue)       | Idempotent skip — operator action is "fine, leave it". NOT a problem. |
| `invalid`                       | `--warning` (amber)   | Validation rejected this row; operator can fix the input file and re-upload. |
| `failed`                        | `--destructive` (red) | Commit-phase failure (ChirpStack rejected, etc.); needs operator inspection. |

This mapping is implemented as the local `OutcomeChip` component in `web/src/routes/devices/bulk-import-dialog.tsx`, exported so the `/admin/imports/:jobId` detail page can reuse it.

## UX-03 Negative Grep Results

Plan acceptance:
```bash
grep -iE "(tenant|application)" web/src/routes/admin/imports/*.tsx web/src/routes/devices/bulk-import-dialog.tsx \
  | grep -v -E "(applicationId|applicationLogic|application/vnd)"
```

Hits across 8 new files:
- `admin/imports/index.test.tsx` — 3 lines (test assertion strings checking absence; not user-facing)
- `admin/imports/$jobId.test.tsx` — 3 lines (test assertion strings checking absence; not user-facing)
- `bulk-import-dialog.tsx` — 0 lines (the previous doc-comment was rewritten to name the constraint without using the forbidden tokens)

User-facing matches: **0**. Every match is in a test file checking that those words don't appear in the rendered DOM.

## Files Created/Modified

**Created (15):**
- `web/src/routes/devices/search-params.ts` — zod schema + useDevicesSearch hook + devicesSearchToQuery helper.
- `web/src/routes/devices/search-params.test.ts` — 11 vitest specs covering schema + hook behavior.
- `web/src/routes/devices/filter-toolbar.tsx` — Search + Site multi-select Command/Popover + Activation Select + Last-seen ToggleGroup + Clear filters + active-filter badge strip + "Showing N of M" counter.
- `web/src/routes/devices/bulk-action-bar.tsx` — sticky h-12 bar with N-selected + Decommission destructive + Export disabled + Clear ghost.
- `web/src/routes/devices/bulk-decommission-dialog.tsx` — AlertDialog with reason input + partial-success toast routing.
- `web/src/routes/devices/bulk-decommission-dialog.test.tsx` — 7 specs.
- `web/src/routes/devices/bulk-import-dialog.tsx` — 3-step ResponsiveDialog with OutcomesTable + OutcomeChip exports.
- `web/src/routes/devices/bulk-import-dialog.test.tsx` — 6 specs covering step transitions + commit nav + UTF-8 error surface.
- `web/src/routes/devices/index.test.tsx` — 8 specs for the rewritten devices page.
- `web/src/routes/admin/imports/index.tsx` — list page with status chip + totals split.
- `web/src/routes/admin/imports/index.test.tsx` — 4 specs.
- `web/src/routes/admin/imports/$jobId.tsx` — detail page with summary card + per-row outcomes + Download errors.xlsx + expired banner.
- `web/src/routes/admin/imports/$jobId.test.tsx` — 5 specs.
- `web/src/lib/imports.ts` — typed client (uploadImport / commitImport / listImportJobs / getImportJob / templateDownloadURL / errorsXLSXDownloadURL).
- `web/src/lib/use-current-user.ts` — useRouteLoaderData('root') accessor.

**Modified (5):**
- `web/src/lib/devices.ts` — added ListDevicesEnvelope + ListDevicesFilters types + listDevicesFiltered() + BulkDecommissionResponse + bulkDecommissionDevices(). Legacy listDevices() now unwraps the envelope so existing callers keep working.
- `web/src/routes/devices/index.tsx` — full rewrite: TanStack Table with manualPagination/manualSorting/manualFiltering, URL state via useDevicesSearch, admin-only row checkboxes + BulkActionBar + BulkDecommissionDialog, BulkImportDialog launcher.
- `web/src/App.tsx` — gave root route `id: 'root'`; added admin/imports + admin/imports/:jobId routes.
- `web/src/components/shell/sidebar.tsx` — admin-only Admin section with Imports nav.
- `web/src/routes/metering-points/swap-meter-dialog.test.tsx` — updated /api/devices mock to return the envelope shape (rows[]).

## Decisions Made

(See key-decisions in frontmatter — 8 key decisions documented there.)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] listDevices() returning array against the Phase 3 envelope-returning backend**
- **Found during:** Task 1
- **Issue:** The Plan 03-06 backend rewrite changed `GET /api/devices` to return `{rows, total_count, page_count, page, per_page}`, but the existing `web/src/lib/devices.ts` `listDevices()` was still typed + parsed as `Device[]`. This was silently broken — `swap-meter-dialog`'s `unboundDevices = devicesQuery.data ?? []` would have gotten the envelope object and tried to `.filter` it, exploding in production.
- **Fix:** `listDevices()` now fetches `/api/devices?per_page=100` and unwraps `env.rows`. Added a separate `listDevicesFiltered(filters)` for the new Phase 3 page that needs the full envelope (pageCount, etc.). Also updated `swap-meter-dialog.test.tsx` mocks to match the new wire format.
- **Files modified:** `web/src/lib/devices.ts`, `web/src/routes/metering-points/swap-meter-dialog.test.tsx`
- **Verification:** Full vitest suite (22 files / 122 tests) passes; `pnpm tsc --noEmit` clean.
- **Committed in:** d089a13 (Task 1 commit)

**2. [Rule 3 - Blocking] `useCurrentUser()` hook needed to gate admin surfaces**
- **Found during:** Task 1
- **Issue:** The bulk-action bar, bulk-import button, and /admin/imports routes all need the current user's role. The codebase had no existing `useCurrentUser` hook — `_root.tsx` exposed user via `useLoaderData()` but only inside the same component.
- **Fix:** Created `web/src/lib/use-current-user.ts` reading via `useRouteLoaderData('root')`, plus gave the root route `id: 'root'` in App.tsx so the lookup works.
- **Files modified:** `web/src/lib/use-current-user.ts` (new), `web/src/App.tsx`
- **Verification:** All admin-gate tests pass (viewer redirect tests in admin/imports + the BulkImportButton viewer test in devices/index).
- **Committed in:** d089a13 (Task 1 commit)

**3. [Rule 3 - Blocking] Sidebar path mismatch**
- **Found during:** Task 3
- **Issue:** PLAN frontmatter listed `web/src/components/sidebar/nav-items.tsx` but the actual sidebar in this codebase lives at `web/src/components/shell/sidebar.tsx`. (The 03-08 SUMMARY had already flagged this; the 03-09 plan inherited the bad path.)
- **Fix:** Updated the existing `web/src/components/shell/sidebar.tsx` to add the admin-only Imports nav under an "Admin" group label.
- **Files modified:** `web/src/components/shell/sidebar.tsx`
- **Verification:** Manual code inspection; Sidebar renders an admin user's nav including Imports. Viewer doesn't see it (defense-in-depth on top of server-side gate).
- **Committed in:** b9a3bd0 (Task 3 commit)

---

**Total deviations:** 3 auto-fixed (1 bug, 2 blocking).
**Impact on plan:** All three are necessary for correctness — Rule 1 fix prevents a runtime crash in `swap-meter-dialog`, Rule 3 fixes unblock the role-gating requirements. No scope creep.

## Issues Encountered

- Zod v4's stricter UUID regex (RFC 9562 variant + version bits) initially failed tests using `11111111-1111-1111-1111-111111111111`-style placeholder UUIDs. Switched test fixtures to valid v4 UUIDs (`11111111-1111-4111-8111-111111111111`). No code change to the schema.
- `userEvent.upload(fileInput, pdf)` silently no-ops in jsdom when the input has `accept=".xlsx,.csv"` because `applyAccept` defaults to true. Used `applyAccept: false` in the negative-extension test so the dialog's own client-side guard is what gets exercised.

## Authentication Gates

None — the dialog gates and route guards rely on already-loaded session state (Plan 01-11/14 install + login).

## User Setup Required

None — no external service configuration needed for the new SPA surfaces. Backend endpoints already shipped in Plans 03-05 and 03-06.

## Next Phase Readiness

- DEV-01 frontend, DEV-06 frontend, DEV-07 visibility, DEV-08, D-36 all delivered.
- Plan 03-10 remains: Reveal Keys dialog (admin-only, on /devices/:id detail page) + final UX-03 vocabulary audit + REQUIREMENTS reconciliation + any cross-plan polish.
- Phase 3 is functionally complete after 03-10.

## Self-Check: PASSED

- All 15 created files exist on disk.
- All 5 modified files have the expected changes.
- Commits `d089a13`, `6dff447`, `b9a3bd0` exist in `git log`.
- `pnpm test --run` → 22 files / 122 tests pass.
- `pnpm tsc --noEmit` → clean.
- `pnpm build` → clean.
- Plan acceptance grep checks (Task 1 + Task 2 + Task 3) all match.

---
*Phase: 03-provisioning-gateways-devices-bulk-import*
*Completed: 2026-05-11*
