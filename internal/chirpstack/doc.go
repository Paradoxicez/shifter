// Package chirpstack wraps the upstream ChirpStack v4 gRPC API plus the MQTT
// subscriber that ingests uplink events. Plan 12 ships the gRPC half:
//
//   - Dial(ctx, cfg)        — opens a *grpc.ClientConn against the v4 control
//     plane with TLS by default and the API token attached as
//     `authorization: Bearer <token>` metadata via a UnaryClientInterceptor.
//   - Client                — high-level wrapper that owns the conn lifecycle
//     and exposes domain methods on top of the generated stubs (Phase 1
//     surface: PingDevices for the CHIRP-01 smoke and Test Connection probe).
//   - ProbeVersion(ctx, conn) — calls InternalService.GetVersion and returns
//     ErrChirpStackV3OrUnknown when the server is v3 or non-ChirpStack.
//     INST-05 — Plan 14/15 (wizard) and Plan 18 (`serve` startup) MUST refuse
//     to proceed when this sentinel is returned.
//
// Plan 13 will land the MQTT subscriber alongside this gRPC surface.
//
// Architectural seam: this package is the SOLE importer of
// github.com/chirpstack/chirpstack/api/go/v4 in production code. The shared
// in-process bufconn mock at internal/testsupport/chirpstack_mock.go also
// imports the v4 protos because it has to register the simulated service
// servers — that is the only sanctioned exception. All other packages MUST
// consume ChirpStack via Dial + Client + ProbeVersion only.
package chirpstack
