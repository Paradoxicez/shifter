// Package settings owns operator-facing configuration that can change after
// install (as distinct from internal/install, which owns the wizard-time
// one-shot setup).
//
// # Data retention (Phase 5 DATA-13 + D-09)
//
// retention_config is a singleton (id=1) row with per-level windows:
//
//	raw_days, hourly_days, daily_days, monthly_days, yearly_days (NULL = forever)
//
// PATCH /api/settings/retention does two things in one pgx.Tx:
//
//  1. UPDATE retention_config (sqlc)
//  2. Call ReconcilePolicies(ctx, tx, before, after) which issues
//     remove_retention_policy(...) + add_retention_policy(...) for each
//     level whose value changed. Both are TimescaleDB-provided functions
//     that take effect at the next retention worker pass (default every
//     1 minute) — they're safe to call inside a tx (only CREATE
//     MATERIALIZED VIEW WITH DATA is the tx-incompatible CAGG operation,
//     per Pitfall #1).
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
// compile-time switch (vetted-literal set), not client input — so no
// runtime quoting/escaping helper is needed. The interval is bound via
// pgx parameter binding (make_interval(days => $1)) so the integer day
// count cannot inject anything.
//
// # Yearly sentinel protocol
//
// RetentionPatch uses a YearlyForever *bool sentinel to distinguish:
//   - patch body omits "yearly_days" entirely → keep existing value unchanged
//   - patch body includes "yearly_forever": true  → set yearly_days = NULL (forever)
//   - patch body includes "yearly_days": N        → set yearly_days = N
//
// This is necessary because JSON "omitempty" cannot distinguish "absent" from
// "explicit null" when the field type is *int.
//
// # SETT-04 traceability
//
// SETT-04 (Settings → Data Retention category) was originally mapped to Phase 6.
// Per RESEARCH §Open Questions #4, the UI ships in Phase 5 alongside DATA-13's
// substrate. REQUIREMENTS.md updated to reflect Phase 5 ownership.
package settings
