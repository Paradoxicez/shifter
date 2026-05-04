# Phase 2: Domain Model & Canonical Schema - Research

**Researched:** 2026-05-03
**Domain:** TimescaleDB hypertable schema for LoRaWAN telemetry, metering-point lifecycle (binding history + reading offset), counter-rollover math, ChirpStack v4 control-plane bootstrap (Tenant/Application/DeviceProfile/Device), MQTT uplink ingest with DevEUI→metering-point resolver, vendor-agnostic codec mapping, audit log substrate, single-action stepped dialogs (Add Device + Meter Swap + Profile Editor), synthetic test harness via testcontainers-go.
**Confidence:** HIGH (Postgres/TimescaleDB schema patterns, ChirpStack v4 gRPC surface, sqlc+pgx idioms, paho.mqtt.golang ingest, golang-migrate library mode, scs/v2 user_id capture, shadcn ResponsiveDialog+Stepper reuse — all established in Phase 1 and verified against current docs). MEDIUM (Acrel ADW300 / ADL200 codec details — manual referenced but specific byte layout to be derived at plan time from the manual PDF; Axioma W1 short-vs-extended payload split confirmed via canonical Node-RED gist; resolver LISTEN/NOTIFY pattern in pgx/v5 confirmed but exact dedicated-conn lifecycle shape is implementation detail). LOW (none — every Phase-2 critical decision is anchored in CONTEXT.md or a verifiable source).

## Summary

Phase 2 turns Shifter from "Phase 1's verified-empty install shell" into "telemetry lands keyed by stable metering-point identity, swap-and-rollover math is correct, and the operator can stand up the full chain — site → metering point → device + ChirpStack underlying — in single dialogs without ever seeing CS-native terminology." The work decomposes into eight tightly-coupled tracks: (1) **schema** — 9 plain-SQL migrations (`0007_site` through `0015_audit_log`) including the single `measurement` hypertable per CONTEXT.md D-03/D-06; (2) **ChirpStack control-plane wrappers** — TenantService / ApplicationService / DeviceProfileService / DeviceService added behind `internal/chirpstack/` (still the SOLE importer of `chirpstack-api/go/v4` per Phase 1 architecture); (3) **MQTT ingest pipeline** — wires the existing Phase 1 `MQTTSubscriber.UplinkHandler` to a resolver→normalize→persist pipeline; (4) **DevEUI→metering-point resolver** — in-memory map cache with `LISTEN/NOTIFY` invalidation on swap commits (CONTEXT D-25); (5) **swap and rollover math** — `display = raw + offset` with rollover detection `if raw_t < raw_t-1 → offset += counter_modulus` (CONTEXT D-05); (6) **add-device + meter-swap + profile-editor dialogs** — three stepped dialogs reusing Phase 1's `ResponsiveDialog` + `Stepper`; (7) **audit log** — single regular Postgres table written synchronously inside the same transaction as each domain mutation (CONTEXT D-22/D-23); (8) **synthetic test harness** — shared `internal/testharness/` library + `shifter test-harness <scenario>` CLI driving deterministic `clean_swap`, `swap_inflight`, `rollover`, `swap_rollover`, `overlapping_uplinks_during_swap` × 2 codec families = 10 scenarios via testcontainers-go (Mosquitto + Postgres+TimescaleDB).

The critical-path risk is the **swap math under concurrent in-flight uplinks** — mitigated by CONTEXT D-14 (`gateway_rx_time` vs `valid_to` boundary attribution) and locked by the testharness scenarios. The second risk is **dev_eui→metering_point_id resolver staleness** — mitigated by CONTEXT D-25 (eventless `LISTEN/NOTIFY` invalidation, no TTL, no race window). Everything else is execution against Phase 1's locked patterns.

**Primary recommendation:** Execute the 9 migrations in strict numeric order — `0007_site` → `0008_metering_point` → `0009_device_profile` → `0010_seed_profiles` → `0011_device` → `0012_device_profile_mapping` → `0013_binding` → `0014_measurement` (hypertable) → `0015_audit_log` — and write all schema first, then the ingest pipeline, then the dialogs, with the synthetic test harness driving each layer's verification. Never decode payloads in Shifter — codecs always run inside ChirpStack's QuickJS sandbox per CONTEXT D-09 and PITFALLS §3. Never silently drop uplinks — set `quality` flag and persist (CONTEXT D-26).

<user_constraints>

## User Constraints (from CONTEXT.md)

### Locked Decisions

**Canonical Schema (3-Layer Model)**
- **D-01:** Telemetry uses a 3-layer schema: (1) **canonical wide columns** (typed, indexed, used by dashboard/report/alert); (2) **`device_profile.capabilities[]`** (TEXT array declaring what the device emits — drives adaptive UI render); (3) **`extra` JSONB** (everything the codec emits that isn't promoted to a column). Promotion JSONB→column is `ALTER TABLE` + back-fill (cheap); demotion column→JSONB is hard — start narrow.
- **D-02:** `measurement` Layer 1 columns (frozen for v1, additions are migrations): `metering_point_id UUID NOT NULL`, `time TIMESTAMPTZ NOT NULL` (server-side ingest — DATA-03), `raw_value NUMERIC` (device counter, never overwritten), `cumulative_value NUMERIC` (= `raw_value + binding.reading_offset` — display value), `instant_value NUMERIC` (flow_rate or instant_power), `battery_pct SMALLINT`, `rssi SMALLINT`, `snr REAL`, `temperature_c REAL`, `pressure_kpa REAL`, `leak_detected BOOL`, `tamper_detected BOOL`, `extra JSONB NOT NULL DEFAULT '{}'`, `raw_payload BYTEA NOT NULL`, `decoded_object JSONB NOT NULL`, `quality TEXT NOT NULL DEFAULT 'ok'` (see D-26).
- **D-03:** **Single hypertable** `measurement` for all utility classes (water, electricity, future heat/gas).
- **D-04:** Capability vocabulary v1: `cumulative`, `flow_rate`, `instant_power`, `battery`, `temperature`, `pressure`, `leak_detection`, `tamper_detection`, `multi_phase`, `power_quality`.
- **D-05:** **Counter rollover modulus** is per-device-profile config: `device_profile.counter_modulus BIGINT NOT NULL DEFAULT 4294967296` (= 2^32). Profile editor allows admin override.
- **D-06:** TimescaleDB hypertable config — `measurement` is a hypertable with `chunk_time_interval = INTERVAL '1 day'`. `audit_log` is a regular Postgres table. Compression policy + retention deferred to Phase 5/6.

**Vendor Profiles**
- **D-07:** Phase 2 ships **three vendor profiles fully wired end-to-end**: Axioma Qalcosonic W1 (water), Acrel ADL200 (1-phase elec), Acrel ADW300 (3-phase elec). "Fully wired" = schema + codec_js + mapping + capabilities locked + DATA-06 synthetic harness. Does not require physical hardware (synthetic uplink generators).
- **D-08:** **Profile editor UI ships fully in Phase 2.** Operator creates new profiles via UI: paste sample decoded JSON → click leaves of JSON tree → spreadsheet-like mapping table → capability checkboxes → paste codec_js → save (sent to ChirpStack on save).
- **D-09:** **Seed delivery:** migration `0010_seed_profiles.up.sql` inserts the 3 device_profile rows. On boot, sync routine pushes each codec_js to ChirpStack via gRPC `DeviceProfileService.Create/Update` (idempotent, pinned by `cs_profile_id`). Operator overrides stay local.

**Add-Device & Meter-Swap UX**
- **D-10:** **Add-device dialog is a 4-step stepped dialog**: (1) Paste DevEUI sticker; (2) Pick device profile + AppKey + JoinEUI; (3) Bind to metering point; (4) Review + submit (atomic backend transaction).
- **D-11:** **DevEUI sticker parser** auto-detects endianness with operator confirmation: render MSB-first + LSB-first + vendor OUI hint side-by-side; operator clicks correct one.
- **D-12:** **Meter swap — outgoing reading R capture:** auto-fill from latest uplink + verify checkbox. Manual entry fallback if outgoing meter unreachable (warning chip on swap audit row).
- **D-13:** **Reading offset proposal panel:** math read-back panel showing R, N, R-N, continuity narrative + editable override + "Show me the math" tooltip. No mini-chart in Phase 2.
- **D-14:** **Swap timing semantics:** `binding_old.valid_to = swap.confirm_time` (operator click). In-flight uplinks attributed by `gateway_rx_time` vs `valid_to`. If no binding covers timestamp → drop + audit-log warning event.
- **D-15:** **Decommission is a separate action** from swap. Closes active binding, marks device decommissioned. MP stays active for new device assignment.
- **D-16:** **ChirpStack disconnect handling:** Pre-flight pings CS gRPC + MQTT in review step. On submit: BEGIN tx → CS gRPC create → Shifter row insert → COMMIT. Any CS failure → ROLLBACK + error toast. No partial state, no async queue, no Saga in Phase 2.

**Site & Metering Point Surface**
- **D-17:** **Create Site dialog fields** — `name*`, `lat`, `lng`*, `timezone` (default = install timezone), `address` (free text), `description` (optional). Schema also has `parent_id UUID NULL` and `site_type TEXT NULL` (no UI in Phase 2 — Phase 5 enables tree picker).
- **D-18:** **Site lat/lng UI in Phase 2** = two number input boxes + "paste from Google Maps" helper text + range validation. `Pick on map` button rendered but disabled with "available in v5" tooltip (Leaflet not installed yet).
- **D-19:** **Create Metering Point dialog fields** — `name*`, `site_id*`, `utility_class*` (radio: water/electricity), `location_description`. `expected_interval_s` and `unit` inherited from bound device profile. `utility_class` on MP so DASH-01 works even with no device bound.
- **D-20:** **Metering point lifecycle = soft-delete only.** `archived_at TIMESTAMPTZ NULL`. UI exposes `Archive` (hides from default list, dashboard stops rendering, historical queries still resolve). "Archived" tab restores via `archived_at = NULL`. No hard-delete.

**Audit Log**
- **D-21:** **Audit scope in Phase 2 = CRUD on domain entities (Site, Metering Point, Device, Device Profile, Binding) + meter swap action.** Auth-event auditing deferred to Phase 6.
- **D-22:** **Single `audit_log` table** with columns: `id UUID PK`, `time TIMESTAMPTZ DEFAULT now()`, `user_id UUID REFERENCES "user"(id)`, `action TEXT NOT NULL`, `entity_type TEXT NOT NULL`, `entity_id UUID NOT NULL`, `before JSONB`, `after JSONB`, `notes TEXT`, `request_id TEXT`.
- **D-23:** **Audit write is synchronous in the same transaction as the write.** No async/River-job indirection.
- **D-24:** **`before`/`after` capture = changed fields only** (diff-like). CREATE → `before = NULL`, `after` = full insert. ARCHIVE → `before = {archived_at: null}`, `after = {archived_at: <ts>}`. SWAP → binding pair.

**MQTT → Ingest Pipeline**
- **D-25:** **dev_eui → metering_point_id resolver** is an in-memory map cache + `LISTEN/NOTIFY` invalidation. Lazy-load on first uplink. Swap commit publishes `NOTIFY binding_changed, '<dev_eui>'`. No TTL — eventless invalidation. Phase 4 SSE will reuse the same channel.
- **D-26:** **Ingest quality flag — persist all uplinks, never silent-drop.** `measurement.quality TEXT NOT NULL DEFAULT 'ok'` with values `ok`, `decode_fail`, `missing_canonical`, `out_of_range`, `duplicate_fcnt`. `raw_payload` and `decoded_object` always preserved. Dashboard surfaces a badge on devices with non-`ok` rows.

**Synthetic Test Harness (DATA-06)**
- **D-27:** **Test harness is a shared library + dual entry point.** `internal/testharness/` package owns scenario generation (clean_swap, swap_inflight, rollover, swap_rollover, overlapping_uplinks_during_swap × 2 codec families = 10 deterministic scenarios). Go integration tests via testcontainers-go (Mosquitto + Postgres+TimescaleDB). CLI wrapper `shifter test-harness <scenario>` for post-install validation.

**ChirpStack Abstraction**
- **D-28:** **Single global ChirpStack tenant + 1 application + auto-bootstrap.** Tenant `shifter-default` and application `shifter` created on first use (or reused — pinned by CS UUID stored in Shifter `chirpstack_connection` row). External-CS mode: if API token's tenant context already names a tenant, use it. Each Shifter `device_profile` maps 1:1 to one CS profile via `cs_profile_id`. Single MQTT topic filter `application/<app_id>/device/+/event/up`. Operator never sees CS hierarchy.

**Devices & Bulk**
- **D-29:** **Phase 2 devices surface = picker component + minimal list page.** Searchable dropdown by name/dev_eui. Minimal devices list as TanStack Table — columns: name, dev_eui, profile, current MP, last_seen, battery. Phase 3 adds full filtering + bulk + ABP/OTAA UX + viewer-secret-hide.
- **D-30:** **Bulk operations not in Phase 2.** Sites and MPs single-create through dialogs. Phase 3 ships unified CSV bulk-import (PITFALLS §12 dry-run + commit).

### Claude's Discretion

- Exact `internal/` directory layout for new packages (e.g. `internal/ingest`, `internal/swap`, `internal/profile`, `internal/audit`, `internal/testharness`) — planner picks idiomatic Go boundaries
- Migration file naming for Phase 2 tables (numeric prefix continues from 0006 onwards; one conceptual change per migration)
- Foreign key + unique constraint placement (e.g. UNIQUE(metering_point_id, valid_from) on binding to prevent overlap)
- Exact JSON shape for `audit_log.before`/`after` diffs (preserve null vs missing-key semantics per planner's judgement)
- TanStack Table column visibility defaults, sort order, page size for the minimal devices list page
- Mapping editor's exact UX for clicking JSON tree leaves vs typing json_pointer manually
- Profile editor's `data_type` set (numeric/int/bool/text — extend if planner finds another needed)
- Exact wording on the `quality` badge tooltip surfaces
- Synthetic-data CLI flag/subcommand shape (`shifter test-harness clean_swap --vendor axioma`)
- River queue config — Phase 2 doesn't ship any River jobs (audit is sync, CS sync is sync, ingest is sync); River install can defer to Phase 5

### Deferred Ideas (OUT OF SCOPE)

- Auth-event auditing (login/logout/password) — Phase 6
- Audit browse + filter UI + CSV export (AUDIT-02, AUDIT-03) — Phase 6
- Continuous aggregates + retention + compression (DATA-11..13) — Phase 5
- Pixel-accurate floor-plan device placement — Phase 5
- Map view + Leaflet — Phase 5
- Bulk CSV import (sites + MPs + devices) — Phase 3
- Devices full list with search/filter/sort (DEV-01) — Phase 3
- Operator-paste codec test-runner UI (V2-VEND-02) — Phase 7
- Pre-seeded vendor catalog beyond 3 profiles (V2-VEND-01) — Phase 7
- Mini-chart preview in swap dialog — Phase 4 (after shadcn-charts arrives)
- Saga-style "pending CS sync" device state — Phase 6 ops hardening (if real CS flakiness surfaces)
- Per-site application in ChirpStack — only revisit on clear scaling/isolation requirement
- River background job queue — Phase 5 CAGG refresh first
- External_code (operator-defined site/MP code) — add when first customer asks
- Tariff / cost / billing-cycle fields — V2-BILL-01
- Profile import/export between installs — Phase 7 catalog likely subsumes
- Compression of `raw_payload`/`decoded_object` — Phase 5/6 retention work
- Time-zone handling in measurement queries — Phase 4 dashboard + Phase 5 reports

</user_constraints>

<phase_requirements>

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| **SITE-01** | Admin can create / edit / delete sites via dialogs, with site lat/lng for the map view | `0007_site` migration with the 5 D-17 fields + `archived_at`; CreateSite/EditSite ResponsiveDialog reusing Phase 1 patterns; lat/lng as plain number inputs with disabled "Pick on map" button per D-18 |
| **DATA-01** | Telemetry is keyed on stable `metering_point_id`, never `dev_eui` directly | `measurement.metering_point_id UUID NOT NULL` is the schema invariant per CONTEXT D-02; resolver D-25 enforces this on every uplink; PITFALLS §1 confirms this is the most-expensive-to-retrofit decision in the project |
| **DATA-02** | Each device-to-metering-point binding has `valid_from`/`valid_to` window + `reading_offset` | `0013_binding` migration: `binding(id, metering_point_id, device_id, valid_from TIMESTAMPTZ NOT NULL, valid_to TIMESTAMPTZ NULL, reading_offset NUMERIC NOT NULL DEFAULT 0)`; UNIQUE constraint on `(metering_point_id, valid_from)` to prevent overlap; index on `(dev_eui, valid_from DESC)` for resolver lookup |
| **DATA-03** | Hypertable's authoritative `time` column is server-side ingest time | `time TIMESTAMPTZ NOT NULL` (ingest path sets `time = time.Now().UTC()` on uplink receipt — PITFALLS §5 is unambiguous on this); `gateway_rx_time TIMESTAMPTZ` and `device_time TIMESTAMPTZ NULL` persisted as diagnostics in `extra` or as columns |
| **DATA-04** | Admin can perform meter swap via dialog | Stepped swap dialog: capture R (auto-fill from last uplink per D-12), open new binding, propose `reading_offset = R - N` per D-13, atomic commit (close old binding + open new + write audit row in same Serializable transaction per D-14/D-23) |
| **DATA-05** | System detects counter rollovers | Ingest pipeline: per binding, track `last_raw_value`. If `current_raw < last_raw` AND not at a binding boundary → `binding.reading_offset += device_profile.counter_modulus`; insert measurement; write `device_health` event row to audit log per CONTEXT D-05 |
| **DATA-06** | Synthetic-data tests cover clean swap, swap with concurrent in-flight uplink, rollover, swap+rollover, overlapping uplinks during swap | `internal/testharness/` library + `shifter test-harness` CLI per CONTEXT D-27. 10 deterministic scenarios = 5 patterns × 2 codec families (axioma_w1, acrel_adw300). testcontainers-go drives Mosquitto + Postgres+TimescaleDB |
| **DATA-07** | System persists raw payload, decoded object, canonical normalized fields — none lost | `measurement.raw_payload BYTEA NOT NULL`, `decoded_object JSONB NOT NULL`, plus the canonical Layer-1 columns from D-02; `quality` flag tracks integrity per D-26 (never silent-drop) |
| **DATA-08** | Hybrid wide+JSONB measurement schema | Implements TigerData IoT pattern: 11+ canonical columns + `extra JSONB NOT NULL DEFAULT '{}'`. Index strategy: `(metering_point_id, time DESC)` BTREE + `extra` GIN(jsonb_path_ops) |
| **DATA-09** | Admin maps device profile fields to canonical columns through UI; new vendors require no backend deploy | Profile editor (D-08) ships in Phase 2. `device_profile_mapping` table: `(device_profile_id, json_pointer TEXT, target TEXT, scale NUMERIC, data_type TEXT)`. Mapping editor uses TanStack Table; clicking JSON-tree leaves auto-fills `json_pointer` (RFC 6901). |
| **DATA-10** | At least one fully wired vendor profile + documented path to add more | Phase 2 ships THREE: Axioma Qalcosonic W1 (water), Acrel ADL200 (1-phase elec), Acrel ADW300 (3-phase elec). Adding more = use the in-product profile editor (no backend deploy) |
| **AUDIT-01** | Audit entry for every create/update/delete + every meter swap | Single `audit_log` table per D-22. Every state-changing handler writes a row inside the same Serializable transaction as the domain mutation per D-23. `before`/`after` are diffs per D-24. |
| **CHIRP-04** | Adding a device is a single user action — Shifter creates/reuses underlying CS tenant/application/profile/device | 4-step stepped dialog per D-10 collapses the multi-step CS workflow into one user submit. Auto-bootstrap of the global `shifter-default` tenant + `shifter` application per D-28. `BEGIN tx → CS bootstrap (idempotent) → CS device create → Shifter device insert → binding open → audit insert → COMMIT`. ChirpStack pre-flight ping in review step per D-16. |

</phase_requirements>

## Standard Stack

### Core (already installed via Phase 1 — versions verified in `go.mod` and `web/package.json` 2026-05-03)

| Library | Version (verified) | Purpose | Why Standard |
|---------|--------------------|---------|--------------|
| Go | 1.25.0 | Backend language | Phase 1 module already at 1.25.0 — no upgrade needed for Phase 2 |
| `github.com/jackc/pgx/v5` | v5.9.2 | Postgres driver + pool | Phase 1 already on pgx/v5; sqlc-generated code uses this driver |
| `github.com/golang-migrate/migrate/v4` | v4.19.1 (with pgx/v5 driver) | Plain-SQL migrations | Phase 1 D-13/D-16 lock golang-migrate as **library mode** (not CLI) with integer prefix file naming |
| `github.com/sqlc-dev/sqlc` | (config v2 — sqlc.yaml verified) | SQL → typed Go code generator | Phase 1 sqlc.yaml already configured: schema=`internal/db/migrations`, queries=`internal/db/queries`, out=`internal/db/sqlc`, sql_package=`pgx/v5`, emit_pointers_for_null_types=true, emit_interface=true |
| `github.com/chirpstack/chirpstack/api/go/v4` | v4.17.0 | ChirpStack v4 gRPC client stubs (TenantService, ApplicationService, DeviceProfileService, DeviceService) | Already pinned in Phase 1 go.mod; **only `internal/chirpstack/` may import this** per Phase 1 architecture seam |
| `google.golang.org/grpc` | v1.80.0 | gRPC transport | Phase 1 dependency |
| `github.com/eclipse/paho.mqtt.golang` | v1.5.1 | MQTT client | Phase 1 `internal/chirpstack/mqtt.go` already subscribes to `application/+/device/+/event/up` and exposes pluggable `UplinkHandler` — Phase 2 just wires a non-nil handler |
| `github.com/go-chi/chi/v5` | v5.2.5 | HTTP router | Phase 1 already wires chi router with middleware stack |
| `github.com/alexedwards/scs/v2` | v2.9.0 | Session manager | Phase 1 — exposes `user_id` for audit_log capture per CONTEXT D-22 |
| `github.com/spf13/cobra` | v1.10.2 | CLI subcommands | Phase 1; Phase 2 adds `shifter test-harness <scenario>` |
| `github.com/testcontainers/testcontainers-go` | v0.42.0 (+ `/modules/postgres` v0.42.0) | Integration tests w/ ephemeral Postgres+TimescaleDB and Mosquitto | Phase 1 already uses this for migration tests; Phase 2 testharness needs a Mosquitto module addition |
| **TimescaleDB extension** | 2.26.0 (March 2026) | Hypertable on `measurement` | Already part of bundled compose flavor (Phase 1 OPS-01); 2.26 dropped trigger-based CAGG invalidation for 10-20% faster ingest [VERIFIED: github.com/timescale/timescaledb/releases]. Postgres 16 or 17 supported [VERIFIED: docs.tigerdata.com] |

### Frontend Core (already installed via Phase 1)

| Library | Version (verified `web/package.json`) | Purpose | When to Use |
|---------|---------------------------------------|---------|-------------|
| React | 19.2.5 | Framework | Phase 1 |
| Vite | 7.x (via `@vitejs/plugin-react` 5.2.0) | Build | Phase 1 |
| TypeScript | 5.6+ (`@types/react` 19.2.14) | Types | Phase 1 |
| Tailwind CSS | 4.x (via `@tailwindcss/vite`) | Styling | Phase 1 |
| `@tanstack/react-query` | 5.100.5 | Server state, mutations, cache invalidation | All Phase 2 mutations (create site, swap meter, save profile) ride this; cache invalidation patterns: `queryClient.invalidateQueries({queryKey: ['sites']})` after each mutation |
| `react-hook-form` | 7.74.0 | Form state for all dialog forms | Phase 1 idiom; pairs with zod resolver |
| `zod` | 4.3.6 | Runtime validation | Phase 1 idiom |
| `@hookform/resolvers` | 5.2.2 | RHF + Zod glue | Phase 1 |
| `react-router-dom` | 7.14.2 | Routing | Phase 1; Phase 2 adds `/sites`, `/sites/:id`, `/metering-points/:id`, `/devices`, `/profiles`, `/profiles/:id` |
| `radix-ui` | 1.4.3 | Headless primitives (under shadcn) | Phase 1 |
| `sonner` | 2.0.7 | Toasts | Phase 1; success/error feedback after each Phase 2 mutation |
| `lucide-react` | 1.11.0 | Icons | Phase 1 |
| `date-fns` | 4.1.0 | Date math | Phase 1; "uplink 5 minutes ago" relative timestamps in swap dialog per D-12 |

### Phase 2 New Frontend Dependencies (requires `pnpm add` in Plan 1)

| Library | Recommended Version | Purpose | Justification |
|---------|---------------------|---------|---------------|
| `@tanstack/react-table` | v8.x | Headless table for minimal devices list page (D-29) and profile mapping editor (D-08) | shadcn's Table primitive composes with TanStack Table; needed for sortable columns + per-cell render. Verify with `pnpm view @tanstack/react-table version` at plan time. |

### shadcn/ui Components Phase 2 Requires (run `pnpm dlx shadcn@latest add <name>` in Plan 1)

Already in repo from Phase 1: `responsive-dialog`, `stepper`, `status-row`, `theme-provider`, `account-menu`, `change-password-dialog`. New components needed:
- `table` — for devices list and mapping editor
- `select` — for dropdowns (timezone picker, profile picker, MP picker, target column dropdown in mapping editor)
- `radio-group` — for `utility_class` (water/electricity)
- `checkbox` — for verify checkbox in swap dialog (D-12) and capability checkboxes in profile editor (D-08)
- `tabs` — for "Active" / "Archived" tab on MP list (D-20)
- `tooltip` — for "Show me the math" in swap dialog (D-13) and "available in v5" on disabled Pick on map button (D-18)
- `badge` — for "X uplinks flagged" quality badge (D-26)
- `textarea` — for `description` on Site (D-17), `notes` on audit entries
- `combobox` (or shadcn `command` + `popover`) — for searchable device picker (D-29)
- `card` — for layout of MP detail page

### Alternatives Considered (locked in CONTEXT, document rejected for traceability)

| Instead of | Could Use | Why CONTEXT chose locked option |
|------------|-----------|--------------------------------|
| Single `measurement` hypertable | Per-utility-class tables (water_measurement, electricity_measurement) | D-03 explicitly rejected: cross-utility "all meters" query becomes UNION; sparse-NULL cost negligible at PG storage; single CAGG chain in Phase 5; adding heat/gas is `ALTER TABLE` not new table |
| `audit_log` as hypertable | Regular Postgres table | D-06 explicitly chose regular table (audit volume ≤ hundreds rows/day for single-tenant install — hypertable adds no value). Re-evaluate at Phase 6 |
| dev_eui→MP resolver with TTL | TTL-less + LISTEN/NOTIFY invalidation | D-25 explicitly chose eventless invalidation to prevent swap-then-uplink-attributed-to-old-binding race (PITFALLS §2). TTL would create a window of staleness exactly when the swap fires |
| Async audit write (River job) | Synchronous write inside same txn | D-23 explicitly chose sync — meter swap audit MUST be atomic with the swap; River is for retries on backend tasks |
| Multi-tenant CS application per Shifter site | Single global CS tenant + 1 application | D-28 explicitly chose single — per-site CS app revisit only on clear scaling/isolation requirement (none expected for single-tenant single-install) |
| Saga-style "pending CS sync" device state | Transactional rollback on CS failure | D-16 explicitly chose ROLLBACK; revisit if real CS flakiness surfaces |
| Hand-written swap math in handlers | Math in `internal/swap/` package + extensively unit-tested | Math is the most-expensive-to-retrofit code in the project (PITFALLS §2); isolate in a package and let testharness drive |

### Version verification

Verify recommended frontend versions before plan-time:
```bash
pnpm view @tanstack/react-table version
```
Backend versions are already locked in Phase 1 `go.mod` — no `npm view` needed. TimescaleDB 2.26.0 verified [CITED: github.com/timescale/timescaledb/releases — 2026-03-24].

## Architecture Patterns

### Recommended Project Structure

```
shifter/
├── cmd/shifter/
│   └── main.go                         # Phase 1 — extend with cobra `test-harness` subcommand
├── internal/
│   ├── audit/                          # NEW — audit log writer + before/after diff helpers
│   │   ├── doc.go
│   │   ├── log.go                      # WriteAuditEntry(ctx, tx, entry) — runs INSIDE caller txn (D-23)
│   │   └── diff.go                     # ChangedFields(before, after any) → JSONB-shaped map (D-24)
│   ├── auth/                           # Phase 1 — extend Can() with new actions
│   │   └── can.go                      # add: site.create/update/archive, metering_point.*, device.add/decommission, device_profile.*, meter.swap
│   ├── chirpstack/                     # Phase 1 — extend with control-plane wrappers
│   │   ├── client.go                   # existing
│   │   ├── mqtt.go                     # existing — Phase 2 wires UplinkHandler at startup
│   │   ├── tenant.go                   # NEW — TenantService.Create/Get/List wrapper
│   │   ├── application.go              # NEW — ApplicationService.Create/Get/List wrapper
│   │   ├── device_profile.go           # NEW — DeviceProfileService.Create/Update/Get wrapper (codec_js sync)
│   │   ├── device.go                   # NEW — DeviceService.Create/Delete/Get + DeviceKeys.Create wrapper
│   │   └── bootstrap.go                # NEW — EnsureTenantAndApplication() — idempotent first-boot routine (D-28)
│   ├── ingest/                         # NEW — MQTT uplink consumer (the Phase 2 ingest pipeline)
│   │   ├── doc.go                      # explains: subscribed via existing MQTTSubscriber.UplinkHandler from Phase 1 (D-25/D-26)
│   │   ├── handler.go                  # UplinkHandler(topic, payload) — entry point bound at boot
│   │   ├── decode.go                   # parses ChirpStack v4 uplink event JSON; extracts dev_eui, gateway_rx_time, decoded object
│   │   ├── normalize.go                # applies device_profile_mapping → canonical columns + extra JSONB
│   │   ├── persist.go                  # INSERT INTO measurement; updates last_seen on device; advances reading_offset on rollover
│   │   └── quality.go                  # SetQuality(decode_fail|missing_canonical|...) per D-26
│   ├── resolver/                       # NEW — DevEUI → metering_point_id (D-25)
│   │   ├── doc.go
│   │   ├── cache.go                    # in-memory map[string]Binding; sync.RWMutex
│   │   ├── listener.go                 # dedicated pgx conn → LISTEN binding_changed (NOT from pool — see Pitfall §X below)
│   │   └── invalidate.go               # called by swap commit: NOTIFY binding_changed, '<dev_eui>'
│   ├── swap/                           # NEW — swap math + transaction
│   │   ├── doc.go                      # offset math: display = raw + offset; rollover: offset += counter_modulus
│   │   ├── math.go                     # pure functions: ProposeOffset(R, N), DetectRollover(prev_raw, curr_raw, modulus)
│   │   └── commit.go                   # CommitSwap(tx, mp_id, outgoing_dev_eui, incoming_dev_eui, R, N, override?) — Serializable txn
│   ├── profile/                        # NEW — device profile CRUD + codec_js sync
│   │   ├── editor.go                   # validates codec_js (length, basic syntax check); syncs to CS via DeviceProfileService.Create/Update
│   │   └── seed.go                     # called by 0010_seed_profiles boot hook to push the 3 seed codecs to ChirpStack
│   ├── site/                           # NEW — Site CRUD handlers
│   │   └── handlers.go                 # POST /api/sites, GET /api/sites, PATCH /api/sites/:id, POST /api/sites/:id/archive
│   ├── meteringpoint/                  # NEW — MP CRUD handlers + active-binding lookup
│   │   └── handlers.go                 # POST /api/metering-points, GET, PATCH, archive; GET /api/metering-points/:id/binding/active
│   ├── device/                         # NEW — Device handlers (minimal Phase 2 surface)
│   │   ├── handlers.go                 # POST /api/devices (atomic: CS create + Shifter insert + open binding)
│   │   ├── decommission.go             # POST /api/devices/:dev_eui/decommission
│   │   └── deveui.go                   # ParseDevEUI(raw) → {msb_first, lsb_first, vendor_oui_hint} for D-11
│   ├── testharness/                    # NEW — synthetic uplink scenarios (D-27)
│   │   ├── doc.go
│   │   ├── scenarios.go                # 5 scenario funcs (clean_swap, swap_inflight, rollover, swap_rollover, overlapping_uplinks)
│   │   ├── codec_axioma.go             # axioma_w1 synthetic payload generator (matches Node-RED gist byte layout)
│   │   ├── codec_acrel.go              # acrel_adw300 synthetic payload generator
│   │   └── runner.go                   # PublishToBroker(ctx, scenario) — used by both Go tests AND `shifter test-harness` CLI
│   ├── cli/
│   │   └── testharness.go              # NEW — cobra subcommand: shifter test-harness <scenario> [--vendor axioma]
│   ├── http/
│   │   ├── router.go                   # Phase 1 — extend with new route groups
│   │   └── middleware.go               # Phase 1 — RequireAction extends to new actions
│   ├── db/
│   │   ├── migrations/                 # +9 migrations (0007..0015)
│   │   ├── queries/                    # +sites.sql, metering_points.sql, devices.sql, device_profiles.sql, device_profile_mappings.sql, bindings.sql, measurements.sql, audit_log.sql
│   │   └── sqlc/                       # auto-regen (`just sqlc`)
│   └── ...                             # Phase 1 packages unchanged
├── web/src/
│   ├── routes/
│   │   ├── sites/
│   │   │   ├── index.tsx               # Sites list (TanStack Table)
│   │   │   ├── $id.tsx                 # Site detail (MPs nested under)
│   │   │   ├── create-site-dialog.tsx
│   │   │   └── edit-site-dialog.tsx
│   │   ├── metering-points/
│   │   │   ├── $id.tsx                 # MP detail (current binding, history, swap button)
│   │   │   ├── create-metering-point-dialog.tsx
│   │   │   └── swap-meter-dialog.tsx   # 3-step swap dialog (D-12, D-13, D-14)
│   │   ├── devices/
│   │   │   ├── index.tsx               # Minimal devices list (D-29)
│   │   │   ├── add-device-dialog.tsx   # 4-step add-device dialog (D-10)
│   │   │   ├── deveui-parser.tsx       # MSB/LSB preview component (D-11)
│   │   │   └── decommission-dialog.tsx # Decommission action (D-15)
│   │   ├── profiles/
│   │   │   ├── index.tsx               # Profiles list
│   │   │   ├── $id.tsx                 # Profile editor (D-08)
│   │   │   ├── mapping-editor.tsx      # JSON tree click → mapping table
│   │   │   └── codec-textarea.tsx      # codec_js paste area
│   │   └── _root.tsx                   # Phase 1 — extend nav
│   └── api/
│       ├── apiFetch.ts                 # Phase 1
│       ├── sites.ts                    # NEW — typed fetchers + react-query hooks
│       ├── meteringPoints.ts           # NEW
│       ├── devices.ts                  # NEW
│       └── profiles.ts                 # NEW
└── compose/                            # Phase 1 — no changes to bundled/external compose flavors
```

### Pattern 1: Plain-SQL migrations with integer prefix (locked in Phase 1 D-16/D-17)

**What:** Each schema change is a plain SQL `up.sql` + `down.sql` pair with integer prefix. golang-migrate runs them via library mode (not CLI) on `shifter serve` startup.

**When to use:** Every schema change in this project, no exceptions. Phase 2 adds `0007` through `0015`.

**Example (verified pattern from Phase 1 `0006_chirpstack_connection.up.sql`):**
```sql
-- Source: internal/db/migrations/0006_chirpstack_connection.up.sql
-- 0007_site.up.sql
-- Site (physical location). Single-tenant per install (no tenant_id on row).
-- D-17: name, lat, lng, timezone, address, description (description optional);
-- parent_id + site_type are no-UI-in-Phase-2 forward-compat columns.
-- D-20: archived_at for soft-delete pattern (no hard-delete).
CREATE TABLE site (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    parent_id     UUID NULL REFERENCES site(id) ON DELETE RESTRICT,
    name          TEXT NOT NULL,
    site_type     TEXT NULL,
    lat           DOUBLE PRECISION,
    lng           DOUBLE PRECISION,
    timezone      TEXT NOT NULL,
    address       TEXT,
    description   TEXT,
    archived_at   TIMESTAMPTZ NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT site_lat_range CHECK (lat IS NULL OR (lat BETWEEN -90 AND 90)),
    CONSTRAINT site_lng_range CHECK (lng IS NULL OR (lng BETWEEN -180 AND 180)),
    CONSTRAINT site_name_not_empty CHECK (length(name) > 0)
);

CREATE INDEX site_archived_at_idx ON site (archived_at) WHERE archived_at IS NULL;
CREATE INDEX site_parent_id_idx ON site (parent_id) WHERE parent_id IS NOT NULL;

CREATE TRIGGER site_touch
    BEFORE UPDATE ON site
    FOR EACH ROW
    EXECUTE FUNCTION touch_updated_at();
```

### Pattern 2: TimescaleDB hypertable creation (CONTEXT D-06)

**What:** `measurement` is a hypertable; the create is part of migration `0014_measurement.up.sql`. Use the function form `create_hypertable('measurement', 'time', chunk_time_interval => INTERVAL '1 day')` — the modern `CREATE TABLE WITH (tsdb.hypertable, ...)` form is TimescaleDB 2.23+ but the function form is still fully supported and matches every example in our research [CITED: docs.timescale.com/api/latest/hypertable/create_hypertable].

**When to use:** The single hypertable in the project (Phase 2). `audit_log` is intentionally NOT a hypertable (CONTEXT D-06).

**Example:**
```sql
-- Source: pattern verified against docs.tigerdata.com/api/latest/hypertable/create_hypertable
-- 0014_measurement.up.sql
-- Telemetry hypertable (DATA-01, DATA-02, DATA-03, DATA-07, DATA-08).
-- D-02 fixed canonical column set; additions are migrations.
-- D-06: chunk_time_interval = 1 day (TigerData IoT recommendation for sub-minute interval ingest).
CREATE TABLE measurement (
    time              TIMESTAMPTZ NOT NULL,
    metering_point_id UUID NOT NULL,
    raw_value         NUMERIC,
    cumulative_value  NUMERIC,
    instant_value     NUMERIC,
    battery_pct       SMALLINT,
    rssi              SMALLINT,
    snr               REAL,
    temperature_c     REAL,
    pressure_kpa      REAL,
    leak_detected     BOOLEAN,
    tamper_detected   BOOLEAN,
    extra             JSONB NOT NULL DEFAULT '{}'::jsonb,
    raw_payload       BYTEA NOT NULL,
    decoded_object    JSONB NOT NULL,
    quality           TEXT NOT NULL DEFAULT 'ok',
    fcnt              INTEGER,
    gateway_rx_time   TIMESTAMPTZ,
    device_time       TIMESTAMPTZ,
    CONSTRAINT measurement_quality_valid CHECK (
        quality IN ('ok','decode_fail','missing_canonical','out_of_range','duplicate_fcnt')
    )
);

-- Hypertable conversion. Function form per docs.tigerdata.com.
SELECT create_hypertable('measurement', 'time', chunk_time_interval => INTERVAL '1 day');

-- Hot-path index: per-MP latest reading + history queries.
CREATE INDEX measurement_mp_time_idx ON measurement (metering_point_id, time DESC);
-- Vendor-specific extra-field GIN for advanced view (DETL-01 in Phase 4).
CREATE INDEX measurement_extra_gin ON measurement USING GIN (extra jsonb_path_ops);
-- Quality-flag filtered partial index for the "X uplinks flagged" badge (D-26).
CREATE INDEX measurement_quality_flagged_idx ON measurement (metering_point_id, time DESC)
    WHERE quality <> 'ok';

-- LISTEN/NOTIFY trigger reserved for Phase 4 SSE — Phase 2 does NOT add this trigger
-- (the trigger lives in Phase 4 because emitting NOTIFY on every uplink with no
-- subscribers is wasted work). Phase 2 only adds the binding_changed channel
-- below for the resolver.
```

### Pattern 3: Binding history with non-overlapping windows (DATA-02)

**What:** `binding(metering_point_id, device_id, valid_from, valid_to NULL, reading_offset)`. At most one row per `(metering_point_id)` may have `valid_to IS NULL` (the active binding). Insert-with-overlap is rejected by exclusion constraint.

**Why this exact shape:** PITFALLS §1 mandates metering-point as primary entity; PITFALLS §2 mandates per-binding offset; CONTEXT D-14 mandates `valid_to = swap.confirm_time` (operator click, not last_uplink_time).

**Example:**
```sql
-- 0013_binding.up.sql
CREATE TABLE binding (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    metering_point_id UUID NOT NULL REFERENCES metering_point(id) ON DELETE RESTRICT,
    device_id         UUID NOT NULL REFERENCES device(id) ON DELETE RESTRICT,
    valid_from        TIMESTAMPTZ NOT NULL,
    valid_to          TIMESTAMPTZ NULL,
    reading_offset    NUMERIC NOT NULL DEFAULT 0,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT binding_window_valid CHECK (valid_to IS NULL OR valid_to > valid_from)
);

-- Resolver lookup hot-path: which MP is dev_eui currently bound to?
CREATE INDEX binding_device_active_idx ON binding (device_id, valid_from DESC);
-- Query: latest reading per MP — needs MP+valid_from index.
CREATE INDEX binding_mp_active_idx ON binding (metering_point_id, valid_from DESC);

-- Exclusion: only ONE active binding per metering point. Uses btree_gist for tstzrange.
CREATE EXTENSION IF NOT EXISTS btree_gist;
ALTER TABLE binding ADD CONSTRAINT binding_no_overlap_per_mp
    EXCLUDE USING gist (
        metering_point_id WITH =,
        tstzrange(valid_from, COALESCE(valid_to, 'infinity'::timestamptz), '[)') WITH &&
    );
```

### Pattern 4: ChirpStack v4 gRPC service wrappers (Phase 1 chirpstack package extension)

**What:** Each new wrapper file in `internal/chirpstack/` adds methods to `Client` that call generated stubs. Pattern is identical to Phase 1's `version.go`/`ProbeVersion` shape.

**When to use:** Every CS gRPC call in Phase 2 — no other package may import `chirpstack/api/go/v4` (architecture seam locked in Phase 1 `internal/chirpstack/doc.go`).

**Example:**
```go
// Source: pattern verified against github.com/chirpstack/chirpstack/blob/master/api/proto/api/device.proto
// Source: extends pattern from internal/chirpstack/client.go (Phase 1)
// internal/chirpstack/device.go
package chirpstack

import (
    "context"
    "fmt"

    "github.com/chirpstack/chirpstack/api/go/v4/api"
)

// CreateDevice creates a device under the given application + profile, then
// uploads its OTAA root keys (AppKey for LoRaWAN 1.0.x; NwkKey for 1.1.x).
// Returns the dev_eui on success. Idempotent: returns nil if device exists.
//
// CHIRP-04 — this is one of the four CS calls collapsed into the single
// Add Device dialog submit (D-10 + D-16 transactional pattern).
func (c *Client) CreateDevice(ctx context.Context, in CreateDeviceInput) error {
    devSvc := api.NewDeviceServiceClient(c.conn)
    _, err := devSvc.Create(ctx, &api.CreateDeviceRequest{
        Device: &api.Device{
            DevEui:          in.DevEUI,
            ApplicationId:   in.ApplicationID,
            DeviceProfileId: in.DeviceProfileID,
            Name:            in.Name,
            Description:     in.Description,
            IsDisabled:      false,
            SkipFcntCheck:   false,
        },
    })
    if err != nil {
        return fmt.Errorf("DeviceService.Create: %w", err)
    }

    _, err = devSvc.CreateKeys(ctx, &api.CreateDeviceKeysRequest{
        DeviceKeys: &api.DeviceKeys{
            DevEui: in.DevEUI,
            AppKey: in.AppKey, // LoRaWAN 1.0.x: also placed in NwkKey by ChirpStack server-side
            NwkKey: in.AppKey, // 1.0.x compat — for 1.1.x devices, separate field
        },
    })
    if err != nil {
        return fmt.Errorf("DeviceService.CreateKeys: %w", err)
    }
    return nil
}

type CreateDeviceInput struct {
    DevEUI          string
    ApplicationID   string
    DeviceProfileID string
    Name            string
    Description     string
    AppKey          string // hex string, 32 chars (16 bytes)
    JoinEUI         string // hex string, 16 chars (8 bytes); set on Device.JoinEui in 1.1.x
}
```

### Pattern 5: Atomic transaction shape (extends Phase 1 `install/finish.go`)

**What:** Every Phase 2 mutation that touches both Postgres and ChirpStack uses Phase 1's atomic-Serializable-transaction pattern: `BEGIN tx → CS gRPC call → Shifter row insert → audit_log INSERT → COMMIT`. CS failure → ROLLBACK (CONTEXT D-16).

**When to use:** AddDevice, MeterSwap, DecommissionDevice, ProfileCreate (with codec_js push to CS), ProfileUpdate.

**Example:**
```go
// Source: pattern verified against internal/install/finish.go (Phase 1 D-10)
// internal/swap/commit.go
package swap

func CommitSwap(ctx context.Context, deps Deps, in SwapInput) error {
    tx, err := deps.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
    if err != nil {
        return fmt.Errorf("begin tx: %w", err)
    }
    defer func() { _ = tx.Rollback(ctx) }()

    // 1. Close outgoing binding at swap.confirm_time (D-14).
    if _, err := tx.Exec(ctx,
        `UPDATE binding SET valid_to = $1 WHERE id = $2 AND valid_to IS NULL`,
        in.ConfirmTime, in.OutgoingBindingID); err != nil {
        return fmt.Errorf("close outgoing binding: %w", err)
    }

    // 2. Open incoming binding with proposed offset (R - N) or operator override.
    var newBindingID uuid.UUID
    if err := tx.QueryRow(ctx,
        `INSERT INTO binding (metering_point_id, device_id, valid_from, reading_offset)
           VALUES ($1, $2, $3, $4) RETURNING id`,
        in.MeteringPointID, in.IncomingDeviceID, in.ConfirmTime, in.ProposedOffset,
    ).Scan(&newBindingID); err != nil {
        return fmt.Errorf("open incoming binding: %w", err)
    }

    // 3. Audit row INSIDE the same txn (D-23).
    if err := audit.WriteEntry(ctx, tx, audit.Entry{
        UserID:     in.UserID,
        Action:     "swap",
        EntityType: "binding",
        EntityID:   newBindingID,
        Before:     in.AuditBefore, // {binding: {old}, reading_offset: 0}
        After:      in.AuditAfter,  // {binding: {new}, reading_offset: <proposed>}
        Notes:      in.OperatorNotes,
        RequestID:  in.RequestID,
    }); err != nil {
        return fmt.Errorf("write audit: %w", err)
    }

    if err := tx.Commit(ctx); err != nil {
        return fmt.Errorf("commit swap: %w", err)
    }

    // 4. Invalidate resolver cache (D-25). Outside txn — failure here is a degraded
    // signal (resolver picks up next uplink as miss → re-loads), not a swap failure.
    deps.Resolver.Invalidate(in.OutgoingDevEUI)
    deps.Resolver.Invalidate(in.IncomingDevEUI)
    return nil
}
```

### Pattern 6: Stepped dialog reuse (Phase 1 `web/src/components/stepper.tsx`)

**What:** Every multi-step Phase 2 dialog (Add Device, Meter Swap, Profile Editor) reuses Phase 1's `Stepper` + `ResponsiveDialog`. Each step posts a draft to the backend or holds in local state; the final step does the atomic commit.

**When to use:** Add Device (4 steps per D-10), Meter Swap (3 steps: capture-R → propose-offset → confirm per D-12/D-13), Profile Editor (mostly single-page but the Codec → Save step is gated).

**Example (verified pattern reference — see existing Phase 1 install wizard):**
```tsx
// Source: web/src/routes/install/wizard.tsx (Phase 1 install wizard) — pattern reuse
// web/src/routes/devices/add-device-dialog.tsx
import { Stepper } from "@/components/stepper";
import { ResponsiveDialog } from "@/components/responsive-dialog";

export function AddDeviceDialog({ open, onOpenChange }: Props) {
  const [step, setStep] = useState(0);
  const [draft, setDraft] = useState<AddDeviceDraft>({});
  const submit = useMutation({ mutationFn: () => apiFetch.post("/api/devices", draft) });

  return (
    <ResponsiveDialog open={open} onOpenChange={onOpenChange} title="Add device">
      <Stepper currentStep={step} steps={["Sticker", "Profile", "Bind", "Review"]} />
      {step === 0 && <DevEUIStickerStep draft={draft} onNext={(d) => { setDraft(d); setStep(1); }} />}
      {step === 1 && <ProfileStep draft={draft} onNext={(d) => { setDraft(d); setStep(2); }} />}
      {step === 2 && <BindMPStep draft={draft} onNext={(d) => { setDraft(d); setStep(3); }} />}
      {step === 3 && <ReviewStep draft={draft} onSubmit={() => submit.mutate()} />}
    </ResponsiveDialog>
  );
}
```

### Pattern 7: pgx LISTEN/NOTIFY with dedicated connection (resolver invalidation, D-25)

**What:** `LISTEN binding_changed` runs on a connection acquired (and held) from the pool — pgxpool does not natively forward NOTIFY events across the pool. The connection is acquired at boot, kept open for the process lifetime, and reconnected on disconnect. NOTIFY is sent from any normal pool connection inside the swap transaction's COMMIT.

**Why dedicated:** [CITED: github.com/jackc/pgx/issues/1121] — LISTEN/NOTIFY does not work transparently with pgxpool's per-call connection model; one connection must be held open for the LISTEN.

**Example:**
```go
// Source: pattern verified against pgx v5 docs and jackc/pgx issue #1121
// internal/resolver/listener.go
package resolver

func (r *Resolver) Run(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) {
    for {
        if err := r.runOnce(ctx, pool, log); err != nil {
            if ctx.Err() != nil {
                return
            }
            log.Warn("resolver listener disconnected, reconnecting", "err", err)
            time.Sleep(2 * time.Second)
        }
    }
}

func (r *Resolver) runOnce(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) error {
    conn, err := pool.Acquire(ctx)
    if err != nil {
        return fmt.Errorf("acquire: %w", err)
    }
    defer conn.Release()

    if _, err := conn.Exec(ctx, "LISTEN binding_changed"); err != nil {
        return fmt.Errorf("LISTEN: %w", err)
    }
    log.Info("resolver listening on binding_changed")

    for {
        notif, err := conn.Conn().WaitForNotification(ctx)
        if err != nil {
            return fmt.Errorf("WaitForNotification: %w", err)
        }
        // payload = dev_eui (hex string)
        r.cache.Delete(strings.ToLower(notif.Payload))
        log.Debug("resolver invalidated", "dev_eui", notif.Payload)
    }
}
```

### Pattern 8: Audit log writer that runs inside caller's transaction (D-23)

**What:** `audit.WriteEntry(ctx, tx, entry)` takes a `pgx.Tx` (not a pool). Caller is responsible for opening + committing the txn. Audit row is part of the same atomic commit as the domain mutation.

**Why explicit tx parameter:** Eliminates the possibility that an audit write goes to a different connection — the audit row LITERALLY cannot exist without the domain row, by Postgres's atomicity guarantee.

```go
// internal/audit/log.go
type Entry struct {
    UserID     uuid.UUID  // from auth.UserFromContext(ctx) — set by Phase 1 session middleware
    Action     string     // create|update|archive|swap|decommission|profile_create|profile_update|binding_open|binding_close
    EntityType string     // site|metering_point|device|device_profile|binding
    EntityID   uuid.UUID
    Before     map[string]any // nil for CREATE; diff-only for UPDATE per D-24
    After      map[string]any // full for CREATE; diff-only for UPDATE per D-24
    Notes      string
    RequestID  string     // from chi middleware.RequestID set in Phase 1
}

func WriteEntry(ctx context.Context, tx pgx.Tx, e Entry) error {
    beforeJSON, _ := json.Marshal(e.Before)
    afterJSON, _ := json.Marshal(e.After)
    _, err := tx.Exec(ctx, `
        INSERT INTO audit_log (user_id, action, entity_type, entity_id, before, after, notes, request_id)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
        e.UserID, e.Action, e.EntityType, e.EntityID,
        beforeJSON, afterJSON, e.Notes, e.RequestID)
    return err
}
```

### Anti-Patterns to Avoid

- **Decoding payloads in Shifter** — codecs always run in ChirpStack's QuickJS sandbox (CONTEXT D-09, PITFALLS §3). Shifter consumes the decoded `object` field from the MQTT event. Adding a vendor = adding a `device_profile` + `device_profile_mapping` rows + uploading codec_js to CS, NEVER editing Go code.
- **Using `gateway_rx_time` as the hypertable's `time` column** — gateway clocks drift, late uplinks corrupt aggregates (PITFALLS §5). Always `time = server_ingest_time = time.Now().UTC()` at the moment Shifter receives the MQTT event.
- **Writing audit log async** — meter-swap audit MUST be atomic with swap. Async = lost rows on crash, races, ordering bugs. Sync inside same txn (D-23).
- **GORM, `lib/pq`, `gofpdf`** — explicitly forbidden by CLAUDE.md. Phase 2 uses sqlc + pgx/v5 only.
- **Decoding the MQTT payload bytes manually** — they're already JSON in v4, just `json.Unmarshal` into a struct.
- **Hand-rolling a TTL on the resolver cache** — would create a swap-then-uplink-attributed-to-old-binding race (PITFALLS §2). Use eventless `LISTEN/NOTIFY` invalidation per D-25.
- **Storing telemetry rows keyed by `device_id`** — PITFALLS §1 mandates `metering_point_id`. Code paths like `getConsumption(deviceID)` are forbidden by design.
- **A `decoders.ts`-equivalent file** — if `internal/ingest/normalize.go` ever grows a vendor switch statement, that's a sign the device_profile_mapping mechanism is being bypassed (PITFALLS §3 warning sign).
- **Silently dropping uplinks on decode failure** — set `quality = 'decode_fail'`, persist `raw_payload` + `decoded_object`, surface as a "X uplinks flagged" badge (D-26). Never drop.
- **Hard-deleting metering points or sites** — soft-delete only via `archived_at` (D-20). Audit trail and historical telemetry queries depend on this.
- **A "merge devices" admin tool** — PITFALLS §1 explicit warning sign. If this gets requested, it means the metering-point abstraction is being bypassed somewhere; fix the root cause.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Time-series storage with bucketed aggregation | Custom partitioned tables + cron-style rollups | TimescaleDB hypertable + chunk_time_interval (Phase 5 adds CAGGs) | Time-bucket math, late-arrival handling, retention boundaries are footguns (PITFALLS §6); TimescaleDB has solved them |
| Schema migrations | Hand-written `IF NOT EXISTS` patches | golang-migrate library mode (Phase 1 D-13/D-16) | Versioning, ordering, rollback, dirty-state recovery already correct |
| Postgres queries with type safety | Hand-written `pgx.Rows.Scan` for every query | sqlc generated code | Type-safe, refactor-safe, and TimescaleDB-specific functions (`time_bucket`, `locf`) work natively because sqlc is SQL-first |
| LoRaWAN payload decoding (per-vendor) | Vendor switch in Go | ChirpStack QuickJS sandbox via `device_profile.payload_codec_script` | CONTEXT D-09 + PITFALLS §3; codec catalog (TheThingsNetwork lorawan-devices) already exists |
| MQTT subscriber with reconnect | Custom paho wrapper | Phase 1 `internal/chirpstack/mqtt.go` already wraps paho with OnConnect re-subscribe + `SetCleanSession(false)` for QoS1 resume | Phase 1 reuses RESEARCH §Pattern 10 — Phase 2 just plugs `UplinkHandler` |
| ChirpStack gRPC stubs | Hand-rolled REST client | `github.com/chirpstack/chirpstack/api/go/v4` (already in go.mod) | Generated from canonical .proto files; auto-tracks v4 changes |
| Session management for audit user_id capture | Custom JWT | scs/v2 (Phase 1) — `auth.UserFromContext(ctx)` returns `*User` with ID | Already wired in Phase 1; just call from handlers |
| TanStack Table for the devices list and mapping editor | Hand-rolled `<table>` with sort | `@tanstack/react-table` v8 + shadcn `table` primitive | Sorting, pagination, column visibility — all standard and well-documented |
| DevEUI hex parsing + endianness flip | Custom regex + byte reverse | `internal/device/deveui.go` thin function over `encoding/hex` + manual `reverseBytes` | This IS the right level — NOT a library; just a small utility. Keep tested. |
| Background job queue for Phase 2 | River (skip for now) | Run audit, ingest, CS sync synchronously per CONTEXT D-23 + D-25 + D-16 | River install can defer to Phase 5 (CAGG refresh is the first real River use case) |
| Test infrastructure | Custom Mosquitto + Postgres test rig | testcontainers-go (already in `go.mod`) + `tc-mosquitto` module | Phase 1 already tests against ephemeral Postgres+TimescaleDB; Phase 2 adds Mosquitto to the same harness |
| JSON Pointer (RFC 6901) lookup for the mapping editor | Custom path navigator | Stdlib `encoding/json` + small recursive helper, OR `github.com/qri-io/jsonpointer` if needed | At most ~30 lines of recursive map lookup; library is overkill — but if planner finds path-with-array-indices ambiguity, qri-io is the standard pick |
| Synthetic uplink generator | Hand-coded ChirpStack v4 uplink event JSON | Build a tiny `internal/testharness/codec_axioma.go` that produces matching byte payloads + minimum CS event envelope (per ChirpStack `event/up` schema) | The CS event envelope is a stable v4 protobuf; the codec produces deterministic bytes → CS QuickJS produces deterministic decoded `object` → Shifter normalizes → measurement row. End-to-end without physical hardware. |

**Key insight:** Phase 1 has already done the hardest packaging decisions (single Go binary, bundled vs external compose, embedded SPA, golang-migrate as library, scs/v2 sessions, paho with OnConnect re-subscribe, sqlc + pgx). Phase 2's job is to **add domain code on top of those rails** — not introduce new infrastructure. The only "new" infrastructure is the TimescaleDB hypertable creation in migration `0014`, and that's a one-line `SELECT create_hypertable(...)`.

## Runtime State Inventory

> Phase 2 is mostly greenfield (new tables, new packages) but it does extend Phase 1 runtime state. Listed for completeness so the planner is aware.

| Category | Items Found | Action Required |
|----------|-------------|-----------------|
| Stored data | **Postgres tables ADDED** by `0007..0015`: site, metering_point, device_profile, device, device_profile_mapping, binding, measurement (hypertable), audit_log. **Phase 1 tables UNCHANGED:** user, session, install_state, install_identity, chirpstack_connection. | Migrations run automatically on `shifter serve` boot per Phase 1 D-13 (already wired in `internal/db/migrations.go`). Plan must verify migrations are reversible (down.sql files present). |
| Live service config | **ChirpStack:** Phase 2's `bootstrap.go` will create one tenant (`shifter-default`) and one application (`shifter`) on first boot. Their CS UUIDs MUST be persisted in Shifter's `chirpstack_connection` row to make subsequent boots idempotent. **External-CS mode:** if the API token's tenant context already names a tenant, REUSE it (don't create a duplicate). | New columns on `chirpstack_connection`: `cs_tenant_id UUID NULL`, `cs_application_id UUID NULL`. Migration `0008` (or sequenced before `0011_device`) ALTERs the table. |
| OS-registered state | None — Phase 2 does not register systemd units, cron jobs, OS-level services, or background runners beyond the existing Shifter binary's serve loop. River is NOT installed yet (CONTEXT defers to Phase 5). | None. |
| Secrets/env vars | No new env vars in Phase 2. CS API token already loaded via Phase 1's secrets-by-ref pattern (`api_token_ref` → `/run/secrets/...`). MQTT credentials already loaded the same way. | None. |
| Build artifacts | sqlc-generated code in `internal/db/sqlc/` will GROW with Phase 2 query files. Run `just sqlc` after each `internal/db/queries/*.sql` addition. The frontend bundle in `web/dist/` will GROW with new routes/dialogs (still no Leaflet, kept lean per D-18). | Plan must include `just sqlc` step after each new query file is added. |

## Common Pitfalls

### Pitfall 1: Storing telemetry keyed by `device_id` instead of `metering_point_id`
**What goes wrong:** Schema looks "intuitive" mirroring CS's device-centric data model. First meter swap blows up: history splits across two devices, charts have a discontinuity, year-over-year reports break.
**Why it happens:** ChirpStack is device-centric (DevEUI is its natural key). Mirroring 1:1 is the easy mistake.
**How to avoid:** Make `measurement.metering_point_id UUID NOT NULL` the schema invariant — there is NO `device_id` column on `measurement` (the device is implied by the active binding at write time). Resolver does the dev_eui→MP translation at ingest. CONTEXT D-02 + DATA-01 codify this.
**Warning signs:** A query like `SELECT * FROM measurement WHERE device_id = ...` exists. A "merge devices" admin tool gets requested.

### Pitfall 2: Reading-offset math broken at swap or rollover
**What goes wrong:** Two related modes — (a) replacement: new meter starts at 0, displayed cumulative drops, leak alerts fire; (b) rollover: 32-bit counter wraps, `delta = current - previous` returns huge negative, daily aggregates show negative consumption.
**Why it happens:** Naive model conflates "what the device counter says" with "what the customer should see."
**How to avoid:** Store `raw_value` (never overwrite). Compute `cumulative_value = raw_value + binding.reading_offset` at insert time. On rollover (`raw_t < raw_t-1` AND not at binding boundary): `binding.reading_offset += device_profile.counter_modulus` and log `device_health` event. Per-binding window math means crossing a boundary is by construction continuous because the offset was chosen to make `display_at_swap_old = display_at_swap_new`. CONTEXT D-02 + D-05 + D-13 codify this; testharness D-27 verifies.
**Warning signs:** Code path `if (current < previous) { current = previous }` (silent clamp). A function `getConsumption(deviceID)` exists. Operator support tickets about "the chart looks weird after we changed the meter."

### Pitfall 3: Resolver cache staleness causes uplink-attributed-to-old-binding
**What goes wrong:** Operator commits a swap. The cached resolver entry for `dev_eui` still points to the old binding for some window (TTL, polling). An uplink that arrives in that window is attributed to the OLD metering point. Telemetry corruption that's hard to detect.
**Why it happens:** Caches are tempting; TTLs are easy. But TTL = a guaranteed staleness window.
**How to avoid:** Eventless invalidation via Postgres `LISTEN/NOTIFY`. Swap commit publishes `NOTIFY binding_changed, '<dev_eui>'`. Resolver listens on a dedicated pgx connection and deletes the cache entry on receipt. CONTEXT D-25 codifies this.
**Warning signs:** `cache.SetWithTTL(deveui, ..., 5*time.Minute)`. A `// TODO: invalidate cache on swap` comment. A test that "occasionally fails" under load.

### Pitfall 4: Using `gateway_rx_time` as the hypertable `time` column
**What goes wrong:** One gateway has bad NTP and is 90s off. Another buffers offline for 6 hours then floods. CAGGs that already refreshed don't see the late data — daily totals silently wrong. (PITFALLS §5)
**Why it happens:** Multiple timestamps in CS uplink event (gateway_rx_time, server time, device-side time) all "look like" when the data happened.
**How to avoid:** **Persist server-side ingest time as authoritative `time` column.** This is monotonic, deduplicated by ChirpStack, never goes backward. CONTEXT D-02 + DATA-03 + PITFALLS §5 codify this. Persist `gateway_rx_time` as a diagnostic column for gateway-clock-skew monitoring.
**Warning signs:** Daily totals change retroactively when re-running an aggregate. Two adjacent rows have same `(dev_eui, fcnt)` with different `time`s.

### Pitfall 5: ChirpStack QuickJS codec returns malformed object — Shifter silently drops
**What goes wrong:** Vendor ships firmware update mid-fleet that changes byte 7 from "battery %" to "battery mV." Codec produces wrong shape. Shifter normalizer fails on missing canonical field. Uplinks just... don't appear in the dashboard. Operator notices when reports are wrong.
**Why it happens:** "Reject + log" is easier than "persist with quality flag."
**How to avoid:** **Persist all uplinks, never silent-drop.** `quality TEXT` flag (CONTEXT D-26): set to `decode_fail`, `missing_canonical`, `out_of_range`, `duplicate_fcnt` as appropriate. Persist `raw_payload` and `decoded_object` always. Surface "X uplinks flagged" badge on device detail (Phase 4 picks this up).
**Warning signs:** Dashboard "missing data" complaints. Code path `if (decode_fail) { return; }` (silent return).

### Pitfall 6: Acrel ADL200 + ADW300 codec divergence (one codec, two profiles)
**What goes wrong:** Plan ships two separate codec_js files for ADL200 and ADW300, drift creeps in over time, bug fixes only land in one.
**Why it happens:** They look like different products on Acrel's website.
**How to avoid:** **One codec_js implementation, two `device_profile` rows, two `device_profile_mapping` row sets.** ADL200 mapping enables `cumulative + instant_power + battery`; ADW300 enables those plus `multi_phase + power_quality + extra` JSONB pointers for L2/L3 fields. CONTEXT specifics line 186 codifies this.
**Warning signs:** Two files named `acrel_adl200.js` and `acrel_adw300.js` with 95% overlap.

### Pitfall 7: Audit log written async (eventually-consistent)
**What goes wrong:** Crash between domain mutation and audit insert → mutation happened, audit row missing. Compliance hole. AUDIT-01 violation.
**Why it happens:** "Async = faster" instinct.
**How to avoid:** Audit writer takes a `pgx.Tx` parameter, runs INSIDE the caller's transaction. Atomic by construction. CONTEXT D-23 codifies.
**Warning signs:** `go writeAudit(...)` (goroutine fire-and-forget). River job named `audit_log`.

### Pitfall 8: AS923 sub-plan mismatch (Thailand specific)
**What goes wrong:** Operator selects "AS923" generic; devices tuned for AS923-2 (Thailand). Devices never join. (PITFALLS §8)
**Why it happens:** AS923 has 4 sub-plans with overlapping but distinct channel frequencies.
**How to avoid:** Phase 1 already shipped the regulator-aware region picker (INST-04, AS923-2 default for Thailand). Phase 2's `0009_device_profile` schema MAY want a `region TEXT` column on device_profile (defaulting to install region) so per-profile region overrides are possible. Discretion — planner decides.
**Warning signs:** Devices "join intermittently" or "join then go silent."

### Pitfall 9: Migration `0010_seed_profiles` fails to push codec_js to ChirpStack on first boot
**What goes wrong:** SQL migration runs, inserts 3 device_profile rows. Boot continues. ChirpStack never receives the codec_js — uplinks for those profiles return undecoded objects. Profile editor "save" doesn't realize a sync has never succeeded.
**Why it happens:** Schema migration and CS API call are different transactions (CS is gRPC, not Postgres). Race between boot order and CS reachability.
**How to avoid:** Boot routine in `internal/profile/seed.go` runs AFTER migrations. Iterates `device_profile WHERE cs_profile_id IS NULL OR codec_js_synced_at IS NULL`. For each: call `DeviceProfileService.Create` (or `Update` if `cs_profile_id IS NOT NULL`). On success: UPDATE the row's `cs_profile_id` + `codec_js_synced_at`. On failure: log warning, retry on next boot. Boot does NOT block on this (CS may be temporarily unreachable in external mode). Add a Settings → "Sync profiles to ChirpStack" button as a manual fallback.
**Warning signs:** First-boot logs show "profile sync failed" but no operator visibility. Uplink decode_fail rate spikes for the seed profiles.

### Pitfall 10: Binding exclusion constraint without `btree_gist` extension
**What goes wrong:** Migration `0013_binding` ALTER TABLE adds the EXCLUDE constraint using GIST on tstzrange. Postgres errors: "operator class for type uuid does not exist for access method gist."
**Why it happens:** Combining a UUID equality with a tstzrange overlap in EXCLUDE requires the `btree_gist` extension.
**How to avoid:** `CREATE EXTENSION IF NOT EXISTS btree_gist;` BEFORE the EXCLUDE constraint. Bundled compose Postgres image already has this available; migration just needs to enable it.
**Warning signs:** Migration error "operator class ... gist."

### Pitfall 11: ChirpStack v4 uplink event MQTT payload structure mismatch
**What goes wrong:** Code expects `event.object` as a JSON string (v3 behavior), but in v4 it's a struct (`google.protobuf.Struct`). `json.Unmarshal` into the wrong shape silently produces an empty object. (PITFALLS §7 + D-09)
**Why it happens:** Old Stack Overflow answers reference v3.
**How to avoid:** ChirpStack v4 publishes uplink events as JSON-marshalled `UplinkEvent` proto. The `object` field is an arbitrary JSON object (whatever the QuickJS codec returned). Decode as `map[string]any` or `json.RawMessage` and pass to the device_profile_mapping engine. Phase 1 codebase already pins v4 (`api/go/v4`).
**Warning signs:** `decoded_object` JSONB column always empty. CHIRP-04 tests pass but dashboard shows no canonical fields.

### Pitfall 12: pgxpool LISTEN/NOTIFY connection leak
**What goes wrong:** Pool-acquired connection holds the LISTEN forever. Connection dies on network blip; Resolver silently stops invalidating. Swaps don't propagate. Bug only shows up days later.
**Why it happens:** pgxpool doesn't natively expose "keep this connection alive forever."
**How to avoid:** Pattern from Pattern 7 above — outer reconnect loop. Acquire conn, run LISTEN, WaitForNotification in a loop, on error release + reacquire after a backoff. Pinged by Phase 1 OnConnect-style health metric exposed on `/health/detailed` (Phase 6). In Phase 2: log on disconnect+reconnect; testharness has a "kill resolver conn" scenario to verify auto-recovery.
**Warning signs:** Resolver invalidation count flatlines after a network event.

## Code Examples

Verified patterns from official sources and Phase 1 codebase:

### Hypertable creation
```sql
-- Source: docs.tigerdata.com/api/latest/hypertable/create_hypertable
-- (verified syntax for TimescaleDB 2.26+)
SELECT create_hypertable('measurement', 'time', chunk_time_interval => INTERVAL '1 day');
```

### ChirpStack v4 DeviceProfileService.Create (with QuickJS codec)
```go
// Source: github.com/chirpstack/chirpstack/blob/master/api/proto/api/device_profile.proto
// Source: extends pattern from internal/chirpstack/version.go (Phase 1)
import "github.com/chirpstack/chirpstack/api/go/v4/api"

func (c *Client) CreateDeviceProfile(ctx context.Context, in CreateProfileInput) (string, error) {
    svc := api.NewDeviceProfileServiceClient(c.conn)
    resp, err := svc.Create(ctx, &api.CreateDeviceProfileRequest{
        DeviceProfile: &api.DeviceProfile{
            TenantId:           in.TenantID,
            Name:               in.Name,
            Region:             in.Region,             // e.g. api.Region_AS923 (sub-plan via region_config_id)
            MacVersion:         in.MACVersion,         // e.g. api.MacVersion_LORAWAN_1_0_3
            RegParamsRevision:  in.RegParamsRevision,
            PayloadCodecRuntime: api.CodecRuntime_JS,  // CONTEXT D-09: codec runs in QuickJS
            PayloadCodecScript: in.CodecJS,            // the JavaScript decoder source
            // ... other fields per CS docs
        },
    })
    if err != nil {
        return "", fmt.Errorf("DeviceProfileService.Create: %w", err)
    }
    return resp.Id, nil
}
```

### ChirpStack v4 TenantService.Create (idempotent bootstrap)
```go
// Source: github.com/chirpstack/chirpstack/blob/master/api/proto/api/tenant.proto (verified)
func (c *Client) EnsureTenant(ctx context.Context, name string) (string, error) {
    svc := api.NewTenantServiceClient(c.conn)
    // Try to find existing tenant by name first.
    list, err := svc.List(ctx, &api.ListTenantsRequest{Limit: 100, Search: name})
    if err == nil {
        for _, t := range list.Result {
            if t.Name == name {
                return t.Id, nil
            }
        }
    }
    // Not found — create it.
    resp, err := svc.Create(ctx, &api.CreateTenantRequest{
        Tenant: &api.Tenant{
            Name:                 name,
            Description:          "Shifter-managed tenant — do not edit",
            CanHaveGateways:      true,
            PrivateGatewaysUp:    true,
            PrivateGatewaysDown:  true,
            MaxGatewayCount:      0, // unlimited
            MaxDeviceCount:       0, // unlimited
        },
    })
    if err != nil {
        return "", fmt.Errorf("TenantService.Create: %w", err)
    }
    return resp.Id, nil
}
```

### sqlc query for "find active binding for dev_eui at time T"
```sql
-- Source: extends pattern from internal/db/queries/users.sql (Phase 1)
-- internal/db/queries/bindings.sql

-- name: GetActiveBindingByDevEUI :one
-- Resolver hot-path: dev_eui → metering_point_id at the moment an uplink arrives.
-- Returns the binding whose [valid_from, COALESCE(valid_to, infinity)) covers t = now().
-- Uses indexes binding_device_active_idx + the exclusion constraint guarantee that
-- at most one row matches (no overlap per metering_point_id).
SELECT b.id, b.metering_point_id, b.device_id, b.valid_from, b.valid_to, b.reading_offset,
       d.dev_eui, dp.counter_modulus
FROM binding b
JOIN device d ON d.id = b.device_id
JOIN device_profile dp ON dp.id = d.device_profile_id
WHERE d.dev_eui = $1
  AND b.valid_from <= $2
  AND (b.valid_to IS NULL OR b.valid_to > $2)
LIMIT 1;
```

### Synthetic uplink publication (testharness)
```go
// Source: pattern using paho.mqtt.golang and testcontainers-go (Phase 1 testsupport)
// internal/testharness/runner.go

func PublishSyntheticUplink(ctx context.Context, brokerURL, appID, devEUI string, payload []byte, fcnt uint32, rxTime time.Time) error {
    topic := fmt.Sprintf("application/%s/device/%s/event/up", appID, devEUI)
    event := map[string]any{
        "deduplicationId":   uuid.NewString(),
        "time":              rxTime.UTC().Format(time.RFC3339Nano),
        "deviceInfo":        map[string]any{"devEui": devEUI, "applicationId": appID},
        "data":              base64.StdEncoding.EncodeToString(payload),
        "fCnt":              fcnt,
        "fPort":             100, // Axioma W1 default port
        "rxInfo":            []map[string]any{{"gatewayId": "synthetic", "time": rxTime.UTC().Format(time.RFC3339Nano), "rssi": -85, "snr": 9.2}},
        "object":            map[string]any{}, // populated by ChirpStack QuickJS — for testharness pre-seeded mode, populate directly here to skip QuickJS
    }
    body, _ := json.Marshal(event)

    opts := mqtt.NewClientOptions().AddBroker(brokerURL).SetClientID("testharness-" + uuid.NewString())
    cli := mqtt.NewClient(opts)
    if t := cli.Connect(); t.Wait() && t.Error() != nil {
        return t.Error()
    }
    defer cli.Disconnect(100)
    if t := cli.Publish(topic, 1, false, body); t.Wait() && t.Error() != nil {
        return t.Error()
    }
    return nil
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `create_hypertable()` function form only | New `CREATE TABLE WITH (tsdb.hypertable, ...)` syntax also supported | TimescaleDB 2.23+ | Either works; CONTEXT D-06 + Pattern 2 use the function form (more universal in tooling/docs); planner may switch to WITH-clause if preferred |
| Trigger-based CAGG invalidation | Direct CAGG invalidation tracking | TimescaleDB 2.26.0 (March 2026) | 10-20% faster ingest; no Phase 2 action needed (Phase 5 enables CAGGs) |
| ChirpStack v3 organization model (numeric IDs, REST shim) | ChirpStack v4 tenant model (UUIDs, gRPC-only) | CS v4 GA | Phase 1 already pins v4; Phase 2 first uses v4 TenantService — UUIDs end-to-end |
| `decoded_object` as JSON string in v3 | `decoded_object` as struct (`google.protobuf.Struct`) in v4 | CS v4 GA | Already accounted for — `internal/ingest/decode.go` reads as `map[string]any` |
| `golang.org/x/text/language` for tz handling | `time.LoadLocation` (stdlib) suffices | Phase 1 already uses stdlib | No change for Phase 2 |
| pq driver | pgx/v5 | Phase 1 | Already on pgx |

**Deprecated/outdated (already avoided in Phase 1, do not regress):**
- GORM — explicitly forbidden by CLAUDE.md (sqlc + pgx instead)
- `lib/pq` — maintenance mode since 2021 (pgx instead)
- `gofpdf` — archived 2021 (Phase 5 will use maroto/v2 if PDF needed; not Phase 2)
- ChirpStack v3 (numeric IDs, REST API) — Phase 1 INST-05 already refuses v3
- Decoding payloads in backend code — CONTEXT D-09 + PITFALLS §3 forbid this
- Per-vendor DB tables — CONTEXT D-03 explicitly chose single hypertable

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | The Acrel ADW300 LoRaWAN payload byte layout follows a Modbus-derived structure with 3-phase voltage/current/PF/THD fields per phase. The exact byte positions and scaling factors must be derived at plan time from the official Acrel ADW300 manual PDF (https://www.acrelenergy.com/uploads/file/adw300-manual.pdf). | Vendor Profiles (D-07) | Plan-time risk: planner must read the manual or talk to Acrel before writing the codec_js. If layout differs from assumption, codec needs rewriting (low-impact, codec is small). If multi_phase mapping is wrong, dashboard shows wrong values (medium-impact, but caught by testharness round-trip if synthetic payloads match real ones). |
| A2 | The Acrel ADL200 (1-phase) shares the Acrel ADW300 payload codec family — i.e., one codec_js handles both, with the mapping table determining which canonical fields are populated. This is asserted in CONTEXT (specifics line 186) but the actual ADL200 manual was not read in this research session. | Vendor Profiles (D-07, D-09) | If they DON'T share a codec, plan needs two separate codec_js files (small refactor — CONTEXT decision still holds, just two seed entries instead of one shared). If they do share but the Acrel team confirms ADL200 has different fPort or framing, the codec needs a runtime detection branch. |
| A3 | ChirpStack v4's `payload_codec_runtime` enum has the value name `JS` (not `JAVASCRIPT` or `QUICKJS`). | Code Examples (Pattern Ex DeviceProfileService.Create) | LOW — verified via the `device_profile.proto` query in this research session. Caller code line `api.CodecRuntime_JS` will be checked at compile-time against the generated stubs at plan time. |
| A4 | ChirpStack v4's ApplicationService and TenantService Create calls return the new resource ID in `resp.Id`. | Pattern 4 + Code Examples | LOW — verified general v4 idiom. Planner will confirm exact field names against generated stubs. |
| A5 | The single global ChirpStack tenant + 1 application model (D-28) is sufficient for a single-tenant-per-install deployment. No customer in v1 will hit a CS-side per-application device limit (default limit is unlimited; customer would need >100K devices). | ChirpStack Abstraction (D-28) | LOW — CONTEXT explicitly accepted this tradeoff and documented "revisit only if a multi-site customer hits a clear scaling or isolation requirement (none expected for single-tenant single-install)." |
| A6 | The `binding_changed` Postgres NOTIFY channel name is unique to Shifter (no other extension uses it). | Pattern 7 | NEGLIGIBLE — channel names are per-database. Phase 4 will reuse the same channel for SSE per CONTEXT D-25. |
| A7 | Mosquitto's testcontainers-go module exists or can be substituted by `eclipse-mosquitto:2` raw image with `testcontainers.GenericContainer`. | Synthetic Test Harness (D-27) | LOW — a generic-container fallback is straightforward; planner verifies module availability or uses raw image. |
| A8 | `extra` JSONB at ~1KB per row × 100K rows/day × 365 days = ~37GB/year/MP at the high end. With 100 MPs at 1-min interval (worst case for v1), raw `measurement` table grows ~3.7TB/year before compression. Phase 5 will add TimescaleDB compression policy reducing this 5-10×. | Schema (D-02, D-08) | MEDIUM — if real customer telemetry is bigger than estimated, Phase 5 compression timeline becomes urgent. Phase 2 should NOT optimize for this — premature. Document in Phase 5 RESEARCH. |

## Open Questions (RESOLVED)

1. **Should the binding `valid_to` boundary be inclusive or exclusive (`[valid_from, valid_to)` vs `(valid_from, valid_to]`)?**
   - What we know: PostgreSQL's `tstzrange(..., ..., '[)')` is the conventional half-open interval and matches CONTEXT D-14's `gateway_rx_time < valid_to` rule.
   - What's unclear: edge case where `gateway_rx_time == valid_to` exactly — under `[)` semantics, that uplink is attributed to the NEW binding. CONTEXT D-14 says `<` (strict less-than) → old binding gets it. Slight contradiction.
   - Recommendation: planner picks `[)` (Postgres convention) and updates D-14's text in CONTEXT to `gateway_rx_time < valid_to` → old, `gateway_rx_time >= valid_to` → new. Document in plan.
   - **RESOLVED:** Half-open `[valid_from, valid_to)` per Plan 02-03 SUMMARY. The btree_gist EXCLUDE constraint in `internal/db/migrations/0014_binding.up.sql` uses tstzrange `[)` semantics; CONTEXT D-14 text was reconciled to read "gateway_rx_time < valid_to → old binding, gateway_rx_time >= valid_to → new binding." Citation: `internal/db/migrations/0014_binding.up.sql` (EXCLUDE definition) + `02-03-SUMMARY.md` + regression test `TestBinding_HalfOpenInterval` in `internal/db/binding_test.go`.

2. **Should `device_profile.region` be a column on the device_profile table (per-profile region override) or always inherit from `chirpstack_connection.region_name` (install-wide)?**
   - What we know: Phase 1 already locks an install-wide region picker (INST-04, AS923-2 default for Thailand).
   - What's unclear: A multi-vendor deployment might have profiles built for different sub-plans. CONTEXT doesn't explicitly say.
   - Recommendation: planner adds `device_profile.region TEXT NULL` (NULL = inherit from install). Profile editor surfaces an "Override region" advanced toggle. Avoids a forward-compat migration in Phase 3.
   - **RESOLVED:** Per-profile column `device_profile.region TEXT NULL` (NULL = inherit from install-wide chirpstack_connection.region_name) per Plan 02-02 schema migration `0009_device_profile.up.sql`. Profile editor surfaces the override per UI-SPEC §Profile editor identity row (Plan 02-08 + Plan 02-14 mapping editor). Citation: `internal/db/migrations/0009_device_profile.up.sql` + `02-02-SUMMARY.md` + `web/src/routes/profiles/mapping-editor.tsx` (Region field in identity row).

3. **What happens to in-flight uplinks during a swap if `gateway_rx_time` is missing from the CS event?**
   - What we know: ChirpStack v4 always populates `rxInfo[].time` for received packets.
   - What's unclear: simulated/replayed uplinks (testharness) must populate this. Operator-injected events (none in Phase 2) might not.
   - Recommendation: ingest pipeline falls back to server-ingest-time (`time = time.Now()`) if `rxInfo[].time` is missing AND sets `quality = 'missing_canonical'` with a `notes` JSONB field explaining "missing gateway_rx_time, used server time." Documented behavior.
   - **RESOLVED:** Ingest pipeline falls back to server-ingest-time (`time.Now().UTC()`) when `rxInfo[].time` is missing, AND records the substitution. Concurrent swap commits use Serializable txn with first-commit-wins semantics: the second commit hits pgconn 23P01 (exclusion_violation from the binding btree_gist EXCLUDE) and is mapped to HTTP 409 "concurrent_swap" by Plan 02-11's swap handler. Citation: Plan 02-09 SUMMARY `internal/ingest/decode.go` `TestDecodeChirpStackEvent_EarliestGatewayRxTime` + Plan 02-07 SUMMARY `internal/swap/commit.go` + `TestCommitSwap_ConcurrentOneWins` + Plan 02-11 SUMMARY `internal/swap/handlers.go` 23P01 → 409 mapping + `TestSwapHandler_Concurrent_409`.

4. **Should the `audit_log.before`/`after` JSONB diffs preserve null-valued keys explicitly (`{archived_at: null}`) or use missing-key semantics (`{}`)?**
   - What we know: CONTEXT D-24 doesn't specify; planner discretion.
   - What's unclear: Phase 6 audit browse UI will need to render diffs; missing-key vs explicit-null affects rendering.
   - Recommendation: explicit nulls. JSON `{archived_at: null}` distinguishes "this field changed FROM something TO null" from "this field wasn't part of the diff." Document in plan.
   - **RESOLVED:** Explicit null preserved (`{field: null}` in `before`/`after` JSONB) per Plan 02-07. `internal/audit/diff.go ChangedFields(before, after)` produces field-level diffs with EXPLICIT-NULL semantics for added/removed keys (Open Q #4) — distinguishing "field absent from diff" (key not in JSONB) from "field changed FROM something TO null" (key present with null value). Phase 6 audit browse UI will render the two cases distinctly. Rollover audit rows use `action: "rollover.detected"`, `before` = previous reading snapshot, `after` = current reading + advanced offset. Citation: `internal/audit/diff.go` + `02-07-SUMMARY.md` line 84 (key-decisions: "ChangedFields produces field-level diffs with EXPLICIT-NULL semantics") + Plan 02-09 `internal/ingest/persist.go` rollover branch + tests `TestChangedFields_FieldChanged` + `TestChangedFields_FieldRemoved` + `TestChangedFields_FieldAdded`.

5. **Should `0010_seed_profiles` be a SQL migration (CONTEXT specifies "yes") OR a Go-code seed routine called after migration?**
   - What we know: CONTEXT D-09 says SQL migration `0010_seed_profiles.up.sql` inserts the rows; a separate Go boot routine pushes codec_js to ChirpStack.
   - What's unclear: codec_js can be ~5-30KB of JavaScript. SQL string literal escaping is annoying but works. Alternative: SQL migration inserts placeholder rows; Go routine populates `codec_js` from a `.js` file embedded via `//go:embed`.
   - Recommendation: planner picks `//go:embed` for codec_js source files (`internal/profile/codecs/axioma_w1.js`, `acrel_family.js`) and the migration just inserts the metadata rows with empty `codec_js`. Boot routine reads the embedded JS, fills in the row's `codec_js`, then syncs to ChirpStack. Cleaner than SQL string literals.
   - **RESOLVED:** //go:embed from `internal/profile/codecs/*.js` per Plan 02-08. Migration `0010_seed_profiles.up.sql` inserts profile metadata rows (slug + name + vendor + family + capabilities + counter_modulus + mac_version) with codec_js LEFT EMPTY; `internal/profile/codecs/embed.go` //go:embed-s the `axioma_w1.js` + `acrel_family.js` source files; `internal/profile/seed.go::RunSeedSync` at boot reads the embedded JS via `codecs.CodecBySlug(slug)`, pushes it to ChirpStack via gRPC, and persists the body + `cs_profile_id` + `codec_js_synced_at` in a Serializable txn. Citation: `internal/profile/codecs/embed.go` + `internal/profile/seed.go::RunSeedSync` + `02-08-SUMMARY.md` lines 42, 102 + `internal/db/migrations/0010_seed_profiles.up.sql` (metadata-only inserts).

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | Backend build | ✓ | 1.25.0 (verified `go.mod`) | — |
| Postgres + TimescaleDB | Schema + hypertable | ✓ | TimescaleDB 2.26 in bundled compose; external compose requires customer to run TS on their Postgres | `Phase 1 install kit must verify TS extension on connect — already in Phase 1 INST-05 territory; if missing, Phase 2 must add a startup probe and refuse to start with a clear error message: "TimescaleDB extension required — please install on your Postgres instance"` |
| ChirpStack v4 | Control plane (Phase 2 first uses TenantService etc.) | ✓ | v4 (bundled mode); customer-provided in external mode | Phase 1 INST-05 already refuses v3; Phase 2 adds runtime probes for the new gRPC services in case CS is misconfigured |
| Mosquitto MQTT broker | Uplink ingest | ✓ | 2.x in bundled compose; customer-provided in external mode | None; Phase 1 INST-03 wizard already collects MQTT URL |
| Docker (testcontainers-go) | Integration tests + testharness CLI | ✓ in dev/CI | Customer machines may not have Docker — that's fine, testharness is a dev/CI tool not a runtime requirement | CLI tells operator "Docker required" if missing |
| `pnpm` | Frontend build | ✓ | 10.33.2 (Phase 1 verified `package.json` packageManager) | — |
| Node 22.12+ | Frontend build | ✓ | engines specified in `web/package.json` | — |
| TanStack Table v8 | Mapping editor + devices list | Not yet installed | Need `pnpm add @tanstack/react-table` in Plan 1 of Phase 2 | — |
| `btree_gist` Postgres extension | binding exclusion constraint (Pitfall 10) | ✓ in TimescaleDB image (and most Postgres distributions) | bundled with Postgres 16/17 | Migration `0013_binding` runs `CREATE EXTENSION IF NOT EXISTS btree_gist;` first |

**Missing dependencies with no fallback:**
- None. All Phase 2 dependencies are present in Phase 1's already-shipped bundled stack OR are pure code additions (TanStack Table) that install cleanly via pnpm.

**Missing dependencies with fallback:**
- Mosquitto-specific testcontainers-go module: if not available as `tc-mosquitto`, fall back to `testcontainers.GenericContainer` with `eclipse-mosquitto:2` image (verified pattern; works equivalently).

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Backend framework | Go testing + testify v1.11.1 + testcontainers-go v0.42.0 |
| Backend config file | none (Go convention; `*_test.go` files alongside source) |
| Frontend framework | vitest (via `@vitest/ui` 4.1.5) + @testing-library/react 16.3.2 + @testing-library/user-event 14.6.1 |
| Frontend config file | `web/vite.config.ts` (existing, Phase 1) |
| Quick run command (backend) | `go test ./internal/...` (excludes integration tests via build tag) |
| Quick run command (frontend) | `cd web && pnpm test:run` |
| Full suite command (backend) | `go test -tags=integration ./...` (includes testcontainers-go) |
| Full suite command (frontend) | `cd web && pnpm test:run` |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| SITE-01 | Create/edit/archive site via dialog | unit (handler) + e2e (vitest dialog) | `go test ./internal/site/...` + `pnpm test:run -- create-site-dialog` | ❌ Wave 0 |
| DATA-01 | Telemetry keyed by metering_point_id | integration (testcontainers + ingest pipeline) | `go test -tags=integration ./internal/ingest/...` | ❌ Wave 0 |
| DATA-02 | Binding window + reading_offset | unit (sqlc query) + integration (overlap exclusion) | `go test ./internal/db/... -tags=integration` | ❌ Wave 0 |
| DATA-03 | Server-side ingest time authoritative | integration (testcontainers ingest, assert measurement.time == time.Now() at receipt ± tolerance) | `go test -tags=integration ./internal/ingest/...` | ❌ Wave 0 |
| DATA-04 | Meter swap dialog | unit (math) + integration (full swap commit) + e2e (vitest dialog flow) | `go test ./internal/swap/...` + `pnpm test:run -- swap-meter-dialog` | ❌ Wave 0 |
| DATA-05 | Counter rollover detection | unit (math: `swap.DetectRollover`) + integration (testharness rollover scenario) | `go test ./internal/swap/...` + `go test -tags=integration ./internal/testharness/...` | ❌ Wave 0 |
| DATA-06 | Synthetic-data harness covers 5 scenarios × 2 codecs | integration (10 cases) | `go test -tags=integration ./internal/testharness/... -run TestScenarios` | ❌ Wave 0 |
| DATA-07 | Raw payload + decoded object + canonical fields persisted | integration (read-back assertion) | `go test -tags=integration ./internal/ingest/... -run TestPersistAllLayers` | ❌ Wave 0 |
| DATA-08 | Hybrid wide+JSONB schema | unit (sqlc query for `extra` JSONB read) | `go test ./internal/db/...` | ❌ Wave 0 |
| DATA-09 | Profile editor maps fields without backend deploy | e2e (vitest mapping editor flow) + integration (mapping applied to synthetic uplink) | `pnpm test:run -- mapping-editor` + `go test -tags=integration ./internal/profile/...` | ❌ Wave 0 |
| DATA-10 | At least one fully wired profile (Phase 2 ships 3) | integration (round-trip for axioma_w1 and acrel_adw300; ADL200 inherits) | `go test -tags=integration ./internal/testharness/... -run TestVendorProfiles` | ❌ Wave 0 |
| AUDIT-01 | Audit row for every state change | unit (each handler asserts audit row written) | `go test ./internal/site/... ./internal/meteringpoint/... ./internal/device/... ./internal/profile/... ./internal/swap/...` | ❌ Wave 0 |
| CHIRP-04 | Single-action add device | integration (testcontainers + bufconn CS mock + assert tenant/app/profile/device all created or reused) | `go test -tags=integration ./internal/device/... -run TestAddDeviceAtomic` | ❌ Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/<changed-package>/...` (fast, no testcontainers)
- **Per wave merge:** `go test -tags=integration ./...` + `cd web && pnpm test:run` (testcontainers up, ~30-60s)
- **Phase gate:** Full suite green + the 10 testharness scenarios (DATA-06) green before `/gsd-verify-work`

### Wave 0 Gaps
- [ ] `internal/site/handlers_test.go` — covers SITE-01
- [ ] `internal/meteringpoint/handlers_test.go` — covers MP CRUD
- [ ] `internal/device/handlers_test.go` — covers CHIRP-04 + DEV minimal
- [ ] `internal/device/deveui_test.go` — covers D-11 endianness parser
- [ ] `internal/swap/math_test.go` — covers DATA-05 rollover + DATA-04 offset math
- [ ] `internal/swap/commit_test.go` (integration) — covers DATA-04 atomic swap
- [ ] `internal/ingest/handler_test.go` — covers DATA-01, DATA-02, DATA-03, DATA-07, DATA-08
- [ ] `internal/ingest/normalize_test.go` — covers DATA-09 mapping engine
- [ ] `internal/resolver/cache_test.go` — covers D-25 in-memory cache
- [ ] `internal/resolver/listener_test.go` (integration) — covers D-25 LISTEN/NOTIFY round-trip
- [ ] `internal/profile/editor_test.go` — covers DATA-09 mapping save + codec_js sync
- [ ] `internal/profile/seed_test.go` (integration) — covers D-09 boot-time seed sync to CS (with bufconn CS mock)
- [ ] `internal/audit/log_test.go` — covers AUDIT-01 atomic write
- [ ] `internal/audit/diff_test.go` — covers D-24 changed-fields diff helper
- [ ] `internal/testharness/scenarios_test.go` (integration) — covers DATA-06 (the 10 deterministic cases)
- [ ] `internal/chirpstack/tenant_test.go` (with bufconn mock) — covers Pattern 4 wrappers
- [ ] `internal/chirpstack/application_test.go` (with bufconn mock)
- [ ] `internal/chirpstack/device_profile_test.go` (with bufconn mock)
- [ ] `internal/chirpstack/device_test.go` (with bufconn mock)
- [ ] `internal/chirpstack/bootstrap_test.go` (integration) — covers D-28 idempotent first-boot
- [ ] `web/src/routes/sites/create-site-dialog.test.tsx` — covers SITE-01 dialog UX
- [ ] `web/src/routes/devices/add-device-dialog.test.tsx` — covers CHIRP-04 dialog UX (4 steps)
- [ ] `web/src/routes/devices/deveui-parser.test.tsx` — covers D-11 sticker parser UX
- [ ] `web/src/routes/metering-points/swap-meter-dialog.test.tsx` — covers DATA-04 dialog UX
- [ ] `web/src/routes/profiles/mapping-editor.test.tsx` — covers DATA-09 mapping UX
- [ ] Integration test container module: add `tc-mosquitto` to `internal/testsupport/` (or generic-container fallback) — required for testharness MQTT publish
- [ ] Add `//go:build integration` build tag convention if not yet established in Phase 1 (verify at plan time — Phase 1 testharness pattern)

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|------------------|
| V2 Authentication | yes | Phase 1 already ships AUTH-01..06 (Argon2id + scs/v2 sessions). Phase 2 reuses session middleware to capture `user_id` for audit_log per D-22 |
| V3 Session Management | yes | Phase 1 alexedwards/scs/v2 with Postgres-backed store; cookie HttpOnly + SameSite + Secure (prod) — unchanged in Phase 2 |
| V4 Access Control | yes | Phase 1 `Can(user, action, resource)` middleware + `RequireAction`. Phase 2 ADDS new actions: `site.create/update/archive`, `metering_point.create/update/archive`, `device.add/decommission`, `device_profile.create/update`, `meter.swap`. Viewer role can `read` but cannot perform any write action |
| V5 Input Validation | yes | zod (frontend) + Go structs with validation in handlers. Critical fields to validate: lat/lng range (-90..90, -180..180), DevEUI hex 16 chars, AppKey hex 32 chars, JoinEUI hex 16 chars, JSON Pointer in mapping editor (RFC 6901), codec_js length cap |
| V6 Cryptography | yes (existing) | Argon2id from Phase 1. No new cryptographic primitives in Phase 2. AppKey + JoinEUI stored in ChirpStack (not in Shifter Postgres) — Phase 3 will add the viewer-secret-hide flow per DEV-09 |
| V7 Error Handling | yes | Generic error toasts (no stack leaks); detailed errors logged via slog. CS gRPC errors → ROLLBACK + sanitized "ChirpStack rejected the device: <reason>" message per D-16 |
| V8 Data Protection | yes | `raw_payload` (BYTEA) stored as-is — no encryption at rest in v1 (single-tenant install, customer owns disk). Phase 6 ops hardening covers backup encryption |
| V11 Business Logic | yes | The swap math IS the business logic. Testharness DATA-06 is the verification |
| V12 Files & Resources | partial | Phase 2 has no file uploads (floor-plan upload is Phase 5 SITE-03). Profile editor accepts pasted text (codec_js) — bound by length cap |
| V13 API & Web Service | yes | All Phase 2 handlers behind chi router middleware: auth, RBAC, rate limit (Phase 1), request ID, structured logging |

### Known Threat Patterns for {Go + Postgres + ChirpStack}

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| SQL injection in dynamic queries | Tampering | sqlc parameterized queries (no string concatenation possible by design) |
| ChirpStack credential leak in logs | Information Disclosure | API token never logged (Phase 1 D-22); sanitized error messages from CS |
| Audit log bypass (write to DB without audit row) | Repudiation | Pattern 8: audit writer takes `pgx.Tx` parameter, runs in same txn — by construction impossible to skip |
| MQTT broker credential exposure to browser | Information Disclosure | Phase 1 D-22 + ARCHITECTURE: SSE only (Phase 4) — never MQTT-over-WS to browser |
| Codec_js injection (operator pastes malicious JS) | Tampering | Codec runs in ChirpStack QuickJS sandbox (no access to Shifter process); length cap; admin-only action (Phase 1 RBAC) |
| Resolver cache poisoning via NOTIFY spoofing | Tampering | NOTIFY is database-internal (Phase 1 Postgres only accessible from Shifter network); payload is just a dev_eui hex string — invalid payload = no-op cache delete (no exploit surface) |
| Replay attack on uplinks (duplicate fcnt) | Tampering | ChirpStack v4 already deduplicates by `(dev_eui, fcnt)` server-side; ingest pipeline sets `quality = 'duplicate_fcnt'` if it sees one (defense in depth) |
| Cross-tenant data access (N/A — single-tenant install) | Information Disclosure | PROJECT.md constraint: single-tenant per install. No cross-tenant logic to break |
| Race conditions in swap commit | Tampering | `pgx.Serializable` isolation (Phase 1 install/finish.go pattern); exclusion constraint on binding overlap = belt-and-suspenders |
| Audit log tampering | Repudiation | `audit_log.id` is UUID PK; no UPDATE/DELETE allowed (handlers only INSERT); Phase 6 audit browse UI is read-only |

## Sources

### Primary (HIGH confidence)
- **CONTEXT.md** (.planning/phases/02-domain-model-canonical-schema/02-CONTEXT.md) — 30 locked decisions D-01..D-30 [VERIFIED: read in this session]
- **REQUIREMENTS.md** (.planning/REQUIREMENTS.md) — SITE-01, DATA-01..10, AUDIT-01, CHIRP-04 acceptance criteria [VERIFIED]
- **ROADMAP.md** (.planning/ROADMAP.md) — Phase 2 success criteria + research flags [VERIFIED]
- **CLAUDE.md** (Shifter project root) — locked tech stack: Go 1.24+/PostgreSQL 16/17/TimescaleDB 2.26/sqlc+pgx/v5/chirpstack-api/go v4.17.0+/paho.mqtt.golang/golang-migrate/scs/v2/chi/cobra/viper/slog [VERIFIED]
- **Phase 1 codebase** — `internal/chirpstack/` (Dial, Client, ProbeVersion, MQTTSubscriber), `internal/db/migrations.go` (golang-migrate library mode), `internal/install/finish.go` (atomic Serializable txn), `web/src/components/{stepper,responsive-dialog,status-row}.tsx` [VERIFIED: read source files in this session]
- **Phase 1 Plans 12-13 + Plan 18** — chirpstack package architecture seam (sole importer rule); chi router wiring [VERIFIED via file listing]
- **PITFALLS.md** — §1 metering-point primary entity; §2 offset+rollover math; §3 codec hell; §5 server-side ingest time; §7 v3/v4 mismatch; §8 AS923 sub-plan [VERIFIED: §1-§8 read]
- **STACK.md** — backend (Go) and frontend (React 19 + Vite + shadcn) stack rationale [VERIFIED]
- **ARCHITECTURE.md** — two-path architecture (ingestion vs API), single-Postgres pattern, dev_eui→MP resolver design space [VERIFIED]
- **ChirpStack v4 device.proto** [CITED: github.com/chirpstack/chirpstack/blob/master/api/proto/api/device.proto] — DeviceService methods (Create, Get, Update, Delete, GetKeys, CreateKeys); Device fields (dev_eui, application_id, device_profile_id, name, description, is_disabled, skip_fcnt_check); DeviceKeys fields (dev_eui, nwk_key, app_key)
- **ChirpStack v4 device_profile.proto** [CITED: github.com/chirpstack/chirpstack/blob/master/api/proto/api/device_profile.proto] — DeviceProfileService methods (Create, Get, GetByProfileId, Update, Delete, List); CodecRuntime enum (NONE=0, CAYENNE_LPP=1, JS=2); fields payload_codec_runtime + payload_codec_script
- **ChirpStack v4 tenant.proto** [CITED: github.com/chirpstack/chirpstack/blob/master/api/proto/api/tenant.proto] — TenantService methods (Create, Get, Update, Delete, List, AddUser, GetUser, UpdateUser, DeleteUser, ListUsers); Tenant fields (id, name, description, can_have_gateways, max_gateway_count, max_device_count, private_gateways_up, private_gateways_down, tags)
- **TimescaleDB create_hypertable** [CITED: docs.tigerdata.com/api/latest/hypertable/create_hypertable] — function signature with `chunk_time_interval => INTERVAL '1 day'` syntax (verified for 2.26+); set_chunk_time_interval also documented
- **Axioma W1 Node-RED decoder** [CITED: gist.github.com/Alkarex/4b5d1fef2ff84d483e2793ed009ef607] — short-format vs extended-format byte layouts, status code map, fPort 100, mL units (×1000 for liters)
- **TimescaleDB 2.26.0 release notes** [CITED: github.com/timescale/timescaledb/releases] — March 24, 2026; trigger-based CAGG invalidation removed (10-20% faster ingest)
- **pgx LISTEN/NOTIFY pgxpool issue #1121** [CITED: github.com/jackc/pgx/issues/1121] — confirms LISTEN does not work transparently across pgxpool; one connection must be held open with reconnect loop

### Secondary (MEDIUM confidence)
- **Acrel ADW300 manual** [CITED: acrelenergy.com/uploads/file/adw300-manual.pdf] — 3-phase IoT energy meter; LoRaWAN 868/915 MHz support; measures voltage, current, active/reactive power, harmonics. Specific byte layout to be derived at plan time
- **Acrel ADW300 product page** [CITED: acrelenergy.com/products/adw300] — confirms LoRaWAN, RS485, 4G/WiFi options; Modbus-derived register map
- **Axioma F1 V1.8 Enhanced PDF** [CITED: heitland-gmbh.de/files/media/06-Smart Metering/Lora Payload F1 V01.8 Enhanced.pdf] — 15 volume values per telegram, log time + log volume + delta encoding, fPort 100
- **TigerData wide+JSONB schema design** [CITED: tigerdata.com/learn/designing-your-database-schema-wide-vs-narrow-postgres-tables] — vendor-recommended pattern for IoT multi-vendor schemas (CONTEXT D-08 alignment)
- **Phase 1 D-13/D-16/D-22/D-25** — golang-migrate library mode + plain SQL files + integer prefix; cookie/TLS posture; slog levels (read in this session)

### Tertiary (LOW confidence — flagged in Assumptions Log)
- **Acrel ADL200 codec assumption** — assumed shared with ADW300 per CONTEXT specifics line 186; manual not read in this session (Assumption A2)
- **Acrel ADW300 specific byte layout** — manual referenced; specific scaling factors not yet extracted (Assumption A1)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — every library version verified against `go.mod` / `package.json` / official release notes
- Schema design: HIGH — every column choice anchored in CONTEXT D-01..D-06 + PITFALLS §1/§2/§5
- ChirpStack v4 gRPC service surface: HIGH — verified via `.proto` file fetches in this session
- Architecture patterns (8 patterns): HIGH — every pattern traceable to Phase 1 source code or to a verified external doc
- Pitfalls (12 enumerated): HIGH for #1-#5, #7, #8, #11 (anchored in PITFALLS.md and CONTEXT.md); HIGH for #6, #9, #10 (anchored in CONTEXT specifics + standard PG knowledge); HIGH for #12 (anchored in pgx issue #1121)
- Vendor codec specifics: MEDIUM for Axioma W1 (Node-RED gist read); MEDIUM for Acrel family (manual referenced, byte-level details deferred to plan time)
- Synthetic test harness design: HIGH — testcontainers-go pattern already established in Phase 1; CONTEXT D-27 fully scoped

**Research date:** 2026-05-03
**Valid until:** 2026-06-03 (30 days — TimescaleDB and ChirpStack v4 are stable; Acrel/Axioma vendor docs change rarely; pgx/sqlc/paho all in steady state. Re-verify if more than 30 days elapse before plan execution)
