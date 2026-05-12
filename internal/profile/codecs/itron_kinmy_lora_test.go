package codecs_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/codec_runner"
	"github.com/shifter-io/shifter/internal/ingest"
	"github.com/shifter-io/shifter/internal/profile/codecs"
)

// TestItronKinmyLoRa_GoldenVector exercises the SOF=0x6F frame format
// against a constructed 28-byte payload that exercises all wire-format fields.
//
// Wire format (from itron_kinmy_lora.js):
//
//	Byte  0     : SOF = 0x6F
//	Bytes 1-7   : meter ID (reversed for hex string)
//	Bytes 8-11  : forward_flow u32 LE, raw liters → ÷1000 = m³
//	Bytes 12-15 : reverse_flow u32 LE, raw liters
//	Byte  16    : reserved
//	Byte  17    : seconds
//	Byte  18    : minutes
//	Byte  19    : hours
//	Byte  20    : day
//	Byte  21    : month
//	Bytes 22-23 : year u16 LE
//	Byte  24    : tamper flag (bit 3 = 0x08)
//	Byte  25    : reserved
//	Byte  26    : leak flag (bit 3 = 0x08)
//	Byte  27    : battery voltage ÷10 (0x24=36 → 3.6V)
//
// Constructed payload:
//   - forward_flow = 1000 liters (0xE8, 0x03, 0x00, 0x00 LE) → 1.0 m³
//   - reverse_flow = 100 liters  (0x64, 0x00, 0x00, 0x00 LE) → 0.1 m³
//   - battery byte = 0x24 = 36 → 3.6V → li_socl2_3v6 curve → 100%
func TestItronKinmyLoRa_GoldenVector(t *testing.T) {
	// 28-byte frame constructed per the itron_kinmy_lora.js wire format spec.
	payload := []byte{
		0x6F,                                       // byte 0:  SOF
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,  // bytes 1-7: meter ID
		0xE8, 0x03, 0x00, 0x00,                     // bytes 8-11:  forward_flow = 1000 L LE → 1.0 m³
		0x64, 0x00, 0x00, 0x00,                     // bytes 12-15: reverse_flow = 100 L LE  → 0.1 m³
		0x00,                                       // byte 16: reserved
		0x1E,                                       // byte 17: seconds = 30
		0x0A,                                       // byte 18: minutes = 10
		0x0C,                                       // byte 19: hours   = 12
		0x0F,                                       // byte 20: day     = 15
		0x06,                                       // byte 21: month   = 6
		0xE8, 0x07,                                 // bytes 22-23: year = 2024 LE (0x07E8)
		0x00,                                       // byte 24: status (no tamper)
		0x00,                                       // byte 25: reserved
		0x00,                                       // byte 26: leak byte (no leak)
		0x24,                                       // byte 27: battery = 0x24=36 → 3.6V
	}
	require.Len(t, payload, 28, "golden vector must be exactly 28 bytes")

	// Load the codec JS from the embedded profile.
	codecJS := codecs.CodecBySlug("itron_kinmy_lora")
	require.NotEmpty(t, codecJS, "CodecBySlug must return non-empty JS for itron_kinmy_lora")

	// Run the codec.
	result := codec_runner.RunCodecTest(codecJS, payload, 1)
	require.Empty(t, result.ErrorMessage,
		"codec decode must not error; stack: %s", result.ErrorStack)
	require.NotNil(t, result.DecodedJSON, "DecodedJSON must be populated")

	// Assert all required fields are present.
	require.Contains(t, result.DecodedJSON, "forward_flow_m3", "must have forward_flow_m3")
	require.Contains(t, result.DecodedJSON, "reverse_flow_m3", "must have reverse_flow_m3")
	require.Contains(t, result.DecodedJSON, "battery_v", "must have battery_v")
	require.Contains(t, result.DecodedJSON, "meter_id", "must have meter_id")

	// Assert decoded values match the constructed payload.
	fwdM3, ok := result.DecodedJSON["forward_flow_m3"].(float64)
	require.True(t, ok, "forward_flow_m3 must be float64")
	require.InDelta(t, 1.0, fwdM3, 1e-6, "forward_flow_m3 should be 1.0 m³")

	revM3, ok := result.DecodedJSON["reverse_flow_m3"].(float64)
	require.True(t, ok, "reverse_flow_m3 must be float64")
	require.InDelta(t, 0.1, revM3, 1e-6, "reverse_flow_m3 should be 0.1 m³")

	battV, ok := result.DecodedJSON["battery_v"].(float64)
	require.True(t, ok, "battery_v must be float64")
	require.InDelta(t, 3.6, battV, 1e-6, "battery_v should be 3.6V (0x24/10)")

	// Assert meter_id is a non-empty string.
	meterID, ok := result.DecodedJSON["meter_id"].(string)
	require.True(t, ok, "meter_id must be string")
	require.NotEmpty(t, meterID, "meter_id must not be empty")

	// Battery curve application — 3.6V via li_socl2_3v6 → 100%.
	pct, curveOK := ingest.ApplyBatteryCurve("li_socl2_3v6", battV)
	require.True(t, curveOK, "li_socl2_3v6 curve must return ok=true")
	require.Equal(t, int16(100), pct, "3.6V on li_socl2_3v6 should be 100%%")
	require.GreaterOrEqual(t, pct, int16(0))
	require.LessOrEqual(t, pct, int16(100))
}
