package importpkg

// CSV parser for bulk-import uploads. D-04a: CSV is secondary (XLSX is
// primary because Thai-locale Excel saves CSVs as TIS-620 / CP874, not
// UTF-8). T-3-42 mitigation: we strictly enforce UTF-8 and return an
// operator-readable error so the operator re-saves rather than silently
// ingesting mojibake.

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// utf8BOM is the 3-byte UTF-8 byte-order mark Excel-on-Windows prepends to
// "CSV UTF-8" exports. We strip it before handing the bytes to encoding/csv
// so the first header cell isn't returned as a BOM-prefixed "dev_eui" string.
const utf8BOM = "\xEF\xBB\xBF"

// nonUTF8Message — D-04a Thai-encoding guard error text. The wording is
// the UX: it tells the operator the exact remediation (Re-save as "CSV
// UTF-8") rather than a generic "invalid encoding". Auto-detection of
// TIS-620 is intentionally NOT attempted (RESEARCH §CSV Parsing — silent
// recovery is worse than a clear error because partial-decoded Thai text
// looks fine in some places and garbled elsewhere).
const nonUTF8Message = "CSV is not valid UTF-8. Re-save the file as 'CSV UTF-8' or use the XLSX template instead."

// ParseCSV reads a CSV upload, strips a leading UTF-8 BOM if present,
// rejects non-UTF-8 input with nonUTF8Message, and returns one ParsedRow
// per data row.
//
// Tolerances (matches RESEARCH §CSV Parsing stdlib quirks):
//   - FieldsPerRecord = -1: trailing-comma irregularity is fine.
//   - LazyQuotes = true: descriptions like `"Building "A" main"` parse.
//   - TrimLeadingSpace = true: cells like ` OTAA ` become "OTAA" after
//     parser-side TrimSpace as well (defense-in-depth).
func ParseCSV(r io.Reader) ([]ParsedRow, error) {
	// Peek-3 to detect + strip the BOM without consuming bytes when absent.
	br := bufio.NewReader(r)
	if peek, _ := br.Peek(3); bytes.Equal(peek, []byte(utf8BOM)) {
		_, _ = br.Discard(3)
	}

	all, err := io.ReadAll(br)
	if err != nil {
		return nil, fmt.Errorf("csv read: %w", err)
	}
	if !utf8.Valid(all) {
		return nil, fmt.Errorf(nonUTF8Message)
	}

	rdr := csv.NewReader(bytes.NewReader(all))
	rdr.FieldsPerRecord = -1
	rdr.LazyQuotes = true
	rdr.TrimLeadingSpace = true

	rows, err := rdr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("csv parse: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("csv is empty")
	}

	header := make([]string, len(rows[0]))
	for i, h := range rows[0] {
		h = strings.TrimSpace(h)
		h = strings.TrimPrefix(h, utf8BOM)
		header[i] = strings.ToLower(h)
	}

	out := make([]ParsedRow, 0, len(rows)-1)
	for i, row := range rows[1:] {
		raw := make(map[string]string, len(header))
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
