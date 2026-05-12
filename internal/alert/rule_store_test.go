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

func setupRuleStoreTest(t *testing.T) (*RuleStore, uuid.UUID) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	// Seed a metering point so scope_kind='metering_point' rules have a
	// scope target. (alert_rule has no FK on scope_id, but tests use real
	// UUIDs for realism.)
	var siteID, mpID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO site (name, timezone) VALUES ('rule-store-site', 'UTC') RETURNING id`,
	).Scan(&siteID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO metering_point (site_id, name, utility_class)
		 VALUES ($1, 'rule-store-mp', 'water') RETURNING id`,
		siteID,
	).Scan(&mpID))

	mp, err := uuid.Parse(mpID)
	require.NoError(t, err)

	return NewRuleStore(pool), mp
}

// TestRuleStore_CreateListEnableDisable — round-trip a rule through every
// operation in the store.
func TestRuleStore_CreateListEnableDisable(t *testing.T) {
	store, mpID := setupRuleStoreTest(t)
	pool := store.pool
	ctx := context.Background()

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()

	high := 40.0
	cmp := "gt"
	unit := "m3/h"
	name := "Test high flow"
	rule, err := store.CreateRule(ctx, tx, CreateRuleParams{
		RuleKind:   "threshold_hourly",
		ScopeKind:  "metering_point",
		ScopeID:    &mpID,
		HighBound:  &high,
		Comparison: &cmp,
		Unit:       &unit,
		Severity:   "critical",
		Name:       &name,
	})
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, rule.ID)
	require.Equal(t, "threshold_hourly", rule.RuleKind)
	require.Equal(t, "critical", rule.Severity)
	require.Equal(t, int32(900), rule.CooldownSeconds, "default cooldown via DB DEFAULT")
	require.Nil(t, rule.DisabledAt)
	require.NoError(t, tx.Commit(ctx))

	// List active by kind returns it.
	got, err := store.ListActiveRulesByKind(ctx, "threshold_hourly")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, rule.ID, got[0].ID)

	// Disable.
	tx, err = pool.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	require.NoError(t, store.DisableRule(ctx, tx, rule.ID))
	require.NoError(t, tx.Commit(ctx))

	// List active by kind excludes the disabled rule.
	got, err = store.ListActiveRulesByKind(ctx, "threshold_hourly")
	require.NoError(t, err)
	require.Empty(t, got, "disabled rule must not appear in active list")

	// ListAllRules with includeDisabled=true sees the disabled row.
	all, err := store.ListAllRules(ctx, true)
	require.NoError(t, err)
	require.Len(t, all, 1)
	require.NotNil(t, all[0].DisabledAt, "disabled_at must be set after Disable")

	// Re-enable.
	tx, err = pool.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	require.NoError(t, store.EnableRule(ctx, tx, rule.ID))
	require.NoError(t, tx.Commit(ctx))
	got, err = store.ListActiveRulesByKind(ctx, "threshold_hourly")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Nil(t, got[0].DisabledAt, "disabled_at must be NULL after Enable")
}

// TestRuleStore_TouchLastFiredAt_DrivesCooldown — set last_fired_at, then
// CooledDown returns false until cooldown_seconds elapses (simulated).
func TestRuleStore_TouchLastFiredAt_DrivesCooldown(t *testing.T) {
	store, mpID := setupRuleStoreTest(t)
	pool := store.pool
	ctx := context.Background()

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	cd := int32(10)
	rule, err := store.CreateRule(ctx, tx, CreateRuleParams{
		RuleKind:        "threshold_instantaneous",
		ScopeKind:       "metering_point",
		ScopeID:         &mpID,
		CooldownSeconds: &cd,
	})
	require.NoError(t, err)
	require.Equal(t, int32(10), rule.CooldownSeconds)
	require.NoError(t, tx.Commit(ctx))

	// Touch last_fired_at.
	tx, err = pool.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	require.NoError(t, store.TouchLastFiredAt(ctx, tx, rule.ID))
	require.NoError(t, tx.Commit(ctx))

	// Reread; CooledDown(now) is false (just fired).
	r, err := store.GetRuleByID(ctx, rule.ID)
	require.NoError(t, err)
	require.NotNil(t, r.LastFiredAt)
	require.False(t, r.CooledDown(time.Now()), "just-fired rule must not be cooled down")

	// CooledDown(future) returns true once cooldown_seconds elapses.
	require.True(t, r.CooledDown(time.Now().Add(time.Duration(cd+1)*time.Second)),
		"cooldown elapsed → CooledDown=true")
}

// TestRuleStore_NotFound — Get/Disable/Enable/Touch on a non-existent id
// return ErrRuleNotFound.
func TestRuleStore_NotFound(t *testing.T) {
	store, _ := setupRuleStoreTest(t)
	pool := store.pool
	ctx := context.Background()

	bogus := uuid.New()

	_, err := store.GetRuleByID(ctx, bogus)
	require.ErrorIs(t, err, ErrRuleNotFound)

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()
	require.ErrorIs(t, store.DisableRule(ctx, tx, bogus), ErrRuleNotFound)
	require.ErrorIs(t, store.EnableRule(ctx, tx, bogus), ErrRuleNotFound)
	require.ErrorIs(t, store.TouchLastFiredAt(ctx, tx, bogus), ErrRuleNotFound)
}
