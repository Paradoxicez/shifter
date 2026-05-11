package meteringpoint

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// detailFixture extends mpFixture with seeding helpers for MP detail tests.
type detailFixture struct {
	*mpFixture
}

func newDetailFixture(t *testing.T) *detailFixture {
	t.Helper()
	// newMPFixture already calls testsupport.StartPostgres + db.RunMigrations.
	return &detailFixture{newMPFixture(t)}
}

// seedMP creates a metering point and returns its ID string.
func (f *detailFixture) seedMP(t *testing.T, name, utilityClass string) string {
	t.Helper()
	var id string
	err := f.pool.QueryRow(context.Background(),
		`INSERT INTO metering_point (site_id, name, utility_class)
		 VALUES ($1, $2, $3) RETURNING id::text`,
		f.siteID, name, utilityClass,
	).Scan(&id)
	require.NoError(t, err)
	return id
}

// seedDeviceAndProfile seeds a device profile + device and returns device_id, profile_id.
func (f *detailFixture) seedDeviceAndProfile(t *testing.T, pool *pgxpool.Pool) (deviceID, profileID string) {
	t.Helper()
	ctx := context.Background()
	err := pool.QueryRow(ctx,
		`INSERT INTO device_profile (slug, name, vendor, expected_interval_s)
		 VALUES ('test-profile-detail', 'Test Profile Detail', 'TestVendor', 3600)
		 ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name
		 RETURNING id::text`,
	).Scan(&profileID)
	require.NoError(t, err)

	err = pool.QueryRow(ctx,
		`INSERT INTO device (dev_eui, name, device_profile_id)
		 VALUES ('aabbccdd11223344', 'Test Device Detail', $1)
		 ON CONFLICT (dev_eui) DO UPDATE SET name = EXCLUDED.name
		 RETURNING id::text`, profileID,
	).Scan(&deviceID)
	require.NoError(t, err)
	return
}

// seedBinding creates an active binding for (mp_id, device_id).
func seedBinding(t *testing.T, pool *pgxpool.Pool, mpID, deviceID string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO binding (metering_point_id, device_id, valid_from)
		 VALUES ($1::uuid, $2::uuid, now() - interval '1 day')`,
		mpID, deviceID,
	)
	require.NoError(t, err)
}

// seedMeasurement inserts a measurement row for the given metering_point_id.
func seedMeasurement(t *testing.T, pool *pgxpool.Pool, mpID string, cumulative float64, quality string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO measurement
		     (time, metering_point_id, cumulative_value, raw_payload, decoded_object, quality)
		 VALUES
		     (now(), $1::uuid, $2, '\xdeadbeef'::bytea, '{"test":true}'::jsonb, $3)`,
		mpID, cumulative, quality,
	)
	require.NoError(t, err)
}

// TestDetailHandler_EmptyMP — D-22: MP with no binding, no measurements → 200
// with active_binding=null, latest_reading=null, online=null.
func TestDetailHandler_EmptyMP(t *testing.T) {
	f := newDetailFixture(t)
	f.seedRole(t, "viewer")

	mpID := f.seedMP(t, "Empty MP", "water")

	res := f.doJSON(t, "GET", "/api/metering-points/"+mpID, nil)
	defer res.Body.Close()

	require.Equal(t, http.StatusOK, res.StatusCode)

	var body DetailResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))

	assert.Equal(t, mpID, body.MeteringPoint.ID)
	assert.Equal(t, "water", body.MeteringPoint.UtilityClass)
	assert.Nil(t, body.ActiveBinding, "active_binding must be null — no binding exists")
	assert.Nil(t, body.LatestReading, "latest_reading must be null — no measurements")
	assert.Nil(t, body.Online, "online must be null — no device bound")
	assert.Equal(t, 0, body.QualitySummary.WindowSize)
	assert.Equal(t, 0, body.QualitySummary.FlaggedCount)
}

// TestDetailHandler_WithBindingAndReading — MP with active binding + measurement.
func TestDetailHandler_WithBindingAndReading(t *testing.T) {
	f := newDetailFixture(t)
	f.seedRole(t, "viewer")

	mpID := f.seedMP(t, "Wired MP", "water")
	deviceID, _ := f.seedDeviceAndProfile(t, f.pool)
	seedBinding(t, f.pool, mpID, deviceID)
	seedMeasurement(t, f.pool, mpID, 1234.56, "ok")

	res := f.doJSON(t, "GET", "/api/metering-points/"+mpID, nil)
	defer res.Body.Close()

	require.Equal(t, http.StatusOK, res.StatusCode)

	var body DetailResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))

	assert.Equal(t, mpID, body.MeteringPoint.ID)
	assert.NotNil(t, body.ActiveBinding, "active_binding must be present")
	assert.NotNil(t, body.LatestReading, "latest_reading must be present")
	assert.NotNil(t, body.Online, "online must be non-null when device is bound")

	// raw_payload_hex: "\xdeadbeef" → "de ad be ef".
	assert.Equal(t, "de ad be ef", body.LatestReading.RawPayloadHex)

	// quality_summary: 1 row with quality=ok.
	assert.Equal(t, 1, body.QualitySummary.WindowSize)
	assert.Equal(t, 0, body.QualitySummary.FlaggedCount)
	assert.Equal(t, map[string]int{"ok": 1}, body.QualitySummary.ByQuality)
}

// TestDetailHandler_NotFound — unknown UUID → 404.
func TestDetailHandler_NotFound(t *testing.T) {
	f := newDetailFixture(t)
	f.seedRole(t, "viewer")

	unknownID := "00000000-0000-0000-0000-000000000001"
	res := f.doJSON(t, "GET", "/api/metering-points/"+unknownID, nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusNotFound, res.StatusCode)
}

// TestDetailHandler_InvalidUUID — malformed UUID → 400.
func TestDetailHandler_InvalidUUID(t *testing.T) {
	f := newDetailFixture(t)
	f.seedRole(t, "viewer")

	res := f.doJSON(t, "GET", "/api/metering-points/not-a-uuid", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)
}

// TestDetailHandler_QualitySummary — mix of ok + flagged rows in window.
func TestDetailHandler_QualitySummary(t *testing.T) {
	f := newDetailFixture(t)
	f.seedRole(t, "viewer")

	mpID := f.seedMP(t, "Quality MP", "water")

	// Insert 3 ok + 2 decode_fail + 1 out_of_range = 6 rows.
	for i := 0; i < 3; i++ {
		seedMeasurementAt(t, f.pool, mpID, float64(i), "ok", time.Now().Add(-time.Duration(i+1)*time.Minute))
	}
	seedMeasurementAt(t, f.pool, mpID, 10, "decode_fail", time.Now().Add(-5*time.Minute))
	seedMeasurementAt(t, f.pool, mpID, 11, "decode_fail", time.Now().Add(-6*time.Minute))
	seedMeasurementAt(t, f.pool, mpID, 12, "out_of_range", time.Now().Add(-7*time.Minute))

	res := f.doJSON(t, "GET", "/api/metering-points/"+mpID, nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var body DetailResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))

	assert.Equal(t, 6, body.QualitySummary.WindowSize)
	assert.Equal(t, 3, body.QualitySummary.FlaggedCount) // decode_fail(2) + out_of_range(1)
	assert.Equal(t, 3, body.QualitySummary.ByQuality["ok"])
	assert.Equal(t, 2, body.QualitySummary.ByQuality["decode_fail"])
	assert.Equal(t, 1, body.QualitySummary.ByQuality["out_of_range"])
}

// seedMeasurementAt inserts a measurement at a specific time.
func seedMeasurementAt(t *testing.T, pool *pgxpool.Pool, mpID string, cumulative float64, quality string, at time.Time) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO measurement
		     (time, metering_point_id, cumulative_value, raw_payload, decoded_object, quality)
		 VALUES
		     ($1, $2::uuid, $3, '\x00'::bytea, '{}'::jsonb, $4)`,
		at, mpID, cumulative, quality,
	)
	require.NoError(t, err)
}
