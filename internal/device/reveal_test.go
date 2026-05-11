package device

// Phase 3 Plan 03-06 / Task 3 — POST /api/devices/{eui}/keys (reveal secrets).
//
// The 9 behaviors covered (mirroring 03-06 PLAN behaviors):
//
//  1. AdminOTAA — admin POST returns OTAA mode + AppKey + NwkKey from CS
//     GetDeviceKeys; audit row written with NO key material.
//  2. AdminABP — CS GetDeviceKeys returns NotFound; GetDeviceActivation
//     returns session keys + dev_addr + fcnts; mode='ABP'; audit clean.
//  3. Viewer403 — viewer role rejected at RequireAction middleware; no audit
//     row written (handler never reached).
//  4. NotFound — device row absent in Shifter PG → 404; CS never called.
//  5. CSUnreachable502 — CS GetDeviceKeys returns a non-NotFound error →
//     502 cs_get_keys_failed.
//  6. NoCredentials409 — both CS calls return NotFound → 409
//     no_credentials_in_cs.
//  7. AuditNoSecretMaterial — read the audit JSONB and assert it contains
//     none of the AppKey / NwkKey / AppSKey / NwkSKey hex strings.
//  8. InvalidDevEUI — /api/devices/notvalid/keys → 400; no DB/CS call.
//  9. MustBePOST — GET /api/devices/{eui}/keys → 405 (chi default).
//
// Defense-in-depth on item 7: a static grep test in this file asserts the
// writeAuditReveal call site in reveal.go does NOT mention any of the four
// key field names (AppKey/NwkKey/AppSKey/NwkSKey) in its `after` map literal.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"sync"
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

// fakeRevealerCS extends fakeCSDevice with GetDeviceKeys + GetDeviceActivation
// so it satisfies BOTH CSDeviceClient (handlers.go) AND CSDeviceRevealer
// (reveal.go). Reveal tests substitute this instead of fakeCSDevice so the
// reveal handler can route through the .(CSDeviceRevealer) type assertion.
type fakeRevealerCS struct {
	mu sync.Mutex

	getKeysErr  error
	getKeysResp *chirpstack.DeviceKeys

	getActErr  error
	getActResp *chirpstack.DeviceActivation
}

// Satisfy CSDeviceClient — never invoked by the reveal flow but the
// embedding deps.CS field is typed as CSDeviceClient so the methods must
// be present.
func (f *fakeRevealerCS) CreateDevice(_ context.Context, _ chirpstack.CreateDeviceInput) error {
	return errors.New("fakeRevealerCS.CreateDevice not implemented")
}
func (f *fakeRevealerCS) CreateDeviceKeys(_ context.Context, _ string, _ string) error {
	return errors.New("fakeRevealerCS.CreateDeviceKeys not implemented")
}
func (f *fakeRevealerCS) DeleteDevice(_ context.Context, _ string) error { return nil }

// CSDeviceRevealer surface.
func (f *fakeRevealerCS) GetDeviceKeys(_ context.Context, _ string) (*chirpstack.DeviceKeys, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getKeysErr != nil {
		return nil, f.getKeysErr
	}
	return f.getKeysResp, nil
}
func (f *fakeRevealerCS) GetDeviceActivation(_ context.Context, _ string) (*chirpstack.DeviceActivation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getActErr != nil {
		return nil, f.getActErr
	}
	return f.getActResp, nil
}

// revealFixture wires the reveal endpoint with a fakeRevealerCS in place of
// the standard fakeCSDevice. Tests interact via doJSON.
type revealFixture struct {
	pool    *pgxpool.Pool
	server  *httptest.Server
	client  *http.Client
	cs      *fakeRevealerCS
	adminID string
}

func newRevealFixture(t *testing.T) *revealFixture {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, slog.New(slog.NewTextHandler(io.Discard, nil))))

	var adminID, viewerID, profileID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('admin-reveal@example.com', 'Admin Reveal', 'x', 'admin') RETURNING id::text`,
	).Scan(&adminID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('viewer-reveal@example.com', 'Viewer Reveal', 'x', 'viewer') RETURNING id::text`,
	).Scan(&viewerID))
	require.NoError(t, pool.QueryRow(ctx,
		`UPDATE device_profile SET cs_profile_id = $1::uuid, codec_js_synced_at = now()
		 WHERE slug = 'axioma_w1' RETURNING id::text`, uuid.NewString(),
	).Scan(&profileID))

	cs := &fakeRevealerCS{}

	sm := auth.NewSessionManager(pool, true, 8*time.Hour, 24*time.Hour)
	deps := Deps{
		Pool:       pool,
		SessionMgr: sm,
		Log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		CS:         cs,
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

	// Seed one canonical device that all OTAA/ABP/audit tests reuse.
	_, err := pool.Exec(ctx,
		`INSERT INTO device (dev_eui, name, device_profile_id, join_eui)
		 VALUES ($1, $2, $3::uuid, $4)`,
		"0102030405060708", "reveal-target", profileID, "0000000000000000",
	)
	require.NoError(t, err)

	return &revealFixture{
		pool: pool, server: srv, client: cli, cs: cs, adminID: adminID,
	}
}

func (f *revealFixture) seedRole(t *testing.T, role string) {
	t.Helper()
	res, err := f.client.Post(f.server.URL+"/test/seed/"+role, "", nil)
	require.NoError(t, err)
	res.Body.Close()
	require.Equal(t, http.StatusNoContent, res.StatusCode)
}

func (f *revealFixture) do(t *testing.T, method, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, f.server.URL+path, nil)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	res, err := f.client.Do(req)
	require.NoError(t, err)
	return res
}

// TestRevealSecrets_AdminOTAA — admin POST → 200 with OTAA payload + audit row.
func TestRevealSecrets_AdminOTAA(t *testing.T) {
	f := newRevealFixture(t)
	f.seedRole(t, "admin")

	f.cs.getKeysResp = &chirpstack.DeviceKeys{
		DevEUI: "0102030405060708",
		AppKey: "cafebabecafebabecafebabecafebabe",
		NwkKey: "cafebabecafebabecafebabecafebabe",
	}

	res := f.do(t, "POST", "/api/devices/0102030405060708/keys")
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	// T-3-53 cache headers present on success.
	require.Contains(t, res.Header.Get("Cache-Control"), "no-store")
	require.Equal(t, "no-cache", res.Header.Get("Pragma"))

	var body map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.Equal(t, "OTAA", body["activation_mode"])
	require.Equal(t, "0102030405060708", body["dev_eui"])
	require.Equal(t, "cafebabecafebabecafebabecafebabe", body["app_key"])
	require.Equal(t, "cafebabecafebabecafebabecafebabe", body["nwk_key"])

	// Audit row written with action='device.reveal_secrets'.
	var action string
	var afterJSON []byte
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT action, after FROM audit_log WHERE action = 'device.reveal_secrets' ORDER BY time DESC LIMIT 1`,
	).Scan(&action, &afterJSON))
	require.Equal(t, "device.reveal_secrets", action)
	var after map[string]any
	require.NoError(t, json.Unmarshal(afterJSON, &after))
	require.Equal(t, "OTAA", after["activation_mode"])
	require.Equal(t, "0102030405060708", after["dev_eui"])
	// Audit row carries ONLY {activation_mode, dev_eui} — no third key.
	require.Len(t, after, 2, "audit `after` map must have exactly 2 fields")
}

// TestRevealSecrets_AdminABP — OTAA GetKeys NotFound, ABP path used.
func TestRevealSecrets_AdminABP(t *testing.T) {
	f := newRevealFixture(t)
	f.seedRole(t, "admin")

	f.cs.getKeysErr = chirpstack.ErrNotFound
	f.cs.getActResp = &chirpstack.DeviceActivation{
		DevEUI:    "0102030405060708",
		DevAddr:   "01020304",
		NwkSKey:   "11111111111111111111111111111111",
		AppSKey:   "22222222222222222222222222222222",
		FCntUp:    42,
		NFCntDown: 11,
		AFCntDown: 0,
	}

	res := f.do(t, "POST", "/api/devices/0102030405060708/keys")
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.Equal(t, "ABP", body["activation_mode"])
	require.Equal(t, "01020304", body["dev_addr"])
	require.Equal(t, "11111111111111111111111111111111", body["nwk_s_key"])
	require.Equal(t, "22222222222222222222222222222222", body["app_s_key"])

	// Audit row.
	var afterJSON []byte
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT after FROM audit_log WHERE action = 'device.reveal_secrets' ORDER BY time DESC LIMIT 1`,
	).Scan(&afterJSON))
	var after map[string]any
	require.NoError(t, json.Unmarshal(afterJSON, &after))
	require.Equal(t, "ABP", after["activation_mode"])
}

// TestRevealSecrets_Viewer403 — viewer denied by RequireAction; no audit row.
func TestRevealSecrets_Viewer403(t *testing.T) {
	f := newRevealFixture(t)
	f.seedRole(t, "viewer")

	res := f.do(t, "POST", "/api/devices/0102030405060708/keys")
	defer res.Body.Close()
	require.Equal(t, http.StatusForbidden, res.StatusCode)

	// No audit row.
	var count int
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE action = 'device.reveal_secrets'`,
	).Scan(&count))
	require.Equal(t, 0, count)
}

// TestRevealSecrets_NotFound — unknown dev_eui (no row in Shifter PG) → 404.
func TestRevealSecrets_NotFound(t *testing.T) {
	f := newRevealFixture(t)
	f.seedRole(t, "admin")

	res := f.do(t, "POST", "/api/devices/aaaaaaaaaaaaaaaa/keys")
	defer res.Body.Close()
	require.Equal(t, http.StatusNotFound, res.StatusCode)

	// CS NEVER called for an unknown PG row.
	var count int
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE action = 'device.reveal_secrets'`,
	).Scan(&count))
	require.Equal(t, 0, count)
}

// TestRevealSecrets_CSUnreachable502 — non-NotFound CS error → 502.
func TestRevealSecrets_CSUnreachable502(t *testing.T) {
	f := newRevealFixture(t)
	f.seedRole(t, "admin")

	f.cs.getKeysErr = errors.New("simulated gRPC dial failure")

	res := f.do(t, "POST", "/api/devices/0102030405060708/keys")
	defer res.Body.Close()
	require.Equal(t, http.StatusBadGateway, res.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.Equal(t, "cs_get_keys_failed", body["error"])
}

// TestRevealSecrets_NoCredentials409 — OTAA NotFound + ABP NotFound → 409.
func TestRevealSecrets_NoCredentials409(t *testing.T) {
	f := newRevealFixture(t)
	f.seedRole(t, "admin")

	f.cs.getKeysErr = chirpstack.ErrNotFound
	f.cs.getActErr = chirpstack.ErrNotFound

	res := f.do(t, "POST", "/api/devices/0102030405060708/keys")
	defer res.Body.Close()
	require.Equal(t, http.StatusConflict, res.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.Equal(t, "no_credentials_in_cs", body["error"])
}

// TestRevealSecrets_AuditNoSecretMaterial — for both OTAA and ABP success
// paths, parse the audit_log.after JSONB and assert NO key hex appears.
// T-3-51 defense-in-depth: even if a future code change accidentally adds
// a key into the audit map literal, this test would fail.
func TestRevealSecrets_AuditNoSecretMaterial(t *testing.T) {
	f := newRevealFixture(t)
	f.seedRole(t, "admin")

	appKey := "deadbeefdeadbeefdeadbeefdeadbeef"
	nwkKey := "feedfacefeedfacefeedfacefeedface"
	f.cs.getKeysResp = &chirpstack.DeviceKeys{
		DevEUI: "0102030405060708",
		AppKey: appKey,
		NwkKey: nwkKey,
	}

	res := f.do(t, "POST", "/api/devices/0102030405060708/keys")
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	// Now switch to ABP mode + run a second reveal so BOTH key flavours appear.
	f.cs.getKeysErr = chirpstack.ErrNotFound
	f.cs.getKeysResp = nil
	nwkSKey := "11112222333344445555666677778888"
	appSKey := "aaaabbbbccccddddeeeeffff00001111"
	f.cs.getActResp = &chirpstack.DeviceActivation{
		DevEUI:  "0102030405060708",
		DevAddr: "01020304",
		NwkSKey: nwkSKey,
		AppSKey: appSKey,
	}
	res2 := f.do(t, "POST", "/api/devices/0102030405060708/keys")
	defer res2.Body.Close()
	require.Equal(t, http.StatusOK, res2.StatusCode)

	// Concatenate every audit row's `after` JSONB and assert NONE of the
	// four secrets appear as substring. This is the runtime side of the
	// DEV-09 / T-3-51 guarantee.
	rows, err := f.pool.Query(context.Background(),
		`SELECT after::text FROM audit_log WHERE action = 'device.reveal_secrets'`)
	require.NoError(t, err)
	defer rows.Close()
	var combined string
	for rows.Next() {
		var s string
		require.NoError(t, rows.Scan(&s))
		combined += s + "\n"
	}
	for label, sec := range map[string]string{
		"AppKey":  appKey,
		"NwkKey":  nwkKey,
		"NwkSKey": nwkSKey,
		"AppSKey": appSKey,
	} {
		require.NotContainsf(t, combined, sec,
			"audit_log.after MUST NOT contain %s (%s) — found in %q",
			label, sec, combined)
	}
}

// TestRevealSecrets_InvalidDevEUI — malformed :eui → 400; no DB/CS call.
func TestRevealSecrets_InvalidDevEUI(t *testing.T) {
	f := newRevealFixture(t)
	f.seedRole(t, "admin")

	res := f.do(t, "POST", "/api/devices/notvalid/keys")
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)

	var body map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	require.Equal(t, "invalid_dev_eui", body["error"])
}

// TestRevealSecrets_MustBePOST — GET → 405 (chi default for unmapped verb).
func TestRevealSecrets_MustBePOST(t *testing.T) {
	f := newRevealFixture(t)
	f.seedRole(t, "admin")

	res := f.do(t, "GET", "/api/devices/0102030405060708/keys")
	defer res.Body.Close()
	require.Equal(t, http.StatusMethodNotAllowed, res.StatusCode)
}

// TestRevealSecrets_AuditWriteSiteCleanliness — static grep at the
// writeAuditReveal call site: the function body MUST NOT mention any of
// AppKey / NwkKey / AppSKey / NwkSKey. T-3-51 / D-28 defense in depth
// against accidental future regression. Runs in -short mode (pure file
// read; no testcontainer).
func TestRevealSecrets_AuditWriteSiteCleanliness(t *testing.T) {
	src, err := os.ReadFile("reveal.go")
	require.NoError(t, err)

	// Extract the body of writeAuditReveal (best-effort: between "func writeAuditReveal" and the next top-level "func ").
	startIdx := strings.Index(string(src), "func writeAuditReveal")
	require.Greater(t, startIdx, 0, "reveal.go must contain writeAuditReveal")
	rest := string(src[startIdx:])
	endIdx := strings.Index(rest[1:], "\nfunc ")
	if endIdx == -1 {
		endIdx = len(rest)
	} else {
		endIdx++ // account for the +1 above
	}
	body := rest[:endIdx]

	// Strip block + line comments so D-28 / T-3-51 doc references don't
	// trip the check. (Tests should fail on REAL code mentions, not on
	// the rule's own description.)
	body = regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(body, "")
	body = regexp.MustCompile(`//[^\n]*`).ReplaceAllString(body, "")

	for _, forbidden := range []string{"AppKey", "NwkKey", "AppSKey", "NwkSKey"} {
		require.NotContainsf(t, body, forbidden,
			"writeAuditReveal MUST NOT reference %s anywhere in its code body (DEV-09 / T-3-51)",
			forbidden)
	}
}
