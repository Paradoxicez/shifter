---
phase: 05-aggregates-reports-map-floor-plans
plan: 10
type: execute
wave: 5
depends_on: [07, 08]
files_modified:
  - web/src/lib/pdfToPng.ts
  - web/src/lib/pdfToPng.test.ts
  - web/src/routes/sites/$id.tsx
  - web/src/components/floor-plan/FloorPlanTab.tsx
  - web/src/components/floor-plan/FloorPlanSelector.tsx
  - web/src/components/floor-plan/FloorPlanCanvas.tsx
  - web/src/components/floor-plan/FloorPlanCanvas.test.tsx
  - web/src/components/floor-plan/DevicePin.tsx
  - web/src/components/floor-plan/DevicePin.test.tsx
  - web/src/components/floor-plan/DevicePinLabel.tsx
  - web/src/components/floor-plan/DevicePinPopover.tsx
  - web/src/components/floor-plan/DeviceSidebar.tsx
  - web/src/components/floor-plan/UploadFloorPlanDialog.tsx
  - web/src/components/floor-plan/ReplaceImageDialog.tsx
  - web/src/components/floor-plan/RemovePinAlertDialog.tsx
  - web/src/lib/hooks/useFloorPlanHealth.ts
  - web/playwright/specs/floor-plan-pinning.spec.ts
  - web/playwright/specs/floor-plan-health.spec.ts
  - web/playwright/specs/site-drill-through.spec.ts
autonomous: true
requirements: [SITE-02, SITE-03, SITE-04, SITE-05, SITE-06]
threat_refs: [T-05-10-01]

must_haves:
  truths:
    - "Site detail page gains a 'Floor plan' tab (D-23: default tab when floor_plan rows exist; otherwise Overview default)"
    - "Floor plan tab shows: FloorPlanSelector (multi-floor pill strip ordered by sort_order) + FloorPlanCanvas + DeviceSidebar with Unplaced/Placed device sections"
    - "Pinning flow: click device in sidebar → ghost cursor → click on canvas → pin lands at (x_frac, y_frac) per D-20"
    - "Drag-to-nudge: pointer events on a pin update fractional coords on pointerup with 300ms debounced PATCH per UI-SPEC"
    - "Right-click pin → RemovePinAlertDialog (destructive variant) → DELETE placement"
    - "DevicePin state color (D-22): green healthy / yellow warning / red offline; ring-2 ring-white; 12px diameter"
    - "Marker state updates live via SSE: extends Phase 4 useSSE hook to subscribe to mp:<uuid> topics for placed devices; state recomputed client-side from measurement payload per RESEARCH §Hub Extension Option B"
    - "Upload dialog accepts PNG / JPG / PDF; PDF converted client-side via pdf.js to PNG before upload (D-17)"
    - "Replace image dialog warns 'Existing {N} pins will be kept' per D-24"
    - "Image dimensions ≤ 8192 enforced client-side BEFORE upload as belt-and-suspenders for server (D-19)"
    - "Pin rendering math (UI-SPEC §Floor Plan — Canvas Coordinate Contract): `left: ${xFrac * 100}%` + `top: ${yFrac * 100}%` + `-translate-x-1/2 -translate-y-1/2`"
    - "touch-action: none on canvas to prevent scroll conflict during pin placement"
    - "DevicePinPopover shows: cumulative value + last reading + battery + RSSI + 'Open device' link to /devices/:id"
    - "Site drill-through path works: /map popup 'View site' → /sites/:id Floor plan tab (default) → DevicePinPopover → 'Open device' (SITE-06 in 3 clicks max)"
  artifacts:
    - path: "web/src/lib/pdfToPng.ts"
      provides: "convertPdfToPng(file: File) → Promise<Blob> — renders page 1 to 150 DPI canvas, exports PNG, releases memory"
      contains: "150 / 72"
    - path: "web/src/components/floor-plan/FloorPlanCanvas.tsx"
      provides: "Image + abs-positioned pins, pointer-events drag, touch-action:none, place-mode cursor"
      contains: "touch-none"
    - path: "web/src/components/floor-plan/DevicePin.tsx"
      provides: "12px state-tinted dot with ring + drag handler + popover + right-click → remove"
      contains: "rounded-full"
    - path: "web/src/components/floor-plan/UploadFloorPlanDialog.tsx"
      provides: "ResponsiveDialog with file input + PDF detection branch (pdf.js) + dim validation + label input"
      contains: "convertPdfToPng"
    - path: "web/src/lib/hooks/useFloorPlanHealth.ts"
      provides: "Per-floor-plan SSE subscription: listens to mp:<uuid> events for each placed device, computes D-22 state client-side"
      contains: "useFloorPlanHealth"
  key_links:
    - from: "web/src/components/floor-plan/FloorPlanCanvas.tsx"
      to: "x_frac/y_frac fractional positioning"
      via: "CSS left/top % math"
      pattern: "xFrac \\* 100"
    - from: "web/src/components/floor-plan/UploadFloorPlanDialog.tsx"
      to: "POST /api/sites/:id/floor-plans (multipart)"
      via: "FormData with file (PNG bytes, possibly converted from PDF) + label"
      pattern: "FormData"
    - from: "web/src/components/floor-plan/DevicePin.tsx"
      to: "PATCH /api/floor-plans/:id/placements/:device_id"
      via: "debounced (300ms) on pointerup drag"
      pattern: "debouncedPatch"
    - from: "web/src/lib/hooks/useFloorPlanHealth.ts"
      to: "Phase 4 useSSE hook"
      via: "subscribe to mp:<uuid> topics for placed devices"
      pattern: "useSSE"
---

<objective>
Ship the floor-plan UX: site detail Floor plan tab, multi-floor selector pill strip (sort_order ordered), FloorPlanCanvas with custom pointer-event-driven pinning (no third-party library per D-20), DevicePin with state-tinted dot + drag + right-click remove + popover, DeviceSidebar with Unplaced/Placed sections, Upload + Replace + Remove dialogs, client-side pdf.js conversion (D-17), and live SSE marker state updates via Phase 4 Hub piggy-backing on `mp:<uuid>` topics.

Purpose: This is SITE-04..06's load-bearing UX — fractional coordinates need to feel right on every viewport (desktop, mobile, Retina) without third-party canvas libraries. The SSE-driven state refresh closes the loop on D-22 "marker colors update live."

Output: 15 React/TS files in `web/src/components/floor-plan/` + `web/src/lib/`, 3 Playwright spec bodies.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/05-aggregates-reports-map-floor-plans/05-CONTEXT.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-RESEARCH.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-UI-SPEC.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-07-floor-plan-placement-decommission-PLAN.md
@web/src/lib/pdfWorker.ts
@web/src/routes/sites
@web/src/components/responsive-dialog.tsx
@web/src/components/ui/alert-dialog.tsx
@web/src/lib/hooks
@web/src/components/ui/popover.tsx
@web/src/components/ui/scroll-area.tsx

<interfaces>
<!-- Server endpoints from plans 05-05 + 05-07 -->
```
POST   /api/sites/{siteID}/floor-plans          multipart (file, label, sort_order?) → floor_plan JSON
GET    /api/sites/{siteID}/floor-plans          → floor_plan[]
GET    /api/floor-plans/{id}                    → floor_plan
PATCH  /api/floor-plans/{id}                    multipart (file, optional label) → floor_plan (image replaced)
PATCH  /api/floor-plans/{id}/label              JSON {label, sort_order} → floor_plan
DELETE /api/floor-plans/{id}                    → 204
GET    /api/floor-plans/{id}/image              → image bytes (auth-gated)
POST   /api/floor-plans/{id}/placements         JSON {device_id, x_frac, y_frac} → placement
PATCH  /api/floor-plans/{id}/placements/{deviceID} JSON {x_frac, y_frac} → placement
DELETE /api/floor-plans/{id}/placements/{deviceID} → 204
GET    /api/floor-plans/{id}/placements         → placement[] denormalized with device + utility + last_seen_at + battery_pct + rssi + expected_interval_s
```

<!-- D-22 state semantics (computed client-side from useFloorPlanHealth) -->
```ts
type DeviceHealthState = 'healthy' | 'warning' | 'offline'

function computeState(p: PlacementWithDeviceData): DeviceHealthState {
  const stale = Date.now() - new Date(p.last_seen_at).getTime() > 2 * p.expected_interval_s * 1000
  if (stale) return 'offline'
  if ((p.battery_pct ?? 100) <= 20) return 'warning'
  if ((p.rssi ?? 0) < -110) return 'warning'
  // quality<>'ok' in last 10 uplinks would also warning — but that's a server-side
  // detail we don't carry client-side; SSE measurement payload's `quality` is
  // recent enough to use as a proxy for the last-N check.
  return 'healthy'
}
```

<!-- Phase 4 useSSE hook (existing) -->
```ts
type UseSSEOptions = { topics: string[]; onMessage: (event: MessageEvent) => void }
function useSSE(opts: UseSSEOptions): { connected: boolean; reconnecting: boolean }
```
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: pdf.js helper + UploadFloorPlanDialog + ReplaceImageDialog</name>
  <read_first>
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-CONTEXT.md §D-17 §D-19 §D-24
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-RESEARCH.md §pdf.js (pdfjs-dist) §Common Pitfalls #3
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-UI-SPEC.md §Floor Plans Surface §Destructive Confirmations
    - web/src/lib/pdfWorker.ts (Plan 05-01 worker shim — import this in pdfToPng.ts)
    - web/src/components/responsive-dialog.tsx (dialog primitive)
  </read_first>
  <behavior>
    - Test 1: convertPdfToPng(File) renders page 1 to OffscreenCanvas at scale 150/72 and returns a Blob with type 'image/png'
    - Test 2: After conversion, page.cleanup() and pdf.destroy() are called (verified via mock spy)
    - Test 3: When passed a non-PDF file → throws ErrInvalidPDF
    - Test 4: UploadFloorPlanDialog accepts PNG file → uploads directly; accepts PDF → shows "Converting…" progress, then uploads PNG
    - Test 5: UploadFloorPlanDialog rejects > 10MB file on the client side before upload (matches D-19)
    - Test 6: UploadFloorPlanDialog rejects unsupported extension via `accept` attribute + JS guard
    - Test 7: ReplaceImageDialog renders "Existing {N} pins will be kept" with N from CountPinsOnFloorPlan API
    - Test 8: ReplaceImageDialog confirm button uses destructive variant
  </behavior>
  <action>
**Step A — `web/src/lib/pdfToPng.ts`:**

```ts
// Client-side PDF → PNG conversion (D-17). The server NEVER sees PDF bytes.
//
// Pitfall #3 (RESEARCH): pdfjs-dist v5 is ESM-only. The worker is set ONCE via
// the side-effect import of './pdfWorker' (plan 05-01). Importing this file
// pulls in the worker shim.

import { pdfjsLib } from './pdfWorker'

export class ErrInvalidPDF extends Error { constructor() { super('not a valid PDF') } }
export class ErrPDFTooLarge extends Error { constructor(public dim: number) { super(`PDF page renders to ${dim}px, exceeds 8192 cap`) } }

const TARGET_DPI = 150
const PDF_NATIVE_DPI = 72
const SCALE = TARGET_DPI / PDF_NATIVE_DPI  // ≈ 2.083

export async function convertPdfToPng(file: File): Promise<Blob> {
  if (file.type !== 'application/pdf') throw new ErrInvalidPDF()

  const buf = await file.arrayBuffer()
  const pdf = await pdfjsLib.getDocument({ data: buf }).promise

  try {
    const page = await pdf.getPage(1)
    const viewport = page.getViewport({ scale: SCALE })

    // Enforce 8192² cap client-side (D-19): server will also reject but we
    // shouldn't waste user bandwidth on a doomed upload.
    if (viewport.width > 8192 || viewport.height > 8192) {
      page.cleanup()
      throw new ErrPDFTooLarge(Math.max(viewport.width, viewport.height))
    }

    const canvas = new OffscreenCanvas(Math.floor(viewport.width), Math.floor(viewport.height))
    const ctx = canvas.getContext('2d')
    if (!ctx) throw new Error('canvas 2d context unavailable')

    await page.render({ canvasContext: ctx, viewport }).promise

    page.cleanup()
    return await canvas.convertToBlob({ type: 'image/png' })
  } finally {
    await pdf.destroy()
  }
}
```

**Step B — Replace `it.skip` in `web/src/lib/pdfToPng.test.ts`:**

```ts
describe('pdfToPng', () => {
  it('renders PDF page 1 to 150-DPI canvas and exports PNG blob (D-17)', async () => {
    // Use a small fixture PDF (encoded as base64 in test).
    const pdfBuffer = base64ToArrayBuffer(FIXTURE_PDF_B64)
    const file = new File([pdfBuffer], 'plan.pdf', { type: 'application/pdf' })
    const blob = await convertPdfToPng(file)
    expect(blob.type).toBe('image/png')
    expect(blob.size).toBeGreaterThan(0)
  })

  it('uses pdfWorker shim with import.meta.url', () => {
    // pdfWorker import is a side effect; spy on pdfjsLib.GlobalWorkerOptions.workerSrc
    expect(pdfjsLib.GlobalWorkerOptions.workerSrc).toContain('pdf.worker.min.mjs')
  })

  it('throws ErrInvalidPDF when given a non-PDF file', async () => {
    const file = new File([new Uint8Array([1, 2, 3])], 'fake.pdf', { type: 'image/png' })
    await expect(convertPdfToPng(file)).rejects.toBeInstanceOf(ErrInvalidPDF)
  })

  it('releases page + document memory after blob export', async () => {
    const cleanupSpy = vi.fn()
    const destroySpy = vi.fn()
    // Inject mock document via vi.mock pdfjs-dist
    // Assert cleanupSpy and destroySpy each called exactly once after convertPdfToPng resolves
  })
})
```

**Step C — `web/src/components/floor-plan/UploadFloorPlanDialog.tsx`:**

```tsx
import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Button } from '@/components/ui/button'
import { Progress } from '@/components/ui/progress'
import { toast } from 'sonner'
import { convertPdfToPng, ErrInvalidPDF, ErrPDFTooLarge } from '@/lib/pdfToPng'
import { apiFetch } from '@/lib/apiFetch'

const MAX_BYTES = 10 << 20  // 10MB matches D-19
const MAX_DIM = 8192

export function UploadFloorPlanDialog({ siteID, open, onOpenChange }: {
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
      form.append('file', file, file instanceof File ? file.name : `plan-${Date.now()}.png`)
      form.append('label', label)
      return apiFetch(`/api/sites/${siteID}/floor-plans`, { method: 'POST', body: form })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['floor-plans', siteID] })
      toast.success('Floor plan uploaded.')
      onOpenChange(false)
    },
    onError: (err: any) => setError(err.message),
  })

  const onFileChange = async (e: React.ChangeEvent<HTMLInputElement>) => {
    setError(null)
    const file = e.target.files?.[0]
    if (!file) return

    if (file.size > MAX_BYTES) {
      setError('File exceeds the 10 MB limit. Resize or compress the image and try again.')
      return
    }
    if (!label) {
      setError('Add a label (e.g. "Ground floor") before uploading.')
      return
    }

    if (file.type === 'application/pdf') {
      setStage('converting')
      try {
        const png = await convertPdfToPng(file)
        setStage('uploading')
        upload.mutate({ file: png, label })
      } catch (err) {
        if (err instanceof ErrInvalidPDF) setError('Could not convert PDF to image. Try exporting the plan as PNG from your design tool.')
        else if (err instanceof ErrPDFTooLarge) setError('PDF dimensions exceed 8192×8192 px after rendering. Reduce the page size and try again.')
        else setError('PDF conversion failed.')
        setStage('pick')
      }
      return
    }

    if (file.type !== 'image/png' && file.type !== 'image/jpeg') {
      setError('Only PNG, JPG, and PDF files are accepted. Convert your file and try again.')
      return
    }

    // PNG/JPG dimension check via ImageBitmap (also fired server-side as defense-in-depth).
    const bmp = await createImageBitmap(file)
    if (bmp.width > MAX_DIM || bmp.height > MAX_DIM) {
      setError(`Image dimensions exceed ${MAX_DIM}×${MAX_DIM} px. Scale it down and try again.`)
      bmp.close()
      return
    }
    bmp.close()

    setStage('uploading')
    upload.mutate({ file, label })
  }

  return (
    <ResponsiveDialog open={open} onOpenChange={onOpenChange} title="Upload floor plan">
      <div className="space-y-4">
        <div>
          <Label htmlFor="floor-plan-label">Label</Label>
          <Input id="floor-plan-label" placeholder="e.g. Ground floor, B1, Site overview" value={label} onChange={(e) => setLabel(e.target.value)} />
        </div>
        <div>
          <Label htmlFor="floor-plan-file">Image</Label>
          <Input id="floor-plan-file" type="file" accept="image/png,image/jpeg,application/pdf" onChange={onFileChange} />
          <p className="text-xs text-muted-foreground mt-1">PNG, JPG, or PDF up to 10 MB. PDFs are converted to PNG in your browser before upload.</p>
        </div>
        {stage === 'converting' && <Progress value={50} aria-label="Converting PDF to image…" />}
        {stage === 'uploading' && <Progress value={75} aria-label="Uploading…" />}
        {error && <div className="text-sm text-destructive">{error}</div>}
      </div>
    </ResponsiveDialog>
  )
}
```

**Step D — `web/src/components/floor-plan/ReplaceImageDialog.tsx`:**

```tsx
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription,
         AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'
import { Input } from '@/components/ui/input'

export function ReplaceImageDialog({ pinCount, open, onOpenChange, onReplace }: {
  pinCount: number
  open: boolean
  onOpenChange: (o: boolean) => void
  onReplace: (file: File | Blob) => void
}) {
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Replace floor plan image?</AlertDialogTitle>
          <AlertDialogDescription>
            Existing {pinCount} pin{pinCount === 1 ? '' : 's'} will be kept at the same fractional positions
            on the new image. Review and reposition them after upload.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <Input type="file" accept="image/png,image/jpeg,application/pdf" onChange={(e) => { /* same pipeline as UploadFloorPlanDialog */ }} />
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction className="bg-destructive text-destructive-foreground hover:bg-destructive/90">
            Replace image
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
```
  </action>
  <verify>
    <automated>pnpm --dir web test:run --reporter=basic web/src/lib/pdfToPng.test.ts web/src/components/floor-plan/ &amp;&amp; pnpm --dir web build</automated>
  </verify>
  <acceptance_criteria>
    - `web/src/lib/pdfToPng.ts` contains literals `150 / 72`, `pdf.destroy()`, `page.cleanup()`, `OffscreenCanvas`
    - `web/src/lib/pdfToPng.ts` imports from `./pdfWorker` (plan 05-01 shim)
    - `web/src/lib/pdfToPng.ts` exports `ErrInvalidPDF` and `ErrPDFTooLarge`
    - `pdfToPng.test.ts` has bodies for: PNG export, worker shim active, ErrInvalidPDF on non-PDF, cleanup/destroy called
    - `UploadFloorPlanDialog.tsx` contains literal `convertPdfToPng` AND `accept="image/png,image/jpeg,application/pdf"` AND error copy `File exceeds the 10 MB limit`
    - `ReplaceImageDialog.tsx` contains literal `Existing {pinCount} pin` text matching UI-SPEC and uses AlertDialog with destructive variant
    - `pnpm --dir web build` exits 0
    - `pnpm --dir web test:run` exits 0
  </acceptance_criteria>
  <done>pdf.js client conversion ships with memory release; upload + replace dialogs enforce client-side dim + size + format guards.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: FloorPlanCanvas + DevicePin + DeviceSidebar + live SSE health</name>
  <read_first>
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-CONTEXT.md §D-20 §D-21 §D-22 §D-23 §D-25
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-UI-SPEC.md §Floor Plan — Canvas Coordinate Contract §Interaction States — Floor Plan Pinning Flow §Component Inventory — Floor Plans Surface
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-RESEARCH.md §Custom Canvas Pin Overlay §Hub Extension for Floor-Plan Device Health
    - web/src/lib/hooks/useSSE.ts (Phase 4 SSE client — confirm signature and topic format)
    - web/src/routes/sites/$id.tsx (existing site detail; we extend with Floor plan tab)
    - web/src/components/ui/alert-dialog.tsx, popover.tsx, scroll-area.tsx
  </read_first>
  <behavior>
    - Test 1: FloorPlanCanvas click handler converts (clientX − rect.left) / rect.width to xFrac in [0,1]
    - Test 2: Drag on a DevicePin updates fractional coords on pointerup with debounced PATCH (300ms)
    - Test 3: Drag clamps coords to [0,1]
    - Test 4: Right-click on a pin opens RemovePinAlertDialog (preventDefault native context menu)
    - Test 5: DevicePin state=healthy → bg-success class
    - Test 6: DevicePin state=warning → bg-warning class
    - Test 7: DevicePin state=offline → bg-destructive class
    - Test 8: DevicePin position uses `left: ${xFrac * 100}%` + `top: ${yFrac * 100}%` + `-translate-x-1/2 -translate-y-1/2`
    - Test 9: FloorPlanTab is the default tab when floor_plan rows exist; Overview default otherwise (D-23)
    - Test 10: useFloorPlanHealth subscribes to mp:<uuid> SSE topics for each placed device; on incoming measurement event, recomputes the device's state via D-22 rules
    - Test 11: DeviceSidebar shows Unplaced count + Placed count badges; click an unplaced device → place-mode active (cursor visual)
  </behavior>
  <action>
**Step A — `web/src/components/floor-plan/FloorPlanCanvas.tsx`:**

```tsx
import { useRef, useState, useCallback } from 'react'
import { DevicePin } from './DevicePin'

export type PlacementView = {
  device_id: string
  device_name: string
  x_frac: number
  y_frac: number
  state: 'healthy' | 'warning' | 'offline'
  utility_class: string
  last_seen_at: string | null
  battery_pct: number | null
  rssi: number | null
}

export function FloorPlanCanvas({
  imageSrc, imageW, imageH,
  placements, placingDeviceID,
  onPlace, onNudge, onRemoveRequest, onOpenDevice,
}: {
  imageSrc: string
  imageW: number
  imageH: number
  placements: PlacementView[]
  placingDeviceID: string | null  // when set, click on canvas fires onPlace
  onPlace: (deviceID: string, xFrac: number, yFrac: number) => void
  onNudge: (deviceID: string, xFrac: number, yFrac: number) => void
  onRemoveRequest: (deviceID: string) => void
  onOpenDevice: (deviceID: string) => void
}) {
  const containerRef = useRef<HTMLDivElement>(null)

  const handleClick = useCallback((e: React.PointerEvent<HTMLDivElement>) => {
    if (!placingDeviceID || !containerRef.current) return
    // Ignore clicks that originate on a pin (handled by pin's own handler).
    if ((e.target as HTMLElement).dataset.role === 'device-pin') return
    const rect = containerRef.current.getBoundingClientRect()
    const xFrac = Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width))
    const yFrac = Math.max(0, Math.min(1, (e.clientY - rect.top) / rect.height))
    onPlace(placingDeviceID, xFrac, yFrac)
  }, [placingDeviceID, onPlace])

  return (
    <div
      ref={containerRef}
      className={`relative w-full touch-none select-none ${placingDeviceID ? 'cursor-crosshair' : 'cursor-default'}`}
      onPointerUp={handleClick}
      role={placingDeviceID ? 'application' : 'img'}
      aria-label={placingDeviceID ? 'Floor plan — click to place device' : 'Floor plan'}
      style={{ aspectRatio: `${imageW} / ${imageH}` }}
    >
      <img src={imageSrc} alt="Floor plan" className="w-full block" draggable={false} />
      {placements.map(p => (
        <DevicePin
          key={p.device_id}
          placement={p}
          containerRef={containerRef}
          onNudge={(xFrac, yFrac) => onNudge(p.device_id, xFrac, yFrac)}
          onRemoveRequest={() => onRemoveRequest(p.device_id)}
          onOpenDevice={() => onOpenDevice(p.device_id)}
        />
      ))}
    </div>
  )
}
```

**Step B — `web/src/components/floor-plan/DevicePin.tsx`:**

```tsx
import { useCallback, useRef } from 'react'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Button } from '@/components/ui/button'
import { Link } from 'react-router-dom'
import { useDebounceCallback } from '@/lib/hooks/useDebounceCallback'  // standard debounce hook (add if missing)

export function DevicePin({ placement, containerRef, onNudge, onRemoveRequest, onOpenDevice }: {
  placement: PlacementView
  containerRef: React.RefObject<HTMLDivElement>
  onNudge: (xFrac: number, yFrac: number) => void
  onRemoveRequest: () => void
  onOpenDevice: () => void
}) {
  const draggingRef = useRef(false)
  const debouncedNudge = useDebounceCallback(onNudge, 300)

  const stateClass = ({
    healthy: 'bg-success',
    warning: 'bg-warning',
    offline: 'bg-destructive',
  } as const)[placement.state]

  const handlePointerDown = useCallback((e: React.PointerEvent) => {
    e.preventDefault()
    e.stopPropagation()
    ;(e.target as HTMLElement).setPointerCapture(e.pointerId)
    draggingRef.current = false

    const rect = containerRef.current!.getBoundingClientRect()

    const onMove = (move: PointerEvent) => {
      draggingRef.current = true
      const xFrac = Math.max(0, Math.min(1, (move.clientX - rect.left) / rect.width))
      const yFrac = Math.max(0, Math.min(1, (move.clientY - rect.top) / rect.height))
      // Optimistic UI: caller may want to setState for instant feedback; for simplicity
      // here we only fire the (debounced) network call. A separate optimistic-cache
      // hook in the parent can render the in-flight value.
      debouncedNudge(xFrac, yFrac)
    }
    const onUp = () => {
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
    }
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
  }, [containerRef, debouncedNudge])

  return (
    <Popover>
      <PopoverTrigger asChild>
        <div
          data-role="device-pin"
          role="button"
          tabIndex={0}
          aria-label={`Device ${placement.device_name}, ${placement.state}`}
          className={`absolute h-3 w-3 rounded-full ring-2 ring-white shadow-sm -translate-x-1/2 -translate-y-1/2 cursor-pointer ${stateClass}`}
          style={{ left: `${placement.x_frac * 100}%`, top: `${placement.y_frac * 100}%` }}
          onPointerDown={handlePointerDown}
          onContextMenu={(e) => { e.preventDefault(); onRemoveRequest() }}
        />
      </PopoverTrigger>
      <PopoverContent className="w-64">
        <div className="space-y-2">
          <div className="font-semibold">{placement.device_name}</div>
          <div className="text-xs text-muted-foreground">{placement.utility_class}</div>
          <dl className="text-sm space-y-0.5">
            <div className="flex justify-between"><dt className="text-muted-foreground">Last seen</dt><dd>{placement.last_seen_at ? formatRelative(placement.last_seen_at) : '—'}</dd></div>
            <div className="flex justify-between"><dt className="text-muted-foreground">Battery</dt><dd>{placement.battery_pct ?? '—'}%</dd></div>
            <div className="flex justify-between"><dt className="text-muted-foreground">RSSI</dt><dd>{placement.rssi ?? '—'} dBm</dd></div>
          </dl>
          <Button asChild className="w-full" size="sm" onClick={onOpenDevice}>
            <Link to={`/devices/${placement.device_id}`}>Open device</Link>
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  )
}
```

**Step C — `web/src/lib/hooks/useFloorPlanHealth.ts`:**

```ts
import { useEffect, useReducer } from 'react'
import { useSSE } from './useSSE'

export type DeviceHealthState = 'healthy' | 'warning' | 'offline'

type Placement = {
  device_id: string
  metering_point_id: string
  last_seen_at: string | null
  battery_pct: number | null
  rssi: number | null
  expected_interval_s: number
}

function computeState(p: Placement): DeviceHealthState {
  if (!p.last_seen_at) return 'offline'
  const stale = Date.now() - new Date(p.last_seen_at).getTime() > 2 * p.expected_interval_s * 1000
  if (stale) return 'offline'
  if ((p.battery_pct ?? 100) <= 20) return 'warning'
  if ((p.rssi ?? 0) < -110) return 'warning'
  return 'healthy'
}

type Action =
  | { type: 'snapshot'; placements: Placement[] }
  | { type: 'measurement'; mpID: string; battery_pct: number | null; rssi: number | null; quality: string; time: string }

function reducer(state: Record<string, Placement>, action: Action): Record<string, Placement> {
  switch (action.type) {
    case 'snapshot':
      return Object.fromEntries(action.placements.map(p => [p.metering_point_id, p]))
    case 'measurement':
      if (!state[action.mpID]) return state
      const next = { ...state[action.mpID], last_seen_at: action.time, battery_pct: action.battery_pct, rssi: action.rssi }
      return { ...state, [action.mpID]: next }
  }
}

// Returns a map device_id → DeviceHealthState that updates live as SSE
// measurement events arrive. Subscribes to mp:<uuid> topics for every placed
// device's metering point (RESEARCH §Hub Extension Option B — client-side
// state computation, no server-side migration required).
export function useFloorPlanHealth(placements: Placement[]) {
  const [state, dispatch] = useReducer(reducer, {})

  useEffect(() => {
    dispatch({ type: 'snapshot', placements })
  }, [placements])

  useSSE({
    topics: placements.map(p => `mp:${p.metering_point_id}`),
    onMessage: (event) => {
      try {
        const payload = JSON.parse(event.data)
        dispatch({
          type: 'measurement',
          mpID: payload.metering_point_id,
          battery_pct: payload.battery_pct,
          rssi: payload.rssi,
          quality: payload.quality,
          time: payload.time,
        })
      } catch (err) {
        console.warn('useFloorPlanHealth: bad SSE payload', err)
      }
    },
  })

  return Object.fromEntries(
    Object.values(state).map(p => [p.device_id, computeState(p)])
  )
}
```

**Step D — `web/src/components/floor-plan/DeviceSidebar.tsx`:**

```tsx
import { useState } from 'react'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Badge } from '@/components/ui/badge'
import { Crosshair, MapPin, CheckCircle2 } from 'lucide-react'

export function DeviceSidebar({ unplaced, placed, placingDeviceID, onSelectDevice }: {
  unplaced: Array<{ id: string; name: string; utility_class: string }>
  placed: Array<{ id: string; name: string; utility_class: string; state: string }>
  placingDeviceID: string | null
  onSelectDevice: (id: string) => void
}) {
  return (
    <div className="w-64 border-r bg-card p-4 space-y-4">
      <section>
        <h3 className="text-xs font-semibold tracking-wide uppercase text-muted-foreground mb-2">
          Unplaced devices <Badge variant="secondary">{unplaced.length}</Badge>
        </h3>
        {unplaced.length === 0 ? (
          <div className="text-center py-6">
            <CheckCircle2 className="h-8 w-8 mx-auto text-success mb-2"/>
            <div className="text-xs text-muted-foreground">All devices placed</div>
          </div>
        ) : (
          <ScrollArea className="h-48">
            {unplaced.map(d => (
              <button
                key={d.id}
                onClick={() => onSelectDevice(d.id)}
                className={`block w-full text-left px-3 py-2 rounded text-sm hover:bg-accent ${placingDeviceID === d.id ? 'bg-primary/10 border border-primary' : ''}`}
              >
                <Crosshair className="h-3 w-3 inline mr-2"/>
                {d.name}
                <div className="text-xs text-muted-foreground">{d.utility_class}</div>
              </button>
            ))}
          </ScrollArea>
        )}
      </section>

      <section>
        <h3 className="text-xs font-semibold tracking-wide uppercase text-muted-foreground mb-2">
          Placed devices <Badge variant="secondary">{placed.length}</Badge>
        </h3>
        <ScrollArea className="h-72">
          {placed.map(d => (
            <div key={d.id} className="px-3 py-2 text-sm">
              <MapPin className="h-3 w-3 inline mr-2"/>
              {d.name}
              <div className="text-xs text-muted-foreground">{d.utility_class} · {d.state}</div>
            </div>
          ))}
        </ScrollArea>
      </section>
    </div>
  )
}
```

**Step E — `web/src/components/floor-plan/FloorPlanTab.tsx`:**

```tsx
import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { apiFetch } from '@/lib/apiFetch'
import { FloorPlanSelector } from './FloorPlanSelector'
import { FloorPlanCanvas, PlacementView } from './FloorPlanCanvas'
import { DeviceSidebar } from './DeviceSidebar'
import { useFloorPlanHealth } from '@/lib/hooks/useFloorPlanHealth'
import { UploadFloorPlanDialog } from './UploadFloorPlanDialog'
import { ReplaceImageDialog } from './ReplaceImageDialog'
import { RemovePinAlertDialog } from './RemovePinAlertDialog'

export function FloorPlanTab({ siteID }: { siteID: string }) {
  const queryClient = useQueryClient()
  const [activePlanID, setActivePlanID] = useState<string | null>(null)
  const [placingDeviceID, setPlacingDeviceID] = useState<string | null>(null)
  const [removeDeviceID, setRemoveDeviceID] = useState<string | null>(null)

  const { data: plans = [] } = useQuery({ queryKey: ['floor-plans', siteID], queryFn: () => apiFetch<any[]>(`/api/sites/${siteID}/floor-plans`) })
  const activePlan = plans.find(p => p.id === activePlanID) ?? plans[0]

  const { data: placements = [] } = useQuery({
    queryKey: ['placements', activePlan?.id],
    queryFn: () => activePlan ? apiFetch<any[]>(`/api/floor-plans/${activePlan.id}/placements`) : Promise.resolve([]),
    enabled: !!activePlan,
  })
  const { data: siteDevices = [] } = useQuery({ queryKey: ['site-devices', siteID], queryFn: () => apiFetch<any[]>(`/api/sites/${siteID}/devices`) })

  const placedIDs = new Set(placements.map(p => p.device_id))
  const unplacedDevices = siteDevices.filter(d => !placedIDs.has(d.id))
  const states = useFloorPlanHealth(placements)
  const placementViews: PlacementView[] = placements.map(p => ({ ...p, state: states[p.device_id] ?? 'healthy' }))

  const placeMutation = useMutation({
    mutationFn: (vars: { deviceID: string; xFrac: number; yFrac: number }) =>
      apiFetch(`/api/floor-plans/${activePlan.id}/placements`, { method: 'POST', body: JSON.stringify({ device_id: vars.deviceID, x_frac: vars.xFrac, y_frac: vars.yFrac }) }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['placements', activePlan?.id] })
      setPlacingDeviceID(null)
    },
  })

  const nudgeMutation = useMutation({
    mutationFn: (vars: { deviceID: string; xFrac: number; yFrac: number }) =>
      apiFetch(`/api/floor-plans/${activePlan.id}/placements/${vars.deviceID}`, { method: 'PATCH', body: JSON.stringify({ x_frac: vars.xFrac, y_frac: vars.yFrac }) }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['placements', activePlan?.id] }),
  })

  const removeMutation = useMutation({
    mutationFn: (deviceID: string) => apiFetch(`/api/floor-plans/${activePlan.id}/placements/${deviceID}`, { method: 'DELETE' }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['placements', activePlan?.id] })
      setRemoveDeviceID(null)
    },
  })

  // Empty state when no plans
  if (plans.length === 0) {
    return <UploadFirstPlanEmpty siteID={siteID} />
  }

  return (
    <div className="flex h-[calc(100vh-10rem)]">
      <DeviceSidebar
        unplaced={unplacedDevices}
        placed={placementViews}
        placingDeviceID={placingDeviceID}
        onSelectDevice={setPlacingDeviceID}
      />
      <div className="flex-1 flex flex-col">
        <FloorPlanSelector plans={plans} activeID={activePlan.id} onChange={setActivePlanID} />
        <FloorPlanCanvas
          imageSrc={`/api/floor-plans/${activePlan.id}/image`}
          imageW={activePlan.image_w}
          imageH={activePlan.image_h}
          placements={placementViews}
          placingDeviceID={placingDeviceID}
          onPlace={(d, x, y) => placeMutation.mutate({ deviceID: d, xFrac: x, yFrac: y })}
          onNudge={(d, x, y) => nudgeMutation.mutate({ deviceID: d, xFrac: x, yFrac: y })}
          onRemoveRequest={setRemoveDeviceID}
          onOpenDevice={(d) => window.location.href = `/devices/${d}`}
        />
      </div>

      <RemovePinAlertDialog
        deviceName={placementViews.find(p => p.device_id === removeDeviceID)?.device_name ?? ''}
        open={!!removeDeviceID}
        onOpenChange={(o) => !o && setRemoveDeviceID(null)}
        onConfirm={() => removeDeviceID && removeMutation.mutate(removeDeviceID)}
      />
    </div>
  )
}
```

**Step F — Update `web/src/routes/sites/$id.tsx`:**

Add a Floor plan tab; default to it when `plans.length > 0` per D-23:

```tsx
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { FloorPlanTab } from '@/components/floor-plan/FloorPlanTab'

export function SiteDetailPage() {
  const { id } = useParams<{ id: string }>()
  const { data: plans = [] } = useQuery({ queryKey: ['floor-plans', id], queryFn: () => apiFetch<any[]>(`/api/sites/${id}/floor-plans`) })

  const defaultTab = plans.length > 0 ? 'floor-plan' : 'overview'

  return (
    <Tabs defaultValue={defaultTab}>
      <TabsList>
        <TabsTrigger value="overview">Overview</TabsTrigger>
        <TabsTrigger value="floor-plan">Floor plan</TabsTrigger>
      </TabsList>
      <TabsContent value="overview">{/* existing overview content */}</TabsContent>
      <TabsContent value="floor-plan"><FloorPlanTab siteID={id!} /></TabsContent>
    </Tabs>
  )
}
```

**Step G — `RemovePinAlertDialog.tsx`:**

```tsx
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription,
         AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'

export function RemovePinAlertDialog({ deviceName, open, onOpenChange, onConfirm }: {
  deviceName: string
  open: boolean
  onOpenChange: (o: boolean) => void
  onConfirm: () => void
}) {
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Remove from plan?</AlertDialogTitle>
          <AlertDialogDescription>
            {deviceName} will be shown in the unplaced list. You can re-pin it at any time.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction onClick={onConfirm} className="bg-destructive text-destructive-foreground hover:bg-destructive/90">
            Remove
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
```

**Step H — `FloorPlanSelector.tsx`:**

Horizontal pill strip (use shadcn `ToggleGroup` or render Buttons in a flex row). Ordered by `sort_order`. Click a pill → setActivePlanID.

**Step I — Replace vitest bodies in `web/src/components/floor-plan/FloorPlanCanvas.test.tsx` and `DevicePin.test.tsx`:**

Minimum cases (matching the behavior list above).

**Step J — Replace `test.skip` bodies in 3 Playwright specs:**

- `floor-plan-pinning.spec.ts`: upload PNG → place 3 devices → reload → pins persist → decommission one → its pin gone (SITE-04 + D-25)
- `floor-plan-health.spec.ts`: SSE measurement event with low battery → marker re-tints to yellow (SITE-05 + D-22)
- `site-drill-through.spec.ts`: map → site marker popup → "View site" → /sites/:id Floor plan tab default → DevicePinPopover → "Open device" (SITE-06)
  </action>
  <verify>
    <automated>pnpm --dir web test:run --reporter=basic web/src/components/floor-plan/ web/src/lib/pdfToPng.test.ts &amp;&amp; pnpm --dir web build &amp;&amp; pnpm --dir web exec playwright test --list floor-plan-pinning.spec.ts floor-plan-health.spec.ts site-drill-through.spec.ts 2&gt;&amp;1 | grep -c "›"</automated>
  </verify>
  <acceptance_criteria>
    - `web/src/components/floor-plan/FloorPlanCanvas.tsx` contains literals `touch-none`, `cursor-crosshair`, `getBoundingClientRect`, `Math.max(0, Math.min(1,`
    - `web/src/components/floor-plan/DevicePin.tsx` contains literals `bg-success`, `bg-warning`, `bg-destructive`, `ring-2 ring-white`, `-translate-x-1/2 -translate-y-1/2`, `${placement.x_frac * 100}%`, `setPointerCapture`, `onContextMenu`
    - `web/src/lib/hooks/useFloorPlanHealth.ts` contains literal `useSSE`, `2 * p.expected_interval_s * 1000`, `battery_pct ?? 100) <= 20`, `rssi ?? 0) < -110`
    - `web/src/routes/sites/$id.tsx` contains literal `plans.length > 0 ? 'floor-plan' : 'overview'` (D-23)
    - `RemovePinAlertDialog.tsx` confirm button uses `bg-destructive` class
    - `ReplaceImageDialog.tsx` (from Task 1) confirm uses `bg-destructive`
    - All 3 Playwright specs no longer use `test.skip`
    - `pnpm --dir web build` exits 0
    - `pnpm --dir web test:run` exits 0
  </acceptance_criteria>
  <done>Floor-plan UX complete: pin, drag, popover, remove, live SSE state, D-23 default-tab, drill-through path proven by E2E.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| Client → POST /api/sites/:id/floor-plans | multipart; client double-validates dim + size before upload but server is authoritative (plan 05-05 enforces) |
| Client → SSE /api/events for mp:<uuid> topics | Auth-gated (Phase 4); subscriptions filtered server-side; client can't read other tenants' MPs (single-tenant install) |
| Pointer events → fractional coords | Client-side math; clamped to [0,1]; server CHECK constraint catches OOR (plan 05-05) |

## STRIDE Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-05-10-01 | Tampering | Crafted fractional coordinates outside [0,1] | low | mitigate | Client clamps via `Math.max(0, Math.min(1, ...))`; server CHECK constraint enforces (plan 05-05 schema); handler additionally rejects 422 (plan 05-07). Triple defense. |
</threat_model>

<verification>
1. `pnpm --dir web test:run` exits 0
2. `pnpm --dir web build` exits 0
3. `grep -E "bg-(success|warning|destructive)" web/src/components/floor-plan/DevicePin.tsx` returns 3 matches
4. `grep "touch-none" web/src/components/floor-plan/FloorPlanCanvas.tsx` returns ≥1 match
5. `grep "useSSE" web/src/lib/hooks/useFloorPlanHealth.ts` returns ≥1 match
6. `grep "plans.length > 0 ? 'floor-plan' : 'overview'" web/src/routes/sites/$id.tsx` returns 1 match (D-23 enforcement)
7. NO third-party canvas/pin library imports: `! grep -E "react-image-pin|fabric|konva" web/package.json`
</verification>

<success_criteria>
- Floor plan tab default when plans exist (D-23)
- FloorPlanCanvas with fractional-coord math + touch-action: none + cursor-crosshair in place-mode
- DevicePin with 3-state coloring + ring + drag-to-nudge + popover + right-click remove
- pdf.js client-side conversion ships with memory cleanup
- Upload + Replace + Remove dialogs match UI-SPEC copy verbatim
- useFloorPlanHealth subscribes to mp:<uuid> SSE topics and recomputes state client-side per D-22
- 3 Playwright specs cover pinning lifecycle, live health update, and SITE-06 drill-through
</success_criteria>

<output>
After completion, create `.planning/phases/05-aggregates-reports-map-floor-plans/05-10-SUMMARY.md` recording:
- Whether `useDebounceCallback` was added new or reused
- Resolution of any race conditions between optimistic UI and PATCH success
- Confirmed pdf.js OffscreenCanvas support across Chromium / Firefox / Safari (Playwright matrix)
- Open question: do we need DPR-aware rendering for Retina (mid-zoom pin precision)?
</output>
