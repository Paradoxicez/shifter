import { Menu } from 'lucide-react'
import shifterLogo from '@/assets/shifter-logo.svg'
import { Button } from '@/components/ui/button'
import { AlertBell } from './AlertBell'

export interface TopbarProps {
  installDisplayName: string
  installLogoUrl?: string
  /** Mobile sidebar trigger (visible on <md). Plan 16/17 wires the sheet drawer. */
  onMenuClick?: () => void
  /** Typically <AccountMenu />, rendered right-aligned. */
  children?: React.ReactNode
}

/**
 * UI-SPEC §App shell §Topbar — sticky h-14, bg-card surface, border-b,
 * install logo (24px) + display name on the left, account-menu slot on
 * the right. Mobile (<md) shows a Menu trigger to open the sidebar sheet.
 *
 * Plan 06-04 mounts <AlertBell /> left of the account menu — severity-tinted
 * dot when there are unread alerts, click opens the slide-over drawer.
 */
export function Topbar({ installDisplayName, installLogoUrl, onMenuClick, children }: TopbarProps) {
  return (
    <header className="sticky top-0 z-20 flex h-14 items-center gap-3 border-b bg-card px-4 md:px-6">
      <Button
        variant="ghost"
        size="icon"
        className="md:hidden"
        aria-label="Open navigation"
        onClick={onMenuClick}
      >
        <Menu className="h-5 w-5" aria-hidden="true" />
      </Button>
      <img src={installLogoUrl ?? shifterLogo} alt="" className="h-6" />
      <span className="text-sm font-semibold">{installDisplayName}</span>
      <div className="ml-auto flex items-center gap-2">
        <AlertBell />
        {children}
      </div>
    </header>
  )
}
