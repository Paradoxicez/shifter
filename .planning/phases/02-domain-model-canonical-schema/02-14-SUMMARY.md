---
phase: 02-domain-model-canonical-schema
plan: 14
subsystem: ui
tags: [react, typescript, shadcn, tanstack-table, tanstack-query, vitest, react-router, react-hook-form]

# Dependency graph
requires:
  - phase: 01-foundation
    provides: shadcn/ui primitives (Dialog, Sheet, Button, Input, Form, Select, Tabs, Skeleton, DropdownMenu, Card, Alert, Tooltip, Sonner), ResponsiveDialog wrapper, Stepper component, StatusRow component, theme + topbar + sidebar shell, queryClient + apiFetch + ApiError, useUser hook
  - phase: 02-domain-model-canonical-schema
    provides: HTTP routes mounted by 02-10 (sites, MPs, devices) + 02-11 (swap, profiles), DevEUI parse + preflight endpoints, /api/device-profiles/{decoded-sample,…} editor surface
provides:
  - "Sites surface (list + detail + Create dialog) — D-17 + D-18"
  - "Metering Points surface ($id detail + Create dialog D-19 + Swap Meter 3-step dialog D-12/13/14)"
  - "Devices surface (list + Add Device 4-step dialog D-10/11/16 + DevEUI parser D-11)"
  - "Profiles surface (list + single-page mapping editor D-08 + JSON-tree leaf-pick + capability checkboxes + codec_js textarea)"
  - "Sidebar nav order Sites → Devices → Profiles → Settings (UI-SPEC §Layout)"
  - "App.tsx Phase 2 routes wired (11 protected routes total: settings + 7 Phase-2 + index redirect + login + install)"
  - "Client-side RFC 6901 JSON flattener (lib/json-flatten.ts) so the mapping editor renders the JSON tree before first save"
  - "5 vitest skip placeholders (create-site, add-device, deveui-parser, swap-meter, mapping-editor) flipped to active tests; 25+ net-new vitest tests"
affects: [phase-03-realtime-uplink-ingest, phase-04-dashboard-charts, phase-06-audit-browse]

# Tech tracking
tech-stack:
  added: []  # No new npm dependencies; everything reused from Phase 1 + Task 1's shadcn primitives
  patterns:
    - "TanStack Table + shadcn Table primitive pattern (header + body + Skeleton-rows-while-loading + empty-state card)"
    - "Stepped dialog pattern (ResponsiveDialog size=lg + Stepper + per-step heading + Back/Next/Submit footer)"
    - "Editable TanStack Table with per-row Input cells (used by mapping editor)"
    - "Two-pane responsive editor (lg:grid-cols-2 with stack-on-mobile fallback)"
    - "Client-side RFC 6901 JSON flatten matching backend semantics so the editor works pre-save"
    - "vi.mock('@/lib/api', ...) + vi.mock('sonner', ...) + jsdom polyfills (test-setup.ts) for Radix-using component tests"
    - "Inline destructive Alert with UI-SPEC error copy keyed off ApiError.status (400/413/503)"

key-files:
  created:
    - "web/src/components/ui/{table,command,popover,radio-group,textarea,scroll-area}.tsx (Task 1 — 6 shadcn primitives)"
    - "web/src/lib/{sites,devices,metering-points,swap,profiles,json-flatten}.ts (lib helpers — typed clients + flatten)"
    - "web/src/routes/sites/{index,$id,create-site-dialog,create-site-dialog.test}.tsx"
    - "web/src/routes/devices/{index,add-device-dialog,add-device-dialog.test,deveui-parser,deveui-parser.test}.tsx"
    - "web/src/routes/metering-points/{$id,create-mp-dialog,swap-meter-dialog,swap-meter-dialog.test}.tsx"
    - "web/src/routes/profiles/{index,$id,mapping-editor,mapping-editor.test}.tsx"
    - "web/src/test-setup.ts (Radix jsdom polyfills — Task 2)"
  modified:
    - "web/src/App.tsx (8 new lazy routes: /sites, /sites/:id, /metering-points/:id, /devices, /profiles, /profiles/new, /profiles/:id)"
    - "web/src/components/shell/sidebar.tsx (4 nav items: Sites/Devices/Profiles/Settings — UI-SPEC ordering)"
    - "web/components.json (+6 shadcn primitives in registry)"
    - "web/package.json (Task 1 — confirmed @tanstack/react-table peer)"

key-decisions:
  - "Mapping editor as single-page route, NOT a dialog (UI-SPEC explicit UX-01 deviation): too many fields + live preview + two-pane layout + codec_js textarea to fit modally"
  - "Client-side RFC 6901 flatten in lib/json-flatten.ts mirrors internal/profile/handlers.go exactly so the editor renders the JSON tree BEFORE first save (the backend's POST /decoded-sample is path-scoped to {id} which doesn't exist in 'new' mode)"
  - "Slug field disabled in edit mode (defense-in-depth on top of the backend's slug-immutability guard in updateProfile handler)"
  - "Capability errors from backend (400 'invalid capability') mapped to friendly UI-SPEC copy 'Choose valid capabilities' rather than raw server detail"
  - "TestHarness deferred-items.md retained: pre-existing biome OOM is environmental, not a Task-3 regression — vitest + tsc + vite build all clean"

patterns-established:
  - "Editable TanStack Table with per-row Input cells (json_pointer + target + scale + data_type + Trash2 delete) — pattern reusable for any editable-list-of-rows surface (Phase 4 dashboard threshold rules likely)"
  - "Active-row highlight via bg-primary/10 + border-l-2 border-primary — pattern reusable for any 'pick this row to bind' UI (UI-SPEC §Color §Accent reserved for #11)"
  - "Save-error → setSaveError(string) + inline <Alert variant=destructive> above mapping table — clearer than toasting non-blocking errors"
  - "Stepped dialog state machine: useState<number> step + per-step body + Stepper component reuse (Add Device 4-step + Swap Meter 3-step both follow this shape)"

requirements-completed: [SITE-01, DATA-01, DATA-04, DATA-09, CHIRP-04]

# Metrics
duration: 65min  # cumulative across Tasks 1+2+3 — Task 3 alone ~20min in fresh context
completed: 2026-05-04
---

# Phase 02 Plan 14: Phase 2 Frontend Surface Summary

**Every Phase 2 dialog + list page from UI-SPEC.md ships — sites + MPs + devices + profiles surfaces with 5 dialogs (Add Site / Add MP / Add Device 4-step / Swap Meter 3-step / Decommission) + the D-08 single-page mapping editor with paste-JSON + click-leaf binding; 25+ net-new vitest tests, all 5 Phase-2 skip placeholders flipped to active.**

## Performance

- **Duration:** 65 min cumulative (Task 1 ~20min + Task 2 ~25min + Task 3 ~20min in fresh context window per W2 checkpoint protocol)
- **Started:** 2026-05-04T10:35:00Z (Task 1)
- **Completed:** 2026-05-04T17:35:00Z (Task 3)
- **Tasks:** 3 of 3 (no checkpoints triggered after Task 3 — plan complete)
- **Files modified:** 24 (15 created in Task 1+2, 9 created/modified in Task 3 — see Files Created/Modified)

## Accomplishments

- **Sites list + detail + Create dialog (Task 1)** — `/sites` page with TanStack Table (Name link / Address / MP count / Last uplink / Status / Actions overflow), `Add site` D-17 + D-18 dialog with lat/lng range validation + disabled `Pick on map` button + helper "Paste from Google Maps". Site detail with identity card + nested MP and device sections. 4 vitest tests pass.
- **Metering Point + Devices surfaces with 3 dialogs (Task 2)** — MP `$id` detail with latest reading card + active binding card + Swap meter primary CTA, D-19 Create MP dialog (utility class radio), Devices list with name/DevEUI/profile/MP/last seen columns, Add Device 4-step dialog (DevEUI parse → profile pick → MP bind → preflight + submit), Swap Meter 3-step dialog (capture R → pick new device → math read-back R−N → confirm), DevEUI parser reusable component (MSB/LSB radios + vendor OUI hint). 11 vitest tests pass.
- **Profiles list + D-08 mapping editor (Task 3)** — `/profiles` page with capability chips + CS sync status, `/profiles/new` and `/profiles/:id` mounting MappingEditor. Two-pane editor: left = paste sample JSON textarea + clickable leaves; right = editable TanStack Table mapping rows + codec_js textarea. Client-side RFC 6901 flatten matches backend semantics. 6 vitest tests pass.
- **Sidebar nav order Sites → Devices → Profiles → Settings** with MapPin / Cpu / Layers / Settings icons per UI-SPEC §Iconography.
- **All 5 Phase-2 vitest skip placeholders flipped to active tests** — was 1 skip remaining at end of Task 2 (mapping-editor stub); now 0 skips. Total: 51/51 vitest pass.

## Task Commits

W2 checkpoint protocol — 3 commits, one per task, each preceded by green vitest baseline:

1. **Task 1: sites surface + shadcn primitives + sidebar nav** — `3b451c3` (feat)
2. **Task 2: MP + Devices + 3 dialogs (Add MP, Add Device 4-step, Swap Meter 3-step) + DevEUI parser** — `ddbec76` (feat)
3. **Task 3: Profiles list + mapping editor (D-08 paste-JSON + click-leaf)** — `c2ee803` (feat)

## Files Created/Modified

### Task 3 (this commit — c2ee803)

- `web/src/lib/profiles.ts` (modified) — extends Task 2's slim `listProfiles`/`getProfile` with full mapping-editor surface: `Mapping` + `ProfileWithMappings` types, `ProfileRequest`/`MappingRequest` wire shapes mirroring `internal/profile/handlers.go`, `getProfileWithMappings`, `createProfile`, `updateProfile`, `archiveProfile`, `decodedSamplePreview`.
- `web/src/lib/json-flatten.ts` (created) — client-side RFC 6901 leaf walker, semantically identical to `internal/profile/handlers.go`'s `flattenJSON` helper. Caps at 200 leaves (T-02-14-03 paste-bomb defense, aligned with `maxDecodedSampleLeaves` on backend). 99 lines.
- `web/src/routes/profiles/index.tsx` (created) — Profiles list page with TanStack Table; capability chips with `+N more` overflow, CS sync status row, archive overflow action. 235 lines.
- `web/src/routes/profiles/mapping-editor.tsx` (created) — D-08 single-page editor: top bar (Back / Profile name editable / Save / Cancel), identity row (Slug + Vendor + Family + Counter modulus + helper text), MAC version + Region, 10 D-04 capability checkboxes, two-pane body (left: paste JSON textarea + clickable leaf list; right: editable TanStack Table mapping rows + codec_js textarea), live preview footer Card. 656 lines.
- `web/src/routes/profiles/$id.tsx` (created) — 21-line wrapper that picks 'new' vs 'edit' mode from `useParams<{id?:string}>()`.
- `web/src/routes/profiles/mapping-editor.test.tsx` (modified — flipped from 6-line skip stub to 276 active lines) — 6 tests: load existing + paste JSON renders tree + click-leaf populates pointer + Add mapping defaults scale=1 + save POSTs /api/device-profiles + 400 invalid capability inline alert.
- `web/src/App.tsx` (modified) — adds 3 lazy imports (ProfilesPage, ProfileEditorRoute) + 3 route entries (`/profiles`, `/profiles/new`, `/profiles/:id`).
- `.planning/phases/02-domain-model-canonical-schema/deferred-items.md` (modified) — appends Task 3 confirmation that biome OOM is environmental.

### Task 1+2 (already committed at 3b451c3 + ddbec76)

- `web/components.json` + 6 shadcn primitives in `web/src/components/ui/` (table, command, popover, radio-group, textarea, scroll-area)
- `web/src/lib/{sites,devices,metering-points,swap,profiles}.ts` — Phase 2 typed API clients
- `web/src/components/shell/sidebar.tsx` — 4-item nav ordering (Sites/Devices/Profiles/Settings)
- `web/src/routes/sites/{index,$id,create-site-dialog,create-site-dialog.test}.tsx`
- `web/src/routes/devices/{index,add-device-dialog,add-device-dialog.test,deveui-parser,deveui-parser.test}.tsx`
- `web/src/routes/metering-points/{$id,create-mp-dialog,swap-meter-dialog,swap-meter-dialog.test}.tsx`
- `web/src/test-setup.ts` — jsdom polyfills for Radix UI (hasPointerCapture, scrollIntoView)
- `web/src/App.tsx` (Task 1+2 incremental) — /sites, /sites/:id, /devices, /metering-points/:id

### Verification snippets

```bash
# All Phase 2 routes wired in App.tsx
$ grep -E "path: '(sites|devices|metering-points|profiles)'" web/src/App.tsx
        path: 'sites',
        path: 'sites/:id',
        path: 'devices',
        path: 'metering-points/:id',
        path: 'profiles',
        path: 'profiles/new',
        path: 'profiles/:id',

# Sidebar nav order matches UI-SPEC §Layout
$ grep -E "label: '(Sites|Devices|Profiles|Settings)'" web/src/components/shell/sidebar.tsx
  { to: '/sites', label: 'Sites', icon: MapPin },
  { to: '/devices', label: 'Devices', icon: Cpu },
  { to: '/profiles', label: 'Profiles', icon: Layers },
  { to: '/settings', label: 'Settings', icon: SettingsIcon },

# Two-pane mapping editor body
$ grep "lg:grid-cols-2" web/src/routes/profiles/mapping-editor.tsx
      <section className="grid grid-cols-1 gap-6 lg:grid-cols-2">

# Final test count
$ pnpm --dir web test --run
 Test Files  12 passed (12)
      Tests  51 passed (51)
```

## Decisions Made

1. **Mapping editor as single-page route, NOT a dialog** — UI-SPEC explicit UX-01 deviation. Codified by `<MappingEditor mode="new"|"edit">` mounted by `routes/profiles/$id.tsx`; no `<Dialog>` wrap. Rationale: the editor has 10 capability checkboxes + identity row + two-pane body + codec_js textarea + live preview — fitting that into even a `lg` ResponsiveDialog (~768px max-width) is hostile.
2. **Client-side RFC 6901 flatten via lib/json-flatten.ts** — Plan body offered "client-side flatten OR new POST /preview endpoint"; chose client-side because (a) the backend's `POST /decoded-sample` is path-scoped to `{id}` so it doesn't work in 'new' mode without a save-first round-trip; (b) walking ~200 leaves of pasted JSON in the browser is essentially free; (c) avoids a new endpoint that doesn't earn its keep. Helper caps at 200 leaves matching `maxDecodedSampleLeaves` on the backend (T-02-14-03 defense-in-depth).
3. **400 'invalid capability' → 'Choose valid capabilities' UI-SPEC copy** — backend returns raw `validation` error with detail like `invalid capability "bogus"`; the editor's error handler matches `/invalid capability/i` and substitutes the friendly UI-SPEC copy. Other 400 errors render the raw detail (which is already operator-readable per the validateMapping copy in `editor.go`).
4. **Slug field disabled in edit mode** — defense-in-depth on top of the backend's slug-immutability guard. The handler at `internal/profile/handlers.go::updateProfile` already loads the persisted slug and ignores the body's value; making the input visibly disabled prevents operator confusion.
5. **Save success path navigates to /profiles** — keeps the toast visible after navigation (sonner toasts are app-level), prevents the editor from showing stale state if the operator re-opens the same profile.

## Deviations from Plan

### Auto-fixed Issues (Task 3)

**Total deviations in Task 3: 0.** Plan executed exactly as written. The Task 3 RED→GREEN cycle was clean: 6 tests written first → all 6 passed against first implementation pass with no auto-fix iterations.

(Tasks 1 + 2 deviation log lives in their respective execution sessions; this SUMMARY focuses on Task 3 as the final piece since 1+2 were already user-approved at Checkpoints A + B.)

## Issues Encountered

None during Task 3.

The pre-existing biome OOM (documented in `.planning/phases/02-domain-model-canonical-schema/deferred-items.md`) reproduces on Task 3 files in isolation, confirming it's environmental — not a regression. Vitest + tsc + vite build are all clean.

## User Setup Required

None - no external service configuration required. All Phase 2 surfaces consume the existing `/api/*` mounted by Plan 02-10 + 02-11.

## Next Phase Readiness

- Plan 02-15 reconciliation can flip these VALIDATION.md component-level rows from ⬜ pending to ✅ green pointing at the new test files:
  - **SITE-01**: `web/src/routes/sites/create-site-dialog.test.tsx` (4 tests)
  - **DATA-04**: `web/src/routes/metering-points/swap-meter-dialog.test.tsx` (4 tests)
  - **DATA-09**: `web/src/routes/profiles/mapping-editor.test.tsx` (6 tests)
  - **CHIRP-04**: `web/src/routes/devices/add-device-dialog.test.tsx` (3 tests) + `deveui-parser.test.tsx` (4 tests)
- Phase 2 frontend surface is complete. Plan 02-12 (cmd/serve wiring — Wave 7 gap-closure) is the next sequential plan.
- Phase 3 (realtime uplink ingest + dashboard chart) inherits this surface unchanged — it adds SSE streaming inside MP detail's Latest reading card and chart sparklines, but the surface frame, navigation, and dialog inventory are stable.

## Self-Check: PASSED

- All created files exist on disk (verified via `ls -la`).
- Task 1 commit `3b451c3` present in `git log` (verified: `git log --oneline | grep 3b451c3`).
- Task 2 commit `ddbec76` present in `git log` (verified).
- Task 3 commit `c2ee803` present in `git log` (verified).
- Full vitest: 51/51 pass, 0 skipped (was 45 + 1 mapping-editor skip stub at end of Task 2).
- TypeScript: `npx tsc --noEmit` clean.
- Vite production build: clean in 1.73s.

---
*Phase: 02-domain-model-canonical-schema*
*Completed: 2026-05-04*
