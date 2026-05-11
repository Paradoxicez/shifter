import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, Plus, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Textarea } from '@/components/ui/textarea'
import { ApiError } from '@/lib/api'
import { flattenJSON, tryParseJSON } from '@/lib/json-flatten'
import {
  createProfile,
  getProfileWithMappings,
  type Mapping,
  type ProfileRequest,
  updateProfile,
} from '@/lib/profiles'

/**
 * UI-SPEC §Profile editor page (D-08).
 *
 * Single-page editor (NOT a dialog) — UI-SPEC explicit UX-01 deviation:
 * a profile carries too many fields + a live preview to fit in <Dialog>.
 *
 *   Top bar:        [← Profiles] + Profile name + [Save profile] [Cancel]
 *   Identity row:   Vendor + Family + Counter modulus
 *   Capability:     10 D-04 capability checkboxes
 *   Body (lg+):     Two-pane — left: paste JSON + clickable tree; right:
 *                   editable mapping table + codec_js textarea.
 *   Footer card:    Live preview (Sample → Mapping → Measurement row).
 *
 * Backend contract: POST/PATCH /api/device-profiles with ProfileRequest.
 *   400 "invalid capability" → inline alert "Choose valid capabilities".
 *   413 codec_too_large      → inline alert with the message.
 *   503 chirpstack_not_bootstrapped → inline alert with the message.
 */

/**
 * Capability tokens — D-04 vocabulary, mirrored to migration 0009 CHECK
 * constraint and validCapabilities in internal/profile/editor.go.
 */
const CAPABILITY_TOKENS: { value: string; label: string }[] = [
  { value: 'cumulative', label: 'Cumulative' },
  { value: 'flow_rate', label: 'Flow rate' },
  { value: 'instant_power', label: 'Instant power' },
  { value: 'battery', label: 'Battery' },
  { value: 'temperature', label: 'Temperature' },
  { value: 'pressure', label: 'Pressure' },
  { value: 'leak_detection', label: 'Leak detection' },
  { value: 'tamper_detection', label: 'Tamper detection' },
  { value: 'multi_phase', label: 'Multi-phase' },
  { value: 'power_quality', label: 'Power quality' },
]

/**
 * Canonical column targets — D-02 + D-04. Mappings can also target
 * "extra.<key>" by typing it directly into the combobox. This list seeds
 * the dropdown but the input itself is also free-form.
 */
const CANONICAL_TARGETS = [
  'cumulative_value',
  'instant_value',
  'flow_rate',
  'battery_pct',
  'temperature_c',
  'pressure_kpa',
  'voltage_v',
  'current_a',
  'power_w',
  'energy_wh',
]

const DATA_TYPES: Mapping['data_type'][] = ['numeric', 'int', 'bool', 'text']

export interface MappingEditorProps {
  /** Existing profile id when mode === "edit"; ignored when mode === "new". */
  profileId?: string
  mode: 'new' | 'edit'
}

/** Local row shape — adds a `_uid` so React keys stay stable when rows
 *  reorder/delete. */
interface EditableMapping extends Mapping {
  _uid: string
}

function newRow(position: number): EditableMapping {
  return {
    _uid: `r${Math.random().toString(36).slice(2, 9)}`,
    json_pointer: '',
    target: '',
    scale: '1',
    data_type: 'numeric',
    position,
  }
}

export function MappingEditor({ profileId, mode }: MappingEditorProps) {
  const qc = useQueryClient()
  const navigate = useNavigate()

  // ----- Form state ------------------------------------------------------
  const [name, setName] = useState('')
  const [slug, setSlug] = useState('')
  const [vendor, setVendor] = useState('')
  const [family, setFamily] = useState('')
  const [region, setRegion] = useState('')
  const [macVersion, setMacVersion] = useState('1.0.4')
  const [counterModulus, setCounterModulus] = useState('4294967296')
  const [capabilities, setCapabilities] = useState<string[]>([])
  const [codecJS, setCodecJS] = useState('')
  const [rows, setRows] = useState<EditableMapping[]>(() => [newRow(0)])

  // Sample JSON textarea + click-to-bind active row.
  const [sampleText, setSampleText] = useState('')
  const [activeRowUid, setActiveRowUid] = useState<string | null>(null)

  // Save error surface.
  const [saveError, setSaveError] = useState<string | null>(null)

  // ----- Load existing profile (edit mode) ------------------------------
  const profileQuery = useQuery({
    queryKey: ['device-profile', profileId],
    queryFn: () => getProfileWithMappings(profileId as string),
    enabled: mode === 'edit' && Boolean(profileId),
  })

  // Hydrate state from loaded profile. Only fires once per id.
  useEffect(() => {
    if (mode !== 'edit') return
    const data = profileQuery.data
    if (!data) return
    setName(data.profile.name)
    setSlug(data.profile.slug)
    setVendor(data.profile.vendor)
    setFamily(data.profile.family ?? '')
    setRegion(data.profile.region ?? '')
    setMacVersion(data.profile.mac_version)
    setCounterModulus(String(data.profile.counter_modulus))
    setCapabilities(data.profile.capabilities ?? [])
    setCodecJS(data.profile.codec_js ?? '')
    if (data.mappings.length > 0) {
      setRows(
        data.mappings
          .slice()
          .sort((a, b) => a.position - b.position)
          .map((m, i) => ({
            _uid: `srv-${m.id ?? i}`,
            json_pointer: m.json_pointer,
            target: m.target,
            scale: m.scale || '1',
            data_type: m.data_type,
            position: m.position,
          })),
      )
    }
  }, [mode, profileQuery.data])

  // ----- JSON tree leaves -------------------------------------------------
  const sampleLeaves = useMemo(() => {
    const parsed = tryParseJSON(sampleText)
    if (parsed === null) return null
    const flat = flattenJSON(parsed)
    return flat
  }, [sampleText])

  // ----- Save mutation ---------------------------------------------------
  const saveMutation = useMutation({
    mutationFn: async () => {
      const body: ProfileRequest = {
        slug: slug.trim(),
        name: name.trim(),
        vendor: vendor.trim(),
        family: family.trim() || undefined,
        capabilities,
        counter_modulus: Number(counterModulus),
        region: region.trim() || undefined,
        mac_version: macVersion.trim(),
        codec_js: codecJS || undefined,
        mappings: rows
          .filter((r) => r.target.trim() || r.json_pointer.trim())
          .map((r, i) => ({
            json_pointer: r.json_pointer,
            target: r.target,
            scale: r.scale || '1',
            data_type: r.data_type,
            position: i,
          })),
      }
      if (mode === 'edit' && profileId) {
        return updateProfile(profileId, body)
      }
      return createProfile(body)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['device-profiles'] })
      if (mode === 'edit' && profileId) {
        qc.invalidateQueries({ queryKey: ['device-profile', profileId] })
      }
      const msg = codecJS.trim()
        ? 'Profile saved. Codec synced to ChirpStack.'
        : 'Profile saved.'
      toast.success(msg)
      setSaveError(null)
      navigate('/profiles')
    },
    onError: (err: unknown) => {
      const status = err instanceof ApiError ? err.status : 0
      const detail =
        err instanceof ApiError
          ? typeof err.body === 'object' && err.body !== null && 'detail' in err.body
            ? String((err.body as { detail?: unknown }).detail ?? err.message)
            : err.message
          : 'Could not save profile.'

      // 400 'invalid capability' → friendly UI-SPEC copy
      if (status === 400 && /invalid capability/i.test(detail)) {
        setSaveError('Choose valid capabilities')
        toast.error('Could not save profile')
        return
      }
      if (status === 413) {
        setSaveError(detail)
        toast.error('Codec too large')
        return
      }
      if (status === 503) {
        setSaveError(
          'ChirpStack is not connected. Open Settings → Test connection.',
        )
        toast.error('ChirpStack not connected')
        return
      }
      setSaveError(detail)
      toast.error('Could not save profile')
    },
  })

  // ----- Row helpers -----------------------------------------------------
  const updateRow = (uid: string, patch: Partial<EditableMapping>) => {
    setRows((prev) => prev.map((r) => (r._uid === uid ? { ...r, ...patch } : r)))
  }
  const removeRow = (uid: string) => {
    setRows((prev) => prev.filter((r) => r._uid !== uid))
  }
  const addRow = () => {
    setRows((prev) => {
      const next = [...prev, newRow(prev.length)]
      setActiveRowUid(next[next.length - 1]._uid)
      return next
    })
  }

  const onLeafClick = (pointer: string) => {
    if (!activeRowUid) {
      // No active row — bind to the first row by default.
      if (rows.length > 0) {
        updateRow(rows[0]._uid, { json_pointer: pointer })
        setActiveRowUid(rows[0]._uid)
      }
      return
    }
    updateRow(activeRowUid, { json_pointer: pointer })
  }

  // ----- Capability checkbox helpers ------------------------------------
  const toggleCapability = (token: string, checked: boolean) => {
    setCapabilities((prev) =>
      checked ? Array.from(new Set([...prev, token])) : prev.filter((t) => t !== token),
    )
  }

  const isLoading = mode === 'edit' && profileQuery.isLoading

  return (
    <div className="flex flex-col gap-6 p-6">
      {/* Top bar */}
      <header className="flex items-center justify-between gap-4">
        <div className="flex items-center gap-3">
          <Button asChild variant="ghost" size="sm">
            <Link to="/profiles">
              <ArrowLeft className="mr-2 h-4 w-4" /> Profiles
            </Link>
          </Button>
          <div className="flex flex-col gap-1">
            <Label htmlFor="profile-name" className="sr-only">
              Profile name
            </Label>
            <Input
              id="profile-name"
              aria-label="Profile name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Profile name"
              className="text-lg font-semibold"
            />
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Button
            type="button"
            variant="ghost"
            onClick={() => navigate('/profiles')}
          >
            Cancel
          </Button>
          <Button
            type="button"
            onClick={() => saveMutation.mutate()}
            disabled={saveMutation.isPending || isLoading}
          >
            {saveMutation.isPending ? 'Saving and syncing codec…' : 'Save profile'}
          </Button>
        </div>
      </header>

      {/* Inline alert above mapping table for save errors */}
      {saveError ? (
        <Alert variant="destructive">
          <AlertDescription>{saveError}</AlertDescription>
        </Alert>
      ) : null}

      {/* Identity row */}
      <section className="grid grid-cols-1 gap-4 lg:grid-cols-4">
        <div className="flex flex-col gap-2">
          <Label htmlFor="profile-slug">Slug</Label>
          <Input
            id="profile-slug"
            aria-label="Slug"
            value={slug}
            onChange={(e) => setSlug(e.target.value)}
            disabled={mode === 'edit'}
            placeholder="axioma_w1"
          />
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="profile-vendor">Vendor</Label>
          <Input
            id="profile-vendor"
            aria-label="Vendor"
            value={vendor}
            onChange={(e) => setVendor(e.target.value)}
            placeholder="Axioma"
          />
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="profile-family">Family</Label>
          <Input
            id="profile-family"
            aria-label="Family"
            value={family}
            onChange={(e) => setFamily(e.target.value)}
            placeholder="W1"
          />
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="profile-counter-modulus">Counter modulus</Label>
          <Input
            id="profile-counter-modulus"
            aria-label="Counter modulus"
            type="text"
            inputMode="numeric"
            value={counterModulus}
            onChange={(e) => setCounterModulus(e.target.value)}
          />
          <p className="text-xs text-muted-foreground">
            Roll-over modulus. Most water meters: 4294967296 (2³²). Most kWh
            meters with 7-digit displays: 10000000.
          </p>
        </div>
      </section>

      {/* MAC version + Region */}
      <section className="grid grid-cols-1 gap-4 lg:grid-cols-4">
        <div className="flex flex-col gap-2">
          <Label htmlFor="profile-mac-version">MAC version</Label>
          <Input
            id="profile-mac-version"
            aria-label="MAC version"
            value={macVersion}
            onChange={(e) => setMacVersion(e.target.value)}
            placeholder="1.0.4"
          />
        </div>
        <div className="flex flex-col gap-2">
          <Label htmlFor="profile-region">Region</Label>
          <Input
            id="profile-region"
            aria-label="Region"
            value={region}
            onChange={(e) => setRegion(e.target.value)}
            placeholder="AS923"
          />
        </div>
      </section>

      {/* Capability checkboxes */}
      <section>
        <Label>Capabilities</Label>
        <div className="mt-2 grid grid-cols-2 gap-3 lg:grid-cols-5">
          {CAPABILITY_TOKENS.map((c) => (
            <label
              key={c.value}
              className="flex items-center gap-2 rounded-md border p-2 text-sm"
            >
              <Checkbox
                checked={capabilities.includes(c.value)}
                onCheckedChange={(v) => toggleCapability(c.value, v === true)}
                aria-label={c.label}
              />
              <span>{c.label}</span>
            </label>
          ))}
        </div>
      </section>

      {/* Two-pane body */}
      <section className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        {/* Left pane — sample JSON + tree */}
        <div className="flex flex-col gap-3">
          <div className="flex flex-col gap-2">
            <Label htmlFor="sample-json">Sample decoded JSON</Label>
            <Textarea
              id="sample-json"
              aria-label="Sample decoded JSON"
              value={sampleText}
              onChange={(e) => setSampleText(e.target.value)}
              rows={10}
              className="font-mono text-sm"
              placeholder='{"cumul": 12345, "battery": 87}'
            />
            <p className="text-xs text-muted-foreground">
              Paste sample decoded JSON above. Once you paste a sample, click
              any leaf to bind it to a mapping row.
            </p>
          </div>

          {sampleLeaves ? (
            <div className="rounded-md border">
              <div className="border-b p-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                Leaves ({sampleLeaves.leaves.length}
                {sampleLeaves.truncated ? ', truncated' : ''})
              </div>
              <ul className="flex flex-col gap-1 p-2">
                {sampleLeaves.leaves.map((leaf) => (
                  <li key={leaf.json_pointer}>
                    <button
                      type="button"
                      onClick={() => onLeafClick(leaf.json_pointer)}
                      className="flex w-full items-center justify-between rounded-md px-2 py-1 text-left text-sm font-mono hover:bg-secondary"
                      title="Bind to mapping row"
                    >
                      <span>{leaf.json_pointer || '(root)'}</span>
                      <span className="text-muted-foreground">
                        {String(leaf.value)}
                      </span>
                    </button>
                  </li>
                ))}
              </ul>
            </div>
          ) : sampleText.trim() ? (
            <p className="text-sm text-destructive">Invalid JSON.</p>
          ) : null}
        </div>

        {/* Right pane — editable mapping table + codec_js */}
        <div className="flex flex-col gap-3">
          <Label>Mappings</Label>
          <div className="rounded-md border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-1/4">json_pointer</TableHead>
                  <TableHead className="w-1/4">target</TableHead>
                  <TableHead className="w-[80px]">scale</TableHead>
                  <TableHead className="w-[110px]">data_type</TableHead>
                  <TableHead className="w-[40px]"> </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.map((row) => {
                  const isActive = activeRowUid === row._uid
                  return (
                    <TableRow
                      key={row._uid}
                      className={
                        isActive
                          ? 'border-l-2 border-primary bg-primary/10'
                          : undefined
                      }
                      onClick={() => setActiveRowUid(row._uid)}
                    >
                      <TableCell>
                        <Input
                          aria-label={`json_pointer ${row.position}`}
                          value={row.json_pointer}
                          onFocus={() => setActiveRowUid(row._uid)}
                          onChange={(e) =>
                            updateRow(row._uid, { json_pointer: e.target.value })
                          }
                          className="font-mono text-sm"
                        />
                      </TableCell>
                      <TableCell>
                        <Input
                          aria-label={`target ${row.position}`}
                          value={row.target}
                          onFocus={() => setActiveRowUid(row._uid)}
                          onChange={(e) =>
                            updateRow(row._uid, { target: e.target.value })
                          }
                          list={`target-options-${row._uid}`}
                          className="font-mono text-sm"
                        />
                        <datalist id={`target-options-${row._uid}`}>
                          {CANONICAL_TARGETS.map((t) => (
                            <option key={t} value={t} />
                          ))}
                        </datalist>
                      </TableCell>
                      <TableCell>
                        <Input
                          aria-label={`scale ${row.position}`}
                          value={row.scale}
                          onChange={(e) =>
                            updateRow(row._uid, { scale: e.target.value })
                          }
                          className="text-sm"
                        />
                      </TableCell>
                      <TableCell>
                        <Select
                          value={row.data_type}
                          onValueChange={(v) =>
                            updateRow(row._uid, {
                              data_type: v as Mapping['data_type'],
                            })
                          }
                        >
                          <SelectTrigger
                            aria-label={`data_type ${row.position}`}
                            className="text-sm"
                          >
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            {DATA_TYPES.map((dt) => (
                              <SelectItem key={dt} value={dt}>
                                {dt}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </TableCell>
                      <TableCell>
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          aria-label={`Delete mapping ${row.position}`}
                          onClick={() => removeRow(row._uid)}
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          </div>
          <Button type="button" variant="ghost" size="sm" onClick={addRow}>
            <Plus className="mr-2 h-4 w-4" />
            Add mapping
          </Button>

          <div className="flex flex-col gap-2">
            <Label htmlFor="codec-js">Codec (codec_js)</Label>
            <Textarea
              id="codec-js"
              aria-label="Codec JS"
              value={codecJS}
              onChange={(e) => setCodecJS(e.target.value)}
              rows={8}
              className="font-mono text-sm leading-6"
              placeholder="function decodeUplink(input) { return { data: {} }; }"
            />
            <p className="text-xs text-muted-foreground">
              Sync to ChirpStack on save.
            </p>
          </div>
        </div>
      </section>

      {/* Live preview footer */}
      <Card>
        <CardContent className="grid grid-cols-1 gap-4 lg:grid-cols-3">
          <div>
            <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
              Sample JSON
            </p>
            <pre className="mt-2 max-h-32 overflow-auto rounded-md bg-secondary p-2 text-xs font-mono">
              {sampleText.trim() || '(empty)'}
            </pre>
          </div>
          <div>
            <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
              Mapping
            </p>
            <ul className="mt-2 flex flex-col gap-1 text-xs font-mono">
              {rows
                .filter((r) => r.json_pointer || r.target)
                .map((r) => (
                  <li key={r._uid}>
                    {r.json_pointer || '(empty)'} → {r.target || '(empty)'}
                  </li>
                ))}
              {rows.every((r) => !r.json_pointer && !r.target) ? (
                <li className="text-muted-foreground">No rows yet</li>
              ) : null}
            </ul>
          </div>
          <div>
            <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
              Measurement row preview
            </p>
            <p className="mt-2 text-xs text-muted-foreground">
              Live preview arrives in Phase 4 with the chart surface.
            </p>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
