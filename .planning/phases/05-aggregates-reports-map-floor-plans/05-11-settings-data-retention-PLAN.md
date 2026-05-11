---
phase: 05-aggregates-reports-map-floor-plans
plan: 11
type: execute
wave: 5
depends_on: [02]
files_modified:
  - internal/db/queries/settings.sql
  - internal/db/sqlc/settings.sql.go
  - internal/settings/doc.go
  - internal/settings/retention.go
  - internal/settings/retention_test.go
  - internal/settings/routes.go
  - internal/http/router.go
  - cmd/serve/serve.go
  - web/src/routes/settings.tsx
  - web/src/routes/settings.test.tsx
  - web/src/components/settings/DataRetentionCard.tsx
  - web/src/components/settings/EditRetentionDialog.tsx
  - web/playwright/specs/retention-settings.spec.ts
  - .planning/REQUIREMENTS.md
autonomous: true
requirements: [DATA-13]
threat_refs: [T-05-11-01]

must_haves:
  truths:
    - "GET /api/settings/retention returns current retention_config row {raw_days, hourly_days, daily_days, monthly_days, yearly_days}"
    - "PATCH /api/settings/retention validates ranges per UI-SPEC: raw 30..365; hourly 180..1825; daily 365..7300; monthly 1825..18250; yearly NULL or 1825..forever"
    - "PATCH writes the new row inside a pgx.Tx that ALSO reconciles TimescaleDB retention policies via remove + add — keeping the singleton config + actual policies in sync"
    - "Audit row written inside the same tx with action='settings.retention_change' and Before/After diffs of changed fields"
    - "Settings page has a new Data Retention card under the existing ChirpStack connection card (UI-SPEC §Settings — Data Retention Card)"
    - "Admin sees Edit ghost button per row; viewer sees read-only values (AUTH-06 frontend hide)"
    - "EditRetentionDialog uses ResponsiveDialog with single Input + zod range validation + Save"
    - "Yearly retention row reads 'Never expires' when value is NULL"
    - "Retention reconciliation runs in the worker that already manages CAGG refresh — OR — as a synchronous follow-up tx after the settings PATCH commits (planner picks; recommendation: synchronous reconciliation so the operator sees consistent state immediately)"
    - "internal/settings/ uses pgx/v5 exclusively — NO `github.com/lib/pq` import; hypertable names are vetted compile-time literals routed through a `switch` (CLAUDE.md banned-deps compliance)"
    - "REQUIREMENTS.md traceability updated: SETT-04 moves from Phase 6 to Phase 5 (RESEARCH Open Q #4 — DATA-13 requires the Settings UI; phase split shipped both here)"
  artifacts:
    - path: "internal/db/queries/settings.sql"
      provides: "GetRetentionConfig + UpdateRetentionConfig sqlc queries"
      contains: "name: GetRetentionConfig"
    - path: "internal/settings/retention.go"
      provides: "GET + PATCH handlers; ReconcilePolicies(ctx, tx, cfg) helper that issues `remove_retention_policy` + `add_retention_policy` for each CAGG level via vetted hypertable-name switch (no lib/pq)"
      contains: "ReconcilePolicies"
    - path: "internal/audit/log.go"
      provides: "ActionRetentionChange = settings.retention_change"
      contains: "ActionRetentionChange"
    - path: "web/src/components/settings/DataRetentionCard.tsx"
      provides: "Card showing 5 retention rows (raw, hourly, daily, monthly, yearly) with Edit buttons (admin only)"
      contains: "DataRetentionCard"
    - path: "web/src/components/settings/EditRetentionDialog.tsx"
      provides: "ResponsiveDialog with Input + zod validation + Save mutation"
      contains: "EditRetentionDialog"
    - path: ".planning/REQUIREMENTS.md"
      provides: "Updated SETT-04 mapping from Phase 6 to Phase 5"
      contains: "SETT-04"
  key_links:
    - from: "internal/settings/retention.go PATCH handler"
      to: "TimescaleDB add_retention_policy / remove_retention_policy"
      via: "tx.Exec inside the same pgx.Tx as the retention_config UPDATE"
      pattern: "remove_retention_policy.*add_retention_policy"
    - from: "internal/settings/retention.go PATCH handler"
      to: "audit.WriteEntry"
      via: "same pgx.Tx"
      pattern: "audit\\.WriteEntry\\(.*tx,"
---

<objective>
Ship the operator-facing Settings → Data Retention card (D-09 / DATA-13) so admins can tighten or loosen retention windows after install — and the backend that reconciles the TimescaleDB retention policies to match the new values inside the same transaction. SETT-04 traceability migrates from Phase 6 to Phase 5 per RESEARCH Open Question #4 (the UI must ship alongside the DATA-13 substrate so operators can act on what they see).

Purpose: Without this plan, retention values land at install (plan 05-02 seed) and never change. With it, ops teams can adjust raw retention down (compliance) or daily retention up (analytics window) without SSH-into-DB. The "same tx" reconciliation prevents drift between the singleton config row and the actual TimescaleDB policies.

Output: 1 sqlc query file, 1 Go package extension (`internal/settings/`), router wire-up, 3 React/TS files, 1 Playwright spec body, REQUIREMENTS.md update.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/05-aggregates-reports-map-floor-plans/05-CONTEXT.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-RESEARCH.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-UI-SPEC.md
@.planning/phases/05-aggregates-reports-map-floor-plans/05-02-cagg-hierarchy-retention-PLAN.md
@.planning/REQUIREMENTS.md
@internal/db/migrations/0029_retention_config.up.sql
@internal/audit/log.go
@internal/dashboard/install_scope_handler.go
@web/src/routes/settings.tsx
@CLAUDE.md

<interfaces>
<!-- From plan 05-02 -->
```sql
retention_config (
  id           INTEGER PRIMARY KEY CHECK (id = 1),
  raw_days     INTEGER NOT NULL CHECK (raw_days BETWEEN 30 AND 365),
  hourly_days  INTEGER NOT NULL CHECK (hourly_days BETWEEN 180 AND 1825),
  daily_days   INTEGER NOT NULL CHECK (daily_days BETWEEN 365 AND 7300),
  monthly_days INTEGER NOT NULL CHECK (monthly_days BETWEEN 1825 AND 18250),
  yearly_days  INTEGER,
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
)
```

<!-- TimescaleDB retention policy API (per RESEARCH §Pattern 3) -->
```sql
SELECT remove_retention_policy('measurement_hourly', if_exists => true);
SELECT add_retention_policy('measurement_hourly', INTERVAL '365 days');
```
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Backend — GET + PATCH /api/settings/retention with same-tx policy reconciliation</name>
  <read_first>
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-CONTEXT.md §D-09
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-UI-SPEC.md §Settings — Data Retention Card (range bounds)
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-RESEARCH.md §Pattern 3 (Retention Policies) §CAGG Retention Footgun
    - internal/db/migrations/0029_retention_config.up.sql (CHECK ranges)
    - internal/audit/log.go (audit constants + WriteEntry pattern)
    - CLAUDE.md §What NOT to Use (lib/pq is BANNED — use pgx/v5 only; hypertable names must come from a compile-time `switch`, never client input)
  </read_first>
  <behavior>
    - Test 1: GET /api/settings/retention returns the seeded row (90, 365, 1825, 7300, null)
    - Test 2: PATCH {raw_days: 60} updates only raw_days; the response reflects the new value
    - Test 3: PATCH {raw_days: 60} also issues `remove_retention_policy('measurement', ...)` + `add_retention_policy('measurement', INTERVAL '60 days')` inside the same tx — verified via `timescaledb_information.jobs` lookup
    - Test 4: Audit row written with action='settings.retention_change' and Before={raw_days: 90} After={raw_days: 60}
    - Test 5: PATCH {raw_days: 10} returns 422 (below 30-day minimum per CHECK + UI-SPEC range)
    - Test 6: PATCH {raw_days: 60, hourly_days: 180} → if any field fails validation, the entire request is rejected; no partial write
    - Test 7: PATCH with `yearly_days: null` sets yearly retention to forever (removes the policy if one exists, no add_retention_policy call); on read the field is null
    - Test 8: Setting raw_days < hourly_days_to_raw_dependent_window is allowed (we don't constrain inter-level relationships — the operator can still misconfigure; CAGG hierarchy footgun is documented in plan 05-02 doc.go)
    - Test 9: Viewer (non-admin) GET works; PATCH returns 403
    - Test 10 (compliance): `grep -r "lib/pq" internal/settings/ | wc -l` returns 0 — package uses pgx/v5 exclusively
  </behavior>
  <action>
**Step A — `internal/db/queries/settings.sql`:**

```sql
-- name: GetRetentionConfig :one
SELECT id, raw_days, hourly_days, daily_days, monthly_days, yearly_days, updated_at
FROM retention_config WHERE id = 1;

-- name: UpdateRetentionConfig :one
UPDATE retention_config
SET raw_days     = COALESCE($1, raw_days),
    hourly_days  = COALESCE($2, hourly_days),
    daily_days   = COALESCE($3, daily_days),
    monthly_days = COALESCE($4, monthly_days),
    yearly_days  = $5,  -- nullable; explicit NULL means "forever" (D-09)
    updated_at   = now()
WHERE id = 1
RETURNING *;
```

**Note on yearly_days:** It's deliberately NOT wrapped in `COALESCE` so the operator CAN set it to NULL explicitly. We use a sentinel signal at the Go layer to distinguish "user didn't include yearly_days in the patch" from "user explicitly set yearly_days = null".

`just sqlc`.

**Step B — Audit constant `internal/audit/log.go`:**

```go
const (
    ActionRetentionChange = "settings.retention_change"
    EntityTypeRetention   = "retention_config"
)
```

Update audit_log vocabulary migration (or extend plan 05-03's 0031_audit_vocab) with these two values.

**Step C — `internal/settings/doc.go`:**

```go
// Package settings owns operator-facing configuration that can change after
// install (as distinct from internal/install, which owns the wizard-time
// one-shot setup).
//
// # Data retention (Phase 5 DATA-13 + D-09)
//
// retention_config is a singleton (id=1) row with per-level windows:
//   raw_days, hourly_days, daily_days, monthly_days, yearly_days (NULL = forever)
//
// PATCH /api/settings/retention does two things in one pgx.Tx:
//
//   1. UPDATE retention_config (sqlc)
//   2. Call ReconcilePolicies(ctx, tx, cfg) which issues
//      remove_retention_policy(...) + add_retention_policy(...) for each
//      level whose value changed. Both are TimescaleDB-provided functions
//      that take effect at the next retention worker pass (default every
//      1 minute) — they're safe to call inside a tx (only CREATE
//      MATERIALIZED VIEW WITH DATA is the tx-incompatible CAGG operation,
//      per Pitfall #1).
//
// # Why same-tx reconciliation
//
// Drift between retention_config and the actual policies is the operational
// nightmare here. Same-tx means: if the policy ALTER fails, the config row
// is also rolled back — the operator never sees "raw_days = 60" in the UI
// while the actual TimescaleDB policy is still 90.
//
// # Why no lib/pq
//
// CLAUDE.md bans github.com/lib/pq (maintenance mode since 2021; ecosystem
// moved to pgx/v5). Hypertable names in the SQL templating come from a
// compile-time `switch` (vetted-literal set), not client input — so no
// runtime quoting/escaping helper is needed. The interval is bound via
// pgx parameter ($1).
//
// # SETT-04 traceability
//
// SETT-04 (Settings → Data Retention category) was originally mapped to Phase 6.
// Per RESEARCH §Open Questions #4, the UI ships in Phase 5 alongside DATA-13's
// substrate. REQUIREMENTS.md updated to reflect Phase 5 ownership.
package settings
```

**Step D — `internal/settings/retention.go`:**

```go
package settings

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "net/http"

    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"

    sqlc "shifter/internal/db/sqlc"
    "shifter/internal/audit"
    "shifter/internal/auth"
)

// IMPORTANT: NO `github.com/lib/pq` import — CLAUDE.md banned dep.
// Hypertable names in ReconcilePolicies come from a compile-time switch
// (vetted literal set), so no runtime escape helper is needed.

type Deps struct {
    Pool    *pgxpool.Pool
    Queries *sqlc.Queries
}

type RetentionResponse struct {
    RawDays     int    `json:"raw_days"`
    HourlyDays  int    `json:"hourly_days"`
    DailyDays   int    `json:"daily_days"`
    MonthlyDays int    `json:"monthly_days"`
    YearlyDays  *int   `json:"yearly_days"`  // nullable = forever
    UpdatedAt   string `json:"updated_at"`
}

type RetentionPatch struct {
    RawDays       *int  `json:"raw_days,omitempty"`
    HourlyDays    *int  `json:"hourly_days,omitempty"`
    DailyDays     *int  `json:"daily_days,omitempty"`
    MonthlyDays   *int  `json:"monthly_days,omitempty"`
    YearlyDays    *int  `json:"yearly_days,omitempty"`
    YearlyForever *bool `json:"yearly_forever,omitempty"`  // sentinel: set to true to NULL out yearly_days
}

func GetHandler(deps Deps) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        cfg, err := deps.Queries.GetRetentionConfig(r.Context())
        if err != nil { writeError(w, 500, "load_failed"); return }
        writeJSON(w, 200, toResponse(cfg))
    }
}

func PatchHandler(deps Deps) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        user, _ := auth.UserFromCtx(r.Context())
        if !auth.Can(user, "settings.update", "retention_config") {
            writeError(w, 403, "forbidden"); return
        }

        var patch RetentionPatch
        if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
            writeError(w, 400, "invalid_json"); return
        }
        if err := validatePatch(patch); err != nil {
            writeError(w, 422, err.Error()); return
        }

        tx, err := deps.Pool.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
        if err != nil { writeError(w, 500, "tx_begin"); return }
        defer tx.Rollback(r.Context())
        q := deps.Queries.WithTx(tx)

        before, err := q.GetRetentionConfig(r.Context())
        if err != nil { writeError(w, 500, "load_failed"); return }

        // Resolve yearly_days from sentinel: forever flag → null; else use patch value (nullable).
        var yearlyDays *int
        switch {
        case patch.YearlyForever != nil && *patch.YearlyForever:
            yearlyDays = nil
        case patch.YearlyDays != nil:
            yearlyDays = patch.YearlyDays
        default:
            yearlyDays = before.YearlyDays  // unchanged
        }

        updated, err := q.UpdateRetentionConfig(r.Context(), sqlc.UpdateRetentionConfigParams{
            RawDays:     patch.RawDays,    // nullable → COALESCE in SQL
            HourlyDays:  patch.HourlyDays,
            DailyDays:   patch.DailyDays,
            MonthlyDays: patch.MonthlyDays,
            YearlyDays:  yearlyDays,
        })
        if err != nil {
            // Map CHECK violation to 422.
            writeError(w, 500, "update_failed"); return
        }

        // Same-tx policy reconciliation for every changed level.
        if err := ReconcilePolicies(r.Context(), tx, before, updated); err != nil {
            writeError(w, 500, "policy_reconcile_failed"); return
        }

        // Audit entry — Before/After only includes the changed fields.
        beforeMap, afterMap := diffFields(before, updated)
        if err := audit.WriteEntry(r.Context(), tx, audit.Entry{
            UserID: user.ID, Action: audit.ActionRetentionChange,
            EntityType: audit.EntityTypeRetention, EntityID: uuid.Nil,
            Before: beforeMap, After: afterMap,
            RequestID: middleware.GetReqID(r.Context()),
        }); err != nil { writeError(w, 500, "audit_failed"); return }

        if err := tx.Commit(r.Context()); err != nil { writeError(w, 500, "tx_commit"); return }
        writeJSON(w, 200, toResponse(updated))
    }
}

// ReconcilePolicies removes + re-adds the matching TimescaleDB retention policy
// for each level that changed. yearly NULL → ensure no policy exists (remove
// only). Same tx as the config row UPDATE so a rollback restores both.
//
// SECURITY / CLAUDE.md compliance: the hypertable name in the SQL template
// comes from a compile-time `switch` — it's NEVER a runtime string. This
// removes the need for any quoting/escaping helper (and specifically the
// banned `github.com/lib/pq` `pq.QuoteLiteral`). The interval value is
// bound via pgx parameter binding ($1).
func ReconcilePolicies(ctx context.Context, tx pgx.Tx, before, after sqlc.RetentionConfig) error {
    // Pairs: (level_key, before_days, after_days_or_nil)
    type level struct{ name string; before, after sql.NullInt32 }
    levels := []level{
        {"measurement",         int32Null(before.RawDays),     int32Null(after.RawDays)},
        {"measurement_hourly",  int32Null(before.HourlyDays),  int32Null(after.HourlyDays)},
        {"measurement_daily",   int32Null(before.DailyDays),   int32Null(after.DailyDays)},
        {"measurement_monthly", int32Null(before.MonthlyDays), int32Null(after.MonthlyDays)},
        {"measurement_yearly",  ptrToNull(before.YearlyDays),  ptrToNull(after.YearlyDays)},
    }
    for _, l := range levels {
        if l.before == l.after { continue }  // unchanged → no-op

        // Resolve the hypertable name from a vetted compile-time literal set.
        // This is the ONLY way the name reaches the SQL template — no runtime
        // strings, no quoting helper, no lib/pq.
        var hypertable string
        switch l.name {
        case "measurement":
            hypertable = "measurement"
        case "measurement_hourly":
            hypertable = "measurement_hourly"
        case "measurement_daily":
            hypertable = "measurement_daily"
        case "measurement_monthly":
            hypertable = "measurement_monthly"
        case "measurement_yearly":
            hypertable = "measurement_yearly"
        default:
            return fmt.Errorf("retention: unknown hypertable %q", l.name)
        }

        // remove_retention_policy: hypertable is a vetted literal; no
        // parameter binding available because TimescaleDB takes the
        // hypertable name as a regclass identifier, not a string param.
        removeSQL := fmt.Sprintf(`SELECT remove_retention_policy('%s', if_exists => true)`, hypertable)
        if _, err := tx.Exec(ctx, removeSQL); err != nil {
            return fmt.Errorf("remove policy %s: %w", hypertable, err)
        }

        // add_retention_policy: hypertable is the vetted literal; the
        // interval VALUE is bound via $1 (pgx parameter binding) so the
        // integer day count cannot inject anything.
        if l.after.Valid && l.after.Int32 > 0 {
            addSQL := fmt.Sprintf(`SELECT add_retention_policy('%s', make_interval(days => $1))`, hypertable)
            if _, err := tx.Exec(ctx, addSQL, int(l.after.Int32)); err != nil {
                return fmt.Errorf("add policy %s: %w", hypertable, err)
            }
        }
    }
    return nil
}

func validatePatch(p RetentionPatch) error {
    if p.RawDays != nil && (*p.RawDays < 30 || *p.RawDays > 365) {
        return errors.New("raw_days_out_of_range")
    }
    if p.HourlyDays != nil && (*p.HourlyDays < 180 || *p.HourlyDays > 1825) {
        return errors.New("hourly_days_out_of_range")
    }
    if p.DailyDays != nil && (*p.DailyDays < 365 || *p.DailyDays > 7300) {
        return errors.New("daily_days_out_of_range")
    }
    if p.MonthlyDays != nil && (*p.MonthlyDays < 1825 || *p.MonthlyDays > 18250) {
        return errors.New("monthly_days_out_of_range")
    }
    if p.YearlyDays != nil && *p.YearlyDays < 1825 {
        return errors.New("yearly_days_out_of_range")
    }
    return nil
}

// ... toResponse, diffFields, int32Null, ptrToNull helpers
```

**Banned-dep guardrail:** verify with `grep -r "lib/pq" internal/settings/ | wc -l` — must return 0. The `make_interval(days => $1)` form is preferred over `INTERVAL '%d days'` formatting because it routes the day count through pgx parameter binding (defense in depth) and matches TimescaleDB's documented signature.

**Step E — `internal/settings/routes.go`:**

```go
package settings

import "github.com/go-chi/chi/v5"

func RegisterRoutes(r chi.Router, deps Deps) {
    r.Get("/api/settings/retention", GetHandler(deps))
    r.Patch("/api/settings/retention", PatchHandler(deps))
}
```

**Step F — Wire in `internal/http/router.go`:**

Add SettingsDeps to Deps struct; pass into RegisterRoutes inside the authenticated group.

Wire in `cmd/serve/serve.go`:

```go
deps.SettingsDeps = &settings.Deps{Pool: pool, Queries: q}
```

**Step G — `auth.Can` extension:**

Add `"settings.update"` action with admin-only resolution (mirroring existing Phase 1 Can() entries for `"chirpstack.update"` or similar). Read `internal/auth/permissions.go` (or wherever Can lives) and append.

**Step H — Replace `t.Skip` in `internal/settings/retention_test.go`:**

```go
func TestRetentionConfigCRUD(t *testing.T) {
    // 1) Bootstrap test pool with migrations through 0029 + seed retention_config row.
    // 2) GET /api/settings/retention → 200 + (90, 365, 1825, 7300, null)
    // 3) PATCH {raw_days: 60} as admin → 200 + raw_days = 60
    // 4) Verify same tx reconciliation: query timescaledb_information.jobs WHERE proc_name='policy_retention' AND hypertable_name='measurement' → expect 60-day INTERVAL in config
    // 5) Verify audit_log entry with action=settings.retention_change AND before={raw_days: 90} AND after={raw_days: 60}
}

func TestRetentionConfig_OutOfRange_422(t *testing.T) {
    // PATCH {raw_days: 10} → 422 "raw_days_out_of_range"
}

func TestRetentionConfig_YearlyForever_RemovesPolicy(t *testing.T) {
    // Seed yearly_days=1825 + an add_retention_policy('measurement_yearly', INTERVAL '5 years').
    // PATCH {yearly_forever: true} → yearly_days = NULL AND policy removed (verify via timescaledb_information.jobs lookup).
}

func TestRetentionConfig_Rollback_OnPolicyFailure(t *testing.T) {
    // Inject a policy reconciliation failure (e.g., mock tx.Exec or alter timescale config to make the call fail).
    // PATCH {raw_days: 60} → 500
    // Verify retention_config STILL shows raw_days=90 (rollback preserved consistency).
}

func TestRetentionConfig_PatchAsViewer_403(t *testing.T) {
    // Authenticate as a viewer user; PATCH → 403 forbidden.
}

func TestRetentionConfig_UnknownHypertable_Rejected(t *testing.T) {
    // Unit-level: call ReconcilePolicies directly with a constructed `level{name: "evil_drop_table"}`
    // — assert it returns an "unknown hypertable" error and issues ZERO tx.Exec calls.
    // (Defense-in-depth proof that the compile-time switch refuses any name not in the vetted set.)
}
```
  </action>
  <verify>
    <automated>just sqlc &amp;&amp; go test ./internal/settings/... -race -count=1 -short=false -run "TestRetentionConfig" &amp;&amp; test "$(grep -r 'lib/pq' internal/settings/ | wc -l | tr -d ' ')" = "0"</automated>
  </verify>
  <acceptance_criteria>
    - `internal/db/queries/settings.sql` contains `name: GetRetentionConfig` and `name: UpdateRetentionConfig`
    - `internal/settings/retention.go` exports `GetHandler`, `PatchHandler`, `ReconcilePolicies`, `RetentionPatch`, `RetentionResponse`
    - `internal/settings/retention.go` contains literal `remove_retention_policy` AND `add_retention_policy` inside ReconcilePolicies
    - `internal/settings/retention.go` PATCH handler contains literal `audit.WriteEntry(r.Context(), tx,` (same-tx audit)
    - `internal/settings/retention.go` contains a `switch l.name` with the five vetted hypertable cases (measurement, measurement_hourly, measurement_daily, measurement_monthly, measurement_yearly) AND a `default:` arm returning an "unknown hypertable" error
    - `internal/settings/retention.go` does NOT import `github.com/lib/pq` — verified by `! grep -q "lib/pq" internal/settings/retention.go`
    - `grep -r "lib/pq" internal/settings/ | wc -l` returns 0 (whole package is pgx-only)
    - `internal/audit/log.go` contains literal `ActionRetentionChange = "settings.retention_change"`
    - `internal/auth/permissions.go` (or equivalent) contains literal `"settings.update"`
    - At least 6 test cases in `retention_test.go` covering: happy path + out-of-range + yearly-forever + rollback + viewer-403 + unknown-hypertable defense
    - `TestRetentionConfig_Rollback_OnPolicyFailure` asserts retention_config row unchanged after injected failure
    - `TestRetentionConfig_UnknownHypertable_Rejected` asserts ReconcilePolicies returns error AND issues zero tx.Exec calls
    - `go test ./internal/settings/... -short=false` exits 0
  </acceptance_criteria>
  <done>Settings retention CRUD live with same-tx policy reconciliation + audit-in-tx + viewer 403 + pgx-only (no lib/pq).</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Frontend Data Retention card + EditRetentionDialog + REQUIREMENTS.md update + Playwright</name>
  <read_first>
    - .planning/phases/05-aggregates-reports-map-floor-plans/05-UI-SPEC.md §Settings — Data Retention Card
    - .planning/REQUIREMENTS.md (SETT-04 traceability)
    - web/src/routes/settings.tsx (existing settings page layout)
    - web/src/components/responsive-dialog.tsx
    - web/src/lib/hooks/useCurrentUser.ts (or wherever role gating lives)
  </read_first>
  <behavior>
    - Test 1: DataRetentionCard renders 5 rows: Raw measurements / Hourly aggregate / Daily aggregate / Monthly aggregate / Yearly aggregate
    - Test 2: Yearly row shows "Never expires" when yearly_days is null
    - Test 3: Admin sees Edit button per row (except yearly which is read-only label)
    - Test 4: Viewer sees no Edit buttons (frontend hide; backend enforces)
    - Test 5: Clicking Edit on a row opens EditRetentionDialog pre-filled with current value
    - Test 6: Out-of-range value shows inline zod error
    - Test 7: Save fires PATCH /api/settings/retention with single-field body; on 200, dialog closes + toast 'Retention settings saved.'
    - Test 8: REQUIREMENTS.md shows SETT-04 under Phase 5 (not Phase 6)
  </behavior>
  <action>
**Step A — `web/src/components/settings/DataRetentionCard.tsx`:**

```tsx
import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { apiFetch } from '@/lib/apiFetch'
import { useCurrentUser } from '@/lib/hooks/useCurrentUser'
import { EditRetentionDialog } from './EditRetentionDialog'

type RetentionConfig = {
  raw_days: number; hourly_days: number; daily_days: number; monthly_days: number; yearly_days: number | null
}

type LevelKey = 'raw_days' | 'hourly_days' | 'daily_days' | 'monthly_days'

const LEVELS: Array<{ key: LevelKey; label: string; unit: 'days' | 'years'; min: number; max: number }> = [
  { key: 'raw_days',     label: 'Raw measurements',  unit: 'days',  min: 30,   max: 365 },
  { key: 'hourly_days',  label: 'Hourly aggregate',  unit: 'days',  min: 180,  max: 1825 },
  { key: 'daily_days',   label: 'Daily aggregate',   unit: 'days',  min: 365,  max: 7300 },
  { key: 'monthly_days', label: 'Monthly aggregate', unit: 'years', min: 1825, max: 18250 },
]

export function DataRetentionCard() {
  const user = useCurrentUser()
  const isAdmin = user?.role === 'admin'
  const { data } = useQuery({ queryKey: ['settings', 'retention'], queryFn: () => apiFetch<RetentionConfig>('/api/settings/retention') })
  const [editing, setEditing] = useState<LevelKey | null>(null)

  if (!data) return null

  return (
    <Card>
      <CardHeader><CardTitle>Data Retention</CardTitle></CardHeader>
      <CardContent className="space-y-2">
        {LEVELS.map(level => (
          <div key={level.key} className="flex justify-between items-center py-2 border-b last:border-0">
            <div>
              <div className="text-sm font-medium">{level.label}</div>
              <div className="text-xs text-muted-foreground">{formatValue(data[level.key], level.unit)}</div>
            </div>
            {isAdmin && (
              <Button variant="ghost" size="sm" onClick={() => setEditing(level.key)}>Edit</Button>
            )}
          </div>
        ))}
        {/* Yearly aggregate row is read-only — special case for null */}
        <div className="flex justify-between items-center py-2">
          <div>
            <div className="text-sm font-medium">Yearly aggregate</div>
            <div className="text-xs text-muted-foreground">
              {data.yearly_days === null ? 'Never expires' : formatValue(data.yearly_days, 'years')}
            </div>
          </div>
        </div>
      </CardContent>

      {editing && (
        <EditRetentionDialog
          level={LEVELS.find(l => l.key === editing)!}
          initialValue={data[editing]}
          open={true}
          onOpenChange={(o) => !o && setEditing(null)}
        />
      )}
    </Card>
  )
}

function formatValue(days: number, unit: 'days' | 'years'): string {
  if (unit === 'years') return `${(days / 365).toFixed(0)} years`
  return `${days} days`
}
```

**Step B — `web/src/components/settings/EditRetentionDialog.tsx`:**

```tsx
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { ResponsiveDialog } from '@/components/responsive-dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Button } from '@/components/ui/button'
import { apiFetch } from '@/lib/apiFetch'

type Level = { key: string; label: string; unit: 'days' | 'years'; min: number; max: number }

export function EditRetentionDialog({ level, initialValue, open, onOpenChange }: {
  level: Level
  initialValue: number
  open: boolean
  onOpenChange: (o: boolean) => void
}) {
  const schema = z.object({
    value: z.coerce.number().int().min(level.min, `Must be at least ${level.min} ${level.unit}`).max(level.max, `Must be at most ${level.max} ${level.unit}`),
  })
  const form = useForm({ resolver: zodResolver(schema), defaultValues: { value: initialValue } })

  const queryClient = useQueryClient()
  const save = useMutation({
    mutationFn: async (vals: { value: number }) => apiFetch('/api/settings/retention', {
      method: 'PATCH',
      body: JSON.stringify({ [level.key]: vals.value }),
    }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['settings', 'retention'] })
      toast.success('Retention settings saved.')
      onOpenChange(false)
    },
    onError: () => toast.error('Could not save retention settings. Try again.'),
  })

  return (
    <ResponsiveDialog open={open} onOpenChange={onOpenChange} title={`Edit ${level.label}`}>
      <form onSubmit={form.handleSubmit((vals) => save.mutate(vals))} className="space-y-4">
        <div>
          <Label htmlFor="value">Value</Label>
          <div className="flex gap-2">
            <Input id="value" type="number" {...form.register('value')} />
            <span className="self-center text-sm text-muted-foreground">{level.unit}</span>
          </div>
          {form.formState.errors.value && (
            <p className="text-sm text-destructive mt-1">{form.formState.errors.value.message}</p>
          )}
          <p className="text-xs text-muted-foreground mt-2">
            Range: {level.min} – {level.max} {level.unit}.
          </p>
        </div>
        <div className="flex justify-end gap-2">
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button type="submit" disabled={save.isPending}>Save retention settings</Button>
        </div>
      </form>
    </ResponsiveDialog>
  )
}
```

**Step C — Extend `web/src/routes/settings.tsx`:**

Add `<DataRetentionCard />` below the existing ChirpStack connection card.

**Step D — Replace `it.skip` in `web/src/routes/settings.test.tsx`:**

Tests for 5-row render, Never expires for yearly, admin sees Edit / viewer doesn't, dialog opens with current value, out-of-range zod error, PATCH success → toast + invalidate.

**Step E — REQUIREMENTS.md update:**

Locate the SETT-04 row (currently mapped to Phase 6). Change the phase mapping to Phase 5. Add a note:

```markdown
| SETT-04 | Phase 5 | Pending | Settings → Data Retention card shipped in Plan 05-11 alongside DATA-13 substrate (RESEARCH Open Q #4). |
```

**Step F — Playwright spec body `web/playwright/specs/retention-settings.spec.ts`:**

```ts
test.describe('Settings — Data Retention (D-09 / DATA-13)', () => {
  test('admin edits raw retention 90→60 days → CAGG refresh policy reflects new bound', async ({ page }) => {
    await login(page, 'admin')
    await page.goto('/settings')
    await expect(page.getByText('Data Retention')).toBeVisible()
    await expect(page.getByText('Raw measurements')).toBeVisible()

    // Click Edit on the raw measurements row.
    await page.locator('div:has-text("Raw measurements") + div button:has-text("Edit")').first().click()

    await page.fill('input[type=number]', '60')
    await page.click('button:has-text("Save retention settings")')

    await expect(page.getByText('Retention settings saved.')).toBeVisible()
    await expect(page.getByText('60 days')).toBeVisible()
  })

  test('viewer sees read-only retention values, no Edit button (AUTH-06 frontend hide)', async ({ page }) => {
    await login(page, 'viewer')
    await page.goto('/settings')
    await expect(page.getByText('Raw measurements')).toBeVisible()
    await expect(page.locator('div:has-text("Raw measurements") + div button:has-text("Edit")')).toHaveCount(0)
  })
})
```
  </action>
  <verify>
    <automated>pnpm --dir web test:run --reporter=basic web/src/routes/settings.test.tsx web/src/components/settings/ &amp;&amp; pnpm --dir web build &amp;&amp; grep -qE "^\\| SETT-04 \\| Phase 5" .planning/REQUIREMENTS.md &amp;&amp; pnpm --dir web exec playwright test --list retention-settings.spec.ts 2&gt;&amp;1 | grep -c "›"</automated>
  </verify>
  <acceptance_criteria>
    - `web/src/components/settings/DataRetentionCard.tsx` contains literal `Data Retention` AND `Never expires` AND 4 LEVELS entries (raw_days / hourly_days / daily_days / monthly_days)
    - `web/src/components/settings/EditRetentionDialog.tsx` contains literal `PATCH` AND `zodResolver` AND `Save retention settings` button
    - Admin/viewer gating: `isAdmin` check used to render Edit button (verified by reading)
    - `web/src/routes/settings.tsx` renders `<DataRetentionCard />`
    - REQUIREMENTS.md row for SETT-04 now reads `| SETT-04 | Phase 5 |` (was Phase 6)
    - At least 7 vitest cases in settings.test.tsx covering the behavior list
    - Playwright spec has no `test.skip`
    - `pnpm --dir web test:run` exits 0
    - `pnpm --dir web build` exits 0
  </acceptance_criteria>
  <done>Data Retention card live with admin-only edit; backend in Task 1 already handles the PATCH + audit + reconciliation.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| Client → PATCH /api/settings/retention | Admin-only (auth.Can('settings.update')); zod-validated body; viewer 403 |
| PATCH handler → TimescaleDB policy ALTER | Same tx as the retention_config UPDATE — drift impossible |
| ReconcilePolicies → SQL template | Hypertable name resolved through compile-time `switch` (vetted literal set); no runtime escape helper (lib/pq is banned) |

## STRIDE Threat Register

| Threat ID | Category | Component | Severity | Disposition | Mitigation Plan |
|-----------|----------|-----------|----------|-------------|-----------------|
| T-05-11-01 | Elevation of Privilege | Viewer changes retention settings | medium | mitigate | Backend `auth.Can('settings.update')` returns 403 for viewers; frontend hides Edit buttons but server is authoritative. Test `TestRetentionConfig_PatchAsViewer_403` pins. |
| T-05-11-02 | Tampering | SQL injection via hypertable name in remove/add_retention_policy template | low | mitigate | Hypertable name is NEVER a runtime string — it is resolved through a 5-case compile-time `switch` (default arm returns error). Interval value bound via pgx parameter ($1). Test `TestRetentionConfig_UnknownHypertable_Rejected` pins the defense. |
</threat_model>

<verification>
1. `go test ./internal/settings/... -race -count=1 -short=false` exits 0
2. `pnpm --dir web test:run` exits 0
3. `pnpm --dir web build` exits 0
4. `grep -q "ActionRetentionChange" internal/audit/log.go`
5. `grep -qE "^\| SETT-04 \| Phase 5" .planning/REQUIREMENTS.md`
6. `grep -r "lib/pq" internal/settings/ | wc -l` returns 0 (CLAUDE.md banned-dep compliance)
</verification>

<success_criteria>
- Backend GET + PATCH retention live with same-tx policy reconciliation + audit + viewer 403
- Frontend Data Retention card with 5 rows + admin-only Edit buttons + zod range validation
- SETT-04 traceability migrated from Phase 6 to Phase 5 in REQUIREMENTS.md
- Playwright covers admin edit + viewer read-only
- `internal/settings/` is pgx-only (no `github.com/lib/pq` import anywhere)
</success_criteria>

<output>
After completion, create `.planning/phases/05-aggregates-reports-map-floor-plans/05-11-SUMMARY.md` recording:
- Whether same-tx policy reconciliation needed any TimescaleDB-version-specific tweaks
- Audit before/after diff coverage (which fields appeared in the test fixture)
- Confirmation that REQUIREMENTS.md update preserved the existing row order
- Confirmation that `grep -r "lib/pq" internal/settings/` returns zero matches (CLAUDE.md compliance)
</output>
