package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// TestTimeseriesHandler_BucketIntervals verifies D-12: handler selects the
// correct bucket interval per range value. Checked via bucket_interval_seconds
// in the JSON response.
func TestTimeseriesHandler_BucketIntervals(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	require.NoError(t, db.RunMigrations(context.Background(), pool, log))
	seedInstallIdentity(t, pool, "water")

	deps := newTestDashboardDeps(pool)

	tests := []struct {
		rangeStr      string
		wantBucketSec int
	}{
		{"today", int((5 * time.Minute).Seconds())},
		{"24h", int((5 * time.Minute).Seconds())},
		{"7d", int(time.Hour.Seconds())},
		{"30d", int((4 * time.Hour).Seconds())},
	}

	for _, tc := range tests {
		t.Run(tc.rangeStr, func(t *testing.T) {
			url := fmt.Sprintf("/api/dashboard/timeseries?range=%s&utility=water", tc.rangeStr)
			req := httptest.NewRequest(http.MethodGet, url, nil)
			w := httptest.NewRecorder()
			deps.handleTimeseries(w, req)

			require.Equal(t, http.StatusOK, w.Code, "unexpected status for range=%s: %s", tc.rangeStr, w.Body.String())

			var resp TimeseriesResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			assert.Equal(t, tc.wantBucketSec, resp.BucketIntervalSeconds, "range=%s", tc.rangeStr)
			assert.Equal(t, "water", resp.Utility)
			assert.NotNil(t, resp.Series)
		})
	}
}

// TestTimeseriesHandler_CustomBuckets verifies D-12 custom range bucketing:
// ≤30d → 1h, >30d → 1day.
func TestTimeseriesHandler_CustomBuckets(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	require.NoError(t, db.RunMigrations(context.Background(), pool, log))
	seedInstallIdentity(t, pool, "water")

	deps := newTestDashboardDeps(pool)

	now := time.Now().UTC()

	// custom ≤ 30d → 1 hour
	start7d := now.Add(-7 * 24 * time.Hour).Format(time.RFC3339)
	end := now.Format(time.RFC3339)
	url := fmt.Sprintf("/api/dashboard/timeseries?range=custom&utility=water&start=%s&end=%s", start7d, end)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	deps.handleTimeseries(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	var resp TimeseriesResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, int(time.Hour.Seconds()), resp.BucketIntervalSeconds, "custom ≤30d should be 1h")

	// custom > 30d → 1 day
	start31d := now.Add(-31 * 24 * time.Hour).Format(time.RFC3339)
	url = fmt.Sprintf("/api/dashboard/timeseries?range=custom&utility=water&start=%s&end=%s", start31d, end)
	req = httptest.NewRequest(http.MethodGet, url, nil)
	w = httptest.NewRecorder()
	deps.handleTimeseries(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, int((24 * time.Hour).Seconds()), resp.BucketIntervalSeconds, "custom >30d should be 1day")
}

// TestTimeseriesHandler_InvalidRange verifies T-04-04-01: invalid range returns 400.
func TestTimeseriesHandler_InvalidRange(t *testing.T) {
	pool := testsupport.StartPostgres(t)
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	require.NoError(t, db.RunMigrations(context.Background(), pool, log))

	deps := newTestDashboardDeps(pool)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/timeseries?range=yearly&utility=water", nil)
	w := httptest.NewRecorder()
	deps.handleTimeseries(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestTimeseriesHandler_InvalidUtility verifies that utility must be water or electricity.
func TestTimeseriesHandler_InvalidUtility(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	require.NoError(t, db.RunMigrations(context.Background(), pool, log))

	deps := newTestDashboardDeps(pool)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/timeseries?range=7d&utility=gas", nil)
	w := httptest.NewRecorder()
	deps.handleTimeseries(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestTimeseriesHandler_CustomMissingStartEnd verifies that custom range without
// start/end returns 400.
func TestTimeseriesHandler_CustomMissingStartEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	require.NoError(t, db.RunMigrations(context.Background(), pool, log))

	deps := newTestDashboardDeps(pool)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/timeseries?range=custom&utility=water", nil)
	w := httptest.NewRecorder()
	deps.handleTimeseries(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestTimeseriesHandler_CustomRangeExceeds1Year verifies T-04-04-03: custom
// range > 1 year returns 400.
func TestTimeseriesHandler_CustomRangeExceeds1Year(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	require.NoError(t, db.RunMigrations(context.Background(), pool, log))

	deps := newTestDashboardDeps(pool)

	now := time.Now().UTC()
	start := now.Add(-366 * 24 * time.Hour).Format(time.RFC3339)
	end := now.Format(time.RFC3339)
	url := fmt.Sprintf("/api/dashboard/timeseries?range=custom&utility=water&start=%s&end=%s", start, end)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	deps.handleTimeseries(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestTimeseriesHandler_EmptyDB verifies that a timeseries query against an
// empty measurement table returns 200 with an empty (not nil) series array.
func TestTimeseriesHandler_EmptyDB(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	require.NoError(t, db.RunMigrations(context.Background(), pool, log))

	deps := newTestDashboardDeps(pool)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/timeseries?range=7d&utility=water", nil)
	w := httptest.NewRecorder()
	deps.handleTimeseries(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp TimeseriesResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "water", resp.Utility)
	assert.NotNil(t, resp.Series)
}
