package report

// template_store.go — Phase 7 Plan 11a: Saved Report Templates store.
//
// TemplateStore wraps the 5 sqlc report_template queries with audit-in-tx
// semantics. Every mutation (Save, Update, Delete) runs inside a pgx.Tx and
// calls audit.WriteEntry in the same transaction (D-23 atomicity requirement;
// T-07-11a-03 mitigation: audit row cannot exist without its domain row).
//
// The store is intentionally thin: no business logic beyond what is required
// for RBAC enforcement, duplicate detection (mapped to 23505 at handler layer),
// and not-found detection (mapped to pgx.ErrNoRows at handler layer).

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shifter-io/shifter/internal/audit"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// TemplateRow is the domain-level representation of a saved report template.
// It mirrors the sqlc.ReportTemplate struct but uses Go-native types so callers
// don't need to import pgtype for reads.
type TemplateRow struct {
	ID          uuid.UUID
	Name        string
	Description string
	State       json.RawMessage
}

// TemplateStore provides CRUD access to the report_template table.
// It must be constructed with NewTemplateStore.
type TemplateStore struct {
	pool *pgxpool.Pool
}

// NewTemplateStore constructs a TemplateStore backed by the given pool.
func NewTemplateStore(pool *pgxpool.Pool) *TemplateStore {
	return &TemplateStore{pool: pool}
}

// Save creates a new report template and writes a report_template.created audit
// row in the same transaction (D-23). Returns the created row.
//
// Callers must map pgconn error 23505 (unique_violation on name) → HTTP 409.
func (s *TemplateStore) Save(ctx context.Context, name, description string, state json.RawMessage, actorID uuid.UUID) (TemplateRow, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return TemplateRow{}, fmt.Errorf("report template save: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := sqlc.New(tx)
	row, err := qtx.CreateReportTemplate(ctx, sqlc.CreateReportTemplateParams{
		Name:        name,
		Description: description,
		State:       []byte(state),
		CreatedBy:   pgtype.UUID{Bytes: actorID, Valid: actorID != uuid.Nil},
	})
	if err != nil {
		return TemplateRow{}, fmt.Errorf("report template save: insert: %w", err)
	}

	tplID := uuid.UUID(row.ID.Bytes)
	if err := audit.WriteEntry(ctx, tx, audit.Entry{
		UserID:     actorID,
		Action:     audit.AuditActionReportTemplateCreated,
		EntityType: audit.EntityTypeReportTemplate,
		EntityID:   tplID,
		After:      map[string]any{"name": name, "description": description},
	}); err != nil {
		return TemplateRow{}, fmt.Errorf("report template save: audit: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return TemplateRow{}, fmt.Errorf("report template save: commit: %w", err)
	}
	return toTemplateRow(row), nil
}

// List returns all report templates ordered case-insensitively by name ascending.
// No audit row is written (read-only operation).
func (s *TemplateStore) List(ctx context.Context) ([]TemplateRow, error) {
	q := sqlc.New(s.pool)
	rows, err := q.ListReportTemplates(ctx)
	if err != nil {
		return nil, fmt.Errorf("report template list: %w", err)
	}
	result := make([]TemplateRow, len(rows))
	for i, r := range rows {
		result[i] = toTemplateRow(r)
	}
	return result, nil
}

// Get returns a single report template by UUID.
// Returns pgx.ErrNoRows (wrapped) when the template does not exist.
func (s *TemplateStore) Get(ctx context.Context, id uuid.UUID) (TemplateRow, error) {
	q := sqlc.New(s.pool)
	row, err := q.GetReportTemplate(ctx, pgtype.UUID{Bytes: id, Valid: true})
	if err != nil {
		return TemplateRow{}, fmt.Errorf("report template get: %w", err)
	}
	return toTemplateRow(row), nil
}

// Update renames/re-describes/re-states an existing report template and writes a
// report_template.updated audit row in the same transaction (D-23).
//
// Callers must map pgconn error 23505 (unique_violation on name rename) → HTTP 409.
// Callers must map pgx.ErrNoRows → HTTP 404 (template was deleted between list and update).
func (s *TemplateStore) Update(ctx context.Context, id uuid.UUID, name, description string, state json.RawMessage, actorID uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("report template update: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := sqlc.New(tx)
	// Verify the row exists first so we can return ErrNoRows on a missing template.
	_, err = qtx.GetReportTemplate(ctx, pgtype.UUID{Bytes: id, Valid: true})
	if err != nil {
		return fmt.Errorf("report template update: get: %w", err)
	}

	if err := qtx.UpdateReportTemplate(ctx, sqlc.UpdateReportTemplateParams{
		ID:          pgtype.UUID{Bytes: id, Valid: true},
		Name:        name,
		Description: description,
		State:       []byte(state),
	}); err != nil {
		return fmt.Errorf("report template update: exec: %w", err)
	}

	if err := audit.WriteEntry(ctx, tx, audit.Entry{
		UserID:     actorID,
		Action:     audit.AuditActionReportTemplateUpdated,
		EntityType: audit.EntityTypeReportTemplate,
		EntityID:   id,
		After:      map[string]any{"name": name, "description": description},
	}); err != nil {
		return fmt.Errorf("report template update: audit: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("report template update: commit: %w", err)
	}
	return nil
}

// Delete hard-deletes a report template and writes a report_template.deleted audit
// row in the same transaction (D-23; T-07-11a-03 mitigation).
//
// Callers must map pgx.ErrNoRows → HTTP 404 when the template was already deleted.
func (s *TemplateStore) Delete(ctx context.Context, id uuid.UUID, actorID uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("report template delete: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qtx := sqlc.New(tx)
	// Verify the row exists before attempting delete so we can return ErrNoRows.
	existing, err := qtx.GetReportTemplate(ctx, pgtype.UUID{Bytes: id, Valid: true})
	if err != nil {
		return fmt.Errorf("report template delete: get: %w", err)
	}

	if err := qtx.DeleteReportTemplate(ctx, pgtype.UUID{Bytes: id, Valid: true}); err != nil {
		return fmt.Errorf("report template delete: exec: %w", err)
	}

	if err := audit.WriteEntry(ctx, tx, audit.Entry{
		UserID:     actorID,
		Action:     audit.AuditActionReportTemplateDeleted,
		EntityType: audit.EntityTypeReportTemplate,
		EntityID:   id,
		Before:     map[string]any{"name": existing.Name},
	}); err != nil {
		return fmt.Errorf("report template delete: audit: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("report template delete: commit: %w", err)
	}
	return nil
}

// toTemplateRow converts a sqlc.ReportTemplate (pgtype fields) to a TemplateRow
// (Go-native types). State bytes are returned as json.RawMessage.
func toTemplateRow(r sqlc.ReportTemplate) TemplateRow {
	return TemplateRow{
		ID:          uuid.UUID(r.ID.Bytes),
		Name:        r.Name,
		Description: r.Description,
		State:       json.RawMessage(r.State),
	}
}

// unwrapErrNoRows returns pgx.ErrNoRows if the error chain contains it, or the
// original error otherwise. Used by handlers to map 404 vs 500.
func unwrapErrNoRows(err error) bool {
	return err != nil && (err == pgx.ErrNoRows ||
		fmt.Sprintf("%s", err) == fmt.Sprintf("report template get: %s", pgx.ErrNoRows) ||
		isErrNoRows(err))
}

// isErrNoRows checks if the error wraps pgx.ErrNoRows anywhere in the chain.
func isErrNoRows(err error) bool {
	if err == nil {
		return false
	}
	if err == pgx.ErrNoRows {
		return true
	}
	type unwrapper interface{ Unwrap() error }
	if u, ok := err.(unwrapper); ok {
		return isErrNoRows(u.Unwrap())
	}
	return false
}
