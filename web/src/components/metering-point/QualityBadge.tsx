/**
 * QualityBadge — Plan 04-09 Task 1
 *
 * D-19 quality badge handoff:
 *   - flaggedCount=0 or windowSize=0: renders subtle "All ok" Badge (no click target)
 *   - flaggedCount>0: renders clickable warning Badge
 *     On click: navigates to ?tab=uplinks&quality=decode_fail,missing_canonical,out_of_range,duplicate_fcnt
 *
 * T-04-09-01: admin-only CTA — caller wraps Can() if needed; this component is purely display.
 */

import { useNavigate } from 'react-router-dom'
import { AlertTriangle, CheckCircle2 } from 'lucide-react'
import { Badge } from '@/components/ui/badge'

interface QualityBadgeProps {
  flaggedCount: number
  windowSize: number
}

const NON_OK_QUALITY = 'decode_fail,missing_canonical,out_of_range,duplicate_fcnt'

export function QualityBadge({ flaggedCount, windowSize }: QualityBadgeProps) {
  const navigate = useNavigate()

  if (flaggedCount === 0 || windowSize === 0) {
    return (
      <Badge variant="secondary" className="gap-1.5">
        <CheckCircle2 className="h-3 w-3 text-success" />
        <span className="text-xs">All ok</span>
      </Badge>
    )
  }

  const handleClick = () => {
    const params = new URLSearchParams(window.location.search)
    params.set('tab', 'uplinks')
    params.set('quality', NON_OK_QUALITY)
    navigate(`?${params.toString()}`)
  }

  return (
    <button type="button" onClick={handleClick} className="text-xs font-semibold">
      <Badge variant="outline" className="gap-1.5 hover:bg-warning/10">
        <AlertTriangle className="h-3 w-3 text-warning" />
        {flaggedCount} of last {windowSize} uplinks flagged
      </Badge>
    </button>
  )
}
