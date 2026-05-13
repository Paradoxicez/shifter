---
phase: 07-multi-vendor-breadth-v1-x-differentiators
plan: "06"
subsystem: import-and-update-dialogs
tags: [catalog, import, update, diff, ui, react-query, surface-2, surface-3]
dependency_graph:
  requires:
    - phase: 07-04
      provides: POST /api/catalog/import, POST /api/catalog/{id}/update
    - phase: 07-05
      provides: VendorCatalogCard, CatalogRow type, CatalogUpdateModal stub
  provides:
    - importFromCatalog + applyCatalogUpdate API client mutations (catalog.ts)
    - ImportFromCatalogDialog 3-step flow (Surface 2)
    - CatalogUpdateModal per-field diff + toggles (Surface 3)
    - GET /api/catalog/{slug} now includes codec_js inline (catalogEntryWithCodec)
  affects: [V2-VEND-01 closed]
tech_stack:
  added: []
  patterns:
    - "useEffect for async form pre-population (avoid in-render form.reset calls)"
    - "D-34 allowlist 8-field diff computed client-side from CatalogEntry vs Profile"
    - "ToggleGroup type=single variant=outline for per-field mine/catalog choice"
    - "409 mapped to recognisable Error('already imported') sentinel in API client"
    - "catalogEntryWithCodec backend wrapper embeds codec_js inline for Review step"
key_files:
  created:
    - web/src/routes/profiles/ImportFromCatalogDialog.tsx
    - web/src/routes/profiles/ImportFromCatalogDialog.test.tsx
    - web/src/routes/settings/CatalogUpdateModal.test.tsx
  modified:
    - web/src/lib/catalog.ts
    - web/src/routes/settings/CatalogUpdateModal.tsx
    - web/src/routes/profiles/index.tsx
    - internal/api/catalog_handler.go
key_decisions:
  - "ImportFromCatalogDialog placed at web/src/routes/profiles/ (project uses profiles/ not device-profiles/)"
  - "GET /api/catalog/{slug} extended with catalogEntryWithCodec to include codec_js inline — avoids separate round-trip for Review step"
  - "Form pre-population uses useEffect (not in-render conditional) to avoid React render-cycle issues in tests"
  - "CatalogUpdateModal computes diff client-side against D-34 8-field allowlist"
  - "ProfilesPage header gains 'Import from catalog' outline button alongside existing 'Create profile' link"
requirements-completed: [V2-VEND-01]
metrics:
  duration_minutes: 25
  completed_date: "2026-05-13"
  tasks_completed: 3
  tasks_total: 4
  files_changed: 7
---

# Phase 07 Plan 06: Import and Update Dialogs Summary

**One-liner:** ImportFromCatalogDialog 3-step flow (RadioGroup → Command picker → Review form) and CatalogUpdateModal per-field diff with ToggleGroup mine/catalog toggles — closes Surface 2 and Surface 3, wiring V2-VEND-01 end-to-end.

## Tasks Completed

| Task | Name | Commit | Files |
|------|------|--------|-------|
| 1 | Add API client mutations for import + update | 86ef372 | catalog.ts |
| 2 | Implement ImportFromCatalogDialog (Surface 2 — 3 steps) | d3b4785 | ImportFromCatalogDialog.tsx, .test.tsx, index.tsx, catalog_handler.go, catalog.ts |
| 3 | Implement CatalogUpdateModal (Surface 3 — diff + per-field toggles) | 47c1274 | CatalogUpdateModal.tsx, CatalogUpdateModal.test.tsx |
| 4 | Operator verification of Import + Update flows | — | checkpoint:human-verify pending |

## What Was Built

### API Client Mutations (`web/src/lib/catalog.ts`)

- `importFromCatalog(slug, name?)` — POST /api/catalog/import; 409 mapped to `Error('already imported')` sentinel
- `applyCatalogUpdate(profileId, body)` — POST /api/catalog/{id}/update; returns `updated_at` + `new_version`
- `CatalogEntry` type extended with `codec_js` field (inline source from backend)

### Backend Extension (`internal/api/catalog_handler.go`)

- `catalogEntryWithCodec` struct embeds `codec.CatalogEntry` + adds `codec_js string` field
- `GetCatalogEntryHandler` now responds with full codec source inline — the Review step pre-populates the codec textarea without a separate round-trip

### ImportFromCatalogDialog (`web/src/routes/profiles/ImportFromCatalogDialog.tsx`)

3-step Surface 2 flow per UI-SPEC:

**Step 1 (start):** RadioGroup with two cards
- "Start blank" / "Create a new profile from scratch"
- "Import from catalog" / "Start with a pre-configured vendor profile"
- Continue disabled until selection

**Step 2 (picker):** shadcn `<Command>` searchable list (max-height 260px)
- Populated from `fetchCatalog()` entries
- Placeholder: "Search vendor profiles…"

**Step 3 (review):** Editable form pre-filled from `fetchCatalogEntry(slug)`
- Profile name (default: `{vendor} {family}`)
- Capability chips (read-only)
- Codec textarea (pre-filled with inline `codec_js`)
- Expected uplink interval (seconds)
- 409 error: inline "A profile with this name already exists."
- Other error: toast "Failed to add profile. Try again."
- Success: toast "Profile added" + cache invalidation

Wired into `ProfilesPage` header as an "Import from catalog" outline button alongside the existing "Create profile" link.

**4 tests:** picker reveal, review form population, success toast, 409 inline error — all pass.

### CatalogUpdateModal (`web/src/routes/settings/CatalogUpdateModal.tsx`)

Full Surface 3 replacing the null stub:

- Fetches catalog entry (slug) + installed profile by ID
- Computes per-field diff against D-34 8-field allowlist
- 5-column table inside `<ScrollArea>` (max-height 400px): Field | Current value | Catalog value | Choice | Flag
- Per-field `<ToggleGroup type="single">`: "Use mine" / "Use catalog"
  - `customerEdited=true` defaults all toggles to "Use mine"
  - Each toggle has aria-label "Use my value for {field}" / "Use catalog value for {field}"
- "Edited" Badge with `aria-label="You have edited this field"` when `customerEdited=true`
- "{N} fields unchanged (not shown)" summary below table
- Footer: "Discard update" (ghost) | "Apply Update" (primary with tooltip "This will update {N} changed fields.")
- Success: toast "Profile updated to v{X.Y.Z}" + cache invalidation + onClose
- Error: toast "Update failed. {message}. Try again."

**3 tests:** diff rows rendered, customer-edited defaults, apply excluding "Use mine" — all pass.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Data] GET /api/catalog/{slug} extended with codec_js inline**
- **Found during:** Task 2 (Review step needs codec source to pre-populate textarea)
- **Issue:** Plan notes "backend's GET /api/catalog/{slug} returns `codec_js` inline" but the handler was encoding `codec.CatalogEntry` directly which has no `codec_js` field (only `codec_js_path`)
- **Fix:** Added `catalogEntryWithCodec` struct embedding `codec.CatalogEntry` + `CodecJS string`; handler now calls `codecs.CodecBySlug(slug)` and wraps result before encoding
- **Files modified:** internal/api/catalog_handler.go, web/src/lib/catalog.ts (added `codec_js` field to CatalogEntry type)
- **Commit:** d3b4785

**2. [Rule 1 - Bug] Form pre-population via useEffect not in-render conditional**
- **Found during:** Task 2 tests — tests 3 & 4 failed because form.name was empty at submit time
- **Issue:** In-render `if (entry && ...) form.reset(...)` ran during the render cycle before `useEffect` had fired; in test environments this caused form values to remain empty when Add Profile was clicked
- **Fix:** Replaced conditional form.reset in render body with `useEffect(() => { if (entry && step === 'review') form.reset({...}) }, [entry, step])` — ensures reset runs after state settles
- **Files modified:** web/src/routes/profiles/ImportFromCatalogDialog.tsx
- **Commit:** d3b4785

**3. [Rule 2 - Pattern] ImportFromCatalogDialog placed at profiles/ not device-profiles/**
- **Found during:** Task 2 (project directory scan)
- **Issue:** Plan specifies `web/src/routes/device-profiles/ImportFromCatalogDialog.tsx` but the project uses `web/src/routes/profiles/` for all profile-related routes. No `device-profiles/` directory exists.
- **Fix:** File placed at `web/src/routes/profiles/ImportFromCatalogDialog.tsx` matching existing project structure
- **Files modified:** web/src/routes/profiles/ImportFromCatalogDialog.tsx
- **Commit:** d3b4785

## Verification Results

- `web/src/routes/profiles/ImportFromCatalogDialog.test.tsx` — **4/4 PASS**
- `web/src/routes/settings/CatalogUpdateModal.test.tsx` — **3/3 PASS**
- `pnpm --dir web typecheck` — PASS
- `pnpm --dir web build` — PASS (4.41s)
- All UI-SPEC verbatim copy strings verified via grep
- Go build clean (`go build ./...`)

## Known Stubs

None — both dialogs call live backend endpoints. CatalogUpdateModal diff computation is client-side but data comes from live `/api/catalog/{slug}` + `/api/device-profiles/{id}` queries.

## Threat Flags

| Flag | File | Description |
|------|------|-------------|
| threat_flag: data-exposure | internal/api/catalog_handler.go | GET /api/catalog/{slug} now returns full codec_js source inline. Auth-gated (ActionCatalogRead, viewer+admin) — same as before, but response body is larger. T-07-06-02 still applies (admin-only dialog trigger). |

## Self-Check: PARTIAL (Task 4 checkpoint pending operator verification)

Files verified on disk:
- web/src/routes/profiles/ImportFromCatalogDialog.tsx: FOUND
- web/src/routes/profiles/ImportFromCatalogDialog.test.tsx: FOUND (4/4 pass)
- web/src/routes/settings/CatalogUpdateModal.tsx: FOUND
- web/src/routes/settings/CatalogUpdateModal.test.tsx: FOUND (3/3 pass)
- web/src/lib/catalog.ts: FOUND (importFromCatalog, applyCatalogUpdate)
- internal/api/catalog_handler.go: FOUND (catalogEntryWithCodec)

Commits verified:
- 86ef372: feat(07-06): add importFromCatalog + applyCatalogUpdate API client mutations
- d3b4785: feat(07-06): implement ImportFromCatalogDialog 3-step flow (Surface 2)
- 47c1274: feat(07-06): implement CatalogUpdateModal per-field diff + toggles (Surface 3)
