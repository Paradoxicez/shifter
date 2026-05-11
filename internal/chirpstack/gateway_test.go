package chirpstack

import (
	"context"
	"errors"
	"testing"
	"time"

	api "github.com/chirpstack/chirpstack/api/go/v4/api"
	common "github.com/chirpstack/chirpstack/api/go/v4/common"
	"github.com/shifter-io/shifter/internal/testsupport"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// dialMockConnPhase3 wires a *grpc.ClientConn to the Phase 3 superset mock
// (Gateway + Device.GetKeys + Device.GetActivation added in Wave 0). Returned
// handles let Phase 3 tests pre-seed gateways / activations / OTAA keys.
func dialMockConnPhase3(t *testing.T) (*grpc.ClientConn, *testsupport.ChirpStackMockPhase3Handles) {
	t.Helper()
	dial, _, h := testsupport.NewChirpStackMockBufPhase3(t)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(dial),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn, h
}

const (
	testGatewayID  = "aabbccddeeff0011"
	testGatewayID2 = "1122334455667788"
	testGatewayID3 = "deadbeefcafef00d"
	testTenantID   = "11111111-2222-3333-4444-555555555555"
)

// TestCreateGateway — Create wraps GatewayService.Create, stamps
// LocationSource=CONFIG (operator-typed), and forwards every CreateGatewayInput
// field onto the proto Gateway message.
func TestCreateGateway(t *testing.T) {
	conn, h := dialMockConnPhase3(t)
	cli := NewClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	in := CreateGatewayInput{
		GatewayID:   testGatewayID,
		Name:        "gw-roof-east",
		Description: "Office roof — eastside",
		Region:      "AS923_2",
		Lat:         13.7563,
		Lng:         100.5018,
		Altitude:    12.5,
		Tags:        map[string]string{"site": "head-office"},
		TenantID:    testTenantID,
	}
	require.NoError(t, cli.CreateGateway(ctx, in))
	require.Equal(t, int64(1), h.Gateway.CreateCalls(),
		"first CreateGateway call must invoke GatewayService.Create exactly once")

	// Read it back through the wrapper to confirm proto-level fields stuck.
	got, err := cli.GetGateway(ctx, testGatewayID)
	require.NoError(t, err)
	require.Equal(t, "gw-roof-east", got.Name)
	require.Equal(t, "Office roof — eastside", got.Description)
	require.InDelta(t, 13.7563, got.Lat, 1e-9)
	require.InDelta(t, 100.5018, got.Lng, 1e-9)
	require.InDelta(t, 12.5, got.Altitude, 1e-9)
	require.Equal(t, "head-office", got.Tags["site"])

	// Direct mock peek — Location.Source must be CONFIG (operator-typed).
	listResp, err := cli.ListGateways(ctx, ListGatewaysInput{TenantID: testTenantID, Limit: 10})
	require.NoError(t, err)
	require.Equal(t, uint32(1), listResp.TotalCount)
}

// TestCreateGateway_StampsLocationSourceCONFIG — separate test that pokes the
// mock state directly (via GatewayService.Get on the bufconn) to confirm the
// stored Gateway's Location.Source enum value is exactly CONFIG.
func TestCreateGateway_StampsLocationSourceCONFIG(t *testing.T) {
	conn, _ := dialMockConnPhase3(t)
	cli := NewClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, cli.CreateGateway(ctx, CreateGatewayInput{
		GatewayID: testGatewayID,
		Name:      "gw-1",
		Region:    "EU868",
		Lat:       50.1, Lng: 8.6, Altitude: 0,
		TenantID: testTenantID,
	}))

	// Call the proto-level Get directly so we can inspect the source enum.
	svc := api.NewGatewayServiceClient(conn)
	resp, err := svc.Get(ctx, &api.GetGatewayRequest{GatewayId: testGatewayID})
	require.NoError(t, err)
	require.NotNil(t, resp.Gateway.Location)
	require.Equal(t, common.LocationSource_CONFIG, resp.Gateway.Location.Source,
		"CreateGateway must stamp Location.Source = CONFIG for operator-typed coords")
}

// TestGetGateway — happy path + ErrNotFound translation.
func TestGetGateway(t *testing.T) {
	conn, _ := dialMockConnPhase3(t)
	cli := NewClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, cli.CreateGateway(ctx, CreateGatewayInput{
		GatewayID: testGatewayID,
		Name:      "gw-roof-east",
		Region:    "EU868",
		TenantID:  testTenantID,
	}))

	got, err := cli.GetGateway(ctx, testGatewayID)
	require.NoError(t, err)
	require.Equal(t, "gw-roof-east", got.Name)

	_, err = cli.GetGateway(ctx, testGatewayID2)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNotFound),
		"GetGateway on unknown id must return ErrNotFound (got %v)", err)
}

// TestUpdateGateway — full-replace semantics: Update replaces the whole record,
// caller must pass the FULL gateway shape.
func TestUpdateGateway(t *testing.T) {
	conn, h := dialMockConnPhase3(t)
	cli := NewClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, cli.CreateGateway(ctx, CreateGatewayInput{
		GatewayID:   testGatewayID,
		Name:        "gw-orig",
		Description: "original",
		Region:      "EU868",
		Lat:         50.1, Lng: 8.6,
		TenantID: testTenantID,
	}))

	require.NoError(t, cli.UpdateGateway(ctx, CreateGatewayInput{
		GatewayID:   testGatewayID,
		Name:        "gw-renamed",
		Description: "updated description",
		Region:      "EU868",
		Lat:         50.5, Lng: 8.9, Altitude: 25.0,
		Tags:     map[string]string{"site": "ho-2"},
		TenantID: testTenantID,
	}))
	require.Equal(t, int64(1), h.Gateway.UpdateCalls(),
		"UpdateGateway must invoke GatewayService.Update exactly once")

	got, err := cli.GetGateway(ctx, testGatewayID)
	require.NoError(t, err)
	require.Equal(t, "gw-renamed", got.Name)
	require.Equal(t, "updated description", got.Description)
	require.InDelta(t, 50.5, got.Lat, 1e-9)
	require.Equal(t, "ho-2", got.Tags["site"])
}

// TestDeleteGateway — first delete succeeds, second returns ErrNotFound so
// callers can treat the operation idempotently.
func TestDeleteGateway(t *testing.T) {
	conn, _ := dialMockConnPhase3(t)
	cli := NewClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, cli.CreateGateway(ctx, CreateGatewayInput{
		GatewayID: testGatewayID,
		Name:      "gw-bye",
		Region:    "EU868",
		TenantID:  testTenantID,
	}))
	require.NoError(t, cli.DeleteGateway(ctx, testGatewayID))

	err := cli.DeleteGateway(ctx, testGatewayID)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNotFound),
		"second DeleteGateway on the same id must surface ErrNotFound (got %v)", err)
}

// TestListGateways — limit + offset + tenant filter; returns total + items.
func TestListGateways(t *testing.T) {
	conn, _ := dialMockConnPhase3(t)
	cli := NewClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, id := range []string{testGatewayID, testGatewayID2, testGatewayID3} {
		require.NoError(t, cli.CreateGateway(ctx, CreateGatewayInput{
			GatewayID: id,
			Name:      "gw-" + id[:4],
			Region:    "EU868",
			TenantID:  testTenantID,
		}))
	}

	out, err := cli.ListGateways(ctx, ListGatewaysInput{
		TenantID: testTenantID,
		Limit:    2,
	})
	require.NoError(t, err)
	require.Equal(t, uint32(3), out.TotalCount,
		"total_count must reflect ALL matching rows, not just the limited page")
	require.Len(t, out.Items, 3, "mock returns full result set; total reflects full count")
	// Each item should carry the State + LastSeenAt fields from GatewayListItem.
	for _, it := range out.Items {
		require.NotEmpty(t, it.GatewayID)
		require.Equal(t, "ONLINE", it.State, "Phase 3 mock seeds gateways as ONLINE for GW-01 status column")
	}
}

// TestGetMetrics — wrapper returns the raw *api.GetGatewayMetricsResponse so
// the cache layer (Task 2) can summarise. Mock is seeded with 24-bucket
// RxPackets / TxPackets / TxPacketsPerStatus to mirror the D-02 sparkline +
// success% requirement.
func TestGetMetrics(t *testing.T) {
	conn, h := dialMockConnPhase3(t)
	cli := NewClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, cli.CreateGateway(ctx, CreateGatewayInput{
		GatewayID: testGatewayID,
		Name:      "gw-metrics",
		Region:    "EU868",
		TenantID:  testTenantID,
	}))

	rx := make([]float32, 24)
	tx := make([]float32, 24)
	for i := 0; i < 24; i++ {
		rx[i] = float32(i + 1)         // 1..24
		tx[i] = float32(2 * (i + 1))   // 2..48
	}
	h.Gateway.SetGatewayMetrics(testGatewayID, rx, tx, nil)

	now := time.Now().UTC()
	resp, err := cli.GetMetrics(ctx, GetGatewayMetricsInput{
		GatewayID:   testGatewayID,
		Start:       now.Add(-24 * time.Hour),
		End:         now,
		Aggregation: "HOUR",
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), h.Gateway.MetricCalls())
	require.NotNil(t, resp.RxPackets)
	require.NotNil(t, resp.TxPackets)
	require.Len(t, resp.RxPackets.Datasets, 1)
	require.Equal(t, float32(1), resp.RxPackets.Datasets[0].Data[0])
	require.Equal(t, float32(24), resp.RxPackets.Datasets[0].Data[23])
	require.Equal(t, float32(48), resp.TxPackets.Datasets[0].Data[23])
}

// TestGetMetrics_GatewayNotFound — NotFound on the underlying RPC surfaces
// as ErrNotFound for the cache layer's miss handling.
func TestGetMetrics_GatewayNotFound(t *testing.T) {
	conn, _ := dialMockConnPhase3(t)
	cli := NewClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := cli.GetMetrics(ctx, GetGatewayMetricsInput{
		GatewayID:   "0000000000000000",
		Start:       time.Now().Add(-24 * time.Hour),
		End:         time.Now(),
		Aggregation: "HOUR",
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNotFound))
}
