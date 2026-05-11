package report

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// ReportConfig is the validated request payload.
type ReportConfig struct {
	Scope           string    // "all" | "site" | "meter"
	SiteID          uuid.UUID // when Scope == "site"
	MeteringPointID uuid.UUID // when Scope == "meter"
	GroupBy         string    // "site" | "category" | "none"
	RangeKind       string    // "daily" | "monthly" | "yearly" | "custom"
	Start           time.Time
	End             time.Time
	Capabilities    string         // install_identity.capabilities (water|electricity|both)
	Timezone        *time.Location // for output formatting; defaults to UTC
}

// ReportData is the assembled in-memory result handed to CSV / Excel / PDF writers.
type ReportData struct {
	Config     ReportConfig
	Summary    Summary
	PeriodRows []PeriodRow
	MeterRows  []MeterRow
}

// Summary holds aggregate totals and optional deltas for the report header.
type Summary struct {
	TotalConsumption float64
	PriorDelta       *DeltaResult       // nil when no prior period in data
	YoYDelta         *DeltaResult       // nil = silent fallback (D-03)
	PerCategory      map[string]float64 // utility_class → total (only populated when GroupBy = "category")
}

// PeriodRow is one row of the period-detail table (one per time bucket).
type PeriodRow struct {
	Period       time.Time
	Consumption  float64
	DeltaVsPrior *DeltaResult
	DeltaVsYoY  *DeltaResult
	SiteID       *uuid.UUID // populated when GroupBy = "site"
	SiteName     string
	UtilityClass string // populated when GroupBy = "category"
}

// MeterRow is one row of the meter-detail table.
type MeterRow struct {
	MeteringPointID uuid.UUID
	Name            string
	UtilityClass    string
	SiteName        string
	Consumption     float64
	DeltaVsPrior    *DeltaResult
}

// BuildReport routes to the right CAGG-backed query and returns the assembled struct.
// Side-effect-free — does NOT INSERT the report row (the handler does that in a tx).
func BuildReport(ctx context.Context, q *sqlc.Queries, cfg ReportConfig) (*ReportData, error) {
	if cfg.Timezone == nil {
		cfg.Timezone = time.UTC
	}

	rpt := &ReportData{Config: cfg}

	// 1) Period rows from the CAGG matching RangeKind + scope/group combination.
	if err := rpt.loadPeriodRows(ctx, q); err != nil {
		return nil, fmt.Errorf("load period rows: %w", err)
	}

	// 2) Meter rows for scope=all or scope=site.
	if cfg.Scope != "meter" {
		if err := rpt.loadMeterRows(ctx, q); err != nil {
			return nil, fmt.Errorf("load meter rows: %w", err)
		}
	}

	// 3) Compute summary (total + prior delta + YoY).
	rpt.computeSummary()

	// 4) Compute per-period prior deltas.
	rpt.computeRowDeltas()

	return rpt, nil
}

// loadPeriodRows populates rpt.PeriodRows based on (RangeKind, Scope, GroupBy).
// Dispatches to one of the 9 CAGG query functions.
func (rpt *ReportData) loadPeriodRows(ctx context.Context, q *sqlc.Queries) error {
	cfg := rpt.Config
	start := pgtype.Timestamptz{Time: cfg.Start, Valid: true}
	end := pgtype.Timestamptz{Time: cfg.End, Valid: true}

	// Determine effective group: if scope=meter or scope=site, group is "none"
	// (per-MP rows). If scope=all, respect cfg.GroupBy.
	effectiveGroup := cfg.GroupBy
	if cfg.Scope == "meter" {
		effectiveGroup = "mp"
	} else if cfg.Scope == "site" && effectiveGroup == "" {
		effectiveGroup = "none"
	}
	if effectiveGroup == "" {
		effectiveGroup = "none"
	}

	switch cfg.RangeKind {
	case "daily":
		return rpt.loadDailyRows(ctx, q, start, end, effectiveGroup)
	case "monthly":
		return rpt.loadMonthlyRows(ctx, q, start, end, effectiveGroup)
	case "yearly":
		return rpt.loadYearlyRows(ctx, q, start, end, effectiveGroup)
	case "custom":
		// Route based on span.
		span := cfg.End.Sub(cfg.Start)
		if span <= 30*24*time.Hour {
			return rpt.loadDailyRows(ctx, q, start, end, effectiveGroup)
		} else if span <= 365*24*time.Hour {
			return rpt.loadMonthlyRows(ctx, q, start, end, effectiveGroup)
		}
		return rpt.loadYearlyRows(ctx, q, start, end, effectiveGroup)
	default:
		return fmt.Errorf("unknown range_kind %q", cfg.RangeKind)
	}
}

func (rpt *ReportData) loadDailyRows(ctx context.Context, q *sqlc.Queries, start, end pgtype.Timestamptz, group string) error {
	cfg := rpt.Config
	switch {
	case cfg.Scope == "meter":
		mpID := pgtype.UUID{Bytes: cfg.MeteringPointID, Valid: cfg.MeteringPointID != uuid.Nil}
		rows, err := q.ReportDailyByMP(ctx, sqlc.ReportDailyByMPParams{
			MeteringPointID: mpID,
			Bucket:          start,
			Bucket_2:        end,
		})
		if err != nil {
			return fmt.Errorf("ReportDailyByMP: %w", err)
		}
		for _, r := range rows {
			rpt.PeriodRows = append(rpt.PeriodRows, PeriodRow{
				Period:      timeFromInterface(r.Period),
				Consumption: float64(r.Consumption),
			})
		}
	case group == "site":
		siteUUID := toNullableUUID(cfg.SiteID)
		rows, err := q.ReportDailyBySite(ctx, sqlc.ReportDailyBySiteParams{
			Bucket:  start,
			Bucket_2: end,
			Column3: siteUUID,
		})
		if err != nil {
			return fmt.Errorf("ReportDailyBySite: %w", err)
		}
		for _, r := range rows {
			sid := uuidFromPgtype(r.SiteID)
			rpt.PeriodRows = append(rpt.PeriodRows, PeriodRow{
				Period:      timeFromInterface(r.Period),
				Consumption: float64(r.Consumption),
				SiteID:      &sid,
				SiteName:    r.SiteName,
			})
		}
	case group == "category":
		rows, err := q.ReportDailyByCategory(ctx, sqlc.ReportDailyByCategoryParams{
			Bucket:  start,
			Bucket_2: end,
		})
		if err != nil {
			return fmt.Errorf("ReportDailyByCategory: %w", err)
		}
		for _, r := range rows {
			if !rpt.capabilityMatches(r.UtilityClass) {
				continue
			}
			rpt.PeriodRows = append(rpt.PeriodRows, PeriodRow{
				Period:       timeFromInterface(r.Period),
				Consumption:  float64(r.Consumption),
				UtilityClass: r.UtilityClass,
			})
		}
	default:
		// scope=all + group=none → fleet-wide, scope=site → per-site total
		siteUUID := toNullableUUID(cfg.SiteID)
		rows, err := q.ReportDailyBySite(ctx, sqlc.ReportDailyBySiteParams{
			Bucket:  start,
			Bucket_2: end,
			Column3: siteUUID,
		})
		if err != nil {
			return fmt.Errorf("ReportDailyBySite(none): %w", err)
		}
		// For group=none collapse across sites (fleet total per period).
		periodMap := map[time.Time]float64{}
		for _, r := range rows {
			t := timeFromInterface(r.Period)
			periodMap[t] += float64(r.Consumption)
		}
		for t, c := range periodMap {
			rpt.PeriodRows = append(rpt.PeriodRows, PeriodRow{
				Period:      t,
				Consumption: c,
			})
		}
		sortPeriodRows(rpt.PeriodRows)
	}
	return nil
}

func (rpt *ReportData) loadMonthlyRows(ctx context.Context, q *sqlc.Queries, start, end pgtype.Timestamptz, group string) error {
	cfg := rpt.Config
	switch {
	case cfg.Scope == "meter":
		mpID := pgtype.UUID{Bytes: cfg.MeteringPointID, Valid: cfg.MeteringPointID != uuid.Nil}
		rows, err := q.ReportMonthlyByMP(ctx, sqlc.ReportMonthlyByMPParams{
			MeteringPointID: mpID,
			Bucket:          start,
			Bucket_2:        end,
		})
		if err != nil {
			return fmt.Errorf("ReportMonthlyByMP: %w", err)
		}
		for _, r := range rows {
			rpt.PeriodRows = append(rpt.PeriodRows, PeriodRow{
				Period:      timeFromInterface(r.Period),
				Consumption: float64(r.Consumption),
			})
		}
	case group == "site":
		siteUUID := toNullableUUID(cfg.SiteID)
		rows, err := q.ReportMonthlyBySite(ctx, sqlc.ReportMonthlyBySiteParams{
			Bucket:  start,
			Bucket_2: end,
			Column3: siteUUID,
		})
		if err != nil {
			return fmt.Errorf("ReportMonthlyBySite: %w", err)
		}
		for _, r := range rows {
			sid := uuidFromPgtype(r.SiteID)
			rpt.PeriodRows = append(rpt.PeriodRows, PeriodRow{
				Period:      timeFromInterface(r.Period),
				Consumption: float64(r.Consumption),
				SiteID:      &sid,
				SiteName:    r.SiteName,
			})
		}
	case group == "category":
		rows, err := q.ReportMonthlyByCategory(ctx, sqlc.ReportMonthlyByCategoryParams{
			Bucket:  start,
			Bucket_2: end,
		})
		if err != nil {
			return fmt.Errorf("ReportMonthlyByCategory: %w", err)
		}
		for _, r := range rows {
			if !rpt.capabilityMatches(r.UtilityClass) {
				continue
			}
			rpt.PeriodRows = append(rpt.PeriodRows, PeriodRow{
				Period:       timeFromInterface(r.Period),
				Consumption:  float64(r.Consumption),
				UtilityClass: r.UtilityClass,
			})
		}
	default:
		siteUUID := toNullableUUID(cfg.SiteID)
		rows, err := q.ReportMonthlyBySite(ctx, sqlc.ReportMonthlyBySiteParams{
			Bucket:  start,
			Bucket_2: end,
			Column3: siteUUID,
		})
		if err != nil {
			return fmt.Errorf("ReportMonthlyBySite(none): %w", err)
		}
		periodMap := map[time.Time]float64{}
		for _, r := range rows {
			t := timeFromInterface(r.Period)
			periodMap[t] += float64(r.Consumption)
		}
		for t, c := range periodMap {
			rpt.PeriodRows = append(rpt.PeriodRows, PeriodRow{
				Period:      t,
				Consumption: c,
			})
		}
		sortPeriodRows(rpt.PeriodRows)
	}
	return nil
}

func (rpt *ReportData) loadYearlyRows(ctx context.Context, q *sqlc.Queries, start, end pgtype.Timestamptz, group string) error {
	cfg := rpt.Config
	switch {
	case cfg.Scope == "meter":
		mpID := pgtype.UUID{Bytes: cfg.MeteringPointID, Valid: cfg.MeteringPointID != uuid.Nil}
		rows, err := q.ReportYearlyByMP(ctx, sqlc.ReportYearlyByMPParams{
			MeteringPointID: mpID,
			Bucket:          start,
			Bucket_2:        end,
		})
		if err != nil {
			return fmt.Errorf("ReportYearlyByMP: %w", err)
		}
		for _, r := range rows {
			rpt.PeriodRows = append(rpt.PeriodRows, PeriodRow{
				Period:      timeFromInterface(r.Period),
				Consumption: float64(r.Consumption),
			})
		}
	case group == "site":
		siteUUID := toNullableUUID(cfg.SiteID)
		rows, err := q.ReportYearlyBySite(ctx, sqlc.ReportYearlyBySiteParams{
			Bucket:  start,
			Bucket_2: end,
			Column3: siteUUID,
		})
		if err != nil {
			return fmt.Errorf("ReportYearlyBySite: %w", err)
		}
		for _, r := range rows {
			sid := uuidFromPgtype(r.SiteID)
			rpt.PeriodRows = append(rpt.PeriodRows, PeriodRow{
				Period:      timeFromInterface(r.Period),
				Consumption: float64(r.Consumption),
				SiteID:      &sid,
				SiteName:    r.SiteName,
			})
		}
	case group == "category":
		rows, err := q.ReportYearlyByCategory(ctx, sqlc.ReportYearlyByCategoryParams{
			Bucket:  start,
			Bucket_2: end,
		})
		if err != nil {
			return fmt.Errorf("ReportYearlyByCategory: %w", err)
		}
		for _, r := range rows {
			if !rpt.capabilityMatches(r.UtilityClass) {
				continue
			}
			rpt.PeriodRows = append(rpt.PeriodRows, PeriodRow{
				Period:       timeFromInterface(r.Period),
				Consumption:  float64(r.Consumption),
				UtilityClass: r.UtilityClass,
			})
		}
	default:
		siteUUID := toNullableUUID(cfg.SiteID)
		rows, err := q.ReportYearlyBySite(ctx, sqlc.ReportYearlyBySiteParams{
			Bucket:  start,
			Bucket_2: end,
			Column3: siteUUID,
		})
		if err != nil {
			return fmt.Errorf("ReportYearlyBySite(none): %w", err)
		}
		periodMap := map[time.Time]float64{}
		for _, r := range rows {
			t := timeFromInterface(r.Period)
			periodMap[t] += float64(r.Consumption)
		}
		for t, c := range periodMap {
			rpt.PeriodRows = append(rpt.PeriodRows, PeriodRow{
				Period:      t,
				Consumption: c,
			})
		}
		sortPeriodRows(rpt.PeriodRows)
	}
	return nil
}

// loadMeterRows fetches per-MP detail for scope=all or scope=site.
func (rpt *ReportData) loadMeterRows(ctx context.Context, q *sqlc.Queries) error {
	cfg := rpt.Config
	params := sqlc.ListMetersInScopeParams{
		Column1: cfg.Scope,
		SiteID:  toNullableUUID(cfg.SiteID),
		ID:      pgtype.UUID{Bytes: cfg.MeteringPointID, Valid: cfg.MeteringPointID != uuid.Nil},
	}
	rows, err := q.ListMetersInScope(ctx, params)
	if err != nil {
		return fmt.Errorf("ListMetersInScope: %w", err)
	}
	for _, r := range rows {
		if !rpt.capabilityMatches(r.UtilityClass) {
			continue
		}
		// Compute per-MP consumption by summing all period rows for this MP.
		// (In a more complete implementation, we'd query per-MP range totals.)
		// For now, MeterRow.Consumption is left at 0; the Period rows contain
		// the consumption data. Plan 05-06 can extend this with per-MP totals.
		rpt.MeterRows = append(rpt.MeterRows, MeterRow{
			MeteringPointID: uuidFromPgtype(r.ID),
			Name:            r.Name,
			UtilityClass:    r.UtilityClass,
			SiteName:        r.SiteName,
		})
	}
	return nil
}

// computeSummary aggregates all period rows into the Summary.
func (rpt *ReportData) computeSummary() {
	var total float64
	byCat := map[string]float64{}
	for _, r := range rpt.PeriodRows {
		total += r.Consumption
		if r.UtilityClass != "" {
			byCat[r.UtilityClass] += r.Consumption
		}
	}
	rpt.Summary.TotalConsumption = total
	if len(byCat) > 0 {
		rpt.Summary.PerCategory = byCat
	}
	// PriorDelta and YoYDelta are computed by the handler using a prior-period
	// query; for the in-memory assembly we leave them nil (silent fallback D-03).
	// The handler can call ComputeDelta / ComputeYoY after getting prior data.
}

// computeRowDeltas fills DeltaVsPrior for each period row using the previous
// row's consumption as the prior (since rows are ordered by period ascending).
func (rpt *ReportData) computeRowDeltas() {
	for i := range rpt.PeriodRows {
		if i == 0 {
			rpt.PeriodRows[i].DeltaVsPrior = nil // first row has no prior
			continue
		}
		prior := rpt.PeriodRows[i-1].Consumption
		rpt.PeriodRows[i].DeltaVsPrior = ComputeDelta(rpt.PeriodRows[i].Consumption, &prior)
	}
	// YoYDelta left nil (D-03 silent fallback). Plan 05-06 can enhance
	// by running a prior-year query and calling ComputeYoY per row.
}

// capabilityMatches returns true when the given utility_class is enabled for
// this install's capabilities (D-09 / D-04 single-capability filter).
func (rpt *ReportData) capabilityMatches(utilityClass string) bool {
	switch rpt.Config.Capabilities {
	case "water":
		return utilityClass == "water"
	case "electricity":
		return utilityClass == "electricity"
	default: // "both" or empty
		return true
	}
}

// --- helpers ---

// timeFromInterface coerces the time_bucket() return value to time.Time.
// TimescaleDB time_bucket returns a timestamptz, which pgx decodes as
// pgtype.Timestamptz when scanned into interface{}.
func timeFromInterface(v interface{}) time.Time {
	switch t := v.(type) {
	case time.Time:
		return t
	case pgtype.Timestamptz:
		if t.Valid {
			return t.Time
		}
	}
	return time.Time{}
}

// toNullableUUID converts a uuid.UUID to pgtype.UUID, marking it invalid if nil.
func toNullableUUID(id uuid.UUID) pgtype.UUID {
	if id == uuid.Nil {
		return pgtype.UUID{Valid: false}
	}
	return pgtype.UUID{Bytes: id, Valid: true}
}

// uuidFromPgtype converts a pgtype.UUID to uuid.UUID.
func uuidFromPgtype(u pgtype.UUID) uuid.UUID {
	if !u.Valid {
		return uuid.Nil
	}
	return uuid.UUID(u.Bytes)
}

// sortPeriodRows sorts period rows by ascending period time.
func sortPeriodRows(rows []PeriodRow) {
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rows[j].Period.Before(rows[j-1].Period); j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
}
