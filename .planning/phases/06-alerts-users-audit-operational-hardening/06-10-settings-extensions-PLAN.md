---
phase: 06-alerts-users-audit-operational-hardening
plan: 10
type: execute
wave: 2
depends_on: [06-01, 06-08]
files_modified:
  - internal/db/migrations/0046_backup_thresholds.up.sql
  - internal/db/migrations/0046_backup_thresholds.down.sql
  - internal/settings/retention.go
  - internal/settings/retention_test.go
  - internal/settings/backup_card.go
  - internal/settings/backup_card_test.go
  - internal/settings/identity.go
  - internal/settings/routes.go
  - internal/auth/authz.go
  - internal/http/router.go
  - web/src/routes/settings.tsx
  - web/src/components/settings/DataRetentionCard.tsx
  - web/src/components/settings/EditRetentionDialog.tsx
  - web/src/components/settings/BackupStatusCard.tsx
  - web/src/components/settings/BackupHistoryList.tsx
  - web/src/components/settings/BackupFreshnessDot.tsx
  - web/src/components/settings/RestoreGuidanceCard.tsx
  - web/src/components/settings/InstallIdentityCard.tsx
  - web/playwright/specs/backup-card.spec.ts
autonomous: true
requirements: [SETT-01, SETT-02, SETT-03, SETT-04, SETT-05]
must_haves:
  truths:
    - "Settings → Data Retention card adds 'Alerts' row (default 365 d) and 'Audit log' row (default 5 y / 1825 d) alongside Phase 5's existing raw/hourly/daily/monthly/yearly rows"
    - "PATCH /api/settings/retention accepts alerts_days + audit_log_days fields; reconciles in same tx as Phase 5 retention update; CHECK constraints in migration 0040 enforce 30..3650 and 90..18250"
    - "Settings → Backup card displays: last backup age (relative + absolute on hover), destination path (monospace + copy-to-clipboard), freshness dot (green/yellow/red per warn/crit thresholds, defaults 24h/168h, operator-editable), 'Run backup now' button, expandable recent-5 history with sha256, 'Configure schedule' link to operator runbook"
    - "POST /api/settings/backup/thresholds accepts {warn_threshold_hours, crit_threshold_hours} with validation warn < crit, both > 0; persisted in retention_config or a new singleton config table"
    - "Install Identity Settings card adds the SETT-02 propagation note: 'Changes apply to future reports.' (D-47)"
    - "Phase 1 retention reconciliation flow extended to write new alerts_days + audit_log_days in same tx as raw/hourly/daily/etc."
    - "Viewer (RoleViewer) sees retention + backup-status + identity values but cannot PATCH/POST (RBAC 403)"
  artifacts:
    - path: internal/settings/retention.go
      provides: "Retention struct + RetentionResponse extended with alerts_days + audit_log_days fields; PATCH handler extended to accept them"
    - path: internal/settings/backup_card.go
      provides: "GET /api/settings/backup status endpoint + POST /api/settings/backup/thresholds threshold setter"
    - path: web/src/components/settings/BackupStatusCard.tsx
      provides: "SETT-05 Backup card matching UI-SPEC §Surface 7"
    - path: web/src/components/settings/DataRetentionCard.tsx
      provides: "Updated to render 7 rows (raw/hourly/daily/monthly/yearly/alerts/audit_log)"
  key_links:
    - from: internal/settings/retention.go::PATCHHandler
    - to: internal/db/migrations/0040_retention_config_phase6.up.sql (Plan 06-01 added alerts_days + audit_log_days columns)
      via: "PATCH UPDATE includes both new columns inside the same SERIALIZABLE tx as Phase 5 retention reconciliation"
      pattern: "alerts_days.*audit_log_days"
    - from: web/src/components/settings/BackupStatusCard.tsx
      to: /api/backup/last + /api/backup/list + /api/settings/backup
      via: "React Query polls /api/backup/last every 60s; renders freshness dot using thresholds from /api/settings/backup"
      pattern: "useBackupLast"
---

<objective>
Wire the Settings page to expose every Phase 6 setting in one place: extend the Phase 5 Data Retention card with Alerts + Audit log rows; add the Backup status card (SETT-05); add the install-identity-propagation note (SETT-02). Closes SETT-02, SETT-05, and adds the Phase 6 retention rows to the existing Phase 5 SETT-04 card. Verifies SETT-01 (categorization) and SETT-03 (ChirpStack creds, already shipped Phase 1) by ensuring the settings page renders the full Phase 6 categorized layout per UI-SPEC §Surface 7.

Purpose: every Phase 6 surface (alerts, audit, backups) has an admin-controlled retention + monitoring knob. This plan ships those knobs as Settings rows + cards.

Output: extended retention handler/UI + Backup status card + Identity propagation note + RBAC + Playwright E2E.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/06-alerts-users-audit-operational-hardening/06-CONTEXT.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-UI-SPEC.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-01-alert-engine-substrate-PLAN.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-08-backup-cli-cron-PLAN.md
@internal/settings/retention.go
@internal/settings/routes.go
@internal/auth/authz.go
@internal/http/router.go
@web/src/routes/settings.tsx
@web/src/components/settings/DataRetentionCard.tsx
@web/src/components/settings/EditRetentionDialog.tsx
@web/src/components/settings/InstallIdentityCard.tsx

<interfaces>
Plan 06-01 migration 0040 added retention_config columns:
- alerts_days INTEGER NOT NULL DEFAULT 365 CHECK (alerts_days BETWEEN 30 AND 3650)
- audit_log_days INTEGER NOT NULL DEFAULT 1825 CHECK (audit_log_days BETWEEN 90 AND 18250)

Existing internal/settings/retention.go (Plan 05-11) — types:
```go
type RetentionResponse struct {
    RawDays     int32  `json:"raw_days"`
    HourlyDays  int32  `json:"hourly_days"`
    DailyDays   int32  `json:"daily_days"`
    MonthlyDays int32  `json:"monthly_days"`
    YearlyDays  *int32 `json:"yearly_days"`
    UpdatedAt   string `json:"updated_at"`
}
type RetentionPatch struct {
    RawDays       *int32 `json:"raw_days"`
    HourlyDays    *int32 `json:"hourly_days"`
    DailyDays     *int32 `json:"daily_days"`
    MonthlyDays   *int32 `json:"monthly_days"`
    YearlyDays    *int32 `json:"yearly_days"`
    YearlyForever *bool  `json:"yearly_forever"`
}
```

Plan 06-08 added:
- ActionBackupRead (admin+viewer)
- ActionBackupRun (admin only)
- ActionBackupConfigure (admin only)
- /api/backup/list, /api/backup/last endpoints

Plan 05-11 frontend already mounts the Settings page with cards: Install Identity / ChirpStack / Units / Timezone / Data Retention.

Plan 06-04 sidebar listed Settings sub-tabs: Identity / ChirpStack / Units / Timezone / Alerts (Plan 06-04) / Users (Plan 06-05) / Data retention / Backup (this plan).

UI-SPEC §Surface 7 (Backup card layout) + §Empty States "Backup never run" + freshness-dot color rules.
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Extend retention.go with alerts_days + audit_log_days fields; PATCH validation; Backup card backend (status + thresholds)</name>
  <files>internal/settings/retention.go, internal/settings/retention_test.go, internal/settings/backup_card.go, internal/settings/backup_card_test.go, internal/settings/identity.go, internal/settings/routes.go, internal/auth/authz.go, internal/http/router.go, internal/db/migrations/0046_backup_thresholds.up.sql, internal/db/migrations/0046_backup_thresholds.down.sql</files>
  <read_first>
    - internal/settings/retention.go (current implementation pattern)
    - internal/settings/retention_test.go (Phase 5 test pattern; this plan extends it)
    - internal/db/migrations/0029_retention_config.up.sql + 0040_retention_config_phase6.up.sql (Plan 06-01 alters)
    - internal/backup/store.go (Plan 06-08: Last + ListRecent methods)
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-CONTEXT.md §D-46 (Backup card spec)
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-UI-SPEC.md §Surface 7
  </read_first>
  <behavior>
    - Test (TestGetRetention_IncludesPhase6Fields): GET /api/settings/retention response JSON contains `alerts_days: 365` and `audit_log_days: 1825` after install_finish defaults run.
    - Test (TestPatchRetention_UpdatesAlertsDays): PATCH `{alerts_days: 730}` → row updated; CHECK constraint accepts 730 (within 30..3650).
    - Test (TestPatchRetention_RejectsOutOfRange): PATCH `{alerts_days: 10000}` → 422 (server-side validation before DB; CHECK as backup).
    - Test (TestPatchRetention_UpdatesBothInSameTx): single PATCH `{alerts_days: 730, audit_log_days: 730}` → atomic SERIALIZABLE tx; both columns updated.
    - Test (TestPatchRetention_AuditRow): PATCH writes audit row 'retention.update' with Before/After diff showing changed alerts_days + audit_log_days fields (reuses Phase 5 audit pattern).
    - Test (TestGetBackupStatus_NeverRun): no backup_run rows → GET /api/settings/backup returns `{never_run: true, warn_threshold_hours: 24, crit_threshold_hours: 168, destination_dir: "/var/lib/shifter/backups", recent: []}`.
    - Test (TestGetBackupStatus_PopulatedAndAge): with backup_run rows seeded → response includes `last: {file_name, size_bytes, sha256, started_at, finished_at, age_seconds}` + `recent: [...5 most recent...]`.
    - Test (TestPatchBackupThresholds_Validation): warn=24, crit=168 → 200. warn=200, crit=100 (warn >= crit) → 422. warn=-1 → 422. both must be int.
    - Test (TestAuthz_BackupConfigure_AdminOnly): viewer PATCH /api/settings/backup/thresholds → 403; admin → 200.
    - Test (TestMigration0046_CreatesThresholdsRow): new migration creates a singleton row in retention_config (or new table) with backup_warn_threshold_hours + backup_crit_threshold_hours columns; defaults 24 + 168.
  </behavior>
  <action>
    **Migration coordination:** Plan 06-10 claims migration `0046` (Plan 06-01 owns 0037-0043, Plan 06-05 owns 0044, Plan 06-08 owns 0045, Plan 06-11 owns 0047).

    **Migration 0046_backup_thresholds.up.sql:** Extend `retention_config` with the two new columns (singleton pattern):
    ```sql
    ALTER TABLE retention_config
        ADD COLUMN backup_warn_threshold_hours INTEGER NOT NULL DEFAULT 24 CHECK (backup_warn_threshold_hours > 0 AND backup_warn_threshold_hours <= 8760),
        ADD COLUMN backup_crit_threshold_hours INTEGER NOT NULL DEFAULT 168 CHECK (backup_crit_threshold_hours > 0 AND backup_crit_threshold_hours <= 8760),
        ADD CONSTRAINT backup_warn_lt_crit CHECK (backup_warn_threshold_hours < backup_crit_threshold_hours);
    COMMENT ON COLUMN retention_config.backup_warn_threshold_hours IS 'D-46: yellow age threshold (Settings Backup card). Default 24h.';
    COMMENT ON COLUMN retention_config.backup_crit_threshold_hours IS 'D-46: red age threshold. Default 168h (7d).';
    ```

    Down migration: ALTER TABLE retention_config DROP COLUMN (×2) + DROP CONSTRAINT.

    **internal/settings/retention.go:** Extend the existing Plan 05-11 file:
    ```go
    type RetentionResponse struct {
        RawDays      int32  `json:"raw_days"`
        HourlyDays   int32  `json:"hourly_days"`
        DailyDays    int32  `json:"daily_days"`
        MonthlyDays  int32  `json:"monthly_days"`
        YearlyDays   *int32 `json:"yearly_days"`
        // Phase 6 additions:
        AlertsDays   int32  `json:"alerts_days"`     // D-13 default 365
        AuditLogDays int32  `json:"audit_log_days"`  // D-38 default 1825
        UpdatedAt    string `json:"updated_at"`
    }
    type RetentionPatch struct {
        RawDays       *int32 `json:"raw_days"`
        HourlyDays    *int32 `json:"hourly_days"`
        DailyDays     *int32 `json:"daily_days"`
        MonthlyDays   *int32 `json:"monthly_days"`
        YearlyDays    *int32 `json:"yearly_days"`
        YearlyForever *bool  `json:"yearly_forever"`
        // Phase 6 additions:
        AlertsDays    *int32 `json:"alerts_days"`
        AuditLogDays  *int32 `json:"audit_log_days"`
    }
    ```

    Update the sqlc `GetRetentionConfig` query (or pgxpool raw SQL in retention.go) to SELECT alerts_days, audit_log_days. Update the PATCH handler:
    1. Validate the new fields: `if patch.AlertsDays != nil && (*patch.AlertsDays < 30 || *patch.AlertsDays > 3650) → 422`
    2. Validate `audit_log_days` similarly (90..18250)
    3. Inside the existing SERIALIZABLE tx, UPDATE retention_config SET ..., alerts_days = $N, audit_log_days = $M WHERE id = 1
    4. The Phase 5 reconciliation against TimescaleDB policies does NOT apply to alerts or audit_log (these aren't hypertables; they're pruned via Plan 06-01's AuditPruneWorker + a similar alerts prune worker which doesn't exist yet — flag a TODO note: "Plan 06-03 ran out of budget for an alerts retention prune worker; the alerts_days config is consumed by the alerts-prune worker added in Plan 06-11 task 1.")

    **Plan adjustment:** add the alerts-prune worker into Plan 06-11 task 1, NOT this plan. The retention_config row is the input; the prune worker is the consumer.

    **internal/settings/backup_card.go (NEW FILE):**
    ```go
    package settings

    type BackupStatusResponse struct {
        NeverRun             bool                     `json:"never_run"`
        Last                 *BackupSummary           `json:"last,omitempty"`
        Recent               []BackupSummary          `json:"recent"`
        WarnThresholdHours   int32                    `json:"warn_threshold_hours"`
        CritThresholdHours   int32                    `json:"crit_threshold_hours"`
        DestinationDir       string                   `json:"destination_dir"`
    }

    type BackupSummary struct {
        ID             string  `json:"id"`
        FileName       string  `json:"file_name"`
        SizeBytes      int64   `json:"size_bytes"`
        SHA256         string  `json:"sha256"`
        StartedAt      string  `json:"started_at"`
        FinishedAt     *string `json:"finished_at"`
        Status         string  `json:"status"`
        AgeSeconds     int64   `json:"age_seconds"`
        TriggerKind    string  `json:"trigger_kind"`
    }

    // GetBackupStatusHandler — admin+viewer (D-46 viewer sees status; cannot edit thresholds or run-now).
    func GetBackupStatusHandler(deps Deps) http.HandlerFunc { ... }
    // PatchBackupThresholdsHandler — admin only.
    func PatchBackupThresholdsHandler(deps Deps) http.HandlerFunc { ... }
    ```

    Wire to internal/backup/store.go's `ListRecent(5)` + `Last()` + the retention_config row for thresholds + cfg.BackupDir.

    PatchBackupThresholdsHandler:
    1. Parse body `{warn_threshold_hours: int, crit_threshold_hours: int}`
    2. Validate: warn > 0 && crit > 0 && warn < crit; both ≤ 8760 (1 year)
    3. SERIALIZABLE tx → UPDATE retention_config SET backup_warn_threshold_hours = $1, backup_crit_threshold_hours = $2 WHERE id = 1
    4. Audit row `retention.update` with Before/After diff
    5. Commit

    **internal/auth/authz.go:** Plan 06-08 added `ActionBackupConfigure` — confirm it's in this plan's reads + properly RBAC-gates the PATCH route.

    **internal/http/router.go:**
    ```go
    r.With(auth.RequireAction(sm, auth.ActionBackupRead)).Get("/api/settings/backup", settings.GetBackupStatusHandler(deps))
    r.With(auth.RequireAction(sm, auth.ActionBackupConfigure)).Patch("/api/settings/backup/thresholds", settings.PatchBackupThresholdsHandler(deps))
    ```

    **internal/settings/identity.go (NEW or extend existing file):** add a simple `IdentityPropagationNoteResponse` that the frontend reads — or just have the frontend render the static D-47 copy "Changes apply to future reports." The simplest: no backend change; the frontend renders the copy directly.
  </action>
  <verify>
    <automated>go test ./internal/settings/... ./internal/auth/... -run "TestGetRetention_IncludesPhase6|TestPatchRetention|TestGetBackupStatus|TestPatchBackupThresholds|TestAuthz_BackupConfigure|TestMigration0046" -count=1 -timeout=90s</automated>
  </verify>
  <acceptance_criteria>
    - `internal/db/migrations/0046_backup_thresholds.up.sql` contains both `backup_warn_threshold_hours INTEGER` and `backup_crit_threshold_hours INTEGER` columns + the `backup_warn_lt_crit` CHECK constraint
    - `internal/settings/retention.go` `RetentionResponse` struct has `AlertsDays int32` AND `AuditLogDays int32` fields (with json tags)
    - `internal/settings/retention.go` `RetentionPatch` struct has `AlertsDays *int32` AND `AuditLogDays *int32` fields
    - PATCH handler validates ranges: grep for `< 30` AND `> 3650` (alerts_days range) AND `< 90` AND `> 18250` (audit_log_days range)
    - `internal/settings/backup_card.go` exists with `GetBackupStatusHandler` and `PatchBackupThresholdsHandler`
    - `internal/http/router.go` mounts `/api/settings/backup` (GET) and `/api/settings/backup/thresholds` (PATCH) with correct RequireAction guards
    - All 10 listed tests pass: `go test ./internal/settings/... ./internal/auth/... -count=1 -timeout=90s` exits 0
    - `TestPatchBackupThresholds_Validation` asserts warn>=crit returns 422
  </acceptance_criteria>
  <done>Backend exposes every Phase 6 setting: alerts retention, audit retention, backup status, backup thresholds. Server-side validation aligned with DB CHECK constraints.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Frontend — extend DataRetentionCard, add BackupStatusCard + RestoreGuidanceCard + Install Identity note + Playwright E2E</name>
  <files>web/src/routes/settings.tsx, web/src/components/settings/DataRetentionCard.tsx, web/src/components/settings/EditRetentionDialog.tsx, web/src/components/settings/BackupStatusCard.tsx, web/src/components/settings/BackupHistoryList.tsx, web/src/components/settings/BackupFreshnessDot.tsx, web/src/components/settings/RestoreGuidanceCard.tsx, web/src/components/settings/InstallIdentityCard.tsx, web/playwright/specs/backup-card.spec.ts</files>
  <read_first>
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-UI-SPEC.md §Surface 7 (Backup status + Restore guidance) — copy verbatim
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-UI-SPEC.md §Backup freshness dot (color rules)
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-UI-SPEC.md §Copywriting Contract (CTA labels) and §Toast Notifications (verbatim toast strings)
    - web/src/components/settings/DataRetentionCard.tsx (existing Plan 05-11 implementation)
    - web/src/routes/settings.tsx (Settings page layout; add Backup card below Data Retention)
  </read_first>
  <behavior>
    - Test (DataRetentionCard_RendersAllSevenRows): renders rows for raw, hourly, daily, monthly, yearly, alerts, audit_log; each row shows current value + Edit button.
    - Test (EditRetentionDialog_AlertsRow): clicking Edit on Alerts row → dialog with number input default 365, min 30, max 3650; Save calls PATCH /api/settings/retention with {alerts_days: N}.
    - Test (EditRetentionDialog_AuditLogRow): same for audit_log row; range 90..18250.
    - Test (BackupStatusCard_FreshDot): mocked /api/settings/backup returns last.age_seconds = 7200, warn=24*3600, crit=168*3600 → freshness dot is green; tooltip "Backup status: fresh (2 hours)".
    - Test (BackupStatusCard_StaleDot): age = 36h, warn=24h, crit=168h → yellow dot; tooltip "Backup status: stale (36 hours, warn threshold 24h)".
    - Test (BackupStatusCard_CriticalDot): age = 200h → red dot.
    - Test (BackupStatusCard_NeverRun): never_run=true → red dot + "Last backup: Never" + "Run a backup now to get started.".
    - Test (BackupStatusCard_RunNowButton): clicking calls POST /api/backup/run-now → button disabled + spinner ("Backing up…"); after success: green dot + sonner toast "Backup completed — {size} written to {destination}".
    - Test (BackupStatusCard_RunNowFail): mock 500 from /api/backup/run-now → red dot + destructive toast "Backup failed: {reason}." + sticky Alert until dismissed.
    - Test (BackupStatusCard_ThresholdInputsValidation): warn=200 crit=100 → Save button disabled + inline error "warn must be < crit".
    - Test (BackupStatusCard_ViewerReadOnly): viewer → status + history visible; threshold inputs disabled with helper "Read-only — viewer role"; "Run backup now" button hidden.
    - Test (BackupHistoryList_TopFive): renders max 5 rows from /api/backup/list; expand shows full sha256; collapsed shows truncated `7a3f...c92b`.
    - Test (RestoreGuidanceCard_StaticDocLink): renders "Restoring a backup requires Shifter to be stopped." + "Open operator runbook ↗" link with `target="_blank"`.
    - Test (InstallIdentityCard_PropagationNote): renders "Changes apply to future reports." note below the form fields (D-47).
    - Test (Settings_SidebarSubtabs): rendering the Settings page shows tabs/sub-routes for Identity / ChirpStack / Units / Timezone / Alerts / Users / Data retention / Backup.
    - Playwright (backup-card.spec.ts): admin logs in → Settings → Backup → never_run state visible → clicks Run backup now → sees spinner → waits for completion → freshness dot green → sha256 displayed in history → expand row → full sha256 visible.
  </behavior>
  <action>
    **web/src/components/settings/DataRetentionCard.tsx:** Extend the existing card to render 7 rows. The new rows:
    - "Alerts" — default 365 d — Edit button opens EditRetentionDialog with `field="alerts_days"`, range [30, 3650]
    - "Audit log" — default 1825 d (5 y) — `field="audit_log_days"`, range [90, 18250]
    Match the existing row component visual + Edit action. Tooltip on the field name explains the retention semantics.

    **web/src/components/settings/EditRetentionDialog.tsx:** Add cases for the two new fields in the existing edit dialog logic. Validation: react-hook-form + zod with min/max per field.

    **web/src/components/settings/BackupFreshnessDot.tsx (NEW):**
    ```tsx
    type Props = { ageSeconds: number; warnHours: number; critHours: number; neverRun?: boolean };
    export function BackupFreshnessDot({ ageSeconds, warnHours, critHours, neverRun }: Props) {
      if (neverRun) return <Dot color="destructive" ariaLabel="Backup status: never run" />;
      const ageHours = ageSeconds / 3600;
      if (ageHours <= warnHours) return <Dot color="success" ariaLabel={`Backup status: fresh (${formatAge(ageSeconds)})`} />;
      if (ageHours <= critHours) return <Dot color="warning" ariaLabel={`Backup status: stale (${formatAge(ageSeconds)}, warn threshold ${warnHours}h)`} />;
      return <Dot color="destructive" ariaLabel={`Backup status: critical (${formatAge(ageSeconds)}, crit threshold ${critHours}h)`} />;
    }
    ```

    **web/src/components/settings/BackupStatusCard.tsx (NEW):** layout per UI-SPEC §Surface 7. State machine:
    - useQuery `['settings','backup']` → /api/settings/backup; refetch every 60s
    - Mutation `useRunBackupNow()` → POST /api/backup/run-now; on success: invalidate `['settings','backup']` + `['backup','list']` + sonner success toast; on error: destructive toast + sticky Alert
    - Threshold form: react-hook-form with zod refining `warn < crit, both > 0, both <= 8760`; Save button calls PATCH /api/settings/backup/thresholds
    - Viewer: read-only (no thresholds form, no Run now button; show "Read-only — viewer role" tooltips)

    **web/src/components/settings/BackupHistoryList.tsx (NEW):** Collapsible (shadcn) listing the top 5 backups from /api/backup/list. Each row: filename (mono), size (KiB/MiB formatted), status (✓/✗ icon), started → finished timestamps, sha256 (truncated with click-to-copy + "Show full" expander).

    **web/src/components/settings/RestoreGuidanceCard.tsx (NEW):** read-only card per UI-SPEC §Surface 7 second sub-card. Two paragraphs verbatim + "Open operator runbook ↗" link (`Button variant="link"` with `ExternalLink` icon, `target="_blank"`, href to the runbook URL — for now point to a relative path like `/docs/operator-runbook#backup-restore` if there's no static doc server; otherwise an external HTML rendering).

    **web/src/components/settings/InstallIdentityCard.tsx:** extend the existing card to render an `<Alert variant="info">` below the form fields: "Changes apply to future reports." per D-47 + UI-SPEC §Surface 7 (referenced inside the broader Settings layout).

    **web/src/routes/settings.tsx:** Mount the new BackupStatusCard + RestoreGuidanceCard below DataRetentionCard. Settings page layout order (per UI-SPEC §Sidebar nav): Identity → ChirpStack → Units → Timezone → Alerts (link to /settings/alerts) → Users (link to /settings/users) → Data retention → Backup.

    Use shadcn `Tabs` for sub-tabs OR scroll-anchored sections — match the existing Plan 05-11 settings page pattern (one long scrolling page with anchor-jumps is the Phase 5 pattern; preserve it).

    **web/playwright/specs/backup-card.spec.ts:**
    ```ts
    test('backup status card cycle', async ({ page }) => {
      await loginAsAdmin(page);
      await page.goto('/settings');
      await page.locator('text=Backup').scrollIntoViewIfNeeded();
      // Never run initially (fresh install)
      await expect(page.locator('[data-testid=backup-status-card]')).toContainText('No backups yet');
      // Run backup now
      const runBtn = page.locator('text=Run backup now');
      await runBtn.click();
      await expect(runBtn).toBeDisabled(); // spinner state
      // Wait for success toast (poll-based; up to 2 min)
      await expect(page.locator('text=Backup completed')).toBeVisible({ timeout: 120_000 });
      // Freshness dot is now green
      await expect(page.locator('[data-testid=backup-freshness-dot]')).toHaveAttribute('data-color', 'success');
      // History list has one entry
      await page.locator('text=Recent backups').click(); // expand collapsible
      await expect(page.locator('[data-testid=backup-history-row]')).toHaveCount(1);
    });
    ```
  </action>
  <verify>
    <automated>pnpm -C web test --run web/src/components/settings web/src/routes/settings.test.tsx && pnpm -C web exec playwright test backup-card</automated>
  </verify>
  <acceptance_criteria>
    - `web/src/components/settings/DataRetentionCard.tsx` renders 7 retention rows (grep for `alerts_days` + `audit_log_days` in JSX or row config)
    - `web/src/components/settings/BackupStatusCard.tsx` exists and uses `useQuery(['settings','backup']...)`
    - `BackupStatusCard.tsx` renders three states: never_run / fresh / stale / critical per UI-SPEC §Backup freshness dot
    - `web/src/components/settings/BackupFreshnessDot.tsx` exists with three color states and aria-labels matching UI-SPEC §Accessibility
    - `web/src/components/settings/BackupHistoryList.tsx` exists and renders max 5 rows in a Collapsible
    - `web/src/components/settings/RestoreGuidanceCard.tsx` exists with "Open operator runbook ↗" link (`target="_blank"` + ExternalLink icon)
    - `web/src/components/settings/InstallIdentityCard.tsx` includes the D-47 note "Changes apply to future reports."
    - All 14 component tests pass: `pnpm -C web test --run web/src/components/settings` exits 0
    - Playwright spec passes: `pnpm -C web exec playwright test backup-card` exits 0
  </acceptance_criteria>
  <done>Settings exposes every Phase 6 control: alerts/audit retention, backup status with freshness dot, Run backup now, history list, thresholds editor, restore guidance card, identity propagation note.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| client→/api/settings/* | Admin can mutate; viewer can read |
| backup thresholds → freshness rendering | Read by viewer too; if poisoned, only affects visual signal (not data) |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-06-10-01 | Elevation of Privilege | viewer changes retention to 0 days (data wipe) | mitigate | PATCH /api/settings/retention is RBAC-gated by ActionSettingsUpdate (admin only, Phase 5 D-25 RBAC). Test (Phase 5 + Plan 06-10 retention): TestRBAC_ViewerPatchRetention403. |
| T-06-10-02 | Tampering | viewer changes backup thresholds to mask staleness | mitigate | PATCH /api/settings/backup/thresholds is ActionBackupConfigure (admin only). Test: TestAuthz_BackupConfigure_AdminOnly. |
| T-06-10-03 | DoS | retention set to absurdly low value | mitigate | DB CHECK constraints enforce ranges (alerts 30..3650, audit_log 90..18250, raw 30..365, etc.); server-side validation also rejects out-of-range. |
| T-06-10-04 | Information Disclosure | viewer sees backup sha256 + filenames | accept | The sha256 + filename are operator-internal artifacts; viewer needs them to file support tickets (per D-46 "viewer sees status + history"). The actual tarball is on a filesystem the viewer doesn't have access to. |
| T-06-10-05 | Spoofing | freshness dot tampered via client-side time skew | accept | Age is server-computed (now() - finished_at) and returned in seconds; client renders. Skew impacts rendering only. |
| T-06-10-06 | Tampering | warn-threshold and crit-threshold violate `warn < crit` | mitigate | DB CHECK constraint `backup_warn_lt_crit` + server-side zod validation + client-side form refinement. Three layers. |
</threat_model>

<verification>
- All SETT-* requirements expose UI:
  - SETT-01 (categorization) — preserved
  - SETT-02 (identity propagation note) — added
  - SETT-03 (ChirpStack creds) — unchanged from Phase 1
  - SETT-04 (retention for Phase 5 levels) — preserved
  - SETT-05 (most-recent-backup + threshold) — added
- D-13 alerts retention row + D-38 audit_log retention row both shipped
- D-46 Backup card + age dot + thresholds + Run-now + recent-5 list
- D-47 identity propagation note
- All listed tests pass
</verification>

<success_criteria>
- SETT-02 covered: install identity edits surface "Changes apply to future reports." note
- SETT-05 covered: Settings → Backup card shows last age + status, configurable thresholds, history list, Run backup now
- SETT-01 + SETT-03 + SETT-04 preserved (already complete from earlier phases)
- D-13, D-38, D-46, D-47 implemented end-to-end
</success_criteria>

<output>
After completion, create `.planning/phases/06-alerts-users-audit-operational-hardening/06-10-SUMMARY.md`
</output>
