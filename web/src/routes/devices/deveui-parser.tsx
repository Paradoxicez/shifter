import { useQuery } from '@tanstack/react-query'
import { useEffect, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { parseDevEUI } from '@/lib/devices'

export interface DevEUIParserProps {
  /** Emits the chosen DevEUI as lowercase 16-hex with no separators. */
  onPick: (devEUI: string) => void
}

/**
 * Format a 16-char hex string with `:` between byte pairs for display.
 * UI-SPEC §Add Device step 1: "01:02:03:04:05:06:07:08".
 */
function formatColons(hex: string): string {
  return hex.match(/.{1,2}/g)?.join(':') ?? hex
}

/**
 * Strip whitespace, ':', '-' and lowercase. Mirrors the backend's strip+lower
 * normalization in internal/device/deveui.go ParseDevEUI.
 */
function normalize(raw: string): string {
  return raw.replace(/[\s:\-]/g, '').toLowerCase()
}

/**
 * UI-SPEC §Add Device dialog Step 1 — D-11 reusable DevEUI parser.
 *
 *   - Input accepts paste with spaces/hyphens/colons; auto-normalizes on blur.
 *   - When normalized length is 16, calls POST /api/devices/parse-deveui to
 *     get both MSB-first + LSB-first interpretations + vendor OUI hints.
 *   - Renders two radio rows ("MSB-first: 01:02..." / "LSB-first: 08:07...");
 *     operator picks one and clicks "Use this DevEUI" → onPick(lowercaseHex).
 *
 * Validation: error "Enter a 16-character hex DevEUI." when len(stripped) != 16.
 */
export function DevEUIParser({ onPick }: DevEUIParserProps) {
  const [raw, setRaw] = useState('')
  const [pick, setPick] = useState<'msb' | 'lsb'>('msb')

  const cleaned = normalize(raw)
  const eligible = cleaned.length === 16 && /^[0-9a-f]{16}$/.test(cleaned)

  const previewQuery = useQuery({
    queryKey: ['parse-deveui', cleaned],
    queryFn: () => parseDevEUI(cleaned),
    enabled: eligible,
    staleTime: Infinity,
  })

  // Reset radio selection when the input changes.
  useEffect(() => {
    setPick('msb')
  }, [cleaned])

  const onBlur = () => {
    // Normalize the input value on blur (UI-SPEC §New field patterns: hex
    // input auto-strips whitespace/hyphens/colons on blur and lowercases).
    if (cleaned !== raw) setRaw(cleaned)
  }

  const onConfirm = () => {
    if (!previewQuery.data) return
    onPick(pick === 'msb' ? previewQuery.data.msb : previewQuery.data.lsb)
  }

  const showLengthError = raw.length > 0 && cleaned.length > 0 && !eligible

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-2">
        <Label htmlFor="deveui-input">DevEUI</Label>
        <Input
          id="deveui-input"
          className="font-mono"
          placeholder="0102030405060708"
          value={raw}
          onChange={(e) => setRaw(e.target.value)}
          onBlur={onBlur}
        />
        {showLengthError ? (
          <p className="text-sm text-destructive">Enter a 16-character hex DevEUI.</p>
        ) : null}
      </div>

      {previewQuery.data ? (
        <RadioGroup value={pick} onValueChange={(v) => setPick(v as 'msb' | 'lsb')}>
          <Card className="px-4 py-3">
            <Label htmlFor="deveui-msb" className="flex items-center gap-3 cursor-pointer">
              <RadioGroupItem id="deveui-msb" value="msb" />
              <span className="text-sm">
                MSB-first:{' '}
                <span className="font-mono">{formatColons(previewQuery.data.msb)}</span>
                {previewQuery.data.msb_vendor && previewQuery.data.msb_vendor !== 'unknown' ? (
                  <span className="ml-2 text-xs text-muted-foreground">
                    (Vendor: {previewQuery.data.msb_vendor})
                  </span>
                ) : null}
              </span>
            </Label>
          </Card>
          <Card className="px-4 py-3">
            <Label htmlFor="deveui-lsb" className="flex items-center gap-3 cursor-pointer">
              <RadioGroupItem id="deveui-lsb" value="lsb" />
              <span className="text-sm">
                LSB-first:{' '}
                <span className="font-mono">{formatColons(previewQuery.data.lsb)}</span>
                {previewQuery.data.lsb_vendor && previewQuery.data.lsb_vendor !== 'unknown' ? (
                  <span className="ml-2 text-xs text-muted-foreground">
                    (Vendor: {previewQuery.data.lsb_vendor})
                  </span>
                ) : null}
              </span>
            </Label>
          </Card>
        </RadioGroup>
      ) : null}

      {previewQuery.data ? (
        <div className="flex justify-end">
          <Button type="button" onClick={onConfirm}>
            Use this DevEUI
          </Button>
        </div>
      ) : null}
    </div>
  )
}
