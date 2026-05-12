/**
 * ReportConfigPanel — Plan 05-09 Task 1
 *
 * Scope radio (All meters / Single site / Single meter) + conditional pickers
 * + range preset buttons + inline calendar for custom range + Generate button.
 *
 * UI-SPEC: §Report Layout Spec, §Copywriting Contract
 * D-05: Three-radio scope model with conditional secondary picker.
 */

import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Loader2, CalendarIcon } from 'lucide-react'
import { format } from 'date-fns'
import type { DateRange } from 'react-day-picker'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Calendar } from '@/components/ui/calendar'
import { Label } from '@/components/ui/label'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { MeterCombobox } from './MeterCombobox'
import { apiFetch } from '@/lib/api'
import type { ReportConfig } from './index'
import type { ReportGenerateRequest } from './useReportGenerate'

// ---------------------------------------------------------------------------
// CustomDateRangePicker — inline popover calendar for custom range
// ---------------------------------------------------------------------------

interface CustomDateRangePickerProps {
  start?: string
  end?: string
  onApply: (start: string, end: string) => void
}

function CustomDateRangePicker({ start, end, onApply }: CustomDateRangePickerProps) {
  const [open, setOpen] = useState(false)
  const [pending, setPending] = useState<DateRange | undefined>(
    start && end ? { from: new Date(start), to: new Date(end) } : undefined
  )

  const label =
    start && end
      ? `${format(new Date(start), 'MMM d')} – ${format(new Date(end), 'MMM d, yyyy')}`
      : 'Pick a date range'

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm" className="gap-1.5" type="button">
          <CalendarIcon className="h-3.5 w-3.5" />
          {label}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-auto p-0" align="start">
        <Calendar
          mode="range"
          selected={pending}
          onSelect={setPending}
          numberOfMonths={2}
        />
        <div className="flex justify-end gap-2 p-2 border-t">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setOpen(false)}
          >
            Cancel
          </Button>
          <Button
            size="sm"
            disabled={!pending?.from || !pending?.to}
            onClick={() => {
              if (pending?.from && pending?.to) {
                onApply(pending.from.toISOString(), pending.to.toISOString())
                setOpen(false)
              }
            }}
          >
            Apply
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  )
}

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface ReportConfigPanelProps {
  cfg: ReportConfig
  onChange: (next: ReportConfig) => void
  onGenerate: (body: ReportGenerateRequest) => void
  isPending: boolean
}

interface Site {
  id: string
  name: string
}

interface MeteringPoint {
  id: string
  name: string
  site_name: string
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function ReportConfigPanel({ cfg, onChange, onGenerate, isPending }: ReportConfigPanelProps) {
  const { data: sites = [] } = useQuery<Site[]>({
    queryKey: ['sites'],
    queryFn: () => apiFetch<Site[]>('/api/sites'),
  })

  const { data: meters = [] } = useQuery<MeteringPoint[]>({
    queryKey: ['metering-points'],
    queryFn: () => apiFetch<MeteringPoint[]>('/api/metering-points'),
    enabled: cfg.scope === 'meter',
  })

  function handleGenerate() {
    const body: ReportGenerateRequest = {
      scope: cfg.scope,
      range: cfg.range,
    }
    if (cfg.scope === 'site' && cfg.site_id) body.site_id = cfg.site_id
    if (cfg.scope === 'meter' && cfg.mp_id) body.mp_id = cfg.mp_id
    if (cfg.scope === 'all' && cfg.group && cfg.group !== 'none') body.group = cfg.group
    if (cfg.range === 'custom') {
      if (cfg.start) body.start = cfg.start
      if (cfg.end) body.end = cfg.end
    }
    onGenerate(body)
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Configure report</CardTitle>
      </CardHeader>
      <CardContent className="space-y-6">

        {/* Scope picker */}
        <div className="space-y-2">
          <Label className="text-xs font-semibold tracking-wide uppercase">Scope</Label>
          <RadioGroup
            value={cfg.scope}
            onValueChange={(v) => onChange({ ...cfg, scope: v as ReportConfig['scope'] })}
          >
            <div className="flex items-center gap-2">
              <RadioGroupItem value="all" id="scope-all" />
              <Label htmlFor="scope-all">All meters</Label>
            </div>
            <div className="flex items-center gap-2">
              <RadioGroupItem value="site" id="scope-site" />
              <Label htmlFor="scope-site">Single site</Label>
            </div>
            <div className="flex items-center gap-2">
              <RadioGroupItem value="meter" id="scope-meter" />
              <Label htmlFor="scope-meter">Single meter</Label>
            </div>
          </RadioGroup>
        </div>

        {/* Group by — visible when scope=all */}
        {cfg.scope === 'all' && (
          <div className="space-y-2">
            <Label htmlFor="group-by">Group by</Label>
            <Select
              value={cfg.group ?? 'none'}
              onValueChange={(v) => onChange({ ...cfg, group: v as ReportConfig['group'] })}
            >
              <SelectTrigger id="group-by">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="none">None (fleet totals)</SelectItem>
                <SelectItem value="site">Site</SelectItem>
                <SelectItem value="category">Utility class</SelectItem>
              </SelectContent>
            </Select>
          </div>
        )}

        {/* Site picker — visible when scope=site */}
        {cfg.scope === 'site' && (
          <div className="space-y-2">
            <Label htmlFor="site-picker">Site</Label>
            <Select
              value={cfg.site_id ?? ''}
              onValueChange={(v) => onChange({ ...cfg, site_id: v })}
            >
              <SelectTrigger id="site-picker">
                <SelectValue placeholder="Select a site" />
              </SelectTrigger>
              <SelectContent>
                {sites.map((s) => (
                  <SelectItem key={s.id} value={s.id}>
                    {s.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}

        {/* Meter combobox — visible when scope=meter */}
        {cfg.scope === 'meter' && (
          <div className="space-y-2">
            <Label>Meter</Label>
            <MeterCombobox
              meters={meters}
              value={cfg.mp_id ?? ''}
              onChange={(id) => onChange({ ...cfg, mp_id: id })}
            />
          </div>
        )}

        {/* Range presets */}
        <div className="space-y-2">
          <Label>Range</Label>
          <div className="flex gap-2 flex-wrap">
            {(['daily', 'monthly', 'yearly', 'custom'] as const).map((r) => (
              <Button
                key={r}
                variant={cfg.range === r ? 'default' : 'outline'}
                size="sm"
                type="button"
                onClick={() => onChange({ ...cfg, range: r })}
              >
                {r.charAt(0).toUpperCase() + r.slice(1)}
              </Button>
            ))}
          </div>

          {/* Custom date range picker */}
          {cfg.range === 'custom' && (
            <CustomDateRangePicker
              start={cfg.start}
              end={cfg.end}
              onApply={(start, end) => onChange({ ...cfg, start, end })}
            />
          )}
        </div>

        {/* Generate button */}
        <Button
          className="w-full"
          onClick={handleGenerate}
          disabled={isPending}
        >
          {isPending && (
            <Loader2 className="h-4 w-4 mr-2 animate-spin motion-reduce:animate-none" />
          )}
          {isPending ? 'Generating…' : 'Generate report'}
        </Button>
      </CardContent>
    </Card>
  )
}
