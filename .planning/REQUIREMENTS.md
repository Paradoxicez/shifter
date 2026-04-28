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
- [ ] **INST-02**: Install wizard captures install identity (display name, logo, address, timezone, units) used in reports and UI chrome
- [ ] **INST-03**: Install wizard lets operator pick ChirpStack mode (bundled or external) and supplies gRPC URL + API token + MQTT URL accordingly
- [ ] **INST-04**: Install wizard shows a regulator-aware LoRaWAN region picker (AS923 sub-plans for Thailand, EU868, US915, etc.) and persists the chosen default
- [x] **INST-05**: System refuses to connect to ChirpStack v3 — only ChirpStack v4 is supported, validated on first connect
- [ ] **INST-06**: System exposes a `/health` endpoint reporting database, ChirpStack, MQTT, disk, last-uplink-age, and the running Shifter version

### ChirpStack Integration

- [x] **CHIRP-01**: Backend connects to ChirpStack over gRPC for the control plane (devices, gateways, profiles, applications)
- [ ] **CHIRP-02**: Backend subscribes to ChirpStack's MQTT integration topic for real-time device uplinks
- [ ] **CHIRP-03**: A "Test connection" action verifies gRPC + MQTT reachability and surfaces a clear error path if either fails
- [ ] **CHIRP-04**: Adding a device is a single user action — Shifter creates / reuses the underlying ChirpStack tenant, application, profile, and device binding behind the scenes
- [ ] **CHIRP-05**: Any other multi-step ChirpStack flow (e.g. activation, key rotation) is collapsed to one user action; if multiple steps are unavoidable, they live inside one stepped dialog
- [ ] **CHIRP-06**: Operator never needs to log into ChirpStack to do day-to-day work

### Gateway Management

- [ ] **GW-01**: Admin can list, search, and filter gateways with online/offline status, last-seen, and lat/lng
- [ ] **GW-02**: Admin can create / edit / delete a gateway via dialogs, with regulator-aware region picker
- [ ] **GW-03**: User can view per-gateway RX/TX statistics (packet counts, success rate)
- [ ] **GW-04**: Gateways appear as pins on the map view with health indicators

### Devices & Meters

- [ ] **DEV-01**: User can list, search, and filter devices with last-seen, battery, RSSI/SNR, and current site/metering-point assignment
- [ ] **DEV-02**: Admin can create / edit / soft-delete a device via dialogs
- [ ] **DEV-03**: Admin can paste a DevEUI from a vendor sticker — the parser handles both endiannesses and shows a preview before commit
- [ ] **DEV-04**: System defaults to OTAA activation; ABP is supported but flagged as not recommended
- [ ] **DEV-05**: Admin can manage device profiles, including custom JS payload codec — codec runs inside ChirpStack's QuickJS sandbox, never inside Shifter
- [ ] **DEV-06**: Admin can bulk-import devices from a CSV — phase 1 is dry-run validation with a per-row error report; phase 2 is commit-all with a per-row outcome log
- [ ] **DEV-07**: A bulk import re-run with the same CSV is idempotent (no duplicates, no spurious creates)
- [ ] **DEV-08**: System provides a downloadable CSV template for bulk import
- [ ] **DEV-09**: Viewer cannot see secret fields like AppKey on any device record (server-side enforced, not just hidden in UI)

### Sites & Physical Layout

- [ ] **SITE-01**: Admin can create / edit / delete sites via dialogs, with site lat/lng for the map view
- [ ] **SITE-02**: A site supports both horizontal layouts (single-floor / campus) and vertical layouts (building with multiple floors)
- [ ] **SITE-03**: Admin can upload one or more floor-plan images per site (PNG / JPG / PDF→PNG, with size cap and format whitelist)
- [ ] **SITE-04**: Admin can drag and drop devices onto a floor plan; positions are stored as normalized fractions (x_frac, y_frac in [0, 1])
- [ ] **SITE-05**: Floor plan view shows each placed device with a state-tinted marker (green / yellow / red) reflecting current health
- [ ] **SITE-06**: User can navigate map → site → floor plan → device detail in a single click-through path

### Metering Point & Meter Swap (Domain Model)

- [ ] **DATA-01**: Telemetry is keyed on a stable `metering_point_id`, never on `dev_eui` directly — this is the schema invariant
- [ ] **DATA-02**: Each device-to-metering-point binding has a `valid_from` / `valid_to` window and a `reading_offset`
- [ ] **DATA-03**: The hypertable's authoritative `time` column is server-side ingest time (with `gateway_rx_time` and device-side time persisted as diagnostics)
- [ ] **DATA-04**: Admin can perform a meter swap via dialog: dialog captures the outgoing reading R, closes the active assignment, opens a new assignment, and proposes a `reading_offset` such that the displayed cumulative is continuous; admin confirms before commit
- [ ] **DATA-05**: System detects counter rollovers (raw reading decreases between consecutive uplinks), advances the offset by the counter modulus, and logs the rollover event
- [ ] **DATA-06**: Synthetic-data tests cover clean swap, swap with concurrent in-flight uplink, rollover, swap+rollover, and overlapping uplinks during swap
- [ ] **DATA-07**: System persists each device's raw payload, decoded `object`, and the canonical normalized fields — none of the three is lost on the way to the hypertable

### Multi-Vendor Measurement Model

- [ ] **DATA-08**: Measurements use a hybrid wide+JSONB schema — canonical first-class columns (cumulative, flow_rate, voltage, current, battery_pct, rssi, snr) plus a JSONB `extra` for vendor-specific parameters
- [ ] **DATA-09**: Admin can map a device profile's decoded fields to canonical columns through a UI; new vendors require no backend deploy
- [ ] **DATA-10**: System comes with at least one fully wired vendor profile (most-common water or electricity meter) and a documented path to add more

### Live Dashboard & Realtime

- [ ] **DASH-01**: Dashboard adapts to install scope — water-only customers see water KPIs, electricity-only customers see electricity, both see both
- [ ] **DASH-02**: Live KPIs include today's consumption, current flow / instantaneous draw, period delta, and online/offline device count
- [ ] **DASH-03**: Dashboard receives real-time measurement updates via SSE driven off Postgres `LISTEN/NOTIFY` from a hypertable insert trigger — UI only ever sees persisted data
- [ ] **DASH-04**: SSE client reconnects automatically with exponential backoff and jitter; reconnects deliver a fresh snapshot rather than relying on cached deltas
- [ ] **DASH-05**: Dashboard renders time-series charts (shadcn-charts) with date-range pickers
- [ ] **DASH-06**: Web UI is fully usable on mobile-sized viewports (responsive, no native app in v1)

### Per-Meter Detail

- [ ] **DETL-01**: Per-meter detail page has zoned sections with a default "normal" view and a collapsible "advanced" view that exposes every parameter the device emits (full decoded JSONB)
- [ ] **DETL-02**: Detail page shows the last 100–500 uplink events with timestamp, raw payload, decoded object, and signal stats
- [ ] **DETL-03**: Detail page shows battery and RSSI/SNR sparklines

### Map View

- [ ] **MAP-01**: Map view renders sites and gateways on OpenStreetMap tiles via Leaflet (or MapLibre as upgrade path)
- [ ] **MAP-02**: Map clusters markers automatically when more than ~50 are in view and supports zoom-driven decluster
- [ ] **MAP-03**: Clicking a site marker drills into the site's floor plan or device list
- [ ] **MAP-04**: Map view does not depend on any paid external API (no Google Maps / Mapbox API key at install time)

### Reports & Exports

- [ ] **REPT-01**: User can generate daily, monthly, and yearly consumption reports
- [ ] **REPT-02**: User can generate reports per single meter and aggregate ("all meters", grouped by site or category)
- [ ] **REPT-03**: User can export any report as CSV (UTF-8 BOM, ISO timestamps, timezone in header)
- [ ] **REPT-04**: User can export any report as Excel with formatted dates, units, and totals
- [ ] **REPT-05**: User can export any report as PDF, branded with install identity (display name / logo / address) from settings
- [ ] **REPT-06**: PDF generation runs as a background job — request returns a downloadable artifact, not a synchronous response
- [ ] **REPT-07**: Reports show period delta and percent change versus the previous period

### Continuous Aggregates (Backing Reports)

- [ ] **DATA-11**: System maintains hierarchical TimescaleDB continuous aggregates (hourly → daily → monthly → yearly) over the measurement hypertable
- [ ] **DATA-12**: CAGG refresh policies have `end_offset` ≥ 2× expected-interval to absorb late uplinks, and CAGG `start_offset` ≤ raw-data retention so aggregates are never silently wiped
- [ ] **DATA-13**: Aggregate tables have their own (longer) retention policy — default raw 90 days, daily 5 years, monthly 20 years, all configurable in settings

### Alerts

- [ ] **ALERT-01**: Admin can configure threshold alerts per meter or per site (daily/hourly/instantaneous limits, high and low)
- [ ] **ALERT-02**: System raises a device-offline alert only when N≥3 consecutive expected uplinks are missed (per-profile expected interval), with a hysteresis grace period
- [ ] **ALERT-03**: When a gateway is offline, device-offline alerts for devices behind that gateway are suppressed — one gateway-down alert, not 200 false device alerts
- [ ] **ALERT-04**: System raises anomaly / leak-detection alerts using statistical rules (P95 of trailing 30 days, IQR outliers, quiet-hour flow); requires ≥21 days of history per metering point before activation, and silently warms up new meters for the first 24 hours
- [ ] **ALERT-05**: In-app alert center shows unread badge, lets user acknowledge with notes, and supports snooze/mute states
- [ ] **ALERT-06**: Alert center distinguishes categories (threshold, anomaly, offline) and severities by color

### Settings

- [ ] **SETT-01**: Settings are organized into clear categories — install identity, ChirpStack connection, units, timezone, alerts, data retention, backup status
- [ ] **SETT-02**: Admin can update install identity at any time and changes propagate to report branding
- [ ] **SETT-03**: Admin can update ChirpStack gRPC + MQTT credentials at any time without redeploying
- [ ] **SETT-04**: Admin can configure data retention windows for raw measurements and each aggregate level
- [ ] **SETT-05**: Settings page surfaces the most recent backup timestamp and a warning if backup is older than the configured threshold

### User Management

- [ ] **USER-01**: Admin can list, create, edit, and disable users via dialogs (no hard-delete — preserves audit trail)
- [ ] **USER-02**: Admin can assign or change a user's role (admin / viewer)
- [ ] **USER-03**: Admin can revoke all sessions for a user ("logout everywhere")
- [ ] **USER-04**: Admin sets initial passwords inline (no SMTP dependency); user is forced to change on next login

### Audit Log

- [ ] **AUDIT-01**: System records an audit entry for every create / update / delete and for every meter swap, capturing user, timestamp, entity, and before/after where relevant
- [ ] **AUDIT-02**: Admin can view the audit log with filters (date range, user, entity type)
- [ ] **AUDIT-03**: Admin can export the audit log as CSV

### Operational Hardening

- [x] **OPS-01**: Project ships two Docker Compose files — `bundled` (Postgres+TimescaleDB+Mosquitto+ChirpStack+Shifter) and `external` (Postgres+TimescaleDB+Shifter, ChirpStack URLs from env) — using the same backend image
- [ ] **OPS-02**: Project ships a TimescaleDB-aware logical backup script that backs up the database and rsyncs the floor-plan image volume
- [ ] **OPS-03**: Backup destination is configurable (local filesystem path or S3-compatible URL)
- [ ] **OPS-04**: Restore procedure is tested in CI (round-trip backup → fresh DB → restore → smoke test)
- [ ] **OPS-05**: Container logs use the `json-file` driver with size and file-count caps configured by default
- [ ] **OPS-06**: All secrets (DB password, ChirpStack API token, session secret) are mounted via Docker Compose `secrets`, not passed via `.env`
- [ ] **OPS-07**: All container image tags are pinned (no `:latest`) — both in the bundle and in the external compose
- [ ] **OPS-08**: Project ships an upgrade runbook with rollback procedure pinned per Shifter release

### UX Conventions

- [ ] **UX-01**: All create / edit / delete flows happen in dialogs — no separate full-page CRUD screens
- [x] **UX-02**: UI uses shadcn/ui components, blue/navy palette, modern minimal aesthetic, English-only copy
- [ ] **UX-03**: Operator never sees ChirpStack-native terminology in the day-to-day UI (no "tenant", "application" — Shifter speaks in customer/site/device language)

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
| INST-02 | Phase 1 | Pending |
| INST-03 | Phase 1 | Pending |
| INST-04 | Phase 1 | Pending |
| INST-05 | Phase 1 | Complete |
| INST-06 | Phase 1 | Pending |
| CHIRP-01 | Phase 1 | Complete |
| CHIRP-02 | Phase 1 | Pending |
| CHIRP-03 | Phase 1 | Pending |
| CHIRP-04 | Phase 2 | Pending |
| CHIRP-05 | Phase 3 | Pending |
| CHIRP-06 | Phase 3 | Pending |
| GW-01 | Phase 3 | Pending |
| GW-02 | Phase 3 | Pending |
| GW-03 | Phase 3 | Pending |
| GW-04 | Phase 3 | Pending |
| DEV-01 | Phase 3 | Pending |
| DEV-02 | Phase 3 | Pending |
| DEV-03 | Phase 3 | Pending |
| DEV-04 | Phase 3 | Pending |
| DEV-05 | Phase 3 | Pending |
| DEV-06 | Phase 3 | Pending |
| DEV-07 | Phase 3 | Pending |
| DEV-08 | Phase 3 | Pending |
| DEV-09 | Phase 3 | Pending |
| SITE-01 | Phase 2 | Pending |
| SITE-02 | Phase 5 | Pending |
| SITE-03 | Phase 5 | Pending |
| SITE-04 | Phase 5 | Pending |
| SITE-05 | Phase 5 | Pending |
| SITE-06 | Phase 5 | Pending |
| DATA-01 | Phase 2 | Pending |
| DATA-02 | Phase 2 | Pending |
| DATA-03 | Phase 2 | Pending |
| DATA-04 | Phase 2 | Pending |
| DATA-05 | Phase 2 | Pending |
| DATA-06 | Phase 2 | Pending |
| DATA-07 | Phase 2 | Pending |
| DATA-08 | Phase 2 | Pending |
| DATA-09 | Phase 2 | Pending |
| DATA-10 | Phase 2 | Pending |
| DATA-11 | Phase 5 | Pending |
| DATA-12 | Phase 5 | Pending |
| DATA-13 | Phase 5 | Pending |
| DASH-01 | Phase 4 | Pending |
| DASH-02 | Phase 4 | Pending |
| DASH-03 | Phase 4 | Pending |
| DASH-04 | Phase 4 | Pending |
| DASH-05 | Phase 4 | Pending |
| DASH-06 | Phase 4 | Pending |
| DETL-01 | Phase 4 | Pending |
| DETL-02 | Phase 4 | Pending |
| DETL-03 | Phase 4 | Pending |
| MAP-01 | Phase 5 | Pending |
| MAP-02 | Phase 5 | Pending |
| MAP-03 | Phase 5 | Pending |
| MAP-04 | Phase 5 | Pending |
| REPT-01 | Phase 5 | Pending |
| REPT-02 | Phase 5 | Pending |
| REPT-03 | Phase 5 | Pending |
| REPT-04 | Phase 5 | Pending |
| REPT-05 | Phase 5 | Pending |
| REPT-06 | Phase 5 | Pending |
| REPT-07 | Phase 5 | Pending |
| ALERT-01 | Phase 6 | Pending |
| ALERT-02 | Phase 6 | Pending |
| ALERT-03 | Phase 6 | Pending |
| ALERT-04 | Phase 6 | Pending |
| ALERT-05 | Phase 6 | Pending |
| ALERT-06 | Phase 6 | Pending |
| SETT-01 | Phase 6 | Pending |
| SETT-02 | Phase 6 | Pending |
| SETT-03 | Phase 6 | Pending |
| SETT-04 | Phase 6 | Pending |
| SETT-05 | Phase 6 | Pending |
| USER-01 | Phase 6 | Pending |
| USER-02 | Phase 6 | Pending |
| USER-03 | Phase 6 | Pending |
| USER-04 | Phase 6 | Pending |
| AUDIT-01 | Phase 2 | Pending |
| AUDIT-02 | Phase 6 | Pending |
| AUDIT-03 | Phase 6 | Pending |
| OPS-01 | Phase 1 | Complete |
| OPS-02 | Phase 6 | Pending |
| OPS-03 | Phase 6 | Pending |
| OPS-04 | Phase 6 | Pending |
| OPS-05 | Phase 6 | Pending |
| OPS-06 | Phase 6 | Pending |
| OPS-07 | Phase 6 | Pending |
| OPS-08 | Phase 6 | Pending |
| UX-01 | Phase 1 | Pending |
| UX-02 | Phase 1 | Complete |
| UX-03 | Phase 3 | Pending |

**Coverage:**
- v1 requirements: 99 total
- Mapped to phases: 99 (100%)
- Unmapped: 0

**Per-phase counts:**
- Phase 1 (Foundation): 18 — AUTH-01..06, INST-01..06, CHIRP-01..03, OPS-01, UX-01, UX-02
- Phase 2 (Domain Model & Canonical Schema): 13 — SITE-01, DATA-01..10, AUDIT-01, CHIRP-04
- Phase 3 (Provisioning): 16 — GW-01..04, DEV-01..09, CHIRP-05, CHIRP-06, UX-03
- Phase 4 (Realtime & Dashboard): 9 — DASH-01..06, DETL-01..03
- Phase 5 (Aggregates, Reports, Map & Floor Plans): 19 — SITE-02..06, MAP-01..04, REPT-01..07, DATA-11..13
- Phase 6 (Alerts, Users, Audit & Ops Hardening): 24 — ALERT-01..06, USER-01..04, AUDIT-02, AUDIT-03, SETT-01..05, OPS-02..08
- Phase 7 (Multi-Vendor Breadth & v1.x Differentiators): 0 v1 REQs (carries v1.x differentiators tracked under V2-VEND-01..03 and ALERT-04 anomaly tuning maturing on real-customer signal)

---
*Requirements defined: 2026-04-27*
*Last updated: 2026-04-27 after roadmap creation (traceability filled)*
