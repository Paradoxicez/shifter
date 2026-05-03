# Phase 2: Domain Model & Canonical Schema - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in 02-CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-05-03
**Phase:** 02-domain-model-canonical-schema
**Mode:** discuss (interactive)
**Areas discussed:** Vendor profile + canonical columns, Add-device + meter-swap UX, Site + metering-point creation surface, Audit log scope + schema, Synthetic test harness, MQTT resolver pipeline, Reading offset proposal UX, Ingest validation/quarantine, ChirpStack abstraction depth, Profile editor mapping UI shape, Decommission vs swap edge cases, ChirpStack disconnect recovery, Devices surface in Phase 2, Bulk operations in Phase 2, Initial seed delivery, TimescaleDB hypertable defaults

---

## Vendor profile + canonical columns

### Schema layering meta-discussion (clarification before formal questions)

User asked how the system would absorb arbitrary vendor feature sets (some meters send only a face-value reading, some report leak detection bool, some have battery, some are Modbus RS485 → LoRa converters with 30+ fields). Confirmed three-layer model:

| Layer | Mechanism | Purpose |
|-------|-----------|---------|
| 1 | canonical wide columns (typed, indexed) | dashboard/report/alert query path |
| 2 | `device_profile.capabilities[]` | UI/alert engine adaptive render |
| 3 | `extra` JSONB | everything else; advanced view renders generically |

Confirmed: 3-layer model **OK**. Layer 1 column set as proposed **OK**. Profile editor UI **ships in full** in Phase 2.

### Vendor #1 selection — "do we need real hardware?"

User asked whether vendor #1 (DATA-10 fully-wired profile) requires physical hardware. Clarified: "fully wired" = schema + codec + mapping + capabilities tested end-to-end via synthetic test harness against the vendor's published payload format. **No hardware purchase needed.** From ChirpStack downward the path is identical whether the payload comes from RF or from a synthetic publisher. Real customer hardware plugs in seamlessly during Phase 3 deployments.

Reframed the vendor question as "which payload format does the v1 reference profile model" — the profile is concrete (Axioma, Kamstrup, etc.), the path to running it is synthetic.

### Recommendation request — water + 1-phase + 3-phase

User asked for one specific vendor recommendation each for water, 1-phase electricity, and 3-phase electricity. After web verification (current as of 2026):

| Class | Recommended | Why |
|-------|-------------|-----|
| Water | Axioma Qalcosonic W1 | Public PDF "F1 V1.8 Enhanced" + Node-RED gist + TTN device repo + AS923 native + flat payload (vs Kamstrup OMS/wM-Bus complexity) |
| 1-phase electricity | Acrel ADL200 | Same Acrel codec family as ADW300 → 1 codec implementation covers 2 device profiles |
| 3-phase electricity | Acrel ADW300 | Best-documented LoRaWAN 3-phase meter, AS923 native, multi-phase + power-quality stress-tests the JSONB extra layer |

Alternatives considered: Kamstrup flowIQ 2200/MULTICAL 21 (water — defer to Phase 7 catalog due to wM-Bus complexity), Diehl IZAR (water alternative), Schneider IEM3xxx + LoRa converter (3-phase alternative for retrofit cases), Milesight WS523 (1-phase plug-form alternative).

### Phase 2 vendor catalog scope — water-only or all 3?

User pushed back on water-only as default. Discussed scope:

| Option | Profiles | Reasoning |
|--------|----------|-----------|
| A | 3 (Axioma + ADL200 + ADW300) | Schema validated for water + 1-phase + 3-phase; multi_phase + power_quality JSONB exercised; Acrel codec family makes ADL200 ~free after ADW300 |
| B | 2 (Axioma + ADW300) | Hardest schema cases (water + 3-phase) covered; ADL200 trivially added Phase 3 |
| C | 1 (Axioma only) | Strict DATA-10 minimum; defers all electricity to Phase 3 |

User selected **Option A** (3 profiles). DATA-10 wording "at least one fully wired" supports shipping more.

### Final 3 questions for Area 1

| Question | Options Presented | Selected |
|----------|-------------------|----------|
| Vendor #1 reference (with 3 profiles confirmed) | Axioma + Acrel pair locked from earlier — formal lock |  D-07 |
| Measurement hypertable | Single (Recommended) / Split per utility / Single + view layer | **Single hypertable** — D-03 |
| Counter rollover modulus mechanism | Per-profile config + override (Recommended) / Per-device override / Auto-detect | **Per-profile config + override** — D-05 |

---

## Add-device & meter-swap UX

| Question | Options Presented | Selected |
|----------|-------------------|----------|
| Add-device dialog shape | Stepped dialog (Recommended) / Single-screen / Quick + advanced disclosure | **Stepped dialog (4 steps)** — D-10 |
| DevEUI sticker endianness handling | Auto-detect + preview (Recommended) / Explicit picker / Always MSB-first | **Auto-detect + preview** — D-11 |
| Outgoing reading R capture at swap | Auto-fill + verify checkbox (Recommended) / Manual type / Hybrid retype | **Auto-fill + verify checkbox** — D-12 |
| valid_to semantics for in-flight uplinks | confirm_time + gateway_rx_time attribution (Recommended) / last_uplink_time / Operator grace window | **confirm_time + gateway_rx_time attribution** — D-14 |

---

## Site & metering-point creation surface

User asked clarification on **Create Site fields** before answering. Provided field-by-field downstream usage table; user picked **Standard** (option A) with all proposed fields.

| Question | Options Presented | Selected |
|----------|-------------------|----------|
| Create Site fields | Standard (Recommended) / Minimal / Rich + site_type + tags | **Standard** — D-17 |
| Site lat/lng UI in Phase 2 | Input box + 'Pick on map' deferred (Recommended) / Install Leaflet now / Plain inputs | **Input box + deferred picker** — D-18 |
| Create MP fields | Standard (Recommended) / Minimal / Rich + tariff/cost | **Standard** — D-19 |
| MP lifecycle | Soft-delete archived (Recommended) / Hard-delete + cascade / Suspend-only | **Soft-delete archived** — D-20 |

---

## Audit log scope & schema

| Question | Options Presented | Selected |
|----------|-------------------|----------|
| Audit scope | CRUD + meter swap (Recommended) / + Auth events / + Sensitive reads / Everything | **CRUD + meter swap** — D-21 |
| Audit table layout | Single audit_log + JSONB (Recommended) / Per-entity / PG triggers | **Single + JSONB** — D-22 |
| Audit write timing | Synchronous in-tx (Recommended) / Async via River / Hybrid | **Synchronous** — D-23 |
| before/after JSONB shape | Changed fields only (Recommended) / Full snapshot / Action description | **Changed fields only** — D-24 |

---

## Synthetic test harness (Phase 2 expansion)

| Question | Options Presented | Selected |
|----------|-------------------|----------|
| Test harness location | Both: lib + Go tests + CLI (Recommended) / Go tests only / CLI only | **Both: shared lib + Go tests + CLI wrapper** — D-27 |

---

## MQTT → metering_point resolver

| Question | Options Presented | Selected |
|----------|-------------------|----------|
| Resolver cache strategy | In-memory + LISTEN/NOTIFY (Recommended) / Query DB every uplink / TTL-only | **In-memory + LISTEN/NOTIFY** — D-25 |

---

## Reading offset proposal UX

| Question | Options Presented | Selected |
|----------|-------------------|----------|
| Offset proposal UI | Math equation + read-back panel (Recommended) / Number-only / Mini-chart | **Math equation + read-back** — D-13 |

---

## Ingest validation / quarantine

| Question | Options Presented | Selected |
|----------|-------------------|----------|
| Invalid uplink handling | Persist + quality flag (Recommended) / Reject + log only / Quarantine table | **Persist + quality flag** — D-26 |

---

## ChirpStack abstraction depth

| Question | Options Presented | Selected |
|----------|-------------------|----------|
| CS hierarchy abstraction | Single global tenant + 1 app + auto-bootstrap (Recommended) / 1 app/site / 1 app/utility_class | **Single global tenant + 1 app + auto-bootstrap** — D-28 |

---

## Profile editor mapping UI shape

| Question | Options Presented | Selected |
|----------|-------------------|----------|
| Mapping UI | Spreadsheet table + paste sample JSON + dropdowns (Recommended) / Card list / Raw JSON editor | **Spreadsheet table + paste JSON + dropdowns** — D-08 |

---

## Decommission vs swap edge cases

| Question | Options Presented | Selected |
|----------|-------------------|----------|
| Decommission flow | Separate 'Decommission device' action (Recommended) / Swap to '(no replacement)' / Archive MP auto-closes binding | **Separate 'Decommission device' action** — D-15 |

---

## ChirpStack disconnect recovery

| Question | Options Presented | Selected |
|----------|-------------------|----------|
| CS disconnect handling | Pre-validate + transactional rollback + clear error (Recommended) / Saga + River retry / Fail fast no-queue | **Pre-validate + transactional rollback** — D-16 |

---

## Devices surface in Phase 2

| Question | Options Presented | Selected |
|----------|-------------------|----------|
| Devices Phase 2 scope | Picker + minimal list page (Recommended) / Picker only / Full DEV-01 in Phase 2 | **Picker + minimal list page** — D-29 |

---

## Bulk create sites/MPs in Phase 2

| Question | Options Presented | Selected |
|----------|-------------------|----------|
| Bulk operations Phase 2 | Single-only (Recommended) / Sites + MPs CSV / Defer all to Phase 3 | **Single-only** — D-30 |

---

## Initial seed delivery (3 profiles)

| Question | Options Presented | Selected |
|----------|-------------------|----------|
| Seed strategy | Migration seed + first-boot CS sync (Recommended) / JSON seed file / UI-only import | **Migration seed + first-boot CS sync** — D-09 |

---

## TimescaleDB hypertable defaults

| Question | Options Presented | Selected |
|----------|-------------------|----------|
| Hypertable layout | measurement hypertable (1d) + audit regular (Recommended) / Both hypertables / Tuned chunk | **measurement hypertable (1d), audit regular** — D-06 |

---

## Claude's Discretion (areas left for planner judgement)

- Exact `internal/` package layout (`internal/ingest`, `internal/swap`, `internal/profile`, `internal/audit`, `internal/testharness`)
- Migration file naming continuing from 0007 onward
- Foreign key + unique constraint placement
- Exact JSON shape for `audit_log.before`/`after` field-level diffs
- TanStack Table column visibility / sort / page size on minimal devices list page
- Mapping editor's exact JSON-tree-click behaviour vs manual json_pointer entry
- Profile editor's `data_type` enum extensions
- Quality badge tooltip wording
- `shifter test-harness` CLI flag/subcommand shape
- River queue install timing (Phase 2 doesn't need it; first usage is Phase 5/6)

## Deferred Ideas (captured to PROJECT/ROADMAP backlog)

See `<deferred>` section of 02-CONTEXT.md for the full list. Notable: auth-event auditing → Phase 6, audit browse UI → Phase 6, CAGGs/retention → Phase 5, floor-plan placement → Phase 5, devices full list → Phase 3, bulk CSV → Phase 3, codec test-runner → Phase 7, pre-seeded vendor catalog → Phase 7, mini-chart in swap dialog → after Phase 4 charts ship.
