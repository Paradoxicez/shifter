/**
 * ReportResultPanel — Plan 05-09 Task 2
 *
 * 3 download tiles (CSV / Excel / PDF) + summary chart + period table + meter table.
 * PDF tile uses useReportPDFStatus to poll until ready, then fires a Sonner toast.
 *
 * This stub is created in Task 1 so index.tsx compiles; Task 2 fills in the full
 * implementation including useReportPDFStatus, PdfStatusPill, ReportSummaryChart,
 * ReportPeriodTable, ReportMeterTable.
 */

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Download } from 'lucide-react'
import type { GenerateResponse } from './useReportGenerate'
import type { ReportConfig } from './index'

interface ReportResultPanelProps {
  result: GenerateResponse
  cfg: ReportConfig
  onClear: () => void
}

export function ReportResultPanel({ result, onClear }: ReportResultPanelProps) {
  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <h2 className="text-xl font-semibold">Report ready</h2>
        <Button variant="ghost" onClick={onClear}>
          Configure another
        </Button>
      </div>

      {/* 3 download tiles */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <Card>
          <CardHeader>
            <CardTitle>CSV</CardTitle>
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

        <Card>
          <CardHeader>
            <CardTitle>Excel</CardTitle>
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

        <Card>
          <CardHeader>
            <CardTitle>PDF</CardTitle>
          </CardHeader>
          <CardContent>
            {/* PdfStatusPill wired in Task 2 */}
            <div className="text-sm text-muted-foreground">Generating PDF…</div>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
