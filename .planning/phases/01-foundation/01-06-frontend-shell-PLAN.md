---
phase: 01-foundation
plan: 06
type: execute
wave: 3
depends_on: [01, 02]
files_modified:
  - web/package.json
  - web/pnpm-lock.yaml
  - web/components.json
  - web/src/index.css
  - web/src/theme.css
  - web/src/lib/utils.ts
  - web/src/components/ui/button.tsx
  - web/src/components/ui/input.tsx
  - web/src/components/ui/label.tsx
  - web/src/components/ui/form.tsx
  - web/src/components/ui/card.tsx
  - web/src/components/ui/dialog.tsx
  - web/src/components/ui/alert.tsx
  - web/src/components/ui/alert-dialog.tsx
  - web/src/components/ui/dropdown-menu.tsx
  - web/src/components/ui/avatar.tsx
  - web/src/components/ui/select.tsx
  - web/src/components/ui/checkbox.tsx
  - web/src/components/ui/separator.tsx
  - web/src/components/ui/skeleton.tsx
  - web/src/components/ui/sonner.tsx
  - web/src/components/ui/tabs.tsx
  - web/src/components/ui/progress.tsx
  - web/src/components/ui/badge.tsx
  - web/src/components/ui/tooltip.tsx
  - web/src/components/ui/sheet.tsx
  - web/src/components/responsive-dialog.tsx
  - web/src/components/responsive-dialog.test.tsx
  - web/src/components/status-row.tsx
  - web/src/components/status-row.test.tsx
  - web/src/components/stepper.tsx
  - web/src/components/theme-provider.tsx
  - web/src/components/theme-provider.test.tsx
  - web/src/components/shell/topbar.tsx
  - web/src/components/shell/sidebar.tsx
  - web/src/components/shell/account-menu.tsx
  - web/src/components/shell/responsive-shell.tsx
  - web/src/lib/api.ts
  - web/src/lib/query-client.ts
  - web/src/App.tsx
  - web/src/main.tsx
  - web/src/routes/_root.tsx
  - web/src/routes/_auth.tsx
  - web/src/routes/index-redirect.tsx
  - web/src/assets/shifter-logo.svg
  - web/src/assets/shifter-mark.svg
autonomous: true
requirements:
  - UX-02
must_haves:
  truths:
    - "shadcn init has run; components.json says style=new-york, baseColor=slate, cssVariables=true"
    - "21 shadcn components are installed under web/src/components/ui/"
    - "ResponsiveDialog renders Dialog on md+ and Sheet[side=bottom] on <md (UI-SPEC §Dialog Conventions)"
    - "StatusRow renders three states: reachable (green), unreachable (red), skipped (muted) — three icons + monospace detail"
    - "ThemeProvider supports light/dark/system, persists choice in localStorage, applies class='dark' to root"
    - "Inter (variable, weights 400+600) and JetBrains Mono fonts are self-hosted via @fontsource (UI-SPEC)"
    - "OKLCH navy primary token resolves to oklch(0.42 0.17 255) in light mode (UI-SPEC §Color)"
    - "react-router-dom v7 router mounted in App.tsx with /, /login, /install, /settings stubs"
    - "@tanstack/react-query QueryClientProvider wraps app"
  artifacts:
    - path: "web/components.json"
      provides: "shadcn config: style=new-york, baseColor=slate, cssVariables=true"
      contains: "new-york"
    - path: "web/src/theme.css"
      provides: "OKLCH navy primary + custom success/warning/info tokens (UI-SPEC §Color)"
      contains: "oklch(0.42 0.17 255)"
    - path: "web/src/components/responsive-dialog.tsx"
      provides: "Dialog↔Sheet swap on md breakpoint (UI-SPEC §Dialog Conventions)"
      contains: "ResponsiveDialog"
    - path: "web/src/components/status-row.tsx"
      provides: "Status dot/icon + label + monospace detail (UI-SPEC pattern)"
      contains: "StatusRow"
    - path: "web/src/components/theme-provider.tsx"
      provides: "Light/dark/system theme persistence (UI-SPEC §Color)"
      contains: "ThemeProvider"
    - path: "web/src/lib/api.ts"
      provides: "Typed fetch wrapper with X-Requested-With CSRF header + 401 redirect"
      contains: "X-Requested-With"
    - path: "web/src/lib/query-client.ts"
      provides: "TanStack Query QueryClient with sane defaults"
      contains: "QueryClient"
    - path: "web/src/App.tsx"
      provides: "Router + QueryClientProvider + ThemeProvider + Toaster wrapping"
      contains: "RouterProvider"
  key_links:
    - from: "web/src/App.tsx"
      to: "web/src/routes/_root.tsx + _auth.tsx"
      via: "createBrowserRouter children"
      pattern: "createBrowserRouter"
    - from: "web/src/lib/api.ts"
      to: "/api/*"
      via: "fetch wrapper sending Cookie automatically"
      pattern: "X-Requested-With"
    - from: "web/src/components/responsive-dialog.tsx"
      to: "shadcn Dialog + Sheet"
      via: "useMediaQuery picks Dialog on md+, Sheet on <md"
      pattern: "min-width.*768"
---

<objective>
Bootstrap the React + shadcn/ui frontend shell: run `shadcn init`, install all 21 shadcn components needed across Phase 1, lock in the OKLCH navy theme (UI-SPEC §Color) via a separate `theme.css` (PITFALL #12), self-host Inter + JetBrains Mono fonts via `@fontsource`, install the four foundational reusable components (`ResponsiveDialog`, `StatusRow`, `Stepper`, `ThemeProvider`), wire the topbar+sidebar app shell, mount react-router-dom v7, install TanStack Query and the typed API fetch wrapper. The login screen, install wizard, and settings page (Plans 11/16/17/23) all fill into this shell.

Purpose: UX-02 (shadcn navy palette, English-only, modern minimal). UI-SPEC propagates to every later phase. Without this plan, every later UI plan would re-decide the design system.

Output: `pnpm dev` shows a placeholder app with the navy theme applied; theme toggle works; `pnpm test --run` passes including new tests for `ResponsiveDialog`, `StatusRow`, `ThemeProvider`.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/01-foundation/01-CONTEXT.md
@.planning/phases/01-foundation/01-RESEARCH.md
@.planning/phases/01-foundation/01-UI-SPEC.md
@01-01-repo-scaffold-PLAN.md
@01-02-test-harness-PLAN.md

<interfaces>
UI-SPEC §Color (lines 144-189) — the canonical OKLCH palette. The verbatim CSS lives in RESEARCH §"shadcn theme override" (lines 1480-1551).

shadcn 21-component inventory (UI-SPEC §"shadcn components required for Phase 1"):
button, input, label, form, card, dialog, alert, alert-dialog, dropdown-menu, avatar, select, checkbox, separator, skeleton, sonner, tabs, progress, badge, tooltip, sheet

ResponsiveDialog signature:
```tsx
interface ResponsiveDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description?: string
  children: React.ReactNode
  footer?: React.ReactNode
}
```

StatusRow signature:
```tsx
interface StatusRowProps {
  status: 'reachable' | 'unreachable' | 'skipped'
  label: string             // "gRPC", "MQTT"
  detail?: string           // "47 ms", "connect: connection refused"
}
```

Theme tokens (web/src/theme.css) verbatim from RESEARCH lines 1480-1551 — do NOT modify values.

Router shape (RESEARCH §"React Router v7 protected route pattern" lines 1556-1596):
```tsx
const router = createBrowserRouter([
  { path: '/install', lazy: () => import('./routes/install') },     // Plan 16
  { path: '/login',   lazy: () => import('./routes/login') },       // Plan 23
  {
    path: '/',
    element: <RootLayout />,                                         // Plan 06 (this plan)
    loader: rootLoader,                                              // first-run + auth gate (Plan 14 + Plan 11)
    children: [
      { index: true, element: <IndexRedirect /> },
      { path: 'settings', lazy: () => import('./routes/settings') }, // Plan 17
    ],
  },
])
```
For Plan 06, route loaders return null so the router compiles. Plan 11 fills the auth loader and Plan 14 fills the install loader.
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 1: shadcn init + 21 components + theme.css + fonts + utility deps</name>
  <files>web/package.json, web/pnpm-lock.yaml, web/components.json, web/src/index.css, web/src/theme.css, web/src/lib/utils.ts, web/src/components/ui/button.tsx, web/src/components/ui/input.tsx, web/src/components/ui/label.tsx, web/src/components/ui/form.tsx, web/src/components/ui/card.tsx, web/src/components/ui/dialog.tsx, web/src/components/ui/alert.tsx, web/src/components/ui/alert-dialog.tsx, web/src/components/ui/dropdown-menu.tsx, web/src/components/ui/avatar.tsx, web/src/components/ui/select.tsx, web/src/components/ui/checkbox.tsx, web/src/components/ui/separator.tsx, web/src/components/ui/skeleton.tsx, web/src/components/ui/sonner.tsx, web/src/components/ui/tabs.tsx, web/src/components/ui/progress.tsx, web/src/components/ui/badge.tsx, web/src/components/ui/tooltip.tsx, web/src/components/ui/sheet.tsx, web/src/assets/shifter-logo.svg, web/src/assets/shifter-mark.svg</files>
  <read_first>
    - .planning/phases/01-foundation/01-UI-SPEC.md §"Design System" (lines 39-79) — style=new-york, base=slate, cssVariables=yes
    - .planning/phases/01-foundation/01-UI-SPEC.md §"Color" (lines 142-215) — exact OKLCH values
    - .planning/phases/01-foundation/01-RESEARCH.md §"shadcn theme override (UI-SPEC §Color)" (lines 1480-1552)
    - .planning/phases/01-foundation/01-RESEARCH.md §"Pitfall 12: shadcn init overwrites palette" (lines 1379-1383) — keep custom tokens in theme.css, not index.css
  </read_first>
  <action>
1. Run shadcn init from the `web/` directory. Use exactly:
   ```bash
   cd web
   pnpm dlx shadcn@latest init
   ```
   When prompted answer:
   - Style: **new-york**
   - Base color: **slate**
   - CSS variables: **yes**
   - Path aliases: `@/components` → `src/components`, `@/lib/utils` → `src/lib/utils`, `@/hooks` → `src/hooks`
   This creates `web/components.json`, `web/src/lib/utils.ts`, and rewrites `web/src/index.css` with the slate preset.

2. Install the 21 shadcn components (UI-SPEC §"shadcn components required for Phase 1"):
   ```bash
   pnpm dlx shadcn@latest add button input label form card dialog alert alert-dialog dropdown-menu avatar select checkbox separator skeleton sonner tabs progress badge tooltip sheet
   ```
   This populates `web/src/components/ui/*.tsx`. Each file is owned by shadcn; biome.json already excludes this directory from formatting.

3. Install supporting deps for the shell (UI-SPEC + STACK.md):
   ```bash
   pnpm add lucide-react react-router-dom@^7 @tanstack/react-query react-hook-form @hookform/resolvers zod date-fns sonner
   pnpm add @fontsource-variable/inter @fontsource/jetbrains-mono
   ```

4. PITFALL #12 prevention — separate the custom theme tokens from shadcn's regenerable `index.css`. After shadcn init, the `index.css` looks like:
   ```css
   @import "tailwindcss";
   @import "tw-animate-css";

   @custom-variant dark (&:is(.dark *));

   :root { --background: ...; --primary: ...; /* slate preset */ }
   .dark { ... }
   @theme inline { ... }
   ```
   Replace `web/src/index.css` with EXACTLY:
   ```css
   @import "tailwindcss";
   @import "tw-animate-css";
   @import "@fontsource-variable/inter/index.css";
   @import "@fontsource/jetbrains-mono/400.css";
   @import "@fontsource/jetbrains-mono/600.css";
   @import "./theme.css";

   @custom-variant dark (&:is(.dark *));

   @layer base {
     html, body, #root { height: 100%; }
     body {
       font-family: 'Inter Variable', system-ui, -apple-system, sans-serif;
       font-feature-settings: 'cv02', 'cv03', 'cv04', 'cv11';
       background: var(--background);
       color: var(--foreground);
     }
     code, .font-mono {
       font-family: 'JetBrains Mono', ui-monospace, monospace;
     }
   }
   ```

5. Create `web/src/theme.css` — VERBATIM from RESEARCH §"shadcn theme override" (lines 1480-1551). This locks the navy primary OKLCH values:
   ```css
   @layer base {
     :root {
       --background: oklch(1 0 0);
       --foreground: oklch(0.18 0.025 240);
       --card: oklch(0.985 0.005 240);
       --card-foreground: oklch(0.18 0.025 240);
       --popover: oklch(1 0 0);
       --popover-foreground: oklch(0.18 0.025 240);
       --primary: oklch(0.42 0.17 255);
       --primary-foreground: oklch(0.98 0 0);
       --secondary: oklch(0.96 0.005 240);
       --secondary-foreground: oklch(0.18 0.025 240);
       --muted: oklch(0.96 0.005 240);
       --muted-foreground: oklch(0.45 0.020 240);
       --accent: oklch(0.96 0.005 240);
       --accent-foreground: oklch(0.18 0.025 240);
       --destructive: oklch(0.55 0.22 27);
       --destructive-foreground: oklch(0.98 0 0);
       --success: oklch(0.62 0.18 145);
       --success-foreground: oklch(0.98 0 0);
       --warning: oklch(0.75 0.16 70);
       --warning-foreground: oklch(0.18 0.025 240);
       --info: oklch(0.55 0.15 240);
       --info-foreground: oklch(0.98 0 0);
       --border: oklch(0.92 0.008 240);
       --input: oklch(0.92 0.008 240);
       --ring: oklch(0.42 0.17 255 / 0.4);
       --radius: 0.5rem;
     }
     .dark {
       --background: oklch(0.18 0.025 240);
       --foreground: oklch(0.97 0.005 240);
       --card: oklch(0.22 0.025 240);
       --card-foreground: oklch(0.97 0.005 240);
       --popover: oklch(0.22 0.025 240);
       --popover-foreground: oklch(0.97 0.005 240);
       --primary: oklch(0.65 0.18 250);
       --primary-foreground: oklch(0.18 0.025 240);
       --secondary: oklch(0.30 0.020 240);
       --secondary-foreground: oklch(0.97 0.005 240);
       --muted: oklch(0.30 0.020 240);
       --muted-foreground: oklch(0.65 0.015 240);
       --accent: oklch(0.30 0.020 240);
       --accent-foreground: oklch(0.97 0.005 240);
       --destructive: oklch(0.62 0.22 27);
       --destructive-foreground: oklch(0.97 0.005 240);
       --success: oklch(0.68 0.18 145);
       --success-foreground: oklch(0.18 0.025 240);
       --warning: oklch(0.78 0.16 70);
       --warning-foreground: oklch(0.18 0.025 240);
       --info: oklch(0.65 0.15 235);
       --info-foreground: oklch(0.18 0.025 240);
       --border: oklch(0.30 0.020 240);
       --input: oklch(0.30 0.020 240);
       --ring: oklch(0.65 0.18 250 / 0.5);
     }
   }
   @theme inline {
     --color-success: var(--success);
     --color-success-foreground: var(--success-foreground);
     --color-warning: var(--warning);
     --color-warning-foreground: var(--warning-foreground);
     --color-info: var(--info);
     --color-info-foreground: var(--info-foreground);
   }
   ```

6. Create placeholder logo assets (UI-SPEC §"Logo & Brand Placement" mandates these paths):
   - `web/src/assets/shifter-logo.svg`: a simple SVG with the text "Shifter" in Inter 600, 24px, navy fill `#1E40AF`. (Sample SVG of width 120 × height 24 with `<text>Shifter</text>` is sufficient — design polish in Plan 24.)
   - `web/src/assets/shifter-mark.svg`: a simple SVG mark (e.g., a small circular icon with "S"), navy fill, 32×32.

7. Verify shadcn install:
   - `web/components.json` exists with `"style": "new-york"`, `"baseColor": "slate"`, `"cssVariables": true`
   - `web/src/components/ui/` contains 21 `.tsx` files (one per component listed)
   - `pnpm build` exits 0
  </action>
  <verify>
    <automated>cd web && test -f components.json && grep -q '"style": "new-york"' components.json && grep -q '"baseColor": "slate"' components.json && ls src/components/ui | grep -E '(button|input|dialog|sheet|tooltip)\.tsx' | wc -l | grep -q '5' && grep -q 'oklch(0.42 0.17 255)' src/theme.css && pnpm build</automated>
  </verify>
  <acceptance_criteria>
    - File `web/components.json` exists and contains `"style": "new-york"`, `"baseColor": "slate"`, `"cssVariables": true`
    - Directory `web/src/components/ui/` contains exactly 21 files: `button.tsx`, `input.tsx`, `label.tsx`, `form.tsx`, `card.tsx`, `dialog.tsx`, `alert.tsx`, `alert-dialog.tsx`, `dropdown-menu.tsx`, `avatar.tsx`, `select.tsx`, `checkbox.tsx`, `separator.tsx`, `skeleton.tsx`, `sonner.tsx`, `tabs.tsx`, `progress.tsx`, `badge.tsx`, `tooltip.tsx`, `sheet.tsx` (and `index.css` may be present from shadcn)
    - File `web/src/theme.css` contains the literal string `oklch(0.42 0.17 255)` (light-mode primary navy)
    - File `web/src/theme.css` contains the literal string `--success: oklch(0.62 0.18 145)` (custom success token)
    - File `web/src/index.css` imports `@fontsource-variable/inter/index.css` AND `@fontsource/jetbrains-mono/400.css`
    - `web/package.json` dependencies include: `react-router-dom@^7`, `@tanstack/react-query`, `react-hook-form`, `@hookform/resolvers`, `zod`, `lucide-react`, `sonner`, `@fontsource-variable/inter`, `@fontsource/jetbrains-mono`
    - File `web/src/assets/shifter-logo.svg` exists and contains an SVG `<text>` element with "Shifter"
    - File `web/src/assets/shifter-mark.svg` exists
    - Command `cd web && pnpm build` exits 0
  </acceptance_criteria>
  <done>
    shadcn initialized with new-york + slate + custom navy OKLCH theme. 21 components installed. Inter + JetBrains Mono self-hosted. Plans 11/16/17/23 build their UI on top of this.
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Foundational components — ResponsiveDialog, StatusRow, Stepper, ThemeProvider — with passing tests</name>
  <files>web/src/components/responsive-dialog.tsx, web/src/components/responsive-dialog.test.tsx, web/src/components/status-row.tsx, web/src/components/status-row.test.tsx, web/src/components/stepper.tsx, web/src/components/theme-provider.tsx, web/src/components/theme-provider.test.tsx</files>
  <read_first>
    - .planning/phases/01-foundation/01-UI-SPEC.md §"Dialog Conventions" (lines 301-377) — exact pattern; mobile = Sheet[side="bottom"]
    - .planning/phases/01-foundation/01-UI-SPEC.md §"State Conventions" §"Test Connection result UI" (lines 444-464) — StatusRow visual contract
    - .planning/phases/01-foundation/01-UI-SPEC.md §"Stepped dialog pattern" (lines 369-377) — Stepper visual
    - .planning/phases/01-foundation/01-UI-SPEC.md §"Iconography" (lines 469-498) — exact lucide icons (`CheckCircle2`, `XCircle`, `MinusCircle`, `Sun`, `Moon`, `Laptop`)
    - 01-02-test-harness-PLAN.md (existing skip-stub describe.skip; replace with describe + tests)
  </read_first>
  <behavior>
    - **ResponsiveDialog tests:**
      - renders Dialog when window width ≥ 768
      - renders Sheet[side='bottom'] when window width < 768
      - title and description render in both modes
    - **StatusRow tests:**
      - status='reachable' renders CheckCircle2 with class containing 'text-success'
      - status='unreachable' renders XCircle with class containing 'text-destructive'
      - status='skipped' renders MinusCircle with class containing 'text-muted-foreground'
      - detail prop renders inside a `<span class="font-mono">` element
    - **ThemeProvider tests:**
      - default theme is 'system'
      - setting theme='dark' adds class 'dark' to document.documentElement
      - setting theme='light' removes class 'dark'
      - choice persists in localStorage under key 'shifter-theme'
  </behavior>
  <action>
1. Create `web/src/components/responsive-dialog.tsx`:
   ```tsx
   import { useEffect, useState } from 'react'
   import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
   import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from '@/components/ui/sheet'

   function useIsMobile(breakpoint = 768) {
     const [isMobile, setIsMobile] = useState(() =>
       typeof window !== 'undefined' ? window.innerWidth < breakpoint : false,
     )
     useEffect(() => {
       const mql = window.matchMedia(`(max-width: ${breakpoint - 1}px)`)
       const onChange = () => setIsMobile(mql.matches)
       mql.addEventListener('change', onChange)
       setIsMobile(mql.matches)
       return () => mql.removeEventListener('change', onChange)
     }, [breakpoint])
     return isMobile
   }

   export interface ResponsiveDialogProps {
     open: boolean
     onOpenChange: (open: boolean) => void
     title: string
     description?: string
     children: React.ReactNode
     footer?: React.ReactNode
   }

   /**
    * UI-SPEC §Dialog Conventions: dialogs use shadcn <Dialog> on md+ and <Sheet side="bottom"> on <md.
    * This wrapper is mandatory — every CRUD surface in Phase 1+ uses ResponsiveDialog, not raw Dialog.
    */
   export function ResponsiveDialog({ open, onOpenChange, title, description, children, footer }: ResponsiveDialogProps) {
     const isMobile = useIsMobile()
     if (isMobile) {
       return (
         <Sheet open={open} onOpenChange={onOpenChange}>
           <SheetContent side="bottom" className="px-6 py-4">
             <SheetHeader>
               <SheetTitle>{title}</SheetTitle>
               {description ? <SheetDescription>{description}</SheetDescription> : null}
             </SheetHeader>
             <div className="py-4">{children}</div>
             {footer ? <div className="border-t pt-4">{footer}</div> : null}
           </SheetContent>
         </Sheet>
       )
     }
     return (
       <Dialog open={open} onOpenChange={onOpenChange}>
         <DialogContent className="sm:max-w-lg px-6 py-5">
           <DialogHeader>
             <DialogTitle>{title}</DialogTitle>
             {description ? <DialogDescription>{description}</DialogDescription> : null}
           </DialogHeader>
           <div className="py-4">{children}</div>
           {footer ? <div className="flex justify-end gap-2 border-t pt-4">{footer}</div> : null}
         </DialogContent>
       </Dialog>
     )
   }
   ```

2. Create `web/src/components/status-row.tsx`:
   ```tsx
   import { CheckCircle2, MinusCircle, XCircle } from 'lucide-react'

   export type StatusRowVariant = 'reachable' | 'unreachable' | 'skipped'

   export interface StatusRowProps {
     status: StatusRowVariant
     label: string
     detail?: string
   }

   /**
    * UI-SPEC pattern: status dot/icon + label + monospace detail.
    * Used by Test Connection (Plan 17) and inherited by Phase 4 SSE health, Phase 6 alert center / backup status.
    */
   export function StatusRow({ status, label, detail }: StatusRowProps) {
     const Icon = status === 'reachable' ? CheckCircle2 : status === 'unreachable' ? XCircle : MinusCircle
     const color =
       status === 'reachable' ? 'text-success' :
       status === 'unreachable' ? 'text-destructive' : 'text-muted-foreground'
     return (
       <div className="flex items-center gap-3 py-2">
         <Icon className={`h-5 w-5 ${color}`} aria-hidden="true" />
         <span className="text-sm font-semibold w-16">{label}</span>
         <span className={`text-sm ${color}`}>
           {status === 'reachable' ? 'Reachable' : status === 'unreachable' ? 'Unreachable' : 'Skipped'}
         </span>
         {detail ? <span className="text-sm font-mono text-muted-foreground ml-auto">{detail}</span> : null}
       </div>
     )
   }
   ```

3. Create `web/src/components/stepper.tsx`:
   ```tsx
   import { Check } from 'lucide-react'

   export interface StepperProps {
     steps: { label: string }[]
     currentIndex: number   // 0-based; values < currentIndex are complete, equal is active, greater is future
   }

   /**
    * UI-SPEC §Stepped dialog pattern: numbered horizontal step list with checkmarks for completed.
    * Phase 1 install wizard uses this; Phase 2 add-device, Phase 5 floor plan upload, Phase 7 codec runner inherit.
    */
   export function Stepper({ steps, currentIndex }: StepperProps) {
     return (
       <ol className="flex items-center gap-4">
         {steps.map((step, idx) => {
           const isComplete = idx < currentIndex
           const isActive = idx === currentIndex
           const dotClass = isComplete
             ? 'bg-success text-success-foreground'
             : isActive
               ? 'bg-primary text-primary-foreground'
               : 'bg-secondary text-muted-foreground'
           return (
             <li key={step.label} className="flex items-center gap-2">
               <span className={`flex h-7 w-7 items-center justify-center rounded-full text-sm font-semibold ${dotClass}`}>
                 {isComplete ? <Check className="h-4 w-4" aria-hidden="true" /> : idx + 1}
               </span>
               <span className={`text-sm ${isActive ? 'font-semibold' : 'text-muted-foreground'}`}>{step.label}</span>
             </li>
           )
         })}
       </ol>
     )
   }
   ```

4. Create `web/src/components/theme-provider.tsx`:
   ```tsx
   import { createContext, useContext, useEffect, useState } from 'react'

   type Theme = 'light' | 'dark' | 'system'
   const STORAGE_KEY = 'shifter-theme'

   interface ThemeContextValue {
     theme: Theme
     setTheme: (theme: Theme) => void
   }

   const ThemeContext = createContext<ThemeContextValue | undefined>(undefined)

   function getSystemPref(): 'light' | 'dark' {
     return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
   }

   function applyTheme(theme: Theme) {
     const resolved = theme === 'system' ? getSystemPref() : theme
     if (resolved === 'dark') document.documentElement.classList.add('dark')
     else document.documentElement.classList.remove('dark')
   }

   export function ThemeProvider({ children, defaultTheme = 'system' }: { children: React.ReactNode; defaultTheme?: Theme }) {
     const [theme, setThemeState] = useState<Theme>(() => {
       if (typeof window === 'undefined') return defaultTheme
       return (localStorage.getItem(STORAGE_KEY) as Theme | null) || defaultTheme
     })

     useEffect(() => {
       applyTheme(theme)
     }, [theme])

     useEffect(() => {
       if (theme !== 'system') return
       const mql = window.matchMedia('(prefers-color-scheme: dark)')
       const onChange = () => applyTheme('system')
       mql.addEventListener('change', onChange)
       return () => mql.removeEventListener('change', onChange)
     }, [theme])

     const setTheme = (t: Theme) => {
       localStorage.setItem(STORAGE_KEY, t)
       setThemeState(t)
     }

     return <ThemeContext.Provider value={{ theme, setTheme }}>{children}</ThemeContext.Provider>
   }

   export function useTheme() {
     const ctx = useContext(ThemeContext)
     if (!ctx) throw new Error('useTheme must be used inside ThemeProvider')
     return ctx
   }
   ```

5. Replace `web/src/components/responsive-dialog.test.tsx`:
   ```tsx
   import { render, screen } from '@testing-library/react'
   import { describe, expect, it, vi, beforeEach } from 'vitest'
   import { ResponsiveDialog } from './responsive-dialog'

   function setViewport(width: number) {
     Object.defineProperty(window, 'innerWidth', { writable: true, configurable: true, value: width })
     window.matchMedia = vi.fn().mockImplementation((query: string) => ({
       matches: query.includes('max-width') ? width < 768 : false,
       media: query, onchange: null,
       addEventListener: vi.fn(), removeEventListener: vi.fn(),
       addListener: vi.fn(), removeListener: vi.fn(), dispatchEvent: vi.fn(),
     }))
   }

   describe('ResponsiveDialog', () => {
     beforeEach(() => { vi.clearAllMocks() })

     it('renders Dialog with title on md+ viewport', () => {
       setViewport(1024)
       render(
         <ResponsiveDialog open onOpenChange={() => {}} title="Test title" description="Test desc">
           <div>body</div>
         </ResponsiveDialog>,
       )
       expect(screen.getByText('Test title')).toBeInTheDocument()
       expect(screen.getByText('Test desc')).toBeInTheDocument()
       expect(screen.getByText('body')).toBeInTheDocument()
     })

     it('renders Sheet content on <md viewport', () => {
       setViewport(500)
       render(
         <ResponsiveDialog open onOpenChange={() => {}} title="Mobile title">
           <div>mobile body</div>
         </ResponsiveDialog>,
       )
       expect(screen.getByText('Mobile title')).toBeInTheDocument()
       expect(screen.getByText('mobile body')).toBeInTheDocument()
     })
   })
   ```

6. Replace `web/src/components/status-row.test.tsx`:
   ```tsx
   import { render, screen } from '@testing-library/react'
   import { describe, expect, it } from 'vitest'
   import { StatusRow } from './status-row'

   describe('StatusRow', () => {
     it('renders reachable state with success color and CheckCircle icon', () => {
       const { container } = render(<StatusRow status="reachable" label="gRPC" detail="47 ms" />)
       expect(screen.getByText('Reachable')).toBeInTheDocument()
       expect(screen.getByText('gRPC')).toBeInTheDocument()
       expect(screen.getByText('47 ms')).toBeInTheDocument()
       expect(container.querySelector('.text-success')).not.toBeNull()
     })

     it('renders unreachable state with destructive color and XCircle icon', () => {
       const { container } = render(<StatusRow status="unreachable" label="MQTT" detail="connect: refused" />)
       expect(screen.getByText('Unreachable')).toBeInTheDocument()
       expect(container.querySelector('.text-destructive')).not.toBeNull()
     })

     it('renders skipped state with muted color', () => {
       const { container } = render(<StatusRow status="skipped" label="MQTT" />)
       expect(screen.getByText('Skipped')).toBeInTheDocument()
       expect(container.querySelector('.text-muted-foreground')).not.toBeNull()
     })

     it('detail renders in a font-mono element', () => {
       const { container } = render(<StatusRow status="reachable" label="gRPC" detail="42 ms" />)
       const monoEl = container.querySelector('.font-mono')
       expect(monoEl).not.toBeNull()
       expect(monoEl?.textContent).toBe('42 ms')
     })
   })
   ```

7. Replace `web/src/components/theme-provider.test.tsx`:
   ```tsx
   import { act, render } from '@testing-library/react'
   import { afterEach, beforeEach, describe, expect, it } from 'vitest'
   import { ThemeProvider, useTheme } from './theme-provider'

   function Probe() {
     const { theme, setTheme } = useTheme()
     return (
       <div>
         <span data-testid="current">{theme}</span>
         <button data-testid="dark"  onClick={() => setTheme('dark')}>dark</button>
         <button data-testid="light" onClick={() => setTheme('light')}>light</button>
       </div>
     )
   }

   describe('ThemeProvider', () => {
     beforeEach(() => {
       localStorage.clear()
       document.documentElement.classList.remove('dark')
     })
     afterEach(() => {
       localStorage.clear()
       document.documentElement.classList.remove('dark')
     })

     it('default theme is system', () => {
       const { getByTestId } = render(<ThemeProvider><Probe /></ThemeProvider>)
       expect(getByTestId('current').textContent).toBe('system')
     })

     it('setting dark adds class="dark" to root and persists', () => {
       const { getByTestId } = render(<ThemeProvider><Probe /></ThemeProvider>)
       act(() => { getByTestId('dark').click() })
       expect(document.documentElement.classList.contains('dark')).toBe(true)
       expect(localStorage.getItem('shifter-theme')).toBe('dark')
     })

     it('setting light removes the dark class', () => {
       const { getByTestId } = render(<ThemeProvider><Probe /></ThemeProvider>)
       act(() => { getByTestId('dark').click() })
       expect(document.documentElement.classList.contains('dark')).toBe(true)
       act(() => { getByTestId('light').click() })
       expect(document.documentElement.classList.contains('dark')).toBe(false)
       expect(localStorage.getItem('shifter-theme')).toBe('light')
     })
   })
   ```
  </action>
  <verify>
    <automated>cd web && pnpm test:run -- responsive-dialog status-row theme-provider 2>&1 | tee /tmp/web-comp.txt && grep -q 'pass' /tmp/web-comp.txt && ! grep -q 'fail' /tmp/web-comp.txt</automated>
  </verify>
  <acceptance_criteria>
    - File `web/src/components/responsive-dialog.tsx` exports `ResponsiveDialog` component with props `{ open, onOpenChange, title, description?, children, footer? }`
    - File `web/src/components/status-row.tsx` exports `StatusRow` component with props `{ status: 'reachable'|'unreachable'|'skipped', label, detail? }`
    - File `web/src/components/stepper.tsx` exports `Stepper` component with props `{ steps, currentIndex }`
    - File `web/src/components/theme-provider.tsx` exports `ThemeProvider` and `useTheme` hook; uses localStorage key `shifter-theme`
    - All tests in `web/src/components/responsive-dialog.test.tsx`, `status-row.test.tsx`, `theme-provider.test.tsx` pass (no `describe.skip`)
    - Command `cd web && pnpm test:run` exits 0 with all suites passing
    - `StatusRow` uses lucide icons `CheckCircle2`, `XCircle`, `MinusCircle` (UI-SPEC §Iconography)
    - `Stepper` uses `Check` icon for completed steps
  </acceptance_criteria>
  <done>
    Foundational reusable components built and tested. Plans 11/16/17/23 import these directly. Stepper used by Plan 16 wizard.
  </done>
</task>

<task type="auto">
  <name>Task 3: App shell (topbar/sidebar/account-menu) + router scaffold + api fetch wrapper + QueryClient</name>
  <files>web/src/components/shell/topbar.tsx, web/src/components/shell/sidebar.tsx, web/src/components/shell/account-menu.tsx, web/src/components/shell/responsive-shell.tsx, web/src/lib/api.ts, web/src/lib/query-client.ts, web/src/App.tsx, web/src/main.tsx, web/src/routes/_root.tsx, web/src/routes/_auth.tsx, web/src/routes/index-redirect.tsx</files>
  <read_first>
    - .planning/phases/01-foundation/01-UI-SPEC.md §"App shell (post-login, post-install)" (lines 250-275) — sidebar w-56, topbar h-14, account-menu items
    - .planning/phases/01-foundation/01-UI-SPEC.md §"Copywriting Contract" (lines 578-606) — exact strings: "Settings", "Change password", "Theme", "Sign out"
    - .planning/phases/01-foundation/01-RESEARCH.md §"React Router v7 protected route pattern" (lines 1556-1596)
  </read_first>
  <action>
1. Create `web/src/lib/query-client.ts`:
   ```ts
   import { QueryClient } from '@tanstack/react-query'

   export const queryClient = new QueryClient({
     defaultOptions: {
       queries: {
         retry: 1,
         refetchOnWindowFocus: false,
         staleTime: 30 * 1000,
       },
       mutations: { retry: 0 },
     },
   })
   ```

2. Create `web/src/lib/api.ts` — typed fetch wrapper. Sends `X-Requested-With: shifter` per RESEARCH §Security Domain "CSRF mitigated by SameSite=Lax + custom request header":
   ```ts
   export class ApiError extends Error {
     status: number
     body: unknown
     constructor(status: number, message: string, body?: unknown) {
       super(message)
       this.status = status
       this.body = body
     }
   }

   export async function apiFetch<T = unknown>(
     path: string,
     init: RequestInit = {},
   ): Promise<T> {
     const headers = new Headers(init.headers)
     headers.set('Accept', 'application/json')
     headers.set('X-Requested-With', 'shifter')
     if (init.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json')

     const res = await fetch(path, { ...init, headers, credentials: 'same-origin' })
     const text = await res.text()
     const body = text ? JSON.parse(text) : null

     if (!res.ok) {
       if (res.status === 401) {
         // Plan 11 wires the post-401 redirect once the login route exists
         window.location.assign(`/login?next=${encodeURIComponent(window.location.pathname)}`)
       }
       throw new ApiError(res.status, body?.error ?? `${res.status} ${res.statusText}`, body)
     }
     return body as T
   }
   ```

3. Create `web/src/components/shell/account-menu.tsx`:
   ```tsx
   import { Avatar, AvatarFallback } from '@/components/ui/avatar'
   import { Button } from '@/components/ui/button'
   import {
     DropdownMenu,
     DropdownMenuContent,
     DropdownMenuItem,
     DropdownMenuLabel,
     DropdownMenuSeparator,
     DropdownMenuSub,
     DropdownMenuSubContent,
     DropdownMenuSubTrigger,
     DropdownMenuTrigger,
   } from '@/components/ui/dropdown-menu'
   import { Laptop, LogOut, Moon, Sun, User } from 'lucide-react'
   import { useTheme } from '@/components/theme-provider'

   export interface AccountMenuProps {
     userEmail: string
     userRole: 'admin' | 'viewer'
     onChangePassword: () => void
     onSignOut: () => void
   }

   export function AccountMenu({ userEmail, userRole, onChangePassword, onSignOut }: AccountMenuProps) {
     const { setTheme } = useTheme()
     return (
       <DropdownMenu>
         <DropdownMenuTrigger asChild>
           <Button variant="ghost" size="icon" aria-label="Account menu">
             <Avatar className="h-8 w-8">
               <AvatarFallback><User className="h-4 w-4" aria-hidden="true" /></AvatarFallback>
             </Avatar>
           </Button>
         </DropdownMenuTrigger>
         <DropdownMenuContent align="end">
           <DropdownMenuLabel>
             <div className="text-sm font-semibold">{userEmail}</div>
             <div className="text-xs text-muted-foreground">{userRole}</div>
           </DropdownMenuLabel>
           <DropdownMenuSeparator />
           <DropdownMenuItem onClick={onChangePassword}>Change password</DropdownMenuItem>
           <DropdownMenuSub>
             <DropdownMenuSubTrigger>Theme</DropdownMenuSubTrigger>
             <DropdownMenuSubContent>
               <DropdownMenuItem onClick={() => setTheme('light')}><Sun className="mr-2 h-4 w-4" aria-hidden="true" />Light</DropdownMenuItem>
               <DropdownMenuItem onClick={() => setTheme('dark')}><Moon className="mr-2 h-4 w-4" aria-hidden="true" />Dark</DropdownMenuItem>
               <DropdownMenuItem onClick={() => setTheme('system')}><Laptop className="mr-2 h-4 w-4" aria-hidden="true" />System</DropdownMenuItem>
             </DropdownMenuSubContent>
           </DropdownMenuSub>
           <DropdownMenuSeparator />
           <DropdownMenuItem onClick={onSignOut}><LogOut className="mr-2 h-4 w-4" aria-hidden="true" />Sign out</DropdownMenuItem>
         </DropdownMenuContent>
       </DropdownMenu>
     )
   }
   ```

4. Create `web/src/components/shell/topbar.tsx`:
   ```tsx
   import shifterLogo from '@/assets/shifter-logo.svg'
   import { Button } from '@/components/ui/button'
   import { Menu } from 'lucide-react'

   export interface TopbarProps {
     installDisplayName: string
     installLogoUrl?: string
     onMenuClick?: () => void   // mobile sidebar trigger
     children?: React.ReactNode  // typically <AccountMenu />
   }

   export function Topbar({ installDisplayName, installLogoUrl, onMenuClick, children }: TopbarProps) {
     return (
       <header className="sticky top-0 z-20 flex h-14 items-center gap-3 border-b bg-card px-4 md:px-6">
         <Button variant="ghost" size="icon" className="md:hidden" aria-label="Open navigation" onClick={onMenuClick}>
           <Menu className="h-5 w-5" aria-hidden="true" />
         </Button>
         <img src={installLogoUrl ?? shifterLogo} alt="" className="h-6" />
         <span className="text-sm font-semibold">{installDisplayName}</span>
         <div className="ml-auto flex items-center gap-2">{children}</div>
       </header>
     )
   }
   ```

5. Create `web/src/components/shell/sidebar.tsx`:
   ```tsx
   import { NavLink } from 'react-router-dom'
   import { Settings as SettingsIcon } from 'lucide-react'

   const NAV = [
     { to: '/settings', label: 'Settings', icon: SettingsIcon },
   ]

   export function Sidebar() {
     return (
       <aside className="hidden w-56 shrink-0 border-r bg-card md:block">
         <nav className="flex flex-col gap-1 p-3">
           {NAV.map(({ to, label, icon: Icon }) => (
             <NavLink
               key={to}
               to={to}
               className={({ isActive }) =>
                 `flex items-center gap-2 rounded-md px-3 py-2 text-sm font-semibold ${
                   isActive ? 'bg-primary text-primary-foreground' : 'hover:bg-secondary'
                 }`
               }
             >
               <Icon className="h-4 w-4" aria-hidden="true" />
               {label}
             </NavLink>
           ))}
         </nav>
       </aside>
     )
   }
   ```

6. Create `web/src/components/shell/responsive-shell.tsx` (wraps topbar + sidebar around children):
   ```tsx
   import { Outlet } from 'react-router-dom'
   import { Sidebar } from './sidebar'
   import { Topbar } from './topbar'
   import { AccountMenu } from './account-menu'

   export interface ShellProps {
     installDisplayName: string
     installLogoUrl?: string
     userEmail: string
     userRole: 'admin' | 'viewer'
     onChangePassword: () => void
     onSignOut: () => void
   }

   export function ResponsiveShell(props: ShellProps) {
     return (
       <div className="flex min-h-screen flex-col">
         <Topbar installDisplayName={props.installDisplayName} installLogoUrl={props.installLogoUrl}>
           <AccountMenu
             userEmail={props.userEmail}
             userRole={props.userRole}
             onChangePassword={props.onChangePassword}
             onSignOut={props.onSignOut}
           />
         </Topbar>
         <div className="flex flex-1">
           <Sidebar />
           <main className="flex-1 px-4 py-4 md:px-8 md:py-6">
             <Outlet />
           </main>
         </div>
       </div>
     )
   }
   ```

7. Create `web/src/routes/_root.tsx` — protected app shell. Plan 11 wires the auth loader; for now, the loader returns a placeholder user.
   ```tsx
   import { ResponsiveShell } from '@/components/shell/responsive-shell'

   export default function RootLayout() {
     // Plan 11 replaces this stub with a real session loader.
     return (
       <ResponsiveShell
         installDisplayName="Shifter"
         userEmail="placeholder@local"
         userRole="admin"
         onChangePassword={() => { /* Plan 11 wires the change-password dialog */ }}
         onSignOut={() => { window.location.assign('/login') }}
       />
     )
   }
   ```

8. Create `web/src/routes/_auth.tsx` — public layout for login/wizard:
   ```tsx
   import { Outlet } from 'react-router-dom'

   export default function AuthLayout() {
     return (
       <div className="flex min-h-screen items-center justify-center px-4 py-8">
         <Outlet />
       </div>
     )
   }
   ```

9. Create `web/src/routes/index-redirect.tsx`:
   ```tsx
   import { Navigate } from 'react-router-dom'

   // Phase 1 only ships /settings under the protected shell. Index redirects there.
   export default function IndexRedirect() {
     return <Navigate to="/settings" replace />
   }
   ```

10. Replace `web/src/App.tsx`:
    ```tsx
    import { QueryClientProvider } from '@tanstack/react-query'
    import { RouterProvider, createBrowserRouter } from 'react-router-dom'
    import { ThemeProvider } from '@/components/theme-provider'
    import { Toaster } from '@/components/ui/sonner'
    import { queryClient } from '@/lib/query-client'
    import RootLayout from '@/routes/_root'
    import AuthLayout from '@/routes/_auth'
    import IndexRedirect from '@/routes/index-redirect'

    const router = createBrowserRouter([
      // Public auth-flow routes (login filled in Plan 23, install in Plan 16)
      {
        element: <AuthLayout />,
        children: [
          { path: '/login',   element: <div>Login screen — Plan 23</div> },
          { path: '/install', element: <div>Install wizard — Plan 16</div> },
        ],
      },
      // Protected shell
      {
        path: '/',
        element: <RootLayout />,
        children: [
          { index: true, element: <IndexRedirect /> },
          { path: 'settings', element: <div>Settings — Plan 17</div> },
        ],
      },
    ])

    export default function App() {
      return (
        <ThemeProvider>
          <QueryClientProvider client={queryClient}>
            <RouterProvider router={router} />
            <Toaster position="top-right" richColors />
          </QueryClientProvider>
        </ThemeProvider>
      )
    }
    ```

11. Update `web/src/main.tsx` (Plan 01 already created the bare version; this version is identical — just confirm imports work):
    No change required if the file from Plan 01 already imports `App` and `index.css`.

12. Verify `pnpm build` and `pnpm test:run` still pass.
  </action>
  <verify>
    <automated>cd web && pnpm build && pnpm test:run</automated>
  </verify>
  <acceptance_criteria>
    - File `web/src/lib/api.ts` exports `apiFetch` and `ApiError`; `apiFetch` adds the `X-Requested-With: shifter` header
    - File `web/src/lib/query-client.ts` exports a `queryClient: QueryClient` instance
    - File `web/src/components/shell/account-menu.tsx` exports `AccountMenu` with menu items: "Change password", "Theme" (with Light/Dark/System submenu), "Sign out" (UI-SPEC verbs)
    - File `web/src/components/shell/topbar.tsx` exports `Topbar` with sticky h-14 + bg-card + border-b
    - File `web/src/components/shell/sidebar.tsx` exports `Sidebar` with `w-56` width and one NavLink to `/settings` labeled exactly "Settings"
    - File `web/src/App.tsx` wraps the router in `ThemeProvider` → `QueryClientProvider` → `RouterProvider` and includes `<Toaster position="top-right" />`
    - File `web/src/App.tsx` defines routes: `/login` (placeholder), `/install` (placeholder), `/` (RootLayout) with `/settings` child
    - Command `cd web && pnpm build` exits 0
    - Command `cd web && pnpm test:run` exits 0 with previous tests still passing
    - `apiFetch` redirects to `/login?next=...` on 401
  </acceptance_criteria>
  <done>
    App shell visible: navy theme, Inter font, sidebar+topbar, account menu with theme submenu. Plans 11/16/17/23 fill the placeholder routes with real screens.
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| browser → /api/* | Cookie-borne session + X-Requested-With CSRF token |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-06-01 | Tampering (CSRF) | state-changing API endpoints | mitigate | `apiFetch` always sends `X-Requested-With: shifter`; backend (Plan 11) requires it on POST/PUT/DELETE. ASVS V13. |
| T-06-02 | Tampering (XSS) | install identity display name rendered in topbar | mitigate | React auto-escapes; never `dangerouslySetInnerHTML` for operator-supplied strings. ASVS V5. |
| T-06-03 | Information Disclosure | source maps in production bundle | accept | Self-hosted single-tenant; sourcemaps aid customer support. |
| T-06-04 | Spoofing | theme provider reads from arbitrary localStorage value | accept | localStorage is per-origin; "system"/"light"/"dark" enum-validated implicitly via the Theme type (anything else falls through to "system" via applyTheme). |
</threat_model>

<verification>
- shadcn init complete (components.json present); 21 ui components installed
- OKLCH navy palette in `web/src/theme.css`; primary at `oklch(0.42 0.17 255)`
- Inter and JetBrains Mono self-hosted via @fontsource (no Google Fonts CDN)
- ResponsiveDialog, StatusRow, Stepper, ThemeProvider built and tested
- App shell mounted; router defines `/`, `/login`, `/install`, `/settings`
- `pnpm build` and `pnpm test:run` both pass
</verification>

<success_criteria>
- UX-02 satisfied (shadcn navy palette + English copy + modern minimal aesthetic)
- All 21 shadcn primitives installed for downstream plans to consume directly
- ResponsiveDialog enforces UX-01 modal-first pattern with mobile sheet swap
- StatusRow ready for Plan 17 Test Connection
- Stepper ready for Plan 16 install wizard
- ThemeProvider supports UI-SPEC dark mode default=system
</success_criteria>

<output>
After completion, create `.planning/phases/01-foundation/01-06-SUMMARY.md` documenting:
- shadcn init choices (style/base/cssVariables) — DO NOT re-run `init` (PITFALL #12)
- 21 components installed — adding more in later phases is `shadcn add <name>` only
- Theme tokens locked in `theme.css` (separate from index.css to survive `shadcn add`)
- Foundational components exported (`ResponsiveDialog`, `StatusRow`, `Stepper`, `ThemeProvider`)
- API conventions (X-Requested-With, 401 redirect, JSON-only)
- Where Plan 11/14/16/17/23 plug in (placeholder elements in App.tsx)
</output>
