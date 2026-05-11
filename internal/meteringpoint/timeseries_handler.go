package meteringpoint

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/shifter-io/shifter/internal/dashboard"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// TimeseriesPoint is one bucket in the per-MP timeseries response.
type TimeseriesPoint struct {
	Bucket          string   `json:"bucket"`
	CumulativeAvg   *float64 `json:"cumulative_avg"`
	InstantAvg      *float64 `json:"instant_avg"`
	BatteryAvg      *float64 `json:"battery_avg"`
	RssiAvg         *float64 `json:"rssi_avg"`
	SnrAvg          *float64 `json:"snr_avg"`
}

// MPTimeseriesResponse is the wire shape for
// GET /api/metering-points/{id}/timeseries.
type MPTimeseriesResponse struct {
	BucketIntervalSeconds int               `json:"bucket_interval_seconds"`
	Series                []TimeseriesPoint `json:"series"`
}

// handleTimeseries serves GET /api/metering-points/{id}/timeseries.
//
// Query params:
//   - range    required  today|24h|7d|30d|custom
//   - start    required when range=custom  RFC 3339
//   - end      required when range=custom  RFC 3339
//
// Reuses dashboard.BucketIntervalForRange for bucket-interval logic (D-12).
// T-04-05-01: uuid-parse `:id`; 400 on parse fail.
func (deps Deps) handleTimeseries(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}
	q := r.URL.Query()
	rangeStr := q.Get("range")

	now := time.Now().UTC()
	var start, end time.Time

	switch rangeStr {
	case "today":
		start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		end = now
	case "24h":
		start = now.Add(-24 * time.Hour)
		end = now
	case "7d":
		start = now.Add(-7 * 24 * time.Hour)
		end = now
	case "30d":
		start = now.Add(-30 * 24 * time.Hour)
		end = now
	case "custom":
		startStr := q.Get("start")
		endStr := q.Get("end")
		if startStr == "" || endStr == "" {
			http.Error(w, "custom range requires start and end parameters", http.StatusBadRequest)
			return
		}
		var parseErr error
		start, parseErr = time.Parse(time.RFC3339, startStr)
		if parseErr != nil {
			http.Error(w, "invalid start: must be RFC 3339", http.StatusBadRequest)
			return
		}
		end, parseErr = time.Parse(time.RFC3339, endStr)
		if parseErr != nil {
			http.Error(w, "invalid end: must be RFC 3339", http.StatusBadRequest)
			return
		}
	default:
		http.Error(w, `range must be one of today|24h|7d|30d|custom`, http.StatusBadRequest)
		return
	}

	bucketDur, err := dashboard.BucketIntervalForRange(rangeStr, start, end)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	bucketInterval := pgtype.Interval{
		Microseconds: int64(bucketDur / time.Microsecond),
		Valid:        true,
	}

	ctx := r.Context()
	queries := sqlc.New(deps.Pool)
	rows, dbErr := queries.MeteringPointTimeseries(ctx, sqlc.MeteringPointTimeseriesParams{
		BucketInterval:  bucketInterval,
		MeteringPointID: pgtype.UUID{Bytes: id, Valid: true},
		StartTime:       pgtype.Timestamptz{Time: start.UTC(), Valid: true},
		EndTime:         pgtype.Timestamptz{Time: end.UTC(), Valid: true},
	})
	if dbErr != nil {
		internalError(deps.Log, w, "timeseries: query", dbErr)
		return
	}

	series := make([]TimeseriesPoint, 0, len(rows))
	for _, row := range rows {
		var bucketStr string
		switch bt := row.Bucket.(type) {
		case time.Time:
			bucketStr = bt.UTC().Format(time.RFC3339)
		default:
			bucketStr = ""
		}
		pt := TimeseriesPoint{Bucket: bucketStr}
		if f := numericToFloat64Ptr(row.CumulativeAvg); f != nil {
			pt.CumulativeAvg = f
		}
		if f := numericToFloat64Ptr(row.InstantAvg); f != nil {
			pt.InstantAvg = f
		}
		if f := numericToFloat64Ptr(row.BatteryAvg); f != nil {
			pt.BatteryAvg = f
		}
		if f := numericToFloat64Ptr(row.RssiAvg); f != nil {
			pt.RssiAvg = f
		}
		if f := numericToFloat64Ptr(row.SnrAvg); f != nil {
			pt.SnrAvg = f
		}
		series = append(series, pt)
	}

	resp := MPTimeseriesResponse{
		BucketIntervalSeconds: int(bucketDur.Seconds()),
		Series:                series,
	}
	writeJSON(w, http.StatusOK, resp)
}

// numericToFloat64Ptr converts a pgtype.Numeric to *float64.
// Returns nil for invalid or zero-valid numerics.
func numericToFloat64Ptr(n pgtype.Numeric) *float64 {
	if !n.Valid {
		return nil
	}
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return nil
	}
	v := f.Float64
	return &v
}
