package meteringpoint

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// validQualityValues is the whitelist for the quality query param (T-04-05-03).
var validQualityValues = map[string]struct{}{
	"ok":                 {},
	"decode_fail":        {},
	"missing_canonical":  {},
	"out_of_range":       {},
	"duplicate_fcnt":     {},
}

const (
	defaultUplinkLimit = 100
	maxUplinkLimit     = 500
)

// UplinkRow is one item in the uplinks log response.
type UplinkRow struct {
	Time            string          `json:"time"`
	CumulativeValue any             `json:"cumulative_value"`
	InstantValue    any             `json:"instant_value"`
	BatteryPct      any             `json:"battery_pct"`
	Rssi            any             `json:"rssi"`
	Snr             any             `json:"snr"`
	Fcnt            any             `json:"fcnt"`
	Quality         string          `json:"quality"`
	RawPayloadHex   string          `json:"raw_payload_hex"`
	DecodedObject   json.RawMessage `json:"decoded_object"`
}

// UplinkListResponse is the wire shape for GET /api/metering-points/{id}/uplinks.
type UplinkListResponse struct {
	Uplinks    []UplinkRow `json:"uplinks"`
	HasMore    bool        `json:"has_more"`
	NextBefore *string     `json:"next_before"`
}

// handleUplinks serves GET /api/metering-points/{id}/uplinks.
//
// Query params:
//   - limit  default 100, max 500 (D-16); explicit values > 500 → 400
//   - before RFC3339 cursor on time DESC (optional)
//   - quality comma-separated whitelist filter (optional)
//
// T-04-05-01: uuid-parse `:id`; 400 on parse fail.
// T-04-05-02: hard-cap limit ≤ 500; 400 if explicitly exceeded.
// T-04-05-03: quality whitelist; 400 on invalid value.
func (deps Deps) handleUplinks(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	q := r.URL.Query()

	// Parse limit (T-04-05-02).
	limit := defaultUplinkLimit
	if limitStr := q.Get("limit"); limitStr != "" {
		n, err := strconv.Atoi(limitStr)
		if err != nil || n < 1 || n > maxUplinkLimit {
			writeJSON(w, http.StatusBadRequest, errorResp{
				Error:  "invalid_limit",
				Detail: "limit must be between 1 and 500",
			})
			return
		}
		limit = n
	}

	// Parse cursor (optional).
	var beforeTs pgtype.Timestamptz
	if beforeStr := q.Get("before"); beforeStr != "" {
		t, err := time.Parse(time.RFC3339, beforeStr)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResp{
				Error:  "invalid_before",
				Detail: "before must be RFC 3339 (e.g. 2026-05-11T12:00:00Z)",
			})
			return
		}
		beforeTs = pgtype.Timestamptz{Time: t.UTC(), Valid: true}
	}

	// Parse quality filter (T-04-05-03).
	var qualityFilter []string
	if qStr := q.Get("quality"); qStr != "" {
		parts := strings.Split(qStr, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if _, ok := validQualityValues[p]; !ok {
				writeJSON(w, http.StatusBadRequest, errorResp{
					Error:  "invalid_quality",
					Detail: "quality must be one of: ok, decode_fail, missing_canonical, out_of_range, duplicate_fcnt",
				})
				return
			}
			qualityFilter = append(qualityFilter, p)
		}
	}

	ctx := r.Context()
	queries := sqlc.New(deps.Pool)

	// Fetch limit+1 rows to determine has_more without an extra COUNT query.
	// We fetch one extra row and then slice to limit.
	fetchLimit := int32(limit + 1)
	rows, err := queries.ListUplinksByMP(ctx, sqlc.ListUplinksByMPParams{
		MeteringPointID: pgtype.UUID{Bytes: id, Valid: true},
		Limit:           fetchLimit,
		Column3:         beforeTs,
		Column4:         qualityFilter,
	})
	if err != nil {
		internalError(deps.Log, w, "list uplinks", err)
		return
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}

	uplinks := make([]UplinkRow, 0, len(rows))
	for _, row := range rows {
		decodedObj := json.RawMessage(row.DecodedObject)
		if len(decodedObj) == 0 {
			decodedObj = json.RawMessage(`{}`)
		}
		snr := any(nil)
		if row.Snr != nil {
			snr = *row.Snr
		}
		fcnt := any(nil)
		if row.Fcnt != nil {
			fcnt = *row.Fcnt
		}
		uplinks = append(uplinks, UplinkRow{
			Time:            row.Time.Time.UTC().Format(time.RFC3339),
			CumulativeValue: numericText(row.CumulativeValue),
			InstantValue:    numericText(row.InstantValue),
			BatteryPct:      derefInt16(row.BatteryPct),
			Rssi:            derefInt16(row.Rssi),
			Snr:             snr,
			Fcnt:            fcnt,
			Quality:         row.Quality,
			RawPayloadHex:   bytesToHex(row.RawPayload),
			DecodedObject:   decodedObj,
		})
	}

	resp := UplinkListResponse{
		Uplinks: uplinks,
		HasMore: hasMore,
	}
	if hasMore && len(rows) > 0 {
		oldest := rows[len(rows)-1].Time.Time.UTC().Format(time.RFC3339)
		resp.NextBefore = &oldest
	}

	writeJSON(w, http.StatusOK, resp)
}
