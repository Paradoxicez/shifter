package report

import (
	"encoding/csv"
	"fmt"
	"io"
	"time"
)

// InstallIdentity holds the install-level branding and locale information
// needed for CSV and Excel report headers. Sourced from install_identity table
// (Phase 1 D-06 / Phase 5 context).
type InstallIdentity struct {
	DisplayName string
	Address     string
	Timezone    *time.Location // install_identity.timezone (D-07)
	Units       string         // "metric" | "imperial" — drives consumption unit label
	LogoPath    string         // used only by PDF writer (plan 05-06)
}

// WriteCSV emits a UTF-8 BOM, a 3-row metadata header, then per-period detail
// rows to the given writer. REPT-03 contract:
//
//   - UTF-8 BOM (\xEF\xBB\xBF) — Excel auto-detects encoding when the BOM is
//     present; without it Excel falls back to system locale and mangles non-ASCII
//     characters in metering-point names.
//   - Comma separator.
//   - ISO-8601 timestamps formatted in install_identity.timezone (D-07).
//   - Timezone label printed in the metadata header so the customer knows the
//     interpretation when the CSV is opened without metadata context.
//   - Empty string for nil DeltaVsYoY (D-03 silent fallback — no "null" literal).
func WriteCSV(w io.Writer, rpt *ReportData, identity InstallIdentity) error {
	tz := identity.Timezone
	if tz == nil {
		tz = time.UTC
	}

	// 1) UTF-8 BOM as raw bytes — MUST be the first bytes emitted, before
	//    the csv.Writer touches the stream.
	if _, err := w.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return fmt.Errorf("csv bom: %w", err)
	}

	cw := csv.NewWriter(w)
	defer cw.Flush()

	// 2) 3-row metadata header block.
	nowISO := time.Now().In(tz).Format(time.RFC3339)
	if err := cw.Write([]string{"Shifter consumption report", identity.DisplayName}); err != nil {
		return fmt.Errorf("csv meta row 1: %w", err)
	}
	if err := cw.Write([]string{"Generated", nowISO, tz.String()}); err != nil {
		return fmt.Errorf("csv meta row 2: %w", err)
	}
	if err := cw.Write([]string{"Scope", describeScope(rpt.Config), "Range", rpt.Config.RangeKind}); err != nil {
		return fmt.Errorf("csv meta row 3: %w", err)
	}
	// Blank separator row (row 4) separates metadata block from data.
	if err := cw.Write([]string{}); err != nil {
		return fmt.Errorf("csv blank row: %w", err)
	}

	// 3) Data header row (row 5).
	if err := cw.Write([]string{"Period", "Consumption", "Unit", "Δ vs prior", "Δ vs YoY"}); err != nil {
		return fmt.Errorf("csv header row: %w", err)
	}

	// 4) Detail rows (starting at row 6).
	for i, row := range rpt.PeriodRows {
		rec := []string{
			row.Period.In(tz).Format(time.RFC3339),
			fmt.Sprintf("%.3f", row.Consumption),
			unitLabel(row.UtilityClass, identity.Units),
			deltaString(row.DeltaVsPrior),
			deltaString(row.DeltaVsYoY), // empty string when nil (D-03 silent fallback)
		}
		if err := cw.Write(rec); err != nil {
			return fmt.Errorf("csv row %d: %w", i, err)
		}
	}
	return nil
}

// deltaString formats a DeltaResult as "+12.3%" / "-8.1%", or empty string
// when the pointer is nil (D-03 silent fallback — never writes "null").
func deltaString(d *DeltaResult) string {
	if d == nil {
		return ""
	}
	return fmt.Sprintf("%+.1f%%", d.Percent)
}

// describeScope returns a human-readable scope description for the metadata header.
func describeScope(cfg ReportConfig) string {
	switch cfg.Scope {
	case "site":
		return fmt.Sprintf("Site: %s", cfg.SiteID)
	case "meter":
		return fmt.Sprintf("Meter: %s", cfg.MeteringPointID)
	default:
		return "All meters"
	}
}

// unitLabel returns the consumption unit label for a given utility class and unit system.
//
//	water + metric   → "m³"
//	water + imperial → "L"   (litres — common in imperial-locale water billing)
//	electricity + any → "kWh"
func unitLabel(utilityClass, units string) string {
	switch utilityClass {
	case "electricity":
		if units == "imperial" {
			return "Wh"
		}
		return "kWh"
	default: // "water" and anything unknown
		if units == "imperial" {
			return "L"
		}
		return "m³"
	}
}
