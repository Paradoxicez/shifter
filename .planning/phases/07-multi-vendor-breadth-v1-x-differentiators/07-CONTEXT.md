# Phase 7: Multi-Vendor Breadth & v1.x Differentiators - Context

**Gathered:** 2026-05-12
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 7 closes the loop on v1.x differentiators that require real-customer signal — pre-seeded vendor profile catalog, codec test-runner UI, additional vendor mappings, statistical anomaly threshold tuning, and install validation hardening. The phase ships **without** any operator-facing tuning UI for anomaly parameters: defaults are refined in the binary based on the empirical data observed during Phase 1–6 customer deployments, then frozen in code for the v1.x release. Phase 7 also ships saved report templates, side-by-side meter/site comparison, and bulk gateway import (V2-VEND-03) so the reports surface graduates from "single-shot generation" to "operator-saved recurring views."

Requirements in scope: **V2-VEND-01** (pre-seeded catalog), **V2-VEND-02** (codec test-runner UI), **V2-VEND-03** (saved templates + side-by-side comparison + bulk gateway import), **ALERT-04 tuning** (anomaly threshold refinement using Phase 6 D-16/D-17 substrate). Plus install validation hardening extending Phase 6 06-11 `shifter doctor`.

Already-complete substrate that Phase 7 builds ON, not against:
- `device_profile` schema with vendor-agnostic measurement model (Phase 2 — wide canonical columns + JSONB `raw`)
- Axioma W1 wired profile + meter-swap canonical schema (Phase 2 DATA-06/10)
- CSV bulk import pattern with dry-run → confirm → commit + re-runnable safe semantics (Phase 3 device-import)
- shadcn/ui modal-first CRUD pattern, blue/navy aesthetic (Phase 1)
- Settings categorized page with per-category tabs (Phase 1 SETT-01) — adds "Vendor Catalog" tab
- River v0.13 + 3 statistical anomaly rule evaluators (`anomaly_p95`, `anomaly_iqr`, `anomaly_quiet_hour`) + cold-start gate + warmup roster (Phase 6 D-16/D-17)
- `shifter doctor` CLI substrate (Phase 6 06-11) — extended with `probe-chirpstack` / `probe-timescale` / `probe-region` subcommands
- Reports backend with assembler + CSV/Excel/PDF writers + ReportConfigPanel (Phase 5 05-03, 05-06, 05-09) — adds saved-template layer

Out of scope (explicitly deferred):
- **Operator-facing anomaly tuning UI** — Phase 7 ships better defaults from real data, but no per-MP/per-vendor/global knob in the UI (V2 territory if customer feedback requires)
- **Hot catalog reload without binary restart** — V2 ops feature; v1 requires binary upgrade to ship new catalog version
- **Periodic install-probe via River cron + /health/detailed banner** — alternative path; v1 uses `shifter doctor` manual invocation only
- **JS sandbox codec runtime** — v1 codecs are embedded Go; v2 may add operator-supplied JS codecs in DB
- **Admin-UI "Add custom vendor" surface** — ROADMAP SC#1 wording "vendor #N is admin-UI work, not backend deploys" is **scope-revised** for v1.x: ship 7 vendors via PR-then-release; the admin-UI add path is deferred to V2 (see D-20 note below)

</domain>

<decisions>
## Implementation Decisions

### A. Profile catalog (V2-VEND-01)

- **D-01:** **Catalog = Go `embed` in the binary** (not external volume / not DB seed migration / not hybrid). Catalog files live at `internal/codec/catalog/*.json` and are bundled via `//go:embed`. Atomic with binary upgrade — no external file deps, no install-time volume mount, no version skew between binary and catalog. Tradeoff: shipping a new catalog version requires a binary upgrade. Acceptable because PROJECT.md install model is per-customer scripted deploy by us.
- **D-02:** **Per-profile semver + Settings → Vendor Catalog "Update available" flow.** Each catalog profile carries its own `version` field (e.g., `"version": "2.3.0"`). On binary upgrade, the Catalog tab in Settings shows rows where the embedded version > the DB-installed version, with an explicit "Update" button per row. Operator-controlled: no silent auto-update on binary start. Mirrors Phase 5 D-09 Settings → Data Retention "explicit operator action" posture.
- **D-03:** **Import flow = "Add from catalog" button inside the device-profile dialog.** Device Profiles → "Add profile" dialog has two start states: "Start blank" or "Import from catalog." Importing prefills the form with catalog values; operator reviews; clicks "Add." Single dialog, single entry point. No standalone "Catalog browser" page. Follows Phase 1 modal-first CRUD convention.
- **D-04:** **Profile is editable after import — catalog rows are seeds, not links.** Import copies catalog fields into the customer's `device_profile` row; subsequent catalog updates do NOT overwrite customer edits unless operator explicitly clicks "Update" on the Catalog tab (D-24). Customer customization survives binary upgrades. Conflict handling on Update click: show diff modal listing changed fields; operator confirms which to merge (deferred refinement — Claude's discretion during planning).

### B. Codec test-runner (V2-VEND-02)

- **D-05:** **Test-runner lives inside the device-profile editor** as a section/tab (not a standalone `/test-runner` page; not also-standalone). Editing a profile → "Test codec" panel appears below the codec field. Profile context is implicit — the test runs against the codec currently being edited. Removes the "which profile am I testing" disambiguation overhead.
- **D-06:** **Input = hex bytes only** (`fPort` + payload hex). No JSON-uplink-event mode, no "pick from last 10 real uplinks" dropdown. Matches the format vendor docs always publish for codec samples (Kamstrup, Diehl spec sheets all show hex). Operators who need to debug a production uplink can paste the hex out of ChirpStack's event log.
- **D-07:** **Output = JSON tree + canonical mapping in a split panel.** Left: decoded JSON tree (expand/collapse, copy-as-JSON button). Right: canonical mapping result (`{mp_id, cumulative_value, battery, rssi, ...}`) — the same shape the ingest pipeline produces. Operator sees the vendor-format → canonical-shape translation immediately. No "expected JSON" diff feature (deferred — V2-VEND-02 wording allows it but operators rarely have curated expected JSON; canonical mapping is the actual contract).
- **D-08:** **No save / no history / no test-case library.** Test-runner is a scratch pad. Reload page → cleared. Operator paste → decode → copy result → done. Keeps the surface minimal and avoids becoming a parallel testing-as-product feature (PROJECT.md doesn't position Shifter as a codec dev tool).

### C. Anomaly threshold tuning (ALERT-04 maturation)

- **D-09:** **Ship better defaults; no operator-facing tuning UI.** Phase 6 already ships the three statistical rules (`anomaly_p95`, `anomaly_iqr`, `anomaly_quiet_hour`) with default parameters. Phase 7 refines those defaults — *in Go code* — based on the empirical signal observed during the first paying customer's first 30+ days. The refined defaults ship as part of the Phase 7 binary release. Operators don't tune; they accept the curated defaults. Aligns with PROJECT.md's self-hosted no-ops product posture.
- **D-10:** **"Test against last 30 days" backtest button on the rule-enable surface.** When operator is about to enable an anomaly rule on a metering point (or in the Settings → Alerts rule library), a "Test against last 30 days" button runs the rule's evaluator over the last 30 days of `cagg_hourly` (or raw measurement for instantaneous) and reports a single number: "Would have fired N times." Operator confidence builder — they see whether the shipped defaults are noisy for *their* install before flipping the switch. Read-only; does not write alert rows.
- **D-11:** **Cold-start visibility unchanged from Phase 6 D-16.** No additions in Phase 7: the MP detail "Anomaly detection — warming up, N days remaining" card + Settings → Alerts warmup roster are already shipped. Phase 7 does not add a "why this MP not firing" debugger surface or a dashboard "Anomaly coverage" tile (both deferred — could go in V2 if real-customer feedback surfaces the need).

### D. Install validation hardening

- **D-12:** **Probes ship as `shifter doctor` subcommands**, not a binary-start probe, not a periodic River cron, not both-doctor-and-periodic. Phase 6 06-11 already establishes `shifter doctor` — Phase 7 adds: `shifter doctor probe-chirpstack` (version + reachability), `shifter doctor probe-timescale` (extension present + version), `shifter doctor probe-region` (gateway region matches install identity region/sub-plan). Single CLI surface; operator-invoked when something feels wrong. Tradeoff: drift between binary install and customer's environment is detected only when operator runs `shifter doctor`; the alternative "ship periodic probe + banner" is deferred to V2 (kept as research note in `<specifics>`).
- **D-13:** **Probe failure = warn (banner + `/health/detailed` row), do not block install.** Failed probes log a warning, surface on the shell banner ("ChirpStack version 4.8 detected — Shifter tested against 4.10+. Continue with caution."), and add a row to `/health/detailed`. The probes do NOT cause `shifter serve` to refuse to start, even for "critical" mismatches like Postgres < 14. Reasoning: operator controls the deploy; we surface drift but never gate. Lockout escape: if Postgres really is incompatible, the binary will fail on its own at first migration / first query — we don't need a probe to enforce that.

### E. V2-VEND-03 (saved templates + side-by-side comparison + bulk gateway import)

- **D-14:** **Saved report templates capture: scope + range preset + sites/meters filter + group-by + capability filter.** A template = the full state of ReportConfigPanel that produces a deterministic report. Operator clicks "Save as template" → names it ("Monthly per-site Building A") → reloads later from "Templates" dropdown → re-runs with one click. Download format (CSV/Excel/PDF) is NOT part of the template — operator picks at run time (a template can be downloaded in multiple formats).
- **D-15:** **Templates are install-wide (shared across all users).** No per-user templates. Aligns with single-tenant operator-team install posture established in Phase 6 D-10 (shared alert inbox) and D-29 (shared user table). Audit row on template create/edit/delete; admin and viewer can both run templates; only admin can edit/delete.
- **D-16:** **Side-by-side comparison shows exactly 2 entities (A vs B).** Two-column layout: scope chip + range + delta + chart (2 series overlaid) + table (2 rows). No "compare 3-4" or "compare N." Fits mobile portrait. Operator use case is "audit this site against the install average" — two-entity is sufficient. Comparison entities can be: two sites, two metering points, or one entity over two time ranges (year-over-year).
- **D-17:** **Bulk gateway import reuses the Phase 3 device-import CSV pattern fully.** Same modal flow: upload CSV → dry-run validation → confirm → commit. Same re-runnable-safe semantics (idempotent on `gateway_eui`). Same audit-row-per-imported-row. Same error-row UI. CSV columns include lat/lng (no separate "pick on map" step — operator pre-fills lat/lng in spreadsheet). If lat/lng are missing/null, gateway is created without map placement; operator can pick on map later via Phase 5 D-15 GW-04 picker.

### F. Vendor mapping expansion

- **D-18:** **Ship 7 vendor profiles in the Phase 7 binary** as per ROADMAP SC#1: Kamstrup MULTICAL (water), Diehl (water), Itron (multi), Axioma (water — already wired since Phase 2; Phase 7 promotes from "one wired profile" to "catalog entry with metadata"), Sagemcom (electricity), Acrel (electricity), Schneider IEM3xxx (electricity). Covers both capabilities at fleet density. Each profile ships with: codec (D-19), canonical mapping, default battery low threshold, expected uplink interval, vendor name + model + capability tag.
- **D-19:** **Codec source = embedded Go functions in `internal/codec/{vendor}/`.** No JS sandbox, no ChirpStack codec passthrough, no hybrid. Each vendor codec is a Go function with signature `func Decode(payload []byte, fPort uint8) (map[string]any, error)`. Compile-time type check, no runtime parse overhead, no sandbox escape risk, single binary. Trade: adding a codec requires a Go developer + binary release (acknowledged below in D-20).
- **D-20:** **New-vendor onboarding = PR workflow (Go codec + catalog JSON entry) → binary release.** A new vendor onboarding in v1.x is: contributor adds `internal/codec/{vendor}.go` + entry in `internal/codec/catalog/{vendor}.json` + unit tests → PR → review → merge → binary release → customer upgrades binary → catalog tab shows new vendor as "Available, click Install." **Scope note:** This is a deliberate v1.x revision of ROADMAP SC#1's "admin UI work, not backend deploys" language. The admin-UI "Add custom vendor" path is deferred to V2 (requires JS sandbox runtime — D-19 explicitly defers that). Document the change in v1.x release notes.

### G. Catalog file format + validation

- **D-21:** **Catalog format = JSON** (not YAML, not Go struct literals). Files at `internal/codec/catalog/*.json`. Decoded via stdlib `encoding/json`. Tool-friendly (every editor highlights JSON), PR-diff-friendly, no extra dependency. Verbosity tradeoff accepted (catalog entries are short — vendor + model + version + ~10 metadata fields).
- **D-22:** **Schema validation via `go test` at build time** (not JSON Schema lib, not runtime-only). A `TestCatalogValid` test in `internal/codec/catalog_test.go` decodes every embedded JSON file, unmarshals into a typed Go struct, and asserts required fields + enum values + EUI prefix format + capability tag validity. CI fails the build if catalog is invalid — production binary cannot ship with a broken catalog. No runtime JSON-Schema library dep needed.

### H. Settings → Vendor Catalog tab UI

- **D-23:** **Catalog tab UI = table layout** (not card grid, not categorized-by-capability sections). Columns: Vendor | Model | Capability | Version | Installed | Status. Status enum: `not-installed` | `installed` | `update-available`. Sortable by any column; filter by capability via chip row. Mirrors Phase 6 D-29 settings-table convention; works on mobile via shadcn DataTable.
- **D-24:** **Update flow = per-row "Update available v2.1.0 → v2.3.0" badge + Update button → confirm dialog with diff modal.** Clicking Update on a row where embedded > installed: dialog shows side-by-side diff of changed fields (e.g., codec changed, expected_interval changed, battery_low_threshold changed). Operator confirms which fields to merge into the customer's device_profile row. Customer-edited fields are flagged in the diff with "you edited this — keep yours?" toggle. Preserves D-04 editability.
- **D-25:** **Show "Devices using" count per profile row.** Column displays N (where N = active devices linked to this profile). Click N → navigates to Devices list pre-filtered by `profile_id`. Helps operator decide whether to disable an unused profile (if N=0, the action is safe). Mirrors Phase 3 gateway-usage pattern.

### Claude's Discretion

- Exact catalog JSON schema fields (icon path? logo embed? short description? long description?) — Claude picks during planning, informed by Phase 2 `device_profile` table shape
- Diff modal layout details (field-by-field rows? unified diff style? structured form?) — Claude picks during UI planning
- Whether "Test against last 30 days" backtest result includes a histogram or just a count — Claude picks based on chart complexity vs information value tradeoff
- Catalog versioning bump strategy (when does a catalog edit warrant a semver bump?) — documented in plan, not in CONTEXT
- Migration path for the existing Axioma W1 profile (in-binary catalog entry vs special-cased legacy row) — Claude picks during planning

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### ROADMAP and requirements
- `.planning/ROADMAP.md` §Phase 7 — Goal, depends_on, requirements, success criteria, UI hint
- `.planning/REQUIREMENTS.md` — V2-VEND-01, V2-VEND-02, V2-VEND-03 entries + ALERT-04 tuning context

### Prior phase decisions Phase 7 builds on
- `.planning/phases/02-domain-model-canonical-schema/02-CONTEXT.md` — vendor-agnostic measurement model (wide canonical + JSONB raw); Axioma W1 wired profile pattern
- `.planning/phases/03-provisioning-gateways-devices-bulk-import/03-CONTEXT.md` — CSV bulk-import pattern (dry-run → confirm → commit, re-runnable safe, audit-per-row)
- `.planning/phases/05-aggregates-reports-map-floor-plans/05-CONTEXT.md` — ReportConfigPanel state shape (scope + range + filters + group-by) that templates serialize; Settings → Data Retention card pattern that Vendor Catalog tab mirrors
- `.planning/phases/06-alerts-users-audit-operational-hardening/06-CONTEXT.md` §D-16, §D-17 — cold-start gate substrate; 3 statistical rule kinds (`anomaly_p95`, `anomaly_iqr`, `anomaly_quiet_hour`) Phase 7 tunes defaults for; D-29 Settings tab pattern; `shifter doctor` substrate Phase 7 extends

### Project-level
- `.planning/PROJECT.md` — Self-hosted single-binary per-customer install; tech stack (Go embed, TimescaleDB, shadcn/ui); no SMTP / no SSO / no live device control in v1; modal-first CRUD; blue/navy aesthetic

### External vendor / library references
- TimescaleDB CAGG docs §`time_bucket` + `cagg_hourly` query patterns — backtest evaluator runs against `cagg_hourly` for hourly anomaly rules
- ChirpStack v4 device profile API — Phase 3 already wired; Phase 7 catalog import does NOT push to ChirpStack (codec runs in Shifter, ChirpStack runs its own MAC layer)
- React Hook Form + zod schema validation — used for template save/load form (Phase 5 pattern)
- shadcn DataTable — used for Vendor Catalog table (D-23)

### No external ADRs
No formal `docs/decisions/` ADRs exist for this phase — all decisions captured here. CONTEXT.md IS the contract; downstream planner should treat each D-NN as locked unless flagged Claude's Discretion.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets

- **`internal/device/profile.go`** — `device_profile` table schema established Phase 2. Phase 7 adds optional `catalog_source_version` column to track which catalog version a profile was imported from (drives Update-available logic).
- **Phase 2's Axioma W1 codec** — already lives somewhere in `internal/codec/` (or `internal/device/codec/`). Phase 7 catalog migration: move it to `internal/codec/axioma/` + add catalog entry for it, preserving the existing wire path so Axioma users see zero behavioral change.
- **`internal/report/assembler.go` + ReportConfigPanel** — Phase 5 wraps reports. Phase 7 adds `report_template` table + `templateService.Save/Load/List/Delete` + a "Templates" dropdown in ReportConfigPanel.
- **`internal/import/csv.go` (or similar)** — Phase 3 device-import pattern is the template for Phase 7's bulk gateway import. Reuse `dryRun → confirm → commit` lifecycle + `audit_log` row-per-import. Wrap with `internal/gateway/import.go`.
- **`internal/alert/anomaly_*.go`** — Phase 6's three anomaly evaluators. Phase 7 changes ONLY the default constants (P95 percentile threshold, IQR multiplier, quiet-hour bounds) — no signature changes, no logic changes. Add `BacktestRun(ctx, rule, mp_id, days)` helper for the D-10 "Test against last 30 days" button.
- **`cmd/shifter/doctor.go` (Phase 6 06-11)** — `shifter doctor` Cobra subcommand. Phase 7 adds three more subcommands inside the same package: `doctor probe-chirpstack`, `doctor probe-timescale`, `doctor probe-region`. Reuse output format + exit-code conventions.
- **`web/src/routes/settings/`** — Settings page with tab strip (Phase 5 D-09, Phase 6 D-29). Phase 7 adds `VendorCatalogCard.tsx` + `ImportFromCatalogDialog.tsx` + `UpdateCatalogDialog.tsx` (the diff modal).
- **`web/src/routes/reports/ReportConfigPanel.tsx`** — Phase 5 09. Phase 7 wraps with `SaveTemplateButton` + `TemplatesDropdown` + persists state via React Hook Form integration already in place.

### Established Patterns

- **Settings tab pattern** (Phase 5 D-09 / Phase 6 D-29): card layout, table inside, dialogs for CRUD, audit-row-on-mutate. Vendor Catalog tab follows this.
- **`go:embed` for static assets** (Phase 1 D-23 — install wizard assets). Phase 7 reuses for catalog JSON files: `//go:embed catalog/*.json var catalogFS embed.FS`.
- **Cobra CLI subcommand convention** (Phase 1 D-12, Phase 6 06-11): each command has `Use`, `Short`, `Long`, `RunE`; output to stdout in human-readable format with `--json` flag for machine output (Phase 1 D-15 standard).
- **Audit-row-in-tx pattern** (Phase 2 D-21/D-22, AUDIT-01): every catalog Import / Update / Disable writes an `audit_log` row in the same Postgres tx as the mutation. Phase 7 catalog actions follow this.
- **React Hook Form + zod** (Phase 5 D-09): used for Template save dialog and Import from Catalog form.

### Integration Points

- **`device_profile` table** (Phase 2 schema) — Phase 7 ALTERs to add `catalog_source` (text, FK to catalog entry by vendor+model), `catalog_source_version` (text, semver of catalog entry at import time), `customer_edited` (bool, set true when operator modifies any field after import). Migration 0048 (or wherever Phase 7 lands).
- **`audit_log` table** (Phase 2 / Phase 6) — Phase 7 adds new `Action` constants: `catalog.profile.imported`, `catalog.profile.updated`, `report_template.created`, `report_template.deleted`, `gateway.bulk_imported`.
- **`/health/detailed` endpoint** (Phase 1 D-19, Phase 6 D-21) — Phase 7 adds `probe_results` block with last-run timestamp + result per probe + drift count. Drives the shell banner referenced in D-13.
- **`router.go`** — Phase 7 mounts `/api/catalog/*` (list, import, update endpoints) and `/api/reports/templates/*` (CRUD) and extends `/api/gateways/*` with `/bulk-import` (CSV upload).
- **Phase 5 PDF worker (River queue)** — saved templates that produce PDFs reuse the existing River worker; no new queue needed.

</code_context>

<specifics>
## Specific Ideas

- **"Ship better defaults" approach for anomaly tuning** is borrowed from Linear's anomaly-detection ergonomics — defaults that work for 90% of users out of the box, and the operator who needs more control runs a CLI flag (deferred to V2). Linear specifically does NOT expose every threshold in the UI because tuning thresholds is a job for the product, not the customer.
- **"Update available" UX in Settings → Vendor Catalog** is shaped after VS Code's extensions tab — a clear badge, a one-click Update button, a diff view before the merge. Familiar pattern from operator's developer-tool muscle memory.
- **Per-profile semver** (not catalog-level semver) follows the design of [Homebrew's Formulary](https://docs.brew.sh/Formula-Cookbook) and Helm chart versioning — each profile is independently versioned because vendor codec changes are uncorrelated (Kamstrup's MULTICAL firmware update is unrelated to Acrel's tariff schedule update).
- **Side-by-side 2-entity comparison** matches the "compare diff" muscle memory operators have from GitHub PR diff view — two columns, aligned rows.
- **Saved templates as install-wide shared resources** matches Linear's "shared views" concept and avoids the per-user fragmentation that makes Notion's "saved filters" so confusing in multi-operator setups.

### Research-noted alternatives (not chosen but worth re-evaluating if Phase 7 misses)

- **JS sandbox codec runtime** (rejected as D-19) — re-evaluate if v2 customer adds a vendor not on our roadmap and demands self-service onboarding. Goja (https://github.com/dop251/goja) is the obvious choice; benchmark before commit.
- **Periodic install-probe via River cron** (rejected as D-12) — re-evaluate if v1.x customers report environments drifting silently between install and first incident; the manual `shifter doctor` cadence is fragile in that case.
- **Operator-facing anomaly tuning UI** (rejected as D-09) — re-evaluate if real-customer feedback shows the shipped defaults are systemically wrong for a class of meter (e.g., commercial water vs residential water have different anomaly profiles).

</specifics>

<deferred>
## Deferred Ideas

These came up during discussion but explicitly belong outside Phase 7 scope:

- **JS sandbox codec runtime + admin-UI "Add custom vendor"** — V2 territory. Requires Goja or V8 binding, sandbox testing, hot-reload semantics, and admin-UI codec editor. Phase 7 ships the 7 catalog vendors via Go; V2 makes the admin-UI add path real.
- **Hot catalog reload without binary restart** — V2 ops feature. Phase 7 requires binary upgrade to ship a new catalog version. V2 may expose a `shifter catalog reload` CLI or a Settings → "Reload catalog" button.
- **Periodic install-probe via River cron + shell banner on drift** — V2 ops hardening. Phase 7 ships only manual `shifter doctor` invocation.
- **Anomaly tuning UI (per-MP / per-vendor / global)** — V2 if real-customer feedback requires. Phase 7 ships better defaults via code refinement only.
- **"Why is this MP not firing" explanation surface** on the MP detail page — V2 alert-debug feature. Phase 7 keeps Phase 6 D-16 warmup card as the only cold-start surface.
- **Dashboard "Anomaly coverage" tile** ("74% of MPs eligible for anomaly detection") — V2 dashboard polish.
- **Compare 3+ entities side-by-side** — V2 power-user feature. Phase 7 ships 2-entity comparison only.
- **Per-user saved templates** — V2 multi-operator workflow. Phase 7 ships install-wide shared templates.
- **CSV-with-pick-on-map step for bulk gateway import** — friction-reduction. Phase 7 ships CSV-only (lat/lng in spreadsheet); operator can pick on map after import via Phase 5 GW-04 picker.
- **Save test-runner test cases as profile fixtures** — V2 testing-as-product feature. Phase 7 ships test-runner as a scratch pad.
- **Diff against expected JSON in test-runner output** — V2 codec-dev feature. Phase 7 ships canonical mapping only.

</deferred>

---

*Phase: 07-multi-vendor-breadth-v1-x-differentiators*
*Context gathered: 2026-05-12*
