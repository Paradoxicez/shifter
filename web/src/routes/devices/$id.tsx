import { useQuery } from '@tanstack/react-query'
import { KeyRound, Radio } from 'lucide-react'
import { useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { getDevice, type Device } from '@/lib/devices'
import { useCurrentUser } from '@/lib/use-current-user'
import { RevealKeysDialog } from './reveal-keys-dialog'

/**
 * UI-SPEC §Device detail page (`/devices/:id`) — Plan 03-10.
 *
 *   - Header: name (text-2xl) + dev_eui (text-sm font-mono) + activation /
 *     last-seen meta row + admin action row (Reveal keys).
 *   - Identity card: name, DevEUI, profile, description.
 *   - Binding card: current site link + last-seen relative time.
 *   - Admin-only action row hosts the Reveal keys button (DEV-09).
 *
 * Data: useQuery(['device', id]) → getDevice(id). 404 → "Device not found".
 *
 * UX-03: vocabulary stays in the Shifter dialect — site / device / metering
 * point / DevEUI. No "tenant" or user-facing "application" wording.
 */
function relativeFromNow(iso: string | null | undefined): string {
  if (!iso) return 'never'
  const t = Date.parse(iso)
  if (Number.isNaN(t)) return 'unknown'
  const diff = Date.now() - t
  const min = Math.floor(diff / 60_000)
  if (min < 1) return 'just now'
  if (min < 60) return `${min} min ago`
  const hr = Math.floor(min / 60)
  if (hr < 24) return `${hr} hr ago`
  const day = Math.floor(hr / 24)
  return `${day} d ago`
}

function ActivationChip({ device }: { device: Device }) {
  if (device.decommissioned_at) {
    return (
      <Badge variant="secondary" className="text-xs">
        Decommissioned
      </Badge>
    )
  }
  if (!device.last_seen_at) {
    return (
      <Badge variant="outline" className="text-xs">
        Never joined
      </Badge>
    )
  }
  const t = Date.parse(device.last_seen_at)
  const stale = !Number.isNaN(t) && Date.now() - t > 24 * 60 * 60 * 1000
  return stale ? (
    <Badge variant="outline" className="text-xs">
      Inactive
    </Badge>
  ) : (
    <Badge
      variant="default"
      className="text-xs"
      style={{ backgroundColor: 'var(--success)' }}
    >
      Active
    </Badge>
  )
}

export default function DeviceDetailPage() {
  const { id = '' } = useParams<{ id: string }>()
  const user = useCurrentUser()
  const isAdmin = user?.role === 'admin'

  const [revealOpen, setRevealOpen] = useState(false)

  const deviceQuery = useQuery({
    queryKey: ['device', id],
    queryFn: () => getDevice(id),
    enabled: Boolean(id),
    retry: false,
  })

  if (deviceQuery.isLoading) {
    return (
      <div className="flex flex-col gap-6 p-6">
        <Skeleton className="h-8 w-64" />
        <Skeleton className="h-40 w-full" />
        <Skeleton className="h-40 w-full" />
      </div>
    )
  }

  if (deviceQuery.error || !deviceQuery.data) {
    return (
      <div className="m-6 flex flex-col items-center gap-4 rounded-md border p-12 text-center">
        <h1 className="text-2xl font-semibold leading-8">Device not found</h1>
        <p className="text-sm text-muted-foreground">
          The device you requested doesn&rsquo;t exist or you don&rsquo;t have
          access.
        </p>
        <Button asChild variant="outline">
          <Link to="/devices">Back to devices</Link>
        </Button>
      </div>
    )
  }

  const d = deviceQuery.data
  const isDecommissioned = d.decommissioned_at != null

  return (
    <div className="flex flex-col gap-6 p-6">
      <header className="flex items-start justify-between gap-4">
        <div className="flex flex-col gap-1">
          <h1 className="text-2xl font-semibold leading-8">{d.name}</h1>
          <p className="text-sm font-mono text-muted-foreground">{d.dev_eui}</p>
          <div className="mt-1 flex items-center gap-2">
            <ActivationChip device={d} />
            <span className="text-sm text-muted-foreground">
              <Radio
                className="mr-1 inline-block h-3.5 w-3.5"
                aria-hidden="true"
              />
              Last seen {relativeFromNow(d.last_seen_at)}
            </span>
          </div>
        </div>
        {isAdmin && !isDecommissioned ? (
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              onClick={() => setRevealOpen(true)}
              aria-label="Reveal keys"
            >
              <KeyRound className="mr-2 h-4 w-4" aria-hidden="true" />
              Reveal keys
            </Button>
          </div>
        ) : null}
      </header>

      {isDecommissioned ? (
        <Alert>
          <AlertTitle>This device is decommissioned.</AlertTitle>
          <AlertDescription>
            Decommissioned on {d.decommissioned_at?.slice(0, 10) ?? 'unknown'}.
            Historical telemetry is preserved against the original metering
            point.
          </AlertDescription>
        </Alert>
      ) : null}

      <Card>
        <CardHeader>
          <CardTitle>Identity</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-3 text-sm">
          <div>
            <span className="font-semibold">Name:</span>{' '}
            <span>{d.name}</span>
          </div>
          <div>
            <span className="font-semibold">DevEUI:</span>{' '}
            <span className="font-mono">{d.dev_eui}</span>
          </div>
          {d.join_eui ? (
            <div>
              <span className="font-semibold">Join EUI:</span>{' '}
              <span className="font-mono">{d.join_eui}</span>
            </div>
          ) : null}
          {d.description ? (
            <div>
              <span className="font-semibold">Description:</span>{' '}
              <span className="text-muted-foreground">{d.description}</span>
            </div>
          ) : null}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Binding</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-3 text-sm">
          <div>
            <span className="font-semibold">Site:</span>{' '}
            {d.current_site_id ? (
              <Link
                to={`/sites/${d.current_site_id}`}
                className="text-primary hover:underline"
              >
                {d.current_site_name ?? d.current_site_id}
              </Link>
            ) : (
              <span className="text-muted-foreground">No active binding</span>
            )}
          </div>
          <div>
            <span className="font-semibold">Last uplink:</span>{' '}
            <span className="text-muted-foreground">
              {relativeFromNow(d.last_seen_at)}
            </span>
          </div>
        </CardContent>
      </Card>

      {isAdmin ? (
        <RevealKeysDialog
          open={revealOpen}
          onOpenChange={setRevealOpen}
          devEUI={d.dev_eui}
        />
      ) : null}
    </div>
  )
}
