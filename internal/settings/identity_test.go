package settings

import (
	"context"
	"net/http"
	"testing"

	"github.com/alexedwards/scs/v2"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	sqlc "github.com/shifter-io/shifter/internal/db/sqlc"
)

// buildIdentityRouter builds a test chi.Router with identity routes registered.
func buildIdentityRouter(t *testing.T, sm *scs.SessionManager) http.Handler {
	t.Helper()
	pool := startTestDB(t)
	deps := Deps{Pool: pool, Queries: sqlc.New(pool)}
	r := chi.NewRouter()
	r.Use(sm.LoadAndSave)
	RegisterIdentityRoutes(r, deps, sm)
	return r
}

// seedIdentity upserts a known install_identity row for tests.
func seedIdentity(t *testing.T, pool *pgxpool.Pool, displayName, timezone, units string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO install_identity (id, display_name, timezone, units)
		 VALUES (1, $1, $2, $3::units_system)
		 ON CONFLICT (id) DO UPDATE SET
		   display_name = EXCLUDED.display_name,
		   timezone     = EXCLUDED.timezone,
		   units        = EXCLUDED.units`,
		displayName, timezone, units)
	if err != nil {
		t.Fatalf("seedIdentity(%q, %q, %q): %v", displayName, timezone, units, err)
	}
}

// TestGetIdentity_ReturnsFields verifies GET /api/settings/identity returns 200
// with install_id, site_name, display_name, timezone, units, and version fields.
func TestGetIdentity_ReturnsFields(t *testing.T) {
	pool := startTestDB(t)
	sm := scs.New()
	deps := Deps{Pool: pool, Queries: sqlc.New(pool)}

	seedIdentity(t, pool, "Acme Water", "UTC", "metric")

	r := chi.NewRouter()
	r.Use(sm.LoadAndSave)
	RegisterIdentityRoutes(r, deps, sm)

	adminID := uuid.New().String()
	seedTestUser(t, pool, adminID, "admin")
	cookie := injectSession(t, sm, adminID, "admin")

	w := doReq(t, r, http.MethodGet, "/api/settings/identity", nil, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/settings/identity: want 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp IdentityResponse
	decodeJSON(t, w, &resp)

	if resp.InstallID != "1" {
		t.Errorf("install_id: want \"1\", got %q", resp.InstallID)
	}
	if resp.SiteName != "Acme Water" {
		t.Errorf("site_name: want \"Acme Water\", got %q", resp.SiteName)
	}
	if resp.DisplayName != "Acme Water" {
		t.Errorf("display_name: want \"Acme Water\", got %q", resp.DisplayName)
	}
	if resp.Timezone != "UTC" {
		t.Errorf("timezone: want \"UTC\", got %q", resp.Timezone)
	}
	if resp.Units != "metric" {
		t.Errorf("units: want \"metric\", got %q", resp.Units)
	}
	if resp.Version == "" {
		t.Error("version: want non-empty string, got empty")
	}
}

// TestGetIdentity_RequiresAuth verifies unauthenticated GET returns 401.
// Passes an empty cookie so SCS finds no valid session.
func TestGetIdentity_RequiresAuth(t *testing.T) {
	sm := scs.New()
	r := buildIdentityRouter(t, sm)

	// Pass an empty cookie — SCS will find no valid session → RequireAction → 401.
	w := doReq(t, r, http.MethodGet, "/api/settings/identity", nil, http.Cookie{})
	if w.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated GET: want 401, got %d: %s", w.Code, w.Body.String())
	}
}

// TestPatchIdentity_UpdatesFields verifies PATCH /api/settings/identity with
// {"display_name":"Updated Name"} returns 200 and persists the change.
func TestPatchIdentity_UpdatesFields(t *testing.T) {
	pool := startTestDB(t)
	sm := scs.New()
	deps := Deps{Pool: pool, Queries: sqlc.New(pool)}

	seedIdentity(t, pool, "Original Name", "UTC", "metric")

	r := chi.NewRouter()
	r.Use(sm.LoadAndSave)
	RegisterIdentityRoutes(r, deps, sm)

	adminID := uuid.New().String()
	seedTestUser(t, pool, adminID, "admin")
	cookie := injectSession(t, sm, adminID, "admin")

	w := doReq(t, r, http.MethodPatch, "/api/settings/identity",
		map[string]any{"display_name": "Updated Name"}, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH: want 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp IdentityResponse
	decodeJSON(t, w, &resp)
	if resp.DisplayName != "Updated Name" {
		t.Errorf("display_name in response: want \"Updated Name\", got %q", resp.DisplayName)
	}

	// Verify persistence via GET.
	wGet := doReq(t, r, http.MethodGet, "/api/settings/identity", nil, cookie)
	var getResp IdentityResponse
	decodeJSON(t, wGet, &getResp)
	if getResp.DisplayName != "Updated Name" {
		t.Errorf("display_name after GET: want \"Updated Name\", got %q", getResp.DisplayName)
	}
}

// TestPatchIdentity_ViewerForbidden verifies viewer role PATCH → 403.
func TestPatchIdentity_ViewerForbidden(t *testing.T) {
	sm := scs.New()
	r := buildIdentityRouter(t, sm)
	viewerCookie := injectSession(t, sm, uuid.New().String(), "viewer")

	w := doReq(t, r, http.MethodPatch, "/api/settings/identity",
		map[string]any{"display_name": "Attempt"}, viewerCookie)
	if w.Code != http.StatusForbidden {
		t.Errorf("viewer PATCH: want 403, got %d: %s", w.Code, w.Body.String())
	}
}

// TestPatchIdentity_WritesAuditRow verifies PATCH writes an audit row with
// action='settings.identity_update' and entity_type='install_identity'.
func TestPatchIdentity_WritesAuditRow(t *testing.T) {
	pool := startTestDB(t)
	sm := scs.New()
	deps := Deps{Pool: pool, Queries: sqlc.New(pool)}

	seedIdentity(t, pool, "Audit Test", "UTC", "metric")

	r := chi.NewRouter()
	r.Use(sm.LoadAndSave)
	RegisterIdentityRoutes(r, deps, sm)

	adminID := uuid.New().String()
	seedTestUser(t, pool, adminID, "admin")
	cookie := injectSession(t, sm, adminID, "admin")

	w := doReq(t, r, http.MethodPatch, "/api/settings/identity",
		map[string]any{"display_name": "After Audit"}, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH: want 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify audit row via ListAuditEntriesByEntity.
	// EntityID is uuid.Nil (all-zeros) stored as a valid non-null UUID —
	// mirrors the audit.WriteEntry call in PatchIdentityHandler.
	q := sqlc.New(pool)
	entries, err := q.ListAuditEntriesByEntity(context.Background(), sqlc.ListAuditEntriesByEntityParams{
		EntityType: "install_identity",
		EntityID:   pgtype.UUID{Bytes: [16]byte{}, Valid: true}, // uuid.Nil, stored as valid all-zeros UUID
		Limit:      10,
		Offset:     0,
	})
	if err != nil {
		t.Fatalf("ListAuditEntriesByEntity: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one audit entry for install_identity, got 0")
	}
	found := false
	for _, e := range entries {
		if e.Action == "settings.identity_update" {
			found = true
			if e.After == nil {
				t.Error("audit entry After diff should not be nil")
			}
			break
		}
	}
	if !found {
		t.Errorf("no audit entry with action=settings.identity_update found (entries: %+v)", entries)
	}
}

// TestPatchIdentity_InvalidUnits verifies PATCH with an invalid units value
// returns 400 with {"error":"invalid_units"}.
func TestPatchIdentity_InvalidUnits(t *testing.T) {
	pool := startTestDB(t)
	sm := scs.New()
	deps := Deps{Pool: pool, Queries: sqlc.New(pool)}

	seedIdentity(t, pool, "Units Test", "UTC", "metric")

	r := chi.NewRouter()
	r.Use(sm.LoadAndSave)
	RegisterIdentityRoutes(r, deps, sm)

	adminID := uuid.New().String()
	seedTestUser(t, pool, adminID, "admin")
	cookie := injectSession(t, sm, adminID, "admin")

	w := doReq(t, r, http.MethodPatch, "/api/settings/identity",
		map[string]any{"units": "gallons"}, cookie)
	if w.Code != http.StatusBadRequest {
		t.Errorf("invalid units: want 400, got %d: %s", w.Code, w.Body.String())
	}
	var errResp map[string]string
	decodeJSON(t, w, &errResp)
	if errResp["error"] != "invalid_units" {
		t.Errorf("error code: want \"invalid_units\", got %q", errResp["error"])
	}
}

// TestMigration0049_AddsVocab verifies migration 0049 extended the audit_log
// CHECK constraints to accept 'settings.identity_update' and 'install_identity'.
func TestMigration0049_AddsVocab(t *testing.T) {
	pool := startTestDB(t)

	adminID := uuid.New().String()
	seedTestUser(t, pool, adminID, "admin")

	// Valid insert: settings.identity_update + install_identity MUST succeed.
	_, insertErr := pool.Exec(context.Background(),
		`INSERT INTO audit_log (user_id, action, entity_type, entity_id)
		 VALUES ($1::uuid, 'settings.identity_update', 'install_identity',
		         '00000000-0000-0000-0000-000000000000'::uuid)`,
		adminID)
	if insertErr != nil {
		t.Fatalf("INSERT with settings.identity_update should succeed after migration 0049: %v", insertErr)
	}

	// Invalid action MUST fail the CHECK constraint (code 23514).
	_, badErr := pool.Exec(context.Background(),
		`INSERT INTO audit_log (user_id, action, entity_type, entity_id)
		 VALUES ($1::uuid, 'settings.identity_update_typo', 'install_identity',
		         '00000000-0000-0000-0000-000000000001'::uuid)`,
		adminID)
	if badErr == nil {
		t.Error("INSERT with settings.identity_update_typo should fail CHECK constraint, but succeeded")
	}
}
