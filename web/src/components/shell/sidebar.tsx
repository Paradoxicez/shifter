import {
  Bell,
  Cpu,
  FileText,
  GitCompare,
  LayoutDashboard,
  Layers,
  Map,
  MapPin,
  Radio,
  Settings as SettingsIcon,
  Shield,
  Upload,
} from 'lucide-react'
import { NavLink } from 'react-router-dom'
import { useAlertsRecent } from '@/hooks/useAlerts'
import { useCurrentUser } from '@/lib/use-current-user'

/**
 * Plan 06-04 §Sidebar order (UI-SPEC §Surface 8):
 *   Dashboard → Reports → Map → Gateways → Sites → Devices → Profiles →
 *   Alerts → Audit (admin-only) → Settings
 *
 * Plan 03-09's "Imports" admin nav stays grouped under Admin.
 */
const NAV: Array<{
  to: string
  label: string
  icon: typeof LayoutDashboard
  badge?: 'alerts-unread'
  adminOnly?: boolean
}> = [
  { to: '/', label: 'Dashboard', icon: LayoutDashboard },
  { to: '/reports', label: 'Reports', icon: FileText },
  { to: '/compare', label: 'Compare', icon: GitCompare },
  { to: '/map', label: 'Map', icon: Map },
  { to: '/gateways', label: 'Gateways', icon: Radio },
  { to: '/sites', label: 'Sites', icon: MapPin },
  { to: '/devices', label: 'Devices', icon: Cpu },
  { to: '/profiles', label: 'Profiles', icon: Layers },
  { to: '/alerts', label: 'Alerts', icon: Bell, badge: 'alerts-unread' },
  { to: '/audit', label: 'Audit', icon: Shield, adminOnly: true },
  { to: '/settings', label: 'Settings', icon: SettingsIcon },
]

const ADMIN_NAV = [
  { to: '/admin/imports', label: 'Imports', icon: Upload },
]

function AlertsBadge() {
  const { data } = useAlertsRecent()
  const counts = data?.unread_counts ?? { critical: 0, warning: 0, info: 0 }
  const total = counts.critical + counts.warning + counts.info
  if (total === 0) return null
  const severity =
    counts.critical > 0 ? 'critical' : counts.warning > 0 ? 'warning' : 'info'
  const dotColor: Record<string, string> = {
    critical: 'bg-red-500',
    warning: 'bg-amber-500',
    info: 'bg-blue-500',
  }
  const label = total > 99 ? '99+' : String(total)
  return (
    <span
      className="ml-auto inline-flex items-center gap-1 text-xs font-medium"
      data-severity={severity}
    >
      <span className={`inline-block size-2 rounded-full ${dotColor[severity]}`} aria-hidden />
      {label}
    </span>
  )
}

/**
 * UI-SPEC §App shell §Sidebar — w-56 on md+, hidden on <md (mobile uses
 * a Sheet drawer triggered from Topbar). Phase 2 adds Sites/Devices/Profiles
 * ahead of Settings.
 * Active state uses bg-primary; hover uses bg-secondary.
 */
export function Sidebar() {
  const user = useCurrentUser()
  const isAdmin = user?.role === 'admin'
  const visibleNav = NAV.filter((item) => !item.adminOnly || isAdmin)
  return (
    <aside className="hidden w-56 shrink-0 border-r bg-card md:block">
      <nav className="flex flex-col gap-1 p-3">
        {visibleNav.map(({ to, label, icon: Icon, badge }) => (
          <NavLink
            key={to}
            to={to}
            className={({ isActive }) =>
              `flex items-center gap-2 rounded-md px-3 py-2 text-sm font-semibold ${
                isActive ? 'bg-primary text-primary-foreground' : 'hover:bg-secondary'
              }`
            }
          >
            <Icon className="h-4 w-4" aria-hidden="true" />
            <span>{label}</span>
            {badge === 'alerts-unread' ? <AlertsBadge /> : null}
          </NavLink>
        ))}
        {isAdmin ? (
          <>
            <div className="mt-4 px-3 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
              Admin
            </div>
            {ADMIN_NAV.map(({ to, label, icon: Icon }) => (
              <NavLink
                key={to}
                to={to}
                className={({ isActive }) =>
                  `flex items-center gap-2 rounded-md px-3 py-2 text-sm font-semibold ${
                    isActive
                      ? 'bg-primary text-primary-foreground'
                      : 'hover:bg-secondary'
                  }`
                }
              >
                <Icon className="h-4 w-4" aria-hidden="true" />
                {label}
              </NavLink>
            ))}
          </>
        ) : null}
      </nav>
    </aside>
  )
}
