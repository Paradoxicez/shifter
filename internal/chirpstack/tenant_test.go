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

// dialMockConnWithHandles wires a *grpc.ClientConn to the Plan 02-05 v4 mock
// and returns the handles so tests can pre-seed state and assert call counts.
func dialMockConnWithHandles(t *testing.T) (*grpc.ClientConn, *testsupport.ChirpStackMockHandles) {
	t.Helper()
	dial, _, h := testsupport.NewChirpStackMockBufWithHandles(t)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(dial),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn, h
}

// TestEnsureTenant_CreatesWhenAbsent — first call against a virgin mock
// triggers exactly one TenantService.Create. Returned UUID is non-empty.
// D-28 idempotent bootstrap.
func TestEnsureTenant_CreatesWhenAbsent(t *testing.T) {
	conn, h := dialMockConnWithHandles(t)
	cli := NewClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	id, err := cli.EnsureTenant(ctx, "shifter-default")
	require.NoError(t, err)
	require.NotEmpty(t, id, "expected non-empty tenant UUID")
	require.Equal(t, int64(1), h.Tenant.CreateCalls(),
		"first EnsureTenant call must invoke TenantService.Create exactly once")
}

// TestEnsureTenant_ReusesWhenPresent — pre-seeding the mock with a tenant of
// the target name short-circuits the create call: returned UUID matches the
// seeded one and the mock records ZERO Create calls (only List).
func TestEnsureTenant_ReusesWhenPresent(t *testing.T) {
	conn, h := dialMockConnWithHandles(t)
	cli := NewClient(conn)

	seededID := h.Tenant.SeedTenant("shifter-default")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	id, err := cli.EnsureTenant(ctx, "shifter-default")
	require.NoError(t, err)
	require.Equal(t, seededID, id, "EnsureTenant must reuse pre-seeded tenant")
	require.Equal(t, int64(0), h.Tenant.CreateCalls(),
		"reuse path must NOT invoke TenantService.Create")
	require.GreaterOrEqual(t, h.Tenant.ListCalls(), int64(1),
		"reuse path must call TenantService.List at least once")
}
