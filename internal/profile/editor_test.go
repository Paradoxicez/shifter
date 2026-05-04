package profile

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/chirpstack"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// fakeConnStore is the in-memory ConnectionStore used by editor + seed tests;
// matches chirpstack.ConnectionStore + profile.ConnectionStore (the read half
// is the same interface shape).
type fakeConnStore struct {
	tenantID string
	appID    string
	getErr   error
}

func (s *fakeConnStore) GetCSConnection(_ context.Context) (string, string, error) {
	if s.getErr != nil {
		return "", "", s.getErr
	}
	return s.tenantID, s.appID, nil
}

// fakeCSClient records every Create/Update call so assertions can verify CS
// side effects without driving a real ChirpStack server. createReturns +
// updateReturns are returned in order; if exhausted the calls succeed with a
// fresh UUID (Create) or no-op (Update).
type fakeCSClient struct {
	mu sync.Mutex

	createCalls atomic.Int64
	updateCalls atomic.Int64

	createReturns []csCreateResult // in-order injection
	updateErrs    []error          // in-order Update failures

	lastCreate chirpstack.CreateProfileInput
	lastUpdate chirpstack.UpdateProfileInput
}

type csCreateResult struct {
	id  string
	err error
}

func (c *fakeCSClient) CreateDeviceProfile(_ context.Context, in chirpstack.CreateProfileInput) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.createCalls.Add(1)
	c.lastCreate = in
	if len(c.createReturns) > 0 {
		r := c.createReturns[0]
		c.createReturns = c.createReturns[1:]
		return r.id, r.err
	}
	return uuid.NewString(), nil
}

func (c *fakeCSClient) UpdateDeviceProfile(_ context.Context, in chirpstack.UpdateProfileInput) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.updateCalls.Add(1)
	c.lastUpdate = in
	if len(c.updateErrs) > 0 {
		err := c.updateErrs[0]
		c.updateErrs = c.updateErrs[1:]
		return err
	}
	return nil
}

func (c *fakeCSClient) CreateCalls() int64 { return c.createCalls.Load() }
func (c *fakeCSClient) UpdateCalls() int64 { return c.updateCalls.Load() }

func nopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// validCodec returns a tiny syntactically-valid codec_js the editor will
// accept. ChirpStack's QuickJS sandbox will accept it too (it returns a static
// empty data object).
func validCodec() string {
	return "function decodeUplink(input) { return { data: {} }; }"
}

// editorFixture seeds an admin user + a CS connection row populated with
// cs_tenant_id so SaveProfile's create-path passes the bootstrap precondition.
type editorFixture struct {
	pool       *pgxpool.Pool
	cs         *fakeCSClient
	store      *fakeConnStore
	operatorID uuid.UUID
}

func newEditorFixture(t *testing.T, ctx context.Context) editorFixture {
	t.Helper()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, nopLogger()))

	var operatorIDStr string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('editor-op@example.com', 'Editor Op', 'x', 'admin') RETURNING id`,
	).Scan(&operatorIDStr))

	return editorFixture{
		pool:       pool,
		cs:         &fakeCSClient{},
		store:      &fakeConnStore{tenantID: uuid.NewString(), appID: uuid.NewString()},
		operatorID: uuid.MustParse(operatorIDStr),
	}
}

// TestSaveProfile_Create — fresh slug, codec_js push, mapping insert, audit
// row 'profile_create'. CS Create called exactly once.
func TestSaveProfile_Create(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	ctx := context.Background()
	f := newEditorFixture(t, ctx)
	deps := Deps{Pool: f.pool, CSClient: f.cs, ConnStore: f.store, Log: nopLogger()}

	in := ProfileSaveInput{
		UserID:         f.operatorID,
		RequestID:      "req-prof-create",
		Slug:           "newvendor_x1",
		Name:           "NewVendor X1",
		Vendor:         "NewVendor",
		Family:         "X",
		Capabilities:   []string{"cumulative", "battery"},
		CounterModulus: 4294967296,
		Region:         "AS923_2",
		MACVersion:     "LORAWAN_1_0_3",
		CodecJS:        validCodec(),
		Mappings: []Mapping{
			{JSONPointer: "/cumulative_l", Target: "cumulative_value", DataType: "numeric", Position: 0},
			{JSONPointer: "/battery_pct", Target: "battery_pct", DataType: "int", Position: 1},
		},
	}

	id, err := SaveProfile(ctx, deps, in)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, id)

	// Profile row exists with codec_js + cs_profile_id + codec_js_synced_at.
	var (
		gotName    string
		gotCodec   string
		gotCSID    *string
		gotSynced  *string
	)
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT name, codec_js, cs_profile_id::text, codec_js_synced_at::text FROM device_profile WHERE id = $1`,
		id,
	).Scan(&gotName, &gotCodec, &gotCSID, &gotSynced))
	require.Equal(t, "NewVendor X1", gotName)
	require.Equal(t, validCodec(), gotCodec)
	require.NotNil(t, gotCSID, "cs_profile_id must be filled after CS push")
	require.NotNil(t, gotSynced, "codec_js_synced_at must be filled after CS push")

	// Mappings inserted.
	var mappingCount int
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT count(*) FROM device_profile_mapping WHERE device_profile_id = $1`,
		id,
	).Scan(&mappingCount))
	require.Equal(t, 2, mappingCount)

	// Audit row.
	var auditAction string
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT action FROM audit_log WHERE entity_id = $1 AND request_id = $2`,
		id, "req-prof-create",
	).Scan(&auditAction))
	require.Equal(t, "profile_create", auditAction)

	// CS Create called exactly once.
	require.Equal(t, int64(1), f.cs.CreateCalls())
	require.Equal(t, int64(0), f.cs.UpdateCalls())
}

// TestSaveProfile_Update — pre-create, then SaveProfile with the same id and
// changed fields → UPDATE path, mappings replaced, audit 'profile_update',
// CS Update called.
func TestSaveProfile_Update(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	ctx := context.Background()
	f := newEditorFixture(t, ctx)
	deps := Deps{Pool: f.pool, CSClient: f.cs, ConnStore: f.store, Log: nopLogger()}

	// Create first.
	create := ProfileSaveInput{
		UserID:         f.operatorID,
		RequestID:      "req-create-for-update",
		Slug:           "myvendor_v1",
		Name:           "MyVendor V1",
		Vendor:         "MyVendor",
		Capabilities:   []string{"cumulative"},
		CounterModulus: 100000,
		MACVersion:     "LORAWAN_1_0_3",
		CodecJS:        validCodec(),
		Mappings: []Mapping{
			{JSONPointer: "/old", Target: "cumulative_value", DataType: "numeric"},
		},
	}
	id, err := SaveProfile(ctx, deps, create)
	require.NoError(t, err)

	require.Equal(t, int64(1), f.cs.CreateCalls(), "first save uses CS Create")

	// Now update.
	update := create
	update.ID = id
	update.RequestID = "req-update"
	update.Name = "MyVendor V1 (renamed)"
	update.CodecJS = validCodec() + "\n// updated"
	update.Mappings = []Mapping{
		{JSONPointer: "/new1", Target: "cumulative_value", DataType: "numeric", Position: 0},
		{JSONPointer: "/new2", Target: "battery_pct", DataType: "int", Position: 1},
		{JSONPointer: "/new3", Target: "extra.foo", DataType: "text", Position: 2},
	}
	gotID, err := SaveProfile(ctx, deps, update)
	require.NoError(t, err)
	require.Equal(t, id, gotID)

	// CS Update called (not a second Create).
	require.Equal(t, int64(1), f.cs.CreateCalls(), "update path must NOT issue a second Create")
	require.Equal(t, int64(1), f.cs.UpdateCalls())

	// Mappings replaced (old row gone, 3 new rows present).
	var mappingCount int
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT count(*) FROM device_profile_mapping WHERE device_profile_id = $1`,
		id,
	).Scan(&mappingCount))
	require.Equal(t, 3, mappingCount)
	var stillHasOld int
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT count(*) FROM device_profile_mapping
		 WHERE device_profile_id = $1 AND json_pointer = '/old'`,
		id,
	).Scan(&stillHasOld))
	require.Equal(t, 0, stillHasOld, "old mapping row must be gone after replace")

	// Audit row 'profile_update' present.
	var auditCount int
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE entity_id = $1 AND action = 'profile_update'`,
		id,
	).Scan(&auditCount))
	require.Equal(t, 1, auditCount)

	// Profile name updated.
	var name string
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT name FROM device_profile WHERE id = $1`, id,
	).Scan(&name))
	require.Equal(t, "MyVendor V1 (renamed)", name)
}

// TestSaveProfile_RejectsCodecOverCap — codec_js > 256 KiB rejected with a
// "exceeds" message; tx never opened.
func TestSaveProfile_RejectsCodecOverCap(t *testing.T) {
	// Pure unit-test territory — no DB needed.
	deps := Deps{
		Pool:      nil, // unused on early-return path
		CSClient:  &fakeCSClient{},
		ConnStore: &fakeConnStore{tenantID: "anything"},
		Log:       nopLogger(),
	}
	huge := strings.Repeat("/* fluff */ ", (MaxCodecJSBytes/12)+1)

	// We need a Pool reference to even reach Begin; the cap check fires before
	// Pool is touched, but Pool != nil is asserted at the top of SaveProfile.
	// Stand up a real pool to exercise the early-return path with a realistic
	// dep graph.
	if testing.Short() {
		t.Skip("skipping: needs pool to satisfy nil-guard; -short skips")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, nopLogger()))
	deps.Pool = pool

	in := ProfileSaveInput{
		UserID:         uuid.New(),
		Slug:           "huge_codec",
		Name:           "Huge",
		Vendor:         "Big",
		Capabilities:   []string{"cumulative"},
		CounterModulus: 1,
		MACVersion:     "LORAWAN_1_0_3",
		CodecJS:        huge,
	}

	_, err := SaveProfile(ctx, deps, in)
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeds")
}

// TestSaveProfile_RejectsInvalidCapability — unknown capability token returns
// error before any DB write.
func TestSaveProfile_RejectsInvalidCapability(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	ctx := context.Background()
	f := newEditorFixture(t, ctx)
	deps := Deps{Pool: f.pool, CSClient: f.cs, ConnStore: f.store, Log: nopLogger()}

	in := ProfileSaveInput{
		UserID:         f.operatorID,
		Slug:           "bad_caps",
		Name:           "Bad",
		Vendor:         "Bad",
		Capabilities:   []string{"cumulative", "nonsense"},
		CounterModulus: 1,
		MACVersion:     "LORAWAN_1_0_3",
		CodecJS:        validCodec(),
	}

	_, err := SaveProfile(ctx, deps, in)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid capability")

	// No row inserted.
	var n int
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT count(*) FROM device_profile WHERE slug = 'bad_caps'`,
	).Scan(&n))
	require.Equal(t, 0, n)
}

// TestSaveProfile_RejectsCodecMissingDecodeUplink — codec_js with no
// decodeUplink contract returns error.
func TestSaveProfile_RejectsCodecMissingDecodeUplink(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	ctx := context.Background()
	f := newEditorFixture(t, ctx)
	deps := Deps{Pool: f.pool, CSClient: f.cs, ConnStore: f.store, Log: nopLogger()}

	in := ProfileSaveInput{
		UserID:         f.operatorID,
		Slug:           "no_decode",
		Name:           "X",
		Vendor:         "X",
		Capabilities:   []string{"cumulative"},
		CounterModulus: 1,
		MACVersion:     "LORAWAN_1_0_3",
		CodecJS:        "function unrelated() { return 1; }", // missing decodeUplink
	}

	_, err := SaveProfile(ctx, deps, in)
	require.Error(t, err)
	require.Contains(t, err.Error(), "decodeUplink")
}

// TestSaveProfile_RollsBackOnCSFailure — CS Create returns error → tx rolls
// back; no profile row, no mapping rows, no audit row.
func TestSaveProfile_RollsBackOnCSFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	ctx := context.Background()
	f := newEditorFixture(t, ctx)
	f.cs.createReturns = []csCreateResult{
		{id: "", err: errors.New("simulated CS Unavailable")},
	}
	deps := Deps{Pool: f.pool, CSClient: f.cs, ConnStore: f.store, Log: nopLogger()}

	in := ProfileSaveInput{
		UserID:         f.operatorID,
		RequestID:      "req-cs-fail",
		Slug:           "cs_fail_profile",
		Name:           "CSFail",
		Vendor:         "CSFail",
		Capabilities:   []string{"cumulative"},
		CounterModulus: 1,
		MACVersion:     "LORAWAN_1_0_3",
		CodecJS:        validCodec(),
		Mappings: []Mapping{
			{JSONPointer: "/x", Target: "cumulative_value", DataType: "numeric"},
		},
	}

	_, err := SaveProfile(ctx, deps, in)
	require.Error(t, err)
	require.Contains(t, err.Error(), "CS CreateDeviceProfile")

	// No profile row by slug.
	var n int
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT count(*) FROM device_profile WHERE slug = 'cs_fail_profile'`,
	).Scan(&n))
	require.Equal(t, 0, n, "tx must have rolled back on CS failure")

	// No audit row for the failed save.
	var auditCount int
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE request_id = 'req-cs-fail'`,
	).Scan(&auditCount))
	require.Equal(t, 0, auditCount)
}

// TestSaveProfile_EmptyCodec_SkipsCS — empty codec_js skips the CS push so
// the editor can save profiles with codec_js empty (operator might paste
// codec later). No CS calls; profile + mapping + audit still committed.
func TestSaveProfile_EmptyCodec_SkipsCS(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	ctx := context.Background()
	f := newEditorFixture(t, ctx)
	deps := Deps{Pool: f.pool, CSClient: f.cs, ConnStore: f.store, Log: nopLogger()}

	in := ProfileSaveInput{
		UserID:         f.operatorID,
		Slug:           "empty_codec",
		Name:           "Empty",
		Vendor:         "Empty",
		Capabilities:   []string{"cumulative"},
		CounterModulus: 1,
		MACVersion:     "LORAWAN_1_0_3",
		CodecJS:        "",
	}

	id, err := SaveProfile(ctx, deps, in)
	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, id)
	require.Equal(t, int64(0), f.cs.CreateCalls(), "empty codec must not push to CS")
	require.Equal(t, int64(0), f.cs.UpdateCalls())

	// codec_js_synced_at stays NULL for the empty-codec save.
	var syncedRaw *string
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT codec_js_synced_at::text FROM device_profile WHERE id = $1`, id,
	).Scan(&syncedRaw))
	require.Nil(t, syncedRaw)
}
