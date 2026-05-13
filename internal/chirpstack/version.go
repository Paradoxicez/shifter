package chirpstack

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	api "github.com/chirpstack/chirpstack/api/go/v4/api"
)

// ProbeVersion validates ChirpStack reachability + token authority using
// TenantService.List(limit=1) — the only RPC that both (a) accepts a global
// API key (unlike InternalService.GetVersion which requires a user-session
// JWT in v4.10+) AND (b) returns Unimplemented on v3 servers.
//
// Returns:
//   - ("v4", nil) on success (v4 server + valid global API key)
//   - ErrChirpStackV3OrUnknown if the server is v3 or a non-ChirpStack endpoint
//     (Unimplemented / NotFound)
//   - ErrInvalidAPIToken if the token is rejected (Unauthenticated /
//     PermissionDenied)
//   - wrapped error otherwise (network unreachable / deadline / etc.)
//
// Why no version string: InternalService.GetVersion requires a user JWT, not
// an API key, in v4.10+. We don't have a user session at install time; the
// global API key is what the operator provides. We return the static literal
// "v4" because passing the v3-detection check is itself the v4 confirmation
// (v3 doesn't expose TenantService).
func ProbeVersion(ctx context.Context, conn *grpc.ClientConn) (string, error) {
	tenantClient := api.NewTenantServiceClient(conn)
	_, err := tenantClient.List(ctx, &api.ListTenantsRequest{Limit: 1})
	if err != nil {
		st, ok := status.FromError(err)
		if ok {
			switch st.Code() {
			case codes.Unimplemented, codes.NotFound:
				return "", ErrChirpStackV3OrUnknown
			case codes.Unauthenticated, codes.PermissionDenied:
				return "", ErrInvalidAPIToken
			}
		}
		return "", fmt.Errorf("chirpstack reachability probe: %w", err)
	}
	return "v4", nil
}
