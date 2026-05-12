/**
 * Plan 06-04 — Add Rule dialog (UI-SPEC §"Add Rule dialog — 5 steps").
 *
 * Steps:
 *  1. Scope         — SKIPPED when invoked with meteringPointId/siteId
 *  2. Kind
 *  3. Conditions
 *  4. Severity + Cooldown + Notes
 *  5. Review        — Save + Test fire button
 *
 * For v1 we keep the form lean — every step renders inline rather than a
 * separate stepper UI — so the dialog stays compact and operator can scan
 * the entire shape at once. The plan body's 5-step ordering is preserved
 * as visual section headers so the contract with UI-SPEC remains stable.
 */

import { useState } from 'react'
import { toast } from 'sonner'
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
import { Textarea } from '@/components/ui/textarea'
import { useCreateRuleMutation, useTestFireMutation } from '@/hooks/useAlerts'

export interface AddRuleDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** When set, step 1 (Scope) is skipped — UI-SPEC D-18 prefilled-from-MP entry */
  meteringPointId?: string
  siteId?: string
}

const RULE_KINDS = [
  'threshold_instantaneous',
  'threshold_hourly',
  'threshold_daily',
  'offline_device',
  'anomaly_p95',
  'anomaly_iqr',
  'anomaly_quiet_hour',
] as const

const SEVERITIES = ['critical', 'warning', 'info'] as const

export function AddRuleDialog({ open, onOpenChange, meteringPointId, siteId }: AddRuleDialogProps) {
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

  // Track if scope step is skipped (prefilled entry from MP/Site detail).
  const scopePrefilled = Boolean(meteringPointId || siteId)

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

          {/* Step 2 — Kind */}
          <section data-step="2">
            <h3 className="text-sm font-semibold">
              Step {scopePrefilled ? '1' : '2'} · Kind
            </h3>
            <Select value={ruleKind} onValueChange={setRuleKind}>
              <SelectTrigger>
                <SelectValue placeholder="Rule kind" />
              </SelectTrigger>
              <SelectContent>
                {RULE_KINDS.map((k) => (
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
