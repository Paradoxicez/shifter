package report

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCSVFormat(t *testing.T) {
	buf := &bytes.Buffer{}
	bangkok, err := time.LoadLocation("Asia/Bangkok")
	require.NoError(t, err)

	rpt := &ReportData{
		Config: ReportConfig{
			Scope:     "meter",
			RangeKind: "daily",
			Start:     time.Now(),
			End:       time.Now(),
			Timezone:  bangkok,
		},
		PeriodRows: []PeriodRow{
			{
				Period:       time.Date(2026, 5, 1, 0, 0, 0, 0, bangkok),
				Consumption:  1.234,
				UtilityClass: "water",
				DeltaVsPrior: &DeltaResult{Percent: 12.3},
				DeltaVsYoY:  nil, // silent fallback (D-03)
			},
			{
				Period:       time.Date(2026, 5, 2, 0, 0, 0, 0, bangkok),
				Consumption:  1.345,
				UtilityClass: "water",
				DeltaVsPrior: &DeltaResult{Percent: -3.1},
				DeltaVsYoY:  &DeltaResult{Percent: 8.0},
			},
		},
	}
	identity := InstallIdentity{DisplayName: "Acme", Timezone: bangkok, Units: "metric"}

	require.NoError(t, WriteCSV(buf, rpt, identity))

	raw := buf.Bytes()

	// Test 1: First three bytes are UTF-8 BOM.
	require.Equal(t, []byte{0xEF, 0xBB, 0xBF}, raw[:3], "must start with UTF-8 BOM")

	// Parse via csv.Reader (after consuming BOM so the CSV parser is clean).
	// FieldsPerRecord=-1 allows variable field counts per row (blank separator row
	// has 0 fields; metadata rows have 2–4 fields; data rows have 5 fields).
	r := csv.NewReader(bytes.NewReader(raw[3:]))
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	require.NoError(t, err)
	// csv.Reader silently skips blank lines (the separator row produced by
	// cw.Write([]string{}) writes a bare "\n" which the reader drops).
	// Expect: 3 metadata rows + 1 data header + 2 data rows = 6 rows.
	require.GreaterOrEqual(t, len(rows), 6, "metadata 3 + data header + 2 data rows")

	// Test 2: Metadata block — rows 0..2.
	require.Equal(t, "Shifter consumption report", rows[0][0])
	require.Equal(t, "Acme", rows[0][1])
	require.Equal(t, "Generated", rows[1][0])
	// ISO-8601 with timezone offset (not "Z") because Bangkok is +07:00.
	require.Regexp(t, `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\+\d{2}:\d{2}$`, rows[1][1], "ISO-8601 with tz offset")
	require.Equal(t, "Asia/Bangkok", rows[1][2])
	require.Equal(t, "Scope", rows[2][0])
	require.Equal(t, "Range", rows[2][2])

	// Test 3: Data header row.
	// The blank separator line is skipped by csv.Reader — rows[3] is the data header.
	require.Equal(t, []string{"Period", "Consumption", "Unit", "Δ vs prior", "Δ vs YoY"}, rows[3])

	// Test 4: First data row (rows[4]).
	require.Regexp(t, `^2026-05-01T00:00:00\+07:00$`, rows[4][0], "period must be in install_tz (Asia/Bangkok = +07:00)")
	require.Equal(t, "1.234", rows[4][1])
	require.Equal(t, "m³", rows[4][2])
	require.Equal(t, "+12.3%", rows[4][3])
	require.Equal(t, "", rows[4][4], "YoY silent fallback must be empty string (D-03), not 'null'")

	// Second data row — YoY present (rows[5]).
	require.Equal(t, "+8.0%", rows[5][4])

	// Test 4 extra: Only one BOM in the entire output.
	require.Equal(t, 1, strings.Count(string(raw), "\xEF\xBB\xBF"), "must contain exactly one BOM")
}
