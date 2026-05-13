---
phase: quick
plan: 260513-pkp
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/db/queries/devices.sql
  - internal/db/sqlc/devices.sql.go
  - internal/db/sqlc/querier.go
  - internal/device/handlers.go
  - internal/api/compare_handler.go
  - web/src/lib/install.ts
  - web/src/App.tsx
  - web/src/components/dashboard/ConsumptionChart.tsx
autonomous: true
requirements: []
---

<objective>
Fix 5 independent medium/low UAT issues: (1) devices list missing device_profile JOIN,
(2) compare entity labels show raw UUIDs, (3) install-state 410 noise in browser console,
(4) React Router HydrateFallback warning, (5) ConsumptionChart blank state on empty data.

Purpose: Close UAT findings from audit-phase-04 and audit-phase-05 before next phase milestone.
Output: 5 atomic commits — backend changes first, then frontend.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
</execution_context>

<context>
@.planning/STATE.md
@web/playwright/fixtures/admin-session.json

Session cookie: shifter_session=w2E8x8ZOtMpJpVH3w9KylWNpOVre7JKDeyfl_R8p4Lg
App URL: http://localhost:8080

Rebuild command (Tasks 1 + 2): docker compose -f compose/bundled.yml build shifter && docker compose -f compose/bundled.yml up -d shifter
sqlc regenerate command: sqlc generate (run from repo root — sqlc.yaml is at root; outputs to internal/db/sqlc/)
</context>

<tasks>

<task type="auto">
  <name>Task 1: Add device_profile JOIN to ListDevicesFiltered SQL + expose new fields</name>
  <files>
    internal/db/queries/devices.sql
    internal/db/sqlc/devices.sql.go
    internal/db/sqlc/querier.go
    internal/device/handlers.go
  </files>
  <action>
**Step A — Edit `internal/db/queries/devices.sql`:**

In the `ListDevicesFiltered` query, add a second CTE or directly join `device_profile` to the main SELECT.

Change the SELECT column list to add these two new fields after `d.updated_at`:
```sql
    dp.name AS device_profile_name,
    dp.expected_interval_s
```

Change the FROM clause to add a LEFT JOIN after the existing `LEFT JOIN active_bindings ab ON ab.device_id = d.id`:
```sql
LEFT JOIN device_profile dp ON dp.id = d.device_profile_id
```

The full modified query SELECT block (columns section only — preserve all WHERE/ORDER BY clauses unchanged):
```sql
SELECT
    d.id,
    d.dev_eui,
    d.name,
    d.device_profile_id,
    d.cs_device_uuid,
    d.join_eui,
    d.description,
    d.last_seen_at,
    d.decommissioned_at,
    d.created_at,
    d.updated_at,
    ab.site_id   AS current_site_id,
    ab.site_name AS current_site_name,
    dp.name AS device_profile_name,
    dp.expected_interval_s
FROM device d
LEFT JOIN active_bindings ab ON ab.device_id = d.id
LEFT JOIN device_profile dp ON dp.id = d.device_profile_id
```

**Step B — Regenerate sqlc:**
```bash
sqlc generate
```
This regenerates `internal/db/sqlc/devices.sql.go` and `internal/db/sqlc/querier.go`.
After generation, `ListDevicesFilteredRow` will have two new fields:
- `DeviceProfileName *string` (nullable because LEFT JOIN)
- `ExpectedIntervalS *int32` (nullable because LEFT JOIN)

**Step C — Update `internal/device/handlers.go` `filteredDevicesToJSON`:**

Add two new keys to the map literal inside `filteredDevicesToJSON` (after `"current_site_name"`):
```go
"device_profile_name":  derefString(r.DeviceProfileName),
"expected_interval_s":  r.ExpectedIntervalS,
```

Also update the frontend `Device` interface in `web/src/lib/devices.ts` to add the two optional fields:
```typescript
device_profile_name?: string | null
expected_interval_s?: number | null
```

**Step D — Rebuild container:**
```bash
docker compose -f compose/bundled.yml build shifter && docker compose -f compose/bundled.yml up -d shifter
```
(Cap at 3 minutes. If rebuild times out, verify with `go test ./internal/device/... -run TestListDevices -count=1` instead.)
  </action>
  <verify>
After rebuild: curl -sS -b "shifter_session=w2E8x8ZOtMpJpVH3w9KylWNpOVre7JKDeyfl_R8p4Lg" http://localhost:8080/api/devices | python3 -c "import json,sys; d=json.load(sys.stdin); r=d.get('rows',d) if isinstance(d,dict) else d; print({k:r[0].get(k) for k in ['dev_eui','device_profile_name','expected_interval_s']})"

Expected output contains non-null `device_profile_name` (e.g., `"Itron Cyble 5"`) and `expected_interval_s` (e.g., `3600`) for the seeded Itron meters.

If rebuild timed out: `go test ./internal/device/... -count=1 -run .` passes.
  </verify>
  <done>
`/api/devices` response rows include `device_profile_name` (string) and `expected_interval_s` (integer) with non-null values for seeded devices.
  </done>
</task>

<task type="auto">
  <name>Task 2: Resolve compare entity labels from UUID to human name</name>
  <files>
    internal/api/compare_handler.go
  </files>
  <action>
`labelForEntity` at line 244 currently returns `entityType + ":" + id.String()`. Fix it to perform a DB lookup.

`compare_handler.go` already receives a `*pgxpool.Pool` via `CompareDeps`. The existing sqlc queries `GetSite` (returns `Site` with `.Name string`) and `GetMP` (returns `MeteringPoint` with `.Name string`) are already available on the `Querier` interface.

**Changes to `compare_handler.go`:**

1. Change `labelForEntity` signature to accept context and `*sqlc.Queries`:
```go
func labelForEntity(ctx context.Context, q *sqlc.Queries, entityType string, id uuid.UUID) string {
    pgID := pgtype.UUID{Bytes: id, Valid: true}
    switch entityType {
    case "site":
        if site, err := q.GetSite(ctx, pgID); err == nil {
            return site.Name
        }
    default: // "metering_point"
        if mp, err := q.GetMP(ctx, pgID); err == nil {
            return mp.Name
        }
    }
    // Fallback: UUID string (entity not found or DB error)
    return entityType + ":" + id.String()
}
```

2. Update the two call sites in `handleEntitiesMode` to pass ctx and q:
```go
aLabel := labelForEntity(ctx, q, req.EntityType, aID)
bLabel := labelForEntity(ctx, q, req.EntityType, bID)
```

3. Update the call site in `handleTimeRangesMode`:
```go
label := labelForEntity(ctx, q, req.EntityType, entityID)
```

Do NOT add a new sqlc query — GetSite and GetMP already exist. Do NOT change any other function signatures. The `q *sqlc.Queries` variable is already in scope in both handle* functions.

**Rebuild container:**
```bash
docker compose -f compose/bundled.yml build shifter && docker compose -f compose/bundled.yml up -d shifter
```
  </action>
  <verify>
After rebuild, POST to compare with a known metering point UUID from the seeded data:

```bash
# Get a metering point ID first
MP_ID=$(curl -sS -b "shifter_session=w2E8x8ZOtMpJpVH3w9KylWNpOVre7JKDeyfl_R8p4Lg" \
  http://localhost:8080/api/metering-points | python3 -c "import json,sys; d=json.load(sys.stdin); print(d[0]['id'])" 2>/dev/null || \
  curl -sS -b "shifter_session=w2E8x8ZOtMpJpVH3w9KylWNpOVre7JKDeyfl_R8p4Lg" \
  "http://localhost:8080/api/devices" | python3 -c "import json,sys; pass" )

# Use any two MP IDs from the system (check /api/devices or /api/sites response)
curl -sS -b "shifter_session=w2E8x8ZOtMpJpVH3w9KylWNpOVre7JKDeyfl_R8p4Lg" \
  -X POST -H "Content-Type: application/json" \
  -d '{"mode":"entities","entity_type":"metering_point","entity_a_id":"<MP_UUID_A>","entity_b_id":"<MP_UUID_B>","range":{"from":"2025-01-01T00:00:00Z","to":"2025-12-31T23:59:59Z"}}' \
  http://localhost:8080/api/reports/compare | python3 -c "import json,sys; d=json.load(sys.stdin); print('a.label=',d['a']['label'],'b.label=',d['b']['label'])"
```

Verify: `a.label` and `b.label` are human names (e.g., `"Site A - Meter 1"`) NOT UUIDs like `"metering_point:abc-123"`.

Alternative quick check: `go build ./internal/...` succeeds (no compile errors).
  </verify>
  <done>
POST /api/reports/compare response: `a.label` and `b.label` contain the metering point or site name, not `"metering_point:{uuid}"` or `"site:{uuid}"`.
  </done>
</task>

<task type="auto">
  <name>Task 3: Cache install-complete signal in sessionStorage to eliminate 410 console noise</name>
  <files>
    web/src/lib/install.ts
  </files>
  <action>
The browser logs a network-level `Failed to load resource: 410 (Gone)` for every call to `/api/install/state` even though the JS `catch` block handles it cleanly. The fix: after the first 410, cache a flag in `sessionStorage` and skip the network call on subsequent invocations.

**Replace `fetchInstallState` in `web/src/lib/install.ts`:**

```typescript
const INSTALL_DONE_KEY = 'shifter_install_completed'

/** Returns null when install is already completed (backend responds 410 Gone). */
export async function fetchInstallState(): Promise<InstallState | null> {
  // Short-circuit: install was already confirmed complete this session.
  if (sessionStorage.getItem(INSTALL_DONE_KEY) === 'true') {
    return null
  }
  try {
    return await apiFetch<InstallState>('/api/install/state')
  } catch (err) {
    if (err instanceof ApiError && err.status === 410) {
      // Cache the "install complete" signal so future navigations skip the network call.
      sessionStorage.setItem(INSTALL_DONE_KEY, 'true')
      return null
    }
    throw err
  }
}
```

Also clear the key when the user logs out. In `web/src/routes/_root.tsx`, `handleSignOut` already calls `logout()` then `window.location.assign('/login')`. Add one line before the assign:

```typescript
const handleSignOut = async () => {
  try {
    await logout()
  } finally {
    sessionStorage.removeItem('shifter_install_completed')
    window.location.assign('/login')
  }
}
```

This file also needs the import: the key constant is in `install.ts` — but since `_root.tsx` already imports from `@/lib/install`, export the constant:

```typescript
// install.ts — export so _root.tsx can reference it on logout
export const INSTALL_DONE_KEY = 'shifter_install_completed'
```

And in `_root.tsx`:
```typescript
import { fetchInstallState, INSTALL_DONE_KEY } from '@/lib/install'
// ...
sessionStorage.removeItem(INSTALL_DONE_KEY)
```

Also handle 401: in `web/src/lib/api.ts` the 401 handler already does `window.location.assign('/login?next=...')`. sessionStorage is NOT cleared there because tab navigation to /login naturally starts a new session. The sessionStorage key persists for the tab lifetime, which is correct — a 401 means the session expired, not that the install state changed.

Files modified:
- `web/src/lib/install.ts` — add constant export + sessionStorage logic
- `web/src/routes/_root.tsx` — clear key on logout
  </action>
  <verify>
Build: `cd web && pnpm build` succeeds (no TypeScript errors).

Runtime test: Open browser DevTools → Network tab → navigate to http://localhost:8080 (with valid admin session). Observe:
1. First navigation: one GET /api/install/state request appears (returns 410, JS handles it).
2. Navigate to /devices, /sites, /gateways (3+ page navigations): zero additional /api/install/state requests in Network tab.
3. Console tab: zero "Failed to load resource: 410" errors after the first navigation.
  </verify>
  <done>
After first navigation, subsequent protected route navigations produce zero `/api/install/state` network requests. Console is clean of 410 noise across multi-page sessions.
  </done>
</task>

<task type="auto">
  <name>Task 4: Add HydrateFallback to root route to silence React Router v7 warning</name>
  <files>
    web/src/App.tsx
  </files>
  <action>
React Router v7 warns: "No `HydrateFallback` element provided to render during initial hydration" because the root route definition at `id: 'root'` in `createBrowserRouter` has no `HydrateFallback` property.

In `web/src/App.tsx`, update the root route object (the one with `id: 'root'`, `path: '/'`, `element: <RootLayout />`) to add `HydrateFallback`:

```typescript
import { Skeleton } from '@/components/ui/skeleton'

// Inside createBrowserRouter([...]):
{
  id: 'root',
  path: '/',
  element: <RootLayout />,
  loader: rootLoader,
  HydrateFallback: () => <Skeleton className="h-screen w-full" />,
  children: [
    // ... unchanged
  ],
},
```

`Skeleton` is already available via shadcn (`web/src/components/ui/skeleton.tsx` exists — it's used by CumulativeChartCard). Import it at the top of App.tsx alongside existing imports.

Do NOT change any other route definitions. Do NOT add HydrateFallback to auth routes (login/install) — they use `<AuthLayout />` which has its own loading state.
  </action>
  <verify>
Build: `cd web && pnpm build` succeeds.

Runtime: Navigate to http://localhost:8080 and http://localhost:8080/login in browser. Open DevTools → Console. Confirm zero occurrences of "No `HydrateFallback`" warning or any React Router hydration warning. Test with both a fresh tab and after page reload.
  </verify>
  <done>
Browser console shows zero React Router HydrateFallback warnings on any route navigation or page reload.
  </done>
</task>

<task type="auto">
  <name>Task 5: Add empty-state to ConsumptionChart when data array is empty</name>
  <files>
    web/src/components/dashboard/ConsumptionChart.tsx
  </files>
  <action>
When `data.length === 0`, the current component renders an AreaChart with empty data — axes display but no line is visible, the chart area appears blank/broken.

**Replace the `ConsumptionChart` component body in `web/src/components/dashboard/ConsumptionChart.tsx`:**

Add an early-return branch before the existing `return` statement. Render the empty state OUTSIDE `ChartContainer` so Recharts does not render an empty SVG canvas with axes:

```typescript
export function ConsumptionChart({ data, liveMode, bucketSec }: ConsumptionChartProps) {
  const xFormatter = (v: string) => {
    const d = new Date(v)
    if (bucketSec < 3600) return format(d, 'HH:mm')
    if (bucketSec < 86400) return format(d, 'HH:mm')
    return format(d, 'MMM d')
  }

  const lastPoint = data.length > 0 ? data[data.length - 1] : null

  // Empty state: no data for this range — render placeholder instead of blank chart.
  if (data.length === 0) {
    return (
      <div className="h-48 sm:h-56 md:h-64 lg:h-72 flex items-center justify-center">
        <p className="text-sm text-muted-foreground">No data for this range</p>
      </div>
    )
  }

  return (
    <div className="relative">
      <ChartContainer config={chartConfig} className="h-48 sm:h-56 md:h-64 lg:h-72 w-full">
        {/* ... rest of chart unchanged ... */}
      </ChartContainer>
    </div>
  )
}
```

Keep the CardContent height in `CumulativeChartCard` consistent — the empty-state div uses matching height classes (`h-48 sm:h-56 md:h-64 lg:h-72`) so the card does not collapse.

Do NOT modify `CumulativeChartCard.tsx`. The empty state lives in `ConsumptionChart.tsx` only.

Add a Playwright spec assertion to the existing dashboard specs OR create a targeted assertion:
In `web/playwright/specs/dashboard-date-range.spec.ts` (which fully passes at 9/9), confirm the today preset test can also check for either chart OR empty-state text. Do NOT modify the existing passing test structure — only add a soft assertion that does not break the existing 9/9 pass:

Actually, per constraints, do NOT add new tests that could break existing passing specs. Just ensure the component renders correctly. The verify step will confirm via manual check.
  </action>
  <verify>
Build: `cd web && pnpm build` succeeds.

Playwright check:
```bash
cd web && pnpm exec playwright test dashboard-date-range.spec.ts --project=chromium 2>&1 | tail -5
```
Existing 9/9 tests must still pass (the empty-state div has matching height so card layout is unchanged).

Visual confirmation: Open http://localhost:8080 in browser with admin session. With default `range=today` and no live uplinks (Itron mocks are stale), the chart card should show "No data for this range" text instead of a blank AreaChart canvas.

Selector check (browser console or Playwright):
```javascript
document.querySelector('.text-muted-foreground')?.textContent
// Expected: "No data for this range"
```
  </verify>
  <done>
`ConsumptionChart` renders "No data for this range" (muted text, vertically centered) when `data.length === 0`. No blank/empty recharts canvas is shown. Existing dashboard-date-range Playwright suite remains 9/9 pass.
  </done>
</task>

</tasks>

<verification>
After all 5 tasks:
1. `go build ./...` — no compile errors (Tasks 1+2)
2. `cd web && pnpm build` — no TypeScript errors (Tasks 3+4+5)
3. `go test ./internal/device/... -count=1` — passes (Task 1 SQL change is correct)
4. `go test ./internal/api/... -count=1` — passes (Task 2 handler change is correct)
5. Browser console on http://localhost:8080: zero HydrateFallback warnings (Task 4), zero repeated 410 errors (Task 3)
6. Dashboard with range=today shows "No data for this range" text instead of blank chart (Task 5)
7. /api/devices response includes `device_profile_name` and `expected_interval_s` (Task 1)
8. /api/reports/compare response `a.label` / `b.label` contain entity names not UUIDs (Task 2)
</verification>

<success_criteria>
- [ ] `ListDevicesFiltered` returns `device_profile_name` and `expected_interval_s` for seeded Itron meters
- [ ] POST /api/reports/compare labels are human names, not `"metering_point:{uuid}"`
- [ ] Only one /api/install/state network request per browser session (first nav only)
- [ ] Zero React Router HydrateFallback console warnings on any route
- [ ] ConsumptionChart shows "No data for this range" when data is empty, not a blank canvas
- [ ] `go build ./...` and `cd web && pnpm build` both succeed
</success_criteria>

<output>
After completion, create `.planning/quick/260513-pkp-fix-5-medium-low-uat-issues-install-stat/260513-pkp-SUMMARY.md`
with what was changed, any deviations from the plan, and final test results.
</output>
