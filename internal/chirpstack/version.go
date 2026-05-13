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

// ProbeVersion calls InternalService.GetVersion to detect v3-vs-v4, then
// validates the API token via TenantService.List (an auth-required RPC).
//
// Two-step design:
//  1. GetVersion — unauthenticated; returns ErrChirpStackV3OrUnknown on
//     Unimplemented/NotFound (INST-05).
//  2. TenantService.List with limit=1 — validates the API token.
//     Unauthenticated or PermissionDenied → ErrInvalidAPIToken.
//
// This separates version detection from token validation, both of which the
// wizard step 2 needs to confirm before persisting the connection.
func ProbeVersion(ctx context.Context, conn *grpc.ClientConn) (string, error) {
	// Step 1: version detection (unauthenticated).
	internalClient := api.NewInternalServiceClient(conn)
	resp, err := internalClient.GetVersion(ctx, &emptypb.Empty{})
	if err != nil {
		st, ok := status.FromError(err)
		if ok && (st.Code() == codes.Unimplemented || st.Code() == codes.NotFound) {
			return "", ErrChirpStackV3OrUnknown
		}
		return "", fmt.Errorf("chirpstack version probe: %w", err)
	}
	if resp.GetVersion() == "" {
		return "", ErrChirpStackV3OrUnknown
	}
	version := resp.GetVersion()

	// Step 2: token validation via an auth-required RPC.
	tenantClient := api.NewTenantServiceClient(conn)
	_, err = tenantClient.List(ctx, &api.ListTenantsRequest{Limit: 1})
	if err != nil {
		st, ok := status.FromError(err)
		if ok && (st.Code() == codes.Unauthenticated || st.Code() == codes.PermissionDenied) {
			return "", ErrInvalidAPIToken
		}
		// Any other error (Unavailable, DeadlineExceeded, etc.) — treat as
		// unreachable so caller maps it to grpc_unreachable.
		return "", fmt.Errorf("chirpstack token validation: %w", err)
	}

	return version, nil
}
