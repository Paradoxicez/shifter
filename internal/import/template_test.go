package importpkg

// Phase 3 Wave 1 — template.go: generates the canonical XLSX template for
// operators to download from /admin/imports. Must produce the golden file
// committed at web/tests/fixtures/import-template.xlsx (byte-identical or
// at least equivalent in structure).

import "testing"

// TestGenerateTemplate_Headers — emitted file has the 10-column header row
// in the canonical order.
func TestGenerateTemplate_Headers(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/import/template.go (03-VALIDATION row template_test.TestGenerateTemplate_Headers)")
}

// TestGenerateTemplate_DevEUITextFormat — col A (dev_eui), col F (join_eui),
// col G (app_key), col H (dev_addr) have NumFmt=49 (text) so Excel doesn't
// convert 16-hex EUIs to scientific notation.
func TestGenerateTemplate_DevEUITextFormat(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/import/template.go (03-VALIDATION row template_test.TestGenerateTemplate_DevEUITextFormat)")
}

// TestGenerateTemplate_ActivationDropdown — col E (activation_mode) has a
// data-validation list constrained to {OTAA, ABP}.
func TestGenerateTemplate_ActivationDropdown(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/import/template.go (03-VALIDATION row template_test.TestGenerateTemplate_ActivationDropdown)")
}

// TestGenerateTemplate_RoundTrip — write a row to the generated template,
// re-parse via parser_xlsx, recover identical row data. Cement the
// generator+parser contract.
func TestGenerateTemplate_RoundTrip(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/import/template.go (03-VALIDATION row template_test.TestGenerateTemplate_RoundTrip)")
}
