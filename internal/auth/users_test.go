package auth_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
	"github.com/stretchr/testify/require"
)

// TestIterateAndRevoke_Exported — the function MUST be callable from another
// package. The mere fact that this file compiles (under `package auth_test`)
// is the load-bearing assertion. The body verifies the no-op behavior when
// no sessions exist for the user, which is the safest unit-test path that
// doesn't require a real SCS session manager wired to a context.
func TestIterateAndRevoke_Exported(t *testing.T) {
	if testing.Short() {
		t.Skip("requires postgres testcontainer")
	}
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool,
		slog.New(slog.NewTextHandler(os.Stderr, nil))))
	store := auth.NewStore(pool)

	sm := auth.NewSessionManager(pool, true, 8*time.Hour, 24*time.Hour)
	// Zero-session-state — IterateAndRevoke must complete without error.
	err := auth.IterateAndRevoke(context.Background(), sm, store,
		"00000000-0000-0000-0000-000000000000", "")
	require.NoError(t, err)
}
