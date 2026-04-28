import { useState } from 'react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ApiError } from '@/lib/api'
import { postStep2 } from '@/lib/install'

/**
 * Wizard step 2 — ChirpStack connection.
 *
 * Plan 15's POST /api/install/step/2 dials the gRPC endpoint and runs
 * ProbeVersion. ChirpStack v3 is rejected with 422 v3_detected (INST-05);
 * destructive Alert renders UI-SPEC verbatim copy.
 */
export function ChirpStackStep({ onAdvance }: { onAdvance: () => void }) {
  const [mode, setMode] = useState<'bundled' | 'external'>('bundled')
  const [grpcUrl, setGrpcUrl] = useState('chirpstack:8080')
  const [apiToken, setApiToken] = useState('')
  const [mqttUrl, setMqttUrl] = useState('tcp://mosquitto:1883')
  const [mqttUser, setMqttUser] = useState('')
  const [mqttPass, setMqttPass] = useState('')
  const [v3, setV3] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)
    setV3(false)
    setBusy(true)
    try {
      await postStep2({
        mode,
        grpc_url: grpcUrl,
        api_token: apiToken,
        mqtt_url: mqttUrl,
        mqtt_user: mqttUser || undefined,
        mqtt_password: mqttPass || undefined,
      })
      onAdvance()
    } catch (err) {
      if (err instanceof ApiError && err.status === 422) {
        const body = err.body as { error?: string; detail?: string } | null
        if (body?.error === 'v3_detected') {
          setV3(true)
        } else if (body?.error === 'grpc_unreachable') {
          setError(`Couldn't reach ChirpStack at ${grpcUrl}: ${body.detail ?? 'unreachable'}`)
        } else {
          setError(body?.error ?? 'Something went wrong.')
        }
      } else {
        setError('Something went wrong. Try again.')
      }
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit} className="flex flex-col gap-4">
      <div>
        <h2 className="text-2xl font-semibold leading-8">Connect to ChirpStack</h2>
        <p className="text-sm text-muted-foreground">
          Shifter wraps your ChirpStack v4 server. Choose how you want it deployed.
        </p>
      </div>
      {v3 ? (
        <Alert variant="destructive">
          <AlertTitle>Shifter doesn't support ChirpStack v3</AlertTitle>
          <AlertDescription>
            We detected ChirpStack v3 at this URL. Shifter requires v4 or newer. Upgrade ChirpStack
            and try again.
          </AlertDescription>
        </Alert>
      ) : null}
      {error ? (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}
      <fieldset className="flex flex-col gap-2">
        <legend className="text-sm font-semibold">Mode</legend>
        <label className="flex items-center gap-2">
          <input
            type="radio"
            checked={mode === 'bundled'}
            onChange={() => setMode('bundled')}
          />
          Bundled — Shifter installs ChirpStack for you
        </label>
        <label className="flex items-center gap-2">
          <input
            type="radio"
            checked={mode === 'external'}
            onChange={() => setMode('external')}
          />
          External — connect to an existing ChirpStack
        </label>
      </fieldset>
      <div className="flex flex-col gap-2">
        <Label htmlFor="cs-grpc">gRPC URL</Label>
        <Input
          id="cs-grpc"
          required
          value={grpcUrl}
          onChange={(e) => setGrpcUrl(e.target.value)}
          className="font-mono"
        />
      </div>
      <div className="flex flex-col gap-2">
        <Label htmlFor="cs-token">API token</Label>
        <Input
          id="cs-token"
          type="password"
          required
          value={apiToken}
          onChange={(e) => setApiToken(e.target.value)}
        />
      </div>
      <div className="flex flex-col gap-2">
        <Label htmlFor="cs-mqtt">MQTT URL</Label>
        <Input
          id="cs-mqtt"
          required
          value={mqttUrl}
          onChange={(e) => setMqttUrl(e.target.value)}
          className="font-mono"
        />
      </div>
      <div className="grid grid-cols-2 gap-4">
        <div className="flex flex-col gap-2">
          <Label htmlFor="cs-mqtt-user">MQTT username (optional)</Label>
          <Input
            id="cs-mqtt-user"
            value={mqttUser}
            onChange={(e) => setMqttUser(e.target.value)}
          />
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="cs-mqtt-pass">MQTT password (optional)</Label>
          <Input
            id="cs-mqtt-pass"
            type="password"
            value={mqttPass}
            onChange={(e) => setMqttPass(e.target.value)}
          />
        </div>
      </div>
      <div className="flex justify-end pt-2">
        <Button type="submit" disabled={busy}>
          {busy ? 'Testing connection…' : 'Next'}
        </Button>
      </div>
    </form>
  )
}
