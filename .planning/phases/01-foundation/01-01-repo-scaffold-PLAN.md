---
phase: 01-foundation
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - .gitignore
  - .editorconfig
  - .dockerignore
  - go.mod
  - go.sum
  - cmd/shifter/main.go
  - Justfile
  - README.md
  - web/package.json
  - web/pnpm-workspace.yaml
  - web/.gitignore
  - web/tsconfig.json
  - web/tsconfig.node.json
  - web/vite.config.ts
  - web/index.html
  - web/src/main.tsx
  - web/src/App.tsx
  - web/src/index.css
  - web/src/vite-env.d.ts
  - web/biome.json
  - .air.toml
autonomous: true
requirements:
  - OPS-01
must_haves:
  truths:
    - "go build ./cmd/shifter produces a runnable binary that prints 'shifter' on `--help`"
    - "pnpm --dir web install completes without errors using pnpm-lock.yaml only (no package-lock.json, no yarn.lock)"
    - "pnpm --dir web build produces web/dist/index.html"
    - "just --list shows dev/build/test/migrate/lint recipes (D-02)"
  artifacts:
    - path: "go.mod"
      provides: "Go module declaration at github.com/shifter-io/shifter, go 1.24"
      contains: "module github.com/shifter-io/shifter"
    - path: "Justfile"
      provides: "Canonical build/dev/test/lint/migrate orchestrator (D-02)"
      contains: "dev:"
    - path: "cmd/shifter/main.go"
      provides: "Cobra root command stub (D-01, D-12)"
      contains: "package main"
    - path: "web/vite.config.ts"
      provides: "Vite + React + Tailwind 4 config with /api and /sse proxy to :8080 (D-04)"
      contains: "proxy"
    - path: "web/package.json"
      provides: "pnpm-locked frontend deps; React 19, Vite 7, TypeScript 5.6+ (D-03)"
      contains: "\"packageManager\": \"pnpm"
    - path: ".air.toml"
      provides: "Air hot-reload config for cmd/shifter (D-04)"
      contains: "cmd/shifter"
  key_links:
    - from: "Justfile"
      to: "cmd/shifter and web/"
      via: "`just dev` runs `air` + `pnpm --dir web dev` in parallel"
      pattern: "air.*pnpm"
    - from: "web/vite.config.ts"
      to: ":8080"
      via: "proxy block routes /api and /sse to Go backend"
      pattern: "localhost:8080"
---

<objective>
Bootstrap the Shifter monorepo at the repo root with the Go module, Cobra entry point, Vite+React+TS+Tailwind 4 frontend, Justfile orchestrator, and Air hot-reload config. This plan creates the empty-but-runnable skeleton — no business logic yet. Every later plan builds on these conventions.

Purpose: Establish D-01 (single Go monorepo, `cmd/shifter/`, `internal/`, `web/`), D-02 (Justfile), D-03 (pnpm only), D-04 (Air + Vite dev with /api+/sse proxy). Without this scaffolding, no later task can compile or run.

Output: A repo where `just build` succeeds, `pnpm --dir web build` produces `web/dist`, and `just dev` can start both processes (binary serves nothing useful yet — Plan 03 adds the HTTP listener).
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/STATE.md
@.planning/ROADMAP.md
@.planning/phases/01-foundation/01-CONTEXT.md
@.planning/phases/01-foundation/01-RESEARCH.md
@.planning/phases/01-foundation/01-UI-SPEC.md

<interfaces>
<!-- New project — no prior interfaces. This plan creates the canonical layout. -->

Target Go module path: `github.com/shifter-io/shifter`

Justfile recipes (canonical names — DO NOT rename):
- `bootstrap` — install dev tools (`go install` for air, sqlc, mockgen, golangci-lint; `pnpm install`)
- `dev` — run Air + Vite concurrently
- `build` — `go build -o bin/shifter ./cmd/shifter` and `pnpm --dir web build`
- `test` — `go test ./... -race -count=1` and `pnpm --dir web test --run`
- `lint` — `golangci-lint run` and `pnpm --dir web lint` (biome)
- `migrate` — wrapper for `bin/shifter migrate up`
- `compose-smoke-bundled` — compose up bundled, wait for /health, compose down (Wave 0 stub OK)
- `compose-smoke-external` — same, external flavor

Vite proxy targets (D-04, RESEARCH §Pattern 9):
```ts
proxy: {
  '/api': { target: 'http://localhost:8080', changeOrigin: true, secure: false },
  '/sse': { target: 'http://localhost:8080', changeOrigin: true, secure: false,
            proxyTimeout: 60*60*1000, timeout: 60*60*1000 },
}
```
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 1: Go monorepo skeleton with Cobra entry stub</name>
  <files>go.mod, go.sum, cmd/shifter/main.go, .gitignore, .editorconfig, .dockerignore, Justfile, README.md, .air.toml</files>
  <read_first>
    - .planning/phases/01-foundation/01-CONTEXT.md (D-01, D-02, D-04, D-12)
    - .planning/phases/01-foundation/01-RESEARCH.md §"Recommended Project Structure" and §"Wiring serve" (lines 254-370 and 1390-1476)
    - CLAUDE.md (Go 1.24+ pinned; do-not-use list)
  </read_first>
  <action>
1. Initialize Go module: `go mod init github.com/shifter-io/shifter` then `go mod edit -go=1.24`. Commit `go.sum` empty for now (Plan 02 will populate via `go get`).
2. Create `cmd/shifter/main.go` with a minimal Cobra root:
   ```go
   package main

   import (
       "os"
       "github.com/spf13/cobra"
   )

   var rootCmd = &cobra.Command{
       Use:   "shifter",
       Short: "Shifter — self-hosted LoRaWAN water/electricity monitoring",
   }

   func main() {
       if err := rootCmd.Execute(); err != nil {
           os.Exit(1)
       }
   }
   ```
   Run `go get github.com/spf13/cobra@latest` so this file compiles.
3. Create `.gitignore` with EXACT contents:
   ```
   # Build artifacts
   /bin/
   /web/dist/
   /web/node_modules/
   /tmp/
   /coverage/
   *.test
   *.out

   # Editor / OS
   .DS_Store
   .idea/
   .vscode/
   *.swp

   # Environment / secrets — D-06 forbids .env for secrets
   .env
   .env.*
   !.env.example
   /secrets/*
   !/secrets/.gitkeep

   # Air hot reload
   /tmp/
   ```
4. Create `.editorconfig`:
   ```
   root = true
   [*]
   end_of_line = lf
   insert_final_newline = true
   charset = utf-8
   indent_style = space
   indent_size = 4
   [*.{ts,tsx,js,jsx,json,yaml,yml,md}]
   indent_size = 2
   [Makefile,*.mk]
   indent_style = tab
   ```
5. Create `.dockerignore`:
   ```
   .git
   .gitignore
   /bin
   /web/node_modules
   /web/dist
   /tmp
   /secrets
   *.md
   .planning
   ```
6. Create `.air.toml` (D-04) configured to watch Go sources only, build to `tmp/shifter`:
   ```toml
   root = "."
   tmp_dir = "tmp"
   [build]
   cmd = "go build -o ./tmp/shifter ./cmd/shifter"
   bin = "tmp/shifter serve"
   include_ext = ["go", "tpl", "tmpl", "html"]
   exclude_dir = ["assets", "tmp", "vendor", "web", "node_modules", ".planning", "secrets"]
   delay = 500
   [log]
   time = false
   [misc]
   clean_on_exit = true
   ```
7. Create `Justfile` (D-02) at repo root with EXACTLY these recipes:
   ```just
   default:
       @just --list

   # Install required dev tools
   bootstrap:
       go install github.com/air-verse/air@latest
       go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
       go install go.uber.org/mock/mockgen@latest
       cd web && pnpm install --frozen-lockfile

   # Run Go binary (Air) + Vite SPA dev server in parallel
   dev:
       #!/usr/bin/env bash
       set -euo pipefail
       (air) & (cd web && pnpm dev) & wait

   # Build production binary + SPA bundle
   build:
       cd web && pnpm build
       go build -o bin/shifter ./cmd/shifter

   # Run all tests
   test:
       go test ./... -race -count=1
       cd web && pnpm test --run

   # Quick test (unit only, skip -count=1 cache)
   test-quick:
       go test ./internal/... -short -race
       cd web && pnpm test --run

   # Lint everything
   lint:
       golangci-lint run ./...
       cd web && pnpm lint

   # Apply migrations via the binary
   migrate *args:
       go run ./cmd/shifter migrate {{args}}

   # Smoke-test bundled compose (Wave 0 stub — Plan 20 implements)
   compose-smoke-bundled:
       @echo "TODO: implemented in Plan 20"
       @exit 1

   compose-smoke-external:
       @echo "TODO: implemented in Plan 21"
       @exit 1
   ```
8. Create a stub `README.md` with: project name, one-line description, `just bootstrap && just dev` instructions placeholder. Plan 24 will write the real README.
9. Verify: `go build ./cmd/shifter` succeeds; `just --list` shows all recipes.
  </action>
  <verify>
    <automated>go build -o /tmp/shifter-test ./cmd/shifter && /tmp/shifter-test --help | grep -q 'shifter' && rm /tmp/shifter-test && just --list | grep -q 'dev'</automated>
  </verify>
  <acceptance_criteria>
    - File `go.mod` exists and contains exactly `module github.com/shifter-io/shifter` on line 1 and `go 1.24` (or higher) on a subsequent line
    - File `cmd/shifter/main.go` exists, package is `main`, imports `github.com/spf13/cobra`, and defines a `rootCmd` variable with `Use: "shifter"`
    - File `.gitignore` exists and contains all of: `/web/node_modules/`, `/web/dist/`, `.env`, `/secrets/*`, `tmp/`
    - File `Justfile` exists and `just --list` exits 0 listing recipes named exactly: `bootstrap`, `dev`, `build`, `test`, `test-quick`, `lint`, `migrate`, `compose-smoke-bundled`, `compose-smoke-external`
    - File `.air.toml` exists and contains `cmd = "go build -o ./tmp/shifter ./cmd/shifter"` (verbatim)
    - Command `go build ./cmd/shifter` exits 0
    - Command `bin/shifter --help` (after `just build`) prints the string `shifter` (D-12)
  </acceptance_criteria>
  <done>
    Bare-bones Go monorepo compiles. `just --list` works. No business logic — just the skeleton.
  </done>
</task>

<task type="auto">
  <name>Task 2: Vite + React 19 + Tailwind 4 + TS frontend scaffold (no shadcn yet)</name>
  <files>web/package.json, web/pnpm-lock.yaml, web/.gitignore, web/tsconfig.json, web/tsconfig.node.json, web/vite.config.ts, web/index.html, web/src/main.tsx, web/src/App.tsx, web/src/index.css, web/src/vite-env.d.ts, web/biome.json</files>
  <read_first>
    - .planning/phases/01-foundation/01-CONTEXT.md (D-01, D-03, D-04)
    - .planning/phases/01-foundation/01-RESEARCH.md §"Pattern 9: Vite proxy for /api and /sse" (lines 729-770) — exact proxy block to use
    - .planning/phases/01-foundation/01-RESEARCH.md §"Frontend (verified peer-deps)" (lines 158-196) — exact versions
    - .planning/phases/01-foundation/01-UI-SPEC.md §"Design System" (lines 39-79) — confirms style/base/CSS variables choices for Plan 06 (this plan ships barebones; Plan 06 runs `shadcn init`)
    - CLAUDE.md (Frontend libs pinned; pnpm only)
  </read_first>
  <action>
1. Initialize web/ with pnpm: `cd web && pnpm init`. Set `"packageManager": "pnpm@10.33.2"` in `web/package.json`. Set `"private": true`.
2. Install React 19 + Vite 7 + TS 5.6+ + Tailwind 4 deps. Run from inside `web/`:
   ```bash
   pnpm add react@^19 react-dom@^19
   pnpm add -D typescript@^5.6 @types/react@^19 @types/react-dom@^19
   pnpm add -D vite@^7 @vitejs/plugin-react
   pnpm add tailwindcss@^4 @tailwindcss/vite
   pnpm add -D @biomejs/biome@latest
   ```
   DO NOT install shadcn here — Plan 06 runs `pnpm dlx shadcn@latest init`. DO NOT install fontsource here — Plan 06 handles fonts.
   DO NOT install routing/forms/data-fetching libs here — they install in later plans.
3. Create `web/.gitignore`:
   ```
   node_modules
   dist
   .vite
   *.log
   ```
4. Create `web/tsconfig.json` with strict mode, JSX preserve, paths alias for `@/*`:
   ```json
   {
     "compilerOptions": {
       "target": "ES2022",
       "useDefineForClassFields": true,
       "lib": ["ES2022", "DOM", "DOM.Iterable"],
       "module": "ESNext",
       "skipLibCheck": true,
       "moduleResolution": "bundler",
       "allowImportingTsExtensions": true,
       "resolveJsonModule": true,
       "isolatedModules": true,
       "noEmit": true,
       "jsx": "react-jsx",
       "strict": true,
       "noUnusedLocals": true,
       "noUnusedParameters": true,
       "noFallthroughCasesInSwitch": true,
       "baseUrl": ".",
       "paths": { "@/*": ["./src/*"] }
     },
     "include": ["src"],
     "references": [{ "path": "./tsconfig.node.json" }]
   }
   ```
5. Create `web/tsconfig.node.json` for vite.config.ts:
   ```json
   {
     "compilerOptions": {
       "composite": true,
       "skipLibCheck": true,
       "module": "ESNext",
       "moduleResolution": "bundler",
       "allowSyntheticDefaultImports": true,
       "strict": true
     },
     "include": ["vite.config.ts"]
   }
   ```
6. Create `web/vite.config.ts` EXACTLY as specified in RESEARCH.md Pattern 9:
   ```ts
   import { defineConfig } from 'vite'
   import react from '@vitejs/plugin-react'
   import tailwindcss from '@tailwindcss/vite'
   import path from 'node:path'

   export default defineConfig({
     plugins: [react(), tailwindcss()],
     resolve: {
       alias: { '@': path.resolve(__dirname, './src') },
     },
     server: {
       port: 5173,
       strictPort: true,
       proxy: {
         '/api': {
           target: 'http://localhost:8080',
           changeOrigin: true,
           secure: false,
         },
         '/sse': {
           target: 'http://localhost:8080',
           changeOrigin: true,
           secure: false,
           proxyTimeout: 60 * 60 * 1000,
           timeout: 60 * 60 * 1000,
         },
       },
     },
     build: { outDir: 'dist', sourcemap: true },
   })
   ```
7. Create `web/index.html`:
   ```html
   <!doctype html>
   <html lang="en" class="">
     <head>
       <meta charset="UTF-8" />
       <meta name="viewport" content="width=device-width, initial-scale=1.0" />
       <title>Shifter</title>
     </head>
     <body>
       <div id="root"></div>
       <script type="module" src="/src/main.tsx"></script>
     </body>
   </html>
   ```
8. Create `web/src/main.tsx`:
   ```tsx
   import React from 'react'
   import ReactDOM from 'react-dom/client'
   import App from './App'
   import './index.css'

   ReactDOM.createRoot(document.getElementById('root')!).render(
     <React.StrictMode>
       <App />
     </React.StrictMode>,
   )
   ```
9. Create `web/src/App.tsx` as a one-screen placeholder until Plan 06 wires the real shell:
   ```tsx
   export default function App() {
     return (
       <div className="min-h-screen flex items-center justify-center">
         <h1 className="text-2xl font-semibold">Shifter</h1>
       </div>
     )
   }
   ```
10. Create `web/src/index.css` (Tailwind 4 single-line import — Plan 06 adds the OKLCH theme):
    ```css
    @import "tailwindcss";
    ```
11. Create `web/src/vite-env.d.ts`:
    ```ts
    /// <reference types="vite/client" />
    ```
12. Create `web/biome.json` (formatter + linter, replaces eslint+prettier):
    ```json
    {
      "$schema": "https://biomejs.dev/schemas/1.9.4/schema.json",
      "vcs": { "enabled": true, "clientKind": "git", "useIgnoreFile": true },
      "files": { "ignore": ["dist", "node_modules", "src/components/ui"] },
      "linter": { "enabled": true, "rules": { "recommended": true } },
      "formatter": {
        "enabled": true,
        "indentStyle": "space",
        "indentWidth": 2,
        "lineWidth": 100
      },
      "javascript": { "formatter": { "quoteStyle": "single", "semicolons": "asNeeded" } },
      "organizeImports": { "enabled": true }
    }
    ```
13. Add scripts to `web/package.json`:
    ```json
    "scripts": {
      "dev": "vite",
      "build": "tsc -b && vite build",
      "preview": "vite preview",
      "lint": "biome check ./src",
      "lint:fix": "biome check --write ./src",
      "format": "biome format --write ./src"
    }
    ```
14. Run `pnpm install` (writes pnpm-lock.yaml). Verify `pnpm build` succeeds.
  </action>
  <verify>
    <automated>cd web && pnpm install --frozen-lockfile && pnpm build && test -f dist/index.html && [ ! -f package-lock.json ] && [ ! -f yarn.lock ]</automated>
  </verify>
  <acceptance_criteria>
    - File `web/package.json` exists; `jq -r '.packageManager' web/package.json` outputs a string starting with `pnpm@`
    - File `web/package.json` contains exactly the dependencies `react@^19`, `react-dom@^19`, `tailwindcss@^4`, `@tailwindcss/vite` and devDependencies `vite@^7`, `@vitejs/plugin-react`, `typescript@^5.6`, `@biomejs/biome`
    - File `web/pnpm-lock.yaml` exists; files `web/package-lock.json` and `web/yarn.lock` do NOT exist (D-03)
    - File `web/vite.config.ts` exists and contains both `'/api'` and `'/sse'` proxy keys with `target: 'http://localhost:8080'`
    - File `web/vite.config.ts` contains `proxyTimeout: 60 * 60 * 1000` (SSE timeout)
    - File `web/tsconfig.json` has `"strict": true` and `"paths": { "@/*": ["./src/*"] }`
    - Command `cd web && pnpm install --frozen-lockfile` exits 0
    - Command `cd web && pnpm build` exits 0 and produces `web/dist/index.html`
    - File `web/biome.json` exists with `"src/components/ui"` in `files.ignore` (so future shadcn components don't get reformatted)
  </acceptance_criteria>
  <done>
    Vite SPA scaffold builds. `just dev` would run both processes (binary listens nowhere yet — Plan 03+ adds the listener). No shadcn, no fontsource, no router yet — those come in Plan 06.
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| build-time → runtime | Bootstrap script trust (npm registry, Go module proxy) |
| dev → prod | `.gitignore` and `.dockerignore` keep secrets/credentials out of build context |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-01-01 | Tampering | npm/Go module supply chain | mitigate | `pnpm install --frozen-lockfile` enforces `pnpm-lock.yaml`; `go.sum` checked in; CLAUDE.md "do not use" list enforced. ASVS V10. |
| T-01-02 | Information Disclosure | accidentally committing `.env` or `secrets/` to git | mitigate | `.gitignore` excludes `.env`, `.env.*`, `/secrets/*`. `.dockerignore` excludes `/secrets`. ASVS V8. |
| T-01-03 | Tampering | dev-mode CSP / mixed origin via Vite proxy | accept | Dev-only proxy on `localhost`; production uses Caddy + embedded SPA (Plan 19, Plan 22). |
| T-01-04 | Information Disclosure | source maps shipped in production bundle | accept | `sourcemap: true` is fine for self-hosted single-tenant; operator can disable per install via Vite env. ASVS V14. |
</threat_model>

<verification>
- `go build ./cmd/shifter` produces a runnable binary
- `cd web && pnpm install --frozen-lockfile && pnpm build` succeeds without errors and emits `web/dist/index.html`
- `just --list` shows the canonical recipe set
- No `package-lock.json` or `yarn.lock` files exist (D-03)
- `.gitignore` excludes secrets and build artifacts
</verification>

<success_criteria>
- Repo is a runnable Go monorepo at `github.com/shifter-io/shifter`
- Justfile is the canonical entry point for dev/build/test/lint
- Frontend builds via pnpm-lock.yaml only (no other lock files)
- Air config exists ready for hot reload (used in Plan 03+)
- Vite proxy targets `:8080` for both `/api` and `/sse` (SSE config follows RESEARCH.md Pattern 9)
</success_criteria>

<output>
After completion, create `.planning/phases/01-foundation/01-01-SUMMARY.md` documenting:
- Go module path, Justfile recipe set, frontend deps installed
- Vite proxy configuration verbatim
- Files created (full list)
- Anything Plan 02+ should know (e.g., where future packages live)
</output>
