package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BackupRunRow mirrors the backup_run table row returned to callers.  Pointer
// fields are nullable in the DB; Go nil ↔ SQL NULL.
type BackupRunRow struct {
	ID             uuid.UUID
	TriggerKind    string
	TriggeredBy    *uuid.UUID
	Status         string
	DestinationDir string
	FileName       *string
	FileSizeBytes  *int64
	SHA256         *string
	ManifestJSON   json.RawMessage
	ChirpStackMode *string
	SchemaVersion  *string
	StartedAt      time.Time
	FinishedAt     *time.Time
	ErrorMessage   *string
}

// StartParams holds the fields written when a backup starts (status='running').
type StartParams struct {
	TriggerKind    string
	TriggeredBy    *uuid.UUID // nil for CLI / cron
	DestinationDir string
	ChirpStackMode *string
	SchemaVersion  *string
}

// CompleteParams holds the fields written on successful completion.
type CompleteParams struct {
	ID             uuid.UUID
	FileName       string
	FileSizeBytes  int64
	SHA256         string
	ManifestJSON   json.RawMessage
	FinishedAt     time.Time
}

// Store owns all backup_run CRUD.  Methods that accept a pgx.Tx run inside
// that transaction; methods without a tx parameter use the pool directly.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore returns a Store backed by pool.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// InsertStartedTx inserts a backup_run row with status='running' inside tx.
// Returns the fully-populated row (id populated by gen_random_uuid(), etc.).
func (s *Store) InsertStartedTx(ctx context.Context, tx pgx.Tx, p StartParams) (BackupRunRow, error) {
	const q = `
		INSERT INTO backup_run
			(trigger_kind, triggered_by, status, destination_dir, chirpstack_mode, schema_version)
		VALUES ($1, $2, 'running', $3, $4, $5)
		RETURNING id, trigger_kind, triggered_by, status, destination_dir,
		          file_name, file_size_bytes, sha256, manifest_json,
		          chirpstack_mode, schema_version, started_at, finished_at, error_message`

	var row BackupRunRow
	var triggeredBy *[16]byte
	if p.TriggeredBy != nil {
		b := [16]byte(*p.TriggeredBy)
		triggeredBy = &b
	}
	dbRow := tx.QueryRow(ctx, q,
		p.TriggerKind,
		triggeredBy,
		p.DestinationDir,
		p.ChirpStackMode,
		p.SchemaVersion,
	)
	row, err := scanRow(dbRow)
	if err != nil {
		return BackupRunRow{}, fmt.Errorf("insert backup_run started: %w", err)
	}
	return row, nil
}

// UpdateCompletedTx updates the backup_run row to status='completed' inside tx.
func (s *Store) UpdateCompletedTx(ctx context.Context, tx pgx.Tx, p CompleteParams) error {
	const q = `
		UPDATE backup_run SET
			status          = 'completed',
			file_name       = $2,
			file_size_bytes = $3,
			sha256          = $4,
			manifest_json   = $5,
			finished_at     = $6
		WHERE id = $1`
	_, err := tx.Exec(ctx, q,
		p.ID,
		p.FileName,
		p.FileSizeBytes,
		p.SHA256,
		p.ManifestJSON,
		p.FinishedAt,
	)
	if err != nil {
		return fmt.Errorf("update backup_run completed: %w", err)
	}
	return nil
}

// UpdateFailedTx updates the backup_run row to status='failed' inside tx.
func (s *Store) UpdateFailedTx(ctx context.Context, tx pgx.Tx, id uuid.UUID, errMsg string) error {
	const q = `
		UPDATE backup_run SET
			status        = 'failed',
			finished_at   = now(),
			error_message = $2
		WHERE id = $1`
	_, err := tx.Exec(ctx, q, id, errMsg)
	if err != nil {
		return fmt.Errorf("update backup_run failed: %w", err)
	}
	return nil
}

// ListRecent returns up to limit rows ordered by started_at DESC.
func (s *Store) ListRecent(ctx context.Context, limit int) ([]BackupRunRow, error) {
	const q = `
		SELECT id, trigger_kind, triggered_by, status, destination_dir,
		       file_name, file_size_bytes, sha256, manifest_json,
		       chirpstack_mode, schema_version, started_at, finished_at, error_message
		FROM backup_run
		ORDER BY started_at DESC
		LIMIT $1`
	rows, err := s.pool.Query(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("list recent backup_runs: %w", err)
	}
	defer rows.Close()
	var out []BackupRunRow
	for rows.Next() {
		row, err := scanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// Last returns the most-recent backup_run row, or nil if no backups have run.
func (s *Store) Last(ctx context.Context) (*BackupRunRow, error) {
	const q = `
		SELECT id, trigger_kind, triggered_by, status, destination_dir,
		       file_name, file_size_bytes, sha256, manifest_json,
		       chirpstack_mode, schema_version, started_at, finished_at, error_message
		FROM backup_run
		ORDER BY started_at DESC
		LIMIT 1`
	r := s.pool.QueryRow(ctx, q)
	row, err := scanRow(r)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get last backup_run: %w", err)
	}
	return &row, nil
}

// Get returns the backup_run row for the given id.
func (s *Store) Get(ctx context.Context, id uuid.UUID) (*BackupRunRow, error) {
	const q = `
		SELECT id, trigger_kind, triggered_by, status, destination_dir,
		       file_name, file_size_bytes, sha256, manifest_json,
		       chirpstack_mode, schema_version, started_at, finished_at, error_message
		FROM backup_run
		WHERE id = $1`
	r := s.pool.QueryRow(ctx, q, id)
	row, err := scanRow(r)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get backup_run %s: %w", id, err)
	}
	return &row, nil
}

// scanner abstracts pgx.Row and pgx.Rows so scanRow works for both.
type scanner interface {
	Scan(dest ...any) error
}

func scanRow(r scanner) (BackupRunRow, error) {
	var row BackupRunRow
	var id [16]byte
	var triggeredBy *[16]byte
	var manifestJSON []byte
	err := r.Scan(
		&id,
		&row.TriggerKind,
		&triggeredBy,
		&row.Status,
		&row.DestinationDir,
		&row.FileName,
		&row.FileSizeBytes,
		&row.SHA256,
		&manifestJSON,
		&row.ChirpStackMode,
		&row.SchemaVersion,
		&row.StartedAt,
		&row.FinishedAt,
		&row.ErrorMessage,
	)
	if err != nil {
		return BackupRunRow{}, err
	}
	row.ID = uuid.UUID(id)
	if triggeredBy != nil {
		u := uuid.UUID(*triggeredBy)
		row.TriggeredBy = &u
	}
	if manifestJSON != nil {
		row.ManifestJSON = json.RawMessage(manifestJSON)
	}
	return row, nil
}
