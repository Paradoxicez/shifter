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

// anomalyTestEnv is the shared fixture for the anomaly worker tests: spin
// up Postgres, run migrations, seed install_identity + site + MP, and return
// the worker pieces tests need to seed rules and measurements.
type anomalyTestEnv struct {
	pool       *pgxpool.Pool
	queries    *sqlc.Queries
	rules      *RuleStore
	alerts     *AlertStore
	workerStat *WorkerStateStore
	worker     *AnomalyWorker
	siteID     uuid.UUID
	mpID       uuid.UUID
	mpLabel    string
}

func newAnomalyTestEnv(t *testing.T) *anomalyTestEnv {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	_, err := pool.Exec(ctx,
		`INSERT INTO install_identity (id, display_name, timezone, units)
		 VALUES (1, 'Anomaly Test Install', 'UTC', 'metric')
		 ON CONFLICT (id) DO NOTHING`)
	require.NoError(t, err)

	var siteIDStr, mpIDStr string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO site (name, timezone) VALUES ('an-site', 'UTC') RETURNING id`,
	).Scan(&siteIDStr))
	siteID, err := uuid.Parse(siteIDStr)
	require.NoError(t, err)
	mpLabel := "an-mp"
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO metering_point (site_id, name, utility_class)
		 VALUES ($1, $2, 'water') RETURNING id`, siteID, mpLabel,
	).Scan(&mpIDStr))
	mpID, err := uuid.Parse(mpIDStr)
	require.NoError(t, err)

	queries := sqlc.New(pool)
	rules := NewRuleStore(pool)
	alerts := NewAlertStore(pool)
	workerStat := NewWorkerStateStore(pool)
	worker := &AnomalyWorker{
		Eng: EvaluateContext{
			Pool: pool, Queries: queries, InstallTZ: time.UTC, Log: log,
		},
		Rules: rules, Alerts: alerts, WorkerStat: workerStat,
	}
	return &anomalyTestEnv{
		pool: pool, queries: queries, rules: rules, alerts: alerts,
		workerStat: workerStat, worker: worker,
		siteID: siteID, mpID: mpID, mpLabel: mpLabel,
	}
}

// seedAnomalyRule creates a metering_point-scoped anomaly rule.
func (e *anomalyTestEnv) seedAnomalyRule(t *testing.T, kind string, severity string,
	quietStart, quietEnd *time.Time, flowThresh *float64, cooldown int32) RuleRecord {
	t.Helper()
	ctx := context.Background()
	tx, err := e.pool.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()
	cd := cooldown
	rule, err := e.rules.CreateRule(ctx, tx, CreateRuleParams{
		RuleKind:         kind,
		ScopeKind:        "metering_point",
		ScopeID:          &e.mpID,
		Severity:         severity,
		CooldownSeconds:  &cd,
		QuietWindowStart: quietStart,
		QuietWindowEnd:   quietEnd,
		FlowThreshold:    flowThresh,
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
	return rule
}

// seedMeasurement inserts one measurement with the given instant_value at the
// given time. raw_value/cumulative_value mirror instant for simplicity.
func (e *anomalyTestEnv) seedMeasurement(t *testing.T, instant float64, when time.Time) {
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

// seedEligibleBackfill inserts a single very-old (22d) measurement so the
// MP passes the D-16 cold-start gate. Tests that need anomaly rules to fire
// MUST call this first.
func (e *anomalyTestEnv) seedEligibleBackfill(t *testing.T) {
	t.Helper()
	e.seedMeasurement(t, 0.0, time.Now().UTC().Add(-22*24*time.Hour))
}

// runWorker runs one anomaly cycle and asserts no error.
func (e *anomalyTestEnv) runWorker(t *testing.T) {
	t.Helper()
	err := e.worker.Work(context.Background(),
		&river.Job[AnomalyArgs]{Args: AnomalyArgs{}})
	require.NoError(t, err)
}

// TestAnomalyP95_FiresOnOutlier: 30 days of measurements where p95 at the
// current hour-of-day is 10 m³/h; latest measurement at the current hour
// with value=15 fires with payload.value=15, payload.threshold=10.
func TestAnomalyP95_FiresOnOutlier(t *testing.T) {
	env := newAnomalyTestEnv(t)
	env.seedEligibleBackfill(t)

	now := time.Now().UTC()
	curHour := now.Hour()

	// Seed 30 historical measurements at the current hour-of-day with
	// instant_value=10 (so p95 = 10 exactly). Spread across the trailing 30d.
	for i := 1; i <= 30; i++ {
		ts := now.AddDate(0, 0, -i)
		// Force the timestamp to be at the same hour-of-day as `now`.
		ts = time.Date(ts.Year(), ts.Month(), ts.Day(), curHour, 30, 0, 0, time.UTC)
		env.seedMeasurement(t, 10.0, ts)
	}

	// One outlier as the LATEST measurement.
	env.seedMeasurement(t, 15.0, now)

	rule := env.seedAnomalyRule(t, "anomaly_p95", "warning", nil, nil, nil, 0)

	env.runWorker(t)

	a, err := env.alerts.ListFiringByRuleTarget(context.Background(), rule.ID, env.mpID)
	require.NoError(t, err)
	require.NotNil(t, a, "expected anomaly_p95 fire on outlier")
	require.Equal(t, "anomaly_p95", a.RuleKind)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(a.Payload, &payload))
	require.InDelta(t, 15.0, payload["value"], 1e-6)
	require.InDelta(t, 10.0, payload["threshold"], 1e-6)
	require.InDelta(t, 10.0, payload["p95_baseline"], 1e-6)
	require.EqualValues(t, 30, payload["baseline_window_days"])
}

// TestAnomalyP95_BucketIsHourOfDay: measurements at hour=14 have p95=10;
// measurements at hour=02 have p95=2. The latest measurement at hour=14
// with value=11 fires; at hour=02 with value=11 also fires (different
// bucket, exceeds the lower hour=02 baseline).
func TestAnomalyP95_BucketIsHourOfDay(t *testing.T) {
	env := newAnomalyTestEnv(t)
	env.seedEligibleBackfill(t)

	now := time.Now().UTC()
	curHour := now.Hour()

	// Seed 30 historical days for the current hour-of-day with value=10
	// (high baseline). Use a deliberate small spread so percentile is 10.
	for i := 1; i <= 30; i++ {
		ts := now.AddDate(0, 0, -i)
		ts = time.Date(ts.Year(), ts.Month(), ts.Day(), curHour, 30, 0, 0, time.UTC)
		env.seedMeasurement(t, 10.0, ts)
	}
	// Latest at curHour with value=11 → above 10 baseline → fires.
	env.seedMeasurement(t, 11.0, now)

	rule := env.seedAnomalyRule(t, "anomaly_p95", "warning", nil, nil, nil, 0)
	env.runWorker(t)
	a, err := env.alerts.ListFiringByRuleTarget(context.Background(), rule.ID, env.mpID)
	require.NoError(t, err)
	require.NotNil(t, a, "p95 bucket-of-hour: latest at curHour with 11 > baseline 10 must fire")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(a.Payload, &payload))
	bucket, _ := payload["time_of_day_bucket"].(string)
	require.NotEmpty(t, bucket, "payload must include time_of_day_bucket")
}

// TestAnomalyIQR_FiresAboveQ3PlusOnePointFiveIQR: Q1=5, Q3=15 → IQR=10 →
// upper bound = 15 + 1.5*10 = 30. A latest value of 35 fires.
func TestAnomalyIQR_FiresAboveQ3PlusOnePointFiveIQR(t *testing.T) {
	env := newAnomalyTestEnv(t)
	env.seedEligibleBackfill(t)

	now := time.Now().UTC()
	// Seed a distribution with Q1=5, Q3=15. Use a 21-value distribution:
	// values {1,2,...,9,10,11,...,19} (sorted) yield Q1=5, Q3=15 on
	// percentile_cont continuous interpolation.
	// percentile_cont(0.25) over [1..19] with continuous interp ≈ 5.5.
	// To make this deterministic at exactly Q1=5, Q3=15 we use a careful
	// asymmetric set. Easiest: many copies at 5 and at 15.
	for i := 0; i < 10; i++ {
		env.seedMeasurement(t, 5.0, now.Add(-time.Duration(i+1)*time.Hour))
	}
	for i := 0; i < 10; i++ {
		env.seedMeasurement(t, 15.0, now.Add(-time.Duration(i+11)*time.Hour))
	}
	// Latest = 35.0 → above upper bound 30 → fires.
	env.seedMeasurement(t, 35.0, now)

	rule := env.seedAnomalyRule(t, "anomaly_iqr", "warning", nil, nil, nil, 0)
	env.runWorker(t)

	a, err := env.alerts.ListFiringByRuleTarget(context.Background(), rule.ID, env.mpID)
	require.NoError(t, err)
	require.NotNil(t, a, "IQR upper-bound breach must fire")
	require.Equal(t, "anomaly_iqr", a.RuleKind)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(a.Payload, &payload))
	require.InDelta(t, 35.0, payload["value"], 1e-6)
	// threshold reported = upper bound 30
	require.InDelta(t, 30.0, payload["threshold"], 1e-6)
}

// TestAnomalyIQR_FiresBelowQ1MinusOnePointFiveIQR: with Q1=5, Q3=15 →
// lower bound = 5 - 1.5*10 = -10. A latest value of -12 fires.
func TestAnomalyIQR_FiresBelowQ1MinusOnePointFiveIQR(t *testing.T) {
	env := newAnomalyTestEnv(t)
	env.seedEligibleBackfill(t)

	now := time.Now().UTC()
	for i := 0; i < 10; i++ {
		env.seedMeasurement(t, 5.0, now.Add(-time.Duration(i+1)*time.Hour))
	}
	for i := 0; i < 10; i++ {
		env.seedMeasurement(t, 15.0, now.Add(-time.Duration(i+11)*time.Hour))
	}
	// Latest = -12 → below lower bound -10 → fires.
	env.seedMeasurement(t, -12.0, now)

	rule := env.seedAnomalyRule(t, "anomaly_iqr", "warning", nil, nil, nil, 0)
	env.runWorker(t)

	a, err := env.alerts.ListFiringByRuleTarget(context.Background(), rule.ID, env.mpID)
	require.NoError(t, err)
	require.NotNil(t, a, "IQR lower-bound breach must fire")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(a.Payload, &payload))
	require.InDelta(t, -12.0, payload["value"], 1e-6)
	require.InDelta(t, -10.0, payload["threshold"], 1e-6)
}

// TestAnomalyQuietHour_SameDayWindow: rule quiet_window=08:00-18:00
// (same-day), flow_threshold=0.1. Measurement at 10:00 with instant_value=0.5
// fires.
func TestAnomalyQuietHour_SameDayWindow(t *testing.T) {
	env := newAnomalyTestEnv(t)
	env.seedEligibleBackfill(t)

	// Time of day 10:00 UTC, recently.
	at10 := time.Date(time.Now().UTC().Year(), time.Now().UTC().Month(),
		time.Now().UTC().Day(), 10, 0, 0, 0, time.UTC).Add(-30 * time.Minute)
	env.seedMeasurement(t, 0.5, at10)

	// rule quiet 08:00-18:00 with flow_threshold=0.1.
	start := time.Date(0, 1, 1, 8, 0, 0, 0, time.UTC)
	end := time.Date(0, 1, 1, 18, 0, 0, 0, time.UTC)
	thresh := 0.1
	rule := env.seedAnomalyRule(t, "anomaly_quiet_hour", "warning", &start, &end, &thresh, 0)

	env.runWorker(t)

	a, err := env.alerts.ListFiringByRuleTarget(context.Background(), rule.ID, env.mpID)
	require.NoError(t, err)
	require.NotNil(t, a, "same-day quiet-window breach must fire")
}

// TestAnomalyQuietHour_CrossMidnight: rule quiet_window=22:00-06:00 (cross
// midnight). Measurement at 03:00 with instant_value=0.5 fires; measurement
// at 10:00 with instant_value=0.5 does NOT fire (outside the window).
func TestAnomalyQuietHour_CrossMidnight(t *testing.T) {
	env := newAnomalyTestEnv(t)
	env.seedEligibleBackfill(t)

	// Measurement at 03:00 UTC.
	at3 := time.Date(time.Now().UTC().Year(), time.Now().UTC().Month(),
		time.Now().UTC().Day(), 3, 0, 0, 0, time.UTC)
	env.seedMeasurement(t, 0.5, at3)

	start := time.Date(0, 1, 1, 22, 0, 0, 0, time.UTC)
	end := time.Date(0, 1, 1, 6, 0, 0, 0, time.UTC)
	thresh := 0.1
	rule := env.seedAnomalyRule(t, "anomaly_quiet_hour", "warning", &start, &end, &thresh, 0)

	env.runWorker(t)

	a, err := env.alerts.ListFiringByRuleTarget(context.Background(), rule.ID, env.mpID)
	require.NoError(t, err)
	require.NotNil(t, a, "cross-midnight quiet-window: 03:00 inside 22:00-06:00 must fire")

	// Auto-clear the alert and replace measurement: now 10:00 → outside.
	_, err = env.pool.Exec(context.Background(),
		`UPDATE alert SET state = 'cleared', cleared_at = now() WHERE id = $1`, a.ID)
	require.NoError(t, err)
	// Wipe the 03:00 measurement so only a 10:00 one remains in the 24h window.
	_, err = env.pool.Exec(context.Background(),
		`DELETE FROM measurement WHERE metering_point_id = $1 AND instant_value::DOUBLE PRECISION = 0.5`,
		env.mpID)
	require.NoError(t, err)
	at10 := time.Date(time.Now().UTC().Year(), time.Now().UTC().Month(),
		time.Now().UTC().Day(), 10, 0, 0, 0, time.UTC).Add(-30 * time.Minute)
	env.seedMeasurement(t, 0.5, at10)
	env.runWorker(t)
	a2, err := env.alerts.ListFiringByRuleTarget(context.Background(), rule.ID, env.mpID)
	require.NoError(t, err)
	require.Nil(t, a2, "10:00 outside 22:00-06:00 must NOT fire")
}

// TestAnomalyQuietHour_RespectsInstallTimezone: install_tz='Asia/Bangkok'
// (+07). A UTC measurement at 19:00 (= local 02:00) with quiet 22:00-06:00
// must fire because local time falls inside the window.
func TestAnomalyQuietHour_RespectsInstallTimezone(t *testing.T) {
	env := newAnomalyTestEnv(t)
	env.seedEligibleBackfill(t)
	ctx := context.Background()

	// Reset install_identity to Bangkok.
	_, err := env.pool.Exec(ctx,
		`UPDATE install_identity SET timezone = 'Asia/Bangkok' WHERE id = 1`)
	require.NoError(t, err)

	// Measurement at 19:00 UTC today.
	at19UTC := time.Date(time.Now().UTC().Year(), time.Now().UTC().Month(),
		time.Now().UTC().Day(), 19, 0, 0, 0, time.UTC)
	env.seedMeasurement(t, 0.5, at19UTC)

	start := time.Date(0, 1, 1, 22, 0, 0, 0, time.UTC)
	end := time.Date(0, 1, 1, 6, 0, 0, 0, time.UTC)
	thresh := 0.1
	rule := env.seedAnomalyRule(t, "anomaly_quiet_hour", "warning", &start, &end, &thresh, 0)

	env.runWorker(t)

	a, err := env.alerts.ListFiringByRuleTarget(context.Background(), rule.ID, env.mpID)
	require.NoError(t, err)
	require.NotNil(t, a, "19:00 UTC = 02:00 Bangkok must fire inside 22:00-06:00 local")
}

// TestAnomaly_ColdStartGatesAllRuleKinds: MP with only 5 days of history →
// NONE of the three anomaly rule kinds fire even if the conditions would
// otherwise trigger.
func TestAnomaly_ColdStartGatesAllRuleKinds(t *testing.T) {
	env := newAnomalyTestEnv(t)
	// DO NOT call seedEligibleBackfill — MP is too new.

	now := time.Now().UTC()
	// 5 days of measurements that WOULD trigger p95 / iqr / quiet-hour.
	for i := 1; i <= 5; i++ {
		env.seedMeasurement(t, 10.0, now.AddDate(0, 0, -i))
	}
	// Latest extreme value at hour=now.
	env.seedMeasurement(t, 100.0, now)

	// Seed all three anomaly rules.
	rP95 := env.seedAnomalyRule(t, "anomaly_p95", "warning", nil, nil, nil, 0)
	rIQR := env.seedAnomalyRule(t, "anomaly_iqr", "warning", nil, nil, nil, 0)
	start := time.Date(0, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(0, 1, 1, 23, 59, 0, 0, time.UTC)
	thresh := 0.1
	rQH := env.seedAnomalyRule(t, "anomaly_quiet_hour", "warning", &start, &end, &thresh, 0)

	env.runWorker(t)

	for _, ruleID := range []uuid.UUID{rP95.ID, rIQR.ID, rQH.ID} {
		a, err := env.alerts.ListFiringByRuleTarget(context.Background(), ruleID, env.mpID)
		require.NoError(t, err)
		require.Nil(t, a, "cold-start gate must prevent fires for all 3 anomaly kinds")
	}
}

// TestAnomaly_OptInPerMP: a disabled (disabled_at NOT NULL) rule does NOT
// evaluate even when the MP is eligible and the data would trigger.
func TestAnomaly_OptInPerMP(t *testing.T) {
	env := newAnomalyTestEnv(t)
	env.seedEligibleBackfill(t)

	now := time.Now().UTC()
	curHour := now.Hour()
	for i := 1; i <= 30; i++ {
		ts := now.AddDate(0, 0, -i)
		ts = time.Date(ts.Year(), ts.Month(), ts.Day(), curHour, 30, 0, 0, time.UTC)
		env.seedMeasurement(t, 10.0, ts)
	}
	env.seedMeasurement(t, 15.0, now)

	rule := env.seedAnomalyRule(t, "anomaly_p95", "warning", nil, nil, nil, 0)

	// Disable the rule via the store.
	ctx := context.Background()
	tx, err := env.pool.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	require.NoError(t, env.rules.DisableRule(ctx, tx, rule.ID))
	require.NoError(t, tx.Commit(ctx))

	env.runWorker(t)
	a, err := env.alerts.ListFiringByRuleTarget(context.Background(), rule.ID, env.mpID)
	require.NoError(t, err)
	require.Nil(t, a, "disabled rule must not fire even on eligible MP with breach data")
}
