import { useQuery } from '@tanstack/react-query'
import { Check, ChevronsUpDown, X } from 'lucide-react'
import { useEffect, useState } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import { Input } from '@/components/ui/input'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { cn } from '@/lib/utils'
import { listSites, type Site } from '@/lib/sites'
import type { DevicesSearch } from './search-params'

/**
 * Plan 03-09 §UI-SPEC §Devices list page extended — toolbar row 1.
 *
 *   Search input + Site multi-select combobox + Activation select +
 *   Last-seen ToggleGroup + "Clear filters" ghost.
 *
 * Debounced 250ms on the text search so we don't spam the URL/history while
 * the operator types. Other filters update synchronously.
 *
 * NOTE: this component is presentational on top of the URL state hook —
 * `search` is the parsed DevicesSearch, `onChange` writes the partial. The
 * page owner stays the source of truth.
 */
export interface FilterToolbarProps {
  search: DevicesSearch
  onChange: (next: Partial<DevicesSearch>) => void
  totalShown: number
  totalAll: number
}

const ACTIVATION_OPTIONS: Array<{
  value: 'any' | NonNullable<DevicesSearch['status']>
  label: string
}> = [
  { value: 'any', label: 'Any activation' },
  { value: 'active', label: 'Active' },
  { value: 'inactive', label: 'Inactive' },
  { value: 'never_joined', label: 'Never joined' },
]

const LAST_SEEN_OPTIONS: Array<{
  value: NonNullable<DevicesSearch['last_seen']>
  label: string
}> = [
  { value: '24h', label: '24h' },
  { value: '7d', label: '7d' },
  { value: '30d', label: '30d' },
  { value: 'all', label: 'All' },
]

export function FilterToolbar({
  search,
  onChange,
  totalShown,
  totalAll,
}: FilterToolbarProps) {
  // Debounce the local q so we don't write the URL on every keystroke.
  const [localQ, setLocalQ] = useState(search.q)
  useEffect(() => {
    // Keep local in sync if the URL changes from elsewhere (back/forward).
    setLocalQ(search.q)
  }, [search.q])

  useEffect(() => {
    if (localQ === search.q) return
    const id = setTimeout(() => {
      onChange({ q: localQ, page: 1 })
    }, 250)
    return () => clearTimeout(id)
    // The debounce only depends on the local string + the latest onChange.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [localQ])

  const sitesQuery = useQuery({
    queryKey: ['sites'],
    queryFn: () => listSites(),
  })
  const sites: Site[] = sitesQuery.data ?? []
  const sitesById = new Map(sites.map((s) => [s.id, s]))

  const activeFilterCount =
    search.site.length +
    (search.status ? 1 : 0) +
    (search.last_seen !== 'all' ? 1 : 0) +
    (search.q ? 1 : 0)

  const clearAll = () => {
    onChange({
      site: [],
      status: undefined,
      last_seen: 'all',
      q: '',
      page: 1,
    })
    setLocalQ('')
  }

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-3">
        <Input
          aria-label="Search devices"
          className="max-w-sm"
          placeholder="Search by name or DevEUI"
          value={localQ}
          onChange={(e) => setLocalQ(e.target.value)}
        />

        <SiteMultiSelect
          sites={sites}
          selected={search.site}
          onChange={(next) => onChange({ site: next, page: 1 })}
        />

        <Select
          value={search.status ?? 'any'}
          onValueChange={(v) =>
            onChange({
              status: v === 'any' ? undefined : (v as DevicesSearch['status']),
              page: 1,
            })
          }
        >
          <SelectTrigger className="w-44" aria-label="Activation status">
            <SelectValue placeholder="Any activation" />
          </SelectTrigger>
          <SelectContent>
            {ACTIVATION_OPTIONS.map((opt) => (
              <SelectItem key={opt.value} value={opt.value}>
                {opt.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <ToggleGroup
          type="single"
          variant="outline"
          value={search.last_seen}
          onValueChange={(v) => {
            if (!v) return
            onChange({ last_seen: v as DevicesSearch['last_seen'], page: 1 })
          }}
          aria-label="Last seen window"
        >
          {LAST_SEEN_OPTIONS.map((opt) => (
            <ToggleGroupItem key={opt.value} value={opt.value} aria-label={opt.label}>
              {opt.label}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>

        {activeFilterCount > 0 ? (
          <Button variant="ghost" size="sm" onClick={clearAll} className="ml-auto">
            <X className="mr-1 h-3 w-3" />
            Clear filters
          </Button>
        ) : null}
      </div>

      {activeFilterCount > 0 ? (
        <div className="flex flex-wrap items-center gap-2">
          {search.site.map((id) => {
            const s = sitesById.get(id)
            return (
              <Badge key={id} variant="secondary" className="gap-1">
                Site: {s?.name ?? id.slice(0, 8)}
                <button
                  type="button"
                  aria-label={`Remove site filter ${s?.name ?? id}`}
                  onClick={() =>
                    onChange({
                      site: search.site.filter((x) => x !== id),
                      page: 1,
                    })
                  }
                >
                  <X className="h-3 w-3" />
                </button>
              </Badge>
            )
          })}
          {search.status ? (
            <Badge variant="secondary" className="gap-1">
              Activation: {search.status}
              <button
                type="button"
                aria-label="Remove activation filter"
                onClick={() => onChange({ status: undefined, page: 1 })}
              >
                <X className="h-3 w-3" />
              </button>
            </Badge>
          ) : null}
          {search.last_seen !== 'all' ? (
            <Badge variant="secondary" className="gap-1">
              Last seen: {search.last_seen}
              <button
                type="button"
                aria-label="Remove last seen filter"
                onClick={() => onChange({ last_seen: 'all', page: 1 })}
              >
                <X className="h-3 w-3" />
              </button>
            </Badge>
          ) : null}
          {search.q ? (
            <Badge variant="secondary" className="gap-1">
              Search: {search.q}
              <button
                type="button"
                aria-label="Remove search filter"
                onClick={() => {
                  onChange({ q: '', page: 1 })
                  setLocalQ('')
                }}
              >
                <X className="h-3 w-3" />
              </button>
            </Badge>
          ) : null}
        </div>
      ) : null}

      <p className="text-sm text-muted-foreground">
        Showing <span className="font-semibold">{totalShown}</span> of{' '}
        <span className="font-semibold">{totalAll}</span> devices
      </p>
    </div>
  )
}

function SiteMultiSelect({
  sites,
  selected,
  onChange,
}: {
  sites: Site[]
  selected: string[]
  onChange: (next: string[]) => void
}) {
  const [open, setOpen] = useState(false)
  const label =
    selected.length === 0
      ? 'Filter by site'
      : selected.length === 1
        ? `Site: ${sites.find((s) => s.id === selected[0])?.name ?? '1 selected'}`
        : `Site: ${selected.length} selected`

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          role="combobox"
          aria-label="Site filter"
          className="w-56 justify-between"
        >
          <span className="truncate">{label}</span>
          <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-56 p-0">
        <Command>
          <CommandInput placeholder="Search sites…" />
          <CommandList>
            <CommandEmpty>No sites.</CommandEmpty>
            <CommandGroup>
              {sites
                .filter((s) => !s.archived_at)
                .map((s) => {
                  const isSelected = selected.includes(s.id)
                  return (
                    <CommandItem
                      key={s.id}
                      value={s.name}
                      onSelect={() => {
                        onChange(
                          isSelected
                            ? selected.filter((x) => x !== s.id)
                            : [...selected, s.id],
                        )
                      }}
                    >
                      <Check
                        className={cn(
                          'mr-2 h-4 w-4',
                          isSelected ? 'opacity-100' : 'opacity-0',
                        )}
                      />
                      {s.name}
                    </CommandItem>
                  )
                })}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
