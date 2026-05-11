package testsupport

// Canonical EUI / key fixtures for Phase 3 normalization, parser, and reveal
// tests. The strings here are stable across tests so assertions can reference
// `testsupport.ValidDevEUI1` rather than re-encoding "70b3d59999000001"
// inline. Mixed-case / separator variants exist so the normalizer
// (internal/import/euikeys.go in Wave 1) can be exercised against all the
// shapes operators paste into the bulk-import sheet.
//
// Values are intentionally NOT random — deterministic test inputs are the
// rule, not the exception (D-08 intra-file duplicate detection assumes the
// fixture stays the same across the dry-run and commit phases).

const (
	// ValidDevEUI1..ValidDevEUI5 — five 16-hex DevEUIs in the IEEE OUI block
	// `70b3d5` (Semtech / ChirpStack reserve range), suffixed `9999` so they
	// can never collide with real production device EUIs during local tests.
	ValidDevEUI1 = "70b3d59999000001"
	ValidDevEUI2 = "70b3d59999000002"
	ValidDevEUI3 = "70b3d59999000003"
	ValidDevEUI4 = "70b3d59999000004"
	ValidDevEUI5 = "70b3d59999000005"

	// ValidGatewayID1 — canonical 16-hex Gateway EUI64. Mac block
	// `ac1f09fffe` is the canonical Multitech-style EUI64 derived from a
	// MAC-48 by sandwiching `fffe` (a common LoRaWAN gateway convention).
	ValidGatewayID1 = "ac1f09fffe000001"

	// ValidJoinEUI — 16-hex JoinEUI (AppEUI). Some vendors ship all-zero
	// JoinEUI; we pick a deterministic non-zero value to catch off-by-one
	// "treated zero as missing" bugs.
	ValidJoinEUI = "0000000000000001"

	// ValidAppKey — 32-hex (16-byte) OTAA AppKey.
	ValidAppKey = "00112233445566778899aabbccddeeff"

	// ValidNwkKey — same shape as AppKey; used for LoRaWAN 1.1 OTAA tests.
	ValidNwkKey = "ffeeddccbbaa99887766554433221100"

	// ValidDevAddr — 8-hex (4-byte) ABP DevAddr.
	ValidDevAddr = "01020304"

	// ValidNwkSEncKey, ValidSNwkSIntKey, ValidFNwkSIntKey, ValidAppSKey —
	// 32-hex session keys for the full ABP activation envelope (D-20 +
	// D-22). Distinct values per slot so leaked-into-wrong-slot bugs are
	// detectable.
	ValidNwkSEncKey   = "11111111111111111111111111111111"
	ValidSNwkSIntKey  = "22222222222222222222222222222222"
	ValidFNwkSIntKey  = "33333333333333333333333333333333"
	ValidAppSKey      = "44444444444444444444444444444444"

	// MalformedEUI — contains non-hex char 'X'; normalizer must reject.
	MalformedEUI = "70b3d5XYZ9000001"

	// ShortEUI — only 6 hex chars; length validator must reject (DevEUI
	// must be exactly 16 hex).
	ShortEUI = "70b3d5"

	// LongEUI — 18 hex chars; over-length must reject.
	LongEUI = "70b3d59999000001ff"

	// MixedCaseEUI — same bytes as ValidDevEUI1 but uppercase. Normalizer
	// must lowercase before persisting / comparing.
	MixedCaseEUI = "70B3D59999000001"

	// WithSeparators — same bytes as ValidDevEUI1 with `:` separators.
	// Normalizer must strip `:` / `-` and validate the remaining 16 hex.
	WithSeparators = "70:b3:d5:99:99:00:00:01"

	// WithDashes — same with dashes (a few firmware vendors output this
	// shape).
	WithDashes = "70-b3-d5-99-99-00-00-01"

	// With0xPrefix — `0x` prefix is accepted and stripped.
	With0xPrefix = "0x70b3d59999000001"

	// AppKeyShort — 30 hex chars; AppKey length validator must reject.
	AppKeyShort = "00112233445566778899aabbccddee"

	// AppKeyLong — 34 hex chars; over-length must reject.
	AppKeyLong = "00112233445566778899aabbccddeeff00"
)

// ValidDevEUIs is a convenience slice of the five canonical valid DevEUIs.
// Useful when iterating to build fixtures like HappyFiveRows().
func ValidDevEUIs() []string {
	return []string{ValidDevEUI1, ValidDevEUI2, ValidDevEUI3, ValidDevEUI4, ValidDevEUI5}
}
