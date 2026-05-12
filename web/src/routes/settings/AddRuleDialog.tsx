/**
 * Plan 06-04 — Add Rule dialog (UI-SPEC §"Add Rule dialog — 5 steps").
 * Plan 07-10 — Phase 7 extensions:
 *   - Profile-aware rule kind filtering by anomaly_compatibility
 *   - "Test against last 30 days" backtest button + sparkline result panel
 *
 * Steps:
 *  1. Scope         — SKIPPED when invoked with meteringPointId/siteId
 *  2. Kind          — filtered by anomaly_compatibility (Phase 7)
 *  3. Conditions    — includes backtest button below rule config (Phase 7)
 *  4. Severity + Cooldown + Notes
 *  5. Review        — Save + Test fire button
 *
 * For v1 we keep the form lean — every step renders inline rather than a
 * separate stepper UI — so the dialog stays compact and operator can scan
 * the entire shape at once. The plan body's 5-step ordering is preserved
 * as visual section headers so the contract with UI-SPEC remains stable.
 */

import { useMemo, useState } from 'react'
import { toast } from 'sonner'
import { useMutation } from '@tanstack/react-query'
import { Bar, BarChart } from 'recharts'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Skeleton } from '@/components/ui/skeleton'
import { Textarea } from '@/components/ui/textarea'
import { useCreateRuleMutation, useTestFireMutation } from '@/hooks/useAlerts'
import { runBacktest } from '@/lib/backtest'
import type { BacktestResponse } from '@/lib/backtest'

export interface AddRuleDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** When set, step 1 (Scope) is skipped — UI-SPEC D-18 prefilled-from-MP entry */
  meteringPointId?: string
  siteId?: string
  /**
   * Plan 07-10 D-42: anomaly_compatibility of the MP's bound device profile.
   * Drives which rule kinds are visible in the Kind dropdown.
   * - 'full'        → all rule kinds available (default)
   * - 'limited'     → anomaly_p95 + anomaly_iqr available; anomaly_quiet_hour hidden
   * - 'unsupported' → all anomaly rule kinds hidden; banner shown
   * - undefined     → treated as 'full' (backward compat for callers without profile)
   */
  anomalyCompatibility?: 'full' | 'limited' | 'unsupported'
}

// Base rule kinds (non-anomaly) always available.
const BASE_RULE_KINDS = [
  'threshold_instantaneous',
  'threshold_hourly',
  'threshold_daily',
  'offline_device',
  'gateway_offline',
  'battery_low',
  'reverse_flow_increase',
] as const

const ANOMALY_ALL = ['anomaly_p95', 'anomaly_iqr', 'anomaly_quiet_hour'] as const
const ANOMALY_LIMITED = ['anomaly_p95', 'anomaly_iqr'] as const

const SEVERITIES = ['critical', 'warning', 'info'] as const

export function AddRuleDialog({
  open,
  onOpenChange,
  meteringPointId,
  siteId,
  anomalyCompatibility,
}: AddRuleDialogProps) {
  const [ruleKind, setRuleKind] = useState<string>('threshold_instantaneous')
  const [scopeKind, setScopeKind] = useState<string>(
    meteringPointId ? 'metering_point' : siteId ? 'site' : 'global',
  )
  const [highBound, setHighBound] = useState<string>('')
  const [comparison, setComparison] = useState<string>('gt')
  const [unit, setUnit] = useState<string>('')
  const [severity, setSeverity] = useState<string>('warning')
  const [cooldown, setCooldown] = useState<string>('900')
  const [name, setName] = useState<string>('')
  const [notes, setNotes] = useState<string>('')

  const createRule = useCreateRuleMutation()
  const testFire = useTestFireMutation()

  // Plan 07-10: backtest mutation — calls POST /api/alerts/backtest read-only.
  const backtestMutation = useMutation({
    mutationFn: ({ kind, mpId }: { kind: string; mpId: string }) =>
      runBacktest(kind, mpId),
  })

  // Track if scope step is skipped (prefilled entry from MP/Site detail).
  const scopePrefilled = Boolean(meteringPointId || siteId)

  // Plan 07-10 D-42: compute visible rule kinds based on anomaly_compatibility.
  const visibleKinds = useMemo(() => {
    const compat = anomalyCompatibility ?? 'full'
    if (compat === 'unsupported') return [...BASE_RULE_KINDS]
    if (compat === 'limited') return [...BASE_RULE_KINDS, ...ANOMALY_LIMITED]
    return [...BASE_RULE_KINDS, ...ANOMALY_ALL]
  }, [anomalyCompatibility])

  const isUnsupported = anomalyCompatibility === 'unsupported'

  const reset = () => {
    setRuleKind('threshold_instantaneous')
    setScopeKind(meteringPointId ? 'metering_point' : siteId ? 'site' : 'global')
    setHighBound('')
    setComparison('gt')
    setUnit('')
    setSeverity('warning')
    setCooldown('900')
    setName('')
    setNotes('')
    backtestMutation.reset()
  }

  const onSave = async () => {
    const body: Record<string, unknown> = {
      rule_kind: ruleKind,
      scope_kind: scopeKind,
      severity,
      comparison,
    }
    if (meteringPointId && scopeKind === 'metering_point') body.scope_id = meteringPointId
    else if (siteId && scopeKind === 'site') body.scope_id = siteId
    if (highBound) body.high_bound = Number.parseFloat(highBound)
    if (unit) body.unit = unit
    if (cooldown) body.cooldown_seconds = Number.parseInt(cooldown, 10)
    if (name) body.name = name
    if (notes) body.notes = notes

    try {
      const rule = await createRule.mutateAsync(body)
      toast.success(`Rule created: ${rule.rule_kind}`)
      reset()
      onOpenChange(false)
    } catch (err) {
      toast.error(`Save failed: ${(err as Error).message}`)
    }
  }

  const onTestFire = async () => {
    // Save first, then test-fire the newly-saved rule.
    try {
      const body: Record<string, unknown> = {
        rule_kind: ruleKind,
        scope_kind: scopeKind,
        severity,
        comparison,
      }
      if (meteringPointId && scopeKind === 'metering_point') body.scope_id = meteringPointId
      if (highBound) body.high_bound = Number.parseFloat(highBound)
      const rule = await createRule.mutateAsync(body)
      await testFire.mutateAsync({ ruleId: rule.id })
      toast.success('Test fire dispatched — auto-clears in 60s')
      reset()
      onOpenChange(false)
    } catch (err) {
      toast.error(`Test fire failed: ${(err as Error).message}`)
    }
  }

  // Plan 07-10: run backtest for the currently-selected rule kind + MP.
  const onRunBacktest = () => {
    const mpId = meteringPointId ?? ''
    if (!mpId) return
    backtestMutation.mutate({ kind: ruleKind, mpId })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Add alert rule</DialogTitle>
          <DialogDescription>
            Configure the rule across 5 steps. {scopePrefilled ? 'Scope is prefilled.' : null}
          </DialogDescription>
        </DialogHeader>

        <div className="flex max-h-[70vh] flex-col gap-4 overflow-y-auto pr-2">
          {/* Step 1 — Scope (skipped when prefilled per D-18) */}
          {!scopePrefilled ? (
            <section data-step="1">
              <h3 className="text-sm font-semibold">Step 1 · Scope</h3>
              <div className="mt-2">
                <Select value={scopeKind} onValueChange={setScopeKind}>
                  <SelectTrigger>
                    <SelectValue placeholder="Scope" />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="global">All meters</SelectItem>
                    <SelectItem value="site">Site (pick later)</SelectItem>
                    <SelectItem value="metering_point">Metering point (pick later)</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </section>
          ) : null}

          {/* Step 2 — Kind (filtered by anomaly_compatibility per Plan 07-10 D-42) */}
          <section data-step="2">
            <h3 className="text-sm font-semibold">
              Step {scopePrefilled ? '1' : '2'} · Kind
            </h3>

            {/* Plan 07-10: unsupported profile banner */}
            {isUnsupported ? (
              <Alert variant="default" className="mt-2">
                <AlertDescription>
                  Anomaly detection is not available for this device type.
                </AlertDescription>
              </Alert>
            ) : null}

            <Select value={ruleKind} onValueChange={setRuleKind}>
              <SelectTrigger>
                <SelectValue placeholder="Rule kind" />
              </SelectTrigger>
              <SelectContent>
                {visibleKinds.map((k) => (
                  <SelectItem key={k} value={k}>
                    {k}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </section>

          {/* Step 3 — Conditions */}
          <section data-step="3">
            <h3 className="text-sm font-semibold">
              Step {scopePrefilled ? '2' : '3'} · Conditions
            </h3>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <Label htmlFor="comparison">Comparison</Label>
                <Select value={comparison} onValueChange={setComparison}>
                  <SelectTrigger id="comparison">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="gt">{'>'}</SelectItem>
                    <SelectItem value="gte">{'≥'}</SelectItem>
                    <SelectItem value="lt">{'<'}</SelectItem>
                    <SelectItem value="lte">{'≤'}</SelectItem>
                    <SelectItem value="eq">{'='}</SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div>
                <Label htmlFor="high">High bound</Label>
                <Input
                  id="high"
                  type="number"
                  value={highBound}
                  onChange={(e) => setHighBound(e.target.value)}
                />
              </div>
              <div className="col-span-2">
                <Label htmlFor="unit">Unit (optional)</Label>
                <Input
                  id="unit"
                  value={unit}
                  onChange={(e) => setUnit(e.target.value)}
                  placeholder="kWh, m³/h, …"
                />
              </div>
            </div>

            {/* Plan 07-10: backtest button + result panel (anomaly rules only, when MP is known) */}
            {meteringPointId ? (
              <div className="mt-3 flex flex-col gap-2">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={onRunBacktest}
                  disabled={backtestMutation.isPending}
                >
                  Test against last 30 days
                </Button>

                {backtestMutation.isPending && <Skeleton className="h-12" />}

                {backtestMutation.data && (
                  <BacktestResultPanel result={backtestMutation.data} />
                )}
              </div>
            ) : null}
          </section>

          {/* Step 4 — Severity + Cooldown + Notes */}
          <section data-step="4">
            <h3 className="text-sm font-semibold">
              Step {scopePrefilled ? '3' : '4'} · Severity + Cooldown
            </h3>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <Label htmlFor="severity">Severity</Label>
                <Select value={severity} onValueChange={setSeverity}>
                  <SelectTrigger id="severity">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {SEVERITIES.map((s) => (
                      <SelectItem key={s} value={s}>
                        {s}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div>
                <Label htmlFor="cooldown">Cooldown (s)</Label>
                <Input
                  id="cooldown"
                  type="number"
                  value={cooldown}
                  onChange={(e) => setCooldown(e.target.value)}
                />
              </div>
              <div className="col-span-2">
                <Label htmlFor="name">Name (optional)</Label>
                <Input id="name" value={name} onChange={(e) => setName(e.target.value)} />
              </div>
              <div className="col-span-2">
                <Label htmlFor="notes">Notes</Label>
                <Textarea
                  id="notes"
                  value={notes}
                  onChange={(e) => setNotes(e.target.value)}
                  rows={3}
                />
              </div>
            </div>
          </section>

          {/* Step 5 — Review */}
          <section data-step="5">
            <h3 className="text-sm font-semibold">
              Step {scopePrefilled ? '4' : '5'} · Review
            </h3>
            <div className="mt-2 rounded-md border border-border bg-muted/30 p-3 text-xs font-mono">
              {ruleKind} · {scopeKind} · {comparison} {highBound || '?'} {unit} · {severity} ·
              cooldown {cooldown}s
            </div>
          </section>
        </div>

        <DialogFooter className="gap-2">
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button variant="outline" onClick={onTestFire} disabled={createRule.isPending || testFire.isPending}>
            Test fire
          </Button>
          <Button onClick={onSave} disabled={createRule.isPending}>
            {createRule.isPending ? 'Saving…' : 'Save rule'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ── BacktestResultPanel ──────────────────────────────────────────────────────

/**
 * Renders the backtest result: fire count headline + 30-bar sparkline.
 * Zero-fires case shows the "Defaults may be well-tuned for this meter." copy.
 */
function BacktestResultPanel({ result }: { result: BacktestResponse }) {
  return (
    <div className="mt-1 rounded-md border border-border p-2">
      <p className="text-sm">
        {result.fires_count} fires in the last 30 days
        {result.fires_count === 0 ? (
          <span className="text-muted-foreground">
            {' '}— this rule would not have fired. Defaults may be well-tuned for this meter.
          </span>
        ) : null}
      </p>
      <BarChart
        width={300}
        height={48}
        data={result.daily_fires}
        margin={{ top: 0, right: 0, bottom: 0, left: 0 }}
        aria-label="Daily fire counts over the last 30 days"
      >
        <Bar dataKey="count" />
      </BarChart>
    </div>
  )
}
