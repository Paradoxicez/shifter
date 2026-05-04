// Quality flag values for the measurement.quality column.
//
// MUST exactly match the CHECK constraint pinned in migration 0015_measurement
// (D-26). Call sites use these constants instead of string literals so a
// future migration that changes the vocabulary surfaces here as a Go-compile
// failure rather than a silent CHECK violation at INSERT time.
//
// The five legal values:
//
//   - QualityOK                — happy-path decode + normalize succeeded.
//   - QualityDecodeFail        — JSON parse / base64 / fundamental shape error.
//     Raw payload is still persisted; canonical columns are NULL.
//   - QualityMissingCanonical  — decode succeeded, but no mapping row populated
//     a canonical column (e.g. unbound device, codec returned empty object).
//   - QualityOutOfRange        — coerced canonical value violated a sanity range
//     (e.g. battery_pct > 100); coerce policy may null the value but row persists.
//   - QualityDuplicateFcnt     — observed (dev_eui, fcnt) replay; reserved for
//     Phase 6 ops monitoring (Phase 2 doesn't ship dedupe detection — ChirpStack
//     v4 dedupes server-side, RESEARCH §Security).
package ingest

const (
	QualityOK               = "ok"
	QualityDecodeFail       = "decode_fail"
	QualityMissingCanonical = "missing_canonical"
	QualityOutOfRange       = "out_of_range"
	QualityDuplicateFcnt    = "duplicate_fcnt"
)
