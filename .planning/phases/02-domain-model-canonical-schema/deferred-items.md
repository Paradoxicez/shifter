# Phase 02 — Deferred items

## Plan 02-14

- **Pre-existing biome lint OOM:** `pnpm --dir web lint` crashes with
  "Linter process terminated abnormally (possibly out of memory)" even
  with NODE_OPTIONS=--max-old-space-size=4096. Reproduced on a single
  existing file (`pnpm exec biome check ./src/lib/sites.ts`), so the
  failure is environmental / pre-existing, not introduced by Plan 02-14.
  Vitest + tsc + vite build all pass cleanly. Suggest a follow-up plan to
  upgrade biome or pin Node memory budget in the lint script.
  **Task 3 update (2026-05-04):** Same OOM reproduces when linting just
  the 7 Task-3 files in isolation (`npx biome check` on 7 paths) so this
  remains an environmental issue, not a Task-3 regression. Vitest 51/51,
  tsc clean, vite build clean.
