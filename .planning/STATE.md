---
gsd_state_version: 1.0
milestone: v1.0
milestone_name: milestone
status: Ready to execute
last_updated: "2026-04-27T18:25:37.045Z"
progress:
  total_phases: 7
  completed_phases: 0
  total_plans: 24
  completed_plans: 1
  percent: 4
---

# Project State: Shifter

**Last Updated:** 2026-04-27 (after Plan 01-01 execution)

## Project Reference

**Core Value:** The operator runs their entire LoRaWAN water/electricity monitoring operation — provisioning, placement, monitoring, reporting — from Shifter alone, and meter swaps never break historical continuity.

**Current Focus:** Phase 01 — foundation

## Current Position

Phase: 01 (foundation) — EXECUTING
Plan: 2 of 24

| Field | Value |
|-------|-------|
| **Phase** | 1 — Foundation |
| **Plan** | 02 — test-harness (next) |
| **Status** | Plan 01 complete; Plan 02 ready to execute |
| **Progress (plans)** | `[░░░░░░░░░░] 1/24 (4%)` |
| **Progress (phases)** | `[░░░░░░░░░░] 0/7 phases` |

**Next action:** `/gsd-execute-plan 01 02` (or `/gsd-execute-phase 01` to continue the chain)

## Performance Metrics

| Metric | Value |
|--------|-------|
| Phases complete | 0 / 7 |
| v1 requirements mapped | 99 / 99 (100%) |
| Plans complete | 1 / 24 |
| Open blockers | 0 |

### Per-plan execution log

| Plan | Duration | Tasks | Files |
|------|----------|-------|-------|
| 01-01 repo-scaffold | 23 min | 2 | 21 |

## Accumulated Context

### Key Decisions (locked at roadmap creation)

- **Backend language:** Go 1.24+ (single-binary deploy, native ChirpStack gRPC stubs, mature MQTT/Postgres ecosystem) — locked in research, executed in Phase 1.
- **Database:** PostgreSQL 16/17 + TimescaleDB 2.26 (one DB for relational metadata + telemetry hypertable + CAGGs).
- **ChirpStack integration:** gRPC for control plane + MQTT for events; v4 only (refuses v3 on first connect).
- **Realtime delivery:** SSE backed by Postgres `LISTEN/NOTIFY` from a hypertable insert trigger — never directly off MQTT, so the dashboard only sees persisted data.
- **Frontend:** Vite + React 19 + TypeScript + Tailwind v4 + shadcn/ui (no Next.js — no SSR/SEO benefit). Served by the Go binary via `go:embed`.
- **Domain anchor:** Metering point + time-windowed device assignment + reading offset is the schema invariant. Every report, chart, alert queries by `metering_point_id`, never `dev_eui`.
- **Canonical measurement schema:** Hybrid wide+JSONB hypertable (canonical first-class columns + `extra` JSONB + `raw` JSONB for the original payload). Codecs run inside ChirpStack's QuickJS sandbox; Shifter only maps decoded objects to canonical fields.
- **Floor-plan placement:** Normalized fractions (`x_frac`, `y_frac` ∈ [0, 1]), never pixel integers. PROJECT.md wording will be updated during Phase 1 or Phase 5.
- **Map:** Leaflet 1.9 + react-leaflet 5 + `leaflet.markercluster` from day 1; OpenStreetMap tiles only (no paid API).
- **Deployment:** Two Docker Compose flavors (`bundled` + `external`) sharing the same backend image. File-based Compose secrets, pinned image tags, Caddy reverse proxy.
- **Audit middleware:** Ships in Phase 2 before the meter-swap UI so swaps are auditable from day 1; UI + CSV export in Phase 6.

### Phase 01 Execution Decisions

- **Plan 01-01 — Justfile is the canonical entry point.** Every dev/CI/install task goes through a `just` recipe; raw `go`/`pnpm`/`golangci-lint`/`migrate` calls in docs or CI are forbidden going forward (D-02).
- **Plan 01-01 — `@vitejs/plugin-react` pinned to ^5.** v6 requires Vite 8; we pin Vite 7. Re-evaluate at phase wrap-up if Vite 8 has stabilized.
- **Plan 01-01 — Biome 2.x config schema.** `assist.actions.source.organizeImports`, `files.includes` with negative globs. Plan template referenced legacy 1.9 schema; future plans must use 2.x.
- **Plan 01-01 — Cobra root carries a Long description.** Lets `--help` print lowercase "shifter" (verification grep) and seeds the help template for upcoming `serve` / `migrate` / `create-admin` subcommands.
- **Plan 01-01 — `web/package.json` uses `@types/node` and `type: "module"`.** Required by Vite 7 + ESM `vite.config.ts`.
- **Plan 01-01 — Host prerequisites:** Go 1.24+, Node 22.12+ (or 20.19+) per Vite 7, pnpm 10.x, `just` (host-installed). Plan 02 (test harness) should add `engines.node` to `web/package.json` and a `.nvmrc`.

### Open Todos

- **Plan 02 — pin Node engines.** Add `"engines": { "node": ">=22.12" }` to `web/package.json` and a `.nvmrc` so contributors get a clear error instead of a Vite runtime warning.
- **Plan 02 — Biome OOM workaround.** `pnpm exec biome` is OOM'ing the linter daemon in this sandbox. Investigate `BIOME_LOG_PATH` / heap flags or fall back to `biome ci` mode if pre-commit hooks fail.

### Open Blockers

(none)

### Pre-Flight Notes

- **PROJECT.md wording fix pending:** "Pixel-coordinate device placement" should read "normalized fractional coordinates on the floor plan" — apply during Phase 1 or Phase 5, whichever lands the floor-plan storage code first.
- **Research flags for downstream planning:** Phases 2, 3, 5, 6, 7 should run `/gsd-research-phase` before planning (see ROADMAP.md "Research Flags"). Phases 1 and 4 use standard patterns.
- **AS923 sub-plan default:** Region picker in Phase 1 install wizard pre-selects the Thailand-correct sub-plan. This is a Phase 1 acceptance check.

## Session Continuity

**If resuming after interruption:**

1. Re-read `.planning/PROJECT.md` (core value + constraints)
2. Re-read `.planning/REQUIREMENTS.md` (v1 scope + traceability)
3. Re-read `.planning/ROADMAP.md` (phase structure + success criteria)
4. Re-read `.planning/research/SUMMARY.md`, `ARCHITECTURE.md`, `PITFALLS.md` for technical context
5. Run `/gsd-plan-phase 1` to begin Phase 1 planning

**Files of record:**

- `.planning/PROJECT.md` — vision + constraints + key decisions
- `.planning/REQUIREMENTS.md` — v1 + v2 + out-of-scope + traceability
- `.planning/ROADMAP.md` — 7-phase structure with success criteria
- `.planning/STATE.md` — this file (current position + accumulated context)
- `.planning/research/SUMMARY.md` — research synthesis
- `.planning/research/ARCHITECTURE.md` — component dependencies
- `.planning/research/PITFALLS.md` — pitfall→phase mapping
- `.planning/config.json` — granularity + workflow settings

---
*State initialized: 2026-04-27 after roadmap creation*
