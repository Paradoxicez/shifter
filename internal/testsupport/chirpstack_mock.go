package testsupport

import (
	"context"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	api "github.com/chirpstack/chirpstack/api/go/v4/api"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// bufconnSize is the buffered listener's in-memory pipe capacity. 1 MiB is the
// canonical default in google.golang.org/grpc/test/bufconn examples and
// comfortably exceeds the largest Phase 1 RPC payload (a single ListDevices
// page response is well under that).
const bufconnSize = 1024 * 1024

// fakeInternalV4 implements ChirpStack v4's InternalService.GetVersion and
// returns the embedded version string. Used by mode "v4".
type fakeInternalV4 struct {
	api.UnimplementedInternalServiceServer
	version string
}

func (f *fakeInternalV4) GetVersion(_ context.Context, _ *emptypb.Empty) (*api.GetVersionResponse, error) {
	return &api.GetVersionResponse{Version: f.version}, nil
}

// fakeInternalV3 simulates a ChirpStack v3 server: GetVersion is not
// implemented, so the server returns codes.Unimplemented (the canonical
// signature per RESEARCH §Pattern 4 / PITFALLS §7).
//
// We deliberately register an InternalService server that overrides the
// embedded UnimplementedInternalServiceServer's GetVersion to return an
// EXPLICIT codes.Unimplemented status. Returning the embedded
// Unimplemented*Server's default ALSO yields codes.Unimplemented, but the
// explicit form documents the intent and survives any future grpc-go change
// to the default (e.g. switching to codes.Unknown).
type fakeInternalV3 struct {
	api.UnimplementedInternalServiceServer
}

func (v *fakeInternalV3) GetVersion(_ context.Context, _ *emptypb.Empty) (*api.GetVersionResponse, error) {
	return nil, status.Error(codes.Unimplemented, "method GetVersion not implemented (simulated v3)")
}

// fakeDevice implements the minimum DeviceService surface needed by Plan 12's
// CHIRP-01 smoke (Client.PingDevices) and is reused by Plans 17/18.
//
// Plan 02-05 extends this with full Create/Get/Delete/CreateKeys so the
// EnsureTenantAndApplication bootstrap + CreateDevice + CreateDeviceKeys
// wrappers can be verified end-to-end without a live ChirpStack.
type fakeDevice struct {
	api.UnimplementedDeviceServiceServer
	mu          sync.Mutex
	devices     map[string]*api.Device     // keyed by DevEUI
	keys        map[string]*api.DeviceKeys // keyed by DevEUI
	createCalls atomic.Int64
	keysCalls   atomic.Int64
	deleteCalls atomic.Int64
}

func newFakeDevice() *fakeDevice {
	return &fakeDevice{
		devices: map[string]*api.Device{},
		keys:    map[string]*api.DeviceKeys{},
	}
}

func (f *fakeDevice) List(_ context.Context, _ *api.ListDevicesRequest) (*api.ListDevicesResponse, error) {
	return &api.ListDevicesResponse{TotalCount: 0, Result: nil}, nil
}

func (f *fakeDevice) Create(_ context.Context, req *api.CreateDeviceRequest) (*emptypb.Empty, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCalls.Add(1)
	if req.GetDevice() == nil {
		return nil, status.Error(codes.InvalidArgument, "device is required")
	}
	dev := req.GetDevice()
	if dev.DevEui == "" {
		return nil, status.Error(codes.InvalidArgument, "dev_eui is required")
	}
	if _, exists := f.devices[dev.DevEui]; exists {
		return nil, status.Error(codes.AlreadyExists, "device already exists")
	}
	// Defensive copy so test mutations of the returned proto do not bleed back.
	stored := &api.Device{
		DevEui:          dev.DevEui,
		Name:            dev.Name,
		Description:     dev.Description,
		ApplicationId:   dev.ApplicationId,
		DeviceProfileId: dev.DeviceProfileId,
		SkipFcntCheck:   dev.SkipFcntCheck,
		IsDisabled:      dev.IsDisabled,
		JoinEui:         dev.JoinEui,
	}
	f.devices[dev.DevEui] = stored
	return &emptypb.Empty{}, nil
}

func (f *fakeDevice) Get(_ context.Context, req *api.GetDeviceRequest) (*api.GetDeviceResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	dev, ok := f.devices[req.GetDevEui()]
	if !ok {
		return nil, status.Error(codes.NotFound, "device not found")
	}
	now := timestamppb.Now()
	return &api.GetDeviceResponse{Device: dev, CreatedAt: now, UpdatedAt: now}, nil
}

func (f *fakeDevice) Delete(_ context.Context, req *api.DeleteDeviceRequest) (*emptypb.Empty, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteCalls.Add(1)
	if _, ok := f.devices[req.GetDevEui()]; !ok {
		return nil, status.Error(codes.NotFound, "device not found")
	}
	delete(f.devices, req.GetDevEui())
	delete(f.keys, req.GetDevEui())
	return &emptypb.Empty{}, nil
}

// CreateCalls / KeysCalls / DeleteCalls expose atomic counters so cross-package
// tests in internal/chirpstack/ can assert RPC volumes without touching
// unexported fields.
func (f *fakeDevice) CreateCalls() int64 { return f.createCalls.Load() }
func (f *fakeDevice) KeysCalls() int64   { return f.keysCalls.Load() }
func (f *fakeDevice) DeleteCalls() int64 { return f.deleteCalls.Load() }

func (f *fakeDevice) CreateKeys(_ context.Context, req *api.CreateDeviceKeysRequest) (*emptypb.Empty, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.keysCalls.Add(1)
	keys := req.GetDeviceKeys()
	if keys == nil || keys.DevEui == "" {
		return nil, status.Error(codes.InvalidArgument, "device_keys.dev_eui is required")
	}
	if _, ok := f.devices[keys.DevEui]; !ok {
		return nil, status.Error(codes.NotFound, "device not found for keys")
	}
	if _, exists := f.keys[keys.DevEui]; exists {
		return nil, status.Error(codes.AlreadyExists, "keys already exist")
	}
	f.keys[keys.DevEui] = &api.DeviceKeys{
		DevEui:    keys.DevEui,
		NwkKey:    keys.NwkKey,
		AppKey:    keys.AppKey,
		GenAppKey: keys.GenAppKey,
	}
	return &emptypb.Empty{}, nil
}

// fakeTenant implements TenantService for EnsureTenant idempotency tests.
//
// List honours Search by substring match on Name (matches CS v4 behaviour
// closely enough for our tests; CS itself uses a Postgres ILIKE which is also
// substring-y). Create generates a deterministic-ish UUID and stores by name.
type fakeTenant struct {
	api.UnimplementedTenantServiceServer
	mu          sync.Mutex
	tenants     map[string]*api.Tenant // keyed by Tenant.Id
	createCalls atomic.Int64
	listCalls   atomic.Int64
}

func newFakeTenant() *fakeTenant {
	return &fakeTenant{tenants: map[string]*api.Tenant{}}
}

func (f *fakeTenant) List(_ context.Context, req *api.ListTenantsRequest) (*api.ListTenantsResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls.Add(1)
	out := make([]*api.TenantListItem, 0, len(f.tenants))
	for _, t := range f.tenants {
		if req.GetSearch() == "" || strings.Contains(t.Name, req.GetSearch()) {
			out = append(out, &api.TenantListItem{
				Id:                  t.Id,
				Name:                t.Name,
				CanHaveGateways:     t.CanHaveGateways,
				PrivateGatewaysUp:   t.PrivateGatewaysUp,
				PrivateGatewaysDown: t.PrivateGatewaysDown,
			})
		}
	}
	return &api.ListTenantsResponse{TotalCount: uint32(len(out)), Result: out}, nil
}

func (f *fakeTenant) Create(_ context.Context, req *api.CreateTenantRequest) (*api.CreateTenantResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCalls.Add(1)
	if req.GetTenant() == nil {
		return nil, status.Error(codes.InvalidArgument, "tenant is required")
	}
	id := uuid.NewString()
	stored := &api.Tenant{
		Id:                  id,
		Name:                req.GetTenant().Name,
		Description:         req.GetTenant().Description,
		CanHaveGateways:     req.GetTenant().CanHaveGateways,
		PrivateGatewaysUp:   req.GetTenant().PrivateGatewaysUp,
		PrivateGatewaysDown: req.GetTenant().PrivateGatewaysDown,
	}
	f.tenants[id] = stored
	return &api.CreateTenantResponse{Id: id}, nil
}

// SeedTenant pre-populates the mock with a tenant of the given name and
// returns the assigned UUID. Used by reuse-path tests.
func (f *fakeTenant) SeedTenant(name string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := uuid.NewString()
	f.tenants[id] = &api.Tenant{
		Id:                  id,
		Name:                name,
		Description:         "Pre-seeded by test",
		CanHaveGateways:     true,
		PrivateGatewaysUp:   true,
		PrivateGatewaysDown: true,
	}
	return id
}

func (f *fakeTenant) CreateCalls() int64 { return f.createCalls.Load() }
func (f *fakeTenant) ListCalls() int64   { return f.listCalls.Load() }

// fakeApplication implements ApplicationService for EnsureApplication tests.
type fakeApplication struct {
	api.UnimplementedApplicationServiceServer
	mu           sync.Mutex
	applications map[string]*api.Application // keyed by Application.Id
	createCalls  atomic.Int64
	listCalls    atomic.Int64
}

func newFakeApplication() *fakeApplication {
	return &fakeApplication{applications: map[string]*api.Application{}}
}

func (f *fakeApplication) List(_ context.Context, req *api.ListApplicationsRequest) (*api.ListApplicationsResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls.Add(1)
	out := make([]*api.ApplicationListItem, 0, len(f.applications))
	for _, app := range f.applications {
		if req.GetTenantId() != "" && app.TenantId != req.GetTenantId() {
			continue
		}
		if req.GetSearch() != "" && !strings.Contains(app.Name, req.GetSearch()) {
			continue
		}
		out = append(out, &api.ApplicationListItem{
			Id:          app.Id,
			Name:        app.Name,
			Description: app.Description,
		})
	}
	return &api.ListApplicationsResponse{TotalCount: uint32(len(out)), Result: out}, nil
}

func (f *fakeApplication) Create(_ context.Context, req *api.CreateApplicationRequest) (*api.CreateApplicationResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCalls.Add(1)
	if req.GetApplication() == nil {
		return nil, status.Error(codes.InvalidArgument, "application is required")
	}
	id := uuid.NewString()
	app := &api.Application{
		Id:          id,
		Name:        req.GetApplication().Name,
		Description: req.GetApplication().Description,
		TenantId:    req.GetApplication().TenantId,
	}
	f.applications[id] = app
	return &api.CreateApplicationResponse{Id: id}, nil
}

// SeedApplication pre-populates a (tenant_id, name) application; returns its UUID.
func (f *fakeApplication) SeedApplication(tenantID, name string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := uuid.NewString()
	f.applications[id] = &api.Application{
		Id:          id,
		Name:        name,
		Description: "Pre-seeded by test",
		TenantId:    tenantID,
	}
	return id
}

func (f *fakeApplication) CreateCalls() int64 { return f.createCalls.Load() }
func (f *fakeApplication) ListCalls() int64   { return f.listCalls.Load() }

// fakeDeviceProfile implements DeviceProfileService for codec_js round-trip tests.
type fakeDeviceProfile struct {
	api.UnimplementedDeviceProfileServiceServer
	mu          sync.Mutex
	profiles    map[string]*api.DeviceProfile // keyed by DeviceProfile.Id
	createCalls atomic.Int64
	updateCalls atomic.Int64
}

func newFakeDeviceProfile() *fakeDeviceProfile {
	return &fakeDeviceProfile{profiles: map[string]*api.DeviceProfile{}}
}

func cloneDeviceProfile(in *api.DeviceProfile) *api.DeviceProfile {
	if in == nil {
		return nil
	}
	return &api.DeviceProfile{
		Id:                  in.Id,
		TenantId:            in.TenantId,
		Name:                in.Name,
		Description:         in.Description,
		Region:              in.Region,
		MacVersion:          in.MacVersion,
		RegParamsRevision:   in.RegParamsRevision,
		AdrAlgorithmId:      in.AdrAlgorithmId,
		PayloadCodecRuntime: in.PayloadCodecRuntime,
		PayloadCodecScript:  in.PayloadCodecScript,
		UplinkInterval:      in.UplinkInterval,
		SupportsOtaa:        in.SupportsOtaa,
	}
}

func (f *fakeDeviceProfile) Create(_ context.Context, req *api.CreateDeviceProfileRequest) (*api.CreateDeviceProfileResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCalls.Add(1)
	if req.GetDeviceProfile() == nil {
		return nil, status.Error(codes.InvalidArgument, "device_profile is required")
	}
	id := uuid.NewString()
	stored := cloneDeviceProfile(req.GetDeviceProfile())
	stored.Id = id
	f.profiles[id] = stored
	return &api.CreateDeviceProfileResponse{Id: id}, nil
}

func (f *fakeDeviceProfile) Get(_ context.Context, req *api.GetDeviceProfileRequest) (*api.GetDeviceProfileResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	dp, ok := f.profiles[req.GetId()]
	if !ok {
		return nil, status.Error(codes.NotFound, "device-profile not found")
	}
	now := timestamppb.Now()
	return &api.GetDeviceProfileResponse{
		DeviceProfile: cloneDeviceProfile(dp),
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}

func (f *fakeDeviceProfile) Update(_ context.Context, req *api.UpdateDeviceProfileRequest) (*emptypb.Empty, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updateCalls.Add(1)
	if req.GetDeviceProfile() == nil || req.GetDeviceProfile().Id == "" {
		return nil, status.Error(codes.InvalidArgument, "device_profile.id is required")
	}
	if _, ok := f.profiles[req.GetDeviceProfile().Id]; !ok {
		return nil, status.Error(codes.NotFound, "device-profile not found")
	}
	f.profiles[req.GetDeviceProfile().Id] = cloneDeviceProfile(req.GetDeviceProfile())
	return &emptypb.Empty{}, nil
}

func (f *fakeDeviceProfile) CreateCalls() int64 { return f.createCalls.Load() }
func (f *fakeDeviceProfile) UpdateCalls() int64 { return f.updateCalls.Load() }

// ChirpStackMockHandles exposes the fake servers registered against the v4
// bufconn mock so Plan 02-05 tests can pre-seed entities and assert call
// counts. Phase 1 callers (which only ever needed PingDevices to succeed)
// keep using the no-op accessor — the handles are nil for the v3 / down
// modes.
type ChirpStackMockHandles struct {
	Tenant        *fakeTenant
	Application   *fakeApplication
	DeviceProfile *fakeDeviceProfile
	Device        *fakeDevice
}

// NewChirpStackMockBufWithHandles is the Plan 02-05 superset constructor: same
// bufconn dialer + apiToken as NewChirpStackMockBuf, plus a handle struct that
// gives tests direct read/write access to the in-memory state of the v4 fakes.
// Phase 1 callers should keep using NewChirpStackMockBuf — this helper exists
// so chirpstack package tests can verify list-then-create idempotency without
// poking at proto-level RPCs they shouldn't be aware of.
func NewChirpStackMockBufWithHandles(t *testing.T) (func(context.Context, string) (net.Conn, error), string, *ChirpStackMockHandles) {
	t.Helper()
	const apiToken = "test-token"

	lis := bufconn.Listen(bufconnSize)
	srv := grpc.NewServer()

	tenant := newFakeTenant()
	app := newFakeApplication()
	dp := newFakeDeviceProfile()
	dev := newFakeDevice()

	api.RegisterInternalServiceServer(srv, &fakeInternalV4{version: "v4.17.0"})
	api.RegisterTenantServiceServer(srv, tenant)
	api.RegisterApplicationServiceServer(srv, app)
	api.RegisterDeviceProfileServiceServer(srv, dp)
	api.RegisterDeviceServiceServer(srv, dev)

	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(func() {
		srv.Stop()
		_ = lis.Close()
	})

	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}
	return dialer, apiToken, &ChirpStackMockHandles{
		Tenant:        tenant,
		Application:   app,
		DeviceProfile: dp,
		Device:        dev,
	}
}

// NewChirpStackMockBuf returns a bufconn-backed in-process gRPC server
// simulating ChirpStack at one of three modes:
//
//   - "v4"   — InternalService.GetVersion returns "v4.17.0";
//     DeviceService.List returns an empty page (CHIRP-01 happy path).
//   - "v3"   — InternalService.GetVersion returns codes.Unimplemented;
//     DeviceService is NOT registered (matches v3's wire surface for our purposes).
//   - "down" — server is started then immediately stopped, so dials succeed
//     but RPCs fail with codes.Unavailable (matches a refused / dead server).
//
// The returned dialer is suitable for grpc.WithContextDialer; the test target
// string passed to grpc.NewClient should be "passthrough:///bufnet" (the
// passthrough resolver is the canonical pairing for a custom dialer).
//
// Cleanup (server.Stop + listener.Close) is wired via t.Cleanup so callers do
// not need to defer anything.
//
// The returned apiToken is a dummy string ("test-token") that mirrors the
// shape of a real ChirpStack v4 API token; the mock does NOT verify the
// metadata header, so production TLS / auth concerns belong to integration
// tests, not unit-level callers of this helper.
func NewChirpStackMockBuf(t *testing.T, mode string) (func(context.Context, string) (net.Conn, error), string) {
	t.Helper()
	const apiToken = "test-token"

	lis := bufconn.Listen(bufconnSize)
	srv := grpc.NewServer()

	switch mode {
	case "v4":
		api.RegisterInternalServiceServer(srv, &fakeInternalV4{version: "v4.17.0"})
		api.RegisterDeviceServiceServer(srv, &fakeDevice{})
	case "v3":
		api.RegisterInternalServiceServer(srv, &fakeInternalV3{})
	case "down":
		// Register nothing; we'll Stop() the server before any dial completes
		// so callers see codes.Unavailable on every RPC.
	default:
		t.Fatalf("NewChirpStackMockBuf: unknown mode %q (want v4, v3, or down)", mode)
	}

	if mode == "down" {
		srv.Stop()
	} else {
		go func() {
			// Serve returns ErrServerStopped at teardown; no useful action.
			_ = srv.Serve(lis)
		}()
	}

	t.Cleanup(func() {
		srv.Stop()
		_ = lis.Close()
	})

	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}
	return dialer, apiToken
}

// NewChirpStackMock retains the Plan 02 stub signature for any test that may
// have referenced it before Plan 12 landed. Today there are zero such callers
// (the stub was a `t.Skip`); we keep the function as a deliberate trap that
// fails loudly if any future plan tries to use the host:port form instead of
// the in-process bufconn helper above. Migrating callers is one mechanical
// rewrite — see internal/chirpstack/{client,version}_test.go for the pattern.
func NewChirpStackMock(t *testing.T, mode string) (addr string, apiToken string) {
	t.Helper()
	t.Fatalf("testsupport.NewChirpStackMock is replaced by NewChirpStackMockBuf as of Plan 12 — switch to the bufconn dialer (mode=%q)", mode)
	return "", ""
}
