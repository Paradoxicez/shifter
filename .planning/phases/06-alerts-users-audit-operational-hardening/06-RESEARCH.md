# Phase 6: Alerts, Users, Audit & Operational Hardening — Research

**Researched:** 2026-05-12
**Domain:** River cron-based alert engine + TimescaleDB-aware logical backup/restore + SCS bulk session revoke + audit browse + Cobra operator-CLI subcommands
**Confidence:** HIGH for backup/restore strategy (official docs + archived upstream tool), HIGH for River patterns (Context7 + official docs), HIGH for SCS revoke (already shipping in `iterateAndRevoke`), MEDIUM for anomaly SQL costs (no Phase-7-style empirical data yet)

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

**Alerts — engine + storage**
- D-01: Alert rules in DB + River cron worker. Rules persisted as `alert_rule` rows; River cron job evaluates active rules every N minutes per-subtype.
- D-02: Three threshold-rule subtypes per CAGG level — `threshold_instantaneous` (raw `measurement` latest per MP), `threshold_hourly` (`cagg_hourly`), `threshold_daily` (`cagg_daily`). Cadence: instantaneous 1 min, hourly 15 min, daily 1 h.
- D-03: Three rule kinds, three workers, one River queue. Threshold + offline + anomaly each have their own evaluator; shared `EvaluateContext` (db, hub, audit, install_tz).
- D-04: Rules are soft-deletable via `disabled_at` (mirrors gateway / MP pattern). "Show disabled" toggle.
- D-05: Per-rule cool-down field, default 900 s (15 min). Operator override per rule.
- D-06: Optional human name + notes field on every rule.

**Alerts — fired-alert lifecycle**
- D-07: Three-tier severity with locked color map. `info` (slate), `warning` (yellow), `critical` (red). Defaults: threshold/offline/anomaly = critical; battery-low/signal-degraded/cold-start = warning; first-uplink = info. Operator override per rule.
- D-08: Auto-clear when condition resolves + audit row. Acked alerts stay `acknowledged`; re-arm only after clearing.
- D-09: Snooze presets 1h / 8h / 24h / 7d + "Mute until I clear".
- D-10: Snooze/mute is global to the alert (shared inbox). Audit row records who snoozed.
- D-11: Routing v1 = no routing. Every authenticated user sees the same alert center.
- D-12: Structured alert payload from day 1. `alert.payload JSONB`: `{rule_id, rule_kind, severity, target: {entity_type, entity_id, label}, value, threshold, comparison, fired_at, install: {display_name}}`. Same shape powers v2 webhook body.
- D-13: Alert retention = 365 days default, Settings-toggle-able. Reuses retention reconciliation flow.

**Alerts — engine specifics**
- D-14: ALERT-03 gateway-down suppression = eval-time check against gateway last_seen. Stateless suppression.
- D-15: ALERT-02 offline threshold = `now() - last_uplink > 3 × expected_interval_s` to fire; hysteresis grace = `< 2 × expected_interval_s` to clear. **Intentionally STRICTER than Phase 4 D-07.**
- D-16: ALERT-04 cold-start gate = visible per-meter status chip on MP detail page + warmup roster in Settings → Alerts.
- D-17: ALERT-04 ships all three statistical rules, opt-in per-MP. `anomaly_p95`, `anomaly_iqr`, `anomaly_quiet_hour`. Phase 7 tunes thresholds; Phase 6 ships the shape.
- D-18: Three entry points for alert rule creation, one dialog. MP detail / Site detail / Settings → Alerts.
- D-19: Test-fire button in rule dialog (UI smoke test, not engine dry-run). `is_test = true`, severity `info`, auto-clears after 60 s.

**Alerts — center UX**
- D-20: Header bell + slide-over drawer + dedicated `/alerts` page. URL-state filter chips on page.
- D-21: Alert worker observability on `/health/detailed`. New row: `{last_run_at, rules_evaluated, fires_emitted, cleared, duration_ms, degraded, last_error}`.
- D-22: Worker failure handling = River retry (max 3 attempts) + degraded-state banner. Shell banner renders when degraded.

**User Management**
- D-23: USER-04 initial password = random + show-once panel. ≥12 chars from full ASCII alphabet; `must_change_password = true`.
- D-24: USER-03 "Logout everywhere" = admin button + AlertDialog → DELETEs all SCS session rows where `user_id = $1`. Same DELETE path invoked on: explicit click, disable user, change role, reset password.
- D-25: Role-change semantics = auto-logout-everywhere on change (one of D-24's four triggers).
- D-26: Block self-demote, self-disable, last-admin-demote. UI greys; server enforces. Lockout recovery = `shifter create-admin --reset`.
- D-27: Re-enable preserves original credentials + state. `disabled_at = NULL` resumes with same email/password hash/must_change_password.
- D-28: Password strength policy = reuse Phase 1 evaluator + same min-score for every surface.
- D-29: Users page lives at Settings → Users tab. Route: `/settings/users`.
- D-30: Auth-event audit retrofit (closes Phase 2 D-21). New vocabulary: `auth.login_success`, `auth.login_failed`, `auth.logout`, `auth.password_change`, `auth.password_reset_by_admin`, `auth.session_revoked`, `user.create`, `user.update`, `user.disable`, `user.enable`, `user.role_change`.

**Audit Log — Browse & Export**
- D-31: Audit browse at `/audit`, admin-only sidebar item. Gated by `auth.Can(user, "audit.read")`.
- D-32: Filter UI = URL-state filter chips above table. Chips: Date range, User, Entity type, Action, Request ID. `useSearchParams + zod`.
- D-33: Default view = last 7 days, all entity types, all users.
- D-34: Per-row diff = expandable JsonTree (reuse Phase 4 D-19 component).
- D-35: CSV export mirrors REPT-03 spec + respects current filter chips. UTF-8 BOM, ISO-8601, timezone label. Columns: `time, user_email, user_id, action, entity_type, entity_id, request_id, notes, before_json, after_json`. Cap 50 000 rows; above → River background job.
- D-36: Pagination = cursor pagination by `(time DESC, id)`. Page size 100. "Load more" button.
- D-37: No live-tail / no SSE for audit. Manual "Refresh".
- D-38: Audit retention = 5 years default + Settings toggle. Application-level prune cron (audit_log is regular table, not hypertable, per Phase 2 D-06).

**Backup & Restore (OPS-02..04)**
- D-39: Backup format = single tar.gz containing `pg_dump` + floor-plan dir + manifest.json. Manifest: `{shifter_version, db_schema_version, chirpstack_mode, included, started_at, finished_at, sha256_sums}`.
- D-40: TimescaleDB-aware pg_dump strategy. Choice between (a) `timescaledb-backup` helper or (b) plain `pg_dump --format=custom` + `timescaledb_pre_restore()` / `timescaledb_post_restore()`. **Researcher resolves per ROADMAP flag.**
- D-41: Bundled mode includes ChirpStack DB; external mode is Shifter-only. Detected via `chirpstack_connection.mode`.
- D-42: Backup destination in v1 = local filesystem path only. Configurable via env (`SHIFTER_BACKUP_DIR`) and Settings.
- D-43: Three triggers, one code path. CLI `shifter backup --to <path>`, cron sidecar (bundled mode), Settings → Backup → "Run backup now" button.
- D-44: Restore = `shifter restore --from <tarball>` CLI; requires Shifter down. PG advisory lock; rsync floor plans; verify sha256.
- D-45: OPS-04 CI round-trip = seed → backup → fresh DB → restore → smoke. **Researcher evaluates cross-version restore variant.**

**Settings — Backup status + Identity propagation (SETT-02, SETT-05)**
- D-46: Settings → Backup card (SETT-05). Age dot, thresholds, run-now button, most-recent-5 list with sha256.
- D-47: SETT-02 install-identity propagation = new reports get new branding; old artifacts immutable.

**Operational Hardening polish (OPS-05/06/07/08)**
- D-48: OPS-05/06/07 verification pass + close any gaps. Audit BOTH compose files line-by-line for `<<: *json-logging`, `:latest`, `.env`-mounted credentials.
- D-49: OPS-08 per-release upgrade runbook in `docs/operator-runbook.md`. Five steps: backup → bump tag → pull → verify → rollback procedure.
- D-50: `shifter doctor` CLI for support diagnostic snapshots. Redacted bundle: config-check, health-detailed, last 100 audit rows (email masked `j***@example.com`), alert worker summary, schema version, CS gRPC ping, last 200 log lines via Docker socket (fallback note).

### Claude's Discretion
- Audit-vocabulary migration numbering — planner picks next sequential numbers
- Alert worker River queue name, schedule cron expressions, cool-down enforcement layer
- Cron sidecar image choice (ofelia / willfarrell/crontab / custom Alpine+tini+crond). Default backup time 02:00 install_tz.
- `timescaledb-backup` helper vs raw `pg_dump` + restore-time helpers — **research resolves**
- Alert payload field names (snake_case)
- JsonTree integration into audit table rows (virtualize expanded panel for large diffs)
- Cold-start chip exact copy + progress-bar styling — follows Phase 4 D-21 three-stage pattern
- Sidebar nav reordering. Suggested: Dashboard / Map / Sites / Devices / Gateways / Reports / Alerts / Audit / Settings.
- CLI prompt copy for `create-admin --reset` and `restore --from`
- Random password alphabet for D-23 (suggested: full printable ASCII excluding `1lI0O`; length ≥ 16)

### Deferred Ideas (OUT OF SCOPE)
- S3 / S3-compatible backup destination (V2-OPS-01)
- Scheduled backup UI inside Shifter
- Hot restore / zero-downtime restore
- Email / SMTP alert delivery (V2-NOTIF-02)
- Webhook alert delivery (V2-INT-01) — payload is ready
- Per-user dashboards / saved views / per-rule subscribers (V2-AUTH-02)
- SSO / OAuth / SCIM (V2-AUTH-01)
- Custom roles beyond admin / viewer (V2-AUTH-03)
- Engine dry-run (rule fires-against-last-30-days preview) — Phase 7
- Live-tail SSE for audit browse
- Audit log full-text search
- Cross-version restore CI test — research may pull into Phase 6 scope or defer
- Saved alert rule templates
- audit_log as a TimescaleDB hypertable with compression
- Per-rule cool-down dry-run
- Multi-channel alert routing
- Bulk user import / SSO group sync
- Encrypted backups at rest (operator's responsibility via filesystem encryption)
- Backup retention / rotation inside Shifter
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| ALERT-01 | Configure threshold alerts (per-meter / per-site, daily/hourly/instantaneous, high/low) | D-01, D-02 — three subtypes; SQL queries vs `measurement` + `cagg_hourly` + `cagg_daily`; see § Alert Engine — Threshold Rules |
| ALERT-02 | Device-offline alerts on N≥3 consecutive missed expected uplinks + hysteresis | D-15 spec; SQL pattern in § Offline Evaluator |
| ALERT-03 | Gateway-down suppression (no false device alerts behind a downed gateway) | D-14 stateless eval-time suppression query in § Offline Evaluator |
| ALERT-04 | Statistical anomaly alerts; cold-start gate at 21 days; warm-up silent first 24 h | D-16, D-17; SQL per rule kind in § Statistical Anomaly Rules |
| ALERT-05 | In-app alert center with unread badge, severities, ack with notes, snooze/mute | D-20 bell + drawer + page; React Query + URL state |
| ALERT-06 | Audit log filters + CSV export (date / user / entity type) | D-31..D-38; cursor pagination in § Audit Browse |
| USER-01 | Admin list/create/edit/disable users (no hard-delete) | D-27 — soft delete; § User Management — Store Surface |
| USER-02 | Admin assigns/changes user roles | D-25 — role change → auto-logout-everywhere |
| USER-03 | Admin revokes all sessions for a user ("logout everywhere") | D-24 — reuses existing `iterateAndRevoke`; see § Session Revoke |
| USER-04 | Admin sets initial password inline; user forced to change on next login | D-23 random + show-once; D-28 reuse Phase 1 strength evaluator |
| AUDIT-02 | Browseable audit log with filters | D-31..D-37; § Audit Browse |
| AUDIT-03 | Export audit log as CSV | D-35; § Audit Export |
| SETT-02 | Install identity propagates to new reports; old artifacts immutable | D-47 — no PDF rewriting, ephemeral reports per Phase 5 D-07 |
| SETT-05 | Most-recent-backup timestamp + age threshold in Settings | D-46 — backup card with age dot |
| OPS-02 | TimescaleDB-aware logical backup script | D-39, D-40, D-41; § Backup Strategy |
| OPS-03 | Backup destination configurable | D-42 — local path; SHIFTER_BACKUP_DIR |
| OPS-04 | Restore round-trip tested in CI | D-45; § CI Round-Trip |
| OPS-05 | Container logs use `json-file` with size/file caps on every service | D-48 verification pass against `compose/{bundled,external}.yml` |
| OPS-06 | Secrets via Docker Compose `secrets:` (no `.env`) | D-48 verification pass |
| OPS-07 | Pinned image tags (no `:latest`) | D-48 verification pass |
| OPS-08 | Per-release upgrade runbook with rollback | D-49 — appends to operator-runbook.md |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- `sqlc + pgx` for queries — **never GORM, never lib/pq**. New alert/user-management queries land in `internal/db/sqlc/`.
- `golang-migrate` (plain SQL files) for migrations; existing pattern 0001…0036. Phase 6 numbering picks up at 0037+.
- `River` (already in `go.mod` at v0.36.0) for jobs; `riverdriver/riverpgxv5`. No Redis.
- `SCS` v2 + `pgxstore` sessions; bulk revoke uses existing `iterateAndRevoke` (already in `internal/auth/account.go`).
- `Argon2id` for password hashing; `internal/auth/password_strength.go` is the single strength evaluator (D-28).
- `Chi` router; `slog` for logs.
- `Spf13/cobra + spf13/viper` for new CLI subcommands.
- Frontend: shadcn/ui + Tailwind 4 + React 19 + Vite 7 + TanStack Query + react-router-dom v7 + react-hook-form + zod + sonner + lucide-react.
- All CRUD via `ResponsiveDialog`. URL state via `useSearchParams + zod` (Phase 3 D-15 / Phase 4 D-14 reused).
- audit_log is INSERT-ONLY by trigger — every Phase 6 mutation writes a row in the SAME transaction.

---

## Summary

Phase 6 has three product surfaces — alerts, user management, audit browse — bolted on top of a substrate Phases 1–5 already shipped (audit_log + write-in-tx pattern, SCS sessions in Postgres, River v0.36 + riverpgxv5, Settings card pattern, retention reconciliation). The new code is largely **additive workers + handlers + dialogs**, with two new operator CLI subcommands (`backup`, `restore`, `doctor`).

The single highest-risk research item is **TimescaleDB-aware backup ordering**. After verifying upstream state, the answer is unambiguous: **`timescaledb-backup` is archived since Feb 2022** (last release Jan 2021). TigerData's official recommendation is **plain `pg_dump --format=custom` + `timescaledb_pre_restore()` / `timescaledb_post_restore()` wrappers on the target side**. This is **Option B** from CONTEXT.md D-40. Option A is removed from consideration on grounds of upstream abandonment.

The CI round-trip (D-45 / OPS-04) is achievable in a single GitHub Actions job using testcontainers (already in `go.mod`) and a seed fixture that loads `100 measurements × 1 MP × 1 device × 1 site + 1 floor plan + 5 audit rows`. The cross-version-restore variant (vN backup → vN+1 schema) can be added by chaining a `shifter migrate up` step between drop-and-restore and smoke. **Recommendation: ship the same-version round-trip in Phase 6; defer the cross-version variant to a Phase 6.5 / v1.x gate** (it adds release-management coupling that v1 doesn't yet have).

The alert engine fits naturally into River's existing setup. Three worker types (threshold / offline / anomaly), one queue, distinct cron cadences via `river.PeriodicJob`. Cool-down lives in a DB column on `alert_rule` and is enforced inside the worker (in-engine check before evaluating, no DB partial unique index needed). Worker failure detection (D-22) uses `riverClient.Subscribe(river.EventKindJobFailed)` to flip the `alert_worker_degraded` flag when a job hits `state = "discarded"` (exhausted retries).

User management leverages existing `iterateAndRevoke` already shipping in `internal/auth/account.go` — D-24's "logout everywhere" is the same code path the Phase 1 change-password handler already calls, with `keepToken = ""` so EVERY session including the current one is dropped (when admin acts on themselves … blocked by D-26 self-edit guards). The pattern is already exercise-tested.

**Primary recommendation:** Implement in this wave order: (Wave 0) test infrastructure + new migrations. (Wave 1) alert engine backend — schema, worker scaffolding, threshold subtype, offline subtype, gateway suppression. (Wave 2) anomaly subtype + cold-start gate. (Wave 3) alert center UI — bell + drawer + page. (Wave 4) user management — store extensions + handlers + Users page. (Wave 5) audit browse + export. (Wave 6) backup / restore / doctor CLI + Settings → Backup card + CI round-trip. (Wave 7) OPS verification pass + upgrade runbook + phase closure.

---

## Standard Stack

### Core — Backend (Go)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/riverqueue/river` | v0.36.0 (or v0.37.0) | Alert worker, backup worker, audit-export worker | Already in `go.mod`; Phase 5 wired the client + workers pattern in `internal/cli/serve.go` |
| `github.com/riverqueue/river/riverdriver/riverpgxv5` | v0.36.0 (matched) | River pgx driver | Already in `go.mod`; needed for `riverClient.InsertTx` inside the same pgx.Tx as audit row |
| `github.com/robfig/cron/v3` | v3.0.1 | Cron expression parser for River cron jobs | Required for "02:00 install_tz" yearly-style schedules and per-subtype cadences with timezone awareness |
| `github.com/alexedwards/scs/v2` | v2.9.0 + pgxstore | Bulk session revoke | Already wired in Phase 1; `iterateAndRevoke` pattern proven |
| `github.com/jackc/pgx/v5` | v5.9.2 | Cursor pagination, streaming CSV export | Already used everywhere |
| `github.com/google/uuid` | v1.6.0 | New entity IDs (`alert_rule.id`, `alert.id`, `backup_run.id`) | Existing import |
| `encoding/csv` (stdlib) | — | Audit CSV export | Stdlib; UTF-8 BOM prepend `\xEF\xBB\xBF` (same as REPT-03) |
| `archive/tar`, `compress/gzip`, `crypto/sha256` (stdlib) | — | Backup tarball construction + sha256 manifest | Stdlib; no third-party tarball lib |
| `os/exec` (stdlib) | — | Invoke `pg_dump` / `pg_restore` from CLI | Stdlib; pin binary paths via config |

**Version verification:**
- [VERIFIED: `go list -m -versions github.com/riverqueue/river`] — latest v0.37.0 (May 2026), v0.36.0 currently pinned. v0.36 → v0.37 is non-breaking per River CHANGELOG; planner can choose to bump.
- [VERIFIED: `cat go.mod | grep -E "river|scs|maroto|excelize|cobra|viper"`] — every Phase 6 backend dep is already in go.mod.
- [CITED: github.com/timescale/timescaledb-backup README] — repository **archived 2022-02-19**; upstream recommends raw `pg_dump`/`pg_restore`.

### Core — Frontend (TypeScript/React)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `@tanstack/react-query` | v5.x (already in `package.json`) | Alerts list, audit list, users list, backup list | Existing |
| `@tanstack/react-table` | v8.x (already in `package.json`) | Audit table, alert center page, users page, backup history | Existing |
| `@tanstack/react-virtual` | (already in `package.json`) | Virtualize audit table when row counts exceed 100 | Existing |
| `react-router-dom` | v7.x (already in `package.json`) | `/alerts`, `/audit`, `/settings/users`, `/settings/backup` routes | Existing |
| `react-hook-form` + `@hookform/resolvers/zod` | (already in `package.json`) | Alert rule form, user create/edit form, retention edit | Existing |
| `zod` | (already in `package.json`) | URL state schemas (filter chips), wire schemas | Existing |
| `sonner` | (already in `package.json`) | Toasts for backup completion, alert ack, etc. | Existing |
| `lucide-react` | (already in `package.json`) | `Bell` (alert), `Shield` (audit), `Users`, `Database` (backup), `Stethoscope`/`Activity` (doctor), `AlertTriangle`/`XCircle` (severity) | Existing |
| `shadcn/ui` components | latest CLI | `AlertDialog`, `DropdownMenu`, `Popover`, `Sheet`, `Tabs`, `Switch` | Mostly already added; verify in Wave 0 |

**Frontend new shadcn component check:** Phase 6 may need `Sheet` (slide-over drawer for D-20 alert bell), `AlertDialog` (already added in earlier phases), `Switch` (snooze toggles). Run `npx shadcn-ui@latest add sheet switch` if missing (planner verifies in Wave 0).

### Supporting — Backup / Operator

| Tool | Image / Version | Purpose | Notes |
|------|----------------|---------|-------|
| `pg_dump` / `pg_restore` (PostgreSQL 16 client tools) | Bundled in `timescale/timescaledb:2.26.0-pg16` image | Backup + restore | Already available inside the `postgres` container; `docker exec postgres pg_dump …` is the canonical pattern. **Shifter binary need not bundle pg client tools; it shells out via `docker exec` OR via direct libpq connection inside the postgres container** — see § Backup Strategy. |
| `mcuadros/ofelia` | v0.3.22 (Apr 2026) | Cron sidecar for bundled-mode nightly backup | Recommended. Actively maintained, written in Go, supports `job-exec` against existing containers via Docker API. Smaller and better-maintained than `willfarrell/crontab` (Alpine + crond) and far less fragile than custom tini+crond. |
| `tar`, `gzip` (Alpine util) | Inside `shifter` image | Tarball construction | The Shifter image already has these; `archive/tar` + `compress/gzip` in stdlib avoids any subprocess. Pure Go is preferred. |

**Cron sidecar comparison:**
- [VERIFIED: github.com/mcuadros/ofelia releases] — v0.3.22 released April 2026. 3.8k stars, 35 releases, written in Go, actively maintained.
- `willfarrell/crontab` — Alpine + crond + supercronic; last release 2022; not actively maintained.
- Custom Alpine + tini + crond — minimal but operator must hand-roll the Docker-socket-mount pattern.
- **Pick: `mcuadros/ofelia:v0.3.22`**. INI-config or docker-labels. `job-exec` invokes `docker exec shifter shifter backup --to /var/lib/shifter/backups` against the running shifter container. Schedule: `@daily` or `0 2 * * *` (default 02:00 install_tz).

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| **Option B (raw pg_dump + pre/post_restore)** | Option A (`timescaledb-backup` helper) | **Option A is archived since 2022; do not use** |
| `mcuadros/ofelia` | `willfarrell/crontab` | willfarrell unmaintained since 2022 |
| `mcuadros/ofelia` | Custom Alpine + tini + crond | More YAML, more fragile, no Docker API integration |
| River subscribe → degraded flag | DB partial unique index for cool-down | Cool-down naturally fits a worker-side check on `last_fired_at + cooldown_seconds < now()`; index adds insert contention; planner picks |
| `os/exec pg_dump` from shifter container | `docker exec postgres pg_dump` orchestrated from a sidecar | `docker exec` requires Docker socket mount → security cost. Shifter shells out to `pg_dump` it bundles in its own image (lightweight: postgres client = ~15MB) OR connects directly to postgres pool and uses a pure-Go pg_dump alternative. **Recommendation: bundle `postgresql16-client` in the Shifter Dockerfile** — simpler, security-friendly, works for both bundled and external modes. |
| `archive/tar` stdlib | `mholt/archiver` third-party | Stdlib is fine for tar+gz; no extra dep |

**Installation (new deps only):**
```bash
# Go — only one new dep
cd /path/to/project
go get github.com/robfig/cron/v3@v3.0.1

# Frontend — no new deps, verify shadcn components
cd web
pnpm dlx shadcn-ui@latest add sheet switch  # if missing
```

---

## Architecture Patterns

### Recommended Project Structure Extensions

```
internal/
├── alert/                        # NEW: alert engine + storage + API
│   ├── engine.go                 # EvaluateContext, shared types
│   ├── rule_store.go             # alert_rule CRUD (sqlc-backed)
│   ├── alert_store.go            # alert CRUD (sqlc-backed)
│   ├── threshold_worker.go       # River worker — threshold_instantaneous/hourly/daily
│   ├── offline_worker.go         # River worker — device offline + gateway suppression
│   ├── anomaly_worker.go         # River worker — p95 / iqr / quiet_hour
│   ├── degraded.go               # River subscriber → alert_worker_degraded flag
│   ├── handler.go                # HTTP: GET/POST/PATCH /api/alerts, /api/alerts/rules
│   └── doc.go
├── user/                         # NEW: user management (extends auth.Store)
│   ├── store.go                  # List, Create, Update, Disable, Enable, ChangeRole
│   ├── handler.go                # /api/users CRUD + /logout-everywhere
│   ├── password.go               # Random password generator (D-23)
│   └── guards.go                 # D-26 server-side blocks (self-demote, self-disable, last-admin)
├── audit/                        # EXISTING: log.go — extend with browse
│   ├── log.go                    # (existing — write-in-tx)
│   ├── browse_store.go           # NEW: List(filter, cursor) + count
│   ├── handler.go                # NEW: GET /api/audit, GET /api/audit/export
│   └── export.go                 # NEW: CSV streaming + River worker for >50k
├── backup/                       # NEW: backup/restore runtime
│   ├── store.go                  # backup_run CRUD
│   ├── runner.go                 # Backup(ctx, dest) — pg_dump + tar + manifest
│   ├── restore.go                # Restore(ctx, src) — un-tar + pre_restore + pg_restore + post_restore
│   ├── manifest.go               # manifest.json schema + verify
│   ├── handler.go                # GET /api/backup/list, POST /api/backup/run-now
│   └── doc.go
├── doctor/                       # NEW: diagnostic bundle
│   ├── doctor.go                 # snapshot + redact
│   └── redact.go                 # PII redaction (email masking)
├── cli/                          # EXISTING: extend with new subcommands
│   ├── backup.go                 # NEW: `shifter backup --to <path>`
│   ├── restore.go                # NEW: `shifter restore --from <tar>`
│   └── doctor.go                 # NEW: `shifter doctor` (file or stdout)
├── auth/                         # EXISTING: extend authz vocabulary
│   ├── authz.go                  # Add ActionAlert* / ActionUser* / ActionBackup* / etc.
│   ├── handlers.go               # Retrofit Plan 01-09 — write audit in tx (D-30)
│   └── account.go                # Retrofit Plan 01-11 — write audit in tx (D-30)
├── settings/                     # EXISTING: extend retention card + backup card
│   ├── retention.go              # Extend with alerts_days + audit_log_days rows
│   ├── backup_card.go            # NEW: GET /api/settings/backup-status
│   └── identity.go               # SETT-02: no change beyond Phase 1 — D-47 = behavior only
└── db/
    └── migrations/
        ├── 0037_audit_vocab_phase6.up.sql        # +12 actions, +5 entity types
        ├── 0038_alert_rule.up.sql                 # rules table
        ├── 0039_alert.up.sql                      # fired-alert table
        ├── 0040_retention_config_phase6.up.sql    # +alerts_days, +audit_log_days cols
        ├── 0041_backup_run.up.sql                 # backup history
        └── 0042_alert_worker_state.up.sql         # worker observability row (singleton)

web/src/
├── routes/
│   ├── alerts/
│   │   ├── index.tsx                 # /alerts page
│   │   ├── AlertCenterFilters.tsx
│   │   ├── AlertTable.tsx
│   │   ├── AckDialog.tsx
│   │   ├── SnoozeMenu.tsx
│   │   └── RuleDialog.tsx
│   ├── audit/
│   │   ├── index.tsx                 # /audit page
│   │   ├── AuditFilters.tsx          # URL-state chips
│   │   ├── AuditTable.tsx
│   │   └── DiffPanel.tsx             # JsonTree side-by-side
│   └── settings/
│       ├── users.tsx                 # /settings/users (D-29)
│       └── backup.tsx                # /settings/backup (D-46)
└── components/
    └── shell/
        ├── AlertBell.tsx             # D-20 bell + Sheet drawer
        ├── DegradedBanner.tsx        # D-22 banner
        └── sidebar.tsx               # NEW items: Alerts, Audit
```

---

## Decision A: Backup Strategy — Raw `pg_dump` + Pre/Post Restore Hooks (resolves D-40)

### Recommendation

**Use plain `pg_dump --format=custom` for backup and `timescaledb_pre_restore()` / `timescaledb_post_restore()` on the target for restore.** This is Option B from D-40.

### Why

[CITED: github.com/timescale/timescaledb-backup README] — repository was archived 2022-02-19 with upstream's explicit recommendation: *"We recommend using the PostgreSQL tools `pg_dump` and `pg_restore` instead. For more information on using these tools with TimescaleDB, please refer to the TimescaleDB backup and restore documentation."*

Bundling an archived helper into a v1 release is a future-maintenance landmine. The pre/post_restore wrappers ship inside the TimescaleDB extension itself — they cannot drift.

### Exact Backup Command

```bash
# Inside the Shifter container (recommended path — bundles postgresql16-client in the image)
pg_dump \
    --host=postgres \
    --port=5432 \
    --username=shifter \
    --dbname=shifter \
    --format=custom \
    --no-owner \
    --no-acl \
    --file=/tmp/shifter-<timestamp>.bak

# In bundled mode, also dump ChirpStack's DB (lives in same Postgres instance)
pg_dump \
    --host=postgres \
    --port=5432 \
    --username=chirpstack \
    --dbname=chirpstack \
    --format=custom \
    --no-owner \
    --no-acl \
    --file=/tmp/chirpstack-<timestamp>.bak
```

**Flag rationale:**
- `--format=custom` (-Fc): binary custom format. **REQUIRED** by `pg_restore` (the alternative — plain SQL — does not support pre_restore mode).
- `--no-owner`, `--no-acl`: dump is portable across environments (different DB users on restore target).
- **NEVER use `--jobs=N`** on `pg_restore`. [CITED: tigerdata.com docs] *"Do not use `pg_restore` with the `-j` option. This option does not correctly restore the TimescaleDB catalogs."* Parallelization breaks the metadata ordering that TimescaleDB requires.

### Exact Restore Procedure (full sequence)

```bash
# Step 1: Stop Shifter (operator runs)
docker compose stop shifter

# Step 2: Restore — Shifter CLI orchestrates the next steps
docker compose run --rm shifter shifter restore --from /var/lib/shifter/backups/<file>.tar.gz
```

Inside `shifter restore`:

```go
// internal/backup/restore.go — pseudocode of the operations
// 1. Acquire PG advisory lock (refuse if Shifter is still serving)
//    SELECT pg_try_advisory_lock(0x5HIFTER1); -- if false: abort with helpful error
// 2. Verify the tarball sha256 against manifest.json
// 3. Extract floor plans to /var/lib/shifter/floor-plans (rsync style)
// 4. For each DB in the manifest (Shifter, optionally ChirpStack):
//    a. Drop and recreate the DB:
//       DROP DATABASE IF EXISTS shifter;
//       CREATE DATABASE shifter;
//    b. Connect to the new DB, install TimescaleDB:
//       CREATE EXTENSION IF NOT EXISTS timescaledb;
//    c. Run the pre-restore hook:
//       SELECT timescaledb_pre_restore();
//    d. Restore from the dump:
//       pg_restore --host=postgres --username=shifter --dbname=shifter \
//                  --no-owner --no-acl \
//                  /tmp/extracted/db/shifter.bak
//    e. Run the post-restore hook:
//       SELECT timescaledb_post_restore();
// 5. Release the advisory lock
// 6. Write audit row: action=backup.restore, entity_type=backup_run, entity_id=manifest.id
```

Then operator runs:

```bash
# Step 3: Start Shifter (which performs a final `migrate up` to ensure schema is current)
docker compose start shifter
```

[CITED: docs.tigerdata.com/migrate/latest/pg-dump-and-restore] verifies this exact ordering.

### CAGG Survival Across Restore

[CITED: tigerdata.com hierarchical CAGG docs] — Continuous aggregates and their refresh-policy / retention-policy state are preserved in `pg_dump --format=custom` output via the `_timescaledb_catalog.*` tables. The pre_restore() function disables background workers (CAGG refresh + retention) for the duration of the load, and post_restore() re-arms them. The hierarchical CAGG chain Phase 5 shipped (hourly → daily → monthly → yearly) restores as a single transaction within the custom-format pg_restore.

**One gotcha:** if the source DB had CAGG-over-CAGG dependencies, the dump's dependency ordering must match. `pg_dump --format=custom` handles this automatically — no manual `--exclude-table` / `--include-table` shenanigans are needed.

### Bundled-Mode Ordering (ChirpStack DB)

Both Shifter's and ChirpStack's databases live in the same Postgres instance (per `compose/bundled.yml`). The Phase 6 backup tarball includes both, BUT they are **independent dumps** — Shifter does not reference ChirpStack's schema and vice versa (Shifter integrates with ChirpStack over gRPC, never over Postgres).

**Restore order is NOT load-bearing for correctness** but IS load-bearing for operator UX:
1. **Restore Shifter first.** If Shifter restore fails, the operator hasn't disturbed ChirpStack.
2. **Restore ChirpStack second** (bundled mode only).
3. ChirpStack has no TimescaleDB tables, so it skips the pre/post_restore wrappers — plain `pg_restore --no-owner --no-acl chirpstack.bak` works.

### Manifest Schema (D-39)

```json
{
  "manifest_version": "1.0",
  "shifter_version": "0.6.0",
  "db_schema_version": "0042",
  "chirpstack_mode": "bundled",
  "chirpstack_db_included": true,
  "install_id": "01HXYZ...",
  "install_slug": "acme-bangkok",
  "started_at": "2026-05-13T02:00:00Z",
  "finished_at": "2026-05-13T02:01:34Z",
  "included": [
    "db/shifter.bak",
    "db/chirpstack.bak",
    "floor-plans/"
  ],
  "sha256_sums": {
    "db/shifter.bak": "abc123...",
    "db/chirpstack.bak": "def456...",
    "floor-plans/<uuid>.png": "ghi789...",
    "manifest.json": null
  }
}
```

`manifest.json`'s own sha256 is NOT included (it would be self-referential). The tarball's outer sha256 (stored in `backup_run.sha256`) covers the whole artifact.

### Bundling pg client tools in Shifter image

The current Shifter Dockerfile uses a `scratch` or `distroless` final stage (verify in Wave 0). For Phase 6 the final image MUST include `pg_dump` + `pg_restore` matching the Postgres major version pinned in compose:

```dockerfile
# At top of Phase 6 Dockerfile change:
# Add the PostgreSQL 16 client tools (~15 MB)
FROM alpine:3.20 AS pgclient
RUN apk add --no-cache postgresql16-client

FROM <existing base> AS shifter
COPY --from=pgclient /usr/bin/pg_dump /usr/bin/pg_dump
COPY --from=pgclient /usr/bin/pg_restore /usr/bin/pg_restore
COPY --from=pgclient /usr/bin/psql /usr/bin/psql
```

[ASSUMED — Wave 0 verification step] — if the current Dockerfile is already alpine-based, this is a one-line `apk add postgresql16-client` in the existing stage. Planner verifies in the Wave 0 task.

### Risks

| Risk | Probability | Mitigation |
|------|-------------|------------|
| pg_dump version mismatch (client 16, server 17) | LOW | The bundled-mode Postgres is pinned to `timescale/timescaledb:2.26.0-pg16` — both sides are PG16. External mode operators document their PG version in install state; if they upgrade to PG17, the Shifter image's pg_dump 16 still works against PG17 servers (pg_dump is forward-compatible). |
| `pg_restore --jobs=N` accidentally used | LOW | Planner: enforce in code; the only caller is `internal/backup/restore.go` and that file MUST NOT use `--jobs`. Lint via grep test in CI. |
| Backup of ChirpStack DB when external mode | NONE | `chirpstack_connection.mode = 'external'` → backup runner skips ChirpStack dump entirely. |
| Floor plan files modified mid-backup | LOW | Backup runner takes a `rsync --link-dest` style snapshot; or operator accepts that any uploaded-during-backup files will be missing — documented in runbook. |
| Schema version drift on restore | MEDIUM | After restore, `shifter migrate up` is auto-invoked on next `docker compose up`. The cross-version variant in CI verifies this works. |

### References

- [CITED: github.com/timescale/timescaledb-backup README] — archived 2022-02-19
- [CITED: docs.tigerdata.com/self-hosted/latest/backup-and-restore/logical-backup] — pg_dump -Fc; **never use -j on pg_restore**
- [CITED: docs.tigerdata.com/api/latest/administration/timescaledb_pre_restore] — pre/post restore semantics
- [CITED: docs.tigerdata.com/migrate/latest/pg-dump-and-restore] — full migration runbook (same sequence as restore)

---

## Decision B: CI Round-Trip Test (resolves D-45 / OPS-04)

### Recommendation

**Ship same-version round-trip test in Phase 6. Defer cross-version restore variant to a Phase 6.5 / v1.x gate.**

Cross-version restore adds release-management coupling (need a pinned "previous version" image to dump from), and adds CI flakiness when migrations land. For v1.0, the same-version round-trip is a meaningful gate. The cross-version variant becomes essential when there are paying customers on v1 and v1.1 ships — which is post-Phase-6.

### Exact CI Job Shape

```yaml
# .github/workflows/backup-restore-roundtrip.yml
name: backup-restore-roundtrip

on:
  pull_request:
    paths:
      - 'internal/backup/**'
      - 'internal/cli/backup.go'
      - 'internal/cli/restore.go'
      - 'internal/db/migrations/**'
  push:
    branches: [main]

jobs:
  roundtrip:
    runs-on: ubuntu-latest
    timeout-minutes: 15
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'

      - name: Run round-trip test
        run: go test ./internal/backup/... -run TestBackupRestoreRoundtrip -count=1 -v
        env:
          # testcontainers handles Postgres+TimescaleDB lifecycle
          TESTCONTAINERS_RYUK_DISABLED: "false"
```

### Test Implementation

```go
// internal/backup/roundtrip_test.go
//go:build integration

package backup_test

import (
    "context"
    "os"
    "path/filepath"
    "testing"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/stretchr/testify/require"
    "github.com/testcontainers/testcontainers-go"
    tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
    "github.com/shifter-io/shifter/internal/backup"
    "github.com/shifter-io/shifter/internal/db"
)

// TestBackupRestoreRoundtrip enforces OPS-04 in CI:
//   seed → backup → fresh DB → restore → smoke
func TestBackupRestoreRoundtrip(t *testing.T) {
    ctx := context.Background()

    // 1. Spin up PG+TimescaleDB via testcontainers
    pg, err := tcpostgres.RunContainer(ctx,
        testcontainers.WithImage("timescale/timescaledb:2.26.0-pg16"),
        tcpostgres.WithDatabase("shifter"),
        tcpostgres.WithUsername("shifter"),
        tcpostgres.WithPassword("test"),
    )
    require.NoError(t, err)
    defer pg.Terminate(ctx)

    pool, err := pgxpool.New(ctx, pg.MustConnectionString(ctx))
    require.NoError(t, err)
    defer pool.Close()

    // 2. Apply migrations
    require.NoError(t, db.RunMigrations(ctx, pool))

    // 3. Seed fixture: 1 site, 1 MP, 1 device, 100 measurements, 1 floor plan, 5 audit rows
    seedFixture(t, ctx, pool)

    // 4. Backup
    tmpDir := t.TempDir()
    tarPath := filepath.Join(tmpDir, "test-backup.tar.gz")
    runner := backup.NewRunner(pool, /* config */)
    require.NoError(t, runner.Backup(ctx, tarPath))
    fi, _ := os.Stat(tarPath)
    require.Greater(t, fi.Size(), int64(1024)) // non-empty

    // 5. Drop+recreate DB (simulate disaster recovery)
    _, err = pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`)
    require.NoError(t, err)
    _, err = pool.Exec(ctx, `CREATE EXTENSION timescaledb;`)
    require.NoError(t, err)

    // 6. Restore
    restorer := backup.NewRestorer(pool, /* config */)
    require.NoError(t, restorer.Restore(ctx, tarPath))

    // 7. Smoke assertions (one SELECT per table + 1 CAGG check)
    var siteCount int
    require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM site`).Scan(&siteCount))
    require.Equal(t, 1, siteCount)

    var mpCount int
    require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM metering_point`).Scan(&mpCount))
    require.Equal(t, 1, mpCount)

    var measurementCount int
    require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM measurement`).Scan(&measurementCount))
    require.Equal(t, 100, measurementCount)

    var auditCount int
    require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM audit_log`).Scan(&auditCount))
    require.Equal(t, 5, auditCount)

    // 8. CAGG survival assertion
    var caggRowCount int
    require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM measurement_daily`).Scan(&caggRowCount))
    require.Greater(t, caggRowCount, 0) // hierarchy refreshed at least once
}

// seedFixture inserts the canonical test fixture (deterministic IDs, 100 measurements,
// 1 floor plan, 5 audit rows).
func seedFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) { /* … */ }
```

### Cross-Version Variant — Why Defer

A cross-version restore test would:
1. Run `git checkout v0.5.0 && go build` to produce a "previous version" Shifter binary
2. Run migrations against a fresh DB to schema vN
3. Seed + backup using v0.5.0
4. Restore on the v0.6.0 (current) binary
5. Run `shifter migrate up` to advance to vN+1 schema
6. Smoke test

This is doable but introduces a binary-checkout step that doubles CI runtime and creates coupling between Phase 6 and any earlier-version-build infrastructure (Phase 1 didn't ship a versioned release CI). It's the right test to gate v1.1+ releases against, but Phase 6 is not the place to invest in it.

**Recommendation: file as a v1.1 prep item; the manifest's `db_schema_version` field is the forward-compat hook that v1.1's restore can validate against.**

### Risks

| Risk | Probability | Mitigation |
|------|-------------|------------|
| Testcontainers TimescaleDB image pull slow on CI | MEDIUM | Pin a tag; rely on GHA Docker cache |
| Test flakiness from CAGG-refresh-timing | LOW | Seed includes the CAGG refresh step explicitly (`CALL refresh_continuous_aggregate(...)`) before backup |
| pg_dump version mismatch in CI | LOW | Use the same image (`timescale/timescaledb:2.26.0-pg16`) — pg_dump runs inside that container |

### References

- [VERIFIED: `cat go.mod`] — testcontainers v0.42.0 + tcpostgres v0.42.0 already present
- [CITED: GitHub Actions docs] — `actions/setup-go@v5`

---

## Decision C: River Alert Engine Architecture (resolves D-01..D-22)

### River Cron Patterns — Verified

[CITED: riverqueue.com/docs/periodic-jobs] verifies:
- `river.PeriodicInterval(N*time.Minute)` for fixed-cadence jobs.
- `cron.ParseStandard("0 2 * * *")` (robfig/cron/v3) for cron expressions; River accepts the schedule directly because their `PeriodicSchedule` interface matches.
- Timezone-prefixed cron: `cron.ParseStandard("CRON_TZ=Asia/Bangkok 0 2 * * *")`.
- Periodic jobs use leader election — only one worker manages periodic schedules at a time across the cluster. Safe for the single-tenant single-Shifter-instance install.

### Three Workers, Three Cadences, One Queue

```go
// internal/cli/serve.go (extension after Phase 5's River setup)
// Workers (added to riverWorkers)
river.AddWorker(riverWorkers, &alert.ThresholdInstantaneousWorker{Pool: pool, Queries: q, Hub: hub, Log: log})
river.AddWorker(riverWorkers, &alert.ThresholdHourlyWorker{Pool: pool, Queries: q, Hub: hub, Log: log})
river.AddWorker(riverWorkers, &alert.ThresholdDailyWorker{Pool: pool, Queries: q, Hub: hub, Log: log})
river.AddWorker(riverWorkers, &alert.OfflineWorker{Pool: pool, Queries: q, Hub: hub, Log: log})
river.AddWorker(riverWorkers, &alert.AnomalyWorker{Pool: pool, Queries: q, Hub: hub, Log: log})
river.AddWorker(riverWorkers, &alert.AuditPruneWorker{Pool: pool, Queries: q, Log: log})

// Periodic jobs (added to riverPeriodicJobs)
periodicJobs := []*river.PeriodicJob{
    // Phase 5 (existing)
    river.NewPeriodicJob(river.PeriodicInterval(1*time.Hour),
        func() (river.JobArgs, *river.InsertOpts) { return report.CleanupExpiredReportsArgs{}, nil },
        &river.PeriodicJobOpts{RunOnStart: false}),

    // Phase 6 — threshold subtypes
    river.NewPeriodicJob(river.PeriodicInterval(1*time.Minute),
        func() (river.JobArgs, *river.InsertOpts) { return alert.ThresholdInstantaneousArgs{}, nil },
        nil),
    river.NewPeriodicJob(river.PeriodicInterval(15*time.Minute),
        func() (river.JobArgs, *river.InsertOpts) { return alert.ThresholdHourlyArgs{}, nil },
        nil),
    river.NewPeriodicJob(river.PeriodicInterval(1*time.Hour),
        func() (river.JobArgs, *river.InsertOpts) { return alert.ThresholdDailyArgs{}, nil },
        nil),

    // Offline + anomaly
    river.NewPeriodicJob(river.PeriodicInterval(2*time.Minute),
        func() (river.JobArgs, *river.InsertOpts) { return alert.OfflineArgs{}, nil },
        nil),
    river.NewPeriodicJob(river.PeriodicInterval(1*time.Hour),
        func() (river.JobArgs, *river.InsertOpts) { return alert.AnomalyArgs{}, nil },
        nil),

    // Audit prune (D-38: 5y default)
    mustParseSchedule("CRON_TZ=" + installTZ + " 0 3 * * *", alert.AuditPruneArgs{}), // 03:00 install_tz daily
}

// Single queue (reuse river.QueueDefault) — uniform retry policy & observability
riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
    Queues:       map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 8}}, // bumped from 4 in Phase 5
    Workers:      riverWorkers,
    PeriodicJobs: periodicJobs,
})
```

### Per-Worker Retry Policy (D-22 — max 3 attempts)

[CITED: riverqueue.com/docs/job-retries] verifies the `JobArgsWithInsertOpts` interface:

```go
// internal/alert/threshold_worker.go
type ThresholdInstantaneousArgs struct{}

func (ThresholdInstantaneousArgs) Kind() string { return "alert_threshold_instantaneous" }

// InsertOpts caps retries at 3 per D-22 (then the run is "discarded" and our
// subscriber flips alert_worker_degraded=true).
func (ThresholdInstantaneousArgs) InsertOpts() river.InsertOpts {
    return river.InsertOpts{
        MaxAttempts: 3,
    }
}

type ThresholdInstantaneousWorker struct {
    river.WorkerDefaults[ThresholdInstantaneousArgs]
    Pool    *pgxpool.Pool
    Queries *sqlc.Queries
    Hub     *events.Hub
    Log     *slog.Logger
}

func (w *ThresholdInstantaneousWorker) Work(ctx context.Context, job *river.Job[ThresholdInstantaneousArgs]) error {
    start := time.Now()
    var rulesEvaluated, firesEmitted, cleared int
    var firstErr error

    // 1. Load all enabled threshold_instantaneous rules
    rules, err := w.Queries.ListEnabledRulesByKind(ctx, "threshold_instantaneous")
    if err != nil { return fmt.Errorf("list rules: %w", err) }

    // 2. For each rule, evaluate against latest measurement for the target MP(s)
    for _, rule := range rules {
        // Cool-down check (D-05) — in-engine, not DB-side
        if rule.LastFiredAt != nil &&
            time.Since(*rule.LastFiredAt) < time.Duration(rule.CooldownSeconds)*time.Second {
            continue
        }

        latest, err := w.Queries.GetLatestMeasurementForMP(ctx, rule.MeteringPointID)
        if err != nil { firstErr = err; continue }

        rulesEvaluated++
        if breach(rule, latest) {
            if err := w.fireAlert(ctx, rule, latest); err != nil { firstErr = err; continue }
            firesEmitted++
        } else if rule.LastFiredAt != nil {
            // Auto-clear (D-08)
            if err := w.clearAlert(ctx, rule); err != nil { firstErr = err; continue }
            cleared++
        }
    }

    // 3. Update alert_worker_state singleton row
    _ = w.Queries.UpsertAlertWorkerState(ctx, sqlc.UpsertAlertWorkerStateParams{
        WorkerKind:       "threshold_instantaneous",
        LastRunAt:        time.Now(),
        RulesEvaluated:   int32(rulesEvaluated),
        FiresEmitted:     int32(firesEmitted),
        Cleared:          int32(cleared),
        DurationMs:       int32(time.Since(start).Milliseconds()),
        Degraded:         false,
        LastError:        nullableErr(firstErr),
    })

    return firstErr // returning error triggers River retry
}
```

### Degraded Detection via River Subscribe (D-22)

[CITED: riverqueue.com/docs/subscriptions] verifies the pattern. The subscriber goroutine listens for `EventKindJobFailed` and, when `job.State == rivertype.JobStateDiscarded`, flips the per-worker `degraded` flag.

```go
// internal/alert/degraded.go
func StartDegradedSubscriber(ctx context.Context, riverClient *river.Client[pgx.Tx], q *sqlc.Queries, log *slog.Logger) {
    sub, cancel := riverClient.Subscribe(river.EventKindJobFailed)
    go func() {
        defer cancel()
        for event := range sub {
            if event == nil { return } // client shutting down
            j := event.Job
            if j.State != rivertype.JobStateDiscarded { continue }

            // Only flip degraded for alert-worker job kinds
            kind := j.Kind
            if !isAlertWorkerKind(kind) { continue }

            log.Error("alert_worker_degraded",
                "kind", kind,
                "attempts", j.Attempt,
                "last_error", lastError(j),
            )

            _ = q.MarkAlertWorkerDegraded(ctx, sqlc.MarkAlertWorkerDegradedParams{
                WorkerKind: kind,
                LastError:  pgtype.Text{String: lastError(j), Valid: true},
            })
        }
    }()
}

func isAlertWorkerKind(k string) bool {
    switch k {
    case "alert_threshold_instantaneous",
        "alert_threshold_hourly",
        "alert_threshold_daily",
        "alert_offline",
        "alert_anomaly":
        return true
    }
    return false
}
```

### Cool-Down Enforcement Layer

**Recommendation: in-engine (worker-side), not DB partial unique index.**

Rationale:
- Cool-down is a soft "don't re-fire for N seconds" — not a uniqueness invariant
- The worker has the rule row in memory already; checking `LastFiredAt + Cooldown < now()` is one comparison, zero query overhead
- A partial unique index `WHERE cooldown_expires_at > now()` would require updating `cooldown_expires_at` on every fire AND prevent legitimate dual-firing across rule-target pairs that share an entity
- Audit trail: every fire still writes a row; cool-down skips are not audited (a deliberate choice — operator would see flapping logs otherwise)

Cool-down expressed in the schema:

```sql
-- in 0038_alert_rule.up.sql
cooldown_seconds   INTEGER NOT NULL DEFAULT 900 CHECK (cooldown_seconds >= 0),
last_fired_at      TIMESTAMPTZ,
```

Worker:

```go
if rule.LastFiredAt.Valid &&
    time.Since(rule.LastFiredAt.Time) < time.Duration(rule.CooldownSeconds)*time.Second {
    continue // suppressed by cool-down
}
```

### Alert Schema Sketch (D-12 payload + D-13 retention)

```sql
-- internal/db/migrations/0038_alert_rule.up.sql
CREATE TABLE alert_rule (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_kind        TEXT NOT NULL CHECK (rule_kind IN (
        'threshold_instantaneous','threshold_hourly','threshold_daily',
        'offline_device','offline_gateway',
        'anomaly_p95','anomaly_iqr','anomaly_quiet_hour'
    )),
    scope_kind       TEXT NOT NULL CHECK (scope_kind IN ('metering_point','site','device','gateway')),
    scope_id         UUID NOT NULL, -- references the entity by scope_kind
    -- threshold-specific
    high_bound       DOUBLE PRECISION,
    low_bound        DOUBLE PRECISION,
    -- common
    severity         TEXT NOT NULL DEFAULT 'critical'
                     CHECK (severity IN ('info','warning','critical')),
    name             TEXT, -- D-06 human override; auto-generated default if NULL
    notes            TEXT, -- D-06
    cooldown_seconds INTEGER NOT NULL DEFAULT 900 CHECK (cooldown_seconds >= 0),
    last_fired_at    TIMESTAMPTZ,
    disabled_at      TIMESTAMPTZ, -- D-04 soft-delete
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX alert_rule_active_idx ON alert_rule (rule_kind) WHERE disabled_at IS NULL;
CREATE INDEX alert_rule_scope_idx  ON alert_rule (scope_kind, scope_id) WHERE disabled_at IS NULL;

-- internal/db/migrations/0039_alert.up.sql
CREATE TABLE alert (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rule_id         UUID NOT NULL REFERENCES alert_rule(id),
    rule_kind       TEXT NOT NULL,  -- denormalized for fast query
    severity        TEXT NOT NULL CHECK (severity IN ('info','warning','critical')),
    state           TEXT NOT NULL DEFAULT 'firing'
                    CHECK (state IN ('firing','acknowledged','cleared','muted','snoozed')),
    payload         JSONB NOT NULL, -- D-12 stable schema (see below)
    target_entity_type TEXT NOT NULL,
    target_entity_id   UUID NOT NULL,
    is_test         BOOLEAN NOT NULL DEFAULT FALSE,
    fired_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    cleared_at      TIMESTAMPTZ,
    acked_at        TIMESTAMPTZ,
    acked_by        UUID REFERENCES "user"(id) ON DELETE SET NULL,
    ack_note        TEXT,
    snoozed_until   TIMESTAMPTZ,
    snoozed_by      UUID REFERENCES "user"(id) ON DELETE SET NULL,
    muted           BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX alert_firing_idx ON alert (fired_at DESC) WHERE state = 'firing';
CREATE INDEX alert_state_idx  ON alert (state, fired_at DESC);
CREATE INDEX alert_target_idx ON alert (target_entity_type, target_entity_id, fired_at DESC);
```

### D-12 Payload Schema (canonical)

```jsonc
// alert.payload — same shape as future webhook body (V2-NOTIF-01 ships by adding a deliverer)
{
  "rule_id":     "01HXYZ...",
  "rule_kind":   "threshold_hourly",
  "severity":    "critical",
  "target": {
    "entity_type": "metering_point",
    "entity_id":   "01HABCD...",
    "label":       "Building A — Cold Water Main"
  },
  "value":       42.7,
  "threshold":   40.0,
  "comparison":  "gt",                 // "gt" | "gte" | "lt" | "lte" | "eq"
  "unit":        "m3/h",
  "fired_at":    "2026-05-13T02:14:08Z",
  "install": {
    "display_name": "Acme Bangkok HQ"
  }
}
```

**Example payloads per rule kind:**

```jsonc
// threshold breach
{ "rule_kind":"threshold_instantaneous", "severity":"critical",
  "target":{"entity_type":"metering_point","entity_id":"...","label":"Floor 3 chiller"},
  "value":12.4, "threshold":10.0, "comparison":"gt", "unit":"kW", "fired_at":"...",
  "install":{"display_name":"Acme Bangkok HQ"} }

// device offline
{ "rule_kind":"offline_device", "severity":"critical",
  "target":{"entity_type":"device","entity_id":"...","label":"Pulse meter MP-042"},
  "value":3, "threshold":3, "comparison":"gte", "unit":"missed_uplinks",
  "last_uplink_at":"2026-05-13T00:14:08Z", "expected_interval_s":900,
  "fired_at":"...", "install":{"display_name":"Acme Bangkok HQ"} }

// gateway offline (suppresses devices behind it per D-14)
{ "rule_kind":"offline_gateway", "severity":"critical",
  "target":{"entity_type":"gateway","entity_id":"...","label":"GW-Rooftop-A"},
  "value":2400, "threshold":900, "comparison":"gt", "unit":"seconds_since_last_seen",
  "suppresses_n_devices":12,
  "fired_at":"...", "install":{"display_name":"Acme Bangkok HQ"} }

// anomaly p95 breach
{ "rule_kind":"anomaly_p95", "severity":"critical",
  "target":{"entity_type":"metering_point","entity_id":"...","label":"Floor 1 main"},
  "value":85.2, "threshold":62.7, "comparison":"gt", "unit":"m3/h",
  "p95_baseline":62.7, "baseline_window_days":30, "time_of_day_bucket":"14:00",
  "fired_at":"...", "install":{"display_name":"Acme Bangkok HQ"} }
```

### Alert Worker Observability Row (D-21)

```sql
-- 0042_alert_worker_state.up.sql
CREATE TABLE alert_worker_state (
    worker_kind        TEXT PRIMARY KEY CHECK (worker_kind IN (
        'threshold_instantaneous','threshold_hourly','threshold_daily',
        'offline','anomaly'
    )),
    last_run_at        TIMESTAMPTZ NOT NULL,
    rules_evaluated    INTEGER NOT NULL DEFAULT 0,
    fires_emitted      INTEGER NOT NULL DEFAULT 0,
    cleared            INTEGER NOT NULL DEFAULT 0,
    duration_ms        INTEGER NOT NULL DEFAULT 0,
    degraded           BOOLEAN NOT NULL DEFAULT FALSE,
    last_error         TEXT,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Seed all 5 worker_kind rows at install_finish so the table is always populated.
```

`/health/detailed` is extended in `internal/http/health.go`:

```go
// alert_worker section
type alertWorkerHealth struct {
    Kind           string    `json:"kind"`
    LastRunAt      time.Time `json:"last_run_at"`
    RulesEvaluated int       `json:"rules_evaluated"`
    FiresEmitted   int       `json:"fires_emitted"`
    Cleared        int       `json:"cleared"`
    DurationMs     int       `json:"duration_ms"`
    Degraded       bool      `json:"degraded"`
    LastError      string    `json:"last_error,omitempty"`
}
// Status field becomes "degraded" if ANY alert_worker_state row has degraded=true OR
// (last_run_at older than 2× expected cadence)
```

### References

- [CITED: riverqueue.com/docs/periodic-jobs] — PeriodicInterval, cron.ParseStandard, CRON_TZ prefix, leader-election semantics
- [CITED: riverqueue.com/docs/job-retries] — JobArgsWithInsertOpts pattern for MaxAttempts; default `attempts ^ 4 + rand(±10%)` backoff
- [CITED: riverqueue.com/docs/subscriptions] — EventKindJobFailed + job.State="discarded" distinguishes "will retry" vs "exhausted"

---

## Decision D: Offline Evaluator + Gateway Suppression (resolves D-14, D-15)

### ALERT-02 Offline Detection (D-15)

```sql
-- internal/alert/queries.sql (sqlc-annotated)
-- name: ListOfflineDevices :many
-- Devices whose last uplink is older than 3× their profile's expected_interval_s.
-- Returns rule-eligible candidates; the worker filters out devices behind a
-- downed gateway per D-14.
SELECT
    d.id                   AS device_id,
    d.label                AS device_label,
    d.last_uplink_at       AS last_uplink_at,
    dp.expected_interval_s AS expected_interval_s,
    d.gateway_id           AS gateway_id,
    g.last_seen_at         AS gateway_last_seen_at,
    EXTRACT(EPOCH FROM (now() - d.last_uplink_at))::INTEGER AS seconds_since_last_uplink
FROM device d
JOIN device_profile dp ON d.profile_id = dp.id
LEFT JOIN gateway g    ON d.gateway_id = g.id
WHERE d.disabled_at IS NULL
  AND d.last_uplink_at IS NOT NULL
  AND now() - d.last_uplink_at > make_interval(secs := 3 * dp.expected_interval_s);

-- name: ListHysteresisClearDevices :many
-- Devices currently alerting that are within the hysteresis-clear band
-- (last_uplink within 2× expected_interval_s).
SELECT a.id AS alert_id, d.id AS device_id
FROM alert a
JOIN device d ON a.target_entity_id = d.id
JOIN device_profile dp ON d.profile_id = dp.id
WHERE a.state = 'firing'
  AND a.rule_kind = 'offline_device'
  AND now() - d.last_uplink_at < make_interval(secs := 2 * dp.expected_interval_s);
```

**Why two thresholds (3× to fire, 2× to clear):** hysteresis. A device whose last_uplink is exactly at `2.5 × expected_interval_s` is in the "uncertain" band — it has not yet fired (needs 3×) and it has not yet cleared (needs <2×). This prevents flap when a slow device's uplinks straddle the boundary.

### D-14 Gateway Suppression — One Query, Eval-Time

```sql
-- name: ListOfflineDevicesWithGatewayStatus :many
-- Single query that returns offline-eligible devices AND a flag for
-- "this device's gateway is itself offline" (suppression candidate).
SELECT
    d.id AS device_id,
    d.label AS device_label,
    d.last_uplink_at,
    dp.expected_interval_s,
    g.id AS gateway_id,
    g.label AS gateway_label,
    g.last_seen_at AS gateway_last_seen_at,
    (
        g.id IS NOT NULL
        AND g.last_seen_at IS NOT NULL
        AND now() - g.last_seen_at > make_interval(secs := 3 * dp.expected_interval_s)
    ) AS gateway_offline
FROM device d
JOIN device_profile dp ON d.profile_id = dp.id
LEFT JOIN gateway g    ON d.gateway_id = g.id
WHERE d.disabled_at IS NULL
  AND d.last_uplink_at IS NOT NULL
  AND now() - d.last_uplink_at > make_interval(secs := 3 * dp.expected_interval_s);
```

Worker logic:

```go
// internal/alert/offline_worker.go
func (w *OfflineWorker) Work(ctx context.Context, _ *river.Job[OfflineArgs]) error {
    candidates, err := w.Queries.ListOfflineDevicesWithGatewayStatus(ctx)
    if err != nil { return err }

    var suppressedGateways = make(map[string]int) // gateway_id → count of suppressed devices

    for _, c := range candidates {
        if c.GatewayOffline {
            // Suppress device alert; tally for the gateway-down alert
            suppressedGateways[c.GatewayID.String()]++
            continue
        }
        // Fire device-offline alert (idempotent: skip if firing alert exists)
        if err := w.fireDeviceOffline(ctx, c); err != nil { return err }
    }

    // Fire one gateway-offline alert per downed gateway
    for gwID, suppressedN := range suppressedGateways {
        if err := w.fireGatewayOffline(ctx, gwID, suppressedN); err != nil { return err }
    }

    // Hysteresis clear
    return w.clearDevicesInHysteresisBand(ctx)
}
```

**Why one query:** stateless suppression. The decision "is this device's gateway down" is evaluated at THIS eval cycle, against THIS query's snapshot. Survives device-gateway remapping mid-outage (the next cycle sees the new mapping).

### Comparison with Phase 4 D-07

Phase 4 D-07 KPI flicker threshold: `2 × expected_interval_s` → device shows as "offline" on dashboard.

Phase 6 ALERT-02 paging threshold: `3 × expected_interval_s` → paging alert fires.

**Both are intentional and documented.** Dashboard wants tight flicker (catch borderline issues visually); alert engine wants strict no-false-positives (an alert means action required).

---

## Decision E: Statistical Anomaly Rules (resolves D-16, D-17)

### Cold-Start Gate (D-16)

```sql
-- name: IsMPEligibleForAnomaly :one
-- True iff the metering point has ≥21 days of measurement history.
SELECT EXISTS(
    SELECT 1 FROM measurement
    WHERE metering_point_id = $1
      AND time < now() - INTERVAL '21 days'
) AS eligible;

-- name: ListAnomalyWarmupRoster :many
-- Returns days_until_eligible for non-eligible MPs (for the Settings → Alerts
-- warmup roster and the per-MP card chip).
SELECT
    mp.id AS metering_point_id,
    mp.label AS metering_point_label,
    CASE
        WHEN MIN(m.time) IS NULL THEN 21
        ELSE GREATEST(0, 21 - EXTRACT(DAY FROM (now() - MIN(m.time)))::INT)
    END AS days_until_eligible
FROM metering_point mp
LEFT JOIN measurement m ON mp.id = m.metering_point_id
WHERE mp.disabled_at IS NULL
GROUP BY mp.id, mp.label
ORDER BY days_until_eligible ASC;
```

### Rule 1: `anomaly_p95` — Trailing-30d P95 per MP × time-of-day bucket

```sql
-- name: P95BaselineForMP :one
-- Compute the trailing-30-day P95 of instant_value for this MP at this hour-of-day.
-- The "this hour-of-day" filter is what makes the rule sensitive to time-of-day
-- patterns (a residential meter's noon usage shouldn't be flagged because
-- midnight is quieter).
WITH bucket_hour AS (
    SELECT EXTRACT(HOUR FROM time)::INT AS h
    FROM measurement
    WHERE metering_point_id = $1
      AND time >= now() - INTERVAL '30 days'
      AND time < now()
)
SELECT percentile_cont(0.95) WITHIN GROUP (ORDER BY instant_value) AS p95
FROM measurement
WHERE metering_point_id = $1
  AND time >= now() - INTERVAL '30 days'
  AND time < now()
  AND EXTRACT(HOUR FROM time)::INT = EXTRACT(HOUR FROM now())::INT;

-- name: LatestInstantValueForMP :one
SELECT instant_value, time
FROM measurement
WHERE metering_point_id = $1
ORDER BY time DESC LIMIT 1;
```

**Performance estimate:** `percentile_cont` on a 30-day × ~hour-bucket window for one MP is **a few hundred rows at typical 15-min cadence**: 30 days × 24 hours × 1 hour-bucket × 4 reads/hour ≈ 2,880 rows. At 500 MPs, scanning 500 × 2,880 = 1.44M rows once per hour is **trivially fast** on a properly-indexed hypertable.

**No CAGG needed for v1.** A `(metering_point_id, time DESC)` index on `measurement` (which exists per Phase 2's hypertable migration) covers it. If Phase 7 finds this hot, the planner adds a `measurement_hourly_p95` CAGG. [ASSUMED — verify indexed scan plan in Wave 0]

### Rule 2: `anomaly_iqr` — Q1/Q3/IQR outliers

```sql
-- name: IQRBaselineForMP :one
SELECT
    percentile_cont(0.25) WITHIN GROUP (ORDER BY instant_value) AS q1,
    percentile_cont(0.75) WITHIN GROUP (ORDER BY instant_value) AS q3
FROM measurement
WHERE metering_point_id = $1
  AND time >= now() - INTERVAL '30 days'
  AND time < now();
-- Worker computes: iqr = q3 - q1; lower = q1 - 1.5*iqr; upper = q3 + 1.5*iqr.
-- Then: if latest instant_value > upper OR < lower → fire.
```

Same performance profile as P95.

### Rule 3: `anomaly_quiet_hour` — Non-zero flow during operator-configured quiet window

**Quiet-window storage decision:** D-17 says "opt-in per-MP". The cleanest schema is a new column on `alert_rule` (when `rule_kind='anomaly_quiet_hour'`):

```sql
-- in 0038_alert_rule.up.sql — extended for quiet-hour rules
quiet_window_start  TIME, -- nullable; only set for rule_kind='anomaly_quiet_hour'
quiet_window_end    TIME, -- nullable; window may cross midnight (e.g. 22:00–06:00)
flow_threshold      DOUBLE PRECISION DEFAULT 0.0, -- minimum flow that counts as "non-zero" (allows for sensor noise)
```

The window is **per-rule**, not per-MP — multiple MPs can share a rule (rule scope_kind='site'), but the rule has one window. If different MPs need different windows, the operator creates one rule per MP. Simpler than a separate `quiet_hour_config` table.

```sql
-- name: NonZeroFlowDuringQuietWindow :one
-- Returns the latest non-zero-flow measurement in the quiet window for THIS MP.
SELECT instant_value, time
FROM measurement
WHERE metering_point_id = $1
  AND time >= now() - INTERVAL '24 hours'
  AND instant_value > $2 -- flow_threshold (allow for noise)
  AND (
      -- Same-day quiet window
      ($3::TIME < $4::TIME
       AND (time AT TIME ZONE $5)::TIME BETWEEN $3::TIME AND $4::TIME)
      OR
      -- Cross-midnight quiet window
      ($3::TIME >= $4::TIME
       AND ((time AT TIME ZONE $5)::TIME >= $3::TIME
            OR (time AT TIME ZONE $5)::TIME <= $4::TIME))
  )
ORDER BY time DESC LIMIT 1;
-- $1 = mp_id, $2 = flow_threshold, $3 = quiet_window_start,
-- $4 = quiet_window_end, $5 = install_tz (e.g. 'Asia/Bangkok')
```

The `time AT TIME ZONE $5` cast applies the operator's install timezone so "22:00–06:00" is local time, not UTC.

### Forward-compat for Phase 7

Phase 7 tunes thresholds (multiplier on IQR, P95 percentile, flow_threshold sensitivity). The Phase 6 schema is forward-compatible:
- All three rules use `alert_rule.high_bound` for the literal threshold (when applicable)
- Phase 7 can add `multiplier DOUBLE PRECISION DEFAULT 1.5` to `alert_rule` without breaking v1 rules
- The cold-start gate (21 days) is hardcoded in Phase 6's worker; Phase 7 can promote it to a config row

### References

- [ASSUMED] — Performance estimate of `percentile_cont` on 30d × hourly window per MP. Wave 0 task can run an EXPLAIN against the existing measurement hypertable for one MP to confirm.
- [CITED: postgresql.org/docs/current/functions-aggregate.html] — `percentile_cont` semantics
- [CITED: FEATURES.md L134] — AMI industry P95 trailing-30d baseline pattern
- [ASSUMED] — Quiet-window storage as nullable columns on alert_rule (vs separate quiet_hour_config table); decision is reversible

---

## Decision F: Audit Browse — Cursor Pagination + Filter Chips (resolves D-31..D-38)

### Cursor Pagination Pattern (D-36)

The existing index in migration 0016 (`audit_log_time_idx ON audit_log (time DESC)`) is sufficient. Cursor key is `(time DESC, id)` to break ties when multiple rows share a microsecond timestamp.

```sql
-- name: ListAuditRowsCursor :many
-- Cursor pagination: pass $1=before_time, $2=before_id (or NULL on first page).
-- Filters are nullable for "no filter on this chip" behavior.
SELECT
    a.id, a.time, a.action, a.entity_type, a.entity_id,
    a.before, a.after, a.notes, a.request_id,
    a.user_id, u.email AS user_email
FROM audit_log a
LEFT JOIN "user" u ON a.user_id = u.id
WHERE ($1::TIMESTAMPTZ IS NULL OR a.time < $1)
   OR (a.time = $1 AND a.id < $2::UUID)  -- tie-break
  AND ($3::TIMESTAMPTZ IS NULL OR a.time >= $3) -- from
  AND ($4::TIMESTAMPTZ IS NULL OR a.time <  $4) -- to
  AND ($5::UUID IS NULL OR a.user_id = $5)      -- user_id
  AND ($6::TEXT IS NULL OR a.entity_type = $6)  -- entity_type
  AND ($7::TEXT IS NULL OR a.action = $7)       -- action
  AND ($8::TEXT IS NULL OR a.request_id ILIKE '%' || $8 || '%') -- request_id LIKE
ORDER BY a.time DESC, a.id DESC
LIMIT 100;
```

**Wait — there's a SQL bug in that WHERE clause.** The OR/AND precedence in cursor + filters needs grouping. The correct form:

```sql
SELECT … FROM audit_log a LEFT JOIN "user" u ON a.user_id = u.id
WHERE (
    $1::TIMESTAMPTZ IS NULL
    OR (a.time, a.id) < ($1::TIMESTAMPTZ, $2::UUID) -- row-comparison: native cursor
)
AND ($3::TIMESTAMPTZ IS NULL OR a.time >= $3)
AND ($4::TIMESTAMPTZ IS NULL OR a.time <  $4)
AND ($5::UUID IS NULL OR a.user_id = $5)
AND ($6::TEXT IS NULL OR a.entity_type = $6)
AND ($7::TEXT IS NULL OR a.action = $7)
AND ($8::TEXT IS NULL OR a.request_id ILIKE '%' || $8 || '%')
ORDER BY a.time DESC, a.id DESC
LIMIT 100;
```

Postgres row-comparison `(time, id) < (cursor_time, cursor_id)` does exactly what cursor pagination needs (lexicographic DESC). [CITED: postgresql.org/docs/current/sql-expressions.html#SQL-SYNTAX-ROW-CONSTRUCTORS] — row-constructor comparison is well-defined.

### Filter Chip URL State (D-32)

```typescript
// web/src/routes/audit/AuditFilters.tsx
import { z } from "zod";
import { useSearchParams } from "react-router-dom";

export const auditFiltersSchema = z.object({
  from:        z.string().datetime().optional(),
  to:          z.string().datetime().optional(),
  user_id:     z.string().uuid().optional(),
  entity_type: z.enum([
    "site","metering_point","device","device_profile","binding","gateway",
    "import_job","report","floor_plan","placement","retention_config",
    "alert_rule","alert","backup_run","user","session", // Phase 6 additions
  ]).optional(),
  action:      z.string().optional(),
  request_id:  z.string().optional(),
});

export type AuditFilters = z.infer<typeof auditFiltersSchema>;

export function useAuditFilters(): [AuditFilters, (f: AuditFilters) => void] {
  const [params, setParams] = useSearchParams();
  const parsed = auditFiltersSchema.parse(
    Object.fromEntries([...params.entries()])
  );
  // … set helper that serializes nullables to absent params
}
```

### Default View (D-33)

Cold-arrival to `/audit` applies an implicit `from = now() - 7d`, `to = now()` (rendered as an active filter chip). Operator can clear to see all-time.

### CSV Export with Streaming + 50k cap (D-35)

```go
// internal/audit/export.go
const csvExportRowCap = 50_000

func StreamCSVExport(ctx context.Context, w http.ResponseWriter, pool *pgxpool.Pool, filter Filter) error {
    // BOM (REPT-03 spec)
    w.Header().Set("Content-Type", "text/csv; charset=utf-8")
    w.Header().Set("Content-Disposition", `attachment; filename="audit-export-`+time.Now().Format("20060102-150405")+`.csv"`)
    w.Write([]byte("\xEF\xBB\xBF"))

    cw := csv.NewWriter(w)
    cw.Write([]string{
        "time","user_email","user_id","action","entity_type","entity_id",
        "request_id","notes","before_json","after_json",
    })

    // Count first; if > cap, surface a JSON error WITHOUT writing CSV bytes
    // (frontend offers "Generate full export" → River job per D-35).
    count, err := countAuditRows(ctx, pool, filter)
    if err != nil { return err }
    if count > csvExportRowCap {
        w.WriteHeader(http.StatusPayloadTooLarge)
        // … emit JSON-shaped error suggesting background job
        return nil
    }

    // pgx streaming via Rows (no full materialization)
    rows, err := pool.Query(ctx, exportQuery, filter.Args()...)
    if err != nil { return err }
    defer rows.Close()

    for rows.Next() {
        var t time.Time
        var userEmail, action, entityType, requestID, notes string
        var userID, entityID pgtype.UUID
        var before, after pgtype.JSONB
        if err := rows.Scan(&t, &userEmail, &userID, &action, &entityType, &entityID,
            &requestID, &notes, &before, &after); err != nil {
            return err
        }
        cw.Write([]string{
            t.In(installTZ).Format(time.RFC3339),
            userEmail,
            uuidToString(userID),
            action, entityType, uuidToString(entityID),
            requestID, notes,
            string(before.Bytes), string(after.Bytes),
        })
    }
    cw.Flush()
    return rows.Err()
}
```

For >50k, frontend POSTs `/api/audit/export-async` and the River `AuditExportWorker` writes to `/var/lib/shifter/reports/<uuid>/audit-export.csv` and emails the operator (no SMTP in v1 — instead, surfaces in a "Background jobs" section in the page with a download link, 24h TTL, mirroring REPT-06 PDF behavior).

### Audit Retention Prune (D-38)

```sql
-- name: PruneAuditRows :execrows
-- Runs daily at 03:00 install_tz via River cron (see § River setup).
-- audit_log is NOT a hypertable, so we DELETE directly. retention_config.audit_log_days
-- gates the cutoff. 5y default = 1825 days.
DELETE FROM audit_log
WHERE time < now() - make_interval(days := $1);
```

```go
// internal/alert/audit_prune_worker.go (or move to internal/audit/ — planner picks)
type AuditPruneArgs struct{}
func (AuditPruneArgs) Kind() string { return "audit_prune" }
func (AuditPruneArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{MaxAttempts: 3} }

func (w *AuditPruneWorker) Work(ctx context.Context, _ *river.Job[AuditPruneArgs]) error {
    cfg, err := w.Queries.GetRetentionConfig(ctx)
    if err != nil { return err }
    deleted, err := w.Queries.PruneAuditRows(ctx, cfg.AuditLogDays)
    if err != nil { return err }
    w.Log.Info("audit_prune.cycle", "deleted", deleted, "cutoff_days", cfg.AuditLogDays)
    return nil
}
```

### References

- [CITED: postgresql.org/docs/current/sql-expressions.html#SQL-SYNTAX-ROW-CONSTRUCTORS] — row-comparison cursor pagination
- [CITED: phase 4 D-15 — uplinks log cursor pagination existing code in `internal/dashboard/uplinks.go`] — reuse the pattern verbatim
- [CITED: encoding/csv stdlib + REPT-03 spec from Phase 5] — BOM + ISO-8601 + UTF-8

---

## Decision G: User Management & Session Revoke (resolves D-23..D-30)

### Bulk Session Revoke — Already Shipping (D-24)

The pattern is **already shipping** in `internal/auth/account.go::iterateAndRevoke`:

```go
func iterateAndRevoke(ctx context.Context, sm *scs.SessionManager, store *Store, userID, keepToken string) error {
    var toDelete []string
    err := sm.Iterate(ctx, func(ictx context.Context) error {
        tok := sm.Token(ictx)
        if tok == keepToken {
            return nil
        }
        if uid := sm.GetString(ictx, sessionUserIDKey); uid == userID {
            toDelete = append(toDelete, tok)
        }
        return nil
    })
    if err != nil { return err }
    for _, tok := range toDelete {
        if _, err := store.Pool().Exec(ctx, `DELETE FROM sessions WHERE token = $1`, tok); err != nil {
            return err
        }
    }
    return nil
}
```

**Phase 6 reuse:** call `iterateAndRevoke(ctx, sm, store, targetUserID, "")` (empty keepToken → ALL sessions including current). The four D-24 callers all use the same function:

```go
// internal/user/handler.go
func LogoutEverywhere(deps Deps) http.HandlerFunc { /* call iterateAndRevoke */ }
func DisableUser(deps Deps)       http.HandlerFunc { /* update + iterateAndRevoke */ }
func ChangeRole(deps Deps)        http.HandlerFunc { /* update + iterateAndRevoke */ }
func AdminResetPassword(deps Deps) http.HandlerFunc { /* update + iterateAndRevoke */ }
```

**Caveat for D-26:** these handlers must reject self-action server-side BEFORE calling iterateAndRevoke. Server guards:

```go
// internal/user/guards.go
var (
    ErrSelfAction    = errors.New("cannot perform this action on yourself")
    ErrLastAdmin     = errors.New("cannot leave zero admins")
)

func RejectSelfAction(actingUserID, targetUserID string) error {
    if actingUserID == targetUserID { return ErrSelfAction }
    return nil
}

func RejectLastAdminDemote(ctx context.Context, pool *pgxpool.Pool, targetUserID, newRole string) error {
    if newRole == "admin" { return nil } // only demote-to-non-admin is the risk
    var adminCount int
    err := pool.QueryRow(ctx, `
        SELECT count(*) FROM "user"
        WHERE role = 'admin' AND disabled_at IS NULL AND id <> $1
    `, targetUserID).Scan(&adminCount)
    if err != nil { return err }
    if adminCount == 0 { return ErrLastAdmin }
    return nil
}
```

### Atomic Auth-Event Audit Retrofit (D-30)

**Retrofit pattern:** wrap the existing handlers in Plan 01-09 (`internal/auth/handlers.go`) and Plan 01-11 (`internal/auth/account.go`) without breaking their tests. The trick is to write the audit row in the SAME transaction as the auth state change.

The login handler currently DOES NOT use a transaction (it just calls `store.GetUserByEmail` + session.PutUser). Phase 6 must restructure to:

```go
// internal/auth/handlers.go — POST /api/auth/login (after retrofit)
func LoginHandler(deps Deps) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // … parse body, rate-limit, etc.
        tx, err := deps.Store.Pool().BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
        if err != nil { /* 500 */ return }
        defer tx.Rollback(r.Context())

        u, err := deps.Store.GetUserByEmailTx(r.Context(), tx, req.Email)
        if err != nil {
            // Audit the failed login attempt WITHOUT user_id (failures are auditable)
            _ = audit.WriteEntry(r.Context(), tx, audit.Entry{
                Action:     "auth.login_failed",
                EntityType: "user",
                EntityID:   uuid.Nil, // failed login has no user_id
                Notes:      stringPtr("email=" + req.Email),
                RequestID:  middleware.GetReqID(r.Context()),
            })
            _ = tx.Commit(r.Context())
            // … return 401
            return
        }
        if !auth.Verify(u.PasswordHash, req.Password) { /* same audit pattern */ }

        // Success path
        _ = audit.WriteEntry(r.Context(), tx, audit.Entry{
            UserID:     &u.ID,
            Action:     "auth.login_success",
            EntityType: "user",
            EntityID:   uuidParse(u.ID),
            RequestID:  middleware.GetReqID(r.Context()),
        })
        if err := tx.Commit(r.Context()); err != nil { /* 500 */ return }

        // Session work happens AFTER commit (session is separate to the user DB tx)
        deps.SessionMgr.PutUser(r.Context(), u.toSessionUser())
        // … 200 response
    }
}
```

**Critical:** the existing tests in `internal/auth/handlers_test.go` need a fresh look — they likely don't assert audit_log row count. The retrofit adds the assertion AND any "the test mocks/fakes the pool" needs to be updated to a real testcontainer (which Phase 1 already uses).

### Random Password Generator (D-23)

```go
// internal/user/password.go
import "crypto/rand"

// D-23 specifies "≥12 chars from full ASCII"; CLAUDE's-Discretion recommends ≥16
// and excludes ambiguous chars (1lI0O). The strength evaluator (Phase 1) must
// score this as STRONG; we generate and re-roll if it doesn't pass.
const passwordAlphabet = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789!@#$%^&*"
const passwordLen = 16

func GenerateRandomPassword() (string, error) {
    for tries := 0; tries < 10; tries++ {
        b := make([]byte, passwordLen)
        if _, err := rand.Read(b); err != nil { return "", err }
        out := make([]byte, passwordLen)
        for i := range b {
            out[i] = passwordAlphabet[int(b[i])%len(passwordAlphabet)]
        }
        candidate := string(out)
        if password.StrengthScore(candidate) >= password.MinScore {
            return candidate, nil
        }
    }
    return "", errors.New("generate: failed to produce strong password after 10 tries")
}
```

**Show-once panel:** the password is returned ONLY in the response body of `POST /api/users` (admin add-user). Once the response leaves the server, it is not stored in plaintext anywhere — `must_change_password = true` so the user rotates on first login.

### Frontend: Show-Once Panel

```tsx
// web/src/routes/settings/AddUserDialog.tsx
const onCreate = async (data: AddUserForm) => {
    const res = await apiPost("/api/users", data);
    if (res.ok) {
        // Flip dialog state from "form" to "credentials-shown"
        setCredentials({ email: data.email, password: res.data.initial_password });
    }
};

return (
    <ResponsiveDialog open={open} onOpenChange={onClose}>
        {credentials ? (
            <CredentialsPanel
                email={credentials.email}
                password={credentials.password}
                onConfirm={() => { setCredentials(null); onClose(); }}
            />
        ) : (
            <AddUserForm onSubmit={onCreate} />
        )}
    </ResponsiveDialog>
);
```

The `CredentialsPanel` has a Copy button (uses `navigator.clipboard.writeText`) + an "I've shared this" confirmation that closes the dialog. The password is in React state only; no localStorage, no Redux. Closing the dialog clears it.

### References

- [VERIFIED: existing code at `internal/auth/account.go::iterateAndRevoke`] — reused verbatim
- [VERIFIED: `internal/auth/password_strength.go`] — reused verbatim per D-28
- [CITED: github.com/alexedwards/scs/v2 + pgxstore — sessions table schema] — `token TEXT PRIMARY KEY, data BYTEA, expiry TIMESTAMP`; data is gob-encoded, so user_id lookup requires Iterate
- [CITED: OWASP password strength scoring (Phase 1 evaluator)] — same min-score across all surfaces

---

## Decision H: `shifter doctor` CLI (resolves D-50)

### Bundle Shape (text or file)

```
shifter doctor              # prints to stdout
shifter doctor --out diagnostic.json  # writes to a file
shifter doctor --no-logs    # skip the Docker-socket log tail
```

Output JSON shape:

```jsonc
{
  "generated_at": "2026-05-13T14:22:00Z",
  "shifter": {
    "version": "0.6.0",
    "schema_version": "0042",
    "uptime_seconds": 1234567
  },
  "config_check": { /* output of internal/cli/configcheck.go */ },
  "health_detailed": { /* same as GET /health/detailed */ },
  "alert_workers": [
    { "kind": "threshold_instantaneous", "last_run_at": "…", "rules_evaluated": 12, "fires_emitted": 0, "degraded": false }
  ],
  "last_backup": { "started_at": "…", "finished_at": "…", "sha256": "abc…", "size_bytes": 1234567 },
  "chirpstack_grpc_ping": { "ok": true, "latency_ms": 14 },
  "recent_audit": [
    {"time":"…", "action":"auth.login_success", "user_email":"j***@example.com", "entity_type":"user", "entity_id":"…"}
  ],
  "logs": [
    "2026-05-13T14:21:50Z INFO …"
  ]
}
```

### PII Redaction (D-50)

```go
// internal/doctor/redact.go
// MaskEmail("john.doe@example.com") → "j***@example.com"
// MaskEmail("jd@example.com")       → "j***@example.com"
// MaskEmail("a@example.com")        → "a***@example.com"
// MaskEmail("not-an-email")         → "[redacted]"
func MaskEmail(email string) string {
    at := strings.Index(email, "@")
    if at <= 0 || at == len(email)-1 { return "[redacted]" }
    return string(email[0]) + "***" + email[at:]
}
```

Apply to ALL audit rows' `user_email` and `notes` (which may contain emails). The `before` / `after` JSONB blobs are tricky — a regex pass on the serialized JSON suffices for email shaped tokens:

```go
var emailRegex = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)

func RedactJSON(b []byte) []byte {
    return emailRegex.ReplaceAllFunc(b, func(match []byte) []byte {
        return []byte(MaskEmail(string(match)))
    })
}
```

### Docker Socket Log Tail (with Fallback)

```go
// internal/doctor/doctor.go
func (d *Doctor) TailLogs(ctx context.Context, lines int) ([]string, error) {
    // Check for Docker socket
    if _, err := os.Stat("/var/run/docker.sock"); err != nil {
        return []string{"<docker socket not mounted — run `docker compose logs --tail=200 shifter` from host>"}, nil
    }

    // Connect via Docker SDK (github.com/docker/docker/client)
    cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
    if err != nil { return nil, err }

    // Find own container ID via /proc/self/cgroup (Docker pattern)
    containerID, err := selfContainerID()
    if err != nil { return nil, err }

    rc, err := cli.ContainerLogs(ctx, containerID, container.LogsOptions{
        ShowStdout: true, ShowStderr: true, Tail: strconv.Itoa(lines),
    })
    if err != nil { return nil, err }
    defer rc.Close()

    body, err := io.ReadAll(rc)
    if err != nil { return nil, err }
    return strings.Split(string(body), "\n"), nil
}
```

**Operational note:** the Docker socket is **NOT mounted into the Shifter container by default** (security posture). The `doctor` CLI gracefully degrades to "run `docker compose logs --tail=200 shifter` from host". Operators who want one-shot bundles can opt-in by mounting `/var/run/docker.sock:/var/run/docker.sock:ro` in their compose override.

### References

- [VERIFIED: existing `internal/cli/configcheck.go` + `internal/cli/healthcheck.go`] — reuse outputs
- [CITED: docker.com/sdk/go] — `ContainerLogs` API
- [ASSUMED] — self-container-ID via `/proc/self/cgroup` reading; pattern is standard but planner verifies on Alpine cgroups v2

---

## Decision I: Cron Sidecar for Bundled-Mode Backup (resolves D-43c)

### Service Block for `compose/bundled.yml`

```yaml
# Add at the bottom of services:
  backup-cron:
    image: mcuadros/ofelia:v0.3.22
    restart: unless-stopped
    depends_on:
      shifter:
        condition: service_started
    command: ["daemon", "--docker"]
    labels:
      ofelia.job-exec.shifter-backup.schedule: "0 2 * * *"      # 02:00 daily
      ofelia.job-exec.shifter-backup.container: "shifter"
      ofelia.job-exec.shifter-backup.command: "shifter backup --to /var/lib/shifter/backups"
      ofelia.job-exec.shifter-backup.user: "shifter"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro            # Docker API access
    logging: *json-logging
    networks: [shifter]
```

[CITED: github.com/mcuadros/ofelia] — `--docker` flag auto-discovers `ofelia.job-exec.<name>.*` labels on running containers OR uses self-labels. The label-driven config keeps everything in one file.

### Why NOT cron in the shifter container itself

- Adding crond inside the Go binary container introduces a process supervisor (PID 1 dilemma)
- Cron-from-container makes `shifter backup` a subprocess invocation with weird env-injection — the operator would need to debug "why does cron not see SHIFTER_BACKUP_DIR"
- Ofelia is single-binary Go ⇒ minimal image ⇒ best-of-breed for this exact use case

### External Mode

`compose/external.yml` does NOT include the cron sidecar by default — external-mode operators are presumed to have their own cron infrastructure (system cron, k8s CronJob, etc.). The operator runbook documents how to add an Ofelia sidecar identical to the bundled one if desired.

### Failure Handling

The `shifter backup` CLI ALWAYS exits non-zero on failure. Ofelia's `job-exec` reports the exit code via Docker logs. The Shifter binary additionally writes a `backup_run` row with `status='failed'` and `error_message` populated, which surfaces on the Settings → Backup card with a red age dot.

### References

- [CITED: github.com/mcuadros/ofelia] — v0.3.22 Apr 2026; --docker mode; label-driven jobs

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Backup helper for TimescaleDB | A custom dump script that walks `_timescaledb_catalog` | `pg_dump --format=custom` + `timescaledb_pre_restore()`/`_post_restore()` | TimescaleDB's own pre/post hooks handle CAGGs, retention, refresh policies. Custom code = "future maintenance landmine." Upstream's archived `timescaledb-backup` tool warns precisely against this. |
| Cron + container scheduling | crond in Shifter image + entrypoint dance | `mcuadros/ofelia` sidecar (label-driven) | Ofelia is a single-binary Go scheduler designed for this. Crond-in-container is PID-1-hell. |
| Cursor pagination | Manual offset/limit | Postgres row-comparison `(time, id) < (cursor_time, cursor_id)` | Row-comparison is stable under concurrent inserts; offset/limit can skip or duplicate. |
| Session revoke by user_id | New session-store schema | Existing `scs.Iterate` + `DELETE FROM sessions WHERE token = $1` pattern (already in `account.go`) | Already shipping. Migration would be a regression. |
| Random password generator | Custom alphabet scramble | `crypto/rand` + alphabet + re-roll if strength score fails | Existing `internal/auth/password_strength.go` is the gate (D-28). |
| Email-shaped PII detection in `doctor` | Custom parser walking JSON tree | `regexp.MustCompile([email pattern])` over serialized bytes | Email regex is well-understood; JSON walk for "any string that contains email" is overkill. |
| Backup tarball | Third-party `mholt/archiver` | stdlib `archive/tar` + `compress/gzip` | Stdlib is plenty for this use case. |
| Manifest JSON validation on restore | Custom struct unmarshal + manual checks | `encoding/json` Decode into typed struct + sha256 verification of file list | Stdlib + sha256 hash check. |
| Audit retention prune | Phase 5 retention reconciliation (TimescaleDB policy) | River cron job calling `DELETE FROM audit_log WHERE time < now() - $1` | `audit_log` is a regular table (Phase 2 D-06), NOT a hypertable. TimescaleDB retention does not apply. |
| Detect job-failed-after-retry | Polling River job state from a goroutine | `riverClient.Subscribe(river.EventKindJobFailed)` + `job.State == JobStateDiscarded` | Idiomatic River; verified pattern. |

**Key insight:** every Phase 6 surface either reuses an existing pattern (audit-in-tx, iterateAndRevoke, retention reconciliation, JsonTree component, ResponsiveDialog) OR composes simple stdlib primitives (archive/tar + crypto/sha256 + encoding/csv). The only genuinely new architectural pieces are the alert engine state machine and the cron sidecar — and even those build on existing River infrastructure.

---

## Common Pitfalls

### Pitfall 1: pg_restore --jobs=N corrupts TimescaleDB catalogs

[CITED: docs.tigerdata.com/self-hosted/latest/backup-and-restore/logical-backup] *"Do not use `pg_restore` with the `-j` option. This option does not correctly restore the TimescaleDB catalogs."*

**Mitigation:** Phase 6 `internal/backup/restore.go` MUST NOT use `--jobs` / `-j`. Lint via grep test in CI:
```bash
! grep -E "pg_restore.*[-]j" internal/backup/*.go
```

### Pitfall 2: timescaledb_pre_restore() forgotten → restore appears to succeed but CAGGs are broken

If pg_restore runs without `timescaledb_pre_restore()` having been called, the background workers stay active during the load and racefully process the half-restored data. The restore "succeeds" (exit 0) but CAGGs are corrupt.

**Mitigation:** ALWAYS wrap pg_restore in the pre/post pair. The `internal/backup/restore.go` orchestrator does this; CI round-trip tests verify CAGG row counts post-restore.

### Pitfall 3: SCS session DELETE by user_id needs Iterate (data is gob, not JSONB)

The pgxstore schema is `(token TEXT, data BYTEA, expiry TIMESTAMP)`. The data column is gob-encoded — you CANNOT `DELETE FROM sessions WHERE data ->> 'user_id' = $1`.

**Mitigation:** use the existing `iterateAndRevoke` pattern from `internal/auth/account.go`. CONTEXT.md `<canonical_refs>` SCS section had a slight inaccuracy ("via JSONB" — incorrect). Already-shipping code is the truth.

### Pitfall 4: Audit-event retrofit breaks existing handler tests

Phase 1's `internal/auth/handlers_test.go` does NOT expect audit_log inserts. Retrofitting login/logout/change-password with audit writes will:
- Need a real pool (testcontainer) instead of any mocks
- Need additional `SELECT count(*) FROM audit_log WHERE action = 'auth.login_success'` assertions
- Need the pool to be the SAME pool the audit writer uses (no separate "auth pool" vs "audit pool")

**Mitigation:** Wave 0 test-prep includes upgrading these tests to write audit assertions, then implement the retrofit, then verify tests pass.

### Pitfall 5: Self-action guards forgotten on server side

D-26 says UI greys out self-action buttons. **The server MUST also enforce.** If an admin curl's `PATCH /api/users/<their-own-id>/disable`, the server should 422.

**Mitigation:** `internal/user/guards.go` runs FIRST in every handler before any state mutation. Test: 422 on self-targeted disable / role-change.

### Pitfall 6: Last-admin check has a TOCTOU race

Two admins simultaneously demote each other → both checks see "1 admin remaining" → both DELETE succeed → zero admins.

**Mitigation:** wrap the demote in a serializable txn with the `RejectLastAdminDemote` check INSIDE the tx; or use a SELECT FOR UPDATE on the user table. Acceptable for v1: serializable txn. Lockout recovery via `shifter create-admin --reset` if it ever happens.

### Pitfall 7: River cron jobs run on every replica

Phase 6 ships single-instance Shifter, but if v2 adds replicas, River's leader election (verified) ensures only one runs cron. **No action needed in v1**, just documented for awareness.

### Pitfall 8: Alert cool-down forgets test alerts (D-19)

A `is_test = true` alert that auto-clears after 60s should NOT update `alert_rule.last_fired_at` — otherwise the next real fire gets suppressed by cool-down.

**Mitigation:** the test-fire code path is separate from the worker's fire path. Worker fires update `last_fired_at`; test fires don't.

### Pitfall 9: Quiet-hour window crossing midnight is the SQL gotcha

The TIME-range query for 22:00–06:00 requires the OR-form (see § Decision E SQL). A naive `BETWEEN 22:00 AND 06:00` always returns empty.

**Mitigation:** the canonical query in § Decision E handles both same-day and cross-midnight cases. Test: a measurement at 03:00 must fire for a 22:00–06:00 window.

### Pitfall 10: Backup runs while Shifter is also writing

Without quiescence, a backup taken during heavy ingest captures a transactionally-consistent snapshot (Postgres MVCC guarantees this) but may MISS data still in MQTT-subscriber buffers OR pending in River queue.

**Mitigation:** v1 documents that "in-flight" data may not be in the backup. The next ingest cycle handles the data, so re-restoring loses at most 1-2 minutes of telemetry — acceptable for a backup window. The runbook calls this out.

---

## Code Examples

### Alert Rule Creation (handler + SQL)

```go
// internal/alert/handler.go — POST /api/alerts/rules
// Body: { rule_kind, scope_kind, scope_id, high_bound?, low_bound?, severity, name?, notes?, cooldown_seconds? }
func CreateRuleHandler(deps Deps) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        user, _ := auth.GetUser(r.Context(), deps.SM)
        var req CreateRuleRequest
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil { /* 400 */ return }
        if err := req.Validate(); err != nil { /* 422 */ return }

        tx, err := deps.Pool.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
        if err != nil { /* 500 */ return }
        defer tx.Rollback(r.Context())

        rule, err := deps.Queries.WithTx(tx).CreateAlertRule(r.Context(), sqlc.CreateAlertRuleParams{
            RuleKind:        req.RuleKind,
            ScopeKind:       req.ScopeKind,
            ScopeID:         req.ScopeID,
            HighBound:       req.HighBound,
            LowBound:        req.LowBound,
            Severity:        req.Severity,
            Name:            req.Name,
            Notes:           req.Notes,
            CooldownSeconds: req.CooldownSeconds,
        })
        if err != nil { /* 500 */ return }

        _ = audit.WriteEntry(r.Context(), tx, audit.Entry{
            UserID:     &user.ID,
            Action:     "alert.rule_create",
            EntityType: "alert_rule",
            EntityID:   rule.ID,
            After:      jsonOf(rule),
            RequestID:  middleware.GetReqID(r.Context()),
        })
        if err := tx.Commit(r.Context()); err != nil { /* 500 */ return }
        writeJSON(w, http.StatusCreated, rule)
    }
}
```

### Audit Browse — Cursor Pagination Handler

```go
// internal/audit/handler.go — GET /api/audit?from=…&to=…&user_id=…&entity_type=…&action=…&request_id=…&cursor=<base64>
func ListHandler(deps Deps) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        filter, err := parseFilter(r.URL.Query())
        if err != nil { /* 422 */ return }
        cursor, _ := decodeCursor(r.URL.Query().Get("cursor")) // nil cursor = first page

        rows, err := deps.Queries.ListAuditRowsCursor(r.Context(), sqlc.ListAuditRowsCursorParams{
            CursorTime: cursor.Time, CursorID: cursor.ID,
            FromTime: filter.From, ToTime: filter.To,
            UserID: filter.UserID, EntityType: filter.EntityType,
            Action: filter.Action, RequestIDLike: filter.RequestIDLike,
        })
        if err != nil { /* 500 */ return }

        nextCursor := ""
        if len(rows) == 100 { // full page → more might exist
            last := rows[len(rows)-1]
            nextCursor = encodeCursor(last.Time, last.ID)
        }
        writeJSON(w, http.StatusOK, map[string]any{
            "rows": rows, "next_cursor": nextCursor,
        })
    }
}
```

### Backup Tarball Construction

```go
// internal/backup/runner.go
func (r *Runner) Backup(ctx context.Context, destPath string) error {
    f, err := os.Create(destPath)
    if err != nil { return err }
    defer f.Close()

    gzw := gzip.NewWriter(f)
    defer gzw.Close()
    tw := tar.NewWriter(gzw)
    defer tw.Close()

    manifest := Manifest{
        ManifestVersion: "1.0",
        ShifterVersion:  version.Info().Version,
        StartedAt:       time.Now().UTC(),
        ChirpStackMode:  r.config.ChirpStackMode,
        SHA256Sums:      map[string]string{},
    }

    // 1. Dump shifter DB
    shifterBak := filepath.Join(os.TempDir(), "shifter.bak")
    cmd := exec.CommandContext(ctx, "pg_dump",
        "--host", r.config.DBHost,
        "--username", r.config.DBUser,
        "--dbname", r.config.DBName,
        "--format=custom", "--no-owner", "--no-acl",
        "--file", shifterBak,
    )
    cmd.Env = append(os.Environ(), "PGPASSWORD="+r.config.DBPassword)
    if err := cmd.Run(); err != nil { return fmt.Errorf("pg_dump shifter: %w", err) }

    // Add to tarball + compute sha256
    if err := addFileToTar(tw, shifterBak, "db/shifter.bak", manifest.SHA256Sums); err != nil { return err }
    manifest.Included = append(manifest.Included, "db/shifter.bak")

    // 2. Dump ChirpStack DB if bundled
    if r.config.ChirpStackMode == "bundled" {
        chirpBak := filepath.Join(os.TempDir(), "chirpstack.bak")
        cmd := exec.CommandContext(ctx, "pg_dump",
            "--host", r.config.DBHost,
            "--username", "chirpstack",
            "--dbname", "chirpstack",
            "--format=custom", "--no-owner", "--no-acl",
            "--file", chirpBak,
        )
        if err := cmd.Run(); err != nil { return fmt.Errorf("pg_dump chirpstack: %w", err) }
        if err := addFileToTar(tw, chirpBak, "db/chirpstack.bak", manifest.SHA256Sums); err != nil { return err }
        manifest.Included = append(manifest.Included, "db/chirpstack.bak")
    }

    // 3. Floor plans (recursive walk)
    if err := addDirToTar(tw, r.config.FloorPlansDir, "floor-plans/", manifest.SHA256Sums); err != nil { return err }
    manifest.Included = append(manifest.Included, "floor-plans/")

    // 4. Manifest as the LAST entry
    manifest.FinishedAt = time.Now().UTC()
    mb, _ := json.MarshalIndent(manifest, "", "  ")
    hdr := &tar.Header{Name: "manifest.json", Size: int64(len(mb)), Mode: 0644}
    if err := tw.WriteHeader(hdr); err != nil { return err }
    if _, err := tw.Write(mb); err != nil { return err }

    return nil
}
```

### Alert Worker — Threshold Subtype (full)

```go
// internal/alert/threshold_worker.go (instantaneous; hourly/daily are near-identical)
type ThresholdInstantaneousArgs struct{}
func (ThresholdInstantaneousArgs) Kind() string { return "alert_threshold_instantaneous" }
func (ThresholdInstantaneousArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{MaxAttempts: 3} }

type ThresholdInstantaneousWorker struct {
    river.WorkerDefaults[ThresholdInstantaneousArgs]
    Pool    *pgxpool.Pool
    Queries *sqlc.Queries
    Hub     *events.Hub
    Log     *slog.Logger
}

func (w *ThresholdInstantaneousWorker) Work(ctx context.Context, _ *river.Job[ThresholdInstantaneousArgs]) error {
    start := time.Now()
    state := newRunState("threshold_instantaneous")

    rules, err := w.Queries.ListActiveThresholdRules(ctx, "threshold_instantaneous")
    if err != nil { return fmt.Errorf("list rules: %w", err) }

    for _, rule := range rules {
        if rule.LastFiredAt.Valid &&
            time.Since(rule.LastFiredAt.Time) < time.Duration(rule.CooldownSeconds)*time.Second {
            continue
        }

        targets, err := w.expandScope(ctx, rule)
        if err != nil { state.recordErr(err); continue }

        for _, t := range targets {
            latest, err := w.Queries.LatestMeasurementForMP(ctx, t.MeteringPointID)
            if err != nil { state.recordErr(err); continue }
            state.rulesEvaluated++

            breached := compareBound(latest.InstantValue, rule.HighBound, rule.LowBound)
            existing, _ := w.Queries.GetFiringAlertForRuleTarget(ctx, rule.ID, t.MeteringPointID)

            if breached && !existing.Valid {
                if err := w.fireAlert(ctx, rule, t, latest); err != nil { state.recordErr(err); continue }
                state.firesEmitted++
            } else if !breached && existing.Valid {
                if err := w.clearAlert(ctx, existing.AlertID); err != nil { state.recordErr(err); continue }
                state.cleared++
            }
        }
    }

    state.durationMs = int(time.Since(start).Milliseconds())
    _ = w.Queries.UpsertAlertWorkerState(ctx, state.toParams())
    return state.firstErr
}

func (w *ThresholdInstantaneousWorker) fireAlert(ctx context.Context, rule sqlc.AlertRule, target Target, latest Measurement) error {
    tx, err := w.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
    if err != nil { return err }
    defer tx.Rollback(ctx)

    payload := buildPayload(rule, target, latest) // D-12 canonical shape
    alert, err := w.Queries.WithTx(tx).InsertAlert(ctx, sqlc.InsertAlertParams{
        RuleID: rule.ID, RuleKind: rule.RuleKind, Severity: rule.Severity,
        TargetEntityType: "metering_point", TargetEntityID: target.MeteringPointID,
        Payload: payload,
    })
    if err != nil { return err }

    _ = audit.WriteEntry(ctx, tx, audit.Entry{
        Action: "alert.fired", EntityType: "alert", EntityID: alert.ID,
        After: payload, // include for compliance reviewers
    })

    _, err = w.Queries.WithTx(tx).TouchAlertRuleLastFired(ctx, rule.ID)
    if err != nil { return err }

    // Optional: publish to Hub for live alert-drawer auto-refresh (D-20 nicety)
    w.Hub.Publish(events.AlertTopic, alert)

    return tx.Commit(ctx)
}
```

---

## Runtime State Inventory

> Phase 6 is not a rename/refactor phase. Included for completeness only.

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | No existing alert/backup tables (all new in Phase 6). User table exists; will be extended via store methods, not migration. Audit_log table exists; vocabulary migration extends CHECK constraints. | New migrations 0037..0042 (planner picks numbering). No data migration needed — `disabled_at` already exists on `user`. |
| Live service config | Retention config: `retention_config` table exists (Phase 5 D-09 / migration 0029). Phase 6 adds `alerts_days` and `audit_log_days` columns via ALTER TABLE in 0040. install_finish seeds defaults (365 / 1825). | ALTER TABLE migration + `internal/install/finish.go` extension + seed values. |
| OS-registered state | None new. The Ofelia cron sidecar is configured via compose labels, not OS cron. | Add `backup-cron` service to `compose/bundled.yml`. |
| Secrets/env vars | New: `SHIFTER_BACKUP_DIR` (default `/var/lib/shifter/backups`). All other backup config (DB host, user, password) is reused from existing `SHIFTER_DB_*` env vars. | viper config addition; documented in operator runbook. |
| Build artifacts | New: Shifter Dockerfile must bundle `postgresql16-client` (`apk add postgresql16-client` if alpine-based). Compose volumes: new `backups` named volume mounted at `/var/lib/shifter/backups`. | Dockerfile patch + compose volume entries (both bundled & external). |

---

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | Backend | ✓ | 1.26.0+ | — |
| Node.js | Frontend build | ✓ | 22.20.0 | — |
| Docker | Compose, CI testcontainers | ✓ | 29.4.1 | — |
| PostgreSQL 16 + TimescaleDB 2.26 | Backups, restore, all DB ops | ✓ via testcontainers in CI; bundled image in prod | 2.26.0-pg16 | — |
| pg_dump / pg_restore (PG16 client) | Backup runner inside Shifter image | NOT YET in Shifter image | — | Wave 0: add `apk add postgresql16-client` to Dockerfile (or equivalent debian-slim apt step) |
| mcuadros/ofelia:v0.3.22 | Bundled-mode cron sidecar | NOT YET in compose | v0.3.22 | Add service to `compose/bundled.yml` |
| robfig/cron/v3 v3.0.1 | River cron expressions in alert engine | NOT YET in go.mod | v3.0.1 | `go get github.com/robfig/cron/v3@v3.0.1` |
| docker/docker SDK | `shifter doctor` log tail | NOT YET in go.mod | client v25+ | `go get github.com/docker/docker/client` (HEAVY dep ~25MB transitive — planner can stub if size matters and gracefully degrade) |
| Docker socket `/var/run/docker.sock` | `shifter doctor --logs` | NOT mounted by default | — | Gracefully degrade: print "run `docker compose logs --tail=200 shifter` from host" |

**Missing dependencies with no fallback:** none — everything has a path.

**Missing dependencies with fallback:**
- Docker SDK in Shifter image: planner can decide to defer log-tailing to a Phase 6.5 / v1.x enhancement and ship `shifter doctor` without the log section. This avoids a 25MB+ transitive dep.

---

## Validation Architecture

> `workflow.nyquist_validation = true` in `.planning/config.json` — section required.

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go: testify v1.11.1 + testcontainers-go v0.42.0 + tcpostgres v0.42.0; Frontend: vitest + playwright (existing) |
| Config file | `go.mod` (Go); `web/vitest.config.ts` (frontend); `web/playwright.config.ts` (e2e) |
| Quick run command | `go test ./internal/alert/... ./internal/audit/... ./internal/user/... ./internal/backup/... -count=1 -short` |
| Full suite command | `go test ./... -count=1` + `pnpm test:run` + `pnpm playwright test` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| ALERT-01 | Threshold rule fires on breach; auto-clears on resolution | integration | `go test ./internal/alert/... -run TestThresholdFireAndClear` | ❌ Wave 0 |
| ALERT-02 | Offline alert needs ≥3× expected_interval; hysteresis clear at <2× | integration | `go test ./internal/alert/... -run TestOfflineHysteresis` | ❌ Wave 0 |
| ALERT-03 | Gateway-down suppresses device offline alerts | integration | `go test ./internal/alert/... -run TestGatewayDownSuppression` | ❌ Wave 0 |
| ALERT-04 | Cold-start gate prevents anomaly alerts < 21 days | integration | `go test ./internal/alert/... -run TestAnomalyColdStart` | ❌ Wave 0 |
| ALERT-05 | Alert center API returns unread count, supports ack/snooze | integration | `go test ./internal/alert/... -run TestAlertCenterAPI` | ❌ Wave 0 |
| ALERT-06 | Audit list endpoint cursor pagination + filter chips | integration | `go test ./internal/audit/... -run TestAuditCursor` | ❌ Wave 0 |
| USER-01 | List, create, update, disable user; soft-delete | integration | `go test ./internal/user/... -run TestUserCRUD` | ❌ Wave 0 |
| USER-02 | Change role triggers logout-everywhere | integration | `go test ./internal/user/... -run TestRoleChangeRevokes` | ❌ Wave 0 |
| USER-03 | Logout-everywhere DELETEs all sessions | integration | `go test ./internal/user/... -run TestLogoutEverywhere` | ❌ Wave 0 |
| USER-04 | Initial password random + must_change_password set | integration | `go test ./internal/user/... -run TestInitialPassword` | ❌ Wave 0 |
| AUDIT-02 | Browse with filters returns correct subset | integration | `go test ./internal/audit/... -run TestAuditFilters` | ❌ Wave 0 |
| AUDIT-03 | CSV export has BOM, ISO timestamps, respects filters | integration | `go test ./internal/audit/... -run TestAuditCSVExport` | ❌ Wave 0 |
| OPS-02 | Backup runner creates valid tarball with manifest | integration | `go test ./internal/backup/... -run TestBackupRunner` | ❌ Wave 0 |
| OPS-04 | Full round-trip seed → backup → drop → restore → smoke | integration | `go test ./internal/backup/... -run TestBackupRestoreRoundtrip` | ❌ Wave 0 |
| OPS-05/06/07 | Compose files have json-logging, secrets, pinned tags | unit (grep test) | `go test ./internal/compose/... -run TestComposeConventions` | ❌ Wave 0 |
| D-22 worker degraded | River subscriber flips degraded on EventKindJobFailed + discarded state | integration | `go test ./internal/alert/... -run TestDegradedSubscriber` | ❌ Wave 0 |
| D-23 password strength | Random password passes strength evaluator | unit | `go test ./internal/user/... -run TestRandomPasswordStrength` | ❌ Wave 0 |
| D-24 self-action guards | Self-demote / self-disable / last-admin-demote return 422 | unit | `go test ./internal/user/... -run TestSelfActionGuards` | ❌ Wave 0 |
| D-46 backup card | Settings backup endpoint returns age + last 5 backups | integration | `go test ./internal/settings/... -run TestBackupStatusEndpoint` | ❌ Wave 0 |
| D-50 doctor redaction | Email masking + JSON walk produces redacted output | unit | `go test ./internal/doctor/... -run TestRedact` | ❌ Wave 0 |

### Sampling Rate

- **Per task commit:** `go test ./internal/$(PACKAGE)/... -count=1 -short` for the package being edited
- **Per wave merge:** `go test ./... -count=1`
- **Phase gate:** Full suite green + `pnpm playwright test` before `/gsd-verify-work`

### Wave 0 Gaps

- [ ] `internal/alert/threshold_test.go` — threshold subtypes (ALERT-01)
- [ ] `internal/alert/offline_test.go` — hysteresis + gateway suppression (ALERT-02, ALERT-03)
- [ ] `internal/alert/anomaly_test.go` — cold-start gate + each rule kind (ALERT-04)
- [ ] `internal/alert/degraded_test.go` — River subscribe → degraded flag (D-22)
- [ ] `internal/alert/handler_test.go` — alert center API (ALERT-05)
- [ ] `internal/audit/browse_test.go` — cursor pagination + filters (AUDIT-02, ALERT-06)
- [ ] `internal/audit/export_test.go` — CSV BOM + ISO timestamps + 50k cap (AUDIT-03)
- [ ] `internal/user/store_test.go` — User CRUD + soft-delete (USER-01)
- [ ] `internal/user/handler_test.go` — Role change → revoke; logout-everywhere; self-guards (USER-02, USER-03, D-24, D-26)
- [ ] `internal/user/password_test.go` — Random password generator (USER-04, D-23)
- [ ] `internal/backup/runner_test.go` — Backup tarball + manifest (OPS-02)
- [ ] `internal/backup/restore_test.go` — Restore single-direction (smoke)
- [ ] `internal/backup/roundtrip_test.go` — Full CI round-trip (OPS-04)
- [ ] `internal/doctor/redact_test.go` — Email + JSON redaction (D-50)
- [ ] `internal/compose/conventions_test.go` — grep-based assertions against compose yml (OPS-05/06/07)
- [ ] `web/src/routes/alerts/index.test.tsx` — alert center page state machine
- [ ] `web/src/routes/audit/index.test.tsx` — URL state chips
- [ ] `web/src/routes/settings/users.test.tsx` — User CRUD UI
- [ ] `web/playwright/specs/alerts-center.spec.ts` — bell + drawer + ack flow
- [ ] `web/playwright/specs/audit-export.spec.ts` — filter → export CSV
- [ ] `web/playwright/specs/user-management.spec.ts` — create + show-once panel + logout-everywhere
- [ ] `web/playwright/specs/backup-card.spec.ts` — age dot + run-now

---

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | Yes (auth event audit retrofit; admin password reset) | Existing SCS + Argon2id + Phase 1 strength evaluator |
| V3 Session Management | Yes (logout-everywhere bulk revoke) | Existing SCS pgxstore + `iterateAndRevoke` |
| V4 Access Control | Yes (admin-only alert rule edit, audit view, backup ops, user mgmt) | Existing `Can()` middleware extended with new actions |
| V5 Input Validation | Yes (alert rule bounds, user email + role, filter chip params, restore manifest verification) | zod (frontend) + Go validation on input + sha256 verify on restore |
| V6 Cryptography | Yes (sha256 manifest checksums, Argon2id passwords) | stdlib `crypto/sha256` + existing argon2 |
| V7 Errors / Logging | Yes (doctor redaction of PII) | Custom redact pass in `internal/doctor/redact.go` |

### Known Threat Patterns

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Audit log tampering by admin | Repudiation | Existing INSERT-ONLY trigger in 0016 — no application can update/delete; T-02-04-02 |
| Backup tarball replay (restore old data) | Tampering | Manifest sha256 + per-file checksums; restore CLI verifies before pg_restore; rejection on mismatch |
| Restore while Shifter is serving (double-write corruption) | Tampering / DoS | PG advisory lock acquired by restore CLI; refuses if Shifter is up |
| Plaintext password leaks via show-once panel | Information Disclosure | Password is in-memory only in React state; never persisted; cleared on dialog close; sent over TLS only (D-22) |
| Session fixation on role change | Spoofing | Role change triggers session revoke; user must re-auth → new session with new role; D-25 |
| Backup destination path traversal | Tampering | `SHIFTER_BACKUP_DIR` is validated as absolute path; manifest paths inside tarball are sanitized (no `..`) |
| Filter param SQL injection | Tampering | All filter values bound as positional pgx params; entity_type and action filters are validated against an enum set in zod (frontend) + sqlc-typed parameter (backend) |
| Audit export CSV injection (formula injection in Excel) | Tampering | Prepend `'` to any cell starting with `=`, `+`, `-`, `@`; same mitigation as REPT-03 CSV in Phase 5 (planner verifies pattern already exists) |
| Last-admin demote race (TOCTOU) | DoS | Serializable transaction with `RejectLastAdminDemote` inside the tx |
| Doctor bundle leaks PII | Information Disclosure | Mask emails (`MaskEmail`), regex-replace email-shaped tokens in JSON before output; documented |
| Docker socket exposure for ofelia/doctor | Elevation of Privilege | `/var/run/docker.sock` is mounted READ-ONLY (`:ro`) into the cron sidecar; only the sidecar (not Shifter) has Docker API access |

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `timescaledb-backup` helper tool | `pg_dump --format=custom` + `timescaledb_pre_restore()` / `timescaledb_post_restore()` | Feb 2022 (helper archived) | Phase 6 ships the modern path; reduces dep surface; aligned with upstream |
| `pg_restore --jobs=N` for speed | Single-threaded `pg_restore` for TimescaleDB | Documented warning, always true | CI round-trip uses single-threaded; lint check prevents accidental `-j` |
| Browser-side encrypted JWTs for sessions | Server-side sessions in Postgres via SCS pgxstore | Phase 1 D-23 lock | Phase 6 leverages: bulk-revoke is trivial; XSS-exfiltration immune |
| Audit log as hypertable with TimescaleDB compression | Regular Postgres table with application-level prune cron | Phase 2 D-06 lock | Phase 6 reuses retention reconciliation pattern; no TS-policy needed |
| Alert engine as standalone service / cron | River background job + Postgres-native | Phase 5 wired River | Phase 6 adds workers; no infra additions |
| Custom session-by-user-id lookup schema | `scs.Iterate` over `sessions` table + token DELETE | Phase 1 D-23 lock | Existing `iterateAndRevoke` pattern reused verbatim |
| Cron in containers | Sidecar pattern (Ofelia) | Industry shift 2020+ | Smaller image, cleaner separation of concerns |
| In-app SMTP for alerts | Structured payload + future webhook deliverer | PROJECT.md v1 constraint | Phase 6 stores webhook-ready payload now; V2 adds deliverer with zero migration |

**Deprecated/outdated:**
- `timescaledb-backup` — archived 2022; do not use
- `pg_restore -j` for TimescaleDB — official warning, persistent
- `willfarrell/crontab` Docker image — last release 2022, unmaintained
- `lib/pq` Postgres driver — maintenance mode since 2021; use pgx (already pinned)
- `gofpdf` — archived 2021 (already excluded by CLAUDE.md)

---

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Shifter Dockerfile is alpine-based and can `apk add postgresql16-client` | Backup Strategy / Environment | If image is distroless or scratch, need a more complex multi-stage build. Wave 0 verifies. |
| A2 | `chirpstack_connection.mode` is the canonical source for "is bundled?" (not `install_state.chirpstack_mode` as CONTEXT.md says) | Backup Strategy / D-41 | Verified in `internal/db/migrations/0006_chirpstack_connection.up.sql` — CONTEXT.md text was imprecise. No code change to planner — just use the right table. |
| A3 | Quiet-window storage as nullable columns on alert_rule is preferred over separate quiet_hour_config table | § Decision E | Reversible if Phase 7 demands richer per-MP quiet-window config; alter table to add `quiet_hour_id` FK |
| A4 | `percentile_cont` on 30d × hour-bucket × 500 MPs runs sub-second | § Decision E | Wave 0 task: EXPLAIN against existing measurement hypertable for one MP. If slow, add a `measurement_hourly_p95` CAGG. |
| A5 | River v0.36.0's `EventKindJobFailed` event reliably fires when state transitions to `discarded` | § Decision C | Wave 0 test: TestDegradedSubscriber asserts subscribe → discarded flow against a worker that always returns error |
| A6 | Docker SDK is acceptable transitive dep size for `shifter doctor --logs` | § Decision H | If size is unacceptable, ship doctor without log tail; document "use docker compose logs --tail=200" |
| A7 | Audit-event retrofit (D-30) doesn't break existing Plan 01-09 / 01-11 test mocks | § Decision G | Wave 0 reviews `internal/auth/handlers_test.go` and `internal/auth/account_test.go`; if mocks block, swap to testcontainer fixtures first |
| A8 | Cross-version restore CI variant can defer to Phase 6.5 / v1.x | § Decision B | If a customer demands cross-version restore at v1.0 launch, planner adds a stub job + filed v1.1 enhancement |
| A9 | `cron.ParseStandard("CRON_TZ=Asia/Bangkok 0 2 * * *")` works in River v0.36 PeriodicJob | § Decision C | Phase 5 already shipped River cron without TZ; Wave 0 verifies TZ syntax with a unit test |
| A10 | The audit_log INSERT-ONLY trigger does not impact retention prune cron | § Decision F | The trigger rejects UPDATE / DELETE on user rows; the DELETE in PruneAuditRows must NOT be rejected. **Risk: trigger applies to DELETE!** Wave 0 verifies — if rejected, we add a `_audit_prune` role exemption or use TRUNCATE with date partition. |

**⚠️ A10 is HIGH risk. Wave 0 MUST verify this.** Look at migration 0016 line 67-75: `CREATE TRIGGER audit_log_no_delete BEFORE DELETE ON audit_log` — yes, **all DELETEs are rejected**. Phase 6 has two options:

**Option α — Bypass trigger via SECURITY DEFINER function:**
```sql
CREATE FUNCTION admin_prune_audit_rows(cutoff_days INTEGER) RETURNS INTEGER
LANGUAGE plpgsql SECURITY DEFINER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    -- Temporarily disable the trigger for this function's tx
    ALTER TABLE audit_log DISABLE TRIGGER audit_log_no_delete;
    DELETE FROM audit_log WHERE time < now() - make_interval(days := cutoff_days);
    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    ALTER TABLE audit_log ENABLE TRIGGER audit_log_no_delete;
    RETURN deleted_count;
END $$;
```

**Option β — Convert audit_log to time-range partitions and DROP whole partitions** (no DELETE → trigger doesn't fire):
```sql
-- Convert audit_log to partitioned table by year; retention prune = DROP PARTITION
ALTER TABLE audit_log … PARTITION BY RANGE (time); -- requires data migration
```

**Recommendation: Option α** — minimal change, preserves "INSERT-ONLY at application layer" invariant, while allowing a single tightly-scoped admin function to prune. The function should:
- Be SECURITY DEFINER (runs as table owner)
- Be granted EXECUTE only to the Shifter DB role (which already owns the table)
- Write an audit_log entry BEFORE the disable (so the prune itself is logged before the lock-down lifts)
- Disable + re-enable trigger within a single transaction so no other actor can sneak a DELETE in

**Wave 0 task: validate by writing TestAuditPruneRespectsTrigger that runs the function and asserts: (1) trigger still rejects raw `DELETE FROM audit_log` from a regular query; (2) admin_prune_audit_rows correctly deletes old rows; (3) the trigger is re-enabled after the function returns.**

---

## Open Questions

1. **Cross-version restore (vN backup → vN+1 schema) — Phase 6 or v1.1?**
   - What we know: Same-version round-trip is well-scoped and achievable in Phase 6. Cross-version adds release-management coupling.
   - What's unclear: Whether ROADMAP's "cross-version restore" research flag explicitly requires a CI test in v1, or just the runbook + manifest design.
   - Recommendation: **Ship same-version test in Phase 6 + design forward-compat manifest with `db_schema_version` field. File cross-version CI as v1.1 prep.**

2. **Should the audit_log INSERT-ONLY trigger be bypassed for prune?**
   - What we know: Migration 0016 explicitly forbids DELETE via trigger. Phase 6 needs to prune for retention.
   - What's unclear: Whether the planner / discuss should resurface this design to the user (the trigger is load-bearing security per T-02-04-02).
   - Recommendation: **Resurface to user in discuss-phase before planning.** Options are (α) SECURITY DEFINER bypass with audit trail OR (β) partition + DROP PARTITION. Both work; α is simpler.

3. **Docker SDK dependency size for `shifter doctor` log tail**
   - What we know: github.com/docker/docker/client pulls in 25MB+ of transitive deps (the full Docker engine API surface).
   - What's unclear: Whether the operator value of one-shot log tail justifies the binary bloat.
   - Recommendation: **Ship `shifter doctor` WITHOUT log tail in Phase 6; document the fallback in the redacted bundle. Add `--logs` flag in v1.1 if customer feedback names it.**

4. **Alert engine timezone correctness for quiet-hour windows**
   - What we know: The install_state has a timezone (Phase 1 D-17). Quiet-window must apply in install_tz.
   - What's unclear: Whether the worker pulls install_tz once at boot or per-rule per-eval (handles operator changing tz at runtime).
   - Recommendation: **Pull install_tz on every eval cycle** (1 extra SELECT per cycle). Trivial cost; correctness benefit.

5. **D-30 audit retrofit scope — only login/logout, or also session refresh / CSRF token rotation?**
   - What we know: D-30 lists `auth.login_success/failed/logout/password_change/password_reset_by_admin/session_revoked`.
   - What's unclear: Whether silent session-refresh events (SCS rotates token per session.PutUser call) should also be audited.
   - Recommendation: **Do NOT audit silent refreshes. Audit only operator-visible auth events.** Otherwise audit_log fills with `auth.session_refreshed` noise.

---

## Sources

### Primary (HIGH confidence)
- [VERIFIED: `cat go.mod`] — River v0.36.0 + riverpgxv5 + maroto v2.4.0 + excelize v2.10.0 + scs v2.9.0 already pinned
- [VERIFIED: `go list -m -versions github.com/riverqueue/river`] — v0.37.0 latest; v0.36.0 currently pinned; no breaking changes per CHANGELOG
- [VERIFIED: github.com/timescale/timescaledb-backup README] — repository archived 2022-02-19
- [VERIFIED: existing code at `internal/auth/account.go::iterateAndRevoke`] — bulk session revoke pattern already in production
- [VERIFIED: existing code at `internal/cli/serve.go`] — River client + worker setup pattern from Phase 5
- [VERIFIED: existing code at `internal/db/migrations/0016_audit_log.up.sql`] — INSERT-ONLY trigger explicitly rejects DELETE
- [VERIFIED: existing code at `internal/db/migrations/0006_chirpstack_connection.up.sql`] — `mode` column is the canonical source for bundled/external
- [CITED: docs.tigerdata.com/self-hosted/latest/backup-and-restore/logical-backup] — pg_dump -Fc; do not use -j on pg_restore; pre_restore/post_restore semantics
- [CITED: docs.tigerdata.com/api/latest/administration/timescaledb_pre_restore] — pre_restore disables background workers; post_restore re-enables
- [CITED: riverqueue.com/docs/periodic-jobs] — PeriodicInterval, cron.ParseStandard, CRON_TZ prefix, leader-election
- [CITED: riverqueue.com/docs/job-retries] — JobArgsWithInsertOpts pattern for MaxAttempts
- [CITED: riverqueue.com/docs/subscriptions] — EventKindJobFailed + job.State distinction for retry-vs-discarded
- [CITED: github.com/mcuadros/ofelia README] — v0.3.22 (Apr 2026); --docker mode; label-driven jobs; 3.8k stars
- [CITED: postgresql.org/docs/current/sql-expressions.html#SQL-SYNTAX-ROW-CONSTRUCTORS] — row-comparison for cursor pagination
- [CITED: postgresql.org/docs/current/functions-aggregate.html] — `percentile_cont` syntax
- [CITED: pkg.go.dev/github.com/alexedwards/scs/v2 + pgxstore] — sessions table schema (token PK, BYTEA data)

### Secondary (MEDIUM confidence)
- [WebSearch verified] — TimescaleDB 2.26 changelog (Mar 2026); 2.27 not yet released as of research date
- [WebSearch verified] — Ofelia vs willfarrell/crontab maintenance status

### Tertiary (LOW confidence)
- [ASSUMED] — `percentile_cont` performance at 500 MPs × 30d (Wave 0 EXPLAIN required)
- [ASSUMED] — Shifter Dockerfile is alpine-based (Wave 0 inspection required)
- [ASSUMED] — Quiet-window storage as alert_rule columns vs separate config table (reversible)
- [ASSUMED] — Docker SDK transitive dep size estimate (planner may opt to defer log-tail feature)
- [ASSUMED] — Cross-version restore CI deferral is acceptable per ROADMAP flag scope (planner should ratify)

---

## Planner Handoff

The single most important thing for the planner to internalize: **Phase 6 is mostly additive. The only structural risk is the audit_log INSERT-ONLY trigger blocking retention prune (Assumption A10).** Address that first.

1. **Backup strategy is settled: Option B** (`pg_dump --format=custom` + `timescaledb_pre_restore()`/`_post_restore()`). The `timescaledb-backup` helper is archived; do not pursue Option A. Bundle `postgresql16-client` in the Shifter Dockerfile (Wave 0 task).

2. **CI round-trip is Phase 6 scope; cross-version restore defers** to v1.1. The manifest's `db_schema_version` field is the forward-compat hook.

3. **Audit prune needs a SECURITY DEFINER bypass function** (`admin_prune_audit_rows`) because migration 0016's trigger rejects ALL DELETEs. This is a small migration but a load-bearing security design — surface to user via discuss-phase before locking the approach.

4. **Alert engine reuses Phase 5's River client.** Six new workers (3 threshold subtypes + offline + anomaly + audit prune). One queue. MaxAttempts=3 via `JobArgsWithInsertOpts`. Degraded flag via `riverClient.Subscribe(river.EventKindJobFailed)` + check `job.State == JobStateDiscarded`.

5. **Cool-down is in-engine** (worker reads `last_fired_at + cooldown_seconds`), not a DB partial unique index. Simpler. Test-fire alerts (D-19) do NOT update `last_fired_at`.

6. **D-30 audit retrofit is Wave 1 work, not a separate phase.** It restructures `internal/auth/handlers.go` to begin a tx, write audit_log row, commit, then write session. Existing tests need conversion to testcontainer-backed tests that assert audit_log row counts.

7. **D-24 logout-everywhere is already-shipping code** (`iterateAndRevoke`). The four callers (explicit click, disable, role change, password reset) all invoke it with `keepToken=""`.

8. **Cron sidecar is `mcuadros/ofelia:v0.3.22`** in `compose/bundled.yml` only. External-mode operators document their own cron in the runbook.

9. **Statistical anomaly schema ships in Phase 6; threshold tuning ships in Phase 7.** The `alert_rule` schema is forward-compatible — Phase 7 adds `multiplier` / `sensitivity` columns without breaking v1 rules.

10. **Migration numbering: 0037..0042 reserved for Phase 6.** Planner picks exact order. Suggested: `0037_audit_vocab_phase6` → `0038_alert_rule` → `0039_alert` → `0040_retention_config_phase6` → `0041_backup_run` → `0042_alert_worker_state` + `0043_audit_prune_function` (the SECURITY DEFINER).

11. **Estimated plan count: 14 plans.** Suggested decomposition: Wave 0 (test infra + migrations + Dockerfile patch) → Plans 06-01, 06-02. Alert engine backend → 06-03 (threshold), 06-04 (offline + gateway), 06-05 (anomaly). Alert center UI → 06-06. User management → 06-07 (store + handlers), 06-08 (UI). Audit retrofit + browse → 06-09 (D-30 retrofit), 06-10 (browse + export). Backup/restore → 06-11 (runner + CLI), 06-12 (Settings card + sidecar). Doctor + runbook + OPS verification → 06-13. Phase closure → 06-14. Adjust by complexity.

---

## Metadata

**Confidence breakdown:**
- Backup strategy: **HIGH** — official docs + archived upstream tool make Option B unambiguous
- CI round-trip: **HIGH** — testcontainers + existing patterns from Phase 5
- River alert engine: **HIGH** — verified against official docs; Phase 5 wired the harness
- Audit browse cursor pagination: **HIGH** — row-comparison is well-defined SQL; Phase 4 D-15 pattern reused
- Anomaly rule SQL costs: **MEDIUM** — performance estimated, Wave 0 EXPLAIN needed
- SCS bulk revoke: **HIGH** — existing code, already exercised
- Audit retention prune via trigger bypass: **MEDIUM** — design is sound, but security-design needs user confirmation
- Cron sidecar: **HIGH** — Ofelia is well-maintained Go binary; label-driven config is documented
- `shifter doctor`: **MEDIUM** — bundle shape clear; Docker SDK dep cost uncertain

**Research date:** 2026-05-12
**Valid until:** 2026-06-12 (30 days — stable ecosystem; TimescaleDB and River are slow-moving; revisit if v1.1 emerges or if upstream archives a key dep)
