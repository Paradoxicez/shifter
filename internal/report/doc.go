// Package report assembles consumption reports from the CAGG hierarchy
// (DATA-11..13) and emits CSV (REPT-03) + Excel (REPT-04) artifacts. PDF
// generation lives in plan 05-06 via a River background worker (REPT-05/06)
// and writes to the same on-disk artifact directory.
//
// # Scope and grouping (D-01, D-04, D-05)
//
//	scope=all + group=site      → per-site rollups
//	scope=all + group=category  → per-utility-class rollups (water / electricity)
//	scope=all + group=none      → fleet-wide totals only
//	scope=site                  → per-MP rows within one site
//	scope=meter                 → single MP detail
//
// # CAGG selection
//
//	range=daily   → measurement_hourly (bucketed to days in the query)
//	range=monthly → measurement_daily  (bucketed to months in the query)
//	range=yearly  → measurement_monthly (bucketed to years in the query)
//	range=custom  → planner-routed: ≤30d uses hourly, ≤1y uses daily, longer uses monthly
//
// Never raw measurement — 90d+ raw queries are expensive (Phase 4 D-12 lesson).
//
// # Period-delta (D-03)
//
//	delta_vs_prior — always populated when a prior period exists in the data
//	delta_vs_yoy   — populated only when ≥1 measurement exists in the same
//	                  window one year earlier; nil = silent fallback (D-03)
//
// # Single-capability install
//
// When install_identity.capabilities = 'water' (Phase 4 D-09), reports
// silently drop the electricity section; the assembler emits Sections only
// for capability-matching MPs. UI-SPEC empty state covers the zero-MP case.
//
// # Ephemeral artifacts (D-07)
//
// Each report.id has an artifact_dir under /var/lib/shifter/reports/<uuid>/.
// CSV + XLSX written synchronously; PDF arrives async via plan 05-06. River
// PeriodicJob purges expires_at < now() every hour.
//
// # Audit (D-23)
//
// audit.WriteEntry(ctx, tx, …) lands in the same pgx.Tx as the report INSERT.
// Action: report.generate. Entity: report. After: scope, range, group_by.
package report
