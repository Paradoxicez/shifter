package doctor

import (
	"context"
	"log/slog"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	api "github.com/chirpstack/chirpstack/api/go/v4/api"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// ---- gRPC stub server helpers ----

type fakeInternalServer struct {
	api.UnimplementedInternalServiceServer
	versionString string
	returnCode    codes.Code
}

func (f *fakeInternalServer) GetVersion(_ context.Context, _ *emptypb.Empty) (*api.GetVersionResponse, error) {
	if f.returnCode != codes.OK {
		return nil, status.Errorf(f.returnCode, "stub error")
	}
	return &api.GetVersionResponse{Version: f.versionString}, nil
}

func startFakeCS(t *testing.T, versionStr string) string {
	t.Helper()
	return startFakeCSWithCode(t, versionStr, codes.OK)
}

func startFakeCSWithCode(t *testing.T, versionStr string, code codes.Code) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := grpc.NewServer()
	api.RegisterInternalServiceServer(srv, &fakeInternalServer{versionString: versionStr, returnCode: code})
	go srv.Serve(lis) //nolint:errcheck
	t.Cleanup(srv.Stop)
	return lis.Addr().String()
}

// ---- ProbeChirpStack tests ----

// TestProbeChirpStack_OK_V410 — stub returns v4.10.0; probe returns status="ok".
func TestProbeChirpStack_OK_V410(t *testing.T) {
	addr := startFakeCS(t, "4.10.0")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := ProbeChirpStack(ctx, addr, "dummy-key")
	require.Equal(t, "ok", result.Status)
	require.Contains(t, result.Message, "4.10")
}

// TestProbeChirpStack_Warn_V48 — stub returns v4.8.0; probe returns status="warn".
func TestProbeChirpStack_Warn_V48(t *testing.T) {
	addr := startFakeCS(t, "4.8.0")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := ProbeChirpStack(ctx, addr, "dummy-key")
	require.Equal(t, "warn", result.Status)
	require.Contains(t, result.Message, "4.10+")
}

// TestProbeChirpStack_Error_V3 — stub returns v3.10.0; probe returns status="error".
func TestProbeChirpStack_Error_V3(t *testing.T) {
	addr := startFakeCS(t, "3.10.0")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result := ProbeChirpStack(ctx, addr, "dummy-key")
	require.Equal(t, "error", result.Status)
	require.Contains(t, result.Message, "v3")
}

// TestProbeTimescale_OK — real TimescaleDB testcontainer; probe returns status="ok".
func TestProbeTimescale_OK(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))

	result := ProbeTimescale(ctx, pool)
	require.Equal(t, "ok", result.Status)
	require.Contains(t, result.Message, "timescaledb")
}

// TestProbeRegion_Mismatch — install region AS923-2, gateway region AS923-1 → status="warn".
func TestProbeRegion_Mismatch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	pool := testsupport.StartPostgres(t)
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))

	// Upsert chirpstack_connection with region_name "AS923-2".
	_, err := pool.Exec(ctx, `
		INSERT INTO chirpstack_connection
		  (id, mode, grpc_url, api_token_ref, mqtt_url, region_name, region_common_name)
		VALUES (1, 'bundled', 'localhost:8080', 'ref', 'mqtt://localhost', 'AS923-2', 'AS923-2')
		ON CONFLICT (id) DO UPDATE
		  SET region_name = 'AS923-2', region_common_name = 'AS923-2'`)
	require.NoError(t, err)

	// Seed a gateway with region AS923-1 (different from install AS923-2).
	_, err = pool.Exec(ctx, `
		INSERT INTO gateway (gateway_id, name, description, region, lat, lng, altitude, tags, cs_tenant_id)
		VALUES ('0102030405060708', 'gw1', '', 'AS923-1', 1.0, 2.0, 0, '{}', 'tenant1')
		ON CONFLICT DO NOTHING`)
	require.NoError(t, err)

	result := ProbeRegion(ctx, pool)
	require.Equal(t, "warn", result.Status)
}
