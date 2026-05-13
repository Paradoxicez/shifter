/**
 * CatalogUpdateModal — Surface 3 (UI-SPEC §"Surface 3").
 *
 * Plan 07-06 / V2-VEND-01.
 *
 * Shows a per-field diff between the installed device profile and the catalog
 * version. Operator can toggle each changed field between "Use mine" and
 * "Use catalog", then Apply Update submits only accepted fields.
 *
 * All copy strings are verbatim from UI-SPEC Surface 3.
 */

import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { ScrollArea } from '@/components/ui/scroll-area'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { applyCatalogUpdate, fetchCatalogEntry } from '@/lib/catalog'
import { getProfileWithMappings } from '@/lib/profiles'
import type { CatalogRow } from './VendorCatalogCard'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type ToggleValue = 'mine' | 'catalog'

type DiffField = {
  name: string
  label: string
  currentValue: string
  catalogValue: string
  customerEdited: boolean
  toggle: ToggleValue
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Converts any value to a compact display string. */
function displayValue(v: unknown): string {
  if (v === null || v === undefined || v === '') return '—'
  if (typeof v === 'string' && v.length > 200) return v.slice(0, 200) + '…'
  return String(v)
}

/** Maps D-34 internal field names to human-readable labels. */
const FIELD_LABELS: Record<string, string> = {
  display_name: 'Display name',
  manufacturer: 'Manufacturer (vendor)',
  model: 'Model (family)',
  codec_js: 'Codec',
  mac_version: 'MAC version',
  reg_params_revision: 'Regional parameters revision',
  counter_modulus: 'Counter modulus',
  region: 'Region',
}

// D-34 allowlist — same 8 fields the backend validates
const D34_FIELDS = [
  'display_name',
  'manufacturer',
  'model',
  'codec_js',
  'mac_version',
  'reg_params_revision',
  'counter_modulus',
  'region',
]

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

interface CatalogUpdateModalProps {
  row: CatalogRow
  onClose: () => void
}

export function CatalogUpdateModal({ row, onClose }: CatalogUpdateModalProps) {
  const qc = useQueryClient()

  // Fetch catalog entry (has the catalog field values)
  const { data: catalogEntry, isLoading: catalogLoading } = useQuery({
    queryKey: ['catalog-entry', row.slug],
    queryFn: () => fetchCatalogEntry(row.slug),
  })

  // Fetch installed profile (has the operator's current values)
  const { data: profileData, isLoading: profileLoading } = useQuery({
    queryKey: ['profile-with-mappings', row.profileId],
    queryFn: () => getProfileWithMappings(row.profileId!),
    enabled: !!row.profileId,
  })

  const isLoading = catalogLoading || profileLoading

  // Build catalog values map for comparison
  const catalogValues: Record<string, unknown> = catalogEntry
    ? {
        display_name: catalogEntry.name,
        manufacturer: catalogEntry.vendor,
        model: catalogEntry.family,
        codec_js: catalogEntry.codec_js ?? '',
        mac_version: catalogEntry.mac_version,
        reg_params_revision: null, // not exposed in catalog schema v1
        counter_modulus: catalogEntry.counter_modulus,
        region: catalogEntry.region,
      }
    : {}

  // Build installed profile values map for comparison
  const profile = profileData?.profile
  const installedValues: Record<string, unknown> = profile
    ? {
        display_name: profile.name,
        manufacturer: profile.vendor,
        model: profile.family ?? null,
        codec_js: profile.codec_js ?? '',
        mac_version: profile.mac_version,
        reg_params_revision: null,
        counter_modulus: profile.counter_modulus,
        region: profile.region ?? null,
      }
    : {}

  // Compute changed fields — only those where catalog value differs from installed
  const changedFields: Omit<DiffField, 'toggle'>[] = !isLoading
    ? D34_FIELDS.flatMap((fieldName) => {
        const catalogVal = catalogValues[fieldName]
        const installedVal = installedValues[fieldName]
        const catalogStr = displayValue(catalogVal)
        const installedStr = displayValue(installedVal)
        if (catalogStr === installedStr) return []
        return [
          {
            name: fieldName,
            label: FIELD_LABELS[fieldName] ?? fieldName,
            currentValue: installedStr,
            catalogValue: catalogStr,
            customerEdited: row.customerEdited,
          },
        ]
      })
    : []

  const unchangedCount = !isLoading ? D34_FIELDS.length - changedFields.length : 0

  // Toggle state — default: "Use mine" if customer_edited, else "Use catalog"
  const [toggles, setToggles] = useState<Record<string, ToggleValue>>(() =>
    Object.fromEntries(
      D34_FIELDS.map((f) => [f, row.customerEdited ? 'mine' : 'catalog'])
    )
  )

  function setToggle(fieldName: string, value: ToggleValue) {
    setToggles((prev) => ({ ...prev, [fieldName]: value }))
  }

  // accepted_fields = fields where toggle is "catalog"
  const acceptedFields = changedFields
    .filter((f) => (toggles[f.name] ?? 'catalog') === 'catalog')
    .map((f) => f.name)

  const mutation = useMutation({
    mutationFn: () =>
      applyCatalogUpdate(row.profileId!, {
        accepted_fields: acceptedFields,
        target_version: row.catalogVersion,
      }),
    onSuccess: (resp) => {
      toast.success(`Profile updated to v${resp.new_version}`)
      qc.invalidateQueries({ queryKey: ['catalog'] })
      qc.invalidateQueries({ queryKey: ['device-profiles'] })
      onClose()
    },
    onError: (err: Error) => {
      toast.error(`Update failed. ${err.message}. Try again.`)
    },
  })

  return (
    <Dialog open onOpenChange={(v) => { if (!v) onClose() }}>
      <DialogContent className="sm:max-w-[700px]">
        <DialogHeader>
          <DialogTitle>
            Update {row.vendor} {row.family}
          </DialogTitle>
          <p className="text-sm text-muted-foreground">
            v{row.installedVersion} → v{row.catalogVersion}
          </p>
          <p className="text-sm text-muted-foreground mt-1">
            Review changes field by field. Fields you&apos;ve edited are flagged — your edits are preserved by default.
          </p>
        </DialogHeader>

        {isLoading ? (
          <p className="py-6 text-sm text-muted-foreground text-center">
            Loading diff…
          </p>
        ) : changedFields.length === 0 ? (
          <p className="py-6 text-sm text-muted-foreground text-center">
            No field differences found between installed and catalog versions.
          </p>
        ) : (
          <ScrollArea className="max-h-[400px]">
            <table className="w-full text-sm border-collapse">
              <thead>
                <tr className="border-b text-left text-xs text-muted-foreground uppercase">
                  <th className="py-2 px-3 font-medium">Field</th>
                  <th className="py-2 px-3 font-medium">Current value</th>
                  <th className="py-2 px-3 font-medium">Catalog value</th>
                  <th className="py-2 px-3 font-medium">Choice</th>
                  <th className="py-2 px-3 font-medium">Flag</th>
                </tr>
              </thead>
              <tbody>
                {changedFields.map((field) => {
                  const toggleVal = toggles[field.name] ?? 'catalog'
                  return (
                    <tr key={field.name} className="border-b last:border-0">
                      <td className="py-3 px-3 font-medium whitespace-nowrap">
                        {field.label}
                      </td>
                      <td className="py-3 px-3 font-mono text-xs max-w-[160px] truncate">
                        {field.currentValue}
                      </td>
                      <td className="py-3 px-3 font-mono text-xs max-w-[160px] truncate">
                        {field.catalogValue}
                      </td>
                      <td className="py-3 px-3">
                        <ToggleGroup
                          type="single"
                          variant="outline"
                          value={toggleVal}
                          onValueChange={(v) => {
                            if (v) setToggle(field.name, v as ToggleValue)
                          }}
                        >
                          <ToggleGroupItem
                            value="mine"
                            aria-label={`Use my value for ${field.label}`}
                          >
                            Use mine
                          </ToggleGroupItem>
                          <ToggleGroupItem
                            value="catalog"
                            aria-label={`Use catalog value for ${field.label}`}
                          >
                            Use catalog
                          </ToggleGroupItem>
                        </ToggleGroup>
                      </td>
                      <td className="py-3 px-3">
                        {field.customerEdited && (
                          <Badge
                            variant="secondary"
                            aria-label="You have edited this field"
                          >
                            Edited
                          </Badge>
                        )}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </ScrollArea>
        )}

        {!isLoading && unchangedCount > 0 && (
          <p className="text-xs text-muted-foreground mt-2">
            {unchangedCount} fields unchanged (not shown)
          </p>
        )}

        <DialogFooter>
          <Button variant="ghost" onClick={onClose} disabled={mutation.isPending}>
            Discard update
          </Button>
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  onClick={() => mutation.mutate()}
                  disabled={mutation.isPending || isLoading}
                >
                  {mutation.isPending ? 'Applying…' : 'Apply Update'}
                </Button>
              </TooltipTrigger>
              <TooltipContent>
                This will update {acceptedFields.length} changed fields.
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
