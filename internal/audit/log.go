package audit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// Action vocabulary — must exactly match the CHECK constraint in
// migration 0016_audit_log (D-22 + D-05 rollover_detected).
const (
	ActionCreate           = "create"
	ActionUpdate           = "update"
	ActionArchive          = "archive"
	ActionRestore          = "restore"
	ActionSwap             = "swap"
	ActionDecommission     = "decommission"
	ActionProfileCreate    = "profile_create"
	ActionProfileUpdate    = "profile_update"
	ActionBindingOpen      = "binding_open"
	ActionBindingClose     = "binding_close"
	ActionRolloverDetected = "rollover_detected"
)

// Entity-type vocabulary — must exactly match the CHECK constraint in
// migration 0016_audit_log (D-22).
const (
	EntityTypeSite          = "site"
	EntityTypeMeteringPoint = "metering_point"
	EntityTypeDevice        = "device"
	EntityTypeDeviceProfile = "device_profile"
	EntityTypeBinding       = "binding"
)

// Phase 3 — Plan 03-02: gateway CRUD, bulk-import envelope, reveal-secrets
// audit. These strings MUST exactly mirror the CHECK literals in
// migration 0020_audit_log_vocabulary.up.sql — any drift = 23514 at write.
const (
	ActionGatewayCreate  = "gateway.create"
	ActionGatewayUpdate  = "gateway.update"
	ActionGatewayArchive = "gateway.archive"
	ActionGatewayRestore = "gateway.restore"
	ActionBulkImport     = "device.bulk_import"
	ActionRevealSecrets  = "device.reveal_secrets"
)

// Phase 3 entity types (mirrors 0020 CHECK literals).
const (
	EntityTypeGateway   = "gateway"
	EntityTypeImportJob = "import_job"
)

// Phase 5 — Plan 05-03: report generation audit constants. These strings MUST
// exactly mirror the CHECK literals in migration 0031_audit_vocab_phase5.up.sql.
const (
	ActionGenerateReport = "report.generate"
)

// Phase 5 entity types (mirrors 0031 CHECK literals).
const (
	EntityTypeReport = "report"
)

// Phase 5 — Plan 05-05: floor-plan CRUD audit constants. These strings MUST
// exactly mirror the CHECK literals in migration 0034_audit_vocab_floor_plan.up.sql.
const (
	ActionFloorPlanUpload  = "floor_plan.upload"
	ActionFloorPlanReplace = "floor_plan.replace_image"
	ActionFloorPlanRename  = "floor_plan.rename"
	ActionFloorPlanDelete  = "floor_plan.delete"
)

// Phase 5 floor-plan entity type (mirrors 0034 CHECK literals).
const (
	EntityTypeFloorPlan = "floor_plan"
)

// Phase 5 — Plan 05-07: placement CRUD audit constants. These strings MUST
// exactly mirror the CHECK literals in migration 0035_audit_vocab_placement.up.sql.
const (
	ActionPlacementPin    = "placement.pin"
	ActionPlacementNudge  = "placement.nudge"
	ActionPlacementRemove = "placement.remove"
)

// Phase 5 placement entity type (mirrors 0035 CHECK literals).
const (
	EntityTypePlacement = "placement"
)

// Phase 5 — Plan 05-11: retention settings audit constants. These strings MUST
// exactly mirror the CHECK literals in migration 0036_audit_vocab_retention.up.sql.
const (
	ActionRetentionChange = "settings.retention_change"
)

// Phase 5 retention_config entity type (mirrors 0036 CHECK literals).
const (
	EntityTypeRetention = "retention_config"
)

// Phase 6 — Plan 06-01: auth-event audit retrofit (D-30; operator-visible only).
// These strings MUST exactly mirror the CHECK literals in
// migration 0037_audit_vocab_phase6.up.sql — any drift = 23514 at write.
const (
	ActionAuthLoginSuccess         = "auth.login_success"
	ActionAuthLoginFailed          = "auth.login_failed"
	ActionAuthLogout               = "auth.logout"
	ActionAuthPasswordChange       = "auth.password_change"
	ActionAuthPasswordResetByAdmin = "auth.password_reset_by_admin"
	ActionAuthSessionRevoked       = "auth.session_revoked"
)

// Phase 6 — Plan 06-01: user management (D-30).
const (
	ActionUserCreate     = "user.create"
	ActionUserUpdate     = "user.update"
	ActionUserDisable    = "user.disable"
	ActionUserEnable     = "user.enable"
	ActionUserRoleChange = "user.role_change"
)

// Phase 6 — Plan 06-01: alert rule + alert lifecycle audit vocabulary.
// alert.acknowledged / alert.snoozed / alert.muted intentionally past-tense
// for symmetry with alert.fired / alert.cleared.
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

// Phase 6 — Plan 06-01: backup verbs (Plan 06-08 / 06-09 consume) + audit
// prune (D-51 — Plan 06-01 consumes) + audit export (D-35 — Plan 06-07).
const (
	ActionBackupStart    = "backup.start"
	ActionBackupComplete = "backup.complete"
	ActionBackupFailed   = "backup.failed"
	ActionBackupRestore  = "backup.restore"
	ActionAuditPrune     = "audit.prune"
	ActionAuditExport    = "audit.export"
)

// Phase 6 — Plan 06-01: new entity types (mirrors 0037 CHECK literals).
// EntityTypeAuditLog is reserved for the D-51 audit.prune meta-row whose
// entity_id self-references the meta-row's own UUID.
const (
	EntityTypeUser      = "user"
	EntityTypeSession   = "session"
	EntityTypeAlertRule = "alert_rule"
	EntityTypeAlert     = "alert"
	EntityTypeBackupRun = "backup_run"
	EntityTypeAuditLog  = "audit_log"
)

// Entry is the value-shape callers fill in when calling WriteEntry. It maps
// 1:1 onto the audit_log columns minus the DB-defaulted id + time. The 8
// caller-provided columns (D-22) plus the txn handle make audit calls
// identical at every site:
//
//	audit.WriteEntry(ctx, tx, audit.Entry{...})
type Entry struct {
	// UserID — from auth.UserFromContext(ctx) at the handler layer. Zero-uuid
	// (uuid.Nil) is allowed for system-emitted events (rollover_detected,
	// binding_changed seed-sync) and writes SQL NULL via pgtype.UUID.Valid=false.
	UserID uuid.UUID

	// Action — one of the Action* constants. Mismatch raises 23514
	// (audit_log_action_valid CHECK violation) which the caller treats as a bug.
	Action string

	// EntityType — one of the EntityType* constants. Mismatch raises 23514.
	EntityType string

	// EntityID — the id of the row being mutated. For 'swap' actions where
	// two binding rows are touched, the convention is to write a single audit
	// row with EntityID = newBinding.id and reference the closed binding's id
	// inside the Before/After diff JSON.
	EntityID uuid.UUID

	// Before — full row state before the mutation, or the changed-fields diff
	// per D-24. nil for CREATE actions (caller passes nil; Postgres stores SQL NULL).
	Before map[string]any

	// After — full row state for CREATE; changed-fields diff for UPDATE per D-24.
	After map[string]any

	// Notes — free-text human-readable annotation (operator's "why I did this"
	// for swap/decommission actions).
	Notes string

	// RequestID — request-scoped identifier from chi middleware.RequestID
	// (Phase 1). Lets Phase 6 audit browse correlate audit rows that belong
	// to the same API call. Empty string is acceptable (writes SQL NULL).
	RequestID string
}

// WriteEntry inserts an audit row INSIDE the caller's transaction (D-23).
// The audit row is part of the same atomic commit as the domain mutation;
// by Postgres atomicity, an audit row literally cannot exist without its
// domain row, and vice versa.
//
// Pattern 8 (RESEARCH §"Audit log writer"): the tx parameter is a pgx.Tx
// (not a *pgxpool.Pool) so call sites pass the SAME tx the domain mutation
// uses. Calling WriteEntry outside an open txn is impossible by signature.
//
// The audit_log_action_valid + audit_log_entity_type_valid CHECKs (0016)
// raise 23514 if Action / EntityType is not in the allowed vocabulary.
// Callers that hit this error treat it as a bug — the constants in this
// package are the only legal values.
func WriteEntry(ctx context.Context, tx pgx.Tx, e Entry) error {
	beforeJSON, err := marshalDiff(e.Before)
	if err != nil {
		return fmt.Errorf("audit: marshal before: %w", err)
	}
	afterJSON, err := marshalDiff(e.After)
	if err != nil {
		return fmt.Errorf("audit: marshal after: %w", err)
	}

	// pgtype.UUID encodes uuid.Nil as SQL NULL when Valid=false. The
	// audit_log.user_id column is NULLable (FK ON DELETE SET NULL); zero-uuid
	// from a system-emitted event lands as NULL rather than the all-zeros UUID.
	uid := pgtype.UUID{Bytes: e.UserID, Valid: e.UserID != uuid.Nil}

	// Notes / RequestID surface as *string in the generated WriteAuditLogParams
	// (sqlc emit_pointers_for_null_types=true). nil pointer = SQL NULL.
	var notesPtr *string
	if e.Notes != "" {
		s := e.Notes
		notesPtr = &s
	}
	var reqIDPtr *string
	if e.RequestID != "" {
		s := e.RequestID
		reqIDPtr = &s
	}

	q := sqlc.New(tx)
	return q.WriteAuditLog(ctx, sqlc.WriteAuditLogParams{
		UserID:     uid,
		Action:     e.Action,
		EntityType: e.EntityType,
		EntityID:   pgtype.UUID{Bytes: e.EntityID, Valid: true},
		Before:     beforeJSON,
		After:      afterJSON,
		Notes:      notesPtr,
		RequestID:  reqIDPtr,
	})
}

// marshalDiff handles the nil-map case so Postgres receives SQL NULL rather
// than the JSON literal "null" or "{}". Phase 6 audit browse distinguishes
// "no diff captured" (NULL) from "empty diff" ({}) so the encoding has to
// preserve that distinction.
func marshalDiff(m map[string]any) ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	return json.Marshal(m)
}
