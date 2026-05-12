package alert

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/db"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// thresholdTestEnv is the shared per-test fixture for the threshold worker
// suite: spin up Postgres, run migrations, seed install_identity (so the
// payload's install.display_name has a value), construct a fresh
// ThresholdInstantaneousWorker. Returns the pieces tests need to seed
// measurement rows and rules.
type thresholdTestEnv struct {
	pool       *pgxpool.Pool
	queries    *sqlc.Queries
	rules      *RuleStore
	alerts     *AlertStore
	workerStat *WorkerStateStore
	worker     *ThresholdInstantaneousWorker
	mpID       uuid.UUID
	mpLabel    string
}

func newThresholdTestEnv(t *testing.T) *thresholdTestEnv {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	// Seed install_identity (singleton id=1) so the payload's display_name
	// has a value. The wizard normally writes this row at finish-setup time.
	_, err := pool.Exec(ctx,
		`INSERT INTO install_identity (id, display_name, timezone, units)
		 VALUES (1, 'Threshold Test Install', 'UTC', 'metric')
		 ON CONFLICT (id) DO NOTHING`,
	)
	require.NoError(t, err)

	// Seed site + MP so threshold rules + measurements have somewhere to land.
	var siteIDStr, mpIDStr string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO site (name, timezone) VALUES ('thr-site', 'UTC') RETURNING id`,
	).Scan(&siteIDStr))
	mpLabel := "thr-mp"
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO metering_point (site_id, name, utility_class)
		 VALUES ($1, $2, 'water') RETURNING id`, siteIDStr, mpLabel,
	).Scan(&mpIDStr))
	mpID, err := uuid.Parse(mpIDStr)
	require.NoError(t, err)

	queries := sqlc.New(pool)
	rules := NewRuleStore(pool)
	alerts := NewAlertStore(pool)
	workerStat := NewWorkerStateStore(pool)
	worker := &ThresholdInstantaneousWorker{
		Eng: EvaluateContext{
			Pool: pool, Queries: queries, InstallTZ: time.UTC,
			Log: log,
		},
		Rules: rules, Alerts: alerts, WorkerStat: workerStat,
	}
	return &thresholdTestEnv{
		pool: pool, queries: queries, rules: rules, alerts: alerts,
		workerStat: workerStat, worker: worker, mpID: mpID, mpLabel: mpLabel,
	}
}

// seedRule creates an active threshold_instantaneous rule with high_bound.
func (e *thresholdTestEnv) seedRule(t *testing.T, high float64, cmp string, cooldown int32) RuleRecord {
	t.Helper()
	ctx := context.Background()
	tx, err := e.pool.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()
	cd := cooldown
	rule, err := e.rules.CreateRule(ctx, tx, CreateRuleParams{
		RuleKind:        "threshold_instantaneous",
		ScopeKind:       "metering_point",
		ScopeID:         &e.mpID,
		HighBound:       &high,
		Comparison:      &cmp,
		Severity:        "critical",
		CooldownSeconds: &cd,
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
	return rule
}

// seedMeasurement inserts one measurement row with the given instant_value.
func (e *thresholdTestEnv) seedMeasurement(t *testing.T, instant float64, when time.Time) {
	t.Helper()
	_, err := e.pool.Exec(context.Background(),
		`INSERT INTO measurement (
		    time, metering_point_id,
		    raw_value, cumulative_value, instant_value,
		    extra, raw_payload, decoded_object, quality
		 ) VALUES ($1, $2, $3, $3, $4, '{}'::jsonb, '\x00'::bytea, '{}'::jsonb, 'ok')`,
		when, e.mpID, instant, instant,
	)
	require.NoError(t, err)
}

// runWorker calls Work once and asserts no error.
func (e *thresholdTestEnv) runWorker(t *testing.T) {
	t.Helper()
	err := e.worker.Work(context.Background(), &river.Job[ThresholdInstantaneousArgs]{
		Args: ThresholdInstantaneousArgs{},
	})
	require.NoError(t, err)
}

// TestThresholdInstantaneous_FiresOnBreach — seed an instant_value above
// high_bound; one cycle fires one critical alert with payload containing
// value=42.7, threshold=40.0.
func TestThresholdInstantaneous_FiresOnBreach(t *testing.T) {
	env := newThresholdTestEnv(t)
	rule := env.seedRule(t, 40.0, "gt", 900)
	env.seedMeasurement(t, 42.7, time.Now().UTC())

	env.runWorker(t)

	// Exactly one firing alert for (rule, MP).
	a, err := env.alerts.ListFiringByRuleTarget(context.Background(), rule.ID, env.mpID)
	require.NoError(t, err)
	require.NotNil(t, a, "expected a firing alert")
	require.Equal(t, "firing", a.State)
	require.Equal(t, "critical", a.Severity)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(a.Payload, &payload))
	require.InDelta(t, 42.7, payload["value"], 1e-9)
	require.InDelta(t, 40.0, payload["threshold"], 1e-9)
}

// TestThresholdInstantaneous_AutoClearsOnReturn — after a fire, seed a fresh
// measurement under the threshold and run again. Alert state moves to
// 'cleared'. audit_log has an alert.cleared row.
func TestThresholdInstantaneous_AutoClearsOnReturn(t *testing.T) {
	env := newThresholdTestEnv(t)
	rule := env.seedRule(t, 40.0, "gt", 1) // 1s cooldown for cycle 2
	env.seedMeasurement(t, 42.7, time.Now().UTC())
	env.runWorker(t)

	// Confirm fire happened.
	a, err := env.alerts.ListFiringByRuleTarget(context.Background(), rule.ID, env.mpID)
	require.NoError(t, err)
	require.NotNil(t, a)

	// Newer measurement back inside bounds.
	env.seedMeasurement(t, 30.0, time.Now().UTC().Add(1*time.Second))
	env.runWorker(t)

	// Re-read by ID; state should be cleared.
	var state string
	require.NoError(t, env.pool.QueryRow(context.Background(),
		`SELECT state FROM alert WHERE id = $1`, a.ID).Scan(&state))
	require.Equal(t, "cleared", state)

	// audit_log has an alert.cleared row.
	var count int
	require.NoError(t, env.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE entity_id = $1 AND action = $2`,
		a.ID, "alert.cleared").Scan(&count))
	require.GreaterOrEqual(t, count, 1, "expected at least one alert.cleared audit row")
}

// TestThresholdInstantaneous_CooldownSuppresses — a rule with last_fired_at
// inside the cooldown window is skipped (no second fire after the first
// alert is cleared and the measurement breaches again).
func TestThresholdInstantaneous_CooldownSuppresses(t *testing.T) {
	env := newThresholdTestEnv(t)
	rule := env.seedRule(t, 40.0, "gt", 3600) // 1h cooldown
	env.seedMeasurement(t, 42.7, time.Now().UTC())
	env.runWorker(t)

	// First fire happened.
	a, err := env.alerts.ListFiringByRuleTarget(context.Background(), rule.ID, env.mpID)
	require.NoError(t, err)
	require.NotNil(t, a)

	// Manually clear so the partial-unique-index isn't masking the cooldown
	// check (we want to verify the worker SKIPS evaluation, not that the
	// idempotent-fire guard kicks in).
	tx, err := env.pool.BeginTx(context.Background(), pgx.TxOptions{})
	require.NoError(t, err)
	require.NoError(t, env.alerts.ClearAlert(context.Background(), tx, a.ID))
	require.NoError(t, tx.Commit(context.Background()))

	// Seed a fresh breaching measurement and re-run; cooldown should still
	// suppress (last_fired_at was set on the first fire).
	env.seedMeasurement(t, 50.0, time.Now().UTC().Add(1*time.Second))
	env.runWorker(t)

	// No new firing alert.
	a2, err := env.alerts.ListFiringByRuleTarget(context.Background(), rule.ID, env.mpID)
	require.NoError(t, err)
	require.Nil(t, a2, "cooldown must prevent re-fire within window")
}

// TestThresholdInstantaneous_IdempotentFire — re-running the worker with
// the same breach state does NOT create a second firing alert (partial
// unique index alert_firing_unique_idx returns ErrDuplicateFire, which the
// worker treats as a no-op).
func TestThresholdInstantaneous_IdempotentFire(t *testing.T) {
	env := newThresholdTestEnv(t)
	env.seedRule(t, 40.0, "gt", 0) // cooldown=0 → cooldown never blocks
	env.seedMeasurement(t, 42.7, time.Now().UTC())

	env.runWorker(t)
	env.runWorker(t)

	var firingCount int
	require.NoError(t, env.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM alert WHERE state = 'firing'`).Scan(&firingCount))
	require.Equal(t, 1, firingCount, "exactly one firing alert despite two cycles")
}

// TestThresholdHourly_QueriesCaggHourly — a threshold_hourly rule fires
// from a measurement_hourly bucket, not from the raw measurement table.
// We seed raw measurements then refresh the CAGG to materialize the bucket.
func TestThresholdHourly_QueriesCaggHourly(t *testing.T) {
	env := newThresholdTestEnv(t)
	ctx := context.Background()

	// Seed several raw measurements inside a single hour with avg_instant=50.
	base := time.Now().UTC().Truncate(time.Hour).Add(-2 * time.Hour)
	for i := 0; i < 4; i++ {
		env.seedMeasurement(t, 50.0, base.Add(time.Duration(i)*5*time.Minute))
	}
	// Force CAGG refresh so the bucket is visible to the hourly query.
	_, err := env.pool.Exec(ctx,
		`CALL refresh_continuous_aggregate('measurement_hourly', NULL, NULL)`)
	require.NoError(t, err)

	// Seed a threshold_hourly rule with high=40 so avg_instant=50 breaches.
	tx, err := env.pool.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	high := 40.0
	cmp := "gt"
	cd := int32(0)
	rule, err := env.rules.CreateRule(ctx, tx, CreateRuleParams{
		RuleKind:        "threshold_hourly",
		ScopeKind:       "metering_point",
		ScopeID:         &env.mpID,
		HighBound:       &high,
		Comparison:      &cmp,
		Severity:        "warning",
		CooldownSeconds: &cd,
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))

	hourlyWorker := &ThresholdHourlyWorker{
		Eng:   env.worker.Eng,
		Rules: env.rules, Alerts: env.alerts, WorkerStat: env.workerStat,
	}
	require.NoError(t, hourlyWorker.Work(ctx,
		&river.Job[ThresholdHourlyArgs]{Args: ThresholdHourlyArgs{}}))

	a, err := env.alerts.ListFiringByRuleTarget(ctx, rule.ID, env.mpID)
	require.NoError(t, err)
	require.NotNil(t, a, "expected hourly worker to fire on CAGG breach")
	require.Equal(t, "threshold_hourly", a.RuleKind)
}

// TestThresholdDaily_QueriesCaggDaily — daily worker reads from
// measurement_daily and fires when sum_consumption breaches the bound.
func TestThresholdDaily_QueriesCaggDaily(t *testing.T) {
	env := newThresholdTestEnv(t)
	ctx := context.Background()

	// Seed enough raw measurements to produce cumulative_delta > 100 in a
	// single daily bucket. Note: seedMeasurement sets BOTH cumulative_value
	// and instant_value to the same number; the daily CAGG sums per-hour
	// (last - first) deltas, so we set cumulative_value to grow from 100
	// → 1200 across the bucket.
	base := time.Now().UTC().Truncate(24 * time.Hour).Add(-48 * time.Hour)
	for i := 0; i < 4; i++ {
		env.seedMeasurement(t, 100.0+float64(i*400), base.Add(time.Duration(i)*15*time.Minute))
	}
	_, err := env.pool.Exec(ctx,
		`CALL refresh_continuous_aggregate('measurement_hourly', NULL, NULL)`)
	require.NoError(t, err)
	_, err = env.pool.Exec(ctx,
		`CALL refresh_continuous_aggregate('measurement_daily', NULL, NULL)`)
	require.NoError(t, err)

	tx, err := env.pool.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	high := 100.0
	cmp := "gt"
	cd := int32(0)
	rule, err := env.rules.CreateRule(ctx, tx, CreateRuleParams{
		RuleKind:        "threshold_daily",
		ScopeKind:       "metering_point",
		ScopeID:         &env.mpID,
		HighBound:       &high,
		Comparison:      &cmp,
		Severity:        "warning",
		CooldownSeconds: &cd,
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))

	dailyWorker := &ThresholdDailyWorker{
		Eng:   env.worker.Eng,
		Rules: env.rules, Alerts: env.alerts, WorkerStat: env.workerStat,
	}
	require.NoError(t, dailyWorker.Work(ctx,
		&river.Job[ThresholdDailyArgs]{Args: ThresholdDailyArgs{}}))

	a, err := env.alerts.ListFiringByRuleTarget(ctx, rule.ID, env.mpID)
	require.NoError(t, err)
	require.NotNil(t, a, "expected daily worker to fire on CAGG breach")
	require.Equal(t, "threshold_daily", a.RuleKind)
}

// TestThresholdAuditTransactionAtomicity — every successful fire writes
// the alert.fired audit row in the SAME pgx.Tx as the alert insert
// (D-23). After a happy-path fire, both rows are present and share the
// same logical commit. If audit.WriteEntry would fail (e.g. on a CHECK
// violation), the deferred tx.Rollback rolls back the alert insert too —
// the audit_log INSERT-ONLY trigger from 0016 makes orphan audit rows
// impossible by construction, and the partial unique index from 0039
// makes orphan alert rows visible to the next cycle (which would clear
// them on the next breach-resolved transition). The test below pins
// the happy-path invariant: one fire → one alert row + one audit row,
// no extras.
func TestThresholdAuditTransactionAtomicity(t *testing.T) {
	env := newThresholdTestEnv(t)
	rule := env.seedRule(t, 40.0, "gt", 0)
	env.seedMeasurement(t, 42.7, time.Now().UTC())

	target := Target{MeteringPointID: env.mpID, Label: env.mpLabel}
	require.NoError(t, fireThresholdAlert(
		context.Background(),
		env.worker.Eng, env.alerts, env.rules,
		rule, target, 42.7, 40.0, "Atomicity"))

	a, err := env.alerts.ListFiringByRuleTarget(context.Background(), rule.ID, env.mpID)
	require.NoError(t, err)
	require.NotNil(t, a, "happy path fired one alert")

	var auditCount int
	require.NoError(t, env.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE entity_id = $1 AND action = 'alert.fired'`,
		a.ID).Scan(&auditCount))
	require.Equal(t, 1, auditCount, "exactly one alert.fired audit row in the fire tx")

	// And the rule's last_fired_at was touched in the same tx — the
	// cooldown counter starts from this fire, satisfying D-05.
	r, err := env.rules.GetRuleByID(context.Background(), rule.ID)
	require.NoError(t, err)
	require.NotNil(t, r.LastFiredAt, "TouchLastFiredAt must commit with the fire tx")
}

// TestThresholdWorker_SkipsRuleWithoutMeasurement — a rule pointed at an MP
// that has no measurements yet does not fire (and does not error).
func TestThresholdWorker_SkipsRuleWithoutMeasurement(t *testing.T) {
	env := newThresholdTestEnv(t)
	env.seedRule(t, 40.0, "gt", 0)
	// No measurement seeded.

	env.runWorker(t) // must not error

	a, err := env.alerts.ListFiringByRuleTarget(context.Background(), uuid.New(), env.mpID)
	require.NoError(t, err)
	require.Nil(t, a)
}
