package dashboard

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// TimeseriesPoint is one bucket in the timeseries response.
type TimeseriesPoint struct {
	Bucket          string  `json:"bucket"` // ISO 8601 UTC
	CumulativeDelta float64 `json:"cumulative_delta"`
}

// TimeseriesResponse is the wire shape for GET /api/dashboard/timeseries.
type TimeseriesResponse struct {
	Utility               string            `json:"utility"`
	BucketIntervalSeconds int               `json:"bucket_interval_seconds"`
	Series                []TimeseriesPoint `json:"series"`
}

// handleTimeseries serves GET /api/dashboard/timeseries.
//
// Query parameters:
//   - range    required  today|24h|7d|30d|custom
//   - utility  required  water|electricity
//   - start    required when range=custom  RFC 3339
//   - end      required when range=custom  RFC 3339
//
// T-04-04-01: range is enum-validated; invalid values return 400.
// T-04-04-03: custom range is bounded to 365 days; returns 400 if exceeded.
// D-12: bucket interval is selected by BucketIntervalForRange.
func (d Deps) handleTimeseries(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()

	rangeStr := q.Get("range")
	utility := q.Get("utility")

	// Validate utility (T-04-04-01).
	if utility != "water" && utility != "electricity" {
		http.Error(w, `utility must be "water" or "electricity"`, http.StatusBadRequest)
		return
	}

	// Compute start/end from range.
	now := time.Now().UTC()
	var start, end time.Time

	switch rangeStr {
	case "today":
		// Today from midnight UTC to now.
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
			http.Error(w, "invalid start: must be RFC 3339 (e.g. 2026-05-11T00:00:00Z)", http.StatusBadRequest)
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

	// Validate and select bucket interval (T-04-04-01, T-04-04-03, D-12).
	bucketDur, err := BucketIntervalForRange(rangeStr, start, end)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Build pgtype.Interval from time.Duration.
	// pgtype.Interval stores microseconds in Microseconds field.
	bucketInterval := pgtype.Interval{
		Microseconds: int64(bucketDur / time.Microsecond),
		Valid:        true,
	}

	queries := sqlc.New(d.Pool)
	rows, dbErr := queries.DashboardTimeseries(ctx, sqlc.DashboardTimeseriesParams{
		UtilityClass:   utility,
		StartTime:      pgtype.Timestamptz{Time: start.UTC(), Valid: true},
		EndTime:        pgtype.Timestamptz{Time: end.UTC(), Valid: true},
		BucketInterval: bucketInterval,
	})
	if dbErr != nil {
		d.Logger.ErrorContext(ctx, "timeseries: query", "err", dbErr)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	series := make([]TimeseriesPoint, 0, len(rows))
	for _, row := range rows {
		// Bucket is interface{} from sqlc (time_bucket return type is opaque).
		// We expect a time.Time underneath.
		var bucketStr string
		switch bt := row.Bucket.(type) {
		case time.Time:
			bucketStr = bt.UTC().Format(time.RFC3339)
		default:
			// Fallback: use string representation.
			bucketStr = ""
		}
		point := TimeseriesPoint{
			Bucket:          bucketStr,
			CumulativeDelta: numericToFloat64(row.CumulativeDelta),
		}
		series = append(series, point)
	}

	resp := TimeseriesResponse{
		Utility:               utility,
		BucketIntervalSeconds: int(bucketDur.Seconds()),
		Series:                series,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		d.Logger.ErrorContext(ctx, "timeseries: encode response", "err", err)
	}
}
