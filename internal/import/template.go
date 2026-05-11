package importpkg

// Canonical import-template XLSX generator. DEV-08 + D-04a: operators
// download this file, paste device fleet data, upload via POST /api/imports.
//
// The template encodes the schema invariants in Excel itself:
//   - NumFmt 49 (text format) on hex columns prevents Excel from converting
//     "70b3d59999000001" to scientific notation "7.03E+15" on save.
//   - Data validation list on activation_mode restricts cells to OTAA / ABP.
//   - Header tooltips (AddComment) explain each field without forcing a docs
//     trip — operators see the field meaning in Excel itself.
//
// Header order matches xlsxImportHeaders in testsupport/xlsx_fixtures.go so
// the template + parser + fixtures round-trip cleanly (template_test.go's
// RoundTrip assertion catches drift).

import (
	"fmt"

	"github.com/xuri/excelize/v2"
)

// TemplateHeaders is the canonical Phase 3 bulk-import column order. Mirrors
// testsupport.xlsxImportHeaders (intentionally — the fixture is what tests
// assert against the parser, and the template is what operators actually
// fill in; both must agree).
var TemplateHeaders = []string{
	"dev_eui",
	"name",
	"device_profile",
	"site_id",
	"activation_mode",
	"join_eui",
	"app_key",
	"dev_addr",
	"f_cnt_up",
	"f_cnt_down",
}

// headerTooltips maps each header to the Excel cell-comment text the
// operator sees when hovering. Keeps the schema documentation co-located
// with the template generator.
var headerTooltips = map[string]string{
	"dev_eui":         "DevEUI: 16 hex chars (big-endian). Example: 70b3d59999000001. Required.",
	"name":            "Operator-visible device name. Required.",
	"device_profile":  "Device profile slug (e.g. axioma_w1). Must already exist. Required.",
	"site_id":         "Site UUID or site name. Required.",
	"activation_mode": "OTAA or ABP. If empty, inferred from filled key columns.",
	"join_eui":        "OTAA join identifier (also called AppEUI in 1.0.x). 16 hex chars.",
	"app_key":         "OTAA root key. 32 hex chars.",
	"dev_addr":        "ABP device address. 8 hex chars.",
	"f_cnt_up":        "ABP frame-counter up (optional; default 0).",
	"f_cnt_down":      "ABP frame-counter down (optional; default 0).",
}

// columnWidths is the operator-readability sizing. Picked once based on
// expected paste-down content lengths; not exposed as a knob.
var columnWidths = map[string]float64{
	"A": 22, // dev_eui
	"B": 28, // name
	"C": 20, // device_profile
	"D": 28, // site_id
	"E": 16, // activation_mode
	"F": 22, // join_eui
	"G": 36, // app_key
	"H": 14, // dev_addr
	"I": 10, // f_cnt_up
	"J": 10, // f_cnt_down
}

// GenerateTemplate returns the canonical import-template.xlsx as a byte
// slice ready to be written to an HTTP response. Errors propagate from
// excelize — they would indicate a generator bug, not operator input.
func GenerateTemplate() ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	const sheet = SheetName
	if _, err := f.NewSheet(sheet); err != nil {
		return nil, fmt.Errorf("new sheet: %w", err)
	}
	// excelize creates "Sheet1" by default — drop it so the template has
	// only the canonical "Devices" tab.
	_ = f.DeleteSheet("Sheet1")

	// 1. Headers row.
	for i, h := range TemplateHeaders {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := f.SetCellStr(sheet, cell, h); err != nil {
			return nil, fmt.Errorf("set header %s: %w", h, err)
		}
	}

	// 2. Header style — bold white text on navy fill, centred. Hex
	// 1E3A8A is the Shifter navy palette anchor (matches the dashboard
	// primary fill — UI-SPEC §Tokens).
	headerStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "#FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#1E3A8A"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	if err != nil {
		return nil, fmt.Errorf("new header style: %w", err)
	}
	endCell, _ := excelize.CoordinatesToCellName(len(TemplateHeaders), 1)
	if err := f.SetCellStyle(sheet, "A1", endCell, headerStyle); err != nil {
		return nil, fmt.Errorf("apply header style: %w", err)
	}

	// 3. Text format (NumFmt=49) on hex columns. Without this, Excel mangles
	// 16-hex DevEUIs to scientific notation on save — silent corruption.
	// Columns: A (dev_eui), F (join_eui), G (app_key), H (dev_addr).
	textStyle, err := f.NewStyle(&excelize.Style{NumFmt: 49})
	if err != nil {
		return nil, fmt.Errorf("new text style: %w", err)
	}
	for _, col := range []string{"A", "F", "G", "H"} {
		if err := f.SetColStyle(sheet, col, textStyle); err != nil {
			return nil, fmt.Errorf("apply text style to %s: %w", col, err)
		}
	}

	// 4. Column widths.
	for col, width := range columnWidths {
		if err := f.SetColWidth(sheet, col, col, width); err != nil {
			return nil, fmt.Errorf("set width %s: %w", col, err)
		}
	}

	// 5. Header tooltips. AddComment is the established pattern for
	// per-cell hints (rich-text-on-comments is not portable across Excel /
	// LibreOffice).
	for i, h := range TemplateHeaders {
		tip, ok := headerTooltips[h]
		if !ok {
			continue
		}
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := f.AddComment(sheet, excelize.Comment{
			Cell:   cell,
			Author: "Shifter",
			Paragraph: []excelize.RichTextRun{
				{Text: tip},
			},
		}); err != nil {
			return nil, fmt.Errorf("add comment %s: %w", h, err)
		}
	}

	// 6. Activation-mode dropdown on column E. Sqref E2:E1000 — covers
	// any realistic paste-down without forcing the operator to expand the
	// range.
	dv := excelize.NewDataValidation(true)
	dv.Sqref = "E2:E1000"
	if err := dv.SetDropList([]string{"OTAA", "ABP"}); err != nil {
		return nil, fmt.Errorf("set drop list: %w", err)
	}
	dv.SetError(excelize.DataValidationErrorStyleStop, "Invalid activation", "Must be OTAA or ABP.")
	if err := f.AddDataValidation(sheet, dv); err != nil {
		return nil, fmt.Errorf("add data validation: %w", err)
	}

	// 7. Freeze header row so the operator scrolls past row 1 cleanly.
	if err := f.SetPanes(sheet, &excelize.Panes{
		Freeze:      true,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	}); err != nil {
		return nil, fmt.Errorf("set panes: %w", err)
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("write buffer: %w", err)
	}
	return buf.Bytes(), nil
}
