/**
 * BackupFreshnessDot — coloured status indicator for backup freshness.
 *
 * - green:  last backup age ≤ warnThresholdHours  (or never ran but warn=0, but we treat neverRun as red)
 * - yellow: warnThresholdHours < age ≤ critThresholdHours
 * - red:    age > critThresholdHours  OR  neverRun=true
 *
 * SETT-05 / Plan 06-10.
 */

export type FreshnessStatus = 'ok' | 'warn' | 'crit'

export interface BackupFreshnessDotProps {
  neverRun: boolean
  ageSeconds: number | null
  warnThresholdHours: number
  critThresholdHours: number
}

export function computeFreshnessStatus({
  neverRun,
  ageSeconds,
  warnThresholdHours,
  critThresholdHours,
}: BackupFreshnessDotProps): FreshnessStatus {
  if (neverRun || ageSeconds === null) return 'crit'
  const ageHours = ageSeconds / 3600
  if (ageHours <= warnThresholdHours) return 'ok'
  if (ageHours <= critThresholdHours) return 'warn'
  return 'crit'
}

const DOT_CLASSES: Record<FreshnessStatus, string> = {
  ok:   'bg-green-500',
  warn: 'bg-yellow-400',
  crit: 'bg-red-500',
}

const DOT_LABELS: Record<FreshnessStatus, string> = {
  ok:   'Backup is current',
  warn: 'Backup is aging',
  crit: 'Backup is overdue',
}

export function BackupFreshnessDot(props: BackupFreshnessDotProps) {
  const status = computeFreshnessStatus(props)
  return (
    <span
      className={`inline-block h-2.5 w-2.5 rounded-full ${DOT_CLASSES[status]}`}
      aria-label={DOT_LABELS[status]}
      data-testid="backup-freshness-dot"
      data-status={status}
    />
  )
}
