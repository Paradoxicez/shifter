/**
 * RestoreGuidanceCard — static card with restore documentation link.
 *
 * Provides a prominent link to restore documentation. Always visible
 * (admin + viewer) — D-47 restore guidance requirement, Plan 06-10.
 */

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'

export function RestoreGuidanceCard() {
  return (
    <Card data-testid="restore-guidance-card">
      <CardHeader>
        <CardTitle>Restore from backup</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        <p className="text-sm text-muted-foreground">
          To restore Shifter from a backup archive, follow the step-by-step restore guide.
          Backups include the full Postgres dump and any uploaded files.
        </p>
        <Button asChild variant="outline" size="sm" className="w-fit">
          <a
            href="/docs/restore"
            target="_blank"
            rel="noopener noreferrer"
            data-testid="restore-docs-link"
          >
            Open restore guide
          </a>
        </Button>
      </CardContent>
    </Card>
  )
}
