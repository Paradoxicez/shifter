/**
 * Plan 06-04 — MP detail "Anomaly detection" card (UI-SPEC §Surface 4).
 *
 * Three states:
 *  - warming_up       : eligible=false; Progress bar shows X/21 days
 *  - eligible_inactive: eligible=true, all rules disabled; show toggles
 *  - active           : eligible=true, ≥1 rule enabled; success border
 *
 * Viewer role: toggles are disabled with a tooltip.
 */

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import { Switch } from '@/components/ui/switch'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import {
  useMPAnomalyState,
  useToggleMPAnomalyRuleMutation,
} from '@/hooks/useAlerts'
import { useCurrentUser } from '@/lib/use-current-user'
import { toast } from 'sonner'

export interface AnomalyStateCardProps {
  meteringPointId: string
}

export function AnomalyStateCard({ meteringPointId }: AnomalyStateCardProps) {
  const user = useCurrentUser()
  const isAdmin = user?.role === 'admin'
  const { data, isLoading } = useMPAnomalyState(meteringPointId)
  const toggle = useToggleMPAnomalyRuleMutation()

  if (isLoading || !data) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Anomaly detection</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="text-sm text-muted-foreground">Loading…</div>
        </CardContent>
      </Card>
    )
  }

  const state = computeState(data)

  const onToggle = (kind: string, next: boolean) => {
    toggle.mutate(
      { mpId: meteringPointId, kind, enabled: next },
      {
        onSuccess: () => toast.success(next ? `${kind} enabled` : `${kind} disabled`),
        onError: (err) => toast.error(`Toggle failed: ${(err as Error).message}`),
      },
    )
  }

  return (
    <Card
      data-state={state}
      className={state === 'active' ? 'border-emerald-500/40' : undefined}
    >
      <CardHeader>
        <CardTitle>Anomaly detection</CardTitle>
      </CardHeader>
      <CardContent>
        {state === 'warming_up' ? (
          <div className="flex flex-col gap-2">
            <div className="text-sm">
              Building baseline — {21 - data.days_until_eligible}/21 days of data captured.
            </div>
            <Progress value={((21 - data.days_until_eligible) / 21) * 100} />
            <div className="text-xs text-muted-foreground">
              Anomaly rules become available after 21 days of measurements.
            </div>
          </div>
        ) : (
          <ul className="flex flex-col gap-2">
            {data.rules.map((rule) => (
              <li
                key={rule.rule_kind}
                className="flex items-center justify-between rounded-md border border-border bg-card p-3 text-sm"
                data-rule-kind={rule.rule_kind}
              >
                <div>
                  <div className="font-medium">{prettyKind(rule.rule_kind)}</div>
                  <div className="text-xs text-muted-foreground">
                    {rule.enabled ? 'Active' : 'Off'}
                    {rule.severity ? ` · ${rule.severity}` : ''}
                  </div>
                </div>
                {isAdmin ? (
                  <Switch
                    checked={rule.enabled}
                    disabled={toggle.isPending}
                    onCheckedChange={(next) => onToggle(toKindParam(rule.rule_kind), next)}
                    aria-label={`Toggle ${rule.rule_kind}`}
                  />
                ) : (
                  <TooltipProvider>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <span>
                          <Switch
                            checked={rule.enabled}
                            disabled
                            aria-label={`${rule.rule_kind} (read-only)`}
                          />
                        </span>
                      </TooltipTrigger>
                      <TooltipContent>Read-only — viewer role</TooltipContent>
                    </Tooltip>
                  </TooltipProvider>
                )}
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}

function computeState(data: { eligible: boolean; rules: Array<{ enabled: boolean }> }) {
  if (!data.eligible) return 'warming_up'
  const anyEnabled = data.rules.some((r) => r.enabled)
  return anyEnabled ? 'active' : 'eligible_inactive'
}

function prettyKind(kind: string): string {
  switch (kind) {
    case 'anomaly_p95':
      return 'P95 outlier detection'
    case 'anomaly_iqr':
      return 'IQR outlier detection'
    case 'anomaly_quiet_hour':
      return 'Quiet-hour flow detection'
    default:
      return kind
  }
}

function toKindParam(ruleKind: string): string {
  return ruleKind.replace(/^anomaly_/, '')
}
