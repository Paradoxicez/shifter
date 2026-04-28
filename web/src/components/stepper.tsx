import { Check } from 'lucide-react'

export interface StepperProps {
  steps: { label: string }[]
  /** 0-based; values < currentIndex are complete, equal is active, greater is future. */
  currentIndex: number
}

/**
 * UI-SPEC §Stepped dialog pattern: numbered horizontal step list with
 * checkmarks for completed. Phase 1 install wizard uses this; Phase 2
 * add-device, Phase 5 floor plan upload, Phase 7 codec runner inherit.
 */
export function Stepper({ steps, currentIndex }: StepperProps) {
  return (
    <ol className="flex items-center gap-4">
      {steps.map((step, idx) => {
        const isComplete = idx < currentIndex
        const isActive = idx === currentIndex
        const dotClass = isComplete
          ? 'bg-success text-success-foreground'
          : isActive
            ? 'bg-primary text-primary-foreground'
            : 'bg-secondary text-muted-foreground'
        return (
          <li key={step.label} className="flex items-center gap-2">
            <span
              className={`flex h-7 w-7 items-center justify-center rounded-full text-sm font-semibold ${dotClass}`}
            >
              {isComplete ? <Check className="h-4 w-4" aria-hidden="true" /> : idx + 1}
            </span>
            <span
              className={`text-sm ${isActive ? 'font-semibold' : 'text-muted-foreground'}`}
            >
              {step.label}
            </span>
          </li>
        )
      })}
    </ol>
  )
}
