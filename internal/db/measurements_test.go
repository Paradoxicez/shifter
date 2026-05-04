package db

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shifter-io/shifter/internal/db/sqlc"
	"github.com/shifter-io/shifter/internal/testsupport"
	"github.com/stretchr/testify/require"
)

// TestAppendMeasurement_RoundTrip — Plan 02-06 Task 3 acceptance test.
// Apply migrations, insert a row through sqlc.AppendMeasurement with all
// 20 fields populated, then assert GetLatestMeasurement round-trips the
// same row. Pins the AppendMeasurement column wiring (positional placeholder
// → migration column order) — if a future schema migration adds a column
// without updating measurements.sql, the row Scan in GetLatestMeasurement
// fails and this test catches it.
func TestAppendMeasurement_RoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, RunMigrations(ctx, pool, log))

	q := sqlc.New(pool)

	// Seed the minimum graph: site → MP → device_profile → device → binding
	// (so the binding_id forward-compat column has a valid UUID to point at).
	var siteID, mpID, profileID, deviceID, bindingID pgtype.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO site (name, timezone) VALUES ('test-site', 'UTC') RETURNING id`,
	).Scan(&siteID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO metering_point (site_id, name, utility_class)
		 VALUES ($1, 'mp-1', 'water') RETURNING id`,
		siteID,
	).Scan(&mpID))
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT id FROM device_profile WHERE slug = 'axioma_w1'`,
	).Scan(&profileID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO device (dev_eui, name, device_profile_id)
		 VALUES ('1111111111111111', 'test-dev', $1) RETURNING id`,
		profileID,
	).Scan(&deviceID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO binding (metering_point_id, device_id, valid_from)
		 VALUES ($1, $2, '2026-01-01T00:00:00Z') RETURNING id`,
		mpID, deviceID,
	).Scan(&bindingID))

	// Build a measurement row with EVERY column populated (no NULLs) so the
	// round-trip exercises every Scan target.
	measurementTime := pgtype.Timestamptz{Time: time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC), Valid: true}
	gatewayRxTime := pgtype.Timestamptz{Time: time.Date(2026, 5, 4, 11, 59, 58, 0, time.UTC), Valid: true}
	deviceTime := pgtype.Timestamptz{Time: time.Date(2026, 5, 4, 11, 59, 57, 0, time.UTC), Valid: true}

	rawVal := numeric(t, "12345.678")
	cumVal := numeric(t, "98765.432")
	instVal := numeric(t, "0.42")

	battery := int16(87)
	rssi := int16(-78)
	snr := float32(9.4)
	tempC := float32(22.5)
	presKpa := float32(101.3)
	leak := false
	tamper := false
	fcnt := int32(42)

	params := sqlc.AppendMeasurementParams{
		Time:            measurementTime,
		MeteringPointID: mpID,
		RawValue:        rawVal,
		CumulativeValue: cumVal,
		InstantValue:    instVal,
		BatteryPct:      &battery,
		Rssi:            &rssi,
		Snr:             &snr,
		TemperatureC:    &tempC,
		PressureKpa:     &presKpa,
		LeakDetected:    &leak,
		TamperDetected:  &tamper,
		Extra:           []byte(`{"vendor_specific_field":"hello"}`),
		RawPayload:      []byte{0x01, 0x02, 0x03, 0x04},
		DecodedObject:   []byte(`{"cumulative_l":12345.678}`),
		Quality:         "ok",
		Fcnt:            &fcnt,
		GatewayRxTime:   gatewayRxTime,
		DeviceTime:      deviceTime,
		BindingID:       bindingID,
	}

	require.NoError(t, q.AppendMeasurement(ctx, params), "AppendMeasurement must succeed with all 20 fields")

	// Round-trip via GetLatestMeasurement and assert each field survived.
	got, err := q.GetLatestMeasurement(ctx, mpID)
	require.NoError(t, err)

	require.Equal(t, measurementTime.Time.UTC(), got.Time.Time.UTC(), "time")
	require.Equal(t, mpID.Bytes, got.MeteringPointID.Bytes, "metering_point_id (DATA-01 invariant)")
	require.Equal(t, "ok", got.Quality, "quality persisted")
	require.NotNil(t, got.BatteryPct)
	require.Equal(t, int16(87), *got.BatteryPct, "battery_pct round-trip")
	require.NotNil(t, got.Rssi)
	require.Equal(t, int16(-78), *got.Rssi, "rssi round-trip")
	require.NotNil(t, got.Snr)
	require.InDelta(t, 9.4, *got.Snr, 0.001, "snr round-trip")
	require.NotNil(t, got.TemperatureC)
	require.InDelta(t, 22.5, *got.TemperatureC, 0.001)
	require.NotNil(t, got.PressureKpa)
	require.InDelta(t, 101.3, *got.PressureKpa, 0.001)
	require.NotNil(t, got.LeakDetected)
	require.False(t, *got.LeakDetected)
	require.NotNil(t, got.TamperDetected)
	require.False(t, *got.TamperDetected)
	require.JSONEq(t, `{"vendor_specific_field":"hello"}`, string(got.Extra), "extra JSONB round-trip")
	require.Equal(t, []byte{0x01, 0x02, 0x03, 0x04}, got.RawPayload, "raw_payload BYTEA round-trip (DATA-07: never lose)")
	require.JSONEq(t, `{"cumulative_l":12345.678}`, string(got.DecodedObject), "decoded_object JSONB round-trip (DATA-07)")
	require.NotNil(t, got.Fcnt)
	require.Equal(t, int32(42), *got.Fcnt)
	require.Equal(t, gatewayRxTime.Time.UTC(), got.GatewayRxTime.Time.UTC())
	require.Equal(t, deviceTime.Time.UTC(), got.DeviceTime.Time.UTC())
	require.Equal(t, bindingID.Bytes, got.BindingID.Bytes, "binding_id forward-compat (Plan 02-04 D-15)")

	// raw_value / cumulative_value / instant_value are NUMERIC — compare as
	// float64 so we don't trip over pgtype.Numeric Int+Exp encoding shifts.
	requireNumericEqual(t, rawVal, got.RawValue, "raw_value")
	requireNumericEqual(t, cumVal, got.CumulativeValue, "cumulative_value")
	requireNumericEqual(t, instVal, got.InstantValue, "instant_value")
}

// TestCountFlaggedRecent_Categorizes — D-26 quality-flag categorization
// regression test. Inserts 3 'ok' + 1 'decode_fail' + 2 'missing_canonical'
// rows; CountFlaggedRecent returns Total=3 (ok rows excluded by WHERE) +
// per-category counts. Pins the count(*) FILTER pattern in the query so a
// future SQL refactor doesn't silently break the badge counts.
func TestCountFlaggedRecent_Categorizes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, RunMigrations(ctx, pool, log))

	q := sqlc.New(pool)

	// Seed graph: site → MP. (No binding needed for measurements with NULL binding_id.)
	var siteID, mpID pgtype.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO site (name, timezone) VALUES ('test-site', 'UTC') RETURNING id`,
	).Scan(&siteID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO metering_point (site_id, name, utility_class)
		 VALUES ($1, 'mp-1', 'water') RETURNING id`,
		siteID,
	).Scan(&mpID))

	// All rows in the recent window. Insert with monotonically advancing time
	// so each row is distinct in the (mp, time) hypertable PK.
	base := time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC)
	insert := func(offset time.Duration, quality string) {
		params := sqlc.AppendMeasurementParams{
			Time:            pgtype.Timestamptz{Time: base.Add(offset), Valid: true},
			MeteringPointID: mpID,
			Extra:           []byte(`{}`),
			RawPayload:      []byte{0x00},
			DecodedObject:   []byte(`{}`),
			Quality:         quality,
		}
		require.NoError(t, q.AppendMeasurement(ctx, params), "AppendMeasurement(%s)", quality)
	}

	insert(0*time.Second, "ok")
	insert(1*time.Second, "ok")
	insert(2*time.Second, "ok")
	insert(3*time.Second, "decode_fail")
	insert(4*time.Second, "missing_canonical")
	insert(5*time.Second, "missing_canonical")

	// Time floor BEFORE all inserted rows so they all match the >= filter.
	floor := pgtype.Timestamptz{Time: base.Add(-1 * time.Hour), Valid: true}
	got, err := q.CountFlaggedRecent(ctx, sqlc.CountFlaggedRecentParams{
		MeteringPointID: mpID,
		Time:            floor,
	})
	require.NoError(t, err)

	// quality <> 'ok' WHERE filter excludes the 3 ok rows; remaining 3 are
	// 1 decode_fail + 2 missing_canonical.
	require.Equal(t, int64(3), got.Total, "Total counts only flagged rows (quality <> 'ok')")
	require.Equal(t, int64(1), got.DecodeFail)
	require.Equal(t, int64(2), got.MissingCanonical)
	require.Equal(t, int64(0), got.OutOfRange)
	require.Equal(t, int64(0), got.DuplicateFcnt)
}

// TestWriteAuditLog_RoundTrip — Plan 02-06 Task 3 acceptance test for the
// 8-field audit_log INSERT. Asserts the row persists with both before and
// after JSONB diffs intact + retrievable via GetAuditEntry. Same-tx
// behavior is asserted at the audit package layer in Plan 02-08; this test
// only pins the sqlc query column wiring.
func TestWriteAuditLog_RoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	ctx := context.Background()
	require.NoError(t, RunMigrations(ctx, pool, log))

	q := sqlc.New(pool)

	// Seed the FK target.
	var userID pgtype.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('audit@example.com', 'Auditor', 'x', 'admin') RETURNING id`,
	).Scan(&userID))

	// Provide a fixed entity_id so we can list-by-entity below.
	var entityID pgtype.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT gen_random_uuid()`,
	).Scan(&entityID))

	notes := "test note"
	reqID := "req-test-123"
	require.NoError(t, q.WriteAuditLog(ctx, sqlc.WriteAuditLogParams{
		UserID:     userID,
		Action:     "update",
		EntityType: "site",
		EntityID:   entityID,
		Before:     []byte(`{"name":"old"}`),
		After:      []byte(`{"name":"new"}`),
		Notes:      &notes,
		RequestID:  &reqID,
	}))

	// Round-trip via ListAuditEntriesByEntity.
	entries, err := q.ListAuditEntriesByEntity(ctx, sqlc.ListAuditEntriesByEntityParams{
		EntityType: "site",
		EntityID:   entityID,
		Limit:      10,
		Offset:     0,
	})
	require.NoError(t, err)
	require.Len(t, entries, 1, "exactly one matching entry")

	got := entries[0]
	require.Equal(t, "update", got.Action)
	require.Equal(t, "site", got.EntityType)
	require.Equal(t, entityID.Bytes, got.EntityID.Bytes)
	require.Equal(t, userID.Bytes, got.UserID.Bytes)
	require.JSONEq(t, `{"name":"old"}`, string(got.Before), "before JSONB round-trip")
	require.JSONEq(t, `{"name":"new"}`, string(got.After), "after JSONB round-trip")
	require.NotNil(t, got.Notes)
	require.Equal(t, "test note", *got.Notes)
	require.NotNil(t, got.RequestID)
	require.Equal(t, "req-test-123", *got.RequestID)

	// CountAuditEntriesByAction — sanity check the count helper.
	floor := pgtype.Timestamptz{Time: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), Valid: true}
	count, err := q.CountAuditEntriesByAction(ctx, sqlc.CountAuditEntriesByActionParams{
		Action: "update",
		Time:   floor,
	})
	require.NoError(t, err)
	require.GreaterOrEqual(t, count, int64(1), "at least one 'update' action since 2020")
}

// numeric builds a pgtype.Numeric from a decimal string so test fixtures
// stay readable without the pgtype.Numeric.Set(*big.Rat) ceremony.
func numeric(t *testing.T, s string) pgtype.Numeric {
	t.Helper()
	var n pgtype.Numeric
	require.NoError(t, n.Scan(s))
	return n
}

// requireNumericEqual compares two pgtype.Numeric values by converting both
// to float64. NUMERIC's Int + Exp encoding can shift between equivalent
// representations after a Postgres round-trip; comparing magnitudes via
// Float64Value avoids false negatives on equal values with different
// internal exponents. The InDelta tolerance is well below the test fixture
// precision (3 decimal places).
func requireNumericEqual(t *testing.T, want, got pgtype.Numeric, msg string) {
	t.Helper()
	wantF := numericToFloat(t, want)
	gotF := numericToFloat(t, got)
	require.InDelta(t, wantF, gotF, 1e-6, "%s: want=%g got=%g", msg, wantF, gotF)
}

func numericToFloat(t *testing.T, n pgtype.Numeric) float64 {
	t.Helper()
	require.True(t, n.Valid, "numeric must be valid")
	f, err := n.Float64Value()
	require.NoError(t, err)
	require.True(t, f.Valid, "float64 conversion produced invalid")
	return f.Float64
}
