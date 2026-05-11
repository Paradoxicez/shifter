---
phase: 05-aggregates-reports-map-floor-plans
plan: 01
type: execute
wave: 0
depends_on: []
files_modified:
  - go.mod
  - go.sum
  - web/package.json
  - web/pnpm-lock.yaml
  - web/src/lib/pdfWorker.ts
  - internal/db/migrations/0024_river_tables.up.sql
  - internal/db/migrations/0024_river_tables.down.sql
  - internal/aggregate/aggregate_test.go
  - internal/report/csv_test.go
  - internal/report/excel_test.go
  - internal/report/pdf_test.go
  - internal/report/delta_test.go
  - internal/report/handlers_test.go
  - internal/report/pdf_worker_test.go
  - internal/floorplan/handlers_test.go
  - internal/floorplan/placement_test.go
  - internal/map/handler_test.go
  - internal/http/floorplan_static_test.go
  - internal/settings/retention_test.go
  - web/src/components/map/MapView.test.tsx
  - web/src/components/floor-plan/DevicePin.test.tsx
  - web/src/components/floor-plan/FloorPlanCanvas.test.tsx
  - web/src/lib/pdfToPng.test.ts
  - web/src/routes/reports/index.test.tsx
  - web/playwright/specs/reports-generate.spec.ts
  - web/playwright/specs/map-drill-down.spec.ts
  - web/playwright/specs/floor-plan-pinning.spec.ts
  - web/playwright/specs/site-drill-through.spec.ts
  - web/playwright/specs/floor-plan-health.spec.ts
  - web/playwright/specs/retention-settings.spec.ts
  - .planning/phases/05-aggregates-reports-map-floor-plans/05-VALIDATION.md
autonomous: true
requirements: [DATA-11, DATA-12, DATA-13, REPT-01, REPT-02, REPT-03, REPT-04, REPT-05, REPT-06, REPT-07, MAP-01, MAP-02, MAP-03, MAP-04, SITE-02, SITE-03, SITE-04, SITE-05, SITE-06]
threat_refs: [T-05-01-01, T-05-01-02]

must_haves:
  truths:
    - "Backend Go modules maroto/v2 v2.4.0 + river v0.36.0 + riverpgxv5 v0.36.0 resolve via go.mod"
    - "Frontend npm packages react-leaflet@5.0.0 + leaflet@1.9.4 + leaflet.markercluster@1.5.3 + react-leaflet-cluster@4.1.3 + pdfjs-dist@5.7.284 resolve via pnpm-lock.yaml"
    - "TypeScript types @types/leaflet@1.9.21 + @types/leaflet.markercluster@1.5.6 are devDependencies"
    - "River's database tables (river_job, river_leader, river_queue, river_migration) ship as golang-migrate file 0024_river_tables and round-trip in migrations_test"
    - "cumulative_delta column existence on measurement hypertable is verified before any CAGG plan touches the schema"
    - "Skeleton test files exist for every Phase 5 testable behavior listed in 05-VALIDATION.md and reference the right test names"
    - "Skeleton files compile (Go) / vitest-recognize (TS) but the body is t.Skip / it.skip so the suite stays green until the implementing plans land bodies"
    - "Playwright fixture file scaffolds (no body yet) exist for the 6 phase E2E specs"
    - "pdfjs-dist worker import-meta.url shim lives in web/src/lib/pdfWorker.ts so any consumer can `import './pdfWorker'` once"
    - "05-VALIDATION.md Per-Task Verification Map is fully populated — at least one row per plan/task across plans 02–12, with every row's Automated Command sourced from the plan's <automated> block; Wave 1 cannot begin until this map is real (Wave 0 hand-off)"
  artifacts:
    - path: "go.mod"
      provides: "Backend deps: maroto/v2, river, riverpgxv5"
      contains: "github.com/johnfercher/maroto/v2 v2.4.0"
    - path: "web/package.json"
      provides: "Frontend deps: react-leaflet, leaflet, leaflet.markercluster, react-leaflet-cluster, pdfjs-dist"
      contains: "\"react-leaflet\": \"5.0.0\""
    - path: "internal/db/migrations/0024_river_tables.up.sql"
      provides: "River schema embedded into golang-migrate sequence (Pitfall #8 mitigation)"
      contains: "CREATE TABLE IF NOT EXISTS river_job"
    - path: "internal/db/migrations/0024_river_tables.down.sql"
      provides: "Rollback path for River tables"
      contains: "DROP TABLE IF EXISTS river_job"
    - path: "web/src/lib/pdfWorker.ts"
      provides: "Single-source pdfjs-dist worker shim (Pitfall #3 mitigation)"
      contains: "pdfjs-dist/build/pdf.worker.min.mjs"
    - path: "internal/aggregate/aggregate_test.go"
      provides: "Skeleton TestCAGGHierarchy / TestRefreshPolicyParams / TestRetentionPolicy"
      contains: "t.Skip"
    - path: "internal/report/csv_test.go"
      provides: "Skeleton TestCSVFormat (BOM, ISO timestamps, comma)"
      contains: "TestCSVFormat"
    - path: "internal/report/excel_test.go"
      provides: "Skeleton TestExcelFormat (3 sheets, bold totals)"
      contains: "TestExcelFormat"
    - path: "internal/report/pdf_test.go"
      provides: "Skeleton TestPDFBranding (logo + address + page X of Y)"
      contains: "TestPDFBranding"
    - path: "internal/report/delta_test.go"
      provides: "Skeleton TestPeriodDelta (prior + YoY silent fallback)"
      contains: "TestPeriodDelta"
    - path: "internal/report/pdf_worker_test.go"
      provides: "Skeleton TestPDFJob (River InsertTx + worker.Work)"
      contains: "TestPDFJob"
    - path: "internal/floorplan/handlers_test.go"
      provides: "Skeleton TestImageUpload / TestMultiFloor / TestReplaceKeepsPins"
      contains: "TestImageUpload"
    - path: "internal/floorplan/placement_test.go"
      provides: "Skeleton TestPlacementCRUD / TestPlacementDecommissionInTx"
      contains: "TestPlacementCRUD"
    - path: "internal/map/handler_test.go"
      provides: "Skeleton TestMapData / TestOSMTileURL"
      contains: "TestMapData"
    - path: "internal/http/floorplan_static_test.go"
      provides: "Skeleton TestFloorPlanImageAuthGated"
      contains: "TestFloorPlanImageAuthGated"
    - path: "internal/settings/retention_test.go"
      provides: "Skeleton TestRetentionConfigCRUD"
      contains: "TestRetentionConfigCRUD"
    - path: "web/src/components/map/MapView.test.tsx"
      provides: "Skeleton MapView render + cluster + Bangkok fallback"
      contains: "describe('MapView'"
    - path: "web/src/components/floor-plan/DevicePin.test.tsx"
      provides: "Skeleton DevicePin state colors"
      contains: "describe('DevicePin'"
    - path: "web/src/components/floor-plan/FloorPlanCanvas.test.tsx"
      provides: "Skeleton FloorPlanCanvas click → fractional coords"
      contains: "describe('FloorPlanCanvas'"
    - path: "web/src/lib/pdfToPng.test.ts"
      provides: "Skeleton pdfToPng 150 DPI render"
      contains: "describe('pdfToPng'"
    - path: "web/src/routes/reports/index.test.tsx"
      provides: "Skeleton ReportsPage config → result panel state machine"
      contains: "describe('ReportsPage'"
    - path: "web/playwright/specs/reports-generate.spec.ts"
      provides: "Skeleton E2E reports flow"
      contains: "test.skip"
    - path: "web/playwright/specs/map-drill-down.spec.ts"
      provides: "Skeleton E2E map → site"
      contains: "test.skip"
    - path: "web/playwright/specs/floor-plan-pinning.spec.ts"
      provides: "Skeleton E2E upload → pin → reload → persist"
      contains: "test.skip"
    - path: "web/playwright/specs/site-drill-through.spec.ts"
      provides: "Skeleton E2E SITE-06 2-click path"
      contains: "test.skip"
    - path: "web/playwright/specs/floor-plan-health.spec.ts"
      provides: "Skeleton E2E SSE marker state update"
      contains: "test.skip"
    - path: "web/playwright/specs/retention-settings.spec.ts"
      provides: "Skeleton E2E retention edit"
      contains: "test.skip"
    - path: ".planning/phases/05-aggregates-reports-map-floor-plans/05-VALIDATION.md"
      provides: "Per-Task Verification Map populated by Task 3 — one row per task across plans 02–12 with Automated Command sourced from each plan's <automated> block"
      contains: "| 05-"
  key_links:
    - from: "internal/db/migrations/0024_river_tables.up.sql"
      to: "river_job + river_leader + river_queue + river_migration tables"
      via: "golang-migrate sequence"
      pattern: "CREATE TABLE IF NOT EXISTS river_(job|leader|queue|migration)"
    - from: "web/src/lib/pdfWorker.ts"
      to: "pdfjs-dist build/pdf.worker.min.mjs"
      via: "import.meta.url"
      pattern: "new URL.*pdf\\.worker\\.min\\.mjs.*import\\.meta\\.url"
    - from: ".planning/phases/05-aggregates-reports-map-floor-plans/05-VALIDATION.md Per-Task Verification Map"
      to: "Each PLAN.md <task><verify><automated> block"
      via: "Task 3 step E reads each plan and emits one row per task"
      pattern: "\\| 05-\\d{2}-\\d{2} \\|"
---

<objective>
Wave 0 ground-zero plan: install every new dependency, embed River's schema as a golang-migrate file (avoids Pitfall #8 — `river migrate-up` not on the install runtime path), establish the pdf.js worker shim once (avoids Pitfall #3 — every consumer paying the v5 ESM-ism tax), verify the load-bearing `cumulative_delta` invariant against existing measurement schema and ingest code, and land skeleton test files for every Phase 5 testable requirement so subsequent plans flip `t.Skip` to body without touching file paths. Also populates `05-VALIDATION.md`'s Per-Task Verification Map with one row per task across plans 02–12 — this map is the Wave 0 → Wave 1 hand-off gate.

Purpose: Establish the test surface and dep baseline so plans 02–11 can implement freely. Pre-checks the open RESEARCH question on `cumulative_delta` column existence — if missing, this plan adds the migration that computes/persists it before any CAGG plan touches the schema. The populated validation map gives the orchestrator a real per-task verification ledger for sampling during execution.

Output: Updated go.mod / package.json with verified versions, a single River-schema migration (0024_river_tables) inside the golang-migrate sequence, the pdfjs worker shim, ~27 skeleton test files + 6 Playwright specs that name every behavior plans 02–11 must implement, AND a populated `05-VALIDATION.md` Per-Task Verification Map (one row per task across plans 02–12).
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/ROADMAP.md
@.planning/STATE.md
@.planning/REQUIREMENTS.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-CONTEXT.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-RESEARCH.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-UI-SPEC.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-VALIDATION.md
@internal/db/migrations/0015_measurement.up.sql
@internal/db/migrations/0021_measurement_inserted_trigger.up.sql
@internal/db/migrations/0023_device_profile_expected_interval.up.sql
@internal/ingest/persist.go
@internal/events/hub.go
@go.mod
@web/package.json

<interfaces>
<!-- Existing measurement column set (from 0015_measurement.up.sql) -->
```
measurement (
  time TIMESTAMPTZ NOT NULL,
  metering_point_id UUID NOT NULL,
  raw_value NUMERIC,
  cumulative_value NUMERIC,   -- absolute meter reading, persisted
  instant_value NUMERIC,
  battery_pct SMALLINT,
  rssi SMALLINT,
  snr REAL,
  ...
  quality TEXT NOT NULL DEFAULT 'ok',
  ...
)
```

<!-- KEY FINDING: there is NO cumulative_delta column. CAGG plans (05-02) must either
     add a stored generated column or use LAG() in the CAGG SELECT.
     This plan's Task 0 confirms the absence in writing so 05-02 can decide. -->

<!-- Existing measurement_inserted NOTIFY payload (from 0021 trigger):
     (metering_point_id, time, cumulative_value, instant_value, quality, battery_pct, rssi)
     site_id is NOT in the payload — floor-plan health markers must compute state
     client-side from device data + measurement payload, per RESEARCH §Hub Extension Option B. -->
```

<!-- Existing events.Hub topic conventions (from internal/events/hub.go) -->
```go
// dashboard:global         — every measurement_inserted
// mp:<uuid>                — per-MP latest reading
// mp:<uuid>:uplinks        — per-MP uplinks log
//
// Phase 5 ADDS no new topics — floor-plan markers piggy-back on mp:<uuid>.
```
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: cumulative_delta pre-check + Go + frontend dep install</name>
  <read_first>
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-CONTEXT.md (Claude's Discretion — PDF/job/marker tunables)
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-RESEARCH.md §Standard Stack §Open Questions #1 (cumulative_delta verification)
    - internal/db/migrations/0015_measurement.up.sql (canonical column set — no cumulative_delta)
    - internal/ingest/persist.go (confirm ingest writes cumulative_value, not cumulative_delta)
    - go.mod (current Go deps baseline)
    - web/package.json (current frontend deps baseline)
  </read_first>
  <behavior>
    - Test 1 (smoke): `go build ./...` exits 0 after `go get` of three new modules
    - Test 2 (smoke): `pnpm --dir web build` exits 0 after `pnpm add` of five new deps and two devDeps
    - Test 3 (frontmatter): a one-line marker file `internal/aggregate/CUMULATIVE_DELTA.md` records the verification verdict so plan 05-02 reads it before writing CAGG SQL
  </behavior>
  <action>
**Step A — cumulative_delta pre-check (RESEARCH §Open Questions #1):**

1. `grep -n "cumulative_delta" internal/db/migrations/*.sql internal/ingest/*.go internal/db/queries/*.sql 2>&1 | tee /tmp/cdelta-grep.txt`
2. If grep finds zero matches (expected per current read), create `internal/aggregate/CUMULATIVE_DELTA.md` with body:

```markdown
# cumulative_delta Pre-Check (Plan 05-01 Task 1)

**Verdict (2026-05-12):** ABSENT. `measurement` hypertable stores `cumulative_value` (absolute reading) but not the per-uplink delta.

**Implication for plan 05-02 (CAGG migrations):** The hourly CAGG MUST compute the delta inline. Two options:

1. Add a stored generated/computed `cumulative_delta` column in a Phase 5 pre-CAGG migration (0025 or similar), backfilled from existing rows via window function.
2. Compute via `cumulative_value - LAG(cumulative_value) OVER (PARTITION BY metering_point_id ORDER BY time)` directly in the hourly CAGG SELECT.

**Recommendation:** Option 2. The CAGG is the single producer of `cumulative_delta` for the rest of Phase 5 (daily/monthly/yearly all sum it from the hourly view). Option 1 would force a backfill window on existing rows and a generated-column constraint on every future INSERT.

**Caveat:** Option 2 introduces a per-MP boundary issue at the first uplink in each hourly bucket — `LAG()` across bucket boundaries needs `OVER (PARTITION BY metering_point_id ORDER BY time)` (not bucketed) so the delta of the first row in a bucket subtracts the last row of the previous bucket. Plan 05-02 implements this verbatim.

**No phase-blocking action required;** plan 05-02 owns the implementation.
```

3. `git add internal/aggregate/CUMULATIVE_DELTA.md`

**Step B — Backend deps (go.mod):**

```bash
go get github.com/johnfercher/maroto/v2@v2.4.0
go get github.com/riverqueue/river@v0.36.0
go get github.com/riverqueue/river/riverdriver/riverpgxv5@v0.36.0
go mod tidy
go build ./...
```

**Step C — Frontend deps (web/package.json):**

```bash
cd web
pnpm add react-leaflet@5.0.0 leaflet@1.9.4 leaflet.markercluster@1.5.3 react-leaflet-cluster@4.1.3 pdfjs-dist@5.7.284
pnpm add -D @types/leaflet@1.9.21 @types/leaflet.markercluster@1.5.6
pnpm build
cd ..
```

(If `pnpm add` complains about peer ranges, append `--strict-peer-dependencies=false` — react-leaflet-cluster's peer ranges include `react@^19.0.0` which we satisfy, but transitive cluster types can drift. NEVER bump or downgrade the pinned versions to "fix" peer warnings — pin verbatim per RESEARCH §Standard Stack.)

**Step D — Verify versions land:**

`grep -E "(maroto/v2|riverqueue/river|leaflet|pdfjs-dist|react-leaflet)" go.mod web/package.json | sort -u`

Should print exactly:
- `github.com/johnfercher/maroto/v2 v2.4.0`
- `github.com/riverqueue/river v0.36.0`
- `github.com/riverqueue/river/riverdriver/riverpgxv5 v0.36.0`
- `"leaflet": "1.9.4"`
- `"leaflet.markercluster": "1.5.3"`
- `"pdfjs-dist": "5.7.284"`
- `"react-leaflet": "5.0.0"`
- `"react-leaflet-cluster": "4.1.3"`
- `"@types/leaflet": "1.9.21"`
- `"@types/leaflet.markercluster": "1.5.6"`
  </action>
  <verify>
    <automated>go build ./... &amp;&amp; pnpm --dir web build &amp;&amp; grep -q "maroto/v2 v2.4.0" go.mod &amp;&amp; grep -q "react-leaflet.*5\\.0\\.0" web/package.json &amp;&amp; test -f internal/aggregate/CUMULATIVE_DELTA.md</automated>
  </verify>
  <acceptance_criteria>
    - `go.mod` contains exactly `github.com/johnfercher/maroto/v2 v2.4.0`
    - `go.mod` contains exactly `github.com/riverqueue/river v0.36.0`
    - `go.mod` contains exactly `github.com/riverqueue/river/riverdriver/riverpgxv5 v0.36.0`
    - `web/package.json` contains exactly `"react-leaflet": "5.0.0"`, `"leaflet": "1.9.4"`, `"leaflet.markercluster": "1.5.3"`, `"react-leaflet-cluster": "4.1.3"`, `"pdfjs-dist": "5.7.284"`
    - `web/package.json` devDependencies contain `"@types/leaflet": "1.9.21"` and `"@types/leaflet.markercluster": "1.5.6"`
    - `internal/aggregate/CUMULATIVE_DELTA.md` exists and contains the literal string `Verdict (2026-05-12): ABSENT`
    - `go build ./...` exits 0
    - `pnpm --dir web build` exits 0
    - NO BANNED packages added: `grep -E "(gofpdf|chromedp|maplibre|gorm)" go.mod web/package.json` returns empty
  </acceptance_criteria>
  <done>All backend + frontend deps install, cumulative_delta verdict file exists, no banned deps.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: River schema as golang-migrate file 0024 + pdf.js worker shim</name>
  <read_first>
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-RESEARCH.md §River — Background Job Queue Setup §Common Pitfalls #8
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-RESEARCH.md §pdf.js (pdfjs-dist) §Common Pitfalls #3
    - internal/db/migrations/0023_device_profile_expected_interval.up.sql (last applied migration; new one is 0024)
    - internal/db/migrations_test.go (round-trip assertion pattern)
  </read_first>
  <behavior>
    - Test 1: After `go run github.com/riverqueue/river/cmd/river@v0.36.0 migrate-up --line main --database-url ${DATABASE_URL_TEST}` schema is recorded → diff that schema → embed as 0024 SQL
    - Test 2: golang-migrate round-trip (up → down → up) on 0024 leaves zero river_* tables after down and all four after up
    - Test 3: `web/src/lib/pdfWorker.ts` exports a side-effect-only initializer that sets `pdfjsLib.GlobalWorkerOptions.workerSrc` exactly once using `new URL('pdfjs-dist/build/pdf.worker.min.mjs', import.meta.url).toString()`
  </behavior>
  <action>
**Step A — Generate River migration SQL by running River's CLI against a throwaway testcontainer Postgres:**

1. Spin up a clean testcontainer Postgres (use the same TimescaleDB image used by `internal/db/migrations_test.go`, OR plain `postgres:16-alpine` — River's tables don't need TimescaleDB).
2. Run `go run github.com/riverqueue/river/cmd/river@v0.36.0 migrate-up --line main --database-url "${TC_URL}"`.
3. Run `pg_dump -s -t 'river_*' "${TC_URL}"` to capture the schema-only dump.
4. Strip pg_dump preamble (SET statements, OWNER TO clauses, search_path) — keep only CREATE TABLE / CREATE INDEX / CREATE TYPE / CREATE FUNCTION statements.
5. Wrap in idempotent `CREATE TABLE IF NOT EXISTS ...` / `CREATE INDEX IF NOT EXISTS ...` form.

Write the result to `internal/db/migrations/0024_river_tables.up.sql` with header:

```sql
-- 0024_river_tables.up.sql
-- River v0.36.0 job-queue schema, embedded into golang-migrate so the install
-- runs a single migration tool (Pitfall #8 mitigation per 05-RESEARCH).
--
-- Generated by capturing `river migrate-up --line main` against a clean Postgres
-- and converting to idempotent CREATE … IF NOT EXISTS form.
--
-- DO NOT hand-edit. To upgrade River:
--   1. Bump river versions in go.mod
--   2. Re-run the capture procedure documented in 05-01-PLAN.md Task 2
--   3. Land the diff as 0025_river_v0_NN_upgrade.up.sql (NOT this file)

CREATE TABLE IF NOT EXISTS river_migration (
  line text PRIMARY KEY,
  version bigint NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);

-- (full River schema follows — verbatim from pg_dump capture, converted to IF NOT EXISTS)
-- river_job, river_leader, river_queue, river_client, river_client_queue, plus
-- type river_job_state and any indexes / functions River creates.

INSERT INTO river_migration (line, version) VALUES ('main', 6)  -- River v0.36 schema version
  ON CONFLICT (line) DO NOTHING;
```

And `internal/db/migrations/0024_river_tables.down.sql`:

```sql
-- 0024_river_tables.down.sql
-- Drop River job-queue tables. Idempotent.

DROP TABLE IF EXISTS river_client_queue CASCADE;
DROP TABLE IF EXISTS river_client CASCADE;
DROP TABLE IF EXISTS river_job CASCADE;
DROP TABLE IF EXISTS river_queue CASCADE;
DROP TABLE IF EXISTS river_leader CASCADE;
DROP TABLE IF EXISTS river_migration CASCADE;
DROP TYPE IF EXISTS river_job_state CASCADE;
```

(Capture-and-paste the exact statements from step 3–5; do NOT type them from memory.)

**Step B — pdf.js worker shim (Pitfall #3 mitigation):**

Create `web/src/lib/pdfWorker.ts` (verbatim):

```ts
// pdfjs-dist v5 is ESM-only and dropped the UMD pdf.worker.min.js build. The
// only Vite-compatible way to load the worker is via the import.meta.url URL
// pattern — letting Vite emit the worker as a build asset. Import this file
// ONCE at the entrypoint of every component that calls pdfjsLib.getDocument()
// (or import it from a shared "pdfToPng" helper that imports this).
//
// Documented in Phase 5 RESEARCH §pdf.js + §Common Pitfalls #3.

import * as pdfjsLib from 'pdfjs-dist'

pdfjsLib.GlobalWorkerOptions.workerSrc = new URL(
  'pdfjs-dist/build/pdf.worker.min.mjs',
  import.meta.url,
).toString()

export { pdfjsLib }
```

**Step C — Migrations round-trip test wiring:**

Update `internal/db/migrations_test.go` so the existing `TestRunMigrations_Clean` table picks up the new 0024 file automatically (the test iterates the filesystem). Add ONE new assertion to the same test:

```go
// 0024 River tables: verify river_migration row exists with line='main'
var version int64
err = pool.QueryRow(ctx, `SELECT version FROM river_migration WHERE line = 'main'`).Scan(&version)
require.NoError(t, err)
require.Equal(t, int64(6), version, "river_migration version must be 6 (v0.36 schema)")
```

Add a separate round-trip test `TestRunMigrations_RiverDownUpClean` that explicitly runs migrate-down through 0024, asserts `river_job` does NOT exist (via `to_regclass`), then runs migrate-up and asserts it does.
  </action>
  <verify>
    <automated>go test ./internal/db/... -race -count=1 -run "TestRunMigrations|TestRunMigrations_RiverDownUpClean" -short=false &amp;&amp; grep -q "import.meta.url" web/src/lib/pdfWorker.ts</automated>
  </verify>
  <acceptance_criteria>
    - `internal/db/migrations/0024_river_tables.up.sql` exists and contains literal `CREATE TABLE IF NOT EXISTS river_job`, `CREATE TABLE IF NOT EXISTS river_leader`, `CREATE TABLE IF NOT EXISTS river_queue`, `CREATE TABLE IF NOT EXISTS river_migration`
    - `internal/db/migrations/0024_river_tables.up.sql` contains literal `INSERT INTO river_migration (line, version) VALUES ('main', 6)`
    - `internal/db/migrations/0024_river_tables.down.sql` exists and contains `DROP TABLE IF EXISTS river_job CASCADE`
    - `internal/db/migrations_test.go` contains literal `TestRunMigrations_RiverDownUpClean`
    - `web/src/lib/pdfWorker.ts` exists and contains literal `new URL('pdfjs-dist/build/pdf.worker.min.mjs', import.meta.url)`
    - `web/src/lib/pdfWorker.ts` contains literal `pdfjsLib.GlobalWorkerOptions.workerSrc =`
    - `go test ./internal/db/... -race -count=1 -short=false -run "TestRunMigrations" ` exits 0
    - NO hand-typed River schema invented (capture procedure documented in Task action; reviewer can re-run to verify)
  </acceptance_criteria>
  <done>River schema embedded as 0024 (round-trips clean), pdf.js worker shim exists, no Pitfall #3 traps remain.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 3: Skeleton test files for every Phase 5 testable behavior + populate 05-VALIDATION.md Per-Task Verification Map</name>
  <read_first>
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-VALIDATION.md (per-subsystem test ladders + Per-Task Verification Map template + Wave 0 Gaps list)
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-RESEARCH.md §Phase Requirements → Test Map
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-02-cagg-hierarchy-retention-PLAN.md (read each plan's <task><verify><automated> blocks — source of truth for the map)
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-03-reports-backend-csv-excel-PLAN.md
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-04-map-backend-PLAN.md
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-05-floor-plan-schema-upload-PLAN.md
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-06-reports-pdf-river-worker-PLAN.md
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-07-floor-plan-placement-decommission-PLAN.md
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-08-map-frontend-gw04-picker-PLAN.md
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-09-reports-frontend-PLAN.md
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-10-floor-plan-frontend-PLAN.md
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-11-settings-data-retention-PLAN.md
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-12-phase-closure-PLAN.md
    - .planning/phases/04-realtime-dashboard/04-01-PLAN.md (skeleton-test pattern, t.Skip with descriptive plan ref)
    - internal/events/hub_test.go (reference: existing skeleton style for sub-package tests)
    - web/src/routes/dashboard.test.tsx (reference: existing vitest skeleton style)
  </read_first>
  <behavior>
    - Test 1: Every skeleton Go file compiles (`go build ./...` exits 0)
    - Test 2: Every skeleton vitest file `pnpm --dir web test:run` discovers and skips (not fails)
    - Test 3: Every Playwright spec `pnpm --dir web exec playwright test --list` lists the test (`.skip` annotated)
    - Test 4: `05-VALIDATION.md` Per-Task Verification Map has at least one row per task across plans 02–12; each row's `Automated Command` column is the literal text from that plan's `<automated>` block (or `Wave 0` when the task is this Wave 0 plan itself); each row's `File Exists` column is ✅ if the relevant test file is in this plan's `files_modified`, ❌ W0 otherwise
  </behavior>
  <action>
Create the following 22 skeleton files. EVERY skeleton uses `t.Skip("Plan 05-XX Task Y will implement")` / `it.skip(...)` / `test.skip(...)` so the suite stays green.

**Go skeletons (one `package` declaration + one or more named tests with t.Skip):**

1. `internal/aggregate/aggregate_test.go`:
```go
package aggregate

import "testing"

// Plan 05-02 owns the CAGG hierarchy.
func TestCAGGHierarchy(t *testing.T)        { t.Skip("Plan 05-02 Task 1: hourly→daily→monthly→yearly chain") }
func TestRefreshPolicyParams(t *testing.T)  { t.Skip("Plan 05-02 Task 2: end_offset >= 2 × expected_interval_s; start_offset <= raw_retention") }
func TestRetentionPolicy(t *testing.T)      { t.Skip("Plan 05-02 Task 3: per-CAGG retention defaults match D-09") }
```

2. `internal/report/csv_test.go`, `excel_test.go`, `pdf_test.go`, `delta_test.go`, `pdf_worker_test.go`, `handlers_test.go` (one file each, package `report`):
```go
package report

import "testing"

func TestCSVFormat(t *testing.T)        { t.Skip("Plan 05-03 Task 2: BOM + ISO timestamps + comma + timezone header") }
func TestExcelFormat(t *testing.T)      { t.Skip("Plan 05-03 Task 3: 3-sheet structure, bold totals row") }
func TestPDFBranding(t *testing.T)      { t.Skip("Plan 05-06 Task 1: logo + display_name + address header; 'page N of M' footer") }
func TestPeriodDelta(t *testing.T)      { t.Skip("Plan 05-03 Task 1: vs prior period always; YoY when data; silent fallback") }
func TestPDFJob(t *testing.T)           { t.Skip("Plan 05-06 Task 2: River InsertTx + worker.Work + file landed at expected path") }
func TestStatusHandler_PendingReadyFailed(t *testing.T) { t.Skip("Plan 05-06 Task 2: GET /api/reports/:id returns pdf_status JSON for poll") }
func TestReportScopeGrouping(t *testing.T) { t.Skip("Plan 05-03 Task 1: all/site/meter + group=site/category") }
```

(Split into the six files listed in `files_modified` so each plan can add bodies independently.)

3. `internal/floorplan/handlers_test.go` + `placement_test.go` (package `floorplan`):
```go
package floorplan

import "testing"

func TestImageUpload(t *testing.T)              { t.Skip("Plan 05-05 Task 2: 10MB cap, dim probe, MIME sniff, PNG+JPG only") }
func TestMultiFloor(t *testing.T)               { t.Skip("Plan 05-05 Task 1: multiple floor_plan rows ordered by sort_order") }
func TestReplaceKeepsPins(t *testing.T)         { t.Skip("Plan 05-05 Task 3: PATCH image keeps device_floor_plan_placement rows") }
func TestPlacementCRUD(t *testing.T)            { t.Skip("Plan 05-07 Task 1: x_frac/y_frac clamped to [0,1]; CRUD via PATCH") }
func TestPlacementDecommissionInTx(t *testing.T) { t.Skip("Plan 05-07 Task 2: device soft-delete also DELETEs placement in same tx + audit") }
```

4. `internal/map/handler_test.go` (package `mapapi` — Go reserves `map` as keyword, dir name `internal/map/` but Go pkg name `mapapi`):
```go
package mapapi

import "testing"

func TestMapData(t *testing.T)    { t.Skip("Plan 05-04 Task 1: returns sites + gateways with lat/lng + summary per D-15") }
func TestOSMTileURL(t *testing.T) { t.Skip("Plan 05-04 Task 1: response/config asserts OSM tile URL, no paid API key — MAP-04") }
```

5. `internal/http/floorplan_static_test.go` (package `http_test` to live alongside router tests):
```go
package http_test

import "testing"

func TestFloorPlanImageAuthGated(t *testing.T) { t.Skip("Plan 05-07 Task 3: GET /api/floor-plans/:id/image is auth-gated; UUID validated; path traversal mitigated") }
```

6. `internal/settings/retention_test.go` (package `settings`):
```go
package settings

import "testing"

func TestRetentionConfigCRUD(t *testing.T) { t.Skip("Plan 05-11 Task 1: PATCH /api/settings/retention writes retention_config; range guarded per UI-SPEC; pgx-only (no lib/pq)") }
```

**Frontend vitest skeletons:**

7. `web/src/components/map/MapView.test.tsx`:
```tsx
import { describe, it } from 'vitest'

describe('MapView', () => {
  it.skip('renders OSM tile layer with leaflet CSS imports loaded (MAP-01) — Plan 05-08', () => {})
  it.skip('auto-fits bounds of sites + gateways (D-13) — Plan 05-08', () => {})
  it.skip('falls back to Bangkok [13.7563, 100.5018] zoom 5 when 0 markers (D-13) — Plan 05-08', () => {})
  it.skip('clusters markers above ~50 (MAP-02) — Plan 05-08', () => {})
})
```

8. `web/src/components/floor-plan/DevicePin.test.tsx`:
```tsx
import { describe, it } from 'vitest'

describe('DevicePin', () => {
  it.skip('renders green for healthy state (D-22) — Plan 05-10', () => {})
  it.skip('renders yellow for warning state (D-22) — Plan 05-10', () => {})
  it.skip('renders red for offline state (D-22) — Plan 05-10', () => {})
  it.skip('positions via left:%/top:% from x_frac/y_frac — Plan 05-10', () => {})
})
```

9. `web/src/components/floor-plan/FloorPlanCanvas.test.tsx`:
```tsx
import { describe, it } from 'vitest'

describe('FloorPlanCanvas', () => {
  it.skip('click → fractional coords math (xFrac = (clientX-rect.left)/rect.width) — Plan 05-10', () => {})
  it.skip('drag updates fractional coords on pointerup, clamped to [0,1] — Plan 05-10', () => {})
  it.skip('right-click triggers RemovePinAlertDialog (D-20) — Plan 05-10', () => {})
})
```

10. `web/src/lib/pdfToPng.test.ts`:
```ts
import { describe, it } from 'vitest'

describe('pdfToPng', () => {
  it.skip('renders PDF page 1 to 150-DPI canvas and exports PNG blob (D-17) — Plan 05-10', () => {})
  it.skip('uses pdfWorker shim with import.meta.url — Plan 05-10', () => {})
  it.skip('releases page + document memory after blob export — Plan 05-10', () => {})
})
```

11. `web/src/routes/reports/index.test.tsx`:
```tsx
import { describe, it } from 'vitest'

describe('ReportsPage', () => {
  it.skip('shows config panel with 3-radio scope picker on mount (D-05) — Plan 05-09', () => {})
  it.skip('Generate replaces config panel with result panel + 3 download tiles (D-06) — Plan 05-09', () => {})
  it.skip('PDF tile shows spinner until job completes; toast fires on ready (D-06) — Plan 05-09', () => {})
  it.skip('navigating away loses result panel (D-07) — Plan 05-09', () => {})
  it.skip('shows empty state when zero sites — UI-SPEC §Empty States — Plan 05-09', () => {})
})
```

**Playwright skeletons (one `test.describe` block + named `test.skip` per scenario):**

12. `web/playwright/specs/reports-generate.spec.ts`:
```ts
import { test } from '@playwright/test'

test.describe('Reports — generate-once flow', () => {
  test.skip('login → /reports → configure single-meter monthly → Generate → download CSV + Excel + PDF', async ({ page }) => {})
})
```

13. `web/playwright/specs/map-drill-down.spec.ts`:
```ts
import { test } from '@playwright/test'

test.describe('Map — drill-down (MAP-03)', () => {
  test.skip('click site marker → popup → "View site" → /sites/:id', async ({ page }) => {})
  test.skip('click cluster → zoom to cluster bounds (D-14)', async ({ page }) => {})
})
```

14. `web/playwright/specs/floor-plan-pinning.spec.ts`:
```ts
import { test } from '@playwright/test'

test.describe('Floor plan — pin lifecycle (SITE-04)', () => {
  test.skip('upload PNG → place 3 devices → reload → pins persist at same fractional positions', async ({ page }) => {})
  test.skip('decommission one device → its pin disappears from plan (D-25)', async ({ page }) => {})
})
```

15. `web/playwright/specs/site-drill-through.spec.ts`:
```ts
import { test } from '@playwright/test'

test.describe('Site click-through (SITE-06)', () => {
  test.skip('map → site marker popup → "View site" → /sites/:id defaults to Floor plan tab when plans exist (D-23)', async ({ page }) => {})
})
```

16. `web/playwright/specs/floor-plan-health.spec.ts`:
```ts
import { test } from '@playwright/test'

test.describe('Floor plan — live marker state (SITE-05)', () => {
  test.skip('SSE measurement → marker re-tints from green to yellow on battery drop (D-22)', async ({ page }) => {})
})
```

17. `web/playwright/specs/retention-settings.spec.ts`:
```ts
import { test } from '@playwright/test'

test.describe('Settings — Data Retention (D-09 / DATA-13)', () => {
  test.skip('admin edits raw retention 90→60 days → CAGG refresh policy reflects new bound', async ({ page }) => {})
  test.skip('viewer sees read-only retention values, no Edit button (AUTH-06 frontend hide)', async ({ page }) => {})
})
```

**Step E — Populate the Per-Task Verification Map in `05-VALIDATION.md`:**

This is the Wave 0 → Wave 1 hand-off gate. Walk every PLAN.md (05-02 through 05-12) and emit one row per `<task>` block. The map template already exists in `05-VALIDATION.md` under `## Per-Task Verification Map`.

Procedure (DO follow this verbatim — the orchestrator depends on the column shape):

1. For each plan file `05-{02..12}-*-PLAN.md`:
   a. Read `wave` and `requirements` from the frontmatter.
   b. Read `threat_refs` if present.
   c. For each `<task>` block in the plan body:
      - Note the task name from `<name>`.
      - Extract the literal content of `<verify><automated>...</automated></verify>` — this becomes the `Automated Command` column (verbatim, do NOT paraphrase).
      - Map the task to one or more requirement IDs from the plan's `requirements` frontmatter (best-effort — if the task explicitly addresses one REQ, use that; otherwise use the plan-level set).
      - Map the task to a threat ref from `threat_refs` if applicable; otherwise `—`.
      - Determine `Test Type`: `unit` (Go *_test.go inside a single package), `integration` (testcontainer / cross-package / DB-required), or `e2e` (Playwright spec).
      - Determine `File Exists`: `✅` if the relevant test file is in THIS plan's `files_modified` (Wave 0 created the skeleton); `❌ W0` if Wave 0 didn't create that file.
      - Set `Status`: ⬜ pending (always — Wave 0 hand-off baseline).

2. Write each row using this exact column order (matches the template):

   `| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |`

   Example rows (the executor will produce many more — one per task across all 11 implementing plans):

   ```markdown
   | 05-02-1  | 02 | 1 | DATA-11        | T-05-02-01 | CAGG hierarchy emits monotonic deltas per MP | integration       | `go test ./internal/aggregate/... -race -count=1 -run TestCAGGHierarchy -short=false` | ✅ (skeleton in 05-01) | ⬜ pending |
   | 05-02-2  | 02 | 1 | DATA-12        | —          | Refresh policy params satisfy CAGG safety window | integration   | `go test ./internal/aggregate/... -race -count=1 -run TestRefreshPolicyParams -short=false` | ✅ | ⬜ pending |
   | 05-03-1  | 03 | 2 | REPT-01..04    | T-05-03-01 | Period deltas computed with YoY silent fallback | unit             | `go test ./internal/report/... -race -count=1 -short -run "TestPeriodDelta\|TestReportScopeGrouping"` | ✅ | ⬜ pending |
   | 05-06-1  | 06 | 3 | REPT-05        | —          | PDF page 'N of M' footer rendered on every page | unit             | `go test ./internal/report/... -race -count=1 -short -run "TestPDFBranding"` | ✅ | ⬜ pending |
   | 05-06-2  | 06 | 3 | REPT-06        | T-05-06-02..04 | Worker enqueue is atomic; StatusHandler returns pdf_status | integration | `go test ./internal/report/... -race -count=1 -short=false -run "TestPDFJob\|TestEnqueuePDF_AtomicWithReport\|TestCleanupWorker\|TestGenerateHandler_EnqueuesPDFJob\|TestStatusHandler_" && grep -q "reports_cache:/var/lib/shifter/reports" compose/bundled.yml && grep -q "reports_cache:/var/lib/shifter/reports" compose/external.yml && grep -q "river.NewClient" cmd/serve/serve.go && grep -q 'r.Get("/api/reports/{id}"' internal/report/handlers.go` | ✅ | ⬜ pending |
   | 05-09-1  | 09 | 5 | REPT-01..07    | T-05-09-01 | useReportGenerate hook + config panel + URL state | unit              | `pnpm --dir web test:run --reporter=basic web/src/routes/reports/ && pnpm --dir web build && grep -q "export function useReportGenerate" web/src/routes/reports/useReportGenerate.ts && grep -q "useReportGenerate(" web/src/routes/reports/index.tsx` | ✅ | ⬜ pending |
   | 05-11-1  | 11 | 5 | DATA-13        | T-05-11-01..02 | PATCH retention reconciles policies in-tx; pgx-only (no lib/pq) | integration | `just sqlc && go test ./internal/settings/... -race -count=1 -short=false -run "TestRetentionConfig" && test "$(grep -r 'lib/pq' internal/settings/ \| wc -l \| tr -d ' ')" = "0"` | ✅ | ⬜ pending |
   ```

3. After all rows are written, also flip the `wave_0_complete:` frontmatter field of `05-VALIDATION.md` to `true` ONLY when:
   - Every plan 02–12 has ≥1 row in the map.
   - Every row's `Automated Command` column is non-empty.
   - Every row's `File Exists` column is either `✅` or `❌ W0`.

   The `nyquist_compliant:` flag stays `false` until plan 05-12 (phase closure) flips it after Wave-by-Wave bodies land. Do NOT touch `nyquist_compliant` in this plan.

4. Commit message: `test(05-01): wave 0 skeletons + River 0024 + pdfjs worker shim + populate VALIDATION map`
  </action>
  <verify>
    <automated>go build ./... &amp;&amp; go test ./internal/{aggregate,report,floorplan,map,settings,http}/... -race -count=1 -run TestNonExistent -short 2&gt;&amp;1 | grep -q "no tests to run\|PASS\|--- SKIP" ; pnpm --dir web test:run --reporter=basic 2&gt;&amp;1 | grep -E "Tests +[0-9]+ skipped" &amp;&amp; pnpm --dir web exec playwright test --list 2&gt;&amp;1 | grep -c "skip" | grep -v "^0$" &amp;&amp; test "$(grep -cE '^\\| 05-[0-9]{2}-[0-9]+' .planning/phases/05-aggregates-reports-map-floor-plans/05-VALIDATION.md)" -ge 12 &amp;&amp; grep -q "wave_0_complete: true" .planning/phases/05-aggregates-reports-map-floor-plans/05-VALIDATION.md</automated>
  </verify>
  <acceptance_criteria>
    - Every file listed in `files_modified` (Go test files + vitest files + Playwright specs + 05-VALIDATION.md) exists / is updated
    - Every Go skeleton compiles: `go build ./...` exits 0
    - Every Go skeleton uses `t.Skip("Plan 05-NN Task M: ...")` — `grep -rn "t.Skip" internal/{aggregate,report,floorplan,map,settings,http}/*_test.go` returns ≥1 match per file
    - Every vitest skeleton uses `it.skip(...)` — `grep -rn "it\\.skip" web/src/{components/map,components/floor-plan,lib,routes/reports}/*.test.{ts,tsx}` returns ≥1 per file
    - Every Playwright spec uses `test.skip(...)` — `grep -rn "test\\.skip" web/playwright/specs/*.spec.ts` returns ≥1 per file
    - `pnpm --dir web test:run` exit 0 (skipped tests do NOT fail the suite)
    - `pnpm --dir web exec playwright test --list` lists every new spec file
    - `05-VALIDATION.md` Per-Task Verification Map has ≥1 row per plan/task across plans 02–12: `grep -cE '^\\| 05-[0-9]{2}-[0-9]+' .planning/phases/05-.../05-VALIDATION.md` returns ≥12
    - Every map row's `Automated Command` column is non-empty and matches the source plan's `<automated>` block (spot-check at least 3 random rows manually)
    - `05-VALIDATION.md` frontmatter `wave_0_complete:` flipped from `false` to `true`
    - `05-VALIDATION.md` frontmatter `nyquist_compliant:` STILL `false` (only plan 05-12 flips it)
  </acceptance_criteria>
  <done>Every Phase 5 testable behavior has a named skeleton, the per-task verification map is real (one row per task across plans 02–12, automated commands sourced from each plan), suite green, wave_0_complete:true, hand-off to Wave 1 ready.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| install runtime → external network | `pnpm add` + `go get` reach npm registry + Go proxy; outbound, not phase-relevant once locked |
| developer machine → testcontainer Postgres | River CLI runs against ephemeral container; no production data exposure |

## STRIDE Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-05-01-01 | Tampering | New deps (maroto/river/leaflet/pdfjs/types) | low | mitigate | Pin exact versions (no `^` ranges in go.mod or package.json); `go.sum` + `pnpm-lock.yaml` lockfiles committed; reviewer can re-resolve to bit-identical artifacts |
| T-05-01-02 | Tampering | River SQL schema embedded as 0024 | low | mitigate | Schema captured by running River's own CLI against a clean Postgres + `pg_dump -s -t 'river_*'`, not hand-typed — eliminates "developer transcribed wrong column type" risk; round-trip test (`TestRunMigrations_RiverDownUpClean`) pins behavior |
</threat_model>

<verification>
**Phase-level checks for this plan:**

1. `go build ./...` exits 0
2. `pnpm --dir web build` exits 0
3. `go test ./internal/db/... -race -count=1 -short=false` exits 0 (round-trip suite)
4. `pnpm --dir web test:run` exits 0 (skeleton skips don't break)
5. `grep -E "(maroto/v2 v2.4.0|river v0.36.0|riverpgxv5 v0.36.0)" go.mod` returns 3 lines
6. `grep -E "(react-leaflet|leaflet|pdfjs-dist).*(5\.0\.0|1\.9\.4|1\.5\.3|4\.1\.3|5\.7\.284)" web/package.json` returns ≥5 lines
7. NO banned package present: `! grep -E "(gofpdf|chromedp|maplibre|gorm)" go.mod web/package.json`
8. `internal/aggregate/CUMULATIVE_DELTA.md` exists with verdict line
9. `internal/db/migrations/0024_river_tables.up.sql` + `.down.sql` exist
10. `web/src/lib/pdfWorker.ts` exists with `import.meta.url` pattern
11. `05-VALIDATION.md` Per-Task Verification Map populated: `grep -cE '^\| 05-[0-9]{2}-[0-9]+' 05-VALIDATION.md` returns ≥12 (one row per plan 02–12 at minimum)
12. `05-VALIDATION.md` frontmatter contains literal `wave_0_complete: true`
</verification>

<success_criteria>
**Wave 0 is complete when:**

- 19 requirement IDs covered by at least one named skeleton test (audit via `grep -rn` of REQ-IDs in test comments — Task 3 names them explicitly)
- `cumulative_delta` verdict file documents the absence so plan 05-02 makes an informed choice
- River schema is part of the golang-migrate sequence (no `river migrate-up` CLI step needed at install time)
- `pdfjs-dist` v5 ESM worker shim exists at `web/src/lib/pdfWorker.ts` — every consumer imports it (or imports `pdfToPng` which imports it)
- `05-VALIDATION.md` Per-Task Verification Map is fully populated — at least one row per task across plans 02–12, with each Automated Command sourced verbatim from the target plan's `<automated>` block
- `wave_0_complete` in 05-VALIDATION.md flipped to `true` so Wave 1 is unblocked
- `nyquist_compliant` STILL `false` (plan 05-12 flips it at phase closure once all bodies land)
</success_criteria>

<output>
After completion, create `.planning/phases/05-aggregates-reports-map-floor-plans/05-01-SUMMARY.md` recording:
- Verbatim version strings of every new dep
- Whether `cumulative_delta` was found (expected: absent — plan 05-02 will compute via LAG)
- Count of skeleton files created and their referenced REQ-IDs
- Total row count in the populated 05-VALIDATION.md Per-Task Verification Map (target: one per task across plans 02–12)
- Any surprises with River schema capture (column types, FK constraints, default values)
- Confirmation that `go.sum` + `pnpm-lock.yaml` are committed
- Confirmation that `wave_0_complete: true` was flipped (and `nyquist_compliant` remained `false`)
</output>
