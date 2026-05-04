package chirpstack

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	api "github.com/chirpstack/chirpstack/api/go/v4/api"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// fakeStore is the in-memory ConnectionStore used by the bootstrap tests.
// Models the chirpstack_connection row: cs_tenant_id + cs_application_id
// columns nullable on first boot, non-NULL after a successful Set.
type fakeStore struct {
	mu              sync.Mutex
	tenantID        string
	applicationID   string
	getErr          error
	setErr          error
	setCalls        int
	lastSetTenant   string
	lastSetAppID    string
	getCalls        int
	settingDisabled bool // when true, SetCSTenantApp is a no-op (used to test failure paths)
}

func (s *fakeStore) GetCSConnection(_ context.Context) (string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getCalls++
	if s.getErr != nil {
		return "", "", s.getErr
	}
	return s.tenantID, s.applicationID, nil
}

func (s *fakeStore) SetCSTenantApp(_ context.Context, tenantID, applicationID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setCalls++
	s.lastSetTenant = tenantID
	s.lastSetAppID = applicationID
	if s.setErr != nil {
		return s.setErr
	}
	if s.settingDisabled {
		return nil
	}
	s.tenantID = tenantID
	s.applicationID = applicationID
	return nil
}

func nopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// TestBootstrap_FirstBoot — virgin store + virgin mock. Both Tenant + App
// are created exactly once; SetCSTenantApp is called once with the returned
// IDs.
func TestBootstrap_FirstBoot(t *testing.T) {
	conn, h := dialMockConnWithHandles(t)
	cli := NewClient(conn)
	store := &fakeStore{}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tID, aID, err := EnsureTenantAndApplication(ctx, cli, store, nopLogger())
	require.NoError(t, err)
	require.NotEmpty(t, tID)
	require.NotEmpty(t, aID)
	require.Equal(t, int64(1), h.Tenant.CreateCalls(), "tenant created once")
	require.Equal(t, int64(1), h.Application.CreateCalls(), "application created once")
	require.Equal(t, 1, store.setCalls, "SetCSTenantApp called exactly once")
	require.Equal(t, tID, store.lastSetTenant)
	require.Equal(t, aID, store.lastSetAppID)
}

// TestBootstrap_AlreadyBootstrapped — store pre-populated with both UUIDs;
// no CS RPCs fire and SetCSTenantApp is NOT called.
func TestBootstrap_AlreadyBootstrapped(t *testing.T) {
	conn, h := dialMockConnWithHandles(t)
	cli := NewClient(conn)
	store := &fakeStore{
		tenantID:      "tenant-uuid-X",
		applicationID: "app-uuid-Y",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tID, aID, err := EnsureTenantAndApplication(ctx, cli, store, nopLogger())
	require.NoError(t, err)
	require.Equal(t, "tenant-uuid-X", tID)
	require.Equal(t, "app-uuid-Y", aID)
	require.Equal(t, int64(0), h.Tenant.CreateCalls(),
		"steady-state boot must not invoke TenantService.Create")
	require.Equal(t, int64(0), h.Tenant.ListCalls(),
		"steady-state boot must not even List — short-circuit on persisted IDs")
	require.Equal(t, int64(0), h.Application.CreateCalls())
	require.Equal(t, 0, store.setCalls,
		"steady-state boot must NOT touch SetCSTenantApp")
}

// TestBootstrap_PartiallyBootstrapped — cs_tenant_id set, cs_application_id
// NULL. Tenant is reused (no Create), Application is created. SetCSTenantApp
// is called with (existing tenant, new app id).
func TestBootstrap_PartiallyBootstrapped(t *testing.T) {
	conn, h := dialMockConnWithHandles(t)
	cli := NewClient(conn)
	store := &fakeStore{
		tenantID:      "tenant-uuid-existing",
		applicationID: "",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tID, aID, err := EnsureTenantAndApplication(ctx, cli, store, nopLogger())
	require.NoError(t, err)
	require.Equal(t, "tenant-uuid-existing", tID,
		"existing cs_tenant_id must be preserved across partial bootstrap")
	require.NotEmpty(t, aID)
	require.NotEqual(t, "", aID)
	require.Equal(t, int64(0), h.Tenant.CreateCalls(),
		"existing tenant must not be re-created")
	require.Equal(t, int64(1), h.Application.CreateCalls(),
		"missing application must be created exactly once")
	require.Equal(t, 1, store.setCalls)
	require.Equal(t, "tenant-uuid-existing", store.lastSetTenant)
	require.Equal(t, aID, store.lastSetAppID)
}

// TestBootstrap_TenantCreateFails — when CS Tenant.Create returns an error,
// EnsureTenantAndApplication returns a wrapped error with "ensure tenant"
// in the message and SetCSTenantApp is NOT called (partial state never
// persisted).
func TestBootstrap_TenantCreateFails(t *testing.T) {
	// Build a one-off mock where Tenant.Create returns codes.Unavailable.
	conn := dialFailingTenantConn(t)
	cli := NewClient(conn)
	store := &fakeStore{}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, _, err := EnsureTenantAndApplication(ctx, cli, store, nopLogger())
	require.Error(t, err)
	require.Contains(t, err.Error(), "ensure tenant")
	require.Equal(t, 0, store.setCalls,
		"partial-bootstrap state must NEVER be persisted on tenant-create failure")
}

// TestBootstrap_NilLogger — nil logger is tolerated (slog.Default() fallback).
// Operationally important: callers shouldn't have to construct a logger just
// to call the bootstrap.
func TestBootstrap_NilLogger(t *testing.T) {
	conn, _ := dialMockConnWithHandles(t)
	cli := NewClient(conn)
	store := &fakeStore{}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, _, err := EnsureTenantAndApplication(ctx, cli, store, nil)
	require.NoError(t, err)
}

// TestBootstrap_StoreReadFails — a read failure surfaces as "read
// chirpstack_connection: ..." and no CS RPCs fire.
func TestBootstrap_StoreReadFails(t *testing.T) {
	conn, h := dialMockConnWithHandles(t)
	cli := NewClient(conn)
	store := &fakeStore{getErr: errors.New("db down")}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, _, err := EnsureTenantAndApplication(ctx, cli, store, nopLogger())
	require.Error(t, err)
	require.Contains(t, err.Error(), "read chirpstack_connection")
	require.Equal(t, int64(0), h.Tenant.ListCalls(),
		"failed store read must short-circuit before any CS RPC")
}

// dialFailingTenantConn registers a failing TenantService (every method
// returns codes.Unavailable) on a bufconn-backed gRPC server. Used only by
// TestBootstrap_TenantCreateFails — adding a "fail mode" flag to the shared
// mock would force every other test to thread it through, so we inline this
// purpose-built fake.
func dialFailingTenantConn(t *testing.T) *grpc.ClientConn {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	api.RegisterTenantServiceServer(srv, failingTenantSvc{})
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(func() {
		srv.Stop()
		_ = lis.Close()
	})

	dial := func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(dial),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

type failingTenantSvc struct {
	api.UnimplementedTenantServiceServer
}

func (failingTenantSvc) List(_ context.Context, _ *api.ListTenantsRequest) (*api.ListTenantsResponse, error) {
	return nil, status.Error(codes.Unavailable, "simulated CS tenant.List unavailable")
}

func (failingTenantSvc) Create(_ context.Context, _ *api.CreateTenantRequest) (*api.CreateTenantResponse, error) {
	return nil, status.Error(codes.Unavailable, "simulated CS tenant.Create unavailable")
}
