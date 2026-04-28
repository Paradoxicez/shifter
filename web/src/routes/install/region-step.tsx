import { useState } from 'react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { ApiError } from '@/lib/api'
import { REGIONS, postStep3 } from '@/lib/install'

const GROUPS = ['asia', 'europe', 'americas', 'oceania', 'india'] as const

/**
 * Wizard step 3 — LoRaWAN region picker.
 *
 * PITFALLS §8: AS923-2 pre-selected for the Thailand operator base; the
 * helper hint reinforces why. Plan 15's POST /api/install/step/3 whitelists
 * the submitted `name` against Plan 14's RegionByName catalog (T-14-04).
 */
export function RegionStep({ onAdvance }: { onAdvance: () => void }) {
  const [selected, setSelected] = useState<string>('as923_2')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const submit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError(null)
    setBusy(true)
    try {
      await postStep3({ name: selected })
      onAdvance()
    } catch (err) {
      if (err instanceof ApiError && err.status === 422) {
        setError("That region isn't available. Pick another.")
      } else {
        setError('Something went wrong. Try again.')
      }
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={submit} className="flex flex-col gap-4">
      <div>
        <h2 className="text-2xl font-semibold leading-8">Choose your LoRaWAN region</h2>
        <p className="text-sm text-muted-foreground">
          This sets the default frequency plan for new gateways. You can override per-gateway later.
        </p>
      </div>
      {error ? (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}
      <div className="flex flex-col gap-2">
        <Label htmlFor="region">Region</Label>
        <Select value={selected} onValueChange={setSelected}>
          <SelectTrigger id="region" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {GROUPS.map((g) => (
              <SelectGroup key={g}>
                <SelectLabel className="capitalize">{g}</SelectLabel>
                {REGIONS.filter((r) => r.group === g).map((r) => (
                  <SelectItem key={r.name} value={r.name}>
                    {r.display}
                  </SelectItem>
                ))}
              </SelectGroup>
            ))}
          </SelectContent>
        </Select>
        {selected === 'as923_2' ? (
          <p className="text-sm text-muted-foreground">
            We pre-selected AS923-2 because the install address is in Thailand.
          </p>
        ) : null}
      </div>
      <div className="flex justify-end pt-2">
        <Button type="submit" disabled={busy}>
          {busy ? 'Saving…' : 'Next'}
        </Button>
      </div>
    </form>
  )
}
