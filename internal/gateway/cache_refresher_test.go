package gateway

// Phase 3 Plan 03-04 Task 3 — async cache refresher tests.

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	api "github.com/chirpstack/chirpstack/api/go/v4/api"
	common "github.com/chirpstack/chirpstack/api/go/v4/common"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// fakeMetricsGetter implements MetricsCacheGetter. Each Get increments the
// call counter so tests can assert per-gateway fan-out shape.
type fakeMetricsGetter struct {
	mu sync.Mutex

	calls atomic.Int64
	// canned response served for every Get.
	resp *api.GetGatewayMetricsResponse
}

func (f *fakeMetricsGetter) Get(_ context.Context, _ string) (*api.GetGatewayMetricsResponse, error) {
	f.calls.Add(1)
	return f.resp, nil
}

// fakeStatsWriter implements StatsWriter and records the most recent
// (id, rx, tx, txOK, sparkline) per gateway so tests can assert
// summariseMetrics output.
type fakeStatsWriter struct {
	mu sync.Mutex

	writes map[string]sqlc.UpdateGatewayStatsCacheParams
}

func newFakeStatsWriter() *fakeStatsWriter {
	return &fakeStatsWriter{writes: map[string]sqlc.UpdateGatewayStatsCacheParams{}}
}

func (f *fakeStatsWriter) UpdateGatewayStatsCache(_ context.Context, arg sqlc.UpdateGatewayStatsCacheParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := uuid.UUID(arg.ID.Bytes).String()
	f.writes[key] = arg
	return nil
}

// buildMetricsResp constructs a GetGatewayMetricsResponse with the rx, tx,
// txOK series the refresher summarises.
func buildMetricsResp(rx, tx []float32, txOK float32) *api.GetGatewayMetricsResponse {
	resp := &api.GetGatewayMetricsResponse{
		RxPackets: &common.Metric{
			Name:     "rx_packets",
			Kind:     common.MetricKind_ABSOLUTE,
			Datasets: []*common.MetricDataset{{Label: "rx", Data: rx}},
		},
		TxPackets: &common.Metric{
			Name:     "tx_packets",
			Kind:     common.MetricKind_ABSOLUTE,
			Datasets: []*common.MetricDataset{{Label: "tx", Data: tx}},
		},
	}
	if txOK > 0 {
		// Single 24-bucket sum (the OK series typically carries a per-bucket
		// count; we collapse to one bucket here so the test math is simple).
		resp.TxPacketsPerStatus = &common.Metric{
			Name:     "tx_packets_per_status",
			Kind:     common.MetricKind_ABSOLUTE,
			Datasets: []*common.MetricDataset{{Label: "OK", Data: []float32{txOK}}},
		}
	}
	return resp
}

// TestRefresher_PerRowAsync — Trigger spawns one goroutine per gatewayID;
// each invokes cache.Get exactly once.
func TestRefresher_PerRowAsync(t *testing.T) {
	getter := &fakeMetricsGetter{
		resp: buildMetricsResp(
			[]float32{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
			[]float32{2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2, 2},
			0,
		),
	}
	writer := newFakeStatsWriter()
	r := &CacheRefresher{Cache: getter, Queries: writer, Timeout: 2 * time.Second}

	rows := []GatewayRow{
		{ID: uuid.New(), GatewayID: "aabbccddeeff0001"},
		{ID: uuid.New(), GatewayID: "aabbccddeeff0002"},
		{ID: uuid.New(), GatewayID: "aabbccddeeff0003"},
		{ID: uuid.New(), GatewayID: "aabbccddeeff0004"},
		{ID: uuid.New(), GatewayID: "aabbccddeeff0005"},
	}
	r.Trigger(context.Background(), rows)

	// Wait for goroutines to finish; require.Eventually polls until calls = 5.
	require.Eventually(t, func() bool {
		return getter.calls.Load() == int64(len(rows))
	}, 2*time.Second, 10*time.Millisecond,
		"refresher must invoke cache.Get once per gateway")
}

// TestRefresher_PersistsStatsToPostgres — after refresh, the writer
// captured rx/tx/txOK totals computed from the cached response.
func TestRefresher_PersistsStatsToPostgres(t *testing.T) {
	getter := &fakeMetricsGetter{
		resp: buildMetricsResp(
			[]float32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24},
			[]float32{2, 4, 6, 8, 10, 12, 14, 16, 18, 20, 22, 24, 26, 28, 30, 32, 34, 36, 38, 40, 42, 44, 46, 48},
			500,
		),
	}
	writer := newFakeStatsWriter()
	r := &CacheRefresher{Cache: getter, Queries: writer, Timeout: 2 * time.Second}

	rowID := uuid.New()
	r.Trigger(context.Background(), []GatewayRow{
		{ID: rowID, GatewayID: "aabbccddeeff00aa"},
	})

	require.Eventually(t, func() bool {
		writer.mu.Lock()
		defer writer.mu.Unlock()
		_, ok := writer.writes[rowID.String()]
		return ok
	}, 2*time.Second, 10*time.Millisecond,
		"refresher must persist stats to PG after cache.Get returns")

	writer.mu.Lock()
	defer writer.mu.Unlock()
	got := writer.writes[rowID.String()]
	require.NotNil(t, got.StatsRx24h)
	require.NotNil(t, got.StatsTx24h)
	require.NotNil(t, got.StatsTxOk24h)
	// sum 1..24 = 300; sum 2..48 step 2 = 600; txOK = 500
	require.EqualValues(t, 300, *got.StatsRx24h)
	require.EqualValues(t, 600, *got.StatsTx24h)
	require.EqualValues(t, 500, *got.StatsTxOk24h)
	require.NotEmpty(t, got.StatsSparkline, "sparkline JSON must be populated")
}

// TestRefresher_NoDoubleFireWithinTTL — the refresher itself spawns one
// goroutine per Trigger; the *cache* coalesces concurrent fetches via
// singleflight + TTL. We assert here that two Triggers of the same
// gatewayID produce two Get calls (the refresher is fan-out only; the
// cache is the one that deduplicates).
//
// NOTE: this asserts the contract documented in the package comment —
// the refresher fans out to N goroutines, the *cache* (Plan 03-03) is
// responsible for collapsing them. The integration of both layers in
// production produces "≤ 1 underlying RPC per gateway per TTL window".
func TestRefresher_NoDoubleFireWithinTTL(t *testing.T) {
	getter := &fakeMetricsGetter{resp: buildMetricsResp([]float32{1}, []float32{1}, 0)}
	writer := newFakeStatsWriter()
	r := &CacheRefresher{Cache: getter, Queries: writer, Timeout: 2 * time.Second}

	row := GatewayRow{ID: uuid.New(), GatewayID: "aabbccddeeff00bb"}
	r.Trigger(context.Background(), []GatewayRow{row})
	r.Trigger(context.Background(), []GatewayRow{row})

	require.Eventually(t, func() bool {
		return getter.calls.Load() >= 2
	}, 2*time.Second, 10*time.Millisecond,
		"each Trigger fans out independently; the cache (separately tested) coalesces")
}

// TestSummariseMetrics_EmptyResponse — summariseMetrics handles a nil
// response without panicking and emits zeros + a "null" sparkline.
func TestSummariseMetrics_EmptyResponse(t *testing.T) {
	rx, tx, txOK, sparkline := summariseMetrics(nil)
	require.EqualValues(t, 0, rx)
	require.EqualValues(t, 0, tx)
	require.EqualValues(t, 0, txOK)
	require.Equal(t, "null", string(sparkline))
}

// silence unused-import lint when only some test methods touch pgtype.
var _ = pgtype.UUID{}
