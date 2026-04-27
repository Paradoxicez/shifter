# Feature Research

**Domain:** LoRaWAN-based utility metering platform (water + electricity), self-hosted, single-tenant, ChirpStack-wrapping
**Researched:** 2026-04-27
**Confidence:** HIGH for table stakes (consensus across competitors); MEDIUM for differentiators (depends on customer profile); HIGH for anti-features (validated against PROJECT.md Out-of-Scope decisions)

---

## Domain Map

The Shifter feature space sits at the intersection of four established product categories. Each contributes its own table-stakes expectation set:

1. **LoRaWAN network servers / device managers** (ChirpStack, The Things Stack, Loriot, ThinkLink) — gateway + device + payload codec management.
2. **Meter Data Management Systems (MDMS)** (Genus, Utilismart, Bynry, Itron) — collection, validation, estimation, billing-grade exports for utilities.
3. **AMR/AMI utility dashboards** (Badger Meter, Kamstrup, Sensus, Core & Main, EKM) — water/electricity consumption monitoring, leak detection, threshold alerts.
4. **IoT visualization / submetering platforms** (ThingsBoard, Datacake, Akenza, Kaa, Sisense floor-plan, Setra, Triacta, Genea) — real-time dashboards, floor-plan placement, multi-tenant building submetering.

What users in 2026 expect from a "modern" utility monitoring product is the union of categories 1–4's table stakes, NOT each one's full feature set. Shifter's mandate is to deliver that union without reproducing ChirpStack's UX (anti-feature: re-skinned ChirpStack) or building MDMS-grade billing (anti-feature: full billing engine).

---

## Feature Landscape

### Table Stakes (Users Expect These)

Missing any of these makes Shifter feel incomplete or "less than ChirpStack." All are already in PROJECT.md Active or are strongly implied by it.

#### 1. Authentication & user management

| Feature | Why Expected | Complexity | v1/v2 | Notes |
|---------|--------------|------------|-------|-------|
| Email + password local login | Every operator-facing IoT tool has it; no install dependencies | LOW | v1 | Already in PROJECT.md. Hash with argon2id/bcrypt; rate-limit. |
| Two roles (admin / viewer) | Operators need read-only seats for site managers/auditors | LOW | v1 | Already in PROJECT.md. Permissions checked server-side, not just UI. |
| Admin-managed users (create / edit / disable) | Without SMTP, admin must manage credentials in-app | LOW | v1 | Already in PROJECT.md. Disable (don't hard-delete) so audit-log foreign keys survive. |
| Force-change-password on first login | Security expectation when admin sets initial password | LOW | v1 | Cheap to add, expected in 2026. |
| Session timeout / "logout everywhere" | Standard expectation; auditors ask | LOW | v1 | JWT with short TTL + refresh, or server-side sessions. |

#### 2. Gateway management

| Feature | Why Expected | Complexity | v1/v2 | Notes |
|---------|--------------|------------|-------|-------|
| List gateways with online/offline status | First thing an operator looks at when something is wrong | LOW | v1 | Pull from ChirpStack via gRPC; show last-seen timestamp. |
| Add / edit / delete gateway | Otherwise users go to ChirpStack — defeats purpose | LOW | v1 | Single-dialog; collapse ChirpStack tenant/application context. |
| Gateway location (lat/lng) on map | Users place gateways physically; want to see coverage | LOW | v1 | Re-uses the OSM map view from sites. |
| Gateway last-seen, RX/TX stats | Diagnose dead gateways before blaming the network | MEDIUM | v1 | ChirpStack exposes via gRPC `GetGatewayStats`. |
| Gateway frequency plan / region selection | One-time config but mandatory at install | LOW | v1 | Default to install-time region; show-but-rarely-edit. |

#### 3. Device & meter management

| Feature | Why Expected | Complexity | v1/v2 | Notes |
|---------|--------------|------------|-------|-------|
| List devices with last-seen + battery + RSSI/SNR | Standard troubleshooting view across every LoRaWAN tool | LOW | v1 | Surface from ChirpStack uplink metadata. |
| Add device (single, dialog) — DevEUI/AppKey/profile | Bread and butter | MEDIUM | v1 | Must collapse ChirpStack's app→profile→device flow into ONE dialog. |
| Bulk device import via CSV | Real deployments register 50–500 meters in one batch; manual entry is unusable | MEDIUM | v1 | Already in PROJECT.md. Match CSV to ChirpStack/TTS conventions (DevEUI, AppKey, profile name, optional location, label). Show preview + per-row validation before commit. |
| Device profiles (vendor model templates) | Different meter models have different keys/parameters/codecs; profiles are how every NS does it | MEDIUM | v1 | Wrap ChirpStack device profiles. Pre-seed with profiles for common models (see Differentiators). |
| Custom JavaScript codec per profile | Multi-vendor reality; ChirpStack v4 already supports this via QuickJS | MEDIUM | v1 | Expose codec editor with test-payload runner. |
| Decode and store canonical measurements | Without canonical fields, dashboards cannot render mixed-vendor fleets | HIGH | v1 | Already in PROJECT.md (vendor-agnostic measurement model). Mirrors ChirpStack v4's "Measurements" tab pattern. |
| Metering point abstraction (location-bound, device-swap-safe) | Physical meters fail; histories must survive replacement | HIGH | v1 | Already in PROJECT.md. This is the central data-model decision; see Architecture. |
| Meter swap with reading offset | Replacement meter starts at 0 (or some other reading); cumulative display must remain continuous | HIGH | v1 | Already in PROJECT.md. Store `offset = old_final_reading - new_initial_reading`; apply on read. Pattern matches Home Assistant utility_meter offset, EKM, Itron MDMS practice. |
| Device label / human-friendly name | Operators don't want to read DevEUIs | LOW | v1 | Free-text + searchable. |
| Disable / archive device (don't hard-delete) | Audit and historical data must survive removal | LOW | v1 | Soft-delete pattern. |
| Search / filter devices by site, status, vendor, type | At 200+ devices the list is unusable without filtering | LOW | v1 | Standard table UX. |

#### 4. Site / location management

| Feature | Why Expected | Complexity | v1/v2 | Notes |
|---------|--------------|------------|-------|-------|
| Create / edit / delete site | Sites are the organizing unit | LOW | v1 | Already in PROJECT.md. |
| Site lat/lng for map view | Without it the map is useless | LOW | v1 | Already in PROJECT.md. |
| Horizontal layout (campus / single floor) | Half the customer base is single-floor | MEDIUM | v1 | Already in PROJECT.md. One floor plan image, devices placed on it. |
| Vertical layout (multi-floor building) | Other half is multi-floor; without it submetering doesn't work | MEDIUM | v1 | Already in PROJECT.md. Multiple floor plans per site, navigate via floor selector. |
| Floor plan image upload | The whole point of indoor placement | MEDIUM | v1 | Already in PROJECT.md. PNG/JPG/PDF→PNG; resize/optimize on upload. |
| Pixel-coordinate device placement on floor plan | Floor plans aren't georeferenced | MEDIUM | v1 | Already in PROJECT.md. Drag-drop placement on uploaded image; matches Kaa/Sisense/Cumulocity/Akenza conventions. |
| Map view (OpenStreetMap) showing all sites + devices | Operators with multiple sites need a portfolio view | MEDIUM | v1 | Already in PROJECT.md. Leaflet or MapLibre, OSM tiles. Marker clustering above ~50 markers. |
| Click-through from map → site → floor plan | Users expect drill-down navigation | LOW | v1 | UX wiring; cheap once both views exist. |

#### 5. Vendor-agnostic device data model

| Feature | Why Expected | Complexity | v1/v2 | Notes |
|---------|--------------|------------|-------|-------|
| Canonical measurement schema (volume, energy, flow rate, pressure, voltage, current, power, etc.) | Without it, "all meters" reports are impossible | HIGH | v1 | Already in PROJECT.md. Canonical kinds with units: m³, kWh, L/min, bar, V, A, W. |
| Per-profile parameter→canonical mapping | The bridge from raw codec output to canonical fields | HIGH | v1 | Map declared per device profile, not per device. |
| Store both raw and canonical readings | Auditors want raw; users want canonical; no rework if mapping changes | MEDIUM | v1 | Two TimescaleDB hypertables OR one with both columns. |
| Unit conversion at display time | Customers may want gallons vs liters; v1 can lock to one unit per install | LOW | v1 | Pick install-default; allow override later. |

#### 6. Live dashboard / real-time updates

| Feature | Why Expected | Complexity | v1/v2 | Notes |
|---------|--------------|------------|-------|-------|
| Push updates to UI (WebSocket or SSE), no manual refresh | 2026 expectation; ThingsBoard/Datacake set the bar | MEDIUM | v1 | Already in PROJECT.md. SSE is simpler for one-way; WebSocket if bidirectional later. |
| Dashboard adapts to install scope (water-only / electricity-only / both) | Per PROJECT.md core value; obvious to users | MEDIUM | v1 | Already in PROJECT.md. Compute from canonical-measurement presence. |
| Headline KPIs (today's consumption, current flow, cost-per-day est.) | Standard energy dashboard pattern | MEDIUM | v1 | Aggregates over recent windows; recompute from continuous aggregate. |
| Current vs prior-period delta | "Up 12% vs last week" is the most-shared screenshot | MEDIUM | v1 | Standard dashboard pattern. |
| Online/offline device count card | First fault-finding signal | LOW | v1 | Counts from device-list query. |
| Time-series charts (line / bar) for selected meter or aggregate | Table stakes since the 2010s | MEDIUM | v1 | shadcn-charts wraps Recharts; works. |
| Date-range picker on every chart | Users always want to zoom in/out | LOW | v1 | shadcn date-range-picker. |

#### 7. Per-meter detail (normal + advanced)

| Feature | Why Expected | Complexity | v1/v2 | Notes |
|---------|--------------|------------|-------|-------|
| Per-meter detail page with current reading, last-24h chart, location | Standard drill-down | MEDIUM | v1 | Already in PROJECT.md. |
| Normal view (cumulative, instantaneous, last-update, alarms) | What 90% of users look at | LOW | v1 | Already in PROJECT.md. |
| Advanced view (every parameter the device emits, raw + decoded) | Auditors and field techs need it; missing = "less capable than ChirpStack" | MEDIUM | v1 | Already in PROJECT.md. Collapsible section. |
| Recent uplinks log (timestamp, RSSI, SNR, FCnt, payload) | Diagnostic table stakes; mirrors ChirpStack device events | MEDIUM | v1 | Last 100–500 events; deeper queryable in audit log. |
| Battery trend / signal trend small-charts | Predict failures; standard in TTS/ChirpStack | LOW | v1 | Sparkline widgets. |

#### 8. Reporting

| Feature | Why Expected | Complexity | v1/v2 | Notes |
|---------|--------------|------------|-------|-------|
| Daily, monthly, yearly consumption summary (per meter) | The reason customers buy this category of product | MEDIUM | v1 | Already in PROJECT.md. TimescaleDB continuous aggregates. |
| Aggregate "all meters" summaries (per site, per category) | Portfolio-level reporting expectation | MEDIUM | v1 | Already in PROJECT.md. Same aggregates rolled up. |
| Compare meters / sites side-by-side | Standard MDMS feature | MEDIUM | v2 | v1 can list-only; full comparison v1.x. |
| Per-period delta and % change | Same reason it's on the dashboard | LOW | v1 | Trivial once aggregates exist. |
| Saved report templates | Operators run the same report monthly | MEDIUM | v2 | Defer; first prove single-shot reports are right. |
| Scheduled / emailed reports | Common MDMS expectation | HIGH | v2 | **Conflicts with "no SMTP" v1 constraint.** Defer until SMTP arrives. |

#### 9. Export (CSV / Excel / PDF)

| Feature | Why Expected | Complexity | v1/v2 | Notes |
|---------|--------------|------------|-------|-------|
| CSV export of any report or telemetry range | Universal. If you can't export, finance won't accept the product | LOW | v1 | Already in PROJECT.md. UTF-8 BOM for Excel compat; ISO timestamps. |
| Excel (.xlsx) export with formatting | Finance prefers .xlsx over .csv; Excel is the actual reporting tool | MEDIUM | v1 | Already in PROJECT.md. Use a server-side library; format dates, units, totals. |
| PDF export of reports | Property managers attach to reports for owners/regulators | MEDIUM | v1 | Already in PROJECT.md. Server-rendered (Puppeteer / wkhtmltopdf / Typst). Include logo + customer info from settings. |
| Raw uplink payload export | Audit / debugging | LOW | v2 | Niche; available via audit log first. |

#### 10. Alerts

| Feature | Why Expected | Complexity | v1/v2 | Notes |
|---------|--------------|------------|-------|-------|
| Threshold alerts (consumption above/below limit) | Standard AMI feature | MEDIUM | v1 | Already in PROJECT.md. Per-meter or per-site thresholds; daily/hourly/instantaneous. |
| Device offline alert | Without this users only notice failures during reporting | LOW | v1 | Already in PROJECT.md. Triggered by last-seen > N minutes; configurable per profile. |
| Anomaly / leak detection alert (statistical baseline) | Selling-point feature for water; competitors lead with it | HIGH | v1 | Already in PROJECT.md. Start simple: "consumption > P95 of last 30 days" or "non-zero flow during quiet hours." Avoid ML in v1. |
| In-app alert center / unread badge | Without UI surface, alerts are invisible | LOW | v1 | List view + mark-read; persists across sessions. |
| Alert acknowledgement + notes | Operators track who handled what | LOW | v1 | Standard ITSM-lite pattern. |
| Alert delivery via email / SMS / webhook | The primary distribution channel for alerts in production | HIGH | v2 | **Conflicts with v1 "no SMTP."** v1 = in-app only; v2 adds webhook (cheap, no SMTP) then email when SMTP arrives. |

#### 11. Settings / configuration

| Feature | Why Expected | Complexity | v1/v2 | Notes |
|---------|--------------|------------|-------|-------|
| Categorized settings page | Already in PROJECT.md | LOW | v1 | Tabs/sections: Install, Users, Branding, Alerts, Integrations, Backup. |
| Customer / install identity (name, logo, address) — appears on PDFs | Without this, exports look generic | LOW | v1 | Single-tenant means "customer" = the install identity. |
| Default units (metric / imperial) | Region-dependent | LOW | v1 | Pick at install; per-user override is v2. |
| Timezone | Affects daily/monthly aggregates and exports | LOW | v1 | One per install; required. |
| ChirpStack connection (URL, API key, mode = bundled/external) | Already in PROJECT.md, must be reachable from settings for re-config | MEDIUM | v1 | Read-only display + "Test connection" button. |
| Map default center / zoom | Avoid every operator panning from London on first load | LOW | v1 | Cheap polish. |
| Currency for cost display (if cost shown) | Defer cost display to v2; if shipped, currency is required | LOW | v2 | Tied to cost feature. |

#### 12. Audit log

| Feature | Why Expected | Complexity | v1/v2 | Notes |
|---------|--------------|------------|-------|-------|
| Log who/what/when for every CRUD on devices, sites, users, settings | Standard expectation in any operator tool; ThingsBoard CE has this | MEDIUM | v1 | Append-only table; user_id, action, entity_type, entity_id, before/after, timestamp. |
| Searchable / filterable audit log UI (admin only) | Without UI it's just a table nobody reads | MEDIUM | v1 | Date range, user, entity-type filters. |
| Audit log export (CSV) | Compliance asks | LOW | v1 | Trivial once CSV export exists. |
| Audit log retention policy | Disk doesn't grow forever | LOW | v2 | Can hardcode 12 months in v1; configurable later. |

#### 13. Backup / restore

| Feature | Why Expected | Complexity | v1/v2 | Notes |
|---------|--------------|------------|-------|-------|
| One-shot backup (DB dump + uploads + ChirpStack config if bundled) | Self-hosted = the customer needs to back up; we ship the recipe | MEDIUM | v1 | Documented `shifter-backup` script + cron example. Single tar.gz output. |
| Restore from backup | Backup without restore is theatre | MEDIUM | v1 | Documented procedure + verified in CI. |
| Settings page surfaces "last backup" status | Operators forget; surfacing nudges them | LOW | v1 | Read a marker file or DB row; display age. |
| Scheduled / push-to-S3 backup | Every "real" deployment ends up here | MEDIUM | v2 | Plug-in points in v1 settings; implementation v2. |

#### 14. Onboarding flows that condense ChirpStack multi-step processes

| Feature | Why Expected | Complexity | v1/v2 | Notes |
|---------|--------------|------------|-------|-------|
| Install wizard (admin user, ChirpStack mode, map default, install identity) | First-run otherwise = blank app | MEDIUM | v1 | Already implied by PROJECT.md. Single-page wizard; can re-run. |
| Add-device dialog hides ChirpStack tenant/application/profile creation | Already in PROJECT.md as the headline UX promise | HIGH | v1 | Auto-create or pick-default for tenant/app; user only chooses profile + DevEUI + AppKey + label. |
| Add-gateway dialog hides ChirpStack tenant context | Same reason | MEDIUM | v1 | Auto-bind to install tenant. |
| Empty-state CTAs ("No sites yet — Add site") | UX table stakes for any list | LOW | v1 | shadcn empty-state pattern. |

---

### Differentiators (Competitive Advantage)

These are where Shifter beats ChirpStack-as-naked-NS, beats vendor-locked AMI portals, and beats generic IoT dashboards. They align with the PROJECT.md Core Value: "the operator runs their entire operation from Shifter alone, and meter swaps never break historical continuity."

| Feature | Value Proposition | Complexity | v1/v2 | Notes |
|---------|-------------------|------------|-------|-------|
| **Metering-point + offset = swap-safe history** | No competitor in this niche makes this their headline. Vendor AMI tools tie history to meter serial; ChirpStack ties it to DevEUI. Shifter's location-bound metering point survives both. | HIGH | v1 | Already in PROJECT.md. This is THE differentiator — name it explicitly in marketing copy. |
| **One-action ChirpStack onboarding** | ChirpStack's actual UX requires tenant→application→profile→device→multicast — a 5-step mental model. Shifter collapsing this is the second-biggest selling point. | HIGH | v1 | Already implied. Quantify in copy: "Add a meter in one dialog, not five screens." |
| **Vendor-agnostic canonical measurement model** | Most AMI tools are single-vendor. Most generic IoT tools punt on canonicalization. Shifter giving operators "all meters, mixed vendors, one chart" is differentiating in heterogeneous fleets (which is most fleets). | HIGH | v1 | Already in PROJECT.md. |
| **Pre-seeded profiles for common LoRaWAN water/electricity meters** | First-time-experience win. Operator picks "Kamstrup MULTICAL 21" from a dropdown; profile + codec + measurement-mapping appear pre-filled. | MEDIUM | v1.x | Curate 5–10 popular profiles (Kamstrup, Diehl, Itron, Axioma, Sagemcom, Acrel, Schneider IEM3xxx). Maintain as a community-style repo (mirror TTN device repo pattern). |
| **Dual-mode ChirpStack (bundled OR external)** | Competitors force you onto their stack. Shifter onboarding both greenfield customers (bundled Compose) and brownfield (existing CS install) is rare. | MEDIUM | v1 | Already in PROJECT.md. Selling point: "We don't make you replatform." |
| **Floor-plan device placement with real-time status overlay** | BMS tools have it; LoRaWAN-focused tools usually don't. With per-device color/state on the floor plan, operators see leaks geographically. | MEDIUM | v1 | Already in PROJECT.md. State-tinted markers (green/yellow/red) on top of pixel-coordinate placement. |
| **OSM-only maps (no API key)** | Self-hosted customers detest paying Mapbox/Google. "No third-party keys" is a procurement-friendly bullet. | LOW | v1 | Already in PROJECT.md. |
| **Single-binary or single-Compose deploy** | "We install in one command" beats "follow these 14 steps" (ChirpStack's bare install). | MEDIUM | v1 | Tie to STACK.md backend choice. |
| **Adaptive dashboard (water-only / electricity-only / both)** | Shows the product respects what the customer actually has. Vendor tools either hide irrelevant tiles awkwardly or force a generic layout. | MEDIUM | v1 | Already in PROJECT.md. |
| **Integrated audit log across the operator surface** | Most LoRaWAN NSes have weak or no audit; vendor MDMS have it but in a separate module. Shifter making audit a first-class settings tab is a procurement win. | MEDIUM | v1 | Listed under table stakes too — it is BOTH (table stakes if asked, differentiator if not). |
| **Modal-first interaction model + minimal blue aesthetic** | Pure UX. Modern operators rate this product over crowded multi-pane competitors. | LOW | v1 | Already in PROJECT.md. Stack-driven (shadcn). |
| **In-product backup that the customer trusts** | Self-hosted competitors hand-wave "use pg_dump." Shifter shipping a `shifter-backup` command and surfacing last-backup age in Settings is a real differentiator for the self-hosted segment. | MEDIUM | v1 | Listed under table stakes too. |
| **Codec test-runner inside profile editor** | Every payload-decoder pain point in TTS/ChirpStack threads ends with "I had to copy bytes into a Node REPL." Inline test-runner kills that pain. | MEDIUM | v1.x | Paste hex → see decoded JSON → see canonical mapping. |
| **Webhook-based outbound integration** (alerts + telemetry) | Lets customers feed Slack, Discord, n8n, Home Assistant, ERP without us building each integration | MEDIUM | v2 | Cheap once alerts exist; sidesteps the no-SMTP constraint. |
| **Continuity verification on meter swap** | UI that compares last reading (old meter) and first reading (new meter) and proposes the offset; admin confirms. Removes a class of bookkeeping mistakes. | MEDIUM | v1 | Direct extension of the swap+offset feature; without this UI, swap is error-prone. |

---

### Anti-Features (Commonly Requested, Often Problematic)

Each of these is something an operator, customer, or stakeholder will ask for at some point. Each has a written reason for refusal. Most are explicitly Out of Scope in PROJECT.md; the rest are derived from competitor patterns that don't fit Shifter's model.

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| **Multi-tenant SaaS in one install** | "Save us a server per customer" | PROJECT.md is explicit single-tenant. Going multi-tenant introduces row-level security, tenant routing, billing — all of which break our deployment model and our data simplicity claim | Deploy one install per customer (already the model) |
| **Native mobile apps (iOS/Android)** | "Field techs want an app" | Doubles surface area, adds app-store cycles, distracts from web. Mobile-responsive web covers field cases at <10% of the cost | Mobile-responsive web (PROJECT.md). PWA install-to-home-screen as v2 if data shows demand |
| **Localization / non-English UI** | International customers ask | i18n machinery slows every UI change; date/number/unit localization is a rabbit hole. Defer until at least one signed customer demands it | English v1; reconsider in v2 with concrete demand |
| **SSO / OAuth (Google/Azure/Okta)** | Enterprise customers ask | Each provider has gotchas; multi-tenant per IdP design adds models and tests. Our customers in v1 segment have <20 users — local accounts cover them | Local accounts v1; SSO designable in v2, v3 implementation |
| **SMTP / email delivery** (password reset, alert emails, scheduled reports) | "Send me an email when X" | Adds an external dependency that fails in customer environments (firewalled outbound 587, broken DNS, no MX) and increases support burden | Webhook outbound v2 (Slack/Discord/n8n cover the email use case via integration); SMTP v3 when justified |
| **Direct ChirpStack UI exposure** | "Power users want it" | Defeats the entire product premise. If we expose CS UI we're a portal not a product | Hard exclusion. Power users get our codec editor, audit log, and raw-payload export instead |
| **Google Maps / Mapbox** | Better tiles, better geocoding | Per-customer API key cost passed to us or them; self-hosted ethos broken | OpenStreetMap via Leaflet/MapLibre (PROJECT.md). For premium tiles a customer can BYO MapTiler key as v2 setting |
| **Real-coordinate (lat/lng) device placement on floor plans** | "Match GPS to building" | Floor plans aren't georeferenced; pretending they are creates subtle bugs (rotation, scale) for zero benefit indoors | Pixel-coordinates on floor plan; lat/lng at site/gateway level (PROJECT.md) |
| **Full billing / invoicing / payment processing** | "We charge tenants based on submeter readings" | This is a separate product (MDMS/billing system, e.g. Genea, Triacta). Re-implementing means tax rules, payment gateways, dispute workflows | Excellent CSV/Excel/PDF exports; integrate with existing billing via webhook (v2) or scheduled export drop |
| **Real-time control / downlink command UI** ("turn off this meter") | LoRaWAN supports downlink, why not? | 99% of utility meters don't accept control commands; the 1% that do (valve shut-off) carry safety/legal risk if mis-pushed from a dashboard | Provide raw downlink only via codec/profile (advanced flag), gated behind admin role + confirmation. Don't make it a primary UX flow |
| **In-app machine-learning anomaly detection** (deep learning leak detection) | "Modern AI water leak detection" research is hot | Models need training data we don't have; per-install drift; explainability is hard for operators | Statistical baselines (P95, IQR, time-of-day quiet check). Document the rules. v2 can ship a learned baseline once we have enough historical data per install |
| **Generic IoT dashboard / widget builder** (à la ThingsBoard) | "We want to add custom tiles" | Drag-drop dashboard editors are a product unto themselves; bug-magnets; turn into ChirpStack-equivalent UX clutter that PROJECT.md explicitly rejects | Curated, opinionated dashboard. v2 can add a small set of admin-toggleable widgets (cost-estimate tile, energy-mix tile) |
| **Automatic ChirpStack version upgrade UI** | "It would be nice if Shifter upgraded ChirpStack" | Cross-version data migrations and codec changes mean an in-app upgrade is a footgun | Document version pinning; manual upgrade procedure; bundled mode pins a specific CS version per Shifter release |
| **End-tenant / resident-facing portal** (residents log in to see their water bill) | Property managers ask | Different product (resident portal); auth, branding, payment, notification rules all change | Out of scope. Admin export → property mgmt's existing tenant portal |
| **Predictive maintenance / battery-life forecast as a primary feature** | "ML can predict meter failure" | Nice but third-rail in v1; easy to over-promise | v1: surface battery trend chart + threshold alert. v2: simple linear forecast. v3+: real model |
| **GIS / pipe-network topology** (water distribution graph, pressure zones) | Water utility advanced asks | This is a SCADA/GIS product (rezatec, Esri). Wholly different scale and ontology | Out of scope. Integration via export/webhook to GIS systems |
| **Built-in OAuth provider / API for third parties** | "Let our other systems pull data" | API design is a real commitment. v1 should expose minimal read-only API at most | v1: read-only REST endpoints for telemetry + entities, token-auth, internal-use first. Public API contract v2 |
| **Per-user dashboards / personalization** | "I want my favourite meters pinned" | Personalization adds a per-user state model that must round-trip through backup, audit, RBAC | Defer. Consistent install-wide dashboard in v1. Saved-views as v2 |
| **Forum / chat / commenting on alerts** | Some BMS competitors have it | Becomes a moderation problem and an SaaS-y bloat vector for a self-hosted operator tool | Per-alert "ack + notes" field is enough. Real comms happen in the customer's ticketing system |

---

## Feature Dependencies

```
[Auth (admin user + roles)]
    └──required-by──> everything else

[ChirpStack gRPC connection]
    └──required-by──> [Gateway mgmt], [Device mgmt], [Codec/profile mgmt], [Live telemetry]

[Sites]
    └──required-by──> [Floor plans], [Map view], [Aggregate reports per site], [Threshold alerts scoped per site]

[Floor plans]
    └──requires──> [Sites]
    └──required-by──> [Pixel-coordinate placement], [Floor-plan status overlay]

[Device profiles + codecs]
    └──requires──> [ChirpStack connection]
    └──required-by──> [Canonical measurement model], [Per-meter detail], [Reports], [Alerts]

[Canonical measurement model]
    └──requires──> [Device profiles + codecs]
    └──required-by──> [Adaptive dashboard], [Aggregate reports], [Anomaly detection], [Threshold alerts]

[Metering point abstraction]
    └──required-by──> [Meter swap + offset], [Reports that survive swaps], [Per-meter detail page identity]

[Meter swap + offset]
    └──requires──> [Metering point abstraction], [Audit log]  # swap event must be auditable

[TimescaleDB continuous aggregates]
    └──required-by──> [Daily/Monthly/Yearly reports], [Dashboard KPI tiles], [Threshold alerts]

[Real-time push (SSE/WebSocket)]
    └──requires──> [Telemetry ingestion pipeline]
    └──required-by──> [Live dashboard], [Alert center live updates]

[Threshold alerts]
    └──requires──> [Canonical measurements], [Aggregates or recent telemetry], [Alert center UI]

[Anomaly / leak alerts]
    └──requires──> [Canonical measurements], [≥2–4 weeks history per meter]   # cold-start caveat
    └──enhanced-by──> [Audit log] for explainability

[Device-offline alerts]
    └──requires──> [Last-seen tracking from ChirpStack uplink metadata]

[CSV / Excel / PDF export]
    └──requires──> [Reports]
    └──enhanced-by──> [Install identity in Settings] (logo, address on PDF)

[Audit log]
    └──required-by──> [Compliance], [Meter swap traceability], [User mgmt accountability]

[Backup / restore]
    └──requires──> [TimescaleDB dump strategy], [Uploads dir snapshot], [Settings + secrets handling]
    └──enhanced-by──> [ChirpStack bundled mode] (extends backup to CS DB)

[Bulk device import (CSV)]
    └──requires──> [Device profiles] (rows reference profiles by name)
    └──requires──> [ChirpStack connection] (creates devices via gRPC)
    └──enhanced-by──> [Sites] (rows can reference site for placement)

[Onboarding wizard]
    └──conflicts-with──> nothing, but must run before everything else on first boot
```

### Critical dependency notes for roadmap phasing

- **Metering point + offset is upstream of half the product.** Introducing it later means rewriting the device→reading data path. Land it in the first data-model phase.
- **Canonical measurement model is upstream of dashboard, reports, and alerts.** Same lesson — model it first, then build feature surfaces on top.
- **Real-time push and TimescaleDB aggregates are independent;** one supports live UI, the other supports historical UI. Both must exist before the dashboard feels complete, but they can be parallel work-streams.
- **Audit log should land before meter-swap UI ships.** Swaps are the highest-risk operation; without audit they're irreproducible.
- **Anomaly alerts have a cold-start dependency on history.** Don't ship them in the first phase that has telemetry — give the install at least 2–4 weeks of data, OR ship simple threshold-only first and layer anomaly later.
- **PDF export depends on install identity (logo/address) being settable.** Sequence Settings → Reports → PDF, not the other way.
- **Conflicts:** SMTP-dependent features (password-reset email, scheduled email reports, email alerts) all conflict with the v1 "no SMTP" constraint. They cluster naturally into a v2 "Notifications" phase.

---

## MVP Definition

### Launch With (v1)

The minimum that lets a real customer (water-only, electricity-only, or both) run their LoRaWAN fleet from Shifter alone with swap-safe history. Anything cut here breaks the Core Value claim.

- [ ] **Auth: email+password, admin/viewer roles, admin-managed users** — no product without auth
- [ ] **Install wizard** — first-run otherwise unusable
- [ ] **ChirpStack connection (bundled or external) via gRPC + "Test connection"** — backbone
- [ ] **Gateway list + add/edit/delete dialogs** — operators expect parity with ChirpStack here
- [ ] **Sites: create/edit, lat/lng, map view (OSM)** — organizing unit + portfolio view
- [ ] **Floor plans: upload + pixel-coordinate device placement (horizontal & vertical layouts)** — differentiator
- [ ] **Device profiles wrapping ChirpStack profiles, with custom JS codec editor** — can't onboard mixed vendors without this
- [ ] **Canonical measurement model + per-profile parameter mapping** — required for dashboard/reports/alerts to work across vendors
- [ ] **Metering point abstraction + meter swap with offset (with confirmation UI)** — THE differentiator
- [ ] **Add device dialog (one-action), bulk CSV import** — bread-and-butter
- [ ] **Live dashboard (real-time push), adaptive to install scope (water/electric/both)** — what users open every day
- [ ] **Per-meter detail with normal/advanced views and recent uplinks log** — drill-down expectation
- [ ] **Reports: daily/monthly/yearly, per-meter and aggregate, with date-range** — purchase justification
- [ ] **Export: CSV, Excel, PDF (with install identity branding)** — finance won't accept the product without
- [ ] **Threshold alerts + device-offline alerts (in-app alert center, ack + notes)** — basic operations
- [ ] **Audit log (admin) with filter + CSV export** — compliance and meter-swap traceability
- [ ] **Settings page (categorized): install identity, ChirpStack conn, units, timezone, alerts config, backup status** — central config surface
- [ ] **Backup script + restore procedure + last-backup display in settings** — self-hosted credibility

### Add After Validation (v1.x)

Land within 1–2 follow-up phases once the MVP is proving itself in real installs.

- [ ] **Pre-seeded profile library for common water/electric meters** — once we know which vendors customers actually run
- [ ] **Codec test-runner inside profile editor** — once first customer has hit "my codec is wrong" pain
- [ ] **Anomaly / leak detection (simple statistical baselines: P95, quiet-hour flow, IQR-based outliers)** — once we have ≥4 weeks of history per install
- [ ] **Webhook outbound for alerts + telemetry events** — Slack/n8n integrations; sidesteps no-SMTP
- [ ] **Saved report templates** — once operators run the same report monthly
- [ ] **Side-by-side meter/site comparison view** — natural extension of reports
- [ ] **Read-only telemetry REST API (token auth)** — first step toward integration story
- [ ] **Bulk gateway import** — only matters at multi-site scale

### Future Consideration (v2+)

Defer until product-market fit, scale, or specific signed customers demand them.

- [ ] **SMTP delivery (password reset + email alerts + scheduled reports)** — when customers consistently ask AND we accept the install dependency
- [ ] **SSO / OAuth (Google, Azure AD, Okta, generic OIDC)** — when an enterprise prospect requires it
- [ ] **Scheduled / push-to-S3 backups** — operationally nice; manual cron covers v1
- [ ] **Native mobile / PWA install-to-homescreen** — when field-tech feedback demands it
- [ ] **Cost / tariff display per meter** — table-stakes-adjacent for billing-aware customers; out of scope until after a billing-bias customer signs
- [ ] **Public, documented, versioned REST/GraphQL API** — when at least two integration customers exist
- [ ] **Localization / i18n** — when at least one signed customer requires non-English UI
- [ ] **Predictive battery / failure forecasting** — when training data per install is sufficient
- [ ] **Webhook inbound (third-party push to Shifter)** — speculative until a use case appears
- [ ] **Map tile BYO key (MapTiler, etc.)** — escape hatch for OSM-tile-quality complaints
- [ ] **Resident-facing portal** — out of scope; do not build, integrate instead

---

## Feature Prioritization Matrix

User Value × Implementation Cost. P1 = MVP must-have, P2 = v1.x, P3 = v2+.

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| Local auth + admin/viewer roles | HIGH | LOW | P1 |
| ChirpStack gRPC connection (bundled + external) | HIGH | MEDIUM | P1 |
| Gateway management (list + CRUD + status) | HIGH | LOW | P1 |
| Sites + map view (OSM) | HIGH | MEDIUM | P1 |
| Floor-plan upload + pixel device placement | HIGH | MEDIUM | P1 |
| Device profiles + JS codec | HIGH | MEDIUM | P1 |
| Canonical measurement model | HIGH | HIGH | P1 |
| Metering-point abstraction + meter-swap offset | HIGH | HIGH | P1 |
| One-action device add + CSV bulk import | HIGH | MEDIUM | P1 |
| Live dashboard with real-time push (SSE) | HIGH | MEDIUM | P1 |
| Per-meter detail (normal + advanced + uplinks log) | HIGH | MEDIUM | P1 |
| Reports (daily/monthly/yearly, per-meter + aggregate) | HIGH | MEDIUM | P1 |
| Export CSV / Excel / PDF | HIGH | MEDIUM | P1 |
| Threshold alerts + device-offline alerts (in-app) | HIGH | MEDIUM | P1 |
| Audit log + UI + CSV export | HIGH | MEDIUM | P1 |
| Settings (categorized) + install identity | HIGH | LOW | P1 |
| Backup script + restore procedure | HIGH | MEDIUM | P1 |
| Install / first-run wizard | HIGH | MEDIUM | P1 |
| Pre-seeded vendor profile library | MEDIUM | MEDIUM | P2 |
| Codec test-runner | MEDIUM | MEDIUM | P2 |
| Anomaly / leak detection (statistical) | HIGH | MEDIUM | P2 |
| Webhook outbound for alerts | MEDIUM | LOW | P2 |
| Saved report templates | MEDIUM | LOW | P2 |
| Side-by-side comparison view | MEDIUM | MEDIUM | P2 |
| Read-only telemetry REST API | MEDIUM | MEDIUM | P2 |
| Bulk gateway import | LOW | LOW | P2 |
| SMTP email delivery | MEDIUM | HIGH | P3 |
| SSO / OAuth | MEDIUM | HIGH | P3 |
| Scheduled / S3 backups | MEDIUM | MEDIUM | P3 |
| Cost / tariff display | LOW | MEDIUM | P3 |
| Localization / i18n | LOW | HIGH | P3 |
| ML-based anomaly detection | LOW | HIGH | P3 |
| Public versioned API | LOW | HIGH | P3 |

---

## Competitor Feature Analysis

Quick map of how the four reference categories handle the features Shifter cares most about. Helps justify Shifter's deliberate choices.

| Feature | ChirpStack (NS) | The Things Stack | ThingsBoard CE/PE | Datacake | Vendor AMI (Kamstrup/Badger/Itron) | Submetering (Genea/Triacta/Setra) | Shifter approach |
|---------|-----------------|------------------|-------------------|----------|------------------------------------|------------------------------------|------------------|
| LoRaWAN-native (gateway, profile, codec, multicast) | Yes — best-in-class | Yes | Via integration | Via integration | Closed/proprietary | Closed/proprietary | Wrap ChirpStack |
| Multi-vendor canonical measurement model | Partial (Measurements tab v4) | Partial | Yes (telemetry keys, but per-device) | Yes | Single-vendor | Limited | First-class, profile-driven mapping |
| Metering-point abstraction (swap-safe history) | No | No | No (device = identity) | No (device = identity) | Yes (within their fleet) | Yes (within their fleet) | First-class — headline differentiator |
| Floor-plan device placement | No | No | Yes (image widget, manual) | Yes (image widget, manual) | Sometimes | Often | Built-in, pixel-coords, status overlay |
| OpenStreetMap default | N/A | N/A | Yes | Yes | N/A | N/A | Yes (no API key) |
| Live push to UI | No (admin UI) | Limited | Yes (WS) | Yes | Web portal (varies) | Web portal (varies) | Yes (SSE) |
| Per-period reports + aggregate | No (NS only) | No (NS only) | DIY via dashboards | Yes (limited) | Yes | Yes | Yes (TimescaleDB continuous aggregates) |
| CSV / Excel / PDF export | Limited | Limited | CE limited; PE yes | Yes | Yes | Yes | All three, branded with install identity |
| Threshold alerts | Via integration | Via integration | Yes (rule chain) | Yes | Yes | Yes | Yes, in-app v1 |
| Anomaly / leak detection | No | No | Rule chain DIY | Limited | Yes (vendor-specific) | Some | Statistical v1.x; no ML in v1 |
| Audit log | No | Limited | Yes | Limited | Yes | Often | First-class admin tab |
| Backup / restore (turnkey) | No (DIY pg_dump) | Cloud only | DIY | SaaS | Vendor SaaS | SaaS | First-class shipped script |
| Multi-tenant per install | Yes | Yes | Yes | Yes | Yes | Yes | **No — single-tenant by design** |
| SSO | PE | Yes | PE | Yes | Yes | Yes | **No v1** |
| Native mobile | No | No | Yes (PE) | No | Sometimes | Sometimes | **No — responsive web** |
| One-action device onboarding | **No (multi-step)** | **No (multi-step)** | Generic, not LoRaWAN | Decent | N/A | N/A | **Yes — primary UX selling point** |

**Reading:** Shifter doesn't try to beat ChirpStack at being a network server (it wraps it), doesn't try to beat ThingsBoard at being a generic IoT platform (it stays opinionated), doesn't try to beat Kamstrup AMI at vendor-specific features (it stays vendor-agnostic), and doesn't try to beat Genea at billing (it exports to billing). It wins at the operator-experience seam between all four.

---

## Open Questions for Roadmap

These surfaced during research and deserve a phase or a follow-up research dive:

1. **What pre-seeded device profiles do real customers need first?** Need a customer-discovery list of 5–10 dominant water + electricity LoRaWAN meter models in the target market. Until this is known, "pre-seeded profiles" is theoretical.
2. **What is the realistic cardinality?** 50 devices? 5,000? Affects whether marker clustering, virtualized tables, and aggregate-only dashboards become P1.
3. **What exact CSV schema will bulk import use?** Compatible with TTS bulk-import? Compatible with Kamstrup's export? Worth validating against at least one real customer's existing inventory file.
4. **Does v1 need cost / tariff display at all?** Some customers say "yes immediately"; others say "we have a billing system." The answer determines whether a tariff entity is in v1 or v2.
5. **Anomaly detection: how much history before alerts go live per device?** Need a documented cold-start rule (e.g. "≥21 days of hourly readings before anomaly engine activates for a meter") so customers don't see false alerts in week 1.
6. **What's the install-side performance target?** "1 customer = 1 VPS" suggests we should size for 5k devices, 1Hz uplink burst, on a 4-vCPU box. Confirm with stack research.

---

## Sources

LoRaWAN ecosystem and ChirpStack:
- [ChirpStack Device Profiles](https://www.chirpstack.io/docs/chirpstack/use/device-profiles.html)
- [ChirpStack Multicast Groups](https://www.chirpstack.io/docs/chirpstack/use/multicast-groups.html)
- [ChirpStack Changelog (v4 series)](https://www.chirpstack.io/docs/chirpstack/changelog.html)
- [In-Depth Comparison of LoRaWAN Network Servers (DEV)](https://dev.to/manthink/in-depth-comparison-of-lorawan-network-servers-thinklink-tts-chirpstack-loriot-and-actility-45l8)
- [ChirpStack Alternatives — AlternativeTo](https://alternativeto.net/software/chirpstack/)
- [TTN Device Repository](https://github.com/TheThingsNetwork/lorawan-devices)
- [Adding Devices in Bulk — The Things Stack](https://www.thethingsindustries.com/docs/hardware/devices/adding-devices/adding-devices-in-bulk/)
- [Datacake Payload Decoders](https://docs.datacake.de/lorawan/payload-decoders)
- [ThingsBoard issue: Multi-vendor LoRaWAN payload decoding](https://github.com/thingsboard/thingsboard/issues/10270)

Smart water / AMI / MDMS:
- [Semtech: Smart Water Metering with LoRa](https://www.semtech.com/lora/lora-applications/smart-water-metering)
- [Kamstrup: LoRaWAN Smart Water Metering](https://www.kamstrup.com/en-en/insights/lorawan-smart-water-metering)
- [LoRaWAN-based smart water management — review article (T&F)](https://www.tandfonline.com/doi/full/10.1080/24751839.2025.2458889)
- [Smarter Technologies: Advanced Metering Infrastructure](https://smartertechnologies.com/building-energy-management/advanced-metering-infrastructure/)
- [Badger Meter: AMR vs AMI](https://www.badgermeter.com/blog/amr-vs-ami-whats-the-difference/)
- [What Is a Meter Data Management System (Mainlink)](https://mainlink.net/what-is-a-meter-data-management-system/)
- [Bynry: What is MDMS](https://www.bynry.com/blog/what-is-mdms)
- [Meter data management — Wikipedia](https://en.wikipedia.org/wiki/Meter_data_management)
- [CSA: MDM for Water Utilities](https://www.csa1.com/meter-data-management-for-water-utilities-what-system-operators-need-to-know/)

Leak / anomaly detection:
- [Detection of emergent leaks using machine learning — IWA](https://iwaponline.com/ws/article/23/6/2370/95124/Detection-of-emergent-leaks-using-machine-learning)
- [Multisource dataset for anomaly detection in water networks — Nature Sci Data](https://www.nature.com/articles/s41597-026-07203-5)
- [Rezatec: Why Utilities Are Turning to Water Leak Detection Software](https://www.rezatec.com/water-leak-detection-software-utilities/)

Building / floor-plan / dashboards:
- [Kaa Tutorial: Floor Plan to Smart Building Dashboard](https://www.kaaiot.com/blog/how-to-add-floor-plan-to-smart-building-dashboard)
- [Cumulocity: Indoor Data-Point Map](https://github.com/Cumulocity-IoT/cumulocity-indoor-data-point-map)
- [Akenza: 3D Floorplan Component](https://akenza.io/blog/3d-floorplan-component)
- [Sisense Floor Plan Map Plugin](https://www.sisense.com/marketplace/add-on/floor-plan-map-plugin/)

ThingsBoard / IoT-platform features:
- [ThingsBoard RBAC](https://thingsboard.io/docs/pe/user-guide/rbac/)
- [ThingsBoard Audit Log](https://thingsboard.io/docs/user-guide/audit-log/)
- [ThingsBoard Energy use cases](https://thingsboard.io/use-cases/smart-energy/)

Submetering:
- [Genea Submetering Billing](https://www.getgenea.com/products/submeter/submeter-billing/billing-for-all-tenant-submeters/)
- [Triacta Tenant Billing](https://www.triacta.com/tenant-billing)
- [Setra Tenant Submetering](https://www.setra.com/energy-management/tenant-submetering)
- [Vitality: Utility Submetering Software](https://vitality.io/utility-submetering/)

Operations / backup / DR:
- [AWS: Disaster Recovery for IoT Platforms](https://aws.amazon.com/blogs/iot/how-to-implement-a-disaster-recovery-solution-for-iot-platforms-on-aws/)
- [IoT World Today: DR for IoT](https://www.iotworldtoday.com/iiot/the-best-disaster-recovery-approaches-for-iot)
- [Asimily: IoT Devices & Cyber DR](https://asimily.com/blog/iot-devices-and-cyber-disaster-recovery/)

Real-time push patterns:
- [WebSocket Alternatives: SSE, WebTransport, MQTT (TechSaaS)](https://www.techsaas.cloud/blog/websocket-alternatives-sse-webtransport-mqtt/)
- [Using WebSockets for Live IoT Data Streaming (IQ2S)](https://iq2s.org/using-websockets-for-live-iot-data-streaming/)

Meter-swap continuity reference patterns:
- [Home Assistant Utility Meter integration (offset semantics)](https://www.home-assistant.io/integrations/utility_meter/)
- [HA Utility Meter offset discussion](https://github.com/orgs/home-assistant/discussions/2210)

User-side complaints driving feature priorities:
- [Dunelabs: Challenges in Reading and Billing Traditional Water Meters](https://dunelabs.ai/2023/05/02/challenges-in-reading-and-billing-traditional-water-meters/)
- [KXAN: Austin Water smart-meter complaints](https://www.kxan.com/investigations/austin-water-leader-responds-to-kxan-investigation-over-customers-smart-meter-complaints/)

---
*Feature research for: Shifter — LoRaWAN water/electricity utility monitoring platform*
*Researched: 2026-04-27*
