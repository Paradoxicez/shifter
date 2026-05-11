package profile

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// TestRunSeedSync_FreshInstall — applying migrations seeds 3 profiles with
// codec_js empty + cs_profile_id NULL. RunSeedSync pushes all 3 codecs to
// CS, fills cs_profile_id + codec_js_synced_at, and CodecBySlug-routed
// codec bodies land in codec_js.
func TestRunSeedSync_FreshInstall(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, nopLogger()))

	cs := &fakeCSClient{}
	store := &fakeConnStore{tenantID: uuid.NewString(), appID: uuid.NewString()}
	deps := Deps{Pool: pool, CSClient: cs, ConnStore: store, Log: nopLogger()}

	// Pre-condition: 3 seeded profiles exist with codec_js empty.
	var preUnsynced int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM device_profile
		 WHERE archived_at IS NULL
		   AND (cs_profile_id IS NULL OR codec_js_synced_at IS NULL)`,
	).Scan(&preUnsynced))
	require.Equal(t, 3, preUnsynced, "0010 seed inserts 3 unsynced profiles")

	RunSeedSync(ctx, deps)

	// CS Create called exactly 3 times (one per seed slug).
	require.Equal(t, int64(3), cs.CreateCalls(),
		"every seeded profile must be Created once on first sync")
	require.Equal(t, int64(0), cs.UpdateCalls())

	// Each seeded profile now has codec_js populated AND cs_profile_id +
	// codec_js_synced_at non-NULL.
	rows, err := pool.Query(ctx,
		`SELECT slug, codec_js, cs_profile_id::text, codec_js_synced_at::text
		 FROM device_profile
		 WHERE archived_at IS NULL
		 ORDER BY slug`)
	require.NoError(t, err)
	defer rows.Close()
	got := map[string]struct {
		codec      string
		csID       *string
		syncedAt   *string
	}{}
	for rows.Next() {
		var slug, codec string
		var csID, syncedAt *string
		require.NoError(t, rows.Scan(&slug, &codec, &csID, &syncedAt))
		got[slug] = struct {
			codec    string
			csID     *string
			syncedAt *string
		}{codec, csID, syncedAt}
	}
	require.NoError(t, rows.Err())

	for _, slug := range []string{"axioma_w1", "acrel_adl200", "acrel_adw300"} {
		row, ok := got[slug]
		require.True(t, ok, "slug %s missing", slug)
		require.NotEmpty(t, row.codec, "codec_js must be filled for %s", slug)
		require.Contains(t, row.codec, "function decodeUplink",
			"codec_js for %s must contain decodeUplink", slug)
		require.NotNil(t, row.csID, "cs_profile_id must be set for %s", slug)
		require.NotNil(t, row.syncedAt, "codec_js_synced_at must be set for %s", slug)
	}

	// Pitfall 6 — Acrel slugs share the SAME codec body.
	require.Equal(t, got["acrel_adl200"].codec, got["acrel_adw300"].codec,
		"Pitfall 6: Acrel ADL200 + ADW300 must share one codec body")
	require.NotEqual(t, got["axioma_w1"].codec, got["acrel_adl200"].codec,
		"Axioma + Acrel must NOT share a codec body")
}

// TestRunSeedSync_AlreadySynced — second invocation with all 3 already
// synced is a no-op: ListUnsyncedProfiles returns 0, no CS calls.
func TestRunSeedSync_AlreadySynced(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, nopLogger()))

	cs := &fakeCSClient{}
	store := &fakeConnStore{tenantID: uuid.NewString(), appID: uuid.NewString()}
	deps := Deps{Pool: pool, CSClient: cs, ConnStore: store, Log: nopLogger()}

	// First run pushes all 3.
	RunSeedSync(ctx, deps)
	require.Equal(t, int64(3), cs.CreateCalls())

	// Second run is a no-op. (Reset call counter on a fresh client to make
	// the assertion explicit.)
	cs2 := &fakeCSClient{}
	deps.CSClient = cs2
	RunSeedSync(ctx, deps)
	require.Equal(t, int64(0), cs2.CreateCalls(), "second run: nothing to sync")
	require.Equal(t, int64(0), cs2.UpdateCalls())
}

// TestRunSeedSync_PartialSyncOnCSFailure — the second profile's CS Create
// fails; the other two succeed. The failed slug stays unsynced
// (cs_profile_id IS NULL); the next run can retry it.
func TestRunSeedSync_PartialSyncOnCSFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, nopLogger()))

	// CS injection: succeed, fail, succeed (in iteration order, which is
	// vendor ASC, name ASC per ListActiveDeviceProfiles ordering — but
	// ListUnsyncedProfiles orders by table-default; the test does NOT depend
	// on which two succeed, only that exactly one slug stays unsynced).
	cs := &fakeCSClient{
		createReturns: []csCreateResult{
			{id: uuid.NewString()},
			{id: "", err: errors.New("simulated CS Unavailable")},
			{id: uuid.NewString()},
		},
	}
	store := &fakeConnStore{tenantID: uuid.NewString(), appID: uuid.NewString()}
	deps := Deps{Pool: pool, CSClient: cs, ConnStore: store, Log: nopLogger()}

	RunSeedSync(ctx, deps)

	require.Equal(t, int64(3), cs.CreateCalls(), "all 3 are attempted, even though one fails")

	// 2 of 3 are now synced; 1 stays unsynced.
	var stillUnsynced int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM device_profile
		 WHERE archived_at IS NULL
		   AND (cs_profile_id IS NULL OR codec_js_synced_at IS NULL)`,
	).Scan(&stillUnsynced))
	require.Equal(t, 1, stillUnsynced, "exactly one profile must remain unsynced after partial failure")

	// A re-run with a fresh client succeeds for the remaining profile.
	cs2 := &fakeCSClient{}
	deps.CSClient = cs2
	RunSeedSync(ctx, deps)
	require.Equal(t, int64(1), cs2.CreateCalls(), "retry must Create exactly the remaining profile")

	var afterRetryUnsynced int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM device_profile
		 WHERE archived_at IS NULL
		   AND (cs_profile_id IS NULL OR codec_js_synced_at IS NULL)`,
	).Scan(&afterRetryUnsynced))
	require.Equal(t, 0, afterRetryUnsynced, "retry must clear all unsynced profiles")
}

// TestRunSeedSync_BootsWithoutCS — Pitfall 9 mitigation. ConnStore returns
// empty tenant ID (CS not bootstrapped). RunSeedSync logs a warning and
// returns; the boot continues. Zero CS calls. Profiles stay unsynced.
func TestRunSeedSync_BootsWithoutCS(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, nopLogger()))

	cs := &fakeCSClient{}
	store := &fakeConnStore{tenantID: ""} // <-- not bootstrapped
	deps := Deps{Pool: pool, CSClient: cs, ConnStore: store, Log: nopLogger()}

	// Must NOT panic, NOT block, NOT error (function returns void per Pitfall 9).
	RunSeedSync(ctx, deps)

	require.Equal(t, int64(0), cs.CreateCalls(), "no CS calls when tenant not bootstrapped")
	require.Equal(t, int64(0), cs.UpdateCalls())

	// All 3 seed profiles still unsynced — they'll retry on next boot.
	var stillUnsynced int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM device_profile
		 WHERE archived_at IS NULL
		   AND (cs_profile_id IS NULL OR codec_js_synced_at IS NULL)`,
	).Scan(&stillUnsynced))
	require.Equal(t, 3, stillUnsynced)
}

// TestRunSeedSync_StoreReadFails — ConnStore.GetCSConnection returns error
// (e.g. db hiccup at boot). RunSeedSync logs and returns; zero CS calls.
func TestRunSeedSync_StoreReadFails(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, nopLogger()))

	cs := &fakeCSClient{}
	store := &fakeConnStore{getErr: errors.New("temporary db hiccup")}
	deps := Deps{Pool: pool, CSClient: cs, ConnStore: store, Log: nopLogger()}

	RunSeedSync(ctx, deps)

	require.Equal(t, int64(0), cs.CreateCalls())
	require.Equal(t, int64(0), cs.UpdateCalls())
}

// TestSeed_ExpectedIntervalS_PerProfile — Plan 04-01 Task 4 (D-07 backfill).
// Migration 0023 adds device_profile.expected_interval_s and seeds realistic
// per-vendor values (3600 for water; 300 for electricity). The KPI rule
// `device.last_seen_at > now() - 2 * expected_interval_s` (D-07) breaks if
// the per-profile values regress to a uniform default.
//
// Asserts:
//   - axioma_w1     (water Axioma Qalcosonic W1)        → 3600 (default)
//   - acrel_adl200  (electricity Acrel ADL200)          → 300  (5-minute cadence)
//   - acrel_adw300  (electricity Acrel ADW300)          → 300  (5-minute cadence)
func TestSeed_ExpectedIntervalS_PerProfile(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, nopLogger()))

	want := map[string]int32{
		"axioma_w1":    3600,
		"acrel_adl200": 300,
		"acrel_adw300": 300,
	}

	for slug, expected := range want {
		var got int32
		err := pool.QueryRow(ctx,
			`SELECT expected_interval_s FROM device_profile WHERE slug = $1`,
			slug,
		).Scan(&got)
		require.NoError(t, err, "looking up expected_interval_s for %s", slug)
		require.Equal(t, expected, got,
			"expected_interval_s mismatch for %s — D-07 KPI threshold depends on this", slug)
	}

	// Defensive: every seeded profile must satisfy the > 0 CHECK constraint.
	var minVal int32
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT min(expected_interval_s) FROM device_profile WHERE archived_at IS NULL`,
	).Scan(&minVal))
	require.Greater(t, minVal, int32(0),
		"every active profile must have positive expected_interval_s (CHECK constraint)")
}

// TestRunSeedSync_UpdatesAlreadyPushed — pre-pin a fake cs_profile_id on one
// profile, then clear codec_js_synced_at (simulates a profile editor save
// that bumped the codec). RunSeedSync MUST take the Update path, not Create.
func TestRunSeedSync_UpdatesAlreadyPushed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, nopLogger()))

	// Pre-pin a UUID on axioma_w1 + clear synced_at so it's "unsynced for update".
	prePinned := uuid.NewString()
	_, err := pool.Exec(ctx,
		`UPDATE device_profile
		 SET cs_profile_id = $1, codec_js_synced_at = NULL
		 WHERE slug = 'axioma_w1'`,
		prePinned)
	require.NoError(t, err)

	// Mark the other two as fully synced so they don't show up in the list.
	_, err = pool.Exec(ctx,
		`UPDATE device_profile
		 SET cs_profile_id = $1, codec_js_synced_at = now(), codec_js = 'noop'
		 WHERE slug IN ('acrel_adl200', 'acrel_adw300')`,
		uuid.NewString())
	require.NoError(t, err)

	cs := &fakeCSClient{}
	store := &fakeConnStore{tenantID: uuid.NewString(), appID: uuid.NewString()}
	deps := Deps{Pool: pool, CSClient: cs, ConnStore: store, Log: nopLogger()}

	RunSeedSync(ctx, deps)

	require.Equal(t, int64(0), cs.CreateCalls(), "pre-pinned cs_profile_id must take Update path")
	require.Equal(t, int64(1), cs.UpdateCalls(), "axioma_w1 must Update exactly once")

	// cs_profile_id stays at the pre-pinned value (Update doesn't change it).
	var got string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT cs_profile_id::text FROM device_profile WHERE slug = 'axioma_w1'`,
	).Scan(&got))
	require.Equal(t, prePinned, got, "Update path must NOT change cs_profile_id")
}
