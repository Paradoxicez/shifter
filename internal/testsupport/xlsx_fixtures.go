package testsupport

// XLSX byte-slice fixtures for the Phase 3 bulk-import parser tests.
//
// Each helper returns an in-memory XLSX file ([]byte) shaped for a specific
// scenario in 03-VALIDATION.md §dryrun_test.go / §commit_test.go:
//
//   - HappyFiveRows()           — 5 valid OTAA rows, all distinct
//   - MixedTenRows()            — 5 valid + 5 invalid (assorted failures)
//   - IntraFileDuplicate()      — row 2 and row 3 share dev_eui (D-08)
//   - AlreadyExistsFixture(eui) — row 1 dev_eui == eui (pre-seeded, D-06)
//   - MalformedEUIFixture()     — row 1 has a non-hex char in dev_eui
//
// The header shape mirrors the import schema decided in D-04:
//
//	dev_eui | name | device_profile | site_id | activation_mode |
//	join_eui | app_key | dev_addr | f_cnt_up | f_cnt_down
//
// Wave 1 (parser_xlsx_test.go) is the consumer; if the schema changes during
// Wave 1, update this fixture in lock-step.

import (
	"bytes"
	"fmt"

	"github.com/xuri/excelize/v2"
)

// xlsxImportHeaders is the canonical Phase 3 bulk-import column order.
// Keep in sync with internal/import/parser_xlsx.go (Wave 1) — the parser
// reads by header name, but for round-trip determinism we pin the column
// order here as well.
var xlsxImportHeaders = []string{
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

// importRow is the in-memory representation of a single XLSX row. Empty
// strings render as blank cells.
type importRow struct {
	devEUI         string
	name           string
	deviceProfile  string
	siteID         string
	activationMode string // "OTAA" or "ABP"
	joinEUI        string
	appKey         string
	devAddr        string
	fCntUp         string
	fCntDown       string
}

func (r importRow) cells() []any {
	return []any{
		r.devEUI, r.name, r.deviceProfile, r.siteID, r.activationMode,
		r.joinEUI, r.appKey, r.devAddr, r.fCntUp, r.fCntDown,
	}
}

// buildXLSX returns the bytes of a single-sheet "Devices" workbook with the
// canonical headers + the provided rows. Cell A1 is the first header.
//
// The DevEUI column (col A) is left unstyled here — the *template generator*
// (internal/import/template.go in Wave 1) is responsible for applying the
// NumFmt=49 text format that prevents Excel from interpreting 16-hex DevEUIs
// as scientific notation. Test fixtures simulate already-correct input.
func buildXLSX(rows []importRow) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	const sheet = "Devices"
	if _, err := f.NewSheet(sheet); err != nil {
		return nil, fmt.Errorf("new sheet: %w", err)
	}
	// excelize creates a default "Sheet1" — remove it so the file has only
	// the "Devices" tab (matches the template generator output).
	_ = f.DeleteSheet("Sheet1")

	// Headers row 1.
	for i, h := range xlsxImportHeaders {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := f.SetCellStr(sheet, cell, h); err != nil {
			return nil, fmt.Errorf("set header %s: %w", h, err)
		}
	}

	// Data rows, starting at row 2.
	for rowIdx, r := range rows {
		for colIdx, v := range r.cells() {
			cell, _ := excelize.CoordinatesToCellName(colIdx+1, rowIdx+2)
			if err := f.SetCellValue(sheet, cell, v); err != nil {
				return nil, fmt.Errorf("set cell %s: %w", cell, err)
			}
		}
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("write buffer: %w", err)
	}
	return buf.Bytes(), nil
}

// mustBuildXLSX panics on error — fixtures are unit-test inputs and a build
// failure means the test code is wrong, not the system under test.
func mustBuildXLSX(rows []importRow) []byte {
	b, err := buildXLSX(rows)
	if err != nil {
		panic(fmt.Sprintf("testsupport: build XLSX fixture: %v", err))
	}
	return b
}

// HappyFiveRows returns 5 valid OTAA rows pointing at the same device profile
// + site, with the five canonical DevEUIs from eui_fixtures.go. Each row is
// individually valid; dry-run should mark all 5 `valid`, commit should
// create all 5 (D-06 happy path).
//
// Test expectation: `valid=5, invalid=0` after dry-run; after commit:
// `created=5, already_exists=0, errored=0`.
func HappyFiveRows() []byte {
	rows := make([]importRow, 0, 5)
	for i, eui := range ValidDevEUIs() {
		rows = append(rows, importRow{
			devEUI:         eui,
			name:           fmt.Sprintf("dev-%d", i+1),
			deviceProfile:  "axioma_w1",
			siteID:         "site-A",
			activationMode: "OTAA",
			joinEUI:        ValidJoinEUI,
			appKey:         ValidAppKey,
		})
	}
	return mustBuildXLSX(rows)
}

// MixedTenRows returns 5 valid + 5 invalid rows. Used by dry-run tests to
// confirm per-row outcome bookkeeping (D-09 expandable reason column).
//
// Invalid rows cover:
//   - row 6: malformed dev_eui
//   - row 7: short dev_eui
//   - row 8: missing site_id (required)
//   - row 9: unknown device_profile slug
//   - row 10: OTAA mode with no app_key
func MixedTenRows() []byte {
	rows := []importRow{
		// 5 valid
		{devEUI: ValidDevEUI1, name: "ok-1", deviceProfile: "axioma_w1", siteID: "site-A", activationMode: "OTAA", joinEUI: ValidJoinEUI, appKey: ValidAppKey},
		{devEUI: ValidDevEUI2, name: "ok-2", deviceProfile: "axioma_w1", siteID: "site-A", activationMode: "OTAA", joinEUI: ValidJoinEUI, appKey: ValidAppKey},
		{devEUI: ValidDevEUI3, name: "ok-3", deviceProfile: "axioma_w1", siteID: "site-A", activationMode: "OTAA", joinEUI: ValidJoinEUI, appKey: ValidAppKey},
		{devEUI: ValidDevEUI4, name: "ok-4", deviceProfile: "axioma_w1", siteID: "site-A", activationMode: "OTAA", joinEUI: ValidJoinEUI, appKey: ValidAppKey},
		{devEUI: ValidDevEUI5, name: "ok-5", deviceProfile: "axioma_w1", siteID: "site-A", activationMode: "OTAA", joinEUI: ValidJoinEUI, appKey: ValidAppKey},
		// 5 invalid (each row models a single distinct failure)
		{devEUI: MalformedEUI, name: "bad-eui", deviceProfile: "axioma_w1", siteID: "site-A", activationMode: "OTAA", joinEUI: ValidJoinEUI, appKey: ValidAppKey},
		{devEUI: ShortEUI, name: "short-eui", deviceProfile: "axioma_w1", siteID: "site-A", activationMode: "OTAA", joinEUI: ValidJoinEUI, appKey: ValidAppKey},
		{devEUI: "70b3d59999000010", name: "no-site", deviceProfile: "axioma_w1", siteID: "", activationMode: "OTAA", joinEUI: ValidJoinEUI, appKey: ValidAppKey},
		{devEUI: "70b3d59999000011", name: "bad-profile", deviceProfile: "nonexistent_profile", siteID: "site-A", activationMode: "OTAA", joinEUI: ValidJoinEUI, appKey: ValidAppKey},
		{devEUI: "70b3d59999000012", name: "otaa-no-key", deviceProfile: "axioma_w1", siteID: "site-A", activationMode: "OTAA", joinEUI: ValidJoinEUI, appKey: ""},
	}
	return mustBuildXLSX(rows)
}

// IntraFileDuplicate returns 3 rows where row 2 and row 3 share the same
// dev_eui. Tests the D-08 intra-file dedup: the parser must reject row 3
// with reason "duplicate_in_file" while keeping row 2 as the canonical row.
//
// Test expectation after dry-run:
//
//	row 1: valid
//	row 2: valid
//	row 3: invalid, reason "duplicate_in_file"
func IntraFileDuplicate() []byte {
	rows := []importRow{
		{devEUI: ValidDevEUI1, name: "first", deviceProfile: "axioma_w1", siteID: "site-A", activationMode: "OTAA", joinEUI: ValidJoinEUI, appKey: ValidAppKey},
		{devEUI: ValidDevEUI2, name: "second", deviceProfile: "axioma_w1", siteID: "site-A", activationMode: "OTAA", joinEUI: ValidJoinEUI, appKey: ValidAppKey},
		{devEUI: ValidDevEUI2, name: "duplicate", deviceProfile: "axioma_w1", siteID: "site-A", activationMode: "OTAA", joinEUI: ValidJoinEUI, appKey: ValidAppKey},
	}
	return mustBuildXLSX(rows)
}

// AlreadyExistsFixture returns a 2-row file where row 1's dev_eui equals
// `seedEUI` (the caller is expected to pre-seed that device in CS or PG
// before running dry-run). The dry-run / commit must mark row 1 as outcome
// `already_exists` (D-06), NOT errored.
//
// Test expectation:
//
//	row 1: outcome=already_exists, no action taken
//	row 2: outcome=created
func AlreadyExistsFixture(seedEUI string) []byte {
	rows := []importRow{
		{devEUI: seedEUI, name: "preexisting", deviceProfile: "axioma_w1", siteID: "site-A", activationMode: "OTAA", joinEUI: ValidJoinEUI, appKey: ValidAppKey},
		{devEUI: ValidDevEUI5, name: "new", deviceProfile: "axioma_w1", siteID: "site-A", activationMode: "OTAA", joinEUI: ValidJoinEUI, appKey: ValidAppKey},
	}
	return mustBuildXLSX(rows)
}

// MalformedEUIFixture returns a 2-row file where row 1's dev_eui contains a
// non-hex character. The parser must reject row 1 with reason
// `invalid_dev_eui` and continue to row 2 (D-09 per-row independence).
func MalformedEUIFixture() []byte {
	rows := []importRow{
		{devEUI: MalformedEUI, name: "bad", deviceProfile: "axioma_w1", siteID: "site-A", activationMode: "OTAA", joinEUI: ValidJoinEUI, appKey: ValidAppKey},
		{devEUI: ValidDevEUI1, name: "good", deviceProfile: "axioma_w1", siteID: "site-A", activationMode: "OTAA", joinEUI: ValidJoinEUI, appKey: ValidAppKey},
	}
	return mustBuildXLSX(rows)
}

// ImportTemplateBytes returns the canonical XLSX template that operators
// download from /admin/imports. Headers only, no data rows, with the DevEUI
// column flagged as text-format (NumFmt=49) so Excel doesn't mangle 16-hex
// EUIs into scientific notation on save.
//
// This is the *golden* fixture asserted-against by parser_xlsx_test.go's
// template round-trip test. Wave 1's template generator must produce an
// equivalent file (same headers, same text-format on col A).
func ImportTemplateBytes() ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	const sheet = "Devices"
	if _, err := f.NewSheet(sheet); err != nil {
		return nil, fmt.Errorf("new sheet: %w", err)
	}
	_ = f.DeleteSheet("Sheet1")

	// Headers.
	for i, h := range xlsxImportHeaders {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := f.SetCellStr(sheet, cell, h); err != nil {
			return nil, fmt.Errorf("set header %s: %w", h, err)
		}
	}

	// Apply NumFmt=49 (text) to dev_eui (col A), join_eui (col F), app_key
	// (col G), dev_addr (col H) so Excel preserves leading-zero / all-hex
	// values verbatim. NumFmt 49 is the built-in "Text" format code.
	textNumFmt := 49
	textStyle, err := f.NewStyle(&excelize.Style{NumFmt: textNumFmt})
	if err != nil {
		return nil, fmt.Errorf("new text style: %w", err)
	}
	for _, col := range []string{"A", "F", "G", "H"} {
		// Apply to a generous 10,000-row range so operator paste-down is
		// covered without surprise reformatting.
		if err := f.SetColStyle(sheet, col, textStyle); err != nil {
			return nil, fmt.Errorf("set col style %s: %w", col, err)
		}
	}

	// Data validation dropdown on activation_mode (col E): OTAA | ABP.
	dv := excelize.NewDataValidation(true)
	dv.Sqref = "E2:E10000"
	if err := dv.SetDropList([]string{"OTAA", "ABP"}); err != nil {
		return nil, fmt.Errorf("set drop list: %w", err)
	}
	if err := f.AddDataValidation(sheet, dv); err != nil {
		return nil, fmt.Errorf("add data validation: %w", err)
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, fmt.Errorf("write buffer: %w", err)
	}
	// Guarantee non-empty for assertions even on accidentally truncated buf.
	out := bytes.Clone(buf.Bytes())
	return out, nil
}
