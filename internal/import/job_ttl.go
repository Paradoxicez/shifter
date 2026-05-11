package importpkg

// Import-job TTL (D-11). Preview jobs expire 1h after creation. We
// implement lazy expiry on read: when a handler reads an import_job that
// is still 'preview' but past expires_at, the row is transitioned to
// 'expired' before being returned.
//
// A future sweeper (Phase 9) can also batch-transition expired previews,
// but the lazy path is enough to keep the operator UX honest without
// requiring background work in Phase 3.

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// JobTTL is the configured preview-job lifetime. Exposed as a var so tests
// can shorten the deadline. Production callers use TTL1Hour.
const TTL1Hour = "1 hour"

// ExpireIfStale loads an import_job by its external job_id (UUID), and if
// it is still 'preview' but past expires_at, transitions it to 'expired'
// in a single UPDATE before returning. Returns the (possibly-updated) row.
//
// Returns pgx.ErrNoRows when the job_id does not exist (handler maps to
// HTTP 404).
func ExpireIfStale(ctx context.Context, q *sqlc.Queries, jobID uuid.UUID) (sqlc.ImportJob, error) {
	job, err := q.GetImportJobByJobID(ctx, pgtypeUUID(jobID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqlc.ImportJob{}, pgx.ErrNoRows
		}
		return sqlc.ImportJob{}, fmt.Errorf("get import_job: %w", err)
	}

	// Only preview jobs can transition.
	if job.Status != sqlc.ImportJobStatusPreview {
		return job, nil
	}
	if !job.ExpiresAt.Valid {
		return job, nil
	}

	// UpdateImportJobToExpired performs the conditional UPDATE (status =
	// preview AND expires_at < now). If the deadline has not passed it
	// returns pgx.ErrNoRows and we return the original row.
	updated, err := q.UpdateImportJobToExpired(ctx, job.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return job, nil
		}
		return sqlc.ImportJob{}, fmt.Errorf("expire import_job: %w", err)
	}
	return updated, nil
}
