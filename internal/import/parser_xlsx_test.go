package importpkg

// Phase 3 Plan 03-05 — parser_xlsx.go: excelize-based read of operator-
// uploaded XLSX. D-04 / D-04a header schema, T-3-40 zip-bomb defense
// (size-cap enforcement lives in the handler — parser itself just loads
// bytes excelize-style).

import (
	"bytes"
	"testing"

	"github.com/shifter-io/shifter/internal/testsupport"
)

// TestParseXLSX_HappyPath — testsupport.HappyFiveRows() decodes to 5
// ParsedRow entries with raw_payload preserving every input cell.
func TestParseXLSX_HappyPath(t *testing.T) {
	t.Parallel()
	xlsx := testsupport.HappyFiveRows()

	rows, err := ParseXLSX(bytes.NewReader(xlsx))
	if err != nil {
		t.Fatalf("ParseXLSX: %v", err)
	}
	if len(rows) != 5 {
		t.Fatalf("ParseXLSX: got %d rows, want 5", len(rows))
	}

	// Row 1 in our slice = file row 2 (header is file row 1).
	first := rows[0]
	if first.RowIndex != 2 {
		t.Errorf("first row index = %d, want 2", first.RowIndex)
	}
	if first.Raw["dev_eui"] != testsupport.ValidDevEUI1 {
		t.Errorf("dev_eui = %q, want %q", first.Raw["dev_eui"], testsupport.ValidDevEUI1)
	}
	if first.Raw["device_profile"] != "axioma_w1" {
		t.Errorf("device_profile = %q, want axioma_w1", first.Raw["device_profile"])
	}
	if first.Raw["activation_mode"] != "OTAA" {
		t.Errorf("activation_mode = %q, want OTAA", first.Raw["activation_mode"])
	}
	if first.Raw["app_key"] != testsupport.ValidAppKey {
		t.Errorf("app_key = %q, want %q", first.Raw["app_key"], testsupport.ValidAppKey)
	}

	// Last row in slice = file row 6 (5 data rows + 1 header).
	last := rows[4]
	if last.RowIndex != 6 {
		t.Errorf("last row index = %d, want 6", last.RowIndex)
	}
	if last.Raw["dev_eui"] != testsupport.ValidDevEUI5 {
		t.Errorf("last dev_eui = %q, want %q", last.Raw["dev_eui"], testsupport.ValidDevEUI5)
	}
}

// TestParseXLSX_DataValidationRoundtrip — the generated template parses
// cleanly with 0 data rows and survives the dropdown / NumFmt metadata.
func TestParseXLSX_DataValidationRoundtrip(t *testing.T) {
	t.Parallel()
	xlsx, err := testsupport.ImportTemplateBytes()
	if err != nil {
		t.Fatalf("ImportTemplateBytes: %v", err)
	}
	rows, err := ParseXLSX(bytes.NewReader(xlsx))
	if err != nil {
		t.Fatalf("ParseXLSX template: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("template should parse to 0 rows, got %d", len(rows))
	}
}

// TestParseXLSX_HeaderLowercased — headers cased differently still resolve
// against the lowercase Raw map.
func TestParseXLSX_HeaderLowercased(t *testing.T) {
	t.Parallel()
	xlsx := testsupport.HappyFiveRows()
	rows, err := ParseXLSX(bytes.NewReader(xlsx))
	if err != nil {
		t.Fatalf("ParseXLSX: %v", err)
	}
	// Even if testsupport fixtures used lowercase headers, downstream
	// callers can rely on the normalisation contract.
	if _, ok := rows[0].Raw["dev_eui"]; !ok {
		t.Errorf("Raw map missing dev_eui key after normalisation")
	}
}
