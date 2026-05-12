---
phase: 06-alerts-users-audit-operational-hardening
plan: 02
type: execute
wave: 2
depends_on: [06-01]
files_modified:
  - internal/alert/threshold_worker.go
  - internal/alert/threshold_worker_test.go
  - internal/alert/offline_worker.go
  - internal/alert/offline_worker_test.go
  - internal/alert/payload.go
  - internal/alert/payload_test.go
  - internal/alert/queries.sql
  - internal/cli/serve.go
autonomous: true
requirements: [ALERT-01, ALERT-02, ALERT-03]
must_haves:
  truths:
    - "ThresholdInstantaneousWorker queries the latest measurement for each rule's MP and fires when value breaches high_bound/low_bound + comparison"
    - "ThresholdHourlyWorker queries cagg_hourly; ThresholdDailyWorker queries cagg_daily — same fire/clear logic, different source table"
    - "OfflineWorker fires ALERT-02 only when now() - last_uplink > 3 × expected_interval_s and clears when < 2× (hysteresis per D-15)"
    - "OfflineWorker suppresses device-offline alerts when the device's gateway is itself offline (D-14) and emits a single offline_gateway alert per downed gateway with suppresses_n_devices populated"
    - "Cool-down per D-05 (default 900s) is enforced in-engine: a rule with last_fired_at+cooldown_seconds > now() is skipped"
    - "Each fire writes audit_log row 'alert.fired' in same tx as the alert insert; each auto-clear writes 'alert.cleared'"
    - "Periodic jobs registered: threshold_instantaneous every 1min, threshold_hourly every 15min, threshold_daily every 1h, offline every 2min"
  artifacts:
    - path: internal/alert/threshold_worker.go
      provides: "ThresholdInstantaneousWorker + ThresholdHourlyWorker + ThresholdDailyWorker"
      contains: "func (w *ThresholdInstantaneousWorker) Work"
    - path: internal/alert/offline_worker.go
      provides: "OfflineWorker with gateway-down suppression"
      contains: "GatewayOffline"
    - path: internal/alert/payload.go
      provides: "BuildPayload(rule, target, value, threshold) per D-12 canonical shape"
      contains: "rule_kind"
  key_links:
    - from: internal/alert/threshold_worker.go
      to: internal/alert/payload.go
      via: "buildPayload constructs D-12 JSONB before InsertAlert"
      pattern: "BuildPayload\\("
    - from: internal/alert/offline_worker.go
      to: alert (table)
      via: "InsertAlert with target_entity_type='device' or 'gateway'"
      pattern: "target_entity_type"
---

<objective>
Implement the three threshold evaluator subtypes (instantaneous/hourly/daily per D-02) and the offline evaluator with gateway-down suppression (D-14, D-15). Closes ALERT-01, ALERT-02, ALERT-03.

Purpose: the threshold + offline rules are the operator's first-line alert surface. They MUST fire only on real conditions (no flap-spam, no false device alerts during gateway outages) and they MUST emit the D-12 canonical payload from day 1 so future webhook delivery (V2-NOTIF-02) ships without migration.

Output: 4 River workers (3 threshold subtypes + 1 offline) + shared payload builder + sqlc queries + cli/serve.go registration with the per-subtype cadences from RESEARCH §Decision C.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/06-alerts-users-audit-operational-hardening/06-CONTEXT.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-01-alert-engine-substrate-PLAN.md
@internal/alert/engine.go
@internal/alert/rule_store.go
@internal/alert/alert_store.go
@internal/audit/log.go
@internal/cli/serve.go
@internal/db/migrations/0015_measurement.up.sql
@internal/db/migrations/0026_cagg_daily.up.sql

<interfaces>
<!-- Plan 06-01 substrate types -->

internal/alert/engine.go (from Plan 06-01):
```go
type EvaluateContext struct { Pool *pgxpool.Pool; Queries *sqlc.Queries; Hub *events.Hub; InstallTZ *time.Location; Log *slog.Logger }
func CompareBound(value float64, high *float64, low *float64) bool
```

internal/alert/rule_store.go:
```go
type RuleRecord struct {
    ID uuid.UUID; RuleKind string; ScopeKind string; ScopeID uuid.UUID
    HighBound *float64; LowBound *float64; Comparison *string; Unit *string
    QuietWindowStart *time.Time; QuietWindowEnd *time.Time; FlowThreshold *float64; DaysOfWeek *int32
    Severity string; Name *string; Notes *string; CooldownSeconds int32
    LastFiredAt *time.Time; DisabledAt *time.Time
    CreatedBy *uuid.UUID; CreatedAt time.Time; UpdatedAt time.Time
}
func (s *RuleStore) ListActiveRulesByKind(ctx context.Context, kind string) ([]RuleRecord, error)
func (s *RuleStore) TouchLastFiredAt(ctx context.Context, tx pgx.Tx, id uuid.UUID) error
```

internal/alert/alert_store.go:
```go
type AlertRecord struct { ID uuid.UUID; RuleID uuid.UUID; ... Payload []byte }
func (s *AlertStore) InsertAlert(ctx context.Context, tx pgx.Tx, p InsertParams) (AlertRecord, error)
func (s *AlertStore) ListFiringByRuleTarget(ctx context.Context, ruleID uuid.UUID, targetID uuid.UUID) (*AlertRecord, error)
func (s *AlertStore) ClearAlert(ctx context.Context, tx pgx.Tx, id uuid.UUID) error
```

internal/audit/log.go (Phase 6 constants from Plan 06-01):
```go
ActionAlertFired   = "alert.fired"
ActionAlertCleared = "alert.cleared"
EntityTypeAlert    = "alert"
```

Existing measurement schema (internal/db/migrations/0015_measurement.up.sql):
- `measurement` hypertable: (time TIMESTAMPTZ, metering_point_id UUID, instant_value DOUBLE PRECISION, cumulative_value DOUBLE PRECISION, ...)

Existing CAGG schemas:
- `measurement_hourly`: (bucket TIMESTAMPTZ, metering_point_id UUID, avg_instant DOUBLE PRECISION, sum_consumption DOUBLE PRECISION, ...)
- `measurement_daily`: same shape, daily bucket

device + device_profile schemas (Phase 2):
- `device`: (id, ..., last_uplink_at TIMESTAMPTZ, profile_id UUID REFERENCES device_profile(id), gateway_id UUID NULLABLE)
- `device_profile`: (id, expected_interval_s INTEGER, ...)
- `gateway`: (id, last_seen_at TIMESTAMPTZ, ...)
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: D-12 payload builder + sqlc queries for threshold + offline evaluators</name>
  <files>internal/alert/payload.go, internal/alert/payload_test.go, internal/alert/queries.sql, internal/db/sqlc/* (generated)</files>
  <read_first>
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md §Decision C "D-12 Payload Schema (canonical)" — exact JSON shape
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md §Decision D "ALERT-02 Offline Detection" — exact SQL
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md §Decision D "D-14 Gateway Suppression — One Query, Eval-Time" — exact SQL
    - internal/db/sqlc/queries.sql (existing queries.sql file location) and sqlc.yaml
    - internal/install/state.go (install_state has display_name; payload includes install.display_name)
  </read_first>
  <behavior>
    - Test: `BuildPayload(rule, target, value, threshold)` returns JSON bytes parseable as map with keys exactly: `rule_id, rule_kind, severity, target.entity_type, target.entity_id, target.label, value, threshold, comparison, unit, fired_at, install.display_name` — no extra top-level keys.
    - Test: for `rule_kind="offline_device"` the payload also has `last_uplink_at` and `expected_interval_s`; for `rule_kind="offline_gateway"` it has `suppresses_n_devices`.
    - Test: `comparison` is the string "gt"/"gte"/"lt"/"lte"/"eq" not the symbol >/>=/etc.
    - Test (TestQueries_ListOfflineDevicesWithGatewayStatus): seeded test fixture with one MP+device whose last_uplink_at is 5×expected_interval_s ago and gateway last_seen_at is also stale → query returns row with `gateway_offline = TRUE`.
  </behavior>
  <action>
    Create `internal/alert/payload.go`:
    ```go
    package alert

    import (
        "encoding/json"
        "time"
        "github.com/google/uuid"
    )

    // PayloadTarget mirrors the D-12 stable schema target sub-object.
    type PayloadTarget struct {
        EntityType string    `json:"entity_type"`
        EntityID   uuid.UUID `json:"entity_id"`
        Label      string    `json:"label"`
    }

    // PayloadInstall mirrors the D-12 install sub-object.
    type PayloadInstall struct {
        DisplayName string `json:"display_name"`
    }

    // BuildPayloadInput is the value bundle the workers fill in before
    // serializing. Keep the shape stable — webhook deliverer in V2-NOTIF-01
    // will deserialize this same shape.
    type BuildPayloadInput struct {
        RuleID         uuid.UUID
        RuleKind       string  // "threshold_instantaneous" | ... | "offline_gateway"
        Severity       string  // "info"|"warning"|"critical"
        Target         PayloadTarget
        Value          float64
        Threshold      float64
        Comparison     string  // "gt"|"gte"|"lt"|"lte"|"eq"
        Unit           string
        FiredAt        time.Time
        InstallName    string
        // Offline-specific extensions (nil for non-offline rule kinds):
        LastUplinkAt        *time.Time `json:"last_uplink_at,omitempty"`
        ExpectedIntervalS   *int32     `json:"expected_interval_s,omitempty"`
        // Gateway-suppression extension (only on offline_gateway):
        SuppressesNDevices  *int32     `json:"suppresses_n_devices,omitempty"`
        // Anomaly extensions (used by Plan 06-03 — leave nil for threshold/offline):
        P95Baseline       *float64 `json:"p95_baseline,omitempty"`
        BaselineWindowDays *int32  `json:"baseline_window_days,omitempty"`
        TimeOfDayBucket   *string  `json:"time_of_day_bucket,omitempty"`
    }

    // BuildPayload serializes BuildPayloadInput into the D-12 canonical
    // JSON shape; returns the bytes ready for alert.payload JSONB column.
    func BuildPayload(in BuildPayloadInput) ([]byte, error) {
        wire := map[string]any{
            "rule_id":   in.RuleID,
            "rule_kind": in.RuleKind,
            "severity":  in.Severity,
            "target": map[string]any{
                "entity_type": in.Target.EntityType,
                "entity_id":   in.Target.EntityID,
                "label":       in.Target.Label,
            },
            "value":      in.Value,
            "threshold":  in.Threshold,
            "comparison": in.Comparison,
            "unit":       in.Unit,
            "fired_at":   in.FiredAt.UTC().Format(time.RFC3339),
            "install":    map[string]any{"display_name": in.InstallName},
        }
        if in.LastUplinkAt != nil { wire["last_uplink_at"] = in.LastUplinkAt.UTC().Format(time.RFC3339) }
        if in.ExpectedIntervalS != nil { wire["expected_interval_s"] = *in.ExpectedIntervalS }
        if in.SuppressesNDevices != nil { wire["suppresses_n_devices"] = *in.SuppressesNDevices }
        if in.P95Baseline != nil { wire["p95_baseline"] = *in.P95Baseline }
        if in.BaselineWindowDays != nil { wire["baseline_window_days"] = *in.BaselineWindowDays }
        if in.TimeOfDayBucket != nil { wire["time_of_day_bucket"] = *in.TimeOfDayBucket }
        return json.Marshal(wire)
    }
    ```

    Create `internal/alert/queries.sql` (sqlc-annotated; copy RESEARCH §Decision D queries verbatim):

    ```sql
    -- name: ListOfflineDevicesWithGatewayStatus :many
    -- D-15 + D-14: returns device-offline candidates AND a gateway-offline flag
    -- per-row. Worker uses the flag to suppress device alerts behind downed
    -- gateways and to emit a single gateway-offline alert.
    SELECT
        d.id AS device_id,
        d.label AS device_label,
        d.last_uplink_at,
        dp.expected_interval_s,
        d.metering_point_id,
        g.id AS gateway_id,
        g.label AS gateway_label,
        g.last_seen_at AS gateway_last_seen_at,
        (
            g.id IS NOT NULL
            AND g.last_seen_at IS NOT NULL
            AND now() - g.last_seen_at > make_interval(secs => 3 * dp.expected_interval_s)
        ) AS gateway_offline
    FROM device d
    JOIN device_profile dp ON d.profile_id = dp.id
    LEFT JOIN gateway g    ON d.gateway_id = g.id
    WHERE d.disabled_at IS NULL
      AND d.last_uplink_at IS NOT NULL
      AND now() - d.last_uplink_at > make_interval(secs => 3 * dp.expected_interval_s);

    -- name: ListHysteresisClearOffline :many
    -- D-15 hysteresis: devices currently 'firing' an offline alert whose
    -- last_uplink is back inside 2× expected_interval — those clear.
    SELECT a.id AS alert_id, d.id AS device_id, a.rule_id
    FROM alert a
    JOIN device d ON a.target_entity_id = d.id
    JOIN device_profile dp ON d.profile_id = dp.id
    WHERE a.state = 'firing'
      AND a.rule_kind = 'offline_device'
      AND now() - d.last_uplink_at < make_interval(secs => 2 * dp.expected_interval_s);

    -- name: GetLatestMeasurementForMP :one
    SELECT time, metering_point_id, instant_value, cumulative_value
    FROM measurement
    WHERE metering_point_id = $1
    ORDER BY time DESC
    LIMIT 1;

    -- name: GetLatestHourlyForMP :one
    SELECT bucket, metering_point_id, avg_instant, sum_consumption
    FROM measurement_hourly
    WHERE metering_point_id = $1
    ORDER BY bucket DESC
    LIMIT 1;

    -- name: GetLatestDailyForMP :one
    SELECT bucket, metering_point_id, avg_instant, sum_consumption
    FROM measurement_daily
    WHERE metering_point_id = $1
    ORDER BY bucket DESC
    LIMIT 1;
    ```

    Run `sqlc generate`; commit the new queries in `internal/db/sqlc/`.
  </action>
  <verify>
    <automated>go test ./internal/alert/... -run "TestBuildPayload|TestQueries_ListOfflineDevicesWithGatewayStatus" -count=1</automated>
  </verify>
  <acceptance_criteria>
    - `internal/alert/payload.go` contains `func BuildPayload(in BuildPayloadInput) ([]byte, error)` and the wire shape has exactly the D-12 top-level keys
    - `internal/alert/queries.sql` contains `name: ListOfflineDevicesWithGatewayStatus :many` and the literal string `gateway_offline`
    - sqlc generates `internal/db/sqlc/queries.sql.go` with `ListOfflineDevicesWithGatewayStatus` function
    - `go test ./internal/alert/... -run TestBuildPayload -count=1` passes
    - `TestQueries_ListOfflineDevicesWithGatewayStatus` exists and asserts `Row.GatewayOffline == true` for the seeded stale-gateway fixture
  </acceptance_criteria>
  <done>Payload shape is locked + tested; the suppression query returns the gateway_offline flag in one SQL round-trip per eval cycle (no N+1).</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: ThresholdInstantaneousWorker + ThresholdHourlyWorker + ThresholdDailyWorker (D-02, ALERT-01)</name>
  <files>internal/alert/threshold_worker.go, internal/alert/threshold_worker_test.go, internal/cli/serve.go</files>
  <read_first>
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md §Code Examples "Alert Worker — Threshold Subtype (full)" — copy verbatim as starting point
    - internal/alert/rule_store.go (from Plan 06-01)
    - internal/alert/alert_store.go (from Plan 06-01)
    - internal/alert/engine.go (CompareBound semantics)
    - internal/cli/serve.go (existing River worker registration around lines 334-357)
  </read_first>
  <behavior>
    - Test (TestThresholdInstantaneous_FiresOnBreach): seed MP + measurement with instant_value=42.7; create rule with high_bound=40.0, comparison='gt'; run worker → exactly one new alert row exists with state='firing', severity='critical', payload.value=42.7, payload.threshold=40.0.
    - Test (TestThresholdInstantaneous_AutoClearsOnReturn): after a fire exists, seed a fresh measurement with instant_value=30; run worker → alert row's state='cleared', cleared_at set, audit row 'alert.cleared' present.
    - Test (TestThresholdInstantaneous_CooldownSuppresses): rule with last_fired_at=now()-60s and cooldown_seconds=900 → worker skips evaluation (no second fire).
    - Test (TestThresholdInstantaneous_IdempotentFire): run worker twice in a row with breach condition unchanged → only one firing alert exists (partial-unique-index alert_firing_unique_idx ensures idempotency).
    - Test (TestThresholdHourly_QueriesCaggHourly): rule_kind='threshold_hourly' worker reads from `measurement_hourly`, not `measurement`.
    - Test (TestThresholdDaily_QueriesCaggDaily): rule_kind='threshold_daily' reads from `measurement_daily`.
    - Test (TestThresholdAuditTransactionAtomicity): a forced error in `WriteEntry` between InsertAlert and Commit causes the alert insert to roll back — no orphan alert row, no orphan audit row.
  </behavior>
  <action>
    Create `internal/alert/threshold_worker.go` with three worker structs (Instantaneous/Hourly/Daily) sharing a common fire/clear helper. Pattern from RESEARCH §Code Examples lines 2017-2094:

    ```go
    type ThresholdInstantaneousArgs struct{}
    func (ThresholdInstantaneousArgs) Kind() string { return "alert_threshold_instantaneous" }
    func (ThresholdInstantaneousArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{MaxAttempts: 3} }

    type ThresholdInstantaneousWorker struct {
        river.WorkerDefaults[ThresholdInstantaneousArgs]
        Eng        EvaluateContext   // from Plan 06-01
        Rules      *RuleStore
        Alerts     *AlertStore
        WorkerStat *WorkerStateStore
    }

    func (w *ThresholdInstantaneousWorker) Work(ctx context.Context, _ *river.Job[ThresholdInstantaneousArgs]) error {
        return runThresholdCycle(ctx, w.Eng, w.Rules, w.Alerts, w.WorkerStat,
            "threshold_instantaneous",
            func(ctx context.Context, mpID uuid.UUID) (float64, time.Time, error) {
                m, err := w.Eng.Queries.GetLatestMeasurementForMP(ctx, mpID)
                if err != nil { return 0, time.Time{}, err }
                return m.InstantValue, m.Time, nil
            })
    }
    ```

    Identical structure for `ThresholdHourlyWorker` (uses `GetLatestHourlyForMP` returning `AvgInstant`) and `ThresholdDailyWorker` (uses `GetLatestDailyForMP` returning `SumConsumption` for daily totals — per D-02 daily checks the day's consumption total).

    Shared `runThresholdCycle`:
    ```go
    func runThresholdCycle(ctx context.Context, eng EvaluateContext, rules *RuleStore,
        alerts *AlertStore, ws *WorkerStateStore, kind string,
        latestFn func(ctx context.Context, mpID uuid.UUID) (float64, time.Time, error)) error {
        start := time.Now()
        state := &RunState{Kind: kind}
        defer func() {
            state.DurationMs = int(time.Since(start).Milliseconds())
            _ = ws.UpsertWorkerState(ctx, "threshold_instantaneous_or_collapse_to_parent", state) // see kind→worker_kind mapping
        }()

        rs, err := rules.ListActiveRulesByKind(ctx, kind)
        if err != nil { return err }
        for _, rule := range rs {
            // Cool-down (D-05)
            if rule.LastFiredAt != nil &&
               time.Since(*rule.LastFiredAt) < time.Duration(rule.CooldownSeconds)*time.Second {
                continue
            }
            targets, err := expandScope(ctx, eng, rule) // metering_point | site | global → list of MP UUIDs + labels
            if err != nil { state.RecordErr(err); continue }
            for _, target := range targets {
                value, _, err := latestFn(ctx, target.MeteringPointID)
                if err != nil { state.RecordErr(err); continue }
                state.RulesEvaluated++
                breached := compareBound(value, rule.HighBound, rule.LowBound, rule.Comparison)
                existing, _ := alerts.ListFiringByRuleTarget(ctx, rule.ID, target.MeteringPointID)
                if breached && existing == nil {
                    if err := fireThresholdAlert(ctx, eng, alerts, rules, rule, target, value); err != nil {
                        state.RecordErr(err); continue
                    }
                    state.FiresEmitted++
                } else if !breached && existing != nil {
                    if err := clearAlert(ctx, eng, alerts, *existing); err != nil {
                        state.RecordErr(err); continue
                    }
                    state.Cleared++
                }
            }
        }
        return state.FirstErr
    }

    func fireThresholdAlert(ctx context.Context, eng EvaluateContext, alerts *AlertStore, rules *RuleStore,
        rule RuleRecord, target Target, value float64) error {
        tx, err := eng.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
        if err != nil { return err }
        defer tx.Rollback(ctx)
        threshold := pickThreshold(rule)
        payload, _ := BuildPayload(BuildPayloadInput{
            RuleID: rule.ID, RuleKind: rule.RuleKind, Severity: rule.Severity,
            Target: PayloadTarget{EntityType: "metering_point", EntityID: target.MeteringPointID, Label: target.Label},
            Value: value, Threshold: threshold,
            Comparison: derefStr(rule.Comparison, "gt"),
            Unit: derefStr(rule.Unit, ""),
            FiredAt: time.Now().UTC(),
            InstallName: eng.InstallDisplayName(), // helper reads install_state.display_name once per cycle
        })
        alertRow, err := alerts.InsertAlert(ctx, tx, AlertInsertParams{
            RuleID: rule.ID, RuleKind: rule.RuleKind, Severity: rule.Severity,
            Payload: payload, TargetEntityType: "metering_point", TargetEntityID: target.MeteringPointID,
        })
        if err != nil { return err }
        if err := audit.WriteEntry(ctx, tx, audit.Entry{
            Action: audit.ActionAlertFired, EntityType: audit.EntityTypeAlert,
            EntityID: alertRow.ID, After: payloadToMap(payload),
        }); err != nil { return err }
        if err := rules.TouchLastFiredAt(ctx, tx, rule.ID); err != nil { return err }
        if err := tx.Commit(ctx); err != nil { return err }
        eng.Hub.Publish(events.AlertTopic, alertRow) // optional optimization for drawer auto-refresh
        return nil
    }
    ```

    `compareBound(value, high, low, comparison)` returns true if (high != nil && value > high) OR (low != nil && value < low). When `rule.Comparison` is set ("gt"/"gte"/"lt"/"lte"/"eq") it's used to refine the operator (e.g., "gte" → value >= high).

    `expandScope(ctx, eng, rule)` is a helper that:
    - if `rule.ScopeKind == "metering_point"`: returns `[{MeteringPointID: rule.ScopeID, Label: <load MP label>}]`
    - if `rule.ScopeKind == "site"`: returns all MPs in that site (one SELECT)
    - if `rule.ScopeKind == "global"`: returns all active MPs
    - if `rule.ScopeKind == "device"` or `"gateway"`: returns the MPs currently bound — threshold rules typically target MPs but device-scope rules may exist for instant_value (still resolves to MP via current binding)

    In `internal/cli/serve.go`, add 3 worker registrations + 3 periodic jobs near the existing River setup:
    ```go
    river.AddWorker(riverWorkers, &alert.ThresholdInstantaneousWorker{Eng: alertEng, Rules: ruleStore, Alerts: alertStore, WorkerStat: workerStat})
    river.AddWorker(riverWorkers, &alert.ThresholdHourlyWorker{Eng: alertEng, Rules: ruleStore, Alerts: alertStore, WorkerStat: workerStat})
    river.AddWorker(riverWorkers, &alert.ThresholdDailyWorker{Eng: alertEng, Rules: ruleStore, Alerts: alertStore, WorkerStat: workerStat})
    // Periodic schedule per RESEARCH §Decision C:
    periodicJobs = append(periodicJobs,
        river.NewPeriodicJob(river.PeriodicInterval(1*time.Minute), func() (river.JobArgs, *river.InsertOpts) { return alert.ThresholdInstantaneousArgs{}, nil }, nil),
        river.NewPeriodicJob(river.PeriodicInterval(15*time.Minute), func() (river.JobArgs, *river.InsertOpts) { return alert.ThresholdHourlyArgs{}, nil }, nil),
        river.NewPeriodicJob(river.PeriodicInterval(1*time.Hour), func() (river.JobArgs, *river.InsertOpts) { return alert.ThresholdDailyArgs{}, nil }, nil),
    )
    ```

    Bump River max workers from 4 (Phase 5) to 8 to accommodate the new periodic flux.
  </action>
  <verify>
    <automated>go test ./internal/alert/... -run "TestThreshold" -count=1 -timeout=120s</automated>
  </verify>
  <acceptance_criteria>
    - `internal/alert/threshold_worker.go` contains `ThresholdInstantaneousWorker`, `ThresholdHourlyWorker`, `ThresholdDailyWorker` struct definitions (grep each)
    - Each worker has `Kind()` returning `"alert_threshold_instantaneous"`, `"alert_threshold_hourly"`, `"alert_threshold_daily"`
    - Each worker has `InsertOpts() river.InsertOpts { return river.InsertOpts{MaxAttempts: 3} }`
    - `internal/cli/serve.go` registers all 3 workers AND adds 3 PeriodicJob entries with intervals `1*time.Minute`, `15*time.Minute`, `1*time.Hour`
    - `internal/cli/serve.go` bumps River QueueDefault MaxWorkers to 8: grep `MaxWorkers: 8`
    - `TestThresholdInstantaneous_FiresOnBreach` + `TestThresholdInstantaneous_AutoClearsOnReturn` + `TestThresholdInstantaneous_CooldownSuppresses` + `TestThresholdInstantaneous_IdempotentFire` + `TestThresholdHourly_QueriesCaggHourly` + `TestThresholdDaily_QueriesCaggDaily` + `TestThresholdAuditTransactionAtomicity` all exist in `internal/alert/threshold_worker_test.go`
    - `go test ./internal/alert/... -run TestThreshold -count=1` passes
  </acceptance_criteria>
  <done>The threshold engine fires on real breaches, clears on resolution, respects cool-down, never double-fires, and writes audit-in-tx for every state transition.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 3: OfflineWorker with gateway-down suppression (ALERT-02, ALERT-03)</name>
  <files>internal/alert/offline_worker.go, internal/alert/offline_worker_test.go, internal/cli/serve.go</files>
  <read_first>
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md §Decision D "ALERT-02 Offline Detection (D-15)" + "D-14 Gateway Suppression — One Query, Eval-Time" — copy worker logic verbatim
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-CONTEXT.md §D-14, §D-15 (the two-threshold hysteresis explanation, intentionally stricter than Phase 4 D-07)
    - internal/alert/queries.sql (from Task 1: ListOfflineDevicesWithGatewayStatus, ListHysteresisClearOffline)
    - internal/alert/threshold_worker.go (Task 2: fireAlert / clearAlert helpers — reuse via shared package functions)
  </read_first>
  <behavior>
    - Test (TestOffline_FiresAtThreeXInterval): device with expected_interval_s=900 and last_uplink_at=now()-2700s (3×) → worker fires one `offline_device` alert. last_uplink_at=now()-1800s (2×) → no fire.
    - Test (TestOffline_HysteresisClears): existing firing offline alert; new uplink arrives setting last_uplink_at=now()-1500s (< 2×900=1800) → worker clears the alert.
    - Test (TestOffline_HysteresisGracePreventsFlap): device at 2.5× interval → neither fires nor clears (stays in current state).
    - Test (TestOffline_GatewaySuppressesDeviceAlerts): 12 devices all behind gateway G; G's last_seen_at is 4× expected_interval_s ago; all 12 devices' last_uplink_at is 4× interval ago → worker emits exactly one `offline_gateway` alert (target_entity_type='gateway', target_entity_id=G.id, payload.suppresses_n_devices=12) and ZERO `offline_device` alerts.
    - Test (TestOffline_GatewayRecoveryClearsSuppression): gateway G comes back online (last_seen_at=now()) → next worker cycle clears the offline_gateway alert AND naturally re-evaluates each device (which may still be offline → fire device alerts then).
    - Test (TestOffline_AuditInTx): a forced error in WriteEntry rolls back the alert insert.
  </behavior>
  <action>
    Create `internal/alert/offline_worker.go`:
    ```go
    type OfflineArgs struct{}
    func (OfflineArgs) Kind() string { return "alert_offline" }
    func (OfflineArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{MaxAttempts: 3} }

    type OfflineWorker struct {
        river.WorkerDefaults[OfflineArgs]
        Eng        EvaluateContext
        Rules      *RuleStore
        Alerts     *AlertStore
        WorkerStat *WorkerStateStore
    }

    func (w *OfflineWorker) Work(ctx context.Context, _ *river.Job[OfflineArgs]) error {
        start := time.Now()
        state := &RunState{Kind: "offline"}
        defer func() {
            state.DurationMs = int(time.Since(start).Milliseconds())
            _ = w.WorkerStat.UpsertWorkerState(ctx, "offline", state)
        }()

        // 1. Load active offline_device + offline_gateway rules. If none, skip.
        deviceRules, err := w.Rules.ListActiveRulesByKind(ctx, "offline_device")
        if err != nil { return err }
        gatewayRules, err := w.Rules.ListActiveRulesByKind(ctx, "offline_gateway")
        if err != nil { return err }
        if len(deviceRules) == 0 && len(gatewayRules) == 0 { return nil }

        // 2. One query returns candidates AND gateway_offline flag
        candidates, err := w.Eng.Queries.ListOfflineDevicesWithGatewayStatus(ctx)
        if err != nil { return err }

        suppressedByGateway := map[uuid.UUID][]offlineCandidate{} // gateway_id → list

        for _, c := range candidates {
            if c.GatewayOffline && c.GatewayID.Valid {
                // D-14: suppress device alert; tally for gateway-down alert
                gwID := c.GatewayID.Bytes
                suppressedByGateway[gwID] = append(suppressedByGateway[gwID], c)
                continue
            }
            // Cool-down + idempotent fire for offline_device rules
            for _, rule := range deviceRules {
                if !scopeMatchesDevice(rule, c) { continue }
                if rule.LastFiredAt != nil &&
                   time.Since(*rule.LastFiredAt) < time.Duration(rule.CooldownSeconds)*time.Second {
                    continue
                }
                existing, _ := w.Alerts.ListFiringByRuleTarget(ctx, rule.ID, c.DeviceID)
                if existing != nil { continue } // already firing; idempotent
                if err := w.fireDeviceOffline(ctx, rule, c); err != nil { state.RecordErr(err); continue }
                state.FiresEmitted++
            }
            state.RulesEvaluated++
        }

        // 3. Emit one offline_gateway alert per downed gateway
        for gwID, suppressed := range suppressedByGateway {
            for _, rule := range gatewayRules {
                if !scopeMatchesGateway(rule, gwID) { continue }
                if rule.LastFiredAt != nil && time.Since(*rule.LastFiredAt) < time.Duration(rule.CooldownSeconds)*time.Second {
                    continue
                }
                existing, _ := w.Alerts.ListFiringByRuleTarget(ctx, rule.ID, gwID)
                if existing != nil {
                    // Update suppresses_n_devices in payload? v1 keeps the first-emitted count
                    continue
                }
                if err := w.fireGatewayOffline(ctx, rule, gwID, int32(len(suppressed))); err != nil { state.RecordErr(err); continue }
                state.FiresEmitted++
            }
        }

        // 4. Hysteresis clears (D-15: now()-last_uplink < 2× expected)
        clears, err := w.Eng.Queries.ListHysteresisClearOffline(ctx)
        if err != nil { return err }
        for _, c := range clears {
            if err := w.Alerts.ClearAlert(ctx, c.AlertID); err != nil { state.RecordErr(err); continue }
            state.Cleared++
        }

        return state.FirstErr
    }

    func (w *OfflineWorker) fireDeviceOffline(ctx context.Context, rule RuleRecord, c offlineCandidate) error {
        tx, _ := w.Eng.Pool.BeginTx(ctx, pgx.TxOptions{})
        defer tx.Rollback(ctx)
        seconds := int32(time.Since(c.LastUplinkAt).Seconds())
        payload, _ := BuildPayload(BuildPayloadInput{
            RuleID: rule.ID, RuleKind: "offline_device", Severity: rule.Severity,
            Target: PayloadTarget{EntityType: "device", EntityID: c.DeviceID, Label: c.DeviceLabel},
            Value: float64(seconds), Threshold: float64(3 * c.ExpectedIntervalS), Comparison: "gt",
            Unit: "seconds_since_last_uplink",
            FiredAt: time.Now().UTC(),
            InstallName: w.Eng.InstallDisplayName(),
            LastUplinkAt: &c.LastUplinkAt, ExpectedIntervalS: &c.ExpectedIntervalS,
        })
        alertRow, err := w.Alerts.InsertAlert(ctx, tx, AlertInsertParams{
            RuleID: rule.ID, RuleKind: "offline_device", Severity: rule.Severity,
            Payload: payload, TargetEntityType: "device", TargetEntityID: c.DeviceID,
        })
        if err != nil { return err }
        if err := audit.WriteEntry(ctx, tx, audit.Entry{
            Action: audit.ActionAlertFired, EntityType: audit.EntityTypeAlert,
            EntityID: alertRow.ID, After: payloadToMap(payload),
        }); err != nil { return err }
        if err := w.Rules.TouchLastFiredAt(ctx, tx, rule.ID); err != nil { return err }
        if err := tx.Commit(ctx); err != nil { return err }
        w.Eng.Hub.Publish(events.AlertTopic, alertRow)
        return nil
    }

    // fireGatewayOffline mirrors fireDeviceOffline but with target_entity_type="gateway"
    // and SuppressesNDevices populated in the payload.
    ```

    In `internal/cli/serve.go` add the offline worker registration + periodic job (2 min cadence per RESEARCH §Decision C):
    ```go
    river.AddWorker(riverWorkers, &alert.OfflineWorker{Eng: alertEng, Rules: ruleStore, Alerts: alertStore, WorkerStat: workerStat})
    periodicJobs = append(periodicJobs,
        river.NewPeriodicJob(river.PeriodicInterval(2*time.Minute), func() (river.JobArgs, *river.InsertOpts) { return alert.OfflineArgs{}, nil }, nil),
    )
    ```
  </action>
  <verify>
    <automated>go test ./internal/alert/... -run "TestOffline" -count=1 -timeout=120s</automated>
  </verify>
  <acceptance_criteria>
    - `internal/alert/offline_worker.go` contains `type OfflineWorker struct` and `func (w *OfflineWorker) Work(`
    - `internal/alert/offline_worker.go` references `ListOfflineDevicesWithGatewayStatus` and `ListHysteresisClearOffline`
    - The gateway-suppression code path produces `target_entity_type="gateway"` AND payload field `suppresses_n_devices`: grep both strings in offline_worker.go
    - `internal/cli/serve.go` adds `&alert.OfflineWorker{` and `river.PeriodicInterval(2*time.Minute)` and `alert.OfflineArgs{}`
    - All 6 listed tests exist in offline_worker_test.go (grep each test name)
    - `go test ./internal/alert/... -run TestOffline -count=1` passes
    - Test `TestOffline_GatewaySuppressesDeviceAlerts` asserts `count(*) FROM alert WHERE rule_kind='offline_device'` is 0 AND `count(*) FROM alert WHERE rule_kind='offline_gateway'` is 1 AND `payload->>'suppresses_n_devices' = '12'`
  </acceptance_criteria>
  <done>Gateway-down outage produces ONE alert (the gateway), not N (the devices). Hysteresis prevents flap. Audit-in-tx on every fire and clear.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| River worker → DB | Workers write `alert` + `audit_log` rows; all writes go through `pgx.Tx` and a Commit |
| Worker → Hub | events.AlertTopic publish is best-effort; failure must not roll back the DB tx |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-06-02-01 | Tampering | duplicate firing alert per rule+target | mitigate | Partial unique index `alert_firing_unique_idx ON alert (rule_id, target_entity_id) WHERE state = 'firing'` (Plan 06-01 migration 0039). `InsertAlert` returns ErrDuplicateFire which workers treat as no-op (idempotent). Test: TestThresholdInstantaneous_IdempotentFire. |
| T-06-02-02 | DoS | flap spam from rule near threshold boundary | mitigate | Cool-down (D-05) default 900s enforced in worker before evaluating. Offline hysteresis (D-15): fire at 3× expected_interval, clear only at < 2×, prevents flap. |
| T-06-02-03 | Spoofing | rule scope_id pointing at deleted entity | mitigate | `expandScope` returns empty list for missing entity → no fire, no error (worker skips). Soft-delete (`disabled_at`) MPs are filtered out by `WHERE disabled_at IS NULL` in scope-expansion queries. |
| T-06-02-04 | DoS | 200 false device-offline alerts during gateway outage | mitigate | D-14 gateway-down suppression: single query returns `gateway_offline` flag; suppressed devices add to a map; one gateway-offline alert emitted with `suppresses_n_devices` count in payload. Test: TestOffline_GatewaySuppressesDeviceAlerts asserts 12-device fleet behind a downed GW produces exactly 1 alert. |
| T-06-02-05 | Repudiation | alert fired but no audit trail | mitigate | Every `fireDeviceOffline` / `fireThresholdAlert` / `fireGatewayOffline` writes `audit.WriteEntry` in the SAME pgx.Tx as the alert insert; tx commit is atomic. Test: TestThresholdAuditTransactionAtomicity forces error mid-tx and asserts no orphan alert exists. |
| T-06-02-06 | Information Disclosure | sensitive value in payload.value | accept | Alert payload contains the breached value (e.g., flow rate, kW reading); this is the operator's data, viewable on the dashboard. AUTH-06 RBAC gates the alert center; no new disclosure surface. |
| T-06-02-07 | Tampering | River worker leader drift on multi-replica deploys | accept | Phase 6 ships single-instance Shifter (PROJECT.md single-tenant per install). River leader-election guarantees only one process runs periodic jobs even if v2 adds replicas. Documented for awareness. |
</threat_model>

<verification>
- 3 threshold workers + 1 offline worker registered in `cli/serve.go`
- 4 periodic jobs in River with cadences 1min / 15min / 1h / 2min
- All alert fires write audit-in-tx; all auto-clears write audit-in-tx
- Gateway-down outage produces one alert per downed gateway (not per device behind it)
- Cool-down (default 900s) suppresses re-fire
- Hysteresis prevents flap when device hovers near the offline threshold
- `go test ./internal/alert/... -count=1 -timeout=120s` passes
</verification>

<success_criteria>
- ALERT-01 covered: threshold rules fire on breach, auto-clear on resolution, three subtypes per CAGG level
- ALERT-02 covered: offline rules respect 3×/2× hysteresis from per-profile expected_interval_s
- ALERT-03 covered: gateway-down suppression emits one alert, not many
- D-12 canonical payload shape written for every fire
- D-05 cool-down enforced in-engine
- D-22 max-3-retries on every worker via InsertOpts
</success_criteria>

<output>
After completion, create `.planning/phases/06-alerts-users-audit-operational-hardening/06-02-SUMMARY.md`
</output>
