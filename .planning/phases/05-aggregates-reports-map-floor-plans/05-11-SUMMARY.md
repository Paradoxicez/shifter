---
phase: 05-aggregates-reports-map-floor-plans
plan: 11
subsystem: settings
tags: [retention, timescaledb, policy-reconciliation, audit, admin-only, pgx-v5]
dependency_graph:
  requires: [05-02]
  provides: [DATA-13, SETT-04]
  affects: [audit_log, retention_config, settings-page]
tech_stack:
  added: []
  patterns:
    - same-tx TimescaleDB retention policy reconciliation (remove + add inside pgx.Tx)
    - singleton retention_config PATCH with yearly_forever sentinel protocol
    - compile-time hypertable name switch (CLAUDE.md banned-dep compliance)
    - zod v4 + react-hook-form valueAsNumber pattern for number inputs
key_files:
  created:
    - internal/db/queries/settings.sql
    - internal/db/sqlc/settings.sql.go
    - internal/settings/doc.go
    - internal/settings/retention.go
    - internal/settings/routes.go
    - internal/db/migrations/0036_audit_vocab_retention.up.sql
    - internal/db/migrations/0036_audit_vocab_retention.down.sql
    - web/src/components/settings/DataRetentionCard.tsx
    - web/src/components/settings/EditRetentionDialog.tsx
    - web/src/routes/settings.test.tsx
  modified:
    - internal/audit/log.go
    - internal/auth/authz.go
    - internal/http/router.go
    - internal/cli/serve.go
    - internal/settings/retention_test.go
    - internal/db/sqlc/querier.go
    - internal/db/migrations_test.go
    - internal/db/roundtrip_test.go
    - web/src/routes/settings.tsx
    - web/playwright/specs/retention-settings.spec.ts
    - .planning/REQUIREMENTS.md
decisions:
  - "same-tx reconciliation chosen over async worker: operator sees consistent state immediately; no drift between retention_config row and actual TimescaleDB policies"
  - "yearly_forever *bool sentinel distinguishes omitted vs explicit null yearly_days — necessary because PATCH omits fields not being changed"
  - "hypertable names from compile-time switch (not runtime strings) satisfies CLAUDE.md lib/pq ban and SQL injection defense (T-05-11-02)"
  - "zod v4 + valueAsNumber: true on number input avoids z.coerce resolver type mismatch with @hookform/resolvers"
  - "SETT-04 migrated Phase 6 → Phase 5: DATA-13 substrate and UI ship together (RESEARCH Open Q #4)"
metrics:
  duration: ~120min
  completed: 2026-05-12
  tasks: 2
  files: 18
---

# Phase 05 Plan 11: Settings Data Retention Summary

**One-liner:** GET/PATCH `/api/settings/retention` with same-transaction TimescaleDB policy reconciliation, audit-in-tx, viewer 403, and a 5-row admin-only DataRetentionCard with zod-validated edit dialog.

## What Was Built

### Task 1: Backend (TDD — RED + GREEN)

**Migration 0036** extends `audit_log` CHECK constraints with `settings.retention_change` action and `retention_config` entity type.

**`internal/settings/` package** (new):
- `GetHandler` — serves GET `/api/settings/retention`; both admin and viewer can read
- `PatchHandler` — admin-only PATCH; validates ranges, updates `retention_config` and reconciles TimescaleDB retention policies inside the same `pgx.Tx`
- `ReconcilePolicies` — issues `remove_retention_policy` + `add_retention_policy` for each changed level via a compile-time `switch` (T-05-11-02 defense)
- `RegisterRoutes` — mounts GET under `ActionConnectionTest` gate, PATCH under `ActionSettingsUpdate` gate (admin-only)

**Auth**: `ActionSettingsUpdate` added to `roleBundles[RoleAdmin]` only — fail-closed.

**Sqlc queries**: `GetRetentionConfig` (`:one`), `UpdateRetentionConfig` (`:one` with `sqlc.narg` COALESCE partial update).

**15 integration tests** all pass (`TestRetentionConfig*`).

### Task 2: Frontend

- `DataRetentionCard` — 5-row card (raw/hourly/daily/monthly/yearly); admin sees Edit ghost buttons, viewer sees read-only values; yearly shows "Never expires" when null
- `EditRetentionDialog` — `ResponsiveDialog` + `react-hook-form` + zod range validation + `useMutation` PATCH + sonner toast
- `settings.tsx` — `DataRetentionCard` added below ChirpStack connection card
- `settings.test.tsx` — 9 vitest cases covering all behaviors
- `retention-settings.spec.ts` — Playwright spec body filled (admin edit + viewer read-only)
- `REQUIREMENTS.md` — SETT-04 moved Phase 6→Phase 5, marked Complete

**`pnpm build` and `pnpm test:run` both exit 0.**

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Integration tests failed with `load_failed` 500**
- **Found during:** Task 1 GREEN phase
- **Issue:** `GetRetentionConfig` returned `pgx.ErrNoRows` in test DB — the `retention_config` singleton row is seeded by `install.FinishSetup` in production but tests skip that step
- **Fix:** Added seed INSERT `(id=1, raw_days=90, ...)` in `startTestDB` after `RunMigrations`
- **Commit:** 4b1163f

**2. [Rule 1 - Bug] Integration tests failed with `audit_failed` 500**
- **Found during:** Task 1 GREEN phase (after load_failed fix)
- **Issue:** `audit_log.user_id` has FK → `"user"(id)`; test sessions used random UUIDs not in the `user` table; FK constraint rejected the INSERT
- **Fix:** Added `seedTestUser` helper that inserts a real `user` row with the session's UUID before tests that call PATCH
- **Commit:** 4b1163f

**3. [Rule 1 - Bug] Test query for audit entry used wrong entity_id encoding**
- **Found during:** Task 1 GREEN phase
- **Issue:** `TestRetentionConfig_AuditEntryWritten` queried `EntityID: pgtype.UUID{Valid: false}` (NULL) but handler stores `uuid.Nil` as `pgtype.UUID{Bytes: [16]byte{}, Valid: true}` (all-zeros UUID)
- **Fix:** Updated query to `pgtype.UUID{Bytes: [16]byte{}, Valid: true}`
- **Commit:** 4b1163f

**4. [Rule 1 - Bug] Zod v4 `invalid_type_error` option doesn't exist**
- **Found during:** Task 2 build
- **Issue:** Zod v4 uses `{ error: '...' }` not `{ invalid_type_error: '...' }` for number type errors
- **Fix:** Changed to `z.number({ error: 'Must be a number' })` and used `valueAsNumber: true` on the input + `resolver: zodResolver(schema) as any` to bypass v4 type inference mismatch with `@hookform/resolvers`
- **Commits:** daf8a27

**5. [Rule 1 - Bug] Vitest Test 6 couldn't trigger form submit through Radix Dialog overlay**
- **Found during:** Task 2 test iteration
- **Issue:** `userEvent.click` on Save button failed silently — Radix Dialog sets `pointer-events: none` on body; `@testing-library/user-event` v14 respects this and drops the event
- **Fix:** Used `fireEvent.submit(form)` directly on the form element (bypasses pointer-events check); consistent with how other tests in the codebase interact with Radix overlays
- **Commits:** daf8a27

### No Architectural Deviations

Plan executed as designed. Same-tx reconciliation required no TimescaleDB-version-specific tweaks — `remove_retention_policy`/`add_retention_policy` work inside transactions in TimescaleDB 2.26.

## Audit Before/After Coverage

The `diffFields` function captures only changed fields. In tests:
- `PATCH {raw_days: 60}` → before: `{raw_days: 90}`, after: `{raw_days: 60}`
- `PATCH {yearly_forever: true}` → before: `{yearly_days: <current>}`, after: `{yearly_days: null}`
- Unchanged fields are omitted from both maps (D-24 compliance)

## REQUIREMENTS.md Update

SETT-04 traceability table row: `Phase 6 | Pending` → `Phase 5 | Complete`. Checkbox marked `[x]`. Row order preserved — SETT-04 stays between SETT-03 and SETT-05.

## lib/pq Compliance (CLAUDE.md)

`grep '"github.com/lib/pq"' internal/settings/` returns zero actual import statements. All appearances are comments documenting the ban. Package uses `pgx/v5` exclusively.

## Known Stubs

None. The `DataRetentionCard` fetches real data from `GET /api/settings/retention` via `useQuery`. No hardcoded empty values flow to the UI.

## Threat Flags

None. All endpoints introduced by this plan are covered by the existing threat model (T-05-11-01, T-05-11-02).

## Self-Check: PASSED

Files exist:
- `/Users/suraboonsung/Documents/Programming/shifter/internal/settings/retention.go` ✓
- `/Users/suraboonsung/Documents/Programming/shifter/internal/settings/routes.go` ✓
- `/Users/suraboonsung/Documents/Programming/shifter/web/src/components/settings/DataRetentionCard.tsx` ✓
- `/Users/suraboonsung/Documents/Programming/shifter/web/src/components/settings/EditRetentionDialog.tsx` ✓
- `/Users/suraboonsung/Documents/Programming/shifter/web/src/routes/settings.test.tsx` ✓
- `/Users/suraboonsung/Documents/Programming/shifter/.planning/phases/05-aggregates-reports-map-floor-plans/05-11-SUMMARY.md` ✓

Commits exist:
- `145ac66` test(05-11): add failing tests (TDD RED) ✓
- `4b1163f` feat(05-11): Task 1 GREEN ✓
- `daf8a27` feat(05-11): Task 2 frontend ✓
