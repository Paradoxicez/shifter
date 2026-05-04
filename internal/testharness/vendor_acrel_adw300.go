package testharness

import (
	"encoding/json"
	"time"
)

// BuildAcrelADW300Uplink — emits decoded.object keys per acrel_family.js
// result.* (kwh_forward, power_total_w, voltage_l1..3, current_l1..3,
// pf_l1..3, battery_pct, temperature_c). Pinned per 02-13-PLAN.md
// `<codec_keys_pinned>` table.
//
// For ADL200 1-phase scenarios the harness uses the same builder but the
// ADL200 mapping rows only consume kwh_forward + power_total_w +
// battery_pct (the 3-phase fields are emitted but harmlessly ignored
// because no mapping row references them).
//
// Raw bytes are NOT synthesized for the Acrel codec (the wire format is
// register-pair-based with multiple variations); ingest does not require
// `data` to be a valid frame because it consumes `object` directly.
func BuildAcrelADW300Uplink(
	kwhForward, powerW float64,
	voltL1, voltL2, voltL3 float64,
	currL1, currL2, currL3 float64,
	pfL1, pfL2, pfL3 float64,
	batteryPct int,
	tempC int8,
	fcnt uint32,
	ingestAt time.Time,
) ([]byte, error) {
	obj := map[string]any{
		"kwh_forward":   kwhForward, // reg 0x0048, applyRegister out.kwh_forward (× 0.01 already)
		"power_total_w": powerW,     // reg 0x0040
		"voltage_l1":    voltL1,     // reg 0x0050 (× 0.1 already)
		"current_l1":    currL1,     // reg 0x0052 (× 0.001 already)
		"pf_l1":         pfL1,       // reg 0x0054
		"voltage_l2":    voltL2,     // reg 0x0058
		"current_l2":    currL2,     // reg 0x005A
		"pf_l2":         pfL2,       // reg 0x005C
		"voltage_l3":    voltL3,     // reg 0x0060
		"current_l3":    currL3,     // reg 0x0062
		"pf_l3":         pfL3,       // reg 0x0064
		"battery_pct":   batteryPct, // reg 0x00B6
		"temperature_c": int(tempC), // reg 0x00C0
	}

	event := map[string]any{
		"deviceInfo": map[string]any{
			"devEui":        "acreladw300tes0", // 15 chars: pad to 16 below
			"applicationId": "00000000-0000-0000-0000-000000000000",
		},
		"fCnt":   fcnt,
		"fPort":  1, // acrel codec doesn't gate on fPort
		"data":   "", // decorative; the codec already produced object
		"object": obj,
		"rxInfo": []map[string]any{
			{
				"gatewayId": "gw-test",
				"time":      ingestAt.UTC().Format(time.RFC3339Nano),
				"rssi":      -90,
				"snr":       8.0,
			},
		},
		"time": ingestAt.UTC().Format(time.RFC3339Nano),
	}
	// Normalize the placeholder devEUI to a valid 16-hex lowercase value.
	if di, ok := event["deviceInfo"].(map[string]any); ok {
		di["devEui"] = "acreladw300test0"
	}
	return json.Marshal(event)
}

// BuildAcrelADW300UplinkForDevEUI lets a scenario stamp a specific dev_eui +
// application_id into the produced event. The default builder uses a
// placeholder dev_eui that does not match real seeded fixtures.
func BuildAcrelADW300UplinkForDevEUI(
	devEUI, applicationID string,
	kwhForward, powerW float64,
	voltL1, voltL2, voltL3 float64,
	currL1, currL2, currL3 float64,
	pfL1, pfL2, pfL3 float64,
	batteryPct int,
	tempC int8,
	fcnt uint32,
	ingestAt time.Time,
) ([]byte, error) {
	raw, err := BuildAcrelADW300Uplink(kwhForward, powerW, voltL1, voltL2, voltL3, currL1, currL2, currL3, pfL1, pfL2, pfL3, batteryPct, tempC, fcnt, ingestAt)
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
