import {
  Cpu,
  FileText,
  LayoutDashboard,
  Layers,
  Map,
  MapPin,
  Radio,
  Settings as SettingsIcon,
  Upload,
} from 'lucide-react'
import { NavLink } from 'react-router-dom'
import { useCurrentUser } from '@/lib/use-current-user'

/**
 * Phase 3 §Layout sidebar order (03-UI-SPEC):
 *   Gateways → Sites → Devices → Profiles → Settings.
 *
 * Plan 03-09 appends admin-only nav under the Admin label:
 *   Imports (`/admin/imports`) — D-36 + T-3-90 mitigation.
 *
 * Dashboard at top — operator's primary surface.
 *
 * Plan 05-08 adds Reports + Map between Dashboard and Gateways (UI-SPEC §Sidebar):
 *   Dashboard → Reports → Map → Gateways → Sites → Devices → Profiles → Settings
 */
const NAV = [
  { to: '/', label: 'Dashboard', icon: LayoutDashboard },
  { to: '/reports', label: 'Reports', icon: FileText },
  { to: '/map', label: 'Map', icon: Map },
  { to: '/gateways', label: 'Gateways', icon: Radio },
  { to: '/sites', label: 'Sites', icon: MapPin },
  { to: '/devices', label: 'Devices', icon: Cpu },
  { to: '/profiles', label: 'Profiles', icon: Layers },
  { to: '/settings', label: 'Settings', icon: SettingsIcon },
]

const ADMIN_NAV = [
  { to: '/admin/imports', label: 'Imports', icon: Upload },
]

/**
 * UI-SPEC §App shell §Sidebar — w-56 on md+, hidden on <md (mobile uses
 * a Sheet drawer triggered from Topbar). Phase 2 adds Sites/Devices/Profiles
 * ahead of Settings.
 * Active state uses bg-primary; hover uses bg-secondary.
 */
export function Sidebar() {
  const user = useCurrentUser()
  const isAdmin = user?.role === 'admin'
  return (
    <aside className="hidden w-56 shrink-0 border-r bg-card md:block">
      <nav className="flex flex-col gap-1 p-3">
        {NAV.map(({ to, label, icon: Icon }) => (
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
            {label}
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
