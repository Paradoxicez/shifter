// Package api — compare HTTP handler (Plan 07-12).
//
// compare_handler.go exposes:
//
//	POST /api/reports/compare — Surface 5 discriminated-union compare endpoint
//
// Security:
//   - ActionReportRead guards the endpoint (admin + viewer; read-only aggregation).
//   - UUIDs validated before any DB call (T-07-12-02 mitigation).
//   - Range capped at 5 years, reversed-range returns 400 (T-07-12-02 mitigation).
//   - CAGG queries are O(days) not O(rows) (T-07-12-01 mitigation).
package api

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shifter-io/shifter/internal/auth"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// CompareDeps bundles the dependencies for the compare HTTP handler.
type CompareDeps struct {
	Pool       *pgxpool.Pool
	SessionMgr *scs.SessionManager
}

// RegisterCompareRoutes mounts POST /api/reports/compare with RBAC middleware.
// ActionReportRead is granted to both admin and viewer (read-only aggregation).
func RegisterCompareRoutes(r chi.Router, deps CompareDeps) {
	r.Group(func(rt chi.Router) {
		rt.Use(auth.RequireAction(deps.SessionMgr, auth.ActionReportRead))
		rt.Post("/api/reports/compare", compareHandler(deps))
	})
}

// maxCompareRangeYears caps how far back a single compare request can reach.
// T-07-12-01: CAGG queries are O(days) so 5 years × 365 rows is trivially fast.
const maxCompareRangeYears = 5

// ---- request / response shapes ----------------------------------------------

type compareTimeRange struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type compareRequest struct {
	Mode       string            `json:"mode"`        // "entities" | "time_ranges"
	EntityType string            `json:"entity_type"` // "site" | "metering_point"
	EntityAID  *string           `json:"entity_a_id,omitempty"`
	EntityBID  *string           `json:"entity_b_id,omitempty"`
	EntityID   *string           `json:"entity_id,omitempty"` // time_ranges mode
	Range      *compareTimeRange `json:"range,omitempty"`
	RangeA     *compareTimeRange `json:"range_a,omitempty"`
	RangeB     *compareTimeRange `json:"range_b,omitempty"`
}

type compareSeriesPoint struct {
	Bucket string  `json:"bucket"`
	Value  float64 `json:"value"`
}

// SeriesResult mirrors the API contract's SeriesResult type.
type SeriesResult struct {
	Label   string               `json:"label"`
	Series  []compareSeriesPoint `json:"series"`
	Total   float64              `json:"total"`
	Peak    float64              `json:"peak"`
	Average float64              `json:"average"`
	Unit    string               `json:"unit"` // always "m3" for water, "kWh" for electricity
}

type compareDeltaResult struct {
	Total   float64 `json:"total"`
	Peak    float64 `json:"peak"`
	Average float64 `json:"average"`
}

// compareResponse is the JSON envelope for POST /api/reports/compare.
type compareResponse struct {
	A     SeriesResult       `json:"a"`
	B     SeriesResult       `json:"b"`
	Delta compareDeltaResult `json:"delta"`
}

// ---- handler ----------------------------------------------------------------

func compareHandler(deps CompareDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req compareRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request body", http.StatusBadRequest)
			return
		}

		ctx := r.Context()
		q := sqlc.New(deps.Pool)

		switch req.Mode {
		case "entities":
			resp, code, msg := handleEntitiesMode(ctx, q, req)
			if code != 0 {
				http.Error(w, msg, code)
				return
			}
			writeCompareJSON(w, resp)

		case "time_ranges":
			resp, code, msg := handleTimeRangesMode(ctx, q, req)
			if code != 0 {
				http.Error(w, msg, code)
				return
			}
			writeCompareJSON(w, resp)

		default:
			http.Error(w, "mode must be 'entities' or 'time_ranges'", http.StatusBadRequest)
		}
	}
}

// handleEntitiesMode handles mode="entities": two entity IDs, one time range.
func handleEntitiesMode(ctx context.Context, q *sqlc.Queries, req compareRequest) (compareResponse, int, string) {
	// Validate entity IDs.
	if req.EntityAID == nil || req.EntityBID == nil {
		return compareResponse{}, http.StatusBadRequest, "entity_a_id and entity_b_id are required for mode=entities"
	}
	aID, err := uuid.Parse(*req.EntityAID)
	if err != nil {
		return compareResponse{}, http.StatusBadRequest, "entity_a_id is not a valid UUID"
	}
	bID, err := uuid.Parse(*req.EntityBID)
	if err != nil {
		return compareResponse{}, http.StatusBadRequest, "entity_b_id is not a valid UUID"
	}

	// Validate time range.
	if req.Range == nil {
		return compareResponse{}, http.StatusBadRequest, "range is required for mode=entities"
	}
	from, to, errCode, errMsg := parseAndValidateRange(req.Range.From, req.Range.To)
	if errCode != 0 {
		return compareResponse{}, errCode, errMsg
	}

	// Determine entity label using the entity type.
	aLabel := labelForEntity(ctx, q, req.EntityType, aID)
	bLabel := labelForEntity(ctx, q, req.EntityType, bID)

	// Run queries.
	aRows, err := runCompareQuery(ctx, q, req.EntityType, aID, from, to)
	if err != nil {
		return compareResponse{}, http.StatusInternalServerError, "query failed for entity A"
	}
	bRows, err := runCompareQuery(ctx, q, req.EntityType, bID, from, to)
	if err != nil {
		return compareResponse{}, http.StatusInternalServerError, "query failed for entity B"
	}

	aResult := buildSeriesResult(aRows, aLabel, req.EntityType)
	bResult := buildSeriesResult(bRows, bLabel, req.EntityType)
	delta := computeDelta(aResult, bResult)

	return compareResponse{A: aResult, B: bResult, Delta: delta}, 0, ""
}

// handleTimeRangesMode handles mode="time_ranges": one entity ID, two time ranges.
func handleTimeRangesMode(ctx context.Context, q *sqlc.Queries, req compareRequest) (compareResponse, int, string) {
	if req.EntityID == nil {
		return compareResponse{}, http.StatusBadRequest, "entity_id is required for mode=time_ranges"
	}
	entityID, err := uuid.Parse(*req.EntityID)
	if err != nil {
		return compareResponse{}, http.StatusBadRequest, "entity_id is not a valid UUID"
	}

	if req.RangeA == nil || req.RangeB == nil {
		return compareResponse{}, http.StatusBadRequest, "range_a and range_b are required for mode=time_ranges"
	}
	fromA, toA, errCode, errMsg := parseAndValidateRange(req.RangeA.From, req.RangeA.To)
	if errCode != 0 {
		return compareResponse{}, errCode, errMsg
	}
	fromB, toB, errCode, errMsg := parseAndValidateRange(req.RangeB.From, req.RangeB.To)
	if errCode != 0 {
		return compareResponse{}, errCode, errMsg
	}

	label := labelForEntity(ctx, q, req.EntityType, entityID)

	aRows, err := runCompareQuery(ctx, q, req.EntityType, entityID, fromA, toA)
	if err != nil {
		return compareResponse{}, http.StatusInternalServerError, "query failed for range A"
	}
	bRows, err := runCompareQuery(ctx, q, req.EntityType, entityID, fromB, toB)
	if err != nil {
		return compareResponse{}, http.StatusInternalServerError, "query failed for range B"
	}

	aResult := buildSeriesResult(aRows, label, req.EntityType)
	bResult := buildSeriesResult(bRows, label, req.EntityType)
	delta := computeDelta(aResult, bResult)

	return compareResponse{A: aResult, B: bResult, Delta: delta}, 0, ""
}

// ---- helpers ----------------------------------------------------------------

// parseAndValidateRange parses ISO-8601 from/to strings and validates the range.
// Returns (from, to, errCode, errMsg). errCode==0 means success.
func parseAndValidateRange(fromStr, toStr string) (time.Time, time.Time, int, string) {
	from, err := time.Parse(time.RFC3339, fromStr)
	if err != nil {
		return time.Time{}, time.Time{}, http.StatusBadRequest, "range.from is not valid RFC-3339"
	}
	to, err := time.Parse(time.RFC3339, toStr)
	if err != nil {
		return time.Time{}, time.Time{}, http.StatusBadRequest, "range.to is not valid RFC-3339"
	}
	// T-07-12-02: reject reversed range.
	if !to.After(from) {
		return time.Time{}, time.Time{}, http.StatusBadRequest, "range.to must be after range.from"
	}
	// T-07-12-01: cap at 5 years.
	if to.Sub(from) > time.Duration(maxCompareRangeYears)*365*24*time.Hour {
		return time.Time{}, time.Time{}, http.StatusBadRequest, "range span exceeds 5-year maximum"
	}
	return from, to, 0, ""
}

// labelForEntity resolves a human-readable name for the entity by querying
// the DB. Falls back to "entityType:uuid" if the entity is not found or the
// DB call fails.
func labelForEntity(ctx context.Context, q *sqlc.Queries, entityType string, id uuid.UUID) string {
	pgID := pgtype.UUID{Bytes: id, Valid: true}
	switch entityType {
	case "site":
		if site, err := q.GetSite(ctx, pgID); err == nil {
			return site.Name
		}
	default: // "metering_point"
		if mp, err := q.GetMP(ctx, pgID); err == nil {
			return mp.Name
		}
	}
	// Fallback: UUID string (entity not found or DB error)
	return entityType + ":" + id.String()
}

// compareRow is a normalised row from either CAGG query.
type compareRow struct {
	Bucket time.Time
	Value  float64
}

// runCompareQuery dispatches to CompareSiteDaily or CompareMeteringPointDaily.
func runCompareQuery(ctx context.Context, q *sqlc.Queries, entityType string, id uuid.UUID, from, to time.Time) ([]compareRow, error) {
	pgID := pgtype.UUID{Bytes: id, Valid: true}
	pgFrom := pgtype.Timestamptz{Time: from, Valid: true}
	pgTo := pgtype.Timestamptz{Time: to, Valid: true}

	switch entityType {
	case "site":
		rows, err := q.CompareSiteDaily(ctx, sqlc.CompareSiteDailyParams{
			SiteID:   pgID,
			Bucket:   pgFrom,
			Bucket_2: pgTo,
		})
		if err != nil {
			return nil, err
		}
		out := make([]compareRow, len(rows))
		for i, r := range rows {
			t := pgDateToTime(r.Bucket)
			out[i] = compareRow{Bucket: t, Value: float64(r.Value)}
		}
		return out, nil

	default: // "metering_point" (and fallback for unknown types treated as MP)
		rows, err := q.CompareMeteringPointDaily(ctx, sqlc.CompareMeteringPointDailyParams{
			MeteringPointID: pgID,
			Bucket:          pgFrom,
			Bucket_2:        pgTo,
		})
		if err != nil {
			return nil, err
		}
		out := make([]compareRow, len(rows))
		for i, r := range rows {
			t := pgDateToTime(r.Bucket)
			out[i] = compareRow{Bucket: t, Value: float64(r.Value)}
		}
		return out, nil
	}
}

// pgDateToTime converts a pgtype.Date to a time.Time (midnight UTC).
func pgDateToTime(d pgtype.Date) time.Time {
	if !d.Valid {
		return time.Time{}
	}
	return time.Date(d.Time.Year(), d.Time.Month(), d.Time.Day(), 0, 0, 0, 0, time.UTC)
}

// buildSeriesResult computes the series + totals + peak + average from raw rows.
func buildSeriesResult(rows []compareRow, label, entityType string) SeriesResult {
	series := make([]compareSeriesPoint, 0, len(rows))
	var total, peak float64
	for _, r := range rows {
		series = append(series, compareSeriesPoint{
			Bucket: r.Bucket.Format("2006-01-02"),
			Value:  r.Value,
		})
		total += r.Value
		if r.Value > peak {
			peak = r.Value
		}
	}
	avg := 0.0
	if len(rows) > 0 {
		avg = total / float64(len(rows))
	}
	unit := "m3"
	if entityType == "electricity" {
		unit = "kWh"
	}
	return SeriesResult{
		Label:   label,
		Series:  series,
		Total:   total,
		Peak:    peak,
		Average: avg,
		Unit:    unit,
	}
}

// computeDelta returns delta totals between series A and B.
func computeDelta(a, b SeriesResult) compareDeltaResult {
	return compareDeltaResult{
		Total:   math.Round((a.Total-b.Total)*1000) / 1000,
		Peak:    math.Round((a.Peak-b.Peak)*1000) / 1000,
		Average: math.Round((a.Average-b.Average)*1000) / 1000,
	}
}

// writeCompareJSON writes the compare response as JSON.
func writeCompareJSON(w http.ResponseWriter, resp compareResponse) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
