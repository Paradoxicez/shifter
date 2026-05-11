# Phase 3: Provisioning (Gateways, Devices, Bulk Import) - Context

**Gathered:** 2026-05-11
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 3 delivers operator-facing provisioning of the **physical LoRaWAN fleet** through Shifter alone:

- **Gateways** — full CRUD (create/edit/decommission/restore), regulator-aware region picker, last-24h RX/TX stats with sparkline. Sites/floor-plan pin-drop UI deferred to Phase 5; numeric lat/lng only here.
- **Devices** — extends Phase 2 minimal `/devices` list with full filter/sort/pagination, server-side viewer secret hiding (new admin-only reveal endpoint), and OTAA **and** ABP activation flows in the add-device dialog.
- **Bulk import** — XLSX/CSV file → two-phase dry-run/commit → per-row outcomes. Replaces "click N times" device onboarding for fleets.

Out of scope (these are own phases): site/floor-plan map UI, telemetry ingestion, reports, alerting, full audit query UI.

</domain>

<decisions>
## Implementation Decisions

### Gateway surface scope & RX/TX stats
- **D-01:** Gateway map pin deferred to Phase 5. Phase 3 ships lat/lng as **numeric inputs + 'Pick on map' button rendered-but-disabled** with tooltip "available in v5", mirroring D-18 from Phase 2 Site dialog. GW-04 lands in Phase 5 alongside MAP-01..04; REQUIREMENTS.md traceability updated at Phase 3 close.
- **D-02:** Gateway list shows **last-24h RX count + TX count + uplink success % + hourly sparkline** column. Driven by ChirpStack `GetGatewayStats(aggregation=HOUR, start=now-24h, end=now)`. Backend caches results **1 minute** to support 50+ gateway fleets without per-row gRPC fan-out on every list refresh.
- **D-03:** Gateway create/edit dialog shows a regulator-aware LoRaWAN **region dropdown pre-selected to the Phase 1 install default (INST-04)**. Admin may override on a per-gateway basis to support multi-region installs. Satisfies GW-02 "regulator-aware region picker".

### Bulk import — schema & file formats
- **D-04:** Import schema —
  - **Required:** `dev_eui`, `name`, `device_profile` (name or id), `site_id` (or `site_name` resolved at validation).
  - **Optional:** `app_eui` / `join_eui`, `app_key` (OTAA); `dev_addr`, `nwk_s_key`, `app_s_key`, `fcnt_up`, `fcnt_down` (ABP); `description`, `tags` (comma-separated or JSON).
- **D-04a:** Supported file formats — **XLSX (primary, Thai-safe)** and **CSV (UTF-8 with BOM)**. Backend uses `excelize/v2` for XLSX parse (already in deps for report generation per CLAUDE.md) and `encoding/csv` stdlib for CSV. The import dialog offers a **"Download template" XLSX** with pre-formatted header row + tooltip cells.
- **D-05:** EUI/Key format — **big-endian hex only** (matches ChirpStack v4 wire format). Backend auto-strips `0x` prefix and `:` / `-` separators, normalises to lowercase 16-hex (EUI) / 32-hex (keys). Reject rows with malformed values.
- **D-06:** Idempotency — `dev_eui` is the natural key. Re-uploading a file with a pre-existing `dev_eui` produces an **`already_exists` row outcome** (not error, not silent update). Re-runs are safe.
- **D-07:** Partial commit — valid rows commit; invalid rows reported with per-row reason. Admin can **download `errors.xlsx`** (same shape as input + reason column) and re-upload only the fixed rows. Matches PITFALLS §12 two-phase pattern.

### Two-phase dry-run / commit UX
- **D-08:** Dry-run = **validate-only, no ChirpStack calls**. Checks EUI format, intra-file duplicate `dev_eui`, pre-existing `dev_eui` (already_exists outcome), site existence, device_profile existence, required fields, key length per activation mode. Fast and deterministic.
- **D-09:** Preview = **summary banner + per-row TanStack Table**. Banner shows "142 will be created, 8 skipped, 3 errors". Table supports filter/sort/expandable rows for the per-row reason. Admin sees what will happen before commit.
- **D-10:** Commit confirmation = **button-only** ("Import N devices" / Cancel). No type-to-confirm. Mirrors Phase 2 D-15 decommission-confirm pattern — friction proportional to risk, and dry-run already showed full impact.
- **D-11:** `import_job` is **server-side**, status enum `preview` → `committed` / `expired`. **TTL 1 hour** between dry-run and commit (resumable via `job_id`). Tab-close/network glitch does not lose preview; expired job → re-upload.

### Devices list filters & pagination
- **D-12:** Filters — **site (multi-select)** + **activation status** (active/inactive/never_joined) + **last_seen window** (24h/7d/30d/all) + **text search** on `name` & `dev_eui`. Server-side, sqlc-generated.
- **D-13:** **Offset-based pagination**, page-size selector 25/50/100 (default 50). Cursor/keyset is overkill at Shifter's scale (≤100K devices/install).
- **D-14:** **Sortable columns:** `name`, `dev_eui`, `site`, `last_seen`, `created_at`. Server-side sort. Indexes on `(site_id, last_seen)` and `(created_at)`.
- **D-15:** Filter state persisted in **URL query params** via TanStack Router search params. Shareable links, back/forward-aware, deep-linkable from future dashboard widgets.
- **D-16:** CSV/XLSX **export = current filtered view**. Columns are round-trip-compatible with the import schema (D-04) + appended `last_seen`, `created_at` audit columns. Export format mirrors imported format choice (XLSX default).
- **D-17:** **Bulk decommission only** — no bulk edit. Select rows → "Decommission N devices" button → confirm dialog (no type-to-confirm). Bulk reassign/edit deferred indefinitely.
- **D-18:** **Fixed column set** for the table (name, dev_eui, site, last_seen, activation, profile). No per-user toggle/visibility — single-tenant operator tool, complexity not justified.

### OTAA / ABP activation in add-device dialog
- **D-19:** Activation mode is a **dedicated step** in the stepped add-device dialog (5 steps total: identity → site → activation (OTAA/ABP radio) → mode-specific keys → review). Extends Phase 2's 4-step OTAA-only dialog. Preserves D-10 stepped-dialog + draft-per-step + finish-as-one-tx pattern.
- **D-20:** ABP fields — required `dev_addr` (8 hex), `nwk_s_key` (32 hex), `app_s_key` (32 hex). **Optional `fcnt_up`, `fcnt_down`** (default 0). FCnt carry-over matters for meter-swap workflows later (Phase 7+); exposing now avoids a breaking field addition.
- **D-21:** After successful create, dialog **success state shows all keys + "Copy keys" button** until admin clicks Done. Rendered once at commit time; not re-fetched on re-open. Reveal-after-creation goes through D-22 path.
- **D-22:** **ChirpStack is the sole key store.** Shifter never columns secrets (extends DEV-09 "no Shifter column" pattern). Reveal = on-demand gRPC `GetDeviceKeys` (OTAA) / `GetDeviceActivation` (ABP), gated by `Can('device.reveal_secrets')`.
- **D-23:** **MAC version is not asked in the dialog** — inherited from the selected `device_profile` (ChirpStack source-of-truth). Avoids profile/activation MAC-version drift.
- **D-24:** OTAA join-identifier field labelled **`Join EUI (AppEUI for v1.0)`** with tooltip explaining the naming history. Backend uses `join_eui` per ChirpStack v4 API; works for both 1.0.x and 1.1.x devices.
- **D-25:** Custom external **join-server deferred to backlog**. Phase 3 uses ChirpStack's internal join-server only — no v1 customer has requested external join-server.

### Server-side viewer hiding for new secret fields
- **D-26:** Reveal authorization via new action `Can('device.reveal_secrets')` added to `internal/auth/can.go`. **Admin role only**; viewer → 403. Per PITFALLS §14 (Can(action, resource) granularity).
- **D-27:** **Structural separation**, not application-layer redaction. List/detail SQL never `SELECT`s secret columns (none exist in Postgres per D-22). Reveal is a **separate endpoint** `POST /api/devices/:eui/keys`, auth-gated, that calls ChirpStack `GetDeviceKeys`/`GetDeviceActivation` on demand. Bug-resistant — no code path can accidentally leak keys to a viewer.
- **D-28:** Each reveal writes an audit row: `action='device.reveal_secrets'`, `resource_id=dev_eui`, **no diff payload** (the secret material itself is never logged). Honours Phase 2 D-22..24 audit_log shape.

### Gateway add-flow shape & decommission
- **D-29:** Add-gateway dialog = **single ResponsiveDialog** (no stepper). Fields: `gateway_id`, `name`, `description`, `region` (defaulting to install default per D-03), location lat/lng (numeric, per D-01), optional `tags`. Atomic CS+PG transaction on submit (D-16 pattern).
- **D-30:** **Decommission semantics:** soft-delete in Postgres (`archived_at`, `archived_reason='operator decommission'`) **+** `DeleteGateway` in ChirpStack so the gateway stops accepting uplinks. Atomic with best-effort ChirpStack rollback if Postgres commit fails (D-16 pattern).
- **D-31:** Decommission is **allowed at any time**, but if the gateway received uplinks from ≥1 device in the last 24h the confirm dialog warns: *"This gateway received uplinks from X devices in the last 24h. They will route via other gateways. Proceed?"* Backed by GetGatewayStats; not a hard block.
- **D-32:** Gateway list **hides archived rows by default**. A "Show archived" toggle reveals faded rows with an admin-only **Restore** action. Honours USER-01 never-hard-delete.

### Audit-log volume & request_id grouping for bulk import
- **D-33:** Bulk-import audit shape — **1 row per device** (`action='device.create'`, `source='bulk_import'`) **+ 1 envelope row** (`action='device.bulk_import'`, `metadata={total, created, skipped, failed, job_id}`). Enables both per-device traceability ("when was this dev_eui created?") and job-level summary queries.
- **D-34:** **`import_job.job_id` (UUID) = audit `request_id`** for every row created in that job. Single `WHERE request_id = $1` query yields all rows for one import. (Web HTTP `X-Request-Id` would collide because the commit endpoint creates N rows in one HTTP call.) Note: the table is named `import_job` (not `csv_import_job`) since D-04a generalised the input format to XLSX + CSV.
- **D-35:** `import_job` and `import_job_row` retention = **90 days**. Archive/purge policy itself deferred to Phase 9. The `audit_log` rows (per-device + envelope) are **kept indefinitely** per Phase 2 D-22..24.
- **D-36:** **No general audit query UI in Phase 3.** Phase 9 (Audit & Settings) owns `/admin/audit`. Phase 3 ships **`/admin/imports/:job_id` detail page** scoped to one import job: shows the per-row TanStack Table from D-09 with final outcomes + download-errors button. Admin can review a recent import without a general audit UI.

### Claude's Discretion
- Exact `import-template.xlsx` layout (column widths, header styling, tooltip cell text)
- TanStack Table column widths, default sort direction, sparkline visual style
- ResponsiveDialog success-state copy and "Copy keys" toast wording
- Error-row CSV/XLSX filename convention (e.g. `device-import-errors-${job_id}.xlsx`)
- Reveal-endpoint rate-limiting strategy (if any)
- Background job vs sync request for commit-phase processing (1k rows is fast; >10k may need backgrounding)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase scope & requirements
- `.planning/PROJECT.md` — Single-tenant per install; ChirpStack-as-backend; dialogs-only CRUD
- `.planning/REQUIREMENTS.md` — **GW-01..04, DEV-01..09, CHIRP-05, CHIRP-06, UX-03, INST-04** map directly to Phase 3 work
- `.planning/ROADMAP.md` §Phase 3 — Goal statement & success criteria

### Prior phase context (locked decisions Phase 3 inherits)
- `.planning/phases/01-foundation/01-CONTEXT.md` — **D-10** install wizard pattern, **INST-04** regulator-aware region picker
- `.planning/phases/02-domain-model-canonical-schema/02-CONTEXT.md` — **D-10** stepped-dialog draft-per-step finish-as-one-tx, **D-15** decommission UX (button-only confirm), **D-16** atomic CS+PG transaction + best-effort CS rollback, **D-22..D-24** audit_log shape (actor/action/resource/diff/request_id), **D-28** single-tenant/app + UX-03 enforcement, **D-30** bulk import deferred to Phase 3 (this phase)

### Research artifacts
- `.planning/research/SUMMARY.md` §Phase 3 — Gateway list, RX/TX strategy, bulk CSV import patterns
- `.planning/research/PITFALLS.md` §12 — Two-phase bulk import (dry-run/commit, partial-commit, idempotency)
- `.planning/research/PITFALLS.md` §13 — DevEUI endianness (big-endian on ChirpStack wire)
- `.planning/research/PITFALLS.md` §14 — `Can(action, resource)` granularity; Phase 3 adds `gateway.create`, `gateway.update`, `gateway.archive`, `gateway.restore`, `device.bulk_import`, `device.reveal_secrets`

### External / vendor
- ChirpStack v4 gRPC API — https://www.chirpstack.io/docs/chirpstack/api/grpc.html (`GatewayService`, `DeviceService` `GetDeviceKeys` / `GetDeviceActivation`)
- LoRaWAN regional parameters (AS923 sub-plans for Thailand) — https://www.thethingsnetwork.org/docs/lorawan/regional-parameters/

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/chirpstack/client.go` — sole importer of `chirpstack-api/go/v4` per `doc.go`; extend with **`GatewayService` wrapper**
- `internal/chirpstack/tenant.go`, `application.go`, `device.go` — mirror these for a new `internal/chirpstack/gateway.go`
- `internal/db/migrations/` — next migration index **0018+** for `gateway`, `import_job`, `import_job_row` tables (+ new `audit_log.source` column or use existing `metadata` JSONB)
- `internal/auth/can.go` — `Can(action, resource)` — **add** `gateway.create`, `gateway.update`, `gateway.archive`, `gateway.restore`, `device.bulk_import`, `device.reveal_secrets`
- `internal/audit/` — `AuditWriter` pattern + diff-only before/after; same-transaction insert for every Phase 3 mutation
- `web/src/components/ui/ResponsiveDialog`, `Stepper`, TanStack Table, TanStack Query (`mutation` + `invalidate`) — full reuse
- `web/src/routes/devices/index.tsx` — Phase 2 minimal list page; Phase 3 extends with filters/sorting/pagination (D-12..D-18)
- `web/src/routes/devices/add-device-dialog.tsx` — Phase 2 4-step OTAA-only dialog; Phase 3 extends to 5 steps with OTAA/ABP toggle (D-19)
- **Add deps:** `github.com/xuri/excelize/v2` (already approved in CLAUDE.md stack — first actual use here)

### Established Patterns
- Stepped-dialog + draft-per-step + finish-as-one-tx (Phase 1 D-10 / Phase 2 D-10)
- Atomic CS+Postgres transaction with best-effort CS rollback on Postgres failure (Phase 2 D-16)
- Soft-delete via decommission/archive — never hard-delete (Phase 2 D-15, USER-01)
- Server-side enforcement of viewer secret hiding via **no-Shifter-column** invariant (Phase 2 DEV-09 structural pattern) — Phase 3 extends to reveal endpoint
- Single global tenant + single application — UI never says "tenant"/"application" (Phase 2 D-28, UX-03)

### Integration Points
- **ChirpStack `GatewayService`** — first use in Phase 3 (`CreateGateway`, `UpdateGateway`, `DeleteGateway`, `GetGatewayStats`)
- **ChirpStack `DeviceService` reveal** — `GetDeviceKeys` (OTAA), `GetDeviceActivation` (ABP) — first calls behind admin-only endpoint
- **MQTT** — no new subscriptions; gateway events already arrive on `application/+/device/+/event/up`
- **Postgres** — new tables: `gateway`, `import_job`, `import_job_row`; reuses existing `pgxpool`
- **Frontend SPA** — new routes `/gateways`, `/gateways/:id` (read-only detail or edit-dialog launcher), `/admin/imports`, `/admin/imports/:job_id`; extends `/devices` with filters and adds `/devices/import` (import dialog launcher)

</code_context>

<specifics>
## Specific Ideas

- "CSV ไม่ค่อย support ภาษาไทย" — operator concern that drove D-04a (XLSX primary, CSV secondary). Real pain: Excel on Windows-Thai locale saves CSV as TIS-620/CP874 by default → garbage in UTF-8 ingest. XLSX sidesteps the encoding negotiation entirely.
- Gateway list should feel like a **fleet ops dashboard** — counts + sparkline + freshness, not a sterile data table.
- ABP `fcnt_up`/`fcnt_down` exposure is **forward-looking**: meter-swap workflows in Phase 7+ will need carry-over counters; adding fields later is breaking.
- "Pick on map" button stays rendered-but-disabled (mirroring Phase 2 D-18) to **anchor the v5 promise visually**, not just in changelogs.

</specifics>

<deferred>
## Deferred Ideas

- **Map UI** for gateway/site pin-drop — Phase 5 (MAP-01..04 + GW-04)
- **General audit query UI** (`/admin/audit` with actor/action/resource filters) — Phase 9 (Audit & Settings)
- **`import_job` archive/purge cron** — Phase 9 (uses retention from D-35)
- **Bulk edit** (reassign site, edit tags across N devices) — not planned; revisit only if operator request
- **Custom external join-server** for LoRaWAN 1.1 — backlog (no v1 customer has asked)
- **Cursor/keyset pagination** for devices list — revisit if any install exceeds ~100K active devices
- **Per-user column-visibility persistence** — not planned (single-tenant operator tool)
- **Background-job processing for very large imports (>10k rows)** — Phase 3 commits synchronously; switch to River queue (already in stack) if benchmarks show timeouts
- **Reveal-endpoint rate limiting** — implement only if abuse is observed; out of v1 threat model

</deferred>

---

*Phase: 03-provisioning-gateways-devices-bulk-import*
*Context gathered: 2026-05-11*
