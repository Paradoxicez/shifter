import { useState } from 'react'
import {
  AlertDialog,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { convertPdfToPng, ErrInvalidPDF, ErrPDFTooLarge } from '@/lib/pdfToPng'

const MAX_BYTES = 10 << 20 // 10 MB
const MAX_DIM = 8192

export function ReplaceImageDialog({
  pinCount,
  open,
  onOpenChange,
  onReplace,
}: {
  pinCount: number
  open: boolean
  onOpenChange: (o: boolean) => void
  onReplace: (file: File | Blob) => void
}) {
  const [selectedFile, setSelectedFile] = useState<File | Blob | null>(null)
  const [converting, setConverting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    setError(null)
    setSelectedFile(null)
    const file = e.target.files?.[0]
    if (!file) return

    if (file.size > MAX_BYTES) {
      setError('File exceeds the 10 MB limit. Resize or compress the image and try again.')
      return
    }

    if (file.type === 'application/pdf') {
      setConverting(true)
      try {
        const png = await convertPdfToPng(file)
        setSelectedFile(png)
      } catch (err) {
        if (err instanceof ErrInvalidPDF) {
          setError('Could not convert PDF to image. Try exporting the plan as PNG from your design tool.')
        } else if (err instanceof ErrPDFTooLarge) {
          setError('PDF dimensions exceed 8192×8192 px after rendering. Reduce the page size and try again.')
        } else {
          setError('PDF conversion failed.')
        }
      } finally {
        setConverting(false)
      }
      return
    }

    if (file.type !== 'image/png' && file.type !== 'image/jpeg') {
      setError('Only PNG, JPG, and PDF files are accepted. Convert your file and try again.')
      return
    }

    // Dimension check via ImageBitmap
    try {
      const bmp = await createImageBitmap(file)
      if (bmp.width > MAX_DIM || bmp.height > MAX_DIM) {
        setError(`Image dimensions exceed ${MAX_DIM}×${MAX_DIM} px. Scale it down and try again.`)
        bmp.close()
        return
      }
      bmp.close()
    } catch {
      // createImageBitmap may not be available in all environments; skip check
    }

    setSelectedFile(file)
  }

  const handleConfirm = () => {
    if (!selectedFile) return
    onReplace(selectedFile)
    setSelectedFile(null)
    setError(null)
  }

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Replace floor plan image?</AlertDialogTitle>
          <AlertDialogDescription>
            Existing {pinCount} pin{pinCount === 1 ? '' : 's'} will be kept at the same fractional
            positions on the new image. Review and reposition them after upload.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <div className="space-y-3 py-2">
          <div>
            <Label htmlFor="replace-fp-file">New image</Label>
            <Input
              id="replace-fp-file"
              type="file"
              accept="image/png,image/jpeg,application/pdf"
              onChange={handleFileChange}
              disabled={converting}
            />
            <p className="text-xs text-muted-foreground mt-1">
              PNG, JPG, or PDF up to 10 MB.{' '}
              {converting && <span className="text-primary">Converting PDF…</span>}
            </p>
          </div>
          {error && <div className="text-sm text-destructive">{error}</div>}
        </div>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <Button
            onClick={handleConfirm}
            disabled={!selectedFile || converting}
            className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
          >
            Replace image
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
