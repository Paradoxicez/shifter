package profile

import (
	"strconv"
	"strings"
)

// Resolve walks the RFC 6901 JSON Pointer ptr through obj and returns the
// addressed value. Returns (root, true) for the empty pointer "". Returns
// (nil, false) for any malformed or non-existent path.
//
// Used by the ingest normalize pass (Plan 02-09) to look up each
// device_profile_mapping.json_pointer against the QuickJS-decoded `object`
// emitted by ChirpStack on every uplink.
//
// Supports the canonical RFC 6901 surface:
//
//	""                  → obj                (whole document)
//	"/foo"              → obj["foo"]
//	"/foo/bar"          → obj["foo"]["bar"]
//	"/list/0"           → obj["list"][0]      (decimal array index)
//	"/with~0tilde"      → obj["with~tilde"]   (~0 → ~)
//	"/with~1slash"      → obj["with/slash"]   (~1 → /)
//
// Per RFC 6901 §4 the unescape order is mandatory: ~1 first, ~0 second.
// Performing them in reverse would treat "~01" as "/1" rather than the
// intended "~1".
//
// Defensive behavior:
//   - A pointer that does not start with "/" (and is non-empty) returns (nil, false).
//   - An array index that is negative, non-decimal, or out-of-range returns (nil, false).
//   - Walking through a leaf value (number/string/bool/nil) returns (nil, false).
//   - Walking nil returns (nil, false).
func Resolve(obj any, ptr string) (any, bool) {
	if ptr == "" {
		return obj, true
	}
	if !strings.HasPrefix(ptr, "/") {
		return nil, false
	}

	// Split on "/" and unescape per RFC 6901 §4: ~1 → /, then ~0 → ~.
	parts := strings.Split(ptr[1:], "/")
	cur := obj
	for _, p := range parts {
		token := strings.ReplaceAll(p, "~1", "/")
		token = strings.ReplaceAll(token, "~0", "~")

		switch v := cur.(type) {
		case map[string]any:
			next, ok := v[token]
			if !ok {
				return nil, false
			}
			cur = next
		case []any:
			idx, err := strconv.Atoi(token)
			if err != nil || idx < 0 || idx >= len(v) {
				return nil, false
			}
			cur = v[idx]
		default:
			return nil, false
		}
	}
	return cur, true
}
