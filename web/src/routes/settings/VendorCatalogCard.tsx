/**
 * VendorCatalogCard — Surface 1 (Settings → Vendor Catalog tab).
 *
 * Plan 07-05 / D-23, D-24, D-25, D-40.
 *
 * Renders a Card with:
 *   - Capability chip filter row (All | Water | Electricity | Multi-utility)
 *   - DataTable with 7 columns (Vendor | Family | Capability | Version |
 *     Installed | Devices using | Status)
 *   - Status cell with badge per status enum + Update button for
 *     update-available rows
 *   - Mobile: Family + Version columns hidden at ≤768px via Tailwind's
 *     `hidden sm:table-cell` classes
 *
 * UI-SPEC §"Surface 1" — all copy strings are verbatim.
 */

import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { type ColumnDef } from '@tanstack/react-table'
import { Link } from 'react-router-dom'
import { CheckIcon } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { DataTable } from '@/components/ui/data-table'
import { fetchCatalog, type CatalogEntry, type CatalogListProfile } from '@/lib/catalog'
import { CatalogUpdateModal } from '@/routes/settings/CatalogUpdateModal'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type CapabilityChip = 'all' | 'water' | 'electricity' | 'multi-utility'

export type CatalogRow = {
  slug: string
  vendor: string
  family: string
  capabilities: string[]
  catalogVersion: string
  installedVersion: string
  status: CatalogListProfile['status']
  devicesUsingCount: number
  profileId: string | null
  customerEdited: boolean
  codecJsSyncedAt: string | null
}

// ---------------------------------------------------------------------------
// Exported components
// ---------------------------------------------------------------------------

/** Main card for the Vendor Catalog tab. */
export function VendorCatalogCard() {
  const { data, isLoading } = useQuery({ queryKey: ['catalog'], queryFn: fetchCatalog })
  const [chip, setChip] = useState<CapabilityChip>('all')
  const [updateRow, setUpdateRow] = useState<CatalogRow | null>(null)

  const rows = mergeEntriesAndProfiles(data?.entries ?? [], data?.profiles ?? [])
  const filtered = filterByCapability(rows, chip)

  const columns: ColumnDef<CatalogRow>[] = [
    {
      accessorKey: 'vendor',
      header: 'Vendor',
      size: 160,
    },
    {
      accessorKey: 'family',
      header: 'Family',
      size: 160,
      meta: { mobileHidden: true },
    },
    {
      accessorKey: 'capabilities',
      header: 'Capability',
      size: 120,
      cell: ({ row }) => <CapabilityChips caps={row.original.capabilities} />,
    },
    {
      accessorKey: 'catalogVersion',
      header: 'Version',
      size: 80,
      meta: { mobileHidden: true },
    },
    {
      accessorKey: 'installedVersion',
      header: 'Installed',
      size: 80,
      cell: ({ row }) =>
        row.original.profileId ? (
          <CheckIcon className="h-4 w-4 text-green-600" aria-label="Installed" />
        ) : (
          <span className="text-muted-foreground">—</span>
        ),
    },
    {
      accessorKey: 'devicesUsingCount',
      header: 'Devices using',
      size: 100,
      cell: ({ row }) => {
        const r = row.original
        if (!r.profileId) return <span className="text-muted-foreground">—</span>
        const label = `View ${r.devicesUsingCount} devices using ${r.vendor} ${r.family}`
        const text = r.devicesUsingCount === 0 ? '0 devices' : `${r.devicesUsingCount} devices`
        return (
          <Link
            to={`/devices?profile_id=${encodeURIComponent(r.profileId)}`}
            aria-label={label}
            className="underline-offset-4 hover:underline"
          >
            {text}
          </Link>
        )
      },
    },
    {
      accessorKey: 'status',
      header: 'Status',
      size: 160,
      cell: ({ row }) => (
        <StatusCell row={row.original} onUpdate={() => setUpdateRow(row.original)} />
      ),
    },
  ]

  if (isLoading) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Vendor Catalog</CardTitle>
          <CardDescription>Manage device vendor profiles</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-12 w-full" />
          <Skeleton className="h-12 w-full" />
          <Skeleton className="h-12 w-full" />
        </CardContent>
      </Card>
    )
  }

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle>Vendor Catalog</CardTitle>
          <CardDescription>Manage device vendor profiles</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <CapabilityFilterRow value={chip} onChange={setChip} />
          {rows.length === 0 ? (
            <EmptyState />
          ) : filtered.length === 0 ? (
            <p className="text-sm text-muted-foreground py-4">
              No profiles match the selected capability filter.
            </p>
          ) : (
            <DataTable
              columns={columns}
              data={filtered}
              role="grid"
              aria-label="Vendor profiles"
            />
          )}
        </CardContent>
      </Card>
      {updateRow && (
        <CatalogUpdateModal row={updateRow} onClose={() => setUpdateRow(null)} />
      )}
    </>
  )
}

/**
 * Tab label helper — exported so settings.tsx can display the (N) badge
 * without re-fetching.
 */
export function VendorCatalogTabLabel({ updateCount }: { updateCount: number }) {
  return updateCount === 0 ? <>Vendor Catalog</> : <>Vendor Catalog ({updateCount})</>
}

// ---------------------------------------------------------------------------
// Sub-components
// ---------------------------------------------------------------------------

function EmptyState() {
  return (
    <div className="flex flex-col items-center gap-3 py-12 text-center">
      <h3 className="text-xl font-semibold">No vendor profiles</h3>
      <p className="text-sm text-muted-foreground max-w-sm">
        Import a profile from the vendor catalog to get started.
      </p>
      <Button>Browse catalog</Button>
    </div>
  )
}

function CapabilityChips({ caps }: { caps: string[] }) {
  const labels = capabilityLabels(caps)
  if (labels.length === 0) return <span className="text-muted-foreground">—</span>
  return (
    <div className="flex flex-wrap gap-1">
      {labels.map((l) => (
        <Badge key={l} variant="outline" className="text-xs">
          {l}
        </Badge>
      ))}
    </div>
  )
}

function CapabilityFilterRow({
  value,
  onChange,
}: {
  value: CapabilityChip
  onChange: (v: CapabilityChip) => void
}) {
  const chips: { key: CapabilityChip; label: string }[] = [
    { key: 'all', label: 'All' },
    { key: 'water', label: 'Water' },
    { key: 'electricity', label: 'Electricity' },
    { key: 'multi-utility', label: 'Multi-utility' },
  ]
  return (
    <div className="flex flex-wrap gap-2 py-2 overflow-x-auto">
      {chips.map((c) => (
        <Badge
          key={c.key}
          variant={value === c.key ? 'secondary' : 'outline'}
          className="cursor-pointer select-none"
          onClick={() => onChange(c.key)}
        >
          {c.label}
        </Badge>
      ))}
    </div>
  )
}

function StatusCell({ row, onUpdate }: { row: CatalogRow; onUpdate: () => void }) {
  if (row.status === 'not-installed') {
    return <Badge variant="outline">Not installed</Badge>
  }

  if (row.status === 'installed') {
    return (
      <div className="flex items-center gap-2">
        <Badge className="bg-[var(--success,theme(colors.green.600))] text-white">Installed</Badge>
        {row.codecJsSyncedAt === null && (
          <Badge className="bg-[var(--warning,theme(colors.amber.500))] text-white">
            Syncing to ChirpStack…
          </Badge>
        )}
      </div>
    )
  }

  // status === 'update-available'
  return (
    <div className="flex items-center gap-2 flex-wrap">
      <Badge className="bg-[var(--warning,theme(colors.amber.500))] text-white">
        Update available v{row.installedVersion} → v{row.catalogVersion}
      </Badge>
      <Button
        variant="outline"
        size="sm"
        aria-label={`Update ${row.vendor} ${row.family} to version ${row.catalogVersion}`}
        onClick={onUpdate}
      >
        Update to v{row.catalogVersion}
      </Button>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Merges catalog entries + installed profile rows into a unified table row. */
export function mergeEntriesAndProfiles(
  entries: CatalogEntry[],
  profiles: CatalogListProfile[]
): CatalogRow[] {
  const profileMap = new Map(profiles.map((p) => [p.slug, p]))

  return entries.map((e) => {
    const profile = profileMap.get(e.slug)
    return {
      slug: e.slug,
      vendor: e.vendor,
      family: e.family,
      capabilities: e.capabilities,
      catalogVersion: e.version,
      installedVersion: profile?.installed_version ?? '',
      status: profile?.status ?? 'not-installed',
      devicesUsingCount: profile?.devices_using_count ?? 0,
      profileId: profile?.profile_id ?? null,
      customerEdited: profile?.customer_edited ?? false,
      codecJsSyncedAt: profile?.codec_js_synced_at ?? null,
    }
  })
}

/**
 * Filters rows by capability chip selection.
 *
 * Water: capabilities contains cumulative + at least one of flow_rate, leak_detection
 * Electricity: capabilities contains instant_power or power_quality
 * Multi-utility: matches BOTH Water and Electricity rules
 */
export function filterByCapability(rows: CatalogRow[], chip: CapabilityChip): CatalogRow[] {
  if (chip === 'all') return rows
  return rows.filter((r) => matchesChip(r.capabilities, chip))
}

function matchesChip(caps: string[], chip: CapabilityChip): boolean {
  const isWater =
    caps.includes('cumulative') &&
    (caps.includes('flow_rate') || caps.includes('leak_detection'))
  const isElectricity = caps.includes('instant_power') || caps.includes('power_quality')

  switch (chip) {
    case 'water':
      return isWater
    case 'electricity':
      return isElectricity
    case 'multi-utility':
      return isWater && isElectricity
    default:
      return true
  }
}

/** Returns human-readable capability labels for display in capability column chips. */
function capabilityLabels(caps: string[]): string[] {
  const labels: string[] = []
  const hasWater =
    caps.includes('cumulative') &&
    (caps.includes('flow_rate') || caps.includes('leak_detection'))
  const hasElectricity = caps.includes('instant_power') || caps.includes('power_quality')
  if (hasWater) labels.push('Water')
  if (hasElectricity) labels.push('Electricity')
  if (!hasWater && !hasElectricity && caps.length > 0) labels.push(caps[0])
  return labels
}
