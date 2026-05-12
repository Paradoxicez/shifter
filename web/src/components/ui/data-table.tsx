/**
 * shadcn/ui DataTable — TanStack Table v8 wrapper.
 *
 * Plan 07-05 (D-40): Generic DataTable component used by VendorCatalogCard
 * (Surface 1) and future catalog surfaces.
 *
 * Based on: https://ui.shadcn.com/docs/components/data-table
 */

import {
  type ColumnDef,
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table'

import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

interface DataTableProps<TData, TValue> {
  columns: ColumnDef<TData, TValue>[]
  data: TData[]
  role?: string
  'aria-label'?: string
}

export function DataTable<TData, TValue>({
  columns,
  data,
  role,
  'aria-label': ariaLabel,
}: DataTableProps<TData, TValue>) {
  const table = useReactTable({
    data,
    columns,
    getCoreRowModel: getCoreRowModel(),
  })

  return (
    <div className="rounded-md border">
      <Table role={role} aria-label={ariaLabel}>
        <TableHeader>
          {table.getHeaderGroups().map((headerGroup) => (
            <TableRow key={headerGroup.id}>
              {headerGroup.headers.map((header) => (
                <TableHead
                  key={header.id}
                  style={header.column.columnDef.size ? { width: header.column.columnDef.size } : undefined}
                  className={
                    (header.column.columnDef.meta as { mobileHidden?: boolean } | undefined)
                      ?.mobileHidden
                      ? 'hidden sm:table-cell'
                      : undefined
                  }
                >
                  {header.isPlaceholder
                    ? null
                    : flexRender(header.column.columnDef.header, header.getContext())}
                </TableHead>
              ))}
            </TableRow>
          ))}
        </TableHeader>
        <TableBody>
          {table.getRowModel().rows?.length ? (
            table.getRowModel().rows.map((row) => (
              <TableRow key={row.id} data-state={row.getIsSelected() && 'selected'}>
                {row.getVisibleCells().map((cell) => (
                  <TableCell
                    key={cell.id}
                    className={
                      (cell.column.columnDef.meta as { mobileHidden?: boolean } | undefined)
                        ?.mobileHidden
                        ? 'hidden sm:table-cell'
                        : undefined
                    }
                  >
                    {flexRender(cell.column.columnDef.cell, cell.getContext())}
                  </TableCell>
                ))}
              </TableRow>
            ))
          ) : (
            <TableRow>
              <TableCell colSpan={columns.length} className="h-24 text-center">
                No results.
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </div>
  )
}
