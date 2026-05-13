package gateway

// Phase 3 Plan 03-04 Task 3 — async per-row gateway metrics refresher.
//
// The list handler returns the response WITHOUT blocking on per-row
// GetMetrics RPCs. Instead it kicks off this fan-out goroutine which
// refreshes the chirpstack.MetricsCache entries lazily — the result lands
// in PG (UpdateGatewayStatsCache) so the NEXT list-page render reads it
// from the gateway row directly.
//
// Single-flight + TTL semantics live in chirpstack.MetricsCache (Plan
// 03-03). The refresher's job is fan-out + persistence:
//
//   - For each requested gatewayID, run cache.Get in its own goroutine
//     under a fresh time-bounded context (caller may have already
//     returned).
//   - On success, summarise rx / tx / tx_ok totals from the 24-bucket
//     HOUR aggregation and write them back into the gateway row via
//     sqlc.UpdateGatewayStatsCache.
//
// The cache's singleflight collapses concurrent triggers for the same
// gatewayID into a single underlying RPC, so two refresh waves landing
// within a TTL produce at most one CS call per gateway.

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	api "github.com/chirpstack/chirpstack/api/go/v4/api"
	common "github.com/chirpstack/chirpstack/api/go/v4/common"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/shifter-io/shifter/internal/chirpstack"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// MetricsCacheGetter is the narrow interface the refresher consumes —
// *chirpstack.MetricsCache satisfies it.
type MetricsCacheGetter interface {
	Get(ctx context.Context, gatewayID string) (*api.GetGatewayMetricsResponse, error)
}

// GatewayInfoGetter exposes the single chirpstack.Client method the refresher
// needs to refresh last_seen_at. *chirpstack.Client satisfies it.
type GatewayInfoGetter interface {
	GetGateway(ctx context.Context, gatewayID string) (*chirpstack.Gateway, error)
}

// StatsWriter is the narrow sqlc subset the refresher needs. Tests can
// swap in a fake. *sqlc.Queries satisfies it.
type StatsWriter interface {
	UpdateGatewayStatsCache(ctx context.Context, arg sqlc.UpdateGatewayStatsCacheParams) error
}

// CacheRefresher fan-out for refreshing gateway stats. Called from the
// list handler with the set of gatewayIDs visible in the current page.
// The refresh is fire-and-forget — the list response returns immediately
// with currently-cached stats; the refresh updates the next request.
//
// Lazy + single-flight + TTL pattern (RESEARCH §GetMetrics Caching).
type CacheRefresher struct {
	Cache   MetricsCacheGetter
	CSInfo  GatewayInfoGetter // optional; nil = skip last_seen_at refresh
	Queries StatsWriter
	Log     *slog.Logger

	// Timeout is the per-goroutine context deadline. Defaults to 10s; tests
	// can shorten to keep the suite fast.
	Timeout time.Duration

	// done is closed by tests to wait for in-flight refreshOne goroutines
	// without sleeping. Optional — production callers leave it nil.
	done chan struct{}
}

// Trigger spawns refresh goroutines for each provided gatewayID. Returns
// immediately. The underlying singleflight inside MetricsCache ensures
// concurrent triggers for the same ID coalesce into one fetch.
//
// ctx is intentionally NOT propagated to the goroutines — the caller's
// ctx is typically the HTTP request ctx and the response has already been
// written by the time Trigger returns. Each refresh uses its OWN fresh
// background context with a Timeout deadline.
func (r *CacheRefresher) Trigger(_ context.Context, gateways []GatewayRow) {
	for _, gw := range gateways {
		gw := gw // capture for goroutine
		go r.refreshOne(gw)
	}
}

func (r *CacheRefresher) refreshOne(gw GatewayRow) {
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resp, err := r.Cache.Get(ctx, gw.GatewayID)
	if err != nil {
		if r.Log != nil {
			r.Log.Warn("gateway metrics refresh failed",
				"gateway_id", gw.GatewayID, "err", err)
		}
		return
	}
	rx, tx, txOK, sparkline := summariseMetrics(resp)
	if r.Queries == nil {
		return
	}
	// Best-effort fetch of LastSeenAt from CS. Optional dep — older test
	// harnesses construct CacheRefresher without CSInfo and we MUST NOT
	// regress those by hard-requiring it.
	var lastSeen pgtype.Timestamptz
	if r.CSInfo != nil {
		if cs, err := r.CSInfo.GetGateway(ctx, gw.GatewayID); err == nil && cs != nil && cs.LastSeenAt != nil {
			lastSeen = pgtype.Timestamptz{Time: *cs.LastSeenAt, Valid: true}
		}
	}
	if err := r.Queries.UpdateGatewayStatsCache(ctx, sqlc.UpdateGatewayStatsCacheParams{
		ID:             pgtype.UUID{Bytes: gw.ID, Valid: true},
		StatsRx24h:     &rx,
		StatsTx24h:     &tx,
		StatsTxOk24h:   &txOK,
		StatsSparkline: sparkline,
		LastSeenAt:     lastSeen,
	}); err != nil {
		if r.Log != nil {
			r.Log.Warn("gateway stats cache write failed",
				"gateway_id", gw.GatewayID, "err", err)
		}
	}
}

// summariseMetrics extracts the canonical 24h totals + sparkline JSON
// from a GetGatewayMetricsResponse.
//
//   - rx       = sum(resp.RxPackets.Datasets[0].Data)            — 24 buckets
//   - tx       = sum(resp.TxPackets.Datasets[0].Data)            — 24 buckets
//   - txOK     = sum(resp.TxPacketsPerStatus.Datasets[i==OK]...) — when present
//   - sparkline JSON: {rx:[24], tx:[24]} for the gateway list-page sparkline
//
// Missing series are treated as zero — partial vendor data shouldn't
// poison the stats columns.
func summariseMetrics(resp *api.GetGatewayMetricsResponse) (rx, tx, txOK int64, sparkline []byte) {
	if resp == nil {
		return 0, 0, 0, []byte("null")
	}
	rxBuckets := firstDatasetData(resp.GetRxPackets())
	txBuckets := firstDatasetData(resp.GetTxPackets())
	rx = sumFloats(rxBuckets)
	tx = sumFloats(txBuckets)

	if status := resp.GetTxPacketsPerStatus(); status != nil {
		for _, ds := range status.GetDatasets() {
			if ds.GetLabel() == "OK" {
				txOK = sumFloats(ds.GetData())
				break
			}
		}
	}

	// Sparkline JSON keeps the per-hour buckets so the UI can render a
	// 24-point chart without recomputing.
	js, err := json.Marshal(map[string]any{
		"rx": float32SliceToInt64(rxBuckets),
		"tx": float32SliceToInt64(txBuckets),
	})
	if err != nil {
		return rx, tx, txOK, nil
	}
	return rx, tx, txOK, js
}

// firstDatasetData returns the data array of the first dataset of m, or
// nil if absent. Real CS responses populate Datasets[0] with the canonical
// series; vendor-overridden mock responses do the same.
func firstDatasetData(m *common.Metric) []float32 {
	if m == nil {
		return nil
	}
	for _, ds := range m.GetDatasets() {
		return ds.GetData()
	}
	return nil
}

func sumFloats(vals []float32) int64 {
	var sum float64
	for _, v := range vals {
		sum += float64(v)
	}
	return int64(sum)
}

func float32SliceToInt64(vals []float32) []int64 {
	out := make([]int64, len(vals))
	for i, v := range vals {
		out[i] = int64(v)
	}
	return out
}
