package swap

import (
	"context"
	"log/slog"
	"math/big"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// fakeInvalidator records every Invalidate call so commit tests can assert
// the resolver-side defense-in-depth fires for both dev_euis.
type fakeInvalidator struct {
	mu    sync.Mutex
	calls []string
}

func (f *fakeInvalidator) Invalidate(devEUI string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, devEUI)
}

func (f *fakeInvalidator) snapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.calls))
	copy(out, f.calls)
	return out
}

// swapFixture seeds the minimum graph needed by every commit test:
//   - 1 site, 1 metering_point, 1 device_profile (axioma_w1 from 0010 seed)
//   - 2 devices (outgoing + incoming) with unique dev_euis
//   - 1 active binding (outgoing device on the MP, valid_from in the past)
type swapFixture struct {
	siteID       pgtype.UUID
	mpID         pgtype.UUID
	profileID    pgtype.UUID
	outDeviceID  uuid.UUID
	outDevEUI    string
	inDeviceID   uuid.UUID
	inDevEUI     string
	bindingID    uuid.UUID
	bindingStart time.Time
}

func seedSwapFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool, suffix string) swapFixture {
	t.Helper()
	var f swapFixture
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO site (name, timezone) VALUES ($1, 'UTC') RETURNING id`,
		"site-"+suffix,
	).Scan(&f.siteID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO metering_point (site_id, name, utility_class)
		 VALUES ($1, $2, 'water') RETURNING id`,
		f.siteID, "mp-"+suffix,
	).Scan(&f.mpID))
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT id FROM device_profile WHERE slug = 'axioma_w1'`,
	).Scan(&f.profileID))

	// dev_eui must be lowercase 16-hex per 0012 CHECK.
	f.outDevEUI = "0011223344" + suffix + "01"
	f.outDevEUI = padDevEUI(f.outDevEUI)
	f.inDevEUI = "0011223344" + suffix + "02"
	f.inDevEUI = padDevEUI(f.inDevEUI)

	var outID, inID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO device (dev_eui, name, device_profile_id)
		 VALUES ($1, $2, $3) RETURNING id`,
		f.outDevEUI, "out-dev-"+suffix, f.profileID,
	).Scan(&outID))
	f.outDeviceID = uuid.MustParse(outID)
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO device (dev_eui, name, device_profile_id)
		 VALUES ($1, $2, $3) RETURNING id`,
		f.inDevEUI, "in-dev-"+suffix, f.profileID,
	).Scan(&inID))
	f.inDeviceID = uuid.MustParse(inID)

	f.bindingStart = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	var bid string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO binding (metering_point_id, device_id, valid_from, reading_offset)
		 VALUES ($1, $2, $3, 0) RETURNING id`,
		f.mpID, outID, f.bindingStart,
	).Scan(&bid))
	f.bindingID = uuid.MustParse(bid)
	return f
}

// padDevEUI pads / truncates to a lowercase 16-hex string.
func padDevEUI(s string) string {
	const want = 16
	hex := ""
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
			hex += string(c)
		case c >= 'a' && c <= 'f':
			hex += string(c)
		case c >= 'A' && c <= 'F':
			hex += string(c - 'A' + 'a')
		}
	}
	for len(hex) < want {
		hex = "0" + hex
	}
	if len(hex) > want {
		hex = hex[len(hex)-want:]
	}
	return hex
}

// TestCommitSwap_HappyPath — DATA-04 baseline:
//   - operator captures R=12345 from outgoing meter
//   - new meter starts at N=0
//   - CommitSwap closes the outgoing binding (valid_to = ConfirmTime),
//     opens a new one with reading_offset = R - N = 12345, writes a single
//     audit_log row with action='swap', returns the new binding id.
func TestCommitSwap_HappyPath(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	f := seedSwapFixture(t, ctx, pool, "happy")

	// Seed an admin user (audit_log.user_id FK target).
	var operatorID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('op@example.com', 'Op', 'x', 'admin') RETURNING id`,
	).Scan(&operatorID))
	operatorUUID := uuid.MustParse(operatorID)

	inv := &fakeInvalidator{}
	deps := Deps{Pool: pool, Resolver: inv, Log: log}

	confirmTime := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	in := SwapInput{
		UserID:            operatorUUID,
		RequestID:         "req-swap-happy",
		MeteringPointID:   uuid.UUID(f.mpID.Bytes),
		OutgoingBindingID: f.bindingID,
		OutgoingDevEUI:    f.outDevEUI,
		IncomingDeviceID:  f.inDeviceID,
		IncomingDevEUI:    f.inDevEUI,
		ConfirmTime:       confirmTime,
		OutgoingReadingR:  big.NewFloat(12345),
		IncomingInitialN:  big.NewFloat(0),
		OperatorNotes:     "annual replacement",
	}

	newBindingID, err := CommitSwap(ctx, deps, in)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, newBindingID)

	// Outgoing binding must have valid_to = confirmTime.
	var validTo pgtype.Timestamptz
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT valid_to FROM binding WHERE id = $1`, f.bindingID,
	).Scan(&validTo))
	require.True(t, validTo.Valid, "outgoing binding must be closed")
	require.WithinDuration(t, confirmTime, validTo.Time, time.Second)

	// New binding must exist with reading_offset = 12345 and valid_from = confirmTime.
	var newOffset pgtype.Numeric
	var newValidFrom pgtype.Timestamptz
	var newDeviceID pgtype.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT reading_offset, valid_from, device_id FROM binding WHERE id = $1`, newBindingID,
	).Scan(&newOffset, &newValidFrom, &newDeviceID))
	offsetF, err := newOffset.Float64Value()
	require.NoError(t, err)
	require.True(t, offsetF.Valid)
	require.InDelta(t, 12345.0, offsetF.Float64, 1e-6, "offset must equal R - N")
	require.WithinDuration(t, confirmTime, newValidFrom.Time, time.Second)
	require.Equal(t, f.inDeviceID[:], newDeviceID.Bytes[:])

	// Single 'swap' audit row, EntityID = new binding id.
	var auditCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log
		 WHERE action = 'swap' AND entity_id = $1 AND request_id = $2`,
		newBindingID, "req-swap-happy",
	).Scan(&auditCount))
	require.Equal(t, 1, auditCount)

	// Resolver got both dev_eui values invalidated.
	calls := inv.snapshot()
	require.Contains(t, calls, f.outDevEUI)
	require.Contains(t, calls, f.inDevEUI)
}

// TestCommitSwap_OperatorOverride — D-13: operator can override the proposed
// offset. The new binding's reading_offset must be the override value, and
// audit_log.after.override_used must be true.
func TestCommitSwap_OperatorOverride(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	f := seedSwapFixture(t, ctx, pool, "override")

	var operatorID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('op2@example.com', 'Op2', 'x', 'admin') RETURNING id`,
	).Scan(&operatorID))
	operatorUUID := uuid.MustParse(operatorID)

	deps := Deps{Pool: pool, Log: log}
	in := SwapInput{
		UserID:            operatorUUID,
		RequestID:         "req-swap-override",
		MeteringPointID:   uuid.UUID(f.mpID.Bytes),
		OutgoingBindingID: f.bindingID,
		OutgoingDevEUI:    f.outDevEUI,
		IncomingDeviceID:  f.inDeviceID,
		IncomingDevEUI:    f.inDevEUI,
		ConfirmTime:       time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC),
		OutgoingReadingR:  big.NewFloat(12345),
		IncomingInitialN:  big.NewFloat(0),
		OperatorOverride:  big.NewFloat(99999),
	}

	newBindingID, err := CommitSwap(ctx, deps, in)
	require.NoError(t, err)

	var newOffset pgtype.Numeric
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT reading_offset FROM binding WHERE id = $1`, newBindingID,
	).Scan(&newOffset))
	offsetF, err := newOffset.Float64Value()
	require.NoError(t, err)
	require.InDelta(t, 99999.0, offsetF.Float64, 1e-6, "override wins over proposed")

	// Audit row's after.override_used must be true. Use a SQL JSONB ->> probe
	// to avoid Postgres' Pretty-printing reformatting (spaces between key/value)
	// breaking a substring match.
	var overrideUsed bool
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT (after->>'override_used')::boolean FROM audit_log
		 WHERE entity_id = $1 AND action = 'swap'`, newBindingID,
	).Scan(&overrideUsed))
	require.True(t, overrideUsed, "audit_log.after.override_used must be true")
}

// TestCommitSwap_FailsOnDoubleClose — re-running CommitSwap on an already-
// closed binding triggers CloseBinding's idempotent guard ("WHERE valid_to
// IS NULL"). The query returns no row, sqlc surfaces pgx.ErrNoRows, and
// CommitSwap returns an error wrapping it. The first commit's data is
// preserved; the failed re-commit doesn't insert a duplicate audit row.
func TestCommitSwap_FailsOnDoubleClose(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	f := seedSwapFixture(t, ctx, pool, "doubleclose")

	var operatorID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('op3@example.com', 'Op3', 'x', 'admin') RETURNING id`,
	).Scan(&operatorID))
	operatorUUID := uuid.MustParse(operatorID)

	deps := Deps{Pool: pool, Log: log}
	in := SwapInput{
		UserID:            operatorUUID,
		MeteringPointID:   uuid.UUID(f.mpID.Bytes),
		OutgoingBindingID: f.bindingID,
		OutgoingDevEUI:    f.outDevEUI,
		IncomingDeviceID:  f.inDeviceID,
		IncomingDevEUI:    f.inDevEUI,
		ConfirmTime:       time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC),
		OutgoingReadingR:  big.NewFloat(12345),
		IncomingInitialN:  big.NewFloat(0),
	}

	// First commit succeeds.
	_, err := CommitSwap(ctx, deps, in)
	require.NoError(t, err)

	// Re-running with the now-closed binding fails on CloseBinding (idempotent
	// guard returns no row).
	_, err = CommitSwap(ctx, deps, in)
	require.Error(t, err, "re-closing a closed binding must fail")
	require.Contains(t, err.Error(), "close outgoing binding")

	// Exactly one swap audit row exists for this MP — the failed re-run
	// rolled back its in-flight audit insert.
	var swapCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log a
		 JOIN binding b ON b.id = a.entity_id
		 WHERE a.action = 'swap' AND b.metering_point_id = $1`,
		f.mpID,
	).Scan(&swapCount))
	require.Equal(t, 1, swapCount, "rolled-back retry must NOT leave an extra audit row")
}

// TestCommitSwap_RolledBack_OnAuditFailure — synthetic failure: pass an
// invalid Action via a test-only helper that bypasses the public WriteEntry
// path and forces audit_log_action_valid CHECK violation. The whole tx must
// roll back: outgoing binding remains open (valid_to NULL), no new binding
// row exists.
//
// Implementation note: CommitSwap doesn't expose a "bad audit" injection
// point because that would defeat the type-system guarantee. We exercise
// the rollback path indirectly: insert a uniqueness-violating outgoing
// binding (manually create a second active binding on the same device,
// then call CommitSwap which tries to OpenBinding for a still-active
// device). The btree_gist EXCLUDE on per-device fires; tx rolls back;
// the original binding state is intact.
func TestCommitSwap_RolledBack_OnExclusionViolation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	f := seedSwapFixture(t, ctx, pool, "rollback")

	// Pre-seed an OVERLAPPING active binding for the incoming device on a
	// DIFFERENT MP. Now CommitSwap's OpenBinding will violate
	// binding_no_overlap_per_device.
	var otherMP pgtype.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO metering_point (site_id, name, utility_class)
		 VALUES ($1, 'mp-other', 'water') RETURNING id`,
		f.siteID,
	).Scan(&otherMP))
	_, err := pool.Exec(ctx,
		`INSERT INTO binding (metering_point_id, device_id, valid_from, reading_offset)
		 VALUES ($1, $2, '2026-01-01T00:00:00Z', 0)`,
		otherMP, f.inDeviceID,
	)
	require.NoError(t, err, "seed conflicting active binding on incoming device")

	var operatorID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('op4@example.com', 'Op4', 'x', 'admin') RETURNING id`,
	).Scan(&operatorID))
	operatorUUID := uuid.MustParse(operatorID)

	deps := Deps{Pool: pool, Log: log}
	in := SwapInput{
		UserID:            operatorUUID,
		MeteringPointID:   uuid.UUID(f.mpID.Bytes),
		OutgoingBindingID: f.bindingID,
		OutgoingDevEUI:    f.outDevEUI,
		IncomingDeviceID:  f.inDeviceID,
		IncomingDevEUI:    f.inDevEUI,
		ConfirmTime:       time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC),
		OutgoingReadingR:  big.NewFloat(12345),
		IncomingInitialN:  big.NewFloat(0),
	}

	_, err = CommitSwap(ctx, deps, in)
	require.Error(t, err, "OpenBinding must fail on per-device EXCLUDE")
	require.Contains(t, err.Error(), "open incoming binding")

	// Outgoing binding must still be OPEN (CloseBinding inside the failed
	// tx was rolled back).
	var validTo pgtype.Timestamptz
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT valid_to FROM binding WHERE id = $1`, f.bindingID,
	).Scan(&validTo))
	require.False(t, validTo.Valid, "outgoing binding must remain open after rollback")

	// No swap audit row written.
	var auditCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log a
		 JOIN binding b ON b.id = a.entity_id
		 WHERE a.action = 'swap' AND b.metering_point_id = $1`,
		f.mpID,
	).Scan(&auditCount))
	require.Equal(t, 0, auditCount, "rolled-back tx must not leave audit row")
}

// TestCommitSwap_ConcurrentOneWins — DATA-04 concurrency contract: two swap
// commits on the same outgoing binding race; one wins, the other returns
// an error wrapping the underlying constraint violation. The handler layer
// (Plan 02-10) maps this to 409 Conflict.
func TestCommitSwap_ConcurrentOneWins(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, log))

	f := seedSwapFixture(t, ctx, pool, "concurrent")

	// Seed a third device — second concurrent commit will use this one as
	// "incoming." Both commits target the same outgoing binding so they
	// race on CloseBinding's WHERE valid_to IS NULL guard.
	var thirdDeviceID string
	thirdDevEUI := padDevEUI("00112233concurrent03")
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO device (dev_eui, name, device_profile_id)
		 VALUES ($1, $2, $3) RETURNING id`,
		thirdDevEUI, "third-dev-concurrent", f.profileID,
	).Scan(&thirdDeviceID))

	var operatorID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('op5@example.com', 'Op5', 'x', 'admin') RETURNING id`,
	).Scan(&operatorID))
	operatorUUID := uuid.MustParse(operatorID)

	deps := Deps{Pool: pool, Log: log}
	confirmTime := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)

	mkInput := func(incomingID uuid.UUID, incomingEUI, reqID string) SwapInput {
		return SwapInput{
			UserID:            operatorUUID,
			RequestID:         reqID,
			MeteringPointID:   uuid.UUID(f.mpID.Bytes),
			OutgoingBindingID: f.bindingID,
			OutgoingDevEUI:    f.outDevEUI,
			IncomingDeviceID:  incomingID,
			IncomingDevEUI:    incomingEUI,
			ConfirmTime:       confirmTime,
			OutgoingReadingR:  big.NewFloat(12345),
			IncomingInitialN:  big.NewFloat(0),
		}
	}

	type result struct {
		newID uuid.UUID
		err   error
	}
	results := make(chan result, 2)

	go func() {
		id, err := CommitSwap(ctx, deps, mkInput(f.inDeviceID, f.inDevEUI, "req-A"))
		results <- result{id, err}
	}()
	go func() {
		id, err := CommitSwap(ctx, deps, mkInput(uuid.MustParse(thirdDeviceID), thirdDevEUI, "req-B"))
		results <- result{id, err}
	}()

	r1 := <-results
	r2 := <-results

	winners := 0
	losers := 0
	for _, r := range []result{r1, r2} {
		if r.err == nil {
			winners++
			require.NotEqual(t, uuid.Nil, r.newID)
		} else {
			losers++
		}
	}
	require.Equal(t, 1, winners, "exactly one commit wins")
	require.Equal(t, 1, losers, "exactly one commit loses")

	// Exactly one swap audit row exists.
	var swapCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log a
		 JOIN binding b ON b.id = a.entity_id
		 WHERE a.action = 'swap' AND b.metering_point_id = $1`,
		f.mpID,
	).Scan(&swapCount))
	require.Equal(t, 1, swapCount, "exactly one audit row from the winner")
}
