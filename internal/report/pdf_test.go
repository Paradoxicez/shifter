package report

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPDFBranding(t *testing.T) {
	bangkok, _ := time.LoadLocation("Asia/Bangkok")
	rpt := &ReportData{
		Config:  ReportConfig{Scope: "meter", RangeKind: "daily", Start: time.Now(), End: time.Now(), Timezone: bangkok},
		Summary: Summary{TotalConsumption: 123.456, PriorDelta: &DeltaResult{Percent: 5}, YoYDelta: nil},
		PeriodRows: []PeriodRow{{Period: time.Now(), Consumption: 50}},
	}
	identity := InstallIdentity{DisplayName: "Acme Co", Address: "123 Main St", Timezone: bangkok}

	raw, err := WritePDF(rpt, identity)
	require.NoError(t, err)
	require.NotEmpty(t, raw)

	// 1) Magic bytes
	require.Equal(t, []byte("%PDF-"), raw[:5], "must be a PDF")

	// 2) Content contains display name + address
	rawStr := string(raw)
	require.Contains(t, rawStr, "Acme Co", "header must include display_name")
	require.Contains(t, rawStr, "123 Main St", "header must include address")
	require.Contains(t, rawStr, "Generated", "footer must include Generated label")
	require.Contains(t, rawStr, "Asia/Bangkok", "footer must include install timezone")
	require.Contains(t, rawStr, "page 1 of 1", "single-page report must show 'page 1 of 1' (M = total page count)")

	// 3) No literal unsubstituted braces — maroto's WithPageNumber overlay
	//    substitutes {current} and {total} at render time.
	require.NotContains(t, rawStr, "{number}", "page-number placeholder must be substituted")
	require.NotContains(t, rawStr, "{total}", "page-total placeholder must be substituted")
}

func TestPDFBranding_NoLogo_StillRenders(t *testing.T) {
	identity := InstallIdentity{DisplayName: "Acme", Timezone: time.UTC} // LogoPath empty
	_, err := WritePDF(&ReportData{Config: ReportConfig{Scope: "all", Timezone: time.UTC}}, identity)
	require.NoError(t, err)
}

func TestPDFBranding_MultiPage(t *testing.T) {
	bangkok, _ := time.LoadLocation("Asia/Bangkok")
	rows := make([]PeriodRow, 100)
	for i := range rows {
		rows[i] = PeriodRow{Period: time.Now().Add(time.Duration(-i) * 24 * time.Hour), Consumption: float64(i)}
	}
	raw, err := WritePDF(&ReportData{Config: ReportConfig{Scope: "meter", Timezone: bangkok}, PeriodRows: rows}, InstallIdentity{DisplayName: "Acme", Timezone: bangkok})
	require.NoError(t, err)
	rawStr := string(raw)

	// Multi-page detection: maroto's SequentialLowMemoryMode (used when >50 rows)
	// writes "/Type/Page" (no space); normal mode writes "/Type /Page". Count both.
	pageCount := strings.Count(rawStr, "/Type /Page\n") +
		strings.Count(rawStr, "/Type /Page>>") +
		strings.Count(rawStr, "/Type/Page")
	require.GreaterOrEqual(t, pageCount, 2, "100 rows should span 2+ pages")

	// maroto's WithPageNumber overlay writes "page N of M" on every page.
	// Both page 1 and page 2 footers must appear in the raw bytes.
	require.Contains(t, rawStr, "page 1 of", "page 1 footer must show 'page 1 of M'")
	require.Contains(t, rawStr, "page 2 of", "page 2 footer must show 'page 2 of M'")
}
