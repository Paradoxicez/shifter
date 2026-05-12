// Package alert — HTTP handlers for /api/alerts/rules + /api/anomaly-roster
// + /api/metering-points/{id}/anomaly-state (Plan 06-04).
//
// Each mutating handler opens a pgx.Tx → mutates → writes audit_log row in
// the SAME tx → commits (D-23). The Phase 6 alert.* audit vocabulary lives
// in internal/audit (audit.ActionAlertRule*).
package alert

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	sqlcdb "github.com/shifter-io/shifter/internal/db/sqlc"

	"github.com/shifter-io/shifter/internal/audit"
	"github.com/shifter-io/shifter/internal/auth"
)

// ─────────────────────────────────────────────────────────────────────────
// Wire shapes
// ─────────────────────────────────────────────────────────────────────────

type ruleDTO struct {
	ID              uuid.UUID  `json:"id"`
	RuleKind        string     `json:"rule_kind"`
	ScopeKind       string     `json:"scope_kind"`
	ScopeID         *uuid.UUID `json:"scope_id,omitempty"`
	HighBound       *float64   `json:"high_bound,omitempty"`
	LowBound        *float64   `json:"low_bound,omitempty"`
	Comparison      *string    `json:"comparison,omitempty"`
	Unit            *string    `json:"unit,omitempty"`
	FlowThreshold   *float64   `json:"flow_threshold,omitempty"`
	DaysOfWeek      *int32     `json:"days_of_week,omitempty"`
	Severity        string     `json:"severity"`
	Name            *string    `json:"name,omitempty"`
	Notes           *string    `json:"notes,omitempty"`
	CooldownSeconds int32      `json:"cooldown_seconds"`
	DisabledAt      *string    `json:"disabled_at,omitempty"`
}

func ruleToDTO(r RuleRecord) ruleDTO {
	d := ruleDTO{
		ID:              r.ID,
		RuleKind:        r.RuleKind,
		ScopeKind:       r.ScopeKind,
		ScopeID:         r.ScopeID,
		HighBound:       r.HighBound,
		LowBound:        r.LowBound,
		Comparison:      r.Comparison,
		Unit:            r.Unit,
		FlowThreshold:   r.FlowThreshold,
		DaysOfWeek:      r.DaysOfWeek,
		Severity:        r.Severity,
		Name:            r.Name,
		Notes:           r.Notes,
		CooldownSeconds: r.CooldownSeconds,
	}
	if r.DisabledAt != nil {
		s := r.DisabledAt.UTC().Format("2006-01-02T15:04:05Z")
		d.DisabledAt = &s
	}
	return d
}

type createRuleRequest struct {
	RuleKind        string     `json:"rule_kind"`
	ScopeKind       string     `json:"scope_kind"`
	ScopeID         *uuid.UUID `json:"scope_id,omitempty"`
	HighBound       *float64   `json:"high_bound,omitempty"`
	LowBound        *float64   `json:"low_bound,omitempty"`
	Comparison      *string    `json:"comparison,omitempty"`
	Unit            *string    `json:"unit,omitempty"`
	FlowThreshold   *float64   `json:"flow_threshold,omitempty"`
	DaysOfWeek      *int32     `json:"days_of_week,omitempty"`
	Severity        string     `json:"severity"`
	Name            *string    `json:"name,omitempty"`
	Notes           *string    `json:"notes,omitempty"`
	CooldownSeconds *int32     `json:"cooldown_seconds,omitempty"`
}

type updateRuleRequest struct {
	HighBound       *float64 `json:"high_bound,omitempty"`
	LowBound        *float64 `json:"low_bound,omitempty"`
	Comparison      *string  `json:"comparison,omitempty"`
	Unit            *string  `json:"unit,omitempty"`
	FlowThreshold   *float64 `json:"flow_threshold,omitempty"`
	DaysOfWeek      *int32   `json:"days_of_week,omitempty"`
	Severity        *string  `json:"severity,omitempty"`
	Name            *string  `json:"name,omitempty"`
	Notes           *string  `json:"notes,omitempty"`
	CooldownSeconds *int32   `json:"cooldown_seconds,omitempty"`
}

type toggleAnomalyRequest struct {
	Enabled bool `json:"enabled"`
}

// ─────────────────────────────────────────────────────────────────────────
// Handlers
// ─────────────────────────────────────────────────────────────────────────

// ListRulesHandler — GET /api/alerts/rules?show_disabled=0|1.
func ListRulesHandler(deps HTTPDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		showDisabled := r.URL.Query().Get("show_disabled") == "1"
		rows, err := deps.Rules.ListAllRules(r.Context(), showDisabled)
		if err != nil {
			internalError(deps.Log, w, "list rules", err)
			return
		}
		dtos := make([]ruleDTO, 0, len(rows))
		for _, row := range rows {
			dtos = append(dtos, ruleToDTO(row))
		}
		writeJSON(w, http.StatusOK, map[string]any{"rules": dtos})
	}
}

// CreateRuleHandler — POST /api/alerts/rules. Validates scope_kind/scope_id
// pairing; writes audit 'alert.rule_create' in the same tx (D-23).
func CreateRuleHandler(deps HTTPDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acting, ok := auth.GetUser(r.Context(), deps.SessionMgr)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errorResp{Error: "unauthorized"})
			return
		}
		var req createRuleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "bad_json"})
			return
		}
		if err := validateScopeKindScopeID(req.ScopeKind, req.ScopeID); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, errorResp{Error: err.Error()})
			return
		}
		if req.RuleKind == "" {
			writeJSON(w, http.StatusUnprocessableEntity, errorResp{Error: "missing_rule_kind"})
			return
		}
		if req.Severity == "" {
			req.Severity = "critical"
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

		created, err := deps.Rules.CreateRule(ctx, tx, CreateRuleParams{
			RuleKind:        req.RuleKind,
			ScopeKind:       req.ScopeKind,
			ScopeID:         req.ScopeID,
			HighBound:       req.HighBound,
			LowBound:        req.LowBound,
			Comparison:      req.Comparison,
			Unit:            req.Unit,
			FlowThreshold:   req.FlowThreshold,
			DaysOfWeek:      req.DaysOfWeek,
			Severity:        req.Severity,
			Name:            req.Name,
			Notes:           req.Notes,
			CooldownSeconds: req.CooldownSeconds,
			CreatedBy:       &actingUUID,
		})
		if err != nil {
			internalError(deps.Log, w, "create rule", err)
			return
		}
		if err := audit.WriteEntry(ctx, tx, audit.Entry{
			UserID:     actingUUID,
			Action:     audit.ActionAlertRuleCreate,
			EntityType: audit.EntityTypeAlertRule,
			EntityID:   created.ID,
			After:      ruleToAuditMap(created),
			RequestID:  middleware.GetReqID(ctx),
		}); err != nil {
			internalError(deps.Log, w, "audit rule create", err)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			internalError(deps.Log, w, "commit", err)
			return
		}
		writeJSON(w, http.StatusCreated, ruleToDTO(created))
	}
}

// UpdateRuleHandler — PATCH /api/alerts/rules/{id}. Applies fields one-by-one;
// the Before/After audit diff records only the changed fields per D-24.
func UpdateRuleHandler(deps HTTPDeps) http.HandlerFunc {
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
		var req updateRuleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "bad_json"})
			return
		}
		actingUUID, err := uuid.Parse(acting.ID)
		if err != nil {
			internalError(deps.Log, w, "parse acting id", err)
			return
		}

		ctx := r.Context()
		before, err := deps.Rules.GetRuleByID(ctx, id)
		if errors.Is(err, ErrRuleNotFound) {
			writeJSON(w, http.StatusNotFound, errorResp{Error: "not_found"})
			return
		}
		if err != nil {
			internalError(deps.Log, w, "load rule", err)
			return
		}

		beforeDiff, afterDiff := diffRuleUpdate(before, req)
		if len(afterDiff) == 0 {
			writeJSON(w, http.StatusOK, ruleToDTO(before))
			return
		}

		tx, err := deps.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
		if err != nil {
			internalError(deps.Log, w, "begin tx", err)
			return
		}
		defer func() { _ = tx.Rollback(ctx) }()

		updated, err := applyRuleUpdate(ctx, tx, before, req)
		if err != nil {
			internalError(deps.Log, w, "update rule", err)
			return
		}
		if err := audit.WriteEntry(ctx, tx, audit.Entry{
			UserID:     actingUUID,
			Action:     audit.ActionAlertRuleUpdate,
			EntityType: audit.EntityTypeAlertRule,
			EntityID:   id,
			Before:     beforeDiff,
			After:      afterDiff,
			RequestID:  middleware.GetReqID(ctx),
		}); err != nil {
			internalError(deps.Log, w, "audit rule update", err)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			internalError(deps.Log, w, "commit", err)
			return
		}
		writeJSON(w, http.StatusOK, ruleToDTO(updated))
	}
}

// DisableRuleHandler — POST /api/alerts/rules/{id}/disable.
func DisableRuleHandler(deps HTTPDeps) http.HandlerFunc {
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

		if err := deps.Rules.DisableRule(ctx, tx, id); err != nil {
			if errors.Is(err, ErrRuleNotFound) {
				writeJSON(w, http.StatusNotFound, errorResp{Error: "not_found"})
				return
			}
			internalError(deps.Log, w, "disable rule", err)
			return
		}
		if err := audit.WriteEntry(ctx, tx, audit.Entry{
			UserID:     actingUUID,
			Action:     audit.ActionAlertRuleDisable,
			EntityType: audit.EntityTypeAlertRule,
			EntityID:   id,
			RequestID:  middleware.GetReqID(ctx),
		}); err != nil {
			internalError(deps.Log, w, "audit rule disable", err)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			internalError(deps.Log, w, "commit", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "disabled": true})
	}
}

// EnableRuleHandler — POST /api/alerts/rules/{id}/enable.
func EnableRuleHandler(deps HTTPDeps) http.HandlerFunc {
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

		if err := deps.Rules.EnableRule(ctx, tx, id); err != nil {
			if errors.Is(err, ErrRuleNotFound) {
				writeJSON(w, http.StatusNotFound, errorResp{Error: "not_found"})
				return
			}
			internalError(deps.Log, w, "enable rule", err)
			return
		}
		if err := audit.WriteEntry(ctx, tx, audit.Entry{
			UserID:     actingUUID,
			Action:     audit.ActionAlertRuleEnable,
			EntityType: audit.EntityTypeAlertRule,
			EntityID:   id,
			RequestID:  middleware.GetReqID(ctx),
		}); err != nil {
			internalError(deps.Log, w, "audit rule enable", err)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			internalError(deps.Log, w, "commit", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": id, "disabled": false})
	}
}

// RosterHandler — GET /api/anomaly-roster.
func RosterHandler(deps HTTPDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		queries := sqlcdb.New(deps.Pool)
		roster, err := ListAnomalyWarmupRoster(r.Context(), queries)
		if err != nil {
			internalError(deps.Log, w, "list anomaly roster", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"rows": roster})
	}
}

// MPAnomalyStateHandler — GET /api/metering-points/{id}/anomaly-state.
func MPAnomalyStateHandler(deps HTTPDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idRaw := chi.URLParam(r, "id")
		mpID, err := uuid.Parse(idRaw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "invalid_id"})
			return
		}
		queries := sqlcdb.New(deps.Pool)
		eligible, err := IsMPEligibleForAnomaly(r.Context(), queries, mpID)
		if err != nil {
			internalError(deps.Log, w, "is mp eligible", err)
			return
		}
		var daysUntil int32
		if !eligible {
			roster, _ := ListAnomalyWarmupRoster(r.Context(), queries)
			for _, row := range roster {
				if row.MeteringPointID == mpID {
					daysUntil = row.DaysUntilEligible
					break
				}
			}
		}
		rules, err := GetMPAnomalyState(r.Context(), queries, mpID)
		if err != nil {
			internalError(deps.Log, w, "get mp anomaly state", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"eligible":            eligible,
			"days_until_eligible": daysUntil,
			"rules":               rules,
		})
	}
}

// ToggleMPAnomalyHandler — PATCH /api/metering-points/{id}/anomaly-rules/{kind}.
func ToggleMPAnomalyHandler(deps HTTPDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acting, ok := auth.GetUser(r.Context(), deps.SessionMgr)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, errorResp{Error: "unauthorized"})
			return
		}
		idRaw := chi.URLParam(r, "id")
		mpID, err := uuid.Parse(idRaw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "invalid_id"})
			return
		}
		kind := chi.URLParam(r, "kind")
		switch kind {
		case "p95", "iqr", "quiet_hour":
			// ok
		default:
			writeJSON(w, http.StatusUnprocessableEntity, errorResp{Error: "invalid_kind"})
			return
		}
		ruleKind := "anomaly_" + kind

		var req toggleAnomalyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "bad_json"})
			return
		}
		actingUUID, err := uuid.Parse(acting.ID)
		if err != nil {
			internalError(deps.Log, w, "parse acting id", err)
			return
		}

		ctx := r.Context()
		existing, err := findMPScopedRule(ctx, deps.Pool, mpID, ruleKind)
		if err != nil {
			internalError(deps.Log, w, "find mp-scoped rule", err)
			return
		}

		tx, err := deps.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
		if err != nil {
			internalError(deps.Log, w, "begin tx", err)
			return
		}
		defer func() { _ = tx.Rollback(ctx) }()

		if existing == nil {
			if !req.Enabled {
				writeJSON(w, http.StatusOK, map[string]any{
					"rule_kind": ruleKind, "enabled": false,
				})
				return
			}
			created, err := deps.Rules.CreateRule(ctx, tx, CreateRuleParams{
				RuleKind:  ruleKind,
				ScopeKind: "metering_point",
				ScopeID:   &mpID,
				Severity:  "warning",
				CreatedBy: &actingUUID,
			})
			if err != nil {
				internalError(deps.Log, w, "create mp anomaly rule", err)
				return
			}
			if err := audit.WriteEntry(ctx, tx, audit.Entry{
				UserID:     actingUUID,
				Action:     audit.ActionAlertRuleCreate,
				EntityType: audit.EntityTypeAlertRule,
				EntityID:   created.ID,
				After:      map[string]any{"rule_kind": ruleKind, "mp_id": mpID.String()},
				RequestID:  middleware.GetReqID(ctx),
			}); err != nil {
				internalError(deps.Log, w, "audit mp anomaly create", err)
				return
			}
			if err := tx.Commit(ctx); err != nil {
				internalError(deps.Log, w, "commit", err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"rule_kind": ruleKind, "enabled": true, "rule_id": created.ID,
			})
			return
		}

		if req.Enabled {
			if err := deps.Rules.EnableRule(ctx, tx, *existing); err != nil &&
				!errors.Is(err, ErrRuleNotFound) {
				internalError(deps.Log, w, "enable mp anomaly rule", err)
				return
			}
			if err := audit.WriteEntry(ctx, tx, audit.Entry{
				UserID: actingUUID, Action: audit.ActionAlertRuleEnable,
				EntityType: audit.EntityTypeAlertRule, EntityID: *existing,
				RequestID: middleware.GetReqID(ctx),
			}); err != nil {
				internalError(deps.Log, w, "audit mp anomaly enable", err)
				return
			}
		} else {
			if err := deps.Rules.DisableRule(ctx, tx, *existing); err != nil &&
				!errors.Is(err, ErrRuleNotFound) {
				internalError(deps.Log, w, "disable mp anomaly rule", err)
				return
			}
			if err := audit.WriteEntry(ctx, tx, audit.Entry{
				UserID: actingUUID, Action: audit.ActionAlertRuleDisable,
				EntityType: audit.EntityTypeAlertRule, EntityID: *existing,
				RequestID: middleware.GetReqID(ctx),
			}); err != nil {
				internalError(deps.Log, w, "audit mp anomaly disable", err)
				return
			}
		}
		if err := tx.Commit(ctx); err != nil {
			internalError(deps.Log, w, "commit", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"rule_kind": ruleKind, "enabled": req.Enabled, "rule_id": *existing,
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────

func validateScopeKindScopeID(scopeKind string, scopeID *uuid.UUID) error {
	switch scopeKind {
	case "global":
		if scopeID != nil {
			return fmt.Errorf("scope_id_must_be_null_for_global")
		}
	case "metering_point", "site", "device", "gateway":
		if scopeID == nil {
			return fmt.Errorf("scope_id_required")
		}
	default:
		return fmt.Errorf("invalid_scope_kind")
	}
	return nil
}

func diffRuleUpdate(before RuleRecord, req updateRuleRequest) (map[string]any, map[string]any) {
	b := map[string]any{}
	a := map[string]any{}
	if req.HighBound != nil && !floatPtrEq(before.HighBound, req.HighBound) {
		b["high_bound"] = before.HighBound
		a["high_bound"] = *req.HighBound
	}
	if req.LowBound != nil && !floatPtrEq(before.LowBound, req.LowBound) {
		b["low_bound"] = before.LowBound
		a["low_bound"] = *req.LowBound
	}
	if req.Comparison != nil && !strPtrEq(before.Comparison, req.Comparison) {
		b["comparison"] = before.Comparison
		a["comparison"] = *req.Comparison
	}
	if req.Unit != nil && !strPtrEq(before.Unit, req.Unit) {
		b["unit"] = before.Unit
		a["unit"] = *req.Unit
	}
	if req.FlowThreshold != nil && !floatPtrEq(before.FlowThreshold, req.FlowThreshold) {
		b["flow_threshold"] = before.FlowThreshold
		a["flow_threshold"] = *req.FlowThreshold
	}
	if req.DaysOfWeek != nil && !int32PtrEq(before.DaysOfWeek, req.DaysOfWeek) {
		b["days_of_week"] = before.DaysOfWeek
		a["days_of_week"] = *req.DaysOfWeek
	}
	if req.Severity != nil && *req.Severity != before.Severity {
		b["severity"] = before.Severity
		a["severity"] = *req.Severity
	}
	if req.Name != nil && !strPtrEq(before.Name, req.Name) {
		b["name"] = before.Name
		a["name"] = *req.Name
	}
	if req.Notes != nil && !strPtrEq(before.Notes, req.Notes) {
		b["notes"] = before.Notes
		a["notes"] = *req.Notes
	}
	if req.CooldownSeconds != nil && *req.CooldownSeconds != before.CooldownSeconds {
		b["cooldown_seconds"] = before.CooldownSeconds
		a["cooldown_seconds"] = *req.CooldownSeconds
	}
	return b, a
}

func applyRuleUpdate(ctx context.Context, tx pgx.Tx, before RuleRecord, req updateRuleRequest) (RuleRecord, error) {
	parts := []string{}
	args := []any{before.ID}
	idx := 2
	if req.HighBound != nil {
		parts = append(parts, "high_bound = $"+strconv.Itoa(idx))
		args = append(args, *req.HighBound)
		idx++
	}
	if req.LowBound != nil {
		parts = append(parts, "low_bound = $"+strconv.Itoa(idx))
		args = append(args, *req.LowBound)
		idx++
	}
	if req.Comparison != nil {
		parts = append(parts, "comparison = $"+strconv.Itoa(idx))
		args = append(args, *req.Comparison)
		idx++
	}
	if req.Unit != nil {
		parts = append(parts, "unit = $"+strconv.Itoa(idx))
		args = append(args, *req.Unit)
		idx++
	}
	if req.FlowThreshold != nil {
		parts = append(parts, "flow_threshold = $"+strconv.Itoa(idx))
		args = append(args, *req.FlowThreshold)
		idx++
	}
	if req.DaysOfWeek != nil {
		parts = append(parts, "days_of_week = $"+strconv.Itoa(idx))
		args = append(args, *req.DaysOfWeek)
		idx++
	}
	if req.Severity != nil {
		parts = append(parts, "severity = $"+strconv.Itoa(idx))
		args = append(args, *req.Severity)
		idx++
	}
	if req.Name != nil {
		parts = append(parts, "name = $"+strconv.Itoa(idx))
		args = append(args, *req.Name)
		idx++
	}
	if req.Notes != nil {
		parts = append(parts, "notes = $"+strconv.Itoa(idx))
		args = append(args, *req.Notes)
		idx++
	}
	if req.CooldownSeconds != nil {
		parts = append(parts, "cooldown_seconds = $"+strconv.Itoa(idx))
		args = append(args, *req.CooldownSeconds)
		idx++
	}
	if len(parts) == 0 {
		return before, nil
	}
	q := "UPDATE alert_rule SET " + strings.Join(parts, ", ") +
		", updated_at = now() WHERE id = $1 RETURNING " + ruleColumnsSelect
	row := tx.QueryRow(ctx, q, args...)
	return scanRule(row)
}

func findMPScopedRule(ctx context.Context, pool *pgxpool.Pool, mpID uuid.UUID, ruleKind string) (*uuid.UUID, error) {
	var idStr string
	err := pool.QueryRow(ctx,
		`SELECT id FROM alert_rule
		 WHERE rule_kind = $1 AND scope_kind = 'metering_point' AND scope_id = $2
		 ORDER BY created_at DESC LIMIT 1`,
		ruleKind, mpID).Scan(&idStr)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func ruleToAuditMap(r RuleRecord) map[string]any {
	m := map[string]any{
		"rule_kind":        r.RuleKind,
		"scope_kind":       r.ScopeKind,
		"severity":         r.Severity,
		"cooldown_seconds": r.CooldownSeconds,
	}
	if r.ScopeID != nil {
		m["scope_id"] = r.ScopeID.String()
	}
	if r.HighBound != nil {
		m["high_bound"] = *r.HighBound
	}
	if r.LowBound != nil {
		m["low_bound"] = *r.LowBound
	}
	if r.Comparison != nil {
		m["comparison"] = *r.Comparison
	}
	if r.Unit != nil {
		m["unit"] = *r.Unit
	}
	if r.Name != nil {
		m["name"] = *r.Name
	}
	return m
}

func floatPtrEq(a, b *float64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
func strPtrEq(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
func int32PtrEq(a, b *int32) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
