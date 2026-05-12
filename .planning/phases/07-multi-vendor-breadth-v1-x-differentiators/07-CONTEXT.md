# Phase 7: Multi-Vendor Breadth & v1.x Differentiators - Context

**Gathered:** 2026-05-12 (initial); updated 2026-05-12 (update pass — added D-26..D-48, corrected D-19 JS-vs-Go architectural mismatch found via codebase scout, seeded Itron+KINMY catalog entry)
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 7 closes the loop on v1.x differentiators that require real-customer signal — pre-seeded vendor profile catalog, codec test-runner UI, additional vendor mappings, statistical anomaly threshold tuning, and install validation hardening. The phase ships **without** any operator-facing tuning UI for anomaly parameters: defaults are refined in the binary based on the empirical data observed during Phase 1–6 customer deployments, then frozen in code for the v1.x release. Phase 7 also ships saved report templates, side-by-side meter/site comparison, and bulk gateway import (V2-VEND-03) so the reports surface graduates from "single-shot generation" to "operator-saved recurring views."

Requirements in scope: **V2-VEND-01** (pre-seeded catalog), **V2-VEND-02** (codec test-runner UI), **V2-VEND-03** (saved templates + side-by-side comparison + bulk gateway import), **ALERT-04 tuning** (anomaly threshold refinement using Phase 6 D-16/D-17 substrate). Plus install validation hardening extending Phase 6 06-11 `shifter doctor`.

**Update-pass scope expansion (2026-05-12):** Phase 7 also refactors **ALERT-03 (offline detection) + ALERT-04 (anomaly rules) to be profile-aware** — per-profile `expected_uplink_interval_seconds`, `offline_threshold_multiplier`, and `anomaly_compatibility` enum drive thresholds (D-41..D-43). Also adds a **`battery_curve` registry in `normalize.go`** (D-44) — vendors that report battery as voltage (e.g., Itron+KINMY) get a non-linear curve applied during ingest so canonical `battery_pct` is meaningful for ALERT-02. And a new **delta-based alert rule `reverse_flow_increase`** (D-46) covering vendors that report a cumulative reverse-flow counter (Itron+KINMY).

Already-complete substrate that Phase 7 builds ON, not against:
- `device_profile` schema with vendor-agnostic measurement model (Phase 2 — wide canonical columns + JSONB `raw`)
- Axioma W1 wired profile + meter-swap canonical schema (Phase 2 DATA-06/10)
- CSV bulk import pattern with dry-run → confirm → commit + re-runnable safe semantics (Phase 3 device-import)
- shadcn/ui modal-first CRUD pattern, blue/navy aesthetic (Phase 1)
- Settings categorized page with per-category tabs (Phase 1 SETT-01) — adds "Vendor Catalog" tab
- River v0.13 + 3 statistical anomaly rule evaluators (`anomaly_p95`, `anomaly_iqr`, `anomaly_quiet_hour`) + cold-start gate + warmup roster (Phase 6 D-16/D-17)
- `shifter doctor` CLI substrate (Phase 6 06-11) — extended with `probe-chirpstack` / `probe-timescale` / `probe-region` subcommands
- Reports backend with assembler + CSV/Excel/PDF writers + ReportConfigPanel (Phase 5 05-03, 05-06, 05-09) — adds saved-template layer
- **JS codec runtime via ChirpStack QuickJS sandbox** (Phase 2 D-09, Pitfalls §3) — codecs live as `*.js` files under `internal/profile/codecs/`, are bundled via `//go:embed`, and pushed to ChirpStack by `internal/profile/seed.go` for execution. Phase 7 catalog extends this — does NOT migrate to Go decoders (see D-19 correction below).

Out of scope (explicitly deferred):
- **Operator-facing anomaly tuning UI** — Phase 7 ships better defaults from real data, but no per-MP/per-vendor/global knob in the UI (V2 territory if customer feedback requires)
- **Hot catalog reload without binary restart** — V2 ops feature; v1 requires binary upgrade to ship new catalog version
- **Periodic install-probe via River cron + /health/detailed banner** — alternative path; v1 uses `shifter doctor` manual invocation only
- **Operator-supplied custom JS codecs in DB / admin-UI "Add custom vendor"** — v1 codecs are PR-then-release (D-20 below); V2 may add admin-UI codec editor
- **Per-MP override of `offline_threshold_multiplier` / `expected_uplink_interval`** — Phase 7 sets catalog-driven defaults only (D-43); per-MP override deferred to Phase 8
- **`meter_id` canonical promotion (cross-vendor meter-swap detection at meter-serial level)** — Phase 7 keeps meter_id in `raw` JSONB only (D-45); Phase 8/v1.1 may promote to canonical column with backfill once more vendor evidence accumulates

</domain>

<decisions>
## Implementation Decisions

### A. Profile catalog (V2-VEND-01)

- **D-01:** **Catalog = Go `embed` in the binary** (not external volume / not DB seed migration / not hybrid). Catalog files live at `internal/codec/catalog/*.json` and are bundled via `//go:embed`. Atomic with binary upgrade — no external file deps, no install-time volume mount, no version skew between binary and catalog. Tradeoff: shipping a new catalog version requires a binary upgrade. Acceptable because PROJECT.md install model is per-customer scripted deploy by us.
- **D-02:** **Per-profile semver + Settings → Vendor Catalog "Update available" flow.** Each catalog profile carries its own `version` field (e.g., `"version": "2.3.0"`). On binary upgrade, the Catalog tab in Settings shows rows where the embedded version > the DB-installed version, with an explicit "Update" button per row. Operator-controlled: no silent auto-update on binary start. Mirrors Phase 5 D-09 Settings → Data Retention "explicit operator action" posture.
- **D-03:** **Import flow = "Add from catalog" button inside the device-profile dialog.** Device Profiles → "Add profile" dialog has two start states: "Start blank" or "Import from catalog." Importing prefills the form with catalog values; operator reviews; clicks "Add." Single dialog, single entry point. No standalone "Catalog browser" page. Follows Phase 1 modal-first CRUD convention.
- **D-04:** **Profile is editable after import — catalog rows are seeds, not links.** Import copies catalog fields into the customer's `device_profile` row; subsequent catalog updates do NOT overwrite customer edits unless operator explicitly clicks "Update" on the Catalog tab (D-24). Customer customization survives binary upgrades. Conflict handling on Update click: D-34 specifies field-by-field rows with side-by-side values + per-field toggle + "you edited this" flag preserving customer_edited fields.

### B. Codec test-runner (V2-VEND-02)

- **D-05:** **Test-runner lives inside the device-profile editor** as a section/tab (not a standalone `/test-runner` page; not also-standalone). Editing a profile → "Test codec" panel appears below the codec field. Profile context is implicit — the test runs against the codec currently being edited. Removes the "which profile am I testing" disambiguation overhead.
- **D-06:** **Input = hex bytes only** (`fPort` + payload hex). No JSON-uplink-event mode, no "pick from last 10 real uplinks" dropdown. Matches the format vendor docs always publish for codec samples (Kamstrup, Diehl spec sheets all show hex). Operators who need to debug a production uplink can paste the hex out of ChirpStack's event log.
- **D-07:** **Output = JSON tree + canonical mapping in a split panel.** Left: decoded JSON tree (expand/collapse, copy-as-JSON button). Right: canonical mapping result (`{mp_id, cumulative_value, battery, rssi, ...}`) — the same shape the ingest pipeline produces. Operator sees the vendor-format → canonical-shape translation immediately. No "expected JSON" diff feature (deferred — V2-VEND-02 wording allows it but operators rarely have curated expected JSON; canonical mapping is the actual contract).
- **D-08:** **No save / no history / no test-case library.** Test-runner is a scratch pad. Reload page → cleared. Operator paste → decode → copy result → done. Keeps the surface minimal and avoids becoming a parallel testing-as-product feature (PROJECT.md doesn't position Shifter as a codec dev tool).
- **D-26:** **Test-runner JS execution = local `goja` (github.com/dop251/goja) runtime in Shifter process.** NOT a round-trip to ChirpStack, NOT a fake-uplink-via-MQTT approach. Add `goja` as a Go dependency (~3MB, mature, ES2020+ supported). Operator hits "Test" → backend evaluates `decodeUplink({bytes: parsedHex, fPort: <fPort>})` inside a goja runtime → returns the decoded JSON + canonical mapping. Fast (<50ms), no ChirpStack dependency for test path, works completely in-process.
- **D-27:** **Test-runner works even when `codec_js_synced_at IS NULL`.** Because D-26 runs codec_js locally via goja, freshly imported profiles (not yet pushed to ChirpStack) can be tested immediately. Operators can validate a codec before pushing to ChirpStack — the workflow becomes "import → test → adjust → push." Removes the "sync first to test" friction.
- **D-28:** **Test-runner failure UX = inline error panel with line/col from goja stack trace.** When codec throws (SyntaxError, TypeError, RangeError) or returns malformed shape: left panel shows error type + message + line/column from goja's stack + original hex; right panel grays out. Operator can copy the stack trace to share with codec-author. Falls back to "decode failed: <message>" if the codec's own try/catch surfaced the error.

### C. Anomaly threshold tuning (ALERT-04 maturation)

- **D-09:** **Ship better defaults; no operator-facing tuning UI.** Phase 6 already ships the three statistical rules (`anomaly_p95`, `anomaly_iqr`, `anomaly_quiet_hour`) with default parameters. Phase 7 refines those defaults — *in Go code* — based on the empirical signal observed during the first paying customer's first 30+ days. The refined defaults ship as part of the Phase 7 binary release. Operators don't tune; they accept the curated defaults. Aligns with PROJECT.md's self-hosted no-ops product posture.
- **D-10:** **"Test against last 30 days" backtest button on the rule-enable surface.** When operator is about to enable an anomaly rule on a metering point (or in the Settings → Alerts rule library), a "Test against last 30 days" button runs the rule's evaluator over the last 30 days of `cagg_hourly` (or raw measurement for instantaneous) and reports the metric defined in D-35. Operator confidence builder — they see whether the shipped defaults are noisy for *their* install before flipping the switch. Read-only; does not write alert rows.
- **D-11:** **Cold-start visibility unchanged from Phase 6 D-16.** No additions in Phase 7: the MP detail "Anomaly detection — warming up, N days remaining" card + Settings → Alerts warmup roster are already shipped. Phase 7 does not add a "why this MP not firing" debugger surface or a dashboard "Anomaly coverage" tile (both deferred — could go in V2 if real-customer feedback surfaces the need).
- **D-35:** **Backtest result format (D-10 specifics) = single count + daily sparkline.** Result panel shows "Would have fired N times in the last 30 days." + a 30-bar sparkline below showing fires-per-day distribution. Operator sees both noise level and clustering (12 fires on one day vs evenly spread). No histogram, no timestamp list. Lightweight UI, high signal.
- **D-41:** **Catalog metadata adds `expected_uplink_interval_seconds` (per profile). ALERT-03 + ALERT-04 refactor to profile-aware.** Field is required on every catalog entry. Examples: Axioma W1 = 3600 (1h), Acrel ADL200/ADW300 = 900 (15min), Itron+KINMY LoRa = 86400 (24h). ALERT-03 offline detection reads this from the bound device_profile (via DevEUI → device → profile join) rather than a global default. ALERT-04 anomaly evaluators use this to scale warmup window length (D-42).
- **D-42:** **Catalog metadata adds `anomaly_compatibility` enum: `full | limited | unsupported`.** Drives which Phase 6 D-17 rules apply to each profile. `full` (Axioma, Acrel — frequent uplinks): all 3 rules (p95, iqr, quiet_hour) available with default warmup. `limited` (Itron+KINMY — daily uplinks): `anomaly_p95` + `anomaly_iqr` available with **extended warmup window 60-90 days**, `anomaly_quiet_hour` hidden/disabled (no concept of quiet hour at 1 uplink/day). `unsupported`: no anomaly rules apply (on-demand meters). Settings → Alerts UI **hides** rules that the bound profile doesn't support — operator never sees a rule they can't usefully enable.
- **D-43:** **Catalog metadata adds `offline_threshold_multiplier` (per profile).** ALERT-03 threshold = `expected_uplink_interval_seconds × offline_threshold_multiplier`. Recommended values: Axioma W1 = 3.0 (3h tolerance), Acrel ADL200/ADW300 = 2.0 (30min), Itron+KINMY = 1.8 (≈43h — pragmatic between 36h and 48h for daily uplinks with random staggered factory schedules). No operator-facing tuning UI in Phase 7; per-MP override deferred to Phase 8 (Out of scope above).
- **D-44:** **Catalog metadata adds `battery_curve` enum + `normalize.go` battery-curve registry.** Curves: `linear_pct` (codec already outputs 0-100, pass-through), `li_socl2_3v6` (3.6V = 100%, 3.2V = 50%, 2.8V = 0%, non-linear plateau-then-cliff matching Li-SOCl2 cells), `li_mnox_3v0` (3.0V nominal), `alkaline_3v0`, `none` (no battery reporting). For voltage-reporting vendors (Itron+KINMY → `li_socl2_3v6`), `normalize.go` reads `data.battery_v` from decoded JSON, applies the curve, writes canonical `battery_pct` to `measurement` row. The raw voltage is preserved in `measurement.raw.battery_v` for debug. Axioma + Acrel (existing battery_pct-reporting) → `linear_pct`. ALERT-02 (battery low) keeps using canonical `battery_pct` — no breaking change.

### D. Install validation hardening

- **D-12:** **Probes ship as `shifter doctor` subcommands**, not a binary-start probe, not a periodic River cron, not both-doctor-and-periodic. Phase 6 06-11 already establishes `shifter doctor` — Phase 7 adds: `shifter doctor probe-chirpstack` (version + reachability), `shifter doctor probe-timescale` (extension present + version), `shifter doctor probe-region` (gateway region matches install identity region/sub-plan). Single CLI surface; operator-invoked when something feels wrong. Tradeoff: drift between binary install and customer's environment is detected only when operator runs `shifter doctor`; the alternative "ship periodic probe + banner" is deferred to V2 (kept as research note in `<specifics>`).
- **D-13:** **Probe failure = warn (banner + `/health/detailed` row), do not block install.** Failed probes log a warning, surface on the shell banner ("ChirpStack version 4.8 detected — Shifter tested against 4.10+. Continue with caution."), and add a row to `/health/detailed`. The probes do NOT cause `shifter serve` to refuse to start, even for "critical" mismatches like Postgres < 14. Reasoning: operator controls the deploy; we surface drift but never gate. Lockout escape: if Postgres really is incompatible, the binary will fail on its own at first migration / first query — we don't need a probe to enforce that.
- **D-40:** **Probe results retention in `/health/detailed` = last-run only (no history array).** Each probe writes its latest result with `last_run_at` timestamp; running again overwrites. No N-day history retained. Aligns with Phase 7's "manual `shifter doctor` invocation only" stance (D-12) — operator pulls when needed; the latest result is what matters for "is the install healthy right now."

### E. V2-VEND-03 (saved templates + side-by-side comparison + bulk gateway import)

- **D-14:** **Saved report templates capture: scope + range preset + sites/meters filter + group-by + capability filter.** A template = the full state of ReportConfigPanel that produces a deterministic report. Operator clicks "Save as template" → names it ("Monthly per-site Building A") → reloads later from "Templates" dropdown → re-runs with one click. Download format (CSV/Excel/PDF) is NOT part of the template — operator picks at run time (a template can be downloaded in multiple formats).
- **D-15:** **Templates are install-wide (shared across all users).** No per-user templates. Aligns with single-tenant operator-team install posture established in Phase 6 D-10 (shared alert inbox) and D-29 (shared user table). Audit row on template create/edit/delete; admin and viewer can both run templates; only admin can edit/delete.
- **D-16:** **Side-by-side comparison shows exactly 2 entities (A vs B).** Two-column layout: scope chip + range + delta + chart (2 series overlaid) + table (2 rows). No "compare 3-4" or "compare N." Fits mobile portrait. Operator use case is "audit this site against the install average" — two-entity is sufficient. Comparison entities can be: two sites, two metering points, or one entity over two time ranges (year-over-year).
- **D-17:** **Bulk gateway import reuses the Phase 3 device-import CSV pattern fully.** Same modal flow: upload CSV → dry-run validation → confirm → commit. Same re-runnable-safe semantics (idempotent on `gateway_eui`). Same audit-row-per-imported-row. Same error-row UI. CSV columns include lat/lng (no separate "pick on map" step — operator pre-fills lat/lng in spreadsheet). If lat/lng are missing/null, gateway is created without map placement; operator can pick on map later via Phase 5 D-15 GW-04 picker.
- **D-37:** **Comparison entity picker = two dropdowns at top of compare view.** Compare view header has two searchable dropdowns: "Entity A" + "Entity B". Each dropdown filtered by entity type (sites OR metering points; can't mix types in one comparison). Swap-A↔B button to flip. Time-range picker shared between A and B. Simplest UX, mobile-friendly, follows shadcn Select + Command (search) pattern.
- **D-38:** **YoY mode toggle: "Compare entities" vs "Compare time ranges".** Radio at top of compare view. "Compare entities" → two entity dropdowns + single time range. "Compare time ranges" → single entity dropdown + two time-range pickers (A range + B range). Clear mode separation; discoverable; one URL/route covers both.
- **D-39:** **Template listing UX = alphabetical with search box.** Templates dropdown shows alphabetical list with a top search box. Operator types to filter. Predictable order regardless of edit history. No "recently used" or category grouping — those are premature for a single-tenant operator team with realistically <30 templates. Phase 8 may add MRU sort if usage data justifies.
- **D-39b (= D-40 above for Update notification surface):** **Catalog "Update available" notification = Settings-only badge** (count on the tab label, e.g., "Vendor Catalog (3)"). No top-of-app banner, no sidebar nav badge. Operator-pull model — they visit Settings when they want to manage profiles. Matches PROJECT.md self-hosted no-ops posture; avoids notification fatigue.

### F. Vendor mapping expansion

- **D-18:** **Ship 7 vendor profiles in the Phase 7 binary** as per ROADMAP SC#1: Kamstrup MULTICAL (water), Diehl (water), **Itron + KINMY LoRa Module (water — 3rd-party module on Itron meter)**, Axioma Qalcosonic W1 (water — already wired since Phase 2; Phase 7 promotes from "one wired profile" to "catalog entry with metadata"), Acrel ADL200 + ADW300 (electricity — already wired since Phase 2; same promotion as Axioma), Sagemcom (electricity), Schneider IEM3xxx (electricity). Each profile ships with: codec (D-19), canonical mapping, default battery_curve (D-44), expected_uplink_interval_seconds (D-41), offline_threshold_multiplier (D-43), anomaly_compatibility (D-42), and vendor name + model + family + capability tag. **JS codecs to author for Phase 7 (existing JS files in `internal/profile/codecs/`):** ✓ `axioma_w1.js`, ✓ `acrel_family.js`, ✓ `itron_kinmy_lora.js` (operator-supplied, adapted to Phase 2 convention 2026-05-12); **to write:** Kamstrup MULTICAL, Diehl, Sagemcom, Schneider — 4 new codecs.
- **D-19:** **(CORRECTED 2026-05-12) Codec source = JavaScript files in `internal/profile/codecs/{slug}.js`, bundled via `//go:embed`, pushed to ChirpStack v4 for execution in its QuickJS sandbox.** Originally written as "embedded Go functions" — that was wrong; the Phase 2 substrate (DATA-09, Pitfalls §3, `internal/chirpstack/device_profile.go: CreateDeviceProfile pushes a new profile to ChirpStack with a QuickJS codec`) already commits to JS-pushed-to-ChirpStack. Phase 7 extends this pattern; does NOT rewrite to Go decoders. Each catalog entry's JSON metadata carries a `codec_js_path` field pointing to the `.js` file. Compile-time embed check via `embed.go` (current pattern). Codec runtime sandbox = ChirpStack's QuickJS for production uplinks; local goja for test-runner (D-26) — both run the same JS source so behavior is consistent.
- **D-20:** **New-vendor onboarding = PR workflow (JS codec + catalog JSON entry + canonical mapping fixture) → binary release.** A new vendor onboarding in v1.x is: contributor adds `internal/profile/codecs/{slug}.js` + `internal/codec/catalog/{slug}.json` + test fixtures (sample hex payload + expected decoded JSON + expected canonical mapping) → PR → review → merge → binary release → customer upgrades binary → catalog tab shows new vendor as "Available, click Install." Admin-UI "Add custom vendor" path is deferred to V2 (requires either a JS sandbox runtime *in Shifter*, or operator-editable codec stored in DB and pushed to ChirpStack — neither is in v1 scope).

### G. Catalog file format + validation

- **D-21:** **Catalog format = JSON** (not YAML, not Go struct literals). Files at `internal/codec/catalog/*.json`. Decoded via stdlib `encoding/json`. Tool-friendly (every editor highlights JSON), PR-diff-friendly, no extra dependency. Verbosity tradeoff accepted (catalog entries are short — vendor + model + version + ~10 metadata fields).
- **D-22:** **Schema validation via `go test` at build time** (not JSON Schema lib, not runtime-only). A `TestCatalogValid` test in `internal/codec/catalog_test.go` decodes every embedded JSON file, unmarshals into a typed Go struct, and asserts required fields + enum values + EUI prefix format + capability tag validity + `codec_js_path` resolves to an existing embedded JS file + `battery_curve` is a known curve + `anomaly_compatibility` is a valid enum. CI fails the build if catalog is invalid — production binary cannot ship with a broken catalog. No runtime JSON-Schema library dep needed.
- **D-33:** **Catalog JSON schema (locked).** Required fields per entry: `slug` (lower, unique), `name` (display), `vendor` (brand), `family` (sub-line / module brand), `capabilities` (subset of Phase 2 capability vocab), `version` (semver), `codec_js_path` (relative path under `internal/profile/codecs/`), `counter_modulus`, `mac_version`, `region` (nullable — inherit from install if NULL), `expected_uplink_interval_seconds` (D-41), `offline_threshold_multiplier` (D-43), `anomaly_compatibility` (D-42), `battery_curve` (D-44), `vendor_has_separate_meter_serial` (bool — D-45). No icon/logo/long-description fields in v1; the table UI (D-23) renders text-only. v1.1 may add display metadata if operators request richer browsing.

### H. Settings → Vendor Catalog tab UI

- **D-23:** **Catalog tab UI = table layout** (not card grid, not categorized-by-capability sections). Columns: Vendor | Family | Capability | Version | Installed | Devices using | Status. Status enum: `not-installed` | `installed` | `update-available`. Sortable by any column; filter by capability via chip row. Mirrors Phase 6 D-29 settings-table convention; works on mobile via shadcn DataTable.
- **D-24:** **Update flow = per-row "Update available v2.1.0 → v2.3.0" badge + Update button → confirm dialog with diff modal (layout D-34).** Clicking Update on a row where embedded > installed: dialog shows side-by-side diff of changed fields (D-34). Operator confirms which fields to merge into the customer's device_profile row. Customer-edited fields are flagged in the diff with "you edited this — keep yours?" toggle. Preserves D-04 editability.
- **D-25:** **Show "Devices using" count per profile row.** Column displays N (where N = active devices linked to this profile). Click N → navigates to Devices list pre-filtered by `profile_id`. Helps operator decide whether to disable an unused profile (if N=0, the action is safe). Mirrors Phase 3 gateway-usage pattern.
- **D-34:** **Diff modal layout = field-by-field rows with side-by-side values.** Each changed field gets one row: `field name | current value | catalog v{X.Y.Z} value | toggle ("use mine" / "use catalog") | flag ("you edited this" if customer_edited=true)`. Operator sees all changes at once, picks per-field. Matches "git mergetool" mental model. NOT a unified-diff text format (poor fit for structured fields like `capabilities` array or `offline_threshold_multiplier` numeric), NOT a pre-populated form (loses the "this changed" signal). Default toggle state: "use catalog" for non-customer-edited fields, "use mine" for customer_edited fields — operator can flip per row.
- **D-36:** **Codec re-sync trigger on catalog Update = auto-clear `codec_js_synced_at` to NULL.** When operator clicks Update and the merged result has a new `codec_js`, migration writes the new JS to the row and sets `codec_js_synced_at = NULL`. The existing Phase 2 seed routine (`internal/profile/seed.go`) already polls for unsynced profiles and re-pushes to ChirpStack — zero new sync code needed. UI shows "Syncing to ChirpStack…" badge until the seed routine updates the timestamp. No second confirmation dialog (D-36 explicitly rejects "click Update, then click Push" — single-action flow).

### I. Existing-seed reconciliation (Phase 2 → Phase 7 catalog)

- **D-29:** **Backfill `catalog_source` + `catalog_source_version` + `customer_edited` on existing 3 seed rows.** Phase 7 migration (likely `0050_*`) ALTERs `device_profile` to add three columns (text, text, bool). Backfill statement: for `slug IN ('axioma_w1', 'acrel_adl200', 'acrel_adw300')`, set `catalog_source = slug`, `catalog_source_version = '1.0.0'`, `customer_edited = drift_check_result`. Future catalog Updates work uniformly for legacy + new rows. Clean — no "legacy" vs "catalog" rows mental split.
- **D-30:** **Backfill version for existing seeds = `1.0.0`.** Catalog ships at v1.0.0 initially across the board. Existing seeds match what's shipped — no "update available" badge on day 1. Update flow kicks in only when a future binary bumps a catalog entry to 1.0.1+. Avoids day-1 notification noise for operators upgrading from Phase 6 to Phase 7.
- **D-31:** **Drift detection on migration: if existing `codec_js` ≠ embedded JS, set `customer_edited = true`, preserve operator's JS.** Migration computes a hash of the row's `codec_js` and the corresponding embedded `*.js` file. If they differ → operator tweaked the codec in the profile editor (Phase 2 D-09 supports this) → preserve their JS and flag them as customer-edited. Future catalog Update will show the diff with "you edited this" toggle per field (D-34) including codec_js. Safe — no silent overwrite.
- **D-32:** **Shared-codec representation: two catalog entries reference same `codec_js_path`.** Acrel pattern: `acrel_adl200.json` and `acrel_adw300.json` both have `codec_js_path: "acrel_family.js"`. Two distinct catalog rows in the UI (operator sees both as separate vendor-model entries), one shared codec file. Matches Phase 2 Pitfall 6 already in `embed.go` (CodecBySlug routes both slugs to AcrelFamily). Catalog validation (D-22 / D-33) does NOT require codec_js_path uniqueness.

### J. Itron + KINMY LoRa Module specifics (operator-supplied seed)

- **D-45:** **Catalog metadata adds `vendor_has_separate_meter_serial: bool` flag.** True for Itron+KINMY (the 3rd-party module sends a `meter_id` field separate from the LoRa DevEUI). False for all-in-one meters (Axioma, Acrel). Phase 7 keeps `meter_id` in `measurement.raw->>'meter_id'` only — no canonical column. The flag is a forward-marker for Phase 8/v1.1 to consume when promoting `meter_id` to canonical (cross-vendor meter-swap detection at meter-serial level). MP detail page Phase 4 advanced tab uses `jsonb_each` and **auto-surfaces** `meter_id` without UI work.
- **D-46:** **Reverse-flow handling = `raw.reverse_flow_m3` + new delta-based alert rule `reverse_flow_increase`.** Codecs that emit `reverse_flow_m3` (Itron+KINMY does; Axioma uses a single `reverse_flow` boolean bit, not cumulative) write the cumulative value to `measurement.raw->>'reverse_flow_m3'`. New ALERT-04 rule `reverse_flow_increase` evaluator: fires when `reverse_flow_now - reverse_flow_N_days_ago > threshold_m3` over an operator-configurable window. Delta-based, not absolute — handles the "reverse_flow never resets after first event" semantic correctly (D-46 confirmation from operator). No canonical column for reverse_flow in v1.
- **D-47:** **Meter clock fields (`meter_date`, `meter_time`) = `raw` JSONB only.** Codec decodes meter's internal clock into `meter_date` + `meter_time` strings. `normalize.go` writes them to `measurement.raw->>'meter_date'` and `measurement.raw->>'meter_time'`. NOT used as canonical timestamp (server timestamp from ChirpStack is authoritative — meter clocks drift). Phase 4 advanced tab auto-surfaces. Phase 8 may add a clock-drift diagnostic alert (compare meter timestamp vs server timestamp at ingest); v1 ships raw data only.
- **D-48:** **Itron+KINMY catalog entry (concrete v1.0.0 seed values).** Catalog JSON:
  - `slug: "itron_kinmy_lora"`
  - `name: "Itron Water Meter (KINMY LoRa Module)"`
  - `vendor: "Itron"`
  - `family: "KINMY LoRa Module"`
  - `capabilities: ["cumulative", "leak_detection", "tamper_detection", "battery"]`
  - `version: "1.0.0"`
  - `codec_js_path: "itron_kinmy_lora.js"`
  - `counter_modulus: 4294967296` (raw 32-bit; codec divides by 1000 → m³)
  - `mac_version: "LORAWAN_1_0_3"`
  - `region: null` (inherit install region)
  - `expected_uplink_interval_seconds: 86400` (24h default; configurable per device in KINMY vendor app — operator may run more frequent)
  - `offline_threshold_multiplier: 1.8` (≈43h — pragmatic for random-staggered daily uplinks)
  - `anomaly_compatibility: "limited"` (p95 + iqr available with extended 60-day warmup; quiet_hour disabled — incompatible at 1 uplink/day)
  - `battery_curve: "li_socl2_3v6"` (Li-SOCl2 3.6V cell; non-linear plateau curve)
  - `vendor_has_separate_meter_serial: true` (codec emits 7-byte `meter_id`)
  - `fPort: null` (codec accepts any fPort; SOF byte 0x6F is the frame filter — operator decision: "รับทุก fPort")

### Claude's Discretion

Items remaining at Claude's discretion during planning (after promotion of D-33/D-34/D-35/D-36):

- Migration ordering — which Phase 7 migration adds the new `device_profile` columns (catalog_source, catalog_source_version, customer_edited) and which adds the seed backfill (single migration or two)
- `report_template` table schema (Postgres column list); template state serialization format (JSON blob vs structured columns)
- Exact placement of the "Test against last 30 days" backtest button (inside rule-enable dialog vs separate "Test" CTA on the rule library row)
- goja sandbox limits — time/memory caps for the test-runner code execution (default to 100ms timeout + 16MB memory unless planning research shows different defaults appropriate)
- Catalog `TestCatalogValid` test layout — table-driven vs per-file subtests
- Mobile layout of the compare view (D-37/D-38) — 2 stacked entity cards vs collapsed dropdown rows
- The naming of the new alert rule kind constant for `reverse_flow_increase` (likely `alert.RuleKind.ReverseFlowIncrease`)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### ROADMAP and requirements
- `.planning/ROADMAP.md` §Phase 7 — Goal, depends_on, requirements, success criteria, UI hint
- `.planning/REQUIREMENTS.md` — V2-VEND-01, V2-VEND-02, V2-VEND-03 entries + ALERT-04 tuning context

### Prior phase decisions Phase 7 builds on
- `.planning/phases/02-domain-model-canonical-schema/02-CONTEXT.md` — vendor-agnostic measurement model (wide canonical + JSONB raw); D-07/D-09 wired profile pattern; codec_js in `device_profile` column; codec push to ChirpStack via gRPC; Pitfalls §3 (codec runs in ChirpStack QuickJS, not Shifter)
- `.planning/phases/03-provisioning-gateways-devices-bulk-import/03-CONTEXT.md` — CSV bulk-import pattern (dry-run → confirm → commit, re-runnable safe, audit-per-row)
- `.planning/phases/05-aggregates-reports-map-floor-plans/05-CONTEXT.md` — ReportConfigPanel state shape (scope + range + filters + group-by) that templates serialize; Settings → Data Retention card pattern that Vendor Catalog tab mirrors; GW-04 map picker for post-bulk-import lat/lng tweaks
- `.planning/phases/06-alerts-users-audit-operational-hardening/06-CONTEXT.md` §D-16, §D-17 — cold-start gate substrate; 3 statistical rule kinds Phase 7 tunes defaults for; D-29 Settings tab pattern; `shifter doctor` substrate Phase 7 extends; ALERT-03 offline detection that Phase 7 refactors to profile-aware

### Project-level
- `.planning/PROJECT.md` — Self-hosted single-binary per-customer install; tech stack (Go embed, TimescaleDB, shadcn/ui); no SMTP / no SSO / no live device control in v1; modal-first CRUD; blue/navy aesthetic
- `CLAUDE.md` (project root) — Go 1.24+, pgx/v5 + sqlc, alexedwards/scs sessions, golang-migrate plain-SQL migrations, **goja for JS evaluation** (now added by D-26)

### Existing codec substrate (read before extending)
- `internal/profile/codecs/embed.go` — `//go:embed` pattern + `CodecBySlug` switch; Phase 7 catalog extends this
- `internal/profile/codecs/axioma_w1.js` — Phase 2 codec convention: ES5 syntax, `{data, errors, warnings}` return shape, defensive length check, try/catch wrapper, helper functions at module level
- `internal/profile/codecs/acrel_family.js` — Same convention; shared-codec pattern (Pitfall 6); register-pair dispatch
- `internal/profile/codecs/itron_kinmy_lora.js` — **Operator-supplied, Phase 7 seed (added 2026-05-12).** Itron water meter + KINMY 3rd-party LoRa module; 28-byte fixed payload with SOF=0x6F filter; outputs `forward_flow_m3` + `reverse_flow_m3` + `tamper` + `leak` + `battery_v` (voltage, needs `battery_curve` conversion per D-44) + `meter_id` (7-byte hex, raw-only per D-45) + `meter_date` + `meter_time` (raw-only per D-47)
- `internal/profile/seed.go` — Boot-time seed routine that reads embedded JS, pushes to ChirpStack via gRPC, writes `cs_profile_id` + `codec_js_synced_at`; Phase 7 D-36 reuses this for catalog Update re-sync

### Existing ingest + normalize substrate
- `internal/chirpstack/device_profile.go` — `CreateDeviceProfile pushes a new profile to ChirpStack with a QuickJS codec` (codec runtime confirmation)
- `internal/db/migrations/0009_device_profile.up.sql` — `device_profile` table schema including `codec_js TEXT`, `cs_profile_id UUID`, `codec_js_synced_at TIMESTAMPTZ`
- `internal/db/migrations/0010_seed_profiles.up.sql` — 3 existing seed profile rows; Phase 7 D-29 backfills these with catalog_source on migration
- `internal/db/queries/device_profiles.sql` — sqlc query for codec_js sync state (`SetProfileCodec`, `MarkProfileSyncedToChirpStack`); Phase 7 D-36 uses same queries

### Existing alert substrate
- `internal/alert/anomaly_worker.go` — Phase 6 anomaly evaluator; Phase 7 D-41/D-42 refactor to read profile-aware thresholds
- `internal/alert/cold_start.go` — Phase 6 D-16 warmup gate; Phase 7 D-42 extends warmup window for `anomaly_compatibility: limited` profiles
- `internal/alert/offline_worker.go` — Phase 6 ALERT-03 evaluator; Phase 7 D-41/D-43 refactor to read `expected_uplink_interval_seconds × offline_threshold_multiplier` from bound profile

### External vendor / library references
- TimescaleDB CAGG docs §`time_bucket` + `cagg_hourly` query patterns — backtest evaluator runs against `cagg_hourly` for hourly anomaly rules
- ChirpStack v4 device profile API — Phase 3 already wired; Phase 7 catalog import does NOT push to ChirpStack from import time (D-36 background sync via Phase 2 seed routine)
- **github.com/dop251/goja** — JS runtime for D-26 test-runner; ES2020+; pure Go, ~3MB, no CGO
- React Hook Form + zod schema validation — used for template save/load form (Phase 5 pattern)
- shadcn DataTable + Command (search) — used for Vendor Catalog table (D-23) and Compare entity picker (D-37)

### No external ADRs
No formal `docs/decisions/` ADRs exist for this phase — all decisions captured here. CONTEXT.md IS the contract; downstream planner should treat each D-NN as locked unless flagged Claude's Discretion.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets

- **`internal/profile/codecs/`** (already exists since Phase 2) — `//go:embed *.js` pattern + `CodecBySlug(slug) string` dispatch. Phase 7 update (2026-05-12) added `itron_kinmy_lora.js` + `ItronKinmyLoRa` embed + slug case. Future Phase 7 vendors (Kamstrup, Diehl, Sagemcom, Schneider) add JS file + embed var + switch case here.
- **`internal/profile/seed.go`** — boot-time codec push to ChirpStack. Phase 7 reuses for catalog Update re-sync (D-36) — clearing `codec_js_synced_at = NULL` triggers re-push, no new sync code.
- **Phase 2 Axioma + Acrel codecs** — codec convention reference (ES5, `{data, errors, warnings}`, defensive length check, try/catch). Phase 7's new codecs (Kamstrup, Diehl, Sagemcom, Schneider) follow this convention; Itron+KINMY already adapted to it.
- **`internal/report/assembler.go` + ReportConfigPanel** — Phase 5 wraps reports. Phase 7 adds `report_template` table + `templateService.Save/Load/List/Delete` + a "Templates" dropdown in ReportConfigPanel.
- **`internal/import/`** (csv/xlsx parser + dryrun + commit + audit) — Phase 3 device-import pattern is the template for Phase 7's bulk gateway import (D-17). Reuse `parser_csv.go` + `dryrun.go` + `commit.go` lifecycle + `audit_log` row-per-import. Wrap with new `internal/gateway/import.go`.
- **`internal/alert/anomaly_worker.go` + `cold_start.go` + `quiet_hour.go`** — Phase 6's three anomaly evaluators. Phase 7 changes ONLY the default constants (P95 percentile threshold, IQR multiplier, quiet-hour bounds) — no signature changes, no logic changes. Add `BacktestRun(ctx, rule, mp_id, days)` helper for the D-10 / D-35 "Test against last 30 days" button.
- **`internal/alert/offline_worker.go`** — Phase 6 ALERT-03. Phase 7 refactor: read `expected_uplink_interval_seconds × offline_threshold_multiplier` from the bound device_profile (via DevEUI → device → profile join) instead of global constant.
- **`internal/cli/doctor.go` + `internal/doctor/doctor.go`** — `shifter doctor` Cobra subcommand. Phase 7 adds three more subcommands inside the same package: `doctor probe-chirpstack`, `doctor probe-timescale`, `doctor probe-region`. Reuse output format + exit-code conventions.
- **`web/src/routes/settings/`** — Settings page with tab strip (Phase 5 D-09, Phase 6 D-29). Phase 7 adds `VendorCatalogCard.tsx` + `ImportFromCatalogDialog.tsx` + `UpdateCatalogDialog.tsx` (the diff modal).
- **`web/src/routes/reports/ReportConfigPanel.tsx`** — Phase 5 09. Phase 7 wraps with `SaveTemplateButton` + `TemplatesDropdown` + persists state via React Hook Form integration already in place.
- **`web/src/routes/metering-point/AdvancedTab.tsx`** (Phase 4 DETL-01) — `jsonb_each` auto-surfaces `raw` fields. Phase 7's `raw.meter_id`, `raw.reverse_flow_m3`, `raw.meter_date`, `raw.meter_time`, `raw.battery_v` (Itron+KINMY) ALL appear here for free.

### Established Patterns

- **Settings tab pattern** (Phase 5 D-09 / Phase 6 D-29): card layout, table inside, dialogs for CRUD, audit-row-on-mutate. Vendor Catalog tab follows this.
- **`go:embed` for static assets** (Phase 1 D-23 — install wizard assets; Phase 2 — codec JS): Phase 7 catalog JSON files use the same pattern: `//go:embed catalog/*.json var catalogFS embed.FS`.
- **Cobra CLI subcommand convention** (Phase 1 D-12, Phase 6 06-11): each command has `Use`, `Short`, `Long`, `RunE`; output to stdout in human-readable format with `--json` flag for machine output (Phase 1 D-15 standard).
- **Audit-row-in-tx pattern** (Phase 2 D-21/D-22, AUDIT-01): every catalog Import / Update / Disable writes an `audit_log` row in the same Postgres tx as the mutation. Phase 7 catalog actions follow this; new vocab: `catalog.profile.imported`, `catalog.profile.updated`, `report_template.created`, `report_template.deleted`, `gateway.bulk_imported`.
- **React Hook Form + zod** (Phase 5 D-09): used for Template save dialog, Import from Catalog form, Diff modal field-toggle form.

### Integration Points

- **`device_profile` table** (Phase 2 schema) — Phase 7 ALTERs to add `catalog_source` (text, FK by slug to catalog entry), `catalog_source_version` (text, semver of catalog entry at import time), `customer_edited` (bool, set true when operator modifies any field after import OR when D-31 drift detected). Migration `0050_*` (or next available).
- **`device_profile` table — battery_curve column** — Phase 7 ALTERs to add `battery_curve` (text, enum-constrained). Backfill: Axioma `linear_pct`, Acrel `linear_pct`. Read by `normalize.go` at ingest.
- **`audit_log` table** (Phase 2 / Phase 6) — Phase 7 adds new `Action` constants: `catalog.profile.imported`, `catalog.profile.updated`, `catalog.profile.codec_resynced`, `report_template.created`, `report_template.updated`, `report_template.deleted`, `gateway.bulk_imported`, `alert.rule.reverse_flow_increase.fired`.
- **`/health/detailed` endpoint** (Phase 1 D-19, Phase 6 D-21) — Phase 7 adds `probe_results` block with last-run timestamp + result per probe + drift count (D-40 last-run only, no array history). Drives the shell banner referenced in D-13.
- **`router.go`** — Phase 7 mounts `/api/catalog/*` (list, import, update endpoints), `/api/reports/templates/*` (CRUD), `/api/reports/compare` (D-37/D-38 2-entity comparison), `/api/profiles/{id}/test-codec` (D-26 hex → JSON via goja), and extends `/api/gateways/*` with `/bulk-import` (CSV upload).
- **Phase 5 PDF worker (River queue)** — saved templates that produce PDFs reuse the existing River worker; no new queue needed.
- **`normalize.go` battery_curve registry** — Phase 7 new module. Read `device_profile.battery_curve` for the ingesting device, apply curve function to `data.battery_v` (if present) → write canonical `measurement.battery_pct`. Curve table is a Go switch/map keyed by enum value.

</code_context>

<specifics>
## Specific Ideas

- **"Ship better defaults" approach for anomaly tuning** is borrowed from Linear's anomaly-detection ergonomics — defaults that work for 90% of users out of the box, and the operator who needs more control runs a CLI flag (deferred to V2). Linear specifically does NOT expose every threshold in the UI because tuning thresholds is a job for the product, not the customer.
- **"Update available" UX in Settings → Vendor Catalog** is shaped after VS Code's extensions tab — a clear badge, a one-click Update button, a diff view before the merge. Familiar pattern from operator's developer-tool muscle memory.
- **Per-profile semver** (not catalog-level semver) follows the design of Homebrew's Formulary and Helm chart versioning — each profile is independently versioned because vendor codec changes are uncorrelated (Kamstrup's MULTICAL firmware update is unrelated to Acrel's tariff schedule update).
- **Side-by-side 2-entity comparison** matches the "compare diff" muscle memory operators have from GitHub PR diff view — two columns, aligned rows.
- **Saved templates as install-wide shared resources** matches Linear's "shared views" concept and avoids the per-user fragmentation that makes Notion's "saved filters" so confusing in multi-operator setups.
- **goja for test-runner** — chosen over rolling a fake-uplink-via-MQTT round-trip because goja runs the exact same JS that ChirpStack's QuickJS runs (both are ES2020-compatible JS engines). Discrepancies in production would be QuickJS-vs-goja runtime differences — same risk as any cross-engine JS code. In practice, deterministic JS (no Date.now, no Math.random, no async) behaves identically; codec authors are advised to write deterministic codecs.
- **Itron+KINMY architecture (3rd-party module + meter brand)** is the first catalog entry where vendor (meter brand "Itron") and family (module brand "KINMY LoRa Module") differ — i.e., the same KINMY module could in the future be paired with Diehl or Sensus meters, producing a different catalog slug (`diehl_kinmy_lora`, `sensus_kinmy_lora`) because the wire format depends on the meter, not just the module. The `vendor_has_separate_meter_serial: true` flag and `meter_id` raw-JSONB pattern (D-45) anticipate this — future phases can promote meter_id to canonical once enough vendors emit it.
- **Itron daily uplink + random staggering** drives the entire D-41..D-44 profile-aware refactor scope. Without it, Phase 7 could have kept global ALERT-03/04 defaults; with it, the architecture must be profile-aware or Itron customers see alert spam.

### Research-noted alternatives (not chosen but worth re-evaluating if Phase 7 misses)

- **JS sandbox codec runtime in Shifter (operator-supplied codecs in DB)** — rejected as D-19/D-20. Re-evaluate if v2 customer adds a vendor not on our roadmap and demands self-service onboarding. goja is already in the binary for the test-runner (D-26), so the runtime cost is already paid — only the storage + UI + sandbox-hardening work would be added. **lower than originally estimated cost** because of the goja dep added in this update pass.
- **Periodic install-probe via River cron** (rejected as D-12) — re-evaluate if v1.x customers report environments drifting silently between install and first incident; the manual `shifter doctor` cadence is fragile in that case.
- **Operator-facing anomaly tuning UI** (rejected as D-09) — re-evaluate if real-customer feedback shows the shipped defaults are systemically wrong for a class of meter (e.g., commercial water vs residential water have different anomaly profiles).
- **Per-MP override of offline / anomaly thresholds** (deferred to Phase 8) — re-evaluate if operators report that some specific MPs need different thresholds than the catalog default (e.g., a high-throughput commercial water meter that legitimately spikes vs residential).
- **Promote `meter_id` to canonical column for cross-vendor meter-swap detection** (deferred to Phase 8 — D-45) — re-evaluate after Phase 7 ships and 3+ vendors emit meter_id, so the canonical column design has multi-vendor evidence.
- **Clock-drift diagnostic alert (D-47)** — Phase 8 candidate. Compare `raw.meter_date`+`raw.meter_time` vs server timestamp at ingest; fire warning if drift > N hours. Requires Phase 7 D-47 raw storage.

</specifics>

<deferred>
## Deferred Ideas

These came up during discussion but explicitly belong outside Phase 7 scope:

- **JS sandbox codec runtime in DB + admin-UI "Add custom vendor"** — V2 territory. Note: goja is now in the binary (D-26) so the runtime cost is paid; V2 work is storage + sandbox-hardening + admin-UI codec editor + push-to-ChirpStack from DB row.
- **Hot catalog reload without binary restart** — V2 ops feature. Phase 7 requires binary upgrade to ship a new catalog version.
- **Periodic install-probe via River cron + shell banner on drift** — V2 ops hardening. Phase 7 ships only manual `shifter doctor` invocation.
- **Anomaly tuning UI (per-MP / per-vendor / global)** — V2 if real-customer feedback requires. Phase 7 ships better defaults via code refinement (D-09) + profile-aware thresholds (D-41..D-43) only.
- **Per-MP override of `expected_uplink_interval_seconds` / `offline_threshold_multiplier` / anomaly_compatibility** — Phase 8 candidate. Phase 7 sets catalog-driven defaults only.
- **Promote `meter_id` to canonical column (cross-vendor meter-swap detection at meter-serial level)** — Phase 8/v1.1. Phase 7 keeps in raw JSONB + `vendor_has_separate_meter_serial` flag (D-45).
- **Clock-drift diagnostic alert** (compare meter clock vs server timestamp) — Phase 8. Phase 7 stores raw clock in JSONB (D-47).
- **"Why is this MP not firing" explanation surface** on the MP detail page — V2 alert-debug feature. Phase 7 keeps Phase 6 D-16 warmup card as the only cold-start surface.
- **Dashboard "Anomaly coverage" tile** ("74% of MPs eligible for anomaly detection") — V2 dashboard polish.
- **Compare 3+ entities side-by-side** — V2 power-user feature. Phase 7 ships 2-entity comparison only (D-37).
- **Per-user saved templates** — V2 multi-operator workflow. Phase 7 ships install-wide shared templates (D-15).
- **MRU sort / categorization for templates dropdown** — Phase 8 if usage data justifies. Phase 7 ships alphabetical + search (D-39).
- **Catalog "Update available" cross-page banner / sidebar badge** — Phase 8 if operators report missing updates. Phase 7 ships Settings-only badge (D-40).
- **Probe results 7-day history in `/health/detailed`** — Phase 8 if operators want to spot "this probe started failing 3 days ago." Phase 7 ships last-run only (D-40).
- **CSV-with-pick-on-map step for bulk gateway import** — friction-reduction. Phase 7 ships CSV-only (D-17); operator can pick on map after import via Phase 5 GW-04 picker.
- **Save test-runner test cases as profile fixtures** — V2 testing-as-product feature. Phase 7 ships test-runner as a scratch pad (D-08).
- **Diff against expected JSON in test-runner output** — V2 codec-dev feature. Phase 7 ships canonical mapping only (D-07).

</deferred>

---

*Phase: 07-multi-vendor-breadth-v1-x-differentiators*
*Context gathered: 2026-05-12 (initial); updated 2026-05-12 (update pass — D-19 correction + D-26..D-48 added + Itron+KINMY codec seeded)*
