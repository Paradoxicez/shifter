// Package logging emits Shifter's structured operational logs (D-24, D-25).
//
// Output is JSON, one event per line, written to stdout so the Docker
// `json-file` driver captures it under the operator's configured size +
// rotation caps. Per D-25 only two levels are exposed: `info` (default) and
// `debug` (toggled via `SHIFTER_LOG_LEVEL=debug`); anything else falls back
// to `info`. PITFALL #9 (slog pretty-printing) is defused by relying on
// slog.NewJSONHandler's default single-line JSON contract.
package logging
