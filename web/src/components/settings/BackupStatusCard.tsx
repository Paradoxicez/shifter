/**
 * BackupStatusCard — SETT-05 backup health + threshold editor.
 *
 * GET  /api/settings/backup  — admin + viewer (ActionBackupRead)
 * PATCH /api/settings/backup/thresholds — admin only (ActionBackupConfigure)
 *
 * Plan 06-10 / SETT-05.
 *
 * Displays:
 *  - Freshness dot (green/yellow/red) + human-readable age
 *  - Last backup metadata
 *  - Threshold editor (admin only) — inline number inputs with save button
 *  - Recent backup history (up to 5 rows, collapsible)
 */

import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { toast } from 'sonner'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { apiFetch } from '@/lib/api'
import { useCurrentUser } from '@/lib/use-current-user'
import { BackupFreshnessDot } from './BackupFreshnessDot'
import { BackupHistoryList, type BackupRunSummary } from './BackupHistoryList'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface BackupStatusResponse {
  never_run: boolean
  last?: BackupRunSummary
  recent: BackupRunSummary[]
  warn_threshold_hours: number
  crit_threshold_hours: number
  destination_dir: string
}

// ---------------------------------------------------------------------------
// Data fetching
// ---------------------------------------------------------------------------

function fetchBackupStatus(): Promise<BackupStatusResponse> {
  return apiFetch<BackupStatusResponse>('/api/settings/backup')
}

function patchThresholds(data: {
  warn_threshold_hours: number
  crit_threshold_hours: number
}): Promise<unknown> {
  return apiFetch('/api/settings/backup/thresholds', {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

// ---------------------------------------------------------------------------
// Threshold form schema
// ---------------------------------------------------------------------------

const thresholdSchema = z
  .object({
    warn_threshold_hours: z.coerce.number().int().min(1, 'Must be at least 1').max(8760),
    crit_threshold_hours: z.coerce.number().int().min(1, 'Must be at least 1').max(8760),
  })
  .refine((d) => d.warn_threshold_hours < d.crit_threshold_hours, {
    message: 'Warning threshold must be less than critical threshold',
    path: ['warn_threshold_hours'],
  })

type ThresholdFormValues = z.infer<typeof thresholdSchema>

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function formatAge(ageSeconds: number): string {
  if (ageSeconds < 3600) return `${Math.round(ageSeconds / 60)} min ago`
  if (ageSeconds < 86400) return `${Math.round(ageSeconds / 3600)} h ago`
  return `${Math.round(ageSeconds / 86400)} d ago`
}

// ---------------------------------------------------------------------------
// Components
// ---------------------------------------------------------------------------

function ThresholdForm({
  defaultWarn,
  defaultCrit,
  onSaved,
}: {
  defaultWarn: number
  defaultCrit: number
  onSaved: () => void
}) {
  const {
    register,
    handleSubmit,
    formState: { errors, isDirty },
  } = useForm<ThresholdFormValues>({
    resolver: zodResolver(thresholdSchema),
    defaultValues: { warn_threshold_hours: defaultWarn, crit_threshold_hours: defaultCrit },
  })

  const mutation = useMutation({
    mutationFn: patchThresholds,
    onSuccess: () => {
      toast.success('Thresholds updated')
      onSaved()
    },
    onError: () => toast.error('Failed to update thresholds'),
  })

  return (
    <form
      onSubmit={handleSubmit((v) => mutation.mutate(v))}
      className="flex flex-col gap-3 pt-2"
      data-testid="backup-threshold-form"
    >
      <div className="flex items-center gap-4">
        <div className="flex flex-col gap-1">
          <Label htmlFor="warn_threshold_hours" className="text-xs">
            Warn (hours)
          </Label>
          <Input
            id="warn_threshold_hours"
            type="number"
            className="h-8 w-24 text-sm"
            {...register('warn_threshold_hours')}
            data-testid="input-warn-hours"
          />
          {errors.warn_threshold_hours && (
            <p className="text-xs text-destructive">{errors.warn_threshold_hours.message}</p>
          )}
        </div>
        <div className="flex flex-col gap-1">
          <Label htmlFor="crit_threshold_hours" className="text-xs">
            Crit (hours)
          </Label>
          <Input
            id="crit_threshold_hours"
            type="number"
            className="h-8 w-24 text-sm"
            {...register('crit_threshold_hours')}
            data-testid="input-crit-hours"
          />
          {errors.crit_threshold_hours && (
            <p className="text-xs text-destructive">{errors.crit_threshold_hours.message}</p>
          )}
        </div>
        <Button
          type="submit"
          size="sm"
          className="mt-4"
          disabled={!isDirty || mutation.isPending}
          data-testid="save-thresholds-btn"
        >
          {mutation.isPending ? 'Saving…' : 'Save'}
        </Button>
      </div>
    </form>
  )
}

export function BackupStatusCard() {
  const user = useCurrentUser()
  const isAdmin = user?.role === 'admin'
  const qc = useQueryClient()

  const { data, isLoading } = useQuery({
    queryKey: ['settings', 'backup'],
    queryFn: fetchBackupStatus,
  })

  if (isLoading || !data) return null

  const ageSeconds = data.last?.age_seconds ?? null

  function humanAge() {
    if (data!.never_run) return 'Never run'
    if (ageSeconds === null) return 'Unknown'
    return formatAge(ageSeconds)
  }

  return (
    <Card data-testid="backup-status-card">
      <CardHeader>
        <div className="flex items-center gap-2">
          <BackupFreshnessDot
            neverRun={data.never_run}
            ageSeconds={ageSeconds}
            warnThresholdHours={data.warn_threshold_hours}
            critThresholdHours={data.crit_threshold_hours}
          />
          <CardTitle>Backup</CardTitle>
        </div>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {/* Last backup row */}
        <div className="flex flex-col gap-1">
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium">Last backup</span>
            <span className="text-sm text-muted-foreground" data-testid="backup-age-text">
              {humanAge()}
            </span>
          </div>
          {data.last && (
            <div className="flex items-center justify-between">
              <span className="text-xs text-muted-foreground">Destination</span>
              <span className="font-mono text-xs text-muted-foreground">
                {data.destination_dir}
              </span>
            </div>
          )}
        </div>

        {/* Threshold editor — admin only */}
        {isAdmin && (
          <div className="border-t pt-3">
            <p className="text-xs font-medium text-muted-foreground">Alert thresholds</p>
            <ThresholdForm
              defaultWarn={data.warn_threshold_hours}
              defaultCrit={data.crit_threshold_hours}
              onSaved={() => qc.invalidateQueries({ queryKey: ['settings', 'backup'] })}
            />
          </div>
        )}

        {/* Recent history */}
        {data.recent.length > 0 && (
          <div className="border-t pt-3">
            <p className="mb-2 text-xs font-medium text-muted-foreground">Recent runs</p>
            <BackupHistoryList runs={data.recent} />
          </div>
        )}
      </CardContent>
    </Card>
  )
}
