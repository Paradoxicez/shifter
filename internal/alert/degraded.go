package alert

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// alertWorkerKindFromJobKind maps River job Kind strings to the
// alert_worker_state.worker_kind enum values. Returns ("", false) for any
// kind that isn't an alert worker (audit_prune, report_cleanup, etc.) so the
// degraded subscriber can ignore unrelated job failures without crashing.
//
// Plan 06-02 and 06-03 will add the threshold + offline + anomaly worker
// kinds; for substrate purposes Plan 06-01 only needs the audit_prune entry,
// but the mapping covers every kind the rest of Phase 6 will register so a
// later plan doesn't have to touch this file.
func alertWorkerKindFromJobKind(jobKind string) (string, bool) {
	switch jobKind {
	case "alert_threshold_instantaneous":
		return "threshold_instantaneous", true
	case "alert_threshold_hourly":
		return "threshold_hourly", true
	case "alert_threshold_daily":
		return "threshold_daily", true
	case "alert_offline":
		return "offline", true
	case "alert_anomaly":
		return "anomaly", true
	default:
		// audit_prune, report_cleanup, pdf_report etc. — not an alert worker.
		return "", false
	}
}

// StartDegradedSubscriber spawns a goroutine that listens for
// EventKindJobFailed events from the River client. When a job's state
// transitions to rivertype.JobStateDiscarded (retries exhausted), the
// subscriber flips alert_worker_state.degraded=true for the corresponding
// worker_kind so the shell banner can render "Alert evaluation degraded".
//
// Cancelling ctx releases the subscription cleanly. Non-alert job failures
// are silently dropped (alertWorkerKindFromJobKind returns false).
//
// The Subscribe channel receives ALL EventKindJobFailed events (including
// transient errors that will be retried). We only act on JobStateDiscarded
// so transient retries don't trip the degraded flag prematurely.
//
// The client type parameter is pgx.Tx because serve.go's *river.Client is
// generic on the same type (riverpgxv5 driver). Test helpers construct a
// client with the same type parameter via riverpgxv5.New(pool).
func StartDegradedSubscriber(ctx context.Context, client *river.Client[pgx.Tx], store *WorkerStateStore, log *slog.Logger) {
	ch, cancel := client.Subscribe(river.EventKindJobFailed)
	go func() {
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-ch:
				if !ok {
					return
				}
				if ev == nil || ev.Job == nil {
					continue
				}
				if ev.Job.State != rivertype.JobStateDiscarded {
					// Retryable failure — wait for the next retry or
					// discard before flipping the flag.
					continue
				}
				kind, ok := alertWorkerKindFromJobKind(ev.Job.Kind)
				if !ok {
					continue
				}
				errMsg := ""
				if len(ev.Job.Errors) > 0 {
					errMsg = ev.Job.Errors[len(ev.Job.Errors)-1].Error
				}
				log.Error("alert_worker_degraded",
					"kind", kind, "job_kind", ev.Job.Kind,
					"attempts", ev.Job.Attempt, "err", errMsg)
				if err := store.MarkWorkerDegraded(ctx, kind, errMsg); err != nil {
					log.Error("alert_worker_degraded: MarkWorkerDegraded failed",
						"kind", kind, "err", err)
				}
			}
		}
	}()
}
