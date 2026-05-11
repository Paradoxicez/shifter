package importpkg

// Phase 3 Plan 03-05 — template.go: generates the canonical import template.
// Tests assert the template carries every UX invariant from D-04 (header
// order, NumFmt text-format on hex columns, OTAA/ABP dropdown, tooltips).

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/xuri/excelize/v2"

	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// TestGenerateTemplate_Headers — emitted file has the 10-column header row
// in the canonical order on the "Devices" sheet.
func TestGenerateTemplate_Headers(t *testing.T) {
	t.Parallel()
	b, err := GenerateTemplate()
	if err != nil {
		t.Fatalf("GenerateTemplate: %v", err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	defer func() { _ = f.Close() }()

	rows, err := f.GetRows(SheetName)
	if err != nil {
		t.Fatalf("GetRows: %v", err)
	}
	if len(rows) == 0 {
		t.Fatalf("template empty")
	}
	if len(rows[0]) != len(TemplateHeaders) {
		t.Fatalf("header count = %d, want %d", len(rows[0]), len(TemplateHeaders))
	}
	for i, h := range TemplateHeaders {
		if rows[0][i] != h {
			t.Errorf("header[%d] = %q, want %q", i, rows[0][i], h)
		}
	}
}

// TestGenerateTemplate_DevEUITextFormat — text format (NumFmt=49) applied
// to dev_eui (A), join_eui (F), app_key (G), dev_addr (H) columns. Verified
// by re-opening the workbook + comparing the column style's number format.
func TestGenerateTemplate_DevEUITextFormat(t *testing.T) {
	t.Parallel()
	b, err := GenerateTemplate()
	if err != nil {
		t.Fatalf("GenerateTemplate: %v", err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	defer func() { _ = f.Close() }()

	for _, col := range []string{"A", "F", "G", "H"} {
		styleID, err := f.GetColStyle(SheetName, col)
		if err != nil {
			t.Fatalf("GetColStyle %s: %v", col, err)
		}
		if styleID == 0 {
			t.Errorf("col %s has no style — NumFmt=49 not applied", col)
			continue
		}
		style, err := f.GetStyle(styleID)
		if err != nil {
			t.Fatalf("GetStyle for col %s: %v", col, err)
		}
		if style.NumFmt != 49 {
			t.Errorf("col %s NumFmt = %d, want 49 (text)", col, style.NumFmt)
		}
	}
}

// TestGenerateTemplate_ActivationDropdown — activation_mode column (E) has
// a data-validation list with [OTAA, ABP]. excelize exposes data
// validations via GetDataValidations on the sheet.
func TestGenerateTemplate_ActivationDropdown(t *testing.T) {
	t.Parallel()
	b, err := GenerateTemplate()
	if err != nil {
		t.Fatalf("GenerateTemplate: %v", err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	defer func() { _ = f.Close() }()

	dvs, err := f.GetDataValidations(SheetName)
	if err != nil {
		t.Fatalf("GetDataValidations: %v", err)
	}
	if len(dvs) == 0 {
		t.Fatal("no data validations found on Devices sheet")
	}
	// Find a validation that references column E and includes OTAA + ABP.
	foundOTAA, foundABP, foundColE := false, false, false
	for _, dv := range dvs {
		if dv.Sqref == "E2:E1000" {
			foundColE = true
		}
		// excelize round-trips the dropdown as a comma-joined Formula1
		// string like `"OTAA,ABP"`. Substring check is enough.
		if dv.Formula1 != "" {
			if containsSubstr(dv.Formula1, "OTAA") {
				foundOTAA = true
			}
			if containsSubstr(dv.Formula1, "ABP") {
				foundABP = true
			}
		}
	}
	if !foundColE {
		t.Errorf("no data validation found for E2:E1000")
	}
	if !foundOTAA {
		t.Errorf("data validation does not contain OTAA option")
	}
	if !foundABP {
		t.Errorf("data validation does not contain ABP option")
	}
}

// TestGenerateTemplate_RoundTrip — generate, parse, verify the parser
// returns 0 rows (template has only the header) and no errors.
func TestGenerateTemplate_RoundTrip(t *testing.T) {
	t.Parallel()
	b, err := GenerateTemplate()
	if err != nil {
		t.Fatalf("GenerateTemplate: %v", err)
	}
	rows, err := ParseXLSX(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("ParseXLSX on generated template: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("template parsed to %d rows, want 0 (header only)", len(rows))
	}
}

// TestGenerateErrorsXLSX — given a job + 2 error rows, emitted XLSX has
// the original schema + appended error_reason column with the row's reason.
func TestGenerateErrorsXLSX(t *testing.T) {
	t.Parallel()
	reason1 := "invalid_dev_eui"
	reason2 := "missing_site"
	raw1, _ := json.Marshal(map[string]any{
		"dev_eui":         "bad",
		"name":            "row-3",
		"device_profile":  "axioma_w1",
		"activation_mode": "OTAA",
	})
	raw2, _ := json.Marshal(map[string]any{
		"dev_eui":         "70b3d59999000010",
		"name":            "row-8",
		"device_profile":  "axioma_w1",
		"activation_mode": "OTAA",
		"site_id":         "",
	})
	rows := []sqlc.ImportJobRow{
		{RowIndex: 3, RawPayload: raw1, Status: sqlc.ImportJobRowStatusInvalid, Reason: &reason1},
		{RowIndex: 8, RawPayload: raw2, Status: sqlc.ImportJobRowStatusInvalid, Reason: &reason2},
	}

	b, err := GenerateErrorsXLSX(rows)
	if err != nil {
		t.Fatalf("GenerateErrorsXLSX: %v", err)
	}

	f, err := excelize.OpenReader(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("OpenReader: %v", err)
	}
	defer func() { _ = f.Close() }()

	got, err := f.GetRows(SheetName)
	if err != nil {
		t.Fatalf("GetRows: %v", err)
	}
	// Expect header + 2 data rows = 3.
	if len(got) != 3 {
		t.Fatalf("got %d rows, want 3", len(got))
	}
	// Header must end with error_reason.
	if got[0][len(got[0])-1] != "error_reason" {
		t.Errorf("last header col = %q, want error_reason", got[0][len(got[0])-1])
	}
	// Row 1 (data row 1) dev_eui = "bad", error_reason = "invalid_dev_eui".
	if got[1][0] != "bad" {
		t.Errorf("row 1 dev_eui = %q, want bad", got[1][0])
	}
	if got[1][len(got[1])-1] != "invalid_dev_eui" {
		t.Errorf("row 1 error_reason = %q, want invalid_dev_eui", got[1][len(got[1])-1])
	}
	if got[2][len(got[2])-1] != "missing_site" {
		t.Errorf("row 2 error_reason = %q, want missing_site", got[2][len(got[2])-1])
	}
}

// containsSubstr is a tiny helper so we don't pull in strings in the test
// file just for one substring check.
func containsSubstr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
