import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
  type ColumnDef,
} from '@tanstack/react-table'
import { Archive, Download, MoreHorizontal, Pencil, Plus } from 'lucide-react'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ImportFromCatalogDialog } from './ImportFromCatalogDialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { archiveProfile, listProfiles, type Profile } from '@/lib/profiles'

/**
 * UI-SPEC §Profiles list page.
 *
 *   Page header: "Device profiles" + primary "Create profile" → /profiles/new.
 *   Table:       Name, Vendor, Family, Capabilities chips (max 3 + "+N more"),
 *                CS sync status row, Last updated, Actions overflow.
 *   Default seed rows render visually identical to operator-created (no
 *   "system" badge per D-09).
 */
export default function ProfilesPage() {
  const qc = useQueryClient()
  const [importOpen, setImportOpen] = useState(false)

  const profilesQuery = useQuery({
    queryKey: ['device-profiles'],
    queryFn: () => listProfiles(),
  })

  const onArchive = async (p: Profile) => {
    try {
      await archiveProfile(p.id)
      qc.invalidateQueries({ queryKey: ['device-profiles'] })
      toast.success('Profile archived')
    } catch {
      toast.error('Could not archive profile')
    }
  }

  const renderCapabilities = (caps: string[]) => {
    if (!caps || caps.length === 0)
      return <span className="text-sm text-muted-foreground">—</span>
    const visible = caps.slice(0, 3)
    const overflow = caps.length - visible.length
    return (
      <div className="flex flex-wrap gap-1">
        {visible.map((c) => (
          <Badge key={c} variant="secondary" className="text-xs uppercase">
            {c}
          </Badge>
        ))}
        {overflow > 0 ? (
          <Badge variant="secondary" className="text-xs">
            +{overflow} more
          </Badge>
        ) : null}
      </div>
    )
  }

  const columns: ColumnDef<Profile>[] = [
    {
      accessorKey: 'name',
      header: 'Name',
      cell: ({ row }) => (
        <Link
          to={`/profiles/${row.original.id}`}
          className="font-medium text-primary hover:underline"
        >
          {row.original.name}
        </Link>
      ),
    },
    {
      accessorKey: 'vendor',
      header: 'Vendor',
      cell: ({ row }) => <span className="text-sm">{row.original.vendor}</span>,
    },
    {
      accessorKey: 'family',
      header: 'Family',
      cell: ({ row }) => (
        <span className="text-sm text-muted-foreground">
          {row.original.family ?? '—'}
        </span>
      ),
    },
    {
      id: 'capabilities',
      header: 'Capabilities',
      cell: ({ row }) => renderCapabilities(row.original.capabilities ?? []),
    },
    {
      id: 'cs_sync',
      header: 'CS sync',
      cell: ({ row }) => (
        <span className="text-sm">
          {row.original.codec_js_synced_at ? (
            <span className="text-success">Synced</span>
          ) : row.original.cs_profile_id ? (
            <span className="text-muted-foreground">Local only</span>
          ) : (
            <span className="text-muted-foreground">Not synced</span>
          )}
        </span>
      ),
    },
    {
      accessorKey: 'updated_at',
      header: 'Last updated',
      cell: ({ row }) => (
        <span className="text-xs text-muted-foreground">
          {row.original.updated_at ?? '—'}
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
              <Link to={`/profiles/${row.original.id}`}>
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
    data: profilesQuery.data ?? [],
    columns,
    getCoreRowModel: getCoreRowModel(),
  })

  const isLoading = profilesQuery.isLoading
  const isEmpty = !isLoading && (profilesQuery.data ?? []).length === 0

  return (
    <div className="flex flex-col gap-6 p-6">
      <header className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold leading-8">Device profiles</h1>
        <div className="flex items-center gap-2">
          <Button variant="outline" onClick={() => setImportOpen(true)}>
            <Download className="mr-2 h-4 w-4" />
            Import from catalog
          </Button>
          <Button asChild>
            <Link to="/profiles/new">
              <Plus className="mr-2 h-4 w-4" />
              Create profile
            </Link>
          </Button>
        </div>
      </header>

      <ImportFromCatalogDialog
        open={importOpen}
        onOpenChange={setImportOpen}
        onImported={() => {
          qc.invalidateQueries({ queryKey: ['device-profiles'] })
        }}
      />

      {isEmpty ? (
        <div className="flex flex-col items-center gap-4 rounded-md border p-12 text-center">
          <h2 className="text-lg font-semibold">No profiles configured</h2>
          <p className="text-sm text-muted-foreground">
            Three default profiles ship with Shifter. If you don't see them,
            run database migrations.
          </p>
          <Button asChild variant="outline">
            <Link to="/settings">Open settings</Link>
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
    </div>
  )
}
