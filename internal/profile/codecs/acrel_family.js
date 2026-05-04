// Acrel ADL200 (1-phase) + ADW300 (3-phase) shared LoRaWAN codec.
//
// Pitfall 6 + 02-RESEARCH.md §"Vendor profile sources" — both meters are
// part of the same Acrel family and emit a Modbus-derived register-pair
// payload. ONE codec_js file, TWO device_profile rows (acrel_adl200 +
// acrel_adw300), TWO mapping subsets (ADL200's mapping references only the
// fields it cares about; ADW300's mapping consumes the superset).
//
// Reference: Acrel ADW300 user manual (linked in 02-RESEARCH.md). Payload
// frame layout (per manual §4 communication protocol):
//   Byte  0     : frame type (1 byte) — 0x03 for "real-time data" frames
//   Byte  1     : register pair count N (1 byte)
//   Bytes 2..   : N × (reg_addr u16 LE + value u32 LE) = N × 6 bytes
//
// Common register addresses (per manual table 4-3 — exact byte layout to be
// verified against the production-firmware manual revision; unknown registers
// fall through to result._unknown so operators can promote them via the
// mapping editor without re-shipping the codec).
//
// CONTEXT D-09 + Pitfalls §3 — runs INSIDE ChirpStack's QuickJS sandbox.

function decodeUplink(input) {
    var bytes = input.bytes;
    var result = {};

    if (!bytes || bytes.length < 2) {
        return { data: {}, errors: ["payload too short: need >= 2 bytes for header"], warnings: [] };
    }

    try {
        var frameType = bytes[0];
        var regCount  = bytes[1];
        var expectedLen = 2 + regCount * 6;
        if (bytes.length < expectedLen) {
            return {
                data: {},
                errors: ["truncated frame: expected " + expectedLen + " bytes for " + regCount + " registers, got " + bytes.length],
                warnings: []
            };
        }
        result._frame_type = frameType;
        result._reg_count  = regCount;

        for (var i = 0; i < regCount; i++) {
            var off = 2 + i * 6;
            var reg = bytes[off] | (bytes[off + 1] << 8);
            var valU32 =
                (bytes[off + 2]) |
                (bytes[off + 3] << 8) |
                (bytes[off + 4] << 16) |
                ((bytes[off + 5] << 24) >>> 0);
            applyRegister(reg, valU32, result);
        }

        // Convenience: if the codec saw L1+L2+L3 voltages, expose a multi_phase
        // flag so the dashboard auto-renders a 3-phase view without operator
        // having to set capabilities.
        if (result.voltage_l1 != null && result.voltage_l2 != null && result.voltage_l3 != null) {
            result.multi_phase = true;
        }

        return { data: result, warnings: [], errors: [] };
    } catch (e) {
        return { data: {}, errors: ["decode failed: " + (e && e.message ? e.message : String(e))], warnings: [] };
    }
}

function applyRegister(reg, val, out) {
    switch (reg) {
        // --- energy ---
        case 0x0048: out.kwh_forward = val * 0.01; break;       // total active forward kWh (×0.01)
        case 0x004A: out.kwh_reverse = val * 0.01; break;       // total active reverse kWh (×0.01)

        // --- power ---
        case 0x0040: out.power_total_w = val; break;            // active power (W)
        case 0x0042: out.power_reactive_var = val; break;       // reactive power (var)

        // --- L1 ---
        case 0x0050: out.voltage_l1 = val * 0.1; break;         // V (×0.1)
        case 0x0052: out.current_l1 = val * 0.001; break;       // A (×0.001)
        case 0x0054: out.pf_l1 = signedShort(val) * 0.001; break;

        // --- L2 (ADW300 only) ---
        case 0x0058: out.voltage_l2 = val * 0.1; break;
        case 0x005A: out.current_l2 = val * 0.001; break;
        case 0x005C: out.pf_l2 = signedShort(val) * 0.001; break;

        // --- L3 (ADW300 only) ---
        case 0x0060: out.voltage_l3 = val * 0.1; break;
        case 0x0062: out.current_l3 = val * 0.001; break;
        case 0x0064: out.pf_l3 = signedShort(val) * 0.001; break;

        // --- frequency / pq ---
        case 0x0070: out.frequency_hz = val * 0.01; break;
        case 0x0072: out.thd_voltage = val * 0.01; break;
        case 0x0074: out.thd_current = val * 0.01; break;

        // --- diagnostics ---
        case 0x00B6: out.battery_pct = Math.min(100, val); break;
        case 0x00C0: {
            // signed int8 °C in low byte
            var t = val & 0xff;
            if (t > 127) { t -= 256; }
            out.temperature_c = t;
            break;
        }

        // Unknown — surface for operator-driven promotion via the mapping editor.
        default: {
            if (!out._unknown) { out._unknown = {}; }
            out._unknown["0x" + padHex4(reg)] = val;
        }
    }
}

// signedShort treats the low 16 bits of val as a signed two's-complement
// integer. Used for power-factor registers which can range -1.000 .. 1.000.
function signedShort(val) {
    var v = val & 0xffff;
    if (v > 0x7fff) { v -= 0x10000; }
    return v;
}

function padHex4(n) {
    var h = (n & 0xffff).toString(16);
    while (h.length < 4) { h = "0" + h; }
    return h;
}
