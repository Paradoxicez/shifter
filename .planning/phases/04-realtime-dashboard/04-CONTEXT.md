# Phase 4: Realtime & Dashboard - Context

**Gathered:** 2026-05-11
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 4 turns Shifter into a daily-driver dashboard by making the canonical telemetry (Phase 2) visible in real time at fleet scale (Phase 3):

- **Live dashboard** — adaptive (water / electricity / both), KPI tiles (today's consumption, current flow/instant draw, period delta, online/offline device count), time-series charts with a date-range picker, mobile-responsive.
- **SSE realtime push** — `measurement` AFTER INSERT trigger emits `pg_notify('measurement_inserted', ...)`; Go fan-out service maintains per-session topic subscriptions and ships compact deltas. Reuses the LISTEN/NOTIFY substrate proven by Phase 2 (`binding_changed`).
- **Per-meter detail page** — tabbed layout (Normal / Advanced / Uplinks log) surfacing canonical fields, full decoded `object`, and the last 100–500 uplinks with battery + RSSI/SNR sparklines.

Out of scope (own phases): continuous aggregates and reports (Phase 5), map view + floor plans (Phase 5), alerts (Phase 6), audit browse UI (Phase 6), user management (Phase 6).

</domain>

<decisions>
## Implementation Decisions

### SSE wire protocol & snapshot semantics

- **D-01:** **One session-scoped `/api/events` stream + server-side topic filter.** Browser opens a single `EventSource('/api/events')` per tab. Client `POST /api/events/subscribe` (or `?topics=` on initial connect) lists topics it cares about — `dashboard:global`, `mp:<uuid>`. Backend keeps a `connection → topics` map and filters before write. Single TCP/HTTP2 stream per tab; subscribe/unsubscribe lifecycle owned by the route component. Avoids the per-page-endpoint connection explosion and the global-broadcast bandwidth tax (PITFALL §9).
- **D-02:** **Compact pg_notify payload — `(metering_point_id, time, cumulative_value, instant_value, quality, battery_pct, rssi)`** as JSON ≤200B. Fits well inside pg_notify's 8KB cap (avoids the silent-truncation risk of stuffing `raw_payload` + `decoded_object` in). KPI tiles + detail-page latest-reading widget update directly from the event; no follow-up REST fetch needed on the hot path. Trigger SQL lives in a new migration (slot reserved at `0015_measurement.up.sql:L78-81`).
- **D-03:** **Always-full-snapshot on initial connect AND every reconnect.** Server emits `event: snapshot` carrying `latest-reading-per-MP` for every subscribed topic (KPIs + chart latest points), then resumes streaming `event: measurement` deltas. No Last-Event-ID cursor, no replay ring buffer — single-tenant scale makes the snapshot cheap (≤ a few hundred MPs) and the protocol stays stateless on the server.
- **D-04:** **Client reconnect = exponential backoff with jitter — `min(30s, 0.5s * 2^n) + rand(0, 1s)`.** Server heartbeat = SSE comment `: heartbeat` every 30s so reverse proxies (Caddy `flush_interval -1` already set in Phase 1 D-20..22) don't drop idle connections and the client can detect zombie sessions. PITFALL §9 mandate.

### KPI definitions & online/offline rules

- **D-05:** **"Today's consumption" = `[00:00 today install_tz → now]`** computed against `install_identity.timezone` (set at wizard step 2). One sqlc query per render aggregates per-utility cumulative deltas across active MPs. Phase 5 reports key off the same boundary, so the definition is consistent end-to-end.
- **D-06:** **"Current flow / instantaneous draw" = latest `measurement.instant_value` per MP, summed per `utility_class`.** Water → L/min total; electricity → W total. Driven by SSE delta events on the dashboard topic — when an `instant_value` arrives, the tile re-renders without a server round-trip.
- **D-07:** **Online vs offline = `device.last_seen_at > now() − 2 × device_profile.expected_interval_s`.** Per-profile so a 60-min water meter and a 15-min electricity meter get appropriately different thresholds. This is intentionally **looser** than Phase 6 ALERT-02 (N≥3 missed uplinks with hysteresis) — KPI flicker is OK; alert paging is not. Backend computes once per dashboard render or per 30s tick, whichever comes first.
- **D-08:** **Period delta = `today [00:00 → now]` vs `yesterday [00:00 → same-time-yesterday]`.** Reports both absolute delta and % change. Apples-to-apples: comparing partial today against the same fraction of yesterday avoids the always-"−50%" footgun of "today partial vs yesterday full-day."
- **D-09:** **Adaptive scope = admin-managed flag in install identity, NOT auto-detected.** New `install_identity.capabilities` column (or equivalent — planner picks shape) with values `water` | `electricity` | `both`. Admin chooses at wizard time and can toggle from Settings later. Auto-detection deferred (see Deferred Ideas). Dashboard renders only the chosen panels; non-matching MPs that get added later still ingest correctly but are gated from the dashboard until admin updates the flag.
- **D-10:** **Dashboard chart dimension = cumulative per utility class.** One chart for water (m³ stacked across MPs in scope), one for electricity (kWh stacked). Y-axis = aggregate cumulative; X-axis = time. Per-MP breakdown lives on the MP detail page, not the dashboard. Instantaneous flow/power surfaces as KPI tile, not as its own dashboard chart.

### Date-range pickers & raw-chart strategy

- **D-11:** **Presets: Today / 24h / 7d / 30d / Custom.** Custom uses shadcn DatePicker range. Stored in URL search params so the view is shareable and back/forward-aware (consistent with Phase 3 D-15 devices URL state).
- **D-12:** **Downsampling via TimescaleDB `time_bucket()` sized to the range, no CAGGs in Phase 4.** Bucket schedule:
  - Today / 24h → 5-minute buckets
  - 7d → 1-hour buckets
  - 30d → 4-hour buckets
  - Custom > 30d → 1-day buckets
  Query shape: `SELECT time_bucket($bucket, time) AS bucket, metering_point_id, sum(cumulative_delta), avg(instant_value) FROM measurement WHERE time BETWEEN $start AND $end AND metering_point_id = ANY($mps) GROUP BY bucket, metering_point_id ORDER BY bucket`. CAGGs (Phase 5) will replace the heavier 7d/30d/custom paths transparently.
- **D-13:** **Live-mode chart update only for `Today` and `24h` presets.** SSE deltas append to the rightmost bucket in place; 7d/30d/Custom are frozen snapshots and refresh only when the user changes the range or clicks an explicit refresh. Mirrors the operator's mental model — "live tail" doesn't make sense on a month-long view.
- **D-14:** **Shared dashboard-level date-range picker — one picker drives every chart on the dashboard.** Per-MP detail-page picker is independent (each MP page has its own). Mirrors the operator workflow of "compare water and electricity over the same 7 days" with zero clicks.

### Per-meter detail layout, uplinks log, empty states

- **D-15:** **Tabbed detail page — `Normal` / `Advanced` / `Uplinks log`.** Default tab is Normal. Mobile-friendly (one tab fills the viewport). Normal = cumulative + instant + last-update timestamp + quality badge (D-19) + alarms (Phase 6 placeholder). Advanced = collapsible JSON tree of `decoded_object` + `extra` JSONB. Uplinks log = TanStack Table of recent events with filter chips.
- **D-16:** **Uplinks log = TanStack Table, default 100 rows + 'Load more' (+100 each click, capped at 500).** Columns: `time` (server ingest), `quality` (badge), `cumulative_value`, `instant_value`, `battery_pct`, `rssi`, `snr`, `fcnt`, `expand` (row reveals raw_payload + decoded_object). Filters: `quality` (multi-select chips for `ok | decode_fail | missing_canonical | out_of_range | duplicate_fcnt`) + date-range picker (independent from dashboard's). SSE prepends new rows when the user is on the Uplinks log tab — virtualized list so 500 rows stay smooth.
- **D-17:** **Battery + RSSI/SNR sparklines = last 24h, `time_bucket('1 hour')` (24 buckets).** Same bucket strategy as the dashboard's 24h chart. SSE updates the rightmost bucket in place. Compact (24 data points) — keeps the Normal tab uncluttered on mobile.
- **D-18:** **Raw payload display = hex monospace + expandable row.** BYTEA rendered as space-separated hex (`0a 1f 3c 4d ...`) in JetBrains Mono (already in Phase 1 font bundle). Vendor docs and DevEUI stickers (Phase 3 D-11) are hex by convention. Expand row → wrapped full hex on left, formatted `decoded_object` JSON on right.
- **D-19:** **Quality badge surfaces on the Normal tab + filter button → Uplinks log.** "3 of last 100 uplinks flagged" amber badge in the Normal-tab status row when `quality != 'ok'` count > 0 over the last 100 uplinks. Click → switches to Uplinks log tab with the `quality != ok` filter pre-applied. Phase 2 D-26 "never silent drop" finally has an operator surface.
- **D-20:** **Advanced tab = collapsible JSON tree.** Recursive component with expand/collapse per node; primitives render inline, objects/arrays render expandable. Handles nested vendor JSON (e.g. Acrel ADW300's per-phase l1/l2/l3 sub-objects) without flattening loss. No third-party `react-json-view`-class dep yet — custom component fits the modern-minimal aesthetic. Phase 7 codec test-runner will reuse this component.
- **D-21:** **Empty state — dashboard: progressive onboarding card.** Detected by `(gateway_count, device_count, uplink_count)` tuple:
  - `(0, _, _)` → "Add your first gateway" + CTA → `/gateways`
  - `(>0, 0, _)` → "Now add your first device" + CTA → `/devices`
  - `(>0, >0, 0)` → "Waiting for first uplink…" with a subdued spinner and gateway-online status row
  - `(>0, >0, >0)` → real dashboard
  Empathetic + actionable, ties directly to Phase 3 surfaces.
- **D-22:** **Empty state — MP detail with no binding/uplink: keep tabs visible.** Normal tab shows MP info card (`name`, `utility_class`, `site`, `location_description`) + status row "No device bound — [Add device]" with CTA. Advanced + Uplinks log tabs render disabled with tooltip "available after first uplink". Avoids layout shift when the operator binds a device + the first uplink lands.
- **D-23:** **Viewer authorization = full read access on dashboard + detail.** Phase 4 introduces no mutations; viewers see every KPI, chart, advanced tab, uplinks log, and raw_payload — same as admin. CTAs that lead to mutations (e.g. "Add device" in empty state) are hidden for viewers using existing `Can()` checks. Matches AUTH-06 ("viewer = read-only") and Phase 2 D-22 (decoded_object is diagnostic, not secret).

### Claude's Discretion

- Exact `time_bucket` widths inside each preset (tunable; D-12 numbers are the starting point)
- KPI tile layout grid (shadcn Card sizes, ordering — water-first vs electricity-first)
- Sparkline visual styling (Recharts area vs line, color)
- JSON tree component design — depth indentation, primitive type colors
- Hex-payload column width and copy-to-clipboard placement
- Snapshot event payload exact field ordering / pagination strategy if a subscribed topic returns >1000 MPs (single-tenant scale makes this unlikely but planner can decide a cap)
- SSE backpressure policy when a client can't keep up (drop oldest delta + force re-snapshot vs drop connection — PITFALL §9 hint, planner finalizes)
- Per-tab SSE topic management (Uplinks log tab subscribes to `mp:<uuid>:uplinks`; Normal tab subscribes to `mp:<uuid>` — naming convention is planner's choice as long as server-side filter matches)
- Whether `device.last_seen_at` is good enough for D-07 or whether a per-binding `last_uplink_at` view is needed (binding swaps change which device feeds an MP — planner reads the resolver code)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase scope & requirements
- `.planning/PROJECT.md` — Single-tenant install; modal-first CRUD (Phase 4 introduces NO mutations); mobile-responsive web; shadcn/ui aesthetic
- `.planning/REQUIREMENTS.md` §DASH-01..06, DETL-01..03 — Phase 4 acceptance criteria
- `.planning/ROADMAP.md` §Phase 4 — Goal statement + 5 success criteria (adaptive scope, SSE-driven KPIs, reconnect-with-snapshot, normal+advanced detail view, recent-uplinks-log with sparklines)

### Research artifacts
- `.planning/research/SUMMARY.md` §"Phase 4: Telemetry realtime + dashboard + per-meter detail" — pattern lockup (pg_notify → SSE; adaptive scope; normal+advanced view; sparklines; per-widget subscription)
- `.planning/research/ARCHITECTURE.md` §Pattern 5 (Realtime Push via Postgres LISTEN/NOTIFY) — single-tenant 8KB-cap discipline; LISTEN/NOTIFY → in-Go fan-out
- `.planning/research/PITFALLS.md` §9 (WebSocket/SSE Reconnect Storms) — exponential-backoff + jitter + heartbeat + per-widget filter (D-01, D-04 directly mitigate)
- `.planning/research/PITFALLS.md` §3 (Codec Hell — never silent drop; D-26 quality badge) — D-19 is the operator surface

### Prior phase context (locked decisions Phase 4 inherits)
- `.planning/phases/01-foundation/01-CONTEXT.md` D-20..D-22 — Caddyfile SSE-aware proxy posture (no extra config needed in Phase 4)
- `.planning/phases/02-domain-model-canonical-schema/02-CONTEXT.md` D-17 (`install_identity.timezone` — D-05 source), D-19 (`metering_point.utility_class` — D-09 enforcement), D-22..D-24 (audit_log shape — Phase 4 reads but doesn't write audit), D-25 (LISTEN/NOTIFY substrate already wired — Phase 4 adds `measurement_inserted` channel), D-26 (never silent drop + `quality` flag — D-19 surface)
- `.planning/phases/03-provisioning-gateways-devices-bulk-import/03-CONTEXT.md` D-12..D-18 (TanStack Table + URL search-params pattern — Uplinks log reuses), D-29 (single-tenant ChirpStack abstraction — Phase 4 still uses `metering_point_id` as the only key)

### Code anchors
- `internal/db/migrations/0015_measurement.up.sql` §L78-81 — slot reserved for the `measurement_inserted` LISTEN/NOTIFY trigger added in Phase 4
- `internal/db/migrations/0005_install_identity.up.sql` — `timezone` column (D-05 source) + adding `capabilities` (D-09) is a new migration
- `internal/db/migrations/0009_device_profile.up.sql` — `expected_interval_s` (D-07 source)
- `internal/db/migrations/0012_device.up.sql` §L28 — `last_seen_at` already maintained by ingest hot path (D-07 reads this)
- `internal/resolver/listener.go` + `doc.go` — the canonical LISTEN/NOTIFY pattern (reconnect with backoff, dedicated pgx conn, defer Release); Phase 4 SSE service mirrors this shape
- `internal/ingest/persist.go` — confirms `measurement` is the single write site (so the trigger fires once per uplink), and `device.last_seen_at` advances inside the same Serializable tx
- `internal/http/router.go` — Phase 4 mounts `GET /api/events` (SSE) under the authenticated group; new handlers `GET /api/dashboard/snapshot`, `GET /api/metering-points/:id/timeseries`, etc.
- `web/src/App.tsx` + `web/src/routes/index-redirect.tsx` — Phase 4 owns `/` and replaces the `IndexRedirect → /settings` placeholder with a real DashboardPage; sidebar comment in `web/src/components/shell/sidebar.tsx` confirms "Phase 4 will insert Dashboard above all"
- `web/src/components/ui/chart.tsx` — Recharts wrapper already wired (Phase 1); Phase 4 uses Recharts area/line for cumulative + sparklines

### External / vendor docs
- TimescaleDB `time_bucket` reference — https://docs.timescale.com/api/latest/hyperfunctions/time_bucket/
- TimescaleDB `LISTEN/NOTIFY` integration — https://docs.timescale.com/use-timescale/latest/extensions/postgres-extensions/ (and Postgres docs: https://www.postgresql.org/docs/current/sql-listen.html, https://www.postgresql.org/docs/current/sql-notify.html — 8KB payload cap is the binding constraint on D-02)
- MDN `EventSource` API — https://developer.mozilla.org/en-US/docs/Web/API/EventSource (Last-Event-ID, retry field semantics)
- SSE vs WebSocket vs MQTT (operational tradeoffs) — https://websocket.org/comparisons/ (confirms the Phase 4 choice of SSE for one-way push)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets

- `internal/resolver/listener.go` — canonical LISTEN/NOTIFY pattern with reconnect-with-backoff, dedicated pgx conn, defer Release. Phase 4 SSE fan-out service is a near-copy with a different channel name (`measurement_inserted`).
- `internal/resolver/invalidate.go` — out-of-band `pg_notify` helper from a tx; useful if planner adds a defensive "republish on swap" path for SSE deltas.
- `internal/ingest/persist.go` — the single write site for `measurement`. The new `AFTER INSERT` trigger fires exactly once per uplink because of the Serializable tx structure documented in this file's package doc.
- `internal/http/router.go` — `Deps` struct + middleware ordering is the pattern. Phase 4 adds `EventsDeps` (SSE fan-out + topic subscription store) + handler-family wiring identical to Phase 3's `GatewayDeps` / `ImportDeps`.
- `internal/auth/can.go` — `Can(user, action, resource)` API; Phase 4 reads existing actions only (no new mutations) but uses `auth.Can` to gate the "Add device" CTAs in empty-state cards.
- `web/src/components/ui/chart.tsx` — Recharts ChartContainer + ChartConfig in place. Phase 4 builds 2 dashboard charts (water cumulative, electricity cumulative) + 3 sparklines (battery, RSSI, SNR) on top.
- `web/src/components/shell/sidebar.tsx` — sidebar comment explicitly reserves the top slot for the Phase 4 Dashboard nav item.
- `web/src/routes/index-redirect.tsx` — placeholder Phase 4 replaces with a real DashboardPage.
- TanStack Query already in the bundle (Phase 1) — used for snapshot fetch + SSE-driven cache invalidation on dashboard + per-MP timeseries.
- TanStack Table already in the bundle (Phase 2) — Uplinks log uses it.
- shadcn primitives `Tabs`, `Card`, `Badge`, `Skeleton`, `Tooltip`, `Popover`, `ScrollArea` — all already added; Phase 4 doesn't need any new shadcn imports for the core dashboard + detail layout. May add `react-day-picker` (shadcn DatePicker recipe) if not already in the chart bundle.

### Established Patterns

- LISTEN/NOTIFY with dedicated pgx connection + outer reconnect loop with 2s backoff (Phase 2 D-25 / `internal/resolver/listener.go`)
- Server-side filter at the broadcast layer (PITFALL §9) — D-01 makes this concrete for SSE
- URL search params via TanStack Router (Phase 3 D-15) — Phase 4 dashboard + MP detail picker state rides this
- TanStack Table for filterable + paginated lists (Phase 3 D-12..D-18) — Uplinks log inherits column-fixed/no-user-toggle policy
- Mobile-responsive shell with Topbar + Sidebar + Outlet (Phase 1) — Phase 4 dashboard fits inside without shell changes
- Modal-first CRUD (UX-01) — Phase 4 introduces NO new dialogs (read-only phase); empty-state CTAs link to existing Phase 3 dialogs

### Integration Points

- **New migration** — `0021_measurement_inserted_trigger.up.sql` adds the `AFTER INSERT` trigger emitting `pg_notify('measurement_inserted', json_build_object(...))` with the 7 compact fields per D-02. Planner picks the next free migration number (0021 confirmed by `ls internal/db/migrations/`).
- **New migration** — `0022_install_capabilities.up.sql` (or extension of existing install_identity table) adds the `capabilities` column for D-09 adaptive scope. Migration scope is single column + default.
- **New Go package** — `internal/events/` (or similar — planner picks idiomatic name) houses the SSE fan-out service: pgx listener loop, per-connection topic registry, write-with-backpressure helper, heartbeat ticker.
- **New HTTP routes** under `internal/http/router.go`:
  - `GET /api/events` (SSE) — authenticated; both roles
  - `POST /api/events/subscribe` / `DELETE /api/events/subscribe` (or `?topics=` on connect — planner picks)
  - `GET /api/dashboard/snapshot` — KPI tiles + latest-reading-per-MP for the install scope
  - `GET /api/dashboard/timeseries?range=...&utility=...` — chart data via `time_bucket()`
  - `GET /api/metering-points/:id/detail` — Normal tab data (cumulative, instant, last-update, quality summary)
  - `GET /api/metering-points/:id/timeseries?range=...` — per-MP chart + sparkline data
  - `GET /api/metering-points/:id/uplinks?limit=100&before=...&quality=...` — Uplinks log
- **Frontend SPA** — `/` now renders `DashboardPage` (instead of redirecting); new `/metering-points/:id` overhaul replaces the Phase 2 minimal page with the 3-tab layout; sidebar gains "Dashboard" nav item at the top.

</code_context>

<specifics>
## Specific Ideas

- The product still must feel like Shifter, never like a re-skinned ChirpStack — even in Advanced view, the JSON tree shows the decoded `object` Shifter-style (with canonical field names where mapping applies) rather than ChirpStack's raw `application/+/device/+/event/up` JSON envelope.
- The empty-state onboarding card is the only Phase 4 surface where Shifter has personality — it's the moment the operator first opens the app and the install is genuinely empty. Tone: empathetic + actionable, not corporate.
- "X of last 100 uplinks flagged" badge (D-19) is the operator's first observable signal that codec drift is happening — and it's been latent in the Phase 2 quality column for two phases. Phase 4 makes Phase 2's design choice pay off.
- 7 compact NOTIFY fields (D-02) is deliberately a superset of what KPI tiles need — `battery_pct` and `rssi` are there so the sparkline tail can update without a follow-up query, even though they don't show on the dashboard's KPI cards.
- Adaptive scope flag (D-09) at install identity feels heavyweight, but it's the only place that survives full-fleet decommission → re-add cycles. Auto-detect from MPs would oscillate during onboarding ("operator added 1 water MP to test, dashboard shows water-only mode, then they add 50 electricity MPs and the layout flickers").
- Live-mode chart auto-update only on `Today/24h` (D-13) matches the actual operator workflow — long-range views are reviewed deliberately, not watched in real time.

</specifics>

<deferred>
## Deferred Ideas

- **Continuous aggregates** for chart performance — Phase 5 (DATA-11..13). Phase 4 uses `time_bucket()` on raw `measurement`; queries get faster automatically when CAGGs land.
- **Auto-detect adaptive scope** from `SELECT DISTINCT utility_class` — kept as a fallback if the admin flag is unset (planner can decide to seed the flag from auto-detect at install finish). Pure-auto detection deferred.
- **Last-Event-ID cursor replay** for SSE reconnect — chose always-full-snapshot (D-03). Revisit if reconnect snapshot becomes too expensive at scale (>1000 MPs per topic).
- **Per-page SSE endpoints** vs session-scoped — chose session-scoped (D-01). Revisit only if topic subscription state management becomes a hotspot.
- **WebSocket** (bidirectional) — Phase 4 is one-way (server → browser). Reconsider when bidirectional needs appear (e.g. live device control in Phase 7+).
- **Saved dashboard views / per-user date-range preferences** — V2-AUTH-02. Phase 4 ships shared dashboard with URL-state range picker.
- **Site-grouped dashboard breakdown** — single dashboard "all MPs in scope" in Phase 4. Per-site dashboard surfaces in Phase 5 alongside the map.
- **Quality-flag drill-down beyond the badge** — D-19 surfaces the badge + filter; deeper analytics (quality % trend, codec-drift detection) belong with Phase 7's codec test-runner.
- **Custom user-tunable bucket widths** — D-12 schedule is fixed in Phase 4; user-tunable buckets deferred to Phase 5/7.
- **Detail-page admin-only gate on Advanced/Uplinks log tab** — viewers see everything in Phase 4 (D-23). Revisit if a customer reports that diagnostic JSON is sensitive.
- **Pre-compute "latest-reading-per-MP" materialized view** for snapshot performance — research note (SUMMARY.md §Phase 4) suggested this; Phase 4 starts with on-demand sqlc query and adds the matview only if snapshot latency is observed as a problem.
- **Server backpressure policy (snapshot reissue vs disconnect)** — left to Claude's Discretion; revisit empirically once SSE is in customer hands.
- **Mobile native dashboard** — V2-MOB-01.
- **Hex / decoded_object copy-to-clipboard buttons** — small polish, planner can decide whether to ship in Phase 4 or defer.

</deferred>

---

*Phase: 04-realtime-dashboard*
*Context gathered: 2026-05-11*
