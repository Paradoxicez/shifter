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

// TestSignalHandler_HappyPath — 200 with correct structure (D-17 fixed 24h window).
func TestSignalHandler_HappyPath(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, nopLogger()))

	f := &detailFixture{newMPFixtureWithPool(t, pool)}
	f.seedRole(t, "viewer")

	mpID := f.seedMP(t, "Signal Test MP", "water")
	// Insert a measurement within the last hour so there is at least one bucket.
	seedMeasurementAt(t, pool, mpID, 50.0, "ok", time.Now().Add(-30*time.Minute))

	res := f.doJSON(t, "GET", "/api/metering-points/"+mpID+"/signal-history", nil)
	defer res.Body.Close()

	require.Equal(t, http.StatusOK, res.StatusCode)

	var body SignalHistoryResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))

	// D-17: always 1-hour buckets.
	assert.Equal(t, 3600, body.BucketIntervalSeconds)
	assert.NotEmpty(t, body.WindowStart)
	assert.NotEmpty(t, body.WindowEnd)

	// At least one series bucket (our measurement above falls in the last hour).
	assert.NotEmpty(t, body.Series)
}

// TestSignalHandler_EmptyMP — MP with no measurements returns 200 with empty series.
func TestSignalHandler_EmptyMP(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, nopLogger()))

	f := &detailFixture{newMPFixtureWithPool(t, pool)}
	f.seedRole(t, "viewer")

	mpID := f.seedMP(t, "Empty Signal MP", "water")

	res := f.doJSON(t, "GET", "/api/metering-points/"+mpID+"/signal-history", nil)
	defer res.Body.Close()

	require.Equal(t, http.StatusOK, res.StatusCode)

	var body SignalHistoryResponse
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))

	assert.Equal(t, 3600, body.BucketIntervalSeconds)
	assert.Empty(t, body.Series, "no measurements → empty series")
}

// TestSignalHandler_InvalidUUID — malformed `:id` → 400.
func TestSignalHandler_InvalidUUID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool, nopLogger()))

	f := &detailFixture{newMPFixtureWithPool(t, pool)}
	f.seedRole(t, "viewer")

	res := f.doJSON(t, "GET", "/api/metering-points/not-a-uuid/signal-history", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)
}
