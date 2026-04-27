package db

import "testing"

// TestRunMigrations_Clean — RunMigrations against an empty `shifter_test`
// database applies every migration and leaves schema_migrations on the
// expected latest version with dirty=false.
// Implementation: Plan 03 (database-layer).
func TestRunMigrations_Clean(t *testing.T) {
	t.Skip("Plan 03: clean-slate migration pending")
}

// TestRunMigrations_Idempotent — Running RunMigrations twice in a row is a
// no-op on the second invocation (no spurious version rows, no errors).
// Implementation: Plan 03 (database-layer).
func TestRunMigrations_Idempotent(t *testing.T) {
	t.Skip("Plan 03: re-run idempotency pending")
}

// TestRunMigrations_DirtyState — When schema_migrations has dirty=true,
// RunMigrations refuses to advance and surfaces a typed error so operators
// can investigate manually rather than silently corrupting state.
// Implementation: Plan 03 (database-layer).
func TestRunMigrations_DirtyState(t *testing.T) {
	t.Skip("Plan 03: dirty-state surfacing pending")
}
