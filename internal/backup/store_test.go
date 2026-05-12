package backup

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
	"github.com/stretchr/testify/require"
)

// TestMigration0046_CreatesBackupRunTable verifies that migration 0046 creates
// the backup_run table with all required columns and constraints.
func TestMigration0046_CreatesBackupRunTable(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test: -short")
	}
	pool := testsupport.StartPostgres(t)
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, slog.New(slog.NewTextHandler(io.Discard, nil))))

	// Table must exist.
	var exists bool
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_name='backup_run')`,
	).Scan(&exists))
	require.True(t, exists, "backup_run table must exist after migration 0046")

	// Required columns.
	for _, col := range []string{
		"id", "trigger_kind", "triggered_by", "status",
		"destination_dir", "file_name", "file_size_bytes", "sha256",
		"manifest_json", "chirpstack_mode", "schema_version",
		"started_at", "finished_at", "error_message",
	} {
		var colExists bool
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM information_schema.columns
			 WHERE table_name='backup_run' AND column_name=$1)`, col,
		).Scan(&colExists))
		require.True(t, colExists, "column backup_run.%s must exist", col)
	}

	// status CHECK constraint: valid values accepted.
	for _, status := range []string{"running", "completed", "failed"} {
		_, err := pool.Exec(ctx,
			`INSERT INTO backup_run (trigger_kind, status, destination_dir)
			 VALUES ('cli', $1, '/tmp') RETURNING id`, status)
		require.NoError(t, err, "status=%q should be accepted", status)
	}

	// status CHECK constraint: invalid value rejected.
	_, err := pool.Exec(ctx,
		`INSERT INTO backup_run (trigger_kind, status, destination_dir)
		 VALUES ('cli', 'bogus', '/tmp')`)
	require.Error(t, err, "invalid status should be rejected by CHECK")

	// trigger_kind CHECK constraint.
	for _, kind := range []string{"cli", "cron", "api"} {
		_, err := pool.Exec(ctx,
			`INSERT INTO backup_run (trigger_kind, status, destination_dir)
			 VALUES ($1, 'running', '/tmp')`, kind)
		require.NoError(t, err, "trigger_kind=%q should be accepted", kind)
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO backup_run (trigger_kind, status, destination_dir)
		 VALUES ('webhook', 'running', '/tmp')`)
	require.Error(t, err, "invalid trigger_kind should be rejected by CHECK")
}

// TestStore_InsertStartedAndUpdate verifies the full lifecycle:
// InsertStartedTx → UpdateCompletedTx → ListRecent → Last → Get.
func TestStore_InsertStartedAndUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test: -short")
	}
	pool := testsupport.StartPostgres(t)
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, slog.New(slog.NewTextHandler(io.Discard, nil))))

	store := NewStore(pool)
	csMode := "external"
	schemaVer := "46"

	// Insert started.
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	row, err := store.InsertStartedTx(ctx, tx, StartParams{
		TriggerKind:    "cli",
		DestinationDir: "/backups",
		ChirpStackMode: &csMode,
		SchemaVersion:  &schemaVer,
	})
	require.NoError(t, err)
	require.Equal(t, "running", row.Status)
	require.Equal(t, "cli", row.TriggerKind)
	require.NoError(t, tx.Commit(ctx))

	// Update completed.
	tx2, err := pool.Begin(ctx)
	require.NoError(t, err)
	fileName := "shifter-backup-test-20260101-0200-46.tar.gz"
	fileSize := int64(1024 * 1024)
	sha := "abc123def456"
	finished := time.Now().UTC()
	err = store.UpdateCompletedTx(ctx, tx2, CompleteParams{
		ID:            row.ID,
		FileName:      fileName,
		FileSizeBytes: fileSize,
		SHA256:        sha,
		FinishedAt:    finished,
	})
	require.NoError(t, err)
	require.NoError(t, tx2.Commit(ctx))

	// Get.
	got, err := store.Get(ctx, row.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "completed", got.Status)
	require.Equal(t, &fileName, got.FileName)
	require.Equal(t, &fileSize, got.FileSizeBytes)
	require.Equal(t, &sha, got.SHA256)

	// Last.
	last, err := store.Last(ctx)
	require.NoError(t, err)
	require.NotNil(t, last)
	require.Equal(t, row.ID, last.ID)

	// ListRecent.
	rows, err := store.ListRecent(ctx, 5)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, row.ID, rows[0].ID)
}

// TestStore_UpdateFailedTx verifies the failure update path.
func TestStore_UpdateFailedTx(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test: -short")
	}
	pool := testsupport.StartPostgres(t)
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, slog.New(slog.NewTextHandler(io.Discard, nil))))

	store := NewStore(pool)
	csMode := "external"
	schemaVer := "46"

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	row, err := store.InsertStartedTx(ctx, tx, StartParams{
		TriggerKind:    "api",
		DestinationDir: "/backups",
		ChirpStackMode: &csMode,
		SchemaVersion:  &schemaVer,
	})
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))

	tx2, err := pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, store.UpdateFailedTx(ctx, tx2, row.ID, "pg_dump exited non-zero"))
	require.NoError(t, tx2.Commit(ctx))

	got, err := store.Get(ctx, row.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, "failed", got.Status)
	require.NotNil(t, got.ErrorMessage)
	require.Equal(t, "pg_dump exited non-zero", *got.ErrorMessage)
}

// TestStore_Last_NoRows verifies that Last returns nil (not an error) when
// the backup_run table is empty.
func TestStore_Last_NoRows(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test: -short")
	}
	pool := testsupport.StartPostgres(t)
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, slog.New(slog.NewTextHandler(io.Discard, nil))))

	store := NewStore(pool)
	last, err := store.Last(ctx)
	require.NoError(t, err)
	require.Nil(t, last, "Last() must return nil when no backups exist")
}

// TestStore_Get_NotFound verifies that Get returns nil (not an error) for a
// non-existent UUID.
func TestStore_Get_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test: -short")
	}
	pool := testsupport.StartPostgres(t)
	ctx := context.Background()
	require.NoError(t, db.RunMigrations(ctx, pool, slog.New(slog.NewTextHandler(io.Discard, nil))))

	store := NewStore(pool)
	got, err := store.Get(ctx, uuid.New())
	require.NoError(t, err)
	require.Nil(t, got, "Get() must return nil for non-existent ID")
}
