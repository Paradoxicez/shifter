package alert

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// TestAlertWorkerKindFromJobKind — the helper maps River job kinds to the
// alert_worker_state.worker_kind enum and returns false for non-alert kinds.
func TestAlertWorkerKindFromJobKind(t *testing.T) {
	cases := []struct {
		jobKind string
		want    string
		ok      bool
	}{
		{"alert_threshold_instantaneous", "threshold_instantaneous", true},
		{"alert_threshold_hourly", "threshold_hourly", true},
		{"alert_threshold_daily", "threshold_daily", true},
		{"alert_offline", "offline", true},
		{"alert_anomaly", "anomaly", true},
		{"audit_prune", "", false},
		{"report_cleanup", "", false},
		{"pdf_report", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := alertWorkerKindFromJobKind(c.jobKind)
		require.Equal(t, c.want, got, "kind=%q", c.jobKind)
		require.Equal(t, c.ok, ok, "kind=%q", c.jobKind)
	}
}

// failingAlertArgs is a job-args type with Kind = "alert_offline" so the
// degraded subscriber recognizes it. The corresponding worker always errors,
// MaxAttempts=1 → next state is JobStateDiscarded.
type failingAlertArgs struct {
	Marker string `json:"marker"`
}

func (failingAlertArgs) Kind() string { return "alert_offline" }
func (failingAlertArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1}
}

type failingAlertWorker struct {
	river.WorkerDefaults[failingAlertArgs]
}

func (failingAlertWorker) Work(ctx context.Context, _ *river.Job[failingAlertArgs]) error {
	return errors.New("intentional failure for degraded subscriber test")
}

// TestDegradedSubscriber_FlipsOnDiscarded — end-to-end test through a real
// River client. Insert a failing job with MaxAttempts=1 → River runs it,
// errors, transitions to JobStateDiscarded → subscriber flips degraded=true
// on alert_worker_state.offline.
func TestDegradedSubscriber_FlipsOnDiscarded(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	store := NewWorkerStateStore(pool)

	// Build a small River client with just the failing worker registered.
	workers := river.NewWorkers()
	river.AddWorker(workers, &failingAlertWorker{})
	rc, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 1}},
		Workers: workers,
	})
	require.NoError(t, err)
	go func() { _ = rc.Start(ctx) }()
	t.Cleanup(func() {
		shutdownCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_ = rc.Stop(shutdownCtx)
	})

	// Start the degraded subscriber on a goroutine.
	subCtx, subCancel := context.WithCancel(ctx)
	defer subCancel()
	StartDegradedSubscriber(subCtx, rc, store, log)

	// Insert + wait for the job to run + fail + be discarded.
	_, err = rc.Insert(ctx, failingAlertArgs{Marker: "go"}, nil)
	require.NoError(t, err)

	// Poll alert_worker_state.offline.degraded until true (or 10s deadline).
	deadline := time.Now().Add(10 * time.Second)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for time.Now().Before(deadline) {
			row, err := store.GetWorkerState(ctx, "offline")
			if err == nil && row.Degraded {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()
	wg.Wait()

	row, err := store.GetWorkerState(ctx, "offline")
	require.NoError(t, err)
	require.True(t, row.Degraded, "subscriber must flip degraded=true after JobStateDiscarded")
	require.NotNil(t, row.LastError, "last_error must be populated")
}

// TestDegradedSubscriber_IgnoresUnrelatedJobs — a non-alert job that fails
// + discards must NOT flip any alert_worker_state row (mitigates T-06-01-06).
func TestDegradedSubscriber_IgnoresUnrelatedJobs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	store := NewWorkerStateStore(pool)

	type unrelatedArgs struct{}
	// Define inline so it can't be mistaken for an alert worker kind.
	// We use the Args-only approach: register a worker that has a non-alert
	// kind. Below we satisfy the river.JobArgs interface explicitly.

	workers := river.NewWorkers()
	river.AddWorker(workers, &unrelatedWorker{})
	rc, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 1}},
		Workers: workers,
	})
	require.NoError(t, err)
	go func() { _ = rc.Start(ctx) }()
	t.Cleanup(func() {
		shutdownCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_ = rc.Stop(shutdownCtx)
	})

	subCtx, subCancel := context.WithCancel(ctx)
	defer subCancel()
	StartDegradedSubscriber(subCtx, rc, store, log)

	_, err = rc.Insert(ctx, unrelatedJobArgs{Marker: "go"}, nil)
	require.NoError(t, err)

	// Wait long enough for the job to fail + discard. Then verify NO alert
	// worker row was flipped.
	time.Sleep(3 * time.Second)
	for _, kind := range []string{"threshold_instantaneous", "threshold_hourly", "threshold_daily", "offline", "anomaly"} {
		row, err := store.GetWorkerState(ctx, kind)
		require.NoError(t, err)
		require.False(t, row.Degraded, "unrelated job failure must not flip degraded for %q", kind)
	}
}

// unrelatedJobArgs has Kind = "definitely_not_an_alert_worker" so the
// degraded subscriber's helper returns ok=false.
type unrelatedJobArgs struct {
	Marker string `json:"marker"`
}

func (unrelatedJobArgs) Kind() string { return "definitely_not_an_alert_worker" }
func (unrelatedJobArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1}
}

type unrelatedWorker struct {
	river.WorkerDefaults[unrelatedJobArgs]
}

func (unrelatedWorker) Work(ctx context.Context, _ *river.Job[unrelatedJobArgs]) error {
	return errors.New("intentional unrelated failure")
}

// keep pgx import used.
var _ pgx.Tx
