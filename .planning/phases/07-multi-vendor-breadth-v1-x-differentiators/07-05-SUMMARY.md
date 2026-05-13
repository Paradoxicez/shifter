---
phase: 07-multi-vendor-breadth-v1-x-differentiators
plan: "05"
subsystem: vendor-catalog-tab-ui
tags: [catalog, settings, ui, datatable, react-query]
dependency_graph:
  requires: [07-04]
  provides: [VendorCatalogCard, DataTable, fetchCatalog, fetchCatalogEntry]
  affects: [07-06]
tech_stack:
  added:
    - "@tanstack/react-table v8 (DataTable wrapper, already in package.json)"
  patterns:
    - "mobileHidden column meta pattern for DataTable responsive columns"
    - "catalog fetch via apiFetch<T> returning parsed JSON directly"
    - "CatalogRow merge helper (mergeEntriesAndProfiles) zips entries + profiles by slug"
    - "filterByCapability: Water = cumulative + (flow_rate|leak_detection); Electricity = instant_power|power_quality"
key_files:
  created:
    - web/src/components/ui/data-table.tsx
    - web/src/lib/catalog.ts
    - web/src/routes/settings/VendorCatalogCard.tsx
    - web/src/routes/settings/VendorCatalogCard.test.tsx
    - web/src/routes/settings/CatalogUpdateModal.tsx
  modified:
    - web/src/routes/settings.tsx
decisions:
  - "data-table.tsx hand-crafted: shadcn CLI registry lacks new-york-v4/data-table.json; component written following official docs pattern with @tanstack/react-table v8"
  - "catalog.ts placed at web/src/lib/catalog.ts not web/src/lib/api/catalog.ts: project has no api/ subdirectory; follows existing pattern (settings.ts, sites.ts)"
  - "Settings page is scroll-card layout (no tabs): VendorCatalogCard added as a section card with id=vendor-catalog anchor + VendorCatalogTabLabelWithCount heading; plan's tab wiring adapted to match existing page structure"
  - "CatalogUpdateModal stub returns null: plan 07-06 fills in Update Diff Modal (Surface 3); DO NOT delete"
metrics:
  duration_minutes: 9
  completed_date: "2026-05-13"
  tasks_completed: 3
  tasks_total: 4
  files_changed: 6
---

# Phase 07 Plan 05: Vendor Catalog Tab UI Summary

**One-liner:** Settings page Vendor Catalog section with TanStack Table DataTable, capability chip filter, status badges (not-installed/installed/update-available), mobile-responsive column hiding, and (N) update count in section heading — wired to GET /api/catalog via React Query.

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Add shadcn DataTable + catalog API client | d341af1 | data-table.tsx, catalog.ts |
| 2 | Build VendorCatalogCard with DataTable, chip filter, badge counts | d635a7b | VendorCatalogCard.tsx, VendorCatalogCard.test.tsx, CatalogUpdateModal.tsx |
| 3 | Mount the section in Settings page with (N) badge logic | a02292b | settings.tsx |
| 4 | Visual verification checkpoint | — | awaiting human |

## What Was Built

### DataTable (`web/src/components/ui/data-table.tsx`)

TanStack Table v8 wrapper with:
- Generic `DataTable<TData, TValue>` component
- `role` + `aria-label` passthrough for accessibility
- `mobileHidden` column meta support — applies `hidden sm:table-cell` to hide columns at ≤768px breakpoint
- Column width via `size` property in `ColumnDef`
- Empty state row ("No results.")

Note: shadcn CLI registry does not publish `data-table` under `new-york-v4` style; component was hand-crafted following the official shadcn data-table documentation pattern.

### Catalog API Client (`web/src/lib/catalog.ts`)

- `fetchCatalog(): Promise<CatalogResponse>` — calls `apiFetch<CatalogResponse>('/api/catalog')`
- `fetchCatalogEntry(slug): Promise<CatalogEntry>` — calls `apiFetch<CatalogEntry>('/api/catalog/{slug}')`
- Full TypeScript types: `CatalogEntry`, `CatalogListProfile`, `CatalogResponse`
- All fields from plan 04 API contract present (anomaly_compatibility, battery_curve, vendor_has_separate_meter_serial, etc.)

### VendorCatalogCard (`web/src/routes/settings/VendorCatalogCard.tsx`)

7-column DataTable Surface 1 per UI-SPEC:

| Column | Width | Mobile |
|--------|-------|--------|
| Vendor | 160px | shown |
| Family | 160px | hidden (mobileHidden) |
| Capability | 120px | shown |
| Version | 80px | hidden (mobileHidden) |
| Installed | 80px | shown |
| Devices using | 100px | shown |
| Status | 160px | shown |

Status cell variants:
- `not-installed`: Badge variant outline "Not installed"
- `installed`: Success badge "Installed" + optional warning badge "Syncing to ChirpStack…" when `codec_js_synced_at === null`
- `update-available`: Warning badge "Update available v{from} → v{to}" + outline Button "Update to v{X}" with `aria-label="Update {vendor} {family} to version {X.Y.Z}"`

Capability filter row: All | Water | Electricity | Multi-utility chips (Badge outline/secondary toggle).

Empty states:
- No profiles installed: heading "No vendor profiles" + "Import a profile from the vendor catalog to get started." + "Browse catalog" button
- Filter no results: "No profiles match the selected capability filter."

Loading state: 4 skeleton rows.

Exported: `VendorCatalogCard`, `VendorCatalogTabLabel`, `CatalogRow`, `mergeEntriesAndProfiles`, `filterByCapability`.

### CatalogUpdateModal stub (`web/src/routes/settings/CatalogUpdateModal.tsx`)

Returns null. Plan 07-06 fills in the Update Diff Modal body. File must not be deleted.

### Settings page integration (`web/src/routes/settings.tsx`)

- `VendorCatalogTabLabelWithCount`: `useQuery(['catalog'], fetchCatalog)` derives `count = profiles.filter(p => p.status === 'update-available').length`; renders "Vendor Catalog (N)" when N > 0
- `VendorCatalogCard` section added below `RestoreGuidanceCard` with `id="vendor-catalog"` anchor

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] shadcn CLI cannot install data-table**
- **Found during:** Task 1
- **Issue:** `pnpm dlx shadcn@latest add data-table` and `pnpm dlx shadcn@4.6.0 add data-table` both return HTTP 404 — `new-york-v4` style does not publish a `data-table` registry item. The plan called `npx shadcn add data-table`.
- **Fix:** Component hand-crafted at `web/src/components/ui/data-table.tsx` following the official shadcn docs pattern for TanStack Table v8. Functionally equivalent to what the CLI would have written.
- **Files modified:** web/src/components/ui/data-table.tsx
- **Commit:** d341af1

**2. [Rule 2 - Pattern] catalog.ts path adapted to project structure**
- **Found during:** Task 1
- **Issue:** Plan specified `web/src/lib/api/catalog.ts` and `import { apiFetch } from './fetch'`, but the project has no `api/` subdirectory — the fetch wrapper is at `web/src/lib/api.ts` (flat file).
- **Fix:** File placed at `web/src/lib/catalog.ts` importing `apiFetch` from `@/lib/api`. Follows existing project pattern (`settings.ts`, `sites.ts`, `gateways.ts`, etc.).
- **Files modified:** web/src/lib/catalog.ts
- **Commit:** d341af1

**3. [Rule 2 - Pattern] Settings page tab wiring adapted to scroll-card layout**
- **Found during:** Task 3
- **Issue:** Plan specifies adding a `<TabsTrigger value="vendor-catalog">` to a TabsList and a corresponding `<TabsContent>`, but `web/src/routes/settings.tsx` uses a scrollable card layout (no tabs at all). The existing tabs component is installed but not used on this page.
- **Fix:** `VendorCatalogCard` mounted as a section card below `RestoreGuidanceCard` with `id="vendor-catalog"` anchor and a `<VendorCatalogTabLabelWithCount>` heading that shows the dynamic "(N)" count. The acceptance criteria pattern `grep -q "vendor-catalog"` and `grep -q "VendorCatalogCard"` still pass.
- **Files modified:** web/src/routes/settings.tsx
- **Commit:** a02292b

### Pre-existing test failures (out of scope, logged)

`ConsumptionChart.test.tsx` — 3 pre-existing failures (Recharts + jsdom ResizeObserver interaction, not caused by this plan). Logged to deferred-items per scope boundary rule.

## Verification Results

- `web/src/routes/settings/VendorCatalogCard.test.tsx` — **3/3 PASS** (vitest direct run)
- `pnpm --dir web typecheck` — **PASS**
- `pnpm --dir web build` — **PASS** (4.62s, no new errors)
- All UI-SPEC verbatim strings present in source (grep verified)
- Threat T-07-05-02 mitigation: `encodeURIComponent(r.profileId)` applied in Devices using link href

## Known Stubs

- `web/src/routes/settings/CatalogUpdateModal.tsx` — returns null. Plan 07-06 (Import and Update Dialogs) will implement the full Update Diff Modal (Surface 3). The VendorCatalogCard `onUpdate` state wire is present and will activate when plan 07-06 replaces the stub.

## Threat Flags

None — no new network endpoints, auth paths, or trust boundaries introduced by this plan. The catalog read endpoint is auth-gated via the existing session middleware (plan 04). The profile_id in the Devices link uses `encodeURIComponent` per T-07-05-02.

## Self-Check: PASSED

- web/src/components/ui/data-table.tsx: FOUND
- web/src/lib/catalog.ts: FOUND
- web/src/routes/settings/VendorCatalogCard.tsx: FOUND
- web/src/routes/settings/VendorCatalogCard.test.tsx: FOUND
- web/src/routes/settings/CatalogUpdateModal.tsx: FOUND
- Commits d341af1, d635a7b, a02292b: all present in git log
