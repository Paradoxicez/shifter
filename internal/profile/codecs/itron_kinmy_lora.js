// Itron + KINMY LoRaWAN module — KINMY 3rd-party LoRa module attached to Itron water meters.
// catalog slug: itron_kinmy_lora; vendor: Itron; family: KINMY LoRa Module.
//
// Source: operator-supplied codec (Phase 7 catalog seed). Adapted to Phase 2
// codec convention: ES5 syntax, ChirpStack v4 codec return shape
// `{data, errors, warnings}`, defensive bytes-length check, try/catch wrapper.
//
// Wire format (28 bytes; fPort TBD — see Phase 7 catalog TODO):
//   Byte  0     : SOF (Start-Of-Frame), expected 0x6F
//   Bytes 1-7   : meter ID, 7 bytes, big-endian when reversed (hex string)
//   Bytes 8-11  : forward flow, u32 LE, raw is liters (÷1000 → m³)
//   Bytes 12-15 : reverse flow, u32 LE, raw is liters (÷1000 → m³)
//   Byte  16    : (reserved / unknown)
//   Byte  17    : meter time — seconds (BCD-like, plain decimal value)
//   Byte  18    : meter time — minutes
//   Byte  19    : meter time — hours
//   Byte  20    : meter date — day
//   Byte  21    : meter date — month
//   Bytes 22-23 : meter date — year, u16 LE
//   Byte  24    : status — bit 3 (0x08) = tamper
//   Byte  25    : (reserved / unknown)
//   Byte  26    : status — bit 3 (0x08) = leakage
//   Byte  27    : battery voltage ÷10 (e.g., 0x24 = 36 → 3.6 V)
//
// Uplink cadence: default 1 uplink / day per module; configurable in vendor app
// to more frequent. Modules ship factory-randomized so the fleet does NOT all
// transmit at the same second — random staggering across the day. Implications
// captured in Phase 7 D-NN (offline threshold, anomaly cold-start window).
//
// CONTEXT D-09 + Pitfalls §3 — runs INSIDE ChirpStack's QuickJS sandbox; the
// decoded `data` object becomes the uplink event's `object` field which the
// Shifter ingest pipeline consumes (battVoltage → battery_v canonical, etc.).

function decodeUplink(input) {
    var bytes = input.bytes;
    var fPort = input.fPort;

    if (!bytes || bytes.length < 28) {
        return {
            data: {},
            errors: ["payload too short: need >= 28 bytes, got " + (bytes ? bytes.length : 0)],
            warnings: []
        };
    }
    if (bytes[0] !== 0x6F) {
        return {
            data: {},
            errors: ["invalid frame: missing SOF 0x6F, got 0x" + bytes[0].toString(16)],
            warnings: []
        };
    }

    var warnings = [];

    try {
        var year = (bytes[23] << 8) | bytes[22];
        var date = year + "-" + pad2(bytes[21]) + "-" + pad2(bytes[20]);
        var time = pad2(bytes[19]) + ":" + pad2(bytes[18]) + ":" + pad2(bytes[17]);

        // Meter ID: bytes 1..7 (7 bytes) — operator-supplied codec reverses then
        // hex-encodes. Preserved verbatim.
        var idBytes = [];
        for (var i = 1; i <= 7; i++) { idBytes.push(bytes[i]); }
        idBytes.reverse();
        var meterId = "";
        for (var j = 0; j < idBytes.length; j++) {
            var hh = (idBytes[j] & 0xff).toString(16);
            meterId += (hh.length === 1 ? "0" : "") + hh;
        }

        // Cumulative volumes — u32 LE, raw is liters; ÷1000 → m³.
        var fwdRaw =
            (bytes[8]) |
            (bytes[9] << 8) |
            (bytes[10] << 16) |
            ((bytes[11] << 24) >>> 0);
        var revRaw =
            (bytes[12]) |
            (bytes[13] << 8) |
            (bytes[14] << 16) |
            ((bytes[15] << 24) >>> 0);

        return {
            data: {
                meter_id:        meterId,
                forward_flow_m3: (fwdRaw >>> 0) / 1000,
                reverse_flow_m3: (revRaw >>> 0) / 1000,
                meter_date:      date,
                meter_time:      time,
                tamper:          (bytes[24] & 0x08) !== 0,
                leak:            (bytes[26] & 0x08) !== 0,
                battery_v:       bytes[27] / 10
            },
            warnings: warnings,
            errors: []
        };
    } catch (e) {
        return {
            data: {},
            errors: ["decode failed: " + (e && e.message ? e.message : String(e))],
            warnings: warnings
        };
    }
}

function pad2(n) {
    var s = (n & 0xff).toString();
    return s.length < 2 ? "0" + s : s;
}
