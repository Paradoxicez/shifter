package testsupport

import "testing"

// NewChirpStackMock returns a gRPC server address that simulates ChirpStack and
// an API token to authenticate against it. Plan 12 (chirpstack-grpc) implements
// the body using mockgen-generated InternalServiceServer stubs.
//
// mode:
//   - "v4"   — GetVersion returns a successful response with version "4.x"
//   - "v3"   — GetVersion returns codes.Unimplemented (canonical v3 signature)
//   - "down" — server refuses connection (closed listener)
//
// Returned addr is in `host:port` form, suitable for grpc.NewClient.
func NewChirpStackMock(t *testing.T, mode string) (addr string, apiToken string) {
	t.Helper()
	t.Skip("Plan 12: ChirpStack gRPC mock pending")
	return "", ""
}
