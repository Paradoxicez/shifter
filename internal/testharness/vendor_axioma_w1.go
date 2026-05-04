package testharness

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"time"
)

// BuildAxiomaW1Uplink returns the v4 ChirpStack event JSON for a synthetic
// Axioma Qalcosonic W1 uplink.
//
// The decoded `object` keys MATCH axioma_w1.js result.* EXACTLY (B2 fix —
// pinned per `<codec_keys_pinned>` table in 02-13-PLAN.md):
//
//	cumulative_l   — uint32 (liters; wraps at 2^32)
//	battery_pct    — int 0..100
//	temperature_c  — int8 (signed)
//	leak           — bool (status bit 0x08)
//	tamper         — bool (status bit 0x02)
//
// The `data` field is a base64-encoded raw frame matching the F1 V1.8 wire
// format (see codec_js source); ChirpStack would normally run the codec to
// produce `object`, but for testharness purposes we embed BOTH so ingest's
// decode.go can consume `object` directly without invoking the codec.
//
// rawCounter wraps at 2^32-1 (matches the Axioma 32-bit liter counter).
func BuildAxiomaW1Uplink(rawCounter uint32, batteryPct uint8, tempC int8, leak, tamper bool, fcnt uint32, ingestAt time.Time) ([]byte, error) {
	// Build the raw frame the codec_js would consume. F1 V1.8 layout:
	//   byte 0    : status flags
	//   bytes 1-4 : log time (UTC unix-32 LE) — zero-filled is fine for tests
	//   bytes 5-8 : cumulative volume liters (LE uint32)
	//   byte 9    : battery percent
	//   byte 10   : temperature signed int8
	rawBytes := make([]byte, 11)
	if tamper {
		rawBytes[0] |= 0x02
	}
	if leak {
		rawBytes[0] |= 0x08
	}
	binary.LittleEndian.PutUint32(rawBytes[5:9], rawCounter)
	rawBytes[9] = batteryPct
	rawBytes[10] = byte(tempC)

	// The decoded `object` keys here MUST match axioma_w1.js result.X.
	// Pinned per `<codec_keys_pinned>` block in 02-13-PLAN.md.
	obj := map[string]any{
		"cumulative_l":  uint64(rawCounter), // result.cumulative_l (axioma_w1.js result.cumulative_l)
		"battery_pct":   int(batteryPct),    // result.battery_pct
		"temperature_c": int(tempC),         // result.temperature_c
		"leak":          leak,               // result.leak
		"tamper":        tamper,             // result.tamper
	}

	event := map[string]any{
		"deviceInfo": map[string]any{
			"devEui":        "axiomaw1testeui0", // 16 hex (lowercase per 0012 CHECK); tests can override
			"applicationId": "00000000-0000-0000-0000-000000000000",
		},
		"fCnt":   fcnt,
		"fPort":  100, // axioma_w1.js requires fPort==100
		"data":   base64.StdEncoding.EncodeToString(rawBytes),
		"object": obj,
		"rxInfo": []map[string]any{
			{
				"gatewayId": "gw-test",
				"time":      ingestAt.UTC().Format(time.RFC3339Nano),
				"rssi":      -85,
				"snr":       9.0,
			},
		},
		"time": ingestAt.UTC().Format(time.RFC3339Nano),
	}
	return json.Marshal(event)
}

// BuildAxiomaW1UplinkForDevEUI is a convenience override of BuildAxiomaW1Uplink
// that lets a scenario stamp a specific dev_eui + application_id into the
// produced event. The default builder uses placeholder values that do not
// match real seeded fixtures.
func BuildAxiomaW1UplinkForDevEUI(devEUI, applicationID string, rawCounter uint32, batteryPct uint8, tempC int8, leak, tamper bool, fcnt uint32, ingestAt time.Time) ([]byte, error) {
	raw, err := BuildAxiomaW1Uplink(rawCounter, batteryPct, tempC, leak, tamper, fcnt, ingestAt)
	if err != nil {
		return nil, err
	}
	var ev map[string]any
	if err := json.Unmarshal(raw, &ev); err != nil {
		return nil, err
	}
	if di, ok := ev["deviceInfo"].(map[string]any); ok {
		di["devEui"] = devEUI
		di["applicationId"] = applicationID
	}
	return json.Marshal(ev)
}
