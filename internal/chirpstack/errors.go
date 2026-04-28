package chirpstack

import "errors"

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
