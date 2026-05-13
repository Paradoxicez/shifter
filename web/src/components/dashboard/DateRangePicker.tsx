/**
 * DateRangePicker — Plan 04-08 Task 1
 *
 * Segmented preset picker (Today / 24h / 7d / 30d) + Custom popover with
 * shadcn Calendar in range mode.
 *
 * mode='shared-url': reads/writes ?range, ?start, ?end via useSearchParams.
 * mode='controlled': parent owns state via value/onChange props.
 *
 * T-04-08-01: zod `.catch('today')` fallback on URL parse.
 * T-04-08-02: computeRangeWindow throws for custom range >1y; commit button
 *             is disabled with tooltip in that case.
 */

import { useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import type { DateRange } from 'react-day-picker'
import { z } from 'zod'
import { CalendarIcon } from 'lucide-react'
import { format } from 'date-fns'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Calendar } from '@/components/ui/calendar'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import { computeRangeWindow, type RangePreset } from '@/lib/dateRange'

// ---------------------------------------------------------------------------
// Schema + constants
// ---------------------------------------------------------------------------

const rangeSchema = z.enum(['today', '24h', '7d', '30d', 'custom']).catch('today')

const PRESETS: { value: RangePreset; label: string }[] = [
  { value: 'today', label: 'Today' },
  { value: '24h', label: '24h' },
  { value: '7d', label: '7d' },
  { value: '30d', label: '30d' },
]

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

export interface DateRangeValue {
  range: RangePreset
  start?: string
  end?: string
}

export interface DateRangePickerProps {
  /** 'shared-url': reads/writes useSearchParams. 'controlled': parent owns state. */
  mode: 'shared-url' | 'controlled'
  value?: DateRangeValue
  onChange?: (next: DateRangeValue) => void
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function DateRangePicker({ mode, value, onChange }: DateRangePickerProps) {
  const [params, setParams] = useSearchParams()

  // Derive current state from URL or controlled prop
  const currentPreset: RangePreset =
    mode === 'shared-url'
      ? rangeSchema.parse(params.get('range') ?? 'today')
      : (value?.range ?? 'today')
  const currentStart =
    mode === 'shared-url' ? (params.get('start') ?? undefined) : value?.start
  const currentEnd =
    mode === 'shared-url' ? (params.get('end') ?? undefined) : value?.end

  // Local state for Custom popover
  const [popoverOpen, setPopoverOpen] = useState(false)
  const [pendingRange, setPendingRange] = useState<DateRange | undefined>(
    currentPreset === 'custom' && currentStart && currentEnd
      ? { from: new Date(currentStart), to: new Date(currentEnd) }
      : undefined
  )

  // Validate the pending custom range
  let customError: string | null = null
  if (pendingRange?.from && pendingRange?.to) {
    try {
      computeRangeWindow('custom', pendingRange.from.toISOString(), pendingRange.to.toISOString())
    } catch (e) {
      customError = 'Pick a range between 1 hour and 1 year.'
    }
  }

  // ---------------------------------------------------------------------------
  // Handlers
  // ---------------------------------------------------------------------------

  function writeState(next: DateRangeValue) {
    if (mode === 'shared-url') {
      setParams(
        (prev) => {
          const sp = new URLSearchParams(prev)
          sp.set('range', next.range)
          if (next.range === 'custom' && next.start && next.end) {
            sp.set('start', next.start)
            sp.set('end', next.end)
          } else {
            sp.delete('start')
            sp.delete('end')
          }
          return sp
        },
        { replace: false }
      )
    } else {
      onChange?.(next)
    }
  }

  function handlePresetClick(preset: RangePreset) {
    writeState({ range: preset })
    setPopoverOpen(false)
  }

  function handleCustomCommit() {
    if (!pendingRange?.from || !pendingRange?.to || customError) return
    writeState({
      range: 'custom',
      start: pendingRange.from.toISOString(),
      end: pendingRange.to.toISOString(),
    })
    setPopoverOpen(false)
  }

  // ---------------------------------------------------------------------------
  // Render
  // ---------------------------------------------------------------------------

  const customLabel =
    currentPreset === 'custom' && currentStart && currentEnd
      ? `${format(new Date(currentStart), 'MMM d')} – ${format(new Date(currentEnd), 'MMM d, yyyy')}`
      : 'Custom'

  return (
    <div className="flex flex-wrap items-center gap-1">
      {/* Preset buttons */}
      {PRESETS.map(({ value: preset, label }) => (
        <Button
          key={preset}
          variant="ghost"
          size="sm"
          data-state={currentPreset === preset ? 'active' : 'inactive'}
          className="data-[state=active]:bg-accent data-[state=active]:text-accent-foreground"
          onClick={() => handlePresetClick(preset)}
        >
          {label}
        </Button>
      ))}

      {/* Custom popover */}
      <Popover open={popoverOpen} onOpenChange={setPopoverOpen}>
        <PopoverTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            data-state={currentPreset === 'custom' ? 'active' : 'inactive'}
            className="gap-1.5 data-[state=active]:bg-accent data-[state=active]:text-accent-foreground"
          >
            <CalendarIcon className="h-3.5 w-3.5" />
            {customLabel}
          </Button>
        </PopoverTrigger>
        <PopoverContent className="w-auto p-0" align="end">
          <div className="p-3">
            <Calendar
              mode="range"
              selected={pendingRange}
              onSelect={setPendingRange}
              numberOfMonths={2}
              disabled={{ after: new Date() }}
            />
            <div className="flex items-center justify-end gap-2 border-t pt-3 mt-3">
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setPopoverOpen(false)}
              >
                Cancel
              </Button>
              {customError ? (
                <TooltipProvider>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <span>
                        <Button size="sm" disabled>
                          Apply
                        </Button>
                      </span>
                    </TooltipTrigger>
                    <TooltipContent>
                      <p>{customError}</p>
                    </TooltipContent>
                  </Tooltip>
                </TooltipProvider>
              ) : (
                <Button
                  size="sm"
                  disabled={!pendingRange?.from || !pendingRange?.to}
                  onClick={handleCustomCommit}
                >
                  Apply
                </Button>
              )}
            </div>
          </div>
        </PopoverContent>
      </Popover>
    </div>
  )
}
