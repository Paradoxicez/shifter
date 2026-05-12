# Phase 6 Deferred Items

## Pre-existing failing tests (out of scope per SCOPE BOUNDARY rule)

**TestPhase3Migrations_0018_Down** and **TestPhase3Migrations_0019_Down** were
failing on `main` (commit 72b0b6c) before Plan 06-01 began. The step-count
expressed in the test (`runMigrateSteps(t, pool, -17)` / `-16`) is off-by-one
relative to the actual chain length at the time of the test write (Plan 05-11
shipped 36 migrations; rolling down from version 36 to "before 0018" needs 19
steps, not 17). The error mode is exactly the same on `main` as in this plan
branch — these are NOT regressions caused by Plan 06-01.

Plan 06-01 updated the comment + step values to +1 / +1 (so the tests remain
internally consistent at chain length 37) but did not fix the underlying
off-by-one. A future plan should rewrite both tests against the actual
chain-end pattern (e.g. assert version after `runMigrateSteps` rather than
relying on a hand-counted step constant).

Repro on `main`:
```
git checkout 72b0b6c -- .
go test ./internal/db/... -run "TestPhase3Migrations_0018_Down" -count=1
# → FAIL (gateway table still present after -17 steps from v36)
```

## Phase 6 forward-compat notes

(none yet)
