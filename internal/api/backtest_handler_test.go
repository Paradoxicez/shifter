package api

// Tests for the POST /api/alerts/backtest handler.
// Plan 07-10 Task 2: BacktestHandler + RBAC guard.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/alert"
	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// Suppress unused import warnings from the compiler.
var _ = context.Background
var _ = fmt.Sprintf

// backtestHandlerSetup creates a real Postgres testcontainer, runs migrations,
// seeds an admin or viewer user, and returns the httptest server + client.
func backtestHandlerSetup(t *testing.T, suffix string, role string) (srv *httptest.Server, cli *http.Client) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, noopLog()))

	// Seed install_identity (required by some middleware).
	_, err := pool.Exec(ctx,
		`INSERT INTO install_identity (id, display_name, timezone, units)
		 VALUES (1, 'BacktestHandlerTest', 'UTC', 'metric') ON CONFLICT (id) DO NOTHING`)
	require.NoError(t, err)

	var userID string
	require.NoError(t, pool.QueryRow(ctx,
		fmt.Sprintf(`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('%s-backtest-%s@ex.com', 'U', 'x', '%s') RETURNING id::text`,
			role, suffix, role),
	).Scan(&userID))

	sm := auth.NewSessionManager(pool, true, 8*time.Hour, 24*time.Hour)
	deps := BacktestDeps{Pool: pool}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	// Session seed endpoint for tests.
	r.Post("/test/seed", func(w http.ResponseWriter, req *http.Request) {
		if err := auth.PutUser(req.Context(), sm, auth.User{ID: userID, Role: role}); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	// Mount backtest endpoint with RBAC.
	r.Group(func(rt chi.Router) {
		rt.Use(auth.RequireAction(sm, auth.ActionAlertRuleCreate))
		rt.Post("/api/alerts/backtest", BacktestHandler(deps))
	})

	server := httptest.NewServer(sm.LoadAndSave(r))
	t.Cleanup(server.Close)

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	res, err2 := client.Post(server.URL+"/test/seed", "", nil)
	require.NoError(t, err2)
	res.Body.Close()

	return server, client
}

// TestBacktestHandler_P95_HappyPath: POST with valid p95 body returns 200 + BacktestResult.
func TestBacktestHandler_P95_HappyPath(t *testing.T) {
	srv, cli := backtestHandlerSetup(t, "p95-happy", "admin")

	mpID := uuid.New()
	body, _ := json.Marshal(map[string]any{
		"rule_kind": "anomaly_p95",
		"mp_id":     mpID.String(),
		"days":      30,
	})
	res, err := cli.Post(srv.URL+"/api/alerts/backtest", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusOK, res.StatusCode)

	var result alert.BacktestResult
	require.NoError(t, json.NewDecoder(res.Body).Decode(&result))
	require.Len(t, result.DailyFires, 30, "must return 30 daily buckets")
	require.Equal(t, 0, result.FiresCount, "empty MP has zero fires")
}

// TestBacktestHandler_InvalidKind: POST with unknown rule_kind returns 400.
func TestBacktestHandler_InvalidKind(t *testing.T) {
	srv, cli := backtestHandlerSetup(t, "invalid-kind", "admin")

	body, _ := json.Marshal(map[string]any{
		"rule_kind": "bogus_kind",
		"mp_id":     uuid.New().String(),
		"days":      30,
	})
	res, err := cli.Post(srv.URL+"/api/alerts/backtest", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusBadRequest, res.StatusCode)
}

// TestBacktestHandler_ViewerForbidden: viewer role cannot call the endpoint (403).
func TestBacktestHandler_ViewerForbidden(t *testing.T) {
	srv, cli := backtestHandlerSetup(t, "viewer-forbidden", "viewer")

	body, _ := json.Marshal(map[string]any{
		"rule_kind": "anomaly_p95",
		"mp_id":     uuid.New().String(),
		"days":      30,
	})
	res, err := cli.Post(srv.URL+"/api/alerts/backtest", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusForbidden, res.StatusCode)
}

// TestBacktestHandler_DefaultsTo30Days: POST without days field defaults to 30.
func TestBacktestHandler_DefaultsTo30Days(t *testing.T) {
	srv, cli := backtestHandlerSetup(t, "default-days", "admin")

	body, _ := json.Marshal(map[string]any{
		"rule_kind": "anomaly_iqr",
		"mp_id":     uuid.New().String(),
		// no "days" field — should default to 30
	})
	res, err := cli.Post(srv.URL+"/api/alerts/backtest", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusOK, res.StatusCode)
	var result alert.BacktestResult
	require.NoError(t, json.NewDecoder(res.Body).Decode(&result))
	require.Len(t, result.DailyFires, 30, "default days=30 must produce 30 buckets")
}
