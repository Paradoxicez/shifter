package audit_test

// Phase 3 Plan 03-05 — bulk-import audit envelope contract.
// D-33 / D-34: a bulk import emits 1 envelope row
// (action='device.bulk_import', entity_type='import_job') plus N per-device
// rows (action='create', entity_type='device', notes='bulk_import'). All N+1
// rows share `request_id = import_job.job_id` so a single SQL query
// reconstructs the import operation.
//
// We're in audit_test (not audit) because importing the importpkg would
// create a cycle (importpkg → audit; the test would close the loop). The
// tests stay vendor-neutral by calling audit.WriteEntry directly with the
// shape Plan 03-05's commit.go uses.

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/audit"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
)

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// TestBulkImport_PerDeviceAndEnvelope — emits 5 per-device rows + 1
// envelope row directly via audit.WriteEntry. Asserts the CHECK
// constraints accept the Phase 3 vocabulary, and that the envelope's
// `after` JSONB carries summary counts without secret material.
func TestBulkImport_PerDeviceAndEnvelope(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, discardLogger()))

	// Seed a user so audit_log.user_id FK is satisfiable.
	var userIDStr string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('admin-audit@example.com', 'Audit Admin', 'x', 'admin') RETURNING id::text`,
	).Scan(&userIDStr))
	actorID := uuid.MustParse(userIDStr)

	jobID := uuid.New()
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })

	// 5 per-device rows.
	for i := 0; i < 5; i++ {
		// We use a synthesised device entity_id since we're not seeding the
		// device table — the audit_log has no FK to device.id (D-22 logs are
		// independent of domain rows).
		devID := uuid.New()
		require.NoError(t, audit.WriteEntry(ctx, tx, audit.Entry{
			UserID:     actorID,
			Action:     audit.ActionCreate,
			EntityType: audit.EntityTypeDevice,
			EntityID:   devID,
			Before:     nil,
			After:      map[string]any{"dev_eui": "70b3d59999000000"},
			Notes:      "bulk_import",
			RequestID:  jobID.String(),
		}))
	}

	// 1 envelope row.
	envelopeAfter := map[string]any{
		"total":          5,
		"created":        5,
		"already_exists": 0,
		"failed":         0,
		"invalid":        0,
		"valid":          5,
		"job_id":         jobID.String(),
		"file_name":      "happy.xlsx",
		"file_format":    "xlsx",
	}
	require.NoError(t, audit.WriteEntry(ctx, tx, audit.Entry{
		UserID:     actorID,
		Action:     audit.ActionBulkImport,
		EntityType: audit.EntityTypeImportJob,
		EntityID:   jobID,
		Before:     nil,
		After:      envelopeAfter,
		Notes:      "",
		RequestID:  jobID.String(),
	}))

	require.NoError(t, tx.Commit(ctx))

	// Assert: 6 audit rows for this job_id.
	var count int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE request_id = $1`, jobID.String(),
	).Scan(&count))
	if count != 6 {
		t.Errorf("audit rows for job_id = %d, want 6", count)
	}

	// Envelope row's `after` JSONB carries summary, no secret material.
	var afterJSON []byte
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT after FROM audit_log WHERE action = 'device.bulk_import' AND request_id = $1`,
		jobID.String(),
	).Scan(&afterJSON))

	got := map[string]any{}
	require.NoError(t, json.Unmarshal(afterJSON, &got))
	if int(got["total"].(float64)) != 5 {
		t.Errorf("envelope after.total = %v, want 5", got["total"])
	}
	if got["job_id"].(string) != jobID.String() {
		t.Errorf("envelope after.job_id mismatch")
	}
	// Defensive: ensure no secret-looking key landed in `after`.
	for _, secret := range []string{"app_key", "nwk_s_key", "app_s_key", "password"} {
		if _, ok := got[secret]; ok {
			t.Errorf("envelope after.%s present — secret material in audit", secret)
		}
	}
}

// TestBulkImport_RequestIDGrouping — single WHERE request_id query yields
// ALL rows for one import (5 per-device + 1 envelope). The audit_log's
// partial index `audit_log_request_id_idx` covers this lookup.
func TestBulkImport_RequestIDGrouping(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, discardLogger()))

	var userIDStr string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('admin-grp@example.com', 'Group Admin', 'x', 'admin') RETURNING id::text`,
	).Scan(&userIDStr))
	actorID := uuid.MustParse(userIDStr)

	// Two separate "imports" — different job_id UUIDs. Each writes 3 rows
	// (2 per-device + 1 envelope). The WHERE request_id query must return
	// only the rows for the specific job, not all 6.
	jobA := uuid.New()
	jobB := uuid.New()

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })

	for _, j := range []uuid.UUID{jobA, jobB} {
		// 2 per-device + 1 envelope per job.
		for i := 0; i < 2; i++ {
			require.NoError(t, audit.WriteEntry(ctx, tx, audit.Entry{
				UserID:     actorID,
				Action:     audit.ActionCreate,
				EntityType: audit.EntityTypeDevice,
				EntityID:   uuid.New(),
				After:      map[string]any{"dev_eui": "70b3d59999ffff00"},
				Notes:      "bulk_import",
				RequestID:  j.String(),
			}))
		}
		require.NoError(t, audit.WriteEntry(ctx, tx, audit.Entry{
			UserID:     actorID,
			Action:     audit.ActionBulkImport,
			EntityType: audit.EntityTypeImportJob,
			EntityID:   j,
			After:      map[string]any{"job_id": j.String()},
			RequestID:  j.String(),
		}))
	}
	require.NoError(t, tx.Commit(ctx))

	for _, j := range []uuid.UUID{jobA, jobB} {
		var c int
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT count(*) FROM audit_log WHERE request_id = $1`, j.String(),
		).Scan(&c))
		if c != 3 {
			t.Errorf("job %s: %d rows, want 3", j, c)
		}
	}
}
