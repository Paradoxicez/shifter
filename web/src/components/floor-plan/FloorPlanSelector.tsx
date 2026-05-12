/**
 * Horizontal pill strip for multi-floor navigation.
 * Ordered by sort_order (server already returns them sorted, but we sort here
 * defensively). Click a pill → setActivePlanID.
 */
export function FloorPlanSelector({
  plans,
  activeID,
  onChange,
}: {
  plans: Array<{ id: string; label: string; sort_order: number }>
  activeID: string | null
  onChange: (id: string) => void
}) {
  if (plans.length <= 1) return null // Single floor — no selector needed

  const sorted = [...plans].sort((a, b) => a.sort_order - b.sort_order)

  return (
    <div
      className="flex gap-1 p-2 border-b bg-card overflow-x-auto shrink-0"
      role="tablist"
      aria-label="Floor plan levels"
    >
      {sorted.map((p) => (
        <button
          key={p.id}
          role="tab"
          aria-selected={p.id === activeID}
          onClick={() => onChange(p.id)}
          className={`px-3 py-1 rounded-full text-sm font-medium transition-colors whitespace-nowrap ${
            p.id === activeID
              ? 'bg-primary text-primary-foreground'
              : 'bg-secondary text-secondary-foreground hover:bg-secondary/80'
          }`}
        >
          {p.label}
        </button>
      ))}
    </div>
  )
}
