package chirpstack

// Phase 3 Wave 1 — gateway metrics cache (TTL + singleflight).
//
// D-02: a list of 50+ gateways must not fan-out per-gateway GetMetrics RPCs
// on every refresh. The cache wraps the wrapper with:
//   - 30s TTL per gateway_id
//   - golang.org/x/sync/singleflight to coalesce concurrent refreshes
//
// Wave 1 implements internal/chirpstack/gateway_metrics_cache.go; these
// tests verify the caching contract independently of the underlying gRPC
// surface (which is covered by gateway_test.go).

import "testing"

// TestMetricsCache_TTL — a cache hit within TTL returns the stored value
// without invoking GetMetrics again; after TTL expiry the next call fetches.
func TestMetricsCache_TTL(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/chirpstack/gateway_metrics_cache.go (03-VALIDATION row gateway_metrics_cache_test.TestMetricsCache_TTL)")
}

// TestMetricsCache_SingleFlight — N concurrent calls for the same gateway_id
// within the in-flight window produce exactly 1 underlying GetMetrics RPC.
func TestMetricsCache_SingleFlight(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/chirpstack/gateway_metrics_cache.go (03-VALIDATION row gateway_metrics_cache_test.TestMetricsCache_SingleFlight)")
}
