import { ScrollArea } from '@/components/ui/scroll-area'
import { Badge } from '@/components/ui/badge'
import { CheckCircle2, Crosshair, MapPin } from 'lucide-react'

export function DeviceSidebar({
  unplaced,
  placed,
  placingDeviceID,
  onSelectDevice,
}: {
  unplaced: Array<{ id: string; name: string; utility_class: string }>
  placed: Array<{ id: string; name: string; utility_class: string; state: string }>
  placingDeviceID: string | null
  onSelectDevice: (id: string) => void
}) {
  return (
    <div className="w-64 shrink-0 border-r bg-card p-4 space-y-4 overflow-hidden flex flex-col">
      <section>
        <h3 className="text-xs font-semibold tracking-wide uppercase text-muted-foreground mb-2 flex items-center gap-1">
          Unplaced devices{' '}
          <Badge variant="secondary" className="ml-1">
            {unplaced.length}
          </Badge>
        </h3>
        {unplaced.length === 0 ? (
          <div className="text-center py-6">
            <CheckCircle2 className="h-8 w-8 mx-auto text-success mb-2" />
            <div className="text-xs text-muted-foreground">All devices placed</div>
          </div>
        ) : (
          <ScrollArea className="h-48">
            <div className="space-y-0.5">
              {unplaced.map((d) => (
                <button
                  key={d.id}
                  onClick={() => onSelectDevice(d.id)}
                  className={`block w-full text-left px-3 py-2 rounded text-sm hover:bg-accent transition-colors ${
                    placingDeviceID === d.id ? 'bg-primary/10 border border-primary' : ''
                  }`}
                >
                  <div className="flex items-center gap-1.5">
                    <Crosshair className="h-3 w-3 shrink-0 text-muted-foreground" />
                    <span className="truncate font-medium">{d.name}</span>
                  </div>
                  <div className="text-xs text-muted-foreground ml-4.5 truncate">
                    {d.utility_class}
                  </div>
                </button>
              ))}
            </div>
          </ScrollArea>
        )}
      </section>

      <section className="flex-1 overflow-hidden flex flex-col">
        <h3 className="text-xs font-semibold tracking-wide uppercase text-muted-foreground mb-2 flex items-center gap-1">
          Placed devices{' '}
          <Badge variant="secondary" className="ml-1">
            {placed.length}
          </Badge>
        </h3>
        {placed.length === 0 ? (
          <div className="text-xs text-muted-foreground py-2">No devices placed yet.</div>
        ) : (
          <ScrollArea className="flex-1">
            <div className="space-y-0.5">
              {placed.map((d) => (
                <div key={d.id} className="px-3 py-2 text-sm">
                  <div className="flex items-center gap-1.5">
                    <MapPin className="h-3 w-3 shrink-0 text-muted-foreground" />
                    <span className="truncate font-medium">{d.name}</span>
                  </div>
                  <div className="text-xs text-muted-foreground ml-4.5 truncate">
                    {d.utility_class} · {d.state}
                  </div>
                </div>
              ))}
            </div>
          </ScrollArea>
        )}
      </section>
    </div>
  )
}
