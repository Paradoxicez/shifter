// Package codec_runner executes vendor codec JS via the local goja runtime
// for the device-profile editor test-runner panel. D-26, D-27, D-28.
package codec_runner

import (
	_ "github.com/dop251/goja" // implementation in plan 07-07
)

// TestResult is returned from RunCodecTest. Errors are surfaced inline,
// never as a Go error from this function.
type TestResult struct {
	DecodedJSON      map[string]any `json:"decoded_json,omitempty"`
	CanonicalMapping any            `json:"canonical_mapping,omitempty"`
	ErrorMessage     string         `json:"error_message,omitempty"`
	ErrorLine        int            `json:"error_line,omitempty"`
	ErrorCol         int            `json:"error_col,omitempty"`
	ErrorStack       string         `json:"error_stack,omitempty"`
}

// RunCodecTest is implemented in plan 07-07. Stub returns empty.
func RunCodecTest(codecJS string, hexBytes []byte, fPort int) TestResult {
	return TestResult{ErrorMessage: "not yet implemented (plan 07-07)"}
}
