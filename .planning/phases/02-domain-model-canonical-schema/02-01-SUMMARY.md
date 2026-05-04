---
phase: 02-domain-model-canonical-schema
plan: 01
subsystem: scaffolding
tags: [wave-0, testing, testcontainers, react-table, validation, mosquitto, stub-then-fill]

requires:
  - phase: 01-foundation
    plan: 02
    provides: Wave 0 stub-then-fill convention + testcontainers helpers + vitest 4 config
  - phase: 02-domain-model-canonical-schema
    plan: 00 (research/context)
    provides: 02-VALIDATION.md per-task verification map (filled by orchestrator at fbd948c)
provides:
  - 20 backend *_test.go stubs across 10 packages — every Phase 2 source file has a paired skip-test
  - 5 frontend *.test.tsx stubs — every Phase 2 dialog/component has a paired it.skip stub
  - 9 new doc.go package declarations (site, meteringpoint, device, swap, ingest, resolver, profile, audit, testharness)
  - @tanstack/react-table 8.21.3 runtime dependency in web/package.json
  - Mosquitto testcontainer pin bumped 2.0.18 -> 2.0.20 (T-02-01-01) — Phase 1 helper, repinned for Phase 2 supply-chain hygiene
  - 02-VALIDATION.md flipped to wave_0_complete: true with all 26 stub-file checkboxes ticked
affects: [02-02, 02-03, 02-04, 02-05, 02-06, 02-07, 02-08, 02-09, 02-10]

tech-stack:
  added:
    - "@tanstack/react-table ^8.21.3 (frontend runtime dep — Phase 2 data-table UIs)"
  patterns:
    - "Wave 0 stub-then-fill (Phase 1 D-02 + Plan 01-02 SUMMARY): every Phase 2 plan EDITS one of these stubs in place; no plan is allowed to CREATE a new test file from scratch"
    - "TODO(plan-02-NN) markers in every stub file point at the implementing plan; greppable and one-per-file"
    - "Pinned testcontainer image tags only — no :latest (T-02-01-01)"

key-files:
  created:
    - internal/site/doc.go
    - internal/site/handlers_test.go
    - internal/meteringpoint/doc.go
    - internal/meteringpoint/handlers_test.go
    - internal/device/doc.go
    - internal/device/handlers_test.go
    - internal/device/deveui_test.go
    - internal/swap/doc.go
    - internal/swap/math_test.go
    - internal/swap/commit_test.go
    - internal/ingest/doc.go
    - internal/ingest/handler_test.go
    - internal/ingest/normalize_test.go
    - internal/resolver/doc.go
    - internal/resolver/cache_test.go
    - internal/resolver/listener_test.go
    - internal/profile/doc.go
    - internal/profile/editor_test.go
    - internal/profile/seed_test.go
    - internal/audit/doc.go
    - internal/audit/log_test.go
    - internal/audit/diff_test.go
    - internal/testharness/doc.go
    - internal/testharness/scenarios_test.go
    - internal/chirpstack/tenant_test.go
    - internal/chirpstack/application_test.go
    - internal/chirpstack/device_profile_test.go
    - internal/chirpstack/device_test.go
    - internal/chirpstack/bootstrap_test.go
    - web/src/routes/sites/create-site-dialog.test.tsx
    - web/src/routes/devices/add-device-dialog.test.tsx
    - web/src/routes/devices/deveui-parser.test.tsx
    - web/src/routes/metering-points/swap-meter-dialog.test.tsx
    - web/src/routes/profiles/mapping-editor.test.tsx
  modified:
    - web/package.json (+@tanstack/react-table)
    - web/pnpm-lock.yaml
    - internal/testsupport/mosquitto.go (image tag 2.0.18 -> 2.0.20, godoc refresh)
    - .planning/phases/02-domain-model-canonical-schema/02-VALIDATION.md (wave_0_complete flag flip + 26 stub checkboxes ticked + framework-install audit line)

key-decisions:
  - "Total stub count is 25 (20 backend + 5 frontend), not 26. The plan's task header reads '21 backend test stubs + 5 frontend' but the explicit mapping table only enumerates 20 backend files. Stuck to the explicit table; the 26 number in the plan's must_haves table refers to backend+frontend+mosquitto helper (20+5+1=26 line items in 02-VALIDATION.md Wave 0 section). All 26 checkboxes in VALIDATION.md are ticked."
  - "Existing internal/testsupport/mosquitto.go from Phase 1 was REPINNED rather than rewritten. The Phase 1 helper at eclipse-mosquitto:2.0.18 already implemented func StartMosquitto(t *testing.T) string with t.Cleanup, t.Fatalf error handling, and the no-auth listener pattern; the only Phase 2 ask was the version pin bump (T-02-01-01) and a godoc note listing Phase 2 callers. Rewriting from scratch would have invalidated existing chirpstack mqtt_test.go callers."
  - "VALIDATION.md per-task verification map left as orchestrator-authored (commit fbd948c) — 38 rows covering all 13 requirements were already in place and met every Task 3 acceptance criterion except wave_0_complete and the stub checkbox ticks. Only the disposition flags + framework-install audit line were edited."
  - "TestPlaceholder is the canonical stub function name when a file has only one stub. Files in the same package that need a second stub use TestPlaceholderXxx (e.g. TestPlaceholderDevice, TestPlaceholderListener) so go test does not collide on the same identifier within a package."

patterns-established:
  - "Pattern: Phase 2 stub bodies = `package <name>` + `import \"testing\"` + 4-line TODO(plan-02-NN) comment + `func TestPlaceholder*(t *testing.T) { t.Skip(\"Wave 0 placeholder — see TODO above\") }` — uniform shape across all 20 backend stubs makes future EDITs minimal-diff"
  - "Pattern: doc.go shape for placeholder Phase 2 packages is a single-line `// Package <name> — Phase 2 placeholder. See plan-02-NN for body.` followed by `package <name>` — zero-cost, gives go vet a Go source file to anchor on"

requirements-completed: []

duration: 6min
completed: 2026-05-04
---

# Phase 02 Plan 01: Wave 0 Stub Scaffolding Summary

**26 Wave-0 artifacts on disk: 20 backend test stubs across 10 packages, 5 frontend test stubs, plus a repinned eclipse-mosquitto:2.0.20 testcontainer helper. @tanstack/react-table 8.21.3 installed. 02-VALIDATION.md is now wave_0_complete: true with every checkbox ticked. Phase 2 plans 02-02..02-10 can each EDIT exactly one stub in place rather than CREATE a new test file.**

## Performance

- **Duration:** ~6 min
- **Started:** 2026-05-04T03:34:27Z
- **Completed:** 2026-05-04T03:40:06Z
- **Tasks:** 3 / 3
- **Files created:** 34
- **Files modified:** 4

## Accomplishments

- `pnpm add @tanstack/react-table` -> 8.21.3 in `web/package.json` `dependencies` (runtime, not devDep). Confirmed via `pnpm ls --depth 0` and `pnpm build`.
- Mosquitto testcontainer pin advanced from 2.0.18 to 2.0.20 in `internal/testsupport/mosquitto.go`. Godoc updated to list Phase 2 callers (testharness, ingest, resolver, profile). `go build ./internal/testsupport/...` clean.
- Created 9 new internal packages (site, meteringpoint, device, swap, ingest, resolver, profile, audit, testharness) — each with a `doc.go` and one or two `*_test.go` skip stubs.
- Created 5 frontend test stub files under new directories `web/src/routes/{sites,devices,metering-points,profiles}/`. All use `it.skip` with a `TODO(plan-02-NN)` marker.
- Added 5 new chirpstack stubs (tenant, application, device_profile, device, bootstrap) to the existing `internal/chirpstack` package — each pointing at plan-02-05.
- Flipped `wave_0_complete: false` -> `true` in 02-VALIDATION.md frontmatter and ticked all 26 Wave 0 stub-file checkboxes ([] -> [x]). Added a framework-install audit line for `@tanstack/react-table`.
- `go test -run TestPlaceholder` exits 0 across all 10 new/modified packages (skipped tests count as pass).
- `pnpm test:run` exits 0: 30 passed + 5 skipped (the 5 new frontend stubs).
- `pnpm build` succeeds — TanStack Table types resolve cleanly even though no consumer imports the library yet.

## Task Commits

1. **Task 1: Install @tanstack/react-table + bump mosquitto pin to 2.0.20** — `1ad01d0` (chore)
2. **Task 2: Create 25 stub test files + 9 doc.go declarations** — `5d96ae3` (test)
3. **Task 3: Flip 02-VALIDATION.md wave_0_complete + tick 26 stub checkboxes** — `2e2a588` (docs)

**Plan metadata commit:** _pending — created at end of plan_

## Stub File -> Implementing Plan Mapping

This is the canonical reference future Phase 2 plans use to know which stub to edit.

### Backend (`internal/`)

| File | Implementing plan (TODO marker) |
|------|---------------------------------|
| `internal/site/handlers_test.go` | plan-02-10 |
| `internal/meteringpoint/handlers_test.go` | plan-02-10 |
| `internal/device/handlers_test.go` | plan-02-10 |
| `internal/device/deveui_test.go` | plan-02-10 |
| `internal/swap/math_test.go` | plan-02-07 |
| `internal/swap/commit_test.go` | plan-02-07 |
| `internal/ingest/handler_test.go` | plan-02-09 |
| `internal/ingest/normalize_test.go` | plan-02-09 |
| `internal/resolver/cache_test.go` | plan-02-07 |
| `internal/resolver/listener_test.go` | plan-02-07 |
| `internal/profile/editor_test.go` | plan-02-08 |
| `internal/profile/seed_test.go` | plan-02-08 |
| `internal/audit/log_test.go` | plan-02-07 |
| `internal/audit/diff_test.go` | plan-02-07 |
| `internal/testharness/scenarios_test.go` | plan-02-15 (out-of-range marker per plan; actual implementer per VALIDATION.md is 02-09) |
| `internal/chirpstack/tenant_test.go` | plan-02-05 |
| `internal/chirpstack/application_test.go` | plan-02-05 |
| `internal/chirpstack/device_profile_test.go` | plan-02-05 |
| `internal/chirpstack/device_test.go` | plan-02-05 |
| `internal/chirpstack/bootstrap_test.go` | plan-02-05 |

### Frontend (`web/src/routes/`)

| File | Implementing plan (TODO marker) |
|------|---------------------------------|
| `sites/create-site-dialog.test.tsx` | plan-02-12 (out-of-range; actual: 02-10) |
| `devices/add-device-dialog.test.tsx` | plan-02-13 (out-of-range; actual: 02-10) |
| `devices/deveui-parser.test.tsx` | plan-02-13 (out-of-range; actual: 02-10) |
| `metering-points/swap-meter-dialog.test.tsx` | plan-02-14 (out-of-range; actual: 02-10) |
| `profiles/mapping-editor.test.tsx` | plan-02-14 (out-of-range; actual: 02-06) |

> **Note on out-of-range plan numbers:** the plan's mapping table cites plan numbers 02-12, 02-13, 02-14, 02-15 even though Phase 2 only has plans 02-01..02-10. The TODO markers were written verbatim per the plan's explicit instructions ("must match the implementing plan exactly"); the actual implementing plan in Phase 2 is whichever plan VALIDATION.md assigns to each requirement (e.g. SITE-01 -> 02-10, DATA-06 -> 02-09). Future plan-02-NN executors editing these stubs MUST update the TODO marker to their own plan number when filling in the body. This is a one-line plan-template inconsistency that the verifier should flag for plan-template hygiene; it does not affect the stub-then-fill mechanic.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Repinned existing mosquitto helper rather than rewriting**
- **Found during:** Task 1 read of `internal/testsupport/postgres.go` neighbors
- **Issue:** Plan 02-01 Task 1 Step B reads as if `internal/testsupport/mosquitto.go` does not exist and instructs to write it from scratch with `ContainerFile{...}` mount semantics. The file already exists from Phase 1 Plan 01-02 with a working `func StartMosquitto(t *testing.T) string` that uses the simpler `Cmd: []string{"mosquitto", "-c", "/mosquitto-no-auth.conf"}` approach (no bind-mount needed because the eclipse-mosquitto image ships the no-auth conf). Rewriting from scratch would have replaced a known-working helper that `internal/chirpstack/mqtt_test.go` already calls, with no functional gain.
- **Fix:** Updated only the pinned image tag (2.0.18 -> 2.0.20 per T-02-01-01) and refreshed the godoc to document the Phase 2 callers. Function signature and body shape preserved.
- **Files modified:** `internal/testsupport/mosquitto.go`
- **Commit:** `1ad01d0`

**2. [Rule 2 - Critical functionality] Acceptance criterion specifies `t.Cleanup(` literal — already present and preserved**
- **Found during:** Task 1 read of existing mosquitto.go
- **Issue:** Plan acceptance criterion #5 demands `internal/testsupport/mosquitto.go contains 't.Cleanup('`. The Phase 1 helper already had it; verified preserved through the edit.
- **Fix:** No fix needed — preserved by the targeted Edit (image-tag swap + godoc only).
- **Files modified:** none (preservation verified)
- **Commit:** n/a

### Plan-template inconsistency flagged (no auto-fix)

**1. [Rule 4 - Architectural — flagged, NOT actioned]** Plan-text frequency drift in TODO markers vs Phase 2 plan count
- The plan's mapping table cites `plan-02-12`, `plan-02-13`, `plan-02-14`, `plan-02-15` (frontend + testharness) as TODO marker values, but Phase 2 has only 10 plans (02-01..02-10).
- Per acceptance criterion #5 ("Every backend test file's TODO marker matches the implementing plan number from the table above"), the markers were written verbatim — `plan-02-15` appears in `internal/testharness/scenarios_test.go`, etc.
- This is a planner-side artifact (likely a copy-paste from a draft that anticipated more plans). It does not break the stub-then-fill mechanic — when a future plan edits one of these files, it will replace the marker with its own plan number.
- Did NOT auto-fix because the plan's acceptance grep `TODO(plan-02-` MUST find exactly one match per file, and inventing a different number than the plan instructed would have failed both the verbatim-mapping criterion and the grep. Preferred: log here for the verifier and the orchestrator's plan-template review.

## Authentication Gates

None — Wave 0 scaffolding is local-only filesystem + lockfile work; no external services were touched.

## Decisions Made

- **Stub style: `t.Skip("Wave 0 placeholder — see TODO above")` body + 4-line file-level TODO comment.** This mirrors the Phase 1 Plan 01-02 canonical shape exactly (file-level comment with implementing plan number; minimal `t.Skip` body). The implementing plan replaces the body wholesale; the comment doubles as a TODO marker that downstream plan-NN executors grep on.
- **One `TestPlaceholder` per single-stub file; `TestPlaceholderXxx` for multi-stub packages.** Go test names must be unique within a package. Files like `device/handlers_test.go` + `device/deveui_test.go` both live in `package device`, so the second one is `TestPlaceholderDevEUI`. Same for `swap` (commit -> `TestPlaceholderCommit`), `ingest` (normalize -> `TestPlaceholderNormalize`), etc. Implementing plans will rename to feature-specific test names — no harm.
- **Did not rewrite VALIDATION.md from scratch.** The orchestrator's pre-fill (commit `fbd948c`) already populated the per-task verification map (38 rows across 13 requirements) and met every Task 3 acceptance criterion except `wave_0_complete: true` and the 26 stub checkboxes. Targeted edits only — flag flip + checkbox ticks + framework-install audit line. Logged as decision because the plan's Task 3 reads as a from-scratch write.
- **Did not add a custom config bind-mount to mosquitto helper.** Plan 02-01 Task 1 Step B describes a `ContainerFile{Reader: strings.NewReader("listener 1883\\nallow_anonymous true\\n"), ContainerFilePath: "/mosquitto/config/mosquitto.conf", FileMode: 0o644}` mount. The Phase 1 helper uses `Cmd: []string{"mosquitto", "-c", "/mosquitto-no-auth.conf"}` instead, which is simpler (no inline file authoring) and works identically for tests because the eclipse-mosquitto image ships `/mosquitto-no-auth.conf` as a no-auth listener config. Preserved.

## Self-Check: PASSED

- `[x] internal/site/handlers_test.go` exists
- `[x] internal/meteringpoint/handlers_test.go` exists
- `[x] internal/device/handlers_test.go` exists
- `[x] internal/device/deveui_test.go` exists
- `[x] internal/swap/math_test.go` exists
- `[x] internal/swap/commit_test.go` exists
- `[x] internal/ingest/handler_test.go` exists
- `[x] internal/ingest/normalize_test.go` exists
- `[x] internal/resolver/cache_test.go` exists
- `[x] internal/resolver/listener_test.go` exists
- `[x] internal/profile/editor_test.go` exists
- `[x] internal/profile/seed_test.go` exists
- `[x] internal/audit/log_test.go` exists
- `[x] internal/audit/diff_test.go` exists
- `[x] internal/testharness/scenarios_test.go` exists
- `[x] internal/chirpstack/{tenant,application,device_profile,device,bootstrap}_test.go` (5) exist
- `[x] web/src/routes/sites/create-site-dialog.test.tsx` exists
- `[x] web/src/routes/devices/add-device-dialog.test.tsx` exists
- `[x] web/src/routes/devices/deveui-parser.test.tsx` exists
- `[x] web/src/routes/metering-points/swap-meter-dialog.test.tsx` exists
- `[x] web/src/routes/profiles/mapping-editor.test.tsx` exists
- `[x] @tanstack/react-table 8.21.3 in web/package.json + pnpm-lock.yaml + node_modules`
- `[x] internal/testsupport/mosquitto.go contains literal "eclipse-mosquitto:2.0.20"`
- `[x] internal/testsupport/mosquitto.go contains "t.Cleanup("`
- `[x] go build ./... clean`
- `[x] go test -run TestPlaceholder ./internal/{site,meteringpoint,device,swap,ingest,resolver,profile,audit,testharness,chirpstack}/... exits 0`
- `[x] pnpm test:run exits 0 (30 passed + 5 skipped)`
- `[x] pnpm build exits 0 (TanStack Table types resolve)`
- `[x] 02-VALIDATION.md frontmatter contains nyquist_compliant: true AND wave_0_complete: true`
- `[x] 02-VALIDATION.md has 0 template placeholders`
- `[x] All 26 Wave 0 stub-file checkboxes are [x]`
- `[x] All 6 Validation Sign-Off checkboxes are [x]`
- `[x] commit 1ad01d0 (Task 1) found in git log`
- `[x] commit 5d96ae3 (Task 2) found in git log`
- `[x] commit 2e2a588 (Task 3) found in git log`
