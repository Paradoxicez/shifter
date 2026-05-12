package alert

import (
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
	"github.com/shifter-io/shifter/internal/events"
)

// EvaluateContext is the dependency bundle shared by every alert worker.
//
// Held by the worker struct, NOT passed per-call (River's Work signature is
// fixed). One instance per process. Every field is goroutine-safe so a single
// EvaluateContext can be shared by all three workers without locking.
//
// Workers receive a *pgx.Tx from EvaluateContext.Pool.BeginTx for each
// evaluation cycle so the per-cycle observability UPSERT (alert_worker_state)
// commits in the same transaction as any fires raised that cycle (D-23
// audit-in-tx applies transitively to fired-alert audit rows).
type EvaluateContext struct {
	// Pool is the project pgxpool used for both reads (rule + measurement
	// queries) and writes (fired-alert inserts + worker-state UPSERTs).
	Pool *pgxpool.Pool

	// Queries is a sqlc-generated *Queries handle bound to the pool. Workers
	// can pass tx-bound versions to per-row helpers via sqlc.New(tx).
	Queries *sqlc.Queries

	// Hub is the Phase 4 events.Hub. Plan 06-04 may publish fire events
	// onto an `alert` topic for drawer live-refresh; this plan's substrate
	// holds the reference so workers don't need a second wiring point.
	Hub *events.Hub

	// InstallTZ is the install-state timezone (e.g. *time.Location for
	// "Europe/Stockholm"). Used by anomaly_quiet_hour to interpret the
	// operator-configured quiet window in install-local time. Workers
	// read it per eval cycle so a timezone change in Settings takes
	// effect at the next cycle without restart.
	InstallTZ *time.Location

	// Log is the slog logger, typically the serve-bound logger with
	// component="alert_engine" pre-set.
	Log *slog.Logger
}

// CompareBound returns true when value breaches either bound.
//
//	high (non-nil): value > *high  → breach
//	low  (non-nil): value < *low   → breach
//
// Both bounds nil returns false. Both bounds non-nil treats either side as a
// breach (the rule operator picks the side; the engine evaluates both).
// Threshold subtypes call this with the value pulled from measurement /
// cagg_hourly / cagg_daily; offline + anomaly evaluators have their own
// breach logic.
func CompareBound(value float64, high, low *float64) bool {
	if high != nil && value > *high {
		return true
	}
	if low != nil && value < *low {
		return true
	}
	return false
}
