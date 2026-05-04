package chirpstack

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestEnsureApplication_CreatesWhenAbsent — first call against a virgin mock
// triggers exactly one ApplicationService.Create scoped to the given tenant.
func TestEnsureApplication_CreatesWhenAbsent(t *testing.T) {
	conn, h := dialMockConnWithHandles(t)
	cli := NewClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tenantID := h.Tenant.SeedTenant("shifter-default")

	id, err := cli.EnsureApplication(ctx, tenantID, "shifter")
	require.NoError(t, err)
	require.NotEmpty(t, id, "expected non-empty application UUID")
	require.Equal(t, int64(1), h.Application.CreateCalls(),
		"first EnsureApplication call must invoke ApplicationService.Create exactly once")
}

// TestEnsureApplication_ReusesWhenPresent — pre-seeded application is reused.
func TestEnsureApplication_ReusesWhenPresent(t *testing.T) {
	conn, h := dialMockConnWithHandles(t)
	cli := NewClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tenantID := h.Tenant.SeedTenant("shifter-default")
	seededAppID := h.Application.SeedApplication(tenantID, "shifter")

	id, err := cli.EnsureApplication(ctx, tenantID, "shifter")
	require.NoError(t, err)
	require.Equal(t, seededAppID, id,
		"EnsureApplication must reuse pre-seeded (tenant, name) application")
	require.Equal(t, int64(0), h.Application.CreateCalls(),
		"reuse path must NOT invoke ApplicationService.Create")
	require.GreaterOrEqual(t, h.Application.ListCalls(), int64(1),
		"reuse path must call ApplicationService.List at least once")
}

// TestEnsureApplication_TenantScoped — an application of the same name in a
// different tenant must NOT be reused; we must create a new one for our tenant.
func TestEnsureApplication_TenantScoped(t *testing.T) {
	conn, h := dialMockConnWithHandles(t)
	cli := NewClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	otherTenantID := h.Tenant.SeedTenant("other-tenant")
	_ = h.Application.SeedApplication(otherTenantID, "shifter")

	myTenantID := h.Tenant.SeedTenant("shifter-default")
	id, err := cli.EnsureApplication(ctx, myTenantID, "shifter")
	require.NoError(t, err)
	require.NotEmpty(t, id)
	require.Equal(t, int64(1), h.Application.CreateCalls(),
		"foreign-tenant collision must NOT short-circuit our tenant's create")
}
