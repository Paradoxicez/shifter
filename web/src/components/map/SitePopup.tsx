/**
 * SitePopup — Plan 05-08
 *
 * Popup content for a site marker.
 * Shows: name, MP count badge, online/offline badges,
 * today's consumption (capability-gated), View site CTA, Get directions OSM link.
 *
 * D-15: "View site" → /sites/:id; "Get directions" → OSM external link.
 * D-04 capability gating: water-only shows only water; electricity-only shows only electricity.
 */

import { Link } from 'react-router-dom'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import type { MapSite } from './MapView'

export interface SitePopupProps {
  site: MapSite
  capabilities: 'water' | 'electricity' | 'both'
}

export function SitePopup({ site, capabilities }: SitePopupProps) {
  // D-15: Get directions via OSM external link
  const directionsURL = `https://www.openstreetmap.org/?mlat=${site.lat}&mlon=${site.lng}#map=16/${site.lat}/${site.lng}`

  const showWater = capabilities === 'water' || capabilities === 'both'
  const showElectricity = capabilities === 'electricity' || capabilities === 'both'

  return (
    <div className="space-y-2 min-w-[220px]">
      <div className="font-semibold text-base">{site.name}</div>
      <div className="flex gap-2 flex-wrap">
        <Badge variant="secondary">
          {site.mp_count} MP{site.mp_count === 1 ? '' : 's'}
        </Badge>
        {site.online_count > 0 && (
          <Badge variant="default">{site.online_count} online</Badge>
        )}
        {site.offline_count > 0 && (
          <Badge variant="destructive">{site.offline_count} offline</Badge>
        )}
      </div>
      {showWater && site.today_consumption.water !== undefined && (
        <div className="text-sm">
          Today (water):{' '}
          <span className="font-mono">
            {site.today_consumption.water.toFixed(3)} m³
          </span>
        </div>
      )}
      {showElectricity && site.today_consumption.electricity !== undefined && (
        <div className="text-sm">
          Today (electricity):{' '}
          <span className="font-mono">
            {site.today_consumption.electricity.toFixed(3)} kWh
          </span>
        </div>
      )}
      <div className="flex gap-2 pt-1">
        <Button asChild size="sm">
          <Link to={`/sites/${site.id}`}>View site</Link>
        </Button>
        <Button asChild size="sm" variant="outline">
          <a href={directionsURL} target="_blank" rel="noreferrer">
            Get directions
          </a>
        </Button>
      </div>
    </div>
  )
}
