---
phase: 05-aggregates-reports-map-floor-plans
plan: 09
type: execute
wave: 5
depends_on: [06, 08]
files_modified:
  - web/src/routes/reports/index.tsx
  - web/src/routes/reports/index.test.tsx
  - web/src/routes/reports/ReportConfigPanel.tsx
  - web/src/routes/reports/ReportResultPanel.tsx
  - web/src/routes/reports/ReportSummaryChart.tsx
  - web/src/routes/reports/ReportPeriodTable.tsx
  - web/src/routes/reports/ReportMeterTable.tsx
  - web/src/routes/reports/PdfStatusPill.tsx
  - web/src/routes/reports/useReportGenerate.ts
  - web/src/routes/reports/useReportPDFStatus.ts
  - web/src/routes/reports/useReportPDFStatus.test.ts
  - web/src/App.tsx
  - web/playwright/specs/reports-generate.spec.ts
autonomous: true
requirements: [REPT-01, REPT-02, REPT-03, REPT-04, REPT-05, REPT-06, REPT-07]
threat_refs: [T-05-09-01]

must_haves:
  truths:
    - "/reports route renders ReportConfigPanel by default; switches to ReportResultPanel after Generate clicked"
    - "Config panel: 3-radio scope picker (All meters / Single site / Single meter) per D-05 with conditional secondary picker"
    - "Config panel: DateRangePicker (reuse Phase 4 component) + Range preset (daily/monthly/yearly) + optional GroupBy when scope=all"
    - "URL state via useSearchParams + zod (Phase 3 D-15 / Phase 4 D-14 pattern): ?scope, ?site_id, ?mp_id, ?range, ?start, ?end, ?group"
    - "useReportGenerate hook (web/src/routes/reports/useReportGenerate.ts) wraps the POST /api/reports/generate mutation; index.tsx consumes it (NO inline useMutation in index.tsx)"
    - "Generate posts to /api/reports/generate; response renders ReportResultPanel with 3 download tiles (CSV / Excel / PDF)"
    - "CSV + Excel tiles enabled immediately (links to /api/reports/:id/file/{csv|xlsx})"
    - "PDF tile shows Loader2 spinner + 'Generating PDF…' until polling detects pdf_status='ready', then becomes 'Download PDF'"
    - "Sonner toast fires on PDF ready: 'Your PDF is ready — click to download.' (with click-to-download action)"
    - "Navigating away from /reports loses the result panel (D-07 ephemeral — no /reports/history page)"
    - "Empty state when zero sites: EmptyStateOnboarding card with 'Go to Sites' CTA"
    - "Period-delta presentation: prior delta always shown; YoY column shown only when ANY row has DeltaVsYoY populated (D-03 silent fallback)"
    - "Capability-gated chart: water-only install renders water chart only; electricity-only renders electricity chart only"
    - "No new shadcn primitives — uses existing radix-via-shadcn components (UI-SPEC Dim 6 PASS)"
  artifacts:
    - path: "web/src/routes/reports/index.tsx"
      provides: "ReportsPage; state machine between config + result panels; empty-state for zero-sites; consumes useReportGenerate hook (no inline mutation)"
      contains: "useReportGenerate("
    - path: "web/src/routes/reports/ReportConfigPanel.tsx"
      provides: "Scope radio + conditional pickers + DateRangePicker + Generate button"
      contains: "Generate report"
    - path: "web/src/routes/reports/ReportResultPanel.tsx"
      provides: "3 download tiles + summary chart + period table + meter table"
      contains: "Download CSV"
    - path: "web/src/routes/reports/PdfStatusPill.tsx"
      provides: "Pill with Loader2/Download icon + status text; reflects pdf_status from poll"
      contains: "Generating PDF"
    - path: "web/src/routes/reports/useReportGenerate.ts"
      provides: "useReportGenerate(): UseMutationResult<GenerateResponse, Error, ReportGenerateRequest> — wraps POST /api/reports/generate"
      contains: "export function useReportGenerate"
    - path: "web/src/routes/reports/useReportPDFStatus.ts"
      provides: "useReportPDFStatus(reportID) hook — polls GET /api/reports/:id every 2s until ready/failed"
      contains: "useReportPDFStatus"
    - path: "web/src/routes/reports/useReportPDFStatus.test.ts"
      provides: "Vitest cases pinning poll-stops-on-ready, toast-fires-once, error-on-failed behaviors"
      contains: "useReportPDFStatus"
  key_links:
    - from: "web/src/routes/reports/index.tsx"
      to: "useReportGenerate hook (web/src/routes/reports/useReportGenerate.ts)"
      via: "import + call site (NO inline useMutation in index.tsx)"
      pattern: "useReportGenerate\\("
    - from: "web/src/routes/reports/useReportGenerate.ts"
      to: "POST /api/reports/generate"
      via: "useMutation wrapper"
      pattern: "/api/reports/generate"
    - from: "web/src/routes/reports/useReportPDFStatus.ts"
      to: "GET /api/reports/:id"
      via: "useQuery with refetchInterval=2000"
      pattern: "refetchInterval"
    - from: "web/src/routes/reports/ReportResultPanel.tsx"
      to: "GET /api/reports/:id/file/{csv|xlsx|pdf}"
      via: "<a download href>"
      pattern: "/file/"
---

<objective>
Ship the /reports surface: ReportConfigPanel (3-radio scope picker + conditional pickers + range preset + URL state via useSearchParams+zod) and ReportResultPanel (3 download tiles with CSV+Excel immediate, PDF async via polling + toast). Empty state for zero-sites case. Capability-gated summary chart that mirrors dashboard chart styling. The Generate mutation is extracted to a dedicated `useReportGenerate.ts` hook (consistent with `useReportPDFStatus.ts`) so index.tsx stays a state-machine glue file rather than a hook-and-render mix.

Purpose: This is the customer-facing JTBD of Phase 5 — "give me a branded report I can hand to my CFO." All the backend pieces (CSV/Excel/PDF/audit/status) exist after plan 05-06; this plan exposes them through the UI per UI-SPEC §Report Layout Spec + Interaction States.

Output: 11 React/TS files in `web/src/routes/reports/` (including the new `useReportGenerate.ts` hook and `useReportPDFStatus.test.ts` vitest spec), App.tsx route wire-up, Playwright spec body.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/05-aggregates-reports-map-floor-plans/05-CONTEXT.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-RESEARCH.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-UI-SPEC.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-06-reports-pdf-river-worker-PLAN.md
@web/src/components/responsive-dialog.tsx
@web/src/components/ui/chart.tsx
@web/src/components/empty-state-onboarding.tsx
@web/src/routes/dashboard.tsx

<interfaces>
<!-- POST /api/reports/generate request -->
```ts
type ReportGenerateRequest = {
  scope: 'all' | 'site' | 'meter'
  site_id?: string         // when scope='site'
  mp_id?: string           // when scope='meter'
  group?: 'site' | 'category' | 'none'
  range: 'daily' | 'monthly' | 'yearly' | 'custom'
  start?: string           // ISO when range='custom'
  end?: string
}

type GenerateResponse = {
  report_id: string
  report: {
    summary: { total_consumption: number; prior_delta?: { absolute: number; percent: number }; yoy_delta?: { absolute: number; percent: number } }
    period_rows: Array<{ period: string; consumption: number; delta_vs_prior?: { absolute: number; percent: number }; delta_vs_yoy?: { absolute: number; percent: number } }>
    meter_rows: Array<{ id: string; name: string; site_name: string; utility_class: string; consumption: number }>
  }
  pdf_status: 'pending' | 'running' | 'ready' | 'failed' | 'expired'
}
```

<!-- GET /api/reports/:id (status poll — provided by plan 05-06 StatusHandler) -->
```ts
type ReportStatus = {
  id: string
  pdf_status: 'pending' | 'running' | 'ready' | 'failed' | 'expired'
  pdf_path?: string
  created_at: string
  scope: 'all' | 'site' | 'meter'
  range: 'daily' | 'monthly' | 'yearly' | 'custom'
}
```

<!-- URL state schema (zod) -->
```ts
const ReportsURLState = z.object({
  scope: z.enum(['all', 'site', 'meter']).catch('all'),
  site_id: z.string().uuid().optional().catch(undefined),
  mp_id: z.string().uuid().optional().catch(undefined),
  group: z.enum(['site', 'category', 'none']).catch('none'),
  range: z.enum(['daily', 'monthly', 'yearly', 'custom']).catch('monthly'),
  start: z.string().optional().catch(undefined),
  end: z.string().optional().catch(undefined),
})
```

<!-- useReportGenerate hook signature (new in this plan, extracted to its own file) -->
```ts
import { UseMutationResult } from '@tanstack/react-query'

export function useReportGenerate(): UseMutationResult<GenerateResponse, Error, ReportGenerateRequest>
```
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: useReportGenerate hook + ReportConfigPanel + URL state + index.tsx state machine</name>
  <read_first>
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-CONTEXT.md §D-05 §D-06
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-UI-SPEC.md §Copywriting Contract §Interaction States — Report Generation Flow §Empty States §Route Architecture
    - web/src/routes/dashboard.tsx (existing useSearchParams + zod pattern reference)
    - web/src/components/ui/radio-group.tsx (shadcn primitive — confirm present)
    - web/src/components/ui/select.tsx + command.tsx (Combobox for MP search)
    - web/src/components/date-range-picker.tsx (Phase 4 DateRangePicker)
  </read_first>
  <behavior>
    - Test 1: ConfigPanel renders 3 radio buttons labeled "All meters", "Single site", "Single meter"
    - Test 2: Selecting "Single site" reveals a site Select dropdown
    - Test 3: Selecting "Single meter" reveals a MP Combobox with search
    - Test 4: Selecting "All meters" reveals a "Group by" select with options (site, category, none)
    - Test 5: Range preset buttons (Daily / Monthly / Yearly / Custom) update URL search params
    - Test 6: Custom range opens the existing DateRangePicker
    - Test 7: Clicking "Generate report" triggers the useReportGenerate mutation (asserted via mocked fetch — POST /api/reports/generate received the current form state body)
    - Test 8: While the mutation is pending, the button shows Loader2 spinner + disabled state (the `isPending` flag from useReportGenerate drives the UI)
    - Test 9: Empty state when zero sites — EmptyStateOnboarding card + "Go to Sites" CTA
    - Test 10: URL state survives refresh — opening /reports?scope=site&site_id=... pre-fills the form
    - Test 11 (hook contract): `useReportGenerate` returns a `UseMutationResult` with `mutate`, `isPending`, `data` keys (smoke test the hook in isolation with a mocked fetch)
  </behavior>
  <action>
**Step A — `web/src/routes/reports/useReportGenerate.ts` (NEW FILE, hook extracted from inline useMutation):**

```ts
import { useMutation, UseMutationResult } from '@tanstack/react-query'
import { apiFetch } from '@/lib/apiFetch'

export type ReportGenerateRequest = {
  scope: 'all' | 'site' | 'meter'
  site_id?: string
  mp_id?: string
  group?: 'site' | 'category' | 'none'
  range: 'daily' | 'monthly' | 'yearly' | 'custom'
  start?: string
  end?: string
}

export type GenerateResponse = {
  report_id: string
  report: {
    summary: {
      total_consumption: number
      prior_delta?: { absolute: number; percent: number }
      yoy_delta?: { absolute: number; percent: number }
    }
    period_rows: Array<{
      period: string
      consumption: number
      delta_vs_prior?: { absolute: number; percent: number }
      delta_vs_yoy?: { absolute: number; percent: number }
    }>
    meter_rows: Array<{ id: string; name: string; site_name: string; utility_class: string; consumption: number }>
  }
  pdf_status: 'pending' | 'running' | 'ready' | 'failed' | 'expired'
}

/**
 * useReportGenerate wraps the POST /api/reports/generate mutation.
 *
 * Extracted to its own file so index.tsx stays a state-machine glue file
 * rather than mixing hook construction with render logic — matches the
 * companion pattern of useReportPDFStatus.ts.
 *
 * Backend contract:
 *   - Request body: ReportGenerateRequest
 *   - Response: GenerateResponse (includes pdf_status seed for the result panel
 *     to pass into useReportPDFStatus's initialStatus)
 */
export function useReportGenerate(): UseMutationResult<GenerateResponse, Error, ReportGenerateRequest> {
  return useMutation({
    mutationFn: async (body: ReportGenerateRequest) =>
      apiFetch<GenerateResponse>('/api/reports/generate', {
        method: 'POST',
        body: JSON.stringify(body),
      }),
  })
}
```

**Step B — `web/src/routes/reports/index.tsx` (consumes useReportGenerate — NO inline useMutation):**

```tsx
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useSearchParams } from 'react-router-dom'
import { z } from 'zod'
import { ReportConfigPanel } from './ReportConfigPanel'
import { ReportResultPanel } from './ReportResultPanel'
import { useReportGenerate, type GenerateResponse } from './useReportGenerate'
import { EmptyStateOnboarding } from '@/components/empty-state-onboarding'
import { FileText } from 'lucide-react'
import { apiFetch } from '@/lib/apiFetch'

const ReportsURLSchema = z.object({
  scope: z.enum(['all', 'site', 'meter']).catch('all'),
  site_id: z.string().uuid().optional().catch(undefined),
  mp_id: z.string().uuid().optional().catch(undefined),
  group: z.enum(['site', 'category', 'none']).catch('none'),
  range: z.enum(['daily', 'monthly', 'yearly', 'custom']).catch('monthly'),
  start: z.string().optional().catch(undefined),
  end: z.string().optional().catch(undefined),
})

type ReportConfig = z.infer<typeof ReportsURLSchema>

export function ReportsPage() {
  const [params, setParams] = useSearchParams()
  const cfg: ReportConfig = ReportsURLSchema.parse(Object.fromEntries(params))

  const [result, setResult] = useState<GenerateResponse | null>(null)

  // Check zero-sites for the empty state.
  const { data: sites = [], isLoading } = useQuery({
    queryKey: ['sites'],
    queryFn: async () => apiFetch<Array<{ id: string; name: string }>>('/api/sites'),
  })

  // Generate mutation is the dedicated hook — no inline useMutation here.
  const { mutate: generate, isPending } = useReportGenerate()

  if (isLoading) return null
  if (sites.length === 0) {
    return (
      <EmptyStateOnboarding
        icon={FileText}
        heading="Nothing to report yet"
        body="Add a site and some devices to generate consumption reports."
        cta={{ label: 'Go to Sites', to: '/sites' }}
      />
    )
  }

  return (
    <div className="p-6 space-y-6 max-w-6xl mx-auto">
      <h1 className="text-2xl font-semibold leading-8">Reports</h1>
      {result === null ? (
        <ReportConfigPanel
          cfg={cfg}
          onChange={(next) => setParams(new URLSearchParams(omitUndefined(next) as Record<string, string>))}
          onGenerate={(body) => generate(body, { onSuccess: setResult })}
          isPending={isPending}
        />
      ) : (
        <ReportResultPanel result={result} cfg={cfg} onClear={() => setResult(null)} />
      )}
    </div>
  )
}

function omitUndefined<T extends Record<string, any>>(obj: T): Partial<T> {
  return Object.fromEntries(Object.entries(obj).filter(([_, v]) => v !== undefined)) as Partial<T>
}
```

**Step C — `web/src/routes/reports/ReportConfigPanel.tsx`:**

```tsx
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Button } from '@/components/ui/button'
import { Loader2 } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { apiFetch } from '@/lib/apiFetch'
import { DateRangePicker } from '@/components/date-range-picker'
import { MeterCombobox } from '@/components/meter-combobox'  // new — light wrapper around Command for MP search

export function ReportConfigPanel({ cfg, onChange, onGenerate, isPending }: {
  cfg: ReportConfig
  onChange: (next: ReportConfig) => void
  onGenerate: (body: ReportConfig) => void
  isPending: boolean
}) {
  const { data: sites = [] } = useQuery({
    queryKey: ['sites'],
    queryFn: async () => apiFetch<Array<{ id: string; name: string }>>('/api/sites'),
  })
  const { data: meters = [] } = useQuery({
    queryKey: ['metering-points'],
    queryFn: async () => apiFetch<Array<{ id: string; name: string; site_name: string }>>('/api/metering-points'),
    enabled: cfg.scope === 'meter',
  })

  return (
    <Card>
      <CardHeader><CardTitle>Configure report</CardTitle></CardHeader>
      <CardContent className="space-y-6">
        <div>
          <Label className="text-xs font-semibold tracking-wide uppercase mb-2">Scope</Label>
          <RadioGroup value={cfg.scope} onValueChange={(v) => onChange({ ...cfg, scope: v as any })}>
            <div className="flex items-center gap-2"><RadioGroupItem value="all" id="scope-all"/><Label htmlFor="scope-all">All meters</Label></div>
            <div className="flex items-center gap-2"><RadioGroupItem value="site" id="scope-site"/><Label htmlFor="scope-site">Single site</Label></div>
            <div className="flex items-center gap-2"><RadioGroupItem value="meter" id="scope-meter"/><Label htmlFor="scope-meter">Single meter</Label></div>
          </RadioGroup>
        </div>

        {cfg.scope === 'all' && (
          <div>
            <Label>Group by</Label>
            <Select value={cfg.group} onValueChange={(v) => onChange({ ...cfg, group: v as any })}>
              <SelectTrigger><SelectValue/></SelectTrigger>
              <SelectContent>
                <SelectItem value="none">None (fleet totals)</SelectItem>
                <SelectItem value="site">Site</SelectItem>
                <SelectItem value="category">Utility class</SelectItem>
              </SelectContent>
            </Select>
          </div>
        )}

        {cfg.scope === 'site' && (
          <div>
            <Label>Site</Label>
            <Select value={cfg.site_id ?? ''} onValueChange={(v) => onChange({ ...cfg, site_id: v })}>
              <SelectTrigger><SelectValue placeholder="Select a site"/></SelectTrigger>
              <SelectContent>
                {sites.map(s => <SelectItem key={s.id} value={s.id}>{s.name}</SelectItem>)}
              </SelectContent>
            </Select>
          </div>
        )}

        {cfg.scope === 'meter' && (
          <div>
            <Label>Meter</Label>
            <MeterCombobox meters={meters} value={cfg.mp_id ?? ''} onChange={(id) => onChange({ ...cfg, mp_id: id })} />
          </div>
        )}

        <div>
          <Label>Range</Label>
          <div className="flex gap-2 flex-wrap">
            {(['daily', 'monthly', 'yearly', 'custom'] as const).map(r => (
              <Button key={r} variant={cfg.range === r ? 'default' : 'outline'} size="sm"
                onClick={() => onChange({ ...cfg, range: r })}>{r.charAt(0).toUpperCase() + r.slice(1)}</Button>
            ))}
          </div>
          {cfg.range === 'custom' && (
            <DateRangePicker value={{ from: cfg.start ? new Date(cfg.start) : undefined, to: cfg.end ? new Date(cfg.end) : undefined }}
              onChange={(range) => onChange({ ...cfg, start: range.from?.toISOString(), end: range.to?.toISOString() })}/>
          )}
        </div>

        <Button onClick={() => onGenerate(cfg)} disabled={isPending} className="w-full">
          {isPending && <Loader2 className="h-4 w-4 mr-2 animate-spin motion-reduce:animate-none"/>}
          {isPending ? 'Generating…' : 'Generate report'}
        </Button>
      </CardContent>
    </Card>
  )
}
```

**Step D — `web/src/components/meter-combobox.tsx` (new light wrapper if no existing MP combobox):**

```tsx
import { useState } from 'react'
import { Command, CommandInput, CommandItem, CommandList } from '@/components/ui/command'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Button } from '@/components/ui/button'
import { ChevronsUpDown } from 'lucide-react'

export function MeterCombobox({ meters, value, onChange }: { meters: Array<{ id: string; name: string; site_name: string }>; value: string; onChange: (id: string) => void }) {
  const [open, setOpen] = useState(false)
  const selected = meters.find(m => m.id === value)
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button variant="outline" role="combobox" aria-expanded={open} className="w-full justify-between">
          {selected ? `${selected.name} — ${selected.site_name}` : 'Select a meter'}
          <ChevronsUpDown className="h-4 w-4 opacity-50"/>
        </Button>
      </PopoverTrigger>
      <PopoverContent className="p-0">
        <Command>
          <CommandInput placeholder="Search meters…"/>
          <CommandList>
            {meters.map(m => (
              <CommandItem key={m.id} onSelect={() => { onChange(m.id); setOpen(false) }}>
                {m.name} <span className="text-muted-foreground ml-2">— {m.site_name}</span>
              </CommandItem>
            ))}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
```

**Step E — Add `/reports` route in `web/src/App.tsx`:**

```tsx
import { ReportsPage } from './routes/reports'
// inside authenticated <Routes>:
<Route path="/reports" element={<ReportsPage />} />
```

**Step F — Replace `it.skip` cases in `web/src/routes/reports/index.test.tsx`:**

```tsx
describe('ReportsPage', () => {
  it('shows config panel with 3-radio scope picker on mount', () => {
    expect(screen.getByLabelText('All meters')).toBeInTheDocument()
    expect(screen.getByLabelText('Single site')).toBeInTheDocument()
    expect(screen.getByLabelText('Single meter')).toBeInTheDocument()
  })

  it('reveals site picker when Single site selected', async () => { /* … */ })
  it('reveals MP combobox when Single meter selected', async () => { /* … */ })
  it('reveals Group by select when All meters selected with group ≠ none', async () => { /* … */ })

  it('Generate triggers useReportGenerate with current cfg as body', async () => {
    // Mock fetch; click Generate; assert mock called with /api/reports/generate and current cfg body shape.
  })

  it('shows Loader2 spinner + disabled state while useReportGenerate.isPending', async () => { /* … */ })
  it('renders ResultPanel after successful Generate (result state set via onSuccess)', async () => { /* … */ })
  it('navigating away (rerender with different route) loses ResultPanel (D-07)', async () => { /* … */ })

  it('shows EmptyStateOnboarding when zero sites', async () => {
    expect(screen.getByText('Nothing to report yet')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Go to Sites' })).toBeInTheDocument()
  })

  it('URL state pre-fills form (?scope=site&site_id=...)', async () => { /* … */ })

  it('useReportGenerate hook smoke (mocked fetch resolves; isPending toggles)', async () => {
    // Render a tiny renderHook harness; call mutate({scope:'meter', mp_id:'...', range:'monthly'});
    // assert isPending true while in-flight, then data matches the mock response,
    // and fetch was called with /api/reports/generate.
  })
})
```
  </action>
  <verify>
    <automated>pnpm --dir web test:run --reporter=basic web/src/routes/reports/ &amp;&amp; pnpm --dir web build &amp;&amp; grep -q "export function useReportGenerate" web/src/routes/reports/useReportGenerate.ts &amp;&amp; grep -q "useReportGenerate(" web/src/routes/reports/index.tsx</automated>
  </verify>
  <acceptance_criteria>
    - `web/src/routes/reports/useReportGenerate.ts` contains literal `export function useReportGenerate`
    - `web/src/routes/reports/useReportGenerate.ts` exports `ReportGenerateRequest` and `GenerateResponse` types
    - `web/src/routes/reports/useReportGenerate.ts` wraps `useMutation` and posts to `/api/reports/generate`
    - `web/src/routes/reports/index.tsx` contains literal `useReportGenerate(` (consumes the hook)
    - `web/src/routes/reports/index.tsx` does NOT contain any inline `useMutation(` call — `grep -c "useMutation(" web/src/routes/reports/index.tsx` returns 0
    - `web/src/routes/reports/index.tsx` contains literal `ReportsURLSchema` AND `useSearchParams` AND `EmptyStateOnboarding`
    - `web/src/routes/reports/ReportConfigPanel.tsx` contains literal text `All meters`, `Single site`, `Single meter`, `Group by`, `Generate report`
    - Empty state copy matches UI-SPEC: literal `Nothing to report yet` AND `Add a site and some devices`
    - At least 11 vitest cases in `index.test.tsx` covering: radio render, conditional pickers, Generate POST, loading state, result-panel render, navigate-away loss, empty state, URL state restoration, useReportGenerate hook smoke
    - `web/src/App.tsx` contains literal `path="/reports"`
    - `pnpm --dir web test:run` exits 0
    - `pnpm --dir web build` exits 0
  </acceptance_criteria>
  <done>Config panel with full URL-state + zod validation + 3-radio scope picker + dedicated useReportGenerate hook (no inline mutation in index.tsx) + empty-state CTA.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: ReportResultPanel + 3 download tiles + PDF poll + toast + useReportPDFStatus.test.ts</name>
  <read_first>
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-UI-SPEC.md §Report Layout Spec §Toast Notifications §Interaction States — Report Generation Flow
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-CONTEXT.md §D-01 §D-03 §D-06 §D-07
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-06-reports-pdf-river-worker-PLAN.md §StatusHandler (consumed endpoint contract)
    - web/src/components/ui/chart.tsx (Recharts wrapper for ReportSummaryChart)
    - web/src/components/ui/sonner.tsx (toast component — confirm import path)
    - web/src/routes/reports/ReportConfigPanel.tsx (sibling — shared types)
  </read_first>
  <behavior>
    - Test 1: ResultPanel renders 3 download tiles: CSV (enabled), Excel (enabled), PDF (spinner + 'Generating PDF…')
    - Test 2: PDF poll request: GET /api/reports/:id every 2s while pdf_status in {pending, running}
    - Test 3: When poll returns pdf_status='ready' → PDF tile shows 'Download PDF' button with href=/api/reports/:id/file/pdf
    - Test 4: Sonner toast fires exactly once when pdf_status flips to 'ready' with copy 'Your PDF is ready — click to download.'
    - Test 5: When pdf_status='failed' → tile shows error variant: 'PDF generation failed'
    - Test 6: SummaryChart renders 1 chart for single-capability install, 2 charts for both
    - Test 7: PeriodTable shows 'vs YoY' column only when ANY row has DeltaVsYoY != null (silent fallback per D-03)
    - Test 8: MeterTable renders only when scope=all or scope=site
    - Test 9: 'Clear' or back-link returns to ConfigPanel state (loses result per D-07)
    - Test 10 (useReportPDFStatus contract): poll stops firing once status leaves {pending, running}
  </behavior>
  <action>
**Step A — `web/src/routes/reports/useReportPDFStatus.ts`:**

```ts
import { useQuery } from '@tanstack/react-query'
import { useEffect, useRef } from 'react'
import { toast } from 'sonner'
import { apiFetch } from '@/lib/apiFetch'

// Matches plan 05-06's StatusResponse JSON shape verbatim.
export type ReportStatus = {
  id: string
  pdf_status: 'pending' | 'running' | 'ready' | 'failed' | 'expired'
  pdf_path?: string
  created_at: string
  scope: 'all' | 'site' | 'meter'
  range: 'daily' | 'monthly' | 'yearly' | 'custom'
}

export function useReportPDFStatus(reportID: string, initialStatus: ReportStatus['pdf_status']) {
  const lastStatus = useRef<ReportStatus['pdf_status']>(initialStatus)

  const query = useQuery({
    queryKey: ['report-status', reportID],
    queryFn: async () => apiFetch<ReportStatus>(`/api/reports/${reportID}`),
    refetchInterval: (q) => {
      const s = q.state.data?.pdf_status ?? initialStatus
      return s === 'pending' || s === 'running' ? 2000 : false
    },
    initialData: {
      id: reportID, pdf_status: initialStatus, created_at: new Date().toISOString(),
      scope: 'meter', range: 'monthly',
    } as ReportStatus,
  })

  useEffect(() => {
    const s = query.data?.pdf_status
    if (s === 'ready' && lastStatus.current !== 'ready') {
      toast.success('Your PDF is ready — click to download.', {
        action: { label: 'Download', onClick: () => window.location.href = `/api/reports/${reportID}/file/pdf` },
      })
      lastStatus.current = 'ready'
    }
    if (s === 'failed' && lastStatus.current !== 'failed') {
      toast.error('PDF generation failed. Try again or contact support.')
      lastStatus.current = 'failed'
    }
  }, [query.data?.pdf_status, reportID])

  return query.data?.pdf_status ?? initialStatus
}
```

**Step B — `web/src/routes/reports/useReportPDFStatus.test.ts` (NEW FILE — listed in files_modified for traceability):**

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useReportPDFStatus } from './useReportPDFStatus'

describe('useReportPDFStatus', () => {
  beforeEach(() => { vi.useFakeTimers() })

  it('polls every 2s while pdf_status is pending', async () => {
    // Mock fetch to return {pdf_status: 'pending'}; advance timers; assert fetch called 2-3 times in 5s
  })

  it('stops polling when pdf_status flips to ready', async () => {
    // Mock fetch: first call returns 'running', second returns 'ready'
    // Advance timers; after 'ready' arrives, advance another 6s and assert fetch was NOT called again
  })

  it('fires toast.success exactly once when status flips to ready', async () => {
    const toastSpy = vi.spyOn(await import('sonner'), 'toast', 'get')
    // Sequence: pending → ready; assert toast.success called with the exact copy and one call
  })

  it('fires toast.error when status is failed', async () => {
    // Mock fetch to return 'failed'; assert toast.error fired once
  })

  it('does not poll when initialStatus is already ready', async () => {
    // initialStatus='ready' → refetchInterval should be false from the first render; fetch should NOT fire
  })
})
```

**Step C — `web/src/routes/reports/PdfStatusPill.tsx`:**

```tsx
import { Loader2, Download, FileText, AlertCircle } from 'lucide-react'
import { Button } from '@/components/ui/button'

export function PdfStatusPill({ status, reportID }: { status: string; reportID: string }) {
  switch (status) {
    case 'pending':
    case 'running':
      return (
        <div className="flex flex-col items-center gap-2 p-4" aria-live="polite">
          <Loader2 className="h-8 w-8 animate-spin motion-reduce:animate-none text-primary" />
          <div className="text-sm text-muted-foreground">Generating PDF…</div>
        </div>
      )
    case 'ready':
      return (
        <Button asChild className="w-full" variant="default">
          <a href={`/api/reports/${reportID}/file/pdf`} download>
            <Download className="h-4 w-4 mr-2"/>
            Download PDF
          </a>
        </Button>
      )
    case 'failed':
      return (
        <div className="flex flex-col items-center gap-2 p-4 text-destructive">
          <AlertCircle className="h-8 w-8"/>
          <div className="text-sm">PDF generation failed</div>
        </div>
      )
    case 'expired':
      return <div className="text-sm text-muted-foreground p-4">Report expired</div>
    default:
      return <FileText className="h-8 w-8 text-muted-foreground"/>
  }
}
```

**Step D — `web/src/routes/reports/ReportResultPanel.tsx`:**

```tsx
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Download } from 'lucide-react'
import { useReportPDFStatus } from './useReportPDFStatus'
import { PdfStatusPill } from './PdfStatusPill'
import { ReportSummaryChart } from './ReportSummaryChart'
import { ReportPeriodTable } from './ReportPeriodTable'
import { ReportMeterTable } from './ReportMeterTable'

export function ReportResultPanel({ result, cfg, onClear }: {
  result: { report_id: string; report: any; pdf_status: string }
  cfg: ReportConfig
  onClear: () => void
}) {
  const pdfStatus = useReportPDFStatus(result.report_id, result.pdf_status as any)

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <h2 className="text-xl font-semibold">Report ready</h2>
        <Button variant="ghost" onClick={onClear}>Configure another</Button>
      </div>

      {/* 3 download tiles */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <Card><CardHeader><CardTitle>CSV</CardTitle></CardHeader><CardContent>
          <Button asChild className="w-full"><a href={`/api/reports/${result.report_id}/file/csv`} download>
            <Download className="h-4 w-4 mr-2"/>Download CSV</a></Button>
        </CardContent></Card>

        <Card><CardHeader><CardTitle>Excel</CardTitle></CardHeader><CardContent>
          <Button asChild className="w-full"><a href={`/api/reports/${result.report_id}/file/xlsx`} download>
            <Download className="h-4 w-4 mr-2"/>Download Excel</a></Button>
        </CardContent></Card>

        <Card><CardHeader><CardTitle>PDF</CardTitle></CardHeader><CardContent>
          <PdfStatusPill status={pdfStatus} reportID={result.report_id}/>
        </CardContent></Card>
      </div>

      {/* Summary chart + tables */}
      <ReportSummaryChart summary={result.report.summary} cfg={cfg}/>
      <ReportPeriodTable rows={result.report.period_rows}/>
      {cfg.scope !== 'meter' && <ReportMeterTable rows={result.report.meter_rows}/>}
    </div>
  )
}
```

**Step E — `web/src/routes/reports/ReportSummaryChart.tsx`:**

Uses shadcn `chart.tsx` (Recharts wrapper from Phase 4). Renders one bar/area chart per utility class present in the data, gated by install capabilities (the data already excludes absent capabilities thanks to plan 05-03's assembler). Single-capability install → 1 chart; both → 2 charts side-by-side on lg, stacked on mobile.

**Step F — `web/src/routes/reports/ReportPeriodTable.tsx`:**

TanStack Table with columns Period / Consumption / Δ prior / Δ YoY. The "Δ YoY" column is conditionally shown by checking `rows.some(r => r.delta_vs_yoy !== null && r.delta_vs_yoy !== undefined)` — silent fallback per D-03.

Cell renderers:
- Period: `format(parseISO(row.period), 'PPP')` (date-fns)
- Consumption: `${row.consumption.toFixed(3)} ${unitLabel}`
- Δ prior: badge with `+12.3%` (green positive for electricity, warning for water positive — UI-SPEC color convention)
- Δ YoY: same; empty when null

**Step G — `web/src/routes/reports/ReportMeterTable.tsx`:**

TanStack Table with columns Meter / Site / Utility / Total / Δ prior. Rendered only when `cfg.scope !== 'meter'`.

**Step H — Playwright spec body in `web/playwright/specs/reports-generate.spec.ts`:**

```ts
test.describe('Reports — generate-once flow', () => {
  test('login → /reports → configure single-meter monthly → Generate → download CSV + Excel + PDF', async ({ page }) => {
    await page.goto('/login')
    await page.fill('[name=email]', 'admin@test.local')
    await page.fill('[name=password]', 'TestPass123!')
    await page.click('button[type=submit]')

    await page.goto('/reports')
    await expect(page.getByText('Configure report')).toBeVisible()

    await page.click('label:has-text("Single meter")')
    await page.click('[role=combobox]')  // open combobox
    await page.click('[role=option]')    // select first meter
    await page.click('button:has-text("Monthly")')

    await page.click('button:has-text("Generate report")')
    await expect(page.getByText('Report ready')).toBeVisible({ timeout: 10_000 })

    // CSV + Excel tiles enabled immediately
    await expect(page.getByRole('link', { name: /Download CSV/ })).toBeVisible()
    await expect(page.getByRole('link', { name: /Download Excel/ })).toBeVisible()

    // PDF tile starts as Generating; wait for ready (poll runs every 2s).
    await expect(page.getByText('Generating PDF…')).toBeVisible()
    await expect(page.getByRole('link', { name: /Download PDF/ })).toBeVisible({ timeout: 30_000 })

    // Toast appears.
    await expect(page.getByText('Your PDF is ready')).toBeVisible()
  })
})
```
  </action>
  <verify>
    <automated>pnpm --dir web test:run --reporter=basic web/src/routes/reports/ &amp;&amp; pnpm --dir web build &amp;&amp; pnpm --dir web exec playwright test --list reports-generate.spec.ts 2&gt;&amp;1 | grep -c "›"</automated>
  </verify>
  <acceptance_criteria>
    - `web/src/routes/reports/useReportPDFStatus.ts` contains literal `refetchInterval` AND `toast.success` AND polling logic stops when status not in pending/running
    - `web/src/routes/reports/useReportPDFStatus.ts` exports `ReportStatus` type matching plan 05-06 StatusResponse shape (id, pdf_status, pdf_path?, created_at, scope, range)
    - `web/src/routes/reports/useReportPDFStatus.test.ts` exists and contains at least 5 vitest cases: poll every 2s while pending, stops on ready, toast.success fires once on ready, toast.error fires on failed, does-not-poll when initialStatus already ready
    - `web/src/routes/reports/PdfStatusPill.tsx` contains literals `Generating PDF…`, `Download PDF`, `PDF generation failed`
    - `web/src/routes/reports/PdfStatusPill.tsx` uses `motion-reduce:animate-none` on Loader2 (accessibility)
    - `web/src/routes/reports/ReportResultPanel.tsx` contains literal `/api/reports/${result.report_id}/file/csv` AND `/file/xlsx` AND uses `useReportPDFStatus`
    - `ReportPeriodTable.tsx` contains literal `rows.some(r => r.delta_vs_yoy` (silent fallback)
    - Toast copy matches UI-SPEC: literal `Your PDF is ready — click to download.`
    - Playwright spec body has no `test.skip`
    - `pnpm --dir web test:run` exits 0
    - `pnpm --dir web build` exits 0
  </acceptance_criteria>
  <done>Result panel ships CSV+Excel immediate downloads and PDF poll+toast async path; useReportPDFStatus contract pinned by dedicated vitest spec; D-03 YoY silent fallback respected.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| Client → /api/reports/generate | POST body validated against zod schema before submit |
| Client → /api/reports/:id (status poll) | Auth-gated; plan 05-06 StatusHandler enforces same auth scope as download |
| Client → /api/reports/:id/file/:kind | Auth-gated download (handler in plan 05-06 enforces) |
| URL query params → form state | zod `.catch()` fallbacks prevent injection of unexpected values |

## STRIDE Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-05-09-01 | Tampering | URL query params manipulated to inject invalid scope/range | low | mitigate | `z.enum(...).catch(default)` falls back to safe defaults on any invalid input; UUID validation on site_id/mp_id catches non-UUID strings |
</threat_model>

<verification>
1. `pnpm --dir web test:run` exits 0
2. `pnpm --dir web build` exits 0
3. `grep -E "Generate report|Generating PDF|Download (CSV|Excel|PDF)" web/src/routes/reports/*.tsx` returns ≥4 matches
4. NO new shadcn primitives added (verify by reading `web/components.json` — count unchanged from Phase 4)
5. Sonner toast invocation: `grep -E "toast\\.(success|error)" web/src/routes/reports/useReportPDFStatus.ts` returns ≥2 matches
6. `grep -c "useMutation(" web/src/routes/reports/index.tsx` returns 0 (no inline mutation; the hook owns it)
7. `web/src/routes/reports/useReportPDFStatus.test.ts` exists and is listed in `files_modified` frontmatter
</verification>

<success_criteria>
- /reports route ships full config + result flow per UI-SPEC
- URL state survives refresh
- Empty state for zero-sites case
- useReportGenerate hook is a standalone file consumed by index.tsx (no inline useMutation)
- PDF poll fires every 2s only while job pending; stops when ready/failed
- Sonner toast fires once on PDF ready
- D-07 navigate-away loses result panel
- D-03 silent YoY fallback respected — column shown only when data present
- useReportPDFStatus contract is pinned by its own dedicated vitest spec (web/src/routes/reports/useReportPDFStatus.test.ts)
</success_criteria>

<output>
After completion, create `.planning/phases/05-aggregates-reports-map-floor-plans/05-09-SUMMARY.md` recording:
- Whether MeterCombobox was new (Phase 5 add) or reused existing
- Number of vitest cases shipped across all reports files (count both index.test.tsx and useReportPDFStatus.test.ts)
- Playwright timing for the PDF poll → toast → download path (typical PDF generation latency)
- Open question: should "Custom" range default the picker to the last 30 days?
</output>
