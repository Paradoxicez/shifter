---
phase: 06-alerts-users-audit-operational-hardening
plan: 03
type: execute
wave: 2
depends_on: [06-01]
files_modified:
  - internal/alert/anomaly_worker.go
  - internal/alert/anomaly_worker_test.go
  - internal/alert/cold_start.go
  - internal/alert/cold_start_test.go
  - internal/alert/quiet_hour.go
  - internal/alert/quiet_hour_test.go
  - internal/alert/queries.sql
  - internal/cli/serve.go
autonomous: true
requirements: [ALERT-04]
must_haves:
  truths:
    - "AnomalyWorker evaluates anomaly_p95, anomaly_iqr, and anomaly_quiet_hour rule kinds in one cycle"
    - "Cold-start gate prevents anomaly fires for MPs with < 21 days of measurement history (D-16)"
    - "Cold-start eligibility roster query returns days_until_eligible per non-eligible MP"
    - "P95 rule fires when latest measurement instant_value > P95 of trailing 30 days for same MP × hour-of-day bucket"
    - "IQR rule fires when latest value outside Q1-1.5·IQR to Q3+1.5·IQR over trailing 30 days"
    - "Quiet-hour rule fires when latest measurement during operator-configured quiet window has instant_value > flow_threshold (D-17)"
    - "Quiet-hour SQL handles cross-midnight windows correctly (22:00→06:00 case)"
    - "Worker runs hourly (1× per hour) per RESEARCH §Decision C"
  artifacts:
    - path: internal/alert/anomaly_worker.go
      provides: "AnomalyWorker dispatching to per-rule-kind evaluators"
      contains: "func (w *AnomalyWorker) Work"
    - path: internal/alert/cold_start.go
      provides: "IsMPEligibleForAnomaly + ListAnomalyWarmupRoster + per-MP anomaly toggle state"
      contains: "IsMPEligibleForAnomaly"
    - path: internal/alert/quiet_hour.go
      provides: "EvalQuietHour with cross-midnight time-range support"
      contains: "cross-midnight"
  key_links:
    - from: internal/alert/anomaly_worker.go
      to: internal/alert/cold_start.go
      via: "worker calls IsMPEligibleForAnomaly before evaluating each MP"
      pattern: "IsMPEligibleForAnomaly\\("
---

<objective>
Implement ALERT-04: statistical anomaly evaluators (P95, IQR, quiet-hour) gated by the 21-day cold-start rule (D-16, D-17). The schema lives in alert_rule (from Plan 06-01) — this plan adds the worker, the cold-start helpers, and the eligibility roster query that feeds Plan 06-04's MP detail "Anomaly detection" card and Settings → Alerts warmup section.

Purpose: ship the SHAPE of statistical anomaly detection in v1. Phase 7 tunes the thresholds against real customer data; Phase 6 ships the three rule kinds with conservative defaults, cold-start gating to prevent false-positive ramp-up alerts on new installs, and the eligibility chip surface so operators can see when a meter "graduates" from warming up.

Output: AnomalyWorker (registered with 1h cadence) + cold-start helpers + quiet-hour cross-midnight SQL + per-MP anomaly_p95/iqr/quiet_hour opt-in toggle.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/06-alerts-users-audit-operational-hardening/06-CONTEXT.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-01-alert-engine-substrate-PLAN.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-02-threshold-offline-evaluators-PLAN.md
@internal/alert/engine.go
@internal/alert/rule_store.go
@internal/alert/alert_store.go
@internal/alert/payload.go
@internal/alert/queries.sql
@internal/db/migrations/0038_alert_rule.up.sql

<interfaces>
internal/alert/payload.go BuildPayloadInput has anomaly-specific fields:
```go
P95Baseline       *float64 // anomaly_p95
BaselineWindowDays *int32  // anomaly_p95, anomaly_iqr
TimeOfDayBucket   *string  // anomaly_p95
```

alert_rule schema (from Plan 06-01):
- quiet_window_start TIME (nullable)
- quiet_window_end TIME (nullable)
- flow_threshold DOUBLE PRECISION DEFAULT 0.0
- days_of_week INTEGER (bitmask, NULL=all days)
- scope_kind/scope_id

Existing measurement schema (Phase 2 migration 0015):
- measurement: (time TIMESTAMPTZ, metering_point_id UUID, instant_value DOUBLE PRECISION, ...)
- Indexed by (metering_point_id, time DESC)

install_state.timezone (Phase 1): the operator's configured tz e.g. 'Asia/Bangkok'

RuleStore.ListActiveRulesByKind(ctx, kind) — works for "anomaly_p95"/"anomaly_iqr"/"anomaly_quiet_hour"
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Cold-start gate + anomaly-eligibility roster (D-16) + sqlc queries</name>
  <files>internal/alert/cold_start.go, internal/alert/cold_start_test.go, internal/alert/queries.sql</files>
  <read_first>
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md §Decision E "Cold-Start Gate (D-16)" — exact SQL for IsMPEligibleForAnomaly and ListAnomalyWarmupRoster
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-CONTEXT.md §D-16 (MP detail card states: warming_up | eligible_inactive | active)
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-UI-SPEC.md §Surface 4 (cold-start card three-state pattern)
    - internal/db/sqlc/ (existing pattern for generated code)
  </read_first>
  <behavior>
    - Test (TestColdStart_NewMPNotEligible): MP with no measurements OR oldest measurement < 21 days ago → IsMPEligibleForAnomaly returns false.
    - Test (TestColdStart_OldMPIsEligible): MP with measurements ≥ 21 days ago → returns true.
    - Test (TestWarmupRoster_DaysUntilEligible): MP with oldest measurement 5 days ago → days_until_eligible = 16. MP with no measurements → days_until_eligible = 21. MP with measurements ≥ 21 days ago → days_until_eligible = 0 (eligible).
    - Test (TestWarmupRoster_OrderingByDaysUntilEligible): roster returned ASC (most-imminent-eligible first).
  </behavior>
  <action>
    Add to `internal/alert/queries.sql`:
    ```sql
    -- name: IsMPEligibleForAnomaly :one
    -- D-16: True iff the metering point has at least one measurement ≥ 21 days old.
    SELECT EXISTS(
        SELECT 1 FROM measurement
        WHERE metering_point_id = $1
          AND time < now() - INTERVAL '21 days'
    ) AS eligible;

    -- name: ListAnomalyWarmupRoster :many
    -- D-16: returns days_until_eligible per active (non-disabled) MP.
    -- 0 means eligible; >0 means still warming up.
    SELECT
        mp.id AS metering_point_id,
        mp.label AS metering_point_label,
        s.label AS site_label,
        CASE
            WHEN MIN(m.time) IS NULL THEN 21
            ELSE GREATEST(0, 21 - EXTRACT(DAY FROM (now() - MIN(m.time)))::INT)
        END AS days_until_eligible
    FROM metering_point mp
    LEFT JOIN measurement m ON mp.id = m.metering_point_id
    LEFT JOIN site s ON mp.site_id = s.id
    WHERE mp.disabled_at IS NULL
    GROUP BY mp.id, mp.label, s.label
    ORDER BY days_until_eligible ASC, mp.label ASC;

    -- name: GetMPAnomalyRules :many
    -- Returns the three anomaly rules (p95, iqr, quiet_hour) that target this MP
    -- (scope_kind='metering_point' AND scope_id=$1) or are global (scope_kind='global').
    -- Used by the MP detail Anomaly Detection card to render per-rule toggles.
    SELECT id, rule_kind, severity, disabled_at IS NULL AS enabled,
           quiet_window_start, quiet_window_end, flow_threshold, days_of_week
    FROM alert_rule
    WHERE rule_kind IN ('anomaly_p95','anomaly_iqr','anomaly_quiet_hour')
      AND (
          (scope_kind = 'metering_point' AND scope_id = $1)
          OR scope_kind = 'global'
      );
    ```

    Run `sqlc generate`.

    Create `internal/alert/cold_start.go`:
    ```go
    package alert

    import (
        "context"
        "github.com/google/uuid"
        sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
    )

    // IsMPEligibleForAnomaly returns true if the MP has ≥ 21 days of history (D-16).
    // The 21-day constant is hardcoded in v1; Phase 7 may promote to retention_config.
    func IsMPEligibleForAnomaly(ctx context.Context, q *sqlc.Queries, mpID uuid.UUID) (bool, error) {
        eligible, err := q.IsMPEligibleForAnomaly(ctx, mpID)
        if err != nil { return false, err }
        return eligible, nil
    }

    // AnomalyEligibility is one row from the warmup roster.
    type AnomalyEligibility struct {
        MeteringPointID    uuid.UUID `json:"metering_point_id"`
        MeteringPointLabel string    `json:"metering_point_label"`
        SiteLabel          string    `json:"site_label"`
        DaysUntilEligible  int32     `json:"days_until_eligible"` // 0 = eligible
    }

    func ListAnomalyWarmupRoster(ctx context.Context, q *sqlc.Queries) ([]AnomalyEligibility, error) {
        rows, err := q.ListAnomalyWarmupRoster(ctx)
        if err != nil { return nil, err }
        out := make([]AnomalyEligibility, 0, len(rows))
        for _, r := range rows {
            out = append(out, AnomalyEligibility{
                MeteringPointID: r.MeteringPointID,
                MeteringPointLabel: r.MeteringPointLabel,
                SiteLabel: r.SiteLabel.String,
                DaysUntilEligible: int32(r.DaysUntilEligible),
            })
        }
        return out, nil
    }

    // MPAnomalyRuleState — per-MP per-kind enable state for the detail card toggles.
    type MPAnomalyRuleState struct {
        RuleKind string  // "anomaly_p95"|"anomaly_iqr"|"anomaly_quiet_hour"
        RuleID   *uuid.UUID // nil if no rule exists yet
        Enabled  bool
    }

    func GetMPAnomalyState(ctx context.Context, q *sqlc.Queries, mpID uuid.UUID) ([]MPAnomalyRuleState, error)
    ```

    Tests use testcontainers + seeded measurement fixtures at different ages.
  </action>
  <verify>
    <automated>go test ./internal/alert/... -run "TestColdStart|TestWarmupRoster" -count=1</automated>
  </verify>
  <acceptance_criteria>
    - `internal/alert/queries.sql` contains `name: IsMPEligibleForAnomaly :one` and `name: ListAnomalyWarmupRoster :many` and `INTERVAL '21 days'` (exact string)
    - `internal/alert/cold_start.go` contains `func IsMPEligibleForAnomaly(` and `func ListAnomalyWarmupRoster(` and `func GetMPAnomalyState(`
    - `internal/alert/cold_start.go` exports `type AnomalyEligibility struct` with `DaysUntilEligible int32` field
    - All 4 tests exist in `cold_start_test.go` and the package builds (`go build ./internal/alert/...`)
    - `go test ./internal/alert/... -run TestColdStart -count=1` passes
  </acceptance_criteria>
  <done>The 21-day eligibility predicate is the single source of truth for "should this MP get anomaly alerts?" — used by the worker AND the API endpoint that powers the MP detail card.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: P95 + IQR + Quiet-hour evaluators (D-17) + AnomalyWorker</name>
  <files>internal/alert/anomaly_worker.go, internal/alert/anomaly_worker_test.go, internal/alert/quiet_hour.go, internal/alert/quiet_hour_test.go, internal/alert/queries.sql, internal/cli/serve.go</files>
  <read_first>
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md §Decision E "Rule 1: anomaly_p95", "Rule 2: anomaly_iqr", "Rule 3: anomaly_quiet_hour" — exact SQL
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md §Common Pitfalls Pitfall 9 "Quiet-hour window crossing midnight is the SQL gotcha" — canonical OR-form query
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-CONTEXT.md §D-17 (three rule kinds, opt-in per MP)
    - internal/alert/payload.go (BuildPayloadInput has anomaly-specific fields)
    - internal/alert/cold_start.go (Task 1: IsMPEligibleForAnomaly)
    - internal/alert/threshold_worker.go (Plan 06-02 helpers — fire/clear pattern; reuse)
  </read_first>
  <behavior>
    - Test (TestAnomalyP95_FiresOnOutlier): MP with 30 days of measurements where 95% are ≤ 10 m³/h; latest measurement is 15 m³/h → fire with payload.p95_baseline=10.0, payload.value=15, payload.threshold=10.0.
    - Test (TestAnomalyP95_BucketIsHourOfDay): seed measurements where hour=14 has p95=10 but hour=02 has p95=2; latest measurement at hour=14 with value=11 fires; latest at hour=02 with value=11 also fires (different bucket).
    - Test (TestAnomalyIQR_FiresAboveQ3PlusOnePointFiveIQR): Q1=5, Q3=15 → IQR=10 → upper bound = 15 + 1.5*10 = 30; value=35 fires.
    - Test (TestAnomalyIQR_FiresBelowQ1MinusOnePointFiveIQR): value=-12 with same baseline fires (lower bound = 5 - 1.5*10 = -10; -12 < -10).
    - Test (TestAnomalyQuietHour_SameDayWindow): rule quiet_window=08:00–18:00, flow_threshold=0.1; measurement at 10:00 with instant_value=0.5 → fire.
    - Test (TestAnomalyQuietHour_CrossMidnight): rule quiet_window=22:00–06:00; measurement at 03:00 with instant_value=0.5 → fire (cross-midnight OR-form). Measurement at 10:00 → no fire.
    - Test (TestAnomalyQuietHour_RespectsInstallTimezone): install_tz='Asia/Bangkok' (+07); a UTC measurement at 19:00 (local 02:00) with 22:00→06:00 quiet window fires.
    - Test (TestAnomaly_ColdStartGatesAllRuleKinds): MP with only 5 days of history → no anomaly_p95 / anomaly_iqr / anomaly_quiet_hour rule fires.
    - Test (TestAnomaly_OptInPerMP): only enabled (disabled_at IS NULL) rules with matching scope evaluate; disabled or unscoped rules ignored.
  </behavior>
  <action>
    Append to `internal/alert/queries.sql`:
    ```sql
    -- name: P95BaselineForMPAndHour :one
    -- D-17 Rule 1: trailing-30-day P95 of instant_value for this MP at this hour-of-day.
    SELECT percentile_cont(0.95) WITHIN GROUP (ORDER BY instant_value) AS p95
    FROM measurement
    WHERE metering_point_id = $1
      AND time >= now() - INTERVAL '30 days'
      AND time < now()
      AND EXTRACT(HOUR FROM time)::INT = $2::INT;

    -- name: IQRBaselineForMP :one
    -- D-17 Rule 2: Q1, Q3 over trailing 30 days. Worker computes
    -- iqr = q3 - q1; lower = q1 - 1.5*iqr; upper = q3 + 1.5*iqr.
    SELECT
        percentile_cont(0.25) WITHIN GROUP (ORDER BY instant_value) AS q1,
        percentile_cont(0.75) WITHIN GROUP (ORDER BY instant_value) AS q3
    FROM measurement
    WHERE metering_point_id = $1
      AND time >= now() - INTERVAL '30 days'
      AND time < now();

    -- name: LatestInstantValueForMP :one
    SELECT instant_value, time
    FROM measurement
    WHERE metering_point_id = $1
    ORDER BY time DESC
    LIMIT 1;

    -- name: NonZeroFlowDuringQuietWindow :one
    -- D-17 Rule 3 + Pitfall 9 cross-midnight OR-form.
    -- $1=mp_id $2=flow_threshold $3=quiet_window_start TIME $4=quiet_window_end TIME $5=install_tz TEXT
    SELECT instant_value, time
    FROM measurement
    WHERE metering_point_id = $1
      AND time >= now() - INTERVAL '24 hours'
      AND instant_value > $2
      AND (
          -- Same-day window (start < end, e.g. 08:00→18:00)
          ($3::TIME < $4::TIME
           AND (time AT TIME ZONE $5)::TIME BETWEEN $3::TIME AND $4::TIME)
          OR
          -- Cross-midnight window (start >= end, e.g. 22:00→06:00)
          ($3::TIME >= $4::TIME
           AND ((time AT TIME ZONE $5)::TIME >= $3::TIME
                OR (time AT TIME ZONE $5)::TIME <= $4::TIME))
      )
    ORDER BY time DESC LIMIT 1;
    ```

    Run `sqlc generate`.

    Create `internal/alert/quiet_hour.go`:
    ```go
    package alert
    // EvalQuietHour returns true if there's a non-zero flow during the rule's
    // quiet window for this MP in the last 24h. install_tz is required for the
    // cross-midnight OR-form to apply local-time, not UTC.
    func EvalQuietHour(ctx context.Context, q *sqlc.Queries, mpID uuid.UUID, rule RuleRecord, installTZ string) (breach bool, value float64, atTime time.Time, err error) {
        if rule.QuietWindowStart == nil || rule.QuietWindowEnd == nil {
            return false, 0, time.Time{}, nil
        }
        threshold := 0.0
        if rule.FlowThreshold != nil { threshold = *rule.FlowThreshold }
        row, err := q.NonZeroFlowDuringQuietWindow(ctx, mpID, threshold, *rule.QuietWindowStart, *rule.QuietWindowEnd, installTZ)
        if err != nil {
            if errors.Is(err, pgx.ErrNoRows) { return false, 0, time.Time{}, nil }
            return false, 0, time.Time{}, err
        }
        return true, row.InstantValue, row.Time, nil
    }
    ```

    Create `internal/alert/anomaly_worker.go`:
    ```go
    type AnomalyArgs struct{}
    func (AnomalyArgs) Kind() string { return "alert_anomaly" }
    func (AnomalyArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{MaxAttempts: 3} }

    type AnomalyWorker struct {
        river.WorkerDefaults[AnomalyArgs]
        Eng        EvaluateContext
        Rules      *RuleStore
        Alerts     *AlertStore
        WorkerStat *WorkerStateStore
    }

    func (w *AnomalyWorker) Work(ctx context.Context, _ *river.Job[AnomalyArgs]) error {
        start := time.Now()
        state := &RunState{Kind: "anomaly"}
        defer func() {
            state.DurationMs = int(time.Since(start).Milliseconds())
            _ = w.WorkerStat.UpsertWorkerState(ctx, "anomaly", state)
        }()

        // Pull install_tz fresh each cycle per RESEARCH Open Question #4.
        installTZ, err := loadInstallTZ(ctx, w.Eng.Pool)
        if err != nil { return err }

        for _, kind := range []string{"anomaly_p95", "anomaly_iqr", "anomaly_quiet_hour"} {
            rules, err := w.Rules.ListActiveRulesByKind(ctx, kind)
            if err != nil { state.RecordErr(err); continue }
            for _, rule := range rules {
                // Cool-down (D-05)
                if rule.LastFiredAt != nil &&
                   time.Since(*rule.LastFiredAt) < time.Duration(rule.CooldownSeconds)*time.Second {
                    continue
                }
                targets, err := expandScope(ctx, w.Eng, rule)
                if err != nil { state.RecordErr(err); continue }
                for _, target := range targets {
                    // Cold-start gate (D-16) — skip if MP not eligible
                    eligible, err := IsMPEligibleForAnomaly(ctx, w.Eng.Queries, target.MeteringPointID)
                    if err != nil { state.RecordErr(err); continue }
                    if !eligible { continue }
                    state.RulesEvaluated++

                    var breach bool
                    var value, threshold float64
                    var extraPayload BuildPayloadInput

                    switch kind {
                    case "anomaly_p95":
                        breach, value, threshold, extraPayload, err = w.evalP95(ctx, target.MeteringPointID)
                    case "anomaly_iqr":
                        breach, value, threshold, extraPayload, err = w.evalIQR(ctx, target.MeteringPointID)
                    case "anomaly_quiet_hour":
                        breach, value, _, err = EvalQuietHour(ctx, w.Eng.Queries, target.MeteringPointID, rule, installTZ)
                        threshold = derefFloat(rule.FlowThreshold, 0.0)
                    }
                    if err != nil { state.RecordErr(err); continue }

                    existing, _ := w.Alerts.ListFiringByRuleTarget(ctx, rule.ID, target.MeteringPointID)
                    if breach && existing == nil {
                        if err := w.fireAnomaly(ctx, rule, target, value, threshold, kind, extraPayload); err != nil {
                            state.RecordErr(err); continue
                        }
                        state.FiresEmitted++
                    } else if !breach && existing != nil {
                        if err := clearAlert(ctx, w.Eng, w.Alerts, *existing); err != nil {
                            state.RecordErr(err); continue
                        }
                        state.Cleared++
                    }
                }
            }
        }
        return state.FirstErr
    }

    func (w *AnomalyWorker) evalP95(ctx context.Context, mpID uuid.UUID) (breach bool, value, threshold float64, extra BuildPayloadInput, err error) {
        hour := time.Now().Hour()
        p95, err := w.Eng.Queries.P95BaselineForMPAndHour(ctx, mpID, int32(hour))
        if err != nil { return }
        latest, err := w.Eng.Queries.LatestInstantValueForMP(ctx, mpID)
        if err != nil { return }
        value = latest.InstantValue
        threshold = p95
        breach = value > p95
        days := int32(30)
        bucket := fmt.Sprintf("%02d:00", hour)
        extra = BuildPayloadInput{P95Baseline: &p95, BaselineWindowDays: &days, TimeOfDayBucket: &bucket}
        return
    }

    func (w *AnomalyWorker) evalIQR(ctx context.Context, mpID uuid.UUID) (breach bool, value, threshold float64, extra BuildPayloadInput, err error) {
        row, err := w.Eng.Queries.IQRBaselineForMP(ctx, mpID)
        if err != nil { return }
        iqr := row.Q3 - row.Q1
        lower := row.Q1 - 1.5*iqr
        upper := row.Q3 + 1.5*iqr
        latest, err := w.Eng.Queries.LatestInstantValueForMP(ctx, mpID)
        if err != nil { return }
        value = latest.InstantValue
        breach = value > upper || value < lower
        threshold = upper // payload reports upper bound when breach is above; lower when below — caller handles
        if value < lower { threshold = lower }
        days := int32(30)
        extra = BuildPayloadInput{BaselineWindowDays: &days}
        return
    }
    ```

    `loadInstallTZ(ctx, pool)` queries `SELECT timezone FROM install_state WHERE id = 1` (returns 'Asia/Bangkok' style IANA name).

    Register in `internal/cli/serve.go`:
    ```go
    river.AddWorker(riverWorkers, &alert.AnomalyWorker{Eng: alertEng, Rules: ruleStore, Alerts: alertStore, WorkerStat: workerStat})
    periodicJobs = append(periodicJobs,
        river.NewPeriodicJob(river.PeriodicInterval(1*time.Hour), func() (river.JobArgs, *river.InsertOpts) { return alert.AnomalyArgs{}, nil }, nil),
    )
    ```
  </action>
  <verify>
    <automated>go test ./internal/alert/... -run "TestAnomaly" -count=1 -timeout=180s</automated>
  </verify>
  <acceptance_criteria>
    - `internal/alert/queries.sql` contains `P95BaselineForMPAndHour`, `IQRBaselineForMP`, `NonZeroFlowDuringQuietWindow`
    - `NonZeroFlowDuringQuietWindow` SQL contains the cross-midnight OR-form: grep for both `\$3::TIME < \$4::TIME` AND `\$3::TIME >= \$4::TIME`
    - `internal/alert/quiet_hour.go` contains `func EvalQuietHour(` and the comment `cross-midnight`
    - `internal/alert/anomaly_worker.go` contains `IsMPEligibleForAnomaly` call (cold-start gate) BEFORE the rule-kind switch
    - `internal/alert/anomaly_worker.go` switches on all 3 rule kinds: `"anomaly_p95"`, `"anomaly_iqr"`, `"anomaly_quiet_hour"`
    - `internal/cli/serve.go` adds `&alert.AnomalyWorker{` and `river.PeriodicInterval(1*time.Hour)` with `alert.AnomalyArgs{}`
    - All 9 listed test names exist in `anomaly_worker_test.go` + `quiet_hour_test.go`
    - `go test ./internal/alert/... -run TestAnomaly -count=1` passes
    - Test `TestAnomaly_ColdStartGatesAllRuleKinds` proves the cold-start gate prevents ALL three rule kinds (not just one) from firing on a < 21-day MP
  </acceptance_criteria>
  <done>All three statistical rule kinds (P95 / IQR / quiet-hour) ship in v1 with the cold-start gate enforced; the quiet-hour SQL handles cross-midnight correctly per Pitfall 9.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| River worker → DB | Same as Plan 06-02 |
| install_state.timezone → SQL | The timezone string is set by the operator in the install wizard (Phase 1 INST-02) and is trusted; the `time AT TIME ZONE $5` SQL is parameter-bound (no injection surface) |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-06-03-01 | DoS | anomaly worker fires constantly on a new install | mitigate | D-16 cold-start gate: `IsMPEligibleForAnomaly` is called BEFORE every rule evaluation for every target; MPs with < 21 days of history are silently skipped (no fire, no audit row). Test: TestAnomaly_ColdStartGatesAllRuleKinds. |
| T-06-03-02 | Tampering | cross-midnight quiet-hour SQL bypass | mitigate | Pitfall 9 canonical OR-form is the only SQL pattern used; tests cover both same-day and cross-midnight cases. The TIME parameters are bound positionally; the install_tz parameter is bound as TEXT and applied via `AT TIME ZONE` operator (Postgres rejects invalid tz strings at parse time). |
| T-06-03-03 | Information Disclosure | P95 baseline leaks to wrong scope | accept | Each baseline query is parameterized on `$1 = mp_id` and only the rule's own scope; cross-MP leak requires an existing DB compromise. AUTH-06 RBAC gates the rule-creation surface. |
| T-06-03-04 | DoS | percentile_cont scan cost at 500 MPs | accept (Phase 7 review) | RESEARCH §Decision E performance estimate: 30d × hour-bucket × 1 MP ≈ 2,880 rows; at 500 MPs hourly ≈ 1.44M rows/hour — trivially fast on the (metering_point_id, time DESC) index. Assumption A4 in RESEARCH says Wave 0 should EXPLAIN; defer hard tuning to Phase 7 when real-customer data is available. |
| T-06-03-05 | Repudiation | anomaly fire/clear untracked | mitigate | Same audit-in-tx pattern as Plan 06-02: every fire writes 'alert.fired'; every clear writes 'alert.cleared'. |
</threat_model>

<verification>
- Cold-start eligibility roster query is the single source of truth used by both worker (gate) and API (Plan 06-04 / 06-10 surface to UI)
- All three anomaly rule kinds fire when breach occurs AND MP is eligible
- Cross-midnight quiet-hour windows work correctly (3am during 22:00→06:00 fires)
- Hourly cadence keeps worker load bounded; cool-down prevents flap on borderline cases
- `go test ./internal/alert/... -run TestAnomaly -count=1 -timeout=180s` passes
</verification>

<success_criteria>
- ALERT-04 covered: P95 + IQR + quiet-hour rule kinds ship; cold-start gate (21 days) enforced
- D-16 surface ready: warmup roster + per-MP anomaly state endpoints available for UI plans
- D-17 forward-compat: per-rule quiet_window_start/end/flow_threshold + days_of_week stored on alert_rule
- Pitfall 9 cross-midnight SQL pattern landed and tested
</success_criteria>

<output>
After completion, create `.planning/phases/06-alerts-users-audit-operational-hardening/06-03-SUMMARY.md`
</output>
