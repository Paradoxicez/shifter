package alert

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrRuleNotFound is returned by Get/UpdateRule/DisableRule/EnableRule when
// the rule id is unknown.
var ErrRuleNotFound = errors.New("alert: rule not found")

// RuleRecord mirrors the alert_rule schema (0038). Pointer types are used for
// columns that are NULL-able at the DB layer so callers can distinguish
// "not set" from a zero-valued comparison.
type RuleRecord struct {
	ID               uuid.UUID
	RuleKind         string
	ScopeKind        string
	ScopeID          *uuid.UUID
	HighBound        *float64
	LowBound         *float64
	Comparison       *string
	Unit             *string
	QuietWindowStart *time.Time
	QuietWindowEnd   *time.Time
	FlowThreshold    *float64
	DaysOfWeek       *int32
	Severity         string
	Name             *string
	Notes            *string
	CooldownSeconds  int32
	LastFiredAt      *time.Time
	DisabledAt       *time.Time
	CreatedBy        *uuid.UUID
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// CooledDown returns true when the cooldown window has elapsed since the last
// fire (or there has never been a fire). Workers MUST call this before
// raising a new alert for the rule.
func (r RuleRecord) CooledDown(now time.Time) bool {
	if r.LastFiredAt == nil {
		return true
	}
	return r.LastFiredAt.Add(time.Duration(r.CooldownSeconds) * time.Second).Before(now)
}

// CreateRuleParams is the input shape for RuleStore.Create. The optional
// fields use pointer types so callers can express "leave NULL" without an
// extra "set" boolean.
type CreateRuleParams struct {
	RuleKind         string
	ScopeKind        string
	ScopeID          *uuid.UUID
	HighBound        *float64
	LowBound         *float64
	Comparison       *string
	Unit             *string
	QuietWindowStart *time.Time
	QuietWindowEnd   *time.Time
	FlowThreshold    *float64
	DaysOfWeek       *int32
	Severity         string // empty → defaults to 'critical' via DB DEFAULT
	Name             *string
	Notes            *string
	CooldownSeconds  *int32 // nil → defaults to 900 via DB DEFAULT
	CreatedBy        *uuid.UUID
}

// RuleStore wraps the alert_rule CRUD surface. Uses pgxpool directly (no sqlc
// for now — RuleStore has just enough queries that hand-rolled SQL is
// readable and avoids churn in sqlc.yaml).
type RuleStore struct {
	pool *pgxpool.Pool
}

// NewRuleStore constructs a RuleStore bound to pool.
func NewRuleStore(pool *pgxpool.Pool) *RuleStore { return &RuleStore{pool: pool} }

const ruleColumnsSelect = `
    id, rule_kind, scope_kind, scope_id,
    high_bound, low_bound, comparison, unit,
    quiet_window_start, quiet_window_end, flow_threshold, days_of_week,
    severity, name, notes,
    cooldown_seconds, last_fired_at, disabled_at,
    created_by, created_at, updated_at`

// scanRule unmarshals a row from the canonical SELECT.
func scanRule(row pgx.Row) (RuleRecord, error) {
	var r RuleRecord
	err := row.Scan(
		&r.ID, &r.RuleKind, &r.ScopeKind, &r.ScopeID,
		&r.HighBound, &r.LowBound, &r.Comparison, &r.Unit,
		&r.QuietWindowStart, &r.QuietWindowEnd, &r.FlowThreshold, &r.DaysOfWeek,
		&r.Severity, &r.Name, &r.Notes,
		&r.CooldownSeconds, &r.LastFiredAt, &r.DisabledAt,
		&r.CreatedBy, &r.CreatedAt, &r.UpdatedAt,
	)
	return r, err
}

// CreateRule INSERTs an alert_rule row. Severity / cooldown_seconds carry
// their DB defaults when the caller passes the zero values ("" / nil).
func (s *RuleStore) CreateRule(ctx context.Context, tx pgx.Tx, p CreateRuleParams) (RuleRecord, error) {
	q := `
		INSERT INTO alert_rule
		    (rule_kind, scope_kind, scope_id,
		     high_bound, low_bound, comparison, unit,
		     quiet_window_start, quiet_window_end, flow_threshold, days_of_week,
		     severity, name, notes,
		     cooldown_seconds, created_by)
		VALUES (
		    $1, $2, $3,
		    $4, $5, $6, $7,
		    $8, $9, $10, $11,
		    COALESCE(NULLIF($12, ''), 'critical'),
		    $13, $14,
		    COALESCE($15::INTEGER, 900),
		    $16
		)
		RETURNING ` + ruleColumnsSelect

	row := tx.QueryRow(ctx, q,
		p.RuleKind, p.ScopeKind, p.ScopeID,
		p.HighBound, p.LowBound, p.Comparison, p.Unit,
		p.QuietWindowStart, p.QuietWindowEnd, p.FlowThreshold, p.DaysOfWeek,
		p.Severity, p.Name, p.Notes,
		p.CooldownSeconds, p.CreatedBy,
	)
	r, err := scanRule(row)
	if err != nil {
		return RuleRecord{}, fmt.Errorf("alert: insert rule: %w", err)
	}
	return r, nil
}

// GetRuleByID returns the rule matching id (or ErrRuleNotFound).
func (s *RuleStore) GetRuleByID(ctx context.Context, id uuid.UUID) (RuleRecord, error) {
	row := s.pool.QueryRow(ctx, `SELECT `+ruleColumnsSelect+` FROM alert_rule WHERE id = $1`, id)
	r, err := scanRule(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return RuleRecord{}, ErrRuleNotFound
	}
	if err != nil {
		return RuleRecord{}, fmt.Errorf("alert: get rule: %w", err)
	}
	return r, nil
}

// ListActiveRulesByKind returns all rules of kind that are NOT soft-deleted.
// Used by workers at the top of each evaluation cycle (one query per worker).
func (s *RuleStore) ListActiveRulesByKind(ctx context.Context, kind string) ([]RuleRecord, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+ruleColumnsSelect+` FROM alert_rule WHERE rule_kind = $1 AND disabled_at IS NULL ORDER BY created_at`,
		kind)
	if err != nil {
		return nil, fmt.Errorf("alert: list active rules by kind: %w", err)
	}
	defer rows.Close()
	out := []RuleRecord{}
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, fmt.Errorf("alert: scan rule: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListAllRules returns every rule. If includeDisabled is false, soft-deleted
// rules are filtered out. Used by the Settings rule library.
func (s *RuleStore) ListAllRules(ctx context.Context, includeDisabled bool) ([]RuleRecord, error) {
	q := `SELECT ` + ruleColumnsSelect + ` FROM alert_rule`
	if !includeDisabled {
		q += ` WHERE disabled_at IS NULL`
	}
	q += ` ORDER BY created_at DESC`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("alert: list all rules: %w", err)
	}
	defer rows.Close()
	out := []RuleRecord{}
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, fmt.Errorf("alert: scan rule: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// DisableRule soft-deletes the rule (sets disabled_at = now()).
// Returns ErrRuleNotFound when id doesn't exist.
func (s *RuleStore) DisableRule(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	tag, err := tx.Exec(ctx, `UPDATE alert_rule SET disabled_at = now() WHERE id = $1 AND disabled_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("alert: disable rule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Either id doesn't exist or already disabled — caller treats both
		// as "no-op" but we surface ErrRuleNotFound to keep the contract
		// symmetric with EnableRule.
		var exists bool
		if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM alert_rule WHERE id = $1)`, id).Scan(&exists); err != nil {
			return fmt.Errorf("alert: disable rule (post-check): %w", err)
		}
		if !exists {
			return ErrRuleNotFound
		}
	}
	return nil
}

// EnableRule clears disabled_at. Returns ErrRuleNotFound when id doesn't exist.
func (s *RuleStore) EnableRule(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	tag, err := tx.Exec(ctx, `UPDATE alert_rule SET disabled_at = NULL WHERE id = $1 AND disabled_at IS NOT NULL`, id)
	if err != nil {
		return fmt.Errorf("alert: enable rule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		var exists bool
		if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM alert_rule WHERE id = $1)`, id).Scan(&exists); err != nil {
			return fmt.Errorf("alert: enable rule (post-check): %w", err)
		}
		if !exists {
			return ErrRuleNotFound
		}
	}
	return nil
}

// TouchLastFiredAt sets last_fired_at = now() inside the caller's tx.
// Workers MUST call this when emitting a real (non-test) fire so cooldown
// gates the next eval cycle. D-19 test-fires do NOT call this.
func (s *RuleStore) TouchLastFiredAt(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	tag, err := tx.Exec(ctx, `UPDATE alert_rule SET last_fired_at = now() WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("alert: touch last_fired_at: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrRuleNotFound
	}
	return nil
}
