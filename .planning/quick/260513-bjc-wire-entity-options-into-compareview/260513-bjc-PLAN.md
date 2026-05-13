---
phase: quick
plan: "260513-bjc"
type: execute
wave: 1
depends_on: []
files_modified:
  - web/src/routes/reports/CompareView.tsx
  - web/src/routes/reports/CompareView.test.tsx
autonomous: true
requirements:
  - QUICK-260513-bjc-wire-entity-options-into-compareview
must_haves:
  truths:
    - "Entity A and Entity B dropdowns render real metering-point options pulled from GET /api/metering-points"
    - "Each option label includes the parent site name as context (e.g. 'MP-01 — Building A')"
    - "Dropdowns show a 'Loading…' placeholder while the metering-points query is in flight"
    - "Existing CompareView.test.tsx tests stay green (empty state, swap button, mode toggle, chart placeholder copy)"
    - "A new test asserts the dropdowns show populated options when the API mock returns data"
    - "pnpm typecheck stays clean"
  artifacts:
    - path: "web/src/routes/reports/CompareView.tsx"
      provides: "Populated EntityDropdown options via useQuery(listMPs) + listSites join"
      contains: "useQuery"
    - path: "web/src/routes/reports/CompareView.test.tsx"
      provides: "Updated mock + new test verifying options render"
      contains: "metering-points"
  key_links:
    - from: "web/src/routes/reports/CompareView.tsx"
      to: "/api/metering-points"
      via: "listMPs() through useQuery"
      pattern: "queryFn.*listMPs"
    - from: "web/src/routes/reports/CompareView.tsx"
      to: "/api/sites"
      via: "listSites() through useQuery (for site-name lookup)"
      pattern: "queryFn.*listSites"
---

<objective>
Wire the EntityDropdown options in CompareView so the Compare report flow is end-to-end usable.

**What:** Replace `const entityOptions: EntityOption[] = []` with two React Query lookups (`listMPs` + `listSites`) and join the responses into `{ id, label }` pairs where label = `${mp.name} — ${site.name}`. Add a 'Loading…' placeholder while data is in flight. Pass the same options list to both dropdowns in entities-mode and to the single dropdown in time_ranges-mode. Keep entity_type fixed to `'metering_point'` (matches the existing `useState<EntityType>('metering_point')` and the SUMMARY note that resolution was deferred).

**Why:** Phase 7 plan 07-12 shipped Compare backend (`POST /api/reports/compare`) and the entire UI shell, but the dropdowns are empty stubs (`CompareView.tsx:234`). Users currently cannot select entities, so the feature is structurally complete but functionally inert. This is the last wire required for the compare flow to work end-to-end.

**Output:** CompareView.tsx with two `useQuery` calls feeding non-empty entityOptions; updated test file with a successful "options render" assertion.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/STATE.md
@.planning/phases/07-multi-vendor-breadth-v1-x-differentiators/07-12-SUMMARY.md
@web/src/routes/reports/CompareView.tsx
@web/src/routes/reports/CompareView.test.tsx
@web/src/lib/compare.ts
@web/src/lib/metering-points.ts
@web/src/lib/sites.ts
@web/src/lib/api.ts

<interfaces>
<!-- Existing exports the executor must use directly — no codebase exploration needed. -->

From `web/src/lib/metering-points.ts`:
```typescript
export interface MeteringPoint {
  id: string
  site_id: string
  name: string
  utility_class: 'water' | 'electricity'
  // ...other fields...
}
export const listMPs: (opts?: { archived?: boolean }) => Promise<MeteringPoint[]>
// GET /api/metering-points → MeteringPoint[]  (NOTE: response does NOT include site_name; join client-side via site_id)
```

From `web/src/lib/sites.ts`:
```typescript
export interface Site {
  id: string
  name: string
  // ...other fields...
}
export const listSites: (opts?: { archived?: boolean }) => Promise<Site[]>
// GET /api/sites → Site[]
```

From `web/src/lib/compare.ts` (already imported by CompareView):
```typescript
export type CompareRequest = CompareEntitiesRequest | CompareTimeRangesRequest
export type CompareResponse = { a: SeriesResult; b: SeriesResult; delta: { total, peak, average } }
```

Existing `EntityOption` shape inside `CompareView.tsx`:
```typescript
interface EntityOption {
  id: string
  label: string
}
```

Existing TanStack Query patterns in the same area (see `web/src/routes/reports/ReportConfigPanel.tsx:130-139`):
```typescript
const { data: sites = [] } = useQuery<Site[]>({
  queryKey: ['sites'],
  queryFn: () => apiFetch<Site[]>('/api/sites'),
})
const { data: meters = [] } = useQuery<MeteringPoint[]>({
  queryKey: ['metering-points'],
  queryFn: () => apiFetch<MeteringPoint[]>('/api/metering-points'),
})
```

Same query keys (`['sites']`, `['metering-points']`) should be reused so the React Query cache is shared with ReportConfigPanel + add-device dialog + filter-toolbar (already-warmed cache → instant dropdown).

The test file mocks `@/lib/api` so a single `apiFetch` mock controls both queries:
```typescript
vi.mock('@/lib/api', () => ({
  apiFetch: vi.fn().mockResolvedValue([]),
}))
```
The executor will switch this to a path-aware mock that returns MPs for `/api/metering-points` and sites for `/api/sites`.
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Wire useQuery(listMPs + listSites), join, and populate EntityDropdown options</name>
  <files>web/src/routes/reports/CompareView.tsx, web/src/routes/reports/CompareView.test.tsx</files>
  <behavior>
    Test additions in `web/src/routes/reports/CompareView.test.tsx`:

    1. **Path-aware apiFetch mock** — replace the current blanket `mockResolvedValue([])` mock with a `mockImplementation` that branches on the path argument:
       - `/api/metering-points` → returns `[{ id: 'mp-1', site_id: 'site-1', name: 'MP One', utility_class: 'water' }, { id: 'mp-2', site_id: 'site-2', name: 'MP Two', utility_class: 'electricity' }]`
       - `/api/sites` → returns `[{ id: 'site-1', name: 'Building A', timezone: 'UTC' }, { id: 'site-2', name: 'Building B', timezone: 'UTC' }]`
       - Anything else → returns `[]`
       (The `compareReports` mock at the top of the file already covers POST /api/reports/compare — keep it.)

    2. **New test:** `it('populates entity dropdowns with metering-point options once data loads')`
       - Renders CompareView, opens the first `combobox` (Entity A) by `fireEvent.click`
       - Asserts `await screen.findByText('MP One')` is in the document and includes the parent site name in some form (e.g. `screen.findByText(/Building A/i)`).
       - Note: `findByText` is used because React Query resolves asynchronously.

    3. **Existing 4 tests must remain green:**
       - mode toggle renders
       - empty state body copy ("Choose entities and a time range…")
       - swap button clickable
       - "Select two entities to compare." placeholder visible

    Implementation in `web/src/routes/reports/CompareView.tsx`:

    1. **Add imports** at the top:
       ```typescript
       import { useMutation, useQuery } from '@tanstack/react-query'
       import { listMPs } from '@/lib/metering-points'
       import { listSites } from '@/lib/sites'
       ```

    2. **Replace** the `const entityOptions: EntityOption[] = []` stub (around line 234) with:
       ```typescript
       const mpsQuery = useQuery({ queryKey: ['metering-points'], queryFn: () => listMPs() })
       const sitesQuery = useQuery({ queryKey: ['sites'], queryFn: () => listSites() })

       const isLoadingOptions = mpsQuery.isPending || sitesQuery.isPending

       const entityOptions: EntityOption[] = useMemo(() => {
         const sites = sitesQuery.data ?? []
         const mps = mpsQuery.data ?? []
         const siteNameById = new Map(sites.map((s) => [s.id, s.name]))
         return mps.map((mp) => {
           const siteName = siteNameById.get(mp.site_id)
           return {
             id: mp.id,
             label: siteName ? `${mp.name} — ${siteName}` : mp.name,
           }
         })
       }, [mpsQuery.data, sitesQuery.data])
       ```
       (Add `useMemo` to the existing `import { useState } from 'react'` line so it becomes `import { useMemo, useState } from 'react'`.)

    3. **Loading placeholder** — pass an `isLoading` prop down to `EntityDropdown` so the trigger button shows `Loading…` instead of the placeholder while options are still fetching. Update the `EntityDropdown` component:
       ```typescript
       interface EntityDropdownProps {
         value: string | null
         onChange: (id: string | null) => void
         placeholder: string
         options: EntityOption[]
         isLoading?: boolean
       }

       function EntityDropdown({ value, onChange, placeholder, options, isLoading }: EntityDropdownProps) {
         // ...
         <Button ... disabled={isLoading}>
           {isLoading ? 'Loading…' : selected ? selected.label : placeholder}
           // ...
         </Button>
         // ...
       }
       ```

    4. **Pass `isLoading={isLoadingOptions}`** to all three EntityDropdown usages (Entity A, Entity B in entities mode; single dropdown in time_ranges mode).

    5. **Keep `entity_type` fixed to `'metering_point'`** — do NOT add a site/MP toggle. The `useState<EntityType>('metering_point')` line stays as-is; the eslint underscore for unused setter is fine (it's already destructured as `const [entityType] = useState<EntityType>('metering_point')` so no change needed).
  </behavior>
  <action>
    1. Edit `web/src/routes/reports/CompareView.test.tsx`:
       - Replace `vi.mock('@/lib/api', () => ({ apiFetch: vi.fn().mockResolvedValue([]) }))` with a path-aware `mockImplementation` (see <behavior>). Type the `path` parameter as `string`.
       - Add the new "populates entity dropdowns" test as the last `it()` in the describe block. Use `findByText` (async) because React Query resolves asynchronously. To open the dropdown: `const [comboA] = screen.getAllByRole('combobox'); fireEvent.click(comboA);` then assert via `await screen.findByText('MP One')` and `await screen.findByText(/Building A/i)`.

    2. Edit `web/src/routes/reports/CompareView.tsx`:
       - Add `useMemo` to the `useState` import line.
       - Add `useQuery` to the `@tanstack/react-query` import line (it currently only imports `useMutation`).
       - Add the two new imports: `import { listMPs } from '@/lib/metering-points'` and `import { listSites } from '@/lib/sites'`.
       - Replace the stubbed `const entityOptions: EntityOption[] = []` with the two `useQuery` calls + `useMemo` join.
       - Extend the `EntityDropdownProps` interface with `isLoading?: boolean`, and render `Loading…` + `disabled` when `isLoading` is true.
       - Pass `isLoading={isLoadingOptions}` to all three EntityDropdown instances.
       - Do NOT change anything else (mutation logic, range pickers, swap, table, chart, copy strings — all stay verbatim).

    3. Validate locally:
       - `pnpm --filter web typecheck` (clean exit)
       - `pnpm --filter web test -- CompareView` (5 tests pass — 4 existing + 1 new)
       - `pnpm --filter web lint` (clean exit) — only if a lint step is wired in the repo; skip otherwise

    Constraints:
    - TypeScript strict mode (no `any`, no `as` casts).
    - Reuse existing query keys `['metering-points']` and `['sites']` — do NOT invent new ones; the cache must remain shared with ReportConfigPanel + AddDeviceDialog + filter-toolbar.
    - Do NOT modify `@/lib/compare`, `@/lib/api`, `@/lib/metering-points`, or `@/lib/sites`.
    - Do NOT introduce a site-vs-MP entity-type toggle — keep `entity_type` fixed to `'metering_point'` per existing CompareView state and 07-12-SUMMARY guidance.
    - UI-SPEC copy strings stay verbatim (heading "Compare", "Compare entities", "Compare time ranges", "Select entity A", "Select entity B", "Swap entities A and B", "Compare two entities", "Choose entities and a time range to see side-by-side consumption data.", "Select two entities to compare.", "No data for this range.").
  </action>
  <verify>
    <automated>cd web && pnpm typecheck && pnpm test -- CompareView --run</automated>
  </verify>
  <done>
    - `web/src/routes/reports/CompareView.tsx` calls `useQuery({ queryKey: ['metering-points'], queryFn: () => listMPs() })` and `useQuery({ queryKey: ['sites'], queryFn: () => listSites() })`.
    - `entityOptions` is derived via `useMemo` from the joined data; labels are `${mp.name} — ${siteName}` when a matching site is found, else `mp.name`.
    - All three EntityDropdown instances receive `options={entityOptions}` and `isLoading={isLoadingOptions}`.
    - `web/src/routes/reports/CompareView.test.tsx` mocks `apiFetch` path-aware (MPs for `/api/metering-points`, sites for `/api/sites`).
    - 5 tests pass in the CompareView suite (4 existing + 1 new "populates entity dropdowns" test).
    - `pnpm typecheck` exits 0.
    - All UI-SPEC copy strings still present verbatim in `CompareView.tsx`.
  </done>
</task>

</tasks>

<verification>
- `cd web && pnpm typecheck` — clean
- `cd web && pnpm test -- CompareView --run` — all 5 CompareView tests pass
- `cd web && pnpm build` — clean (catches any prod-bundle TS surface that test-mode skips)
- Manual smoke (optional, not gating): open `/compare` with backend running, both Entity A and Entity B dropdowns list the install's metering points with site context in the label.
</verification>

<success_criteria>
- Compare flow is end-to-end usable: operator can pick entity A, entity B, a date range, and click Compare to fire `POST /api/reports/compare` against real entity IDs.
- React Query cache key `['metering-points']` is shared with ReportConfigPanel/AddDeviceDialog/filter-toolbar — opening CompareView after using one of those screens shows instant dropdowns (no second fetch).
- No regression in existing CompareView tests.
- No backend changes. No new dependencies.
</success_criteria>

<output>
After completion, create `.planning/quick/260513-bjc-wire-entity-options-into-compareview/260513-bjc-SUMMARY.md` documenting:
- Files changed (CompareView.tsx + CompareView.test.tsx)
- Test count (4 → 5)
- Any auto-fixes if the executor had to bend the plan
- Confirmation that the known stub from 07-12-SUMMARY ("EntityDropdown options always empty") is now resolved.
</output>
