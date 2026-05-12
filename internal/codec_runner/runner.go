// Package codec_runner executes vendor codec JS via the local goja runtime
// for the device-profile editor test-runner panel. D-26, D-27, D-28.
//
// Security: this package is the only HIGH-severity security surface in
// Phase 7 — operator-supplied JS execution. All mitigations are applied:
//
//   - T-07-07-01: 100ms interrupt timeout (AfterFunc + Interrupt)
//   - T-07-07-02: SetMaxCallStackSize(500) prevents native stack overflow
//   - T-07-07-03: newSandboxRuntime() exposes no Go stdlib bindings
//   - T-07-07-06: payload capped at 256 bytes before VM invocation
//   - T-07-07-09: fresh *goja.Runtime per call — no shared state
package codec_runner

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/dop251/goja"
)

const (
	maxPayloadBytes    = 256
	executionTimeoutMS = 100
	maxCallStackFrames = 500
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

// RunCodecTest executes decodeUplink({bytes, fPort}) inside a fresh goja
// sandbox with a 100ms timeout and 16MB (500-frame) call stack cap.
//
// Errors are surfaced via TestResult fields — this function never returns a
// Go error.
func RunCodecTest(codecJS string, hexBytes []byte, fPort int) TestResult {
	// T-07-07-06: cap payload size before any VM involvement.
	if len(hexBytes) > maxPayloadBytes {
		return TestResult{ErrorMessage: fmt.Sprintf("payload too large: max %d bytes", maxPayloadBytes)}
	}

	// T-07-07-09: fresh runtime per call — no shared mutable state.
	vm := newSandboxRuntime()

	// T-07-07-01: schedule interrupt after 100ms.
	timer := time.AfterFunc(executionTimeoutMS*time.Millisecond, func() {
		vm.Interrupt("codec timeout exceeded 100ms")
	})
	defer timer.Stop()

	// Compile + execute codec source. Syntax errors and early runtime errors
	// are caught here.
	if _, err := vm.RunString(codecJS); err != nil {
		return gojaErrToResult(err)
	}

	fn, ok := goja.AssertFunction(vm.Get("decodeUplink"))
	if !ok {
		return TestResult{ErrorMessage: "decodeUplink is not a function"}
	}

	// Build input {bytes: [...int...], fPort: N}.
	// Per Pitfall §7: pass []any of ints (not []byte) so JS sees numeric array
	// rather than a Uint8Array or Go-opaque value.
	bytesArr := make([]any, len(hexBytes))
	for i, b := range hexBytes {
		bytesArr[i] = int(b)
	}
	inputObj := vm.NewObject()
	_ = inputObj.Set("bytes", vm.ToValue(bytesArr))
	_ = inputObj.Set("fPort", fPort)

	result, err := fn(goja.Undefined(), inputObj)
	if err != nil {
		return gojaErrToResult(err)
	}

	exported := result.Export()
	// ChirpStack codec return shape: {data, errors, warnings}. Extract .data.
	if m, ok := exported.(map[string]any); ok {
		if data, ok := m["data"].(map[string]any); ok {
			return TestResult{DecodedJSON: data}
		}
	}
	// Fallback: codec returned something else — marshal/unmarshal to map.
	decoded := map[string]any{}
	if asJSON, jerr := json.Marshal(exported); jerr == nil {
		_ = json.Unmarshal(asJSON, &decoded)
	}
	return TestResult{DecodedJSON: decoded}
}

// stackLocationRE matches "at <name>:<line>:<col>" in goja runtime error stacks.
var stackLocationRE = regexp.MustCompile(`at .*?:(\d+):(\d+)`)

// syntaxLocationRE matches "Line N:M" in goja syntax error messages.
// goja reports syntax errors as "SyntaxError: ...: Line 1:41 Unexpected ..."
var syntaxLocationRE = regexp.MustCompile(`Line (\d+):(\d+)`)

// gojaErrToResult converts a goja runtime or interrupt error into a TestResult
// with ErrorMessage, ErrorStack, ErrorLine, ErrorCol populated.
func gojaErrToResult(err error) TestResult {
	if ex, ok := err.(*goja.Exception); ok {
		stack := ex.String()
		msg := ex.Value().String()
		res := TestResult{
			ErrorMessage: msg,
			ErrorStack:   stack,
		}
		// Try runtime stack format first ("at ...:line:col"), then syntax
		// error format ("Line N:M") which goja uses for parse failures.
		if m := stackLocationRE.FindStringSubmatch(stack); len(m) == 3 {
			res.ErrorLine, _ = strconv.Atoi(m[1])
			res.ErrorCol, _ = strconv.Atoi(m[2])
		} else if m := syntaxLocationRE.FindStringSubmatch(msg); len(m) == 3 {
			res.ErrorLine, _ = strconv.Atoi(m[1])
			res.ErrorCol, _ = strconv.Atoi(m[2])
		}
		return res
	}
	if iv, ok := err.(*goja.InterruptedError); ok {
		return TestResult{ErrorMessage: fmt.Sprintf("interrupted: %v", iv.Value())}
	}
	return TestResult{ErrorMessage: err.Error()}
}
