# Phase 6 Deferred Items

## Pre-existing failing tests (out of scope per SCOPE BOUNDARY rule)

**TestPhase3Migrations_0018_Down** and **TestPhase3Migrations_0019_Down**
were failing on `main` (commit 72b0b6c) before Plan 06-01 began. The
step-count expressed in the test (`runMigrateSteps(t, pool, -17)` /
`-16` at the time) was off-by-one relative to the actual chain length
when the test was written (Plan 05-11 shipped 36 migrations; rolling
down from version 36 to "before 0018" needs 19 steps, not 17). The
error mode is identical on `main` and on this plan branch — these are
NOT regressions caused by Plan 06-01.

Plan 06-01 updated the comment + step values to reflect the new chain
length (43 with a deliberate gap at 0041):
- `_0018_Down`: -17 → -23
- `_0019_Down`: -16 → -22

Plan 06-02 added 0045_device_gateway_link (chain length 45) and bumped
the step counts again to keep them internally consistent — the
underlying off-by-one in the test logic is unchanged:
- `_0018_Down`: -23 → -25 (with the intermediate -24 landing in Plan 06-05)
- `_0019_Down`: -22 → -24 (with the intermediate -23 landing in Plan 06-05)

The underlying off-by-one was NOT fixed — that's a logic bug independent
of Phase 6 scope. A future plan should rewrite both tests to assert the
final schema_migrations version after the step rather than encoding a
hand-counted step constant. Suggested rewrite:

```go
// Roll back until "0018" is the highest unapplied migration:
for {
    var v int
    pool.QueryRow(ctx, `SELECT version FROM schema_migrations`).Scan(&v)
    if v < 18 { break }
    runMigrateSteps(t, pool, -1)
}
```

Repro on `main`:
```
git checkout 72b0b6c -- internal/db/migrations_test.go
go test ./internal/db/... -run "TestPhase3Migrations_0018_Down" -count=1
# → FAIL (gateway table still present after -17 steps from v36)
```

## D-51 architectural adjustment (Plan body vs reality)

The plan body proposed `SET LOCAL session_replication_role = 'replica'`
to disable triggers inside the SECURITY DEFINER prune function. In
PostgreSQL 13+ this parameter requires superuser, which contradicts the
"owned by a restricted role (shifter_audit_admin)" goal that the same
plan body asserts.

Plan 06-01 applies Rule 1 (auto-fix bug) and replaces the bypass with a
custom-GUC marker (`shifter.allow_audit_prune`) inside an updated
`audit_log_reject_modification()` trigger function. The trigger DDL from
0016 is unchanged; only the function body is rewritten in 0043. The
marker is set via `set_config(..., is_local := true)` which works for
every role on user-defined GUCs.

This preserves the table-level INSERT-ONLY invariant for every code path
that does not first call `admin_prune_audit_rows()` AND deliberately
opts into the bypass for one transaction. The `shifter_audit_admin` role
also picks up SELECT on `audit_log` (in addition to DELETE+INSERT) — the
`DELETE ... WHERE time < $1` statement reads the rows it deletes, so
SELECT is unavoidable.

Documented in the migration body header. Threat-model row T-06-01-02
("session_replication_role escape") is rendered moot because that
mechanism is no longer used; the new mechanism's mitigation is the
narrower trigger-function gate + transaction-local scope of
set_config(is_local=true).
