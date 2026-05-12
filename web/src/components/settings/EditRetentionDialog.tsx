import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Button } from '@/components/ui/button'
import { apiFetch } from '@/lib/api'
import type { RetentionLevel } from './DataRetentionCard'

/**
 * EditRetentionDialog — single-field PATCH dialog for a retention level.
 *
 * Uses ResponsiveDialog (UI-SPEC §Dialog Conventions), react-hook-form with
 * zodResolver for range validation, and a useMutation that fires
 * PATCH /api/settings/retention with the single changed field.
 *
 * On success: invalidates ['settings','retention'] query, shows sonner toast,
 * closes the dialog. On error: shows destructive toast.
 *
 * UI-SPEC §Settings — Data Retention Card: Save button label is
 * "Save retention settings".
 *
 * Plan 05-11 Task 2.
 */
export function EditRetentionDialog({
  level,
  initialValue,
  open,
  onOpenChange,
}: {
  level: RetentionLevel
  initialValue: number
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const schema = z.object({
    value: z
      .number({ error: 'Must be a number' })
      .int(`Must be a whole number`)
      .min(level.min, `Must be at least ${level.min} ${level.unit}`)
      .max(level.max, `Must be at most ${level.max} ${level.unit}`),
  })

  type FormValues = z.infer<typeof schema>

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const form = useForm<FormValues>({
    // zodResolver infers 'unknown' for input type with zod v4; cast bypasses the mismatch.
    resolver: zodResolver(schema) as any,
    defaultValues: { value: initialValue },
  })

  const queryClient = useQueryClient()

  const save = useMutation({
    mutationFn: (vals: FormValues) =>
      apiFetch('/api/settings/retention', {
        method: 'PATCH',
        body: JSON.stringify({ [level.key]: vals.value }),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['settings', 'retention'] })
      toast.success('Retention settings saved.')
      onOpenChange(false)
    },
    onError: () => {
      toast.error('Could not save retention settings. Try again.')
    },
  })

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={onOpenChange}
      title={`Edit ${level.label}`}
      description={`Set the retention window for ${level.label.toLowerCase()}. Range: ${level.min}–${level.max} ${level.unit}.`}
    >
      <form
        id="edit-retention-form"
        onSubmit={form.handleSubmit((vals) => save.mutate(vals))}
        className="flex flex-col gap-4"
      >
        <div className="flex flex-col gap-2">
          <Label htmlFor="retention-value">
            Value ({level.unit})
          </Label>
          <div className="flex items-center gap-2">
            <Input
              id="retention-value"
              type="number"
              min={level.min}
              max={level.max}
              step={1}
              {...form.register('value', { valueAsNumber: true })}
            />
            <span className="text-sm text-muted-foreground whitespace-nowrap">{level.unit}</span>
          </div>
          {form.formState.errors.value && (
            <p className="text-sm text-destructive" role="alert">
              {form.formState.errors.value.message}
            </p>
          )}
          <p className="text-xs text-muted-foreground">
            Range: {level.min}–{level.max} {level.unit}.
          </p>
        </div>

        <div className="flex justify-end gap-2">
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button type="submit" disabled={save.isPending}>
            {save.isPending ? 'Saving…' : 'Save retention settings'}
          </Button>
        </div>
      </form>
    </ResponsiveDialog>
  )
}
