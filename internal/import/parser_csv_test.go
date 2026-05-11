package importpkg

// Phase 3 Plan 03-05 — parser_csv.go: stdlib encoding/csv fallback for
// operators who insist on CSV. D-04a UTF-8 enforcement, T-3-42 mitigation.

import (
	"bytes"
	"strings"
	"testing"
)

// TestParseCSV_UTF8BOM — UTF-8 BOM at file start is stripped before
// encoding/csv sees the bytes; downstream Raw map has clean header keys.
func TestParseCSV_UTF8BOM(t *testing.T) {
	t.Parallel()
	input := utf8BOM + "dev_eui,name\n70b3d59999000001,dev-1\n"
	rows, err := ParseCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].Raw["dev_eui"] != "70b3d59999000001" {
		t.Errorf("dev_eui = %q, want 70b3d59999000001", rows[0].Raw["dev_eui"])
	}
	if rows[0].Raw["name"] != "dev-1" {
		t.Errorf("name = %q, want dev-1", rows[0].Raw["name"])
	}
}

// TestParseCSV_NonUTF8Reject — TIS-620 bytes (0xB1 0xB2 etc. are valid
// TIS-620 / CP874 but invalid as standalone bytes in a UTF-8 stream).
// Operator gets the D-04a remediation message.
func TestParseCSV_NonUTF8Reject(t *testing.T) {
	t.Parallel()
	// Construct a header line that is invalid UTF-8 by injecting a lone
	// 0xB1 (TIS-620 ก) outside any multi-byte UTF-8 sequence. utf8.Valid
	// returns false → parser rejects with nonUTF8Message.
	input := append([]byte("dev_eui,name\n"), 0xB1, 0xB2, 0x2C, 0x64, 0x65, 0x76, 0x0A)
	_, err := ParseCSV(bytes.NewReader(input))
	if err == nil {
		t.Fatal("ParseCSV: expected error for non-UTF-8 input, got nil")
	}
	if !strings.Contains(err.Error(), "not valid UTF-8") {
		t.Errorf("error text %q missing operator-readable UTF-8 message", err.Error())
	}
	if !strings.Contains(err.Error(), "CSV UTF-8") {
		t.Errorf("error text %q missing 'CSV UTF-8' remediation", err.Error())
	}
}

// TestParseCSV_BOMStrip — explicit assert that the BOM doesn't end up
// inside the first header key. Catches the regression where Peek-3 +
// Discard-3 is replaced by something that consumes only on read.
func TestParseCSV_BOMStrip(t *testing.T) {
	t.Parallel()
	input := utf8BOM + "dev_eui,name\n70b3d59999000001,n\n"
	rows, err := ParseCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	// Defensive: the map must NOT contain a BOM-prefixed key like
	// "\xEF\xBB\xBFdev_eui".
	bomKey := utf8BOM + "dev_eui"
	if _, ok := rows[0].Raw[bomKey]; ok {
		t.Errorf("Raw map contains BOM-prefixed key %q — BOM not stripped", bomKey)
	}
	if rows[0].Raw["dev_eui"] == "" {
		t.Errorf("Raw[dev_eui] empty — BOM affected header parse")
	}
}

// TestParseCSV_LazyQuotes — descriptions containing literal quotes don't
// kill the parse (LazyQuotes = true).
func TestParseCSV_LazyQuotes(t *testing.T) {
	t.Parallel()
	input := `dev_eui,name,description
70b3d59999000001,n,Building "A" main
`
	rows, err := ParseCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseCSV: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
}
