---
phase: 06-alerts-users-audit-operational-hardening
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/db/migrations/0037_audit_vocab_phase6.up.sql
  - internal/db/migrations/0037_audit_vocab_phase6.down.sql
  - internal/db/migrations/0038_alert_rule.up.sql
  - internal/db/migrations/0038_alert_rule.down.sql
  - internal/db/migrations/0039_alert.up.sql
  - internal/db/migrations/0039_alert.down.sql
  - internal/db/migrations/0040_retention_config_phase6.up.sql
  - internal/db/migrations/0040_retention_config_phase6.down.sql
  - internal/db/migrations/0042_alert_worker_state.up.sql
  - internal/db/migrations/0042_alert_worker_state.down.sql
  - internal/db/migrations/0043_admin_prune_audit_rows.up.sql
  - internal/db/migrations/0043_admin_prune_audit_rows.down.sql
  - internal/audit/log.go
  - internal/alert/engine.go
  - internal/alert/rule_store.go
  - internal/alert/alert_store.go
  - internal/alert/degraded.go
  - internal/alert/worker_state.go
  - internal/alert/engine_test.go
  - internal/alert/rule_store_test.go
  - internal/alert/alert_store_test.go
  - internal/alert/degraded_test.go
  - internal/alert/audit_prune_worker.go
  - internal/alert/audit_prune_worker_test.go
  - internal/install/finish.go
  - go.mod
autonomous: true
requirements: [ALERT-05, ALERT-06, SETT-01, OPS-03]
must_haves:
  truths:
    - "Schema migrations 0037..0043 land cleanly and rollback cleanly"
    - "audit_log CHECK constraints accept Phase 6 vocabulary (auth.*, user.*, alert.*, backup.*, restore.*, audit.prune)"
    - "alert_rule + alert + alert_worker_state tables exist with indexes and CHECK constraints from RESEARCH §Decision C"
    - "retention_config has alerts_days (default 365) and audit_log_days (default 1825) columns"
    - "admin_prune_audit_rows(cutoff_days INTEGER) SECURITY DEFINER function exists and bypasses the INSERT-ONLY trigger via SET LOCAL session_replication_role"
    - "River subscriber StartDegradedSubscriber flips alert_worker_state.degraded=true when a job hits state='discarded'"
    - "AuditPruneWorker runs daily at 03:00 install_tz via River cron, deletes rows older than audit_log_days, writes meta audit row"
  artifacts:
    - path: internal/db/migrations/0038_alert_rule.up.sql
      provides: "alert_rule table with rule_kind/scope_kind/severity/cooldown_seconds/disabled_at/quiet_window_start/quiet_window_end"
      contains: "CREATE TABLE alert_rule"
    - path: internal/db/migrations/0039_alert.up.sql
      provides: "alert table with state machine + payload JSONB + ack/snooze/clear/test fields"
      contains: "CREATE TABLE alert"
    - path: internal/db/migrations/0043_admin_prune_audit_rows.up.sql
      provides: "SECURITY DEFINER function admin_prune_audit_rows"
      contains: "SECURITY DEFINER"
    - path: internal/alert/engine.go
      provides: "EvaluateContext + RunState helpers shared by all workers"
    - path: internal/alert/degraded.go
      provides: "StartDegradedSubscriber(ctx, riverClient, queries, log) goroutine"
    - path: internal/alert/audit_prune_worker.go
      provides: "AuditPruneWorker for 03:00 daily prune (calls admin_prune_audit_rows())"
  key_links:
    - from: internal/install/finish.go
      to: retention_config
      via: "INSERT/UPDATE retention_config sets alerts_days=365 + audit_log_days=1825 inside FinishSetup tx"
      pattern: "alerts_days.*365"
    - from: internal/alert/degraded.go
      to: internal/alert/worker_state.go
      via: "subscriber calls MarkAlertWorkerDegraded on JobStateDiscarded"
      pattern: "JobStateDiscarded"
---

<objective>
Land the Phase 6 substrate: every migration the rest of the phase depends on (audit vocabulary expansion, alert_rule/alert/alert_worker_state tables, retention_config extension, admin_prune_audit_rows SECURITY DEFINER function), plus the engine-level helpers (EvaluateContext, RunState upserter, River degraded-subscriber, AuditPruneWorker). After this plan, the rest of the phase plans can run in parallel because they only ADD code on top of this substrate.

Purpose: separate the load-bearing schema decisions and shared engine code from the per-worker / per-UI plans, so a failure in any one of those does not corrupt the substrate. Locks in D-30 vocabulary, D-12 alert payload schema, D-13 alert retention, D-38 audit retention, D-51 SECURITY DEFINER prune function, D-21/D-22 alert worker observability.

Output: 6 new migrations (0037, 0038, 0039, 0040, 0042, 0043) + new `internal/alert/` package skeleton with engine + degraded subscriber + audit prune worker + tests + `internal/install/finish.go` seeds defaults.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/ROADMAP.md
@.planning/STATE.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-CONTEXT.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-VALIDATION.md
@internal/db/migrations/0016_audit_log.up.sql
@internal/db/migrations/0020_audit_log_vocabulary.up.sql
@internal/db/migrations/0031_audit_vocab_phase5.up.sql
@internal/db/migrations/0029_retention_config.up.sql
@internal/audit/log.go
@internal/install/finish.go
@internal/cli/serve.go

<interfaces>
<!-- Existing types Phase 6 substrate builds on -->

audit.Entry (internal/audit/log.go):
```go
type Entry struct {
    UserID     uuid.UUID  // uuid.Nil for system events
    Action     string     // must be in CHECK vocabulary
    EntityType string     // must be in CHECK vocabulary
    EntityID   uuid.UUID
    Before     map[string]any
    After      map[string]any
    Notes      string
    RequestID  string
}
func WriteEntry(ctx context.Context, tx pgx.Tx, e Entry) error
```

audit_log existing CHECK (from 0016 + 0020 + 0031 + 0034 + 0035 + 0036):
Existing actions: create, update, archive, restore, swap, decommission, profile_create, profile_update, binding_open, binding_close, rollover_detected, gateway.create, gateway.update, gateway.archive, gateway.restore, device.bulk_import, device.reveal_secrets, report.generate, floor_plan.upload, floor_plan.replace_image, floor_plan.rename, floor_plan.delete, placement.create, placement.update, placement.delete, retention.update
Existing entity_types: site, metering_point, device, device_profile, binding, gateway, import_job, report, floor_plan, placement, retention_config

audit_log INSERT-ONLY trigger (from 0016, lines 32-50):
`CREATE TRIGGER audit_log_no_delete BEFORE DELETE ON audit_log FOR EACH ROW EXECUTE FUNCTION audit_log_reject_modification();`
**Bypass requires SECURITY DEFINER + SET LOCAL session_replication_role = 'replica' inside the function tx.**

retention_config existing columns (from 0029):
`id INTEGER PRIMARY KEY CHECK (id = 1), raw_days, hourly_days, daily_days, monthly_days, yearly_days NULL, updated_at`

River setup pattern in internal/cli/serve.go (lines 334-357):
`river.AddWorker(riverWorkers, &W{...})` then `river.NewPeriodicJob(interval, argsFunc, opts)` — periodic jobs list passed to river.Config.

D-12 canonical payload (RESEARCH §Decision C):
```json
{"rule_id":"...", "rule_kind":"threshold_hourly", "severity":"critical",
 "target":{"entity_type":"metering_point","entity_id":"...","label":"..."},
 "value":42.7, "threshold":40.0, "comparison":"gt", "unit":"m3/h",
 "fired_at":"...","install":{"display_name":"..."}}
```
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Migration 0037 — audit vocabulary expansion for Phase 6 (D-30, D-51)</name>
  <files>internal/db/migrations/0037_audit_vocab_phase6.up.sql, internal/db/migrations/0037_audit_vocab_phase6.down.sql, internal/audit/log.go</files>
  <read_first>
    - internal/db/migrations/0016_audit_log.up.sql (existing CHECK constraint structure)
    - internal/db/migrations/0020_audit_log_vocabulary.up.sql (Phase 3 vocab extension pattern — copy this exact ALTER TABLE DROP/ADD CONSTRAINT pattern)
    - internal/db/migrations/0031_audit_vocab_phase5.up.sql (Phase 5 vocab extension pattern)
    - internal/db/migrations/0036_audit_vocab_retention.up.sql (Phase 5 retention vocab — most recent example)
    - internal/audit/log.go (constants pattern; new constants must mirror migration string literals exactly per existing comment)
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-CONTEXT.md §D-30 (exact action list)
  </read_first>
  <behavior>
    - Test: After migration up, `INSERT INTO audit_log(user_id, action, entity_type, entity_id) VALUES (gen_random_uuid(), 'auth.login_success', 'user', gen_random_uuid())` succeeds.
    - Test: After migration up, `INSERT INTO audit_log(... action 'unknown.action', entity_type 'user' ...)` fails with 23514.
    - Test: After migration down, every new Phase 6 vocabulary string fails 23514 on INSERT (rollback removes them).
    - Test: every constant in `internal/audit/log.go` Phase 6 block matches a string literal in 0037 (string equality grep).
  </behavior>
  <action>
    Create migration `0037_audit_vocab_phase6.up.sql` that ALTERs `audit_log_action_valid` CHECK to add (in addition to existing entries from 0016/0020/0031/0034/0035/0036) exactly these 23 new action strings (D-30 operator-visible events + D-51 prune + Phase 6 alert/user/backup verbs):
    - `auth.login_success`, `auth.login_failed`, `auth.logout`, `auth.password_change`, `auth.password_reset_by_admin`, `auth.session_revoked`
    - `user.create`, `user.update`, `user.disable`, `user.enable`, `user.role_change`
    - `alert.rule_create`, `alert.rule_update`, `alert.rule_disable`, `alert.rule_enable`, `alert.fired`, `alert.cleared`, `alert.acknowledged`, `alert.snoozed`, `alert.muted`, `alert.test_fired`
    - `backup.start`, `backup.complete`, `backup.failed`, `backup.restore`
    - `audit.prune`

    ALTERs `audit_log_entity_type_valid` to add: `user`, `session`, `alert_rule`, `alert`, `backup_run`, `audit_log` (the last one for the meta `audit.prune` row per D-51).

    Pattern (mirror exactly the Phase 5 0036_audit_vocab_retention.up.sql shape):
    ```sql
    ALTER TABLE audit_log DROP CONSTRAINT audit_log_action_valid;
    ALTER TABLE audit_log ADD CONSTRAINT audit_log_action_valid CHECK (action IN (
      'create','update','archive','restore','swap','decommission',
      'profile_create','profile_update','binding_open','binding_close',
      'rollover_detected',
      'gateway.create','gateway.update','gateway.archive','gateway.restore',
      'device.bulk_import','device.reveal_secrets',
      'report.generate',
      'floor_plan.upload','floor_plan.replace_image','floor_plan.rename','floor_plan.delete',
      'placement.create','placement.update','placement.delete',
      'retention.update',
      -- Phase 6 (D-30 operator-visible auth events):
      'auth.login_success','auth.login_failed','auth.logout',
      'auth.password_change','auth.password_reset_by_admin','auth.session_revoked',
      -- Phase 6 user management:
      'user.create','user.update','user.disable','user.enable','user.role_change',
      -- Phase 6 alerts:
      'alert.rule_create','alert.rule_update','alert.rule_disable','alert.rule_enable',
      'alert.fired','alert.cleared','alert.acknowledged','alert.snoozed','alert.muted','alert.test_fired',
      -- Phase 6 backup/restore + audit prune (D-51):
      'backup.start','backup.complete','backup.failed','backup.restore','audit.prune'
    ));
    -- Same DROP/ADD for audit_log_entity_type_valid adding: 'user','session','alert_rule','alert','backup_run','audit_log'
    ```

    Down migration: reverse — drop both new CHECKs, recreate the Phase 5 versions verbatim from 0036.

    In `internal/audit/log.go` append a new const block "Phase 6 — Plan 06-01: alerts, users, audit-prune, backup, auth-event retrofit" with these exact string constants:
    ```go
    // Phase 6 — Plan 06-01: auth-event audit retrofit (D-30; operator-visible only).
    const (
        ActionAuthLoginSuccess        = "auth.login_success"
        ActionAuthLoginFailed         = "auth.login_failed"
        ActionAuthLogout              = "auth.logout"
        ActionAuthPasswordChange      = "auth.password_change"
        ActionAuthPasswordResetByAdmin= "auth.password_reset_by_admin"
        ActionAuthSessionRevoked      = "auth.session_revoked"
    )
    // Phase 6 user mgmt:
    const (
        ActionUserCreate      = "user.create"
        ActionUserUpdate      = "user.update"
        ActionUserDisable     = "user.disable"
        ActionUserEnable      = "user.enable"
        ActionUserRoleChange  = "user.role_change"
    )
    // Phase 6 alerts:
    const (
        ActionAlertRuleCreate  = "alert.rule_create"
        ActionAlertRuleUpdate  = "alert.rule_update"
        ActionAlertRuleDisable = "alert.rule_disable"
        ActionAlertRuleEnable  = "alert.rule_enable"
        ActionAlertFired       = "alert.fired"
        ActionAlertCleared     = "alert.cleared"
        ActionAlertAcked       = "alert.acknowledged"
        ActionAlertSnoozed     = "alert.snoozed"
        ActionAlertMuted       = "alert.muted"
        ActionAlertTestFired   = "alert.test_fired"
    )
    // Phase 6 backup + audit prune (D-51):
    const (
        ActionBackupStart    = "backup.start"
        ActionBackupComplete = "backup.complete"
        ActionBackupFailed   = "backup.failed"
        ActionBackupRestore  = "backup.restore"
        ActionAuditPrune     = "audit.prune"
    )
    // Phase 6 entity types:
    const (
        EntityTypeUser      = "user"
        EntityTypeSession   = "session"
        EntityTypeAlertRule = "alert_rule"
        EntityTypeAlert     = "alert"
        EntityTypeBackupRun = "backup_run"
        EntityTypeAuditLog  = "audit_log" // for audit.prune meta-rows (D-51)
    )
    ```
  </action>
  <verify>
    <automated>go test ./internal/audit/... -run TestPhase6VocabularyConstants -count=1</automated>
  </verify>
  <acceptance_criteria>
    - `internal/db/migrations/0037_audit_vocab_phase6.up.sql` exists and contains all 26 new action strings AND all 6 new entity-type strings (grep: `grep -c "'auth.login_success'" internal/db/migrations/0037_audit_vocab_phase6.up.sql` returns >= 1 — appears exactly once in the new CHECK literal list)
    - Down migration drops the Phase 6 CHECK and re-creates the Phase 5 vocabulary verbatim
    - `internal/audit/log.go` contains all 26 const names: `grep -c "ActionAuthLoginSuccess\|ActionAuthLoginFailed\|ActionAuthLogout\|ActionAuthPasswordChange\|ActionAuthPasswordResetByAdmin\|ActionAuthSessionRevoked\|ActionUserCreate\|ActionUserUpdate\|ActionUserDisable\|ActionUserEnable\|ActionUserRoleChange\|ActionAlertRuleCreate\|ActionAlertRuleUpdate\|ActionAlertRuleDisable\|ActionAlertRuleEnable\|ActionAlertFired\|ActionAlertCleared\|ActionAlertAcked\|ActionAlertSnoozed\|ActionAlertMuted\|ActionAlertTestFired\|ActionBackupStart\|ActionBackupComplete\|ActionBackupFailed\|ActionBackupRestore\|ActionAuditPrune" internal/audit/log.go` returns 26
    - All 6 entity types present: `grep -c "EntityTypeUser\|EntityTypeSession\|EntityTypeAlertRule\|EntityTypeAlert\|EntityTypeBackupRun\|EntityTypeAuditLog" internal/audit/log.go` returns 6
    - `go test ./internal/audit/... -count=1` passes
  </acceptance_criteria>
  <done>Migration up/down apply cleanly against a fresh testcontainer DB; INSERT of every new vocabulary string succeeds; INSERT of an unknown string still 23514s; constants drift test green.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Migrations 0038 (alert_rule), 0039 (alert), 0042 (alert_worker_state) + 0040 (retention_config extension) + 0043 (admin_prune_audit_rows SECURITY DEFINER)</name>
  <files>internal/db/migrations/0038_alert_rule.up.sql, internal/db/migrations/0038_alert_rule.down.sql, internal/db/migrations/0039_alert.up.sql, internal/db/migrations/0039_alert.down.sql, internal/db/migrations/0040_retention_config_phase6.up.sql, internal/db/migrations/0040_retention_config_phase6.down.sql, internal/db/migrations/0042_alert_worker_state.up.sql, internal/db/migrations/0042_alert_worker_state.down.sql, internal/db/migrations/0043_admin_prune_audit_rows.up.sql, internal/db/migrations/0043_admin_prune_audit_rows.down.sql, internal/install/finish.go</files>
  <read_first>
    - internal/db/migrations/0029_retention_config.up.sql (existing retention_config table; this plan ALTERs it)
    - internal/db/migrations/0016_audit_log.up.sql (INSERT-ONLY trigger — see lines 32-50; the SECURITY DEFINER function must bypass this)
    - internal/install/finish.go (FinishSetup atomic Serializable tx — Phase 6 seeds defaults inside)
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md §Decision C "Alert Schema Sketch"
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md §Decision F "Audit Retention Prune"
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-CONTEXT.md §D-51 (full function body)
  </read_first>
  <behavior>
    - Test (TestAlertRuleSchema): alert_rule row inserts with required fields succeed; rule_kind/scope_kind/severity CHECK constraints reject unknown values with 23514.
    - Test (TestAlertSchema): alert row insert with payload JSONB succeeds; state CHECK rejects unknown states.
    - Test (TestAlertWorkerStateSeed): 5 rows exist after install_finish (one per worker_kind).
    - Test (TestRetentionConfigPhase6Columns): SELECT alerts_days, audit_log_days FROM retention_config WHERE id=1 returns 365, 1825.
    - Test (TestAdminPruneAuditRows_Bypasses_Trigger): direct `DELETE FROM audit_log WHERE time < ...` raises 'audit_log is INSERT-ONLY'; `SELECT admin_prune_audit_rows(0)` succeeds and writes one `audit.prune` meta row; subsequent direct DELETE still raises (trigger re-enabled by COMMIT releasing SET LOCAL).
    - Test (TestAdminPruneAuditRows_ReturnsRowCount): function returns the number of pruned rows as INTEGER.
  </behavior>
  <action>
    **Migration 0038 — alert_rule** (copy RESEARCH §Decision C schema sketch verbatim, with quiet-hour extensions from §Decision E):
    ```sql
    CREATE TABLE alert_rule (
        id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        rule_kind        TEXT NOT NULL CHECK (rule_kind IN (
            'threshold_instantaneous','threshold_hourly','threshold_daily',
            'offline_device','offline_gateway',
            'anomaly_p95','anomaly_iqr','anomaly_quiet_hour'
        )),
        scope_kind       TEXT NOT NULL CHECK (scope_kind IN ('metering_point','site','device','gateway','global')),
        scope_id         UUID, -- nullable; required unless scope_kind='global'
        high_bound       DOUBLE PRECISION,
        low_bound        DOUBLE PRECISION,
        comparison       TEXT CHECK (comparison IS NULL OR comparison IN ('gt','gte','lt','lte','eq')),
        unit             TEXT,
        -- D-17 quiet-hour extension:
        quiet_window_start TIME,
        quiet_window_end   TIME,
        flow_threshold     DOUBLE PRECISION DEFAULT 0.0,
        days_of_week       INTEGER, -- bitmask Mon=1,Tue=2,...,Sun=64; NULL=all days
        -- D-07 severity + D-05 cooldown + D-06 name/notes:
        severity         TEXT NOT NULL DEFAULT 'critical' CHECK (severity IN ('info','warning','critical')),
        name             TEXT,
        notes            TEXT,
        cooldown_seconds INTEGER NOT NULL DEFAULT 900 CHECK (cooldown_seconds >= 0),
        last_fired_at    TIMESTAMPTZ,
        -- D-04 soft-delete:
        disabled_at      TIMESTAMPTZ,
        created_by       UUID REFERENCES "user"(id) ON DELETE SET NULL,
        created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
        updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
        CHECK (scope_kind = 'global' OR scope_id IS NOT NULL)
    );
    CREATE INDEX alert_rule_active_kind_idx ON alert_rule (rule_kind) WHERE disabled_at IS NULL;
    CREATE INDEX alert_rule_scope_idx       ON alert_rule (scope_kind, scope_id) WHERE disabled_at IS NULL;
    -- updated_at trigger reusing existing touch_updated_at pattern from 0014/0015 migrations
    ```

    **Migration 0039 — alert** (D-12 payload + ack/snooze/clear state machine):
    ```sql
    CREATE TABLE alert (
        id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        rule_id            UUID NOT NULL REFERENCES alert_rule(id),
        rule_kind          TEXT NOT NULL,
        severity           TEXT NOT NULL CHECK (severity IN ('info','warning','critical')),
        state              TEXT NOT NULL DEFAULT 'firing' CHECK (state IN ('firing','acknowledged','cleared','muted','snoozed')),
        payload            JSONB NOT NULL,
        target_entity_type TEXT NOT NULL,
        target_entity_id   UUID NOT NULL,
        is_test            BOOLEAN NOT NULL DEFAULT FALSE,
        fired_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
        cleared_at         TIMESTAMPTZ,
        acked_at           TIMESTAMPTZ,
        acked_by           UUID REFERENCES "user"(id) ON DELETE SET NULL,
        ack_note           TEXT,
        snoozed_until      TIMESTAMPTZ,
        snoozed_by         UUID REFERENCES "user"(id) ON DELETE SET NULL,
        muted              BOOLEAN NOT NULL DEFAULT FALSE
    );
    CREATE INDEX alert_firing_idx ON alert (fired_at DESC) WHERE state = 'firing';
    CREATE INDEX alert_state_idx  ON alert (state, fired_at DESC);
    CREATE INDEX alert_target_idx ON alert (target_entity_type, target_entity_id, fired_at DESC);
    CREATE INDEX alert_rule_id_idx ON alert (rule_id, fired_at DESC);
    -- Partial unique to prevent duplicate firing alerts per rule+target (idempotent fire pattern):
    CREATE UNIQUE INDEX alert_firing_unique_idx ON alert (rule_id, target_entity_id) WHERE state = 'firing';
    ```

    **Migration 0040 — retention_config_phase6** (extend Phase 5 table; D-13 + D-38):
    ```sql
    ALTER TABLE retention_config
        ADD COLUMN alerts_days INTEGER NOT NULL DEFAULT 365 CHECK (alerts_days BETWEEN 30 AND 3650),
        ADD COLUMN audit_log_days INTEGER NOT NULL DEFAULT 1825 CHECK (audit_log_days BETWEEN 90 AND 18250);
    COMMENT ON COLUMN retention_config.alerts_days IS 'D-13: alerts table retention. Default 365d (1 year).';
    COMMENT ON COLUMN retention_config.audit_log_days IS 'D-38: audit_log retention. Default 1825d (5 years).';
    ```

    **Migration 0042 — alert_worker_state** (D-21):
    ```sql
    CREATE TABLE alert_worker_state (
        worker_kind     TEXT PRIMARY KEY CHECK (worker_kind IN (
            'threshold_instantaneous','threshold_hourly','threshold_daily','offline','anomaly'
        )),
        last_run_at     TIMESTAMPTZ NOT NULL DEFAULT '1970-01-01T00:00:00Z',
        rules_evaluated INTEGER NOT NULL DEFAULT 0,
        fires_emitted   INTEGER NOT NULL DEFAULT 0,
        cleared         INTEGER NOT NULL DEFAULT 0,
        duration_ms     INTEGER NOT NULL DEFAULT 0,
        degraded        BOOLEAN NOT NULL DEFAULT FALSE,
        last_error      TEXT,
        updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
    );
    INSERT INTO alert_worker_state(worker_kind) VALUES
      ('threshold_instantaneous'),('threshold_hourly'),('threshold_daily'),('offline'),('anomaly');
    ```

    **Migration 0043 — admin_prune_audit_rows** (D-51 verbatim function body):
    ```sql
    CREATE OR REPLACE FUNCTION admin_prune_audit_rows(cutoff_days INTEGER)
    RETURNS INTEGER
    LANGUAGE plpgsql
    SECURITY DEFINER
    SET search_path = public, pg_temp
    AS $$
    DECLARE
        deleted_count INTEGER;
        cutoff_ts     TIMESTAMPTZ := now() - make_interval(days := cutoff_days);
        meta_id       UUID := gen_random_uuid();
    BEGIN
        IF cutoff_days < 0 THEN
            RAISE EXCEPTION 'admin_prune_audit_rows: cutoff_days must be >= 0';
        END IF;
        -- D-51: SET LOCAL session_replication_role = 'replica' disables ALL triggers
        -- (including audit_log_no_delete) for this tx only. Released at COMMIT/ROLLBACK.
        SET LOCAL session_replication_role = 'replica';
        DELETE FROM audit_log WHERE time < cutoff_ts;
        GET DIAGNOSTICS deleted_count = ROW_COUNT;
        -- INSERT is permitted by the trigger; write meta audit row inside the same tx
        -- AFTER setting replication_role back to 'origin' so the meta row passes
        -- through normal trigger logic if any INSERT triggers exist.
        SET LOCAL session_replication_role = 'origin';
        INSERT INTO audit_log (id, action, entity_type, entity_id, notes)
        VALUES (
            meta_id,
            'audit.prune',
            'audit_log',
            meta_id, -- self-reference; no real FK target
            'Pruned ' || deleted_count || ' rows older than ' || cutoff_ts::date
        );
        RETURN deleted_count;
    END
    $$;
    REVOKE ALL ON FUNCTION admin_prune_audit_rows(INTEGER) FROM PUBLIC;
    GRANT EXECUTE ON FUNCTION admin_prune_audit_rows(INTEGER) TO shifter;
    COMMENT ON FUNCTION admin_prune_audit_rows IS 'D-51: SECURITY DEFINER bypass of audit_log INSERT-ONLY trigger for retention prune. Called by River AuditPruneWorker. Trigger remains active for all other code paths.';
    ```

    **internal/install/finish.go extension:** Inside the existing Serializable tx (after the Phase 5 retention seed), UPDATE retention_config SET alerts_days=365, audit_log_days=1825 WHERE id=1. The columns have NOT NULL DEFAULTs so this is belt-and-suspenders; the test confirms values.

    All down migrations: reverse order (drop function, drop table, drop columns).
  </action>
  <verify>
    <automated>go test ./internal/db/... ./internal/install/... -run "TestAlertRuleSchema|TestAlertSchema|TestAlertWorkerStateSeed|TestRetentionConfigPhase6Columns|TestAdminPruneAuditRows" -count=1</automated>
  </verify>
  <acceptance_criteria>
    - `internal/db/migrations/0038_alert_rule.up.sql` contains `CREATE TABLE alert_rule (` and `cooldown_seconds INTEGER NOT NULL DEFAULT 900` and `quiet_window_start TIME`
    - `internal/db/migrations/0039_alert.up.sql` contains `CREATE TABLE alert (` and `payload JSONB NOT NULL` and `state TEXT NOT NULL DEFAULT 'firing'` and `is_test BOOLEAN NOT NULL DEFAULT FALSE` and partial unique index `alert_firing_unique_idx`
    - `internal/db/migrations/0040_retention_config_phase6.up.sql` contains `ADD COLUMN alerts_days INTEGER NOT NULL DEFAULT 365` and `ADD COLUMN audit_log_days INTEGER NOT NULL DEFAULT 1825`
    - `internal/db/migrations/0042_alert_worker_state.up.sql` contains `CREATE TABLE alert_worker_state (` and exactly 5 INSERT VALUES rows for worker_kind seeds
    - `internal/db/migrations/0043_admin_prune_audit_rows.up.sql` contains `SECURITY DEFINER` and `SET LOCAL session_replication_role = 'replica'` and `'audit.prune'` and `RETURN deleted_count`
    - `go test ./internal/db/... -count=1` passes
    - Direct `DELETE FROM audit_log` from a regular query still raises 'audit_log is INSERT-ONLY' (trigger intact); the function call succeeds
    - `internal/install/finish.go` references the alerts_days + audit_log_days columns: `grep "alerts_days\|audit_log_days" internal/install/finish.go` returns ≥ 1 line
  </acceptance_criteria>
  <done>All 5 new migrations apply and rollback cleanly. The SECURITY DEFINER function is the only code path that can DELETE from audit_log; the INSERT-ONLY invariant holds for every other caller.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 3: Alert engine substrate (engine.go, rule_store, alert_store, degraded subscriber, audit_prune worker)</name>
  <files>internal/alert/engine.go, internal/alert/rule_store.go, internal/alert/alert_store.go, internal/alert/degraded.go, internal/alert/worker_state.go, internal/alert/audit_prune_worker.go, internal/alert/engine_test.go, internal/alert/rule_store_test.go, internal/alert/alert_store_test.go, internal/alert/degraded_test.go, internal/alert/audit_prune_worker_test.go, internal/alert/doc.go, go.mod</files>
  <read_first>
    - internal/cli/serve.go (lines 320-360: existing River client + worker registration + PeriodicJob list)
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md §Decision C "Three Workers, Three Cadences, One Queue" + "Degraded Detection via River Subscribe"
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md §Decision F "Audit Retention Prune"
    - internal/audit/log.go (WriteEntry signature; ActionAuditPrune constant from Task 1)
    - go.mod (verify River v0.36.0 + riverpgxv5; add robfig/cron/v3 v3.0.1)
  </read_first>
  <behavior>
    - Test (TestEngineEvaluateContext_HoldsDeps): EvaluateContext struct exposes Pool, Queries, Hub, Audit, InstallTZ.
    - Test (TestRuleStore_CreateListEnableDisable): create rule, list returns it (active filter true), disable sets disabled_at, list with active filter excludes it.
    - Test (TestAlertStore_InsertAndIdempotentRefire): InsertAlert succeeds; a second InsertAlert for the same rule_id+target_entity_id while state='firing' fails with unique-constraint violation (alert_firing_unique_idx).
    - Test (TestAlertWorkerStateUpserter): UpsertAlertWorkerState updates the row + recorded duration_ms.
    - Test (TestDegradedSubscriber_FlipsOnDiscarded): given a worker that always returns error and InsertOpts{MaxAttempts:1}, after the job exhausts and reaches JobStateDiscarded, the subscriber flips `alert_worker_state.degraded=true` for that worker_kind.
    - Test (TestAuditPruneWorker_DeletesOldRows): seed 10 audit rows with time=now()-2000d, set audit_log_days=1825, run worker, assert 10 rows deleted + 1 'audit.prune' meta row inserted; the meta row is itself queryable.
    - Test (TestAuditPruneWorker_RejectsDirectDelete): direct `DELETE FROM audit_log` outside the function still raises 'INSERT-ONLY'.
  </behavior>
  <action>
    Run `go get github.com/robfig/cron/v3@v3.0.1` and commit go.mod / go.sum.

    Create `internal/alert/doc.go` with package doc explaining the three-worker / one-queue pattern.

    Create `internal/alert/engine.go`:
    ```go
    package alert

    import (
        "context"
        "log/slog"
        "time"
        "github.com/jackc/pgx/v5/pgxpool"
        "github.com/shifter-io/shifter/internal/events"
        sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
    )

    // EvaluateContext is the dependency bundle shared by every alert worker.
    // Held by the worker struct, NOT passed per-call (River's Work signature
    // is fixed). One instance per process; safe for concurrent worker calls
    // because each field is itself goroutine-safe.
    type EvaluateContext struct {
        Pool      *pgxpool.Pool
        Queries   *sqlc.Queries
        Hub       *events.Hub
        InstallTZ *time.Location  // D-RESEARCH-Q4: pulled per-eval-cycle from install_state
        Log       *slog.Logger
    }

    // CompareBound returns true if the value breaches either bound.
    // high_bound: value > high_bound = breach (when high_bound is non-nil)
    // low_bound:  value < low_bound  = breach (when low_bound is non-nil)
    // RuleKind="threshold_*" semantics; offline/anomaly evaluators use their own logic.
    func CompareBound(value float64, high *float64, low *float64) bool { ... }
    ```

    Create `internal/alert/rule_store.go` with methods (using pgxpool directly per existing internal/auth/users.go convention; sqlc later if needed):
    - `CreateRule(ctx, tx, params) (RuleRecord, error)` — insert + return created row
    - `ListActiveRulesByKind(ctx, kind) ([]RuleRecord, error)` — SELECT * FROM alert_rule WHERE rule_kind=$1 AND disabled_at IS NULL
    - `ListAllRules(ctx, includeDisabled bool) ([]RuleRecord, error)`
    - `GetRuleByID(ctx, id) (RuleRecord, error)`
    - `UpdateRule(ctx, tx, id, params) (RuleRecord, error)`
    - `DisableRule(ctx, tx, id) error` — sets disabled_at=now()
    - `EnableRule(ctx, tx, id) error` — sets disabled_at=NULL
    - `TouchLastFiredAt(ctx, tx, id) error` — UPDATE alert_rule SET last_fired_at=now()

    `RuleRecord` struct mirrors all columns of `alert_rule` (id, rule_kind, scope_kind, scope_id, high_bound, low_bound, comparison, unit, quiet_window_start, quiet_window_end, flow_threshold, days_of_week, severity, name, notes, cooldown_seconds, last_fired_at, disabled_at, created_by, created_at, updated_at).

    Create `internal/alert/alert_store.go`:
    - `InsertAlert(ctx, tx, params) (AlertRecord, error)` — returns ErrDuplicateFire if alert_firing_unique_idx violated (caller treats as success — idempotent fire)
    - `ClearAlert(ctx, tx, id) error` — sets state='cleared', cleared_at=now()
    - `ListFiringByRuleTarget(ctx, ruleID, targetEntityID) (*AlertRecord, error)`
    - `AckAlert(ctx, tx, id, userID, note) error`
    - `SnoozeAlert(ctx, tx, id, userID, until time.Time) error`
    - `MuteAlert(ctx, tx, id, userID) error`
    - `ListRecent(ctx, limit, statuses []string) ([]AlertRecord, error)` — for drawer (last 10)

    Create `internal/alert/worker_state.go`:
    - `RunState` struct (rulesEvaluated, firesEmitted, cleared, durationMs, firstErr) with `recordErr(err)` method
    - `UpsertWorkerState(ctx, tx, kind string, state *RunState) error` — UPDATE alert_worker_state SET ... WHERE worker_kind=$1
    - `MarkWorkerDegraded(ctx, kind, lastError string) error` — UPDATE alert_worker_state SET degraded=true, last_error=$2 WHERE worker_kind=$1

    Create `internal/alert/degraded.go`:
    ```go
    func StartDegradedSubscriber(ctx context.Context, riverClient *river.Client[pgx.Tx],
        worker *WorkerStateStore, log *slog.Logger) {
        sub, cancel := riverClient.Subscribe(river.EventKindJobFailed)
        go func() {
            defer cancel()
            for {
                select {
                case <-ctx.Done(): return
                case event, ok := <-sub:
                    if !ok { return }
                    j := event.Job
                    if j.State != rivertype.JobStateDiscarded { continue }
                    workerKind, ok := alertWorkerKindFromJobKind(j.Kind)
                    if !ok { continue }
                    log.Error("alert_worker_degraded", "kind", workerKind, "attempts", j.Attempt)
                    _ = worker.MarkWorkerDegraded(ctx, workerKind, lastError(j))
                }
            }
        }()
    }
    // alertWorkerKindFromJobKind maps River job Kind strings to
    // alert_worker_state.worker_kind enum values. Returns (kind, true) only for
    // alert worker job kinds; (_, false) means "not an alert worker — ignore".
    ```

    Create `internal/alert/audit_prune_worker.go` (D-51 + D-38):
    ```go
    type AuditPruneArgs struct{}
    func (AuditPruneArgs) Kind() string { return "audit_prune" }
    func (AuditPruneArgs) InsertOpts() river.InsertOpts {
        return river.InsertOpts{MaxAttempts: 3}
    }

    type AuditPruneWorker struct {
        river.WorkerDefaults[AuditPruneArgs]
        Pool    *pgxpool.Pool
        Queries *sqlc.Queries
        Log     *slog.Logger
    }

    func (w *AuditPruneWorker) Work(ctx context.Context, _ *river.Job[AuditPruneArgs]) error {
        // Read retention config
        var auditLogDays int32
        if err := w.Pool.QueryRow(ctx,
            `SELECT audit_log_days FROM retention_config WHERE id = 1`).Scan(&auditLogDays); err != nil {
            return fmt.Errorf("read retention_config: %w", err)
        }
        // Call the SECURITY DEFINER function (D-51); it handles meta-audit-row insert
        var deleted int
        if err := w.Pool.QueryRow(ctx,
            `SELECT admin_prune_audit_rows($1)`, auditLogDays).Scan(&deleted); err != nil {
            return fmt.Errorf("admin_prune_audit_rows: %w", err)
        }
        w.Log.Info("audit_prune.cycle", "deleted", deleted, "cutoff_days", auditLogDays)
        return nil
    }
    ```

    Tests in `_test.go` files use testcontainers-go/postgres + `db.RunMigrations` to spin up real DB. `TestAuditPruneWorker_DeletesOldRows` inserts 10 audit rows with `INSERT INTO audit_log (time, ...) VALUES (now()-INTERVAL '2000 days', ...)` (must satisfy the audit_log_action_valid CHECK — use existing vocab like 'create'); after the worker runs, `SELECT count(*) FROM audit_log WHERE action != 'audit.prune'` returns 0, `SELECT count(*) FROM audit_log WHERE action='audit.prune'` returns 1.

    Wire `StartDegradedSubscriber` + `AuditPruneWorker` registration in `internal/cli/serve.go` near the existing River setup (lines ~334-357). Add periodic job:
    ```go
    mustParseSchedule := func(spec string, args river.JobArgs) *river.PeriodicJob {
        sched, err := cron.ParseStandard(spec)
        if err != nil { panic(err) }
        return river.NewPeriodicJob(sched,
            func() (river.JobArgs, *river.InsertOpts) { return args, nil },
            nil)
    }
    // Pull install timezone from install_state for CRON_TZ prefix:
    installTZ, _ := ... // read from install_state
    periodicJobs = append(periodicJobs,
        mustParseSchedule("CRON_TZ="+installTZ+" 0 3 * * *", alert.AuditPruneArgs{}))
    ```

    Per RESEARCH §Decision C, the alert engine is in-engine-cool-down: workers read `LastFiredAt + CooldownSeconds < now()` from `RuleRecord` before evaluating. Test-fired alerts (D-19) DO NOT call `TouchLastFiredAt` — only worker-emitted fires do.
  </action>
  <verify>
    <automated>go test ./internal/alert/... -count=1 -timeout=120s</automated>
  </verify>
  <acceptance_criteria>
    - `internal/alert/engine.go` contains `type EvaluateContext struct {` and `Pool`, `Queries`, `Hub`, `InstallTZ *time.Location`, `Log *slog.Logger` fields
    - `internal/alert/rule_store.go` contains functions: `CreateRule`, `ListActiveRulesByKind`, `DisableRule`, `EnableRule`, `TouchLastFiredAt` (grep each function name)
    - `internal/alert/alert_store.go` contains: `InsertAlert`, `ClearAlert`, `AckAlert`, `SnoozeAlert`, `MuteAlert`, `ListFiringByRuleTarget`
    - `internal/alert/degraded.go` contains `func StartDegradedSubscriber(` and `rivertype.JobStateDiscarded`
    - `internal/alert/audit_prune_worker.go` contains `SELECT admin_prune_audit_rows($1)` and `MaxAttempts: 3`
    - `go.mod` contains `github.com/robfig/cron/v3 v3.0.1`
    - `internal/cli/serve.go` references `alert.AuditPruneArgs{}` and `cron.ParseStandard("CRON_TZ=`
    - `go test ./internal/alert/... -count=1` exits 0
    - Test `TestDegradedSubscriber_FlipsOnDiscarded` is in `internal/alert/degraded_test.go` and asserts `alert_worker_state.degraded = true` after a job reaches `rivertype.JobStateDiscarded`
    - Test `TestAuditPruneWorker_DeletesOldRows` is in `internal/alert/audit_prune_worker_test.go` and asserts both the row-deletion AND the 'audit.prune' meta-row insert
  </acceptance_criteria>
  <done>The alert engine substrate package builds, every store function has a test, the degraded subscriber flips the flag on discarded jobs, the audit prune worker calls the SECURITY DEFINER function and observes both effects (row deletion + meta audit row).</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| client→API | None in this plan (substrate only — no HTTP handlers added) |
| Postgres role boundary | `admin_prune_audit_rows` is SECURITY DEFINER → runs as table owner → can DELETE from audit_log; all other roles cannot |
| River cron tasks | run in same process as serve binary; trusted; no external input |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-06-01-01 | Tampering | audit_log INSERT-ONLY trigger bypass | mitigate | SECURITY DEFINER function `admin_prune_audit_rows` is the ONLY code path that can DELETE; `REVOKE ALL FROM PUBLIC` + `GRANT EXECUTE TO shifter` ensures only the Shifter DB role can invoke; function writes a meta `audit.prune` row INSIDE the same tx (auditable trail of every prune). Test: TestAuditPruneWorker_RejectsDirectDelete verifies the trigger still rejects regular DELETEs after the function returns. |
| T-06-01-02 | Tampering | SET LOCAL session_replication_role escape | mitigate | The replication-role override is `SET LOCAL` (released at COMMIT) inside a SECURITY DEFINER function — no external code path can call `SET session_replication_role` without first being inside this function's tx. Test: a SELECT after the function call shows `SHOW session_replication_role` returns 'origin'. |
| T-06-01-03 | Repudiation | audit prune itself untraceable | mitigate | The function INSERTs a meta `audit.prune` row with `notes='Pruned N rows older than YYYY-MM-DD'` AFTER setting replication_role back to 'origin' but inside the same tx — so the meta row is atomic with the DELETE and survives any future replay. Compliance reviewer can grep `WHERE action='audit.prune'` for full trail. |
| T-06-01-04 | DoS | runaway audit prune | mitigate | Worker runs at 03:00 install_tz daily (low traffic); River MaxAttempts=3 caps retries; `audit_log_days` has CHECK BETWEEN 90 AND 18250 to prevent operator from setting `cutoff_days=0` and wiping the entire table; the function also rejects `cutoff_days < 0`. |
| T-06-01-05 | Information Disclosure | alert_rule schema leaks scope_id of sensitive entities | accept | scope_id is a UUID reference to existing entities (site/MP/device/gateway); leakage requires existing DB access; AUTH-06 enforces same-role view. |
| T-06-01-06 | Tampering | River subscriber crashes on malformed event | mitigate | `StartDegradedSubscriber` is a goroutine with a context cancel; the deferred `cancel()` releases the subscription on shutdown; the alertWorkerKindFromJobKind helper returns false for unknown kinds so subscribed-to-but-unrelated job failures are ignored (no crash). Test: TestDegradedSubscriber_IgnoresUnrelatedJobs. |
</threat_model>

<verification>
- All 5 new migrations apply cleanly against a fresh PG16+TimescaleDB 2.26 testcontainer (Phase 1's `db.RunMigrations` helper)
- All down migrations revert cleanly (testcontainer-up → up → down → down → up sequence is idempotent)
- The audit_log INSERT-ONLY invariant survives this plan: only the SECURITY DEFINER function can DELETE
- River subscriber starts during `cmd/shifter serve` boot; no panic, no error
- AuditPruneWorker is registered with cron `0 3 * * *` (install_tz prefix); River dashboard shows scheduled job
- `go test ./internal/alert/... ./internal/audit/... ./internal/db/... ./internal/install/... -count=1` passes
</verification>

<success_criteria>
- Migrations 0037 / 0038 / 0039 / 0040 / 0042 / 0043 land in `internal/db/migrations/` (up + down)
- `internal/alert/` package exists with engine.go, rule_store.go, alert_store.go, degraded.go, worker_state.go, audit_prune_worker.go, doc.go and matching `_test.go` files
- `internal/audit/log.go` has 26 new Phase 6 action constants + 6 entity-type constants
- `internal/install/finish.go` references `alerts_days` and `audit_log_days` columns
- `internal/cli/serve.go` wires `StartDegradedSubscriber` and registers `AuditPruneWorker` + `cron 0 3 * * *` periodic job
- `go.mod` has `github.com/robfig/cron/v3 v3.0.1` (the only new Go dep this plan introduces)
</success_criteria>

<output>
After completion, create `.planning/phases/06-alerts-users-audit-operational-hardening/06-01-SUMMARY.md`
</output>
