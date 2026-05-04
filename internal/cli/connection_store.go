// Package cli — chirpstack_connection adapter.
//
// ConnectionStore satisfies BOTH chirpstack.ConnectionStore (read+write) and
// profile.ConnectionStore (read only). The composition root in serve.go
// constructs ONE *ConnectionStore and passes it to bootstrap.EnsureTenantAnd
// Application + profile.RunSeedSync + profile.HTTPDeps. Lives in the cli
// package because it depends on the sqlc-generated symbols and adapting them
// at call sites would force every consuming package to import sqlc.
//
// Plan 02-12 W5 fix: this file IS the missing wiring referenced as "future
// Plan 02-15 cmd/serve wiring" by Plans 02-05 + 02-06 + 02-08.
package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shifter-io/shifter/internal/chirpstack"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
	"github.com/shifter-io/shifter/internal/profile"
)

// Compile-time interface satisfaction assertions. A single *ConnectionStore
// satisfies BOTH the read+write chirpstack.ConnectionStore (used by
// EnsureTenantAndApplication) and the read-only profile.ConnectionStore
// (used by SaveProfile + RunSeedSync).
var (
	_ chirpstack.ConnectionStore = (*ConnectionStore)(nil)
	_ profile.ConnectionStore    = (*ConnectionStore)(nil)
)

// ConnectionStore is the singleton chirpstack_connection-row adapter.
type ConnectionStore struct {
	pool *pgxpool.Pool
}

// NewConnectionStore returns a ConnectionStore over the given pool.
func NewConnectionStore(pool *pgxpool.Pool) *ConnectionStore {
	return &ConnectionStore{pool: pool}
}

// GetCSConnection returns the persisted (cs_tenant_id, cs_application_id).
// Empty strings indicate "not yet bootstrapped" (the row is missing OR the
// columns are NULL — bootstrap.EnsureTenantAndApplication treats both the
// same way per its current behavior).
//
// pgx.ErrNoRows is mapped to ("", "", nil) so a pre-Phase-1-wizard install
// (no chirpstack_connection row yet) does NOT fail the boot pipeline. The
// install wizard itself populates the row; serve.go's degraded-CS branch
// proceeds without crashing.
func (s *ConnectionStore) GetCSConnection(ctx context.Context) (string, string, error) {
	q := sqlc.New(s.pool)
	row, err := q.GetChirpStackTenantApp(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil // pre-install — degrade silently
	}
	if err != nil {
		return "", "", fmt.Errorf("connection_store: get: %w", err)
	}
	var t, a string
	if row.CsTenantID != nil {
		t = *row.CsTenantID
	}
	if row.CsApplicationID != nil {
		a = *row.CsApplicationID
	}
	return t, a, nil
}

// SetCSTenantApp persists the bootstrapped (cs_tenant_id, cs_application_id).
// The columns are TEXT (per migration 0011) so values pass through verbatim;
// callers (chirpstack.EnsureTenantAndApplication) hand us the gRPC-returned
// UUID strings without parsing.
func (s *ConnectionStore) SetCSTenantApp(ctx context.Context, tenantID, applicationID string) error {
	if tenantID == "" || applicationID == "" {
		return fmt.Errorf("connection_store: refusing to persist empty (tenant=%q, application=%q)", tenantID, applicationID)
	}
	q := sqlc.New(s.pool)
	tID := tenantID
	aID := applicationID
	if err := q.SetChirpStackTenantApp(ctx, sqlc.SetChirpStackTenantAppParams{
		CsTenantID:      &tID,
		CsApplicationID: &aID,
	}); err != nil {
		return fmt.Errorf("connection_store: set: %w", err)
	}
	return nil
}
