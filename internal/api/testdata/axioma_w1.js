// Axioma Qalcosonic W1 LoRaWAN F1 V1.8 Enhanced — payload decoder.
//
// Reference: 02-RESEARCH.md §"Vendor profile sources" + Node-RED gist
//   https://gist.github.com/Alkarex/4b5d1fef2ff84d483e2793ed009ef607
//
// Wire format (fPort=100):
//   Byte  0     : status flags (bitmask)
//   Bytes 1-4   : log time, UTC unix-32, little-endian
//   Bytes 5-8   : cumulative volume in liters, signed int32, little-endian
//   Byte  9     : battery (0..100, percent — vendor docs say "% × 2 capped at 200"
//                 but in practice the field is a direct 0..100 value; we cap to 100
//                 defensively in case a meter mis-reports)
//   Byte  10    : temperature in °C, signed int8
//   Bytes 11..  : (extended packet only) 15 historical readings — Phase 2 ingest
//                 pipeline does NOT consume the historical block; flow_rate is
//                 derived from successive cumulative_l deltas in normalize.go.
//
// CONTEXT D-09 + Pitfalls §3 — this codec runs INSIDE ChirpStack's QuickJS
// sandbox; Shifter never executes it. The decoded `data` object becomes the
// uplink event's `object` field which the ingest pipeline consumes.

function decodeUplink(input) {
    var bytes = input.bytes;
    var fPort = input.fPort;

    if (fPort !== 100) {
        return {
            data: {},
            errors: ["unexpected fPort " + fPort + ", expected 100"],
            warnings: []
        };
    }
    if (!bytes || bytes.length < 9) {
        return {
            data: {},
            errors: ["payload too short: need >= 9 bytes, got " + (bytes ? bytes.length : 0)],
            warnings: []
        };
    }

    var result = {};
    var warnings = [];

    try {
        // Status byte (byte 0) — bit-flag decode.
        var status = bytes[0];
        result.tamper          = (status & 0x02) !== 0;
        result.battery_low     = (status & 0x04) !== 0;
        result.leak            = (status & 0x08) !== 0;
        result.permanent_error = (status & 0x10) !== 0;
        result.temporary_error = (status & 0x20) !== 0;
        result.empty_pipe      = (status & 0x40) !== 0;
        result.reverse_flow    = (status & 0x80) !== 0;

        // Bytes 1-4: log time (UTC unix-32 LE).
        result.log_time_unix =
            (bytes[1]) |
            (bytes[2] << 8) |
            (bytes[3] << 16) |
            ((bytes[4] << 24) >>> 0);

        // Bytes 5-8: cumulative volume in liters (signed int32 LE).
        var litersRaw =
            (bytes[5]) |
            (bytes[6] << 8) |
            (bytes[7] << 16) |
            (bytes[8] << 24);
        // Treat as unsigned for the canonical cumulative value — meter counters
        // are monotonically increasing, the high bit only flips during 32-bit
        // rollover which the rollover detector (D-05) handles at ingest time.
        var litersUnsigned = litersRaw >>> 0;
        result.cumulative_l  = litersUnsigned;
        result.cumulative_m3 = litersUnsigned / 1000.0;

        // Byte 9: battery percent. Cap at 100 — some firmware revisions emit
        // 0..200 ("% × 2") which would render >100% in the dashboard.
        if (bytes.length >= 10) {
            var batt = bytes[9];
            if (batt > 100) {
                warnings.push("battery byte " + batt + " >100; clamped (vendor firmware reports % × 2)");
                batt = Math.min(100, Math.floor(batt / 2));
            }
            result.battery_pct = batt;
        }

        // Byte 10: temperature in °C (signed int8).
        if (bytes.length >= 11) {
            var t = bytes[10];
            if (t > 127) { t -= 256; }
            result.temperature_c = t;
        }

        return { data: result, warnings: warnings, errors: [] };
    } catch (e) {
        return {
            data: { _raw_hex: bytesToHex(bytes) },
            errors: ["decode failed: " + (e && e.message ? e.message : String(e))],
            warnings: warnings
        };
    }
}

function bytesToHex(b) {
    var s = "";
    for (var i = 0; i < b.length; i++) {
        var h = (b[i] & 0xff).toString(16);
        s += (h.length === 1 ? "0" : "") + h;
    }
    return s;
}
