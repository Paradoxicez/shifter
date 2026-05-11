package meteringpoint

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// SignalPoint is one hourly bucket in the signal-history response.
type SignalPoint struct {
	Bucket     string   `json:"bucket"`
	BatteryPct *float64 `json:"battery_pct"`
	Rssi       *float64 `json:"rssi"`
	Snr        *float64 `json:"snr"`
}

// SignalHistoryResponse is the wire shape for
// GET /api/metering-points/{id}/signal-history.
//
// D-17: fixed 24-hour window, hourly buckets. Always returns the last 24h
// regardless of query params. Frontend uses this for the battery/RSSI/SNR
// sparklines on the Normal tab of the MP detail page.
type SignalHistoryResponse struct {
	WindowStart           string        `json:"window_start"`
	WindowEnd             string        `json:"window_end"`
	BucketIntervalSeconds int           `json:"bucket_interval_seconds"`
	Series                []SignalPoint `json:"series"`
}

// handleSignalHistory serves GET /api/metering-points/{id}/signal-history.
//
// D-17: always last 24h, hourly buckets. No query params accepted.
// T-04-05-01: uuid-parse `:id`; 400 on parse fail.
func (deps Deps) handleSignalHistory(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	now := time.Now().UTC()
	windowEnd := now
	windowStart := now.Add(-24 * time.Hour)
	bucketDur := time.Hour

	bucketInterval := pgtype.Interval{
		Microseconds: int64(bucketDur / time.Microsecond),
		Valid:        true,
	}

	ctx := r.Context()
	queries := sqlc.New(deps.Pool)
	rows, err := queries.MeteringPointTimeseries(ctx, sqlc.MeteringPointTimeseriesParams{
		BucketInterval:  bucketInterval,
		MeteringPointID: pgtype.UUID{Bytes: id, Valid: true},
		StartTime:       pgtype.Timestamptz{Time: windowStart, Valid: true},
		EndTime:         pgtype.Timestamptz{Time: windowEnd, Valid: true},
	})
	if err != nil {
		internalError(deps.Log, w, "signal-history: query", err)
		return
	}

	series := make([]SignalPoint, 0, len(rows))
	for _, row := range rows {
		var bucketStr string
		switch bt := row.Bucket.(type) {
		case time.Time:
			bucketStr = bt.UTC().Format(time.RFC3339)
		default:
			bucketStr = ""
		}
		pt := SignalPoint{Bucket: bucketStr}
		if f := numericToFloat64Ptr(row.BatteryAvg); f != nil {
			pt.BatteryPct = f
		}
		if f := numericToFloat64Ptr(row.RssiAvg); f != nil {
			pt.Rssi = f
		}
		if f := numericToFloat64Ptr(row.SnrAvg); f != nil {
			pt.Snr = f
		}
		series = append(series, pt)
	}

	resp := SignalHistoryResponse{
		WindowStart:           windowStart.Format(time.RFC3339),
		WindowEnd:             windowEnd.Format(time.RFC3339),
		BucketIntervalSeconds: int(bucketDur.Seconds()),
		Series:                series,
	}
	writeJSON(w, http.StatusOK, resp)
}
