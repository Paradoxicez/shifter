import { useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { apiFetch } from '@/lib/api'
import { useCurrentUser } from '@/lib/use-current-user'
import { EditRetentionDialog } from './EditRetentionDialog'

/**
 * Data Retention settings card (D-09 / DATA-13 / Plan 05-11).
 *
 * Displays 5 retention levels (raw, hourly, daily, monthly, yearly).
 * Admin users see an "Edit" ghost button per editable row.
 * Viewers see read-only values (AUTH-06 frontend hide; server enforces 403).
 *
 * Yearly aggregate is always read-only in this card — null means "Never expires".
 *
 * UI-SPEC §Settings — Data Retention Card.
 */

export interface RetentionConfig {
  raw_days: number
  hourly_days: number
  daily_days: number
  monthly_days: number
  yearly_days: number | null
  updated_at: string
}

export type EditableLevelKey = 'raw_days' | 'hourly_days' | 'daily_days' | 'monthly_days'

export interface RetentionLevel {
  key: EditableLevelKey
  label: string
  unit: 'days' | 'years'
  min: number
  max: number
}

export const RETENTION_LEVELS: RetentionLevel[] = [
  { key: 'raw_days',     label: 'Raw measurements',  unit: 'days',  min: 30,   max: 365 },
  { key: 'hourly_days',  label: 'Hourly aggregate',  unit: 'days',  min: 180,  max: 1825 },
  { key: 'daily_days',   label: 'Daily aggregate',   unit: 'days',  min: 365,  max: 7300 },
  { key: 'monthly_days', label: 'Monthly aggregate', unit: 'years', min: 1825, max: 18250 },
]

export function formatRetentionValue(days: number, unit: 'days' | 'years'): string {
  if (unit === 'years') return `${Math.round(days / 365)} years`
  return `${days} days`
}

export function fetchRetentionConfig(): Promise<RetentionConfig> {
  return apiFetch<RetentionConfig>('/api/settings/retention')
}

export function DataRetentionCard() {
  const user = useCurrentUser()
  const isAdmin = user?.role === 'admin'
  const queryClient = useQueryClient()

  const { data, isLoading } = useQuery({
    queryKey: ['settings', 'retention'],
    queryFn: fetchRetentionConfig,
  })

  const [editing, setEditing] = useState<EditableLevelKey | null>(null)

  if (isLoading || !data) return null

  const editingLevel = editing ? RETENTION_LEVELS.find((l) => l.key === editing) : null

  return (
    <Card>
      <CardHeader>
        <CardTitle>Data Retention</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-0">
        {RETENTION_LEVELS.map((level) => (
          <div
            key={level.key}
            className="flex items-center justify-between border-b py-3 last:border-0"
            data-testid={`retention-row-${level.key}`}
          >
            <div>
              <div className="text-sm font-medium">{level.label}</div>
              <div className="text-xs text-muted-foreground">
                {formatRetentionValue(data[level.key], level.unit)}
              </div>
            </div>
            {isAdmin && (
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setEditing(level.key)}
                aria-label={`Edit ${level.label}`}
              >
                Edit
              </Button>
            )}
          </div>
        ))}

        {/* Yearly aggregate — read-only; null means "Never expires" (D-09) */}
        <div
          className="flex items-center justify-between py-3"
          data-testid="retention-row-yearly_days"
        >
          <div>
            <div className="text-sm font-medium">Yearly aggregate</div>
            <div className="text-xs text-muted-foreground">
              {data.yearly_days === null
                ? 'Never expires'
                : formatRetentionValue(data.yearly_days, 'years')}
            </div>
          </div>
          {/* yearly is intentionally read-only — no Edit button */}
        </div>
      </CardContent>

      {editing && editingLevel && (
        <EditRetentionDialog
          level={editingLevel}
          initialValue={data[editing]}
          open={true}
          onOpenChange={(open) => {
            if (!open) {
              setEditing(null)
              queryClient.invalidateQueries({ queryKey: ['settings', 'retention'] })
            }
          }}
        />
      )}
    </Card>
  )
}
