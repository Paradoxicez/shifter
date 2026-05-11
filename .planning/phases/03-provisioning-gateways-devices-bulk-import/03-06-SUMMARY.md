---
phase: 03-provisioning-gateways-devices-bulk-import
plan: 06
subsystem: api
tags: [devices, list-pagination, bulk-decommission, reveal-secrets, sqlc, chirpstack, audit, dev-09]

requires:
  - phase: 02-domain-model-canonical-schema
    provides: "audit_log shape, atomic CS+PG pattern, DEV-09 no-Shifter-column invariant, decommission semantics"
  - phase: 03-provisioning-gateways-devices-bulk-import
    provides: "03-02 ActionDeviceRevealSecrets + audit vocabulary, 03-03 GetDeviceKeys/GetDeviceActivation wrappers"
provides:
  - "GET /api/devices server-side filter (site[], status, last_seen, q) + sort + offset pagination"
  - "POST /api/devices/bulk-decommission with per-row atomic CS+PG + partial-success report (max 200)"
  - "POST /api/devices/{eui}/keys admin-only reveal returning OTAA or ABP shape; audit row with NO key material"
  - "Two new sqlc queries (ListDevicesFiltered + CountDevicesFiltered) with site-via-active-binding join"
affects: [03-09 devices list page, 03-10 device detail page reveal button, 06 audit-query UI]

tech-stack:
  added: []
  patterns:
    - "Per-row Serializable tx for bulk operations (failure of one row does NOT roll back siblings)"
    - "CSDeviceRevealer narrow interface (type-assert from CSDeviceClient) for reveal-only CS surface"
    - "Static defense-in-depth test that parses Go source + asserts forbidden tokens are absent"
    - "no-Shifter-column + endpoint-side authz: list response is structurally key-free; reveal is the only path"

key-files:
  created:
    - "internal/device/reveal.go — revealSecrets handler + writeAuditReveal + CSDeviceRevealer interface"
    - "internal/device/reveal_test.go — 10 cases (9 behavioral + 1 static cleanliness check)"
    - "internal/device/list_filtered_test.go — 11 cases for filter/sort/page + DEV-09 invariant"
    - "internal/device/bulk_decommission_test.go — 4 cases (partial / atomic / cap / viewer)"
  modified:
    - "internal/db/queries/devices.sql — appended ListDevicesFiltered + CountDevicesFiltered"
    - "internal/db/sqlc/devices.sql.go, internal/db/sqlc/querier.go — sqlc regen"
    - "internal/device/handlers.go — replaced listDevices, added bulkDecommissionDevices + bulkDecommissionOne, route mounts"
    - "internal/http/router.go — cross-reference comment for the new device routes"
    - "internal/api/devices_{list,bulk_decommission,reveal}_test.go — markers pointing to canonical tests"

key-decisions:
  - "Plan called the device soft-delete column 'archived_at' but the schema (migration 0012) uses decommissioned_at. Queries adapted accordingly — the device semantic is identical, only the column name differs."
  - "Reveal CS surface modeled as a narrow CSDeviceRevealer interface (GetDeviceKeys + GetDeviceActivation), type-asserted from deps.CS. Keeps the write-path interface (CSDeviceClient) free of GetKeys responsibility and prevents the reveal flow from accidentally calling CreateDevice."
  - "writeAuditReveal runs in its OWN short-lived tx (not the request tx, which doesn't exist for a read-only reveal). On audit-write failure the reveal response still succeeds — we do NOT deny an operator a key reveal because audit_log had a transient hiccup; failures are warning-logged."
  - "bulkDecommissionDevices accepts an optional `reason` (audit notes prefix). Body schema { device_ids: [string], reason: string }. Each id is a Shifter UUID (matches the single-device decommission path keyed by /api/devices/{id}/decommission)."
  - "TestRevealSecrets_AuditWriteSiteCleanliness is a static source-parse test that strips comments and asserts none of AppKey/NwkKey/AppSKey/NwkSKey appear in writeAuditReveal's code body. Runs in -short, no testcontainer needed, future-proofs DEV-09 / T-3-51 against accidental regression."

patterns-established:
  - "Per-row Serializable tx for bulk endpoints — partial-success report with {succeeded, failed, outcomes[]}"
  - "no-Shifter-column structural DEV-09 enforcement extended: list projection JSON has zero key fields; reveal is the only path that surfaces secrets"
  - "Static defense-in-depth grep-as-test for invariants that span ‘this function MUST NOT mention this thing’"

requirements-completed: [DEV-01, DEV-02, DEV-09, UX-03]

duration: 22 min
completed: 2026-05-11
---

# Phase 3 Plan 06: Devices list filter/sort/pagination + bulk-decommission + reveal endpoint Summary

**Server-side `/api/devices` with full filter/sort/page surface, atomic-per-row bulk decommission, and admin-only reveal endpoint that surfaces OTAA or ABP secrets without ever writing key material to the audit row.**

## Performance

- **Duration:** ~22 min
- **Tasks:** 3
- **Files modified:** 6
- **Files created:** 4

## Accomplishments

- **Devices list (D-12..D-18 / DEV-01):** Phase 2's minimal `LIMIT 100 OFFSET 0` replaced with a full server-side filter/sort/paginate handler. Filter contract = site multi-select, activation status (active/inactive/never_joined), last-seen window (24h/7d/30d/all), text search on name OR dev_eui. Sort = name/dev_eui/site/last_seen/created_at, asc or desc via '-' prefix. Pagination = page+per_page in {25,50,100}, default per_page=50. Response envelope `{total_count, page_count, page, per_page, rows[]}`. Site is resolved via the active-binding chain (no `current_site_id` denormalisation per RESEARCH Open Q #2).
- **Bulk decommission (D-17 / DEV-02):** `POST /api/devices/bulk-decommission` accepts up to 200 device_ids per request (T-3-56 DoS bound) and runs each device in its OWN Serializable tx so partial success is honest. CS DeleteDevice is best-effort on a FRESH context (Pitfall 02-05) and CS-not-found is folded into success for idempotency. Each successful row writes an audit row with `notes='bulk decommission: <reason>'`; the chi request_id groups all rows of a single bulk call.
- **Reveal endpoint (D-22 / D-26 / D-27 / D-28 / DEV-09):** `POST /api/devices/{eui}/keys` is admin-only via `RequireAction(ActionDeviceRevealSecrets)`. OTAA-first fallback to ABP. Audit row contains ONLY `{activation_mode, dev_eui}` — no key bytes ever. Cache-Control: no-store + Pragma: no-cache headers prevent browser/proxy caching of revealed material (T-3-53). 7 distinct error codes (400/401/403/404/409/502/200) distinguish device-existence vs CS-unreachable vs no-credentials.
- **DEV-09 structural invariant defended at three layers:**
  - schema: device table has no key columns (Phase 2 — unchanged)
  - JSON projection: `filteredDevicesToJSON` lists every field explicitly; TestListDevices_DEV09_NoKeysInRows asserts the list response has zero `app_key/nwk_key/app_s_key/nwk_s_key` keys
  - audit: TestRevealSecrets_AuditNoSecretMaterial reads `audit_log.after` JSONB and asserts no AppKey/NwkKey/AppSKey/NwkSKey hex appears for any reveal row + TestRevealSecrets_AuditWriteSiteCleanliness statically asserts those tokens never appear in writeAuditReveal's code body.

## Task Commits

1. **Task 1: sqlc ListDevicesFiltered + CountDevicesFiltered** — `c06461c` (feat)
2. **Task 2: devices list filter/sort/page + bulk-decommission** — `1ef3cf6` (feat)
3. **Task 3: reveal endpoint POST /api/devices/{eui}/keys** — `b921a17` (feat)

## Files Created/Modified

### Created

- `internal/device/reveal.go` — `revealSecrets` handler, `writeAuditReveal` audit writer, `CSDeviceRevealer` narrow interface, `uuidFromPgUUID` helper. ~210 lines including extensive doc comment that lays out the surface contract + DEV-09 invariants.
- `internal/device/reveal_test.go` — 10 test cases covering all 9 behaviors from the plan PLUS a static cleanliness test that parses reveal.go and asserts the writeAuditReveal body never references AppKey/NwkKey/AppSKey/NwkSKey.
- `internal/device/list_filtered_test.go` — 11 cases (site single, site multi, each status branch, last_seen 24h, q name + q dev_eui substrings, sort asc/desc, pagination across 75 devices, max per_page rejection, defaults applied, viewer reads OK, DEV-09 no-keys-in-rows assertion).
- `internal/device/bulk_decommission_test.go` — 4 cases (partial success with one missing id, atomic-per-row sibling continuation after one failure, 201-id 400 rejection, viewer 403).

### Modified

- `internal/db/queries/devices.sql` — Appended two new queries; the existing Phase 2 queries are untouched so the swap-commit path and Phase 2 device tests continue to use `ListActiveDevices` / `SearchDevices` unchanged.
- `internal/db/sqlc/devices.sql.go`, `internal/db/sqlc/querier.go` — sqlc regeneration (committed source per CLAUDE.md / Plan 01-03 convention).
- `internal/device/handlers.go` — `listDevices` body replaced with the filtered handler (envelope JSON shape changes from bare array to `{total_count, …, rows}`); `parseListDevicesParams` + `parseSortParam` helpers + `filteredDevicesToJSON` projection added; `bulkDecommissionDevices` + `bulkDecommissionOne` added; reveal route mounted via the new admin-only group.
- `internal/http/router.go` — cross-reference comment listing the three new device routes for discoverability when reading the router only.
- `internal/api/devices_{list,bulk_decommission,reveal}_test.go` — Wave-0 `t.Skip` placeholders replaced with pointer markers naming the canonical tests in `internal/device/`. CI continues to acknowledge the 03-VALIDATION row names.

## sqlc Query Contracts

```sql
-- ListDevicesFiltered :many
-- ($1 site_ids UUID[], $2 status TEXT, $3 last_seen_cutoff TIMESTAMPTZ,
--  $4 q TEXT, $5 sort_col TEXT, $6 sort_desc BOOL,
--  $7 limit INT, $8 offset INT)
-- Returns: device.* + current_site_id + current_site_name (NULL when no active binding)

-- CountDevicesFiltered :one
-- Same WHERE clause; returns COUNT(*) for total_count
```

## HTTP Endpoint Contracts

### `GET /api/devices`

Query params (all optional):

| Param | Allowed | Default |
|---|---|---|
| `site` (repeatable) | UUID | (no filter) |
| `status` | `active` / `inactive` / `never_joined` | (no filter) |
| `last_seen` | `24h` / `7d` / `30d` / `all` | (no filter) |
| `q` | substring | (no filter) |
| `sort` | `name` / `dev_eui` / `site` / `last_seen` / `created_at` (prefix `-` for desc) | `-last_seen` |
| `page` | int ≥ 1 | `1` |
| `per_page` | `25` / `50` / `100` | `50` |

Response 200 envelope:
```json
{
  "total_count": 75,
  "page_count": 3,
  "page": 2,
  "per_page": 25,
  "rows": [
    { "id": "...", "dev_eui": "...", "name": "...", "device_profile_id": "...",
      "join_eui": "...", "description": "...", "last_seen_at": "...",
      "decommissioned_at": null, "created_at": "...", "updated_at": "...",
      "current_site_id": "..." | null, "current_site_name": "..." | null }
  ]
}
```

400 codes: `invalid_per_page`, `invalid_page`, `invalid_sort`, `invalid_status`, `invalid_last_seen`, `invalid_site_id`.

### `POST /api/devices/bulk-decommission`

Body: `{ "device_ids": ["uuid", ...], "reason": "..." }`. Max 200 ids.

Response 200:
```json
{
  "succeeded": 2,
  "failed": 1,
  "outcomes": [
    { "id": "...", "status": "decommissioned" },
    { "id": "...", "status": "failed", "reason": "not_found" }
  ]
}
```

400 codes: `bad_request`, `no_device_ids`, `too_many_devices`. 403 viewer (`forbidden`).

### `POST /api/devices/{eui}/keys`

Admin-only via `RequireAction(ActionDeviceRevealSecrets)`.

| Status | Body | When |
|---|---|---|
| 200 OTAA | `{ activation_mode:"OTAA", dev_eui, join_eui, app_key, nwk_key }` | CS GetDeviceKeys success |
| 200 ABP | `{ activation_mode:"ABP",  dev_eui, dev_addr, nwk_s_key, app_s_key, f_cnt_up, n_f_cnt_down, a_f_cnt_down }` | CS GetDeviceKeys ErrNotFound → GetDeviceActivation success |
| 400 | `{ error:"invalid_dev_eui" }` | :eui is not 16-lowercase-hex |
| 401 | `{ error:"unauthorized" }` | No session (RequireAction) |
| 403 | `{ error:"forbidden" }` | Viewer (RequireAction; T-3-50) |
| 404 | `{ error:"not_found" }` | Device row absent in Shifter PG |
| 409 | `{ error:"no_credentials_in_cs" }` | Both CS GetKeys + GetActivation NotFound |
| 502 | `{ error:"cs_get_keys_failed", detail }` | Non-NotFound CS error on GetKeys |
| 502 | `{ error:"cs_get_activation_failed", detail }` | Non-NotFound CS error on GetActivation |
| 503 | `{ error:"cs_unavailable" }` | deps.CS does not satisfy CSDeviceRevealer (mis-wiring) |

All 200/40x/50x responses on this route emit `Cache-Control: no-store, no-cache, must-revalidate` + `Pragma: no-cache` (T-3-53).

## DEV-09 Invariant Proof

```bash
$ grep -E "app_key|nwk_s_key|app_s_key|nwk_key" internal/device/handlers.go
# (empty) — list/detail handlers never reference key field names

$ grep -E "app_key|nwk_s_key|app_s_key|nwk_key" internal/device/reveal.go
# Occurrences ONLY inside the 200 success-response map literals — the JSON
# response body that the caller explicitly asked for. NEVER inside any DB
# write-path (only audit.WriteEntry call is in writeAuditReveal which the
# static test TestRevealSecrets_AuditWriteSiteCleanliness asserts is
# key-name-free).
```

The audit row shape for a reveal call:

```json
{ "activation_mode": "OTAA" | "ABP", "dev_eui": "0102030405060708" }
```

Pinned by `TestRevealSecrets_AdminOTAA` (`require.Len(t, after, 2)`) and `TestRevealSecrets_AuditNoSecretMaterial` (substring-search every reveal audit row's JSONB for AppKey/NwkKey/AppSKey/NwkSKey).

## Decisions Made

- **Column name reality vs plan wording:** the plan referenced `archived_at IS NULL` for the device soft-delete predicate; the actual schema uses `decommissioned_at` (Phase 2 migration 0012). Adapted the new queries accordingly. The semantic — "device is currently live" — is identical; only the column name differs.
- **`ArchiveDevice` vs `DecommissionDevice`:** the plan referenced a non-existent `q.ArchiveDevice` for the bulk-decommission path. Reused the existing `q.DecommissionDevice` (Phase 2). Bulk path mirrors single-device decommission flow exactly (close active binding → CS DeleteDevice best-effort → DecommissionDevice → audit row).
- **Reveal CS surface narrowed:** introduced `CSDeviceRevealer` (GetDeviceKeys + GetDeviceActivation only) rather than widening `CSDeviceClient`. The reveal handler does a `.(CSDeviceRevealer)` type assertion; when deps.CS does not satisfy (router unit tests with no CS wiring), the handler returns 503 cs_unavailable instead of panicking.
- **Static cleanliness test alongside runtime check:** `TestRevealSecrets_AuditWriteSiteCleanliness` parses `reveal.go` at test time, strips comments, and asserts none of AppKey/NwkKey/AppSKey/NwkSKey appear in `writeAuditReveal`'s body. Runs in `-short` (no testcontainer), so a regression caused by an editor auto-import or copy-paste is caught at lint time, not in CI's 4-minute Postgres-spinup loop.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Schema column name mismatch (`archived_at` → `decommissioned_at`)**
- **Found during:** Task 1
- **Issue:** The plan's example SQL referenced `WHERE d.archived_at IS NULL`. The device table from Phase 2 migration 0012 uses `decommissioned_at` for the soft-delete column. Using the plan literal would have failed sqlc compile.
- **Fix:** Substituted `decommissioned_at IS NULL` everywhere in the two new queries, with a comment in the SQL noting the plan's "archived_at" wording.
- **Files modified:** `internal/db/queries/devices.sql`
- **Verification:** `go vet ./internal/db/...` clean; `go build ./...` clean.
- **Committed in:** `c06461c` (Task 1 commit).

**2. [Rule 3 - Blocking] Missing `q.ArchiveDevice` reference in plan body**
- **Found during:** Task 2
- **Issue:** The plan's bulk-decommission pseudocode called `q.ArchiveDevice(ctx, id, body.Reason)` which does not exist in sqlc. The Phase 2 schema has `q.DecommissionDevice(id)`.
- **Fix:** Bulk path reuses `q.DecommissionDevice` (no body.Reason parameter; reason is stored in the audit row's `notes` instead — same place where single-device decommission would record it).
- **Files modified:** `internal/device/handlers.go`
- **Verification:** `TestBulkDecommissionDevices_PartialSuccess` validates audit notes contain "bulk decommission" prefix.
- **Committed in:** `1ef3cf6` (Task 2 commit).

**3. [Rule 2 - Missing Critical] DEV-09 list-response defense-in-depth test**
- **Found during:** Task 2
- **Issue:** The plan asserted DEV-09 structurally via "the device row has no key columns", but did not include a test that pins this at the JSON projection layer. A future patch could (e.g.) add `app_key` to `filteredDevicesToJSON` and silently regress DEV-09.
- **Fix:** Added `TestListDevices_DEV09_NoKeysInRows` which iterates every row in the list envelope and asserts none of `app_key/nwk_key/app_s_key/nwk_s_key` appear as keys. Cheap, fast, future-proof.
- **Files modified:** `internal/device/list_filtered_test.go`
- **Verification:** Test passes; intentional regression (adding `"app_key": "test"` to `filteredDevicesToJSON`) triggers a clear failure.
- **Committed in:** `1ef3cf6` (Task 2 commit).

**4. [Rule 2 - Missing Critical] Audit-row write-site static cleanliness check**
- **Found during:** Task 3
- **Issue:** `TestRevealSecrets_AuditNoSecretMaterial` proves no key hex appears in the audit JSONB at runtime, but it requires running the testcontainer Postgres + driving two reveal calls. A more direct lint-level invariant would catch the regression at edit time.
- **Fix:** Added `TestRevealSecrets_AuditWriteSiteCleanliness` which reads `reveal.go`, isolates `writeAuditReveal`'s function body, strips comments, and asserts none of `AppKey/NwkKey/AppSKey/NwkSKey` appear anywhere in the function's code. Runs in `-short`.
- **Files modified:** `internal/device/reveal_test.go`
- **Verification:** Test passes; intentional regression (adding `"app_key": dev.AppKey` to writeAuditReveal — if that field existed) triggers a clear failure.
- **Committed in:** `b921a17` (Task 3 commit).

---

**Total deviations:** 4 auto-fixed (2 blocking, 2 missing-critical defensive tests). **Impact:** Two blocking fixes resolve plan/schema drift (column name + missing sqlc function); two missing-critical fixes harden the DEV-09 invariant at the test layer beyond what the plan asked for. No scope creep — every fix lands directly in the files the plan already enumerated.

## Issues Encountered

- Initial run of `TestListDevicesFiltered_TextSearch` failed because the seed used non-hex characters (`ee00000000meter1`) for a dev_eui — the device table has a CHECK constraint requiring `[0-9a-f]{16}`. Rewrote the test to use hex-only substrings (`beef`) for the dev_eui-substring branch. Caught immediately by the migration's CHECK constraint — exactly the kind of defense-in-depth Phase 2 set up.

- Parallel testcontainer Postgres runs across 4 packages exhausted Docker resources (port allocation failure on the testcontainer reaper). Re-running each failing test in isolation confirmed they pass. No code-level issue; this is a known testcontainers concurrency tradeoff. Recommend `-p 1` for the verification suite on shared CI nodes.

## User Setup Required

None.

## Next Phase Readiness

- Plan 03-09 (Devices list page) can consume `GET /api/devices?site=&status=&last_seen=&q=&sort=&page=&per_page=` directly.
- Plan 03-10 (Device detail page) can wire the "Reveal keys" button to `POST /api/devices/{eui}/keys` with the OTAA/ABP shape switch.
- Plan 03-09 bulk-decommission UX can call `POST /api/devices/bulk-decommission` with the selected rows.
- No blockers carried forward.

---
*Phase: 03-provisioning-gateways-devices-bulk-import*
*Completed: 2026-05-11*

## Self-Check: PASSED

- All listed files exist on disk.
- All three task commits exist in `git log` (`c06461c`, `1ef3cf6`, `b921a17`).
