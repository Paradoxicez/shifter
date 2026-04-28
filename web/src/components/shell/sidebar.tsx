import { Settings as SettingsIcon } from 'lucide-react'
import { NavLink } from 'react-router-dom'

const NAV = [{ to: '/settings', label: 'Settings', icon: SettingsIcon }]

/**
 * UI-SPEC §App shell §Sidebar — w-56 on md+, hidden on <md (mobile uses
 * a Sheet drawer triggered from Topbar). Phase 1 ships only the Settings
 * nav item; Phase 2+ adds Sites/Devices/Gateways/Dashboard/Reports/etc.
 * Active state uses bg-primary; hover uses bg-secondary.
 */
export function Sidebar() {
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
      </nav>
    </aside>
  )
}
