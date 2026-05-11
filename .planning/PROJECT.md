# Shifter

## What This Is

Shifter is a self-hosted water and electricity monitoring platform that wraps ChirpStack as its LoRaWAN backend. It gives each customer one app to manage gateways, sites, devices, and meters — replacing the need to ever touch ChirpStack directly — and presents real-time and historical consumption through a modern, minimal dashboard. Each install serves a single customer (single-tenant per install) and is deployed by us on the customer's infrastructure.

## Core Value

The operator runs their entire LoRaWAN water/electricity monitoring operation — provisioning, placement, monitoring, reporting — from Shifter alone, and meter swaps never break historical continuity.

## Requirements

### Validated

<!-- Shipped and confirmed valuable. -->

**Foundation (Phase 1, 2026-05-02 — runtime UAT pending in 01-HUMAN-UAT.md)**
- [x] Local-only authentication (email + password) — Argon2id, SCS+pgxstore sessions, per-IP+username rate limit
- [x] Two roles: admin and viewer — `Can(action, resource)` fail-closed authz
- [x] Connect to ChirpStack via gRPC (v4 only — refuses v3 at boot, wizard, and settings)
- [x] Support both deployment modes — `compose/bundled.yml` (8 services) + `compose/external.yml` (3 services), shared `shifter:0.1.0` image
- [x] All create / edit / delete flows in dialogs — ResponsiveDialog, ChangePasswordDialog, EditConnectionDialog
- [x] Modern minimal aesthetic in blue family on shadcn/ui — custom navy OKLCH theme, 21+ components, self-hosted Inter+JetBrains Mono
- [x] UI in English only

**Domain model & canonical schema (Phase 2, 2026-05-05 — runtime UAT pending in 02-HUMAN-UAT.md)**
- [x] Single-action Add Device dialog auto-creates ChirpStack tenant/application/profile/device behind the scenes (CHIRP-04 atomic transaction with full rollback)
- [x] Telemetry lands in TimescaleDB hypertable keyed by `metering_point_id` (never `dev_eui`) with server-side ingest time, raw payload, decoded object, and canonical normalized fields (DATA-01..03, DATA-08)
- [x] Meter-swap dialog reads outgoing reading R, closes active assignment, opens new, proposes `reading_offset` for cumulative continuity (DATA-04)
- [x] Counter rollovers auto-detected, advance offset by counter modulus, logged as device-health audit event (DATA-05)
- [x] Site CRUD with lat/lng (SITE-01); MP CRUD; Device CRUD; Profile editor (D-08 paste-JSON + click-leaf) — all dialog-driven
- [x] Synthetic-data test harness covers clean swap, swap+inflight uplink, rollover, swap+rollover, overlapping uplinks; one wired vendor profile (Axioma W1) produces correct canonical fields E2E (DATA-06, DATA-10)
- [x] Every state-changing action lands in audit_log inside the same transaction (AUDIT-01)
- [x] Vendor-agnostic measurement model — wide canonical columns + JSONB `raw` for vendor-specific debug fields (DATA-08, DATA-09)

**Provisioning — gateways, devices, bulk import (Phase 3, 2026-05-11)**
- [x] Manage gateways from Shifter (no need to log into ChirpStack) — full CRUD, archive/restore, atomic CS+PG transactions (GW-01, GW-02, GW-03)
- [x] Collapse multi-step ChirpStack flows into single app actions — Add Device dialog supports OTAA + ABP in a 5-step dialog that creates everything in one atomic transaction (DEV-02, DEV-04, CHIRP-05, CHIRP-06)
- [x] Bulk import devices (CSV / spreadsheet) — XLSX + CSV, dry-run preview, idempotent commit, downloadable template, errors export, 5000-row cap (DEV-07, DEV-08)
- [x] D-30 verbatim gateway decommission — PG soft-delete + CS DeleteGateway, atomic via tx ordering, best-effort CS re-create on PG failure, restore from archived_snapshot
- [x] Devices list with server-side filter / sort / pagination + bulk decommission + deeplinkable URL state via useSearchParams + zod (DEV-01, DEV-06)
- [x] Device-keys reveal endpoint admin-only + structurally hidden from viewers (DEV-09 defended at schema, JSON projection, and audit-row layers)
- [x] Operator-facing UI free of ChirpStack vocabulary leaks — "tenant / application / object" never reach the operator (UX-03)
- [x] GW-04 (map pin) PARTIAL: numeric lat/lng inputs ship; "Pick on map" disabled with v5 tooltip — full map UI deferred to Phase 5

**Realtime & dashboard (Phase 4, 2026-05-11 — runtime UAT pending in 04-HUMAN-UAT.md)**
- [x] Real-time updates from IoT devices (push to UI, no manual refresh) — Postgres LISTEN/NOTIFY → in-process Hub → SSE `/api/events` → `useSSE` hook with D-04 exponential backoff + jitter, Caddy `@sse` matcher with `flush_interval -1` (DASH-02, DASH-03)
- [x] Per-customer dashboard scoped to what they actually use — capability-gated `KpiGrid` driven by `/api/dashboard/scope`, omits inactive utility tiles entirely (DASH-01 D-09)
- [x] Per-meter detail view with two density levels — 3-tab page (Normal / Advanced / Uplinks log), Normal auto-updates from SSE while Advanced surfaces "Newer payload available" banner per D-20 forensic-safety rule, custom recursive `JsonTree` (no third-party JSON viewer) (DETL-01, DETL-02, DETL-03)
- [x] Date-range picker drives consumption charts with URL-state (mirrors Phase 3 D-15) — Recharts `AreaChart` with D-12 server-mirrored bucket schedule, D-13 live-mode pulse marker on Today/24h only (DASH-04, DASH-05)
- [x] Empty-state progressive onboarding when no devices yet — D-21 3-stage cards link to `/gateways` and `/devices`; live-channel banner reflects SSE connection state (DASH-06)

### Active

<!-- Current scope. Building toward these. -->

**Identity & access (Phase 6 finishes admin-managed users)**
- [ ] Admin can create / edit / disable users from inside the app

**Sites & physical layout (Phase 5)**
- [ ] Support both horizontal (campus / floor) and vertical (building with multiple floors) layouts
- [ ] Import floor plans / images and drop devices as normalized fractional points (x_frac, y_frac in [0, 1]) on them — resolution-independent, survives image replacement and Retina/mobile DPR
- [ ] Show all sites and their devices on a real-world map (OpenStreetMap via Leaflet or MapLibre) — also delivers GW-04 "Pick on map" for gateways

**Live data & dashboard**
- [ ] Map view of sites and devices

**Reporting**
- [ ] Daily, monthly, and yearly consumption summaries
- [ ] Both per-meter detail and aggregate ("all meters") summaries
- [ ] Export reports as CSV / Excel
- [ ] Export reports as PDF

**App behavior & feel**
- [ ] All create / edit / delete flows happen in dialogs (modal-first interaction model)
- [ ] Settings page with the workflow's settings split into clear categories
- [ ] UI in English only
- [ ] Mobile-responsive web (no native app in v1)
- [ ] Modern minimal aesthetic in a blue color family, built on shadcn/ui and the shadcn ecosystem (charts, layout, blocks) for speed and visual consistency
- [ ] The product should feel like Shifter — never like a re-skinned ChirpStack

**Operational**
- [ ] Threshold alerts (consumption above/below configured limits)
- [ ] Anomaly / leak detection alerts (unusual consumption patterns)
- [ ] Device-offline alerts

### Out of Scope

<!-- Explicit boundaries. Includes reasoning to prevent re-adding. -->

- **Multi-tenant SaaS in a single install** — Each install serves one customer (single-tenant); we deploy and maintain per customer
- **Native mobile app** — Mobile-responsive web only in v1; native app deferred
- **Non-English UI / i18n** — English-only; localization deferred until justified by demand
- **SSO / OAuth login** — Local accounts only in v1; SSO can be designed in later but is not built
- **SMTP / email delivery for password reset** — Local accounts manage credentials in-app; reduces install dependencies
- **Direct ChirpStack UI access for end-users** — Hard exclusion; the entire point is that operators only touch Shifter
- **Google Maps / Mapbox** — OpenStreetMap (Leaflet/MapLibre) only, since self-hosted installs should not depend on third-party paid map APIs
- **Real-coordinate (lat/lng) device placement on floor plans** — Floor-plan device positions are normalized fractions on the uploaded image; map-level placement uses site lat/lng instead

## Context

- **Domain:** LoRaWAN-based utility monitoring (water meters, electricity meters). ChirpStack is the de-facto open-source LoRaWAN Network Server and is the backend Shifter wraps.
- **Customer model:** Each customer gets their own install. Some customers monitor only water, some only electricity, some both — the dashboard must adapt to what they actually have.
- **Device fleet reality:** Vendors and meter models will mix and grow over time. Some meters report a single cumulative value; others report dozens of parameters. The data model must absorb this without code changes per vendor.
- **Lifecycle reality:** Physical meters fail and get swapped. The system's source of truth is the *metering point at a location*, not the physical device — otherwise every replacement breaks the customer's history.
- **Build philosophy:** Pick a strong, opinionated, fast-to-build stack. Lean on frameworks and shadcn-family libraries to write less custom code and stay consistent. Backend language is deferred to research (gRPC interop with ChirpStack and operational simplicity for self-hosted installs are the deciding factors).

## Constraints

- **Tech stack — backend ↔ ChirpStack:** Must communicate over gRPC (ChirpStack's recommended integration surface)
- **Tech stack — time-series:** TimescaleDB (Postgres extension) for telemetry — single database to deploy and back up, native daily/monthly/yearly continuous aggregates
- **Tech stack — frontend:** shadcn/ui plus the shadcn ecosystem (charts, layout/blocks); blue/navy palette; modern minimal
- **Tech stack — maps:** OpenStreetMap via Leaflet or MapLibre (no paid API keys at install time)
- **Deployment:** Self-hosted per customer; install must be a single, scripted operation we run for each customer; ChirpStack must be optionally bundleable (Docker Compose) so a customer with no existing LoRaWAN stack can be onboarded end-to-end
- **Interaction model:** All CRUD via dialogs; multi-step external flows (e.g. ChirpStack device activation) condensed into one user action or, at most, a stepped dialog
- **Tenancy:** Single-tenant per install — no cross-customer data isolation logic needed
- **Auth:** Local accounts only (email + password); no SMTP dependency in v1

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Single-tenant per install (not multi-tenant) | Each customer self-hosts; matches deployment model and keeps data model simple | — Pending |
| ChirpStack integration over gRPC | Recommended by ChirpStack; strongly typed, performant | — Pending |
| Support both bundled and external ChirpStack | Some customers already run ChirpStack; new customers want a one-shot install | — Pending |
| TimescaleDB for telemetry | One database to deploy/back up; native time-bucket aggregates for daily/monthly/yearly | — Pending |
| shadcn/ui for the entire frontend | Consistency, speed, large ecosystem (charts, blocks) — minimizes custom UI code | — Pending |
| Normalized fractional device placement on floor plans | Floor plans are uploaded images, not georeferenced; normalized fractions (0..1) survive image replacement, Retina, and mobile DPR drift | — Pending |
| OpenStreetMap (Leaflet/MapLibre) for map view | Self-hostable, no API key cost passed to customers | — Pending |
| Metering-point abstraction with device swap + offset | Physical meters get replaced; histories must survive replacement | — Pending |
| Local-only auth in v1 (no SMTP, no SSO) | Removes install dependencies; SSO can be added later without breaking the model | — Pending |
| English-only UI in v1 | Avoid i18n overhead until it's justified | — Pending |
| Backend language: defer to research | gRPC interop, single-binary deploy ergonomics, and team velocity all matter — research will recommend | — Pending |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd-transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd-complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-05-11 — Phase 4 (realtime & dashboard) complete. 10/10 plans shipped, automated verification passed (5/5 must-haves, 9/9 requirements), 4 real-stack UAT items pending in 04-HUMAN-UAT.md (SSE round-trip, Playwright fixture regen, Caddy in production, mobile reconnect).*
