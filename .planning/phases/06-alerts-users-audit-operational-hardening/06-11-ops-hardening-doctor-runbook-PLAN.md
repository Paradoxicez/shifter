---
phase: 06-alerts-users-audit-operational-hardening
plan: 11
type: execute
wave: 3
depends_on: [06-01, 06-08, 06-09]
files_modified:
  - compose/bundled.yml
  - compose/external.yml
  - docs/operator-runbook.md
  - internal/doctor/doctor.go
  - internal/doctor/doctor_test.go
  - internal/doctor/redact.go
  - internal/doctor/redact_test.go
  - internal/cli/doctor.go
  - internal/cli/doctor_test.go
  - internal/cli/root.go
  - internal/alert/alerts_prune_worker.go
  - internal/alert/alerts_prune_worker_test.go
  - internal/db/migrations/0047_audit_vocab_alert_prune.up.sql
  - internal/db/migrations/0047_audit_vocab_alert_prune.down.sql
  - internal/cli/serve.go
  - internal/http/health.go
  - internal/http/health_test.go
  - internal/compose/conventions_test.go
autonomous: true
requirements: [OPS-05, OPS-06, OPS-07, OPS-08]
must_haves:
  truths:
    - "compose/bundled.yml + compose/external.yml audited line-by-line: every service has `<<: *json-logging` anchor; no `:latest` tag anywhere; every credential mounted via Compose `secrets:` (not `.env`); no `${SHIFTER_DB_PASSWORD}` env-var leakage of secret values (D-48)"
    - "docs/operator-runbook.md gets 3 new sections: 'Compose conventions' (canonical patterns) + 'Upgrading Shifter' (5-step + rollback per D-49) + Backup & Restore already added in Plan 06-09"
    - "`shifter doctor` CLI emits a redacted diagnostic bundle to stdout or --out file; bundle includes: config-check, /health/detailed, last 100 audit rows (emails masked), alert worker summaries, schema version, ChirpStack gRPC ping. NO Docker socket log tail in v1 (per RESEARCH Open Question #3 deferral)"
    - "Email masking: `MaskEmail('john.doe@x.com') => 'j***@x.com'`; regex pass over serialized JSON before output"
    - "/health/detailed extended with `alert_worker` array (one row per worker_kind: kind, last_run_at, rules_evaluated, fires_emitted, cleared, duration_ms, degraded, last_error) + `last_backup` summary (filename, started_at, age_seconds, status, sha256) (D-21, D-50)"
    - "alerts retention prune worker (River cron 03:30 install_tz) calls admin_prune_alerts SECURITY DEFINER function (mirrors audit prune pattern); consumes retention_config.alerts_days (Plan 06-10)"
    - "Compose conventions enforced by an automated test: internal/compose/conventions_test.go greps both compose files and asserts properties (no :latest, every service has json-logging anchor, secrets mounted not env)"
  artifacts:
    - path: docs/operator-runbook.md
      provides: "Compose conventions + Upgrading Shifter + (Backup & Restore from Plan 06-09)"
    - path: internal/doctor/doctor.go
      provides: "Doctor.SnapshotBundle producing the diagnostic JSON"
    - path: internal/cli/doctor.go
      provides: "shifter doctor subcommand with --out flag"
    - path: internal/alert/alerts_prune_worker.go
      provides: "AlertsPruneWorker daily 03:30 cron"
    - path: internal/compose/conventions_test.go
      provides: "Automated lint: every service has json-logging, no :latest, no .env credentials"
  key_links:
    - from: internal/cli/doctor.go
      to: internal/doctor/doctor.go::SnapshotBundle
      via: "CLI invokes Doctor.SnapshotBundle(ctx) and writes JSON to stdout/file"
      pattern: "SnapshotBundle"
    - from: internal/alert/alerts_prune_worker.go
      to: internal/db/migrations/0047_audit_vocab_alert_prune.up.sql
      via: "Worker calls SELECT admin_prune_alerts(cutoff_days); SECURITY DEFINER function"
      pattern: "admin_prune_alerts"
---

<objective>
Final phase plan: close OPS-05/06/07/08 with a compose audit pass + lint test + 3 runbook sections; ship `shifter doctor` support-diagnostic CLI; add the missing alerts retention prune worker (companion to Plan 06-01's audit prune); extend /health/detailed with alert_worker + last_backup rows.

Purpose: every operational lever the operator needs — Compose hygiene, upgrade procedure, support bundle, alert observability — lives in this plan. After Plan 06-11 lands, Phase 6 is shippable.

Output: compose audit + lint test + 3 runbook sections + Doctor package + Cobra subcommand + alerts retention prune worker + /health/detailed extension.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/06-alerts-users-audit-operational-hardening/06-CONTEXT.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-UI-SPEC.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-01-alert-engine-substrate-PLAN.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-08-backup-cli-cron-PLAN.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-09-restore-cli-ci-roundtrip-PLAN.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-10-settings-extensions-PLAN.md
@compose/bundled.yml
@compose/external.yml
@docs/operator-runbook.md
@internal/cli/configcheck.go
@internal/cli/healthcheck.go
@internal/cli/serve.go
@internal/http/health.go
@internal/audit/log.go
@internal/install/state.go

<interfaces>
Plan 06-01 added internal/alert/audit_prune_worker.go pattern.
Plan 06-08 added /api/backup/last endpoint returning the last backup_run summary.
Plan 06-10 added retention_config.alerts_days + backup_warn_threshold_hours + backup_crit_threshold_hours.

Existing Phase 1 /health/detailed handler (internal/http/health.go OR internal/cli/healthcheck.go) returns JSON with sections: status, version, uptime, db (connection ok), chirpstack (gRPC reachable), mqtt (subscriber state), disk (free bytes). Plan 06-11 extends with: alert_worker[] array, last_backup object.

Existing `shifter config-check` (internal/cli/configcheck.go) probes env config + DB + ChirpStack gRPC reachability + returns JSON or text.

Plan 06-08 added pg client binaries in the Docker image — doctor CLI can rely on those for pg_isready probe if desired.

D-50 (RESEARCH Open Question #3): NO Docker socket / no log-tail in v1; document fallback in the doctor bundle output.
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Alerts retention prune worker + admin_prune_alerts SECURITY DEFINER function + /health/detailed extension</name>
  <files>internal/db/migrations/0047_audit_vocab_alert_prune.up.sql, internal/db/migrations/0047_audit_vocab_alert_prune.down.sql, internal/alert/alerts_prune_worker.go, internal/alert/alerts_prune_worker_test.go, internal/cli/serve.go, internal/http/health.go, internal/http/health_test.go</files>
  <read_first>
    - internal/db/migrations/0043_admin_prune_audit_rows.up.sql (Plan 06-01: mirror this SECURITY DEFINER pattern for alerts)
    - internal/alert/audit_prune_worker.go (Plan 06-01: mirror the worker pattern)
    - internal/http/health.go (current /health/detailed JSON shape)
    - internal/db/migrations/0042_alert_worker_state.up.sql (Plan 06-01: 5 worker_kind rows)
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-CONTEXT.md §D-13 (alerts retention = 365 days default)
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md §Decision C "Alert Worker Observability Row (D-21)"
  </read_first>
  <behavior>
    - Test (TestAdminPruneAlerts_DeletesOldRows): seed 10 alert rows with fired_at = now() - 400 days; alerts_days config = 365 → SELECT admin_prune_alerts(365) returns 10; 10 rows deleted; meta row 'alert.cleared' OR no meta row (alerts table is not audit_log; no special trigger) — actually alerts table has no DELETE trigger, so the function is just a privileged DELETE for symmetry with audit prune. SIMPLIFY: skip SECURITY DEFINER for alerts; use a plain DELETE inside the worker. Re-read CONTEXT.md... no special INSERT-ONLY trigger on alert table → plain DELETE suffices. **REVISE this task: NO admin_prune_alerts function; just a worker that does `DELETE FROM alert WHERE fired_at < now() - alerts_days`.**
    - Test (TestAlertsPruneWorker_PrunesPerRetention): seed retention_config.alerts_days = 365; seed 20 alerts (10 fresh + 10 older than 365 days) → worker run deletes the 10 old, leaves the 10 fresh.
    - Test (TestAlertsPruneWorker_Idempotent): running the worker twice with no new old rows deletes 0 rows.
    - Test (TestAlertsPruneWorker_NeverPrunesFiring): an alert in state='firing' older than retention is STILL pruned (D-13 is a hard cap; firing-but-stale alerts are still cleared by this prune).
    - Test (TestAlertsPruneWorker_AuditRow): each run writes an audit row 'alert.cleared' (or a new 'alert.pruned' action — but vocabulary in Plan 06-01 doesn't include alert.pruned; reuse 'alert.cleared' OR add 'alert.pruned' here as a new vocabulary addition migration). **DECISION: add migration 0047_audit_vocab_alert_prune.up.sql adding 'alert.pruned' action. Cleaner than reusing alert.cleared which has different semantics (clear = condition resolved, not retention-aged-out).**
    - Test (TestHealthDetailed_AlertWorkerArray): GET /health/detailed (admin) → response.alert_worker is an array of length 5, one entry per worker_kind, each with kind/last_run_at/rules_evaluated/fires_emitted/cleared/duration_ms/degraded/last_error.
    - Test (TestHealthDetailed_LastBackup): response.last_backup has filename, started_at, age_seconds, status, sha256. Never-run case → last_backup is null.
    - Test (TestHealthDetailed_Status_DegradedIfWorkerDegraded): if any alert_worker_state.degraded=true → overall status="degraded".
    - Test (TestHealthDetailed_Status_DegradedIfBackupTooOld): if last backup age > backup_crit_threshold_hours → overall status="degraded".
  </behavior>
  <action>
    **Migration coordination:** Plan 06-11 claims migration `0047` (Plan 06-01 owns 0037-0043, Plan 06-05 owns 0044, Plan 06-08 owns 0045, Plan 06-10 owns 0046).

    Decision: the alerts table has no INSERT-ONLY trigger, so a plain DELETE inside the worker suffices — NO `admin_prune_alerts` SECURITY DEFINER function. We DO add a new vocabulary migration `0047_audit_vocab_alert_prune.up.sql` adding the `'alert.pruned'` action since the prune writes a meta audit row.

    **Migration 0047_audit_vocab_alert_prune.up.sql:**
    ```sql
    ALTER TABLE audit_log DROP CONSTRAINT audit_log_action_valid;
    ALTER TABLE audit_log ADD CONSTRAINT audit_log_action_valid CHECK (action IN (
      -- ... (all 50 actions from previous migrations) ...
      'alert.pruned'
    ));
    ```
    The DOWN migration recreates the previous Plan 06-01 0037 CHECK without 'alert.pruned'. Carry every action from 0037 + extend to keep the DROP/ADD pattern.

    Append to `internal/audit/log.go`:
    ```go
    // Plan 06-11: alerts retention prune
    const ActionAlertPruned = "alert.pruned"
    ```

    **internal/alert/alerts_prune_worker.go:**
    ```go
    type AlertsPruneArgs struct{}
    func (AlertsPruneArgs) Kind() string { return "alerts_prune" }
    func (AlertsPruneArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{MaxAttempts: 3} }

    type AlertsPruneWorker struct {
        river.WorkerDefaults[AlertsPruneArgs]
        Pool *pgxpool.Pool
        Log  *slog.Logger
    }

    func (w *AlertsPruneWorker) Work(ctx context.Context, _ *river.Job[AlertsPruneArgs]) error {
        // 1. Read retention_config.alerts_days
        var days int32
        if err := w.Pool.QueryRow(ctx, `SELECT alerts_days FROM retention_config WHERE id = 1`).Scan(&days); err != nil {
            return err
        }
        // 2. Plain DELETE inside a tx (no INSERT-ONLY trigger on alert table)
        tx, err := w.Pool.BeginTx(ctx, pgx.TxOptions{})
        if err != nil { return err }
        defer tx.Rollback(ctx)

        var deleted int
        if err := tx.QueryRow(ctx,
            `WITH del AS (
                DELETE FROM alert WHERE fired_at < now() - make_interval(days => $1::int) RETURNING id
            ) SELECT count(*) FROM del`, days).Scan(&deleted); err != nil {
            return err
        }
        // 3. Audit meta-row 'alert.pruned' — use gen_random_uuid() for the
        //    self-referential entity_id, matching the audit_prune_worker pattern
        //    from Plan 06-01 (D-51). Each prune cycle gets a unique meta-id so
        //    successive prunes are independently queryable in audit history.
        var metaID uuid.UUID
        if err := tx.QueryRow(ctx, `SELECT gen_random_uuid()`).Scan(&metaID); err != nil {
            return fmt.Errorf("gen meta id: %w", err)
        }
        if err := audit.WriteEntry(ctx, tx, audit.Entry{
            Action:     audit.ActionAlertPruned,        // Plan 06-11 const "alert.pruned"
            EntityType: audit.EntityTypeAlert,          // Plan 06-01 const "alert"
            EntityID:   metaID,                          // meta-id, self-referential
            Notes:      fmt.Sprintf("Pruned %d alerts older than %d days", deleted, days),
        }); err != nil { return err }
        if err := tx.Commit(ctx); err != nil { return err }
        w.Log.Info("alerts_prune.cycle", "deleted", deleted, "cutoff_days", days)
        return nil
    }
    ```

    **internal/cli/serve.go:** Register the new worker + cron schedule (03:30 install_tz daily — 30 minutes after audit prune):
    ```go
    river.AddWorker(riverWorkers, &alert.AlertsPruneWorker{Pool: pool, Log: log})
    periodicJobs = append(periodicJobs, mustParseSchedule("CRON_TZ="+installTZ+" 30 3 * * *", alert.AlertsPruneArgs{}))
    ```

    **internal/http/health.go:** Extend /health/detailed JSON shape:
    ```go
    type DetailedHealth struct {
        // ... existing fields ...
        AlertWorkers []AlertWorkerHealth `json:"alert_workers"`
        LastBackup   *LastBackupHealth   `json:"last_backup"`
    }
    type AlertWorkerHealth struct {
        Kind            string    `json:"kind"`
        LastRunAt       time.Time `json:"last_run_at"`
        RulesEvaluated  int       `json:"rules_evaluated"`
        FiresEmitted    int       `json:"fires_emitted"`
        Cleared         int       `json:"cleared"`
        DurationMs      int       `json:"duration_ms"`
        Degraded        bool      `json:"degraded"`
        LastError       string    `json:"last_error,omitempty"`
    }
    type LastBackupHealth struct {
        FileName    string    `json:"file_name"`
        StartedAt   time.Time `json:"started_at"`
        AgeSeconds  int64     `json:"age_seconds"`
        Status      string    `json:"status"`
        SHA256      string    `json:"sha256"`
    }
    ```

    Overall status: "ok" if all subsystem checks pass + no alert_worker_state.degraded + last_backup.age_seconds < crit_threshold; "degraded" otherwise.
  </action>
  <verify>
    <automated>go test ./internal/alert/... ./internal/http/... ./internal/db/... -run "TestAlertsPruneWorker|TestHealthDetailed|TestMigration0047" -count=1 -timeout=60s</automated>
  </verify>
  <acceptance_criteria>
    - `internal/db/migrations/0047_audit_vocab_alert_prune.up.sql` contains `'alert.pruned'` in the CHECK constraint
    - `internal/audit/log.go` declares `ActionAlertPruned = "alert.pruned"`
    - `internal/alert/alerts_prune_worker.go` contains `DELETE FROM alert WHERE fired_at < now() - make_interval(days =>`
    - `internal/alert/alerts_prune_worker.go` uses `audit.WriteEntry` with `audit.ActionAlertPruned` AND `gen_random_uuid()` for the meta-row entity_id (matches Plan 06-01 audit_prune pattern): `grep "gen_random_uuid" internal/alert/alerts_prune_worker.go` returns ≥ 1 line AND `grep "audit.WriteEntry" internal/alert/alerts_prune_worker.go` returns ≥ 1 line
    - `internal/alert/alerts_prune_worker.go` does NOT use `uuid.Nil`: `grep "uuid.Nil" internal/alert/alerts_prune_worker.go` returns 0
    - `internal/cli/serve.go` registers `&alert.AlertsPruneWorker{` AND has a periodic job with cron `30 3 * * *` (03:30 install_tz)
    - `internal/http/health.go` declares `AlertWorkers []AlertWorkerHealth` AND `LastBackup *LastBackupHealth` fields
    - `internal/http/health.go` overall status = "degraded" when any alert_worker_state.degraded=true OR last_backup.age_seconds > crit_threshold (grep both conditions)
    - All 9 listed tests pass: `go test ./internal/alert/... ./internal/http/... -count=1` exits 0
  </acceptance_criteria>
  <done>Alerts retention has its own prune worker mirroring audit retention; /health/detailed exposes alert worker observability + last backup status powering the degraded banner (Plan 06-04).</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: `shifter doctor` CLI + redaction + diagnostic bundle</name>
  <files>internal/doctor/doctor.go, internal/doctor/doctor_test.go, internal/doctor/redact.go, internal/doctor/redact_test.go, internal/cli/doctor.go, internal/cli/doctor_test.go, internal/cli/root.go, internal/auth/authz.go</files>
  <read_first>
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md §Decision H "Bundle Shape" + "PII Redaction" + "Docker Socket Log Tail" deferral note
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-CONTEXT.md §D-50 (no docker socket dep; defer log tail)
    - internal/cli/configcheck.go (reuse config-check output JSON)
    - internal/cli/healthcheck.go (reuse health-detailed probe)
    - internal/audit/log.go (audit row vocabulary; last-100 query)
    - internal/install/state.go (schema version + install metadata)
  </read_first>
  <behavior>
    - Test (TestRedact_MaskEmailSimple): MaskEmail("john.doe@example.com") returns "j***@example.com".
    - Test (TestRedact_MaskEmailShort): MaskEmail("a@x.com") returns "a***@x.com".
    - Test (TestRedact_MaskEmailMalformed): MaskEmail("not-an-email") returns "[redacted]".
    - Test (TestRedact_JSONReplacesEmailTokens): RedactJSON applied to `{"user":{"email":"jdoe@acme.io"}}` returns the bytes with `"j***@acme.io"`.
    - Test (TestRedact_JSONHandlesMultipleEmails): array of 3 audit rows each with an email → all 3 masked.
    - Test (TestDoctor_SnapshotBundleShape): Doctor.SnapshotBundle returns JSON with keys: generated_at, shifter.{version,schema_version,uptime_seconds}, config_check, health_detailed, alert_workers (array), last_backup, chirpstack_grpc_ping, recent_audit (array of 100), logs (string "<docker socket not mounted — run...>").
    - Test (TestDoctor_RecentAuditIsRedacted): seed audit rows with operator emails → bundle's recent_audit entries have masked emails.
    - Test (TestDoctor_LogsContainsFallbackNote): bundle.logs is the literal string "<docker socket not mounted — run `docker compose logs --tail=200 shifter` from host>" (Docker SDK NOT pulled into v1 binary per RESEARCH Open Question #3).
    - Test (TestCLIDoctor_StdoutDefault): `shifter doctor` writes JSON to stdout; exits 0.
    - Test (TestCLIDoctor_OutFile): `shifter doctor --out /tmp/d.json` writes to file; file exists; stdout silent.
    - Test (TestCLIDoctor_AdminRequiredViaCLI): unauthenticated invocation (no admin context) — the doctor CLI is operator-only (runs inside the binary's process, not via HTTP); RBAC doesn't apply at CLI layer. Verify that no PII leaks regardless of caller.
  </behavior>
  <action>
    **internal/auth/authz.go:** Add `ActionDoctorRead Action = "doctor.read"` if a future HTTP `/api/doctor` is wanted (NOT in v1 — CLI only). Skip for now.

    **internal/doctor/redact.go** (RESEARCH §Decision H PII Redaction verbatim):
    ```go
    package doctor

    import (
        "regexp"
        "strings"
    )

    var emailRegex = regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`)

    // MaskEmail returns "j***@example.com" form. Returns "[redacted]" for malformed.
    func MaskEmail(email string) string {
        at := strings.Index(email, "@")
        if at <= 0 || at == len(email)-1 { return "[redacted]" }
        return string(email[0]) + "***" + email[at:]
    }

    // RedactJSON walks the serialized JSON bytes and replaces every email-shaped
    // token with its masked form. Treats every email shape conservatively (may
    // mask a legitimate non-email token like "user@host" in connection strings;
    // acceptable for a support bundle).
    func RedactJSON(b []byte) []byte {
        return emailRegex.ReplaceAllFunc(b, func(match []byte) []byte {
            return []byte(MaskEmail(string(match)))
        })
    }
    ```

    **internal/doctor/doctor.go:**
    ```go
    package doctor

    type Bundle struct {
        GeneratedAt        time.Time            `json:"generated_at"`
        Shifter            ShifterInfo          `json:"shifter"`
        ConfigCheck        any                  `json:"config_check"`
        HealthDetailed     any                  `json:"health_detailed"`
        AlertWorkers       []AlertWorkerHealth  `json:"alert_workers"`
        LastBackup         *LastBackupHealth    `json:"last_backup"`
        ChirpstackGRPCPing GRPCPing             `json:"chirpstack_grpc_ping"`
        RecentAudit        []AuditRow           `json:"recent_audit"`
        Logs               []string             `json:"logs"`
    }

    type Doctor struct {
        Pool     *pgxpool.Pool
        InstallState *install.Store
        CSDialer csDialer  // existing ChirpStack ping helper
        ConfigChecker *cli.ConfigChecker
        Health   *http.HealthProbe
    }

    func (d *Doctor) SnapshotBundle(ctx context.Context) (*Bundle, error) {
        bundle := &Bundle{GeneratedAt: time.Now().UTC()}
        bundle.Shifter = d.gatherShifterInfo(ctx)
        bundle.ConfigCheck = d.ConfigChecker.Run(ctx)
        bundle.HealthDetailed = d.Health.Detailed(ctx)
        bundle.AlertWorkers = d.loadAlertWorkers(ctx)
        bundle.LastBackup = d.loadLastBackup(ctx)
        bundle.ChirpstackGRPCPing = d.pingChirpstack(ctx)
        bundle.RecentAudit = d.loadRecentAudit(ctx, 100)
        bundle.Logs = []string{"<docker socket not mounted — run `docker compose logs --tail=200 shifter` from host>"}
        return bundle, nil
    }

    // MarshalRedacted returns the bundle as JSON bytes after running the email
    // regex pass over the serialized payload.
    func (b *Bundle) MarshalRedacted() ([]byte, error) {
        raw, err := json.MarshalIndent(b, "", "  ")
        if err != nil { return nil, err }
        return RedactJSON(raw), nil
    }
    ```

    `loadRecentAudit` calls audit/browse_store.go's `ListCursor` with limit=100, no filter; renders each row including `user_email` field (which redaction then masks). The before/after JSON columns are passed through RedactJSON too (the top-level marshal+redact handles them automatically since RedactJSON runs on the whole bundle bytes).

    **internal/cli/doctor.go:**
    ```go
    var doctorCmd = &cobra.Command{
        Use:   "doctor",
        Short: "Print a redacted diagnostic bundle for Shifter support",
        Long:  `Outputs a JSON bundle with config-check, /health/detailed, alert worker status, last backup, ChirpStack gRPC ping, last 100 audit rows (emails masked j***@example.com). Operator emails the bundle when filing a support ticket. Container logs are NOT included in v1 (Docker socket dependency deferred to v1.x); the bundle's 'logs' field instructs operators to attach 'docker compose logs --tail=200 shifter' manually.`,
        RunE:  runDoctorCmd,
    }
    var doctorFlagOut string
    func init() {
        doctorCmd.Flags().StringVar(&doctorFlagOut, "out", "", "Optional output file path (default stdout)")
        rootCmd.AddCommand(doctorCmd)
    }
    func runDoctorCmd(cmd *cobra.Command, args []string) error {
        // 1. Build Doctor with current viper config (DB + ChirpStack + install state)
        // 2. ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
        // 3. bundle, err := d.SnapshotBundle(ctx); err handling
        // 4. raw, err := bundle.MarshalRedacted(); err handling
        // 5. if doctorFlagOut != "": os.WriteFile(doctorFlagOut, raw, 0o600); else: os.Stdout.Write(raw)
    }
    ```

    Register doctorCmd via the same init() addCommand pattern as backup/restore.

    NOTE: the doctor CLI runs in the same binary as serve; it must NOT require the HTTP server to be running. The Doctor probes DB + ChirpStack directly via the existing config-check and healthcheck primitives. The /health/detailed JSON shape is reused, but the Doctor calls the underlying probe functions, not the HTTP endpoint.
  </action>
  <verify>
    <automated>go test ./internal/doctor/... ./internal/cli/... -run "TestRedact|TestDoctor|TestCLIDoctor" -count=1 -timeout=60s</automated>
  </verify>
  <acceptance_criteria>
    - `internal/doctor/redact.go` contains `func MaskEmail(` AND `var emailRegex = regexp.MustCompile(`
    - `internal/doctor/doctor.go` contains `type Bundle struct` with all 9 listed fields
    - `internal/doctor/doctor.go` contains `func (b *Bundle) MarshalRedacted(` which calls RedactJSON on the serialized output
    - `internal/cli/doctor.go` has `var doctorCmd` with `Use: "doctor"` and a `--out` flag
    - `internal/cli/doctor.go` `doctorCmd.Long` mentions "logs are NOT included in v1" (D-50 deferral documentation)
    - `internal/doctor/doctor.go::loadLogs` returns the static fallback string (no actual docker socket call)
    - All 11 listed tests pass: `go test ./internal/doctor/... ./internal/cli/... -run "TestRedact|TestDoctor|TestCLIDoctor" -count=1` exits 0
    - `shifter doctor --out /tmp/bundle.json` (CLI smoke test) produces a valid JSON file with all expected top-level keys
  </acceptance_criteria>
  <done>Operators have a one-command support diagnostic; emails masked; no Docker socket dependency in v1.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 3: Compose conventions audit + lint test + 3 operator-runbook sections</name>
  <files>compose/bundled.yml, compose/external.yml, docs/operator-runbook.md, internal/compose/conventions_test.go, internal/compose/doc.go</files>
  <read_first>
    - compose/bundled.yml (full file — audit every service)
    - compose/external.yml (full file)
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-CONTEXT.md §D-48, D-49
    - docs/operator-runbook.md (current sections; Plan 06-09 already added Backup & Restore — append Compose conventions + Upgrading Shifter)
  </read_first>
  <behavior>
    - Test (TestCompose_NoLatestTag): both compose files contain ZERO occurrences of `:latest` (across all service definitions).
    - Test (TestCompose_EveryServiceHasJsonLogging): every `services.<name>` block has `logging: *json-logging` (anchor reference). Plan 06-08's `backup-cron` service must include this too.
    - Test (TestCompose_NoEnvCredentials): no service has env vars with the literal pattern `_PASSWORD: ${...}` or `_API_TOKEN: ${...}` — credentials should be `_FILE: /run/secrets/...` via the `secrets:` declaration.
    - Test (TestCompose_AllSecretsMounted): every secret referenced via `_FILE: /run/secrets/<name>` has a corresponding `secrets.<name>.file: ../secrets/<file>` entry.
    - Test (TestCompose_BackupsVolumeMounted): both files have the `backups:` named volume + mounted in the shifter service.
    - Test (TestCompose_PinnedImageVersions): every `image:` line matches a regex with a version tag (`image: foo/bar:v1.2.3` or `image: foo/bar:1.2.3` not `image: foo/bar`).
    - Test (TestCompose_NoExposedInternalServices): postgres, mosquitto, redis, chirpstack do NOT have host port mappings in their service blocks; only caddy (80/443) + chirpstack-gateway-bridge (1700/udp).
    - Test (TestRunbook_HasComposeConventions): docs/operator-runbook.md contains the heading "## Compose conventions".
    - Test (TestRunbook_HasUpgradingSection): docs/operator-runbook.md contains the heading "## Upgrading Shifter" + the 5 steps (1 backup, 2 edit tag, 3 pull+up, 4 verify health, 5 rollback procedure).
    - Test (TestRunbook_BackupRestoreSection): Plan 06-09 added "## Backup & Restore"; preserved in this audit.
  </behavior>
  <action>
    **internal/compose/conventions_test.go (NEW FILE):**
    ```go
    package compose_test

    import (
        "os"
        "regexp"
        "strings"
        "testing"
        "github.com/stretchr/testify/require"
    )

    func readCompose(t *testing.T, path string) string {
        t.Helper()
        b, err := os.ReadFile(path)
        require.NoError(t, err)
        return string(b)
    }

    func TestCompose_NoLatestTag(t *testing.T) {
        for _, p := range []string{"../../compose/bundled.yml", "../../compose/external.yml"} {
            content := readCompose(t, p)
            require.NotContains(t, content, ":latest", "compose file %s must not contain :latest", p)
        }
    }

    func TestCompose_PinnedImageVersions(t *testing.T) {
        re := regexp.MustCompile(`image:\s+\S+:\S+`)        // image: name:tag
        bareImage := regexp.MustCompile(`^\s+image:\s+\S+$`) // image: name (no tag)
        for _, p := range []string{"../../compose/bundled.yml", "../../compose/external.yml"} {
            content := readCompose(t, p)
            lines := strings.Split(content, "\n")
            for i, line := range lines {
                if bareImage.MatchString(line) && !re.MatchString(line) {
                    t.Fatalf("%s line %d has un-pinned image: %s", p, i+1, line)
                }
            }
        }
    }

    func TestCompose_EveryServiceHasJsonLogging(t *testing.T) {
        // Parse the compose YAML and assert each service has logging: *json-logging
        // OR a logging: block with driver: json-file + size + file caps.
        // Implementation: yaml.Unmarshal into a map[string]any; iterate services.
    }

    func TestCompose_NoEnvCredentials(t *testing.T) {
        for _, p := range []string{"../../compose/bundled.yml", "../../compose/external.yml"} {
            content := readCompose(t, p)
            envVarLeak := regexp.MustCompile(`_(PASSWORD|API_TOKEN|SECRET|KEY):\s+\$\{`)
            require.NotRegexp(t, envVarLeak, content, "compose file %s leaks secret via env var", p)
        }
    }

    // ... TestCompose_BackupsVolumeMounted, TestCompose_NoExposedInternalServices ...
    ```

    **compose/bundled.yml audit:** Re-read the file in full. For each `services.<name>` block, verify:
    1. `image: foo/bar:vX.Y.Z` — pinned tag, not :latest. If any service has :latest, replace with a pinned tag (current state in repo from Phase 1 should already be pinned; verify and fix if not).
    2. `logging: *json-logging` anchor present. If missing on any service (the new `backup-cron` from Plan 06-08 must include it), add.
    3. Credentials mounted via `secrets:` with `_FILE: /run/secrets/<name>`. No `_PASSWORD: ${SOMETHING}` env-var patterns.

    **compose/external.yml audit:** Same checks.

    **docs/operator-runbook.md** — Append two new sections:

    ```markdown
    ## Compose conventions

    All Shifter Compose files follow these conventions (enforced by `go test ./internal/compose/...`):

    - **Pinned image tags only.** No `:latest` anywhere. Every service has a specific version (e.g., `timescale/timescaledb:2.26.0-pg16`, `mcuadros/ofelia:v0.3.22`).
    - **json-file logging caps.** Every service block contains `logging: *json-logging` (or an equivalent explicit `logging:` block with `driver: json-file` + `max-size: 10m` + `max-file: 3`). Prevents unbounded disk usage.
    - **Secrets via `secrets:` declarations.** Credentials are read from `/run/secrets/<name>` files (mounted by Compose from `../secrets/*.txt`), NOT from environment variables. Never use `${SHIFTER_DB_PASSWORD}` in compose files.
    - **No exposed internal services.** Postgres, Mosquitto, Redis, ChirpStack do NOT bind host ports. Only Caddy (80/443) and the ChirpStack gateway bridge (1700/udp) bind externally.
    - **Backups volume.** Both compose flavors mount `/var/lib/shifter/backups` via a named volume `backups`. Bundled mode adds the `mcuadros/ofelia` cron sidecar (read-only Docker socket); external mode operators add their own cron per the runbook.

    When editing a compose file, run `go test ./internal/compose/...` to confirm conventions hold.

    ## Upgrading Shifter

    Operator-driven five-step procedure per Shifter release (no in-app upgrade in v1):

    1. **Take a backup** (per the Backup & Restore section):
       ```sh
       docker compose -f compose/bundled.yml exec shifter shifter backup --to /var/lib/shifter/backups
       ```
       Confirm the tarball is written and the `backup_run` row shows `status='completed'`.

    2. **Bump the image tag.** Edit `compose/bundled.yml` (or `compose/external.yml`) and change `image: shifter:0.X.Y` to the new release tag. NEVER use `:latest` — the convention test will fail.

    3. **Pull + restart Shifter only.** Other services (Postgres, ChirpStack, etc.) stay running:
       ```sh
       docker compose -f compose/bundled.yml pull shifter
       docker compose -f compose/bundled.yml up -d shifter
       ```
       Expected downtime: ≤ 5 seconds (HTTP server restart). Ingest pause window: ≤ 5 seconds. Uplinks queued in MQTT during restart are processed on resume.

    4. **Verify the upgrade.** Hit `/health` (public) and `/health/detailed` (admin auth):
       ```sh
       curl https://shifter.example.com/health
       # expected: {"status":"ok","version":"0.X.Y","uptime_seconds":...}
       ```
       Sign in to the UI and confirm `/health/detailed` reports:
       - `alert_workers[*].degraded = false` for every worker
       - `last_backup.age_seconds < backup_warn_threshold_hours * 3600`
       - `chirpstack.reachable = true`

       If any signal is red, rollback (step 5).

    5. **Rollback procedure** (if step 4 fails or any blocker surfaces within 24 h):
       ```sh
       docker compose -f compose/bundled.yml stop shifter
       docker compose -f compose/bundled.yml run --rm shifter shifter restore --from /var/lib/shifter/backups/<pre-upgrade>.tar.gz
       # Edit compose file: revert image tag to the previous version
       docker compose -f compose/bundled.yml up -d shifter
       ```

    ### Per-release breaking-migration notes

    Each Shifter release appends a "v0.X.Y release notes" subsection here when migrations change in ways operators must know about. Phase 6 (v0.6.0) adds migrations 0037–0047 (audit vocabulary, alert engine substrate, retention extensions, backup_run history). No data migration risk: all are additive.
    ```

    `internal/compose/doc.go`: package doc explaining the lint test's purpose.
  </action>
  <verify>
    <automated>go test ./internal/compose/... -count=1 -timeout=30s</automated>
  </verify>
  <acceptance_criteria>
    - `compose/bundled.yml` contains zero `:latest` (grep returns 0)
    - `compose/external.yml` contains zero `:latest`
    - Every service block in BOTH files has `logging: *json-logging` (or full json-file logging block); grep count of "logging:" matches the number of services in each file
    - No `_PASSWORD:` or `_API_TOKEN:` env-var entries with `${...}` interpolation in either file (grep returns 0)
    - `internal/compose/conventions_test.go` exists with at least 6 test functions
    - `docs/operator-runbook.md` contains both new headings `## Compose conventions` AND `## Upgrading Shifter`
    - The Upgrading section has 5 numbered steps (count grep `^[1-5]\. ` returns >= 5 within the section)
    - All 9 listed tests pass: `go test ./internal/compose/... -count=1 -timeout=30s` exits 0
    - `TestCompose_BackupsVolumeMounted` confirms Plan 06-08's backups volume is correctly mounted in both flavors
  </acceptance_criteria>
  <done>Compose hygiene is now machine-verified on every PR; the runbook documents upgrade + rollback; operator can ship Phase 6 confidently.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| support bundle → operator email | Bundle bytes leave the install; PII must be masked |
| compose lint → CI | The lint test runs on every PR; failing build prevents convention drift |
| /health/detailed → admin | Existing admin-only authn (Phase 1); Plan 06-11 extends payload |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-06-11-01 | Information Disclosure | doctor bundle leaks operator emails | mitigate | RedactJSON pass over serialized bundle bytes; MaskEmail returns "j***@example.com" form; tests verify masking on audit rows + JSON walks. |
| T-06-11-02 | Information Disclosure | doctor bundle leaks password hashes | mitigate | recent_audit query selects audit_log columns ONLY (action, entity_type, entity_id, user_email via JOIN, request_id, notes, time); audit_log.before/after JSONB columns are included BUT no Phase 6 mutation writes password_hash into before/after (verified by Plan 06-05 ToDTO without PasswordHash). |
| T-06-11-03 | Information Disclosure | doctor bundle leaks connection strings with embedded passwords | mitigate | config_check output sanitizes DB password (Phase 1 already does `password=***` for env-var leaks); the RedactJSON pass catches any email-shaped tokens in connection strings. Additional: doctor.go's `gatherShifterInfo` does NOT include `os.Environ()` or `cfg.DBPassword` in the bundle. |
| T-06-11-04 | Tampering | compose drift introduces :latest or .env credentials | mitigate | `internal/compose/conventions_test.go` runs in CI; any drift fails the build. Backstop: D-48 manual audit also performed in this plan. |
| T-06-11-05 | DoS | alerts retention prune runs while operator views the alert center | accept | DELETE happens at 03:30 install_tz; operator-traffic windows differ. Even during a query, MVCC isolates the DELETE; the operator sees pre-prune state until their query completes. |
| T-06-11-06 | Elevation of Privilege | /health/detailed exposes internal state to unauth | mitigate | Existing Phase 1 authn applies; Plan 06-11 extends the JSON shape but does NOT add a new route. Test (Phase 1): TestHealthDetailed_AdminOnly preserves. |
| T-06-11-07 | Repudiation | alerts pruned without trail | mitigate | AlertsPruneWorker writes a single 'alert.pruned' audit_log row per cycle with the deleted row count. Test: TestAlertsPruneWorker_AuditRow. |
</threat_model>

<verification>
- Compose conventions lint passes on both compose files
- shifter doctor produces valid JSON with masked emails
- /health/detailed includes alert_workers + last_backup
- Alerts retention prune runs daily at 03:30 install_tz
- Operator runbook has Compose conventions + Upgrading Shifter sections (Backup & Restore from Plan 06-09 preserved)
- All listed tests pass: `go test ./internal/alert/... ./internal/doctor/... ./internal/cli/... ./internal/http/... ./internal/compose/... -count=1 -timeout=120s`
</verification>

<success_criteria>
- OPS-05 covered: json-file logging caps verified on every service via lint
- OPS-06 covered: secrets via Compose secrets: verified via lint
- OPS-07 covered: pinned image tags verified via lint
- OPS-08 covered: per-release upgrade runbook with rollback documented
- D-21, D-22, D-48, D-49, D-50 implemented
</success_criteria>

<output>
After completion, create `.planning/phases/06-alerts-users-audit-operational-hardening/06-11-SUMMARY.md`
</output>
