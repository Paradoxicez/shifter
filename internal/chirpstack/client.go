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

// Dial opens a gRPC connection to the ChirpStack v4 control plane.
//
// The API token (cfg.APIToken) is attached to every outgoing call as gRPC
// metadata `authorization: Bearer <token>` via a UnaryClientInterceptor — the
// canonical wire format documented in chirpstack-api/go-examples and verified
// by ChirpStack's own admin UI. TLS is on by default
// (credentials.NewTLS with TLS 1.2 floor); cfg.Insecure=true switches to
// plaintext (insecure.NewCredentials) and is intended only for in-cluster /
// localhost paths where the gRPC channel never crosses an untrusted network.
//
// The returned *grpc.ClientConn is owned by the caller — close it via Client.Close
// when wrapped, or grpc.ClientConn.Close directly when used standalone.
//
// Per ARCHITECTURE: this package is the SOLE importer of
// github.com/chirpstack/chirpstack/api/go/v4. Other packages must consume the
// ChirpStack control plane through Client + ProbeVersion only.
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
	// ctx is reserved for future per-dial timeout / tracing integration;
	// grpc.NewClient itself is non-blocking (lazy connect) so no use today.
	_ = ctx
	return conn, nil
}

// authInterceptor returns a UnaryClientInterceptor that appends
// `authorization: Bearer <token>` to every outgoing call when token is non-empty.
// An empty token (e.g. unauthenticated probe paths) is silently skipped — the
// server then enforces auth on whatever RPC was attempted.
//
// Token is captured via closure rather than read from ctx so that a single
// Dial call can serve many goroutines without each having to thread the token
// through every context. Token is never logged (T-12-02).
func authInterceptor(token string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if token != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// Client wraps a ChirpStack v4 *grpc.ClientConn and owns its lifecycle. Phase 1
// exposes only the CHIRP-01 smoke (PingDevices) and the conn accessor for the
// version probe; Plan 17 (Test Connection) consumes both. Phase 2/3 will add
// TenantService / ApplicationService / DeviceProfileService / GatewayService
// methods on this same struct so the chirpstack-api import stays in this
// package alone.
type Client struct {
	conn *grpc.ClientConn
}

// NewClient wraps an existing *grpc.ClientConn (typically obtained from Dial)
// in the high-level Client facade. The Client takes ownership of the conn —
// callers MUST NOT call conn.Close() directly after wrapping; use Client.Close.
func NewClient(conn *grpc.ClientConn) *Client { return &Client{conn: conn} }

// Conn returns the underlying *grpc.ClientConn for callers that need to drive
// a generated stub directly (e.g. ProbeVersion, which lives outside Client to
// allow probing against a bare conn before it is wrapped).
func (c *Client) Conn() *grpc.ClientConn { return c.conn }

// Close releases the underlying gRPC channel. Idempotent at the grpc level.
func (c *Client) Close() error { return c.conn.Close() }
