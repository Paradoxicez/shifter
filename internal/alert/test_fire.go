// Package alert — test-fire support (D-19). The "Test fire" button in
// AddRuleDialog (Plan 06-04 UI) creates a synthetic alert tagged
// is_test=true that auto-clears 60 seconds later via a River one-shot job.
//
// PITFALL 8 mitigation: TestFireHandler explicitly does NOT call
// RuleStore.TouchLastFiredAt — test fires must not start the cooldown
// timer or they would suppress real fires that legitimately need to raise
// for the same (rule, target).
package alert

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	sqlcdb "github.com/shifter-io/shifter/internal/db/sqlc"

	"github.com/shifter-io/shifter/internal/audit"
	"github.com/shifter-io/shifter/internal/auth"
)

// TestFireDeps bundles the test-fire wiring. Constructed once at boot and
// shared with the HTTP handler + the auto-clear worker.
type TestFireDeps struct {
	HTTPDeps
	// EnqueueClear is the closure that schedules a one-shot
	// TestFireClearArgs job 60 seconds in the future. The serve.go wiring
	// fills this in with a riverClient.InsertTx call so the schedule lands
	// atomically with the synthetic alert insert + audit row (D-23).
	EnqueueClear func(ctx context.Context, tx pgx.Tx, alertID uuid.UUID, scheduledAt time.Time) error
}

// TestFireClearArgs is the River one-shot job that clears a test-fire alert
// 60s after creation (D-19). Carries only the alert id — every other
// detail (cleared_at, audit row) is computed at Work() time.
type TestFireClearArgs struct {
	AlertID uuid.UUID `json:"alert_id"`
}

// Kind returns the unique job kind. Distinct from the periodic alert
// worker kinds — the degraded subscriber (degraded.go) does NOT track this
// kind because failures here don't gate the alert engine's overall health.
func (TestFireClearArgs) Kind() string { return "alert_test_fire_clear" }

// InsertOpts: cap retries at 3 so a permanently-broken auto-clear cycle
// (e.g. someone deleted the alert) eventually JobStateDiscards instead of
// looping forever.
func (TestFireClearArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 3}
}

// TestFireClearWorker clears the test-fire alert + writes the corresponding
// 'alert.cleared' audit row in the SAME tx (D-23). Idempotent: if the alert
// was already cleared (e.g. operator hit the manual Clear button before the
// 60s timer), the worker treats it as a no-op.
type TestFireClearWorker struct {
	river.WorkerDefaults[TestFireClearArgs]

	Pool  *pgxpool.Pool
	Store *AlertStore
	Log   *slog.Logger
}

// Work runs one clear cycle.
func (w *TestFireClearWorker) Work(ctx context.Context, job *river.Job[TestFireClearArgs]) error {
	if w.Pool == nil || w.Store == nil {
		return errors.New("alert: test-fire clear worker not wired")
	}
	tx, err := w.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := w.Store.ClearAlert(ctx, tx, job.Args.AlertID); err != nil {
		// "Not found" or already-cleared is a no-op — operator manually
		// cleared the test alert before the 60s timer.
		if errors.Is(err, ErrAlertNotFound) {
			return tx.Commit(ctx)
		}
		return fmt.Errorf("alert test-fire clear: %w", err)
	}
	if err := audit.WriteEntry(ctx, tx, audit.Entry{
		Action:     audit.ActionAlertCleared,
		EntityType: audit.EntityTypeAlert,
		EntityID:   job.Args.AlertID,
		Notes:      "auto-cleared after 60s test-fire window (D-19)",
	}); err != nil {
		return fmt.Errorf("alert test-fire audit: %w", err)
	}
	if w.Log != nil {
		w.Log.Info("alert test-fire cleared", "alert_id", job.Args.AlertID)
	}
	return tx.Commit(ctx)
}

// TestFireHandler — POST /api/alerts/rules/{id}/test-fire.
//
// Behavior (D-19):
//  1. Load the rule (404 if not found).
//  2. Build a synthetic D-12 payload tagged "TEST".
//  3. Open tx → InsertAlert(is_test=true) → audit 'alert.test_fired' →
//     enqueue TestFireClearArgs at now()+60s → commit.
//  4. Does NOT call rules.TouchLastFiredAt — see Pitfall 8.
//
// The synthetic alert uses the rule's first valid target so the operator
// sees "TEST: <rule name> — <target label>" in the drawer. If the rule has
// no resolvable target (e.g. global rule with zero active MPs), the test
// fire targets the rule itself via target_entity_type='alert_rule' so the
// alert still appears in the drawer with a meaningful label.
func TestFireHandler(deps TestFireDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acting, ok := auth.GetUser(r.Context(), deps.SessionMgr)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errorResp{Error: "unauthorized"})
			return
		}
		idRaw := chi.URLParam(r, "id")
		ruleID, err := uuid.Parse(idRaw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "invalid_id"})
			return
		}
		actingUUID, err := uuid.Parse(acting.ID)
		if err != nil {
			internalError(deps.Log, w, "parse acting id", err)
			return
		}
		ctx := r.Context()

		rule, err := deps.Rules.GetRuleByID(ctx, ruleID)
		if errors.Is(err, ErrRuleNotFound) {
			writeJSON(w, http.StatusNotFound, errorResp{Error: "not_found"})
			return
		}
		if err != nil {
			internalError(deps.Log, w, "load rule", err)
			return
		}

		// Build a synthetic target. If the rule is MP-scoped, use the MP;
		// otherwise fall back to a synthetic "this rule" pseudo-target so
		// the alert still appears in the drawer.
		targetType, targetID, targetLabel := pickSyntheticTarget(ctx, deps, rule)

		queries := sqlcdb.New(deps.Pool)
		installName := readSyntheticInstallName(ctx, queries)
		payload, err := BuildPayload(BuildPayloadInput{
			RuleID:      rule.ID,
			RuleKind:    rule.RuleKind,
			Severity:    "info",
			Target:      PayloadTarget{EntityType: targetType, EntityID: targetID, Label: "TEST: " + targetLabel},
			Value:       0,
			Threshold:   0,
			Comparison:  derefStr(rule.Comparison, "gt"),
			Unit:        derefStr(rule.Unit, ""),
			FiredAt:     time.Now().UTC(),
			InstallName: installName,
		})
		if err != nil {
			internalError(deps.Log, w, "build payload", err)
			return
		}

		tx, err := deps.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
		if err != nil {
			internalError(deps.Log, w, "begin tx", err)
			return
		}
		defer func() { _ = tx.Rollback(ctx) }()

		row, err := deps.Alerts.InsertAlert(ctx, tx, InsertAlertParams{
			RuleID:           rule.ID,
			RuleKind:         rule.RuleKind,
			Severity:         "info",
			Payload:          payload,
			TargetEntityType: targetType,
			TargetEntityID:   targetID,
			IsTest:           true,
		})
		if err != nil {
			if errors.Is(err, ErrDuplicateFire) {
				writeJSON(w, http.StatusConflict, errorResp{Error: "duplicate_fire"})
				return
			}
			internalError(deps.Log, w, "insert test alert", err)
			return
		}
		if err := audit.WriteEntry(ctx, tx, audit.Entry{
			UserID:     actingUUID,
			Action:     audit.ActionAlertTestFired,
			EntityType: audit.EntityTypeAlert,
			EntityID:   row.ID,
			Notes:      "rule.test_fire button (D-19); auto-clears in 60s",
			RequestID:  middleware.GetReqID(ctx),
		}); err != nil {
			internalError(deps.Log, w, "audit test fire", err)
			return
		}

		// IMPORTANT: Does NOT call rules.TouchLastFiredAt (Pitfall 8).
		// Test fires must not start the cooldown timer.

		// Schedule the 60s auto-clear inside the same tx so the schedule
		// lands atomically with the alert (D-23). EnqueueClear may be nil
		// in test envs that don't have a River client wired.
		if deps.EnqueueClear != nil {
			scheduledAt := time.Now().UTC().Add(60 * time.Second)
			if err := deps.EnqueueClear(ctx, tx, row.ID, scheduledAt); err != nil {
				internalError(deps.Log, w, "enqueue test-fire clear", err)
				return
			}
		}
		if err := tx.Commit(ctx); err != nil {
			internalError(deps.Log, w, "commit", err)
			return
		}
		writeJSON(w, http.StatusOK, alertToDTO(row))
	}
}

// pickSyntheticTarget returns the target metadata for a test-fire. For
// metering_point-scoped rules, uses the rule's MP. Otherwise points at the
// first active MP, or — if none exist — uses the rule itself as a synthetic
// target.
func pickSyntheticTarget(ctx context.Context, deps TestFireDeps, rule RuleRecord) (string, uuid.UUID, string) {
	if rule.ScopeKind == "metering_point" && rule.ScopeID != nil {
		queries := sqlcdb.New(deps.Pool)
		row, err := queries.GetMeteringPointLabel(ctx, pgUUID(*rule.ScopeID))
		if err == nil {
			return "metering_point", uuid.UUID(row.ID.Bytes), row.Label
		}
	}
	queries := sqlcdb.New(deps.Pool)
	rows, err := queries.ListAllActiveMeteringPoints(ctx)
	if err == nil && len(rows) > 0 {
		first := rows[0]
		return "metering_point", uuid.UUID(first.ID.Bytes), first.Label
	}
	// No active MP — synthetic target = the rule itself.
	label := derefStr(rule.Name, rule.RuleKind)
	return "alert_rule", rule.ID, label
}

func readSyntheticInstallName(ctx context.Context, q *sqlcdb.Queries) string {
	name, err := q.GetInstallDisplayName(ctx)
	if err != nil {
		return ""
	}
	return name
}
