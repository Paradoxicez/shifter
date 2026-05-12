/**
 * InstallIdentityCard — displays install identity fields (D-47).
 *
 * Shows: install_id, site name, version. Admin-only propagation note
 * informing that install_id is embedded in backup archives and restore
 * targets must have a matching identity (D-47 / Plan 06-10).
 *
 * This card is intentionally static (no mutations). The identity is set
 * at install time and is read-only in the UI.
 */

import { useQuery } from '@tanstack/react-query'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { apiFetch } from '@/lib/api'
import { useCurrentUser } from '@/lib/use-current-user'

interface InstallIdentity {
  install_id: string
  site_name: string
  version: string
}

function fetchInstallIdentity(): Promise<InstallIdentity> {
  return apiFetch<InstallIdentity>('/api/settings/identity')
}

export function InstallIdentityCard() {
  const user = useCurrentUser()
  const isAdmin = user?.role === 'admin'

  const { data, isLoading } = useQuery({
    queryKey: ['settings', 'identity'],
    queryFn: fetchInstallIdentity,
    // Identity rarely changes — long stale time
    staleTime: 5 * 60 * 1000,
  })

  return (
    <Card data-testid="install-identity-card">
      <CardHeader>
        <CardTitle>Install identity</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-2">
        {isLoading || !data ? (
          <Skeleton className="h-16 w-full" />
        ) : (
          <>
            <div className="flex justify-between">
              <span className="text-sm font-medium">Install ID</span>
              <span className="font-mono text-sm" data-testid="install-id">{data.install_id}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-sm font-medium">Site name</span>
              <span className="text-sm" data-testid="site-name">{data.site_name}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-sm font-medium">Version</span>
              <span className="font-mono text-sm" data-testid="version">{data.version}</span>
            </div>

            {/* D-47 propagation note — admin only */}
            {isAdmin && (
              <p className="mt-2 text-xs text-muted-foreground" data-testid="d47-propagation-note">
                The install ID is embedded in backup archives. When restoring on a new host, ensure
                the target install has a matching identity before importing the backup.
              </p>
            )}
          </>
        )}
      </CardContent>
    </Card>
  )
}
