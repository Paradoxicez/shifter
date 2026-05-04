// Package device — DevEUI sticker parser (D-11) + handler implementation
// for Plan 02-10 atomic Add Device flow.
//
// ParseDevEUI implements D-11: vendor stickers print the EUI in either
// MSB-first or LSB-first byte order, and the operator can rarely tell which
// at a glance. The Add Device dialog calls ParseDevEUI once, presents BOTH
// interpretations side-by-side with a vendor-OUI hint per interpretation,
// and lets the operator click the right one. The chosen value is then sent
// to the addDevice handler verbatim (lowercase 16-hex).
//
// Out of scope here: ChirpStack normalizes DevEUI to lowercase; the schema
// CHECK in 0012_device enforces lowercase + hex16. ParseDevEUI does the
// normalization at the UI boundary so the schema CHECK is defense-in-depth.
package device

import (
	"encoding/hex"
	"errors"
	"strings"
	"unicode"
)

// DevEUIPreview is the value-shape Plan 02-10's POST /api/devices/parse-deveui
// returns. Both MSB and LSB are lowercase 16-hex strings; vendor hints are
// human-readable strings derived from the leading 24-bit IEEE OUI block.
type DevEUIPreview struct {
	// MSB is the EUI as pasted (lowercased + separator-stripped). The leading
	// byte is the vendor OUI's most-significant byte.
	MSB string
	// LSB is MSB byte-reversed — the alternate sticker orientation. If the
	// vendor printed the EUI LSB-first, this is the byte order the operator
	// actually wants.
	LSB string
	// MSBVendor / LSBVendor are best-effort vendor lookups derived from each
	// interpretation's first 6 hex chars (24-bit OUI). "unknown" when the OUI
	// is not in the lookup table (Phase 7 vendor catalog will expand it).
	MSBVendor string
	LSBVendor string
}

// ErrBadDevEUI is returned by ParseDevEUI when the input doesn't reduce to
// exactly 16 hex characters after whitespace/colon/hyphen stripping.
var ErrBadDevEUI = errors.New("DevEUI must be 16 hex characters after whitespace strip")

// ParseDevEUI accepts a hex string from a vendor sticker — possibly with
// whitespace, colons, hyphens, and mixed case — and returns BOTH the MSB-
// first and LSB-first interpretations along with a vendor OUI hint per
// interpretation.
//
// Normalization (in order):
//
//  1. Drop ' ', '-', ':', '\t', '\n', '\r' from the input.
//  2. Lowercase every remaining character.
//  3. Reject if the result is not exactly 16 chars or is not hex.
//
// The byte-reverse for LSB walks 8 byte pairs (16 chars) from end to start;
// hex.DecodeString → reverse → hex.EncodeToString is the most readable form
// and is fast enough for this user-typing-driven hot path.
func ParseDevEUI(raw string) (DevEUIPreview, error) {
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '-', ':', '\t', '\n', '\r':
			return -1
		}
		return unicode.ToLower(r)
	}, raw)
	if len(cleaned) != 16 {
		return DevEUIPreview{}, ErrBadDevEUI
	}
	b, err := hex.DecodeString(cleaned)
	if err != nil {
		return DevEUIPreview{}, ErrBadDevEUI
	}

	msb := cleaned

	// Reverse bytes for the LSB interpretation. b was just decoded above so
	// the slice is exactly 8 bytes; the swap loop is symmetric.
	rev := make([]byte, len(b))
	copy(rev, b)
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	lsb := hex.EncodeToString(rev)

	return DevEUIPreview{
		MSB:       msb,
		LSB:       lsb,
		MSBVendor: ouiVendor(msb[:6]),
		LSBVendor: ouiVendor(lsb[:6]),
	}, nil
}

// ouiVendor performs a minimal IEEE OUI lookup for the Phase 2 vendor
// families that ship out-of-the-box (Axioma + Acrel). Phase 7 (vendor
// catalog) will replace this with a richer lookup possibly backed by a
// generated table. Unknown OUI returns "unknown" so the UI can render the
// alternative as the more likely choice.
//
// References:
//   - 70:B3:D5 — IEEE block including Axioma allocations (and many other
//     small-vendor lessees; the hint says "Axioma (or other 70:B3:D5 lessee)"
//     to avoid false certainty).
//   - A8:40:41 — Acrel-allocated block per IEEE OUI registry.
func ouiVendor(oui6hex string) string {
	switch strings.ToLower(oui6hex) {
	case "70b3d5":
		return "Axioma (or other 70:B3:D5 lessee)"
	case "a84041":
		return "Acrel"
	}
	return "unknown"
}
