/**
 * ReportsPage — Plan 05-09 Task 1
 *
 * State machine: config panel ↔ result panel (ephemeral — D-07).
 * URL state via useSearchParams + zod (D-05, Phase 3 D-15 / Phase 4 D-14 pattern).
 * Generate mutation delegated to useReportGenerate hook — NO inline useMutation here.
 * Empty state for zero-sites case (UI-SPEC §Empty States).
 */

import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useSearchParams } from 'react-router-dom'
import { z } from 'zod'
import { FileText } from 'lucide-react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Link } from 'react-router-dom'
import { apiFetch } from '@/lib/api'
import { ReportConfigPanel } from './ReportConfigPanel'
import { ReportResultPanel } from './ReportResultPanel'
import { useReportGenerate } from './useReportGenerate'
import type { GenerateResponse } from './useReportGenerate'

// ---------------------------------------------------------------------------
// URL state schema — T-05-09-01: zod .catch() fallbacks prevent injection
// ---------------------------------------------------------------------------

const ReportsURLSchema = z.object({
  scope: z.enum(['all', 'site', 'meter']).catch('all'),
  site_id: z.string().uuid().optional().catch(undefined),
  mp_id: z.string().uuid().optional().catch(undefined),
  group: z.enum(['site', 'category', 'none']).catch('none'),
  range: z.enum(['daily', 'monthly', 'yearly', 'custom']).catch('monthly'),
  start: z.string().optional().catch(undefined),
  end: z.string().optional().catch(undefined),
})

export type ReportConfig = z.infer<typeof ReportsURLSchema>

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function omitUndefined<T extends Record<string, unknown>>(obj: T): Partial<Record<keyof T, string>> {
  return Object.fromEntries(
    Object.entries(obj).filter(([, v]) => v !== undefined)
  ) as Partial<Record<keyof T, string>>
}

// ---------------------------------------------------------------------------
// Empty state component for zero-sites case
// ---------------------------------------------------------------------------

function ReportsEmptyState() {
  return (
    <Card className="mx-auto max-w-md text-center">
      <CardHeader>
        <div className="mx-auto mb-2 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
          <FileText className="h-6 w-6 text-primary" aria-hidden="true" />
        </div>
        <CardTitle>Nothing to report yet</CardTitle>
        <CardDescription>
          Add a site and some devices to generate consumption reports.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <Button asChild>
          <Link to="/sites">Go to Sites</Link>
        </Button>
      </CardContent>
    </Card>
  )
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export function ReportsPage() {
  const [params, setParams] = useSearchParams()
  const cfg: ReportConfig = ReportsURLSchema.parse(Object.fromEntries(params))

  // Ephemeral result state — navigating away resets this (D-07)
  const [result, setResult] = useState<GenerateResponse | null>(null)

  // Zero-sites check for empty state
  const { data: sites = [], isLoading } = useQuery<Array<{ id: string; name: string }>>({
    queryKey: ['sites'],
    queryFn: () => apiFetch<Array<{ id: string; name: string }>>('/api/sites'),
  })

  // Generate mutation via dedicated hook — NO inline useMutation in this file
  const { mutate: generate, isPending } = useReportGenerate()

  function handleCfgChange(next: ReportConfig) {
    setParams(new URLSearchParams(omitUndefined(next) as Record<string, string>))
  }

  function handleGenerate(body: Parameters<typeof generate>[0]) {
    generate(body, { onSuccess: (data) => setResult(data) })
  }

  if (isLoading) return null

  if (sites.length === 0) {
    return (
      <div className="p-6 flex justify-center">
        <ReportsEmptyState />
      </div>
    )
  }

  return (
    <div className="p-6 space-y-6 max-w-6xl mx-auto">
      <h1 className="text-2xl font-semibold leading-8">Reports</h1>
      {result === null ? (
        <ReportConfigPanel
          cfg={cfg}
          onChange={handleCfgChange}
          onGenerate={handleGenerate}
          isPending={isPending}
        />
      ) : (
        <ReportResultPanel
          result={result}
          cfg={cfg}
          onClear={() => setResult(null)}
        />
      )}
    </div>
  )
}

export default ReportsPage
