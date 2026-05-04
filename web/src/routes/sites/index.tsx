import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
  type ColumnDef,
} from '@tanstack/react-table'
import { Archive, MoreHorizontal, Pencil, Plus } from 'lucide-react'
import { useState } from 'react'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
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
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { archiveSite, listSites, type Site } from '@/lib/sites'
import { CreateSiteDialog } from './create-site-dialog'

/**
 * UI-SPEC §Sites list page:
 *   - Page header: text-2xl "Sites" + Plus-leading "Add site" button.
 *   - Toolbar: Search input (max-w-sm) + Tabs [All]/[Archived].
 *   - Table: TanStack Table — Name (link), Address, Metering points (count),
 *     Last uplink (relative), Status, Actions.
 *   - Empty state: "No sites yet" + body + "Add site" CTA.
 *   - Loading: 5 Skeleton rows.
 */
export default function SitesPage() {
  const qc = useQueryClient()
  const [tab, setTab] = useState<'all' | 'archived'>('all')
  const [search, setSearch] = useState('')
  const [createOpen, setCreateOpen] = useState(false)

  const sitesQuery = useQuery({
    queryKey: ['sites', tab],
    queryFn: () => listSites({ archived: tab === 'archived' }),
  })

  const filtered = (sitesQuery.data ?? []).filter((s) =>
    search ? s.name.toLowerCase().includes(search.toLowerCase()) : true,
  )

  const onArchive = async (site: Site) => {
    try {
      await archiveSite(site.id)
      qc.invalidateQueries({ queryKey: ['sites'] })
      toast.success('Site archived')
    } catch {
      toast.error('Could not archive site')
    }
  }

  const columns: ColumnDef<Site>[] = [
    {
      accessorKey: 'name',
      header: 'Name',
      cell: ({ row }) => (
        <Link
          to={`/sites/${row.original.id}`}
          className="font-medium text-primary hover:underline"
        >
          {row.original.name}
        </Link>
      ),
    },
    {
      accessorKey: 'address',
      header: 'Address',
      cell: ({ row }) => (
        <span className="text-sm text-muted-foreground">
          {row.original.address ?? '—'}
        </span>
      ),
    },
    {
      // Metering point count requires a future backend enhancement; render
      // "—" placeholder for now (UI-SPEC anticipates this is a count column).
      id: 'mp_count',
      header: 'Metering points',
      cell: () => <span className="text-sm text-muted-foreground">—</span>,
    },
    {
      id: 'last_uplink',
      header: 'Last uplink',
      cell: () => <span className="text-xs text-muted-foreground">—</span>,
    },
    {
      id: 'status',
      header: 'Status',
      cell: ({ row }) => (
        <span className="text-sm">
          {row.original.archived_at ? 'Archived' : 'Active'}
        </span>
      ),
    },
    {
      id: 'actions',
      header: '',
      cell: ({ row }) => (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="sm" aria-label="Row actions">
              <MoreHorizontal className="h-4 w-4" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem asChild>
              <Link to={`/sites/${row.original.id}`}>
                <Pencil className="mr-2 h-4 w-4" />
                Edit
              </Link>
            </DropdownMenuItem>
            {!row.original.archived_at ? (
              <DropdownMenuItem onClick={() => onArchive(row.original)}>
                <Archive className="mr-2 h-4 w-4" />
                Archive
              </DropdownMenuItem>
            ) : null}
          </DropdownMenuContent>
        </DropdownMenu>
      ),
    },
  ]

  const table = useReactTable({
    data: filtered,
    columns,
    getCoreRowModel: getCoreRowModel(),
  })

  const isLoading = sitesQuery.isLoading
  const isEmpty = !isLoading && filtered.length === 0 && !search

  return (
    <div className="flex flex-col gap-6 p-6">
      <header className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold leading-8">Sites</h1>
        <Button onClick={() => setCreateOpen(true)}>
          <Plus className="mr-2 h-4 w-4" /> Add site
        </Button>
      </header>

      <div className="flex items-center gap-4">
        <Input
          className="max-w-sm"
          placeholder="Search sites by name"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
        />
        <Tabs value={tab} onValueChange={(v) => setTab(v as 'all' | 'archived')}>
          <TabsList>
            <TabsTrigger value="all">All</TabsTrigger>
            <TabsTrigger value="archived">Archived</TabsTrigger>
          </TabsList>
        </Tabs>
      </div>

      {isEmpty ? (
        <div className="flex flex-col items-center gap-4 rounded-md border p-12 text-center">
          <h2 className="text-lg font-semibold">No sites yet</h2>
          <p className="text-sm text-muted-foreground">
            Create a site so you can attach metering points and devices.
          </p>
          <Button onClick={() => setCreateOpen(true)}>
            <Plus className="mr-2 h-4 w-4" /> Add site
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

      <CreateSiteDialog open={createOpen} onOpenChange={setCreateOpen} />
    </div>
  )
}
