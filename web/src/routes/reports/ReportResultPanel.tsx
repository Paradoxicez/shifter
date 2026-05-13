/**
 * ReportResultPanel — Plan 05-09 Task 2
 *
 * 3 download tiles (CSV / Excel / PDF) + summary chart + period table + meter table.
 *
 * D-06: CSV + Excel tiles enabled immediately (direct download links).
 *        PDF tile uses useReportPDFStatus to poll every 2s until ready/failed,
 *        then Sonner toast fires (wired inside useReportPDFStatus).
 *
 * D-07: "Configure another" button calls onClear() which sets result=null in
 *        the parent state machine, returning to the config panel.
 *
 * D-03: ReportPeriodTable shows Δ YoY column only when data present (silent fallback).
 *
 * Meter table rendered only when scope != 'meter' (single-meter report
 * has no cross-meter breakdown).
 */

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Download } from 'lucide-react'
import { useReportPDFStatus } from './useReportPDFStatus'
import { PdfStatusPill } from './PdfStatusPill'
import { ReportSummaryChart } from './ReportSummaryChart'
import { ReportPeriodTable } from './ReportPeriodTable'
import { ReportMeterTable } from './ReportMeterTable'
import type { GenerateResponse } from './useReportGenerate'
import type { ReportConfig } from './index'

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface ReportResultPanelProps {
  result: GenerateResponse
  cfg: ReportConfig
  onClear: () => void
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function ReportResultPanel({ result, cfg, onClear }: ReportResultPanelProps) {
  // Poll pdf_status — fires Sonner toast internally when ready/failed
  const pdfStatus = useReportPDFStatus(result.report_id, result.pdf_status)

  // Derive utility classes present in meter_rows for capability-gated chart.
  // Backend returns null instead of [] when scope=meter (single MP, no rollup) —
  // tolerate either shape so this panel works for both fleet and per-meter reports.
  const meterRows = result.report.meter_rows ?? []
  const utilityClasses = [...new Set(meterRows.map((r) => r.utility_class))]

  return (
    <div className="space-y-6">
      {/* Header row */}
      <div className="flex justify-between items-center">
        <h2 className="text-xl font-semibold">Report ready</h2>
        <Button variant="ghost" onClick={onClear}>
          Configure another
        </Button>
      </div>

      {/* 3 download tiles */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        {/* CSV tile — immediately enabled */}
        <Card>
          <CardHeader>
            <CardTitle className="text-base">CSV</CardTitle>
          </CardHeader>
          <CardContent>
            <Button asChild className="w-full">
              <a href={`/api/reports/${result.report_id}/file/csv`} download>
                <Download className="h-4 w-4 mr-2" />
                Download CSV
              </a>
            </Button>
          </CardContent>
        </Card>

        {/* Excel tile — immediately enabled */}
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Excel</CardTitle>
          </CardHeader>
          <CardContent>
            <Button asChild className="w-full">
              <a href={`/api/reports/${result.report_id}/file/xlsx`} download>
                <Download className="h-4 w-4 mr-2" />
                Download Excel
              </a>
            </Button>
          </CardContent>
        </Card>

        {/* PDF tile — async; PdfStatusPill reflects poll state */}
        <Card>
          <CardHeader>
            <CardTitle className="text-base">PDF</CardTitle>
          </CardHeader>
          <CardContent>
            <PdfStatusPill status={pdfStatus} reportID={result.report_id} />
          </CardContent>
        </Card>
      </div>

      {/* Summary chart — capability-gated by utility classes in data */}
      {result.report.period_rows.length > 0 && (
        <ReportSummaryChart
          periodRows={result.report.period_rows}
          utilityClasses={utilityClasses.length > 0 ? utilityClasses : undefined}
        />
      )}

      {/* Period breakdown table (D-03: YoY column shown only when data present) */}
      <ReportPeriodTable rows={result.report.period_rows} />

      {/* Meter table — only for all-meters or single-site scope */}
      {cfg.scope !== 'meter' && <ReportMeterTable rows={meterRows} />}
    </div>
  )
}
