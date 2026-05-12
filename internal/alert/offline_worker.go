package alert

import (
	"context"

	"github.com/riverqueue/river"
)

// OfflineArgs is the River job-args type for ALERT-02 + ALERT-03 (the
// offline_device + offline_gateway rule kinds share one worker because
// gateway-down suppression requires evaluating both in the same cycle —
// D-14).
//
// RESEARCH §Decision C cadence: 2 minutes.
type OfflineArgs struct{}

// Kind returns the unique River job kind. Must match the
// alertWorkerKindFromJobKind switch in degraded.go.
func (OfflineArgs) Kind() string { return "alert_offline" }

// InsertOpts caps retries at 3 (D-22) so a permanently-broken worker hits
// JobStateDiscarded and trips the degraded flag.
func (OfflineArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 3}
}

// OfflineWorker evaluates offline_device + offline_gateway rules with
// D-14 gateway-down suppression and D-15 hysteresis. The full
// implementation lands in Plan 06-02 Task 3; this file ships the worker
// surface so serve.go can register the worker + cron at Task 2 time
// without breaking the build.
type OfflineWorker struct {
	river.WorkerDefaults[OfflineArgs]

	Eng        EvaluateContext
	Rules      *RuleStore
	Alerts     *AlertStore
	WorkerStat *WorkerStateStore
}

// Work is the cycle entry. Body filled in by Task 3.
func (w *OfflineWorker) Work(ctx context.Context, _ *river.Job[OfflineArgs]) error {
	return nil
}
