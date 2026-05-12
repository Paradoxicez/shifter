---
phase: 06-alerts-users-audit-operational-hardening
plan: 12
subsystem: settings
tags: [settings, identity, rbac, audit-in-tx, frontend, migration, gap-closure, SETT-01, SETT-02]
dependency_graph:
  requires: [06-10, 06-11]
  provides: [SETT-01, SETT-02]
  affects:
    - internal/db/migrations
    - internal/audit/log.go
    - internal/auth/authz.go
    - internal/settings
    - web/src/components/settings/InstallIdentityCard.tsx
    - web/src/routes/settings.tsx
tech_stack:
  added: []
  patterns:
    - "load-merge-validate PATCH pattern (mirrors backup_card.go PatchBackupThresholdsHandler)"
    - "Serializable tx + audit.WriteEntry in same atomic unit (D-30 pattern)"
    - "Admin-only ResponsiveDialog with react-hook-form + zod for identity edit"
    - "isTxSerializationFailure helper for 40001 detection in settings package"
key_files:
  created:
    - internal/db/migrations/0049_audit_vocab_identity.up.sql
    - internal/db/migrations/0049_audit_vocab_identity.down.sql
    - internal/settings/identity.go
    - internal/settings/identity_test.go
  modified:
    - internal/audit/log.go
    - internal/auth/authz.go
    - internal/settings/routes.go
    - internal/db/migrations_test.go
    - internal/db/roundtrip_test.go
    - web/src/components/settings/InstallIdentityCard.tsx
    - web/src/routes/settings.tsx
decisions:
  - "GET /api/settings/identity gated by ActionConnectionTest (any authed user) to match viewer read-only access model"
  - "isTxSerializationFailure defined in identity.go (not shared) — backup_card.go and retention.go do not handle 40001, so no shared helper is warranted"
  - "InstallIdentityCard uses dual-fragment render (<> + EditIdentityDialog) rather than Portal to keep dialog lifecycle tied to card mount"
  - "EditIdentityDialog invalidates query on success via qc.invalidateQueries — card refetches from backend, no optimistic state"
metrics:
  duration_minutes: 12
  completed_date: "2026-05-12"
  tasks_completed: 2
  tasks_total: 2
  new_files: 4
  modified_files: 7
requirements_addressed:
  - SETT-01
  - SETT-02
---

# Phase 06 Plan 12: Install Identity Surface Summary

**One-liner:** Migration 0049 extends audit CHECK constraints + GET/PATCH /api/settings/identity handlers (Serializable tx + audit-in-tx) + admin edit dialog in InstallIdentityCard, closing SETT-01/SETT-02 gaps from Phase 6 verification.

## Objective

Close SETT-02 (admin can update install identity at any time) and SETT-01 (partial — install identity category absent from Settings page). Both gaps traced to a single missing file: `internal/settings/identity.go`.

## Tasks Completed

| Task | Description | Commit | Key Files |
|------|-------------|--------|-----------|
| 1 | Migration 0049 + auth constants + identity.go backend + tests | 29fb5a5 | 0049_*.sql, identity.go, identity_test.go, authz.go, routes.go |
| 2 | Mount InstallIdentityCard + admin EditIdentityDialog in frontend | a5d6a78 | InstallIdentityCard.tsx, settings.tsx |

## What Was Built

### Task 1 — Backend

**Migration 0049** (`0049_audit_vocab_identity`) extends both `audit_log` CHECK constraints using the established DROP+re-ADD pattern:
- `audit_log_action_valid`: adds `'settings.identity_update'`
- `audit_log_entity_type_valid`: adds `'install_identity'`

**`internal/audit/log.go`** — two new constants:
- `ActionSettingsIdentityUpdate = "settings.identity_update"`
- `EntityTypeInstallIdentity = "install_identity"`

**`internal/auth/authz.go`** — new action constant `ActionSettingsIdentityUpdate Action = "settings.identity_update"` + added to `roleBundles[RoleAdmin]`. Viewer does NOT get this action (T-06-12-02 mitigation).

**`internal/settings/identity.go`** — two handlers:
- `GetIdentityHandler`: queries `GetInstallIdentity`, returns `IdentityResponse` with `install_id` (string of integer id=1), `site_name`, `display_name`, `address`, `timezone`, `units`, `version`. Gated by `ActionConnectionTest` (admin + viewer).
- `PatchIdentityHandler`: load-merge-validate pattern → Serializable tx → `UpsertInstallIdentity` → `audit.WriteEntry` → commit. Gated by `ActionSettingsIdentityUpdate` (admin only). Invalid `units` value → 400 `invalid_units`. `isTxSerializationFailure` helper handles 40001.

**`internal/settings/routes.go`** — `RegisterRoutes` now calls `RegisterIdentityRoutes` at the end, so identity routes are automatically included in both `RegisterRoutes` and `RegisterRoutesWithBackup`.

**Migration version assertions** bumped 48 → 49 in `migrations_test.go` and `roundtrip_test.go`.

**Tests** (7 in `identity_test.go`):
- `TestGetIdentity_ReturnsFields` — GET returns 200 with correct install_id, site_name, display_name, timezone, units, version
- `TestGetIdentity_RequiresAuth` — unauthenticated GET (empty cookie) → 401
- `TestPatchIdentity_UpdatesFields` — PATCH {"display_name":"Updated Name"} → 200; subsequent GET confirms value
- `TestPatchIdentity_ViewerForbidden` — viewer PATCH → 403
- `TestPatchIdentity_WritesAuditRow` — after PATCH, audit_log has row with action='settings.identity_update', entity_type='install_identity'
- `TestPatchIdentity_InvalidUnits` — PATCH {"units":"gallons"} → 400 {"error":"invalid_units"}
- `TestMigration0049_AddsVocab` — valid INSERT with settings.identity_update succeeds; typo variant fails CHECK constraint

### Task 2 — Frontend

**`InstallIdentityCard.tsx`** — updated from read-only to full read+edit surface:
- Extended `InstallIdentity` interface with `display_name`, `address`, `timezone`, `units` fields
- `EditIdentityDialog` component with `react-hook-form` + zod schema (`display_name` required 1..200, `address` optional, `timezone` required 1..64, `units` enum)
- Admin-only "Edit identity" button (data-testid="edit-identity-button") opens dialog
- `useMutation` PATCH with `onSuccess` → `qc.invalidateQueries(['settings', 'identity'])` + `toast.success('Identity updated')` + close dialog
- D-47 propagation note updated to mention "Changes to identity fields apply to future reports"
- Card retains `data-testid="install-identity-card"` for test compatibility

**`settings.tsx`** — imports `InstallIdentityCard` and mounts it as the first card in the settings page (above Account card), establishing the Install Identity category (SETT-01).

## Deviations from Plan

None. Plan executed exactly as written.

The 3 pre-existing TypeScript errors in `BackupStatusCard.tsx` and `audit/index.test.tsx` and the `TestRunMigrations_CAGGsDropClean` failure are confirmed pre-existing (present before this plan's changes, verified by `git stash` test) and are out of scope per deviation rules.

## Known Stubs

None. The InstallIdentityCard stub identified in Plan 06-10 is fully resolved:
- Backend endpoint exists and is registered
- Card is mounted in settings.tsx
- Admin edit path wired with PATCH mutation

## Threat Flags

No new threat surface beyond what is documented in the plan's threat model. The `/api/settings/identity` endpoints are gated by existing `RequireAction` middleware.

## Self-Check: PASSED

Files created/exist:
- FOUND: internal/db/migrations/0049_audit_vocab_identity.up.sql
- FOUND: internal/db/migrations/0049_audit_vocab_identity.down.sql
- FOUND: internal/settings/identity.go
- FOUND: internal/settings/identity_test.go
- FOUND: web/src/components/settings/InstallIdentityCard.tsx (updated)
- FOUND: web/src/routes/settings.tsx (updated)

Commits verified:
- FOUND: 29fb5a5 (feat(06-12): migration 0049 + identity GET/PATCH backend)
- FOUND: a5d6a78 (feat(06-12): mount InstallIdentityCard + admin edit dialog)

Tests:
- `go test ./internal/settings/... -count=1` — 42 passed (includes 7 new identity tests)
- `go test ./internal/db/... -run TestRunMigrations_Clean` — 1 passed (version=49)
- `pnpm --dir web test:run` — 378 passed, 0 failed
