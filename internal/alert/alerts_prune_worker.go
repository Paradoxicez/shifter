package alert

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/shifter-io/shifter/internal/audit"
)

// AlertsPruneArgs is the River job-args type for the daily alerts retention
// prune (D-13 / Plan 06-11). Carries no payload — the worker reads
// retention_config.alerts_days each cycle so a Settings change takes effect
// at the next scheduled run.
type AlertsPruneArgs struct{}

// Kind returns the unique job kind identifier consumed by River's worker
// registry.
func (AlertsPruneArgs) Kind() string { return "alerts_prune" }

// InsertOpts caps retry attempts. A permanently-broken prune eventually hits
// JobStateDiscarded rather than retrying forever.
func (AlertsPruneArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 3}
}

// AlertsPruneWorker runs the daily alerts retention prune (D-13). It deletes
// alert rows where fired_at is older than retention_config.alerts_days and
// writes a meta 'alert.pruned' audit row per cycle.
//
// The alerts table has no INSERT-ONLY trigger (unlike audit_log) so a plain
// DELETE inside a transaction is sufficient — no SECURITY DEFINER function
// needed (contrast with AuditPruneWorker which calls admin_prune_audit_rows).
//
// River cron in serve.go schedules this for 03:30 install_tz daily (30
// minutes after the audit prune at 03:00, so they don't contend on
// retention_config).
type AlertsPruneWorker struct {
	river.WorkerDefaults[AlertsPruneArgs]

	Pool *pgxpool.Pool
	Log  *slog.Logger
}

// Work executes one prune cycle.
//
// Steps:
//  1. Read retention_config.alerts_days (singleton id=1 row).
//  2. DELETE FROM alert WHERE fired_at < now() - make_interval(days => alerts_days).
//  3. Write a meta 'alert.pruned' audit row with deleted-count notes.
//  4. Commit.
//
// Errors propagate to River for retry under MaxAttempts=3.
func (w *AlertsPruneWorker) Work(ctx context.Context, _ *river.Job[AlertsPruneArgs]) error {
	// 1. Read retention config.
	var alertsDays int32
	if err := w.Pool.QueryRow(ctx,
		`SELECT alerts_days FROM retention_config WHERE id = 1`,
	).Scan(&alertsDays); err != nil {
		return fmt.Errorf("alerts_prune: read retention_config: %w", err)
	}

	// 2. Open transaction.
	tx, err := w.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("alerts_prune: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback on non-commit path is intentional

	// 3. DELETE stale alert rows (all states — D-13 hard cap includes firing).
	var deleted int64
	if err := tx.QueryRow(ctx,
		`WITH del AS (
			DELETE FROM alert
			WHERE fired_at < now() - make_interval(days => $1::int)
			RETURNING id
		) SELECT count(*) FROM del`,
		alertsDays,
	).Scan(&deleted); err != nil {
		return fmt.Errorf("alerts_prune: delete stale alerts: %w", err)
	}

	// 4. Write meta audit row 'alert.pruned'. A new UUID is generated for
	//    every cycle so successive prune cycles are independently queryable
	//    in audit history (mirrors the D-51 audit.prune meta-row pattern).
	var metaID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT gen_random_uuid()`).Scan(&metaID); err != nil {
		return fmt.Errorf("alerts_prune: gen meta id: %w", err)
	}
	if err := audit.WriteEntry(ctx, tx, audit.Entry{
		Action:     audit.ActionAlertPruned,
		EntityType: audit.EntityTypeAlert,
		EntityID:   metaID,
		Notes:      fmt.Sprintf("Pruned %d alerts older than %d days", deleted, alertsDays),
	}); err != nil {
		return fmt.Errorf("alerts_prune: write audit row: %w", err)
	}

	// 5. Commit.
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("alerts_prune: commit: %w", err)
	}

	if w.Log != nil {
		w.Log.Info("alerts_prune.cycle",
			"deleted", deleted,
			"cutoff_days", alertsDays,
		)
	}
	return nil
}

