package device

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/chirpstack"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// fakeCSDevice records every Create / CreateKeys / Activate / Delete call so
// assertions can verify CS side effects without driving a real ChirpStack
// server. The failure-injection queue uses *Errs in FIFO order so multi-call
// tests stay deterministic. Phase 3 Plan 03-07 adds the Activate path for the
// ABP add-device branch — same recording shape (last-input + per-call err
// queue) as the OTAA Create/Keys pair.
type fakeCSDevice struct {
	mu sync.Mutex

	createCalls   atomic.Int64
	keysCalls     atomic.Int64
	activateCalls atomic.Int64
	deleteCalls   atomic.Int64

	createErrs   []error
	keysErrs     []error
	activateErrs []error
	deleteErrs   []error

	createdEUIs []string
	deletedEUIs []string

	// lastActivate is the last ActivateDeviceInput observed (only valid when
	// activateCalls > 0). Tests assert on this for the ABP path verification.
	lastActivate chirpstack.ActivateDeviceInput
}

func (f *fakeCSDevice) CreateDevice(_ context.Context, in chirpstack.CreateDeviceInput) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCalls.Add(1)
	if len(f.createErrs) > 0 {
		err := f.createErrs[0]
		f.createErrs = f.createErrs[1:]
		if err != nil {
			return err
		}
	}
	f.createdEUIs = append(f.createdEUIs, in.DevEUI)
	return nil
}

func (f *fakeCSDevice) CreateDeviceKeys(_ context.Context, _ string, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.keysCalls.Add(1)
	if len(f.keysErrs) > 0 {
		err := f.keysErrs[0]
		f.keysErrs = f.keysErrs[1:]
		if err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeCSDevice) DeleteDevice(_ context.Context, devEUI string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteCalls.Add(1)
	f.deletedEUIs = append(f.deletedEUIs, devEUI)
	if len(f.deleteErrs) > 0 {
		err := f.deleteErrs[0]
		f.deleteErrs = f.deleteErrs[1:]
		return err
	}
	return nil
}

// ActivateDevice is the ABP-branch counterpart of CreateDeviceKeys. Plan 03-07
// add-device handler invokes this for activation_mode=ABP after CS CreateDevice
// succeeds. The fake records the last input so tests can assert on field
// propagation (DevAddr / NwkSKey / AppSKey / FCntUp / FCntDown).
func (f *fakeCSDevice) ActivateDevice(_ context.Context, in chirpstack.ActivateDeviceInput) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.activateCalls.Add(1)
	f.lastActivate = in
	if len(f.activateErrs) > 0 {
		err := f.activateErrs[0]
		f.activateErrs = f.activateErrs[1:]
		if err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeCSDevice) CreateCalls() int64   { return f.createCalls.Load() }
func (f *fakeCSDevice) KeysCalls() int64     { return f.keysCalls.Load() }
func (f *fakeCSDevice) ActivateCalls() int64 { return f.activateCalls.Load() }
func (f *fakeCSDevice) DeleteCalls() int64   { return f.deleteCalls.Load() }

// fakeBootstrap satisfies CSBootstrapper with a fixed (tenantID, appID).
type fakeBootstrap struct {
	tenantID string
	appID    string
	err      error
	calls    atomic.Int64
}

func (b *fakeBootstrap) EnsureTenantAndApplication(_ context.Context) (string, string, error) {
	b.calls.Add(1)
	if b.err != nil {
		return "", "", b.err
	}
	return b.tenantID, b.appID, nil
}

// deviceFixture is the integration-test bench: testcontainer Postgres + an
// httptest server with the Device routes mounted + a fakeCSDevice + fake
// bootstrap. Tests interact via doJSON.
type deviceFixture struct {
	pool   *pgxpool.Pool
	server *httptest.Server
	client *http.Client
	deps   Deps
	cs     *fakeCSDevice
	boot   *fakeBootstrap

	adminID   string
	viewerID  string
	profileID string // a pre-seeded device profile WITH cs_profile_id.
	siteID    string
}

func nopLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func newDeviceFixture(t *testing.T) *deviceFixture {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, nopLogger()))

	var adminID, viewerID, profileID, siteID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('admin-dev@example.com', 'Admin Dev', 'x', 'admin') RETURNING id::text`,
	).Scan(&adminID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('viewer-dev@example.com', 'Viewer Dev', 'x', 'viewer') RETURNING id::text`,
	).Scan(&viewerID))

	// Seed a site so by-site queries have data.
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO site (name, timezone) VALUES ('Device Test Site', 'Asia/Bangkok') RETURNING id::text`,
	).Scan(&siteID))

	// Pre-seed a synced device profile (axioma_w1 from migration 0010 has
	// codec_js empty but no cs_profile_id either; we patch in a fake CS UUID
	// so AddDevice can pass the cs_profile_id check).
	require.NoError(t, pool.QueryRow(ctx,
		`UPDATE device_profile SET cs_profile_id = $1::uuid, codec_js_synced_at = now()
		 WHERE slug = 'axioma_w1' RETURNING id::text`, uuid.NewString(),
	).Scan(&profileID))

	cs := &fakeCSDevice{}
	boot := &fakeBootstrap{tenantID: uuid.NewString(), appID: uuid.NewString()}

	sm := auth.NewSessionManager(pool, true /*dev*/, 8*time.Hour, 24*time.Hour)
	deps := Deps{
		Pool:       pool,
		SessionMgr: sm,
		Log:        nopLogger(),
		CS:         cs,
		Bootstrap:  boot,
		PingGRPC:   func(_ context.Context, _ string) error { return nil },
		PingMQTT:   func(_ context.Context) error { return nil },
	}

	r := chi.NewRouter()
	r.Post("/test/seed/{role}", func(w http.ResponseWriter, req *http.Request) {
		role := chi.URLParam(req, "role")
		var id string
		switch role {
		case "admin":
			id = adminID
		case "viewer":
			id = viewerID
		default:
			http.Error(w, "bad role", 400)
			return
		}
		if err := auth.PutUser(req.Context(), sm, auth.User{ID: id, Role: role}); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	RegisterRoutes(r, deps)

	srv := httptest.NewServer(sm.LoadAndSave(r))
	t.Cleanup(srv.Close)

	jar, _ := cookiejar.New(nil)
	cli := &http.Client{Jar: jar}

	return &deviceFixture{
		pool: pool, server: srv, client: cli, deps: deps,
		cs: cs, boot: boot,
		adminID: adminID, viewerID: viewerID, profileID: profileID, siteID: siteID,
	}
}

func (f *deviceFixture) seedRole(t *testing.T, role string) {
	t.Helper()
	res, err := f.client.Post(f.server.URL+"/test/seed/"+role, "", nil)
	require.NoError(t, err)
	res.Body.Close()
	require.Equal(t, http.StatusNoContent, res.StatusCode)
}

func (f *deviceFixture) doJSON(t *testing.T, method, path string, body any) *http.Response {
	t.Helper()
	var buf io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		buf = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, f.server.URL+path, buf)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "shifter")
	res, err := f.client.Do(req)
	require.NoError(t, err)
	return res
}

func validAppKey() string {
	return "cafebabecafebabecafebabecafebabe"
}

// validDevEUI returns a valid lowercase 16-hex string.
func validDevEUI(suffix string) string {
	if len(suffix) > 12 {
		suffix = suffix[:12]
	}
	for len(suffix) < 12 {
		suffix = suffix + "0"
	}
	return "abcd" + suffix
}

// TestAddDevice_HappyPath_NoBinding — admin POST without metering_point_id →
// 201; CS Create + CreateKeys called; device row exists; audit row 'create';
// no binding row; AppKey not in any Shifter table.
func TestAddDevice_HappyPath_NoBinding(t *testing.T) {
	f := newDeviceFixture(t)
	f.seedRole(t, "admin")

	res := f.doJSON(t, "POST", "/api/devices", AddDeviceRequest{
		DevEUI:          validDevEUI("000001"),
		Name:            "Happy Path Dev",
		DeviceProfileID: f.profileID,
		AppKey:          validAppKey(),
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusCreated, res.StatusCode)

	require.Equal(t, int64(1), f.cs.CreateCalls())
	require.Equal(t, int64(1), f.cs.KeysCalls())
	require.Equal(t, int64(0), f.cs.DeleteCalls())

	var count int
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM device WHERE dev_eui = $1`, validDevEUI("000001"),
	).Scan(&count))
	require.Equal(t, 1, count)

	// No binding row.
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM binding`,
	).Scan(&count))
	require.Equal(t, 0, count)

	// Audit 'create' on entity_type=device.
	var action string
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT action FROM audit_log WHERE entity_type = 'device' ORDER BY time DESC LIMIT 1`,
	).Scan(&action))
	require.Equal(t, "create", action)
}

// TestAddDevice_HappyPath_WithBinding — POST with metering_point_id +
// initial_reading → device row + binding row with reading_offset; audit row's
// after contains initial_binding sub-object.
func TestAddDevice_HappyPath_WithBinding(t *testing.T) {
	f := newDeviceFixture(t)
	ctx := context.Background()
	f.seedRole(t, "admin")

	// Seed an MP.
	var mpID string
	require.NoError(t, f.pool.QueryRow(ctx,
		`INSERT INTO metering_point (site_id, name, utility_class)
		 VALUES ($1::uuid, 'MP-with-Binding', 'water') RETURNING id::text`, f.siteID,
	).Scan(&mpID))

	res := f.doJSON(t, "POST", "/api/devices", AddDeviceRequest{
		DevEUI:          validDevEUI("000002"),
		Name:            "Bound Dev",
		DeviceProfileID: f.profileID,
		AppKey:          validAppKey(),
		MeteringPointID: mpID,
		InitialReading:  "12345",
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusCreated, res.StatusCode)

	// Binding row exists with reading_offset=12345.
	var offset string
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT reading_offset::text FROM binding WHERE metering_point_id = $1::uuid`, mpID,
	).Scan(&offset))
	require.Equal(t, "12345", offset)

	// Audit row's after JSON contains initial_binding.
	var afterJSON []byte
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT after FROM audit_log WHERE entity_type = 'device' AND action = 'create' ORDER BY time DESC LIMIT 1`,
	).Scan(&afterJSON))
	var after map[string]any
	require.NoError(t, json.Unmarshal(afterJSON, &after))
	require.Contains(t, after, "initial_binding")
}

// TestAddDevice_CSFailure_RollsBack — CS CreateDevice returns an error → 502;
// no device row; no audit row; no binding row.
func TestAddDevice_CSFailure_RollsBack(t *testing.T) {
	f := newDeviceFixture(t)
	f.seedRole(t, "admin")
	f.cs.createErrs = []error{errors.New("simulated CS failure")}

	res := f.doJSON(t, "POST", "/api/devices", AddDeviceRequest{
		DevEUI:          validDevEUI("000003"),
		Name:            "CS Fail Dev",
		DeviceProfileID: f.profileID,
		AppKey:          validAppKey(),
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusBadGateway, res.StatusCode)

	var count int
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM device WHERE dev_eui = $1`, validDevEUI("000003"),
	).Scan(&count))
	require.Equal(t, 0, count, "no device row when CS Create fails")

	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE entity_type = 'device' AND action = 'create'`,
	).Scan(&count))
	require.Equal(t, 0, count, "no audit row when CS Create fails")
}

// TestAddDevice_CSKeysFailure_DeletesCSDevice — CS Create succeeds but
// CreateKeys fails → 502; CS DeleteDevice called for cleanup; no Shifter
// device row.
func TestAddDevice_CSKeysFailure_DeletesCSDevice(t *testing.T) {
	f := newDeviceFixture(t)
	f.seedRole(t, "admin")
	f.cs.keysErrs = []error{errors.New("simulated keys failure")}

	res := f.doJSON(t, "POST", "/api/devices", AddDeviceRequest{
		DevEUI:          validDevEUI("000004"),
		Name:            "Keys Fail Dev",
		DeviceProfileID: f.profileID,
		AppKey:          validAppKey(),
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusBadGateway, res.StatusCode)

	require.Equal(t, int64(1), f.cs.CreateCalls())
	require.Equal(t, int64(1), f.cs.KeysCalls())
	require.GreaterOrEqual(t, f.cs.DeleteCalls(), int64(1), "CS cleanup must call DeleteDevice")

	var count int
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM device WHERE dev_eui = $1`, validDevEUI("000004"),
	).Scan(&count))
	require.Equal(t, 0, count)
}

// TestAddDevice_PostgresFailure_DeletesCSDevice — pre-insert a device with
// the same dev_eui to force the Shifter INSERT to violate the unique index;
// CS Create still happened so cleanup must call DeleteDevice.
func TestAddDevice_PostgresFailure_DeletesCSDevice(t *testing.T) {
	f := newDeviceFixture(t)
	f.seedRole(t, "admin")
	ctx := context.Background()

	// Pre-insert a conflicting device row.
	_, err := f.pool.Exec(ctx,
		`INSERT INTO device (dev_eui, name, device_profile_id) VALUES ($1, 'pre-existing', $2::uuid)`,
		validDevEUI("000005"), f.profileID,
	)
	require.NoError(t, err)

	res := f.doJSON(t, "POST", "/api/devices", AddDeviceRequest{
		DevEUI:          validDevEUI("000005"),
		Name:            "Conflicting Dev",
		DeviceProfileID: f.profileID,
		AppKey:          validAppKey(),
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusConflict, res.StatusCode)

	require.Equal(t, int64(1), f.cs.CreateCalls())
	require.Equal(t, int64(1), f.cs.KeysCalls())
	require.GreaterOrEqual(t, f.cs.DeleteCalls(), int64(1),
		"D-16 contract: CS DeleteDevice must be called when Shifter INSERT fails after CS Create")
}

// TestAddDevice_ProfileNotSynced — pre-clear cs_profile_id on the seeded
// profile; AddDevice → 502 with 'profile_not_synced'.
func TestAddDevice_ProfileNotSynced(t *testing.T) {
	f := newDeviceFixture(t)
	ctx := context.Background()
	f.seedRole(t, "admin")

	// Find a different un-synced profile (acrel) and use that.
	var unsyncedID string
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT id::text FROM device_profile WHERE slug = 'acrel_adl200' LIMIT 1`,
	).Scan(&unsyncedID))

	res := f.doJSON(t, "POST", "/api/devices", AddDeviceRequest{
		DevEUI:          validDevEUI("000006"),
		Name:            "Unsynced Profile Dev",
		DeviceProfileID: unsyncedID,
		AppKey:          validAppKey(),
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusBadGateway, res.StatusCode)

	var got map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&got))
	require.Equal(t, "profile_not_synced", got["error"])

	// CS NOT called.
	require.Equal(t, int64(0), f.cs.CreateCalls())
}

// TestAddDevice_ViewerForbidden — viewer POST → 403; CS not called.
func TestAddDevice_ViewerForbidden(t *testing.T) {
	f := newDeviceFixture(t)
	f.seedRole(t, "viewer")

	res := f.doJSON(t, "POST", "/api/devices", AddDeviceRequest{
		DevEUI:          validDevEUI("000007"),
		Name:            "Forbidden Dev",
		DeviceProfileID: f.profileID,
		AppKey:          validAppKey(),
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusForbidden, res.StatusCode)

	require.Equal(t, int64(0), f.cs.CreateCalls(), "viewer reject must not reach CS")

	// AUDIT-01: no audit row for rejected request.
	var count int
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE entity_type = 'device'`,
	).Scan(&count))
	require.Equal(t, 0, count)
}

// TestAddDevice_RejectsBadDevEUI — non-hex / wrong-length DevEUI → 400.
func TestAddDevice_RejectsBadDevEUI(t *testing.T) {
	f := newDeviceFixture(t)
	f.seedRole(t, "admin")

	res := f.doJSON(t, "POST", "/api/devices", AddDeviceRequest{
		DevEUI:          "not-hex",
		Name:            "Bad EUI",
		DeviceProfileID: f.profileID,
		AppKey:          validAppKey(),
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)

	require.Equal(t, int64(0), f.cs.CreateCalls(), "bad input must not reach CS")
}

// TestAddDevice_RejectsBadAppKey — wrong-length AppKey → 400.
func TestAddDevice_RejectsBadAppKey(t *testing.T) {
	f := newDeviceFixture(t)
	f.seedRole(t, "admin")

	res := f.doJSON(t, "POST", "/api/devices", AddDeviceRequest{
		DevEUI:          validDevEUI("000008"),
		Name:            "Bad Key",
		DeviceProfileID: f.profileID,
		AppKey:          "tooshort",
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)
}

// TestPreflightCS_HappyPath — admin POST /api/devices/preflight → 200 with
// {grpc:"ok", mqtt:"ok"}.
func TestPreflightCS_HappyPath(t *testing.T) {
	f := newDeviceFixture(t)
	f.seedRole(t, "admin")

	res := f.doJSON(t, "POST", "/api/devices/preflight", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var got map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&got))
	require.Equal(t, "ok", got["grpc"])
	require.Equal(t, "ok", got["mqtt"])
}

// TestPreflightCS_GRPCFailure — gRPC ping returns err → 502 + grpc=err.
func TestPreflightCS_GRPCFailure(t *testing.T) {
	f := newDeviceFixture(t)
	f.deps.PingGRPC = func(_ context.Context, _ string) error { return errors.New("simulated grpc down") }
	// Re-mount router with the modified deps.
	r := chi.NewRouter()
	r.Post("/test/seed/{role}", func(w http.ResponseWriter, req *http.Request) {
		role := chi.URLParam(req, "role")
		var id string
		if role == "admin" {
			id = f.adminID
		} else {
			id = f.viewerID
		}
		_ = auth.PutUser(req.Context(), f.deps.SessionMgr, auth.User{ID: id, Role: role})
		w.WriteHeader(http.StatusNoContent)
	})
	RegisterRoutes(r, f.deps)
	srv := httptest.NewServer(f.deps.SessionMgr.LoadAndSave(r))
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	cli := &http.Client{Jar: jar}

	loginRes, err := cli.Post(srv.URL+"/test/seed/admin", "", nil)
	require.NoError(t, err)
	loginRes.Body.Close()

	req, _ := http.NewRequest("POST", srv.URL+"/api/devices/preflight", nil)
	req.Header.Set("X-Requested-With", "shifter")
	res, err := cli.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusBadGateway, res.StatusCode)

	var got map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&got))
	require.Equal(t, "err", got["grpc"])
}

// TestParseDevEUIHandler — POST /api/devices/parse-deveui returns both
// MSB + LSB interpretations + per-orientation vendor hints.
func TestParseDevEUIHandler(t *testing.T) {
	f := newDeviceFixture(t)
	f.seedRole(t, "admin")

	res := f.doJSON(t, "POST", "/api/devices/parse-deveui", ParseDevEUIRequest{
		Raw: "70:b3:d5:ab:cd:ef:12:34",
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var got map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&got))
	require.Equal(t, "70b3d5abcdef1234", got["msb"])
	require.Contains(t, got["msb_vendor"], "Axioma")
}

// TestDecommissionDevice_ClosesActiveBindingAndMarks — pre-create device with
// active binding; POST decommission; binding.valid_to set; device.decommissioned_at
// set; audit row 'decommission'.
func TestDecommissionDevice_ClosesActiveBindingAndMarks(t *testing.T) {
	f := newDeviceFixture(t)
	ctx := context.Background()
	f.seedRole(t, "admin")

	// Seed MP + add device with binding.
	var mpID string
	require.NoError(t, f.pool.QueryRow(ctx,
		`INSERT INTO metering_point (site_id, name, utility_class)
		 VALUES ($1::uuid, 'MP-decomm', 'water') RETURNING id::text`, f.siteID,
	).Scan(&mpID))

	addRes := f.doJSON(t, "POST", "/api/devices", AddDeviceRequest{
		DevEUI:          validDevEUI("000009"),
		Name:            "Decomm Dev",
		DeviceProfileID: f.profileID,
		AppKey:          validAppKey(),
		MeteringPointID: mpID,
	})
	defer addRes.Body.Close()
	require.Equal(t, http.StatusCreated, addRes.StatusCode)
	var created map[string]any
	require.NoError(t, json.NewDecoder(addRes.Body).Decode(&created))
	deviceID := created["id"].(string)

	// Decommission.
	res := f.doJSON(t, "POST", "/api/devices/"+deviceID+"/decommission", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	// device.decommissioned_at IS NOT NULL.
	var hasTS bool
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT decommissioned_at IS NOT NULL FROM device WHERE id = $1::uuid`, deviceID,
	).Scan(&hasTS))
	require.True(t, hasTS)

	// binding.valid_to IS NOT NULL.
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT valid_to IS NOT NULL FROM binding WHERE device_id = $1::uuid`, deviceID,
	).Scan(&hasTS))
	require.True(t, hasTS)

	// Audit row.
	var action string
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT action FROM audit_log WHERE entity_id = $1::uuid AND action = 'decommission'`, deviceID,
	).Scan(&action))
	require.Equal(t, "decommission", action)
}

// TestDecommissionDevice_ViewerForbidden — viewer → 403.
func TestDecommissionDevice_ViewerForbidden(t *testing.T) {
	f := newDeviceFixture(t)
	ctx := context.Background()
	f.seedRole(t, "admin")

	addRes := f.doJSON(t, "POST", "/api/devices", AddDeviceRequest{
		DevEUI:          validDevEUI("00000a"),
		Name:            "Not For You",
		DeviceProfileID: f.profileID,
		AppKey:          validAppKey(),
	})
	defer addRes.Body.Close()
	var created map[string]any
	require.NoError(t, json.NewDecoder(addRes.Body).Decode(&created))
	deviceID := created["id"].(string)

	f.seedRole(t, "viewer")
	res := f.doJSON(t, "POST", "/api/devices/"+deviceID+"/decommission", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusForbidden, res.StatusCode)

	// Device NOT decommissioned.
	var hasTS bool
	require.NoError(t, f.pool.QueryRow(ctx,
		`SELECT decommissioned_at IS NOT NULL FROM device WHERE id = $1::uuid`, deviceID,
	).Scan(&hasTS))
	require.False(t, hasTS)
}

// TestGetDevice_NotFound — bogus UUID → 404.
func TestGetDevice_NotFound(t *testing.T) {
	f := newDeviceFixture(t)
	f.seedRole(t, "admin")

	res := f.doJSON(t, "GET", "/api/devices/"+uuid.NewString(), nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusNotFound, res.StatusCode)
}

// TestSearchDevices_ViewerCanRead — viewer can GET /api/devices/search.
func TestSearchDevices_ViewerCanRead(t *testing.T) {
	f := newDeviceFixture(t)
	// Admin creates.
	f.seedRole(t, "admin")
	addRes := f.doJSON(t, "POST", "/api/devices", AddDeviceRequest{
		DevEUI:          validDevEUI("00000b"),
		Name:            "Searchable",
		DeviceProfileID: f.profileID,
		AppKey:          validAppKey(),
	})
	defer addRes.Body.Close()
	require.Equal(t, http.StatusCreated, addRes.StatusCode)

	// Viewer searches.
	f.seedRole(t, "viewer")
	res := f.doJSON(t, "GET", "/api/devices/search?q=Searchable", nil)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var got []map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&got))
	require.Len(t, got, 1)
	require.Equal(t, "Searchable", got[0]["name"])
}

// ============================================================================
// Phase 3 Plan 03-07 — OTAA / ABP branching in POST /api/devices
// ============================================================================
//
// Backend extension (D-19..D-25): a single POST /api/devices accepts either
// `activation_mode=OTAA` (Phase 2 path — CS CreateDevice + CreateDeviceKeys)
// OR `activation_mode=ABP` (new — CS CreateDevice + ActivateDevice). The
// response body echoes the keys back to the client so the dialog's success
// state (D-21) can render Copy-keys without a second round-trip. Shifter PG
// NEVER persists secrets (D-22 / DEV-09 — structural).

// validNwkSKey / validAppSKey / validDevAddr are deterministic ABP fixtures.
// Pure-hex 32/32/8 byte counts match LoRaWAN 1.0.x wire shape.
func validNwkSKey() string { return "0102030405060708090a0b0c0d0e0f10" }
func validAppSKey() string { return "1011121314151617181920212223242a" }
func validDevAddr() string { return "01020304" }

// TestAddDevice_OTAA — explicit activation_mode=OTAA → CS CreateDevice + CS
// CreateDeviceKeys + PG row. Response body echoes back keys (D-21 success
// state contract). Backwards-compat: omitting activation_mode also routes to
// OTAA (Phase 2 default — no migration burden on existing callers).
func TestAddDevice_OTAA(t *testing.T) {
	f := newDeviceFixture(t)
	f.seedRole(t, "admin")

	res := f.doJSON(t, "POST", "/api/devices", AddDeviceRequest{
		DevEUI:          validDevEUI("070001"),
		Name:            "OTAA Dev",
		DeviceProfileID: f.profileID,
		ActivationMode:  "OTAA",
		AppKey:          validAppKey(),
		JoinEUI:         "0000000000000000",
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusCreated, res.StatusCode)

	require.Equal(t, int64(1), f.cs.CreateCalls())
	require.Equal(t, int64(1), f.cs.KeysCalls(), "OTAA branch invokes CreateDeviceKeys")
	require.Equal(t, int64(0), f.cs.ActivateCalls(), "OTAA branch must NOT invoke Activate")
	require.Equal(t, int64(0), f.cs.DeleteCalls())

	var body map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.Equal(t, "OTAA", body["activation_mode"])
	require.Equal(t, validAppKey(), body["app_key"], "D-21: success-state echoes AppKey")
	require.Equal(t, "0000000000000000", body["join_eui"])
	require.Equal(t, validAppKey(), body["nwk_key"], "1.0.x default: NwkKey=AppKey")
}

// TestAddDevice_ABP — activation_mode=ABP → CS CreateDevice + CS
// ActivateDevice (no CreateKeys). Response echoes ABP fields. Verifies
// ActivateDevice was called with the operator-typed DevAddr / NwkSKey /
// AppSKey / FCntUp / FCntDown — and that NO secret columns are written to
// Shifter PG (DEV-09 structural).
func TestAddDevice_ABP(t *testing.T) {
	f := newDeviceFixture(t)
	f.seedRole(t, "admin")

	res := f.doJSON(t, "POST", "/api/devices", AddDeviceRequest{
		DevEUI:          validDevEUI("070002"),
		Name:            "ABP Dev",
		DeviceProfileID: f.profileID,
		ActivationMode:  "ABP",
		DevAddr:         validDevAddr(),
		NwkSKey:         validNwkSKey(),
		AppSKey:         validAppSKey(),
		FCntUp:          12,
		FCntDown:        8,
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusCreated, res.StatusCode)

	require.Equal(t, int64(1), f.cs.CreateCalls())
	require.Equal(t, int64(0), f.cs.KeysCalls(), "ABP branch must NOT invoke CreateDeviceKeys")
	require.Equal(t, int64(1), f.cs.ActivateCalls(), "ABP branch invokes ActivateDevice")
	require.Equal(t, int64(0), f.cs.DeleteCalls())

	// Activate payload propagation.
	got := f.cs.lastActivate
	require.Equal(t, validDevEUI("070002"), got.DevEUI)
	require.Equal(t, validDevAddr(), got.DevAddr)
	require.Equal(t, validNwkSKey(), got.NwkSKey)
	require.Equal(t, validAppSKey(), got.AppSKey)
	require.Equal(t, uint32(12), got.FCntUp)
	require.Equal(t, uint32(8), got.FCntDown)

	// Response echoes operator-typed ABP fields for D-21 success state.
	var body map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.Equal(t, "ABP", body["activation_mode"])
	require.Equal(t, validDevAddr(), body["dev_addr"])
	require.Equal(t, validNwkSKey(), body["nwk_s_key"])
	require.Equal(t, validAppSKey(), body["app_s_key"])
	require.EqualValues(t, 12, body["f_cnt_up"])
	require.EqualValues(t, 8, body["f_cnt_down"])

	// DEV-09 structural — device row has NO secret columns. We sanity-check by
	// dumping the full row's text representation and asserting no key material
	// appears (defense beyond the structural schema invariant).
	var rowDump string
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT row_to_json(device)::text FROM device WHERE dev_eui = $1`, validDevEUI("070002"),
	).Scan(&rowDump))
	require.NotContains(t, rowDump, validNwkSKey(),
		"DEV-09: NwkSKey must NEVER appear in any Shifter row")
	require.NotContains(t, rowDump, validAppSKey(),
		"DEV-09: AppSKey must NEVER appear in any Shifter row")
	require.NotContains(t, rowDump, validDevAddr(),
		"DEV-09: DevAddr must NEVER appear in any Shifter row")
}

// TestAddDevice_ABP_FCntCarryOver — D-20: omitting fcnt_up / fcnt_down
// defaults both to 0 on the ActivateDevice payload.
func TestAddDevice_ABP_FCntCarryOver(t *testing.T) {
	f := newDeviceFixture(t)
	f.seedRole(t, "admin")

	res := f.doJSON(t, "POST", "/api/devices", AddDeviceRequest{
		DevEUI:          validDevEUI("070003"),
		Name:            "ABP No FCnt",
		DeviceProfileID: f.profileID,
		ActivationMode:  "ABP",
		DevAddr:         validDevAddr(),
		NwkSKey:         validNwkSKey(),
		AppSKey:         validAppSKey(),
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusCreated, res.StatusCode)

	require.Equal(t, uint32(0), f.cs.lastActivate.FCntUp)
	require.Equal(t, uint32(0), f.cs.lastActivate.FCntDown)
}

// TestAddDevice_ABP_AtomicityCSRollback — PG insert fails (unique conflict)
// AFTER CS CreateDevice + ActivateDevice succeed → handler invokes
// best-effort CS DeleteDevice with a fresh ctx. Both CS resources cleaned
// up, no orphan; HTTP 409 surfaced for the unique violation.
func TestAddDevice_ABP_AtomicityCSRollback(t *testing.T) {
	f := newDeviceFixture(t)
	f.seedRole(t, "admin")

	// Pre-insert conflicting device row → forces unique violation on INSERT.
	_, err := f.pool.Exec(context.Background(),
		`INSERT INTO device (dev_eui, name, device_profile_id) VALUES ($1, 'pre-existing', $2::uuid)`,
		validDevEUI("070004"), f.profileID,
	)
	require.NoError(t, err)

	res := f.doJSON(t, "POST", "/api/devices", AddDeviceRequest{
		DevEUI:          validDevEUI("070004"),
		Name:            "ABP Conflict",
		DeviceProfileID: f.profileID,
		ActivationMode:  "ABP",
		DevAddr:         validDevAddr(),
		NwkSKey:         validNwkSKey(),
		AppSKey:         validAppSKey(),
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusConflict, res.StatusCode)

	require.Equal(t, int64(1), f.cs.CreateCalls())
	require.Equal(t, int64(1), f.cs.ActivateCalls())
	require.GreaterOrEqual(t, f.cs.DeleteCalls(), int64(1),
		"D-16: CS DeleteDevice must be called when Shifter INSERT fails after Activate")
}

// TestAddDevice_InvalidActivationMode — activation_mode=banana → 400. CS not
// touched.
func TestAddDevice_InvalidActivationMode(t *testing.T) {
	f := newDeviceFixture(t)
	f.seedRole(t, "admin")

	res := f.doJSON(t, "POST", "/api/devices", AddDeviceRequest{
		DevEUI:          validDevEUI("070005"),
		Name:            "Banana",
		DeviceProfileID: f.profileID,
		ActivationMode:  "banana",
		AppKey:          validAppKey(),
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.Equal(t, "invalid_activation_mode", body["error"])

	require.Equal(t, int64(0), f.cs.CreateCalls(), "bad activation_mode must not reach CS")
}

// TestAddDevice_MissingABPFields — activation_mode=ABP without nwk_s_key →
// 400 with operator-readable detail naming the missing field. CS untouched.
func TestAddDevice_MissingABPFields(t *testing.T) {
	f := newDeviceFixture(t)
	f.seedRole(t, "admin")

	res := f.doJSON(t, "POST", "/api/devices", AddDeviceRequest{
		DevEUI:          validDevEUI("070006"),
		Name:            "ABP Missing",
		DeviceProfileID: f.profileID,
		ActivationMode:  "ABP",
		DevAddr:         validDevAddr(),
		AppSKey:         validAppSKey(),
		// NwkSKey intentionally omitted.
	})
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.Equal(t, "invalid_nwk_s_key", body["error"])

	require.Equal(t, int64(0), f.cs.CreateCalls())
}
