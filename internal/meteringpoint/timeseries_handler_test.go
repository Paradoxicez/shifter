package meteringpoint

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// TestTimeseriesHandler_InvalidRange — unknown range → 400.
func TestTimeseriesHandler_InvalidRange(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, nopLogger()))

	f := &detailFixture{newMPFixtureWithPool(t, pool)}
	f.seedRole(t, "viewer")

	mpID := f.seedMP(t, "TS Range Test MP", "water")

	res := f.doJSON(t, "GET", "/api/metering-points/"+mpID+"/timeseries?range=bogus", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)
}

// TestTimeseriesHandler_CustomMissingDates — custom range without start/end → 400.
func TestTimeseriesHandler_CustomMissingDates(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, nopLogger()))

	f := &detailFixture{newMPFixtureWithPool(t, pool)}
	f.seedRole(t, "viewer")

	mpID := f.seedMP(t, "TS Custom Test MP", "water")

	res := f.doJSON(t, "GET", "/api/metering-points/"+mpID+"/timeseries?range=custom", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)
}

// TestTimeseriesHandler_HappyPath_24h — valid 24h range returns 200 with series.
func TestTimeseriesHandler_HappyPath_24h(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, nopLogger()))

	f := &detailFixture{newMPFixtureWithPool(t, pool)}
	f.seedRole(t, "viewer")

	mpID := f.seedMP(t, "TS Happy MP", "water")
	// Insert 2 measurement rows in the last hour.
	seedMeasurementAt(t, pool, mpID, 100.0, "ok", time.Now().Add(-10*time.Minute))
	seedMeasurementAt(t, pool, mpID, 101.0, "ok", time.Now().Add(-20*time.Minute))

	res := f.doJSON(t, "GET", "/api/metering-points/"+mpID+"/timeseries?range=24h", nil)
	defer res.Body.Close()

	require.Equal(t, http.StatusOK, res.StatusCode)

	var body MPTimeseriesResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))

	// 24h → 5-minute buckets (D-12).
	assert.Equal(t, 300, body.BucketIntervalSeconds)
	assert.NotEmpty(t, body.Series)
}

// TestTimeseriesHandler_InvalidUUID — malformed `:id` → 400.
func TestTimeseriesHandler_InvalidUUID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, nopLogger()))

	f := &detailFixture{newMPFixtureWithPool(t, pool)}
	f.seedRole(t, "viewer")

	res := f.doJSON(t, "GET", "/api/metering-points/not-a-uuid/timeseries?range=24h", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)
}
