package install

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestFinishSetup_SeedsRetentionConfig verifies that FinishSetup inserts the
// D-09 defaults (90, 365, 1825, 7300, NULL) into retention_config inside the
// same Serializable transaction as admin + identity + chirpstack_connection.
func TestFinishSetup_SeedsRetentionConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	deps, store := setupForFinish(t)
	seedAllFourSteps(t, store)

	ctx := context.Background()
	require.NoError(t, FinishSetup(ctx, deps))

	// Query the seeded row.
	var rawDays, hourlyDays, dailyDays, monthlyDays int
	var yearlyDays *int
	err := deps.Pool.QueryRow(ctx, `
		SELECT raw_days, hourly_days, daily_days, monthly_days, yearly_days
		FROM retention_config
		WHERE id = 1
	`).Scan(&rawDays, &hourlyDays, &dailyDays, &monthlyDays, &yearlyDays)
	require.NoError(t, err, "retention_config row id=1 must exist after FinishSetup")

	// D-09 defaults.
	require.Equal(t, 90, rawDays, "raw_days must be 90 (D-09)")
	require.Equal(t, 365, hourlyDays, "hourly_days must be 365 (D-09: 1 year)")
	require.Equal(t, 1825, dailyDays, "daily_days must be 1825 (D-09: 5 years)")
	require.Equal(t, 7300, monthlyDays, "monthly_days must be 7300 (D-09: 20 years)")
	require.Nil(t, yearlyDays, "yearly_days must be NULL (D-09: never drops)")

	// Phase 6 — D-13 + D-38 defaults must also be seeded (alerts_days=365,
	// audit_log_days=1825). 0040 sets NOT NULL DEFAULTs that bring these in
	// even on the bare INSERT path, but FinishSetup explicitly UPDATEs them
	// inside the same Serializable tx (belt-and-suspenders against future
	// column drift).
	var alertsDays, auditLogDays int
	err = deps.Pool.QueryRow(ctx,
		`SELECT alerts_days, audit_log_days FROM retention_config WHERE id = 1`,
	).Scan(&alertsDays, &auditLogDays)
	require.NoError(t, err, "Phase 6 columns must exist after FinishSetup")
	require.Equal(t, 365, alertsDays, "alerts_days must be 365 (D-13)")
	require.Equal(t, 1825, auditLogDays, "audit_log_days must be 1825 (D-38)")
}

// TestFinishSetup_RetentionRollsBackOnFailure verifies that when FinishSetup
// is prevented from running (admin already exists → ErrAlreadyCompleted),
// the retention_config row is also absent — the whole transaction was never
// committed.
func TestFinishSetup_RetentionRollsBackOnFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	deps, store := setupForFinish(t)
	seedAllFourSteps(t, store)

	ctx := context.Background()

	// Pre-insert the admin user with the SAME email as the wizard draft
	// (alice@example.com per seedAllFourSteps), so the FinishSetup pre-check
	// short-circuits with ErrAlreadyCompleted before opening a transaction.
	_, err := deps.Pool.Exec(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('alice@example.com', 'Pre-existing Admin', 'x', 'admin')`,
	)
	require.NoError(t, err, "pre-seed admin user for conflict setup")

	// FinishSetup pre-check: admin already exists → ErrAlreadyCompleted.
	err = FinishSetup(ctx, deps)
	require.ErrorIs(t, err, ErrAlreadyCompleted, "duplicate admin must return ErrAlreadyCompleted")

	// retention_config must be empty (no row seeded because txn was never opened).
	var count int
	require.NoError(t, deps.Pool.QueryRow(ctx,
		`SELECT count(*) FROM retention_config`,
	).Scan(&count))
	require.Equal(t, 0, count, "retention_config must be empty when FinishSetup aborted before txn")
}

// TestFinishSetup_RerunIsIdempotent verifies that calling FinishSetup a second
// time after success returns ErrAlreadyCompleted and does NOT insert a duplicate
// retention_config row (ON CONFLICT DO NOTHING belt-and-suspenders).
func TestFinishSetup_RerunIsIdempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	deps, store := setupForFinish(t)
	seedAllFourSteps(t, store)

	ctx := context.Background()
	require.NoError(t, FinishSetup(ctx, deps))

	// Second call: pre-check sees admin exists → ErrAlreadyCompleted.
	err := FinishSetup(ctx, deps)
	require.ErrorIs(t, err, ErrAlreadyCompleted)

	// retention_config must still have exactly 1 row.
	var count int
	require.NoError(t, deps.Pool.QueryRow(ctx,
		`SELECT count(*) FROM retention_config`,
	).Scan(&count))
	require.Equal(t, 1, count, "retention_config must have exactly 1 row after idempotent re-run")
}
