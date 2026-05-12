# Phase 7: Multi-Vendor Breadth & v1.x Differentiators - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in 07-CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-05-12
**Phase:** 07-multi-vendor-breadth-v1-x-differentiators
**Mode:** discuss (interactive)
**Areas discussed (8 total):** Profile catalog, Codec test-runner, Anomaly threshold tuning, Install validation, V2-VEND-03 (templates + comparison + bulk import), Vendor mapping expansion, Catalog file format, Settings → Vendor Catalog tab

---

## Profile catalog

### Q1: Catalog storage location

| Option | Description | Selected |
|--------|-------------|----------|
| Go embed in binary | JSON/YAML embedded via go:embed; atomic with binary upgrade; no external volume | ✓ |
| External JSON/YAML files | `/var/lib/shifter/profiles/*.json`; operator copies file per release; version skew risk | |
| DB seed migration | Migration inserts device_profile rows on install; customer DB can customize after | |
| Hybrid: embed + DB override | Read-only default + customer customize layer; conflict handling needed | |

**User's choice:** Go embed in binary (recommended)
**Notes:** Aligns with PROJECT.md self-hosted single-binary posture. Tradeoff acknowledged: catalog update = binary upgrade.

### Q2: Versioning + update strategy

| Option | Description | Selected |
|--------|-------------|----------|
| Per-profile semver + Settings UI Update | Each profile has version field; Settings → Vendor Catalog shows "Update available"; operator clicks Update | ✓ |
| Auto-bump silent on binary start | Binary start checks catalog vs DB; silent merge | |
| Manual import only | No auto-check; operator clicks "Import from catalog" per profile | |
| Claude decides | — | |

**User's choice:** Per-profile semver + Settings UI Update (recommended)

### Q3: Import flow

| Option | Description | Selected |
|--------|-------------|----------|
| "Add from catalog" button in device-profile dialog | 2 paths: Start blank / Import from catalog; modal-first pattern | ✓ |
| Auto-show catalog in device-profile list (Installed / Catalog tabs) | List bloat + operator may install unused entries | |
| CLI command (`shifter catalog list/install`) | 2 surfaces (UI + CLI) — conflicts with PROJECT.md "UI is primary" | |
| Claude decides | — | |

**User's choice:** "Add from catalog" button in device-profile dialog (recommended)

### Q4: Profile editability after import

| Option | Description | Selected |
|--------|-------------|----------|
| Editable — catalog as seed only | Import copies fields; customer edits survive; catalog updates don't auto-overwrite | ✓ |
| Read-only — catalog as single source of truth | Update binary = update everywhere; customer can't customize for site-specific codec | |
| Read-only with Fork action | Default read-only + Fork button creates editable copy; 2 states to UI | |
| Claude decides | — | |

**User's choice:** Editable — catalog as seed only (recommended)

---

## Codec test-runner

### Q5: Placement

| Option | Description | Selected |
|--------|-------------|----------|
| Inside device-profile editor (tab/section) | Profile context implicit; no need to select profile | ✓ |
| Standalone /test-runner page | Sidebar nav item; explicit profile dropdown; extra nav | |
| Both (modal in editor + standalone) | 2 entry points; UI maintenance overhead | |
| Claude decides | — | |

**User's choice:** Inside device-profile editor (recommended)

### Q6: Input format

| Option | Description | Selected |
|--------|-------------|----------|
| Hex only | Matches vendor doc samples; fPort + payload | ✓ |
| JSON uplink payload from ChirpStack | ChirpStack uplink event JSON; debug production easier | |
| Both (radio toggle) | UI complexity goes up; rarely used JSON path | |
| Pick from real uplinks (dropdown) | "Last 10 uplinks from this device" — vendor docs use case suffers | |

**User's choice:** Hex only (recommended)

### Q7: Output display

| Option | Description | Selected |
|--------|-------------|----------|
| JSON tree + canonical mapping (split panel) | Left: decoded JSON tree; Right: canonical mapping result | ✓ |
| JSON tree only | Operator infers canonical themselves; lighter | |
| Diff against expected | Operator pastes expected JSON; runner shows red/green diff | |
| All three (JSON tree + canonical + diff) | Complete but UI gets dense | |

**User's choice:** JSON tree + canonical mapping (recommended)

### Q8: Save test cases

| Option | Description | Selected |
|--------|-------------|----------|
| No save — scratch pad only | Reload page → cleared; no history; minimal surface | ✓ |
| Save per profile as fixtures | Test cases ship with catalog entries; scope creep beyond V2-VEND-02 | |
| Save per session (localStorage) | Reload preserves; no cross-operator sync | |
| Claude decides | — | |

**User's choice:** No save — scratch pad (recommended)

---

## Anomaly threshold tuning

### Q9: Tuning model

| Option | Description | Selected |
|--------|-------------|----------|
| Ship better-defaults — no tuning UI | Phase 7 refines Phase 6 defaults from real-customer data; defaults frozen in code | ✓ |
| Per-MP tuning UI | MP detail → "Anomaly rules" card with P95 percentile, IQR multiplier inputs | |
| Per-vendor-profile tuning | Tune by device_profile; vendor-level signal granularity | |
| Global per-install settings | Settings → Alerts → Anomaly params; install-wide knobs | |

**User's choice:** Ship better-defaults — no tuning UI (recommended)
**Notes:** Aligns with PROJECT.md self-hosted no-ops product posture.

### Q10: Would-have-fired backtest preview

| Option | Description | Selected |
|--------|-------------|----------|
| Yes — "Test against last 30 days" button | Before enabling rule, run evaluator over last 30 days; report count | ✓ |
| No — trust the engine | Phase 6 ships 3 rules; Phase 7 only refines defaults | |
| Yes but read-only chart | MP detail chart "Last 30 days with threshold overlay"; non-interactive | |
| Claude decides | — | |

**User's choice:** "Test against last 30 days" button (recommended)

### Q11: Cold-start gate Phase 7 additions

| Option | Description | Selected |
|--------|-------------|----------|
| Unchanged from Phase 6 D-16 | Warmup card + Settings roster only; no Phase 7 additions | ✓ |
| Add "why this MP not firing" explanation | MP detail → reasons (warming up / no data / sparse / disabled / no trigger) | |
| Add Dashboard tile "Anomaly coverage" | Tile: "74% MPs eligible (320/430)" with proportional view | |
| Claude decides | — | |

**User's choice:** Unchanged from Phase 6 D-16

---

## Install validation hardening

### Q12: Probe surface

| Option | Description | Selected |
|--------|-------------|----------|
| Extend `shifter doctor` (06-11) | Add probe-chirpstack / probe-timescale / probe-region subcommands | ✓ |
| Probe on binary start | Probe runs on `shifter serve`; warn or fail boot if drift | |
| /health/detailed + periodic River cron | Periodic probe (6h cron); update /health/detailed; banner on drift | |
| Both doctor + periodic /health | 2 paths; comprehensive but heavier | |

**User's choice:** Extend `shifter doctor` (recommended)

### Q13: Probe failure mode

| Option | Description | Selected |
|--------|-------------|----------|
| Warn — don't block install | Log + banner + /health row; install proceeds; operator decides | ✓ |
| Refuse — binary exit on ChirpStack v3 / unsupported PG | Critical probe failure exits binary; can't run misconfigured | |
| 2-tier (critical refuse, soft warn) | Categorize probes; ChirpStack v3 / PG < 14 refuses; region drift warns | |
| Claude decides | — | |

**User's choice:** Warn — don't block install (recommended)

### Q14: V2-VEND-03 (saved templates + comparison + bulk gateway import)

| Option | Description | Selected |
|--------|-------------|----------|
| Defer — don't lock decision | V2-VEND-03 belongs to real-customer-signal portion of Phase 7 | (initially) |
| Discuss now — ship in Phase 7 | Lock decisions during this session | ✓ (after follow-up) |
| Drop — move to backlog | Move out of Phase 7 entirely | |

**User's choice:** Initially deferred; user opted to explore more gray areas and locked decisions in subsequent area.

---

## V2-VEND-03 — saved templates + comparison + bulk gateway import

### Q15: Saved report templates — what's captured

| Option | Description | Selected |
|--------|-------------|----------|
| Scope + range + sites/meters + group-by | Full ReportConfigPanel state minus download format | ✓ |
| Scope + range only | Lighter; operator re-picks sites each time | |
| Full state including download format | Single-click "Run my Monthly Building A PDF" | |
| Claude decides | — | |

**User's choice:** Scope + range + sites/meters + group-by (recommended)

### Q16: Templates per-user or install-wide

| Option | Description | Selected |
|--------|-------------|----------|
| Install-wide (shared) | Team-level resource; matches single-tenant operator-team posture | ✓ |
| Per-user | Each user has own templates; conflicts with shared-inbox philosophy | |
| Hybrid: shared default + per-user pinned | Shared + per-user pin/unpin; schema heavier | |
| Claude decides | — | |

**User's choice:** Install-wide (shared) (recommended)

### Q17: Side-by-side comparison entities

| Option | Description | Selected |
|--------|-------------|----------|
| 2 (A vs B) | Simple 2-column layout; chart overlay 2 series | ✓ |
| 3-4 (small group) | Comparison group; fits desktop, mobile portrait struggles | |
| N (unlimited) | Operator picks; chart with many series; UX degrades | |
| Drop comparison feature | Move side-by-side to backlog | |

**User's choice:** 2 (A vs B) (recommended)

### Q18: Bulk gateway import pattern

| Option | Description | Selected |
|--------|-------------|----------|
| Reuse Phase 3 device-import CSV pattern fully | Same dry-run → confirm → commit, re-runnable safe | ✓ |
| Reuse + add "pick on map" step | CSV import + per-row map placement step; friction up | |
| Don't ship — move to backlog | Bulk gateway less common than bulk device; form one-by-one | |
| Claude decides | — | |

**User's choice:** Reuse Phase 3 device-import CSV pattern fully (recommended)

---

## Vendor mapping expansion strategy

### Q19: Initial vendor count for Phase 7 ship

| Option | Description | Selected |
|--------|-------------|----------|
| 7 per SC#1 | Kamstrup MULTICAL, Diehl, Itron, Axioma, Sagemcom, Acrel, Schneider IEM3xxx | ✓ |
| Wait for real-customer signal | Ship Axioma W1 + add others as customers deploy; matches "deferred until ≥1 customer" | |
| Subset: 3 most common | Kamstrup (water), Acrel (electricity), Schneider (electricity) | |
| Claude decides | — | |

**User's choice:** 7 per SC#1 (recommended)

### Q20: Codec runtime

| Option | Description | Selected |
|--------|-------------|----------|
| Embedded Go | Each codec is Go function in internal/codec/{vendor}; compile-time check; no sandbox | ✓ |
| JS sandbox (Goja) | Codec as JS string in catalog; runtime parse; update without rebuild | |
| ChirpStack codec passthrough | ChirpStack decodes; Shifter consumes decoded JSON; conflicts with "don't expose ChirpStack" | |
| Hybrid Go + JS override | Catalog ship Go; operator override via JS; 2 runtime paths | |

**User's choice:** Embedded Go (recommended)

### Q21: New-vendor onboarding process

| Option | Description | Selected |
|--------|-------------|----------|
| PR (Go codec + catalog JSON) → release | Contributor adds Go codec + catalog entry + tests → PR → merge → binary release | ✓ |
| Operator add via UI (JS codec in DB) | Operator pastes JS codec via UI; requires JS sandbox runtime | |
| Hybrid: catalog = embed (us-shipped) + custom = DB (operator-shipped) | Two paths; "Add custom vendor" UI ships in V2 | |
| Claude decides | — | |

**User's choice:** PR-then-release (recommended)
**Notes:** This contradicts ROADMAP SC#1's "vendor #N is admin-UI work, not backend deploys" language. Documented as scope revision: V1.x = backend deploy each vendor; admin-UI add path deferred to V2.

---

## Catalog file format

### Q22: File format

| Option | Description | Selected |
|--------|-------------|----------|
| JSON | stdlib encoding/json; PR-diff-friendly; verbose but acceptable | ✓ |
| YAML | go-yaml/yaml.v3; cleaner human-edit; extra dep | |
| Go struct literal | Compile-time validation; non-Go contributors blocked | |
| Claude decides | — | |

**User's choice:** JSON (recommended)

### Q23: Schema validation

| Option | Description | Selected |
|--------|-------------|----------|
| go test at build time | TestCatalogValid decodes all JSONs + checks required fields + enums | ✓ |
| JSON Schema file + jsonschema lib | catalog.schema.json + runtime validation; extra dep | |
| Runtime parse only | Decode on startup; binary panics if invalid; production risk | |
| Claude decides | — | |

**User's choice:** go test at build time (recommended)

---

## Settings → Vendor Catalog tab UI

### Q24: List layout

| Option | Description | Selected |
|--------|-------------|----------|
| Table: Vendor / Model / Version / Installed_at / Status | shadcn DataTable; sortable, filterable; works on mobile | ✓ |
| Card grid with vendor logo | Vendor logo + name + capabilities; logos not in binary | |
| Categorized by capability (water/electricity/multi) | Group expand/collapse; UI complexity up | |
| Claude decides | — | |

**User's choice:** Table layout (recommended)

### Q25: Update flow

| Option | Description | Selected |
|--------|-------------|----------|
| Per-row "Update available" badge + Update button + diff modal | Operator-controlled; transparency via diff | ✓ |
| Auto-update silent on binary start | Conflicts with D-02 operator-controlled deploy | |
| Update button → diff modal only | Same as ✓ but explicit Q distinguishes diff design | |
| Claude decides | — | |

**User's choice:** Per-row "Update available" badge + button (recommended)

### Q26: Show "Devices using" count per profile

| Option | Description | Selected |
|--------|-------------|----------|
| Show count, clickable to filter device list | Helps decide if profile is safe to disable; matches Phase 3 gateway pattern | ✓ |
| Don't show; operator looks at devices separately | Simpler column; informed decisions harder | |
| Show via hover tooltip | Information hidden; mobile lacks hover | |
| Claude decides | — | |

**User's choice:** Show count (recommended)

---

## Claude's Discretion

Items where Claude has flexibility during planning:
- Exact catalog JSON schema (icon path / logo embed / short description fields)
- Diff modal layout (field-by-field rows / unified diff / structured form)
- Whether backtest result includes histogram or just count
- Catalog versioning bump strategy (when to bump)
- Migration path for existing Axioma W1 profile (catalog entry vs legacy row)

## Deferred Ideas

All deferred items captured in 07-CONTEXT.md `<deferred>` section. Key categories:
- JS sandbox + admin-UI "Add custom vendor" → V2
- Hot catalog reload → V2 ops
- Periodic install-probe + banner → V2 ops hardening
- Operator-facing anomaly tuning UI → V2 if customer feedback requires
- 3+ entity comparison → V2 power-user
- Per-user templates → V2 multi-operator
- CSV-with-pick-on-map → friction-reduction, V2
- Test-runner fixtures + expected-JSON diff → V2 codec-dev tools

---

# Update Pass — 2026-05-12

**Mode:** discuss (interactive, update)
**Areas discussed (4 + 7 Itron+KINMY clarifications):** Codec runtime + test-runner, Catalog migration for 3 existing seeds, Promote Claude's Discretion items, Comparison + template + notification UX, plus Itron+KINMY operator-supplied codec specifics (fPort, battery encoding, reverse_flow semantics, meter_id usage, meter clock fields, profile-aware anomaly/offline thresholds, slug naming)

**Trigger:** User invoked `/gsd-discuss-phase 7` → "Update it". Codebase scout surfaced critical contradiction: D-19 said "Codec source = embedded Go functions" but actual Phase 2 architecture is JS codecs pushed to ChirpStack QuickJS. Update pass corrects D-19, promotes 4 Claude's-Discretion items to locked decisions, and adds 13 new decisions (D-26..D-48).

---

## Codec runtime + test-runner

### Q1: D-19 correction — how should catalog ship codecs?

| Option | Description | Selected |
|--------|-------------|----------|
| JS files via //go:embed | Match existing arch: catalog references `internal/profile/codecs/*.js`, ChirpStack QuickJS runs them | ✓ |
| Inline JS string in catalog JSON | Single-file vendor entries; JSON-escaped JS ugly to diff | |
| Migrate to Go decoders | Rewrite Phase 2; add goja runtime in Shifter; deprecate ChirpStack codec push | |

**User's choice:** JS files via //go:embed (recommended)
**Notes:** Zero rewrite of Phase 2 substrate. Becomes D-19 corrected wording.

### Q2: Test-runner execution path

| Option | Description | Selected |
|--------|-------------|----------|
| Local goja JS runtime | Embed `dop251/goja`; run codec_js in Shifter process; <50ms, no ChirpStack dep | ✓ |
| ChirpStack codec-test gRPC API | Authoritative; runs exact engine prod uses; requires API to exist in v4.17 | |
| Round-trip via fake uplink | Publish synthetic MQTT uplink, read decoded event back; brittle, high latency | |

**User's choice:** Local goja JS runtime (recommended)
**Notes:** Becomes D-26. Adds ~3MB Go dep but enables D-27 (pre-sync testing).

### Q3: Test-runner behavior when codec_js_synced_at IS NULL

| Option | Description | Selected |
|--------|-------------|----------|
| Always works (local exec) | Test pre-sync (D-26 enables this); operator validates before pushing | ✓ |
| Disable until synced | Forces sync-first workflow; worse UX | |
| Warn but allow | Banner "results may differ from ChirpStack QuickJS"; muddy | |

**User's choice:** Always works (recommended)
**Notes:** Becomes D-27.

### Q4: Test-runner failure UX

| Option | Description | Selected |
|--------|-------------|----------|
| Inline error panel with line/col | Left panel: error type + msg + goja stack line/col + original hex; right panel grays out | ✓ |
| Toast + clear panels | Simpler UI; loses debug context | |
| Inline error + last successful result | Could confuse operator; "why does right show old data?" | |

**User's choice:** Inline error panel (recommended)
**Notes:** Becomes D-28.

---

## Catalog migration for 3 existing seeds

### Q5: How to reconcile 3 existing seed rows with catalog system

| Option | Description | Selected |
|--------|-------------|----------|
| Backfill catalog_source on existing rows | Migration adds 3 cols; backfills slug + v1.0.0 + customer_edited; clean future Updates | ✓ |
| Treat existing rows as legacy (NULL catalog_source) | Forces operator delete + re-import to get on catalog track | |
| Re-import wizard on first Phase 7 boot | One-time UX wizard; more friction but explicit | |

**User's choice:** Backfill (recommended). Becomes D-29.

### Q6: Backfill version for existing seeds

| Option | Description | Selected |
|--------|-------------|----------|
| 1.0.0 (catalog's initial seed version) | Existing seeds match what's shipped; no day-1 update noise | ✓ |
| 0.9.0 (force "update available" on day 1) | Demonstrates the flow; creates noise | |
| NULL (untracked, never auto-updates) | Pairs with legacy treatment | |

**User's choice:** 1.0.0 (recommended). Becomes D-30.

### Q7: Operator codec_js drift handling on migration

| Option | Description | Selected |
|--------|-------------|----------|
| Detect drift, set customer_edited=true, preserve operator's JS | Hash-compare row vs embedded; flag for D-34 diff UI | ✓ |
| Overwrite with catalog version | Operator tweaks lost; violates D-04 | |
| Skip migration if any drift, error to operator | Too aggressive; blocks upgrade | |

**User's choice:** Detect drift + preserve (recommended). Becomes D-31.

### Q8: Acrel shared-codec representation in catalog

| Option | Description | Selected |
|--------|-------------|----------|
| Two catalog entries, same codec_js_path | Two distinct catalog rows; share codec source; matches Phase 2 Pitfall 6 | ✓ |
| One catalog entry with two model variants | Single family entry with models[] array; needs UI for one-to-many | |
| Flatten — codec inlined into both JSON entries | Self-contained but maintenance pain | |

**User's choice:** Two catalog entries, same codec_js_path (recommended). Becomes D-32.

---

## Promote Claude's Discretion items

### Q9: Catalog JSON schema fields

| Option | Description | Selected |
|--------|-------------|----------|
| Minimal: slug, name, vendor, family, capabilities, version, codec_js_path, counter_modulus, mac_version, region | Mirrors device_profile table 1:1 | ✓ |
| Minimal + display metadata (icon, descriptions, vendor_url) | UI polish; 5 extra fields per entry | |
| Minimal + display + telemetry hints | Adds expected_interval, default_battery_low, payload_size_bytes | |

**User's choice:** Minimal (recommended). Becomes D-33 (later expanded to include Phase 7-specific fields D-41..D-45).

### Q10: Diff modal layout (D-24 specifics)

| Option | Description | Selected |
|--------|-------------|----------|
| Field-by-field rows with side-by-side values | git-mergetool mental model; per-field toggle + "you edited this" flag | ✓ |
| Unified diff (text-style) | Familiar but noisy for non-codec fields | |
| Structured form pre-populated with catalog values | Loses "this changed" signal | |

**User's choice:** Field-by-field rows (recommended). Becomes D-34.

### Q11: Backtest result format (D-10 specifics)

| Option | Description | Selected |
|--------|-------------|----------|
| Single count + daily sparkline | Lightweight; shows clustering vs even distribution | ✓ |
| Single count only | Simplest; loses distribution nuance | |
| Count + histogram + timestamp list | Power-user surface; overkill | |

**User's choice:** Count + sparkline (recommended). Becomes D-35.

### Q12: Codec re-sync trigger on catalog Update

| Option | Description | Selected |
|--------|-------------|----------|
| Auto-clear codec_js_synced_at; background sync via existing seed routine | Reuses Phase 2 substrate; zero new sync code | ✓ |
| Synchronous push as part of Update click | Worse UX if ChirpStack slow; tight coupling | |
| Operator confirms re-push in second dialog | Adds friction; second dialog is noise | |

**User's choice:** Auto-clear (recommended). Becomes D-36.

---

## Comparison + template + notification UX

### Q13: Comparison entity picker UX (D-16 specifics)

| Option | Description | Selected |
|--------|-------------|----------|
| Two dropdowns at top of compare view | Searchable; swap A↔B button; mobile-friendly | ✓ |
| Click "Compare with…" from any list row | Contextual entrypoint but spreads across pages | |
| Dedicated compare-builder page (multi-step wizard) | Overkill for 2-entity feature | |

**User's choice:** Two dropdowns (recommended). Becomes D-37.

### Q14: YoY mode engagement

| Option | Description | Selected |
|--------|-------------|----------|
| Toggle at top: "Compare entities" vs "Compare time ranges" | Clear mode separation; discoverable | ✓ |
| Entity B picker has "Same as A" option revealing 2nd time range | Discoverability poor | |
| Separate "Year-over-year" tab in entity detail page | Splits the compare story | |

**User's choice:** Top toggle (recommended). Becomes D-38.

### Q15: Template listing UX

| Option | Description | Selected |
|--------|-------------|----------|
| Alphabetical with search box | Predictable; search essential at 10+ templates | ✓ |
| Most-recently-used at top, then alphabetical | Frequent workflows one click away; adds last_run_at maint | |
| Categorized by scope | Premature structure for <30 templates | |

**User's choice:** Alphabetical + search (recommended). Becomes D-39.

### Q16: Update notification surface + probe retention

| Option | Description | Selected |
|--------|-------------|----------|
| Settings-only badge for catalog; /health/detailed last-run for probes | Operator-pull model; no notification fatigue | ✓ |
| Sidebar nav badge for catalog; last-run for probes | More visible but trains operators to ignore | |
| Catalog banner + 7-day probe history | More signal; more surface to maintain | |

**User's choice:** Settings-only + last-run (recommended). Becomes D-40.

---

## Itron+KINMY operator-supplied codec — clarifications

User submitted a JS decoder for "Itron LoRa Module" (3rd-party module on Itron water meter). Codec saved at `internal/profile/codecs/itron_kinmy_lora.js` (renamed from `itron_lora_module.js` after slug clarification — see Q23). Below are clarifications.

### Q17: fPort filter

| Option | Description | Selected |
|--------|-------------|----------|
| Restrict to known fPort (e.g., fPort == 2) | Like axioma_w1 fPort=100 check; rejects ACK/keepalive frames | |
| Accept all fPorts; SOF=0x6F filter only | Trust frame-format byte 0 check; flexible if firmware uses multiple fPorts | ✓ |

**User's choice:** Accept all (operator answered "รับทุก fPort")
**Notes:** Catalog metadata sets `fPort: null` (no fPort filter). Reflected in D-48 catalog entry.

### Q18: Battery encoding (voltage vs percent)

| Option | Description | Selected |
|--------|-------------|----------|
| A: Keep battery_v raw; no canonical pct | ALERT-02 (battery low) broken — alert needs pct | |
| B: Convert in codec (linear curve) | Simple; but Li-SOCl2 discharge non-linear → misleading % | |
| C: Codec emits battery_v raw; normalize.go applies per-vendor curve | Vendor curve in Go (testable, updateable); raw voltage preserved in JSONB | ✓ |

**User's choice:** C (Claude recommended after asking "แนะนำอันไหน")
**Notes:** Becomes D-44. New `battery_curve` registry in normalize.go. Itron+KINMY uses `li_socl2_3v6` curve.

### Q19: reverse_flow semantics

| Option | Description | Selected |
|--------|-------------|----------|
| A: raw JSONB only, no surface | Lost data; debug-only | |
| B: New canonical column reverse_cumulative_value | Schema migration; sparse for most vendors | |
| C: raw JSONB + new alert rule `reverse_flow_increase` (delta-based) | No canonical schema touch; practical alert | ✓ |
| D: Calculate delta-from-previous in alert evaluator only | Subset of C | |

**User's choice:** C (Claude recommended; operator confirmed + added crucial context: "reverse_flow is cumulative and never resets — used to check meter installed wrong direction")
**Notes:** Becomes D-46. Cumulative-never-reset semantic dictates delta-based alert (not absolute threshold).

### Q20: meter_id usage

| Option | Description | Selected |
|--------|-------------|----------|
| A: raw JSONB only, no surface | Misses cross-vendor meter-swap detection | |
| B: Promote to canonical meter_serial column + swap detection in Phase 7 | Correct but big scope (schema migration + swap logic + cumulative discontinuity handling) | |
| C: raw JSONB + `vendor_has_separate_meter_serial: true` flag | Phase 4 advanced tab auto-surfaces; Phase 8 can promote later | ✓ |

**User's choice:** C (Claude recommended after "แนะนำอันไหน")
**Notes:** Becomes D-45. The `vendor_has_separate_meter_serial` flag is a forward-marker for Phase 8 canonical promotion.

### Q21: meter clock fields (meter_date, meter_time)

| Option | Description | Selected |
|--------|-------------|----------|
| A: Drop (use server timestamp only) | Loses metadata for debug | |
| B: Store in raw JSONB | Free auto-surface; Phase 8 can add clock-drift alert | ✓ |
| C: raw JSONB + clock-drift alert in Phase 7 | Adds scope; deferred to Phase 8 | |
| D: Override canonical timestamp with meter clock | Breaks timescaledb CAGGs; never do this | |

**User's choice:** B
**Notes:** Becomes D-47. Phase 4 advanced tab auto-surfaces.

### Q22 (6.1): `expected_uplink_interval_seconds` per profile

| Option | Description | Selected |
|--------|-------------|----------|
| A: Add field to catalog; refactor ALERT-03/04 profile-aware | Correct; scope creep | ✓ |
| B: No field; global threshold for all profiles | Itron alert spam; tunability lost | |
| C: Catalog default + per-MP override | A + override UI; max flex but more scope | |

**User's choice:** A
**Notes:** Becomes D-41. Phase 7 scope expands to refactor ALERT-03/04. Per-MP override deferred to Phase 8.

### Q23 (6.2): `anomaly_compatibility` enum per profile

| Option | Description | Selected |
|--------|-------------|----------|
| A: Enum `full \| limited \| unsupported`; Settings UI hides incompatible rules | Simple; covers Phase 7 cases | ✓ |
| B: No flag; operator manually disable per rule (docs guide) | Tech debt; muddy | |
| C: Granular per-rule per-profile list (e.g., anomaly_rules: ["p95", "iqr"]) | Max flex; bigger scope | |

**User's choice:** A
**Notes:** Becomes D-42. Itron+KINMY = `limited` (p95+iqr with 60-day warmup; quiet_hour disabled).

### Q24 (6.3): ALERT-03 offline threshold multiplier

| Option | Description | Selected |
|--------|-------------|----------|
| A: ×1.5 (=36h for Itron) | Sensitive; false-positive risk on retry chains | |
| B: ×2.0 (=48h) | Standard default; slow detection | |
| C: ×2.5 (=60h) | Conservative; 2.5-day business value lost | |
| D: per-profile multiplier in catalog (Itron=1.8 → 43h) | Vendor-aware; ships better defaults | ✓ |
| E: Global ×2 + per-MP override UI | Future upgrade path | |

**User's choice:** D
**Notes:** Becomes D-43. Itron+KINMY multiplier = 1.8 (≈43h). Other vendors set per catalog.

### Q25: Slug + vendor field for Itron+KINMY entry

| Option | Description | Selected |
|--------|-------------|----------|
| A: vendor=Itron, family=LoRa Module | Operator searches by meter brand; misses module brand | |
| B: vendor=KINMY, family=Itron meter | Technically correct; operators don't know module brand | |
| C: vendor=Itron, family=KINMY LoRa Module | Hybrid — search by meter, family ID's module | ✓ |
| D: vendor=3rd-Party, family=Itron LoRa Module | Awkward vendor name in UI | |

**User's choice:** C, plus slug renamed to `itron_kinmy_lora` (more explicit than `itron_lora_module`)
**Notes:** Becomes D-48. Codec file renamed `itron_lora_module.js` → `itron_kinmy_lora.js`; embed.go updated.

---

## Claude's Discretion (remaining after update pass)

After update pass promotions, the still-discretion items are:
- Migration ordering (single migration or two for catalog_source columns + battery_curve)
- `report_template` table schema details
- Backtest button placement (inside rule-enable dialog vs separate Test CTA)
- goja sandbox limits (timeout, memory cap)
- Catalog `TestCatalogValid` test layout (table-driven vs per-file subtests)
- Compare view mobile layout (stacked cards vs collapsed dropdown rows)
- Reverse-flow alert rule constant naming (`alert.RuleKind.ReverseFlowIncrease`)

## Auto-Resolved

None — update pass was fully interactive.

## Codec compatibility report (Itron+KINMY)

Operator-supplied codec adapted to Phase 2 convention before saving:
- Return shape `{data, errors, warnings}` (was `{data: {error: "..."}}`)
- Defensive `bytes.length < 28` check added
- try/catch wrapper added
- Field names snake_case (was camelCase): `forward_flow_m3`, `reverse_flow_m3`, `battery_v`, `meter_id`, `meter_date`, `meter_time`, `tamper`, `leak`
- ES5 syntax (was ES6 arrow + spread + const + template literals)
- Helper `pad2()` as named function (was `const pad = n =>`)

Logic preserved verbatim: byte offsets, bit masks, ÷1000 (raw liters → m³), ÷10 (raw voltage × 10 → V), BE-reversed meter_id.

## Architectural correction logged

D-19 originally said "Codec source = embedded Go functions in `internal/codec/{vendor}/`." Codebase scout surfaced contradiction: actual Phase 2 architecture is JS codecs (`internal/profile/codecs/*.js`) embedded via `//go:embed`, pushed to ChirpStack v4 via gRPC, executed in ChirpStack's QuickJS sandbox. D-19 corrected in CONTEXT.md update. Cascades into D-26..D-28 (test-runner = local goja), D-32 (shared codec_js_path), D-36 (re-sync via codec_js_synced_at NULL).
