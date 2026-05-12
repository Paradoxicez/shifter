package api

// Tests for the 4 catalog HTTP handlers.
// V2-VEND-01 backend: ListCatalog, GetCatalogEntry, ImportFromCatalog, ApplyCatalogUpdate.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
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

// catalogHandlerSetup creates a real Postgres testcontainer, runs migrations,
// and returns the httptest server, cookie-jar HTTP client, and pgxpool.
func catalogHandlerSetup(t *testing.T, suffix string, role string) (srv *httptest.Server, cli *http.Client, pool *pgxpool.Pool) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping: -short")
	}
	ctx := context.Background()
	pgPool := testsupport.StartPostgres(t)
	require.NoError(t, db.RunMigrations(ctx, pgPool, noopLog()))

	var userID string
	require.NoError(t, pgPool.QueryRow(ctx,
		`INSERT INTO "user" (email, name, password_hash, role)
		 VALUES ('`+role+`-catalog-`+suffix+`@ex.com', 'U', 'x', '`+role+`') RETURNING id::text`,
	).Scan(&userID))

	sm := auth.NewSessionManager(pgPool, true, 8*time.Hour, 24*time.Hour)
	deps := CatalogDeps{Pool: pgPool, SessionMgr: sm}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Post("/test/seed", func(w http.ResponseWriter, req *http.Request) {
		if err := auth.PutUser(req.Context(), sm, auth.User{ID: userID, Role: role}); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	RegisterCatalogRoutes(r, deps)

	server := httptest.NewServer(sm.LoadAndSave(r))
	t.Cleanup(server.Close)

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	res, err := client.Post(server.URL+"/test/seed", "", nil)
	require.NoError(t, err)
	res.Body.Close()

	return server, client, pgPool
}

// TestListCatalogHandler_AllStatuses — seeds 1 imported profile @ v1.0.0,
// simulates catalog (which ships v1.0.0 entries), asserts response shape.
func TestListCatalogHandler_AllStatuses(t *testing.T) {
	srv, cli, _ := catalogHandlerSetup(t, "list", "admin")

	res, err := cli.Get(srv.URL + "/api/catalog")
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusOK, res.StatusCode)
	var body struct {
		Entries  []map[string]any `json:"entries"`
		Profiles []map[string]any `json:"profiles"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&body))
	// 4 catalog entries (axioma_w1, acrel_adl200, acrel_adw300, itron_kinmy_lora)
	require.Equal(t, 4, len(body.Entries), "should have 4 catalog entries")
	// Each entry must have slug, name, vendor
	for _, e := range body.Entries {
		require.NotEmpty(t, e["slug"])
		require.NotEmpty(t, e["name"])
		require.NotEmpty(t, e["vendor"])
	}
	// Profiles includes catalog-sourced rows (4 seeded by migration 0050) plus
	// not-installed rows for entries that have no matching profile. Since 0050
	// seeds all 4, profiles array should have at least 4 entries.
	require.GreaterOrEqual(t, len(body.Profiles), 4)
	// Every profile row must have a status field
	for _, p := range body.Profiles {
		require.NotEmpty(t, p["status"])
	}
}

// TestImportFromCatalogHandler_HappyPath — POST {slug: "axioma_w1"} for a
// new (not-yet-imported via API) slug should fail with 409 since migration
// 0050 already seeded it. We test with a fresh non-seeded slug if available,
// otherwise we just verify the route is working. Actually, since 0050 seeds
// all 4 slugs, importing any will get 409. We verify the handler decodes
// correctly by testing with a completely unknown slug path first (404), then
// test with a valid slug but note 409 due to seed data.
//
// To test a true happy path we delete the seeded itron row first.
func TestImportFromCatalogHandler_HappyPath(t *testing.T) {
	srv, cli, pool := catalogHandlerSetup(t, "import-happy", "admin")
	ctx := context.Background()

	// Delete the seeded itron profile so we can import it fresh.
	_, err := pool.Exec(ctx, `DELETE FROM device_profile WHERE slug = 'itron_kinmy_lora'`)
	require.NoError(t, err)

	res, err := cli.Post(srv.URL+"/api/catalog/import",
		"application/json",
		bytes.NewBufferString(`{"slug":"itron_kinmy_lora"}`))
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusOK, res.StatusCode)
	var resp map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&resp))
	require.NotEmpty(t, resp["profile_id"], "import must return profile_id")

	// Verify audit row was written.
	var auditCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE action = 'catalog.profile.imported'`,
	).Scan(&auditCount))
	require.Equal(t, 1, auditCount, "import must write audit row with catalog.profile.imported")
}

// TestImportFromCatalogHandler_UnknownSlug — POST {slug: "no-such-vendor"} → 404.
func TestImportFromCatalogHandler_UnknownSlug(t *testing.T) {
	srv, cli, _ := catalogHandlerSetup(t, "import-unknown", "admin")

	res, err := cli.Post(srv.URL+"/api/catalog/import",
		"application/json",
		bytes.NewBufferString(`{"slug":"no-such-vendor-ever"}`))
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusNotFound, res.StatusCode)
}

// TestImportFromCatalogHandler_DuplicateSlug — axioma_w1 is already seeded by
// migration 0050; importing it again must return 409.
func TestImportFromCatalogHandler_DuplicateSlug(t *testing.T) {
	srv, cli, _ := catalogHandlerSetup(t, "import-dup", "admin")

	res, err := cli.Post(srv.URL+"/api/catalog/import",
		"application/json",
		bytes.NewBufferString(`{"slug":"axioma_w1"}`))
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusConflict, res.StatusCode)
}

// TestApplyCatalogUpdateHandler_PartialAccept — POST {accepted_fields:
// ["codec_js"], target_version: "1.1.0"} on the axioma_w1 profile.
// Since the catalog entry is at v1.0.0 and target_version is "1.1.0" which
// is greater, the update should succeed. (The handler accepts any target_version
// that is semver-greater than current, regardless of whether the catalog itself
// has that version.)
// We set catalog_source_version to "0.9.0" so "1.1.0" qualifies as an upgrade.
func TestApplyCatalogUpdateHandler_PartialAccept(t *testing.T) {
	srv, cli, pool := catalogHandlerSetup(t, "update-partial", "admin")
	ctx := context.Background()

	// Set catalog_source_version to 0.9.0 so 1.0.0 or 1.1.0 qualifies as an upgrade.
	var profileID string
	require.NoError(t, pool.QueryRow(ctx,
		`UPDATE device_profile
		 SET catalog_source = 'axioma_w1', catalog_source_version = '0.9.0'
		 WHERE slug = 'axioma_w1'
		 RETURNING id::text`,
	).Scan(&profileID))

	body := map[string]any{
		"accepted_fields": []string{"codec_js"},
		"target_version":  "1.1.0",
	}
	bodyJSON, _ := json.Marshal(body)
	res, err := cli.Post(srv.URL+"/api/catalog/"+profileID+"/update",
		"application/json",
		bytes.NewBuffer(bodyJSON))
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusOK, res.StatusCode)
	var resp map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&resp))
	require.NotEmpty(t, resp["updated_at"])
	require.Equal(t, "1.1.0", resp["new_version"])

	// codec_js_synced_at must be NULL after update (cleared by ApplyCatalogUpdate).
	var syncedAtIsNull bool
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT codec_js_synced_at IS NULL FROM device_profile WHERE slug = 'axioma_w1'`,
	).Scan(&syncedAtIsNull))
	require.True(t, syncedAtIsNull, "codec_js_synced_at must be NULL after catalog update")

	// customer_edited must be true because only 1 of 8 fields was accepted.
	var customerEdited bool
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT customer_edited FROM device_profile WHERE slug = 'axioma_w1'`,
	).Scan(&customerEdited))
	require.True(t, customerEdited, "customer_edited must be true when not all fields accepted")
}

// TestApplyCatalogUpdateHandler_FullAccept_CustomerEditedFalse — POST with all
// 8 allowed fields accepted. customer_edited must remain false (operator accepted
// everything from catalog).
func TestApplyCatalogUpdateHandler_FullAccept_CustomerEditedFalse(t *testing.T) {
	srv, cli, pool := catalogHandlerSetup(t, "update-full", "admin")
	ctx := context.Background()

	var profileID string
	require.NoError(t, pool.QueryRow(ctx,
		`UPDATE device_profile
		 SET catalog_source = 'axioma_w1', catalog_source_version = '0.9.0',
		     customer_edited = FALSE
		 WHERE slug = 'axioma_w1'
		 RETURNING id::text`,
	).Scan(&profileID))

	body := map[string]any{
		"accepted_fields": []string{
			"codec_js", "capabilities", "battery_curve",
			"expected_uplink_interval_seconds", "offline_threshold_multiplier",
			"anomaly_compatibility", "counter_modulus", "mac_version",
		},
		"target_version": "1.0.0",
	}
	bodyJSON, _ := json.Marshal(body)
	res, err := cli.Post(srv.URL+"/api/catalog/"+profileID+"/update",
		"application/json",
		bytes.NewBuffer(bodyJSON))
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusOK, res.StatusCode)

	// customer_edited must be false (all fields accepted from catalog).
	var customerEdited bool
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT customer_edited FROM device_profile WHERE slug = 'axioma_w1'`,
	).Scan(&customerEdited))
	require.False(t, customerEdited, "customer_edited must be false when all 8 fields accepted")
}

// TestApplyCatalogUpdateHandler_AuditWritten — asserts audit row with action
// catalog.profile.updated is inserted on a successful update.
func TestApplyCatalogUpdateHandler_AuditWritten(t *testing.T) {
	srv, cli, pool := catalogHandlerSetup(t, "update-audit", "admin")
	ctx := context.Background()

	var profileID string
	require.NoError(t, pool.QueryRow(ctx,
		`UPDATE device_profile
		 SET catalog_source = 'axioma_w1', catalog_source_version = '0.9.0'
		 WHERE slug = 'axioma_w1'
		 RETURNING id::text`,
	).Scan(&profileID))

	body := map[string]any{
		"accepted_fields": []string{"codec_js"},
		"target_version":  "1.0.0",
	}
	bodyJSON, _ := json.Marshal(body)
	res, err := cli.Post(srv.URL+"/api/catalog/"+profileID+"/update",
		"application/json",
		bytes.NewBuffer(bodyJSON))
	require.NoError(t, err)
	res.Body.Close()

	require.Equal(t, http.StatusOK, res.StatusCode)

	var auditCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FROM audit_log WHERE action = 'catalog.profile.updated'`,
	).Scan(&auditCount))
	require.Equal(t, 1, auditCount, "update must write audit row with catalog.profile.updated")
}

// TestApplyCatalogUpdateHandler_UnknownField — POST {accepted_fields: ["bogus"],
// ...} must return 400 + body {"error":"unknown_field","field":"bogus"}.
func TestApplyCatalogUpdateHandler_UnknownField(t *testing.T) {
	srv, cli, pool := catalogHandlerSetup(t, "update-unknown-field", "admin")
	ctx := context.Background()

	var profileID string
	require.NoError(t, pool.QueryRow(ctx,
		`UPDATE device_profile
		 SET catalog_source = 'axioma_w1', catalog_source_version = '0.9.0'
		 WHERE slug = 'axioma_w1'
		 RETURNING id::text`,
	).Scan(&profileID))

	body := map[string]any{
		"accepted_fields": []string{"bogus"},
		"target_version":  "1.0.0",
	}
	bodyJSON, _ := json.Marshal(body)
	res, err := cli.Post(srv.URL+"/api/catalog/"+profileID+"/update",
		"application/json",
		bytes.NewBuffer(bodyJSON))
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusBadRequest, res.StatusCode)
	var errBody map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&errBody))
	require.Equal(t, "unknown_field", errBody["error"])
	require.Equal(t, "bogus", errBody["field"])
}

// TestApplyCatalogUpdateHandler_RejectDowngrade — profile installed at v1.1.0,
// POST target_version: "1.0.0" must return 400 (downgrade rejected per T-07-04-07).
func TestApplyCatalogUpdateHandler_RejectDowngrade(t *testing.T) {
	srv, cli, pool := catalogHandlerSetup(t, "update-downgrade", "admin")
	ctx := context.Background()

	var profileID string
	require.NoError(t, pool.QueryRow(ctx,
		`UPDATE device_profile
		 SET catalog_source = 'axioma_w1', catalog_source_version = '1.1.0'
		 WHERE slug = 'axioma_w1'
		 RETURNING id::text`,
	).Scan(&profileID))

	body := map[string]any{
		"accepted_fields": []string{"codec_js"},
		"target_version":  "1.0.0",
	}
	bodyJSON, _ := json.Marshal(body)
	res, err := cli.Post(srv.URL+"/api/catalog/"+profileID+"/update",
		"application/json",
		bytes.NewBuffer(bodyJSON))
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusBadRequest, res.StatusCode)
}

// TestImportFromCatalogHandler_ViewerForbidden — viewer session returns 403.
func TestImportFromCatalogHandler_ViewerForbidden(t *testing.T) {
	srv, cli, _ := catalogHandlerSetup(t, "import-viewer", "viewer")

	res, err := cli.Post(srv.URL+"/api/catalog/import",
		"application/json",
		bytes.NewBufferString(`{"slug":"axioma_w1"}`))
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusForbidden, res.StatusCode)
}

// TestGetCatalogEntryHandler_HappyPath — GET /api/catalog/axioma_w1 returns 200 + JSON.
func TestGetCatalogEntryHandler_HappyPath(t *testing.T) {
	srv, cli, _ := catalogHandlerSetup(t, "get-entry", "admin")

	res, err := cli.Get(srv.URL + "/api/catalog/axioma_w1")
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusOK, res.StatusCode)
	var entry map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&entry))
	require.Equal(t, "axioma_w1", entry["slug"])
}

// TestGetCatalogEntryHandler_NotFound — GET /api/catalog/no-such-slug → 404.
func TestGetCatalogEntryHandler_NotFound(t *testing.T) {
	srv, cli, _ := catalogHandlerSetup(t, "get-404", "admin")

	res, err := cli.Get(srv.URL + "/api/catalog/no-such-slug-ever")
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusNotFound, res.StatusCode)
}

// verifyViewerCatalogReadAllowed — viewer CAN GET /api/catalog (ActionCatalogRead is viewer-accessible).
func TestViewerCatalogReadAllowed(t *testing.T) {
	srv, cli, _ := catalogHandlerSetup(t, "viewer-read", "viewer")

	res, err := cli.Get(srv.URL + "/api/catalog")
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusOK, res.StatusCode,
		"viewer must be able to GET /api/catalog (ActionCatalogRead)")
}

// viewerCannotUpdate — viewer POST to /api/catalog/{id}/update → 403.
func TestViewerCatalogUpdateForbidden(t *testing.T) {
	srv, cli, _ := catalogHandlerSetup(t, "viewer-update", "viewer")

	res, err := cli.Post(srv.URL+"/api/catalog/"+uuid.New().String()+"/update",
		"application/json",
		bytes.NewBufferString(`{"accepted_fields":["codec_js"],"target_version":"1.0.0"}`))
	require.NoError(t, err)
	defer res.Body.Close()

	require.Equal(t, http.StatusForbidden, res.StatusCode,
		"viewer must NOT be able to POST to /api/catalog/{id}/update (T-07-04-01)")
}
