package alert

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrDuplicateFire signals that a fire INSERT collided with the
// alert_firing_unique_idx partial unique. Callers MUST treat this as a
// successful no-op — the (rule, target) is already in 'firing' state from
// an earlier worker cycle.
var ErrDuplicateFire = errors.New("alert: already firing for (rule, target)")

// ErrAlertNotFound is returned when an alert id is unknown.
var ErrAlertNotFound = errors.New("alert: not found")

// AlertRecord mirrors the alert schema (0039).
type AlertRecord struct {
	ID               uuid.UUID
	RuleID           uuid.UUID
	RuleKind         string
	Severity         string
	State            string
	Payload          []byte // raw JSONB; caller unmarshals when needed
	TargetEntityType string
	TargetEntityID   uuid.UUID
	IsTest           bool
	FiredAt          time.Time
	ClearedAt        *time.Time
	AckedAt          *time.Time
	AckedBy          *uuid.UUID
	AckNote          *string
	SnoozedUntil     *time.Time
	SnoozedBy        *uuid.UUID
	Muted            bool
}

// InsertAlertParams is the input shape for AlertStore.InsertAlert.
type InsertAlertParams struct {
	RuleID           uuid.UUID
	RuleKind         string
	Severity         string
	Payload          []byte // canonical D-12 JSONB
	TargetEntityType string
	TargetEntityID   uuid.UUID
	IsTest           bool
}

// AlertStore wraps the alert lifecycle queries.
type AlertStore struct {
	pool *pgxpool.Pool
}

// NewAlertStore constructs an AlertStore.
func NewAlertStore(pool *pgxpool.Pool) *AlertStore { return &AlertStore{pool: pool} }

const alertColumnsSelect = `
    id, rule_id, rule_kind, severity, state, payload,
    target_entity_type, target_entity_id, is_test,
    fired_at, cleared_at, acked_at, acked_by, ack_note,
    snoozed_until, snoozed_by, muted`

func scanAlert(row pgx.Row) (AlertRecord, error) {
	var a AlertRecord
	err := row.Scan(
		&a.ID, &a.RuleID, &a.RuleKind, &a.Severity, &a.State, &a.Payload,
		&a.TargetEntityType, &a.TargetEntityID, &a.IsTest,
		&a.FiredAt, &a.ClearedAt, &a.AckedAt, &a.AckedBy, &a.AckNote,
		&a.SnoozedUntil, &a.SnoozedBy, &a.Muted,
	)
	return a, err
}

// InsertAlert inserts a new alert row in state='firing'. Returns
// ErrDuplicateFire when the alert_firing_unique_idx partial unique fires —
// the caller MUST treat this as a successful no-op (idempotent fire pattern).
func (s *AlertStore) InsertAlert(ctx context.Context, tx pgx.Tx, p InsertAlertParams) (AlertRecord, error) {
	q := `
		INSERT INTO alert
		    (rule_id, rule_kind, severity, payload,
		     target_entity_type, target_entity_id, is_test)
		VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7)
		RETURNING ` + alertColumnsSelect
	row := tx.QueryRow(ctx, q,
		p.RuleID, p.RuleKind, p.Severity, string(p.Payload),
		p.TargetEntityType, p.TargetEntityID, p.IsTest,
	)
	a, err := scanAlert(row)
	if err == nil {
		return a, nil
	}
	// Map the 23505 partial-unique violation to ErrDuplicateFire.
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "alert_firing_unique_idx") {
		return AlertRecord{}, ErrDuplicateFire
	}
	return AlertRecord{}, fmt.Errorf("alert: insert: %w", err)
}

// ClearAlert transitions state=firing/acknowledged → cleared and stamps
// cleared_at. Auto-clear from the engine and operator-clear go through here.
// Returns ErrAlertNotFound when id doesn't exist.
func (s *AlertStore) ClearAlert(ctx context.Context, tx pgx.Tx, id uuid.UUID) error {
	tag, err := tx.Exec(ctx,
		`UPDATE alert SET state = 'cleared', cleared_at = now()
		 WHERE id = $1 AND state IN ('firing', 'acknowledged', 'snoozed')`, id)
	if err != nil {
		return fmt.Errorf("alert: clear: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrAlertNotFound
	}
	return nil
}

// AckAlert transitions firing → acknowledged. note is optional.
func (s *AlertStore) AckAlert(ctx context.Context, tx pgx.Tx, id, userID uuid.UUID, note string) error {
	var notePtr *string
	if note != "" {
		notePtr = &note
	}
	tag, err := tx.Exec(ctx,
		`UPDATE alert SET state = 'acknowledged', acked_at = now(),
		                  acked_by = $2, ack_note = $3
		 WHERE id = $1 AND state = 'firing'`,
		id, userID, notePtr)
	if err != nil {
		return fmt.Errorf("alert: ack: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrAlertNotFound
	}
	return nil
}

// SnoozeAlert sets snoozed_until and snoozed_by. State moves to 'snoozed'.
func (s *AlertStore) SnoozeAlert(ctx context.Context, tx pgx.Tx, id, userID uuid.UUID, until time.Time) error {
	tag, err := tx.Exec(ctx,
		`UPDATE alert SET state = 'snoozed', snoozed_until = $2, snoozed_by = $3
		 WHERE id = $1 AND state IN ('firing','acknowledged')`,
		id, until, userID)
	if err != nil {
		return fmt.Errorf("alert: snooze: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrAlertNotFound
	}
	return nil
}

// MuteAlert flips muted=true (indefinite mute). Operator must explicitly
// unmute (handled in a future plan / handler).
func (s *AlertStore) MuteAlert(ctx context.Context, tx pgx.Tx, id, userID uuid.UUID) error {
	tag, err := tx.Exec(ctx,
		`UPDATE alert SET state = 'muted', muted = TRUE, snoozed_by = $2
		 WHERE id = $1 AND muted = FALSE`,
		id, userID)
	if err != nil {
		return fmt.Errorf("alert: mute: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrAlertNotFound
	}
	return nil
}

// ListFiringByRuleTarget returns the currently-firing alert for the (rule,
// target) pair, or nil if none. Used by workers to decide between an auto-
// clear (when the condition resolves) and a fresh fire (when state already
// went to cleared via a previous cycle).
func (s *AlertStore) ListFiringByRuleTarget(ctx context.Context, ruleID, targetEntityID uuid.UUID) (*AlertRecord, error) {
	row := s.pool.QueryRow(ctx,
		`SELECT `+alertColumnsSelect+` FROM alert
		 WHERE rule_id = $1 AND target_entity_id = $2 AND state = 'firing'
		 ORDER BY fired_at DESC
		 LIMIT 1`,
		ruleID, targetEntityID)
	a, err := scanAlert(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("alert: list firing by rule/target: %w", err)
	}
	return &a, nil
}

// ListRecent returns the most-recent alerts ordered by fired_at DESC,
// limited to limit rows. When states is non-empty, restricts to those
// states (drawer surface — typically ['firing','acknowledged']).
func (s *AlertStore) ListRecent(ctx context.Context, limit int, states []string) ([]AlertRecord, error) {
	if limit <= 0 {
		limit = 10
	}
	var rows pgx.Rows
	var err error
	if len(states) == 0 {
		rows, err = s.pool.Query(ctx,
			`SELECT `+alertColumnsSelect+` FROM alert ORDER BY fired_at DESC LIMIT $1`,
			limit)
	} else {
		rows, err = s.pool.Query(ctx,
			`SELECT `+alertColumnsSelect+` FROM alert WHERE state = ANY($1) ORDER BY fired_at DESC LIMIT $2`,
			states, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("alert: list recent: %w", err)
	}
	defer rows.Close()
	out := []AlertRecord{}
	for rows.Next() {
		a, err := scanAlert(rows)
		if err != nil {
			return nil, fmt.Errorf("alert: scan: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
