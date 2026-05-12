package alert

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// WorkerStateStore is the typed surface for the alert_worker_state table.
//
// Plan 06-01 only needs UpsertWorkerState (per-cycle observability bump) and
// MarkWorkerDegraded (D-22 retry-exhaustion sentinel — flipped by the River
// subscriber in degraded.go). Plan 06-04 will add a read surface for
// /health/detailed.
type WorkerStateStore struct {
	pool *pgxpool.Pool
}

// NewWorkerStateStore constructs a WorkerStateStore.
func NewWorkerStateStore(pool *pgxpool.Pool) *WorkerStateStore {
	return &WorkerStateStore{pool: pool}
}

// RunState accumulates per-cycle statistics inside a worker. The worker
// builds one of these as it iterates rules, then calls
// WorkerStateStore.UpsertWorkerState at the end of the cycle to persist.
// recordErr captures the FIRST error encountered (subsequent errors are
// logged but not surfaced — Plan 06-04 may surface error_count separately).
type RunState struct {
	RulesEvaluated int32
	FiresEmitted   int32
	Cleared        int32
	DurationMS     int32
	FirstErr       string
}

// RecordErr stores the first non-empty error message in FirstErr. Subsequent
// calls are no-ops.
func (r *RunState) RecordErr(err error) {
	if err == nil || r.FirstErr != "" {
		return
	}
	r.FirstErr = err.Error()
}

// UpsertWorkerState writes per-cycle stats. The five worker_kind rows are
// pre-seeded by 0042 so UPSERT here is equivalent to UPDATE; we keep the
// ON CONFLICT clause defensively in case a future seed deletes a row.
//
// A successful cycle clears the degraded flag and last_error — only the
// River subscriber (degraded.go) sets degraded=true.
func (s *WorkerStateStore) UpsertWorkerState(ctx context.Context, tx pgx.Tx, kind string, state *RunState) error {
	if state == nil {
		return errors.New("alert: nil RunState")
	}
	q := `
		INSERT INTO alert_worker_state
		    (worker_kind, last_run_at, rules_evaluated, fires_emitted,
		     cleared, duration_ms, degraded, last_error, updated_at)
		VALUES ($1, now(), $2, $3, $4, $5, FALSE, NULLIF($6, ''), now())
		ON CONFLICT (worker_kind) DO UPDATE SET
		    last_run_at     = EXCLUDED.last_run_at,
		    rules_evaluated = EXCLUDED.rules_evaluated,
		    fires_emitted   = EXCLUDED.fires_emitted,
		    cleared         = EXCLUDED.cleared,
		    duration_ms     = EXCLUDED.duration_ms,
		    degraded        = FALSE,
		    last_error      = EXCLUDED.last_error,
		    updated_at      = EXCLUDED.updated_at`
	_, err := tx.Exec(ctx, q, kind,
		state.RulesEvaluated, state.FiresEmitted,
		state.Cleared, state.DurationMS,
		state.FirstErr)
	if err != nil {
		return fmt.Errorf("alert: upsert worker state %q: %w", kind, err)
	}
	return nil
}

// MarkWorkerDegraded is called by the River subscriber when a job reaches
// rivertype.JobStateDiscarded (retries exhausted). It does NOT reset the
// per-cycle stats — Plan 06-04 surfaces both the staleness AND the degraded
// flag so the shell banner can render "Alert evaluation degraded — last
// successful run 2h ago".
//
// Uses the pool directly (not a tx) because the subscriber runs in its own
// goroutine outside any caller tx.
func (s *WorkerStateStore) MarkWorkerDegraded(ctx context.Context, kind, lastError string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE alert_worker_state
		 SET degraded = TRUE, last_error = NULLIF($2, ''), updated_at = now()
		 WHERE worker_kind = $1`,
		kind, lastError)
	if err != nil {
		return fmt.Errorf("alert: mark degraded %q: %w", kind, err)
	}
	return nil
}

// WorkerStateRow mirrors a single alert_worker_state row.
type WorkerStateRow struct {
	WorkerKind     string
	LastRunAt      time.Time
	RulesEvaluated int32
	FiresEmitted   int32
	Cleared        int32
	DurationMS     int32
	Degraded       bool
	LastError      *string
	UpdatedAt      time.Time
}

// GetWorkerState returns the row for kind (or pgx.ErrNoRows).
func (s *WorkerStateStore) GetWorkerState(ctx context.Context, kind string) (WorkerStateRow, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT worker_kind, last_run_at, rules_evaluated, fires_emitted,
		        cleared, duration_ms, degraded, last_error, updated_at
		 FROM alert_worker_state WHERE worker_kind = $1`, kind)
	var r WorkerStateRow
	err := row.Scan(&r.WorkerKind, &r.LastRunAt, &r.RulesEvaluated, &r.FiresEmitted,
		&r.Cleared, &r.DurationMS, &r.Degraded, &r.LastError, &r.UpdatedAt)
	if err != nil {
		return WorkerStateRow{}, fmt.Errorf("alert: get worker state %q: %w", kind, err)
	}
	return r, nil
}
