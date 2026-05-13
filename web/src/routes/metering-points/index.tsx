import { useQuery } from '@tanstack/react-query'
import {
  flexRender,
  getCoreRowModel,
  useReactTable,
  type ColumnDef,
} from '@tanstack/react-table'
import { Link } from 'react-router-dom'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { listMPs, type MeteringPoint } from '@/lib/metering-points'

const columns: ColumnDef<MeteringPoint>[] = [
  {
    accessorKey: 'name',
    header: 'Name',
    cell: ({ row }) => (
      <Link
        to={`/metering-points/${row.original.id}`}
        className="font-medium text-primary hover:underline"
      >
        {row.original.name}
      </Link>
    ),
  },
  {
    accessorKey: 'utility_class',
    header: 'Utility',
    cell: ({ row }) => (
      <span className="text-sm text-muted-foreground capitalize">
        {row.original.utility_class}
      </span>
    ),
  },
  {
    accessorKey: 'site_id',
    header: 'Site',
    cell: ({ row }) => (
      <span className="text-sm text-muted-foreground">{row.original.site_id}</span>
    ),
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
]

export default function MeteringPointsPage() {
  const query = useQuery({
    queryKey: ['metering-points'],
    queryFn: () => listMPs(),
  })

  const table = useReactTable({
    data: query.data ?? [],
    columns,
    getCoreRowModel: getCoreRowModel(),
  })

  const isLoading = query.isLoading
  const isEmpty = !isLoading && (query.data ?? []).length === 0

  return (
    <div className="flex flex-col gap-6 p-6">
      <header>
        <h1 className="text-2xl font-semibold leading-8">Metering points</h1>
      </header>

      {isEmpty ? (
        <div className="flex flex-col items-center gap-4 rounded-md border p-12 text-center">
          <h2 className="text-lg font-semibold">No metering points yet</h2>
          <p className="text-sm text-muted-foreground">
            Add a metering point from within a site to get started.
          </p>
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
                ? Array.from({ length: 4 }).map((_, i) => (
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
