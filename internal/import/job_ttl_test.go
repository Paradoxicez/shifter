package importpkg

// Phase 3 Plan 03-05 — TTL lazy-expiry tests. D-11: preview jobs expire 1h
// after creation; ExpireIfStale transitions them on read.

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// TestImportJob_TTL1Hour — a preview job whose expires_at is in the future
// stays 'preview' after ExpireIfStale.
func TestImportJob_TTL1Hour(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()

	jobID := uuid.New()
	_, err := f.q.CreateImportJob(ctx, sqlc.CreateImportJobParams{
		JobID:      pgtypeUUID(jobID),
		OwnerID:    pgtypeUUID(f.adminID),
		FileName:   "ttl-fresh.xlsx",
		FileFormat: "xlsx",
		TotalRows:  0,
		ExpiresAt:  pgtype.Timestamptz{Time: time.Now().UTC().Add(time.Hour), Valid: true},
	})
	require.NoError(t, err)

	job, err := ExpireIfStale(ctx, f.q, jobID)
	if err != nil {
		t.Fatalf("ExpireIfStale: %v", err)
	}
	if job.Status != sqlc.ImportJobStatusPreview {
		t.Errorf("status = %s, want preview", job.Status)
	}
}

// TestImportJob_ExpiredOnRead — a preview job whose expires_at is in the
// past transitions to 'expired' on the next ExpireIfStale call. The
// returned row reflects the new status; the second call sees the already-
// expired row and is a no-op.
func TestImportJob_ExpiredOnRead(t *testing.T) {
	f := newImportFixture(t)
	ctx := context.Background()

	jobID := uuid.New()
	_, err := f.q.CreateImportJob(ctx, sqlc.CreateImportJobParams{
		JobID:      pgtypeUUID(jobID),
		OwnerID:    pgtypeUUID(f.adminID),
		FileName:   "ttl-stale.xlsx",
		FileFormat: "xlsx",
		TotalRows:  0,
		// expires_at = 65 minutes ago — past the 1h TTL.
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().UTC().Add(-65 * time.Minute), Valid: true},
	})
	require.NoError(t, err)

	job, err := ExpireIfStale(ctx, f.q, jobID)
	if err != nil {
		t.Fatalf("ExpireIfStale: %v", err)
	}
	if job.Status != sqlc.ImportJobStatusExpired {
		t.Errorf("status = %s, want expired", job.Status)
	}

	// Repeated call returns the already-expired row.
	job2, err := ExpireIfStale(ctx, f.q, jobID)
	if err != nil {
		t.Fatalf("ExpireIfStale repeat: %v", err)
	}
	if job2.Status != sqlc.ImportJobStatusExpired {
		t.Errorf("repeat status = %s, want expired", job2.Status)
	}
}
