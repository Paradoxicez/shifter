/**
 * Plan 06-04 — small Badge wrapper applying the locked severity color
 * map from UI-SPEC §Severity map.
 *
 * Used everywhere a severity is rendered: bell badge, drawer rows,
 * /alerts table rows, alert detail dialog header, rule library row.
 */

import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

export type Severity = 'critical' | 'warning' | 'info'

export interface SeverityPillProps {
  severity: Severity
  children?: ReactNode
  className?: string
}

const SEVERITY_STYLES: Record<Severity, string> = {
  critical: 'bg-red-500/15 text-red-600 ring-red-500/30',
  warning: 'bg-amber-500/15 text-amber-700 ring-amber-500/30',
  info: 'bg-blue-500/15 text-blue-600 ring-blue-500/30',
}

const SEVERITY_LABELS: Record<Severity, string> = {
  critical: 'Critical',
  warning: 'Warning',
  info: 'Info',
}

export function SeverityPill({ severity, children, className }: SeverityPillProps) {
  return (
    <span
      className={cn(
        'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset',
        SEVERITY_STYLES[severity],
        className,
      )}
      data-severity={severity}
    >
      {children ?? SEVERITY_LABELS[severity]}
    </span>
  )
}

/**
 * SeverityDot — small circle for the bell badge / sidebar nav badge / drawer
 * row leading marker. Uses the same color map but renders a 6px dot.
 */
export function SeverityDot({ severity, className }: { severity: Severity; className?: string }) {
  const colors: Record<Severity, string> = {
    critical: 'bg-red-500',
    warning: 'bg-amber-500',
    info: 'bg-blue-500',
  }
  return (
    <span
      aria-hidden
      className={cn('inline-block size-2 rounded-full', colors[severity], className)}
      data-severity={severity}
    />
  )
}
