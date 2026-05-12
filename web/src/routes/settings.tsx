import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { fetchSessionUser } from '@/lib/auth'
import {
  fetchChirpStackSettings,
  testChirpStackConnection,
  type ChirpStackSettings,
  type TestConnResult,
} from '@/lib/settings'
import { DataRetentionCard } from '@/components/settings/DataRetentionCard'
import { BackupStatusCard } from '@/components/settings/BackupStatusCard'
import { RestoreGuidanceCard } from '@/components/settings/RestoreGuidanceCard'
import { EditConnectionDialog } from './settings/edit-connection-dialog'
import { TestConnectionPanel } from './settings/test-connection'

/**
 * Phase 1 Settings page.
 *
 * UI-SPEC §"Settings shell (Phase 1)" — two cards: Account + ChirpStack
 * connection. Account card shows email + role badge (Plan 11 backend handler).
 * ChirpStack card shows mode + grpc_url + mqtt_url READ-ONLY (api_token is
 * NEVER displayed — T-17-01 / V8) + Edit / Test Connection actions.
 *
 * The Edit action is admin-only (AUTH-06 frontend hiding); viewers see only
 * the Test Connection button. Backend RequireAction middleware (Plan 18)
 * enforces the same gate at the API level.
 *
 * UI-SPEC verbatim copy:
 *   - "Account" / "ChirpStack connection" card titles
 *   - "Test connection" / "Testing…" button labels
 *   - "Edit connection" admin button label
 *   - "Connection updated" success toast (raised by parent on PUT success)
 */
export default function SettingsPage() {
  const meQ = useQuery({ queryKey: ['me'], queryFn: fetchSessionUser })
  const csQ = useQuery({ queryKey: ['cs-settings'], queryFn: fetchChirpStackSettings })
  const qc = useQueryClient()
  const [editOpen, setEditOpen] = useState(false)
  const [testResult, setTestResult] = useState<TestConnResult | null>(null)

  const testM = useMutation({
    mutationFn: () => {
      if (!csQ.data) throw new Error('settings not loaded')
      return testChirpStackConnection({
        grpc_url: csQ.data.grpc_url,
        api_token: '',
        mqtt_url: csQ.data.mqtt_url,
        mqtt_user: csQ.data.mqtt_user,
      })
    },
    onSuccess: (r) => setTestResult(r),
  })

  return (
    <div className="flex max-w-3xl flex-col gap-12">
      <Card>
        <CardHeader>
          <CardTitle>Account</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-2">
          {meQ.data ? (
            <>
              <div className="flex justify-between">
                <span className="text-sm font-semibold">Email</span>
                <span className="text-sm">{meQ.data.email}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-sm font-semibold">Role</span>
                <Badge variant="secondary">{meQ.data.role}</Badge>
              </div>
            </>
          ) : (
            <Skeleton className="h-8 w-full" />
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>ChirpStack connection</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {csQ.data ? (
            <>
              <div className="flex justify-between">
                <span className="text-sm font-semibold">Mode</span>
                <span className="text-sm">{csQ.data.mode}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-sm font-semibold">gRPC URL</span>
                <span className="font-mono text-sm">{csQ.data.grpc_url}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-sm font-semibold">MQTT URL</span>
                <span className="font-mono text-sm">{csQ.data.mqtt_url}</span>
              </div>
              <div className="flex gap-2 pt-2">
                {meQ.data?.role === 'admin' ? (
                  <Button variant="outline" onClick={() => setEditOpen(true)}>
                    Edit connection
                  </Button>
                ) : null}
                <Button onClick={() => testM.mutate()} disabled={testM.isPending}>
                  {testM.isPending ? 'Testing…' : 'Test connection'}
                </Button>
              </div>
              <TestConnectionPanel result={testResult} />
            </>
          ) : (
            <Skeleton className="h-32 w-full" />
          )}
        </CardContent>
      </Card>

      {/* Users — Phase 6 / Plan 06-05 (D-29 lives under Settings) */}
      {meQ.data?.role === 'admin' ? (
        <Card>
          <CardHeader>
            <CardTitle>Users</CardTitle>
          </CardHeader>
          <CardContent className="flex items-center justify-between">
            <span className="text-sm text-muted-foreground">
              Manage admin and viewer accounts.
            </span>
            <Button asChild variant="outline">
              <Link to="/settings/users">Open</Link>
            </Button>
          </CardContent>
        </Card>
      ) : null}

      {/* Data Retention card — D-09 / DATA-13 / Plan 05-11 + Phase 6 extensions (06-10) */}
      <DataRetentionCard />

      {/* Backup status card — SETT-05 / Plan 06-10 */}
      <BackupStatusCard />

      {/* Restore guidance — D-47 / Plan 06-10 */}
      <RestoreGuidanceCard />

      {csQ.data ? (
        <EditConnectionDialog
          open={editOpen}
          onOpenChange={setEditOpen}
          current={csQ.data as ChirpStackSettings}
          onSaved={() => {
            // UI-SPEC §"Phase 1 copy table": success toast string.
            toast.success('Connection updated')
            qc.invalidateQueries({ queryKey: ['cs-settings'] })
          }}
        />
      ) : null}
    </div>
  )
}
