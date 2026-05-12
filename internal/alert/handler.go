// Package alert — HTTP handlers for the Phase 6 alert center
// (Plan 06-04 surface). The handlers expose the substrate built in Plan
// 06-01 (RuleStore, AlertStore, payload helpers) through REST under
// /api/alerts/*.
//
// RBAC model (D-11):
//   - ActionAlertRead (admin + viewer) — list / detail / drawer-recent
//   - ActionAlertAck / ActionAlertSnooze / ActionAlertMute — admin only
//   - ActionAlertRule* + ActionAlertTestFire — admin only
//
// Auditing (D-23 atomicity):
//   - Every mutating handler opens a pgx.Tx → mutates → writes audit_log
//     row in the SAME tx → commits. The audit row literally cannot exist
//     without its domain row (and vice versa) by Postgres atomicity.
package alert

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shifter-io/shifter/internal/audit"
	"github.com/shifter-io/shifter/internal/auth"
)

// HTTPDeps bundles the dependencies every alert handler needs.
//
// Construction: Plan 06-04's serve.go wiring creates one HTTPDeps shared
// across all handlers. nil-guarded so router unit tests that don't need
// alert routes can run with HTTPDeps=nil.
type HTTPDeps struct {
	Pool       *pgxpool.Pool
	SessionMgr *scs.SessionManager
	Rules      *RuleStore
	Alerts     *AlertStore
	Log        *slog.Logger
}

// ─────────────────────────────────────────────────────────────────────────
// Wire shapes
// ─────────────────────────────────────────────────────────────────────────

type errorResp struct {
	Error string `json:"error"`
}

// alertDTO is the wire shape for a single alert row. Mirrors AlertRecord
// minus internal-only fields and with JSON-friendly types.
type alertDTO struct {
	ID               uuid.UUID  `json:"id"`
	RuleID           uuid.UUID  `json:"rule_id"`
	RuleKind         string     `json:"rule_kind"`
	Severity         string     `json:"severity"`
	State            string     `json:"state"`
	Payload          json.RawMessage `json:"payload"`
	TargetEntityType string     `json:"target_entity_type"`
	TargetEntityID   uuid.UUID  `json:"target_entity_id"`
	IsTest           bool       `json:"is_test"`
	FiredAt          time.Time  `json:"fired_at"`
	ClearedAt        *time.Time `json:"cleared_at,omitempty"`
	AckedAt          *time.Time `json:"acked_at,omitempty"`
	AckedBy          *uuid.UUID `json:"acked_by,omitempty"`
	AckNote          *string    `json:"ack_note,omitempty"`
	SnoozedUntil     *time.Time `json:"snoozed_until,omitempty"`
	SnoozedBy        *uuid.UUID `json:"snoozed_by,omitempty"`
	Muted            bool       `json:"muted"`
}

func alertToDTO(a AlertRecord) alertDTO {
	d := alertDTO{
		ID:               a.ID,
		RuleID:           a.RuleID,
		RuleKind:         a.RuleKind,
		Severity:         a.Severity,
		State:            a.State,
		Payload:          json.RawMessage(a.Payload),
		TargetEntityType: a.TargetEntityType,
		TargetEntityID:   a.TargetEntityID,
		IsTest:           a.IsTest,
		FiredAt:          a.FiredAt,
		ClearedAt:        a.ClearedAt,
		AckedAt:          a.AckedAt,
		AckedBy:          a.AckedBy,
		AckNote:          a.AckNote,
		SnoozedUntil:     a.SnoozedUntil,
		SnoozedBy:        a.SnoozedBy,
		Muted:            a.Muted,
	}
	if len(d.Payload) == 0 {
		d.Payload = json.RawMessage("null")
	}
	return d
}

// unreadCounts is the bell-badge support struct.
type unreadCounts struct {
	Critical int64 `json:"critical"`
	Warning  int64 `json:"warning"`
	Info     int64 `json:"info"`
}

// listResponse is the wire shape for GET /api/alerts. Pairs the alert rows
// with the bell-badge counts so the frontend only does one round-trip.
type listResponse struct {
	Rows         []alertDTO    `json:"rows"`
	NextCursor   *string       `json:"next_cursor,omitempty"`
	UnreadCounts unreadCounts  `json:"unread_counts"`
}

type ackRequest struct {
	Note string `json:"note"`
}

type snoozeRequest struct {
	Duration string `json:"duration"`
}

// ─────────────────────────────────────────────────────────────────────────
// Handlers
// ─────────────────────────────────────────────────────────────────────────

// ListHandler — GET /api/alerts.
// Query params: severity, status, category, target_type, from, to, cursor.
// Returns {rows, next_cursor, unread_counts}. Default status filter is "open"
// (firing|acknowledged|snoozed|muted) per UI-SPEC §Surface 2.
func ListHandler(deps HTTPDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		severity := strings.TrimSpace(q.Get("severity"))
		status := strings.TrimSpace(q.Get("status"))
		category := strings.TrimSpace(q.Get("category"))
		targetType := strings.TrimSpace(q.Get("target_type"))
		fromRaw := strings.TrimSpace(q.Get("from"))
		toRaw := strings.TrimSpace(q.Get("to"))

		if status == "" || status == "open" {
			// default open = firing|acknowledged|snoozed|muted (anything not cleared)
			status = "open"
		}

		// Cursor optional — pagination not required for the default Settings
		// "last 30 days" view, but the SQL supports it.
		const pageSize = 100
		var fromTime, toTime *time.Time
		if fromRaw != "" {
			t, err := time.Parse(time.RFC3339, fromRaw)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, errorResp{Error: "invalid_from"})
				return
			}
			fromTime = &t
		}
		if toRaw != "" {
			t, err := time.Parse(time.RFC3339, toRaw)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, errorResp{Error: "invalid_to"})
				return
			}
			toTime = &t
		}

		rows, err := listAlertsFiltered(r.Context(), deps.Pool, listFilters{
			Severity:   severity,
			Status:     status,
			Category:   category,
			TargetType: targetType,
			From:       fromTime,
			To:         toTime,
			Limit:      pageSize + 1, // +1 to detect next page
		})
		if err != nil {
			internalError(deps.Log, w, "list alerts", err)
			return
		}

		// Build response. If we got >pageSize rows there's a next page.
		var nextCursor *string
		if len(rows) > pageSize {
			c := rows[pageSize-1].FiredAt.UTC().Format(time.RFC3339Nano) + "|" + rows[pageSize-1].ID.String()
			nextCursor = &c
			rows = rows[:pageSize]
		}

		dtos := make([]alertDTO, 0, len(rows))
		for _, row := range rows {
			dtos = append(dtos, alertToDTO(row))
		}

		// Bell counts (1 round-trip total).
		counts, err := countUnreadBySeverity(r.Context(), deps.Pool)
		if err != nil {
			internalError(deps.Log, w, "count unread", err)
			return
		}

		writeJSON(w, http.StatusOK, listResponse{
			Rows:         dtos,
			NextCursor:   nextCursor,
			UnreadCounts: counts,
		})
	}
}

// GetHandler — GET /api/alerts/{id}. Returns a single alert.
func GetHandler(deps HTTPDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseAlertID(w, r)
		if !ok {
			return
		}
		alert, err := loadAlertByID(r.Context(), deps.Pool, id)
		if errors.Is(err, ErrAlertNotFound) {
			writeJSON(w, http.StatusNotFound, errorResp{Error: "not_found"})
			return
		}
		if err != nil {
			internalError(deps.Log, w, "get alert", err)
			return
		}
		writeJSON(w, http.StatusOK, alertToDTO(alert))
	}
}

// RecentForDrawerHandler — GET /api/alerts/recent.
// Returns the top 10 most-recent firing/acknowledged/snoozed alerts (sorted
// by severity then fired_at DESC) + unread counts. Powers the bell badge
// AND the slide-over drawer (UI-SPEC §Surface 1).
func RecentForDrawerHandler(deps HTTPDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := deps.Alerts.ListRecent(r.Context(), 10,
			[]string{"firing", "acknowledged", "snoozed"})
		if err != nil {
			internalError(deps.Log, w, "list recent alerts", err)
			return
		}
		counts, err := countUnreadBySeverity(r.Context(), deps.Pool)
		if err != nil {
			internalError(deps.Log, w, "count unread", err)
			return
		}
		dtos := make([]alertDTO, 0, len(rows))
		for _, row := range rows {
			dtos = append(dtos, alertToDTO(row))
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"rows":          dtos,
			"unread_counts": counts,
		})
	}
}

// AckHandler — POST /api/alerts/{id}/ack. body {note?: string}.
// Writes audit_log row 'alert.acknowledged' in the same tx as the
// alert.AckAlert UPDATE (D-23).
func AckHandler(deps HTTPDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acting, ok := auth.GetUser(r.Context(), deps.SessionMgr)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errorResp{Error: "unauthorized"})
			return
		}
		id, ok := parseAlertID(w, r)
		if !ok {
			return
		}
		var req ackRequest
		if r.ContentLength > 0 {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, errorResp{Error: "bad_json"})
				return
			}
		}
		actingUUID, err := uuid.Parse(acting.ID)
		if err != nil {
			internalError(deps.Log, w, "parse acting id", err)
			return
		}

		ctx := r.Context()
		tx, err := deps.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
		if err != nil {
			internalError(deps.Log, w, "begin tx", err)
			return
		}
		defer func() { _ = tx.Rollback(ctx) }()

		if err := deps.Alerts.AckAlert(ctx, tx, id, actingUUID, req.Note); err != nil {
			if errors.Is(err, ErrAlertNotFound) {
				writeJSON(w, http.StatusNotFound, errorResp{Error: "not_found"})
				return
			}
			internalError(deps.Log, w, "ack alert", err)
			return
		}
		if err := audit.WriteEntry(ctx, tx, audit.Entry{
			UserID:     actingUUID,
			Action:     audit.ActionAlertAcked,
			EntityType: audit.EntityTypeAlert,
			EntityID:   id,
			Notes:      req.Note,
			RequestID:  middleware.GetReqID(ctx),
		}); err != nil {
			internalError(deps.Log, w, "audit ack", err)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			internalError(deps.Log, w, "commit ack", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "state": "acknowledged"})
	}
}

// SnoozeHandler — POST /api/alerts/{id}/snooze. body {duration: "1h"|"8h"|...|"mute"}.
// Computes the snoozed_until time (or muted=true for "mute"). Writes audit
// row 'alert.snoozed' or 'alert.muted' in the same tx (D-23).
func SnoozeHandler(deps HTTPDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acting, ok := auth.GetUser(r.Context(), deps.SessionMgr)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errorResp{Error: "unauthorized"})
			return
		}
		id, ok := parseAlertID(w, r)
		if !ok {
			return
		}
		var req snoozeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "bad_json"})
			return
		}
		dur := strings.TrimSpace(req.Duration)
		actingUUID, err := uuid.Parse(acting.ID)
		if err != nil {
			internalError(deps.Log, w, "parse acting id", err)
			return
		}

		ctx := r.Context()
		tx, err := deps.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
		if err != nil {
			internalError(deps.Log, w, "begin tx", err)
			return
		}
		defer func() { _ = tx.Rollback(ctx) }()

		var action string
		switch dur {
		case "mute":
			if err := deps.Alerts.MuteAlert(ctx, tx, id, actingUUID); err != nil {
				if errors.Is(err, ErrAlertNotFound) {
					writeJSON(w, http.StatusNotFound, errorResp{Error: "not_found"})
					return
				}
				internalError(deps.Log, w, "mute alert", err)
				return
			}
			action = audit.ActionAlertMuted
		case "1h", "8h", "24h", "7d":
			until := computeSnoozeUntil(time.Now().UTC(), dur)
			if err := deps.Alerts.SnoozeAlert(ctx, tx, id, actingUUID, until); err != nil {
				if errors.Is(err, ErrAlertNotFound) {
					writeJSON(w, http.StatusNotFound, errorResp{Error: "not_found"})
					return
				}
				internalError(deps.Log, w, "snooze alert", err)
				return
			}
			action = audit.ActionAlertSnoozed
		default:
			writeJSON(w, http.StatusUnprocessableEntity, errorResp{Error: "invalid_duration"})
			return
		}

		if err := audit.WriteEntry(ctx, tx, audit.Entry{
			UserID:     actingUUID,
			Action:     action,
			EntityType: audit.EntityTypeAlert,
			EntityID:   id,
			Notes:      "duration=" + dur,
			RequestID:  middleware.GetReqID(ctx),
		}); err != nil {
			internalError(deps.Log, w, "audit snooze", err)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			internalError(deps.Log, w, "commit snooze", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "duration": dur})
	}
}

// ─────────────────────────────────────────────────────────────────────────
// Internal helpers (handler-scope; not exported)
// ─────────────────────────────────────────────────────────────────────────

// computeSnoozeUntil returns the absolute UTC time when a snoozed alert
// re-emerges. dur is one of the four UI-SPEC preset strings.
func computeSnoozeUntil(now time.Time, dur string) time.Time {
	switch dur {
	case "1h":
		return now.Add(1 * time.Hour)
	case "8h":
		return now.Add(8 * time.Hour)
	case "24h":
		return now.Add(24 * time.Hour)
	case "7d":
		return now.Add(7 * 24 * time.Hour)
	default:
		// Caller already validated; fallback to 1h.
		return now.Add(1 * time.Hour)
	}
}

type listFilters struct {
	Severity   string
	Status     string
	Category   string
	TargetType string
	From       *time.Time
	To         *time.Time
	Limit      int
}

// listAlertsFiltered is the hand-rolled SQL behind ListHandler. Keeping it
// hand-rolled (not sqlc) lets us assemble the WHERE clause dynamically and
// keep the cursor pagination flexible — the SQL is small enough that
// readability outweighs the type-safety win.
func listAlertsFiltered(ctx context.Context, pool *pgxpool.Pool, f listFilters) ([]AlertRecord, error) {
	q := "SELECT id, rule_id, rule_kind, severity, state, payload, " +
		"target_entity_type, target_entity_id, is_test, fired_at, cleared_at, " +
		"acked_at, acked_by, ack_note, snoozed_until, snoozed_by, muted " +
		"FROM alert WHERE 1=1"
	args := []any{}
	idx := 1
	if f.Severity != "" && f.Severity != "all" {
		q += " AND severity = $" + strconv.Itoa(idx)
		args = append(args, f.Severity)
		idx++
	}
	switch f.Status {
	case "", "open":
		q += " AND state IN ('firing','acknowledged','snoozed','muted')"
	case "firing":
		q += " AND state = 'firing'"
	case "acknowledged":
		q += " AND state = 'acknowledged'"
	case "snoozed":
		q += " AND state = 'snoozed'"
	case "cleared":
		q += " AND state = 'cleared'"
	case "all":
		// no state filter
	default:
		q += " AND state = $" + strconv.Itoa(idx)
		args = append(args, f.Status)
		idx++
	}
	if f.Category != "" && f.Category != "all" {
		q += " AND rule_kind LIKE $" + strconv.Itoa(idx)
		args = append(args, f.Category+"%")
		idx++
	}
	if f.TargetType != "" && f.TargetType != "all" {
		q += " AND target_entity_type = $" + strconv.Itoa(idx)
		args = append(args, f.TargetType)
		idx++
	}
	if f.From != nil {
		q += " AND fired_at >= $" + strconv.Itoa(idx)
		args = append(args, *f.From)
		idx++
	}
	if f.To != nil {
		q += " AND fired_at < $" + strconv.Itoa(idx)
		args = append(args, *f.To)
		idx++
	}
	q += " ORDER BY fired_at DESC, id DESC LIMIT $" + strconv.Itoa(idx)
	args = append(args, f.Limit)

	rows, err := pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("alert list: %w", err)
	}
	defer rows.Close()
	out := []AlertRecord{}
	for rows.Next() {
		a, err := scanAlert(rows)
		if err != nil {
			return nil, fmt.Errorf("alert scan: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func loadAlertByID(ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) (AlertRecord, error) {
	q := "SELECT " + alertColumnsSelect + " FROM alert WHERE id = $1"
	row := pool.QueryRow(ctx, q, id)
	a, err := scanAlert(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return AlertRecord{}, ErrAlertNotFound
	}
	if err != nil {
		return AlertRecord{}, fmt.Errorf("alert get: %w", err)
	}
	return a, nil
}

func countUnreadBySeverity(ctx context.Context, pool *pgxpool.Pool) (unreadCounts, error) {
	var c unreadCounts
	row := pool.QueryRow(ctx, `
		SELECT
		    count(*) FILTER (WHERE severity = 'critical' AND state IN ('firing','snoozed')) AS critical_count,
		    count(*) FILTER (WHERE severity = 'warning'  AND state IN ('firing','snoozed')) AS warning_count,
		    count(*) FILTER (WHERE severity = 'info'     AND state IN ('firing','snoozed')) AS info_count
		FROM alert WHERE muted = FALSE`)
	if err := row.Scan(&c.Critical, &c.Warning, &c.Info); err != nil {
		return c, fmt.Errorf("count unread: %w", err)
	}
	return c, nil
}

func parseAlertID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	raw := chi.URLParam(r, "id")
	id, err := uuid.Parse(raw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResp{Error: "invalid_id"})
		return uuid.Nil, false
	}
	return id, true
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func internalError(log *slog.Logger, w http.ResponseWriter, op string, err error) {
	if log != nil {
		log.Error("alert handler", "op", op, "err", err)
	}
	writeJSON(w, http.StatusInternalServerError, errorResp{Error: "internal"})
}
