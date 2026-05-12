package alert

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// AuditPruneArgs is the River job-args type for the daily audit retention
// prune (D-38 / D-51). Carries no payload — the worker reads
// retention_config.audit_log_days each cycle so a Settings change takes
// effect at the next scheduled run.
type AuditPruneArgs struct{}

// Kind returns the unique job kind identifier consumed by River's worker
// registry. The exact string is matched by alertWorkerKindFromJobKind for
// the degraded subscriber — keep them in lockstep.
func (AuditPruneArgs) Kind() string { return "audit_prune" }

// InsertOpts caps the retry attempts so a permanently-broken prune (e.g.
// the SECURITY DEFINER function dropped, role grants revoked) eventually
// hits JobStateDiscarded and trips the degraded flag — instead of retrying
// forever and silently filling the queue.
//
// Note: audit_prune is not a degraded-tracked alert worker (the degraded
// subscriber only watches kinds in alertWorkerKindFromJobKind). The retry
// cap is a defense-in-depth — operator notices a stale audit-log retention
// via the audit table not shrinking, then checks the River dashboard.
func (AuditPruneArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 3}
}

// AuditPruneWorker runs the daily audit_log retention prune (D-38). It
// calls the SECURITY DEFINER function admin_prune_audit_rows(cutoff_days)
// where cutoff_days = retention_config.audit_log_days at run time.
//
// The function itself handles the trigger bypass + writes a meta
// 'audit.prune' row, so this worker only needs to invoke it and log the
// result. River cron in serve.go schedules this for 03:00 install_tz daily.
type AuditPruneWorker struct {
	river.WorkerDefaults[AuditPruneArgs]

	Pool    *pgxpool.Pool
	Queries *sqlc.Queries // present for future use (currently the worker is a raw SQL call)
	Log     *slog.Logger
}

// Work executes one prune cycle.
//
// Steps:
//  1. Read retention_config.audit_log_days (singleton id=1 row).
//  2. SELECT admin_prune_audit_rows(audit_log_days) — returns the deleted row count.
//  3. Log structured cycle summary.
//
// Errors propagate to River for retry under MaxAttempts=3.
func (w *AuditPruneWorker) Work(ctx context.Context, _ *river.Job[AuditPruneArgs]) error {
	var auditLogDays int32
	if err := w.Pool.QueryRow(ctx,
		`SELECT audit_log_days FROM retention_config WHERE id = 1`,
	).Scan(&auditLogDays); err != nil {
		return fmt.Errorf("audit_prune: read retention_config: %w", err)
	}

	var deleted int32
	if err := w.Pool.QueryRow(ctx,
		`SELECT admin_prune_audit_rows($1)`, auditLogDays,
	).Scan(&deleted); err != nil {
		return fmt.Errorf("audit_prune: admin_prune_audit_rows(%d): %w", auditLogDays, err)
	}

	if w.Log != nil {
		w.Log.Info("audit_prune.cycle",
			"deleted", deleted,
			"cutoff_days", auditLogDays,
		)
	}
	return nil
}
