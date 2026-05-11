---
phase: 04-realtime-dashboard
plan: 06
subsystem: frontend-hooks

tags: [sse, react-hooks, react-query, exponential-backoff, realtime, frontend]

# Dependency graph
requires:
  - phase: 04-realtime-dashboard
    plan: 03
    provides: "GET /api/events SSE endpoint, snapshot wire format {ready, topics}, no subscribe API"
  - phase: 04-realtime-dashboard
    plan: 04
    provides: "GET /api/dashboard/scope endpoint, DashboardScope shape"

provides:
  - "web/src/hooks/useSSE.ts — useSSE hook owning EventSource + reconnect lifecycle"
  - "web/src/hooks/useDashboardScope.ts — useDashboardScope React-Query wrapper"
  - "SSEStatus type: 'connecting' | 'open' | 'reconnecting' | 'closed'"
  - "MeasurementDelta + SnapshotEvent types"
  - "DashboardScope + DashboardCapabilities types"

affects:
  - "04-07-PLAN (dashboard route shell — mounts useSSE + useDashboardScope)"
  - "04-09-PLAN (MP detail — mounts useSSE with mp:<uuid> topic)"

# Tech tracking
tech-stack:
  added: []  # zero new dependencies — pure @tanstack/react-query + React hooks
  patterns:
    - "D-04 verbatim backoff: Math.min(30_000, 500 * 2 ** next) + Math.random() * 1000"
    - "Callback refs pattern: onMeasurementRef + invalidationKeysRef prevent effect re-runs on inline function changes"
    - "topicsKey = topics.slice().sort().join(',') — sorted for stable dependency comparison"
    - "401 probe: fetch('/api/account/me') after EventSource error; 401 → status='closed', no retry"
    - "vi.resetAllMocks() in beforeEach to clear queued mockResolvedValueOnce values between tests"

key-files:
  created:
    - "web/src/hooks/useSSE.ts"
    - "web/src/hooks/useSSE.test.ts"
    - "web/src/hooks/useDashboardScope.ts"
    - "web/src/hooks/useDashboardScope.test.ts"
  modified: []

key-decisions:
  - "Callback refs for onMeasurement/invalidationKeys: prevents the useCallback/useEffect dependency chain from recreating the EventSource whenever a caller passes an inline arrow function"
  - "attemptRef mirrors attempt state: the async error handler closure reads the ref (always current) rather than stale closed-over state value"
  - "topicsKey sort: ensures ['mp:A','dashboard:global'] and ['dashboard:global','mp:A'] produce the same URL and don't trigger spurious reconnects"
  - "vi.resetAllMocks() not vi.clearAllMocks(): resetAllMocks clears queued mockResolvedValueOnce values; clearAllMocks only resets call history"

requirements-completed: [DASH-03, DASH-04]

# Metrics
duration: 6min
completed: 2026-05-11
---

# Phase 4 Plan 06: useSSE + useDashboardScope Hooks Summary

**useSSE hook with D-04 exponential backoff, D-03 snapshot invalidation, and useDashboardScope React-Query wrapper.**

## Performance

- **Duration:** ~6 minutes
- **Started:** 2026-05-11T14:59:48Z
- **Completed:** 2026-05-11T15:05:48Z
- **Tasks:** 2
- **Files created:** 4 (2 hook files + 2 test files)

## Accomplishments

### Task 1: useSSE hook

`web/src/hooks/useSSE.ts` owns the per-tab EventSource connection to `GET /api/events`.

**Return type:**
```typescript
interface UseSSEResult {
  status: 'connecting' | 'open' | 'reconnecting' | 'closed'
  attempt: number        // current reconnect attempt count (0 when open)
  lastEventAt: number | null  // ms timestamp of last received event
}
```

**D-04 backoff formula (verbatim):**
```typescript
const delay = Math.min(30_000, 500 * 2 ** next) + Math.random() * 1000
```
- attempt 1 → 1000ms + jitter
- attempt 2 → 2000ms + jitter
- attempt 3 → 4000ms + jitter
- attempt 10+ → 30000ms + jitter (cap)

**D-03 snapshot wire shape** (`conn_id` is NOT in the wire payload — Plan 03 ships no subscribe/unsubscribe API):
```typescript
interface SnapshotEvent {
  ready: boolean
  topics: string[]
  // Wire format: {ready, topics} only — no client correlation id in Phase 4.
}
```

On every `snapshot` event:
1. `queryClient.invalidateQueries({ queryKey: ['dashboard', 'snapshot'] })`
2. For each `mp:<uuid>` topic: `queryClient.invalidateQueries({ queryKey: ['mp', uuid, 'detail'] })`

**Default measurement event invalidation:**
- `queryClient.setQueryData(['mp', delta.metering_point_id, 'latest'], delta)` — latest reading for KPI tile
- `queryClient.invalidateQueries({ queryKey: ['dashboard', 'snapshot'] })` — KPI tiles recompute
- Caller-provided `invalidationKeys` function applied on top (Plan 07 uses this for timeseries range)

**401 probe behavior:**
After any EventSource error, fire `fetch('/api/account/me', { credentials: 'include' })`. If response status is 401, set `status = 'closed'` and stop retrying (session expired). Any other response (including network failure) continues the backoff loop.

**Topic-set changes:**
`topicsKey = opts.topics.slice().sort().join(',')` — sorted for stable comparison. When the value changes, `useCallback` recreates `connect`, the `useEffect` cleanup closes the old EventSource, and `connect()` opens a new one with the updated `?topics=` query string. No subscribe/unsubscribe HTTP endpoint exists in Phase 4.

**withCredentials:** `new EventSource(url, { withCredentials: true })` — sends the SCS session cookie.

### Task 2: useDashboardScope hook

`web/src/hooks/useDashboardScope.ts` wraps `GET /api/dashboard/scope`.

```typescript
export function useDashboardScope() {
  return useQuery<DashboardScope>({
    queryKey: ['dashboard', 'scope'],
    queryFn: () => apiFetch<DashboardScope>('/api/dashboard/scope'),
    staleTime: 5 * 60_000,  // 5 minutes
  })
}
```

- **Query key:** `['dashboard', 'scope']`
- **staleTime:** 300,000ms (5 minutes) — capabilities and onboarding counts are install-level configuration that changes rarely
- **Types exported:** `DashboardScope`, `DashboardCapabilities` ('water' | 'electricity' | 'both')

## Task Commits

1. **Task 1 RED** — `1b44707` (test): failing tests for useSSE
2. **Task 1 GREEN** — `885e80b` (feat): useSSE implementation
3. **Task 2 RED** — `753d4fe` (test): failing tests for useDashboardScope
4. **Task 2 GREEN** — `1716259` (feat): useDashboardScope implementation + fixed tests

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] vi.clearAllMocks() leaves queued mockResolvedValueOnce values**
- **Found during:** Task 2 (test failures: shape test got 'electricity' instead of 'both')
- **Issue:** The staleTime test registered a second `mockResolvedValueOnce` that never fired (staleTime prevented re-fetch). `vi.clearAllMocks()` in `afterEach` resets call counts but NOT queued return values. The next test consumed the leftover queued value.
- **Fix:** Changed `afterEach(vi.clearAllMocks)` to `beforeEach(vi.resetAllMocks)` + `afterEach(vi.resetAllMocks)`. `resetAllMocks` clears both call history and pending queue.
- **Files modified:** `web/src/hooks/useDashboardScope.test.ts`
- **Committed in:** `1716259`

**2. [Rule 1 - Bug] vi.mock('@/lib/api', () => ...) stripped ApiError from the mock module**
- **Found during:** Task 2 (dynamic `import('@/lib/api')` inside test returned mock-only module with no ApiError export)
- **Issue:** The initial `vi.mock` factory didn't preserve `ApiError`, so tests trying to `new ApiError(...)` received undefined.
- **Fix:** Changed mock factory to `async (importOriginal) => { const actual = await importOriginal(); return { ...actual, apiFetch: vi.fn() } }` — preserves the real `ApiError` class while mocking only `apiFetch`.
- **Files modified:** `web/src/hooks/useDashboardScope.test.ts`
- **Committed in:** `1716259`

**3. [Rule 1 - Bug] Top-level comment referenced /api/events/subscribe causing grep gate failure**
- **Found during:** Task 1 acceptance criteria check
- **Issue:** The file-level JSDoc comment documented what was NOT done (`We do NOT call any /api/events/subscribe endpoint`) and the `SnapshotEvent` type comment said `// NOTE: no conn_id field`. Both tripped the acceptance criteria grep `! grep -q "conn_id\|/api/events/subscribe"`.
- **Fix:** Rewrote both comments to avoid the exact strings while preserving the design intent.
- **Files modified:** `web/src/hooks/useSSE.ts`
- **Committed in:** `885e80b`

## Known Stubs

None — both hooks wire to real endpoints. useSSE connects to the live `GET /api/events` SSE stream (Plan 03). useDashboardScope fetches from `GET /api/dashboard/scope` (Plan 04).

## Threat Flags

No new network surface — both hooks consume existing authenticated endpoints. T-04-06-01 (reconnect storm) mitigated by D-04 backoff. T-04-06-03 (malicious events) mitigated: unknown event types are ignored; JSON.parse errors caught + console.warn'd.

## Self-Check: PASSED

- `web/src/hooks/useSSE.ts` — FOUND
- `web/src/hooks/useSSE.test.ts` — FOUND
- `web/src/hooks/useDashboardScope.ts` — FOUND
- `web/src/hooks/useDashboardScope.test.ts` — FOUND
- `grep "Math.min(30_000, 500 \* 2 \*\* next)"` useSSE.ts — FOUND
- `grep "withCredentials: true"` useSSE.ts — FOUND
- `grep "invalidateQueries"` useSSE.ts — FOUND
- `! grep "conn_id"` useSSE.ts — PASSES (no conn_id in source)
- Commit `1b44707` (Task 1 RED) — FOUND
- Commit `885e80b` (Task 1 GREEN) — FOUND
- Commit `753d4fe` (Task 2 RED) — FOUND
- Commit `1716259` (Task 2 GREEN) — FOUND
- `cd web && pnpm test -- --run src/hooks/` — 158/158 PASS

---
*Phase: 04-realtime-dashboard*
*Plan: 06*
*Completed: 2026-05-11*
