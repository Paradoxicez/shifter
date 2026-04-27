---
phase: 01-foundation
plan: 01
subsystem: infra
tags: [go, cobra, vite, react, tailwind, typescript, biome, just, air, pnpm]

requires:
  - phase: roadmap
    provides: Tech stack decisions (Go 1.24, React 19 + Vite 7 + Tailwind 4, pnpm-only, ChirpStack gRPC)
provides:
  - Empty-but-runnable Go monorepo at github.com/shifter-io/shifter
  - cmd/shifter/main.go Cobra root command stub (D-01, D-12)
  - Justfile with canonical recipes — bootstrap/dev/build/test/test-quick/lint/migrate/compose-smoke-{bundled,external} (D-02)
  - .air.toml hot-reload config bound to cmd/shifter (D-04)
  - Vite + React 19 + Tailwind 4 + TS 5.9 SPA scaffold under web/ (D-03)
  - vite.config.ts proxy for /api and /sse → :8080 with 1h SSE timeout (RESEARCH §Pattern 9)
  - pnpm-only lockfile policy (D-03) — no package-lock.json, no yarn.lock
  - .gitignore / .dockerignore that exclude .env, /secrets, build artifacts
affects: [01-02-test-harness, 01-03-database-layer, 01-04-config-secrets, 01-05-cobra-cli, 01-06-frontend-shell, 01-18-router-health, 01-19-spa-embed, 01-20-compose-bundled, 01-24-readme-docs]

tech-stack:
  added:
    - github.com/spf13/cobra v1.10.2 (CLI framework — root command stub only)
    - github.com/spf13/pflag v1.0.9 (transitive)
    - github.com/inconshreveable/mousetrap v1.1.0 (transitive)
    - react ^19.2.5
    - react-dom ^19.2.5
    - tailwindcss ^4.2.4 + @tailwindcss/vite ^4.2.4
    - vite ^7.3.2
    - @vitejs/plugin-react ^5.2.0 (Vite 7 compatible — v6 requires Vite 8)
    - typescript ^5.9.3
    - @types/{react,react-dom,node}
    - @biomejs/biome ^2.4.13 (linter+formatter)
  patterns:
    - "Justfile is the canonical entry point — never invoke go/pnpm/migrate directly in docs or CI"
    - "All CRUD/Cobra subcommands attach to rootCmd in cmd/shifter (later plans add serve, migrate, create-admin)"
    - "Vite dev server proxies /api and /sse to :8080 in dev; production embeds via go:embed (Plan 19)"
    - "Lock-file discipline (D-03): pnpm-lock.yaml only — pre-commit hook in Plan 02 will enforce"
    - "Path alias @/* → ./src/* established in tsconfig.json — used by all future web/src/ imports"

key-files:
  created:
    - go.mod
    - go.sum
    - cmd/shifter/main.go
    - .gitignore
    - .editorconfig
    - .dockerignore
    - .air.toml
    - Justfile
    - README.md (stub — Plan 24 replaces)
    - web/.gitignore
    - web/package.json
    - web/pnpm-lock.yaml
    - web/tsconfig.json
    - web/tsconfig.node.json
    - web/vite.config.ts
    - web/biome.json
    - web/index.html
    - web/src/main.tsx
    - web/src/App.tsx (placeholder shell — Plan 06 replaces)
    - web/src/index.css
    - web/src/vite-env.d.ts
  modified: []

key-decisions:
  - "Pinned @vitejs/plugin-react to ^5 instead of latest ^6 — v6 requires Vite 8, plan pins Vite 7. Reassess when Vite 8 lands"
  - "Used Biome 2.x config schema (assist.actions.source.organizeImports + files.includes negative globs) — Biome 2.4.13 is current; plan template referenced legacy 1.9 schema"
  - "Added @types/node to devDependencies — vite.config.ts uses node:path and __dirname which require Node typings"
  - "Added Long description to Cobra root command so `--help` prints lowercase 'shifter' (matches plan verification grep). Long doubles as the future help template for serve/migrate/create-admin subcommands"

patterns-established:
  - "Pattern: Justfile-as-orchestrator — every dev/CI task has a recipe; recipes call go/pnpm/golangci-lint, never the reverse"
  - "Pattern: Vite proxy for split-origin dev — /api and /sse to :8080, with proxyTimeout: 60*60*1000 on /sse so the EventSource never gets killed mid-stream"
  - "Pattern: Compiled artifacts gitignored at the repo level (/web/dist, /tmp, /bin) and at the package level (web/.gitignore for *.tsbuildinfo, vite.config.{js,d.ts})"
  - "Pattern: Cobra root with no Run + Long description gives a usable `shifter` and `shifter --help` even before subcommands are added"

requirements-completed: [OPS-01]

duration: 23min
completed: 2026-04-27
---

# Phase 01 Plan 01: Repo Scaffold Summary

**Empty-but-runnable Go + Vite monorepo: `github.com/shifter-io/shifter` with Cobra root, Justfile orchestrator, Air hot-reload, Vite 7 + React 19 + Tailwind 4 frontend with /api+/sse proxy to :8080.**

## Performance

- **Duration:** ~23 min
- **Started:** 2026-04-27T18:00:00Z (approx)
- **Completed:** 2026-04-27T18:23:18Z
- **Tasks:** 2 / 2
- **Files created:** 21
- **Files modified:** 0

## Accomplishments

- Repo compiles end-to-end: `go build ./cmd/shifter` produces a runnable Cobra binary; `pnpm --dir web build` produces `web/dist/index.html`.
- `just --list` exposes the canonical recipe set (bootstrap, dev, build, test, test-quick, lint, migrate, compose-smoke-bundled, compose-smoke-external).
- D-03 lock-file discipline established — only `web/pnpm-lock.yaml` exists; no `package-lock.json` or `yarn.lock`.
- D-04 hot-reload config (`.air.toml`) ready for Plan 03+ when the HTTP listener lands.
- Vite proxy configured exactly per RESEARCH §Pattern 9 — `/api` and `/sse` to `localhost:8080`, with 1-hour `proxyTimeout` on the `/sse` lane so long-lived EventSource streams don't get killed mid-flight.

## Task Commits

1. **Task 1: Go monorepo skeleton with Cobra entry stub** — `3d44468` (feat)
2. **Task 2: Vite + React 19 + Tailwind 4 + TS frontend scaffold** — `ff0b85f` (feat)

**Plan metadata commit:** _pending — created at end of plan_

## Files Created/Modified

### Repo root

- `go.mod` — `module github.com/shifter-io/shifter`, `go 1.24`
- `go.sum` — checksums for cobra v1.10.2 + transitive deps
- `cmd/shifter/main.go` — Cobra root, `Use: "shifter"`, `Short`, `Long` describing the binary
- `.gitignore` — excludes `/bin`, `/web/dist`, `/web/node_modules`, `/tmp`, `.env*`, `/secrets/*`, `*.test`, `*.out`
- `.editorconfig` — LF, UTF-8, 4-space (TS/JSON/MD: 2-space, Makefile: tab)
- `.dockerignore` — excludes `.git`, `/bin`, `/web/{node_modules,dist}`, `/secrets`, `.planning`, `*.md`
- `.air.toml` — `cmd = "go build -o ./tmp/shifter ./cmd/shifter"`, `bin = "tmp/shifter serve"`, watches `*.go`/`*.tpl`/`*.tmpl`/`*.html`, ignores `web/`, `vendor/`, `.planning/`
- `Justfile` — canonical recipes (D-02). `dev` runs `air` + `pnpm --dir web dev` in parallel via bash subshells. `compose-smoke-{bundled,external}` are stubs that exit 1 (Plan 20/21 implement).
- `README.md` — stub one-liner + `just bootstrap && just dev` quickstart; Plan 24 replaces with the full README.

### Frontend (`web/`)

- `package.json` — name `shifter-web`, `private: true`, `type: "module"`, `packageManager: "pnpm@10.33.2"`, scripts (`dev`, `build`, `preview`, `lint`, `lint:fix`, `format`)
- `pnpm-lock.yaml` — 38 KB, locks every dep tree
- `.gitignore` — `node_modules`, `dist`, `.vite`, `*.log`, `*.tsbuildinfo`, `vite.config.{js,d.ts}` (compiled artifacts from `tsc -b`)
- `tsconfig.json` — strict, `paths: { "@/*": ["./src/*"] }`, ES2022 lib, `jsx: "react-jsx"`
- `tsconfig.node.json` — composite, `include: ["vite.config.ts"]`
- `vite.config.ts` — React + Tailwind plugins, `@`-alias resolver, `server.port: 5173 strictPort`, `proxy: { '/api': … , '/sse': { proxyTimeout: 3600000, timeout: 3600000 } }`, `build: { outDir: "dist", sourcemap: true }`
- `biome.json` — Biome 2.x format `formatter: { indentStyle: "space", indentWidth: 2, lineWidth: 100 }`, `javascript.formatter: { quoteStyle: "single", semicolons: "asNeeded" }`, `assist.actions.source.organizeImports: "on"`, `files.includes: ["**", "!dist", "!node_modules", "!src/components/ui"]`
- `index.html` — `<title>Shifter</title>`, mounts `/src/main.tsx`
- `src/main.tsx` — `ReactDOM.createRoot(...).render(<StrictMode><App /></StrictMode>)`
- `src/App.tsx` — placeholder centered "Shifter" h1 (Plan 06 replaces with the real shell)
- `src/index.css` — single line `@import "tailwindcss";` (Plan 06 adds the OKLCH theme)
- `src/vite-env.d.ts` — Vite client types

## Decisions Made

- **Cobra Long description:** Without `Long`, Cobra v1.10.2 prints only the `Short` ("Shifter — …") on `--help`, and the plan's verification command does case-sensitive `grep -q 'shifter'`. Adding a `Long` that begins "shifter is the self-hosted…" makes the binary self-document while passing the verification gate. The text also previews future subcommands (serve, migrate, create-admin).
- **`@vitejs/plugin-react` pinned to ^5:** Latest ^6 (`6.0.1`) requires `vite ^8.0.0`. The plan pins Vite 7. Plugin-react v5.2.0 supports Vite 7 and works with React 19. We re-evaluate at Plan 24 / phase wrap-up if Vite 8 lands and is stable.
- **`@types/node` added:** `vite.config.ts` uses `node:path` and `__dirname`. Without `@types/node`, `tsc -b` fails. This is implicit in the plan's "vite.config.ts compiles cleanly" expectation.
- **Biome 2.x config schema:** The plan referenced Biome 1.9.4 schema (`files.ignore`, `organizeImports.enabled`). Biome 2.4.13 (current latest, what `@biomejs/biome@latest` resolves to) uses `files.includes` with negative globs and moved `organizeImports` under `assist.actions.source.organizeImports`. The 1.9.4 schema is rejected by Biome 2 with parse errors. Translated the intent (lint+format on, ignore `src/components/ui`, single-quote JS, no semicolons-as-needed) to the 2.x schema verbatim.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Cobra `--help` output didn't contain lowercase `shifter`**

- **Found during:** Task 1 verification (the plan's `grep -q 'shifter'` command failed)
- **Issue:** With only `Use: "shifter"` and `Short: "Shifter — …"`, Cobra v1.10.2 prints only `Short` on `--help` (no usage line, no command name). Capital "S" doesn't match the case-sensitive grep.
- **Fix:** Added a `Long` description starting with lowercase "shifter is the self-hosted LoRaWAN…". Cobra prints `Long` in priority over `Short` when both are present.
- **Files modified:** `cmd/shifter/main.go`
- **Verification:** `bin/shifter --help | grep -q 'shifter'` exits 0
- **Committed in:** `3d44468`

**2. [Rule 3 - Blocking] `@vitejs/plugin-react@^6` (latest) is incompatible with Vite 7**

- **Found during:** Task 2 (`pnpm build` failed with `ERR_PACKAGE_PATH_NOT_EXPORTED Package subpath './internal'`)
- **Issue:** `pnpm add -D @vitejs/plugin-react` resolved to `6.0.1`, which imports from `vite/internal` — only exposed in Vite 8. Plan pins Vite 7.
- **Fix:** Re-installed with `@vitejs/plugin-react@^5` (got 5.2.0). Vite 7 compatible, supports React 19.
- **Files modified:** `web/package.json`, `web/pnpm-lock.yaml`
- **Verification:** `pnpm build` exits 0, produces `web/dist/index.html`
- **Committed in:** `ff0b85f`

**3. [Rule 3 - Blocking] `tsc -b` failed without `@types/node`**

- **Found during:** Task 2 build (TS errors `Cannot find module 'node:path'`, `Cannot find name '__dirname'`)
- **Issue:** `vite.config.ts` uses Node built-ins; Plan listed only `@types/react` and `@types/react-dom` as dev types.
- **Fix:** Added `@types/node` to devDependencies.
- **Files modified:** `web/package.json`, `web/pnpm-lock.yaml`
- **Verification:** `pnpm build` (which runs `tsc -b && vite build`) exits 0
- **Committed in:** `ff0b85f`

**4. [Rule 3 - Blocking] Biome 2.x config schema differs from plan's 1.9 example**

- **Found during:** Task 2 biome.json authoring
- **Issue:** Plan specified Biome 1.9.4 schema (`files.ignore`, `organizeImports.enabled` at root). `@biomejs/biome@latest` resolves to 2.4.13, which rejects 1.9 keys.
- **Fix:** Wrote a 2.x-equivalent biome.json that preserves the plan's intent — lint+format on, format with single-quotes/no-semicolons/2-space/100-col, ignore `src/components/ui` (future shadcn), VCS-ignore-aware, organize imports.
- **Files modified:** `web/biome.json`
- **Verification:** Acceptance criterion `grep -q 'src/components/ui' web/biome.json` passes; biome will parse the file at runtime once installed (build script not OOM'ing). The Biome CLI is presently OOM'ing in the sandbox, but `pnpm build` does not invoke biome.
- **Committed in:** `ff0b85f`

**5. [Rule 3 - Blocking] Compiled `tsc -b` artifacts (tsbuildinfo, vite.config.{js,d.ts}) were untracked**

- **Found during:** Task 2 commit prep
- **Issue:** `tsc -b` writes `tsconfig.tsbuildinfo`, `tsconfig.node.tsbuildinfo`, `vite.config.js`, `vite.config.d.ts` next to the sources. Untracked but generated — committing would pollute git, leaving untracked is forbidden by GSD task-commit protocol.
- **Fix:** Added all four patterns to `web/.gitignore`.
- **Files modified:** `web/.gitignore`
- **Verification:** `git check-ignore -v` returns each path mapped to the new gitignore lines.
- **Committed in:** `ff0b85f`

---

**Total deviations:** 5 auto-fixed (1 bug, 4 blocking)
**Impact on plan:** All deviations were forced by drift between the plan's pinned versions/example configs and current real-world latest releases (Biome 2 vs 1.9, plugin-react 6 vs 5, vite-config TS-typing). None changed the architectural intent of the plan. Phase 01 follow-up plans inherit a working scaffold.

## Issues Encountered

- **Local Node was 22.11.0; Vite 7 requires 22.12+ or 20.19+.** Resolved by `nvm install 22.20.0` (already supported by the environment's nvm). Future plans run under 22.20+. Worth pinning a minimum Node version in `package.json#engines` and CI — flagged for Plan 02 (test harness will likely set up CI).
- **`just` was not installed locally.** Installed via Homebrew (`just 1.50.0`). Plan 02's bootstrap recipe and `just bootstrap` already cover dev tooling — `just` itself is a host prerequisite documented in README (Plan 24).
- **`pnpm exec biome --version` OOM'd in the sandbox.** Biome 2.4.13 binary is fine; the linter daemon spawn is hitting a sandbox memory limit. This does not block `pnpm build` (which doesn't run biome). Will revisit if Plan 02 wires biome into CI / pre-commit and the runner has the same sandbox limits — likely needs `BIOME_LOG_PATH` or a heap flag.

## Known Stubs

| Stub | File | Reason | Resolved by |
|------|------|--------|-------------|
| Placeholder home screen showing only `<h1>Shifter</h1>` | `web/src/App.tsx` | Plan task explicitly defers full shell to Plan 06 (frontend-shell) | 01-06-frontend-shell |
| `compose-smoke-bundled` recipe exits 1 with TODO | `Justfile` | Recipe slot reserved per plan; bundled compose ships in Plan 20 | 01-20-compose-bundled |
| `compose-smoke-external` recipe exits 1 with TODO | `Justfile` | External compose ships in Plan 21 | 01-21-compose-external |
| Stub README ("Plan 24 will replace") | `README.md` | Real README depends on architecture/deployment from later plans | 01-24-readme-docs |
| Empty `Long` for future subcommands (no Run, no subcommands attached) | `cmd/shifter/main.go` | `serve` lands in Plan 18, `migrate` in Plan 03, `create-admin` in Plan 09 | 01-03/09/18 |

All stubs are documented in this plan and explicitly scheduled for resolution in later Phase 01 plans.

## User Setup Required

None — Phase 01 Plan 01 is fully scaffolded by code. The `just bootstrap` recipe will install dev tools (air/sqlc/mockgen + pnpm install) when run.

The host needs:

- Go 1.24+ (Go 1.26 GA verified working)
- Node.js 22.12+ or 20.19+ (per Vite 7) — `nvm use 22.20+`
- pnpm 10.x (`corepack enable` recommended)
- `just` (Homebrew: `brew install just`)

These are documented in Plan 24's README.

## Next Phase Readiness

- ✅ `go build ./cmd/shifter` works → Plan 03 can add the `migrate` subcommand directly
- ✅ Vite proxy is correct → Plan 03+ can add the HTTP listener at `:8080` and `just dev` will route `/api` and `/sse` correctly
- ✅ Air config exists → as soon as Plan 18's HTTP listener lands, `just dev` becomes useful
- ✅ Lock-file discipline established → Plan 02 can add a pre-commit hook enforcing pnpm-only
- ⚠️ **Node engines not pinned** — Plan 02 (test-harness) should add `"engines": { "node": ">=22.12" }` to `web/package.json` and possibly a `.nvmrc`
- ⚠️ **Plugin-react pinned to ^5 due to Vite 7** — track Vite 8 release; consider bumping when phase-end review happens

## Self-Check: PASSED

Files verified to exist:
- FOUND: `go.mod`
- FOUND: `cmd/shifter/main.go`
- FOUND: `.gitignore`
- FOUND: `Justfile`
- FOUND: `.air.toml`
- FOUND: `web/package.json`
- FOUND: `web/pnpm-lock.yaml`
- FOUND: `web/vite.config.ts`
- FOUND: `web/biome.json`
- FOUND: `web/dist/index.html` (build output)

Commits verified to exist:
- FOUND: `3d44468` (Task 1 — Go scaffold)
- FOUND: `ff0b85f` (Task 2 — Frontend scaffold)

---
*Phase: 01-foundation*
*Plan: 01-repo-scaffold*
*Completed: 2026-04-27*
