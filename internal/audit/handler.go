package audit

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Deps bundles the shared infra every audit browse handler needs.
// The audit package does NOT import auth (cycle: auth→audit). Route-level
// RequireAction guards are applied by the caller (internal/http/router.go).
type Deps struct {
	Pool        *pgxpool.Pool
	Store       *Store
	Log         *slog.Logger
	RiverClient RiverInserter // optional; nil = ExportAsyncHandler skips River enqueue
}

// ListHandler handles GET /api/audit with cursor pagination and filters.
// Response: {rows: [...], next_cursor: "..." | "", total: N}
func ListHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter, err := parseFilterFromQuery(r.URL.Query())
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}

		var cursor *Cursor
		if c := r.URL.Query().Get("cursor"); c != "" {
			cursor = DecodeCursor(c)
		}

		result, err := deps.Store.ListCursor(r.Context(), filter, cursor)
		if err != nil {
			deps.Log.Error("audit.list", "err", err)
			writeError(w, http.StatusInternalServerError, "internal_error")
			return
		}

		// Serialize rows to wire format.
		type wireRow struct {
			ID         string          `json:"id"`
			Time       string          `json:"time"`
			Action     string          `json:"action"`
			EntityType string          `json:"entity_type"`
			EntityID   string          `json:"entity_id"`
			Before     json.RawMessage `json:"before"`
			After      json.RawMessage `json:"after"`
			Notes      string          `json:"notes"`
			RequestID  string          `json:"request_id"`
			UserID     *string         `json:"user_id"`
			UserEmail  string          `json:"user_email"`
		}
		wireRows := make([]wireRow, 0, len(result.Rows))
		for _, row := range result.Rows {
			wr := wireRow{
				ID:         row.ID.String(),
				Time:       row.Time.UTC().Format(time.RFC3339),
				Action:     row.Action,
				EntityType: row.EntityType,
				EntityID:   row.EntityID.String(),
				Before:     nullableJSON(row.Before),
				After:      nullableJSON(row.After),
				Notes:      row.Notes,
				RequestID:  row.RequestID,
				UserEmail:  row.UserEmail,
			}
			if row.UserID != nil {
				s := row.UserID.String()
				wr.UserID = &s
			}
			wireRows = append(wireRows, wr)
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"rows":        wireRows,
			"next_cursor": result.NextCursor,
			"total":       result.Total,
		})
	}
}

// CountHandler handles GET /api/audit/count?... → {count: N}.
// Used by the Export button's >50k decision.
func CountHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter, err := parseFilterFromQuery(r.URL.Query())
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		n, err := deps.Store.Count(r.Context(), filter)
		if err != nil {
			deps.Log.Error("audit.count", "err", err)
			writeError(w, http.StatusInternalServerError, "internal_error")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"count": n})
	}
}

// DistinctsHandler handles GET /api/audit/distincts →
// {actions: [...], entity_types: [...]}
// Used to populate filter dropdown options.
func DistinctsHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actions, err := deps.Store.DistinctActions(r.Context())
		if err != nil {
			deps.Log.Error("audit.distincts.actions", "err", err)
			writeError(w, http.StatusInternalServerError, "internal_error")
			return
		}
		entityTypes, err := deps.Store.DistinctEntityTypes(r.Context())
		if err != nil {
			deps.Log.Error("audit.distincts.entity_types", "err", err)
			writeError(w, http.StatusInternalServerError, "internal_error")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"actions":      actions,
			"entity_types": entityTypes,
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────
// Filter parsing
// ─────────────────────────────────────────────────────────────────────────

// validEntityTypes is the server-side vocabulary for entity_type validation.
// Must mirror the CHECK constraint in migration 0016 + 0037 + phase 6 expansions.
// T-06-07-02 mitigation: unknown values are rejected with 422.
var validEntityTypes = map[string]bool{
	EntityTypeSite:          true,
	EntityTypeMeteringPoint: true,
	EntityTypeDevice:        true,
	EntityTypeDeviceProfile: true,
	EntityTypeBinding:       true,
	EntityTypeGateway:       true,
	EntityTypeImportJob:     true,
	EntityTypeReport:        true,
	EntityTypeFloorPlan:     true,
	EntityTypePlacement:     true,
	EntityTypeRetention:     true,
	EntityTypeUser:          true,
	EntityTypeSession:       true,
	EntityTypeAlertRule:     true,
	EntityTypeAlert:         true,
	EntityTypeBackupRun:     true,
	EntityTypeAuditLog:      true,
}

// validActions is the server-side vocabulary for action validation.
var validActions = map[string]bool{
	ActionCreate:                   true,
	ActionUpdate:                   true,
	ActionArchive:                  true,
	ActionRestore:                  true,
	ActionSwap:                     true,
	ActionDecommission:             true,
	ActionProfileCreate:            true,
	ActionProfileUpdate:            true,
	ActionBindingOpen:              true,
	ActionBindingClose:             true,
	ActionRolloverDetected:         true,
	ActionGatewayCreate:            true,
	ActionGatewayUpdate:            true,
	ActionGatewayArchive:           true,
	ActionGatewayRestore:           true,
	ActionBulkImport:               true,
	ActionRevealSecrets:            true,
	ActionGenerateReport:           true,
	ActionFloorPlanUpload:          true,
	ActionFloorPlanReplace:         true,
	ActionFloorPlanRename:          true,
	ActionFloorPlanDelete:          true,
	ActionPlacementPin:             true,
	ActionPlacementNudge:           true,
	ActionPlacementRemove:          true,
	ActionRetentionChange:          true,
	ActionAuthLoginSuccess:         true,
	ActionAuthLoginFailed:          true,
	ActionAuthLogout:               true,
	ActionAuthPasswordChange:       true,
	ActionAuthPasswordResetByAdmin: true,
	ActionAuthSessionRevoked:       true,
	ActionUserCreate:               true,
	ActionUserUpdate:               true,
	ActionUserDisable:              true,
	ActionUserEnable:               true,
	ActionUserRoleChange:           true,
	ActionAlertRuleCreate:          true,
	ActionAlertRuleUpdate:          true,
	ActionAlertRuleDisable:         true,
	ActionAlertRuleEnable:          true,
	ActionAlertFired:               true,
	ActionAlertCleared:             true,
	ActionAlertAcked:               true,
	ActionAlertSnoozed:             true,
	ActionAlertMuted:               true,
	ActionAlertTestFired:           true,
	ActionBackupStart:              true,
	ActionBackupComplete:           true,
	ActionBackupFailed:             true,
	ActionBackupRestore:            true,
	ActionAuditPrune:               true,
	ActionAuditExport:              true,
}

// parseFilterFromQuery parses URL query parameters into a Filter.
// Returns an error (mapped to 422) on unknown entity_type or action values.
func parseFilterFromQuery(q url.Values) (Filter, error) {
	var f Filter

	if s := q.Get("from"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return f, fmt.Errorf("bad_from: %w", err)
		}
		f.From = &t
	}

	if s := q.Get("to"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return f, fmt.Errorf("bad_to: %w", err)
		}
		f.To = &t
	}

	if s := q.Get("user_id"); s != "" {
		uid, err := uuid.Parse(s)
		if err != nil {
			return f, fmt.Errorf("bad_user_id")
		}
		f.UserID = &uid
	}

	// entity_type may be repeated: ?entity_type=user&entity_type=site
	// or comma-separated: ?entity_type=user,site
	for _, v := range q["entity_type"] {
		for _, part := range splitComma(v) {
			if !validEntityTypes[part] {
				return f, fmt.Errorf("unknown_entity_type: %s", part)
			}
			f.EntityTypes = append(f.EntityTypes, part)
		}
	}

	// action[] filter — same pattern.
	for _, v := range q["action"] {
		for _, part := range splitComma(v) {
			if !validActions[part] {
				return f, fmt.Errorf("unknown_action: %s", part)
			}
			f.Actions = append(f.Actions, part)
		}
	}

	if s := q.Get("request_id"); s != "" {
		f.RequestIDLike = &s
	}

	return f, nil
}

func splitComma(s string) []string {
	if !strings.Contains(s, ",") {
		return []string{s}
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// ─────────────────────────────────────────────────────────────────────────
// Shared helpers
// ─────────────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}

func nullableJSON(b []byte) json.RawMessage {
	if len(b) == 0 {
		return json.RawMessage("null")
	}
	return json.RawMessage(b)
}
