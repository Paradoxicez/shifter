package importpkg

// Phase 3 Wave 1 — dry-run validation pass (no CS or PG writes).
// D-06 / D-08 / D-09: per-row outcome + reason, intra-file dup detection,
// pre-existing detection (against PG + CS).

import "testing"

// TestDryRun_FormatErrors — malformed EUI / key surfaces as outcome=invalid
// with reason="invalid_dev_eui" or "invalid_app_key".
func TestDryRun_FormatErrors(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/import/dryrun.go (03-VALIDATION row dryrun_test.TestDryRun_FormatErrors)")
}

// TestDryRun_IntraFileDuplicate — testsupport.IntraFileDuplicate() fixture:
// row 3 reported as invalid with reason="duplicate_in_file" and a
// `duplicate_of_row=2` hint.
func TestDryRun_IntraFileDuplicate(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/import/dryrun.go (03-VALIDATION row dryrun_test.TestDryRun_IntraFileDuplicate)")
}

// TestDryRun_PreExistingAlreadyExists — testsupport.AlreadyExistsFixture()
// fixture (seeded in PG): row 1 reported as outcome=already_exists, NOT
// invalid (D-06).
func TestDryRun_PreExistingAlreadyExists(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/import/dryrun.go (03-VALIDATION row dryrun_test.TestDryRun_PreExistingAlreadyExists)")
}

// TestDryRun_MissingSite — site_id refers to an unknown site → invalid
// with reason="missing_site".
func TestDryRun_MissingSite(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/import/dryrun.go (03-VALIDATION row dryrun_test.TestDryRun_MissingSite)")
}

// TestDryRun_MissingProfile — device_profile slug unknown → invalid with
// reason="missing_device_profile".
func TestDryRun_MissingProfile(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/import/dryrun.go (03-VALIDATION row dryrun_test.TestDryRun_MissingProfile)")
}

// TestDryRun_RequiredFields — empty required cells (dev_eui, name,
// activation_mode) → invalid with reason="missing_required".
func TestDryRun_RequiredFields(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/import/dryrun.go (03-VALIDATION row dryrun_test.TestDryRun_RequiredFields)")
}

// TestDryRun_KeyLengthPerMode — OTAA requires app_key (32 hex) and
// join_eui (16 hex); ABP requires dev_addr (8 hex) and session keys.
// Length mismatches per mode produce mode-specific reasons.
func TestDryRun_KeyLengthPerMode(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/import/dryrun.go (03-VALIDATION row dryrun_test.TestDryRun_KeyLengthPerMode)")
}
