// Package aggregate documents the four-level continuous-aggregate hierarchy
// landed by migrations 0025–0028 (Phase 5 DATA-11..13).
//
// # Hierarchy
//
//	measurement (hypertable, raw, 90d retention)
//	  ↓ time_bucket('1 hour') + last−first(cumulative_value) + agg
//	measurement_hourly (real-time ON, 1y retention)
//	  ↓ time_bucket('1 day') + Σ + agg
//	measurement_daily  (real-time ON, 5y retention)
//	  ↓ time_bucket('1 month') + Σ + agg
//	measurement_monthly (real-time OFF, 20y retention)
//	  ↓ time_bucket('1 year') + Σ + agg
//	measurement_yearly (real-time OFF, no retention — forever)
//
// # cumulative_delta semantics
//
// The raw measurement table stores cumulative_value (absolute meter reading,
// monotonic per metering_point modulo rollover). Phase 5's hourly CAGG computes
// the per-bucket consumption as:
//
//	cumulative_delta = last(cumulative_value, time) - first(cumulative_value, time)
//
// This is semantically equivalent to Σ(LAG deltas) because cumulative_value is
// monotonically non-decreasing within a bucket. The first reading of a bucket
// is the continuation point from the previous bucket's last reading, giving
// correct cross-bucket delta semantics without requiring a window function
// nested inside an aggregate (which Postgres forbids: SQLSTATE 42803).
//
// Example with readings [100,110,130,145] in hour-09 and [145,200] in hour-10:
//   - hour-09: last(145) − first(100) = 45
//   - hour-10: last(200) − first(145) = 55
//
// Higher CAGGs (daily/monthly/yearly) sum the lower CAGG's cumulative_delta
// directly — no further math needed because the delta is already a
// non-cumulative consumption value.
//
// # Why not LAG() inside aggregate
//
// sum(cumulative_value - LAG(cumulative_value) OVER (PARTITION BY metering_point_id
// ORDER BY time)) is invalid in TimescaleDB CAGGs because Postgres forbids
// window function calls nested inside aggregate function calls (SQLSTATE 42803).
// The first/last approach is the CAGG-compatible equivalent.
//
// # Retention policy footgun (Pitfall #2 / RESEARCH §CAGG Retention Footgun)
//
// add_retention_policy('measurement', INTERVAL '90 days') drops chunks older
// than 90d. CAGG refresh policies MUST keep start_offset within the 90d window
// or the CAGG silently overwrites already-materialized buckets with NULLs when
// it tries to re-read dropped raw data. Our hourly start_offset is 2 hours —
// safely inside 90 days. The daily/monthly/yearly CAGGs source from lower CAGGs
// (not raw), so their start_offset can exceed 90d without footgun risk.
//
// # Real-time mode (D-11)
//
// hourly + daily: materialized_only=false. Freshly-ingested rows appear in
// 7d/30d charts within the next refresh interval.
//
// monthly + yearly: materialized_only=true. Real-time on rarely-queried
// upper CAGGs causes expensive scan-on-read combining materialized data with
// all raw rows since last refresh. Off is the right default.
//
// # Reference
//
//   - 05-CONTEXT.md D-08 through D-11
//   - 05-RESEARCH.md §CAGG Hierarchy, §CAGG Retention Footgun, §Common Pitfalls
//   - internal/aggregate/CUMULATIVE_DELTA.md (Plan 05-01 pre-check)
package aggregate
