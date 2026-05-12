---
phase: 06-alerts-users-audit-operational-hardening
plan: 12
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/db/migrations/0049_audit_vocab_identity.up.sql
  - internal/db/migrations/0049_audit_vocab_identity.down.sql
  - internal/auth/authz.go
  - internal/settings/identity.go
  - internal/settings/identity_test.go
  - internal/settings/routes.go
  - web/src/components/settings/InstallIdentityCard.tsx
  - web/src/routes/settings.tsx
autonomous: true
gap_closure: true
requirements_addressed:
  - SETT-01
  - SETT-02

must_haves:
  truths:
    - "GET /api/settings/identity returns 200 for any authenticated user and includes install_id, site_name, display_name, address, timezone, units, version fields"
    - "PATCH /api/settings/identity returns 200 for admin and updates display_name, address, timezone, units in install_identity table"
    - "PATCH /api/settings/identity writes a settings.identity_update audit row in the same Serializable transaction"
    - "PATCH /api/settings/identity returns 403 for viewer role (admin-only gate enforced server-side)"
    - "InstallIdentityCard is mounted in settings.tsx and renders install identity fields"
    - "InstallIdentityCard has an admin-only Edit button that opens a dialog to update identity fields"
    - "Settings page has a visible Install Identity category section"
  artifacts:
    - path: "internal/db/migrations/0049_audit_vocab_identity.up.sql"
      provides: "Extends audit_log CHECK constraint with settings.identity_update action and install_identity entity type"
      contains: "settings.identity_update"
    - path: "internal/settings/identity.go"
      provides: "GET and PATCH handlers for /api/settings/identity"
      exports: ["GetIdentityHandler", "PatchIdentityHandler"]
    - path: "internal/settings/identity_test.go"
      provides: "Automated tests for identity GET/PATCH handlers"
      contains: "TestGetIdentity"
    - path: "web/src/components/settings/InstallIdentityCard.tsx"
      provides: "Read + edit surface for install identity (updated with edit dialog)"
      contains: "EditIdentityDialog"
    - path: "web/src/routes/settings.tsx"
      provides: "Settings page that mounts InstallIdentityCard"
      contains: "InstallIdentityCard"
  key_links:
    - from: "web/src/components/settings/InstallIdentityCard.tsx"
      to: "/api/settings/identity"
      via: "GET fetch in useQuery"
      pattern: "apiFetch.*settings/identity"
    - from: "internal/settings/identity.go"
      to: "sqlc.Queries.GetInstallIdentity / UpsertInstallIdentity"
      via: "deps.Queries in handler"
      pattern: "GetInstallIdentity|UpsertInstallIdentity"
    - from: "internal/settings/identity.go"
      to: "audit_log"
      via: "audit.WriteEntry inside Serializable tx"
      pattern: "audit\\.WriteEntry"
    - from: "internal/settings/routes.go"
      to: "internal/http/router.go"
      via: "RegisterRoutes calls RegisterIdentityRoutes"
      pattern: "RegisterIdentityRoutes"
---

<objective>
Close SETT-02 gap: implement the missing /api/settings/identity GET+PATCH backend, wire routes, and mount the orphaned InstallIdentityCard into the Settings page with admin-only edit capability.

Purpose: SETT-02 (admin can update install identity at any time, changes propagate to report branding) was never delivered — the Phase 6 execution created the frontend card but left the backend as a known stub. This plan delivers the full stack.

Output:
- migration 0049 extending audit vocabulary with `settings.identity_update` action + `install_identity` entity type
- `internal/settings/identity.go` — GetIdentityHandler + PatchIdentityHandler
- `internal/settings/routes.go` — extended with identity routes
- `internal/auth/authz.go` — ActionSettingsIdentityUpdate constant + admin role bundle entry
- `internal/settings/identity_test.go` — 5 tests covering GET/PATCH/RBAC/audit/validation
- `web/src/components/settings/InstallIdentityCard.tsx` — updated with Edit button + dialog
- `web/src/routes/settings.tsx` — mounts InstallIdentityCard above ChirpStack card
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/ROADMAP.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-CONTEXT.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-10-settings-extensions-SUMMARY.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-11-ops-hardening-doctor-runbook-SUMMARY.md

<interfaces>
<!-- Key types the executor needs. Extracted from the codebase — no exploration needed. -->

From internal/db/sqlc/install_identity.sql.go:
```go
// GetInstallIdentity fetches the singleton install_identity row (id=1).
func (q *Queries) GetInstallIdentity(ctx context.Context) (InstallIdentity, error)

// UpsertInstallIdentity upserts the singleton row (ON CONFLICT DO UPDATE).
// Used by install wizard finish AND now by the settings PATCH handler.
type UpsertInstallIdentityParams struct {
    DisplayName string
    LogoPath    *string
    Address     *string
    Timezone    string
    Units       UnitsSystem
}
func (q *Queries) UpsertInstallIdentity(ctx context.Context, arg UpsertInstallIdentityParams) (InstallIdentity, error)
```

From internal/db/sqlc/models.go:
```go
type InstallIdentity struct {
    ID           int32
    DisplayName  string
    LogoPath     *string
    Address      *string
    Timezone     string
    Units        UnitsSystem
    CreatedAt    pgtype.Timestamptz
    UpdatedAt    pgtype.Timestamptz
    Capabilities string
}
type UnitsSystem string
const (
    UnitsSystemMetric   UnitsSystem = "metric"
    UnitsSystemImperial UnitsSystem = "imperial"
)
```

From internal/audit/log.go (action constants — add new ones for this plan):
```go
// All action strings must match audit_log_action_valid CHECK constraint.
// Phase 6 existing vocab does NOT include settings.identity_update.
// This plan adds it via migration 0049 + the constant below.
const ActionSettingsIdentityUpdate = "settings.identity_update"  // NEW — to be added

// Entity type (new — add to migration 0049):
const EntityTypeInstallIdentity = "install_identity"  // NEW — to be added

// WriteEntry signature (audit-in-tx pattern):
func WriteEntry(ctx context.Context, tx pgx.Tx, e Entry) error
type Entry struct {
    UserID     uuid.UUID
    Action     string
    EntityType string
    EntityID   uuid.UUID
    RequestID  string
    Before     any
    After      any
    Notes      string
}
```

From internal/auth/authz.go (add after ActionSettingsUpdate):
```go
// Already present:
ActionSettingsUpdate Action = "settings.update"

// Add:
ActionSettingsIdentityUpdate Action = "settings.identity_update"

// roleBundles[RoleAdmin] must include:
ActionSettingsIdentityUpdate: true,
// ActionConnectionTest covers GET for viewer (any authed user can read)
```

From internal/settings/routes.go (extend RegisterRoutes):
```go
// Current signature:
func RegisterRoutes(r chi.Router, deps Deps, sm *scs.SessionManager)

// Add RegisterIdentityRoutes (called from RegisterRoutes):
func RegisterIdentityRoutes(r chi.Router, deps Deps, sm *scs.SessionManager)
// Mounts:
//   GET  /api/settings/identity — ActionConnectionTest (admin + viewer)
//   PATCH /api/settings/identity — ActionSettingsIdentityUpdate (admin only)
```

From internal/version/version.go (used in GET response):
```go
// version.Info() returns a map. Use version.Version variable directly:
import "github.com/shifter-io/shifter/internal/version"
// version.Version is a string (e.g. "0.1.0")
```

From internal/settings/backup_card.go (canonical handler pattern):
```go
// Handler pattern: no separate Serializable wrapper needed for GET.
// For PATCH (mutating): open pgx.Serializable tx, run UpsertInstallIdentity
// inside tx, call audit.WriteEntry inside same tx, commit.
// On 40001 serialization failure → 409 serialization_failure.
// writeError / writeJSON helpers are in the settings package already.
```

From web/src/components/settings/InstallIdentityCard.tsx (existing read-only card):
```typescript
// Current interface — DO NOT CHANGE these field names (GET handler must match):
interface InstallIdentity {
  install_id: string   // numeric install_identity.id as string (or UUID if available)
  site_name: string    // install_identity.display_name
  version: string      // version.Version
}

// The card calls: apiFetch<InstallIdentity>('/api/settings/identity')
// staleTime: 5 * 60 * 1000
```

NOTE on install_id: The backup manifest uses install_identity.id (an integer, id=1) converted to string. Return it as a string from the GET handler.

NOTE on PATCH response fields: The PATCH response should return the full updated identity.
The PATCH accepts: { display_name?: string, address?: string, timezone?: string, units?: "metric"|"imperial" }
Fields not supplied retain current values (load-merge-patch pattern, same as backup_card.go).

From web/src/routes/settings.tsx (where to mount the card):
```typescript
// Current imports (add InstallIdentityCard):
import { InstallIdentityCard } from '@/components/settings/InstallIdentityCard'

// Mount ABOVE the ChirpStack card (first card in the settings page):
<InstallIdentityCard />
// Then <Card> Account ...
// Then <Card> ChirpStack connection ...
```
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Migration 0049 + auth constants + identity.go backend</name>
  <files>
    internal/db/migrations/0049_audit_vocab_identity.up.sql
    internal/db/migrations/0049_audit_vocab_identity.down.sql
    internal/auth/authz.go
    internal/settings/identity.go
    internal/settings/identity_test.go
    internal/settings/routes.go
  </files>
  <read_first>
    - internal/db/migrations/0048_audit_vocab_alert_prune.up.sql (migration pattern: DROP+ADD constraint)
    - internal/db/migrations/0037_audit_vocab_phase6.up.sql (full current CHECK vocabulary to copy verbatim)
    - internal/db/migrations/0048_audit_vocab_alert_prune.down.sql (down migration pattern)
    - internal/audit/log.go (existing action/entity constants — copy pattern, append new ones)
    - internal/auth/authz.go (ActionSettingsUpdate and roleBundles — add adjacent)
    - internal/settings/routes.go (RegisterRoutes — extend to call RegisterIdentityRoutes)
    - internal/settings/backup_card.go (PatchBackupThresholdsHandler — canonical Serializable tx + audit-in-tx pattern)
    - internal/settings/retention.go (writeError/writeJSON helpers are defined here — same package)
    - internal/settings/doc.go (package doc)
    - internal/db/migrations_test.go (version assertion — bump to 49)
    - internal/db/roundtrip_test.go (version assertion — bump to 49)
  </read_first>
  <behavior>
    - TestGetIdentity_ReturnsFields: GET /api/settings/identity with seeded install_identity row returns 200 with install_id="1", site_name=display_name, version non-empty
    - TestGetIdentity_RequiresAuth: unauthenticated GET returns 401
    - TestPatchIdentity_UpdatesDisplayName: PATCH /api/settings/identity with {"display_name":"Acme Water"} returns 200, GetInstallIdentity now returns "Acme Water"
    - TestPatchIdentity_WritesAuditRow: after PATCH, audit_log has 1 row with action="settings.identity_update", entity_type="install_identity"
    - TestPatchIdentity_ViewerForbidden: viewer role PATCH → 403
    - TestPatchIdentity_InvalidUnits: PATCH with {"units":"gallons"} → 400 with {"error":"invalid_units"}
    - TestMigration0049_AddsVocab: after migration 0049, INSERT audit_log row with action='settings.identity_update' succeeds; INSERT with action='settings.identity_update_typo' fails CHECK
  </behavior>
  <action>
**Step 1 — Migration 0049:**

Create `internal/db/migrations/0049_audit_vocab_identity.up.sql`:
```sql
-- 0049_audit_vocab_identity.up.sql
-- Gap closure Plan 06-12: adds settings.identity_update action and
-- install_identity entity type to audit_log CHECK constraints.
-- Follows the DROP+re-ADD pattern established in 0020, 0031, 0034..0037, 0048.

ALTER TABLE audit_log DROP CONSTRAINT audit_log_action_valid;
ALTER TABLE audit_log ADD CONSTRAINT audit_log_action_valid CHECK (action IN (
    'create','update','archive','restore','swap','decommission',
    'profile_create','profile_update','binding_open','binding_close',
    'rollover_detected',
    'gateway.create','gateway.update','gateway.archive','gateway.restore',
    'device.bulk_import','device.reveal_secrets',
    'report.generate',
    'floor_plan.upload','floor_plan.replace_image','floor_plan.rename','floor_plan.delete',
    'placement.pin','placement.nudge','placement.remove',
    'settings.retention_change',
    'auth.login_success','auth.login_failed','auth.logout',
    'auth.password_change','auth.password_reset_by_admin','auth.session_revoked',
    'user.create','user.update','user.disable','user.enable','user.role_change',
    'alert.rule_create','alert.rule_update','alert.rule_disable','alert.rule_enable',
    'alert.fired','alert.cleared','alert.acknowledged','alert.snoozed','alert.muted','alert.test_fired',
    'backup.start','backup.complete','backup.failed','backup.restore','audit.prune','audit.export',
    'alert.pruned',
    -- Gap closure Plan 06-12 (SETT-02):
    'settings.identity_update'
));

ALTER TABLE audit_log DROP CONSTRAINT audit_log_entity_type_valid;
ALTER TABLE audit_log ADD CONSTRAINT audit_log_entity_type_valid CHECK (entity_type IN (
    'site','metering_point','device','device_profile','binding',
    'gateway','import_job',
    'report',
    'floor_plan',
    'placement',
    'retention_config',
    'user','session','alert_rule','alert','backup_run','audit_log',
    -- Gap closure Plan 06-12 (SETT-02):
    'install_identity'
));
```

Create `internal/db/migrations/0049_audit_vocab_identity.down.sql`:
```sql
-- 0049_audit_vocab_identity.down.sql
-- Revert to 0048 vocabulary (remove settings.identity_update + install_identity).
ALTER TABLE audit_log DROP CONSTRAINT audit_log_action_valid;
ALTER TABLE audit_log ADD CONSTRAINT audit_log_action_valid CHECK (action IN (
    'create','update','archive','restore','swap','decommission',
    'profile_create','profile_update','binding_open','binding_close',
    'rollover_detected',
    'gateway.create','gateway.update','gateway.archive','gateway.restore',
    'device.bulk_import','device.reveal_secrets',
    'report.generate',
    'floor_plan.upload','floor_plan.replace_image','floor_plan.rename','floor_plan.delete',
    'placement.pin','placement.nudge','placement.remove',
    'settings.retention_change',
    'auth.login_success','auth.login_failed','auth.logout',
    'auth.password_change','auth.password_reset_by_admin','auth.session_revoked',
    'user.create','user.update','user.disable','user.enable','user.role_change',
    'alert.rule_create','alert.rule_update','alert.rule_disable','alert.rule_enable',
    'alert.fired','alert.cleared','alert.acknowledged','alert.snoozed','alert.muted','alert.test_fired',
    'backup.start','backup.complete','backup.failed','backup.restore','audit.prune','audit.export',
    'alert.pruned'
));
ALTER TABLE audit_log DROP CONSTRAINT audit_log_entity_type_valid;
ALTER TABLE audit_log ADD CONSTRAINT audit_log_entity_type_valid CHECK (entity_type IN (
    'site','metering_point','device','device_profile','binding',
    'gateway','import_job',
    'report',
    'floor_plan',
    'placement',
    'retention_config',
    'user','session','alert_rule','alert','backup_run','audit_log'
));
```

**Step 2 — internal/audit/log.go:**

Append two new constant blocks AFTER the existing `ActionAlertPruned` block:
```go
// Gap closure Plan 06-12 — SETT-02: install identity update audit.
const (
    ActionSettingsIdentityUpdate = "settings.identity_update"
)

// Gap closure Plan 06-12 — SETT-02: install_identity entity type.
const (
    EntityTypeInstallIdentity = "install_identity"
)
```

**Step 3 — internal/auth/authz.go:**

After the `ActionSettingsUpdate` const block, append:
```go
// Gap closure Plan 06-12 — SETT-02: identity update (admin only).
// GET /api/settings/identity uses ActionConnectionTest (any authed user).
// PATCH /api/settings/identity uses ActionSettingsIdentityUpdate (admin only).
const (
    ActionSettingsIdentityUpdate Action = "settings.identity_update"
)
```

In `roleBundles[RoleAdmin]`, after the `ActionSettingsUpdate: true` entry, add:
```go
ActionSettingsIdentityUpdate: true,
```

Do NOT add `ActionSettingsIdentityUpdate` to `roleBundles[RoleViewer]` — viewer cannot mutate identity.

**Step 4 — internal/settings/identity.go:**

```go
// Package settings — install identity GET + PATCH handlers (Plan 06-12 / SETT-02).
//
// GET  /api/settings/identity  — admin + viewer (ActionConnectionTest)
// PATCH /api/settings/identity — admin only (ActionSettingsIdentityUpdate)
//
// PATCH follows the load-merge-validate pattern from backup_card.go:
//   1. Load current install_identity row.
//   2. Merge supplied nullable fields over the current values.
//   3. Validate merged result.
//   4. UpsertInstallIdentity inside Serializable tx + audit.WriteEntry.
//
// The GET response shape matches the existing InstallIdentityCard.tsx interface:
//   { install_id, site_name, display_name, address, timezone, units, version }
// install_id returns the integer id=1 as a string.
// version returns the build version from the version package.

package settings

import (
    "encoding/json"
    "errors"
    "net/http"
    "strconv"

    "github.com/alexedwards/scs/v2"
    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"
    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgconn"

    "github.com/shifter-io/shifter/internal/audit"
    "github.com/shifter-io/shifter/internal/auth"
    sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
    "github.com/shifter-io/shifter/internal/version"
)

// IdentityResponse is the wire shape for GET /api/settings/identity.
// Field names are locked by InstallIdentityCard.tsx — do not rename.
type IdentityResponse struct {
    InstallID   string  `json:"install_id"`
    SiteName    string  `json:"site_name"`
    DisplayName string  `json:"display_name"`
    Address     *string `json:"address,omitempty"`
    Timezone    string  `json:"timezone"`
    Units       string  `json:"units"`
    Version     string  `json:"version"`
}

// IdentityPatch is the wire shape for PATCH /api/settings/identity.
// All fields are optional — omitted fields retain their current value.
type IdentityPatch struct {
    DisplayName *string `json:"display_name"`
    Address     *string `json:"address"`
    Timezone    *string `json:"timezone"`
    Units       *string `json:"units"`
}

// RegisterIdentityRoutes mounts the identity endpoints on r.
//
//	GET   /api/settings/identity — ActionConnectionTest (admin + viewer)
//	PATCH /api/settings/identity — ActionSettingsIdentityUpdate (admin only)
func RegisterIdentityRoutes(r chi.Router, deps Deps, sm *scs.SessionManager) {
    r.Group(func(rt chi.Router) {
        rt.Use(auth.RequireAction(sm, auth.ActionConnectionTest))
        rt.Get("/api/settings/identity", GetIdentityHandler(deps))
    })
    r.Group(func(rt chi.Router) {
        rt.Use(auth.RequireAction(sm, auth.ActionSettingsIdentityUpdate))
        rt.Patch("/api/settings/identity", PatchIdentityHandler(deps, sm))
    })
}

// GetIdentityHandler serves GET /api/settings/identity.
// Accessible to any authenticated user (admin + viewer).
func GetIdentityHandler(deps Deps) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        row, err := deps.Queries.GetInstallIdentity(r.Context())
        if err != nil {
            writeError(w, http.StatusInternalServerError, "load_failed")
            return
        }
        resp := IdentityResponse{
            InstallID:   strconv.Itoa(int(row.ID)),
            SiteName:    row.DisplayName,
            DisplayName: row.DisplayName,
            Address:     row.Address,
            Timezone:    row.Timezone,
            Units:       string(row.Units),
            Version:     version.Version,
        }
        writeJSON(w, http.StatusOK, resp)
    }
}

// PatchIdentityHandler serves PATCH /api/settings/identity.
// Admin-only (ActionSettingsIdentityUpdate gate in RegisterIdentityRoutes).
// Follows load-merge-validate → Serializable tx → UpsertInstallIdentity + audit.WriteEntry.
func PatchIdentityHandler(deps Deps, sm *scs.SessionManager) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        var patch IdentityPatch
        if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
            writeError(w, http.StatusBadRequest, "invalid_json")
            return
        }

        // Load current identity for merge.
        current, err := deps.Queries.GetInstallIdentity(r.Context())
        if err != nil {
            writeError(w, http.StatusInternalServerError, "load_failed")
            return
        }

        // Merge: apply supplied fields over current values.
        merged := sqlc.UpsertInstallIdentityParams{
            DisplayName: current.DisplayName,
            LogoPath:    current.LogoPath,
            Address:     current.Address,
            Timezone:    current.Timezone,
            Units:       current.Units,
        }
        if patch.DisplayName != nil {
            merged.DisplayName = *patch.DisplayName
        }
        if patch.Address != nil {
            merged.Address = patch.Address
        }
        if patch.Timezone != nil {
            merged.Timezone = *patch.Timezone
        }
        if patch.Units != nil {
            u := sqlc.UnitsSystem(*patch.Units)
            if u != sqlc.UnitsSystemMetric && u != sqlc.UnitsSystemImperial {
                writeError(w, http.StatusBadRequest, "invalid_units")
                return
            }
            merged.Units = u
        }

        // Validate merged values.
        if len(merged.DisplayName) == 0 || len(merged.DisplayName) > 200 {
            writeError(w, http.StatusBadRequest, "display_name_invalid")
            return
        }
        if len(merged.Timezone) == 0 || len(merged.Timezone) > 64 {
            writeError(w, http.StatusBadRequest, "timezone_invalid")
            return
        }

        // Caller identity for audit row — mirrors backup_card.go PatchBackupThresholdsHandler.
        // auth.GetUser always succeeds here: RequireAction already returned 403 if no user.
        user, ok := auth.GetUser(r.Context(), sm)
        if !ok {
            writeError(w, http.StatusUnauthorized, "unauthorized")
            return
        }
        callerID := uuid.MustParse(user.ID)
        reqID := middleware.GetReqID(r.Context())

        // Build before/after for audit diff.
        before := map[string]any{
            "display_name": current.DisplayName,
            "address":      current.Address,
            "timezone":     current.Timezone,
            "units":        string(current.Units),
        }

        // Serializable tx: upsert + audit in one atomic unit (D-30 pattern).
        tx, err := deps.Pool.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.Serializable})
        if err != nil {
            writeError(w, http.StatusInternalServerError, "tx_begin_failed")
            return
        }
        defer tx.Rollback(r.Context())

        qtx := deps.Queries.WithTx(tx)
        updated, err := qtx.UpsertInstallIdentity(r.Context(), merged)
        if err != nil {
            writeError(w, http.StatusInternalServerError, "upsert_failed")
            return
        }

        after := map[string]any{
            "display_name": updated.DisplayName,
            "address":      updated.Address,
            "timezone":     updated.Timezone,
            "units":        string(updated.Units),
        }

        // EntityID: use uuid.Nil (no UUID column on install_identity; id=1 integer).
        if err := audit.WriteEntry(r.Context(), tx, audit.Entry{
            UserID:     callerID,
            Action:     audit.ActionSettingsIdentityUpdate,
            EntityType: audit.EntityTypeInstallIdentity,
            EntityID:   uuid.Nil,
            RequestID:  reqID,
            Before:     before,
            After:      after,
        }); err != nil {
            writeError(w, http.StatusInternalServerError, "audit_failed")
            return
        }

        if err := tx.Commit(r.Context()); err != nil {
            if isTxSerializationFailure(err) {
                writeError(w, http.StatusConflict, "serialization_failure")
                return
            }
            writeError(w, http.StatusInternalServerError, "tx_commit_failed")
            return
        }

        resp := IdentityResponse{
            InstallID:   strconv.Itoa(int(updated.ID)),
            SiteName:    updated.DisplayName,
            DisplayName: updated.DisplayName,
            Address:     updated.Address,
            Timezone:    updated.Timezone,
            Units:       string(updated.Units),
            Version:     version.Version,
        }
        writeJSON(w, http.StatusOK, resp)
    }
}

// isTxSerializationFailure returns true when err is a Postgres 40001
// serialization_failure (two concurrent Serializable txns conflicted).
// NOTE: this helper does NOT exist elsewhere in the settings package —
// add it here at the bottom of identity.go.
func isTxSerializationFailure(err error) bool {
    var pgErr *pgconn.PgError
    return errors.As(err, &pgErr) && pgErr.Code == "40001"
}
```

**NOTE: `isTxSerializationFailure` does NOT exist anywhere in the `settings` package (confirmed: `retention.go` and `backup_card.go` do not define it; only `isSerializationFailure` in `internal/user/handler.go` exists, in a different package with a different name). Add the helper shown above at the bottom of `identity.go` — it is the only definition in the package.**

`"github.com/jackc/pgx/v5/pgconn"` is already in go.mod (used by pgx/v5 transitively).

**Step 5 — internal/settings/routes.go:**

In `RegisterRoutes`, add a call to `RegisterIdentityRoutes` at the end of the function:
```go
// Identity (SETT-02 gap closure — Plan 06-12).
RegisterIdentityRoutes(r, deps, sm)
```

`RegisterRoutesWithBackup` calls `RegisterRoutes` at line 46 of `routes.go`, so identity routes are automatically included without further changes.

**Step 6 — Bump migration version assertions:**

In `internal/db/migrations_test.go`, find the line asserting the migration count/version (currently `48`) and update to `49`.
In `internal/db/roundtrip_test.go`, same update.

**Step 7 — Write tests in internal/settings/identity_test.go:**

Follow the `backup_card_test.go` pattern (spin up testcontainers Postgres, run migrations, seed install_identity, create Deps, call handlers via httptest).

Minimum 5 tests:
1. `TestGetIdentity_ReturnsFields` — GET returns 200 with correct fields for seeded row
2. `TestGetIdentity_RequiresAuth` — unauthenticated GET returns 401 (use router with RequireAction)
3. `TestPatchIdentity_UpdatesFields` — PATCH {"display_name":"Updated"} returns 200; subsequent GET returns updated value
4. `TestPatchIdentity_ViewerForbidden` — viewer session PATCH returns 403
5. `TestPatchIdentity_WritesAuditRow` — after PATCH, `SELECT count(*) FROM audit_log WHERE action='settings.identity_update'` = 1
6. `TestPatchIdentity_InvalidUnits` — PATCH {"units":"gallons"} returns 400 {"error":"invalid_units"}
  </action>
  <verify>
    <automated>go test ./internal/settings/... -run TestGetIdentity -v && go test ./internal/settings/... -run TestPatchIdentity -v && go test ./internal/db/... -run TestRunMigrations -v</automated>
  </verify>
  <done>
    - `grep -q "settings.identity_update" internal/db/migrations/0049_audit_vocab_identity.up.sql` exits 0
    - `grep -q "ActionSettingsIdentityUpdate" internal/auth/authz.go` exits 0
    - `grep -q "ActionSettingsIdentityUpdate.*true" internal/auth/authz.go` exits 0 (in roleBundles)
    - `grep -q "func GetIdentityHandler" internal/settings/identity.go` exits 0
    - `grep -q "func PatchIdentityHandler" internal/settings/identity.go` exits 0
    - `grep -q "RegisterIdentityRoutes" internal/settings/routes.go` exits 0
    - All 6 tests in identity_test.go pass
    - `go test ./internal/settings/... -v` exits 0
    - `go test ./internal/db/... -run TestRunMigrations -v` exits 0
    - `go build ./...` exits 0
  </done>
</task>

<task type="auto">
  <name>Task 2: Mount InstallIdentityCard + add edit dialog in frontend</name>
  <files>
    web/src/components/settings/InstallIdentityCard.tsx
    web/src/routes/settings.tsx
  </files>
  <read_first>
    - web/src/components/settings/InstallIdentityCard.tsx (current read-only card — add edit capability)
    - web/src/routes/settings.tsx (mount point — add import + render card above Account card)
    - web/src/components/settings/BackupStatusCard.tsx (admin-only form pattern in settings cards)
    - web/src/components/responsive-dialog.tsx (dialog component — same pattern as other settings dialogs)
    - web/src/lib/api.ts (apiFetch for mutations)
    - web/src/routes/settings.test.tsx (test file — extend with 2 new assertions)
  </read_first>
  <action>
**Goal:** Update `InstallIdentityCard.tsx` to:
1. Keep the existing read-only display (install_id, site_name, version) — DO NOT change the GET contract
2. Add admin-only "Edit" button below the display fields
3. The Edit button opens a `ResponsiveDialog` (reuse the existing component) with a react-hook-form form containing:
   - `display_name` (text input, required, min 1 char, max 200 chars)
   - `address` (text input, optional)
   - `timezone` (text input, required)
   - `units` (select: "metric" | "imperial")
4. On save: `PATCH /api/settings/identity` with `{ display_name, address, timezone, units }`
5. On success: invalidate `['settings', 'identity']` query + show `sonner` toast "Identity updated"
6. D-47 propagation note: keep the existing note ("Changes apply to future reports") visible in the admin view, updated to reference the edit form.

**Edit fields the PATCH accepts** (per backend IdentityPatch — these are NOT in the current GET response shape; extend the GET response interface to include these editable fields):
```typescript
// Extended interface (superset of current — add editable fields):
interface InstallIdentity {
  install_id: string
  site_name: string
  display_name: string   // was not in original interface — add
  address?: string       // add
  timezone: string       // add
  units: 'metric' | 'imperial'  // add
  version: string
}
```

**PATCH body type:**
```typescript
interface IdentityPatch {
  display_name?: string
  address?: string
  timezone?: string
  units?: 'metric' | 'imperial'
}
```

**EditIdentityDialog pattern** (follow BackupStatusCard threshold form):
```typescript
// Use react-hook-form + zod schema:
const identitySchema = z.object({
  display_name: z.string().min(1).max(200),
  address: z.string().optional(),
  timezone: z.string().min(1).max(64),
  units: z.enum(['metric', 'imperial']),
})
```

**Mutation:** Use `useMutation` from `@tanstack/react-query`:
```typescript
const mutation = useMutation({
  mutationFn: (patch: IdentityPatch) =>
    apiFetch<InstallIdentity>('/api/settings/identity', {
      method: 'PATCH',
      body: JSON.stringify(patch),
    }),
  onSuccess: () => {
    qc.invalidateQueries({ queryKey: ['settings', 'identity'] })
    toast.success('Identity updated')
    setEditOpen(false)
  },
  onError: () => toast.error('Failed to update identity'),
})
```

**Full updated InstallIdentityCard.tsx structure:**
```typescript
import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import * as z from 'zod'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { apiFetch } from '@/lib/api'
import { useCurrentUser } from '@/lib/use-current-user'
import { ResponsiveDialog } from '@/components/responsive-dialog'
// ... full implementation
```

**settings.tsx change:**

Add at the top of `SettingsPage` JSX, BEFORE the Account card:
```tsx
{/* Install Identity — SETT-01 / SETT-02 / Plan 06-12 gap closure */}
<InstallIdentityCard />
```

Import at top of file:
```typescript
import { InstallIdentityCard } from '@/components/settings/InstallIdentityCard'
```

**web/src/routes/settings.test.tsx:**

Add 2 new tests:
1. `InstallIdentityCard renders within Settings page` — MSW handler for GET /api/settings/identity returning stub data; assert `screen.getByTestId('install-identity-card')` present
2. `Edit button visible for admin, hidden for viewer` — mock user role in test; assert button visibility

Use MSW handler pattern already present in the test file for other cards.
  </action>
  <verify>
    <automated>pnpm --dir web test:run && grep -q "InstallIdentityCard" web/src/routes/settings.tsx</automated>
  </verify>
  <done>
    - `grep -q "InstallIdentityCard" web/src/routes/settings.tsx` exits 0
    - `grep -q "import.*InstallIdentityCard" web/src/routes/settings.tsx` exits 0
    - `grep -q "EditIdentityDialog\|edit.*identity\|setEditOpen" web/src/components/settings/InstallIdentityCard.tsx` exits 0
    - `grep -q "display_name" web/src/components/settings/InstallIdentityCard.tsx` exits 0 (edit form field)
    - `pnpm --dir web build` exits 0
    - `pnpm --dir web test:run` exits 0 (all vitest tests pass)
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| browser → GET /api/settings/identity | Any authenticated session (admin or viewer) can read identity — no secret fields exposed |
| browser → PATCH /api/settings/identity | Admin-only; viewer-forged PATCH returns 403 at RequireAction middleware |
| install_identity.display_name → report branding | Downstream: report PDF/CSV generators read display_name at generation time (D-47) — no injection risk since maroto escapes strings |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-06-12-01 | Tampering | PATCH /api/settings/identity display_name field | mitigate | Validate 1 ≤ len ≤ 200 at handler layer; Postgres column is NOT NULL with no length constraint — application gate is the only check |
| T-06-12-02 | Elevation of Privilege | PATCH /api/settings/identity by viewer | mitigate | `auth.RequireAction(sm, auth.ActionSettingsIdentityUpdate)` middleware returns 403 before handler body executes; `roleBundles[RoleViewer]` does not include this action |
| T-06-12-03 | Tampering | units field injection | mitigate | Only `"metric"` or `"imperial"` accepted; any other value → 400 `invalid_units` before tx opens |
| T-06-12-04 | Repudiation | Identity changed without audit trail | mitigate | `audit.WriteEntry` with `ActionSettingsIdentityUpdate` inside the same Serializable tx — commit cannot succeed without the audit row |
| T-06-12-05 | Information Disclosure | GET /api/settings/identity exposes version string | accept | Version is already public on `/health` endpoint (Phase 1 D-18); install_id=1 (integer) reveals nothing sensitive; no secrets in identity row |
| T-06-12-06 | Denial of Service | Large display_name or address payload | mitigate | 200-char and 1000-char limits enforced before Postgres write; chi's `middleware.RequestSize` is already global on the router |
</threat_model>

<verification>
Run after both tasks complete:

```bash
# 1. Backend unit tests
go test ./internal/settings/... -v -run TestGetIdentity
go test ./internal/settings/... -v -run TestPatchIdentity
go test ./internal/db/... -v -run TestRunMigrations

# 2. Full Go test suite (no regressions)
go test ./... -count=1

# 3. Frontend tests
pnpm --dir web test:run

# 4. Frontend build (no type errors)
pnpm --dir web build

# 5. Acceptance criteria grep checks
grep -q "settings.identity_update" internal/db/migrations/0049_audit_vocab_identity.up.sql
grep -q "ActionSettingsIdentityUpdate" internal/auth/authz.go
grep -q "func GetIdentityHandler" internal/settings/identity.go
grep -q "func PatchIdentityHandler" internal/settings/identity.go
grep -q "RegisterIdentityRoutes" internal/settings/routes.go
grep -q "InstallIdentityCard" web/src/routes/settings.tsx
grep -q "EditIdentityDialog\|setEditOpen" web/src/components/settings/InstallIdentityCard.tsx
```
</verification>

<success_criteria>
1. GET /api/settings/identity returns 200 with `install_id`, `site_name`, `display_name`, `address`, `timezone`, `units`, `version` for any authenticated user
2. PATCH /api/settings/identity returns 200 for admin, 403 for viewer
3. PATCH writes a `settings.identity_update` audit row in the same transaction
4. `InstallIdentityCard` is mounted in `settings.tsx` — Install Identity category visible on Settings page
5. Admin sees an Edit button on the card; clicking it opens a dialog with `display_name`, `address`, `timezone`, `units` fields
6. `go test ./internal/settings/... -count=1` passes (≥6 identity tests green)
7. `go test ./... -count=1` passes (no regressions across the full suite)
8. `pnpm --dir web test:run` passes (no frontend regressions)
9. `pnpm --dir web build` exits 0
10. SETT-02 verified: admin can update identity fields post-install; changes propagate to future report branding via `GetInstallIdentity` at report generation time (D-47)
11. SETT-01 verified: Settings page has a visible Install Identity category (InstallIdentityCard rendered above Account card)
</success_criteria>

<output>
After completion, create `.planning/phases/06-alerts-users-audit-operational-hardening/06-12-install-identity-surface-SUMMARY.md`
</output>
