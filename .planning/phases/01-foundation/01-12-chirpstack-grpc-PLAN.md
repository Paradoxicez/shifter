---
phase: 01-foundation
plan: 12
type: execute
wave: 8
depends_on: [02, 04]
files_modified:
  - go.mod
  - go.sum
  - internal/chirpstack/client.go
  - internal/chirpstack/version.go
  - internal/chirpstack/version_test.go
  - internal/chirpstack/client_test.go
  - internal/chirpstack/ping.go
  - internal/chirpstack/errors.go
  - internal/testsupport/chirpstack_mock.go
autonomous: true
requirements:
  - CHIRP-01
  - INST-05
must_haves:
  truths:
    - "Dial(ctx, cfg) returns a *grpc.ClientConn against a v4 mock with API token in metadata"
    - "ProbeVersion(ctx, conn) on v4 mock returns the version string"
    - "ProbeVersion(ctx, conn) on v3 mock (returns Unimplemented) returns ErrChirpStackV3OrUnknown (INST-05)"
    - "ListDevices via the gRPC client succeeds against the mock (CHIRP-01 gRPC reachability)"
    - "Client wraps the gRPC connection and only this package imports chirpstack/api/go/v4"
  artifacts:
    - path: "internal/chirpstack/client.go"
      provides: "Dial + Client wrapping with API token interceptor"
      contains: "Dial"
    - path: "internal/chirpstack/version.go"
      provides: "ProbeVersion using InternalService.GetVersion(emptypb.Empty)"
      contains: "InternalServiceClient"
    - path: "internal/chirpstack/errors.go"
      provides: "ErrChirpStackV3OrUnknown sentinel error"
      contains: "ErrChirpStackV3OrUnknown"
    - path: "internal/testsupport/chirpstack_mock.go"
      provides: "Plan 02 stub replaced — bufconn-based gRPC mock with v3/v4/down modes"
      contains: "bufconn"
  key_links:
    - from: "internal/chirpstack/version.go"
      to: "github.com/chirpstack/chirpstack/api/go/v4/api.NewInternalServiceClient"
      via: "gRPC InternalService.GetVersion"
      pattern: "InternalServiceClient"
    - from: "internal/chirpstack/client.go"
      to: "credentials.NewTLS / insecure.NewCredentials"
      via: "TLS by default; cfg.Insecure=true uses plaintext"
      pattern: "NewClientConn"
---

<objective>
Implement the ChirpStack gRPC client (`Dial`), the version probe (`ProbeVersion` using `InternalService.GetVersion`), and the v3-rejection sentinel error. Also implement the shared testcontainers-style gRPC mock (in-process bufconn) that simulates v3 (Unimplemented), v4 (success), and down (refused) — used by Plans 12 (this), 14, 15, 17.

Purpose: CHIRP-01 (gRPC control-plane), INST-05 (v3 rejection at install + at server start). Plans 14/15/17 depend on `ProbeVersion` and `Dial`.

Output: `go test ./internal/chirpstack -run 'TestProbeVersion_'` passes against the bufconn mock for both v3 and v4.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/01-foundation/01-RESEARCH.md
@.planning/phases/01-foundation/01-CONTEXT.md
@.planning/research/PITFALLS.md
@01-04-config-secrets-PLAN.md

<interfaces>
RESEARCH §Pattern 4 (lines 468-498) — verbatim ProbeVersion:
```go
client := api.NewInternalServiceClient(conn)
resp, err := client.GetVersion(ctx, &emptypb.Empty{})
if err != nil {
    st, ok := status.FromError(err)
    if ok && (st.Code() == codes.Unimplemented || st.Code() == codes.NotFound) {
        return "", ErrChirpStackV3OrUnknown
    }
    return "", fmt.Errorf("chirpstack version probe failed: %w", err)
}
return resp.Version, nil
```

Public API:
```go
package chirpstack

func Dial(ctx context.Context, cfg config.CSConfig) (*grpc.ClientConn, error)

type Client struct { conn *grpc.ClientConn }
func NewClient(conn *grpc.ClientConn) *Client
func (c *Client) ListDevices(ctx context.Context, applicationID string) (...)  // CHIRP-01 smoke

func ProbeVersion(ctx context.Context, conn *grpc.ClientConn) (string, error)
var ErrChirpStackV3OrUnknown = errors.New("...")
```

Mock (in `internal/testsupport/chirpstack_mock.go`):
```go
func NewChirpStackMock(t *testing.T, mode string) (addr string, apiToken string)
//   mode = "v4" → register InternalServiceServer that returns version string
//   mode = "v3" → register a server that returns codes.Unimplemented from GetVersion
//   mode = "down" → don't register anything (port returns RST)
```

Auth: ChirpStack v4 uses a per-API-token bearer in gRPC metadata key `authorization` value `Bearer <token>` (verified pkg.go.dev/chirpstack/chirpstack/api/go/v4 examples).
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Dial + Client + ProbeVersion + bufconn mock + tests</name>
  <files>go.mod, go.sum, internal/chirpstack/client.go, internal/chirpstack/version.go, internal/chirpstack/errors.go, internal/chirpstack/ping.go, internal/chirpstack/version_test.go, internal/chirpstack/client_test.go, internal/testsupport/chirpstack_mock.go</files>
  <read_first>
    - .planning/phases/01-foundation/01-RESEARCH.md §"Pattern 4: ChirpStack v3 rejection probe" (lines 468-498)
    - .planning/phases/01-foundation/01-RESEARCH.md §"Pitfall 1: ChirpStack v3 connection accepted" (lines 1301-1310)
    - .planning/phases/01-foundation/01-VALIDATION.md (TestProbeVersion_v3, TestProbeVersion_v4, TestClient_ListDevices_Mock names)
    - 01-04-config-secrets-PLAN.md (CSConfig shape: GRPCURL, APIToken, Insecure)
  </read_first>
  <behavior>
    - TestProbeVersion_v4: NewChirpStackMock(t, "v4") → ProbeVersion returns ("v4.17.0", nil).
    - TestProbeVersion_v3: NewChirpStackMock(t, "v3") → ProbeVersion returns ("", ErrChirpStackV3OrUnknown).
    - TestProbeVersion_Down: passes a closed conn → ProbeVersion returns ("", non-nil error wrapping rpc error).
    - TestClient_ListDevices_Mock: client lists devices on v4 mock; returns slice; CHIRP-01 verified.
    - TestDial_AddsBearerToken: dial against a recorder server that captures metadata; assert `authorization: Bearer <token>` present.
  </behavior>
  <action>
1. Install gRPC + ChirpStack stubs:
   ```bash
   go get google.golang.org/grpc@latest
   go get google.golang.org/grpc/credentials@latest
   go get google.golang.org/grpc/credentials/insecure@latest
   go get google.golang.org/grpc/test/bufconn@latest
   go get google.golang.org/protobuf/types/known/emptypb@latest
   go get github.com/chirpstack/chirpstack/api/go/v4@latest
   ```

2. Create `internal/chirpstack/errors.go`:
   ```go
   package chirpstack

   import "errors"

   // ErrChirpStackV3OrUnknown is returned by ProbeVersion when the server doesn't
   // implement InternalService.GetVersion — the canonical signal for "not v4".
   // Per INST-05, the install wizard and `serve` startup must refuse to proceed.
   var ErrChirpStackV3OrUnknown = errors.New("chirpstack: server is v3 or non-ChirpStack — Shifter requires v4")
   ```

3. Create `internal/chirpstack/client.go`:
   ```go
   package chirpstack

   import (
       "context"
       "crypto/tls"
       "fmt"

       "google.golang.org/grpc"
       "google.golang.org/grpc/credentials"
       "google.golang.org/grpc/credentials/insecure"
       "google.golang.org/grpc/metadata"

       "github.com/shifter-io/shifter/internal/config"
   )

   // Dial opens a gRPC connection to the ChirpStack control plane.
   //
   // The API token is attached to every outgoing call via a UnaryClientInterceptor.
   // TLS is on by default; cfg.Insecure=true uses plaintext (only for in-cluster).
   func Dial(ctx context.Context, cfg config.CSConfig) (*grpc.ClientConn, error) {
       var creds credentials.TransportCredentials
       if cfg.Insecure {
           creds = insecure.NewCredentials()
       } else {
           creds = credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})
       }
       conn, err := grpc.NewClient(
           cfg.GRPCURL,
           grpc.WithTransportCredentials(creds),
           grpc.WithUnaryInterceptor(authInterceptor(cfg.APIToken)),
       )
       if err != nil {
           return nil, fmt.Errorf("grpc.NewClient: %w", err)
       }
       _ = ctx
       return conn, nil
   }

   func authInterceptor(token string) grpc.UnaryClientInterceptor {
       return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
           if token != "" {
               ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
           }
           return invoker(ctx, method, req, reply, cc, opts...)
       }
   }

   // Client is a thin wrapper that owns the conn lifecycle and exposes domain
   // methods on top of the generated stubs. Phase 1 needs only ListDevices for the
   // CHIRP-01 smoke test; Plan 17 uses it for the gRPC channel of Test Connection.
   // Phase 2/3 extends with TenantService, ApplicationService, DeviceProfileService etc.
   type Client struct {
       conn *grpc.ClientConn
   }

   func NewClient(conn *grpc.ClientConn) *Client { return &Client{conn: conn} }
   func (c *Client) Conn() *grpc.ClientConn       { return c.conn }
   func (c *Client) Close() error                 { return c.conn.Close() }
   ```

4. Create `internal/chirpstack/version.go` — VERBATIM from RESEARCH §Pattern 4:
   ```go
   package chirpstack

   import (
       "context"
       "fmt"

       "google.golang.org/grpc"
       "google.golang.org/grpc/codes"
       "google.golang.org/grpc/status"
       "google.golang.org/protobuf/types/known/emptypb"

       api "github.com/chirpstack/chirpstack/api/go/v4/api"
   )

   // ProbeVersion calls InternalService.GetVersion and returns the version string.
   // If the server is v3 (or any server that does NOT implement InternalService),
   // returns ErrChirpStackV3OrUnknown. Per INST-05, callers must refuse to continue.
   func ProbeVersion(ctx context.Context, conn *grpc.ClientConn) (string, error) {
       client := api.NewInternalServiceClient(conn)
       resp, err := client.GetVersion(ctx, &emptypb.Empty{})
       if err != nil {
           st, ok := status.FromError(err)
           if ok && (st.Code() == codes.Unimplemented || st.Code() == codes.NotFound) {
               return "", ErrChirpStackV3OrUnknown
           }
           return "", fmt.Errorf("chirpstack version probe: %w", err)
       }
       if resp.Version == "" {
           return "", ErrChirpStackV3OrUnknown
       }
       return resp.Version, nil
   }
   ```

5. Create `internal/chirpstack/ping.go` — used by Plan 17 Test Connection:
   ```go
   package chirpstack

   import (
       "context"
       "fmt"

       api "github.com/chirpstack/chirpstack/api/go/v4/api"
   )

   // PingDevices is a CHIRP-01 smoke test: list devices via the gRPC client.
   // Phase 1 invokes this only inside Test Connection (Plan 17) — applicationID
   // can be empty to list across all applications (per chirpstack-api docs).
   func (c *Client) PingDevices(ctx context.Context, applicationID string) error {
       cli := api.NewDeviceServiceClient(c.conn)
       _, err := cli.List(ctx, &api.ListDevicesRequest{ApplicationId: applicationID, Limit: 1})
       if err != nil {
           return fmt.Errorf("list devices: %w", err)
       }
       return nil
   }
   ```

6. Create `internal/testsupport/chirpstack_mock.go` (replaces the Plan 02 skip-stub):
   ```go
   package testsupport

   import (
       "context"
       "net"
       "testing"

       "google.golang.org/grpc"
       "google.golang.org/grpc/codes"
       "google.golang.org/grpc/credentials/insecure"
       "google.golang.org/grpc/status"
       "google.golang.org/grpc/test/bufconn"
       "google.golang.org/protobuf/types/known/emptypb"

       api "github.com/chirpstack/chirpstack/api/go/v4/api"
   )

   const bufSize = 1024 * 1024

   type fakeInternal struct {
       api.UnimplementedInternalServiceServer
       version string
   }

   func (f *fakeInternal) GetVersion(_ context.Context, _ *emptypb.Empty) (*api.GetVersionResponse, error) {
       return &api.GetVersionResponse{Version: f.version}, nil
   }

   type v3Internal struct {
       api.UnimplementedInternalServiceServer
   }

   func (v *v3Internal) GetVersion(_ context.Context, _ *emptypb.Empty) (*api.GetVersionResponse, error) {
       return nil, status.Error(codes.Unimplemented, "method GetVersion not implemented (simulated v3)")
   }

   type fakeDevice struct {
       api.UnimplementedDeviceServiceServer
   }

   func (f *fakeDevice) List(_ context.Context, _ *api.ListDevicesRequest) (*api.ListDevicesResponse, error) {
       return &api.ListDevicesResponse{TotalCount: 0, Result: nil}, nil
   }

   // NewChirpStackMockBuf returns a bufconn dial helper.
   // mode is one of: "v4", "v3", "down" (closed listener).
   // The helper returns a net.Dial-able function and a teardown.
   //
   // Tests typically call:
   //   dial, _ := NewChirpStackMockBuf(t, "v4")
   //   conn, _ := grpc.NewClient("passthrough:///bufnet",
   //       grpc.WithContextDialer(dial),
   //       grpc.WithTransportCredentials(insecure.NewCredentials()))
   func NewChirpStackMockBuf(t *testing.T, mode string) (func(context.Context, string) (net.Conn, error), string) {
       t.Helper()
       const apiToken = "test-token"

       lis := bufconn.Listen(bufSize)
       srv := grpc.NewServer()

       switch mode {
       case "v4":
           api.RegisterInternalServiceServer(srv, &fakeInternal{version: "v4.17.0"})
           api.RegisterDeviceServiceServer(srv, &fakeDevice{})
       case "v3":
           api.RegisterInternalServiceServer(srv, &v3Internal{})
       case "down":
           // No server registration — caller's dial will succeed but RPCs return Unavailable.
           // To simulate truly down, stop the server immediately:
           srv.Stop()
       default:
           t.Fatalf("unknown mock mode %q", mode)
       }

       if mode != "down" {
           go func() { _ = srv.Serve(lis) }()
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

   // Compatibility shim retained from Plan 02 stub; new code should use NewChirpStackMockBuf.
   func NewChirpStackMock(t *testing.T, mode string) (addr string, apiToken string) {
       t.Skip("Use NewChirpStackMockBuf for in-process gRPC mocks")
       return "", ""
   }

   var _ = insecure.NewCredentials
   ```

7. Replace `internal/chirpstack/version_test.go`:
   ```go
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

   func dialMock(t *testing.T, mode string) *grpc.ClientConn {
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

   func TestProbeVersion_v4(t *testing.T) {
       conn := dialMock(t, "v4")
       ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
       defer cancel()
       v, err := ProbeVersion(ctx, conn)
       require.NoError(t, err)
       require.Equal(t, "v4.17.0", v)
   }

   func TestProbeVersion_v3(t *testing.T) {
       conn := dialMock(t, "v3")
       ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
       defer cancel()
       _, err := ProbeVersion(ctx, conn)
       require.Error(t, err)
       require.True(t, errors.Is(err, ErrChirpStackV3OrUnknown), "INST-05: v3 must yield ErrChirpStackV3OrUnknown")
   }
   ```

8. Replace `internal/chirpstack/client_test.go`:
   ```go
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
       require.NoError(t, cli.PingDevices(ctx, ""), "CHIRP-01: gRPC client must list devices via mock")
   }
   ```
  </action>
  <verify>
    <automated>go test ./internal/chirpstack -race -count=1 -v</automated>
  </verify>
  <acceptance_criteria>
    - File `internal/chirpstack/client.go` exports `func Dial(ctx, cfg) (*grpc.ClientConn, error)`, `type Client`, `func NewClient(conn) *Client`, `func (c *Client) Close()`
    - `Dial` uses TLS by default and `insecure.NewCredentials()` when `cfg.Insecure == true`
    - `Dial` attaches `authorization: Bearer <token>` metadata via UnaryClientInterceptor (grep proof: `metadata.AppendToOutgoingContext`)
    - File `internal/chirpstack/version.go` exports `func ProbeVersion(ctx, conn) (string, error)` calling `api.NewInternalServiceClient(conn).GetVersion(ctx, &emptypb.Empty{})`
    - File `internal/chirpstack/errors.go` exports `var ErrChirpStackV3OrUnknown` as a package-level error sentinel
    - File `internal/testsupport/chirpstack_mock.go` exports `func NewChirpStackMockBuf(t, mode) (dialer, token)` supporting modes `"v4"`, `"v3"`, `"down"`
    - Command `go test ./internal/chirpstack -run TestProbeVersion_v4 -race` exits 0 (per VALIDATION.md)
    - Command `go test ./internal/chirpstack -run TestProbeVersion_v3 -race` exits 0 (per VALIDATION.md)
    - Command `go test ./internal/chirpstack -run TestClient_ListDevices_Mock -race` exits 0 (per VALIDATION.md)
    - Only `internal/chirpstack/*` imports `github.com/chirpstack/chirpstack/api/go/v4` — verified by `grep -r "chirpstack/api/go/v4" internal/ | grep -v internal/chirpstack` returning no matches
  </acceptance_criteria>
  <done>
    gRPC client + version probe ready. Plan 14/15 (wizard) and Plan 17 (Test Connection) and Plan 18 (`serve` startup refusal) consume `Dial` + `ProbeVersion`. Mock used by all subsequent tests.
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| Shifter → ChirpStack gRPC | API token in metadata; TLS in production |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-12-01 | Spoofing (v3 acceptance) | install wizard against v3 server | mitigate | INST-05: `ProbeVersion` returns `ErrChirpStackV3OrUnknown` for `Unimplemented`/`NotFound`; checked by Plan 14 (wizard step 2) and Plan 18 (`serve` startup). PITFALLS §1 / RESEARCH §Pattern 4. |
| T-12-02 | Information Disclosure | API token leaked in logs | mitigate | Token never appears in slog calls; `authInterceptor` attaches token without `log.Info`. ASVS V7. |
| T-12-03 | Tampering (TLS downgrade) | gRPC over plaintext in production | mitigate | Default `cfg.Insecure=false` uses TLS; `Validate()` could later reject Insecure outside dev. ASVS V9. |
| T-12-04 | Spoofing | client trusts arbitrary server cert | accept | Phase 1 uses system CA; `cfg.Insecure=true` is documented for in-cluster. Phase 6 may add `cfg.CACert`. ASVS V9. |
</threat_model>

<verification>
- `internal/chirpstack/` is the only package importing `chirpstack/api/go/v4` (architectural seam)
- `Dial` adds Bearer token to outgoing metadata
- `ProbeVersion` distinguishes v4 (success) from v3 (`Unimplemented` → `ErrChirpStackV3OrUnknown`)
- bufconn mock supports v4/v3/down modes
- All 3 VALIDATION.md tests pass
</verification>

<success_criteria>
- CHIRP-01 smoke (ListDevices) passes against mock
- INST-05 sentinel error in place; Plans 14/15/18 will check it
- Plan 17 Test Connection consumes `Dial` + `ProbeVersion`
- Architectural seam preserved (only `chirpstack/` imports protos)
</success_criteria>

<output>
After completion, create `.planning/phases/01-foundation/01-12-SUMMARY.md` documenting:
- Public API of internal/chirpstack
- bufconn mock usage pattern
- Bearer token interceptor pattern
- Architectural seam rule (`chirpstack/` only protos importer)
</output>
