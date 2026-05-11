# Phase 3: Provisioning (Gateways, Devices, Bulk Import) - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-05-11 (resumed from 2026-05-09 checkpoint)
**Phase:** 03-provisioning-gateways-devices-bulk-import
**Mode:** discuss (interactive)
**Language:** Thai (user-facing); English (technical terms, code, file paths)
**Areas discussed:** Gateway surface scope & RX/TX stats; Bulk import — schema & file formats; Two-phase dry-run/commit UX; Devices list filters & pagination; OTAA/ABP UX in add-device; Server-side viewer hiding for new secret fields; Gateway add-flow shape & decommission; Audit-log volume & request_id grouping for bulk import

---

## 1. Gateway surface scope & RX/TX stats

### Q-1.1 — Gateway map pin: ship in Phase 3 or defer to Phase 5?

| Option | Description | Selected |
|--------|-------------|----------|
| Defer to Phase 5; Phase 3 ships lat/lng numeric inputs + disabled 'Pick on map' button mirroring D-18 (RECOMMENDED) | Anchor v5 visually; no Leaflet install yet | ✓ |
| Install Leaflet in Phase 3 for gateways only | Map UI partial in Phase 3 | |
| Install Leaflet in Phase 3 for gateways + sites (reopen D-18) | Reverse Phase 2 D-18 deferral | |

**Decision:** D-01

### Q-1.2 — Gateway list RX/TX stats: which time window?

| Option | Description | Selected |
|--------|-------------|----------|
| Last 24h: RX + TX + uplink success % + sparkline (RECOMMENDED) | Fleet-ops feel; cached 1 min | ✓ |
| Last 7d: aggregate only, no sparkline | Lower fan-out but blander | |
| Lifetime cumulative + 24h sparkline | Long-tail data weight | |
| Online/offline + last_seen + lat/lng — RX/TX in detail page | Minimal list; detail-page split | |

**Decision:** D-02

### Q-1.3 — Gateway region picker: inherit install default or per-gateway override?

| Option | Description | Selected |
|--------|-------------|----------|
| Inherit install default + override (RECOMMENDED) | GW-02 regulator-aware; supports multi-region | ✓ |
| Force install default — no dropdown | Lock to install region | |
| Force explicit selection every time | Friction; no default | |

**Decision:** D-03

---

## 2. Bulk import — schema, formats, idempotency, errors

### Q-2.1 — CSV column schema

| Option | Description | Selected |
|--------|-------------|----------|
| Minimum + optional metadata (RECOMMENDED) | Required dev_eui/name/profile/site + optional keys/desc/tags | ✓ |
| Full ChirpStack contract | All API fields; CSV row width | |
| Minimum only — no metadata | Edit metadata later | |

**Decision:** D-04 (schema)

### Q-2.2 — DevEUI/AppKey format (PITFALLS §13)

| Option | Description | Selected |
|--------|-------------|----------|
| Big-endian hex only + auto-strip 0x/colons (RECOMMENDED) | Normalize → lowercase 16-hex; reject otherwise | ✓ |
| Accept both endianness with flag | Operator-error risk | |
| Big-endian only — reject without normalize | Friction for operator | |

**Decision:** D-05

### Q-2.3 — Idempotency on re-upload

| Option | Description | Selected |
|--------|-------------|----------|
| dev_eui = idempotency key; second upload skips/reports existing (RECOMMENDED) | already_exists row outcome | ✓ |
| Job-level idempotency: SHA-256 of file | Brittle if any row changes | |
| Update-on-conflict mode (UPSERT toggle) | Footgun; bulk-overwrite key risk | |

**Decision:** D-06

### Q-2.4 — Partial commit vs hard-fail

| Option | Description | Selected |
|--------|-------------|----------|
| Partial commit: valid rows commit, invalid rows reported (RECOMMENDED) | Per-row outcome + error CSV download | ✓ |
| All-or-nothing: any invalid row aborts whole file | Data-integrity-pure but friction | |
| Defer to dry-run/commit (Q-3.x) | Wrong scope | |

**Decision:** D-07

### Q-2.8 (revisited after user feedback) — Supported import file formats

> **User feedback:** "csv ไม่ค่อย support ภาษาไทย" — Excel on Windows-Thai locale saves CSV as TIS-620/CP874 by default, breaking UTF-8 ingest. Revisited file-format scope.

| Option | Description | Selected |
|--------|-------------|----------|
| XLSX + CSV (UTF-8 with BOM) (RECOMMENDED) | XLSX primary (Thai-safe); CSV secondary; download-template button | ✓ |
| XLSX only — drop CSV | Loses ops-script ergonomics | |
| CSV only (UTF-8 + BOM) | Friction for Thai operator | |

**Decision:** D-04a; also drove rename `csv_import_job` → `import_job` (D-34)

---

## 3. Two-phase dry-run / commit UX

### Q-3.1 — Dry-run scope

| Option | Description | Selected |
|--------|-------------|----------|
| Validate-only — no ChirpStack calls (RECOMMENDED) | Local validation; deterministic | ✓ |
| Full simulation — call ChirpStack then rollback | Catches CS-side errors but dirty-CS risk | |
| Hybrid — validate + 1-shot CS lookup for refs | Compromise | |

**Decision:** D-08

### Q-3.2 — Preview presentation

| Option | Description | Selected |
|--------|-------------|----------|
| Summary banner + per-row table (RECOMMENDED) | Banner counts + TanStack Table filter/sort | ✓ |
| Summary only — no row table | Less to scan; less detail | |
| Per-row table only — no summary | Pure but slow scan for 5k rows | |

**Decision:** D-09

### Q-3.3 — Commit confirmation

| Option | Description | Selected |
|--------|-------------|----------|
| Button-only confirmation (RECOMMENDED) | Mirrors Phase 2 D-15 decommission | ✓ |
| Type-to-confirm count or "CONFIRM" | Anti-fat-finger but friction-heavy | |
| Checkbox + button | Middle ground | |

**Decision:** D-10

### Q-3.4 — Dry-run job lifecycle

| Option | Description | Selected |
|--------|-------------|----------|
| Server-side job, TTL 1h, resumable via job_id (RECOMMENDED) | Survives tab close/network glitch | ✓ |
| Session-only (in-memory) | Closed tab = lost preview | |
| Persistent (no TTL) + manual cleanup | DB bloat | |

**Decision:** D-11

---

## 4. Devices list — filters, paging, sort, state, export, bulk actions, columns

### Q-4.1 — Filter dimensions

| Option | Description | Selected |
|--------|-------------|----------|
| Site + activation status + last_seen window + search (RECOMMENDED) | Phase 3 baseline | ✓ |
| Full set incl. profile/tags/region/battery | UI overload for Phase 3 | |
| Minimal — site + search only | Underpowered | |

**Decision:** D-12

### Q-4.2 — Pagination

| Option | Description | Selected |
|--------|-------------|----------|
| Offset-based, page size 50 (RECOMMENDED) | Cursor overkill at Shifter scale | ✓ |
| Cursor-based (keyset) | Million-row pattern | |
| Infinite scroll | Wrong UX for admin tool | |

**Decision:** D-13

### Q-4.3 — Sortable columns

| Option | Description | Selected |
|--------|-------------|----------|
| name, dev_eui, site, last_seen, created_at (RECOMMENDED) | Identity + freshness; indexes cheap | ✓ |
| name + last_seen only | Restrictive | |
| All columns sortable | Index storage overhead | |

**Decision:** D-14

### Q-4.4 — Filter state persistence

| Option | Description | Selected |
|--------|-------------|----------|
| URL query params (RECOMMENDED) | Shareable, back/forward-aware | ✓ |
| Server-side per-user via localStorage | Personal but not shareable | |
| Session-only — no persist | Friction on refresh | |

**Decision:** D-15

### Q-4.5 — CSV/XLSX export

| Option | Description | Selected |
|--------|-------------|----------|
| Export current filtered view + round-trip-compatible columns (RECOMMENDED) | Export → fix → re-import flow | ✓ |
| Export all (ignore filter) | Often unwanted | |
| No export in Phase 3 | Defer; reports phase has its own | |

**Decision:** D-16

### Q-4.6 — Bulk actions

| Option | Description | Selected |
|--------|-------------|----------|
| Decommission only (RECOMMENDED) | Meter-swap workflow; mirrors D-10/D-15 confirm | ✓ |
| Decommission + reassign + tag edit | Too much footgun surface | |
| No bulk action in Phase 3 | Forces per-row work | |

**Decision:** D-17

### Q-4.7 — Column visibility toggle

| Option | Description | Selected |
|--------|-------------|----------|
| Fixed columns — no toggle (RECOMMENDED) | Single-tenant operator tool | ✓ |
| User-toggleable + localStorage | Per-user complexity | |
| User-toggleable + URL params | Noisy URL | |

**Decision:** D-18

---

## 5. OTAA / ABP UX in add-device dialog

### Q-5.1 — How to pick activation mode

| Option | Description | Selected |
|--------|-------------|----------|
| Dedicated step in stepped dialog (RECOMMENDED) | Extends Phase 2 4-step → 5-step | ✓ |
| Toggle on same step as keys | Form complexity in one step | |
| Inferred from device_profile | Profile-mode vs activation-mode mismatch risk | |

**Decision:** D-19

### Q-5.2 — ABP fields

| Option | Description | Selected |
|--------|-------------|----------|
| Manual entry 3 fields + optional fcnt_up/down (RECOMMENDED) | Forward-compat for meter-swap (Phase 7+) | ✓ |
| Auto-generate keys server-side | Doesn't fit ABP use case | |
| Required 3 keys, no fcnt | Breaks future swap workflows | |

**Decision:** D-20

### Q-5.3 — Showing keys after creation

| Option | Description | Selected |
|--------|-------------|----------|
| Success state shows keys until close + Copy button (RECOMMENDED) | Operator needs to program meter immediately | ✓ |
| Close dialog + redirect to detail page | Friction & confusion | |
| Don't show — admin must reveal on detail | High friction | |

**Decision:** D-21

### Q-5.4 — Key storage strategy

| Option | Description | Selected |
|--------|-------------|----------|
| ChirpStack-only — Shifter never stores keys (RECOMMENDED) | Extends DEV-09 invariant; smallest attack surface | ✓ |
| Shifter cache + CS source-of-truth | Drift risk; key-management overhead | |
| Shifter encrypted-at-rest column | Breaks DEV-09 invariant | |

**Decision:** D-22

### Q-5.5 — MAC version pick

| Option | Description | Selected |
|--------|-------------|----------|
| Inherited from device_profile (RECOMMENDED) | ChirpStack source-of-truth; no drift | ✓ |
| Inherit + override per device | Mismatch risk | |
| Hardcode 1.0.3 | Locks out 1.1.x | |

**Decision:** D-23

### Q-5.6 — Join-EUI vs App-EUI labelling

| Option | Description | Selected |
|--------|-------------|----------|
| Label `Join EUI (AppEUI for v1.0)` (RECOMMENDED) | Covers both spec generations | ✓ |
| Label varies by MAC version | UI shift confusing | |
| Hide field — always zeros | Breaks vendors using JoinEUI | |

**Decision:** D-24

### Q-5.7 — Custom join-server

| Option | Description | Selected |
|--------|-------------|----------|
| Defer — not in Phase 3 (RECOMMENDED) | No customer ask | ✓ |
| Expose optional URL field | YAGNI for v1 | |
| Always-use ChirpStack internal | Same effect as defer | |

**Decision:** D-25

---

## 6. Server-side viewer hiding for new secret fields

### Q-6.1 — Reveal endpoint authorization

| Option | Description | Selected |
|--------|-------------|----------|
| Admin role only via Can('device.reveal_secrets') (RECOMMENDED) | New action in can.go; viewer → 403 | ✓ |
| Admin + audit (always audit reveal) | Combined with Q-6.3 — see D-28 | |
| Admin + reveal expires after 30s | Wrong knob; meter programming takes longer | |

**Decision:** D-26 (and combined audit in D-28)

### Q-6.2 — Implementation strategy

| Option | Description | Selected |
|--------|-------------|----------|
| Separate query path — reveal endpoint ≠ list query (RECOMMENDED) | Structural, bug-resistant | ✓ |
| Same query, app-layer redact for viewers | Leak risk on code-path change | |
| Show encrypted, client decrypts if perm | Overkill; doesn't fit session model | |

**Decision:** D-27

### Q-6.3 — Audit row shape

| Option | Description | Selected |
|--------|-------------|----------|
| action=device.reveal_secrets + resource_id + no diff (RECOMMENDED) | Never log secret material | ✓ |
| + IP + user-agent | audit_log shape change | |
| No audit — authz alone | No forensic trail | |

**Decision:** D-28

---

## 7. Gateway add-flow shape & decommission

### Q-7.1 — Add-gateway dialog shape

| Option | Description | Selected |
|--------|-------------|----------|
| Single dialog — no stepper (RECOMMENDED) | Field count modest; atomic submit | ✓ |
| 3-step stepper | Overkill for fleet provisioning | |
| Inline form (no dialog) | Violates PROJECT.md "All CRUD via dialogs" | |

**Decision:** D-29

### Q-7.2 — Decommission semantics

| Option | Description | Selected |
|--------|-------------|----------|
| Soft-delete PG + DeleteGateway in CS (RECOMMENDED) | Atomic + best-effort rollback (D-16) | ✓ |
| Soft-delete PG only — don't touch CS | Gateway still serving uplinks | |
| CS delete only — keep PG row | Orphan refs from devices | |

**Decision:** D-30

### Q-7.3 — Decommission timing constraints

| Option | Description | Selected |
|--------|-------------|----------|
| Allow anytime + warn if uplinks in last 24h (RECOMMENDED) | Operator trust; meter-swap-friendly | ✓ |
| Block if uplinks in 24h | Friction for legitimate ops | |
| Always allow — no warning | No fat-finger safeguard | |

**Decision:** D-31

### Q-7.4 — Gateway list visibility post-decommission

| Option | Description | Selected |
|--------|-------------|----------|
| Hidden by default + Show archived toggle (RECOMMENDED) | Clear active/archived mental model | ✓ |
| Fully hidden — direct link only | Loses recovery path | |
| Mixed list with archived flag column | Daily-ops noise | |

**Decision:** D-32

---

## 8. Audit-log volume & request_id grouping for bulk import

### Q-8.1 — Audit rows per bulk import

| Option | Description | Selected |
|--------|-------------|----------|
| 1 row per device + 1 envelope row (RECOMMENDED) | Per-device traceability + job summary | ✓ |
| 1 envelope row only | Loses per-device when-created queries | |
| Per-row only, no envelope | No job summary | |

**Decision:** D-33

### Q-8.2 — request_id grouping

| Option | Description | Selected |
|--------|-------------|----------|
| import_job.job_id = request_id of every row (RECOMMENDED) | Groups all rows of a single import | ✓ |
| HTTP X-Request-Id | Not unique per row in single-commit tx | |
| No group — infer from time+actor | Fragile | |

**Decision:** D-34

### Q-8.3 — import_job row retention (user requested explanation; chose recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| 90 days + archive policy deferred to Phase 9 (RECOMMENDED) | Balances debug needs vs DB bloat | ✓ |
| Keep indefinitely | Bloat from daily imports | |
| 30 days | Too short for quarterly audits | |

**Decision:** D-35

### Q-8.4 — Audit query UI in Phase 3 (user requested explanation; chose recommended)

| Option | Description | Selected |
|--------|-------------|----------|
| No general audit UI in Phase 3; only /admin/imports/:job_id detail page (RECOMMENDED) | Phase 9 owns /admin/audit; Phase 3 ships per-job view | ✓ |
| Add /admin/audit in Phase 3 | Scope creep | |
| Inline audit on device detail page | Detail page not in Phase 3 scope | |

**Decision:** D-36

---

## Claude's Discretion

User explicitly left to Claude:
- `import-template.xlsx` layout details
- TanStack Table column widths, sort directions, sparkline styling
- ResponsiveDialog success-state copy
- Error-row export filename convention
- Reveal-endpoint rate limiting
- Sync vs background-job processing for >10k-row imports

## Deferred Ideas

- Map UI for gateway/site pin-drop → Phase 5
- General audit query UI → Phase 9
- `import_job` archive/purge cron → Phase 9
- Bulk edit (reassign site / edit tags) → not planned
- Custom external join-server → backlog
- Cursor pagination for devices → revisit at ~100K devices
- Per-user column-visibility persistence → not planned
- Background-job processing for very large imports → Phase 3 sync first; queue via River if needed
- Reveal-endpoint rate limiting → not in v1 threat model
