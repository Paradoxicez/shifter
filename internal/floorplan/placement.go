package floorplan

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/shifter-io/shifter/internal/audit"
	"github.com/shifter-io/shifter/internal/auth"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// PinRequest is the JSON body for POST /api/floor-plans/{id}/placements.
type PinRequest struct {
	DeviceID string  `json:"device_id"`
	XFrac    float32 `json:"x_frac"`
	YFrac    float32 `json:"y_frac"`
}

// NudgeRequest is the JSON body for PATCH /api/floor-plans/{id}/placements/{deviceID}.
type NudgeRequest struct {
	XFrac float32 `json:"x_frac"`
	YFrac float32 `json:"y_frac"`
}

// UpsertPlacementHandler handles POST /api/floor-plans/{id}/placements.
//
// Same-site integrity guard (T-05-07-01): returns 409 site_mismatch if the
// device's site (via active binding → metering_point.site_id) does not match
// the floor_plan's site_id. Unbound devices are rejected with 422.
//
// Coordinates are clamped client-side and also validated here (defense-in-depth).
// The DB CHECK on x_frac/y_frac ∈ [0,1] is the final layer.
func UpsertPlacementHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.GetUser(r.Context(), deps.SessionMgr)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		planID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_plan_id")
			return
		}

		var req PinRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json")
			return
		}
		deviceID, err := uuid.Parse(req.DeviceID)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_device_id")
			return
		}

		// Defense-in-depth: clamp to [0,1] even though DB CHECK enforces it.
		if req.XFrac < 0 || req.XFrac > 1 || req.YFrac < 0 || req.YFrac > 1 {
			writeError(w, http.StatusUnprocessableEntity, "fraction_out_of_range")
			return
		}

		tx, err := deps.Pool.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "tx_begin")
			return
		}
		defer tx.Rollback(r.Context())

		q := deps.Queries.WithTx(tx)

		// Same-site integrity check: load the floor plan to get its site_id.
		plan, err := q.GetFloorPlan(r.Context(), pgtype.UUID{Bytes: planID, Valid: true})
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "plan_not_found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "plan_load_failed")
			return
		}

		// GetDeviceSiteID returns NULL site_id when device has no active binding.
		deviceSiteID, err := q.GetDeviceSiteID(r.Context(), pgtype.UUID{Bytes: deviceID, Valid: true})
		if errors.Is(err, pgx.ErrNoRows) {
			// ErrNoRows means the device does not exist (decommissioned_at IS NOT NULL
			// OR no device row). Treat as 404.
			writeError(w, http.StatusNotFound, "device_not_found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "device_lookup_failed")
			return
		}
		if !deviceSiteID.Valid {
			// NULL site_id means device exists but has no active binding.
			writeError(w, http.StatusUnprocessableEntity, "device_unbound")
			return
		}
		if deviceSiteID.Bytes != plan.SiteID.Bytes {
			writeError(w, http.StatusConflict, "site_mismatch")
			return
		}

		// Capture "before" state for audit (empty = first pin).
		before, beforeErr := q.GetPlacementByDevice(r.Context(), pgtype.UUID{Bytes: deviceID, Valid: true})
		action := audit.ActionPlacementPin
		var beforeMap map[string]any
		if beforeErr == nil {
			// Existing placement — this is a re-pin (move to new plan or nudge).
			action = audit.ActionPlacementNudge
			beforeMap = map[string]any{
				"floor_plan_id": uuid.UUID(before.FloorPlanID.Bytes).String(),
				"x_frac":        before.XFrac,
				"y_frac":        before.YFrac,
			}
		}

		placement, err := q.UpsertPlacement(r.Context(), sqlc.UpsertPlacementParams{
			DeviceID:    pgtype.UUID{Bytes: deviceID, Valid: true},
			FloorPlanID: pgtype.UUID{Bytes: planID, Valid: true},
			XFrac:       req.XFrac,
			YFrac:       req.YFrac,
		})
		if err != nil {
			// DB CHECK violation on x_frac/y_frac → 422 (defense-in-depth; handler
			// already rejected out-of-range above, this catches any bypass).
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23514" {
				writeError(w, http.StatusUnprocessableEntity, "fraction_out_of_range")
				return
			}
			writeError(w, http.StatusInternalServerError, "upsert_failed")
			return
		}

		if err := audit.WriteEntry(r.Context(), tx, audit.Entry{
			UserID:     userUUID(user.ID),
			Action:     action,
			EntityType: audit.EntityTypePlacement,
			EntityID:   deviceID,
			Before:     beforeMap,
			After: map[string]any{
				"floor_plan_id": planID.String(),
				"x_frac":        req.XFrac,
				"y_frac":        req.YFrac,
			},
			RequestID: middleware.GetReqID(r.Context()),
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "audit_failed")
			return
		}

		if err := tx.Commit(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "tx_commit")
			return
		}
		writeJSON(w, http.StatusCreated, placement)
	}
}

// UpdatePlacementHandler handles PATCH /api/floor-plans/{id}/placements/{deviceID}.
//
// Drag-to-nudge: only x_frac / y_frac change; floor_plan_id stays.
// Audit action = placement.nudge with before/after coords.
func UpdatePlacementHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.GetUser(r.Context(), deps.SessionMgr)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		_, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_plan_id")
			return
		}
		deviceID, err := uuid.Parse(chi.URLParam(r, "deviceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_device_id")
			return
		}

		var req NudgeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_json")
			return
		}
		if req.XFrac < 0 || req.XFrac > 1 || req.YFrac < 0 || req.YFrac > 1 {
			writeError(w, http.StatusUnprocessableEntity, "fraction_out_of_range")
			return
		}

		tx, err := deps.Pool.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "tx_begin")
			return
		}
		defer tx.Rollback(r.Context())

		q := deps.Queries.WithTx(tx)

		// Capture before state for audit.
		before, err := q.GetPlacementByDevice(r.Context(), pgtype.UUID{Bytes: deviceID, Valid: true})
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "placement_not_found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "placement_lookup_failed")
			return
		}

		updated, err := q.UpdatePlacement(r.Context(), sqlc.UpdatePlacementParams{
			DeviceID: pgtype.UUID{Bytes: deviceID, Valid: true},
			XFrac:    req.XFrac,
			YFrac:    req.YFrac,
		})
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23514" {
				writeError(w, http.StatusUnprocessableEntity, "fraction_out_of_range")
				return
			}
			writeError(w, http.StatusInternalServerError, "update_failed")
			return
		}

		if err := audit.WriteEntry(r.Context(), tx, audit.Entry{
			UserID:     userUUID(user.ID),
			Action:     audit.ActionPlacementNudge,
			EntityType: audit.EntityTypePlacement,
			EntityID:   deviceID,
			Before: map[string]any{
				"x_frac": before.XFrac,
				"y_frac": before.YFrac,
			},
			After: map[string]any{
				"x_frac": req.XFrac,
				"y_frac": req.YFrac,
			},
			RequestID: middleware.GetReqID(r.Context()),
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "audit_failed")
			return
		}

		if err := tx.Commit(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "tx_commit")
			return
		}
		writeJSON(w, http.StatusOK, updated)
	}
}

// DeletePlacementHandler handles DELETE /api/floor-plans/{id}/placements/{deviceID}.
//
// Removes the placement row and writes a placement.remove audit entry in the same tx.
func DeletePlacementHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := auth.GetUser(r.Context(), deps.SessionMgr)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		_, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_plan_id")
			return
		}
		deviceID, err := uuid.Parse(chi.URLParam(r, "deviceID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_device_id")
			return
		}

		tx, err := deps.Pool.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "tx_begin")
			return
		}
		defer tx.Rollback(r.Context())

		q := deps.Queries.WithTx(tx)

		// Capture before state for audit.
		before, err := q.GetPlacementByDevice(r.Context(), pgtype.UUID{Bytes: deviceID, Valid: true})
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "placement_not_found")
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, "placement_lookup_failed")
			return
		}

		if err := q.DeletePlacementByDevice(r.Context(), pgtype.UUID{Bytes: deviceID, Valid: true}); err != nil {
			writeError(w, http.StatusInternalServerError, "delete_failed")
			return
		}

		if err := audit.WriteEntry(r.Context(), tx, audit.Entry{
			UserID:     userUUID(user.ID),
			Action:     audit.ActionPlacementRemove,
			EntityType: audit.EntityTypePlacement,
			EntityID:   deviceID,
			Before: map[string]any{
				"floor_plan_id": uuid.UUID(before.FloorPlanID.Bytes).String(),
				"x_frac":        before.XFrac,
				"y_frac":        before.YFrac,
			},
			RequestID: middleware.GetReqID(r.Context()),
		}); err != nil {
			writeError(w, http.StatusInternalServerError, "audit_failed")
			return
		}

		if err := tx.Commit(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, "tx_commit")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ListPlacementsHandler handles GET /api/floor-plans/{id}/placements.
//
// Returns denormalized rows for client-side D-22 health computation (state
// colors). Each row includes device_name, utility_class, last_seen_at,
// battery_pct, rssi, and expected_interval_s without a follow-up call.
func ListPlacementsHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		planID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_plan_id")
			return
		}

		rows, err := deps.Queries.ListPlacementsByPlan(r.Context(), pgtype.UUID{Bytes: planID, Valid: true})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "db_query_failed")
			return
		}
		if rows == nil {
			rows = []sqlc.ListPlacementsByPlanRow{}
		}
		writeJSON(w, http.StatusOK, rows)
	}
}
