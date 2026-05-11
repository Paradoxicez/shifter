/**
 * HexPayloadCell — Plan 04-09 Task 1
 *
 * Renders D-18 hex payload (lowercase space-separated bytes) in monospace with word-wrap.
 * Includes a copy-to-clipboard button with Sonner toast feedback.
 */

import { Copy } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'

interface HexPayloadCellProps {
  hex: string
}

export function HexPayloadCell({ hex }: HexPayloadCellProps) {
  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(hex)
      toast.success('Hex payload copied to clipboard')
    } catch {
      toast.error('Failed to copy to clipboard')
    }
  }

  return (
    <div className="flex flex-col gap-1">
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs text-muted-foreground uppercase tracking-wide font-medium">
          Raw payload
        </span>
        <Button variant="ghost" size="sm" onClick={handleCopy} title="Copy hex payload">
          <Copy className="h-3 w-3" />
        </Button>
      </div>
      <pre className="text-xs font-mono break-all whitespace-pre-wrap bg-muted/30 rounded p-2">
        {hex}
      </pre>
    </div>
  )
}
