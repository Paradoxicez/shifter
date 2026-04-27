# Project Research Summary

**Project:** Shifter
**Domain:** Self-hosted, single-tenant LoRaWAN water/electricity utility monitoring (ChirpStack-wrapping operator console)
**Researched:** 2026-04-27
**Confidence:** HIGH

## Executive Summary

Shifter is a "wrap, do not replace" operator console over ChirpStack v4. The four research streams converge on a clear shape: a single Go binary (with embedded SPA) that talks to ChirpStack over **two channels simultaneously** — gRPC for the control plane (devices, gateways, profiles) and MQTT for the telemetry event stream — and persists everything into one Postgres+TimescaleDB instance. Both the bundled-ChirpStack and external-ChirpStack deployment modes are achieved by changing only environment variables; the binary is bit-identical. The frontend is a Vite + React 19 + shadcn/ui SPA served by the Go binary, with realtime push to the browser via SSE driven off Postgres `LISTEN/NOTIFY` from a hypertable insert trigger — never directly off MQTT, so the dashboard only ever sees data that has been durably persisted.

The core architectural decision is not the language or the framework — it is the **metering-point + time-windowed device assignment + reading offset** model. Every report, chart, and alert queries by `metering_point_id`, never by `dev_eui`. Devices are bound to metering points for windows of time, with a `reading_offset` that keeps the displayed cumulative continuous across swaps and counter rollovers. This is the headline differentiator (no competitor in this niche centers on it) and also the schema invariant that, if violated, requires a multi-week migration to recover. Codecs live in ChirpStack's per-profile QuickJS sandbox; Shifter only ever speaks in canonical fields (`cumulative`, `flow_rate`, `voltage`, `battery_pct`) plus a per-row JSONB `extra` for everything else. New vendors are admin-UI work, never backend work.

The principal risks cluster around (a) data-model footguns that are nearly free to prevent on day 1 but extremely expensive to retrofit (device-as-source-of-truth, swap/rollover math, wrong timestamp column, retention-wipes-aggregates, pixel-coords-on-floor-plans), (b) operational gaps unique to self-hosted single-tenant per install (backups, image volumes, log rotation, secrets, upgrade path, codebase drift across customers), and (c) ChirpStack-specific traps (v3/v4 API mismatch, AS923 sub-plan picker for Thailand, false offline alerts from normal LoRaWAN packet loss, OTAA endianness on the device sticker). All are addressable but they must be ordered correctly into the roadmap — most belong to Phase 1 and Phase 2 because retrofitting them later is what kills these projects.

## Key Findings

### Recommended Stack

The research recommends **Go 1.24+** as the backend language, decided on three converging criteria: native gRPC stubs published by ChirpStack (`github.com/chirpstack/chirpstack/api/go/v4`), a 5–15MB single-binary deploy that fits the per-customer self-hosted model, and a mature ecosystem for every workload Shifter actually needs (MQTT subscribers, Postgres/TimescaleDB inserts, REST APIs, scheduled jobs). Node.js and Python were rejected on deploy weight; Rust on velocity and ecosystem maturity. The frontend is **Vite + React 19 + TypeScript + Tailwind v4 + shadcn/ui** — explicitly not Next.js, because Shifter is a logged-in dashboard with no SEO, no SSR benefit, and no marketing surface. The Go binary serves the SPA via `go:embed`. Realtime is **SSE** (one-way, server→browser), not WebSocket — the data flow is one-way, browsers handle reconnection natively, and reverse proxies don't need WebSocket-specific config. WebSocket is the right pick later if/when bidirectional needs (e.g., live device control) emerge.

**Core technologies:**
- **Go 1.24+** — backend language. Single binary, native ChirpStack gRPC stubs, mature MQTT/Postgres/scheduler libraries.
- **PostgreSQL 16/17 + TimescaleDB 2.26** — one DB for everything (relational metadata, telemetry hypertable, continuous aggregates). Shared with ChirpStack in bundled mode (separate databases inside the same instance).
- **ChirpStack v4** — non-negotiable LoRaWAN NS+AS that Shifter wraps. v3 is unsupported.
- **Mosquitto 2.x** — MQTT broker carrying the ChirpStack→Shifter event stream and gateway↔CS traffic.
- **Vite + React 19 + TypeScript + shadcn/ui (Tailwind 4)** — frontend SPA, served as static assets by the Go binary.
- **react-leaflet 5 + Leaflet 1.9** — OSM map view (lighter than MapLibre and right-sized for "site pins on tiles"). MapLibre is the upgrade path if marker counts cross ~5k or vector tiles become required.
- **sqlc + pgx/v5** — typed Go SQL access; preserves TimescaleDB-specific functions (`time_bucket`, hyperfunctions) that ORMs hide. Explicitly **avoid GORM**.
- **River** — Postgres-backed background job queue (no Redis dependency).
- **alexedwards/scs** — server-side sessions in Postgres; httpOnly cookie; no JWT in localStorage.
- **excelize/v2 + maroto/v2 + stdlib `encoding/csv`** — Excel, PDF, CSV exports. `gofpdf` is archived; do not use.
- **golang-migrate** — plain SQL migration files.
- **TanStack Query + react-hook-form + zod** — data fetching, forms, validation on the SPA side.
- **Caddy** — reverse proxy / TLS termination in front of the Go binary.
- **Docker Compose × 2 files** — `docker-compose.bundled.yml` (Postgres+TimescaleDB+Mosquitto+ChirpStack+Shifter) and `docker-compose.external.yml` (Postgres+TimescaleDB+Shifter, ChirpStack+MQTT URLs supplied via env). Same backend image in both.

### Expected Features

The research benchmarks Shifter against four product categories whose feature sets it must union: LoRaWAN network servers (ChirpStack/TTS), MDMS systems (Itron/Bynry/Genus), AMI dashboards (Kamstrup/Badger/Sensus), and IoT visualization platforms (ThingsBoard/Datacake/Akenza/Kaa). The MVP must deliver the union of these category's *table stakes*, never the full superset of any one of them.

**Must have (table stakes — v1):**
- Local auth (email + password), admin/viewer roles, admin-managed users, force-change-password on first login, session timeout — operator security baseline.
- Install wizard (admin user, ChirpStack mode, region default, install identity) — first-run otherwise blank.
- ChirpStack connection over gRPC + "Test connection" — backbone, both bundled and external modes.
- Gateway list with online/offline + add/edit/delete + lat/lng on map + RX/TX stats + frequency plan — operator-parity with ChirpStack here is mandatory.
- Device list with last-seen + battery + RSSI/SNR; one-action add device dialog (collapses CS tenant→app→profile→device into one form); device profile management with custom JS codec; soft-delete; search/filter.
- Bulk CSV import with **two-phase dry-run + commit**, idempotent re-run, per-row outcome log.
- Sites with create/edit/delete, site lat/lng for map, horizontal (campus) and vertical (multi-floor) layouts, floor-plan image upload (PNG/JPG/PDF→PNG), device placement on floor plans (stored as **normalized fractions** 0..1, not pixel integers — see pitfalls).
- OSM map view (Leaflet) with marker clustering above ~50 markers; click-through map → site → floor plan.
- Canonical measurement model: hybrid wide+JSONB hypertable. Per-profile parameter→canonical mapping.
- **Metering-point abstraction + meter-swap with reading offset + confirmation UI** — the headline differentiator.
- Live dashboard via SSE, adaptive to install scope (water-only / electricity-only / both), KPIs (today's consumption, current flow, period-delta), online/offline device count, time-series charts, date-range pickers.
- Per-meter detail (normal + advanced view + recent uplinks log + battery/signal sparklines).
- Reports: daily/monthly/yearly, per-meter and aggregate; CSV/Excel/PDF export with install identity branding.
- Threshold alerts + device-offline alerts (gateway-down suppression, N≥3 consecutive missed uplinks, hysteresis grace) + in-app alert center with ack + notes.
- Audit log (admin) with filter + CSV export + retention policy.
- Settings page (categorized): install identity, ChirpStack connection, units, timezone, alerts config, backup status. Region picker is regulator-aware (AS923-x sub-plan defaulted for Thailand).
- Backup script (TimescaleDB-aware logical backup + image-volume rsync) + restore procedure tested in CI + last-backup display in settings.

**Should have (differentiators — v1 where checked, v1.x where deferred):**
- v1 — One-action device onboarding that hides ChirpStack's 5-step mental model. *This is a top-three selling point.*
- v1 — Vendor-agnostic canonical measurement model — competitors are usually single-vendor or punt on canonicalization.
- v1 — Floor-plan placement with state-tinted markers (green/yellow/red) overlay — BMS-class feature most LoRaWAN tools lack.
- v1 — Dual-mode ChirpStack (bundled or external) — onboarding both greenfield and brownfield is rare in the niche.
- v1 — Adaptive dashboard (water-only / electricity-only / both).
- v1 — Single-Compose deploy with two compose flavors.
- v1 — In-product backup with last-backup-age surfaced in Settings.
- v1.x — Pre-seeded vendor profile catalog (Kamstrup MULTICAL, Diehl, Itron, Axioma, Sagemcom, Acrel, Schneider IEM3xxx, etc.) — first-time-experience win once we know which vendors customers run.
- v1.x — Codec test-runner inside profile editor — paste hex → see decoded JSON → see canonical mapping.
- v1.x — Statistical anomaly / leak detection (P95, IQR, quiet-hour flow); cold-start gate of ≥21 days history. **No ML in v1.**
- v1.x — Webhook outbound for alerts (Slack/Discord/n8n) — sidesteps the "no SMTP" v1 constraint cleanly.

**Defer (v2+):**
- SMTP delivery (password reset, email alerts, scheduled email reports) — accept the install dependency only when justified.
- SSO / OAuth (Google, Azure AD, Okta, OIDC) — when an enterprise prospect requires it.
- Scheduled / S3 backups; cost / tariff display; native mobile / PWA; localization / i18n; predictive maintenance ML; public versioned REST/GraphQL API; resident-facing portal (do not build — integrate); per-user dashboards; in-app commenting.

### Architecture Approach

The dominant pattern is a **two-path architecture inside one Go binary**: a write-heavy MQTT-driven ingestion path (subscribe → normalize → resolve metering point → INSERT into hypertable) and a read-heavy HTTP/SSE API path (REST CRUD + history queries + realtime push). They share storage (one Postgres) but never call each other directly — the database is the seam. CQRS-lite. Realtime push is decoupled from MQTT via a Postgres `AFTER INSERT` trigger that emits `pg_notify('measurements', …)`; a `LISTEN`er in the Go binary fans out to per-session SSE subscribers filtered by visible metering points. This guarantees the browser only sees persisted data — no "appears live then disappears on reload" bugs. Continuous aggregates (hourly → daily → monthly → yearly, hierarchical) are TimescaleDB CAGGs with refresh policies, not application cron jobs. ChirpStack is the only module that imports the protobuf types; everything else speaks `domain/` types, so a future LNS variant or ChirpStack version bump is an isolated change.

**Major components:**
1. **ChirpStack (external dependency)** — LoRaWAN NS+AS+codec runtime. Bundled (Compose) or external (env-configured URLs). Never modified.
2. **Mosquitto (broker)** — carries `application/+/device/+/event/up` and gateway↔CS traffic. Bundled in greenfield mode; external customer's broker in brownfield mode.
3. **Shifter ingestion module** (Go, in-binary) — MQTT subscriber → vendor→canonical normalizer → metering-point resolver (with reading-offset application) → hypertable INSERT.
4. **Shifter API module** (Go, in-binary) — chi-based REST for CRUD and history queries; SSE hub backed by `LISTEN` on `pg_notify`; static SPA serving via `go:embed`.
5. **ChirpStack gRPC adapter** (Go, in-binary) — the *only* module that imports CS protos; wraps `DeviceService`, `GatewayService`, `DeviceProfileService`, etc., and exposes domain-shaped methods. Collapses CS multi-step flows into single calls.
6. **Background workers** (Go, in-binary, River-based) — alert evaluation, offline-watchdog, CAGG refresh hints, report generation jobs.
7. **PostgreSQL 16/17 + TimescaleDB 2.26** — relational tables (users, sites, floor_plans, metering_points, devices, alerts, audit_log) + `measurements` hypertable + hierarchical CAGGs (`measurement_hourly` → `_daily` → `_monthly` → `_yearly`).
8. **Filesystem volume** — floor-plan images on a local Docker volume; thin `files` package abstraction so a future `S3_BUCKET=…` swap is a one-env change.
9. **Caddy reverse proxy** — TLS termination, single 80/443 ingress, routes `/api`, `/sse`, `/files`, `/` (SPA).
10. **Frontend SPA** — Vite + React 19 + shadcn/ui; talks REST for CRUD and history, SSE for live measurement push.

The ingestion path and API path run as goroutines in one process. There is no microservice split until the rare scale event (50+ gateways, 50k+ meters, 1M+ uplinks/hour) warrants moving the ingestion module out — and even then it stays in the same codebase.

### Critical Pitfalls

1. **Device as source of truth (loses history on swap).** The schema must center the `metering_point` entity with a time-windowed `metering_point_assignment(metering_point_id, dev_eui, valid_from, valid_to, reading_offset)` table. Telemetry is keyed by `metering_point_id`, *never* by `dev_eui`. Retrofitting this after launch is a multi-week migration. Lock it in Phase 1.
2. **Cumulative-reading math (swap + counter rollover).** Store the *raw* device-reported register value, never overwrite. Compute `display = raw + offset_for_active_binding`. Detect rollover when `raw_t < raw_t-1` and advance the offset by the counter modulus, logging it. Synthetic-data tests must cover swap, rollover, swap+rollover, and overlapping-uplinks-during-swap.
3. **Codec hell (per-vendor decoder switch in Go).** Decode in **ChirpStack's per-profile QuickJS sandbox**, never in Shifter. Shifter only consumes the decoded `object` and maps to canonical fields. New vendor = new ChirpStack profile + a Shifter mapping row, *not* a backend deploy. Maintain a Shifter-side device profile catalog (curated codecs + canonical mappings) seeded from the TTN lorawan-devices repo where possible.
4. **False offline alerts (LoRaWAN normal packet loss looks like outages).** Define "offline" as **N≥3 consecutive missed expected uplinks**, where expected interval is per-profile (or per-device override). Hysteresis grace period. Suppress device-offline alerts globally when the *gateway* serving them is offline (one alert per gateway, not 200 alerts for the devices behind it).
5. **Wrong timestamp column on the hypertable.** Use the **server-side ingest time** as the authoritative `time` column — monotonic, deduplicated by ChirpStack, never goes backward. Persist `gateway_rx_time` and device-side time as diagnostic columns. Set CAGG `end_offset` ≥ a couple of expected-interval windows to absorb late uplinks. Changing this later is a hypertable rebuild.
6. **Retention silently wipes continuous aggregates.** A 90-day retention on raw + a 7-day-window CAGG refresh = re-refresh of a 6-month-old bucket sees no source data and overwrites the materialization with empty. The CAGG `start_offset` MUST be ≤ the retention window, OR have a separate longer retention policy on the aggregate hypertable itself (raw 90d, daily-agg 5y, monthly-agg 20y).
7. **ChirpStack v3 vs v4 API mismatch.** Target v4 only. Install validation must refuse v3. Use UUIDs end-to-end, regenerate API tokens on upgrade, consume integration events via MQTT topic `application/+/device/+/event/up` with the decoded `object` field as a struct (not JSON string).
8. **AS923 sub-plan confusion (Thailand).** Region picker is regulator-aware — "Thailand (AS923-2)" pre-defaulted, not free-text "AS923". Device profiles in the catalog declare which sub-plans they support. Surface a warning when a device's profile doesn't match the gateway region.
9. **Floor-plan pixel coordinates that drift on image replacement and Retina.** Store **normalized fractions (`x_frac`, `y_frac` ∈ [0, 1])**, not pixel integers — resolution-independent by construction, survives image replacement and `devicePixelRatio`. **PROJECT.md currently says "pixel coordinate placement" — this should be reworded to "normalized fractional coordinates on the floor plan" before/during Phase 1.**
10. **Bulk import that partially commits on validation failure.** Two-phase: validate-all (dry-run with full per-row error report), then commit-all. Idempotent re-run on identical CSV. DevEUI normalization (case-insensitive, hex-only, 16 chars).
11. **Self-hosted operations gaps (backups, image volumes, log rotation, secrets, upgrade path).** Each install is a tiny ops surface; the install kit is the deliverable. CI-tested backup+restore script, file-based Compose secrets (not `.env`), pinned image tags (never `:latest`), documented upgrade runbook, `/health` endpoint reporting DB+CS+MQTT+disk+last-uplink-age.
12. **Codebase drift across customer installs.** One main branch, no customer branches. Customer-specific behavior is config flags or seed data, never forks. Every install reports its version in `/health`. Forward-only idempotent migrations.

(Full inventory of 16 critical pitfalls + technical-debt patterns + integration gotchas + performance traps + security mistakes + UX pitfalls + a "Looks Done But Isn't" checklist + recovery strategies lives in PITFALLS.md.)

## Implications for Roadmap

Based on combined research, the suggested phase structure is **seven phases**. Phases 1–2 are non-negotiable foundations because retrofitting them later is what kills these projects. Phases 3–5 deliver the user-visible product. Phase 6 closes the operational and security loop. Phase 7 broadens vendor coverage and hardens the install kit for paying customers.

### Phase 1: Foundation (skeleton, auth, ChirpStack adapter, dual-channel integration, single binary)

**Rationale:** Every subsequent phase requires (a) a Go binary that boots and serves a session-authenticated SPA, (b) a working two-channel ChirpStack integration (gRPC for control, MQTT for events), and (c) a Postgres+TimescaleDB instance reachable from the binary. Land these together because each one is meaningless without the others.

**Delivers:**
- Single Go binary (`shifter serve`, `shifter migrate`, `shifter create-admin` via Cobra) with embedded SPA.
- Postgres 16/17 + TimescaleDB 2.26 + golang-migrate plumbing.
- ChirpStack gRPC client wired (lists devices/gateways read-only) + MQTT subscriber wired (logs `application/+/device/+/event/up` to stdout, no DB write yet).
- Local auth: email/password (Argon2id), SCS server-side sessions in Postgres, admin/viewer roles via `can(user, action, resource)` (forward-compatible for future role bundles), force-change-password on first login, rate-limited login.
- Install wizard skeleton: admin user, ChirpStack mode (bundled/external) with version-validation gate that refuses v3, region default with **AS923 sub-plan picker (Thailand-correct)**, install identity (name/logo/address/timezone/units).
- Two compose files (`docker-compose.bundled.yml`, `docker-compose.external.yml`) sharing the same backend image; `.env.example`; Caddyfile; pinned image tags; file-based Compose secrets.
- Release-engineering rules: one main branch, no customer branches, `/health` endpoint reports DB+CS+MQTT+disk+last-uplink-age and the running version.

**Addresses (FEATURES.md):** Auth, install wizard, ChirpStack connection (Test Connection button), settings shell.

**Avoids (PITFALLS.md):** #7 v3/v4 mismatch (install gate), #8 AS923 sub-plan, #14 two-role coarseness (`can()` API), #15 ops gaps (compose secrets, pinned tags, `/health`), #16 codebase drift (one-branch rule documented).

### Phase 2: Domain model — metering points, canonical measurements, device assignment, swap-with-offset

**Rationale:** The metering-point + time-windowed assignment + reading offset is the schema invariant that everything else depends on. Building dashboards or reports on a device-keyed schema is the single most expensive mistake possible in this project. The canonical hybrid wide+JSONB measurement schema is upstream of dashboard, reports, and alerts. Both must be implemented and synthetic-data-tested before the first telemetry row is written for real.

**Delivers:**
- Schema: `site`, `floor_plan`, `metering_point`, `device`, `metering_point_assignment(valid_from, valid_to, reading_offset)`, `measurement` hypertable (canonical wide columns + JSONB `extra` + JSONB `raw`), CAGGs scaffolded but not yet populated.
- **Server-side ingest time as the authoritative `time` column** on the hypertable; `gateway_rx_time` + device-side time as diagnostic columns.
- Ingestion service end-to-end: MQTT subscribe → decode CS event → look up device-profile family (cached) → normalize CS `object` → canonical Measurement → resolve `dev_eui` → active `metering_point_id` (cached, TTL ~60s) → apply `reading_offset` → INSERT.
- Rollover detection: when `raw_t < raw_t-1`, advance offset by counter modulus; log + surface as gateway/device health flag.
- One device family wired in the normalizer (pick the most-common vendor first).
- REST endpoints (chi) for sites/metering-points/devices CRUD; **one-action add-device dialog** that auto-creates ChirpStack tenant/app/profile bindings (collapses the 5-step CS flow).
- **Meter-swap UI (the headline differentiator):** dialog reads last reading R from outgoing device, closes current assignment with `valid_to=now()`, inserts new assignment with `valid_from=now()`, `reading_offset = previous_offset + R - new_meter_initial`. Continuity-verification UI compares last/first readings and proposes the offset; admin confirms.
- Synthetic-data test harness covering: clean swap, swap with concurrent in-flight uplink, counter rollover, swap+rollover, overlapping old/new uplinks during swap.
- Audit log table + middleware ships **before** the swap UI so swaps are auditable from day 1.

**Addresses (FEATURES.md):** Sites + site lat/lng, devices + device profiles, metering point + meter swap, canonical measurement, audit log (skeleton).

**Avoids (PITFALLS.md):** #1 device-as-source-of-truth, #2 rollover/swap math, #3 codec hell (codecs in ChirpStack, mappings in Shifter), #5 wrong timestamp.

### Phase 3: Devices + gateways management + bulk CSV import

**Rationale:** Phase 2 shipped the *data model* and the *one-device path*. Phase 3 turns those into the operator's daily provisioning surface. Bulk import is the difference between "demo" and "real customer install" — real deployments register 50–500 meters at once. Two-phase dry-run + commit is mandatory, not an MVP cut.

**Delivers:**
- Gateway list (online/offline, last-seen, RX/TX stats from `GetGatewayStats`), add/edit/delete dialog with regulator-aware region picker, gateway lat/lng on map.
- Device list with last-seen + battery + RSSI/SNR + soft-delete + search/filter by site/status/vendor/type.
- Device profile management UI wrapping ChirpStack profiles, with custom JS codec field (codec runs in ChirpStack QuickJS sandbox, *not* in Shifter).
- DevEUI sticker parser handling both endiannesses with operator preview; OTAA-default with explicit ABP-not-recommended warning; no "Disable frame-counter validation" default.
- Bulk CSV import: dry-run validator (duplicate DevEUI in CSV, duplicate against existing devices, missing required fields, malformed AppKey, unknown profile, invalid metering-point binding) with downloadable report → commit phase with per-row outcome log → idempotent re-run on identical CSV. Downloadable CSV template.
- Audit-log entries on every CRUD + every bulk-import row outcome.

**Addresses (FEATURES.md):** Gateway management, device management, bulk import, audit log (CRUD breadth).

**Avoids (PITFALLS.md):** #4 false offline alerts (RX/TX stats wired correctly), #12 partial bulk import, #13 OTAA endianness, #14 admin-only fields server-side (viewer cannot see AppKey).

### Phase 4: Telemetry realtime + dashboard + per-meter detail

**Rationale:** Phase 2 inserts measurements; Phase 4 makes them visible. Realtime push and dashboard are independent of CAGGs (Phase 5) but together with Phase 5 deliver the "what users open every day" surface. Land realtime first so the dashboard feels alive; add aggregates next so it feels useful.

**Delivers:**
- Postgres `AFTER INSERT` trigger on `measurement` emits `pg_notify('measurements', payload)` with `(metering_point_id, time, cumulative, flow_rate)`.
- SSE hub in Go binary: per-session subscription filter scoped to visible `metering_point_id`s; `EventSource` from browser; exponential-backoff reconnect with jitter on the client; TLS session resumption + heartbeat ping/pong; backpressure drops oldest deltas + sends snapshot on slow client.
- Live dashboard adaptive to install scope (water-only / electricity-only / both), KPI tiles (today's consumption, current flow, period-delta, online/offline device count), shadcn-charts time-series with date-range pickers.
- Per-meter detail page: normal view (cumulative, instantaneous, last-update, alarms), advanced view (full decoded `object` from JSONB `extra`, generic key/value tree), recent uplinks log (last 100–500 events), battery/signal sparklines.
- Latest-reading-per-metering-point materialized view, refreshed on uplink, to keep dashboard load O(metering_points), not O(measurements).

**Addresses (FEATURES.md):** Live dashboard with real-time push, adaptive scope, per-meter detail (normal+advanced+uplinks log), time-series charts.

**Avoids (PITFALLS.md):** #9 WebSocket/SSE reconnect storms (jitter + backoff + heartbeat from day 1), and the anti-pattern of "coupling realtime push directly to MQTT" (we go through `pg_notify`, so the dashboard only ever sees persisted data).

### Phase 5: Continuous aggregates + reports + exports + map + floor plans

**Rationale:** Reports are the purchase justification. Once the dashboard exists, reporting needs CAGGs (hierarchical: hourly → daily → monthly → yearly), and exports need install-identity (logo/address) from Settings — sequence Settings → Reports → PDF, not the other way. The map and floor-plan placement complete the spatial UX surface; both are mid-complexity but have a couple of high-impact pitfalls (#10 marker count, #11 fractional coords).

**Delivers:**
- Hierarchical CAGGs: `measurement_hourly` (over `measurement`), `measurement_daily` (over `_hourly`), `measurement_monthly` (over `_daily`), `measurement_yearly` (over `_monthly`); refresh policies with `end_offset` ≥ 2× expected-interval; **retention policy on raw `measurement` (default 90d) MUST have `start_offset ≤ retention window`, with separate longer retention on the aggregate tables (5y daily, 20y monthly)**.
- Reports endpoints: per-meter and aggregate ("all meters" by site / by category) for daily/monthly/yearly with date-range; period-delta and % change calculations.
- Exports: CSV (UTF-8 BOM, ISO timestamps, timezone in header), Excel (`excelize/v2`, formatted dates/units/totals), PDF (`maroto/v2` with install identity branding from Settings).
- Map view (Leaflet 1.9 + react-leaflet 5) with **`leaflet.markercluster`** clustering from day 1; viewport culling via server-side bbox query; mobile zoom-level limit; click-through map → site → floor plan.
- Floor-plan upload (PNG/JPG/PDF→PNG; re-encode to canonical orientation; max-size cap; whitelist formats); device placement stored as **normalized fractions (`x_frac`, `y_frac` ∈ [0, 1])**, never pixel integers; render as `screen_x = x_frac × image_displayed_width`. State-tinted markers (green/yellow/red) overlay.
- **PROJECT.md update:** the line "pixel-coordinate device placement" is reworded to "normalized fractional coordinates on the floor plan."

**Addresses (FEATURES.md):** Reports (daily/monthly/yearly per-meter + aggregate), CSV/Excel/PDF export, map view, floor-plan placement (horizontal + vertical layouts).

**Avoids (PITFALLS.md):** #6 retention-wipes-aggregates, #10 map performance, #11 floor-plan resolution drift, plus performance trap "PDF report generation in the request thread" (PDFs run as River background jobs with downloadable artifact).

### Phase 6: Alerts + user management UI + operational hardening

**Rationale:** Alerts depend on canonical measurements (Phase 2) and aggregates/recent telemetry (Phases 4–5). Threshold and offline ship in v1; statistical anomaly is v1.x because it has a cold-start dependency on history (≥21 days). User management UI matures the auth surface from "admin can create users" to "admin can manage the org." Operational hardening (backup, log rotation, secrets, upgrade runbook, install kit) is what turns the build into a product we can deploy to a paying customer without 3am pages.

**Delivers:**
- Threshold alerts (per-meter / per-site, daily/hourly/instantaneous limits) and device-offline alerts (**N≥3 consecutive missed expected uplinks per profile, hysteresis grace, gateway-down suppresses descendant device alerts**).
- In-app alert center with unread badge, ack + notes, distinct categories/colors, snooze/mute states.
- User management UI: list, create, edit, disable (no hard-delete to preserve audit FKs); admin-managed credentials only (still no SMTP); session-revocation ("logout everywhere").
- Audit log UI (admin) with filter (date range, user, entity type) + CSV export; retention policy (default 12mo, configurable).
- **Backup script** (TimescaleDB-aware logical backup using `pg_dump`/single-threaded `pg_restore` — never `-j` on a hypertable; `rsync` of floor-plan image volume; configurable destination including S3-compatible URL); **restore procedure tested in CI**; settings page surfaces "last backup" age.
- Log rotation (`json-file` driver with `max-size`/`max-file`); secrets via Compose `secrets`; upgrade runbook with rollback procedure pinned per Shifter release; default retention windows (raw 90d, daily 5y, monthly 20y) parameterized.
- Documentation: install kit deliverable (single scripted operation per customer); central version register pattern.

**Addresses (FEATURES.md):** Threshold + offline alerts (in-app), alert center (ack+notes), user management, audit log UI, backup/restore, settings polish.

**Avoids (PITFALLS.md):** #4 false offline alerts (gateway-aware suppression + N≥3 + hysteresis), #15 ops gaps end-to-end, plus security mistakes around admin-only fields, default credentials, and rate-limited login.

### Phase 7: Multi-vendor breadth + v1.x differentiators + install validation polish

**Rationale:** Phase 7 closes the loop on the differentiators that depend on real-world install signal. The pre-seeded vendor profile catalog needs to know which vendors customers actually run — defer until Phases 1–6 have produced at least one signed customer's inventory. Statistical anomaly detection needs ≥21 days of per-meter history before it can fire without false positives. Webhook outbound is cheap once the alerts engine exists. Codec test-runner kills a documented operator pain point.

**Delivers:**
- Pre-seeded device profile catalog (Kamstrup MULTICAL 21, Diehl, Itron, Axioma, Sagemcom, Acrel, Schneider IEM3xxx, etc.) — curated profiles with codecs + canonical mappings; mirrors the TTN device repo pattern; ships as a versioned data file, not a backend dependency.
- Codec test-runner UI inside the profile editor: paste hex → see decoded JSON → see canonical mapping → diff against expected.
- Statistical anomaly / leak detection: P95-of-trailing-30-days threshold, IQR-based outliers, quiet-hour-flow rule. **Cold-start gate of ≥21 days hourly readings per metering point before activation**; first-24h-silent warmup for a fresh device.
- Webhook outbound (alerts + telemetry events): operator configures URL + signing secret in Settings; payload includes alert type + acknowledgement state; replaces SMTP for v1.x integrations (Slack/Discord/n8n/Home Assistant/ERP).
- Saved report templates; side-by-side meter/site comparison view; read-only telemetry REST API (token auth, internal use first); bulk gateway import.
- Install validation hardening: ChirpStack version probe on first connect (refuses v3 or unknown), Postgres+TimescaleDB extension probe, region/sub-plan probe against gateway profile.
- Additional normalizer mappings as new device families are encountered; "advanced view" rendering of `extra` JSONB iterates better.

**Addresses (FEATURES.md):** Pre-seeded profile library, codec test-runner, statistical anomaly detection, webhook outbound, saved reports, comparison view, read-only API, bulk gateway import.

**Avoids (PITFALLS.md):** Cold-start false-positive anomaly alerts; codec hell (catalog enforces "codec lives in ChirpStack profile, mapping in Shifter").

### Phase Ordering Rationale

- **Foundation (1) and Domain Model (2) are non-negotiable upstream.** Per PITFALLS.md, retrofitting metering-point indirection or the canonical measurement schema after launch is a multi-week migration. Per ARCHITECTURE.md, the metering-point abstraction and canonical measurement model are non-negotiable foundations. Both phases deliberately precede any user-visible reporting feature.
- **Provisioning (3) before Realtime + Dashboard (4)** so the binary can be loaded with realistic device fleets when realtime is wired. Bulk CSV import is what turns testing-with-3-devices into testing-with-300-devices.
- **Realtime (4) before Aggregates+Reports (5)** because realtime is independent of CAGGs and gives faster user feedback during development; CAGGs are then layered onto already-flowing telemetry rather than designed in a vacuum.
- **Aggregates+Reports+Map+Floor (5)** are clustered because they all consume canonical measurements and produce user-visible artifacts whose absence hurts the purchase justification together. Sequencing inside the phase: Settings (install identity) → Reports → CSV → Excel → PDF; floor-plan upload → device placement (normalized fractions) → map clustering.
- **Alerts + User Mgmt + Ops Hardening (6)** is the "ship to a paying customer" gate. Without backup/restore tested in CI, log rotation, secrets, and a documented upgrade runbook, Phase 1–5 is a demo, not a product.
- **Differentiators + Install Polish (7)** is deferred deliberately because pre-seeded profiles and statistical anomaly detection both *require real-world signal* (which vendors? how much history?) — building them earlier means designing on hypotheticals.

### Research Flags

Phases likely needing deeper research during planning (`/gsd-research-phase`):

- **Phase 2 (Domain model):** the rollover-detection logic, the offset-at-swap math under concurrent uplinks, and the `dev_eui→metering_point_id` resolver cache TTL semantics deserve a focused research dive — these are the highest-cost-to-retrofit decisions in the entire project. Synthetic-data harness design is also non-trivial.
- **Phase 3 (Bulk import):** CSV schema choice (TTS-compatible? Kamstrup-compatible?) is an open question that needs at least one real customer's existing inventory file as input.
- **Phase 5 (CAGG retention interplay):** the retention-policy + CAGG-refresh-policy interaction is the kind of footgun that benefits from a dedicated test plan and a documented invariant. Hierarchical CAGGs (CAGG-over-CAGG) are mature in TimescaleDB 2.26 but worth verifying for the daily→monthly→yearly chain.
- **Phase 6 (Backup/restore):** TimescaleDB-aware logical backup (no `pg_restore -j` on hypertables), correct order of operations relative to the running CS database (in bundled mode), and cross-version restore (vN backup onto vN+1 schema) need a concrete runbook + CI test.
- **Phase 7 (Statistical anomaly detection):** the cold-start rule, the per-meter baseline shape (P95 vs IQR vs time-of-day quiet check), and false-positive rates need empirical data from at least one Phase-1–6 install before this phase ships.

Phases with standard patterns (skip `/gsd-research-phase`):

- **Phase 1 (Foundation):** Go + chi + SCS + sqlc + pgx + golang-migrate + Cobra is a well-trodden boring-tech path; Compose-based dual-flavor deploy is documented.
- **Phase 4 (Realtime + dashboard):** SSE + `LISTEN`/`NOTIFY` + shadcn charts is well-documented; the architecture sketch in ARCHITECTURE.md is sufficient detail for execution.

### Schema Note for the Roadmap Author

PROJECT.md currently says "drop devices as **pixel-coordinate** points on [floor plans]" and lists "Pixel-coordinate device placement on floor plans" as a Key Decision. Per PITFALLS.md #11, this should be **normalized fractional coordinates (0..1)** to survive image replacement, Retina, and mobile DPR drift. The roadmap should treat the PROJECT.md wording as a documentation update during Phase 1 or 5; the schema decision itself is "fractions, not pixels" and is non-negotiable.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | All major decisions corroborated by official docs and 2026-current package state. Some library substitutions (Gin/Echo, Asynq) are equally defensible. |
| Features | HIGH (table stakes), MEDIUM (differentiator weighting) | Table stakes converged across four product categories. Differentiator weighting depends on real-customer signal. |
| Architecture | HIGH | Two-path (ingest/API), metering-point abstraction, canonical measurement, `LISTEN`/`NOTIFY`-driven SSE, hierarchical CAGGs, dual-mode ChirpStack — all corroborated. |
| Pitfalls | HIGH (LoRaWAN/ChirpStack/TimescaleDB), MEDIUM (utility billing edge cases), LOW (some scale thresholds) | Protocol-level pitfalls verified against official docs; ops/billing edge cases synthesized from forums and standards. |

**Overall confidence:** HIGH

### Gaps to Address

- **CSV schema for bulk import** — needs a real customer inventory file as input.
- **Realistic install cardinality** — default to "5k devices on a 4-vCPU VPS" as the v1 sizing target.
- **Pre-seeded vendor profile list** — defer Phase 7's profile catalog until at least one signed customer's inventory is in hand.
- **Anomaly detection cold-start window** — currently estimated at ≥21 days; needs empirical validation.
- **Cost / tariff display in v1** — not in v1 by default unless a billing-bias customer signs first.
- **PROJECT.md wording: "pixel-coordinate" → "normalized fractional"** — small but non-negotiable wording fix.
- **Timezone semantics** — operator-selectable per install, defaulted to system, displayed prominently on every report.

---
*Research completed: 2026-04-27*
*Ready for roadmap: yes*
