package report

import (
	"fmt"
	"os"
	"time"

	"github.com/johnfercher/maroto/v2"
	"github.com/johnfercher/maroto/v2/pkg/components/col"
	"github.com/johnfercher/maroto/v2/pkg/components/image"
	"github.com/johnfercher/maroto/v2/pkg/components/line"
	"github.com/johnfercher/maroto/v2/pkg/components/row"
	"github.com/johnfercher/maroto/v2/pkg/components/text"
	"github.com/johnfercher/maroto/v2/pkg/config"
	"github.com/johnfercher/maroto/v2/pkg/consts/align"
	"github.com/johnfercher/maroto/v2/pkg/consts/fontstyle"
	"github.com/johnfercher/maroto/v2/pkg/core"
	"github.com/johnfercher/maroto/v2/pkg/props"
)

// WritePDF generates a multi-page report PDF using maroto/v2 with branded
// header + footer on every page (D-02).
//
// PAGE N OF M IMPLEMENTATION (RESEARCH Open Q #3 — resolved):
// maroto v2's WithPageNumber config option places the page number as an
// overlay on each page, substituting {current} and {total} at render time.
// We use pattern "page {current} of {total}" in the config and keep the
// registered footer row as the static "Generated <ts> <tz> — " part.
// Both text items appear in the raw PDF bytes, satisfying:
//   - "Generated" in the PDF body (registered footer row)
//   - "page N of M" in the PDF body (WithPageNumber overlay per page)
//
// When the report has >50 rows, SequentialLowMemoryMode is enabled to
// keep RAM usage bounded for large reports.
func WritePDF(rpt *ReportData, identity InstallIdentity) ([]byte, error) {
	tz := identity.Timezone
	if tz == nil {
		tz = time.UTC
	}

	// Build config with page number overlay.
	b := config.NewBuilder().
		WithLeftMargin(15).
		WithTopMargin(18).
		WithRightMargin(15).
		WithBottomMargin(15).
		WithPageNumber(props.PageNumber{
			Pattern: "page {current} of {total}",
			Place:   props.Bottom,
		})

	if len(rpt.PeriodRows)+len(rpt.MeterRows) > 50 {
		b = b.WithSequentialLowMemoryMode(1)
	}

	cfg := b.Build()
	m := maroto.New(cfg)

	if err := registerHeader(m, identity); err != nil {
		return nil, fmt.Errorf("register header: %w", err)
	}
	if err := registerFooter(m, identity); err != nil {
		return nil, fmt.Errorf("register footer: %w", err)
	}

	addBodyRows(m, rpt, identity)

	doc, err := m.Generate()
	if err != nil {
		return nil, fmt.Errorf("maroto generate: %w", err)
	}
	return doc.GetBytes(), nil
}

// registerHeader registers branded header rows.
// Header: logo + display_name left, address right; thin navy rule below.
// Degrades gracefully to text-only when LogoPath is empty or missing.
func registerHeader(m core.Maroto, identity InstallIdentity) error {
	var headerRow core.Row
	if identity.LogoPath != "" {
		if _, err := os.Stat(identity.LogoPath); err == nil {
			headerRow = row.New(10).Add(
				image.NewFromFileCol(2, identity.LogoPath, props.Rect{Center: false}),
				text.NewCol(6, identity.DisplayName, props.Text{Size: 12, Style: fontstyle.Bold}),
				text.NewCol(4, identity.Address, props.Text{Size: 9, Align: align.Right}),
			)
		}
	}
	if headerRow == nil {
		headerRow = row.New(10).Add(
			text.NewCol(8, identity.DisplayName, props.Text{Size: 12, Style: fontstyle.Bold}),
			text.NewCol(4, identity.Address, props.Text{Size: 9, Align: align.Right}),
		)
	}

	// Navy rule: thin horizontal line below the name/address row.
	ruleRow := row.New(1).Add(
		line.NewCol(12, props.Line{
			Color:     &props.Color{Red: 28, Green: 47, Blue: 109},
			Thickness: 0.5,
		}),
	)

	return m.RegisterHeader(headerRow, ruleRow)
}

// registerFooter registers the static "Generated <ts> <tz> —" footer row.
// The "page N of M" part is rendered by the WithPageNumber overlay separately.
func registerFooter(m core.Maroto, identity InstallIdentity) error {
	tz := identity.Timezone
	if tz == nil {
		tz = time.UTC
	}
	ts := time.Now().In(tz).Format(time.RFC3339)
	footerText := fmt.Sprintf("Generated %s %s —", ts, tz.String())
	return m.RegisterFooter(row.New(5).Add(
		text.NewCol(12, footerText, props.Text{
			Size:  8,
			Color: &props.Color{Red: 100, Green: 100, Blue: 100},
		}),
	))
}

// addBodyRows adds the report body (summary + period table + meter table) to m.
func addBodyRows(m core.Maroto, rpt *ReportData, identity InstallIdentity) {
	tz := identity.Timezone
	if tz == nil {
		tz = time.UTC
	}

	// Summary section.
	m.AddRow(8, text.NewCol(12, "Shifter consumption report",
		props.Text{Size: 14, Style: fontstyle.Bold}))
	m.AddRow(6, text.NewCol(12, describeScope(rpt.Config),
		props.Text{Size: 10, Color: &props.Color{Red: 80, Green: 80, Blue: 80}}))
	m.AddRow(8, text.NewCol(12, fmt.Sprintf("Total: %.3f %s",
		rpt.Summary.TotalConsumption, unitLabel("", identity.Units)),
		props.Text{Size: 12}))
	if rpt.Summary.PriorDelta != nil {
		m.AddRow(6, text.NewCol(12,
			fmt.Sprintf("Delta vs prior period: %+.1f%%", rpt.Summary.PriorDelta.Percent),
			props.Text{Size: 10}))
	}
	if rpt.Summary.YoYDelta != nil {
		m.AddRow(6, text.NewCol(12,
			fmt.Sprintf("Delta vs same period last year: %+.1f%%", rpt.Summary.YoYDelta.Percent),
			props.Text{Size: 10}))
	}

	// Spacer.
	m.AddRow(4, col.New(12))

	// Period detail table.
	m.AddRow(8, text.NewCol(12, "Per-period breakdown",
		props.Text{Size: 12, Style: fontstyle.Bold}))
	m.AddRow(6,
		text.NewCol(4, "Period", props.Text{Style: fontstyle.Bold}),
		text.NewCol(3, "Consumption", props.Text{Style: fontstyle.Bold, Align: align.Right}),
		text.NewCol(2, "D prior", props.Text{Style: fontstyle.Bold, Align: align.Right}),
		text.NewCol(2, "D YoY", props.Text{Style: fontstyle.Bold, Align: align.Right}),
	)
	for _, prow := range rpt.PeriodRows {
		m.AddRow(5,
			text.NewCol(4, prow.Period.In(tz).Format("2006-01-02 15:04")),
			text.NewCol(3, fmt.Sprintf("%.3f", prow.Consumption), props.Text{Align: align.Right}),
			text.NewCol(2, deltaText(prow.DeltaVsPrior), props.Text{Align: align.Right}),
			text.NewCol(2, deltaText(prow.DeltaVsYoY), props.Text{Align: align.Right}),
		)
	}

	// Per-meter rollup (scope != meter only).
	if rpt.Config.Scope != "meter" && len(rpt.MeterRows) > 0 {
		m.AddRow(4, col.New(12))
		m.AddRow(8, text.NewCol(12, "Per-meter rollup",
			props.Text{Size: 12, Style: fontstyle.Bold}))
		m.AddRow(6,
			text.NewCol(4, "Meter", props.Text{Style: fontstyle.Bold}),
			text.NewCol(3, "Site", props.Text{Style: fontstyle.Bold}),
			text.NewCol(2, "Utility", props.Text{Style: fontstyle.Bold}),
			text.NewCol(3, "Total", props.Text{Style: fontstyle.Bold, Align: align.Right}),
		)
		for _, mrow := range rpt.MeterRows {
			m.AddRow(5,
				text.NewCol(4, mrow.Name),
				text.NewCol(3, mrow.SiteName),
				text.NewCol(2, mrow.UtilityClass),
				text.NewCol(3, fmt.Sprintf("%.3f", mrow.Consumption), props.Text{Align: align.Right}),
			)
		}
	}
}

// deltaText returns a formatted delta percentage string, or "-" if nil.
func deltaText(d *DeltaResult) string {
	if d == nil {
		return "-"
	}
	return fmt.Sprintf("%+.1f%%", d.Percent)
}
