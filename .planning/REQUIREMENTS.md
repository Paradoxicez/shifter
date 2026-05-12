# Requirements: Shifter

**Defined:** 2026-04-27
**Core Value:** The operator runs their entire LoRaWAN water/electricity monitoring operation — provisioning, placement, monitoring, reporting — from Shifter alone, and meter swaps never break historical continuity.

## v1 Requirements

Requirements for initial release. Each maps to roadmap phases.

### Authentication

- [x] **AUTH-01**: User can log in with email and password
- [x] **AUTH-02**: User session persists across browser refresh and times out after configurable idle period
- [ ] **AUTH-03**: User is forced to change the default admin password on first login
- [x] **AUTH-04**: Failed login attempts are rate-limited to prevent brute force
- [x] **AUTH-05**: User can change their own password from their account menu
- [x] **AUTH-06**: System enforces two roles — admin (full control) and viewer (read-only) — across every API endpoint and every UI surface

### Install & Setup

- [x] **INST-01**: First-run install wizard guides operator through creating the initial admin account
- [x] **INST-02**: Install wizard captures install identity (display name, logo, address, timezone, units) used in reports and UI chrome
- [x] **INST-03**: Install wizard lets operator pick ChirpStack mode (bundled or external) and supplies gRPC URL + API token + MQTT URL accordingly
- [x] **INST-04**: Install wizard shows a regulator-aware LoRaWAN region picker (AS923 sub-plans for Thailand, EU868, US915, etc.) and persists the chosen default
- [x] **INST-05**: System refuses to connect to ChirpStack v3 — only ChirpStack v4 is supported, validated on first connect
- [x] **INST-06**: System exposes `/health` (public, no auth — `{status, version, uptime_seconds}` minimum) and `/health/detailed` (admin-required — DB connection state in Phase 1; Phase 6 expands with ChirpStack, MQTT, disk, and last-uplink-age). Reframed by Phase 1 D-18/D-19; the original single-endpoint-with-everything wording is replaced by this split.

### ChirpStack Integration

- [x] **CHIRP-01**: Backend connects to ChirpStack over gRPC for the control plane (devices, gateways, profiles, applications)
- [x] **CHIRP-02**: Backend subscribes to ChirpStack's MQTT integration topic for real-time device uplinks
- [x] **CHIRP-03**: A "Test connection" action verifies gRPC + MQTT reachability and surfaces a clear error path if either fails
- [x] **CHIRP-04**: Adding a device is a single user action — Shifter creates / reuses the underlying ChirpStack tenant, application, profile, and device binding behind the scenes
- [x] **CHIRP-05**: Any other multi-step ChirpStack flow (e.g. activation, key rotation) is collapsed to one user action; if multiple steps are unavoidable, they live inside one stepped dialog
- [x] **CHIRP-06**: Operator never needs to log into ChirpStack to do day-to-day work

### Gateway Management

- [x] **GW-01**: Admin can list, search, and filter gateways with online/offline status, last-seen, and lat/lng
- [x] **GW-02**: Admin can create / edit / delete a gateway via dialogs, with regulator-aware region picker
- [x] **GW-03**: User can view per-gateway RX/TX statistics (packet counts, success rate)
- [ ] **GW-04**: Gateways appear as pins on the map view with health indicators

### Devices & Meters

- [x] **DEV-01**: User can list, search, and filter devices with last-seen, battery, RSSI/SNR, and current site/metering-point assignment
- [x] **DEV-02**: Admin can create / edit / soft-delete a device via dialogs
- [x] **DEV-03**: Admin can paste a DevEUI from a vendor sticker — the parser handles both endiannesses and shows a preview before commit
- [x] **DEV-04**: System defaults to OTAA activation; ABP is supported but flagged as not recommended
- [ ] **DEV-05**: Admin can manage device profiles, including custom JS payload codec — codec runs inside ChirpStack's QuickJS sandbox, never inside Shifter
- [x] **DEV-06**: Admin can bulk-import devices from a CSV — phase 1 is dry-run validation with a per-row error report; phase 2 is commit-all with a per-row outcome log
- [x] **DEV-07**: A bulk import re-run with the same CSV is idempotent (no duplicates, no spurious creates)
- [x] **DEV-08**: System provides a downloadable CSV template for bulk import
- [x] **DEV-09**: Viewer cannot see secret fields like AppKey on any device record (server-side enforced, not just hidden in UI)

### Sites & Physical Layout

- [x] **SITE-01**: Admin can create / edit / delete sites via dialogs, with site lat/lng for the map view
- [x] **SITE-02**: A site supports both horizontal layouts (single-floor / campus) and vertical layouts (building with multiple floors)
- [x] **SITE-03**: Admin can upload one or more floor-plan images per site (PNG / JPG / PDF→PNG, with size cap and format whitelist)
- [x] **SITE-04**: Admin can drag and drop devices onto a floor plan; positions are stored as normalized fractions (x_frac, y_frac in [0, 1])
- [x] **SITE-05**: Floor plan view shows each placed device with a state-tinted marker (green / yellow / red) reflecting current health
- [x] **SITE-06**: User can navigate map → site → floor plan → device detail in a single click-through path

### Metering Point & Meter Swap (Domain Model)

- [x] **DATA-01**: Telemetry is keyed on a stable `metering_point_id`, never on `dev_eui` directly — this is the schema invariant
- [x] **DATA-02**: Each device-to-metering-point binding has a `valid_from` / `valid_to` window and a `reading_offset`
- [x] **DATA-03**: The hypertable's authoritative `time` column is server-side ingest time (with `gateway_rx_time` and device-side time persisted as diagnostics)
- [x] **DATA-04**: Admin can perform a meter swap via dialog: dialog captures the outgoing reading R, closes the active assignment, opens a new assignment, and proposes a `reading_offset` such that the displayed cumulative is continuous; admin confirms before commit
- [x] **DATA-05**: System detects counter rollovers (raw reading decreases between consecutive uplinks), advances the offset by the counter modulus, and logs the rollover event
- [x] **DATA-06**: Synthetic-data tests cover clean swap, swap with concurrent in-flight uplink, rollover, swap+rollover, and overlapping uplinks during swap
- [x] **DATA-07**: System persists each device's raw payload, decoded `object`, and the canonical normalized fields — none of the three is lost on the way to the hypertable

### Multi-Vendor Measurement Model

- [x] **DATA-08**: Measurements use a hybrid wide+JSONB schema — canonical first-class columns (cumulative, flow_rate, voltage, current, battery_pct, rssi, snr) plus a JSONB `extra` for vendor-specific parameters
- [x] **DATA-09**: Admin can map a device profile's decoded fields to canonical columns through a UI; new vendors require no backend deploy
- [x] **DATA-10**: System comes with at least one fully wired vendor profile (most-common water or electricity meter) and a documented path to add more

### Live Dashboard & Realtime

- [x] **DASH-01**: Dashboard adapts to install scope — water-only customers see water KPIs, electricity-only customers see electricity, both see both
- [x] **DASH-02**: Live KPIs include today's consumption, current flow / instantaneous draw, period delta, and online/offline device count
- [x] **DASH-03**: Dashboard receives real-time measurement updates via SSE driven off Postgres `LISTEN/NOTIFY` from a hypertable insert trigger — UI only ever sees persisted data
- [x] **DASH-04**: SSE client reconnects automatically with exponential backoff and jitter; reconnects deliver a fresh snapshot rather than relying on cached deltas
- [x] **DASH-05**: Dashboard renders time-series charts (shadcn-charts) with date-range pickers
- [x] **DASH-06**: Web UI is fully usable on mobile-sized viewports (responsive, no native app in v1)

### Per-Meter Detail

- [x] **DETL-01**: Per-meter detail page has zoned sections with a default "normal" view and a collapsible "advanced" view that exposes every parameter the device emits (full decoded JSONB)
- [x] **DETL-02**: Detail page shows the last 100–500 uplink events with timestamp, raw payload, decoded object, and signal stats
- [x] **DETL-03**: Detail page shows battery and RSSI/SNR sparklines

### Map View

- [x] **MAP-01**: Map view renders sites and gateways on OpenStreetMap tiles via Leaflet (or MapLibre as upgrade path)
- [x] **MAP-02**: Map clusters markers automatically when more than ~50 are in view and supports zoom-driven decluster
- [x] **MAP-03**: Clicking a site marker drills into the site's floor plan or device list
- [x] **MAP-04**: Map view does not depend on any paid external API (no Google Maps / Mapbox API key at install time)

### Reports & Exports

- [x] **REPT-01**: User can generate daily, monthly, and yearly consumption reports
- [x] **REPT-02**: User can generate reports per single meter and aggregate ("all meters", grouped by site or category)
- [x] **REPT-03**: User can export any report as CSV (UTF-8 BOM, ISO timestamps, timezone in header)
- [x] **REPT-04**: User can export any report as Excel with formatted dates, units, and totals
- [x] **REPT-05**: User can export any report as PDF, branded with install identity (display name / logo / address) from settings
- [x] **REPT-06**: PDF generation runs as a background job — request returns a downloadable artifact, not a synchronous response
- [x] **REPT-07**: Reports show period delta and percent change versus the previous period

### Continuous Aggregates (Backing Reports)

- [x] **DATA-11**: System maintains hierarchical TimescaleDB continuous aggregates (hourly → daily → monthly → yearly) over the measurement hypertable
- [x] **DATA-12**: CAGG refresh policies have `end_offset` ≥ 2× expected-interval to absorb late uplinks, and CAGG `start_offset` ≤ raw-data retention so aggregates are never silently wiped
- [x] **DATA-13**: Aggregate tables have their own (longer) retention policy — default raw 90 days, daily 5 years, monthly 20 years, all configurable in settings

### Alerts

- [x] **ALERT-01**: Admin can configure threshold alerts per meter or per site (daily/hourly/instantaneous limits, high and low)
- [x] **ALERT-02**: System raises a device-offline alert only when N≥3 consecutive expected uplinks are missed (per-profile expected interval), with a hysteresis grace period
- [x] **ALERT-03**: When a gateway is offline, device-offline alerts for devices behind that gateway are suppressed — one gateway-down alert, not 200 false device alerts
- [x] **ALERT-04**: System raises anomaly / leak-detection alerts using statistical rules (P95 of trailing 30 days, IQR outliers, quiet-hour flow); requires ≥21 days of history per metering point before activation, and silently warms up new meters for the first 24 hours
- [x] **ALERT-05**: In-app alert center shows unread badge, lets user acknowledge with notes, and supports snooze/mute states
- [x] **ALERT-06**: Alert center distinguishes categories (threshold, anomaly, offline) and severities by color

### Settings

- [x] **SETT-01**: Settings are organized into clear categories — install identity, ChirpStack connection, units, timezone, alerts, data retention, backup status
- [ ] **SETT-02**: Admin can update install identity at any time and changes propagate to report branding
- [x] **SETT-03**: Admin can update ChirpStack gRPC + MQTT credentials at any time without redeploying
- [x] **SETT-04**: Admin can configure data retention windows for raw measurements and each aggregate level
- [ ] **SETT-05**: Settings page surfaces the most recent backup timestamp and a warning if backup is older than the configured threshold

### User Management

- [x] **USER-01**: Admin can list, create, edit, and disable users via dialogs (no hard-delete — preserves audit trail)
- [x] **USER-02**: Admin can assign or change a user's role (admin / viewer)
- [x] **USER-03**: Admin can revoke all sessions for a user ("logout everywhere")
- [x] **USER-04**: Admin sets initial passwords inline (no SMTP dependency); user is forced to change on next login

### Audit Log

- [x] **AUDIT-01**: System records an audit entry for every create / update / delete and for every meter swap, capturing user, timestamp, entity, and before/after where relevant
- [ ] **AUDIT-02**: Admin can view the audit log with filters (date range, user, entity type)
- [ ] **AUDIT-03**: Admin can export the audit log as CSV

### Operational Hardening

- [x] **OPS-01**: Project ships two Docker Compose files — `bundled` (Postgres+TimescaleDB+Mosquitto+ChirpStack+Shifter) and `external` (Postgres+TimescaleDB+Shifter, ChirpStack URLs from env) — using the same backend image
- [ ] **OPS-02**: Project ships a TimescaleDB-aware logical backup script that backs up the database and rsyncs the floor-plan image volume
- [x] **OPS-03**: Backup destination is configurable (local filesystem path or S3-compatible URL)
- [ ] **OPS-04**: Restore procedure is tested in CI (round-trip backup → fresh DB → restore → smoke test)
- [ ] **OPS-05**: Container logs use the `json-file` driver with size and file-count caps configured by default
- [ ] **OPS-06**: All secrets (DB password, ChirpStack API token, session secret) are mounted via Docker Compose `secrets`, not passed via `.env`
- [ ] **OPS-07**: All container image tags are pinned (no `:latest`) — both in the bundle and in the external compose
- [ ] **OPS-08**: Project ships an upgrade runbook with rollback procedure pinned per Shifter release

### UX Conventions

- [x] **UX-01**: All create / edit / delete flows happen in dialogs — no separate full-page CRUD screens
- [x] **UX-02**: UI uses shadcn/ui components, blue/navy palette, modern minimal aesthetic, English-only copy
- [x] **UX-03**: Operator never sees ChirpStack-native terminology in the day-to-day UI (no "tenant", "application" — Shifter speaks in customer/site/device language)

## v2 Requirements

Deferred to future release. Tracked but not in current roadmap.

### Notifications & Email

- **V2-NOTIF-01**: SMTP delivery for password reset
- **V2-NOTIF-02**: Email alerts (threshold, offline, anomaly)
- **V2-NOTIF-03**: Scheduled email reports

### Outbound Integration

- **V2-INT-01**: Webhook outbound for alerts (Slack / Discord / n8n / Home Assistant / ERP)
- **V2-INT-02**: Public versioned REST/GraphQL telemetry API

### Authentication & Identity

- **V2-AUTH-01**: SSO / OAuth (Google, Azure AD, Okta, OIDC)
- **V2-AUTH-02**: Per-user dashboards / saved views
- **V2-AUTH-03**: Custom role bundles beyond admin/viewer

### Vendor Catalog

- **V2-VEND-01**: Pre-seeded vendor profile catalog (Kamstrup MULTICAL, Diehl, Itron, Axioma, Sagemcom, Acrel, Schneider IEM3xxx, etc.)
- **V2-VEND-02**: Codec test-runner UI inside profile editor (paste hex → see decoded JSON → see canonical mapping)
- **V2-VEND-03**: Saved report templates and side-by-side meter/site comparison view

### Backup & Operations

- **V2-OPS-01**: Scheduled backups to S3-compatible destinations (currently manual / scripted)

### Mobile & Internationalization

- **V2-MOB-01**: Native mobile / PWA application
- **V2-I18N-01**: Localization / multi-language UI

### Billing

- **V2-BILL-01**: Cost / tariff display and billing-cycle alignment

## Out of Scope

Explicitly excluded. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Multi-tenant SaaS in a single install | Each install serves one customer (single-tenant); we deploy per customer |
| Direct ChirpStack UI access for end-users | The whole product premise is operators only touch Shifter |
| ML-based predictive maintenance | Out of scope for v1 and v2 — defer until empirical signal justifies |
| Resident-facing portal | Integrate with existing portals if needed; do not build one |
| In-app commenting / messaging | Adds platform surface unrelated to monitoring |
| Generic widget builder | Dashboard is opinionated, not a Grafana clone |
| Real-coordinate (lat/lng) device placement on floor plans | Floor plans are pixel-based images; site-level uses lat/lng instead |
| Google Maps / Mapbox | OpenStreetMap (Leaflet/MapLibre) only; no paid map APIs at install time |
| In-app ChirpStack version upgrade | Out of scope; upgrades are a documented operator runbook step |

## Traceability

Which phases cover which requirements. Updated 2026-04-27 at roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| AUTH-01 | Phase 1 | Complete |
| AUTH-02 | Phase 1 | Complete |
| AUTH-03 | Phase 1 | Pending |
| AUTH-04 | Phase 1 | Complete |
| AUTH-05 | Phase 1 | Complete |
| AUTH-06 | Phase 1 | Complete |
| INST-01 | Phase 1 | Complete |
| INST-02 | Phase 1 | Complete |
| INST-03 | Phase 1 | Complete |
| INST-04 | Phase 1 | Complete |
| INST-05 | Phase 1 | Complete |
| INST-06 | Phase 1 | Complete |
| CHIRP-01 | Phase 1 | Complete |
| CHIRP-02 | Phase 1 | Complete |
| CHIRP-03 | Phase 1 | Complete |
| CHIRP-04 | Phase 2 | Complete |
| CHIRP-05 | Phase 3 | Complete |
| CHIRP-06 | Phase 3 | Complete |
| GW-01 | Phase 3 | Complete |
| GW-02 | Phase 3 | Complete |
| GW-03 | Phase 3 | Complete |
| GW-04 | Phase 3 | Pending |
| DEV-01 | Phase 3 | Complete |
| DEV-02 | Phase 3 | Complete |
| DEV-03 | Phase 3 | Complete |
| DEV-04 | Phase 3 | Complete |
| DEV-05 | Phase 2 | Pending |
| DEV-06 | Phase 3 | Complete |
| DEV-07 | Phase 3 | Complete |
| DEV-08 | Phase 3 | Complete |
| DEV-09 | Phase 3 | Complete |
| SITE-01 | Phase 2 | Complete |
| SITE-02 | Phase 5 | Complete |
| SITE-03 | Phase 5 | Complete |
| SITE-04 | Phase 5 | Complete |
| SITE-05 | Phase 5 | Complete |
| SITE-06 | Phase 5 | Complete |
| DATA-01 | Phase 2 | Complete |
| DATA-02 | Phase 2 | Complete |
| DATA-03 | Phase 2 | Complete |
| DATA-04 | Phase 2 | Complete |
| DATA-05 | Phase 2 | Complete |
| DATA-06 | Phase 2 | Complete |
| DATA-07 | Phase 2 | Complete |
| DATA-08 | Phase 2 | Complete |
| DATA-09 | Phase 2 | Complete |
| DATA-10 | Phase 2 | Complete |
| DATA-11 | Phase 5 | Complete |
| DATA-12 | Phase 5 | Complete |
| DATA-13 | Phase 5 | Complete |
| DASH-01 | Phase 4 | Complete |
| DASH-02 | Phase 4 | Complete |
| DASH-03 | Phase 4 | Complete |
| DASH-04 | Phase 4 | Complete |
| DASH-05 | Phase 4 | Complete |
| DASH-06 | Phase 4 | Complete |
| DETL-01 | Phase 4 | Complete |
| DETL-02 | Phase 4 | Complete |
| DETL-03 | Phase 4 | Complete |
| MAP-01 | Phase 5 | Complete |
| MAP-02 | Phase 5 | Complete |
| MAP-03 | Phase 5 | Complete |
| MAP-04 | Phase 5 | Complete |
| REPT-01 | Phase 5 | Complete |
| REPT-02 | Phase 5 | Complete |
| REPT-03 | Phase 5 | Complete |
| REPT-04 | Phase 5 | Complete |
| REPT-05 | Phase 5 | Complete |
| REPT-06 | Phase 5 | Complete |
| REPT-07 | Phase 5 | Complete |
| ALERT-01 | Phase 6 | Complete |
| ALERT-02 | Phase 6 | Complete |
| ALERT-03 | Phase 6 | Complete |
| ALERT-04 | Phase 6 | Complete |
| ALERT-05 | Phase 6 | Complete |
| ALERT-06 | Phase 6 | Complete |
| SETT-01 | Phase 6 | Complete |
| SETT-02 | Phase 6 | Pending |
| SETT-03 | Phase 6 | Complete |
| SETT-04 | Phase 5 | Complete |
| SETT-05 | Phase 6 | Pending |
| USER-01 | Phase 6 | Complete |
| USER-02 | Phase 6 | Complete |
| USER-03 | Phase 6 | Complete |
| USER-04 | Phase 6 | Complete |
| AUDIT-01 | Phase 2 | Complete |
| AUDIT-02 | Phase 6 | Pending |
| AUDIT-03 | Phase 6 | Pending |
| OPS-01 | Phase 1 | Complete |
| OPS-02 | Phase 6 | Pending |
| OPS-03 | Phase 6 | Complete |
| OPS-04 | Phase 6 | Pending |
| OPS-05 | Phase 6 | Pending |
| OPS-06 | Phase 6 | Pending |
| OPS-07 | Phase 6 | Pending |
| OPS-08 | Phase 6 | Pending |
| UX-01 | Phase 1 | Complete |
| UX-02 | Phase 1 | Complete |
| UX-03 | Phase 3 | Complete |

**Coverage:**
- v1 requirements: 99 total
- Mapped to phases: 99 (100%)
- Unmapped: 0

**Per-phase counts:**
- Phase 1 (Foundation): 18 — AUTH-01..06, INST-01..06, CHIRP-01..03, OPS-01, UX-01, UX-02
- Phase 2 (Domain Model & Canonical Schema): 14 — SITE-01, DATA-01..10, DEV-05, AUDIT-01, CHIRP-04
- Phase 3 (Provisioning): 15 — GW-01..04, DEV-01..04, DEV-06..09, CHIRP-05, CHIRP-06, UX-03
- Phase 4 (Realtime & Dashboard): 9 — DASH-01..06, DETL-01..03
- Phase 5 (Aggregates, Reports, Map & Floor Plans): 19 — SITE-02..06, MAP-01..04, REPT-01..07, DATA-11..13
- Phase 6 (Alerts, Users, Audit & Ops Hardening): 24 — ALERT-01..06, USER-01..04, AUDIT-02, AUDIT-03, SETT-01..05, OPS-02..08
- Phase 7 (Multi-Vendor Breadth & v1.x Differentiators): 0 v1 REQs (carries v1.x differentiators tracked under V2-VEND-01..03 and ALERT-04 anomaly tuning maturing on real-customer signal)

---
*Requirements defined: 2026-04-27*
*Last updated: 2026-05-11 — Phase 4 closure (Plan 04-10). All 9 Phase 4 requirements (DASH-01..06, DETL-01..03) verified Complete with shipping evidence after Plan 04-10 landed. Evidence trail per Phase 4 requirement:*
*- DASH-01 → `internal/db/migrations/0022_install_capabilities.up.sql` (capabilities CHECK) + `internal/dashboard/snapshot_handler_test.go::TestSnapshotHandler_WaterOnly + _ElectricityOnly` + `internal/dashboard/install_scope_handler_test.go` + `web/src/routes/dashboard.test.tsx::"capability=water hides electricity"` + `web/playwright/specs/dashboard-capability-filter.spec.ts`*
*- DASH-02 → `internal/db/migrations/0021_measurement_inserted_trigger.up.sql` (7-field NOTIFY payload) + `internal/events/trigger_test.go::TestMeasurementTrigger_PropagatesToChunks` + `internal/events/hub_test.go` + `internal/dashboard/snapshot_handler_test.go` + `web/src/components/dashboard/KpiCard.test.tsx` + `web/playwright/specs/dashboard-live-update.spec.ts`*
*- DASH-03 → `internal/events/handler.go` (SSE wire format: ready event + heartbeat) + `internal/events/handler_test.go` + `web/src/hooks/useSSE.test.ts::"backoff math"` + `web/playwright/specs/dashboard-live-update.spec.ts`*
*- DASH-04 → `web/src/hooks/useSSE.test.ts::"exponential backoff + jitter"` + snapshot-on-reconnect assertion + `Caddyfile` @sse flush_interval -1 (T-04-03 Pitfall §7 guard)*
*- DASH-05 → `web/src/lib/dateRange.test.ts` (computeRangeWindow mirrors D-12) + `web/src/components/dashboard/DateRangePicker.test.tsx` + `web/src/components/dashboard/ConsumptionChart.test.tsx` + `web/playwright/specs/dashboard-date-range.spec.ts`*
*- DASH-06 → `web/playwright/specs/dashboard-mobile.spec.ts` (375×667 viewport; grid-cols-1; no horizontal scroll) + KpiGrid `data-kpi-grid` attribute (Plan 04-10 wiring)*
*- DETL-01 → `web/src/components/metering-point/JsonTree.test.tsx` + `web/src/components/metering-point/AdvancedTab.test.tsx` + `web/playwright/specs/metering-point-detail.spec.ts`*
*- DETL-02 → `internal/meteringpoint/uplinks_handler_test.go` (cursor pagination, ≤500 cap) + `web/src/components/metering-point/UplinksLogTab.test.tsx::"500 row cap" + quality filter chips` + `web/playwright/specs/metering-point-detail.spec.ts`*
*- DETL-03 → `web/src/components/metering-point/SparklineTriplet.test.tsx` (battery/RSSI/SNR band colors) + `internal/meteringpoint/signal_handler_test.go` + `web/playwright/specs/metering-point-detail.spec.ts`*

*Phase 3 closure (2026-05-11) — Plan 03-10 reconciliation. All 14 actively-Complete Phase 3 requirements (GW-01..03, DEV-01..04, DEV-06..09, CHIRP-05, CHIRP-06, UX-03) verified Complete with shipping evidence; GW-04 ships partial (backend lat/lng + numeric inputs landed; map pin deferred to Phase 5 MAP-01..04). DEV-05 re-mapped to Phase 2 (delivered via `internal/profile/`). Evidence trail per Phase 3 requirement:* All 14 actively-Complete Phase 3 requirements (GW-01..03, DEV-01..04, DEV-06..09, CHIRP-05, CHIRP-06, UX-03) verified Complete with shipping evidence; GW-04 ships partial (backend lat/lng + numeric inputs landed; map pin deferred to Phase 5 MAP-01..04). DEV-05 re-mapped to Phase 2 (delivered via `internal/profile/`). Evidence trail per Phase 3 requirement:*
*- GW-01 → `internal/api/gateways_handler_test.go::TestListGatewaysHandler` + `web/src/routes/gateways/index.test.tsx::TestGatewaysList_Render` + Playwright `gateway-crud.spec.ts`*
*- GW-02 → `internal/api/gateways_handler_test.go::TestCreateGatewayHandler_RegionDefault` + `TestUpdateGatewayHandler` + `web/src/routes/gateways/add-gateway-dialog.test.tsx` + `decommission-gateway-dialog.test.tsx` + Playwright `gateway-crud.spec.ts`*
*- GW-03 → `internal/chirpstack/gateway_metrics_cache_test.go::TestMetricsCache_TTL + _SingleFlight` + `web/src/routes/gateways/index.test.tsx::TestGatewaysList_SparklineUsesCSSVars`*
*- GW-04 → **PARTIAL** — backend lat/lng columns landed + frontend numeric inputs + disabled "Pick on map" placeholder. Map pin (Leaflet/MapLibre integration) deferred to Phase 5 (MAP-01..04). Status remains Pending → Phase 5.*
*- DEV-01 → `internal/api/devices_list_test.go::TestListDevicesFiltered_*` + `web/src/routes/devices/search-params.test.ts::TestZodSchema_*` + `web/src/routes/devices/index.test.tsx` + Playwright `devices-filters-deeplink.spec.ts`*
*- DEV-02 → Phase 2 add device + Phase 3 `internal/api/devices_bulk_decommission_test.go::TestBulkDecommissionDevices_PartialSuccess` + `web/src/routes/devices/bulk-decommission-dialog.test.tsx`*
*- DEV-03 → Phase 2 `internal/device/deveui_test.go::TestParseDevEUI` (reused for gateway_id paste-parser; extended as `ParseEUI64` alias) + `web/src/routes/devices/deveui-parser.test.tsx`*
*- DEV-04 → `internal/api/devices_add_test.go::TestAddDevice_OTAA + TestAddDevice_ABP + TestAddDevice_ABP_FCntCarryOver` + `web/src/routes/devices/add-device-dialog.test.tsx::TestAddDevice_Step3_OTAA_DefaultSelected + TestAddDevice_Step3_ABPRadio_SwitchesStep4` + Playwright `device-add-otaa.spec.ts` + `device-add-abp.spec.ts`*
*- DEV-05 → Re-mapped to Phase 2; delivered via `internal/profile/` (editor + handlers + seed) + `web/src/routes/profiles/mapping-editor.test.tsx`. Phase 3 traceability table row stays Phase 2.*
*- DEV-06 → `internal/import/parser_xlsx_test.go + parser_csv_test.go + dryrun_test.go + commit_test.go + template_test.go` + `web/src/routes/devices/bulk-import-dialog.test.tsx` + `web/src/routes/admin/imports/index.test.tsx + $jobId.test.tsx` + Playwright `bulk-import.spec.ts`*
*- DEV-07 → `internal/import/commit_test.go::TestCommit_Idempotent` + Playwright `bulk-import.spec.ts` re-upload assertion (`already_exists` outcomes)*
*- DEV-08 → `internal/import/template_test.go::TestGenerateTemplate_Headers + _DevEUITextFormat + _ActivationDropdown + _RoundTrip`*
*- DEV-09 → `internal/api/devices_reveal_test.go::TestRevealSecrets_AdminOTAA + _AdminABP + _Viewer403 + _AuditNoSecretMaterial` + `web/src/routes/devices/reveal-keys-dialog.test.tsx::TestRevealKeys_403 + _NoCacheTime (gcTime:0)` + `web/src/routes/devices/$id.test.tsx::TestDeviceDetail_RevealButtonAdminOnly_Viewer` + Playwright `reveal-secrets-rbac.spec.ts` (viewer 403 + admin Cache-Control:no-store + admin 200)*
*- CHIRP-05 → 5-step Add Device dialog (Plan 03-07) + Bulk import 3-step dialog (Plan 03-09) + Add Gateway single dialog (Plan 03-08) + Reveal Keys dialog (Plan 03-10) — every multi-step ChirpStack flow collapsed to one user action*
*- CHIRP-06 → UX-03 vocabulary audit zero user-facing matches across `web/src/`; gateway management surface complete (Plan 03-08); reveal-keys + add-device flows mean operator never needs ChirpStack admin UI for daily work*
*- UX-03 → Vocabulary audit grep zero user-facing matches across `web/src/` (only doc comments + test assertions reference the forbidden words). Two Phase 3 leaks remediated in Plan 03-10 (`mapping-editor.tsx` 503 copy: "tenant not bootstrapped" → "not connected"; `add-device-dialog.tsx` ABP step 4 label: "Application Session Key" → "AppSKey").*

*Phase 2 closure (2026-05-04) — Plan 02-15 reconciliation. All 13 Phase 2 requirements (SITE-01, DATA-01..10, AUDIT-01, CHIRP-04) verified Complete with shipping evidence after gap-closure plans 02-11..14 landed. Evidence trail per requirement:*
*- SITE-01 → `internal/site/handlers_test.go` + `web/src/routes/sites/create-site-dialog.test.tsx`*
*- DATA-01 → `internal/db/migrations/0015_measurement.up.sql` invariant + `internal/resolver/cache_test.go` + `TestServe_FullBoot_MQTTUplinkPersists`*
*- DATA-02 → `internal/db/migrations/0014_binding.up.sql` btree_gist EXCLUDE + `internal/resolver/cache_test.go`*
*- DATA-03 → `internal/ingest/handler_test.go` + `TestServe_FullBoot_MQTTUplinkPersists` (server-time authoritative)*
*- DATA-04 → `internal/swap/math_test.go` + `internal/swap/commit_test.go` + `internal/swap/handlers_test.go` + `web/src/routes/metering-points/swap-meter-dialog.test.tsx`*
*- DATA-05 → `internal/swap/math_test.go` rollover detect + `internal/testharness/scenarios_test.go` Rollover scenarios*
*- DATA-06 → `internal/testharness/scenarios_test.go` (5 named scenarios all PASS, W4 per-test loop verified)*
*- DATA-07 → `internal/ingest/handler_test.go` + `TestServe_FullBoot_MQTTUplinkPersists`*
*- DATA-08 → `internal/db/measurements_test.go` hybrid schema + `internal/ingest/normalize_test.go`*
*- DATA-09 → `internal/profile/editor_test.go` + `internal/profile/handlers_test.go` + `web/src/routes/profiles/mapping-editor.test.tsx`*
*- DATA-10 → `internal/profile/seed_test.go` (3 seeded profiles) + `TestScenario_AxiomaW1_E2E`*
*- AUDIT-01 → `internal/audit/log_test.go` + `internal/audit/diff_test.go` + 13 same-tx call sites in production*
*- CHIRP-04 → `internal/device/handlers_test.go` + `web/src/routes/devices/add-device-dialog.test.tsx` + `TestServe_FullBoot_*` boot wiring*

*Phase 5 closure (2026-05-12) — Plan 05-12 reconciliation. All 19 Phase 5 requirements (SITE-02..06, MAP-01..04, REPT-01..07, DATA-11..13) plus SETT-04 (migrated from Phase 6) verified Complete with shipping evidence. Evidence trail per Phase 5 requirement:*
*- DATA-11 → `internal/db/migrations/0025_cagg_hourly.up.sql + 0026_cagg_daily.up.sql + 0027_cagg_monthly.up.sql + 0028_cagg_yearly.up.sql` (4-level CAGG hierarchy, CAGG-over-CAGG) + `internal/aggregate/aggregate_test.go::TestCAGGHierarchy + TestCAGGChain_DeltaCorrectness` (Plans 05-01 Wave 0 scaffolding + 05-02 CAGG hierarchy)*
*- DATA-12 → `internal/aggregate/aggregate_test.go::TestRefreshPolicyParams` (end_offset ≥ 2× expected_interval_s; start_offset ≤ raw_retention) + `internal/aggregate/aggregate_test.go::TestRetentionPolicy` (Plan 05-02)*
*- DATA-13 → `internal/db/migrations/0029_retention_config.up.sql` (retention_config table + D-09 defaults seeded in FinishSetup Serializable txn) + `internal/settings/retention_test.go::TestRetentionConfig` + `web/src/components/settings/DataRetentionCard.tsx` + `web/playwright/specs/retention-settings.spec.ts` (Plans 05-02 substrate + 05-11 Settings UI)*
*- MAP-01 → `web/src/components/map/MapView.tsx` (react-leaflet v5 + OSM tile URL) + `web/src/components/map/MapView.test.tsx::TestMapView_OSMTileURL + TestMapView_AutoFitBounds` + `web/playwright/specs/map-drill-down.spec.ts` (Plans 05-04 backend + 05-08 frontend)*
*- MAP-02 → `web/src/components/map/MapView.tsx` (react-leaflet-cluster clustering above ~50 markers) + `web/src/components/map/MapView.test.tsx::TestMapView_ClusterAbove50` + `web/playwright/specs/map-drill-down.spec.ts` (Plans 05-04 + 05-08)*
*- MAP-03 → `web/src/components/map/SitePopup.tsx` ("View site" link + "Get directions" OSM link) + `web/playwright/specs/map-drill-down.spec.ts::click site marker → popup → View site → /sites/:id` (Plans 05-04 + 05-08)*
*- MAP-04 → `web/src/components/map/MapView.tsx` (OSM tiles only; no paid API key in config) + `internal/map/handler_test.go::TestMapData_OSMTileURL` (Plans 05-04 + 05-08)*
*- REPT-01 → `internal/report/assembler.go` (daily/monthly/yearly scope via CAGG time_bucket queries) + `internal/report/handlers_test.go::TestGenerateHandler` + `web/playwright/specs/reports-generate.spec.ts` (Plans 05-03 backend + 05-09 frontend)*
*- REPT-02 → `internal/report/assembler.go` (scope picker: all/site/single + group-by utility class) + `internal/report/handlers_test.go::TestReportScopeGrouping` (Plans 05-03 + 05-09)*
*- REPT-03 → `internal/report/csv.go` (UTF-8 BOM \xEF\xBB\xBF + ISO-8601 timestamps + timezone header + comma separator) + `internal/report/csv_test.go::TestCSVFormat` (Plan 05-03)*
*- REPT-04 → `internal/report/excel.go` (excelize/v2 3-sheet: Summary/Period Detail/Meter Detail; bold totals row; formatted dates) + `internal/report/excel_test.go::TestExcelFormat` (Plan 05-03)*
*- REPT-05 → `internal/report/pdf.go` (maroto/v2 RegisterHeader logo+displayName+address; RegisterFooter generated-ts+timezone+page-X-of-Y) + `internal/report/pdf_test.go::TestPDFBranding` (Plan 05-06)*
*- REPT-06 → `internal/report/job.go` (River worker; enqueue in same tx as report record; StatusHandler returns pdf_status) + `internal/report/pdf_worker_test.go::TestPDFJob + TestEnqueuePDF_AtomicWithReport` + `web/src/routes/reports/useReportPDFStatus.ts` (Plans 05-06 + 05-09)*
*- REPT-07 → `internal/report/assembler.go` delta math (vs previous period always; YoY when ≥1 measurement in prior-year window; silent fallback per D-03) + `internal/report/delta_test.go::TestPeriodDelta` + `web/src/routes/reports/ReportPeriodTable.tsx` (D-03 YoY column hidden when absent) (Plans 05-03 + 05-09)*
*- SITE-02 → `internal/db/migrations/0032_floor_plan.up.sql` (multiple floor_plan rows per site for multi-floor; sort_order column) + `web/src/components/floor-plan/FloorPlanSelector.tsx` (Plans 05-05 schema + 05-10 frontend)*
*- SITE-03 → `internal/floorplan/image.go` (MIME sniff + dimension probe + 10MB cap + PNG/JPG/PDF→PNG allowlist) + `internal/floorplan/handlers_test.go::TestImageUpload + TestMultiFloor + TestReplaceKeepsPins` + `web/src/components/floor-plan/UploadFloorPlanDialog.tsx` (Plans 05-05 + 05-10)*
*- SITE-04 → `internal/db/migrations/0033_device_floor_plan_placement.up.sql` (x_frac/y_frac REAL clamped [0,1]) + `internal/floorplan/placement_test.go::TestPlacementCRUD + TestPlacement_Rejects + TestPlacement_Upsert` + `web/src/components/floor-plan/FloorPlanCanvas.tsx` (click→frac coords; drag) + `web/playwright/specs/floor-plan-pinning.spec.ts` (Plans 05-05 + 05-07 + 05-10)*
*- SITE-05 → `web/src/components/floor-plan/DevicePin.tsx` (green/yellow/red per D-22 thresholds) + `web/src/components/floor-plan/DevicePin.test.tsx::TestDevicePin_StateColors` + `web/playwright/specs/floor-plan-health.spec.ts` (Plans 05-07 + 05-10)*
*- SITE-06 → map → SitePopup "View site" → /sites/:id (D-23: Floor plan tab default when plans exist) → DevicePinPopover "Open device" → /devices/:id; 2-click path from map; verified by `web/playwright/specs/site-drill-through.spec.ts` (Plans 05-08 + 05-10)*
*- SETT-04 → `internal/db/migrations/0029_retention_config.up.sql` + `internal/settings/retention.go` (PATCH reconciles TimescaleDB policies in same tx) + `internal/settings/retention_test.go::TestRetentionConfig` + `web/src/components/settings/DataRetentionCard.tsx + EditRetentionDialog.tsx` + `web/playwright/specs/retention-settings.spec.ts` (Plan 05-11; migrated Phase 6 → Phase 5 per RESEARCH Open Q #4)*
*Phase 5 gap-closure (Plan 05-13, 2026-05-12) — corrected three migration filename citations in the Phase 5 evidence trail to match on-disk filenames. The implementations were always at the correct paths; only the documentation pointers were stale (off-by-one to off-by-three due to intervening 0030_report + 0031_audit_vocab_phase5 migrations). DATA-13 + SETT-04 now cite `0029_retention_config.up.sql`; SITE-02 cites `0032_floor_plan.up.sql`; SITE-04 cites `0033_device_floor_plan_placement.up.sql`.*
