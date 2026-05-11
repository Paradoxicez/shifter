# Phase 4: Realtime & Dashboard - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-05-11
**Phase:** 04-realtime-dashboard
**Areas discussed:** SSE wire protocol & snapshot semantics; KPI definitions & online/offline rules; Date-range pickers & raw-chart strategy; Per-meter detail layout, uplinks log, empty states

---

## SSE wire protocol & snapshot semantics

### Q1 — Channel topology

| Option | Description | Selected |
|--------|-------------|----------|
| Session-scoped `/api/events` + server-side filter | Single EventSource per tab; client subscribes to topics; backend filters before write | ✓ |
| Per-page endpoints (`/api/events/dashboard`, `/api/events/metering-points/:id`) | URL bakes scope; multiplies connections when multiple tabs open | |
| Global stream + client-side filter | Server pushes everything; client ignores irrelevant; bandwidth scales with fleet × users | |

**User's choice:** Session-scoped + server-side filter
**Notes:** User explicitly asked for clarification before deciding. Decision aligned with PITFALL §9 broadcast-layer filtering recommendation and ARCHITECTURE.md Pattern 5.

### Q2 — pg_notify payload shape

| Option | Description | Selected |
|--------|-------------|----------|
| Compact: (mp_id, time, cumulative, instant, quality, battery, rssi) | ~200B JSON, well under 8KB cap; KPI + sparkline update without refetch | ✓ |
| ID-only: (mp_id, time) + client refetch | Trigger as signal; TanStack Query handles dedup/cache; extra round-trip | |
| Full row (20 columns + raw_payload + decoded_object) | One source of truth; risk of >8KB truncation on multi-phase electricity | |

**User's choice:** Compact ~200B JSON

### Q3 — Snapshot-on-reconnect semantics

| Option | Description | Selected |
|--------|-------------|----------|
| Always-full-snapshot on initial connect + every reconnect | Stateless server; cheap at single-tenant scale | ✓ |
| Last-Event-ID cursor + ring buffer replay | Standard SSE replay; adds buffer state + PITFALL §9 unbounded-buffer risk | |
| No snapshot — client GET REST on connect | Bypasses SSE for snapshot; no guarantee deltas don't arrive before REST | |

**User's choice:** Always-full-snapshot

### Q4 — Reconnect + heartbeat strategy

| Option | Description | Selected |
|--------|-------------|----------|
| Exponential backoff + jitter + 30s heartbeat | min(30s, 0.5s * 2^n) + rand(0,1s); PITFALL §9 mandate | ✓ |
| Linear backoff (5s/10s/15s) + 60s heartbeat | Gentler ramp but thundering-herd risk on mass restart | |
| Native EventSource auto-reconnect + 30s heartbeat | Browser built-in (3s default no jitter); fails PITFALL §9 jitter rule | |

**User's choice:** Exponential backoff + jitter + 30s heartbeat

---

## KPI definitions & online/offline rules

### Q1 — "Today's consumption" timezone

| Option | Description | Selected |
|--------|-------------|----------|
| Install timezone, midnight-to-now | `install_identity.timezone` source-of-truth; consistent with Phase 5 reports | ✓ |
| Rolling 24h (now − 24h → now) | Simple; no timezone lookup; "today" is misleading | |
| UTC midnight-to-now | Easiest; wrong for Thai users (UTC+7 shifts "today" by 7h) | |

**User's choice:** Install timezone, midnight-to-now

### Q2 — Current flow / instantaneous draw KPI

| Option | Description | Selected |
|--------|-------------|----------|
| Latest `measurement.instant_value` per MP, summed by utility_class | Driven by SSE deltas; sums across all in-scope MPs | ✓ |
| Latest of MP with `last_seen < 5min` | Filters stale meters; 5min unsuitable for 15-60min LoRaWAN intervals | |
| Rolling 1h average of `instant_value` | Smooths noise but not "current" anymore; needs CAGGs Phase 5 doesn't have yet | |

**User's choice:** Latest per MP summed by utility_class

### Q3 — Online vs offline rule

| Option | Description | Selected |
|--------|-------------|----------|
| Per-profile: `last_seen > now − 2 × expected_interval_s` | Looser than Phase 6 ALERT-02; KPI flicker acceptable, alert paging not | ✓ |
| Global 2h threshold | Simple but punishes 15-min electricity, lazy for 4h-interval meters | |
| Configurable in Settings (default + per-profile override) | Most flexible but Phase 4 has no alert-settings UI (Phase 6) | |

**User's choice:** Per-profile 2× expected_interval

### Q4 — Period delta basis

| Option | Description | Selected |
|--------|-------------|----------|
| Today [00:00 → now] vs yesterday [00:00 → same-time-yesterday] | Apples-to-apples partial-day comparison | ✓ |
| Today vs yesterday full 24h | Today partial always looks "lower"; misleading early in the day | |
| Rolling 24h vs previous 24h | No midnight boundary; doesn't match "today" KPI framing | |

**User's choice:** Today partial vs yesterday partial

### Q5 — Adaptive scope detection

| Option | Description | Selected |
|--------|-------------|----------|
| Auto-detect: `SELECT DISTINCT utility_class FROM metering_point` | Zero settings field; cached 30s; oscillates during onboarding | |
| Settings flag: admin toggles Water/Electricity/Both | Explicit; admin-controlled; needs sync if MP utility_class diverges | ✓ |
| Show both panels always, render `—` when no MP | Doesn't satisfy DASH-01 "adapts to scope" | |

**User's choice:** Settings flag (admin-managed)
**Notes:** Auto-detect kept as deferred-idea fallback for install-finish seeding. User preferred explicit admin control over implicit detection.

---

## Date-range pickers & raw-chart strategy

### Q1 — Date-range picker presets

| Option | Description | Selected |
|--------|-------------|----------|
| Today / 24h / 7d / 30d / Custom | 5 presets covering daily ops + weekly + monthly | ✓ |
| Today / 7d / 30d / Custom (no 24h) | Trims 24h; 24h-rolling has different meaning ("today partial" vs "rolling 24h") | |
| Custom-only (no presets) | Conflicts with "modern minimal dashboard" goal — too much friction | |

**User's choice:** Today / 24h / 7d / 30d / Custom

### Q2 — Chart downsampling strategy (no CAGGs in Phase 4)

| Option | Description | Selected |
|--------|-------------|----------|
| TimescaleDB `time_bucket()` sized to range | 5m/1h/4h/1d depending on range; native Timescale feature | ✓ |
| Always 1-min raw + frontend aggregation | 30d range = 200k+ points; browser-side aggregation too expensive | |
| Fixed 200-point output regardless of range | Mathematically clean but 7d/200 ≠ 1h-bucket; non-intuitive boundaries | |

**User's choice:** `time_bucket()` per-range

### Q3 — Live-mode auto-update

| Option | Description | Selected |
|--------|-------------|----------|
| Live only for Today / 24h presets | "Live tail" mismatches long-range review workflow | ✓ |
| Live for all ranges (auto-append even on 7d/30d) | Operator on 30d view doesn't expect rightmost bucket twitching | |
| All frozen + manual refresh | Loses the "live" dashboard vibe Phase 4 promises | |

**User's choice:** Live only for Today / 24h

### Q4 — Date-picker scope

| Option | Description | Selected |
|--------|-------------|----------|
| Shared dashboard-level picker | One picker drives every chart; matches "compare water vs electricity same week" workflow | ✓ |
| Per-chart picker | Maximum flexibility but rare workflow + UI noise | |
| Hybrid: shared default + per-chart override | Engineering cost > workflow value | |

**User's choice:** Shared dashboard-level picker

### Q5 — Chart dimension(s)

| Option | Description | Selected |
|--------|-------------|----------|
| Cumulative per utility (water m³, electricity kWh) | 1 chart per utility; aligns with "today's consumption" KPI; per-MP detail elsewhere | ✓ |
| Instantaneous (flow/power) per utility | Live feel but 30d view becomes pure noise without smoothing | |
| Both (cumulative + instant side-by-side per utility) | Information dense; mobile cramped; 4 charts on dashboard | |

**User's choice:** Cumulative per utility class

---

## Per-meter detail layout, uplinks log, empty states

### Q1 — Detail page layout

| Option | Description | Selected |
|--------|-------------|----------|
| Tabs: Normal / Advanced / Uplinks log | 3 tabs; mobile-friendly one-tab-per-viewport | ✓ |
| Collapsible: normal default + accordion-expand Advanced + Uplinks log | Spec says "collapsible" but scroll on mobile is rough | |
| Single scrollable page (no hide) | Hides nothing but contradicts DETL-01 "collapsible" default | |

**User's choice:** Tabs (Normal / Advanced / Uplinks log)

### Q2 — Uplinks log default count + filters

| Option | Description | Selected |
|--------|-------------|----------|
| Default 100 rows + 'Load more' (cap 500) + quality + date filters | DETL-02 spec sweet spot | ✓ |
| Default 500 + no pagination | 500 rows on mobile = render lag; manual SSE prepend bookkeeping | |
| Pagination 50/page + date filter (no SSE prepend) | Loses "live" feel; user must manually refresh | |

**User's choice:** Default 100 + Load more

### Q3 — Sparkline window (battery + RSSI/SNR)

| Option | Description | Selected |
|--------|-------------|----------|
| Last 24h, `time_bucket('1 hour')` (24 buckets) | Matches dashboard 24h chart; SSE updates rightmost bucket | ✓ |
| Last 100 uplinks | Aligns with uplinks log default but X-axis irregular (uplink intervals vary) | |
| Last 7d, 1h-bucket (168 buckets) | More trend visible but heavier query per MP × 3 metrics × 168 buckets | |

**User's choice:** Last 24h, downsampled

### Q4 — Empty state — dashboard

| Option | Description | Selected |
|--------|-------------|----------|
| Friendly onboarding card with progressive CTA based on (gw, dev, uplink) state | Empathetic + actionable; ties to Phase 3 surfaces | ✓ |
| Skeleton placeholders + KPI tiles at 0 | Passive; user unsure what to do next | |
| Bypass: redirect / → /devices when no uplinks | Skips Phase 4 dashboard surface entirely; loses "first impression" moment | |

**User's choice:** Friendly onboarding card

### Q5 — Quality badge surface

| Option | Description | Selected |
|--------|-------------|----------|
| Badge on Normal tab + click → Uplinks log with quality filter | Operator catches codec drift in context | ✓ |
| Badge on devices list only | Fleet dashboard angle but detail page has no surface | |
| Both (devices list + detail page) | Devices list redesign is Phase 4 scope creep | |

**User's choice:** Badge on Normal tab + filter button

### Q6 — Empty state — MP detail (no binding/uplink)

| Option | Description | Selected |
|--------|-------------|----------|
| Keep tabs visible; Normal shows info + "No device bound" status row + CTA; Advanced/Uplinks log disabled | No layout shift when bind + first uplink lands | ✓ |
| Hide tabs; show info card + CTA only | Tabs pop in after first uplink — disorienting | |
| 404 or redirect to MP list | Wrong for active-but-empty MP | |

**User's choice:** Keep tabs visible

### Q7 — Raw payload display

| Option | Description | Selected |
|--------|-------------|----------|
| Hex monospace + expandable row | JetBrains Mono already in bundle; matches vendor doc + Phase 3 DevEUI sticker convention | ✓ |
| Base64 + copy button | Shorter but LoRaWAN convention is hex | |
| Truncated hex (16 bytes) + 'View full' popover | Saves space; extra click | |

**User's choice:** Hex monospace + expandable

### Q8 — Advanced tab JSON rendering

| Option | Description | Selected |
|--------|-------------|----------|
| JSON tree view (collapsible nodes) | Handles nested objects (e.g. ADW300 l1/l2/l3); custom component, no new dep | ✓ |
| Flat key=value table (`jsonb_each`) | Loses nested structure; multi-phase data flattens to noise | |
| Raw JSON in `<pre>` block + copy button | Technically correct but not "advanced view" — readable matters | |

**User's choice:** Collapsible JSON tree view

### Q9 — Viewer authorization for Phase 4

| Option | Description | Selected |
|--------|-------------|----------|
| Viewer sees dashboard + detail (all tabs) | No mutations in Phase 4; matches AUTH-06 viewer = read-only | ✓ |
| Viewer sees dashboard + Normal tab only; Advanced/Uplinks log = admin-only | Raw payload feels diagnostic; spec doesn't gate it | |

**User's choice:** Viewer full read access

---

## Claude's Discretion

- Exact `time_bucket` widths inside each preset
- KPI tile layout grid sizes / ordering
- Sparkline visual styling (Recharts area vs line, colors)
- JSON tree component depth indentation, primitive type colors
- Hex-payload column width and copy-to-clipboard placement
- Snapshot event payload field ordering / pagination cap for >1000 MPs
- SSE backpressure policy on slow clients (snapshot reissue vs disconnect)
- SSE topic naming convention (`mp:<uuid>`, `mp:<uuid>:uplinks`)
- Whether `device.last_seen_at` is sufficient for D-07 or whether a per-binding `last_uplink_at` view is needed (planner to read resolver code)

## Deferred Ideas

- Auto-detect adaptive scope (fallback if admin flag unset)
- Last-Event-ID cursor replay (revisit if snapshot becomes expensive at scale)
- Per-page SSE endpoints
- WebSocket (bidirectional)
- Saved dashboard views / per-user preferences (V2-AUTH-02)
- Site-grouped dashboard breakdown
- Quality-flag drill-down beyond badge
- User-tunable bucket widths
- Detail-page admin-only gate on Advanced/Uplinks log
- Pre-computed latest-reading-per-MP matview (start on-demand; add only if snapshot latency becomes a problem)
- Mobile native dashboard (V2-MOB-01)
- Hex / decoded_object copy-to-clipboard buttons (small polish — planner decides)
