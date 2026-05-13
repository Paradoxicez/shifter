package chirpstack

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shifter-io/shifter/internal/testsupport"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// dialMockConn is a small helper used by every chirpstack package test that
// needs a *grpc.ClientConn wired to the in-process bufconn mock.
func dialMockConn(t *testing.T, mode string) *grpc.ClientConn {
	t.Helper()
	dial, _ := testsupport.NewChirpStackMockBuf(t, mode)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(dial),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// TestProbeVersion_v4 — Against a v4 mock that responds to TenantService.List
// (the canonical v4 reachability probe), ProbeVersion returns the literal
// "v4" and nil error. We no longer return a precise version string because
// InternalService.GetVersion requires a user-session JWT in v4.10+; the
// global API key the operator provides is rejected by that RPC.
func TestProbeVersion_v4(t *testing.T) {
	conn := dialMockConn(t, "v4")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	v, err := ProbeVersion(ctx, conn)
	require.NoError(t, err)
	require.Equal(t, "v4", v)
}

// TestProbeVersion_v3 — Against a v3 mock that returns codes.Unimplemented
// for GetVersion, ProbeVersion returns the typed sentinel
// ErrChirpStackV3OrUnknown so the wizard / serve startup can refuse v3.
// Implementation: Plan 12 (chirpstack-grpc).
func TestProbeVersion_v3(t *testing.T) {
	conn := dialMockConn(t, "v3")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := ProbeVersion(ctx, conn)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrChirpStackV3OrUnknown),
		"INST-05: v3 must yield ErrChirpStackV3OrUnknown (got %v)", err)
}
