package device

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestParseDevEUI_HappyPath_MSB — clean MSB-first input round-trips to MSB
// unchanged AND produces the byte-reversed LSB.
func TestParseDevEUI_HappyPath_MSB(t *testing.T) {
	p, err := ParseDevEUI("0102030405060708")
	require.NoError(t, err)
	require.Equal(t, "0102030405060708", p.MSB)
	require.Equal(t, "0807060504030201", p.LSB)
}

// TestParseDevEUI_StripsSeparators — colons, hyphens, whitespace get dropped.
func TestParseDevEUI_StripsSeparators(t *testing.T) {
	p, err := ParseDevEUI("01:02:03:04:05:06:07:08")
	require.NoError(t, err)
	require.Equal(t, "0102030405060708", p.MSB)
}

// TestParseDevEUI_StripsHyphens — hyphen separators handled.
func TestParseDevEUI_StripsHyphens(t *testing.T) {
	p, err := ParseDevEUI("01-02-03-04-05-06-07-08")
	require.NoError(t, err)
	require.Equal(t, "0102030405060708", p.MSB)
}

// TestParseDevEUI_StripsWhitespace — spaces and tabs stripped.
func TestParseDevEUI_StripsWhitespace(t *testing.T) {
	p, err := ParseDevEUI("01 02 03 04 05 06 07 08")
	require.NoError(t, err)
	require.Equal(t, "0102030405060708", p.MSB)
}

// TestParseDevEUI_LowercaseUppercase — uppercase input gets lowercased AND
// the byte-reversed LSB is also produced.
func TestParseDevEUI_LowercaseUppercase(t *testing.T) {
	p, err := ParseDevEUI("ABCDEF1234567890")
	require.NoError(t, err)
	require.Equal(t, "abcdef1234567890", p.MSB)
	require.Equal(t, "9078563412efcdab", p.LSB)
}

// TestParseDevEUI_RejectsTooShort — fewer than 16 hex chars after stripping
// returns ErrBadDevEUI.
func TestParseDevEUI_RejectsTooShort(t *testing.T) {
	_, err := ParseDevEUI("01020304")
	require.ErrorIs(t, err, ErrBadDevEUI)
}

// TestParseDevEUI_RejectsTooLong — more than 16 hex chars after stripping
// returns ErrBadDevEUI.
func TestParseDevEUI_RejectsTooLong(t *testing.T) {
	_, err := ParseDevEUI("0102030405060708090a")
	require.ErrorIs(t, err, ErrBadDevEUI)
}

// TestParseDevEUI_RejectsNonHex — input with non-hex characters returns
// ErrBadDevEUI even when length is right.
func TestParseDevEUI_RejectsNonHex(t *testing.T) {
	// 18 chars → after strip is still 18 if the 'xx' aren't separators.
	_, err := ParseDevEUI("0102030405060708xx")
	require.ErrorIs(t, err, ErrBadDevEUI)

	// Length-correct but with non-hex characters.
	_, err = ParseDevEUI("0102030405060ZZZ")
	require.ErrorIs(t, err, ErrBadDevEUI)
}

// TestParseDevEUI_RejectsEmpty — empty input returns ErrBadDevEUI.
func TestParseDevEUI_RejectsEmpty(t *testing.T) {
	_, err := ParseDevEUI("")
	require.ErrorIs(t, err, ErrBadDevEUI)
}

// TestParseDevEUI_AxiomaOUIHint — input starting 70:b3:d5 returns the
// Axioma vendor hint on the MSB side.
func TestParseDevEUI_AxiomaOUIHint(t *testing.T) {
	p, err := ParseDevEUI("70b3d5abcdef1234")
	require.NoError(t, err)
	require.Contains(t, strings.ToLower(p.MSBVendor), "axioma")
	// The byte-reversed form's first 6 chars are "341234" which isn't in the
	// vendor table — so LSBVendor is "unknown".
	require.Equal(t, "unknown", p.LSBVendor)
}

// TestParseDevEUI_AcrelOUIHint — input starting a8:40:41 returns the
// Acrel vendor hint on the MSB side.
func TestParseDevEUI_AcrelOUIHint(t *testing.T) {
	p, err := ParseDevEUI("a84041deadbeef00")
	require.NoError(t, err)
	require.Equal(t, "Acrel", p.MSBVendor)
}

// TestParseDevEUI_UnknownOUI — neither orientation matches a known vendor
// → both vendor hints are "unknown".
func TestParseDevEUI_UnknownOUI(t *testing.T) {
	p, err := ParseDevEUI("deadbeef12345678")
	require.NoError(t, err)
	require.Equal(t, "unknown", p.MSBVendor)
	require.Equal(t, "unknown", p.LSBVendor)
}
