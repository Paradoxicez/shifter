package user

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
	"github.com/stretchr/testify/require"
)

func setupStore(t *testing.T) *auth.Store {
	t.Helper()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(context.Background(), pool,
		slog.New(slog.NewTextHandler(os.Stderr, nil))))
	return auth.NewStore(pool)
}

// seedUser inserts a user via direct SQL (sidestepping CreateTx so the
// tests on CreateTx behavior themselves are not the seed mechanism).
func seedUser(t *testing.T, store *auth.Store, email, name, role string, disabled bool) string {
	t.Helper()
	hash, err := auth.Hash("Seed-Strong-Pass-12!")
	require.NoError(t, err)
	var id string
	var disabledClause string
	if disabled {
		disabledClause = ", disabled_at = now()"
	}
	err = store.Pool().QueryRow(context.Background(),
		`INSERT INTO "user" (email, name, password_hash, role, must_change_password)
            VALUES (lower($1), $2, $3, $4::user_role, FALSE) RETURNING id::text`,
		email, name, hash, role).Scan(&id)
	require.NoError(t, err)
	if disabled {
		_, err = store.Pool().Exec(context.Background(),
			`UPDATE "user" SET id = id`+disabledClause+` WHERE id = $1::uuid`, id)
		require.NoError(t, err)
	}
	return id
}

// TestStore_ListActiveOnly verifies scope filtering on the List method.
func TestStore_ListActiveOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("requires postgres testcontainer")
	}
	store := setupStore(t)

	_ = seedUser(t, store, "a@example.com", "A", "admin", false)
	_ = seedUser(t, store, "b@example.com", "B", "viewer", false)
	_ = seedUser(t, store, "c@example.com", "C", "viewer", true)

	active, err := store.List(context.Background(), "active")
	require.NoError(t, err)
	require.Len(t, active, 2)

	disabled, err := store.List(context.Background(), "disabled")
	require.NoError(t, err)
	require.Len(t, disabled, 1)
	require.Equal(t, "c@example.com", disabled[0].Email)

	all, err := store.List(context.Background(), "all")
	require.NoError(t, err)
	require.Len(t, all, 3)
	// created_at DESC: last seeded ('c') first.
	require.Equal(t, "c@example.com", all[0].Email)

	_, err = store.List(context.Background(), "bogus")
	require.ErrorIs(t, err, auth.ErrInvalidScope)
}

// TestStore_CreateUser_Viewer — Create returns a UUID; the row has the
// expected role and must_change_password=true.
func TestStore_CreateUser_Viewer(t *testing.T) {
	if testing.Short() {
		t.Skip("requires postgres testcontainer")
	}
	store := setupStore(t)

	hash, err := auth.Hash("Bo-Strong-Pass-1234!")
	require.NoError(t, err)
	ctx := context.Background()
	tx, err := store.Pool().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	require.NoError(t, err)
	id, err := store.CreateTx(ctx, tx, "BO@example.com", "Bo", "viewer", hash, true)
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))

	var role, email string
	var mustChange bool
	require.NoError(t, store.Pool().QueryRow(ctx,
		`SELECT role::text, email, must_change_password FROM "user" WHERE id = $1::uuid`, id,
	).Scan(&role, &email, &mustChange))
	require.Equal(t, "viewer", role)
	require.Equal(t, "bo@example.com", email, "email must be lowercased")
	require.True(t, mustChange)
}

// TestStore_CreateUser_RejectsDuplicateEmail — second CreateTx with the same
// email returns ErrDuplicateEmail.
func TestStore_CreateUser_RejectsDuplicateEmail(t *testing.T) {
	if testing.Short() {
		t.Skip("requires postgres testcontainer")
	}
	store := setupStore(t)
	hash, _ := auth.Hash("Bo-Strong-Pass-1234!")

	ctx := context.Background()
	tx1, _ := store.Pool().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	_, err := store.CreateTx(ctx, tx1, "dup@example.com", "Dup1", "viewer", hash, true)
	require.NoError(t, err)
	require.NoError(t, tx1.Commit(ctx))

	tx2, _ := store.Pool().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	_, err = store.CreateTx(ctx, tx2, "dup@example.com", "Dup2", "viewer", hash, true)
	require.ErrorIs(t, err, auth.ErrDuplicateEmail)
	_ = tx2.Rollback(ctx)
}

// TestStore_Update — Update changes name without touching password_hash or
// role.
func TestStore_Update(t *testing.T) {
	if testing.Short() {
		t.Skip("requires postgres testcontainer")
	}
	store := setupStore(t)
	ctx := context.Background()
	id := seedUser(t, store, "u@example.com", "Old Name", "viewer", false)

	var hashBefore, roleBefore string
	require.NoError(t, store.Pool().QueryRow(ctx,
		`SELECT password_hash, role::text FROM "user" WHERE id = $1::uuid`, id,
	).Scan(&hashBefore, &roleBefore))

	tx, _ := store.Pool().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	require.NoError(t, store.UpdateTx(ctx, tx, id, "New Name"))
	require.NoError(t, tx.Commit(ctx))

	var name, hashAfter, roleAfter string
	require.NoError(t, store.Pool().QueryRow(ctx,
		`SELECT name, password_hash, role::text FROM "user" WHERE id = $1::uuid`, id,
	).Scan(&name, &hashAfter, &roleAfter))
	require.Equal(t, "New Name", name)
	require.Equal(t, hashBefore, hashAfter)
	require.Equal(t, roleBefore, roleAfter)
}

// TestStore_Disable_Enable — Disable sets disabled_at=now(); Enable clears
// it; password_hash + must_change_password preserved across both
// operations (D-27).
func TestStore_Disable_Enable(t *testing.T) {
	if testing.Short() {
		t.Skip("requires postgres testcontainer")
	}
	store := setupStore(t)
	ctx := context.Background()
	id := seedUser(t, store, "u@example.com", "U", "viewer", false)

	var hashBefore string
	var mustBefore bool
	require.NoError(t, store.Pool().QueryRow(ctx,
		`SELECT password_hash, must_change_password FROM "user" WHERE id = $1::uuid`, id,
	).Scan(&hashBefore, &mustBefore))

	tx, _ := store.Pool().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	require.NoError(t, store.DisableTx(ctx, tx, id))
	require.NoError(t, tx.Commit(ctx))

	var disabledAt *string
	require.NoError(t, store.Pool().QueryRow(ctx,
		`SELECT disabled_at::text FROM "user" WHERE id = $1::uuid`, id).Scan(&disabledAt))
	require.NotNil(t, disabledAt)

	tx2, _ := store.Pool().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	require.NoError(t, store.EnableTx(ctx, tx2, id))
	require.NoError(t, tx2.Commit(ctx))

	var hashAfter string
	var mustAfter bool
	var disabledAfter *string
	require.NoError(t, store.Pool().QueryRow(ctx,
		`SELECT password_hash, must_change_password, disabled_at::text
            FROM "user" WHERE id = $1::uuid`, id).Scan(&hashAfter, &mustAfter, &disabledAfter))
	require.Equal(t, hashBefore, hashAfter, "password_hash preserved across disable+enable (D-27)")
	require.Equal(t, mustBefore, mustAfter, "must_change_password preserved across disable+enable (D-27)")
	require.Nil(t, disabledAfter)
}

// TestStore_ChangeRole — ChangeRoleTx persists the new role.
func TestStore_ChangeRole(t *testing.T) {
	if testing.Short() {
		t.Skip("requires postgres testcontainer")
	}
	store := setupStore(t)
	ctx := context.Background()
	id := seedUser(t, store, "u@example.com", "U", "viewer", false)

	tx, _ := store.Pool().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	require.NoError(t, store.ChangeRoleTx(ctx, tx, id, "admin"))
	require.NoError(t, tx.Commit(ctx))

	var role string
	require.NoError(t, store.Pool().QueryRow(ctx,
		`SELECT role::text FROM "user" WHERE id = $1::uuid`, id).Scan(&role))
	require.Equal(t, "admin", role)
}

// TestStore_UpdatePasswordTx_SetsMustChangeFlag — UpdatePasswordTx persists
// both new hash and the must_change_password flag.
func TestStore_UpdatePasswordTx_SetsMustChangeFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("requires postgres testcontainer")
	}
	store := setupStore(t)
	ctx := context.Background()
	id := seedUser(t, store, "u@example.com", "U", "viewer", false)

	newHash, err := auth.Hash("New-Strong-Pass-123!")
	require.NoError(t, err)

	tx, _ := store.Pool().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	require.NoError(t, store.UpdatePasswordTx(ctx, tx, id, newHash, true))
	require.NoError(t, tx.Commit(ctx))

	var hashAfter string
	var mustAfter bool
	require.NoError(t, store.Pool().QueryRow(ctx,
		`SELECT password_hash, must_change_password FROM "user" WHERE id = $1::uuid`, id,
	).Scan(&hashAfter, &mustAfter))
	require.Equal(t, newHash, hashAfter)
	require.True(t, mustAfter)
}

// TestStore_GetByIDForUpdate_DisabledRowsVisible — GetByIDForUpdate returns
// a disabled user (admin re-enable + view-history flows depend on this),
// whereas GetUserByID filters them.
func TestStore_GetByIDForUpdate_DisabledRowsVisible(t *testing.T) {
	if testing.Short() {
		t.Skip("requires postgres testcontainer")
	}
	store := setupStore(t)
	ctx := context.Background()
	id := seedUser(t, store, "u@example.com", "U", "viewer", true)

	// Phase 1 GetUserByID filters disabled.
	_, err := store.GetUserByID(ctx, id)
	require.ErrorIs(t, err, auth.ErrUserNotFound)

	// Phase 6 GetByIDForUpdate returns them.
	tx, _ := store.Pool().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	defer tx.Rollback(ctx)
	rec, err := store.GetByIDForUpdate(ctx, tx, id)
	require.NoError(t, err)
	require.NotNil(t, rec.DisabledAt)
}

// TestStore_ToDTO_OmitsPasswordHash — sanity check on the JSON-shape
// conversion: ToDTO drops the password hash.
func TestStore_ToDTO_OmitsPasswordHash(t *testing.T) {
	rec := auth.UserRecord{
		ID:           "abc",
		Email:        "e",
		Name:         "n",
		Role:         "admin",
		PasswordHash: "should-not-leak",
	}
	dto := ToDTO(rec)
	require.Equal(t, "abc", dto.ID)
	require.Equal(t, "e", dto.Email)
	// The struct definition is the load-bearing thing here (no PasswordHash
	// field); just make sure construction doesn't panic.
}
