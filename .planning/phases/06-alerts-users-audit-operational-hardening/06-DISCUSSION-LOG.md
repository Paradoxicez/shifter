# Phase 6: Alerts, Users, Audit & Operational Hardening - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-05-12
**Phase:** 06-alerts-users-audit-operational-hardening
**Areas discussed:** Alerts (engine + center UX), User management, Audit browse + CSV export, Backup/restore + ops polish

---

## Alerts (engine + center UX)

### Q1: How should alert rules be stored and evaluated?

| Option | Description | Selected |
|--------|-------------|----------|
| DB rules + River cron worker (recommended) | Rules in DB; River cron job every N min scans active rules. Durable retries, reuse Phase 5 install. | ✓ |
| DB rules + per-MP Postgres trigger | Synchronous in trigger on measurement insert. Sub-second latency; couples ingest to alert logic. | |
| DB rules + dedicated goroutine poll loop | Long-running goroutine in binary; no River dep for this; loses River features. | |

**User's choice:** DB rules + River cron worker (recommended)

### Q2: Threshold rule shape (ALERT-01 daily/hourly/instantaneous)?

| Option | Description | Selected |
|--------|-------------|----------|
| Three rule subtypes per CAGG level (recommended) | `threshold_instantaneous`, `_hourly`, `_daily`. Each rule picks one + bound. | ✓ |
| Single rule type with operator-chosen window | One `threshold` rule with `window: instantaneous \| 1h \| 24h`. | |
| Only instantaneous + daily in v1, hourly v1.x | Drop hourly to shrink v1. | |

**User's choice:** Three rule subtypes per CAGG level (recommended)

### Q3: ALERT-04 21-day cold-start communication?

| Option | Description | Selected |
|--------|-------------|----------|
| Per-meter visible status chip (recommended) | MP detail "Warming up — 16d remaining" with progress bar + Settings warmup roster. | ✓ |
| Settings-only banner | Cold-start state only on Settings → Alerts page. | |
| Silent until eligible | No UI surface; anomaly just doesn't fire. | |

**User's choice:** Per-meter visible status chip (recommended)

### Q4: Where does the alert center live in the shell?

| Option | Description | Selected |
|--------|-------------|----------|
| Header bell + drawer + dedicated `/alerts` page (recommended) | GitHub/Linear pattern. | ✓ |
| Sidebar nav `Alerts` item with badge, full-page only | Cleaner sidebar but no drawer for context preservation. | |
| Header bell + drawer only (no dedicated page) | Compact; loses URL-shareable filtered views. | |

**User's choice:** Header bell + drawer + dedicated `/alerts` page (recommended)

### Q5: ALERT-03 gateway-down suppression?

| Option | Description | Selected |
|--------|-------------|----------|
| Eval-time check against gateway last_seen (recommended) | Stateless suppression; survives mapping changes. | ✓ |
| Snapshot device-gateway mapping at offline-event time | State machine per pair. | |
| Single 'cluster' alert per offline gateway | Devices listed as casualties. | |

**User's choice:** Eval-time check against gateway last_seen (recommended)

### Q6: Severity model?

| Option | Description | Selected |
|--------|-------------|----------|
| Three tiers: info / warning / critical (recommended) | Standard ITSM-lite; slate/yellow/red. | ✓ |
| Two tiers: warning / critical | Simpler; loses info bucket. | |
| Per-rule operator-chosen severity | Maximum flexibility, no convention. | |

**User's choice:** Three tiers: info / warning / critical (recommended)

### Q7: Snooze/mute options?

| Option | Description | Selected |
|--------|-------------|----------|
| Presets (1h / 8h / 24h / 7d) + 'Mute until I clear' (recommended) | Slack/Linear UX. | ✓ |
| Presets only (1h / 8h / 24h) | Drop 7d + indefinite. | |
| Operator-typed datetime + indefinite | Maximum flexibility, more clicks. | |

**User's choice:** Presets (1h / 8h / 24h / 7d) + 'Mute until I clear' (recommended)

### Q8: When does an alert clear?

| Option | Description | Selected |
|--------|-------------|----------|
| Auto-clear when condition resolves + audit row (recommended) | Avoids stale red badges. | ✓ |
| Manual ack only — alerts never auto-clear | Higher signal cost. | |
| Auto-clear for offline + anomaly; manual for threshold breach | Mixed model. | |

**User's choice:** Auto-clear when condition resolves + audit row (recommended)

### Q9: Which ALERT-04 anomaly rules ship in v1?

| Option | Description | Selected |
|--------|-------------|----------|
| All three: P95 + IQR + quiet-hour flow (recommended) | Full set; opt-in per MP; tuning Phase 7. | ✓ |
| Two: P95 + quiet-hour (drop IQR) | Drop noisiest. | |
| One: quiet-hour flow only | Water-only signal. | |

**User's choice:** All three: P95 trailing-30d + IQR outliers + quiet-hour flow (recommended)

### Q10: Where do operators create/edit alert rules?

| Option | Description | Selected |
|--------|-------------|----------|
| From MP detail + Site detail + Settings→Alerts library (recommended) | Three entry points, one dialog. | ✓ |
| Settings→Alerts only | Single source; more clicks. | |
| MP/Site detail only (no central library) | No global library page. | |

**User's choice:** From MP detail + Site detail + Settings→Alerts library (recommended)

### Q11: Alert TTL?

| Option | Description | Selected |
|--------|-------------|----------|
| 365 days fixed + Settings→Data Retention toggle (recommended) | One year default; SETT-04 pattern. | ✓ |
| Same as raw measurement retention (90d default) | Tied to raw retention. | |
| Forever (no automatic prune) | Tiny storage; maximum traceability. | |

**User's choice:** 365 days fixed + Settings→Data Retention toggle (recommended)

### Q12: Alert worker failure handling?

| Option | Description | Selected |
|--------|-------------|----------|
| River retry policy + degraded-state banner (recommended) | River retry (3) + shell banner + /health/detailed. | ✓ |
| Retry forever, no banner | No UI signal. | |
| Fail fast, no retry, page on first error | Risk alert fatigue on transient hiccups. | |

**User's choice:** River retry policy + degraded-state banner (recommended)

### Q13: Cool-down between consecutive fires?

| Option | Description | Selected |
|--------|-------------|----------|
| Per-rule cool-down field, default 15 min (recommended) | Override per-rule; prevents flap spam. | ✓ |
| Global 5-min cool-down, not per-rule | Simpler UX. | |
| No cool-down — dedupe by 'open alert exists' | Avoids config entirely. | |

**User's choice:** Per-rule cool-down field, default 15 min (recommended)

### Q14: Webhook prep for v2?

| Option | Description | Selected |
|--------|-------------|----------|
| Yes — structured payload from day 1 (recommended) | JSONB payload with stable schema; v2 ships without migration. | ✓ |
| Minimal columns only, design payload in v2 | Lean schema; v2 may need backfill. | |

**User's choice:** Yes — record alerts with structured payload from day 1 (recommended)

### Q15: Routing — who gets paged?

| Option | Description | Selected |
|--------|-------------|----------|
| All admins + viewers see in-app alert center (recommended) | Shared inbox; v2 adds routing. | ✓ |
| Admins only — viewers don't see alerts | Cleaner separation; viewer can't ack. | |
| Per-rule operator-chosen subscribers | Most flexible, most config. | |

**User's choice:** All admins + viewers see in-app alert center (recommended)

### Q16: Snooze/mute scope — per-user or globally?

| Option | Description | Selected |
|--------|-------------|----------|
| Globally per alert (recommended) | Shared-inbox model; single source of truth. | ✓ |
| Per-user (each user has their own snooze) | Requires user_alert_state join table. | |

**User's choice:** Globally per alert (recommended)

### Q17: Per-rule notes / labels?

| Option | Description | Selected |
|--------|-------------|----------|
| Optional name + notes field (recommended) | Auto-generated default + override + `notes TEXT`. | ✓ |
| Auto-generated names only, no notes | Less data; loses context handoff. | |

**User's choice:** Optional name + notes field (recommended)

### Q18: Test-fire button?

| Option | Description | Selected |
|--------|-------------|----------|
| Yes — 'Test fire' creates synthetic alert (recommended) | UI smoke test; auto-clears 60s. | ✓ |
| Yes — 'Dry run' against last 24h measurements | Would-fire preview; more engine work. | |
| No test/dry-run | Smaller v1; more friction tuning. | |

**User's choice:** Yes — 'Test fire' button creates synthetic alert (recommended)

### Q19: What does /health/detailed expose about the alert worker?

| Option | Description | Selected |
|--------|-------------|----------|
| Last-run ts + rules-evaluated + duration + degraded flag (recommended) | Drives shell banner. | ✓ |
| Just last-run + degraded flag | Minimal; counts on River dashboard. | |
| No alert-worker row — use River's built-in observability | Lower surface, operator learns River UI. | |

**User's choice:** Last-run ts + rules-evaluated count + duration + degraded flag (recommended)

### Q20: Rule enable/disable without deletion?

| Option | Description | Selected |
|--------|-------------|----------|
| Yes — `disabled_at` soft-delete (recommended) | Mirrors USER/SITE/DEVICE patterns. | ✓ |
| Hard-delete only | Cleaner table; `rule_snapshot` keeps history. | |

**User's choice:** Yes — `disabled_at` soft-delete (recommended)

---

## User Management (USER-01..04)

### Q1: Admin-created initial password?

| Option | Description | Selected |
|--------|-------------|----------|
| Random + show-once panel after create (recommended) | Strong random; operator never types; force-change on first login. | ✓ |
| Admin-typed initial password | Strength meter from Phase 1; risk of weak pwd. | |
| Both — toggle in dialog | Generate / type; more dialog surface. | |

**User's choice:** Random + show-once panel after create (recommended)

### Q2: 'Logout everywhere' UX (USER-03)?

| Option | Description | Selected |
|--------|-------------|----------|
| Admin button + confirm AlertDialog → immediate kick (recommended) | DELETEs all SCS sessions; audit row. | ✓ |
| Same with optional 'Reason' note (audit-visible) | Compliance traceability. | |
| Schedule-driven kick via session middleware | No bulk DELETE; latency up to 1 request. | |

**User's choice:** Admin button + confirm AlertDialog → immediate kick (recommended)

### Q3: Role-change semantics?

| Option | Description | Selected |
|--------|-------------|----------|
| Auto-logout-everywhere on role change (recommended) | Avoids stale auth context. | ✓ |
| Reflect on next request — no logout | Smoother UX; client cache risk. | |
| Banner + user logs out themselves | Lowest disruption; user may delay. | |

**User's choice:** Auto-logout-everywhere on role change (recommended)

### Q4: Auth events to audit_log?

| Option | Description | Selected |
|--------|-------------|----------|
| Yes — retrofit Phase 1 handlers + new event types (recommended) | Closes Phase 2 D-21 deferred work. | ✓ |
| Only state-changing events (skip login/logout) | Smaller growth; loses compliance use. | |
| Separate `auth_log` table | Cleaner separation; union for full picture. | |

**User's choice:** Yes — retrofit Phase 1 auth handlers + add new event types (recommended)

### Q5: Where does Users admin page live?

| Option | Description | Selected |
|--------|-------------|----------|
| Settings→Users tab (recommended) | SETT-01 already enumerates Users category. | ✓ |
| Sidebar 'Users' nav item (admin-only) | More discoverable; pollutes sidebar. | |
| Both — sidebar shortcut + Settings tab | Redundant but discoverable. | |

**User's choice:** Settings→Users tab (recommended)

### Q6: Can admin demote/disable themselves?

| Option | Description | Selected |
|--------|-------------|----------|
| Block both — always require another admin (recommended) | Last-admin guard; CLI escape hatch unchanged. | ✓ |
| Allow self-role-change with confirm + warning | Still block last-admin demote. | |
| Block all self-affecting actions including self-pwd-change | Most restrictive. | |

**User's choice:** Block both — always require another admin (recommended)

### Q7: Re-enable disabled user?

| Option | Description | Selected |
|--------|-------------|----------|
| Show disabled toggle + Re-enable preserves state (recommended) | Phase 3 archive pattern. | ✓ |
| Force password reset on re-enable | Treats as new account for security. | |
| Hard re-create via Add user | Breaks USER-01 no-hard-delete. | |

**User's choice:** Show disabled in 'Show disabled' toggle + Re-enable action (recommended)

### Q8: Password strength policy?

| Option | Description | Selected |
|--------|-------------|----------|
| Reuse Phase 1 evaluator + same min-score (recommended) | Single source of truth. | ✓ |
| Stricter for admin role (14+ chars, 3-of-4 classes) | Threat-model-driven. | |
| Relaxed min for generated random | Smoother UX; weaker baseline. | |

**User's choice:** Reuse Phase 1 `password_strength.go` evaluator + same min-score (recommended)

---

## Audit browse + CSV export (AUDIT-02, AUDIT-03)

### Q1: Where does Audit browse live?

| Option | Description | Selected |
|--------|-------------|----------|
| Top-level `/audit` route, admin-only sidebar (recommended) | Dedicated compliance surface. | ✓ |
| Settings→Audit tab | Bundled with admin surfaces. | |
| Per-entity History tabs + global /audit overview | Richest UX, more surfaces. | |

**User's choice:** Top-level `/audit` route, admin-only sidebar item (recommended)

### Q2: Filter UI shape?

| Option | Description | Selected |
|--------|-------------|----------|
| URL-state filter chips above table (recommended) | Phase 3/4 D-15 pattern; deep-linkable. | ✓ |
| Left-rail sidebar facets with counts | Navigational; cost per facet. | |
| Simple text-search + date range only | Minimal v1. | |

**User's choice:** URL-state filter chips above table (recommended)

### Q3: Before/after diff presentation?

| Option | Description | Selected |
|--------|-------------|----------|
| Expandable JsonTree per row (recommended) | Reuse Phase 4 D-19 component. | ✓ |
| Auto-generated change-summary line + expand for JSON | Most readable; requires per-entity summarizer. | |
| JSON-only modal on row click | Cleaner table; extra click. | |

**User's choice:** Expandable JsonTree per row (recommended)

### Q4: CSV export format + scope?

| Option | Description | Selected |
|--------|-------------|----------|
| Mirror REPT-03 spec + export-current-filter (recommended) | UTF-8 BOM, ISO, install tz; respects filter; 50k cap then River job. | ✓ |
| Same format but always full table | Simpler; less useful for compliance. | |
| Two formats: filtered CSV + full SQL dump | More surface. | |

**User's choice:** Mirror REPT-03 spec + export-current-filter (recommended)

### Q5: audit_log retention?

| Option | Description | Selected |
|--------|-------------|----------|
| 5 years default + Settings→Data Retention toggle (recommended) | Matches daily-CAGG retention; Settings reuses Phase 5 pattern. | ✓ |
| 10 years default | Longer baseline. | |
| Forever — no automatic prune | Tiny storage; manual prune. | |

**User's choice:** 5 years default + Settings→Data Retention toggle (recommended)

### Q6: Pagination model?

| Option | Description | Selected |
|--------|-------------|----------|
| Cursor pagination by (time DESC, id) (recommended) | Phase 4 D-15 pattern; stable. | ✓ |
| Offset with page numbers | Familiar; drift risk. | |
| Infinite scroll with virtualization | Smoothest; more frontend work. | |

**User's choice:** Cursor pagination by (time DESC, id) (recommended)

### Q7: Default view on cold arrival?

| Option | Description | Selected |
|--------|-------------|----------|
| Last 7 days, all entity types, all users (recommended) | Sensible default; fast first paint. | ✓ |
| Last 30 days | Broader; more rows. | |
| No default time filter — last 100 rows | Inconsistent "how far back" feel. | |

**User's choice:** Last 7 days, all entity types, all users (recommended)

### Q8: Live tail / SSE for audit?

| Option | Description | Selected |
|--------|-------------|----------|
| No — manual refresh button, no SSE (recommended) | Forensic surface, not monitor. | ✓ |
| Yes — extend Phase 4 SSE hub with audit topic | More polish; more topic surface. | |

**User's choice:** No — manual refresh button, no SSE (recommended)

---

## Backup/restore + ops polish (OPS-02..08, SETT-02, SETT-05)

### Q1: Backup format?

| Option | Description | Selected |
|--------|-------------|----------|
| Single tar.gz: pg_dump + floor-plan dir + manifest.json (recommended) | One artifact per backup; manifest pins version + sha256. | ✓ |
| Split: db.sql.gz + floor-plans.tar.gz + manifest.json | Three files; easier per-piece verify. | |
| Streaming pg_dump to stdout via tar pipe | No intermediate disk; failure-prone. | |

**User's choice:** Single tar.gz: `pg_dump` + floor-plan dir + manifest.json (recommended)

### Q2: TimescaleDB-aware pg_dump strategy?

| Option | Description | Selected |
|--------|-------------|----------|
| pg_dump --format=custom + timescaledb-backup helper or pre/post hooks (recommended) | Research finalizes. | ✓ |
| Logical pg_dump --data-only + migrate-rebuilds-schema on restore | Doesn't cleanly capture extension state. | |
| Physical backup (pg_basebackup / WAL archiving) | Faster restore; PG-version-locked. | |

**User's choice:** `pg_dump --format=custom` with `timescaledb-backup` helper or documented post-restore steps (recommended)

### Q3: Backup destination?

| Option | Description | Selected |
|--------|-------------|----------|
| Local path only in v1, S3 v1.x (recommended) | Configurable local path; S3 ships per V2-OPS-01. | ✓ |
| Local path + S3-compatible in v1 (full OPS-03) | aws-sdk-go-v2 in v1. | |
| Local path + stream-to-stdout | Minimal Shifter code; operator wraps. | |

**User's choice:** Local path only in v1, S3 v1.x (recommended)

### Q4: Backup trigger?

| Option | Description | Selected |
|--------|-------------|----------|
| shifter backup CLI + cron snippet + Settings button (recommended) | Three triggers, one code path. | ✓ |
| CLI only — operator wires their own cron | Smallest surface; most friction. | |
| River cron-scheduled inside binary | Tightly coupled; touches app lifetime. | |

**User's choice:** `shifter backup` CLI + cron snippet + manual Settings button (recommended)

### Q5: Restore procedure?

| Option | Description | Selected |
|--------|-------------|----------|
| shifter restore CLI — requires Shifter down (recommended) | PG advisory lock refuses while serving. | ✓ |
| Hot restore via UI | Complex; partial-state risk. | |
| Restore container variant | Isolates from main binary lifecycle. | |

**User's choice:** `shifter restore --from <tarball>` CLI — requires Shifter down (recommended)

### Q6: CI round-trip test (OPS-04)?

| Option | Description | Selected |
|--------|-------------|----------|
| Seed → backup → fresh DB → restore → smoke (recommended) | Closes OPS-04 verbatim. | ✓ |
| Same + cross-version restore (vN → vN+1) | Catches research-flagged concern; slower CI. | |
| Manual smoke only, no CI | Conflicts with OPS-04 "tested in CI." | |

**User's choice:** Seed → backup → fresh DB → restore → smoke (recommended)

### Q7: Settings→Backup status panel (SETT-05)?

| Option | Description | Selected |
|--------|-------------|----------|
| Last-backup ts + age + run-now button + threshold warning (recommended) | Green/yellow/red dot; last-5 list. | ✓ |
| Status only, no run-now button | Loses one-click affordance. | |
| Status + Run-now + dedicated History page | Most polished. | |

**User's choice:** Last-backup ts + age + run-now button + threshold warning (recommended)

### Q8: Upgrade runbook (OPS-08)?

| Option | Description | Selected |
|--------|-------------|----------|
| Pre-upgrade backup + version-pin bump + migrate + verify (recommended) | Standard self-hosted runbook. | ✓ |
| Above + blue-green deployment | Doesn't fit single-tenant install. | |
| Above + automated `shifter upgrade` CLI | Permissions complexity. | |

**User's choice:** Pre-upgrade backup + version-pin bump + migrate + verify (recommended)

### Q9: ChirpStack DB backup scope in bundled mode?

| Option | Description | Selected |
|--------|-------------|----------|
| Yes — bundled mode includes ChirpStack DB (recommended) | Detected via install_state; one-tarball recovery. | ✓ |
| No — Shifter-only; CS backup is operator's responsibility | Smaller scope; risk forgetting. | |
| Yes but opt-in via --include-chirpstack flag | Forces opt-in. | |

**User's choice:** Yes — bundled mode backup includes ChirpStack DB (recommended)

### Q10: SETT-02 identity-change propagation?

| Option | Description | Selected |
|--------|-------------|----------|
| New reports get new branding; old artifacts immutable (recommended) | Aligns with Phase 5 D-07 "reports are ephemeral." | ✓ |
| Regenerate all cached reports on identity change | River jobs; burst risk. | |
| Mark cached stale; show 'Regenerate?' on visit | Mixed model; more UI. | |

**User's choice:** New reports get new branding; old artifacts immutable (recommended)

### Q11: OPS-05/06/07 verification?

| Option | Description | Selected |
|--------|-------------|----------|
| Verification pass + close gaps in this phase (recommended) | Audit both compose files; document conventions. | ✓ |
| Trust Phase 1; just document | Smaller scope; drift risk. | |

**User's choice:** Verification pass + close gaps in this phase (recommended)

### Q12: Anything non-obvious about ops hardening?

| Option | Description | Selected |
|--------|-------------|----------|
| Add `shifter doctor` CLI for support snapshot (recommended) | Redacted diagnostic bundle. | ✓ |
| No new CLI — document /health/detailed + logs as support surface | Smaller v1; more friction per ticket. | |
| Light version: `shifter healthcheck --verbose` | Conflates lifecycle probes with operator-snapshot. | |

**User's choice:** Add `shifter doctor` CLI — diagnostic snapshot for support (recommended)

---

## Claude's Discretion

(Items the user implicitly deferred by selecting "recommended" with downstream-discretion details — see CONTEXT.md `<decisions>` § Claude's Discretion for the full list.)

- Audit-vocabulary migration numbering (planner picks)
- Alert worker River queue name + cron expressions (planner picks)
- Cron sidecar image choice for bundled-mode nightly backup (planner picks smallest pinned image)
- `timescaledb-backup` helper vs raw `pg_dump` + restore-time helpers (research finalizes)
- Alert payload field naming conventions (snake_case)
- JsonTree integration in audit table rows — virtualize or not (planner picks)
- Cold-start chip copy and progress-bar styling (planner picks; follows Phase 4 D-21 pattern)
- Sidebar nav reordering (planner picks final order)
- CLI prompt copy for create-admin --reset and restore --from
- Random password alphabet for D-23 (suggested: full printable ASCII excluding `1lI0O`, length ≥ 16)

## Deferred Ideas

(Ideas mentioned during discussion that were noted for future phases — see CONTEXT.md `<deferred>` for the full annotated list.)

- S3 / S3-compatible backup destination → V2-OPS-01
- Scheduled backup UI inside Shifter → v1.x
- Hot restore / zero-downtime → v2 if SLO needs it
- Email / SMTP alert delivery → V2-NOTIF-02
- Webhook alert delivery → V2-INT-01
- Per-user dashboards / saved views / per-rule subscribers → V2-AUTH-02
- SSO / OAuth / SCIM → V2-AUTH-01
- Custom roles beyond admin/viewer → V2-AUTH-03
- Engine dry-run (rule-fires-against-last-30-days) → Phase 7 anomaly tuning
- Live-tail SSE for audit → v2
- Audit log full-text search → v1.x
- Cross-version restore CI test → may pull into Phase 6 or defer to Phase 6.5
- Saved alert rule templates → v1.x
- audit_log as TimescaleDB hypertable with compression → v1.x if it grows beyond ~10 M rows
- Multi-channel alert routing → V2-NOTIF-*
- Bulk user import / SSO group sync → V2-AUTH-01
- Encrypted backups at rest → v1.x (operator's responsibility for now via FS encryption)
- Backup retention / rotation inside Shifter → v1.x
