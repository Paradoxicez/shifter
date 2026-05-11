package testsupport

// Phase 3 extension of the bufconn-backed ChirpStack mock.
//
// The Phase 2 mock (chirpstack_mock.go) covered TenantService /
// ApplicationService / DeviceProfileService / DeviceService for the
// EnsureTenantAndApplication bootstrap + CreateDevice + CreateDeviceKeys
// surface needed by Plan 02-05. Phase 3 extends that surface with:
//
//   - GatewayService: Create / Get / Update / Delete / List / GetMetrics
//     (so internal/chirpstack/gateway.go wrappers can be tested without a
//     live ChirpStack)
//   - DeviceService.GetKeys (OTAA reveal) + GetActivation (ABP reveal)
//     (so internal/api/devices_reveal_test.go can mock the read-back path
//     without requiring a real CS instance)
//
// Keying convention: all IDs are stored lowercase. Real ChirpStack stores
// EUIs case-insensitively and we follow that — the production wrapper
// normalizes before calling the gRPC client, but the mock is forgiving so
// tests that accidentally pass mixed-case IDs still resolve.

import (
	"context"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	api "github.com/chirpstack/chirpstack/api/go/v4/api"
	common "github.com/chirpstack/chirpstack/api/go/v4/common"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// bufconnDialer matches the Phase 2 NewChirpStackMockBuf signature so
// callers can pass the returned dialer to grpc.WithContextDialer.
type bufconnDialer = func(context.Context, string) (net.Conn, error)

// fakeGateway implements the Phase 3 GatewayService surface against an
// in-memory map. The 6 RPCs in CONTEXT D-01..D-03 are covered; relay-gateway
// + duty-cycle + client-certificate RPCs are intentionally left as the
// embedded Unimplemented defaults (no Phase 3 caller needs them).
type fakeGateway struct {
	api.UnimplementedGatewayServiceServer

	mu          sync.Mutex
	gateways    map[string]*api.Gateway                   // keyed by lowercase gateway_id
	metrics     map[string]*api.GetGatewayMetricsResponse // keyed by lowercase gateway_id
	createCalls atomic.Int64
	getCalls    atomic.Int64
	updateCalls atomic.Int64
	deleteCalls atomic.Int64
	listCalls   atomic.Int64
	metricCalls atomic.Int64
}

func newFakeGateway() *fakeGateway {
	return &fakeGateway{
		gateways: map[string]*api.Gateway{},
		metrics:  map[string]*api.GetGatewayMetricsResponse{},
	}
}

// gwKey lowercases the gateway_id so all lookups are case-insensitive.
func gwKey(id string) string { return strings.ToLower(id) }

func cloneGateway(in *api.Gateway) *api.Gateway {
	if in == nil {
		return nil
	}
	cp := &api.Gateway{
		GatewayId:     in.GatewayId,
		Name:          in.Name,
		Description:   in.Description,
		Location:      in.Location,
		TenantId:      in.TenantId,
		StatsInterval: in.StatsInterval,
	}
	if in.Tags != nil {
		cp.Tags = make(map[string]string, len(in.Tags))
		for k, v := range in.Tags {
			cp.Tags[k] = v
		}
	}
	if in.Metadata != nil {
		cp.Metadata = make(map[string]string, len(in.Metadata))
		for k, v := range in.Metadata {
			cp.Metadata[k] = v
		}
	}
	return cp
}

func (f *fakeGateway) Create(_ context.Context, req *api.CreateGatewayRequest) (*emptypb.Empty, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCalls.Add(1)
	if req.GetGateway() == nil {
		return nil, status.Error(codes.InvalidArgument, "gateway is required")
	}
	gw := req.GetGateway()
	if gw.GatewayId == "" {
		return nil, status.Error(codes.InvalidArgument, "gateway_id is required")
	}
	key := gwKey(gw.GatewayId)
	if _, exists := f.gateways[key]; exists {
		return nil, status.Error(codes.AlreadyExists, "gateway already exists")
	}
	f.gateways[key] = cloneGateway(gw)
	return &emptypb.Empty{}, nil
}

func (f *fakeGateway) Get(_ context.Context, req *api.GetGatewayRequest) (*api.GetGatewayResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getCalls.Add(1)
	gw, ok := f.gateways[gwKey(req.GetGatewayId())]
	if !ok {
		return nil, status.Error(codes.NotFound, "gateway not found")
	}
	now := timestamppb.Now()
	return &api.GetGatewayResponse{
		Gateway:    cloneGateway(gw),
		CreatedAt:  now,
		UpdatedAt:  now,
		LastSeenAt: now,
	}, nil
}

func (f *fakeGateway) Update(_ context.Context, req *api.UpdateGatewayRequest) (*emptypb.Empty, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updateCalls.Add(1)
	if req.GetGateway() == nil || req.GetGateway().GatewayId == "" {
		return nil, status.Error(codes.InvalidArgument, "gateway.gateway_id is required")
	}
	key := gwKey(req.GetGateway().GatewayId)
	if _, ok := f.gateways[key]; !ok {
		return nil, status.Error(codes.NotFound, "gateway not found")
	}
	f.gateways[key] = cloneGateway(req.GetGateway())
	return &emptypb.Empty{}, nil
}

func (f *fakeGateway) Delete(_ context.Context, req *api.DeleteGatewayRequest) (*emptypb.Empty, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteCalls.Add(1)
	key := gwKey(req.GetGatewayId())
	if _, ok := f.gateways[key]; !ok {
		return nil, status.Error(codes.NotFound, "gateway not found")
	}
	delete(f.gateways, key)
	delete(f.metrics, key)
	return &emptypb.Empty{}, nil
}

func (f *fakeGateway) List(_ context.Context, req *api.ListGatewaysRequest) (*api.ListGatewaysResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls.Add(1)
	out := make([]*api.GatewayListItem, 0, len(f.gateways))
	for _, gw := range f.gateways {
		if req.GetTenantId() != "" && gw.TenantId != req.GetTenantId() {
			continue
		}
		if s := req.GetSearch(); s != "" && !strings.Contains(gw.Name, s) {
			continue
		}
		now := timestamppb.Now()
		out = append(out, &api.GatewayListItem{
			TenantId:    gw.TenantId,
			GatewayId:   gw.GatewayId,
			Name:        gw.Name,
			Description: gw.Description,
			Location:    gw.Location,
			CreatedAt:   now,
			UpdatedAt:   now,
			LastSeenAt:  now,
			State:       api.GatewayState_ONLINE,
		})
	}
	return &api.ListGatewaysResponse{TotalCount: uint32(len(out)), Result: out}, nil
}

func (f *fakeGateway) GetMetrics(_ context.Context, req *api.GetGatewayMetricsRequest) (*api.GetGatewayMetricsResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.metricCalls.Add(1)
	key := gwKey(req.GetGatewayId())
	if _, ok := f.gateways[key]; !ok {
		return nil, status.Error(codes.NotFound, "gateway not found")
	}
	if m, ok := f.metrics[key]; ok {
		return m, nil
	}
	// Default: 24 zero-buckets for rx/tx (canonical D-02 sparkline default).
	zeros := make([]float32, 24)
	zero := func(name string) *common.Metric {
		return &common.Metric{
			Name:    name,
			Kind:    common.MetricKind_ABSOLUTE,
			Datasets: []*common.MetricDataset{{Label: name, Data: zeros}},
		}
	}
	return &api.GetGatewayMetricsResponse{
		RxPackets: zero("rx_packets"),
		TxPackets: zero("tx_packets"),
	}, nil
}

// SetGatewayMetrics overrides the GetMetrics response for the given gateway.
// rx / tx must be the same length; ok is reserved for future RX-status series
// (D-02) — for now it is unused but kept in the signature so callers don't
// rewrite when stats expand.
func (f *fakeGateway) SetGatewayMetrics(gatewayID string, rx, tx, _ []float32) {
	f.mu.Lock()
	defer f.mu.Unlock()
	metric := func(name string, data []float32) *common.Metric {
		return &common.Metric{
			Name:     name,
			Kind:     common.MetricKind_ABSOLUTE,
			Datasets: []*common.MetricDataset{{Label: name, Data: data}},
		}
	}
	f.metrics[gwKey(gatewayID)] = &api.GetGatewayMetricsResponse{
		RxPackets: metric("rx_packets", rx),
		TxPackets: metric("tx_packets", tx),
	}
}

// SeedGateway pre-populates the mock with a gateway for read-path tests.
func (f *fakeGateway) SeedGateway(gw *api.Gateway) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gateways[gwKey(gw.GatewayId)] = cloneGateway(gw)
}

func (f *fakeGateway) CreateCalls() int64 { return f.createCalls.Load() }
func (f *fakeGateway) GetCalls() int64    { return f.getCalls.Load() }
func (f *fakeGateway) UpdateCalls() int64 { return f.updateCalls.Load() }
func (f *fakeGateway) DeleteCalls() int64 { return f.deleteCalls.Load() }
func (f *fakeGateway) ListCalls() int64   { return f.listCalls.Load() }
func (f *fakeGateway) MetricCalls() int64 { return f.metricCalls.Load() }

// fakeDeviceRevealExt extends fakeDevice with GetKeys + GetActivation so
// the Plan 03-XX /api/devices/:eui/keys handler can read back OTAA / ABP
// credentials. We model this as separate seedable maps on the *existing*
// fakeDevice (added via methods on a wrapper type) — see GetDeviceKeys /
// GetDeviceActivation methods below, which are registered against the
// fakeDevice instance constructed by newFakeDevice().
//
// Rationale: we cannot redeclare methods on fakeDevice in this file because
// Go's method-on-type rules forbid split-file method sets only for embedded
// interface satisfaction at runtime — same-package methods on the same
// receiver are legal, so we just add them here.

// activations stores ABP activation state injected via SeedDeviceActivation.
// We keep this on a sidecar struct because the Phase 2 fakeDevice only knows
// about OTAA keys (createKeys path).
type fakeDeviceRevealState struct {
	mu          sync.Mutex
	activations map[string]*api.DeviceActivation // keyed by lowercase DevEUI
}

var deviceRevealState = &fakeDeviceRevealState{
	activations: map[string]*api.DeviceActivation{},
}

func (s *fakeDeviceRevealState) seedActivation(devEUI string, act *api.DeviceActivation) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activations[strings.ToLower(devEUI)] = act
}

func (s *fakeDeviceRevealState) getActivation(devEUI string) (*api.DeviceActivation, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	act, ok := s.activations[strings.ToLower(devEUI)]
	return act, ok
}

// SeedDeviceActivation injects an ABP activation for the given DevEUI so
// /api/devices/:eui/keys can return it. Stored on a package-level sidecar
// because the Phase 2 fakeDevice was OTAA-only.
//
// Callers in tests:
//
//	handles.Device.SeedDeviceActivation(eui, &api.DeviceActivation{...})
func (f *fakeDevice) SeedDeviceActivation(devEUI string, act *api.DeviceActivation) {
	deviceRevealState.seedActivation(devEUI, act)
}

// SeedDeviceKeys injects OTAA keys for the given DevEUI without requiring a
// prior CreateDevice call. Phase 2's CreateKeys requires the device to exist;
// reveal tests sometimes want to short-circuit that.
func (f *fakeDevice) SeedDeviceKeys(devEUI string, keys *api.DeviceKeys) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.devices[devEUI] == nil {
		// Provision a minimal placeholder device so Get/Delete still work.
		f.devices[devEUI] = &api.Device{DevEui: devEUI}
	}
	f.keys[devEUI] = keys
}

// GetDeviceKeys returns the previously-CreateKeys'd OTAA material, or
// codes.NotFound if no keys are recorded. Mirrors CS v4 behaviour: a device
// with no keys returns NotFound, which the production wrapper maps to 409
// no_credentials (D-27).
func (f *fakeDevice) GetDeviceKeys(_ context.Context, req *api.GetDeviceKeysRequest) (*api.GetDeviceKeysResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	keys, ok := f.keys[req.GetDevEui()]
	if !ok {
		return nil, status.Error(codes.NotFound, "device keys not found")
	}
	now := timestamppb.Now()
	return &api.GetDeviceKeysResponse{
		DeviceKeys: keys,
		CreatedAt:  now,
		UpdatedAt:  now,
	}, nil
}

// GetDeviceActivation returns the ABP activation seeded via
// SeedDeviceActivation, or codes.NotFound otherwise. CS v4 returns NotFound
// for an OTAA device that has never joined; D-27 surfaces that as 409
// no_credentials.
func (f *fakeDevice) GetDeviceActivation(_ context.Context, req *api.GetDeviceActivationRequest) (*api.GetDeviceActivationResponse, error) {
	act, ok := deviceRevealState.getActivation(req.GetDevEui())
	if !ok {
		return nil, status.Error(codes.NotFound, "device activation not found")
	}
	return &api.GetDeviceActivationResponse{DeviceActivation: act}, nil
}

// ChirpStackMockPhase3Handles is the Phase 3 superset of
// ChirpStackMockHandles — same Tenant / Application / DeviceProfile / Device
// pointers plus the new Gateway fake. Tests obtain this via
// NewChirpStackMockBufPhase3 instead of the Plan 02-05 NewChirpStackMockBufWithHandles.
type ChirpStackMockPhase3Handles struct {
	Tenant        *fakeTenant
	Application   *fakeApplication
	DeviceProfile *fakeDeviceProfile
	Device        *fakeDevice
	Gateway       *fakeGateway
}

// NewChirpStackMockBufPhase3 returns a bufconn dialer + apiToken + handle
// struct exposing the full Phase 3 mock surface. Implements the
// `RegisterGatewayServiceServer` registration in addition to the Phase 2
// services.
//
// Use this constructor in any Phase 3 test that needs Gateway* CRUD or
// device-key / activation reveal. Phase 1/2 tests can keep using
// NewChirpStackMockBuf / NewChirpStackMockBufWithHandles unchanged.
func NewChirpStackMockBufPhase3(t *testing.T) (dialer bufconnDialer, apiToken string, handles *ChirpStackMockPhase3Handles) {
	t.Helper()
	apiToken = "test-token"

	lis := bufconn.Listen(bufconnSize)
	srv := grpc.NewServer()

	tenant := newFakeTenant()
	app := newFakeApplication()
	dp := newFakeDeviceProfile()
	dev := newFakeDevice()
	gw := newFakeGateway()

	api.RegisterInternalServiceServer(srv, &fakeInternalV4{version: "v4.17.0"})
	api.RegisterTenantServiceServer(srv, tenant)
	api.RegisterApplicationServiceServer(srv, app)
	api.RegisterDeviceProfileServiceServer(srv, dp)
	api.RegisterDeviceServiceServer(srv, dev)
	api.RegisterGatewayServiceServer(srv, gw)

	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(func() {
		srv.Stop()
		_ = lis.Close()
	})

	dialer = func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}
	return dialer, apiToken, &ChirpStackMockPhase3Handles{
		Tenant:        tenant,
		Application:   app,
		DeviceProfile: dp,
		Device:        dev,
		Gateway:       gw,
	}
}
