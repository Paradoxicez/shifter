# Phase 6: Alerts, Users, Audit & Operational Hardening - Context

**Gathered:** 2026-05-12
**Status:** Ready for planning

<domain>
## Phase Boundary

Phase 6 is the "ship to a paying customer" gate. It delivers three product surfaces — **alerts**, **user management**, **audit browse + export** — on top of the substrate Phases 1–5 already built (`audit_log` is written-on-every-mutation since Phase 2, SCS sessions live in Postgres, River is installed for jobs, Settings is categorized) and closes the **operational hardening** loop (backup/restore, secrets/logs/pinning audit, upgrade runbook, support-diagnostic CLI) so the binary can be handed to a customer with confidence.

Requirements in scope (24 items): **ALERT-01..06**, **USER-01..04**, **AUDIT-02..03**, **SETT-02**, **SETT-05**, **OPS-02..08**.

Already-complete substrate that Phase 6 builds ON, not against:
- `audit_log` table + write-in-tx pattern (Phase 2 D-21/D-22, AUDIT-01 complete)
- `"user"` table with `disabled_at` soft-delete + `must_change_password` (Phase 1)
- SCS session manager in Postgres + Argon2id + per-IP/per-username rate limit (Phase 1 D-23, AUTH-01/02/04 complete)
- River v0.13 (Phase 5 PDF worker — alert worker reuses)
- Settings categorized page with install identity, ChirpStack connection, units, timezone, alerts placeholder, data retention, backup-status placeholder (SETT-01 complete, SETT-03 complete, SETT-04 complete)
- json-file logging caps, Compose `secrets:`, pinned image tags (Phase 1 D-24 / Plan 01-20 + 01-21) — Phase 6 verifies + closes any drift
- `/health/detailed` admin-auth endpoint (Phase 1 D-19) — Phase 6 adds `alert_worker` row and richer payload

Out of scope (explicitly deferred):
- **Statistical-anomaly threshold tuning** — Phase 7 territory; Phase 6 ships the cold-start gate + the three rule kinds (P95 / IQR / quiet-hour) but their parameters get empirical tuning in Phase 7 against real-customer data
- **Email / SMTP / webhook delivery of alerts** — PROJECT.md "no SMTP in v1"; V2-NOTIF-01/-02. Phase 6 records structured payload so v2 webhook ships without migration
- **Per-user dashboards / saved views / per-rule subscribers** — V2-AUTH-02; Phase 6 is shared-inbox single-tenant
- **SSO / OAuth / SCIM provisioning** — PROJECT.md "Local accounts only"; V2-AUTH-01
- **S3-compatible backup destination** — V2-OPS-01; Phase 6 ships local path only with the file format ready to be uploaded externally
- **Scheduled backup UI inside the binary** — operator wires their own cron; v1.x can add a Settings scheduler
- **Hot restore (no-downtime)** — restore requires `shifter` stopped (PG advisory lock); zero-downtime upgrades not pursued (single-tenant install posture)
- **In-app ChirpStack version upgrade** — PROJECT.md hard exclusion
- **Live-tail SSE for audit browse** — audit is a forensic surface, not a live monitor

</domain>

<decisions>
## Implementation Decisions

### Alerts — engine + storage

- **D-01:** **Alert rules in DB + River cron worker.** Rules persisted as `alert_rule` rows; a River cron job evaluates active rules every N minutes (planner picks per-subtype cadence — see D-02). Reuses Phase 5's River install (no Redis dep). Durable retries, observable via River dashboard, easy to add new rule kinds without restart.
- **D-02:** **Three threshold-rule subtypes per CAGG level.** `threshold_instantaneous` (queries raw `measurement` for latest value per MP), `threshold_hourly` (queries `cagg_hourly`), `threshold_daily` (queries `cagg_daily`). Each rule picks one subtype + high/low bound + comparison side. Maps cleanly to the data layer; each subtype is one SELECT against one table. Worker cadence: instantaneous every 1 min, hourly every 15 min, daily every 1 h (suggested — planner finalizes).
- **D-03:** **Three rule kinds, three workers, one River queue.** Threshold + offline + anomaly each have their own evaluator; all three land in the same River queue so observability and retry policy are uniform. Engine code shares an `EvaluateContext` (db, hub, audit, install_tz) so adding rule kinds doesn't fan out infrastructure.
- **D-04:** **Rules are soft-deletable via `disabled_at`** (mirrors Phase 3 gateway / Phase 2 metering-point pattern). Rule library defaults to enabled; "Show disabled" toggle reveals others. Disabled rules don't fire; their fired-alert history stays queryable. Re-enable possible; both events audited.
- **D-05:** **Per-rule cool-down field, default 900 s (15 min).** After a rule fires for a given target, it can't fire again for that target until `cooldown_seconds` elapses. Operator can override per-rule (longer for noisy battery rules, 0 for critical thresholds). Prevents flap spam without losing high-signal repeat fires.
- **D-06:** **Optional human name + notes field on every rule.** Auto-generated default name (e.g., "High flow rate — Building A meter"); operator can override + add `notes TEXT`. Notes surface in rule library and on fired alert rows. Helps multi-operator hand-off.

### Alerts — fired-alert lifecycle

- **D-07:** **Three-tier severity with locked color map.** `info` (slate), `warning` (yellow), `critical` (red). Default severities by rule kind: threshold-breach + offline + anomaly = `critical`; battery-low / signal-degraded / cold-start-eligibility = `warning`; first-uplink-from-new-device = `info`. Operator can override per rule. Color used on alert center badge, drawer rows, severity chip, severity dot in shell bell badge.
- **D-08:** **Auto-clear when condition resolves + audit row.** Threshold / offline / anomaly alerts auto-transition to `cleared` on the next eval cycle when the condition no longer holds (value back in range, device online again). An audit row records the auto-clear. Acked alerts stay `acknowledged`; they re-arm only after clearing (so an unresolved-but-acked condition does not generate duplicate alerts).
- **D-09:** **Snooze presets 1h / 8h / 24h / 7d + "Mute until I clear".** Four-button quick presets cover shift/day/week timeboxes; indefinite mute (`muted=true`) requires explicit unmute. Mirrors Slack / Linear ergonomics; mobile-friendly.
- **D-10:** **Snooze/mute is global to the alert (shared inbox).** Snooze state lives on the alert row itself. Whoever snoozes sets it for everyone. Single source of truth; matches the single-tenant operator-team install posture. Audit row records who snoozed.
- **D-11:** **Routing v1 = no routing.** Every authenticated user (admin + viewer) sees the same alert center. Admins can ack/snooze; viewers see read-only. Per-user routing / per-rule subscribers is v2 (V2-AUTH-02). Simpler v1, no schema additions.
- **D-12:** **Structured alert payload from day 1.** `alert` table has `payload JSONB` with the stable schema: `{rule_id, rule_kind, severity, target: {entity_type, entity_id, label}, value, threshold, comparison, fired_at, install: {display_name}}`. Same shape powers in-app alert card AND future webhook POST body (V2-NOTIF-01); v2 webhook ships by adding a deliverer with no migration / no backfill.
- **D-13:** **Alert retention = 365 days default, Settings-toggle-able.** Add an "Alerts" row to the Settings → Data Retention card (same shape as SETT-04 / Phase 5 D-09). Reuses the same-tx retention reconciliation flow. Default one year fits compliance + audit common asks without unbounded growth.

### Alerts — engine specifics

- **D-14:** **ALERT-03 gateway-down suppression = eval-time check against gateway last_seen.** When the offline-evaluator is about to raise a device-offline alert for device D, it first checks: does D's last-known gateway have `last_seen_at` older than the ALERT-02 threshold? If yes, suppress the device alert and ensure a single `gateway_offline` alert exists for that gateway. Stateless suppression. Survives device-gateway mapping changes mid-outage. Pair with: when the gateway comes back online, the next offline-evaluator cycle naturally re-checks each device.
- **D-15:** **ALERT-02 offline threshold = N≥3 missed expected uplinks + hysteresis.** Per-profile `expected_interval_s` already on `device_profile` (Phase 2). Threshold = `now() - last_uplink > 3 × expected_interval_s` to fire; hysteresis grace = `now() - last_uplink < 2 × expected_interval_s` to clear (looser clear bound so flapping devices don't re-fire immediately). **This is intentionally STRICTER than Phase 4 D-07's KPI flicker threshold** (Phase 4 D-07: `2× expected_interval_s` for online/offline dashboard tile; ALERT-02 is for paging alerts) — two thresholds, two purposes, both documented.
- **D-16:** **ALERT-04 cold-start gate = visible per-meter status chip.** MP detail page surfaces an "Anomaly detection" card: "Warming up — 16 days of history remaining before anomaly alerts fire" with progress bar. Settings → Alerts page lists the warmup roster ("Anomaly detection: 4 meters eligible, 12 warming up"). When eligible, MP card flips to "Anomaly detection: active (P95 / IQR / quiet-hour rules)". Cold-start is documented behavior, not a bug — operator can explain to customer why no anomaly alerts yet.
- **D-17:** **ALERT-04 ships all three statistical rules, opt-in per-MP.** `anomaly_p95` (value > P95 of trailing 30 days for this MP × time-of-day bucket), `anomaly_iqr` (value outside Q1-1.5·IQR to Q3+1.5·IQR), `anomaly_quiet_hour` (non-zero flow during operator-configured quiet window). Each MP can enable/disable each rule. Water installs typically enable quiet-hour-flow (leak signature); electricity installs lean on P95. Phase 7 tunes parameters from real-customer data — Phase 6 ships the shape.
- **D-18:** **Three entry points for alert rule creation, one dialog.** (a) MP detail → "Add alert rule" button (rule scoped to this MP). (b) Site detail → "Add alert rule" (rule scoped to all MPs in this site). (c) Settings → Alerts → rule library page lists all rules + bulk enable/disable + "Add rule" button (scope chosen inside dialog). Lower friction at the point of context; central library for governance.
- **D-19:** **Test-fire button in rule dialog (UI smoke test, not engine dry-run).** Dialog "Test fire" raises one synthetic alert row with `is_test = true`, severity `info`, auto-clears after 60 s, badge labeled `TEST`. Operator verifies bell, drawer, page, ack flow all work for this rule's target. Doesn't execute the rule logic against measurements — that's an engine dry-run (deferred to Phase 7 anomaly-tuning needs).

### Alerts — center UX

- **D-20:** **Header bell + slide-over drawer + dedicated `/alerts` page.** Top-right bell in the app shell with unread badge (counts unread `critical` first, then `warning`, severity-tinted dot). Click bell → slide-over drawer (last 10 alerts, ack/snooze inline, "See all" link). `/alerts` page hosts the full list with URL-state filter chips (severity, category, status, target). Mirrors GitHub / Linear / Slack pattern. Drawer keeps current task context; page for deep review.
- **D-21:** **Alert worker observability on `/health/detailed`.** Add `alert_worker` row: `{last_run_at, rules_evaluated, fires_emitted, cleared, duration_ms, degraded, last_error}`. Drives the shell-banner "Alert evaluation degraded — last successful run 2h ago" when `degraded = true`. Matches Phase 1 D-19 detailed-health pattern.
- **D-22:** **Worker failure handling = River retry + degraded-state banner.** River's exponential-backoff retry (max 3 attempts) handles transient errors. If a cron run still fails after retries, set `alert_worker_degraded` flag; shell banner renders; click → `/health/detailed`. Prevents silent alert blindness without alert-fatigue from per-error pages.

### User Management

- **D-23:** **USER-04 initial password = random + show-once panel.** Backend generates a strong random password (≥16 chars from full ASCII alphabet excluding ambiguous chars `1lI0O` — let Phase 1's strength evaluator pass on it). Add-user dialog flips on success to a "Share these credentials" panel showing email + plaintext password + Copy button + "I've shared this" confirmation. After confirmation, the password never appears again. `must_change_password = true` so user rotates on first login. Operator never types the password — reduces weak-password risk and reflects the no-SMTP install context.
- **D-24:** **USER-03 "Logout everywhere" = admin button + confirm AlertDialog → immediate kick.** Users page row action; AlertDialog confirms the side-effect ("User will be signed out across all devices immediately"). Backend DELETEs all SCS session rows where `user_id = $1`. Audit row. User's next request returns 401. The same DELETE is invoked implicitly on (a) admin disables user, (b) admin changes user's role, (c) admin resets user's password, (d) explicit Logout-everywhere click — four implicit triggers, one code path.
- **D-25:** **Role-change semantics = auto-logout-everywhere on change.** Setting `role` on a user atomically triggers the same session-revoke as USER-03 (one of the four implicit triggers in D-24). User must sign back in for the new role to load. Avoids stale `auth.User` context cached server-side and stale role-derived data in the React Query client cache. Audit row.
- **D-26:** **Block self-demote, self-disable, and last-admin-demote.** Server-side guards: cannot change own role, cannot disable own account, cannot leave zero admins. Users page UI greys out the affected actions on the row matching `current_user.id` and on the row that would leave zero admins. Lockout recovery uses the Phase 1 D-14 `shifter create-admin` CLI escape hatch (unchanged).
- **D-27:** **Re-enable preserves original credentials + state.** Users list defaults to active; "Show disabled" toggle (mirrors Phase 3 archive pattern). Re-enable sets `disabled_at = NULL`; user resumes with the same email, password hash, and `must_change_password` flag they had at disable. Audit row. No forced password reset on re-enable (admin can chain Reset Password if they want one).
- **D-28:** **Password strength policy = reuse Phase 1 evaluator + same min-score for every surface.** `internal/auth/password_strength.go` is the single source of truth. Applies to: (a) admin Add-User (random gens must pass, admin-typed also must pass), (b) USER-04 force-change-on-first-login, (c) account-menu self-change (Phase 1 already does this), (d) admin Reset-Password flow.
- **D-29:** **Users page lives at Settings → Users tab.** SETT-01 already enumerates "Users" as a Settings category (alongside install identity, ChirpStack, units, etc.). Route: `/settings/users`. Lists + dialogs follow Phase 3 pattern (gateways/devices). Sidebar stays clean; no role-conditional top-level nav items.
- **D-30:** **Auth-event audit retrofit (closes Phase 2 D-21 deferred work) — operator-visible events only.** Add audit vocabulary: `auth.login_success`, `auth.login_failed`, `auth.logout`, `auth.password_change`, `auth.password_reset_by_admin`, `auth.session_revoked`, `user.create`, `user.update`, `user.disable`, `user.enable`, `user.role_change`. Retrofit Plan 01-09 (login/logout/change-password handlers) + Plan 01-11 (account UI) to write inside the same tx. New user-mgmt handlers also write in-tx. **Scope = operator-visible events only**: silent SCS session refreshes / idle-timeout expirations / per-request session reads are NOT audited (too volume-heavy, no operator value). Auditing fires only on the verbs above (explicit action by a user or by an admin). **Resolves researcher Open Question #4.** Closes AUDIT-01 + AUTH-04 + USER-04 compliance gaps in one migration.

### Audit Log — Browse & Export

- **D-31:** **Audit browse lives at top-level `/audit`, admin-only sidebar item.** Audit is the compliance surface — it deserves a dedicated nav slot. Sidebar item below "Reports" gated by `auth.Can(user, "audit.read")` (admin-only); hidden for viewers (consistent with Phase 4 role-conditional shell). Page hosts filter chips + table + export.
- **D-32:** **Filter UI = URL-state filter chips above table.** Chips: Date range, User, Entity type, Action, Request ID (free-text). Each chip uses the Phase 3 D-15 / Phase 4 D-14 `useSearchParams + zod` pattern. URL deep-linkable: `/audit?from=2026-04-01&to=2026-05-01&user_id=...&entity_type=device&action=swap`. Chip clear/clear-all UX. Empty state on zero rows.
- **D-33:** **Default view on cold-arrival to `/audit` = last 7 days, all entity types, all users.** Mirrors typical SIEM/audit-tool defaults. Filter chip "Last 7 days" rendered as active. Fast first paint (low row count). Operator adjusts date range as needed.
- **D-34:** **Per-row diff = expandable JsonTree (reuse Phase 4 D-19 component).** Table row has an expand affordance. Expanded view shows side-by-side `before` / `after` JsonTree with changed keys highlighted yellow. Mobile collapses to stacked. Default-collapsed keeps the table scan-friendly.
- **D-35:** **CSV export mirrors REPT-03 spec + respects current filter chips.** UTF-8 BOM, ISO-8601 timestamps in install timezone, timezone label in header block. Columns: `time, user_email, user_id, action, entity_type, entity_id, request_id, notes, before_json, after_json` (last two as compact JSON strings). Export button respects active filters — you export exactly what you see. Cap 50 000 rows; above that, "Generate full export" runs as a River background job (same pattern as REPT-06 PDF) with a downloadable artifact + 24 h TTL.
- **D-36:** **Pagination = cursor pagination by `(time DESC, id)`.** Page size 100. "Load more" button at the bottom. Backed by the `audit_log (time DESC)` index that already exists (migration 0016). Stable under concurrent inserts (no row-skip / row-duplicate that offset pagination has). Mirrors Phase 4 D-15 uplinks log pattern.
- **D-37:** **No live-tail / no SSE for audit.** Audit is a forensic / compliance surface, not a live monitor. Manual "Refresh" button + relative-time formatting ("2 min ago") suffice. Avoids spinning up an SSE topic for low-cardinality state. v2 could add a `?live=1` mode if incident-response use cases demand it.
- **D-38:** **Audit retention = 5 years default + Settings toggle.** Mirrors daily-CAGG retention (Phase 5 D-09); 5y is a sensible compliance baseline for self-hosted utility monitoring (low volume ~hundreds/day per Phase 2 D-06 → ~1 M rows over 5y, trivially fast). Add "Audit log" row to Settings → Data Retention card alongside the "Alerts" row from D-13. Same-tx retention reconciliation flow from Phase 5. Prune mechanism: D-51.
- **D-51:** **Audit prune via `admin_prune_audit_rows()` SECURITY DEFINER function** (works around the migration 0016 INSERT-ONLY trigger that blocks DELETE on `audit_log`). New migration ships a SQL function owned by a dedicated non-superuser role (`shifter_audit_admin`). Inside the function: `SET LOCAL session_replication_role = 'replica'` (disables triggers for this tx only), `DELETE FROM audit_log WHERE time < $1`, returns the row count. Function called only from one place — the River audit-retention cron job (daily, 02:30 install_tz). Function definition lives in migration; any edit goes through migration review. The trigger itself stays unchanged — invariant "no DELETE on audit_log" holds at the table level for every code path except the documented escape hatch. The function ALSO writes a meta audit row before returning: `INSERT INTO audit_log (action, entity_type, entity_id, notes) VALUES ('audit.prune', 'audit_log', gen_random_uuid(), 'Pruned N rows older than YYYY-MM-DD')` (INSERT is permitted by the trigger). Meta-event is itself subject to retention — eventually self-prunes. Compliance reviewer can grep `audit.prune` rows for full trail of retention activity. **Resolves researcher Open Question #2.**

### Backup & Restore (OPS-02..04)

- **D-39:** **Backup format = single tar.gz containing `pg_dump` (Shifter + ChirpStack DBs if bundled) + floor-plan dir + manifest.json.** Manifest contains: `{shifter_version, db_schema_version, chirpstack_mode: bundled|external, included: [...paths...], started_at, finished_at, sha256_sums: {...}}`. Single artifact per backup. Easy to copy / upload / inspect. Tarball name: `shifter-backup-<install_slug>-<YYYYMMDD-HHMM>-<schema_version>.tar.gz`.
- **D-40:** **TimescaleDB-aware pg_dump strategy = raw `pg_dump --format=custom` + `timescaledb_pre_restore()` / `timescaledb_post_restore()` hooks (research-resolved).** The `timescaledb-backup` helper tool was **archived by TigerData in Feb 2022** with an explicit "use raw pg_dump/pg_restore instead" directive — Option B is the only maintained path. **Backup CLI sequence:** `pg_dump --format=custom --file=db/shifter.dump --dbname=$SHIFTER_DB` (+ same for ChirpStack in bundled mode per D-41). **Restore CLI sequence:** `psql -c "SELECT timescaledb_pre_restore();"` → `pg_restore --dbname=$SHIFTER_DB db/shifter.dump` → `psql -c "SELECT timescaledb_post_restore();"`. Captures schema + data + extension internal state (CAGGs, retention policies, compression policies). Sources: https://www.tigerdata.com/docs/self-hosted/latest/backup-and-restore/logical-backup, https://www.tigerdata.com/docs/api/latest/administration/timescaledb_pre_restore/, archived https://github.com/timescale/timescaledb-backup README. Plan uses `--no-owner --no-acl` on `pg_dump` and `pg_restore` to enable cross-host restore where role names differ; the literal command line in operator-runbook documents this addition.
- **D-41:** **Bundled mode backup includes ChirpStack DB; external mode is Shifter-only.** Detected via `install_state.chirpstack_mode`. Bundled mode: both Shifter's and ChirpStack's databases share the same Postgres instance — `pg_dump` runs against each, results stored under `db/shifter.dump` and `db/chirpstack.dump` in the tarball. External mode: only `db/shifter.dump`; ChirpStack backup is operator's responsibility (documented in runbook). Operators on bundled mode get one-tarball recovery for the whole stack.
- **D-42:** **Backup destination in v1 = local filesystem path only.** Configurable via env var (`SHIFTER_BACKUP_DIR`) and Settings → Backup. Compose ships a volume mount at `/var/lib/shifter/backups/` (bundled + external). Operator can mount the volume at any path (NFS, host directory, encrypted FS). S3 / S3-compatible destination ships in v1.x per V2-OPS-01. Reduces v1 dependency surface (no AWS SDK). OPS-03 "Backup destination is configurable" honored by path-is-configurability.
- **D-43:** **Three backup triggers, one code path.** (a) `shifter backup --to /path/to/dest` CLI subcommand for operator-owned cron. (b) Bundled compose ships **`mcuadros/ofelia:v0.3.22` cron sidecar (research-resolved)** — single-binary Go scheduler with `--docker` mode + label-driven configuration (`ofelia.job-exec.shifter-backup.schedule = "0 2 * * *"` on the shifter service). Runs `shifter backup` nightly at 02:00 install_tz into the default local path. (c) Settings → Backup → "Run backup now" button calls the same CLI internally and polls status. All three write an audit row + manifest. Idiomatic Unix; lowest scope. Settings UI also exposes a "Schedule" docs link explaining how to adjust the ofelia labels. Source: https://github.com/mcuadros/ofelia.
- **D-44:** **Restore = `shifter restore --from <tarball>` CLI; requires Shifter down.** Operator: `docker compose stop shifter` → `docker compose run --rm shifter restore --from /var/lib/shifter/backups/<file>.tar.gz` → `docker compose start shifter`. Restore CLI: extracts tar, acquires PG advisory lock (refuses if Shifter is still serving — defends against double-write), runs `pg_restore` against both DBs (bundled) or just Shifter (external), rsyncs floor-plan dir, verifies manifest sha256, reports success/failure. ChirpStack data restored alongside Shifter data in bundled mode. Restore invocation uses `pg_restore --no-owner --no-acl` (mirrors D-40 backup flags) so the restore is portable across hosts where role names may differ.
- **D-45:** **OPS-04 CI round-trip = same-version seed → backup → fresh DB → restore → smoke (Phase 6); cross-version variant deferred to v1.1 (research-resolved Open Question #1).** GitHub Actions job: spin up Postgres+TimescaleDB testcontainers + ChirpStack stub, run `shifter migrate up`, seed test fixture (1 site, 1 MP, 1 device, 100 measurements, 1 floor plan, 5 audit rows), run `shifter backup` to tmp tarball, drop+recreate both DBs, run `shifter restore`, run smoke test: 1 SELECT per table verifies row counts, 1 HTTP GET `/api/sites/:id` verifies app boots and serves restored data, 1 `SELECT * FROM cagg_daily LIMIT 1` verifies TimescaleDB CAGG state survived. Fail CI on any step. **Cross-version restore (vN backup → vN+1 schema via `migrate up` then restore) defers to v1.1** — it requires release-management coupling (tagged release artifacts available at CI time) that v1.0 doesn't yet have. The manifest's `db_schema_version` field (D-39) is the forward-compat hook v1.1 will consume.

### Settings — Backup status + Identity propagation (SETT-02, SETT-05)

- **D-46:** **Settings → Backup card (SETT-05).** Displays: "Last backup: 14 hours ago • destination: `/var/lib/shifter/backups/`" with green/yellow/red age dot. Thresholds: green ≤ `warn_threshold_hours` (default 24 h), yellow ≤ `crit_threshold_hours` (default 168 h = 7 d), red beyond. Both thresholds operator-configurable in the same card. "Run backup now" button (D-43 trigger c). Most-recent-5 backups list: filename, size, status, started/finished ts, sha256 (collapsible). "Configure schedule" link to operator runbook section. Tooltip on dot explains the threshold.
- **D-47:** **SETT-02 install-identity propagation = new reports get new branding; old artifacts immutable.** Identity changes (`display_name`, `logo_path`, `address`) apply to reports generated AFTER the change. Cached PDFs / CSVs already on disk under their 24 h TTL (Phase 5 D-07) retain old branding. Aligns with "reports are ephemeral" — no PDF rewriting, no batch job. Install Identity card in Settings shows a note: "Changes apply to future reports." Matches typical product behavior and explains itself in-place.

### Operational Hardening polish (OPS-05/06/07/08)

- **D-48:** **OPS-05/06/07 verification pass + close any gaps.** Phase 1 already shipped json-file logging caps (Plan 01-20), Compose `secrets:` (Plan 01-04 + 01-20), pinned image tags (Plan 01-20 + 01-21). Phase 6 Plan: (a) audit BOTH `compose/bundled.yml` AND `compose/external.yml` line-by-line — every service has the `<<: *json-logging` anchor; no `:latest`; no `.env`-mounted credentials. (b) Add any missing entries. (c) Document the canonical compose conventions in `docs/operator-runbook.md` so future Compose changes don't drift. (d) Verify any Phase 5 service additions (River-related, floor-plan volume, etc.) inherit the conventions.
- **D-49:** **OPS-08 per-release upgrade runbook.** New section in `docs/operator-runbook.md`: "Upgrading Shifter". Five steps: (1) `shifter backup` (D-43 CLI), (2) edit `compose/{bundled,external}.yml` to bump `shifter:0.X.0` tag, (3) `docker compose pull && docker compose up -d shifter` (zero-downtime for read traffic; ingest pause window ≤ 5 s), (4) verify `/health` returns new version + `/health/detailed` shows alert_worker, last-backup, all green, (5) rollback = `docker compose stop shifter && shifter restore --from <pre-upgrade-backup> && revert tag in compose && docker compose up -d shifter`. Per-release breaking-migration notes are appended by the release process. No automated upgrade CLI in v1 (operator-driven Compose tag bump is the documented path).
- **D-50:** **`shifter doctor` CLI for support diagnostic snapshots — no Docker socket dep in v1 (research-resolved Open Question #3).** New CLI subcommand outputs a redacted diagnostic bundle to stdout or file: `config-check` results, `/health/detailed` JSON (DB + ChirpStack + MQTT + disk + last-uplink-age + alert_worker + last-backup), last 100 audit_log rows (PII redacted: emails masked to `j***@example.com`), last alert worker run summary, schema version, ChirpStack gRPC ping result. **Container-log tail deferred to v1.x** — pulling `docker/docker` (~15 MB dep, large transitive graph) into the v1 binary just for `docker logs` access isn't worth it; doctor output instead instructs operators to attach `docker compose logs --since 1h shifter` themselves. Operator emails the bundle when filing a support ticket. Builds on Phase 1 `config-check` and `healthcheck` patterns; one redaction layer.

### Claude's Discretion

- **Audit-vocabulary migration numbering** — planner picks the next sequential migration number(s) for the auth-event + user-mgmt + alert-rule + alert + backup-related vocabulary additions. Existing pattern: each phase adds one or more `*_audit_vocab_*.up.sql` migrations.
- **Alert worker River queue name, schedule cron expressions, cool-down enforcement layer** (in-engine vs in-DB row exclusion) — planner picks based on River v0.13 idioms. Default cool-down 900 s is the constant.
- **Alert payload field names** (D-12 schema) — planner picks idiomatic snake_case (`metering_point_id` not `meteringPointId`); stable shape is the constraint, not field-name aesthetics.
- **JsonTree integration into audit table rows (D-34)** — planner picks whether to virtualize the expanded panel (probably yes for large `before`/`after` objects).
- **Cold-start chip exact copy + progress-bar styling (D-16)** — planner / UI follows Phase 4 D-21 three-stage empty-state card pattern.
- **Sidebar nav reordering** (adding `/alerts` and `/audit` and Settings → Users / Backup tabs) — planner picks the final order; suggested top-to-bottom: Dashboard / Map / Sites / Devices / Gateways / Reports / Alerts / Audit / Settings.
- **CLI prompt copy** for `create-admin --reset` and `restore --from` confirmations — planner picks (single source of truth in `docs/operator-runbook.md`).
- **Random password alphabet for D-23** — planner picks (suggested: full printable ASCII excluding ambiguous chars `1lI0O`). Length ≥ 16 to pass any conceivable strength evaluator threshold.

### Folded Todos

No todos were folded into Phase 6 scope (`gsd-tools todo match-phase 6` returned `todo_count: 0` at session time).

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase scope & requirements
- `.planning/PROJECT.md` — Core value, single-tenant-per-install posture, local-auth + no-SMTP constraint, "no SSO / OAuth in v1", "no SMTP in v1", Out-of-Scope list (multi-tenant SaaS, native mobile, i18n, SSO/OAuth, SMTP, paid maps, in-app CS version upgrade)
- `.planning/REQUIREMENTS.md` §ALERT-01..06, §USER-01..04, §AUDIT-01..03, §SETT-01..05, §OPS-01..08, §AUTH-03 — full requirement text and v2 deferrals (V2-NOTIF-01/02/03, V2-AUTH-01/02/03, V2-OPS-01)
- `.planning/ROADMAP.md` §Phase 6 — success criteria (5 verbatim items) + research flag for backup/restore ordering and cross-version restore

### Prior phase decisions to honor
- `.planning/phases/01-foundation/01-CONTEXT.md` — D-09 (AUTH-03 reframed: applies to admin-created secondary users = USER-04 territory), D-14 (`shifter create-admin --reset` lockout escape hatch — unchanged by D-26 self-edit blocks), D-18/D-19 (`/health` public + `/health/detailed` admin-only — Phase 6 adds `alert_worker` + `last_backup` rows), D-22/D-23 (cookie Secure + SCS sessions in Postgres — basis for D-24 session-revoke), D-24 (json-file logging caps — Phase 6 D-48 verifies), `internal/auth/password_strength.go` (reused per D-28), Plan 01-09/01-11 handlers (Phase 6 retrofits per D-30)
- `.planning/phases/02-domain-model-canonical-schema/02-CONTEXT.md` — D-06 (audit_log is regular table, not hypertable — fits Phase 6 D-38 retention model), D-20 (no-hard-delete pattern locked for USER mgmt — basis for D-27), D-21 (auth-event audit explicitly deferred to Phase 6 — Phase 6 D-30 closes), D-22 (audit_log schema columns + vocabulary CHECK constraints — Phase 6 D-30 adds new vocabulary)
- `.planning/phases/04-realtime-dashboard/04-CONTEXT.md` — D-07 KPI flicker threshold = `2× expected_interval_s` (intentionally **looser** than Phase 6 D-15 ALERT-02 paging threshold), D-14 URL-state pattern via `useSearchParams + zod` (Phase 6 D-32 reuses for audit filters), D-15 cursor-pagination uplinks log (Phase 6 D-36 mirrors for audit), D-19 recursive `JsonTree` component (Phase 6 D-34 reuses for audit row diff), D-21 three-stage empty-state card pattern (Phase 6 D-16 cold-start chip + alert center empty states follow)
- `.planning/phases/05-aggregates-reports-map-floor-plans/05-CONTEXT.md` — D-07 reports-are-ephemeral + 24 h artifact TTL (Phase 6 D-47 SETT-02 propagation), D-09 retention defaults + Settings → Data Retention card pattern (Phase 6 D-13 alerts row + D-38 audit_log row), retention reconciliation flow in `internal/settings/retention.go`, River v0.13 install + Postgres-native job queue (Phase 6 D-01 alert worker reuses), REPT-03 CSV format spec (Phase 6 D-35 audit CSV mirrors), Phase 5 D-25 decommission-cascade audit pattern (Phase 6 D-30 mirrors for auth-event retrofit)

### Existing code anchors (Phase 6 builds on)
- `internal/audit/log.go` — `WriteEntry(ctx, tx, Entry)` is the in-tx audit writer; Phase 6 D-30 adds new action + entity-type constants
- `internal/db/migrations/0016_audit_log.up.sql` — `audit_log` table schema, indexes (`time DESC`, `user_id`, `entity_type`+`entity_id`, `request_id`); Phase 6 D-36 leverages existing `time DESC` index
- `internal/db/migrations/0020_audit_log_vocabulary.up.sql`, `0031_audit_vocab_phase5.up.sql`, `0034_audit_vocab_floor_plan.up.sql`, `0035_audit_vocab_placement.up.sql`, `0036_audit_vocab_retention.up.sql` — vocabulary migration pattern; Phase 6 adds at least one new such migration per D-30
- `internal/auth/users.go` — `Store` user CRUD surface (currently: GetUserByEmail, GetUserByID, AdminExists, InsertAdminUser, UpdatePassword). Phase 6 USER-01..04 extends this with List, Create (non-admin), Update, Disable/Enable, ChangeRole.
- `internal/auth/session.go`, `internal/auth/handlers.go`, `internal/auth/account.go` — SCS session manager + login/logout/change-password handlers. D-24 session-revoke uses SCS DELETE; D-30 retrofits these handlers with audit writes.
- `internal/auth/authz.go` — `Can(user, action, resource)` + `RequireAction` middleware; Phase 6 adds new actions (`alert.read`, `alert.ack`, `alert.snooze`, `alert.rule_create/update/disable`, `audit.read`, `audit.export`, `user.create/update/disable/role_change/reset_password`, `backup.run`, `backup.read`, `settings.identity_update`, `settings.alerts_update`).
- `internal/settings/retention.go`, `internal/settings/routes.go` — Settings → Data Retention card already exists; Phase 6 adds "Alerts" + "Audit log" rows + extends `PATCH /api/settings/retention` reconciliation.
- `internal/events/` — Phase 4 `Hub` fan-out (per-MP topics + per-device-health topics). Phase 6 may add an `alert` topic for alert-center live updates (drawer auto-refresh on new fire), though D-37 explicitly skips live-tail for audit.
- `internal/db/migrations/0029_retention_config.up.sql` — retention config table; Phase 6 adds rows for `alerts` and `audit_log` retention.
- `cmd/shifter/main.go` (+ `internal/cli/`) — Cobra root + subcommands. Phase 6 adds `backup`, `restore`, `doctor` subcommands.
- `compose/bundled.yml`, `compose/external.yml` — Phase 6 D-48 audits + Phase 6 D-43 adds the cron sidecar to bundled.yml.
- `install/bundled/install.sh`, `install/external/install.sh` — install scripts; Phase 6 D-43 documents the cron sidecar in operator runbook.
- `docs/operator-runbook.md` — Phase 1 shipped first version; Phase 6 D-49 appends "Upgrading Shifter" + D-48 appends "Compose conventions" + D-43 appends "Backup schedule cron sidecar".

### TimescaleDB-specific (research-flagged)
- TimescaleDB Backup & Restore docs — https://docs.timescale.com/self-hosted/latest/backup-and-restore/
- `pg_dump` with TimescaleDB — https://docs.timescale.com/self-hosted/latest/backup-and-restore/logical-backup/
- `timescaledb_pre_restore()` / `timescaledb_post_restore()` — https://docs.timescale.com/api/latest/administration/timescaledb_pre_restore/
- Cross-version restore considerations (TimescaleDB 2.x) — researcher confirms 2.26 specifics

### River v0.13 (already installed Phase 5)
- River docs — https://riverqueue.com/docs
- River cron — https://riverqueue.com/docs/cron
- River observability / health surfaces — https://riverqueue.com/docs/observability

### SCS sessions (already installed Phase 1)
- alexedwards/scs/v2 — https://pkg.go.dev/github.com/alexedwards/scs/v2 (DELETE-all-sessions-for-user pattern: iterate the `sessions` table via the pgxstore + `DELETE WHERE data ->> 'user_id' = $1`, with audit_log row in same tx)

### Statistical anomaly references (planner reads if needed)
- P95 trailing-window approach for utility-consumption anomaly — standard AMI practice; FEATURES.md L134 cites it
- IQR outliers in time-series — robust against long-tail distributions when used per time-of-day bucket
- Quiet-hour flow leak signature for water meters — industry-standard leak-detection heuristic; Badger Meter / Kamstrup / Sensus docs reference this pattern

### ROADMAP research flag (MANDATORY — research before planning)
- ROADMAP.md §Research Flags Phase 6: "Backup/Restore: TimescaleDB-aware logical backup ordering relative to bundled-mode ChirpStack DB, cross-version restore (vN backup → vN+1 schema) need a concrete runbook + CI test." → `/gsd-research-phase 6` resolves D-40 / D-44 / D-45 specifics before `/gsd-plan-phase 6`.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets

**Backend (Go):**
- `internal/audit/log.go` — `WriteEntry(ctx, tx, Entry)` already used at 13+ production sites; Phase 6 alert fires, user-mgmt mutations, backup runs, restore runs, retention-config edits all land here in the same transaction. Adding new action/entity-type constants is the standard pattern (one migration per new vocabulary group).
- `internal/auth/users.go` — User store has the narrow surface for Phase 1's needs (Get/InsertAdmin/UpdatePassword); Phase 6 adds List(scope), CreateUser(role), Update, Disable/Enable, ChangeRole. Pattern uses raw SQL on pgxpool (not sqlc) — Phase 6 may move to sqlc for consistency now that the user CRUD surface is more than three queries.
- `internal/auth/session.go`, `internal/auth/account.go`, `internal/auth/handlers.go` — Phase 1 login/logout/change-password + SCS session manager. D-24 "logout everywhere" iterates pgxstore session rows + DELETE. D-30 wraps these handlers with `audit.WriteEntry` in the same tx.
- `internal/auth/authz.go` (`Can`, `RequireAction`) — Already has 20+ actions and the `RequireAction` middleware pattern; Phase 6 adds ~12 new action constants.
- `internal/settings/retention.go` — Settings → Data Retention card already does same-tx reconciliation against TimescaleDB policies; Phase 6 extends with `alerts` and `audit_log` rows (no TimescaleDB policy for those — just an application-level prune cron).
- `internal/events/Hub` — Phase 4 in-process fan-out. Phase 6 may add an `alert` topic for the alert center drawer (optional optimization).
- `github.com/riverqueue/river` + `riverdriver/riverpgxv5` — Phase 5 installed; Phase 6 alert worker + backup worker + audit-export worker reuse the same queue infrastructure.
- `github.com/alexedwards/scs/v2` + pgxstore — Phase 1 install; Phase 6 D-24 leverages the underlying `sessions` table to bulk-revoke by user_id.
- `internal/auth/password_strength.go` — Phase 1 strength evaluator + zxcvbn-like scoring; D-28 reuses for every Phase 6 password surface.
- `internal/install/state.go` — `install_state` table with `chirpstack_mode` flag (`bundled` | `external`); D-41 reads this to know whether to dump ChirpStack DB.
- `internal/cli/` (Cobra subcommands) — Phase 1 ships `serve`, `migrate`, `version`, `create-admin`, `config-check`, `healthcheck`. Phase 6 adds `backup`, `restore`, `doctor`.

**Frontend (React/TS):**
- `web/src/routes/_root.tsx` + `web/src/components/shell/sidebar.tsx` — App shell + sidebar nav. Phase 6 adds bell icon in header, `/alerts` + `/audit` sidebar items, role-conditional visibility.
- `web/src/components/responsive-dialog.tsx` (Phase 1) — All Add Rule / Add User / Confirm Logout-Everywhere / Confirm Restore dialogs use this.
- `web/src/components/AlertDialog` (shadcn) — D-24 logout-everywhere confirmation, D-26 self-edit blocks, D-27 re-enable confirmation, D-44 restore-needs-shifter-down warning.
- `web/src/components/json-tree.tsx` (Phase 4 D-19) — D-34 audit row diff reuses with `before` / `after` side-by-side mode.
- `@tanstack/react-query` + `@tanstack/react-table` + `@tanstack/react-virtual` — already in `package.json`; alerts page, audit page, users page, backup history all use these.
- `react-router-dom@7 useSearchParams + zod` — D-32 audit filter chips reuse the Phase 3 D-15 / Phase 4 D-14 URL-state pattern.
- `lucide-react` icons — `Bell` (alert center bell), `Shield` (audit), `Users` (users page), `Database` (backup), `Stethoscope` or `Activity` (doctor / health), `AlertTriangle` (severity warning), `XCircle` (severity critical).
- `sonner` toast — backup completion, restore start/end, alert auto-clear notifications.
- `react-hook-form` + `@hookform/resolvers` + `zod` — alert rule form, user create/edit form, restore confirmation.

### Established Patterns

- All CRUD lives in `ResponsiveDialog` (UX-01). Phase 6 alert-rule editor, add-user, edit-user, reset-password, change-role, run-backup-now, restore-from-tarball all follow.
- `shadcn/ui` blue/navy palette (OKLCH custom theme); English-only copy (UX-02). Severity tints: slate (info) / yellow (warning) / red (critical).
- URL-state via `useSearchParams + zod` (Phase 3 D-15 / Phase 4 D-14 / Phase 5 D-05) — Phase 6 alert center page and audit browse page both deep-link.
- LISTEN/NOTIFY → in-process Hub → SSE for live UI updates (Phase 4 D-01/D-02/D-03) — Phase 6 optional `alert` topic for drawer auto-refresh; explicitly skipped for audit per D-37.
- Atomic CS+PG transactions with audit-in-tx (Phase 2/3 pattern) — applies to every alert fire, user-mgmt mutation, backup-run, restore-run, retention-config change in Phase 6.
- Migrations as SQL files, sqlc for queries (Phase 1 D-13/D-16). New migrations expected: `0037_alert_rule.up.sql`, `0038_alert.up.sql`, `0039_audit_vocab_phase6.up.sql`, `0040_retention_config_phase6.up.sql` (planner picks exact numbering; check `internal/db/migrations/` before assigning).
- Two-flavor compose (bundled/external); volume mounts for persistent state (Phase 1 OPS-01, Phase 5 floor_plans volume) — Phase 6 adds `backups` volume + (bundled-only) cron sidecar.
- React Query for server state, React Hook Form + zod for forms, `sonner` for toasts — uniform across phases.

### Integration Points

- `internal/http/router.go` — new route groups: `/api/alerts/*` (list, ack, snooze, clear), `/api/alerts/rules/*` (CRUD), `/api/users/*` (admin CRUD), `/api/audit/*` (list, export), `/api/backup/*` (run, list, status), `/api/health/detailed` (extended).
- `internal/events/Hub` — optional `alert` topic for alert-drawer live updates (planner decides whether to add or rely on react-query polling).
- `compose/bundled.yml` — add `backup-cron` sidecar service (smallest pinned image with `docker exec` capability) + `backups` named volume mounted at `/var/lib/shifter/backups` (consistent with Phase 5 floor-plans volume pattern).
- `compose/external.yml` — add `backups` volume only; no cron sidecar by default (external-mode operators are presumed to have their own cron infrastructure; runbook documents how to add one).
- `web/src/routes/` — new tree: `alerts/`, `audit/`; extend `settings.tsx` with Users tab + Backup tab; add `bell.tsx` slide-over drawer component to app shell.
- `web/src/components/shell/sidebar.tsx` — new nav items: "Alerts" (with unread badge), "Audit" (admin-only); reorder above Settings.
- `internal/install/finish.go` — extend to seed default retention rows for `alerts` (365 d) and `audit_log` (5 y) at install time.
- `docs/operator-runbook.md` — three new sections: "Backup & Restore" (D-43..D-45), "Upgrading Shifter" (D-49), "Compose conventions" (D-48).
- `.github/workflows/*.yml` — new CI job for D-45 round-trip backup/restore test; runs on PR + main.

</code_context>

<specifics>
## Specific Ideas

- "Threshold + offline + anomaly are three separate engines but live in one River queue — uniform observability and retry, no infrastructure fan-out."
- "Cold-start gate is visible behavior, not a hidden bug. The MP detail page shows 'Warming up — 16 days remaining' so the operator can explain it to the customer."
- "Header bell + drawer + dedicated page — the GitHub/Linear/Slack pattern. Drawer keeps task context, page is for deep review."
- "Auto-clear when condition resolves + audit row. Operator doesn't have to manually clear every transient blip; the audit log preserves the history."
- "Logout-everywhere is the same DELETE regardless of trigger — explicit button, disable-user, role-change, reset-password. One code path, four callers."
- "Random + show-once initial passwords. Operator never types passwords; the install context (no SMTP) means we can't email them either, so 'Share these credentials' panel is the moment of truth."
- "Block self-demote / self-disable / last-admin-demote at the server. UI greys them out for clarity, but the server is the lock. `shifter create-admin --reset` stays as the lockout escape hatch."
- "Audit is a forensic surface. No live-tail SSE. Manual refresh + relative timestamps. Compliance reviewers want a snapshot, not a flickering feed."
- "Single tar.gz backup. One artifact per backup. Easy to copy, easy to upload to anywhere later. Manifest pins the version and schema."
- "Bundled mode backup includes ChirpStack DB. External mode is Shifter-only. Detected via `install_state.chirpstack_mode`. Operator doesn't have to think about it."
- "Restore requires Shifter down. PG advisory lock refuses if Shifter is still serving. Operator: stop, restore, start. Boring and reliable."
- "OPS-04 CI round-trip test is the gate. Seed → backup → fresh DB → restore → smoke. Fails CI on any drift."
- "Identity changes apply to future reports. No PDF rewriting. Aligns with 'reports are ephemeral' from Phase 5 D-07."
- "`shifter doctor` is the support-ticket payload. Redacted diagnostic bundle. Operator emails it; we triage faster."

</specifics>

<deferred>
## Deferred Ideas

- **S3 / S3-compatible backup destination** — V2-OPS-01. v1 ships local-path; the tarball format is upload-ready. Adding S3 in v1.x means `aws-sdk-go-v2` + Settings UI + IAM/credential UX — not in v1 scope but unblocked by Phase 6 design.
- **Scheduled backup UI inside Shifter** — current model is operator-managed cron (bundled-mode sidecar, external-mode operator's own cron). A "Schedule" panel inside Settings → Backup is v1.x once customer feedback names it.
- **Hot restore / zero-downtime restore** — single-tenant install posture makes "stop, restore, start" acceptable. v2 if a customer SLO requires zero-downtime DR.
- **Email / SMTP alert delivery** — V2-NOTIF-02; blocked by PROJECT.md "no SMTP in v1". Alert payload (D-12) is already structured to plug in a deliverer.
- **Webhook alert delivery (Slack / Discord / n8n / Home Assistant / ERP)** — V2-INT-01. D-12 payload is the body; v1.x adds a deliverer + Settings → Integrations UI.
- **Per-user dashboards / saved views / per-rule subscribers** — V2-AUTH-02. v1 is shared-inbox; D-11 explicitly defers this.
- **SSO / OAuth / SCIM** — V2-AUTH-01. Local-only auth in v1.
- **Custom roles beyond admin / viewer** — V2-AUTH-03. Two-role authz from Phase 1 is the v1 surface.
- **Engine dry-run (rule fires-against-last-30-days preview)** — useful for anomaly threshold tuning; Phase 7 territory where empirical tuning happens.
- **Live-tail SSE for audit browse** — D-37 explicitly defers; v2 if incident-response use cases demand it.
- **Audit log full-text search** — D-32's free-text Request ID chip + filter chips suffice for v1. v1.x if compliance reviewers ask for "find all rows mentioning 'firmware'".
- **Cross-version restore CI test** — research flag may pull this into Phase 6 scope; if it's complex it can defer to a Phase 6.5 / v1.x gate before public release.
- **Saved alert rule templates** — "Battery low default", "High flow during quiet hours default". v1.x if operators want one-click rule libraries.
- **Audit retention as a TimescaleDB hypertable with compression** — Phase 2 D-06 explicitly says audit_log is a regular table at v1 volume. v1.x if audit_log grows beyond ~10 M rows.
- **Per-rule cool-down dry-run** — see "engine dry-run" above; v2.
- **Multi-channel alert routing** (email + webhook + Slack + in-app) — V2-NOTIF-*; v1 is in-app only.
- **Bulk user import / SSO group sync** — V2-AUTH-01 + bulk import; v2.
- **Encrypted backups at rest** — operator's responsibility via filesystem encryption (LUKS, ZFS, encrypted volume); v1.x if customer requires app-layer encryption.
- **Backup retention / rotation inside Shifter** — operator manages disk; documented in runbook. v1.x for "keep last N backups, delete older".

### Reviewed Todos (not folded)

None — `gsd-tools todo match-phase 6` returned `todo_count: 0` at session time.

</deferred>

---

*Phase: 06-alerts-users-audit-operational-hardening*
*Context gathered: 2026-05-12*
