package doctor

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	api "github.com/chirpstack/chirpstack/api/go/v4/api"
	"google.golang.org/protobuf/types/known/emptypb"
)

// ProbeResult is the result of running a single install probe. Status is one
// of "ok", "warn", or "error". LastRunAt is always set to the time the probe
// function was called (not a cached value — probes are synchronous).
type ProbeResult struct {
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	LastRunAt time.Time `json:"last_run_at"`
}

// ProbeChirpStack dials the ChirpStack gRPC endpoint, calls GetVersion, and
// returns a ProbeResult describing the server's compatibility.
//
// CRITICAL (T-07-14-03 / checker B-3): the API key is NEVER included in
// ProbeResult.Message or ProbeResult.Status under any code path. Error messages
// are constructed from grpcURL host:port and gRPC error classes only.
//
// Status semantics:
//   - "ok"    — ChirpStack v4.10+ detected
//   - "warn"  — ChirpStack v4 detected but < 4.10 (tested against 4.10+)
//   - "error" — v3 detected, unreachable, or auth failure
func ProbeChirpStack(ctx context.Context, grpcURL, apiKey string) ProbeResult {
	result := ProbeResult{
		Name:      "chirpstack",
		LastRunAt: time.Now().UTC(),
	}

	host := safeHostFromGRPCURL(grpcURL)

	// Dial with insecure credentials for probe; grpc.NewClient is non-blocking.
	conn, err := grpc.NewClient(
		grpcURL,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(apiKeyInterceptor(apiKey)),
	)
	if err != nil {
		// DO NOT include apiKey in the message.
		result.Status = "error"
		result.Message = fmt.Sprintf("ChirpStack unreachable at %s: %s", host, errorClass(err))
		return result
	}
	defer conn.Close()

	// Use a context with 5s timeout for the actual RPC.
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	client := api.NewInternalServiceClient(conn)
	resp, err := client.GetVersion(callCtx, &emptypb.Empty{})
	if err != nil {
		// DO NOT include apiKey in the message.
		result.Status = "error"
		result.Message = fmt.Sprintf("ChirpStack GetVersion failed at %s: %s", host, errorClass(err))
		return result
	}

	ver := resp.GetVersion()
	if ver == "" {
		result.Status = "error"
		result.Message = fmt.Sprintf("ChirpStack at %s returned empty version string", host)
		return result
	}

	// Determine status from version string prefix.
	switch {
	case strings.HasPrefix(ver, "3."):
		result.Status = "error"
		result.Message = fmt.Sprintf("ChirpStack v3 detected at %s — Shifter requires v4", host)
	case isV4Before410(ver):
		result.Status = "warn"
		result.Message = fmt.Sprintf("ChirpStack v%s detected at %s — Shifter tested against 4.10+", ver, host)
	default:
		result.Status = "ok"
		result.Message = fmt.Sprintf("ChirpStack v%s detected at %s", ver, host)
	}

	return result
}

// ProbeTimescale checks that the timescaledb extension is installed in the
// connected PostgreSQL database.
//
// Status semantics:
//   - "ok"    — timescaledb extension present
//   - "error" — extension not found or query failed
func ProbeTimescale(ctx context.Context, pool *pgxpool.Pool) ProbeResult {
	result := ProbeResult{
		Name:      "timescale",
		LastRunAt: time.Now().UTC(),
	}

	var extname, extversion string
	err := pool.QueryRow(ctx,
		`SELECT extname, extversion FROM pg_extension WHERE extname = 'timescaledb'`,
	).Scan(&extname, &extversion)
	if err != nil {
		result.Status = "error"
		result.Message = "timescaledb extension not found in pg_extension"
		return result
	}

	result.Status = "ok"
	result.Message = fmt.Sprintf("timescaledb extension present (version %s)", extversion)
	return result
}

// ProbeRegion compares each gateway's region column against the install's
// configured region (chirpstack_connection.region_name). A mismatch means
// some gateways are configured for a different LoRaWAN region than the install.
//
// Status semantics:
//   - "ok"    — all gateway regions match the install region (or no gateways)
//   - "warn"  — one or more gateways have a region != install region
//   - "error" — query failed or no chirpstack_connection row
func ProbeRegion(ctx context.Context, pool *pgxpool.Pool) ProbeResult {
	result := ProbeResult{
		Name:      "region",
		LastRunAt: time.Now().UTC(),
	}

	// Read install region from chirpstack_connection.
	var installRegion string
	err := pool.QueryRow(ctx,
		`SELECT region_name FROM chirpstack_connection WHERE id = 1`,
	).Scan(&installRegion)
	if err != nil {
		result.Status = "error"
		result.Message = "could not read install region from chirpstack_connection"
		return result
	}

	// Count gateways with a different region (exclude archived gateways).
	var mismatchCount int
	err = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM gateway WHERE region != $1 AND archived_at IS NULL`,
		installRegion,
	).Scan(&mismatchCount)
	if err != nil {
		result.Status = "error"
		result.Message = "could not query gateway regions"
		return result
	}

	if mismatchCount > 0 {
		result.Status = "warn"
		result.Message = fmt.Sprintf("%d gateway(s) have a region different from install region %q", mismatchCount, installRegion)
		return result
	}

	result.Status = "ok"
	result.Message = fmt.Sprintf("all gateways match install region %q", installRegion)
	return result
}

// safeHostFromGRPCURL extracts the host:port from a gRPC URL string, stripping
// any scheme or embedded credentials so it is safe to include in error messages.
// Returns the original grpcURL if parsing fails (still no credentials leakage
// since gRPC URLs contain no bearer tokens in the URL itself).
func safeHostFromGRPCURL(grpcURL string) string {
	s := grpcURL
	if !strings.Contains(s, "://") {
		s = "grpc://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return grpcURL
	}
	return u.Host
}

// errorClass returns a safe, non-sensitive string describing the error category.
// It never includes the full error message (which may contain gRPC metadata
// including authorization headers) — only the gRPC status code name or a
// generic category. This is the CRITICAL redaction helper for T-07-14-03.
func errorClass(err error) string {
	if s, ok := status.FromError(err); ok {
		return s.Code().String()
	}
	return "transport_error"
}

// apiKeyInterceptor returns a gRPC UnaryClientInterceptor that injects the API
// key as a Bearer token in outgoing metadata. The key is captured by closure
// and never logged or included in error messages.
func apiKeyInterceptor(apiKey string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any,
		cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if apiKey != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+apiKey)
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// isV4Before410 reports whether the version string is a v4 release before 4.10.
// Version strings are expected in "MAJOR.MINOR.PATCH" format.
func isV4Before410(ver string) bool {
	if !strings.HasPrefix(ver, "4.") {
		return false
	}
	parts := strings.SplitN(ver, ".", 3)
	if len(parts) < 2 {
		return false
	}
	var minor int
	fmt.Sscanf(parts[1], "%d", &minor)
	return minor < 10
}
