/**
 * MapEmptyState — Plan 05-08
 *
 * Shown on /map when there are zero sites and zero gateways.
 * Mirrors Phase 4 D-21 empty-state card pattern.
 */

import { Map } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardTitle,
} from '@/components/ui/card'

export interface MapEmptyStateProps {
  onAddSite: () => void
}

export function MapEmptyState({ onAddSite }: MapEmptyStateProps) {
  return (
    <Card className="max-w-md mx-auto mt-32">
      <CardContent className="pt-6 text-center space-y-4">
        <div className="mx-auto h-12 w-12 bg-primary/10 rounded-full flex items-center justify-center">
          <Map className="h-6 w-6 text-primary" />
        </div>
        <div>
          <CardTitle>No locations on the map</CardTitle>
          <CardDescription>
            Add a site or gateway to see them plotted here.
          </CardDescription>
        </div>
        <Button onClick={onAddSite}>Add a site</Button>
      </CardContent>
    </Card>
  )
}
