package report

import (
	"bytes"
	"fmt"
	"time"

	"github.com/xuri/excelize/v2"
)

// WriteExcel returns an in-memory .xlsx workbook with 3 sheets per UI-SPEC:
//
//	Summary       — period range + totals + optional delta tiles
//	Period Detail — per-period breakdown with formatted Excel dates
//	Meter Detail  — per-MP rows when scope = all or site (empty for scope=meter)
//
// excelize/v2 v2.10.0 is already in go.mod (Phase 3 bulk import).
// RESEARCH §Don't Hand-Roll: custom xlsx ZIP is 200+ LoC of manifest math;
// excelize handles it with a clean API.
func WriteExcel(rpt *ReportData, identity InstallIdentity) ([]byte, error) {
	tz := identity.Timezone
	if tz == nil {
		tz = time.UTC
	}

	f := excelize.NewFile()
	defer f.Close()

	// Rename the default "Sheet1" to "Summary".
	if err := f.SetSheetName("Sheet1", "Summary"); err != nil {
		return nil, fmt.Errorf("rename sheet to Summary: %w", err)
	}
	if _, err := f.NewSheet("Period Detail"); err != nil {
		return nil, fmt.Errorf("new sheet Period Detail: %w", err)
	}
	if _, err := f.NewSheet("Meter Detail"); err != nil {
		return nil, fmt.Errorf("new sheet Meter Detail: %w", err)
	}

	// ---- SUMMARY sheet ----
	f.SetCellValue("Summary", "A1", "Shifter consumption report")
	f.SetCellValue("Summary", "A2", identity.DisplayName)
	f.SetCellValue("Summary", "A3", "Generated")
	f.SetCellValue("Summary", "B3", time.Now().In(tz))
	f.SetCellValue("Summary", "C3", tz.String())
	f.SetCellValue("Summary", "A5", "Period range")
	f.SetCellValue("Summary", "B5", rpt.Config.Start.In(tz))
	f.SetCellValue("Summary", "C5", rpt.Config.End.In(tz))
	f.SetCellValue("Summary", "A7", "Total consumption")
	f.SetCellValue("Summary", "B7", rpt.Summary.TotalConsumption)
	if rpt.Summary.PriorDelta != nil {
		f.SetCellValue("Summary", "A8", "Δ vs prior period")
		f.SetCellValue("Summary", "B8", rpt.Summary.PriorDelta.Percent/100) // store as ratio
	}
	if rpt.Summary.YoYDelta != nil {
		f.SetCellValue("Summary", "A9", "Δ vs same period last year")
		f.SetCellValue("Summary", "B9", rpt.Summary.YoYDelta.Percent/100)
	}

	// Apply Excel date numFmt to B3, B5, C5 (numFmt 22 = m/d/yyyy h:mm).
	dateStyle, err := f.NewStyle(&excelize.Style{NumFmt: 22})
	if err != nil {
		return nil, fmt.Errorf("new date style: %w", err)
	}
	f.SetCellStyle("Summary", "B3", "B3", dateStyle)
	f.SetCellStyle("Summary", "B5", "C5", dateStyle)

	// ---- PERIOD DETAIL sheet ----
	ul := primaryUnitLabel(rpt, identity.Units)
	f.SetCellValue("Period Detail", "A1", "Period")
	f.SetCellValue("Period Detail", "B1", fmt.Sprintf("Consumption (%s)", ul))
	f.SetCellValue("Period Detail", "C1", "Δ vs prior")
	f.SetCellValue("Period Detail", "D1", "Δ vs YoY")

	// Date style for the period column.
	periodDateStyle, err := f.NewStyle(&excelize.Style{NumFmt: 14}) // numFmt 14 = m/d/yyyy
	if err != nil {
		return nil, fmt.Errorf("new period date style: %w", err)
	}
	// Percent style for delta columns.
	pctStyle, err := f.NewStyle(&excelize.Style{NumFmt: 10}) // numFmt 10 = 0.00%
	if err != nil {
		return nil, fmt.Errorf("new pct style: %w", err)
	}

	for i, row := range rpt.PeriodRows {
		rowIdx := i + 2
		cellA := fmt.Sprintf("A%d", rowIdx)
		cellB := fmt.Sprintf("B%d", rowIdx)
		cellC := fmt.Sprintf("C%d", rowIdx)
		cellD := fmt.Sprintf("D%d", rowIdx)

		f.SetCellValue("Period Detail", cellA, row.Period.In(tz))
		f.SetCellStyle("Period Detail", cellA, cellA, periodDateStyle)

		f.SetCellValue("Period Detail", cellB, row.Consumption)

		if row.DeltaVsPrior != nil {
			f.SetCellValue("Period Detail", cellC, row.DeltaVsPrior.Percent/100)
			f.SetCellStyle("Period Detail", cellC, cellC, pctStyle)
		}
		if row.DeltaVsYoY != nil {
			f.SetCellValue("Period Detail", cellD, row.DeltaVsYoY.Percent/100)
			f.SetCellStyle("Period Detail", cellD, cellD, pctStyle)
		}
	}

	// Totals row (bold + top border) at the end of Period Detail.
	totalRow := len(rpt.PeriodRows) + 2
	totalStyle, err := f.NewStyle(&excelize.Style{
		Font:   &excelize.Font{Bold: true},
		Border: []excelize.Border{{Type: "top", Color: "000000", Style: 1}},
	})
	if err != nil {
		return nil, fmt.Errorf("new total style: %w", err)
	}
	f.SetCellValue("Period Detail", fmt.Sprintf("A%d", totalRow), "Total")
	f.SetCellFormula("Period Detail", fmt.Sprintf("B%d", totalRow),
		fmt.Sprintf("SUM(B2:B%d)", totalRow-1))
	f.SetCellStyle("Period Detail", fmt.Sprintf("A%d", totalRow), fmt.Sprintf("D%d", totalRow), totalStyle)

	// ---- METER DETAIL sheet ----
	if rpt.Config.Scope != "meter" {
		f.SetCellValue("Meter Detail", "A1", "Meter")
		f.SetCellValue("Meter Detail", "B1", "Site")
		f.SetCellValue("Meter Detail", "C1", "Utility")
		f.SetCellValue("Meter Detail", "D1", fmt.Sprintf("Total (%s)", ul))
		f.SetCellValue("Meter Detail", "E1", "Δ vs prior")

		for i, m := range rpt.MeterRows {
			rowIdx := i + 2
			f.SetCellValue("Meter Detail", fmt.Sprintf("A%d", rowIdx), m.Name)
			f.SetCellValue("Meter Detail", fmt.Sprintf("B%d", rowIdx), m.SiteName)
			f.SetCellValue("Meter Detail", fmt.Sprintf("C%d", rowIdx), m.UtilityClass)
			f.SetCellValue("Meter Detail", fmt.Sprintf("D%d", rowIdx), m.Consumption)
			if m.DeltaVsPrior != nil {
				f.SetCellValue("Meter Detail", fmt.Sprintf("E%d", rowIdx), m.DeltaVsPrior.Percent/100)
			}
		}
	}

	// Set Summary as the active sheet (index 0).
	f.SetActiveSheet(0)

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, fmt.Errorf("write xlsx: %w", err)
	}
	return buf.Bytes(), nil
}

// primaryUnitLabel returns the consumption unit for the report's primary utility class.
// Falls back to "m³" (water metric) when utility class is unset or mixed.
func primaryUnitLabel(rpt *ReportData, units string) string {
	// Determine dominant utility class from period rows.
	classCounts := map[string]int{}
	for _, r := range rpt.PeriodRows {
		if r.UtilityClass != "" {
			classCounts[r.UtilityClass]++
		}
	}
	// If only electricity rows, use electricity label.
	_, hasElec := classCounts["electricity"]
	_, hasWater := classCounts["water"]
	if hasElec && !hasWater {
		return unitLabel("electricity", units)
	}
	// Capabilities-driven fallback.
	if rpt.Config.Capabilities == "electricity" {
		return unitLabel("electricity", units)
	}
	return unitLabel("water", units)
}
