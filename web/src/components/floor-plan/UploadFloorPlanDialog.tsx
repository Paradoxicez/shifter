import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Button } from '@/components/ui/button'
import { Progress } from '@/components/ui/progress'
import { convertPdfToPng, ErrInvalidPDF, ErrPDFTooLarge } from '@/lib/pdfToPng'
import { apiFetch } from '@/lib/api'

const MAX_BYTES = 10 << 20 // 10 MB matches D-19
const MAX_DIM = 8192

export function UploadFloorPlanDialog({
  siteID,
  open,
  onOpenChange,
}: {
  siteID: string
  open: boolean
  onOpenChange: (o: boolean) => void
}) {
  const queryClient = useQueryClient()
  const [label, setLabel] = useState('')
  const [stage, setStage] = useState<'pick' | 'converting' | 'uploading'>('pick')
  const [error, setError] = useState<string | null>(null)

  const upload = useMutation({
    mutationFn: async ({ file, label }: { file: File | Blob; label: string }) => {
      const form = new FormData()
      form.append(
        'file',
        file,
        file instanceof File ? file.name : `plan-${Date.now()}.png`,
      )
      form.append('label', label)
      return apiFetch(`/api/sites/${siteID}/floor-plans`, { method: 'POST', body: form })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['floor-plans', siteID] })
      toast.success('Floor plan uploaded.')
      setLabel('')
      setStage('pick')
      onOpenChange(false)
    },
    onError: (err: unknown) => {
      const message = err instanceof Error ? err.message : 'Upload failed.'
      setError(message)
      setStage('pick')
    },
  })

  const onFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    setError(null)
    const file = e.target.files?.[0]
    if (!file) return

    if (file.size > MAX_BYTES) {
      setError('File exceeds the 10 MB limit. Resize or compress the image and try again.')
      return
    }

    if (!label.trim()) {
      setError('Add a label (e.g. "Ground floor") before uploading.')
      return
    }

    if (file.type === 'application/pdf') {
      setStage('converting')
      try {
        const png = await convertPdfToPng(file)
        setStage('uploading')
        upload.mutate({ file: png, label: label.trim() })
      } catch (err) {
        if (err instanceof ErrInvalidPDF) {
          setError('Could not convert PDF to image. Try exporting the plan as PNG from your design tool.')
        } else if (err instanceof ErrPDFTooLarge) {
          setError('PDF dimensions exceed 8192×8192 px after rendering. Reduce the page size and try again.')
        } else {
          setError('PDF conversion failed.')
        }
        setStage('pick')
      }
      return
    }

    if (file.type !== 'image/png' && file.type !== 'image/jpeg') {
      setError('Only PNG, JPG, and PDF files are accepted. Convert your file and try again.')
      return
    }

    // PNG/JPG dimension check via ImageBitmap (also enforced server-side as defense-in-depth).
    try {
      const bmp = await createImageBitmap(file)
      if (bmp.width > MAX_DIM || bmp.height > MAX_DIM) {
        setError(`Image dimensions exceed ${MAX_DIM}×${MAX_DIM} px. Scale it down and try again.`)
        bmp.close()
        return
      }
      bmp.close()
    } catch {
      // createImageBitmap may not be available in all test environments; skip dimension check
    }

    setStage('uploading')
    upload.mutate({ file, label: label.trim() })
  }

  return (
    <ResponsiveDialog open={open} onOpenChange={onOpenChange} title="Upload floor plan">
      <div className="space-y-4">
        <div>
          <Label htmlFor="floor-plan-label">Label</Label>
          <Input
            id="floor-plan-label"
            placeholder='e.g. Ground floor, B1, Site overview'
            value={label}
            onChange={(e) => setLabel(e.target.value)}
          />
        </div>
        <div>
          <Label htmlFor="floor-plan-file">Image</Label>
          <Input
            id="floor-plan-file"
            type="file"
            accept="image/png,image/jpeg,application/pdf"
            onChange={onFileChange}
            disabled={stage !== 'pick'}
          />
          <p className="text-xs text-muted-foreground mt-1">
            PNG, JPG, or PDF up to 10 MB. PDFs are converted to PNG in your browser before upload.
          </p>
        </div>
        {stage === 'converting' && (
          <Progress value={50} aria-label="Converting PDF to image…" />
        )}
        {stage === 'uploading' && <Progress value={75} aria-label="Uploading…" />}
        {error && <div className="text-sm text-destructive">{error}</div>}
        <div className="flex justify-end gap-2">
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={stage !== 'pick'}>
            Cancel
          </Button>
        </div>
      </div>
    </ResponsiveDialog>
  )
}
