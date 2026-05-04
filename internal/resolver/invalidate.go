package resolver

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
)

// EmitInvalidate publishes NOTIFY binding_changed manually with payload =
// lowercase devEUI. The migration 0017 trigger fires NOTIFY automatically on
// binding INSERT / UPDATE OF valid_to, so this helper is for:
//
//  1. tests (TestListener_*) — to verify the listener picks up payloads
//     without going through a binding row mutation;
//  2. defensive belt-and-suspenders for hypothetical code paths that need
//     to invalidate without touching the binding table (rare; none exist
//     in Phase 2 production paths).
//
// Takes a pgx.Tx (not a pool conn) so it can be issued inside the caller's
// transaction; the NOTIFY is buffered until the tx commits, matching the
// 0017 trigger behavior. To send out-of-tx, callers can wrap with
// pool.BeginTx → EmitInvalidate → tx.Commit.
func EmitInvalidate(ctx context.Context, tx pgx.Tx, devEUI string) error {
	_, err := tx.Exec(ctx, "SELECT pg_notify('binding_changed', $1)", strings.ToLower(devEUI))
	return err
}
