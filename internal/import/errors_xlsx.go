package importpkg

// errors.xlsx generator. D-07: operators download a fixed-up XLSX containing
// only the rows that failed validation / commit, with an appended
// `error_reason` column. They paste fixes back into the same file and
// re-upload — by D-06 idempotency, already-created rows in their original
// upload come back as `already_exists`, so the re-upload only creates new
// rows.

import (
	"encoding/json"
	"fmt"

	"github.com/xuri/excelize/v2"

	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// GenerateErrorsXLSX builds the errors-only XLSX from the persisted
// import_job_row rows (status ∈ invalid, failed). Cells are reconstructed
// from raw_payload JSONB — lossless round-trip with the operator's original
// file. The appended `error_reason` column is the human-readable failure
// message from import_job_row.reason.
//
// T-3-41 mitigation: every cell is written via SetCellStr (text), never
// SetCellValue (typed). A pasted formula like `=cmd|...` becomes the
// literal string `=cmd|...` in the export rather than an active formula —
// Excel will display the `=` but won't evaluate it.
func GenerateErrorsXLSX(rows []sqlc.ImportJobRow) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	const sheet = SheetName
	if _, err := f.NewSheet(sheet); err != nil {
		return nil, fmt.Errorf("new sheet: %w", err)
	}
	_ = f.DeleteSheet("Sheet1")

	// Header row = TemplateHeaders + error_reason. Same column order so the
	// operator can paste fixes back into the import template.
	headers := append([]string{}, TemplateHeaders...)
	headers = append(headers, "error_reason")
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := f.SetCellStr(sheet, cell, h); err != nil {
			return nil, fmt.Errorf("set header %s: %w", h, err)
		}
	}

	// Header style — same navy fill as the template so visually-correlated.
	headerStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "#FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#1E3A8A"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	if err != nil {
		return nil, fmt.Errorf("new header style: %w", err)
	}
	endCell, _ := excelize.CoordinatesToCellName(len(headers), 1)
	if err := f.SetCellStyle(sheet, "A1", endCell, headerStyle); err != nil {
		return nil, fmt.Errorf("apply header style: %w", err)
	}

	// Text format on hex columns (preserves leading-zero DevEUI).
	textStyle, err := f.NewStyle(&excelize.Style{NumFmt: 49})
	if err != nil {
		return nil, fmt.Errorf("new text style: %w", err)
	}
	for _, col := range []string{"A", "F", "G", "H"} {
		if err := f.SetColStyle(sheet, col, textStyle); err != nil {
			return nil, fmt.Errorf("apply text style to %s: %w", col, err)
		}
	}

	// Data rows — iterate persisted import_job_row rows, decode raw_payload
	// JSONB, write cells.
	rowIdx := 2
	for _, r := range rows {
		raw := map[string]any{}
		if len(r.RawPayload) > 0 {
			if err := json.Unmarshal(r.RawPayload, &raw); err != nil {
				return nil, fmt.Errorf("row %d raw_payload unmarshal: %w", r.RowIndex, err)
			}
		}
		for col, h := range TemplateHeaders {
			cell, _ := excelize.CoordinatesToCellName(col+1, rowIdx)
			val := ""
			if v, ok := raw[h]; ok && v != nil {
				val = fmt.Sprintf("%v", v)
			}
			// SetCellStr (not SetCellValue) — T-3-41: prevents `=cmd|...`
			// pasted by an adversary from being evaluated when the operator
			// opens errors.xlsx in Excel.
			if err := f.SetCellStr(sheet, cell, val); err != nil {
				return nil, fmt.Errorf("set cell %s: %w", cell, err)
			}
		}
		// error_reason column.
		reasonCell, _ := excelize.CoordinatesToCellName(len(TemplateHeaders)+1, rowIdx)
		reason := ""
		if r.Reason != nil {
			reason = *r.Reason
		}
		if err := f.SetCellStr(sheet, reasonCell, reason); err != nil {
			return nil, fmt.Errorf("set reason cell %s: %w", reasonCell, err)
		}
		rowIdx++
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("write buffer: %w", err)
	}
	return buf.Bytes(), nil
}
