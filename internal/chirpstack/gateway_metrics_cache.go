package chirpstack

// Phase 3 Plan 03-03 — gateway metrics cache.
//
// CONTEXT D-02 requires a 1-minute TTL on the per-gateway metrics that drive
// the gateway list page (24h RX/TX sparkline + success%). Naïve per-row
// fan-out (50+ gateways × per-render GetMetrics RPC) is wasteful and
// rate-limit-risky against larger CS deployments.
//
// Strategy:
//   1. In-memory map keyed by gateway_id, holding the most recent
//      *api.GetGatewayMetricsResponse + the wall-clock time it was refreshed.
//   2. Get(ctx, gatewayID) returns the cached response when (now - refreshed)
//      < ttl; otherwise fetches via Client.GetMetrics, stores, and returns.
//   3. Concurrent Get calls for the same gatewayID coalesce via
//      golang.org/x/sync/singleflight — N concurrent list refreshes produce
//      exactly ONE underlying RPC per gateway (T-3-23 mitigation).
//   4. Invalidate(gatewayID) drops the entry so decommission / restore flows
//      can force a fresh fetch on the next list render.

import (
	"context"
	"sync"
	"time"

	api "github.com/chirpstack/chirpstack/api/go/v4/api"
	"golang.org/x/sync/singleflight"
)

// defaultMetricsTTL is the freshness window per CONTEXT D-02 (1 minute).
// Exported as a package-level const so tests + future tuning can reference
// the canonical value rather than the inline literal.
const defaultMetricsTTL = 1 * time.Minute

// metricsLookbackWindow is how far back GetMetrics looks (24h) — produces
// 24 buckets at HOUR aggregation, matching the D-02 sparkline shape.
const metricsLookbackWindow = 24 * time.Hour

// cachedMetric is one map entry: the response + when it was last refreshed.
type cachedMetric struct {
	resp      *api.GetGatewayMetricsResponse
	refreshed time.Time
}

// MetricsCache wraps Client.GetMetrics with 1-minute TTL + singleflight
// deduplication. Safe for concurrent use; a single MetricsCache instance is
// the intended pattern (one per Shifter process, attached to the gateway
// handler).
type MetricsCache struct {
	client *Client
	ttl    time.Duration

	mu    sync.RWMutex
	cache map[string]cachedMetric // key: gatewayID (case-sensitive — caller normalises)

	sf singleflight.Group // dedupes concurrent fetches per gatewayID

	// now is the clock source — injectable so unit tests can advance time
	// deterministically without sleeping. Production callers leave it as
	// time.Now (set by NewMetricsCache).
	now func() time.Time
}

// NewMetricsCache constructs a fresh cache attached to the given Client.
// TTL is fixed at defaultMetricsTTL (1 minute); change the constant if the
// D-02 contract is ever revised.
func NewMetricsCache(client *Client) *MetricsCache {
	return &MetricsCache{
		client: client,
		ttl:    defaultMetricsTTL,
		cache:  make(map[string]cachedMetric),
		now:    time.Now,
	}
}

// Get returns the most recent metrics for gatewayID. Within the TTL window
// the call resolves from the cache without an RPC; outside the window the
// fetch is deduplicated across concurrent callers via singleflight so a
// thundering herd of N goroutines produces a single underlying RPC.
//
// The returned *api.GetGatewayMetricsResponse is shared across all callers
// for the lifetime of the cache entry — callers MUST treat it as read-only.
func (m *MetricsCache) Get(ctx context.Context, gatewayID string) (*api.GetGatewayMetricsResponse, error) {
	// Fast path: read-lock + freshness check.
	m.mu.RLock()
	entry, ok := m.cache[gatewayID]
	m.mu.RUnlock()
	if ok && m.now().Sub(entry.refreshed) < m.ttl {
		return entry.resp, nil
	}

	// Slow path: single-flight the fetch keyed by gatewayID. All concurrent
	// Get calls for the same gatewayID share one underlying invocation.
	v, err, _ := m.sf.Do(gatewayID, func() (any, error) {
		// Re-check inside the singleflight in case another goroutine just
		// populated the entry between our RLock read and Do's dispatch.
		m.mu.RLock()
		if entry, ok := m.cache[gatewayID]; ok && m.now().Sub(entry.refreshed) < m.ttl {
			m.mu.RUnlock()
			return entry.resp, nil
		}
		m.mu.RUnlock()

		now := m.now()
		resp, err := m.client.GetMetrics(ctx, GetGatewayMetricsInput{
			GatewayID:   gatewayID,
			Start:       now.Add(-metricsLookbackWindow),
			End:         now,
			Aggregation: "HOUR",
		})
		if err != nil {
			return nil, err
		}
		m.mu.Lock()
		m.cache[gatewayID] = cachedMetric{resp: resp, refreshed: now}
		m.mu.Unlock()
		return resp, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*api.GetGatewayMetricsResponse), nil
}

// Invalidate removes the cache entry for gatewayID. Called by the
// decommission / restore handlers (D-30, D-32) so the list page never
// displays metrics for a gateway that no longer exists.
func (m *MetricsCache) Invalidate(gatewayID string) {
	m.mu.Lock()
	delete(m.cache, gatewayID)
	m.mu.Unlock()
}
