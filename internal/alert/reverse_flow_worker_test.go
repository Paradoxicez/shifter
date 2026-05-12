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
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/db"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// reverseFlowTestEnv is the shared fixture for reverse_flow_increase worker tests.
type reverseFlowTestEnv struct {
	pool    *pgxpool.Pool
	queries *sqlc.Queries
	rules   *RuleStore
	alerts  *AlertStore
	worker  *ReverseFlowIncreaseWorker
	siteID  uuid.UUID
	mpID    uuid.UUID
}

func newReverseFlowTestEnv(t *testing.T) *reverseFlowTestEnv {
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
		 VALUES (1, 'RF Test Install', 'UTC', 'metric')
		 ON CONFLICT (id) DO NOTHING`)
	require.NoError(t, err)

	var siteIDStr, mpIDStr string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO site (name, timezone) VALUES ('rf-site', 'UTC') RETURNING id`,
	).Scan(&siteIDStr))
	siteID, err := uuid.Parse(siteIDStr)
	require.NoError(t, err)

	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO metering_point (site_id, name, utility_class) VALUES ($1, 'rf-mp', 'water') RETURNING id`,
		siteID,
	).Scan(&mpIDStr))
	mpID, err := uuid.Parse(mpIDStr)
	require.NoError(t, err)

	queries := sqlc.New(pool)
	rules := NewRuleStore(pool)
	alerts := NewAlertStore(pool)
	worker := &ReverseFlowIncreaseWorker{
		Eng:    EvaluateContext{Pool: pool, Queries: queries, Log: log},
		Rules:  rules,
		Alerts: alerts,
	}

	return &reverseFlowTestEnv{
		pool: pool, queries: queries, rules: rules, alerts: alerts,
		worker: worker, siteID: siteID, mpID: mpID,
	}
}

// seedRFRule creates a reverse_flow_increase rule scoped to env.mpID.
// windowDays is stored in DaysOfWeek (repurposed as window size for this kind).
// thresholdM3 is stored in FlowThreshold.
func (e *reverseFlowTestEnv) seedRFRule(t *testing.T, windowDays int32, thresholdM3 float64) RuleRecord {
	t.Helper()
	ctx := context.Background()
	tx, err := e.pool.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()
	cd := int32(0)
	rule, err := e.rules.CreateRule(ctx, tx, CreateRuleParams{
		RuleKind:        "reverse_flow_increase",
		ScopeKind:       "metering_point",
		ScopeID:         &e.mpID,
		Severity:        "warning",
		CooldownSeconds: &cd,
		DaysOfWeek:      &windowDays,
		FlowThreshold:   &thresholdM3,
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
	return rule
}

// seedRawMeasurement inserts a measurement row with extra JSONB containing
// reverse_flow_m3 at the given time. The measurement.extra column is the
// vendor-extension JSONB field where Itron+KINMY stores reverse_flow_m3.
func (e *reverseFlowTestEnv) seedRawMeasurement(t *testing.T, reverseFlowM3 float64, when time.Time) {
	t.Helper()
	extraJSON, err := json.Marshal(map[string]any{"reverse_flow_m3": reverseFlowM3})
	require.NoError(t, err)
	_, err = e.pool.Exec(context.Background(),
		`INSERT INTO measurement (time, metering_point_id, raw_value, cumulative_value,
		    instant_value, extra, raw_payload, decoded_object, quality)
		 VALUES ($1, $2, $3, $3, $3, $4::jsonb, '\x00'::bytea, '{}'::jsonb, 'ok')`,
		when, e.mpID, reverseFlowM3, string(extraJSON),
	)
	require.NoError(t, err)
}

// ─────────────────────────────────────────────────────────────────────────────
// Tests
// ─────────────────────────────────────────────────────────────────────────────

// TestReverseFlowIncrease_Fires: delta (2.5 - 0) > threshold 1.0 → alert fires.
func TestReverseFlowIncrease_Fires(t *testing.T) {
	env := newReverseFlowTestEnv(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Seed: past value = 0.0 at 8d ago; current value = 2.5 now.
	env.seedRawMeasurement(t, 0.0, now.Add(-8*24*time.Hour))
	env.seedRawMeasurement(t, 2.5, now)

	rule := env.seedRFRule(t, 7, 1.0) // window=7d, threshold=1.0 m³

	require.NoError(t, env.worker.Run(ctx))

	a, err := env.alerts.ListFiringByRuleTarget(ctx, rule.ID, env.mpID)
	require.NoError(t, err)
	require.NotNil(t, a, "delta 2.5 > threshold 1.0 must fire reverse_flow_increase alert")
	require.Equal(t, "reverse_flow_increase", a.RuleKind)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(a.Payload, &payload))
	require.InDelta(t, 2.5, payload["value"], 1e-6)
	require.InDelta(t, 1.0, payload["threshold"], 1e-6)
}

// TestReverseFlowIncrease_DoesNotFire: delta (0.5 - 0) < threshold 1.0 → no fire.
func TestReverseFlowIncrease_DoesNotFire(t *testing.T) {
	env := newReverseFlowTestEnv(t)
	ctx := context.Background()
	now := time.Now().UTC()

	env.seedRawMeasurement(t, 0.0, now.Add(-8*24*time.Hour))
	env.seedRawMeasurement(t, 0.5, now)

	rule := env.seedRFRule(t, 7, 1.0)

	require.NoError(t, env.worker.Run(ctx))

	a, err := env.alerts.ListFiringByRuleTarget(ctx, rule.ID, env.mpID)
	require.NoError(t, err)
	require.Nil(t, a, "delta 0.5 < threshold 1.0 must NOT fire")
}

// TestReverseFlowIncrease_MissingRaw: MP has no raw.reverse_flow_m3 → both
// COALESCE to 0, delta=0 < any positive threshold, no fire, no crash.
func TestReverseFlowIncrease_MissingRaw(t *testing.T) {
	env := newReverseFlowTestEnv(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Seed plain measurements with no reverse_flow_m3 in raw.
	_, err := env.pool.Exec(ctx,
		`INSERT INTO measurement (time, metering_point_id, raw_value, cumulative_value,
		    instant_value, extra, raw_payload, decoded_object, quality)
		 VALUES ($1, $2, 1.0, 1.0, 1.0, '{}'::jsonb, '\x00'::bytea, '{}'::jsonb, 'ok')`,
		now, env.mpID,
	)
	require.NoError(t, err)

	rule := env.seedRFRule(t, 7, 1.0)

	require.NoError(t, env.worker.Run(ctx))

	a, err := env.alerts.ListFiringByRuleTarget(ctx, rule.ID, env.mpID)
	require.NoError(t, err)
	require.Nil(t, a, "missing reverse_flow_m3 in raw → delta=0 must NOT fire")
}
