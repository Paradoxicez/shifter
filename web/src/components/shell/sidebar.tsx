import { Cpu, Layers, MapPin, Radio, Settings as SettingsIcon } from 'lucide-react'
import { NavLink } from 'react-router-dom'

/**
 * Phase 3 §Layout sidebar order (03-UI-SPEC):
 *   Gateways → Sites → Devices → Profiles → Settings.
 *
 * Gateways sits at the top because in operator mental model the gateway is
 * the network edge — without it, no devices uplink. Phase 4 will insert
 * Dashboard above all; don't pre-reserve that slot.
 */
const NAV = [
  { to: '/gateways', label: 'Gateways', icon: Radio },
  { to: '/sites', label: 'Sites', icon: MapPin },
  { to: '/devices', label: 'Devices', icon: Cpu },
  { to: '/profiles', label: 'Profiles', icon: Layers },
  { to: '/settings', label: 'Settings', icon: SettingsIcon },
]

/**
 * UI-SPEC §App shell §Sidebar — w-56 on md+, hidden on <md (mobile uses
 * a Sheet drawer triggered from Topbar). Phase 2 adds Sites/Devices/Profiles
 * ahead of Settings.
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
