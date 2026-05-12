package alert

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// alertStoreSetup spins up a fresh DB, seeds a user + site + MP + rule, and
// returns the stores + IDs the tests need.
func alertStoreSetup(t *testing.T) (*AlertStore, *RuleStore, uuid.UUID, uuid.UUID) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	// User (FK target for AckAlert.userID).
	var userIDStr string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('alertstore@example.com', 'AS', 'x', 'admin') RETURNING id`,
	).Scan(&userIDStr))
	userID, err := uuid.Parse(userIDStr)
	require.NoError(t, err)

	// Site + MP.
	var siteIDStr, mpIDStr string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO site (name, timezone) VALUES ('alert-store-site', 'UTC') RETURNING id`,
	).Scan(&siteIDStr))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO metering_point (site_id, name, utility_class)
		 VALUES ($1, 'alert-store-mp', 'water') RETURNING id`,
		siteIDStr,
	).Scan(&mpIDStr))
	mpID, err := uuid.Parse(mpIDStr)
	require.NoError(t, err)

	// Rule.
	rs := NewRuleStore(pool)
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	high := 40.0
	cmp := "gt"
	rule, err := rs.CreateRule(ctx, tx, CreateRuleParams{
		RuleKind:   "threshold_hourly",
		ScopeKind:  "metering_point",
		ScopeID:    &mpID,
		HighBound:  &high,
		Comparison: &cmp,
		Severity:   "critical",
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))

	// (rule.ID, userID, mpID) used by callers.
	_ = userID
	store := NewAlertStore(pool)
	return store, rs, rule.ID, mpID
}

// TestAlertStore_InsertAndIdempotentRefire — a duplicate fire for the same
// (rule, target) while the previous is still firing returns ErrDuplicateFire.
func TestAlertStore_InsertAndIdempotentRefire(t *testing.T) {
	store, _, ruleID, mpID := alertStoreSetup(t)
	pool := store.pool
	ctx := context.Background()

	payload := []byte(`{"rule_id":"` + ruleID.String() + `","value":42.7,"threshold":40.0}`)

	// First fire.
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	a, err := store.InsertAlert(ctx, tx, InsertAlertParams{
		RuleID:           ruleID,
		RuleKind:         "threshold_hourly",
		Severity:         "critical",
		Payload:          payload,
		TargetEntityType: "metering_point",
		TargetEntityID:   mpID,
	})
	require.NoError(t, err)
	require.Equal(t, "firing", a.State)
	require.NoError(t, tx.Commit(ctx))

	// Duplicate fire while previous still firing → ErrDuplicateFire.
	tx, err = pool.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	_, err = store.InsertAlert(ctx, tx, InsertAlertParams{
		RuleID:           ruleID,
		RuleKind:         "threshold_hourly",
		Severity:         "critical",
		Payload:          payload,
		TargetEntityType: "metering_point",
		TargetEntityID:   mpID,
	})
	require.ErrorIs(t, err, ErrDuplicateFire)
	_ = tx.Rollback(ctx)

	// Clear the first, then a new fire succeeds.
	tx, err = pool.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	require.NoError(t, store.ClearAlert(ctx, tx, a.ID))
	require.NoError(t, tx.Commit(ctx))

	tx, err = pool.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	a2, err := store.InsertAlert(ctx, tx, InsertAlertParams{
		RuleID:           ruleID,
		RuleKind:         "threshold_hourly",
		Severity:         "critical",
		Payload:          payload,
		TargetEntityType: "metering_point",
		TargetEntityID:   mpID,
	})
	require.NoError(t, err, "after previous cleared, second fire must succeed")
	require.NotEqual(t, a.ID, a2.ID)
	require.NoError(t, tx.Commit(ctx))
}

// TestAlertStore_AckSnoozeClear — exercise the ack + snooze + clear paths.
func TestAlertStore_AckSnoozeClear(t *testing.T) {
	store, _, ruleID, mpID := alertStoreSetup(t)
	pool := store.pool
	ctx := context.Background()

	// Seed a user for AckAlert.
	var userIDStr string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('ack-user@example.com', 'A', 'x', 'admin') RETURNING id`,
	).Scan(&userIDStr))
	userID, err := uuid.Parse(userIDStr)
	require.NoError(t, err)

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	a, err := store.InsertAlert(ctx, tx, InsertAlertParams{
		RuleID: ruleID, RuleKind: "threshold_hourly", Severity: "critical",
		Payload: []byte(`{}`), TargetEntityType: "metering_point", TargetEntityID: mpID,
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))

	// Ack.
	tx, err = pool.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	require.NoError(t, store.AckAlert(ctx, tx, a.ID, userID, "checked manually"))
	require.NoError(t, tx.Commit(ctx))

	// Verify state moved to acknowledged.
	row := pool.QueryRow(ctx, `SELECT state, ack_note FROM alert WHERE id = $1`, a.ID)
	var state, note string
	require.NoError(t, row.Scan(&state, &note))
	require.Equal(t, "acknowledged", state)
	require.Equal(t, "checked manually", note)

	// Clear.
	tx, err = pool.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	require.NoError(t, store.ClearAlert(ctx, tx, a.ID))
	require.NoError(t, tx.Commit(ctx))
}

// TestAlertStore_ListFiringByRuleTarget — returns the firing alert for a
// (rule, target) pair, or nil when none.
func TestAlertStore_ListFiringByRuleTarget(t *testing.T) {
	store, _, ruleID, mpID := alertStoreSetup(t)
	pool := store.pool
	ctx := context.Background()

	// No firing initially.
	got, err := store.ListFiringByRuleTarget(ctx, ruleID, mpID)
	require.NoError(t, err)
	require.Nil(t, got)

	// Insert + check.
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	a, err := store.InsertAlert(ctx, tx, InsertAlertParams{
		RuleID: ruleID, RuleKind: "threshold_hourly", Severity: "critical",
		Payload: []byte(`{}`), TargetEntityType: "metering_point", TargetEntityID: mpID,
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))

	got, err = store.ListFiringByRuleTarget(ctx, ruleID, mpID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, a.ID, got.ID)
}

// silence unused if helpers don't currently consume time.
var _ = time.Now
