/**
 * EmptyStateOnboarding — Plan 04-07 Task 1
 *
 * 3-state progressive empty-state per D-21:
 *   (0, _, _) → "Add your first gateway"  → CTA → /gateways
 *   (>0, 0, _) → "Now add your first device" → CTA → /devices
 *   (>0, >0, 0) → "Waiting for first uplink…" + spinner
 *
 * CTAs use react-router-dom <Link> for client-side navigation.
 */

import { Cpu, Loader2, Wifi } from 'lucide-react'
import { Link } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'

export interface EmptyStateOnboardingProps {
  onboarding: {
    gateway_count: number
    device_count: number
    uplink_count: number
  }
}

export function EmptyStateOnboarding({ onboarding }: EmptyStateOnboardingProps) {
  const { gateway_count, device_count } = onboarding

  // Stage 1: No gateways
  if (gateway_count === 0) {
    return (
      <Card className="mx-auto max-w-md text-center">
        <CardHeader>
          <div className="mx-auto mb-2 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
            <Wifi className="h-6 w-6 text-primary" aria-hidden="true" />
          </div>
          <CardTitle>Add your first gateway</CardTitle>
          <CardDescription>
            A gateway connects your LoRaWAN devices to Shifter. Add one to get started.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Button asChild>
            <Link to="/gateways">Add gateway</Link>
          </Button>
        </CardContent>
      </Card>
    )
  }

  // Stage 2: No devices
  if (device_count === 0) {
    return (
      <Card className="mx-auto max-w-md text-center">
        <CardHeader>
          <div className="mx-auto mb-2 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
            <Cpu className="h-6 w-6 text-primary" aria-hidden="true" />
          </div>
          <CardTitle>Now add your first device</CardTitle>
          <CardDescription>
            Your gateway is ready. Add a meter device to start collecting readings.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Button asChild>
            <Link to="/devices">Add device</Link>
          </Button>
        </CardContent>
      </Card>
    )
  }

  // Stage 3: Waiting for first uplink
  return (
    <Card className="mx-auto max-w-md text-center">
      <CardHeader>
        <div className="mx-auto mb-2 flex h-12 w-12 items-center justify-center rounded-full bg-primary/10">
          <Loader2 className="h-6 w-6 animate-spin text-primary" aria-hidden="true" />
        </div>
        <CardTitle>Waiting for first uplink…</CardTitle>
        <CardDescription>
          Your device is registered and the gateway is listening. The dashboard will appear once the
          first reading arrives. This typically takes up to one reporting interval.
        </CardDescription>
      </CardHeader>
    </Card>
  )
}
