package report

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
)

func TestExcelFormat(t *testing.T) {
	bangkok, err := time.LoadLocation("Asia/Bangkok")
	require.NoError(t, err)

	rpt := &ReportData{
		Config: ReportConfig{
			Scope:        "all",
			RangeKind:    "monthly",
			Start:        time.Now().AddDate(0, -1, 0),
			End:          time.Now(),
			Capabilities: "water",
			Timezone:     bangkok,
		},
		Summary: Summary{
			TotalConsumption: 123.456,
			PriorDelta:       &DeltaResult{Percent: 12.3},
		},
		PeriodRows: []PeriodRow{
			{
				Period:       time.Now().AddDate(0, 0, -1).In(bangkok),
				Consumption:  50,
				UtilityClass: "water",
				DeltaVsPrior: &DeltaResult{Percent: 5},
			},
		},
		MeterRows: []MeterRow{
			{Name: "MP-1", SiteName: "HQ", UtilityClass: "water", Consumption: 50},
		},
	}

	raw, err := WriteExcel(rpt, InstallIdentity{DisplayName: "Acme", Timezone: bangkok, Units: "metric"})
	require.NoError(t, err)
	require.NotEmpty(t, raw)

	f, err := excelize.OpenReader(bytes.NewReader(raw))
	require.NoError(t, err)
	defer f.Close()

	// Test 1: Exactly 3 sheets named "Summary", "Period Detail", "Meter Detail".
	sheets := f.GetSheetList()
	require.Equal(t, []string{"Summary", "Period Detail", "Meter Detail"}, sheets)

	// Test 2: Period column on "Period Detail" has a date style applied (not string).
	styleID, err := f.GetCellStyle("Period Detail", "A2")
	require.NoError(t, err)
	require.NotZero(t, styleID, "Period column A2 must have a date style applied (not default style 0)")

	// Test 3: Column header includes unit label in parentheses.
	header, err := f.GetCellValue("Period Detail", "B1")
	require.NoError(t, err)
	require.Contains(t, header, "(m³)", "header must contain unit in parentheses")

	// Test 4: Last row of "Period Detail" is the bold/border totals row.
	totalRowIdx := len(rpt.PeriodRows) + 2
	cellA, err := f.GetCellValue("Period Detail", fmt.Sprintf("A%d", totalRowIdx))
	require.NoError(t, err)
	require.Equal(t, "Total", cellA, "last row must be labelled 'Total'")

	// The totals row must have a style (bold + top border) applied — style ID != 0.
	totalStyle, err := f.GetCellStyle("Period Detail", fmt.Sprintf("B%d", totalRowIdx))
	require.NoError(t, err)
	require.NotZero(t, totalStyle, "totals row must have bold+border style (not default style 0)")

	// Test 5: Summary sheet has the display name.
	displayName, err := f.GetCellValue("Summary", "A2")
	require.NoError(t, err)
	require.Equal(t, "Acme", displayName)

	// Test 6: Meter Detail sheet has the meter row.
	meterName, err := f.GetCellValue("Meter Detail", "A2")
	require.NoError(t, err)
	require.Equal(t, "MP-1", meterName)
}
