/**
 * BackupHistoryList — collapsible list of up to 5 recent backup runs.
 *
 * Columns: date/time, trigger, status, size, sha256 (first 8 chars).
 * Plan 06-10 / SETT-05.
 */

import { useState } from 'react'
import { Button } from '@/components/ui/button'

export interface BackupRunSummary {
  id: string
  file_name: string
  sha256: string
  started_at: string
  finished_at: string | null
  status: string
  trigger_kind: string
  size_bytes: number
  age_seconds: number
}

interface BackupHistoryListProps {
  runs: BackupRunSummary[]
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function formatDate(iso: string): string {
  try {
    return new Date(iso).toLocaleString(undefined, {
      dateStyle: 'short',
      timeStyle: 'short',
    })
  } catch {
    return iso
  }
}

export function BackupHistoryList({ runs }: BackupHistoryListProps) {
  const [expanded, setExpanded] = useState(false)
  const visible = expanded ? runs : runs.slice(0, 3)

  if (runs.length === 0) {
    return (
      <p className="text-xs text-muted-foreground" data-testid="backup-history-empty">
        No backup runs recorded yet.
      </p>
    )
  }

  return (
    <div data-testid="backup-history-list">
      <table className="w-full text-xs">
        <thead>
          <tr className="border-b text-left text-muted-foreground">
            <th className="pb-1 pr-3 font-medium">Date</th>
            <th className="pb-1 pr-3 font-medium">Trigger</th>
            <th className="pb-1 pr-3 font-medium">Status</th>
            <th className="pb-1 pr-3 font-medium">Size</th>
            <th className="pb-1 font-medium">SHA256</th>
          </tr>
        </thead>
        <tbody>
          {visible.map((run) => (
            <tr key={run.id} className="border-b last:border-0">
              <td className="py-1.5 pr-3 tabular-nums">{formatDate(run.started_at)}</td>
              <td className="py-1.5 pr-3 capitalize">{run.trigger_kind}</td>
              <td className="py-1.5 pr-3 capitalize">{run.status}</td>
              <td className="py-1.5 pr-3 tabular-nums">{formatBytes(run.size_bytes)}</td>
              <td className="py-1.5 font-mono">{run.sha256.slice(0, 8)}</td>
            </tr>
          ))}
        </tbody>
      </table>

      {runs.length > 3 && (
        <Button
          variant="ghost"
          size="sm"
          className="mt-1 h-auto px-0 text-xs"
          onClick={() => setExpanded((v) => !v)}
          data-testid="backup-history-toggle"
        >
          {expanded ? 'Show less' : `Show all ${runs.length}`}
        </Button>
      )}
    </div>
  )
}
