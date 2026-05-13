/**
 * CompareView — Plan 07-12 Task 2
 *
 * Surface 5: side-by-side entity comparison (D-16, D-37, D-38).
 *
 * UI-SPEC strings (verbatim):
 *   Page heading:          "Compare"
 *   Mode A:                "Compare entities"
 *   Mode B:                "Compare time ranges"
 *   Entity A placeholder:  "Select entity A"
 *   Entity B placeholder:  "Select entity B"
 *   Swap aria-label:       "Swap entities A and B"
 *   Empty heading:         "Compare two entities"
 *   Empty body:            "Choose entities and a time range to see side-by-side consumption data."
 *   Chart empty (no entity):"Select two entities to compare."
 *   Chart no data:         "No data for this range."
 *   Delta column label:    "Delta"
 *   Chart aria-label:      "Comparison chart for {A name} vs {B name}"
 *   Chart role:            "img"
 *   Mode RadioGroup aria:  "Comparison mode"
 */

import { useMemo, useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { ArrowLeftRight, CalendarIcon, ChevronsUpDown, Check } from 'lucide-react'
import { format } from 'date-fns'
import type { DateRange } from 'react-day-picker'

import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Calendar } from '@/components/ui/calendar'
import { Command, CommandEmpty, CommandInput, CommandItem, CommandList } from '@/components/ui/command'
import {
  ChartContainer,
  ChartTooltip,
  ChartTooltipContent,
} from '@/components/ui/chart'
import { LineChart, Line, XAxis, YAxis, CartesianGrid } from 'recharts'
import { cn } from '@/lib/utils'
import { compareReports } from '@/lib/compare'
import type { CompareRequest, CompareResponse, SeriesResult } from '@/lib/compare'
import { listMPs } from '@/lib/metering-points'
import { listSites } from '@/lib/sites'

// ---- types ------------------------------------------------------------------

type Mode = 'entities' | 'time_ranges'
type EntityType = 'site' | 'metering_point'

interface EntityOption {
  id: string
  label: string
}

// ---- EntityDropdown ---------------------------------------------------------

interface EntityDropdownProps {
  value: string | null
  onChange: (id: string | null) => void
  placeholder: string
  options: EntityOption[]
  isLoading?: boolean
}

function EntityDropdown({ value, onChange, placeholder, options, isLoading }: EntityDropdownProps) {
  const [open, setOpen] = useState(false)
  const selected = options.find((o) => o.id === value)

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          role="combobox"
          aria-expanded={open}
          className="w-full justify-between font-normal"
          disabled={isLoading}
        >
          {isLoading ? 'Loading…' : selected ? selected.label : placeholder}
          <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="p-0 w-[--radix-popover-trigger-width]">
        <Command>
          <CommandInput placeholder="Search…" />
          <CommandList>
            <CommandEmpty>No results found.</CommandEmpty>
            {options.map((opt) => (
              <CommandItem
                key={opt.id}
                value={opt.label}
                onSelect={() => {
                  onChange(opt.id === value ? null : opt.id)
                  setOpen(false)
                }}
              >
                <Check
                  className={cn('mr-2 h-4 w-4', value === opt.id ? 'opacity-100' : 'opacity-0')}
                />
                {opt.label}
              </CommandItem>
            ))}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}

// ---- DateRangePickerInline --------------------------------------------------

interface DateRangePickerProps {
  value: { from: string; to: string } | null
  onChange: (range: { from: string; to: string } | null) => void
  label?: string
}

function DateRangePicker({ value, onChange, label }: DateRangePickerProps) {
  const [open, setOpen] = useState(false)
  const [pending, setPending] = useState<DateRange | undefined>(
    value ? { from: new Date(value.from), to: new Date(value.to) } : undefined,
  )

  const btnLabel =
    value
      ? `${format(new Date(value.from), 'MMM d')} – ${format(new Date(value.to), 'MMM d, yyyy')}`
      : label ?? 'Pick a date range'

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button variant="outline" size="sm" className="gap-1.5" type="button">
          <CalendarIcon className="h-3.5 w-3.5" />
          {btnLabel}
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-auto p-0" align="start">
        <Calendar mode="range" selected={pending} onSelect={setPending} numberOfMonths={2} />
        <div className="flex justify-end gap-2 p-2 border-t">
          <Button variant="ghost" size="sm" onClick={() => setOpen(false)}>
            Cancel
          </Button>
          <Button
            size="sm"
            disabled={!pending?.from || !pending?.to}
            onClick={() => {
              if (pending?.from && pending?.to) {
                onChange({
                  from: pending.from.toISOString(),
                  to: pending.to.toISOString(),
                })
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

// ---- DeltaCell --------------------------------------------------------------

function DeltaCell({ delta }: { delta: number }) {
  const sign = delta > 0 ? '+' : delta < 0 ? '' : ''
  return (
    <span className={cn('font-mono', delta > 0 ? 'text-green-600' : delta < 0 ? 'text-red-600' : '')}>
      {sign}{delta.toFixed(2)}
    </span>
  )
}

// ---- formatSeriesLabel ------------------------------------------------------

function seriesLabel(result: SeriesResult): string {
  // Strip the "entity_type:uuid" prefix returned by the backend and return a
  // short UUID display. In production this will be replaced by the entity name
  // once the backend resolves labels; for now the UUID is surfaced as-is.
  return result.label
}

// ---- CompareView (export) ---------------------------------------------------

export function CompareView() {
  const [mode, setMode] = useState<Mode>('entities')
  const [entityType] = useState<EntityType>('metering_point')

  // entities mode state
  const [entityA, setEntityA] = useState<string | null>(null)
  const [entityB, setEntityB] = useState<string | null>(null)
  const [range, setRange] = useState<{ from: string; to: string } | null>(null)

  // time_ranges mode state
  const [singleEntity, setSingleEntity] = useState<string | null>(null)
  const [rangeA, setRangeA] = useState<{ from: string; to: string } | null>(null)
  const [rangeB, setRangeB] = useState<{ from: string; to: string } | null>(null)

  const mutation = useMutation<CompareResponse, Error, CompareRequest>({
    mutationFn: compareReports,
  })

  // Build and fire the compare request.
  function handleCompare() {
    if (mode === 'entities') {
      if (!entityA || !entityB || !range) return
      mutation.mutate({
        mode: 'entities',
        entity_type: entityType,
        entity_a_id: entityA,
        entity_b_id: entityB,
        range,
      })
    } else {
      if (!singleEntity || !rangeA || !rangeB) return
      mutation.mutate({
        mode: 'time_ranges',
        entity_type: entityType,
        entity_id: singleEntity,
        range_a: rangeA,
        range_b: rangeB,
      })
    }
  }

  const mpsQuery = useQuery({ queryKey: ['metering-points'], queryFn: () => listMPs() })
  const sitesQuery = useQuery({ queryKey: ['sites'], queryFn: () => listSites() })

  const isLoadingOptions = mpsQuery.isPending || sitesQuery.isPending

  const entityOptions: EntityOption[] = useMemo(() => {
    const sites = sitesQuery.data ?? []
    const mps = mpsQuery.data ?? []
    const siteNameById = new Map(sites.map((s) => [s.id, s.name]))
    return mps.map((mp) => {
      const siteName = siteNameById.get(mp.site_id)
      return {
        id: mp.id,
        label: siteName ? `${mp.name} — ${siteName}` : mp.name,
      }
    })
  }, [mpsQuery.data, sitesQuery.data])

  // Determine if the "run" action is ready.
  const canCompare =
    mode === 'entities'
      ? Boolean(entityA && entityB && range)
      : Boolean(singleEntity && rangeA && rangeB)

  // Derive whether we have data to show.
  const data = mutation.data

  // Chart config for shadcn ChartContainer.
  const chartConfig = {
    a: { label: data ? seriesLabel(data.a) : 'Entity A', color: 'hsl(var(--primary))' },
    b: { label: data ? seriesLabel(data.b) : 'Entity B', color: 'hsl(var(--info, 200 80% 50%))' },
  }

  // Merged series for recharts: join by bucket.
  const mergedSeries = (() => {
    if (!data) return []
    const map = new Map<string, { bucket: string; a?: number; b?: number }>()
    for (const pt of data.a.series) {
      map.set(pt.bucket, { bucket: pt.bucket, a: pt.value })
    }
    for (const pt of data.b.series) {
      const existing = map.get(pt.bucket) ?? { bucket: pt.bucket }
      map.set(pt.bucket, { ...existing, b: pt.value })
    }
    return Array.from(map.values()).sort((x, y) => x.bucket.localeCompare(y.bucket))
  })()

  return (
    <div className="flex flex-col gap-6 p-6">
      <h1 className="text-2xl font-semibold">Compare</h1>

      {/* Mode toggle */}
      <RadioGroup
        value={mode}
        onValueChange={(v) => setMode(v as Mode)}
        aria-label="Comparison mode"
        className="flex flex-row gap-4"
      >
        <div className="flex items-center gap-2">
          <RadioGroupItem value="entities" id="mode-entities" />
          <Label htmlFor="mode-entities">Compare entities</Label>
        </div>
        <div className="flex items-center gap-2">
          <RadioGroupItem value="time_ranges" id="mode-time-ranges" />
          <Label htmlFor="mode-time-ranges">Compare time ranges</Label>
        </div>
      </RadioGroup>

      {/* Entity / range controls */}
      {mode === 'entities' ? (
        <div className="flex flex-col sm:flex-row gap-2 items-start sm:items-center">
          <div className="flex-1 min-w-0">
            <EntityDropdown
              value={entityA}
              onChange={setEntityA}
              placeholder="Select entity A"
              options={entityOptions}
              isLoading={isLoadingOptions}
            />
          </div>
          <Button
            variant="ghost"
            size="icon"
            aria-label="Swap entities A and B"
            onClick={() => {
              const tmp = entityA
              setEntityA(entityB)
              setEntityB(tmp)
            }}
          >
            <ArrowLeftRight className="h-4 w-4" />
          </Button>
          <div className="flex-1 min-w-0">
            <EntityDropdown
              value={entityB}
              onChange={setEntityB}
              placeholder="Select entity B"
              options={entityOptions}
              isLoading={isLoadingOptions}
            />
          </div>
          <DateRangePicker value={range} onChange={setRange} label="Pick a date range" />
        </div>
      ) : (
        <div className="flex flex-col gap-3">
          <div className="w-full sm:w-64">
            <EntityDropdown
              value={singleEntity}
              onChange={setSingleEntity}
              placeholder="Select entity A"
              options={entityOptions}
              isLoading={isLoadingOptions}
            />
          </div>
          <div className="flex flex-col sm:flex-row gap-2">
            <DateRangePicker value={rangeA} onChange={setRangeA} label="Range A" />
            <DateRangePicker value={rangeB} onChange={setRangeB} label="Range B" />
          </div>
        </div>
      )}

      {/* Run button */}
      <Button
        onClick={handleCompare}
        disabled={!canCompare || mutation.isPending}
        className="self-start"
      >
        {mutation.isPending ? 'Comparing…' : 'Compare'}
      </Button>

      {/* Chart area */}
      {data ? (
        <div
          aria-label={`Comparison chart for ${seriesLabel(data.a)} vs ${seriesLabel(data.b)}`}
          role="img"
        >
          <ChartContainer config={chartConfig} className="h-64">
            <LineChart data={mergedSeries} margin={{ top: 8, right: 8, left: 0, bottom: 0 }}>
              <CartesianGrid strokeDasharray="3 3" />
              <XAxis dataKey="bucket" tick={{ fontSize: 11 }} />
              <YAxis tick={{ fontSize: 11 }} />
              <ChartTooltip content={<ChartTooltipContent />} />
              <Line
                type="monotone"
                dataKey="a"
                name={chartConfig.a.label as string}
                stroke={chartConfig.a.color}
                dot={false}
              />
              <Line
                type="monotone"
                dataKey="b"
                name={chartConfig.b.label as string}
                stroke={chartConfig.b.color}
                dot={false}
              />
            </LineChart>
          </ChartContainer>
        </div>
      ) : mutation.isPending ? null : (
        (() => {
          const noEntitiesChosen =
            mode === 'entities' ? !(entityA && entityB) : !singleEntity
          return (
            <p className="text-sm text-muted-foreground">
              {noEntitiesChosen
                ? 'Select two entities to compare.'
                : 'No data for this range.'}
            </p>
          )
        })()
      )}

      {/* Empty state — shown before first compare */}
      {!data && !mutation.isPending && (
        <div className="flex flex-col items-center gap-2 py-12 text-center">
          <h2 className="text-lg font-semibold">Compare two entities</h2>
          <p className="text-sm text-muted-foreground max-w-md">
            Choose entities and a time range to see side-by-side consumption data.
          </p>
        </div>
      )}

      {/* Results table */}
      {data && (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Entity</TableHead>
              <TableHead className="text-right">Total</TableHead>
              <TableHead className="text-right">Average</TableHead>
              <TableHead className="text-right">Peak</TableHead>
              <TableHead className="text-right">Delta</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow>
              <TableCell className="font-medium">{seriesLabel(data.a)}</TableCell>
              <TableCell className="text-right">{data.a.total.toFixed(2)}</TableCell>
              <TableCell className="text-right">{data.a.average.toFixed(2)}</TableCell>
              <TableCell className="text-right">{data.a.peak.toFixed(2)}</TableCell>
              <TableCell className="text-right">
                <DeltaCell delta={data.delta.total} />
              </TableCell>
            </TableRow>
            <TableRow>
              <TableCell className="font-medium">{seriesLabel(data.b)}</TableCell>
              <TableCell className="text-right">{data.b.total.toFixed(2)}</TableCell>
              <TableCell className="text-right">{data.b.average.toFixed(2)}</TableCell>
              <TableCell className="text-right">{data.b.peak.toFixed(2)}</TableCell>
              <TableCell className="text-right">
                <DeltaCell delta={-data.delta.total} />
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      )}
    </div>
  )
}
