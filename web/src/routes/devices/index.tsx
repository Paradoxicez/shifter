import { useQuery } from '@tanstack/react-query'
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
  type ColumnDef,
} from '@tanstack/react-table'
import { Plus } from 'lucide-react'
import { useMemo, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { type Device, listDevices } from '@/lib/devices'
import { AddDeviceDialog } from './add-device-dialog'

/**
 * UI-SPEC §Devices list page (D-29) — minimal Phase 2 surface.
 *
 *   - Page header: "Devices" + "Add device" button.
 *   - Toolbar: Search input only (Phase 3 owns full filtering / DEV-01).
 *   - Table: TanStack Table — Name, DevEUI mono, Profile, Bound MP, Last seen,
 *     Battery + dot, Status. Default sort last_seen desc, page size 50.
 *   - Empty state: "No devices yet" + Add device CTA.
 */
export default function DevicesPage() {
  const [search, setSearch] = useState('')
  const [createOpen, setCreateOpen] = useState(false)

  const devicesQuery = useQuery({
    queryKey: ['devices'],
    queryFn: () => listDevices(),
  })

  const filtered = useMemo(() => {
    const rows = devicesQuery.data ?? []
    if (!search) return rows
    const needle = search.toLowerCase()
    return rows.filter(
      (d) =>
        d.name.toLowerCase().includes(needle) || d.dev_eui.toLowerCase().includes(needle),
    )
  }, [devicesQuery.data, search])

  const columns: ColumnDef<Device>[] = [
    {
      accessorKey: 'name',
      header: 'Name',
      cell: ({ row }) => <span className="text-sm font-medium">{row.original.name}</span>,
    },
    {
      accessorKey: 'dev_eui',
      header: 'DevEUI',
      cell: ({ row }) => <span className="text-sm font-mono">{row.original.dev_eui}</span>,
    },
    {
      id: 'profile',
      header: 'Profile',
      cell: () => <span className="text-sm text-muted-foreground">—</span>,
    },
    {
      id: 'bound_mp',
      header: 'Bound MP',
      cell: () => <span className="text-sm text-muted-foreground">—</span>,
    },
    {
      accessorKey: 'last_seen_at',
      header: 'Last seen',
      cell: ({ row }) => (
        <span className="text-xs text-muted-foreground">
          {row.original.last_seen_at ?? '—'}
        </span>
      ),
    },
    {
      id: 'battery',
      header: 'Battery',
      cell: () => <span className="text-sm text-muted-foreground">—</span>,
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
  ]

  const table = useReactTable({
    data: filtered,
    columns,
    getCoreRowModel: getCoreRowModel(),
  })

  const isLoading = devicesQuery.isLoading
  const isEmpty = !isLoading && filtered.length === 0 && !search

  return (
    <div className="flex flex-col gap-6 p-6">
      <header className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold leading-8">Devices</h1>
        <Button onClick={() => setCreateOpen(true)}>
          <Plus className="mr-2 h-4 w-4" />
          Add device
        </Button>
      </header>

      <div className="flex items-center gap-4">
        <Input
          className="max-w-sm"
          placeholder="Search by name or DevEUI"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
      </div>

      {isEmpty ? (
        <div className="flex flex-col items-center gap-4 rounded-md border p-12 text-center">
          <h2 className="text-lg font-semibold">No devices yet</h2>
          <p className="text-sm text-muted-foreground">
            Add a device to start receiving telemetry.
          </p>
          <Button onClick={() => setCreateOpen(true)}>
            <Plus className="mr-2 h-4 w-4" />
            Add device
          </Button>
        </div>
      ) : (
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
                    <TableRow key={row.id}>
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
      )}

      <AddDeviceDialog open={createOpen} onOpenChange={setCreateOpen} />
    </div>
  )
}
