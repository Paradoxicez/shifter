// Package alert is the Phase 6 alert-engine substrate.
//
// # Three workers, three cadences, one River queue (D-03)
//
// Phase 6 ships three distinct rule kinds — threshold, offline, anomaly —
// each evaluated by a different River worker:
//
//   - ThresholdWorker  (Plan 06-02): polls measurement/CAGGs, three subtypes
//   - OfflineWorker    (Plan 06-02): N≥3 missed expected uplinks, gateway suppression
//   - AnomalyWorker    (Plan 06-03): P95, IQR, quiet-hour rules; 21-day cold-start gate
//
// All three workers land in the default River queue so retry policy and
// observability surfaces are uniform. Per-cycle observability lives in the
// alert_worker_state table (one row per worker_kind, see 0042 migration).
//
// # Substrate (this plan, 06-01)
//
// This package ships the SHARED scaffolding consumed by all three evaluator
// plans:
//
//   - EvaluateContext (engine.go): dependency bundle held by every worker
//   - RuleStore       (rule_store.go): CRUD over alert_rule (D-04 soft delete)
//   - AlertStore      (alert_store.go): InsertAlert + state transitions
//   - WorkerStateStore(worker_state.go): UPSERT alert_worker_state
//   - StartDegradedSubscriber (degraded.go): D-22 retry-exhaustion → degraded flag
//   - AuditPruneWorker (audit_prune_worker.go): D-51 retention prune
//
// # Cooldown enforcement (D-05)
//
// Cooldown is enforced IN-ENGINE by reading alert_rule.last_fired_at +
// cooldown_seconds < now() before evaluating a rule. Worker-emitted fires
// call TouchLastFiredAt; D-19 test-fires do NOT — operators can validate the
// UI plumbing without consuming the cooldown budget.
//
// # Idempotent fire (D-08)
//
// alert.alert_firing_unique_idx is a partial unique index on (rule_id,
// target_entity_id) WHERE state='firing'. InsertAlert returns ErrDuplicateFire
// when a worker re-emits a fire for the same (rule, target) while the
// previous one is still firing; callers MUST treat this as a no-op (NOT an
// error to surface).
package alert
