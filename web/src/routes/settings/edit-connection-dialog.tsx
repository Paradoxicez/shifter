import { useState } from 'react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { ApiError } from '@/lib/api'
import { putChirpStackSettings, type ChirpStackSettings } from '@/lib/settings'

export interface EditConnectionDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  current: ChirpStackSettings
  onSaved: () => void
}

/**
 * Edit ChirpStack connection dialog (SETT-03 + Open Question 2 recommendation).
 *
 * UI-SPEC verbatim copy:
 *   - Title: "Edit ChirpStack connection"
 *   - Description: "Changes apply immediately. We'll re-test the connection
 *     after you save."
 *   - Submit idle: "Save and test"
 *   - Submit loading: "Saving…"
 *
 * The PUT handler re-runs Dial + ProbeVersion + PingMQTT BEFORE persisting;
 * we surface the resulting 422 errors as inline Alerts mapped per error code.
 *
 * Footer order locks Plan 11's Cancel-LEFT, primary-RIGHT convention.
 */
export function EditConnectionDialog({
  open,
  onOpenChange,
  current,
  onSaved,
}: EditConnectionDialogProps) {
  const [mode, setMode] = useState<'bundled' | 'external'>(current.mode)
  const [grpcUrl, setGrpcUrl] = useState(current.grpc_url)
  const [apiToken, setApiToken] = useState('') // empty = keep stored token
  const [mqttUrl, setMqttUrl] = useState(current.mqtt_url)
  const [mqttUser, setMqttUser] = useState(current.mqtt_user)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)
    setBusy(true)
    try {
      await putChirpStackSettings({
        mode,
        grpc_url: grpcUrl,
        api_token: apiToken || undefined,
        mqtt_url: mqttUrl,
        mqtt_user: mqttUser || undefined,
        region_name: current.region.name,
        region_common_name: current.region.common_name,
      })
      onSaved()
      onOpenChange(false)
    } catch (err) {
      if (err instanceof ApiError && err.status === 422) {
        const body = err.body as { error?: string; detail?: string } | null
        if (body?.error === 'v3_detected') {
          setError("Shifter doesn't support ChirpStack v3. Upgrade and try again.")
        } else if (body?.error === 'grpc_unreachable') {
          setError(`Couldn't reach ChirpStack: ${body.detail ?? 'unreachable'}`)
        } else if (body?.error === 'mqtt_unreachable') {
          setError(`Couldn't reach MQTT: ${body.detail ?? 'unreachable'}`)
        } else if (body?.error === 'invalid_mode' || body?.error === 'missing_fields') {
          setError('Please fill in every required field.')
        } else {
          setError(body?.error ?? 'Validation failed.')
        }
      } else {
        setError('Something went wrong. Try again.')
      }
    } finally {
      setBusy(false)
    }
  }

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={onOpenChange}
      title="Edit ChirpStack connection"
      description="Changes apply immediately. We'll re-test the connection after you save."
      footer={
        <>
          <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button form="edit-cs-form" type="submit" disabled={busy}>
            {busy ? 'Saving…' : 'Save and test'}
          </Button>
        </>
      }
    >
      <form id="edit-cs-form" onSubmit={submit} className="flex flex-col gap-4">
        {error ? (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}
        <fieldset className="flex flex-col gap-2">
          <legend className="text-sm font-semibold">Mode</legend>
          <label className="flex items-center gap-2">
            <input type="radio" checked={mode === 'bundled'} onChange={() => setMode('bundled')} />
            Bundled
          </label>
          <label className="flex items-center gap-2">
            <input
              type="radio"
              checked={mode === 'external'}
              onChange={() => setMode('external')}
            />
            External
          </label>
        </fieldset>
        <div className="flex flex-col gap-2">
          <Label htmlFor="ec-grpc">gRPC URL</Label>
          <Input
            id="ec-grpc"
            required
            value={grpcUrl}
            onChange={(e) => setGrpcUrl(e.target.value)}
            className="font-mono"
          />
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="ec-token">New API token (leave blank to keep current)</Label>
          <Input
            id="ec-token"
            type="password"
            autoComplete="new-password"
            value={apiToken}
            onChange={(e) => setApiToken(e.target.value)}
          />
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="ec-mqtt">MQTT URL</Label>
          <Input
            id="ec-mqtt"
            required
            value={mqttUrl}
            onChange={(e) => setMqttUrl(e.target.value)}
            className="font-mono"
          />
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="ec-mqtt-user">MQTT user</Label>
          <Input
            id="ec-mqtt-user"
            value={mqttUser}
            onChange={(e) => setMqttUser(e.target.value)}
          />
        </div>
      </form>
    </ResponsiveDialog>
  )
}
