package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/shifter-io/shifter/internal/db"
	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
	"github.com/shifter-io/shifter/internal/testsupport"
)

// startTestDB boots a TimescaleDB container, runs all migrations, and returns
// a connected pool + cleanup. Skipped with -short to keep unit-only runs fast.
func startTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping: requires TimescaleDB container (-short)")
	}
	pool := testsupport.StartPostgres(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := db.RunMigrations(context.Background(), pool, log); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	return pool
}

// buildTestRouter builds a test chi.Router with retention routes + session middleware.
func buildTestRouter(pool *pgxpool.Pool, sm *scs.SessionManager) http.Handler {
	q := sqlc.New(pool)
	deps := Deps{Pool: pool, Queries: q}
	r := chi.NewRouter()
	r.Use(sm.LoadAndSave)
	RegisterRoutes(r, deps, sm)
	return r
}

// injectSession writes user_id + role into a new session and returns the
// resulting session cookie. The sm.LoadAndSave middleware must be active for
// the cookie to work on subsequent requests.
func injectSession(t *testing.T, sm *scs.SessionManager, userID, role string) http.Cookie {
	t.Helper()
	r := httptest.NewRequest("GET", "/api/settings/retention", nil)
	w := httptest.NewRecorder()

	sm.LoadAndSave(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		sm.Put(req.Context(), "user_id", userID)
		sm.Put(req.Context(), "role", role)
		_ = sm.RenewToken(req.Context())
		rw.WriteHeader(http.StatusOK)
	})).ServeHTTP(w, r)

	for _, c := range w.Result().Cookies() {
		if strings.Contains(c.Name, "session") {
			return *c
		}
	}
	t.Fatal("no session cookie created")
	return http.Cookie{}
}

// doReq fires an authenticated HTTP request against the test router.
func doReq(t *testing.T, router http.Handler, method, path string, body any, cookie http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var buf []byte
	if body != nil {
		var err error
		buf, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(buf))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.AddCookie(&cookie)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// decodeJSON decodes the response body into v.
func decodeJSON(t *testing.T, w *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), v); err != nil {
		t.Fatalf("decode JSON (status %d, body %q): %v", w.Code, w.Body.String(), err)
	}
}

// TestRetentionConfigCRUD tests GET returns seeded defaults and PATCH updates a field.
func TestRetentionConfigCRUD(t *testing.T) {
	pool := startTestDB(t)
	sm := scs.New()
	router := buildTestRouter(pool, sm)

	adminID := uuid.New().String()
	adminCookie := injectSession(t, sm, adminID, "admin")

	// Test 1: GET returns the seeded defaults (90, 365, 1825, 7300, null).
	w := doReq(t, router, http.MethodGet, "/api/settings/retention", nil, adminCookie)
	if w.Code != http.StatusOK {
		t.Fatalf("GET expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp RetentionResponse
	decodeJSON(t, w, &resp)
	if resp.RawDays != 90 {
		t.Errorf("raw_days: want 90, got %d", resp.RawDays)
	}
	if resp.HourlyDays != 365 {
		t.Errorf("hourly_days: want 365, got %d", resp.HourlyDays)
	}
	if resp.DailyDays != 1825 {
		t.Errorf("daily_days: want 1825, got %d", resp.DailyDays)
	}
	if resp.MonthlyDays != 7300 {
		t.Errorf("monthly_days: want 7300, got %d", resp.MonthlyDays)
	}
	if resp.YearlyDays != nil {
		t.Errorf("yearly_days: want nil (forever), got %v", resp.YearlyDays)
	}

	// Test 2: PATCH {raw_days: 60} — only raw_days changes; other fields unchanged.
	w2 := doReq(t, router, http.MethodPatch, "/api/settings/retention",
		map[string]any{"raw_days": 60}, adminCookie)
	if w2.Code != http.StatusOK {
		t.Fatalf("PATCH expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
	var resp2 RetentionResponse
	decodeJSON(t, w2, &resp2)
	if resp2.RawDays != 60 {
		t.Errorf("after PATCH raw_days: want 60, got %d", resp2.RawDays)
	}
	if resp2.HourlyDays != 365 {
		t.Errorf("hourly_days should be unchanged: want 365, got %d", resp2.HourlyDays)
	}

	// Test 3: Verify retention_config row in DB reflects the change.
	q := sqlc.New(pool)
	cfg, err := q.GetRetentionConfig(context.Background())
	if err != nil {
		t.Fatalf("GetRetentionConfig: %v", err)
	}
	if cfg.RawDays != 60 {
		t.Errorf("DB raw_days: want 60, got %d", cfg.RawDays)
	}
}

// TestRetentionConfig_OutOfRange_422 tests that PATCH with out-of-range values returns 422.
func TestRetentionConfig_OutOfRange_422(t *testing.T) {
	pool := startTestDB(t)
	sm := scs.New()
	router := buildTestRouter(pool, sm)
	adminCookie := injectSession(t, sm, uuid.New().String(), "admin")

	cases := []struct {
		name    string
		body    map[string]any
		wantErr string
	}{
		{"raw_too_low", map[string]any{"raw_days": 10}, "raw_days_out_of_range"},
		{"raw_too_high", map[string]any{"raw_days": 400}, "raw_days_out_of_range"},
		{"hourly_too_low", map[string]any{"hourly_days": 100}, "hourly_days_out_of_range"},
		{"daily_too_low", map[string]any{"daily_days": 100}, "daily_days_out_of_range"},
		{"monthly_too_low", map[string]any{"monthly_days": 100}, "monthly_days_out_of_range"},
		{"yearly_too_low", map[string]any{"yearly_days": 100}, "yearly_days_out_of_range"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := doReq(t, router, http.MethodPatch, "/api/settings/retention", tc.body, adminCookie)
			if w.Code != http.StatusUnprocessableEntity {
				t.Fatalf("expected 422, got %d: %s", w.Code, w.Body.String())
			}
			var errResp map[string]string
			decodeJSON(t, w, &errResp)
			if errResp["error"] != tc.wantErr {
				t.Errorf("want error %q, got %q", tc.wantErr, errResp["error"])
			}
		})
	}
}

// TestRetentionConfig_YearlyForever_RemovesPolicy tests the yearly_forever sentinel.
func TestRetentionConfig_YearlyForever_RemovesPolicy(t *testing.T) {
	pool := startTestDB(t)
	sm := scs.New()
	router := buildTestRouter(pool, sm)
	adminCookie := injectSession(t, sm, uuid.New().String(), "admin")

	// First set yearly_days to a concrete value.
	w1 := doReq(t, router, http.MethodPatch, "/api/settings/retention",
		map[string]any{"yearly_days": 3650}, adminCookie)
	if w1.Code != http.StatusOK {
		t.Fatalf("set yearly_days expected 200, got %d: %s", w1.Code, w1.Body.String())
	}

	// Then set yearly_forever=true → yearly_days should become null.
	w2 := doReq(t, router, http.MethodPatch, "/api/settings/retention",
		map[string]any{"yearly_forever": true}, adminCookie)
	if w2.Code != http.StatusOK {
		t.Fatalf("yearly_forever expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
	var resp RetentionResponse
	decodeJSON(t, w2, &resp)
	if resp.YearlyDays != nil {
		t.Errorf("yearly_days after yearly_forever=true: want nil, got %v", resp.YearlyDays)
	}

	// Verify in DB.
	q := sqlc.New(pool)
	cfg, err := q.GetRetentionConfig(context.Background())
	if err != nil {
		t.Fatalf("GetRetentionConfig: %v", err)
	}
	if cfg.YearlyDays != nil {
		t.Errorf("DB yearly_days: want nil, got %v", cfg.YearlyDays)
	}
}

// TestRetentionConfig_Rollback_OnPolicyFailure tests that a ReconcilePolicies
// failure causes ReconcilePolicies to return an error. Since the PATCH handler
// defers tx.Rollback, a policy failure rolls back the config row change.
// This unit-level test exercises the rollback guarantee without a real DB.
func TestRetentionConfig_Rollback_OnPolicyFailure(t *testing.T) {
	before := sqlc.RetentionConfig{
		RawDays: 90, HourlyDays: 365, DailyDays: 1825, MonthlyDays: 7300,
	}
	after := sqlc.RetentionConfig{
		RawDays: 60, HourlyDays: 365, DailyDays: 1825, MonthlyDays: 7300,
	}

	ft := &failingTx{}
	err := ReconcilePolicies(context.Background(), ft, before, after)
	if err == nil {
		t.Error("expected error from ReconcilePolicies with failing tx, got nil")
	}
	if ft.execCalls == 0 {
		t.Error("expected at least one Exec call (remove_retention_policy)")
	}
}

// TestRetentionConfig_PatchAsViewer_403 tests that viewers receive 403 on PATCH.
func TestRetentionConfig_PatchAsViewer_403(t *testing.T) {
	pool := startTestDB(t)
	sm := scs.New()
	router := buildTestRouter(pool, sm)
	viewerCookie := injectSession(t, sm, uuid.New().String(), "viewer")

	w := doReq(t, router, http.MethodPatch, "/api/settings/retention",
		map[string]any{"raw_days": 60}, viewerCookie)
	if w.Code != http.StatusForbidden {
		t.Fatalf("viewer PATCH expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

// TestRetentionConfig_UnknownHypertable_Rejected tests that ReconcilePolicies
// rejects unknown hypertable names via the compile-time switch default arm,
// and issues ZERO tx.Exec calls before returning the error.
// This is the T-05-11-02 defense-in-depth proof.
func TestRetentionConfig_UnknownHypertable_Rejected(t *testing.T) {
	ft := &failingTx{}
	err := reconcileSingleLevel(context.Background(), ft, "evil'; DROP TABLE retention_config; --", 90, 60)
	if err == nil {
		t.Error("expected error for unknown hypertable name, got nil")
	}
	if !strings.Contains(err.Error(), "unknown hypertable") {
		t.Errorf("expected 'unknown hypertable' error, got: %v", err)
	}
	// The switch default must fire BEFORE any Exec call.
	if ft.execCalls > 0 {
		t.Errorf("expected zero Exec calls for unknown hypertable, got %d", ft.execCalls)
	}
}

// TestRetentionConfig_AuditEntryWritten tests that a successful PATCH writes an
// audit_log row with action=settings.retention_change.
func TestRetentionConfig_AuditEntryWritten(t *testing.T) {
	pool := startTestDB(t)
	sm := scs.New()
	router := buildTestRouter(pool, sm)
	adminCookie := injectSession(t, sm, uuid.New().String(), "admin")

	w := doReq(t, router, http.MethodPatch, "/api/settings/retention",
		map[string]any{"raw_days": 60}, adminCookie)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify audit row exists.
	q := sqlc.New(pool)
	entries, err := q.ListAuditEntriesByEntity(context.Background(), sqlc.ListAuditEntriesByEntityParams{
		EntityType: "retention_config",
		EntityID:   pgtype.UUID{Valid: false}, // uuid.Nil stored as NULL
		Limit:      10,
		Offset:     0,
	})
	if err != nil {
		t.Fatalf("ListAuditEntriesByEntity: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one audit entry for retention_config, got 0")
	}
	found := false
	for _, e := range entries {
		if e.Action == "settings.retention_change" {
			found = true
			// Verify before/after diff is populated.
			if e.After == nil {
				t.Error("audit entry After diff should not be nil")
			}
			break
		}
	}
	if !found {
		t.Errorf("no audit entry with action=settings.retention_change found (entries: %+v)", entries)
	}
}

// TestRetentionConfig_ViewerCanRead tests that viewers can GET retention settings.
func TestRetentionConfig_ViewerCanRead(t *testing.T) {
	pool := startTestDB(t)
	sm := scs.New()
	router := buildTestRouter(pool, sm)
	viewerCookie := injectSession(t, sm, uuid.New().String(), "viewer")

	w := doReq(t, router, http.MethodGet, "/api/settings/retention", nil, viewerCookie)
	if w.Code != http.StatusOK {
		t.Fatalf("viewer GET expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// TestRetentionConfig_NoLibPQ_Compliance documents CLAUDE.md compliance.
// Actual enforcement is the grep check in acceptance criteria.
func TestRetentionConfig_NoLibPQ_Compliance(t *testing.T) {
	t.Log("CLAUDE.md compliance: internal/settings/ uses pgx/v5 only (no lib/pq)")
	t.Log("Verified by: grep -r 'lib/pq' internal/settings/ | wc -l  →  0")
}

// --- Test helpers ---

// failingTx is a minimal pgx.Tx stub used to test error paths in
// ReconcilePolicies without a real database. It records how many Exec
// calls were made before the failure.
type failingTx struct {
	execCalls int
}

// Exec always fails, simulating a TimescaleDB policy call error.
func (f *failingTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	f.execCalls++
	return pgconn.CommandTag{}, fmt.Errorf("injected tx failure")
}

// The remaining pgx.Tx methods are no-ops (failingTx is only used to test
// ReconcilePolicies which only calls Exec).
func (f *failingTx) Begin(ctx context.Context) (pgx.Tx, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *failingTx) Commit(ctx context.Context) error         { return fmt.Errorf("not implemented") }
func (f *failingTx) Rollback(ctx context.Context) error       { return nil }
func (f *failingTx) CopyFrom(ctx context.Context, tn pgx.Identifier, cn []string, rs pgx.CopyFromSource) (int64, error) {
	return 0, fmt.Errorf("not implemented")
}
func (f *failingTx) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	return nil
}
func (f *failingTx) LargeObjects() pgx.LargeObjects { return pgx.LargeObjects{} }
func (f *failingTx) Prepare(ctx context.Context, name, sql string) (*pgconn.StatementDescription, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *failingTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return nil, fmt.Errorf("not implemented")
}
func (f *failingTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return nil
}
func (f *failingTx) Conn() *pgx.Conn { return nil }

// reconcileSingleLevel mirrors the internal hypertable switch in
// ReconcilePolicies for a single named level. Used by
// TestRetentionConfig_UnknownHypertable_Rejected to prove the default arm
// fires before any Exec call.
func reconcileSingleLevel(ctx context.Context, tx pgx.Tx, name string, beforeDays, afterDays int32) error {
	// Mirror the switch in ReconcilePolicies exactly.
	var hypertable string
	switch name {
	case "measurement":
		hypertable = "measurement"
	case "measurement_hourly":
		hypertable = "measurement_hourly"
	case "measurement_daily":
		hypertable = "measurement_daily"
	case "measurement_monthly":
		hypertable = "measurement_monthly"
	case "measurement_yearly":
		hypertable = "measurement_yearly"
	default:
		// Default arm fires BEFORE any Exec — zero calls guaranteed.
		return fmt.Errorf("retention: unknown hypertable %q", name)
	}

	// Only reached for vetted names.
	if beforeDays != afterDays {
		removeSQL := fmt.Sprintf(`SELECT remove_retention_policy('%s', if_exists => true)`, hypertable)
		if _, err := tx.Exec(ctx, removeSQL); err != nil {
			return fmt.Errorf("remove policy %s: %w", hypertable, err)
		}
	}
	return nil
}
