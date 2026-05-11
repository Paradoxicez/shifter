package importpkg

// Phase 3 Wave 1 — import_job TTL between dry-run and commit.
// D-11: an import_job is valid for 1 hour from dry-run; commit against an
// expired job returns status="expired" and the operator must re-upload.

import "testing"

// TestImportJob_TTL1Hour — a job created < 1h ago is still valid for commit.
func TestImportJob_TTL1Hour(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/import/job.go TTL handling (03-VALIDATION row job_ttl_test.TestImportJob_TTL1Hour)")
}

// TestImportJob_ExpiredOnRead — fast-forward clock 65 minutes; reading the
// job returns expired=true and commit rejects with 409 status=expired.
func TestImportJob_ExpiredOnRead(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/import/job.go TTL handling (03-VALIDATION row job_ttl_test.TestImportJob_ExpiredOnRead)")
}
