# Shifter

## What This Is

Shifter is a self-hosted water and electricity monitoring platform that wraps ChirpStack as its LoRaWAN backend. It gives each customer one app to manage gateways, sites, devices, and meters — replacing the need to ever touch ChirpStack directly — and presents real-time and historical consumption through a modern, minimal dashboard. Each install serves a single customer (single-tenant per install) and is deployed by us on the customer's infrastructure.

## Core Value

The operator runs their entire LoRaWAN water/electricity monitoring operation — provisioning, placement, monitoring, reporting — from Shifter alone, and meter swaps never break historical continuity.

## Requirements

### Validated

<!-- Shipped and confirmed valuable. -->

(None yet — ship to validate)

### Active

<!-- Current scope. Building toward these. -->

**Identity & access**
- [ ] Local-only authentication (email + password)
- [ ] Two roles: admin (full control) and viewer (read-only)
- [ ] Admin can create / edit / disable users from inside the app

**ChirpStack integration**
- [ ] Connect to ChirpStack via gRPC (per ChirpStack's recommendation)
- [ ] Support both deployment modes: bundled ChirpStack (Docker Compose) and external ChirpStack (gRPC URL + API key configured at install)
- [ ] Manage gateways from Shifter (no need to log into ChirpStack)
- [ ] Manage devices from Shifter (no need to log into ChirpStack)
- [ ] Collapse multi-step ChirpStack flows (e.g. device activation) into a single app action — or, if absolutely needed, into a stepped dialog — so the user never context-switches

**Sites & physical layout**
- [ ] Create and manage sites
- [ ] Support both horizontal (campus / floor) and vertical (building with multiple floors) layouts
- [ ] Import floor plans / images and drop devices as normalized fractional points (x_frac, y_frac in [0, 1]) on them — resolution-independent, survives image replacement and Retina/mobile DPR
- [ ] Show all sites and their devices on a real-world map (OpenStreetMap via Leaflet or MapLibre)

**Devices & meters**
- [ ] Bulk import devices (CSV / spreadsheet)
- [ ] Decouple meter from location: a "metering point" persists; the physical device behind it can be swapped without losing the historical data series
- [ ] When a meter is replaced, support an offset (or equivalent reading-continuity mechanism) so the displayed cumulative reading stays correct
- [ ] Support multiple device vendors with different parameter sets through a vendor-agnostic measurement model — ingest whatever the device sends and map it to canonical fields the UI knows how to render

**Live data & dashboard**
- [ ] Real-time updates from IoT devices (push to UI, no manual refresh)
- [ ] Per-customer dashboard scoped to what they actually use — water-only customers see a water dashboard; electricity-only see electricity; both see both
- [ ] Per-meter detail view with two density levels: a default "normal" view and a collapsible "advanced" view that exposes every parameter the device emits
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
*Last updated: 2026-04-27 after initialization*
