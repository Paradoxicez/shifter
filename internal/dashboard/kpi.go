package dashboard

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// UtilityKPI holds the KPI fields for a single utility class (water or
// electricity). All fields use JSON tags matching the Plan 07 contract exactly.
// PeriodDeltaAbs and PeriodDeltaPct are nullable: they are nil when yesterday
// window had no data (install <24 h old — D-08).
type UtilityKPI struct {
	TodayConsumption float64  `json:"today_consumption"`
	TodayUnit        string   `json:"today_unit"`
	InstantTotal     float64  `json:"instant_total"`
	InstantUnit      string   `json:"instant_unit"`
	PeriodDeltaAbs   *float64 `json:"period_delta_abs"` // nullable
	PeriodDeltaPct   *float64 `json:"period_delta_pct"` // nullable
	OnlineCount      int64    `json:"online_count"`
	TotalCount       int64    `json:"total_count"`
}

// LatestReading is the per-MP latest reading shape returned in the snapshot
// response (Plan 07 contract).
type LatestReading struct {
	MeteringPointID string   `json:"metering_point_id"`
	UtilityClass    string   `json:"utility_class"`
	Time            *string  `json:"time"`            // ISO 8601, nullable (no readings yet)
	CumulativeValue *float64 `json:"cumulative_value"` // nullable
	InstantValue    *float64 `json:"instant_value"`   // nullable
	Quality         string   `json:"quality"`
	BatteryPct      *int16   `json:"battery_pct"` // nullable
	Rssi            *int16   `json:"rssi"`         // nullable
}

// Snapshot is the wire shape for GET /api/dashboard/snapshot.
// KPIs only contains entries for the active utilities (no null keys — the
// frontend uses "water" in kpis to decide what to render, per D-09).
type Snapshot struct {
	Capabilities   string               `json:"capabilities"`
	GeneratedAt    time.Time            `json:"generated_at"`
	KPIs           map[string]UtilityKPI `json:"kpis"`
	LatestReadings []LatestReading      `json:"latest_readings"`
}

// utilitiesFor returns the list of utility_class values active for the given
// capability setting.
func utilitiesFor(capabilities string) []string {
	switch capabilities {
	case "water":
		return []string{"water"}
	case "electricity":
		return []string{"electricity"}
	default: // "both"
		return []string{"water", "electricity"}
	}
}

// unitLabel returns the today_unit and instant_unit strings for a utility class.
// water → m³ / L/min; electricity → kWh / W.
func unitLabel(utilityClass string) (todayUnit, instantUnit string) {
	if utilityClass == "electricity" {
		return "kWh", "W"
	}
	return "m³", "L/min"
}

// numericToFloat64 safely converts a pgtype.Numeric to float64, returning 0
// for invalid/NaN values.
func numericToFloat64(n pgtype.Numeric) float64 {
	if !n.Valid {
		return 0
	}
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return 0
	}
	return f.Float64
}

// BuildSnapshot fetches all KPI data for the given capabilities and returns the
// full Snapshot struct. Queries run sequentially (one per utility per KPI type)
// using the shared queries handle. On any DB error the function returns the
// partial result accumulated so far plus the error — callers surface 500.
//
// D-05 timezone is passed verbatim from install_identity.timezone to each query.
// D-09 capability gating: only active utility keys appear in the returned KPIs map.
func BuildSnapshot(ctx context.Context, q *sqlc.Queries, timezone, capabilities string) (Snapshot, error) {
	utilities := utilitiesFor(capabilities)

	kpis := make(map[string]UtilityKPI, len(utilities))
	for _, u := range utilities {
		todayUnit, instantUnit := unitLabel(u)
		kpi := UtilityKPI{
			TodayUnit:   todayUnit,
			InstantUnit: instantUnit,
		}

		// D-05: today's consumption.
		todayDelta, err := q.TodayConsumptionByUtility(ctx, sqlc.TodayConsumptionByUtilityParams{
			Timezone:     timezone,
			UtilityClass: u,
		})
		if err != nil {
			return Snapshot{}, fmt.Errorf("today consumption (%s): %w", u, err)
		}
		kpi.TodayConsumption = numericToFloat64(todayDelta)

		// D-06: instantaneous total.
		instantTotal, err := q.CurrentInstantSumByUtility(ctx, u)
		if err != nil {
			return Snapshot{}, fmt.Errorf("instant total (%s): %w", u, err)
		}
		kpi.InstantTotal = numericToFloat64(instantTotal)

		// D-08: period delta (today vs yesterday).
		delta, err := q.PeriodDeltaByUtility(ctx, sqlc.PeriodDeltaByUtilityParams{
			Timezone:     timezone,
			UtilityClass: u,
		})
		if err != nil {
			return Snapshot{}, fmt.Errorf("period delta (%s): %w", u, err)
		}
		todayD := numericToFloat64(delta.TodayD)
		yesterdayD := numericToFloat64(delta.YesterdayD)
		if delta.YesterdayD.Valid && yesterdayD != 0 {
			abs := todayD - yesterdayD
			pct := abs / yesterdayD * 100
			kpi.PeriodDeltaAbs = &abs
			kpi.PeriodDeltaPct = &pct
		}
		// else: both remain nil — install <24h old, no yesterday data.

		// D-07: online/offline count.
		onlineRow, err := q.DeviceOnlineCount(ctx, u)
		if err != nil {
			return Snapshot{}, fmt.Errorf("device online count (%s): %w", u, err)
		}
		kpi.OnlineCount = onlineRow.OnlineCount
		kpi.TotalCount = onlineRow.TotalCount

		kpis[u] = kpi
	}

	// Latest readings: fetch for all active utilities in one query.
	rows, err := q.DashboardLatestReadings(ctx, utilities)
	if err != nil {
		return Snapshot{}, fmt.Errorf("latest readings: %w", err)
	}
	readings := make([]LatestReading, 0, len(rows))
	for _, row := range rows {
		lr := LatestReading{
			MeteringPointID: uuidString(row.MeteringPointID),
			UtilityClass:    row.UtilityClass,
			Quality:         row.Quality,
			BatteryPct:      row.BatteryPct,
			Rssi:            row.Rssi,
		}
		if row.Time.Valid {
			ts := row.Time.Time.UTC().Format(time.RFC3339)
			lr.Time = &ts
		}
		if row.CumulativeValue.Valid {
			f, ferr := row.CumulativeValue.Float64Value()
			if ferr == nil && f.Valid {
				lr.CumulativeValue = &f.Float64
			}
		}
		if row.InstantValue.Valid {
			f, ferr := row.InstantValue.Float64Value()
			if ferr == nil && f.Valid {
				lr.InstantValue = &f.Float64
			}
		}
		readings = append(readings, lr)
	}

	return Snapshot{
		Capabilities:   capabilities,
		GeneratedAt:    time.Now().UTC(),
		KPIs:           kpis,
		LatestReadings: readings,
	}, nil
}

// uuidString converts a pgtype.UUID to its canonical hyphenated string form.
// Returns empty string if the UUID is not valid.
func uuidString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// BucketIntervalForRange returns the time.Duration bucket interval per the D-12
// bucket schedule. For "custom", start and end must already be validated (start
// < end, span ≤ 365 days) — the function returns an error if they are not.
//
// D-12 schedule:
//
//	| range        | bucket_interval |
//	|--------------|-----------------|
//	| today        | 5 minutes       |
//	| 24h          | 5 minutes       |
//	| 7d           | 1 hour          |
//	| 30d          | 4 hours         |
//	| custom ≤ 30d | 1 hour          |
//	| custom > 30d | 1 day           |
func BucketIntervalForRange(rangeStr string, start, end time.Time) (time.Duration, error) {
	switch rangeStr {
	case "today", "24h":
		return 5 * time.Minute, nil
	case "7d":
		return time.Hour, nil
	case "30d":
		return 4 * time.Hour, nil
	case "custom":
		span := end.Sub(start)
		if span <= 0 {
			return 0, errors.New("custom range requires start < end")
		}
		if span > 365*24*time.Hour {
			return 0, errors.New("custom range exceeds 1 year maximum")
		}
		if span <= 30*24*time.Hour {
			return time.Hour, nil
		}
		return 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("invalid range %q: must be one of today|24h|7d|30d|custom", rangeStr)
	}
}
