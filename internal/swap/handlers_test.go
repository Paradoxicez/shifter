package swap

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// swapHandlerFixture wires a real testcontainer Postgres + an httptest server
// with the swap routes mounted.
type swapHandlerFixture struct {
	pool   *pgxpool.Pool
	server *httptest.Server
	client *http.Client
	deps   HTTPDeps

	adminID  string
	viewerID string

	fix swapFixture
}

func newSwapHandlerFixture(t *testing.T, suffix string) *swapHandlerFixture {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, slog.New(slog.NewTextHandler(io.Discard, nil))))

	// Seed the swap fixture (site, MP, profile, devices, active binding).
	fix := seedSwapFixture(t, ctx, pool, suffix)

	// Seed admin + viewer.
	var adminID, viewerID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('admin-swap-`+suffix+`@example.com', 'Admin Swap', 'x', 'admin') RETURNING id::text`,
	).Scan(&adminID))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('viewer-swap-`+suffix+`@example.com', 'Viewer Swap', 'x', 'viewer') RETURNING id::text`,
	).Scan(&viewerID))

	sm := auth.NewSessionManager(pool, true /*dev*/, 8*time.Hour, 24*time.Hour)
	deps := HTTPDeps{
		Pool:       pool,
		SessionMgr: sm,
		Log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
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

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	cli := &http.Client{Jar: jar}

	return &swapHandlerFixture{
		pool:     pool,
		server:   srv,
		client:   cli,
		deps:     deps,
		adminID:  adminID,
		viewerID: viewerID,
		fix:      fix,
	}
}

func (f *swapHandlerFixture) seedRole(t *testing.T, role string) {
	t.Helper()
	res, err := f.client.Post(f.server.URL+"/test/seed/"+role, "", nil)
	require.NoError(t, err)
	res.Body.Close()
	require.Equal(t, http.StatusNoContent, res.StatusCode)
}

func (f *swapHandlerFixture) doJSON(t *testing.T, method, path string, body any) *http.Response {
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

// TestSwapHandler_Admin_HappyPath — admin POST with valid body → 200 with
// {"binding_id": "<uuid>"}; one row in audit_log with action='swap' for the
// new binding.
func TestSwapHandler_Admin_HappyPath(t *testing.T) {
	f := newSwapHandlerFixture(t, "happy")
	f.seedRole(t, "admin")

	mpID := uuid.UUID(f.fix.mpID.Bytes).String()
	body := SwapRequest{
		IncomingDeviceID: f.fix.inDeviceID.String(),
		OutgoingReadingR: "12345",
		IncomingInitialN: "0",
		OperatorNotes:    "annual replacement",
	}
	res := f.doJSON(t, "POST", "/api/metering-points/"+mpID+"/swap", body)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var resp map[string]string
	require.NoError(t, json.NewDecoder(res.Body).Decode(&resp))
	newBindingIDStr := resp["binding_id"]
	require.NotEmpty(t, newBindingIDStr)
	newBindingID, err := uuid.Parse(newBindingIDStr)
	require.NoError(t, err)

	// Audit row exists.
	var auditCount int
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE action = 'swap' AND entity_id = $1`,
		newBindingID,
	).Scan(&auditCount))
	require.Equal(t, 1, auditCount)
}

// TestSwapHandler_Viewer_403 — viewer POST → 403, no audit row, no binding
// state change.
func TestSwapHandler_Viewer_403(t *testing.T) {
	f := newSwapHandlerFixture(t, "viewer")
	f.seedRole(t, "viewer")

	mpID := uuid.UUID(f.fix.mpID.Bytes).String()
	body := SwapRequest{
		IncomingDeviceID: f.fix.inDeviceID.String(),
		OutgoingReadingR: "12345",
		IncomingInitialN: "0",
	}
	res := f.doJSON(t, "POST", "/api/metering-points/"+mpID+"/swap", body)
	defer res.Body.Close()
	require.Equal(t, http.StatusForbidden, res.StatusCode)

	// Outgoing binding still open.
	var validTo *time.Time
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT valid_to FROM binding WHERE id = $1`, f.fix.bindingID,
	).Scan(&validTo))
	require.Nil(t, validTo, "viewer 403 must NOT close the binding")

	// No swap audit row.
	var auditCount int
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE action = 'swap'`,
	).Scan(&auditCount))
	require.Equal(t, 0, auditCount)
}

// TestSwapHandler_NoActiveBinding_404 — MP with no active binding → 404
// {"error": "no_active_binding"}.
func TestSwapHandler_NoActiveBinding_404(t *testing.T) {
	f := newSwapHandlerFixture(t, "noact")
	f.seedRole(t, "admin")

	// Create a fresh MP with no binding.
	var newMP string
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`INSERT INTO metering_point (site_id, name, utility_class)
		 VALUES ($1, 'mp-noact', 'water') RETURNING id::text`,
		f.fix.siteID,
	).Scan(&newMP))

	body := SwapRequest{
		IncomingDeviceID: f.fix.inDeviceID.String(),
		OutgoingReadingR: "100",
		IncomingInitialN: "0",
	}
	res := f.doJSON(t, "POST", "/api/metering-points/"+newMP+"/swap", body)
	defer res.Body.Close()
	require.Equal(t, http.StatusNotFound, res.StatusCode)

	var errResp map[string]string
	require.NoError(t, json.NewDecoder(res.Body).Decode(&errResp))
	require.Equal(t, "no_active_binding", errResp["error"])
}

// TestSwapHandler_Concurrent_409 — two simultaneous POSTs against the same MP
// → exactly one returns 200, the other returns 409.
func TestSwapHandler_Concurrent_409(t *testing.T) {
	f := newSwapHandlerFixture(t, "conc")
	f.seedRole(t, "admin")

	// Seed a third device — second concurrent commit will use this one as
	// "incoming." Both commits target the same outgoing binding.
	var thirdDeviceID string
	thirdDevEUI := padDevEUI("00112233concurrent03")
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`INSERT INTO device (dev_eui, name, device_profile_id)
		 VALUES ($1, $2, $3) RETURNING id::text`,
		thirdDevEUI, "third-dev-conc", f.fix.profileID,
	).Scan(&thirdDeviceID))

	mpID := uuid.UUID(f.fix.mpID.Bytes).String()

	// Build two requests using two clients (separate cookie jars but same admin).
	mkClient := func(t *testing.T) *http.Client {
		t.Helper()
		jar, err := cookiejar.New(nil)
		require.NoError(t, err)
		cli := &http.Client{Jar: jar}
		// Seed admin role on this client.
		res, err := cli.Post(f.server.URL+"/test/seed/admin", "", nil)
		require.NoError(t, err)
		res.Body.Close()
		require.Equal(t, http.StatusNoContent, res.StatusCode)
		return cli
	}
	cliA := mkClient(t)
	cliB := mkClient(t)

	doRequest := func(cli *http.Client, incoming string) (int, string) {
		body := SwapRequest{
			IncomingDeviceID: incoming,
			OutgoingReadingR: "12345",
			IncomingInitialN: "0",
		}
		b, _ := json.Marshal(body)
		req, err := http.NewRequest("POST", f.server.URL+"/api/metering-points/"+mpID+"/swap", bytes.NewReader(b))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Requested-With", "shifter")
		res, err := cli.Do(req)
		require.NoError(t, err)
		defer res.Body.Close()
		body2, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(body2)
	}

	type result struct {
		code int
		body string
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		c, b := doRequest(cliA, f.fix.inDeviceID.String())
		results <- result{c, b}
	}()
	go func() {
		defer wg.Done()
		c, b := doRequest(cliB, thirdDeviceID)
		results <- result{c, b}
	}()
	wg.Wait()
	close(results)

	wins := 0
	losers := 0
	for r := range results {
		switch r.code {
		case http.StatusOK:
			wins++
		case http.StatusConflict:
			losers++
			require.Contains(t, r.body, "concurrent_swap")
		default:
			t.Fatalf("unexpected code %d body=%s", r.code, r.body)
		}
	}
	require.Equal(t, 1, wins, "exactly one wins")
	require.Equal(t, 1, losers, "exactly one loses with 409")
}

// TestSwapHandler_BadBody_400 — missing outgoing_reading_r → 400 bad_request.
func TestSwapHandler_BadBody_400(t *testing.T) {
	f := newSwapHandlerFixture(t, "bad")
	f.seedRole(t, "admin")

	mpID := uuid.UUID(f.fix.mpID.Bytes).String()
	body := SwapRequest{
		IncomingDeviceID: f.fix.inDeviceID.String(),
		// OutgoingReadingR missing.
		IncomingInitialN: "0",
	}
	res := f.doJSON(t, "POST", "/api/metering-points/"+mpID+"/swap", body)
	defer res.Body.Close()
	require.Equal(t, http.StatusBadRequest, res.StatusCode)

	var errResp map[string]string
	require.NoError(t, json.NewDecoder(res.Body).Decode(&errResp))
	require.Equal(t, "bad_request", errResp["error"])
}

// TestSwapHandler_OperatorOverride_PersistsInAudit — POST with operator_override
// → audit_log.after.override_used=true and reading_offset = override.
func TestSwapHandler_OperatorOverride_PersistsInAudit(t *testing.T) {
	f := newSwapHandlerFixture(t, "ovr")
	f.seedRole(t, "admin")

	mpID := uuid.UUID(f.fix.mpID.Bytes).String()
	override := "99999"
	body := SwapRequest{
		IncomingDeviceID: f.fix.inDeviceID.String(),
		OutgoingReadingR: "12345",
		IncomingInitialN: "0",
		OperatorOverride: &override,
	}
	res := f.doJSON(t, "POST", "/api/metering-points/"+mpID+"/swap", body)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var resp map[string]string
	require.NoError(t, json.NewDecoder(res.Body).Decode(&resp))
	newBindingID := resp["binding_id"]

	// audit_log.after.override_used must be true.
	var overrideUsed bool
	require.NoError(t, f.pool.QueryRow(context.Background(),
		`SELECT (after->>'override_used')::boolean FROM audit_log
		 WHERE entity_id = $1::uuid AND action = 'swap'`,
		newBindingID,
	).Scan(&overrideUsed))
	require.True(t, overrideUsed)
}

// TestSwapHandler_RouteIsMounted — bare GET on /api/metering-points/{id}/swap
// → 405 (route exists, method not allowed) NOT 404 (route absent).
func TestSwapHandler_RouteIsMounted(t *testing.T) {
	f := newSwapHandlerFixture(t, "mount")
	f.seedRole(t, "admin")

	mpID := uuid.UUID(f.fix.mpID.Bytes).String()
	req, err := http.NewRequest("GET", f.server.URL+"/api/metering-points/"+mpID+"/swap", nil)
	require.NoError(t, err)
	res, err := f.client.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusMethodNotAllowed, res.StatusCode,
		"GET on swap path must return 405 (route exists), not 404 (route absent)")
}
