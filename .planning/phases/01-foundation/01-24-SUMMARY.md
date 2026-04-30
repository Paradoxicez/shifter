---
phase: 01-foundation
plan: 24
subsystem: docs
tags: [readme, docs, install-guide, runbook, requirements, inst-06, d-19]

# Dependency graph
requires:
  - phase: 01-foundation
    plan: 05
    provides: shifter CLI surface (serve, migrate, version, create-admin, config-check, healthcheck) — README CLI table mirrors this exactly
  - phase: 01-foundation
    plan: 18
    provides: /health + /health/detailed split (D-18/D-19) — README + docs/install.md document both
  - phase: 01-foundation
    plan: 20
    provides: install/bundled/install.sh + bundled compose flavor — README "Bundled flavor" section + docs/install.md walkthrough
  - phase: 01-foundation
    plan: 21
    provides: install/external/install.sh + external compose flavor + .env.example — README "External flavor" section + docs/install.md walkthrough
  - phase: 01-foundation
    plan: 22
    provides: Caddyfile + 3 TLS modes (acme/byo/internal per D-21) — docs/install.md TLS modes table
  - phase: 01-foundation
    plan: 23
    provides: login UI — docs/install.md "After install" step references it

provides:
  - README.md (canonical operator-facing entry point)
  - docs/install.md (full bundled + external install walkthrough + TLS modes + wizard breakdown + health endpoints)
  - docs/operator-runbook.md (recovery procedures: dirty migration, locked-out admin, v3 refusal, /health/detailed inspection, ChirpStack credential update)
  - Updated REQUIREMENTS.md INST-06 wording aligned with D-19 split

affects:
  - Phase 2+ (every future plan that ships an operator-facing surface MUST add a sibling docs/<feature>.md and link from README.md "Recovery" or a new top-level section)
  - Phase 1 sign-off (all operator artifacts have a documentation entry; live compose smoke remains the only deferred verification, tracked at the Plan 19/20/21/22 boundary)

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Pattern: docs/<topic>.md tree pattern. Every Phase 2+ feature with an operator-facing surface (alerts, reports, bulk-import, backups) MUST ship a sibling `docs/<topic>.md` and link from README.md. Phase 1 establishes `install.md` (one-time setup) + `operator-runbook.md` (recovery procedures); Phase 2+ adds capability-specific docs (e.g., `docs/alerts.md`, `docs/bulk-import.md`)."
    - "Pattern: README.md is the single entry point. README has Quick Start (dev), Production Install (compose flavor table), CLI table, Health Endpoints, Configuration, Recovery (link-only). Anything longer goes in docs/. Future plans MUST NOT bloat README.md with feature-specific walkthroughs."
    - "Pattern: REQUIREMENTS.md wording-update commits when a phase decision reframes a requirement. D-19 explicitly flagged 'INST-06 wording must be updated during Phase 1 implementation' — this plan closes that loop. Future phase decisions that reframe requirements (vs adding new ones) MUST include the wording update in the same plan that ships the implementation."

key-files:
  created:
    - "docs/install.md (84 lines — bundled + external walkthroughs, TLS modes, wizard breakdown, post-install, SETT-03, health probes)"
    - "docs/operator-runbook.md (76 lines — locked-out admin, dirty migration, v3 refusal, /health/detailed, ChirpStack credential SQL fallback, logs, backups Phase 6 forward-look)"
    - ".planning/phases/01-foundation/01-24-SUMMARY.md (this file)"
  modified:
    - "README.md (replaced Plan 01 stub with the canonical operator-facing README — Quick Start, Production Install, CLI table, Health Endpoints, Configuration, Recovery; 92 insertions / 6 deletions)"
    - ".planning/REQUIREMENTS.md (INST-06 wording aligned with D-19 split — 1 line replaced)"

key-decisions:
  - "README.md is link-only for recovery + install walkthroughs. The README has a Recovery section that links to docs/operator-runbook.md instead of inlining the procedures; same for docs/install.md vs the README's Production Install table. Rationale: operators reading the README for the first time should see the elevator pitch + the one-shot install path. Reach for docs/* only when something needs explaining (TLS modes, wizard internals) or fixing (dirty migration, locked-out admin)."
  - "TLS modes table lives in docs/install.md, NOT in README.md. README mentions the three modes by name (`acme`/`byo`/`internal`) and links to docs/install.md for the full table. Rationale: the table needs operational context (when to choose each, what each does) that doesn't fit a top-level README. Same logic applies to the wizard's 5-step breakdown."
  - "INST-06 wording update is a single line replacement, not a deletion + new requirement. The original INST-06 reserved /health for everything; D-19 split it. Replacing the wording (vs deleting INST-06 + adding INST-06a + INST-06b) preserves the requirement ID + traceability table entry, which means no downstream plan files need updating. The replacement text explicitly references D-18/D-19 so future reviewers can audit the reframing."
  - "Phase 6 forward-looks are documented in operator-runbook.md but NOT promised in INST-06's wording. The runbook says '`checks.db: false` → Postgres is unreachable; Phase 6 will add ChirpStack/MQTT/disk/last-uplink-age checks'; INST-06 says 'Phase 6 expands with...'. Both are forward-looks, not commitments — Phase 6 plan-counter has not started. If Phase 6 reframes again, both wordings get updated together."
  - "docs/install.md mentions ZeroSSL fallback once (in the TLS modes table) but does not walk through ZeroSSL config. Rationale: Plan 22 reserved {$CADDY_GLOBAL_TLS_BLOCK} for the future ZeroSSL switch but did not ship the operator-facing instructions. Adding ZeroSSL specifics to docs/install.md would be premature — when ZeroSSL fallback ships, that plan adds the instructions in the same commit as the implementation."

patterns-established:
  - "Pattern: `docs/` tree at repo root. Every operator-facing topic that doesn't fit the README gets a sibling `docs/<topic>.md`. Phase 2+ MUST follow this layout — no docs/ subdirectories per topic, no docs at the repo root."
  - "Pattern: README sections in this order (1) Title + value prop, (2) Why Shifter, (3) Status, (4) Quick Start (dev), (5) Production Install (table + flavor sections), (6) CLI, (7) Health Endpoints, (8) Configuration, (9) Recovery (link), (10) License. Future README updates MAY add sections between (5) and (9), but MUST NOT reorder the existing sections."
  - "Pattern: Recovery procedures cite the relevant Phase 1 decision IDs (D-09 for must_change_password posture, D-14 for create-admin --reset). Future runbook entries MUST cite the decision ID (or PITFALL number) that backs the procedure. Bare 'do X to fix Y' instructions are not allowed — every recovery has a 'why' linkable to a planning document."
  - "Pattern: `Last updated` and version markers live in the planning artifacts (.planning/REQUIREMENTS.md footer, SUMMARY.md frontmatter), NOT in the operator-facing docs. README + docs/* should never have a 'Last updated' line; operators care about what's true today, not when the doc was edited."

requirements-completed: [INST-06]

# Metrics
duration: 2min46s
completed: 2026-04-30
---

# Phase 1 Plan 24: README + Docs + Runbook Summary

**Operator-facing documentation surface for Phase 1: canonical README at repo root with Quick Start + bundled/external compose flavor table + CLI table + health endpoints split per D-19 + secrets-not-in-`.env` enforcement, `docs/install.md` walking both compose flavors end-to-end including 3 TLS modes (acme/byo/internal) + 5-step wizard breakdown (admin → ChirpStack → region AS923-2 default → install identity → review) + INST-05 v3 refusal + post-install steps, `docs/operator-runbook.md` covering the recovery escape hatches (locked-out admin via `shifter create-admin --reset` per D-14, dirty-migration recovery via `shifter migrate force`, ChirpStack v3 boot refusal per INST-05, `/health/detailed` inspection, ChirpStack credential SQL fallback when UI is down). REQUIREMENTS.md INST-06 wording reframed to match D-19's `/health` (public minimal) + `/health/detailed` (admin-required) split. Phase 1 ships with 100% of operator-facing artifacts documented.**

## Performance

- **Duration:** ~2 min 46 sec
- **Started:** 2026-04-30T15:47:27Z
- **Completed:** 2026-04-30T15:50:13Z
- **Tasks:** 3 / 3
- **Commits:** 3 (Task 1, Task 2, Task 3 — plus a final docs commit for SUMMARY + state)
- **Files created:** 3 (`docs/install.md`, `docs/operator-runbook.md`, this SUMMARY)
- **Files modified:** 2 (`README.md`, `.planning/REQUIREMENTS.md`)

## Accomplishments

### README.md (operator-facing canonical entry point)

Replaced the Plan 01 stub with the canonical Phase 1 README:

| Section | Content |
| --- | --- |
| **Title + value prop** | "Self-hosted LoRaWAN water and electricity monitoring." Single-tenant per install. |
| **Why Shifter** | 3 bullets: wrap-don't-replace, meter-swap continuity (Phase 2), two compose flavors. |
| **Status** | Phase 1 complete; Phase 2 next; ROADMAP link. |
| **Quick Start (dev)** | `just bootstrap && just dev` + dev compose for Postgres/Mosquitto/ChirpStack. |
| **Production Install** | Bundled vs External comparison table; `install/bundled/install.sh` + `install/external/install.sh` invocations; pointer to `docs/install.md`. |
| **CLI** | All 6 `shifter` subcommands with one-line descriptions per Plan 05. |
| **Health Endpoints** | `GET /health` public + `GET /health/detailed` admin-required (D-19). |
| **Configuration** | `/etc/shifter/config.yaml` + `SHIFTER_*` env override + `_FILE` secret pattern + .env-forbidden-for-secrets per D-06. |
| **Recovery** | Link-only to `docs/operator-runbook.md`. |
| **License** | TBD (single-customer commercial). |

### docs/install.md (84 lines)

Walks both compose flavors end-to-end:

- **Prerequisites** — Docker Compose v2, DNS for `acme`, ports 80/443, external-flavor needs existing ChirpStack v4 + MQTT broker.
- **Choosing a flavor** — bundled (greenfield) vs external (brownfield).
- **TLS modes table** — 3 rows: acme (Let's Encrypt + ZeroSSL fallback), byo (cert.pem + key.pem mounted from secrets/), internal (Caddy local CA, LAN-only).
- **Bundled flavor — step-by-step** — 5 numbered steps including the wizard URL.
- **External flavor — step-by-step** — 6 numbered steps including .env editing + token paste + chmod 0600.
- **What the wizard captures** — admin user (Argon2id), ChirpStack mode + creds + INST-05 v3 refusal, region AS923-2 default, install identity, atomic commit.
- **After install** — login URL, Settings → Test connection, Phase 1 sidebar shows only Settings.
- **Updating credentials post-install (SETT-03)** — Settings → ChirpStack → Edit connection → re-runs v3 probe.
- **Health probes for monitoring** — `/health` for Docker/LB probes; `/health/detailed` for admin (DB now, more in Phase 6).

### docs/operator-runbook.md (76 lines)

Recovery procedures, decision-ID-cited:

- **I'm locked out** — `shifter create-admin --reset` (D-14); explains `--reset` semantics + Argon2id hashing + `must_change_password` not set per D-09.
- **Database is dirty after a partial migration** — psql to inspect `schema_migrations`, `shifter migrate force <N>`, restart, "don't bypass dirty state lightly".
- **ChirpStack v3 detected at boot** — INST-05 boot-refusal error format + recommended action (upgrade ChirpStack).
- **/health/detailed shows degraded** — current Phase 1 minimum (DB only per D-19) + Phase 6 forward-look.
- **Updating ChirpStack credentials** — UI path (Settings → Edit → Save re-tests) + non-UI SQL fallback for `chirpstack_connection` table.
- **Logs** — json-file driver + 10m/3 caps (PITFALL §15 + OPS-05); `docker compose logs -f shifter`.
- **Backups (Phase 6)** — Phase 1 ships no backup automation; pg_dump as interim.

### REQUIREMENTS.md INST-06 update

Replaced the original wording:

> System exposes a `/health` endpoint reporting database, ChirpStack, MQTT, disk, last-uplink-age, and the running Shifter version

With the D-19-aligned wording:

> System exposes `/health` (public, no auth — `{status, version, uptime_seconds}` minimum) and `/health/detailed` (admin-required — DB connection state in Phase 1; Phase 6 expands with ChirpStack, MQTT, disk, and last-uplink-age). Reframed by Phase 1 D-18/D-19; the original single-endpoint-with-everything wording is replaced by this split.

The traceability-table row (`| INST-06 | Phase 1 | Complete |`) is unchanged.

## Task Commits

1. **Task 1 — README.md canonical operator-facing version** — `e751e86` (feat)
2. **Task 2 — docs/install.md + docs/operator-runbook.md** — `e710c71` (docs)
3. **Task 3 — REQUIREMENTS.md INST-06 wording aligned with D-19** — `e22cd15` (docs)

**Plan metadata commit:** _follows immediately_

## Behavior Summary

| Path | Outcome |
| --- | --- |
| New operator opens README.md | Sees value prop → Quick Start (dev) OR Production Install (bundled vs external) → links to docs/install.md and docs/operator-runbook.md |
| Operator runs `./install/bundled/install.sh shifter.example.com` | Per docs/install.md "Bundled flavor — step-by-step": idempotent secret generation → image build → compose up → /health poll via Caddy → "Visit https://shifter.example.com/install" message |
| Operator runs `./install/external/install.sh` | Per docs/install.md "External flavor — step-by-step": .env validated → secret files validated (REFUSES to fabricate ChirpStack API token) → image build → compose up → /health poll via Caddy |
| Locked-out admin | Per docs/operator-runbook.md: `docker compose ... exec shifter shifter create-admin --reset --email ... --password ...` resets the password (D-14 escape hatch) |
| Dirty migration on boot | Per docs/operator-runbook.md: psql → SELECT FROM schema_migrations → `shifter migrate force <PREVIOUS_CLEAN_VERSION>` → restart |
| ChirpStack v3 attached at boot | Per docs/operator-runbook.md: shifter prints `INST-05: refusing to start — ChirpStack v3 detected at <url>` and exits; operator must upgrade ChirpStack |

## Verification Matrix

| Check | Tool | Result |
| --- | --- | --- |
| README has Quick Start heading | `grep 'Quick Start' README.md` | PASS |
| README mentions `just bootstrap` and `just dev` | grep | PASS — both present |
| README mentions both install scripts | grep `install/bundled/install.sh` + grep `install/external/install.sh` | PASS — both present |
| README has bundled-vs-external comparison table | inspection | PASS — table at "Production Install" |
| README mentions all 6 CLI subcommands | `grep -E 'shifter (serve\|migrate\|version\|create-admin\|config-check\|healthcheck)'` | PASS — 7 matches (some commands appear twice across `serve`/`migrate up` rows) |
| README documents `/health` and `/health/detailed` per D-19 | grep | PASS — both endpoints documented in Health Endpoints section |
| README mentions Compose secrets and `.env` is forbidden for secrets (D-06) | `grep "'.env' is forbidden"` | PASS |
| README links to docs/install.md and docs/operator-runbook.md | grep | PASS — both linked |
| docs/install.md has Bundled flavor section | grep | PASS |
| docs/install.md has External flavor section | grep | PASS |
| docs/install.md documents all 3 TLS modes | grep | PASS — acme/byo/internal all in TLS modes table |
| docs/install.md documents wizard 5 steps including AS923-2 default + v3 refusal | grep `AS923-2` + grep `INST-05` | PASS |
| docs/install.md documents both /health endpoints per D-19 | grep | PASS — both in "Health probes for monitoring" section |
| docs/operator-runbook.md contains `shifter migrate force <N>` | grep | PASS |
| docs/operator-runbook.md contains `shifter create-admin --reset` | grep | PASS |
| docs/operator-runbook.md documents v3 boot refusal | grep `INST-05` + grep `ChirpStack v3` | PASS |
| REQUIREMENTS.md INST-06 contains both `/health` AND `/health/detailed` | grep | PASS |
| REQUIREMENTS.md INST-06 references "admin-required" + "Phase 6" | grep | PASS — both present |
| `grep -c 'INST-06' REQUIREMENTS.md` ≥ 2 | grep -c | PASS — 2 (definition line + traceability table) |

All 16 acceptance-criteria checks pass. Plan-level verification (4 items) all pass.

## Threat Surface Notes

All two STRIDE register entries from the plan's `<threat_model>` are mitigated:

| Threat | Mitigation |
| --- | --- |
| T-24-01 (docs publish secrets handling steps) | Docs reference `secrets/<name>.txt` files but never include real values; install scripts generate dummy ones for smoke (per Plan 20/21) and require operator-provided real secrets for production. ASVS V8 satisfied. |
| T-24-02 (runbook recommends `migrate force` carelessly) | Runbook explicitly says "Don't bypass dirty state lightly. It typically signals real schema corruption that needs a code fix, not a force." |

No new threat surface introduced.

## Threat Flags

(none — pure documentation; no new attack surface)

## Deviations from Plan

None. All three tasks executed exactly as written; all acceptance criteria + success criteria + verification items pass.

The plan's task bodies were verbatim-followed. No Rule 1/2/3 deviations needed.

## Issues Encountered

None.

## Known Stubs

(none — every artifact in this plan is production-quality)

## Deferred Verifications

(none introduced by this plan)

The Phase 1 sign-off still has the deferred live compose smoke from Plans 19/20/21/22 — Plan 24 does not contribute to that backlog.

## User Setup Required

(none — pure documentation)

## Next Phase Readiness

- All Phase 1 operator-facing artifacts have a documentation entry: README points at install scripts + CLI + health endpoints; docs/install.md walks both compose flavors with TLS mode coverage; docs/operator-runbook.md covers the recovery escape hatches.
- D-19's "REQUIREMENTS.md must be updated during Phase 1" directive is closed — INST-06 wording is now consistent with the implementation Plans 14/15/18 shipped.
- Phase 2 plans MAY extend `docs/` with capability-specific files (e.g., `docs/canonical-schema.md` when the metering-point abstraction lands). The pattern is established.
- Phase 1 sign-off can proceed once the deferred live compose smoke (Plan 19/20/21/22 boundary) is run on a host with a working Docker daemon.

## Self-Check: PASSED

Files verified to exist:

- FOUND: `README.md` (canonical version, 105 lines after replacement)
- FOUND: `docs/install.md` (84 lines)
- FOUND: `docs/operator-runbook.md` (76 lines)
- FOUND: `.planning/REQUIREMENTS.md` (INST-06 line updated; 344 lines total)
- FOUND: `.planning/phases/01-foundation/01-24-SUMMARY.md` (this file)

Commits verified to exist (`git log --oneline -5`):

- FOUND: `e22cd15` (Task 3 — docs(01-24): align INST-06 wording with D-19 health split)
- FOUND: `e710c71` (Task 2 — docs(01-24): operator install guide + recovery runbook)
- FOUND: `e751e86` (Task 1 — feat(01-24): canonical operator-facing README)

Acceptance grep proofs (rerun at self-check time):

- `grep -q 'Quick Start' README.md` → PASS
- `grep -q 'just bootstrap' README.md` → PASS
- `grep -q 'install/bundled/install.sh' README.md && grep -q 'install/external/install.sh' README.md` → PASS
- `grep -q 'shifter migrate force' README.md` → PASS
- `grep -q 'Bundled flavor' docs/install.md && grep -q 'External flavor' docs/install.md` → PASS
- `grep -q '/health' docs/install.md` → PASS
- `grep -q 'shifter migrate force' docs/operator-runbook.md` → PASS
- `grep -q 'shifter create-admin --reset' docs/operator-runbook.md` → PASS
- `grep -A1 'INST-06' .planning/REQUIREMENTS.md | head -3 | grep -q '/health/detailed'` → PASS

---
*Phase: 01-foundation*
*Plan: 24-readme-docs*
*Completed: 2026-04-30*
