import { useMutation } from '@tanstack/react-query'
import { Copy, KeyRound, Loader2, ShieldAlert } from 'lucide-react'
import { useEffect, useState } from 'react'
import { toast } from 'sonner'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { ApiError } from '@/lib/api'
import {
  revealDeviceKeys,
  type DeviceKeysResponse,
} from '@/lib/devices'

/**
 * Plan 03-10 — Reveal Keys dialog (DEV-09 / D-22 / D-26..D-28).
 *
 * Shown only from the device detail page (`/devices/:id`) when an admin
 * clicks "Reveal keys". The trigger button itself is gated on
 * `useCurrentUser().role === 'admin'` — viewers never see it. The backend
 * `RequireAction(ActionDeviceRevealSecrets)` middleware is authoritative;
 * this UI hide is defense-in-depth (Plan 03-06).
 *
 * State machine:
 *   idle    → initial render. Warning + Reveal CTA. No network call.
 *   loading → mutation pending (admin clicked Reveal). Spinner.
 *   success → keys panel + per-key value + Copy all keys CTA.
 *   error   → error-message-from-api card + Retry CTA.
 *
 * Security guarantees:
 *   - useMutation is configured with `gcTime: 0` so TanStack Query does NOT
 *     retain the response in its cache (T-3-72 / D-26).
 *   - Local component state is cleared on every close → reopen cycle so a
 *     reopened dialog cannot show stale keys.
 *   - The backend response includes `Cache-Control: no-store, no-cache,
 *     must-revalidate` (Plan 03-06 reveal.go) so browsers + proxies cannot
 *     retain the body either.
 *   - UX-03: no "tenant"/"application" wording.
 */
export interface RevealKeysDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  devEUI: string
}

type ViewState = 'idle' | 'success' | 'error'

interface KeyRow {
  label: string
  value: string
}

const ERROR_MESSAGES: Record<number, string> = {
  401: 'Your session expired. Sign in again to reveal device keys.',
  403: 'You do not have permission to reveal device keys. Ask an administrator.',
  404: 'Device not found in Shifter database.',
  409: 'ChirpStack has no credentials for this device. The device may have been removed externally.',
  502: 'ChirpStack is unreachable. Try again or check Settings → Connection.',
}

function errorMessageFromApi(err: unknown): string {
  if (err instanceof ApiError && ERROR_MESSAGES[err.status]) {
    return ERROR_MESSAGES[err.status]
  }
  if (err instanceof Error && err.message) {
    return `Could not reveal keys — ${err.message}`
  }
  return 'Could not reveal keys. Try again.'
}

function rowsFromKeys(keys: DeviceKeysResponse): KeyRow[] {
  if (keys.activation_mode === 'OTAA') {
    return [
      { label: 'Activation', value: 'OTAA' },
      { label: 'DevEUI', value: keys.dev_eui },
      { label: 'Join EUI', value: keys.join_eui },
      { label: 'AppKey', value: keys.app_key },
      { label: 'NwkKey', value: keys.nwk_key },
    ]
  }
  return [
    { label: 'Activation', value: 'ABP' },
    { label: 'DevEUI', value: keys.dev_eui },
    { label: 'DevAddr', value: keys.dev_addr },
    { label: 'NwkSKey', value: keys.nwk_s_key },
    { label: 'AppSKey', value: keys.app_s_key },
    { label: 'FCnt Up', value: String(keys.f_cnt_up) },
    { label: 'NFCnt Down', value: String(keys.n_f_cnt_down) },
    { label: 'AFCnt Down', value: String(keys.a_f_cnt_down) },
  ]
}

export function RevealKeysDialog({
  open,
  onOpenChange,
  devEUI,
}: RevealKeysDialogProps) {
  const [view, setView] = useState<ViewState>('idle')
  const [keys, setKeys] = useState<DeviceKeysResponse | null>(null)
  const [errorMsg, setErrorMsg] = useState('')

  const mutation = useMutation({
    mutationFn: () => revealDeviceKeys(devEUI),
    // Defense-in-depth: never retain keys in the TanStack Query cache.
    gcTime: 0,
    onSuccess: (data) => {
      setKeys(data)
      setView('success')
    },
    onError: (err: unknown) => {
      setErrorMsg(errorMessageFromApi(err))
      setView('error')
    },
  })

  // Clear local state on close so reopen cannot show stale keys.
  useEffect(() => {
    if (!open) {
      setView('idle')
      setKeys(null)
      setErrorMsg('')
      mutation.reset()
    }
    // mutation.reset is stable; deps intentionally narrow.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  const onReveal = () => {
    mutation.mutate()
  }

  const onCopyAll = async () => {
    if (!keys) return
    try {
      await navigator.clipboard.writeText(JSON.stringify(keys, null, 2))
      toast.success('Keys copied')
    } catch {
      toast.error('Could not copy to clipboard')
    }
  }

  const onCopyOne = async (label: string, value: string) => {
    try {
      await navigator.clipboard.writeText(value)
      toast.success(`${label} copied`)
    } catch {
      toast.error('Could not copy to clipboard')
    }
  }

  const isLoading = mutation.isPending

  return (
    <ResponsiveDialog
      open={open}
      onOpenChange={onOpenChange}
      title="Reveal device keys"
      description="Keys are displayed once. Shifter does not retain them — do not refresh this dialog after copying."
      footer={
        view === 'success' && keys ? (
          <>
            <Button variant="ghost" onClick={() => onOpenChange(false)}>
              Done
            </Button>
            <Button onClick={onCopyAll}>
              <Copy className="mr-2 h-4 w-4" aria-hidden="true" />
              Copy all keys
            </Button>
          </>
        ) : view === 'error' ? (
          <>
            <Button variant="ghost" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button
              onClick={() => {
                setView('idle')
                setErrorMsg('')
                mutation.reset()
              }}
            >
              Try again
            </Button>
          </>
        ) : (
          <>
            <Button
              variant="ghost"
              onClick={() => onOpenChange(false)}
              disabled={isLoading}
            >
              Cancel
            </Button>
            <Button onClick={onReveal} disabled={isLoading}>
              {isLoading ? (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />
              ) : (
                <KeyRound className="mr-2 h-4 w-4" aria-hidden="true" />
              )}
              {isLoading ? 'Revealing…' : 'Reveal keys'}
            </Button>
          </>
        )
      }
    >
      {view === 'idle' ? (
        <div className="flex flex-col gap-3">
          <Alert>
            <ShieldAlert className="h-4 w-4" aria-hidden="true" />
            <AlertDescription>
              Keys will be displayed once. Use Copy to save them — Shifter
              does not retain a copy after you close this dialog.
            </AlertDescription>
          </Alert>
          <p className="text-xs text-muted-foreground">
            This action is recorded in the audit log.
          </p>
        </div>
      ) : null}

      {view === 'success' && keys ? (
        <div className="flex flex-col gap-2">
          {rowsFromKeys(keys).map((row) => (
            <div
              key={row.label}
              className="flex items-center justify-between gap-2 rounded-md border bg-card px-3 py-2"
            >
              <span className="text-sm font-semibold">{row.label}</span>
              <div className="flex items-center gap-2">
                <span className="text-sm font-mono select-all break-all text-right">
                  {row.value}
                </span>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  aria-label={`Copy ${row.label}`}
                  onClick={() => onCopyOne(row.label, row.value)}
                >
                  <Copy className="h-3.5 w-3.5" aria-hidden="true" />
                </Button>
              </div>
            </div>
          ))}
        </div>
      ) : null}

      {view === 'error' ? (
        <Alert variant="destructive">
          <AlertDescription>{errorMsg}</AlertDescription>
        </Alert>
      ) : null}
    </ResponsiveDialog>
  )
}
