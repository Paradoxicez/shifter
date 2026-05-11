# cumulative_delta Pre-Check (Plan 05-01 Task 1)

**Verdict (2026-05-12):** ABSENT. `measurement` hypertable stores `cumulative_value` (absolute reading) but not the per-uplink delta.

**Implication for plan 05-02 (CAGG migrations):** The hourly CAGG MUST compute the delta inline. Two options:

1. Add a stored generated/computed `cumulative_delta` column in a Phase 5 pre-CAGG migration (0025 or similar), backfilled from existing rows via window function.
2. Compute via `cumulative_value - LAG(cumulative_value) OVER (PARTITION BY metering_point_id ORDER BY time)` directly in the hourly CAGG SELECT.

**Recommendation:** Option 2. The CAGG is the single producer of `cumulative_delta` for the rest of Phase 5 (daily/monthly/yearly all sum it from the hourly view). Option 1 would force a backfill window on existing rows and a generated-column constraint on every future INSERT.

**Caveat:** Option 2 introduces a per-MP boundary issue at the first uplink in each hourly bucket — `LAG()` across bucket boundaries needs `OVER (PARTITION BY metering_point_id ORDER BY time)` (not bucketed) so the delta of the first row in a bucket subtracts the last row of the previous bucket. Plan 05-02 implements this verbatim.

**No phase-blocking action required;** plan 05-02 owns the implementation.
