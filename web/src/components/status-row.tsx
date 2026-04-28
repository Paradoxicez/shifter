import { CheckCircle2, MinusCircle, XCircle } from 'lucide-react'

export type StatusRowVariant = 'reachable' | 'unreachable' | 'skipped'

export interface StatusRowProps {
  status: StatusRowVariant
  label: string
  detail?: string
}

/**
 * UI-SPEC pattern: status dot/icon + label + monospace detail.
 * Used by Test Connection (Plan 17) and inherited by Phase 4 SSE health,
 * Phase 6 alert center / backup status.
 */
export function StatusRow({ status, label, detail }: StatusRowProps) {
  const Icon =
    status === 'reachable' ? CheckCircle2 : status === 'unreachable' ? XCircle : MinusCircle
  const color =
    status === 'reachable'
      ? 'text-success'
      : status === 'unreachable'
        ? 'text-destructive'
        : 'text-muted-foreground'
  const stateLabel =
    status === 'reachable' ? 'Reachable' : status === 'unreachable' ? 'Unreachable' : 'Skipped'
  return (
    <div className="flex items-center gap-3 py-2">
      <Icon className={`h-5 w-5 ${color}`} aria-hidden="true" />
      <span className="text-sm font-semibold w-16">{label}</span>
      <span className={`text-sm ${color}`}>{stateLabel}</span>
      {detail ? (
        <span className="text-sm font-mono text-muted-foreground ml-auto">{detail}</span>
      ) : null}
    </div>
  )
}
