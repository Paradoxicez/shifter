package codec_runner_test

import (
	_ "embed"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/codec_runner"
)

//go:embed testdata/axioma_w1.js
var axiomaW1JS string

//go:embed testdata/itron_kinmy_lora.js
var itronKinmyJS string

// TestRunCodecTest_AxiomaW1 decodes a known Axioma W1 hex sample.
// Hex: fPort=100, 11 bytes:
//   0x00 = status (no flags)
//   0x01 0x02 0x03 0x04 = log_time_unix (little-endian: 0x04030201)
//   0xE8 0x03 0x00 0x00 = cumulative_l = 1000 liters (little-endian)
//   0x4B = battery_pct = 75
//   0x18 = temperature_c = 24°C
func TestRunCodecTest_AxiomaW1(t *testing.T) {
	hexBytes := []byte{
		0x00,                   // status
		0x01, 0x02, 0x03, 0x04, // log_time_unix LE
		0xE8, 0x03, 0x00, 0x00, // cumulative_l = 1000 LE
		0x4B,                   // battery_pct = 75
		0x18,                   // temperature_c = 24
	}
	result := codec_runner.RunCodecTest(axiomaW1JS, hexBytes, 100)

	require.Empty(t, result.ErrorMessage, "axioma decode should not error: %s / stack: %s", result.ErrorMessage, result.ErrorStack)
	require.NotNil(t, result.DecodedJSON, "DecodedJSON should be set")
	require.Contains(t, result.DecodedJSON, "cumulative_l", "decoded must have cumulative_l")
	require.Contains(t, result.DecodedJSON, "battery_pct", "decoded must have battery_pct")
}

// TestRunCodecTest_ItronKinmy decodes a known 28-byte Itron+KINMY SOF=0x6F sample.
// Constructed per the itron_kinmy_lora.js wire format.
func TestRunCodecTest_ItronKinmy(t *testing.T) {
	// 28-byte frame: SOF=0x6F, meterID bytes 1-7, fwd flow (bytes 8-11 LE),
	// rev flow (bytes 12-15 LE), reserved, time(s,m,h), date(d,m), year LE,
	// status byte 24, reserved, leak byte 26, battery byte 27
	hexBytes := []byte{
		0x6F,                   // SOF
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, // meter ID bytes 1-7
		0xE8, 0x03, 0x00, 0x00, // forward_flow = 1000 liters LE → 1.0 m³
		0x00, 0x00, 0x00, 0x00, // reverse_flow = 0
		0x00,                   // reserved byte 16
		0x1E,                   // seconds = 30
		0x0A,                   // minutes = 10
		0x0C,                   // hours = 12
		0x0F,                   // day = 15
		0x06,                   // month = 6
		0xE8, 0x07,             // year = 2024 LE (0x07E8)
		0x00,                   // status byte 24 (no tamper)
		0x00,                   // reserved byte 25
		0x00,                   // leak byte 26 (no leak)
		0x24,                   // battery = 0x24 = 36 → 3.6V
	}
	result := codec_runner.RunCodecTest(itronKinmyJS, hexBytes, 1)

	require.Empty(t, result.ErrorMessage, "itron decode should not error: %s", result.ErrorMessage)
	require.NotNil(t, result.DecodedJSON, "DecodedJSON should be set")
	require.Contains(t, result.DecodedJSON, "forward_flow_m3", "decoded must have forward_flow_m3")
	require.Contains(t, result.DecodedJSON, "meter_id", "decoded must have meter_id")
	require.Contains(t, result.DecodedJSON, "battery_v", "decoded must have battery_v")
}

// TestRunCodecTest_Timeout verifies that an infinite-loop codec returns within
// 250ms with an error containing "timeout" or "interrupted".
func TestRunCodecTest_Timeout(t *testing.T) {
	src := `function decodeUplink(input) { while(true) {} }`
	start := time.Now()
	res := codec_runner.RunCodecTest(src, []byte{0x01}, 1)
	elapsed := time.Since(start)

	require.Less(t, elapsed, 250*time.Millisecond, "timeout codec must return within 250ms, took %s", elapsed)
	require.True(t,
		strings.Contains(res.ErrorMessage, "timeout") || strings.Contains(res.ErrorMessage, "interrupted"),
		"error message must mention timeout/interrupted, got: %q", res.ErrorMessage,
	)
}

// TestRunCodecTest_SyntaxError verifies that a broken codec produces ErrorLine and ErrorCol > 0.
func TestRunCodecTest_SyntaxError(t *testing.T) {
	src := `function decodeUplink({ // broken syntax`
	res := codec_runner.RunCodecTest(src, []byte{0x01}, 1)

	require.NotEmpty(t, res.ErrorMessage, "syntax error must produce an error message")
	require.True(t, res.ErrorLine > 0 || res.ErrorCol > 0,
		"syntax error must populate ErrorLine or ErrorCol, got line=%d col=%d message=%q",
		res.ErrorLine, res.ErrorCol, res.ErrorMessage)
}
