package audit

// Phase 3 Wave 1 — bulk-import audit envelope contract.
// D-33 / D-34: a bulk import emits 1 envelope row
// (action='device.bulk_import', entity_type='import_job') plus N per-device
// rows (action='create' / 'update' depending on outcome). All N+1 rows
// share `request_id = import_job.job_id`, so a single SQL query against
// audit_log by request_id reconstructs the entire import operation.

import "testing"

// TestBulkImport_PerDeviceAndEnvelope — for a 5-device import, exactly 6
// audit rows are written: 1 envelope + 5 per-device. The envelope row's
// `after` JSONB contains a summary (total, created, already_exists,
// errored counts) but NO secret material.
func TestBulkImport_PerDeviceAndEnvelope(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/audit/bulk_import.go (03-VALIDATION row bulk_import_envelope_test.TestBulkImport_PerDeviceAndEnvelope)")
}

// TestBulkImport_RequestIDGrouping — all 6 audit rows from the same import
// share `request_id`, allowing a single SQL query
// (`WHERE request_id = $1`) to reconstruct the operation.
func TestBulkImport_RequestIDGrouping(t *testing.T) {
	t.Skip("Wave 1: awaiting internal/audit/bulk_import.go (03-VALIDATION row bulk_import_envelope_test.TestBulkImport_RequestIDGrouping)")
}
