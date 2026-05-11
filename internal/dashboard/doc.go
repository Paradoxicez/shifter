// Package dashboard implements the three dashboard REST endpoints that feed
// the KPI tiles, consumption charts, and onboarding empty-state in the
// Shifter frontend:
//
//   - GET /api/dashboard/scope      — D-09 capability flag + D-21 onboarding tuple
//   - GET /api/dashboard/snapshot   — D-05/D-06/D-07/D-08 KPI tiles + latest readings
//   - GET /api/dashboard/timeseries — D-12 time-bucket aggregated cumulative series
//
// Design decisions:
//
// D-05 "today" boundary uses install_identity.timezone (read server-side; NEVER
// from the request query string per T-04-04-04 threat register). The timezone
// string is passed as a sqlc.arg parameter to the TodayConsumptionByUtility and
// PeriodDeltaByUtility queries.
//
// D-06 instantaneous total: latest instant_value per metering-point, summed
// across all active MPs of the given utility class. The handler uses
// CurrentInstantSumByUtility.
//
// D-07 online/offline rule (verbatim):
//
//	COUNT(*) FILTER (
//	    WHERE d.last_seen_at > now() - (2 * dp.expected_interval_s * INTERVAL '1 second')
//	) AS online_count
//
// Per-profile expected_interval_s from migration 0023 (Plan 04-01) supplies
// the per-vendor threshold (water=3600s, electricity=300s).
//
// D-08 period delta: today [00:00→now] vs yesterday [00:00→same-time-yesterday].
// The PeriodDeltaByUtility query returns both slices; the handler computes
// abs + pct change. Returns null for both fields when yesterday has no data
// (install <24 h old).
//
// D-09 capability gating: the kpis map in GET /api/dashboard/snapshot only
// includes keys for active utilities. If capabilities='water', the electricity
// key is absent entirely (not null). The frontend uses 'water' in kpis to gate
// rendering.
//
// D-12 bucket schedule (implemented in BucketIntervalForRange):
//
//	| range        | bucket_interval |
//	|--------------|-----------------|
//	| today        | 5 minutes       |
//	| 24h          | 5 minutes       |
//	| 7d           | 1 hour          |
//	| 30d          | 4 hours         |
//	| custom ≤ 30d | 1 hour          |
//	| custom > 30d | 1 day           |
//
// D-21 progressive onboarding: the scope endpoint returns gateway_count,
// device_count, and uplink_count so the frontend can decide which empty-state
// card to show (no gateways → add a gateway; gateways but no devices → add a
// device; devices but no uplinks → check LoRaWAN signal).
//
// D-23 authz: all three endpoints are mounted under the authenticated chi
// group (any role — both admin and viewer have full read access to dashboard
// data). The auth gate is enforced at the router level via DashboardDeps
// placement inside the authenticated group in internal/http/router.go.
//
// T-04-04-01/03: the range and utility parameters are enum-validated; invalid
// values return 400 before any DB query executes. Custom range is bounded to
// 365 days maximum (T-04-04-03 DoS protection).
//
// T-04-04-04: timezone is always sourced from install_identity.timezone
// (server-side DB read); never from the URL query string.
package dashboard
