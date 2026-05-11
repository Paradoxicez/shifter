package importpkg

// Phase 3 Plan 03-05 — Normalize* coverage. D-05 normalisation: strip `0x`,
// `:`, `-`, whitespace, lowercase, length-validate. Table-driven so every
// rejection mode is documented in test data, not test prose.

import (
	"strings"
	"testing"

	"github.com/shifter-io/shifter/internal/testsupport"
)

// TestNormalizeDevEUI — 16-hex canonical, all sticker-paste variants converge.
func TestNormalizeDevEUI(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"lowercase canonical", testsupport.ValidDevEUI1, testsupport.ValidDevEUI1, false},
		{"uppercase", testsupport.MixedCaseEUI, testsupport.ValidDevEUI1, false},
		{"colon-separated", testsupport.WithSeparators, testsupport.ValidDevEUI1, false},
		{"dash-separated", testsupport.WithDashes, testsupport.ValidDevEUI1, false},
		{"0x prefix", testsupport.With0xPrefix, testsupport.ValidDevEUI1, false},
		{"reject non-hex char", testsupport.MalformedEUI, "", true},
		{"reject short", testsupport.ShortEUI, "", true},
		{"reject long", testsupport.LongEUI, "", true},
		{"reject empty", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeDevEUI(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("NormalizeDevEUI(%q) = %q, want error", tc.input, got)
				}
				if !strings.Contains(err.Error(), "dev_eui") {
					t.Errorf("error %q missing field label dev_eui", err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeDevEUI(%q) unexpected error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("NormalizeDevEUI(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestNormalizeJoinEUI — same EUI64 shape; canonical zero-JoinEUI permitted.
func TestNormalizeJoinEUI(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"non-zero canonical", testsupport.ValidJoinEUI, testsupport.ValidJoinEUI, false},
		{"all zero", "0000000000000000", "0000000000000000", false},
		{"uppercase", "0000000000000001", "0000000000000001", false},
		{"reject 15-char", "000000000000000", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeJoinEUI(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("NormalizeJoinEUI(%q) = %q, want error", tc.input, got)
				}
				if !strings.Contains(err.Error(), "join_eui") {
					t.Errorf("error %q missing field label join_eui", err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeJoinEUI(%q) unexpected error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("NormalizeJoinEUI(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}

	// ParseEUI64 alias must match NormalizeDevEUI/NormalizeJoinEUI exactly
	// (both EUI64 shape).
	got, err := ParseEUI64(testsupport.WithSeparators)
	if err != nil {
		t.Fatalf("ParseEUI64 unexpected error: %v", err)
	}
	if got != testsupport.ValidDevEUI1 {
		t.Errorf("ParseEUI64 = %q, want %q", got, testsupport.ValidDevEUI1)
	}
}

// TestNormalizeAppKey — 32-hex AES-128 OTAA root key.
func TestNormalizeAppKey(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"lowercase canonical", testsupport.ValidAppKey, testsupport.ValidAppKey, false},
		{"uppercase", strings.ToUpper(testsupport.ValidAppKey), testsupport.ValidAppKey, false},
		{"with separators", "00:11:22:33:44:55:66:77:88:99:aa:bb:cc:dd:ee:ff",
			testsupport.ValidAppKey, false},
		{"reject short 30-char", testsupport.AppKeyShort, "", true},
		{"reject long 34-char", testsupport.AppKeyLong, "", true},
		{"reject non-hex", "00112233445566778899aabbccddeegg", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeAppKey(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("NormalizeAppKey(%q) = %q, want error", tc.input, got)
				}
				if !strings.Contains(err.Error(), "app_key") {
					t.Errorf("error %q missing field label app_key", err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeAppKey(%q) unexpected error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("NormalizeAppKey(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestNormalizeDevAddr — 8-hex 32-bit ABP DevAddr.
func TestNormalizeDevAddr(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"canonical", testsupport.ValidDevAddr, testsupport.ValidDevAddr, false},
		{"uppercase", "01020304", "01020304", false},
		{"reject 7-char", "0102030", "", true},
		{"reject 9-char", "010203045", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeDevAddr(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("NormalizeDevAddr(%q) = %q, want error", tc.input, got)
				}
				if !strings.Contains(err.Error(), "dev_addr") {
					t.Errorf("error %q missing field label dev_addr", err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeDevAddr(%q) unexpected error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("NormalizeDevAddr(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestNormalizeNwkSKey — 32-hex session key. Reuses AppKey shape exactly.
func TestNormalizeNwkSKey(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"canonical", testsupport.ValidNwkSEncKey, testsupport.ValidNwkSEncKey, false},
		{"reject 31-char", "1111111111111111111111111111111", "", true},
		{"reject 33-char", "111111111111111111111111111111111", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeNwkSKey(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("NormalizeNwkSKey(%q) = %q, want error", tc.input, got)
				}
				if !strings.Contains(err.Error(), "nwk_s_key") {
					t.Errorf("error %q missing field label nwk_s_key", err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeNwkSKey(%q) unexpected error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("NormalizeNwkSKey(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}

	// AppSKey same shape — quick smoke test.
	got, err := NormalizeAppSKey(testsupport.ValidAppSKey)
	if err != nil {
		t.Fatalf("NormalizeAppSKey unexpected error: %v", err)
	}
	if got != testsupport.ValidAppSKey {
		t.Errorf("NormalizeAppSKey = %q, want %q", got, testsupport.ValidAppSKey)
	}
}

// TestNormalize_StripPrefixesAndSeparators — the strings.Map+TrimPrefix
// pipeline must strip whitespace (space/tab/newline/CR), hyphen, colon, and
// `0x` prefix in any combination.
func TestNormalize_StripPrefixesAndSeparators(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"spaces interleaved", "70 b3 d5 99 99 00 00 01", testsupport.ValidDevEUI1},
		{"tabs interleaved", "70\tb3\td5\t99\t99\t00\t00\t01", testsupport.ValidDevEUI1},
		{"newline + carriage return", "70b3d59999\n\r000001", testsupport.ValidDevEUI1},
		{"0x prefix + colons", "0x70:b3:d5:99:99:00:00:01", testsupport.ValidDevEUI1},
		{"mixed case + 0X prefix", "0X70B3D59999000001", testsupport.ValidDevEUI1},
		{"trailing whitespace", "70b3d59999000001\n", testsupport.ValidDevEUI1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeDevEUI(tc.input)
			if err != nil {
				t.Fatalf("NormalizeDevEUI(%q) unexpected error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("NormalizeDevEUI(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
