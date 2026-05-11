package importpkg

// Phase 3 Plan 03-05 — dry-run validator integration tests against
// testcontainer Postgres. Covers D-06 / D-08 / D-09 outcomes:
//
//   - invalid (format, missing required, missing FK, intra-file dup)
//   - already_exists (pre-existing dev_eui in PG, NOT invalid per D-06)

import (
	"context"
	"strings"
	"testing"

	"github.com/shifter-io/shifter/internal/testsupport"
)

// TestDryRun_FormatErrors — row with malformed dev_eui → invalid /
// invalid_dev_eui.
func TestDryRun_FormatErrors(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	rows := rowsFromFixture(t, f, testsupport.MalformedEUIFixture())

	out, err := NewDryRunDeps(f.q).Validate(ctx, rows)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("got %d outcomes, want 2", len(out))
	}
	if out[0].Status != StatusInvalid {
		t.Errorf("row 0: status %s, want invalid", out[0].Status)
	}
	if !strings.Contains(out[0].Reason, "invalid_dev_eui") {
		t.Errorf("row 0: reason %q, want invalid_dev_eui", out[0].Reason)
	}
	if out[1].Status != StatusValid {
		t.Errorf("row 1: status %s, want valid (good dev_eui)", out[1].Status)
	}
}

// TestDryRun_IntraFileDuplicate — second occurrence of a dev_eui in the
// upload is invalid; first remains valid (D-08).
func TestDryRun_IntraFileDuplicate(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	rows := rowsFromFixture(t, f, testsupport.IntraFileDuplicate())

	out, err := NewDryRunDeps(f.q).Validate(ctx, rows)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("got %d outcomes, want 3", len(out))
	}
	if out[0].Status != StatusValid {
		t.Errorf("row 0: status %s, want valid", out[0].Status)
	}
	if out[1].Status != StatusValid {
		t.Errorf("row 1: status %s, want valid", out[1].Status)
	}
	if out[2].Status != StatusInvalid {
		t.Errorf("row 2: status %s, want invalid", out[2].Status)
	}
	if !strings.Contains(out[2].Reason, "duplicate_in_file") {
		t.Errorf("row 2: reason %q, want duplicate_in_file", out[2].Reason)
	}
}

// TestDryRun_PreExistingAlreadyExists — seeded device in PG → already_exists
// outcome (D-06). NOT invalid.
func TestDryRun_PreExistingAlreadyExists(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	f.seedDevice(t, testsupport.ValidDevEUI1)

	rows := rowsFromFixture(t, f, testsupport.AlreadyExistsFixture(testsupport.ValidDevEUI1))

	out, err := NewDryRunDeps(f.q).Validate(ctx, rows)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("got %d outcomes, want 2", len(out))
	}
	if out[0].Status != StatusAlreadyExists {
		t.Errorf("row 0: status %s, want already_exists", out[0].Status)
	}
	if out[1].Status != StatusValid {
		t.Errorf("row 1: status %s, want valid", out[1].Status)
	}
}

// TestDryRun_MissingSite — site_id empty → invalid / missing_required:site_id.
func TestDryRun_MissingSite(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	rows := rowsFromFixture(t, f, testsupport.MixedTenRows())

	out, err := NewDryRunDeps(f.q).Validate(ctx, rows)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	// MixedTenRows row index 8 (slice idx 7, file row 9) has site_id="" and
	// is expected to be invalid with missing_required:site_id. We assert by
	// scanning for that exact reason.
	foundMissingSite := false
	for _, o := range out {
		if o.Status == StatusInvalid && strings.Contains(o.Reason, "missing_required:site_id") {
			foundMissingSite = true
		}
	}
	if !foundMissingSite {
		t.Errorf("expected at least one missing_required:site_id outcome in mixed-ten-rows")
	}
}

// TestDryRun_MissingProfile — unknown device_profile slug → invalid /
// missing_device_profile.
func TestDryRun_MissingProfile(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	rows := rowsFromFixture(t, f, testsupport.MixedTenRows())

	out, err := NewDryRunDeps(f.q).Validate(ctx, rows)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	foundMissingProfile := false
	for _, o := range out {
		if o.Status == StatusInvalid && strings.Contains(o.Reason, "missing_device_profile") {
			foundMissingProfile = true
		}
	}
	if !foundMissingProfile {
		t.Errorf("expected at least one missing_device_profile outcome")
	}
}

// TestDryRun_RequiredFields — synthesise rows missing the name cell to
// confirm the required-field reason path.
func TestDryRun_RequiredFields(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()

	rows := []ParsedRow{
		// missing name
		{RowIndex: 2, Raw: map[string]string{
			"dev_eui":         testsupport.ValidDevEUI1,
			"name":            "",
			"device_profile":  "axioma_w1",
			"site_id":         f.siteName,
			"activation_mode": "OTAA",
			"join_eui":        testsupport.ValidJoinEUI,
			"app_key":         testsupport.ValidAppKey,
		}},
		// missing dev_eui
		{RowIndex: 3, Raw: map[string]string{
			"dev_eui":         "",
			"name":            "x",
			"device_profile":  "axioma_w1",
			"site_id":         f.siteName,
			"activation_mode": "OTAA",
			"join_eui":        testsupport.ValidJoinEUI,
			"app_key":         testsupport.ValidAppKey,
		}},
	}

	out, err := NewDryRunDeps(f.q).Validate(ctx, rows)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if out[0].Status != StatusInvalid {
		t.Errorf("row 0 missing name: status %s, want invalid", out[0].Status)
	}
	if !strings.Contains(out[0].Reason, "missing_required:name") {
		t.Errorf("row 0 reason %q, want missing_required:name", out[0].Reason)
	}
	if out[1].Status != StatusInvalid {
		t.Errorf("row 1 missing dev_eui: status %s, want invalid", out[1].Status)
	}
	if !strings.Contains(out[1].Reason, "missing_required:dev_eui") {
		t.Errorf("row 1 reason %q, want missing_required:dev_eui", out[1].Reason)
	}
}

// TestDryRun_KeyLengthPerMode — mode-specific key length / presence checks.
func TestDryRun_KeyLengthPerMode(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()

	rows := []ParsedRow{
		// OTAA with bad-length app_key (30 hex).
		{RowIndex: 2, Raw: map[string]string{
			"dev_eui":         testsupport.ValidDevEUI1,
			"name":            "otaa-short-key",
			"device_profile":  "axioma_w1",
			"site_id":         f.siteName,
			"activation_mode": "OTAA",
			"join_eui":        testsupport.ValidJoinEUI,
			"app_key":         testsupport.AppKeyShort,
		}},
		// ABP missing dev_addr.
		{RowIndex: 3, Raw: map[string]string{
			"dev_eui":         testsupport.ValidDevEUI2,
			"name":            "abp-no-addr",
			"device_profile":  "axioma_w1",
			"site_id":         f.siteName,
			"activation_mode": "ABP",
			"nwk_s_key":       testsupport.ValidNwkSEncKey,
			"app_s_key":       testsupport.ValidAppSKey,
		}},
		// ABP with bad nwk_s_key length.
		{RowIndex: 4, Raw: map[string]string{
			"dev_eui":         testsupport.ValidDevEUI3,
			"name":            "abp-short-nwk",
			"device_profile":  "axioma_w1",
			"site_id":         f.siteName,
			"activation_mode": "ABP",
			"dev_addr":        testsupport.ValidDevAddr,
			"nwk_s_key":       "1111111111111111111111111111111", // 31 hex
			"app_s_key":       testsupport.ValidAppSKey,
		}},
	}

	out, err := NewDryRunDeps(f.q).Validate(ctx, rows)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if out[0].Status != StatusInvalid || !strings.Contains(out[0].Reason, "invalid_app_key") {
		t.Errorf("OTAA short key: %+v", out[0])
	}
	if out[1].Status != StatusInvalid || !strings.Contains(out[1].Reason, "missing_required:dev_addr") {
		t.Errorf("ABP no addr: %+v", out[1])
	}
	if out[2].Status != StatusInvalid || !strings.Contains(out[2].Reason, "invalid_nwk_s_key") {
		t.Errorf("ABP short nwk: %+v", out[2])
	}
}
