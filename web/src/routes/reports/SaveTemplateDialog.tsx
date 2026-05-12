/**
 * SaveTemplateDialog — Plan 07-11b Task 1
 *
 * Dialog for saving the current ReportConfigPanel state as a named template.
 * Name (required, max 80) + Description (optional, max 200).
 *
 * UI-SPEC Surface 6 copy verbatim.
 */

import { useEffect } from 'react'
import { useForm, type SubmitHandler } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Label } from '@/components/ui/label'
import { createTemplate } from '@/lib/reportTemplates'
import { ApiError } from '@/lib/api'

// ---------------------------------------------------------------------------
// Schema
// ---------------------------------------------------------------------------

const schema = z.object({
  name: z.string().min(1, 'Name is required.').max(80, 'Name must be 80 characters or fewer.'),
  description: z.string().max(200, 'Description must be 200 characters or fewer.'),
})

type FormValues = z.infer<typeof schema>

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

interface SaveTemplateDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentState: object
  onSaved?: () => void
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function SaveTemplateDialog({
  open,
  onOpenChange,
  currentState,
  onSaved,
}: SaveTemplateDialogProps) {
  const qc = useQueryClient()

  const {
    register,
    handleSubmit,
    reset,
    setError,
    formState: { errors, isSubmitting },
  } = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { name: '', description: '' },
  })

  // Reset form when dialog opens
  useEffect(() => {
    if (open) reset({ name: '', description: '' })
  }, [open, reset])

  const onSubmit: SubmitHandler<FormValues> = async (values) => {
    try {
      const template = await createTemplate(values.name, values.description, currentState)
      await qc.invalidateQueries({ queryKey: ['report-templates'] })
      toast.success(`Template saved: ${template.name}`)
      onSaved?.()
      onOpenChange(false)
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        setError('name', { message: 'A template with this name already exists.' })
      } else {
        setError('name', { message: 'Failed to save template. Please try again.' })
      }
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Save as Template</DialogTitle>
        </DialogHeader>

        <form onSubmit={handleSubmit(onSubmit)} className="space-y-4" noValidate>
          <div className="space-y-1.5">
            <Label htmlFor="template-name">Name</Label>
            <Input
              id="template-name"
              {...register('name')}
              placeholder="e.g. Monthly per-site Building A"
              aria-describedby={errors.name ? 'template-name-error' : undefined}
              aria-invalid={!!errors.name}
            />
            {errors.name && (
              <p id="template-name-error" className="text-sm text-destructive">
                {errors.name.message}
              </p>
            )}
          </div>

          <div className="space-y-1.5">
            <Label htmlFor="template-description">Description (optional)</Label>
            <Textarea
              id="template-description"
              {...register('description')}
              placeholder="Optional description for this template."
              rows={3}
            />
            {errors.description && (
              <p className="text-sm text-destructive">{errors.description.message}</p>
            )}
          </div>

          <DialogFooter>
            <Button
              type="button"
              variant="ghost"
              onClick={() => onOpenChange(false)}
              disabled={isSubmitting}
            >
              Discard template
            </Button>
            <Button type="submit" disabled={isSubmitting}>
              Save Template
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
