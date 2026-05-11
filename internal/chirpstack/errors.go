package chirpstack

import (
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrChirpStackV3OrUnknown is returned by ProbeVersion when the server doesn't
// implement InternalService.GetVersion — the canonical signal for "not v4".
//
// Per INST-05 (the v3 rejection requirement), the install wizard (Plan 14/15)
// and `serve` startup (Plan 18) MUST treat this sentinel as a hard refusal:
// v3 and v4 share gRPC ports and superficial proto names, but v3's MQTT
// uplink event payloads are incompatible with Shifter's canonical schema, so
// dialing succeeds while the rest of the system silently misbehaves
// (PITFALLS §7). The version probe is the only reliable distinguisher.
var ErrChirpStackV3OrUnknown = errors.New("chirpstack: server is v3 or non-ChirpStack — Shifter requires v4")

// ErrNotFound is the typed sentinel every wrapper in this package emits when
// the underlying gRPC call returns codes.NotFound. Handlers compare with
// errors.Is(err, ErrNotFound) to decide between 404 / 409 / continuing past a
// missing-but-expected resource (e.g. GetDeviceKeys on an ABP-only device →
// 409 no_credentials per D-27).
//
// Other gRPC error codes flow through unwrapped so callers retain status-code
// information for 5xx mapping (codes.Unavailable / codes.Unknown / DeadlineExceeded
// are the typical 502 candidates — see PITFALLS §3).
var ErrNotFound = errors.New("chirpstack: not found")

// wrapCSErr translates a gRPC error returned from a ChirpStack RPC into the
// canonical Shifter shape:
//
//   - nil                       → nil
//   - codes.NotFound            → ErrNotFound (wrapped so the original status
//                                  message is preserved for log diagnostics)
//   - any other status / error  → fmt.Errorf-wrapped error preserving the
//                                  original cause for upstream classification
//
// Wrappers in this package call wrapCSErr unconditionally on the err return of
// every csClient.XYZ call so all CS surface presents the same error contract.
func wrapCSErr(err error) error {
	if err == nil {
		return nil
	}
	if st, ok := status.FromError(err); ok && st.Code() == codes.NotFound {
		return fmt.Errorf("%w: %s", ErrNotFound, st.Message())
	}
	return fmt.Errorf("chirpstack: %w", err)
}
