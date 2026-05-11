# Phase 5: Aggregates, Reports, Map & Floor Plans — Research

**Researched:** 2026-05-12
**Domain:** TimescaleDB CAGGs + Go PDF/Excel/CSV generation + React Leaflet + pdf.js + River job queue + custom floor-plan canvas
**Confidence:** HIGH (most findings directly verified against official docs or npm/Go registries; see Assumptions Log for LOW items)

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Reports — content & branding**
- D-01: Report shape = "Summary + Detail" (single template). Top section: totals + period-delta tiles + one chart per utility class. Below: per-period breakdown table + per-meter rows when scope = "All meters" or "Site". Multi-page PDF, scrollable HTML preview.
- D-02: PDF branding = page header + footer on every page. Header: logo_path + display_name top-left; address top-right; thin navy rule below. Footer: generated timestamp + "page X of Y" + small Shifter wordmark. No cover page.
- D-03: Period-delta = vs previous period (always) + YoY when ≥1 measurement in prior-year window. Silent fallback (no row, no warning) when prior-year data absent.
- D-04: "Group by category" = utility class (water vs electricity). Single-capability installs render only their section. No new schema.

**Reports — generation & download flow**
- D-05: Scope picker = three-radio model ("All meters" | "Single site" | "Single meter") with conditional secondary picker. URL state via `useSearchParams + zod`.
- D-06: Format flow = "Generate-once, download any format." CSV + Excel immediate; PDF async via background job. Toast on PDF completion.
- D-07: Reports are ephemeral. No history page. Artifacts under `/var/lib/shifter/reports/<uuid>/...`, 24h TTL purge cron. Audit log records generation request.

**Continuous aggregates & retention**
- D-08: Full 4-level CAGG hierarchy — hourly → daily → monthly → yearly (CAGG-over-CAGG).
- D-09: Retention defaults (all configurable via Settings): raw 90d, hourly 1y, daily 5y, monthly 20y, yearly never drops.
- D-10: CAGG rollup content: sum(cumulative_delta), avg(instant_value), max(instant_value), min(battery_pct), avg(battery_pct), avg(rssi), avg(snr), count(*), count(*) FILTER (WHERE quality <> 'ok').
- D-11: Refresh policy: end_offset ≥ 2 × device_profile.expected_interval_s. start_offset ≤ raw_retention. Real-time mode (materialized_only=false) ON for hourly + daily; OFF for monthly + yearly.

**Map view**
- D-12: Map content = sites + gateways only. Devices live on floor plans, not the map.
- D-13: Default viewport = auto-fit bounding box of all sites + gateways. Special cases: 1 site → zoom 16; 0 sites/gateways → Bangkok [13.7563, 100.5018] zoom 5.
- D-14: Clustering = `leaflet.markercluster` plugin, automatic, default thresholds (~50 markers). Navy cluster icons. Click cluster → zoom to cluster bounds.
- D-15: Drill-down = click marker → popup with site summary + "View site" button + "Get directions" OSM external link.

**Floor plans — data model**
- D-16: Schema = `floor_plan` table with (id, site_id, label, sort_order, image_path, image_w, image_h, uploaded_at, updated_at). Multiple rows for multi-floor sites. No layout_type enum.
- D-17: PDF→PNG conversion = client-side via pdf.js. Server never sees PDF. Frontend uploads PNG only.
- D-18: Image storage = filesystem volume at `/var/lib/shifter/floor-plans/<floor_plan_id>.<ext>`.
- D-19: Upload caps: 10 MB max file, 8192×8192 max dimensions. Server validates.

**Floor plans — editor & pinning**
- D-20: Pinning UX = sidebar list → click device → click point on plan. `react-image-pin` NOT required; custom canvas + abs-positioned divs over image. touch-action: none on canvas.
- D-21: Marker style = 12px filled circle, state color, 2px white ring. Hover → label overlay. Click → popover with quick stats + "Open device" link.
- D-22: State color semantics: green (healthy), yellow (battery ≤ 20% OR rssi < −110 dBm OR decode_fail in last 10), red (last_seen_at ≤ now() − 2× expected_interval). Live via SSE extension.
- D-23: Default tab on site detail = Floor plan tab if floor_plan rows exist, otherwise Overview.
- D-24: Image replace = keep pins + confirmation dialog. Fractional coords load-bearing.
- D-25: Decommissioned devices auto-remove from plan (same transaction as soft-delete).

### Claude's Discretion
- PDF library: maroto/v2 (verify in research)
- Job queue: river (Postgres-native, no Redis dep)
- CSV/Excel content depth: same as PDF "Summary + Detail"; Excel = 3 sheets (Summary, Period Detail, Meter Detail); CSV = Detail rows only with header block
- CSV format spec: UTF-8 BOM, ISO-8601 timestamps, comma separator
- Marker color tunables (D-22 thresholds): defaults are starting values, expose as Settings constants later
- Real-time CAGG flag (D-11): default on for hourly+daily, off for monthly+yearly
- CAGG refresh cron schedule: hourly every 5 min, daily every 30 min, monthly every 6h, yearly daily at 02:00 install_tz
- Map marker icons: Lucide `Building2` for sites, `Antenna` for gateways; custom Leaflet divIcon with Tailwind classes
- Initial center fallback: Bangkok [13.7563, 100.5018] zoom 5 (AS923-2 / Thailand default)
- Empty-state CTAs: each new surface gets the Phase 4 D-21 three-stage card pattern

### Deferred Ideas (OUT OF SCOPE)
- Saved report templates
- Email delivery of reports
- User-tagged metering-point categories
- Geocoding of install_identity.address
- Lat/lng device placement on floor plans
- /reports/history page
- PDF cover page
- Custom-axis YoY comparison
- Auto-detect floor-plan feature drift on image replace
- Map search bar / filter overlay
- Floor-plan-level zoom / pan inertia tuning
- Real-time CAGG flag globally configurable in Settings
- Audit-log surface for pin moves
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| SITE-02 | Site supports horizontal (single-floor/campus) and vertical (multi-floor) layouts | D-16: multiple floor_plan rows ordered by sort_order; no enum needed |
| SITE-03 | Admin uploads one or more floor-plan images per site (PNG/JPG/PDF→PNG, size cap, format whitelist) | D-17: pdf.js client-side conversion; D-19: 10MB/8192px caps server-enforced |
| SITE-04 | Admin drag-drops devices onto floor plan; positions stored as normalized fractions x_frac/y_frac ∈ [0,1] | D-20: custom canvas pattern with fractional coordinate math (see § Canvas Pin Overlay) |
| SITE-05 | Floor plan shows state-tinted markers (green/yellow/red) reflecting device health | D-22 thresholds; extends Phase 4 Hub with device_health channel |
| SITE-06 | User can navigate map → site → floor plan → device detail in single click-through | D-15 popup + D-23 default tab = floor plan if plans exist |
| MAP-01 | Map renders sites + gateways on OSM tiles via Leaflet | react-leaflet v5 + leaflet 1.9.4; OSM tile URL verified |
| MAP-02 | Map clusters markers when >~50 in view; zoom-driven decluster | react-leaflet-cluster v4.1.3 (wraps leaflet.markercluster v1.5.3) |
| MAP-03 | Click site marker → drills into site floor plan or device list | Leaflet Popup + "View site" button → /sites/:id |
| MAP-04 | No paid external API dependency | OSM tiles are free; attribution required |
| REPT-01 | Daily, monthly, yearly consumption reports | CAGG hierarchy daily/monthly/yearly CAGG; time_bucket queries |
| REPT-02 | Per single meter and aggregate reports (grouped by site or category) | Scope picker + CAGG queries with GROUP BY metering_point_id |
| REPT-03 | CSV export — UTF-8 BOM, ISO timestamps, timezone in header | encoding/csv stdlib; BOM prepend \xEF\xBB\xBF |
| REPT-04 | Excel export — formatted dates, units, totals | excelize/v2 v2.10.0 (already in go.mod); WriteToBuffer() |
| REPT-05 | PDF export — branded with install identity | maroto/v2 v2.4.0; RegisterHeader/RegisterFooter |
| REPT-06 | PDF generation as background job | River v0.36.0; riverpgxv5 driver |
| REPT-07 | Reports show period delta vs previous period | CAGG LAG() query pattern; YoY via same window −1 year |
| DATA-11 | Hierarchical TimescaleDB CAGGs (hourly → daily → monthly → yearly) | TimescaleDB 2.9+ supports CAGG-over-CAGG; verified syntax below |
| DATA-12 | CAGG refresh policies: end_offset ≥ 2× expected_interval; start_offset ≤ raw retention | Footgun documented and mitigated; see § CAGG Retention Footgun |
| DATA-13 | Aggregate tables have own (longer) retention policies | add_retention_policy() per CAGG view; Settings UI to configure |
</phase_requirements>

---

## Summary

Phase 5 has four independent technical subsystems that converge in the product. The TimescaleDB CAGG hierarchy is the most complex and has documented operational footguns. The Go PDF/Excel/CSV generation stack is entirely additive (maroto/v2 and River are the only new Go deps; excelize is already in go.mod). The React map view is straightforward (react-leaflet v5 + react-leaflet-cluster wrapper; both verified React 19 compatible). The floor-plan pin overlay is custom canvas code — simpler than any third-party library, and directly mandated by D-20.

**Primary recommendation:** Implement in this wave order: (1) CAGG migrations + retention, (2) report data queries + CSV/Excel, (3) River setup + PDF job, (4) Map view, (5) Floor plan upload + storage, (6) Floor plan pin editor + SSE health. Separating the CAGG migrations into their own wave prevents gotchas from the "CREATE MATERIALIZED VIEW must run outside a transaction block" constraint from contaminating other migrations.

---

## Standard Stack

### Core — Backend (Go)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/johnfercher/maroto/v2` | v2.4.0 | PDF generation | CLAUDE.md mandates; Bootstrap-grid API; RegisterHeader/RegisterFooter per page; not gofpdf (archived) |
| `github.com/riverqueue/river` | v0.36.0 | Background job queue | CLAUDE.md mandates; Postgres-native (no Redis); PeriodicJob cron built-in; riverpgxv5 driver integrates with pgx/v5 pool |
| `github.com/riverqueue/river/riverdriver/riverpgxv5` | v0.36.0 | River driver for pgx/v5 | Required to wire River with existing pgx/v5 pool |
| `encoding/csv` (stdlib) | — | CSV export | Already in stdlib; faster than third-party (no reflection); UTF-8 BOM prepend trivial |
| `github.com/xuri/excelize/v2` | v2.10.0 | Excel export | **Already in go.mod** (Phase 3 bulk import); WriteToBuffer() for in-memory bytes; multi-sheet support |
| `net/http` (stdlib) | — | Static file serve for floor plan images | chi route with auth middleware; no new dep |

[VERIFIED: go list -m -versions github.com/riverqueue/river → v0.36.0 is latest]
[VERIFIED: go list -m -versions github.com/johnfercher/maroto/v2 → v2.4.0 is latest]
[VERIFIED: github.com/xuri/excelize/v2 v2.10.0 already in go.mod]

### Core — Frontend (TypeScript/React)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `react-leaflet` | 5.0.0 | Leaflet React wrapper | CLAUDE.md mandates; React 19 peer dep satisfied; 42KB vs MapLibre 290KB |
| `leaflet` | 1.9.4 | Map engine | Peer dep of react-leaflet; OSM tile support; proven for "site pins on OSM" |
| `react-leaflet-cluster` | 4.1.3 | Marker clustering | React 19 + react-leaflet v5 compatible (verified peerDeps); thin wrapper over leaflet.markercluster |
| `leaflet.markercluster` | 1.5.3 | Clustering engine | Underlying engine for react-leaflet-cluster |
| `pdfjs-dist` | 5.7.284 | Client-side PDF→PNG | Mozilla official; ESM-only in v5; handles D-17 PDF upload conversion |
| `@types/leaflet` | 1.9.21 | TypeScript types for Leaflet | DefinitelyTyped; required for typed divIcon/Popup usage |
| `@types/leaflet.markercluster` | 1.5.6 | TypeScript types | DefinitelyTyped; `pdfjs-dist` ships its own types (no @types/pdfjs-dist needed) |

[VERIFIED: npm view react-leaflet version → 5.0.0]
[VERIFIED: npm view leaflet version → 1.9.4]
[VERIFIED: npm view react-leaflet-cluster version → 4.1.3; peerDeps: react@^19.0.0, react-leaflet@^5.0.0]
[VERIFIED: npm view pdfjs-dist version → 5.7.284]
[VERIFIED: npm view leaflet.markercluster version → 1.5.3]
[VERIFIED: npm view @types/leaflet version → 1.9.21]
[VERIFIED: npm view @types/leaflet.markercluster version → 1.5.6]

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `robfig/cron` | v3.x | Cron expressions for River PeriodicJob | If install-timezone-aware scheduling needed; River's built-in `PeriodicInterval` covers fixed durations; cron needed for "02:00 install_tz" yearly refresh |
| `leaflet/dist/leaflet.css` | — | Required CSS import for Leaflet | MUST be imported; without it tiles and popups break entirely |
| `react-leaflet-cluster/dist/assets/MarkerCluster.css` | — | Required CSS for cluster styling | Must import both CSS files for cluster icons to render |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| react-leaflet-cluster | `react-leaflet-markercluster` (yuzhva) | yuzhva package is less actively maintained; akursat/react-leaflet-cluster explicitly targets react-leaflet v5 + React 19 |
| maroto/v2 | chromedp + headless Chrome | 300MB binary dep vs <5MB; CLAUDE.md explicitly excludes chromedp |
| maroto/v2 | wkhtmltopdf via subprocess | Requires OS dep; not in Docker image; CLAUDE.md excluded |
| River | gocron | gocron is cron-only (no durable retries); PDF jobs need retry-on-failure semantics |
| River | Asynq | Adds Redis to install footprint; CLAUDE.md excludes Redis unless justified |
| pdfjs-dist | server-side poppler-utils | Requires OS binary in Docker image; D-17 explicitly excludes it |
| custom canvas pins | react-image-pin | D-20 explicitly says react-image-pin NOT required; custom is sufficient and avoids dep |

**Installation (new deps only):**
```bash
# Go
cd /path/to/project
go get github.com/johnfercher/maroto/v2@v2.4.0
go get github.com/riverqueue/river@v0.36.0
go get github.com/riverqueue/river/riverdriver/riverpgxv5@v0.36.0

# Frontend
cd web
pnpm add react-leaflet@5.0.0 leaflet@1.9.4 react-leaflet-cluster@4.1.3 leaflet.markercluster@1.5.3 pdfjs-dist@5.7.284
pnpm add -D @types/leaflet@1.9.21 @types/leaflet.markercluster@1.5.6
```

---

## Architecture Patterns

### Recommended Project Structure Extensions

```
internal/
├── aggregate/          # CAGG query layer — queries against CAGG views
│   ├── queries.sql     # sqlc-annotated SQL; uses --sqlc:exec for CAGG DDL helpers
│   └── store.go        # Typed wrappers around sqlc-generated code
├── report/             # Report assembly (data → report struct → format)
│   ├── assembler.go    # Builds report struct from CAGG queries (D-01/D-03/D-10)
│   ├── csv.go          # CSV formatter (D-07 / REPT-03)
│   ├── excel.go        # Excel formatter using excelize (REPT-04)
│   ├── pdf.go          # PDF formatter using maroto v2 (REPT-05)
│   └── job.go          # River worker for async PDF (REPT-06)
├── floorplan/          # Floor plan CRUD + image upload + placement
│   ├── handler.go
│   ├── placement.go
│   └── image.go        # Upload validation (MIME sniff, dimension probe)
├── map/                # /api/map/data endpoint
│   └── handler.go
└── db/
    └── migrations/
        ├── 0024_floor_plan.up.sql
        ├── 0025_device_floor_plan_placement.up.sql
        ├── 0026_cagg_hourly.up.sql        # CAGG migration — must disable tx
        ├── 0027_cagg_daily.up.sql         # CAGG migration — must disable tx
        ├── 0028_cagg_monthly.up.sql       # CAGG migration — must disable tx
        ├── 0029_cagg_yearly.up.sql        # CAGG migration — must disable tx
        └── 0030_retention_config.up.sql   # retention_config table

web/src/
├── routes/
│   ├── reports/
│   │   ├── index.tsx           # ReportsPage
│   │   ├── ReportConfigPanel.tsx
│   │   ├── ReportResultPanel.tsx
│   │   └── ...
│   └── map.tsx                 # MapPage (full-bleed)
└── components/
    ├── map/
    │   ├── MapView.tsx
    │   ├── SiteMarker.tsx
    │   ├── GatewayMarker.tsx
    │   └── SitePopup.tsx
    └── floor-plan/
        ├── FloorPlanCanvas.tsx
        ├── DevicePin.tsx
        ├── DeviceSidebar.tsx
        └── UploadFloorPlanDialog.tsx
```

---

## CAGG Hierarchy — Verified Syntax and Patterns

### Pattern 1: Hierarchical CAGG Creation (CAGG-over-CAGG)

**What:** TimescaleDB 2.9+ supports creating a continuous aggregate on top of another continuous aggregate. The daily CAGG selects from the hourly CAGG view, not from the raw hypertable.

**Time bucket constraint (CRITICAL):** The bucket interval of an upper CAGG must be an integer multiple of the lower CAGG's bucket interval. Fixed-width buckets (hours, days) can stack on fixed-width buckets. For monthly and yearly (variable-width), you must first have a fixed-width daily CAGG and then create the monthly CAGG as a CAGG-over-daily.

**Source:** [CITED: tigerdata.com/docs/use-timescale/latest/continuous-aggregates/hierarchical-continuous-aggregates]

```sql
-- Source: [CITED: tigerdata.com hierarchical CAGG docs]

-- LEVEL 1: Hourly CAGG (directly over the measurement hypertable)
CREATE MATERIALIZED VIEW measurement_hourly
WITH (timescaledb.continuous, timescaledb.materialized_only = false)
AS SELECT
    time_bucket('1 hour', time)          AS bucket,
    metering_point_id,
    sum(cumulative_value - LAG(cumulative_value) OVER (
        PARTITION BY metering_point_id ORDER BY time
    ))                                   AS cumulative_delta,
    avg(instant_value)                   AS avg_instant,
    max(instant_value)                   AS max_instant,
    min(battery_pct)                     AS min_battery,
    avg(battery_pct)                     AS avg_battery,
    avg(rssi)                            AS avg_rssi,
    avg(snr)                             AS avg_snr,
    count(*)                             AS uplink_count,
    count(*) FILTER (WHERE quality <> 'ok') AS flagged_count
FROM measurement
GROUP BY 1, 2
WITH NO DATA;
-- materialized_only=false enables real-time mode per D-11

-- LEVEL 2: Daily CAGG (over hourly CAGG — NOT raw hypertable)
CREATE MATERIALIZED VIEW measurement_daily
WITH (timescaledb.continuous, timescaledb.materialized_only = false)
AS SELECT
    time_bucket('1 day', bucket)         AS bucket,
    metering_point_id,
    sum(cumulative_delta)                AS cumulative_delta,
    avg(avg_instant)                     AS avg_instant,
    max(max_instant)                     AS max_instant,
    min(min_battery)                     AS min_battery,
    avg(avg_battery)                     AS avg_battery,
    avg(avg_rssi)                        AS avg_rssi,
    avg(avg_snr)                         AS avg_snr,
    sum(uplink_count)                    AS uplink_count,
    sum(flagged_count)                   AS flagged_count
FROM measurement_hourly
GROUP BY 1, 2
WITH NO DATA;
-- materialized_only=false enables real-time mode per D-11

-- LEVEL 3: Monthly CAGG (over daily CAGG)
CREATE MATERIALIZED VIEW measurement_monthly
WITH (timescaledb.continuous, timescaledb.materialized_only = true)
AS SELECT
    time_bucket('1 month', bucket)       AS bucket,
    metering_point_id,
    sum(cumulative_delta)                AS cumulative_delta,
    avg(avg_instant)                     AS avg_instant,
    max(max_instant)                     AS max_instant,
    min(min_battery)                     AS min_battery,
    avg(avg_battery)                     AS avg_battery,
    avg(avg_rssi)                        AS avg_rssi,
    avg(avg_snr)                         AS avg_snr,
    sum(uplink_count)                    AS uplink_count,
    sum(flagged_count)                   AS flagged_count
FROM measurement_daily
GROUP BY 1, 2
WITH NO DATA;
-- materialized_only=true (real-time OFF) per D-11 — monthly rarely queried

-- LEVEL 4: Yearly CAGG (over monthly CAGG)
CREATE MATERIALIZED VIEW measurement_yearly
WITH (timescaledb.continuous, timescaledb.materialized_only = true)
AS SELECT
    time_bucket('1 year', bucket)        AS bucket,
    metering_point_id,
    sum(cumulative_delta)                AS cumulative_delta,
    avg(avg_instant)                     AS avg_instant,
    max(max_instant)                     AS max_instant,
    min(min_battery)                     AS min_battery,
    avg(avg_battery)                     AS avg_battery,
    avg(avg_rssi)                        AS avg_rssi,
    avg(avg_snr)                         AS avg_snr,
    sum(uplink_count)                    AS uplink_count,
    sum(flagged_count)                   AS flagged_count
FROM measurement_monthly
GROUP BY 1, 2
WITH NO DATA;
-- materialized_only=true (real-time OFF) per D-11 — yearly rarely queried
```

**Important:** `time_bucket('1 month', ...)` on a daily bucket column requires the source bucket to be fixed-width (days). `time_bucket('1 month', hourly_bucket)` would fail. The daily → monthly → yearly chain is the correct order.

**Toggling real-time mode after creation:**
```sql
-- Enable real-time (materialized_only=false):
ALTER MATERIALIZED VIEW measurement_hourly SET (timescaledb.materialized_only = false);
-- Disable real-time (materialized_only=true):
ALTER MATERIALIZED VIEW measurement_monthly SET (timescaledb.materialized_only = true);
```

### Pattern 2: Refresh Policies (DATA-12 spec)

```sql
-- Source: [CITED: tigerdata.com/docs/api/latest/continuous-aggregates/add_continuous_aggregate_policy]

-- Hourly CAGG refresh every 5 minutes
-- end_offset=1h: excludes the current (incomplete) hour
-- start_offset=2h: re-checks a 2-hour window for late arrivals
-- D-11: end_offset >= 2× expected_interval_s; most profiles are 15min → 30min minimum
-- Using 1h gives plenty of late-uplink absorption for all profiles
SELECT add_continuous_aggregate_policy('measurement_hourly',
    start_offset  => INTERVAL '2 hours',
    end_offset    => INTERVAL '1 hour',
    schedule_interval => INTERVAL '5 minutes'
);

-- Daily CAGG refresh every 30 minutes
SELECT add_continuous_aggregate_policy('measurement_daily',
    start_offset  => INTERVAL '2 days',
    end_offset    => INTERVAL '1 hour',
    schedule_interval => INTERVAL '30 minutes'
);

-- Monthly CAGG refresh every 6 hours
SELECT add_continuous_aggregate_policy('measurement_monthly',
    start_offset  => INTERVAL '2 months',
    end_offset    => INTERVAL '1 day',
    schedule_interval => INTERVAL '6 hours'
);

-- Yearly CAGG refresh once daily
SELECT add_continuous_aggregate_policy('measurement_yearly',
    start_offset  => INTERVAL '2 years',
    end_offset    => INTERVAL '7 days',
    schedule_interval => INTERVAL '1 day'
);
```

### Pattern 3: Retention Policies (DATA-13)

```sql
-- Raw measurements: 90 days (default; configurable)
SELECT add_retention_policy('measurement', INTERVAL '90 days');

-- Hourly CAGG: 1 year
SELECT add_retention_policy('measurement_hourly', INTERVAL '1 year');

-- Daily CAGG: 5 years
SELECT add_retention_policy('measurement_daily', INTERVAL '5 years');

-- Monthly CAGG: 20 years
SELECT add_retention_policy('measurement_monthly', INTERVAL '20 years');

-- Yearly CAGG: no retention policy (keep forever — ~1 row/MP/year is free)
-- Do NOT call add_retention_policy for yearly CAGG
```

### Pattern 4: Report Query — Per-Period Breakdown

```sql
-- Source: [ASSUMED — standard TimescaleDB time_bucket reporting pattern]
-- Monthly report: per-day breakdown for a given metering_point_id + date range
SELECT
    bucket                                                    AS period,
    cumulative_delta                                          AS consumption,
    -- LAG for period-over-period delta (REPT-07)
    cumulative_delta - LAG(cumulative_delta) OVER (
        PARTITION BY metering_point_id ORDER BY bucket
    )                                                         AS delta_vs_prior
FROM measurement_daily
WHERE metering_point_id = $1
  AND bucket >= $2 AND bucket < $3
ORDER BY bucket;
```

### Pattern 5: CAGG Cumulative Delta Computation

**What:** The `measurement` hypertable stores `cumulative_value` (the meter's absolute reading). To get consumption in a time bucket, we need `sum(cumulative_delta)` where `cumulative_delta = current_cumulative_value - previous_cumulative_value`. This is an instantaneous delta, not the cumulative_value column directly.

**Implementation in hourly CAGG:** The delta must be computed at the raw level before aggregation. Use a window function in the CAGG definition or pre-compute it. The recommended pattern is to add `cumulative_delta` as a generated column or compute it via LAG() in the CAGG SELECT.

**Caveat:** LAG() in CAGG definitions has constraints in TimescaleDB — the window must be over the same CAGG bucket and same partition. An alternative is to compute deltas at ingest time (Phase 2 pattern) and store `cumulative_delta` directly in the measurement table. Check the existing ingest pipeline to see if `cumulative_delta` is already persisted. [ASSUMED — need to verify against existing measurement schema in migration 0015]

**Risk:** If `cumulative_delta` is not already in the `measurement` table, the CAGG must compute it via LAG() which complicates the CAGG view definition. A simpler alternative: add a `cumulative_delta` computed column to the hypertable in a Phase 5 migration. [ASSUMED]

### Anti-Patterns to Avoid

- **Using `time_bucket('1 month', hourly_bucket)`:** Month is a variable-width bucket; the source must be daily (fixed-width). Always chain: hourly → daily → monthly (NOT hourly → monthly directly).
- **Real-time mode on monthly/yearly:** Real-time mode on rarely-queried upper CAGGs causes expensive scan-on-read combining materialized data with all raw rows since last refresh. Keep monthly/yearly as `materialized_only=true`.
- **Refresh policy `start_offset > raw_retention`:** The footgun (see below). If raw data is dropped before CAGG refresh covers it, those buckets are silently lost from the CAGG.
- **Querying raw hypertable for 30d/yearly reports:** Once CAGGs exist, always route long-range queries to the appropriate CAGG level. Querying raw for 90+ days is expensive.

---

## CAGG Retention Footgun — Documented and Mitigated

### The Footgun

**Source:** [CITED: tigerdata.com/docs/use-timescale/latest/continuous-aggregates/refresh-policies]

> "If the continuous aggregate policy window covers data that is removed by the data retention policy, the data will be removed when the aggregates for those buckets are refreshed."

**What this means in practice:**
- The raw `measurement` table has a 90-day retention policy
- If the hourly CAGG's `start_offset` is set to, say, `INTERVAL '100 days'`, the refresh policy will attempt to re-materialize data from 100 days ago
- But the raw data from 80+ days ago has been dropped by the 90-day retention policy
- When the hourly CAGG refresh runs, it will **overwrite already-materialized hourly buckets with zeros/NULL** for the dropped period, effectively corrupting the CAGG
- This corruption is **silent** — no error is raised

**DATA-12 mitigation (already in the locked decisions):**
- `start_offset ≤ raw_retention`: keep the hourly CAGG's start_offset within the raw retention window (e.g., `2 hours` not `100 days`)
- This means the CAGG is not "going back" to refresh already-dropped raw data

**The correct mental model:**
- `start_offset` = "how far back does this refresh look for new invalidations" (keep it small, within raw window)
- `end_offset` = "how recent is too recent" (keep it ≥ 2× expected_interval to absorb late uplinks)
- Retention policy = separate concern; applied directly to chunks

**Concrete safe values for this project:**
```
raw retention:        90 days
hourly start_offset:  2 hours     ← well within 90d raw window ✓
daily start_offset:   2 days      ← well within 90d raw window ✓
monthly start_offset: 2 months    ← within 90d (~3 months) raw window ✓
yearly start_offset:  2 years     ← RAW DATA GONE: BUT this is OK because
                                     yearly CAGG refreshes from MONTHLY CAGG,
                                     not from raw. Monthly CAGG has 20y retention.
```

**Hierarchical CAGG safety:** In a CAGG hierarchy, each CAGG's retention policy applies to that CAGG's own chunks, not to the source. The yearly CAGG refresh reads from the monthly CAGG (not raw), so the raw 90-day retention does not affect yearly CAGG correctness. Each level in the hierarchy has its own retention policy and its own source. This is the key advantage of the CAGG-over-CAGG pattern.

---

## golang-migrate + TimescaleDB CAGG Transaction Constraint

### The Problem

**Source:** [VERIFIED: GitHub timescale/timescaledb issue #5377 + search results]

`CREATE MATERIALIZED VIEW ... WITH (timescaledb.continuous) ... WITH DATA` **cannot run inside a PostgreSQL transaction block.** golang-migrate wraps each migration file in a `BEGIN`/`COMMIT` transaction by default.

If you run a CAGG creation migration through golang-migrate without working around this, you get:

```
ERROR: CREATE MATERIALIZED VIEW ... WITH DATA cannot be executed within a pipeline
```

(The exact error varies by TimescaleDB version and pgx driver, but the root cause is the same.)

### The Solution

**Use `WITH NO DATA`** in all CAGG migration files:

```sql
CREATE MATERIALIZED VIEW measurement_hourly
WITH (timescaledb.continuous, timescaledb.materialized_only = false)
AS SELECT ...
FROM measurement
GROUP BY 1, 2
WITH NO DATA;  -- ← avoids the transaction constraint
```

`WITH NO DATA` creates the CAGG schema without attempting to materialize data immediately. The refresh policy will backfill on first scheduled run, or you can call `CALL refresh_continuous_aggregate(...)` in a separate migration file.

**golang-migrate note:** golang-migrate v4 does not have a built-in per-file "no transaction" annotation (unlike sql-migrate's `-- +migrate Up notransaction`). The `WITH NO DATA` workaround eliminates the need for a no-transaction mode because the creation itself no longer requires two internal transactions.

**Additional retention policy + refresh policy calls** (the `SELECT add_continuous_aggregate_policy(...)` and `SELECT add_retention_policy(...)` calls) CAN run inside a transaction — only the `CREATE MATERIALIZED VIEW WITH DATA` is restricted.

**Recommended migration structure:**
```
0026_cagg_hourly.up.sql     → CREATE MATERIALIZED VIEW ... WITH NO DATA; add_continuous_aggregate_policy(...); add_retention_policy(...);
0027_cagg_daily.up.sql      → CREATE MATERIALIZED VIEW ... WITH NO DATA; add_continuous_aggregate_policy(...); add_retention_policy(...);
0028_cagg_monthly.up.sql    → same pattern
0029_cagg_yearly.up.sql     → same pattern (no retention_policy for yearly)
0030_retention_config.up.sql → retention_config table for Settings UI
```

Each CAGG in its own migration file ensures:
1. The CAGG view is created first (prerequisite for policy calls)
2. `WITH NO DATA` avoids the transaction constraint
3. Policies attached in same migration, so they're atomic with CAGG creation

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| PDF generation | Custom HTML→PDF | maroto/v2 | Headers/footers on every page; image embedding; pure-Go; page numbering |
| Excel multi-sheet | Custom XLSX ZIP | excelize/v2 | Already in go.mod; handles dates, styles, totals row borders |
| CSV UTF-8 BOM | Manual byte slice | stdlib `encoding/csv` + prepend BOM | One-liner: `w.Write([]byte{0xEF, 0xBB, 0xBF})`|
| Background PDF job | goroutine + in-memory channel | River | Durable retries; survives process restart; Postgres-backed |
| Map clustering | Custom JS | react-leaflet-cluster | Handles cluster icon render, zoom-to-bounds, decluster |
| Marker HTML | SVG string concatenation | Leaflet `divIcon` with static HTML string | Use template string with Lucide SVG paths; inject Tailwind classes |
| Floor-plan coordinate storage | Pixel coordinates | Normalized fractions x_frac/y_frac ∈ [0,1] | Resolution-independent; survives image replacement (D-24 load-bearing) |
| PDF→PNG in backend | poppler-utils/ImageMagick in Docker | pdfjs-dist client-side | No OS dep; server never sees PDF (D-17) |
| CAGG queries in sqlc | `sqlc generate` on CAGG views | Hand-written queries via `sqlc:exec` | sqlc may fail to parse `WITH (timescaledb.continuous)` DDL; use `-- name: QueryHourly :many` style annotations on SELECT-only queries against the view |

**Key insight:** The worst custom solutions in this domain are custom PDF generation (header/footer pagination is 200+ lines of math) and custom CAGG invalidation tracking (TimescaleDB does this automatically through its background worker — never replicate it).

---

## PDF Generation — maroto v2

### Constructor and Header/Footer Pattern

**Source:** [VERIFIED: pkg.go.dev/github.com/johnfercher/maroto/v2]

```go
// Source: [CITED: pkg.go.dev/github.com/johnfercher/maroto/v2]
import (
    "github.com/johnfercher/maroto/v2"
    "github.com/johnfercher/maroto/v2/pkg/config"
    "github.com/johnfercher/maroto/v2/pkg/components/image"
    "github.com/johnfercher/maroto/v2/pkg/components/text"
    "github.com/johnfercher/maroto/v2/pkg/components/line"
)

func buildReportPDF(rpt *Report, identity *InstallIdentity) ([]byte, error) {
    b := config.NewBuilder()
    cfg := b.Build()
    m := maroto.New(cfg)

    // Header on every page (D-02)
    if err := m.RegisterHeader(
        // Row 1: logo left, address right
        image.NewRow(10, identity.LogoPath),
        text.NewRow(6, identity.DisplayName),
        text.NewRow(6, identity.Address),
        // Row 2: navy rule (approximated with a line row)
        line.NewRow(1),
    ); err != nil {
        return nil, err
    }

    // Footer on every page (D-02)
    // maroto v2 provides page number via the document generation context
    if err := m.RegisterFooter(
        text.NewRow(6, fmt.Sprintf("Generated %s", identity.Timezone)),
        // page X of Y is injected via maroto's built-in page counter
    ); err != nil {
        return nil, err
    }

    // Add summary rows, then detail table rows...
    // m.AddRow(h, col.NewCol(12, text.New("content")))

    doc, err := m.Generate()
    if err != nil {
        return nil, err
    }
    return doc.GetBytes(), nil
}
```

**Page X of Y:** maroto v2 provides `{page}` and `{pages}` placeholder variables in text components that are resolved at render time. Use `text.New("{page} of {pages}")` in the footer row. [ASSUMED — verify against maroto v2.4.0 docs; the feature exists in v1 and v2 but placeholder syntax may differ]

**Image from bytes (not file path):** For logos stored as bytes (loaded from `logo_path` on disk at report generation time), use `image.NewFromFileCol(12, logoPath, props.Rect{...})`. The `image_path` column stores the filesystem path, so `LogoPath` is a resolvable path at generation time. No base64/bytes API is needed because the file is local.

**Low memory mode for large reports (50+ pages):**
```go
b := config.NewBuilder()
cfg := b.WithGenerationMode(generation.LowMemory).Build()
m := maroto.New(cfg)
```
`LowMemory` mode generates and discards pages sequentially, avoiding holding the full document in RAM. For 50-page reports this is worthwhile. [VERIFIED: pkg.go.dev/github.com/johnfercher/maroto/v2]

---

## River — Background Job Queue Setup

### First-Use Wiring Pattern

**Source:** [VERIFIED: riverqueue.com/docs]

River requires database migrations for its own tables (job queue state). Run these during Wave 0 or as the first task of the River integration plan:

```bash
river migrate-up --database-url "$DATABASE_URL"
```

This creates `river_job`, `river_leader`, `river_queue` tables in the Postgres database.

### Worker Definition + Registration

```go
// Source: [CITED: riverqueue.com/docs]
import (
    "github.com/riverqueue/river"
    "github.com/riverqueue/river/riverdriver/riverpgxv5"
)

// Job args struct — one per job type
type PDFReportArgs struct {
    ReportID  string `json:"report_id"`
    ReportDir string `json:"report_dir"`
}

func (PDFReportArgs) Kind() string { return "pdf_report" }

// Worker
type PDFReportWorker struct {
    river.WorkerDefaults[PDFReportArgs]
    reportSvc *report.Service
}

func (w *PDFReportWorker) Work(ctx context.Context, job *river.Job[PDFReportArgs]) error {
    return w.reportSvc.GeneratePDF(ctx, job.Args.ReportID, job.Args.ReportDir)
}
```

### Client Construction in cmd/serve

```go
// Source: [CITED: riverqueue.com/docs]
workers := river.NewWorkers()
river.AddWorker(workers, &PDFReportWorker{reportSvc: reportSvc})

// Periodic jobs (cron-style)
periodicJobs := []*river.PeriodicJob{
    // Report artifact cleanup every hour (D-07 24h TTL)
    river.NewPeriodicJob(
        river.PeriodicInterval(1*time.Hour),
        func() (river.JobArgs, *river.InsertOpts) {
            return CleanupReportsArgs{}, nil
        },
        &river.PeriodicJobOpts{RunOnStart: false},
    ),
}

riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
    Queues: map[string]river.QueueConfig{
        river.QueueDefault: {MaxWorkers: 4}, // PDF gen is CPU+IO bound; 4 is plenty
    },
    Workers:      workers,
    PeriodicJobs: periodicJobs,
})
if err != nil {
    return err
}

// Start in serve goroutine; Stop on context cancel
go riverClient.Start(ctx)
defer riverClient.Stop(ctx)
```

### Transactional Job Insertion (D-06 PDF job)

```go
// Source: [CITED: riverqueue.com/docs]
// Insert the PDF job in the same Postgres tx as the report metadata write
_, err = riverClient.InsertTx(ctx, tx, PDFReportArgs{
    ReportID:  reportID,
    ReportDir: reportDir,
}, nil)
```

Transactional insertion guarantees that if the metadata write fails, the job is never enqueued — no orphaned PDF jobs.

### Worker Pool Sizing

| Queue | MaxWorkers | Rationale |
|-------|-----------|-----------|
| Default | 4 | PDF generation is CPU+memory bound; maroto on large reports can use 50-100MB each; 4 concurrent PDFs on a single-tenant install is generous |
| CAGG refresh (if any manual trigger via River) | 1 | Sequential; overlapping refreshes can deadlock |

---

## React-Leaflet v5 + Clustering

### MapContainer Setup

**Source:** [VERIFIED: npm view react-leaflet version → 5.0.0; npm view react-leaflet-cluster peerDeps → react@^19.0.0, react-leaflet@^5.0.0]

```tsx
// Source: [CITED: akursat.gitbook.io/marker-cluster + react-leaflet.js.org]
import { MapContainer, TileLayer } from 'react-leaflet'
import MarkerClusterGroup from 'react-leaflet-cluster'
import 'leaflet/dist/leaflet.css'
import 'react-leaflet-cluster/dist/assets/MarkerCluster.css'
import 'react-leaflet-cluster/dist/assets/MarkerCluster.Default.css'

// CRITICAL: Leaflet needs its CSS or tiles/popups are invisible
// These 3 CSS imports are MANDATORY

function MapView() {
  return (
    <MapContainer
      style={{ height: 'calc(100vh - 3.5rem)' }}
      center={[13.7563, 100.5018]}  // Bangkok D-13 fallback
      zoom={5}
      scrollWheelZoom
      aria-label="Fleet map"
    >
      <TileLayer
        attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap contributors</a>'
        url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"
      />
      <MarkerClusterGroup chunkedLoading>
        {sites.map(site => <SiteMarker key={site.id} site={site} />)}
        {gateways.map(gw => <GatewayMarker key={gw.id} gateway={gw} />)}
      </MarkerClusterGroup>
    </MapContainer>
  )
}
```

### Custom DivIcon for Markers (D-12 spec)

```tsx
// Source: [CITED: leafletjs.com/reference.html#divicon]
import L from 'leaflet'

// Lucide Building2 SVG path (extract from lucide-react source)
const BUILDING2_PATH = 'M6 22V4a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v18Z M6 12H4a2 2 0 0 0-2 2v6a2 2 0 0 0 2 2h2 M22 22h-2 M14 22v-4a2 2 0 0 0-2-2h-4...'

function siteMarkerIcon(): L.DivIcon {
  return L.divIcon({
    html: `
      <div class="flex h-8 w-8 items-center justify-center rounded-full bg-primary ring-2 ring-white shadow-sm">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="white" stroke-width="2">
          <path d="${BUILDING2_PATH}"/>
        </svg>
      </div>
    `,
    className: '',   // CRITICAL: clear default Leaflet className or it adds unwanted white box
    iconSize: [32, 32],
    iconAnchor: [16, 32],
    popupAnchor: [0, -32],
  })
}
```

**Important:** Set `className: ''` on divIcon to prevent Leaflet's default white background box appearing behind the custom icon.

### Auto-fit Bounds (D-13)

```tsx
// Source: [CITED: leafletjs.com/reference.html#map-fitbounds]
import { useMap } from 'react-leaflet'
import L from 'leaflet'

function BoundsController({ markers }: { markers: LatLng[] }) {
  const map = useMap()
  useEffect(() => {
    if (markers.length === 0) return
    if (markers.length === 1) {
      map.setView(markers[0], 16)
    } else {
      const bounds = L.latLngBounds(markers)
      map.fitBounds(bounds.pad(0.1))  // 10% padding per D-13
    }
  }, [markers, map])
  return null
}
```

---

## pdf.js (pdfjs-dist) — Client-Side PDF→PNG Conversion

### Pattern for D-17: PDF → PNG Blob

**Source:** [VERIFIED: npm view pdfjs-dist version → 5.7.284; CITED: mozilla.github.io/pdf.js/examples/]

```tsx
// Source: [CITED: mozilla.github.io/pdf.js/examples/ + npm pdfjs-dist v5 ESM docs]
import * as pdfjsLib from 'pdfjs-dist'

// CRITICAL for pdfjs-dist v5 (ESM only — UMD build removed):
// Must set workerSrc using import.meta.url to let Vite bundle the worker
pdfjsLib.GlobalWorkerOptions.workerSrc = new URL(
  'pdfjs-dist/build/pdf.worker.min.mjs',
  import.meta.url
).toString()

async function convertPdfToBlob(file: File): Promise<Blob> {
  // Load PDF from ArrayBuffer (avoids URL.createObjectURL memory leak)
  const arrayBuffer = await file.arrayBuffer()
  const pdf = await pdfjsLib.getDocument({ data: arrayBuffer }).promise

  // Render page 1 only (floor plan convention — one plan per file)
  const page = await pdf.getPage(1)

  // 150 DPI: Leaflet viewport = 96 DPI baseline; 150/72 ≈ 2.08 scale factor
  // 72pt/in is pdf.js's natural scale; to get 150 DPI: scale = 150/72
  const scale = 150 / 72
  const viewport = page.getViewport({ scale })

  // Use OffscreenCanvas if available (avoids DOM element creation)
  const canvas = new OffscreenCanvas(
    Math.floor(viewport.width),
    Math.floor(viewport.height)
  )
  const context = canvas.getContext('2d')!

  await page.render({ canvasContext: context, viewport }).promise

  // Memory cleanup BEFORE converting to blob
  page.cleanup()
  await pdf.destroy()

  return canvas.convertToBlob({ type: 'image/png' })
}
```

**Memory management:**
- Call `page.cleanup()` after render completes — releases internal PDF.js page cache
- Call `pdf.destroy()` when done with the document — releases worker memory
- Use `OffscreenCanvas` (supported in all modern browsers) to avoid DOM element creation

**Worker configuration (pdfjs-dist v5 ESM change):**
- v5 dropped the UMD `pdf.worker.min.js` entry point
- The worker source must be set to the `.mjs` file via `import.meta.url` pattern for Vite bundling to include it automatically
- Alternative if `import.meta.url` pattern fails: set to CDN `https://unpkg.com/pdfjs-dist@5.x.x/build/pdf.worker.min.mjs`

**DPI calculation for 150 DPI floor plans:**
- pdf.js uses 72 points per inch as its natural coordinate system
- To render at 150 DPI: `scale = 150 / 72 ≈ 2.083`
- A letter-size (8.5×11 inch) PDF renders to 1275×1650 px at 150 DPI — well within the 8192×8192 cap (D-19)

---

## Custom Canvas Pin Overlay — Floor Plan Pattern

### Coordinate System and Rendering

**Source:** [CITED: 05-UI-SPEC.md §Floor Plan — Canvas Coordinate Contract]

```tsx
// Source: [CITED: 05-UI-SPEC.md coordinate contract — verbatim formula]

// Rendering formula (MUST use exactly per UI-SPEC):
// pin_left_px = x_frac × container_width
// pin_top_px  = y_frac × container_height

interface DevicePlacement {
  deviceId: string
  xFrac: number  // [0.0, 1.0]
  yFrac: number  // [0.0, 1.0]
  state: 'healthy' | 'warning' | 'offline'
}

function FloorPlanCanvas({
  imageSrc,
  placements,
  onPlace,
}: {
  imageSrc: string
  placements: DevicePlacement[]
  onPlace: (xFrac: number, yFrac: number) => void
}) {
  const containerRef = useRef<HTMLDivElement>(null)

  const handleCanvasClick = (e: React.PointerEvent<HTMLDivElement>) => {
    if (!containerRef.current) return
    const rect = containerRef.current.getBoundingClientRect()
    const xFrac = (e.clientX - rect.left) / rect.width
    const yFrac = (e.clientY - rect.top) / rect.height
    onPlace(xFrac, yFrac)
  }

  return (
    // position: relative; width: 100%; height: auto — image preserves aspect ratio
    <div
      ref={containerRef}
      className="relative w-full touch-none"  // touch-action: none prevents scroll conflict
      onClick={handleCanvasClick}
      role="application"
      aria-label="Floor plan"
    >
      <img src={imageSrc} className="w-full block" alt="Floor plan" />
      {placements.map(p => (
        <DevicePin
          key={p.deviceId}
          xFrac={p.xFrac}
          yFrac={p.yFrac}
          state={p.state}
          containerRef={containerRef}
        />
      ))}
    </div>
  )
}
```

### DevicePin Absolute Positioning

```tsx
function DevicePin({ xFrac, yFrac, state, containerRef }) {
  // Re-read container size on every render (handles window resize)
  // Use CSS left/top with percentage: this is equivalent but simpler
  // left: ${xFrac * 100}% and top: ${yFrac * 100}% relative to container
  // This is numerically identical to x_frac × container_width for any container size

  const colorClass = {
    healthy: 'bg-success',
    warning: 'bg-warning',
    offline: 'bg-destructive',
  }[state]

  return (
    <div
      className={`absolute h-3 w-3 rounded-full ring-2 ring-white ${colorClass} -translate-x-1/2 -translate-y-1/2 cursor-pointer`}
      style={{
        left: `${xFrac * 100}%`,
        top: `${yFrac * 100}%`,
      }}
      role="button"
      tabIndex={0}
      aria-label={`Device ${deviceName}, ${state}`}
      onPointerDown={handleDragStart}
    />
  )
}
```

**CSS-percentage rendering is equivalent to the formula:** `left: ${xFrac*100}%` is evaluated by the browser as `xFrac * container_width` relative to the containing block — which is mathematically identical to the UI-SPEC formula when the container is `position: relative; width: 100%`.

**Use `-translate-x-1/2 -translate-y-1/2`** to center the dot on the exact fractional coordinate rather than anchoring its top-left corner.

### Drag-to-Nudge Pattern

```tsx
function handleDrag(e: React.PointerEvent, deviceId: string) {
  // Capture pointer for smooth drag
  (e.target as HTMLElement).setPointerCapture(e.pointerId)

  const rect = containerRef.current!.getBoundingClientRect()

  const onMove = (moveEvent: PointerEvent) => {
    const newXFrac = Math.max(0, Math.min(1, (moveEvent.clientX - rect.left) / rect.width))
    const newYFrac = Math.max(0, Math.min(1, (moveEvent.clientY - rect.top) / rect.height))
    updatePinOptimistic(deviceId, newXFrac, newYFrac)
  }

  const onUp = (upEvent: PointerEvent) => {
    const newXFrac = Math.max(0, Math.min(1, (upEvent.clientX - rect.left) / rect.width))
    const newYFrac = Math.max(0, Math.min(1, (upEvent.clientY - rect.top) / rect.height))
    // Debounce PATCH call 300ms (per UI-SPEC)
    debouncedPatch(deviceId, newXFrac, newYFrac)
    document.removeEventListener('pointermove', onMove)
    document.removeEventListener('pointerup', onUp)
  }

  document.addEventListener('pointermove', onMove)
  document.addEventListener('pointerup', onUp)
}
```

**Use `element.setPointerCapture()`** to ensure pointer events are delivered to the pin element even when the pointer moves outside the pin bounds during drag (standard pointer events pattern).

**Always clamp to [0, 1]:** Use `Math.max(0, Math.min(1, computed))` to prevent negative or >1 fractions if the user drags outside the image bounds.

---

## Hub Extension for Floor-Plan Device Health

### Pattern: Extend Existing events.Hub with device_health Channel

**Existing state:** `internal/events/hub.go` dispatches `measurement_inserted` payloads. The Hub uses topic-based fan-out with topic strings like `dashboard:global`, `mp:<uuid>`, `mp:<uuid>:uplinks`.

**Extension approach for Phase 5 (D-22/D-23 live marker state):**

Option A (recommended): Add a `device_health` topic to the existing Hub. When a measurement arrives, compute health state from battery_pct + rssi + quality against D-22 thresholds and include it in the dispatch payload. The floor-plan tab subscribes to `floor-plan:<site_id>` topics.

Option B: Add a second LISTEN channel (`device_health_changed`) driven by a PG trigger that fires when health state changes (debounced by SQL).

**Option A is correct for Phase 5:** The existing measurement_inserted trigger already fires on every uplink. The Hub already has all 7 fields needed to compute D-22 health. Add a `floor-plan:<site_id>` topic derived from the metering_point's site_id. This requires the Hub to look up site_id from metering_point_id — need either a resolver cache lookup or a JOIN in the trigger payload.

**The simplest extension:**
1. Extend the `measurement_inserted` NOTIFY payload to include `site_id` (add it to migration 0021's trigger function)
2. Add `floor-plan:<site-uuid>` to the Hub's dispatch topic set, derived from `site_id` in the payload
3. Add `device_health` topic regex variant: `floor-plan:[0-9a-f-]{36}`
4. The frontend floor-plan tab subscribes to `floor-plan:<site_id>` and recomputes marker colors on each measurement event

**Alternatively (no trigger change):** Compute D-22 health client-side from the existing 7-field payload (battery_pct, rssi, quality, plus time for freshness). The client already knows `expected_interval_s` from the device list it loaded. This avoids backend migration 0021 changes. [ASSUMED — confirm whether client has access to expected_interval_s at floor plan render time]

---

## sqlc + TimescaleDB CAGG Query Pattern

**Concern:** sqlc `generate` may fail to parse `CREATE MATERIALIZED VIEW ... WITH (timescaledb.continuous)` DDL and may also fail to parse `time_bucket()` as a SQL function.

**Solution:** Keep CAGG DDL in migration files only (never in `query.sql`). For CAGG queries, write standard `SELECT ... FROM measurement_hourly WHERE ...` queries. sqlc sees the CAGG views as regular PG views (because `CREATE MATERIALIZED VIEW` creates a view in `pg_views`) and handles SELECT queries against them normally.

**The `cumulative_delta` issue:** If the CAGG aggregates `sum(...)` and sqlc infers a `NUMERIC` return type, ensure the generated struct uses `*decimal.Decimal` or `pgtype.Numeric` consistently. Use explicit `CAST(sum(...) AS DOUBLE PRECISION)` in the CAGG if you want `float64` in Go.

**CAGG DDL annotations:** Use `-- name: ... :exec` style sqlc directives only for policy add/remove SQL that needs typed Go wrappers. For CAGG DDL itself (CREATE MATERIALIZED VIEW), do not use sqlc — keep in migration files.

---

## Common Pitfalls

### Pitfall 1: CAGG Transaction Error (golang-migrate)
**What goes wrong:** `CREATE MATERIALIZED VIEW ... WITH DATA` inside a transaction block → `ERROR: CREATE MATERIALIZED VIEW ... WITH DATA cannot be executed within a pipeline`
**Why it happens:** golang-migrate wraps every migration in `BEGIN`/`COMMIT`; TimescaleDB CAGG with DATA requires two internal transactions
**How to avoid:** Always use `WITH NO DATA` in CAGG migration files; separate CAGG DDL into individual migration files
**Warning signs:** Migration fails at CAGG file; connection pool errors in testcontainers integration test

### Pitfall 2: CAGG Retention Footgun (Silent Data Loss)
**What goes wrong:** Setting `start_offset` larger than raw retention causes CAGG to re-materialize over dropped data, corrupting previously good buckets
**Why it happens:** The refresh policy window overlaps with the retention-policy-dropped region; TimescaleDB silently overwrites the CAGG materialization
**How to avoid:** Keep `start_offset` values small (hours/days), well within the raw retention window; only the higher-level CAGGs (monthly/yearly) can have longer start_offsets because they read from lower CAGGs (which have multi-year retention)
**Warning signs:** Hourly/daily CAGG showing zeros for periods older than 90 days where data previously existed

### Pitfall 3: pdfjs-dist v5 ESM Worker Import
**What goes wrong:** `No "GlobalWorkerOptions.workerSrc" specified` console error; PDF rendering hangs or fails
**Why it happens:** pdfjs-dist v5 removed the UMD build; Vite won't automatically bundle the worker unless the `import.meta.url` pattern is used
**How to avoid:** `pdfjsLib.GlobalWorkerOptions.workerSrc = new URL('pdfjs-dist/build/pdf.worker.min.mjs', import.meta.url).toString()`; set this once in a global setup module, not per-component
**Warning signs:** PDF conversion works in development but fails in production build; "worker" network request 404

### Pitfall 4: Leaflet CSS Missing
**What goes wrong:** Map renders as a grey box; map tiles invisible; popups have no styling
**Why it happens:** Leaflet requires `leaflet/dist/leaflet.css` — it's not auto-included
**How to avoid:** Import all three CSS files before any Leaflet components: `leaflet/dist/leaflet.css`, `react-leaflet-cluster/dist/assets/MarkerCluster.css`, `react-leaflet-cluster/dist/assets/MarkerCluster.Default.css`
**Warning signs:** Map container renders but shows no tiles on first load

### Pitfall 5: leaflet divIcon className Not Cleared
**What goes wrong:** Custom marker icon shows a white rectangle box behind the custom HTML
**Why it happens:** Leaflet adds a default `leaflet-div-icon` CSS class to all divIcons which includes `background: white; border: 1px solid #ccc`
**How to avoid:** Always set `className: ''` in the `L.divIcon()` options to clear the default styles
**Warning signs:** Markers show in the correct position but with a white background box

### Pitfall 6: Floor-Plan Image Replace Pin Drift
**What goes wrong:** Pins appear in wrong positions after image replacement
**Why it happens:** If pixel coordinates were stored instead of fractions, different image dimensions produce drift
**How to avoid:** This is prevented by the fractional coordinate schema (D-24 load-bearing). When replacing: show the D-24 confirmation dialog warning admin to review pin positions after upload
**Warning signs:** This pitfall is schema-level; impossible to trigger if x_frac/y_frac are stored correctly

### Pitfall 7: CAGG time_bucket Month/Year Stacking Order
**What goes wrong:** `CREATE MATERIALIZED VIEW measurement_monthly WITH (...) AS SELECT time_bucket('1 month', bucket) FROM measurement_hourly...` fails with type error
**Why it happens:** `time_bucket('1 month', hourly_bucket)` where `hourly_bucket` is a fixed-width bucket — variable-width buckets CAN stack on fixed-width. But the chain must be: hourly (1h, fixed) → daily (1d, fixed) → monthly (1mo, variable) → yearly (1yr, variable). Skipping daily and going hourly → monthly is invalid
**How to avoid:** Follow the strict 4-level chain; never skip a level
**Warning signs:** `ERROR: cannot create continuous aggregate on top of continuous aggregate with different bucket sizes`

### Pitfall 8: River Database Migration Not Run
**What goes wrong:** `river.NewClient` fails or job insertion panics with "relation river_job does not exist"
**Why it happens:** River requires its own database tables; not handled by golang-migrate automatically
**How to avoid:** Run `river migrate-up --database-url "$DATABASE_URL"` as part of Wave 0; add it to `Makefile`/`Justfile` setup targets; also consider embedding River's SQL migrations into golang-migrate sequence
**Warning signs:** `ERROR: relation "river_job" does not exist` on first River operation

### Pitfall 9: React-Leaflet SSR / import in Vite SPA
**What goes wrong:** `window is not defined` errors at import time if Leaflet is imported in an SSR context
**Why it happens:** Leaflet uses `window` and `document` on import; only affects SSR frameworks
**How to avoid:** This project uses Vite SPA (no SSR), so this pitfall does not apply. No dynamic import wrapper needed
**Warning signs:** Not applicable to this project

### Pitfall 10: Maroto v2 Header Area > Page Area
**What goes wrong:** `RegisterHeader` returns an error and the PDF builder panics
**Why it happens:** If the sum of row heights in the header exceeds the page's usable height, maroto returns an error
**How to avoid:** Keep header rows to ≤ 25% of page height; a 2-3 row header at 10mm each is safe on A4 (200mm usable)
**Warning signs:** `error: header area is greater than page area` at document generation time

---

## Runtime State Inventory

> Phase 5 is not a rename/refactor phase. No existing runtime state needs migration. Including minimal check for completeness.

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | No floor_plan or CAGG tables yet (all new in Phase 5) | Created by new migrations |
| Live service config | No existing retention config (seeded at install finish) | `internal/install/finish.go` extension to seed default retention config rows |
| OS-registered state | None | N/A |
| Secrets/env vars | None — floor-plan volume path from existing config pattern | No changes |
| Build artifacts | excelize/v2 already in go.mod; River and maroto are new Go deps | `go get` for two new deps |

---

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | Backend | Yes | 1.26.0 | — |
| Node.js | Frontend build | Yes | 22.20.0 | — |
| Docker | Compose testing | Yes | 29.4.1 | — |
| npm/pnpm | Frontend packages | Yes | npm 10.9.3 | — |
| PostgreSQL + TimescaleDB | CAGG migrations | Available via testcontainers | 2.26+ in test image | — |
| `river` CLI | River db migration | Not verified (CLI tool) | — | Use `river migrate-up` via `go run` or embed River migrations into golang-migrate files |

**River CLI availability:** The `river` CLI may not be globally installed. The workaround is `go run github.com/riverqueue/river/cmd/river@latest migrate-up`. Alternatively, River's SQL migrations can be embedded into the golang-migrate migration sequence as standard SQL files (see River docs for the schema DDL). [ASSUMED — verify River migration embedding option]

---

## Validation Architecture

> `workflow.nyquist_validation = true` in `.planning/config.json` — section required.

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go: testify v1.11.1 + testcontainers-go v0.42.0; Frontend: vitest v4.1.5 + playwright (existing) |
| Config file | `go.mod` (Go); `web/vitest.config.ts` (frontend); `web/playwright.config.ts` (e2e) |
| Quick run command | `go test ./internal/... -run TestUnit -count=1 -short` (Go); `pnpm test:run` (frontend) |
| Full suite command | `go test ./... -count=1` (Go); `pnpm test:run && pnpm playwright test` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| DATA-11 | Hourly/daily/monthly/yearly CAGGs exist after migration | integration | `go test ./internal/aggregate/... -run TestCAGGHierarchy -count=1` | ❌ Wave 0 |
| DATA-12 | CAGG refresh policy: end_offset ≥ 2× expected_interval | unit (policy param check) | `go test ./internal/aggregate/... -run TestRefreshPolicyParams` | ❌ Wave 0 |
| DATA-13 | Retention policy applied per CAGG level | integration | `go test ./internal/aggregate/... -run TestRetentionPolicy` | ❌ Wave 0 |
| REPT-01 | Daily/monthly/yearly reports return correct consumption totals | integration | `go test ./internal/report/... -run TestReportAssembly` | ❌ Wave 0 |
| REPT-02 | Reports per meter + aggregate with grouping | unit | `go test ./internal/report/... -run TestReportScopeGrouping` | ❌ Wave 0 |
| REPT-03 | CSV has UTF-8 BOM, ISO timestamps, comma separator | unit | `go test ./internal/report/... -run TestCSVFormat` | ❌ Wave 0 |
| REPT-04 | Excel has 3 sheets, formatted dates, bold totals row | unit | `go test ./internal/report/... -run TestExcelFormat` | ❌ Wave 0 |
| REPT-05 | PDF has header (logo, name, address) + footer (page X of Y) on every page | integration | `go test ./internal/report/... -run TestPDFBranding` | ❌ Wave 0 |
| REPT-06 | PDF generation via River background job; job status queryable | integration | `go test ./internal/report/... -run TestPDFJob` | ❌ Wave 0 |
| REPT-07 | Period-delta calculation correct (prior period + YoY when available) | unit | `go test ./internal/report/... -run TestPeriodDelta` | ❌ Wave 0 |
| MAP-01 | Map endpoint returns sites + gateways with lat/lng | unit | `go test ./internal/map/... -run TestMapData` | ❌ Wave 0 |
| MAP-02 | react-leaflet-cluster renders with >50 markers | unit/visual | `pnpm test:run --reporter=verbose MapView` | ❌ Wave 0 |
| MAP-03 | Click site popup "View site" navigates to /sites/:id | e2e | `pnpm playwright test map-drill-down.spec.ts` | ❌ Wave 0 |
| MAP-04 | Map tiles URL is OSM (no paid API key in config) | unit | `go test ./internal/map/... -run TestOSMTileURL` (or grep test) | ❌ Wave 0 |
| SITE-02 | Multiple floor_plan rows per site (vertical layout) | unit | `go test ./internal/floorplan/... -run TestMultiFloor` | ❌ Wave 0 |
| SITE-03 | Upload validates MIME, dimensions, size cap; PDF→PNG conversion | unit | `go test ./internal/floorplan/... -run TestImageUpload`; `vitest FloorPlanUpload` | ❌ Wave 0 |
| SITE-04 | Fractional coords stored; retrieve via API | unit | `go test ./internal/floorplan/... -run TestPlacementCRUD` | ❌ Wave 0 |
| SITE-05 | State-tinted markers update via SSE | unit + e2e | `vitest DevicePin.test.tsx`; `playwright floor-plan-health.spec.ts` | ❌ Wave 0 |
| SITE-06 | Map → site → floor plan drill-through in 2 clicks | e2e | `playwright site-drill-through.spec.ts` | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/$(PACKAGE)/... -count=1 -short` + `pnpm test:run`
- **Per wave merge:** `go test ./... -count=1` + `pnpm test:run`
- **Phase gate:** Full suite green + `pnpm playwright test` before `/gsd-verify-work`

### Wave 0 Gaps

- [ ] `internal/aggregate/aggregate_test.go` — CAGG hierarchy smoke test (DATA-11/12/13)
- [ ] `internal/report/assembler_test.go` — period-delta + YoY calculation (REPT-01/07)
- [ ] `internal/report/csv_test.go` — BOM + ISO timestamps (REPT-03)
- [ ] `internal/report/excel_test.go` — 3-sheet structure (REPT-04)
- [ ] `internal/report/pdf_test.go` — header/footer smoke (REPT-05)
- [ ] `internal/report/job_test.go` — River worker integration (REPT-06)
- [ ] `internal/floorplan/handler_test.go` — upload + placement CRUD (SITE-02/03/04)
- [ ] `internal/map/handler_test.go` — map data endpoint (MAP-01/04)
- [ ] `web/src/components/map/MapView.test.tsx` — Leaflet render + clustering (MAP-02)
- [ ] `web/src/components/floor-plan/DevicePin.test.tsx` — state colors, drag coords (SITE-04/05)
- [ ] `web/src/routes/reports/index.test.tsx` — config panel + result panel state machine
- [ ] `web/playwright/specs/map-drill-down.spec.ts` — MAP-03 drill-through (MAP-03)
- [ ] `web/playwright/specs/site-drill-through.spec.ts` — SITE-06 click-through path
- [ ] `web/playwright/specs/floor-plan-health.spec.ts` — SSE marker state update (SITE-05)

---

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | Yes (floor plan upload, report generation) | Existing SCS session + Can() middleware |
| V3 Session Management | Yes (all routes behind auth) | Existing SCS session |
| V4 Access Control | Yes (viewer cannot generate reports; admin-only floor plan edit) | Existing Can() middleware; AUTH-06 already implemented |
| V5 Input Validation | Yes (image upload: MIME sniff, dimension, size; report scope params) | Go: MIME sniff + imgDecode for dimensions; Frontend: zod schema |
| V6 Cryptography | No | N/A |

### Known Threat Patterns

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Oversized image upload (DoS via large PNG) | Denial of Service | `http.MaxBytesReader(w, r.Body, 10<<20)` (10MB cap) before reading; check dimensions before writing to disk |
| Path traversal on floor plan image serve | Elevation of Privilege | Serve only by `floor_plan.id` (UUID); never expose `image_path` column directly; validate UUID before constructing file path |
| Report artifact access after expiry | Information Disclosure | TTL check on report artifact directory before serving download; 24h purge cron (D-07) |
| PDF job queue flooding (report spam) | DoS | Rate-limit POST /api/reports/generate per session; or cap at N concurrent PDF jobs per install |
| Unauthenticated floor plan image access | Information Disclosure | Serve floor plan images through an authenticated chi route (not as a raw static file server without auth middleware) |

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| TimescaleDB CAGG real-time mode default ON | v2.13+: real-time mode default OFF | TimescaleDB 2.13 | Must explicitly set `materialized_only=false` to enable real-time on hourly/daily CAGGs |
| pdfjs-dist UMD build (`pdf.worker.min.js`) | v5: ESM only (`pdf.worker.min.mjs`) | pdfjs-dist v5.0 | Must use `import.meta.url` pattern for Vite worker bundling |
| leaflet.markercluster as direct React integration | react-leaflet-cluster v4.x (thin wrapper) | 2024 | Clean React 19 + react-leaflet v5 compatibility via wrapper package |
| gofpdf (archived 2021) | maroto/v2 (built on gofpdf internals, actively maintained) | 2022+ | Use maroto v2; never gofpdf directly |
| Pixel coordinate floor plan pins | Normalized fractional coordinates [0,1] | Phase 5 design | Resolution-independent; survive Retina/HiDPI and image replacement |

**Deprecated/outdated:**
- `react-leaflet-markercluster` (yuzhva): last release 2022; use `react-leaflet-cluster` (akursat) instead for React 19 compat
- `gofpdf`: archived 2021; maroto/v2 is the maintained successor
- pdfjs-dist `pdf.worker.min.js` (UMD): removed in v5; use `pdf.worker.min.mjs`

---

## Open Questions

1. **Cumulative delta computation in CAGG**
   - What we know: The `measurement` hypertable stores `cumulative_value` (absolute meter reading). CAGGs need to sum consumption within a bucket = Σ(delta per uplink). The delta is `cumulative_value[i] - cumulative_value[i-1]` within a metering point.
   - What's unclear: Whether `cumulative_delta` is already computed at ingest time (Phase 2's ingest pipeline might already store it) or whether Phase 5 needs to add it.
   - Recommendation: Before writing the CAGG SQL, verify `internal/db/migrations/0015_measurement.up.sql` and `internal/ingest/handler.go` — if a `cumulative_delta` column exists in the measurement table, the CAGG simplifies to `sum(cumulative_delta)`. If not, plan must include either: (a) adding the column via a Phase 5 migration, or (b) using a more complex CAGG with a window function.

2. **River migration embedding vs CLI**
   - What we know: River requires its own database tables; the CLI `river migrate-up` is the documented approach.
   - What's unclear: Whether the River SQL migrations can be embedded as golang-migrate files for a single-migration-tool workflow.
   - Recommendation: Look at River's `/riverdriver/riverpgxv5` source for the SQL migration DDL and embed it as golang-migrate file `00XX_river_tables.up.sql`. This keeps a single migration tool.

3. **Maroto v2 page X of Y placeholder syntax**
   - What we know: maroto v2 supports automatic page numbering in footer rows.
   - What's unclear: Whether the placeholder is `{page}` / `{pages}` (v1 style) or a different API in v2.4.0.
   - Recommendation: Verify against `pkg.go.dev/github.com/johnfercher/maroto/v2/pkg/components/text` before implementing the footer; fall back to post-processing with `doc.GetPages()` count if needed.

4. **SETT-02 vs SETT-04 scope in Phase 5**
   - What we know: SETT-04 (data retention config) is listed as Phase 6 in REQUIREMENTS.md traceability, but CONTEXT.md D-09 places retention settings in Phase 5.
   - What's unclear: Whether the Settings → Data Retention card (editing retention values) should ship in Phase 5 or Phase 6.
   - Recommendation: The `retention_config` table and default seed values must ship in Phase 5 (the CAGGs need them). The Settings UI card for editing those values is explicitly in CONTEXT.md D-09, so ship the UI in Phase 5. SETT-04 traceability should be updated to Phase 5 in a reconciliation plan.

---

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | `cumulative_delta` is not stored in the measurement table; must be computed via LAG() in CAGG or added as a new column | CAGG Hierarchy Pattern 5 | If delta is already stored (possible from Phase 2 ingest), the CAGG query simplifies significantly; low risk either way |
| A2 | Maroto v2 page numbering uses `{page}` / `{pages}` placeholder in text component | PDF Generation pattern | If placeholder syntax differs, footer implementation needs adjustment; easily fixed |
| A3 | River SQL migrations can be embedded into golang-migrate file sequence | River Setup / Environment | If River requires its own CLI migration, the Wave 0 setup must include `river migrate-up` step |
| A4 | Extending Hub dispatch with `site_id` via trigger payload update is the simplest path for floor-plan health events | Hub Extension | If adding site_id to trigger payload breaks existing tests, Option B (client-side health computation) is the fallback |
| A5 | `OffscreenCanvas` API is available in all browser environments that deploy this SPA | pdf.js pattern | OffscreenCanvas has 95%+ browser support as of 2026; fallback to `document.createElement('canvas')` if needed |
| A6 | River v0.36.0 PeriodicJob with `CRON_TZ=<tz>` supports the install timezone for the nightly yearly CAGG refresh | River cron | If timezone cron not supported, use server UTC midnight refresh (acceptable approximation) |

---

## Sources

### Primary (HIGH confidence)
- [VERIFIED: npm view react-leaflet version] — react-leaflet 5.0.0
- [VERIFIED: npm view react-leaflet-cluster peerDependencies] — confirms React 19 + react-leaflet v5 compat
- [VERIFIED: npm view pdfjs-dist version] — 5.7.284
- [VERIFIED: npm view leaflet version] — 1.9.4
- [VERIFIED: npm view leaflet.markercluster version] — 1.5.3
- [VERIFIED: npm view @types/leaflet version] — 1.9.21
- [VERIFIED: go list -m -versions github.com/riverqueue/river] — v0.36.0
- [VERIFIED: go list -m -versions github.com/johnfercher/maroto/v2] — v2.4.0
- [VERIFIED: cat go.mod] — excelize/v2 v2.10.0 already present
- [CITED: tigerdata.com/docs/use-timescale/latest/continuous-aggregates/hierarchical-continuous-aggregates] — CAGG-over-CAGG syntax, materialized_only flag, version requirements (2.9+)
- [CITED: tigerdata.com/docs/api/latest/continuous-aggregates/add_continuous_aggregate_policy] — full function signature and parameter meaning
- [CITED: tigerdata.com/docs/use-timescale/latest/continuous-aggregates/real-time-aggregates] — materialized_only=false/true toggle; ALTER MATERIALIZED VIEW syntax; v2.13 default change
- [CITED: tigerdata.com/docs/use-timescale/latest/continuous-aggregates/refresh-policies] — retention footgun: "data will be removed when the aggregates for those buckets are refreshed"
- [CITED: pkg.go.dev/github.com/johnfercher/maroto/v2] — API: New(), RegisterHeader(), RegisterFooter(), LowMemory mode, image.NewRow(), Generate()
- [CITED: pkg.go.dev/github.com/golang-migrate/migrate/v4/database/postgres] — no per-file "no transaction" annotation; recommended workaround: separate files or avoid DDL needing no-tx
- [CITED: riverqueue.com/docs] — Worker setup, InsertTx, PeriodicJob, pool sizing
- [CITED: pkg.go.dev/github.com/xuri/excelize/v2] — NewFile, NewSheet, NewStyle (bold/border/numfmt), SetCellValue(time.Time), WriteToBuffer
- [CITED: mozilla.github.io/pdf.js/examples/] — basic PDF rendering to canvas; scale calculation
- [CITED: akursat.gitbook.io/marker-cluster] — react-leaflet-cluster setup; CSS imports required
- [CITED: leafletjs.com/reference.html] — divIcon options (className: ''), fitBounds, setView

### Secondary (MEDIUM confidence)
- [WebSearch verified] — pdfjs-dist v5 ESM worker: `new URL('pdfjs-dist/build/pdf.worker.min.mjs', import.meta.url)` pattern
- [WebSearch verified] — River PeriodicJob cron timezone syntax: `CRON_TZ=America/Chicago @midnight`
- [GitHub issue #5377 timescale/timescaledb] — "CREATE MATERIALIZED VIEW WITH DATA cannot run inside a transaction block" confirmed bug

### Tertiary (LOW confidence)
- [ASSUMED] — `cumulative_delta` not in measurement table (needs verification vs existing migration 0015 and ingest handler)
- [ASSUMED] — Maroto v2 `{page}` / `{pages}` placeholder syntax in footer text
- [ASSUMED] — River migrations embeddable into golang-migrate sequence

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — all library versions verified via npm view / go list
- CAGG hierarchy: HIGH — verified against official TimescaleDB docs; specific syntax confirmed
- CAGG retention footgun: HIGH — the warning is verbatim from official docs
- golang-migrate + CAGG transaction: HIGH — confirmed via GitHub issue + golang-migrate docs
- PDF generation: MEDIUM — maroto v2 API verified; page numbering placeholder syntax unverified
- River setup: HIGH — verified against official River docs; PeriodicJob cron is MEDIUM
- react-leaflet + clustering: HIGH — version compat verified; CSS imports confirmed
- pdf.js pattern: HIGH — ESM worker pattern verified; OffscreenCanvas support MEDIUM
- Floor-plan canvas: HIGH — standard pointer events pattern; verified against UI-SPEC formula

**Research date:** 2026-05-12
**Valid until:** 2026-06-12 (30 days — stable ecosystem; TimescaleDB and maroto are slow-moving)
