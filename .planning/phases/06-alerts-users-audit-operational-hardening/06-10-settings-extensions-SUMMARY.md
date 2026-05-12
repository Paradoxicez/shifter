---
phase: 06-alerts-users-audit-operational-hardening
plan: 10
subsystem: settings
tags: [settings, backup, retention, rbac, frontend, migration]
dependency_graph:
  requires: [06-01, 06-08]
  provides: [SETT-05, retention-phase6-fields]
  affects: [internal/settings, web/src/components/settings, internal/db/migrations]
tech_stack:
  added: []
  patterns:
    - "Backup freshness dot with three-state color logic (ok/warn/crit)"
    - "SERIALIZABLE tx for threshold patch with audit trail"
    - "Partial PATCH for backup thresholds (load existing, merge supplied fields, re-validate)"
key_files:
  created:
    - internal/db/migrations/0047_backup_thresholds.up.sql
    - internal/db/migrations/0047_backup_thresholds.down.sql
    - internal/settings/backup_card.go
    - internal/settings/backup_card_test.go
    - internal/settings/retention_phase6_test.go
    - web/src/components/settings/BackupFreshnessDot.tsx
    - web/src/components/settings/BackupHistoryList.tsx
    - web/src/components/settings/BackupStatusCard.tsx
    - web/src/components/settings/InstallIdentityCard.tsx
    - web/src/components/settings/RestoreGuidanceCard.tsx
    - web/playwright/specs/backup-card.spec.ts
  modified:
    - internal/db/queries/settings.sql
    - internal/db/sqlc/settings.sql.go
    - internal/settings/retention.go
    - internal/settings/routes.go
    - internal/http/router.go
    - internal/cli/serve.go
    - internal/db/migrations_test.go
    - internal/db/roundtrip_test.go
    - web/src/components/settings/DataRetentionCard.tsx
    - web/src/routes/settings.tsx
    - web/src/routes/settings.test.tsx
decisions:
  - "Used migration 0047 (not 0046 as plan stated) because 0046_backup_run was already created by plan 06-08; Rule 3 deviation"
  - "BackupStatusCard threshold form shows save only when form is dirty; partial PATCH supported by load-merge-validate pattern"
  - "InstallIdentityCard added as a new component (D-47 propagation note) but /api/settings/identity endpoint is a stub pending plan 06-11"
metrics:
  duration: "~8 minutes"
  completed_date: "2026-05-12"
  tasks_completed: 2
  files_changed: 21
---

# Phase 06 Plan 10: Settings Extensions Summary

**One-liner:** Extended Settings with Phase 6 retention fields (alerts_days + audit_log_days), backup freshness card (SETT-05) with three-state dot + threshold editor, and restore guidance card — all with admin/viewer RBAC.

## Tasks Completed

| Task | Description | Commit |
|------|-------------|--------|
| 1 | Backend: migration 0047, retention Phase 6 fields, backup card handlers | 0e1b45a |
| 2 | Frontend: DataRetentionCard extensions, BackupStatusCard, RestoreGuidanceCard, tests | c045a9f |

## What Was Built

### Task 1 — Backend

**Migration 0047** (`0047_backup_thresholds`) adds `backup_warn_threshold_hours` (default 24) and `backup_crit_threshold_hours` (default 168) to `retention_config`, plus a `backup_warn_lt_crit` CHECK constraint.

**Retention Phase 6 fields** — `GetRetentionConfig` query extended to include the new threshold columns; `UpdateRetentionConfig` extended to accept `alerts_days` + `audit_log_days` via COALESCE nullable params. `retentionSnapshot`, `RetentionResponse`, `RetentionPatch`, `validatePatch`, `diffFields`, and `toResponse` all updated to carry the new fields. Validation ranges: `alerts_days` 30..3650, `audit_log_days` 90..18250.

**Backup card handlers** — `GET /api/settings/backup` (ActionBackupRead, admin + viewer) returns `never_run`, `last`, `recent` (up to 5), `warn_threshold_hours`, `crit_threshold_hours`, `destination_dir`. `PATCH /api/settings/backup/thresholds` (ActionBackupConfigure, admin only) applies partial patch with load-merge-validate pattern, writes audit row via `audit.WriteEntry`, commits in SERIALIZABLE tx.

**Router wiring** — `RegisterRoutesWithBackup` convenience wrapper added to `routes.go`; `router.go` updated to carry `BackupStore` + `BackupCardCfg` fields; `serve.go` wires `backup.NewStore(pool)` and `BackupCardConfig{BackupDir: cfg.BackupDir}`.

**Tests** (5 in `backup_card_test.go`, 5 in `retention_phase6_test.go`):
- `TestGetBackupStatus_NeverRun` — empty table returns `never_run=true` with correct threshold defaults
- `TestGetBackupStatus_PopulatedAndAge` — 3 seeded rows, verifies `never_run=false`, `last != nil`, `recent` length
- `TestPatchBackupThresholds_Validation` — 6 cases: valid, warn≥crit, warn=crit, warn≤0, crit too large
- `TestAuthz_BackupConfigure_AdminOnly` — viewer→403, admin→200
- `TestMigration0047_CreatesThresholdsColumns` — verifies column defaults after migration
- `TestGetRetention_IncludesPhase6Fields`, `TestPatchRetention_UpdatesAlertsDays`, `TestPatchRetention_RejectsOutOfRange`, `TestPatchRetention_UpdatesBothInSameTx`, `TestPatchRetention_AuditRow`

### Task 2 — Frontend

**DataRetentionCard** extended from 5 rows (raw/hourly/daily/monthly/yearly) to 7 (adds alerts + audit_log as editable rows). `RetentionConfig` interface, `EditableLevelKey`, and `RETENTION_LEVELS` array all updated.

**BackupFreshnessDot** — pure function `computeFreshnessStatus` drives three-state colour (green=ok, yellow=warn, red=crit). `data-status` attribute exposed for tests and Playwright assertions.

**BackupHistoryList** — collapsible table up to 5 rows. First 3 visible, remainder behind "Show all N" toggle. SHA256 truncated to 8 chars.

**BackupStatusCard** — TanStack Query fetch from `/api/settings/backup`. Renders freshness dot + age text + destination dir. Admin sees threshold form (react-hook-form + zod; save disabled until dirty). Recent history section renders when `recent.length > 0`.

**RestoreGuidanceCard** — static card with `target="_blank"` link to `/docs/restore`.

**InstallIdentityCard** — fetches `/api/settings/identity`; shows install_id, site_name, version. D-47 propagation note rendered for admin only. (Endpoint is a stub — see Known Stubs.)

**settings.tsx** — mounts `BackupStatusCard` + `RestoreGuidanceCard` below `DataRetentionCard`.

**Tests** — `settings.test.tsx` extended with 12 new tests:
- 4 `computeFreshnessStatus` unit tests (neverRun, ok, warn, crit branches)
- 4 `BackupStatusCard` render tests (neverRun state, ok state, admin form visible, viewer form hidden)
- Updated existing edit-button count assertion from 4 → 6

**Playwright** — `backup-card.spec.ts` covers admin card render, threshold form inputs, save flow (PATCH + toast), viewer read-only, restore guidance card.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Migration number conflict: plan claimed 0046, already taken by 06-08**
- **Found during:** Task 1 setup
- **Issue:** Plan 06-10 specified migration `0046_backup_thresholds` but `0046_backup_run` was created by plan 06-08. Using 0046 again would produce a duplicate migration filename and fail the migration runner.
- **Fix:** Used `0047_backup_thresholds` as the migration number. Migration version assertions in `migrations_test.go` and `roundtrip_test.go` bumped to `47`. Plan 06-11 must use 0048 as its first new migration number.
- **Files modified:** `internal/db/migrations/0047_backup_thresholds.up.sql`, `internal/db/migrations/0047_backup_thresholds.down.sql`, `internal/db/migrations_test.go`, `internal/db/roundtrip_test.go`
- **Commit:** 0e1b45a

**2. [Rule 2 - Missing validation] Backup threshold partial PATCH needs merged re-validation**
- **Found during:** Task 1 implementation
- **Issue:** If only `warn_threshold_hours` is supplied, the initial `validateBackupThresholds` call cannot check `warn < crit` because `crit` is nil. Need to load existing values and merge before final validation.
- **Fix:** `PatchBackupThresholdsHandler` loads current values first, applies supplied fields, then calls `validateBackupThresholds` again on the merged result.
- **Files modified:** `internal/settings/backup_card.go`
- **Commit:** 0e1b45a

**3. [Rule 1 - Bug] Test fixture missing Phase 6 retention fields caused duplicate text match**
- **Found during:** Task 2 vitest run
- **Issue:** `DataRetentionCard — value display` test used `getByText('90 days')` but with `alerts_days=90` also rendering "90 days", the query matched two elements.
- **Fix:** Scoped assertions to `getByTestId('retention-row-raw_days')` and `getByTestId('retention-row-hourly_days')` instead of bare text match.
- **Files modified:** `web/src/routes/settings.test.tsx`
- **Commit:** c045a9f

## Known Stubs

| Stub | File | Line | Reason |
|------|------|------|--------|
| `GET /api/settings/identity` endpoint | `web/src/components/settings/InstallIdentityCard.tsx` | 19 | Backend endpoint not implemented in this plan; `InstallIdentityCard` will show skeleton until plan 06-11 wires the identity endpoint |

The stub does not block the plan goal (backup status card + retention Phase 6 rows) — `InstallIdentityCard` renders gracefully in loading state and is not mounted in `settings.tsx` yet.

## Self-Check: PASSED

Files created/exist:
- `internal/db/migrations/0047_backup_thresholds.up.sql` — FOUND
- `internal/db/migrations/0047_backup_thresholds.down.sql` — FOUND
- `internal/settings/backup_card.go` — FOUND
- `internal/settings/backup_card_test.go` — FOUND
- `internal/settings/retention_phase6_test.go` — FOUND
- `web/src/components/settings/BackupFreshnessDot.tsx` — FOUND
- `web/src/components/settings/BackupHistoryList.tsx` — FOUND
- `web/src/components/settings/BackupStatusCard.tsx` — FOUND
- `web/src/components/settings/RestoreGuidanceCard.tsx` — FOUND
- `web/src/components/settings/InstallIdentityCard.tsx` — FOUND
- `web/playwright/specs/backup-card.spec.ts` — FOUND

Commits:
- `0e1b45a` feat(06-10): backend — FOUND
- `c045a9f` feat(06-10): frontend — FOUND

Tests: 378 passed, 0 failed (vitest)
