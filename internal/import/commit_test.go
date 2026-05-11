package importpkg

// Phase 3 Plan 03-05 — commit-pass integration tests. Exercises the full
// path: dry-run → persist job + rows → commit → assert CS calls + PG
// devices + audit shape (envelope + per-device with request_id=job_id).

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// runDryRunAndPersist runs ParseXLSX → Validate → INSERT import_job +
// import_job_row rows. Returns the persisted job_id. Mirrors what the
// upload HTTP handler will do in Task 4.
func runDryRunAndPersist(t *testing.T, f *importFixture, xlsx []byte, fileName string) uuid.UUID {
	t.Helper()
	ctx := context.Background()

	rows, err := ParseXLSX(bytesReader(xlsx))
	require.NoError(t, err)

	outcomes, err := NewDryRunDeps(f.q).Validate(ctx, rows)
	require.NoError(t, err)

	// Aggregate counters.
	var valid, invalid, already int
	for _, o := range outcomes {
		switch o.Status {
		case StatusValid:
			valid++
		case StatusInvalid:
			invalid++
		case StatusAlreadyExists:
			already++
		}
	}

	jobID := uuid.New()
	expiresAt := pgtype.Timestamptz{Time: time.Now().UTC().Add(time.Hour), Valid: true}

	job, err := f.q.CreateImportJob(ctx, sqlc.CreateImportJobParams{
		JobID:      pgtypeUUID(jobID),
		OwnerID:    pgtypeUUID(f.adminID),
		FileName:   fileName,
		FileFormat: "xlsx",
		TotalRows:  int32(len(outcomes)),
		ExpiresAt:  expiresAt,
	})
	require.NoError(t, err)

	for _, o := range outcomes {
		rawRaw, _ := json.Marshal(rows[o.RowIndex-2].Raw)
		var parsedJSON []byte
		if o.Parsed != nil {
			parsedJSON, _ = json.Marshal(o.Parsed)
		}
		reason := o.Reason
		var reasonPtr *string
		if reason != "" {
			reasonPtr = &reason
		}
		_, err := f.q.InsertImportJobRow(ctx, sqlc.InsertImportJobRowParams{
			ImportJobID: job.ID,
			RowIndex:    int32(o.RowIndex),
			RawPayload:  rawRaw,
			Parsed:      parsedJSON,
			Status:      o.Status.ToSQLC(),
			Reason:      reasonPtr,
		})
		require.NoError(t, err)
	}

	_, err = f.q.UpdateImportJobCounters(ctx, sqlc.UpdateImportJobCountersParams{
		ID:                 job.ID,
		TotalRows:          int32(len(outcomes)),
		ValidCount:         int32(valid),
		InvalidCount:       int32(invalid),
		AlreadyExistsCount: int32(already),
	})
	require.NoError(t, err)

	return jobID
}

func newCommitDeps(f *importFixture) *CommitDeps {
	return &CommitDeps{
		Pool:      f.pool,
		CS:        f.cs,
		Bootstrap: f.boot,
		Log:       discardLogger(),
	}
}

// TestCommit_HappyPath — 5 valid OTAA rows commit cleanly: 5 CS Create, 5
// CS CreateKeys, 5 PG device rows, 1 envelope + 5 per-device audit rows.
func TestCommit_HappyPath(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()

	jobID := runDryRunAndPersist(t, f, testsupport.HappyFiveRows(), "happy.xlsx")
	deps := newCommitDeps(f)

	summary, err := deps.Commit(ctx, jobID, f.adminID)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if summary.Created != 5 {
		t.Errorf("summary.Created = %d, want 5", summary.Created)
	}
	if summary.Failed != 0 {
		t.Errorf("summary.Failed = %d, want 0", summary.Failed)
	}
	if f.cs.createCalls.Load() != 5 {
		t.Errorf("CS CreateDevice calls = %d, want 5", f.cs.createCalls.Load())
	}
	if f.cs.keysCalls.Load() != 5 {
		t.Errorf("CS CreateDeviceKeys calls = %d, want 5", f.cs.keysCalls.Load())
	}

	// Verify 5 device rows landed in PG.
	var deviceCount int
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT count(*) FROM device WHERE decommissioned_at IS NULL`,
	).Scan(&deviceCount))
	if deviceCount != 5 {
		t.Errorf("PG device rows = %d, want 5", deviceCount)
	}

	// Verify 6 audit rows (1 envelope + 5 per-device) share request_id.
	var auditCount int
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE request_id = $1`, jobID.String(),
	).Scan(&auditCount))
	if auditCount != 6 {
		t.Errorf("audit rows for job_id = %d, want 6", auditCount)
	}
}

// TestCommit_PartialCommit — mixed file: valid rows commit, already_exists +
// invalid are NOT attempted, counters track each bucket.
func TestCommit_PartialCommit(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	jobID := runDryRunAndPersist(t, f, testsupport.MixedTenRows(), "mixed.xlsx")
	deps := newCommitDeps(f)

	summary, err := deps.Commit(ctx, jobID, f.adminID)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// MixedTenRows has 5 valid + 5 invalid (per the fixture comments).
	if summary.Created != 5 {
		t.Errorf("Created = %d, want 5", summary.Created)
	}
	if summary.Invalid != 5 {
		t.Errorf("Invalid = %d, want 5", summary.Invalid)
	}
	// Only the 5 valid rows hit CS — invalid rows are NEVER attempted.
	if f.cs.createCalls.Load() != 5 {
		t.Errorf("CS CreateDevice calls = %d, want 5", f.cs.createCalls.Load())
	}
}

// TestCommit_Idempotent — committing the same job_id twice returns the
// stored summary on the second call, CS is NOT called a second time.
func TestCommit_Idempotent(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	jobID := runDryRunAndPersist(t, f, testsupport.HappyFiveRows(), "idempotent.xlsx")
	deps := newCommitDeps(f)

	_, err := deps.Commit(ctx, jobID, f.adminID)
	require.NoError(t, err)
	firstCSCalls := f.cs.createCalls.Load()

	// Second commit — short-circuits via summaryFromJob.
	summary2, err := deps.Commit(ctx, jobID, f.adminID)
	if err != nil {
		t.Fatalf("second Commit: %v", err)
	}
	if summary2.Created != 5 {
		t.Errorf("idempotent summary Created = %d, want 5", summary2.Created)
	}
	if f.cs.createCalls.Load() != firstCSCalls {
		t.Errorf("CS calls grew after second commit: %d → %d",
			firstCSCalls, f.cs.createCalls.Load())
	}

	// Verify exactly 5 device rows exist (no duplicates).
	var deviceCount int
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT count(*) FROM device WHERE decommissioned_at IS NULL`,
	).Scan(&deviceCount))
	if deviceCount != 5 {
		t.Errorf("PG device rows after re-commit = %d, want 5", deviceCount)
	}
}

// TestCommit_AuditEnvelopeRow — D-33/D-34: 1 envelope + N per-device rows
// share request_id = job_id. Envelope is action='device.bulk_import' on
// entity_type='import_job'; per-device is action='create' on
// entity_type='device' with notes='bulk_import'.
func TestCommit_AuditEnvelopeRow(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	jobID := runDryRunAndPersist(t, f, testsupport.HappyFiveRows(), "audit.xlsx")
	deps := newCommitDeps(f)

	if _, err := deps.Commit(ctx, jobID, f.adminID); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	type auditRow struct {
		action     string
		entityType string
		notes      *string
		requestID  *string
		afterJSON  []byte
	}
	rs, err := f.pool.Query(ctx,
		`SELECT action, entity_type, notes, request_id, after FROM audit_log
		 WHERE request_id = $1 ORDER BY action ASC`,
		jobID.String(),
	)
	require.NoError(t, err)
	defer rs.Close()
	var got []auditRow
	for rs.Next() {
		var r auditRow
		if err := rs.Scan(&r.action, &r.entityType, &r.notes, &r.requestID, &r.afterJSON); err != nil {
			t.Fatalf("scan audit row: %v", err)
		}
		got = append(got, r)
	}

	if len(got) != 6 {
		t.Fatalf("audit row count = %d, want 6", len(got))
	}

	envelope, devCount := 0, 0
	for _, r := range got {
		switch r.action {
		case "device.bulk_import":
			envelope++
			if r.entityType != "import_job" {
				t.Errorf("envelope entity_type = %s, want import_job", r.entityType)
			}
		case "create":
			devCount++
			if r.entityType != "device" {
				t.Errorf("per-device entity_type = %s, want device", r.entityType)
			}
			if r.notes == nil || *r.notes != "bulk_import" {
				t.Errorf("per-device notes = %v, want bulk_import", r.notes)
			}
		}
		if r.requestID == nil || *r.requestID != jobID.String() {
			t.Errorf("request_id = %v, want %s", r.requestID, jobID.String())
		}
	}
	if envelope != 1 {
		t.Errorf("envelope rows = %d, want 1", envelope)
	}
	if devCount != 5 {
		t.Errorf("per-device rows = %d, want 5", devCount)
	}
}

// TestCommit_PerRowCS_FailureSkipsRow — CS Create fails on the first row;
// remaining rows still commit. Failed row gets status='failed' + reason
// containing cs_create_failed.
func TestCommit_PerRowCS_FailureSkipsRow(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()
	jobID := runDryRunAndPersist(t, f, testsupport.HappyFiveRows(), "fail.xlsx")
	deps := newCommitDeps(f)

	// Inject a CS failure on the first CreateDevice call only.
	f.cs.createErrs = []error{errors.New("cs unavailable")}

	summary, err := deps.Commit(ctx, jobID, f.adminID)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if summary.Failed != 1 {
		t.Errorf("Failed = %d, want 1", summary.Failed)
	}
	if summary.Created != 4 {
		t.Errorf("Created = %d, want 4", summary.Created)
	}

	// Verify the failed import_job_row carries the reason.
	rows, err := f.pool.Query(ctx,
		`SELECT row_index, status, reason FROM import_job_row
		 WHERE status = 'failed' ORDER BY row_index`)
	require.NoError(t, err)
	defer rows.Close()
	failedCount := 0
	for rows.Next() {
		var idx int
		var status string
		var reason *string
		require.NoError(t, rows.Scan(&idx, &status, &reason))
		failedCount++
		if reason == nil || !strings.Contains(*reason, "cs_create_failed") {
			t.Errorf("row %d: reason = %v, want cs_create_failed", idx, reason)
		}
	}
	if failedCount != 1 {
		t.Errorf("failed rows = %d, want 1", failedCount)
	}
}
