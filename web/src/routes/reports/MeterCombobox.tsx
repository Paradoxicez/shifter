/**
 * MeterCombobox — Plan 05-09 Task 1
 *
 * Light Command/Popover wrapper for searching and selecting a metering point.
 * Used in ReportConfigPanel when scope='meter'.
 */

import { useState } from 'react'
import { Command, CommandInput, CommandItem, CommandList, CommandEmpty } from '@/components/ui/command'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Button } from '@/components/ui/button'
import { ChevronsUpDown, Check } from 'lucide-react'
import { cn } from '@/lib/utils'

interface MeterOption {
  id: string
  name: string
  site_name: string
}

interface MeterComboboxProps {
  meters: MeterOption[]
  value: string
  onChange: (id: string) => void
}

export function MeterCombobox({ meters, value, onChange }: MeterComboboxProps) {
  const [open, setOpen] = useState(false)
  const selected = meters.find((m) => m.id === value)

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          role="combobox"
          aria-expanded={open}
          className="w-full justify-between font-normal"
        >
          {selected ? `${selected.name} — ${selected.site_name}` : 'Select a meter'}
          <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="p-0 w-[--radix-popover-trigger-width]">
        <Command>
          <CommandInput placeholder="Search meters…" />
          <CommandList>
            <CommandEmpty>No meters found.</CommandEmpty>
            {meters.map((m) => (
              <CommandItem
                key={m.id}
                value={`${m.name} ${m.site_name}`}
                onSelect={() => {
                  onChange(m.id)
                  setOpen(false)
                }}
              >
                <Check className={cn('mr-2 h-4 w-4', value === m.id ? 'opacity-100' : 'opacity-0')} />
                {m.name}
                <span className="text-muted-foreground ml-2">— {m.site_name}</span>
              </CommandItem>
            ))}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
