package install

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// State is the in-memory snapshot of the singleton install_state row.
//
// Phase 1 schema: id INT PK CHECK (id = 1) — there is exactly one wizard run
// per install. Per-step JSONB drafts are validated client-side (zod) and
// re-validated by Plan 15's handlers before persistence. Secrets are stored
// by reference (path under /run/secrets) — never the raw value (D-04).
type State struct {
	StartedAt       time.Time
	CompletedAt     *time.Time
	CurrentStep     int
	Step1Admin      json.RawMessage
	Step2ChirpStack json.RawMessage
	Step3Region     json.RawMessage
	Step4Identity   json.RawMessage
}

// Store wraps install_state CRUD against a pgxpool. The store is stateless —
// it owns no caches; the FirstRunGate (middleware.go) is the only component
// that caches "admin exists" and it lives outside the Store.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore returns a Store backed by pool. The pool is reused (not closed by
// Store); callers manage its lifecycle.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// GetOrCreate returns the singleton install_state row, creating it on first
// call. Re-entrant: subsequent invocations return the same row because
// CHECK (id = 1) + ON CONFLICT (id) DO NOTHING short-circuits the second
// insert. This is the entry point Plan 15's GET handlers use to hydrate
// the wizard form on every step navigation (D-10, D-11).
func (s *Store) GetOrCreate(ctx context.Context) (*State, error) {
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO install_state (id) VALUES (1) ON CONFLICT (id) DO NOTHING`,
	); err != nil {
		return nil, fmt.Errorf("ensure install_state row: %w", err)
	}

	row := s.pool.QueryRow(ctx,
		`SELECT started_at, completed_at, current_step,
		        step1_admin, step2_chirpstack, step3_region, step4_identity
		   FROM install_state
		  WHERE id = 1`)

	var st State
	var s1, s2, s3, s4 *string
	if err := row.Scan(
		&st.StartedAt,
		&st.CompletedAt,
		&st.CurrentStep,
		&s1, &s2, &s3, &s4,
	); err != nil {
		return nil, fmt.Errorf("scan install_state: %w", err)
	}
	st.Step1Admin = jsonOrNil(s1)
	st.Step2ChirpStack = jsonOrNil(s2)
	st.Step3Region = jsonOrNil(s3)
	st.Step4Identity = jsonOrNil(s4)
	return &st, nil
}

func jsonOrNil(s *string) json.RawMessage {
	if s == nil {
		return nil
	}
	return json.RawMessage(*s)
}

// UpdateStep1 persists the admin draft (email/name/password_hash) and bumps
// current_step to >= 2. The payload is whatever the handler validated;
// Store does not interpret JSONB content.
func (s *Store) UpdateStep1(ctx context.Context, payload []byte) error {
	return s.updateStep(ctx, "step1_admin", 2, payload)
}

// UpdateStep2 persists the ChirpStack capture (mode + URLs + token ref) and
// bumps current_step to >= 3. Plan 15 calls this AFTER ProbeVersion confirms
// v4 (rejects v3 with destructive banner) so v3 drafts never reach the DB.
func (s *Store) UpdateStep2(ctx context.Context, payload []byte) error {
	return s.updateStep(ctx, "step2_chirpstack", 3, payload)
}

// UpdateStep3 persists the LoRaWAN region pick (name + common_name) and
// bumps current_step to >= 4. Region values come from Regions() — the
// handler whitelists against that catalog (T-14-04).
func (s *Store) UpdateStep3(ctx context.Context, payload []byte) error {
	return s.updateStep(ctx, "step3_region", 4, payload)
}

// UpdateStep4 persists the install identity (display_name, timezone, units)
// and bumps current_step to >= 5 (the "ready to finish" sentinel).
func (s *Store) UpdateStep4(ctx context.Context, payload []byte) error {
	return s.updateStep(ctx, "step4_identity", 5, payload)
}

// updateStep is the shared write path. column is whitelisted by call sites —
// it is NEVER user input (T-14-04, ASVS V5). nextStep is the target floor;
// GREATEST keeps current_step monotonic so a re-submission of an earlier
// step doesn't regress the wizard pointer.
func (s *Store) updateStep(ctx context.Context, column string, nextStep int, payload []byte) error {
	// Static interpolation of a hardcoded column name. The four call sites
	// above are the only producers; payload remains a $-bound parameter.
	q := fmt.Sprintf(
		`UPDATE install_state
		    SET %s = $1::jsonb,
		        current_step = GREATEST(current_step, $2)
		  WHERE id = 1`, column)
	if _, err := s.pool.Exec(ctx, q, payload, nextStep); err != nil {
		return fmt.Errorf("update install_state.%s: %w", column, err)
	}
	return nil
}

// Delete drops the singleton row. Plan 15's FinishSetup commits the
// drafts (admin user, chirpstack_connection, install_identity) in a single
// transaction and then calls Delete to enter the post-install state where
// the FirstRunGate is a pass-through.
func (s *Store) Delete(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM install_state WHERE id = 1`); err != nil {
		return fmt.Errorf("delete install_state: %w", err)
	}
	return nil
}
