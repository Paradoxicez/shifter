/**
 * ImportFromCatalogDialog — Surface 2 (UI-SPEC §"Surface 2").
 *
 * Plan 07-06 / V2-VEND-01.
 *
 * 3-step flow:
 *   Step 1 (start):  RadioGroup — "Start blank" | "Import from catalog"
 *   Step 2 (picker): Searchable Command list of catalog entries
 *   Step 3 (review): Editable form pre-filled with vendor values
 *
 * All copy strings are verbatim from UI-SPEC Surface 2.
 */

import { useState, useEffect } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useForm, type SubmitHandler } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Command,
  CommandInput,
  CommandList,
  CommandItem,
  CommandEmpty,
} from '@/components/ui/command'
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Textarea } from '@/components/ui/textarea'
import {
  fetchCatalog,
  fetchCatalogEntry,
  importFromCatalog,
} from '@/lib/catalog'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type Step = 'start' | 'picker' | 'review'
type StartChoice = 'blank' | 'catalog'

const reviewSchema = z.object({
  name: z.string().min(1, 'Name is required').max(80, 'Name is too long'),
  codec_js: z.string().min(1, 'Codec is required'),
  expected_uplink_interval_seconds: z.number().int().positive(),
})
type ReviewValues = z.infer<typeof reviewSchema>

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

interface ImportFromCatalogDialogProps {
  open: boolean
  onOpenChange: (v: boolean) => void
  /** Called with the new profile ID after a successful import. */
  onImported?: (profileId: string) => void
}

export function ImportFromCatalogDialog({
  open,
  onOpenChange,
  onImported,
}: ImportFromCatalogDialogProps) {
  const [step, setStep] = useState<Step>('start')
  const [startChoice, setStartChoice] = useState<StartChoice | null>(null)
  const [selectedSlug, setSelectedSlug] = useState<string | null>(null)
  const qc = useQueryClient()

  // Catalog list — fetched when we enter the picker step
  const { data: catalog } = useQuery({
    queryKey: ['catalog'],
    queryFn: fetchCatalog,
    enabled: step === 'picker',
  })

  // Individual entry — fetched when we enter the review step
  const { data: entry, isLoading: entryLoading } = useQuery({
    queryKey: ['catalog-entry', selectedSlug],
    queryFn: () => fetchCatalogEntry(selectedSlug!),
    enabled: !!selectedSlug && step === 'review',
  })

  const form = useForm<ReviewValues>({
    resolver: zodResolver(reviewSchema),
    defaultValues: { name: '', codec_js: '', expected_uplink_interval_seconds: 3600 },
  })

  const mutation = useMutation({
    mutationFn: (values: ReviewValues) => importFromCatalog(selectedSlug!, values.name),
    onSuccess: (resp) => {
      toast.success('Profile added')
      qc.invalidateQueries({ queryKey: ['catalog'] })
      qc.invalidateQueries({ queryKey: ['device-profiles'] })
      onImported?.(resp.profile_id)
      handleClose(false)
    },
    onError: (err: Error) => {
      if (err.message === 'already imported') {
        form.setError('name', { message: 'A profile with this name already exists.' })
      } else {
        toast.error('Failed to add profile. Try again.')
      }
    },
  })

  function handleClose(next: boolean) {
    if (!next) {
      // reset state on close
      setStep('start')
      setStartChoice(null)
      setSelectedSlug(null)
      form.reset()
    }
    onOpenChange(next)
  }

  function handleContinueFromStart() {
    if (startChoice === 'catalog') {
      setStep('picker')
    }
    // "blank" path: close dialog and let caller navigate to /profiles/new
    // (the existing "Create profile" button already does this)
    if (startChoice === 'blank') {
      handleClose(false)
    }
  }

  function handleSelectSlug(slug: string) {
    setSelectedSlug(slug)
    setStep('review')
  }

  // Pre-populate the review form once the entry loads
  useEffect(() => {
    if (entry && step === 'review') {
      form.reset({
        name: `${entry.vendor} ${entry.family}`,
        codec_js: entry.codec_js ?? '',
        expected_uplink_interval_seconds: entry.expected_uplink_interval_seconds,
      })
    }
  }, [entry, step]) // eslint-disable-line react-hooks/exhaustive-deps

  const onSubmit: SubmitHandler<ReviewValues> = (values) => {
    mutation.mutate(values)
  }

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="sm:max-w-[560px]">
        {/* ---- Step 1: Start choice ---------------------------------------- */}
        {step === 'start' && (
          <>
            <DialogHeader>
              <DialogTitle>Add Device Profile</DialogTitle>
            </DialogHeader>

            <RadioGroup
              value={startChoice ?? ''}
              onValueChange={(v) => setStartChoice(v as StartChoice)}
              className="flex flex-col gap-3 py-2"
            >
              <label className="flex cursor-pointer items-start gap-3 rounded-md border p-4 hover:bg-accent">
                <RadioGroupItem value="blank" className="mt-0.5" />
                <div>
                  <div className="font-semibold">Start blank</div>
                  <div className="text-sm text-muted-foreground">
                    Create a new profile from scratch
                  </div>
                </div>
              </label>
              <label className="flex cursor-pointer items-start gap-3 rounded-md border p-4 hover:bg-accent">
                <RadioGroupItem value="catalog" className="mt-0.5" />
                <div>
                  <div className="font-semibold">Import from catalog</div>
                  <div className="text-sm text-muted-foreground">
                    Start with a pre-configured vendor profile
                  </div>
                </div>
              </label>
            </RadioGroup>

            <DialogFooter>
              <Button variant="ghost" onClick={() => handleClose(false)}>
                Cancel
              </Button>
              <Button disabled={!startChoice} onClick={handleContinueFromStart}>
                Continue
              </Button>
            </DialogFooter>
          </>
        )}

        {/* ---- Step 2: Catalog picker -------------------------------------- */}
        {step === 'picker' && (
          <>
            <DialogHeader>
              <DialogTitle>Add Device Profile</DialogTitle>
            </DialogHeader>

            <Command className="max-h-[260px] overflow-hidden rounded-md border">
              <CommandInput placeholder="Search vendor profiles…" />
              <CommandList>
                <CommandEmpty>No profiles found.</CommandEmpty>
                {(catalog?.entries ?? []).map((e) => (
                  <CommandItem
                    key={e.slug}
                    value={`${e.vendor} ${e.family} ${e.slug}`}
                    onSelect={() => handleSelectSlug(e.slug)}
                    className="cursor-pointer"
                  >
                    <div className="flex w-full items-center justify-between">
                      <span className="font-medium">
                        {e.vendor} {e.family}
                      </span>
                      <span className="text-sm text-muted-foreground">v{e.version}</span>
                    </div>
                  </CommandItem>
                ))}
              </CommandList>
            </Command>

            <DialogFooter>
              <Button variant="ghost" onClick={() => setStep('start')}>
                Back
              </Button>
              <Button disabled={!selectedSlug} onClick={() => setStep('review')}>
                Continue
              </Button>
            </DialogFooter>
          </>
        )}

        {/* ---- Step 3: Review form ----------------------------------------- */}
        {step === 'review' && (
          <>
            <DialogHeader>
              <DialogTitle>
                {entry
                  ? `Review Profile: ${entry.vendor} ${entry.family}`
                  : 'Review Profile: …'}
              </DialogTitle>
            </DialogHeader>

            {entryLoading ? (
              <p className="text-sm text-muted-foreground py-4">Loading profile…</p>
            ) : (
              <form
                id="import-catalog-form"
                onSubmit={form.handleSubmit(onSubmit)}
                className="flex flex-col gap-4 py-2"
              >
                {/* Capability chips (read-only) */}
                {entry && entry.capabilities.length > 0 && (
                  <div className="flex flex-wrap gap-1">
                    {entry.capabilities.map((c) => (
                      <Badge key={c} variant="outline" className="text-xs uppercase">
                        {c}
                      </Badge>
                    ))}
                  </div>
                )}

                {/* Profile name */}
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor="import-name">Profile name</Label>
                  <Input id="import-name" autoComplete="off" {...form.register('name')} />
                  {form.formState.errors.name && (
                    <span className="text-sm text-destructive">
                      {form.formState.errors.name.message}
                    </span>
                  )}
                </div>

                {/* Codec */}
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor="import-codec">Codec</Label>
                  <Textarea
                    id="import-codec"
                    {...form.register('codec_js')}
                    rows={8}
                    className="font-mono text-xs"
                  />
                  {form.formState.errors.codec_js && (
                    <span className="text-sm text-destructive">
                      {form.formState.errors.codec_js.message}
                    </span>
                  )}
                </div>

                {/* Expected uplink interval */}
                <div className="flex flex-col gap-1.5">
                  <Label htmlFor="import-interval">
                    Expected uplink interval (seconds)
                  </Label>
                  <Input
                    id="import-interval"
                    type="number"
                    min={1}
                    {...form.register('expected_uplink_interval_seconds')}
                  />
                  {form.formState.errors.expected_uplink_interval_seconds && (
                    <span className="text-sm text-destructive">
                      {form.formState.errors.expected_uplink_interval_seconds.message}
                    </span>
                  )}
                </div>
              </form>
            )}

            <DialogFooter>
              <Button
                type="button"
                variant="ghost"
                onClick={() => {
                  form.reset()
                  setStep('picker')
                }}
              >
                Back
              </Button>
              <Button
                type="submit"
                form="import-catalog-form"
                disabled={mutation.isPending || entryLoading}
              >
                {mutation.isPending ? 'Adding…' : 'Add Profile'}
              </Button>
            </DialogFooter>
          </>
        )}
      </DialogContent>
    </Dialog>
  )
}
