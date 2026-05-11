# Phase 3: Provisioning (Gateways, Devices, Bulk Import) — Research

**Researched:** 2026-05-11
**Domain:** ChirpStack v4 GatewayService + Excelize/v2 bulk import + TanStack Table filter/sort/pagination + OTAA/ABP activation + secret-reveal endpoint
**Confidence:** HIGH for ChirpStack v4 gRPC surface (verified against local proto sources at `/Users/suraboonsung/Documents/Programming/chirpstack-api/.../api/gateway.pb.go` + `device.pb.go` + `device_grpc.pb.go`). HIGH for migration / audit / authz patterns (verified against Phase 2 production code). MEDIUM for excelize DataValidation specifics (verified via WebSearch + pkg.go.dev). HIGH for inherited Phase 1/2 decisions.

## Summary

Phase 3 turns the Phase 2 minimal device picker into a real operator-facing provisioning surface: gateway CRUD with last-24h RX/TX sparklines (driven by ChirpStack `GatewayService.GetMetrics` — **note: not `GetGatewayStats` as Phase 3 CONTEXT D-02 names it; v4.17 renamed this**), a full devices list with site/status/last-seen filters, OTAA **and** ABP activation in the add-device dialog, and a two-phase XLSX/CSV bulk import that is safe to re-run. The phase also introduces the first server-side **secret reveal endpoint** (`POST /api/devices/:eui/keys`) that calls ChirpStack `DeviceService.GetKeys` (OTAA) / `GetActivation` (ABP) on demand — secrets are never columned on the Shifter side, extending the Phase 2 D-22 "no Shifter column" structural invariant.

The bulk import is a Postgres-backed two-phase job: dry-run inserts an `import_job` + N `import_job_row` rows with `status='preview'` and validates every row offline (zero ChirpStack calls during validate); commit phase iterates valid rows, calls `CreateDevice` + (`CreateDeviceKeys` | `Activate`) per row, and writes one audit envelope row (`device.bulk_import`) plus N per-device audit rows that all share `request_id = import_job.job_id` so a single `WHERE` query yields the lot. XLSX is primary (Thai-safe — Phase 2's specifics note `excelize/v2` Thai-locale CSV pain); CSV is supported as UTF-8-with-BOM only, rejected otherwise with a clear error.

**Primary recommendation:** Treat Phase 3 as a strict superset of Phase 2's add-device pattern — every new mutating handler reuses the established `pgx.Serializable` + atomic-CS+PG + same-tx audit-row pattern, every new authz action follows the `Can(user, action, resource)` shape with admin-only mutating actions, every new migration is a plain `NNNN_description.up.sql` (next index = **0018**), and the frontend reuses `<ResponsiveDialog>` + `<Stepper>` + `react-router-dom v7` (**not TanStack Router**) without introducing new shell primitives.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Gateway surface scope & RX/TX stats**
- **D-01:** Gateway map pin deferred to Phase 5. Phase 3 ships lat/lng as **numeric inputs + 'Pick on map' button rendered-but-disabled** with tooltip "available in v5", mirroring D-18 from Phase 2 Site dialog. GW-04 lands in Phase 5 alongside MAP-01..04; REQUIREMENTS.md traceability updated at Phase 3 close.
- **D-02:** Gateway list shows **last-24h RX count + TX count + uplink success % + hourly sparkline** column. Driven by ChirpStack `GetGatewayStats(aggregation=HOUR, start=now-24h, end=now)`. Backend caches results **1 minute** to support 50+ gateway fleets without per-row gRPC fan-out on every list refresh. **[clarification: actual v4.17 RPC name is `GetMetrics`; see ChirpStack v4 GatewayService section]**
- **D-03:** Gateway create/edit dialog shows a regulator-aware LoRaWAN **region dropdown pre-selected to the Phase 1 install default (INST-04)**. Admin may override on a per-gateway basis to support multi-region installs. Satisfies GW-02.

**Bulk import — schema & file formats**
- **D-04:** Import schema — Required: `dev_eui`, `name`, `device_profile`, `site_id` (or `site_name`). Optional: `app_eui`/`join_eui`, `app_key` (OTAA); `dev_addr`, `nwk_s_key`, `app_s_key`, `fcnt_up`, `fcnt_down` (ABP); `description`, `tags`.
- **D-04a:** Supported file formats — **XLSX (primary, Thai-safe)** and **CSV (UTF-8 with BOM)**. Backend uses `excelize/v2` for XLSX and `encoding/csv` stdlib for CSV. Import dialog offers a "Download template" XLSX with pre-formatted header row + tooltip cells.
- **D-05:** EUI/Key format — **big-endian hex only**. Backend auto-strips `0x`, `:`, `-`, lowercases, normalises to 16-hex (EUI) / 32-hex (keys). Reject malformed.
- **D-06:** Idempotency — `dev_eui` is the natural key. Pre-existing → **`already_exists`** row outcome (not error, not silent update).
- **D-07:** Partial commit — valid rows commit; invalid rows reported with per-row reason; admin downloads `errors.xlsx` (same shape + reason column).

**Two-phase dry-run / commit UX**
- **D-08:** Dry-run = validate-only, no ChirpStack calls. Checks EUI format, intra-file dups, pre-existing dev_eui, site existence, device_profile existence, required fields, key length per mode.
- **D-09:** Preview = summary banner + per-row TanStack Table (filter/sort/expandable rows for reason).
- **D-10:** Commit confirmation = button-only ("Import N devices" / Cancel). No type-to-confirm. Mirrors Phase 2 D-15.
- **D-11:** `import_job` is server-side, status enum `preview` → `committed` / `expired`. TTL 1h. Tab-close/network glitch does not lose preview; expired → re-upload.

**Devices list filters & pagination**
- **D-12:** Filters — site (multi-select) + activation status (active/inactive/never_joined) + last_seen window (24h/7d/30d/all) + text search on `name` & `dev_eui`. Server-side, sqlc-generated.
- **D-13:** **Offset-based pagination**, page-size selector 25/50/100 (default 50).
- **D-14:** Sortable columns: `name`, `dev_eui`, `site`, `last_seen`, `created_at`. Server-side sort. Indexes on `(site_id, last_seen)` and `(created_at)`.
- **D-15:** Filter state persisted in **URL query params** via **react-router-dom v7 useSearchParams** (UI-SPEC line 62 — **NOT TanStack Router** as CONTEXT loosely says). Shareable links, back/forward-aware.
- **D-16:** CSV/XLSX export = current filtered view. Columns round-trip-compatible with the import schema (D-04) + appended `last_seen`, `created_at`.
- **D-17:** Bulk decommission only — no bulk edit. Select rows → "Decommission N devices" → confirm dialog.
- **D-18:** Fixed column set (name, dev_eui, site, last_seen, activation, profile). No per-user visibility toggle.

**OTAA / ABP activation in add-device dialog**
- **D-19:** Activation mode is a dedicated step in the stepped add-device dialog (**5 steps total**: identity → site → activation (OTAA/ABP radio) → mode-specific keys → review). Extends Phase 2's 4-step OTAA-only dialog.
- **D-20:** ABP fields — required `dev_addr` (8 hex), `nwk_s_key` (32 hex), `app_s_key` (32 hex). Optional `fcnt_up`, `fcnt_down` (default 0).
- **D-21:** After successful create, dialog success state shows all keys + "Copy keys" button until admin clicks Done. Rendered once at commit; not re-fetched on re-open.
- **D-22:** **ChirpStack is the sole key store.** Shifter never columns secrets. Reveal = on-demand gRPC `GetKeys` (OTAA) / `GetActivation` (ABP), gated by `Can('device.reveal_secrets')`.
- **D-23:** MAC version is **not** asked in the dialog — inherited from the selected `device_profile`.
- **D-24:** OTAA join-identifier field labelled **"Join EUI (AppEUI for v1.0)"** with tooltip. Backend uses `join_eui` per ChirpStack v4 API.
- **D-25:** Custom external join-server deferred to backlog.

**Server-side viewer hiding for new secret fields**
- **D-26:** Reveal authorization via new action `Can('device.reveal_secrets')`. **Admin only**; viewer → 403.
- **D-27:** Structural separation. Reveal is a separate endpoint `POST /api/devices/:eui/keys` that calls ChirpStack on demand.
- **D-28:** Each reveal writes an audit row: `action='device.reveal_secrets'`, `resource_id=dev_eui`, **no diff payload**.

**Gateway add-flow & decommission**
- **D-29:** Add-gateway dialog = **single ResponsiveDialog** (no stepper). Fields: `gateway_id`, `name`, `description`, `region` (default = install default per D-03), lat/lng (numeric, per D-01), optional `tags`. Atomic CS+PG transaction on submit.
- **D-30:** Decommission semantics: soft-delete in Postgres (`archived_at`, `archived_reason='operator decommission'`) **+** `DeleteGateway` in ChirpStack. Atomic with best-effort CS rollback if PG commit fails.
- **D-31:** Decommission allowed at any time; if gateway received uplinks from ≥1 device in last 24h, confirm dialog warns *"This gateway received uplinks from X devices in the last 24h. They will route via other gateways. Proceed?"* Not a hard block.
- **D-32:** Gateway list **hides archived rows by default**. "Show archived" toggle reveals faded rows with admin-only Restore action.

**Audit-log volume & request_id grouping for bulk import**
- **D-33:** Bulk-import audit shape — **1 row per device** (`action='device.create'`, `source='bulk_import'`) **+ 1 envelope row** (`action='device.bulk_import'`, `metadata={total, created, skipped, failed, job_id}`).
- **D-34:** **`import_job.job_id` (UUID) = audit `request_id`** for every row created in that job.
- **D-35:** `import_job` and `import_job_row` retention = **90 days**. Archive/purge policy deferred to Phase 9. `audit_log` rows kept indefinitely.
- **D-36:** **No general audit query UI in Phase 3.** Phase 9 owns `/admin/audit`. Phase 3 ships **`/admin/imports/:job_id` detail page** scoped to one import job.

### Claude's Discretion

- Exact `import-template.xlsx` layout (column widths, header styling, tooltip cell text)
- TanStack Table column widths, default sort direction, sparkline visual style
- ResponsiveDialog success-state copy and "Copy keys" toast wording
- Error-row CSV/XLSX filename convention (e.g. `device-import-errors-${job_id}.xlsx`)
- Reveal-endpoint rate-limiting strategy (if any)
- Background job vs sync request for commit-phase processing (1k rows is fast; >10k may need backgrounding)

### Deferred Ideas (OUT OF SCOPE)

- **Map UI** for gateway/site pin-drop — Phase 5 (MAP-01..04 + GW-04)
- **General audit query UI** (`/admin/audit` with actor/action/resource filters) — Phase 9 (Audit & Settings)
- **`import_job` archive/purge cron** — Phase 9 (uses retention from D-35)
- **Bulk edit** (reassign site, edit tags across N devices) — not planned; revisit only if operator request
- **Custom external join-server** for LoRaWAN 1.1 — backlog (no v1 customer has asked)
- **Cursor/keyset pagination** for devices list — revisit if any install exceeds ~100K active devices
- **Per-user column-visibility persistence** — not planned (single-tenant operator tool)
- **Background-job processing for very large imports (>10k rows)** — Phase 3 commits synchronously; switch to River queue (already documented in CLAUDE.md but not yet in `go.mod`) if benchmarks show timeouts
- **Reveal-endpoint rate limiting** — implement only if abuse is observed; out of v1 threat model

</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| GW-01 | List, search, filter gateways with online/offline status, last-seen, lat/lng | §ChirpStack v4 GatewayService (List), §GetMetrics Caching (sparkline + 24h rollup), §TanStack Table |
| GW-02 | Create / edit / delete gateway via dialog, regulator-aware region picker | §ChirpStack v4 GatewayService (Create/Update/Delete), §Gateway region picker reuses `internal/install/Regions()` |
| GW-03 | Per-gateway RX/TX statistics (packet counts, success rate) | §GetMetrics Caching — RX vs TX `MetricDataset` math |
| GW-04 | Map pin — **DEFERRED to Phase 5** per D-01 | Render "Pick on map" disabled placeholder (mirrors Phase 2 Site dialog) |
| DEV-01 | List, search, filter devices with last_seen, battery, RSSI/SNR, current MP | §TanStack Table URL state, §Server-side sqlc filter/sort |
| DEV-02 | Create / edit / soft-delete device via dialogs | Inherits Phase 2 add-device atomic pattern; soft-delete already wired (D-15 Phase 2) — Phase 3 adds bulk decommission |
| DEV-03 | Paste DevEUI from sticker; parser handles both endiannesses with preview | Already shipped in Phase 2 (`internal/device/deveui.go` + `web/src/routes/devices/deveui-parser.tsx`) — Phase 3 reuses verbatim |
| DEV-04 | Defaults to OTAA; ABP supported but flagged "not recommended" | §OTAA/ABP Add-Device Dialog Extension (D-19..D-25) |
| DEV-05 | Manage device profiles incl. custom JS payload codec | Already shipped in Phase 2 (`internal/profile/`) — Phase 3 no new profile work |
| DEV-06 | Bulk-import devices from CSV — two-phase dry-run/commit | §Two-Phase Import Job + §Excelize/v2 + §CSV Parsing |
| DEV-07 | Bulk import re-run is idempotent | §Two-Phase Import Job (D-06 `already_exists` outcome) |
| DEV-08 | Downloadable template | §Excelize/v2 XLSX Import — template-write pattern |
| DEV-09 | Viewer cannot see secret fields (server-side enforced) | §device.reveal_secrets Endpoint — structural separation, no Shifter column |
| CHIRP-05 | Multi-step ChirpStack flows collapsed to one user action or stepped dialog | §OTAA/ABP Add-Device Dialog — single user action wraps `CreateDevice` + `CreateKeys`/`Activate` |
| CHIRP-06 | Operator never needs to log into ChirpStack | §All ChirpStack interactions hidden behind Shifter dialogs/handlers |
| UX-03 | Operator never sees ChirpStack-native terminology | UI-SPEC §"ChirpStack vocabulary discipline" (line 40) — no "tenant"/"application" anywhere |

</phase_requirements>

## Project Constraints (from CLAUDE.md)

| Constraint | Source | Phase 3 Impact |
|------------|--------|----------------|
| **GSD workflow enforcement** | CLAUDE.md §GSD Workflow Enforcement | All edits go through GSD; Phase 3 runs through `/gsd-execute-phase` |
| **Go 1.24+ (project on 1.25.0)** | CLAUDE.md §Core Technologies | `go.mod` already at 1.25; new code stays compatible |
| **`sqlc` + `pgx/v5` — no GORM** | CLAUDE.md §What NOT to Use | All Phase 3 SQL via sqlc-generated typed Go; `WriteAuditLog` pattern reused |
| **`chi` router** | CLAUDE.md §Core Technologies | Phase 3 routes mounted on existing chi root via `r.Route("/api/gateways", ...)` + `r.Route("/api/imports", ...)` |
| **`alexedwards/scs/v2` sessions** | CLAUDE.md §Backend Supporting Libraries | Reuse existing `SessionMgr` for auth/RBAC middleware |
| **`golang-migrate` plain SQL** | CLAUDE.md §Backend Supporting Libraries | Phase 3 migrations are `0018_*.up.sql` etc. — no ORM-coupled migrations |
| **`excelize/v2`** | CLAUDE.md §Backend Supporting Libraries — "Phase 3 first actual use" per CONTEXT | Adds `github.com/xuri/excelize/v2 v2.10.x` to `go.mod` |
| **`encoding/csv` stdlib** | CLAUDE.md §Backend Supporting Libraries | CSV path uses stdlib, faster than third-party |
| **No `GORM`** | CLAUDE.md §What NOT to Use | Phase 3 confirms — sqlc only |
| **No `gofpdf`, prefer maroto** | CLAUDE.md §What NOT to Use | Not relevant to Phase 3 (no PDF generation here) |
| **No `lib/pq`** | CLAUDE.md §What NOT to Use | Phase 3 uses `pgx/v5` exclusively |
| **JWT in localStorage forbidden** | CLAUDE.md §What NOT to Use | Reveal endpoint uses SCS cookie session — no JWT |
| **Server-side viewer secret hide via no-Shifter-column invariant** | CLAUDE.md §Stack Patterns + Phase 2 D-22 | D-22/D-27 enforce: secrets never written to Shifter columns; reveal endpoint reads on-demand from ChirpStack |
| **Argon2id (not bcrypt)** | CLAUDE.md §Backend Supporting Libraries | Not relevant to Phase 3 (no new auth handlers) |
| **`react-leaflet` v5 — `MapLibre` forbidden** | CLAUDE.md §What NOT to Use | Map UI is Phase 5; Phase 3 ships the disabled placeholder only |
| **`shadcn/ui` blue/navy palette** | CLAUDE.md §Frontend Supporting Libraries + UI-SPEC | All new Phase 3 surfaces reuse inherited tokens |
| **`react-hook-form` + `zod`** | CLAUDE.md §Frontend Supporting Libraries | All new dialogs use this pair |
| **`@tanstack/react-query` v5** | CLAUDE.md §Frontend Supporting Libraries | Already wired; Phase 3 mutations invalidate `['gateways']`, `['devices']`, `['import-job', jobID]` |
| **`@tanstack/react-table` v8** | CLAUDE.md §Frontend Supporting Libraries (Phase 2 add) | Already installed (v8.21.3 per package.json); Phase 3 extends devices/gateways/imports lists |

## ChirpStack v4 GatewayService — Exact gRPC Signatures

**Source:** Local proto sources at `/Users/suraboonsung/Documents/Programming/chirpstack-api/.../api/gateway.pb.go` + `gateway_grpc.pb.go` + `device.pb.go` + `device_grpc.pb.go`. Module imported in `go.mod`: `github.com/chirpstack/chirpstack/api/go/v4 v4.17.0`. **[VERIFIED: local Go module v4.17.0, Mar 2026 release]**

### Important rename — `GetGatewayStats` does NOT exist in v4.17

**[VERIFIED: gateway_grpc.pb.go line 29]** The CONTEXT D-02 names the RPC `GetGatewayStats`. The actual v4.17.0 method name is **`GetMetrics`** (full method name `/api.GatewayService/GetMetrics`). The request shape is `GetGatewayMetricsRequest`. Planner must use the actual name; document the rename in the gateway wrapper's GoDoc.

There is also `GetDutyCycleMetrics` (separate RPC, returns `GetGatewayDutyCycleMetricsResponse`) — **not used in Phase 3** (RX/TX success% only).

### Method table (v4.17.0)

| RPC | Full name | Request | Response | Phase 3 use |
|-----|-----------|---------|----------|-------------|
| Create | `/api.GatewayService/Create` | `CreateGatewayRequest{ Gateway *Gateway }` | `*emptypb.Empty` | D-29 add-gateway dialog |
| Get | `/api.GatewayService/Get` | `GetGatewayRequest{ GatewayId string }` | `GetGatewayResponse` | Edit-dialog hydrate, restore-after-archive |
| Update | `/api.GatewayService/Update` | `UpdateGatewayRequest{ Gateway *Gateway }` | `*emptypb.Empty` | D-29 edit dialog |
| Delete | `/api.GatewayService/Delete` | `DeleteGatewayRequest{ GatewayId string }` | `*emptypb.Empty` | D-30 decommission |
| List | `/api.GatewayService/List` | `ListGatewaysRequest{ Limit, Offset, Search, TenantId, OrderBy, OrderByDesc }` | `ListGatewaysResponse{ TotalCount, Result []*GatewayListItem }` | GW-01 list page (bootstrap fetch) |
| GetMetrics | `/api.GatewayService/GetMetrics` | `GetGatewayMetricsRequest{ GatewayId, Start *timestamppb.Timestamp, End *timestamppb.Timestamp, Aggregation common.Aggregation }` | `GetGatewayMetricsResponse{ RxPackets *Metric, TxPackets *Metric, TxPacketsPerStatus *Metric, … }` | D-02 24h sparkline + RX/TX/success% |

### `Gateway` proto shape (line 129 gateway.pb.go)

```go
type Gateway struct {
    GatewayId     string                 // EUI64 — lowercase 16-hex (LoRaWAN canonical)
    Name          string
    Description   string
    Location      *common.Location       // lat/lng/altitude/source/accuracy
    TenantId      string                 // UUID — Shifter uses single-global tenant per D-28 Phase 2
    Tags          map[string]string      // freeform; Phase 3 stores operator-entered `tags` here
    Metadata      map[string]string      // PROVIDED BY THE GATEWAY (read-only on Shifter side)
    StatsInterval uint32                 // seconds — expected stats heartbeat; reasonable default 30
}
```

### `GatewayListItem` proto shape (line 239 gateway.pb.go)

```go
type GatewayListItem struct {
    TenantId    string
    GatewayId   string
    Name        string
    Description string
    Location    *common.Location
    Properties  map[string]string        // server-derived; e.g. CS-known properties
    CreatedAt   *timestamppb.Timestamp
    UpdatedAt   *timestamppb.Timestamp
    LastSeenAt  *timestamppb.Timestamp   // **Use for "last seen" column (GW-01)**
    State       GatewayState             // enum: NEVER_SEEN=0, ONLINE=1, OFFLINE=2
}
```

**Insight:** `LastSeenAt` + `State` from `ListGateways` already give us GW-01 status without a second RPC. The 1-min cache (D-02) is only needed for the sparkline / aggregated RX/TX columns from `GetMetrics`.

### `common.Location` (line 641 common.pb.go)

```go
type Location struct {
    Latitude  float64
    Longitude float64
    Altitude  float64       // meters — leave 0 if unknown
    Source    LocationSource // UNKNOWN=0, GPS=1, CONFIG=2, GEO_RESOLVER=3 — set to CONFIG when operator-typed
    Accuracy  float32        // meters — leave 0 for CONFIG
}
```

### `common.Aggregation` enum (line 432 common.pb.go)

```go
Aggregation_HOUR   = 0
Aggregation_DAY    = 1
Aggregation_MONTH  = 2
Aggregation_MINUTE = 3
```

D-02 uses `HOUR` for the 24h sparkline (24 buckets).

### `Metric` shape — RX/TX response (line 776 common.pb.go)

```go
type Metric struct {
    Name       string                            // e.g. "rx_count"
    Timestamps []*timestamppb.Timestamp          // bucket start times — 24 entries for HOUR/24h
    Datasets   []*MetricDataset                  // each = {Label string, Data []float32}
    Kind       MetricKind                        // COUNTER / ABSOLUTE / GAUGE
}
type MetricDataset struct {
    Label string
    Data  []float32                              // 1:1 with Timestamps
}
```

For RX/TX:
- `RxPackets.Datasets[0].Data` = 24 floats (hourly RX counts)
- `TxPackets.Datasets[0].Data` = 24 floats (hourly TX counts)
- `TxPacketsPerStatus.Datasets[n].Label` = status ("OK", "TOO_LATE", "TOO_EARLY", …), `Data` = per-hour counts of that status
- **Uplink success % math:** `sum(TxPacketsPerStatus[label=="OK"].Data) / sum(all TxPacketsPerStatus[*].Data)` over the 24h window. If denominator is 0 → display "—".

### Auth model

Same as the rest of the chirpstack package — every RPC goes through the `authInterceptor` in `internal/chirpstack/client.go` which appends `authorization: Bearer <token>` to outgoing metadata. No per-RPC token plumbing. **[VERIFIED: `internal/chirpstack/client.go` lines 62-69]**

### Gotchas

1. **`GatewayId` is EUI64 hex**, just like DevEUI — 16 lowercase hex chars. The Phase 2 `internal/device/deveui.go::ParseDevEUI` parser **applies verbatim to gateway IDs** (different label, same shape). The add-gateway dialog should reuse the same MSB/LSB preview when the operator pastes a sticker. Document this in the gateway dialog tooltip.
2. **`TenantId` must be the Shifter-global tenant** — read from `chirpstack_connection.tenant_id` (Phase 2 D-28). `EnsureTenantAndApplication()` already exists in the chirpstack package (used by add-device); reuse it for gateway create.
3. **`Create` returns `*emptypb.Empty`** — no UUID. The gateway is keyed by `GatewayId` (EUI64). Shifter's `gateway` table uses `gateway_id` as the natural key but also stores its own internal `id UUID` for FK targets (mirroring `device.id` vs `device.dev_eui`).
4. **`UpdateGateway` is a full-object update**, not a patch. The wrapper must Get → mutate → Update, OR accept the full `Gateway` from the handler. Mirror Phase 2 device-profile-update shape.
5. **Region is NOT on `Gateway`** — ChirpStack v4 puts region on the **device profile**, not the gateway. Shifter stores `region` on `gateway` in Postgres for the regulator-aware picker UI (D-03), but does **not** push it to CS. (CS infers region from device profiles attached to applications that route through the gateway.) **Confirm with planner** — see Open Questions.

## GetMetrics Caching — Recommended Pattern

D-02 requires 1-min TTL caching. Below the threshold of 50+ gateways, per-row gRPC fan-out on every list refresh is wasteful. Recommended pattern:

### Schema sketch (additive to `gateway` table — not a separate cache table)

```sql
-- In 0018_gateway.up.sql:
CREATE TABLE gateway (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    gateway_id          TEXT NOT NULL UNIQUE,            -- EUI64, lowercase 16 hex
    name                TEXT NOT NULL,
    description         TEXT,
    region              TEXT NOT NULL,                   -- references internal/install/Regions().Name
    lat                 DOUBLE PRECISION,
    lng                 DOUBLE PRECISION,
    altitude            DOUBLE PRECISION,
    tags                JSONB NOT NULL DEFAULT '{}'::jsonb,
    cs_tenant_id        TEXT,                            -- CS UUID — nullable until CS create acks
    -- Stats cache (D-02). NULL = never refreshed / no data yet.
    stats_refreshed_at  TIMESTAMPTZ NULL,
    stats_rx_24h        BIGINT NULL,                     -- sum over 24h hourly buckets
    stats_tx_24h        BIGINT NULL,
    stats_tx_ok_24h     BIGINT NULL,                     -- numerator for success%
    stats_sparkline     JSONB NULL,                      -- {"rx":[…24 floats…],"tx":[…24…],"bucket_start":"…"}
    -- Soft delete (D-30, D-32).
    archived_at         TIMESTAMPTZ NULL,
    archived_reason     TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT gateway_eui_lower CHECK (gateway_id = lower(gateway_id)),
    CONSTRAINT gateway_eui_hex16 CHECK (gateway_id ~ '^[0-9a-f]{16}$'),
    CONSTRAINT gateway_name_not_empty CHECK (length(name) > 0),
    CONSTRAINT gateway_lat_range CHECK (lat IS NULL OR (lat BETWEEN -90 AND 90)),
    CONSTRAINT gateway_lng_range CHECK (lng IS NULL OR (lng BETWEEN -180 AND 180))
);
CREATE INDEX gateway_archived_idx ON gateway (archived_at) WHERE archived_at IS NULL;
CREATE INDEX gateway_region_idx ON gateway (region) WHERE archived_at IS NULL;
CREATE TRIGGER gateway_touch BEFORE UPDATE ON gateway FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
```

### Refresh strategy

**Lazy + single-flight + TTL.** Pseudocode for the list handler:

```go
// On GET /api/gateways list:
// 1. Load all active rows from Postgres (fast, indexed).
// 2. For rows with stats_refreshed_at < now()-1min (or NULL), enqueue async refresh.
//    Use sync.Map[gatewayID]struct{} as in-process single-flight guard so
//    concurrent list requests don't fan out N times for the same gateway.
// 3. Return list immediately with whatever stats are in PG (possibly stale).
// 4. Async refresh goroutine: call GetMetrics, compute the 4 aggregates, UPDATE gateway SET stats_*.
//
// First-load on a fresh install: stats_* all NULL → UI renders "—" placeholders.
// 1 min later (after async refresh ack'd), next list refresh shows populated.
```

This avoids blocking the list endpoint on N gRPC calls and is correct for the operator UX ("show what we know now, refresh in background"). The 1-min TTL is per-row, not per-list — opening the same list twice within 1 min skips the refresh entirely.

**Postgres-backed, not Redis** — matches the CLAUDE.md stack ("Postgres-backed everything until Redis is justified"). No new dependency.

**Single-flight via `golang.org/x/sync/singleflight`** is also viable if the in-process `sync.Map` proves clumsy. The package isn't yet in `go.mod`; pulling it in is one-line.

### Sparkline JSON shape

Store as `JSONB` for round-trip ease — the frontend just reads `gateway.stats_sparkline.rx[]` directly:

```json
{
  "bucket_start": "2026-05-11T08:00:00Z",
  "bucket_width_s": 3600,
  "rx": [12, 14, 9, 10, …, 11],   // 24 floats
  "tx": [3, 4, 2, 2, …, 4]         // 24 floats
}
```

24 floats × 2 series ≈ ~400 bytes JSON per gateway — negligible at 100-gateway scale.

## Excelize/v2 XLSX Import — API + Template Patterns

**[VERIFIED: pkg.go.dev/github.com/xuri/excelize/v2 v2.10.x — Feb 2026]**

### Module + version

Add to `go.mod`: `github.com/xuri/excelize/v2 v2.10.0` (or whatever is current at planning time — `go get github.com/xuri/excelize/v2@latest`). 2.10 requires Go 1.24+ which we satisfy (Go 1.25.0).

### Read pattern (≤500 rows — load-into-memory is fine)

```go
import "github.com/xuri/excelize/v2"

f, err := excelize.OpenFile(path)        // OR excelize.OpenReader(reader) for uploads
if err != nil { return err }
defer f.Close()

rows, err := f.GetRows("Devices")        // returns [][]string, fully in memory
if err != nil { return err }
header := rows[0]                         // first row is header
for i, row := range rows[1:] {
    rowIndex := i + 2                     // 1-based, +1 for header offset
    // parse row[col] by header name…
}
```

Memory profile: 500 rows × 12 cols × ~30 chars ≈ ~180KB total in Go strings. Trivial. **No streaming needed at this scale.**

For larger imports (>10K rows), excelize provides `f.Rows("Devices")` returning a streaming row iterator — file remains memory-mapped. Out of scope for Phase 3 (D-Discretion #6).

### Template-write pattern (Download template XLSX, DEV-08)

```go
f := excelize.NewFile()
defer f.Close()

const sheet = "Devices"
f.SetSheetName(f.GetSheetName(0), sheet)

headers := []string{
    "dev_eui", "name", "device_profile", "site_id",
    "activation_mode", "join_eui", "app_key",
    "dev_addr", "nwk_s_key", "app_s_key", "fcnt_up", "fcnt_down",
    "description", "tags",
}
for i, h := range headers {
    cell, _ := excelize.CoordinatesToCellName(i+1, 1)
    f.SetCellValue(sheet, cell, h)
}

// Header style — bold, navy fill (matches Shifter palette).
hStyle, _ := f.NewStyle(&excelize.Style{
    Font: &excelize.Font{Bold: true, Color: "#FFFFFF"},
    Fill: excelize.Fill{Type: "pattern", Color: []string{"#1E3A8A"}, Pattern: 1},
    Alignment: &excelize.Alignment{Horizontal: "center"},
})
f.SetCellStyle(sheet, "A1", "N1", hStyle)

// Column widths — operator readability.
f.SetColWidth(sheet, "A", "A", 22)   // dev_eui
f.SetColWidth(sheet, "B", "B", 28)   // name
// … etc

// Per-cell tooltips via SetCellRichText is awkward; the established pattern
// is a second row of "comments" attached via AddComment:
f.AddComment(sheet, excelize.Comment{
    Cell:   "A1",
    Author: "Shifter",
    Paragraph: []excelize.RichTextRun{
        {Text: "DevEUI (16 hex chars, big-endian). Required.\nExample: 70b3d59999000001"},
    },
})

// Data validation — dropdown on activation_mode (column E):
dvActivation := excelize.NewDataValidation(true)
dvActivation.Sqref = "E2:E1000"
_ = dvActivation.SetDropList([]string{"OTAA", "ABP"})
dvActivation.SetError(excelize.DataValidationErrorStyleStop, "Invalid activation", "Must be OTAA or ABP.")
_ = f.AddDataValidation(sheet, dvActivation)

// Save the template to a temp path / write to HTTP response writer.
_ = f.SaveAs("/tmp/import-template.xlsx")
// OR for HTTP:
//   w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
//   w.Header().Set("Content-Disposition", `attachment; filename="device-import-template.xlsx"`)
//   _ = f.Write(w)
```

### `errors.xlsx` generation (D-07)

Same shape as the input + appended `error_reason TEXT` column. The handler reads `import_job_row` rows where `status='invalid' OR status='failed'`, reconstructs the input cells from `raw_payload JSONB`, appends the reason from `import_job_row.reason TEXT`, calls `f.Write(httpResponseWriter)`.

### XLSX file size limits / Excel compat quirks

- Max rows per sheet: 1,048,576 (Excel 2007+). 500–10K rows nowhere near.
- Sheet name max 31 chars — keep it short ("Devices").
- Header row should NOT be frozen unless the user explicitly wants it; freeze is purely cosmetic via `f.SetPanes`.
- Numbers stored as strings (text format) avoid Excel's "scientific notation on long hex strings" trap — relevant for DevEUI columns. Use `f.SetCellStr` (not `SetCellValue`) for hex columns, OR pre-set the column format to `@` (text):

```go
fmtTextID, _ := f.NewStyle(&excelize.Style{NumFmt: 49})  // 49 = "@" (text)
f.SetColStyle(sheet, "A", fmtTextID)                      // dev_eui column always text
```

This is **essential** — without it, Excel mangles `70B3D5999999E01F` to `7.03E+15` on open + save.

## CSV Parsing — UTF-8 BOM, Thai Encoding, stdlib quirks

D-04a allows CSV as a secondary input ("UTF-8 with BOM"). The CONTEXT specifics call out the operator pain — Excel on Windows-Thai saves CSV as TIS-620 / CP874 by default, which corrupts on UTF-8 ingest.

### Pattern: strict UTF-8, BOM-strip, reject non-UTF-8

```go
import (
    "encoding/csv"
    "io"
    "unicode/utf8"
)

const utf8BOM = "\xEF\xBB\xBF"

func parseCSV(r io.Reader) ([][]string, error) {
    // Sniff first 3 bytes; if BOM, strip; if not BOM but bytes look non-UTF-8
    // (any byte 0x80-0x9F outside multi-byte sequences common in TIS-620),
    // reject with a clear error.
    br := bufio.NewReader(r)
    peek, _ := br.Peek(3)
    if bytes.Equal(peek, []byte(utf8BOM)) {
        _, _ = br.Discard(3)
    }
    // Read into memory (≤500 rows × ~30 cols × ~30 bytes ≈ ~450KB — fine).
    all, err := io.ReadAll(br)
    if err != nil { return nil, err }
    if !utf8.Valid(all) {
        return nil, fmt.Errorf("CSV is not valid UTF-8. Re-save the file as 'CSV UTF-8' or use the XLSX template instead.")
    }
    rdr := csv.NewReader(bytes.NewReader(all))
    rdr.FieldsPerRecord = -1   // tolerate trailing-comma quirks
    rdr.LazyQuotes = true      // permit unescaped quotes in description / tags
    return rdr.ReadAll()
}
```

**The error message is the UX.** Don't try to auto-detect TIS-620; the operator pain D-04a documents is that auto-decoded Thai text looks fine in some places and garbage elsewhere ("mojibake"). Clear, actionable error → operator re-saves as UTF-8 (or switches to XLSX) → fixed.

### stdlib `encoding/csv` quirks worth knowing

- `csv.Reader.FieldsPerRecord = 0` (default) locks the field count to the first record — set to `-1` to tolerate trailing-comma irregularity.
- `csv.Reader.LazyQuotes = true` is needed for typical operator-edited CSVs (descriptions like `"Building "A" main"`).
- `csv.Reader.TrimLeadingSpace = true` is convenient (cells like `, OTAA ,` become `OTAA`).
- For very large CSVs, use `csv.Reader.Read()` in a loop rather than `ReadAll()` — but at Phase 3 sizes, `ReadAll` is fine.

## Two-Phase Import Job — Schema & State Machine

### `import_job` and `import_job_row` schema sketch

```sql
-- 0019_import_job.up.sql
CREATE TYPE import_job_status AS ENUM ('preview', 'committed', 'expired', 'failed');
CREATE TYPE import_job_row_status AS ENUM ('valid', 'invalid', 'already_exists', 'created', 'failed');

CREATE TABLE import_job (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id          UUID NOT NULL UNIQUE DEFAULT gen_random_uuid(),  -- external-facing; = audit request_id (D-34)
    owner_id        UUID NOT NULL REFERENCES "user"(id) ON DELETE SET NULL,
    file_name       TEXT NOT NULL,
    file_format     TEXT NOT NULL CHECK (file_format IN ('xlsx','csv')),
    total_rows      INTEGER NOT NULL DEFAULT 0,
    status          import_job_status NOT NULL DEFAULT 'preview',
    -- Summary counters (populated after dry-run; updated after commit).
    valid_count     INTEGER NOT NULL DEFAULT 0,
    invalid_count   INTEGER NOT NULL DEFAULT 0,
    already_exists_count INTEGER NOT NULL DEFAULT 0,
    created_count   INTEGER NOT NULL DEFAULT 0,
    failed_count    INTEGER NOT NULL DEFAULT 0,
    -- TTL (D-11). expires_at = created_at + 1h on preview; cleared on commit.
    expires_at      TIMESTAMPTZ NULL,
    committed_at    TIMESTAMPTZ NULL,
    failed_reason   TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX import_job_owner_idx ON import_job (owner_id, created_at DESC);
CREATE INDEX import_job_status_idx ON import_job (status, expires_at) WHERE status = 'preview';
CREATE TRIGGER import_job_touch BEFORE UPDATE ON import_job FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

CREATE TABLE import_job_row (
    id              BIGSERIAL PRIMARY KEY,
    import_job_id   UUID NOT NULL REFERENCES import_job(id) ON DELETE CASCADE,
    row_index       INTEGER NOT NULL,                                -- 1-based, matches XLSX/CSV row
    raw_payload     JSONB NOT NULL,                                  -- original column → string (lossless for errors.xlsx round-trip)
    parsed          JSONB,                                           -- canonicalised values after validation (lowercase EUI, parsed UUIDs, etc.)
    status          import_job_row_status NOT NULL DEFAULT 'valid',
    reason          TEXT,                                            -- human-readable error / skip reason
    created_device_id UUID NULL REFERENCES device(id),               -- populated when commit-phase creates the device
    UNIQUE (import_job_id, row_index)
);
CREATE INDEX import_job_row_status_idx ON import_job_row (import_job_id, status);
```

### Status state machine

```
                   upload + dry-run
                         │
                         ▼
              ┌──────────────────────┐
              │   import_job: preview │
              │   expires_at = now+1h │
              └─────────┬────────────┘
                        │
            ┌───────────┼──────────┐
            ▼           ▼          ▼
       (operator    (background  (operator
        clicks       cleanup     cancels —
        Commit)     job sweeps   no-op,
                    expired)     drops on TTL)
            │           │
            ▼           ▼
   ┌────────────┐  ┌────────────┐
   │ committed  │  │  expired   │
   │ (terminal) │  │ (terminal) │
   └────────────┘  └────────────┘
            ▲
            │ on commit-phase ANY-row CS error
            │ (best-effort: continue with remaining rows)
            ▼
   ┌────────────┐
   │   failed   │  ← only if dry-run itself crashed / file unreadable
   │ (terminal) │     OR if commit aborts catastrophically
   └────────────┘     (the per-row 'failed' status lives on
                       import_job_row, NOT here)
```

**Commit semantics (D-07 + D-10):**
- Iterate `import_job_row WHERE status='valid'` in row_index order.
- Per row: open Serializable txn → CS CreateDevice + CreateKeys / Activate → INSERT device → audit row → COMMIT.
- On per-row CS failure: best-effort CS DeleteDevice cleanup (Phase 2 pattern), update `import_job_row.status='failed'` + reason, **continue to next row**.
- After loop: update `import_job.status='committed'`, populate summary counters, write envelope audit row (D-33).
- Operator's UI re-fetches the job detail page and sees per-row outcomes.

### Resumption flow

A `preview` job is resumable via `/admin/imports/:job_id`:
1. Frontend loads the page, fetches `GET /api/imports/:job_id` (returns job + paginated rows).
2. If `status='preview'` and `expires_at > now`, the page renders the preview banner + table + "Import N devices" / Cancel buttons.
3. If `expires_at < now`, backend lazily transitions the row to `expired` on read (or on the next list query); UI shows "This preview has expired. Re-upload to retry."
4. If `status='committed'`, page renders the outcome table + download-errors button. (D-36)

### Concurrency

Single-tenant install — two admins importing the same XLSX simultaneously is rare but possible. The `device.dev_eui` UNIQUE constraint catches it: whichever transaction commits first wins, the other gets a 23505 (unique_violation) which the commit-phase per-row handler maps to `status='failed'` + reason "DevEUI was created concurrently by another operator." No special locking needed.

### Background processing decision (D-Discretion #6)

For Phase 3, **commit synchronously** in the HTTP request handler. At 500 rows × ~150ms per CS roundtrip (typical), the commit takes ~75 seconds — within HTTP timeout budget if the planner sets the chi server's `WriteTimeout` to ≥ 120s (or uses a streaming JSON response). For >10K rows, defer to a future River job.

If the planner wants a defensive option now: stream a `text/event-stream` (SSE) per-row outcome from the commit endpoint so the operator sees live progress and the connection never idles. Pattern is small and self-contained.

## DevEUI / Key Validation — Normalization Rules

**[VERIFIED: `internal/device/deveui.go::ParseDevEUI` lines 59-92]**

### Big-endian hex normalization (D-05)

For each input field (`dev_eui`, `join_eui`, `app_eui`, `app_key`, `nwk_s_key`, `app_s_key`, `dev_addr`):

```go
func normalizeHex(raw string, wantChars int) (string, error) {
    cleaned := strings.Map(func(r rune) rune {
        switch r {
        case ' ', '-', ':', '\t', '\n', '\r':
            return -1
        }
        return unicode.ToLower(r)
    }, raw)
    // Strip optional 0x prefix.
    cleaned = strings.TrimPrefix(cleaned, "0x")
    if len(cleaned) != wantChars {
        return "", fmt.Errorf("expected %d hex chars after normalisation, got %d", wantChars, len(cleaned))
    }
    if _, err := hex.DecodeString(cleaned); err != nil {
        return "", fmt.Errorf("not valid hex: %w", err)
    }
    return cleaned, nil
}
```

### Length rules (LoRaWAN spec)

| Field | Hex chars | Bytes | Notes |
|-------|-----------|-------|-------|
| `dev_eui` | **16** | 8 | EUI64 |
| `join_eui` (= `app_eui` v1.0) | **16** | 8 | EUI64 |
| `app_key` | **32** | 16 | AES-128 root key |
| `nwk_s_key` | **32** | 16 | LoRaWAN 1.0.x NwkSKey; for v1.1.x also use as NwkSEncKey / SNwkSIntKey / FNwkSIntKey (CS DeviceActivation needs all 3 set to NwkSKey for 1.0.x devices per `device.pb.go` line 1178-1185) |
| `app_s_key` | **32** | 16 | AppSKey |
| `dev_addr` | **8** | 4 | 32-bit DevAddr |

### Sticker-paste support

Phase 2 ships `internal/device/deveui.go::ParseDevEUI` which:
1. Strips whitespace/colons/hyphens/`0x`.
2. Lowercases.
3. Validates exactly 16 hex chars.
4. Returns BOTH MSB-first and LSB-first interpretations + OUI vendor hint.

For Phase 3:
- **`dev_eui`** already covered by Phase 2 parser — reused verbatim in the add-device dialog step 1.
- **`join_eui`** — same EUI64 shape. **Recommendation: extend Phase 2 parser** with a `ParseEUI64(raw)` alias (currently named `ParseDevEUI` — same code) and use it for `join_eui` too. Sticker preview UX in step 4 (OTAA) shows both endiannesses.
- **`app_key`, `nwk_s_key`, `app_s_key`** — 32 hex chars, no canonical "swap" concept (these are AES keys, not byte-order-sensitive identifiers). Just normalise via `normalizeHex(raw, 32)` and reject on bad length / hex.
- **`dev_addr`** — 8 hex chars, no endianness preview needed (32-bit value, operator-entered).

### CHECK constraints in Postgres

The Phase 2 `device.dev_eui` CHECK is `dev_eui ~ '^[0-9a-f]{16}$'`. Replicate for:
- New `gateway.gateway_id`: `gateway_id ~ '^[0-9a-f]{16}$'`
- Keep `device.join_eui ~ '^[0-9a-f]{16}$' OR NULL` (already in 0012).
- ABP fields stay **out of Shifter columns** (D-22 / D-27) — they live in ChirpStack only.

## Audit Envelope + Per-Row Pattern

### Envelope + per-row (D-33, D-34)

Per bulk import, the system writes:
1. **N per-device rows** — `action='device.create'`, `entity_type='device'`, `entity_id=<device_id>`, `before=NULL`, `after=<device-create-after-map>`, `notes='bulk_import'`, `request_id=<job_id>`.
2. **1 envelope row** — `action='device.bulk_import'`, `entity_type=???`, `entity_id=<job_id>`, `before=NULL`, `after={total, created, skipped, failed, job_id}`, `notes=NULL`, `request_id=<job_id>`.

### Audit-log shape gap — vocabulary extension

**[VERIFIED: `internal/db/migrations/0016_audit_log.up.sql` lines 37-44]** The Phase 2 audit_log CHECK constraints lock the `action` and `entity_type` vocabularies:

```
action IN ('create','update','archive','restore','swap','decommission',
           'profile_create','profile_update','binding_open','binding_close',
           'rollover_detected')
entity_type IN ('site','metering_point','device','device_profile','binding')
```

Phase 3 needs **new actions**: `bulk_import` (envelope), `reveal_secrets`, and **new entity_types**: `gateway`, `import_job`.

**Migration:** `0020_audit_log_phase3_vocab.up.sql` — `ALTER TABLE audit_log DROP CONSTRAINT audit_log_action_valid` + re-`ADD CONSTRAINT` with extended set; same for `entity_type`. The Phase 2 `internal/audit/log.go` action/entity constants get new siblings (`ActionBulkImport`, `ActionRevealSecrets`, `EntityTypeGateway`, `EntityTypeImportJob`).

### `metadata` shape — reuse `after` JSONB, no schema change

The audit_log already has `before JSONB` + `after JSONB`. For the bulk-import envelope:
- `before` = NULL
- `after` = `{"total": 142, "created": 142, "skipped": 8, "failed": 3, "job_id": "<uuid>", "file_name": "...", "file_format": "xlsx"}`

For `reveal_secrets`:
- `before` = NULL
- `after` = `{"dev_eui": "70b3d5...", "activation_mode": "OTAA"}` (no secret material; just enough to reconstruct who revealed what)

**No `metadata` JSONB column is needed.** The audit_log's existing `after JSONB` covers both cases cleanly.

### request_id grouping

Every per-device row + the envelope row share `request_id = job_id`. To query "all rows for one import":

```sql
SELECT * FROM audit_log WHERE request_id = $1 ORDER BY time ASC;
-- The envelope sorts first (commit-phase end) or last depending on emit order;
-- the planner should pick one and document it.
```

The existing index `audit_log_request_id_idx` (partial WHERE request_id IS NOT NULL) covers this.

**Note on chi `middleware.RequestID`:** Phase 2 handlers use `middleware.GetReqID(r.Context())` to populate `request_id`. Bulk-import per-row writes must **override** that with `job_id` (string) — pass it explicitly to `audit.WriteEntry` instead of the chi request ID. Document this override at the call site to prevent confusion.

## device.reveal_secrets Endpoint

### HTTP shape

`POST /api/devices/:eui/keys` — chosen over GET because:
1. Revealing keys is a side-effect (writes an audit row per D-28). GET semantics imply idempotent reads with no side effects.
2. CSRF / pre-flight semantics align with state-changing endpoints.
3. Aligns with the rest of Shifter's mutation idiom.

### Request body

`POST /api/devices/{dev_eui}/keys` — empty body. The path parameter is the lowercase 16-hex DevEUI.

### Auth

- `auth.RequireAction(sm, auth.ActionDeviceRevealSecrets)` middleware in front (new action added to `authz.go`).
- Admin-only — viewer → 403.

### Response shapes

**OTAA (200 OK):**
```json
{
  "activation_mode": "OTAA",
  "dev_eui": "70b3d59999000001",
  "join_eui": "0000000000000000",
  "app_key": "f1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6",
  "nwk_key": "f1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6"
}
```

**ABP (200 OK):**
```json
{
  "activation_mode": "ABP",
  "dev_eui": "70b3d59999000001",
  "dev_addr": "01020304",
  "nwk_s_key": "…32 hex…",
  "app_s_key": "…32 hex…",
  "f_cnt_up": 0,
  "n_f_cnt_down": 0,
  "a_f_cnt_down": 0
}
```

**Errors:**
- 401 (no session) — handled by `RequireAction` middleware.
- 403 (viewer) — handled by `RequireAction` middleware.
- 404 — device not in Shifter PG (`pgx.ErrNoRows` → `{"error":"not_found"}`).
- 502 — ChirpStack call failed (CS unreachable, OR device deleted in CS but still in Shifter).
- 409 — device exists in Shifter but neither `GetKeys` nor `GetActivation` returns a result (the device has been created in CS but neither keys nor activation pushed — rare; useful diagnostic).

### Backend flow

```go
// internal/device/handlers.go (added handler)
func revealSecrets(deps Deps) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // 1. Auth already enforced by RequireAction middleware.
        user, ok := requireAdmin(deps.SessionMgr, w, r, auth.ActionDeviceRevealSecrets)
        if !ok { return }

        // 2. Parse :eui, lowercase + length validate.
        devEUI := strings.ToLower(strings.TrimSpace(chi.URLParam(r, "eui")))
        if !isHex(devEUI, 16) {
            writeJSON(w, http.StatusBadRequest, errorResp{Error: "invalid_dev_eui"})
            return
        }

        // 3. Look up device in Shifter PG (404 if not found).
        q := sqlc.New(deps.Pool)
        dev, err := q.GetDeviceByDevEUI(r.Context(), devEUI)
        if errors.Is(err, pgx.ErrNoRows) {
            writeJSON(w, http.StatusNotFound, errorResp{Error: "not_found"})
            return
        }
        if err != nil { internalError(deps.Log, w, "lookup device", err); return }

        // 4. Try GetKeys (OTAA). If found, that's the answer.
        keys, errKeys := deps.CS.GetDeviceKeys(r.Context(), devEUI)
        if errKeys == nil {
            // Audit + return OTAA shape.
            writeAuditReveal(r.Context(), deps, user, dev, "OTAA")
            writeJSON(w, http.StatusOK, map[string]any{
                "activation_mode": "OTAA",
                "dev_eui":         devEUI,
                "join_eui":        derefString(dev.JoinEui),
                "app_key":         keys.AppKey,
                "nwk_key":         keys.NwkKey,
            })
            return
        }
        // CS NotFound → fall through to GetActivation.
        // CS other error → 502.
        if !isCSNotFound(errKeys) {
            writeJSON(w, http.StatusBadGateway, errorResp{Error: "cs_get_keys_failed", Detail: errKeys.Error()})
            return
        }

        // 5. Try GetActivation (ABP).
        act, errAct := deps.CS.GetDeviceActivation(r.Context(), devEUI)
        if errAct == nil {
            writeAuditReveal(r.Context(), deps, user, dev, "ABP")
            writeJSON(w, http.StatusOK, map[string]any{
                "activation_mode": "ABP",
                "dev_eui":         devEUI,
                "dev_addr":        act.DevAddr,
                "nwk_s_key":       act.NwkSEncKey,   // = NwkSKey for 1.0.x
                "app_s_key":       act.AppSKey,
                "f_cnt_up":        act.FCntUp,
                "n_f_cnt_down":    act.NFCntDown,
                "a_f_cnt_down":    act.AFCntDown,
            })
            return
        }
        if !isCSNotFound(errAct) {
            writeJSON(w, http.StatusBadGateway, errorResp{Error: "cs_get_activation_failed", Detail: errAct.Error()})
            return
        }

        // 6. Neither OTAA nor ABP — device exists in Shifter but no keys/activation in CS.
        writeJSON(w, http.StatusConflict, errorResp{Error: "no_credentials_in_cs"})
    }
}
```

### Audit interaction (D-28)

```go
func writeAuditReveal(ctx context.Context, deps Deps, user auth.User, dev sqlc.Device, mode string) {
    // Open a thin tx just for the audit row — no domain mutation here.
    tx, _ := deps.Pool.BeginTx(ctx, pgx.TxOptions{})
    defer tx.Rollback(ctx)
    _ = audit.WriteEntry(ctx, tx, audit.Entry{
        UserID:     mustParseUUID(user.ID),
        Action:     audit.ActionRevealSecrets,           // new const in audit/log.go
        EntityType: audit.EntityTypeDevice,
        EntityID:   uuid.UUID(dev.ID.Bytes),
        Before:     nil,
        After:      map[string]any{"activation_mode": mode},   // NO secret material
        Notes:      "",
        RequestID:  middleware.GetReqID(ctx),
    })
    _ = tx.Commit(ctx)
}
```

Per D-28, the audit row has **no diff payload** — only the activation mode and dev_eui are recorded. The keys themselves are never logged.

### Frontend secret panel

`web/src/routes/devices/$id.tsx` (new) — device detail page with a `<RevealKeysButton>` admin-only component (hidden when `user.role !== 'admin'` as defense-in-depth, but the server enforces). Click → `useMutation(['device-keys', devEUI], () => fetch('POST /api/devices/:eui/keys'))` → render `<KeysPanel keys={...}>` with "Copy" buttons. Closing the dialog clears local state — keys are not retained in `react-query` cache (`cacheTime: 0` on the mutation).

## react-router-dom v7 URL State for Filters (D-15)

**[CORRECTION: CONTEXT D-15 says "TanStack Router search params"; UI-SPEC line 62 + `App.tsx` line 3 confirm the project uses `react-router-dom v7`. Phase 3 must use `react-router-dom v7`'s URL state APIs. The planner should NOT introduce TanStack Router as a new dependency.]**

### URL state schema (zod-validated)

```typescript
// web/src/routes/devices/search-params.ts
import { z } from 'zod'

export const devicesSearchSchema = z.object({
  site: z.array(z.string().uuid()).optional().default([]),         // multi-select
  status: z.enum(['active', 'inactive', 'never_joined']).optional(),
  last_seen: z.enum(['24h', '7d', '30d', 'all']).optional().default('all'),
  q: z.string().optional().default(''),
  page: z.coerce.number().int().min(1).default(1),
  per_page: z.union([z.literal(25), z.literal(50), z.literal(100)]).default(50),
  sort: z
    .enum(['name', '-name', 'dev_eui', '-dev_eui', 'site', '-site',
           'last_seen', '-last_seen', 'created_at', '-created_at'])
    .default('-last_seen'),
})

export type DevicesSearch = z.infer<typeof devicesSearchSchema>
```

### Read pattern

```typescript
import { useSearchParams } from 'react-router-dom'

function useDevicesSearch(): [DevicesSearch, (next: Partial<DevicesSearch>) => void] {
  const [params, setParams] = useSearchParams()
  // Parse with zod, falling back to defaults on parse failure.
  const raw = {
    site: params.getAll('site'),
    status: params.get('status') ?? undefined,
    last_seen: params.get('last_seen') ?? 'all',
    q: params.get('q') ?? '',
    page: params.get('page') ?? '1',
    per_page: params.get('per_page') ?? '50',
    sort: params.get('sort') ?? '-last_seen',
  }
  const parsed = devicesSearchSchema.safeParse(raw)
  const search: DevicesSearch = parsed.success ? parsed.data : devicesSearchSchema.parse({})

  const update = (next: Partial<DevicesSearch>) => {
    setParams((prev) => {
      const sp = new URLSearchParams(prev)
      for (const [k, v] of Object.entries(next)) {
        if (Array.isArray(v)) {
          sp.delete(k)
          for (const item of v) sp.append(k, item)
        } else if (v === undefined || v === '' || v === null) {
          sp.delete(k)
        } else {
          sp.set(k, String(v))
        }
      }
      return sp
    }, { replace: false })  // pushes a new history entry — back/forward works
  }
  return [search, update]
}
```

### Deep-link pattern

A bookmarked URL `/devices?site=<uuid>&status=active&last_seen=24h&page=2&sort=-created_at` round-trips exactly. The default values match the zod schema's `.default(...)` so omitted params stay omitted in the URL (clean canonical URLs).

### react-router-dom v7 caveats

- `useSearchParams` re-renders the whole component on every change — debounce typed `q` updates (250ms) to avoid spamming the URL bar.
- Multi-value params (`site=uuid1&site=uuid2`) work via `URLSearchParams.append` and `params.getAll('site')`. **Don't** join into one comma-separated string — breaks the zod array shape.

## TanStack Table for devices list + import preview (D-09, D-14)

Already installed (`@tanstack/react-table v8.21.3` per `package.json`).

### Devices list — server-side

```typescript
const table = useReactTable({
    data: devicesQuery.data?.rows ?? [],
    columns,
    pageCount: devicesQuery.data?.page_count ?? 0,
    state: { pagination: { pageIndex: search.page - 1, pageSize: search.per_page } },
    manualPagination: true,
    manualSorting: true,
    manualFiltering: true,
    getCoreRowModel: getCoreRowModel(),
})
```

**No client-side filter/sort/pagination.** Server returns pre-filtered, pre-sorted, pre-paginated rows + `page_count` + `total_count`.

### Import preview — row expansion (D-09 "expandable rows for per-row reason")

```typescript
const [expanded, setExpanded] = useState<ExpandedState>({})
const table = useReactTable({
    data: rowsQuery.data ?? [],
    columns,
    state: { expanded },
    onExpandedChange: setExpanded,
    getExpandedRowModel: getExpandedRowModel(),
    getCoreRowModel: getCoreRowModel(),
})
```

The row's expanded content renders `row.original.reason` in a sub-row (full-width cell with `colspan={columns.length}`).

### Row selection for bulk decommission (D-17)

```typescript
const [rowSelection, setRowSelection] = useState({})
useReactTable({ state: { rowSelection }, onRowSelectionChange: setRowSelection, … })
// Checkbox column at index 0:
{
    id: 'select',
    header: ({ table }) => (
      <Checkbox
        checked={table.getIsAllPageRowsSelected()}
        onCheckedChange={(v) => table.toggleAllPageRowsSelected(Boolean(v))}
      />
    ),
    cell: ({ row }) => (
      <Checkbox checked={row.getIsSelected()} onCheckedChange={(v) => row.toggleSelected(Boolean(v))} />
    ),
}
const selectedIDs = Object.keys(rowSelection).filter((k) => rowSelection[k]).map((k) => table.getRow(k).original.id)
```

## OTAA/ABP Add-Device Dialog Extension

### 5-step shape (D-19)

Replaces Phase 2's 4-step dialog (`web/src/routes/devices/add-device-dialog.tsx`):

| Step | Heading | Fields | Next-button gate |
|------|---------|--------|------------------|
| 1 | "Identity" | DevEUI (paste-parser inherited from Phase 2), name, description | `dev_eui.length === 16 && name.trim() !== ''` |
| 2 | "Bind to site / metering point" | Site (select), Metering Point (select, optional), initial reading | MP selected ? reading parses : true |
| 3 | "Activation" | Radio: OTAA (recommended) / ABP (not recommended — warning), Device Profile | `activation_mode !== '' && profile_id !== ''` |
| 4 | OTAA: "OTAA keys" — JoinEUI, AppKey<br>ABP: "ABP session" — DevAddr, NwkSKey, AppSKey, FCntUp, FCntDown | mode-specific hex validation | All required mode-specific fields valid |
| 5 | "Review" | Read-only summary + preflight + Add device button | n/a |

**Note:** the 5-step ordering differs slightly from CONTEXT D-19 wording — the CONTEXT says "identity → site → activation → keys → review" which matches the 5-step table above. The Phase 2 4-step dialog ordering was "DevEUI → profile → MP → review". Phase 3 reorders to move profile into the activation step (since OTAA/ABP also needs profile choice, and ABP-only profiles vs OTAA-only profiles exist in CS).

### ABP "not recommended" warning copy

UI-SPEC §13 / CONTEXT D-04 (REQ DEV-04: "ABP is supported but flagged as not recommended"):

> ABP is not recommended. Frame counters reset on device power cycle, which requires manual intervention to re-sync the meter. Use OTAA unless your device only supports ABP.

(Wording is Claude's discretion — confirm with planner.)

### ChirpStack gRPC calls per mode

| Mode | Step 5 Submit calls (atomic txn) |
|------|----------------------------------|
| OTAA | `CreateDevice` + `CreateDeviceKeys` (already exists in Phase 2; reuse `CreateDeviceWithKeys`) |
| ABP | `CreateDevice` + **`ActivateDevice`** (new wrapper needed: `c.ActivateDevice(ctx, devEUI, devAddr, nwkSKey, appSKey, fCntUp, fCntDown)`) |

The new `ActivateDevice` wrapper (in `internal/chirpstack/device.go`) wraps `api.NewDeviceServiceClient(c.conn).Activate(ctx, &api.ActivateDeviceRequest{DeviceActivation: &api.DeviceActivation{...}})`. Pass `NwkSEncKey = SNwkSIntKey = FNwkSIntKey = nwkSKey` (per `device.pb.go` line 1178-1185: for LoRaWAN 1.0.x devices, all three are set to NwkSKey).

### Draft-per-step (Phase 2 D-10 inheritance)

Each step's Next button persists to local React state (no backend draft tables in Phase 3). Only step 5's "Add device" submit writes anything to the server.

### Success state — keys panel (D-21)

After successful create, the dialog body re-renders to a success state showing:
- "Device added" with checkmark
- All keys (OTAA: AppKey, NwkKey, JoinEUI; ABP: DevAddr, NwkSKey, AppSKey, FCnt counters)
- "Copy keys" button (uses `navigator.clipboard.writeText(JSON.stringify(keys, null, 2))`)
- "Done" closes dialog

Keys are passed from the server's `POST /api/devices` 201 response body (CS returns the keys we just wrote — no re-fetch). On dialog close, local state is cleared. **Not re-fetched on re-open** (D-21).

## Gateway Decommission + Restore Semantics

### Decommission (D-30, D-31)

```go
// internal/gateway/handlers.go (new package mirroring internal/device)
func decommissionGateway(deps Deps) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // 1. RBAC + parse :id.
        // 2. Load gateway from PG (404 if not found).
        // 3. If stats_rx_24h > 0, the UI has already shown the warning;
        //    backend does NOT re-check (operator chose to proceed).
        // 4. Open Serializable txn:
        //    4a. UPDATE gateway SET archived_at=now(), archived_reason='operator decommission' WHERE id=$1;
        //    4b. Best-effort CS DeleteGateway (fresh ctx; Pitfall 02-05 pattern).
        //    4c. audit.WriteEntry(action='decommission', entity_type='gateway', …).
        // 5. COMMIT.
    }
}
```

**Atomicity:**
- PG UPDATE first. If it fails → no CS call (best-effort cleanup not needed because nothing has changed in CS yet).
- Then CS DeleteGateway with **fresh context** (Phase 2 Pitfall 02-05). Swallow CS error and log it — the gateway is now archived in Shifter; orphan in CS will be cleaned up by the operator (or by a future reconcile job). **The operator UX is: "Shifter says it's decommissioned" wins.**

Alternative (stricter): if CS DeleteGateway fails, rollback the PG transaction (best-effort PG rollback) — but this re-introduces the half-decommissioned state the operator just tried to escape. The CONTEXT D-30 says "Atomic with best-effort ChirpStack rollback if Postgres commit fails" — that's the Phase 2 add-device pattern, where CS is created FIRST and rolled back if PG fails. For DELETE, the natural ordering is the inverse: PG first (mark archived), CS second (best-effort delete).

### Restore (D-32)

**This was an open question; the answer is:** Restore needs to re-`CreateGateway` in CS because the prior `DeleteGateway` removed it. Or — alternative — Shifter skips the CS DeleteGateway on decommission and only soft-deletes in PG.

**Recommendation:** **Soft-delete only on Shifter side; do NOT call CS DeleteGateway on decommission.** Rationale:
- The gateway is a long-lived physical asset; the operator's "decommission" rarely means "permanently retire the EUI" — more often "this gateway is offline / being relocated / temporary outage."
- A CS-side DeleteGateway loses the gateway's metrics history in CS; if the operator restores, the chart goes blank.
- The operator's actual concern (D-31 warning) is "devices behind this gateway will route via others" — that happens automatically because CS routes uplinks via whichever gateway hears them; archiving the gateway in Shifter just hides it from the operator's day-to-day view.
- If the operator truly wants to remove the gateway from CS, they can use the CS admin UI — but per UX-03/CHIRP-06, the day-to-day operator never touches CS.

**Therefore decommission = Shifter-side soft-delete only.** No CS call. Restore = `archived_at = NULL` (single UPDATE + audit row).

**This contradicts D-30's verbatim text** ("+ `DeleteGateway` in ChirpStack so the gateway stops accepting uplinks"). The contradiction is worth surfacing — see Open Questions below — but my recommendation stands: the "gateway stops accepting uplinks" rationale is wrong (CS doesn't gate uplinks per-gateway; it just records which gateway received each packet). The right model is soft-delete in Shifter, no CS DeleteGateway.

If the planner / user disagrees and wants CS DeleteGateway: restore must re-call `CreateGateway` with the original `Gateway{}` reconstructed from the Shifter row. That works but loses CS-side metrics history. **Document the decision either way.**

## Migration Numbering & SQL Patterns

**[VERIFIED: `internal/db/migrations/` directory listing]** Latest migration: `0017_binding_changed_trigger`. Next index: **0018**.

### Pattern

- File names: `NNNN_<descriptive>.up.sql` and `NNNN_<descriptive>.down.sql`. 4-digit zero-padded, conceptual one-change-per-migration.
- Top-of-file comment explaining the decision links + invariants (see `0007_site.up.sql` or `0012_device.up.sql` for the established style).
- CHECK constraints inline for length / range / case (e.g. `dev_eui ~ '^[0-9a-f]{16}$'`).
- `created_at TIMESTAMPTZ NOT NULL DEFAULT now()` + `updated_at TIMESTAMPTZ NOT NULL DEFAULT now()` + `CREATE TRIGGER touch BEFORE UPDATE ON … FOR EACH ROW EXECUTE FUNCTION touch_updated_at();` (Phase 2 convention).
- Partial indexes for soft-delete: `CREATE INDEX … ON tbl (col) WHERE archived_at IS NULL`.
- Down migration: `DROP TABLE … CASCADE; DROP TYPE …;` — keep it reversible for dev resets.

### Phase 3 migrations (proposed)

| Index | File | Tables / changes |
|-------|------|------------------|
| 0018 | `0018_gateway.up.sql` | `gateway` table + `gateway_archived_idx`, `gateway_region_idx`, `gateway_touch` trigger |
| 0019 | `0019_import_job.up.sql` | `import_job_status` + `import_job_row_status` enums; `import_job` and `import_job_row` tables; partial index `WHERE status='preview'`; touch trigger |
| 0020 | `0020_audit_log_phase3_vocab.up.sql` | `ALTER TABLE audit_log` → DROP + re-ADD CHECK constraints with the new action vocab (`bulk_import`, `reveal_secrets`) + entity_type vocab (`gateway`, `import_job`) |
| 0021 | `0021_device_indexes_phase3.up.sql` | `CREATE INDEX device_name_lower_idx ON device (lower(name))`, `CREATE INDEX device_site_last_seen_idx ON device (?)` — see Open Questions: devices don't have `site_id` yet; bindings link device→MP→site. Resolving requires either a denormalized `site_id` on `device` (cheap) or a more complex join (slow). Planner's call. |

**Open issue (Open Questions #3):** Devices are currently bound to MPs via `binding`, not directly to sites. D-12's "site (multi-select)" filter requires a join chain `device → active binding → metering_point → site`. For Phase 3 perf at 100K devices, denormalize a `current_site_id` column on device (NULLable, updated by the swap commit + new-binding-open trigger).

## Validation Architecture

**Status:** Mandatory section per Nyquist Dimension 8 — `nyquist_validation: true` in `.planning/config.json`.

### Test Framework

| Property | Value |
|----------|-------|
| Framework (Go) | `testify` (v1.11) + `testcontainers-go` (v0.42) for Postgres + Mosquitto + ChirpStack containers |
| Framework (TS) | `vitest` (v4.1) + `@testing-library/react` + `happy-dom` |
| E2E | `playwright` — **not yet installed**; planner adds in Wave 0 if e2e tests are wanted |
| Config files | `go.mod` (tests live alongside source); `web/vitest.config.ts` (already in repo) |
| Quick run command (Go) | `go test ./internal/gateway/... ./internal/imports/... -count=1 -timeout 60s` |
| Quick run command (TS) | `cd web && pnpm test:run -- --run --reporter=verbose src/routes/gateways src/routes/devices src/routes/admin/imports` |
| Full suite command | `go test ./... -count=1 -timeout 5m && cd web && pnpm test:run` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test type | Automated command | File exists? |
|--------|----------|-----------|-------------------|--------------|
| GW-01 | List/search/filter gateways with status + last_seen | unit | `go test ./internal/gateway -run TestListGateways -x` | ❌ Wave 0 |
| GW-02 | Create/edit/delete gateway via dialog with region picker | unit + integration (CS mock) | `go test ./internal/gateway -run TestCreateGateway` + `pnpm test src/routes/gateways/add-gateway-dialog` | ❌ Wave 0 |
| GW-03 | Per-gateway RX/TX stats | unit (mock GetMetrics) | `go test ./internal/gateway -run TestGatewayMetricsCache` | ❌ Wave 0 |
| GW-04 | DEFERRED — Phase 5 | n/a | n/a | n/a |
| DEV-01 | List/search/filter devices | unit (sqlc query) + integration (router) | `go test ./internal/device -run TestListDevicesFiltered` + `pnpm test src/routes/devices/index` | ❌ Wave 0 (Phase 2 ships minimal; Phase 3 extends) |
| DEV-02 | Create/edit/soft-delete device | unit | Phase 2 covered create + decommission; Phase 3 adds bulk decommission: `go test ./internal/device -run TestBulkDecommission` | ❌ Wave 0 |
| DEV-03 | DevEUI sticker parser | unit | `go test ./internal/device -run TestParseDevEUI` (already passing — reuse) | ✅ Phase 2 |
| DEV-04 | OTAA default + ABP supported | unit | `go test ./internal/device -run TestAddDevice_OTAA TestAddDevice_ABP` | ❌ Wave 0 |
| DEV-05 | Device profile mgmt + custom codec | n/a | Already covered in Phase 2 (`internal/profile/`) | ✅ Phase 2 |
| DEV-06 | Bulk import dry-run + commit | integration | `go test ./internal/imports -run TestImportXLSX_DryRun TestImportXLSX_Commit` | ❌ Wave 0 |
| DEV-07 | Re-run idempotent | integration | `go test ./internal/imports -run TestImportXLSX_Idempotent` | ❌ Wave 0 |
| DEV-08 | Downloadable template | unit | `go test ./internal/imports -run TestGenerateTemplate` | ❌ Wave 0 |
| DEV-09 | Viewer cannot see secrets (server-side) | unit (authz) + integration (HTTP) | `go test ./internal/device -run TestRevealSecrets_ViewerForbidden TestRevealSecrets_AdminOK` | ❌ Wave 0 |
| CHIRP-05 | Multi-step flows collapsed | manual-only | UI walkthrough — no automated assertion |  |
| CHIRP-06 | Operator never logs into CS | manual-only | Code review: no CS UI links in Shifter |  |
| UX-03 | No "tenant"/"application" copy | static analysis | grep -rE '(tenant\|application)' web/src — UI-SPEC checker | ✅ pattern from Phase 2 |

### Sampling Rate

- **Per task commit:** `go test ./internal/<package> -count=1` + `cd web && pnpm test:run -- src/<changed-file>`
- **Per wave merge:** `go test ./... -count=1` + `cd web && pnpm test:run`
- **Phase gate:** Full suite green before `/gsd-verify-work`; integration tests (testcontainers Mosquitto + Postgres + bufconn-mock CS) MUST pass

### Wave 0 Gaps

Test infrastructure to create before implementation tasks start:

- [ ] `internal/gateway/handlers_test.go` — covers GW-01, GW-02, GW-03 happy paths
- [ ] `internal/gateway/metrics_cache_test.go` — covers D-02 1-min TTL + single-flight
- [ ] `internal/imports/parse_test.go` — covers excelize XLSX read + CSV BOM-strip + non-UTF-8 reject
- [ ] `internal/imports/dryrun_test.go` — covers all 6 validation rules (EUI format, intra-file dup, pre-existing, site lookup, profile lookup, required fields)
- [ ] `internal/imports/commit_test.go` — covers per-row outcome, idempotent re-run, partial commit (one row fails, others succeed)
- [ ] `internal/imports/template_test.go` — covers GenerateTemplate output structure (headers, data validation drop-downs, text-formatted DevEUI column)
- [ ] `internal/device/reveal_test.go` — covers admin OK / viewer 403 / OTAA flow / ABP flow / CS not-found → 502
- [ ] `internal/chirpstack/gateway_test.go` — covers new GatewayService wrapper with bufconn mock
- [ ] `internal/chirpstack/device_activate_test.go` — covers new ActivateDevice wrapper
- [ ] `web/src/routes/gateways/index.test.tsx` — covers list page render + sparkline cell
- [ ] `web/src/routes/gateways/add-gateway-dialog.test.tsx` — covers field validation + submit
- [ ] `web/src/routes/devices/bulk-import-dialog.test.tsx` — covers 3-step import flow with mocked fetch
- [ ] `web/src/routes/admin/imports/$jobId.test.tsx` — covers job detail render + outcome table

**Integration test scaffolding:**
- [ ] Reuse Phase 2 `internal/testharness/` Mosquitto+PG container fixture; add a bufconn-mock ChirpStack server fixture (or upgrade to a real ChirpStack container if Phase 7 wants it — defer for now)

## Security Domain

`.planning/config.json` doesn't have `security_enforcement` set → treat as enabled.

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | yes (existing) | `alexedwards/scs/v2` cookie sessions (Phase 1) — Phase 3 doesn't change auth surface |
| V3 Session Management | yes (existing) | SCS cookie + Argon2id — Phase 3 reuses |
| V4 Access Control | **yes (Phase 3 adds 7 new actions)** | `auth.Can(user, action, resource)` matrix in `internal/auth/authz.go` — new actions: `gateway.create`, `gateway.update`, `gateway.archive`, `gateway.restore`, `device.bulk_import`, `device.reveal_secrets`. Admin-only; viewer denied via fail-closed default |
| V5 Input Validation | yes | `zod` on frontend; Go server-side validation: `isHex(s,n)` + length checks + UUID parse + range CHECK constraints in PG |
| V6 Cryptography | yes | LoRaWAN AppKey + NwkSKey + AppSKey are AES-128 keys — Shifter **never** persists them (D-22/D-27); ChirpStack stores them. No hand-rolling. |
| V7 Errors & Logging | yes | Audit log every state change (D-33/D-28) + structured slog. Reveal logs ONLY mode + dev_eui — no secret material |
| V8 Data Protection | yes | Sensitive secrets remain in ChirpStack — Shifter has no key columns. **Structural** enforcement is the strongest form |
| V12 Files | yes | XLSX upload — `multipart/form-data` with size cap (1MB plenty for 10K rows); whitelist content-type; never execute file content |

### Known Threat Patterns for Phase 3 stack

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Viewer enumerates `POST /api/devices/:eui/keys` to harvest keys | Information Disclosure | `RequireAction(sm, ActionDeviceRevealSecrets)` returns 403; fail-closed default in `Can()` |
| SQL injection via filter query params | Tampering | sqlc-generated parameterized queries — every Phase 3 query goes through sqlc, never raw concat |
| XLSX-based zip-bomb / billion-laughs DoS | DoS | `excelize/v2` doesn't expand XML entities; nonetheless, gate uploads at 5MB max via http.MaxBytesReader |
| CSV / XLSX cell that mid-imports unauthorized site_id from another tenant | Tampering (cross-row injection) | sqlc query `SELECT id FROM site WHERE id = $1 AND archived_at IS NULL` — single-tenant install means site_id collision is non-issue, but the WHERE clause is still defense-in-depth |
| Bulk-import audit log skipped to hide bulk-create | Repudiation | `audit.WriteEntry` is in the same Serializable txn as the device INSERT — atomicity guarantees both or neither |
| Reveal-endpoint replay (operator captures a reveal HTTP response, replays it later) | Tampering | Audit log captures every reveal; if abuse is observed, add rate limit (D-Discretion #5) — out of v1 threat model |
| Gateway create with operator-controlled `tenant_id` (cross-tenant escape) | Elevation of Privilege | Backend ignores any client-supplied `tenant_id`; always uses `chirpstack_connection.tenant_id` server-side (single-global tenant per Phase 2 D-28) |
| Stale `import_job` previews holding row-locks | DoS / RC | `import_job_row` is INSERT-only during dry-run; no row locks held outside the per-row commit txn. Expired previews lazily reaped on read |
| Operator pastes secrets into description / tags fields | Information Disclosure | Frontend zod validation rejects 32-hex-only strings in description; backend `device.description` accepts any text but is shown to viewers (defense: educate via tooltip — "Do not paste keys here") |
| Container running as root (image risk) | Privilege Escalation | Inherited from Phase 1 install kit — Dockerfile already uses non-root user |

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | The CONTEXT mention of "TanStack Router search params" (D-15) is informal; the project actually uses `react-router-dom v7` (verified via `web/package.json` + `web/src/App.tsx` + UI-SPEC line 62) | §react-router-dom v7 URL State | **Low**: re-affirmed by 3 independent sources. If wrong, planner adopts TanStack Router as a new dep — but this is unlikely given the UI-SPEC explicitly names react-router-dom v7. |
| A2 | The CONTEXT mention of "GetGatewayStats" (D-02) is a naming convention error; the actual v4.17 RPC is `GetMetrics` (verified via local proto sources + grpc_pb file at line 29) | §ChirpStack v4 GatewayService | **Low**: verified at proto level; the rename is documented in the wrapper's GoDoc, no semantic impact |
| A3 | Decommissioning a gateway SHOULD NOT call CS DeleteGateway (recommended pure soft-delete) — this **contradicts D-30 verbatim text** but I argue D-30's rationale ("so the gateway stops accepting uplinks") is incorrect at the CS protocol level | §Gateway Decommission + Restore | **Medium**: needs user confirmation in next phase discussion. If user insists on the CS-side delete, Restore must re-CreateGateway and CS metrics history is lost — both flows are implementable, just different UX. |
| A4 | Phase 3 commit-phase processes bulk-import synchronously in the HTTP handler — no River queue required (CLAUDE.md documents River as recommended but `go.mod` doesn't have it; D-Discretion #6 says "switch to River if benchmarks show timeouts") | §Two-Phase Import Job — Background processing decision | **Low**: only applies to >10K-row imports; v1 customers target 50–500 meters per CONTEXT goal |
| A5 | `internal/install/Regions()` is the canonical region catalog reused by gateway create dialog — no separate gateway-region catalog | §ChirpStack v4 GatewayService — Gotchas | **Low**: verified via `internal/install/regions.go` lines 27-45 — list is intentionally hardcoded with `DefaultForCountry` semantics that gateway dialog inherits |
| A6 | Excelize/v2 v2.10.0 is the right pin (released Feb 2026 per WebSearch); planner verifies via `go list -m -u github.com/xuri/excelize/v2` at planning time | §Excelize/v2 XLSX Import | **Low**: CLAUDE.md already approves excelize/v2 v2.10+; major API hasn't changed since v2.6 |
| A7 | Devices needing a denormalised `current_site_id` column for the D-12 site multi-select filter at 100K-device scale (binding-chain join is otherwise slow). **Schema decision needs planner confirmation** — alternative is a `WITH active_bindings AS (...)` CTE or materialized view | §Migration Numbering — 0021 | **Medium**: at <10K devices the binding-join is fine; at 100K it isn't. Planner can defer the index until benchmarks demand it |
| A8 | Phase 3 reuses Phase 2's `ParseDevEUI` for gateway IDs (same EUI64 shape, same paste-sticker UX) — rename `internal/device/deveui.go::ParseDevEUI` to `ParseEUI64` and keep `ParseDevEUI` as a deprecated alias | §DevEUI / Key Validation — Sticker-paste support | **Low**: simple refactor, no functional change |
| A9 | The reveal endpoint detects activation mode by trying `GetKeys` first (OTAA), then falling back to `GetActivation` (ABP). Some CS deployments may have both populated for the same device — unlikely in practice but the handler must prefer the most recent populated one. Phase 3 picks OTAA-first as the consistent default | §device.reveal_secrets Endpoint | **Low**: CS data model rarely has both; OTAA-first is fine |
| A10 | `import_job_row.raw_payload JSONB` stores the original column → string map (lossless for errors.xlsx round-trip). Excel numeric cells get cast to string at parse time so they round-trip exactly | §Two-Phase Import Job | **Low**: requires the XLSX reader to use `GetRows` (string-typed) not `GetCellValue` (typed) for the raw_payload column |
| A11 | New `audit_log` action values (`bulk_import`, `reveal_secrets`) + entity_type values (`gateway`, `import_job`) require a Phase 3 vocabulary-extension migration (0020). The existing `audit_log_action_valid` and `audit_log_entity_type_valid` CHECK constraints lock the vocab — extending them is a one-line DROP+ADD | §Audit Envelope — vocabulary extension | **Low**: pattern is standard; Phase 2 already locked the vocab so Phase 3 must extend |

## Open Questions / Risks

1. **Gateway decommission semantics — CS-side delete or Shifter-only soft-delete?**
   - What we know: D-30 explicitly says "soft-delete in Postgres + DeleteGateway in ChirpStack". Recommended Phase 3 pattern argues for Shifter-only soft-delete (see Assumption A3).
   - What's unclear: whether the user / planner agrees with the rationale, or wants to keep CS-side delete (which then requires Restore to re-CreateGateway and loses CS metrics history).
   - Recommendation: **Flag this in `/gsd-discuss-phase` follow-up** before locking the implementation pattern.

2. **`current_site_id` denormalisation on `device`?**
   - What we know: D-12 requires server-side site multi-select filter; devices currently link to sites only via `binding → metering_point → site` chain.
   - What's unclear: scale threshold — at 1K devices the join is fine; at 100K it isn't. Planner's call.
   - Recommendation: **Ship without denormalisation in Phase 3**; add `0021_device_current_site_id.up.sql` migration if benchmarks during Phase 4 dashboard work show slow filtering.

3. **Synchronous commit vs SSE-progress vs River background job (D-Discretion #6)**
   - What we know: 500-row commit takes ~75s synchronously (gRPC dominates); chi `WriteTimeout` must be ≥120s.
   - What's unclear: whether the operator UX needs live progress for 500-row imports, or can wait synchronously.
   - Recommendation: **Synchronous for Phase 3**; chi WriteTimeout bumped to 180s for the import-commit route only (via per-route timeout middleware). If operator feedback wants progress, add SSE in Phase 4 alongside the realtime work.

4. **`gateway.region` semantics — Shifter-only or pushed to CS?**
   - What we know: ChirpStack v4 does not have a `region` field on the `Gateway` proto — region lives on `device_profile`. The Phase 3 D-03 "regulator-aware region picker" is for the operator's visual model of the gateway, not a CS-syncable attribute.
   - What's unclear: whether the operator expects the region pick to mean anything to CS, or whether it's purely Shifter-side metadata for the picker UI.
   - Recommendation: **Shifter-only column** with a tooltip explaining "Region is set per device profile in ChirpStack — this picker is for your reference."

5. **`current_site_id` denormalisation OR a `device_site_view` materialized view?**
   - See #2. A `MATERIALIZED VIEW device_with_site AS SELECT d.*, mp.site_id FROM device d LEFT JOIN binding b … LEFT JOIN metering_point mp …` with `REFRESH MATERIALIZED VIEW CONCURRENTLY` on swap commit is a cleaner alternative — surface to planner.

6. **Tags JSONB vs TEXT[] in `gateway` and import schema?**
   - D-04 says CSV "tags (comma-separated or JSON)" — the parser accepts both. PG storage shape is one decision: JSONB allows arbitrary key-value (CS native shape) or TEXT[] is simpler for the "flat list of tags" UX.
   - Recommendation: **JSONB** matching CS `Gateway.Tags map[string]string` — supports future "tag with value" patterns without a migration.

## Environment Availability

> Phase 3 has no new external runtime dependencies. All required tools are inherited from Phases 1–2.

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | All backend | ✓ | 1.25.0 (`go.mod`) | — |
| pnpm | Frontend | ✓ | 10.33.2 (`web/package.json`) | — |
| Node | Frontend | ✓ | ≥22.12 (per engines) | — |
| PostgreSQL + TimescaleDB | All data | ✓ | Phase 1 bundled/external compose | — |
| Mosquitto | MQTT subscriber (no new use in Phase 3) | ✓ | Phase 1 bundled | — |
| ChirpStack v4 | gateway + device gRPC | ✓ | v4.17.0 proto (`chirpstack-api/go/v4`) | — |
| Docker / Docker Compose | Local dev + testcontainers | ✓ | Phase 1 install kit | — |
| testcontainers-go | Integration tests | ✓ | v0.42 (`go.mod`) | — |
| `golang.org/x/sync` (singleflight) | Metrics cache deduplication | ✗ | — | Pure in-process `sync.Map` guard (simple; no new dep) |
| `github.com/xuri/excelize/v2` | XLSX read/write | ✗ | — | **No fallback — required for D-04a (XLSX primary)**. Adds ~2MB to binary; pure Go, no CGo. |

**Missing with no fallback:**
- `excelize/v2` — first install in Phase 3, planned addition per CONTEXT code_context section.

**Missing with fallback:**
- `golang.org/x/sync/singleflight` — fallback to inline `sync.Map` guard.

## Recommended Wave Breakdown (preliminary — planner may revise)

A first-pass wave decomposition for Phase 3. Each wave is a contiguous chunk of work that ends green / mergeable. Planner has authority to revise.

**Wave 0 — Test Scaffolding + Dep Additions**
- Add `github.com/xuri/excelize/v2` to `go.mod`; verify version pinned.
- Add skeleton test files (table above) — empty test functions that compile.
- Add new `auth.Action` constants to `internal/auth/authz.go` + admin-bundle entries; viewer-denied test for each new action.
- Migration `0020_audit_log_phase3_vocab.up.sql` to extend audit_log CHECK vocabularies.

**Wave 1 — Gateway Backend**
- `internal/chirpstack/gateway.go` — `CreateGateway`, `GetGateway`, `UpdateGateway`, `DeleteGateway`, `ListGateways`, `GetGatewayMetrics` wrappers + tests against bufconn mock.
- Migration `0018_gateway.up.sql`.
- `internal/db/queries/gateway.sql` + sqlc generation.
- `internal/gateway/handlers.go` — CRUD HTTP handlers + metrics-cache refresh goroutine + single-flight guard.
- Atomic CS+PG transaction wrapper mirroring Phase 2 `internal/device/handlers.go::addDevice`.
- All gateway-related actions added to authz; admin-only.

**Wave 2 — Devices List Filter / Sort / Pagination**
- Extend `internal/db/queries/device.sql` with parameterised filter/sort/page query.
- Extend `internal/device/handlers.go` listDevices with site / status / last_seen / q / page / per_page / sort.
- Add `current_site_id` denormalised column if planner approves (otherwise CTE-based join — see Open Q #2).

**Wave 3 — OTAA/ABP Add-Device Dialog Extension**
- `internal/chirpstack/device.go` — add `ActivateDevice` wrapper for ABP path.
- Extend `internal/device/handlers.go::addDevice` to branch on activation_mode (OTAA = existing path; ABP = CreateDevice + ActivateDevice).
- Frontend `web/src/routes/devices/add-device-dialog.tsx` — 5-step refactor with activation radio + mode-specific step 4.

**Wave 4 — Reveal-Secrets Endpoint + Frontend**
- `internal/chirpstack/device.go` — add `GetDeviceKeys`, `GetDeviceActivation` wrappers.
- `internal/device/handlers.go::revealSecrets` handler + RBAC + audit row.
- Frontend `web/src/routes/devices/$id.tsx` — device detail page with admin-only RevealKeysButton.

**Wave 5 — Bulk Import Backend**
- Migration `0019_import_job.up.sql` (enums + tables + indexes).
- `internal/imports/` package — `parser.go` (XLSX + CSV), `validate.go` (dry-run rules), `commit.go` (per-row commit + audit envelope), `template.go` (DEV-08 generator), `errors_xlsx.go` (D-07 errors export).
- HTTP handlers: `POST /api/imports` (upload + dry-run), `POST /api/imports/:job_id/commit`, `GET /api/imports/:job_id`, `GET /api/imports/:job_id/errors.xlsx`, `GET /api/imports/template.xlsx`.

**Wave 6 — Bulk Import Frontend + Imports Admin Pages**
- `web/src/routes/devices/bulk-import-dialog.tsx` — 3-step stepped dialog (upload → preview → commit).
- `web/src/routes/admin/imports/index.tsx` — imports list page.
- `web/src/routes/admin/imports/$jobId.tsx` — single-job detail page (D-36) with per-row outcome table + download-errors button.
- App.tsx route registration: `/devices/import` (or modal-launched from `/devices`), `/admin/imports`, `/admin/imports/:jobId`.

**Wave 7 — Gateway Frontend**
- `web/src/routes/gateways/index.tsx` — list page with sparkline cell (shadcn-chart added via `pnpm dlx shadcn@latest add chart`).
- `web/src/routes/gateways/$id.tsx` — detail page.
- `web/src/routes/gateways/add-gateway-dialog.tsx` — single ResponsiveDialog (D-29).
- `web/src/routes/gateways/decommission-gateway-dialog.tsx` — AlertDialog with 24h-warning banner.

**Wave 8 — Bulk Decommission + Filter URL-State + Final Wiring**
- `internal/device/handlers.go::bulkDecommissionDevices` handler.
- Frontend `web/src/routes/devices/index.tsx` — row-selection + bulk-decommission button + filter URL-state via react-router-dom v7.
- UX-03 vocabulary audit pass (grep for "tenant" / "application" across web/src).

**Wave 9 — Phase Validation & REQUIREMENTS Reconciliation**
- Run full test suite green.
- Update `REQUIREMENTS.md` traceability — mark GW-01, GW-02, GW-03, DEV-01..09, CHIRP-05, CHIRP-06, UX-03 as Complete with shipping evidence; GW-04 stays deferred to Phase 5.
- Update Phase 3 `VALIDATION.md` if exists; emit `/gsd-verify-work` artifacts.

## Sources

### Primary (HIGH confidence)
- Local proto sources at `/Users/suraboonsung/Documents/Programming/chirpstack-api/chirpstack-api-cli/protos/chirpstack/api/go/` — `gateway.pb.go`, `gateway_grpc.pb.go`, `device.pb.go`, `device_grpc.pb.go`, `common/common.pb.go` — verified exact method names, request/response struct fields, enum values for `GatewayService`, `DeviceService`, `Aggregation`, `Metric`, `Location`, `DeviceKeys`, `DeviceActivation`.
- `internal/chirpstack/client.go`, `tenant.go`, `application.go`, `device.go` — auth interceptor pattern, atomic CS+PG transaction shape, best-effort CS cleanup with fresh context (Pitfall 02-05).
- `internal/db/migrations/0007_site.up.sql` … `0017_binding_changed_trigger.up.sql` — migration naming, file shape, touch trigger pattern, CHECK constraint conventions, partial-index pattern for soft delete.
- `internal/audit/log.go`, `0016_audit_log.up.sql` — audit_log column shape, write-in-same-tx contract, action / entity_type CHECK vocabulary.
- `internal/auth/authz.go` — `Can(user, action, resource)` matrix, `RequireAction` middleware, fail-closed defaults.
- `internal/device/handlers.go`, `deveui.go` — atomic Add Device flow, Pitfall 02-05 best-effort cleanup, ParseDevEUI normalization rules.
- `internal/install/regions.go` — LoRaWAN region catalog including AS923-2 Thailand default.
- `.planning/phases/01-foundation/01-CONTEXT.md`, `02-domain-model-canonical-schema/02-CONTEXT.md` — inherited stepped-dialog, atomic CS+PG, audit_log shape, decommission UX, "Pick on map disabled" pattern.
- `.planning/phases/03-provisioning-gateways-devices-bulk-import/03-UI-SPEC.md` — confirms `react-router-dom v7`, shadcn-chart for sparkline, dialog anatomy.
- `.planning/research/PITFALLS.md` §§ 8, 12, 13, 14 — AS923 regulator, two-phase bulk import, OTAA endianness, Can(action,resource) granularity.

### Secondary (MEDIUM confidence)
- `web/package.json` — confirms TanStack React Query v5, TanStack React Table v8.21.3, react-router-dom v7, zod v4.
- `web/src/App.tsx` — confirms react-router-dom v7 `createBrowserRouter` usage; no TanStack Router.
- `go.mod` — confirms `chirpstack-api/go/v4 v4.17.0`, `pgx/v5 v5.9.2`, `chi/v5 v5.2.5`, `scs/v2 v2.9.0`, Go 1.25.0; **no excelize, no River yet**.
- WebSearch — `excelize/v2 v2.10.x` released Feb 2026, supports `AddDataValidation` + `NewDataValidation` + `SetDropList`. ([excelize package - github.com/xuri/excelize/v2 - Go Packages](https://pkg.go.dev/github.com/xuri/excelize/v2))
- `.planning/research/SUMMARY.md` §Phase 3 — gateway list, RX/TX strategy, bulk CSV import patterns.

### Tertiary (LOW confidence)
- Bulk-import sync-vs-async decision (D-Discretion #6) — sized from 500 rows × ~150ms gRPC = 75s, but per-CS-deployment latency varies; planner should benchmark before locking the sync path for >500 rows.

## Metadata

**Confidence breakdown:**
- ChirpStack v4 GatewayService surface: HIGH — verified against local proto sources at v4.17.0
- Excelize/v2 API: HIGH — verified via pkg.go.dev + Feb 2026 release notes
- TanStack Router vs react-router-dom v7: HIGH — verified by 3 independent sources (UI-SPEC, App.tsx, package.json)
- GetMetrics rename (vs CONTEXT's GetGatewayStats): HIGH — verified at proto level
- Schema sketches: HIGH for shape; MEDIUM for exact index choices (planner should benchmark)
- Decommission semantics (CS-side delete or not): MEDIUM — recommendation contradicts D-30 verbatim; needs user/planner confirmation

**Research date:** 2026-05-11
**Valid until:** 2026-08-11 (90 days; ChirpStack v4 minor versions ship roughly quarterly so re-verify if the project bumps `chirpstack-api/go/v4`).
