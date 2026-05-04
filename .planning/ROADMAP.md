# Roadmap: Shifter

**Defined:** 2026-04-27
**Granularity:** standard
**Total Phases:** 7
**Coverage:** 99/99 v1 requirements mapped

## Core Value

The operator runs their entire LoRaWAN water/electricity monitoring operation — provisioning, placement, monitoring, reporting — from Shifter alone, and meter swaps never break historical continuity.

## Phases

- [ ] **Phase 1: Foundation** - Single Go binary, dual-channel ChirpStack integration, local auth, install wizard, two-flavor compose deploy
- [ ] **Phase 2: Domain Model & Canonical Schema** - Metering point + reading-offset + canonical measurement schema with end-to-end ingest path and meter-swap UI
- [ ] **Phase 3: Provisioning (Gateways, Devices, Bulk Import)** - Daily-driver provisioning surface so the binary runs at realistic fleet size
- [ ] **Phase 4: Realtime & Dashboard** - SSE-driven live updates, adaptive dashboard, per-meter detail
- [ ] **Phase 5: Aggregates, Reports, Map & Floor Plans** - Continuous aggregates, branded exports, OSM map, normalized fractional floor-plan placement
- [ ] **Phase 6: Alerts, Users, Audit & Operational Hardening** - The "ship to a paying customer" gate (alerts, user mgmt, audit UI, backup/restore, secrets, upgrade runbook)
- [ ] **Phase 7: Multi-Vendor Breadth & v1.x Differentiators** - Pre-seeded profile catalog, codec test-runner, statistical anomaly detection, install-validation polish

## Phase Details

### Phase 1: Foundation

**Goal**: Operator can install Shifter, sign in, and confirm Shifter is talking to ChirpStack — no telemetry yet, just a verified, scripted, secure starting point.

**Depends on**: Nothing (first phase)

**Requirements**: AUTH-01, AUTH-02, AUTH-03, AUTH-04, AUTH-05, AUTH-06, INST-01, INST-02, INST-03, INST-04, INST-05, INST-06, CHIRP-01, CHIRP-02, CHIRP-03, OPS-01, UX-01, UX-02

**Success Criteria** (what must be TRUE):
  1. Operator can run a single scripted install (bundled or external compose flavor) and reach the login screen on first boot.
  2. First-run wizard captures the initial admin user, ChirpStack mode + gRPC + MQTT credentials, regulator-aware region default (AS923 sub-plan picker for Thailand), and install identity (display name, logo, address, timezone, units), and refuses to connect to ChirpStack v3.
  3. Operator can log in with email + password, is forced to change the default admin password on first login, gets rate-limited on failed attempts, and can change their own password from the account menu.
  4. The "Test connection" action in Settings reports gRPC and MQTT reachability with a clear error path; the `/health` endpoint reports DB, ChirpStack, MQTT, disk, last-uplink-age, and the running version.
  5. The shell UI applies the shadcn/ui blue/navy aesthetic in English and uses dialogs for the few CRUD flows present (admin password change, settings edits) — establishing the modal-first convention for every later phase.

**Plans**: 24 plans

Plans:
- [x] 01-01-repo-scaffold-PLAN.md — Bootstrap the Go monorepo + Vite/React/Tailwind frontend skeleton + Justfile + Air (D-01..D-04)
- [x] 01-02-test-harness-PLAN.md — Wave 0: install testify+testcontainers-go+vitest, create all skeleton test files (VALIDATION.md scaffolding)
- [x] 01-03-database-layer-PLAN.md — pgxpool + golang-migrate-as-library + 6 initial migrations + sqlc config (D-13, D-16)
- [x] 01-04-config-secrets-PLAN.md — viper YAML+env config + Compose-secrets reader + slog JSON logger + version package (D-05, D-06, D-22, D-24)
- [x] 01-05-cobra-cli-PLAN.md — Cobra CLI: serve/migrate/version/create-admin/config-check/healthcheck (D-12, D-15)
- [x] 01-06-frontend-shell-PLAN.md — shadcn init (new-york + slate + custom navy OKLCH) + 21 components + ResponsiveDialog/StatusRow/Stepper/ThemeProvider + router shell + apiFetch (UX-02)
- [x] 01-07-argon2id-PLAN.md — Argon2id Hash/Verify (PHC encoding, OWASP m=19456 t=2 p=1) + password strength evaluator (AUTH-01)
- [x] 01-08-session-manager-PLAN.md — alexedwards/scs/v2 + pgxstore session manager with dev-mode Cookie.Secure toggle (AUTH-02, D-23)
- [x] 01-09-login-ratelimit-PLAN.md — Login + logout + change-password handlers + per-IP/per-username rate limit + create-admin body (AUTH-01, AUTH-03 reframed, AUTH-04, AUTH-05)
- [x] 01-10-authz-PLAN.md — Can(user, action, resource) + RequireAction middleware (PITFALLS §14, AUTH-06 server-side)
- [x] 01-11-account-ui-PLAN.md — RootLayout loader + AccountMenu + ChangePasswordDialog (AUTH-05 frontend, AUTH-06 frontend hiding)
- [x] 01-12-chirpstack-grpc-PLAN.md — ChirpStack gRPC Dial + ProbeVersion + bufconn mock (CHIRP-01, INST-05 sentinel)
- [x] 01-13-mqtt-subscriber-PLAN.md — paho.mqtt.golang subscriber with OnConnect re-subscribe + PingMQTT (CHIRP-02, supports CHIRP-03)
- [x] 01-14-install-middleware-PLAN.md — install_state Store (singleton CHECK id=1) + Regions catalog + FirstRunGate with cache (INST-01, D-08, PITFALL #10)
- [x] 01-15-install-handlers-PLAN.md — 5 wizard step handlers + atomic Serializable FinishSetup transaction (INST-01..05, D-10, D-11)
- [x] 01-16-install-wizard-ui-PLAN.md — 5-step wizard frontend; Thailand AS923-2 default; v3 destructive banner (INST-01..05, PITFALLS §8)
- [x] 01-17-test-connection-PLAN.md — Test Connection two-channel probe + Settings page + Edit Connection dialog + config-check probes (CHIRP-03, SETT-01, SETT-03, D-07)
- [x] 01-18-router-health-PLAN.md — chi router wiring all routes + middleware stack + /health[/detailed] + serve.go full body with INST-05 boot gate (D-18, D-19, INST-05/06)
- [x] 01-19-spa-embed-PLAN.md — //go:embed all:web/dist + history-mode SPA fallback handler (RESEARCH §Pattern 8)
- [x] 01-20-compose-bundled-PLAN.md — Bundled compose flavor (Postgres+Mosquitto+ChirpStack+Caddy+Shifter) + Dockerfile + install.sh (OPS-01)
- [x] 01-21-compose-external-PLAN.md — External compose flavor (Postgres+Caddy+Shifter only; CS+MQTT URLs via env) + install.sh (OPS-01)
- [x] 01-22-caddyfile-PLAN.md — Caddyfile with env-driven TLS modes (acme/byo/internal) + security headers + SSE-aware proxy (D-20..D-22, PITFALL #7)
- [x] 01-23-login-ui-PLAN.md — Login screen with verbatim UI-SPEC copy + 401/429 error mapping (AUTH-01, AUTH-04, UX-02)
- [x] 01-24-readme-docs-PLAN.md — README + docs/install.md + docs/operator-runbook.md + REQUIREMENTS.md INST-06 wording update (D-19)

**UI hint**: yes

### Phase 2: Domain Model & Canonical Schema

**Goal**: Telemetry flows end-to-end into a metering-point-keyed canonical schema, and the meter-swap differentiator works — proven by synthetic-data tests covering every swap and rollover edge case.

**Depends on**: Phase 1

**Requirements**: SITE-01, DATA-01, DATA-02, DATA-03, DATA-04, DATA-05, DATA-06, DATA-07, DATA-08, DATA-09, DATA-10, AUDIT-01, CHIRP-04

**Success Criteria** (what must be TRUE):
  1. Admin can create a site (with site lat/lng), create a metering point on that site, and add a device through a single one-action dialog that auto-creates the underlying ChirpStack tenant/application/profile/device behind the scenes.
  2. Real uplinks land in the `measurement` hypertable keyed by `metering_point_id` (never by `dev_eui`) with the server-side ingest time as the authoritative `time` column, and the row preserves raw payload, decoded `object`, and canonical normalized fields.
  3. Admin can perform a meter swap via dialog — the dialog reads the outgoing reading R, closes the active assignment, opens a new assignment, proposes a `reading_offset` for continuity, and the cumulative chart shows no discontinuity after admin confirms.
  4. Counter rollovers (raw reading decreases between consecutive uplinks) are auto-detected, advance the offset by the counter modulus, and are logged as a device-health event.
  5. Synthetic-data test harness passes for clean swap, swap with concurrent in-flight uplink, rollover, swap+rollover, and overlapping uplinks during swap; one fully-wired vendor profile (most-common water or electricity meter) produces correct canonical fields end-to-end and every state-changing action lands in the audit table.

**Plans**: 15 plans (10 original + 5 gap-closure)

Plans:
- [x] 02-01-PLAN.md — Wave 0 test scaffolding + tanstack table install
- [x] 02-02-PLAN.md — site / metering_point / device_profile / device migrations + sqlc baseline
- [x] 02-03-PLAN.md — resolver dev_eui→MP cache + LISTEN/NOTIFY listener
- [x] 02-04-PLAN.md — measurement hypertable + raw_payload/decoded_object persistence
- [x] 02-05-PLAN.md — chirpstack pkg gRPC wrappers (tenant/application/profile/device) + bootstrap
- [x] 02-06-PLAN.md — Phase 2 sqlc query suite (sites, MPs, devices, profiles, mappings, bindings, measurements, audit)
- [x] 02-07-PLAN.md — swap math + atomic CommitSwap + audit-in-tx + Invalidator interface
- [x] 02-08-PLAN.md — profile editor (programmatic API) + first-boot CS seed sync
- [x] 02-09-PLAN.md — ingest pipeline (decode + normalize + persist + handler) + SetUplinkHandler hook
- [x] 02-10-PLAN.md — site/MP/device CRUD HTTP handlers + DevEUI parser + CHIRP-04 atomic Add Device
- [x] 02-11-PLAN.md — gap closure: swap + profile HTTP route surface (Plan 02-11)
- [ ] 02-12-PLAN.md — gap closure: cmd/serve full Phase 2 boot wiring (formerly missing as Plan 02-15)
- [x] 02-13-PLAN.md — gap closure: DATA-06 synthetic test harness + `shifter test-harness` CLI
- [x] 02-14-PLAN.md — gap closure: frontend dialogs + list pages + App.tsx routing
- [ ] 02-15-PLAN.md — gap closure: REQUIREMENTS.md + VALIDATION.md status reconciliation

**UI hint**: yes

### Phase 3: Provisioning (Gateways, Devices, Bulk Import)

**Goal**: Operator can stand up a real customer fleet (50–500 meters) entirely from Shifter — gateways, devices, profiles, and a CSV import that is safe to re-run.

**Depends on**: Phase 2

**Requirements**: GW-01, GW-02, GW-03, GW-04, DEV-01, DEV-02, DEV-03, DEV-04, DEV-05, DEV-06, DEV-07, DEV-08, DEV-09, CHIRP-05, CHIRP-06, UX-03

**Success Criteria** (what must be TRUE):
  1. Admin can list, search, and filter gateways with online/offline status, last-seen, lat/lng, and per-gateway RX/TX statistics, and can create / edit / delete a gateway via dialogs with a regulator-aware region picker.
  2. Admin can list, search, and filter devices by site / status / vendor / type with last-seen + battery + RSSI/SNR + current metering-point assignment, soft-delete a device, and paste a DevEUI from a vendor sticker that the parser handles in either endianness with operator preview.
  3. Admin can manage device profiles (including custom JS payload codec that runs in ChirpStack's QuickJS sandbox, never in Shifter); OTAA is the default with an explicit "ABP not recommended" warning; viewers cannot see secret fields like AppKey on any device record (server-side enforced).
  4. Admin can bulk-import devices from a CSV with a two-phase flow — dry-run validation produces a per-row error report; commit phase produces a per-row outcome log; re-running the same CSV is idempotent (no duplicates, no spurious creates); a downloadable CSV template is provided.
  5. Throughout provisioning UI the operator never sees ChirpStack-native terminology ("tenant", "application") — Shifter speaks in customer/site/device language; multi-step ChirpStack flows are collapsed to one user action or a stepped dialog; every CRUD and every bulk-import row outcome lands in the audit log.

**Plans**: TBD
**UI hint**: yes

### Phase 4: Realtime & Dashboard

**Goal**: Operator opens Shifter and sees their fleet live — KPIs update without refresh, charts respond to date pickers, per-meter detail surfaces both the normal view and the full vendor firehose.

**Depends on**: Phase 3 (needs a realistic fleet to exercise SSE at scale)

**Requirements**: DASH-01, DASH-02, DASH-03, DASH-04, DASH-05, DASH-06, DETL-01, DETL-02, DETL-03

**Success Criteria** (what must be TRUE):
  1. Dashboard adapts to install scope — a water-only customer sees water KPIs, an electricity-only customer sees electricity, a mixed install sees both.
  2. Live KPIs (today's consumption, current flow / instantaneous draw, period delta, online/offline device count) update in the browser without manual refresh, driven by SSE backed by Postgres `LISTEN/NOTIFY` from a hypertable insert trigger — and the UI only ever reflects persisted data.
  3. The SSE client reconnects automatically with exponential backoff and jitter; reconnects deliver a fresh snapshot rather than relying on cached deltas; the dashboard remains usable on mobile-sized viewports.
  4. Per-meter detail page shows a default "normal" view (cumulative, instantaneous, last-update, alarms) and a collapsible "advanced" view that exposes every parameter the device emits via the JSONB `extra` column.
  5. Per-meter detail page shows the last 100–500 uplink events (timestamp, raw payload, decoded object, signal stats) plus battery and RSSI/SNR sparklines, and time-series charts on dashboard support date-range pickers.

**Plans**: TBD
**UI hint**: yes

### Phase 5: Aggregates, Reports, Map & Floor Plans

**Goal**: Reports become the purchase justification — daily/monthly/yearly summaries export as branded CSV/Excel/PDF; sites surface on the OSM map; devices drop onto floor plans without resolution drift.

**Depends on**: Phase 4 (telemetry must already be flowing through the canonical schema before CAGGs and reports key off it)

**Requirements**: SITE-02, SITE-03, SITE-04, SITE-05, SITE-06, MAP-01, MAP-02, MAP-03, MAP-04, REPT-01, REPT-02, REPT-03, REPT-04, REPT-05, REPT-06, REPT-07, DATA-11, DATA-12, DATA-13

**Success Criteria** (what must be TRUE):
  1. User can generate daily, monthly, and yearly consumption reports per single meter and aggregate ("all meters", grouped by site or category) with period-delta and percent-change versus the previous period; reports export as CSV (UTF-8 BOM, ISO timestamps, timezone in header), Excel (formatted dates / units / totals), and PDF (branded with install identity, generated as a background job that returns a downloadable artifact).
  2. Hierarchical TimescaleDB continuous aggregates (hourly → daily → monthly → yearly) back the reports with refresh policies whose `end_offset` ≥ 2× expected-interval and whose `start_offset` ≤ raw retention; aggregate tables have their own longer retention (default raw 90d, daily 5y, monthly 20y), all configurable in settings — so aggregates are never silently wiped.
  3. Map view renders sites and gateways on OpenStreetMap tiles via Leaflet with automatic marker clustering above ~50 markers and zoom-driven decluster — without depending on any paid external API.
  4. Admin can upload one or more floor-plan images per site (PNG/JPG/PDF→PNG with size cap and format whitelist), supporting both horizontal (campus / single-floor) and vertical (multi-floor building) layouts; admin drags devices onto a floor plan and positions are stored as normalized fractions (`x_frac`, `y_frac` ∈ [0, 1]) — resolution-independent, surviving image replacement and Retina/mobile DPR.
  5. Floor plan view shows each placed device with a state-tinted marker (green / yellow / red) reflecting current health; user can navigate map → site → floor plan → device detail in a single click-through path.

**Plans**: TBD
**UI hint**: yes

### Phase 6: Alerts, Users, Audit & Operational Hardening

**Goal**: Cross the line from demo to product — alerts fire correctly without false positives, the audit log is browsable, user management is mature, and operations (backup/restore, secrets, upgrades) are CI-tested and documented.

**Depends on**: Phase 5 (alerts need canonical measurements + aggregates + recent telemetry; ops hardening is the final gate before paying customers)

**Requirements**: ALERT-01, ALERT-02, ALERT-03, ALERT-04, ALERT-05, ALERT-06, USER-01, USER-02, USER-03, USER-04, AUDIT-02, AUDIT-03, SETT-01, SETT-02, SETT-03, SETT-04, SETT-05, OPS-02, OPS-03, OPS-04, OPS-05, OPS-06, OPS-07, OPS-08

**Success Criteria** (what must be TRUE):
  1. Admin can configure threshold alerts (per meter or per site, daily/hourly/instantaneous high/low limits) and device-offline alerts that only fire when N≥3 consecutive expected uplinks are missed (per-profile expected interval) with hysteresis grace; when a gateway is offline, device-offline alerts for devices behind it are suppressed (one gateway-down alert, not 200 false device alerts).
  2. Statistical anomaly / leak-detection alerts (P95 of trailing 30 days, IQR outliers, quiet-hour flow) ship in v1 but stay silent until the metering point has ≥21 days of history (cold-start gate), and silently warm up new meters for the first 24 hours — documented behavior, not a bug.
  3. In-app alert center shows an unread badge, distinguishes categories (threshold / anomaly / offline) and severities by color, lets admin acknowledge with notes, and supports snooze/mute states; admin can view the audit log with filters (date range, user, entity type) and export it as CSV.
  4. Admin can list, create, edit, and disable users via dialogs (no hard-delete — preserves audit trail), assign or change a user's role (admin / viewer), set initial passwords inline (no SMTP dependency, user forced to change on next login), and revoke all of a user's sessions ("logout everywhere"); settings are organized into clear categories — install identity, ChirpStack connection, units, timezone, alerts, data retention, backup status — and surfacing the most recent backup timestamp with a warning if older than threshold.
  5. The install kit ships a TimescaleDB-aware logical backup script (rsync of floor-plan volume + `pg_dump`, configurable to local path or S3-compatible URL), restore is round-trip tested in CI (backup → fresh DB → restore → smoke test), container logs use `json-file` with size and file-count caps, all secrets are mounted via Docker Compose `secrets` (not `.env`), every container image tag is pinned (no `:latest`), and the project ships a per-release upgrade runbook with rollback procedure.

**Plans**: TBD
**UI hint**: yes

### Phase 7: Multi-Vendor Breadth & v1.x Differentiators

**Goal**: Close the loop on differentiators that require real-customer signal — pre-seeded profile catalog, codec test-runner UI, additional vendor mappings, and install-validation polish — so vendor #2 / #3 / #N are admin-UI work, not backend deploys.

**Depends on**: Phase 6 (deferred until at least one signed customer's inventory and ≥21 days of history exist; statistical anomaly detection's empirical tuning lives here)

**Requirements**: (No primary v1 REQ owners — Phase 7 carries v1.x differentiators tracked under V2-VEND-01, V2-VEND-02, V2-VEND-03 and additional normalizer mappings as new device families are encountered. Phase 7 also delivers tuning for ALERT-04's anomaly thresholds and any install-validation hardening that surfaces during Phase 1–6 deployments.)

**Success Criteria** (what must be TRUE):
  1. Pre-seeded device profile catalog (Kamstrup MULTICAL, Diehl, Itron, Axioma, Sagemcom, Acrel, Schneider IEM3xxx, etc.) ships as a versioned data file — operator picks a vendor + model from the catalog and gets curated codec + canonical mapping without backend work.
  2. Codec test-runner UI inside the profile editor lets operator paste hex → see decoded JSON → see canonical mapping → diff against expected — closing a documented operator pain point.
  3. Side-by-side meter/site comparison view and saved report templates ship for power users; bulk gateway import lands alongside saved-template support.
  4. Statistical anomaly detection thresholds (P95/IQR/quiet-hour) are tuned against real-customer data from Phase 1–6 installs, and the cold-start gate behavior is documented and observable in the alert center.
  5. Install validation hardening: ChirpStack version probe on first connect (refuses v3 or unknown), Postgres + TimescaleDB extension probe, region/sub-plan probe against gateway profile — surfacing any drift from the install kit's expected baseline.

**Plans**: TBD
**UI hint**: yes

## Phase Dependencies

```
Phase 1 (Foundation)
    │
    ▼
Phase 2 (Domain Model & Canonical Schema)
    │
    ▼
Phase 3 (Provisioning)
    │
    ▼
Phase 4 (Realtime & Dashboard)
    │
    ▼
Phase 5 (Aggregates, Reports, Map & Floor Plans)
    │
    ▼
Phase 6 (Alerts, Users, Audit & Operational Hardening)
    │
    ▼
Phase 7 (Multi-Vendor Breadth & v1.x Differentiators)
```

Strictly linear dependency chain. The research is unambiguous: Foundation and Domain Model are non-negotiable upstream (retrofit cost is multi-week migrations); provisioning before realtime so the realtime path is exercised at fleet size; realtime before CAGGs because realtime is independent and gives faster feedback; CAGGs+reports+map+floor-plans cluster because they all consume canonical measurements; alerts + ops hardening is the "ship to a paying customer" gate; vendor breadth is deferred until real-customer signal exists.

## Progress

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Foundation | 24/24 | Complete | 2026-04-30 |
| 2. Domain Model & Canonical Schema | 6/10 | In Progress | - |
| 3. Provisioning (Gateways, Devices, Bulk Import) | 0/0 | Not started | - |
| 4. Realtime & Dashboard | 0/0 | Not started | - |
| 5. Aggregates, Reports, Map & Floor Plans | 0/0 | Not started | - |
| 6. Alerts, Users, Audit & Operational Hardening | 0/0 | Not started | - |
| 7. Multi-Vendor Breadth & v1.x Differentiators | 0/0 | Not started | - |

## Coverage Summary

**v1 Requirements Mapped:**
- AUTH-01..06 → Phase 1 (6)
- INST-01..06 → Phase 1 (6)
- CHIRP-01, CHIRP-02, CHIRP-03 → Phase 1 (3); CHIRP-04 → Phase 2 (1); CHIRP-05, CHIRP-06 → Phase 3 (2)
- OPS-01 → Phase 1 (1); OPS-02..08 → Phase 6 (7)
- UX-01, UX-02 → Phase 1 (2); UX-03 → Phase 3 (1)
- SITE-01 → Phase 2 (1); SITE-02..06 → Phase 5 (5)
- DATA-01..10 → Phase 2 (10); DATA-11..13 → Phase 5 (3)
- AUDIT-01 → Phase 2 (1); AUDIT-02, AUDIT-03 → Phase 6 (2)
- GW-01..04 → Phase 3 (4)
- DEV-01..09 → Phase 3 (9)
- DASH-01..06 → Phase 4 (6)
- DETL-01..03 → Phase 4 (3)
- REPT-01..07 → Phase 5 (7)
- MAP-01..04 → Phase 5 (4)
- ALERT-01..06 → Phase 6 (6)
- USER-01..04 → Phase 6 (4)
- SETT-01..05 → Phase 6 (5)

**Total v1 mapped:** 99 / 99 (100%)

**Phase 7** carries v1.x differentiators (V2-VEND-01..03 promoted by user request and additional vendor mappings discovered during Phase 1–6 deployments). No v1 REQ-IDs are orphaned to Phase 7 as primary owners — ALERT-04 is owned by Phase 6 with cold-start tuning maturing in Phase 7.

## Research Flags

Phases that should run `/gsd-research-phase` before planning:

- **Phase 2 (Domain Model)**: Rollover-detection logic, offset-at-swap math under concurrent uplinks, `dev_eui→metering_point_id` resolver cache TTL semantics, and synthetic-data harness design — these are the highest-cost-to-retrofit decisions in the project.
- **Phase 3 (Bulk Import)**: CSV schema choice (TTS-compatible? Kamstrup-compatible?) — needs at least one real customer's inventory file as input.
- **Phase 5 (CAGG Retention Interplay)**: Retention-policy + CAGG-refresh-policy interaction is a documented footgun; hierarchical CAGGs (CAGG-over-CAGG) need verification for the daily→monthly→yearly chain.
- **Phase 6 (Backup/Restore)**: TimescaleDB-aware logical backup ordering relative to bundled-mode ChirpStack DB, cross-version restore (vN backup → vN+1 schema) need a concrete runbook + CI test.
- **Phase 7 (Statistical Anomaly Detection)**: Cold-start rule, per-meter baseline shape (P95 vs IQR vs time-of-day quiet check), and false-positive rates need empirical data from at least one Phase 1–6 install before this phase ships.

Phases with standard patterns (skip `/gsd-research-phase`):

- **Phase 1 (Foundation)**: Go + chi + SCS + sqlc + pgx + golang-migrate + Cobra is well-trodden boring tech; Compose-based dual-flavor deploy is documented.
- **Phase 4 (Realtime + Dashboard)**: SSE + `LISTEN`/`NOTIFY` + shadcn charts is well-documented; ARCHITECTURE.md sketch is sufficient.

## Documentation Note

PROJECT.md currently lists "Pixel-coordinate device placement on floor plans" as a Key Decision. Per PITFALLS.md #11, this should be **normalized fractional coordinates (0..1)**. The schema decision is non-negotiable (fractions, not pixels); the PROJECT.md wording should be updated to "normalized fractional coordinates on the floor plan" during Phase 1 or Phase 5 — whichever lands the floor-plan storage code first.

---
*Roadmap created: 2026-04-27*
*Last updated: 2026-04-27*
