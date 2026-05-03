# Phase 2: Domain Model & Canonical Schema - Context

**Gathered:** 2026-05-03
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 2 delivers the **vendor-agnostic canonical telemetry schema** plus the **meter-swap differentiator**. End-to-end: a real uplink from a real (or synthetic) LoRaWAN meter lands in a `metering_point_id`-keyed `measurement` hypertable with raw payload + decoded object + canonical normalized fields preserved (DATA-07), and the operator can swap a physical meter via a single dialog without breaking the customer's cumulative chart (DATA-04). Counter rollovers are auto-detected and logged (DATA-05). One synthetic-data test harness exercises every swap/rollover edge case (DATA-06) against three fully-wired vendor profiles spanning water + 1-phase + 3-phase electricity (DATA-10). Single-action "Add device" auto-creates the underlying ChirpStack tenant/application/profile/device behind a stepped dialog (CHIRP-04). Sites get a minimal create-edit-archive surface (SITE-01). Every state-changing action lands in `audit_log` (AUDIT-01).

**In scope:** SITE-01, DATA-01..10, AUDIT-01, CHIRP-04.

**Out of scope (explicit deferrals — not scope creep):**
- Floor-plan upload, multi-floor layouts, drag-drop device placement (SITE-02..06 → Phase 5)
- Devices full list with search/filter/bulk import (DEV-01..09 → Phase 3 — Phase 2 ships only basic device picker + minimal list page)
- Live SSE dashboard, KPI cards, per-meter detail with sparklines (DASH/DETL → Phase 4)
- Continuous aggregates, retention policy, compression, reports, map view (DATA-11..13, REPT, MAP → Phase 5)
- Audit browse UI + CSV export (AUDIT-02, AUDIT-03 → Phase 6); Phase 2 ships only the substrate
- Authentication-event auditing (auth events → Phase 6 with USER-XX retrofit)
- Codec test-runner UI (V2-VEND-02 → Phase 7)
- Pre-seeded vendor catalog beyond the 3 reference profiles (V2-VEND-01 → Phase 7)

</domain>

<decisions>
## Implementation Decisions

### Canonical Schema (3-Layer Model)

- **D-01:** Telemetry uses a 3-layer schema: (1) **canonical wide columns** (typed, indexed, used by dashboard/report/alert); (2) **`device_profile.capabilities[]`** (TEXT array declaring what the device emits — drives adaptive UI render); (3) **`extra` JSONB** (everything the codec emits that isn't promoted to a column). Promotion JSONB→column is `ALTER TABLE` + back-fill (cheap); demotion column→JSONB is hard — start narrow.
- **D-02:** `measurement` Layer 1 columns (frozen for v1, additions are migrations): `metering_point_id UUID NOT NULL`, `time TIMESTAMPTZ NOT NULL` (server-side ingest — DATA-03), `raw_value NUMERIC` (device counter, never overwritten), `cumulative_value NUMERIC` (= `raw_value + binding.reading_offset` — display value), `instant_value NUMERIC` (flow_rate or instant_power), `battery_pct SMALLINT`, `rssi SMALLINT`, `snr REAL`, `temperature_c REAL`, `pressure_kpa REAL`, `leak_detected BOOL`, `tamper_detected BOOL`, `extra JSONB NOT NULL DEFAULT '{}'`, `raw_payload BYTEA NOT NULL`, `decoded_object JSONB NOT NULL`, `quality TEXT NOT NULL DEFAULT 'ok'` (see D-26).
- **D-03:** **Single hypertable** `measurement` for all utility classes (water, electricity, future heat/gas). Sparse-NULL cost negligible at PG storage. Cross-utility "all meters" query of Phase 5 stays a single SELECT, single CAGG chain, single retention policy. Adding `heat`/`gas` is `ALTER TABLE ADD COLUMN`, not new table.
- **D-04:** Capability vocabulary v1 (used by `device_profile.capabilities[]`): `cumulative`, `flow_rate`, `instant_power`, `battery`, `temperature`, `pressure`, `leak_detection`, `tamper_detection`, `multi_phase`, `power_quality`. Phase 5/6/7 alert/report rules key off these tokens.
- **D-05:** **Counter rollover modulus** is per-device-profile config: `device_profile.counter_modulus BIGINT NOT NULL DEFAULT 4294967296` (= 2^32). Profile editor lets admin override per profile (e.g. ADW300 7-digit kWh display → 10^7). Operator never sees this field at add-device time — inherits from profile.
- **D-06:** TimescaleDB hypertable config — `measurement` is a hypertable with `chunk_time_interval = INTERVAL '1 day'` (TigerData IoT recommendation for sub-minute interval ingest). `audit_log` is a regular Postgres table (audit volume is low, ≤ ~hundreds of rows/day per single-tenant install — hypertable adds no value yet). Compression policy + retention deferred to Phase 5/6 alongside CAGGs.

### Vendor Profiles (3 fully-wired in Phase 2 — DATA-10)

- **D-07:** Phase 2 ships **three vendor profiles fully wired end-to-end**, validating the schema across utility classes and codec families:
  - **Axioma Qalcosonic W1** (water, AS923) — capabilities: `cumulative`, `flow_rate`, `battery`, `temperature`, `leak_detection`, `tamper_detection`. Codec from public Axioma "F1 V1.8 Enhanced" PDF + TTN device repository.
  - **Acrel ADL200** (1-phase electricity, AS923) — capabilities: `cumulative`, `instant_power`, `battery`. Codec base shared with ADW300 (Acrel family), separate mapping subset.
  - **Acrel ADW300** (3-phase electricity, AS923) — capabilities: `cumulative`, `instant_power`, `battery`, `temperature`, `multi_phase`, `power_quality`. Codec from Acrel manual; phase A/B/C voltage/current/PF/THD land in `extra` JSONB.
  - "Fully wired" means schema + codec_js + mapping + capabilities locked + DATA-06 synthetic harness exercises every edge case. **Does not require physical hardware** — synthetic uplink generators publish payloads matching each vendor's documented format to the MQTT broker; ChirpStack QuickJS runs the codec the same way regardless of payload origin.
- **D-08:** **Profile editor UI ships fully in Phase 2.** Operator can create new profiles entirely via UI without backend deploy: paste sample decoded JSON → click leaves of JSON tree to auto-fill `json_pointer` → spreadsheet-like table (TanStack Table) of mapping rows with columns `json_pointer | target | scale | data_type` (target dropdown lists canonical columns + `extra.<key>` freeform) → capability checkboxes → paste codec_js (sent to ChirpStack on save). Live preview shows "this sample JSON → these mapping rows → this measurement row." V2-VEND-02 codec test-runner stays Phase 7.
- **D-09:** **Seed delivery:** migration `0010_seed_profiles.up.sql` inserts the 3 device_profile rows + mapping rows + capability arrays + codec_js as SQL string literals. On first `shifter serve` boot, a sync routine pushes each codec_js to ChirpStack via gRPC `DeviceProfileService.Create` (or `Update` if `cs_profile_id` already pinned in the Shifter row — idempotent). Operator can override any seed profile through the editor; the override stays local (unique sync stops touching it).

### Add-Device & Meter-Swap UX

- **D-10:** **Add-device dialog is a 4-step stepped dialog** (reuses Phase 1 D-10 install-wizard pattern):
  1. **Paste DevEUI sticker** → parser shows MSB-first + LSB-first preview side-by-side + vendor OUI heuristic guess → operator clicks correct interpretation.
  2. **Pick device profile** + AppKey + JoinEUI (`Custom` option opens profile editor inline).
  3. **Bind to metering point** (existing dropdown OR `+ Create new metering point` inline).
  4. **Review + submit** → backend transaction: create CS device → insert Shifter device row → open binding (valid_from = now). All four CS calls (tenant/application bootstrap if needed, profile sync if needed, device create) happen in one user action.
- **D-11:** **DevEUI sticker parser** auto-detects endianness with operator confirmation: paste raw → render both MSB-first and LSB-first interpretations + vendor OUI lookup hint → operator clicks correct one. Mitigates PITFALLS §13. AppKey/JoinEUI follow same pattern.
- **D-12:** **Meter swap — outgoing reading R capture:** dialog auto-fills R from the most recent uplink for the outgoing device + shows uplink timestamp ("Outgoing reading: 12,345 m³ from uplink 5 minutes ago") + checkbox "I verified this matches the physical meter." If the outgoing meter is unreachable (no recent uplink), operator manually types R; UI surfaces this as a warning chip on the swap audit row.
- **D-13:** **Reading offset proposal panel:** swap dialog shows a math read-back panel — `Outgoing reading: R | New meter initial: N | Proposed offset: R - N | Display continuity: chart will continue from R, no spike` — plus an editable override field for special cases. "Show me the math" tooltip explains `display = raw + offset`. No mini-chart in Phase 2 (shadcn-charts arrives Phase 4).
- **D-14:** **Swap timing semantics:** `binding_old.valid_to = swap.confirm_time` (operator click, not last_uplink_time). In-flight uplinks arriving after confirm are attributed to old or new binding by `gateway_rx_time` vs the `valid_to` boundary. If `gateway_rx_time < valid_to` → old binding; else → new binding. If no binding covers the timestamp (rare race) → row dropped + audit-log warning event. Aligns with DATA-03 "server-side ingest is authoritative."
- **D-15:** **Decommission is a separate action** from swap. Device detail page exposes `Decommission device` (closes active binding `valid_to = now`, marks device `decommissioned`). Metering point stays active so operator can `Add device` again. This avoids the "swap to nothing" UX of forcing every retire through swap dialog.
- **D-16:** **ChirpStack disconnect handling:** `Add device` review step pre-flight-pings ChirpStack gRPC + MQTT — if either is unreachable, submit is disabled with a status row "ChirpStack unreachable. Settings → Test connection." On submit: `BEGIN tx → CS gRPC create → Shifter row insert → COMMIT`. Any CS failure → ROLLBACK → error toast "ChirpStack rejected the device: <reason>." No partial state, no async queue, no Saga in Phase 2.

### Site & Metering Point Surface

- **D-17:** **Create Site dialog fields** — `name*`, `lat`, `lng`*, `timezone` (default = install timezone, override-able dropdown), `address` (free text 1–2 lines, used as Phase 5 PDF report header), `description` (optional). Schema also has `parent_id UUID NULL REFERENCES site(id)` and `site_type TEXT NULL` (no UI in Phase 2 — Phase 5 enables tree picker for floor-plan multi-floor layouts without requiring migration).
- **D-18:** **Site lat/lng UI in Phase 2** = two number input boxes + helper text "paste from Google Maps" + range validation. A `Pick on map` button is rendered but disabled with tooltip "available in v5" (Leaflet not installed yet — installs in Phase 5 to keep Phase 2 frontend bundle lean).
- **D-19:** **Create Metering Point dialog fields** — `name*` (e.g. "Building A — main water"), `site_id*` (dropdown), `utility_class*` (radio: `water` | `electricity`), `location_description` (free text — "main inlet, basement east riser"). `expected_interval_s` and `unit` are inherited from the bound device profile (not asked at MP creation — picked up when first device is bound). `utility_class` is on the metering point so DASH-01 (Phase 4) can render the right view even when the MP has no device bound yet.
- **D-20:** **Metering point lifecycle = soft-delete only.** Add `archived_at TIMESTAMPTZ NULL` column. UI exposes `Archive` (hides from default list, dashboard stops rendering, but historical query still resolves). An "Archived" tab restores via `archived_at = NULL`. No hard-delete — preserves audit trail (matches USER-01 "no hard-delete" pattern locked for Phase 6).

### Audit Log

- **D-21:** **Audit scope in Phase 2 = CRUD on domain entities (Site, Metering Point, Device, Device Profile, Binding) + meter swap action.** Authentication-event auditing (login, logout, password change, failed_login) deferred to Phase 6 alongside USER mgmt + the audit retrofit of Phase 1 handlers — keeps Phase 2 surface bounded.
- **D-22:** **Single `audit_log` table** with columns: `id UUID PK`, `time TIMESTAMPTZ DEFAULT now()`, `user_id UUID REFERENCES "user"(id)`, `action TEXT NOT NULL` (`create`|`update`|`archive`|`swap`|`decommission`|`profile_create`|`profile_update`|`binding_open`|`binding_close`), `entity_type TEXT NOT NULL` (`site`|`metering_point`|`device`|`device_profile`|`binding`), `entity_id UUID NOT NULL`, `before JSONB`, `after JSONB`, `notes TEXT`, `request_id TEXT`. Phase 6 audit browse + CSV export queries this single table with filters.
- **D-23:** **Audit write is synchronous in the same transaction as the write.** `BEGIN → INSERT INTO target_table → INSERT INTO audit_log → COMMIT`. No async/River-job indirection — meter swap audit MUST be atomic with the swap. River is for retries on backend tasks, not audit logging.
- **D-24:** **`before`/`after` capture = changed fields only.** UPDATE → `before` and `after` contain only the JSONB sub-object of fields that changed (diff-like). CREATE → `before = NULL`, `after` = full insert. ARCHIVE → `before = {archived_at: null}`, `after = {archived_at: <ts>}`. SWAP → `before = {binding: {...}, reading_offset: ...}`, `after = {binding: {...}, reading_offset: ...}` (binding pair). Reduces storage and makes audit browse easy to highlight.

### MQTT → Ingest Pipeline

- **D-25:** **dev_eui → metering_point_id resolver** is an in-memory map cache + `LISTEN/NOTIFY` invalidation. Resolver lazy-loads on first uplink for a `dev_eui`. Swap commit publishes `NOTIFY binding_changed, '<dev_eui>'`; resolver listens and invalidates the entry. No TTL — eventless invalidation prevents the swap-then-uplink-attributed-to-old-binding race documented in PITFALLS §2. Phase 4 SSE will reuse the same `LISTEN/NOTIFY` channel for live KPI updates.
- **D-26:** **Ingest quality flag — persist all uplinks, never silent-drop.** `measurement.quality TEXT NOT NULL DEFAULT 'ok'` with values `ok`, `decode_fail`, `missing_canonical`, `out_of_range`, `duplicate_fcnt`. `raw_payload` and `decoded_object` are always preserved (DATA-07). Dashboard surfaces a badge "X uplinks flagged" on devices with non-`ok` rows so operators see codec drift instead of silent data loss (PITFALLS §3).

### Synthetic Test Harness (DATA-06)

- **D-27:** **Test harness is a shared library + dual entry point.** `internal/testharness/` package owns scenario generation (clean_swap, swap_inflight, rollover, swap_rollover, overlapping_uplinks_during_swap × 2 codec families = 10 deterministic scenarios). Go integration tests drive it via testcontainers-go (Mosquitto + Postgres+TimescaleDB) — runs in CI. CLI wrapper `shifter test-harness <scenario>` publishes the same scenarios to the broker of a deployed install — operator can validate post-install without dev environment. The library is reused in Phase 7 by the codec test-runner UI.

### ChirpStack Abstraction

- **D-28:** **Single global ChirpStack tenant + 1 application + auto-bootstrap.** Shifter creates tenant `shifter-default` and application `shifter` on first use (or reuses if they already exist — pinned by CS UUID stored in Shifter `chirpstack_connection` row, so subsequent boots are no-ops). External-CS mode: if the API token's tenant context already names a tenant, Shifter uses it (no duplicate). Each Shifter `device_profile` maps 1:1 to one CS profile via `cs_profile_id`. Single MQTT topic filter `application/<app_id>/device/+/event/up`. Operator never sees CS hierarchy — Shifter UI speaks site/metering point/device only.

### Devices & Bulk

- **D-29:** **Phase 2 devices surface = picker component + minimal list page.** The device picker (used by swap dialog and MP detail "Add device") is a searchable dropdown by name/dev_eui. The minimal devices list page is a TanStack Table — columns: name, dev_eui, profile, current MP, last_seen, battery — without advanced filtering or bulk actions. Phase 3 (DEV-01..09) extends with full search/filter, soft-delete, bulk CSV import, ABP/OTAA UX, viewer-secret-hide.
- **D-30:** **Bulk operations not in Phase 2.** Sites and metering points are single-create through dialogs. Phase 3 (DEV-06..08) ships the CSV bulk-import flow that handles sites + MPs + devices in a unified two-phase pattern (PITFALLS §12 dry-run + commit). Avoids building two separate CSV pipelines.

### Claude's Discretion

- Exact `internal/` directory layout for new packages (e.g. `internal/ingest`, `internal/swap`, `internal/profile`, `internal/audit`, `internal/testharness`) — planner picks idiomatic Go boundaries
- Migration file naming for Phase 2 tables (numeric prefix continues from 0006 onwards — site, metering_point, device_profile, device, device_profile_mapping, binding, measurement, audit_log, seed_profiles; one conceptual change per migration)
- Foreign key + unique constraint placement (e.g. UNIQUE(metering_point_id, valid_from) on binding to prevent overlap)
- Exact JSON shape for `audit_log.before`/`after` diffs (preserve null vs missing-key semantics per planner's judgement)
- TanStack Table column visibility defaults, sort order, page size for the minimal devices list page
- Mapping editor's exact UX for clicking JSON tree leaves vs typing json_pointer manually
- Profile editor's `data_type` set (numeric/int/bool/text — extend if planner finds another needed)
- Exact wording on the `quality` badge tooltip surfaces ("4 uplinks flagged: 3 missing battery, 1 decode_fail")
- Synthetic-data CLI flag/subcommand shape (`shifter test-harness clean_swap --vendor axioma`)
- River queue config — Phase 2 doesn't ship any River jobs (audit is sync, CS sync is sync, ingest is sync); River install can defer to Phase 5 retention/CAGG refresh jobs

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Project & phase
- `.planning/PROJECT.md` — Vision, constraints (single-tenant, blue/navy + shadcn, modal-first, English-only), Key Decisions table (metering-point + offset, vendor-agnostic measurement model)
- `.planning/REQUIREMENTS.md` §SITE-01, DATA-01..10, AUDIT-01, CHIRP-04 — Phase 2 acceptance criteria
- `.planning/ROADMAP.md` Phase 2 — 5 success criteria + Research Flags (resolver cache TTL, offset math under concurrency, harness design)

### Phase 1 carry-over (reusable patterns + locked decisions)
- `.planning/phases/01-foundation/01-CONTEXT.md` D-10 (stepped-dialog pattern), D-13 (`migrate up` on serve startup), D-16/D-17 (golang-migrate library mode + plain SQL files + integer prefix), D-18/D-19 (`/health` split — Phase 2 doesn't add new endpoints), D-22 (cookie/TLS posture), D-25 (slog levels)
- `.planning/phases/01-foundation/01-RESEARCH.md` §Pattern 8 (SPA embed) + §Pattern 10 (paho.mqtt.golang OnConnect re-subscribe) — Phase 2 ingest pipeline plugs into existing MQTT subscriber

### Research synthesis
- `.planning/research/SUMMARY.md` — Cross-stream synthesis; vendor-agnostic measurement model; metering-point + offset differentiator; two-channel ChirpStack
- `.planning/research/STACK.md` — Library versions and rationale (TimescaleDB 2.26 hypertable + chunk_time_interval, sqlc + pgx/v5 query patterns, paho.mqtt.golang QoS/cleansession, alexedwards/scs/v2 user_id source for audit_log)
- `.planning/research/ARCHITECTURE.md` — Two-path architecture (ingestion vs API), single-Postgres pattern (separate databases inside one instance for bundled mode), `dev_eui→metering_point_id` resolver design space
- `.planning/research/FEATURES.md` — Vendor profile catalog discussion (Phase 2 ships 3, Phase 7 ships catalog)
- `.planning/research/PITFALLS.md` §1 (metering-point primary entity), §2 (offset math + rollover), §3 (codec hell — decode in CS, never in Shifter; reject + flag, never silent drop), §5 (server-side ingest time as authoritative), §12 (bulk-import two-phase — informs Phase 3 not Phase 2), §13 (DevEUI endianness), §14 (`Can(user, action, resource)` API — already locked in Phase 1)

### External / vendor docs
- ChirpStack v4 gRPC API — `https://www.chirpstack.io/docs/chirpstack/api/grpc.html` (TenantService, ApplicationService, DeviceProfileService, DeviceService — Phase 2 first uses these beyond version probe)
- ChirpStack MQTT integration — `https://www.chirpstack.io/docs/chirpstack/integrations/mqtt.html` (decoded `object` is a struct in v4, not a JSON string — informs ingest)
- ChirpStack QuickJS payload codec — `https://www.chirpstack.io/docs/chirpstack/use/device-profiles.html#payload-codec` (codec runs in CS sandbox, never in Shifter)
- TimescaleDB hypertable + chunk_time_interval — `https://docs.timescale.com/use-timescale/latest/hypertables/about-hypertables/` and `https://docs.timescale.com/api/latest/hypertable/create_hypertable/`
- TigerData IoT schema design — `https://www.tigerdata.com/learn/designing-your-database-schema-wide-vs-narrow-postgres-tables` (wide + JSONB hybrid pattern)
- Postgres `LISTEN/NOTIFY` — `https://www.postgresql.org/docs/current/sql-listen.html` (resolver invalidation channel; Phase 4 SSE will reuse)

### Vendor profile sources (for the 3 wired profiles)
- Axioma Qalcosonic W1 LoRaWAN payload F1 V1.8 (PDF — distributed via Axioma) + Node-RED decoder gist `https://gist.github.com/Alkarex/4b5d1fef2ff84d483e2793ed009ef607` + TTN device repository `https://www.thethingsnetwork.org/device-repository/devices/axioma/`
- Acrel ADW300 manual — `https://www.acrelenergy.com/uploads/file/adw300-manual.pdf` (3-phase electricity, AS923 supported, payload format documented)
- Acrel ADL200/ADL400 product family — `https://jsacrel.en.made-in-china.com/product/UQqYlzPEFpkr/...` (1-phase variant; codec base shared with ADW300)
- Optional reference for codec quality benchmark — Milesight SensorDecoders `https://github.com/Milesight-IoT/SensorDecoders` (best-in-class codec repo; not used for Phase 2 profiles but referenced as the standard)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets (from Phase 1)

- `internal/chirpstack/client.go` — `Dial` + `Client` + `authInterceptor` already exist. **Phase 2 extends this package** (per its own `doc.go` rule that it is the SOLE importer of `chirpstack-api/go/v4`) with `TenantService`, `ApplicationService`, `DeviceProfileService`, `DeviceService` wrappers. No other Shifter package imports `chirpstack-api`.
- `internal/chirpstack/mqtt.go` — `MQTTSubscriber` with pluggable `UplinkHandler` is already in place. **Phase 2 wires the ingest pipeline as a non-nil `UplinkHandler`** that routes uplinks through the resolver (D-25), profile mapping (D-08 `device_profile_mapping`), normalize, and persist. The default stdout handler in `NewMQTTSubscriber` becomes a fallback only.
- `internal/chirpstack/version.go` + `ProbeVersion` — Reused unchanged for Add-device pre-flight (D-16).
- `internal/db/migrations.go` + `internal/db/migrations/` — **golang-migrate library mode** (D-13/D-16 of Phase 1) — Phase 2 adds 9–10 migrations: `0007_site`, `0008_metering_point`, `0009_device_profile`, `0010_seed_profiles`, `0011_device`, `0012_device_profile_mapping`, `0013_binding`, `0014_measurement` (hypertable), `0015_audit_log`. Numeric prefix continues unbroken.
- `internal/db/queries/*.sql` + `sqlc.yaml` — **sqlc query pattern** is already established. Phase 2 adds queries for sites, metering points, devices, profiles, mappings, bindings, measurements, audit log. Same `:one`, `:many`, `:exec` annotations.
- `internal/db/pool.go` — `pgxpool` setup already in place; reused unchanged.
- `internal/auth/can.go` (Phase 1 D-10/PITFALLS §14) — `Can(user, action, resource)` API. **Phase 2 adds new actions:** `site.create/update/archive`, `metering_point.create/update/archive`, `device.add/decommission`, `device_profile.create/update`, `meter.swap`. Hooked from each handler before the DB write + audit insert.
- `internal/install/finish.go` — Pattern for atomic Serializable transaction commit (used by wizard) — informs the swap-commit transaction shape.
- `web/` (shadcn `new-york` + slate + custom navy OKLCH, ResponsiveDialog, Stepper) — **All add-device, swap, profile-editor, site, MP dialogs reuse Stepper + ResponsiveDialog**. No new shadcn primitives needed except `Table` (TanStack Table integration) for the minimal devices list page (D-29) and the mapping editor (D-08).
- `web/src/api/apiFetch.ts` (Phase 1) — Frontend API helper; Phase 2 endpoints follow same idiom.
- TanStack Query pre-installed (Phase 1 STACK.md) — Phase 2 mutations + cache invalidation (e.g. swap → invalidate metering-point detail + dashboard) ride this.

### Established Patterns Phase 2 Inherits

- **Layered Go module** — `cmd/shifter` → `internal/<domain>` → `internal/db/sqlc` (generated) → `internal/db/migrations` (plain SQL)
- **Stepped-dialog pattern** — install wizard (Phase 1 D-10) is the template for add-device (D-10) and swap (D-12/D-13). Each step writes a draft to backend; finish commits all in one transaction.
- **Status-row pattern** — Test Connection (Phase 1) → reused for ChirpStack pre-flight in add-device review step (D-16) and for the per-device "X uplinks flagged" badge (D-26).
- **Modal-first CRUD** — Sites, metering points, profiles, devices, swaps all in dialogs (UX-01).
- **English-only canonical verb table** — Add, Edit, Archive, Restore, Swap, Decommission (extending Phase 1's Sign in / Set password / Save and test / Add / Edit / Delete / Revoke / Disable).

### Integration Points

- **ChirpStack v4 gRPC** — Phase 2 first calls `TenantService` (bootstrap), `ApplicationService` (bootstrap), `DeviceProfileService` (seed sync + per-profile create/update), `DeviceService` (add/decommission). All from `internal/chirpstack/`.
- **MQTT broker** — `internal/chirpstack/mqtt.go` already subscribes to `application/+/device/+/event/up`. Phase 2's `UplinkHandler` consumes events.
- **Postgres + TimescaleDB** — Phase 2 turns on hypertable creation (`SELECT create_hypertable('measurement', 'time', chunk_time_interval => INTERVAL '1 day')` in migration 0014). Reuses pgxpool.
- **Frontend SPA** — Existing router shell (Phase 1 RootLayout + AccountMenu) gets new routes: `/sites`, `/sites/:id`, `/metering-points/:id`, `/devices`, `/profiles`, `/profiles/:id`. All composed with existing `ResponsiveDialog` + `Stepper`.
- **Compose flavors** — No new services added (Mosquitto + ChirpStack + Postgres+TS + Caddy + Shifter unchanged from Phase 1).

</code_context>

<specifics>
## Specific Ideas

- The product still must feel like Shifter, never like a re-skinned ChirpStack. **Operator never sees the words "tenant" or "application"** in any Phase 2 UI — the abstraction is total (D-28).
- "Auto-fill from last uplink + verify checkbox" for the swap reading R is the right friction balance: most swaps the meter is reachable until the moment of physical replacement, so the auto-fill is correct. The checkbox protects against the rare case where the meter was already pulled before the operator opened the dialog.
- Mapping editor: the "paste sample JSON → click leaf → auto-fill json_pointer" flow is the single most-impactful operator-UX win. It's the difference between "I have to read JSON Pointer RFC 6901" and "I click the field I want."
- Acrel ADL200 + ADW300 share Acrel's payload format family — **one codec_js implementation, two device_profile rows, two mapping subsets**. Mapping for ADL200 enables `cumulative` + `instant_power` + `battery`; for ADW300 enables those plus `multi_phase` + `power_quality` + sets `extra` JSONB pointers for L2/L3 fields.
- Synthetic test harness scenarios are concrete and named: `clean_swap`, `swap_with_inflight_uplink`, `rollover`, `swap_then_rollover`, `overlapping_uplinks_during_swap`. Each runs against `axioma_w1` and `acrel_adw300` codec families = 10 deterministic test cases. ADL200 is structurally identical to ADW300 in the swap/rollover dimensions, so it inherits coverage.
- Reading offset history (audit trail) is captured naturally by AUDIT-01 + D-22 + D-24 — every swap and every operator override of the proposed offset writes a `before`/`after` row keyed to the binding entity. No separate "offset_change_log" table.
- The `Pick on map` button in Site dialog being rendered-but-disabled in Phase 2 (D-18) is intentional UX continuity: when Phase 5 enables it, no UI surface change for the operator.
- The audit_log being a regular Postgres table (D-06) is conscious — re-evaluate at Phase 6 when AUDIT-02 (browse) and AUDIT-03 (CSV export) ship and we know real volume per install.

</specifics>

<deferred>
## Deferred Ideas

- **Auth-event auditing** (login, logout, password change, failed_login) — Phase 6 alongside USER-XX. Phase 2 establishes the substrate; Phase 6 retrofits Phase 1 handlers.
- **Audit browse + filter UI + CSV export** (AUDIT-02, AUDIT-03) — Phase 6.
- **Continuous aggregates + retention + compression policy** (DATA-11..13) — Phase 5.
- **CAGG-aware retention boundary tests** (PITFALLS §6) — Phase 5 install-validation.
- **Pixel-accurate floor-plan device placement** — Phase 5 (SITE-04 with normalized fractional coordinates).
- **Map view + Leaflet** — Phase 5 (MAP-01..04). The `Pick on map` button in Site dialog stays disabled until then.
- **Bulk CSV import** (sites + MPs + devices in one unified flow) — Phase 3 (DEV-06..08) using PITFALLS §12 two-phase pattern.
- **Devices full list with search/filter/sort** (DEV-01) — Phase 3.
- **Operator-paste codec test-runner UI** (V2-VEND-02) — Phase 7.
- **Pre-seeded vendor catalog beyond the 3 reference profiles** (V2-VEND-01) — Phase 7. The 3 Phase-2 profiles are real, but the broader Kamstrup/Diehl/Itron/Sagemcom/Schneider IEM3xxx catalog is Phase 7.
- **Mini-chart preview in swap dialog** showing before/after continuity — defer until shadcn-charts ships in Phase 4. Phase 2 ships text/math read-back only.
- **Saga-style "pending CS sync" device state** — D-16 picked transactional rollback; if a customer hits real CS flakiness, revisit Saga in Phase 6 ops hardening.
- **Per-site application in ChirpStack** — D-28 picked single global application; revisit only if a multi-site customer hits a clear scaling or isolation requirement (none expected for single-tenant single-install).
- **River background job queue** — not needed in Phase 2 (audit sync, CS sync, ingest sync). First River usage is Phase 5 CAGG refresh + Phase 6 alert evaluation.
- **External_code (operator-defined site/MP code)** — not in Phase 2. ALTER TABLE ADD COLUMN is non-destructive — add when first customer asks.
- **Tariff / cost / billing-cycle fields on metering point** — V2-BILL-01.
- **Profile import/export between installs** — useful but not in Phase 2 scope. Phase 7 codec catalog likely subsumes this.
- **Compression of `raw_payload`/`decoded_object`** — TimescaleDB column compression considered in Phase 5/6 retention work.
- **Time-zone handling in measurement queries** — measurement.time is `TIMESTAMPTZ` so the storage is unambiguous. Per-site/per-install timezone display rules belong with Phase 4 dashboard + Phase 5 reports.

</deferred>

---

*Phase: 02-domain-model-canonical-schema*
*Context gathered: 2026-05-03*
