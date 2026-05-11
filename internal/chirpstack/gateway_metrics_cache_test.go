package chirpstack

// Phase 3 Plan 03-03 — gateway metrics cache (TTL + singleflight).
//
// D-02: a list of 50+ gateways must not fan-out per-gateway GetMetrics RPCs
// on every refresh. The cache wraps the wrapper with:
//   - 1-minute TTL per gateway_id
//   - golang.org/x/sync/singleflight to coalesce concurrent refreshes

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestMetricsCache_TTL — hit-within-TTL must not re-invoke the underlying
// GetMetrics RPC; the next call after TTL expiry does.
func TestMetricsCache_TTL(t *testing.T) {
	conn, h := dialMockConnPhase3(t)
	cli := NewClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Seed a gateway so GetMetrics doesn't NotFound.
	require.NoError(t, cli.CreateGateway(ctx, CreateGatewayInput{
		GatewayID: testGatewayID,
		Name:      "gw-cache",
		Region:    "EU868",
		TenantID:  testTenantID,
	}))

	// Controllable clock — start at a known instant.
	fakeNow := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	cache := NewMetricsCache(cli)
	cache.now = func() time.Time { return fakeNow }

	// t = 0 → cache miss → 1 underlying call.
	_, err := cache.Get(ctx, testGatewayID)
	require.NoError(t, err)
	require.Equal(t, int64(1), h.Gateway.MetricCalls())

	// t = +30s → cache hit → still 1 underlying call.
	fakeNow = fakeNow.Add(30 * time.Second)
	_, err = cache.Get(ctx, testGatewayID)
	require.NoError(t, err)
	require.Equal(t, int64(1), h.Gateway.MetricCalls(),
		"call within TTL must not trigger another GetMetrics RPC")

	// t = +61s → cache stale → 2nd underlying call.
	fakeNow = fakeNow.Add(31 * time.Second) // total +61s past initial
	_, err = cache.Get(ctx, testGatewayID)
	require.NoError(t, err)
	require.Equal(t, int64(2), h.Gateway.MetricCalls(),
		"call past TTL must trigger a fresh GetMetrics RPC")
}

// TestMetricsCache_SingleFlight — N concurrent calls for the same gateway_id
// produce EXACTLY ONE underlying GetMetrics RPC (singleflight coalescing).
func TestMetricsCache_SingleFlight(t *testing.T) {
	conn, h := dialMockConnPhase3(t)
	cli := NewClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, cli.CreateGateway(ctx, CreateGatewayInput{
		GatewayID: testGatewayID,
		Name:      "gw-sf",
		Region:    "EU868",
		TenantID:  testTenantID,
	}))

	cache := NewMetricsCache(cli)
	// Fixed clock so TTL window doesn't elapse during the goroutine fan-out.
	fixed := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	cache.now = func() time.Time { return fixed }

	const N = 10
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			<-start
			_, err := cache.Get(ctx, testGatewayID)
			require.NoError(t, err)
		}()
	}
	close(start)
	wg.Wait()

	require.Equal(t, int64(1), h.Gateway.MetricCalls(),
		"%d concurrent Get calls for the same gateway must collapse to exactly 1 underlying RPC", N)
}

// TestMetricsCache_PerGatewayKey — different gatewayIDs are NOT coalesced;
// each gets its own RPC. Bug guard: singleflight key must include gatewayID.
func TestMetricsCache_PerGatewayKey(t *testing.T) {
	conn, h := dialMockConnPhase3(t)
	cli := NewClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, id := range []string{testGatewayID, testGatewayID2} {
		require.NoError(t, cli.CreateGateway(ctx, CreateGatewayInput{
			GatewayID: id,
			Name:      "gw-" + id[:4],
			Region:    "EU868",
			TenantID:  testTenantID,
		}))
	}

	cache := NewMetricsCache(cli)
	_, err := cache.Get(ctx, testGatewayID)
	require.NoError(t, err)
	_, err = cache.Get(ctx, testGatewayID2)
	require.NoError(t, err)

	require.Equal(t, int64(2), h.Gateway.MetricCalls(),
		"two distinct gatewayIDs must produce two separate RPCs (singleflight key must include gatewayID)")
}

// TestMetricsCache_Invalidate — manual Invalidate clears the entry so the
// next Get bypasses TTL and fetches fresh. Used by the decommission flow.
func TestMetricsCache_Invalidate(t *testing.T) {
	conn, h := dialMockConnPhase3(t)
	cli := NewClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, cli.CreateGateway(ctx, CreateGatewayInput{
		GatewayID: testGatewayID,
		Name:      "gw-inv",
		Region:    "EU868",
		TenantID:  testTenantID,
	}))

	cache := NewMetricsCache(cli)
	fixed := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	cache.now = func() time.Time { return fixed }

	_, err := cache.Get(ctx, testGatewayID)
	require.NoError(t, err)
	require.Equal(t, int64(1), h.Gateway.MetricCalls())

	// Without invalidation, within TTL → cache hit, no new RPC.
	_, err = cache.Get(ctx, testGatewayID)
	require.NoError(t, err)
	require.Equal(t, int64(1), h.Gateway.MetricCalls())

	// Invalidate forces refresh on next Get.
	cache.Invalidate(testGatewayID)
	_, err = cache.Get(ctx, testGatewayID)
	require.NoError(t, err)
	require.Equal(t, int64(2), h.Gateway.MetricCalls(),
		"Invalidate must clear the entry so next Get fetches fresh")
}
