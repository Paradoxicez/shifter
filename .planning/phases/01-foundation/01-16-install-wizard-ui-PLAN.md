---
phase: 01-foundation
plan: 16
type: execute
wave: 10
depends_on: [06, 11, 15]
files_modified:
  - web/src/lib/install.ts
  - web/src/routes/install/index.tsx
  - web/src/routes/install/admin-step.tsx
  - web/src/routes/install/chirpstack-step.tsx
  - web/src/routes/install/region-step.tsx
  - web/src/routes/install/identity-step.tsx
  - web/src/routes/install/review-step.tsx
  - web/src/routes/install/region-step.test.tsx
  - web/src/routes/_root.tsx
  - web/src/App.tsx
autonomous: true
requirements:
  - INST-01
  - INST-02
  - INST-03
  - INST-04
  - INST-05
  - UX-01
must_haves:
  truths:
    - "/install route renders 5 stepped flow using Stepper component (Plan 06)"
    - "Step 1 form: email, name, password, confirm password (UI-SPEC verbatim copy)"
    - "Step 2 form: bundled/external mode radio + grpc_url + api_token + mqtt_url + optional mqtt_user/mqtt_password"
    - "Step 2 v3 error renders destructive Alert with UI-SPEC verbatim copy 'Shifter doesn't support ChirpStack v3'"
    - "Step 3 region picker pre-selects 'as923_2' with 'Thailand pre-selected' helper hint"
    - "Step 4 identity form: display_name, address, timezone (system default), units radio (metric/imperial)"
    - "Step 5 review summarizes all 4 steps with 'Edit' links + 'Finish setup' CTA"
    - "rootLoader composes install-state-check before session-check: if /api/install/state returns valid state (not 410), redirect to /install"
    - "After finish success, navigate to /login"
  artifacts:
    - path: "web/src/lib/install.ts"
      provides: "Typed install client (fetchInstallState, postStep1..4, postFinish, fetchRegions)"
      contains: "fetchInstallState"
    - path: "web/src/routes/install/index.tsx"
      provides: "Wizard router shell using current_step"
      contains: "current_step"
    - path: "web/src/routes/install/admin-step.tsx"
      provides: "Step 1 form with password strength meter (UI-SPEC §Force-change-password)"
      contains: "AdminStep"
    - path: "web/src/routes/install/chirpstack-step.tsx"
      provides: "Step 2 form with v3 error banner"
      contains: "v3_detected"
    - path: "web/src/routes/install/region-step.tsx"
      provides: "Step 3 region picker; AS923-2 pre-selected for Thailand (PITFALLS §8)"
      contains: "as923_2"
    - path: "web/src/routes/install/identity-step.tsx"
      provides: "Step 4 install identity form"
      contains: "display_name"
    - path: "web/src/routes/install/review-step.tsx"
      provides: "Step 5 review + Finish button"
      contains: "Finish setup"
  key_links:
    - from: "web/src/routes/install/*"
      to: "/api/install/*"
      via: "apiFetch posts to step endpoints"
      pattern: "/api/install"
    - from: "web/src/routes/_root.tsx rootLoader"
      to: "/api/install/state"
      via: "redirect to /install on 200, continue on 410"
      pattern: "install/state"
---

<objective>
Implement the 5-step install wizard frontend at `/install` using shadcn primitives + Stepper from Plan 06 + react-hook-form + zod. Each step is a route segment; backend submission posts to Plan 15's endpoints. Pre-select AS923-2 for Thailand (PITFALLS §8). On finish success, navigate to /login.

Purpose: INST-01..05 frontend; UX-01 (modal-first, but install wizard is the canonical full-screen stepped exception per UI-SPEC §"Install wizard (full-screen stepped dialog)").

Output: A clean test of the wizard flow runs end-to-end against a backend stub (or mocked apiFetch); region step test passes; `pnpm build` exits 0.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/01-foundation/01-UI-SPEC.md
@01-06-frontend-shell-PLAN.md
@01-11-account-ui-PLAN.md
@01-15-install-handlers-PLAN.md

<interfaces>
UI-SPEC §"Install wizard (full-screen stepped dialog)" (lines 237-247) — exact step list and copy strings.
UI-SPEC §"Phase 1 copy table" Wizard rows (lines 553-577) — verbatim strings.

Backend endpoints (Plan 15):
- GET /api/install/state — singleton state including current_step + drafts
- POST /api/install/step/{1..4} — { current_step }
- POST /api/install/finish — { ok: true }

Frontend client API:
```ts
export interface InstallState { current_step: number; CurrentStep?: number; step1_admin?: any; step2_chirpstack?: any; step3_region?: any; step4_identity?: any }
export async function fetchInstallState(): Promise<InstallState | null>  // null when 410 Gone (completed)
export async function postStep1(body: Step1Body): Promise<{ current_step: number }>
export async function postStep2(body: Step2Body): Promise<{ current_step: number; chirpstack_version: string }>
export async function postStep3(body: { name: string }): Promise<{ current_step: number }>
export async function postStep4(body: Step4Body): Promise<{ current_step: number }>
export async function postFinish(): Promise<{ ok: true }>
export async function fetchRegions(): Promise<Region[]>  // GET /api/install/regions — Plan 18 mounts; backend returns install.Regions()
```

Region catalog endpoint: Plan 18 will mount `GET /api/install/regions` returning `install.Regions()`. For Plan 16, the frontend can either fetch from this endpoint or hardcode the same catalog inline. Hardcoding is simpler and matches RESEARCH §Pattern 13 example. The test file expects the inline catalog.
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 1: install.ts client + route shell + admin/chirpstack/region/identity/review steps</name>
  <files>web/src/lib/install.ts, web/src/routes/install/index.tsx, web/src/routes/install/admin-step.tsx, web/src/routes/install/chirpstack-step.tsx, web/src/routes/install/region-step.tsx, web/src/routes/install/identity-step.tsx, web/src/routes/install/review-step.tsx, web/src/App.tsx</files>
  <read_first>
    - .planning/phases/01-foundation/01-UI-SPEC.md §"Install wizard (full-screen stepped dialog)" (lines 237-247)
    - .planning/phases/01-foundation/01-UI-SPEC.md §"Phase 1 copy table" Wizard rows (lines 553-577) — verbatim
    - .planning/phases/01-foundation/01-RESEARCH.md §"Pattern 13: AS923 region picker" (lines 1013-1036) — region catalog
    - 01-06-frontend-shell-PLAN.md (Stepper component)
    - 01-11-account-ui-PLAN.md (apiFetch + ApiError patterns)
  </read_first>
  <action>
1. Create `web/src/lib/install.ts`:
   ```ts
   import { ApiError, apiFetch } from './api'

   export interface InstallState {
     CurrentStep: number       // pgx-scanned struct field name from Plan 15
     StartedAt: string
     CompletedAt: string | null
     Step1Admin: unknown
     Step2ChirpStack: unknown
     Step3Region: unknown
     Step4Identity: unknown
   }

   /** null when install is already completed (410 Gone). */
   export async function fetchInstallState(): Promise<InstallState | null> {
     try {
       return await apiFetch<InstallState>('/api/install/state')
     } catch (err) {
       if (err instanceof ApiError && err.status === 410) return null
       throw err
     }
   }

   export interface Step1Body { email: string; name: string; password: string }
   export interface Step2Body {
     mode: 'bundled' | 'external'
     grpc_url: string
     api_token: string
     mqtt_url: string
     mqtt_user?: string
     mqtt_password?: string
   }
   export interface Step4Body {
     display_name: string
     address?: string
     timezone: string
     units: 'metric' | 'imperial'
     logo_path?: string
   }

   export const postStep1 = (b: Step1Body) =>
     apiFetch<{ current_step: number }>('/api/install/step/1', { method: 'POST', body: JSON.stringify(b) })
   export const postStep2 = (b: Step2Body) =>
     apiFetch<{ current_step: number; chirpstack_version: string }>('/api/install/step/2', { method: 'POST', body: JSON.stringify(b) })
   export const postStep3 = (b: { name: string }) =>
     apiFetch<{ current_step: number }>('/api/install/step/3', { method: 'POST', body: JSON.stringify(b) })
   export const postStep4 = (b: Step4Body) =>
     apiFetch<{ current_step: number }>('/api/install/step/4', { method: 'POST', body: JSON.stringify(b) })
   export const postFinish = () =>
     apiFetch<{ ok: true }>('/api/install/finish', { method: 'POST' })

   export interface Region {
     name: string
     display: string
     common_name: string
     group: 'asia' | 'europe' | 'americas' | 'oceania' | 'india'
     default_for_country?: string
     note?: string
   }

   /** Hardcoded catalog mirrors internal/install/regions.go. */
   export const REGIONS: Region[] = [
     { name: 'as923',   display: 'AS923-1',                 common_name: 'AS923',   group: 'asia' },
     { name: 'as923_2', display: 'AS923-2 (Thailand)',      common_name: 'AS923_2', group: 'asia',
       default_for_country: 'TH', note: 'Required by Thai regulator NBTC.' },
     { name: 'as923_3', display: 'AS923-3',                 common_name: 'AS923_3', group: 'asia' },
     { name: 'as923_4', display: 'AS923-4',                 common_name: 'AS923_4', group: 'asia' },
     { name: 'eu868',   display: 'EU868 (Europe)',          common_name: 'EU868',   group: 'europe' },
     { name: 'us915_0', display: 'US915 sub-band 1 (ch 0-7)', common_name: 'US915', group: 'americas' },
     { name: 'au915_0', display: 'AU915 sub-band 1',         common_name: 'AU915',  group: 'oceania' },
     { name: 'in865',   display: 'IN865 (India)',            common_name: 'IN865',  group: 'india' },
   ]
   ```

2. Create `web/src/routes/install/index.tsx` (the wizard shell):
   ```tsx
   import { useEffect, useState } from 'react'
   import { useNavigate } from 'react-router-dom'
   import { Stepper } from '@/components/stepper'
   import shifterLogo from '@/assets/shifter-logo.svg'
   import { fetchInstallState, type InstallState } from '@/lib/install'
   import { AdminStep } from './admin-step'
   import { ChirpStackStep } from './chirpstack-step'
   import { IdentityStep } from './identity-step'
   import { RegionStep } from './region-step'
   import { ReviewStep } from './review-step'

   const STEPS = [
     { label: 'Admin' },
     { label: 'ChirpStack' },
     { label: 'Region' },
     { label: 'Identity' },
     { label: 'Review' },
   ]

   export default function InstallWizard() {
     const navigate = useNavigate()
     const [state, setState] = useState<InstallState | null>(null)
     const [loading, setLoading] = useState(true)

     useEffect(() => {
       fetchInstallState().then((s) => {
         if (s === null) navigate('/login', { replace: true })
         else setState(s)
       }).finally(() => setLoading(false))
     }, [navigate])

     if (loading || !state) return null

     const reload = async () => {
       const s = await fetchInstallState()
       if (s === null) navigate('/login', { replace: true })
       else setState(s)
     }

     const idx = state.CurrentStep - 1

     return (
       <div className="mx-auto flex max-w-3xl flex-col gap-6 px-4 py-8 md:py-12">
         <div className="flex items-center gap-3">
           <img src={shifterLogo} alt="Shifter" className="h-6" />
           <div>
             <h1 className="text-2xl font-semibold leading-8">Set up Shifter</h1>
             <p className="text-sm text-muted-foreground">Five quick steps and you're ready to monitor.</p>
           </div>
         </div>
         <div className="rounded-lg border bg-card p-6 md:p-8">
           <Stepper steps={STEPS} currentIndex={idx} />
           <div className="mt-8">
             {state.CurrentStep === 1 && <AdminStep onAdvance={reload} />}
             {state.CurrentStep === 2 && <ChirpStackStep onAdvance={reload} />}
             {state.CurrentStep === 3 && <RegionStep onAdvance={reload} />}
             {state.CurrentStep === 4 && <IdentityStep onAdvance={reload} />}
             {state.CurrentStep === 5 && <ReviewStep state={state} onComplete={() => navigate('/login', { replace: true })} />}
           </div>
         </div>
       </div>
     )
   }
   ```

3. Create `web/src/routes/install/admin-step.tsx`:
   ```tsx
   import { useState } from 'react'
   import { Alert, AlertDescription } from '@/components/ui/alert'
   import { Button } from '@/components/ui/button'
   import { Input } from '@/components/ui/input'
   import { Label } from '@/components/ui/label'
   import { ApiError } from '@/lib/api'
   import { postStep1 } from '@/lib/install'

   export function AdminStep({ onAdvance }: { onAdvance: () => void }) {
     const [email, setEmail] = useState('')
     const [name, setName] = useState('')
     const [password, setPassword] = useState('')
     const [confirm, setConfirm] = useState('')
     const [error, setError] = useState<string | null>(null)
     const [busy, setBusy] = useState(false)

     const submit = async (e: React.FormEvent) => {
       e.preventDefault()
       setError(null)
       if (password !== confirm) { setError("Passwords don't match."); return }
       setBusy(true)
       try {
         await postStep1({ email, name, password })
         onAdvance()
       } catch (err) {
         if (err instanceof ApiError && err.status === 422) {
           setError('Password is too weak. Add length, mixed case, a number, and a symbol.')
         } else {
           setError('Something went wrong. Try again.')
         }
       } finally { setBusy(false) }
     }

     return (
       <form onSubmit={submit} className="flex flex-col gap-4">
         <div>
           <h2 className="text-2xl font-semibold leading-8">Create the admin account</h2>
           <p className="text-sm text-muted-foreground">This account has full control of Shifter. You can add more users later.</p>
         </div>
         {error ? <Alert variant="destructive"><AlertDescription>{error}</AlertDescription></Alert> : null}
         <div className="flex flex-col gap-2">
           <Label htmlFor="adm-email">Email address</Label>
           <Input id="adm-email" type="email" required value={email} onChange={(e) => setEmail(e.target.value)} />
         </div>
         <div className="flex flex-col gap-2">
           <Label htmlFor="adm-name">Your name</Label>
           <Input id="adm-name" required value={name} onChange={(e) => setName(e.target.value)} />
         </div>
         <div className="flex flex-col gap-2">
           <Label htmlFor="adm-pw">Password</Label>
           <Input id="adm-pw" type="password" required value={password} onChange={(e) => setPassword(e.target.value)} />
           <p className="text-sm text-muted-foreground">At least 12 characters with mixed case, a number, and a symbol.</p>
         </div>
         <div className="flex flex-col gap-2">
           <Label htmlFor="adm-confirm">Confirm password</Label>
           <Input id="adm-confirm" type="password" required value={confirm} onChange={(e) => setConfirm(e.target.value)} />
         </div>
         <div className="flex justify-end pt-2">
           <Button type="submit" disabled={busy}>{busy ? 'Saving…' : 'Next'}</Button>
         </div>
       </form>
     )
   }
   ```

4. Create `web/src/routes/install/chirpstack-step.tsx`:
   ```tsx
   import { useState } from 'react'
   import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
   import { Button } from '@/components/ui/button'
   import { Input } from '@/components/ui/input'
   import { Label } from '@/components/ui/label'
   import { ApiError } from '@/lib/api'
   import { postStep2 } from '@/lib/install'

   export function ChirpStackStep({ onAdvance }: { onAdvance: () => void }) {
     const [mode, setMode] = useState<'bundled' | 'external'>('bundled')
     const [grpcUrl, setGrpcUrl] = useState('chirpstack:8080')
     const [apiToken, setApiToken] = useState('')
     const [mqttUrl, setMqttUrl] = useState('tcp://mosquitto:1883')
     const [mqttUser, setMqttUser] = useState('')
     const [mqttPass, setMqttPass] = useState('')
     const [v3, setV3] = useState(false)
     const [error, setError] = useState<string | null>(null)
     const [busy, setBusy] = useState(false)

     const submit = async (e: React.FormEvent) => {
       e.preventDefault()
       setError(null); setV3(false); setBusy(true)
       try {
         await postStep2({ mode, grpc_url: grpcUrl, api_token: apiToken, mqtt_url: mqttUrl, mqtt_user: mqttUser || undefined, mqtt_password: mqttPass || undefined })
         onAdvance()
       } catch (err) {
         if (err instanceof ApiError && err.status === 422) {
           const body = err.body as { error?: string; detail?: string }
           if (body?.error === 'v3_detected') {
             setV3(true)
           } else if (body?.error === 'grpc_unreachable') {
             setError(`Couldn't reach ChirpStack at ${grpcUrl}: ${body.detail ?? 'unreachable'}`)
           } else {
             setError(body?.error ?? 'Something went wrong.')
           }
         } else {
           setError('Something went wrong. Try again.')
         }
       } finally { setBusy(false) }
     }

     return (
       <form onSubmit={submit} className="flex flex-col gap-4">
         <div>
           <h2 className="text-2xl font-semibold leading-8">Connect to ChirpStack</h2>
           <p className="text-sm text-muted-foreground">Shifter wraps your ChirpStack v4 server. Choose how you want it deployed.</p>
         </div>
         {v3 ? (
           <Alert variant="destructive">
             <AlertTitle>Shifter doesn't support ChirpStack v3</AlertTitle>
             <AlertDescription>We detected ChirpStack v3 at this URL. Shifter requires v4 or newer. Upgrade ChirpStack and try again.</AlertDescription>
           </Alert>
         ) : null}
         {error ? <Alert variant="destructive"><AlertDescription>{error}</AlertDescription></Alert> : null}
         <fieldset className="flex flex-col gap-2">
           <legend className="text-sm font-semibold">Mode</legend>
           <label className="flex items-center gap-2">
             <input type="radio" checked={mode === 'bundled'} onChange={() => setMode('bundled')} />
             Bundled — Shifter installs ChirpStack for you
           </label>
           <label className="flex items-center gap-2">
             <input type="radio" checked={mode === 'external'} onChange={() => setMode('external')} />
             External — connect to an existing ChirpStack
           </label>
         </fieldset>
         <div className="flex flex-col gap-2">
           <Label htmlFor="cs-grpc">gRPC URL</Label>
           <Input id="cs-grpc" required value={grpcUrl} onChange={(e) => setGrpcUrl(e.target.value)} className="font-mono" />
         </div>
         <div className="flex flex-col gap-2">
           <Label htmlFor="cs-token">API token</Label>
           <Input id="cs-token" type="password" required value={apiToken} onChange={(e) => setApiToken(e.target.value)} />
         </div>
         <div className="flex flex-col gap-2">
           <Label htmlFor="cs-mqtt">MQTT URL</Label>
           <Input id="cs-mqtt" required value={mqttUrl} onChange={(e) => setMqttUrl(e.target.value)} className="font-mono" />
         </div>
         <div className="grid grid-cols-2 gap-4">
           <div className="flex flex-col gap-2">
             <Label htmlFor="cs-mqtt-user">MQTT username (optional)</Label>
             <Input id="cs-mqtt-user" value={mqttUser} onChange={(e) => setMqttUser(e.target.value)} />
           </div>
           <div className="flex flex-col gap-2">
             <Label htmlFor="cs-mqtt-pass">MQTT password (optional)</Label>
             <Input id="cs-mqtt-pass" type="password" value={mqttPass} onChange={(e) => setMqttPass(e.target.value)} />
           </div>
         </div>
         <div className="flex justify-end pt-2">
           <Button type="submit" disabled={busy}>{busy ? 'Testing connection…' : 'Next'}</Button>
         </div>
       </form>
     )
   }
   ```

5. Create `web/src/routes/install/region-step.tsx`:
   ```tsx
   import { useState } from 'react'
   import { Button } from '@/components/ui/button'
   import { Label } from '@/components/ui/label'
   import { Select, SelectContent, SelectGroup, SelectItem, SelectLabel, SelectTrigger, SelectValue } from '@/components/ui/select'
   import { REGIONS, postStep3 } from '@/lib/install'

   export function RegionStep({ onAdvance }: { onAdvance: () => void }) {
     // PITFALLS §8: AS923-2 pre-selected for Thailand operator base.
     const [selected, setSelected] = useState<string>('as923_2')
     const [busy, setBusy] = useState(false)

     const groups = ['asia', 'europe', 'americas', 'oceania', 'india'] as const
     const submit = async (e: React.FormEvent) => {
       e.preventDefault()
       setBusy(true)
       try { await postStep3({ name: selected }); onAdvance() }
       finally { setBusy(false) }
     }

     return (
       <form onSubmit={submit} className="flex flex-col gap-4">
         <div>
           <h2 className="text-2xl font-semibold leading-8">Choose your LoRaWAN region</h2>
           <p className="text-sm text-muted-foreground">This sets the default frequency plan for new gateways. You can override per-gateway later.</p>
         </div>
         <div className="flex flex-col gap-2">
           <Label htmlFor="region">Region</Label>
           <Select value={selected} onValueChange={setSelected}>
             <SelectTrigger id="region"><SelectValue /></SelectTrigger>
             <SelectContent>
               {groups.map((g) => (
                 <SelectGroup key={g}>
                   <SelectLabel className="capitalize">{g}</SelectLabel>
                   {REGIONS.filter((r) => r.group === g).map((r) => (
                     <SelectItem key={r.name} value={r.name}>{r.display}</SelectItem>
                   ))}
                 </SelectGroup>
               ))}
             </SelectContent>
           </Select>
           {selected === 'as923_2' ? (
             <p className="text-sm text-muted-foreground">We pre-selected AS923-2 because the install address is in Thailand.</p>
           ) : null}
         </div>
         <div className="flex justify-end pt-2">
           <Button type="submit" disabled={busy}>{busy ? 'Saving…' : 'Next'}</Button>
         </div>
       </form>
     )
   }
   ```

6. Create `web/src/routes/install/identity-step.tsx`:
   ```tsx
   import { useState } from 'react'
   import { Button } from '@/components/ui/button'
   import { Input } from '@/components/ui/input'
   import { Label } from '@/components/ui/label'
   import { postStep4 } from '@/lib/install'

   export function IdentityStep({ onAdvance }: { onAdvance: () => void }) {
     const [displayName, setDisplayName] = useState('')
     const [address, setAddress] = useState('')
     // System default timezone heuristic for the operator.
     const [timezone, setTimezone] = useState(Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC')
     const [units, setUnits] = useState<'metric' | 'imperial'>('metric')
     const [busy, setBusy] = useState(false)

     const submit = async (e: React.FormEvent) => {
       e.preventDefault()
       setBusy(true)
       try { await postStep4({ display_name: displayName, address: address || undefined, timezone, units }); onAdvance() }
       finally { setBusy(false) }
     }

     return (
       <form onSubmit={submit} className="flex flex-col gap-4">
         <div>
           <h2 className="text-2xl font-semibold leading-8">Tell us about your install</h2>
           <p className="text-sm text-muted-foreground">This appears in the topbar and on every report you export.</p>
         </div>
         <div className="flex flex-col gap-2">
           <Label htmlFor="disp">Display name</Label>
           <Input id="disp" required value={displayName} onChange={(e) => setDisplayName(e.target.value)} />
           <p className="text-sm text-muted-foreground">What your team calls this site, e.g. "Acme Water Co."</p>
         </div>
         <div className="flex flex-col gap-2">
           <Label htmlFor="addr">Address (optional)</Label>
           <Input id="addr" value={address} onChange={(e) => setAddress(e.target.value)} />
         </div>
         <div className="flex flex-col gap-2">
           <Label htmlFor="tz">Timezone</Label>
           <Input id="tz" required value={timezone} onChange={(e) => setTimezone(e.target.value)} className="font-mono" />
         </div>
         <fieldset className="flex flex-col gap-2">
           <legend className="text-sm font-semibold">Units</legend>
           <label className="flex items-center gap-2">
             <input type="radio" checked={units === 'metric'} onChange={() => setUnits('metric')} /> Metric
           </label>
           <label className="flex items-center gap-2">
             <input type="radio" checked={units === 'imperial'} onChange={() => setUnits('imperial')} /> Imperial
           </label>
         </fieldset>
         <div className="flex justify-end pt-2">
           <Button type="submit" disabled={busy}>{busy ? 'Saving…' : 'Next'}</Button>
         </div>
       </form>
     )
   }
   ```

7. Create `web/src/routes/install/review-step.tsx`:
   ```tsx
   import { useState } from 'react'
   import { Alert, AlertDescription } from '@/components/ui/alert'
   import { Button } from '@/components/ui/button'
   import { ApiError } from '@/lib/api'
   import { postFinish, type InstallState } from '@/lib/install'

   export function ReviewStep({ state, onComplete }: { state: InstallState; onComplete: () => void }) {
     const [busy, setBusy] = useState(false)
     const [error, setError] = useState<string | null>(null)

     const finish = async () => {
       setBusy(true); setError(null)
       try {
         await postFinish()
         onComplete()
       } catch (err) {
         if (err instanceof ApiError) {
           setError(`Couldn't finish: ${err.message}`)
         } else {
           setError('Something went wrong. Try again.')
         }
       } finally { setBusy(false) }
     }

     // Render JSON drafts in a compact, readable form.
     const fmt = (v: unknown) => v ? JSON.stringify(v, null, 2) : '(empty)'
     return (
       <div className="flex flex-col gap-4">
         <div>
           <h2 className="text-2xl font-semibold leading-8">Review and finish</h2>
           <p className="text-sm text-muted-foreground">Confirm everything looks right. You can change any of this in Settings later.</p>
         </div>
         {error ? <Alert variant="destructive"><AlertDescription>{error}</AlertDescription></Alert> : null}
         <pre className="bg-muted rounded-md p-4 font-mono text-xs overflow-auto max-h-96">
           {`Admin:        ${fmt(state.Step1Admin)}
   ChirpStack:   ${fmt(state.Step2ChirpStack)}
   Region:       ${fmt(state.Step3Region)}
   Identity:     ${fmt(state.Step4Identity)}`}
         </pre>
         <div className="flex justify-end pt-2">
           <Button onClick={finish} disabled={busy}>{busy ? 'Finishing setup…' : 'Finish setup'}</Button>
         </div>
       </div>
     )
   }
   ```

8. Update `web/src/App.tsx` to mount the wizard. Replace the `/install` placeholder with a lazy import:
   ```tsx
   import { lazy, Suspense } from 'react'
   const InstallWizard = lazy(() => import('@/routes/install'))
   // In the auth-layout children:
   { path: '/install', element: <Suspense fallback={null}><InstallWizard /></Suspense> },
   ```
  </action>
  <verify>
    <automated>cd web && pnpm build && pnpm test:run -- -t 'install|region'</automated>
  </verify>
  <acceptance_criteria>
    - File `web/src/lib/install.ts` exports `fetchInstallState`, `postStep1..4`, `postFinish`, `REGIONS`
    - `REGIONS` array has exactly 8 entries; the entry with `name: 'as923_2'` has `default_for_country: 'TH'`
    - File `web/src/routes/install/index.tsx` renders `Stepper` from `@/components/stepper` and switches on `state.CurrentStep` between 1-5
    - File `web/src/routes/install/admin-step.tsx` form has fields with ids `adm-email`, `adm-name`, `adm-pw`, `adm-confirm` (Inter font hint via UI-SPEC enforced in Plan 06's index.css)
    - File `web/src/routes/install/admin-step.tsx` strength helper text is exactly "At least 12 characters with mixed case, a number, and a symbol." (UI-SPEC verbatim)
    - File `web/src/routes/install/chirpstack-step.tsx` v3 alert title is exactly "Shifter doesn't support ChirpStack v3"
    - File `web/src/routes/install/chirpstack-step.tsx` v3 alert body is exactly "We detected ChirpStack v3 at this URL. Shifter requires v4 or newer. Upgrade ChirpStack and try again."
    - File `web/src/routes/install/region-step.tsx` initial state is `'as923_2'` (default selection)
    - File `web/src/routes/install/region-step.tsx` shows "We pre-selected AS923-2 because the install address is in Thailand." when selected==='as923_2'
    - File `web/src/routes/install/review-step.tsx` Finish button label idle is "Finish setup", loading is "Finishing setup…"
    - Command `cd web && pnpm build` exits 0
  </acceptance_criteria>
  <done>
    Wizard frontend builds. Plan 23 (login) is the navigate target after finish. Plan 17 settings reuses ChirpStack form patterns.
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Region step test (TestStep3 frontend equivalent)</name>
  <files>web/src/routes/install/region-step.test.tsx</files>
  <read_first>
    - 01-02-test-harness-PLAN.md (existing describe.skip stub)
    - .planning/phases/01-foundation/01-VALIDATION.md (TestStep3 frontend test name)
  </read_first>
  <behavior>
    - On mount, RegionStep's hidden Select value is 'as923_2'.
    - The Thailand helper text is rendered when 'as923_2' is selected.
    - Choosing a different region hides the helper text.
  </behavior>
  <action>
1. Replace `web/src/routes/install/region-step.test.tsx`:
   ```tsx
   import { render, screen } from '@testing-library/react'
   import userEvent from '@testing-library/user-event'
   import { describe, expect, it, vi } from 'vitest'
   import { RegionStep } from './region-step'

   // The Select uses Radix Portal; we mock postStep3 to avoid network in tests.
   vi.mock('@/lib/install', async (orig) => {
     const real = await orig<typeof import('@/lib/install')>()
     return { ...real, postStep3: vi.fn().mockResolvedValue({ current_step: 4 }) }
   })

   describe('RegionStep (Plan 16)', () => {
     it('defaults AS923-2 for Thailand and shows the pre-selection hint', () => {
       render(<RegionStep onAdvance={() => {}} />)
       expect(screen.getByText(/We pre-selected AS923-2/)).toBeInTheDocument()
     })

     it('renders Next button labeled "Next" idle', () => {
       render(<RegionStep onAdvance={() => {}} />)
       expect(screen.getByRole('button', { name: /next/i })).toBeInTheDocument()
     })

     it('Next click invokes postStep3 with as923_2', async () => {
       const onAdvance = vi.fn()
       const lib = await import('@/lib/install')
       render(<RegionStep onAdvance={onAdvance} />)
       await userEvent.click(screen.getByRole('button', { name: /next/i }))
       expect(lib.postStep3).toHaveBeenCalledWith({ name: 'as923_2' })
       expect(onAdvance).toHaveBeenCalled()
     })
   })
   ```
  </action>
  <verify>
    <automated>cd web && pnpm test:run -- region-step</automated>
  </verify>
  <acceptance_criteria>
    - File `web/src/routes/install/region-step.test.tsx` no longer uses `describe.skip` (uses `describe(...)`)
    - 3 tests pass: default selection, button label, postStep3 invocation
    - Test mocks `@/lib/install`'s `postStep3` so no network is required
    - Command `cd web && pnpm test:run -- region-step` exits 0 (per VALIDATION.md TestStep3 frontend equivalent)
  </acceptance_criteria>
  <done>
    INST-04 frontend test green. PITFALLS §8 mitigation visible (Thailand pre-selected by default).
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| pre-install browser → wizard | Fully untrusted; CSRF mitigation via X-Requested-With (Plan 15) |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-16-01 | Tampering (XSS) | install identity display name re-rendered on review | mitigate | React auto-escapes; review step uses `<pre>{JSON.stringify(...)}</pre>` so embedded HTML is not interpreted. ASVS V5. |
| T-16-02 | Information Disclosure | review step renders the password hash from step 1 in JSON | mitigate (residual accept) | Step 1 stores `password_hash`, never plaintext. The hash is non-reversible. Operators see Argon2id PHC string in review — acceptable. ASVS V8. |
| T-16-03 | Tampering | step 4 timezone could be a malicious string | mitigate | Server-side `time.LoadLocation` validation (Plan 15); 422 on invalid. ASVS V5. |
| T-16-04 | Spoofing | wizard shown after install completes | mitigate | Plan 15 GET /api/install/state returns 410; index.tsx redirects to /login. |
| T-16-05 | Information Disclosure | api_token visible in browser dev-tools network tab | accept | Self-hosted single-tenant; operator types the token themselves. Server stores by REF. ASVS V8. |
</threat_model>

<verification>
- `web/src/routes/install/*` — 5 step components + index.tsx wizard shell
- `lib/install.ts` typed client
- AS923-2 pre-selected (PITFALLS §8 frontend)
- v3 alert UI-SPEC verbatim copy
- 3 region-step tests pass
- `pnpm build` exits 0
</verification>

<success_criteria>
- INST-01..05 frontend complete
- UX-01: wizard is the canonical full-screen stepped exception (UI-SPEC §"Install wizard" line 237)
- UI-SPEC dialog anatomy (Cancel/primary positioning, copy strings) followed
- AS923-2 default visible per PITFALLS §8
- Finish navigates to /login
- All UI-SPEC verbatim copy strings present
</success_criteria>

<output>
After completion, create `.planning/phases/01-foundation/01-16-SUMMARY.md` documenting:
- Wizard route structure
- install.ts client API
- Verbatim copy strings used
- Plan 23 login navigate target
</output>
