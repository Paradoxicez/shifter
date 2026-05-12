package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shifter-io/shifter/internal/audit"
	"github.com/shifter-io/shifter/internal/auth"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// IMPORTANT: NO "github.com/lib/pq" import — CLAUDE.md banned dep.
// Hypertable names in ReconcilePolicies come from a compile-time switch
// (vetted literal set), so no runtime escape helper is needed.

// Deps groups every dependency the settings handlers need.
type Deps struct {
	Pool    *pgxpool.Pool
	Queries *sqlc.Queries
}

// retentionSnapshot is the local subset of retention_config columns the
// handler + reconciler + audit-diff helpers need. sqlc v1.31 emits a
// per-query Row type for GetRetentionConfig + UpdateRetentionConfig (the
// SELECT list no longer matches the full table after migration 0040 added
// alerts_days / audit_log_days, even though we now select those too —
// the row alias is sticky for stability). Converting the sqlc rows into
// this local snapshot in ONE place keeps every other caller stable.
type retentionSnapshot struct {
	ID          int32
	RawDays     int32
	HourlyDays  int32
	DailyDays   int32
	MonthlyDays int32
	YearlyDays  *int32
	// Phase 6 additions (Plan 06-10 / migration 0040):
	AlertsDays   int32
	AuditLogDays int32
	UpdatedAt    pgtype.Timestamptz
}

func fromGetRow(r sqlc.GetRetentionConfigRow) retentionSnapshot {
	return retentionSnapshot{
		ID: r.ID, RawDays: r.RawDays, HourlyDays: r.HourlyDays,
		DailyDays: r.DailyDays, MonthlyDays: r.MonthlyDays,
		YearlyDays:   r.YearlyDays,
		AlertsDays:   r.AlertsDays,
		AuditLogDays: r.AuditLogDays,
		UpdatedAt:    r.UpdatedAt,
	}
}

func fromUpdateRow(r sqlc.UpdateRetentionConfigRow) retentionSnapshot {
	return retentionSnapshot{
		ID: r.ID, RawDays: r.RawDays, HourlyDays: r.HourlyDays,
		DailyDays: r.DailyDays, MonthlyDays: r.MonthlyDays,
		YearlyDays:   r.YearlyDays,
		AlertsDays:   r.AlertsDays,
		AuditLogDays: r.AuditLogDays,
		UpdatedAt:    r.UpdatedAt,
	}
}

// RetentionResponse is the wire shape for GET /api/settings/retention.
type RetentionResponse struct {
	RawDays     int32  `json:"raw_days"`
	HourlyDays  int32  `json:"hourly_days"`
	DailyDays   int32  `json:"daily_days"`
	MonthlyDays int32  `json:"monthly_days"`
	YearlyDays  *int32 `json:"yearly_days"` // null = forever (D-09)
	// Phase 6 additions (Plan 06-10 / migration 0040):
	AlertsDays   int32  `json:"alerts_days"`    // D-13 default 365
	AuditLogDays int32  `json:"audit_log_days"` // D-38 default 1825
	UpdatedAt    string `json:"updated_at"`
}

// RetentionPatch is the wire shape for PATCH /api/settings/retention.
//
// Fields are pointers so omitted fields can be distinguished from zero values.
// YearlyForever is a sentinel: when true, yearly_days is set to NULL (forever)
// regardless of the YearlyDays value. See doc.go for the full sentinel protocol.
type RetentionPatch struct {
	RawDays       *int32 `json:"raw_days"`
	HourlyDays    *int32 `json:"hourly_days"`
	DailyDays     *int32 `json:"daily_days"`
	MonthlyDays   *int32 `json:"monthly_days"`
	YearlyDays    *int32 `json:"yearly_days"`
	YearlyForever *bool  `json:"yearly_forever"` // sentinel: true → NULL out yearly_days
	// Phase 6 additions (Plan 06-10 / migration 0040):
	AlertsDays   *int32 `json:"alerts_days"`
	AuditLogDays *int32 `json:"audit_log_days"`
}

// GetHandler serves GET /api/settings/retention.
// Both admin and viewer can read retention settings (AUTH-06 read access).
func GetHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg, err := deps.Queries.GetRetentionConfig(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "load_failed")
			return
		}
		writeJSON(w, http.StatusOK, toResponse(fromGetRow(cfg)))
	}
}

// PatchHandler serves PATCH /api/settings/retention.
// Admin-only (T-05-11-01). Validates ranges, updates retention_config and
// reconciles TimescaleDB policies inside the same pgx.Tx, then writes an
// audit entry before commit.
//
// Phase 6 Plan 06-10: alerts_days and audit_log_days are updated in the same
// transaction as the Phase 5 fields. TimescaleDB policy reconciliation does
// NOT apply to alerts/audit_log — those tables are not hypertables; their
// retention is enforced by dedicated pruning workers that read retention_config.
// TODO (Plan 06-11 task 1): wire the alerts-prune worker that reads alerts_days.
func PatchHandler(deps Deps, sm *scs.SessionManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Auth: admin-only per T-05-11-01. RequireAction middleware at the router
		// level also gates this; Can() here is defense-in-depth.
		user, ok := auth.GetUser(r.Context(), sm)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if !auth.Can(&user, auth.ActionSettingsUpdate, nil) {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}

		var patch RetentionPatch
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json")
			return
		}
		if err := validatePatch(patch); err != nil {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}

		tx, err := deps.Pool.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "tx_begin")
			return
		}
		defer tx.Rollback(r.Context()) //nolint:errcheck

		q := deps.Queries.WithTx(tx)

		// Snapshot before-state for audit diff.
		beforeRow, err := q.GetRetentionConfig(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "load_failed")
			return
		}
		before := fromGetRow(beforeRow)

		// Resolve yearly_days using the sentinel protocol (see doc.go):
		//   yearly_forever=true  → NULL (forever)
		//   yearly_days=N        → N
		//   neither present      → keep existing value
		yearlyDays := resolveYearly(patch, before.YearlyDays)

		updatedRow, err := q.UpdateRetentionConfig(r.Context(), sqlc.UpdateRetentionConfigParams{
			RawDays:      patch.RawDays,
			HourlyDays:   patch.HourlyDays,
			DailyDays:    patch.DailyDays,
			MonthlyDays:  patch.MonthlyDays,
			YearlyDays:   yearlyDays,
			AlertsDays:   patch.AlertsDays,
			AuditLogDays: patch.AuditLogDays,
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "update_failed")
			return
		}
		updated := fromUpdateRow(updatedRow)

		// Same-tx policy reconciliation: remove + re-add TimescaleDB retention
		// policies for every level that changed. If any policy call fails, the
		// whole tx rolls back — retention_config and actual policies stay in sync.
		// NOTE: alerts/audit_log are NOT hypertables; ReconcilePolicies only
		// reconciles the 5 measurement hypertables — alerts/audit_log are pruned
		// by dedicated workers that read retention_config directly.
		if err := ReconcilePolicies(r.Context(), tx, before, updated); err != nil {
			writeError(w, http.StatusInternalServerError, "policy_reconcile_failed")
			return
		}

		// Audit entry — before/after diff of only the changed fields.
		uid := uuid.MustParse(user.ID)
		beforeMap, afterMap := diffFields(before, updated)
		if err := audit.WriteEntry(r.Context(), tx, audit.Entry{
			UserID:     uid,
			Action:     audit.ActionRetentionChange,
			EntityType: audit.EntityTypeRetention,
			EntityID:   uuid.Nil, // singleton has no UUID; uuid.Nil is conventional
			Before:     beforeMap,
			After:      afterMap,
			RequestID:  middleware.GetReqID(r.Context()),
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "audit_failed")
			return
		}

		if err := tx.Commit(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "tx_commit")
			return
		}

		writeJSON(w, http.StatusOK, toResponse(updated))
	}
}

// ReconcilePolicies removes + re-adds the matching TimescaleDB retention policy
// for each level that changed value. yearly NULL → ensure no policy exists
// (remove only). Same tx as the config row UPDATE so a rollback restores both.
//
// SECURITY / CLAUDE.md compliance: the hypertable name in the SQL template
// comes from a compile-time switch — it is NEVER a runtime string from
// client input. This removes the need for any quoting/escaping helper (and
// specifically the banned "github.com/lib/pq" pq.QuoteLiteral). The interval
// value is bound via pgx parameter binding ($1) so the integer day count
// cannot inject anything.
func ReconcilePolicies(ctx context.Context, tx pgx.Tx, before, after retentionSnapshot) error {
	type level struct {
		name   string
		before *int32
		after  *int32
	}

	levels := []level{
		{"measurement", &before.RawDays, &after.RawDays},
		{"measurement_hourly", &before.HourlyDays, &after.HourlyDays},
		{"measurement_daily", &before.DailyDays, &after.DailyDays},
		{"measurement_monthly", &before.MonthlyDays, &after.MonthlyDays},
		{"measurement_yearly", before.YearlyDays, after.YearlyDays},
		// alerts_days and audit_log_days are intentionally NOT here:
		// those tables are not TimescaleDB hypertables and do not have
		// TimescaleDB retention policies. Their retention is enforced by
		// dedicated workers (AuditPruneWorker in Plan 06-01; alerts-prune
		// worker in Plan 06-11 task 1) that read retention_config directly.
	}

	for _, l := range levels {
		// Skip if unchanged.
		if int32PtrEq(l.before, l.after) {
			continue
		}

		// Resolve the hypertable name from a vetted compile-time literal set.
		// This is the ONLY way the name reaches the SQL template — no runtime
		// strings, no quoting helper, no lib/pq. The default arm is the
		// defense-in-depth gate (TestRetentionConfig_UnknownHypertable_Rejected).
		var hypertable string
		switch l.name {
		case "measurement":
			hypertable = "measurement"
		case "measurement_hourly":
			hypertable = "measurement_hourly"
		case "measurement_daily":
			hypertable = "measurement_daily"
		case "measurement_monthly":
			hypertable = "measurement_monthly"
		case "measurement_yearly":
			hypertable = "measurement_yearly"
		default:
			return fmt.Errorf("retention: unknown hypertable %q", l.name)
		}

		// remove_retention_policy: hypertable is a vetted literal; if_exists
		// prevents errors when no policy exists yet (e.g. yearly=forever).
		removeSQL := fmt.Sprintf(`SELECT remove_retention_policy('%s', if_exists => true)`, hypertable)
		if _, err := tx.Exec(ctx, removeSQL); err != nil {
			return fmt.Errorf("remove policy %s: %w", hypertable, err)
		}

		// add_retention_policy: hypertable is the vetted literal; the interval
		// VALUE is bound via $1 (pgx parameter binding) so the integer day count
		// cannot inject anything. Skip when after=nil (forever — remove only).
		if l.after != nil {
			addSQL := fmt.Sprintf(`SELECT add_retention_policy('%s', make_interval(days => $1))`, hypertable)
			if _, err := tx.Exec(ctx, addSQL, int(*l.after)); err != nil {
				return fmt.Errorf("add policy %s: %w", hypertable, err)
			}
		}
	}
	return nil
}

// validatePatch enforces the UI-SPEC range bounds for each retention level.
// Returns an error string used directly as the 422 response body "error" field.
func validatePatch(p RetentionPatch) error {
	if p.RawDays != nil && (*p.RawDays < 30 || *p.RawDays > 365) {
		return errors.New("raw_days_out_of_range")
	}
	if p.HourlyDays != nil && (*p.HourlyDays < 180 || *p.HourlyDays > 1825) {
		return errors.New("hourly_days_out_of_range")
	}
	if p.DailyDays != nil && (*p.DailyDays < 365 || *p.DailyDays > 7300) {
		return errors.New("daily_days_out_of_range")
	}
	if p.MonthlyDays != nil && (*p.MonthlyDays < 1825 || *p.MonthlyDays > 18250) {
		return errors.New("monthly_days_out_of_range")
	}
	if p.YearlyDays != nil && *p.YearlyDays < 1825 {
		return errors.New("yearly_days_out_of_range")
	}
	// Phase 6 additions (Plan 06-10): alerts_days and audit_log_days range checks.
	// Mirrors the CHECK constraints added by migration 0040.
	if p.AlertsDays != nil && (*p.AlertsDays < 30 || *p.AlertsDays > 3650) {
		return errors.New("alerts_days_out_of_range")
	}
	if p.AuditLogDays != nil && (*p.AuditLogDays < 90 || *p.AuditLogDays > 18250) {
		return errors.New("audit_log_days_out_of_range")
	}
	return nil
}

// resolveYearly applies the sentinel protocol for the yearly_days field.
// See doc.go for full protocol description.
func resolveYearly(patch RetentionPatch, current *int32) *int32 {
	if patch.YearlyForever != nil && *patch.YearlyForever {
		return nil // explicit forever
	}
	if patch.YearlyDays != nil {
		return patch.YearlyDays // explicit value
	}
	return current // unchanged — keep existing
}

// diffFields returns before/after maps containing only fields that changed.
// Used for the audit entry (D-24 changed-fields diff).
func diffFields(before, after retentionSnapshot) (beforeMap, afterMap map[string]any) {
	beforeMap = make(map[string]any)
	afterMap = make(map[string]any)

	if before.RawDays != after.RawDays {
		beforeMap["raw_days"] = before.RawDays
		afterMap["raw_days"] = after.RawDays
	}
	if before.HourlyDays != after.HourlyDays {
		beforeMap["hourly_days"] = before.HourlyDays
		afterMap["hourly_days"] = after.HourlyDays
	}
	if before.DailyDays != after.DailyDays {
		beforeMap["daily_days"] = before.DailyDays
		afterMap["daily_days"] = after.DailyDays
	}
	if before.MonthlyDays != after.MonthlyDays {
		beforeMap["monthly_days"] = before.MonthlyDays
		afterMap["monthly_days"] = after.MonthlyDays
	}
	if !int32PtrEq(before.YearlyDays, after.YearlyDays) {
		beforeMap["yearly_days"] = before.YearlyDays
		afterMap["yearly_days"] = after.YearlyDays
	}
	// Phase 6 additions:
	if before.AlertsDays != after.AlertsDays {
		beforeMap["alerts_days"] = before.AlertsDays
		afterMap["alerts_days"] = after.AlertsDays
	}
	if before.AuditLogDays != after.AuditLogDays {
		beforeMap["audit_log_days"] = before.AuditLogDays
		afterMap["audit_log_days"] = after.AuditLogDays
	}
	return beforeMap, afterMap
}

// toResponse converts a retentionSnapshot to the wire response shape.
func toResponse(cfg retentionSnapshot) RetentionResponse {
	return RetentionResponse{
		RawDays:      cfg.RawDays,
		HourlyDays:   cfg.HourlyDays,
		DailyDays:    cfg.DailyDays,
		MonthlyDays:  cfg.MonthlyDays,
		YearlyDays:   cfg.YearlyDays,
		AlertsDays:   cfg.AlertsDays,
		AuditLogDays: cfg.AuditLogDays,
		UpdatedAt:    cfg.UpdatedAt.Time.Format(time.RFC3339),
	}
}

// int32PtrEq returns true iff both pointers point to the same value (or both nil).
func int32PtrEq(a, b *int32) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

type errorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, errorResponse{Error: msg})
}
