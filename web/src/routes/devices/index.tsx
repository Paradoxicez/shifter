import { useQuery } from '@tanstack/react-query'
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
  type ColumnDef,
  type RowSelectionState,
} from '@tanstack/react-table'
import {
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  ChevronsUpDown,
  ChevronUp,
  Plus,
  Upload,
} from 'lucide-react'
import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { type Device, listDevicesFiltered } from '@/lib/devices'
import { useCurrentUser } from '@/lib/use-current-user'
import { AddDeviceDialog } from './add-device-dialog'
import { BulkActionBar } from './bulk-action-bar'
import { BulkDecommissionDialog } from './bulk-decommission-dialog'
import { BulkImportDialog } from './bulk-import-dialog'
import { FilterToolbar } from './filter-toolbar'
import { useDevicesSearch, type DevicesSearch } from './search-params'

/**
 * Plan 03-09 §UI-SPEC §Devices list page extended (DEV-01, D-12..D-18).
 *
 *   - URL-state filter toolbar (react-router-dom v7 useSearchParams + zod —
 *     NOT TanStack Router; 03-RESEARCH corrected the CONTEXT D-15 wording).
 *   - TanStack Table with manualPagination + manualSorting + manualFiltering.
 *   - Click-to-sort headers cycle asc → desc → default (-last_seen).
 *   - Admin-only row checkbox column + sticky BulkActionBar.
 *   - Bulk Import 3-step dialog launched from header CTA.
 *   - Bulk Decommission AlertDialog launched from BulkActionBar.
 */
export default function DevicesPage() {
  const [createOpen, setCreateOpen] = useState(false)
  const [importOpen, setImportOpen] = useState(false)
  const [bulkDecommissionOpen, setBulkDecommissionOpen] = useState(false)
  const [rowSelection, setRowSelection] = useState<RowSelectionState>({})

  const user = useCurrentUser()
  const isAdmin = user?.role === 'admin'

  const [search, updateSearch] = useDevicesSearch()

  const devicesQuery = useQuery({
    queryKey: ['devices', search],
    queryFn: () => listDevicesFiltered(search),
    // Keep previous data while paginating so the table doesn't flash empty.
    placeholderData: (previous) => previous,
  })

  const rows: Device[] = devicesQuery.data?.rows ?? []
  const totalCount = devicesQuery.data?.total_count ?? 0
  const pageCount = devicesQuery.data?.page_count ?? 0

  const selectedIDs = useMemo(
    () => Object.keys(rowSelection).filter((id) => rowSelection[id]),
    [rowSelection],
  )

  const onSortClick = (col: SortableColumn) => {
    // Cycle: unsorted → asc → desc → unsorted (returns to default -last_seen).
    const asc = col
    const desc = `-${col}` as const
    if (search.sort === asc) {
      updateSearch({ sort: desc })
    } else if (search.sort === desc) {
      updateSearch({ sort: '-last_seen' }) // default
    } else {
      updateSearch({ sort: asc })
    }
  }

  const columns = useMemo<ColumnDef<Device>[]>(() => {
    const cols: ColumnDef<Device>[] = []
    if (isAdmin) {
      cols.push({
        id: 'select',
        header: ({ table }) => (
          <Checkbox
            aria-label="Select all rows on this page"
            checked={
              table.getIsAllPageRowsSelected()
                ? true
                : table.getIsSomePageRowsSelected()
                  ? 'indeterminate'
                  : false
            }
            onCheckedChange={(v) => table.toggleAllPageRowsSelected(!!v)}
          />
        ),
        cell: ({ row }) => (
          <Checkbox
            aria-label={`Select ${row.original.name}`}
            checked={row.getIsSelected()}
            onCheckedChange={(v) => row.toggleSelected(!!v)}
          />
        ),
      })
    }
    cols.push(
      {
        accessorKey: 'name',
        header: () => <SortableHeader label="Name" col="name" search={search} onSort={onSortClick} />,
        cell: ({ row }) => (
          <Link
            to={`/devices/${row.original.id}`}
            className="text-sm font-semibold text-primary hover:underline"
          >
            {row.original.name}
          </Link>
        ),
      },
      {
        accessorKey: 'dev_eui',
        header: () => <SortableHeader label="DevEUI" col="dev_eui" search={search} onSort={onSortClick} />,
        cell: ({ row }) => <span className="text-sm font-mono">{row.original.dev_eui}</span>,
      },
      {
        id: 'site',
        header: () => <SortableHeader label="Site" col="site" search={search} onSort={onSortClick} />,
        cell: ({ row }) =>
          row.original.current_site_id ? (
            <Link
              to={`/sites/${row.original.current_site_id}`}
              className="text-sm text-primary hover:underline"
            >
              {row.original.current_site_name ?? '—'}
            </Link>
          ) : (
            <span className="text-sm text-muted-foreground">—</span>
          ),
      },
      {
        accessorKey: 'last_seen_at',
        header: () => (
          <SortableHeader label="Last seen" col="last_seen" search={search} onSort={onSortClick} />
        ),
        cell: ({ row }) => (
          <span className="text-xs text-muted-foreground">
            {relativeFromNow(row.original.last_seen_at)}
          </span>
        ),
      },
      {
        id: 'activation',
        header: 'Activation',
        cell: ({ row }) => <ActivationChip device={row.original} />,
      },
      {
        id: 'status',
        header: 'Status',
        cell: ({ row }) => (
          <span className="text-sm">
            {row.original.decommissioned_at ? 'Decommissioned' : 'Active'}
          </span>
        ),
      },
    )
    return cols
  }, [isAdmin, search])

  const table = useReactTable({
    data: rows,
    columns,
    state: { rowSelection },
    enableRowSelection: isAdmin,
    onRowSelectionChange: setRowSelection,
    getRowId: (row) => row.id,
    getCoreRowModel: getCoreRowModel(),
    manualPagination: true,
    manualSorting: true,
    manualFiltering: true,
    pageCount,
  })

  const isLoading = devicesQuery.isLoading
  const isEmpty = !isLoading && rows.length === 0
  const hasActiveFilters =
    search.site.length > 0 ||
    !!search.status ||
    search.last_seen !== 'all' ||
    !!search.q

  return (
    <div className="flex flex-col gap-6 p-6">
      <header className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold leading-8">Devices</h1>
        <div className="flex items-center gap-2">
          {isAdmin ? (
            <Button variant="outline" onClick={() => setImportOpen(true)}>
              <Upload className="mr-2 h-4 w-4" />
              Bulk import
            </Button>
          ) : null}
          <Button onClick={() => setCreateOpen(true)}>
            <Plus className="mr-2 h-4 w-4" />
            Add device
          </Button>
        </div>
      </header>

      <FilterToolbar
        search={search}
        onChange={updateSearch}
        totalShown={rows.length}
        totalAll={totalCount}
      />

      {isAdmin ? (
        <BulkActionBar
          selectedIDs={selectedIDs}
          onClearSelection={() => setRowSelection({})}
          onDecommission={() => setBulkDecommissionOpen(true)}
        />
      ) : null}

      {isEmpty ? (
        <div className="flex flex-col items-center gap-4 rounded-md border p-12 text-center">
          {hasActiveFilters ? (
            <>
              <h2 className="text-lg font-semibold">No devices match your filters</h2>
              <p className="text-sm text-muted-foreground">
                Try widening the time window or clearing some filters.
              </p>
              <Button
                variant="outline"
                onClick={() =>
                  updateSearch({
                    site: [],
                    status: undefined,
                    last_seen: 'all',
                    q: '',
                    page: 1,
                  })
                }
              >
                Clear all filters
              </Button>
            </>
          ) : (
            <>
              <h2 className="text-lg font-semibold">No devices yet</h2>
              <p className="text-sm text-muted-foreground">
                Add a device to start receiving telemetry.
              </p>
              <Button onClick={() => setCreateOpen(true)}>
                <Plus className="mr-2 h-4 w-4" />
                Add device
              </Button>
            </>
          )}
        </div>
      ) : (
        <>
          <div className="rounded-md border">
            <Table>
              <TableHeader>
                {table.getHeaderGroups().map((hg) => (
                  <TableRow key={hg.id}>
                    {hg.headers.map((h) => (
                      <TableHead key={h.id}>
                        {flexRender(h.column.columnDef.header, h.getContext())}
                      </TableHead>
                    ))}
                  </TableRow>
                ))}
              </TableHeader>
              <TableBody>
                {isLoading
                  ? Array.from({ length: 5 }).map((_, i) => (
                      <TableRow key={`sk-${i}`}>
                        {columns.map((_c, ci) => (
                          <TableCell key={ci}>
                            <Skeleton className="h-4 w-full" />
                          </TableCell>
                        ))}
                      </TableRow>
                    ))
                  : table.getRowModel().rows.map((row) => (
                      <TableRow
                        key={row.id}
                        data-state={row.getIsSelected() ? 'selected' : undefined}
                      >
                        {row.getVisibleCells().map((c) => (
                          <TableCell key={c.id}>
                            {flexRender(c.column.columnDef.cell, c.getContext())}
                          </TableCell>
                        ))}
                      </TableRow>
                    ))}
              </TableBody>
            </Table>
          </div>

          <PaginationFooter
            search={search}
            onChange={updateSearch}
            pageCount={pageCount}
            totalCount={totalCount}
          />
        </>
      )}

      <AddDeviceDialog open={createOpen} onOpenChange={setCreateOpen} />
      {isAdmin ? (
        <>
          <BulkImportDialog open={importOpen} onOpenChange={setImportOpen} />
          <BulkDecommissionDialog
            open={bulkDecommissionOpen}
            onOpenChange={(v) => {
              setBulkDecommissionOpen(v)
              if (!v) setRowSelection({})
            }}
            deviceIds={selectedIDs}
          />
        </>
      ) : null}
    </div>
  )
}

// ---------- internal pieces -----------------------------------------------

type SortableColumn = 'name' | 'dev_eui' | 'site' | 'last_seen' | 'created_at'

function SortableHeader({
  label,
  col,
  search,
  onSort,
}: {
  label: string
  col: SortableColumn
  search: DevicesSearch
  onSort: (col: SortableColumn) => void
}) {
  const asc = search.sort === col
  const desc = search.sort === `-${col}`
  const Icon = asc ? ChevronUp : desc ? ChevronDown : ChevronsUpDown
  return (
    <button
      type="button"
      onClick={() => onSort(col)}
      className="inline-flex items-center gap-1 text-sm font-semibold hover:text-foreground"
    >
      {label}
      <Icon className="h-3.5 w-3.5 text-muted-foreground" aria-hidden="true" />
    </button>
  )
}

function ActivationChip({ device }: { device: Device }) {
  if (device.decommissioned_at) {
    return (
      <Badge variant="secondary" className="text-xs">
        Decommissioned
      </Badge>
    )
  }
  if (!device.last_seen_at) {
    return (
      <Badge variant="outline" className="text-xs">
        Never joined
      </Badge>
    )
  }
  const t = Date.parse(device.last_seen_at)
  const stale = !Number.isNaN(t) && Date.now() - t > 24 * 60 * 60 * 1000
  return stale ? (
    <Badge variant="outline" className="text-xs">
      Inactive
    </Badge>
  ) : (
    <Badge variant="default" className="text-xs" style={{ backgroundColor: 'var(--success)' }}>
      Active
    </Badge>
  )
}

function PaginationFooter({
  search,
  onChange,
  pageCount,
  totalCount,
}: {
  search: DevicesSearch
  onChange: (next: Partial<DevicesSearch>) => void
  pageCount: number
  totalCount: number
}) {
  const onFirst = search.page <= 1
  const onLast = search.page >= Math.max(1, pageCount)
  return (
    <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
      <p className="text-sm text-muted-foreground">
        Page <span className="font-semibold">{search.page}</span> of{' '}
        <span className="font-semibold">{Math.max(1, pageCount)}</span> ·{' '}
        <span className="font-semibold">{search.per_page}</span> per page ·{' '}
        <span className="font-semibold">{totalCount}</span> total
      </p>
      <div className="flex items-center gap-2">
        <Select
          value={String(search.per_page)}
          onValueChange={(v) =>
            onChange({ per_page: Number(v) as 25 | 50 | 100, page: 1 })
          }
        >
          <SelectTrigger className="w-28" aria-label="Page size">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="25">25 / page</SelectItem>
            <SelectItem value="50">50 / page</SelectItem>
            <SelectItem value="100">100 / page</SelectItem>
          </SelectContent>
        </Select>
        <Button
          variant="outline"
          size="sm"
          disabled={onFirst}
          onClick={() => onChange({ page: search.page - 1 })}
          aria-label="Previous page"
        >
          <ChevronLeft className="h-4 w-4" />
        </Button>
        <Button
          variant="outline"
          size="sm"
          disabled={onLast}
          onClick={() => onChange({ page: search.page + 1 })}
          aria-label="Next page"
        >
          <ChevronRight className="h-4 w-4" />
        </Button>
      </div>
    </div>
  )
}

function relativeFromNow(iso: string | null | undefined): string {
  if (!iso) return '—'
  const t = Date.parse(iso)
  if (Number.isNaN(t)) return '—'
  const diff = Date.now() - t
  const min = Math.floor(diff / 60_000)
  if (min < 1) return 'just now'
  if (min < 60) return `${min} min ago`
  const hr = Math.floor(min / 60)
  if (hr < 24) return `${hr} hr ago`
  const day = Math.floor(hr / 24)
  return `${day} d ago`
}
