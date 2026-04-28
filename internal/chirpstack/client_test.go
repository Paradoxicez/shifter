package chirpstack

import (
	"context"
	"testing"
	"time"

	"github.com/shifter-io/shifter/internal/testsupport"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TestClient_ListDevices_Mock — CHIRP-01 smoke: against the v4 mock the
// high-level Client.PingDevices wrapper succeeds (returns nil) — proving the
// gRPC channel + Bearer-token interceptor + DeviceServiceClient end-to-end.
// Implementation: Plan 12 (chirpstack-grpc).
func TestClient_ListDevices_Mock(t *testing.T) {
	dial, _ := testsupport.NewChirpStackMockBuf(t, "v4")
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(dial),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	cli := NewClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, cli.PingDevices(ctx, ""),
		"CHIRP-01: gRPC client must list devices via mock")
}
