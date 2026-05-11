# Phase 4: Realtime & Dashboard — Research

**Researched:** 2026-05-11
**Domain:** SSE realtime fan-out over Postgres `LISTEN/NOTIFY` + adaptive dashboard + per-meter detail (Go 1.25 backend, React 19 / Vite SPA frontend, TimescaleDB hypertable)
**Confidence:** HIGH for the bulk of the stack (every dependency already locked by Phases 1–3); MEDIUM for the SSE wire-protocol micro-decisions and the schema-gap items called out under §Schema Gaps.

## Summary

Every backend and frontend dependency Phase 4 needs is already in the repo. The job is **wiring, not stack selection**:

1. Mount `GET /api/events` (SSE) under the same `chi.Router` + `SessionMgr.LoadAndSave` + `auth.RequireAction` chain Phases 1–3 use, plus 5 new REST endpoints (`/api/dashboard/snapshot`, `/api/dashboard/timeseries`, `/api/metering-points/:id/detail`, `/api/metering-points/:id/timeseries`, `/api/metering-points/:id/uplinks`) — same `Deps` struct pattern as `GatewayDeps`/`ImportDeps`.
2. Add a new Go package (`internal/events/` recommended) that mirrors `internal/resolver/listener.go` but `LISTEN measurement_inserted` and fans each notification out to a per-session topic registry. The existing resolver listener is the canonical shape — reuse 100% of its `Run(ctx, pool, log)` reconnect loop, just swap the channel name and the in-process broadcast.
3. Add **two new migrations**: `0021_measurement_inserted_trigger` (AFTER INSERT trigger emitting `pg_notify('measurement_inserted', json_build_object(...))` on the parent hypertable — TimescaleDB propagates parent-table triggers to chunks automatically) and `0022_install_capabilities` (single new column on `install_identity` for D-09 adaptive scope).
4. **Schema gap to surface to the planner:** CONTEXT D-07 references `device_profile.expected_interval_s`, which does not exist in 0009 today. The plan must add this column (either inside the 0022 migration or a separate one) AND backfill it for the seeded profiles (`internal/profile/seed.go`).
5. Frontend: replace `IndexRedirect → /settings` with a real `DashboardPage` (component does NOT exist), build the 3-tab MP detail page on top of the existing Phase 2 minimal page, add a Dashboard nav row to the sidebar, and write a thin `useSSE` hook that owns an `EventSource` per `RouterProvider` tree with manual exponential-backoff-with-jitter (native `EventSource.onerror` reconnect has no backoff control). React-Query is the source of truth for snapshot/REST data; SSE deltas trigger `queryClient.invalidateQueries` or surgical `setQueryData` writes.

**Primary recommendation:** Treat SSE events as **invalidation hints with embedded compact deltas** — the compact 7-field NOTIFY payload (D-02) is enough for KPI tiles to update without a refetch, while heavier surfaces (timeseries chart re-buckets, Advanced-tab full decoded JSON) invalidate the matching React-Query key and refetch from the REST endpoints. Source of truth stays on the server; the SSE pipe is for low-latency updates and "something changed" signals.

## User Constraints (from CONTEXT.md)

### Locked Decisions

**SSE wire protocol & snapshot semantics**

- **D-01:** **One session-scoped `/api/events` stream + server-side topic filter.** Browser opens a single `EventSource('/api/events')` per tab. Client `POST /api/events/subscribe` (or `?topics=` on initial connect) lists topics it cares about — `dashboard:global`, `mp:<uuid>`. Backend keeps a `connection → topics` map and filters before write.
- **D-02:** **Compact pg_notify payload — `(metering_point_id, time, cumulative_value, instant_value, quality, battery_pct, rssi)`** as JSON ≤200B. KPI tiles + detail-page latest-reading widget update directly from the event; no follow-up REST fetch needed on the hot path. Trigger SQL lives in a new migration (slot reserved at `0015_measurement.up.sql:L78-81`).
- **D-03:** **Always-full-snapshot on initial connect AND every reconnect.** Server emits `event: snapshot` carrying `latest-reading-per-MP` for every subscribed topic, then resumes streaming `event: measurement` deltas. No Last-Event-ID cursor, no replay ring buffer.
- **D-04:** **Client reconnect = exponential backoff with jitter — `min(30s, 0.5s * 2^n) + rand(0, 1s)`.** Server heartbeat = SSE comment `: heartbeat` every 30s. Reverse proxies (Caddy `flush_interval -1` already set in Phase 1 D-20..22) don't drop idle connections and the client can detect zombie sessions.

**KPI definitions & online/offline rules**

- **D-05:** **"Today's consumption" = `[00:00 today install_tz → now]`** computed against `install_identity.timezone`.
- **D-06:** **"Current flow / instantaneous draw" = latest `measurement.instant_value` per MP, summed per `utility_class`.**
- **D-07:** **Online vs offline = `device.last_seen_at > now() − 2 × device_profile.expected_interval_s`.** Intentionally looser than Phase 6 ALERT-02 (N≥3 missed uplinks with hysteresis).
- **D-08:** **Period delta = `today [00:00 → now]` vs `yesterday [00:00 → same-time-yesterday]`.** Reports both absolute delta and % change.
- **D-09:** **Adaptive scope = admin-managed flag in install identity, NOT auto-detected.** New `install_identity.capabilities` column with values `water` | `electricity` | `both`.
- **D-10:** **Dashboard chart dimension = cumulative per utility class.** One chart for water (m³), one for electricity (kWh).

**Date-range pickers & raw-chart strategy**

- **D-11:** **Presets: Today / 24h / 7d / 30d / Custom.** Custom uses shadcn DatePicker range. URL-state.
- **D-12:** **Downsampling via TimescaleDB `time_bucket()` sized to the range, no CAGGs in Phase 4.** Today/24h → 5-min, 7d → 1-hour, 30d → 4-hour, Custom > 30d → 1-day.
- **D-13:** **Live-mode chart update only for `Today` and `24h` presets.**
- **D-14:** **Shared dashboard-level date-range picker. Per-MP detail-page picker is independent.**

**Per-meter detail layout, uplinks log, empty states**

- **D-15:** **Tabbed detail page — `Normal` / `Advanced` / `Uplinks log`.** Default tab is Normal.
- **D-16:** **Uplinks log = TanStack Table, default 100 rows + 'Load more' (+100 each click, capped at 500).** Virtualized list when row count > 200.
- **D-17:** **Battery + RSSI/SNR sparklines = last 24h, `time_bucket('1 hour')` (24 buckets).**
- **D-18:** **Raw payload display = hex monospace + expandable row.** JSON column rendered side-by-side.
- **D-19:** **Quality badge surfaces on the Normal tab + filter button → Uplinks log.** Click → switches to Uplinks log tab with `quality != ok` filter pre-applied.
- **D-20:** **Advanced tab = collapsible JSON tree.** Custom recursive component; no `react-json-view`.
- **D-21:** **Empty state — dashboard: progressive onboarding card** with 3 sub-states keyed on `(gateway_count, device_count, uplink_count)` tuple.
- **D-22:** **MP detail with no binding/uplink: keep tabs visible.** Normal tab shows MP info card + "No device bound — [Add device]" CTA. Advanced + Uplinks log disabled with tooltip.
- **D-23:** **Viewer authorization = full read access on dashboard + detail.** Phase 4 introduces no mutations.

### Claude's Discretion

- Exact `time_bucket` widths inside each preset (tunable; D-12 numbers are the starting point)
- KPI tile layout grid (shadcn Card sizes, ordering — water-first vs electricity-first)
- Sparkline visual styling (Recharts area vs line, color)
- JSON tree component design — depth indentation, primitive type colors
- Hex-payload column width and copy-to-clipboard placement
- Snapshot event payload exact field ordering / pagination strategy if a subscribed topic returns >1000 MPs
- SSE backpressure policy when a client can't keep up (drop oldest delta + force re-snapshot vs drop connection)
- Per-tab SSE topic management (Uplinks log tab subscribes to `mp:<uuid>:uplinks`; Normal tab subscribes to `mp:<uuid>` — naming convention is planner's choice)
- Whether `device.last_seen_at` is good enough for D-07 or whether a per-binding `last_uplink_at` view is needed

### Deferred Ideas (OUT OF SCOPE)

- Continuous aggregates for chart performance — Phase 5 (DATA-11..13)
- Auto-detect adaptive scope from `SELECT DISTINCT utility_class` — pure-auto deferred
- Last-Event-ID cursor replay for SSE reconnect — chose always-full-snapshot (D-03)
- Per-page SSE endpoints vs session-scoped — chose session-scoped (D-01)
- WebSocket (bidirectional) — Phase 4 is one-way
- Saved dashboard views / per-user date-range preferences — V2-AUTH-02
- Site-grouped dashboard breakdown — Phase 5
- Quality-flag drill-down beyond the badge — Phase 7
- Custom user-tunable bucket widths — Phase 5/7
- Detail-page admin-only gate on Advanced/Uplinks log tab — Phase 4 viewers see everything
- Pre-compute "latest-reading-per-MP" materialized view — start with on-demand sqlc query
- Server backpressure policy (snapshot reissue vs disconnect) — left to Claude's Discretion
- Mobile native dashboard — V2-MOB-01
- Hex / decoded_object copy-to-clipboard buttons — small polish, planner can defer

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| DASH-01 | Dashboard adapts to install scope (water / electricity / both) | §Architecture Pattern 4 (Capability detection); D-09 lock-in; §Schema Gaps (`install_identity.capabilities`) |
| DASH-02 | Live KPIs: today's consumption, current flow / instantaneous draw, period delta, online/offline device count | §Architecture Pattern 5 (KPI SQL); §Schema Gaps (`device_profile.expected_interval_s`); D-05..D-08 |
| DASH-03 | SSE driven by Postgres LISTEN/NOTIFY from a hypertable insert trigger — UI only sees persisted data | §Architecture Pattern 1 (LISTEN/NOTIFY → SSE fan-out); §Architecture Pattern 2 (AFTER INSERT trigger fires inside same tx as INSERT — NOTIFY commits with the tx) |
| DASH-04 | SSE client reconnects with exponential backoff + jitter; reconnect delivers fresh snapshot | §Architecture Pattern 3 (custom EventSource wrapper or `@microsoft/fetch-event-source`); D-03 + D-04 |
| DASH-05 | Time-series charts with date-range pickers | §Architecture Pattern 6 (`time_bucket` + shadcn date-range recipe); D-11 + D-12 |
| DASH-06 | Fully usable on mobile-sized viewports | §Mobile Responsiveness; existing shadcn responsive grid patterns from Phase 1/3 |
| DETL-01 | Normal + Advanced collapsible view exposing every JSONB field | §Architecture Pattern 7 (Custom recursive JSON tree); D-15 + D-20 |
| DETL-02 | Last 100–500 uplink events with timestamp, raw payload, decoded object, signal stats | §Architecture Pattern 8 (Hypertable LIMIT query + index hint); D-16 |
| DETL-03 | Battery + RSSI/SNR sparklines | §Architecture Pattern 6 (`time_bucket('1 hour')`); D-17 |

## Project Constraints (from CLAUDE.md)

| Constraint | Phase 4 impact |
|-----------|---------------|
| **Backend = Go 1.24+; gRPC to ChirpStack; sqlc + pgx/v5; chi router; SCS sessions; River for jobs; Argon2id; slog stdlib** | All present in repo; Phase 4 reuses every one. No new go.mod entries required. |
| **Frontend = Vite 7 + React 19 + Tailwind 4 + shadcn/ui (new-york + slate + custom navy); TanStack Query; TanStack Table; react-router-dom v7; recharts (already wrapped in `chart.tsx`)** | All present. Phase 4 adds `react-day-picker` only if not already pulled by shadcn (likely already transitive via the chart recipe). Verified: `web/package.json` already lists recharts 3.8.0, @tanstack/react-query 5.100.5, @tanstack/react-table 8.21.3, react-router-dom 7.14.2. |
| **Realtime delivery = SSE backed by Postgres LISTEN/NOTIFY from a hypertable insert trigger. Never MQTT to browser. UI only shows persisted data.** | This is the core wiring of the phase. Pattern proven by Phase 2's `binding_changed` listener (resolver). |
| **Modal-first CRUD (UX-01)** | Phase 4 introduces NO new dialogs (read-only phase). |
| **All `/api/*` requests carry `X-Requested-With: shifter`** | `apiFetch` already does this. SSE is GET with cookie auth — header not required because no state change. |
| **Server-side authz via `Can()` / `RequireAction`** | New routes mount inside an authenticated chi group; D-23 means both admin and viewer get the same Phase 4 surface. |
| **Single-binary deploy; SPA served by Go via `go:embed`** | Phase 4 adds frontend code that ships inside the existing `//go:embed all:web/dist` block. |
| **Hybrid wide+JSONB measurement schema** | Already exists (0015). Phase 4 reads `cumulative_value`, `instant_value`, `battery_pct`, `rssi`, `snr`, `quality`, `decoded_object`, `extra`, `raw_payload`, `fcnt`. |
| **`measurement` is keyed on `metering_point_id`, never `dev_eui`** | Phase 4 queries by `metering_point_id` only. |
| **Caddy SSE-aware (`flush_interval -1`) already configured in Phase 1 D-20..22** | No deploy changes required. |

## Standard Stack

### Core (already in go.mod / web/package.json)

| Library | Version (verified) | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go | 1.25.0 (go.mod) | Backend language | Single-binary deploy; gRPC native; mature SSE story via stdlib `net/http` `Flusher` |
| `github.com/jackc/pgx/v5` | v5.9.2 | Postgres driver + LISTEN/NOTIFY dedicated conn | `pgxpool` lets us reserve a `*pgxpool.Conn` for the listener loop; `conn.Conn().WaitForNotification(ctx)` is the canonical pattern; pattern already proven by `internal/resolver/listener.go` |
| `github.com/go-chi/chi/v5` | v5.2.5 | HTTP router | Standard chi group pattern with `RequireAction` middleware already in use |
| `github.com/alexedwards/scs/v2` | v2.9.0 | Session manager | `EventSource` sends cookies same-origin; `LoadAndSave` middleware decodes session into context for every `/api/*` request including SSE GET |
| `github.com/google/uuid` | v1.6.0 | UUID parsing for `mp:<uuid>` topic ids | Already imported |
| `log/slog` | stdlib | Logger | Already used everywhere |
| React | 19.x | Frontend | Phase 1 lock; required by react-leaflet 5 + recharts 3 peer deps |
| Vite | 7.x | Frontend build | Phase 1 lock |
| `@tanstack/react-query` | v5.100.5 | Server state + cache invalidation on SSE deltas | Already used by Phase 2/3 list/detail pages |
| `@tanstack/react-table` | v8.21.3 | Uplinks log table | Phase 3 D-12..D-18 column-shape patterns reuse verbatim |
| `react-router-dom` | v7.14.2 | Routing + `useSearchParams` for URL state | Phase 3 D-15 URL-state pattern reuse for date-range + quality filter |
| `recharts` | 3.8.0 | Charts (cumulative + sparklines) | Already wrapped by `web/src/components/ui/chart.tsx` (`ChartContainer`, `ChartTooltip`, `ChartLegend` exports verified) |
| `zod` | v4.3.6 | URL params parsing | Phase 3 reuse pattern |
| `date-fns` | v4.1.0 | Date math for range pickers + period delta labels | Already in bundle |
| `sonner` | v2.0.7 | Toasts | Already in bundle |
| `lucide-react` | latest | Icons | All Phase 4 icons listed in `04-UI-SPEC.md` §Iconography are stock lucide |
| `tailwindcss` | 4.x | Styling | Phase 1 lock |

### Supporting — verify these are already present; add if missing

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `react-day-picker` | v9.x (peer of shadcn calendar) | Backing the shadcn `<Calendar mode="range">` recipe used by the Custom date-range picker | Required only when the Custom preset is selected. Verify presence with `grep react-day-picker web/package.json`; if absent, add via `pnpm dlx shadcn@latest add calendar date-picker` (shadcn CLI pulls the peer dep) |
| `@tanstack/react-virtual` | v3.x | Virtualization of Uplinks log when rows > 200 (D-16) | NOT currently in `web/package.json` — planner must add it before the Uplinks log virtualization milestone. `pnpm --filter web add @tanstack/react-virtual` |
| `@microsoft/fetch-event-source` | v2.x | OPTIONAL — better-controlled SSE client | Recommended if planner picks "fetch-based SSE" route; lets us set the exponential backoff policy without subclassing `EventSource`. **A pure-stdlib `EventSource` wrapper is also viable and is the lower-dependency choice.** See §Architecture Pattern 3 |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| Postgres LISTEN/NOTIFY → SSE | WebSocket | Bidirectional; not needed in Phase 4. Deferred. |
| Postgres LISTEN/NOTIFY → SSE | Read MQTT directly in the browser | Leaks broker auth + entire telemetry topic tree into the browser. CLAUDE.md explicitly forbids. |
| Native `EventSource` + manual backoff | `@microsoft/fetch-event-source` | Native = zero new deps; fetch-based = nicer reconnect API + can send headers. We can ship without a new dep if we wrap `EventSource` thinly. |
| Custom JSON tree | `react-json-view` / `react18-json-view` | D-20 already locked: ship a custom component. Third-party libraries are >50 KB gzipped and don't match shadcn's modern-minimal aesthetic. |
| `pgxlisten` (https://pkg.go.dev/github.com/jackc/pgxlisten) | Inline copy of `internal/resolver/listener.go` shape | `pgxlisten` is the higher-level abstraction maintained by Jack Christensen (pgx author). The resolver's existing listener pattern is already proven, tested, and works in the codebase. **Stay with the inline pattern** to keep dependency count down and not have two LISTEN/NOTIFY mechanisms living in the codebase. |
| Recompute KPIs server-side on every NOTIFY | Client invalidate + refetch | Client refetch is cheaper at server CPU and matches the "compact NOTIFY + heavy data via REST" pattern. Servers stay stateless re: per-client KPI projections. |

**Installation summary** (only if missing — most likely just react-virtual):

```bash
pnpm --filter web add @tanstack/react-virtual
# Optional, if planner picks fetch-based SSE:
pnpm --filter web add @microsoft/fetch-event-source
# Only if react-day-picker is not yet a peer dep:
pnpm dlx shadcn@latest add calendar date-picker
```

**Version verification:**

```bash
# Run these to confirm published versions match the table above before locking the plan
npm view @tanstack/react-virtual version
npm view @microsoft/fetch-event-source version
npm view react-day-picker version
go list -m github.com/jackc/pgx/v5
go list -m github.com/go-chi/chi/v5
```

## Schema Gaps

> **PLANNER MUST ADDRESS** — these are missing pieces of the codebase that CONTEXT D-07 / D-09 / DASH-02 assume exist.

| Item | Where CONTEXT references it | Current state in repo | Action |
|------|----------------------------|----------------------|--------|
| `device_profile.expected_interval_s` | CONTEXT D-07 ("Online vs offline = `device.last_seen_at > now() − 2 × device_profile.expected_interval_s`") and CONTEXT canonical refs (`internal/db/migrations/0009_device_profile.up.sql` — D-07 source) | **Does NOT exist** in `0009_device_profile.up.sql`. Verified: `grep expected_interval internal/db/migrations/*.up.sql` returns zero hits. Phase 2 research only mentioned "per-device-profile expected interval" as a Phase 6 ALERT-02 concern. | New migration adds `expected_interval_s INTEGER NOT NULL DEFAULT 3600 CHECK (expected_interval_s > 0)` to `device_profile`. Backfill the 3 seeded profiles in `internal/profile/seed.go` (Axioma W1 typically 3600s, Acrel ADW300 typically 60–300s — confirm against datasheets at plan time). Surface this column in the device-profile editor UI (or accept defaults for Phase 4 and defer the UI to Phase 7). |
| `install_identity.capabilities` | CONTEXT D-09 ("New `install_identity.capabilities` column or equivalent — planner picks shape") | **Does NOT exist** in `0005_install_identity.up.sql`. | New migration adds `capabilities TEXT NOT NULL DEFAULT 'both' CHECK (capabilities IN ('water','electricity','both'))`. Default = `'both'` lets every existing install upgrade without explicit reconfig; the Settings adaptive-scope toggle (UI-SPEC) lets admin narrow later. |
| `measurement_inserted` trigger | CONTEXT D-02 + code anchor `internal/db/migrations/0015_measurement.up.sql:L78-81` ("slot reserved for the `measurement_inserted` LISTEN/NOTIFY trigger added in Phase 4") | **Comment-only placeholder** at L78-81 of 0015; no trigger exists. | New migration `0021_measurement_inserted_trigger.up.sql` creates `AFTER INSERT FOR EACH ROW EXECUTE FUNCTION measurement_notify()` on the parent hypertable. Function body builds the D-02 compact 7-field JSON via `json_build_object` and emits `PERFORM pg_notify('measurement_inserted', payload::text)`. TimescaleDB propagates parent-table triggers to chunks automatically (verified — see §Architecture Pattern 2). |
| Settings adaptive-scope toggle | UI-SPEC §Settings adaptive-scope toggle | **Does NOT exist** in `web/src/routes/settings.tsx`. | Frontend adds a new row in the Install Identity settings card with a `<Select>` writing to a new PATCH endpoint (or extends an existing PATCH `install_identity` if one exists — verify at plan time). |
| Sidebar Dashboard nav item | UI-SPEC §Sidebar nav + code anchor `web/src/components/shell/sidebar.tsx` comment "Phase 4 will insert Dashboard above all" | **Reserved slot** with comment in `sidebar.tsx`; no nav row. | Frontend adds `{ to: '/', label: 'Dashboard', icon: LayoutDashboard }` at index 0 of the `NAV` array. |
| `IndexRedirect` → `DashboardPage` | UI-SPEC §Dashboard page | **`web/src/routes/index-redirect.tsx` still redirects to `/settings`** (verified: 5 lines, `<Navigate to="/settings" replace />`). | Replace with a real `DashboardPage` route component. Update `App.tsx` lazy registration. |

## Architecture Patterns

### Recommended Project Structure

```
internal/
├── events/                      # NEW — SSE fan-out service + LISTEN measurement_inserted
│   ├── doc.go                   # Package doc mirroring resolver/doc.go shape
│   ├── listener.go              # near-copy of resolver/listener.go, channel='measurement_inserted'
│   ├── hub.go                   # in-process pub/sub: connection→topics registry + Broadcast()
│   ├── handler.go               # GET /api/events SSE handler + POST /api/events/subscribe
│   └── *_test.go                # unit + integration tests
├── dashboard/                   # NEW — REST endpoints feeding the dashboard
│   ├── snapshot_handler.go      # GET /api/dashboard/snapshot
│   ├── timeseries_handler.go    # GET /api/dashboard/timeseries
│   └── kpi.go                   # sqlc-wrapped KPI computations (today's consumption, period delta, online count)
├── meteringpoint/               # EXISTS — extend with detail + timeseries + uplinks handlers
│   ├── detail_handler.go        # NEW: GET /api/metering-points/:id/detail
│   ├── timeseries_handler.go    # NEW: GET /api/metering-points/:id/timeseries
│   └── uplinks_handler.go       # NEW: GET /api/metering-points/:id/uplinks
├── db/
│   ├── migrations/
│   │   ├── 0021_measurement_inserted_trigger.up.sql   # NEW
│   │   ├── 0022_install_capabilities.up.sql           # NEW
│   │   └── 0023_device_profile_expected_interval.up.sql # NEW — schema gap
│   └── queries/
│       ├── dashboard.sql        # NEW — KPI queries
│       ├── timeseries.sql       # NEW — time_bucket query
│       └── uplinks.sql          # NEW — paginated uplinks log

web/src/
├── routes/
│   ├── index.tsx                # NEW — DashboardPage (replaces IndexRedirect)
│   ├── metering-points/
│   │   └── $id.tsx              # EXTEND — replace Phase 2 minimal page with 3-tab layout
│   └── settings.tsx             # EXTEND — add adaptive-scope <Select> row
├── components/
│   ├── shell/
│   │   └── sidebar.tsx          # EXTEND — add Dashboard NAV row at index 0
│   ├── dashboard/
│   │   ├── kpi-tile.tsx         # NEW
│   │   ├── kpi-grid.tsx         # NEW — adaptive layout per D-09
│   │   ├── cumulative-chart-card.tsx  # NEW
│   │   ├── date-range-picker.tsx      # NEW — segmented preset + Custom popover
│   │   ├── live-channel-banner.tsx    # NEW — reconnecting / paused / auth-expired
│   │   └── empty-state-onboarding.tsx # NEW — 3-step staged onboarding
│   ├── metering-point/
│   │   ├── normal-tab.tsx       # NEW
│   │   ├── advanced-tab.tsx     # NEW — collapsible JSON tree
│   │   ├── uplinks-log-tab.tsx  # NEW — TanStack Table + virtualizer
│   │   ├── json-tree.tsx        # NEW — custom recursive component (D-20)
│   │   ├── sparkline-triplet.tsx # NEW — Battery / RSSI / SNR
│   │   └── quality-badge.tsx    # NEW — button + filter handoff (D-19)
└── hooks/
    ├── useSSE.ts                # NEW — owns EventSource, subscriptions, exponential backoff
    └── useDashboardScope.ts     # NEW — reads install_identity.capabilities from /api/settings or similar
```

### Pattern 1: LISTEN/NOTIFY → in-process SSE fan-out (Go)

**What:** A single dedicated `*pgxpool.Conn` runs `LISTEN measurement_inserted` for the process lifetime. On each notification, the listener decodes the JSON payload and writes it into a Go-channel-backed hub. Per-HTTP-handler subscribers (one per open SSE connection) get their topic-filtered slice of the stream.

**When to use:** Always, in Phase 4. Mirrors `internal/resolver/listener.go` shape.

**Example (skeleton — full Code Examples section below):**

```go
// Source: internal/resolver/listener.go (verified in repo) — Phase 4 events/listener.go mirrors this
func (h *Hub) Run(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) {
    for {
        if ctx.Err() != nil { return }
        err := h.runOnce(ctx, pool, log)
        if ctx.Err() != nil { return }
        log.Warn("events listener disconnected, reconnecting", "err", err)
        select {
        case <-ctx.Done(): return
        case <-time.After(reconnectBackoff): // 2 * time.Second
        }
    }
}

func (h *Hub) runOnce(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) error {
    conn, err := pool.Acquire(ctx)
    if err != nil { return fmt.Errorf("acquire: %w", err) }
    defer conn.Release()
    if _, err := conn.Exec(ctx, "LISTEN measurement_inserted"); err != nil {
        return fmt.Errorf("LISTEN: %w", err)
    }
    for {
        notif, err := conn.Conn().WaitForNotification(ctx)
        if err != nil { return fmt.Errorf("WaitForNotification: %w", err) }
        h.broadcast(notif.Payload) // fans out to all matching subscribers
    }
}
```

**Sources:**
- `[VERIFIED: internal/resolver/listener.go]` — canonical pattern in the repo, exact reconnect/release semantics
- `[CITED: https://brandur.org/notifier]` — "The Notifier Pattern for Applications That Use Postgres"
- `[CITED: https://github.com/jackc/pgx/issues/195]` — confirms a single dedicated connection is required (cannot pool LISTEN)

### Pattern 2: AFTER INSERT trigger on parent hypertable fires per chunk

**What:** TimescaleDB hypertables are technically inheritance-based. When you `CREATE TRIGGER ... AFTER INSERT ON measurement FOR EACH ROW EXECUTE FUNCTION measurement_notify()`, TimescaleDB **automatically propagates the trigger to every chunk** (existing and future). You write the trigger once against the parent table.

**Verified mechanism:** "TimescaleDB automatically creates indexes, constraints, or triggers on chunks that the user has declared on the hypertable." `[CITED: https://github.com/timescale/timescaledb/issues/2304]`

**Critical txn semantics:** Postgres `NOTIFY` is **transactional** — the notification is queued during the transaction and only delivered to listeners after `COMMIT`. This naturally satisfies CONTEXT D-03 / DASH-03's "UI only ever reflects persisted data" because the SSE client cannot receive an event for an uplink the database hasn't committed yet.

**Caveat — CONSTRAINT TRIGGER doesn't work on hypertables in some versions** `[CITED: https://github.com/timescale/timescaledb/issues/1343]`. Phase 4 needs only `AFTER INSERT` (not constraint trigger), so this is unblocking. Confirm against TimescaleDB 2.26 at migration-test time.

**Trigger SQL sketch:**

```sql
-- 0021_measurement_inserted_trigger.up.sql
CREATE OR REPLACE FUNCTION measurement_notify() RETURNS trigger AS $$
DECLARE
    payload TEXT;
BEGIN
    payload := json_build_object(
        'metering_point_id', NEW.metering_point_id,
        'time',              to_char(NEW.time AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),
        'cumulative_value',  NEW.cumulative_value,
        'instant_value',     NEW.instant_value,
        'quality',           NEW.quality,
        'battery_pct',       NEW.battery_pct,
        'rssi',              NEW.rssi
    )::text;
    -- pg_notify channel name + payload (8KB hard cap on payload)
    PERFORM pg_notify('measurement_inserted', payload);
    RETURN NULL; -- AFTER trigger; return value ignored
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER measurement_inserted_notify
    AFTER INSERT ON measurement
    FOR EACH ROW
    EXECUTE FUNCTION measurement_notify();
```

**Test that the trigger fires on a chunk:** Insert a row, then `SELECT pg_notify('test_channel','x')` from a separate connection won't help — instead, in an integration test (`testcontainers-go` Postgres+Timescale), open a second pgx connection issuing `LISTEN measurement_inserted`, INSERT a row from the test goroutine, and `WaitForNotification` with a 2s deadline. If the trigger were broken on chunks the test times out.

**Payload size discipline (8KB hard cap):** the D-02 7-field JSON above is ≤200 bytes for realistic values — well inside the limit. Never stuff `raw_payload` or `decoded_object` into the NOTIFY payload; clients refetch the heavy rows via `/api/metering-points/:id/uplinks`. PostgreSQL silently truncates NOTIFY payloads over 8000 bytes — losing data without erroring.

**Sources:**
- `[CITED: https://github.com/timescale/timescaledb/issues/2304]` — trigger propagation to chunks
- `[CITED: https://www.postgresql.org/docs/current/sql-notify.html]` — 8000-byte payload limit + transactional delivery
- `[VERIFIED: internal/db/migrations/0015_measurement.up.sql]` — measurement schema; reserved slot for this trigger at L78-81

### Pattern 3: SSE client in React 19 with exponential backoff + jitter + React-Query coexistence

**Native `EventSource` limitations:**

1. **Cannot send custom headers** — but Phase 4 doesn't need to. SCS session cookies travel automatically on same-origin requests.
2. **Auto-reconnects with NO backoff control** — naive `EventSource` will hammer a flapping server. CONTEXT D-04 mandates `min(30s, 0.5s * 2^n) + rand(0, 1s)` exponential backoff with jitter.

**Two viable approaches:**

**A. Wrap native `EventSource` (zero new deps, slightly more code):**

```typescript
// Source: extension of MDN EventSource API + CONTEXT D-04 backoff policy
// https://developer.mozilla.org/en-US/docs/Web/API/EventSource
type Status = 'connecting' | 'open' | 'reconnecting' | 'auth-failed' | 'closed'

function useSSE(url: string, topics: string[]) {
  const queryClient = useQueryClient()
  const attemptRef = useRef(0)
  const esRef = useRef<EventSource | null>(null)
  const [status, setStatus] = useState<Status>('connecting')

  useEffect(() => {
    let cancelled = false
    let timer: number | undefined

    const connect = () => {
      if (cancelled) return
      const es = new EventSource(`${url}?topics=${encodeURIComponent(topics.join(','))}`, {
        withCredentials: true, // session cookie
      })
      esRef.current = es
      setStatus('connecting')

      es.addEventListener('open', () => {
        attemptRef.current = 0
        setStatus('open')
      })

      es.addEventListener('snapshot', (ev) => {
        const snap = JSON.parse((ev as MessageEvent).data)
        // Always invalidate snapshot-driven queries on snapshot event
        queryClient.setQueryData(['dashboard', 'snapshot'], snap)
      })

      es.addEventListener('measurement', (ev) => {
        const delta = JSON.parse((ev as MessageEvent).data)
        // Update KPI tile cache directly (low-latency)
        // For heavy timeseries surfaces, invalidateQueries instead
        queryClient.setQueryData(['mp', delta.metering_point_id, 'latest'], delta)
        queryClient.invalidateQueries({ queryKey: ['dashboard', 'kpi'] })
      })

      es.addEventListener('error', () => {
        // EventSource will auto-close on hard errors (401/403/404); state=CLOSED
        es.close()
        esRef.current = null
        if (cancelled) return
        // Detect 401 by hitting a probe endpoint; if 401, surface auth-expired
        // (heuristic — EventSource doesn't expose the HTTP status on close)
        attemptRef.current += 1
        const backoff = Math.min(30_000, 500 * 2 ** attemptRef.current) + Math.random() * 1000
        setStatus('reconnecting')
        timer = window.setTimeout(connect, backoff)
      })
    }

    connect()

    return () => {
      cancelled = true
      if (timer) clearTimeout(timer)
      esRef.current?.close()
    }
  }, [url, topics.join(','), queryClient])

  return { status }
}
```

**B. Use `@microsoft/fetch-event-source` (one new dep, cleaner API):** lets us inspect HTTP status on disconnect (so 401 → auth-failed without a probe ping), set custom Retry policy, send POST or custom headers if ever needed. Worth the ~5KB if planner wants the cleaner code.

**SSE event as invalidation hint pattern:**

- KPI tile values: write directly into React-Query cache from the SSE `measurement` event — low-latency.
- Dashboard cumulative chart: SSE event triggers `invalidateQueries(['dashboard','timeseries',range])` only if `range ∈ {today, 24h}` (matches D-13 live-mode rule). For 7d/30d/Custom, ignore SSE deltas.
- MP detail Advanced tab: do NOT auto-apply the new payload; instead set a `newerPayloadAvailable: true` flag that the UI surfaces as the "Newer payload available · Refresh" alert (UI-SPEC §Advanced tab).
- Uplinks log: when on the Uplinks log tab AND filtering by no quality filter (or quality includes the new row's quality), prepend the new row to the table state. Otherwise ignore.

**On reconnect: always re-issue snapshot** (D-03). The server-side flow is: client opens `/api/events?topics=...`, server emits `event: snapshot\ndata: {...}\n\n`, then streams `event: measurement\ndata: {...}` as they arrive. On reconnect, the same flow repeats — client gets a fresh snapshot, drops any stale React-Query cache for the snapshot keys, and resumes streaming.

**`prefers-reduced-motion`** disables the live-mode pulse marker and the new-row prepend pulse — see UI-SPEC §Iconography note.

### Pattern 4: Capability detection (DASH-01 / D-09)

**Decision (per CONTEXT D-09):** admin-managed flag in `install_identity`. Three options were considered; the locked answer is **(c) persisted on install_identity table, edited via Settings UI, defaults to `'both'`**.

**Read path on dashboard load:** the existing `/api/account/me` already returns the current user; extend the existing settings/install-identity GET endpoint to include `capabilities`. Or expose a tiny `/api/dashboard/scope` endpoint that returns `{ "scope": "water" | "electricity" | "both" }` and is cached by React-Query for the session.

**Write path:** PATCH `/api/settings/install-identity` with `{ capabilities: "water" }` — admin-only via `ActionInstallIdentityEdit` (new action constant). Optimistic UI update + toast.

**Edge case (UI-SPEC §Empty):** `scope = water` AND no water MPs → "No metering points match your dashboard scope." card with CTA "Open settings." This is a 2nd-class empty state — the dashboard isn't empty, the *filtered view* is empty.

### Pattern 5: KPI computation (DASH-02)

Five KPI queries — all `sqlc`-managed:

```sql
-- queries/dashboard.sql — sketch; planner finalizes column names/types

-- Today's consumption (D-05)
-- Parameters: $1 = install_identity.timezone, $2 = utility_class
-- Returns: sum of cumulative_delta across all active MPs of the given utility class
-- since 00:00 install_tz today, excluding MPs with no reading in window.
-- name: TodayConsumptionByUtility :one
SELECT
    SUM(latest.cumulative_value - earliest.cumulative_value) AS today_delta
FROM metering_point mp
INNER JOIN LATERAL (
    SELECT cumulative_value FROM measurement m
    WHERE m.metering_point_id = mp.id
      AND m.time >= date_trunc('day', now() AT TIME ZONE $1) AT TIME ZONE $1
    ORDER BY m.time ASC LIMIT 1
) earliest ON TRUE
INNER JOIN LATERAL (
    SELECT cumulative_value FROM measurement m
    WHERE m.metering_point_id = mp.id
    ORDER BY m.time DESC LIMIT 1
) latest ON TRUE
WHERE mp.utility_class = $2 AND mp.archived_at IS NULL;

-- Current flow / instantaneous draw (D-06)
-- name: CurrentInstantSumByUtility :one
SELECT SUM(latest.instant_value) AS instant_total
FROM metering_point mp
INNER JOIN LATERAL (
    SELECT instant_value FROM measurement m
    WHERE m.metering_point_id = mp.id
    ORDER BY m.time DESC LIMIT 1
) latest ON TRUE
WHERE mp.utility_class = $1 AND mp.archived_at IS NULL;

-- Period delta (D-08) — today [00:00 → now] vs yesterday [00:00 → same-time-yesterday]
-- name: PeriodDeltaByUtility :one
WITH
today AS (
    -- same shape as TodayConsumptionByUtility
    SELECT SUM(latest.cumulative_value - earliest.cumulative_value) AS d
    FROM ... -- as above
),
yesterday AS (
    -- same shape but window = [yesterday 00:00, yesterday 00:00 + (now - today 00:00)]
    SELECT SUM(latest.cumulative_value - earliest.cumulative_value) AS d
    FROM ... -- bounded windows
)
SELECT today.d AS today_d, yesterday.d AS yesterday_d FROM today, yesterday;

-- Online/offline device count (D-07)
-- Requires expected_interval_s column on device_profile (SCHEMA GAP — see §Schema Gaps)
-- name: DeviceOnlineCount :one
SELECT
    COUNT(*) FILTER (
        WHERE d.last_seen_at > now() - (2 * dp.expected_interval_s * INTERVAL '1 second')
    ) AS online_count,
    COUNT(*) AS total_count
FROM device d
INNER JOIN device_profile dp ON dp.id = d.device_profile_id
WHERE d.decommissioned_at IS NULL;
```

**Refresh strategy (per CONTEXT D-06, D-07):**

- "Current flow" tile re-renders **from the SSE event itself** (the NOTIFY payload carries `instant_value`). Client maintains a per-MP map of latest instant_values and sums by utility on every event. Server does NOT recompute the KPI on each NOTIFY — the SSE delivers the raw delta and the client (React) does the projection. This satisfies "live KPIs update without manual refresh" with zero server-side projection cost.
- "Today's consumption" tile recomputes on a 30s ticker OR when the user changes capability scope. Client uses `useQuery({ queryKey: ['dashboard','kpi','today',scope], staleTime: 30_000 })`.
- "Period delta" follows the same 30s ticker — comparison snapshot doesn't move fast.
- "Online/offline" tile: backend computes once per render OR per 30s tick (whichever first). Client re-queries every 30s.

**Live KPI update flow when an uplink fires:**

1. INSERT into `measurement` commits.
2. Trigger emits `pg_notify('measurement_inserted', '{... compact 7 fields ...}')`.
3. Go listener decodes payload, broadcasts to all SSE subscribers whose topic set includes `dashboard:global` OR `mp:<this-uuid>`.
4. SSE handler writes `event: measurement\ndata: {...}\n\n` to each matching connection.
5. Browser `useSSE` hook fires its `measurement` listener:
   - Updates per-MP latest map → `current_flow` tile re-renders.
   - If `quality != 'ok'`, toggles the per-MP `qualityFlagged` flag (Normal-tab badge).
   - `cumulative_value` updates the rightmost bucket of the live chart (only if range = today or 24h).

### Pattern 6: Charts with date-range pickers (DASH-05, DETL-03)

**Backend query shape** — one sqlc query parameterized by `($mp_ids [], $start, $end, $bucket_interval)`:

```sql
-- queries/timeseries.sql
-- name: DashboardTimeseries :many
SELECT
    time_bucket($4::interval, time) AS bucket,
    metering_point_id,
    SUM(cumulative_value - LAG(cumulative_value) OVER (PARTITION BY metering_point_id ORDER BY time)) AS cumulative_delta,
    AVG(instant_value) AS instant_avg
FROM measurement
WHERE time BETWEEN $2 AND $3
  AND metering_point_id = ANY($1::uuid[])
GROUP BY bucket, metering_point_id
ORDER BY bucket;
```

For the dashboard's "cumulative across N MPs stacked" chart, an outer `SUM(cumulative_delta)` per bucket aggregates across MPs. Per-MP detail chart drops the `SUM` outer and renders `metering_point_id` as a single series.

**Bucket schedule (D-12):**
- Today / 24h → `INTERVAL '5 minutes'` (288 / 288 buckets)
- 7d → `INTERVAL '1 hour'` (168 buckets)
- 30d → `INTERVAL '4 hours'` (180 buckets)
- Custom > 30d → `INTERVAL '1 day'` (max ~365 buckets)

**Index coverage:** existing `measurement_mp_time_idx ON measurement (metering_point_id, time DESC)` (verified in 0015) — query planner walks this for the `time BETWEEN` + `metering_point_id = ANY(...)` filter.

**Sparkline queries (D-17):** identical shape, scoped to a single MP, fixed `INTERVAL '1 hour'`, fixed window `now() - INTERVAL '24 hours'`. Three columns picked: `AVG(battery_pct)`, `AVG(rssi)`, `AVG(snr)`.

**Frontend wiring:**

```typescript
// Source: shadcn/ui Calendar + Date Picker docs (https://ui.shadcn.com/docs/components/radix/date-picker)
// + Phase 3 D-15 URL-state pattern
const [params, setParams] = useSearchParams()
const range = z.enum(['today','24h','7d','30d','custom']).catch('today').parse(params.get('range') ?? 'today')
const startISO = range === 'custom' ? params.get('start') : computeStart(range)
const endISO = range === 'custom' ? params.get('end') : computeEnd(range)

const { data, isLoading } = useQuery({
  queryKey: ['dashboard','timeseries',range,startISO,endISO,scope],
  queryFn: () => fetchTimeseries({ range, start: startISO, end: endISO, utility: scope }),
  staleTime: range === 'today' || range === '24h' ? 0 : 5 * 60_000,
})
```

Date-range picker is a segmented control of 4 ghost buttons + 1 popover trigger for Custom. shadcn's `<Calendar mode="range">` recipe is the right primitive. **Verify `react-day-picker` is already present** (shadcn calendar pulls it transitively); if not, run the shadcn CLI add. Search results confirm shadcn's date-picker recipe is built on react-day-picker and date-fns `[CITED: https://ui.shadcn.com/docs/components/radix/date-picker]`.

### Pattern 7: Custom recursive JSON tree (D-20)

**Why custom, not `react-json-view`:** D-20 explicit + shadcn's modern-minimal aesthetic. The component is ~60 lines.

```tsx
// Source: pattern from react-aria treeitem + shadcn primitives
function JsonTree({ value, name, depth = 0, defaultOpen = false }: Props) {
  const [open, setOpen] = useState(defaultOpen)
  if (value === null) {
    return <Row depth={depth}><Key>{name}</Key>: <Null>null</Null></Row>
  }
  if (typeof value === 'string') {
    return <Row depth={depth}><Key>{name}</Key>: <Str>"{value}"</Str></Row>
  }
  if (typeof value === 'number') {
    return <Row depth={depth}><Key>{name}</Key>: <Num>{String(value)}</Num></Row>
  }
  if (typeof value === 'boolean') {
    return <Row depth={depth}><Key>{name}</Key>: <Bool>{String(value)}</Bool></Row>
  }
  const isArray = Array.isArray(value)
  const entries = isArray ? value.map((v,i) => [String(i), v]) : Object.entries(value)
  return (
    <div>
      <button
        aria-expanded={open}
        onClick={() => setOpen(o => !o)}
        className="flex items-center gap-1 text-sm font-mono font-semibold"
      >
        {open ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
        <Key>{name}</Key>
        <span className="text-xs text-muted-foreground">{`(${entries.length} item${entries.length === 1 ? '' : 's'})`}</span>
      </button>
      {open && (
        <div style={{ paddingLeft: 16 * (depth + 1) }}>
          {entries.map(([k,v]) => (
            <JsonTree key={k} name={k} value={v} depth={depth + 1} />
          ))}
        </div>
      )}
    </div>
  )
}
```

Color tokens match UI-SPEC §Advanced-tab JSON: string=`text-success`, number=`text-info`, boolean=`text-warning`, null=`text-muted-foreground italic`, key=`text-foreground font-semibold`.

**Top-level open behavior:** root `object` and `extra` open by default (UI-SPEC). Use `defaultOpen={depth < 1 || name === 'extra'}`.

**Copy JSON button (UI-SPEC):** card-header `<Button variant="ghost" size="sm">` calling `navigator.clipboard.writeText(JSON.stringify(value, null, 2))` + sonner toast "Copied JSON to clipboard."

**Newer-payload-available alert:** when SSE delivers a new uplink for the currently-viewed MP, set a local `pendingPayload` state but DO NOT swap. Render an `<Alert variant="info">` at top of card with "Newer payload available · Refresh". Clicking Refresh swaps + dismisses.

### Pattern 8: Hypertable LIMIT query for Uplinks log (DETL-02)

```sql
-- queries/uplinks.sql
-- name: ListUplinksByMP :many
-- $1 = mp_id, $2 = limit (≤500), $3 = before_time (cursor, ISO), $4 = quality_filter (TEXT[])
SELECT
    time, metering_point_id, cumulative_value, instant_value,
    battery_pct, rssi, snr, fcnt, quality,
    raw_payload, decoded_object
FROM measurement
WHERE metering_point_id = $1
  AND time < COALESCE($3::timestamptz, 'infinity'::timestamptz)
  AND ($4::text[] IS NULL OR quality = ANY($4))
ORDER BY time DESC
LIMIT $2;
```

**Index path:** `measurement_mp_time_idx (metering_point_id, time DESC)` (verified in 0015) — perfect match for `WHERE metering_point_id = $1 ORDER BY time DESC LIMIT N`.

**Pagination:** cursor-based on `time` (each "Load more" sends `before=<oldest_loaded_time>`). NOT offset-based — large offsets on hypertables are slow.

**Quality filter index:** `measurement_quality_flagged_idx ON (metering_point_id, time DESC) WHERE quality <> 'ok'` (verified in 0015) — accelerates the "show only flagged" filter from the D-19 quality-badge handoff.

**Live-prepend logic:** when on Uplinks log tab, SSE `measurement` event for this MP arrives. If current quality filter is empty or includes the new event's quality, prepend to the rendered list and bump a "X new uplinks" floating button (UI-SPEC). Don't auto-scroll.

**Virtualization:** when rendered row count > 200, switch to `@tanstack/react-virtual`. Below 200, plain rendering. **react-virtual is NOT currently in `web/package.json`** — flagged in §Standard Stack Supporting.

### Anti-Patterns to Avoid

- **Computing KPI values server-side on every NOTIFY.** The compact NOTIFY payload carries enough for the client to project the new KPI value. Server stays stateless re: per-client KPI projections.
- **Using offset-based pagination on the hypertable.** Offsets degrade with chunk count. Always use `time < cursor` cursor pagination.
- **Storing live state in SSE messages and treating them as the source of truth.** SSE is a low-latency hint pipe. REST endpoints remain the source of truth (and survive client reconnects, network blips, and tab restores).
- **Subscribing one EventSource per dashboard widget.** D-01 explicitly forbids this — one EventSource per tab, server-side topic filter.
- **Putting `raw_payload` or `decoded_object` in the NOTIFY payload.** 8KB hard cap; PostgreSQL silently truncates over-limit.
- **Setting `withCredentials: false` on `EventSource`.** SCS session cookie must travel.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| LISTEN/NOTIFY connection management | Wrapping `database/sql` with custom poll | `pgx/v5` + dedicated `pgxpool.Conn` + `WaitForNotification` (already proven by resolver) | pgx's `WaitForNotification` is async-cancellable; the resolver pattern works |
| In-process pub/sub for SSE fan-out | Goroutine + sync.Map + channels with hand-tuned dropping | Same as above but with `chan []byte` per subscriber + non-blocking send with `default:` clause that drops on slow client | Slow-client backpressure is well-trodden; just match `internal/resolver/listener.go` style. CONTEXT marks the exact policy (drop oldest delta + force re-snapshot vs disconnect) as Claude's Discretion |
| SSE handler (write + flush + heartbeat) | Roll your own response-writer | stdlib `http.Flusher` (every chi handler's `http.ResponseWriter` satisfies this on HTTP/1.1 + HTTP/2) + a `time.Ticker` for the 30s heartbeat + `r.Context().Done()` for per-client cancel | Pattern is 30 lines; no library needed |
| Date math for period delta and bucket widths | Manual `time.Time` arithmetic on the frontend | `date-fns` (already in bundle) — `startOfDay`, `subDays`, `differenceInMinutes` | Already a dep; well-tested edge cases (DST, leap days) |
| Date range picker UI | Custom calendar component | shadcn `<Calendar mode="range">` recipe + `react-day-picker` (transitive peer dep of shadcn calendar) | Pure UI, no algorithmic work |
| TanStack Table virtualization on large lists | Render-all-then-pray | `@tanstack/react-virtual` integration | shadcn + tanstack-table docs show the exact recipe |
| JSON pretty-print | Manual string-build | `JSON.stringify(value, null, 2)` (Copy JSON button) | One line of stdlib |
| Cumulative chart rendering | DIY SVG | Recharts `<AreaChart>` wrapped by `chart.tsx` | Already in the codebase |
| Sparkline rendering | DIY SVG | Recharts `<LineChart>` / `<AreaChart>` with `hide` axes — pattern already used by Phase 3 gateway sparkline | Inherit, don't reinvent |
| Exponential backoff + jitter | Reimplement | Inline math `min(30000, 500 * 2 ** n) + Math.random() * 1000` per CONTEXT D-04 | Six lines; no need for a library |
| Auth cookie propagation to SSE | Custom token plumbing | `EventSource(url, { withCredentials: true })` carries SCS session cookies same-origin | Native; tested cross-browser |

**Key insight:** Phase 4 is **wiring, not building**. Every primitive already exists in the codebase or in shadcn/tanstack/recharts. The plan should be densely small commits gluing primitives together — not large new abstractions.

## Common Pitfalls

### Pitfall 1: SSE behind buffering reverse proxy

**What goes wrong:** Default nginx buffers SSE responses; the client never sees events until the connection closes. Manifests as "live updates work locally, dead in prod."
**Why it happens:** Reverse proxies optimize for typical HTTP; SSE is the unusual case.
**How to avoid:** Caddy `flush_interval -1` is **already configured in Phase 1 D-20..D-22** for `/sse /sse/*` paths. Phase 4 must mount the SSE route at `/api/events` — confirm the Caddyfile matcher covers `/api/events` OR amend it to add the path. **Verify at plan time.** If shifting to a non-Caddy setup later, nginx requires `proxy_buffering off`.
**Warning signs:** SSE events delivered in batches every 30+ seconds (Caddy buffering kicks in) instead of within milliseconds.

### Pitfall 2: LISTEN connection borrowed from pgxpool

**What goes wrong:** Acquiring a pool connection and issuing LISTEN, then letting the connection return to the pool — another goroutine borrows it, and LISTEN state is lost (or interleaves with foreign queries).
**Why it happens:** Pooled connections recycle; LISTEN state is per-connection in Postgres.
**How to avoid:** Use `pool.Acquire(ctx)` and **never release** while the listener runs. The `defer conn.Release()` should only fire on listener exit (process shutdown OR ctx cancel OR reconnect). `internal/resolver/listener.go` already does this correctly; the new `internal/events/listener.go` follows the exact same shape.
**Warning signs:** Random "no notifications received" gaps with no error; events delivered to the wrong process.

### Pitfall 3: AFTER UPDATE / BEFORE INSERT triggers on hypertables

**What goes wrong:** Some TimescaleDB versions don't propagate constraint triggers or BEFORE INSERT triggers to chunks; the trigger fires only on the parent (which never holds rows).
**Why it happens:** Hypertable internals route inserts directly into the chunk; only certain trigger types propagate cleanly.
**How to avoid:** Use **`AFTER INSERT FOR EACH ROW`** only. Confirmed by `[CITED: https://github.com/timescale/timescaledb/issues/2304]` that triggers declared on parent propagate to chunks automatically. AFTER INSERT is the canonical safe choice. Avoid CONSTRAINT TRIGGER on hypertables `[CITED: https://github.com/timescale/timescaledb/issues/1343]`.
**Warning signs:** Trigger compiles; insert tests succeed; but `LISTEN` never receives a notification on real-uplink ingest. Integration test catching this: insert a row from a goroutine while a second pgx connection holds LISTEN, fail if no notification within 2s.

### Pitfall 4: NOTIFY payload >8000 bytes silently truncated

**What goes wrong:** Adding `raw_payload` (BYTEA, often 16–64 bytes hex-encoded ~32–128 chars) or `decoded_object` (JSONB, often 200B–2KB) to the NOTIFY payload — at the upper end you cross 8KB and Postgres truncates.
**Why it happens:** `pg_notify(channel, payload)` documented limit is 8000 bytes for the payload string.
**How to avoid:** D-02 7-field compact JSON is ≤200 bytes — well safe. NEVER add `raw_payload` or `decoded_object` to the NOTIFY. Clients fetch heavy data via the REST endpoint.
**Warning signs:** JSON parse errors in the Go listener; intermittent missing rows on the client.

### Pitfall 5: `EventSource` auto-reconnect without backoff

**What goes wrong:** Native `EventSource` reconnects on every error with the same fixed delay (default 3s). On a flaky network or a temporarily-overloaded server, the client hammers reconnects, amplifying the problem.
**Why it happens:** Standards-compliant browser behavior; no API to set per-attempt backoff.
**How to avoid:** Either (a) wrap `EventSource` and manually `.close()` on error + `setTimeout` for the exponential-backoff retry (Pattern 3A), OR (b) use `@microsoft/fetch-event-source` which exposes a `RetryAfter` / custom retry policy (Pattern 3B). D-04 mandates `min(30s, 0.5s * 2^n) + rand(0, 1s)`.
**Warning signs:** Server log shows 100+ reconnect attempts in a single minute from one IP.

### Pitfall 6: Mobile Safari kills idle background tabs

**What goes wrong:** User backgrounds the dashboard tab on iPhone; iOS pauses the SSE connection; coming back, `EventSource` shows `OPEN` state but no events arriving — or, after >30s suspended, the connection is dead.
**Why it happens:** iOS Safari aggressively suspends background tabs and timers; long-lived connections are casualties.
**How to avoid:** The SSE-disconnected banner already triggers when no heartbeat arrives in >30s. The reconnect-with-snapshot flow (D-03) is exactly the recovery: client reconnects, server emits `event: snapshot`, client invalidates React-Query keys and re-renders. **Crucial:** the snapshot must be a fresh state read, NOT a replay of buffered events.
**Warning signs:** Mobile users report "values frozen until I tap somewhere"; check the heartbeat detection logic.

### Pitfall 7: `EventSource` cannot send custom headers (including `X-Requested-With`)

**What goes wrong:** Phase 1 D-09 + `apiFetch` set the `X-Requested-With: shifter` CSRF guard on all `/api/*` POSTs. SSE GET cannot send custom headers via the native `EventSource` API.
**Why it happens:** Native `EventSource` API restriction.
**How to avoid:** SSE is GET-only and read-only — CSRF threat model doesn't apply (no state change). The Phase 1 CSRF guard pattern is for POST/PUT/DELETE; we **don't need** `X-Requested-With` on SSE. SameSite=Lax cookie + same-origin is enough.
**Warning signs:** Auth middleware tries to enforce X-Requested-With on the SSE route and 403s the connection; planner must verify the SSE route is exempted from the CSRF middleware.

### Pitfall 8: Recharts re-renders on every SSE event

**What goes wrong:** Every SSE delta triggers a React state update, which triggers a full Recharts redraw — visible as flicker, CPU spike, dropped frames.
**Why it happens:** Recharts isn't optimized for high-frequency updates; redraws are SVG-heavy.
**How to avoid:** (a) Throttle chart updates to ~1Hz (use `_.throttle` on the state setter that feeds the chart series); (b) For sparklines and live-mode chart marker, write to a ref instead of state and force re-render only on a `requestAnimationFrame` tick; (c) Disable Recharts animation on data prop change (already-empty animation in `chart.tsx` config? — verify).
**Warning signs:** Chrome DevTools Performance tab shows >30ms paint on every SSE event.

### Pitfall 9: SSE-disconnected banner false positives on slow snapshot

**What goes wrong:** Initial connect; server takes 800ms to assemble the snapshot; client classifies "no event in 30s" as "disconnected" too eagerly.
**Why it happens:** Heartbeat detection isn't smart about the initial-connect grace period.
**How to avoid:** Banner shows "Reconnecting…" only after EITHER `EventSource.readyState !== OPEN` OR no heartbeat in 30s since last `OPEN` state. Initial connect: don't show the banner during the first ~3s.
**Warning signs:** Banner flickers on/off on every page navigation.

### Pitfall 10: `expected_interval_s` defaults too aggressive

**What goes wrong:** D-07 online threshold = `2 × expected_interval_s`. If we default to `3600` (1 hour) and a customer has 60-second-interval meters, the dashboard shows "12 of 12 online" for up to 2 hours of actual silence — false healthy reading.
**Why it happens:** Wrong default; missing per-profile reality check.
**How to avoid:** Seed the existing profiles with realistic values: Axioma W1 ~3600s, Acrel ADW300 ~60–300s (confirm at plan time from vendor datasheets). The schema-gap migration must backfill all existing seeded profiles.
**Warning signs:** "All devices online" reported while a customer says all devices are dead.

## Runtime State Inventory

> N/A — this is a greenfield feature phase (new code + new migrations + new frontend routes). No rename, refactor, or string migration is happening. Phase 4 reads existing telemetry produced by Phase 2 and renders it; existing data is untouched.

## Code Examples

### SSE handler (Go, chi)

```go
// Source: stdlib net/http + chi v5 + project pattern (verified against internal/http/router.go + internal/resolver/listener.go)
// File: internal/events/handler.go (new in Phase 4)

func (deps Deps) EventsHandler() http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        flusher, ok := w.(http.Flusher)
        if !ok {
            http.Error(w, "streaming unsupported", http.StatusInternalServerError)
            return
        }

        user, err := auth.UserFromContext(r.Context())
        if err != nil {
            http.Error(w, "unauthorized", http.StatusUnauthorized)
            return
        }

        // Parse topic subscription from ?topics= (CSV) or POST body — D-01
        topics := parseTopics(r.URL.Query().Get("topics"))

        // SSE headers — every one is load-bearing
        w.Header().Set("Content-Type", "text/event-stream")
        w.Header().Set("Cache-Control", "no-cache")
        w.Header().Set("Connection", "keep-alive")
        w.Header().Set("X-Accel-Buffering", "no") // belt+suspenders for nginx if ever swapped in
        w.WriteHeader(http.StatusOK)

        // Subscribe to hub; sub.Ch is a buffered chan []byte
        sub := deps.Hub.Subscribe(user.ID, topics)
        defer deps.Hub.Unsubscribe(sub)

        // Initial snapshot (D-03)
        snapshot := deps.Snapshot(r.Context(), topics)
        snapBytes, _ := json.Marshal(snapshot)
        fmt.Fprintf(w, "event: snapshot\ndata: %s\n\n", snapBytes)
        flusher.Flush()

        // Heartbeat every 30s — D-04
        heartbeat := time.NewTicker(30 * time.Second)
        defer heartbeat.Stop()

        for {
            select {
            case <-r.Context().Done():
                return
            case <-heartbeat.C:
                fmt.Fprint(w, ": heartbeat\n\n")
                flusher.Flush()
            case msg, ok := <-sub.Ch:
                if !ok { return }
                fmt.Fprintf(w, "event: measurement\ndata: %s\n\n", msg)
                flusher.Flush()
            }
        }
    }
}
```

### Hub broadcast with non-blocking send (Go)

```go
// File: internal/events/hub.go (new in Phase 4)

func (h *Hub) broadcast(payload string) {
    parsed, err := parsePayload(payload) // {metering_point_id, time, ...}
    if err != nil {
        h.log.Warn("invalid notify payload", "err", err)
        return
    }

    h.mu.RLock()
    defer h.mu.RUnlock()

    for _, sub := range h.subs {
        if !sub.matches(parsed) { continue }
        select {
        case sub.Ch <- payload: // happy path
        default:
            // Slow client — drop this delta + flag for re-snapshot
            // Backpressure policy is Claude's Discretion per CONTEXT
            sub.flagResnapshot()
            h.log.Debug("dropped delta for slow client", "user_id", sub.UserID)
        }
    }
}
```

### useSSE hook (React)

```typescript
// File: web/src/hooks/useSSE.ts (new in Phase 4)
// Source: MDN EventSource + CONTEXT D-04 backoff + Phase 3 useSearchParams pattern

export type SSEStatus = 'connecting' | 'open' | 'reconnecting' | 'paused' | 'auth-failed'

export function useSSE(topics: string[]) {
  const queryClient = useQueryClient()
  const [status, setStatus] = useState<SSEStatus>('connecting')
  const attemptRef = useRef(0)
  const lastHeartbeatRef = useRef(Date.now())
  const esRef = useRef<EventSource | null>(null)

  useEffect(() => {
    let cancelled = false
    let backoffTimer: ReturnType<typeof setTimeout> | undefined

    const connect = () => {
      if (cancelled) return
      const url = `/api/events?topics=${encodeURIComponent(topics.join(','))}`
      const es = new EventSource(url, { withCredentials: true })
      esRef.current = es
      lastHeartbeatRef.current = Date.now()

      es.addEventListener('open', () => {
        attemptRef.current = 0
        setStatus('open')
      })

      es.addEventListener('snapshot', (ev) => {
        lastHeartbeatRef.current = Date.now()
        const snap = JSON.parse((ev as MessageEvent).data)
        queryClient.setQueryData(['dashboard','snapshot'], snap)
        // Invalidate snapshot-dependent KPI queries
        queryClient.invalidateQueries({ queryKey: ['dashboard','kpi'] })
      })

      es.addEventListener('measurement', (ev) => {
        lastHeartbeatRef.current = Date.now()
        const delta = JSON.parse((ev as MessageEvent).data)
        queryClient.setQueryData(['mp', delta.metering_point_id, 'latest'], delta)
        // Live KPI projection — sum across MPs is computed in a selector
      })

      // Heartbeat is a comment line; browsers don't fire an event for it,
      // but the connection staying alive is the signal. We watch a tick.
      // (Alternative: parse 'message' for empty-data and treat as heartbeat.)

      es.addEventListener('error', () => {
        es.close()
        esRef.current = null
        if (cancelled) return
        attemptRef.current += 1
        const backoff = Math.min(30_000, 500 * 2 ** attemptRef.current) + Math.random() * 1000
        setStatus(attemptRef.current > 5 ? 'paused' : 'reconnecting')
        backoffTimer = setTimeout(connect, backoff)
      })
    }

    connect()

    // Heartbeat watchdog — D-04
    const watchdog = setInterval(() => {
      if (Date.now() - lastHeartbeatRef.current > 45_000 && esRef.current?.readyState === EventSource.OPEN) {
        esRef.current?.close() // forces reconnect via 'error' handler
      }
    }, 5_000)

    return () => {
      cancelled = true
      clearInterval(watchdog)
      if (backoffTimer) clearTimeout(backoffTimer)
      esRef.current?.close()
    }
  }, [topics.join(','), queryClient])

  return status
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| MQTT-over-WebSocket directly to browser | SSE driven by Postgres LISTEN/NOTIFY | Phase 4 design lockup; reaffirmed by SUMMARY.md §Phase 4 + Architecture Pattern 5 + this research | Browser never sees the broker; per-user filtering happens server-side; only persisted data shows up |
| Polling every N seconds | SSE invalidation hints + React Query | Phase 1 stack lockup | Sub-second latency for tile updates; no client-side timer-driven HTTP load |
| Long-polling | SSE | Same | Same |
| `EventSource` with default 3s reconnect | Manual wrapping with `min(30s, 0.5s * 2^n) + rand(0,1s)` | D-04 + this research | Avoids reconnect storms on flaky networks (PITFALL §9) |
| `pq` driver with no LISTEN support | `pgx/v5` `WaitForNotification` | Project start | Async cancellation works; pq is in maintenance mode |
| `react-leaflet` v4 + React 18 | React 19 + `react-leaflet` v5 (Phase 5) | Phase 1 stack lockup | Not used in Phase 4 |
| `recharts` v2 | `recharts` v3.8.0 (in repo) | Phase 1 trunk | React 19 peer dep met |

**Deprecated/outdated:**

- `lib/pq` — replaced by `pgx/v5` everywhere in the codebase.
- `gofpdf` — archived; not used in Phase 4.
- Naive long-polling — replaced by SSE.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Axioma W1 expected uplink interval ≈ 3600s; Acrel ADW300 ≈ 60–300s | §Schema Gaps + §Pitfalls 10 | Wrong defaults → false-healthy or false-offline KPI; mitigation = confirm against vendor datasheets at plan time before backfilling seed migrations |
| A2 | Caddy SSE matcher in current Caddyfile covers `/api/events` OR is path-agnostic for the API group | §Pitfalls 1 | If only `/sse` is matched, the SSE handler will be buffered behind Caddy and live updates silently break; verify the Caddyfile at plan time and amend if necessary |
| A3 | `react-day-picker` is already a transitive peer dep via shadcn calendar in the current bundle | §Standard Stack Supporting | Need to run `pnpm dlx shadcn@latest add calendar` if missing |
| A4 | Existing `chart.tsx` Recharts wrapper exports `ChartContainer`, `ChartTooltip`, `ChartLegend` and doesn't need re-export for the dashboard area/line charts | §Code Examples §Recharts | Verified: 11 matches for these names in `web/src/components/ui/chart.tsx` (low risk) |
| A5 | The `apiFetch` `X-Requested-With` middleware is enforced only on state-changing HTTP methods (POST/PUT/PATCH/DELETE), not GET | §Pitfalls 7 | If GET is also gated, the SSE route will be blocked; planner verifies the middleware code at plan time and adjusts |
| A6 | TimescaleDB 2.26 propagates `AFTER INSERT FOR EACH ROW` triggers from parent hypertable to chunks (search-result-based; not directly verified against 2.26 release notes) | §Architecture Pattern 2 + §Pitfalls 3 | If the version has a regression, the trigger fires on the parent (which holds no rows) and NOTIFY never fires; integration test with `testcontainers-go` catches this on day one |
| A7 | The existing `apiFetch` and SCS session cookie flow lets `EventSource` GET `/api/events` carry the session — verified by spec (`withCredentials: true` + same-origin), not yet tested in repo | §Code Examples §useSSE | If cookie isn't forwarded, the SSE handler will see no `auth.UserFromContext` and 401; Playwright E2E catches this |
| A8 | Mobile Safari behavior on suspended SSE matches typical browser behavior (connection drops within 30–60s of backgrounding) | §Pitfalls 6 | If Safari holds the connection longer than expected, the 45s watchdog timer correctly forces a reconnect-and-snapshot anyway |
| A9 | `time_bucket` performance on raw hypertable at 30d range with 1-hour buckets is acceptable for Phase 4 single-tenant scale (≤ a few hundred MPs) | §Architecture Pattern 6 | If queries slow >2s, Phase 5 CAGGs are the planned mitigation; planner should benchmark with Phase 3 synthetic fleet (`shifter test-harness`) before sign-off |

**If A1 / A2 / A6 turn out wrong**, the plan changes shape (different seed values, different Caddy block, different trigger strategy). All three need to be **confirmed at plan time**, not at execute time. The remaining assumptions are low-stakes — either trivially confirmable or have natural failure-detection in the test suite.

## Open Questions

1. **Is `device.last_seen_at` advanced inside the same Serializable txn as the `measurement` INSERT?**
   - What we know: `internal/ingest/persist.go` package doc says yes ("`device.last_seen_at` advances inside the same Serializable tx"). CONTEXT canonical refs confirm.
   - What's unclear: Whether `last_seen_at` is updated before or after the INSERT — affects whether the SSE NOTIFY observes the new `last_seen_at` value in subsequent online-count queries.
   - Recommendation: planner reads `internal/ingest/persist.go` at plan time and pins the exact ordering; if `last_seen_at` updates after the measurement INSERT but inside the same txn, both commit atomically and online-count queries are correct. Likely already fine.

2. **Where should the SSE backpressure policy live?**
   - Claude's Discretion per CONTEXT. Options: (a) drop oldest delta + flag re-snapshot on next tick; (b) close the connection and let client reconnect for fresh snapshot.
   - Recommendation: (a) is friendlier UX (no banner flicker). Planner picks at plan time. Document the choice in the SSE-handler doc comment.

3. **Does `apiFetch` middleware enforce `X-Requested-With` on GET requests?** Assumption A5.
   - Recommendation: planner greps `internal/http/router.go` and the auth middleware for the exact methods enforced; the answer is one line in the code.

4. **Is `react-virtual` already a transitive dep via tanstack-table?**
   - Verified: it is NOT in `web/package.json` (grep returned 0 matches).
   - Recommendation: planner adds `@tanstack/react-virtual` to the plan's Wave 0 install list.

5. **Should the dashboard scope flag live on `install_identity` or a new `dashboard_settings` table?**
   - CONTEXT D-09 says "or equivalent — planner picks shape." Adding a column to `install_identity` is minimal and matches the singleton pattern. A new table is over-engineering at single-tenant scale.
   - Recommendation: add `capabilities` column to `install_identity`.

6. **Per-binding `last_uplink_at` view (D-07 alternative)?**
   - CONTEXT marks this as Claude's Discretion: "Whether `device.last_seen_at` is good enough for D-07 or whether a per-binding `last_uplink_at` view is needed (binding swaps change which device feeds an MP — planner reads the resolver code)".
   - Investigation: `device.last_seen_at` measures the *device*, not the *binding*. After a swap, the new device's `last_seen_at` is empty until first uplink — but the MP has continuous history via the old device. Phase 4 KPI tile reports "online devices," which is device-centric, so `device.last_seen_at` is the correct field for D-07. A future "online metering points" KPI would need binding-level tracking — defer to Phase 6.
   - Recommendation: stick with `device.last_seen_at`; document the device-vs-MP distinction in the KPI doc comment.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | Backend build | yes | 1.26.0 (exceeds 1.24+ floor) | — |
| Node.js | Frontend build | yes | v22.20.0 (exceeds 22.12+ floor) | — |
| pnpm | Frontend package mgr | yes | 10.33.2 | — |
| Docker | Test containers + bundled compose smoke | yes | 29.4.1 | — |
| `just` | Recipes | yes | 1.50.0 | — |
| PostgreSQL 16 + TimescaleDB 2.26 (test container image) | Migration tests + LISTEN integration test | yes (via testcontainers — pinned to `timescale/timescaledb:2.26.0-pg16` in Phase 1 D-02) | 2.26 | — |
| Mosquitto (test container) | not needed in Phase 4 (Phase 4 doesn't touch MQTT directly) | yes (already pulled by Phase 2 ingest tests) | 2.0.x | — |
| Playwright | E2E tests | yes (Phase 3 set up `web/playwright/specs/`) | latest | — |

**Missing dependencies with no fallback:** none.

**Missing dependencies with fallback:** none — all required tooling present.

**npm-side missing modules** (need install): `@tanstack/react-virtual` (Uplinks log virtualization at >200 rows, D-16). Optional: `@microsoft/fetch-event-source` if planner picks fetch-based SSE.

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework (Go) | `testing` stdlib + `testify/require` (v1.11.1) + `testcontainers-go` (v0.42.0) for Postgres + Mosquitto |
| Framework (Frontend unit) | Vitest + Testing Library |
| Framework (Frontend E2E) | Playwright (set up Phase 3) |
| Config files | `go.mod` test deps + `web/vitest.config.ts` + `web/playwright.config.ts` |
| Quick run command (Go) | `go test ./internal/events/... ./internal/dashboard/... ./internal/meteringpoint/... -short -count=1` |
| Quick run command (frontend) | `pnpm --dir web test --run` |
| Full suite command (Go) | `go test ./... -race -count=1` |
| Full suite command (frontend) | `pnpm --dir web test --run && pnpm --dir web exec playwright test` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| DASH-01 | Dashboard adapts to install scope (water / electricity / both) | unit (web) | `pnpm --dir web test --run web/src/routes/index.test.tsx` | ❌ Wave 0 |
| DASH-01 | `install_identity.capabilities` column constraint enforced | integration (Go) | `go test ./internal/db -run TestInstallIdentity_CapabilitiesCheck -count=1` | ❌ Wave 0 |
| DASH-02 | KPI SQL returns correct sums per utility class against synthetic fleet | integration (Go) | `go test ./internal/dashboard -run TestKPI_TodayConsumption -count=1` | ❌ Wave 0 |
| DASH-02 | Online-count uses `2 × expected_interval_s` threshold | integration (Go) | `go test ./internal/dashboard -run TestKPI_OnlineCount -count=1` | ❌ Wave 0 |
| DASH-03 | `measurement_inserted` trigger fires on hypertable chunks | integration (Go) | `go test ./internal/events -run TestTrigger_FiresOnChunkInsert -count=1` | ❌ Wave 0 |
| DASH-03 | NOTIFY payload schema matches the D-02 7-field shape | integration (Go) | `go test ./internal/events -run TestTrigger_PayloadShape -count=1` | ❌ Wave 0 |
| DASH-03 | LISTEN connection reconnects on simulated drop with 2s backoff | unit (Go) | `go test ./internal/events -run TestHub_Reconnect -count=1` | ❌ Wave 0 |
| DASH-03 | SSE handler flushes after each event + emits 30s heartbeat | unit (Go) | `go test ./internal/events -run TestEventsHandler_HeartbeatCadence -count=1` | ❌ Wave 0 |
| DASH-03 | SSE handler honors `r.Context().Done()` on client disconnect | unit (Go) | `go test ./internal/events -run TestEventsHandler_ContextCancellation -count=1` | ❌ Wave 0 |
| DASH-03 | End-to-end: INSERT → trigger → listener → hub broadcast → SSE write | integration (Go) | `go test ./internal/events -run TestE2E_InsertToSubscriberDelivery -count=1` | ❌ Wave 0 |
| DASH-04 | Reconnect backoff math: `min(30s, 0.5s * 2^n) + rand(0, 1s)` | unit (web) | `pnpm --dir web test --run hooks/useSSE.test.ts` | ❌ Wave 0 |
| DASH-04 | Reconnect delivers fresh snapshot, NOT cached deltas | E2E (Playwright) | `pnpm --dir web exec playwright test specs/dashboard-reconnect.spec.ts` | ❌ Wave 0 |
| DASH-05 | Date-range presets resolve to correct `time_bucket` widths | unit (Go) | `go test ./internal/dashboard -run TestTimeseries_BucketWidthsPerPreset -count=1` | ❌ Wave 0 |
| DASH-05 | Custom range bound: 1 hour ≤ range ≤ 1 year | unit (web) | `pnpm --dir web test --run components/dashboard/date-range-picker.test.tsx` | ❌ Wave 0 |
| DASH-05 | URL state survives back/forward navigation | E2E (Playwright) | `pnpm --dir web exec playwright test specs/dashboard-url-state.spec.ts` | ❌ Wave 0 |
| DASH-06 | Mobile viewport (375×667) — KPI grid stacks, sidebar collapses | E2E (Playwright) | `pnpm --dir web exec playwright test specs/dashboard-mobile.spec.ts --device 'iPhone SE'` | ❌ Wave 0 |
| DETL-01 | Advanced tab walks every JSONB key (no flattening loss on nested vendor JSON) | unit (web) | `pnpm --dir web test --run components/metering-point/json-tree.test.tsx` | ❌ Wave 0 |
| DETL-01 | Normal-tab quality badge surfaces only when count(quality != 'ok' in last 100) > 0 | unit (web) | `pnpm --dir web test --run components/metering-point/quality-badge.test.tsx` | ❌ Wave 0 |
| DETL-02 | Uplinks log paginates by `time < cursor` (cursor-based, not offset) | integration (Go) | `go test ./internal/meteringpoint -run TestUplinks_CursorPagination -count=1` | ❌ Wave 0 |
| DETL-02 | Uplinks log cap is 500 rows total (Load more disabled at cap) | unit (web) | `pnpm --dir web test --run components/metering-point/uplinks-log-tab.test.tsx` | ❌ Wave 0 |
| DETL-02 | Quality filter URL state round-trips and pre-applies from D-19 handoff | E2E (Playwright) | `pnpm --dir web exec playwright test specs/mp-detail-quality-handoff.spec.ts` | ❌ Wave 0 |
| DETL-03 | Sparkline triplet renders Battery / RSSI / SNR last 24h via `time_bucket('1 hour')` | unit (web) | `pnpm --dir web test --run components/metering-point/sparkline-triplet.test.tsx` | ❌ Wave 0 |

### Sampling Rate

- **Per task commit (TDD cycle):** the relevant unit suite for the touched package
  - Go example: `go test ./internal/events/... -short -count=1`
  - Web example: `pnpm --dir web test --run components/dashboard/`
- **Per wave merge:** full unit suite both sides
  - `go test ./... -short -count=1` + `pnpm --dir web test --run`
- **Phase gate (before `/gsd-verify-work`):** full suite green
  - `go test ./... -race -count=1` + `pnpm --dir web test --run` + `pnpm --dir web exec playwright test`

### Wave 0 Gaps

All test files listed above are net-new (Phase 4 introduces the surfaces). Wave 0 plan must create skeletons (with `t.Skip` / `describe.skip`) for:

- [ ] `internal/events/listener_test.go` — TestHub_Reconnect, TestE2E_InsertToSubscriberDelivery
- [ ] `internal/events/hub_test.go` — TestHub_BroadcastTopicFilter, TestHub_BackpressureDropOnSlowClient
- [ ] `internal/events/handler_test.go` — TestEventsHandler_HeartbeatCadence, TestEventsHandler_ContextCancellation, TestEventsHandler_Auth401, TestEventsHandler_Subscribe
- [ ] `internal/events/trigger_test.go` — TestTrigger_FiresOnChunkInsert, TestTrigger_PayloadShape (these run against the testcontainers Postgres+TimescaleDB image)
- [ ] `internal/dashboard/snapshot_handler_test.go` — TestSnapshotHandler_ScopeFilter
- [ ] `internal/dashboard/kpi_test.go` — TestKPI_TodayConsumption, TestKPI_CurrentInstant, TestKPI_PeriodDelta, TestKPI_OnlineCount
- [ ] `internal/dashboard/timeseries_handler_test.go` — TestTimeseries_BucketWidthsPerPreset, TestTimeseries_URLBoundChecking
- [ ] `internal/meteringpoint/detail_handler_test.go` — TestMPDetail_AuthorizedReads
- [ ] `internal/meteringpoint/timeseries_handler_test.go` — TestMPTimeseries_PerMPScope
- [ ] `internal/meteringpoint/uplinks_handler_test.go` — TestUplinks_CursorPagination, TestUplinks_Cap500, TestUplinks_QualityFilter
- [ ] `internal/db/migrations/0021_..._test.go` — TestMigration0021_TriggerPropagatesToChunks (or place in existing `internal/db/migrations_test.go`)
- [ ] `internal/db/migrations/0022_..._test.go` — TestMigration0022_CapabilitiesCheck
- [ ] `internal/db/migrations/0023_..._test.go` — TestMigration0023_ExpectedIntervalDefault
- [ ] `web/src/hooks/useSSE.test.ts` — backoff math + reconnect snapshot semantics
- [ ] `web/src/routes/index.test.tsx` — DashboardPage adaptive scope + onboarding empty states
- [ ] `web/src/components/dashboard/date-range-picker.test.tsx` — preset state + URL params + custom-range bounds
- [ ] `web/src/components/dashboard/kpi-tile.test.tsx` — period-delta semantics + comparison-unavailable case
- [ ] `web/src/components/dashboard/live-channel-banner.test.tsx` — reconnecting / paused / auth-expired tiered states
- [ ] `web/src/components/dashboard/empty-state-onboarding.test.tsx` — 3 sub-states keyed on tuple
- [ ] `web/src/components/metering-point/json-tree.test.tsx` — recursive nested vendor JSON; type-color encoding
- [ ] `web/src/components/metering-point/quality-badge.test.tsx` — hidden when count=0, handoff URL params
- [ ] `web/src/components/metering-point/uplinks-log-tab.test.tsx` — Load more 100/500 cap, live prepend, virtualizer threshold
- [ ] `web/src/components/metering-point/sparkline-triplet.test.tsx` — 3 sparklines + health-band fill color
- [ ] `web/src/routes/metering-points/$id.test.tsx` — tabbed layout + empty state (no binding) keeps tabs visible
- [ ] `web/playwright/specs/dashboard-reconnect.spec.ts` — fake server drop → reconnect → snapshot replay
- [ ] `web/playwright/specs/dashboard-url-state.spec.ts` — back/forward + share URL
- [ ] `web/playwright/specs/dashboard-mobile.spec.ts` — iPhone SE viewport
- [ ] `web/playwright/specs/mp-detail-quality-handoff.spec.ts` — click badge → tab switch + filter applied
- [ ] `web/playwright/specs/mp-detail-advanced-newer-payload.spec.ts` — SSE arrives while on Advanced tab → "Newer payload available" alert
- [ ] `web/playwright/specs/sse-snapshot-on-reconnect.spec.ts` — kill SSE → wait → verify snapshot, not delta replay

**Framework installs needed:** none (`@tanstack/react-virtual` is the only new prod-side library; no new test frameworks).

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | yes | SCS session cookies (already in place from Phase 1); `EventSource` `withCredentials: true` carries the cookie |
| V3 Session Management | yes | `LoadAndSave` middleware honors session timeout on the SSE GET; expired session → handler returns 401 → client surfaces auth-expired banner |
| V4 Access Control | yes | `auth.RequireAction` middleware on every new Phase 4 route. Phase 4 introduces NO new actions (D-23 = full read for both roles); empty-state CTAs hide for viewers via existing `Can()` checks |
| V5 Input Validation | yes | zod on URL params (date range presets, quality filter); UUID parsing for `mp:<uuid>` topic ids; reject invalid topics server-side |
| V6 Cryptography | no | Phase 4 introduces no new crypto. SCS cookies + TLS (Caddy) handle transport |
| V7 Error Handling | yes | Don't leak DB errors in REST responses; SSE error event payload limited to error code, not stack |
| V13 API & Web Service | yes | SSE handler sets correct CORS-related headers; SameSite=Lax + same-origin enforced |

### Known Threat Patterns for SSE + Postgres LISTEN/NOTIFY

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Slow-client connection exhaustion (one client holds many SSE streams open, depleting backend connection pool) | Denial of Service | Per-session connection cap (e.g., 5 SSE per user_id) inside the hub; CONTEXT marks this as Claude's Discretion. Default to 5; bump if customers complain |
| Topic-enumeration attack (subscribe to `mp:<every-uuid>`) | Information Disclosure | Server-side authz check on subscribe — single-tenant, so admin and viewer both see all MPs (per D-23); no per-MP gating. But validate UUIDs; reject invalid topic strings; cap topic-set size per session (e.g., 1000) |
| SSE stream forced to consume memory by sending huge initial snapshot | DoS | Snapshot size cap: at single-tenant scale (≤ a few hundred MPs), the snapshot is bounded. If a customer somehow has >1000 MPs in scope, paginate the snapshot |
| Replay attack via Last-Event-ID | Tampering | D-03 explicitly rejects Last-Event-ID cursor replay — server always sends fresh snapshot. No tamperable cursor exists |
| Cross-origin SSE hijack | Tampering | SameSite=Lax cookie + same-origin SSE; `Caddy` is the trust boundary |
| NOTIFY-payload injection (malicious data crafted to embed `\n\n` and split SSE events) | Tampering | Server constructs the SSE wire frame from the structured payload after JSON-decoding the NOTIFY content; never echoes raw bytes to the wire. The compact 7-field shape contains only numeric / enum / UUID / ISO-timestamp fields — no free-form strings |
| Session fixation via cookie pinning on SSE | Spoofing | Existing SCS session-rotation pattern on login (`PutUser` bundles `Put + RenewToken`) already mitigates |

## Sources

### Primary (HIGH confidence)

- `[VERIFIED: internal/resolver/listener.go]` — exact LISTEN/NOTIFY listener pattern; reconnect loop with 2s backoff; dedicated `pgxpool.Conn`; reused verbatim shape by Phase 4 `internal/events/listener.go`.
- `[VERIFIED: internal/db/migrations/0015_measurement.up.sql]` — full measurement schema; index coverage; `measurement_inserted` trigger slot reserved at L78-81.
- `[VERIFIED: internal/http/router.go]` — chi middleware order + `Deps` struct pattern + canonical PITFALL #4 SPA fallback ordering.
- `[VERIFIED: web/src/components/ui/chart.tsx]` — Recharts wrapper (`ChartContainer`, `ChartTooltip`, `ChartLegend`); ready for cumulative chart and sparklines.
- `[VERIFIED: web/src/App.tsx]` — current routing skeleton with `IndexRedirect`; lazy-imports all routes.
- `[VERIFIED: web/src/routes/index-redirect.tsx]` — confirmed placeholder; replaces with `DashboardPage` in Phase 4.
- `[VERIFIED: web/src/components/shell/sidebar.tsx]` — comment "Phase 4 will insert Dashboard above all" confirms the slot.
- `[VERIFIED: web/package.json]` — recharts 3.8.0, @tanstack/react-query 5.100.5, @tanstack/react-table 8.21.3, react-router-dom 7.14.2, zod 4.3.6, sonner 2.0.7, date-fns 4.1.0 — every Phase 4 frontend dep present.
- `[VERIFIED: go.mod]` — pgx/v5 5.9.2, chi v5.2.5, scs/v2 2.9.0, testcontainers-go 0.42.0; every backend dep present.
- `[VERIFIED: .planning/phases/04-realtime-dashboard/04-CONTEXT.md]` — D-01..D-23 locked decisions; this research operates inside that envelope.
- `[VERIFIED: .planning/phases/04-realtime-dashboard/04-UI-SPEC.md]` — UI design contract; visual/interaction shapes referenced throughout.
- `[CITED: https://www.postgresql.org/docs/current/sql-notify.html]` — 8000-byte payload limit + transactional NOTIFY semantics.
- `[CITED: https://www.postgresql.org/docs/current/sql-listen.html]` — LISTEN session-bound semantics.
- `[CITED: https://developer.mozilla.org/en-US/docs/Web/API/EventSource]` — EventSource API, retry field semantics, no-custom-headers limitation.

### Secondary (MEDIUM confidence, verified against official source)

- `[CITED: https://docs.timescale.com/api/latest/hypertable/create_hypertable/]` — hypertable creation; chunk semantics.
- `[CITED: https://github.com/timescale/timescaledb/issues/2304]` — trigger propagation from parent hypertable to chunks documented behavior.
- `[CITED: https://github.com/timescale/timescaledb/issues/1343]` — CONSTRAINT TRIGGER limitation on hypertables (Phase 4 doesn't use this trigger type, so unblocking).
- `[CITED: https://brandur.org/notifier]` — "The Notifier Pattern" — single dedicated connection per process for LISTEN; corroborates the resolver pattern.
- `[CITED: https://github.com/jackc/pgx/issues/195]` — confirms LISTEN cannot share a pooled connection.
- `[CITED: https://github.com/jackc/pgx/issues/928]` — chat-app example pattern for async `WaitForNotification`.
- `[CITED: https://github.com/jackc/pgx/issues/1121]` — pgxpool LISTEN community pattern discussion.
- `[CITED: https://pkg.go.dev/github.com/jackc/pgxlisten]` — higher-level alternative library (decided NOT to adopt; resolver pattern is already in repo).
- `[CITED: https://ui.shadcn.com/docs/components/radix/date-picker]` — shadcn date-picker recipe built on react-day-picker + date-fns.
- `[CITED: https://ui.shadcn.com/docs/components/radix/calendar]` — shadcn `<Calendar mode="range">` recipe.
- `[CITED: https://github.com/Azure/fetch-event-source]` — `@microsoft/fetch-event-source` API as alternative to native `EventSource`.

### Tertiary (LOW confidence; corroborate before locking decisions on these alone)

- TimescaleDB 2.26 specific trigger propagation behavior — confirmed via search hits but not verified against the 2.26 release notes directly. Integration test catches regressions (see §Pitfalls 3).
- Vendor expected-uplink intervals for Axioma W1 and Acrel ADW300 — values quoted are typical defaults; confirm against actual product datasheets at plan time before backfilling seed migration.

## Metadata

**Confidence breakdown:**

- Standard stack: HIGH — every dep already in repo at known-good versions; CLAUDE.md locks the choices.
- Architecture patterns: HIGH for SSE handler, LISTEN listener (proven in resolver), trigger SQL, KPI computation, time_bucket charts; MEDIUM for SSE backpressure micro-policy (Claude's Discretion) and the per-binding-vs-per-device online detection nuance.
- Pitfalls: HIGH — most are documented in CONTEXT canonical refs (PITFALLS §9, §3, §12) or in this research from verified sources.
- Schema gaps: HIGH — verified by direct codebase grep (`expected_interval_s`, `install_identity.capabilities` both 0 hits).
- TimescaleDB 2.26 trigger behavior on chunks: MEDIUM — verified by issue thread; integration test in Wave 0 will catch any regression definitively.

**Research date:** 2026-05-11
**Valid until:** 2026-06-10 (30 days — stable stack, slow-moving libraries; revisit only if pgx, recharts, react-day-picker, or TimescaleDB ship a major in this window)
