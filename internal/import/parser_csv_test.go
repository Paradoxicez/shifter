package importpkg

// Phase 3 Wave 1 — parser_csv.go: stdlib encoding/csv fallback for operators
// who insist on CSV. D-04a: UTF-8 (with optional BOM) accepted; everything
// else rejected with an operator-readable error.

import "testing"

// TestParseCSV_UTF8BOM — UTF-8 BOM (`EF BB BF`) at file start is stripped
// before passing to encoding/csv.
func TestParseCSV_UTF8BOM(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/import/parser_csv.go (03-VALIDATION row parser_csv_test.TestParseCSV_UTF8BOM)")
}

// TestParseCSV_NonUTF8Reject — CP874 / TIS-620 / UTF-16 byte sequences are
// rejected with a clear "save as UTF-8" error message.
func TestParseCSV_NonUTF8Reject(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/import/parser_csv.go (03-VALIDATION row parser_csv_test.TestParseCSV_NonUTF8Reject)")
}

// TestParseCSV_BOMStrip — explicit golden test that a 3-byte UTF-8 BOM is
// removed from header row 1 so column lookups don't see "<BOM>dev_eui".
func TestParseCSV_BOMStrip(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/import/parser_csv.go (03-VALIDATION row parser_csv_test.TestParseCSV_BOMStrip)")
}
