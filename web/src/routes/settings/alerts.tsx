/**
 * Plan 06-04 — Settings → Alerts page (UI-SPEC §Surface 3).
 *
 * Top card: AnomalyRosterSection (eligible / warming-up counts +
 * expandable roster grouped by status).
 * Bottom card: rule library — table of rules with row toggle + Add Rule
 * button + Show disabled toggle (?show_disabled URL param).
 */

import { useState } from 'react'
import { Link } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Switch } from '@/components/ui/switch'
import { apiFetch } from '@/lib/api'
import { useAlertRulesList } from '@/hooks/useAlerts'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { useCurrentUser } from '@/lib/use-current-user'
import { AddRuleDialog } from './AddRuleDialog'
import { AnomalyRosterSection } from './AnomalyRosterSection'

export default function AlertRulesPage() {
  const user = useCurrentUser()
  const isAdmin = user?.role === 'admin'
  const [showDisabled, setShowDisabled] = useState(false)
  const [dialogOpen, setDialogOpen] = useState(false)
  const { data, isLoading } = useAlertRulesList(showDisabled)
  const qc = useQueryClient()

  const toggleRule = useMutation({
    mutationFn: async (input: { id: string; enable: boolean }) => {
      const action = input.enable ? 'enable' : 'disable'
      return apiFetch(`/api/alerts/rules/${input.id}/${action}`, { method: 'POST' })
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ['alert-rules'] }),
  })

  return (
    <div className="flex flex-col gap-6">
      <header className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold">Settings → Alerts</h1>
        <Link to="/settings" className="text-sm text-muted-foreground underline">
          ← Back to Settings
        </Link>
      </header>

      <AnomalyRosterSection />

      <Card>
        <CardHeader className="flex flex-row items-center justify-between">
          <CardTitle>Rule library</CardTitle>
          <div className="flex items-center gap-3">
            <label className="flex items-center gap-2 text-xs">
              <Switch checked={showDisabled} onCheckedChange={setShowDisabled} />
              Show disabled
            </label>
            {isAdmin ? (
              <Button size="sm" onClick={() => setDialogOpen(true)}>
                Add rule
              </Button>
            ) : null}
          </div>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <div className="text-sm text-muted-foreground">Loading rules…</div>
          ) : (data?.rules.length ?? 0) === 0 ? (
            <div className="rounded-md border border-dashed border-border p-8 text-center text-sm text-muted-foreground">
              No rules yet.{' '}
              {isAdmin ? (
                <button className="underline" onClick={() => setDialogOpen(true)} type="button">
                  Add your first rule
                </button>
              ) : null}
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Enabled</TableHead>
                  <TableHead>Kind</TableHead>
                  <TableHead>Scope</TableHead>
                  <TableHead>Severity</TableHead>
                  <TableHead>Cooldown</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data?.rules.map((rule) => {
                  const enabled = !rule.disabled_at
                  return (
                    <TableRow key={rule.id} data-disabled={!enabled}>
                      <TableCell>
                        <Switch
                          checked={enabled}
                          disabled={!isAdmin || toggleRule.isPending}
                          onCheckedChange={(next) => {
                            toggleRule.mutate(
                              { id: rule.id, enable: next },
                              {
                                onSuccess: () =>
                                  toast.success(next ? 'Rule enabled' : 'Rule disabled'),
                                onError: (e) =>
                                  toast.error(`Toggle failed: ${(e as Error).message}`),
                              },
                            )
                          }}
                          aria-label={enabled ? 'Disable rule' : 'Enable rule'}
                        />
                      </TableCell>
                      <TableCell className="font-mono text-xs">{rule.rule_kind}</TableCell>
                      <TableCell className="text-xs">{rule.scope_kind}</TableCell>
                      <TableCell className="text-xs">{rule.severity}</TableCell>
                      <TableCell className="text-xs">{rule.cooldown_seconds}s</TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      {isAdmin ? <AddRuleDialog open={dialogOpen} onOpenChange={setDialogOpen} /> : null}
    </div>
  )
}
