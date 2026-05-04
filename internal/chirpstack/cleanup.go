package chirpstack

import "time"

// defaultCleanupTimeout caps best-effort rollback RPCs (e.g. DeleteDevice on
// CreateDeviceWithKeys failure) so a hung CS server cannot keep the caller
// blocked indefinitely after the original error has already been determined.
// 5s is plenty for a Delete RPC against a healthy server and short enough to
// surface a failing CS instance in operator timing.
const defaultCleanupTimeout = 5 * time.Second
