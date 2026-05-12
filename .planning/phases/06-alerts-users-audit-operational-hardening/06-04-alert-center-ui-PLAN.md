---
phase: 06-alerts-users-audit-operational-hardening
plan: 04
type: execute
wave: 2
depends_on: [06-01]
files_modified:
  - internal/alert/handler.go
  - internal/alert/handler_test.go
  - internal/alert/rule_handler.go
  - internal/alert/rule_handler_test.go
  - internal/alert/test_fire.go
  - internal/alert/test_fire_test.go
  - internal/alert/queries.sql
  - internal/auth/authz.go
  - internal/auth/authz_test.go
  - internal/http/router.go
  - internal/events/topics.go
  - internal/events/handler.go
  - internal/cli/serve.go
  - web/src/routes/alerts/index.tsx
  - web/src/routes/alerts/index.test.tsx
  - web/src/routes/alerts/AlertCenterFilters.tsx
  - web/src/routes/alerts/AlertRow.tsx
  - web/src/routes/alerts/SnoozeMenu.tsx
  - web/src/routes/alerts/AlertDetailDialog.tsx
  - web/src/routes/settings/alerts.tsx
  - web/src/routes/settings/alerts.test.tsx
  - web/src/routes/settings/AddRuleDialog.tsx
  - web/src/routes/settings/AnomalyRosterSection.tsx
  - web/src/components/metering-point/AnomalyStateCard.tsx
  - web/src/components/metering-point/AnomalyStateCard.test.tsx
  - web/src/components/shell/AlertBell.tsx
  - web/src/components/shell/AlertDrawer.tsx
  - web/src/components/shell/AlertWorkerBanner.tsx
  - web/src/components/shell/topbar.tsx
  - web/src/components/shell/sidebar.tsx
  - web/src/components/alerts/SeverityPill.tsx
  - web/src/hooks/useAlerts.ts
  - web/src/lib/alertParams.ts
  - web/src/App.tsx
  - web/playwright/specs/alerts-center.spec.ts
autonomous: true
requirements: [ALERT-05, ALERT-06]
must_haves:
  truths:
    - "Header bell with severity-tinted unread badge appears on every authenticated route except /login and the install wizard"
    - "Clicking bell opens slide-over drawer with last 10 alerts (severity DESC, fired_at DESC); ack + snooze inline; viewer sees read-only"
    - "/alerts page lists all alerts with URL-state filter chips (severity, status, category, target_type, date range)"
    - "Three entry points for rule creation — MP detail + Site detail + Settings → Alerts — all open the same Add Rule ResponsiveDialog; scope step skipped when prefilled (D-18)"
    - "/settings/alerts shows: anomaly roster section (eligible/warming up counts + expandable list) + rule library table with row toggles, bulk select, Add Rule button"
    - "MP detail page renders AnomalyStateCard (warming_up | eligible_inactive | active) ABOVE the existing Phase 4 tabs (D-16)"
    - "Test fire button creates synthetic info-severity alert with is_test=true and auto-clears in 60s (D-19); does NOT touch alert_rule.last_fired_at"
    - "Sidebar reordered to Dashboard → Reports → Map → Gateways → Sites → Devices → Profiles → Alerts → Audit → Settings (audit admin-only)"
    - "Shell degraded banner renders above topbar when /health/detailed returns alert_worker.degraded=true; auto-dismisses on recovery (D-22)"
  artifacts:
    - path: internal/alert/handler.go
      provides: "/api/alerts list (cursor), GET /alerts/{id}, POST /alerts/{id}/ack, POST /alerts/{id}/snooze, POST /alerts/{id}/mute"
      exports: ["ListHandler","AckHandler","SnoozeHandler","MuteHandler","GetHandler"]
    - path: internal/alert/rule_handler.go
      provides: "/api/alerts/rules CRUD"
      exports: ["ListRulesHandler","CreateRuleHandler","UpdateRuleHandler","DisableRuleHandler","EnableRuleHandler","GetRuleHandler","RosterHandler","MPAnomalyStateHandler","ToggleMPAnomalyHandler"]
    - path: internal/alert/test_fire.go
      provides: "TestFireHandler — creates synthetic is_test=true alert per D-19"
      exports: ["TestFireHandler"]
    - path: web/src/components/shell/AlertBell.tsx
      provides: "Topbar bell with severity-tinted unread badge"
    - path: web/src/components/shell/AlertDrawer.tsx
      provides: "Slide-over drawer (Sheet side='right')"
    - path: web/src/routes/alerts/index.tsx
      provides: "/alerts page with URL-state filter chips + virtualized list"
    - path: web/src/routes/settings/alerts.tsx
      provides: "Rule library + anomaly roster"
    - path: web/src/components/metering-point/AnomalyStateCard.tsx
      provides: "MP detail three-state anomaly card"
  key_links:
    - from: web/src/routes/alerts/index.tsx
      to: /api/alerts
      via: "React Query + useSearchParams + zod (Phase 3 D-15 pattern)"
      pattern: "useSearchParams"
    - from: web/src/components/shell/AlertBell.tsx
      to: /api/alerts?status=open&limit=10
      via: "React Query poll every 30s + optional events.alert SSE topic"
      pattern: "useAlerts"
    - from: web/src/components/metering-point/AnomalyStateCard.tsx
      to: /api/metering-points/{id}/anomaly-state
      via: "GET returns eligibility + per-rule toggles; PATCH toggles enable"
      pattern: "anomaly-state"
---

<objective>
Ship the entire alert center user experience: header bell + slide-over drawer + dedicated `/alerts` page + Settings → Alerts rule library + MP detail anomaly state card + shell degraded banner. Wires the three D-18 entry points (MP detail, Site detail, Settings → Alerts) to the same Add Rule dialog. Closes ALERT-05 and ALERT-06 frontend + ALERT-06's backend handlers (list/ack/snooze for the in-app center; CSV export is separate in Plan 06-07).

Purpose: this is the operator's primary feedback surface for the alert engine. Every Phase 6 surface beneath this (rule kinds, user mgmt, audit) ultimately becomes visible in the alert center.

Output: backend handlers + frontend routes + components + 1 Playwright E2E spec covering bell→drawer→ack→snooze flow.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/06-alerts-users-audit-operational-hardening/06-CONTEXT.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-UI-SPEC.md
@.planning/phases/06-alerts-users-audit-operational-hardening/06-01-alert-engine-substrate-PLAN.md
@internal/alert/engine.go
@internal/alert/rule_store.go
@internal/alert/alert_store.go
@internal/alert/cold_start.go
@internal/audit/log.go
@internal/auth/authz.go
@internal/http/router.go
@internal/events/handler.go
@internal/events/topics.go
@web/src/components/shell/sidebar.tsx
@web/src/components/shell/topbar.tsx
@web/src/components/responsive-dialog.tsx
@web/src/components/stepper.tsx
@web/src/components/metering-point/JsonTree.tsx
@web/src/components/dashboard/EmptyStateOnboarding.tsx
@web/src/components/dashboard/DateRangePicker.tsx
@web/src/App.tsx

<interfaces>
Plan 06-01 substrate types (RuleStore, AlertStore, RuleRecord, AlertRecord — see Plan 06-01 interfaces block).
Plan 06-03 cold-start helpers: IsMPEligibleForAnomaly, ListAnomalyWarmupRoster, GetMPAnomalyState.

internal/auth/authz.go existing Action type pattern (line 32). New actions to add:
- ActionAlertRead       Action = "alert.read"
- ActionAlertAck        Action = "alert.ack"
- ActionAlertSnooze     Action = "alert.snooze"
- ActionAlertMute       Action = "alert.mute"
- ActionAlertRuleCreate Action = "alert.rule_create"
- ActionAlertRuleUpdate Action = "alert.rule_update"
- ActionAlertRuleDisable Action = "alert.rule_disable"
- ActionAlertRuleEnable Action = "alert.rule_enable"
- ActionAlertTestFire   Action = "alert.test_fire"

roleBundles entries:
- RoleAdmin: all 9 above true
- RoleViewer: ActionAlertRead=true only (read-only per D-11)

internal/events/topics.go: existing per-MP and per-device-health topics; Plan 06-04 adds AlertTopic = events.Topic("alert").

internal/http/router.go: chi router with auth.RequireAction wrappers; new route groups:
- /api/alerts/* — GET list (admin+viewer), GET/POST individual (admin only for ack/snooze/mute)
- /api/alerts/rules/* — admin only
- /api/metering-points/{id}/anomaly-state — GET (admin+viewer), PATCH (admin)
- /api/anomaly-roster — GET (admin+viewer)

UI shadcn components: Sheet (drawer), Popover (filter chips), DropdownMenu (snooze + row overflow), AlertDialog (destructive confirms; already added in Phase 1).

web/src/components/responsive-dialog.tsx — existing pattern, use for Add Rule + Alert Detail.
web/src/components/stepper.tsx — existing 5-step rule builder.
web/src/components/dashboard/EmptyStateOnboarding.tsx — three-stage empty-state card.
web/src/components/dashboard/DateRangePicker.tsx — URL-state date range.
web/src/components/metering-point/JsonTree.tsx — payload rendering in detail dialog.

zod URL-state schema for /alerts (06-UI-SPEC §Route Architecture):
```ts
export const alertsParams = z.object({
  severity: z.enum(["critical","warning","info","all"]).catch("all"),
  status: z.enum(["open","acknowledged","snoozed","cleared","all"]).catch("open"),
  category: z.enum(["threshold","offline","anomaly","all"]).catch("all"),
  target_type: z.enum(["metering_point","site","gateway","all"]).catch("all"),
  from: z.string().datetime().optional(),
  to: z.string().datetime().optional(),
});
```
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Backend handlers for /api/alerts + /api/alerts/rules + /api/anomaly-roster + test_fire</name>
  <files>internal/alert/handler.go, internal/alert/handler_test.go, internal/alert/rule_handler.go, internal/alert/rule_handler_test.go, internal/alert/test_fire.go, internal/alert/test_fire_test.go, internal/alert/queries.sql, internal/auth/authz.go, internal/auth/authz_test.go, internal/http/router.go, internal/events/topics.go, internal/events/handler.go, internal/cli/serve.go</files>
  <read_first>
    - internal/auth/authz.go (Action constants pattern + roleBundles map structure; lines 30-280)
    - internal/http/router.go (existing route group + RequireAction wrapper pattern)
    - internal/events/handler.go (SSE handler — for adding AlertTopic subscribe support, optional drawer auto-refresh)
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-RESEARCH.md §Code Examples "Alert Rule Creation (handler + SQL)"
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-UI-SPEC.md §Surface 1, 2, 3, 4 (acceptance lists)
    - internal/alert/alert_store.go (Plan 06-01: InsertAlert, AckAlert, SnoozeAlert, MuteAlert, ClearAlert, ListRecent)
    - internal/alert/rule_store.go (Plan 06-01: CreateRule, ListAllRules, UpdateRule, DisableRule, EnableRule)
  </read_first>
  <behavior>
    - Test (TestAuthz_AlertActionsAdminOnlyExceptRead): RoleAdmin has all 9 new actions=true; RoleViewer has only ActionAlertRead=true; Can(viewer, ActionAlertAck) returns false.
    - Test (TestAlertList_OpenStatusDefault): GET /api/alerts → returns rows with state IN ('firing','acknowledged','snoozed','muted') sorted by severity DESC then fired_at DESC; cursor pagination with next_cursor when 100 rows returned.
    - Test (TestAlertList_FilterChips): GET /api/alerts?severity=critical&category=threshold returns only matching rows.
    - Test (TestAlertAck_WritesAuditInTx): POST /api/alerts/{id}/ack with body {"note":"checked floor 3"} returns 200; alert row state='acknowledged', acked_at set, acked_by=current user, ack_note='checked floor 3'; audit_log has one row action='alert.acknowledged' with user_id=current user.
    - Test (TestAlertSnooze_AcceptsPresets): POST /api/alerts/{id}/snooze body {"duration":"8h"} → alert.snoozed_until = now()+8h, state='snoozed'. body {"duration":"mute"} → alert.muted=true, state='muted'.
    - Test (TestAlertSnooze_RejectsInvalidDuration): body {"duration":"99y"} → 422.
    - Test (TestRuleCreate_WritesAuditInTx): POST /api/alerts/rules with valid body → 201 + audit row 'alert.rule_create' in same tx.
    - Test (TestRuleCreate_RejectsScopeIDMismatch): scope_kind='global' with scope_id set → 422; scope_kind='metering_point' with scope_id=null → 422.
    - Test (TestRuleUpdate_Diff): PATCH writes 'alert.rule_update' with Before/After diff showing changed fields.
    - Test (TestRuleDisable_AuditsAndPersists): DELETE /api/alerts/rules/{id} (semantic disable) → audit row 'alert.rule_disable'; disabled_at set; rule excluded from worker's ListActiveRulesByKind on next eval.
    - Test (TestAnomalyRoster_AdminAndViewerBothAllowed): GET /api/anomaly-roster returns rows sorted by days_until_eligible ASC for both roles.
    - Test (TestMPAnomalyState_PatchToggle): PATCH /api/metering-points/{id}/anomaly-rules/p95 with {"enabled":true} → creates anomaly_p95 rule scoped to this MP (if none) OR enables existing (sets disabled_at=NULL); writes 'alert.rule_create' or 'alert.rule_enable' audit row.
    - Test (TestTestFire_CreatesSyntheticAlert): POST /api/alerts/rules/{id}/test-fire → returns 200; alert row exists with is_test=true, severity='info', payload.note contains 'TEST'; alert auto-clears after 60s wall-clock (test uses time-warp helper).
    - Test (TestTestFire_DoesNotTouchLastFiredAt): rule.last_fired_at unchanged after test-fire (Pitfall 8 mitigation).
    - Test (TestEventsAlertTopic_Subscribe): SSE /api/events with `?topic=alert` accepted; Hub.Publish(AlertTopic, alertRow) delivers a "data: {...}\n\n" event to the subscriber.
  </behavior>
  <action>
    **internal/auth/authz.go:** Add 9 new const ActionAlert* and 9 new entries in `roleBundles[RoleAdmin]`. Add `ActionAlertRead: true` to `roleBundles[RoleViewer]`. The viewer entries reflect D-11 (viewers see alerts read-only).

    Replace the existing `ActionAuditView` const at line 71 with a fully-aliased name reservation: keep it as an alias to ActionAuditRead (already declared at line 116; do NOT redeclare). The newer plans use `ActionAuditRead`.

    **internal/alert/queries.sql** (append):
    ```sql
    -- name: ListAlertsCursor :many
    -- D-32-style cursor (time DESC, id DESC) over alert table with filters.
    -- $1=cursor_time, $2=cursor_id, $3..=filters
    SELECT a.id, a.rule_id, a.rule_kind, a.severity, a.state, a.payload,
           a.target_entity_type, a.target_entity_id, a.is_test,
           a.fired_at, a.cleared_at, a.acked_at, a.acked_by, a.ack_note,
           a.snoozed_until, a.snoozed_by, a.muted
    FROM alert a
    WHERE (
        $1::TIMESTAMPTZ IS NULL
        OR (a.fired_at, a.id) < ($1::TIMESTAMPTZ, $2::UUID)
    )
    AND ($3::TEXT IS NULL OR a.severity = $3)
    AND ($4::TEXT IS NULL OR a.state = $4)
    AND ($5::TEXT IS NULL OR a.rule_kind LIKE $5 || '%') -- category prefix: 'threshold' | 'offline' | 'anomaly'
    AND ($6::TEXT IS NULL OR a.target_entity_type = $6)
    AND ($7::TIMESTAMPTZ IS NULL OR a.fired_at >= $7)
    AND ($8::TIMESTAMPTZ IS NULL OR a.fired_at <  $8)
    ORDER BY a.fired_at DESC, a.id DESC
    LIMIT 100;

    -- name: CountAlertsUnreadBySeverity :one
    -- For bell badge: highest-severity unread count.
    SELECT
        count(*) FILTER (WHERE severity = 'critical' AND state IN ('firing','snoozed')) AS critical_count,
        count(*) FILTER (WHERE severity = 'warning'  AND state IN ('firing','snoozed')) AS warning_count,
        count(*) FILTER (WHERE severity = 'info'     AND state IN ('firing','snoozed')) AS info_count
    FROM alert
    WHERE muted = FALSE;

    -- name: ListRecentAlertsForDrawer :many
    -- Top 10 most-recent firing/snoozed alerts, sorted severity DESC then fired_at DESC.
    SELECT a.id, a.rule_id, a.rule_kind, a.severity, a.state, a.payload, a.fired_at,
           a.target_entity_type, a.target_entity_id, a.is_test
    FROM alert a
    WHERE a.state IN ('firing','acknowledged','snoozed')
      AND a.muted = FALSE
    ORDER BY
        CASE a.severity WHEN 'critical' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END ASC,
        a.fired_at DESC
    LIMIT 10;
    ```

    Run `sqlc generate`.

    **internal/alert/handler.go:**
    - `ListHandler` — GET /api/alerts; parse query filters via zod-mirroring Go struct (severity/status/category/target_type/from/to/cursor); call ListAlertsCursor; return `{rows, next_cursor, unread_counts}` where unread_counts comes from CountAlertsUnreadBySeverity (eliminates a second round-trip for the badge).
    - `GetHandler` — GET /api/alerts/{id}; returns single alert + payload.
    - `AckHandler` — POST /api/alerts/{id}/ack; body `{note?: string}`; calls `AlertStore.AckAlert(ctx, tx, id, userID, note)` + writes audit row `alert.acknowledged` in same tx; commits; returns 200.
    - `SnoozeHandler` — POST /api/alerts/{id}/snooze; body `{duration: "1h"|"8h"|"24h"|"7d"|"mute"}`; if "mute" → MuteAlert; else compute until time and SnoozeAlert; audit row `alert.snoozed` or `alert.muted`.
    - `RecentForDrawerHandler` — GET /api/alerts/recent → top 10 + unread_counts (powers the bell + drawer).

    **internal/alert/rule_handler.go:**
    - `ListRulesHandler` — GET /api/alerts/rules?show_disabled=0|1; calls RuleStore.ListAllRules.
    - `CreateRuleHandler` — POST; validates scope_kind/scope_id pairing; copies RESEARCH §Code Examples handler pattern; writes audit row `alert.rule_create`.
    - `UpdateRuleHandler` — PATCH /api/alerts/rules/{id}; loads existing row for Before, applies changes, writes 'alert.rule_update' with diff.
    - `DisableRuleHandler` — POST /api/alerts/rules/{id}/disable; audit row `alert.rule_disable`.
    - `EnableRuleHandler` — POST /api/alerts/rules/{id}/enable; audit row `alert.rule_enable`.
    - `RosterHandler` — GET /api/anomaly-roster → calls cold_start.ListAnomalyWarmupRoster.
    - `MPAnomalyStateHandler` — GET /api/metering-points/{id}/anomaly-state → returns `{eligible: bool, days_until_eligible: int, rules: [{rule_kind, rule_id?, enabled}]}` per UI-SPEC Surface 4.
    - `ToggleMPAnomalyHandler` — PATCH /api/metering-points/{id}/anomaly-rules/{kind} body `{enabled: bool}`; creates rule (scope_kind='metering_point', scope_id=mpID, rule_kind=kind) if missing, or flips disabled_at; one audit row per call.

    **internal/alert/test_fire.go:**
    ```go
    // TestFireHandler is the D-19 UI smoke test. Creates a synthetic alert
    // (is_test=true, severity='info'); writes 'alert.test_fired' audit row;
    // schedules a River one-shot job 60s later that ClearAlerts the row.
    // Does NOT call rules.TouchLastFiredAt (Pitfall 8: test fires must not
    // suppress real fires by spuriously setting last_fired_at).
    func TestFireHandler(deps Deps) http.HandlerFunc { ... }
    ```
    The 60s auto-clear uses a one-shot River job (kind="alert_test_fire_clear", InsertOpts.ScheduledAt = now()+60s); register a `TestFireClearWorker` that calls `AlertStore.ClearAlert(ctx, tx, alertID)` + writes 'alert.cleared' audit.

    **internal/cli/serve.go:** Register the worker in the existing `river.NewWorkers()` block (~lines 320-360) — add this line alongside the other `river.AddWorker` calls (e.g., immediately after the alert.AuditPruneWorker registration from Plan 06-01):
    ```go
    river.AddWorker(workers, &alert.TestFireClearWorker{Store: alertStore, Audit: auditStore, Log: log})
    ```
    No periodic schedule needed (one-shot job scheduled by `TestFireHandler` via `InsertOpts.ScheduledAt`). The worker definition lives in `internal/alert/test_fire.go` (declared alongside the handler):
    ```go
    type TestFireClearArgs struct {
        AlertID uuid.UUID `json:"alert_id"`
    }
    func (TestFireClearArgs) Kind() string { return "alert_test_fire_clear" }
    func (TestFireClearArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{MaxAttempts: 3} }

    type TestFireClearWorker struct {
        river.WorkerDefaults[TestFireClearArgs]
        Store *AlertStore
        Audit *audit.Store
        Log   *slog.Logger
    }

    func (w *TestFireClearWorker) Work(ctx context.Context, job *river.Job[TestFireClearArgs]) error {
        tx, err := w.Store.Pool().BeginTx(ctx, pgx.TxOptions{})
        if err != nil { return err }
        defer tx.Rollback(ctx)
        if err := w.Store.ClearAlert(ctx, tx, job.Args.AlertID); err != nil { return err }
        if err := audit.WriteEntry(ctx, tx, audit.Entry{
            Action: audit.ActionAlertCleared, EntityType: audit.EntityTypeAlert,
            EntityID: job.Args.AlertID,
            Notes: "auto-cleared after 60s test-fire window (D-19)",
        }); err != nil { return err }
        return tx.Commit(ctx)
    }
    ```

    **internal/events/topics.go:** Add `var AlertTopic = Topic("alert")`. Update `internal/events/handler.go` SSE handler to accept `?topic=alert` and subscribe to AlertTopic; publish payload as JSON for drawer auto-refresh. This is optional optimization per D-20 — the drawer can also use React Query polling (30s interval).

    **internal/http/router.go:** Add route group:
    ```go
    r.Route("/api/alerts", func(r chi.Router) {
        r.With(auth.RequireAction(sm, auth.ActionAlertRead)).Get("/", alert.ListHandler(deps))
        r.With(auth.RequireAction(sm, auth.ActionAlertRead)).Get("/recent", alert.RecentForDrawerHandler(deps))
        r.With(auth.RequireAction(sm, auth.ActionAlertRead)).Get("/{id}", alert.GetHandler(deps))
        r.With(auth.RequireAction(sm, auth.ActionAlertAck)).Post("/{id}/ack", alert.AckHandler(deps))
        r.With(auth.RequireAction(sm, auth.ActionAlertSnooze)).Post("/{id}/snooze", alert.SnoozeHandler(deps))
        r.Route("/rules", func(r chi.Router) {
            r.With(auth.RequireAction(sm, auth.ActionAlertRead)).Get("/", alert.ListRulesHandler(deps))
            r.With(auth.RequireAction(sm, auth.ActionAlertRuleCreate)).Post("/", alert.CreateRuleHandler(deps))
            r.With(auth.RequireAction(sm, auth.ActionAlertRuleUpdate)).Patch("/{id}", alert.UpdateRuleHandler(deps))
            r.With(auth.RequireAction(sm, auth.ActionAlertRuleDisable)).Post("/{id}/disable", alert.DisableRuleHandler(deps))
            r.With(auth.RequireAction(sm, auth.ActionAlertRuleEnable)).Post("/{id}/enable", alert.EnableRuleHandler(deps))
            r.With(auth.RequireAction(sm, auth.ActionAlertTestFire)).Post("/{id}/test-fire", alert.TestFireHandler(deps))
        })
    })
    r.With(auth.RequireAction(sm, auth.ActionAlertRead)).Get("/api/anomaly-roster", alert.RosterHandler(deps))
    r.With(auth.RequireAction(sm, auth.ActionAlertRead)).Get("/api/metering-points/{id}/anomaly-state", alert.MPAnomalyStateHandler(deps))
    r.With(auth.RequireAction(sm, auth.ActionAlertRuleCreate)).Patch("/api/metering-points/{id}/anomaly-rules/{kind}", alert.ToggleMPAnomalyHandler(deps))
    ```
  </action>
  <verify>
    <automated>go test ./internal/alert/... ./internal/auth/... -run "TestAuthz_AlertActions|TestAlert|TestRule|TestAnomalyRoster|TestMPAnomalyState|TestTestFire|TestEventsAlertTopic" -count=1 -timeout=120s</automated>
  </verify>
  <acceptance_criteria>
    - `internal/auth/authz.go` has all 9 new const declarations: `grep -c "ActionAlertRead\|ActionAlertAck\|ActionAlertSnooze\|ActionAlertMute\|ActionAlertRuleCreate\|ActionAlertRuleUpdate\|ActionAlertRuleDisable\|ActionAlertRuleEnable\|ActionAlertTestFire" internal/auth/authz.go` returns 9
    - `roleBundles[RoleViewer]` map contains `ActionAlertRead: true` (grep for this exact entry)
    - All 9 actions present in `roleBundles[RoleAdmin]`
    - `internal/alert/handler.go` contains all 5 handlers: `ListHandler`, `GetHandler`, `AckHandler`, `SnoozeHandler`, `RecentForDrawerHandler`
    - `internal/alert/rule_handler.go` contains: `ListRulesHandler`, `CreateRuleHandler`, `UpdateRuleHandler`, `DisableRuleHandler`, `EnableRuleHandler`, `RosterHandler`, `MPAnomalyStateHandler`, `ToggleMPAnomalyHandler`
    - `internal/alert/test_fire.go` contains `TestFireHandler` and the comment `Does NOT call rules.TouchLastFiredAt`
    - `internal/alert/test_fire.go` contains `type TestFireClearWorker struct` with `Work(ctx context.Context, job *river.Job[TestFireClearArgs])` method
    - `internal/cli/serve.go` registers the auto-clear worker: `grep "TestFireClearWorker" internal/cli/serve.go` returns ≥ 1 line
    - `internal/events/topics.go` contains `AlertTopic = Topic("alert")`
    - `internal/http/router.go` contains 14 routes mapped (count of `auth.RequireAction(sm, auth.ActionAlert` matches >= 12)
    - All 13 listed tests pass: `go test ./internal/alert/... ./internal/auth/... -count=1 -timeout=120s` exits 0
    - `TestTestFire_DoesNotTouchLastFiredAt` asserts last_fired_at unchanged after test-fire (Pitfall 8)
  </acceptance_criteria>
  <done>Every backend route the alert center UI needs exists and is RBAC-gated; D-11 viewer-read-only is enforced server-side; every mutation writes audit-in-tx; test-fire pattern doesn't suppress real fires.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Alert bell + drawer + /alerts page + Settings → Alerts + MP detail anomaly card + sidebar reorder + degraded banner</name>
  <execution_split>
    This task is too large for a single Claude execution window (23 files). The executor MUST commit-split into 2a + 2b for safer execution:

    **Sub-task 2a — Shell + nav + state files (~11 files; commit when green):**
    - web/src/components/shell/AlertBell.tsx
    - web/src/components/shell/AlertDrawer.tsx
    - web/src/components/shell/AlertWorkerBanner.tsx
    - web/src/components/shell/topbar.tsx
    - web/src/components/shell/sidebar.tsx
    - web/src/components/alerts/SeverityPill.tsx
    - web/src/hooks/useAlerts.ts
    - web/src/lib/alertParams.ts
    - web/src/App.tsx (only the route imports + mount points; no /alerts route component yet — placeholder ok)
    - All `_test.tsx` files for the above components
    Verify after 2a: `pnpm -C web test --run web/src/components/shell web/src/components/alerts` exits 0.

    **Sub-task 2b — Route components + rule library + MP detail card + Playwright (~12 files; commit when green):**
    - web/src/routes/alerts/index.tsx
    - web/src/routes/alerts/AlertCenterFilters.tsx
    - web/src/routes/alerts/AlertRow.tsx
    - web/src/routes/alerts/SnoozeMenu.tsx
    - web/src/routes/alerts/AlertDetailDialog.tsx
    - web/src/routes/settings/alerts.tsx
    - web/src/routes/settings/AddRuleDialog.tsx
    - web/src/routes/settings/AnomalyRosterSection.tsx
    - web/src/components/metering-point/AnomalyStateCard.tsx + .test.tsx
    - web/playwright/specs/alerts-center.spec.ts
    - web/src/App.tsx (update — wire the actual /alerts and /settings/alerts route elements)
    - All `_test.tsx` files for the above components
    Verify after 2b: the FULL automated command below.

    Both sub-tasks share the `<read_first>`, `<behavior>`, `<action>`, `<verify>`, `<acceptance_criteria>`, and `<done>` sections — they describe ONE feature delivered in two commits.
  </execution_split>
  <files>web/src/components/shell/AlertBell.tsx, web/src/components/shell/AlertDrawer.tsx, web/src/components/shell/AlertWorkerBanner.tsx, web/src/components/shell/topbar.tsx, web/src/components/shell/sidebar.tsx, web/src/components/alerts/SeverityPill.tsx, web/src/components/metering-point/AnomalyStateCard.tsx, web/src/components/metering-point/AnomalyStateCard.test.tsx, web/src/routes/alerts/index.tsx, web/src/routes/alerts/index.test.tsx, web/src/routes/alerts/AlertCenterFilters.tsx, web/src/routes/alerts/AlertRow.tsx, web/src/routes/alerts/SnoozeMenu.tsx, web/src/routes/alerts/AlertDetailDialog.tsx, web/src/routes/settings/alerts.tsx, web/src/routes/settings/alerts.test.tsx, web/src/routes/settings/AddRuleDialog.tsx, web/src/routes/settings/AnomalyRosterSection.tsx, web/src/hooks/useAlerts.ts, web/src/lib/alertParams.ts, web/src/App.tsx</files>
  <read_first>
    - .planning/phases/06-alerts-users-audit-operational-hardening/06-UI-SPEC.md (all 8 surfaces; copy CTA labels and chip orderings verbatim)
    - web/src/components/responsive-dialog.tsx (use for AddRuleDialog + AlertDetailDialog)
    - web/src/components/stepper.tsx (use for 5-step AddRuleDialog)
    - web/src/components/metering-point/JsonTree.tsx (use in AlertDetailDialog for payload preview)
    - web/src/components/dashboard/EmptyStateOnboarding.tsx (use for all empty states)
    - web/src/components/dashboard/DateRangePicker.tsx (use for /alerts date range chip)
    - web/src/components/shell/sidebar.tsx (current sidebar — reorder per UI-SPEC §Surface 8)
    - web/src/lib/use-current-user.ts (for role-conditional rendering)
    - web/src/App.tsx (route table — add /alerts, /settings/alerts)
  </read_first>
  <behavior>
    - Test (AlertBell_BadgeReflectsSeverity): mock /api/alerts/recent returns critical_count=2; bell shows red dot. critical=0, warning=1 → yellow. all 0 → no badge.
    - Test (AlertDrawer_RendersMax10Rows): drawer fetches /api/alerts/recent and renders ≤10 rows sorted by severity then fired_at.
    - Test (AlertDrawer_AckOptimistic): clicking Ack on a row immediately dims it (optimistic); POST /api/alerts/{id}/ack is fired; on success row removed from Unread chip filter.
    - Test (AlertDrawer_ViewerHidesAckSnooze): currentUser.role='viewer' → row action buttons hidden; "See all" link visible.
    - Test (AlertsPage_URLStateFilters): navigating to /alerts?severity=critical&status=open populates filter chips; clicking a chip updates URL via setSearchParams.
    - Test (AlertsPage_ExportRespectsFilters): export button calls /api/alerts/export?severity=critical&... (this is a stub in this plan; full CSV export ships in Plan 06-07's audit-export pattern).
    - Test (SettingsAlerts_RuleLibraryTable): renders rules from /api/alerts/rules; row toggle calls POST /disable or /enable; Show disabled toggle reads ?show_disabled URL param.
    - Test (AddRuleDialog_FiveStepFlow): test instantiates with no prefilled scope → 5 steps render in stepper order; instantiates with `meteringPointId` prefilled → step 1 (Scope) skipped, stepper starts at step 2 (Kind).
    - Test (AddRuleDialog_TestFireButton): step 5 review has "Test fire" button; clicking calls POST /api/alerts/rules/{id}/test-fire; success toast.
    - Test (AnomalyStateCard_WarmingUp): mock /api/metering-points/{id}/anomaly-state returns `{eligible:false, days_until_eligible:16}` → card renders warming_up state with Progress bar at 5/21.
    - Test (AnomalyStateCard_EligibleInactive): `{eligible:true, rules:[all disabled]}` → ready-to-enable state with toggles.
    - Test (AnomalyStateCard_Active): `{eligible:true, rules:[p95 enabled, quiet_hour enabled]}` → active state with success border.
    - Test (AnomalyStateCard_ViewerReadOnly): viewer role disables toggles with tooltip "Read-only — viewer role".
    - Test (Sidebar_OrderAndAudit): sidebar items appear in exact order: Dashboard, Reports, Map, Gateways, Sites, Devices, Profiles, Alerts, Audit, Settings (Audit link present for admin, absent for viewer).
    - Test (Sidebar_AlertsUnreadBadge): badge shows count + severity dot when /api/alerts/recent returns counts; 99+ cap; hidden when 0.
    - Test (DegradedBanner_RendersWhenDegraded): mock /health/detailed returns `alert_worker:{degraded:true,...}` → banner renders above topbar with warning styling; "View details" admin-only.
    - Test (DegradedBanner_AutoDismisses): subsequent /health/detailed `{degraded:false}` → banner unmounts on next poll.
    - Playwright (alerts-center.spec.ts): admin logs in → creates a threshold rule via Settings → Alerts (uses test fire) → bell badge increments → opens drawer → clicks Ack → alert disappears from Unread filter → navigates to /alerts page → sees acknowledged alert with filter chip status=acknowledged.
  </behavior>
  <action>
    **web/src/lib/alertParams.ts:** zod schema verbatim from UI-SPEC §Route Architecture (alertsParams). Export hook `useAlertParams()` that returns `[params, setParams]` via useSearchParams + zod parse with .catch fallbacks (Phase 3 D-15 pattern).

    **web/src/hooks/useAlerts.ts:**
    - `useAlertsList(params)` — React Query `['alerts', params]` fetching /api/alerts with pagination; returns `{rows, nextCursor, unreadCounts, hasNextPage}`.
    - `useAlertsRecent()` — `['alerts','recent']` fetching /api/alerts/recent every 30s; refetchOnWindowFocus.
    - `useAlertRulesList(showDisabled)` — `['alert-rules', showDisabled]`.
    - `useAnomalyRoster()` — `['anomaly-roster']`.
    - `useMPAnomalyState(mpId)` — `['mp-anomaly-state', mpId]`.
    - `useAckMutation()`, `useSnoozeMutation()`, `useCreateRuleMutation()`, `useToggleMPAnomalyRuleMutation()` — useMutation hooks invalidating relevant query keys.

    **web/src/components/alerts/SeverityPill.tsx:** small Badge wrapper applying the locked severity color map from UI-SPEC §Severity map. Props: `{severity: 'critical'|'warning'|'info', children?}`. Used everywhere.

    **web/src/components/shell/AlertBell.tsx:** lucide Bell icon button + absolute-positioned severity-tinted dot when `unreadCounts.critical > 0 || warning > 0 || info > 0` (color follows critical > warning > info precedence). aria-label `Alerts (${total} unread, ${critical} critical)`. Click opens AlertDrawer (Sheet controlled).

    **web/src/components/shell/AlertDrawer.tsx:** shadcn `Sheet side="right"` per UI-SPEC §Surface 1; ToggleGroup "All"/"Unread"; virtualized row list (`@tanstack/react-virtual` already in package.json); footer `Button variant="link"` "See all alerts" navigating to /alerts. Row renders AlertRow component with severity dot + 2-line layout (title + relative time) + inline Ack button + SnoozeMenu split-button. Empty/loading/error states per UI-SPEC.

    **web/src/components/shell/AlertWorkerBanner.tsx:** queries /api/health/detailed every 60s; renders top-of-shell warning Alert when `alert_worker.degraded=true`; "View details →" link admin-only (uses use-current-user). `role="alert"` for screen readers; `sticky top-0 z-50`; auto-dismisses on next poll returning degraded=false. NOTE: this banner DOES NOT require user authentication enforcement — it's read by both admin + viewer.

    **web/src/components/shell/topbar.tsx:** mount `<AlertBell />` left of the existing `<AccountMenu />`.

    **web/src/components/shell/sidebar.tsx:** replace the existing NAV array with the new order (UI-SPEC §Surface 8):
    ```tsx
    const NAV = [
      { to: '/', label: 'Dashboard', icon: LayoutDashboard },
      { to: '/reports', label: 'Reports', icon: FileText },
      { to: '/map', label: 'Map', icon: Map },
      { to: '/gateways', label: 'Gateways', icon: Radio },
      { to: '/sites', label: 'Sites', icon: MapPin },
      { to: '/devices', label: 'Devices', icon: Cpu },
      { to: '/profiles', label: 'Profiles', icon: Layers },
      { to: '/alerts', label: 'Alerts', icon: Bell, badge: 'alerts-unread' },
      { to: '/audit', label: 'Audit', icon: Shield, adminOnly: true },
      { to: '/settings', label: 'Settings', icon: SettingsIcon },
    ]
    ```
    Add badge rendering for `badge === 'alerts-unread'` using `useAlertsRecent` returns; severity-tinted dot + count text (cap "99+"). Filter `adminOnly` items by `user.role === 'admin'`. Audit nav item is `Shield` icon (lucide).

    **web/src/routes/alerts/index.tsx (`AlertsPage`):** layout per UI-SPEC §Surface 2:
    - H1 "Alerts (${total})" + right-aligned Export CSV button (stub action — opens a toast "Coming in Plan 06-07")
    - Filter chips row via AlertCenterFilters component (Popover per chip: Severity / Status / Category / Target type / Date range using DateRangePicker; "Clear all" link)
    - Virtualized list of AlertRow rows (reuse drawer's component)
    - "Load more" button when nextCursor present (D-36 pattern)
    - Empty states per UI-SPEC §Empty States table
    - Keyboard nav per UI-SPEC §Surface 2 behavior list (↑/↓ row focus, Enter opens detail, A ack, S snooze menu)

    **web/src/routes/alerts/AlertCenterFilters.tsx:** Popover-per-chip pattern. Each chip is a Button with the current value as label and a chevron; opens Popover with options. URL-state via useAlertParams. "Clear all" resets all chips.

    **web/src/routes/alerts/AlertRow.tsx:** shared between drawer and page; severity dot + multi-line title (rule name → entity label → reading) + actions (Ack button + SnoozeMenu) gated by `useCurrentUser().role === 'admin'`. Click body opens AlertDetailDialog.

    **web/src/routes/alerts/SnoozeMenu.tsx:** DropdownMenu with exactly 5 items in order: "Snooze 1 hour", "Snooze 8 hours", "Snooze 24 hours", "Snooze 7 days", "Mute until I clear" (UI-SPEC §Copywriting). On select calls useSnoozeMutation with the corresponding duration value.

    **web/src/routes/alerts/AlertDetailDialog.tsx:** ResponsiveDialog with title (rule name + SeverityPill aligned right; TEST badge inline if `is_test=true`); body sections per UI-SPEC §Surface 1b (target row / status / reading / notes timeline / collapsible payload JsonTree); footer Acknowledge + Snooze (split) + Cancel. Viewer hides Ack/Snooze.

    **web/src/routes/settings/alerts.tsx (`AlertRulesPage`):** layout per UI-SPEC §Surface 3:
    - AnomalyRosterSection card at top: "Anomaly detection — X meters eligible · Y warming up · [Show roster ▾]" with collapsible roster list grouped by Eligible / Warming up; each row links to /metering-points/${id}
    - Rule library card with: "Show disabled" toggle (URL-state ?show_disabled=0|1), TanStack Table of rules (columns: enabled toggle, name, kind, target, severity, last fired, overflow menu), "Add rule" button top-right
    - Overflow menu (DropdownMenu): Edit / Test fire / Disable | Enable
    - Empty state per UI-SPEC

    **web/src/routes/settings/AddRuleDialog.tsx:** ResponsiveDialog + Stepper with 5 steps from UI-SPEC §"Add Rule dialog — 5 steps":
    1. Scope — Radio: All meters / Site (Combobox) / Metering point (Combobox); **SKIPPED when invoked with prefilled `meteringPointId` or `siteId`**
    2. Kind — Radio (threshold instant/hourly/daily / offline / anomaly p95/iqr/quiet_hour); inline warning if target is `warming_up`
    3. Conditions — depending on kind: threshold gets comparison + value + unit; offline shows read-only computed window; anomaly_quiet_hour gets time-range + days-of-week
    4. Severity + Cooldown + Notes — severity radio (defaults from D-07), cooldown input (default 900), Name override, Notes textarea
    5. Review — summary + Test fire button (D-19) + Save rule
    Form via react-hook-form + zod resolver; on submit calls useCreateRuleMutation. Same component used at three D-18 entry points.

    **web/src/components/metering-point/AnomalyStateCard.tsx:** Card per UI-SPEC §Surface 4; three states (warming_up / eligible_inactive / active); uses `useMPAnomalyState(mpId)`; per-rule Toggle components call useToggleMPAnomalyRuleMutation; admin-only mutation; viewer sees disabled toggles with tooltip.

    Mount card in `web/src/routes/metering-points/$id.tsx` (or equivalent file) **above** the existing Phase 4 tabs.

    **web/src/App.tsx:** Register routes:
    ```tsx
    <Route path="/alerts" element={<AlertsPage />} />
    <Route path="/settings/alerts" element={<AlertRulesPage />} />
    ```
    The /audit route is registered in Plan 06-07.

    **Mount `<AlertWorkerBanner />` and `<AlertBell />`** in the app shell (`web/src/routes/_root.tsx`).

    **web/playwright/specs/alerts-center.spec.ts:** E2E covering rule create → test-fire → drawer ack → /alerts list filter; assertions on toast copy, badge color, chip URL-state.
  </action>
  <verify>
    <automated>pnpm -C web test --run web/src/routes/alerts web/src/routes/settings/alerts web/src/components/shell web/src/components/metering-point/AnomalyStateCard</automated>
  </verify>
  <acceptance_criteria>
    - `web/src/components/shell/sidebar.tsx` contains nav array with `Alerts` and `Audit` items in the new order; `Audit` has `adminOnly: true`
    - `web/src/components/shell/AlertBell.tsx` exists and imports `Bell` from `lucide-react`
    - `web/src/components/shell/AlertDrawer.tsx` uses shadcn `Sheet side="right"` and renders max 10 rows
    - `web/src/components/shell/AlertWorkerBanner.tsx` reads `/api/health/detailed` and renders `Alert` (warning variant) when `alert_worker.degraded=true`
    - `web/src/routes/alerts/index.tsx` exists; uses `useSearchParams` + zod via `alertParams` from `web/src/lib/alertParams.ts`
    - `web/src/routes/alerts/SnoozeMenu.tsx` has exactly 5 DropdownMenuItem entries with the exact UI-SPEC copy: `Snooze 1 hour`, `Snooze 8 hours`, `Snooze 24 hours`, `Snooze 7 days`, `Mute until I clear`
    - `web/src/routes/settings/alerts.tsx` uses TanStack Table and renders AnomalyRosterSection
    - `web/src/routes/settings/AddRuleDialog.tsx` uses `Stepper` and has 5 steps (grep `step={1}` ... `step={5}` or equivalent)
    - `AddRuleDialog` has a prop `meteringPointId?: string` and the Step 1 (Scope) component is conditionally rendered based on whether it's prefilled (grep `meteringPointId` and Step 1 skip logic)
    - `web/src/components/metering-point/AnomalyStateCard.tsx` exists and renders three states based on `eligible`/`days_until_eligible`/`rules`
    - The MP detail route file imports `AnomalyStateCard` and renders it **above** the tabs: grep `<AnomalyStateCard` in the MP detail file
    - All listed component tests pass: `pnpm -C web test --run web/src/routes/alerts -- --reporter=verbose` exits 0
    - Playwright spec passes: `pnpm -C web exec playwright test alerts-center` exits 0
  </acceptance_criteria>
  <done>Operator can: see severity-tinted bell badge; open drawer; ack from drawer; click "See all"; filter on /alerts via URL chips; create a rule from Settings → Alerts using the 5-step dialog; test-fire; see anomaly state on MP detail; see degraded banner during alert worker failures. Viewer is read-only throughout.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| client→API | All /api/alerts/* + /api/anomaly-roster + /api/metering-points/.../anomaly-state routes accept HTTP from authenticated users |
| viewer→admin actions | Viewer must not be able to ack/snooze/mute or create/edit rules even by crafting raw HTTP |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-06-04-01 | Elevation of Privilege | viewer ack/snooze via curl | mitigate | `auth.RequireAction(sm, auth.ActionAlertAck)` middleware wrapping POST /api/alerts/{id}/ack; ActionAlertAck NOT in roleBundles[RoleViewer]; Can() returns false; 403. Test: TestAuthz_AlertActionsAdminOnlyExceptRead asserts viewer denied. |
| T-06-04-02 | Tampering | rule scope_id pointing at wrong entity type | mitigate | `CreateRuleHandler.validateScopeKindScopeID(scope_kind, scope_id)` rejects mismatched type/id pairs; 422. Scope expansion in workers is type-aware. |
| T-06-04-03 | Information Disclosure | viewer sees alert payload with sensitive readings | accept | Alert payload values (kW, m³/h, last_uplink) are operator's own data already visible on dashboard; AUTH-06 RBAC permits viewer read. |
| T-06-04-04 | DoS | test-fire spam suppressing real fires | mitigate | Pitfall 8: TestFireHandler explicitly does NOT call `TouchLastFiredAt`; test alerts have is_test=true and auto-clear at 60s via River one-shot job. Test: TestTestFire_DoesNotTouchLastFiredAt. |
| T-06-04-05 | Information Disclosure | severity-tinted bell badge leaks alert count to unauthenticated user | mitigate | Bell + drawer + page mounted in authenticated app shell only (`_root.tsx` is post-auth-loader); login screen has no bell. |
| T-06-04-06 | Tampering | optimistic Ack UI shows ack even if server 403s | mitigate | useAckMutation has onError handler that revalidates the query and unrolls the optimistic update; sonner destructive toast shown. |
| T-06-04-07 | Information Disclosure | "View details" admin-only link visible to viewer | mitigate | DegradedBanner conditionally renders the link via `useCurrentUser()?.role === 'admin'`; viewer sees text-only banner. Test: DegradedBanner_ViewerHidesViewDetailsLink. |
</threat_model>

<verification>
- All listed routes are wired in router.go with RequireAction guards
- All UI surfaces render the locked CTA copy from UI-SPEC §Copywriting
- Sidebar order matches UI-SPEC §Surface 8 exactly
- Severity color map per UI-SPEC §Severity map (locked)
- 5-step Add Rule dialog skips Step 1 when scope is prefilled (D-18)
- Test-fire never updates last_fired_at (Pitfall 8)
- Viewer has read-only access throughout (drawer + page + dialog) — server-enforced via Can()
- `go test ./internal/alert/... -count=1 -timeout=120s` passes
- `pnpm -C web test --run` passes for the new test files
- Playwright `alerts-center.spec.ts` passes
</verification>

<success_criteria>
- ALERT-05 covered: in-app alert center with unread badge, ack with notes, snooze/mute, distinguished severities by color (the locked map), three categories
- ALERT-06 covered: the alert browse (in-app) ships here; the audit CSV export is Plan 06-07 territory (this plan calls "Export CSV" but stubs the route)
- D-11 viewers see alerts read-only (server + UI)
- D-16 cold-start chip on MP detail ships
- D-18 three rule-creation entry points all open the same Add Rule dialog
- D-19 test-fire pattern doesn't suppress real fires
- D-20 bell + drawer + page surface shipped
- D-22 degraded banner ships and auto-dismisses
</success_criteria>

<output>
After completion, create `.planning/phases/06-alerts-users-audit-operational-hardening/06-04-SUMMARY.md`
</output>
