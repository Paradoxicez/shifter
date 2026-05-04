// Package swap implements DATA-04 (atomic close-old-binding + open-new-binding
// + audit-write at Serializable isolation) and DATA-05 (counter rollover
// detection + offset advance) — the two pieces of "domain mechanics" that
// make meter swaps and counter wraps preserve the cumulative_value
// continuity invariant from PROJECT.md.
//
// The package splits cleanly into:
//
//   - math.go — three pure functions (ProposeOffset, DetectRollover,
//     ApplyRollover) with no DB / IO / time.Now() so they can be unit-tested
//     for every plausible numeric edge case in microseconds. Numbers are
//     math/big.Float at prec=128 — exceeds any plausible meter precision
//     without IEEE-754 rounding errors. All three are called from the
//     ingest pipeline (Plan 02-09) per uplink as well as from the swap
//     commit path here.
//
//   - commit.go — the orchestrator. CommitSwap takes a pgxpool.Pool +
//     SwapInput, opens a Serializable txn, calls sqlc.CloseBinding +
//     sqlc.OpenBinding + audit.WriteEntry inside that txn, commits, and
//     calls deps.Resolver.Invalidate for both the outgoing and incoming
//     dev_eui as defense in depth on top of the 0017 NOTIFY trigger.
//     Concurrent commits on the same MP race against the binding_no_overlap
//     EXCLUDE constraint — one wins, the other gets 23P01 (exclusion_violation)
//     which the API layer surfaces as 409 Conflict.
package swap
