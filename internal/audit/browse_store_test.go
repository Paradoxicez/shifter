package audit_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shifter-io/shifter/internal/audit"
	"github.com/shifter-io/shifter/internal/auth"
	"github.com/shifter-io/shifter/internal/db"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// ─────────────────────────────────────────────────────────────────────────
// Test fixture
// ─────────────────────────────────────────────────────────────────────────

type auditFixture struct {
	pool     *pgxpool.Pool
	server   *httptest.Server
	client   *http.Client
	adminID  string
	viewerID string
}

func setupAuditFixture(t *testing.T) *auditFixture {
	t.Helper()
	if testing.Short() {
		t.Skip("requires postgres testcontainer")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))

	userStore := auth.NewStore(pool)
	hash, err := auth.Hash("Strong-Pass-1234!")
	require.NoError(t, err)
	adminID, err := userStore.InsertAdminUser(ctx, "admin@example.com", "Admin", hash)
	require.NoError(t, err)

	var viewerID string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('viewer@example.com', 'Viewer', $1, 'viewer') RETURNING id::text`,
		hash).Scan(&viewerID))

	sm := auth.NewSessionManager(pool, true /*dev*/, 8*time.Hour, 24*time.Hour)
	store := audit.NewStore(pool)
	deps := audit.Deps{
		Pool:  pool,
		Store: store,
		Log:   slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	}

	r := chi.NewRouter()
	// Session seed helper for tests — mirrors the user package test pattern.
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

	// Mount audit routes with RequireAction guards (mirrors what router.go will do).
	r.Route("/api/audit", func(rt chi.Router) {
		rt.With(auth.RequireAction(sm, auth.ActionAuditRead)).Get("/", audit.ListHandler(deps))
		rt.With(auth.RequireAction(sm, auth.ActionAuditRead)).Get("/count", audit.CountHandler(deps))
		rt.With(auth.RequireAction(sm, auth.ActionAuditRead)).Get("/distincts", audit.DistinctsHandler(deps))
	})

	srv := httptest.NewServer(sm.LoadAndSave(r))
	t.Cleanup(srv.Close)

	jar, _ := cookiejar.New(nil)
	cli := &http.Client{Jar: jar}

	return &auditFixture{
		pool:     pool,
		server:   srv,
		client:   cli,
		adminID:  adminID,
		viewerID: viewerID,
	}
}

func (f *auditFixture) seedRole(t *testing.T, role string) {
	t.Helper()
	res, err := f.client.Post(f.server.URL+"/test/seed/"+role, "", nil)
	require.NoError(t, err)
	res.Body.Close()
	require.Equal(t, http.StatusNoContent, res.StatusCode)
}

func (f *auditFixture) doGet(t *testing.T, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, f.server.URL+path, nil)
	require.NoError(t, err)
	req.Header.Set("X-Requested-With", "shifter")
	res, err := f.client.Do(req)
	require.NoError(t, err)
	return res
}

func (f *auditFixture) doPost(t *testing.T, path string, body io.Reader) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, f.server.URL+path, body)
	require.NoError(t, err)
	req.Header.Set("X-Requested-With", "shifter")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := f.client.Do(req)
	require.NoError(t, err)
	return res
}

// insertAuditRow inserts via direct pool query and returns the new row's UUID.
func insertAuditRow(t *testing.T, ctx context.Context, pool *pgxpool.Pool, action, entityType string, userID *uuid.UUID) uuid.UUID {
	t.Helper()
	entityID := uuid.New()
	var userIDStr interface{}
	if userID != nil {
		userIDStr = userID.String()
	}
	var rowIDStr string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO audit_log (user_id, action, entity_type, entity_id, request_id)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		userIDStr, action, entityType, entityID.String(), fmt.Sprintf("req-%s", entityID.String()[:8]),
	).Scan(&rowIDStr))
	id, err := uuid.Parse(rowIDStr)
	require.NoError(t, err)
	return id
}

// ─────────────────────────────────────────────────────────────────────────
// Browse store tests (direct store calls — no HTTP)
// ─────────────────────────────────────────────────────────────────────────

func TestCursorQuery_DefaultsTo7Days(t *testing.T) {
	if testing.Short() {
		t.Skip("requires postgres testcontainer")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))
	store := audit.NewStore(pool)

	// Insert one row within the last 7 days (implicitly via now()).
	_ = insertAuditRow(t, ctx, pool, audit.ActionCreate, audit.EntityTypeSite, nil)

	// Insert one row 30 days ago.
	_, err := pool.Exec(ctx,
		`INSERT INTO audit_log (user_id, action, entity_type, entity_id, time)
		 VALUES (NULL, $1, $2, $3, now() - INTERVAL '30 days')`,
		audit.ActionCreate, audit.EntityTypeSite, uuid.New().String(),
	)
	require.NoError(t, err)

	// No from/to in Filter → store defaults to last 7 days.
	result, err := store.ListCursor(ctx, audit.Filter{}, nil)
	require.NoError(t, err)
	// The 30-day-old row should NOT appear.
	for _, row := range result.Rows {
		assert.True(t, time.Since(row.Time) < 8*24*time.Hour, "row time should be within last 7 days, got %v", row.Time)
	}
}

func TestCursorQuery_RowComparisonStable(t *testing.T) {
	if testing.Short() {
		t.Skip("requires postgres testcontainer")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))
	store := audit.NewStore(pool)

	// Seed exactly 250 rows spread across time.
	seedFrom := time.Now().UTC().Add(-250 * time.Minute)
	for i := 0; i < 250; i++ {
		ts := seedFrom.Add(time.Duration(i) * time.Minute)
		_, err := pool.Exec(ctx,
			`INSERT INTO audit_log (user_id, action, entity_type, entity_id, time)
			 VALUES (NULL, $1, $2, $3, $4)`,
			audit.ActionCreate, audit.EntityTypeSite, uuid.New().String(), ts,
		)
		require.NoError(t, err)
	}

	// Wide date range covering all 250 rows.
	wideFrom := seedFrom.Add(-1 * time.Hour)
	wideTo := time.Now().Add(1 * time.Hour)
	filter := audit.Filter{From: &wideFrom, To: &wideTo}

	// Page 1.
	page1, err := store.ListCursor(ctx, filter, nil)
	require.NoError(t, err)
	assert.Len(t, page1.Rows, 100, "page 1 should have 100 rows")
	assert.NotEmpty(t, page1.NextCursor)

	// Concurrent insert between page1 and page2 (simulates real-world race).
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = pool.Exec(ctx,
			`INSERT INTO audit_log (user_id, action, entity_type, entity_id, time)
			 VALUES (NULL, $1, $2, $3, now())`,
			audit.ActionCreate, audit.EntityTypeSite, uuid.New().String(),
		)
	}()
	wg.Wait()

	cursor1 := audit.MustDecodeCursor(page1.NextCursor)

	// Page 2.
	page2, err := store.ListCursor(ctx, filter, cursor1)
	require.NoError(t, err)
	assert.Len(t, page2.Rows, 100, "page 2 should have 100 rows")
	assert.NotEmpty(t, page2.NextCursor)

	cursor2 := audit.MustDecodeCursor(page2.NextCursor)

	// Page 3.
	page3, err := store.ListCursor(ctx, filter, cursor2)
	require.NoError(t, err)
	assert.Len(t, page3.Rows, 50, "page 3 should have 50 rows")
	assert.Empty(t, page3.NextCursor, "last page has no next cursor")

	// No row dropped or duplicated across all three pages.
	seen := map[string]bool{}
	for _, p := range []*audit.ListResult{page1, page2, page3} {
		for _, row := range p.Rows {
			id := row.ID.String()
			assert.False(t, seen[id], "duplicate row: %s", id)
			seen[id] = true
		}
	}
	assert.Equal(t, 250, len(seen), "all 250 original rows present exactly once (concurrent insert was outside the window)")
}

func TestCursorQuery_FiltersByEntityType(t *testing.T) {
	if testing.Short() {
		t.Skip("requires postgres testcontainer")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))
	store := audit.NewStore(pool)

	for i := 0; i < 3; i++ {
		_ = insertAuditRow(t, ctx, pool, audit.ActionUserCreate, audit.EntityTypeUser, nil)
		_ = insertAuditRow(t, ctx, pool, audit.ActionCreate, audit.EntityTypeSite, nil)
	}

	filter := audit.Filter{EntityTypes: []string{audit.EntityTypeUser, audit.EntityTypeAlert}}
	result, err := store.ListCursor(ctx, filter, nil)
	require.NoError(t, err)
	for _, row := range result.Rows {
		assert.True(t, row.EntityType == audit.EntityTypeUser || row.EntityType == audit.EntityTypeAlert,
			"expected user or alert, got %s", row.EntityType)
	}
}

func TestCursorQuery_FiltersByActionArray(t *testing.T) {
	if testing.Short() {
		t.Skip("requires postgres testcontainer")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))
	store := audit.NewStore(pool)

	_ = insertAuditRow(t, ctx, pool, audit.ActionAuthLoginSuccess, audit.EntityTypeSession, nil)
	_ = insertAuditRow(t, ctx, pool, audit.ActionAuthLogout, audit.EntityTypeSession, nil)
	_ = insertAuditRow(t, ctx, pool, audit.ActionCreate, audit.EntityTypeSite, nil)

	filter := audit.Filter{Actions: []string{audit.ActionAuthLoginSuccess, audit.ActionAuthLogout}}
	result, err := store.ListCursor(ctx, filter, nil)
	require.NoError(t, err)
	require.NotEmpty(t, result.Rows)
	for _, row := range result.Rows {
		assert.True(t, row.Action == audit.ActionAuthLoginSuccess || row.Action == audit.ActionAuthLogout,
			"expected login or logout, got %s", row.Action)
	}
}

func TestCursorQuery_RequestIDLike(t *testing.T) {
	if testing.Short() {
		t.Skip("requires postgres testcontainer")
	}
	ctx := context.Background()
	pool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pool, slog.New(slog.NewTextHandler(os.Stderr, nil))))
	store := audit.NewStore(pool)

	reqID := "abc-unique-req"
	_, err := pool.Exec(ctx,
		`INSERT INTO audit_log (user_id, action, entity_type, entity_id, request_id)
		 VALUES (NULL, $1, $2, $3, $4)`,
		audit.ActionCreate, audit.EntityTypeSite, uuid.New().String(), reqID,
	)
	require.NoError(t, err)
	_ = insertAuditRow(t, ctx, pool, audit.ActionCreate, audit.EntityTypeSite, nil)

	like := "abc"
	filter := audit.Filter{RequestIDLike: &like}
	result, err := store.ListCursor(ctx, filter, nil)
	require.NoError(t, err)
	require.NotEmpty(t, result.Rows)
	for _, row := range result.Rows {
		assert.Contains(t, row.RequestID, "abc")
	}
}

// ─────────────────────────────────────────────────────────────────────────
// Handler tests (HTTP via test server)
// ─────────────────────────────────────────────────────────────────────────

func TestCursorQuery_RejectsInvalidEntityType(t *testing.T) {
	f := setupAuditFixture(t)
	f.seedRole(t, "admin")
	res := f.doGet(t, "/api/audit?entity_type=nonexistent")
	defer res.Body.Close()
	assert.Equal(t, http.StatusUnprocessableEntity, res.StatusCode)
}

func TestHandler_AdminOnly(t *testing.T) {
	f := setupAuditFixture(t)
	f.seedRole(t, "viewer")
	res := f.doGet(t, "/api/audit")
	defer res.Body.Close()
	assert.Equal(t, http.StatusForbidden, res.StatusCode)
}

func TestHandler_ReturnsCursorAndCount(t *testing.T) {
	f := setupAuditFixture(t)

	// Seed a row.
	_ = insertAuditRow(t, context.Background(), f.pool, audit.ActionCreate, audit.EntityTypeSite, nil)

	f.seedRole(t, "admin")
	res := f.doGet(t, "/api/audit")
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&resp))
	assert.Contains(t, resp, "rows")
	assert.Contains(t, resp, "next_cursor")
	assert.Contains(t, resp, "total")
}

func TestHandler_ExportButtonAware(t *testing.T) {
	f := setupAuditFixture(t)

	_ = insertAuditRow(t, context.Background(), f.pool, audit.ActionCreate, audit.EntityTypeSite, nil)

	f.seedRole(t, "admin")
	res := f.doGet(t, "/api/audit/count")
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&resp))
	assert.Contains(t, resp, "count")
}
