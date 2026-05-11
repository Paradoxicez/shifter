package importpkg

// XLSX parser for bulk-import uploads. D-04a: XLSX is the primary, Thai-safe
// format. excelize/v2 loads the whole workbook into memory; for Phase 3's
// ≤5000-row cap that's bounded at ~6MB of in-memory strings (trivial).
//
// Output shape: []ParsedRow with raw_payload (column-name → trimmed cell
// string) preserving the original file lossless. Dry-run validator
// (dryrun.go) consumes ParsedRow and writes the canonicalised values into
// import_job_row.parsed.

import (
	"fmt"
	"io"
	"strings"

	"github.com/xuri/excelize/v2"
)

// SheetName is the canonical sheet name written by GenerateTemplate and read
// by ParseXLSX. Operators sometimes rename it (or paste into Sheet1); when
// "Devices" is not present we fall back to the first sheet so we don't
// 400-error on slightly-mangled uploads.
const SheetName = "Devices"

// ParsedRow is the lossless raw form of a single file row. RowIndex is
// 1-based and matches the underlying XLSX / CSV row number so error
// messages reference cells the operator can see in Excel. Raw maps the
// lowercase header name to the trimmed cell value (empty string for blank
// cells, never a missing key for declared columns).
type ParsedRow struct {
	// RowIndex — 1-based, matches the XLSX / CSV row number (row 1 is the
	// header row, so data starts at row 2).
	RowIndex int

	// Raw — column name (lowercased) → original cell value (whitespace
	// trimmed). Lossless: errors.xlsx round-tripping reconstructs the input
	// from this map.
	Raw map[string]string
}

// ParseXLSX reads an XLSX upload and returns one ParsedRow per data row.
// Row 1 MUST be the header row; the parser keys cells by lowercased header
// name so column reordering / case-mangling does not break ingestion.
//
// Empty data rows (every cell blank) ARE preserved as ParsedRow entries —
// the dry-run validator decides what "valid data row" means. This keeps
// row_index stable across parser → dry-run → errors.xlsx (operators expect
// the row number in errors.xlsx to match what they see in Excel).
//
// .xlsm macro-enabled workbooks must be rejected at the HTTP handler
// (extension check) — this parser will happily open a .xlsm but the threat
// model (T-3-40) forbids it.
func ParseXLSX(r io.Reader) ([]ParsedRow, error) {
	f, err := excelize.OpenReader(r)
	if err != nil {
		return nil, fmt.Errorf("xlsx open: %w", err)
	}
	defer func() { _ = f.Close() }()

	// Locate the data sheet. Prefer "Devices" (the template default); fall
	// back to the first sheet if the operator renamed it. GetSheetIndex
	// returns -1 (not an error) when the sheet is missing — that's why we
	// branch on the int rather than err.
	sn := SheetName
	if idx, _ := f.GetSheetIndex(sn); idx < 0 {
		sn = f.GetSheetName(0)
		if sn == "" {
			return nil, fmt.Errorf("xlsx: workbook has no sheets")
		}
	}

	rows, err := f.GetRows(sn)
	if err != nil {
		return nil, fmt.Errorf("xlsx rows: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("xlsx is empty")
	}

	header := normalizeHeader(rows[0])
	out := make([]ParsedRow, 0, len(rows)-1)
	for i, row := range rows[1:] {
		raw := make(map[string]string, len(header))
		// Initialise every declared header key — blank cells should produce
		// "" rather than a missing key (downstream validators rely on
		// raw["dev_eui"] being safe to read).
		for _, h := range header {
			if h == "" {
				continue
			}
			raw[h] = ""
		}
		for j, cell := range row {
			if j >= len(header) || header[j] == "" {
				continue
			}
			raw[header[j]] = strings.TrimSpace(cell)
		}
		out = append(out, ParsedRow{RowIndex: i + 2, Raw: raw})
	}
	return out, nil
}

// normalizeHeader lowercases and trims every header cell. A stray
// BOM-prefixed first header (from a CSV-resave-to-XLSX) is also stripped.
func normalizeHeader(row []string) []string {
	out := make([]string, len(row))
	for i, h := range row {
		h = strings.TrimSpace(h)
		h = strings.TrimPrefix(h, utf8BOM)
		out[i] = strings.ToLower(h)
	}
	return out
}
