package cli

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
	"github.com/stretchr/testify/require"
)

// TestCreateAdmin_CreatesUser — first invocation against an empty DB inserts
// an admin row that successfully verifies against the supplied password.
// D-14 / D-09.
func TestCreateAdmin_CreatesUser(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))

	require.NoError(t,
		createOrResetAdmin(ctx, pool, "ops@example.com", "Ops", "ops-Pass-1234567!", false, nil))

	store := auth.NewStore(pool)
	user, err := store.GetUserByEmail(ctx, "ops@example.com")
	require.NoError(t, err)
	require.Equal(t, "admin", user.Role)
	require.False(t, user.MustChangePassword, "D-09: must_change_password=false on create-admin")

	ok, err := auth.Verify("ops-Pass-1234567!", user.PasswordHash)
	require.NoError(t, err)
	require.True(t, ok, "supplied password must verify against the stored hash")
}

// TestCreateAdmin_RefusesExisting — second invocation without --reset refuses
// with a clear "use --reset" message.
func TestCreateAdmin_RefusesExisting(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))

	require.NoError(t,
		createOrResetAdmin(ctx, pool, "ops@example.com", "Ops", "ops-Pass-1234567!", false, nil))

	err := createOrResetAdmin(ctx, pool, "ops@example.com", "Ops", "another-Pass-2!", false, nil)
	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), "--reset")
}

// TestCreateAdmin_Reset — --reset updates the password of an existing admin.
func TestCreateAdmin_Reset(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))

	require.NoError(t,
		createOrResetAdmin(ctx, pool, "ops@example.com", "Ops", "ops-Pass-1234567!", false, nil))
	require.NoError(t,
		createOrResetAdmin(ctx, pool, "ops@example.com", "Ops", "rotated-Pass-9!", true, nil))

	store := auth.NewStore(pool)
	user, err := store.GetUserByEmail(ctx, "ops@example.com")
	require.NoError(t, err)

	ok, err := auth.Verify("rotated-Pass-9!", user.PasswordHash)
	require.NoError(t, err)
	require.True(t, ok, "rotated password must verify")

	ok, err = auth.Verify("ops-Pass-1234567!", user.PasswordHash)
	require.NoError(t, err)
	require.False(t, ok, "old password must NOT verify after reset")
}

// TestCreateAdmin_LowercaseEmail — emails are lowercased before insert
// regardless of casing the operator typed (defends user_email_lowercase CHECK).
func TestCreateAdmin_LowercaseEmail(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))

	require.NoError(t,
		createOrResetAdmin(ctx, pool, "OPS@Example.COM", "Ops", "ops-Pass-1234567!", false, nil))

	store := auth.NewStore(pool)
	user, err := store.GetUserByEmail(ctx, "ops@example.com")
	require.NoError(t, err)
	require.Equal(t, "ops@example.com", user.Email)
}
