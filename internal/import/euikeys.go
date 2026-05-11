// Package importpkg — Phase 3 bulk-import backend (D-04..D-11, D-33..D-36).
//
// Package name is `importpkg` because `import` is a reserved keyword in Go;
// the directory remains `internal/import/` so the URL slug (`/admin/imports`)
// and human-readable plan references map cleanly.
//
// This file is the EUI / key normalisation primitive layer used by the parser,
// dry-run validator, and commit pass. Every hex field operators paste — DevEUI,
// JoinEUI/AppEUI, AppKey, DevAddr, NwkSKey, AppSKey — flows through one of the
// Normalize* helpers so the rest of the package only ever sees the canonical
// lowercase form. Per D-05: backend auto-strips `0x` prefix, `:` / `-`
// separators and whitespace, lowercases, then length-validates.
package importpkg

import (
	"encoding/hex"
	"fmt"
	"strings"
	"unicode"
)

// normalizeHex is the canonical hex-normalisation helper. Strips spaces,
// tabs, newlines, hyphens, colons, and the optional `0x` prefix; lowercases.
// Returns the canonical lowercase hex string OR an error describing
// length / character mismatch.
//
// The label is interpolated into error messages so the operator sees
// "dev_eui: expected 16 hex chars after normalisation, got 15" rather than
// a generic "bad hex" with no field context.
func normalizeHex(raw string, wantChars int, label string) (string, error) {
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '-', ':', '\t', '\n', '\r':
			return -1
		}
		return unicode.ToLower(r)
	}, raw)
	cleaned = strings.TrimPrefix(cleaned, "0x")
	if len(cleaned) != wantChars {
		return "", fmt.Errorf("%s: expected %d hex chars after normalisation, got %d", label, wantChars, len(cleaned))
	}
	if _, err := hex.DecodeString(cleaned); err != nil {
		return "", fmt.Errorf("%s: not valid hex: %w", label, err)
	}
	return cleaned, nil
}

// NormalizeDevEUI normalises a sticker-pasted DevEUI to the canonical
// lowercase 16-hex form (EUI64; 8 bytes).
func NormalizeDevEUI(raw string) (string, error) { return normalizeHex(raw, 16, "dev_eui") }

// NormalizeJoinEUI normalises the OTAA join identifier (also known as
// AppEUI in LoRaWAN 1.0.x). Same EUI64 shape as DevEUI.
func NormalizeJoinEUI(raw string) (string, error) { return normalizeHex(raw, 16, "join_eui") }

// NormalizeAppKey normalises the OTAA root key. 32 hex chars = 128-bit AES.
func NormalizeAppKey(raw string) (string, error) { return normalizeHex(raw, 32, "app_key") }

// NormalizeDevAddr normalises the ABP DevAddr. 8 hex chars = 32-bit value.
func NormalizeDevAddr(raw string) (string, error) { return normalizeHex(raw, 8, "dev_addr") }

// NormalizeNwkSKey normalises the ABP network session key. 32 hex chars.
// For LoRaWAN 1.0.x devices this is also copied into NwkSEncKey /
// SNwkSIntKey / FNwkSIntKey by the ChirpStack wrapper (see
// internal/chirpstack/device.go ActivateDevice).
func NormalizeNwkSKey(raw string) (string, error) { return normalizeHex(raw, 32, "nwk_s_key") }

// NormalizeAppSKey normalises the ABP application session key. 32 hex chars.
func NormalizeAppSKey(raw string) (string, error) { return normalizeHex(raw, 32, "app_s_key") }

// ParseEUI64 is an alias of NormalizeDevEUI for sticker-paste UX (gateway_id
// and join_eui both follow the EUI64 shape). Exported so future call sites
// can express intent without coupling to the DevEUI-specific name.
func ParseEUI64(raw string) (string, error) { return NormalizeDevEUI(raw) }
