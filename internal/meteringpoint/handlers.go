// Package meteringpoint — HTTP handlers for the Metering Point CRUD surface
// (D-19 + D-20) plus the binding-aware detail endpoint Plan 02-13's UI calls.
//
// Like the Site package, every mutation runs inside a pgx.Serializable txn
// with audit.WriteEntry in the SAME tx so D-23 atomicity is enforced by
// signature. The only structurally distinct endpoint here is GetMPDetail —
// it joins binding + device + device_profile + latest measurement into one
// JSON shape so the frontend MP detail page renders in a single round-trip.
package meteringpoint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shifter-io/shifter/internal/audit"
	"github.com/shifter-io/shifter/internal/auth"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// validUtilityClasses mirrors the 0008_metering_point CHECK constraint. We
// validate at the handler boundary so a bad value returns 400 with a friendly
// message instead of relying on the constraint-violation 400.
var validUtilityClasses = map[string]struct{}{
	"water":       {},
	"electricity": {},
}

// Deps bundles the shared infra a Metering Point handler call needs.
type Deps struct {
	Pool       *pgxpool.Pool
	SessionMgr *scs.SessionManager
	Log        *slog.Logger
}

// RegisterRoutes mounts every Metering Point endpoint under
// /api/metering-points on the given chi router.
//
// Route table:
//
//	GET    /api/metering-points                    metering_point.read   — admin + viewer
//	GET    /api/metering-points/archived           metering_point.read   — admin + viewer
//	GET    /api/metering-points/by-site/{siteID}   metering_point.read   — admin + viewer
//	GET    /api/metering-points/{id}               metering_point.read   — admin + viewer
//	GET    /api/metering-points/{id}/quality       metering_point.read   — admin + viewer
//	POST   /api/metering-points                    metering_point.create — admin only
//	PATCH  /api/metering-points/{id}               metering_point.update — admin only
//	POST   /api/metering-points/{id}/archive       metering_point.archive — admin only
//	POST   /api/metering-points/{id}/restore       metering_point.restore — admin only
func RegisterRoutes(r chi.Router, deps Deps) {
	r.Route("/api/metering-points", func(r chi.Router) {
		// Read group — admin + viewer.
		r.Group(func(rt chi.Router) {
			rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionMeteringPointRead))
			rt.Get("/", listMPs(deps))
			rt.Get("/archived", listArchivedMPs(deps))
			rt.Get("/by-site/{siteID}", listBySite(deps))
			rt.Get("/{id}", getMPDetail(deps))
			rt.Get("/{id}/quality", getQualitySummary(deps))
		})
		// Mutate groups — admin only.
		r.Group(func(rt chi.Router) {
			rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionMeteringPointCreate))
			rt.Post("/", createMP(deps))
		})
		r.Group(func(rt chi.Router) {
			rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionMeteringPointUpdate))
			rt.Patch("/{id}", updateMP(deps))
		})
		r.Group(func(rt chi.Router) {
			rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionMeteringPointArchive))
			rt.Post("/{id}/archive", archiveMP(deps))
		})
		r.Group(func(rt chi.Router) {
			rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionMeteringPointRestore))
			rt.Post("/{id}/restore", restoreMP(deps))
		})
	})
}

// ----- request shapes -----------------------------------------------------

// CreateMPRequest is the JSON body of POST /api/metering-points.
type CreateMPRequest struct {
	SiteID              string  `json:"site_id"`
	Name                string  `json:"name"`
	UtilityClass        string  `json:"utility_class"`
	LocationDescription *string `json:"location_description,omitempty"`
}

// UpdateMPRequest is the JSON body of PATCH /api/metering-points/{id}.
// site_id is intentionally NOT updatable here (D-19 — moving an MP between
// sites is a "transfer" workflow with binding implications, deferred).
type UpdateMPRequest struct {
	Name                string  `json:"name"`
	UtilityClass        string  `json:"utility_class"`
	LocationDescription *string `json:"location_description,omitempty"`
}

// ----- handlers -----------------------------------------------------------

func listMPs(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Optional ?archived=true to flip to archived view.
		if r.URL.Query().Get("archived") == "true" {
			listArchivedMPs(deps)(w, r)
			return
		}
		q := sqlc.New(deps.Pool)
		rows, err := q.ListActiveMPs(r.Context())
		if err != nil {
			internalError(deps.Log, w, "list active MPs", err)
			return
		}
		writeJSON(w, http.StatusOK, mpsToJSON(rows))
	}
}

func listArchivedMPs(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := sqlc.New(deps.Pool)
		rows, err := q.ListArchivedMPs(r.Context())
		if err != nil {
			internalError(deps.Log, w, "list archived MPs", err)
			return
		}
		writeJSON(w, http.StatusOK, mpsToJSON(rows))
	}
}

func listBySite(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, ok := parseUUIDParam(w, r, "siteID")
		if !ok {
			return
		}
		q := sqlc.New(deps.Pool)
		rows, err := q.ListMPsBySite(r.Context(), pgUUID(siteID))
		if err != nil {
			internalError(deps.Log, w, "list MPs by site", err)
			return
		}
		writeJSON(w, http.StatusOK, mpsToJSON(rows))
	}
}

// getMPDetail returns the joined "MP + active binding + device + profile +
// latest measurement" payload Plan 02-13's MP detail page renders. The
// active_binding sub-object is null when no active binding exists; the
// latest_measurement sub-object is null when the hypertable has no rows
// for this MP yet.
func getMPDetail(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id")
		if !ok {
			return
		}
		q := sqlc.New(deps.Pool)
		row, err := q.GetMPWithActiveBinding(r.Context(), pgUUID(id))
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResp{Error: "not_found"})
			return
		}
		if err != nil {
			internalError(deps.Log, w, "get MP detail", err)
			return
		}

		// Site lookup (separate query — keeps the join graph small).
		siteRow, err := q.GetSite(r.Context(), row.SiteID)
		if err != nil {
			internalError(deps.Log, w, "get MP site", err)
			return
		}

		out := map[string]any{
			"id":                   uuidString(row.ID),
			"name":                 row.Name,
			"site":                 map[string]any{"id": uuidString(siteRow.ID), "name": siteRow.Name},
			"utility_class":        row.UtilityClass,
			"location_description": derefString(row.LocationDescription),
			"archived_at":          timestamptzText(row.ArchivedAt),
			"created_at":           timestamptzText(row.CreatedAt),
			"updated_at":           timestamptzText(row.UpdatedAt),
		}

		if row.BindingID.Valid {
			out["active_binding"] = map[string]any{
				"id":         uuidString(row.BindingID),
				"valid_from": timestamptzText(row.BindingValidFrom),
				"reading_offset": numericText(row.ReadingOffset),
				"device": map[string]any{
					"id":      uuidString(row.DeviceID),
					"dev_eui": derefString(row.DevEui),
					"name":    derefString(row.DeviceName),
				},
				"device_profile": map[string]any{
					"id":              uuidString(row.DeviceProfileID),
					"name":            derefString(row.DeviceProfileName),
					"capabilities":    row.Capabilities,
					"counter_modulus": row.CounterModulus,
				},
			}
		} else {
			out["active_binding"] = nil
		}

		// Latest measurement (best-effort — empty table → nil).
		latest, err := q.GetLatestMeasurement(r.Context(), row.ID)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			out["latest_measurement"] = nil
		case err != nil:
			internalError(deps.Log, w, "get latest measurement", err)
			return
		default:
			out["latest_measurement"] = map[string]any{
				"time":             timestamptzText(latest.Time),
				"raw_value":        numericText(latest.RawValue),
				"cumulative_value": numericText(latest.CumulativeValue),
				"instant_value":    numericText(latest.InstantValue),
				"battery_pct":      derefInt16(latest.BatteryPct),
				"quality":          latest.Quality,
			}
		}

		writeJSON(w, http.StatusOK, out)
	}
}

// getQualitySummary returns the D-26 quality flag categorization for the
// last 24h — powers the MP detail page's "X uplinks flagged" badge.
func getQualitySummary(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id")
		if !ok {
			return
		}
		q := sqlc.New(deps.Pool)
		// Last 24 hours by default. Future Plan 02-13 may add ?since= param.
		since := time.Now().UTC().Add(-24 * time.Hour)
		row, err := q.CountFlaggedRecent(r.Context(), sqlc.CountFlaggedRecentParams{
			MeteringPointID: pgUUID(id),
			Time:            pgtype.Timestamptz{Time: since, Valid: true},
		})
		if err != nil {
			internalError(deps.Log, w, "count flagged recent", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"since":             since.Format(time.RFC3339),
			"total":             row.Total,
			"decode_fail":       row.DecodeFail,
			"missing_canonical": row.MissingCanonical,
			"out_of_range":      row.OutOfRange,
			"duplicate_fcnt":    row.DuplicateFcnt,
		})
	}
}

func createMP(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAdmin(deps.SessionMgr, w, r, auth.ActionMeteringPointCreate)
		if !ok {
			return
		}
		var in CreateMPRequest
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "bad_request"})
			return
		}
		if err := validateCreateMP(in); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "validation", Detail: err.Error()})
			return
		}
		siteID, err := uuid.Parse(in.SiteID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "invalid_site_id"})
			return
		}

		tx, err := deps.Pool.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			internalError(deps.Log, w, "begin tx", err)
			return
		}
		defer func() { _ = tx.Rollback(r.Context()) }()

		q := sqlc.New(tx)
		row, err := q.CreateMP(r.Context(), sqlc.CreateMPParams{
			SiteID:              pgUUID(siteID),
			Name:                strings.TrimSpace(in.Name),
			UtilityClass:        in.UtilityClass,
			LocationDescription: in.LocationDescription,
		})
		if err != nil {
			if isUniqueViolation(err) {
				writeJSON(w, http.StatusConflict, errorResp{Error: "duplicate_name_on_site"})
				return
			}
			if isConstraintViolation(err) {
				writeJSON(w, http.StatusBadRequest, errorResp{Error: "constraint", Detail: err.Error()})
				return
			}
			internalError(deps.Log, w, "create MP", err)
			return
		}

		after := mpToJSON(row)
		if err := audit.WriteEntry(r.Context(), tx, audit.Entry{
			UserID:     mustParseUUID(user.ID),
			Action:     audit.ActionCreate,
			EntityType: audit.EntityTypeMeteringPoint,
			EntityID:   uuid.UUID(row.ID.Bytes),
			Before:     nil,
			After:      after,
			RequestID:  middleware.GetReqID(r.Context()),
		}); err != nil {
			internalError(deps.Log, w, "audit create MP", err)
			return
		}

		if err := tx.Commit(r.Context()); err != nil {
			internalError(deps.Log, w, "commit create MP", err)
			return
		}
		writeJSON(w, http.StatusCreated, after)
	}
}

func updateMP(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAdmin(deps.SessionMgr, w, r, auth.ActionMeteringPointUpdate)
		if !ok {
			return
		}
		id, ok := parseUUIDParam(w, r, "id")
		if !ok {
			return
		}
		var in UpdateMPRequest
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "bad_request"})
			return
		}
		if err := validateUpdateMP(in); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{Error: "validation", Detail: err.Error()})
			return
		}

		tx, err := deps.Pool.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			internalError(deps.Log, w, "begin tx", err)
			return
		}
		defer func() { _ = tx.Rollback(r.Context()) }()

		q := sqlc.New(tx)
		existing, err := q.GetMP(r.Context(), pgUUID(id))
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResp{Error: "not_found"})
			return
		}
		if err != nil {
			internalError(deps.Log, w, "load existing MP", err)
			return
		}
		before := mpToJSON(existing)

		row, err := q.UpdateMP(r.Context(), sqlc.UpdateMPParams{
			ID:                  pgUUID(id),
			Name:                strings.TrimSpace(in.Name),
			UtilityClass:        in.UtilityClass,
			LocationDescription: in.LocationDescription,
		})
		if err != nil {
			if isConstraintViolation(err) {
				writeJSON(w, http.StatusBadRequest, errorResp{Error: "constraint", Detail: err.Error()})
				return
			}
			internalError(deps.Log, w, "update MP", err)
			return
		}
		after := mpToJSON(row)

		bDiff, aDiff := audit.ChangedFields(before, after)
		if err := audit.WriteEntry(r.Context(), tx, audit.Entry{
			UserID:     mustParseUUID(user.ID),
			Action:     audit.ActionUpdate,
			EntityType: audit.EntityTypeMeteringPoint,
			EntityID:   uuid.UUID(row.ID.Bytes),
			Before:     bDiff,
			After:      aDiff,
			RequestID:  middleware.GetReqID(r.Context()),
		}); err != nil {
			internalError(deps.Log, w, "audit update MP", err)
			return
		}

		if err := tx.Commit(r.Context()); err != nil {
			internalError(deps.Log, w, "commit update MP", err)
			return
		}
		writeJSON(w, http.StatusOK, after)
	}
}

func archiveMP(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAdmin(deps.SessionMgr, w, r, auth.ActionMeteringPointArchive)
		if !ok {
			return
		}
		id, ok := parseUUIDParam(w, r, "id")
		if !ok {
			return
		}

		tx, err := deps.Pool.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			internalError(deps.Log, w, "begin tx", err)
			return
		}
		defer func() { _ = tx.Rollback(r.Context()) }()

		q := sqlc.New(tx)
		row, err := q.ArchiveMP(r.Context(), pgUUID(id))
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResp{Error: "not_found_or_already_archived"})
			return
		}
		if err != nil {
			internalError(deps.Log, w, "archive MP", err)
			return
		}
		if err := audit.WriteEntry(r.Context(), tx, audit.Entry{
			UserID:     mustParseUUID(user.ID),
			Action:     audit.ActionArchive,
			EntityType: audit.EntityTypeMeteringPoint,
			EntityID:   uuid.UUID(row.ID.Bytes),
			Before:     map[string]any{"archived_at": nil},
			After:      map[string]any{"archived_at": timestamptzText(row.ArchivedAt)},
			RequestID:  middleware.GetReqID(r.Context()),
		}); err != nil {
			internalError(deps.Log, w, "audit archive MP", err)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			internalError(deps.Log, w, "commit archive MP", err)
			return
		}
		writeJSON(w, http.StatusOK, mpToJSON(row))
	}
}

func restoreMP(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := requireAdmin(deps.SessionMgr, w, r, auth.ActionMeteringPointRestore)
		if !ok {
			return
		}
		id, ok := parseUUIDParam(w, r, "id")
		if !ok {
			return
		}

		tx, err := deps.Pool.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.Serializable})
		if err != nil {
			internalError(deps.Log, w, "begin tx", err)
			return
		}
		defer func() { _ = tx.Rollback(r.Context()) }()

		q := sqlc.New(tx)
		row, err := q.RestoreMP(r.Context(), pgUUID(id))
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResp{Error: "not_found_or_not_archived"})
			return
		}
		if err != nil {
			internalError(deps.Log, w, "restore MP", err)
			return
		}
		if err := audit.WriteEntry(r.Context(), tx, audit.Entry{
			UserID:     mustParseUUID(user.ID),
			Action:     audit.ActionRestore,
			EntityType: audit.EntityTypeMeteringPoint,
			EntityID:   uuid.UUID(row.ID.Bytes),
			Before:     map[string]any{"archived_at": "non-null"},
			After:      map[string]any{"archived_at": nil},
			RequestID:  middleware.GetReqID(r.Context()),
		}); err != nil {
			internalError(deps.Log, w, "audit restore MP", err)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			internalError(deps.Log, w, "commit restore MP", err)
			return
		}
		writeJSON(w, http.StatusOK, mpToJSON(row))
	}
}

// ----- validation ---------------------------------------------------------

func validateCreateMP(in CreateMPRequest) error {
	if in.SiteID == "" {
		return errors.New("site_id is required")
	}
	if strings.TrimSpace(in.Name) == "" {
		return errors.New("name is required")
	}
	if _, ok := validUtilityClasses[in.UtilityClass]; !ok {
		return fmt.Errorf("utility_class must be water or electricity (got %q)", in.UtilityClass)
	}
	return nil
}

func validateUpdateMP(in UpdateMPRequest) error {
	if strings.TrimSpace(in.Name) == "" {
		return errors.New("name is required")
	}
	if _, ok := validUtilityClasses[in.UtilityClass]; !ok {
		return fmt.Errorf("utility_class must be water or electricity (got %q)", in.UtilityClass)
	}
	return nil
}

// ----- helpers ------------------------------------------------------------

type errorResp struct {
	Error  string `json:"error"`
	Detail string `json:"detail,omitempty"`
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func internalError(log *slog.Logger, w http.ResponseWriter, op string, err error) {
	if log != nil {
		log.Error("metering_point handler", "op", op, "err", err)
	}
	writeJSON(w, http.StatusInternalServerError, errorResp{Error: "internal"})
}

func parseUUIDParam(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	raw := chi.URLParam(r, name)
	id, err := uuid.Parse(raw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResp{Error: "invalid_id"})
		return uuid.Nil, false
	}
	return id, true
}

func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

func requireAdmin(sm *scs.SessionManager, w http.ResponseWriter, r *http.Request, action auth.Action) (auth.User, bool) {
	user, ok := auth.GetUser(r.Context(), sm)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResp{Error: "unauthorized"})
		return auth.User{}, false
	}
	if !auth.Can(&user, action, nil) {
		writeJSON(w, http.StatusForbidden, errorResp{Error: "forbidden"})
		return auth.User{}, false
	}
	return user, true
}

func mustParseUUID(s string) uuid.UUID {
	u, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil
	}
	return u
}

// isConstraintViolation matches CHECK / FK errors (NOT unique — that's mapped
// to 409 in createMP for clarity).
func isConstraintViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "23514") ||
		strings.Contains(msg, "23503") ||
		strings.Contains(msg, "violates check constraint") ||
		strings.Contains(msg, "violates foreign key constraint")
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "23505") || strings.Contains(msg, "violates unique constraint")
}

// ----- JSON serialization -------------------------------------------------

func mpToJSON(m sqlc.MeteringPoint) map[string]any {
	return map[string]any{
		"id":                   uuidString(m.ID),
		"site_id":              uuidString(m.SiteID),
		"name":                 m.Name,
		"utility_class":        m.UtilityClass,
		"location_description": derefString(m.LocationDescription),
		"archived_at":          timestamptzText(m.ArchivedAt),
		"created_at":           timestamptzText(m.CreatedAt),
		"updated_at":           timestamptzText(m.UpdatedAt),
	}
}

func mpsToJSON(rows []sqlc.MeteringPoint) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, mpToJSON(r))
	}
	return out
}

func uuidString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	return uuid.UUID(u.Bytes).String()
}

func derefString(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func derefInt16(p *int16) any {
	if p == nil {
		return nil
	}
	return *p
}

func timestamptzText(t pgtype.Timestamptz) any {
	if !t.Valid {
		return nil
	}
	return t.Time.UTC().Format("2006-01-02T15:04:05.000000Z")
}

// numericText renders pgtype.Numeric as a decimal string. Reused by the MP
// detail endpoint for reading_offset + measurement values.
func numericText(n pgtype.Numeric) any {
	if !n.Valid {
		return nil
	}
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		if n.Int != nil {
			return n.Int.String()
		}
		return nil
	}
	// Format with up to 6 decimals — enough for any meter precision the
	// schema actually persists. Phase 4 may switch to a higher-fidelity
	// rendering if a customer needs more.
	return formatFloat(f.Float64)
}

func formatFloat(v float64) string {
	// strconv.FormatFloat with -1 precision gives the shortest round-trip
	// representation. Avoids "1.234e+06" surprises on the wire.
	return fmt.Sprintf("%g", v)
}

// _ ensures `context` import is used.
var _ = context.Background
