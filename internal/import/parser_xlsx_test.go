package importpkg

// Phase 3 Wave 1 — parser_xlsx.go: excelize-based read of the operator's
// uploaded XLSX. D-04 / D-04a: schema, header order, data-validation
// dropdown round-trip, BOM-handling fallback (none for XLSX since it's a
// zip — but the test asserts the parser doesn't choke on stray UTF-8 BOM
// in cell values).

import "testing"

// TestParseXLSX_HappyPath — feed testsupport.HappyFiveRows() bytes;
// parser returns 5 normalized rows.
func TestParseXLSX_HappyPath(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/import/parser_xlsx.go (03-VALIDATION row parser_xlsx_test.TestParseXLSX_HappyPath)")
}

// TestParseXLSX_DataValidationRoundtrip — feed testsupport.ImportTemplateBytes()
// (headers + NumFmt=49 + activation_mode dropdown); parser returns 0 rows
// without error (template only); the activation_mode dropdown survives
// re-serialization to bytes for the round-trip golden test.
func TestParseXLSX_DataValidationRoundtrip(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/import/parser_xlsx.go (03-VALIDATION row parser_xlsx_test.TestParseXLSX_DataValidationRoundtrip)")
}
