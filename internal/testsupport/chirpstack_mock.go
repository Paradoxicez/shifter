package testsupport

import (
	"context"
	"net"
	"testing"

	api "github.com/chirpstack/chirpstack/api/go/v4/api"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"
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
type fakeDevice struct {
	api.UnimplementedDeviceServiceServer
}

func (f *fakeDevice) List(_ context.Context, _ *api.ListDevicesRequest) (*api.ListDevicesResponse, error) {
	return &api.ListDevicesResponse{TotalCount: 0, Result: nil}, nil
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
