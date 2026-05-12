package codec_runner_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/codec_runner"
)

// TestSandbox_NoRequire verifies that `require` is undefined inside the sandbox.
func TestSandbox_NoRequire(t *testing.T) {
	src := `function decodeUplink(input) { return { data: { v: typeof require } }; }`
	result := codec_runner.RunCodecTest(src, []byte{0x01}, 1)
	require.Empty(t, result.ErrorMessage, "sandbox should not error: %s", result.ErrorMessage)
	require.Equal(t, "undefined", result.DecodedJSON["v"])
}

// TestSandbox_NoStdlib verifies that os, fs, process, setTimeout, fetch,
// and WebAssembly are all undefined inside the sandbox. T-07-07-03 mitigation.
func TestSandbox_NoStdlib(t *testing.T) {
	for _, name := range []string{"os", "fs", "process", "setTimeout", "fetch", "WebAssembly"} {
		name := name
		t.Run(name, func(t *testing.T) {
			src := `function decodeUplink(input) { return { data: { v: typeof ` + name + ` } }; }`
			result := codec_runner.RunCodecTest(src, []byte{0x01}, 1)
			require.Empty(t, result.ErrorMessage)
			require.Equal(t, "undefined", result.DecodedJSON["v"], "%s should be undefined inside sandbox", name)
		})
	}
}

// TestSandbox_OversizedPayloadRejected verifies that a 257-byte payload is
// rejected before reaching the goja VM. T-07-07-06 mitigation.
func TestSandbox_OversizedPayloadRejected(t *testing.T) {
	bigPayload := make([]byte, 257)
	result := codec_runner.RunCodecTest(`function decodeUplink(){return{};}`, bigPayload, 1)
	require.Contains(t, result.ErrorMessage, "payload too large")
}
