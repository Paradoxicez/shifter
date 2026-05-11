package importpkg

// Phase 3 Wave 1 — commit pass: takes the dry-run plan + job_id and applies
// the valid rows to CS + PG atomically per device. D-06 / D-07 / D-10 / D-11.

import "testing"

// TestCommit_HappyPath — 5 valid rows from HappyFiveRows() result in 5
// CS CreateDevice + 5 CS CreateDeviceKeys + 5 PG inserts; outcomes table
// shows 5 created.
func TestCommit_HappyPath(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/import/commit.go (03-VALIDATION row commit_test.TestCommit_HappyPath)")
}

// TestCommit_PartialCommit — MixedTenRows(): 5 valid commit successfully,
// 5 invalid are skipped with their dry-run reasons preserved (no PG
// transaction wrapping the whole file — per-row atomic).
func TestCommit_PartialCommit(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/import/commit.go (03-VALIDATION row commit_test.TestCommit_PartialCommit)")
}

// TestCommit_Idempotent — re-running commit with the same job_id is a no-op
// (returns the prior outcomes); D-11.
func TestCommit_Idempotent(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/import/commit.go (03-VALIDATION row commit_test.TestCommit_Idempotent)")
}

// TestCommit_AuditEnvelopeRow — per D-33/D-34: 1 envelope audit row
// (action='device.bulk_import') + N per-device rows all share
// `request_id = import_job.job_id`.
func TestCommit_AuditEnvelopeRow(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/import/commit.go (03-VALIDATION row commit_test.TestCommit_AuditEnvelopeRow)")
}
