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
