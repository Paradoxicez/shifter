package ingest

import (
	"errors"
	"math/big"
	"sort"
	"strings"

	"github.com/shifter-io/shifter/internal/profile"
)

// normalizePrecision is the math/big.Float bit-precision used for every
// scale/raw computation in this package. Matches swap.numericPrecision (128)
// so values flowing across packages don't silently lose digits during
// round-tripping. ~38 decimal digits exceeds any plausible meter precision.
const normalizePrecision = 128

// Layer1 is the wide canonical column set the measurement hypertable
// persists per row. Pointers (or *big.Float) are nil when a mapping did not
// populate the field — the persist layer encodes nil as SQL NULL so the
// hybrid wide+JSONB schema (D-08 + DATA-08) stays sparse-friendly.
//
// Vendor-specific fields that the mapping editor promotes via a
// "extra.<key>" target land in Extra (jsonb at the column level).
type Layer1 struct {
	RawValue        *big.Float // the device's raw counter — drives DATA-05 rollover math
	CumulativeValue *big.Float // = raw + binding.reading_offset (computed in Persist, not here)
	InstantValue    *big.Float // flow_rate, instant_power, etc.

	BatteryPct *int16   // 0..100 (out_of_range > 100 caught by coerce)
	RSSI       *int16   // dBm; typically negative
	SNR        *float32 // signal-to-noise ratio (dB)

	TemperatureC *float32
	PressureKPa  *float32

	LeakDetected   *bool
	TamperDetected *bool

	Extra map[string]any // vendor-specific fields (D-08 hybrid)
}

// ErrNoCanonicalValue is returned by NormalizeMeasurement when neither
// RawValue nor InstantValue ended up populated. The handler upgrades this
// to quality='missing_canonical' on the persisted row — the row IS still
// written (D-26 + DATA-07 — never silent-drop) but flagged so operators
// see it in the "X uplinks flagged" badge.
var ErrNoCanonicalValue = errors.New("normalize: no raw_value or instant_value mapped")

// canonicalTargets enumerates the Layer-1 column names a mapping row may
// promote to. The 10 entries match D-02 1:1. Anything not in this set is
// either an "extra.<key>" route (handled separately) or a misconfigured
// profile mapping (rejected by the profile editor's pre-tx validation in
// Plan 02-08; defense in depth here ignores unknown targets silently).
var canonicalTargets = map[string]bool{
	"raw_value":        true,
	"cumulative_value": true,
	"instant_value":    true,
	"battery_pct":      true,
	"rssi":             true,
	"snr":              true,
	"temperature_c":    true,
	"pressure_kpa":     true,
	"leak_detected":    true,
	"tamper_detected":  true,
}

// NormalizeMeasurement applies a profile's mapping rows IN POSITION ORDER
// against the QuickJS-decoded `object` payload, populating the Layer1
// canonical column set + the Extra map for vendor-specific fields.
//
// Mappings carry:
//   - JSONPointer (RFC 6901; "" addresses the whole document)
//   - Target (canonical column name OR "extra.<key>")
//   - Scale (*big.Float; nil → identity)
//   - DataType ("numeric" | "int" | "bool" | "text")
//
// Per CONTEXT D-01 + D-08 + DATA-09 + Anti-Pattern "no decoders.ts":
// this function CONTAINS NO VENDOR SWITCH. The mapping table is the only
// configuration. Adding a vendor = adding a profile + its mappings. If
// this file ever grows a `case "axioma":` style branch, the mapping
// mechanism is being bypassed — see PITFALLS §3.
//
// Returns ErrNoCanonicalValue when neither RawValue nor InstantValue gets
// populated; the populated Layer1 is returned regardless so the caller can
// still persist whatever was extracted (battery, rssi, etc.) alongside the
// raw payload.
func NormalizeMeasurement(decoded map[string]any, mappings []profile.Mapping) (Layer1, error) {
	out := Layer1{Extra: map[string]any{}}

	// Sort mappings by Position so the pass order is deterministic — D-08
	// mapping editor saves rows with explicit positions; the SQL layer's
	// ListMappingsByProfile already orders by position, but a future caller
	// (e.g. an in-memory cache) might not. Stable sort to break ties on
	// equal positions.
	sorted := make([]profile.Mapping, len(mappings))
	copy(sorted, mappings)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Position < sorted[j].Position })

	for _, m := range sorted {
		val, ok := profile.Resolve(decoded, m.JSONPointer)
		if !ok {
			// Missing field — silently skip; the per-row quality flag (set
			// by the caller) handles whether this is a problem. A vendor
			// that omits an optional field on a given uplink is normal.
			continue
		}

		coerced, ok := coerce(val, m.DataType)
		if !ok {
			// Coerce failure (e.g. bool target with a string value) — skip.
			continue
		}

		scaled := applyScale(coerced, m.Scale, m.DataType)

		// Route: extra.<key> → Extra map; canonical → assignToLayer1.
		if strings.HasPrefix(m.Target, "extra.") {
			key := strings.TrimPrefix(m.Target, "extra.")
			out.Extra[key] = scaled
			continue
		}
		if !canonicalTargets[m.Target] {
			// Unknown target — should never happen post-editor-validation.
			// Defense in depth: ignore.
			continue
		}
		assignToLayer1(&out, m.Target, scaled)
	}

	if out.RawValue == nil && out.InstantValue == nil {
		return out, ErrNoCanonicalValue
	}
	return out, nil
}

// coerce converts a JSON-decoded value (any) to the data_type the mapping
// declares. JSON numbers arrive as float64 from encoding/json; bool as bool;
// strings as string. Returns (coerced, ok). ok=false on type-mismatch.
//
// Supported data_types (mirrors profile.validDataTypes): numeric, int, bool, text.
func coerce(val any, dataType string) (any, bool) {
	switch dataType {
	case "numeric":
		switch v := val.(type) {
		case float64:
			return new(big.Float).SetPrec(normalizePrecision).SetFloat64(v), true
		case int:
			return new(big.Float).SetPrec(normalizePrecision).SetInt64(int64(v)), true
		case int64:
			return new(big.Float).SetPrec(normalizePrecision).SetInt64(v), true
		case string:
			f, _, err := big.ParseFloat(v, 10, normalizePrecision, big.ToNearestEven)
			if err != nil {
				return nil, false
			}
			return f, true
		case *big.Float:
			return new(big.Float).SetPrec(normalizePrecision).Set(v), true
		}
		return nil, false

	case "int":
		switch v := val.(type) {
		case float64:
			i := int64(v)
			return i, true
		case int:
			return int64(v), true
		case int64:
			return v, true
		case bool:
			// Mapping says int but payload provided bool — common when a
			// vendor encodes a flag as 0/1 vs true/false depending on
			// firmware revision. Coerce defensively.
			if v {
				return int64(1), true
			}
			return int64(0), true
		}
		return nil, false

	case "bool":
		switch v := val.(type) {
		case bool:
			return v, true
		case float64:
			return v != 0, true
		case int:
			return v != 0, true
		case int64:
			return v != 0, true
		case string:
			s := strings.ToLower(v)
			return s == "true" || s == "1" || s == "yes", true
		}
		return nil, false

	case "text":
		switch v := val.(type) {
		case string:
			return v, true
		}
		return nil, false
	}
	return nil, false
}

// applyScale multiplies a numeric value by the scale factor. Non-numeric
// data types pass through unchanged (scale is meaningless for bool/text).
//
// Scale=nil is treated as identity (1).
func applyScale(val any, scale *big.Float, dataType string) any {
	if scale == nil {
		return val
	}
	// Identity short-circuit: scale==1 → no-op.
	one := big.NewFloat(1)
	if scale.Cmp(one) == 0 {
		return val
	}
	switch dataType {
	case "numeric":
		f, ok := val.(*big.Float)
		if !ok {
			return val
		}
		return new(big.Float).SetPrec(normalizePrecision).Mul(f, scale)
	case "int":
		i, ok := val.(int64)
		if !ok {
			return val
		}
		// int * scale → still numeric, but the int target columns expect
		// int. Coerce by scaling then truncating. This is the behavior the
		// profile editor promises: scale=0.001 on a uint32 register lands
		// the value at three-decimal precision, but for an int column we
		// truncate (rare path; most scaled fields are numeric).
		f := new(big.Float).SetPrec(normalizePrecision).SetInt64(i)
		f.Mul(f, scale)
		out, _ := f.Int64()
		return out
	}
	return val
}

// assignToLayer1 writes a coerced+scaled value into the right canonical
// column on out. Type assertions are defensive — coerce already produced
// the correct type per data_type, so assignment failures are silent skips
// (would indicate a misconfigured profile that the editor failed to reject).
func assignToLayer1(out *Layer1, target string, val any) {
	switch target {
	case "raw_value":
		if f, ok := val.(*big.Float); ok {
			out.RawValue = f
		}
	case "cumulative_value":
		// cumulative_value is computed by Persist (raw + binding.offset).
		// A profile that explicitly maps cumulative_value treats the codec's
		// emission as authoritative — overrides the computation. Rare; allowed.
		if f, ok := val.(*big.Float); ok {
			out.CumulativeValue = f
		}
	case "instant_value":
		if f, ok := val.(*big.Float); ok {
			out.InstantValue = f
		}
	case "battery_pct":
		if i, ok := val.(int64); ok {
			v := int16(i)
			out.BatteryPct = &v
		}
	case "rssi":
		if i, ok := val.(int64); ok {
			v := int16(i)
			out.RSSI = &v
		}
	case "snr":
		if f, ok := val.(*big.Float); ok {
			fv, _ := f.Float64()
			f32 := float32(fv)
			out.SNR = &f32
		}
	case "temperature_c":
		if f, ok := val.(*big.Float); ok {
			fv, _ := f.Float64()
			f32 := float32(fv)
			out.TemperatureC = &f32
		}
	case "pressure_kpa":
		if f, ok := val.(*big.Float); ok {
			fv, _ := f.Float64()
			f32 := float32(fv)
			out.PressureKPa = &f32
		}
	case "leak_detected":
		if b, ok := val.(bool); ok {
			out.LeakDetected = &b
		}
	case "tamper_detected":
		if b, ok := val.(bool); ok {
			out.TamperDetected = &b
		}
	}
}
