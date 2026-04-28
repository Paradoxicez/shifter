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

// ProbeVersion calls InternalService.GetVersion(Empty) and returns the version
// string the server reports. This is the ONLY reliable v3-vs-v4 distinguisher
// (per RESEARCH §Pattern 4 / PITFALLS §7) because v3 and v4 share gRPC ports
// and superficial proto names — the gRPC dial succeeds against either, so we
// must call an RPC that exists only on v4.
//
// Errors:
//   - codes.Unimplemented or codes.NotFound from the server → ErrChirpStackV3OrUnknown
//     (the server is either v3, or a non-ChirpStack gRPC endpoint that happens to
//     respond on the same port). Callers (Plan 14 wizard, Plan 18 serve startup)
//     MUST refuse to proceed on this sentinel — INST-05.
//   - any other RPC error → wrapped %w of the underlying status error.
//   - empty version string from a successful response → ErrChirpStackV3OrUnknown
//     (a defensive belt: a misimplemented mock or a malformed v4 response should
//     not silently look like a healthy v4 server).
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
	if resp.GetVersion() == "" {
		return "", ErrChirpStackV3OrUnknown
	}
	return resp.GetVersion(), nil
}
